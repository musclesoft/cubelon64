// Package main is a Go prototype of the CUBELON 64 rotating-square renderer.
//
// The algorithm is expressed entirely using 8-bit primitives from cpu8.go,
// so every operation has a direct 6502 equivalent.  Once the algorithm is
// correct here, translating it to assembly is mechanical.
//
// Pipeline (mirrors c64port/src/rotation.s + render.s):
//  1. Sin/cos table lookup (by angle byte)
//  2. rotate_point: signed 16×16 fixed-point multiply (mul16x16_signed32)
//  3. calc_rotation: compute 4 rotated corners
//  4. scale_corners_2x: double corners for 48×48 supersample space
//  5. init 4 DDA edges
//  6. For each output row 0-23: get spans for supersample rows 2Y and 2Y+1
//  7. Downsample 2×2 → 1 pixel (0=off, 1-2=medium, 3-4=full)
//  8. Pack bits into charset bytes (bit7 = leftmost pixel, X=0)
//
// Usage:
//   go run . <angle>          -- render single frame to stdout (text)
//   go run . frames [n]       -- render n frames (default 256) to proto/frames/

package main

import (
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
// Each entry: sin(angle*2π/256)*256 rounded, stored as [lo, hi] Fixed88.
// Indexed by frame number 0-20.
//
// Memory savings: 512 bytes (256×2) → 42 bytes (21×2) = 470 bytes saved.

var sineTable [21][2]byte
var cosineTable [21][2]byte

// Angle bytes for the 21 animation frames (4.5° steps, 0° to 90°).
var animationAngles = [21]byte{0, 3, 6, 10, 13, 16, 19, 22, 26, 29, 32, 35, 38, 42, 45, 48, 51, 54, 58, 61, 64}

func initTables() {
	// Compute sin/cos only for the 21 angles we actually use.
	for i := 0; i < 21; i++ {
		angle := animationAngles[i]
		sv := int16(math.Round(math.Sin(float64(angle)*2*math.Pi/256) * 256))
		cv := int16(math.Round(math.Cos(float64(angle)*2*math.Pi/256) * 256))
		sineTable[i] = [2]byte{byte(sv), byte(sv >> 8)}
		cosineTable[i] = [2]byte{byte(cv), byte(cv >> 8)}
	}
}

// ─── Signed 16×16 → 32-bit multiply ─────────────────────────────────────────
// Mirrors mul16x16_signed32 in rotation.s.
// All 4 partial products use mul8x8; signs handled by negate-if-negative.

func mul16x16(aLo, aHi, bLo, bHi byte) (r0, r1, r2, r3 byte) {
	resultNeg := isNeg(aHi) != isNeg(bHi)
	if isNeg(aHi) {
		aLo, aHi = neg16(aLo, aHi)
	}
	if isNeg(bHi) {
		bLo, bHi = neg16(bLo, bHi)
	}

	var carry byte

	// AL*BL → r0, r1
	r0, r1 = mul8x8(aLo, bLo)

	// AL*BH → add to r1, r2
	lo, hi := mul8x8(aLo, bHi)
	r1, carry = adc(r1, lo, 0)
	r2, _ = adc(r2, hi, carry)

	// AH*BL → add to r1, r2 (carry propagates to r3)
	lo, hi = mul8x8(aHi, bLo)
	r1, carry = adc(r1, lo, 0)
	r2, carry = adc(r2, hi, carry)
	r3, _ = adc(r3, 0, carry)

	// AH*BH → add to r2, r3
	lo, hi = mul8x8(aHi, bHi)
	r2, carry = adc(r2, lo, 0)
	r3, _ = adc(r3, hi, carry)

	if resultNeg {
		r0, r1, r2, r3 = neg32(r0, r1, r2, r3)
	}
	return
}

// ─── Fixed-point point rotation ──────────────────────────────────────────────
// rotatePoint: x' = (px*cos − py*sin)>>8, y' = (px*sin + py*cos)>>8.
// All values are Fixed88 (lo=frac byte, hi=integer byte).
// Mirrors rotate_point in rotation.s.
//
// The >>8 is achieved by discarding the lowest byte (b0) of the 32-bit
// product and keeping bytes b1 (lo) and b2 (hi) as the Fixed88 result.

func rotatePoint(pxLo, pxHi, pyLo, pyHi, cosLo, cosHi, sinLo, sinHi byte) (rxLo, rxHi, ryLo, ryHi byte) {
	// x*cos (32-bit)
	xc0, xc1, xc2, xc3 := mul16x16(pxLo, pxHi, cosLo, cosHi)
	// y*sin (32-bit)
	ys0, ys1, ys2, ys3 := mul16x16(pyLo, pyHi, sinLo, sinHi)

	// x*cos - y*sin (32-bit subtraction)
	var c byte
	var d1, d2 byte
	_, c = sbc(xc0, ys0, 1) // byte 0: discarded
	d1, c = sbc(xc1, ys1, c)
	d2, c = sbc(xc2, ys2, c)
	_, _ = sbc(xc3, ys3, c) // byte 3: discarded (sign extension)
	rxLo, rxHi = d1, d2     // >>8: keep bytes [1],[2]

	// x*sin (32-bit)
	xs0, xs1, xs2, xs3 := mul16x16(pxLo, pxHi, sinLo, sinHi)
	// y*cos (32-bit)
	yc0, yc1, yc2, yc3 := mul16x16(pyLo, pyHi, cosLo, cosHi)

	// x*sin + y*cos (32-bit addition)
	var carry byte
	var s1, s2 byte
	_, carry = adc(xs0, yc0, 0) // byte 0: discarded
	s1, carry = adc(xs1, yc1, carry)
	s2, carry = adc(xs2, yc2, carry)
	_, _ = adc(xs3, yc3, carry) // byte 3: discarded
	ryLo, ryHi = s1, s2         // >>8: keep bytes [1],[2]

	return
}

// ─── Corner coordinates ──────────────────────────────────────────────────────

type corner struct{ xLo, xHi, yLo, yHi byte }

// scale2x doubles a Fixed88 value by shifting left 1 (ASL lo / ROL hi).
func scale2x(lo, hi byte) (byte, byte) {
	lo, c := asl(lo)
	hi, _ = rol(hi, c)
	return lo, hi
}

// calcRotation computes the 4 rotated corners for the given animation frame (0-20).
// Uses symmetry optimization to reduce 16 muls → 8 muls:
//
// Square corners relative to pivot (18,18) in Fixed88:
//   0: (-12, -12)  →  full rotation (4 muls)
//   1: (  0, -12)  →  full rotation (4 muls)
//   2: (  0,   0)  →  pivot itself, no rotation (0 muls)
//   3: (-12,   0)  →  90° rotation of corner 1 (0 muls: swap + negate)
//
// Corner 3 symmetry: rotating (0, -12) then rotating 90° more = rotating (-12, 0).
// 90° rotation: (x, y) → (y, -x)
func calcRotation(frameNum byte) [4]corner {
	sinLo, sinHi := sineTable[frameNum][0], sineTable[frameNum][1]
	cosLo, cosHi := cosineTable[frameNum][0], cosineTable[frameNum][1]

	// Pivot (18,18) in Fixed88: hi=$12, lo=$00
	const pivotLo, pivotHi byte = 0x00, 0x12

	var corners [4]corner

	// Corner 0: (-12, -12) - full rotation
	rx0Lo, rx0Hi, ry0Lo, ry0Hi := rotatePoint(0x00, 0xF4, 0x00, 0xF4, cosLo, cosHi, sinLo, sinHi)
	var carry byte
	corners[0].xLo, carry = adc(rx0Lo, pivotLo, 0)
	corners[0].xHi, _ = adc(rx0Hi, pivotHi, carry)
	corners[0].yLo, carry = adc(ry0Lo, pivotLo, 0)
	corners[0].yHi, _ = adc(ry0Hi, pivotHi, carry)

	// Corner 1: (0, -12) - full rotation
	rx1Lo, rx1Hi, ry1Lo, ry1Hi := rotatePoint(0x00, 0x00, 0x00, 0xF4, cosLo, cosHi, sinLo, sinHi)
	corners[1].xLo, carry = adc(rx1Lo, pivotLo, 0)
	corners[1].xHi, _ = adc(rx1Hi, pivotHi, carry)
	corners[1].yLo, carry = adc(ry1Lo, pivotLo, 0)
	corners[1].yHi, _ = adc(ry1Hi, pivotHi, carry)

	// Corner 2: (0, 0) - pivot itself, no rotation
	corners[2].xLo = pivotLo
	corners[2].xHi = pivotHi
	corners[2].yLo = pivotLo
	corners[2].yHi = pivotHi

	// Corner 3: 90° rotation of corner 1's RELATIVE result: (x1_rot, y1_rot) → (y1_rot, -x1_rot)
	// Then add pivot. 90° rotation formula: (x, y) → (y, -x)
	// x3_rot = y1_rot
	rx3Lo := ry1Lo
	rx3Hi := ry1Hi
	// y3_rot = -x1_rot = ~x1_rot + 1 (two's complement)
	var c byte
	ry3Lo, c := adc(^rx1Lo, 1, 0)
	ry3Hi, _ := adc(^rx1Hi, 0, c)
	// Add pivot offset
	corners[3].xLo, carry = adc(rx3Lo, pivotLo, 0)
	corners[3].xHi, _ = adc(rx3Hi, pivotHi, carry)
	corners[3].yLo, carry = adc(ry3Lo, pivotLo, 0)
	corners[3].yHi, _ = adc(ry3Hi, pivotHi, carry)

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

// initEdge sets up a DDA edge from Fixed88 corner (x0,y0) to (x1,y1).
// All coordinates are full 8.8 Fixed88: lo=fractional byte, hi=integer byte.
// Uses the ceil rule for yStart/yEnd to match the reference's half-open interval.
// Mirrors init_edge_common in render.s (updated for 16-bit Bresenham).
func initEdge(x0Lo, x0Hi, y0Lo, y0Hi, x1Lo, x1Hi, y1Lo, y1Hi byte) ddaEdge {
	var e ddaEdge

	y0Full := uint16(y0Hi)<<8 | uint16(y0Lo)
	y1Full := uint16(y1Hi)<<8 | uint16(y1Lo)

	// Sort top-to-bottom (smaller y = higher on screen).
	// 6502: compare y0/y1 as 16-bit; conditional swap of all 4 byte pairs.
	if y0Full > y1Full {
		x0Lo, x0Hi, y0Lo, y0Hi, x1Lo, x1Hi, y1Lo, y1Hi =
			x1Lo, x1Hi, y1Lo, y1Hi, x0Lo, x0Hi, y0Lo, y0Hi
		y0Full, y1Full = y1Full, y0Full
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

	// dy_full = y1_fixed − y0_fixed (16-bit, > 0).
	// 6502: SEC; LDA y1Lo; SBC y0Lo → dyLo (with borrow); LDA y1Hi; SBC y0Hi → dyHi
	dyFull := y1Full - y0Full
	e.dyLo = byte(dyFull)
	e.dyHi = byte(dyFull >> 8)

	// dx_full: 16-bit magnitude and sign.
	x0Full := uint16(x0Hi)<<8 | uint16(x0Lo)
	x1Full := uint16(x1Hi)<<8 | uint16(x1Lo)
	var dxFull uint16
	if x1Full >= x0Full {
		dxFull = x1Full - x0Full
		e.dxSign = 0
	} else {
		dxFull = x0Full - x1Full
		e.dxSign = 1
	}

	// Bresenham decomposition: quotient (8-bit, ≤48 for our geometry) + 16-bit remainder.
	// 6502: @divide_16x16 (8 iterations since quotient ≤ 255; quotient byte + 16-bit rem).
	e.baseStep = byte(dxFull / dyFull)
	remFull := dxFull % dyFull
	e.remLo = byte(remFull)
	e.remHi = byte(remFull >> 8)

	// Sub-pixel start correction: advance xCurrent and seed err to account for
	// the fractional y of the start corner (distance from y0_fixed to yStart*256).
	//   yDelta = yStart*256 − y0_fixed = 256 − y0Lo   (0 when y0Lo==0)
	// 6502: if y0Lo==0 skip; LDA #0; SEC; SBC y0Lo → yDelta (fits in byte: 1–255)
	var yDelta uint16
	if y0Lo > 0 {
		yDelta = 256 - uint16(y0Lo)
	}
	// xAdvanceFull = dx_full * yDelta / dy_full  (in Fixed88 units, ≤ dx_full ≤ 12288 → uint16 ok)
	// errInit      = dx_full * yDelta % dy_full   (16-bit Bresenham seed)
	// 6502: 16×8 multiply (dxFull × yDelta) → 24-bit; 24÷16 divide by dyFull
	xAdvanceFull := uint16(uint32(dxFull) * uint32(yDelta) / uint32(dyFull))
	errInit := uint16(uint32(dxFull) * uint32(yDelta) % uint32(dyFull))

	// xCurrent = (x0_fixed ± xAdvanceFull) >> 8  (integer pixel at yStart)
	// Round to nearest pixel (+128 before >>8) to match reference intersection rounding.
	if e.dxSign == 0 {
		e.xCurrent = byte((x0Full + xAdvanceFull + 128) >> 8)
	} else {
		e.xCurrent = byte((x0Full - xAdvanceFull + 128) >> 8)
	}
	e.errLo = byte(errInit)
	e.errHi = byte(errInit >> 8)
	return e
}

// stepEdge checks whether scanY intersects this edge and, if so, records the
// X position in spanX1 or spanX2 and advances the Bresenham accumulator.
// Mirrors step_edge_0_1 / step_edge_1_2 / ... in render.s.
func stepEdge(e *ddaEdge, scanY byte, ic *byte, spanX1, spanX2 *byte) {
	// Range check: [yStart, yEnd)
	// Note: (dyHi==0 && dyLo==0) check is redundant because initEdge ensures
	// yStart >= yEnd when dy==0, so the range check already covers inactive edges.
	if scanY < e.yStart || cmpGE(scanY, e.yEnd) {
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
	if e.dxSign == 0 {
		e.xCurrent, _ = adc(e.xCurrent, e.baseStep, 0) // clc / adc baseStep
	} else {
		e.xCurrent, _ = sbc(e.xCurrent, e.baseStep, 1) // sec / sbc baseStep
	}
	var carry byte
	e.errLo, carry = adc(e.errLo, e.remLo, 0)  // clc / adc remLo
	e.errHi, _ = adc(e.errHi, e.remHi, carry)  // adc remHi
	if cmp16GE(e.errLo, e.errHi, e.dyLo, e.dyHi) { // err >= dy_full?
		var c byte
		e.errLo, c = sbc(e.errLo, e.dyLo, 1) // sec / sbc dyLo
		e.errHi, _ = sbc(e.errHi, e.dyHi, c) // sbc dyHi
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

// renderSquare renders the rotated square for the given animation frame (0-20).
//
// charset layout: column-major, 64-byte stride.
//   charset[y]       = row y, column 0 bitmask
//   charset[64+y]    = row y, column 1 bitmask
//   charset[128+y]   = row y, column 2 bitmask
//
// Output resolution: 24×24 pixels (3×3 characters of 8×8 each).
// Supersampling: 48×48 intermediate space, 2×2 → 1 with threshold 0→off,1-2→med,3-4→full.
// "full" pixels go into charset; "medium" pixels are returned in mediumCharset.
// Mirrors render_square in render.s.
func renderSquare(frameNum byte) (charset [192]byte, mediumCharset [192]byte) {
	cycles += 10 // Function entry overhead: JSR (6) + stack setup (4)
	corners := calcRotation(frameNum)
	cycles += 20 // Table lookups: 4 reads × 5 cycles avg

	// Scale corners 2× for 48×48 supersample space (ASL lo / ROL hi per corner).
	var sc [4]corner
	for i, c := range corners {
		sc[i].xLo, sc[i].xHi = scale2x(c.xLo, c.xHi)
		sc[i].yLo, sc[i].yHi = scale2x(c.yLo, c.yHi)
	}

	// Initialize 4 DDA edges (corner 0→1, 1→2, 2→3, 3→0) using full Fixed88 coords.
	edges := [4]ddaEdge{
		initEdge(sc[0].xLo, sc[0].xHi, sc[0].yLo, sc[0].yHi, sc[1].xLo, sc[1].xHi, sc[1].yLo, sc[1].yHi),
		initEdge(sc[1].xLo, sc[1].xHi, sc[1].yLo, sc[1].yHi, sc[2].xLo, sc[2].xHi, sc[2].yLo, sc[2].yHi),
		initEdge(sc[2].xLo, sc[2].xHi, sc[2].yLo, sc[2].yHi, sc[3].xLo, sc[3].xHi, sc[3].yLo, sc[3].yHi),
		initEdge(sc[3].xLo, sc[3].xHi, sc[3].yLo, sc[3].yHi, sc[0].xLo, sc[0].xHi, sc[0].yLo, sc[0].yHi),
	}

	// Geometry optimization: Based on actual bounds from refRenderSquare:
	//   - Row 0 is always empty (globalMin = 1)
	//   - Rows 18-23 are always empty (globalMax = 17)
	// Clear these rows explicitly to maintain full 24-row overdraw,
	// then process only rows 1-17 that may contain pixels.

	// Set row 0 to 0xFF (all lit background, no square pixels).
	charset[0] = 0xFF
	charset[64] = 0xFF
	charset[128] = 0xFF
	mediumCharset[0] = 0xFF
	mediumCharset[64] = 0xFF
	mediumCharset[128] = 0xFF

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

		ssY, _ := asl(outY) // 2Y
		row0 := getSpan(ssY)
		ssY++               // 2Y+1
		row1 := getSpan(ssY)

		// Early exit: if both supersample rows are empty, write 0xFF (all lit) and skip pixel loop.
		// Saves ~150-200 cycles per empty row.
		// 6502: LDA row0.valid / ORA row1.valid / BEQ write_ones
		if !row0.valid && !row1.valid {
			charset[outY] = 0xFF
			charset[64+outY] = 0xFF
			charset[128+outY] = 0xFF
			mediumCharset[outY] = 0xFF
			mediumCharset[64+outY] = 0xFF
			mediumCharset[128+outY] = 0xFF
			continue
		}

		// Downsample: for each output column count subpixels then threshold.
		// Initialize to 0xFF (all pixels lit by default, we'll clear bits where square is).
		var col0Full, col1Full, col2Full byte = 0xFF, 0xFF, 0xFF     // full-brightness charset
		var col0Med, col1Med, col2Med byte = 0xFF, 0xFF, 0xFF        // medium-brightness (sprite/overlay)

		// Span-based filling with overdraw: compute bounds, only check pixels in range.
		// Pixels outside span stay lit (0xFF), guaranteeing complete overdraw.
		var outXMin, outXMax byte = 24, 0 // Start with empty range
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
			// Convert to output pixel range: x/2
			outXMin = ssXMin >> 1
			outXMax = (ssXMax + 1) >> 1
			if outXMax > 24 {
				outXMax = 24
			}
			// Handle wrapping: if any span extends past 48, include left edge
			if (row0.valid && cmpGE(row0.x2, 48)) || (row1.valid && cmpGE(row1.x2, 48)) {
				outXMin = 0 // Process from start to handle wrapped pixels
			}
		}

		// Helper: check if supersample coordinate sx is inside span s
		inSpan := func(sx byte, s span) bool {
			if !s.valid {
				return false
			}
			// Normal range [x1, x2)
			if cmpGE(sx, s.x1) && !cmpGE(sx, s.x2) {
				return true
			}
			// Horizontal wrapping: if span extends past 48, check wrapped position
			if cmpGE(s.x2, 48) {
				sxWrap, _ := adc(sx, 48, 0)
				if cmpGE(sxWrap, s.x1) && !cmpGE(sxWrap, s.x2) {
					return true
				}
			}
			return false
		}

		// Iterate only over pixels within span bounds; others stay 0 (cleared).
		for outX := outXMin; outX < outXMax; outX++ {
			ss2x, _ := asl(outX)        // 2X in supersample space
			ss2x1, _ := adc(ss2x, 1, 0) // 2X+1

			// Count how many of the 4 subpixels are inside.
			// Early exit at count==3: since 3 and 4 both map to full brightness,
			// no need to check the 4th subpixel once we have 3.
			// Saves ~100-115 inSpan calls per frame (~1200-1400 cycles, 7-8% faster).
			var count byte
			if inSpan(ss2x, row0) {
				count++
			}
			if inSpan(ss2x1, row0) {
				count++
			}
			if count < 3 && inSpan(ss2x, row1) {
				count++
			}
			if count < 3 && inSpan(ss2x1, row1) {
				count++
			}

			// Only clear bits if count > 0 (pixel inside square); otherwise stays lit (1 bit).
			if count > 0 {
				// Get bitmask for this pixel's position within its character column.
				bitPos := outX & 7      // X & 7
				mask := bitMask[bitPos] // lookup

				if cmpGE(count, 3) { // 3 or 4 subpixels: full darkness (clear full bit)
					if !cmpGE(outX, 16) {
						if !cmpGE(outX, 8) {
							col0Full &= ^mask // and col0Full with inverted mask
						} else {
							col1Full &= ^mask
						}
					} else {
						col2Full &= ^mask
					}
				} else { // 1 or 2 subpixels: medium darkness (clear medium bit)
					if !cmpGE(outX, 16) {
						if !cmpGE(outX, 8) {
							col0Med &= ^mask
						} else {
							col1Med &= ^mask
						}
					} else {
						col2Med &= ^mask
					}
				}
			}
		}

		// Write scanline bitmasks to charset arrays.
		charset[outY] = col0Full
		charset[64+outY] = col1Full
		charset[128+outY] = col2Full
		mediumCharset[outY] = col0Med
		mediumCharset[64+outY] = col1Med
		mediumCharset[128+outY] = col2Med
	}

	// Set rows 18-23 to 0xFF (all lit background, no square pixels).
	for outY = 18; outY < 24; outY++ {
		charset[outY] = 0xFF
		charset[64+outY] = 0xFF
		charset[128+outY] = 0xFF
		mediumCharset[outY] = 0xFF
		mediumCharset[64+outY] = 0xFF
		mediumCharset[128+outY] = 0xFF
	}

	return
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
			// Square is a "hole" in the $0 foreground:
			// full=0, med=1 → inside square, high coverage → $f (background shows through)
			// full=1, med=0 → inside square, low coverage → $b (sprite layer)
			// full=1, med=1 → outside square → $0 (foreground)
			switch {
			case !fullBit && medBit:
				c = bgColor // $f light grey (background through hole)
			case fullBit && !medBit:
				c = medColor // $b dark grey (sprite layer)
			case fullBit && medBit:
				c = fullColor // $0 black (foreground)
			default:
				continue // shouldn't happen (full=0, med=0)
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
	initTables()

	// C64 palette colors
	black := color.RGBA{0x00, 0x00, 0x00, 0xFF}      // $0: black (full brightness square pixels)
	darkGrey := color.RGBA{0x5F, 0x5F, 0x5F, 0xFF}   // $b: dark grey (sprite/medium pixels)
	lightGrey := color.RGBA{0xC6, 0xC6, 0xC6, 0xFF}  // $f: light grey (background)

	if len(os.Args) >= 2 && os.Args[1] == "check" {
		runChecks()
		return
	}

	if len(os.Args) >= 2 && os.Args[1] == "compare" {
		runCompare()
		return
	}

	if len(os.Args) >= 2 && os.Args[1] == "trace" {
		traceMain()
		return
	}

	if len(os.Args) >= 2 && os.Args[1] == "cycles" {
		frameNum := byte(0)
		if len(os.Args) >= 3 {
			var frame int
			fmt.Sscanf(os.Args[2], "%d", &frame)
			if frame < 0 || frame > 20 {
				fmt.Fprintf(os.Stderr, "Frame number must be 0-20\n")
				os.Exit(1)
			}
			frameNum = byte(frame)
		}
		resetCycles()
		renderSquare(frameNum)
		c := getCycles()
		degs := [21]float64{0, 4.5, 9, 13.5, 18, 22.5, 27, 31.5, 36, 40.5, 45, 49.5, 54, 58.5, 63, 67.5, 72, 76.5, 81, 85.5, 90}
		fmt.Printf("Rendered frame %d (%.1f°): %d cycles\n", frameNum, degs[frameNum], c)
		fmt.Printf("At 1 MHz: %.2f ms per frame\n", float64(c)/1000.0)
		fmt.Printf("Max FPS at 1 MHz: %.1f\n", 1_000_000.0/float64(c))
		return
	}

	if len(os.Args) >= 2 && os.Args[1] == "stats" {
		// Compute mean cycle count and pixel error across all 21 animation frames.
		cycleList := make([]int64, 21)
		diffList := make([]int, 21)

		for i := 0; i < 21; i++ {
			resetCycles()
			protFull, _ := renderSquare(byte(i))
			refFull, _ := refRenderSquare(byte(i))
			cycleList[i] = getCycles()
			diffList[i] = diffCharsets(protFull, refFull)
		}

		// Compute median and mean
		median := func(vals []int64) (int64, float64) {
			sorted := make([]int64, len(vals))
			copy(sorted, vals)
			for i := 0; i < len(sorted); i++ {
				for j := i + 1; j < len(sorted); j++ {
					if sorted[i] > sorted[j] {
						sorted[i], sorted[j] = sorted[j], sorted[i]
					}
				}
			}
			med := sorted[len(sorted)/2]
			var sum int64
			for _, v := range vals {
				sum += v
			}
			mean := float64(sum) / float64(len(vals))
			return med, mean
		}

		medianDiff := func(vals []int) (int, float64) {
			sorted := make([]int, len(vals))
			copy(sorted, vals)
			for i := 0; i < len(sorted); i++ {
				for j := i + 1; j < len(sorted); j++ {
					if sorted[i] > sorted[j] {
						sorted[i], sorted[j] = sorted[j], sorted[i]
					}
				}
			}
			med := sorted[len(sorted)/2]
			var sum int
			for _, v := range vals {
				sum += v
			}
			mean := float64(sum) / float64(len(vals))
			return med, mean
		}

		_, cycMean := median(cycleList)
		_, diffMean := medianDiff(diffList)

		fps := 1_000_000.0 / cycMean
		errorRate := diffMean * 100.0 / (24.0 * 24.0)

		fmt.Printf("Animation performance (21 reference angles, 4.5° steps):\n")
		fmt.Printf("  FPS @ 1 MHz: %.1f (%.0f cycles/frame avg)\n", fps, cycMean)
		fmt.Printf("  Error rate:  %.2f%% (%.1f pixels/frame avg)\n", errorRate, diffMean)
		return
	}

	if len(os.Args) >= 2 && os.Args[1] == "frames" {
		if err := os.MkdirAll("frames", 0755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		// Generate the 21 animation frames (0°, 4.5°, 9°, ...90° in 4.5° steps).
		degs := [21]float64{0, 4.5, 9, 13.5, 18, 22.5, 27, 31.5, 36, 40.5, 45, 49.5, 54, 58.5, 63, 67.5, 72, 76.5, 81, 85.5, 90}

		for i := 0; i < 21; i++ {
			full, med := renderSquare(byte(i))
			img := charsetToImage(full, med, 8, black, darkGrey, lightGrey)
			// Format degrees: use %04.1f to get "004.5", "009.0", etc.
			path := fmt.Sprintf("frames/frame_%02d_%05.1fdeg.png", i, degs[i])
			f, err := os.Create(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			png.Encode(f, img)
			f.Close()
		}
		fmt.Printf("Wrote 21 frames to frames/\n")
		return
	}

	// Default mode: render all 21 frames as sprite sheet + print stats.
	// Compute stats
	cycleList := make([]int64, 21)
	diffList := make([]int, 21)
	var frames [21][192]byte
	var mediums [21][192]byte
	var refs [21][192]byte
	var refMeds [21][192]byte

	for i := 0; i < 21; i++ {
		resetCycles()
		protFull, protMed := renderSquare(byte(i))
		refFull, refMed := refRenderSquare(byte(i))
		frames[i] = protFull
		mediums[i] = protMed
		refs[i] = refFull
		refMeds[i] = refMed
		cycleList[i] = getCycles()
		diffList[i] = diffCharsets(protFull, refFull)
	}

	// Compute mean cycles and diffs
	var sumCycles int64
	var sumDiffs int
	for i := 0; i < 21; i++ {
		sumCycles += cycleList[i]
		sumDiffs += diffList[i]
	}
	cycMean := float64(sumCycles) / 21.0
	diffMean := float64(sumDiffs) / 21.0
	fps := 1_000_000.0 / cycMean
	errorRate := diffMean * 100.0 / (24.0 * 24.0)

	// Create sprite sheet: 7 columns × 3 rows with 1px separators
	pixelSize := 8
	frameSize := 24 * pixelSize
	cols := 7
	rows := 3
	sep := 1 // separator width
	sheetWidth := cols*frameSize + (cols-1)*sep
	sheetHeight := rows*frameSize + (rows-1)*sep
	sheet := image.NewRGBA(image.Rect(0, 0, sheetWidth, sheetHeight))

	// Fill background (white separators)
	separator := color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	for y := 0; y < sheetHeight; y++ {
		for x := 0; x < sheetWidth; x++ {
			sheet.Set(x, y, separator)
		}
	}

	// Helper: count subpixel coverage (0-4) for a pixel from charset bytes.
	getCount := func(full, med [192]byte, outX, outY int) int {
		col := outX / 8
		bit := 7 - (outX % 8)
		fullBit := (full[col*64+outY]>>bit)&1 != 0
		medBit := (med[col*64+outY]>>bit)&1 != 0
		// full=0, med=1 → 3-4 subpixels inside square
		// full=1, med=0 → 1-2 subpixels inside square
		// full=1, med=1 → 0 subpixels (outside square)
		if !fullBit && medBit {
			return 3 // high coverage
		} else if fullBit && !medBit {
			return 1 // low coverage
		}
		return 0 // outside square
	}

	red := color.RGBA{0xFF, 0x00, 0x00, 0xFF}

	// Draw each frame
	for i := 0; i < 21; i++ {
		col := i % cols
		row := i / cols
		frameImg := charsetToImage(frames[i], mediums[i], pixelSize, black, darkGrey, lightGrey)

		// Overlay red dots where proto differs from reference
		for outY := 0; outY < 24; outY++ {
			for outX := 0; outX < 24; outX++ {
				protCount := getCount(frames[i], mediums[i], outX, outY)
				refCount := getCount(refs[i], refMeds[i], outX, outY)
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
							frameImg.Set(centerX+dx, centerY+dy, red)
						}
					}
				}
			}
		}

		// Copy frame into sheet with separator offset
		offsetX := col * (frameSize + sep)
		offsetY := row * (frameSize + sep)
		for y := 0; y < frameSize; y++ {
			for x := 0; x < frameSize; x++ {
				sheet.Set(offsetX+x, offsetY+y, frameImg.At(x, y))
			}
		}
	}

	// Write sprite sheet
	path := "animation.png"
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	png.Encode(f, sheet)
	f.Close()

	// Print stats
	fmt.Printf("Animation sprite sheet: %s (21 frames, 7×3 grid)\n", path)
	fmt.Printf("Performance @ 1 MHz:\n")
	fmt.Printf("  FPS: %.1f (%.0f cycles/frame avg)\n", fps, cycMean)
	fmt.Printf("  Error: %.2f%% (%.1f pixels/frame avg)\n", errorRate, diffMean)
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

	// Scale 2× for 48×48 supersample space.
	var sxs, sys [4]int32
	for i := range xs {
		sxs[i] = xs[i] << 1
		sys[i] = ys[i] << 1
	}

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
			if count >= 3 {
				if outX < 8 {
					col0F |= mask
				} else if outX < 16 {
					col1F |= mask
				} else {
					col2F |= mask
				}
			} else {
				if outX < 8 {
					col0M |= mask
				} else if outX < 16 {
					col1M |= mask
				} else {
					col2M |= mask
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
		protFull, _ := renderSquare(byte(i))
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
			protFull, protMed := renderSquare(byte(i))
			refFull, refMed := refRenderSquare(byte(i))

			// C64 palette colors
			black := color.RGBA{0x00, 0x00, 0x00, 0xFF}      // $0: black (full brightness square pixels)
			darkGrey := color.RGBA{0x5F, 0x5F, 0x5F, 0xFF}   // $b: dark grey (sprite/medium pixels)
			lightGrey := color.RGBA{0xC6, 0xC6, 0xC6, 0xFF}  // $f: light grey (background)
			red := color.RGBA{0xFF, 0x00, 0x00, 0xFF}

			// Diff image: proto output with diff pixels marked by red dots sized by error magnitude.
			diffImg := charsetToImage(protFull, protMed, 8, black, darkGrey, lightGrey)

			// Helper: count subpixel coverage (0-4) for a pixel from charset bytes.
			getCount := func(full, med [192]byte, outX, outY int) int {
				col := outX / 8
				bit := 7 - (outX % 8)
				fullBit := (full[col*64+outY]>>bit)&1 != 0
				medBit := (med[col*64+outY]>>bit)&1 != 0
				// full=0, med=1 → 3-4 subpixels inside square
				// full=1, med=0 → 1-2 subpixels inside square
				// full=1, med=1 → 0 subpixels (outside square)
				if !fullBit && medBit {
					return 3 // high coverage
				} else if fullBit && !medBit {
					return 1 // low coverage
				}
				return 0 // outside square
			}

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
	full, _ := renderSquare(frameNum)
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
	allPass = assertAngle("frame 0 (0°)", 0, func(row int) (byte, byte, byte) {
		if row >= 6 && row <= 17 {
			return 0x03, 0xFF, 0xC0
		}
		return 0, 0, 0
	}) && allPass

	// frame 10 (45°): diamond, pixel-perfect due to symmetry.
	// Geometric reference detects wrapped pixels at edges.
	allPass = assertAngle("frame 10 (45°)", 10, func(row int) (byte, byte, byte) {
		switch row {
		case 2, 17:
			return 0x00, 0x00, 0x60
		case 3, 16:
			return 0x00, 0x00, 0xF0
		case 4, 15:
			return 0x00, 0x01, 0xF8
		case 5, 14:
			return 0x00, 0x03, 0xFC
		case 6, 13:
			return 0x00, 0x07, 0xFE
		case 7, 12:
			return 0x00, 0x0F, 0xFF
		case 8, 11:
			return 0x80, 0x1F, 0xFF // col0 wrapped pixels
		case 9, 10:
			return 0xC0, 0x3F, 0xFF // col0 wrapped pixels
		}
		return 0, 0, 0
	}) && allPass

	// frame 20 (90°): 12×12 square rotated 90° with horizontal wrapping.
	allPass = assertAngle("frame 20 (90°)", 20, func(row int) (byte, byte, byte) {
		if row >= 6 && row <= 17 {
			return 0xFC, 0x00, 0x3F // col0 shows wrapped pixels
		}
		return 0, 0, 0
	}) && allPass

	if allPass {
		fmt.Println("All checks PASSED")
	} else {
		fmt.Fprintln(os.Stderr, "FAILED")
		os.Exit(1)
	}
}
