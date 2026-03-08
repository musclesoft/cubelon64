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

// Rotate with rounding (current method)
func rotateRounded(p Point, pivot Point, cos256, sin256 int32) Point {
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

// Rotate with truncation (simpler - just >> 8)
func rotateTruncated(p Point, pivot Point, cos256, sin256 int32) Point {
	dx := int32(p.X - pivot.X)
	dy := int32(p.Y - pivot.Y)

	rxTemp := dx*cos256 - dy*sin256
	ryTemp := dx*sin256 + dy*cos256

	// Simple truncation - no rounding
	rx := rxTemp >> 8
	ry := ryTemp >> 8

	return Point{
		X: pivot.X + Fixed88(rx),
		Y: pivot.Y + Fixed88(ry),
	}
}

// Render quadrilateral with specified rotation method
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

type RotateFunc func(Point, Point, int32, int32) Point

// Evaluate with rotation method
func evaluateWithRotation(size, cx, cy, px, py Fixed88, targets [11][][]bool, rotateFn RotateFunc) (float64, int) {
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
			rotated[i] = rotateFn(initial[i], pivot, cos256, sin256)
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

	fmt.Println("🔍 Testing Truncation vs Rounding")
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

	params := []struct {
		name string
		size, cx, cy, px, py Fixed88
	}{
		{
			"Optimal",
			Fixed88(3065), Fixed88(3010), Fixed88(2954), Fixed88(4578), Fixed88(4552),
		},
		{
			"Quarter-pixel",
			Fixed88(3072), Fixed88(3008), Fixed88(2944), Fixed88(4608), Fixed88(4544),
		},
		{
			"Simple",
			Fixed88(3072), Fixed88(3072), Fixed88(3072), Fixed88(4608), Fixed88(4608),
		},
	}

	fmt.Println("Comparing rounding vs truncation:")
	fmt.Println()

	for _, p := range params {
		fmt.Printf("%s parameters:\n", p.name)
		fmt.Printf("  Size=%d (%.2f), Center=(%d,%d) (%.2f,%.2f), Pivot=(%d,%d) (%.2f,%.2f)\n",
			p.size, p.size.toFloat(),
			p.cx, p.cy, p.cx.toFloat(), p.cy.toFloat(),
			p.px, p.py, p.px.toFloat(), p.py.toFloat())

		roundPixel, roundPerfect := evaluateWithRotation(p.size, p.cx, p.cy, p.px, p.py, targets, rotateRounded)
		truncPixel, truncPerfect := evaluateWithRotation(p.size, p.cx, p.cy, p.px, p.py, targets, rotateTruncated)

		fmt.Printf("  With rounding:   %.2f%% (%d/11 perfect)\n", roundPixel, roundPerfect)
		fmt.Printf("  With truncation: %.2f%% (%d/11 perfect)\n", truncPixel, truncPerfect)
		fmt.Printf("  Difference:      %.2f%% (-%d perfect)\n", roundPixel-truncPixel, roundPerfect-truncPerfect)
		fmt.Println()
	}

	fmt.Println("Testing if we can find better params for truncation:")
	fmt.Println("(Searching size/center/pivot adjustments)")
	fmt.Println()

	// Try to optimize for truncation with simple base params
	baseSize := Fixed88(3072)
	baseCX := Fixed88(3072)
	baseCY := Fixed88(3072)
	basePX := Fixed88(4608)
	basePY := Fixed88(4608)

	bestPixel := 0.0
	var bestParams struct{ size, cx, cy, px, py Fixed88 }

	// Small search around simple params for truncation
	for sizeOffset := -128; sizeOffset <= 128; sizeOffset += 32 {
		for cxOffset := -128; cxOffset <= 128; cxOffset += 32 {
			for cyOffset := -128; cyOffset <= 128; cyOffset += 32 {
				for pxOffset := -128; pxOffset <= 128; pxOffset += 64 {
					for pyOffset := -128; pyOffset <= 128; pyOffset += 64 {
						size := baseSize + Fixed88(sizeOffset)
						cx := baseCX + Fixed88(cxOffset)
						cy := baseCY + Fixed88(cyOffset)
						px := basePX + Fixed88(pxOffset)
						py := basePY + Fixed88(pyOffset)

						pixel, _ := evaluateWithRotation(size, cx, cy, px, py, targets, rotateTruncated)
						if pixel > bestPixel {
							bestPixel = pixel
							bestParams.size = size
							bestParams.cx = cx
							bestParams.cy = cy
							bestParams.px = px
							bestParams.py = py
						}
					}
				}
			}
		}
	}

	fmt.Println("Best parameters for truncation:")
	fmt.Printf("  Size:   %d (%.6f)\n", bestParams.size, bestParams.size.toFloat())
	fmt.Printf("  Center: (%d, %d) (%.6f, %.6f)\n", bestParams.cx, bestParams.cy, bestParams.cx.toFloat(), bestParams.cy.toFloat())
	fmt.Printf("  Pivot:  (%d, %d) (%.6f, %.6f)\n", bestParams.px, bestParams.py, bestParams.px.toFloat(), bestParams.py.toFloat())
	_, perfect := evaluateWithRotation(bestParams.size, bestParams.cx, bestParams.cy, bestParams.px, bestParams.py, targets, rotateTruncated)
	fmt.Printf("  Accuracy: %.2f%% (%d/11 perfect)\n", bestPixel, perfect)
}
