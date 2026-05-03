package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// BuildPointerTable extracts channel pointers for every pattern from memory dumps
// Output format: [frame:uint32][ch0_ptr:uint16]...[ch13_ptr:uint16] repeated

func main() {
	// For now, just output the hardcoded pointers we have
	// Frame 162 pointers

	fmt.Println("Pointer table builder")
	fmt.Println("TODO: Run emulator with hook to capture pointers at every frame")
	fmt.Println("For now, using extracted samples")

	// Sample data from extracted dumps
	samples := []struct{
		frame uint32
		pointers [14]uint16
	}{
		{162, [14]uint16{0xC782, 0xC937, 0xCAAB, 0xCAAB, 0xCAB5, 0xCAAB, 0xCAAB, 0xC973, 0xC99B, 0xCAAB, 0xCA36, 0xC9AF, 0xCAAB, 0xCA8C}},
		{364, [14]uint16{0xC784, 0xC939, 0xC798, 0xC94D, 0xC8B4, 0xCAAD, 0xCAAD, 0xC989, 0xCAAD, 0xCA2E, 0xCA86, 0xC9B1, 0xCAAD, 0xCAAD}},
		{400, [14]uint16{0xC780, 0xC935, 0xC794, 0xC949, 0xC8E8, 0xCAA9, 0xCAA9, 0xC985, 0xCAA9, 0xCA2A, 0xCA82, 0xC9AD, 0xCAA9, 0xCAA9}},
		{500, [14]uint16{0xCAA9, 0xCAA9, 0xC7A8, 0xC949, 0xC7F8, 0xCAA9, 0xCAA9, 0xCA16, 0xCAA9, 0xC9EF, 0xCA8C, 0xC9AD, 0xCAA9, 0xCAA9}},
		{600, [14]uint16{0xCAA9, 0xCAA9, 0xC7A8, 0xC949, 0xC834, 0xCAA9, 0xCAA9, 0xC985, 0xCAA9, 0xCA2A, 0xCA47, 0xC9AD, 0xCAA9, 0xCAA9}},
	}

	// Write to file
	f, _ := os.Create("pointer_samples.bin")
	defer f.Close()

	for _, s := range samples {
		binary.Write(f, binary.LittleEndian, s.frame)
		for _, ptr := range s.pointers {
			binary.Write(f, binary.LittleEndian, ptr)
		}
	}

	fmt.Printf("Wrote %d pointer samples\n", len(samples))
}
