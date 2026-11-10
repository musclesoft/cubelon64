package main

import (
	"fmt"
	"strconv"
)

var analyzePeriods = []int{8, 12, 16, 24, 48}

type tileTransform struct {
	name  string
	apply func(morphTile) morphTile
}

var tileTransforms = []tileTransform{
	{"same", func(tile morphTile) morphTile { return tile }},
	{"mirror-x", func(tile morphTile) morphTile { return transformMorphTile(tile, true, false, false) }},
	{"mirror-y", func(tile morphTile) morphTile { return transformMorphTile(tile, false, true, false) }},
	{"rot180", func(tile morphTile) morphTile { return transformMorphTile(tile, true, true, false) }},
	{"transpose", func(tile morphTile) morphTile { return transformMorphTile(tile, false, false, true) }},
}

// Describes how the ZX screen of each frame repeats: its periods, the symmetries of its first
// tile and how every tile relates to the tile left of it and above it
func runAnalyze(args []string) {
	for _, arg := range args {
		frame, err := strconv.Atoi(arg)
		if err != nil {
			fmt.Printf("Not a frame number: %s\n", arg)
			continue
		}
		screen, err := loadReferenceScreen(frame)
		if err != nil {
			fmt.Printf("No ZX reference: %v\n", err)
			continue
		}
		analyzeScreen(frame, &screen)
	}
}

func analyzeScreen(frame int, screen *referenceScreen) {
	fmt.Printf("Frame %d\n", frame)
	fmt.Print("  pixels differing when shifted by")
	for _, period := range analyzePeriods {
		dx, dy := shiftedDifference(screen, period, 0), shiftedDifference(screen, 0, period)
		fmt.Printf(" %d: x %d y %d |", period, dx, dy)
	}
	fmt.Println()

	tile := screen.tile(0, 0)
	fmt.Print("  tile (0,0) differs from its own")
	for _, transform := range tileTransforms[1:] {
		fmt.Printf(" %s: %d |", transform.name, countMorphDiff(transform.apply(tile), tile))
	}
	fmt.Println()

	for _, neighbour := range [][2]int{{1, 0}, {0, 1}} {
		fmt.Printf("  tile (%d,%d) as seen from (0,0) is", neighbour[0], neighbour[1])
		other := screen.tile(neighbour[0]*morphTileSize, neighbour[1]*morphTileSize)
		for _, transform := range tileTransforms {
			fmt.Printf(" %s: %d", transform.name, countMorphDiff(transform.apply(tile), other))
			fmt.Print(" |")
		}
		fmt.Println()
	}
	fmt.Println("  every tile follows its checkerboard parity as one of:")
	for _, transform := range tileTransforms {
		if checkerboardOf(screen, transform) {
			fmt.Printf("    tile and %s\n", transform.name)
		}
	}
	for y := range tile {
		fmt.Print("  ")
		for x := range tile[y] {
			if tile[y][x] {
				fmt.Print("#")
			} else {
				fmt.Print(".")
			}
		}
		fmt.Println()
	}
}

func shiftedDifference(screen *referenceScreen, dx, dy int) int {
	different := 0
	for y := 0; y < referenceScreenSize-dy; y++ {
		for x := 0; x < referenceScreenSize-dx; x++ {
			if screen[y][x] != screen[y+dy][x+dx] {
				different++
			}
		}
	}
	return different
}

func checkerboardOf(screen *referenceScreen, transform tileTransform) bool {
	tile := screen.tile(0, 0)
	other := transform.apply(tile)
	for tileY := 0; tileY < referenceScreenSize/morphTileSize; tileY++ {
		for tileX := 0; tileX < referenceScreenSize/morphTileSize; tileX++ {
			want := tile
			if (tileX+tileY)%2 == 1 {
				want = other
			}
			if screen.tile(tileX*morphTileSize, tileY*morphTileSize) != want {
				return false
			}
		}
	}
	return true
}

func transformMorphTile(tile morphTile, flipX, flipY, transpose bool) morphTile {
	var transformed morphTile
	last := morphTileSize - 1
	for y := range tile {
		for x := range tile[y] {
			destX, destY := x, y
			if flipX {
				destX = last - destX
			}
			if flipY {
				destY = last - destY
			}
			if transpose {
				destX, destY = destY, destX
			}
			transformed[destY][destX] = tile[y][x]
		}
	}
	return transformed
}
