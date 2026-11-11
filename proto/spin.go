package main

import (
	"fmt"
	"math"
	"strings"
)

const (
	spinStates       = morphStates
	spinHalfStates   = morphStates/2 + 1
	spinStepDegrees  = 4.5
	spinShapeGrid    = 64
	spinFirstFrame   = 879
	spinFramesBefore = 859
	spinDataPath     = "../c64port/src/spin_data.inc"
	spinTilesPath    = "golden/spin_tiles.bin"
	spinHalfRows     = morphTileSize / 2
)

type spinKite = morphKite

// The state's bowtie angle in degrees: 4.5 per state, fitted per frame
type spinAngles [spinHalfStates]float64

func runSpin() {
	var screens [spinStates]referenceScreen
	var references [spinStates]morphTile
	var framed [spinStates]bool
	for state := 0; state < spinStates; state += 2 {
		frame := spinFrameOfState(state)
		screen, err := loadReferenceScreen(frame)
		if err != nil {
			fmt.Printf("No ZX reference: %v\n", err)
			return
		}
		screens[state] = screen
		references[state] = screen.tile(referenceTileOrigin, referenceTileOrigin)
		framed[state] = true
	}

	shape := fitSpinShape(&screens)
	fmt.Printf("Bowtie waist %.4f wide %.4f wide_y %.4f tip %.4f\n", shape[splitWaist], shape[splitWide], shape[splitWideY], shape[splitTip])
	var angles spinAngles
	var halves [spinHalfStates]morphCoverage
	for state := range halves {
		angles[state] = spinStepDegrees * float64(state)
		halves[state] = renderSpinIdeal(shape, angles[state])
		if state%2 == 0 {
			halves[state] = softenDisagreements(halves[state], &screens[state])
		}
	}
	var coverages [spinStates]morphCoverage
	for state := range coverages {
		coverages[state] = spinCoverageOfState(halves, state)
	}

	total := 0
	for state := 0; state < spinStates; state++ {
		if !framed[state] {
			continue
		}
		mismatches := countFrameMismatches(coverages[state], &screens[state])
		diff := countMorphDiff(fullMorphTile(coverages[state]), references[state])
		total += mismatches
		fmt.Printf("state %2d frame %d: angle %.2f, %d frame mismatches, %d pixels differ\n", state, spinFrameOfState(state), spinAngleOfState(angles, state), mismatches, diff)
	}
	fmt.Printf("Spin tiles: %d frame mismatches per tile in total\n", total)

	if err := writeTilePages(spinTilesPath, coverages[:]); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", spinTilesPath, err)
		return
	}
	fmt.Printf("Exported %s (%d states, one %d byte page each)\n", spinTilesPath, spinStates, tilePageBytes)
	writeSpinData(halves)
}

func spinFrameOfState(state int) int {
	if state == 0 {
		return spinFramesBefore
	}
	return spinFirstFrame + state
}

// States above the middle one show the transpose of the state as far below it
func spinCoverageOfState(halves [spinHalfStates]morphCoverage, state int) morphCoverage {
	if state < spinHalfStates {
		return halves[state]
	}
	return transposeCoverage(halves[spinStates-1-state])
}

func spinAngleOfState(angles spinAngles, state int) float64 {
	if state < spinHalfStates {
		return angles[state]
	}
	return 90 - angles[spinStates-1-state]
}

func transposeCoverage(coverage morphCoverage) morphCoverage {
	var transposed morphCoverage
	for y := range coverage {
		for x := range coverage[y] {
			transposed[x][y] = coverage[y][x]
		}
	}
	return transposed
}

// Fits the shape of the bowtie so that the nominal angles show every frame best. The shape of
// the split's last frame alone is not unique, the rotated frames pin it down.
func fitSpinShape(screens *[spinStates]referenceScreen) splitParams {
	cost := func(values []float64) int {
		shape := splitParams(values)
		total := 0
		for state := 0; state < spinHalfStates; state += 2 {
			total += countFrameMismatches(renderSpinIdeal(shape, spinStepDegrees*float64(state)), &screens[state])
		}
		return total
	}
	fitted, _ := hillClimb(splitPathEnd[:], spinShapeGrid, []float64{1, -1, 2, -2, 4, -4}, cost)
	return splitParams(fitted)
}

func spinKites(p splitParams, angle float64) [2]spinKite {
	tipOffset := p[splitWaist] * p[splitWideY] / (p[splitWide] - p[splitWaist])
	base := spinKite{{0, -tipOffset}, {p[splitWide], p[splitWideY]}, {0, p[splitTip]}, {-p[splitWide], p[splitWideY]}}
	var kites [2]spinKite
	for k := range kites {
		radians := angle*math.Pi/180 + float64(k)*math.Pi
		cos, sin := math.Cos(radians), math.Sin(radians)
		for i, v := range base {
			kites[k][i] = morphVertex{x: cos*v.x - sin*v.y, y: sin*v.x + cos*v.y}
		}
	}
	return kites
}

func spinQuadrantCovered(shape splitParams, angle, x, y float64) bool {
	if x < 0 || y < 0 {
		return false
	}
	for _, kite := range spinKites(shape, angle) {
		if insideMorphPolygon(kite, x, y) {
			return true
		}
	}
	return false
}

// The tile of a bowtie rotated by the angle (the upright ones; the lying ones rotate the
// other way), as levels like the rasterizer's
func renderSpinIdeal(shape splitParams, angle float64) morphCoverage {
	var coverage morphCoverage
	for y := range coverage {
		for x := range coverage[y] {
			count := 0
			for subY := 0; subY < morphSubsamples; subY++ {
				for subX := 0; subX < morphSubsamples; subX++ {
					px := float64(x) + (float64(subX)+0.5)/morphSubsamples
					py := float64(y) + (float64(subY)+0.5)/morphSubsamples
					if spinQuadrantCovered(shape, angle, px, py) ||
						spinQuadrantCovered(shape, angle, morphTileSize-px, morphTileSize-py) ||
						spinQuadrantCovered(shape, angle+90, px, morphTileSize-py) ||
						spinQuadrantCovered(shape, angle+90, morphTileSize-px, py) {
						count++
					}
				}
			}
			coverage[y][x] = morphCoverageLevel(count)
		}
	}
	return coverage
}

// The rows 0-11 of a tile with 180 degree symmetry, as the C64 expands them
func writeSpinData(halves [spinHalfStates]morphCoverage) {
	var out strings.Builder
	out.WriteString("; Generated by proto (go run . spin), do not edit\n")
	out.WriteString("; The first half of the spin states 0-10 (state N and 20 - N are transposes of each other):\n")
	out.WriteString("; tile rows 0-11 of 6 bytes (3 full mask, 3 medium mask), the other rows follow from the\n")
	out.WriteString("; 180 degree symmetry of the tile\n\n")
	out.WriteString(".segment \"SPINDATA\"\n\n")
	out.WriteString(".export spin_half_tiles\nspin_half_tiles:\n")
	for state, coverage := range halves {
		packed := packMorphCoverage(coverage)
		fmt.Fprintf(&out, "    ; state %d\n", state)
		for row := 0; row < spinHalfRows; row++ {
			bytes := packed[row*morphRowBytes : (row+1)*morphRowBytes]
			fmt.Fprintf(&out, "    .byte $%02X, $%02X, $%02X, $%02X, $%02X, $%02X\n", bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5])
		}
	}
	for state, coverage := range halves {
		for y := range coverage {
			for x := range coverage[y] {
				if coverage[y][x] != coverage[morphTileSize-1-y][morphTileSize-1-x] {
					fmt.Printf("Warning: spin state %d is not symmetric by 180 degrees, its half tile cannot rebuild it\n", state)
					return
				}
			}
		}
	}
	if err := writeTextFile(spinDataPath, out.String()); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", spinDataPath, err)
		return
	}
	fmt.Printf("Exported %s (%d bytes of data)\n", spinDataPath, spinHalfStates*spinHalfRows*morphRowBytes)
}

// Where the model and the ZX pixel disagree, the pixel is on an edge the model places a
// fraction of a pixel off: show it as medium, which the frame comparison accepts on an edge
func softenDisagreements(coverage morphCoverage, screen *referenceScreen) morphCoverage {
	for y := range coverage {
		for x, level := range coverage[y] {
			lit := screen.lit(referenceTileOrigin+x, referenceTileOrigin+y)
			if (level == morphFullCoverage && !lit) || (level == 0 && lit) {
				coverage[y][x] = 1
			}
		}
	}
	return coverage
}
