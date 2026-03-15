package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

// Pre-computed frame storage (only active rows 1-17)
type precomputedFrames struct {
	// 11 frames (0-10) × 17 rows × 6 bytes (3 charset + 3 medium)
	data [11][17][6]byte
}

var precomputed precomputedFrames

// Pre-compute frames 0-10 at startup
func precomputeFrames() {
	for frame := byte(0); frame <= 10; frame++ {
		charset, medium := renderSquare(frame)

		// Store only active rows (1-17)
		for row := 1; row <= 17; row++ {
			precomputed.data[frame][row-1][0] = charset[row]
			precomputed.data[frame][row-1][1] = charset[64+row]
			precomputed.data[frame][row-1][2] = charset[128+row]
			precomputed.data[frame][row-1][3] = medium[row]
			precomputed.data[frame][row-1][4] = medium[64+row]
			precomputed.data[frame][row-1][5] = medium[128+row]
		}
	}
}

// Get animated frame (handles literal copy and mirror+shift)
func getAnimatedFrame(frameNum byte) (charset [192]byte, medium [192]byte) {
	cycles = 0

	// Arrays are zero-initialized by default (0x00)
	// Set row 0 to 0xFF (all lit background)
	charset[0] = 0xFF
	charset[64] = 0xFF
	charset[128] = 0xFF
	medium[0] = 0xFF
	medium[64] = 0xFF
	medium[128] = 0xFF

	// Set rows 18-23 to 0xFF (all lit background)
	for row := byte(18); row < 24; row++ {
		charset[row] = 0xFF
		charset[64+row] = 0xFF
		charset[128+row] = 0xFF
		medium[row] = 0xFF
		medium[64+row] = 0xFF
		medium[128+row] = 0xFF
	}
	cycles += 10 // Initialization overhead

	if frameNum <= 10 {
		// Direct copy from pre-computed buffer
		cycles += 5 // Frame selection overhead

		for row := 1; row <= 17; row++ {
			charset[row] = precomputed.data[frameNum][row-1][0]
			charset[64+row] = precomputed.data[frameNum][row-1][1]
			charset[128+row] = precomputed.data[frameNum][row-1][2]
			medium[row] = precomputed.data[frameNum][row-1][3]
			medium[64+row] = precomputed.data[frameNum][row-1][4]
			medium[128+row] = precomputed.data[frameNum][row-1][5]
			cycles += 24 // 6× LDA + STA (4 cycles each)
		}
	} else {
		// Mirror+shift transformation from corresponding frame
		sourceFrame := byte(20 - frameNum) // 11→9, 12→8, ..., 20→0
		cycles += 5 // Frame selection and calculation

		for row := 1; row <= 17; row++ {
			cycles += 5 // Loop overhead

			// Load source row
			srcCol0 := precomputed.data[sourceFrame][row-1][0]
			srcCol1 := precomputed.data[sourceFrame][row-1][1]
			srcCol2 := precomputed.data[sourceFrame][row-1][2]
			srcMedCol0 := precomputed.data[sourceFrame][row-1][3]
			srcMedCol1 := precomputed.data[sourceFrame][row-1][4]
			srcMedCol2 := precomputed.data[sourceFrame][row-1][5]
			cycles += 24 // 6× LDA (4 cycles each)

			// Apply mirror+shift transformation: pixel(x) → pixel((35-x) mod 24)
			// This requires bit reversal and column rotation
			var outCol0, outCol1, outCol2 byte
			var outMedCol0, outMedCol1, outMedCol2 byte

			// Per-pixel transformation (24 bits)
			for outX := 0; outX < 24; outX++ {
				srcX := (35 - outX) % 24

				// Extract source bit
				srcCol := srcX / 8
				srcBit := 7 - (srcX % 8)
				var srcByte, srcMedByte byte
				switch srcCol {
				case 0:
					srcByte = srcCol0
					srcMedByte = srcMedCol0
				case 1:
					srcByte = srcCol1
					srcMedByte = srcMedCol1
				case 2:
					srcByte = srcCol2
					srcMedByte = srcMedCol2
				}

				srcVal := (srcByte >> srcBit) & 1
				srcMedVal := (srcMedByte >> srcBit) & 1

				// Set output bit
				outCol := outX / 8
				outBit := 7 - (outX % 8)

				if srcVal != 0 {
					switch outCol {
					case 0: outCol0 |= (1 << outBit)
					case 1: outCol1 |= (1 << outBit)
					case 2: outCol2 |= (1 << outBit)
					}
				}
				if srcMedVal != 0 {
					switch outCol {
					case 0: outMedCol0 |= (1 << outBit)
					case 1: outMedCol1 |= (1 << outBit)
					case 2: outMedCol2 |= (1 << outBit)
					}
				}
			}

			// On 6502: table-based bit manipulation (~50 cycles/row)
			cycles += 50

			// Write output
			charset[row] = outCol0
			charset[64+row] = outCol1
			charset[128+row] = outCol2
			medium[row] = outMedCol0
			medium[64+row] = outMedCol1
			medium[128+row] = outMedCol2
			cycles += 24 // 6× STA (4 cycles each)
		}
	}

	return charset, medium
}

// Animate through all 21 frames and show performance
func animateDemo() {
	precomputeFrames()

	// C64 palette colors
	black := color.RGBA{0x00, 0x00, 0x00, 0xFF}
	darkGrey := color.RGBA{0x5F, 0x5F, 0x5F, 0xFF}
	lightGrey := color.RGBA{0xC6, 0xC6, 0xC6, 0xFF}
	red := color.RGBA{0xFF, 0x00, 0x00, 0xFF}

	var totalCycles int
	var frames [21][192]byte
	var mediums [21][192]byte
	var refs [21][192]byte
	var refMeds [21][192]byte

	for frame := byte(0); frame <= 20; frame++ {
		cycles = 0
		charset, medium := getAnimatedFrame(frame)
		totalCycles += int(cycles)
		frames[frame] = charset
		mediums[frame] = medium

		// Verify optimization correctness
		expectedCharset, expectedMedium := renderSquare(frame)
		for i := 0; i < 192; i++ {
			if charset[i] != expectedCharset[i] || medium[i] != expectedMedium[i] {
				fmt.Printf("ERROR: frame %d doesn't match renderSquare\n", frame)
				return
			}
		}

		// Get reference for error visualization (DDA vs exact intersection)
		refCharset, refMedium := refRenderSquare(frame)
		refs[frame] = refCharset
		refMeds[frame] = refMedium
	}

	// Create sprite sheet: 7 columns × 3 rows
	pixelSize := 8
	frameSize := 24 * pixelSize
	cols := 7
	rows := 3
	sep := 1
	sheetWidth := cols*frameSize + (cols-1)*sep
	sheetHeight := rows*frameSize + (rows-1)*sep
	sheet := image.NewRGBA(image.Rect(0, 0, sheetWidth, sheetHeight))

	// Fill background (white separators)
	separator := color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	for y := 0; y < sheetHeight; y++ {
		for x := 0; x < sheetWidth; x++ {
			sheet.Set(x, y, separator)
		}
	}

	// Draw each frame
	for i := 0; i < 21; i++ {
		col := i % cols
		row := i / cols
		frameImg := charsetToImage(frames[i], mediums[i], pixelSize, black, darkGrey, lightGrey)

		// Overlay red dots where proto differs from reference
		for outY := 0; outY < 24; outY++ {
			for outX := 0; outX < 24; outX++ {
				protCount := getCount(frames[i], mediums[i], outX, outY)
				refCount := getCount(refs[i], refMeds[i], outX, outY)
				errMag := protCount - refCount
				if errMag < 0 {
					errMag = -errMag
				}
				if errMag > 0 {
					// Dot radius: 1×1 for error=1, 3×3 for error=2-3, 5×5 for error=4
					radius := errMag
					if radius > 2 {
						radius = 2
					}
					// Draw centered dot
					centerX := outX*8 + 4
					centerY := outY*8 + 4
					for dy := -radius; dy <= radius; dy++ {
						for dx := -radius; dx <= radius; dx++ {
							frameImg.Set(centerX+dx, centerY+dy, red)
						}
					}
				}
			}
		}

		// Copy frame into sheet
		offsetX := col * (frameSize + sep)
		offsetY := row * (frameSize + sep)
		for y := 0; y < frameSize; y++ {
			for x := 0; x < frameSize; x++ {
				sheet.Set(offsetX+x, offsetY+y, frameImg.At(x, y))
			}
		}
	}

	// Write sprite sheet
	path := "animation.png"
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	png.Encode(f, sheet)
	f.Close()

	// Compute error statistics
	var totalDiffs int
	for i := 0; i < 21; i++ {
		totalDiffs += diffCharsets(frames[i], refs[i])
	}
	avgDiffs := float64(totalDiffs) / 21.0
	errorRate := avgDiffs * 100.0 / (24.0 * 24.0)

	avgCycles := totalCycles / 21
	fmt.Printf("%s: %.1f FPS @ 1MHz (%d cyc/frame, %.2f%% error)\n",
		path, 1000000.0/float64(avgCycles), avgCycles, errorRate)
}
