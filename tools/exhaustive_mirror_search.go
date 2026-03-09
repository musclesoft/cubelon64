package main

import (
	"fmt"
	"math"
)

type Fixed88 int16

func toFixed88(f float64) Fixed88 {
	return Fixed88(f * 256.0)
}

func (f Fixed88) toFloat() float64 {
	return float64(f) / 256.0
}

type Point struct {
	X, Y Fixed88
}

func rotateTruncated(p Point, pivot Point, cos256, sin256 int32) Point {
	dx := int32(p.X - pivot.X)
	dy := int32(p.Y - pivot.Y)

	rxTemp := dx*cos256 - dy*sin256
	ryTemp := dx*sin256 + dy*cos256

	rx := rxTemp >> 8
	ry := ryTemp >> 8

	return Point{
		X: pivot.X + Fixed88(rx),
		Y: pivot.Y + Fixed88(ry),
	}
}

func renderQuadHighRes(corners [4]Point) [][]bool {
	grid := make([][]bool, 48)
	for i := range grid {
		grid[i] = make([]bool, 48)
	}

	var scaledCorners [4]Point
	for i := 0; i < 4; i++ {
		scaledCorners[i] = Point{
			X: corners[i].X << 1,
			Y: corners[i].Y << 1,
		}
	}

	minY, maxY := 48*256, -1
	for _, c := range scaledCorners {
		y := int(c.Y)
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}

	for y := minY >> 8; y <= (maxY>>8) && y < 48; y++ {
		if y < 0 {
			continue
		}
		yFixed := Fixed88(y << 8)

		var intersections []int
		for i := 0; i < 4; i++ {
			p1 := scaledCorners[i]
			p2 := scaledCorners[(i+1)%4]

			if (p1.Y <= yFixed && p2.Y > yFixed) || (p2.Y <= yFixed && p1.Y > yFixed) {
				dy := int32(p2.Y - p1.Y)
				if dy != 0 {
					t := int32(yFixed-p1.Y) * 256 / dy
					x := int32(p1.X) + (int32(p2.X-p1.X)*t)/256
					intersections = append(intersections, int(x))
				}
			}
		}

		for i := 0; i < len(intersections)-1; i++ {
			for j := i + 1; j < len(intersections); j++ {
				if intersections[i] > intersections[j] {
					intersections[i], intersections[j] = intersections[j], intersections[i]
				}
			}
		}

		for i := 0; i+1 < len(intersections); i += 2 {
			x1 := (intersections[i] + 128) >> 8
			x2 := (intersections[i+1] + 128) >> 8
			for x := x1; x < x2; x++ {
				if x >= 0 && x < 48 {
					grid[y][x] = true
				} else {
					grid[y][(x%48+48)%48] = true
				}
			}
		}
	}

	return grid
}

func renderQuadAntialiased(corners [4]Point) [][]uint8 {
	highRes := renderQuadHighRes(corners)

	grid := make([][]uint8, 24)
	for y := 0; y < 24; y++ {
		grid[y] = make([]uint8, 24)
		for x := 0; x < 24; x++ {
			count := 0
			if highRes[y*2][x*2] {
				count++
			}
			if highRes[y*2][x*2+1] {
				count++
			}
			if highRes[y*2+1][x*2] {
				count++
			}
			if highRes[y*2+1][x*2+1] {
				count++
			}

			if count >= 3 {
				grid[y][x] = 170
			} else if count >= 1 {
				grid[y][x] = 104
			} else {
				grid[y][x] = 0
			}
		}
	}

	return grid
}

func hasAliasing(grid [][]uint8) int {
	artifacts := 0
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if grid[y][x] == 104 {
				artifacts++
			}
		}
	}
	return artifacts
}

func compareMirrorY(grid1, grid2 [][]uint8) float64 {
	bestMatch := 0

	for xShift := 0; xShift < 24; xShift++ {
		matches := 0
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				mirrorY := 23 - y
				mirrorX := (x + xShift) % 24

				if grid1[y][x] == grid2[mirrorY][mirrorX] {
					matches++
				}
			}
		}
		if matches > bestMatch {
			bestMatch = matches
		}
	}

	return float64(bestMatch) / 576.0 * 100.0
}

func main() {
	fmt.Println("🔍 Exhaustive Search for 100% Mirror Symmetry")
	fmt.Println()

	angleStep := math.Pi / 20.0

	bestScore := 0.0
	var bestSize, bestCenter Fixed88

	fmt.Println("Searching all combinations (step by 2)...")
	fmt.Println()

	// Exhaustive search with finer granularity
	for size := 3000; size <= 3120; size += 2 {
		for center := 3000; center <= 3120; center += 2 {
			sizeFx := Fixed88(size)
			centerFx := Fixed88(center)

			half := sizeFx / 2
			initial := [4]Point{
				{centerFx - half, centerFx - half},
				{centerFx + half, centerFx - half},
				{centerFx + half, centerFx + half},
				{centerFx - half, centerFx + half},
			}

			pivot := Point{centerFx + half, centerFx + half}

			// Render all frames
			var frames [11][][]uint8
			for frame := 0; frame < 11; frame++ {
				angle := float64(frame) * angleStep
				cos := math.Cos(angle)
				sin := math.Sin(angle)
				cos256 := int32(cos * 256)
				sin256 := int32(sin * 256)

				var rotated [4]Point
				for i := 0; i < 4; i++ {
					rotated[i] = rotateTruncated(initial[i], pivot, cos256, sin256)
				}

				frames[frame] = renderQuadAntialiased(rotated)
			}

			// Check aliasing in frames 0 and 10
			aliasing0 := hasAliasing(frames[0])
			aliasing10 := hasAliasing(frames[10])

			// Require clean axis-aligned frames
			if aliasing0 > 20 || aliasing10 > 20 {
				continue
			}

			// Check mirror symmetry
			totalMirror := 0.0
			pairs := [][2]int{{0, 10}, {1, 9}, {2, 8}, {3, 7}, {4, 6}}
			for _, pair := range pairs {
				mirror := compareMirrorY(frames[pair[0]], frames[pair[1]])
				totalMirror += mirror
			}
			avgMirror := totalMirror / 5.0

			if avgMirror > bestScore {
				bestScore = avgMirror
				bestSize = sizeFx
				bestCenter = centerFx

				fmt.Printf("  %.2f%% - Size=%d (%.4f), Center=%d (%.4f)\n",
					avgMirror, size, sizeFx.toFloat(), center, centerFx.toFloat())
				fmt.Printf("    Aliasing: 0°=%d, 90°=%d\n", aliasing0, aliasing10)

				if avgMirror >= 99.9 {
					fmt.Println("    ⭐ Near-perfect mirror symmetry!")
				}
			}
		}
	}

	fmt.Println()
	if bestScore >= 99.9 {
		fmt.Println("✓ Found near-perfect parameters!")
	} else {
		fmt.Println("Best achievable mirror symmetry:")
	}
	fmt.Printf("  Size:    %d (%.6f) = $%04X\n", bestSize, bestSize.toFloat(), bestSize)
	fmt.Printf("  Center:  %d (%.6f) = $%04X\n", bestCenter, bestCenter.toFloat(), bestCenter)
	fmt.Printf("  Score:   %.2f%%\n", bestScore)
	fmt.Println()

	if bestScore < 99.9 {
		fmt.Println("Note: 100% mirror symmetry may be impossible due to:")
		fmt.Println("  - Fixed-point quantization")
		fmt.Println("  - Rasterization discretization")
		fmt.Println("  - Constraint of clean axis-aligned frames")
	}
}
