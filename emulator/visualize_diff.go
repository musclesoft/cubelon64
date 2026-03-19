package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: visualize_diff <expected.bin> <actual.bin> <output.png>")
		os.Exit(1)
	}

	expectedPath := os.Args[1]
	actualPath := os.Args[2]
	outputPath := os.Args[3]

	// Read expected data
	expectedData, err := os.ReadFile(expectedPath)
	if err != nil {
		fmt.Printf("Error reading expected: %v\n", err)
		os.Exit(1)
	}

	// Read actual data
	actualData, err := os.ReadFile(actualPath)
	if err != nil {
		fmt.Printf("Error reading actual: %v\n", err)
		os.Exit(1)
	}

	if len(expectedData) != len(actualData) {
		fmt.Printf("Size mismatch: expected %d, actual %d\n", len(expectedData), len(actualData))
	}

	// Sequential format: 17 rows × 6 bytes (col0_full, col1_full, col2_full, col0_med, col1_med, col2_med)
	// Visualize as 24×24 output pixels

	// Create image: 3 panels side-by-side (expected, actual, diff)
	width := 24 * 3 * 4 // 3 panels × 24 pixels × 4x scale
	height := 24 * 4     // 24 pixels × 4x scale
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Background
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{40, 40, 40, 255})
		}
	}

	// Draw each panel
	for row := 1; row <= 17; row++ {
		offset := (row - 1) * 6
		if offset+5 >= len(expectedData) || offset+5 >= len(actualData) {
			continue
		}

		// Parse charset and sprite data
		for col := 0; col < 3; col++ {
			expFull := expectedData[offset+col]
			actFull := actualData[offset+col]
			expMed := expectedData[offset+3+col]
			actMed := actualData[offset+3+col]

			// Render 8 pixels per byte
			for bit := 0; bit < 8; bit++ {
				x := col*8 + bit
				y := row - 1 // Sequential format uses rows 1-17, output to 0-16

				// Check bit state
				expFullBit := (expFull >> (7 - bit)) & 1
				actFullBit := (actFull >> (7 - bit)) & 1
				expMedBit := (expMed >> (7 - bit)) & 1
				actMedBit := (actMed >> (7 - bit)) & 1

				// Determine pixel state (0=bg, 1=med, 2=full)
				expState := 0
				if expFullBit == 0 && expMedBit == 0 {
					expState = 2 // Full brightness (both cleared)
				} else if expFullBit == 0 {
					expState = 1 // Medium (charset cleared, sprite not)
				}

				actState := 0
				if actFullBit == 0 && actMedBit == 0 {
					actState = 2
				} else if actFullBit == 0 {
					actState = 1
				}

				// Colors
				bgColor := color.RGBA{20, 20, 20, 255}
				medColor := color.RGBA{100, 100, 255, 255}
				fullColor := color.RGBA{200, 200, 255, 255}
				errorColor := color.RGBA{255, 50, 50, 255}

				expColor := bgColor
				if expState == 1 {
					expColor = medColor
				} else if expState == 2 {
					expColor = fullColor
				}

				actColor := bgColor
				if actState == 1 {
					actColor = medColor
				} else if actState == 2 {
					actColor = fullColor
				}

				diffColor := bgColor
				if expState != actState {
					diffColor = errorColor
				} else if expState == 1 {
					diffColor = medColor
				} else if expState == 2 {
					diffColor = fullColor
				}

				// Draw scaled pixels
				scale := 4
				// Panel 1: Expected
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						img.Set(x*scale+dx, y*scale+dy, expColor)
					}
				}
				// Panel 2: Actual
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						img.Set((24*scale)+x*scale+dx, y*scale+dy, actColor)
					}
				}
				// Panel 3: Diff (errors in red)
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						img.Set((48*scale)+x*scale+dx, y*scale+dy, diffColor)
					}
				}
			}
		}
	}

	// Save image
	f, err := os.Create(outputPath)
	if err != nil {
		fmt.Printf("Error creating output: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		fmt.Printf("Error encoding PNG: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Visualization saved to %s\n", outputPath)

	// Print statistics
	diffs := 0
	for i := 0; i < len(expectedData) && i < len(actualData); i++ {
		if expectedData[i] != actualData[i] {
			diffs++
		}
	}
	fmt.Printf("Differences: %d/%d bytes (%.1f%%)\n", diffs, len(expectedData), float64(diffs)*100/float64(len(expectedData)))
}
