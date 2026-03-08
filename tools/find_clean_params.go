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

// Blur pattern for visual comparison
func blurPattern(pattern [][]bool) [][]float64 {
	blurred := make([][]float64, 24)
	for i := range blurred {
		blurred[i] = make([]float64, 24)
	}

	kernel := [][]float64{
		{0.0625, 0.125, 0.0625},
		{0.125, 0.25, 0.125},
		{0.0625, 0.125, 0.0625},
	}

	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			sum := 0.0
			for ky := -1; ky <= 1; ky++ {
				for kx := -1; kx <= 1; kx++ {
					ny := y + ky
					nx := x + kx
					if ny >= 0 && ny < 24 && nx >= 0 && nx < 24 {
						if pattern[ny][nx] {
							sum += kernel[ky+1][kx+1]
						}
					}
				}
			}
			blurred[y][x] = sum
		}
	}

	return blurred
}

// Visual similarity
func visualSimilarity(a, b [][]bool) float64 {
	blurA := blurPattern(a)
	blurB := blurPattern(b)

	mse := 0.0
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			diff := blurA[y][x] - blurB[y][x]
			mse += diff * diff
		}
	}
	mse /= 576.0

	return (1.0 - math.Sqrt(mse)) * 100.0
}

// Round to nearest multiple
func roundTo(val Fixed88, multiple int) Fixed88 {
	v := int(val)
	remainder := v % multiple
	if remainder >= multiple/2 {
		return Fixed88(v + (multiple - remainder))
	}
	return Fixed88(v - remainder)
}

// Evaluate parameters
func evaluateParams(size, cx, cy, px, py Fixed88, targets [11][][]bool) (float64, float64, int) {
	half := size / 2
	angleStep := math.Pi / 20.0

	totalPixel := 0.0
	totalVisual := 0.0
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
		visual := visualSimilarity(targets[frame], rendered)

		totalPixel += pixel
		totalVisual += visual
		if pixel == 100.0 {
			perfect++
		}
	}

	return totalPixel / 11.0, totalVisual / 11.0, perfect
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	fmt.Println("🔍 Finding Clean Parameters (Round Numbers)")
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

	// Optimal parameters (baseline)
	optSize := Fixed88(3065)
	optCenterX := Fixed88(3010)
	optCenterY := Fixed88(2954)
	optPivotX := Fixed88(4578)
	optPivotY := Fixed88(4552)

	fmt.Println("Optimal parameters (baseline):")
	fmt.Printf("  Size:   %d (%.6f)\n", optSize, optSize.toFloat())
	fmt.Printf("  Center: (%d, %d) (%.6f, %.6f)\n", optCenterX, optCenterY, optCenterX.toFloat(), optCenterY.toFloat())
	fmt.Printf("  Pivot:  (%d, %d) (%.6f, %.6f)\n", optPivotX, optPivotY, optPivotX.toFloat(), optPivotY.toFloat())
	optPixel, optVisual, optPerfect := evaluateParams(optSize, optCenterX, optCenterY, optPivotX, optPivotY, targets)
	fmt.Printf("  Accuracy: %.2f%% pixel, %.2f%% visual (%d/11 perfect)\n", optPixel, optVisual, optPerfect)
	fmt.Println()

	// Test different rounding levels
	roundings := []struct {
		name     string
		multiple int
		desc     string
	}{
		{"Quarter pixel", 64, "0.25 pixel (1/4)"},
		{"Half pixel", 128, "0.5 pixel (1/2)"},
		{"Whole pixel", 256, "1.0 pixel"},
		{"Simple (clean)", 256, "12.0, 12.0, 18.0"},
	}

	fmt.Println("Testing rounded parameters:")
	fmt.Println()

	bestPixel := 0.0
	bestVisual := 0.0
	var bestParams struct {
		size, cx, cy, px, py Fixed88
		name                 string
	}

	for _, r := range roundings {
		if r.name == "Simple (clean)" {
			// Use simple clean params
			size := Fixed88(3072)
			cx := Fixed88(3072)
			cy := Fixed88(3072)
			px := Fixed88(4608)
			py := Fixed88(4608)

			pixel, visual, perfect := evaluateParams(size, cx, cy, px, py, targets)
			fmt.Printf("%s:\n", r.name)
			fmt.Printf("  Size:   %d (%.6f)\n", size, size.toFloat())
			fmt.Printf("  Center: (%d, %d) (%.6f, %.6f)\n", cx, cy, cx.toFloat(), cy.toFloat())
			fmt.Printf("  Pivot:  (%d, %d) (%.6f, %.6f)\n", px, py, px.toFloat(), py.toFloat())
			fmt.Printf("  Accuracy: %.2f%% pixel, %.2f%% visual (%d/11 perfect)\n", pixel, visual, perfect)
			fmt.Printf("  Loss: -%.2f%% pixel, -%.2f%% visual\n", optPixel-pixel, optVisual-visual)
			fmt.Println()

			if visual > bestVisual {
				bestPixel = pixel
				bestVisual = visual
				bestParams.size = size
				bestParams.cx = cx
				bestParams.cy = cy
				bestParams.px = px
				bestParams.py = py
				bestParams.name = r.name
			}
		} else {
			// Round optimal params to nearest multiple
			size := roundTo(optSize, r.multiple)
			cx := roundTo(optCenterX, r.multiple)
			cy := roundTo(optCenterY, r.multiple)
			px := roundTo(optPivotX, r.multiple)
			py := roundTo(optPivotY, r.multiple)

			pixel, visual, perfect := evaluateParams(size, cx, cy, px, py, targets)
			fmt.Printf("%s (%s):\n", r.name, r.desc)
			fmt.Printf("  Size:   %d (%.6f)\n", size, size.toFloat())
			fmt.Printf("  Center: (%d, %d) (%.6f, %.6f)\n", cx, cy, cx.toFloat(), cy.toFloat())
			fmt.Printf("  Pivot:  (%d, %d) (%.6f, %.6f)\n", px, py, px.toFloat(), py.toFloat())
			fmt.Printf("  Accuracy: %.2f%% pixel, %.2f%% visual (%d/11 perfect)\n", pixel, visual, perfect)
			fmt.Printf("  Loss: -%.2f%% pixel, -%.2f%% visual\n", optPixel-pixel, optVisual-visual)
			fmt.Println()

			if visual > bestVisual {
				bestPixel = pixel
				bestVisual = visual
				bestParams.size = size
				bestParams.cx = cx
				bestParams.cy = cy
				bestParams.px = px
				bestParams.py = py
				bestParams.name = r.name
			}
		}
	}

	fmt.Println("==========================================")
	fmt.Println("RECOMMENDATION:")
	fmt.Printf("Best clean parameters: %s\n", bestParams.name)
	fmt.Printf("  Size:   %d (%.6f)\n", bestParams.size, bestParams.size.toFloat())
	fmt.Printf("  Center: (%d, %d) (%.6f, %.6f)\n", bestParams.cx, bestParams.cy, bestParams.cx.toFloat(), bestParams.cy.toFloat())
	fmt.Printf("  Pivot:  (%d, %d) (%.6f, %.6f)\n", bestParams.px, bestParams.py, bestParams.px.toFloat(), bestParams.py.toFloat())
	fmt.Printf("  Accuracy: %.2f%% pixel, %.2f%% visual\n", bestPixel, bestVisual)
	fmt.Printf("  Loss vs optimal: -%.2f%% pixel, -%.2f%% visual\n", optPixel-bestPixel, optVisual-bestVisual)
	fmt.Println("==========================================")
}
