// Package main is a Go prototype of the CUBELON 64 rotating-square renderer.
//
// The algorithm is expressed entirely using 8-bit primitives from cpu8.go,
// so every operation has a direct 6502 equivalent.  Once the algorithm is
// correct here, translating it to assembly is mechanical.
//
// Pipeline (mirrors c64port/src/rotation.s + render.s):
//  1. Sin/cos table lookup (by angle byte)
//  2. calc_rotation: compute 4 rotated corners directly in 48×48 supersample space using mul24_signed16
//  3. init 4 DDA edges
//  4. For each output row 0-23: get spans for supersample rows 2Y and 2Y+1
//  5. Downsample 2×2 → 1 pixel (0=off, 1-2=medium, 3-4=full)
//  6. Pack bits into charset bytes (bit7 = leftmost pixel, X=0)
//
// Usage:
//   go run . <angle>          -- render single frame to stdout (text)
//   go run . frames [n]       -- render n frames (default 256) to proto/frames/
//   go run . morph            -- render the morph scene tiles and compare with the ZX reference

package main

import (
	"cubelon64-proto/cpu8"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
)

// ─── Sin/cos tables ──────────────────────────────────────────────────────────
// Optimized: only store the 21 angles used in animation (0°, 4.5°, 9°, ...90°).
// Pre-multiplied: values are sin(angle) * 256 * 24, stored as 16-bit little-endian.
// Indexed by frame number 0-20.
// Cosine computed as sine[20-frame] since cos(θ) = sin(90° - θ).
//
// Memory savings: 512 bytes (256×2) → 42 bytes (21×2) = 470 bytes saved.
// Code savings: Eliminates mul24_signed16 function (~75 bytes).

var sineTable [11][2]byte
var cosineTable [11][2]byte

func initTables() {
	// Compute sine*24 and cosine*24 for 11 frames at 4.5° steps (0° to 45°)
	// Pre-multiply by 24 to eliminate runtime multiplication.
	// Store both sin and cos separately (can't use sin[20-frame] trick with only 11 entries).
	for i := 0; i < 11; i++ {
		degrees := float64(i) * 4.5
		radians := degrees * math.Pi / 180.0
		// sin(angle) * 256 * 24
		sv24 := int16(math.Round(math.Sin(radians) * 256 * 24))
		sineTable[i][0] = byte(sv24)
		sineTable[i][1] = byte(sv24 >> 8)
		// cos(angle) * 256 * 24
		cv24 := int16(math.Round(math.Cos(radians) * 256 * 24))
		cosineTable[i][0] = byte(cv24)
		cosineTable[i][1] = byte(cv24 >> 8)
	}
}

func dumpTables() {
	fmt.Println("; Generated sine/cosine lookup tables for 11 precomputed frames")
	fmt.Println("; Pre-multiplied: sin/cos(i * 4.5°) * 256 * 24, stored as 16-bit little-endian")
	fmt.Println()
	fmt.Println(".segment \"RODATA\"")
	fmt.Println()
	fmt.Println(".export sine_table")
	fmt.Println("sine_table:")
	for i := 0; i < 11; i++ {
		val := int16(sineTable[i][0]) | (int16(sineTable[i][1]) << 8)
		fmt.Printf("    .byte $%02X, $%02X", sineTable[i][0], sineTable[i][1])
		if i < 10 {
			fmt.Printf(",")
		}
		fmt.Printf("  ; frame %2d: %5.1f° (value: %d)\n",
			i, float64(i)*4.5, val)
	}
	fmt.Println()
	fmt.Println(".export cosine_table")
	fmt.Println("cosine_table:")
	for i := 0; i < 11; i++ {
		val := int16(cosineTable[i][0]) | (int16(cosineTable[i][1]) << 8)
		fmt.Printf("    .byte $%02X, $%02X", cosineTable[i][0], cosineTable[i][1])
		if i < 10 {
			fmt.Printf(",")
		}
		fmt.Printf("  ; frame %2d: %5.1f° (value: %d)\n",
			i, float64(i)*4.5, val)
	}
}

// ─── Corner coordinates ──────────────────────────────────────────────────────

type corner struct{ xLo, xHi, yLo, yHi byte }

// calcRotation computes the 4 rotated corners for the given animation frame (0-20).
// Coordinates are directly in 48×48 supersample space (no scaling needed).
// Uses pre-multiplied sin/cos table: values are already sin*256*24.
//
// Square corners relative to pivot (36,36) in Fixed88 supersample space:
//   0: (-24, -24)  →  computed from Corner1 + Corner3 - Pivot
//   1: (  0, -24)  →  uses sin24, cos24
//   2: (  0,   0)  →  pivot itself, no rotation
//   3: (-24,   0)  →  90° rotation of corner 1 (swap + negate)
//
// Optimization: Corner 0 derived from linearity of rotation.
// Corner 3 symmetry: rotating (0, -24) then rotating 90° more = rotating (-24, 0).
// 90° rotation: (x, y) → (y, -x)
func calcRotation(frameNum byte) [4]corner {
	// Load pre-multiplied table: values are already sin*256*24 and cos*256*24
	sin24Lo := sineTable[frameNum][0]
	sin24Hi := sineTable[frameNum][1]
	cos24Lo := cosineTable[frameNum][0]
	cos24Hi := cosineTable[frameNum][1]

	// Pivot (28,36) in Fixed88 supersample space for -4px storage optimization
	// X: 28 = 36 - 8 supersample (-4 output px), Y: 36 unchanged
	const pivotXLo, pivotXHi byte = 0x00, 0x1C
	const pivotYLo, pivotYHi byte = 0x00, 0x24

	var corners [4]corner

	sin24 := int16(sin24Lo) | (int16(sin24Hi) << 8)
	cos24 := int16(cos24Lo) | (int16(cos24Hi) << 8)
	pivotX := int16(pivotXLo) | (int16(pivotXHi) << 8)
	pivotY := int16(pivotYLo) | (int16(pivotYHi) << 8)

	// Corner 1: (0, -24) relative to pivot
	// With px = 0, py = -24:
	//   x' = 0*cos - (-24)*sin = 24*sin
	//   y' = 0*sin + (-24)*cos = -24*cos
	rx1 := sin24
	ry1 := -cos24
	corners[1].xLo, corners[1].xHi = byte(rx1+pivotX), byte((rx1+pivotX)>>8)
	corners[1].yLo, corners[1].yHi = byte(ry1+pivotY), byte((ry1+pivotY)>>8)

	// Corner 3: 90° rotation of corner 1's RELATIVE result: (x1_rot, y1_rot) → (y1_rot, -x1_rot)
	// Then add pivot. 90° rotation formula: (x, y) → (y, -x)
	rx3 := ry1
	ry3 := -rx1
	corners[3].xLo, corners[3].xHi = byte(rx3+pivotX), byte((rx3+pivotX)>>8)
	corners[3].yLo, corners[3].yHi = byte(ry3+pivotY), byte((ry3+pivotY)>>8)

	// Corner 0: Computed from Corner 1 + Corner 3 - Pivot
	// Since rotation is linear: Corner0(rel) = Corner1(rel) + Corner3(rel)
	// In absolute coords: Corner0 = Corner1 + Corner3 - Pivot
	c1x := int16(corners[1].xLo) | (int16(corners[1].xHi) << 8)
	c1y := int16(corners[1].yLo) | (int16(corners[1].yHi) << 8)
	c3x := int16(corners[3].xLo) | (int16(corners[3].xHi) << 8)
	c3y := int16(corners[3].yLo) | (int16(corners[3].yHi) << 8)
	c0x := c1x + c3x - pivotX
	c0y := c1y + c3y - pivotY
	corners[0].xLo, corners[0].xHi = byte(c0x), byte(c0x>>8)
	corners[0].yLo, corners[0].yHi = byte(c0y), byte(c0y>>8)

	// Corner 2: (0, 0) - pivot itself, no rotation
	corners[2].xLo = pivotXLo
	corners[2].xHi = pivotXHi
	corners[2].yLo = pivotYLo
	corners[2].yHi = pivotYHi

	return corners
}

// ─── DDA edge walking ────────────────────────────────────────────────────────

type ddaEdge struct {
	xCurrent byte // current X position (integer pixel)
	errLo    byte // 16-bit Bresenham error accumulator, lo byte
	errHi    byte // 16-bit Bresenham error accumulator, hi byte
	remLo    byte // (dx_full % dy_full), lo byte — added to err each step
	remHi    byte // (dx_full % dy_full), hi byte
	dyLo     byte // dy_full = y1_fixed − y0_fixed, lo byte — threshold for err
	dyHi     byte // dy_full, hi byte; both zero means inactive (0-span edge)
	baseStep byte // floor(dx_full / dy_full): unconditional X pixels per scanline
	dxSign   byte // 0=positive (INC X), 1=negative (DEC X)
	yStart   byte // first active integer scanline  = ceil(y0_fixed/256)
	yEnd     byte // first inactive integer scanline = ceil(y1_fixed/256)
}

// edgeDivResult holds the division result shared between parallel edges.
type edgeDivResult struct {
	baseStep byte
	remLo    byte
	remHi    byte
	dyLo     byte
	dyHi     byte
}

// computeEdgeDivision computes the Bresenham division for an edge (x0,y0)→(x1,y1).
// Returns division result that can be shared between parallel edges.
// Mirrors the div16x16 portion of initEdge.
func computeEdgeDivision(x0Lo, x0Hi, y0Lo, y0Hi, x1Lo, x1Hi, y1Lo, y1Hi byte) edgeDivResult {
	y0Full := uint16(y0Hi)<<8 | uint16(y0Lo)
	y1Full := uint16(y1Hi)<<8 | uint16(y1Lo)
	x0Full := uint16(x0Hi)<<8 | uint16(x0Lo)
	x1Full := uint16(x1Hi)<<8 | uint16(x1Lo)

	// Sort top-to-bottom to get canonical direction
	if y0Full > y1Full {
		x0Lo, x0Hi, y0Lo, y0Hi, x1Lo, x1Hi, y1Lo, y1Hi =
			x1Lo, x1Hi, y1Lo, y1Hi, x0Lo, x0Hi, y0Lo, y0Hi
		y0Full, y1Full = y1Full, y0Full
		x0Full, x1Full = x1Full, x0Full
	}

	dyFull := y1Full - y0Full
	dyLo := byte(dyFull)
	dyHi := byte(dyFull >> 8)

	// dx_full magnitude (unsigned)
	var dxFullLo, dxFullHi byte
	if x1Full >= x0Full {
		dxFullLo = byte(x1Full - x0Full)
		dxFullHi = byte((x1Full - x0Full) >> 8)
	} else {
		dxFullLo = byte(x0Full - x1Full)
		dxFullHi = byte((x0Full - x1Full) >> 8)
	}

	// Division: dx_full / dy_full → baseStep + remainder
	baseStep, remLo, remHi := cpu8.Div16x16(dxFullLo, dxFullHi, dyLo, dyHi)

	return edgeDivResult{baseStep, remLo, remHi, dyLo, dyHi}
}

// initEdge sets up a DDA edge from Fixed88 corner (x0,y0) to (x1,y1).
// All coordinates are full 8.8 Fixed88: lo=fractional byte, hi=integer byte.
// Uses the ceil rule for yStart/yEnd to match the reference's half-open interval.
// Accepts optional pre-computed division result to share between parallel edges.
// Mirrors init_edge_common in render.s (updated for 16-bit Bresenham).
func initEdge(x0Lo, x0Hi, y0Lo, y0Hi, x1Lo, x1Hi, y1Lo, y1Hi byte, divResult *edgeDivResult) ddaEdge {
	var e ddaEdge

	y0Full := uint16(y0Hi)<<8 | uint16(y0Lo)
	y1Full := uint16(y1Hi)<<8 | uint16(y1Lo)
	x0Full := uint16(x0Hi)<<8 | uint16(x0Lo)
	x1Full := uint16(x1Hi)<<8 | uint16(x1Lo)

	// Sort top-to-bottom (smaller y = higher on screen).
	// 6502: compare y0/y1 as 16-bit; conditional swap of all 4 byte pairs.
	if y0Full > y1Full {
		x0Lo, x0Hi, y0Lo, y0Hi, x1Lo, x1Hi, y1Lo, y1Hi =
			x1Lo, x1Hi, y1Lo, y1Hi, x0Lo, x0Hi, y0Lo, y0Hi
		y0Full, y1Full = y1Full, y0Full
		x0Full, x1Full = x1Full, x0Full
	}

	// Active scanline range using the ceil rule:
	//   reference activates edge at integer y iff y0_fixed <= y*256 < y1_fixed.
	//   → yStart = ceil(y0_fixed/256), yEnd = ceil(y1_fixed/256)
	// 6502: LDA yLo; BEQ no_inc; INC yHi
	e.yStart = y0Hi
	if y0Lo > 0 {
		e.yStart++
	}
	e.yEnd = y1Hi
	if y1Lo > 0 {
		e.yEnd++
	}

	if e.yStart >= e.yEnd {
		return e // spans 0 scanlines; dyHi|dyLo stay 0 → stepEdge skips
	}

	// dx_full: 16-bit magnitude and sign.
	var dxFullLo, dxFullHi byte
	if x1Full >= x0Full {
		dxFullLo = byte(x1Full - x0Full)
		dxFullHi = byte((x1Full - x0Full) >> 8)
		e.dxSign = 0
	} else {
		dxFullLo = byte(x0Full - x1Full)
		dxFullHi = byte((x0Full - x1Full) >> 8)
		e.dxSign = 1
	}

	// Use pre-computed division result if provided (for parallel edge optimization)
	if divResult != nil {
		e.baseStep = divResult.baseStep
		e.remLo = divResult.remLo
		e.remHi = divResult.remHi
		e.dyLo = divResult.dyLo
		e.dyHi = divResult.dyHi
	} else {
		// dy_full = y1_fixed − y0_fixed (16-bit, > 0).
		// 6502: SEC; LDA y1Lo; SBC y0Lo → dyLo (with borrow); LDA y1Hi; SBC y0Hi → dyHi
		dyFull := y1Full - y0Full
		e.dyLo = byte(dyFull)
		e.dyHi = byte(dyFull >> 8)

		// Bresenham decomposition: quotient (8-bit, ≤48 for our geometry) + 16-bit remainder.
		// 6502: div16x16 using shift-and-subtract (16 iterations).
		e.baseStep, e.remLo, e.remHi = cpu8.Div16x16(dxFullLo, dxFullHi, e.dyLo, e.dyHi)
	}

	// Sub-pixel start correction: advance xCurrent and seed err to account for
	// the fractional y of the start corner (distance from y0_fixed to yStart*256).
	//   yDelta = yStart*256 − y0_fixed = 256 − y0Lo   (0 when y0Lo==0)
	// 6502: if y0Lo==0 skip; LDA #0; SEC; SBC y0Lo → yDelta (fits in byte: 1–255)
	var yDelta byte
	if y0Lo > 0 {
		yDelta, _ = cpu8.Sbc(0, y0Lo, 1) // 256 - y0Lo
	}

	if yDelta > 0 {
		// xAdvanceFull = dx_full * yDelta / dy_full  (in Fixed88 units, ≤ dx_full ≤ 12288 → uint16 ok)
		// errInit      = dx_full * yDelta % dy_full   (16-bit Bresenham seed)
		// 6502: mul16x8 (dxFull × yDelta) → 24-bit; div24x16 by dyFull
		prod0, prod1, prod2 := cpu8.Mul16x8(dxFullLo, dxFullHi, yDelta)
		xAdvLo, xAdvHi, errLo, errHi := cpu8.Div24x16(prod0, prod1, prod2, e.dyLo, e.dyHi)

		// xCurrent = (x0_fixed ± xAdvanceFull + 128) >> 8  (integer pixel at yStart, rounded)
		// Round to nearest pixel: add 128 before >>8.
		// 6502: CLC/ADC or SEC/SBC for 16-bit with carry threading
		var xFixedLo, xFixedHi byte
		var carry byte
		if e.dxSign == 0 {
			// Positive: (x0Full + xAdvanceFull + 128) >> 8
			xFixedLo, carry = cpu8.Adc(x0Lo, xAdvLo, 0)      // Add xAdv lo
			xFixedHi, carry = cpu8.Adc(x0Hi, xAdvHi, carry)  // Add xAdv hi with carry
			xFixedLo, carry = cpu8.Adc(xFixedLo, 128, 0)     // Add 128 for rounding
			xFixedHi, _ = cpu8.Adc(xFixedHi, 0, carry)       // Propagate carry to hi
			e.xCurrent = xFixedHi                       // Take hi byte (>>8)
		} else {
			// Negative: (x0Full - xAdvanceFull + 128) >> 8
			xFixedLo, carry = cpu8.Sbc(x0Lo, xAdvLo, 1)      // Subtract xAdv lo
			xFixedHi, carry = cpu8.Sbc(x0Hi, xAdvHi, carry)  // Subtract xAdv hi with borrow
			xFixedLo, carry = cpu8.Adc(xFixedLo, 128, 0)     // Add 128 for rounding
			xFixedHi, _ = cpu8.Adc(xFixedHi, 0, carry)       // Propagate carry to hi
			e.xCurrent = xFixedHi                       // Take hi byte (>>8)
		}

		e.errLo = errLo
		e.errHi = errHi
	} else {
		// No sub-pixel correction: xCurrent = round(x0Full), errInit = 0
		// 6502: LDA x0Lo; CMP #128; LDA x0Hi; ADC #0
		e.xCurrent = x0Hi
		if x0Lo >= 128 {
			e.xCurrent++
		}
		e.errLo = 0
		e.errHi = 0
	}

	return e
}

// stepEdge checks whether scanY intersects this edge and, if so, records the
// X position in spanX1 or spanX2 and advances the Bresenham accumulator.
// Mirrors step_edge_0_1 / step_edge_1_2 / ... in render.s.
func stepEdge(e *ddaEdge, scanY byte, ic *byte, spanX1, spanX2 *byte) {
	// Range check: [yStart, yEnd)
	// Note: (dyHi==0 && dyLo==0) check is redundant because initEdge ensures
	// yStart >= yEnd when dy==0, so the range check already covers inactive edges.
	if scanY < e.yStart || cpu8.CmpGE(scanY, e.yEnd) {
		return
	}
	// Record intersection (first or second)
	if *ic == 0 {
		*spanX1 = e.xCurrent
	} else {
		*spanX2 = e.xCurrent
	}
	*ic++

	// Bresenham step:
	//   1. Apply base step unconditionally.
	//   2. Accumulate 16-bit remainder into 16-bit error; if error >= dy_full → extra ±1.
	// 6502: CLC/ADC or SEC/SBC for base step, then CLC/ADC for remainder, CMP + branch for threshold.
	if e.dxSign == 0 {
		e.xCurrent, _ = cpu8.Adc(e.xCurrent, e.baseStep, 0) // clc / adc baseStep
	} else {
		e.xCurrent, _ = cpu8.Sbc(e.xCurrent, e.baseStep, 1) // sec / sbc baseStep
	}

	var carry byte
	e.errLo, carry = cpu8.Adc(e.errLo, e.remLo, 0)  // clc / adc remLo
	e.errHi, _ = cpu8.Adc(e.errHi, e.remHi, carry)  // adc remHi

	if cpu8.Cmp16GE(e.errLo, e.errHi, e.dyLo, e.dyHi) { // err >= dy_full?
		var c byte
		e.errLo, c = cpu8.Sbc(e.errLo, e.dyLo, 1) // sec / sbc dyLo
		e.errHi, _ = cpu8.Sbc(e.errHi, e.dyHi, c) // sbc dyHi
		if e.dxSign == 0 {
			e.xCurrent++ // inc
		} else {
			e.xCurrent-- // dec
		}
	}
}

// ─── Render ──────────────────────────────────────────────────────────────────

// Bit-mask table: bit7=leftmost pixel, bit0=rightmost (same as C64 charset).
var bitMask = [8]byte{0x80, 0x40, 0x20, 0x10, 0x08, 0x04, 0x02, 0x01}

// Mask lookup indexed by supersample coordinate (ssX & 15).
// Since ssX is always even (0, 2, 4, ..., 46), this directly maps ssX to mask
// without needing to divide by 2 first. Odd entries duplicate even for completeness.
var maskBySSX = [16]byte{
	0x80, 0x80, // ssX=0,1 → mask for bit 0
	0x40, 0x40, // ssX=2,3 → mask for bit 1
	0x20, 0x20, // ssX=4,5 → mask for bit 2
	0x10, 0x10, // ssX=6,7 → mask for bit 3
	0x08, 0x08, // ssX=8,9 → mask for bit 4
	0x04, 0x04, // ssX=10,11 → mask for bit 5
	0x02, 0x02, // ssX=12,13 → mask for bit 6
	0x01, 0x01, // ssX=14,15 → mask for bit 7
}

// renderSquare renders the rotated square for the given animation frame (0-20).
//
// Writes directly to dest in sequential 6-bytes-per-row format (rows 1-17 only):
//   dest[(row-1)*6+0] = charset row, column 0 bitmask
//   dest[(row-1)*6+1] = charset row, column 1 bitmask
//   dest[(row-1)*6+2] = charset row, column 2 bitmask
//   dest[(row-1)*6+3] = sprite row, column 0 bitmask
//   dest[(row-1)*6+4] = sprite row, column 1 bitmask
//   dest[(row-1)*6+5] = sprite row, column 2 bitmask
//
// Output resolution: 24×24 pixels (3×3 characters of 8×8 each).
// Supersampling: 48×48 intermediate space, 2×2 → 1 with threshold 0→off,1-2→med,3-4→full.
// Mirrors render_square in render.s.
func renderSquare(frameNum byte, dest []byte) {
	renderCorners(calcRotation(frameNum), true, dest)
}

// renderCorners fills the convex quad given by corners (Fixed88, 48×48 supersample space)
// and writes rows 1-17 of the two-tone bitmaps to dest.
// sharedDivisions is only valid for parallelograms: opposite edges then share one division.
func renderCorners(corners [4]corner, sharedDivisions bool, dest []byte) {
	var edges [4]ddaEdge
	if sharedDivisions {
		// Parallel edge optimization: square has opposite edges parallel with identical division results.
		// Edges 0→1 and 2→3 are parallel (share baseStep, rem, dy).
		// Edges 1→2 and 3→0 are parallel (share baseStep, rem, dy).
		// Compute division once per parallel pair → saves ~600-800 cycles (2 div16x16 calls).
		div01 := computeEdgeDivision(corners[0].xLo, corners[0].xHi, corners[0].yLo, corners[0].yHi, corners[1].xLo, corners[1].xHi, corners[1].yLo, corners[1].yHi)
		div12 := computeEdgeDivision(corners[1].xLo, corners[1].xHi, corners[1].yLo, corners[1].yHi, corners[2].xLo, corners[2].xHi, corners[2].yLo, corners[2].yHi)
		edges = [4]ddaEdge{
			initEdge(corners[0].xLo, corners[0].xHi, corners[0].yLo, corners[0].yHi, corners[1].xLo, corners[1].xHi, corners[1].yLo, corners[1].yHi, &div01),
			initEdge(corners[1].xLo, corners[1].xHi, corners[1].yLo, corners[1].yHi, corners[2].xLo, corners[2].xHi, corners[2].yLo, corners[2].yHi, &div12),
			initEdge(corners[2].xLo, corners[2].xHi, corners[2].yLo, corners[2].yHi, corners[3].xLo, corners[3].xHi, corners[3].yLo, corners[3].yHi, &div01),
			initEdge(corners[3].xLo, corners[3].xHi, corners[3].yLo, corners[3].yHi, corners[0].xLo, corners[0].xHi, corners[0].yLo, corners[0].yHi, &div12),
		}
	} else {
		for i := range edges {
			from, to := corners[i], corners[(i+1)%4]
			div := computeEdgeDivision(from.xLo, from.xHi, from.yLo, from.yHi, to.xLo, to.xHi, to.yLo, to.yHi)
			edges[i] = initEdge(from.xLo, from.xHi, from.yLo, from.yHi, to.xLo, to.xHi, to.yLo, to.yHi, &div)
		}
	}

	// Geometry optimization: Based on actual bounds from refRenderSquare:
	//   - Row 0 is always empty (globalMin = 1)
	//   - Rows 18-23 are always empty (globalMax = 17)
	// Only process rows 1-17 (rows 0, 18-23 not written to dest)

	// Process each output row (1-17).
	var outY byte
	for outY = 1; outY < 18; outY++ {
		// Fetch spans for supersample rows 2Y and 2Y+1.
		type span struct {
			x1, x2  byte
			valid   bool
		}

		getSpan := func(ssY byte) span {
			var ic byte
			var x1, x2 byte
			for i := range edges {
				stepEdge(&edges[i], ssY, &ic, &x1, &x2)
				// Early exit: once we have 2 intersections, no need to check more edges.
				// 6502: CPX #2 / BEQ done
				if ic >= 2 {
					break
				}
			}
			if ic < 2 {
				return span{}
			}
			if x1 > x2 {
				x1, x2 = x2, x1
			}
			return span{x1, x2, true}
		}

		ssY, _ := cpu8.Asl(outY) // 2Y
		row0 := getSpan(ssY)
		ssY++               // 2Y+1
		row1 := getSpan(ssY)

		// Early exit: if both supersample rows are empty, write 0xFF (all lit) and skip pixel loop.
		// Saves ~150-200 cycles per empty row.
		// 6502: LDA row0.valid / ORA row1.valid / BEQ write_ones (6 STA)
		if !row0.valid && !row1.valid {
			offset := (outY - 1) * 6
			dest[offset+0] = 0xFF // col0 charset
			dest[offset+1] = 0xFF // col1 charset
			dest[offset+2] = 0xFF // col2 charset
			dest[offset+3] = 0xFF // col0 sprite
			dest[offset+4] = 0xFF // col1 sprite
			dest[offset+5] = 0xFF // col2 sprite
			continue
		}

		// Downsample: for each output column count subpixels then threshold.
		// Initialize to 0xFF (all pixels lit by default, we'll clear bits where square is).
		var col0Full, col1Full, col2Full byte = 0xFF, 0xFF, 0xFF     // full-brightness charset
		var col0Med, col1Med, col2Med byte = 0xFF, 0xFF, 0xFF        // medium-brightness (sprite/overlay)

		// Span-based filling with overdraw: compute bounds, only check pixels in range.
		// Pixels outside span stay lit (0xFF), guaranteeing complete overdraw.
		// Compute supersample bounds directly (already in 48×48 space).
		ssXStart, ssXEnd := byte(48), byte(0)
		if row0.valid || row1.valid {
			// Find min/max supersample X across both rows
			ssXMin, ssXMax := byte(48), byte(0)
			if row0.valid {
				if row0.x1 < ssXMin {
					ssXMin = row0.x1
				}
				if row0.x2 > ssXMax {
					ssXMax = row0.x2
				}
			}
			if row1.valid {
				if row1.x1 < ssXMin {
					ssXMin = row1.x1
				}
				if row1.x2 > ssXMax {
					ssXMax = row1.x2
				}
			}
			// Round to even coordinates for loop (increment by 2)
			ssXStart = ssXMin & ^byte(1)        // Round down to even
			ssXEnd = (ssXMax + 2) & ^byte(1)    // Round up to next even after ssXMax
		}

		// Iterate in supersample space (2× coordinates) to avoid conversion overhead.
		// Use ssX as loop variable (0, 2, 4, ..., 46), derive outX when needed.
		// Track both ssX (even) and ssX1 (odd) to avoid recomputing ssX+1 every iteration.
		// On 6502: X register for ssX, Y register for ssX1, increment both (INX INX / INY INY).
		ssX1 := ssXStart + 1 // ssX + 1, tracked in parallel
		for ssX := ssXStart; ssX < ssXEnd; {

			// Count how many of the 4 subpixels are inside spans.
			// Hoist validity checks: check row0.valid once (not twice) and row1.valid once (not twice).
			// On 6502: LDA row.valid + BEQ is 4 cycles. Hoisting saves 4-8 cycles per pixel.
			var count byte

			// Check both subpixels in row0 if valid (one validity check instead of two)
			if row0.valid {
				if cpu8.CmpGE(ssX, row0.x1) && !cpu8.CmpGE(ssX, row0.x2) {
					count++
				}
				if cpu8.CmpGE(ssX1, row0.x1) && !cpu8.CmpGE(ssX1, row0.x2) {
					count++
				}
			}

			// Check subpixels in row1 if valid (one validity check, with early exit at count>=3)
			if row1.valid {
				if cpu8.CmpGE(ssX, row1.x1) && !cpu8.CmpGE(ssX, row1.x2) {
					count++
				}
				// Early exit: if count >= 3, skip 4th subpixel check (3 and 4 both map to full brightness)
				if count < 3 {
					if cpu8.CmpGE(ssX1, row1.x1) && !cpu8.CmpGE(ssX1, row1.x2) {
						count++
					}
				}
			}

			// Only clear bits if count > 0 (pixel inside square); otherwise stays lit (1 bit).
			if count > 0 {
				// For -4px storage: wrap ssX >= 48 to col0 (circular 24-pixel / 48-supersample space)
				wrappedSSX := ssX
				if cpu8.CmpGE(ssX, 48) {
					wrappedSSX = ssX - 48
				}

				// Direct mask lookup from supersample coordinate.
				// Replaces: outX = ssX>>1 (LSR, 2cyc), bitPos = outX&7 (AND, 2cyc), mask = bitMask[bitPos] (LDA, 4cyc)
				// With: maskIdx = ssX&15 (AND, 2cyc), mask = maskBySSX[maskIdx] (LDA, 4cyc)
				// Saves: 8 - 6 = 2 cycles per pixel with coverage.
				maskIdx := wrappedSSX & 15
				mask := maskBySSX[maskIdx]

				// Column selection using supersample coordinate (thresholds doubled).

				if cpu8.CmpGE(count, 3) { // 3 or 4 subpixels inside: clear both → $f background
					if !cpu8.CmpGE(wrappedSSX, 32) {
						if !cpu8.CmpGE(wrappedSSX, 16) {
							col0Full &= ^mask
							col0Med &= ^mask
						} else {
							col1Full &= ^mask
							col1Med &= ^mask
						}
					} else {
						col2Full &= ^mask
						col2Med &= ^mask
					}
				} else { // 1 or 2 subpixels inside: clear full only → $b sprite
					if !cpu8.CmpGE(wrappedSSX, 32) {
						if !cpu8.CmpGE(wrappedSSX, 16) {
							col0Full &= ^mask
						} else {
							col1Full &= ^mask
						}
					} else {
						col2Full &= ^mask
					}
				}
			}

			// Increment by 2 in supersample space.
			// 6502: INX INX / INY INY (8 cycles total) vs previous (ssX+=2, ssX1=ssX+1: 10 cycles) = 2 cycles saved.
			ssX, _ = cpu8.Adc(ssX, 2, 0)
			ssX1 += 2
		}

		// Write scanline bitmasks to dest in sequential format.
		offset := (outY - 1) * 6
		dest[offset+0] = col0Full // col0 charset
		dest[offset+1] = col1Full // col1 charset
		dest[offset+2] = col2Full // col2 charset
		dest[offset+3] = col0Med  // col0 sprite
		dest[offset+4] = col1Med  // col1 sprite
		dest[offset+5] = col2Med  // col2 sprite
	}
}

// ─── Visualisation ───────────────────────────────────────────────────────────

// charsetToImage converts 3×8 charset rows to a 24×24 pixel grid scaled by
// pixelSize for visibility. full pixels → fullColor, medium → medColor.
func charsetToImage(full, medium [192]byte, pixelSize int, fullColor, medColor, bgColor color.Color) *image.RGBA {
	size := 24 * pixelSize
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, bgColor)
		}
	}
	for py := 0; py < 24; py++ {
		for px := 0; px < 24; px++ {
			col := px / 8
			bit := 7 - (px % 8)
			fullBit := (full[col*64+py]>>bit)&1 != 0
			medBit := (medium[col*64+py]>>bit)&1 != 0
			var c color.Color
			// C64 hardware display (sprites behind foreground):
			// full=0, med=0 → inside square, high coverage → $f background (both clear)
			// full=0, med=1 → inside square, edge → $b sprite (charset clear, sprite set)
			// full=1, med=X → outside square → $0 foreground (charset set, covers sprite)
			switch {
			case !fullBit && !medBit:
				c = bgColor // $f light grey (background)
			case !fullBit && medBit:
				c = medColor // $b dark grey (sprite layer)
			case fullBit:
				c = fullColor // $0 black (foreground)
			}
			for dy := 0; dy < pixelSize; dy++ {
				for dx := 0; dx < pixelSize; dx++ {
					img.Set(px*pixelSize+dx, py*pixelSize+dy, c)
				}
			}
		}
	}
	return img
}

// ─── Main ────────────────────────────────────────────────────────────────────

func main() {
	if len(os.Args) > 1 && os.Args[1] == "morph" {
		runMorph()
		return
	}
	initTables()
	animateDemo()
	visualizeFrames()
	createMontage()
}

// ─── Reference renderer (intersection-based, not DDA) ────────────────────────
//
// refRenderSquare uses the same corners as renderSquare (calcRotation with our
// lookup table) but replaces the Bresenham DDA with the direct intersection
// formula from tools/generate_comparison_with_reference.go:
//
//   t = (scanY - y0) / (y1 - y0)
//   x = x0 + (x1-x0)*t   →  rounded: (x*256 + 128) >> 8
//
// This is the ground truth for the DDA correctness check.

// cornersAsFixed88 reconstructs int16 Fixed88 values from the byte-pair corner.
func cornersAsFixed88(frameNum byte) (xs, ys [4]int32) {
	c := calcRotation(frameNum)
	for i, cn := range c {
		xs[i] = int32(int16(uint16(cn.xLo) | uint16(cn.xHi)<<8))
		ys[i] = int32(int16(uint16(cn.yLo) | uint16(cn.yHi)<<8))
	}
	return
}

func refRenderSquare(frameNum byte) (charset [192]byte, mediumCharset [192]byte) {
	xs, ys := cornersAsFixed88(frameNum)

	// Corners are already in 48×48 supersample space - no scaling needed.
	sxs, sys := xs, ys

	// Point-in-polygon test using cross products.
	// For a convex quad, point P is inside if it's on the left side of all edges (CCW winding).
	// Edge i→j: cross = (x[j]-x[i])*(P.y-y[i]) - (y[j]-y[i])*(P.x-x[i])
	// Positive cross = P is on left (inside for CCW).
	pointInside := func(px, py int32) bool {
		for i := 0; i < 4; i++ {
			j := (i + 1) % 4
			cross := (sxs[j]-sxs[i])*(py-sys[i]) - (sys[j]-sys[i])*(px-sxs[i])
			if cross < 0 {
				return false
			}
		}
		return true
	}

	for outY := 0; outY < 24; outY++ {
		var col0F, col1F, col2F byte
		var col0M, col1M, col2M byte

		for outX := 0; outX < 24; outX++ {
			// Test 4 subpixel centers: (outX*2+0.5, outY*2+0.5), etc. in supersample space.
			// In Fixed88: subpixel center = (ss_coord << 8) + 128
			var count byte
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					ssX := int32((outX*2 + dx) << 8)
					ssY := int32((outY*2 + dy) << 8)
					// Add 0.5 (128 in Fixed88) to sample at subpixel center
					ssX += 128
					ssY += 128
					// Handle horizontal wrapping: test both positions
					if pointInside(ssX, ssY) {
						count++
					} else if pointInside(ssX+int32(48<<8), ssY) {
						count++
					}
				}
			}
			if count == 0 {
				continue
			}
			mask := bitMask[outX&7]
			if count >= 3 { // Set both, inverts to 0,0 → $f background
				if outX < 8 {
					col0F |= mask
					col0M |= mask
				} else if outX < 16 {
					col1F |= mask
					col1M |= mask
				} else {
					col2F |= mask
					col2M |= mask
				}
			} else { // Set full only, inverts to 0,1 → $b sprite
				if outX < 8 {
					col0F |= mask
				} else if outX < 16 {
					col1F |= mask
				} else {
					col2F |= mask
				}
			}
		}
		charset[outY] = col0F
		charset[64+outY] = col1F
		charset[128+outY] = col2F
		mediumCharset[outY] = col0M
		mediumCharset[64+outY] = col1M
		mediumCharset[128+outY] = col2M
	}

	// Invert all bits to match renderSquare output format.
	for i := 0; i < 192; i++ {
		charset[i] ^= 0xFF
		mediumCharset[i] ^= 0xFF
	}

	return
}

// getCount counts subpixel coverage (0-4) for a pixel from charset bytes.
func getCount(full, med [192]byte, outX, outY int) int {
	col := outX / 8
	bit := 7 - (outX % 8)
	fullBit := (full[col*64+outY]>>bit)&1 != 0
	medBit := (med[col*64+outY]>>bit)&1 != 0
	// Encoding: bit=1 means lit, bit=0 means square pixel
	// full=0, med=0 → both dark → $f background (3-4 subpixels)
	// full=0, med=1 → charset dark, sprite lit → $b sprite (1-2 subpixels)
	// full=1, med=X → charset lit → $0 foreground black (0 subpixels, outside)
	if !fullBit && !medBit {
		return 3 // high coverage
	} else if !fullBit && medBit {
		return 1 // medium coverage
	}
	return 0 // outside square
}

// diffCharsets counts bit-level pixel differences between two charset arrays
// (only the full-brightness layer; 24 rows × 3 columns = 72 bytes checked).
func diffCharsets(a, b [192]byte) (diffPixels int) {
	for col := 0; col < 3; col++ {
		for row := 0; row < 24; row++ {
			xor := a[col*64+row] ^ b[col*64+row]
			for xor != 0 {
				diffPixels += int(xor & 1)
				xor >>= 1
			}
		}
	}
	return
}

// runCompare compares proto DDA output against reference intersection output
// for the 11 angles used in generate_comparison_with_reference.go (0°–90° in 9° steps).
func runCompare() {
	initTables()

	// Compare all 21 animation frames.
	degs := [21]float64{0, 4.5, 9, 13.5, 18, 22.5, 27, 31.5, 36, 40.5, 45, 49.5, 54, 58.5, 63, 67.5, 72, 76.5, 81, 85.5, 90}

	total := 0
	fmt.Printf("%-6s %-8s  %-6s  notes\n", "angle", "frame", "diffs")
	fmt.Println(strings.Repeat("-", 50))
	for i := 0; i < 21; i++ {
		var tempBuf [128]byte
		renderSquare(byte(i), tempBuf[:])
		protFull, _ := unpackPrecomputed(tempBuf[:])
		refFull, _ := refRenderSquare(byte(i))
		d := diffCharsets(protFull, refFull)
		total += d
		note := ""
		if d == 0 {
			note = "exact match"
		}
		fmt.Printf("%5.1f°  frame %-3d  %4d px  %s\n", degs[i], i, d, note)
	}
	fmt.Printf("%-6s %-8s  %-6d\n", "total", "", total)

	if err := os.MkdirAll("frames", 0755); err == nil {
		for i := 0; i < 21; i++ {
			var tempBuf [128]byte
			renderSquare(byte(i), tempBuf[:])
			protFull, protMed := unpackPrecomputed(tempBuf[:])
			refFull, refMed := refRenderSquare(byte(i))

			// C64 palette colors
			black := color.RGBA{0x00, 0x00, 0x00, 0xFF}      // $0: black (full brightness square pixels)
			darkGrey := color.RGBA{0x5F, 0x5F, 0x5F, 0xFF}   // $b: dark grey (sprite/medium pixels)
			lightGrey := color.RGBA{0xC6, 0xC6, 0xC6, 0xFF}  // $f: light grey (background)
			red := color.RGBA{0xFF, 0x00, 0x00, 0xFF}

			// Diff image: proto output with diff pixels marked by red dots sized by error magnitude.
			diffImg := charsetToImage(protFull, protMed, 8, black, darkGrey, lightGrey)

			// Overlay red dots where proto differs from reference.
			// Dot radius scales with error: 1×1 (error=1) to 5×5 (error=4).
			for outY := 0; outY < 24; outY++ {
				for outX := 0; outX < 24; outX++ {
					protCount := getCount(protFull, protMed, outX, outY)
					refCount := getCount(refFull, refMed, outX, outY)
					errMag := protCount - refCount
					if errMag < 0 {
						errMag = -errMag
					}
					if errMag > 0 {
						// Dot radius: 1×1 for error=1, 3×3 for error=2-3, 5×5 for error=4
						radius := errMag
						if radius > 2 {
							radius = 2
						}
						// Draw centered dot
						centerX := outX*8 + 4
						centerY := outY*8 + 4
						for dy := -radius; dy <= radius; dy++ {
							for dx := -radius; dx <= radius; dx++ {
								diffImg.Set(centerX+dx, centerY+dy, red)
							}
						}
					}
				}
			}
			f, _ := os.Create(fmt.Sprintf("frames/compare_%02d_%05.1fdeg.png", i, degs[i]))
			png.Encode(f, diffImg)
			f.Close()

			// Also save the reference frame for side-by-side inspection.
			refImg := charsetToImage(refFull, refMed, 8, black, darkGrey, lightGrey)
			f, _ = os.Create(fmt.Sprintf("frames/ref_%02d_%05.1fdeg.png", i, degs[i]))
			png.Encode(f, refImg)
			f.Close()
		}
		fmt.Printf("\nWrote proto+diff and reference frames to frames/\n")
	}
}

// ─── Self-checks against emulator pixel-perfect assertions ───────────────────

func assertAngle(label string, frameNum byte, want func(row int) (col0, col1, col2 byte)) bool {
	var tempBuf [128]byte
	renderSquare(frameNum, tempBuf[:])
	full, _ := unpackPrecomputed(tempBuf[:])
	pass := true
	for row := 0; row < 24; row++ {
		wc0, wc1, wc2 := want(row)
		c0, c1, c2 := full[row], full[64+row], full[128+row]
		if c0 != wc0 || c1 != wc1 || c2 != wc2 {
			fmt.Printf("  %s row %2d: c0=%02X(want %02X) c1=%02X(want %02X) c2=%02X(want %02X)\n",
				label, row, c0, wc0, c1, wc1, c2, wc2)
			pass = false
		}
	}
	if pass {
		fmt.Printf("  %s PASS\n", label)
	}
	return pass
}

func runChecks() {
	initTables()
	allPass := true

	// Verify critical angles are pixel-perfect (exact matches with reference).
	// Intermediate angles have ~216 total diffs (~1.8% error) due to 8-bit
	// Bresenham approximation vs. reference's per-scanline exact intersection.

	// frame 0 (0°): 12×12 square at (6,6)-(17,17), no rotation.
	// Inverted format: square pixels are 0 (cleared), background is 1 (set).
	allPass = assertAngle("frame 0 (0°)", 0, func(row int) (byte, byte, byte) {
		if row >= 6 && row <= 17 {
			return 0xFC, 0x00, 0x3F
		}
		return 0xFF, 0xFF, 0xFF
	}) && allPass

	// frame 10 (45°): diamond, pixel-perfect due to symmetry.
	// Geometric reference detects wrapped pixels at edges.
	allPass = assertAngle("frame 10 (45°)", 10, func(row int) (byte, byte, byte) {
		switch row {
		case 2, 17:
			return 0xFF, 0xFF, 0x9F
		case 3, 16:
			return 0xFF, 0xFF, 0x0F
		case 4, 15:
			return 0xFF, 0xFE, 0x07
		case 5, 14:
			return 0xFF, 0xFC, 0x03
		case 6, 13:
			return 0xFF, 0xF8, 0x01
		case 7, 12:
			return 0xFF, 0xF0, 0x00
		case 8, 11:
			return 0x7F, 0xE0, 0x00 // col0 wrapped pixels
		case 9, 10:
			return 0x3F, 0xC0, 0x00 // col0 wrapped pixels
		}
		return 0xFF, 0xFF, 0xFF
	}) && allPass

	// frame 20 (90°): 12×12 square rotated 90° with horizontal wrapping.
	allPass = assertAngle("frame 20 (90°)", 20, func(row int) (byte, byte, byte) {
		if row >= 6 && row <= 17 {
			return 0x03, 0xFF, 0xC0 // col0 shows wrapped pixels
		}
		return 0xFF, 0xFF, 0xFF
	}) && allPass

	if allPass {
		fmt.Println("All checks PASSED")
	} else {
		fmt.Fprintln(os.Stderr, "FAILED")
		os.Exit(1)
	}
}
