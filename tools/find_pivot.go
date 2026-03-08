package main

import (
	"fmt"
	"image/png"
	"math"
	"os"
)

// 8.8 fixed-point type
type Fixed88 int16

func toFixed88(f float64) Fixed88 {
	return Fixed88(f * 256.0)
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

// Rotate point around pivot using 8.8 fixed-point
func rotateFixed88(p Point, pivot Point, cos256, sin256 int32) Point {
	dx := int32(p.X - pivot.X)
	dy := int32(p.Y - pivot.Y)
	rx := (dx*cos256 - dy*sin256) >> 8
	ry := (dx*sin256 + dy*cos256) >> 8
	return Point{
		X: pivot.X + Fixed88(rx),
		Y: pivot.Y + Fixed88(ry),
	}
}

// Render quadrilateral to 24x24 grid
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
			x1 := intersections[i] >> 8
			x2 := intersections[i+1] >> 8
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

// Evaluate with given pivot
func evaluateWithPivot(pivotX, pivotY float64, targets [11][][]bool) float64 {
	size := 12.0
	centerX := 12.0
	centerY := 12.0

	half := toFixed88(size / 2.0)
	cx := toFixed88(centerX)
	cy := toFixed88(centerY)

	initial := [4]Point{
		{cx - half, cy - half},
		{cx + half, cy - half},
		{cx + half, cy + half},
		{cx - half, cy + half},
	}

	pivot := Point{toFixed88(pivotX), toFixed88(pivotY)}
	angleStep := (math.Pi / 2.0) / 10.0
	totalAccuracy := 0.0

	for frame := 0; frame < 11; frame++ {
		angle := float64(frame) * angleStep
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

	return totalAccuracy / 11.0
}

func main() {
	fmt.Println("🔍 Finding Optimal Pivot Point")
	fmt.Println()

	// Load target patterns
	fmt.Println("Loading target patterns...")
	var targets [11][][]bool
	for i := 0; i < 11; i++ {
		pattern, err := loadPattern(fmt.Sprintf("../emulator/frames/cycle_%02d.png", i+1))
		if err != nil {
			fmt.Printf("Error loading pattern %d: %v\n", i+1, err)
			return
		}
		targets[i] = pattern
	}
	fmt.Println("✓ Loaded 11 patterns\n")

	// Grid search for pivot point
	fmt.Println("Searching for optimal pivot point...")
	fmt.Println("(Testing grid from 6,6 to 24,24 with 0.5 pixel increments)")
	fmt.Println()

	bestAccuracy := 0.0
	bestPivotX := 18.0
	bestPivotY := 18.0

	for py := 6.0; py <= 24.0; py += 0.5 {
		for px := 6.0; px <= 24.0; px += 0.5 {
			accuracy := evaluateWithPivot(px, py, targets)
			if accuracy > bestAccuracy {
				bestAccuracy = accuracy
				bestPivotX = px
				bestPivotY = py
				fmt.Printf("  New best: pivot=(%.1f, %.1f) → %.4f%%\n",
					bestPivotX, bestPivotY, bestAccuracy)
			}
		}
	}

	fmt.Println()
	fmt.Println("=" + "=")
	fmt.Println("🎯 OPTIMAL PIVOT POINT")
	fmt.Println("="	+ "=")
	fmt.Printf("Pivot: (%.1f, %.1f)\n", bestPivotX, bestPivotY)
	fmt.Printf("Accuracy: %.4f%%\n", bestAccuracy)
	fmt.Println()

	// Show details
	fmt.Println("With optimal pivot:")
	fmt.Println("  Size: 12 x 12")
	fmt.Println("  Center: (12, 12)")
	fmt.Printf("  Pivot: (%.1f, %.1f)\n", bestPivotX, bestPivotY)
	fmt.Printf("  Pivot offset from center: (%.1f, %.1f)\n",
		bestPivotX-12.0, bestPivotY-12.0)
}
