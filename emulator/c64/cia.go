package c64

const (
	ciaPortB     = 0x01
	ciaTimerALo  = 0x04
	ciaTimerAHi  = 0x05
	ciaTimerBLo  = 0x06
	ciaTimerBHi  = 0x07
	ciaInterrupt = 0x0D
	ciaControlA  = 0x0E
	ciaControlB  = 0x0F

	ciaControlStart     = 0x01
	ciaControlPortOn    = 0x02
	ciaControlToggle    = 0x04
	ciaControlForceLoad = 0x10
)

// CIA1Timer is one of the CIA 1 down counters. It counts one per cycle, and the cycle after it
// reads zero it underflows: it reloads from its latch, raises its interrupt flag and, with
// PBON and toggle output set, flips its port B pin.
type CIA1Timer struct {
	Latch   uint16
	Value   uint16
	Running bool
	Control byte
	Level   bool

	// OnUnderflow is called with the cycle of each underflow and the pin level after it.
	OnUnderflow func(cycle uint64, level bool)

	flagBit byte
	cia     *CIA1
}

// CIA1 models the timers of CIA 1 and their interrupts. Registers are read and written at the
// cycle given to AdvanceTo.
type CIA1 struct {
	TimerA CIA1Timer
	TimerB CIA1Timer

	now   uint64
	mask  byte
	flags byte
}

func NewCIA1() *CIA1 {
	c := &CIA1{}
	c.TimerA.flagBit = 0x01
	c.TimerA.cia = c
	c.TimerB.flagBit = 0x02
	c.TimerB.cia = c
	return c
}

// AdvanceTo runs the timers up to the given cycle. Going backwards does nothing.
func (c *CIA1) AdvanceTo(cycle uint64) {
	if cycle <= c.now {
		return
	}
	c.TimerA.advance(c.now, cycle)
	c.TimerB.advance(c.now, cycle)
	c.now = cycle
}

// IRQ reports whether an enabled interrupt flag is pending.
func (c *CIA1) IRQ() bool {
	return c.flags&c.mask != 0
}

func (c *CIA1) Read(register byte) byte {
	switch register {
	case ciaPortB:
		value := byte(0x3F)
		if c.TimerA.pinHigh() {
			value |= 0x40
		}
		if c.TimerB.pinHigh() {
			value |= 0x80
		}
		return value
	case ciaTimerALo:
		return byte(c.TimerA.Value)
	case ciaTimerAHi:
		return byte(c.TimerA.Value >> 8)
	case ciaTimerBLo:
		return byte(c.TimerB.Value)
	case ciaTimerBHi:
		return byte(c.TimerB.Value >> 8)
	case ciaInterrupt:
		flags := c.flags
		c.flags = 0
		if flags&c.mask != 0 {
			flags |= 0x80
		}
		return flags
	case ciaControlA:
		return c.TimerA.Control
	case ciaControlB:
		return c.TimerB.Control
	}
	return 0xFF
}

func (c *CIA1) Write(register byte, value byte) {
	switch register {
	case ciaTimerALo:
		c.TimerA.Latch = c.TimerA.Latch&0xFF00 | uint16(value)
	case ciaTimerAHi:
		c.TimerA.writeLatchHigh(value)
	case ciaTimerBLo:
		c.TimerB.Latch = c.TimerB.Latch&0xFF00 | uint16(value)
	case ciaTimerBHi:
		c.TimerB.writeLatchHigh(value)
	case ciaInterrupt:
		if value&0x80 != 0 {
			c.mask |= value &^ 0x80
		} else {
			c.mask &^= value
		}
	case ciaControlA:
		c.TimerA.writeControl(value)
	case ciaControlB:
		c.TimerB.writeControl(value)
	}
}

func (t *CIA1Timer) pinHigh() bool {
	return t.Control&(ciaControlPortOn|ciaControlToggle) == ciaControlPortOn|ciaControlToggle && t.Level
}

func (t *CIA1Timer) writeLatchHigh(value byte) {
	t.Latch = t.Latch&0x00FF | uint16(value)<<8
	if !t.Running {
		t.Value = t.Latch
	}
}

func (t *CIA1Timer) writeControl(value byte) {
	t.Control = value &^ ciaControlForceLoad
	start := value&ciaControlStart != 0
	if start && !t.Running {
		t.Level = true
	}
	t.Running = start
	if value&ciaControlForceLoad != 0 {
		t.Value = t.Latch
	}
}

func (t *CIA1Timer) advance(from, to uint64) {
	if !t.Running {
		return
	}
	cycle := from
	for {
		underflowCycle := cycle + uint64(t.Value) + 1
		if underflowCycle > to {
			t.Value -= uint16(to - cycle)
			return
		}
		cycle = underflowCycle
		t.Value = t.Latch
		t.Level = !t.Level
		t.cia.flags |= t.flagBit
		if t.OnUnderflow != nil {
			t.OnUnderflow(cycle, t.Level)
		}
	}
}
