package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
)

const (
	splitFieldCount  = 4
	splitFitGrid     = 64
	splitSteps       = morphSteps
	splitStates      = morphStates
	splitTableGrid   = 512
	splitTablePath   = "../c64port/src/split_table.inc"
	splitImagesPath  = "golden/split_images.bin"
	splitComparePath = "golden/split_compare.png"

	splitReferenceWeight = 4
)

var splitFrames = [splitSteps]int{819, 841, 843, 845, 847, 849, 851, 853, 855, 857, 859}

type splitParams [splitFieldCount]float64

const (
	splitWaist = iota
	splitWide
	splitWideY
	splitTip
)

var splitFieldNames = [splitFieldCount]string{"waist", "wide", "wide_y", "tip"}

var (
	splitSquare    = splitParams{8, 8, 8, 8}
	splitPathStart = splitParams{8.35, 8.11, 8.1825, 8.27}
	splitPathEnd   = splitParams{1.9, 6.7, 10.3, 13.7}
)

func runSplit() {
	var references [splitSteps]morphTile
	var screens [splitSteps]referenceScreen
	for step, frame := range splitFrames {
		reference, err := loadMorphReference(frame)
		if err != nil {
			fmt.Printf("No ZX reference: %v\n", err)
			return
		}
		if !isMirroredCheckerboard(frame) {
			fmt.Printf("Warning: frame %d is not a checkerboard of the tile and its mirror image\n", frame)
		}
		references[step] = reference
		screens[step], _ = loadReferenceScreen(frame)
	}

	var formulas [splitStates]splitParams
	for state := range formulas {
		formulas[state] = splitParamsAtState(state)
	}
	formulaDiff := 0
	for step := 1; step < splitSteps; step++ {
		formulaDiff += countMorphDiff(fullMorphTile(renderSplitIdeal(formulas[2*step])), references[step])
	}
	fmt.Printf("Formula: %d pixels differ from the ZX reference in total\n", formulaDiff)

	var tables [splitStates]splitParams
	var coverages [splitStates]morphCoverage
	tableDiff, mismatch := 0, 0
	for state := range tables {
		ideal := renderSplitIdeal(formulas[state])
		var screen *referenceScreen
		if state%2 == 0 {
			screen = &screens[state/2]
		}
		tables[state] = fitSplitTable(formulas[state], ideal, screen)
		coverages[state] = renderSplitDDA(tables[state])
		mismatch += morphLevelMismatch(coverages[state], ideal)
		if state%2 == 0 {
			diff := countMorphDiff(fullMorphTile(coverages[state]), references[state/2])
			tableDiff += diff
			fmt.Printf("frame %d: %d pixels differ from ZX reference\n", splitFrames[state/2], diff)
		}
	}
	fmt.Printf("Table rendered by the cube rasterizer: %d pixels differ from the ZX reference in total\n", tableDiff)
	fmt.Printf("Table versus formula render: %d pixels have a different level over all %d states\n", mismatch, splitStates)

	var mirrored []morphCoverage
	for _, coverage := range coverages[1:] {
		mirrored = append(mirrored, mirrorCoverage(coverage))
	}
	if err := writeTileImages(splitImagesPath, coverages[1:], mirrored); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", splitImagesPath, err)
		return
	}
	fmt.Printf("Exported %s (%d states and their mirror images, %d bytes each)\n", splitImagesPath, splitStates-1, imageBytes)

	writeSplitComparison(references, coverages)
	writeSplitTable(tables)
}

func splitParamsAtState(state int) splitParams {
	if state == 0 {
		return splitSquare
	}
	var p splitParams
	progress := float64(state) / (2 * (splitSteps - 1))
	for i := range p {
		p[i] = splitPathStart[i] + (splitPathEnd[i]-splitPathStart[i])*progress
	}
	return p
}

func isMirroredCheckerboard(frame int) bool {
	screen, err := loadReferenceScreen(frame)
	return err == nil && checkerboardOf(&screen, tileTransforms[1])
}

// The quadrant of a bowtie of two kites, in the frame of the bowtie's centre:
// x runs along the waist, y along the long axis.
func splitQuadrant(p splitParams) morphKite {
	return morphKite{{0, 0}, {p[splitWaist], 0}, {p[splitWide], p[splitWideY]}, {0, p[splitTip]}}
}

func transposeSplitQuad(quad morphKite) morphKite {
	var transposed morphKite
	for i, v := range quad {
		transposed[i] = morphVertex{x: v.y, y: v.x}
	}
	return transposed
}

func flipSplitQuad(quad morphKite, flipX, flipY bool) morphKite {
	var flipped morphKite
	for i, v := range quad {
		if flipX {
			v.x = morphTileSize - v.x
		}
		if flipY {
			v.y = morphTileSize - v.y
		}
		flipped[i] = v
	}
	return flipped
}

// The tile corners are bowtie centres: upright at the top left and bottom right,
// lying down at the top right and bottom left.
func splitTilePolygons(p splitParams) [4]morphKite {
	upright := splitQuadrant(p)
	lying := transposeSplitQuad(upright)
	return [4]morphKite{
		upright,
		flipSplitQuad(upright, true, true),
		flipSplitQuad(lying, true, false),
		flipSplitQuad(lying, false, true),
	}
}

func renderSplitIdeal(p splitParams) morphCoverage {
	polygons := splitTilePolygons(p)
	var coverage morphCoverage
	for y := range coverage {
		for x := range coverage[y] {
			for subY := 0; subY < morphSubsamples; subY++ {
				for subX := 0; subX < morphSubsamples; subX++ {
					px := float64(x) + (float64(subX)+0.5)/morphSubsamples
					py := float64(y) + (float64(subY)+0.5)/morphSubsamples
					for _, polygon := range polygons {
						if insideMorphPolygon(polygon, px, py) {
							coverage[y][x]++
							break
						}
					}
				}
			}
		}
	}
	return coverage
}

func renderSplitDDA(p splitParams) morphCoverage {
	upright := renderSplitQuadrantDDA(splitQuadrant(p))
	lying := renderSplitQuadrantDDA(transposeSplitQuad(splitQuadrant(p)))
	var coverage morphCoverage
	last := morphTileSize - 1
	for y := range coverage {
		for x := range coverage[y] {
			coverage[y][x] = max(upright[y][x], upright[last-y][last-x], lying[y][last-x], lying[last-y][x])
		}
	}
	return coverage
}

func renderSplitQuadrantDDA(quad morphKite) morphCoverage {
	var corners [4]corner
	for i, vertex := range quad {
		corners[i] = morphCorner(vertex)
	}
	dest := make([]byte, morphDDAByteRows)
	renderCorners(corners, false, dest)

	var coverage morphCoverage
	for shiftedY := morphDDAFirstRow; shiftedY <= morphDDALastRow; shiftedY++ {
		offset := (shiftedY - morphDDAFirstRow) * morphRowBytes
		for shiftedX := 0; shiftedX < morphTileSize; shiftedX++ {
			x := (shiftedX - morphDDAShift + morphTileSize) % morphTileSize
			y := (shiftedY - morphDDAShift + morphTileSize) % morphTileSize
			coverage[y][x] = morphPetalLevel(dest[offset:offset+morphRowBytes], shiftedX)
		}
	}
	return coverage
}

func fitSplitTable(formula splitParams, ideal morphCoverage, screen *referenceScreen) splitParams {
	cost := func(p splitParams) int {
		coverage := renderSplitDDA(p)
		total := morphLevelMismatch(coverage, ideal)
		if screen != nil {
			total += splitReferenceWeight * countFrameMismatches(coverage, screen)
		}
		return total
	}
	fitted, _ := hillClimb(formula[:], splitFitGrid, []float64{1, -1, 2, -2, 4, -4, 8, -8}, func(values []float64) int {
		return cost(splitParams(values))
	})
	return splitParams(fitted)
}

func writeSplitTable(tables [splitStates]splitParams) {
	var out strings.Builder
	out.WriteString("; Generated by proto (go run . split), do not edit\n")
	out.WriteString("; Split bowtie quadrant for 50 Hz states 1-20 (state 0 is the morph's large square), four 20-word arrays\n")
	out.WriteString("; Index every array with X = (state - 1) * 2\n")
	out.WriteString("; Quadrant: O=(0,0) W=(waist,0) B=(wide,wide_y) T=(0,tip) in the frame of the bowtie's centre\n")
	out.WriteString("; A word is the coordinate in pixels * 512, the rasterizer's 8.8 units in its 48x48 supersample frame\n")
	out.WriteString("; Line comments show the coordinate in pixels\n\n")
	out.WriteString(".segment \"RODATA\"\n")
	for field, name := range splitFieldNames {
		fmt.Fprintf(&out, "\n.export split_%s_table\nsplit_%s_table:\n", name, name)
		for state := 1; state < splitStates; state++ {
			fmt.Fprintf(&out, "    .word %d  ; state %2d: %.4f\n", int(tables[state][field]*splitTableGrid+0.5), state, tables[state][field])
		}
	}
	if err := os.WriteFile(splitTablePath, []byte(out.String()), 0644); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", splitTablePath, err)
		return
	}
	fmt.Printf("Exported %s (%d bytes of data)\n", splitTablePath, (splitStates-1)*splitFieldCount*2)
}

func writeSplitComparison(references [splitSteps]morphTile, coverages [splitStates]morphCoverage) {
	const gap = 4
	cell := morphTileSize * morphScale
	img := image.NewRGBA(image.Rect(0, 0, splitStates*(cell+gap)-gap, 3*(cell+gap)-gap))
	fillMorphRect(img, img.Rect, color.RGBA{R: 40, G: 60, B: 160, A: 255})
	for state := range coverages {
		left := state * (cell + gap)
		drawMorphCoverage(img, left, cell+gap, coverages[state])
		if state%2 == 0 {
			drawMorphReference(img, left, 0, references[state/2])
			drawMorphDiff(img, left, 2*(cell+gap), references[state/2], fullMorphTile(coverages[state]))
		}
	}
	file, err := os.Create(splitComparePath)
	if err != nil {
		fmt.Printf("Warning: failed to create %s: %v\n", splitComparePath, err)
		return
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		fmt.Printf("Warning: failed to encode %s: %v\n", splitComparePath, err)
		return
	}
	fmt.Printf("Exported %s (rows: ZX reference, table via cube rasterizer, difference; one column per 50 Hz state)\n", splitComparePath)
}
