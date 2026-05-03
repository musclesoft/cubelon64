// Analyze the complete music data format from memory dump
package main

import (
	"fmt"
	"os"
	"sort"
)

func main() {
	data, err := os.ReadFile("../emulator/memory_frame300.bin")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("=== CUBELON Music Format Analysis ===\n")

	// Channel data pointers from frame 300
	channelAddrs := []uint16{
		0xC792, 0xC947, 0xC7A6, 0xC95B, 0xC8FA, 0xCABB, 0xCABB,
		0xC997, 0xCABB, 0xCA3C, 0xCA94, 0xC9BF, 0xCABB, 0xC9D4,
	}

	// Find min/max addresses to determine data region
	minAddr := uint16(0xFFFF)
	maxAddr := uint16(0)
	for _, addr := range channelAddrs {
		if addr < minAddr {
			minAddr = addr
		}
		if addr > maxAddr {
			maxAddr = addr
		}
	}

	// Scan forward from each channel to find data size
	// Pattern data format (from disassembly):
	// - Bit 7 set: timing/wait command
	// - Bit 6 set: special command
	// - Bit 5 set: end marker
	// - Bits 0-2: channel/register number

	fmt.Printf("Channel Data Analysis:\n\n")

	totalBytes := 0
	uniqueAddrs := make(map[uint16]bool)

	for i, addr := range channelAddrs {
		fmt.Printf("Channel %2d at $%04X: ", i, addr)

		// Scan for data (simplified - look for end pattern)
		ptr := int(addr)
		bytes := 0

		// Scan up to 256 bytes or until we hit known end marker
		for bytes < 256 && ptr < len(data) {
			b := data[ptr]

			// Bit 5 set = end marker (from disassembly at 7F61)
			if (b & 0x20) != 0 {
				bytes++
				break
			}

			uniqueAddrs[uint16(ptr)] = true
			bytes++
			ptr++

			// If byte has bit 7 or 6, it's followed by additional data
			if (b & 0x80) != 0 {
				// Timing command - followed by 2 bytes (16-bit wait)
				ptr += 2
				bytes += 2
			} else if (b & 0x40) != 0 {
				// Effect command - followed by 2 bytes
				ptr += 2
				bytes += 2
			} else {
				// Note/register write - followed by 2 bytes (reg, value)
				ptr += 2
				bytes += 2
			}
		}

		fmt.Printf("%4d bytes\n", bytes)
		totalBytes += bytes
	}

	dataSize := len(uniqueAddrs)

	fmt.Printf("\nTotal pattern data (unique addresses): %d bytes\n", dataSize)
	fmt.Printf("Data region: $%04X-$%04X\n", minAddr, maxAddr)

	// Analyze pattern sequence
	seqStart := 0xC000
	seqEnd := seqStart
	for seqEnd < len(data) && data[seqEnd] != 0xFF {
		seqEnd++
	}
	seqSize := seqEnd - seqStart

	fmt.Printf("\n=== Complete Size Breakdown ===\n")
	fmt.Printf("Music Player Code:    218 bytes ($7EC5-$7F9E)\n")
	fmt.Printf("Pattern Sequence:     %d bytes ($C000-$%04X)\n", seqSize, seqStart+seqSize-1)
	fmt.Printf("Pattern Data:         ~%d bytes ($%04X-$%04X)\n", dataSize, minAddr, maxAddr)

	// Look for frequency tables
	fmt.Printf("\n=== Searching for Frequency Tables ===\n")

	// AY frequency tables are typically 12-bit values
	// Look for sequences of 16-bit values in reasonable range (100-2000)

	findFreqTable := func(start, size int, name string) {
		validCount := 0
		for i := 0; i < size; i += 2 {
			if start+i+1 >= len(data) {
				break
			}
			val := uint16(data[start+i]) | (uint16(data[start+i+1]) << 8)
			if val >= 100 && val <= 2000 {
				validCount++
			}
		}
		if validCount > 10 {
			fmt.Printf("  Possible table at $%04X: %d/%d values in range\n",
				start, validCount, size/2)
		}
	}

	// Search common regions
	for addr := 0x7000; addr < 0x9000; addr += 2 {
		findFreqTable(addr, 96, "freq")
	}

	// Analyze note data bytes
	fmt.Printf("\n=== Note Data Analysis ===\n")

	noteBytes := make(map[byte]int)
	for addr := minAddr; addr <= maxAddr && int(addr) < len(data); addr++ {
		b := data[addr]
		// High nibble E or F indicates note data
		if (b & 0xF0) == 0xE0 || (b & 0xF0) == 0xF0 {
			noteBytes[b]++
		}
	}

	// Sort by frequency
	type nb struct {
		b byte
		c int
	}
	notes := []nb{}
	for b, c := range noteBytes {
		notes = append(notes, nb{b, c})
	}
	sort.Slice(notes, func(i, j int) bool {
		return notes[i].c > notes[j].c
	})

	fmt.Printf("Most common note bytes (top 20):\n")
	for i := 0; i < 20 && i < len(notes); i++ {
		fmt.Printf("  $%02X: %3d times\n", notes[i].b, notes[i].c)
	}

	fmt.Printf("\n=== TOTAL MUSIC DATA SIZE ===\n")
	totalSize := 218 + seqSize + dataSize
	fmt.Printf("Player + Sequence + Patterns = %d bytes\n", totalSize)

	// Export the complete pattern data region
	fmt.Printf("\n=== Exporting Pattern Data ===\n")
	patternSize := int(maxAddr - minAddr + 256) // Add buffer
	f, err := os.Create("pattern_data.bin")
	if err == nil {
		f.Write(data[minAddr : int(minAddr)+patternSize])
		f.Close()
		fmt.Printf("✓ Saved pattern_data.bin (%d bytes from $%04X)\n", patternSize, minAddr)
	}
}
