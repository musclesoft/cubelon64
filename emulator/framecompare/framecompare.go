package framecompare

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	ExpectedLastMatchingFrame = 2099
	ExpectedZXFrameCount      = 3052
	MaxMismatchedPixels       = 512

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
	zxFrames   []int
	reference  *image.RGBA
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

// Check compares a C64 frame with the ZX reference. The ZX run saves a frame only when its screen
// changes, so a frame without a ZX file is either an interpolation next to a reference frame,
// which is not compared, or a frame of a hold, which must be identical to the C64 frame of the
// reference it holds (and that matched the ZX frame).
func (validator *Validator) Check(frame int, c64 *image.RGBA) {
	if validator.failed || frame < validator.startFrame {
		return
	}
	zx, found, err := validator.loadZX(frame)
	if err != nil {
		panic(err)
	}
	if !found {
		validator.checkHold(frame, c64)
		return
	}
	if countMismatches(zx, c64) > MaxMismatchedPixels {
		validator.failed = true
		return
	}
	validator.LastMatchingFrame = frame
	validator.reference = c64
}

func (validator *Validator) checkHold(frame int, c64 *image.RGBA) {
	if validator.reference == nil || validator.nearReference(frame) {
		return
	}
	if !bytes.Equal(validator.reference.Pix, c64.Pix) {
		fmt.Printf("Frame %d differs from the C64 frame of the ZX reference it holds: %d pixels\n", frame, countDifferences(validator.reference, c64))
		validator.failed = true
	}
}

func (validator *Validator) nearReference(frame int) bool {
	if validator.zxFrames == nil {
		validator.zxFrames = validator.listZXFrames()
	}
	next := sort.SearchInts(validator.zxFrames, frame)
	if next < len(validator.zxFrames) && validator.zxFrames[next]-frame <= 1 {
		return true
	}
	return next > 0 && frame-validator.zxFrames[next-1] <= 1
}

func (validator *Validator) listZXFrames() []int {
	matches, err := filepath.Glob(filepath.Join(validator.dir, "frame_*_zx.png"))
	if err != nil {
		panic(err)
	}
	frames := []int{}
	for _, match := range matches {
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(match), "frame_"), "_zx.png")
		number, err := strconv.Atoi(name)
		if err != nil {
			panic(err)
		}
		frames = append(frames, number)
	}
	sort.Ints(frames)
	return frames
}

func countDifferences(a *image.RGBA, b *image.RGBA) int {
	differences := 0
	for i := 0; i < len(a.Pix) && i < len(b.Pix); i += 4 {
		if !bytes.Equal(a.Pix[i:i+4], b.Pix[i:i+4]) {
			differences++
		}
	}
	return differences
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
