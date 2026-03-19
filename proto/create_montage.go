package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
)

// Create a montage of all frames
func createMontage() {
	const (
		framesPerRow = 10
		frameSize    = 192 // 24*8 pixels
		padding      = 8
	)

	rows := (41 + framesPerRow - 1) / framesPerRow
	montageWidth := framesPerRow*frameSize + (framesPerRow+1)*padding
	montageHeight := rows*frameSize + (rows+1)*padding

	montage := image.NewRGBA(image.Rect(0, 0, montageWidth, montageHeight))

	// Fill with white background
	draw.Draw(montage, montage.Bounds(), image.White, image.Point{}, draw.Src)

	for frame := 0; frame <= 40; frame++ {
		visFile := fmt.Sprintf("golden/frame%d_vis.png", frame)
		f, err := os.Open(visFile)
		if err != nil {
			fmt.Printf("Error opening %s: %v\n", visFile, err)
			continue
		}

		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			fmt.Printf("Error decoding %s: %v\n", visFile, err)
			continue
		}

		// Calculate position in montage
		row := frame / framesPerRow
		col := frame % framesPerRow
		x := padding + col*(frameSize+padding)
		y := padding + row*(frameSize+padding)

		// Draw frame into montage
		dst := image.Rect(x, y, x+frameSize, y+frameSize)
		draw.Draw(montage, dst, img, image.Point{}, draw.Src)
	}

	// Save montage
	outFile := "golden/frames_montage.png"
	f, err := os.Create(outFile)
	if err != nil {
		fmt.Printf("Error creating montage: %v\n", err)
		return
	}
	defer f.Close()

	err = png.Encode(f, montage)
	if err != nil {
		fmt.Printf("Error encoding montage: %v\n", err)
		return
	}

	fmt.Printf("Created %s (%d×%d)\n", outFile, montageWidth, montageHeight)
}
