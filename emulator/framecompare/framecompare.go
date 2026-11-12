package framecompare

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

const (
	ExpectedLastMatchingFrame = 1419
	ExpectedZXFrameCount      = 3052
	MaxMismatchedPixels       = 504

	blackBelow     = 40
	lightFrom      = 150
	litFrom        = 128
	neighborRadius = 1
	neighborPixels = (2*neighborRadius + 1) * (2*neighborRadius + 1)
)

type Validator struct {
	LastMatchingFrame int

	dir        string
	failed     bool
	startFrame int
}

func NewValidator(dir string, startFrame int) *Validator {
	return &Validator{dir: dir, LastMatchingFrame: -1, startFrame: startFrame}
}

func (validator *Validator) ZXFrameCount() int {
	matches, err := filepath.Glob(filepath.Join(validator.dir, "frame_*_zx.png"))
	if err != nil {
		panic(err)
	}
	return len(matches)
}

func (validator *Validator) Failed() bool {
	return validator.failed
}

func (validator *Validator) Check(frame int, c64 *image.RGBA) {
	if validator.failed || frame < validator.startFrame {
		return
	}
	zx, found, err := validator.loadZX(frame)
	if err != nil {
		panic(err)
	}
	if !found {
		return
	}
	if countMismatches(zx, c64) > MaxMismatchedPixels {
		validator.failed = true
		return
	}
	validator.LastMatchingFrame = frame
}

func (validator *Validator) Verify() {
	last := validator.LastMatchingFrame
	if last < 0 {
		panic("no C64 frame was compared with a ZX frame: run the ZX emulator first to save frames/frame_NNNNN_zx.png")
	}
	fmt.Printf("Last matching frame: %d (expected %d)\n", last, ExpectedLastMatchingFrame)
	if last < ExpectedLastMatchingFrame {
		panic(fmt.Sprintf("C64 frames regressed: last matching frame %d, expected %d", last, ExpectedLastMatchingFrame))
	}
	if last > ExpectedLastMatchingFrame {
		panic(fmt.Sprintf("more C64 frames match: set ExpectedLastMatchingFrame to %d in emulator/framecompare", last))
	}
}

func (validator *Validator) loadZX(frame int) (*image.Gray, bool, error) {
	path := filepath.Join(validator.dir, fmt.Sprintf("frame_%05d_zx.png", frame))
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	decoded, err := png.Decode(file)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	bounds := decoded.Bounds()
	gray := image.NewGray(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			red, green, blue, _ := decoded.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			gray.Pix[y*gray.Stride+x] = uint8((red + green + blue) / 3 >> 8)
		}
	}
	return gray, true, nil
}

func countMismatches(zx *image.Gray, c64 *image.RGBA) int {
	bounds := c64.Bounds()
	if zx.Bounds().Dx() != bounds.Dx() || zx.Bounds().Dy() != bounds.Dy() {
		panic(fmt.Sprintf("frame sizes differ: ZX %v, C64 %v", zx.Bounds(), bounds))
	}
	mismatches := 0
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			if !pixelMatches(zx, x, y, c64.RGBAAt(bounds.Min.X+x, bounds.Min.Y+y)) {
				mismatches++
			}
		}
	}
	return mismatches
}

func pixelMatches(zx *image.Gray, x int, y int, c64Pixel color.RGBA) bool {
	luma := (int(c64Pixel.R) + int(c64Pixel.G) + int(c64Pixel.B)) / 3
	switch {
	case luma < blackBelow:
		return !zxLit(zx, x, y)
	case luma >= lightFrom:
		return zxLit(zx, x, y)
	default:
		lit := litNeighbors(zx, x, y)
		return lit > 0 && lit < neighborPixels
	}
}

func litNeighbors(zx *image.Gray, x int, y int) int {
	lit := 0
	for dy := -neighborRadius; dy <= neighborRadius; dy++ {
		for dx := -neighborRadius; dx <= neighborRadius; dx++ {
			if zxLit(zx, x+dx, y+dy) {
				lit++
			}
		}
	}
	return lit
}

func zxLit(zx *image.Gray, x int, y int) bool {
	bounds := zx.Bounds()
	if x < 0 || y < 0 || x >= bounds.Dx() || y >= bounds.Dy() {
		return false
	}
	return zx.Pix[y*zx.Stride+x] >= litFrom
}
