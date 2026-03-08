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

func main() {
	frames := []int{318, 320, 322, 324, 326, 328, 330, 332, 334, 336}

	fmt.Println("RAW FRAMES (BEFORE CENTERING)")
	fmt.Println("==============================")
	fmt.Println()

	var bitmaps [][][]bool
	for _, frameNum := range frames {
		content, err := os.ReadFile(fmt.Sprintf("../emulator/frames/raw_frame%d.txt", frameNum))
		if err != nil {
			fmt.Printf("Error reading raw_frame%d.txt: %v\n", frameNum, err)
			continue
		}
		bitmap, err := parseRawFrame(string(content))
		if err != nil {
			fmt.Printf("Error parsing frame %d: %v\n", frameNum, err)
			continue
		}
		bitmaps = append(bitmaps, bitmap)
	}

	// Analyze bounds
	fmt.Println("Raw frame bounds (NO centering applied):")
	fmt.Println("Frame  MinX  MaxX  CenterX  Width")
	fmt.Println("-----  ----  ----  -------  -----")

	for i, bitmap := range bitmaps {
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
		if maxX >= 0 {
			centerX := float64(minX+maxX) / 2.0
			width := maxX - minX + 1
			fmt.Printf("%3d    %4d  %4d   %6.1f   %5d\n",
				frames[i], minX, maxX, centerX, width)
		}
	}
	fmt.Println()

	// Create visualization
	scale := 10
	width := len(bitmaps) * 24 * scale
	height := 24 * scale
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill background
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{240, 240, 240, 255})
		}
	}

	// Draw each frame
	for i, bitmap := range bitmaps {
		offsetX := i * 24 * scale

		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				col := color.RGBA{255, 255, 255, 255}
				if bitmap[y][x] {
					col = color.RGBA{0, 0, 0, 255}
				}

				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						img.Set(offsetX+x*scale+dx, y*scale+dy, col)
					}
				}
			}
		}

		// Frame separator
		for y := 0; y < height; y++ {
			img.Set(offsetX+24*scale-1, y, color.RGBA{200, 0, 0, 255})
		}
	}

	f, _ := os.Create("truly_raw_frames.png")
	defer f.Close()
	png.Encode(f, img)

	fmt.Println("Created truly_raw_frames.png")
	fmt.Println("Shows RAW frames BEFORE any centering was applied by emulator")
}
