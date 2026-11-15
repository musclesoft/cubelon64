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
	flowerFirstFrame     = 3359
	flowerSquareFrame    = 3339
	flowerLastState      = 18
	flowerFadeEnd        = 17
	flowerFadeStart      = 5
	flowerFadeTailSquare = 0.75
	flowerFadeTailState  = 12
	flowerPetals         = 6
	flowerOddFlipRow     = 11
	flowerCentreX        = 12.0
	flowerCentreY        = 11.5
	flowerSquareX        = 12.0
	flowerSquareY        = 12.0
	flowerComparePath    = "golden/flower_compare.png"
	flowerImagesPath     = "golden/flower_images.bin"
)

type flowerParams struct {
	shade      float64
	radial     float64
	ring       float64
	square     float64
	tangential float64
	turn       float64
	phase      int
	squareY    float64
}

func (params flowerParams) slice() []float64 {
	return []float64{params.shade, params.radial, params.ring, params.square, params.tangential, params.turn}
}

func flowerParamsOf(values []float64) flowerParams {
	return flowerParams{shade: values[0], radial: values[1], ring: values[2], square: values[3], tangential: values[4], turn: values[5]}
}

func flowerFrameOfState(state int) int {
	if state == 0 {
		return flowerSquareFrame
	}
	return flowerFirstFrame + state
}

func flowerCoverage(params flowerParams) morphCoverage {
	return flowerKindCoverage(params)
}

func flowerOddCoverage(params flowerParams) morphCoverage {
	return flipCoverageAboutRow(flowerKindCoverage(params), flowerOddFlipRow)
}

func flipCoverageAboutRow(coverage morphCoverage, row int) morphCoverage {
	var flipped morphCoverage
	for y := range coverage {
		if target := 2*row - y; target >= 0 && target < morphTileSize {
			flipped[target] = coverage[y]
		}
	}
	return flipped
}

func flowerKindCoverage(params flowerParams) morphCoverage {
	var coverage morphCoverage
	for y := range coverage {
		for x := range coverage[y] {
			petals, squareOnly := 0, 0
			for subY := 0; subY < morphSubsamples; subY++ {
				for subX := 0; subX < morphSubsamples; subX++ {
					px := float64(x) + (float64(subX)+0.5)/morphSubsamples
					py := float64(y) + (float64(subY)+0.5)/morphSubsamples
					switch {
					case inFlowerPetals(params, px, py):
						petals++
					case inFlowerSquare(params.square, px, py, params.squareY):
						squareOnly++
					}
				}
			}
			coverage[y][x] = flowerPixelLevel(int(math.Round(params.shade)), petals, squareOnly, x, y, params.phase)
		}
	}
	return coverage
}

// The solid square and the petals merge before the grey level is chosen, so a pixel half in a petal
// and half in the square is covered; a fading square only adds what lies outside the petals
func flowerPixelLevel(shade, petals, squareOnly, x, y, phase int) int {
	if shade == 4 {
		return morphCoverageLevel(petals + squareOnly)
	}
	return max(morphCoverageLevel(petals), flowerSquareLevel(shade, squareOnly, x, y, phase))
}

// The square fades in five steps: light grey, a checkerboard of light and dark grey, dark grey, a
// checkerboard of dark grey and black, black. The checkerboard shifts by a pixel (2 subpixels) on odd states.
func flowerSquareLevel(shade, count, x, y, phase int) int {
	if count == 0 {
		return 0
	}
	lit := flowerCheckerLit(x, y, phase)
	full := count >= morphFullCoverage
	switch shade {
	case 4:
		return morphCoverageLevel(count)
	case 3:
		if lit && full {
			return morphFullCoverage
		}
		return 1
	case 2:
		return 1
	case 1:
		if lit {
			return 1
		}
	}
	return 0
}

func flowerCheckerLit(x, y, phase int) bool {
	return (x+y+phase)%2 == 1
}

func inFlowerSquare(half, px, py, squareY float64) bool {
	return math.Abs(px-flowerSquareX) <= half && math.Abs(py-squareY) <= half
}

func inFlowerPetals(params flowerParams, px, py float64) bool {
	if params.radial <= 0 || params.tangential <= 0 {
		return false
	}
	for petal := 0; petal < flowerPetals; petal++ {
		angle := (params.turn + 360.0*float64(petal)/flowerPetals) * math.Pi / 180
		axisX, axisY := math.Cos(angle), -math.Sin(angle)
		dx := px - flowerCentreX - params.ring*axisX
		dy := py - flowerCentreY - params.ring*axisY
		along := dx*axisX + dy*axisY
		across := -dx*axisY + dy*axisX
		if along*along/(params.radial*params.radial)+across*across/(params.tangential*params.tangential) <= 1 {
			return true
		}
	}
	return false
}

func flowerMismatches(params flowerParams, screen *referenceScreen) (even, odd int) {
	coverage := flowerCoverage(params)
	even = countTileMismatches(coverage, screen, referenceTileOrigin, referenceTileOrigin)
	odd = countTileMismatches(flowerOddCoverage(params), screen, referenceTileOrigin, referenceTileOrigin+morphTileSize)
	return even, odd
}

func runFlower(args []string) {
	var screens [flowerLastState/2 + 1]referenceScreen
	for index := range screens {
		var err error
		if screens[index], err = loadReferenceScreen(flowerFrameOfState(2 * index)); err != nil {
			fmt.Printf("No ZX reference: %v\n", err)
			return
		}
	}
	table := flowerTable
	if len(args) > 0 && args[0] == "fit" {
		table = fitFlowerTable(&screens)
		printFlowerTable(table)
	}
	reportFlower(&screens, &table)
	writeFlowerComparison(&screens, &table)
	writeFlowerImages(&table)
}

func flowerTurn(state float64) float64 {
	return 84 - 3*state
}

type flowerRows [flowerLastState/2 + 1]flowerParams

var flowerTable = flowerRows{
	{radial: 0, ring: 5.25, square: 6, tangential: 0},
	{radial: 1.125, ring: 5.375, square: 5.125, tangential: 0.875},
	{radial: 1.875, ring: 5.625, square: 4.125, tangential: 1.5},
	{radial: 2.375, ring: 5.875, square: 2.75, tangential: 1.875},
	{radial: 2.5, ring: 6.25, square: 2.125, tangential: 2.125},
	{radial: 2.375, ring: 6.5, square: 1, tangential: 2.125},
	{radial: 2.625, ring: 6.75, square: 0.75, tangential: 2.125},
	{radial: 2.75, ring: 6.875, square: 0, tangential: 2.125},
	{radial: 2.875, ring: 7.25, square: 0, tangential: 2.125},
	{radial: 3, ring: 7.625, square: 0, tangential: 2.125},
}

func flowerParamsAtState(table *flowerRows, state float64) flowerParams {
	position := min(max(state/2, 0), float64(len(table)-1))
	lower := min(int(position), len(table)-2)
	fraction := position - float64(lower)
	values := make([]float64, 6)
	for i, a := range table[lower].slice() {
		values[i] = a + (table[lower+1].slice()[i]-a)*fraction
	}
	params := flowerParamsOf(values)
	params.turn = flowerTurn(state)
	params.phase = int(state) % 2
	return flowerFaded(params, state)
}

// The square fades from light grey (state 5) to black (state 15) in equal steps through the five
// shades and shrinks to nothing by then; before the tail only its size comes from the table
func flowerFaded(params flowerParams, state float64) flowerParams {
	params.squareY = flowerSquareCentreY(state)
	params.shade = min(max(4*(flowerFadeEnd-state)/(flowerFadeEnd-flowerFadeStart), 0), 4)
	if state > flowerFadeTailState {
		params.square = flowerFadeTailSquare * (flowerFadeEnd - state) / (flowerFadeEnd - flowerFadeTailState)
	}
	params.square = max(params.square, 0)
	return params
}

func flowerRowsFromValues(rows flowerRows, values []float64) flowerRows {
	for index := 1; index < len(rows); index++ {
		offset := (index - 1) * 4
		rows[index].radial, rows[index].ring, rows[index].square, rows[index].tangential = values[offset], values[offset+1], values[offset+2], values[offset+3]
	}
	return rows
}

func flowerValuesFromRows(rows flowerRows) []float64 {
	var values []float64
	for _, row := range rows[1:] {
		values = append(values, row.radial, row.ring, row.square, row.tangential)
	}
	return values
}

func fitFlowerTable(screens *[flowerLastState/2 + 1]referenceScreen) flowerRows {
	rows := flowerTable
	for round := 0; round < 3; round++ {
		cost := func(values []float64) int {
			candidate := flowerRowsFromValues(rows, values)
			total := 0
			for index := 1; index < len(candidate); index++ {
				params := candidate[index]
				params.turn = flowerTurn(float64(2 * index))
				params = flowerFaded(params, float64(2*index))
				even, odd := flowerMismatches(params, &screens[index])
				total += 4 * (even + odd)
			}
			return 4*total + int(4*flowerRoughness(&candidate))
		}
		values, _ := hillClimb(flowerValuesFromRows(rows), 8, []float64{1, -1, 2, -2, 4, -4}, cost)
		rows = flowerRowsFromValues(rows, values)
	}
	return rows
}

func flowerRoughness(rows *flowerRows) float64 {
	total := 0.0
	for index := 1; index+1 < len(rows); index++ {
		previous, current, next := rows[index-1].slice(), rows[index].slice(), rows[index+1].slice()
		for _, field := range []int{1, 2, 4} {
			bend := previous[field] - 2*current[field] + next[field]
			total += 3 * bend * bend
		}
	}
	return total
}

func printFlowerTable(rows flowerRows) {
	for _, row := range rows {
		fmt.Printf("\t{radial: %.4g, ring: %.4g, square: %.4g, tangential: %.4g},\n", row.radial, row.ring, row.square, row.tangential)
	}
}

func reportFlower(screens *[flowerLastState/2 + 1]referenceScreen, table *flowerRows) {
	worst := 0
	for index := 1; index < len(screens); index++ {
		state := 2 * index
		even, odd := flowerMismatches(flowerParamsAtState(table, float64(state)), &screens[index])
		fmt.Printf("state %2d (frame %d): %d mismatches in the even tile, %d in the odd tile\n", state, flowerFrameOfState(state), even, odd)
		worst = max(worst, even, odd)
	}
	fmt.Printf("Flower: worst tile has %d mismatches (limit 8)\n", worst)
}

func writeFlowerComparison(screens *[flowerLastState/2 + 1]referenceScreen, table *flowerRows) {
	const gap = 4
	const rows = 6
	cell := morphTileSize * morphScale
	img := image.NewRGBA(image.Rect(0, 0, (flowerLastState+1)*(cell+gap)-gap, rows*(cell+gap)-gap))
	fillMorphRect(img, img.Rect, color.RGBA{R: 40, G: 60, B: 160, A: 255})
	for state := 0; state <= flowerLastState; state++ {
		left := state * (cell + gap)
		params := flowerParamsAtState(table, float64(state))
		even, odd := flowerCoverage(params), flowerOddCoverage(params)
		if state == 0 {
			odd = even
		}
		drawMorphCoverage(img, left, cell+gap, even)
		drawMorphCoverage(img, left, 4*(cell+gap), odd)
		if state%2 != 0 {
			continue
		}
		screen := &screens[state/2]
		referenceEven := screen.tile(referenceTileOrigin, referenceTileOrigin)
		referenceOdd := screen.tile(referenceTileOrigin, referenceTileOrigin+morphTileSize)
		drawMorphReference(img, left, 0, referenceEven)
		drawFlowerDiff(img, left, 2*(cell+gap), even, screen, referenceTileOrigin)
		drawFlowerDiff(img, left, 3*(cell+gap), odd, screen, referenceTileOrigin+morphTileSize)
		drawMorphReference(img, left, 5*(cell+gap), referenceOdd)
	}
	file, err := os.Create(flowerComparePath)
	if err != nil {
		fmt.Printf("Warning: failed to create %s: %v\n", flowerComparePath, err)
		return
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		fmt.Printf("Warning: failed to encode %s: %v\n", flowerComparePath, err)
		return
	}
	fmt.Printf("Exported %s (rows: ZX even tile, C64 even tile, even difference, odd difference, C64 odd tile, ZX odd tile; one column per 50 Hz state 0-%d)\n", flowerComparePath, flowerLastState)
}

func writeFlowerImages(table *flowerRows) {
	even := make([]morphCoverage, flowerLastState)
	odd := make([]morphCoverage, flowerLastState)
	for state := 1; state <= flowerLastState; state++ {
		params := flowerParamsAtState(table, float64(state))
		even[state-1], odd[state-1] = flowerCoverage(params), flowerOddCoverage(params)
	}
	if err := writeTileImages(flowerImagesPath, even, odd); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", flowerImagesPath, err)
		return
	}
	fmt.Printf("Exported %s (the even and then the odd cells' %d states, %d bytes each)\n", flowerImagesPath, flowerLastState, imageBytes)
}

// Both kinds hold the square of state 0 at the same place and the flip of state 2 sets them apart; in
// between, the square sits on the flip row, which puts the two kinds' squares on top of each other
func flowerSquareCentreY(state float64) float64 {
	return flowerSquareY - 0.5*max(1-math.Abs(state-1), 0)
}

// Marks the pixels the frame comparison rejects (red: the ZX pixel is lit, cyan: it is dark); a grey
// pixel passes where the ZX picture has both lit and dark pixels around it, like a checkerboard
func drawFlowerDiff(img *image.RGBA, left, top int, coverage morphCoverage, screen *referenceScreen, originY int) {
	for y := range coverage {
		for x, level := range coverage[y] {
			lit := screen.lit(referenceTileOrigin+x, originY+y)
			shade := color.RGBA{A: 255}
			switch {
			case diamondPixelAccepted(screen, referenceTileOrigin+x, originY+y, level):
				if lit {
					shade = color.RGBA{R: 64, G: 64, B: 64, A: 255}
				}
			case lit:
				shade = color.RGBA{R: 255, A: 255}
			default:
				shade = color.RGBA{G: 255, B: 255, A: 255}
			}
			fillMorphRect(img, morphPixelRect(left, top, x, y), shade)
		}
	}
}
