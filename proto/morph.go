package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

const (
	morphScale     = 6
	morphSteps     = 11
	morphTileSize  = 24
	morphTileBytes = 72
)

type morphVertex struct{ x, y float64 }

type morphTile [morphTileSize][morphTileSize]bool

var morphFrames = [morphSteps]int{779, 801, 803, 805, 807, 809, 811, 813, 815, 817, 819}

var morphKites = [morphSteps][4]morphVertex{
	{{6, 6}, {12, 6}, {12, 12}, {6, 12}},
	{{4, 4}, {12, 6}, {12.25, 12.25}, {6, 12}},
	{{2.75, 2.75}, {11, 5.5}, {12, 12}, {5.5, 11}},
	{{2.5, 2.5}, {10.75, 5}, {11.625, 11.625}, {5, 10.75}},
	{{2, 2}, {10.125, 4}, {11.375, 11.5}, {4, 10}},
	{{1.25, 1.5}, {9.25, 3.25}, {11.5, 10.875}, {3.75, 10.25}},
	{{0.75, 0.75}, {9, 2.75}, {10.5, 10.5}, {2.75, 9}},
	{{0, 0}, {8.5, 2}, {10.5, 10.5}, {2, 8.5}},
	{{-0.5, -0.5}, {8, 1}, {10, 10}, {1, 8}},
	{{-2, -0.5}, {7.75, 0.625}, {9.5, 9.25}, {1, 8}},
	{{0, 0}, {8, 0}, {8, 8}, {0, 8}},
}

func runMorph() {
	var tiles [morphSteps]morphTile
	var packed []byte
	for step := range tiles {
		tiles[step] = renderMorphTile(morphKites[step])
		packed = append(packed, packMorphTile(tiles[step])...)
	}

	os.MkdirAll("golden", 0755)
	if err := os.WriteFile("golden/morph_tiles.bin", packed, 0644); err != nil {
		fmt.Printf("Warning: failed to write golden/morph_tiles.bin: %v\n", err)
		return
	}
	fmt.Printf("Exported golden/morph_tiles.bin (%d bytes)\n", len(packed))

	var references [morphSteps]morphTile
	for step, frame := range morphFrames {
		reference, err := loadMorphReference(frame)
		if err != nil {
			fmt.Printf("No ZX reference: %v\n", err)
			return
		}
		references[step] = reference
		fmt.Printf("frame %d: %d pixels differ from ZX reference\n", frame, countMorphDiff(tiles[step], reference))
	}

	writeMorphComparison(references, tiles)
}

func renderMorphTile(kite [4]morphVertex) morphTile {
	var polygons [4][4]morphVertex
	for corner := range polygons {
		for i, v := range kite {
			polygons[corner][i] = mirrorMorphVertex(v, corner)
		}
	}

	var tile morphTile
	for y := range tile {
		for x := range tile[y] {
			tile[y][x] = coveredByAnyKite(polygons, float64(x)+0.5, float64(y)+0.5)
		}
	}
	return tile
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

func coveredByAnyKite(polygons [4][4]morphVertex, px, py float64) bool {
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

func insideMorphPolygon(polygon [4]morphVertex, px, py float64) bool {
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

func packMorphTile(tile morphTile) []byte {
	packed := make([]byte, 0, morphTileBytes)
	for _, row := range tile {
		for byteIndex := 0; byteIndex < morphTileSize/8; byteIndex++ {
			var b byte
			for bit := 0; bit < 8; bit++ {
				b <<= 1
				if row[byteIndex*8+bit] {
					b |= 1
				}
			}
			packed = append(packed, b)
		}
	}
	return packed
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

func writeMorphComparison(references, generated [morphSteps]morphTile) {
	const gap = 4
	cell := morphTileSize * morphScale
	img := image.NewRGBA(image.Rect(0, 0, 3*cell+2*gap, morphSteps*(cell+gap)))
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 40, G: 60, B: 160, A: 255})
		}
	}
	for step := range generated {
		top := step * (cell + gap)
		drawMorphTile(img, 0, top, references[step])
		drawMorphTile(img, cell+gap, top, generated[step])
		drawMorphDiff(img, 2*(cell+gap), top, references[step], generated[step])
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
	fmt.Println("Exported golden/morph_compare.png (columns: ZX reference, generated, difference)")
}

func drawMorphTile(img *image.RGBA, left, top int, tile morphTile) {
	for y := range tile {
		for x := range tile[y] {
			shade := color.RGBA{A: 255}
			if tile[y][x] {
				shade = color.RGBA{R: 255, G: 255, B: 255, A: 255}
			}
			fillMorphPixel(img, left+x*morphScale, top+y*morphScale, shade)
		}
	}
}

func drawMorphDiff(img *image.RGBA, left, top int, reference, generated morphTile) {
	for y := range reference {
		for x := range reference[y] {
			shade := color.RGBA{A: 255}
			switch {
			case reference[y][x] && !generated[y][x]:
				shade = color.RGBA{R: 255, A: 255}
			case !reference[y][x] && generated[y][x]:
				shade = color.RGBA{G: 255, B: 255, A: 255}
			case reference[y][x]:
				shade = color.RGBA{R: 64, G: 64, B: 64, A: 255}
			}
			fillMorphPixel(img, left+x*morphScale, top+y*morphScale, shade)
		}
	}
}

func fillMorphPixel(img *image.RGBA, left, top int, shade color.RGBA) {
	for dy := 0; dy < morphScale; dy++ {
		for dx := 0; dx < morphScale; dx++ {
			img.SetRGBA(left+dx, top+dy, shade)
		}
	}
}
