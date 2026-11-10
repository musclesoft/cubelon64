package c64

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

// CaptureScreen captures the C64 screen to a PNG file
func CaptureScreen(mem *Memory, filename string) error {
	croppedImg := RenderScreen(mem)
	if croppedImg == nil {
		return nil
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, croppedImg)
}

// RenderScreen renders the C64 screen, cropped to the 192x192 center region.
// Supports character mode and bitmap mode. Returns nil without a VIC.
func RenderScreen(mem *Memory) *image.RGBA {
	if mem.VIC == nil {
		return nil
	}

	vic := mem.VIC

	// C64 display is 320x200 pixels (or 160x200 in multicolor mode)
	// Render full frame first
	fullImg := image.NewRGBA(image.Rect(0, 0, 320, 200))

	if !vic.IsDisplayEnabled() {
		// Display disabled - fill with border color
		borderCol := C64Palette[vic.Registers[VIC_BORDER_COL]&0x0F]
		for y := 0; y < 200; y++ {
			for x := 0; x < 320; x++ {
				fullImg.Set(x, y, color.RGBA{borderCol.R, borderCol.G, borderCol.B, 255})
			}
		}
	} else if vic.IsBitmapMode() {
		// Bitmap mode
		renderBitmapMode(fullImg, mem, vic)
	} else {
		// Character mode
		renderCharacterMode(fullImg, vic)
	}

	fullImg = scrollDown(fullImg, int(vic.Registers[VIC_CTRL1]&7)-defaultYScroll, vic.Registers[VIC_BG_COL0]&0x0F)

	// Render sprites on top
	renderSprites(fullImg, mem, vic)

	// Crop to 192x192 center region
	// X: (320-192)/2 = 64, Y: (200-192)/2 = 4
	cropX, cropY := 64, 4
	cropWidth, cropHeight := 192, 192
	croppedImg := image.NewRGBA(image.Rect(0, 0, cropWidth, cropHeight))

	for y := 0; y < cropHeight; y++ {
		for x := 0; x < cropWidth; x++ {
			croppedImg.Set(x, y, fullImg.At(cropX+x, cropY+y))
		}
	}

	return croppedImg
}

// renderCharacterMode renders the screen in character mode
func renderCharacterMode(img *image.RGBA, vic *VIC) {
	bgColor := C64Palette[vic.Registers[VIC_BG_COL0]&0x0F]

	// 40x25 characters, each 8x8 pixels
	for row := 0; row < 25; row++ {
		textRow := vic.textRowToRender(row)

		for col := 0; col < 40; col++ {
			fgColor := C64Palette[textRow.Colors[col]]

			for line := 0; line < 8; line++ {
				charLine := textRow.Glyphs[col][line]

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

// The image's first row is raster line 51, where the text starts with YSCROLL 3.
const (
	defaultYScroll   = 3
	firstDisplayLine = 51
)

// scrollDown moves the display down by dy rows (up when negative), filling the
// rows uncovered at the edge with the background color.
func scrollDown(img *image.RGBA, dy int, backgroundColor byte) *image.RGBA {
	if dy == 0 {
		return img
	}

	background := C64Palette[backgroundColor]
	shifted := image.NewRGBA(img.Bounds())
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			sourceY := y - dy
			if sourceY >= 0 && sourceY < 200 {
				shifted.Set(x, y, img.At(x, sourceY))
			} else {
				shifted.Set(x, y, color.RGBA{background.R, background.G, background.B, 255})
			}
		}
	}
	return shifted
}

// renderSprites draws the sprites line by line from what the VIC recorded for each
// line. Without a recording there is nothing to draw: which data a sprite shows on a
// line depends on how the VIC fetched it.
func renderSprites(img *image.RGBA, mem *Memory, vic *VIC) {
	if !vic.Recording {
		return
	}
	for y := 0; y < 200; y++ {
		renderSpriteLine(img, &vic.Lines[firstDisplayLine+y], y)
	}
}

func renderSpriteLine(img *image.RGBA, state *LineState, y int) {
	registers := &state.Registers
	bgColor := C64Palette[registers[VIC_BG_COL0]&0x0F]

	for sprite := 0; sprite < 8; sprite++ {
		if !state.Sprites[sprite].Display {
			continue
		}

		// VIC-II sprite coords include border: X=24 is left of char area.
		// The emulator renders chars starting at pixel (0,0), so subtract the offset.
		spriteX := int(registers[VIC_SPR0_X+sprite*2])
		if registers[VIC_SPR_HI_X]&(1<<sprite) != 0 {
			spriteX |= 0x100
		}

		spriteColor := C64Palette[registers[VIC_SPR0_COL+sprite]&0x0F]
		behindForeground := registers[VIC_SPR_DP]&(1<<sprite) != 0

		for col := 0; col < 3; col++ {
			spriteByte := state.Sprites[sprite].Data[col]

			for bit := 0; bit < 8; bit++ {
				if (spriteByte>>(7-bit))&1 == 0 {
					continue
				}
				x := spriteX - 24 + col*8 + bit
				if x < 0 || x >= 320 {
					continue
				}
				if behindForeground {
					r, g, b, _ := img.At(x, y).RGBA()
					if uint8(r>>8) != bgColor.R || uint8(g>>8) != bgColor.G || uint8(b>>8) != bgColor.B {
						continue
					}
				}
				img.Set(x, y, color.RGBA{spriteColor.R, spriteColor.G, spriteColor.B, 255})
			}
		}
	}
}
