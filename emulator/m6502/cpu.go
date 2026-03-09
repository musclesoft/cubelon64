package m6502

// CPU represents a MOS 6502 processor
type CPU struct {
	// 8-bit registers
	A byte // Accumulator
	X byte // Index register X
	Y byte // Index register Y

	// Status register (NV-BDIZC)
	// Bit 7: N (Negative)
	// Bit 6: V (Overflow)
	// Bit 5: - (unused, always 1)
	// Bit 4: B (Break)
	// Bit 3: D (Decimal mode)
	// Bit 2: I (Interrupt disable)
	// Bit 1: Z (Zero)
	// Bit 0: C (Carry)
	P byte

	// Stack pointer (points to 0x0100-0x01FF)
	SP byte

	// Program counter
	PC uint16

	// Memory interface
	Mem Memory

	// Cycle counter
	Cycles uint64
}

type Memory interface {
	Read(addr uint16) byte
	Write(addr uint16, val byte)
}

// Status register flags
const (
	FlagC = 1 << 0 // Carry
	FlagZ = 1 << 1 // Zero
	FlagI = 1 << 2 // Interrupt Disable
	FlagD = 1 << 3 // Decimal Mode
	FlagB = 1 << 4 // Break
	FlagU = 1 << 5 // Unused (always 1)
	FlagV = 1 << 6 // Overflow
	FlagN = 1 << 7 // Negative
)

// Interrupt vectors
const (
	NMIVector   = 0xFFFA
	ResetVector = 0xFFFC
	IRQVector   = 0xFFFE
)

func New(mem Memory) *CPU {
	return &CPU{
		Mem: mem,
		P:   FlagU | FlagI, // Unused bit always set, interrupts disabled
		SP:  0xFF,
	}
}

// Reset initializes the CPU to power-on state
func (c *CPU) Reset() {
	c.PC = c.read16(ResetVector)
	c.SP = 0xFD // After reset, SP is typically 0xFD
	c.P = FlagU | FlagI
	c.Cycles = 7 // Reset takes 7 cycles
}

// Step executes one instruction
func (c *CPU) Step() {
	opcode := c.fetch()
	c.execute(opcode)
}

func (c *CPU) fetch() byte {
	val := c.Mem.Read(c.PC)
	c.PC++
	return val
}

// Stack operations
func (c *CPU) push(val byte) {
	c.Mem.Write(0x0100|uint16(c.SP), val)
	c.SP--
}

func (c *CPU) push16(val uint16) {
	c.push(byte(val >> 8))   // High byte first
	c.push(byte(val & 0xFF)) // Low byte second
}

func (c *CPU) pop() byte {
	c.SP++
	return c.Mem.Read(0x0100 | uint16(c.SP))
}

func (c *CPU) pop16() uint16 {
	lo := c.pop()
	hi := c.pop()
	return uint16(hi)<<8 | uint16(lo)
}

// Memory access helpers
func (c *CPU) read16(addr uint16) uint16 {
	lo := c.Mem.Read(addr)
	hi := c.Mem.Read(addr + 1)
	return uint16(hi)<<8 | uint16(lo)
}

// Flag operations
func (c *CPU) setFlag(flag byte, condition bool) {
	if condition {
		c.P |= flag
	} else {
		c.P &^= flag
	}
}

func (c *CPU) getFlag(flag byte) bool {
	return (c.P & flag) != 0
}

func (c *CPU) updateZN(val byte) {
	c.setFlag(FlagZ, val == 0)
	c.setFlag(FlagN, (val&0x80) != 0)
}

// NMI triggers a non-maskable interrupt
func (c *CPU) NMI() {
	c.push16(c.PC)
	c.push(c.P &^ FlagB) // Push status without Break flag
	c.setFlag(FlagI, true)
	c.PC = c.read16(NMIVector)
	c.Cycles += 7
}

// IRQ triggers a maskable interrupt
func (c *CPU) IRQ() {
	if c.getFlag(FlagI) {
		return // IRQ disabled
	}
	c.push16(c.PC)
	c.push(c.P &^ FlagB) // Push status without Break flag
	c.setFlag(FlagI, true)
	c.PC = c.read16(IRQVector)
	c.Cycles += 7
}
