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

// Rotate with rounding (optimal method)
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

// Rotate with truncation (recommended for C64)
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

// Create diff visualization
func createDiffImage(target, rendered [][]bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if target[y][x] == rendered[y][x] {
				img.Set(x, y, color.RGBA{128, 128, 128, 255})
			} else if target[y][x] && !rendered[y][x] {
				img.Set(x, y, color.RGBA{255, 0, 0, 255}) // Missing - RED
			} else {
				img.Set(x, y, color.RGBA{0, 255, 0, 255}) // Extra - GREEN
			}
		}
	}
	return img
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	fmt.Println("🎯 Final Comparison: Optimal vs Truncation-Optimized")
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

	// Optimal parameters (with rounding)
	optSize := Fixed88(3065)
	optCenterX := Fixed88(3010)
	optCenterY := Fixed88(2954)
	optPivotX := Fixed88(4578)
	optPivotY := Fixed88(4552)

	// Truncation-optimized parameters (recommended for C64)
	truncSize := Fixed88(3072)
	truncCenterX := Fixed88(3072)
	truncCenterY := Fixed88(2944)
	truncPivotX := Fixed88(4608)
	truncPivotY := Fixed88(4480)

	angleStep := math.Pi / 20.0

	fmt.Println("Optimal (with rounding):")
	fmt.Printf("  Size:   %d (%.3f) = $%04X\n", optSize, optSize.toFloat(), optSize)
	fmt.Printf("  Center: (%d, %d) (%.3f, %.3f)\n", optCenterX, optCenterY, optCenterX.toFloat(), optCenterY.toFloat())
	fmt.Printf("  Pivot:  (%d, %d) (%.3f, %.3f)\n", optPivotX, optPivotY, optPivotX.toFloat(), optPivotY.toFloat())
	fmt.Println()

	fmt.Println("Truncation-Optimized (RECOMMENDED for C64):")
	fmt.Printf("  Size:   %d (%.3f) = $%04X\n", truncSize, truncSize.toFloat(), truncSize)
	fmt.Printf("  Center: (%d, %d) (%.3f, %.3f) = ($%04X, $%04X)\n", truncCenterX, truncCenterY, truncCenterX.toFloat(), truncCenterY.toFloat(), truncCenterX, truncCenterY)
	fmt.Printf("  Pivot:  (%d, %d) (%.3f, %.3f) = ($%04X, $%04X)\n", truncPivotX, truncPivotY, truncPivotX.toFloat(), truncPivotY.toFloat(), truncPivotX, truncPivotY)
	fmt.Println()

	optPixelAcc := 0.0
	truncPixelAcc := 0.0
	optPerfect := 0
	truncPerfect := 0

	fmt.Println("Frame-by-frame comparison:")
	fmt.Println("Frame | Optimal | Truncation | Δ      | Status")
	fmt.Println("------|---------|------------|--------|--------")

	for frame := 0; frame < 11; frame++ {
		angle := float64(frame) * angleStep
		cos := math.Cos(angle)
		sin := math.Sin(angle)
		cos256 := int32(cos * 256)
		sin256 := int32(sin * 256)

		// Render with optimal (rounding)
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
			optRotated[i] = rotateRounded(optInitial[i], optPivot, cos256, sin256)
		}
		optRendered := renderQuad(optRotated)
		optPixel := comparePatterns(targets[frame], optRendered)
		optPixelAcc += optPixel
		if optPixel == 100.0 {
			optPerfect++
		}

		// Render with truncation-optimized
		truncHalf := truncSize / 2
		truncInitial := [4]Point{
			{truncCenterX - truncHalf, truncCenterY - truncHalf},
			{truncCenterX + truncHalf, truncCenterY - truncHalf},
			{truncCenterX + truncHalf, truncCenterY + truncHalf},
			{truncCenterX - truncHalf, truncCenterY + truncHalf},
		}
		truncPivot := Point{truncPivotX, truncPivotY}
		var truncRotated [4]Point
		for i := 0; i < 4; i++ {
			truncRotated[i] = rotateTruncated(truncInitial[i], truncPivot, cos256, sin256)
		}
		truncRendered := renderQuad(truncRotated)
		truncPixel := comparePatterns(targets[frame], truncRendered)
		truncPixelAcc += truncPixel
		if truncPixel == 100.0 {
			truncPerfect++
		}

		status := ""
		if optPixel == 100.0 {
			status += "✓"
		} else {
			status += " "
		}
		if truncPixel == 100.0 {
			status += "✓"
		} else {
			status += " "
		}

		fmt.Printf("  %02d  | %6.2f%% | %6.2f%%   | %+.2f%% | %s\n",
			frame+1, optPixel, truncPixel, truncPixel-optPixel, status)
	}

	fmt.Println()
	fmt.Printf("Overall:  %6.2f%% | %6.2f%%   | %+.2f%%\n",
		optPixelAcc/11.0, truncPixelAcc/11.0, (truncPixelAcc-optPixelAcc)/11.0)
	fmt.Printf("Perfect:  %d/11    | %d/11\n", optPerfect, truncPerfect)
	fmt.Println()

	fmt.Println("Generating comparison image...")

	// Create 5-column image: Target | Optimal | Trunc | Opt-Diff | Trunc-Diff
	scale := 8
	imgWidth := 24*scale*5 + 4*10
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

		// Column 1: Optimal render
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
			optRotated[i] = rotateRounded(optInitial[i], optPivot, cos256, sin256)
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

		// Column 2: Truncation render
		truncHalf := truncSize / 2
		truncInitial := [4]Point{
			{truncCenterX - truncHalf, truncCenterY - truncHalf},
			{truncCenterX + truncHalf, truncCenterY - truncHalf},
			{truncCenterX + truncHalf, truncCenterY + truncHalf},
			{truncCenterX - truncHalf, truncCenterY + truncHalf},
		}
		truncPivot := Point{truncPivotX, truncPivotY}
		var truncRotated [4]Point
		for i := 0; i < 4; i++ {
			truncRotated[i] = rotateTruncated(truncInitial[i], truncPivot, cos256, sin256)
		}
		truncRendered := renderQuad(truncRotated)

		xOffset = 24*scale*2 + 20
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				c := color.RGBA{255, 255, 255, 255}
				if truncRendered[y][x] {
					c = color.RGBA{0, 0, 0, 255}
				}
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						compImg.Set(xOffset+x*scale+sx, yOffset+y*scale+sy, c)
					}
				}
			}
		}

		// Column 3: Optimal diff
		optDiff := createDiffImage(targets[frame], optRendered)
		xOffset = 24*scale*3 + 30
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

		// Column 4: Truncation diff
		truncDiff := createDiffImage(targets[frame], truncRendered)
		xOffset = 24*scale*4 + 40
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				c := truncDiff.At(x, y)
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						compImg.Set(xOffset+x*scale+sx, yOffset+y*scale+sy, c)
					}
				}
			}
		}
	}

	// Save image
	outFile, err := os.Create("final_comparison.png")
	if err != nil {
		fmt.Printf("Error creating output: %v\n", err)
		return
	}
	defer outFile.Close()

	if err := png.Encode(outFile, compImg); err != nil {
		fmt.Printf("Error encoding PNG: %v\n", err)
		return
	}

	fmt.Println("✓ Saved final_comparison.png")
	fmt.Println("  Columns: Target | Optimal | Truncation-Optimized | Opt-Diff | Trunc-Diff")
	fmt.Println()
	fmt.Println("RECOMMENDATION: Use Truncation-Optimized for C64")
	fmt.Println("  - Only -0.06% accuracy loss")
	fmt.Println("  - No rounding needed (just use high byte)")
	fmt.Println("  - Clean parameter values")
	fmt.Println("  - Simpler 6502 code")
}
