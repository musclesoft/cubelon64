// Find the exact channel-to-register mapping
package main

import (
	"fmt"
	"os"
)

func main() {
	mem, err := os.ReadFile("zx_music_complete_memory.bin")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("=== Finding Channel to Register Mapping ===\n")

	// Pattern 8 channels
	channels := []uint16{
		0xF644, 0xECC6, 0xEA37, 0xF57B, 0xF58F, 0xF58F, 0xF58F,
		0xF071, 0xF237, 0xF139, 0xF58F, 0xEFEA, 0xF58F, 0xED9D,
	}

	// Expected AY output at frame 172
	expectedRegs := []byte{
		0xE0, 0x08, 0x00, 0x00, 0x8E, 0x00, 0x00,
		0x68, 0x10, 0x00, 0x0D, 0x24, 0x00,
	}

	// Read first byte from each channel
	channelValues := make([]byte, 14)
	for i, ptr := range channels {
		if ptr >= 0xC000 {
			offset := int(ptr - 0xC000)
			if offset < len(mem) {
				channelValues[i] = mem[offset]
			}
		}
	}

	fmt.Println("Trying all possible channel → register mappings...")
	fmt.Println()

	// Try to find mapping
	bestMapping := make([]int, 14)
	bestScore := 0

	for ch := 0; ch < 14; ch++ {
		chVal := channelValues[ch]

		// Find which register this value matches
		for reg := 0; reg < 13; reg++ {
			if chVal == expectedRegs[reg] {
				fmt.Printf("Channel %2d ($%02X) matches Register %2d\n", ch, chVal, reg)
				bestMapping[ch] = reg
				bestScore++
				break
			}
		}
	}

	fmt.Printf("\nMatches found: %d/14\n", bestScore)

	// Show unmatched channels
	fmt.Println("\nUnmatched channels:")
	for ch := 0; ch < 14; ch++ {
		chVal := channelValues[ch]
		matched := false
		for reg := 0; reg < 13; reg++ {
			if chVal == expectedRegs[reg] {
				matched = true
				break
			}
		}
		if !matched {
			fmt.Printf("  Channel %2d: value=$%02X (no direct match in AY output)\n", ch, chVal)
		}
	}

	// Check if unmatched values might be derived
	fmt.Println("\nChecking if unmatched values are transformations...")

	for ch := 0; ch < 14; ch++ {
		chVal := channelValues[ch]
		matched := false
		for reg := 0; reg < 13; reg++ {
			if chVal == expectedRegs[reg] {
				matched = true
				break
			}
		}

		if !matched {
			// Try some transformations
			for reg := 0; reg < 13; reg++ {
				regVal := expectedRegs[reg]

				// Try AND, OR, XOR, shifts
				if (chVal & 0x0F) == regVal {
					fmt.Printf("  Ch %2d ($%02X) & $0F = Reg %2d ($%02X)\n", ch, chVal, reg, regVal)
				}
				if (chVal >> 4) == regVal {
					fmt.Printf("  Ch %2d ($%02X) >> 4 = Reg %2d ($%02X)\n", ch, chVal, reg, regVal)
				}
				if (chVal | 0x80) == regVal {
					fmt.Printf("  Ch %2d ($%02X) | $80 = Reg %2d ($%02X)\n", ch, chVal, reg, regVal)
				}
			}
		}
	}

	// Maybe some channels don't output on frame 172?
	fmt.Println("\n=== Alternative: Some channels inactive at frame 172 ===")
	fmt.Println("Channels 4, 9, 10 might be inactive or output $00 elsewhere")

	// Check frame 182 which has more changes
	fmt.Println("\n=== Checking Frame 182 for more data points ===")
	fmt.Println("Frame 182 AY output:")
	fmt.Println("  R2=$1C R3=$01 R7=$70 R8=$00 R9=$0E")
	fmt.Println()
	fmt.Println("Checking if any channels match these:")
	fmt.Printf("  Ch 2 @ $EA37: $%02X (expect R2=$1C) - ", channelValues[2])
	if channelValues[2] == 0x1C {
		fmt.Println("✓ MATCH!")
	} else {
		fmt.Println("no match")
	}

	fmt.Printf("  Ch 3 @ $F57B: $%02X (expect R3=$01) - ", channelValues[3])
	if channelValues[3] == 0x01 {
		fmt.Println("✓ MATCH!")
	} else {
		fmt.Println("no match")
	}
}
