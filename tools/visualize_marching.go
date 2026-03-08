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
	// Best parameters
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

	frames := []int{318, 320, 322, 324, 326, 328, 330, 332, 334, 336}

	// First pass: find the fixed ground level (average of bottom edges)
	// and horizontal span
	var groundYSum float64
	var minX, maxX float64 = 999, -999

	for _, frameNum := range frames {
		s := squares[frameNum]
		angleRad := s.angle * math.Pi / 180.0

		var corners [4][2]float64
		for i := 0; i < 4; i++ {
			cornerAngle := angleRad + float64(i)*math.Pi/2.0
			corners[i] = [2]float64{
				s.cx + s.size*math.Cos(cornerAngle),
				s.cy + s.size*math.Sin(cornerAngle),
			}
		}

		// Find bottom edge
		var maxEdgeY float64 = -999
		for i := 0; i < 4; i++ {
			j := (i + 1) % 4
			edgeY := (corners[i][1] + corners[j][1]) / 2.0
			if edgeY > maxEdgeY {
				maxEdgeY = edgeY
			}
			// Track X bounds
			if corners[i][0] < minX {
				minX = corners[i][0]
			}
			if corners[i][0] > maxX {
				maxX = corners[i][0]
			}
		}
		groundYSum += maxEdgeY
	}

	fixedGroundY := groundYSum / float64(len(frames))

	fmt.Printf("Fixed ground level: Y = %.3f\n", fixedGroundY)
	fmt.Printf("Horizontal span: %.3f to %.3f\n", minX, maxX)
	fmt.Println()

	// Create image showing all frames positioned relative to fixed ground
	scale := 6
	width := int((maxX - minX + 4) * float64(scale))
	height := int((fixedGroundY + 4) * float64(scale))
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill background
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{240, 240, 250, 255})
		}
	}

	// Draw ground line
	groundPixelY := int(fixedGroundY * float64(scale))
	for x := 0; x < width; x++ {
		for dy := -2; dy <= 2; dy++ {
			img.Set(x, groundPixelY+dy, color.RGBA{100, 100, 100, 255})
		}
	}

	// Draw each square with absolute positioning
	colors := []color.RGBA{
		{255, 0, 0, 255},     // Red
		{255, 128, 0, 255},   // Orange
		{255, 255, 0, 255},   // Yellow
		{0, 255, 0, 255},     // Green
		{0, 255, 255, 255},   // Cyan
		{0, 128, 255, 255},   // Blue
		{128, 0, 255, 255},   // Purple
		{255, 0, 255, 255},   // Magenta
		{255, 0, 128, 255},   // Pink
		{128, 128, 128, 255}, // Gray
	}

	for idx, frameNum := range frames {
		s := squares[frameNum]
		angleRad := s.angle * math.Pi / 180.0

		var corners [4][2]float64
		for i := 0; i < 4; i++ {
			cornerAngle := angleRad + float64(i)*math.Pi/2.0
			corners[i] = [2]float64{
				s.cx + s.size*math.Cos(cornerAngle),
				s.cy + s.size*math.Sin(cornerAngle),
			}
		}

		col := colors[idx%len(colors)]

		// Draw square edges
		for i := 0; i < 4; i++ {
			next := (i + 1) % 4
			x1, y1 := corners[i][0], corners[i][1]
			x2, y2 := corners[next][0], corners[next][1]

			steps := 200
			for step := 0; step <= steps; step++ {
				t := float64(step) / float64(steps)
				x := x1 + t*(x2-x1)
				y := y1 + t*(y2-y1)
				px := int((x - minX + 2) * float64(scale))
				py := int(y * float64(scale))
				if px >= 0 && px < width && py >= 0 && py < height {
					for dx := -2; dx <= 2; dx++ {
						for dy := -2; dy <= 2; dy++ {
							if px+dx >= 0 && px+dx < width && py+dy >= 0 && py+dy < height {
								img.Set(px+dx, py+dy, col)
							}
						}
					}
				}
			}
		}

		// Draw frame number
		cx := int((s.cx - minX + 2) * float64(scale))
		cy := int(s.cy * float64(scale))
		for dx := -4; dx <= 4; dx++ {
			for dy := -4; dy <= 4; dy++ {
				if cx+dx >= 0 && cx+dx < width && cy+dy >= 0 && cy+dy < height {
					img.Set(cx+dx, cy+dy, color.RGBA{0, 0, 0, 255})
				}
			}
		}
	}

	// Save
	f, _ := os.Create("marching_visualization.png")
	defer f.Close()
	png.Encode(f, img)

	fmt.Println("Created marching_visualization.png")
	fmt.Println("Shows all frames with FIXED ground line (no individual centering)")
	fmt.Println("Colors show sequence: Red→Orange→Yellow→Green→Cyan→Blue→Purple→Magenta→Pink→Gray")
}
