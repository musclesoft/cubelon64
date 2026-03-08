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

// Evaluate parameters
func evaluateParams(size, cx, cy, px, py Fixed88, targets [11][][]bool) (float64, int) {
	half := size / 2
	angleStep := math.Pi / 20.0

	totalPixel := 0.0
	perfect := 0

	for frame := 0; frame < 11; frame++ {
		angle := float64(frame) * angleStep
		cos := math.Cos(angle)
		sin := math.Sin(angle)
		cos256 := int32(cos * 256)
		sin256 := int32(sin * 256)

		initial := [4]Point{
			{cx - half, cy - half},
			{cx + half, cy - half},
			{cx + half, cy + half},
			{cx - half, cy + half},
		}
		pivot := Point{px, py}

		var rotated [4]Point
		for i := 0; i < 4; i++ {
			rotated[i] = rotateFixed88(initial[i], pivot, cos256, sin256)
		}

		rendered := renderQuad(rotated)
		pixel := comparePatterns(targets[frame], rendered)

		totalPixel += pixel
		if pixel == 100.0 {
			perfect++
		}
	}

	return totalPixel / 11.0, perfect
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	fmt.Println("🔍 Testing Hybrid Pivot Y")
	fmt.Println()

	// Load target patterns
	var targets [11][][]bool
	for i := 0; i < 11; i++ {
		pattern, err := loadPattern(fmt.Sprintf("../emulator/frames/cycle_%02d.png", i+1))
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		targets[i] = pattern
	}

	size := Fixed88(3072)    // 12.0
	cx := Fixed88(3008)      // 11.75
	cy := Fixed88(2944)      // 11.50
	px := Fixed88(4608)      // 18.0

	fmt.Println("Testing different Pivot Y values:")
	fmt.Println("(Size=3072, CX=3008, CY=2944, PX=4608)")
	fmt.Println()

	optimalPivotY := Fixed88(4552) // 17.781 (optimal)

	// Test range around optimal
	for py := 4480; py <= 4640; py += 32 {
		pyFixed := Fixed88(py)
		pixel, perfect := evaluateParams(size, cx, cy, px, pyFixed, targets)

		marker := "  "
		if py == 4544 {
			marker = "← quarter-pixel"
		} else if py == 4608 {
			marker = "← simple (18.0)"
		} else if py == int(optimalPivotY) {
			marker = "← optimal"
		}

		fmt.Printf("  PY=%4d (%.3f): %.2f%% (%d/11 perfect) %s\n",
			py, pyFixed.toFloat(), pixel, perfect, marker)
	}

	fmt.Println()
	fmt.Println("Testing smaller steps around 17.75-18.00:")
	for py := 4544; py <= 4608; py += 8 {
		pyFixed := Fixed88(py)
		pixel, perfect := evaluateParams(size, cx, cy, px, pyFixed, targets)

		marker := "  "
		if py == 4544 {
			marker = "← quarter (17.75)"
		} else if py == 4608 {
			marker = "← simple (18.00)"
		} else if py == int(optimalPivotY) {
			marker = "← optimal (17.781)"
		} else if py == 4560 {
			marker = "  (17.81)"
		} else if py == 4576 {
			marker = "  (17.88)"
		}

		fmt.Printf("  PY=%4d (%.3f): %.2f%% (%d/11 perfect) %s\n",
			py, pyFixed.toFloat(), pixel, perfect, marker)
	}
}
