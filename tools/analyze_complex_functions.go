// Analyze Complex patterns to find underlying mathematical functions
package main

import (
	"fmt"
	"math"
)

func AnalyzeComplexFunctions(notes []NoteWithMetadata) {
	fmt.Printf("\n=== Analyzing Complex Pattern Functions ===\n")

	complexPatterns := [][]uint16{}

	for _, note := range notes {
		if note.isRest || len(note.freqSeq) <= 1 {
			continue
		}

		// Check if frequency changes
		hasChange := false
		for i := 1; i < len(note.freqSeq); i++ {
			if note.freqSeq[i] != note.freqSeq[0] {
				hasChange = true
				break
			}
		}
		if !hasChange {
			continue
		}

		// Check if it matches drum templates (skip those)
		matchesDrum := false
		drumTemplates := [][]uint16{
			{2831, 2047, 1603, 1209, 1039, 911, 811, 665, 610, 524},
			{39403, 59104, 52673, 52673, 52673, 52673, 52673, 52673, 52673, 52673},
			{2552, 1897, 1509, 1155, 999, 886, 793, 653, 599, 517},
			{14776, 2993, 2295, 13134, 14776},
		}

		for _, template := range drumTemplates {
			if len(note.freqSeq) >= len(template) {
				matches := true
				for i := 0; i < len(template); i++ {
					if note.freqSeq[i] != template[i] {
						matches = false
						break
					}
				}
				if matches {
					matchesDrum = true
					break
				}
			}
		}

		if matchesDrum {
			continue
		}

		// This is a Complex pattern
		complexPatterns = append(complexPatterns, note.freqSeq)
		if len(complexPatterns) >= 50 {
			break // Analyze first 50
		}
	}

	fmt.Printf("Found %d Complex patterns (analyzing first 50)\n", len(complexPatterns))

	// Categorize by pattern type
	linearCount := 0
	exponentialCount := 0
	logarithmicCount := 0
	vibratoCount := 0
	stepCount := 0
	randomCount := 0

	for i, pattern := range complexPatterns {
		if i >= 50 {
			break
		}

		// Analyze this pattern
		analysis := analyzePattern(pattern)

		if i < 10 {
			fmt.Printf("\nPattern %d: len=%d, type=%s\n", i, len(pattern), analysis.Type)
			fmt.Printf("  First 10 values: ")
			for j := 0; j < min(10, len(pattern)); j++ {
				fmt.Printf("%d ", pattern[j])
			}
			fmt.Printf("\n")
			fmt.Printf("  %s\n", analysis.Description)
		}

		switch analysis.Type {
		case "linear":
			linearCount++
		case "exponential":
			exponentialCount++
		case "logarithmic":
			logarithmicCount++
		case "vibrato":
			vibratoCount++
		case "step":
			stepCount++
		default:
			randomCount++
		}
	}

	fmt.Printf("\n=== Classification Summary ===\n")
	total := len(complexPatterns)
	fmt.Printf("Linear:       %3d (%.1f%%) - constant delta per frame\n", linearCount, float64(linearCount)*100.0/float64(total))
	fmt.Printf("Exponential:  %3d (%.1f%%) - geometric progression\n", exponentialCount, float64(exponentialCount)*100.0/float64(total))
	fmt.Printf("Logarithmic:  %3d (%.1f%%) - log curve\n", logarithmicCount, float64(logarithmicCount)*100.0/float64(total))
	fmt.Printf("Vibrato:      %3d (%.1f%%) - oscillating\n", vibratoCount, float64(vibratoCount)*100.0/float64(total))
	fmt.Printf("Step:         %3d (%.1f%%) - discrete steps/holds\n", stepCount, float64(stepCount)*100.0/float64(total))
	fmt.Printf("Random/Other: %3d (%.1f%%) - no clear pattern\n", randomCount, float64(randomCount)*100.0/float64(total))

	// Analyze length distribution
	fmt.Printf("\n=== Length Distribution ===\n")
	lengthDist := make(map[int]int)
	for _, pattern := range complexPatterns {
		lengthDist[len(pattern)]++
	}

	lengths := []int{}
	for length := range lengthDist {
		lengths = append(lengths, length)
	}
	// Sort
	for i := 0; i < len(lengths); i++ {
		for j := i + 1; j < len(lengths); j++ {
			if lengths[j] < lengths[i] {
				lengths[i], lengths[j] = lengths[j], lengths[i]
			}
		}
	}

	for _, length := range lengths {
		count := lengthDist[length]
		fmt.Printf("  len=%2d: %3d patterns (%.1f%%)\n", length, count, float64(count)*100.0/float64(total))
	}

	// Analyze delta patterns
	fmt.Printf("\n=== Delta Analysis ===\n")
	analyzeDeltaPatterns(complexPatterns[:min(20, len(complexPatterns))])

	// Check for near-duplicates and variations of drum templates
	fmt.Printf("\n=== Near-Duplicate and Template Variation Analysis ===\n")
	analyzeNearDuplicates(complexPatterns)
}

type PatternAnalysis struct {
	Type        string
	Description string
	Error       float64
}

func analyzePattern(freqs []uint16) PatternAnalysis {
	if len(freqs) <= 1 {
		return PatternAnalysis{Type: "constant", Description: "Single value"}
	}

	// Calculate deltas
	deltas := make([]int16, len(freqs)-1)
	for i := 1; i < len(freqs); i++ {
		deltas[i-1] = int16(freqs[i]) - int16(freqs[i-1])
	}

	// Check for constant delta (linear)
	if checkConstantDelta(deltas) {
		return PatternAnalysis{
			Type:        "linear",
			Description: fmt.Sprintf("Linear: constant delta %+d per frame", deltas[0]),
		}
	}

	// Check for step pattern (values hold for multiple frames)
	if checkStepPattern(freqs) {
		return PatternAnalysis{
			Type:        "step",
			Description: "Step function: values hold for multiple frames",
		}
	}

	// Check for vibrato (oscillating around center)
	if checkVibratoPattern(freqs) {
		return PatternAnalysis{
			Type:        "vibrato",
			Description: "Vibrato: oscillating around center frequency",
		}
	}

	// Check for exponential
	expError := checkExponentialPattern(freqs)
	if expError < 0.10 {
		return PatternAnalysis{
			Type:        "exponential",
			Description: fmt.Sprintf("Exponential: geometric progression (error %.1f%%)", expError*100),
			Error:       expError,
		}
	}

	// Check for logarithmic
	logError := checkLogarithmicPattern(freqs)
	if logError < 0.10 {
		return PatternAnalysis{
			Type:        "logarithmic",
			Description: fmt.Sprintf("Logarithmic: log curve (error %.1f%%)", logError*100),
			Error:       logError,
		}
	}

	return PatternAnalysis{
		Type:        "random",
		Description: "No clear mathematical pattern",
	}
}

func checkConstantDelta(deltas []int16) bool {
	if len(deltas) == 0 {
		return true
	}

	first := deltas[0]
	for _, d := range deltas[1:] {
		if d != first {
			return false
		}
	}
	return true
}

func checkStepPattern(freqs []uint16) bool {
	// Check if values hold for at least 2 frames
	holds := 0
	for i := 1; i < len(freqs); i++ {
		if freqs[i] == freqs[i-1] {
			holds++
		}
	}
	// If > 30% of frames hold the same value, it's a step pattern
	return holds > len(freqs)/3
}

func checkVibratoPattern(freqs []uint16) bool {
	if len(freqs) < 8 {
		return false
	}

	// Calculate center frequency
	sum := uint32(0)
	for _, f := range freqs {
		sum += uint32(f)
	}
	center := uint16(sum / uint32(len(freqs)))

	// Check if oscillates around center
	crossings := 0
	lastAbove := freqs[0] > center
	for i := 1; i < len(freqs); i++ {
		above := freqs[i] > center
		if above != lastAbove {
			crossings++
			lastAbove = above
		}
	}

	// Vibrato should cross center multiple times
	return crossings >= 4
}

func checkExponentialPattern(freqs []uint16) float64 {
	if len(freqs) < 3 {
		return 1.0
	}

	start := float64(freqs[0])
	end := float64(freqs[len(freqs)-1])

	if start == 0 || end == 0 {
		return 1.0
	}

	// Calculate geometric ratio: r = (end/start)^(1/(n-1))
	ratio := math.Pow(end/start, 1.0/float64(len(freqs)-1))

	// Calculate error
	maxError := 0.0
	for i := 0; i < len(freqs); i++ {
		predicted := start * math.Pow(ratio, float64(i))
		actual := float64(freqs[i])
		error := math.Abs(predicted - actual)
		relError := error / actual
		if relError > maxError {
			maxError = relError
		}
	}

	return maxError
}

func checkLogarithmicPattern(freqs []uint16) float64 {
	if len(freqs) < 3 {
		return 1.0
	}

	start := float64(freqs[0])
	end := float64(freqs[len(freqs)-1])

	// Calculate error for log curve
	maxError := 0.0
	for i := 0; i < len(freqs); i++ {
		t := float64(i) / float64(len(freqs)-1)
		logT := math.Log(1.0+9.0*t) / math.Log(10.0)
		predicted := start + (end-start)*logT
		actual := float64(freqs[i])
		error := math.Abs(predicted - actual)
		relError := error / actual
		if relError > maxError {
			maxError = relError
		}
	}

	return maxError
}

func analyzeDeltaPatterns(patterns [][]uint16) {
	fmt.Printf("Analyzing delta sequences for first 20 patterns:\n")

	for i, pattern := range patterns {
		if len(pattern) <= 1 {
			continue
		}

		deltas := make([]int16, len(pattern)-1)
		for j := 1; j < len(pattern); j++ {
			deltas[j-1] = int16(pattern[j]) - int16(pattern[j-1])
		}

		// Find most common delta
		deltaCount := make(map[int16]int)
		for _, d := range deltas {
			deltaCount[d]++
		}

		mostCommon := int16(0)
		maxCount := 0
		for delta, count := range deltaCount {
			if count > maxCount {
				maxCount = count
				mostCommon = delta
			}
		}

		fmt.Printf("  Pattern %d: ", i)
		if maxCount == len(deltas) {
			fmt.Printf("constant delta %+d\n", mostCommon)
		} else if maxCount > len(deltas)*2/3 {
			fmt.Printf("mostly delta %+d (%d/%d frames)\n", mostCommon, maxCount, len(deltas))
		} else {
			fmt.Printf("mixed deltas, most common %+d (%d/%d)\n", mostCommon, maxCount, len(deltas))
			// Show first few deltas
			fmt.Printf("    Sequence: ")
			for j := 0; j < min(10, len(deltas)); j++ {
				fmt.Printf("%+d ", deltas[j])
			}
			fmt.Printf("\n")
		}
	}
}

func analyzeNearDuplicates(patterns [][]uint16) {
	// Group patterns by signature (first 5 values)
	signatureGroups := make(map[string][]int)

	for i, pattern := range patterns {
		if len(pattern) < 5 {
			continue
		}

		sig := fmt.Sprintf("%d,%d,%d,%d,%d", pattern[0], pattern[1], pattern[2], pattern[3], pattern[4])
		signatureGroups[sig] = append(signatureGroups[sig], i)
	}

	// Find groups with multiple patterns
	fmt.Printf("Patterns with identical first 5 values:\n")
	duplicateCount := 0
	for sig, indices := range signatureGroups {
		if len(indices) > 1 {
			duplicateCount += len(indices)
			fmt.Printf("  Signature %s: %d patterns (indices: ", sig, len(indices))
			for j, idx := range indices {
				if j > 0 {
					fmt.Printf(", ")
				}
				fmt.Printf("%d", idx)
				if j >= 9 {
					fmt.Printf("...")
					break
				}
			}
			fmt.Printf(")\n")

			// Show full sequences for first group
			if duplicateCount <= 10 {
				for _, idx := range indices[:min(3, len(indices))] {
					fmt.Printf("    Pattern %d: ", idx)
					for j := 0; j < len(patterns[idx]); j++ {
						fmt.Printf("%d ", patterns[idx][j])
					}
					fmt.Printf("\n")
				}
			}
		}
	}

	fmt.Printf("\nTotal near-duplicate patterns: %d / %d (%.1f%%)\n",
		duplicateCount, len(patterns), float64(duplicateCount)*100.0/float64(len(patterns)))

	// Check similarity to drum templates (within threshold)
	fmt.Printf("\n=== Similarity to Drum Templates ===\n")
	drumTemplates := [][]uint16{
		{2831, 2047, 1603, 1209, 1039, 911, 811, 665, 610, 524},
		{39403, 59104, 52673, 52673, 52673, 52673, 52673, 52673, 52673, 52673},
		{2552, 1897, 1509, 1155, 999, 886, 793, 653, 599, 517},
		{14776, 2993, 2295, 13134, 14776},
	}
	drumNames := []string{"Drum Sweep 1", "Noise Burst", "Drum Sweep 2", "Modulation"}

	for templateIdx, template := range drumTemplates {
		similarCount := 0

		for i, pattern := range patterns {
			similarity := calculateSimilarity(pattern, template)
			if similarity > 0.80 && similarity < 1.0 { // 80% similar but not exact match
				similarCount++
				if similarCount <= 3 {
					fmt.Printf("  Pattern %d is %.1f%% similar to %s\n", i, similarity*100, drumNames[templateIdx])
					fmt.Printf("    Template: ")
					for j := 0; j < min(10, len(template)); j++ {
						fmt.Printf("%d ", template[j])
					}
					fmt.Printf("\n")
					fmt.Printf("    Pattern:  ")
					for j := 0; j < min(10, len(pattern)); j++ {
						fmt.Printf("%d ", pattern[j])
					}
					fmt.Printf("\n")
				}
			}
		}

		if similarCount > 0 {
			fmt.Printf("  → %d patterns are 80-99%% similar to %s\n", similarCount, drumNames[templateIdx])
		}
	}

	// Encoding recommendation
	fmt.Printf("\n=== Encoding Recommendations ===\n")
	fmt.Printf("If near-duplicates use reference + offset encoding:\n")
	fmt.Printf("  Instead of: 50 bytes per pattern\n")
	fmt.Printf("  Use:        2 bytes (base pattern index + variant byte)\n")
	fmt.Printf("  Savings:    48 bytes × %d patterns = %d bytes\n", duplicateCount, 48*duplicateCount)
}

func calculateSimilarity(pattern1, pattern2 []uint16) float64 {
	minLen := min(len(pattern1), len(pattern2))
	if minLen == 0 {
		return 0.0
	}

	matches := 0
	for i := 0; i < minLen; i++ {
		// Allow small differences (within 5%)
		diff := float64(pattern1[i]) - float64(pattern2[i])
		if diff < 0 {
			diff = -diff
		}

		avg := float64(pattern1[i]+pattern2[i]) / 2.0
		if avg > 0 && diff/avg < 0.05 {
			matches++
		} else if diff == 0 {
			matches++
		}
	}

	return float64(matches) / float64(minLen)
}
