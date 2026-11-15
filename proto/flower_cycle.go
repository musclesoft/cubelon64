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
	flowerMarchKickStates = 20
	flowerMorphPairs      = flowerLastState / 2
	flowerMirroredPairs   = flowerMorphPairs
	flowerSceneImagesPath = "golden/flower_march_images.bin"
	flowerMirroredPath    = "golden/flower_mirrored_images.bin"
	flowerSceneDataPath   = "../c64port/src/flower_march_data.inc"
	flowerTilesPath       = "golden/flower_march_tiles.bin"
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

func rollCoverage(coverage morphCoverage, shift int) morphCoverage {
	var rolled morphCoverage
	for y := range coverage {
		for x := range coverage[y] {
			rolled[y][((x+shift)%morphTileSize+morphTileSize)%morphTileSize] = coverage[y][x]
		}
	}
	return rolled
}

// The shift of the flower in the pair of kicks, at the step j (two states each): 0-10 in the first kick, 10-19 in the
// second, which is the first rolled by 12 pixels
func flowerMarchShift(step int) int {
	if step < flowerMarchTurns {
		return flowerRound(flowerMarchSweeps.shift.at(2*step), 1)
	}
	return flowerRound(flowerMarchSweeps.shift.at(flowerMarchKickPeriod+2*(step-flowerMarchTurns)), 1)
}

// The flower at the turn of step j without a shift. The turns repeat every ten steps, as the flower repeats every 60 degrees.
func flowerMarchParams(step int) flowerParams {
	params := flowerParamsAtState(flowerLastState)
	params.turn = float64(flowerMarchTurnStart - flowerMarchTurnStep*(step%flowerMarchTurns))
	params.phase = 0
	return params
}

// The two tiles of the step j of the marching flowers: the even rows' tile is the flower rolled to the right by the
// shift, the odd rows' tile is the flower rolled to the left and flipped
func flowerMarchPair(step int) (even, odd morphCoverage) {
	base := flowerCoverage(flowerMarchParams(step))
	shift := flowerMarchShift(step)
	return rollCoverage(base, shift), flipCoverageAboutRow(rollCoverage(base, -shift), flowerOddFlipRow)
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

// The rolled tiles must be what the sweeps' direct drawing gives
func verifyFlowerMarchRolls() {
	different := 0
	for step := 0; step < flowerMarchPairs; step++ {
		kickState := 2 * step
		if step >= flowerMarchTurns {
			kickState = flowerMarchKickPeriod + 2*(step-flowerMarchTurns)
		}
		wantEven, wantOdd := flowerMarchImages(&flowerSweeps, kickState, false)
		gotEven, gotOdd := flowerMarchPair(step)
		if gotEven != wantEven || gotOdd != wantOdd {
			different++
			fmt.Printf("step %d: the rolled tiles differ from the drawn ones\n", step)
		}
	}
	fmt.Printf("Marching flowers: %d of %d steps differ from the drawn turns\n", different, flowerMarchPairs)
}

// The tiles the C64 renders for the turns before it rolls them, in the order it renders them: the turns 1-9, then 0
func writeFlowerTurnTiles() {
	var tiles []byte
	for index := 1; index <= flowerMarchTurns; index++ {
		tiles = append(tiles, packMorphCoverage(flowerCoverage(flowerMarchParams(index%flowerMarchTurns)))...)
	}
	if err := os.WriteFile(flowerTilesPath, tiles, 0644); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", flowerTilesPath, err)
	}
}

func writeFlowerSceneImages() {
	writeFlowerTurnTiles()
	var march, mirrored []morphCoverage
	for step := 0; step < flowerMarchPairs; step++ {
		even, odd := flowerMarchPair(step)
		march = append(march, even, odd)
	}
	for index := 0; index < flowerMirroredPairs; index++ {
		even, odd := flowerMirroredPair(index)
		mirrored = append(mirrored, even, odd)
	}
	if err := writeTileImages(flowerSceneImagesPath, march); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", flowerSceneImagesPath, err)
		return
	}
	if err := writeTileImages(flowerMirroredPath, mirrored); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", flowerMirroredPath, err)
		return
	}
	fmt.Printf("Exported %s and %s (%d and %d images, %d bytes each)\n", flowerSceneImagesPath, flowerMirroredPath, len(march), len(mirrored), imageBytes)
}

// What the C64 reads for the marching flowers: the records of the turns 1-9 (the turn 0 is the last state of the
// morph) and the shift of each step
func writeFlowerSceneData() {
	var text strings.Builder
	text.WriteString("; Generated by proto (go run -C proto . flower), do not edit\n")
	text.WriteString("; The records of the marching flowers' turns 1-9 (see flower_data.inc), then the shift in pixels of each of their 20 steps\n\n")
	text.WriteString(".segment \"FLOWERDATA\"\n\n.export flower_march_records\n.export flower_march_shifts\n\nflower_march_records:\n")
	for turn := 1; turn < flowerMarchTurns; turn++ {
		record := newFlowerRecord(flowerMarchParams(turn))
		if !flowerRecordFits(&record) {
			fmt.Printf("Warning: the record of turn %d does not fit its bytes\n", turn)
		}
		fmt.Fprintf(&text, "    ; turn %d\n", turn)
		text.WriteString(flowerByteLine(flowerRecordValues(&record)))
	}
	text.WriteString("\nflower_march_shifts:\n")
	shifts := make([]int, flowerMarchPairs)
	for step := range shifts {
		shifts[step] = flowerMarchShift(step)
	}
	text.WriteString(flowerByteLine(shifts))
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
		turned := record.coverageOfRows(record.halfTurnRows(), 0)
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
