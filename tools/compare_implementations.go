package main

import (
	"fmt"
	"image"
	"image/color"
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

// Compare patterns (pixel accuracy)
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

	// Simple 3x3 gaussian blur
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

// Visual similarity (blur-based perceptual metric)
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

	// Convert MSE to percentage similarity
	// MSE of 0 = 100% similar, MSE of 1 = 0% similar
	return (1.0 - math.Sqrt(mse)) * 100.0
}

// Create diff visualization
func createDiffImage(target, rendered [][]bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if target[y][x] == rendered[y][x] {
				// Match - show as gray
				img.Set(x, y, color.RGBA{128, 128, 128, 255})
			} else if target[y][x] && !rendered[y][x] {
				// Should be black, rendered white - RED (missing pixel)
				img.Set(x, y, color.RGBA{255, 0, 0, 255})
			} else {
				// Should be white, rendered black - GREEN (extra pixel)
				img.Set(x, y, color.RGBA{0, 255, 0, 255})
			}
		}
	}
	return img
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	fmt.Println("🔍 Comparing Implementations: Optimized vs Simple")
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

	// Optimized parameters (from exhaustive search)
	optSize := Fixed88(3065)
	optCenterX := Fixed88(3010)
	optCenterY := Fixed88(2954)
	optPivotX := Fixed88(4578)
	optPivotY := Fixed88(4552)

	// Quarter-pixel parameters (clean middle-ground)
	quarterSize := Fixed88(3072)    // Exactly 12.0 pixels
	quarterCenterX := Fixed88(3008) // 11.75
	quarterCenterY := Fixed88(2944) // 11.50
	quarterPivotX := Fixed88(4608)  // Exactly 18.0
	quarterPivotY := Fixed88(4544)  // 17.75

	// Simple parameters (clean 8-bit math)
	simpleSize := Fixed88(3072)    // Exactly 12.0 pixels
	simpleCenterX := Fixed88(3072) // Exactly 12.0
	simpleCenterY := Fixed88(3072) // Exactly 12.0
	simplePivotX := Fixed88(4608)  // Exactly 18.0 (6 pixels offset)
	simplePivotY := Fixed88(4608)  // Exactly 18.0

	angleStep := math.Pi / 20.0 // Exactly 9 degrees

	fmt.Println("Optimized parameters (99.73% accurate):")
	fmt.Printf("  Size:   %d (%.6f)\n", optSize, optSize.toFloat())
	fmt.Printf("  Center: (%d, %d) (%.6f, %.6f)\n", optCenterX, optCenterY, optCenterX.toFloat(), optCenterY.toFloat())
	fmt.Printf("  Pivot:  (%d, %d) (%.6f, %.6f)\n", optPivotX, optPivotY, optPivotX.toFloat(), optPivotY.toFloat())
	fmt.Println()

	fmt.Println("Quarter-pixel parameters (clean middle-ground):")
	fmt.Printf("  Size:   %d (%.6f)\n", quarterSize, quarterSize.toFloat())
	fmt.Printf("  Center: (%d, %d) (%.6f, %.6f)\n", quarterCenterX, quarterCenterY, quarterCenterX.toFloat(), quarterCenterY.toFloat())
	fmt.Printf("  Pivot:  (%d, %d) (%.6f, %.6f)\n", quarterPivotX, quarterPivotY, quarterPivotX.toFloat(), quarterPivotY.toFloat())
	fmt.Println()

	fmt.Println("Simple parameters (clean 8-bit math):")
	fmt.Printf("  Size:   %d (%.6f)\n", simpleSize, simpleSize.toFloat())
	fmt.Printf("  Center: (%d, %d) (%.6f, %.6f)\n", simpleCenterX, simpleCenterY, simpleCenterX.toFloat(), simpleCenterY.toFloat())
	fmt.Printf("  Pivot:  (%d, %d) (%.6f, %.6f)\n", simplePivotX, simplePivotY, simplePivotX.toFloat(), simplePivotY.toFloat())
	fmt.Println()

	// Evaluate all three implementations
	optPixelAcc := 0.0
	optVisualAcc := 0.0
	quarterPixelAcc := 0.0
	quarterVisualAcc := 0.0
	simplePixelAcc := 0.0
	simpleVisualAcc := 0.0
	optPerfect := 0
	quarterPerfect := 0
	simplePerfect := 0

	fmt.Println("Frame-by-frame comparison:")
	fmt.Println("Frame | Opt Pix% | Opt Vis% | Qtr Pix% | Qtr Vis% | Sim Pix% | Sim Vis% | Status")
	fmt.Println("------|----------|----------|----------|----------|----------|----------|--------")

	for frame := 0; frame < 11; frame++ {
		angle := float64(frame) * angleStep
		cos := math.Cos(angle)
		sin := math.Sin(angle)
		cos256 := int32(cos * 256)
		sin256 := int32(sin * 256)

		// Render with optimized params
		optHalf := optSize / 2
		optInitial := [4]Point{
			{optCenterX - optHalf, optCenterY - optHalf},
			{optCenterX + optHalf, optCenterY - optHalf},
			{optCenterX + optHalf, optCenterY + optHalf},
			{optCenterX - optHalf, optCenterY + optHalf},
		}
		optPivot := Point{optPivotX, optPivotY}
		var optRotated [4]Point
		for i := 0; i < 4; i++ {
			optRotated[i] = rotateFixed88(optInitial[i], optPivot, cos256, sin256)
		}
		optRendered := renderQuad(optRotated)
		optPixel := comparePatterns(targets[frame], optRendered)
		optVisual := visualSimilarity(targets[frame], optRendered)
		optPixelAcc += optPixel
		optVisualAcc += optVisual
		if optPixel == 100.0 {
			optPerfect++
		}

		// Render with quarter-pixel params
		quarterHalf := quarterSize / 2
		quarterInitial := [4]Point{
			{quarterCenterX - quarterHalf, quarterCenterY - quarterHalf},
			{quarterCenterX + quarterHalf, quarterCenterY - quarterHalf},
			{quarterCenterX + quarterHalf, quarterCenterY + quarterHalf},
			{quarterCenterX - quarterHalf, quarterCenterY + quarterHalf},
		}
		quarterPivot := Point{quarterPivotX, quarterPivotY}
		var quarterRotated [4]Point
		for i := 0; i < 4; i++ {
			quarterRotated[i] = rotateFixed88(quarterInitial[i], quarterPivot, cos256, sin256)
		}
		quarterRendered := renderQuad(quarterRotated)
		quarterPixel := comparePatterns(targets[frame], quarterRendered)
		quarterVisual := visualSimilarity(targets[frame], quarterRendered)
		quarterPixelAcc += quarterPixel
		quarterVisualAcc += quarterVisual
		if quarterPixel == 100.0 {
			quarterPerfect++
		}

		// Render with simple params
		simpleHalf := simpleSize / 2
		simpleInitial := [4]Point{
			{simpleCenterX - simpleHalf, simpleCenterY - simpleHalf},
			{simpleCenterX + simpleHalf, simpleCenterY - simpleHalf},
			{simpleCenterX + simpleHalf, simpleCenterY + simpleHalf},
			{simpleCenterX - simpleHalf, simpleCenterY + simpleHalf},
		}
		simplePivot := Point{simplePivotX, simplePivotY}
		var simpleRotated [4]Point
		for i := 0; i < 4; i++ {
			simpleRotated[i] = rotateFixed88(simpleInitial[i], simplePivot, cos256, sin256)
		}
		simpleRendered := renderQuad(simpleRotated)
		simplePixel := comparePatterns(targets[frame], simpleRendered)
		simpleVisual := visualSimilarity(targets[frame], simpleRendered)
		simplePixelAcc += simplePixel
		simpleVisualAcc += simpleVisual
		if simplePixel == 100.0 {
			simplePerfect++
		}

		status := ""
		if optPixel == 100.0 {
			status += "✓"
		} else {
			status += " "
		}
		if quarterPixel == 100.0 {
			status += "✓"
		} else {
			status += " "
		}
		if simplePixel == 100.0 {
			status += "✓"
		} else {
			status += " "
		}

		fmt.Printf("  %02d  |  %.2f%% |  %.2f%% |  %.2f%% |  %.2f%% |  %.2f%% |  %.2f%% |  %s\n",
			frame+1, optPixel, optVisual, quarterPixel, quarterVisual, simplePixel, simpleVisual, status)
	}

	fmt.Println()
	fmt.Println("Summary:")
	fmt.Printf("Optimized:     %.2f%% pixel, %.2f%% visual (%d/11 perfect)\n",
		optPixelAcc/11.0, optVisualAcc/11.0, optPerfect)
	fmt.Printf("Quarter-pixel: %.2f%% pixel, %.2f%% visual (%d/11 perfect)\n",
		quarterPixelAcc/11.0, quarterVisualAcc/11.0, quarterPerfect)
	fmt.Printf("Simple:        %.2f%% pixel, %.2f%% visual (%d/11 perfect)\n",
		simplePixelAcc/11.0, simpleVisualAcc/11.0, simplePerfect)
	fmt.Println()

	fmt.Println("Generating comparison image...")

	// Create 7-column comparison image
	scale := 8
	imgWidth := 24*scale*7 + 6*10 // 7 columns with 10px gaps
	imgHeight := 24*scale*11 + 10*10
	compImg := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))

	// Fill background
	for y := 0; y < imgHeight; y++ {
		for x := 0; x < imgWidth; x++ {
			compImg.Set(x, y, color.RGBA{40, 40, 40, 255})
		}
	}

	for frame := 0; frame < 11; frame++ {
		angle := float64(frame) * angleStep
		cos := math.Cos(angle)
		sin := math.Sin(angle)
		cos256 := int32(cos * 256)
		sin256 := int32(sin * 256)

		yOffset := frame*(24*scale+10) + 10

		// Column 0: Target
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				c := color.RGBA{255, 255, 255, 255}
				if targets[frame][y][x] {
					c = color.RGBA{0, 0, 0, 255}
				}
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						compImg.Set(x*scale+sx, yOffset+y*scale+sy, c)
					}
				}
			}
		}

		// Column 1: Optimized render
		optHalf := optSize / 2
		optInitial := [4]Point{
			{optCenterX - optHalf, optCenterY - optHalf},
			{optCenterX + optHalf, optCenterY - optHalf},
			{optCenterX + optHalf, optCenterY + optHalf},
			{optCenterX - optHalf, optCenterY + optHalf},
		}
		optPivot := Point{optPivotX, optPivotY}
		var optRotated [4]Point
		for i := 0; i < 4; i++ {
			optRotated[i] = rotateFixed88(optInitial[i], optPivot, cos256, sin256)
		}
		optRendered := renderQuad(optRotated)

		xOffset := 24*scale + 10
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				c := color.RGBA{255, 255, 255, 255}
				if optRendered[y][x] {
					c = color.RGBA{0, 0, 0, 255}
				}
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						compImg.Set(xOffset+x*scale+sx, yOffset+y*scale+sy, c)
					}
				}
			}
		}

		// Column 2: Quarter-pixel render
		quarterHalf := quarterSize / 2
		quarterInitial := [4]Point{
			{quarterCenterX - quarterHalf, quarterCenterY - quarterHalf},
			{quarterCenterX + quarterHalf, quarterCenterY - quarterHalf},
			{quarterCenterX + quarterHalf, quarterCenterY + quarterHalf},
			{quarterCenterX - quarterHalf, quarterCenterY + quarterHalf},
		}
		quarterPivot := Point{quarterPivotX, quarterPivotY}
		var quarterRotated [4]Point
		for i := 0; i < 4; i++ {
			quarterRotated[i] = rotateFixed88(quarterInitial[i], quarterPivot, cos256, sin256)
		}
		quarterRendered := renderQuad(quarterRotated)

		xOffset = 24*scale*2 + 20
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				c := color.RGBA{255, 255, 255, 255}
				if quarterRendered[y][x] {
					c = color.RGBA{0, 0, 0, 255}
				}
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						compImg.Set(xOffset+x*scale+sx, yOffset+y*scale+sy, c)
					}
				}
			}
		}

		// Column 3: Simple render
		simpleHalf := simpleSize / 2
		simpleInitial := [4]Point{
			{simpleCenterX - simpleHalf, simpleCenterY - simpleHalf},
			{simpleCenterX + simpleHalf, simpleCenterY - simpleHalf},
			{simpleCenterX + simpleHalf, simpleCenterY + simpleHalf},
			{simpleCenterX - simpleHalf, simpleCenterY + simpleHalf},
		}
		simplePivot := Point{simplePivotX, simplePivotY}
		var simpleRotated [4]Point
		for i := 0; i < 4; i++ {
			simpleRotated[i] = rotateFixed88(simpleInitial[i], simplePivot, cos256, sin256)
		}
		simpleRendered := renderQuad(simpleRotated)

		xOffset = 24*scale*3 + 30
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				c := color.RGBA{255, 255, 255, 255}
				if simpleRendered[y][x] {
					c = color.RGBA{0, 0, 0, 255}
				}
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						compImg.Set(xOffset+x*scale+sx, yOffset+y*scale+sy, c)
					}
				}
			}
		}

		// Column 4: Optimized diff
		optDiff := createDiffImage(targets[frame], optRendered)
		xOffset = 24*scale*4 + 40
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				c := optDiff.At(x, y)
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						compImg.Set(xOffset+x*scale+sx, yOffset+y*scale+sy, c)
					}
				}
			}
		}

		// Column 5: Quarter-pixel diff
		quarterDiff := createDiffImage(targets[frame], quarterRendered)
		xOffset = 24*scale*5 + 50
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				c := quarterDiff.At(x, y)
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						compImg.Set(xOffset+x*scale+sx, yOffset+y*scale+sy, c)
					}
				}
			}
		}

		// Column 6: Simple diff
		simpleDiff := createDiffImage(targets[frame], simpleRendered)
		xOffset = 24*scale*6 + 60
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				c := simpleDiff.At(x, y)
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						compImg.Set(xOffset+x*scale+sx, yOffset+y*scale+sy, c)
					}
				}
			}
		}
	}

	// Save comparison image
	outFile, err := os.Create("implementation_comparison.png")
	if err != nil {
		fmt.Printf("Error creating output: %v\n", err)
		return
	}
	defer outFile.Close()

	if err := png.Encode(outFile, compImg); err != nil {
		fmt.Printf("Error encoding PNG: %v\n", err)
		return
	}

	fmt.Println("✓ Saved implementation_comparison.png")
	fmt.Println("  Columns: Target | Optimized | Quarter-Pixel | Simple | Opt-Diff | Qtr-Diff | Sim-Diff")
}
