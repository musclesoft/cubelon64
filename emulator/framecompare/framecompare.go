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
	ExpectedLastMatchingFrame      = 3339
	ExpectedZXFrameCount           = 3052
	MaxBlockMismatches             = 8.0
	MaxInterpolationMismatches     = 80
	MaxCubeInterpolationMismatches = 8
	CubeScenesFirstFrame           = 2700
	CubeScenesEndFrame             = 3199
	TileSize                       = 24

	blackBelow     = 40
	lightFrom      = 150
	litFrom        = 128
	neighborRadius = 1
	neighborPixels = (2*neighborRadius + 1) * (2*neighborRadius + 1)
)

type interpolation struct {
	frame int
	c64   *image.RGBA
}

type Validator struct {
	LastMatchingFrame  int
	WorstInterpolation int

	dir        string
	failed     bool
	startFrame int
	zxFrames   []int
	reference  *image.RGBA
	pending    []interpolation
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
// which is not compared (only the frame just before a reference frame can be one: the one just after a
// reference frame is a hold unless the next reference is 2 frames on), or a frame of a hold, which must be identical to the C64 frame of the
// reference it holds (and that matched the ZX frame). A frame with a ZX file is one where the ZX
// picture changed, so the C64 frame must differ from the C64 frame compared before it.
func (validator *Validator) Check(frame int, c64 *image.RGBA) {
	if validator.failed || frame < validator.startFrame {
		return
	}
	zx, found, err := validator.loadZX(frame)
	if err != nil {
		panic(err)
	}
	if !found {
		if validator.reference != nil && validator.nearReference(frame) {
			validator.pending = append(validator.pending, interpolation{frame, c64})
			return
		}
		validator.checkHold(frame, c64)
		return
	}
	if worst := worstBlockMismatches(zx, c64); worst > MaxBlockMismatches {
		fmt.Printf("Frame %d has %.1f mismatches in one block of %dx%d pixels (at most %.0f)\n", frame, worst, TileSize, TileSize, MaxBlockMismatches)
		validator.failed = true
		return
	}
	if validator.reference != nil && bytes.Equal(validator.reference.Pix, c64.Pix) {
		fmt.Printf("Frame %d is the same as the C64 frame compared before it, but the ZX frame has changed\n", frame)
		validator.failed = true
		return
	}
	validator.checkInterpolations(c64)
	validator.LastMatchingFrame = frame
	validator.reference = c64
}

// The frames between two reference frames are the C64's own in-betweens: each pixel must be what the
// frame before or the frame after shows at its place or one pixel to the left or right (so a checkerboard
// that moves by a pixel passes), except for a few pixels per block where the shape in between is not
// the shape of either.
func (validator *Validator) checkInterpolations(next *image.RGBA) {
	for _, between := range validator.pending {
		worst := worstBlockUnexplained(between.c64, validator.reference, next)
		validator.WorstInterpolation = max(validator.WorstInterpolation, worst)
		if limit := interpolationLimit(between.frame); worst > limit {
			fmt.Printf("Frame %d has %d pixels in one block that are neither in the frame before nor in the frame after (at most %d)\n", between.frame, worst, limit)
			validator.failed = true
			return
		}
	}
	validator.pending = nil
}

func interpolationLimit(frame int) int {
	if frame >= CubeScenesFirstFrame && frame < CubeScenesEndFrame {
		return MaxCubeInterpolationMismatches
	}
	return MaxInterpolationMismatches
}

func worstBlockUnexplained(frame, before, after *image.RGBA) int {
	bounds := frame.Bounds()
	worst := 0
	for blockY := 0; blockY < bounds.Dy(); blockY += TileSize {
		for blockX := 0; blockX < bounds.Dx(); blockX += TileSize {
			unexplained := 0
			for y := blockY; y < min(blockY+TileSize, bounds.Dy()); y++ {
				for x := blockX; x < min(blockX+TileSize, bounds.Dx()); x++ {
					pixel := frame.RGBAAt(bounds.Min.X+x, bounds.Min.Y+y)
					if !shownNear(before, pixel, bounds.Min.X+x, bounds.Min.Y+y) && !shownNear(after, pixel, bounds.Min.X+x, bounds.Min.Y+y) {
						unexplained++
					}
				}
			}
			worst = max(worst, unexplained)
		}
	}
	return worst
}

// Whether the frame shows the pixel's colour at the position or one pixel to the left or right: a checkerboard
// that moves by a pixel passes
func shownNear(frame *image.RGBA, pixel color.RGBA, x, y int) bool {
	bounds := frame.Bounds()
	for dx := -1; dx <= 1; dx++ {
		if x+dx >= bounds.Min.X && x+dx < bounds.Max.X && frame.RGBAAt(x+dx, y) == pixel {
			return true
		}
	}
	return false
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
	return next < len(validator.zxFrames) && validator.zxFrames[next]-frame <= 1
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
	fmt.Printf("Worst in-between frame: %d pixels in one block neither in the frame before nor after (at most %d, %d from frame %d to %d)\n", validator.WorstInterpolation, MaxInterpolationMismatches, MaxCubeInterpolationMismatches, CubeScenesFirstFrame, CubeScenesEndFrame-1)
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

// The most mismatches in one of the blocks of TileSize by TileSize pixels, the cells of the demo's grid: a
// few wrong pixels in one place weigh more than the same number all over the frame
func worstBlockMismatches(zx *image.Gray, c64 *image.RGBA) float64 {
	bounds := c64.Bounds()
	if zx.Bounds().Dx() != bounds.Dx() || zx.Bounds().Dy() != bounds.Dy() {
		panic(fmt.Sprintf("frame sizes differ: ZX %v, C64 %v", zx.Bounds(), bounds))
	}
	worst := 0.0
	for blockY := 0; blockY < bounds.Dy(); blockY += TileSize {
		for blockX := 0; blockX < bounds.Dx(); blockX += TileSize {
			mismatches := 0.0
			for y := blockY; y < min(blockY+TileSize, bounds.Dy()); y++ {
				for x := blockX; x < min(blockX+TileSize, bounds.Dx()); x++ {
					mismatches += mismatchWeight(zx, x, y, c64.RGBAAt(bounds.Min.X+x, bounds.Min.Y+y))
				}
			}
			worst = max(worst, mismatches)
		}
	}
	return worst
}

// How much a pixel counts as a mismatch. A black C64 pixel must be black in the ZX frame and a light
// one white; if not, it counts 1. A pixel of the grey in between is an antialiased edge: it matches when
// its 3x3 ZX neighbourhood holds both black and white pixels, and counts half a mismatch when it does not.
func mismatchWeight(zx *image.Gray, x int, y int, c64Pixel color.RGBA) float64 {
	luma := (int(c64Pixel.R) + int(c64Pixel.G) + int(c64Pixel.B)) / 3
	switch {
	case luma < blackBelow:
		if zxLit(zx, x, y) {
			return 1
		}
		return 0
	case luma >= lightFrom:
		if zxLit(zx, x, y) {
			return 0
		}
		return 1
	}
	if lit := litNeighbors(zx, x, y); lit > 0 && lit < neighborPixels {
		return 0
	}
	return 0.5
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
