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

func parseFrame(lines []string) ([][]bool, error) {
	bitmap := make([][]bool, 24)
	lineIdx := 0
	for lineIdx < len(lines) && !strings.HasPrefix(lines[lineIdx], "===") {
		lineIdx++
	}
	if lineIdx >= len(lines) {
		return nil, fmt.Errorf("no frame header")
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

func loadFrame(content string, targetFrame int) ([][]bool, error) {
	lines := strings.Split(content, "\n")
	framePattern := regexp.MustCompile(`=== Frame (\d+)`)
	currentFrameNum := -1
	var currentFrameLines []string

	for _, line := range lines {
		if match := framePattern.FindStringSubmatch(line); match != nil {
			if currentFrameNum == targetFrame && len(currentFrameLines) > 0 {
				return parseFrame(currentFrameLines)
			}
			currentFrameNum, _ = strconv.Atoi(match[1])
			currentFrameLines = []string{line}
		} else {
			currentFrameLines = append(currentFrameLines, line)
		}
	}
	if currentFrameNum == targetFrame {
		return parseFrame(currentFrameLines)
	}
	return nil, fmt.Errorf("frame not found")
}

func main() {
	content, err := os.ReadFile("../emulator/frames/animation_318-336.txt")
	if err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	frames := []int{318, 320, 322, 324, 326, 328, 330, 332, 334, 336}

	fmt.Println("RAW FRAME DISPLAY - NO CENTERING APPLIED")
	fmt.Println("=========================================")
	fmt.Println("Showing original 24×24 bitmaps side by side")
	fmt.Println()

	// Load all frames
	var bitmaps [][][]bool
	for _, frameNum := range frames {
		bitmap, err := loadFrame(string(content), frameNum)
		if err != nil {
			fmt.Printf("Error loading frame %d: %v\n", frameNum, err)
			continue
		}
		bitmaps = append(bitmaps, bitmap)
	}

	// Create image: 10 frames × 24px wide each, scaled 8x
	scale := 8
	width := len(frames) * 24 * scale
	height := 24 * scale
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill background
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{240, 240, 240, 255})
		}
	}

	// Draw each frame side by side (NO transformation, NO centering)
	for i, bitmap := range bitmaps {
		offsetX := i * 24 * scale

		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				col := color.RGBA{255, 255, 255, 255} // White
				if bitmap[y][x] {
					col = color.RGBA{0, 0, 0, 255} // Black
				}

				// Draw scaled pixel
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						img.Set(offsetX+x*scale+dx, y*scale+dy, col)
					}
				}
			}
		}

		// Draw frame separator
		for y := 0; y < height; y++ {
			img.Set(offsetX+24*scale-1, y, color.RGBA{200, 0, 0, 255})
		}

		// Draw frame number at bottom
		frameLabel := fmt.Sprintf("%d", frames[i])
		_ = frameLabel // Would need font rendering to display
	}

	// Save image
	f, _ := os.Create("raw_frames_uncentered.png")
	defer f.Close()
	png.Encode(f, img)

	fmt.Println("Created raw_frames_uncentered.png")
	fmt.Println("Shows frames 318-336 exactly as they appear in original data")
	fmt.Println("NO centering, NO transformation - pure raw bitmaps")
	fmt.Println()

	// Analyze horizontal position of content in each frame
	fmt.Println("Analyzing content position in each frame:")
	fmt.Println("Frame  LeftX  RightX  CenterX  Width")
	fmt.Println("-----  -----  ------  -------  -----")

	for i, bitmap := range bitmaps {
		minX, maxX := 24, -1
		foundAny := false

		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				if bitmap[y][x] {
					foundAny = true
					if x < minX {
						minX = x
					}
					if x > maxX {
						maxX = x
					}
				}
			}
		}

		if foundAny {
			centerX := (minX + maxX) / 2.0
			width := maxX - minX + 1
			fmt.Printf("%3d    %3d    %3d     %5.1f    %3d\n",
				frames[i], minX, maxX, centerX, width)
		}
	}
}
