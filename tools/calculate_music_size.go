// Calculate raw music data size from ZX format and converted tracker format
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("=== ZX Spectrum Music Data Size Analysis ===\n")

	// From the analysis we did with zx_music_extractor
	fmt.Println("## Original ZX Format (Estimated)")
	fmt.Println("\nBased on extracted data from AY command logs:")

	// Frequency table
	uniqueFreqs := 43
	freqTableSize := uniqueFreqs * 2 // 2 bytes per 12-bit AY period
	fmt.Printf("- Frequency table: %d entries × 2 bytes = %d bytes\n", uniqueFreqs, freqTableSize)

	// Note data (from analysis: 589 notes total)
	totalNotes := 589
	bytesPerNote := 4 // Rough estimate: freq index (1) + duration (1) + instrument (1) + effect (1)
	noteDataSize := totalNotes * bytesPerNote
	fmt.Printf("- Note data: %d notes × ~%d bytes = %d bytes (raw, uncompressed)\n",
		totalNotes, bytesPerNote, noteDataSize)

	// Instrument table (estimated ~5-10 instruments based on typical ZX music)
	instruments := 8
	bytesPerInstrument := 8 // ADSR (4) + flags (2) + vibrato (2)
	instrumentSize := instruments * bytesPerInstrument
	fmt.Printf("- Instruments: ~%d instruments × %d bytes = %d bytes\n",
		instruments, bytesPerInstrument, instrumentSize)

	// Stream sequencing (3 channels)
	// From analysis: Channel A=127, B=197, C=265 notes
	// Pattern-based would use ~1 byte per pattern reference
	streamSize := 127 + 197 + 265
	fmt.Printf("- Stream data: %d pattern references × 1 byte = %d bytes\n",
		streamSize, streamSize)

	// Effect patterns (vibrato tables, envelope shapes, etc.)
	effectSize := 100 // Rough estimate
	fmt.Printf("- Effect patterns: ~%d bytes (vibrato, portamento tables)\n", effectSize)

	rawTotal := freqTableSize + noteDataSize + instrumentSize + streamSize + effectSize
	fmt.Printf("\n**Total raw size: %d bytes (~%.1f KB)**\n", rawTotal, float64(rawTotal)/1024.0)

	// Compressed/optimized size from IMPLEMENTATION_PLAN.md
	fmt.Printf("\n**Estimated ZX music data size: %d bytes (~%.1f KB)**\n", rawTotal, float64(rawTotal)/1024.0)

	fmt.Println("\n## IMPORTANT: C64 Tracker System is Separate")
	fmt.Println("\nThe extract_tracker_format.go tool creates a DIFFERENT format:")
	fmt.Println("- NOT the original ZX format")
	fmt.Println("- New tracker format designed for C64/SID")
	fmt.Println("- Current size: 5401 bytes (5.3 KB)")
	fmt.Println("- Target: 3072 bytes (3.0 KB) with phrase-based optimization")
	fmt.Println("\nThese are TWO SEPARATE SYSTEMS:")
	fmt.Println("  1. ZX Format (this tool): ~3.1 KB - original format")
	fmt.Println("  2. C64 Tracker: 5.4 KB - new format for C64 port")

	fmt.Println("\n## Notes")
	fmt.Println("- ZX format estimate is uncompressed note-by-note data")
	fmt.Println("- Original ZX binary uses compressed data blocks (see cubelon_data_*.bin)")
	fmt.Println("- 123,027 AY register writes are generated from this compressed data")
	fmt.Println("- The exact original compression method is not fully reverse-engineered")

	// Try to detect if we're in the cubelon64 directory
	if _, err := os.Stat("../emulator/ay_commands.bin"); err == nil {
		fmt.Println("\n✓ Run zx_music_extractor.go for detailed analysis of the actual music data")
	}
}
