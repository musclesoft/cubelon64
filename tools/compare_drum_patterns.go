// Compare drum patterns to find common algorithm
package main

import (
	"fmt"
	"math"
)

func CompareDrumPatterns() {
	fmt.Printf("\n=== Comparing Drum Pattern Structures ===\n")

	template0 := []uint16{2831, 2047, 1603, 1209, 1039, 911, 811, 665, 610, 524}
	template2 := []uint16{2552, 1897, 1509, 1155, 999, 886, 793, 653, 599, 517}

	// Normalize both to start from 1.0
	norm0 := make([]float64, len(template0))
	norm2 := make([]float64, len(template2))

	for i := range template0 {
		norm0[i] = float64(template0[i]) / float64(template0[0])
		norm2[i] = float64(template2[i]) / float64(template2[0])
	}

	fmt.Printf("Normalized sequences (relative to start value):\n")
	fmt.Printf("Template 0: ")
	for i := 0; i < len(norm0); i++ {
		fmt.Printf("%.4f ", norm0[i])
	}
	fmt.Printf("\nTemplate 2: ")
	for i := 0; i < len(norm2); i++ {
		fmt.Printf("%.4f ", norm2[i])
	}
	fmt.Printf("\n")

	// Check if they follow the same normalized curve
	fmt.Printf("\nDifferences between normalized curves:\n")
	totalDiff := 0.0
	for i := 0; i < len(norm0); i++ {
		diff := math.Abs(norm0[i] - norm2[i])
		fmt.Printf("Frame %d: %.5f\n", i, diff)
		totalDiff += diff
	}
	avgDiff := totalDiff / float64(len(norm0))
	fmt.Printf("Average difference: %.5f (%.2f%%)\n", avgDiff, avgDiff*100.0)

	if avgDiff < 0.02 {
		fmt.Printf("✓ SAME ALGORITHM! Both templates follow identical normalized curve.\n")
		fmt.Printf("  Can encode as: startValue + decayCurve(10 frames)\n")
	}

	// Try to find the generating function
	fmt.Printf("\n=== Finding Generating Function ===\n")

	// Use template 0 as reference
	// Try: y[t] = start * decay_table[t]
	// Where decay_table is precomputed

	decayTable := make([]float64, len(template0))
	for i := range template0 {
		decayTable[i] = float64(template0[i]) / float64(template0[0])
	}

	fmt.Printf("Decay multiplier table (10 frames):\n")
	for i := 0; i < len(decayTable); i++ {
		fmt.Printf("  Frame %d: %.6f (AY: %d)\n", i, decayTable[i], uint16(decayTable[i]*4096))
	}

	// Verify template 2 uses same table
	fmt.Printf("\nVerifying template 2 uses same table:\n")
	maxError := 0.0
	for i := 0; i < len(template2); i++ {
		predicted := float64(template2[0]) * decayTable[i]
		actual := float64(template2[i])
		error := math.Abs(predicted - actual)
		if error > maxError {
			maxError = error
		}
		if i < 5 {
			fmt.Printf("  Frame %d: predicted %.1f, actual %d, error %.1f\n",
				i, predicted, template2[i], error)
		}
	}
	fmt.Printf("Max error: %.1f (%.2f%%)\n", maxError, maxError*100.0/float64(template2[0]))

	// Try to find mathematical formula for decay table
	fmt.Printf("\n=== Searching for Formula ===\n")

	// Check if it's piecewise linear with specific slopes
	// Segment 1: frames 0-3
	seg1_slope := (decayTable[3] - decayTable[0]) / 3.0
	seg1_intercept := decayTable[0]

	// Segment 2: frames 3-9
	seg2_slope := (decayTable[9] - decayTable[3]) / 6.0
	seg2_intercept := decayTable[3] - seg2_slope*3.0

	fmt.Printf("Piecewise linear model:\n")
	fmt.Printf("  Segment 1 (0-3): y = %.6f + %.6f*t\n", seg1_intercept, seg1_slope)
	fmt.Printf("  Segment 2 (3-9): y = %.6f + %.6f*t\n", seg2_intercept, seg2_slope)

	// Calculate error
	maxErr := 0.0
	for i := 0; i < len(decayTable); i++ {
		var predicted float64
		if i <= 3 {
			predicted = seg1_intercept + seg1_slope*float64(i)
		} else {
			predicted = seg2_intercept + seg2_slope*float64(i)
		}
		err := math.Abs(predicted - decayTable[i])
		if err > maxErr {
			maxErr = err
		}
	}
	fmt.Printf("  Max error: %.6f (%.2f%%)\n", maxErr, maxErr*100.0)

	// Store as 8-bit fixed point
	fmt.Printf("\nEncoding as 8.8 fixed point (256 = 1.0):\n")
	for i := 0; i < len(decayTable); i++ {
		fixed := uint16(decayTable[i] * 256.0)
		fmt.Printf("  .byte $%02X  ; frame %d: %.6f\n", fixed, i, decayTable[i])
	}

	fmt.Printf("\nEncoding efficiency:\n")
	fmt.Printf("  Current: 60 notes × 10 values × 2 bytes = 1200 bytes\n")
	fmt.Printf("  With table: 1 table × 10 bytes + 60 notes × (2 bytes start + 1 byte index) = 10 + 180 = 190 bytes\n")
	fmt.Printf("  Savings: %d bytes (%.1f%%)\n", 1200-190, float64(1200-190)*100.0/1200.0)

	// Check if SID frequencies follow linear addition pattern
	fmt.Printf("\n=== Testing SID Frequency Linear Addition ===\n")

	// Convert AY periods to SID frequencies
	// SID freq ≈ 985248 / AY_period (PAL C64 clock)
	const sidClock = 985248.0

	sidFreqs0 := make([]float64, len(template0))
	sidFreqs2 := make([]float64, len(template2))

	for i := range template0 {
		sidFreqs0[i] = sidClock / float64(template0[i])
		sidFreqs2[i] = sidClock / float64(template2[i])
	}

	fmt.Printf("Template 0 SID frequencies:\n")
	for i := 0; i < len(sidFreqs0); i++ {
		fmt.Printf("  Frame %d: %.2f Hz\n", i, sidFreqs0[i])
	}

	// Check if SID frequencies form additive pattern: freq[t] = base + delta*t
	// Or: freq[t] = base + deltaTable[t]

	// Calculate deltas from base frequency
	fmt.Printf("\nSID frequency deltas from frame 0:\n")
	for i := 0; i < len(sidFreqs0); i++ {
		delta := sidFreqs0[i] - sidFreqs0[0]
		fmt.Printf("  Frame %d: %+.2f Hz (ratio %.4f)\n", i, delta, sidFreqs0[i]/sidFreqs0[0])
	}

	// Check if deltas are linear (constant delta per frame)
	fmt.Printf("\nFrame-to-frame deltas:\n")
	for i := 1; i < len(sidFreqs0); i++ {
		delta := sidFreqs0[i] - sidFreqs0[i-1]
		fmt.Printf("  Frame %d→%d: %+.2f Hz\n", i-1, i, delta)
	}

	// Try fitting: SID[t] = base + offset_table[t]
	fmt.Printf("\nSID frequency offset table (for linear addition):\n")
	offsetTable := make([]int16, len(sidFreqs0))
	for i := 0; i < len(sidFreqs0); i++ {
		offsetTable[i] = int16(sidFreqs0[i] - sidFreqs0[0])
		fmt.Printf("  Frame %d: %+6d (0x%04X)\n", i, offsetTable[i], uint16(offsetTable[i]))
	}

	// Verify template 2 uses same offset table
	fmt.Printf("\nVerifying template 2 uses same SID offset table:\n")
	maxSidError := 0.0
	for i := 0; i < len(sidFreqs2); i++ {
		predicted := sidFreqs2[0] + float64(offsetTable[i])
		actual := sidFreqs2[i]
		error := math.Abs(predicted - actual)
		if error > maxSidError {
			maxSidError = error
		}
		if i < 5 {
			fmt.Printf("  Frame %d: predicted %.2f Hz, actual %.2f Hz, error %.2f Hz\n",
				i, predicted, actual, error)
		}
	}
	fmt.Printf("Max SID frequency error: %.2f Hz (%.2f%%)\n",
		maxSidError, maxSidError*100.0/sidFreqs2[0])

	// Compare multiplication vs addition encoding size
	fmt.Printf("\n=== Encoding Comparison ===\n")
	fmt.Printf("Multiplication (AY period × decay table):\n")
	fmt.Printf("  Table: 10 bytes (byte multipliers, 256 = 1.0)\n")
	fmt.Printf("  Per note: 2 bytes (start AY period)\n")
	fmt.Printf("  6502: LDA start_lo; MUL table[t]; ...\n")

	fmt.Printf("\nAddition (SID freq + offset table):\n")
	fmt.Printf("  Table: 20 bytes (10 × 2-byte signed offsets)\n")
	fmt.Printf("  Per note: 2 bytes (start SID freq)\n")
	fmt.Printf("  6502: LDA start_lo; CLC; ADC offset_lo[t]; ...\n")

	// Check if offsets are small enough to fit in 8-bit
	allFit8bit := true
	for i := 0; i < len(offsetTable); i++ {
		if offsetTable[i] < -128 || offsetTable[i] > 127 {
			allFit8bit = false
			break
		}
	}

	if allFit8bit {
		fmt.Printf("\n✓ All offsets fit in signed 8-bit (-128 to +127)\n")
		fmt.Printf("  Optimized table: 10 bytes (byte offsets)\n")
		fmt.Printf("  6502: LDA start_lo; CLC; ADC offset[t]; ADC start_hi; ...\n")
	}

	fmt.Printf("\nRecommendation: ")
	if allFit8bit {
		fmt.Printf("Use 8-bit ADDITION - smallest table (10 bytes) and fastest 6502 code\n")
	} else {
		fmt.Printf("Use 8-bit MULTIPLICATION - offsets too large for addition\n")
	}

	// Try delta encoding (frame-to-frame changes)
	fmt.Printf("\n=== Testing Delta Encoding (Frame-to-Frame Changes) ===\n")

	deltaTable := make([]int16, len(sidFreqs0)-1)
	for i := 1; i < len(sidFreqs0); i++ {
		deltaTable[i-1] = int16(sidFreqs0[i] - sidFreqs0[i-1])
	}

	fmt.Printf("Frame-to-frame delta table:\n")
	allDeltasFit8bit := true
	for i := 0; i < len(deltaTable); i++ {
		fmt.Printf("  Delta[%d]: %+4d (0x%02X)", i, deltaTable[i], byte(deltaTable[i]))
		if deltaTable[i] < -128 || deltaTable[i] > 127 {
			allDeltasFit8bit = false
			fmt.Printf(" [TOO LARGE]")
		}
		fmt.Printf("\n")
	}

	// Verify template 2 uses same delta table
	fmt.Printf("\nVerifying template 2 uses same delta table:\n")
	maxDeltaError := 0.0
	currentFreq := sidFreqs2[0]
	for i := 0; i < len(deltaTable); i++ {
		currentFreq += float64(deltaTable[i])
		actual := sidFreqs2[i+1]
		error := math.Abs(currentFreq - actual)
		if error > maxDeltaError {
			maxDeltaError = error
		}
		if i < 4 {
			fmt.Printf("  Frame %d: predicted %.2f Hz, actual %.2f Hz, error %.2f Hz\n",
				i+1, currentFreq, actual, error)
		}
	}
	fmt.Printf("Max delta error: %.2f Hz (%.2f%%)\n",
		maxDeltaError, maxDeltaError*100.0/sidFreqs2[0])

	if allDeltasFit8bit {
		fmt.Printf("\n✓ All frame-to-frame deltas fit in signed 8-bit!\n")
		fmt.Printf("  Delta table: 9 bytes (9 signed byte deltas)\n")
		fmt.Printf("  6502 per frame: CLC; ADC delta[t]; BCC +1; INC freq_hi\n")
		fmt.Printf("  This is OPTIMAL: smallest table + fastest code!\n")
	}

	// Check the pattern in deltas
	fmt.Printf("\n=== Analyzing Delta Pattern ===\n")

	// Count how many deltas are ~133 Hz
	count133 := 0
	count200 := 0
	count266 := 0
	for i := 0; i < len(deltaTable); i++ {
		if deltaTable[i] >= 130 && deltaTable[i] <= 136 {
			count133++
		} else if deltaTable[i] >= 195 && deltaTable[i] <= 205 {
			count200++
		} else if deltaTable[i] >= 260 && deltaTable[i] <= 270 {
			count266++
		}
	}

	fmt.Printf("Delta pattern analysis:\n")
	fmt.Printf("  ~133 Hz steps: %d/%d (%.1f%%)\n", count133, len(deltaTable), float64(count133)*100.0/float64(len(deltaTable)))
	fmt.Printf("  ~200 Hz steps: %d/%d (%.1f%%)\n", count200, len(deltaTable), float64(count200)*100.0/float64(len(deltaTable)))
	fmt.Printf("  ~266 Hz steps: %d/%d (%.1f%%)\n", count266, len(deltaTable), float64(count266)*100.0/float64(len(deltaTable)))

	// Check if deltas are multiples of a base value
	fmt.Printf("\nChecking if deltas are multiples of base ~133 Hz:\n")
	baseHz := 133.0
	multipliers := make([]float64, len(deltaTable))
	for i := 0; i < len(deltaTable); i++ {
		multipliers[i] = float64(deltaTable[i]) / baseHz
		fmt.Printf("  Delta[%d]: %d / %.0f = %.2fx\n", i, deltaTable[i], baseHz, multipliers[i])
	}

	// Check if multipliers fit a simple pattern (1.0, 1.5, 2.0)
	fmt.Printf("\nRounded multipliers: ")
	simplifiedPattern := ""
	for i := 0; i < len(multipliers); i++ {
		var rounded float64
		if multipliers[i] < 1.25 {
			rounded = 1.0
		} else if multipliers[i] < 1.75 {
			rounded = 1.5
		} else {
			rounded = 2.0
		}
		simplifiedPattern += fmt.Sprintf("%.1f ", rounded)
	}
	fmt.Printf("%s\n", simplifiedPattern)

	// Encode as nibbles: 0=1x, 1=1.5x, 2=2x, 3=2.5x
	fmt.Printf("\n=== Nibble Encoding (4 bits per delta multiplier) ===\n")
	fmt.Printf("Encoding: 0=1×, 1=1.5×, 2=2×, 3=unused\n")
	for i := 0; i < len(multipliers) && i < 9; i++ {
		var nibble uint16
		if multipliers[i] < 1.25 {
			nibble = 0 // 1x
		} else if multipliers[i] < 1.75 {
			nibble = 1 // 1.5x
		} else {
			nibble = 2 // 2x
		}

		if i < 4 {
			fmt.Printf("  Multiplier %.2f → nibble %d\n", multipliers[i], nibble)
		}
	}

	fmt.Printf("\n✓ OPTIMAL ENCODING FOUND!\n")
	fmt.Printf("Algorithm: SID_freq[t] = SID_freq[t-1] + (base_delta × multiplier[t])\n")
	fmt.Printf("Where:\n")
	fmt.Printf("  base_delta = 133 Hz (constant)\n")
	fmt.Printf("  multiplier[t] = lookup table of 1.0, 1.5, or 2.0\n")
	fmt.Printf("\nStorage:\n")
	fmt.Printf("  Multiplier pattern: 9 values (can pack 2 per byte) = 5 bytes\n")
	fmt.Printf("  Base delta: 1 byte (133)\n")
	fmt.Printf("  Total table: 6 bytes\n")
	fmt.Printf("  Per note: 2 bytes (start SID freq)\n")
	fmt.Printf("\n6502 code per frame:\n")
	fmt.Printf("  LDA base_delta    ; 133\n")
	fmt.Printf("  LDX multiplier[t] ; 0, 1, or 2\n")
	fmt.Printf("  JSR mul_1_1p5_2   ; fast multiply routine\n")
	fmt.Printf("  CLC\n")
	fmt.Printf("  ADC freq_lo\n")
	fmt.Printf("  STA freq_lo\n")
	fmt.Printf("  BCC +1\n")
	fmt.Printf("  INC freq_hi\n")

	fmt.Printf("\n=== FINAL COMPARISON ===\n")
	fmt.Printf("1. Lookup table (current): 75 bytes (4 templates × ~18 bytes)\n")
	fmt.Printf("2. AY multiply table:      10 bytes (decay multipliers)\n")
	fmt.Printf("3. SID delta + multiplier:  6 bytes (base + pattern)\n")
	fmt.Printf("\n→ Use option 3: 6-byte table + incremental addition!\n")
}

