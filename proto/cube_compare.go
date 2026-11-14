package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

const (
	cubeCompareGap    = 2
	cubeCompareScale  = 3
	cubeCompareRows   = 6
	cubeMaxMismatches = 8.0
)

var (
	cubeBackground = color.RGBA{R: 60, G: 60, B: 60, A: 255}
	cubeDarkGrey   = color.RGBA{R: 0x5F, G: 0x5F, B: 0x5F, A: 255}
	cubeDiffHard   = color.RGBA{R: 255, A: 255}
	cubeDiffOff    = color.RGBA{R: 20, G: 20, B: 20, A: 255}
	cubeDiffOn     = color.RGBA{R: 110, G: 110, B: 110, A: 255}
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

func cubeBlackWhite(lit bool) color.RGBA {
	if lit {
		return cubeGrey(255)
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

func cubeDiffColour(candidates *cubeCandidates, x, y int, lit bool) color.RGBA {
	switch {
	case !candidates.explains(x, y, lit):
		return cubeDiffHard
	case candidates.centreOn(x, y) != lit:
		return cubeDiffStrict
	case lit:
		return cubeDiffOn
	}
	return cubeDiffOff
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

func cubeValidatorMismatches(levels *[morphTileSize][morphTileSize]int, target *morphTile) float64 {
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

// One column per ZX state, six rows: the ZX frame; the model with solid colours, decided by
// which cube side is where (light grey white face, dark grey checker, darker grey dots, black);
// the model as the C64 renders it (2x2 subpixels; black, dark grey, light grey; the dots face
// as a checkerboard of dark grey and black); the model with the ZX dither
// patterns of the colours; the difference (red: no candidate shade explains the ZX
// pixel, orange: only the pixel's centre sample disagrees, grey: match); and the C64 render
// again with the validator's mismatches (red: 1, orange: 0.5)
func writeCubeComparison(targets []morphTile, fit cubeFit) {
	cell := morphTileSize * cubeCompareScale
	pitch := cell + cubeCompareGap
	img := image.NewRGBA(image.Rect(0, 0, cubeStates*pitch, cubeCompareRows*pitch))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = cubeBackground.R, cubeBackground.G, cubeBackground.B, cubeBackground.A
	}
	hardTotal, strictTotal, overLimit, worstState := 0, 0, 0, 0
	worstWeight, weightTotal := 0.0, 0.0
	for state := range targets {
		target := &targets[state]
		rotation := fit.Motion.rotationAt(float64(state))
		candidates := fit.Camera.candidates(rotation)
		levels := fit.Camera.subpixelLevels(rotation)
		left := state * pitch
		drawCubeCell(img, left, 0, func(x, y int) color.RGBA { return cubeBlackWhite(target[y][x]) })
		drawCubeCell(img, left, pitch, func(x, y int) color.RGBA { return cubeShadeColour(candidates.centre[y][x]) })
		drawCubeCell(img, left, 2*pitch, func(x, y int) color.RGBA { return cubeLevelColour(levels[y][x]) })
		drawCubeCell(img, left, 3*pitch, func(x, y int) color.RGBA { return cubeBlackWhite(candidates.centreOn(x, y)) })
		drawCubeCell(img, left, 4*pitch, func(x, y int) color.RGBA { return cubeDiffColour(&candidates, x, y, target[y][x]) })
		drawCubeCell(img, left, 5*pitch, func(x, y int) color.RGBA { return cubeValidatorColour(levels[y][x], target, x, y) })
		hard, strict := candidates.mismatches(target)
		hardTotal += hard
		strictTotal += strict
		weight := cubeValidatorMismatches(&levels, target)
		weightTotal += weight
		if weight > worstWeight {
			worstWeight, worstState = weight, state
		}
		if weight > cubeMaxMismatches {
			overLimit++
		}
		fmt.Printf("state %2d frame %d: validator mismatches %.1f\n", state, cubeFirstFrame+cubeFrameStep*state, weight)
		if hard > 0 {
			fmt.Printf("state %2d frame %d: %d hard mismatches\n", state, cubeFirstFrame+cubeFrameStep*state, hard)
		}
	}
	fmt.Printf("C64 render: worst state %d with %.1f validator mismatches in the block (%.1f in total), %d of %d states over the limit of %.0f\n", worstState, worstWeight, weightTotal, overLimit, cubeStates, cubeMaxMismatches)
	fmt.Printf("Cube: %d hard mismatches, %d tolerated centre-sample mismatches over %d states\n", hardTotal, strictTotal, cubeStates)
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
	fmt.Printf("Exported %s (rows: ZX reference, model in solid colours, model as the C64 renders it (2x2 subpixels), model dithered, difference, C64 render with validator mismatches; one column per 25 Hz state)\n", cubeComparePath)
}
