package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

// Pre-computed frame storage (only active rows 1-17)
// Uses 128-byte stride per frame for efficient 6502 access via shift
type precomputedFrames struct {
	// 11 frames × 128 bytes (102 used: 17 rows × 6 bytes, 26 padding)
	data [11][128]byte
}

// Reverse bits in a byte (for horizontal mirror)
func reverseBits(b byte) byte {
	b = (b&0xF0)>>4 | (b&0x0F)<<4
	b = (b&0xCC)>>2 | (b&0x33)<<2
	b = (b&0xAA)>>1 | (b&0x55)<<1
	return b
}

var precomputed precomputedFrames

// Pre-compute frames 0-10 at startup
func precomputeFrames() {
	for frame := byte(0); frame <= 10; frame++ {
		charset, medium := renderSquare(frame)

		// Store only active rows (1-17) at 6-byte stride per row
		for row := 1; row <= 17; row++ {
			offset := (row - 1) * 6
			precomputed.data[frame][offset+0] = charset[row]
			precomputed.data[frame][offset+1] = charset[64+row]
			precomputed.data[frame][offset+2] = charset[128+row]
			precomputed.data[frame][offset+3] = medium[row]
			precomputed.data[frame][offset+4] = medium[64+row]
			precomputed.data[frame][offset+5] = medium[128+row]
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
			offset := (row - 1) * 6
			charset[row] = precomputed.data[frameNum][offset+0]
			charset[64+row] = precomputed.data[frameNum][offset+1]
			charset[128+row] = precomputed.data[frameNum][offset+2]
			medium[row] = precomputed.data[frameNum][offset+3]
			medium[64+row] = precomputed.data[frameNum][offset+4]
			medium[128+row] = precomputed.data[frameNum][offset+5]
			cycles += 24 // 6× LDA + STA (4 cycles each)
		}
	} else {
		// Mirror+shift transformation from corresponding frame
		sourceFrame := byte(20 - frameNum) // 11→9, 12→8, ..., 20→0
		cycles += 5 // Frame selection and calculation

		for row := 1; row <= 17; row++ {
			cycles += 5 // Loop overhead

			// Load source row
			offset := (row - 1) * 6
			srcCol0 := precomputed.data[sourceFrame][offset+0]
			srcCol1 := precomputed.data[sourceFrame][offset+1]
			srcCol2 := precomputed.data[sourceFrame][offset+2]
			srcMedCol0 := precomputed.data[sourceFrame][offset+3]
			srcMedCol1 := precomputed.data[sourceFrame][offset+4]
			srcMedCol2 := precomputed.data[sourceFrame][offset+5]
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
	var frames [41][192]byte
	var mediums [41][192]byte
	var refs [41][192]byte
	var refMeds [41][192]byte

	for frame := byte(0); frame <= 40; frame++ {
		// Phase 1 (0-10): none
		// Phase 2 (11-20): mirror+shift
		// Phase 3 (21-31): rotate right 12px (frames 1-11)
		// Phase 4 (32-40): horizontal mirror (frames 9-1, reverse order)
		sourceFrame := frame
		transform := 0 // 0=none, 1=rotate_r12, 2=hmirror

		if frame >= 32 {
			sourceFrame = 41 - frame
			transform = 2
		} else if frame >= 21 {
			sourceFrame = frame - 20
			transform = 1
		}

		cycles = 0
		charset, medium := getAnimatedFrame(sourceFrame)
		totalCycles += int(cycles)

		// Apply transform
		if transform != 0 {
			for row := 1; row <= 17; row++ {
				col0 := charset[row]
				col1 := charset[64+row]
				col2 := charset[128+row]
				medCol0 := medium[row]
				medCol1 := medium[64+row]
				medCol2 := medium[128+row]

				if transform == 1 {
					// Rotate right 12 pixels
					charset[row] = (col1 << 4) | (col2 >> 4)
					charset[64+row] = (col2 << 4) | (col0 >> 4)
					charset[128+row] = (col0 << 4) | (col1 >> 4)
					medium[row] = (medCol1 << 4) | (medCol2 >> 4)
					medium[64+row] = (medCol2 << 4) | (medCol0 >> 4)
					medium[128+row] = (medCol0 << 4) | (medCol1 >> 4)
				} else {
					// Horizontal mirror
					charset[row] = reverseBits(col2)
					charset[64+row] = reverseBits(col1)
					charset[128+row] = reverseBits(col0)
					medium[row] = reverseBits(medCol2)
					medium[64+row] = reverseBits(medCol1)
					medium[128+row] = reverseBits(medCol0)
				}
			}
		}

		frames[frame] = charset
		mediums[frame] = medium

		// Verify optimization correctness
		expectedCharset, expectedMedium := renderSquare(sourceFrame)
		if transform != 0 {
			for row := 1; row <= 17; row++ {
				col0 := expectedCharset[row]
				col1 := expectedCharset[64+row]
				col2 := expectedCharset[128+row]
				medCol0 := expectedMedium[row]
				medCol1 := expectedMedium[64+row]
				medCol2 := expectedMedium[128+row]

				if transform == 1 {
					expectedCharset[row] = (col1 << 4) | (col2 >> 4)
					expectedCharset[64+row] = (col2 << 4) | (col0 >> 4)
					expectedCharset[128+row] = (col0 << 4) | (col1 >> 4)
					expectedMedium[row] = (medCol1 << 4) | (medCol2 >> 4)
					expectedMedium[64+row] = (medCol2 << 4) | (medCol0 >> 4)
					expectedMedium[128+row] = (medCol0 << 4) | (medCol1 >> 4)
				} else {
					expectedCharset[row] = reverseBits(col2)
					expectedCharset[64+row] = reverseBits(col1)
					expectedCharset[128+row] = reverseBits(col0)
					expectedMedium[row] = reverseBits(medCol2)
					expectedMedium[64+row] = reverseBits(medCol1)
					expectedMedium[128+row] = reverseBits(medCol0)
				}
			}
		}
		for i := 0; i < 192; i++ {
			if charset[i] != expectedCharset[i] || medium[i] != expectedMedium[i] {
				fmt.Printf("ERROR: frame %d doesn't match renderSquare\n", frame)
				return
			}
		}

		// Get reference for error visualization (DDA vs exact intersection)
		refCharset, refMedium := refRenderSquare(sourceFrame)
		if transform != 0 {
			for row := 1; row <= 17; row++ {
				col0 := refCharset[row]
				col1 := refCharset[64+row]
				col2 := refCharset[128+row]
				medCol0 := refMedium[row]
				medCol1 := refMedium[64+row]
				medCol2 := refMedium[128+row]

				if transform == 1 {
					refCharset[row] = (col1 << 4) | (col2 >> 4)
					refCharset[64+row] = (col2 << 4) | (col0 >> 4)
					refCharset[128+row] = (col0 << 4) | (col1 >> 4)
					refMedium[row] = (medCol1 << 4) | (medCol2 >> 4)
					refMedium[64+row] = (medCol2 << 4) | (medCol0 >> 4)
					refMedium[128+row] = (medCol0 << 4) | (medCol1 >> 4)
				} else {
					refCharset[row] = reverseBits(col2)
					refCharset[64+row] = reverseBits(col1)
					refCharset[128+row] = reverseBits(col0)
					refMedium[row] = reverseBits(medCol2)
					refMedium[64+row] = reverseBits(medCol1)
					refMedium[128+row] = reverseBits(medCol0)
				}
			}
		}
		refs[frame] = refCharset
		refMeds[frame] = refMedium
	}

	// Create sprite sheet: 7 columns × 6 rows
	pixelSize := 8
	frameSize := 24 * pixelSize
	cols := 7
	rows := 6
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
	for i := 0; i < 41; i++ {
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
	for i := 0; i < 41; i++ {
		totalDiffs += diffCharsets(frames[i], refs[i])
	}
	avgDiffs := float64(totalDiffs) / 41.0
	errorRate := avgDiffs * 100.0 / (24.0 * 24.0)

	avgCycles := totalCycles / 41
	fmt.Printf("%s: %.1f FPS @ 1MHz (%d cyc/frame, %.2f%% error)\n",
		path, 1000000.0/float64(avgCycles), avgCycles, errorRate)
}
