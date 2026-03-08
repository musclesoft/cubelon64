package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func main() {
	// Best parameters for all 10 frames
	squares := map[int]struct{ cx, cy, size, angle float64 }{
		318: {12.003906, 11.496094, 7.906250, 33.70},
		320: {11.750000, 10.875000, 8.109375, 63.50},
		322: {12.269531, 11.359375, 8.085938, 70.90},
		324: {11.597656, 11.175781, 7.914062, 81.40},
		326: {11.750000, 11.250000, 7.753906, -1.70},
		328: {12.250000, 11.050781, 7.988281, 11.20},
		330: {11.664062, 11.410156, 8.066406, 17.30},
		332: {11.750000, 11.023438, 8.132812, 26.60},
		334: {11.750000, 11.656250, 8.054688, -146.30},
		336: {12.000000, 11.500000, 8.000000, 45.00},
	}

	perfect := map[int]bool{322: true, 326: true, 330: true, 336: true}
	distorted := map[int]bool{320: true, 334: true}

	// Create large image with all frames in a 2x5 grid
	width, height := 24*10*4, 24*2*4 // 4x scale for visibility
	scale := 4
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill background
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{240, 240, 245, 255})
		}
	}

	frames := []int{318, 320, 322, 324, 326, 328, 330, 332, 334, 336}

	// Draw each frame
	for i, frameNum := range frames {
		row := i / 5
		col := i % 5
		offsetX := col * 24 * scale
		offsetY := row * 24 * scale

		s := squares[frameNum]

		// Draw grid lines
		for g := 0; g <= 24; g++ {
			for p := 0; p < scale; p++ {
				if g%4 == 0 {
					// Horizontal grid
					for x := 0; x < 24*scale; x++ {
						img.Set(offsetX+x, offsetY+g*scale+p, color.RGBA{220, 220, 225, 255})
					}
					// Vertical grid
					for y := 0; y < 24*scale; y++ {
						img.Set(offsetX+g*scale+p, offsetY+y, color.RGBA{220, 220, 225, 255})
					}
				}
			}
		}

		// Calculate corners
		angleRad := s.angle * math.Pi / 180.0
		var corners [4][2]float64
		for j := 0; j < 4; j++ {
			cornerAngle := angleRad + float64(j)*math.Pi/2.0
			corners[j] = [2]float64{
				s.cx + s.size*math.Cos(cornerAngle),
				s.cy + s.size*math.Sin(cornerAngle),
			}
		}

		// Draw square edges
		edgeColor := color.RGBA{100, 100, 150, 255}
		if perfect[frameNum] {
			edgeColor = color.RGBA{0, 180, 0, 255} // Green for perfect squares
		} else if distorted[frameNum] {
			edgeColor = color.RGBA{0, 100, 255, 255} // Blue for distorted quadrilaterals
		} else {
			edgeColor = color.RGBA{200, 100, 0, 255} // Orange for rasterization diffs
		}

		for j := 0; j < 4; j++ {
			next := (j + 1) % 4
			x1, y1 := corners[j][0], corners[j][1]
			x2, y2 := corners[next][0], corners[next][1]

			// Draw edge line
			steps := 100
			for step := 0; step <= steps; step++ {
				t := float64(step) / float64(steps)
				x := x1 + t*(x2-x1)
				y := y1 + t*(y2-y1)
				px := int(x*float64(scale)) + offsetX
				py := int(y*float64(scale)) + offsetY
				if px >= offsetX && px < offsetX+24*scale && py >= offsetY && py < offsetY+24*scale {
					for dx := -1; dx <= 1; dx++ {
						for dy := -1; dy <= 1; dy++ {
							img.Set(px+dx, py+dy, edgeColor)
						}
					}
				}
			}
		}

		// Draw pivot point (center) - RED DOT
		pivotColor := color.RGBA{255, 0, 0, 255}
		pivotSize := 3
		px := int(s.cx*float64(scale)) + offsetX
		py := int(s.cy*float64(scale)) + offsetY
		for dx := -pivotSize; dx <= pivotSize; dx++ {
			for dy := -pivotSize; dy <= pivotSize; dy++ {
				if dx*dx+dy*dy <= pivotSize*pivotSize {
					img.Set(px+dx, py+dy, pivotColor)
				}
			}
		}

		// Draw crosshair through pivot
		for d := -6; d <= 6; d++ {
			img.Set(px+d, py, pivotColor)
			img.Set(px, py+d, pivotColor)
		}

		// Draw frame number
		// (Simple text rendering would require a font package, skip for now)
	}

	// Add legend/labels
	legendY := height - 30
	legendEntries := []struct {
		x     int
		label string
		col   color.RGBA
	}{
		{10, "Perfect Square", color.RGBA{0, 180, 0, 255}},
		{200, "Distorted Quad", color.RGBA{0, 100, 255, 255}},
		{400, "Rasterization Diff", color.RGBA{200, 100, 0, 255}},
		{620, "Pivot Point", color.RGBA{255, 0, 0, 255}},
	}

	for _, entry := range legendEntries {
		// Draw color box
		for dy := 0; dy < 10; dy++ {
			for dx := 0; dx < 20; dx++ {
				img.Set(entry.x+dx, legendY+dy, entry.col)
			}
		}
	}

	// Save image
	f, _ := os.Create("pivot_visualization.png")
	defer f.Close()
	png.Encode(f, img)

	fmt.Println("Created pivot_visualization.png")
	fmt.Println()
	fmt.Println("Pivot Point Analysis:")
	fmt.Println("====================")

	// Calculate statistics
	var sumX, sumY float64
	var minX, maxX, minY, maxY float64
	first := true
	for _, s := range squares {
		sumX += s.cx
		sumY += s.cy
		if first {
			minX, maxX = s.cx, s.cx
			minY, maxY = s.cy, s.cy
			first = false
		} else {
			if s.cx < minX {
				minX = s.cx
			}
			if s.cx > maxX {
				maxX = s.cx
			}
			if s.cy < minY {
				minY = s.cy
			}
			if s.cy > maxY {
				maxY = s.cy
			}
		}
	}
	avgX := sumX / 10.0
	avgY := sumY / 10.0

	fmt.Printf("Average pivot: (%.3f, %.3f)\n", avgX, avgY)
	fmt.Printf("Pivot range X: %.3f to %.3f (delta: %.3f)\n", minX, maxX, maxX-minX)
	fmt.Printf("Pivot range Y: %.3f to %.3f (delta: %.3f)\n", minY, maxY, maxY-minY)
	fmt.Println()

	fmt.Println("Per-Frame Pivot Points:")
	fmt.Println("Frame  Center(cx, cy)         Size    Angle     Type")
	fmt.Println("-----  --------------------   ------  --------  ----")
	for _, frameNum := range frames {
		s := squares[frameNum]
		typeStr := "Rasterization"
		if perfect[frameNum] {
			typeStr = "Perfect Square"
		} else if distorted[frameNum] {
			typeStr = "Distorted Quad"
		}
		fmt.Printf("%3d    (%6.3f, %6.3f)      %6.3f  %7.2f°  %s\n",
			frameNum, s.cx, s.cy, s.size, s.angle, typeStr)
	}
	fmt.Println()

	// Check if pivot is moving
	maxDeviation := 0.0
	for _, s := range squares {
		dx := s.cx - avgX
		dy := s.cy - avgY
		deviation := math.Sqrt(dx*dx + dy*dy)
		if deviation > maxDeviation {
			maxDeviation = deviation
		}
	}

	fmt.Printf("Maximum deviation from average: %.3f pixels\n", maxDeviation)
	if maxDeviation < 0.5 {
		fmt.Println("✓ Pivot point is STABLE - single fixed rotation center")
	} else {
		fmt.Println("✗ Pivot point MOVES - center shifts during animation")
	}
}
