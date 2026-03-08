package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func parseFrame(content string) ([][]bool, error) {
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

func loadFrameFromFile(content string, targetFrame int) ([][]bool, error) {
	lines := strings.Split(content, "\n")
	pat := regexp.MustCompile(`=== Frame (\d+)`)
	cur := -1
	var curLines []string
	for _, line := range lines {
		if m := pat.FindStringSubmatch(line); m != nil {
			if cur == targetFrame && len(curLines) > 0 {
				return parseFrame(strings.Join(curLines, "\n"))
			}
			cur, _ = strconv.Atoi(m[1])
			curLines = []string{line}
		} else {
			curLines = append(curLines, line)
		}
	}
	if cur == targetFrame {
		return parseFrame(strings.Join(curLines, "\n"))
	}
	return nil, fmt.Errorf("not found")
}

func main() {
	frames := []int{318, 320, 322, 324, 326, 328, 330, 332, 334, 336, 338}

	fmt.Println("11-FRAME MARCHING CYCLE (Complete Upright-to-Upright)")
	fmt.Println("=====================================================")
	fmt.Println()

	var bitmaps [][][]bool

	// Load frames 318-336 from raw files
	for i := 0; i < 10; i++ {
		frameNum := frames[i]
		content, err := os.ReadFile(fmt.Sprintf("../emulator/frames/raw_frame%d.txt", frameNum))
		if err != nil {
			fmt.Printf("Error loading raw frame %d: %v\n", frameNum, err)
			return
		}
		bitmap, _ := parseFrame(string(content))
		bitmaps = append(bitmaps, bitmap)
	}

	// Load frame 338 from animation file
	content338, err := os.ReadFile("../emulator/frames/animation_336-354.txt")
	if err != nil {
		fmt.Println("Error loading animation file for frame 338")
		return
	}
	bitmap338, err := loadFrameFromFile(string(content338), 338)
	if err != nil {
		fmt.Println("Error parsing frame 338:", err)
		return
	}
	bitmaps = append(bitmaps, bitmap338)

	// Estimated march positions (1 pixel per frame)
	marchPositions := []float64{
		11.5, 13.5, 14.5, 15.5, 16.5, 17.5, 18.5, 19.5, 20.5, 21.5, 22.5,
	}

	fmt.Println("11-frame cycle march progression:")
	fmt.Println("Frame  MarchX  Delta")
	for i, frameNum := range frames {
		delta := ""
		if i > 0 {
			delta = fmt.Sprintf("+%.1f", marchPositions[i]-marchPositions[i-1])
		}
		fmt.Printf("%3d    %5.1f   %s\n", frameNum, marchPositions[i], delta)
	}
	fmt.Printf("\nTotal march: %.1f pixels (≈12px for full rotation cycle)\n\n",
		marchPositions[10]-marchPositions[0])

	// Create visualization
	scale := 8
	canvasWidth := 40 * scale
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

	// 12-pixel markers
	for gx := 0; gx <= 36; gx += 12 {
		for y := 0; y < canvasHeight; y++ {
			img.Set(gx*scale, y, color.RGBA{200, 200, 210, 255})
		}
	}

	// Colors for 11 frames
	colors := []color.RGBA{
		{255, 0, 0, 200},     // Red
		{255, 120, 0, 200},   // Orange
		{255, 200, 0, 200},   // Yellow-orange
		{200, 255, 0, 200},   // Yellow-green
		{100, 255, 0, 200},   // Green
		{0, 255, 120, 200},   // Cyan-green
		{0, 200, 255, 200},   // Cyan
		{0, 120, 255, 200},   // Blue
		{80, 0, 255, 200},    // Purple
		{180, 0, 255, 200},   // Violet
		{255, 0, 180, 200},   // Pink
	}

	// Draw all 11 frames
	for i, bitmap := range bitmaps {
		col := colors[i]
		baseX := marchPositions[i]

		// Detect wrapping
		hasLeft, hasRight := false, false
		for y := 0; y < 24; y++ {
			if bitmap[y][0] {
				hasLeft = true
			}
			if bitmap[y][23] {
				hasRight = true
			}
		}
		wrapped := hasLeft && hasRight

		// Find split
		splitGap := -1
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

		// Draw
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				if bitmap[y][x] {
					drawX := baseX + float64(x)
					if wrapped && splitGap > 0 && x < splitGap {
						drawX = baseX + float64(x+24)
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

	f, _ := os.Create("11frame_complete_cycle.png")
	defer f.Close()
	png.Encode(f, img)

	fmt.Println("Created 11frame_complete_cycle.png")
	fmt.Println("Shows complete marching cycle: upright (326) → full rotation → upright (338)")
	fmt.Println("Vertical lines at 0, 12, 24, 36 pixels")
	fmt.Println("Total march distance: ~11 pixels (≈12px for complete cycle)")
}
