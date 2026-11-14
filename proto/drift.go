package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"strings"
)

const (
	driftLastState     = 20
	driftFirstFrame    = 2079
	driftDataPath      = "../c64port/src/drift_data.inc"
	driftImagesPath    = "golden/drift_images.bin"
	driftComparePath   = "golden/drift_compare.png"
	driftFormulaGrid   = 64
	driftFitRestarts   = 60
	driftRestartSpread = 0.3
)

const (
	driftNear = iota
	driftSide
	driftHalf
	driftFar
	driftFieldCount
)

// After the 20 states every kite has moved 12 pixels along its axis and 12 sideways, so that it is where
// another kite was in the first picture
const driftDistance = 12.0

// The kite's near vertex, its two side vertices at the half width and the distance of their line from
// the corner, its far vertex, all measured from the corner along the axis; proto fits them to the
// ZX frames (go run -C proto . drift fit)
var driftFormula = [driftFieldCount]float64{5.59375, 12.046875, 6.34375, 18.3125}

// The diamonds' last picture is the first of this scene: its kites, rigid, move on diagonally, the two
// kites of a bowtie in opposite directions, until each is where another one was: after 20 states they
// have moved 12 pixels along the axis of their bowtie, away from its corner, and 12 sideways, and the
// picture is the same again.
func driftKite() morphKite {
	f := driftFormula
	return morphKite{{0, f[driftNear]}, {f[driftHalf], f[driftSide]}, {0, f[driftFar]}, {-f[driftHalf], f[driftSide]}}
}

// The polygons of the tile at diamondOrigin in the state: a kite of every bowtie of the lattice
// around it, the first one of a standing bowtie below its corner, the other above it, moved by
// the steps along the axis and sideways, all in the direction away from the corner on the axis
func driftPolygons(state float64) []morphKite {
	kite := driftKite()
	steps := driftDistance * state / driftLastState
	var polygons []morphKite
	for row := -1; row <= 3; row++ {
		for column := -1; column <= 3; column++ {
			cornerX := float64(diamondOrigin + column*morphTileSize)
			cornerY := float64(diamondOrigin + row*morphTileSize)
			for _, direction := range []float64{1, -1} {
				var polygon morphKite
				for i, v := range kite {
					x, y := (v.x-steps)*direction, (v.y+steps)*direction
					if (column+row)%2 != 0 {
						x, y = y, x
					}
					polygon[i] = morphVertex{x: cornerX + x, y: cornerY + y}
				}
				polygons = append(polygons, polygon)
			}
		}
	}
	return polygons
}

// The ZX frame of an even state; the state 0 is the diamonds' last picture, held
func driftFrameOfState(state int) int {
	if state == 0 {
		return diamondFirstFrame + diamondLastState
	}
	return driftFirstFrame + state
}

func driftEvenTile(state int) morphCoverage {
	return mirrorCoverage(diamondTile(driftPolygons(float64(state)), diamondOrigin, diamondOrigin))
}

func runDrift(args []string) {
	var screens [driftLastState/diamondStateStep + 1]referenceScreen
	for index := range screens {
		var err error
		if screens[index], err = loadReferenceScreen(driftFrameOfState(diamondStateStep * index)); err != nil {
			fmt.Printf("No ZX reference: %v\n", err)
			return
		}
	}
	if len(args) > 0 && args[0] == "fit" {
		fitDriftFormula(&screens)
	}
	total := 0
	for index := 1; index < len(screens); index++ {
		state := diamondStateStep * index
		tile := driftEvenTile(state)
		if state == driftLastState {
			tile = diamondReferenceTile(&screens[0])
		}
		diff := countMorphDiff(fullMorphTile(tile), screens[index].tile(diamondOrigin, diamondOrigin))
		mismatches := countTileMismatches(tile, &screens[index], diamondOrigin, diamondOrigin)
		total += diff
		fmt.Printf("state %2d: %2d pixels differ from the ZX tile, %d frame mismatches\n", state, diff, mismatches)
	}
	fmt.Printf("Drift tiles: %d pixels differ in total\n", total)
	fmt.Printf("The formula's state %d differs from the first picture in %d pixels; the last picture is the first, exactly\n", driftLastState, countMorphDiff(fullMorphTile(driftEvenTile(driftLastState)), fullMorphTile(diamondReferenceTile(&screens[0]))))

	var all [driftLastState + 1]morphCoverage
	all[0] = diamondReferenceTile(&screens[0])
	for state := 1; state < driftLastState; state++ {
		all[state] = driftEvenTile(state)
	}
	all[driftLastState] = all[0]
	writeDriftComparison(&screens, &all)
	writeDriftImages(&all)
	writeDriftData(&all)
}

func fitDriftFormula(screens *[driftLastState/diamondStateStep + 1]referenceScreen) {
	cost := func(values []float64) int {
		saved := driftFormula
		copy(driftFormula[:], values)
		total := 0
		for index := 1; index < len(screens)-1; index++ {
			tile := driftEvenTile(diamondStateStep * index)
			total += countMorphDiff(fullMorphTile(tile), screens[index].tile(diamondOrigin, diamondOrigin))
			total += diamondMismatchWeight * countTileMismatches(tile, &screens[index], diamondOrigin, diamondOrigin)
		}
		driftFormula = saved
		return total
	}
	fmt.Printf("Formula before the fit: cost %d\n", cost(driftFormula[:]))
	steps := []float64{1, -1, 2, -2, 4, -4, 8, -8}
	fitted, fittedCost := hillClimb(driftFormula[:], driftFormulaGrid, steps, cost)
	rng := rand.New(rand.NewSource(1))
	for restart := 0; restart < driftFitRestarts && fittedCost > 0; restart++ {
		start := append([]float64(nil), fitted...)
		for i := range start {
			start[i] += (rng.Float64() - 0.5) * driftRestartSpread
		}
		candidate, candidateCost := hillClimb(start, driftFormulaGrid, steps, cost)
		if candidateCost < fittedCost {
			fitted, fittedCost = candidate, candidateCost
		}
	}
	fmt.Printf("Fitted formula: cost %d\nvar driftFormula = %v\n", fittedCost, fitted)
	copy(driftFormula[:], fitted)
}

// The states 1-20 as the C64 holds them: for every state the tile of the even cells, then its
// transpose, the tile of the odd cells (the lattice of the kites is its own transpose one tile on).
// State 0, the diamonds' last picture, is theirs
func writeDriftImages(all *[driftLastState + 1]morphCoverage) {
	var images []morphCoverage
	for _, coverage := range all[1:] {
		images = append(images, coverage, transposeCoverage(coverage))
	}
	if err := writeTileImages(driftImagesPath, images); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", driftImagesPath, err)
		return
	}
	fmt.Printf("Exported %s (%d states, the tiles of the even and of the odd cells, %d bytes each)\n", driftImagesPath, driftLastState, imageBytes)
}

// Rows 0-11 of each tile, the rest follows from its half turn symmetry
func writeDriftData(all *[driftLastState + 1]morphCoverage) {
	var out strings.Builder
	out.WriteString("; Generated by proto (go run . drift), do not edit\n")
	out.WriteString("; The tiles of the drift's states 1-20: tile rows 0-11 of 6 bytes (3 full mask, 3 medium mask), the other rows\n")
	out.WriteString("; follow from the 180 degree symmetry of the tile. The tile of the odd cells of the checkerboard is its transpose\n\n")
	out.WriteString(".segment \"RODATA\"\n\n")
	out.WriteString(".export drift_half_tiles\ndrift_half_tiles:\n")
	bytes := 0
	for state := 1; state <= driftLastState; state++ {
		packed := packMorphCoverage(all[state])
		fmt.Fprintf(&out, "    ; state %d\n", state)
		for row := 0; row < diamondHalfRows; row++ {
			rowBytes := packed[row*morphRowBytes : (row+1)*morphRowBytes]
			fmt.Fprintf(&out, "    .byte $%02X, $%02X, $%02X, $%02X, $%02X, $%02X\n", rowBytes[0], rowBytes[1], rowBytes[2], rowBytes[3], rowBytes[4], rowBytes[5])
			bytes += morphRowBytes
		}
	}
	if err := writeTextFile(driftDataPath, out.String()); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", driftDataPath, err)
		return
	}
	fmt.Printf("Exported %s (%d bytes of data)\n", driftDataPath, bytes)
}

func writeDriftComparison(screens *[driftLastState/diamondStateStep + 1]referenceScreen, all *[driftLastState + 1]morphCoverage) {
	const gap = 4
	cell := morphTileSize * morphScale
	img := image.NewRGBA(image.Rect(0, 0, len(all)*(cell+gap)-gap, 3*(cell+gap)-gap))
	fillMorphRect(img, img.Rect, color.RGBA{R: 40, G: 60, B: 160, A: 255})
	for state := range all {
		left := state * (cell + gap)
		drawMorphCoverage(img, left, cell+gap, all[state])
		if state%diamondStateStep == 0 {
			reference := screens[state/diamondStateStep].tile(diamondOrigin, diamondOrigin)
			drawMorphReference(img, left, 0, reference)
			drawMorphDiff(img, left, 2*(cell+gap), reference, fullMorphTile(all[state]))
		}
	}
	file, err := os.Create(driftComparePath)
	if err != nil {
		fmt.Printf("Warning: failed to create %s: %v\n", driftComparePath, err)
		return
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		fmt.Printf("Warning: failed to encode %s: %v\n", driftComparePath, err)
		return
	}
	fmt.Printf("Exported %s (rows: ZX reference, tile as the C64 shows it, difference; one column per 50 Hz state 0-20, the ZX demo shows the even ones)\n", driftComparePath)
}
