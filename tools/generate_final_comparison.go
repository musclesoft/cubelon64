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
	fmt.Println("🎯 Generating Final Comparison (3 columns)")
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

	// Improved truncation-optimized parameters
	size := Fixed88(3064)
	centerX := Fixed88(3064)
	centerY := Fixed88(2960)
	pivotX := Fixed88(4612)
	pivotY := Fixed88(4488)

	angleStep := math.Pi / 20.0

	fmt.Println("Parameters (improved truncation-optimized):")
	fmt.Printf("  Size:   %d (%.6f) = $%04X\n", size, size.toFloat(), size)
	fmt.Printf("  Center: (%d, %d) = (%.6f, %.6f) = ($%04X, $%04X)\n",
		centerX, centerY, centerX.toFloat(), centerY.toFloat(), centerX, centerY)
	fmt.Printf("  Pivot:  (%d, %d) = (%.6f, %.6f) = ($%04X, $%04X)\n",
		pivotX, pivotY, pivotX.toFloat(), pivotY.toFloat(), pivotX, pivotY)
	fmt.Println()

	totalPixel := 0.0
	perfect := 0

	fmt.Println("Frame-by-frame accuracy:")
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

		rendered := renderQuad(rotated)
		pixel := comparePatterns(targets[frame], rendered)
		totalPixel += pixel

		status := " "
		if pixel == 100.0 {
			perfect++
			status = "✓"
		}

		fmt.Printf("  Frame %02d (%.0f°): %6.2f%% %s\n", frame+1, angle*180/math.Pi, pixel, status)
	}

	fmt.Println()
	fmt.Printf("Overall:  %.2f%% (%d/11 perfect)\n", totalPixel/11.0, perfect)
	fmt.Println()

	fmt.Println("Generating comparison image...")

	// Create 3-column image: Target | Rendered | Difference
	scale := 8
	imgWidth := 24*scale*3 + 2*10
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

		// Column 1: Rendered
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
		rendered := renderQuad(rotated)

		xOffset := 24*scale + 10
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				c := color.RGBA{255, 255, 255, 255}
				if rendered[y][x] {
					c = color.RGBA{0, 0, 0, 255}
				}
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						compImg.Set(xOffset+x*scale+sx, yOffset+y*scale+sy, c)
					}
				}
			}
		}

		// Column 2: Difference
		diff := createDiffImage(targets[frame], rendered)
		xOffset = 24*scale*2 + 20
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				c := diff.At(x, y)
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
	fmt.Println("  3 columns: Target | Rendered | Difference")
	fmt.Println("  (Red = missing pixel, Green = extra pixel, Gray = match)")
}
