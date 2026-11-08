package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"os"
	"strings"
)

const (
	morphAsymmetryPenalty = 100
	morphOuterTipGrid     = 32
	morphTableGrid        = 64
	morphTablePath        = "../c64port/src/morph_table.inc"
	morphTableRestarts    = 300
	morphFullCoverage     = 3
	morphRowBytes         = 6
	morphScale            = 4
	morphSteps            = 11
	morphStates           = 2*(morphSteps-1) + 1
	morphSubsamples       = 2
	morphTileSize         = 24
	morphTileStateBytes   = morphTileSize * morphRowBytes
)

type morphVertex struct{ x, y float64 }

type morphKite [4]morphVertex

type morphCoverage [morphTileSize][morphTileSize]int

type morphTile [morphTileSize][morphTileSize]bool

var morphFrames = [morphSteps]int{779, 801, 803, 805, 807, 809, 811, 813, 815, 817, 819}

var (
	morphStartKite = morphKite{{6, 6}, {12, 6}, {12, 12}, {6, 12}}
	morphEndKite   = morphKite{{0, 0}, {8, 0}, {8, 8}, {0, 8}}
)

type morphCurve struct{ constant, linear, quadratic float64 }

var (
	morphOuterTip = morphCurve{3.275206, -0.487419, -0.016092}
	morphInnerTip = morphCurve{12.503447, -0.335882, 0}
	morphSideNear = morphCurve{11.823416, -0.487872, 0}
	morphSideFar  = morphCurve{6.095170, -0.622873, -0.011037}
)

const morphLastState = morphStates - 1

func runMorph() {
	var references [morphSteps]morphTile
	for step, frame := range morphFrames {
		reference, err := loadMorphReference(frame)
		if err != nil {
			fmt.Printf("No ZX reference: %v\n", err)
			return
		}
		references[step] = reference
	}

	formulaDiff := 0
	for step, reference := range references {
		formulaDiff += countMorphDiff(fullMorphTile(renderMorphCoverage(morphKiteAtState(2*step))), reference)
	}
	fmt.Printf("Formula: %d pixels differ from the ZX reference in total\n", formulaDiff)

	rows := fitMorphTable()
	var coverages [morphStates]morphCoverage
	var packed []byte
	coverages[0] = renderMorphDDA(morphKiteCoordinates(morphStartKite))
	for state := 1; state < morphStates; state++ {
		coverages[state] = renderMorphDDA(rows[state])
	}
	tableDiff := 0
	for step, frame := range morphFrames {
		diff := countMorphDiff(fullMorphTile(coverages[2*step]), references[step])
		tableDiff += diff
		fmt.Printf("frame %d: %d pixels differ from ZX reference\n", frame, diff)
	}
	fmt.Printf("Table rendered by the cube rasterizer: %d pixels differ from the ZX reference in total\n", tableDiff)
	idealMismatch := 0
	for state, coverage := range coverages {
		idealMismatch += morphLevelMismatch(coverage, renderMorphCoverage(morphKiteAtState(state)))
	}
	fmt.Printf("Table versus formula render: %d pixels have a different level over all %d states\n", idealMismatch, morphStates)
	asymmetry := 0
	for _, coverage := range coverages {
		packed = append(packed, packMorphCoverage(coverage)...)
		asymmetry += morphAsymmetry(coverage)
	}
	fmt.Printf("Table asymmetry: %d pixels differ from their transpose over all %d states\n", asymmetry, morphStates)
	duplicates := 0
	for state := 1; state < morphStates; state++ {
		if coverages[state] == coverages[state-1] {
			duplicates++
		}
	}
	levels := ""
	for _, coverage := range coverages {
		levels += fmt.Sprintf("%d ", coverage[morphTileSize/2][morphTileSize/2])
	}
	fmt.Printf("Table centre levels per state: %s\n", levels)
	fmt.Printf("Table duplicates: %d consecutive states render identically\n", duplicates)

	os.MkdirAll("golden", 0755)
	if err := os.WriteFile("golden/morph_tiles.bin", packed, 0644); err != nil {
		fmt.Printf("Warning: failed to write golden/morph_tiles.bin: %v\n", err)
		return
	}
	fmt.Printf("Exported golden/morph_tiles.bin (%d states, %d bytes)\n", morphStates, len(packed))

	var formulas [morphStates]morphCoverage
	for state := range formulas {
		formulas[state] = renderMorphCoverage(morphKiteAtState(state))
	}
	writeMorphComparison(references, formulas, coverages)
	writeMorphTable(rows)
}

func morphKiteAtState(state int) morphKite {
	switch state {
	case 0:
		return morphStartKite
	case morphLastState:
		return morphEndKite
	case 1:
		return averageMorphKites(morphStartKite, morphKiteAtState(2))
	case morphLastState - 1:
		return averageMorphKites(morphKiteAtState(morphLastState-2), morphEndKite)
	}
	t := float64(state)/2 - 1
	outer := morphOuterTip.at(t)
	inner := morphInnerTip.at(t)
	near := morphSideNear.at(t)
	far := morphSideFar.at(t)
	return morphKite{{outer, outer}, {near, far}, {inner, inner}, {far, near}}
}

func (curve morphCurve) at(t float64) float64 {
	return curve.constant + curve.linear*t + curve.quadratic*t*t
}

func averageMorphKites(a, b morphKite) morphKite {
	var kite morphKite
	for i := range kite {
		kite[i] = morphVertex{x: (a[i].x + b[i].x) / 2, y: (a[i].y + b[i].y) / 2}
	}
	return kite
}

func renderMorphCoverage(kite morphKite) morphCoverage {
	var polygons [4]morphKite
	for corner := range polygons {
		for i, v := range kite {
			polygons[corner][i] = mirrorMorphVertex(v, corner)
		}
	}

	var coverage morphCoverage
	for y := range coverage {
		for x := range coverage[y] {
			for subY := 0; subY < morphSubsamples; subY++ {
				for subX := 0; subX < morphSubsamples; subX++ {
					px := float64(x) + (float64(subX)+0.5)/morphSubsamples
					py := float64(y) + (float64(subY)+0.5)/morphSubsamples
					if coveredByAnyKite(polygons, px, py) {
						coverage[y][x]++
					}
				}
			}
		}
	}
	return coverage
}

func mirrorMorphVertex(v morphVertex, corner int) morphVertex {
	if corner&1 != 0 {
		v.x = morphTileSize - v.x
	}
	if corner&2 != 0 {
		v.y = morphTileSize - v.y
	}
	return v
}

func coveredByAnyKite(polygons [4]morphKite, px, py float64) bool {
	for _, polygon := range polygons {
		for wrapX := -morphTileSize; wrapX <= morphTileSize; wrapX += morphTileSize {
			for wrapY := -morphTileSize; wrapY <= morphTileSize; wrapY += morphTileSize {
				if insideMorphPolygon(polygon, px-float64(wrapX), py-float64(wrapY)) {
					return true
				}
			}
		}
	}
	return false
}

func insideMorphPolygon(polygon morphKite, px, py float64) bool {
	inside := false
	previous := polygon[3]
	for _, current := range polygon {
		if (current.y > py) != (previous.y > py) &&
			px < (previous.x-current.x)*(py-current.y)/(previous.y-current.y)+current.x {
			inside = !inside
		}
		previous = current
	}
	return inside
}

func packMorphCoverage(coverage morphCoverage) []byte {
	packed := make([]byte, 0, morphTileStateBytes)
	for _, row := range coverage {
		var full, medium [morphTileSize / 8]byte
		for x, count := range row {
			bit := byte(0x80) >> (x % 8)
			if count == 0 {
				full[x/8] |= bit
			}
			if count < morphFullCoverage {
				medium[x/8] |= bit
			}
		}
		packed = append(packed, full[:]...)
		packed = append(packed, medium[:]...)
	}
	return packed
}

func fullMorphTile(coverage morphCoverage) morphTile {
	var tile morphTile
	for y := range coverage {
		for x, count := range coverage[y] {
			tile[y][x] = count >= morphFullCoverage
		}
	}
	return tile
}

func loadMorphReference(frame int) (morphTile, error) {
	var tile morphTile
	path := fmt.Sprintf("../emulator/frames/frame_%05d_zx.png", frame)
	file, err := os.Open(path)
	if err != nil {
		return tile, err
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		return tile, err
	}
	for y := range tile {
		for x := range tile[y] {
			r, _, _, _ := img.At(x, y).RGBA()
			tile[y][x] = r > 0x8000
		}
	}
	return tile, nil
}

func countMorphDiff(a, b morphTile) int {
	diff := 0
	for y := range a {
		for x := range a[y] {
			if a[y][x] != b[y][x] {
				diff++
			}
		}
	}
	return diff
}

func writeMorphComparison(references [morphSteps]morphTile, formulas, coverages [morphStates]morphCoverage) {
	const gap = 4
	cell := morphTileSize * morphScale
	img := image.NewRGBA(image.Rect(0, 0, morphStates*(cell+gap)-gap, 6*(cell+gap)-gap))
	fillMorphRect(img, img.Rect, color.RGBA{R: 40, G: 60, B: 160, A: 255})

	for state := range coverages {
		left := state * (cell + gap)
		drawMorphCoverage(img, left, cell+gap, formulas[state])
		drawMorphCoverage(img, left, 3*(cell+gap), coverages[state])
		if state%2 == 0 {
			step := state / 2
			drawMorphReference(img, left, 0, references[step])
			drawMorphDiff(img, left, 2*(cell+gap), references[step], fullMorphTile(formulas[state]))
			drawMorphDiff(img, left, 4*(cell+gap), references[step], fullMorphTile(coverages[state]))
		}
		drawMorphLevelDiff(img, left, 5*(cell+gap), formulas[state], coverages[state])
	}

	file, err := os.Create("golden/morph_compare.png")
	if err != nil {
		fmt.Printf("Warning: failed to create golden/morph_compare.png: %v\n", err)
		return
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		fmt.Printf("Warning: failed to encode golden/morph_compare.png: %v\n", err)
		return
	}
	fmt.Println("Exported golden/morph_compare.png (rows: ZX reference, formula, formula difference, table via cube rasterizer, table difference, table versus formula; one column per 50 Hz state)")
}

func drawMorphReference(img *image.RGBA, left, top int, tile morphTile) {
	for y := range tile {
		for x := range tile[y] {
			shade := color.RGBA{A: 255}
			if tile[y][x] {
				shade = color.RGBA{R: 198, G: 198, B: 198, A: 255}
			}
			fillMorphRect(img, morphPixelRect(left, top, x, y), shade)
		}
	}
}

func drawMorphCoverage(img *image.RGBA, left, top int, coverage morphCoverage) {
	for y := range coverage {
		for x, count := range coverage[y] {
			shade := color.RGBA{A: 255}
			switch {
			case count >= morphFullCoverage:
				shade = color.RGBA{R: 198, G: 198, B: 198, A: 255}
			case count > 0:
				shade = color.RGBA{R: 95, G: 95, B: 95, A: 255}
			}
			fillMorphRect(img, morphPixelRect(left, top, x, y), shade)
		}
	}
}

func drawMorphDiff(img *image.RGBA, left, top int, reference, replica morphTile) {
	for y := range reference {
		for x := range reference[y] {
			shade := color.RGBA{A: 255}
			switch {
			case reference[y][x] && !replica[y][x]:
				shade = color.RGBA{R: 255, A: 255}
			case !reference[y][x] && replica[y][x]:
				shade = color.RGBA{G: 255, B: 255, A: 255}
			case reference[y][x]:
				shade = color.RGBA{R: 64, G: 64, B: 64, A: 255}
			}
			fillMorphRect(img, morphPixelRect(left, top, x, y), shade)
		}
	}
}

func drawMorphLevelDiff(img *image.RGBA, left, top int, ideal, replica morphCoverage) {
	for y := range ideal {
		for x := range ideal[y] {
			want, got := morphCoverageLevel(ideal[y][x]), replica[y][x]
			shade := color.RGBA{A: 255}
			switch {
			case want > got:
				shade = color.RGBA{R: 255, A: 255}
			case want < got:
				shade = color.RGBA{G: 255, B: 255, A: 255}
			case want > 0:
				shade = color.RGBA{R: 64, G: 64, B: 64, A: 255}
			}
			fillMorphRect(img, morphPixelRect(left, top, x, y), shade)
		}
	}
}

func morphPixelRect(left, top, x, y int) image.Rectangle {
	origin := image.Pt(left+x*morphScale, top+y*morphScale)
	return image.Rectangle{Min: origin, Max: origin.Add(image.Pt(morphScale, morphScale))}
}

func fillMorphRect(img *image.RGBA, rect image.Rectangle, shade color.RGBA) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.SetRGBA(x, y, shade)
		}
	}
}

type morphCoordinates [4]float64

func fitMorphTable() [morphStates]morphCoordinates {
	var rows [morphStates]morphCoordinates
	rows[0] = morphKiteCoordinates(morphStartKite)
	rows[morphLastState] = morphKiteCoordinates(morphEndKite)
	rng := rand.New(rand.NewSource(11))
	for state := 1; state < morphLastState; state++ {
		formula := morphKiteCoordinates(morphKiteAtState(state))
		rows[state] = fitMorphCoordinates(renderMorphCoverage(morphKiteAtState(state)), formula, rng)
	}
	return rows
}

func fitMorphCoordinates(ideal morphCoverage, formula morphCoordinates, rng *rand.Rand) morphCoordinates {
	best, bestCost := descendMorphCoordinates(ideal, snapMorphCoordinates(formula))
	for restart := 0; restart < morphTableRestarts && bestCost > 0; restart++ {
		start := formula
		for i := range start {
			start[i] += float64(rng.Intn(33)-16) / morphTableGrid
		}
		if candidate, cost := descendMorphCoordinates(ideal, snapMorphCoordinates(start)); cost < bestCost {
			best, bestCost = candidate, cost
		}
	}
	return best
}

func descendMorphCoordinates(ideal morphCoverage, start morphCoordinates) (morphCoordinates, int) {
	current := start
	currentCost := morphCoordinatesCost(current, ideal)
	for improved := true; improved; {
		improved = false
		for i := range current {
			for _, delta := range []float64{1, -1, 2, -2, 4, -4, 8, -8, 16, -16} {
				candidate := current
				candidate[i] += delta / morphTableGrid
				candidate = snapMorphCoordinates(candidate)
				if cost := morphCoordinatesCost(candidate, ideal); cost < currentCost {
					current, currentCost, improved = candidate, cost, true
				}
			}
		}
	}
	return current, currentCost
}

func morphCoordinatesCost(c morphCoordinates, ideal morphCoverage) int {
	coverage := renderMorphDDA(c)
	return morphLevelMismatch(coverage, ideal) + morphAsymmetryPenalty*morphAsymmetry(coverage)
}

func morphLevelMismatch(coverage, ideal morphCoverage) int {
	mismatch := 0
	for y := range coverage {
		for x := range coverage[y] {
			if coverage[y][x] != morphCoverageLevel(ideal[y][x]) {
				mismatch++
			}
		}
	}
	return mismatch
}

func morphCoverageLevel(count int) int {
	switch {
	case count >= morphFullCoverage:
		return morphFullCoverage
	case count > 0:
		return 1
	}
	return 0
}

type morphPackedTable struct {
	bytes                                   []byte
	outerBase, innerBase, nearBase, farBase int
}

func packMorphTable(rows [morphStates]morphCoordinates) (morphPackedTable, bool) {
	var units [morphStates][4]int
	low := [4]int{1 << 30, 1 << 30, 1 << 30, 1 << 30}
	high := [4]int{-(1 << 30), -(1 << 30), -(1 << 30), -(1 << 30)}
	for state := 1; state < morphStates; state++ {
		outer := int(math.Round(rows[state][0] * morphOuterTipGrid))
		units[state][0] = outer
		for i := 1; i < 4; i++ {
			units[state][i] = int(math.Round((rows[state][i] - rows[state][0]) * morphTableGrid))
		}
		for i := range low {
			low[i], high[i] = min(low[i], units[state][i]), max(high[i], units[state][i])
		}
	}

	table := morphPackedTable{outerBase: low[0], innerBase: low[1], nearBase: low[2], farBase: low[3]}
	fits := true
	for i := range low {
		if high[i]-low[i] > 255 {
			fmt.Printf("Warning: morph table field %d spans %d units, more than one byte\n", i, high[i]-low[i])
			fits = false
		}
	}
	for state := 1; state < morphStates; state++ {
		for i := range low {
			table.bytes = append(table.bytes, byte(units[state][i]-low[i]))
		}
	}
	return table, fits
}

func unpackMorphTable(table morphPackedTable, state int) morphCoordinates {
	record := table.bytes[(state-1)*4 : state*4]
	outer := float64(int(record[0])+table.outerBase) / morphOuterTipGrid
	var c morphCoordinates
	c[0] = outer
	c[1] = outer + float64(int(record[1])+table.innerBase)/morphTableGrid
	c[2] = outer + float64(int(record[2])+table.nearBase)/morphTableGrid
	c[3] = outer + float64(int(record[3])+table.farBase)/morphTableGrid
	return c
}

func writeMorphTable(rows [morphStates]morphCoordinates) {
	table, fits := packMorphTable(rows)
	if !fits {
		return
	}
	for state := 1; state < morphStates; state++ {
		if unpackMorphTable(table, state) != rows[state] {
			fmt.Printf("Warning: morph table state %d does not unpack to its fitted coordinates\n", state)
			return
		}
	}

	var out strings.Builder
	out.WriteString("; Generated by proto (go run . morph), do not edit\n")
	out.WriteString("; Morph kite vertices for 50 Hz states 1-20 (state 0 is the marching square), four 20-byte arrays\n")
	out.WriteString("; Index every array with X = state - 1, so stepping through the states is INX or DEX\n")
	out.WriteString("; Kite: A=(a,a) B=(p,q) C=(c,c) D=(q,p), mirrored into all four tile corners\n")
	out.WriteString("; Line comments show the decoded coordinate in pixels: a, c, p or q\n")
	out.WriteString("; Decode to the rasterizer's 8.8 words (48x48 supersample frame, +4 pixel shift included):\n")
	out.WriteString(";   a word = morph_outer_table,X * 16 + MORPH_OUTER_BASE\n")
	out.WriteString(";   c word = a word + morph_inner_table,X * 8 + MORPH_INNER_BASE\n")
	out.WriteString(";   p word = a word + morph_near_table,X * 8 + MORPH_NEAR_BASE\n")
	out.WriteString(";   q word = a word + morph_far_table,X * 8 + MORPH_FAR_BASE\n")
	out.WriteString("; Use a word as is for X; subtract $0080 from it for Y (the rasterizer samples scanlines at their top edge)\n\n")
	fmt.Fprintf(&out, "MORPH_OUTER_BASE = %d\n", table.outerBase*16+morphDDAShift*512)
	fmt.Fprintf(&out, "MORPH_INNER_BASE = %d\n", table.innerBase*8)
	fmt.Fprintf(&out, "MORPH_NEAR_BASE = %d\n", table.nearBase*8)
	fmt.Fprintf(&out, "MORPH_FAR_BASE = %d\n\n", table.farBase*8)
	out.WriteString(".segment \"RODATA\"\n")
	for field, name := range []string{"outer", "inner", "near", "far"} {
		fmt.Fprintf(&out, "\n.export morph_%s_table\nmorph_%s_table:\n", name, name)
		for state := 1; state < morphStates; state++ {
			fmt.Fprintf(&out, "    .byte $%02X  ; state %2d: %.4f\n", table.bytes[(state-1)*4+field], state, rows[state][field])
		}
	}
	if err := os.WriteFile(morphTablePath, []byte(out.String()), 0644); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", morphTablePath, err)
		return
	}
	fmt.Printf("Exported %s (%d bytes of data)\n", morphTablePath, len(table.bytes))
}

func morphKiteCoordinates(kite morphKite) morphCoordinates {
	return morphCoordinates{kite[0].x, kite[2].x, kite[1].x, kite[1].y}
}

func morphKiteFromCoordinates(c morphCoordinates) morphKite {
	return morphKite{{c[0], c[0]}, {c[2], c[3]}, {c[1], c[1]}, {c[3], c[2]}}
}

func snapMorphCoordinates(c morphCoordinates) morphCoordinates {
	for i := range c {
		grid := float64(morphTableGrid)
		if i == 0 {
			grid = morphOuterTipGrid
		}
		c[i] = math.Round(c[i]*grid) / grid
	}
	return c
}

func morphAsymmetry(coverage morphCoverage) int {
	asymmetry := 0
	for y := range coverage {
		for x := range coverage[y] {
			if coverage[y][x] != coverage[x][y] {
				asymmetry++
			}
		}
	}
	return asymmetry
}

func morphTableWord(value float64) uint16 {
	return fixed88(value + morphDDAShift)
}
