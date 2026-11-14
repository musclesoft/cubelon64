package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/bits"
	"math/rand"
	"os"
	"strings"
)

const (
	diamondLastState      = 20
	diamondStateStep      = 2
	diamondStates         = diamondLastState / diamondStateStep
	diamondOrigin         = referenceTileOrigin
	diamondFormulaGrid    = 64
	diamondMismatchWeight = 8
	diamondFitRestarts    = 60
	diamondRestartSpread  = 0.3
	diamondDataPath       = "../c64port/src/diamond_data.inc"
	diamondImagesPath     = "golden/diamond_images.bin"
	diamondComparePath    = "golden/diamond_compare.png"
	diamondHalfRows       = morphTileSize / 2
	diamondFirstFrame     = 2039
	diamondSquareFrame    = 2019
)

const (
	diamondSideX = iota
	diamondSideY
	diamondTip
	diamondFieldCount
)

// The edge from B to the corner starts across the bowtie, at 90 degrees to the line across it, and
// turns by 45 degrees, the square's turn into a diamond
const (
	diamondAngleStart = 90.0
	diamondAngleEnd   = 45.0
)

// The animation of the bowties turning into diamonds: the bowtie of the split goes on, its two
// kites move apart and become diamonds. It starts from the large square and takes 20 states.
// A bowtie sits at every corner of the tiles, standing where the column and the row of the
// corner add up to an even number and lying where they add up to an odd number; its quadrant
// with x across the bowtie and y along its axis is the polygon below. B = (side x, side y) is the
// vertex on the side, the tip is the far vertex on the axis, and the edge from B towards the
// corner makes an angle with the line across the bowtie. proto fits the start and the end value
// of B and the tip, and all four move linearly (go run -C proto . diamond fit).
var diamondFormula = [2 * diamondFieldCount]float64{8.046875, 5.953125, 8.28125, 11.65625, 7.796875, 17.890625}

// The fixed ends of the animation, exactly: the large square, whose quadrant is a square of 8 pixels
// with the edge from B across the bowtie, and the diamonds, kites of 12 by 12 pixels 12 pixels from
// the corner with the edge turned by 45 degrees. The states in between move from the fitted start
// to the fitted end of the formula; the states next to the ends are halfway between the end and
// their neighbour
var (
	diamondStart = [diamondFieldCount]float64{8, 8, 8}
	diamondEnd   = [diamondFieldCount]float64{6, 12, 18}
)

func diamondValue(field, state int) float64 {
	switch state {
	case 0:
		return diamondStart[field]
	case diamondLastState:
		return diamondEnd[field]
	case 1:
		return (diamondStart[field] + diamondFormulaValue(field, 2)) / 2
	case diamondLastState - 1:
		return (diamondFormulaValue(field, diamondLastState-2) + diamondEnd[field]) / 2
	}
	return diamondFormulaValue(field, state)
}

func diamondFormulaValue(field, state int) float64 {
	return diamondLine(diamondFormula[2*field], diamondFormula[2*field+1], state)
}

func diamondLine(start, end float64, state int) float64 {
	return start + (end-start)*float64(state)/diamondLastState
}

// The quadrant of a bowtie with its corner at the origin, for x, y >= 0. The edge from B at the
// angle runs down to the line across the bowtie (y = 0) at the waist, while that is on the right
// of the corner. Once the waist has passed the corner the two kites have separated, and the edge
// ends on the axis (x = 0), at the near vertex of the kite.
func diamondQuadrant(state int) morphKite {
	sideX, sideY, tip := diamondValue(diamondSideX, state), diamondValue(diamondSideY, state), diamondValue(diamondTip, state)
	tangent := math.Tan(diamondLine(diamondAngleStart, diamondAngleEnd, state) * math.Pi / 180)
	side := morphVertex{x: sideX, y: sideY}
	far := morphVertex{x: 0, y: tip}
	if waist := sideX - sideY/tangent; waist >= 0 {
		return morphKite{{0, 0}, {waist, 0}, side, far}
	}
	near := morphVertex{x: 0, y: sideY - sideX*tangent}
	return morphKite{near, near, side, far}
}

// The polygons of the bowties at the four corners of the tile at diamondOrigin, four quadrants
// each. A quadrant reaches 18 pixels from its corner at most, so the corners of the neighbouring
// tiles do not touch the tile
func diamondLatticePolygons(state int) []morphKite {
	quadrant := diamondQuadrant(state)
	var polygons []morphKite
	for row := 0; row <= 1; row++ {
		for column := 0; column <= 1; column++ {
			cornerX := float64(diamondOrigin + column*morphTileSize)
			cornerY := float64(diamondOrigin + row*morphTileSize)
			for _, signs := range [4][2]float64{{1, 1}, {-1, 1}, {1, -1}, {-1, -1}} {
				var polygon morphKite
				for i, v := range quadrant {
					x, y := v.x*signs[0], v.y*signs[1]
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

// The ZX frame of an even state; the large square of the state 0 is held from the frame before
func diamondFrameOfState(state int) int {
	if state == 0 {
		return diamondSquareFrame
	}
	return diamondFirstFrame + state
}

// The ZX demo shows the mirror image of the lattice's tile on the even cells of the checkerboard
// and the tile itself on the odd cells; the neighbouring tile is irrelevant for the fit, so the
// tile of the even cell is rendered for each state. The C64 keeps the rows 0-11 of the tile itself.
func diamondEvenTile(state int) morphCoverage {
	return mirrorCoverage(diamondTile(diamondLatticePolygons(state), diamondOrigin, diamondOrigin))
}

func runDiamond(args []string) {
	var screens [diamondStates + 1]referenceScreen
	for index := range screens {
		var err error
		if screens[index], err = loadReferenceScreen(diamondFrameOfState(diamondStateStep * index)); err != nil {
			fmt.Printf("No ZX reference: %v\n", err)
			return
		}
	}
	if len(args) > 0 && args[0] == "fit" {
		fitDiamondFormula(&screens)
	}

	var tiles [diamondStates + 1]morphCoverage
	total := 0
	for index := range tiles {
		state := diamondStateStep * index
		tiles[index] = diamondEvenTile(state)
		if index == diamondStates {
			tiles[index] = diamondReferenceTile(&screens[index])
		}
		diff := countMorphDiff(fullMorphTile(tiles[index]), screens[index].tile(diamondOrigin, diamondOrigin))
		mismatches := countTileMismatches(tiles[index], &screens[index], diamondOrigin, diamondOrigin)
		total += diff
		fmt.Printf("state %2d: %2d pixels differ from the ZX tile, %d frame mismatches\n", state, diff, mismatches)
	}
	fmt.Printf("Diamond tiles: %d pixels differ in total\n", total)

	checkDiamondSymmetry(&tiles)
	writeDiamondComparison(&screens, &tiles)
	writeDiamondImages(&tiles)
	writeDiamondData(&tiles)
}

// The tile of every state at 50 Hz, 0-20: the even states are those the ZX demo shows, the odd ones
// come from the formula and have no ZX frame
func diamondFiftyHertzTiles(tiles *[diamondStates + 1]morphCoverage) [diamondLastState + 1]morphCoverage {
	var all [diamondLastState + 1]morphCoverage
	for state := range all {
		if state%diamondStateStep == 0 {
			all[state] = tiles[state/diamondStateStep]
		} else {
			all[state] = diamondEvenTile(state)
		}
	}
	return all
}

// The tile of the last state taken from the ZX frame as it is: the diamonds are a constant, only
// the states between the large square and the diamonds follow the formula
func diamondReferenceTile(screen *referenceScreen) morphCoverage {
	var coverage morphCoverage
	for y := range coverage {
		for x := range coverage[y] {
			if screen.lit(diamondOrigin+x, diamondOrigin+y) {
				coverage[y][x] = morphFullCoverage
			}
		}
	}
	return coverage
}

func fitDiamondFormula(screens *[diamondStates + 1]referenceScreen) {
	cost := func(values []float64) int {
		saved := diamondFormula
		copy(diamondFormula[:], values)
		total := 0
		for index := 1; index < diamondStates; index++ {
			tile := diamondEvenTile(diamondStateStep * index)
			total += countMorphDiff(fullMorphTile(tile), screens[index].tile(diamondOrigin, diamondOrigin))
			total += diamondMismatchWeight * countTileMismatches(tile, &screens[index], diamondOrigin, diamondOrigin)
		}
		diamondFormula = saved
		return total
	}
	fmt.Printf("Formula before the fit: cost %d (pixels that differ from the ZX tiles plus the weighted frame mismatches)\n", cost(diamondFormula[:]))
	steps := []float64{1, -1, 2, -2, 4, -4, 8, -8}
	fitted, fittedCost := hillClimb(diamondFormula[:], diamondFormulaGrid, steps, cost)
	rng := rand.New(rand.NewSource(1))
	for restart := 0; restart < diamondFitRestarts && fittedCost > 0; restart++ {
		start := append([]float64(nil), fitted...)
		for i := range start {
			start[i] += (rng.Float64() - 0.5) * diamondRestartSpread
		}
		candidate, candidateCost := hillClimb(start, diamondFormulaGrid, steps, cost)
		if candidateCost < fittedCost {
			fitted, fittedCost = candidate, candidateCost
		}
	}
	fmt.Printf("Fitted formula: cost %d\nvar diamondFormula = %v\n", fittedCost, fitted)
	copy(diamondFormula[:], fitted)
}

// The coverage levels of the tile with its corner at the pixel (originX, originY)
func diamondTile(polygons []morphKite, originX, originY int) morphCoverage {
	var coverage morphCoverage
	for y := 0; y < diamondHalfRows; y++ {
		for x := range coverage[y] {
			count := 0
			for subY := 0; subY < morphSubsamples; subY++ {
				for subX := 0; subX < morphSubsamples; subX++ {
					px := float64(originX+x) + (float64(subX)+0.5)/morphSubsamples
					py := float64(originY+y) + (float64(subY)+0.5)/morphSubsamples
					for _, polygon := range polygons {
						if insideMorphPolygon(polygon, px, py) {
							count++
							break
						}
					}
				}
			}
			coverage[y][x] = morphCoverageLevel(count)
		}
	}
	return halfTurnOfUpperHalf(coverage)
}

// The lattice turns into itself under a half turn about the centre of the tile; sampling an edge that
// runs exactly through the sample points is not symmetric, so the lower half is made from the upper
func halfTurnOfUpperHalf(coverage morphCoverage) morphCoverage {
	for y := diamondHalfRows; y < morphTileSize; y++ {
		for x := range coverage[y] {
			coverage[y][x] = coverage[morphTileSize-1-y][morphTileSize-1-x]
		}
	}
	return coverage
}

// Like countFrameMismatches for the tile with its corner at (originX, originY); the coverage is
// already in levels
func countTileMismatches(coverage morphCoverage, screen *referenceScreen, originX, originY int) int {
	mismatches := 0
	for y := range coverage {
		for x, level := range coverage[y] {
			if !diamondPixelAccepted(screen, originX+x, originY+y, level) {
				mismatches++
			}
		}
	}
	return mismatches
}

func diamondPixelAccepted(screen *referenceScreen, x, y, level int) bool {
	switch level {
	case 0:
		return !screen.lit(x, y)
	case morphFullCoverage:
		return screen.lit(x, y)
	}
	lit := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if screen.lit(x+dx, y+dy) {
				lit++
			}
		}
	}
	return lit > 0 && lit < 9
}

// The tiles turn into themselves under a half turn, so rows 0-11 of each rebuild it
func checkDiamondSymmetry(tiles *[diamondStates + 1]morphCoverage) {
	for index, coverage := range tiles {
		for y := range coverage {
			for x := range coverage[y] {
				if coverage[y][x] != coverage[morphTileSize-1-y][morphTileSize-1-x] {
					fmt.Printf("Warning: the tile of state %d is not symmetric by 180 degrees, its half tile cannot rebuild it\n", diamondStateStep*index)
					return
				}
			}
		}
	}
}

// The states 1-20 as the C64 holds them: for every state the tile of the even cells, then its
// transpose, which is its mirror image, the tile of the odd cells. State 0, the large square, is the morph's
func writeDiamondImages(tiles *[diamondStates + 1]morphCoverage) {
	all := diamondFiftyHertzTiles(tiles)
	var images []morphCoverage
	for state := 1; state <= diamondLastState; state++ {
		if transposeCoverage(all[state]) != mirrorCoverage(all[state]) {
			fmt.Printf("Warning: the transpose of the tile of state %d is not its mirror image\n", state)
		}
		images = append(images, all[state], transposeCoverage(all[state]))
	}
	if err := writeTileImages(diamondImagesPath, images); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", diamondImagesPath, err)
		return
	}
	fmt.Printf("Exported %s (%d states, the tiles of the even and of the odd cells, %d bytes each)\n", diamondImagesPath, diamondLastState, imageBytes)
}

// The tile turns into itself under a quarter turn, too, so its top left quarter,
// 12 rows of 12 pixels, rebuilds it: 3 bytes a row, the first 8 pixels of the full mask, the first
// 8 of the medium mask, and the next 4 of both as the two nibbles of the third byte
func diamondQuarterTile(packed []byte) [diamondHalfRows * 3]byte {
	var quarter [diamondHalfRows * 3]byte
	for row := 0; row < diamondHalfRows; row++ {
		bytes := packed[row*morphRowBytes : (row+1)*morphRowBytes]
		quarter[3*row] = bytes[0]
		quarter[3*row+1] = bytes[3]
		quarter[3*row+2] = bytes[1]&0xF0 | bytes[4]>>4
	}
	return quarter
}

// What the C64 does with a quarter: unpack its rows into a tile that is empty elsewhere (a set bit is
// not covered), turn it into the top right quarter, which is the quarter transposed and mirrored, and
// let the half turn make the lower half
func expandDiamondQuarter(quarter [diamondHalfRows * 3]byte) []byte {
	tile := make([]byte, morphTileStateBytes)
	for i := range tile {
		tile[i] = 0xFF
	}
	for row := 0; row < diamondHalfRows; row++ {
		tile[6*row] = quarter[3*row]
		tile[6*row+3] = quarter[3*row+1]
		tile[6*row+1] = quarter[3*row+2] | 0x0F
		tile[6*row+4] = quarter[3*row+2]<<4 | 0x0F
	}
	transposed := transposeTileBytes(tile)
	for row := 0; row < diamondHalfRows; row++ {
		for x, source := range [morphRowBytes]int{2, 1, 0, 5, 4, 3} {
			tile[6*row+x] &= bits.Reverse8(transposed[6*row+source])
		}
	}
	for row := 0; row < diamondHalfRows; row++ {
		for x, source := range [morphRowBytes]int{2, 1, 0, 5, 4, 3} {
			tile[6*(morphTileSize-1-row)+x] = bits.Reverse8(tile[6*row+source])
		}
	}
	return tile
}

// The tile of 24 rows of 6 bytes transposed: every mask of 3 bytes a row on its own
func transposeTileBytes(tile []byte) []byte {
	transposed := make([]byte, len(tile))
	for i := range transposed {
		transposed[i] = 0xFF
	}
	for mask := 0; mask < 2; mask++ {
		for y := 0; y < morphTileSize; y++ {
			for x := 0; x < morphTileSize; x++ {
				if tile[6*y+3*mask+x/8]&(0x80>>(x%8)) == 0 {
					transposed[6*x+3*mask+y/8] &^= 0x80 >> (y % 8)
				}
			}
		}
	}
	return transposed
}

func writeDiamondData(tiles *[diamondStates + 1]morphCoverage) {
	all := diamondFiftyHertzTiles(tiles)
	var out strings.Builder
	out.WriteString("; Generated by proto (go run . diamond), do not edit\n")
	out.WriteString("; The tiles of the diamond animation's states 1-20, each as its top left quarter: 12 rows of 3 bytes (the\n")
	out.WriteString("; first 8 pixels of the full mask, of the medium mask, and the next 4 of both in the nibbles of the third\n")
	out.WriteString("; byte). The tile has quarter turn symmetry; the tile of the odd cells of the checkerboard is its transpose\n\n")
	out.WriteString(".segment \"RODATA\"\n\n")
	out.WriteString(".export diamond_quarter_tiles\ndiamond_quarter_tiles:\n")
	bytes := 0
	for state := 1; state <= diamondLastState; state++ {
		packed := packMorphCoverage(all[state])
		quarter := diamondQuarterTile(packed)
		if got := expandDiamondQuarter(quarter); string(got) != string(packed) {
			fmt.Printf("Warning: the quarter of state %d does not rebuild its tile\n", state)
		}
		fmt.Fprintf(&out, "    ; state %d\n", state)
		for row := 0; row < diamondHalfRows; row++ {
			fmt.Fprintf(&out, "    .byte $%02X, $%02X, $%02X\n", quarter[3*row], quarter[3*row+1], quarter[3*row+2])
			bytes += 3
		}
	}
	if err := writeTextFile(diamondDataPath, out.String()); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", diamondDataPath, err)
		return
	}
	fmt.Printf("Exported %s (%d bytes of data)\n", diamondDataPath, bytes)
}

func writeDiamondComparison(screens *[diamondStates + 1]referenceScreen, tiles *[diamondStates + 1]morphCoverage) {
	const gap = 4
	all := diamondFiftyHertzTiles(tiles)
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
	file, err := os.Create(diamondComparePath)
	if err != nil {
		fmt.Printf("Warning: failed to create %s: %v\n", diamondComparePath, err)
		return
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		fmt.Printf("Warning: failed to encode %s: %v\n", diamondComparePath, err)
		return
	}
	fmt.Printf("Exported %s (rows: ZX reference, tile as the C64 shows it, difference; one column per 50 Hz state 0-20, the ZX demo shows the even ones)\n", diamondComparePath)
}
