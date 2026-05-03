// Extract music data from CUBELON memory dump
package main

import (
	"fmt"
	"os"
)

func main() {
	data, err := os.ReadFile("../emulator/memory_frame300.bin")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("=== CUBELON Music Data Extraction ===\n")

	// The music player is at $7EC5-$7F9E
	playerStart := 0x7EC5
	playerEnd := 0x7F9E
	playerSize := playerEnd - playerStart + 1

	fmt.Printf("Music Player Code:\n")
	fmt.Printf("  Address: $%04X-$%04X\n", playerStart, playerEnd)
	fmt.Printf("  Size: %d bytes\n\n", playerSize)

	// Pattern sequence data is at $C000
	// From disassembly: reads from $C000-$C00A and beyond
	fmt.Printf("Pattern Sequence Data:\n")
	fmt.Printf("  Address: $C000+ (in banked RAM)\n")

	// Show first pattern sequence
	seqStart := 0xC000
	fmt.Printf("\n  First 256 bytes:\n")
	for i := 0; i < 256; i += 16 {
		fmt.Printf("    %04X:", seqStart+i)
		for j := 0; j < 16; j++ {
			fmt.Printf(" %02X", data[seqStart+i+j])
		}
		fmt.Print("  ")
		for j := 0; j < 16; j++ {
			b := data[seqStart+i+j]
			if b >= 32 && b < 127 {
				fmt.Printf("%c", b)
			} else {
				fmt.Print(".")
			}
		}
		fmt.Println()
	}

	// Channel data pointers loaded at $7F11-$7F46
	// 14 pointers stored to: $7D45, $7D52, $7D5F, $7D6C, $7D79, $7D86,
	//                        $7D93, $7DA0, $7DAD, $7DBA, $7DC7, $7DD4,
	//                        $7DE1, $7DED

	channelPtrAddrs := []uint16{
		0x7D45, 0x7D52, 0x7D5F, 0x7D6C, 0x7D79, 0x7D86,
		0x7D93, 0x7DA0, 0x7DAD, 0x7DBA, 0x7DC7, 0x7DD4,
		0x7DE1, 0x7DED,
	}

	fmt.Printf("\n\nChannel Data Pointers (from memory at frame 300):\n")
	for i, addr := range channelPtrAddrs {
		lo := data[addr]
		hi := data[addr+1]
		ptr := uint16(lo) | (uint16(hi) << 8)
		fmt.Printf("  Channel %2d ($%04X): $%04X\n", i, addr, ptr)
	}

	// Try to identify pattern data tables
	fmt.Printf("\n\nSearching for pattern data regions...\n")

	// Pattern data starts after sequence (look for pointer table)
	// Each pattern is referenced by sequence bytes as indices

	// The tick counter address
	fmt.Printf("\nTick Counter: $7EC4 = %d\n", data[0x7EC4])

	// Bank switching value
	fmt.Printf("Bank Switch: $7D35 = $%02X\n", data[0x7D35])

	// Export player code
	fmt.Printf("\n\nExporting player code...\n")
	f, err := os.Create("music_player.bin")
	if err == nil {
		f.Write(data[playerStart : playerEnd+1])
		f.Close()
		fmt.Printf("✓ Saved music_player.bin (%d bytes)\n", playerSize)
	}

	// Export sequence data
	fmt.Printf("\nExporting pattern sequence data...\n")
	// Find end of sequence (look for FF marker or length from analysis)
	seqEnd := seqStart
	for seqEnd < len(data) && data[seqEnd] != 0xFF {
		seqEnd++
	}
	if seqEnd >= len(data) {
		seqEnd = seqStart + 512 // Fallback
	}
	seqSize := seqEnd - seqStart

	f, err = os.Create("pattern_sequence.bin")
	if err == nil {
		f.Write(data[seqStart:seqEnd])
		f.Close()
		fmt.Printf("✓ Saved pattern_sequence.bin (%d bytes)\n", seqSize)
	}

	// Calculate total music data size
	fmt.Printf("\n=== Size Summary ===\n")
	fmt.Printf("Player code: %d bytes\n", playerSize)
	fmt.Printf("Pattern sequence: %d bytes\n", seqSize)
	fmt.Printf("Pattern data: (need to trace from sequence)\n")
	fmt.Printf("Frequency tables: (need to identify)\n")
	fmt.Printf("\nNext: Need to dump all RAM banks to get complete pattern data\n")
}
