package main

import "math"

const (
	morphDDAFirstRow = 1
	morphDDALastRow  = 17
	morphDDAShift    = 4
	morphDDAByteRows = morphDDALastRow * morphRowBytes
)

func renderMorphDDA(c morphCoordinates) morphCoverage {
	kite := morphKiteFromCoordinates(c)
	var corners [4]corner
	for i, vertex := range kite {
		corners[i] = morphCorner(vertex)
	}

	dest := make([]byte, morphDDAByteRows)
	renderCorners(corners, false, dest)
	return mirrorMorphPetal(dest)
}

func morphCorner(vertex morphVertex) corner {
	x := fixed88(vertex.x + morphDDAShift)
	y := fixed88(vertex.y + morphDDAShift)
	return corner{xLo: byte(x), xHi: byte(x >> 8), yLo: byte(y), yHi: byte(y >> 8)}
}

func fixed88(pixels float64) uint16 {
	return uint16(math.Round(pixels * 2 * 256))
}

func mirrorMorphPetal(rows []byte) morphCoverage {
	var coverage morphCoverage
	for shiftedY := morphDDAFirstRow; shiftedY <= morphDDALastRow; shiftedY++ {
		offset := (shiftedY - morphDDAFirstRow) * morphRowBytes
		for shiftedX := 0; shiftedX < morphTileSize; shiftedX++ {
			level := morphPetalLevel(rows[offset:offset+morphRowBytes], shiftedX)
			if level == 0 {
				continue
			}
			x := (shiftedX - morphDDAShift + morphTileSize) % morphTileSize
			y := (shiftedY - morphDDAShift + morphTileSize) % morphTileSize
			for _, mirrored := range [4][2]int{{x, y}, {morphTileSize - 1 - x, y}, {x, morphTileSize - 1 - y}, {morphTileSize - 1 - x, morphTileSize - 1 - y}} {
				coverage[mirrored[1]][mirrored[0]] = max(coverage[mirrored[1]][mirrored[0]], level)
			}
		}
	}
	return coverage
}

func morphPetalLevel(row []byte, x int) int {
	bit := byte(0x80) >> (x % 8)
	switch {
	case row[x/8]&bit != 0:
		return 0
	case row[3+x/8]&bit != 0:
		return 1
	}
	return morphFullCoverage
}
