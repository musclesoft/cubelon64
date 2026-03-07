package main

import (
	"fmt"
	"math"
)

// Fixed-point 8.8 math
const (
	fpShift = 8
	fpScale = 1 << fpShift
)

type fp88 int32

func floatToFP(f float64) fp88 {
	return fp88(f * fpScale)
}

func fpMul(a, b fp88) fp88 {
	return fp88((int64(a) * int64(b)) >> fpShift)
}

func fpToInt(f fp88) int {
	return int(f >> fpShift)
}

// Edge for scanline fill
type Edge struct {
	yStart float64
	yEnd   float64
	xStart float64
	dxDy   fp88
}

func newEdge(x1, y1, x2, y2 float64) *Edge {
	if y1 > y2 {
		x1, y1, x2, y2 = x2, y2, x1, y1
	}

	e := &Edge{
		yStart: y1,
		yEnd:   y2,
		xStart: x1,
	}

	dy := y2 - y1
	if dy > 0.001 {
		e.dxDy = floatToFP((x2 - x1) / dy)
	}

	return e
}

func (e *Edge) getXAtY(y float64) int {
	dy := y - e.yStart
	dx := fpMul(floatToFP(dy), e.dxDy)
	x := floatToFP(e.xStart) + dx
	return fpToInt(x)
}

// Render square using 4-edge scanline fill
func renderSquare(cx, cy, size, angleDeg float64, width, height int) [][]bool {
	bitmap := make([][]bool, height)
	for i := range bitmap {
		bitmap[i] = make([]bool, width)
	}

	angleRad := angleDeg * math.Pi / 180.0
	halfSize := size / 2.0

	cos := math.Cos(angleRad)
	sin := math.Sin(angleRad)

	// Four corners
	corners := [4][2]float64{
		{-halfSize, -halfSize},
		{halfSize, -halfSize},
		{halfSize, halfSize},
		{-halfSize, halfSize},
	}

	vertices := [4][2]float64{}
	for i := 0; i < 4; i++ {
		x := corners[i][0]*cos - corners[i][1]*sin + cx
		y := corners[i][0]*sin + corners[i][1]*cos + cy
		vertices[i] = [2]float64{x, y}
	}

	// Create 4 edges
	edges := make([]*Edge, 4)
	for i := 0; i < 4; i++ {
		next := (i + 1) % 4
		edges[i] = newEdge(vertices[i][0], vertices[i][1], vertices[next][0], vertices[next][1])
	}

	// Debug: print vertices
	fmt.Println("Vertices:")
	for i, v := range vertices {
		fmt.Printf("  %d: (%.2f, %.2f)\n", i, v[0], v[1])
	}
	fmt.Println()

	// Scanline fill
	for y := 0; y < height; y++ {
		yf := float64(y) + 0.5

		// Find active edges
		var activeEdges []*Edge
		for _, e := range edges {
			if yf >= e.yStart && yf < e.yEnd {
				activeEdges = append(activeEdges, e)
			}
		}

		if len(activeEdges) < 2 {
			continue
		}

		// Get X intersections
		xIntersects := make([]int, len(activeEdges))
		for i, e := range activeEdges {
			xIntersects[i] = e.getXAtY(yf)
		}

		// Sort intersections
		for i := 0; i < len(xIntersects)-1; i++ {
			for j := i + 1; j < len(xIntersects); j++ {
				if xIntersects[i] > xIntersects[j] {
					xIntersects[i], xIntersects[j] = xIntersects[j], xIntersects[i]
				}
			}
		}

		// Debug first line
		if y == 6 {
			fmt.Printf("Y=%d: active=%d, x=[", y, len(activeEdges))
			for i, x := range xIntersects {
				if i > 0 {
					fmt.Print(", ")
				}
				fmt.Printf("%d", x)
			}
			fmt.Println("]")
		}

		// Fill spans
		for i := 0; i < len(xIntersects); i += 2 {
			if i+1 >= len(xIntersects) {
				break
			}

			x1 := xIntersects[i]
			x2 := xIntersects[i+1]

			if x1 < 0 {
				x1 = 0
			}
			if x2 >= width {
				x2 = width - 1
			}

			for x := x1; x <= x2; x++ {
				if x >= 0 && x < width {
					bitmap[y][x] = true
				}
			}
		}
	}

	return bitmap
}

func main() {
	// Test parameters from final_optimization.py
	cx, cy, size, angle := 12.0, 11.0, 8.0, 61.1

	fmt.Println("Go rendering test:")
	fmt.Printf("cx=%.1f, cy=%.1f, size=%.1f, angle=%.1f°\n\n", cx, cy, size, angle)

	bitmap := renderSquare(cx, cy, size, angle, 24, 24)

	// Print bitmap
	fmt.Println("Bitmap:")

	count := 0
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if bitmap[y][x] {
				fmt.Print("█")
				count++
			} else {
				fmt.Print(" ")
			}
		}
		fmt.Println()
	}

	fmt.Printf("\nPixels set: %d\n", count)
}
