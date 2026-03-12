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
	"strconv"
	"strings"
)

// ─── Sin/cos tables ──────────────────────────────────────────────────────────
// Each entry: sin(angle*2π/256)*256 rounded, stored as [lo, hi] Fixed88.
// Indexed by angle byte 0-255.

var sineTable [256][2]byte
var cosineTable [256][2]byte

func initTables() {
	for i := 0; i < 256; i++ {
		sv := int16(math.Round(math.Sin(float64(i)*2*math.Pi/256) * 256))
		cv := int16(math.Round(math.Cos(float64(i)*2*math.Pi/256) * 256))
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

// calcRotation computes the 4 rotated corners of the square for the given angle.
// Mirrors calc_rotation in rotation.s.
//
// Square corners relative to pivot (18,18) in Fixed88:
//   0: (-12, -12)  →  lo=$00, hi=$F4
//   1: (  0, -12)  →  lo=$00, hi=$00 / lo=$00, hi=$F4
//   2: (  0,   0)  →  always maps to pivot (18,18)
//   3: (-12,   0)  →  lo=$00, hi=$F4 / lo=$00, hi=$00

func calcRotation(angle byte) [4]corner {
	sinLo, sinHi := sineTable[angle][0], sineTable[angle][1]
	cosLo, cosHi := cosineTable[angle][0], cosineTable[angle][1]

	// Pivot (18,18) in Fixed88: hi=$12, lo=$00
	const pivotLo, pivotHi byte = 0x00, 0x12

	// [xLo, xHi, yLo, yHi] relative to pivot
	rel := [4][4]byte{
		{0x00, 0xF4, 0x00, 0xF4}, // corner 0: (-12, -12)
		{0x00, 0x00, 0x00, 0xF4}, // corner 1: (  0, -12)
		{0x00, 0x00, 0x00, 0x00}, // corner 2: (  0,   0)
		{0x00, 0xF4, 0x00, 0x00}, // corner 3: (-12,   0)
	}

	var corners [4]corner
	for i, r := range rel {
		rxLo, rxHi, ryLo, ryHi := rotatePoint(r[0], r[1], r[2], r[3], cosLo, cosHi, sinLo, sinHi)
		// Add pivot: (rotated + pivot)
		var carry byte
		corners[i].xLo, carry = adc(rxLo, pivotLo, 0)
		corners[i].xHi, _ = adc(rxHi, pivotHi, carry)
		corners[i].yLo, carry = adc(ryLo, pivotLo, 0)
		corners[i].yHi, _ = adc(ryHi, pivotHi, carry)
	}
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
	if scanY < e.yStart || cmpGE(scanY, e.yEnd) || (e.dyHi == 0 && e.dyLo == 0) {
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

// renderSquare renders the rotated square for the given angle into charset.
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
func renderSquare(angle byte) (charset [192]byte, mediumCharset [192]byte) {
	corners := calcRotation(angle)

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

	// Process each output row (0-23).
	var outY byte
	for outY = 0; outY < 24; outY++ {
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

		// Downsample: for each output column count subpixels then threshold.
		var col0Full, col1Full, col2Full byte     // full-brightness charset
		var col0Med, col1Med, col2Med byte        // medium-brightness (sprite/overlay)

		var outX byte
		for outX = 0; outX < 24; outX++ {
			ss2x, _ := asl(outX) // 2X in supersample space

			// Count how many of the 4 subpixels (2X,row0),(2X+1,row0),(2X,row1),(2X+1,row1) are inside.
			var count byte
			checkPixel := func(sx byte, s span) {
				if !s.valid {
					return
				}
				// Check normal range [x1, x2)
				if cmpGE(sx, s.x1) && !cmpGE(sx, s.x2) {
					count++
					return
				}
				// Handle horizontal wrapping: if span extends past right edge (x2 >= 48),
				// check if sx+48 falls in [x1, x2), meaning sx wraps from left edge.
				// 6502: CLC / ADC #48 / CMP x1 / BCC skip / CMP x2 / BCS skip / INC count
				if cmpGE(s.x2, 48) {
					sxWrap, _ := adc(sx, 48, 0)
					if cmpGE(sxWrap, s.x1) && !cmpGE(sxWrap, s.x2) {
						count++
					}
				}
			}
			ss2x1, _ := adc(ss2x, 1, 0) // 2X+1
			checkPixel(ss2x, row0)
			checkPixel(ss2x1, row0)
			checkPixel(ss2x, row1)
			checkPixel(ss2x1, row1)

			if count == 0 {
				outX++ // manually undo the increment so the loop does it
				outX--
				continue
			}

			// Get bitmask for this pixel's position within its character column.
			bitPos := outX & 7           // X & 7
			mask := bitMask[bitPos]      // lookup

			if cmpGE(count, 3) { // 3 or 4 subpixels: full brightness
				if !cmpGE(outX, 16) {
					if !cmpGE(outX, 8) {
						col0Full |= mask // ora col0Full
					} else {
						col1Full |= mask
					}
				} else {
					col2Full |= mask
				}
			} else { // 1 or 2 subpixels: medium brightness
				if !cmpGE(outX, 16) {
					if !cmpGE(outX, 8) {
						col0Med |= mask
					} else {
						col1Med |= mask
					}
				} else {
					col2Med |= mask
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
			fullSet := (full[col*64+py]>>bit)&1 != 0
			medSet := (medium[col*64+py]>>bit)&1 != 0
			var c color.Color
			switch {
			case fullSet:
				c = fullColor
			case medSet:
				c = medColor
			default:
				continue
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

	yellow := color.RGBA{0xFF, 0xFF, 0x00, 0xFF}
	blue := color.RGBA{0x00, 0x00, 0xAA, 0xFF}
	grey := color.RGBA{0x77, 0x77, 0x77, 0xFF}

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

	if len(os.Args) >= 2 && os.Args[1] == "frames" {
		if err := os.MkdirAll("frames", 0755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		// Generate the 11 reference angles (0°,9°,18°...90°).
		angles := [11]byte{0, 6, 13, 19, 26, 32, 38, 45, 51, 58, 64}
		degs := [11]int{0, 9, 18, 27, 36, 45, 54, 63, 72, 81, 90}

		for i, ab := range angles {
			full, med := renderSquare(ab)
			img := charsetToImage(full, med, 8, blue, grey, yellow)
			path := fmt.Sprintf("frames/frame_%02d_%03ddeg.png", i, degs[i])
			f, err := os.Create(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			png.Encode(f, img)
			f.Close()
		}
		fmt.Printf("Wrote 11 frames to frames/\n")
		return
	}

	// Single angle mode (default or explicit angle arg).
	angleDeg := 0
	if len(os.Args) >= 2 {
		fmt.Sscanf(os.Args[1], "%d", &angleDeg)
	}
	// Map degrees to angle byte (0-255 = full rotation).
	angle := byte(angleDeg * 256 / 360)
	full, med := renderSquare(angle)

	img := charsetToImage(full, med, 8, blue, grey, yellow)
	path := fmt.Sprintf("frame_%03d.png", angleDeg)
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	png.Encode(f, img)
	f.Close()
	fmt.Printf("Angle %d° → angle byte %d → %s\n", angleDeg, angle, path)

	// Also print charset bytes for comparison with emulator assertions.
	fmt.Printf("Charset (col 0, rows 0-7): ")
	for y := 0; y < 8; y++ {
		fmt.Printf("%02X ", full[y])
	}
	fmt.Println()
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
func cornersAsFixed88(angle byte) (xs, ys [4]int32) {
	c := calcRotation(angle)
	for i, cn := range c {
		xs[i] = int32(int16(uint16(cn.xLo) | uint16(cn.xHi)<<8))
		ys[i] = int32(int16(uint16(cn.yLo) | uint16(cn.yHi)<<8))
	}
	return
}

func refRenderSquare(angle byte) (charset [192]byte, mediumCharset [192]byte) {
	xs, ys := cornersAsFixed88(angle)

	// Scale 2× for 48×48 supersample space.
	var sxs, sys [4]int32
	for i := range xs {
		sxs[i] = xs[i] << 1
		sys[i] = ys[i] << 1
	}

	// Compute span for one supersample scanline using direct intersection.
	// Mirrors renderQuadHighRes from tools/generate_comparison_with_reference.go.
	spanForRow := func(y int) (x1, x2 int, ok bool) {
		yFixed := int32(y << 8) // scanline y as Fixed88
		var ints [2]int
		n := 0
		for i := 0; i < 4 && n < 2; i++ {
			p1y, p2y := sys[i], sys[(i+1)%4]
			p1x, p2x := sxs[i], sxs[(i+1)%4]
			if !((p1y <= yFixed && p2y > yFixed) || (p2y <= yFixed && p1y > yFixed)) {
				continue
			}
			dy := p2y - p1y
			if dy == 0 {
				continue
			}
			t := (yFixed - p1y) * 256 / dy
			x := p1x + (p2x-p1x)*t/256
			ints[n] = int(x)
			n++
		}
		if n < 2 {
			return 0, 0, false
		}
		a, b := ints[0], ints[1]
		if a > b {
			a, b = b, a
		}
		// Convert Fixed88 → pixel using rounding (+128 >> 8), same as reference.
		x1 = (a + 128) >> 8
		x2 = (b + 128) >> 8
		ok = true
		return
	}

	for outY := 0; outY < 24; outY++ {
		var col0F, col1F, col2F byte
		var col0M, col1M, col2M byte

		ssY := outY * 2
		r0x1, r0x2, r0ok := spanForRow(ssY)
		r1x1, r1x2, r1ok := spanForRow(ssY + 1)

		for outX := 0; outX < 24; outX++ {
			sx := outX * 2
			var count byte
			// Check pixel with horizontal wrapping support
			checkRef := func(px int, x1, x2 int, valid bool) {
				if !valid {
					return
				}
				// Normal range check
				if px >= x1 && px < x2 {
					count++
					return
				}
				// Wrapping: if span extends past right edge, check wrapped position
				if x2 >= 48 {
					pxWrap := px + 48
					if pxWrap >= x1 && pxWrap < x2 {
						count++
					}
				}
			}
			checkRef(sx, r0x1, r0x2, r0ok)
			checkRef(sx+1, r0x1, r0x2, r0ok)
			checkRef(sx, r1x1, r1x2, r1ok)
			checkRef(sx+1, r1x1, r1x2, r1ok)
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

	// Angle bytes closest to 0°,9°,18°,27°,36°,45°,54°,63°,72°,81°,90°.
	// (N * 256 / 360, truncated to nearest integer.)
	angles := [11]byte{0, 6, 13, 19, 26, 32, 38, 45, 51, 58, 64}
	degs := [11]int{0, 9, 18, 27, 36, 45, 54, 63, 72, 81, 90}

	total := 0
	fmt.Printf("%-6s %-8s  %-6s  notes\n", "angle", "byte", "diffs")
	fmt.Println(strings.Repeat("-", 50))
	for i, ab := range angles {
		protFull, _ := renderSquare(ab)
		refFull, _ := refRenderSquare(ab)
		d := diffCharsets(protFull, refFull)
		total += d
		note := ""
		if d == 0 {
			note = "exact match"
		}
		fmt.Printf("%3d°   byte %-3d  %4d px  %s\n", degs[i], ab, d, note)
	}
	fmt.Printf("%-6s %-8s  %-6d\n", "total", "", total)

	if err := os.MkdirAll("frames", 0755); err == nil {
		for i, ab := range angles {
			protFull, protMed := renderSquare(ab)
			refFull, refMed := refRenderSquare(ab)

			yellow := color.RGBA{0xFF, 0xFF, 0x00, 0xFF}
			blue := color.RGBA{0x00, 0x00, 0xAA, 0xFF}
			grey := color.RGBA{0x77, 0x77, 0x77, 0xFF}
			red := color.RGBA{0xFF, 0x00, 0x00, 0xFF}

			// Diff image: proto output with diff pixels highlighted by small red dots.
			diffImg := charsetToImage(protFull, protMed, 8, blue, grey, yellow)
			// Overlay small red dots where proto differs from reference (full layer).
			for row := 0; row < 24; row++ {
				for col := 0; col < 3; col++ {
					xorByte := protFull[col*64+row] ^ refFull[col*64+row]
					for bit := 0; bit < 8; bit++ {
						if (xorByte>>(7-bit))&1 != 0 {
							px := col*8 + bit
							// Draw a small 3×3 red dot in the center of the 8×8 pixel block.
							for dy := 3; dy < 6; dy++ {
								for dx := 3; dx < 6; dx++ {
									diffImg.Set(px*8+dx, row*8+dy, red)
								}
							}
						}
					}
				}
			}
			f, _ := os.Create(fmt.Sprintf("frames/compare_%02d_%03ddeg.png", i, degs[i]))
			png.Encode(f, diffImg)
			f.Close()

			// Also save the reference frame for side-by-side inspection.
			refImg := charsetToImage(refFull, refMed, 8, blue, grey, yellow)
			f, _ = os.Create(fmt.Sprintf("frames/ref_%02d_%03ddeg.png", i, degs[i]))
			png.Encode(f, refImg)
			f.Close()
		}
		fmt.Printf("\nWrote proto+diff and reference frames to frames/\n")
	}
}

// ─── Self-checks against emulator pixel-perfect assertions ───────────────────

func assertAngle(label string, angleByte byte, want func(row int) (col0, col1, col2 byte)) bool {
	full, _ := renderSquare(angleByte)
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
	// Intermediate angles have ~35 total diffs (<0.5% error) due to 8-bit
	// Bresenham approximation vs. reference's per-scanline exact intersection.

	// angle=0: 12×12 square at (6,6)-(17,17), no rotation.
	allPass = assertAngle("angle=0", 0, func(row int) (byte, byte, byte) {
		if row >= 6 && row <= 17 {
			return 0x03, 0xFF, 0xC0
		}
		return 0, 0, 0
	}) && allPass

	// angle=32 (45°): diamond, pixel-perfect due to symmetry.
	allPass = assertAngle("angle=32 (45°)", 32, func(row int) (byte, byte, byte) {
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
			return 0x00, 0x1F, 0xFF
		case 9, 10:
			return 0x00, 0x3F, 0xFF
		}
		return 0, 0, 0
	}) && allPass

	// angle=64 (90°): 12×12 square rotated 90°.
	allPass = assertAngle("angle=64 (90°)", 64, func(row int) (byte, byte, byte) {
		if row >= 6 && row <= 17 {
			return 0x00, 0x00, 0x3F
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
