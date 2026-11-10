// Command framediff shows where a saved C64 frame differs from the ZX frame of the same
// number, by the rule of the validator: mismatches in total and per 24x24 tile, and an
// image with the C64 frame, the ZX frame and the mismatches side by side.
//
// Save the C64 frames first:
//
//	go run -C emulator run_cubelon.go assertions.go motion_analyzer.go -c64 -save-frames=855-860 ../c64port/build/cubelon64.prg
//
// Then:
//
//	go run -C emulator ./framecompare/cmd/framediff -image=diff.png 855 857 859
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"cubelon-emu/framecompare"
)

func main() {
	dir := flag.String("dir", "frames", "folder of the frame PNGs")
	imagePath := flag.String("image", "", "write the side by side image of the last frame to this file")
	scale := flag.Int("scale", 4, "scale of the image")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: framediff [-dir frames] [-image out.png] FRAME...")
		os.Exit(2)
	}

	var last *framecompare.Report
	for _, arg := range flag.Args() {
		frame, err := strconv.Atoi(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "not a frame number: %s\n", arg)
			os.Exit(2)
		}
		report, err := framecompare.Diff(*dir, frame)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("frame %d: %d mismatched pixels (limit %d)\n", frame, report.Total, framecompare.MaxMismatchedPixels)
		for _, row := range report.Tiles {
			fmt.Println(row)
		}
		last = report
	}
	if *imagePath != "" {
		if err := last.WriteImage(*imagePath, *scale); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", *imagePath)
	}
}
