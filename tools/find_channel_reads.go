// Find which code reads from channel data regions
package main

import (
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

	fmt.Println("=== Finding Channel Data Access Patterns ===\n")

	// From pattern 8 at frame 172, channel 1 is at $ECC6
	// Channel 1 data contains: 08 08 08 08...
	// AY output shows: Reg 1 = 08

	// Let's check if there's a simple mapping
	fmt.Println("Hypothesis: Channel N writes to Register N")
	fmt.Println("Testing with frame 172 data...\n")

	// Pattern 8 channel pointers (from earlier analysis)
	channels := []uint16{
		0xF644, // Ch 0
		0xECC6, // Ch 1
		0xEA37, // Ch 2
		0xF57B, // Ch 3
		0xF58F, // Ch 4
		0xF58F, // Ch 5
		0xF58F, // Ch 6
		0xF071, // Ch 7
		0xF237, // Ch 8
		0xF139, // Ch 9
		0xF58F, // Ch 10
		0xEFEA, // Ch 11
		0xF58F, // Ch 12
		0xED9D, // Ch 13
	}

	// Expected frame 172 output
	expectedRegs := []byte{
		0xE0, // Reg 0
		0x08, // Reg 1
		0x00, // Reg 2
		0x00, // Reg 3
		0x8E, // Reg 4
		0x00, // Reg 5
		0x00, // Reg 6
		0x68, // Reg 7
		0x10, // Reg 8
		0x00, // Reg 9
		0x0D, // Reg 10
		0x24, // Reg 11
		0x00, // Reg 12
	}

	fmt.Println("Channel → Expected AY Register Value:")
	for ch := 0; ch < 13 && ch < len(channels); ch++ {
		ptr := channels[ch]
		if ptr < 0xC000 {
			continue
		}

		offset := int(ptr - 0xC000)
		if offset >= len(mem) {
			continue
		}

		// Read first byte from channel
		val := mem[offset]
		expected := expectedRegs[ch]

		match := ""
		if val == expected {
			match = "✓ MATCH"
		} else {
			match = fmt.Sprintf("✗ mismatch (got $%02X, want $%02X)", val, expected)
		}

		fmt.Printf("  Ch %2d @ $%04X: $%02X → Reg %2d: %s\n", ch, ptr, val, ch, match)
	}

	fmt.Println("\n=== Checking if direct byte = register value ===")

	// Check Channel 1 in detail
	ch1Ptr := channels[1]
	ch1Offset := int(ch1Ptr - 0xC000)

	fmt.Printf("\nChannel 1 data stream (first 32 bytes):\n")
	for i := 0; i < 32 && ch1Offset+i < len(mem); i++ {
		val := mem[ch1Offset+i]
		if i%16 == 0 {
			fmt.Printf("  $%04X: ", ch1Ptr+uint16(i))
		}
		fmt.Printf("%02X ", val)
		if i%16 == 15 {
			fmt.Println()
		}
	}

	fmt.Println("\n\n=== Testing Hypothesis ===")
	fmt.Println("If Channel 1 byte stream is [08 08 08...] and AY Reg 1 = 08,")
	fmt.Println("then the format is: Each channel outputs one byte per frame directly to its register")
}
