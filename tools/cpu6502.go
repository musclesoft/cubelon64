package main

import "fmt"

// CPU6502 is a self-contained MOS 6502 emulator used to run and validate the
// C64 tracker player. SID register writes ($D400–$D418) are intercepted and
// appended to SIDWrites with their frame number.
//
// Ported from nin64k/tools/forge/simulate/vm.go.
type CPU6502 struct {
	A, X, Y byte
	SP       byte
	PC       uint16
	P        byte
	Memory   [65536]byte
	SIDWrites    []SIDWrite
	Cycles       int64
	CycleBase    int64
	pageCrossed  bool
	CurrentFrame int
	DebugAddrs   map[uint16]string
	DebugFrameLimit int

	// IORead/IOWrite intercept memory-mapped I/O accesses before falling back
	// to plain RAM. Returning ok=false means "not intercepted, use RAM".
	IORead  func(addr uint16) (val byte, ok bool)
	IOWrite func(addr uint16, val byte) (ok bool)
}

const (
	cpu6502FlagC byte = 1 << 0
	cpu6502FlagZ byte = 1 << 1
	cpu6502FlagI byte = 1 << 2
	cpu6502FlagD byte = 1 << 3
	cpu6502FlagB byte = 1 << 4
	cpu6502FlagU byte = 1 << 5
	cpu6502FlagV byte = 1 << 6
	cpu6502FlagN byte = 1 << 7
)

func newCPU6502() *CPU6502 {
	return &CPU6502{SP: 0xFF, P: cpu6502FlagU | cpu6502FlagI}
}

func (c *CPU6502) cpuRead(addr uint16) byte {
	if c.IORead != nil {
		if val, ok := c.IORead(addr); ok {
			return val
		}
	}
	return c.Memory[addr]
}
func (c *CPU6502) cpuRead16(addr uint16) uint16 {
	return uint16(c.cpuRead(addr)) | uint16(c.cpuRead(addr+1))<<8
}
func (c *CPU6502) cpuWrite(addr uint16, val byte) {
	if addr >= 0xD400 && addr <= 0xD418 {
		c.SIDWrites = append(c.SIDWrites, SIDWrite{Cycle: c.Cycles - c.CycleBase, Frame: uint32(c.CurrentFrame), Addr: addr, Value: val})
	}
	if c.DebugAddrs != nil && c.CurrentFrame < c.DebugFrameLimit {
		if name, ok := c.DebugAddrs[addr]; ok {
			fmt.Printf("  [f%d] WRITE %s ($%04X) = $%02X  (from PC=$%04X)\n", c.CurrentFrame, name, addr, val, c.PC)
		}
	}
	if c.IOWrite != nil && c.IOWrite(addr, val) {
		return
	}
	c.Memory[addr] = val
}

func (c *CPU6502) cpuPush(val byte)   { c.cpuWrite(0x0100|uint16(c.SP), val); c.SP-- }
func (c *CPU6502) cpuPull() byte      { c.SP++; return c.cpuRead(0x0100 | uint16(c.SP)) }
func (c *CPU6502) cpuPush16(val uint16) { c.cpuPush(byte(val >> 8)); c.cpuPush(byte(val)) }
func (c *CPU6502) cpuPull16() uint16  { return uint16(c.cpuPull()) | uint16(c.cpuPull())<<8 }

func (c *CPU6502) setZ(v byte) { if v == 0 { c.P |= cpu6502FlagZ } else { c.P &^= cpu6502FlagZ } }
func (c *CPU6502) setN(v byte) { if v&0x80 != 0 { c.P |= cpu6502FlagN } else { c.P &^= cpu6502FlagN } }
func (c *CPU6502) setZN(v byte) { c.setZ(v); c.setN(v) }
func (c *CPU6502) getFlag(f byte) bool { return c.P&f != 0 }
func (c *CPU6502) setFlag(f byte, v bool) { if v { c.P |= f } else { c.P &^= f } }

func (c *CPU6502) addrImm() uint16   { a := c.PC; c.PC++; return a }
func (c *CPU6502) addrZP() uint16    { a := uint16(c.Memory[c.PC]); c.PC++; return a }
func (c *CPU6502) addrZPX() uint16   { a := uint16(c.Memory[c.PC]+c.X); c.PC++; return a }
func (c *CPU6502) addrZPY() uint16   { a := uint16(c.Memory[c.PC]+c.Y); c.PC++; return a }
func (c *CPU6502) addrAbs() uint16   { lo := uint16(c.Memory[c.PC]); hi := uint16(c.Memory[c.PC+1]); c.PC += 2; return hi<<8 | lo }
func (c *CPU6502) indexed(base uint16, index byte) uint16 {
	addr := base + uint16(index)
	c.pageCrossed = base&0xFF00 != addr&0xFF00
	return addr
}
func (c *CPU6502) addrAbsX() uint16  { return c.indexed(c.addrAbs(), c.X) }
func (c *CPU6502) addrAbsY() uint16  { return c.indexed(c.addrAbs(), c.Y) }
func (c *CPU6502) addrIndX() uint16  { zp := c.Memory[c.PC]+c.X; c.PC++; return uint16(c.Memory[uint16(zp)]) | uint16(c.Memory[uint16(zp+1)])<<8 }
func (c *CPU6502) addrIndY() uint16  { zp := c.Memory[c.PC]; c.PC++; base := uint16(c.Memory[uint16(zp)]) | uint16(c.Memory[uint16(zp+1)])<<8; return c.indexed(base, c.Y) }

func (c *CPU6502) adc(val byte) {
	a, v := uint16(c.A), uint16(val)
	carry := uint16(0); if c.getFlag(cpu6502FlagC) { carry = 1 }
	sum := a + v + carry
	c.setFlag(cpu6502FlagC, sum > 0xFF)
	c.setFlag(cpu6502FlagV, (^(a^v))&(a^sum)&0x80 != 0)
	c.A = byte(sum); c.setZN(c.A)
}
func (c *CPU6502) sbc(val byte) { c.adc(^val) }
func (c *CPU6502) cmp(reg, val byte) { c.setFlag(cpu6502FlagC, reg >= val); c.setZN(reg - val) }
func (c *CPU6502) branch(cond bool) {
	offset := int8(c.Memory[c.PC]); c.PC++
	if cond {
		target := uint16(int32(c.PC) + int32(offset))
		c.Cycles++
		if target&0xFF00 != c.PC&0xFF00 { c.Cycles++ }
		c.PC = target
	}
}

// step executes one instruction and advances Cycles by its cost. Cycles is
// charged before the body runs, so SID writes are stamped with the cycle the
// instruction completes on.
func (c *CPU6502) step() bool {
	op := c.Memory[c.PC]
	c.pageCrossed = false
	c.Cycles += int64(cpu6502Cycles[op])
	returned := c.execute()
	if c.pageCrossed && cpu6502PageCrossPenalty[op] {
		c.Cycles++
	}
	return returned
}

func (c *CPU6502) execute() bool {
	op := c.Memory[c.PC]; c.PC++
	switch op {
	case 0xA9: c.A = c.cpuRead(c.addrImm()); c.setZN(c.A)
	case 0xA5: c.A = c.cpuRead(c.addrZP()); c.setZN(c.A)
	case 0xB5: c.A = c.cpuRead(c.addrZPX()); c.setZN(c.A)
	case 0xAD: c.A = c.cpuRead(c.addrAbs()); c.setZN(c.A)
	case 0xBD: c.A = c.cpuRead(c.addrAbsX()); c.setZN(c.A)
	case 0xB9: c.A = c.cpuRead(c.addrAbsY()); c.setZN(c.A)
	case 0xA1: c.A = c.cpuRead(c.addrIndX()); c.setZN(c.A)
	case 0xB1: c.A = c.cpuRead(c.addrIndY()); c.setZN(c.A)
	case 0xA2: c.X = c.cpuRead(c.addrImm()); c.setZN(c.X)
	case 0xA6: c.X = c.cpuRead(c.addrZP()); c.setZN(c.X)
	case 0xB6: c.X = c.cpuRead(c.addrZPY()); c.setZN(c.X)
	case 0xAE: c.X = c.cpuRead(c.addrAbs()); c.setZN(c.X)
	case 0xBE: c.X = c.cpuRead(c.addrAbsY()); c.setZN(c.X)
	case 0xA0: c.Y = c.cpuRead(c.addrImm()); c.setZN(c.Y)
	case 0xA4: c.Y = c.cpuRead(c.addrZP()); c.setZN(c.Y)
	case 0xB4: c.Y = c.cpuRead(c.addrZPX()); c.setZN(c.Y)
	case 0xAC: c.Y = c.cpuRead(c.addrAbs()); c.setZN(c.Y)
	case 0xBC: c.Y = c.cpuRead(c.addrAbsX()); c.setZN(c.Y)
	case 0x85: c.cpuWrite(c.addrZP(), c.A)
	case 0x95: c.cpuWrite(c.addrZPX(), c.A)
	case 0x8D: c.cpuWrite(c.addrAbs(), c.A)
	case 0x9D: c.cpuWrite(c.addrAbsX(), c.A)
	case 0x99: c.cpuWrite(c.addrAbsY(), c.A)
	case 0x81: c.cpuWrite(c.addrIndX(), c.A)
	case 0x91: c.cpuWrite(c.addrIndY(), c.A)
	case 0x86: c.cpuWrite(c.addrZP(), c.X)
	case 0x96: c.cpuWrite(c.addrZPY(), c.X)
	case 0x8E: c.cpuWrite(c.addrAbs(), c.X)
	case 0x84: c.cpuWrite(c.addrZP(), c.Y)
	case 0x94: c.cpuWrite(c.addrZPX(), c.Y)
	case 0x8C: c.cpuWrite(c.addrAbs(), c.Y)
	case 0xAA: c.X = c.A; c.setZN(c.X)
	case 0xA8: c.Y = c.A; c.setZN(c.Y)
	case 0x8A: c.A = c.X; c.setZN(c.A)
	case 0x98: c.A = c.Y; c.setZN(c.A)
	case 0xBA: c.X = c.SP; c.setZN(c.X)
	case 0x9A: c.SP = c.X
	case 0x48: c.cpuPush(c.A)
	case 0x68: c.A = c.cpuPull(); c.setZN(c.A)
	case 0x08: c.cpuPush(c.P | cpu6502FlagB | cpu6502FlagU)
	case 0x28: c.P = c.cpuPull()&^cpu6502FlagB | cpu6502FlagU
	case 0x29: c.A &= c.cpuRead(c.addrImm()); c.setZN(c.A)
	case 0x25: c.A &= c.cpuRead(c.addrZP()); c.setZN(c.A)
	case 0x35: c.A &= c.cpuRead(c.addrZPX()); c.setZN(c.A)
	case 0x2D: c.A &= c.cpuRead(c.addrAbs()); c.setZN(c.A)
	case 0x3D: c.A &= c.cpuRead(c.addrAbsX()); c.setZN(c.A)
	case 0x39: c.A &= c.cpuRead(c.addrAbsY()); c.setZN(c.A)
	case 0x21: c.A &= c.cpuRead(c.addrIndX()); c.setZN(c.A)
	case 0x31: c.A &= c.cpuRead(c.addrIndY()); c.setZN(c.A)
	case 0x09: c.A |= c.cpuRead(c.addrImm()); c.setZN(c.A)
	case 0x05: c.A |= c.cpuRead(c.addrZP()); c.setZN(c.A)
	case 0x15: c.A |= c.cpuRead(c.addrZPX()); c.setZN(c.A)
	case 0x0D: c.A |= c.cpuRead(c.addrAbs()); c.setZN(c.A)
	case 0x1D: c.A |= c.cpuRead(c.addrAbsX()); c.setZN(c.A)
	case 0x19: c.A |= c.cpuRead(c.addrAbsY()); c.setZN(c.A)
	case 0x01: c.A |= c.cpuRead(c.addrIndX()); c.setZN(c.A)
	case 0x11: c.A |= c.cpuRead(c.addrIndY()); c.setZN(c.A)
	case 0x49: c.A ^= c.cpuRead(c.addrImm()); c.setZN(c.A)
	case 0x45: c.A ^= c.cpuRead(c.addrZP()); c.setZN(c.A)
	case 0x55: c.A ^= c.cpuRead(c.addrZPX()); c.setZN(c.A)
	case 0x4D: c.A ^= c.cpuRead(c.addrAbs()); c.setZN(c.A)
	case 0x5D: c.A ^= c.cpuRead(c.addrAbsX()); c.setZN(c.A)
	case 0x59: c.A ^= c.cpuRead(c.addrAbsY()); c.setZN(c.A)
	case 0x41: c.A ^= c.cpuRead(c.addrIndX()); c.setZN(c.A)
	case 0x51: c.A ^= c.cpuRead(c.addrIndY()); c.setZN(c.A)
	case 0x24: val := c.cpuRead(c.addrZP()); c.setFlag(cpu6502FlagZ, c.A&val == 0); c.setFlag(cpu6502FlagV, val&0x40 != 0); c.setFlag(cpu6502FlagN, val&0x80 != 0)
	case 0x2C: val := c.cpuRead(c.addrAbs()); c.setFlag(cpu6502FlagZ, c.A&val == 0); c.setFlag(cpu6502FlagV, val&0x40 != 0); c.setFlag(cpu6502FlagN, val&0x80 != 0)
	case 0x69: c.adc(c.cpuRead(c.addrImm()))
	case 0x65: c.adc(c.cpuRead(c.addrZP()))
	case 0x75: c.adc(c.cpuRead(c.addrZPX()))
	case 0x6D: c.adc(c.cpuRead(c.addrAbs()))
	case 0x7D: c.adc(c.cpuRead(c.addrAbsX()))
	case 0x79: c.adc(c.cpuRead(c.addrAbsY()))
	case 0x61: c.adc(c.cpuRead(c.addrIndX()))
	case 0x71: c.adc(c.cpuRead(c.addrIndY()))
	case 0xE9: c.sbc(c.cpuRead(c.addrImm()))
	case 0xE5: c.sbc(c.cpuRead(c.addrZP()))
	case 0xF5: c.sbc(c.cpuRead(c.addrZPX()))
	case 0xED: c.sbc(c.cpuRead(c.addrAbs()))
	case 0xFD: c.sbc(c.cpuRead(c.addrAbsX()))
	case 0xF9: c.sbc(c.cpuRead(c.addrAbsY()))
	case 0xE1: c.sbc(c.cpuRead(c.addrIndX()))
	case 0xF1: c.sbc(c.cpuRead(c.addrIndY()))
	case 0xC9: c.cmp(c.A, c.cpuRead(c.addrImm()))
	case 0xC5: c.cmp(c.A, c.cpuRead(c.addrZP()))
	case 0xD5: c.cmp(c.A, c.cpuRead(c.addrZPX()))
	case 0xCD: c.cmp(c.A, c.cpuRead(c.addrAbs()))
	case 0xDD: c.cmp(c.A, c.cpuRead(c.addrAbsX()))
	case 0xD9: c.cmp(c.A, c.cpuRead(c.addrAbsY()))
	case 0xC1: c.cmp(c.A, c.cpuRead(c.addrIndX()))
	case 0xD1: c.cmp(c.A, c.cpuRead(c.addrIndY()))
	case 0xE0: c.cmp(c.X, c.cpuRead(c.addrImm()))
	case 0xE4: c.cmp(c.X, c.cpuRead(c.addrZP()))
	case 0xEC: c.cmp(c.X, c.cpuRead(c.addrAbs()))
	case 0xC0: c.cmp(c.Y, c.cpuRead(c.addrImm()))
	case 0xC4: c.cmp(c.Y, c.cpuRead(c.addrZP()))
	case 0xCC: c.cmp(c.Y, c.cpuRead(c.addrAbs()))
	case 0xE6: addr := c.addrZP(); val := c.cpuRead(addr)+1; c.cpuWrite(addr, val); c.setZN(val)
	case 0xF6: addr := c.addrZPX(); val := c.cpuRead(addr)+1; c.cpuWrite(addr, val); c.setZN(val)
	case 0xEE: addr := c.addrAbs(); val := c.cpuRead(addr)+1; c.cpuWrite(addr, val); c.setZN(val)
	case 0xFE: addr := c.addrAbsX(); val := c.cpuRead(addr)+1; c.cpuWrite(addr, val); c.setZN(val)
	case 0xC6: addr := c.addrZP(); val := c.cpuRead(addr)-1; c.cpuWrite(addr, val); c.setZN(val)
	case 0xD6: addr := c.addrZPX(); val := c.cpuRead(addr)-1; c.cpuWrite(addr, val); c.setZN(val)
	case 0xCE: addr := c.addrAbs(); val := c.cpuRead(addr)-1; c.cpuWrite(addr, val); c.setZN(val)
	case 0xDE: addr := c.addrAbsX(); val := c.cpuRead(addr)-1; c.cpuWrite(addr, val); c.setZN(val)
	case 0xE8: c.X++; c.setZN(c.X)
	case 0xC8: c.Y++; c.setZN(c.Y)
	case 0xCA: c.X--; c.setZN(c.X)
	case 0x88: c.Y--; c.setZN(c.Y)
	case 0x0A: c.setFlag(cpu6502FlagC, c.A&0x80 != 0); c.A <<= 1; c.setZN(c.A)
	case 0x06: addr := c.addrZP(); v := c.cpuRead(addr); c.setFlag(cpu6502FlagC, v&0x80 != 0); v <<= 1; c.cpuWrite(addr, v); c.setZN(v)
	case 0x16: addr := c.addrZPX(); v := c.cpuRead(addr); c.setFlag(cpu6502FlagC, v&0x80 != 0); v <<= 1; c.cpuWrite(addr, v); c.setZN(v)
	case 0x0E: addr := c.addrAbs(); v := c.cpuRead(addr); c.setFlag(cpu6502FlagC, v&0x80 != 0); v <<= 1; c.cpuWrite(addr, v); c.setZN(v)
	case 0x1E: addr := c.addrAbsX(); v := c.cpuRead(addr); c.setFlag(cpu6502FlagC, v&0x80 != 0); v <<= 1; c.cpuWrite(addr, v); c.setZN(v)
	case 0x4A: c.setFlag(cpu6502FlagC, c.A&0x01 != 0); c.A >>= 1; c.setZN(c.A)
	case 0x46: addr := c.addrZP(); v := c.cpuRead(addr); c.setFlag(cpu6502FlagC, v&0x01 != 0); v >>= 1; c.cpuWrite(addr, v); c.setZN(v)
	case 0x56: addr := c.addrZPX(); v := c.cpuRead(addr); c.setFlag(cpu6502FlagC, v&0x01 != 0); v >>= 1; c.cpuWrite(addr, v); c.setZN(v)
	case 0x4E: addr := c.addrAbs(); v := c.cpuRead(addr); c.setFlag(cpu6502FlagC, v&0x01 != 0); v >>= 1; c.cpuWrite(addr, v); c.setZN(v)
	case 0x5E: addr := c.addrAbsX(); v := c.cpuRead(addr); c.setFlag(cpu6502FlagC, v&0x01 != 0); v >>= 1; c.cpuWrite(addr, v); c.setZN(v)
	case 0x2A: carry := c.P & cpu6502FlagC; c.setFlag(cpu6502FlagC, c.A&0x80 != 0); c.A = c.A<<1 | carry; c.setZN(c.A)
	case 0x26: addr := c.addrZP(); v := c.cpuRead(addr); carry := c.P & cpu6502FlagC; c.setFlag(cpu6502FlagC, v&0x80 != 0); v = v<<1 | carry; c.cpuWrite(addr, v); c.setZN(v)
	case 0x36: addr := c.addrZPX(); v := c.cpuRead(addr); carry := c.P & cpu6502FlagC; c.setFlag(cpu6502FlagC, v&0x80 != 0); v = v<<1 | carry; c.cpuWrite(addr, v); c.setZN(v)
	case 0x2E: addr := c.addrAbs(); v := c.cpuRead(addr); carry := c.P & cpu6502FlagC; c.setFlag(cpu6502FlagC, v&0x80 != 0); v = v<<1 | carry; c.cpuWrite(addr, v); c.setZN(v)
	case 0x3E: addr := c.addrAbsX(); v := c.cpuRead(addr); carry := c.P & cpu6502FlagC; c.setFlag(cpu6502FlagC, v&0x80 != 0); v = v<<1 | carry; c.cpuWrite(addr, v); c.setZN(v)
	case 0x6A: carry := c.P & cpu6502FlagC; c.setFlag(cpu6502FlagC, c.A&0x01 != 0); c.A = c.A>>1 | carry<<7; c.setZN(c.A)
	case 0x66: addr := c.addrZP(); v := c.cpuRead(addr); carry := c.P & cpu6502FlagC; c.setFlag(cpu6502FlagC, v&0x01 != 0); v = v>>1 | carry<<7; c.cpuWrite(addr, v); c.setZN(v)
	case 0x76: addr := c.addrZPX(); v := c.cpuRead(addr); carry := c.P & cpu6502FlagC; c.setFlag(cpu6502FlagC, v&0x01 != 0); v = v>>1 | carry<<7; c.cpuWrite(addr, v); c.setZN(v)
	case 0x6E: addr := c.addrAbs(); v := c.cpuRead(addr); carry := c.P & cpu6502FlagC; c.setFlag(cpu6502FlagC, v&0x01 != 0); v = v>>1 | carry<<7; c.cpuWrite(addr, v); c.setZN(v)
	case 0x7E: addr := c.addrAbsX(); v := c.cpuRead(addr); carry := c.P & cpu6502FlagC; c.setFlag(cpu6502FlagC, v&0x01 != 0); v = v>>1 | carry<<7; c.cpuWrite(addr, v); c.setZN(v)
	case 0x4C: c.PC = c.addrAbs()
	case 0x6C: addr := c.cpuRead16(c.PC); lo := uint16(c.cpuRead(addr)); hi := uint16(c.cpuRead((addr&0xFF00)|((addr+1)&0x00FF))); c.PC = hi<<8 | lo
	case 0x20: addr := c.addrAbs(); c.cpuPush16(c.PC - 1); c.PC = addr
	case 0x60: c.PC = c.cpuPull16() + 1; return true
	case 0x40: c.P = c.cpuPull()&^cpu6502FlagB | cpu6502FlagU; c.PC = c.cpuPull16()
	case 0x10: c.branch(!c.getFlag(cpu6502FlagN))
	case 0x30: c.branch(c.getFlag(cpu6502FlagN))
	case 0x50: c.branch(!c.getFlag(cpu6502FlagV))
	case 0x70: c.branch(c.getFlag(cpu6502FlagV))
	case 0x90: c.branch(!c.getFlag(cpu6502FlagC))
	case 0xB0: c.branch(c.getFlag(cpu6502FlagC))
	case 0xD0: c.branch(!c.getFlag(cpu6502FlagZ))
	case 0xF0: c.branch(c.getFlag(cpu6502FlagZ))
	case 0x18: c.P &^= cpu6502FlagC
	case 0x38: c.P |= cpu6502FlagC
	case 0x58: c.P &^= cpu6502FlagI
	case 0x78: c.P |= cpu6502FlagI
	case 0xB8: c.P &^= cpu6502FlagV
	case 0xD8: c.P &^= cpu6502FlagD
	case 0xF8: c.P |= cpu6502FlagD
	case 0xEA: // NOP
	case 0x00: c.PC++; c.cpuPush16(c.PC); c.cpuPush(c.P | cpu6502FlagB | cpu6502FlagU); c.P |= cpu6502FlagI; c.PC = c.cpuRead16(0xFFFE)
	default: panic(cpu6502Panicf("unknown opcode $%02X at $%04X", op, c.PC-1))
	}
	return false
}

// cpu6502Call runs the subroutine at addr, returning when RTS pops the sentinel.
func (c *CPU6502) cpu6502Call(addr uint16) {
	c.cpuPush16(0xFFFF)
	c.PC = addr
	for i := 0; i < 2000000; i++ {
		if c.step() && c.PC == 0x0000 {
			return
		}
	}
	panic(cpu6502Panicf("call to $%04X did not return (PC=$%04X)", addr, c.PC))
}

// cpu6502RunFrames calls playAddr once per frame, tagging SIDWrites with the frame number.
func (c *CPU6502) cpu6502RunFrames(initAddr, playAddr uint16, frames int) []SIDWrite {
	c.SIDWrites = nil
	c.CurrentFrame = 0
	c.cpu6502Call(initAddr)
	for f := 0; f < frames; f++ {
		c.CurrentFrame = f
		c.cpu6502Call(playAddr)
	}
	return c.SIDWrites
}

func cpu6502Panicf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
