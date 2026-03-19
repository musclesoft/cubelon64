package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

// Visualize all 41 golden frames
func visualizeFrames() {
	const scale = 8 // Scale up each pixel for visibility

	// C64 colors
	black := color.RGBA{0, 0, 0, 255}
	darkGrey := color.RGBA{98, 98, 98, 255}
	lightGrey := color.RGBA{187, 187, 187, 255}

	for frame := 0; frame <= 40; frame++ {
		// Read charset and sprite data
		charsetFile := fmt.Sprintf("golden/frame%d_charset.bin", frame)
		spriteFile := fmt.Sprintf("golden/frame%d_sprite.bin", frame)

		charset, err := os.ReadFile(charsetFile)
		if err != nil {
			fmt.Printf("Error reading %s: %v\n", charsetFile, err)
			continue
		}

		sprite, err := os.ReadFile(spriteFile)
		if err != nil {
			fmt.Printf("Error reading %s: %v\n", spriteFile, err)
			continue
		}

		// Create image: 24x24 pixels
		img := image.NewRGBA(image.Rect(0, 0, 24*scale, 24*scale))

		// Fill with light grey background
		for y := 0; y < 24*scale; y++ {
			for x := 0; x < 24*scale; x++ {
				img.Set(x, y, lightGrey)
			}
		}

		// Render 3x3 character grid (columns 0, 1, 2)
		for row := 0; row < 24; row++ {
			for col := 0; col < 3; col++ {
				charByte := byte(0xFF) // Default background
				if row >= 1 && row <= 17 {
					// Row 1-17 read from indices 0-16 (n->n+1 pattern like sprites)
					charByte = charset[col*64+(row-1)]
				} else if row == 0 || row >= 18 {
					charByte = 0xFF // Background
				}

				// Draw 8 pixels from this byte
				for bit := 0; bit < 8; bit++ {
					pixelOn := (charByte & (0x80 >> bit)) == 0
					pixelColor := lightGrey
					if pixelOn {
						pixelColor = black
					}

					// Draw scaled pixel
					px := col*8 + bit
					py := row
					for sy := 0; sy < scale; sy++ {
						for sx := 0; sx < scale; sx++ {
							img.Set(px*scale+sx, py*scale+sy, pixelColor)
						}
					}
				}
			}
		}

		// Overlay sprite (dark grey antialiasing)
		// Sprite is 21 rows × 3 bytes in row-major format
		for row := 0; row < 21; row++ {
			for col := 0; col < 3; col++ {
				if row*3+col >= len(sprite) {
					continue
				}
				spriteByte := sprite[row*3+col]

				// Draw 8 pixels from this byte
				for bit := 0; bit < 8; bit++ {
					pixelOn := (spriteByte & (0x80 >> bit)) == 0
					if pixelOn {
						// Overlay dark grey sprite pixel
						px := col*8 + bit
						py := row + 1 // Sprites start at row 1
						for sy := 0; sy < scale; sy++ {
							for sx := 0; sx < scale; sx++ {
								img.Set(px*scale+sx, py*scale+sy, darkGrey)
							}
						}
					}
				}
			}
		}

		// Save image
		outFile := fmt.Sprintf("golden/frame%d_vis.png", frame)
		f, err := os.Create(outFile)
		if err != nil {
			fmt.Printf("Error creating %s: %v\n", outFile, err)
			continue
		}

		err = png.Encode(f, img)
		f.Close()
		if err != nil {
			fmt.Printf("Error encoding %s: %v\n", outFile, err)
			continue
		}

		fmt.Printf("Generated %s\n", outFile)
	}
}
