package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/bits"
	"os"
)

const (
	dissolveFirstFrame  = 3951
	dissolveLastFrame   = 3977
	dissolveTurnFrame   = 3953
	dissolveTurnPerStep = 3.0
	dissolveComparePath = "golden/flower_dissolve_compare.png"
)

// The radii of the black petal that cuts the outline out of the white one, as a share of the white petal's. The ring is one
// subpixel thick at 0.82, the thinnest share that leaves every ring in one piece (from 0.83 on rings break); 0.72 makes it
// about one and a half
const dissolveInnerScale = 0.72

type dissolveMask [morphTileSize][morphTileSize]bool

// What a petal shows in a ZX frame: W the whole petal, C a checkerboard of the petal (the phase is the parity of
// x + y), T the petal in flat dark grey (the ZX shows a lattice), O the petal's
// outline, K nothing
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
	{{3957, dissolveStep{'C', 1}}, {3959, dissolveStep{'C', 0}}, {3961, dissolveStep{'K', 0}}, {3963, dissolveStep{'O', 1}}, {3965, dissolveStep{'K', 0}}},
	{{3967, dissolveStep{'C', 1}}, {3969, dissolveStep{'T', 0}}, {3971, dissolveStep{'K', 0}}, {3973, dissolveStep{'O', 1}}, {3975, dissolveStep{'K', 0}}},
	{{3967, dissolveStep{'C', 0}}, {3969, dissolveStep{'C', 1}}, {3971, dissolveStep{'O', 1}}, {3973, dissolveStep{'K', 0}}},
	{{3959, dissolveStep{'C', 0}}, {3961, dissolveStep{'C', 1}}, {3963, dissolveStep{'K', 0}}, {3965, dissolveStep{'O', 1}}, {3967, dissolveStep{'K', 0}}},
	{{3963, dissolveStep{'C', 1}}, {3965, dissolveStep{'C', 0}}, {3967, dissolveStep{'K', 0}}, {3969, dissolveStep{'O', 1}}, {3971, dissolveStep{'K', 0}}},
	{{3967, dissolveStep{'C', 1}}, {3969, dissolveStep{'C', 0}}, {3971, dissolveStep{'O', 0}}, {3973, dissolveStep{'O', 1}}, {3975, dissolveStep{'K', 0}}},
}

func dissolveStepAt(petal, frame int) dissolveStep {
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
// checkerboard is light grey and dark grey. The ZX's lattice (T) is a checkerboard of subpixels, 2 to a pixel, which is a
// flat dark grey (the phase is which half of the subpixels). The outline step O has a checkerboard of dark grey and
// black: 2 subpixels in the pixels it lights and none in the others (the phase is which half of the pixels).
//
//	W     C0    C1    T0    T1    O0    O1
//	####  #...  ..#.  #.#.  .#.#  ....  ....
//	####  ####  ####  .#.#  #.#.  ..##  ##..
//	####  ..#.  #...  #.#.  .#.#  ....  ....
//	####  ####  ####  .#.#  #.#.  ##..  ..##
var dissolveMasks = map[byte][]uint16{
	'W': {0xFFFF},
	'C': {0xF4F1, 0xF1F4},
	'T': {0xA5A5, 0x5A5A},
	'O': {0x30C0, 0xC030},
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
	return kind == 'O'
}

// The petal without the smaller petal of the same centre, whose radii are the petal's times dissolveInnerScale
func dissolveRing(frame, petal int) (ring [flowerSubRows]uint64) {
	params := dissolveParams(frame)
	outerRecord := newFlowerRecord(params)
	outer := outerRecord.onePetalRows(petal)
	params.radial *= dissolveInnerScale
	params.tangential *= dissolveInnerScale
	innerRecord := newFlowerRecord(params)
	inner := innerRecord.onePetalRows(petal)
	for row := range ring {
		ring[row] = outer[row] &^ inner[row]
	}
	return ring
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

func runFlowerDissolve() {
	shown := dissolveDistinctFrames()
	cell := morphTileSize * morphScale
	const gap = 4
	img := image.NewRGBA(image.Rect(0, 0, len(shown)*(cell+gap)-gap, 3*(cell+gap)-gap))
	fillMorphRect(img, img.Rect, color.RGBA{R: 40, G: 60, B: 160, A: 255})
	worstHard, totalLight := 0.0, 0
	for index, frame := range shown {
		screen, err := loadReferenceScreen(frame)
		if err != nil {
			fmt.Printf("No ZX reference: %v\n", err)
			return
		}
		coverage := dissolveCoverage(frame)
		hard := dissolveWeight(coverage, &screen)
		light := flowerBandDifference(coverage, &screen, referenceTileOrigin)
		worstHard = max(worstHard, hard)
		totalLight += light
		left := index * (cell + gap)
		drawMorphReference(img, left, 0, screen.tile(referenceTileOrigin, referenceTileOrigin))
		drawMorphCoverage(img, left, cell+gap, coverage)
		drawFlowerDiff(img, left, 2*(cell+gap), coverage, &screen, referenceTileOrigin)
		fmt.Printf("frame %d: weight %.1f, %d light pixels differ from the ZX\n", frame, hard, light)
	}
	fmt.Printf("Dissolve: %d distinct frames %v, worst tile weighs %.1f in the frame comparison (limit 8), %d light pixels differ in all\n", len(shown), shown, worstHard, totalLight)
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
	fmt.Printf("Exported %s (rows: ZX, C64 model with light grey, dark grey and black, difference as in the other comparisons)\n", dissolveComparePath)
}
