package main

import (
	"cubelon64-proto/cpu8"
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
// On 6502: table lookup using LDA abs,X (4 cycles)
func reverseBits(b byte) byte {
	cpu8.TableLookup(b) // Table lookup via LDA abs,X
	b = (b&0xF0)>>4 | (b&0x0F)<<4
	b = (b&0xCC)>>2 | (b&0x33)<<2
	b = (b&0xAA)>>1 | (b&0x55)<<1
	return b
}

var precomputed precomputedFrames

// Pre-compute frames 0-10 at startup (renderSquare already outputs at -4px)
func precomputeFrames() {
	for frame := byte(0); frame <= 10; frame++ {
		charset, medium := renderSquare(frame)

		// Store active rows (1-17) - already at -4px from renderSquare
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

// Animate through all 41 frames and show performance
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
		cpu8.ResetCycles()

		var charset [192]byte
		var medium [192]byte

		// Initialize background
		charset[0] = 0xFF
		charset[64] = 0xFF
		charset[128] = 0xFF
		medium[0] = 0xFF
		medium[64] = 0xFF
		medium[128] = 0xFF
		for row := byte(18); row < 24; row++ {
			charset[row] = 0xFF
			charset[64+row] = 0xFF
			charset[128+row] = 0xFF
			medium[row] = 0xFF
			medium[64+row] = 0xFF
			medium[128+row] = 0xFF
		}

		var sourceFrame byte
		if frame <= 10 {
			// Phase 1: rotate right 4px
			sourceFrame = frame
			for row := 1; row <= 17; row++ {
				offset := (row - 1) * 6
				col0 := cpu8.Load(precomputed.data[sourceFrame][offset+0])
				col1 := cpu8.Load(precomputed.data[sourceFrame][offset+1])
				col2 := cpu8.Load(precomputed.data[sourceFrame][offset+2])
				medCol0 := cpu8.Load(precomputed.data[sourceFrame][offset+3])
				medCol1 := cpu8.Load(precomputed.data[sourceFrame][offset+4])
				medCol2 := cpu8.Load(precomputed.data[sourceFrame][offset+5])

				// Rotate right 4px: LSR×4 + ORA + ASL×4 per byte
				temp0, _ := cpu8.Lsr(col0)
				temp0, _ = cpu8.Lsr(temp0)
				temp0, _ = cpu8.Lsr(temp0)
				temp0, _ = cpu8.Lsr(temp0)
				shift2, _ := cpu8.Asl(col2)
				shift2, _ = cpu8.Asl(shift2)
				shift2, _ = cpu8.Asl(shift2)
				shift2, _ = cpu8.Asl(shift2)
				charset[row] = temp0 | shift2 // ORA (not counted by cpu8)
				cpu8.Store(charset[row])

				temp1, _ := cpu8.Lsr(col1)
				temp1, _ = cpu8.Lsr(temp1)
				temp1, _ = cpu8.Lsr(temp1)
				temp1, _ = cpu8.Lsr(temp1)
				shift0, _ := cpu8.Asl(col0)
				shift0, _ = cpu8.Asl(shift0)
				shift0, _ = cpu8.Asl(shift0)
				shift0, _ = cpu8.Asl(shift0)
				charset[64+row] = temp1 | shift0
				cpu8.Store(charset[64+row])

				temp2, _ := cpu8.Lsr(col2)
				temp2, _ = cpu8.Lsr(temp2)
				temp2, _ = cpu8.Lsr(temp2)
				temp2, _ = cpu8.Lsr(temp2)
				shift1, _ := cpu8.Asl(col1)
				shift1, _ = cpu8.Asl(shift1)
				shift1, _ = cpu8.Asl(shift1)
				shift1, _ = cpu8.Asl(shift1)
				charset[128+row] = temp2 | shift1
				cpu8.Store(charset[128+row])

				tempM0, _ := cpu8.Lsr(medCol0)
				tempM0, _ = cpu8.Lsr(tempM0)
				tempM0, _ = cpu8.Lsr(tempM0)
				tempM0, _ = cpu8.Lsr(tempM0)
				shiftM2, _ := cpu8.Asl(medCol2)
				shiftM2, _ = cpu8.Asl(shiftM2)
				shiftM2, _ = cpu8.Asl(shiftM2)
				shiftM2, _ = cpu8.Asl(shiftM2)
				medium[row] = tempM0 | shiftM2
				cpu8.Store(medium[row])

				tempM1, _ := cpu8.Lsr(medCol1)
				tempM1, _ = cpu8.Lsr(tempM1)
				tempM1, _ = cpu8.Lsr(tempM1)
				tempM1, _ = cpu8.Lsr(tempM1)
				shiftM0, _ := cpu8.Asl(medCol0)
				shiftM0, _ = cpu8.Asl(shiftM0)
				shiftM0, _ = cpu8.Asl(shiftM0)
				shiftM0, _ = cpu8.Asl(shiftM0)
				medium[64+row] = tempM1 | shiftM0
				cpu8.Store(medium[64+row])

				tempM2, _ := cpu8.Lsr(medCol2)
				tempM2, _ = cpu8.Lsr(tempM2)
				tempM2, _ = cpu8.Lsr(tempM2)
				tempM2, _ = cpu8.Lsr(tempM2)
				shiftM1, _ := cpu8.Asl(medCol1)
				shiftM1, _ = cpu8.Asl(shiftM1)
				shiftM1, _ = cpu8.Asl(shiftM1)
				shiftM1, _ = cpu8.Asl(shiftM1)
				medium[128+row] = tempM2 | shiftM1
				cpu8.Store(medium[128+row])

				cpu8.LoopOverhead()
			}
		} else if frame <= 20 {
			// Phase 2: horizontal mirror + rotate right 8px
			sourceFrame = 20 - frame
			for row := 1; row <= 17; row++ {
				offset := (row - 1) * 6
				col0 := cpu8.Load(precomputed.data[sourceFrame][offset+0])
				col1 := cpu8.Load(precomputed.data[sourceFrame][offset+1])
				col2 := cpu8.Load(precomputed.data[sourceFrame][offset+2])
				medCol0 := cpu8.Load(precomputed.data[sourceFrame][offset+3])
				medCol1 := cpu8.Load(precomputed.data[sourceFrame][offset+4])
				medCol2 := cpu8.Load(precomputed.data[sourceFrame][offset+5])

				// Horizontal mirror (table lookup) + rotate right 8px = byte reorder
				rev0 := reverseBits(col2) // counts 4 cycles
				rev1 := reverseBits(col1)
				rev2 := reverseBits(col0)
				revMed0 := reverseBits(medCol2)
				revMed1 := reverseBits(medCol1)
				revMed2 := reverseBits(medCol0)

				charset[row] = rev2
				charset[64+row] = rev0
				charset[128+row] = rev1
				medium[row] = revMed2
				medium[64+row] = revMed0
				medium[128+row] = revMed1
				cpu8.Store(charset[row])
				cpu8.Store(charset[64+row])
				cpu8.Store(charset[128+row])
				cpu8.Store(medium[row])
				cpu8.Store(medium[64+row])
				cpu8.Store(medium[128+row])

				cpu8.LoopOverhead()
			}
		} else if frame < 31 {
			// Phase 3: undo -4px storage then rotate right 12px
			sourceFrame = frame - 20
			for row := 1; row <= 17; row++ {
				offset := (row - 1) * 6
				col0 := cpu8.Load(precomputed.data[sourceFrame][offset+0])
				col1 := cpu8.Load(precomputed.data[sourceFrame][offset+1])
				col2 := cpu8.Load(precomputed.data[sourceFrame][offset+2])
				medCol0 := cpu8.Load(precomputed.data[sourceFrame][offset+3])
				medCol1 := cpu8.Load(precomputed.data[sourceFrame][offset+4])
				medCol2 := cpu8.Load(precomputed.data[sourceFrame][offset+5])

				// Total: rotate right 16px = rotate left 8px = byte reorder
				charset[row] = col1
				charset[64+row] = col2
				charset[128+row] = col0
				medium[row] = medCol1
				medium[64+row] = medCol2
				medium[128+row] = medCol0
				cpu8.Store(charset[row])
				cpu8.Store(charset[64+row])
				cpu8.Store(charset[128+row])
				cpu8.Store(medium[row])
				cpu8.Store(medium[64+row])
				cpu8.Store(medium[128+row])

				cpu8.LoopOverhead()
			}
		} else if frame == 31 {
			// Frame 31: undo -4px storage then rotate right 12px
			sourceFrame = 20 - (frame - 20)
			for row := 1; row <= 17; row++ {
				offset := (row - 1) * 6
				col0 := cpu8.Load(precomputed.data[sourceFrame][offset+0])
				col1 := cpu8.Load(precomputed.data[sourceFrame][offset+1])
				col2 := cpu8.Load(precomputed.data[sourceFrame][offset+2])
				medCol0 := cpu8.Load(precomputed.data[sourceFrame][offset+3])
				medCol1 := cpu8.Load(precomputed.data[sourceFrame][offset+4])
				medCol2 := cpu8.Load(precomputed.data[sourceFrame][offset+5])

				// Total: rotate right 16px = rotate left 8px = byte reorder
				charset[row] = col1
				charset[64+row] = col2
				charset[128+row] = col0
				medium[row] = medCol1
				medium[64+row] = medCol2
				medium[128+row] = medCol0
				cpu8.Store(charset[row])
				cpu8.Store(charset[64+row])
				cpu8.Store(charset[128+row])
				cpu8.Store(medium[row])
				cpu8.Store(medium[64+row])
				cpu8.Store(medium[128+row])

				cpu8.LoopOverhead()
			}
		} else {
			// Phase 4: horizontal mirror + rotate left 4px
			sourceFrame = 41 - frame
			for row := 1; row <= 17; row++ {
				offset := (row - 1) * 6
				col0 := cpu8.Load(precomputed.data[sourceFrame][offset+0])
				col1 := cpu8.Load(precomputed.data[sourceFrame][offset+1])
				col2 := cpu8.Load(precomputed.data[sourceFrame][offset+2])
				medCol0 := cpu8.Load(precomputed.data[sourceFrame][offset+3])
				medCol1 := cpu8.Load(precomputed.data[sourceFrame][offset+4])
				medCol2 := cpu8.Load(precomputed.data[sourceFrame][offset+5])

				// Horizontal mirror (table lookup) + rotate left 4px
				rev0 := reverseBits(col2) // counts 4 cycles
				rev1 := reverseBits(col1)
				rev2 := reverseBits(col0)
				revMed0 := reverseBits(medCol2)
				revMed1 := reverseBits(medCol1)
				revMed2 := reverseBits(medCol0)

				// ASL×4 on rev0, LSR×4 on rev1
				shift0, _ := cpu8.Asl(rev0)
				shift0, _ = cpu8.Asl(shift0)
				shift0, _ = cpu8.Asl(shift0)
				shift0, _ = cpu8.Asl(shift0)
				temp1, _ := cpu8.Lsr(rev1)
				temp1, _ = cpu8.Lsr(temp1)
				temp1, _ = cpu8.Lsr(temp1)
				temp1, _ = cpu8.Lsr(temp1)
				charset[row] = shift0 | temp1
				cpu8.Store(charset[row])

				shift1, _ := cpu8.Asl(rev1)
				shift1, _ = cpu8.Asl(shift1)
				shift1, _ = cpu8.Asl(shift1)
				shift1, _ = cpu8.Asl(shift1)
				temp2, _ := cpu8.Lsr(rev2)
				temp2, _ = cpu8.Lsr(temp2)
				temp2, _ = cpu8.Lsr(temp2)
				temp2, _ = cpu8.Lsr(temp2)
				charset[64+row] = shift1 | temp2
				cpu8.Store(charset[64+row])

				shift2, _ := cpu8.Asl(rev2)
				shift2, _ = cpu8.Asl(shift2)
				shift2, _ = cpu8.Asl(shift2)
				shift2, _ = cpu8.Asl(shift2)
				temp0, _ := cpu8.Lsr(rev0)
				temp0, _ = cpu8.Lsr(temp0)
				temp0, _ = cpu8.Lsr(temp0)
				temp0, _ = cpu8.Lsr(temp0)
				charset[128+row] = shift2 | temp0
				cpu8.Store(charset[128+row])

				shiftM0, _ := cpu8.Asl(revMed0)
				shiftM0, _ = cpu8.Asl(shiftM0)
				shiftM0, _ = cpu8.Asl(shiftM0)
				shiftM0, _ = cpu8.Asl(shiftM0)
				tempM1, _ := cpu8.Lsr(revMed1)
				tempM1, _ = cpu8.Lsr(tempM1)
				tempM1, _ = cpu8.Lsr(tempM1)
				tempM1, _ = cpu8.Lsr(tempM1)
				medium[row] = shiftM0 | tempM1
				cpu8.Store(medium[row])

				shiftM1, _ := cpu8.Asl(revMed1)
				shiftM1, _ = cpu8.Asl(shiftM1)
				shiftM1, _ = cpu8.Asl(shiftM1)
				shiftM1, _ = cpu8.Asl(shiftM1)
				tempM2, _ := cpu8.Lsr(revMed2)
				tempM2, _ = cpu8.Lsr(tempM2)
				tempM2, _ = cpu8.Lsr(tempM2)
				tempM2, _ = cpu8.Lsr(tempM2)
				medium[64+row] = shiftM1 | tempM2
				cpu8.Store(medium[64+row])

				shiftM2, _ := cpu8.Asl(revMed2)
				shiftM2, _ = cpu8.Asl(shiftM2)
				shiftM2, _ = cpu8.Asl(shiftM2)
				shiftM2, _ = cpu8.Asl(shiftM2)
				tempM0, _ := cpu8.Lsr(revMed0)
				tempM0, _ = cpu8.Lsr(tempM0)
				tempM0, _ = cpu8.Lsr(tempM0)
				tempM0, _ = cpu8.Lsr(tempM0)
				medium[128+row] = shiftM2 | tempM0
				cpu8.Store(medium[128+row])

				cpu8.LoopOverhead()
			}
		}

		totalCycles += int(cpu8.GetCycles())
		frames[frame] = charset
		mediums[frame] = medium

		// Verification
		var verifySourceFrame byte
		var needRotateR12, needHMirror, phase2 bool

		if frame <= 10 {
			verifySourceFrame = frame
		} else if frame <= 20 {
			verifySourceFrame = 20 - frame
			phase2 = true
		} else if frame == 31 {
			verifySourceFrame = 20 - (frame - 20)
			needRotateR12 = true
		} else if frame < 32 {
			verifySourceFrame = frame - 20
			needRotateR12 = true
		} else {
			verifySourceFrame = 41 - frame
			needHMirror = true
		}

		expectedCharset, expectedMedium := renderSquare(verifySourceFrame)
		if !phase2 {
			for row := 1; row <= 17; row++ {
				col0 := expectedCharset[row]
				col1 := expectedCharset[64+row]
				col2 := expectedCharset[128+row]
				medCol0 := expectedMedium[row]
				medCol1 := expectedMedium[64+row]
				medCol2 := expectedMedium[128+row]

				expectedCharset[row] = (col0 >> 4) | (col2 << 4)
				expectedCharset[64+row] = (col1 >> 4) | (col0 << 4)
				expectedCharset[128+row] = (col2 >> 4) | (col1 << 4)
				expectedMedium[row] = (medCol0 >> 4) | (medCol2 << 4)
				expectedMedium[64+row] = (medCol1 >> 4) | (medCol0 << 4)
				expectedMedium[128+row] = (medCol2 >> 4) | (medCol1 << 4)
			}
		}
		if phase2 || needRotateR12 || needHMirror {
			for row := 1; row <= 17; row++ {
				col0 := expectedCharset[row]
				col1 := expectedCharset[64+row]
				col2 := expectedCharset[128+row]
				medCol0 := expectedMedium[row]
				medCol1 := expectedMedium[64+row]
				medCol2 := expectedMedium[128+row]

				if phase2 {
					rev0 := reverseBits(col2)
					rev1 := reverseBits(col1)
					rev2 := reverseBits(col0)
					revMed0 := reverseBits(medCol2)
					revMed1 := reverseBits(medCol1)
					revMed2 := reverseBits(medCol0)
					expectedCharset[row] = rev2
					expectedCharset[64+row] = rev0
					expectedCharset[128+row] = rev1
					expectedMedium[row] = revMed2
					expectedMedium[64+row] = revMed0
					expectedMedium[128+row] = revMed1
				} else if needRotateR12 {
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

		refCharset, refMedium := refRenderSquare(verifySourceFrame)
		if !phase2 {
			for row := 1; row <= 17; row++ {
				col0 := refCharset[row]
				col1 := refCharset[64+row]
				col2 := refCharset[128+row]
				medCol0 := refMedium[row]
				medCol1 := refMedium[64+row]
				medCol2 := refMedium[128+row]

				refCharset[row] = (col0 >> 4) | (col2 << 4)
				refCharset[64+row] = (col1 >> 4) | (col0 << 4)
				refCharset[128+row] = (col2 >> 4) | (col1 << 4)
				refMedium[row] = (medCol0 >> 4) | (medCol2 << 4)
				refMedium[64+row] = (medCol1 >> 4) | (medCol0 << 4)
				refMedium[128+row] = (medCol2 >> 4) | (medCol1 << 4)
			}
		}
		if phase2 || needRotateR12 || needHMirror {
			for row := 1; row <= 17; row++ {
				col0 := refCharset[row]
				col1 := refCharset[64+row]
				col2 := refCharset[128+row]
				medCol0 := refMedium[row]
				medCol1 := refMedium[64+row]
				medCol2 := refMedium[128+row]

				if phase2 {
					rev0 := reverseBits(col2)
					rev1 := reverseBits(col1)
					rev2 := reverseBits(col0)
					revMed0 := reverseBits(medCol2)
					revMed1 := reverseBits(medCol1)
					revMed2 := reverseBits(medCol0)
					refCharset[row] = rev2
					refCharset[64+row] = rev0
					refCharset[128+row] = rev1
					refMedium[row] = revMed2
					refMedium[64+row] = revMed0
					refMedium[128+row] = revMed1
				} else if needRotateR12 {
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

	// Create sprite sheet
	pixelSize := 8
	frameSize := 24 * pixelSize
	cols := 7
	rows := 6
	sep := 1
	sheetWidth := cols*frameSize + (cols-1)*sep
	sheetHeight := rows*frameSize + (rows-1)*sep
	sheet := image.NewRGBA(image.Rect(0, 0, sheetWidth, sheetHeight))

	separator := color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	for y := 0; y < sheetHeight; y++ {
		for x := 0; x < sheetWidth; x++ {
			sheet.Set(x, y, separator)
		}
	}

	for i := 0; i < 41; i++ {
		col := i % cols
		row := i / cols
		frameImg := charsetToImage(frames[i], mediums[i], pixelSize, black, darkGrey, lightGrey)

		for outY := 0; outY < 24; outY++ {
			for outX := 0; outX < 24; outX++ {
				protCount := getCount(frames[i], mediums[i], outX, outY)
				refCount := getCount(refs[i], refMeds[i], outX, outY)
				errMag := protCount - refCount
				if errMag < 0 {
					errMag = -errMag
				}
				if errMag > 0 {
					radius := errMag
					if radius > 2 {
						radius = 2
					}
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

		offsetX := col * (frameSize + sep)
		offsetY := row * (frameSize + sep)
		for y := 0; y < frameSize; y++ {
			for x := 0; x < frameSize; x++ {
				sheet.Set(offsetX+x, offsetY+y, frameImg.At(x, y))
			}
		}
	}

	path := "animation.png"
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	png.Encode(f, sheet)
	f.Close()

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
