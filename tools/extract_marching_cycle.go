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
	// Full cycle: 11 frames from upright through complete rotation
	// Frame 326 is nearly upright (-1.7°), so let's go 326-346 (21 frames = ~2 rotations)
	// Or better: 318-338 to capture the early march
	frames := []int{318, 320, 322, 324, 326, 328, 330, 332, 334, 336, 338}

	fmt.Println("MARCHING SQUARE - FULL CYCLE EXTRACTION")
	fmt.Println("========================================")
	fmt.Println("11 frames showing horizontal march + rotation")
	fmt.Println()

	var bitmaps [][][]bool
	for _, frameNum := range frames {
		var content []byte
		var err error

		// Try raw frames first
		content, err = os.ReadFile(fmt.Sprintf("../emulator/frames/raw_frame%d.txt", frameNum))
		if err != nil {
			// Try from centered animation file
			content, err = os.ReadFile("../emulator/frames/animation_336-354.txt")
			if err != nil {
				fmt.Printf("Can't load frame %d\n", frameNum)
				continue
			}
		}

		bitmap, err := parseRawFrame(string(content))
		if err != nil {
			fmt.Printf("Error parsing frame %d\n", frameNum)
			continue
		}
		bitmaps = append(bitmaps, bitmap)
	}

	// Analyze each frame's horizontal extent
	type FrameInfo struct {
		frameNum int
		minX     int
		maxX     int
		wrapped  bool
	}

	var frameInfos []FrameInfo

	fmt.Println("Frame analysis:")
	fmt.Println("Frame  MinX  MaxX  Width  Wrapped?")
	for i, bitmap := range bitmaps {
		minX, maxX := 24, -1
		hasLeft := false  // Pixels at X=0
		hasRight := false // Pixels at X=23

		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				if bitmap[y][x] {
					if x < minX {
						minX = x
					}
					if x > maxX {
						maxX = x
					}
					if x == 0 {
						hasLeft = true
					}
					if x == 23 {
						hasRight = true
					}
				}
			}
		}

		wrapped := minX == 0 && maxX == 23 && hasLeft && hasRight
		width := maxX - minX + 1

		fmt.Printf("%3d    %3d   %3d   %4d    %v\n",
			frames[i], minX, maxX, width, wrapped)

		frameInfos = append(frameInfos, FrameInfo{
			frameNum: frames[i],
			minX:     minX,
			maxX:     maxX,
			wrapped:  wrapped,
		})
	}
	fmt.Println()

	// Create extended canvas showing horizontal march
	// Calculate total width needed (24px per frame position + some margin)
	scale := 8
	canvasWidth := 48 * scale  // Wide enough to show ~2 full 24px positions
	canvasHeight := 24 * scale

	// Create image
	img := image.NewRGBA(image.Rect(0, 0, canvasWidth, canvasHeight))

	// Fill background
	for y := 0; y < canvasHeight; y++ {
		for x := 0; x < canvasWidth; x++ {
			img.Set(x, y, color.RGBA{245, 245, 250, 255})
		}
	}

	// Draw ground line at Y=18
	groundY := 18 * scale
	for x := 0; x < canvasWidth; x++ {
		for dy := -1; dy <= 1; dy++ {
			img.Set(x, groundY+dy, color.RGBA{150, 150, 150, 255})
		}
	}

	// Draw each frame at its horizontal position
	colors := []color.RGBA{
		{255, 0, 0, 200},
		{255, 100, 0, 200},
		{255, 180, 0, 200},
		{200, 200, 0, 200},
		{100, 255, 0, 200},
		{0, 255, 100, 200},
		{0, 200, 255, 200},
		{0, 100, 255, 200},
		{100, 0, 255, 200},
		{200, 0, 255, 200},
		{255, 0, 200, 200},
	}

	for i, info := range frameInfos {
		bitmap := bitmaps[i]
		col := colors[i%len(colors)]

		// For wrapped frames, unwrap by detecting the split point
		if info.wrapped {
			// Find where the square is split (gap in the middle)
			splitX := -1
			for x := 1; x < 23; x++ {
				hasPixelLeft := false
				hasPixelRight := false
				for y := 0; y < 24; y++ {
					if bitmap[y][x-1] {
						hasPixelLeft = true
					}
					if bitmap[y][x] {
						hasPixelRight = true
					}
				}
				// Large gap indicates wrap point
				if hasPixelLeft && !hasPixelRight {
					gapSize := 0
					for gx := x; gx < 24; gx++ {
						hasAny := false
						for y := 0; y < 24; y++ {
							if bitmap[y][gx] {
								hasAny = true
								break
							}
						}
						if hasAny {
							break
						}
						gapSize++
					}
					if gapSize > 5 { // Significant gap
						splitX = x
						break
					}
				}
			}

			// Draw unwrapped: right part first, then left part shifted
			for y := 0; y < 24; y++ {
				for x := 0; x < 24; x++ {
					if bitmap[y][x] {
						drawX := x
						// Shift left part (X<splitX) to the right beyond the right part
						if splitX > 0 && x < splitX {
							drawX = x + 24
						}
						px := drawX * scale
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
		} else {
			// Not wrapped - draw at actual position
			for y := 0; y < 24; y++ {
				for x := 0; x < 24; x++ {
					if bitmap[y][x] {
						px := x * scale
						py := y * scale
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

	// Save
	f, _ := os.Create("marching_cycle_11frames.png")
	defer f.Close()
	png.Encode(f, img)

	fmt.Println("Created marching_cycle_11frames.png")
	fmt.Println("Shows 11-frame cycle with horizontal march preserved")
	fmt.Println("Colors: Red→Orange→Yellow→Green→Cyan→Blue→Purple→Magenta")
	fmt.Println("Wrapped frames are unwrapped but show actual horizontal progression")
}
