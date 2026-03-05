package spectrum

// IO handles ZX Spectrum I/O ports
type IO struct {
	Border byte
	AY     *AY38912
	
	// Memory paging
	Mem *Memory
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

func (io *IO) Out(port byte, val byte) {
	// 8-bit port OUT (n), A - use A register as high byte
	io.Out16(uint16(port), val)
}

func (io *IO) Out16(port uint16, val byte) {
	switch {
	case (port & 0xFF) == 0xFE:
		// ULA - border, beeper (port 0xFE, any high byte)
		io.Border = val & 0x07

	case port == 0x7FFD:
		// 0x7FFD - Memory paging (128K)
		// Bits 0-2: RAM page at 0xC000
		// Bit 3: Screen select (0=bank 5, 1=bank 7)
		// Bit 4: ROM select
		// Bit 5: Paging lock
		io.Mem.SetPaging(val)

	case port == 0xFFFD:
		// 0xFFFD - AY register select
		io.AY.SelectRegister(val)

	case port == 0xBFFD:
		// 0xBFFD - AY register write
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
