// Complete music data extraction - final version
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

	fmt.Println("=== CUBELON Complete Music Extraction ===\n")

	// Music Player Code: $7EC5-$7F9E (218 bytes)
	playerStart := 0x7EC5
	playerEnd := 0x7F9E
	playerSize := playerEnd - playerStart + 1

	// Pattern Sequence: $C000-$C1B0 (432 bytes, ends at FF)
	seqStart := 0xC000
	seqEnd := seqStart
	for seqEnd < len(data) && data[seqEnd] != 0xFF {
		seqEnd++
	}
	seqSize := seqEnd - seqStart

	// Pattern Table: $C400+ (patterns are indexed by sequence bytes)
	// Each sequence byte indexes into pattern table
	// Pattern format: [tick_count] [note value reg] [note value reg] ... [00 00 00]

	// Scan from $C400 to find all pattern data
	patternStart := 0xC400
	patternEnd := patternStart

	// Find last pattern by scanning for end markers
	searching := true
	for patternEnd < 0xD000 && searching {
		// Look for pattern end marker: 00 00 00
		if patternEnd+2 < len(data) &&
			data[patternEnd] == 0x00 &&
			data[patternEnd+1] == 0x00 &&
			data[patternEnd+2] == 0x00 {
			patternEnd += 3

			// Check if next bytes look like a new pattern or end of data
			if patternEnd+4 < len(data) {
				nextTick := data[patternEnd]
				// Patterns start with tick count (typically 0x14 = 20)
				if nextTick == 0 || nextTick > 0x80 {
					searching = false
				}
			}
		} else {
			patternEnd++
		}
	}

	patternSize := patternEnd - patternStart

	// Frequency/Note tables (from disassembly analysis)
	// These are typically at $7900+ area based on memory patterns
	freqTableStart := 0x7900
	freqTableSize := 256 // Conservative estimate

	fmt.Println("=== Size Breakdown ===\n")
	fmt.Printf("1. Music Player Code:\n")
	fmt.Printf("   Address: $%04X-$%04X\n", playerStart, playerEnd)
	fmt.Printf("   Size: %d bytes\n\n", playerSize)

	fmt.Printf("2. Pattern Sequence:\n")
	fmt.Printf("   Address: $%04X-$%04X\n", seqStart, seqEnd-1)
	fmt.Printf("   Size: %d bytes\n", seqSize)
	fmt.Printf("   (Index bytes for pattern table)\n\n")

	fmt.Printf("3. Pattern Table:\n")
	fmt.Printf("   Address: $%04X-$%04X\n", patternStart, patternEnd-1)
	fmt.Printf("   Size: %d bytes\n", patternSize)
	fmt.Printf("   (Note/effect sequences for all patterns)\n\n")

	fmt.Printf("4. Frequency/Note Tables:\n")
	fmt.Printf("   Address: $%04X+ (estimated)\n", freqTableStart)
	fmt.Printf("   Size: ~%d bytes (estimated)\n\n", freqTableSize)

	totalSize := playerSize + seqSize + patternSize + freqTableSize

	fmt.Printf("=== TOTAL SIZE ===\n")
	fmt.Printf("Player: %d bytes\n", playerSize)
	fmt.Printf("Sequence: %d bytes\n", seqSize)
	fmt.Printf("Patterns: %d bytes\n", patternSize)
	fmt.Printf("Tables: ~%d bytes\n", freqTableSize)
	fmt.Printf("TOTAL: ~%d bytes\n\n", totalSize)

	// Export all components
	fmt.Println("=== Exporting Components ===\n")

	// 1. Player code
	f, err := os.Create("zx_music_player.bin")
	if err == nil {
		f.Write(data[playerStart : playerEnd+1])
		f.Close()
		fmt.Printf("✓ zx_music_player.bin (%d bytes)\n", playerSize)
	}

	// 2. Pattern sequence
	f, err = os.Create("zx_pattern_sequence.bin")
	if err == nil {
		f.Write(data[seqStart:seqEnd])
		f.Close()
		fmt.Printf("✓ zx_pattern_sequence.bin (%d bytes)\n", seqSize)
	}

	// 3. Pattern table
	f, err = os.Create("zx_pattern_table.bin")
	if err == nil {
		f.Write(data[patternStart:patternEnd])
		f.Close()
		fmt.Printf("✓ zx_pattern_table.bin (%d bytes)\n", patternSize)
	}

	// 4. Frequency tables
	f, err = os.Create("zx_freq_tables.bin")
	if err == nil {
		f.Write(data[freqTableStart : freqTableStart+freqTableSize])
		f.Close()
		fmt.Printf("✓ zx_freq_tables.bin (%d bytes)\n", freqTableSize)
	}

	// 5. Complete bundle
	f, err = os.Create("zx_music_complete.bin")
	if err == nil {
		// Write all components with headers
		fmt.Fprintf(f, "CUBELON_MUSIC_V1\n")
		fmt.Fprintf(f, "PLAYER:%04X:%04X\n", playerStart, playerSize)
		f.Write(data[playerStart : playerEnd+1])

		fmt.Fprintf(f, "SEQUENCE:%04X:%04X\n", seqStart, seqSize)
		f.Write(data[seqStart:seqEnd])

		fmt.Fprintf(f, "PATTERNS:%04X:%04X\n", patternStart, patternSize)
		f.Write(data[patternStart:patternEnd])

		fmt.Fprintf(f, "FREQTAB:%04X:%04X\n", freqTableStart, freqTableSize)
		f.Write(data[freqTableStart : freqTableStart+freqTableSize])

		f.Close()
		fmt.Printf("✓ zx_music_complete.bin (complete bundle)\n")
	}

	fmt.Println("\n=== Format Documentation ===\n")
	fmt.Println("Pattern Sequence Format:")
	fmt.Println("  - Each byte is an index into the pattern table")
	fmt.Println("  - Index calculation: offset = (index >> 3 & 0x1F) << 8 | (index & 0xE0)")
	fmt.Println("  - Base address: $C400")
	fmt.Println("  - Sequence ends with $FF marker\n")

	fmt.Println("Pattern Table Format:")
	fmt.Println("  - Each pattern: [tick_count] [data...] [00 00 00]")
	fmt.Println("  - Data bytes: note/effect commands")
	fmt.Println("  - Pattern indexed by sequence bytes\n")

	fmt.Println("Note Data Format (from pattern table):")
	fmt.Println("  - High nibble $E or $F = note data")
	fmt.Println("  - Followed by value byte and register byte")
	fmt.Println("  - Commands with bit 7 set indicate timing")
	fmt.Println("  - Patterns end with 00 00 00 marker\n")

	fmt.Println("=== Extraction Complete ===")
	fmt.Printf("Total music data extracted: ~%d bytes\n", totalSize)
}
