package c64

// VIC represents the VIC-II chip (6567/6569)
type VIC struct {
	// Registers $D000-$D02E
	Registers [47]byte

	// Internal state
	RasterLine    uint16 // Current raster line (0-312 for PAL, 0-262 for NTSC)
	RasterIRQ     bool   // Raster interrupt flag
	Cycles        uint64 // Total cycles executed

	// Memory access
	Mem *Memory

	// While Recording, StepLine stores the state each raster line starts with
	// in Lines, so CaptureScreen can draw raster effects such as a sprite
	// multiplexer. Off, StepLine costs nothing extra.
	Recording bool
	Lines     []LineState
	TextRows  [25]TextRow

	// Constants for PAL (can be adjusted for NTSC)
	MaxRasterLine uint16 // 312 for PAL, 262 for NTSC
	CyclesPerLine uint64 // 63 for PAL, 65 for NTSC
}

// LineState is what the VIC-II displays on one raster line.
type LineState struct {
	Registers      [47]byte
	SpritePointers [8]byte
	SpriteData     [8][63]byte
}

// TextRow is what the VIC-II fetches for one row of characters at the start of its first line.
type TextRow struct {
	Colors [40]byte
	Glyphs [40][8]byte
}

// VIC-II register offsets
const (
	// Sprite X/Y positions (sprites 0-7)
	VIC_SPR0_X = 0x00
	VIC_SPR0_Y = 0x01
	// ... (sprites 1-7 follow same pattern)

	// Sprite X MSB (bit 8 of X coordinate)
	VIC_SPR_HI_X = 0x10

	// Control register 1
	VIC_CTRL1 = 0x11 // Bit 7: raster bit 8, Bit 5: BMM, Bit 4: DEN, Bit 3: RSEL, Bits 0-2: YSCROLL

	// Raster counter
	VIC_RASTER = 0x12

	// Light pen X/Y
	VIC_LIGHTPEN_X = 0x13
	VIC_LIGHTPEN_Y = 0x14

	// Sprite enable
	VIC_SPR_ENA = 0x15

	// Control register 2
	VIC_CTRL2 = 0x16 // Bit 5: RES, Bit 4: MCM, Bit 3: CSEL, Bits 0-2: XSCROLL

	// Sprite Y expansion
	VIC_SPR_EXP_Y = 0x17

	// Memory pointers
	VIC_MEMPTR = 0x18 // Bits 4-7: Video matrix base, Bits 1-3: Character base

	// Interrupt register
	VIC_IRR = 0x19 // Interrupt request register

	// Interrupt enable
	VIC_IER = 0x1A // Interrupt enable register

	// Sprite data priority
	VIC_SPR_DP = 0x1B

	// Sprite multicolor mode
	VIC_SPR_MC = 0x1C

	// Sprite X expansion
	VIC_SPR_EXP_X = 0x1D

	// Sprite-sprite collision
	VIC_SPR_SPR_COLL = 0x1E

	// Sprite-data collision
	VIC_SPR_BG_COLL = 0x1F

	// Border color
	VIC_BORDER_COL = 0x20

	// Background colors 0-3
	VIC_BG_COL0 = 0x21
	VIC_BG_COL1 = 0x22
	VIC_BG_COL2 = 0x23
	VIC_BG_COL3 = 0x24

	// Sprite multicolor registers
	VIC_SPR_MC0 = 0x25 // Sprite multicolor 0
	VIC_SPR_MC1 = 0x26 // Sprite multicolor 1

	// Sprite colors (sprites 0-7)
	VIC_SPR0_COL = 0x27
	// ... (sprites 1-7 follow at 0x28-0x2E)
)

func NewVIC(mem *Memory) *VIC {
	vic := &VIC{
		Mem:           mem,
		MaxRasterLine: 312, // PAL
		CyclesPerLine: 63,  // PAL
	}

	// Default register values
	vic.Registers[VIC_CTRL1] = 0x1B  // Display enabled, 25 rows
	vic.Registers[VIC_CTRL2] = 0x08  // 40 columns
	vic.Registers[VIC_MEMPTR] = 0x14 // Default memory setup

	return vic
}

// ReadRegister reads a VIC-II register
func (v *VIC) ReadRegister(reg byte) byte {
	switch reg {
	case VIC_CTRL1:
		// Bit 7 is bit 8 of raster counter
		val := v.Registers[VIC_CTRL1] & 0x7F
		if v.RasterLine >= 0x100 {
			val |= 0x80
		}
		return val

	case VIC_RASTER:
		// Lower 8 bits of raster counter
		return byte(v.RasterLine & 0xFF)

	case VIC_IRR:
		// Interrupt request register
		val := v.Registers[VIC_IRR]
		// Bit 7 is set if any enabled interrupt occurred
		if val&v.Registers[VIC_IER]&0x0F != 0 {
			val |= 0x80
		}
		return val

	case VIC_SPR_SPR_COLL, VIC_SPR_BG_COLL:
		// Collision registers are cleared on read
		val := v.Registers[reg]
		v.Registers[reg] = 0
		return val

	default:
		return v.Registers[reg]
	}
}

// WriteRegister writes a VIC-II register
func (v *VIC) WriteRegister(reg byte, val byte) {
	switch reg {
	case VIC_CTRL1:
		// Writing to CTRL1
		v.Registers[VIC_CTRL1] = val & 0x7F
		// Update raster compare bit 8
		rasterCompare := uint16(v.Registers[VIC_RASTER])
		if val&0x80 != 0 {
			rasterCompare |= 0x100
		} else {
			rasterCompare &^= 0x100
		}
		v.Registers[VIC_RASTER] = byte(rasterCompare)

	case VIC_RASTER:
		// Set raster compare value (low 8 bits)
		val8 := (v.Registers[VIC_CTRL1] & 0x80) << 1
		v.Registers[VIC_RASTER] = val
		// Raster compare is stored in RASTER + bit 7 of CTRL1
		_ = val8

	case VIC_IRR:
		// Writing to IRR clears interrupt flags (write 1 to clear)
		v.Registers[VIC_IRR] &^= (val & 0x0F)
		// Clear RasterIRQ flag if raster interrupt bit was cleared
		if val&0x01 != 0 {
			v.RasterIRQ = false
		}

	case VIC_SPR_SPR_COLL, VIC_SPR_BG_COLL:
		// Collision registers are read-only

	default:
		if reg < 47 {
			v.Registers[reg] = val
		}
	}
}

// Step advances VIC-II by one raster line
func (v *VIC) StepLine() {
	v.Cycles += v.CyclesPerLine

	v.RasterLine++
	if v.RasterLine > v.MaxRasterLine {
		v.RasterLine = 0
	}

	if v.Recording {
		v.recordLine()
	}

	// Check for raster interrupt
	rasterCompare := uint16(v.Registers[VIC_RASTER])
	if v.Registers[VIC_CTRL1]&0x80 != 0 {
		rasterCompare |= 0x100
	}

	if v.RasterLine == rasterCompare {
		// Trigger raster interrupt if enabled
		if v.Registers[VIC_IER]&0x01 != 0 {
			v.Registers[VIC_IRR] |= 0x01
			v.RasterIRQ = true
		}
	}
}

func (v *VIC) recordLine() {
	if v.Lines == nil {
		v.Lines = make([]LineState, v.MaxRasterLine+1)
	}
	v.fillLineState(&v.Lines[v.RasterLine])

	firstTextLine := 48 + int(v.Registers[VIC_CTRL1]&7)
	linesIntoText := int(v.RasterLine) - firstTextLine
	if linesIntoText >= 0 && linesIntoText%8 == 0 && linesIntoText/8 < 25 {
		v.fillTextRow(&v.TextRows[linesIntoText/8], linesIntoText/8)
	}
}

// textRowToRender returns the row as the VIC fetched it while recording, else as memory holds it now.
func (v *VIC) textRowToRender(row int) TextRow {
	if v.Recording {
		return v.TextRows[row]
	}

	var textRow TextRow
	v.fillTextRow(&textRow, row)
	return textRow
}

func (v *VIC) fillTextRow(textRow *TextRow, row int) {
	memptr := v.Registers[VIC_MEMPTR]
	videoMatrix := v.VideoMatrixBase(memptr)
	charBase := v.CharacterBase(memptr)

	for col := 0; col < 40; col++ {
		charCode := v.Mem.RAM[videoMatrix+uint16(row*40+col)]
		textRow.Colors[col] = v.Mem.ColorRAM[row*40+col] & 0x0F

		glyphAddr := charBase + uint16(charCode)*8
		if charBase >= 0x1000 && charBase < 0x2000 {
			copy(textRow.Glyphs[col][:], v.Mem.CHARSET[glyphAddr-0x1000:glyphAddr-0x1000+8])
		} else {
			copy(textRow.Glyphs[col][:], v.Mem.RAM[glyphAddr:glyphAddr+8])
		}
	}
}

// fillLineState captures the registers and the sprite data the VIC would fetch for the line,
// since the CPU keeps rewriting that data while the frame is displayed.
func (v *VIC) fillLineState(line *LineState) {
	line.Registers = v.Registers
	pointers := v.GetVideoMatrixBase() + 0x3F8
	copy(line.SpritePointers[:], v.Mem.RAM[pointers:pointers+8])

	bankBase := uint16(3-v.Mem.VICBank) * 0x4000
	for sprite := 0; sprite < 8; sprite++ {
		if v.Registers[VIC_SPR_ENA]&(1<<sprite) != 0 {
			dataAddr := bankBase + uint16(line.SpritePointers[sprite])*64
			copy(line.SpriteData[sprite][:], v.Mem.RAM[dataAddr:dataAddr+63])
		}
	}
}

// GetVideoMatrixBase returns the video matrix base address
func (v *VIC) GetVideoMatrixBase() uint16 {
	return v.VideoMatrixBase(v.Registers[VIC_MEMPTR])
}

// VideoMatrixBase returns the video matrix base address for a VIC_MEMPTR value
func (v *VIC) VideoMatrixBase(memptr byte) uint16 {
	// Bits 4-7 of VIC_MEMPTR, multiplied by 1024
	offset := uint16(memptr>>4) * 1024
	// Plus VIC bank address from CIA2 (bits 0-1, inverted)
	// Bank values: 3=bank0($0000), 2=bank1($4000), 1=bank2($8000), 0=bank3($C000)
	bankBase := uint16(3-v.Mem.VICBank) * 0x4000
	return bankBase + offset
}

// GetCharacterBase returns the character generator base address
func (v *VIC) GetCharacterBase() uint16 {
	return v.CharacterBase(v.Registers[VIC_MEMPTR])
}

// CharacterBase returns the character generator base address for a VIC_MEMPTR value
func (v *VIC) CharacterBase(memptr byte) uint16 {
	// Bits 1-3 of VIC_MEMPTR, multiplied by 2048
	offset := uint16((memptr>>1)&0x07) * 2048
	// Plus VIC bank address
	bankBase := uint16(3-v.Mem.VICBank) * 0x4000
	return bankBase + offset
}

// GetBitmapBase returns the bitmap base address (for bitmap mode)
func (v *VIC) GetBitmapBase() uint16 {
	// Bit 3 of VIC_MEMPTR, multiplied by 8192
	var offset uint16
	if v.Registers[VIC_MEMPTR]&0x08 != 0 {
		offset = 8192
	}
	// Plus VIC bank address
	bankBase := uint16(3-v.Mem.VICBank) * 0x4000
	return bankBase + offset
}

// IsBitmapMode returns true if bitmap mode is enabled
func (v *VIC) IsBitmapMode() bool {
	return (v.Registers[VIC_CTRL1] & 0x20) != 0
}

// IsMulticolorMode returns true if multicolor mode is enabled
func (v *VIC) IsMulticolorMode() bool {
	return (v.Registers[VIC_CTRL2] & 0x10) != 0
}

// IsDisplayEnabled returns true if the display is enabled
func (v *VIC) IsDisplayEnabled() bool {
	return (v.Registers[VIC_CTRL1] & 0x10) != 0
}

// C64 color palette (Pepto)
var C64Palette = [16]struct{ R, G, B byte }{
	{0, 0, 0},       // 0: Black
	{255, 255, 255}, // 1: White
	{129, 51, 56},   // 2: Red
	{117, 206, 200}, // 3: Cyan
	{142, 60, 151},  // 4: Purple
	{86, 172, 77},   // 5: Green
	{46, 44, 155},   // 6: Blue
	{237, 241, 113}, // 7: Yellow
	{142, 80, 41},   // 8: Orange
	{85, 56, 0},     // 9: Brown
	{196, 108, 113}, // A: Light red
	{74, 74, 74},    // B: Dark grey
	{123, 123, 123}, // C: Grey
	{169, 255, 159}, // D: Light green
	{112, 109, 235}, // E: Light blue
	{178, 178, 178}, // F: Light grey
}
