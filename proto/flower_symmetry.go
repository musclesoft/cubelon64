package main

import "fmt"

const (
	flowerSourceQuarter = 0x40
	flowerSourceSwapped = 0x80
	flowerMirrorColumn  = 11
)

// Where the pair of a step (or of the state between two steps) of the marching flowers comes from, for the C64,
// which renders only the states 1-10 of the first kick (each at its own shift) and gets the rest by mirroring:
//   - the state s of the second kick is the state 20 - s' of the first kick with the rows swapped (the flower
//     turned by t and shifted by x, with the rows swapped, is the flower turned by -t and shifted by -x, which
//     is the flower at 12 - x in the first kick, turned by -t)
//   - the state 20 - m is the state m mirrored about x = 6 (a mirror turns t into -t, and x = 6 puts the shift
//     x at 12 - x)
//   - the odd rows' tile is the mirror of the even rows' tile
//
// The byte is the index of the rendered tile (the even states 0-10 or the odd states 1-9), 0x40 for the
// mirror about x = 6 and 0x80 for the swapped rows.
func flowerMarchSource(step int, between bool) int {
	state := flowerMarchStepState(step)
	if between {
		state = flowerBetweenKickState(step)
	}
	source := state
	swapped := 0
	if state >= flowerMarchKickPeriod {
		source = flowerMarchKickStates - (state - flowerMarchKickPeriod)
		swapped = flowerSourceSwapped
	}
	quarter := 0
	if source > flowerMarchKickStates/2 {
		source = flowerMarchKickStates - source
		quarter = flowerSourceQuarter
	}
	return source/2 | quarter | swapped
}

// The state the byte of flowerMarchSource names
func flowerMarchSourceState(source int, between bool) int {
	state := 2 * (source & 7)
	if between {
		state++
	}
	return state
}

func flowerMarchPairFromSource(sweeps *flowerSweepSet, source int, between bool) (even, odd morphCoverage) {
	base, _ := flowerMarchImages(sweeps, flowerMarchSourceState(source, between), false)
	if source&flowerSourceQuarter != 0 {
		base = mirrorAboutQuarter(base)
	}
	even, odd = base, mirrorCoverage(base)
	if source&flowerSourceSwapped != 0 {
		even, odd = odd, even
	}
	return even, odd
}

func verifyFlowerMarchSymmetries() {
	turnedDifferent, mirrorDifferent, sourceDifferent := 0, 0, 0
	for state := 0; state < flowerMarchKickPeriod+flowerMarchKickStates+1; state++ {
		params := flowerMarchParamsOfState(state)
		record := newFlowerRecord(params)
		direct := record.coverage(params.shift, 0)
		turned := record.coverageOfRows(record.halfTurnRows(params.shift), 0)
		turnedDifferent += countCoverageDifferences(direct, turned)
		even, odd := flowerMarchImages(&flowerSweeps, state, false)
		mirrorDifferent += countCoverageDifferences(odd, mirrorCoverage(even))
	}
	for step := 0; step < flowerMarchPairs; step++ {
		for _, between := range []bool{false, true} {
			wantEven, wantOdd := flowerMarchPair(step)
			if between {
				wantEven, wantOdd = flowerBetweenMarchPair(step)
			}
			gotEven, gotOdd := flowerMarchPairFromSource(&flowerSweeps, flowerMarchSource(step, between), between)
			sourceDifferent += countCoverageDifferences(wantEven, gotEven) + countCoverageDifferences(wantOdd, gotOdd)
		}
	}
	fmt.Printf("Marching flowers: petals 3-5 as the half turn of 0-2 differ in %d pixels, the odd rows' tiles from the mirrored even ones in %d, the pairs made from the rendered states 1-10 from the drawn ones in %d\n", turnedDifferent, mirrorDifferent, sourceDifferent)
}

func flowerMarchParamsOfState(state int) flowerParams {
	params := flowerParamsOfSweeps(&flowerSweeps, flowerLastState)
	params.turn = flowerMarchSweeps.turn.at(state)
	params.phase = 0
	params.shift = flowerMarchShiftUnits(state)
	return params
}

// The tile mirrored about x = 6: the pixel x goes to 11 - x, wrapped at the tile
func mirrorAboutQuarter(coverage morphCoverage) morphCoverage {
	var mirrored morphCoverage
	for y := range coverage {
		for x := range coverage[y] {
			mirrored[y][((flowerMirrorColumn-x)%morphTileSize+morphTileSize)%morphTileSize] = coverage[y][x]
		}
	}
	return mirrored
}

// The least mismatches of the tile with the ZX frame, which may be moved a pixel left or right
func leastDriftedMismatches(coverage morphCoverage, screen *referenceScreen, originY int) int {
	least := countTileMismatches(coverage, screen, referenceTileOrigin, originY)
	for drift := -1; drift <= 1; drift += 2 {
		least = min(least, countTileMismatches(coverage, screen, referenceTileOrigin+drift, originY))
	}
	return least
}
