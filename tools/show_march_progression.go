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

	var bitmaps [][][]bool
	var centerXs []float64

	fmt.Println("HORIZONTAL MARCH PROGRESSION")
	fmt.Println("============================")
	fmt.Println()
	fmt.Println("Frame  CenterX  March")
	fmt.Println("-----  -------  -----")

	prevCenter := 0.0
	for _, frameNum := range frames {
		content, err := os.ReadFile(fmt.Sprintf("../emulator/frames/raw_frame%d.txt", frameNum))
		if err != nil {
			continue
		}
		bitmap, _ := parseRawFrame(string(content))
		bitmaps = append(bitmaps, bitmap)

		// Calculate center X
		minX, maxX := 24, -1
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				if bitmap[y][x] {
					if x < minX {
						minX = x
					}
					if x > maxX {
						maxX = x
					}
				}
			}
		}

		centerX := float64(minX+maxX) / 2.0
		centerXs = append(centerXs, centerX)

		march := ""
		if prevCenter > 0 {
			delta := centerX - prevCenter
			march = fmt.Sprintf("%+.1f px", delta)
		}
		prevCenter = centerX

		fmt.Printf("%3d    %6.1f   %s\n", frameNum, centerX, march)
	}
	fmt.Println()

	// Create side-by-side view with unwrapped squares
	scale := 8
	frameWidth := 30 * scale // Give more space per frame to show march
	totalWidth := len(frames) * frameWidth
	height := 24 * scale

	img := image.NewRGBA(image.Rect(0, 0, totalWidth, height))

	// Background
	for y := 0; y < height; y++ {
		for x := 0; x < totalWidth; x++ {
			img.Set(x, y, color.RGBA{245, 245, 250, 255})
		}
	}

	// Ground line
	groundY := 18 * scale
	for x := 0; x < totalWidth; x++ {
		img.Set(x, groundY, color.RGBA{150, 150, 150, 255})
	}

	// Draw each frame
	for i, bitmap := range bitmaps {
		baseX := i * frameWidth

		// Draw frame in black
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				if bitmap[y][x] {
					px := baseX + x*scale
					py := y * scale
					for dy := 0; dy < scale; dy++ {
						for dx := 0; dx < scale; dx++ {
							if px+dx < totalWidth {
								img.Set(px+dx, py+dy, color.RGBA{0, 0, 0, 255})
							}
						}
					}
				}
			}
		}

		// Frame separator
		for y := 0; y < height; y++ {
			img.Set(baseX+24*scale, y, color.RGBA{200, 0, 0, 255})
		}
	}

	f, _ := os.Create("march_progression_sidebyside.png")
	defer f.Close()
	png.Encode(f, img)

	fmt.Println("Created march_progression_sidebyside.png")
	fmt.Println("Shows 10 frames side by side (raw positioning within each 24px frame)")
}
