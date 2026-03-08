package main

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"runtime"
	"strings"
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

// Evaluate with given angles
func evaluateAngles(angles [11]float64, size, cx, cy, px, py Fixed88, targets [11][][]bool) (float64, int) {
	half := size / 2
	initial := [4]Point{
		{cx - half, cy - half},
		{cx + half, cy - half},
		{cx + half, cy + half},
		{cx - half, cy + half},
	}

	pivot := Point{px, py}
	totalAccuracy := 0.0
	perfectCount := 0

	for frame := 0; frame < 11; frame++ {
		cos := math.Cos(angles[frame])
		sin := math.Sin(angles[frame])
		cos256 := int32(cos * 256)
		sin256 := int32(sin * 256)

		var rotated [4]Point
		for i := 0; i < 4; i++ {
			rotated[i] = rotateFixed88(initial[i], pivot, cos256, sin256)
		}

		rendered := renderQuad(rotated)
		accuracy := comparePatterns(rendered, targets[frame])
		totalAccuracy += accuracy
		if accuracy == 100.0 {
			perfectCount++
		}
	}

	return totalAccuracy / 11.0, perfectCount
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	fmt.Println("🎯 Angle-Only Optimization (Greedy per-frame)")
	fmt.Println("   Using optimal spatial parameters from exhaustive search")
	fmt.Println("   Optimizing each frame's angle independently")
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

	// Best known spatial parameters (from exhaustive search)
	size := Fixed88(3065)
	centerX := Fixed88(3010)
	centerY := Fixed88(2954)
	pivotX := Fixed88(4578)
	pivotY := Fixed88(4552)

	fmt.Println("Fixed spatial parameters:")
	fmt.Printf("  Size:    %d (%.6f)\n", size, size.toFloat())
	fmt.Printf("  Center:  (%d, %d) (%.6f, %.6f)\n", centerX, centerY, centerX.toFloat(), centerY.toFloat())
	fmt.Printf("  Pivot:   (%d, %d) (%.6f, %.6f)\n", pivotX, pivotY, pivotX.toFloat(), pivotY.toFloat())
	fmt.Println()

	// Initialize with ideal 9° steps
	baseAngleStep := math.Pi / 20.0 // 9 degrees
	angles := [11]float64{}
	for i := 0; i < 11; i++ {
		angles[i] = float64(i) * baseAngleStep
	}

	// Greedy per-frame optimization
	fmt.Println("Optimizing angles (greedy search, ±2° with 0.1° steps)...")
	improved := true
	iteration := 0

	for improved {
		improved = false
		iteration++

		for frame := 0; frame < 11; frame++ {
			originalAngle := angles[frame]
			bestAngle := originalAngle
			currentAccuracy, currentPerfect := evaluateAngles(angles, size, centerX, centerY, pivotX, pivotY, targets)

			// Search ±2 degrees in 0.1 degree steps
			searchRange := 0.035 // ~2 degrees in radians
			searchStep := 0.002  // ~0.11 degrees
			for delta := -searchRange; delta <= searchRange; delta += searchStep {
				testAngle := originalAngle + delta
				angles[frame] = testAngle
				testAccuracy, testPerfect := evaluateAngles(angles, size, centerX, centerY, pivotX, pivotY, targets)

				if testPerfect > currentPerfect || (testPerfect == currentPerfect && testAccuracy > currentAccuracy) {
					bestAngle = testAngle
					currentAccuracy = testAccuracy
					currentPerfect = testPerfect
					improved = true
				}
			}

			angles[frame] = bestAngle
		}

		accuracy, perfect := evaluateAngles(angles, size, centerX, centerY, pivotX, pivotY, targets)
		fmt.Printf("  Iteration %d: %.4f%% (%d/11 perfect)\n", iteration, accuracy, perfect)

		if perfect == 11 {
			fmt.Println("\n🎉 FOUND 11/11 PERFECT!")
			break
		}
	}

	// Final evaluation
	finalAccuracy, perfectCount := evaluateAngles(angles, size, centerX, centerY, pivotX, pivotY, targets)

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("🎉 FINAL RESULTS")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("Overall accuracy: %.6f%% (%d/11 perfect frames)\n", finalAccuracy, perfectCount)
	fmt.Println()

	fmt.Println("Per-frame angles:")
	for frame := 0; frame < 11; frame++ {
		idealAngle := float64(frame) * baseAngleStep
		deviation := angles[frame] - idealAngle

		// Check this frame's accuracy
		testAngles := [11]float64{}
		for i := 0; i < 11; i++ {
			testAngles[i] = angles[i]
		}
		half := size / 2
		initial := [4]Point{
			{centerX - half, centerY - half},
			{centerX + half, centerY - half},
			{centerX + half, centerY + half},
			{centerX - half, centerY + half},
		}
		pivot := Point{pivotX, pivotY}
		cos := math.Cos(angles[frame])
		sin := math.Sin(angles[frame])
		cos256 := int32(cos * 256)
		sin256 := int32(sin * 256)
		var rotated [4]Point
		for i := 0; i < 4; i++ {
			rotated[i] = rotateFixed88(initial[i], pivot, cos256, sin256)
		}
		rendered := renderQuad(rotated)
		frameAccuracy := comparePatterns(rendered, targets[frame])

		status := " "
		if frameAccuracy == 100.0 {
			status = "✓"
		}

		fmt.Printf("  Frame %02d: %.6f rad (%.3f°) [ideal: %.3f°, Δ: %+.3f°] %.2f%% %s\n",
			frame+1,
			angles[frame],
			angles[frame]*180/math.Pi,
			idealAngle*180/math.Pi,
			deviation*180/math.Pi,
			frameAccuracy,
			status)
	}
}
