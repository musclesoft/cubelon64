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

// Rotate with truncation
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

// Render with ceiling on left edge, rounding on right edge
func renderQuadCeilingLeft(corners [4]Point) [][]bool {
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
			// Use CEILING for left edge
			x1 := (intersections[i] + 255) >> 8
			// Use ROUNDING for right edge
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

// Original rendering (rounding on both edges)
func renderQuadRounding(corners [4]Point) [][]bool {
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
	fmt.Println("🔍 Testing Ceiling Left Edge vs Rounding Both Edges")
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

	// Truncation-optimized parameters
	size := Fixed88(3072)
	centerX := Fixed88(3072)
	centerY := Fixed88(2944)
	pivotX := Fixed88(4608)
	pivotY := Fixed88(4480)

	angleStep := math.Pi / 20.0

	fmt.Println("Frame-by-frame comparison:")
	fmt.Println("Frame | Rounding  | Ceiling-Left | Δ")
	fmt.Println("------|-----------|--------------|----------")

	totalRounding := 0.0
	totalCeiling := 0.0
	perfectRounding := 0
	perfectCeiling := 0

	for frame := 0; frame < 11; frame++ {
		angle := float64(frame) * angleStep
		cos := math.Cos(angle)
		sin := math.Sin(angle)
		cos256 := int32(cos * 256)
		sin256 := int32(sin * 256)

		half := size / 2
		initial := [4]Point{
			{centerX - half, centerY - half},
			{centerX + half, centerY - half},
			{centerX + half, centerY + half},
			{centerX - half, centerY + half},
		}
		pivot := Point{pivotX, pivotY}

		var rotated [4]Point
		for i := 0; i < 4; i++ {
			rotated[i] = rotateTruncated(initial[i], pivot, cos256, sin256)
		}

		renderedRounding := renderQuadRounding(rotated)
		renderedCeiling := renderQuadCeilingLeft(rotated)

		pixelRounding := comparePatterns(targets[frame], renderedRounding)
		pixelCeiling := comparePatterns(targets[frame], renderedCeiling)

		totalRounding += pixelRounding
		totalCeiling += pixelCeiling

		if pixelRounding == 100.0 {
			perfectRounding++
		}
		if pixelCeiling == 100.0 {
			perfectCeiling++
		}

		status := ""
		if pixelRounding == 100.0 {
			status += "R"
		} else {
			status += " "
		}
		if pixelCeiling == 100.0 {
			status += "C"
		} else {
			status += " "
		}

		fmt.Printf("  %02d  | %6.2f%% %s | %6.2f%% %s   | %+.2f%%\n",
			frame+1,
			pixelRounding, string(status[0]),
			pixelCeiling, string(status[1]),
			pixelCeiling-pixelRounding)
	}

	fmt.Println()
	fmt.Printf("Overall:  %6.2f%%    | %6.2f%%      | %+.2f%%\n",
		totalRounding/11.0, totalCeiling/11.0, (totalCeiling-totalRounding)/11.0)
	fmt.Printf("Perfect:  %d/11       | %d/11\n", perfectRounding, perfectCeiling)
	fmt.Println()

	// Detailed analysis of frame 6
	fmt.Println("Frame 6 (45°) detailed comparison:")
	angle := 5.0 * angleStep
	cos := math.Cos(angle)
	sin := math.Sin(angle)
	cos256 := int32(cos * 256)
	sin256 := int32(sin * 256)

	half := size / 2
	initial := [4]Point{
		{centerX - half, centerY - half},
		{centerX + half, centerY - half},
		{centerX + half, centerY + half},
		{centerX - half, centerY + half},
	}
	pivot := Point{pivotX, pivotY}

	var rotated [4]Point
	for i := 0; i < 4; i++ {
		rotated[i] = rotateTruncated(initial[i], pivot, cos256, sin256)
	}

	target := targets[5]
	renderedRounding := renderQuadRounding(rotated)
	renderedCeiling := renderQuadCeilingLeft(rotated)

	mismatchesRounding := 0
	mismatchesCeiling := 0
	fixed := 0
	broken := 0

	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			roundMatch := target[y][x] == renderedRounding[y][x]
			ceilMatch := target[y][x] == renderedCeiling[y][x]

			if !roundMatch {
				mismatchesRounding++
			}
			if !ceilMatch {
				mismatchesCeiling++
			}

			if !roundMatch && ceilMatch {
				fixed++
				fmt.Printf("  FIXED: (%d, %d)\n", x, y)
			} else if roundMatch && !ceilMatch {
				broken++
				fmt.Printf("  BROKEN: (%d, %d)\n", x, y)
			}
		}
	}

	fmt.Printf("\nRounding mismatches: %d\n", mismatchesRounding)
	fmt.Printf("Ceiling mismatches: %d\n", mismatchesCeiling)
	fmt.Printf("Pixels fixed: %d\n", fixed)
	fmt.Printf("Pixels broken: %d\n", broken)
	fmt.Printf("Net improvement: %d pixels\n", fixed-broken)
}
