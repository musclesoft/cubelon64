package main

import (
	"fmt"
	"os"
)

const (
	cubeImagesPath    = "golden/cube_images.bin"
	cubeImageBytes    = 104
	cubeImageCount    = 2 * cubeHalfLoop
	cubeSpriteRowsLow = 3
	cubeSpriteRows    = 18
)

var cubeCharRows = [3][2]int{{4, 16}, {3, 18}, {4, 16}}

// The image of a frame as the C64 keeps it: the rows of the three char columns that are ever not
// black (16, 18 and 16 bytes) followed by the same rows' medium layer for the sprite (18 rows of
// 3 bytes), the char layer set where a pixel is black and the sprite layer where it is not light grey
func cubeImage(levels cubeLevels) []byte {
	packed := packMorphCoverage(morphCoverage(levels))
	image := make([]byte, 0, cubeImageBytes)
	for column, rows := range cubeCharRows {
		for row := rows[0]; row < rows[0]+rows[1]; row++ {
			image = append(image, packed[row*morphRowBytes+column])
		}
	}
	for row := cubeSpriteRowsLow; row < cubeSpriteRowsLow+cubeSpriteRows; row++ {
		image = append(image, packed[row*morphRowBytes+3:row*morphRowBytes+6]...)
	}
	return image
}

func cubeCroppedRowsAreBlack(levels cubeLevels) bool {
	packed := packMorphCoverage(morphCoverage(levels))
	for row := 0; row < morphTileSize; row++ {
		for column := 0; column < morphRowBytes; column++ {
			spriteLayer := column >= 3
			inside := false
			if spriteLayer {
				inside = row >= cubeSpriteRowsLow && row < cubeSpriteRowsLow+cubeSpriteRows
			} else {
				rows := cubeCharRows[column]
				inside = row >= rows[0] && row < rows[0]+rows[1]
			}
			if !inside && packed[row*morphRowBytes+column] != 0xFF {
				return false
			}
		}
	}
	return true
}

func writeCubeImages(stored *[cubeStoredStates]cubeRecord) {
	var images []byte
	cropped := true
	for frame := 0; frame < 2*cubeLoopStates; frame++ {
		levels := cubeFrameLevels(stored, frame)
		cropped = cropped && cubeCroppedRowsAreBlack(levels)
		if frame < cubeImageCount {
			images = append(images, cubeImage(levels)...)
		}
	}
	if !cropped {
		fmt.Println("Warning: a cube frame has pixels outside the cropped rows")
	}
	if err := os.WriteFile(cubeImagesPath, images, 0644); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", cubeImagesPath, err)
		return
	}
	fmt.Printf("Exported %s (%d images of %d bytes)\n", cubeImagesPath, cubeImageCount, cubeImageBytes)
}
