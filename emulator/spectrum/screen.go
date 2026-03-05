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
	Image         *image.RGBA
	Mem           *Memory
	LastCapturedLine int
}

// NewFrameBuffer creates a new frame buffer for progressive capture
func NewFrameBuffer(mem *Memory) *FrameBuffer {
	return &FrameBuffer{
		Image: image.NewRGBA(image.Rect(0, 0, 256, 192)),
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
				fb.Image.Set(x*8+bit, y, col)
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
