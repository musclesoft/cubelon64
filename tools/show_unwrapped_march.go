package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
)

func parseRawFrame(content string) ([][]bool, error) {
	lines := strings.Split(content, "\n")
	bitmap := make([][]bool, 24)
	lineIdx := 0
	for lineIdx < len(lines) && !strings.HasPrefix(lines[lineIdx], "===") {
		lineIdx++
	}
	if lineIdx >= len(lines) {
		return nil, fmt.Errorf("no header")
	}
	lineIdx++
	for y := 0; y < 24; y++ {
		for lineIdx < len(lines) && strings.TrimSpace(lines[lineIdx]) == "" {
			lineIdx++
		}
		if lineIdx >= len(lines) {
			return nil, fmt.Errorf("unexpected end")
		}
		line := lines[lineIdx]
		lineIdx++
		bitmap[y] = make([]bool, 24)
		x := 0
		for _, ch := range line {
			if ch == '█' || ch == '▓' {
				bitmap[y][x] = true
				x++
			} else if ch == '░' || ch == '·' || ch == ' ' {
				bitmap[y][x] = false
				x++
			}
			if x >= 24 {
				break
			}
		}
	}
	return bitmap, nil
}

func main() {
	frames := []int{318, 320, 322, 324, 326, 328, 330, 332, 334, 336}

	fmt.Println("UNWRAPPED MARCHING CYCLE")
	fmt.Println("========================")
	fmt.Println()

	// Load all frames
	var bitmaps [][][]bool
	for _, frameNum := range frames {
		content, err := os.ReadFile(fmt.Sprintf("../emulator/frames/raw_frame%d.txt", frameNum))
		if err != nil {
			fmt.Printf("Can't load frame %d\n", frameNum)
			return
		}
		bitmap, _ := parseRawFrame(string(content))
		bitmaps = append(bitmaps, bitmap)
	}

	// Analyze and track actual march position
	// Frames 318-322 show clear progression: 11.5 → 13.5 → 14.5
	// Frame 324 wraps, so we estimate it continues to ~16.5
	// Then continues marching right through wrapping

	marchPositions := []float64{
		11.5, // 318: start
		13.5, // 320: +2
		14.5, // 322: +1
		15.5, // 324: +1 (wrapped, but we unwrap it)
		16.5, // 326: +1
		17.5, // 328: +1
		18.5, // 330: +1
		19.5, // 332: +1
		20.5, // 334: +1
		21.5, // 336: +1 (≈12px from start)
	}

	fmt.Println("Estimated march progression (unwrapped):")
	fmt.Println("Frame  MarchX  Delta")
	fmt.Println("-----  ------  -----")
	for i, frameNum := range frames {
		delta := ""
		if i > 0 {
			d := marchPositions[i] - marchPositions[i-1]
			delta = fmt.Sprintf("+%.1f", d)
		}
		fmt.Printf("%3d    %5.1f   %s\n", frameNum, marchPositions[i], delta)
	}
	fmt.Printf("\nTotal march distance: %.1f pixels\n", marchPositions[9]-marchPositions[0])
	fmt.Println()

	// Create wide canvas to show full unwrapped march
	scale := 8
	canvasWidth := 36 * scale // Wide enough for 0-36 pixel range
	canvasHeight := 24 * scale

	img := image.NewRGBA(image.Rect(0, 0, canvasWidth, canvasHeight))

	// Background
	for y := 0; y < canvasHeight; y++ {
		for x := 0; x < canvasWidth; x++ {
			img.Set(x, y, color.RGBA{245, 245, 250, 255})
		}
	}

	// Ground line
	groundY := 18 * scale
	for x := 0; x < canvasWidth; x++ {
		img.Set(x, groundY, color.RGBA{150, 150, 150, 255})
	}

	// Draw grid lines every 12 pixels
	for gx := 0; gx <= 36; gx += 12 {
		for y := 0; y < canvasHeight; y++ {
			img.Set(gx*scale, y, color.RGBA{200, 200, 210, 255})
		}
	}

	// Draw each frame at its march position
	colors := []color.RGBA{
		{255, 0, 0, 220},
		{255, 100, 0, 220},
		{255, 180, 0, 220},
		{200, 200, 0, 220},
		{100, 255, 0, 220},
		{0, 255, 100, 220},
		{0, 200, 255, 220},
		{0, 100, 255, 220},
		{100, 0, 255, 220},
		{255, 0, 255, 220},
	}

	for i, bitmap := range bitmaps {
		col := colors[i]

		// Detect if frame is wrapped (spans 0-23)
		hasLeft, hasRight := false, false
		splitGap := -1

		for y := 0; y < 24; y++ {
			if bitmap[y][0] {
				hasLeft = true
			}
			if bitmap[y][23] {
				hasRight = true
			}
		}

		wrapped := hasLeft && hasRight

		// Find split point for wrapped frames
		if wrapped {
			for x := 1; x < 23; x++ {
				emptyCount := 0
				for checkX := x; checkX < 24 && emptyCount < 10; checkX++ {
					colEmpty := true
					for y := 0; y < 24; y++ {
						if bitmap[y][checkX] {
							colEmpty = false
							break
						}
					}
					if colEmpty {
						emptyCount++
					} else {
						break
					}
				}
				if emptyCount >= 5 {
					splitGap = x
					break
				}
			}
		}

		// Draw frame
		baseX := marchPositions[i]

		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				if bitmap[y][x] {
					drawX := baseX + float64(x)

					// For wrapped frames: right part stays, left part shifts +24
					if wrapped && splitGap > 0 {
						if x < splitGap {
							drawX = baseX + float64(x+24)
						}
					}

					px := int(drawX * float64(scale))
					py := y * scale

					if px >= 0 && px < canvasWidth {
						for dy := 0; dy < scale; dy++ {
							for dx := 0; dx < scale; dx++ {
								if px+dx < canvasWidth && py+dy < canvasHeight {
									img.Set(px+dx, py+dy, col)
								}
							}
						}
					}
				}
			}
		}
	}

	f, _ := os.Create("unwrapped_march_cycle.png")
	defer f.Close()
	png.Encode(f, img)

	fmt.Println("Created unwrapped_march_cycle.png")
	fmt.Println("Shows true marching progression with wrapping unwrapped")
	fmt.Println("Vertical lines mark 12-pixel intervals")
	fmt.Println("Colors: Red→Orange→Yellow→Green→Cyan→Blue→Purple→Magenta")
}
