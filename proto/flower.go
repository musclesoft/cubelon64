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
	flowerFirstFrame     = 3359
	flowerSquareFrame    = 3339
	flowerLastState      = 18
	flowerPetals         = 6
	flowerShapes         = flowerPetals / 2
	flowerHeldSquare     = 6
	flowerChordTableSize = 16
	flowerChordGrid      = 16
	flowerShearShift     = 5
	flowerShearGrid      = 1 << flowerShearShift
	flowerSweepGrid      = 256
	flowerOddFlipRow     = 11
	flowerCentreX        = 12.0
	flowerCentreY        = 11.5
	flowerSquareX        = 12.0
	flowerCentreUnitsX   = 12 * flowerChordGrid
	flowerCentreUnitsY   = 23 * flowerChordGrid / 2
	flowerTileUnits      = morphTileSize * flowerChordGrid
	flowerSubRows        = morphTileSize * morphSubsamples
	flowerSampleStep     = flowerChordGrid / morphSubsamples
	flowerSampleOffset   = flowerSampleStep / 2
	flowerCircleScale    = 255
	flowerMaxHalf        = 63
	flowerComparePath    = "golden/flower_compare.png"
	flowerFitComparePath = "golden/flower_compare_fit.png"
	flowerImagesPath     = "golden/flower_images.bin"
	flowerDataPath       = "../c64port/src/flower_data.inc"
)

type flowerParams struct {
	shade      float64
	radial     float64
	ring       float64
	square     float64
	squareY    float64
	tangential float64
	turn       float64
	phase      int
	shift      int
}

func flowerFrameOfState(state int) int {
	if state == 0 {
		return flowerSquareFrame
	}
	return flowerFirstFrame + state
}

func flowerCoverage(params flowerParams) morphCoverage {
	record := newFlowerRecord(params)
	return record.coverage(params.shift, params.phase)
}

func flowerOddCoverage(params flowerParams) morphCoverage {
	return flipCoverageAboutRow(flowerCoverage(params), flowerOddFlipRow)
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

// Every parameter is brought to integers the same way in every frame: scaled by its grid, 0.5 added,
// rounded down
func flowerRound(value, grid float64) int {
	return int(math.Floor(value*grid + 0.5))
}

// A petal is an ellipse, whose rows are chords: the centre of the chord moves along a straight line
// (the shear) and its half-width follows a half circle, so the C64 steps one row at a time with an add
// for the centre and a lookup of the width in a table of the half circle that is already multiplied
// by the petal's width. The petal k+3 is the petal k turned by half a turn, which has the same
// rows, so a state has three shapes. A flowerRecord holds everything the C64 reads to draw a state
// (the bytes of flower_data.inc); the marching flowers use the same records and only add the shift
// of the whole flower and wrap the petals at the tile. All of it is integers: lengths in 1/16 pixel,
// the shear in 1/32 pixel per 1/16 pixel.
type flowerShape struct {
	half, shear, width int
}

type flowerRecord struct {
	shade      int
	squareHalf int
	squareY    int
	shapes     [flowerShapes]flowerShape
	offsets    [flowerShapes][2]int
}

var flowerCircle = flowerCircleTable()

func flowerCircleTable() [flowerChordTableSize]int {
	var table [flowerChordTableSize]int
	for index := range table {
		table[index] = flowerRound(math.Sqrt(1-math.Pow((float64(index)+0.5)/flowerChordTableSize, 2)), flowerCircleScale)
	}
	return table
}

func newFlowerRecord(params flowerParams) flowerRecord {
	record := flowerRecord{
		shade:      flowerRound(params.shade, 1),
		squareHalf: flowerRound(params.square, flowerChordGrid),
		squareY:    flowerRound(params.squareY, flowerChordGrid),
	}
	if params.radial <= 0 || params.tangential <= 0 {
		return record
	}
	for index := range record.shapes {
		angle := flowerPetalAngle(params, index)
		record.shapes[index] = newFlowerShape(params, angle)
		record.offsets[index] = [2]int{
			flowerRound(params.ring*math.Cos(angle), flowerChordGrid),
			flowerRound(-params.ring*math.Sin(angle), flowerChordGrid),
		}
	}
	return record
}

func flowerPetalAngle(params flowerParams, petal int) float64 {
	return (params.turn + 360.0*float64(petal)/flowerPetals) * math.Pi / 180
}

func newFlowerShape(params flowerParams, angle float64) flowerShape {
	sine, cosine := math.Sin(angle), math.Cos(angle)
	radial, tangential := params.radial, params.tangential
	half := math.Sqrt(radial*radial*sine*sine + tangential*tangential*cosine*cosine)
	return flowerShape{
		half:  flowerRound(half, flowerChordGrid),
		shear: flowerRound(-(radial*radial-tangential*tangential)*sine*cosine/(half*half), flowerShearGrid),
		width: flowerRound(radial*tangential/half, flowerChordGrid),
	}
}

// The centre of the chord moves by shear * dy, rounded to 1/16 pixel with halves away from zero, so that
// a petal turned by half a turn has exactly the rows of the petal it is turned from
func (shape *flowerShape) shearAt(dy int) int {
	moved := shape.shear * dy
	if moved < 0 {
		return -((-moved + flowerShearGrid/2) >> flowerShearShift)
	}
	return (moved + flowerShearGrid/2) >> flowerShearShift
}

// The half-width of the chord at the distance (in 1/16 pixel) from the centre of the petal
func (shape *flowerShape) rowWidth(distance int) int {
	circle := flowerCircle[distance*flowerChordTableSize/shape.half]
	return (shape.width*circle + flowerCircleScale/2) / flowerCircleScale
}

// The petals' rows of subpixels, one bit for each of the 48 subpixels of a row and one row for each of the
// 48 sample rows. The samples lie at the middle of a subpixel: 4 and 12 sixteenths in a pixel. The petals
// are drawn at their shifted place; the pattern repeats every tile, so a petal that crosses the edge of
// the tile reappears on the other side.
func (record *flowerRecord) petalRows(shift int) [flowerSubRows]uint64 {
	var rows [flowerSubRows]uint64
	wraps := []int{0}
	if shift != 0 {
		wraps = []int{0, -flowerTileUnits, flowerTileUnits}
	}
	for petal := 0; petal < flowerPetals; petal++ {
		shape := &record.shapes[petal%flowerShapes]
		sign := 1 - 2*(petal/flowerShapes)
		offset := record.offsets[petal%flowerShapes]
		centreX := flowerCentreUnitsX + shift*flowerChordGrid + sign*offset[0]
		centreY := flowerCentreUnitsY + sign*offset[1]
		for row := range rows {
			dy := flowerSampleOffset + flowerSampleStep*row - centreY
			distance := max(dy, -dy)
			if distance >= shape.half {
				continue
			}
			halfWidth := shape.rowWidth(distance)
			centre := centreX + shape.shearAt(dy)
			for _, wrap := range wraps {
				rows[row] |= flowerSpan(centre+wrap, halfWidth)
			}
		}
	}
	return rows
}

func (record *flowerRecord) squareRows() [flowerSubRows]uint64 {
	var rows [flowerSubRows]uint64
	span := flowerSpan(flowerCentreUnitsX, record.squareHalf)
	for row := range rows {
		dy := flowerSampleOffset + flowerSampleStep*row - record.squareY
		if max(dy, -dy) <= record.squareHalf {
			rows[row] = span
		}
	}
	return rows
}

// The subpixels whose samples lie within halfWidth of the centre (all in 1/16 pixel): the sample of
// subpixel j is at 8j+4
func flowerSpan(centre, halfWidth int) uint64 {
	first := max((centre-halfWidth+flowerSampleStep-flowerSampleOffset-1)>>3, 0)
	last := min((centre+halfWidth-flowerSampleOffset)>>3, flowerSubRows-1)
	if first > last {
		return 0
	}
	return (uint64(1)<<(last-first+1) - 1) << first
}

func (record *flowerRecord) coverage(shift, phase int) morphCoverage {
	return record.coverageOfRows(record.petalRows(shift), phase)
}

// The first three petals' rows together with the same rows turned by half a turn about the centre of the flower: the
// sample row k of the turned rows is the row 45 - k, its subpixels in reverse order
func (record *flowerRecord) halfTurnRows() [flowerSubRows]uint64 {
	var rows [flowerSubRows + 2]uint64
	for petal := 0; petal < flowerShapes; petal++ {
		shape := &record.shapes[petal]
		offset := record.offsets[petal]
		centreX := flowerCentreUnitsX + offset[0]
		centreY := flowerCentreUnitsY + offset[1]
		for row := range rows {
			dy := flowerSampleOffset + flowerSampleStep*(row-2) - centreY
			distance := max(dy, -dy)
			if distance >= shape.half {
				continue
			}
			rows[row] |= flowerSpan(centreX+shape.shearAt(dy), shape.rowWidth(distance))
		}
	}
	var turned [flowerSubRows]uint64
	for row := range turned {
		turned[row] = rows[row+2] | bits.Reverse64(rows[flowerSubRows-1-row])>>(64-flowerSubRows)
	}
	return turned
}

func (record *flowerRecord) coverageOfRows(petals [flowerSubRows]uint64, phase int) morphCoverage {
	square := record.squareRows()
	var coverage morphCoverage
	for y := range coverage {
		for x := range coverage[y] {
			pixel := uint64(1<<morphSubsamples-1) << (morphSubsamples * x)
			petalCount, squareCount := 0, 0
			for subY := 0; subY < morphSubsamples; subY++ {
				row := morphSubsamples*y + subY
				petalCount += bits.OnesCount64(petals[row] & pixel)
				squareCount += bits.OnesCount64(square[row] &^ petals[row] & pixel)
			}
			coverage[y][x] = flowerPixelLevel(record.shade, petalCount, squareCount, x, y, phase)
		}
	}
	return coverage
}

func flowerMismatches(params flowerParams, screen *referenceScreen) (even, odd int) {
	coverage := flowerCoverage(params)
	even = countTileMismatches(coverage, screen, referenceTileOrigin, referenceTileOrigin)
	odd = countTileMismatches(flowerOddCoverage(params), screen, referenceTileOrigin, referenceTileOrigin+morphTileSize)
	return even, odd
}

// The pixels in which the light grey and the ZX frame disagree: the ZX lights a pixel exactly when
// 3 of its 4 subpixels are covered, which is the light grey. The square's own area is left out while it
// fades, because the ZX shows it there as a pattern.
func flowerLightDifference(coverage morphCoverage, params flowerParams, screen *referenceScreen, originY int) int {
	different := 0
	fading := flowerRound(params.shade, 1) < 4
	for y := range coverage {
		for x, level := range coverage[y] {
			if fading && max(math.Abs(float64(x)+0.5-flowerSquareX), math.Abs(float64(y)+0.5-params.squareY)) <= params.square+1.5 {
				continue
			}
			if (level == morphFullCoverage) != screen.lit(referenceTileOrigin+x, originY+y) {
				different++
			}
		}
	}
	return different
}

func flowerLightDifferences(params flowerParams, screen *referenceScreen) (even, odd int) {
	coverage := flowerCoverage(params)
	even = flowerLightDifference(coverage, params, screen, referenceTileOrigin)
	odd = flowerLightDifference(flowerOddCoverage(params), params, screen, referenceTileOrigin+morphTileSize)
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
	if len(args) > 0 && args[0] == "cycles" {
		verifyFlowerCycles()
		verifyFlowerMarchRolls()
		verifyFlowerTurnedPetals()
		return
	}
	if len(args) > 0 && args[0] == "fit" {
		fitFlowerSweeps(&screens)
		return
	}
	reportFlower(&screens)
	writeFlowerComparison(&screens, &flowerSweeps, flowerComparePath)
	writeFlowerImages()
	writeFlowerData()
	reportFlowerMarch(&flowerSweeps)
	writeFlowerMarchComparison(&flowerSweeps)
	writeFlowerSceneImages()
	writeFlowerSceneData()
}

// Every parameter of a state comes from a sweep: from its start state to its end state it changes by a
// constant step a state (in 1/256), before it holds the start value and after it the end value. A
// parameter that changes its pace is a row of sweeps; each sweep starts with its own value.
type flowerSweep struct {
	from, to, start, step int
}

type flowerCurve []flowerSweep

func (curve flowerCurve) at(state int) float64 {
	value := curve[0].start
	for _, sweep := range curve {
		if state >= sweep.from {
			value = sweep.start + sweep.step*(min(state, sweep.to)-sweep.from)
		}
	}
	return float64(value) / flowerSweepGrid
}

var flowerSweeps = struct {
	radial, ring, shade, square, squareY, tangential, turn flowerCurve
}{
	radial:     flowerCurve{{from: 1, to: 6, start: 160, step: 96}, {from: 6, to: 17, start: 640, step: 12}},
	ring:       flowerCurve{{from: 3, to: 16, start: 1376, step: 42}},
	shade:      flowerCurve{{from: 5, to: 17, start: 4 * flowerSweepGrid, step: -85}},
	square:     flowerCurve{{from: 0, to: 9, start: 1536, step: -135}, {from: 9, to: 17, start: 320, step: -36}},
	squareY:    flowerCurve{{from: 0, to: 1, start: 12 * flowerSweepGrid, step: -flowerSweepGrid / 2}, {from: 1, to: 2, start: 11.5 * flowerSweepGrid, step: flowerSweepGrid / 2}},
	tangential: flowerCurve{{from: 2, to: 6, start: 352, step: 24}, {from: 6, to: 17, start: 448, step: 17}},
	turn:       flowerCurve{{from: 0, to: flowerLastState, start: 84 * flowerSweepGrid, step: -3 * flowerSweepGrid}},
}

func flowerParamsAtState(state int) flowerParams {
	return flowerParamsOfSweeps(&flowerSweeps, state)
}

type flowerSweepSet = struct {
	radial, ring, shade, square, squareY, tangential, turn flowerCurve
}

func flowerParamsOfSweeps(sweeps *flowerSweepSet, state int) flowerParams {
	return flowerParams{
		shade:      sweeps.shade.at(state),
		radial:     sweeps.radial.at(state),
		ring:       sweeps.ring.at(state),
		square:     sweeps.square.at(state),
		squareY:    sweeps.squareY.at(state),
		tangential: sweeps.tangential.at(state),
		turn:       sweeps.turn.at(state),
		phase:      state % 2,
	}
}

func writeFlowerComparison(screens *[flowerLastState/2 + 1]referenceScreen, sweeps *flowerSweepSet, path string) {
	const gap = 4
	const rows = 6
	cell := morphTileSize * morphScale
	img := image.NewRGBA(image.Rect(0, 0, (flowerLastState+1)*(cell+gap)-gap, rows*(cell+gap)-gap))
	fillMorphRect(img, img.Rect, color.RGBA{R: 40, G: 60, B: 160, A: 255})
	for state := 0; state <= flowerLastState; state++ {
		left := state * (cell + gap)
		params := flowerParamsOfSweeps(sweeps, state)
		if state == 0 {
			params.radial, params.tangential = 0, 0
		}
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
	file, err := os.Create(path)
	if err != nil {
		fmt.Printf("Warning: failed to create %s: %v\n", path, err)
		return
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		fmt.Printf("Warning: failed to encode %s: %v\n", path, err)
		return
	}
	fmt.Printf("Exported %s (rows: ZX even tile, C64 even tile, even difference, odd difference, C64 odd tile, ZX odd tile; one column per 50 Hz state 0-%d)\n", path, flowerLastState)
}

func writeFlowerImages() {
	var pairs []morphCoverage
	for index := 0; index <= flowerMorphPairs; index++ {
		even, odd := flowerMorphPair(index)
		pairs = append(pairs, even, odd)
	}
	if err := writeTileImages(flowerImagesPath, pairs); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", flowerImagesPath, err)
		return
	}
	fmt.Printf("Exported %s (the even and the odd cells' image of each of the %d even states, then of the square, %d bytes each)\n", flowerImagesPath, flowerMorphPairs, imageBytes)
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

// What the C64 reads for a state, in this order: the square's half-size, its centre row and its shade, then
// for each of the three shapes the half-height, the shear and the width, then the offset from the centre of
// the flower of the first petal of each shape (the other three petals lie opposite). The C64 builds the
// shapes' width tables from the half circle in flower_circle.
func writeFlowerData() {
	var text strings.Builder
	text.WriteString("; Generated by proto (go run -C proto . flower), do not edit\n")
	text.WriteString("; The records of the flower's even states 0-18: square half size, square centre row, shade, three times\n")
	text.WriteString("; (half height, shear, width) and three times (x, y) offsets; lengths in 1/16 pixel, signed ones as bytes\n\n")
	text.WriteString(".segment \"FLOWERDATA\"\n\n.export flower_circle\n.export flower_records\n\nflower_circle:\n")
	text.WriteString(flowerByteLine(flowerCircle[:]))
	text.WriteString("\nflower_records:\n")
	for state := 0; state <= flowerLastState; state += 2 {
		record := newFlowerRecord(flowerStateParams(state))
		if !flowerRecordFits(&record) {
			fmt.Printf("Warning: the record of state %d does not fit its bytes\n", state)
		}
		fmt.Fprintf(&text, "    ; state %d\n", state)
		text.WriteString(flowerByteLine(flowerRecordValues(&record)))
	}
	if err := os.WriteFile(flowerDataPath, []byte(text.String()), 0644); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", flowerDataPath, err)
		return
	}
	fmt.Printf("Exported %s (%d records)\n", flowerDataPath, flowerLastState/2+1)
}

func flowerRecordValues(record *flowerRecord) []int {
	values := []int{record.squareHalf, record.squareY, record.shade}
	for _, shape := range record.shapes {
		values = append(values, shape.half, shape.shear, shape.width)
	}
	for _, offset := range record.offsets {
		values = append(values, offset[0], offset[1])
	}
	return values
}

func flowerByteLine(values []int) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = fmt.Sprintf("$%02X", value&0xFF)
	}
	return "    .byte " + strings.Join(parts, ", ") + "\n"
}

// The C64 keeps half heights below 64, shears and offsets as signed bytes and the rest as unsigned bytes
func flowerRecordFits(record *flowerRecord) bool {
	signed := func(value int) bool { return value >= -128 && value <= 127 }
	fits := record.squareHalf >= 0 && record.squareHalf < 256 && record.squareY >= 0 && record.squareY < 256
	for index, shape := range record.shapes {
		offset := record.offsets[index]
		fits = fits && shape.half <= flowerMaxHalf && signed(shape.shear) && shape.width >= 0 && shape.width <= flowerMaxHalf
		fits = fits && signed(offset[0]) && signed(offset[1])
	}
	return fits
}

func reportFlower(screens *[flowerLastState/2 + 1]referenceScreen) {
	worst := 0
	for index := 1; index < len(screens); index++ {
		state := 2 * index
		params := flowerParamsAtState(state)
		even, odd := flowerMismatches(params, &screens[index])
		lightEven, lightOdd := flowerLightDifferences(params, &screens[index])
		fmt.Printf("state %2d (frame %d): %d mismatches in the even tile, %d in the odd tile; light pixels differ from the ZX lit ones in %d and %d\n", state, flowerFrameOfState(state), even, odd, lightEven, lightOdd)
		worst = max(worst, even, odd)
	}
	fmt.Printf("Flower: worst tile has %d mismatches (limit 8)\n", worst)
}

type flowerKnot struct{ time, value float64 }

// The sweeps the fit moves, in the order of their knots
func fittedFlowerCurves(sweeps *flowerSweepSet) []flowerCurve {
	return []flowerCurve{sweeps.ring, sweeps.radial, sweeps.tangential, sweeps.square}
}

// The points the sweeps of a curve go through: where each sweep starts and where the last one ends
func flowerKnotsOfCurve(curve flowerCurve) []flowerKnot {
	var knots []flowerKnot
	for _, sweep := range curve {
		knots = append(knots, flowerKnot{float64(sweep.from), float64(sweep.start) / flowerSweepGrid})
	}
	last := curve[len(curve)-1]
	return append(knots, flowerKnot{float64(last.to), curve.at(last.to)})
}

func flowerCurveOfKnots(knots []flowerKnot) flowerCurve {
	var curve flowerCurve
	previous := -1000
	times := make([]int, len(knots))
	for index, knot := range knots {
		times[index] = min(max(int(math.Floor(knot.time+0.5)), previous+1, 0), flowerLastState)
		previous = times[index]
	}
	for index := 0; index+1 < len(knots); index++ {
		start := flowerRound(knots[index].value, flowerSweepGrid)
		end := float64(flowerRound(knots[index+1].value, flowerSweepGrid))
		step := int(math.Floor((end-float64(start))/float64(times[index+1]-times[index]) + 0.5))
		curve = append(curve, flowerSweep{from: times[index], to: times[index+1], start: start, step: step})
	}
	return curve
}

func flowerSweepsOfValues(shapes [][]flowerKnot, values []float64) *flowerSweepSet {
	sweeps := flowerSweeps
	curves := make([]flowerCurve, len(shapes))
	offset := 0
	for index, knots := range shapes {
		rebuilt := make([]flowerKnot, len(knots))
		for k := range knots {
			rebuilt[k] = flowerKnot{values[offset], values[offset+1]}
			offset += 2
		}
		curves[index] = flowerCurveOfKnots(rebuilt)
	}
	sweeps.ring, sweeps.radial, sweeps.tangential, sweeps.square = curves[0], curves[1], curves[2], curves[3]
	return &sweeps
}

// What the fit minimises: five for every pixel the frame comparison rejects, one for every pixel
// where the light grey and the ZX lit pixels differ, and a large penalty for breaking the rules of
// the square: exactly the held square at state 0, and state 1 halfway between states 0 and 2
func flowerSweepsCost(sweeps *flowerSweepSet, screens *[flowerLastState/2 + 1]referenceScreen, marchScreens map[int]*referenceScreen) int {
	total := 0
	if flowerRound(sweeps.square.at(0), flowerSweepGrid) != flowerHeldSquare*flowerSweepGrid {
		total += 1000
	}
	if math.Abs(sweeps.square.at(1)-(sweeps.square.at(0)+sweeps.square.at(2))/2) > 1.0/flowerSweepGrid {
		total += 1000
	}
	for index := 1; index < len(screens); index++ {
		params := flowerParamsOfSweeps(sweeps, 2*index)
		even, odd := flowerMismatches(params, &screens[index])
		lightEven, lightOdd := flowerLightDifferences(params, &screens[index])
		total += 5*(even+odd) + lightEven + lightOdd
	}
	for frame, screen := range marchScreens {
		hardEven, hardOdd, lightEven, lightOdd, _ := flowerMarchDifferences(sweeps, frame, screen)
		total += 5*(hardEven+hardOdd) + lightEven + lightOdd
	}
	return total
}

// Starts from the sweeps in flowerSweeps and prints the sweeps to paste into it
func fitFlowerSweeps(screens *[flowerLastState/2 + 1]referenceScreen) {
	var shapes [][]flowerKnot
	var start []float64
	for _, curve := range fittedFlowerCurves(&flowerSweeps) {
		knots := flowerKnotsOfCurve(curve)
		shapes = append(shapes, knots)
		for _, knot := range knots {
			start = append(start, knot.time, knot.value)
		}
	}
	marchScreens := map[int]*referenceScreen{}
	for _, frame := range flowerMarchFitFrames() {
		screen, err := loadReferenceScreen(frame)
		if err != nil {
			fmt.Printf("No ZX reference: %v\n", err)
			return
		}
		marchScreens[frame] = &screen
	}
	cost := func(values []float64) int {
		return flowerSweepsCost(flowerSweepsOfValues(shapes, values), screens, marchScreens)
	}
	bakedCost := flowerSweepsCost(&flowerSweeps, screens, marchScreens)
	best, bestCost := start, cost(start)
	for restart := 0; restart < 30; restart++ {
		candidate := append([]float64(nil), best...)
		if restart > 0 {
			for i := range candidate {
				candidate[i] += float64((restart*7+i*11)%5-2) * 0.125
			}
		}
		values, valueCost := hillClimb(candidate, 8, []float64{1, -1, 2, -2, 4, -4, 8, -8}, cost)
		if valueCost < bestCost {
			best, bestCost = values, valueCost
		}
	}
	fmt.Printf("cost of the sweeps in flowerSweeps: %d, cost of the fitted sweeps: %d\n", bakedCost, bestCost)
	if bestCost >= bakedCost {
		fmt.Println("The fit found nothing better than the sweeps in flowerSweeps")
		return
	}
	fitted := flowerSweepsOfValues(shapes, best)
	writeFlowerComparison(screens, fitted, flowerFitComparePath)
	fmt.Println("Replace in flowerSweeps:")
	fmt.Printf("\tradial:     %s,\n", flowerCurveLiteral(fitted.radial))
	fmt.Printf("\tring:       %s,\n", flowerCurveLiteral(fitted.ring))
	fmt.Printf("\tsquare:     %s,\n", flowerCurveLiteral(fitted.square))
	fmt.Printf("\ttangential: %s,\n", flowerCurveLiteral(fitted.tangential))
}

func flowerCurveLiteral(curve flowerCurve) string {
	text := "flowerCurve{"
	for index, sweep := range curve {
		if index > 0 {
			text += ", "
		}
		text += fmt.Sprintf("{from: %d, to: %d, start: %d, step: %d}", sweep.from, sweep.to, sweep.start, sweep.step)
	}
	return text + "}"
}
