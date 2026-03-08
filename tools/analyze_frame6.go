package main

import (
	"fmt"
	"image"
	"image/color"
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

	// Simple truncation - no rounding
	rx := rxTemp >> 8
	ry := ryTemp >> 8

	return Point{
		X: pivot.X + Fixed88(rx),
		Y: pivot.Y + Fixed88(ry),
	}
}

// Render quadrilateral with detailed debugging
func renderQuadDetailed(corners [4]Point) ([][]bool, map[string]interface{}) {
	grid := make([][]bool, 24)
	for i := range grid {
		grid[i] = make([]bool, 24)
	}

	debug := make(map[string]interface{})
	scanlineInfo := make([]map[string]interface{}, 0)

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

		scanInfo := map[string]interface{}{
			"y":            y,
			"intersections": intersections,
			"pixels":       make([]int, 0),
		}

		for i := 0; i+1 < len(intersections); i += 2 {
			x1 := (intersections[i] + 128) >> 8
			x2 := (intersections[i+1] + 128) >> 8

			scanInfo["x1_raw"] = intersections[i]
			scanInfo["x2_raw"] = intersections[i+1]
			scanInfo["x1_rounded"] = x1
			scanInfo["x2_rounded"] = x2

			pixels := make([]int, 0)
			for x := x1; x < x2; x++ {
				if x >= 0 && x < 24 {
					grid[y][x] = true
					pixels = append(pixels, x)
				} else if x >= 24 {
					grid[y][x%24] = true
					pixels = append(pixels, x%24)
				}
			}
			scanInfo["pixels"] = pixels
		}
		scanlineInfo = append(scanlineInfo, scanInfo)
	}

	debug["corners"] = corners
	debug["scanlines"] = scanlineInfo
	return grid, debug
}

func main() {
	fmt.Println("🔍 Analyzing Frame 6 (45°) in Detail")
	fmt.Println()

	// Load frame 6 (45° rotation)
	target, err := loadPattern("../emulator/frames/cycle_06.png")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Truncation-optimized parameters
	size := Fixed88(3072)
	centerX := Fixed88(3072)
	centerY := Fixed88(2944)
	pivotX := Fixed88(4608)
	pivotY := Fixed88(4480)

	// 45° angle
	angle := math.Pi / 4.0
	cos := math.Cos(angle)
	sin := math.Sin(angle)
	cos256 := int32(cos * 256)
	sin256 := int32(sin * 256)

	fmt.Printf("Parameters:\n")
	fmt.Printf("  Size:   %d (%.3f)\n", size, size.toFloat())
	fmt.Printf("  Center: (%d, %d) = (%.3f, %.3f)\n", centerX, centerY, centerX.toFloat(), centerY.toFloat())
	fmt.Printf("  Pivot:  (%d, %d) = (%.3f, %.3f)\n", pivotX, pivotY, pivotX.toFloat(), pivotY.toFloat())
	fmt.Printf("  Angle:  45° (π/4)\n")
	fmt.Printf("  cos256: %d (%.6f)\n", cos256, float64(cos256)/256.0)
	fmt.Printf("  sin256: %d (%.6f)\n", sin256, float64(sin256)/256.0)
	fmt.Println()

	// Render with detailed debugging
	half := size / 2
	initial := [4]Point{
		{centerX - half, centerY - half},
		{centerX + half, centerY - half},
		{centerX + half, centerY + half},
		{centerX - half, centerY + half},
	}
	pivot := Point{pivotX, pivotY}

	fmt.Println("Initial corners (before rotation):")
	for i, p := range initial {
		fmt.Printf("  Corner %d: (%d, %d) = (%.3f, %.3f)\n", i, p.X, p.Y, p.X.toFloat(), p.Y.toFloat())
	}
	fmt.Println()

	var rotated [4]Point
	fmt.Println("Rotated corners:")
	for i := 0; i < 4; i++ {
		rotated[i] = rotateTruncated(initial[i], pivot, cos256, sin256)
		fmt.Printf("  Corner %d: (%d, %d) = (%.3f, %.3f)\n", i, rotated[i].X, rotated[i].Y, rotated[i].X.toFloat(), rotated[i].Y.toFloat())
	}
	fmt.Println()

	rendered, _ := renderQuadDetailed(rotated)

	// Compare and find mismatches
	mismatches := make([]struct{ x, y int; target, rendered bool }, 0)
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if target[y][x] != rendered[y][x] {
				mismatches = append(mismatches, struct{ x, y int; target, rendered bool }{
					x: x, y: y, target: target[y][x], rendered: rendered[y][x],
				})
			}
		}
	}

	fmt.Printf("Total mismatches: %d pixels (%.2f%% accuracy)\n", len(mismatches), (1.0-float64(len(mismatches))/576.0)*100.0)
	fmt.Println()

	if len(mismatches) > 0 {
		fmt.Println("Mismatched pixels:")
		for i, m := range mismatches {
			state := "missing"
			if !m.target && m.rendered {
				state = "extra"
			}
			fmt.Printf("  %2d: (%2d, %2d) - %s pixel (target=%v rendered=%v)\n",
				i+1, m.x, m.y, state, m.target, m.rendered)
		}
		fmt.Println()

		// Analyze pattern of mismatches
		fmt.Println("Mismatch pattern analysis:")

		// Group by type
		missing := 0
		extra := 0
		for _, m := range mismatches {
			if m.target && !m.rendered {
				missing++
			} else {
				extra++
			}
		}
		fmt.Printf("  Missing pixels (should be black): %d\n", missing)
		fmt.Printf("  Extra pixels (should be white): %d\n", extra)
		fmt.Println()

		// Check if mismatches are on edges
		edgeMismatches := 0
		for _, m := range mismatches {
			// Check 8 neighbors
			neighbors := 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					ny := m.y + dy
					nx := m.x + dx
					if ny >= 0 && ny < 24 && nx >= 0 && nx < 24 {
						if target[ny][nx] != rendered[ny][nx] {
							neighbors++
						}
					}
				}
			}
			if neighbors == 0 {
				edgeMismatches++
				fmt.Printf("  Isolated mismatch at (%d, %d)\n", m.x, m.y)
			}
		}
		fmt.Printf("  Edge/boundary mismatches: %d/%d\n", len(mismatches)-edgeMismatches, len(mismatches))
		fmt.Printf("  Isolated mismatches: %d/%d\n", edgeMismatches, len(mismatches))
	}

	// Generate detailed visualization
	scale := 16
	imgWidth := 24*scale*3 + 40
	imgHeight := 24*scale + 40
	img := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))

	// Fill background
	for y := 0; y < imgHeight; y++ {
		for x := 0; x < imgWidth; x++ {
			img.Set(x, y, color.RGBA{40, 40, 40, 255})
		}
	}

	// Column 0: Target
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			c := color.RGBA{255, 255, 255, 255}
			if target[y][x] {
				c = color.RGBA{0, 0, 0, 255}
			}
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					img.Set(20+x*scale+sx, 20+y*scale+sy, c)
				}
			}
		}
	}

	// Column 1: Rendered
	xOffset := 24*scale + 30
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			c := color.RGBA{255, 255, 255, 255}
			if rendered[y][x] {
				c = color.RGBA{0, 0, 0, 255}
			}
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					img.Set(xOffset+x*scale+sx, 20+y*scale+sy, c)
				}
			}
		}
	}

	// Column 2: Difference (highlight mismatches)
	xOffset = 24*scale*2 + 40
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			var c color.RGBA
			if target[y][x] == rendered[y][x] {
				// Match - gray
				c = color.RGBA{128, 128, 128, 255}
			} else if target[y][x] && !rendered[y][x] {
				// Missing - bright red
				c = color.RGBA{255, 0, 0, 255}
			} else {
				// Extra - bright green
				c = color.RGBA{0, 255, 0, 255}
			}
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					img.Set(xOffset+x*scale+sx, 20+y*scale+sy, c)
				}
			}
		}
	}

	// Save image
	outFile, err := os.Create("frame6_analysis.png")
	if err != nil {
		fmt.Printf("Error creating output: %v\n", err)
		return
	}
	defer outFile.Close()

	if err := png.Encode(outFile, img); err != nil {
		fmt.Printf("Error encoding PNG: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✓ Saved frame6_analysis.png")
	fmt.Println("  Columns: Target | Rendered | Difference")
	fmt.Println("  Red = missing pixel, Green = extra pixel, Gray = match")
}
