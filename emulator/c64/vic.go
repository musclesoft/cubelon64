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

	// Constants for PAL (can be adjusted for NTSC)
	MaxRasterLine uint16 // 312 for PAL, 262 for NTSC
	CyclesPerLine uint64 // 63 for PAL, 65 for NTSC
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

// GetVideoMatrixBase returns the video matrix base address
func (v *VIC) GetVideoMatrixBase() uint16 {
	// Bits 4-7 of VIC_MEMPTR, multiplied by 1024
	// Plus VIC bank address from CIA2
	return uint16(v.Registers[VIC_MEMPTR]>>4) * 1024
}

// GetCharacterBase returns the character generator base address
func (v *VIC) GetCharacterBase() uint16 {
	// Bits 1-3 of VIC_MEMPTR, multiplied by 2048
	return uint16((v.Registers[VIC_MEMPTR]>>1)&0x07) * 2048
}

// GetBitmapBase returns the bitmap base address (for bitmap mode)
func (v *VIC) GetBitmapBase() uint16 {
	// Bit 3 of VIC_MEMPTR, multiplied by 8192
	if v.Registers[VIC_MEMPTR]&0x08 != 0 {
		return 8192
	}
	return 0
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

// GetSpriteX returns the X coordinate of a sprite (0-7)
func (v *VIC) GetSpriteX(sprite byte) uint16 {
	if sprite > 7 {
		return 0
	}
	x := uint16(v.Registers[VIC_SPR0_X+sprite*2])
	// Check MSB bit
	if v.Registers[VIC_SPR_HI_X]&(1<<sprite) != 0 {
		x |= 0x100
	}
	return x
}

// GetSpriteY returns the Y coordinate of a sprite (0-7)
func (v *VIC) GetSpriteY(sprite byte) byte {
	if sprite > 7 {
		return 0
	}
	return v.Registers[VIC_SPR0_Y+sprite*2]
}

// IsSpriteEnabled returns true if a sprite is enabled
func (v *VIC) IsSpriteEnabled(sprite byte) bool {
	if sprite > 7 {
		return false
	}
	return (v.Registers[VIC_SPR_ENA] & (1 << sprite)) != 0
}

// C64 color palette
var C64Palette = [16]struct{ R, G, B byte }{
	{0, 0, 0},       // 0: Black
	{255, 255, 255}, // 1: White
	{104, 55, 43},   // 2: Red
	{112, 164, 178}, // 3: Cyan
	{111, 61, 134},  // 4: Purple
	{88, 141, 67},   // 5: Green
	{53, 40, 121},   // 6: Blue
	{184, 199, 111}, // 7: Yellow
	{111, 79, 37},   // 8: Orange
	{67, 57, 0},     // 9: Brown
	{154, 103, 89},  // A: Light red
	{68, 68, 68},    // B: Dark grey
	{108, 108, 108}, // C: Grey
	{154, 210, 132}, // D: Light green
	{108, 94, 181},  // E: Light blue
	{149, 149, 149}, // F: Light grey
}
