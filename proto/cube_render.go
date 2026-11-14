package main

import (
	"cubelon64-proto/cpu8"
	"math"
)

const cubeSuperSize = 2 * morphTileSize

type cubeSpan struct {
	valid  bool
	x1, x2 byte
}

type cubeCounts [morphTileSize][morphTileSize]int

func cubeFixedByte(value float64) (low, high byte) {
	fixed := uint16(math.Round(value * 256))
	return byte(fixed), byte(fixed >> 8)
}

// A face as the quad renderer takes it: its corners in 8.8 fixed point on the 48x48 subpixel
// grid. The renderer samples a subpixel row at its top edge where the model samples the
// centre, so the rows move up by half a subpixel.
func cubeFaceCornersFixed(corners [4][2]float64) [4]corner {
	var fixed [4]corner
	for i, c := range corners {
		fixed[i].xLo, fixed[i].xHi = cubeFixedByte(2 * c[0])
		fixed[i].yLo, fixed[i].yHi = cubeFixedByte(2*c[1] - 0.5)
	}
	return fixed
}

// The subpixel columns each subpixel row of the quad covers, from the DDA edges of the
// quad renderer
func cubeQuadSpans(corners [4]corner) [cubeSuperSize]cubeSpan {
	var edges [4]ddaEdge
	for i := range edges {
		from, to := corners[i], corners[(i+1)%len(corners)]
		edges[i] = cubeInitEdge(from, to)
	}
	var spans [cubeSuperSize]cubeSpan
	for row := range spans {
		var found byte
		var x1, x2 byte
		for i := range edges {
			stepEdge(&edges[i], byte(row), &found, &x1, &x2)
			if found >= 2 {
				break
			}
		}
		if found < 2 {
			continue
		}
		if x1 > x2 {
			x1, x2 = x2, x1
		}
		spans[row] = cubeSpan{valid: true, x1: x1, x2: x2}
	}
	return spans
}

// The subpixels a face draws: the white face all of them, the checker face (50%) only those
// on odd lines, the dots face (25%) only every second line, there one column in four, shifted
// by two columns on each drawn line. That leaves one subpixel on every second pixel, a
// checkerboard of dark grey and black pixels.
func cubeSubpixelDrawn(shade, column, row int) bool {
	switch shade {
	case cubeCheckerShade:
		return row%2 == 1
	case cubeDotsShade:
		return row%2 == 0 && column%4 == 2*(row/2%2)
	}
	return true
}

// A drawn line covers the union of the spans of the lines around it
func (counts *cubeCounts) addFace(spans *[cubeSuperSize]cubeSpan, shade int) {
	overdraw := cubeOverdrawRows[shade]
	for row := range spans {
		first, last, found := cubeSuperSize, -1, false
		for near := max(0, row-overdraw); near <= min(cubeSuperSize-1, row+overdraw); near++ {
			if spans[near].valid {
				found = true
				first, last = min(first, int(spans[near].x1)), max(last, int(spans[near].x2))
			}
		}
		if !found {
			continue
		}
		for column := first; column < min(cubeSuperSize, last); column++ {
			if cubeSubpixelDrawn(shade, column, row) {
				counts[row/2][column/2]++
			}
		}
	}
}

// How many subpixel lines above and below a face each of its drawn lines reaches. Only the
// checker face needs it: it draws odd lines only, so where it meets the dots face, which draws
// even lines only, a pixel could be left with neither drawn. Overdrawing one line fills them,
// without a per-pixel correction that quads cannot do. More overdraw or any in columns
// darkens the edges and scores worse.
var cubeOverdrawRows = [cubeShades]int{cubeCheckerShade: 1}

// What the quad renderer draws for the visible faces: every face on its own, the subpixels
// its pattern draws counted per pixel (3-4 give light grey, 1-2 dark grey, like every quad the
// renderer draws), and the faces combined by taking the lightest tone, like ORing their masks
func (camera cubeCamera) renderLevels(rotation cubeMatrix) cubeLevels {
	var levels cubeLevels
	for _, face := range camera.visibleFaces(rotation) {
		var counts cubeCounts
		spans := cubeQuadSpans(cubeFaceCornersFixed(face.corners))
		counts.addFace(&spans, face.shade)
		for y := range levels {
			for x := range levels[y] {
				levels[y][x] = max(levels[y][x], morphCoverageLevel(counts[y][x]))
			}
		}
	}
	return levels
}

// initEdge for sampling subpixel columns at their centres: x is exactly the rounded edge
// position on every row. The Bresenham error starts as the fraction of the rounded start
// position (for edges going left, its complement) times the edge's dy, one mul16x8 per edge,
// where initEdge seeds only a rounding remainder and so lets x drift by up to a subpixel.
func cubeInitEdge(from, to corner) ddaEdge {
	if cubeFixed(to.yLo, to.yHi) < cubeFixed(from.yLo, from.yHi) {
		from, to = to, from
	}
	var e ddaEdge
	y0, y1 := cubeFixed(from.yLo, from.yHi), cubeFixed(to.yLo, to.yHi)
	e.yStart, e.yEnd = from.yHi, to.yHi
	if from.yLo > 0 {
		e.yStart++
	}
	if to.yLo > 0 {
		e.yEnd++
	}
	if e.yStart >= e.yEnd {
		return e
	}
	x0, x1 := cubeFixed(from.xLo, from.xHi), cubeFixed(to.xLo, to.xHi)
	dxLo, dxHi := byte(x1-x0), byte((x1-x0)>>8)
	if x1 < x0 {
		dxLo, dxHi = byte(x0-x1), byte((x0-x1)>>8)
		e.dxSign = 1
	}
	e.dyLo, e.dyHi = byte(y1-y0), byte((y1-y0)>>8)
	e.baseStep, e.remLo, e.remHi = cpu8.Div16x16(dxLo, dxHi, e.dyLo, e.dyHi)

	var yDelta byte
	if from.yLo > 0 {
		yDelta, _ = cpu8.Sbc(0, from.yLo, 1)
	}
	advance := uint16(0)
	if yDelta > 0 {
		prod0, prod1, prod2 := cpu8.Mul16x8(dxLo, dxHi, yDelta)
		advLo, advHi, _, _ := cpu8.Div24x16(prod0, prod1, prod2, e.dyLo, e.dyHi)
		advance = uint16(advHi)<<8 | uint16(advLo)
	}
	start := x0 + advance + 128
	if e.dxSign == 1 {
		start = x0 - advance + 128
	}
	e.xCurrent = byte(start >> 8)
	_, seedLow, seedHigh := cpu8.Mul16x8(e.dyLo, e.dyHi, byte(start))
	seed := uint16(seedHigh)<<8 | uint16(seedLow)
	if e.dxSign == 1 {
		seed = uint16(e.dyHi)<<8 | uint16(e.dyLo) - seed
	}
	e.errLo, e.errHi = byte(seed), byte(seed>>8)
	return e
}

func cubeFixed(low, high byte) uint16 {
	return uint16(high)<<8 | uint16(low)
}
