package c64

// Cycle-level VIC-II (6569, PAL) behavior that raster effects depend on: sprite DMA with its
// MCBASE counter and Y expansion flip-flop, and the cycles the VIC takes from the CPU for
// bad lines and sprite DMA. The CPU is run instruction by instruction; Retire gives every
// cycle of the instruction to the VIC first, so register writes land on the cycle the CPU
// performs them and the CPU waits for the cycles the VIC owns.
//
// Cycle numbers are the VIC's (1-63 per line). Events happen in the first clock phase of
// their cycle, so a register write in cycle N is seen by events from cycle N+1 on.

const (
	cyclesPerLine = 63

	firstBadLine = 0x30
	lastBadLine  = 0xF7

	badLineBusLowCycle    = 12
	badLineFirstReadStop  = 12
	badLineFirstWriteHalt = 15
	badLineLastCycle      = 54

	mcBaseFirstCycle    = 15
	mcBaseSecondCycle   = 16
	flipFlopCycle       = 55
	yMatchFirstCycle    = 55
	yMatchSecondCycle   = 56
	displayCheckCycle   = 58
	spriteRecordCycle   = 12
	spriteMCBaseLimit   = 63
	spriteBusLowLead    = 3
	spriteFirstDMACycle = 58
	spriteLinesCycles   = 2
	spriteDataBytes     = 3
)

type registerWrite struct{ reg, val byte }

type spriteState struct {
	dma, display, expandFlipFlop bool
	mcBase, mc                   int
	data                         [spriteDataBytes]byte
}

// spritePointerCycle is the cycle in which sprite n's pointer is read: sprites 0-2 at the
// end of a line, sprites 3-7 at the start of the next one.
var spritePointerCycle = [8]int{58, 60, 62, 1, 3, 5, 7, 9}

// Retire runs the cycles of one executed instruction through the VIC and returns the cycle
// after it ended, later than start+cycles when the VIC held the CPU. The instruction itself
// has already executed in the CPU model; opcode tells which of its cycles write.
func (v *VIC) Retire(start uint64, cycles int, opcode byte) uint64 {
	t := start
	for i := 0; i < cycles; i++ {
		write := isWriteCycle(opcode, i, cycles)
		for {
			v.advanceThrough(t)
			if !v.cpuHeld(write) {
				break
			}
			t++
		}
		if i == cycles-1 {
			v.commitWrites()
		}
		t++
	}
	return t
}

func (v *VIC) advanceThrough(t uint64) {
	for v.nextCycle <= t {
		v.stepCycle()
		v.nextCycle++
	}
}

func (v *VIC) commitWrites() {
	for _, write := range v.pending {
		v.Timed[write.reg] = write.val
		if write.reg == VIC_SPR_EXP_Y {
			for n := range v.sprites {
				if write.val&(1<<n) == 0 {
					v.sprites[n].expandFlipFlop = true
				}
			}
		}
	}
	v.pending = v.pending[:0]
}

func (v *VIC) stepCycle() {
	v.cycleInLine++
	if v.cycleInLine > cyclesPerLine {
		v.cycleInLine = 1
		v.RasterLine++
		if v.RasterLine > v.MaxRasterLine {
			v.RasterLine = 0
		}
	}
	cycle := v.cycleInLine

	switch cycle {
	case 1:
		v.badLine = false
		v.compareRaster()
	case badLineBusLowCycle:
		v.badLine = v.isBadLine()
	case mcBaseFirstCycle:
		v.stepMCBase(2)
	case mcBaseSecondCycle:
		v.stepMCBase(1)
	case flipFlopCycle:
		v.toggleExpandFlipFlops()
	}
	switch cycle {
	case yMatchFirstCycle, yMatchSecondCycle:
		v.startSpriteDMA()
	case displayCheckCycle:
		v.checkSpriteDisplay()
	}
	for n, pointerCycle := range spritePointerCycle {
		if cycle == pointerCycle {
			v.fetchSprite(n)
		}
	}
	switch cycle {
	case spriteRecordCycle:
		v.recordSprites()
	case mcBaseFirstCycle:
		v.recordTextRow()
	}
}

func (v *VIC) compareRaster() {
	rasterCompare := uint16(v.Registers[VIC_RASTER])
	if v.Registers[VIC_CTRL1]&0x80 != 0 {
		rasterCompare |= 0x100
	}
	if v.RasterLine == rasterCompare && v.Registers[VIC_IER]&0x01 != 0 {
		v.Registers[VIC_IRR] |= 0x01
		v.RasterIRQ = true
	}
}

func (v *VIC) isBadLine() bool {
	control := v.Timed[VIC_CTRL1]
	return control&0x10 != 0 &&
		v.RasterLine >= firstBadLine && v.RasterLine <= lastBadLine &&
		byte(v.RasterLine&7) == control&7
}

func (v *VIC) stepMCBase(increment int) {
	for n := range v.sprites {
		sprite := &v.sprites[n]
		if sprite.expandFlipFlop {
			sprite.mcBase += increment
		}
		if increment == 1 && sprite.mcBase == spriteMCBaseLimit {
			sprite.dma = false
		}
	}
}

func (v *VIC) toggleExpandFlipFlops() {
	for n := range v.sprites {
		if v.Timed[VIC_SPR_EXP_Y]&(1<<n) != 0 {
			v.sprites[n].expandFlipFlop = !v.sprites[n].expandFlipFlop
		}
	}
}

func (v *VIC) startSpriteDMA() {
	for n := range v.sprites {
		sprite := &v.sprites[n]
		if sprite.dma || v.Timed[VIC_SPR_ENA]&(1<<n) == 0 || !v.spriteYMatches(n) {
			continue
		}
		sprite.dma = true
		sprite.mcBase = 0
		if v.Timed[VIC_SPR_EXP_Y]&(1<<n) != 0 {
			sprite.expandFlipFlop = false
		}
	}
}

func (v *VIC) spriteYMatches(n int) bool {
	return v.Timed[VIC_SPR0_Y+2*n] == byte(v.RasterLine)
}

func (v *VIC) checkSpriteDisplay() {
	for n := range v.sprites {
		sprite := &v.sprites[n]
		sprite.mc = sprite.mcBase
		if sprite.dma && v.spriteYMatches(n) {
			sprite.display = true
		}
		if !sprite.dma {
			sprite.display = false
		}
	}
}

func (v *VIC) fetchSprite(n int) {
	sprite := &v.sprites[n]
	if !sprite.dma {
		return
	}
	bank := uint16(3-v.Mem.VICBank) * 0x4000
	pointerAddress := v.VideoMatrixBase(v.Timed[VIC_MEMPTR]) + 0x3F8 + uint16(n)
	dataAddress := bank + uint16(v.Mem.RAM[pointerAddress])*64 + uint16(sprite.mc)
	copy(sprite.data[:], v.Mem.RAM[dataAddress:dataAddress+spriteDataBytes])
	sprite.mc += spriteDataBytes
}

func (v *VIC) recordSprites() {
	if !v.Recording {
		return
	}
	v.allocateLines()
	line := &v.Lines[v.RasterLine]
	line.Registers = v.Timed
	for n := range v.sprites {
		line.Sprites[n] = SpriteLine{Data: v.sprites[n].data, Display: v.sprites[n].display}
	}
}

func (v *VIC) recordTextRow() {
	if !v.Recording || !v.badLine {
		return
	}
	row := (int(v.RasterLine) - firstBadLine - int(v.Timed[VIC_CTRL1]&7)) / 8
	if row >= 0 && row < len(v.TextRows) {
		v.fillTextRow(&v.TextRows[row], row)
	}
}

func (v *VIC) allocateLines() {
	if v.Lines == nil {
		v.Lines = make([]LineState, v.MaxRasterLine+1)
	}
}

// cpuHeld tells whether the VIC keeps the CPU from using the current cycle: reads stop as
// soon as the bus request (BA) is low, writes only once the VIC owns the bus (AEC), which
// comes 3 cycles later.
func (v *VIC) cpuHeld(write bool) bool {
	cycle := v.cycleInLine
	if v.badLine && cycle >= badLineFirstReadStop && cycle <= badLineLastCycle {
		if !write || cycle >= badLineFirstWriteHalt {
			return true
		}
	}
	for n := range v.sprites {
		if !v.sprites[n].dma {
			continue
		}
		busLow := spritePointerCycle[0] - spriteBusLowLead + 2*n
		owned := spriteFirstDMACycle + 2*n
		last := owned + spriteLinesCycles - 1
		for _, c := range [2]int{cycle, cycle + cyclesPerLine} {
			if c >= busLow && c <= last && (!write || c >= owned) {
				return true
			}
		}
	}
	return false
}

// InterruptOpcode is the opcode to give Retire for an interrupt sequence, which pushes like BRK.
const InterruptOpcode = 0x00

// isWriteCycle tells whether cycle i (of n) of the instruction writes to memory, which the
// CPU can still do while the VIC has only requested the bus.
func isWriteCycle(opcode byte, i, n int) bool {
	switch opcode {
	case 0x84, 0x85, 0x86, 0x8C, 0x8D, 0x8E, 0x94, 0x95, 0x96, 0x99, 0x9D, 0x81, 0x91, 0x48, 0x08:
		return i == n-1
	case 0x06, 0x0E, 0x16, 0x1E, 0x26, 0x2E, 0x36, 0x3E, 0x46, 0x4E, 0x56, 0x5E,
		0x66, 0x6E, 0x76, 0x7E, 0xC6, 0xCE, 0xD6, 0xDE, 0xE6, 0xEE, 0xF6, 0xFE:
		return i >= n-2
	case 0x20:
		return i == 3 || i == 4
	case 0x00:
		return i >= 2 && i <= 4
	}
	return false
}
