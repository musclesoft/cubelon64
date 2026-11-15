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
	flowerPetals         = 6
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
	flowerComparePath    = "golden/flower_compare.png"
	flowerImagesPath     = "golden/flower_images.bin"
	flowerSetupPath      = "golden/flower_setup.bin"
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
	return flowerSetupCoverage(params, newFlowerSetup(params))
}

func flowerSetupCoverage(params flowerParams, setup *flowerSetup) morphCoverage {
	var coverage morphCoverage
	for y := range coverage {
		for x := range coverage[y] {
			petals, squareOnly := 0, 0
			for subY := 0; subY < morphSubsamples; subY++ {
				for subX := 0; subX < morphSubsamples; subX++ {
					px := float64(x) + (float64(subX)+0.5)/morphSubsamples
					py := float64(y) + (float64(subY)+0.5)/morphSubsamples
					switch {
					case setup.covers(int(px*flowerChordGrid), int(py*flowerChordGrid)):
						petals++
					case inFlowerSquare(params, int(px*flowerChordGrid), int(py*flowerChordGrid)):
						squareOnly++
					}
				}
			}
			coverage[y][x] = flowerPixelLevel(flowerRound(params.shade, 1), petals, squareOnly, x, y, params.phase)
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

func inFlowerSquare(params flowerParams, x, y int) bool {
	half := flowerRound(params.square, flowerChordGrid)
	dx, dy := x-flowerRound(flowerSquareX, flowerChordGrid), y-flowerRound(params.squareY, flowerChordGrid)
	return max(dx, -dx) <= half && max(dy, -dy) <= half
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
// rows, so a state needs three tables; they are built before the state is drawn. All of it is
// integers: lengths in 1/8 pixel, the shear in 1/32 pixel per pixel.
type flowerShape struct {
	half, shear, width int
	widths             [flowerChordTableSize + 1]int
}

type flowerSetup struct {
	any    bool
	petals [flowerPetals][2]int
	shapes [flowerPetals / 2]flowerShape
}

func newFlowerSetup(params flowerParams) *flowerSetup {
	setup := &flowerSetup{}
	if params.radial <= 0 || params.tangential <= 0 {
		return setup
	}
	setup.any = true
	for index := range setup.shapes {
		setup.shapes[index] = newFlowerShape(params, flowerPetalAngle(params, index))
	}
	for petal := range setup.petals {
		angle := flowerPetalAngle(params, petal)
		setup.petals[petal] = [2]int{
			flowerRound(flowerCentreX+params.ring*math.Cos(angle), flowerChordGrid),
			flowerRound(flowerCentreY-params.ring*math.Sin(angle), flowerChordGrid),
		}
	}
	return setup
}

func flowerPetalAngle(params flowerParams, petal int) float64 {
	return (params.turn + 360.0*float64(petal)/flowerPetals) * math.Pi / 180
}

func newFlowerShape(params flowerParams, angle float64) flowerShape {
	sine, cosine := math.Sin(angle), math.Cos(angle)
	radial, tangential := params.radial, params.tangential
	half := math.Sqrt(radial*radial*sine*sine + tangential*tangential*cosine*cosine)
	shape := flowerShape{
		half:  flowerRound(half, flowerChordGrid),
		shear: flowerRound(-(radial*radial-tangential*tangential)*sine*cosine/(half*half), flowerShearGrid),
		width: flowerRound(radial*tangential/half, flowerChordGrid),
	}
	shape.fillWidths()
	return shape
}

func (shape *flowerShape) fillWidths() {
	for index := range shape.widths {
		circle := flowerRound(math.Sqrt(1-math.Pow((float64(index)+0.5)/flowerChordTableSize, 2)), 255)
		shape.widths[index] = (shape.width*circle + 127) / 255
	}
}

// Whether the sample at (x, y), in 1/8 pixel, is inside a petal
func (setup *flowerSetup) covers(x, y int) bool {
	if !setup.any {
		return false
	}
	for petal, centre := range setup.petals {
		shape := &setup.shapes[petal%len(setup.shapes)]
		dy := y - centre[1]
		distance := max(dy, -dy)
		if distance >= shape.half {
			continue
		}
		index := distance * flowerChordTableSize / shape.half
		shear := (shape.shear*dy + flowerShearGrid/2) >> flowerShearShift
		dx := x - centre[0] - shear
		if max(dx, -dx) <= shape.widths[index] {
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
	if len(args) > 0 && args[0] == "fit" {
		fitFlowerSweeps(&screens)
		return
	}
	reportFlower(&screens)
	writeFlowerComparison(&screens)
	writeFlowerImages()
	writeFlowerSetups()
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
	radial:     flowerCurve{{from: 2, to: 5, start: 192, step: 139}, {from: 5, to: 21, start: 608, step: 14}},
	ring:       flowerCurve{{from: 1, to: 17, start: 1312, step: 40}},
	shade:      flowerCurve{{from: 5, to: 17, start: 4 * flowerSweepGrid, step: -85}},
	square:     flowerCurve{{from: 0, to: 9, start: 1536, step: -132}, {from: 9, to: 17, start: 352, step: -44}},
	squareY:    flowerCurve{{from: 0, to: 1, start: 12 * flowerSweepGrid, step: -flowerSweepGrid / 2}, {from: 1, to: 2, start: 11.5 * flowerSweepGrid, step: flowerSweepGrid / 2}},
	tangential: flowerCurve{{from: -1, to: 7, start: 192, step: 40}, {from: 7, to: 18, start: 512, step: 9}},
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

func writeFlowerComparison(screens *[flowerLastState/2 + 1]referenceScreen) {
	const gap = 4
	const rows = 6
	cell := morphTileSize * morphScale
	img := image.NewRGBA(image.Rect(0, 0, (flowerLastState+1)*(cell+gap)-gap, rows*(cell+gap)-gap))
	fillMorphRect(img, img.Rect, color.RGBA{R: 40, G: 60, B: 160, A: 255})
	for state := 0; state <= flowerLastState; state++ {
		left := state * (cell + gap)
		params := flowerParamsAtState(state)
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

func writeFlowerImages() {
	even := make([]morphCoverage, flowerLastState)
	odd := make([]morphCoverage, flowerLastState)
	for state := 1; state <= flowerLastState; state++ {
		params := flowerParamsAtState(state)
		even[state-1], odd[state-1] = flowerCoverage(params), flowerOddCoverage(params)
	}
	if err := writeTileImages(flowerImagesPath, even, odd); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", flowerImagesPath, err)
		return
	}
	fmt.Printf("Exported %s (the even and then the odd cells' %d states, %d bytes each)\n", flowerImagesPath, flowerLastState, imageBytes)
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

// What the C64 reads per state to build the three width tables and draw the six petals: for each
// of the three shapes the half-height, the shear and the width, then the centre of each petal
func writeFlowerSetups() {
	var data []byte
	for state := 1; state <= flowerLastState; state++ {
		setup := newFlowerSetup(flowerParamsAtState(state))
		for _, shape := range setup.shapes {
			data = append(data, byte(shape.half), byte(int8(shape.shear)), byte(shape.width))
		}
		for _, centre := range setup.petals {
			data = append(data, byte(centre[0]), byte(centre[1]))
		}
	}
	if err := os.MkdirAll("golden", 0755); err != nil {
		fmt.Printf("Warning: failed to create golden: %v\n", err)
		return
	}
	if err := os.WriteFile(flowerSetupPath, data, 0644); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", flowerSetupPath, err)
		return
	}
	fmt.Printf("Exported %s (%d states, %d bytes each)\n", flowerSetupPath, flowerLastState, len(data)/flowerLastState)
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

var flowerKnotStart = [][]flowerKnot{
	{{0, 5.25}, {18, 7.5}},
	{{-2, 0}, {5, 2.4}, {18, 3}},
	{{-2, 0}, {8, 2.4}, {18, 2.4}},
	{{0, 6}, {10, 0.875}, {17, 0}},
}

func flowerCurveOfKnots(knots []flowerKnot) flowerCurve {
	var curve flowerCurve
	previous := -1000
	times := make([]int, len(knots))
	for index, knot := range knots {
		times[index] = max(int(math.Floor(knot.time+0.5)), previous+1)
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

func flowerSweepsOfValues(values []float64) *flowerSweepSet {
	sweeps := flowerSweeps
	curves := make([]flowerCurve, len(flowerKnotStart))
	offset := 0
	for index, knots := range flowerKnotStart {
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

func fitFlowerSweeps(screens *[flowerLastState/2 + 1]referenceScreen) {
	var start []float64
	for _, knots := range flowerKnotStart {
		for _, knot := range knots {
			start = append(start, knot.time, knot.value)
		}
	}
	cost := func(values []float64) int {
		sweeps := flowerSweepsOfValues(values)
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
		return total
	}
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
	sweeps := flowerSweepsOfValues(best)
	fmt.Printf("total mismatches %d\n", bestCost)
	for name, curve := range map[string]flowerCurve{"ring": sweeps.ring, "radial": sweeps.radial, "tangential": sweeps.tangential, "square": sweeps.square} {
		fmt.Printf("%s: %v\n", name, curve)
	}
}
