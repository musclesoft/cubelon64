package spectrum

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

// ZX Spectrum palette
var Palette = []color.RGBA{
	{0, 0, 0, 255},       // Black
	{0, 0, 215, 255},     // Blue
	{215, 0, 0, 255},     // Red
	{215, 0, 215, 255},   // Magenta
	{0, 215, 0, 255},     // Green
	{0, 215, 215, 255},   // Cyan
	{215, 215, 0, 255},   // Yellow
	{215, 215, 215, 255}, // White
	{0, 0, 0, 255},       // Bright Black
	{0, 0, 255, 255},     // Bright Blue
	{255, 0, 0, 255},     // Bright Red
	{255, 0, 255, 255},   // Bright Magenta
	{0, 255, 0, 255},     // Bright Green
	{0, 255, 255, 255},   // Bright Cyan
	{255, 255, 0, 255},   // Bright Yellow
	{255, 255, 255, 255}, // Bright White
}

// CaptureScreen captures screen from the active screen bank
func CaptureScreen(mem *Memory, filename string) error {
	img := image.NewRGBA(image.Rect(0, 0, 256, 192))

	// Get screen data from the active screen bank
	screenBank := mem.ScreenPageIndex
	screenData := mem.Banks[screenBank][:]

	// Screen layout: 6144 bytes bitmap + 768 bytes attributes
	for y := 0; y < 192; y++ {
		// ZX Spectrum's weird screen layout
		section := y / 64
		row := (y % 64) / 8
		line := y % 8

		lineOffset := section*2048 + line*256 + row*32
		attrOffset := 0x1800 + (y/8)*32

		for x := 0; x < 32; x++ {
			pixelByte := screenData[lineOffset+x]
			attrByte := screenData[attrOffset+x]

			ink := attrByte & 0x07
			paper := (attrByte >> 3) & 0x07
			bright := (attrByte >> 6) & 0x01

			inkColor := Palette[ink+bright*8]
			paperColor := Palette[paper+bright*8]

			for bit := 0; bit < 8; bit++ {
				pixelOn := (pixelByte >> (7 - bit)) & 1
				var col color.RGBA
				if pixelOn == 1 {
					col = inkColor
				} else {
					col = paperColor
				}
				img.Set(x*8+bit, y, col)
			}
		}
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, img)
}

// FrameBuffer holds a progressive screen capture during frame rendering
type FrameBuffer struct {
	Image         *image.Paletted
	Mem           *Memory
	LastCapturedLine int
}

// NewFrameBuffer creates a new frame buffer for progressive capture
func NewFrameBuffer(mem *Memory) *FrameBuffer {
	// 1-bit monochrome palette (black and white)
	palette := color.Palette{
		color.RGBA{0, 0, 0, 255},       // 0: Black
		color.RGBA{255, 255, 255, 255}, // 1: White
	}

	return &FrameBuffer{
		Image: image.NewPaletted(image.Rect(0, 0, 256, 192), palette),
		Mem:   mem,
		LastCapturedLine: -1,
	}
}

// CaptureUpToLine captures all scanlines up to the specified line
func (fb *FrameBuffer) CaptureUpToLine(line int) {
	if line > 191 {
		line = 191
	}

	screenBank := fb.Mem.ScreenPageIndex
	screenData := fb.Mem.Banks[screenBank][:]

	// Capture all lines from LastCapturedLine+1 to line
	for y := fb.LastCapturedLine + 1; y <= line; y++ {
		section := y / 64
		row := (y % 64) / 8
		scanline := y % 8

		lineOffset := section*2048 + scanline*256 + row*32
		attrOffset := 0x1800 + (y/8)*32

		for x := 0; x < 32; x++ {
			pixelByte := screenData[lineOffset+x]
			attrByte := screenData[attrOffset+x]

			ink := attrByte & 0x07
			paper := (attrByte >> 3) & 0x07
			bright := (attrByte >> 6) & 0x01

			// Check if ink/paper are white (color 7, or 15 with bright)
			inkIsWhite := (ink == 7) || (ink+bright*8 == 15)
			paperIsWhite := (paper == 7) || (paper+bright*8 == 15)

			// Set pixels directly using palette index (0=black, 1=white)
			pixelOffset := y*256 + x*8
			for bit := 0; bit < 8; bit++ {
				pixelOn := (pixelByte >> (7 - bit)) & 1
				var paletteIndex uint8
				if pixelOn == 1 {
					if inkIsWhite {
						paletteIndex = 1
					} else {
						paletteIndex = 0
					}
				} else {
					if paperIsWhite {
						paletteIndex = 1
					} else {
						paletteIndex = 0
					}
				}
				fb.Image.Pix[pixelOffset+bit] = paletteIndex
			}
		}
	}

	fb.LastCapturedLine = line
}

// Save writes the frame buffer to a PNG file
func (fb *FrameBuffer) Save(filename string) error {
	// Ensure all lines are captured
	fb.CaptureUpToLine(191)

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, fb.Image)
}

// SaveCropped writes a cropped portion of the frame buffer to a PNG file
func (fb *FrameBuffer) SaveCropped(filename string, x, y, width, height int) error {
	// Ensure all lines are captured
	fb.CaptureUpToLine(191)

	// Create cropped image
	cropped := image.NewRGBA(image.Rect(0, 0, width, height))
	for dy := 0; dy < height; dy++ {
		for dx := 0; dx < width; dx++ {
			cropped.Set(dx, dy, fb.Image.At(x+dx, y+dy))
		}
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, cropped)
}

// CloneCropped creates a copy of the cropped image data (for async saving)
func (fb *FrameBuffer) CloneCropped(x, y, width, height int) *image.RGBA {
	cropped := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fast copy: copy entire scanlines using raw pixel data
	// Each pixel is 4 bytes (RGBA), so we copy width*4 bytes per row
	srcStride := fb.Image.Stride
	dstStride := cropped.Stride
	bytesPerRow := width * 4

	for row := 0; row < height; row++ {
		srcOffset := (y+row)*srcStride + x*4
		dstOffset := row * dstStride
		copy(cropped.Pix[dstOffset:dstOffset+bytesPerRow],
		     fb.Image.Pix[srcOffset:srcOffset+bytesPerRow])
	}

	return cropped
}

// CloneFull creates a copy of the full image data (for async saving)
func (fb *FrameBuffer) CloneFull() *image.RGBA {
	cloned := image.NewRGBA(fb.Image.Bounds())
	copy(cloned.Pix, fb.Image.Pix)
	return cloned
}

// CaptureAllBanks captures all 8 RAM banks in a single image (4x2 grid)
func CaptureAllBanks(mem *Memory, filename string) error {
	// 4 banks per row, 2 rows = 1024x384 image
	img := image.NewRGBA(image.Rect(0, 0, 1024, 384))

	for bank := 0; bank < 8; bank++ {
		bankX := (bank % 4) * 256
		bankY := (bank / 4) * 192

		screenData := mem.Banks[bank][:]

		for y := 0; y < 192; y++ {
			section := y / 64
			row := (y % 64) / 8
			line := y % 8

			lineOffset := section*2048 + line*256 + row*32
			attrOffset := 0x1800 + (y/8)*32

			for x := 0; x < 32; x++ {
				pixelByte := screenData[lineOffset+x]
				attrByte := screenData[attrOffset+x]

				ink := attrByte & 0x07
				paper := (attrByte >> 3) & 0x07
				bright := (attrByte >> 6) & 0x01

				inkColor := Palette[ink+bright*8]
				paperColor := Palette[paper+bright*8]

				for bit := 0; bit < 8; bit++ {
					pixelOn := (pixelByte >> (7 - bit)) & 1
					var col color.RGBA
					if pixelOn == 1 {
						col = inkColor
					} else {
						col = paperColor
					}
					img.Set(bankX+x*8+bit, bankY+y, col)
				}
			}
		}
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, img)
}
