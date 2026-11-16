package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
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
	flowerMarchShiftQuantum = 1
)

// The flower marches in pairs of kicks of 20 states, 21 frames apart in the ZX demo: the rows of even
// cells move right and turn clockwise, the rows of odd cells move left and turn the other way, and after
// two kicks the pattern is its own half a cell on. The next pair does the same with the two kinds of
// rows swapped. A sweep describes each pair; the state is the number of frames since the pair began,
// and the values hold between the kicks. The shift is a straight line, 0.6 pixel a state, as constant as
// the turn (the ZX moves by whole pixels with an uneven step, a drawing flaw the C64 does not copy), and is
// in steps of 1/16 pixel, the renderer's resolution; the mirrors it is made from are exact (verifyFlowerMarchSymmetries).
var flowerMarchSweeps = struct {
	turn flowerCurve
}{
	turn: flowerCurve{{from: 0, to: 20, start: 30 * flowerSweepGrid, step: -3 * flowerSweepGrid}, {from: 40, to: 60, start: -30 * flowerSweepGrid, step: -3 * flowerSweepGrid}},
}

// The shift of the flower in a state of a pair, in 1/16 pixel: 9.6 a state along the kicks (0-20, then 40-60, and
// held between), rounded once, which makes the shifts of the states m and 20 - m add up to exactly 12 pixels
func flowerMarchShiftUnits(state int) int {
	along := min(max(state, 0), 20)
	if state >= 40 {
		along = 20 + min(state-40, 20)
	}
	exact := float64(along) * 9.6
	return int(math.Floor(exact/float64(flowerMarchShiftQuantum)+0.5)) * flowerMarchShiftQuantum
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

// The cells of the even and the odd rows in a state of a pair, drawn with their shift: the renderer puts
// the petals there and wraps them at the tile. The petals are the ones the morph ends with.
func flowerMarchImages(sweeps *flowerSweepSet, state int, swapped bool) (even, odd morphCoverage) {
	params := flowerParamsOfSweeps(sweeps, flowerLastState)
	params.turn = flowerMarchSweeps.turn.at(state)
	params.phase = 0
	params.shift = flowerMarchShiftUnits(state)
	even = flowerCoverage(params)
	params.shift = -params.shift
	odd = flowerOddCoverage(params)
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
	hardEven = leastDriftedMismatches(even, screen, referenceTileOrigin)
	hardOdd = leastDriftedMismatches(odd, screen, referenceTileOrigin+morphTileSize)
	lightEven = flowerBandDifference(even, screen, referenceTileOrigin)
	lightOdd = flowerBandDifference(odd, screen, referenceTileOrigin+morphTileSize)
	return hardEven, hardOdd, lightEven, lightOdd, true
}

// The ZX saves a frame only when the screen changes, so the first state shows the last frame of the morph
func flowerMarchReferenceFrame(state int) int {
	if state == 0 {
		return flowerFrameOfState(flowerLastState)
	}
	return flowerMarchFirstFrame + state
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
		if max(hardEven, hardOdd) > 14 || max(lightEven, lightOdd) > 10 {
			fmt.Printf("frame %d: %d and %d mismatches, light pixels differ in %d and %d\n", frame, hardEven, hardOdd, lightEven, lightOdd)
		}
	}
	fmt.Printf("Marching flowers: %d ZX frames, worst tile has %d mismatches when the ZX frame may drift a pixel in x (the validator's weighing differs, see MaxFlowerMarchBlockMismatches) and %d light pixels that differ from the ZX ones\n", frames, worstHard, worstLight)
}

// One column per state of the first kick; rows: ZX even row, C64 even row, difference. The second kick is
// the first rolled by 12 pixels (checked on the ZX frames), the odd rows are the flip of the even
// ones, and the next pair swaps the kinds of rows.
func writeFlowerMarchComparison(sweeps *flowerSweepSet) {
	const gap = 4
	const rows = 3
	const states = 21
	cell := morphTileSize * morphScale
	img := image.NewRGBA(image.Rect(0, 0, states*(cell+gap)-gap, rows*(cell+gap)-gap))
	fillMorphRect(img, img.Rect, color.RGBA{R: 40, G: 60, B: 160, A: 255})
	for state := 0; state < states; state++ {
		left := state * (cell + gap)
		even, _ := flowerMarchImages(sweeps, state, false)
		drawMorphCoverage(img, left, cell+gap, even)
		if state%2 != 0 {
			continue
		}
		screen, err := loadReferenceScreen(flowerMarchReferenceFrame(state))
		if err != nil {
			continue
		}
		drawMorphReference(img, left, 0, screen.tile(referenceTileOrigin, referenceTileOrigin))
		drawFlowerDiff(img, left, 2*(cell+gap), even, &screen, referenceTileOrigin)
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
	fmt.Printf("Exported %s (rows: ZX even row, C64 even row, difference; one column per state 0-20 of the first kick)\n", flowerMarchComparePath)
}
