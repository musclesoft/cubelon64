package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

const (
	cubeCompareGap   = 2
	cubeCompareScale = 3
	cubeCompareRows  = 4
)

var (
	cubeBackground = color.RGBA{R: 60, G: 60, B: 60, A: 255}
	cubeDarkGrey   = color.RGBA{R: 0x5F, G: 0x5F, B: 0x5F, A: 255}
	cubeDiffHard   = color.RGBA{R: 255, A: 255}
	cubeDiffStrict = color.RGBA{R: 255, G: 160, A: 255}
	cubeLightGrey  = color.RGBA{R: 0xC6, G: 0xC6, B: 0xC6, A: 255}
)

func cubeLevelColour(level int) color.RGBA {
	switch level {
	case morphFullCoverage:
		return cubeLightGrey
	case 1:
		return cubeDarkGrey
	}
	return cubeGrey(0)
}

func cubeGrey(level float64) color.RGBA {
	grey := uint8(level + 0.5)
	return color.RGBA{R: grey, G: grey, B: grey, A: 255}
}

func cubeReferenceColour(lit bool) color.RGBA {
	if lit {
		return cubeLightGrey
	}
	return cubeGrey(0)
}

func cubeShadeColour(shade int) color.RGBA {
	if shade < 0 {
		return cubeGrey(0)
	}
	return cubeGrey(cubeShadeGrey[shade])
}

func drawCubeCell(img *image.RGBA, left, top int, colourOf func(x, y int) color.RGBA) {
	for y := 0; y < morphTileSize*cubeCompareScale; y++ {
		for x := 0; x < morphTileSize*cubeCompareScale; x++ {
			img.SetRGBA(left+x, top+y, colourOf(x/cubeCompareScale, y/cubeCompareScale))
		}
	}
}

// The validator's rule for one pixel of the C64 render: black must be black in the ZX frame, light
// grey must be white, and the grey between them is an edge, which matches when the 3x3 ZX
// neighbourhood holds both black and white and counts half a mismatch otherwise
func cubeValidatorWeight(level int, target *morphTile, x, y int) float64 {
	switch level {
	case 0:
		if cubeTileLit(target, x, y) {
			return 1
		}
		return 0
	case morphFullCoverage:
		if cubeTileLit(target, x, y) {
			return 0
		}
		return 1
	}
	lit := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if cubeTileLit(target, x+dx, y+dy) {
				lit++
			}
		}
	}
	if lit > 0 && lit < 9 {
		return 0
	}
	return 0.5
}

func cubeTileLit(target *morphTile, x, y int) bool {
	return x >= 0 && y >= 0 && x < morphTileSize && y < morphTileSize && target[y][x]
}

func cubeValidatorMismatches(levels *cubeLevels, target *morphTile) float64 {
	total := 0.0
	for y := range levels {
		for x, level := range levels[y] {
			total += cubeValidatorWeight(level, target, x, y)
		}
	}
	return total
}

func cubeValidatorColour(level int, target *morphTile, x, y int) color.RGBA {
	switch cubeValidatorWeight(level, target, x, y) {
	case 1:
		return cubeDiffHard
	case 0.5:
		return cubeDiffStrict
	}
	return cubeLevelColour(level)
}

// One column per 50 Hz frame (a ZX state every second column, the C64 frames between them
// have no ZX reference), four rows: the ZX frame, its lit pixels in the C64's light grey; the
// model as the C64 renders it (2x2 subpixels, black, dark grey, light grey, with the patterns
// of the sides); the C64 render again with the validator's mismatches (red: 1, orange: 0.5);
// and the 3D model with one solid tone per cube side (white face, checker face, dots face,
// background black)
func writeCubeComparison(targets []morphTile, fit cubeFit) {
	cell := morphTileSize * cubeCompareScale
	pitch := cell + cubeCompareGap
	img := image.NewRGBA(image.Rect(0, 0, cubeFrames*pitch, cubeCompareRows*pitch))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = cubeBackground.R, cubeBackground.G, cubeBackground.B, cubeBackground.A
	}
	stored := cubeStoredRecords(fit)
	levelsOfFrame := make([]cubeLevels, cubeFrames)
	for frame := range levelsOfFrame {
		levelsOfFrame[frame] = cubeFrameLevels(stored, frame)
	}
	hardTotal, strictTotal, overLimit, worstState, junctionPairs := 0, 0, 0, 0, 0
	worstWeight, weightTotal := 0.0, 0.0
	for frame := 0; frame < cubeFrames; frame++ {
		rotation := fit.Motion.rotationAt(float64(frame) / 2)
		candidates := fit.Camera.candidates(rotation)
		levels := &levelsOfFrame[frame]
		junctionPairs += fit.Camera.junctionDarkPairs(rotation, levels)
		left := frame * pitch
		drawCubeCell(img, left, 3*pitch, func(x, y int) color.RGBA { return cubeShadeColour(candidates.centre[y][x]) })
		drawCubeCell(img, left, pitch, func(x, y int) color.RGBA { return cubeLevelColour(levels[y][x]) })
		if frame%2 != 0 {
			continue
		}
		state := frame / 2
		target := &targets[state]
		drawCubeCell(img, left, 0, func(x, y int) color.RGBA { return cubeReferenceColour(target[y][x]) })
		drawCubeCell(img, left, 2*pitch, func(x, y int) color.RGBA { return cubeValidatorColour(levels[y][x], target, x, y) })
		hard, strict := candidates.mismatches(target)
		hardTotal += hard
		strictTotal += strict
		weight := cubeValidatorMismatches(levels, target)
		weightTotal += weight
		if weight > worstWeight {
			worstWeight, worstState = weight, state
		}
		if weight > cubeMaxMismatches {
			overLimit++
		}
		if weight > cubeRenderSoftLimit {
			fmt.Printf("state %2d frame %d: validator mismatches %.1f\n", state, cubeFirstFrame+cubeFrameStep*state, weight)
		}
		if hard > 0 {
			fmt.Printf("state %2d frame %d: %d hard mismatches\n", state, cubeFirstFrame+cubeFrameStep*state, hard)
		}
	}
	fmt.Printf("C64 render: worst state %d with %.1f validator mismatches in the block (%.1f in total), %d of %d states over the limit of %.0f\n", worstState, worstWeight, weightTotal, overLimit, cubeStates, cubeMaxMismatches)
	fmt.Printf("Cube: %d hard mismatches, %d tolerated centre-sample mismatches over %d states\n", hardTotal, strictTotal, cubeStates)
	fmt.Printf("Dark pixels side by side at white/checker junctions: %d pairs over %d frames\n", junctionPairs, cubeFrames)
	reportCubeFlicker(levelsOfFrame)
	file, err := os.Create(cubeComparePath)
	if err != nil {
		fmt.Printf("Warning: failed to create %s: %v\n", cubeComparePath, err)
		return
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		fmt.Printf("Warning: failed to encode %s: %v\n", cubeComparePath, err)
		return
	}
	fmt.Printf("Exported %s (rows: ZX reference, model as the C64 renders it (2x2 subpixels), C64 render with validator mismatches, 3D model in solid colours; one column per 50 Hz frame)\n", cubeComparePath)
}

// The frames between the ZX states have no reference, so the best check is that they only
// change what their neighbours change: a pixel that differs from both neighbours while those
// agree with each other flickers for one frame
func reportCubeFlicker(levelsOfFrame []cubeLevels) {
	total, worst, worstFrame := 0, 0, 0
	for frame := 1; frame < len(levelsOfFrame)-1; frame += 2 {
		flickering := 0
		for y := range levelsOfFrame[frame] {
			for x, level := range levelsOfFrame[frame][y] {
				previous, next := levelsOfFrame[frame-1][y][x], levelsOfFrame[frame+1][y][x]
				if previous == next && level != previous {
					flickering++
				}
			}
		}
		total += flickering
		if flickering > worst {
			worst, worstFrame = flickering, frame
		}
	}
	fmt.Printf("Intermediate C64 frames: %d flickering pixels in total, at most %d (frame %d)\n", total, worst, cubeFirstFrame+worstFrame)
}
