package main

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"runtime"
)

// 8.8 fixed-point type
type Fixed88 int16

func toFixed88(f float64) Fixed88 {
	return Fixed88(f * 256.0)
}

func (f Fixed88) toFloat() float64 {
	return float64(f) / 256.0
}

// Point in 8.8 fixed-point
type Point struct {
	X, Y Fixed88
}

// Load pattern from PNG
func loadPattern(filename string) ([][]bool, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	scale := bounds.Dx() / 24

	pattern := make([][]bool, 24)
	for y := 0; y < 24; y++ {
		pattern[y] = make([]bool, 24)
		for x := 0; x < 24; x++ {
			r, _, _, _ := img.At(x*scale+scale/2, y*scale+scale/2).RGBA()
			pattern[y][x] = (r < 32768)
		}
	}
	return pattern, nil
}

// Rotate point around pivot
func rotateFixed88(p Point, pivot Point, cos256, sin256 int32) Point {
	dx := int32(p.X - pivot.X)
	dy := int32(p.Y - pivot.Y)

	rxTemp := dx*cos256 - dy*sin256
	ryTemp := dx*sin256 + dy*cos256

	var rx, ry int32
	if rxTemp >= 0 {
		rx = (rxTemp + 128) >> 8
	} else {
		rx = (rxTemp - 128) >> 8
	}
	if ryTemp >= 0 {
		ry = (ryTemp + 128) >> 8
	} else {
		ry = (ryTemp - 128) >> 8
	}

	return Point{
		X: pivot.X + Fixed88(rx),
		Y: pivot.Y + Fixed88(ry),
	}
}

// Render quadrilateral
func renderQuad(corners [4]Point) [][]bool {
	grid := make([][]bool, 24)
	for i := range grid {
		grid[i] = make([]bool, 24)
	}

	minY, maxY := 24*256, -1
	for _, c := range corners {
		y := int(c.Y)
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}

	for y := minY >> 8; y <= (maxY>>8) && y < 24; y++ {
		if y < 0 {
			continue
		}
		yFixed := Fixed88(y << 8)

		var intersections []int
		for i := 0; i < 4; i++ {
			p1 := corners[i]
			p2 := corners[(i+1)%4]

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
				if x >= 0 && x < 24 {
					grid[y][x] = true
				} else if x >= 24 {
					grid[y][x%24] = true
				}
			}
		}
	}

	return grid
}

// Compare patterns
func comparePatterns(a, b [][]bool) float64 {
	matches := 0
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if a[y][x] == b[y][x] {
				matches++
			}
		}
	}
	return float64(matches) / 576.0 * 100.0
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	fmt.Println("🔍 Testing Rectangle (non-square) hypothesis")
	fmt.Println()

	// Load patterns
	var targets [11][][]bool
	for i := 0; i < 11; i++ {
		pattern, err := loadPattern(fmt.Sprintf("../emulator/frames/cycle_%02d.png", i+1))
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		targets[i] = pattern
	}

	// Best parameters so far (square: 12.0117)
	centerX := toFixed88(11.79296875)
	centerY := toFixed88(11.50390625)
	pivotX := toFixed88(17.88671875)
	pivotY := toFixed88(17.74218750)
	angle0 := 0.0

	// Try different width/height ratios
	fmt.Println("Testing width/height ratios:")
	fmt.Println()

	bestAccuracy := 0.0
	bestWidth := 12.0
	bestHeight := 12.0

	for width := 11.5; width <= 12.5; width += 0.05 {
		for height := 11.5; height <= 12.5; height += 0.05 {
			halfW := toFixed88(width / 2.0)
			halfH := toFixed88(height / 2.0)

			initial := [4]Point{
				{centerX - halfW, centerY - halfH},
				{centerX + halfW, centerY - halfH},
				{centerX + halfW, centerY + halfH},
				{centerX - halfW, centerY + halfH},
			}

			pivot := Point{pivotX, pivotY}
			angleStep := (math.Pi / 2.0) / 10.0
			totalAccuracy := 0.0

			for frame := 0; frame < 11; frame++ {
				angle := angle0 + float64(frame)*angleStep
				cos := math.Cos(angle)
				sin := math.Sin(angle)
				cos256 := int32(cos * 256)
				sin256 := int32(sin * 256)

				var rotated [4]Point
				for i := 0; i < 4; i++ {
					rotated[i] = rotateFixed88(initial[i], pivot, cos256, sin256)
				}

				rendered := renderQuad(rotated)
				accuracy := comparePatterns(rendered, targets[frame])
				totalAccuracy += accuracy
			}

			avgAccuracy := totalAccuracy / 11.0
			if avgAccuracy > bestAccuracy {
				bestAccuracy = avgAccuracy
				bestWidth = width
				bestHeight = height
				fmt.Printf("  Width=%.2f Height=%.2f → %.4f%%\n", width, height, avgAccuracy)
			}
		}
	}

	fmt.Println()
	fmt.Printf("Best: Width=%.3f Height=%.3f → %.4f%%\n", bestWidth, bestHeight, bestAccuracy)
	fmt.Println()

	if math.Abs(bestWidth-bestHeight) < 0.1 {
		fmt.Println("✓ Confirmed: It's essentially a square")
	} else {
		fmt.Println("⚠ It might be a rectangle!")
		fmt.Printf("  Difference: %.3f pixels\n", math.Abs(bestWidth-bestHeight))
	}
}
