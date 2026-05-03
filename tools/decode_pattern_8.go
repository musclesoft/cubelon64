// Decode pattern 8 which starts at frame 160 and first produces sound at frame 172
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

func main() {
	// Load memory
	mem, err := os.ReadFile("zx_music_complete_memory.bin")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Load AY log to see what should be output
	ayData, err := os.ReadFile("../emulator/ay_commands.bin")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("=== Decoding Pattern 8 ===\n")

	// Pattern 8 in sequence
	patIdx := byte(0x08)

	// Calculate address
	rotated := patIdx
	for i := 0; i < 3; i++ {
		carry := rotated & 1
		rotated = (rotated >> 1) | (carry << 7)
	}
	hi := (rotated & 0x1F) + 0xC4
	lo := rotated & 0xE0
	absAddr := (uint16(hi) << 8) | uint16(lo)
	offset := int(absAddr - 0xC000)

	fmt.Printf("Pattern $%02X @ $%04X (offset $%04X)\n", patIdx, absAddr, offset)

	tickCount := mem[offset]
	fmt.Printf("Tick count: %d frames\n\n", tickCount)
	offset++

	// Load 14 channel pointers
	fmt.Println("Channel pointers:")
	channels := make([]uint16, 14)
	for i := 0; i < 14; i++ {
		lo := mem[offset]
		hi := mem[offset+1]
		ptr := uint16(lo) | (uint16(hi) << 8)
		channels[i] = ptr
		fmt.Printf("  Ch %2d: $%04X\n", i, ptr)
		offset += 2
	}

	// Decode each channel's data
	fmt.Println("\n=== Channel Data ===\n")

	for ch := 0; ch < 3; ch++ { // Just first 3 channels for now
		ptr := channels[ch]
		if ptr < 0xC000 {
			continue
		}

		chOffset := int(ptr - 0xC000)
		fmt.Printf("Channel %d data at $%04X:\n", ch, ptr)

		for cmd := 0; cmd < 20 && chOffset < len(mem); cmd++ {
			cmdByte := mem[chOffset]
			fmt.Printf("  %02d [@$%04X]: $%02X", cmd, ptr+uint16(cmd), cmdByte)

			chOffset++

			// Check bits
			if (cmdByte & 0x20) != 0 {
				fmt.Println(" → END")
				break
			}

			if (cmdByte & 0x80) != 0 {
				// Timing
				if chOffset+1 < len(mem) {
					lo := mem[chOffset]
					hi := mem[chOffset+1]
					wait := uint16(lo) | (uint16(hi) << 8)
					fmt.Printf(" → WAIT %d\n", wait)
					chOffset += 2
				}
				continue
			}

			if (cmdByte & 0x40) != 0 {
				// Effect
				if chOffset+1 < len(mem) {
					lo := mem[chOffset]
					hi := mem[chOffset+1]
					param := uint16(lo) | (uint16(hi) << 8)
					fmt.Printf(" → EFFECT type=%d param=$%04X\n", cmdByte&0x07, param)
					chOffset += 2
				}
				continue
			}

			// Normal command
			if chOffset+1 < len(mem) {
				val1 := mem[chOffset]
				val2 := mem[chOffset+1]
				fmt.Printf(" → CMD reg=%d? v1=$%02X v2=$%02X\n", cmdByte&0x07, val1, val2)
				chOffset += 2
			}
		}

		fmt.Println()
	}

	// Show what actually gets output at frame 172
	fmt.Println("=== Expected AY Output (Frame 172) ===\n")

	pos := 4
	count := binary.LittleEndian.Uint32(ayData[0:4])
	for i := uint32(0); i < count; i++ {
		frame := binary.LittleEndian.Uint32(ayData[pos : pos+4])
		reg := ayData[pos+4]
		val := ayData[pos+5]
		pos += 6

		if frame == 172 {
			fmt.Printf("  Reg %2d = $%02X\n", reg, val)
		}
		if frame > 172 {
			break
		}
	}
}
