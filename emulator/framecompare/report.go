package framecompare

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

const TileSize = 24

type Report struct {
	Frame      int
	Total      int
	Mismatched *image.Gray
	Tiles      [][]int
	ZX         *image.Gray
	C64        *image.RGBA
}

// Compares the saved C64 and ZX frames the way the validator does and reports where
// they differ. Both PNGs must exist in dir.
func Diff(dir string, frame int) (*Report, error) {
	validator := NewValidator(dir, 0)
	zx, found, err := validator.loadZX(frame)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("no ZX frame %d in %s", frame, dir)
	}
	c64, err := loadC64(filepath.Join(dir, fmt.Sprintf("frame_%05d_c64.png", frame)))
	if err != nil {
		return nil, err
	}

	bounds := c64.Bounds()
	report := &Report{Frame: frame, Mismatched: image.NewGray(image.Rect(0, 0, bounds.Dx(), bounds.Dy())), ZX: zx, C64: c64}
	report.Tiles = make([][]int, (bounds.Dy()+TileSize-1)/TileSize)
	for row := range report.Tiles {
		report.Tiles[row] = make([]int, (bounds.Dx()+TileSize-1)/TileSize)
	}
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			if !pixelMatches(zx, x, y, c64.RGBAAt(bounds.Min.X+x, bounds.Min.Y+y)) {
				report.Mismatched.SetGray(x, y, color.Gray{Y: 255})
				report.Tiles[y/TileSize][x/TileSize]++
				report.Total++
			}
		}
	}
	return report, nil
}

// Writes the C64 frame, the ZX frame and the mismatches (red) side by side, scaled up
func (report *Report) WriteImage(path string, scale int) error {
	bounds := report.C64.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	const gap = 4
	img := image.NewRGBA(image.Rect(0, 0, (3*width+2*gap)*scale, height*scale))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 40, G: 60, B: 160, A: 255}), image.Point{}, draw.Src)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c64 := report.C64.RGBAAt(bounds.Min.X+x, bounds.Min.Y+y)
			zx := report.ZX.GrayAt(x, y).Y
			mismatch := color.RGBA{R: zx / 3, G: zx / 3, B: zx / 3, A: 255}
			if report.Mismatched.GrayAt(x, y).Y != 0 {
				mismatch = color.RGBA{R: 255, A: 255}
			}
			fillScaled(img, 0, x, y, scale, c64)
			fillScaled(img, width+gap, x, y, scale, color.RGBA{R: zx, G: zx, B: zx, A: 255})
			fillScaled(img, 2*(width+gap), x, y, scale, mismatch)
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

func fillScaled(img *image.RGBA, offsetX, x, y, scale int, shade color.RGBA) {
	for dy := 0; dy < scale; dy++ {
		for dx := 0; dx < scale; dx++ {
			img.SetRGBA((offsetX+x)*scale+dx, y*scale+dy, shade)
		}
	}
}

func loadC64(path string) (*image.RGBA, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: save it with the emulator's -c64 -save-frames flag", err)
	}
	defer file.Close()
	decoded, err := png.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	bounds := decoded.Bounds()
	img := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(img, img.Bounds(), decoded, bounds.Min, draw.Src)
	return img, nil
}
