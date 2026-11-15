package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

const (
	flowerMarchFirstFrame   = 3399
	flowerMarchKickPeriod   = 40
	flowerMarchPairKicks    = 2
	flowerMarchPairPeriod   = flowerMarchKickPeriod * flowerMarchPairKicks * 2
	flowerMarchPairStates   = 60
	flowerMarchKicksInCycle = 8
	flowerMarchComparePath  = "golden/flower_march_compare.png"
	flowerMarchImagesPath   = "golden/flower_march_images.bin"
)

// The flower marches in pairs of kicks of 20 states, 21 frames apart in the ZX demo: the rows of even
// cells move right and turn clockwise, the rows of odd cells move left and turn the other way, and after
// two kicks the pattern is its own half a cell on. The next pair does the same with the two kinds of
// rows swapped. A sweep describes each pair; the state is the number of frames since the pair began,
// and the values hold between the kicks. The shift is in whole pixels (the ZX rolls its rows by whole
// pixels), so its sweep starts at -0.5 to make the usual rounding of half up come out as a round down.
var flowerMarchSweeps = struct {
	shift, turn flowerCurve
}{
	shift: flowerCurve{{from: 0, to: 20, start: -128, step: 154}, {from: 40, to: 60, start: 11.5 * flowerSweepGrid, step: 154}},
	turn:  flowerCurve{{from: 0, to: 20, start: 30 * flowerSweepGrid, step: -3 * flowerSweepGrid}, {from: 40, to: 60, start: -30 * flowerSweepGrid, step: -3 * flowerSweepGrid}},
}

// The pair, the state in it and whether the kinds of rows are swapped, for a ZX frame; ok is false
// outside the marching kicks (the others turn the flower back into the square and grow it again)
func flowerMarchOfFrame(frame int) (state int, swapped, ok bool) {
	since := frame - flowerMarchFirstFrame
	if since < 0 {
		return 0, false, false
	}
	kick := since / flowerMarchKickPeriod
	switch kick % flowerMarchKicksInCycle {
	case 0, 1, 4, 5:
	default:
		return 0, false, false
	}
	pair := since / flowerMarchPairPeriod
	return since % flowerMarchPairPeriod, pair%2 == 1, true
}

func rollCoverage(coverage morphCoverage, shift int) morphCoverage {
	var rolled morphCoverage
	for y := range coverage {
		for x := range coverage[y] {
			rolled[y][(x+shift+morphTileSize*morphTileSize)%morphTileSize] = coverage[y][x]
		}
	}
	return rolled
}

// The cells of the even and the odd rows in a state of a pair. The petals are the ones the morph ends with.
func flowerMarchImages(sweeps *flowerSweepSet, state int, swapped bool) (even, odd morphCoverage) {
	params := flowerParamsOfSweeps(sweeps, flowerLastState)
	params.turn = flowerMarchSweeps.turn.at(state)
	params.phase = 0
	shift := flowerRound(flowerMarchSweeps.shift.at(state), 1)
	even = rollCoverage(flowerCoverage(params), shift)
	odd = rollCoverage(flowerOddCoverage(params), -shift)
	if swapped {
		even, odd = odd, even
	}
	return even, odd
}

func flowerMarchDifferences(sweeps *flowerSweepSet, frame int, screen *referenceScreen) (hardEven, hardOdd, lightEven, lightOdd int, ok bool) {
	state, swapped, ok := flowerMarchOfFrame(frame)
	if !ok {
		return 0, 0, 0, 0, false
	}
	even, odd := flowerMarchImages(sweeps, state, swapped)
	hardEven = countTileMismatches(even, screen, referenceTileOrigin, referenceTileOrigin)
	hardOdd = countTileMismatches(odd, screen, referenceTileOrigin, referenceTileOrigin+morphTileSize)
	lightEven = flowerBandDifference(even, screen, referenceTileOrigin)
	lightOdd = flowerBandDifference(odd, screen, referenceTileOrigin+morphTileSize)
	return hardEven, hardOdd, lightEven, lightOdd, true
}

func flowerBandDifference(coverage morphCoverage, screen *referenceScreen, originY int) int {
	different := 0
	for y := range coverage {
		for x, level := range coverage[y] {
			if (level == morphFullCoverage) != screen.lit(referenceTileOrigin+x, originY+y) {
				different++
			}
		}
	}
	return different
}

// The frames of the first pair's first kick, which the fit uses
func flowerMarchFitFrames() []int {
	var frames []int
	for frame := flowerMarchFirstFrame + 2; frame <= flowerMarchFirstFrame+20; frame += 2 {
		frames = append(frames, frame)
	}
	return frames
}

func reportFlowerMarch(sweeps *flowerSweepSet) {
	worstHard, worstLight, frames := 0, 0, 0
	for frame := flowerMarchFirstFrame; frame < 3951; frame++ {
		screen, err := loadReferenceScreen(frame)
		if err != nil {
			continue
		}
		hardEven, hardOdd, lightEven, lightOdd, ok := flowerMarchDifferences(sweeps, frame, &screen)
		if !ok {
			continue
		}
		frames++
		worstHard = max(worstHard, hardEven, hardOdd)
		worstLight = max(worstLight, lightEven, lightOdd)
		if max(hardEven, hardOdd) > 4 || max(lightEven, lightOdd) > 10 {
			fmt.Printf("frame %d: %d and %d mismatches, light pixels differ in %d and %d\n", frame, hardEven, hardOdd, lightEven, lightOdd)
		}
	}
	fmt.Printf("Marching flowers: %d ZX frames, worst tile has %d mismatches (limit 8) and %d light pixels that differ from the ZX ones\n", frames, worstHard, worstLight)
}

func writeFlowerMarchImages(sweeps *flowerSweepSet) {
	var even, odd []morphCoverage
	for state := 0; state <= 20; state++ {
		e, o := flowerMarchImages(sweeps, state, false)
		even, odd = append(even, e), append(odd, o)
	}
	if err := writeTileImages(flowerMarchImagesPath, even, odd); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", flowerMarchImagesPath, err)
		return
	}
	fmt.Printf("Exported %s (the even and then the odd rows' states 0-20 of the first kick, %d bytes each)\n", flowerMarchImagesPath, imageBytes)
}

// One column per state of the first pair's two kicks (the states in between hold); rows as in the morph's picture
func writeFlowerMarchComparison(sweeps *flowerSweepSet) {
	const gap = 4
	const rows = 6
	var states []int
	for state := 0; state <= 20; state++ {
		states = append(states, state)
	}
	for state := 40; state <= 60; state++ {
		states = append(states, state)
	}
	cell := morphTileSize * morphScale
	img := image.NewRGBA(image.Rect(0, 0, len(states)*(cell+gap)-gap, rows*(cell+gap)-gap))
	fillMorphRect(img, img.Rect, color.RGBA{R: 40, G: 60, B: 160, A: 255})
	for column, state := range states {
		left := column * (cell + gap)
		even, odd := flowerMarchImages(sweeps, state, false)
		drawMorphCoverage(img, left, cell+gap, even)
		drawMorphCoverage(img, left, 4*(cell+gap), odd)
		if state%2 != 0 {
			continue
		}
		screen, err := loadReferenceScreen(flowerMarchFirstFrame + state)
		if err != nil {
			continue
		}
		referenceEven := screen.tile(referenceTileOrigin, referenceTileOrigin)
		referenceOdd := screen.tile(referenceTileOrigin, referenceTileOrigin+morphTileSize)
		drawMorphReference(img, left, 0, referenceEven)
		drawFlowerDiff(img, left, 2*(cell+gap), even, &screen, referenceTileOrigin)
		drawFlowerDiff(img, left, 3*(cell+gap), odd, &screen, referenceTileOrigin+morphTileSize)
		drawMorphReference(img, left, 5*(cell+gap), referenceOdd)
	}
	file, err := os.Create(flowerMarchComparePath)
	if err != nil {
		fmt.Printf("Warning: failed to create %s: %v\n", flowerMarchComparePath, err)
		return
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		fmt.Printf("Warning: failed to encode %s: %v\n", flowerMarchComparePath, err)
		return
	}
	fmt.Printf("Exported %s (rows: ZX even row, C64 even row, even difference, odd difference, C64 odd row, ZX odd row; one column per state of the first two kicks)\n", flowerMarchComparePath)
}
