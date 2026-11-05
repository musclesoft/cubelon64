package main

// cpu6502_irq.go extends CPU6502 with just enough hardware simulation to run
// c64port/src/player_harness_irq.s the way real hardware/VICE does: CIA1
// Timer A/B counting down and firing, a VIC raster IRQ once per frame, and
// real interrupt dispatch through $FFFE. The direct-call harness driven by
// player_6502.go only ever calls music_player_sid_update+step — it never executes
// music_player_write_ctrl, so this is the only path that
// exercises CIA-driven waveform alternation (used by every envelope-mode
// note and every mixed-state NPT drum hit).

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
// once-per-frame VIC raster IRQ, dispatching through the IRQ vector at
// $FFFE. This exercises the CIA1 timer IRQ handlers
// (music_player_write_ctrl) and the raster-driven music_player_sid_update+step
// call under real interrupt semantics (I-flag masking, nested interrupts),
// with the CPU's instruction cycle counts driving the timers.
type IRQMachine struct {
	CPU *CPU6502

	ciaTAValue, ciaLatchA uint16
	ciaTBValue, ciaLatchB uint16
	ciaTARun, ciaTBRun    bool
	ciaCRA, ciaCRB        byte // control registers as read back (force-load strobe reads 0)
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

	// HandlerCycles counts the cycles spent inside interrupt handlers (entry,
	// body and RTI of every raster and CIA fire) during the last RunFrame call.
	HandlerCycles int64

	// Breakpoints maps an address to a callback invoked with the interrupt
	// nesting depth just before an interrupt handler executes the instruction
	// at that address. Read CPU.Cycles in it to time code spans.
	Breakpoints map[uint16]func(interruptDepth int)
}

// NewIRQMachine wraps an already-initialized CPU6502 (program loaded) and installs I/O intercepts for the CIA1/VIC
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
	case irqCIA1TALo:
		return byte(m.ciaTAValue), true
	case irqCIA1TAHi:
		return byte(m.ciaTAValue >> 8), true
	case irqCIA1TBLo:
		return byte(m.ciaTBValue), true
	case irqCIA1TBHi:
		return byte(m.ciaTBValue >> 8), true
	case irqCIA1CRA:
		return m.ciaCRA, true
	case irqCIA1CRB:
		return m.ciaCRB, true
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
		return true
	case irqCIA1TAHi:
		m.ciaLatchA = (m.ciaLatchA &^ 0xFF00) | uint16(val)<<8
		if !m.ciaTARun {
			m.ciaTAValue = m.ciaLatchA
		}
		return true
	case irqCIA1TBLo:
		m.ciaLatchB = (m.ciaLatchB &^ 0x00FF) | uint16(val)
		return true
	case irqCIA1TBHi:
		m.ciaLatchB = (m.ciaLatchB &^ 0xFF00) | uint16(val)<<8
		if !m.ciaTBRun {
			m.ciaTBValue = m.ciaLatchB
		}
		return true
	case irqCIA1ICR:
		if val&0x80 != 0 {
			m.ciaICRMask |= val &^ 0x80
		} else {
			m.ciaICRMask &^= val
		}
		return true
	case irqCIA1CRA:
		m.ciaCRA = val &^ 0x10
		if val&0x01 != 0 && !m.ciaTARun {
			m.ciaTALevel = true
		}
		m.ciaTARun = val&0x01 != 0
		m.ciaTAOutToggle = val&0x06 == 0x06 // PBON (bit1) && OUTMODE=toggle (bit2)
		if val&0x10 != 0 {                  // FORCE LOAD bit
			m.ciaTAValue = m.ciaLatchA
		}
		return true
	case irqCIA1CRB:
		m.ciaCRB = val &^ 0x10
		if val&0x01 != 0 && !m.ciaTBRun {
			m.ciaTBLevel = true
		}
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
	m.HandlerCycles = 0
	m.frameEnd += int64(m.cyclesPerFrame)
	for m.CPU.Cycles < m.frameEnd || m.interruptDepth > 0 {
		before, depthBefore := m.CPU.Cycles, m.interruptDepth
		m.runOne()
		if depthBefore > 0 || m.interruptDepth > 0 {
			m.HandlerCycles += m.CPU.Cycles - before
		}
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
	if onBreakpoint, ok := m.Breakpoints[c.PC]; ok {
		onBreakpoint(m.interruptDepth)
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
