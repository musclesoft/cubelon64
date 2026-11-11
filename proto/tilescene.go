package main

import (
	"fmt"
	"image/png"
	"os"
)

const (
	referenceScreenSize = 192
	tilePageBytes       = 256
	referenceTileOrigin = 2 * morphTileSize
)

type referenceScreen [referenceScreenSize][referenceScreenSize]bool

func (screen *referenceScreen) lit(x, y int) bool {
	return x >= 0 && y >= 0 && x < referenceScreenSize && y < referenceScreenSize && screen[y][x]
}

func (screen *referenceScreen) tile(originX, originY int) morphTile {
	var tile morphTile
	for y := range tile {
		for x := range tile[y] {
			tile[y][x] = screen.lit(originX+x, originY+y)
		}
	}
	return tile
}

func loadReferenceScreen(frame int) (screen referenceScreen, err error) {
	file, err := os.Open(fmt.Sprintf("../emulator/frames/frame_%05d_zx.png", frame))
	if err != nil {
		return screen, err
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		return screen, err
	}
	for y := range screen {
		for x := range screen[y] {
			r, _, _, _ := img.At(x, y).RGBA()
			screen[y][x] = r > 0x8000
		}
	}
	return screen, nil
}

// Counts the pixels of an interior tile the frame comparison rejects: a full or empty
// pixel must match the ZX pixel, a medium one must lie on an edge of the ZX picture.
// The tile at referenceTileOrigin must show the same orientation as the coverage.
func countFrameMismatches(coverage morphCoverage, screen *referenceScreen) int {
	mismatches := 0
	for y := range coverage {
		for x, level := range coverage[y] {
			screenX, screenY := referenceTileOrigin+x, referenceTileOrigin+y
			switch level {
			case 0:
				if screen.lit(screenX, screenY) {
					mismatches++
				}
			case morphFullCoverage:
				if !screen.lit(screenX, screenY) {
					mismatches++
				}
			default:
				lit := 0
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if screen.lit(screenX+dx, screenY+dy) {
							lit++
						}
					}
				}
				if lit == 0 || lit == 9 {
					mismatches++
				}
			}
		}
	}
	return mismatches
}

// Writes one page per state, the layout of the tile pages in C64 memory, so a single
// assertion can compare all of them
func writeTilePages(path string, coverages []morphCoverage) error {
	pages := make([]byte, 0, len(coverages)*tilePageBytes)
	for _, coverage := range coverages {
		page := make([]byte, tilePageBytes)
		copy(page, packMorphCoverage(coverage))
		pages = append(pages, page...)
	}
	if err := os.MkdirAll("golden", 0755); err != nil {
		return err
	}
	return os.WriteFile(path, pages, 0644)
}

// Moves one coordinate at a time by the given steps (in units of 1/grid) while that lowers
// the cost, and returns where it stopped
func hillClimb(start []float64, grid float64, steps []float64, cost func([]float64) int) ([]float64, int) {
	current := snapToGrid(start, grid)
	currentCost := cost(current)
	for improved := true; improved; {
		improved = false
		for i := range current {
			for _, step := range steps {
				candidate := append([]float64(nil), current...)
				candidate[i] += step / grid
				candidate = snapToGrid(candidate, grid)
				if candidateCost := cost(candidate); candidateCost < currentCost {
					current, currentCost, improved = candidate, candidateCost, true
				}
			}
		}
	}
	return current, currentCost
}

func snapToGrid(values []float64, grid float64) []float64 {
	snapped := make([]float64, len(values))
	for i, value := range values {
		snapped[i] = float64(int(value*grid+0.5)) / grid
	}
	return snapped
}

func writeTextFile(path, text string) error {
	return os.WriteFile(path, []byte(text), 0644)
}
