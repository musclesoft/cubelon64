// Quick standalone program to dump pattern table at frame 920
// We need to see ALL 8 addresses the drawing routine uses

package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

func main() {
	// Read Bank 2 data (pattern table is in Bank 2)
	bankData, err := os.ReadFile("../emulator/final_banks/bank_2.bin")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Printf("Note: This needs bank dumps. Run emulator first to generate them.\n")
		return
	}

	// Pattern table at $84C0, which is $84C0 - $8000 = $04C0 into Bank 2
	tableOffset := 0x04C0

	fmt.Printf("Pattern table at $84C0 (Bank 2 offset $%04X):\n\n", tableOffset)

	// The drawing routine does DJNZ 8 times, reading 2 bytes each time
	// So it reads 16 bytes total (8 addresses) from the pattern table
	for i := 0; i < 8; i++ {
		offset := tableOffset + i*2
		lo := bankData[offset]
		hi := bankData[offset+1]
		addr := uint16(lo) | (uint16(hi) << 8)

		fmt.Printf("Entry %d: $%04X", i, addr)

		// Determine which bank this address is in
		if addr >= 0xC000 {
			fmt.Printf(" (Bank 2 offset $%04X)", addr-0xC000)
		} else if addr >= 0x8000 {
			fmt.Printf(" (Bank 2 offset $%04X)", addr-0x8000)
		}
		fmt.Printf("\n")
	}

	// Calculate spacing
	fmt.Printf("\nAddress spacing:\n")
	addrs := make([]uint16, 8)
	for i := 0; i < 8; i++ {
		offset := tableOffset + i*2
		lo := bankData[offset]
		hi := bankData[offset+1]
		addrs[i] = uint16(lo) | (uint16(hi) << 8)
	}

	for i := 1; i < 8; i++ {
		diff := int(addrs[i]) - int(addrs[i-1])
		fmt.Printf("  Entry %d - Entry %d = %+d bytes ($%04X)\n", i, i-1, diff, uint16(diff))
	}
}
