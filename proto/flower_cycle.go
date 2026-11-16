package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	flowerCycleFrames     = 40
	flowerCycleKinds      = 4
	flowerLastCycle       = 14
	flowerMarchTurns      = 10
	flowerMarchPairs      = 2 * flowerMarchTurns
	flowerMarchTurnStep   = 6
	flowerMarchTurnStart  = 30
	flowerBetweenTurnStep = flowerMarchTurnStep / 2
	flowerBetweenStates   = flowerLastState/2 - 1
	flowerMarchKickStates = 20
	flowerMorphPairs      = flowerLastState / 2
	flowerMirroredPairs   = flowerMorphPairs
	flowerSceneImagesPath = "golden/flower_march_images.bin"
	flowerSceneDataPath   = "../c64port/src/flower_march_data.inc"
	flowerTilesPath       = "golden/flower_march_tiles.bin"

	flowerBetweenImagesPath    = "golden/flower_odd_images.bin"
	flowerBetweenTurnTilesPath = "golden/flower_odd_turn_tiles.bin"
)

// The scene is a loop of cycles of 40 frames, from the morph's frame 3359: the square turns into the flower (kind 0),
// the flower marches twice (kinds 1 and 2) and turns back into the square (kind 3). The ZX shows every second state
// of a cycle. Every four cycles, from the one before the first march (the kind 3 cycle), the even rows and the odd
// rows swap their tiles.
func flowerCycleOfFrame(frame int) (cycle, state int) {
	since := frame - flowerFirstFrame
	return since / flowerCycleFrames, since % flowerCycleFrames
}

func flowerRowsSwapped(cycle int) bool {
	return (cycle+1)/flowerCycleKinds%2 == 1
}

// The state 0 of the morph is the square the marching cubes end with
func flowerStateParams(state int) flowerParams {
	if state == 0 {
		return flowerParams{radial: 0, tangential: 0, square: flowerHeldSquare, squareY: 12, shade: 4}
	}
	return flowerParamsAtState(state)
}

// The state of a pair of kicks the step j (two states each) shows: 0-18 in the first kick, 40-58 in the second
func flowerMarchStepState(step int) int {
	if step < flowerMarchTurns {
		return 2 * step
	}
	return flowerMarchKickPeriod + 2*(step-flowerMarchTurns)
}

// The flower at the turn of step j without a shift (the record the C64 renders from)
func flowerMarchParams(step int) flowerParams {
	params := flowerParamsAtState(flowerLastState)
	params.turn = float64(flowerMarchTurnStart - flowerMarchTurnStep*(step%flowerMarchTurns))
	params.phase = 0
	return params
}

// The two tiles of the step j of the marching flowers: the flower drawn at its shift, the odd rows' tile its mirror image
func flowerMarchPair(step int) (even, odd morphCoverage) {
	return flowerMarchImages(&flowerSweeps, flowerMarchStepState(step), false)
}

// The 50 Hz states between the states the ZX shows: the odd states of the morph and of the marching kicks. The
// frame between two shown states shows the state between them, which the ZX holds but the C64 draws.
// The turn and the shift of the kick state 2 * step + 1 (the second kick of the pair starts at the state 40)
func flowerBetweenKickState(step int) int {
	if step < flowerMarchTurns {
		return 2*step + 1
	}
	return flowerMarchKickPeriod + 2*(step-flowerMarchTurns) + 1
}

func flowerBetweenMarchParams(step int) flowerParams {
	params := flowerParamsAtState(flowerLastState)
	params.turn = float64(flowerMarchTurnStart - flowerBetweenTurnStep - flowerMarchTurnStep*(step%flowerMarchTurns))
	params.phase = 0
	return params
}

func flowerBetweenMarchPair(step int) (even, odd morphCoverage) {
	return flowerMarchImages(&flowerSweeps, flowerBetweenKickState(step), false)
}

// The tiles of the odd states 3-17 (the indices 0-7)
func flowerBetweenMorphPair(index int) (even, odd morphCoverage) {
	params := flowerParamsAtState(3 + 2*index)
	return flowerCoverage(params), flowerOddCoverage(params)
}

// The tiles of the backwards morph between its states: the odd states 15-1 mirrored (the indices 0-7)
func flowerBetweenMirroredPair(index int) (even, odd morphCoverage) {
	mirrored := mirrorCoverage(flowerCoverage(flowerParamsAtState(15 - 2*index)))
	return mirrored, flipCoverageAboutRow(mirrored, flowerOddFlipRow)
}

// The tiles of the backwards morph, which is the morph mirrored: the index 0 is the state 16, the index 8 the square
func flowerMirroredPair(index int) (even, odd morphCoverage) {
	mirrored := mirrorCoverage(flowerCoverage(flowerStateParams(flowerLastState - 2*(index+1))))
	return mirrored, flipCoverageAboutRow(mirrored, flowerOddFlipRow)
}

// The tiles of the even states 2-18 (the indices 0-8) and of the square (the index 9)
func flowerMorphPair(index int) (even, odd morphCoverage) {
	params := flowerStateParams(2 * (index + 1) % (flowerLastState + 2))
	return flowerCoverage(params), flowerOddCoverage(params)
}

// The tiles a state of a cycle shows
func flowerCycleTiles(cycle, state int) (even, odd morphCoverage) {
	step := state / 2
	swapped := flowerRowsSwapped(cycle)
	switch cycle % flowerCycleKinds {
	case 0:
		even, odd = flowerMorphPair(min(max(step, 1), flowerMorphPairs) - 1)
	case 1:
		even, odd = flowerMarchPair(min(step, flowerMarchTurns))
	case 2:
		even, odd = flowerMarchPair((flowerMarchTurns + step) % flowerMarchPairs)
	default:
		even, odd = flowerMirroredPair(min(max(step, 1), flowerMirroredPairs) - 1)
		swapped = !swapped
	}
	if swapped {
		even, odd = odd, even
	}
	return even, odd
}

func verifyFlowerCycles() {
	worst, frames := 0, 0
	for frame := flowerFirstFrame + 2; frame < 3980; frame += 2 {
		cycle, state := flowerCycleOfFrame(frame)
		screen, err := loadReferenceScreen(frame)
		if err != nil || state > flowerMarchKickStates || cycle > flowerLastCycle {
			continue
		}
		even, odd := flowerCycleTiles(cycle, state)
		hardEven := countTileMismatches(even, &screen, referenceTileOrigin, referenceTileOrigin)
		hardOdd := countTileMismatches(odd, &screen, referenceTileOrigin, referenceTileOrigin+morphTileSize)
		frames++
		worst = max(worst, hardEven, hardOdd)
		if max(hardEven, hardOdd) > 4 {
			fmt.Printf("frame %d (cycle %d kind %d state %d): %d and %d mismatches\n", frame, cycle, cycle%flowerCycleKinds, state, hardEven, hardOdd)
		}
	}
	fmt.Printf("Flower cycles: %d ZX frames, worst tile has %d mismatches (limit 8)\n", frames, worst)
}

func countCoverageDifferences(a, b morphCoverage) int {
	different := 0
	for y := range a {
		for x := range a[y] {
			if a[y][x] != b[y][x] {
				different++
			}
		}
	}
	return different
}

// The tiles the C64 renders for the marching flowers, each at its own shift: the even states 2-10, then (in the other
// file) the odd states 1-9. The states 11-20 are mirrors of these (see flowerMarchSource).
func writeFlowerTurnTiles() {
	var tiles []byte
	for state := 2; state <= flowerMarchKickStates/2; state += 2 {
		even, _ := flowerMarchImages(&flowerSweeps, state, false)
		tiles = append(tiles, packMorphCoverage(even)...)
	}
	if err := os.WriteFile(flowerTilesPath, tiles, 0644); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", flowerTilesPath, err)
	}
}

func writeFlowerBetweenTurnTiles() {
	var tiles []byte
	for state := 1; state < flowerMarchKickStates/2; state += 2 {
		even, _ := flowerMarchImages(&flowerSweeps, state, false)
		tiles = append(tiles, packMorphCoverage(even)...)
	}
	if err := os.WriteFile(flowerBetweenTurnTilesPath, tiles, 0644); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", flowerBetweenTurnTilesPath, err)
	}
}

func writeFlowerSceneImages() {
	writeFlowerTurnTiles()
	writeFlowerBetweenTurnTiles()
	var march []morphCoverage
	for step := 0; step < flowerMarchPairs; step++ {
		even, odd := flowerMarchPair(step)
		march = append(march, even, odd)
	}
	for step := 0; step < flowerMarchPairs; step++ {
		even, odd := flowerBetweenMarchPair(step)
		march = append(march, even, odd)
	}
	if err := writeTileImages(flowerSceneImagesPath, march); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", flowerSceneImagesPath, err)
		return
	}
	fmt.Printf("Exported %s (%d images, %d bytes each: the pairs of the 20 steps, then of the 20 states between them)\n", flowerSceneImagesPath, len(march), imageBytes)
}

// What the C64 reads for the marching flowers: the records of the turns 1-9 (the turn 0 is the last state of the
// morph) and the shift of each step
func writeFlowerSceneData() {
	var text strings.Builder
	text.WriteString("; Generated by proto (go run -C proto . flower), do not edit\n")
	text.WriteString("; The records of the marching flowers' turns 1-5 (the turns 6-9 are those flipped, the turn 0 is the morph's last state, see flower_data.inc), then the shift in 1/16 pixel of the states 2, 4 ... 10 and of the states 1, 3 ... 9 (the states the C64 renders), then where the 40 pairs of the steps and of the states between them come from (flowerMarchSource), then the same for the states between (shifts, records of the turns 0-4)\n\n")
	text.WriteString(".segment \"FLOWERDATA\"\n\n.export flower_march_records\n.export flower_march_shifts\n.export flower_march_between_shifts\n.export flower_march_sources\n.export flower_march_between_records\n\nflower_march_records:\n")
	for turn := 1; turn <= flowerMarchTurns/2; turn++ {
		record := newFlowerRecord(flowerMarchParams(turn))
		if !flowerRecordFits(&record) {
			fmt.Printf("Warning: the record of turn %d does not fit its bytes\n", turn)
		}
		fmt.Fprintf(&text, "    ; turn %d\n", turn)
		text.WriteString(flowerByteLine(flowerRecordValues(&record)))
	}
	text.WriteString("\nflower_march_shifts:\n")
	var shifts, betweenShifts []int
	for state := 2; state <= flowerMarchKickStates/2; state += 2 {
		shifts = append(shifts, flowerMarchShiftUnits(state))
	}
	for state := 1; state < flowerMarchKickStates/2; state += 2 {
		betweenShifts = append(betweenShifts, flowerMarchShiftUnits(state))
	}
	text.WriteString(flowerByteLine(shifts))
	text.WriteString("\nflower_march_between_shifts:\n")
	text.WriteString(flowerByteLine(betweenShifts))
	text.WriteString("\nflower_march_sources:\n")
	var sources, betweenSources []int
	for step := 0; step < flowerMarchPairs; step++ {
		sources = append(sources, flowerMarchSource(step, false))
		betweenSources = append(betweenSources, flowerMarchSource(step, true))
	}
	text.WriteString(flowerByteLine(sources))
	text.WriteString(flowerByteLine(betweenSources))
	text.WriteString("\n.segment \"FLOWERDATA2\"\n\nflower_march_between_records:\n")
	for turn := 0; turn < flowerMarchTurns/2; turn++ {
		record := newFlowerRecord(flowerBetweenMarchParams(turn))
		if !flowerRecordFits(&record) {
			fmt.Printf("Warning: the record of the turn between %d does not fit its bytes\n", turn)
		}
		fmt.Fprintf(&text, "    ; turn between %d\n", turn)
		text.WriteString(flowerByteLine(flowerRecordValues(&record)))
	}
	if err := os.WriteFile(flowerSceneDataPath, []byte(text.String()), 0644); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", flowerSceneDataPath, err)
		return
	}
	fmt.Printf("Exported %s\n", flowerSceneDataPath)
}

func verifyFlowerTurnedPetals() {
	for turn := 0; turn < flowerMarchTurns; turn++ {
		record := newFlowerRecord(flowerMarchParams(turn))
		direct := record.coverage(0, 0)
		turned := record.coverageOfRows(record.halfTurnRows(0), 0)
		different := 0
		for y := range direct {
			for x := range direct[y] {
				if direct[y][x] != turned[y][x] {
					different++
				}
			}
		}
		fmt.Printf("turn %d: the turned petals differ from the drawn ones in %d pixels\n", turn, different)
	}
}

func verifyFlowerFlipsAreMirrors() {
	different := 0
	for step := 0; step < flowerMarchPairs; step++ {
		even, odd := flowerMarchPair(step)
		different += countCoverageDifferences(odd, mirrorCoverage(even))
	}
	fmt.Printf("Marching flowers: the odd rows' tiles differ from the mirrored even rows' tiles in %d pixels\n", different)
}

// The tiles of the 50 Hz state between the steps (state - 1) and (state + 1) of a cycle, which the C64 draws
func flowerBetweenTiles(cycle, state int) (even, odd morphCoverage) {
	step := state / 2
	swapped := flowerRowsSwapped(cycle)
	switch cycle % flowerCycleKinds {
	case 0:
		params := flowerParamsAtState(state)
		even, odd = flowerCoverage(params), flowerOddCoverage(params)
	case 1:
		even, odd = flowerBetweenMarchPair(step)
	case 2:
		even, odd = flowerBetweenMarchPair((flowerMarchTurns + step) % flowerMarchPairs)
	default:
		even, odd = flowerBetweenMirroredPair(step - 1)
		swapped = !swapped
	}
	if swapped {
		even, odd = odd, even
	}
	return even, odd
}

func unexplainedPixels(tile, before, after morphCoverage) int {
	shown := func(frame morphCoverage, x, y int) bool {
		for dx := -1; dx <= 1; dx++ {
			if x+dx >= 0 && x+dx < morphTileSize && frame[y][x+dx] == tile[y][x] {
				return true
			}
		}
		return false
	}
	unexplained := 0
	for y := range tile {
		for x := range tile[y] {
			if !shown(before, x, y) && !shown(after, x, y) {
				unexplained++
			}
		}
	}
	return unexplained
}

// Every state between two shown ones must be neither of them and made of what they show (as the frame comparison requires)
func verifyFlowerBetweenStates() {
	for cycle := 0; cycle < flowerCycleKinds; cycle++ {
		worst, same := 0, 0
		for state := 3; state < flowerMarchKickStates; state += 2 {
			if state > flowerLastState-1 && cycle%flowerCycleKinds%3 == 0 {
				continue
			}
			beforeEven, beforeOdd := flowerCycleTiles(cycle, state-1)
			afterEven, afterOdd := flowerCycleTiles(cycle, state+1)
			even, odd := flowerBetweenTiles(cycle, state)
			worst = max(worst, unexplainedPixels(even, beforeEven, afterEven), unexplainedPixels(odd, beforeOdd, afterOdd))
			if (even == beforeEven && odd == beforeOdd) || (even == afterEven && odd == afterOdd) {
				same++
			}
		}
		fmt.Printf("Kind %d: worst tile has %d pixels that neither neighbour shows, %d in-between frames look like a neighbour\n", cycle, worst, same)
	}
}
