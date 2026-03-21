package main

import "fmt"

func main() {
	// Sequence table data from memory dump at $7A7B
	table := []byte{
		0xC3, 0x00, 0x00, 0x04, 0x79, 0xC0, 0x50, 0x00, 0x0C, 0x79, 0x40, 0x26, 0x79, 0x00, 0x00, 0x98,
		0xC1, 0x9F, 0x00, 0x2E, 0x79, 0xC4, 0x20, 0x03, 0x42, 0x79, 0x44, 0x5B, 0x79, 0xC0, 0x9F, 0x0A,
		0x63, 0x79, 0x40, 0x7C, 0x79, 0x00, 0x00, 0x90, 0x46, 0x84, 0x79, 0xC4, 0xA0, 0x0F, 0x8C, 0x79,
		0x00, 0x12, 0x90, 0xC0, 0xE0, 0x10, 0xAA, 0x79, 0x00, 0x24, 0x90, 0xC4, 0x20, 0x12, 0xCB, 0x79,
		0x00, 0x36, 0x90, 0xC0, 0x60, 0x13, 0xDF, 0x79, 0x40, 0xF9, 0x79, 0xC4, 0x9F, 0x14, 0x01, 0x7A,
		0x44, 0x09, 0x7A, 0xC0, 0x5F, 0x18, 0x15, 0x7A, 0x40, 0x21, 0x7A, 0x20, 0x00, 0x00, 0x00, 0x00,
	}

	// Known thresholds from execution trace
	knownThresholds := map[int]uint16{
		0x06: 0x0050, // offset $7A81-82, value 80
		0x11: 0x009F, // offset $7A8C-8D, value 159
		0x16: 0x0320, // offset $7A91-92, value 800
	}

	fmt.Println("=== CUBELON Sequence Table Parser ===\n")
	fmt.Println("Known thresholds from execution trace:")
	fmt.Printf("  Offset $%02X (table+%d): $%04X = %d frames (%.2fs)\n", 0x06, 0x06, knownThresholds[0x06], knownThresholds[0x06], float64(knownThresholds[0x06])/50.0)
	fmt.Printf("  Offset $%02X (table+%d): $%04X = %d frames (%.2fs)\n", 0x11, 0x11, knownThresholds[0x11], knownThresholds[0x11], float64(knownThresholds[0x11])/50.0)
	fmt.Printf("  Offset $%02X (table+%d): $%04X = %d frames (%.2fs)\n", 0x16, 0x16, knownThresholds[0x16], knownThresholds[0x16], float64(knownThresholds[0x16])/50.0)

	fmt.Println("\n=== Searching for potential thresholds ===")
	fmt.Println("(16-bit little-endian values that look like frame numbers)\n")

	// Scan for potential threshold values (reasonable frame numbers)
	// Demo runs ~9420 frames total, so thresholds should be < 10000
	for i := 0; i < len(table)-1; i++ {
		lo := table[i]
		hi := table[i+1]
		val := uint16(lo) | (uint16(hi) << 8)

		// Look for values that could be frame numbers:
		// - Between 50 and 10000
		// - Not in middle of known pointers ($79xx, $7Axx ranges)
		// - Not flag bytes (typically $C0, $C4, $40, $44, etc.)

		isLikelyThreshold := val >= 50 && val <= 10000
		isKnownPointerRange := (hi == 0x79 || hi == 0x7A) && lo < 0xF0
		isLikelyFlag := (lo >= 0xC0 || lo == 0x40 || lo == 0x44 || lo == 0x46) && hi < 0x30

		if isLikelyThreshold && !isKnownPointerRange && !isLikelyFlag {
			seconds := float64(val) / 50.0
			marker := ""
			if _, known := knownThresholds[i]; known {
				marker = " ✓ CONFIRMED"
			}
			fmt.Printf("  Offset $%02X: %02X %02X → $%04X = %5d frames (%6.2fs)%s\n",
				i, lo, hi, val, val, seconds, marker)
		}
	}

	fmt.Println("\n=== Table hex dump with known structure ===\n")
	fmt.Println("Offset  +0  +1  +2  +3  +4  +5  +6  +7  +8  +9  +A  +B  +C  +D  +E  +F")
	fmt.Println("------  --  --  --  --  --  --  --  --  --  --  --  --  --  --  --  --")

	for i := 0; i < len(table); i += 16 {
		fmt.Printf("  %02X:   ", i)
		for j := 0; j < 16 && i+j < len(table); j++ {
			fmt.Printf("%02X  ", table[i+j])
		}

		// Add annotations for known thresholds
		annotation := ""
		if i <= 0x06 && i+16 > 0x06 {
			annotation = "  ← $0050 (80)"
		} else if i <= 0x11 && i+16 > 0x11 {
			annotation = "  ← $009F (159)"
		} else if i <= 0x16 && i+16 > 0x16 {
			annotation = "  ← $0320 (800)"
		}
		fmt.Println(annotation)
	}
}
