package z80

// Registers holds Z80 register state for snapshots
type Registers struct {
	AF, BC, DE, HL     uint16
	AF_, BC_, DE_, HL_ uint16
	IX, IY, SP, PC     uint16
	I, R               byte
	IFF1, IFF2         bool
	IM                 byte
}

// CPU represents a Z80 processor
type CPU struct {
	// 8-bit registers
	A, F, B, C, D, E, H, L byte
	
	// Alternate registers
	A_, F_, B_, C_, D_, E_, H_, L_ byte
	
	// 16-bit registers
	IX, IY, SP, PC uint16
	
	// Special
	I, R byte
	IFF1, IFF2 bool
	IM byte
	
	// Memory interface
	Mem Memory
	
	// I/O interface  
	IO IO
	
	Halted bool
	Cycles uint64
}

type Memory interface {
	Read(addr uint16) byte
	Write(addr uint16, val byte)
}

type IO interface {
	In(port byte) byte
	In16(port uint16) byte
	Out(port byte, val byte, cycles uint64)
	Out16(port uint16, val byte, cycles uint64)
}

func New(mem Memory, io IO) *CPU {
	return &CPU{
		Mem: mem,
		IO: io,
		SP: 0xFFFF,
	}
}

// SetRegisters sets CPU registers from snapshot
func (z *CPU) SetRegisters(r Registers) {
	z.A = byte(r.AF >> 8)
	z.F = byte(r.AF)
	z.B = byte(r.BC >> 8)
	z.C = byte(r.BC)
	z.D = byte(r.DE >> 8)
	z.E = byte(r.DE)
	z.H = byte(r.HL >> 8)
	z.L = byte(r.HL)

	z.A_ = byte(r.AF_ >> 8)
	z.F_ = byte(r.AF_)
	z.B_ = byte(r.BC_ >> 8)
	z.C_ = byte(r.BC_)
	z.D_ = byte(r.DE_ >> 8)
	z.E_ = byte(r.DE_)
	z.H_ = byte(r.HL_ >> 8)
	z.L_ = byte(r.HL_)

	z.IX = r.IX
	z.IY = r.IY
	z.SP = r.SP
	z.PC = r.PC
	z.I = r.I
	z.R = r.R
	z.IFF1 = r.IFF1
	z.IFF2 = r.IFF2
	z.IM = r.IM
}

// Step executes one instruction
func (z *CPU) Step() {
	if z.Halted {
		z.Cycles += 4
		return
	}
	
	opcode := z.fetch()
	z.executeExtended(opcode)
}

func (z *CPU) fetch() byte {
	val := z.Mem.Read(z.PC)
	z.PC++
	z.R = (z.R & 0x80) | ((z.R + 1) & 0x7F) // Update R register
	return val
}

func (z *CPU) push(val uint16) {
	z.SP -= 2
	z.Mem.Write(z.SP, byte(val))
	z.Mem.Write(z.SP+1, byte(val>>8))
}

func (z *CPU) pop() uint16 {
	low := z.Mem.Read(z.SP)
	high := z.Mem.Read(z.SP + 1)
	z.SP += 2
	return uint16(high)<<8 | uint16(low)
}

// IOOut16 performs a 16-bit port OUT operation
func (z *CPU) IOOut16(port uint16, val byte) {
	z.IO.Out16(port, val, z.Cycles)
}

// IOIn16 performs a 16-bit port IN operation
func (z *CPU) IOIn16(port uint16) byte {
	return z.IO.In16(port)
}

// Interrupt triggers an interrupt
func (z *CPU) Interrupt() {
	if !z.IFF1 {
		return
	}
	
	z.Halted = false
	z.IFF1 = false
	z.IFF2 = false
	
	// IM 1: RST 38h
	if z.IM == 1 {
		z.push(z.PC)
		z.PC = 0x0038
		z.Cycles += 13
	}
	// IM 2: Vectored interrupt
	if z.IM == 2 {
		z.push(z.PC)
		// Use I register as high byte, 0xFF as low byte (common for demos)
		vector := uint16(z.I)<<8 | 0xFF
		low := z.Mem.Read(vector)
		high := z.Mem.Read(vector + 1)
		z.PC = uint16(high)<<8 | uint16(low)
		z.Cycles += 19
	}
}
