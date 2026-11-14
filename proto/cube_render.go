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

func cubeFixedCorner(point [2]float64) corner {
	var fixed corner
	fixed.xLo, fixed.xHi = cubeFixedByte(math.Max(0, point[0]))
	fixed.yLo, fixed.yHi = cubeFixedByte(math.Max(0, point[1]))
	return fixed
}

// The subpixel columns each subpixel row of a face covers, from the DDA edges of the quad
// renderer. The corners are in subpixels; the scan lines sample the top of a subpixel row where
// the model samples the centre, so the rows move up by half a subpixel.
func cubeFaceSpans(corners [4][2]float64) [cubeSuperSize]cubeSpan {
	var points [4][2]float64
	for i, c := range corners {
		points[i] = [2]float64{2 * c[0], 2*c[1] - 0.5}
	}
	var edges [4]ddaEdge
	for i := range edges {
		edges[i] = cubeInitEdge(cubeFixedCorner(points[i]), cubeFixedCorner(points[(i+1)%len(points)]))
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

// The subpixels a face draws: the white face all of them, the dots face (25%) only those on odd
// lines, which makes it a solid dark grey, and the checker face (50%) those and a sparse dot on
// every second pixel: on even lines, one column in four, shifted by two columns on each drawn
// line. Those pixels count three, light grey, the others two, dark grey: a checkerboard.
func cubeSubpixelDrawn(shade, column, row int) bool {
	switch shade {
	case cubeCheckerShade:
		return row%2 == 1 || column%4 == (row+1)%4
	case cubeDotsShade:
		return column%4 == (row+1)%4
	}
	return true
}

func (counts *cubeCounts) addFace(spans *[cubeSuperSize]cubeSpan, shade int) {
	for row, span := range spans {
		if !span.valid {
			continue
		}
		for column := int(span.x1); column < int(span.x2) && column < cubeSuperSize; column++ {
			if cubeSubpixelDrawn(shade, column, row) {
				counts[row/2][column/2]++
			}
		}
	}
}

// Pairs of neighbouring dark grey pixels that both lie where the white and the checker face
// meet. The checker's own dark pixels are never neighbours, so such a pair means a junction
// pixel that should have been light.
func (camera cubeCamera) junctionDarkPairs(rotation cubeMatrix, levels *cubeLevels) int {
	var touched [cubeShades]cubeCounts
	for _, face := range camera.visibleFaces(rotation) {
		spans := cubeFaceSpans(face.corners)
		for row, span := range spans {
			for column := int(span.x1); span.valid && column < int(span.x2) && column < cubeSuperSize; column++ {
				touched[face.shade][row/2][column/2]++
			}
		}
	}
	junction := func(x, y int) bool {
		return touched[cubeWhiteShade][y][x] > 0 && touched[cubeCheckerShade][y][x] > 0
	}
	pairs := 0
	for y := range levels {
		for x := range levels[y] {
			if levels[y][x] != 1 {
				continue
			}
			if x+1 < morphTileSize && levels[y][x+1] == 1 && junction(x, y) && junction(x+1, y) {
				pairs++
			}
			if y+1 < morphTileSize && levels[y+1][x] == 1 && junction(x, y) && junction(x, y+1) {
				pairs++
			}
		}
	}
	return pairs
}

// What the renderer draws for the visible faces in one go: the subpixels the patterns of all
// faces draw are counted together per pixel, so a pixel two faces share is thresholded on its
// whole coverage: 3-4 subpixels give light grey, 1-2 dark grey, like every quad the renderer draws
func (camera cubeCamera) renderLevels(rotation cubeMatrix) cubeLevels {
	var counts cubeCounts
	for _, face := range camera.visibleFaces(rotation) {
		spans := cubeFaceSpans(face.corners)
		counts.addFace(&spans, face.shade)
	}
	var levels cubeLevels
	for y := range levels {
		for x := range levels[y] {
			levels[y][x] = morphCoverageLevel(counts[y][x])
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
