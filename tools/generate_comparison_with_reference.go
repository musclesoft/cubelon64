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

// Render at high resolution for reference (16x16 = 384x384) using float
func renderQuadSuperHighRes(corners [4]Point) [][]bool {
	grid := make([][]bool, 384)
	for i := range grid {
		grid[i] = make([]bool, 384)
	}

	// Convert corners to float in pixel space
	type PointF struct{ X, Y float64 }
	var cornersF [4]PointF
	for i := 0; i < 4; i++ {
		cornersF[i] = PointF{
			X: corners[i].X.toFloat(),
			Y: corners[i].Y.toFloat(),
		}
	}

	// Find Y range
	minY, maxY := cornersF[0].Y, cornersF[0].Y
	for i := 1; i < 4; i++ {
		if cornersF[i].Y < minY {
			minY = cornersF[i].Y
		}
		if cornersF[i].Y > maxY {
			maxY = cornersF[i].Y
		}
	}

	// Render at 384x384 (16 sub-pixels per output pixel)
	for y := 0; y < 384; y++ {
		// Convert sub-pixel y to pixel coordinate in 24-pixel space
		yPos := float64(y) / 16.0

		if yPos < minY || yPos > maxY {
			continue
		}

		var intersections []float64
		for i := 0; i < 4; i++ {
			p1 := cornersF[i]
			p2 := cornersF[(i+1)%4]

			if (p1.Y <= yPos && p2.Y > yPos) || (p2.Y <= yPos && p1.Y > yPos) {
				dy := p2.Y - p1.Y
				if math.Abs(dy) > 1e-9 {
					t := (yPos - p1.Y) / dy
					x := p1.X + (p2.X-p1.X)*t
					intersections = append(intersections, x)
				}
			}
		}

		// Sort intersections
		for i := 0; i < len(intersections)-1; i++ {
			for j := i + 1; j < len(intersections); j++ {
				if intersections[i] > intersections[j] {
					intersections[i], intersections[j] = intersections[j], intersections[i]
				}
			}
		}

		// Fill between pairs
		for i := 0; i+1 < len(intersections); i += 2 {
			// Convert from 24-pixel space to 384-pixel space
			x1 := int(math.Floor(intersections[i] * 16.0))
			x2 := int(math.Ceil(intersections[i+1] * 16.0))
			for x := x1; x < x2; x++ {
				if x >= 0 && x < 384 {
					grid[y][x] = true
				} else {
					grid[y][(x%384+384)%384] = true
				}
			}
		}
	}

	return grid
}

// Render with 16x16 supersampling (256 gray levels)
func renderQuadReference(corners [4]Point) [][]uint8 {
	highRes := renderQuadSuperHighRes(corners)

	grid := make([][]uint8, 24)
	for y := 0; y < 24; y++ {
		grid[y] = make([]uint8, 24)
		for x := 0; x < 24; x++ {
			// Count filled sub-pixels in 16x16 block
			count := 0
			for sy := 0; sy < 16; sy++ {
				for sx := 0; sx < 16; sx++ {
					if highRes[y*16+sy][x*16+sx] {
						count++
					}
				}
			}
			// Map to 0-255 grayscale
			grid[y][x] = uint8(count * 255 / 256)
		}
	}

	return grid
}

// Render at 2x resolution
func renderQuadHighRes(corners [4]Point) [][]bool {
	grid := make([][]bool, 48)
	for i := range grid {
		grid[i] = make([]bool, 48)
	}

	var scaledCorners [4]Point
	for i := 0; i < 4; i++ {
		scaledCorners[i] = Point{
			X: corners[i].X << 1,
			Y: corners[i].Y << 1,
		}
	}

	minY, maxY := 48*256, -1
	for _, c := range scaledCorners {
		y := int(c.Y)
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}

	for y := minY >> 8; y <= (maxY>>8) && y < 48; y++ {
		if y < 0 {
			continue
		}
		yFixed := Fixed88(y << 8)

		var intersections []int
		for i := 0; i < 4; i++ {
			p1 := scaledCorners[i]
			p2 := scaledCorners[(i+1)%4]

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
				if x >= 0 && x < 48 {
					grid[y][x] = true
				} else {
					grid[y][(x%48+48)%48] = true
				}
			}
		}
	}

	return grid
}

// Render with 2x2 supersampling and threshold
func renderQuadAntialiasedThreshold(corners [4]Point, threshold int) [][]uint8 {
	highRes := renderQuadHighRes(corners)

	grid := make([][]uint8, 24)
	for y := 0; y < 24; y++ {
		grid[y] = make([]uint8, 24)
		for x := 0; x < 24; x++ {
			count := 0
			if highRes[y*2][x*2] {
				count++
			}
			if highRes[y*2][x*2+1] {
				count++
			}
			if highRes[y*2+1][x*2] {
				count++
			}
			if highRes[y*2+1][x*2+1] {
				count++
			}

			// Store as marker values: 0=$0, 68=$B, 108=$C, 149=$F
			switch threshold {
			case 0: // 0-B-F default: 0→$0, 1-2→$B, 3-4→$F
				if count >= 3 {
					grid[y][x] = 149
				} else if count >= 1 {
					grid[y][x] = 68
				} else {
					grid[y][x] = 0
				}
			case 1: // 0-B-F aggressive: 0→$0, 1→$B, 2-4→$F
				if count >= 2 {
					grid[y][x] = 149
				} else if count >= 1 {
					grid[y][x] = 68
				} else {
					grid[y][x] = 0
				}
			case 2: // 0-B-F conservative: 0-1→$0, 2-3→$B, 4→$F
				if count >= 4 {
					grid[y][x] = 149
				} else if count >= 2 {
					grid[y][x] = 68
				} else {
					grid[y][x] = 0
				}
			case 3: // 0-C-F default: 0→$0, 1-2→$C, 3-4→$F
				if count >= 3 {
					grid[y][x] = 149
				} else if count >= 1 {
					grid[y][x] = 108
				} else {
					grid[y][x] = 0
				}
			case 4: // 0-C-F aggressive: 0→$0, 1→$C, 2-4→$F
				if count >= 2 {
					grid[y][x] = 149
				} else if count >= 1 {
					grid[y][x] = 108
				} else {
					grid[y][x] = 0
				}
			case 5: // 0-C-F conservative: 0-1→$0, 2-3→$C, 4→$F
				if count >= 4 {
					grid[y][x] = 149
				} else if count >= 2 {
					grid[y][x] = 108
				} else {
					grid[y][x] = 0
				}
			case 6: // 0-B-F: 0→$0, 1-3→$B, 4→$F
				if count >= 4 {
					grid[y][x] = 149
				} else if count >= 1 {
					grid[y][x] = 68
				} else {
					grid[y][x] = 0
				}
			case 7: // 0-B-F: 0-1→$0, 2→$B, 3-4→$F
				if count >= 3 {
					grid[y][x] = 149
				} else if count >= 2 {
					grid[y][x] = 68
				} else {
					grid[y][x] = 0
				}
			case 8: // 0-B-F: 0-2→$0, 3→$B, 4→$F
				if count >= 4 {
					grid[y][x] = 149
				} else if count >= 3 {
					grid[y][x] = 68
				} else {
					grid[y][x] = 0
				}
			case 9: // 0-C-F: 0→$0, 1-3→$C, 4→$F
				if count >= 4 {
					grid[y][x] = 149
				} else if count >= 1 {
					grid[y][x] = 108
				} else {
					grid[y][x] = 0
				}
			case 10: // 0-C-F: 0-1→$0, 2→$C, 3-4→$F
				if count >= 3 {
					grid[y][x] = 149
				} else if count >= 2 {
					grid[y][x] = 108
				} else {
					grid[y][x] = 0
				}
			case 11: // 0-C-F: 0-2→$0, 3→$C, 4→$F
				if count >= 4 {
					grid[y][x] = 149
				} else if count >= 3 {
					grid[y][x] = 108
				} else {
					grid[y][x] = 0
				}
			}
		}
	}

	return grid
}

// Calculate visual error between two grids
func calculateError(grid [][]uint8, reference [][]uint8) float64 {
	totalError := 0.0
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			// Grid values are already in C64 color space (0, 68, 108, 149)
			quantizedColor := float64(grid[y][x])
			// Scale reference from 0-255 to C64 range 0-149
			refColor := float64(reference[y][x]) * 149.0 / 255.0
			diff := quantizedColor - refColor
			totalError += diff * diff
		}
	}
	return math.Sqrt(totalError / (24 * 24))
}

func main() {
	fmt.Println("🎯 Generating Comparison with Reference + Error Analysis")
	fmt.Println()

	// Perfect 12x12 square geometry
	sizeAA := Fixed88(3072)   // Exactly 12.0 pixels
	centerAA := Fixed88(3072) // Exactly 12.0 pixels
	angleStep := math.Pi / 20.0

	halfAA := sizeAA / 2
	initial := [4]Point{
		{centerAA - halfAA, centerAA - halfAA},
		{centerAA + halfAA, centerAA - halfAA},
		{centerAA + halfAA, centerAA + halfAA},
		{centerAA - halfAA, centerAA + halfAA},
	}
	// Perfect corner pivot
	pivot := Point{centerAA + halfAA, centerAA + halfAA} // (18.0, 18.0)

	// Rendering offset to optimize rasterization
	renderOffsetX := Fixed88(0)
	renderOffsetY := Fixed88(-4) // -0.015625 pixels

	// Render all frames for all variations
	var references [11][][]uint8
	var variants [12][11][][]uint8 // 12 variations, 11 frames each
	var errors [11][12]float64      // [frame][variant]

	variantNames := []string{
		"0,B,B,F,F", "0,B,F,F,F", "0,0,B,B,F",
		"0,C,C,F,F", "0,C,F,F,F", "0,0,C,C,F",
		"0,B,B,B,F", "0,0,B,F,F", "0,0,0,B,F",
		"0,C,C,C,F", "0,0,C,F,F", "0,0,0,C,F",
	}

	fmt.Println("Rendering frames and computing errors...")
	for frame := 0; frame < 11; frame++ {
		angle := float64(frame) * angleStep
		cos := math.Cos(angle)
		sin := math.Sin(angle)
		cos256 := int32(cos * 256)
		sin256 := int32(sin * 256)

		var rotated [4]Point
		for i := 0; i < 4; i++ {
			rotated[i] = rotateTruncated(initial[i], pivot, cos256, sin256)
		}

		// Apply rendering offset to optimize rasterization
		var offsetCorners [4]Point
		for i := 0; i < 4; i++ {
			offsetCorners[i] = Point{
				X: rotated[i].X + renderOffsetX,
				Y: rotated[i].Y + renderOffsetY,
			}
		}

		references[frame] = renderQuadReference(offsetCorners)

		// Render all 12 variants
		for v := 0; v < 12; v++ {
			variants[v][frame] = renderQuadAntialiasedThreshold(offsetCorners, v)
			errors[frame][v] = calculateError(variants[v][frame], references[frame])
		}

		// Find best for this frame
		best := 0
		for v := 1; v < 12; v++ {
			if errors[frame][v] < errors[frame][best] {
				best = v
			}
		}

		fmt.Printf("  Frame %2d (%3.0f°): Errors: ", frame+1, angle*180/math.Pi)
		for v := 0; v < 12; v++ {
			if v > 0 && v%6 == 0 {
				fmt.Printf("\n                         ")
			}
			fmt.Printf("%s=%.2f ", variantNames[v], errors[frame][v])
		}
		fmt.Printf("→ Best: %s\n", variantNames[best])
	}

	// Compute median errors (copy errors for each variant and sort)
	medianErrors := [12]float64{}
	for v := 0; v < 12; v++ {
		// Collect errors for this variant across all frames
		variantErrors := make([]float64, 11)
		for frame := 0; frame < 11; frame++ {
			variantErrors[frame] = errors[frame][v]
		}
		// Sort to find median
		for i := 0; i < len(variantErrors)-1; i++ {
			for j := i + 1; j < len(variantErrors); j++ {
				if variantErrors[i] > variantErrors[j] {
					variantErrors[i], variantErrors[j] = variantErrors[j], variantErrors[i]
				}
			}
		}
		// Median is the middle value (11 frames, so index 5)
		medianErrors[v] = variantErrors[5]
	}

	fmt.Println()
	fmt.Printf("Median RMS errors:\n")
	for v := 0; v < 12; v++ {
		fmt.Printf("  %-12s %.3f\n", variantNames[v]+":", medianErrors[v])
	}

	// Find overall best
	overallBest := 0
	for v := 1; v < 12; v++ {
		if medianErrors[v] < medianErrors[overallBest] {
			overallBest = v
		}
	}
	fmt.Printf("  → Overall best: %s\n", variantNames[overallBest])
	fmt.Println()

	// Sort variants by quality (best to worst)
	type variantRank struct {
		idx   int
		error float64
	}
	ranks := make([]variantRank, 12)
	for v := 0; v < 12; v++ {
		ranks[v] = variantRank{idx: v, error: medianErrors[v]}
	}
	// Sort by error (ascending)
	for i := 0; i < len(ranks)-1; i++ {
		for j := i + 1; j < len(ranks); j++ {
			if ranks[j].error < ranks[i].error {
				ranks[i], ranks[j] = ranks[j], ranks[i]
			}
		}
	}

	fmt.Println("Column order (sorted by quality):")
	for i, r := range ranks {
		fmt.Printf("  Col %2d: %s (%.3f)\n", i+2, variantNames[r.idx], r.error)
	}
	fmt.Println()

	// Create image with color-coded borders
	scale := 8
	borderSize := 3
	cellWidth := 24*scale + 2*borderSize
	cellHeight := 24*scale + 2*borderSize
	imgWidth := cellWidth * 13 // Reference + 12 variants
	imgHeight := cellHeight * 11

	compImg := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))

	// Fill background
	for y := 0; y < imgHeight; y++ {
		for x := 0; x < imgWidth; x++ {
			compImg.Set(x, y, color.RGBA{40, 40, 40, 255})
		}
	}

	for frame := 0; frame < 11; frame++ {
		yBase := frame * cellHeight

		// Find best and worst for this frame for color coding
		best := 0
		worst := 0
		for v := 1; v < 12; v++ {
			if errors[frame][v] < errors[frame][best] {
				best = v
			}
			if errors[frame][v] > errors[frame][worst] {
				worst = v
			}
		}

		// Helper to get border color (green=best, red=worst, gradient between)
		getBorderColor := func(variant int) color.RGBA {
			if variant == best {
				return color.RGBA{0, 255, 0, 255} // Green for best
			} else if variant == worst {
				return color.RGBA{255, 0, 0, 255} // Red for worst
			} else {
				// Gradient from yellow to orange for middle values
				rank := 0
				for v := 0; v < 12; v++ {
					if v != best && v != worst && errors[frame][v] < errors[frame][variant] {
						rank++
					}
				}
				// Yellow to orange gradient
				return color.RGBA{255, 255 - uint8(rank*20), 0, 255}
			}
		}

		// Render each column
		for col := 0; col < 13; col++ {
			xBase := col * cellWidth

			var grid [][]uint8
			var borderColor color.RGBA

			if col == 0 {
				// Reference
				grid = references[frame]
				borderColor = color.RGBA{128, 128, 255, 255} // Blue for reference
			} else {
				// Variant columns (sorted by quality)
				sortedIdx := col - 1
				variantIdx := ranks[sortedIdx].idx
				grid = variants[variantIdx][frame]
				borderColor = getBorderColor(variantIdx)
			}

			// Draw border
			for by := 0; by < cellHeight; by++ {
				for bx := 0; bx < cellWidth; bx++ {
					if bx < borderSize || bx >= cellWidth-borderSize ||
						by < borderSize || by >= cellHeight-borderSize {
						compImg.Set(xBase+bx, yBase+by, borderColor)
					}
				}
			}

			// Draw grid content
			for y := 0; y < 24; y++ {
				for x := 0; x < 24; x++ {
					val := grid[y][x]
					var c color.RGBA

					if col == 0 {
						// Reference: map 0-255 to C64 color range 0-149
						scaled := uint8(int(val) * 149 / 255)
						c = color.RGBA{scaled, scaled, scaled, 255}
					} else {
						// Quantized columns: map stored values to C64 colors
						switch val {
						case 0:
							c = color.RGBA{0, 0, 0, 255} // $0
						case 68:
							c = color.RGBA{68, 68, 68, 255} // $B
						case 108:
							c = color.RGBA{108, 108, 108, 255} // $C
						case 149:
							c = color.RGBA{149, 149, 149, 255} // $F
						default:
							c = color.RGBA{val, val, val, 255}
						}
					}

					for sy := 0; sy < scale; sy++ {
						for sx := 0; sx < scale; sx++ {
							compImg.Set(xBase+borderSize+x*scale+sx,
								yBase+borderSize+y*scale+sy, c)
						}
					}
				}
			}
		}
	}

	// Save image
	outFile, err := os.Create("emulator/frames/comparison_with_reference.png")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer outFile.Close()

	if err := png.Encode(outFile, compImg); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("✓ Saved comparison_with_reference.png")
	fmt.Println("  13 columns: Reference (16x16) + 12 variants (sorted by quality)")
	fmt.Println("  Border colors: Blue=Reference, Green=Best, Red=Worst, Orange/Yellow=Middle")
}
