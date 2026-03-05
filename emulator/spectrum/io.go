package spectrum

// IO handles ZX Spectrum I/O ports
type IO struct {
	Border      byte
	BeeperState byte
	AY          *AY38912

	// Memory paging
	Mem *Memory

	// Audio buffer for cycle-accurate updates
	Audio *AudioBuffer

	// Stats tracking
	AYSelectCount int
	AYWriteCount  int
	ULAWriteCount int
}

func NewIO(mem *Memory) *IO {
	return &IO{
		Mem: mem,
		AY: NewAY38912(),
	}
}

func (io *IO) In(port byte) byte {
	// Keyboard, AY, etc.
	return 0xFF
}

func (io *IO) In16(port uint16) byte {
	// 16-bit port IN - keyboard, Kempston joystick, AY, etc.
	// For now, return 0xFF (all bits high) for all ports
	return 0xFF
}

func (io *IO) Out(port byte, val byte, cycles uint64) {
	// 8-bit port OUT (n), A - full port address is (A << 8) | n
	// where A is both the value AND the high byte of the port
	fullPort := (uint16(val) << 8) | uint16(port)
	io.Out16(fullPort, val, cycles)
}

func (io *IO) Out16(port uint16, val byte, cycles uint64) {
	switch {
	case (port & 0xFF) == 0xFE:
		// ULA - border, beeper (port 0xFE, any high byte)
		io.ULAWriteCount++
		io.Border = val & 0x07
		io.BeeperState = (val >> 4) & 0x01

	case (port & 0x8002) == 0:
		// 0x7FFD - Memory paging (128K)
		// Bits 0-2: RAM page at 0xC000
		// Bit 3: Screen select (0=bank 5, 1=bank 7)
		// Bit 4: ROM select
		// Bit 5: Paging lock
		io.Mem.SetPaging(val)

	case (port & 0xc002) == 0xc000:
		// AY register select (matches 0xFFFD and similar)
		// Uses partial address decoding like real hardware
		io.AYSelectCount++
		io.AY.SelectRegister(val)

	case (port & 0xc002) == 0x8000:
		// AY register write (matches 0xBFFD and similar)
		// Uses partial address decoding like real hardware
		// IMPORTANT: Update audio BEFORE writing register (matches JSSpeccy timing)
		if io.Audio != nil {
			io.Audio.Update(io.AY, io.BeeperState, cycles)
		}
		io.AYWriteCount++
		io.AY.WriteRegister(val)
	}
}

// AY38912 represents the AY-3-8912 sound chip
type AY38912 struct {
	Registers [16]byte
	Selected  byte
}

func NewAY38912() *AY38912 {
	return &AY38912{}
}

func (ay *AY38912) SelectRegister(reg byte) {
	if reg < 16 {
		ay.Selected = reg
	}
}

func (ay *AY38912) WriteRegister(val byte) {
	if ay.Selected < 16 {
		ay.Registers[ay.Selected] = val
	}
}

// GetTone returns frequency for a channel (0-2)
func (ay *AY38912) GetTone(ch int) uint16 {
	if ch > 2 {
		return 0
	}
	reg := ch * 2
	return uint16(ay.Registers[reg]) | (uint16(ay.Registers[reg+1]&0x0F) << 8)
}

// GetVolume returns volume for a channel (0-2)
func (ay *AY38912) GetVolume(ch int) byte {
	if ch > 2 {
		return 0
	}
	return ay.Registers[8+ch] & 0x0F
}
