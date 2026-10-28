package main

// cpu6502_irq.go extends CPU6502 with just enough hardware simulation to run
// c64port/src/player_harness_irq.s the way real hardware/VICE does: CIA1
// Timer A/B counting down and firing, a VIC raster IRQ once per frame, and
// real interrupt dispatch through $FFFE. The direct-call harness driven by
// player_6502.go only ever calls player_tick — it never executes
// player_cia1a_fire/player_cia1b_fire, so this is the only path that
// exercises CIA-driven waveform alternation (used by every envelope-mode
// note and every mixed-state NPT drum hit).
//
// The real KERNAL ROM is copyrighted, so instead of embedding it this
// installs a small equivalent stub at the two addresses player_harness_irq.s
// hardcodes:
//   $FF48 (the $FFFE hardware vector target) — saves A/X/Y, indirect-jumps
//     through $0314/$0315 (the same vector the KERNAL exposes for user IRQs).
//   $EA81 (JMP target at the end of irq_dispatch) — restores A/X/Y and RTIs.
// This reproduces the real chain's observable behavior without copying ROM.

const (
	stubIRQEntry = 0xFF48 // matches the real $FFFE vector target
	stubIRQExit  = 0xEA81 // matches player_harness_irq.s's KERNAL_IRQ_EXIT
)

// installKernalIRQStub writes tiny hand-assembled routines at stubIRQEntry
// and stubIRQExit, and points the hardware IRQ vector ($FFFE/$FFFF) at
// stubIRQEntry, matching the real KERNAL's layout closely enough for
// player_harness_irq.s (which hardcodes both addresses) to run unmodified.
func installKernalIRQStub(c *CPU6502) {
	// $FF48: PHA; TXA; PHA; TYA; PHA; JMP ($0314)
	entry := []byte{
		0x48,       // PHA
		0x8A, 0x48, // TXA; PHA
		0x98, 0x48, // TYA; PHA
		0x6C, 0x14, 0x03, // JMP ($0314)
	}
	copy(c.Memory[stubIRQEntry:], entry)

	// $EA81: PLA; TAY; PLA; TAX; PLA; RTI
	exit := []byte{
		0x68,       // PLA
		0xA8,       // TAY
		0x68,       // PLA
		0xAA,       // TAX
		0x68,       // PLA
		0x40,       // RTI
	}
	copy(c.Memory[stubIRQExit:], exit)

	entryAddr := uint16(stubIRQEntry)
	c.Memory[0xFFFE] = byte(entryAddr)
	c.Memory[0xFFFF] = byte(entryAddr >> 8)
}

// CIA1 and VIC register addresses (must match c64port/src/player_harness_irq.s
// and c64port/src/player.s).
const (
	irqVicRaster = 0xD012
	irqVicCtrl1  = 0xD011
	irqVicIMR    = 0xD01A
	irqVicIRR    = 0xD019
	irqCIA1PRB   = 0xDC01
	irqCIA1TALo  = 0xDC04
	irqCIA1TAHi  = 0xDC05
	irqCIA1TBLo  = 0xDC06
	irqCIA1TBHi  = 0xDC07
	irqCIA1ICR   = 0xDC0D
	irqCIA1CRA   = 0xDC0E
	irqCIA1CRB   = 0xDC0F
)

// IRQMachine drives a CPU6502 with a CIA1 Timer A/B countdown model and a
// once-per-frame VIC raster IRQ, dispatching through the stub IRQ entry at
// $FFFE. This exercises the CIA1 timer IRQ handlers
// (player_cia1a_fire/player_cia1b_fire) and the raster-driven player_tick
// call under real interrupt semantics (I-flag masking, nested interrupts),
// with the CPU's instruction cycle counts driving the timers.
type IRQMachine struct {
	CPU *CPU6502

	ciaTAValue, ciaLatchA uint16
	ciaTBValue, ciaLatchB uint16
	ciaTARun, ciaTBRun    bool
	ciaICRMask            byte // bits enabled via ICR SET-bit ($8x) writes
	ciaICRFlags           byte // latched interrupt flags (bit0=TA, bit1=TB)

	// PB6/PB7 toggle output (CRA/CRB bit1=PBON, bit2=OUTMODE=toggle): when
	// both set, the pin flips on every underflow instead of pulsing, and the
	// current level reads back from PRB ($DC01) bit6/bit7. player.s uses
	// this instead of tracking phase in software.
	ciaTAOutToggle, ciaTBOutToggle bool // PBON && OUTMODE for each timer
	ciaTALevel, ciaTBLevel         bool // current toggle level

	vicIMR      byte
	vicIRRFlags byte

	cyclesPerFrame int
	nextRaster     int64
	frameEnd       int64
	interruptDepth int

	// Diagnostics: interrupts as the handlers observe them per RunFrame call —
	// CIA fires counted when ICR is read, raster fires when VIC_IRR is acked.
	// Cleared at the start of each RunFrame.
	RasterFires int
	CIAAFires   int
	CIABFires   int
}

// NewIRQMachine wraps an already-initialized CPU6502 (program + stub KERNAL
// IRQ entry/exit loaded) and installs I/O intercepts for the CIA1/VIC
// registers player_harness_irq.s and player.s touch.
func NewIRQMachine(c *CPU6502, cyclesPerFrame int) *IRQMachine {
	m := &IRQMachine{CPU: c, cyclesPerFrame: cyclesPerFrame}
	c.IORead = m.ioRead
	c.IOWrite = m.ioWrite
	return m
}

func (m *IRQMachine) ioRead(addr uint16) (byte, bool) {
	switch addr {
	case irqCIA1ICR:
		// Reading ICR returns pending flags (bit7 set if any) then clears them.
		flags := m.ciaICRFlags
		m.ciaICRFlags = 0
		if flags&0x01 != 0 {
			m.CIAAFires++
		}
		if flags&0x02 != 0 {
			m.CIABFires++
		}
		if flags != 0 {
			flags |= 0x80
		}
		return flags, true
	case irqCIA1PRB:
		var val byte
		if m.ciaTAOutToggle && m.ciaTALevel {
			val |= 0x40
		}
		if m.ciaTBOutToggle && m.ciaTBLevel {
			val |= 0x80
		}
		return val, true
	case irqVicIRR:
		return m.vicIRRFlags, true
	}
	return 0, false
}

func (m *IRQMachine) ioWrite(addr uint16, val byte) bool {
	switch addr {
	case irqCIA1TALo:
		m.ciaLatchA = (m.ciaLatchA &^ 0x00FF) | uint16(val)
		m.ciaTAValue = m.ciaLatchA
		return true
	case irqCIA1TAHi:
		m.ciaLatchA = (m.ciaLatchA &^ 0xFF00) | uint16(val)<<8
		m.ciaTAValue = m.ciaLatchA
		return true
	case irqCIA1TBLo:
		m.ciaLatchB = (m.ciaLatchB &^ 0x00FF) | uint16(val)
		m.ciaTBValue = m.ciaLatchB
		return true
	case irqCIA1TBHi:
		m.ciaLatchB = (m.ciaLatchB &^ 0xFF00) | uint16(val)<<8
		m.ciaTBValue = m.ciaLatchB
		return true
	case irqCIA1ICR:
		if val&0x80 != 0 {
			m.ciaICRMask |= val &^ 0x80
		} else {
			m.ciaICRMask &^= val
		}
		return true
	case irqCIA1CRA:
		m.ciaTARun = val&0x01 != 0
		m.ciaTAOutToggle = val&0x06 == 0x06 // PBON (bit1) && OUTMODE=toggle (bit2)
		if val&0x10 != 0 {                  // FORCE LOAD bit
			m.ciaTAValue = m.ciaLatchA
		}
		return true
	case irqCIA1CRB:
		m.ciaTBRun = val&0x01 != 0
		m.ciaTBOutToggle = val&0x06 == 0x06
		if val&0x10 != 0 {
			m.ciaTBValue = m.ciaLatchB
		}
		return true
	case irqVicRaster, irqVicCtrl1:
		return true // accepted, not modeled (raster fires once per RunFrame instead)
	case irqVicIMR:
		m.vicIMR = val
		return true
	case irqVicIRR:
		if val&m.vicIRRFlags&0x01 != 0 {
			m.RasterFires++
		}
		m.vicIRRFlags &^= val
		return true
	}
	return false
}

// tickCycle advances CIA1 timers by one cycle, latching interrupt flags on
// underflow and reloading (player.s always arms with $11 = LOAD+START,
// continuous mode).
func (m *IRQMachine) tickCycle() {
	if m.ciaTARun {
		if m.ciaTAValue == 0 {
			m.ciaICRFlags |= 0x01
			m.ciaTAValue = m.ciaLatchA
			m.ciaTALevel = !m.ciaTALevel
		} else {
			m.ciaTAValue--
		}
	}
	if m.ciaTBRun {
		if m.ciaTBValue == 0 {
			m.ciaICRFlags |= 0x02
			m.ciaTBValue = m.ciaLatchB
			m.ciaTBLevel = !m.ciaTBLevel
		} else {
			m.ciaTBValue--
		}
	}
}

func (m *IRQMachine) irqPending() bool {
	if m.ciaICRFlags&m.ciaICRMask != 0 {
		return true
	}
	if m.vicIRRFlags&m.vicIMR != 0 {
		return true
	}
	return false
}

func (m *IRQMachine) tickCycles(cycles int64) {
	for i := int64(0); i < cycles; i++ {
		m.tickCycle()
	}
}

// Begin starts the song timeline: the cycles init spent so far become
// negative SID write stamps, and the first raster IRQ is due immediately.
func (m *IRQMachine) Begin() {
	c := m.CPU
	for i := range c.SIDWrites {
		c.SIDWrites[i].Cycle -= c.Cycles
	}
	c.CycleBase = c.Cycles
	m.nextRaster = c.Cycles
	m.frameEnd = c.Cycles
}

func (m *IRQMachine) pollRaster() {
	for m.CPU.Cycles >= m.nextRaster {
		m.vicIRRFlags |= 0x01
		m.nextRaster += int64(m.cyclesPerFrame)
	}
}

// RunFrame advances the machine by one frame of cycles. The CPU executes
// instruction by instruction while the CIA timers count each cycle, and an
// enabled interrupt is taken between instructions whenever the I flag is
// clear — so a CIA underflow can preempt the raster handler once it has
// executed cli. When no handler is running the CPU idles one cycle at a
// time. The VIC raster IRQ is raised at the first cycle of every frame; a
// handler still running at the frame boundary is run to completion.
func (m *IRQMachine) RunFrame() {
	m.RasterFires, m.CIAAFires, m.CIABFires = 0, 0, 0
	m.frameEnd += int64(m.cyclesPerFrame)
	for m.CPU.Cycles < m.frameEnd || m.interruptDepth > 0 {
		m.runOne()
	}
}

func (m *IRQMachine) runOne() {
	c := m.CPU
	m.pollRaster()
	if !c.getFlag(cpu6502FlagI) && m.irqPending() {
		m.enterInterrupt()
		return
	}
	if m.interruptDepth == 0 {
		c.Cycles++
		m.tickCycle()
		return
	}
	op := c.Memory[c.PC]
	before := c.Cycles
	preTicked := int64(cpu6502Cycles[op]) - 1
	m.tickCycles(preTicked)
	c.step()
	m.tickCycles(c.Cycles - before - preTicked)
	if op == 0x40 {
		m.interruptDepth--
	}
}

// enterInterrupt pushes PC/P, sets the I flag, and jumps through $FFFE,
// charging the 7-cycle interrupt entry.
func (m *IRQMachine) enterInterrupt() {
	c := m.CPU
	c.cpuPush16(c.PC)
	c.cpuPush(c.P&^cpu6502FlagB | cpu6502FlagU)
	c.P |= cpu6502FlagI
	c.PC = c.cpuRead16(0xFFFE)
	c.Cycles += 7
	m.tickCycles(7)
	m.interruptDepth++
}
