package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
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

// Rotate point around pivot using 8.8 fixed-point with rounding
func rotateFixed88(p Point, pivot Point, cos256, sin256 int32) Point {
	dx := int32(p.X - pivot.X)
	dy := int32(p.Y - pivot.Y)

	rxTemp := dx*cos256 - dy*sin256
	ryTemp := dx*sin256 + dy*cos256

	// Round to nearest (add 128 if positive, subtract 128 if negative)
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

// Render quadrilateral to 24x24 grid (no wrapping)
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
			// Round intersections to nearest pixel (add 128 = +0.5 before >> 8)
			x1 := (intersections[i] + 128) >> 8
			x2 := (intersections[i+1] + 128) >> 8
			// Fill from x1 (inclusive) to x2 (exclusive) to get correct pixel count
			for x := x1; x < x2; x++ {
				if x >= 0 && x < 24 {
					grid[y][x] = true
				} else if x >= 24 {
					// Wrap to left side
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
	fmt.Println("🎯 Verifying Hardcoded Parameters")
	fmt.Println()

	// Hardcoded parameters (from exhaustive search - 85M combinations tested)
	// Optimal parameters: 99.73% accuracy, 8/11 perfect frames
	// Visually perfect in motion - only ~15 pixels differ across all frames
	size := 11.97265625    // 8.8: 3065
	centerX := 11.7578125  // 8.8: 3010
	centerY := 11.5390625  // 8.8: 2954
	pivotX := 17.8828125   // 8.8: 4578
	pivotY := 17.78125     // 8.8: 4552
	angle0 := 0.0          // Exactly 0°

	half := toFixed88(size / 2.0)
	cx := toFixed88(centerX)
	cy := toFixed88(centerY)

	// Optimal pivot
	pivot := Point{toFixed88(pivotX), toFixed88(pivotY)}

	fmt.Println("Parameters:")
	fmt.Printf("  Size: %.8f x %.8f (8.8: %d)\n", size, size, int(size*256))
	fmt.Printf("  Center: (%.8f, %.8f) (8.8: %d, %d)\n", centerX, centerY, int(centerX*256), int(centerY*256))
	fmt.Printf("  Pivot: (%.8f, %.8f) (8.8: %d, %d) [optimal position]\n", pivotX, pivotY, int(pivotX*256), int(pivotY*256))
	fmt.Printf("  Pivot offset from center: (+%.8f, +%.8f)\n",
		pivotX-centerX, pivotY-centerY)
	fmt.Printf("  Initial angle: %.6f degrees\n", angle0)
	fmt.Printf("  Rotation per frame: %.1f degrees (90° total over 10 steps)\n", 90.0/10.0)
	fmt.Println()

	// Initial square corners (upright)
	initial := [4]Point{
		{cx - half, cy - half}, // Top-left
		{cx + half, cy - half}, // Top-right
		{cx + half, cy + half}, // Bottom-right (also pivot)
		{cx - half, cy + half}, // Bottom-left
	}

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

	// Evaluate each frame
	// 11 frames from 0° to 90° = 10 steps
	angleStep := (math.Pi / 2.0) / 10.0 // 9 degrees per step
	totalAccuracy := 0.0

	fmt.Println("Per-frame results:")
	for frame := 0; frame < 11; frame++ {
		angle := angle0 + float64(frame)*angleStep
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

		status := " "
		if accuracy >= 95.0 {
			status = "✓"
		}
		fmt.Printf("  Frame %02d: %6.2f%% (angle: %5.1f°) %s\n",
			frame+1, accuracy, angle*180/math.Pi, status)
	}

	avgAccuracy := totalAccuracy / 11.0
	fmt.Printf("\nOverall accuracy: %.2f%%\n", avgAccuracy)

	// Generate debug PNG
	fmt.Println("\nGenerating debug PNG...")

	scale := 8
	margin := 2
	frameWidth := 24 * scale
	frameHeight := 24 * scale
	cols := 3 // target, rendered, diff
	rows := 11

	imgWidth := cols*frameWidth + (cols+1)*margin
	imgHeight := rows*frameHeight + (rows+1)*margin

	debugImg := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))

	// Fill background with dark gray
	bgColor := color.RGBA{40, 40, 40, 255}
	for y := 0; y < imgHeight; y++ {
		for x := 0; x < imgWidth; x++ {
			debugImg.Set(x, y, bgColor)
		}
	}

	white := color.RGBA{255, 255, 255, 255}
	black := color.RGBA{0, 0, 0, 255}
	red := color.RGBA{255, 0, 0, 255}       // Extra pixel
	green := color.RGBA{0, 255, 0, 255}     // Missing pixel
	yellow := color.RGBA{255, 255, 0, 255}  // Pivot point

	pivotXInt := int(pivot.X >> 8)
	pivotYInt := int(pivot.Y >> 8)
	debugAngleStep := (math.Pi / 2.0) / 10.0 // 9 degrees per step

	for frame := 0; frame < 11; frame++ {
		angle := angle0 + float64(frame)*debugAngleStep
		cos := math.Cos(angle)
		sin := math.Sin(angle)
		cos256 := int32(cos * 256)
		sin256 := int32(sin * 256)

		var rotated [4]Point
		for i := 0; i < 4; i++ {
			rotated[i] = rotateFixed88(initial[i], pivot, cos256, sin256)
		}

		rendered := renderQuad(rotated)
		target := targets[frame]

		rowY := margin + frame*(frameHeight+margin)

		// Column 0: Target
		col0X := margin
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				c := white
				if target[y][x] {
					c = black
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						debugImg.Set(col0X+x*scale+dx, rowY+y*scale+dy, c)
					}
				}
			}
		}

		// Column 1: Rendered
		col1X := margin + frameWidth + margin
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				c := white
				if rendered[y][x] {
					c = black
				}
				// Mark pivot
				if x == pivotXInt && y == pivotYInt {
					c = yellow
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						debugImg.Set(col1X+x*scale+dx, rowY+y*scale+dy, c)
					}
				}
			}
		}

		// Column 2: Diff
		col2X := margin + 2*(frameWidth+margin)
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				var c color.RGBA
				if x == pivotXInt && y == pivotYInt {
					c = yellow
				} else if rendered[y][x] == target[y][x] {
					// Match - show as gray
					c = color.RGBA{100, 100, 100, 255}
				} else if rendered[y][x] && !target[y][x] {
					// Extra pixel
					c = red
				} else {
					// Missing pixel
					c = green
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						debugImg.Set(col2X+x*scale+dx, rowY+y*scale+dy, c)
					}
				}
			}
		}
	}

	// Save debug image
	debugFile, err := os.Create("debug_hardcoded.png")
	if err != nil {
		fmt.Printf("Error creating debug PNG: %v\n", err)
		return
	}
	defer debugFile.Close()

	if err := png.Encode(debugFile, debugImg); err != nil {
		fmt.Printf("Error encoding debug PNG: %v\n", err)
		return
	}

	fmt.Println("✓ Saved debug_hardcoded.png")
	fmt.Println("  Layout: Target | Rendered (with pivot) | Diff (red=extra, green=missing, yellow=pivot)")

	// Text visualization
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("TEXT VISUALIZATION")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("Legend: · = match, X = extra pixel, O = missing pixel, + = pivot")
	fmt.Println()

	for frame := 0; frame < 11; frame++ {
		angle := angle0 + float64(frame)*angleStep
		cos := math.Cos(angle)
		sin := math.Sin(angle)
		cos256 := int32(cos * 256)
		sin256 := int32(sin * 256)

		var rotated [4]Point
		for i := 0; i < 4; i++ {
			rotated[i] = rotateFixed88(initial[i], pivot, cos256, sin256)
		}

		rendered := renderQuad(rotated)
		target := targets[frame]
		accuracy := comparePatterns(rendered, target)

		fmt.Printf("Frame %02d (%.2f%%, angle %.1f°):\n", frame+1, accuracy, angle*180/math.Pi)
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				if x == pivotXInt && y == pivotYInt {
					fmt.Print("+")
				} else if rendered[y][x] == target[y][x] {
					fmt.Print("·")
				} else if rendered[y][x] && !target[y][x] {
					fmt.Print("X")
				} else {
					fmt.Print("O")
				}
			}
			fmt.Println()
		}
		fmt.Println()
	}
}
