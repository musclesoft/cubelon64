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
	morphTableGrid      = 8
	morphTablePath      = "../c64port/src/morph_table.inc"
	morphTableRestarts  = 60
	morphFullCoverage   = 3
	morphRowBytes       = 6
	morphScale          = 4
	morphSteps          = 11
	morphStates         = 2*(morphSteps-1) + 1
	morphSubsamples     = 2
	morphTileSize       = 24
	morphTileStateBytes = morphTileSize * morphRowBytes
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
	var coverages [morphStates]morphCoverage
	var packed []byte
	for state := range coverages {
		coverages[state] = renderMorphCoverage(morphKiteAtState(state))
		packed = append(packed, packMorphCoverage(coverages[state])...)
	}

	os.MkdirAll("golden", 0755)
	if err := os.WriteFile("golden/morph_tiles.bin", packed, 0644); err != nil {
		fmt.Printf("Warning: failed to write golden/morph_tiles.bin: %v\n", err)
		return
	}
	fmt.Printf("Exported golden/morph_tiles.bin (%d states, %d bytes)\n", morphStates, len(packed))

	var references [morphSteps]morphTile
	for step, frame := range morphFrames {
		reference, err := loadMorphReference(frame)
		if err != nil {
			fmt.Printf("No ZX reference: %v\n", err)
			return
		}
		references[step] = reference
		fmt.Printf("frame %d: %d pixels differ from ZX reference\n", frame, countMorphDiff(fullMorphTile(coverages[2*step]), reference))
	}

	writeMorphComparison(references, coverages)
	exportMorphTable(references)
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
	path := fmt.Sprintf("../emulator/frames/frame_%05d.png", frame)
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

func writeMorphComparison(references [morphSteps]morphTile, coverages [morphStates]morphCoverage) {
	const gap = 4
	cell := morphTileSize * morphScale
	img := image.NewRGBA(image.Rect(0, 0, morphStates*(cell+gap)-gap, 3*(cell+gap)-gap))
	fillMorphRect(img, img.Rect, color.RGBA{R: 40, G: 60, B: 160, A: 255})

	for state := range coverages {
		left := state * (cell + gap)
		drawMorphCoverage(img, left, cell+gap, coverages[state])
		if state%2 == 0 {
			step := state / 2
			drawMorphReference(img, left, 0, references[step])
			drawMorphDiff(img, left, 2*(cell+gap), references[step], fullMorphTile(coverages[state]))
		}
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
	fmt.Println("Exported golden/morph_compare.png (rows: ZX reference, replica, difference; one column per 50 Hz state)")
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

func exportMorphTable(references [morphSteps]morphTile) {
	var rows [morphStates]morphCoordinates
	rows[morphLastState] = morphKiteCoordinates(morphEndKite)
	rng := rand.New(rand.NewSource(11))
	for state := 2; state < morphLastState; state += 2 {
		rows[state] = fitMorphCoordinates(references[state/2], morphKiteCoordinates(morphKiteAtState(state)), rng)
	}
	rows[1] = averageMorphCoordinates(morphKiteCoordinates(morphStartKite), rows[2])
	for state := 3; state < morphLastState; state += 2 {
		rows[state] = averageMorphCoordinates(rows[state-1], rows[state+1])
	}

	totalDiff := 0
	for state := 2; state < morphLastState; state += 2 {
		replica := fullMorphTile(renderMorphCoverage(morphKiteFromCoordinates(rows[state])))
		totalDiff += countMorphDiff(replica, references[state/2])
	}
	fmt.Printf("Morph table: %d pixels differ from the ZX reference in total\n", totalDiff)

	var out strings.Builder
	out.WriteString("; Generated by proto (go run . morph), do not edit\n")
	out.WriteString("; Morph kite vertices for 50 Hz states 1-20 (state 0 is the marching square)\n")
	out.WriteString("; Signed bytes in 1/8 pixel units: outer tip a, inner tip c, side vertex p, side vertex q\n")
	out.WriteString("; Kite: A=(a,a) B=(p,q) C=(c,c) D=(q,p), mirrored into all four tile corners\n\n")
	out.WriteString(".segment \"RODATA\"\n\n.export morph_kite_table\nmorph_kite_table:\n")
	for state := 1; state < morphStates; state++ {
		row := rows[state]
		fmt.Fprintf(&out, "    .byte $%02X, $%02X, $%02X, $%02X  ; state %2d: a=%.3f c=%.3f p=%.3f q=%.3f\n",
			morphTableByte(row[0]), morphTableByte(row[1]), morphTableByte(row[2]), morphTableByte(row[3]),
			state, row[0], row[1], row[2], row[3])
	}
	if err := os.WriteFile(morphTablePath, []byte(out.String()), 0644); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", morphTablePath, err)
		return
	}
	fmt.Printf("Exported %s (%d bytes of data)\n", morphTablePath, 4*(morphStates-1))
}

func morphKiteCoordinates(kite morphKite) morphCoordinates {
	return morphCoordinates{kite[0].x, kite[2].x, kite[1].x, kite[1].y}
}

func morphKiteFromCoordinates(c morphCoordinates) morphKite {
	return morphKite{{c[0], c[0]}, {c[2], c[3]}, {c[1], c[1]}, {c[3], c[2]}}
}

func snapMorphCoordinates(c morphCoordinates) morphCoordinates {
	for i := range c {
		c[i] = math.Round(c[i]*morphTableGrid) / morphTableGrid
	}
	return c
}

func averageMorphCoordinates(a, b morphCoordinates) morphCoordinates {
	var c morphCoordinates
	for i := range c {
		c[i] = (a[i] + b[i]) / 2
	}
	return snapMorphCoordinates(c)
}

func morphTableByte(value float64) byte {
	return byte(int8(math.Round(value * morphTableGrid)))
}

func fitMorphCoordinates(reference morphTile, formula morphCoordinates, rng *rand.Rand) morphCoordinates {
	best, bestDiff := descendMorphCoordinates(reference, snapMorphCoordinates(formula))
	for restart := 0; restart < morphTableRestarts && bestDiff > 0; restart++ {
		start := formula
		for i := range start {
			start[i] += float64(rng.Intn(9)-4) / morphTableGrid
		}
		if candidate, diff := descendMorphCoordinates(reference, snapMorphCoordinates(start)); diff < bestDiff {
			best, bestDiff = candidate, diff
		}
	}
	return best
}

func descendMorphCoordinates(reference morphTile, start morphCoordinates) (morphCoordinates, int) {
	current := start
	currentDiff := morphCoordinatesDiff(current, reference)
	for improved := true; improved; {
		improved = false
		for i := range current {
			for _, delta := range []float64{1, -1, 2, -2} {
				candidate := current
				candidate[i] += delta / morphTableGrid
				if diff := morphCoordinatesDiff(candidate, reference); diff < currentDiff {
					current, currentDiff, improved = candidate, diff, true
				}
			}
		}
	}
	return current, currentDiff
}

func morphCoordinatesDiff(c morphCoordinates, reference morphTile) int {
	return countMorphDiff(fullMorphTile(renderMorphCoverage(morphKiteFromCoordinates(c))), reference)
}
