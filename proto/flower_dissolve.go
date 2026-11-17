package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/bits"
	"os"
	"strings"
)

const (
	dissolveFirstFrame     = 3951
	dissolveLastFrame      = 3977
	dissolveTurnFrame      = 3953
	dissolveTurnPerStep    = 3.0
	dissolveComparePath    = "golden/flower_dissolve_compare.png"
	dissolveFiftyHertzPath = "golden/flower_dissolve_50hz.png"
)

// The size of the black petal that cuts the outline out of the white one, as a share of the white petal's in 1/256 (0.66)
const dissolveInnerScale = 169

type dissolveMask [morphTileSize][morphTileSize]bool

// What a petal shows in a ZX frame: W the whole petal, C a checkerboard of the petal (the phase is the parity of
// x + y), T the petal in flat dark grey, a checkerboard of subpixels (the ZX shows its second checkerboard or a lattice), U a checkerboard
// of dark grey and black pixels, darker (only the lower left petal at 3969), O the petal's outline, K nothing
type dissolveStep struct {
	kind  byte
	phase int
}

type dissolveFrame struct {
	frame int
	step  dissolveStep
}

// The six petals of the even rows' tile in the order of their place at the frame 3951: left top, left bottom, top,
// bottom, right top, right bottom
var dissolvePetalCentres = [flowerPetals][2]float64{{5.05, 7.19}, {5.05, 14.81}, {11.5, 3.5}, {11.5, 18.5}, {17.95, 7.19}, {17.95, 14.81}}

// What each petal shows, read from the ZX frames (the odd rows' tile shows the mirror image two frames later)
var dissolveScripts = [flowerPetals][]dissolveFrame{
	{{3957, dissolveStep{'C', 1}}, {3959, dissolveStep{'T', 0}}, {3961, dissolveStep{'K', 0}}, {3963, dissolveStep{'O', 1}}, {3965, dissolveStep{'K', 0}}},
	{{3967, dissolveStep{'C', 1}}, {3969, dissolveStep{'U', 0}}, {3971, dissolveStep{'K', 0}}, {3973, dissolveStep{'O', 1}}, {3975, dissolveStep{'K', 0}}},
	{{3967, dissolveStep{'C', 0}}, {3969, dissolveStep{'T', 1}}, {3971, dissolveStep{'O', 1}}, {3973, dissolveStep{'K', 0}}},
	{{3959, dissolveStep{'C', 0}}, {3961, dissolveStep{'T', 1}}, {3963, dissolveStep{'K', 0}}, {3965, dissolveStep{'O', 1}}, {3967, dissolveStep{'K', 0}}},
	{{3963, dissolveStep{'C', 1}}, {3965, dissolveStep{'T', 0}}, {3967, dissolveStep{'K', 0}}, {3969, dissolveStep{'O', 1}}, {3971, dissolveStep{'K', 0}}},
	{{3967, dissolveStep{'C', 1}}, {3969, dissolveStep{'T', 0}}, {3971, dissolveStep{'O', 0}}, {3973, dissolveStep{'O', 1}}, {3975, dissolveStep{'K', 0}}},
}

func dissolveStepAt(petal, frame int) dissolveStep {
	if frame%2 == 0 {
		return dissolveBetween(dissolveStepAt(petal, frame-1), dissolveStepAt(petal, frame+1))
	}
	script := dissolveScripts[petal]
	if frame < script[0].frame {
		return dissolveStep{'W', 0}
	}
	step := dissolveStep{'K', 0}
	for _, entry := range script {
		if entry.frame <= frame {
			step = entry.step
		}
	}
	return step
}

// What a petal shows in the 50 Hz frame between two ZX frames, which the ZX holds for two frames: a step between the two,
// from the ladder of the square's patterns (the phase of a checkerboard is the parity of x + y of the pixels it lights)
//
//	from            to              between
//	W               C               N, three light pixels and one dark in each 2 by 2
//	C               T               M, one light pixel and three dark
//	C               U               T
//	T               K or O          D, a checkerboard of dark grey and black
//	U               K or O          S, one dark pixel in each 2 by 2
//	K               O               R, the ring as a checkerboard of dark grey and black
//	O               K               R
//	O               O               R, which darkens the ring on its way towards black
func dissolveBetween(from, to dissolveStep) dissolveStep {
	switch {
	case from.kind == 'W' && to.kind == 'C':
		return dissolveStep{'N', to.phase}
	case from.kind == 'C' && to.kind == 'T':
		return dissolveStep{'M', from.phase}
	case from.kind == 'C' && to.kind == 'U':
		return dissolveStep{'T', from.phase}
	case from.kind == 'T' && (to.kind == 'K' || to.kind == 'O'):
		return dissolveStep{'D', 0}
	case from.kind == 'U' && (to.kind == 'K' || to.kind == 'O'):
		return dissolveStep{'S', from.phase}
	case from.kind == 'K' && to.kind == 'O':
		return dissolveStep{'R', to.phase}
	case from.kind == 'O' && to.kind == 'K':
		return dissolveStep{'R', from.phase}
	case from.kind == 'O' && to.kind == 'O':
		return dissolveStep{'R', from.phase}
	}
	return to
}

// The petals of the flower at a frame, each as the pixels the ZX would light (3 of 4 subpixels); the flower keeps
// turning by 3 degrees a frame while it dissolves, and the petals are the final flower's
func dissolveParams(frame int) flowerParams {
	params := flowerParamsOfSweeps(&flowerSweeps, flowerLastState)
	params.phase = 0
	params.turn = 30 + dissolveTurnPerStep*float64(max(0, frame-dissolveTurnFrame))
	return params
}

func dissolveFootprints(frame int) (masks [flowerPetals]dissolveMask) {
	record := newFlowerRecord(dissolveParams(frame))
	for petal := range masks {
		rows := record.onePetalRows(petal)
		coverage := record.coverageOfRows(rows, 0)
		for y := range coverage {
			for x, level := range coverage[y] {
				masks[petal][y][x] = level == morphFullCoverage
			}
		}
	}
	return masks
}

func (record *flowerRecord) onePetalRows(petal int) [flowerSubRows]uint64 {
	var rows [flowerSubRows]uint64
	shape := &record.shapes[petal%flowerShapes]
	sign := 1 - 2*(petal/flowerShapes)
	offset := record.offsets[petal%flowerShapes]
	centreX := flowerCentreUnitsX + sign*offset[0]
	centreY := flowerCentreUnitsY + sign*offset[1]
	for row := range rows {
		dy := flowerSampleOffset + flowerSampleStep*row - centreY
		distance := max(dy, -dy)
		if distance >= shape.half {
			continue
		}
		rows[row] = flowerSpan(centreX+shape.shearAt(dy), shape.rowWidth(distance))
	}
	return rows
}

func (mask *dissolveMask) centre() (float64, float64) {
	count, sumX, sumY := 0, 0.0, 0.0
	for y := range mask {
		for x := range mask[y] {
			if mask[y][x] {
				count++
				sumX += float64(x)
				sumY += float64(y)
			}
		}
	}
	if count == 0 {
		return 0, 0
	}
	return sumX / float64(count), sumY / float64(count)
}

// The patterns of the steps are masks of 4 by 4 subpixels, 2 by 2 pixels, tiled over the tile (bit 4 * (y % 4) + x % 4 of
// the subpixel x, y), like the cube's patterns: a pixel counts the subpixels of its 2 by 2 that the petal covers and a
// mask lets through, 3 or 4 of them give light grey, 1 or 2 dark grey, none black. A pixel the ZX lights has 3 subpixels in
// the mask (the bottom row and the top left one) and one the ZX shows dark inside the petal has 2 (the bottom row), so the
// checkerboard is light grey and dark grey. The ZX's second checkerboard and its lattice (T) are a checkerboard of
// subpixels, 2 to a pixel, a flat dark grey (the phase is which half of the subpixels); U has 2 subpixels in two of the
// four pixels of a 2 by 2 and none in the others (the phase is which half of the pixels), S in one of the four. The
// outline step O is the ring's subpixels in the bottom row, 2 to a pixel, a solid dark grey; the step R towards black has
// a checkerboard of dark grey and black: 2 subpixels in the pixels it lights and none in the others (the phase is which
// half of the pixels).
//
//	W     C0    C1    T0    T1    U0    U1    S0    S1    O     R0    R1
//	####  #...  ..#.  #.#.  .#.#  ....  ....  ....  ....  ....  ....  ....
//	####  ####  ####  .#.#  #.#.  ##..  ..##  ..##  ##..  ####  ..##  ##..
//	####  ..#.  #...  #.#.  .#.#  ....  ....  ....  ....  ....  ....  ....
//	####  ####  ####  .#.#  #.#.  ..##  ##..  ....  ....  ####  ##..  ..##
var dissolveMasks = map[byte][]uint16{
	'W': {0xFFFF},
	'C': {0xF4F1, 0xF1F4},
	'T': {0xA5A5, 0x5A5A},
	'U': {0xC030, 0x30C0},
	'S': {0x00C0, 0x0030},
	'O': {0xF0F0},
	'N': {0xF4F5, 0xF1F5},
	'M': {0xF0F1, 0xF0F4},
	'D': {0xC030, 0x30C0},
	'R': {0x30C0, 0xC030},
}

// The rows of subpixels a petal shows in a step: the mask of the step tiled over the tile and cut to the petal's covered
// subpixels. The outline step shows the petal's ring instead, the petal drawn in white and covered by a smaller petal in
// black, through its mask
func dissolveShown(rows, ring [flowerSubRows]uint64, step dissolveStep) (shown [flowerSubRows]uint64) {
	masks, ok := dissolveMasks[step.kind]
	if !ok {
		return shown
	}
	mask := masks[step.phase%len(masks)]
	base := rows
	if dissolveOutlineOnly(step.kind) {
		base = ring
	}
	for row := range rows {
		cells := uint64(mask>>(4*(row%4))) & 0xF
		shown[row] = base[row] & (cells * 0x111111111111)
	}
	return shown
}

func dissolveOutlineOnly(kind byte) bool {
	return kind == 'O' || kind == 'R'
}

// The petal without the smaller petal of the same centre, whose half height and width are the petal's times
// dissolveInnerScale/256 (rounded once, in integers: the C64 derives the smaller petal's shape the same way)
func dissolveRing(frame, petal int) (ring [flowerSubRows]uint64) {
	outerRecord := dissolveRecord(frame)
	outer := outerRecord.onePetalRows(petal)
	innerRecord := dissolveInnerRecord(&outerRecord)
	inner := innerRecord.onePetalRows(petal)
	for row := range ring {
		ring[row] = outer[row] &^ inner[row]
	}
	return ring
}

func dissolveRecord(frame int) flowerRecord {
	return newFlowerRecord(dissolveParams(frame))
}

func dissolveInnerRecord(outer *flowerRecord) flowerRecord {
	inner := *outer
	for index, shape := range outer.shapes {
		inner.shapes[index].half = (shape.half*dissolveInnerScale + 128) >> 8
		inner.shapes[index].width = (shape.width*dissolveInnerScale + 128) >> 8
	}
	return inner
}

// The tile of the even rows at a frame as the C64 draws it: every petal's subpixels its step shows, counted 2 by 2 to a
// pixel, and the pixel's grey from the count
func dissolveCoverage(frame int) (coverage morphCoverage) {
	record := newFlowerRecord(dissolveParams(frame))
	var counts [morphTileSize][morphTileSize]int
	for petal := 0; petal < flowerPetals; petal++ {
		shown := dissolveShown(record.onePetalRows(petal), dissolveRing(frame, petal), dissolveStepAt(dissolvePetalOfCentre(petal), frame))
		for y := range counts {
			for x := range counts[y] {
				pixel := uint64(1<<morphSubsamples-1) << (morphSubsamples * x)
				for subY := 0; subY < morphSubsamples; subY++ {
					counts[y][x] += bits.OnesCount64(shown[morphSubsamples*y+subY] & pixel)
				}
			}
		}
	}
	for y := range coverage {
		for x := range coverage[y] {
			coverage[y][x] = morphCoverageLevel(counts[y][x])
		}
	}
	return coverage
}

// The script of the petal whose place at the frame 3951 is nearest to the proto petal's place
func dissolvePetalOfCentre(petal int) int {
	footprints := dissolveFootprints(dissolveFirstFrame)
	x, y := footprints[petal].centre()
	best, least := 0, math.Inf(1)
	for index, centre := range dissolvePetalCentres {
		if distance := math.Hypot(x-centre[0], y-centre[1]); distance < least {
			best, least = index, distance
		}
	}
	return best
}

func dissolveReferenceTile(screen *referenceScreen) (tile dissolveMask) {
	for y := range tile {
		for x := range tile[y] {
			tile[y][x] = screen.lit(referenceTileOrigin+x, referenceTileOrigin+y)
		}
	}
	return tile
}

// The ZX frames of the dissolve without the ones that show what the frame before shows
func dissolveDistinctFrames() (frames []int) {
	var previous dissolveMask
	for frame := dissolveFirstFrame; frame <= dissolveLastFrame; frame += 2 {
		screen, err := loadReferenceScreen(frame)
		if err != nil {
			fmt.Printf("No ZX reference: %v\n", err)
			return nil
		}
		tile := dissolveReferenceTile(&screen)
		if len(frames) == 0 || tile != previous {
			frames = append(frames, frame)
		}
		previous = tile
	}
	return frames
}

// What the frame comparison counts in the tile
func dissolveWeight(coverage morphCoverage, screen *referenceScreen) (weight float64) {
	for y := range coverage {
		for x, level := range coverage[y] {
			weight += flowerPixelWeight(screen, referenceTileOrigin+x, referenceTileOrigin+y, level)
		}
	}
	return weight
}

// The frames the comparison shows: the first frame, which is no part of the animation but the reference, and then the animation, from
// the frame before the first ZX frame that differs to the frame after the last one that is not black (the ZX holds the first
// and the last picture for several frames, which the C64 does not repeat)
func dissolveShownFrames() []int {
	distinct := dissolveDistinctFrames()
	last := distinct[0]
	for _, frame := range distinct {
		screen, err := loadReferenceScreen(frame)
		if err == nil && dissolveReferenceTile(&screen) != (dissolveMask{}) {
			last = frame
		}
	}
	shown := []int{distinct[0]}
	for frame := distinct[1] - 1; frame <= last+1; frame++ {
		shown = append(shown, frame)
	}
	return shown
}

func runFlowerDissolve() {
	writeFlowerDissolveData()
	cell := morphTileSize * morphScale
	const gap = 4
	shown := dissolveShownFrames()
	img := image.NewRGBA(image.Rect(0, 0, len(shown)*(cell+gap)-gap, 3*(cell+gap)-gap))
	fillMorphRect(img, img.Rect, color.RGBA{R: 40, G: 60, B: 160, A: 255})
	worstHard, totalLight, worstBetween := 0.0, 0, 0
	for index, frame := range shown {
		left := index * (cell + gap)
		coverage := dissolveCoverage(frame)
		drawMorphCoverage(img, left, cell+gap, coverage)
		if frame%2 == 0 {
			unexplained := dissolveUnexplained(frame)
			worstBetween = max(worstBetween, unexplained)
			fmt.Printf("frame %d (between %d and %d): %d pixels that neither neighbour shows\n", frame, frame-1, frame+1, unexplained)
			continue
		}
		screen, err := loadReferenceScreen(frame)
		if err != nil {
			fmt.Printf("No ZX reference: %v\n", err)
			return
		}
		hard := dissolveWeight(coverage, &screen)
		light := flowerBandDifference(coverage, &screen, referenceTileOrigin)
		worstHard = max(worstHard, hard)
		totalLight += light
		drawMorphReference(img, left, 0, screen.tile(referenceTileOrigin, referenceTileOrigin))
		drawFlowerDiff(img, left, 2*(cell+gap), coverage, &screen, referenceTileOrigin)
		fmt.Printf("frame %d: weight %.1f, %d light pixels differ from the ZX\n", frame, hard, light)
	}
	fmt.Printf("Dissolve: worst ZX frame weighs %.1f in the frame comparison (limit 8), %d light pixels differ in all; the frames between the ZX frames have at most %d pixels that neither neighbour shows (limit 80)\n", worstHard, totalLight, worstBetween)
	file, err := os.Create(dissolveComparePath)
	if err != nil {
		fmt.Printf("Warning: failed to create %s: %v\n", dissolveComparePath, err)
		return
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		fmt.Printf("Warning: failed to encode %s: %v\n", dissolveComparePath, err)
		return
	}
	fmt.Printf("Exported %s (rows: ZX at 25 Hz, C64 model at 50 Hz with light grey, dark grey and black, difference as in the other comparisons; the first column is the reference, no part of the animation; one column per 50 Hz frame)\n", dissolveComparePath)
}

// The number of pixels of the frame between two ZX frames that neither neighbouring C64 frame shows at its place or one
// pixel to the left or right (the frame comparison allows 80 in a block)
func dissolveUnexplained(frame int) int {
	return unexplainedPixels(dissolveCoverage(frame), dissolveCoverage(frame-1), dissolveCoverage(frame+1))
}

// Whether the frame shows the pixel's level at its place or one pixel to the left or right
func dissolveShownNear(frame, tile morphCoverage, x, y int) bool {
	for dx := -1; dx <= 1; dx++ {
		if x+dx >= 0 && x+dx < morphTileSize && frame[y][x+dx] == tile[y][x] {
			return true
		}
	}
	return false
}

const (
	dissolveDataPath   = "../c64port/src/flower_dissolve_data.inc"
	dissolveImagesPath = "golden/flower_dissolve_images.bin"
	dissolveImageFirst = dissolveFirstFrame + 1
	dissolveImageCount = 23
)

// The masks of the square's ladder and the two the dissolve adds after the solid one
func flowerLadderMask(shade, phase int) uint16 {
	if shade == dissolveLatticeShade {
		return dissolveMasks['T'][phase%2]
	}
	return flowerSquareMask(shade, phase)
}

// What a step is to the C64: the shade of the square's ladder whose mask it uses (flower_square_masks; the shade 8 is the
// solid mask and 9 the lattice T, which only the dissolve uses), the phase in that mask and whether the ring is shown: the
// byte is 4 * shade + 2 * ring + phase, and 0 shows nothing
const dissolveLatticeShade = flowerSolidShade + 1

var dissolveShades = map[byte]int{'S': 1, 'U': 2, 'D': 2, 'R': 2, 'O': 4, 'M': 5, 'C': 6, 'N': 7, 'W': flowerSolidShade, 'T': dissolveLatticeShade}

// Whether the phases of the step's masks are in the opposite order in the ladder's
var dissolveSwappedPhases = map[byte]bool{'U': true, 'D': true, 'M': true, 'C': true, 'N': true}

func dissolveStepByte(step dissolveStep) int {
	if step.kind == 'K' {
		return 0
	}
	shade := dissolveShades[step.kind]
	phase := step.phase % len(dissolveMasks[step.kind])
	ladderPhase := phase
	if dissolveSwappedPhases[step.kind] {
		ladderPhase = 1 - phase
	}
	want := dissolveMasks[step.kind][phase]
	if got := flowerLadderMask(shade, ladderPhase); got != want {
		panic(fmt.Sprintf("the mask of step %c%d is %04X in the ladder, not %04X", step.kind, step.phase, got, want))
	}
	ring := 0
	if dissolveOutlineOnly(step.kind) {
		ring = 1
	}
	return 4*shade + 2*ring + ladderPhase
}

// The images the C64 renders: one for each frame from 3952 to 3974, every frame has its own turn
func dissolveImageFrame(image int) int {
	return dissolveImageFirst + image
}

func dissolveTurnIndex(frame int) int {
	return max(0, frame-dissolveTurnFrame)
}

func writeFlowerDissolveData() {
	var text strings.Builder
	text.WriteString("; Generated by proto (go run -C proto . flower dissolve), do not edit\n")
	text.WriteString("; The records of the dissolve's turns 0-21 (the frames 3953-3974; the frame 3952 has the turn 0 too), then for each of the 23\n")
	text.WriteString("; images (the frames 3952-3974) the step of each of the 6 petals (see dissolveStepByte)\n\n")
	text.WriteString(".segment \"DISSOLVEDATA\"\n\n.export flower_dissolve_records\n.export flower_dissolve_steps\n\nflower_dissolve_records:\n")
	turns := dissolveTurnIndex(dissolveImageFrame(dissolveImageCount-1)) + 1
	for turn := 0; turn < turns; turn++ {
		record := dissolveRecord(dissolveTurnFrame + turn)
		if !flowerRecordFits(&record) {
			fmt.Printf("Warning: the record of the dissolve's turn %d does not fit its bytes\n", turn)
		}
		fmt.Fprintf(&text, "    ; turn %d\n", turn)
		text.WriteString(flowerByteLine(flowerRecordValues(&record)))
	}
	text.WriteString("\nflower_dissolve_steps:\n")
	var images []morphCoverage
	for image := 0; image < dissolveImageCount; image++ {
		frame := dissolveImageFrame(image)
		var indices []int
		for petal := 0; petal < flowerPetals; petal++ {
			indices = append(indices, dissolveStepByte(dissolveStepAt(dissolvePetalOfCentre(petal), frame)))
		}
		fmt.Fprintf(&text, "    ; frame %d\n", frame)
		text.WriteString(flowerByteLine(indices))
		images = append(images, dissolveCoverage(frame))
	}
	if err := os.WriteFile(dissolveDataPath, []byte(text.String()), 0644); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", dissolveDataPath, err)
		return
	}
	if err := writeTileImages(dissolveImagesPath, images); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", dissolveImagesPath, err)
		return
	}
	fmt.Printf("Exported %s and %s (%d images)\n", dissolveDataPath, dissolveImagesPath, len(images))
}
