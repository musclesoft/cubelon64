package c64

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

// CaptureScreen captures the C64 screen to a PNG file
// Supports character mode and bitmap mode
func CaptureScreen(mem *Memory, filename string) error {
	if mem.VIC == nil {
		return nil
	}

	vic := mem.VIC

	// C64 display is 320x200 pixels (or 160x200 in multicolor mode)
	img := image.NewRGBA(image.Rect(0, 0, 320, 200))

	if !vic.IsDisplayEnabled() {
		// Display disabled - fill with border color
		borderCol := C64Palette[vic.Registers[VIC_BORDER_COL]&0x0F]
		for y := 0; y < 200; y++ {
			for x := 0; x < 320; x++ {
				img.Set(x, y, color.RGBA{borderCol.R, borderCol.G, borderCol.B, 255})
			}
		}
	} else if vic.IsBitmapMode() {
		// Bitmap mode
		renderBitmapMode(img, mem, vic)
	} else {
		// Character mode
		renderCharacterMode(img, mem, vic)
	}

	// Render sprites on top
	renderSprites(img, mem, vic)

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, img)
}

// renderCharacterMode renders the screen in character mode
func renderCharacterMode(img *image.RGBA, mem *Memory, vic *VIC) {
	videoMatrix := vic.GetVideoMatrixBase()
	charBase := vic.GetCharacterBase()
	bgColor := C64Palette[vic.Registers[VIC_BG_COL0]&0x0F]

	// 40x25 characters, each 8x8 pixels
	for row := 0; row < 25; row++ {
		for col := 0; col < 40; col++ {
			screenOffset := videoMatrix + uint16(row*40+col)
			colorOffset := uint16(row*40 + col)

			charCode := mem.RAM[screenOffset]
			colorCode := mem.ColorRAM[colorOffset] & 0x0F

			// Get character data from character ROM or RAM
			charAddr := charBase + uint16(charCode)*8

			fgColor := C64Palette[colorCode]

			// Render 8x8 character
			for line := 0; line < 8; line++ {
				var charLine byte
				if charBase >= 0x1000 && charBase < 0x2000 {
					// Character ROM area
					charLine = mem.CHARSET[charAddr-0x1000+uint16(line)]
				} else {
					charLine = mem.RAM[charAddr+uint16(line)]
				}

				for bit := 0; bit < 8; bit++ {
					x := col*8 + bit
					y := row*8 + line

					var pixelColor color.RGBA
					if (charLine>>(7-bit))&1 != 0 {
						pixelColor = color.RGBA{fgColor.R, fgColor.G, fgColor.B, 255}
					} else {
						pixelColor = color.RGBA{bgColor.R, bgColor.G, bgColor.B, 255}
					}

					img.Set(x, y, pixelColor)
				}
			}
		}
	}
}

// renderBitmapMode renders the screen in bitmap mode
func renderBitmapMode(img *image.RGBA, mem *Memory, vic *VIC) {
	bitmapBase := vic.GetBitmapBase()
	videoMatrix := vic.GetVideoMatrixBase()
	isMulticolor := vic.IsMulticolorMode()

	// 320x200 bitmap (or 160x200 multicolor)
	for row := 0; row < 25; row++ {
		for col := 0; col < 40; col++ {
			// Each 8x8 cell
			colorAddr := videoMatrix + uint16(row*40+col)
			colorData := mem.RAM[colorAddr]

			fgColor := C64Palette[(colorData>>4)&0x0F]
			bgColor := C64Palette[colorData&0x0F]

			for line := 0; line < 8; line++ {
				bitmapAddr := bitmapBase + uint16(row*320+col*8+line)
				bitmapData := mem.RAM[bitmapAddr]

				if isMulticolor {
					// Multicolor bitmap: 160x200, 4 colors per 8x8 cell
					// Each 2 bits = 1 pixel (4 pixels per byte)
					for bitPair := 0; bitPair < 4; bitPair++ {
						bits := (bitmapData >> (6 - bitPair*2)) & 0x03
						x := col*8 + bitPair*2
						y := row*8 + line

						var pixelColor color.RGBA
						switch bits {
						case 0: // Background color 0
							c := C64Palette[vic.Registers[VIC_BG_COL0]&0x0F]
							pixelColor = color.RGBA{c.R, c.G, c.B, 255}
						case 1: // Upper 4 bits of screen RAM
							pixelColor = color.RGBA{fgColor.R, fgColor.G, fgColor.B, 255}
						case 2: // Lower 4 bits of screen RAM
							pixelColor = color.RGBA{bgColor.R, bgColor.G, bgColor.B, 255}
						case 3: // Color RAM
							colorRAMData := mem.ColorRAM[row*40+col] & 0x0F
							c := C64Palette[colorRAMData]
							pixelColor = color.RGBA{c.R, c.G, c.B, 255}
						}

						// Each pixel is 2 pixels wide in multicolor mode
						img.Set(x, y, pixelColor)
						img.Set(x+1, y, pixelColor)
					}
				} else {
					// Hires bitmap: 320x200, 2 colors per 8x8 cell
					for bit := 0; bit < 8; bit++ {
						x := col*8 + bit
						y := row*8 + line

						var pixelColor color.RGBA
						if (bitmapData>>(7-bit))&1 != 0 {
							pixelColor = color.RGBA{fgColor.R, fgColor.G, fgColor.B, 255}
						} else {
							pixelColor = color.RGBA{bgColor.R, bgColor.G, bgColor.B, 255}
						}

						img.Set(x, y, pixelColor)
					}
				}
			}
		}
	}
}

// renderSprites renders sprites on top of the background
func renderSprites(img *image.RGBA, mem *Memory, vic *VIC) {
	// Sprite rendering (simplified - no collision detection, no priorities)
	for sprite := byte(0); sprite < 8; sprite++ {
		if !vic.IsSpriteEnabled(sprite) {
			continue
		}

		spriteX := int(vic.GetSpriteX(sprite))
		spriteY := int(vic.GetSpriteY(sprite))
		spriteColor := C64Palette[vic.Registers[VIC_SPR0_COL+sprite]&0x0F]

		// Get sprite data pointer from screen RAM
		spritePtr := mem.RAM[vic.GetVideoMatrixBase()+0x3F8+uint16(sprite)]
		spriteDataAddr := uint16(spritePtr) * 64

		// Render 24x21 sprite
		// VIC-II sprite coords include border: X=24 is left of char area, Y=51 is top.
		// The emulator renders chars starting at pixel (0,0), so subtract the offsets.
		screenX := spriteX - 24
		screenY := spriteY - 51
		for row := 0; row < 21; row++ {
			for col := 0; col < 3; col++ {
				spriteByte := mem.RAM[spriteDataAddr+uint16(row*3+col)]

				for bit := 0; bit < 8; bit++ {
					if (spriteByte>>(7-bit))&1 != 0 {
						x := screenX + col*8 + bit
						y := screenY + row

						// Check bounds
						if x >= 0 && x < 320 && y >= 0 && y < 200 {
							img.Set(x, y, color.RGBA{spriteColor.R, spriteColor.G, spriteColor.B, 255})
						}
					}
				}
			}
		}
	}
}
