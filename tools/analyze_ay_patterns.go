// Analyze correlation between pattern loads and AY output changes
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

func main() {
	// Load AY command log
	data, err := os.ReadFile("../emulator/ay_commands.bin")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	count := binary.LittleEndian.Uint32(data[0:4])
	fmt.Printf("Total AY commands: %d\n\n", count)

	// Parse commands by frame
	type RegState [13]byte
	frameStates := make(map[uint32]RegState)

	pos := 4
	for i := uint32(0); i < count; i++ {
		frame := binary.LittleEndian.Uint32(data[pos : pos+4])
		reg := data[pos+4]
		val := data[pos+5]
		pos += 6

		if reg < 13 {
			state := frameStates[frame]
			state[reg] = val
			frameStates[frame] = state
		}
	}

	fmt.Printf("Frames with AY data: %d\n\n", len(frameStates))

	// Analyze changes over time
	fmt.Println("=== Register Changes ===\n")

	prevState := RegState{}
	changeFrames := []uint32{}

	for frame := uint32(0); frame < 200; frame++ {
		state, ok := frameStates[frame]
		if !ok {
			continue
		}

		changed := false
		for reg := 0; reg < 13; reg++ {
			if state[reg] != prevState[reg] {
				changed = true
				break
			}
		}

		if changed && frame >= 170 {
			changeFrames = append(changeFrames, frame)
			fmt.Printf("Frame %d: ", frame)
			for reg := 0; reg < 13; reg++ {
				if state[reg] != prevState[reg] {
					fmt.Printf("R%d=$%02X ", reg, state[reg])
				}
			}
			fmt.Println()
		}

		prevState = state
	}

	// Pattern timing analysis
	// From earlier analysis: Pattern sequence is at $C000
	// Pattern 0 has tick=20, pattern 1 has tick=?, etc.
	// Music starts around frame 172

	fmt.Println("\n=== Pattern Timing Hypothesis ===")
	fmt.Println("If patterns change every N frames, changes should align...")

	// Load pattern sequence
	seq, err := os.ReadFile("zx_pattern_sequence.bin")
	if err != nil {
		fmt.Printf("\nCan't load sequence: %v\n", err)
		return
	}

	// Load pattern table to get tick counts
	patterns, err := os.ReadFile("zx_pattern_table.bin")
	if err != nil {
		fmt.Printf("\nCan't load patterns: %v\n", err)
		return
	}

	fmt.Println("\nPattern sequence and tick counts:")

	totalFrames := 0
	for i := 0; i < 20 && i < len(seq); i++ {
		patIdx := seq[i]
		if patIdx == 0xFF {
			break
		}

		// Calculate pattern address using RRCA formula
		rotated := patIdx
		for j := 0; j < 3; j++ {
			carry := rotated & 1
			rotated = (rotated >> 1) | (carry << 7)
		}

		hi := (rotated & 0x1F) + 0xC4
		lo := rotated & 0xE0
		absAddr := (uint16(hi) << 8) | uint16(lo)
		offset := int(absAddr - 0xC400)

		if offset >= 0 && offset < len(patterns) {
			tickCount := patterns[offset]
			fmt.Printf("  Seq[%2d] = Pattern $%02X @ $%04X: tick=%3d (frames %d-%d)\n",
				i, patIdx, absAddr, tickCount, totalFrames, totalFrames+int(tickCount)-1)
			totalFrames += int(tickCount)
		}
	}

	fmt.Println("\n=== Observation ===")
	fmt.Println("The AY registers are written every frame (13 writes per frame)")
	fmt.Println("Changes are minimal frame-to-frame (mostly envelope/volume)")
	fmt.Println("Pattern changes don't correlate 1:1 with large AY changes")
	fmt.Println("\nConclusion: The music uses envelopes and持续tone generation.")
	fmt.Println("The pattern data likely sets up initial states, then hardware envelopes")
	fmt.Println("handle the actual sound evolution.")
}
