// Analyze per-frame tone changes to find encoding patterns
package main

import (
	"fmt"
)

// AnalyzePerFramePatterns looks for structure in frame-by-frame frequency changes
func AnalyzePerFramePatterns(notes []NoteWithMetadata) {
	fmt.Printf("\n=== Per-Frame Tone Change Analysis ===\n")

	// Analyze all patterns with frame-by-frame changes
	perFrameCount := 0
	uniquePatterns := make(map[string]int)

	for _, note := range notes {
		if note.isRest || len(note.freqSeq) <= 1 {
			continue
		}

		// Check if every frame has a different frequency
		allDifferent := true
		for i := 1; i < len(note.freqSeq) && i < 3; i++ { // Check first 3 frames
			if note.freqSeq[i] == note.freqSeq[i-1] {
				allDifferent = false
				break
			}
		}

		if !allDifferent {
			continue
		}

		perFrameCount++

		// Create pattern signature (first 5 frequencies)
		sig := ""
		for i := 0; i < min(5, len(note.freqSeq)); i++ {
			if i > 0 {
				sig += ","
			}
			sig += fmt.Sprintf("%d", note.freqSeq[i])
		}

		uniquePatterns[sig]++

		// Show first few examples
		if perFrameCount <= 5 {
			fmt.Printf("\nPattern #%d:\n", perFrameCount)
			fmt.Printf("  Length: %d frames\n", len(note.freqSeq))
			fmt.Printf("  Sequence: ")
			for i := 0; i < min(10, len(note.freqSeq)); i++ {
				fmt.Printf("%d ", note.freqSeq[i])
			}
			if len(note.freqSeq) > 10 {
				fmt.Printf("...")
			}
			fmt.Printf("\n")

			// Compute deltas
			fmt.Printf("  Deltas: ")
			for i := 1; i < min(10, len(note.freqSeq)); i++ {
				delta := int(note.freqSeq[i]) - int(note.freqSeq[i-1])
				fmt.Printf("%d ", delta)
			}
			fmt.Printf("\n")

			// Check if it's a simple repeat pattern
			if len(note.freqSeq) >= 4 {
				// Check for 2-step pattern
				twoStep := true
				for i := 2; i < len(note.freqSeq); i += 2 {
					if i < len(note.freqSeq) && note.freqSeq[i] != note.freqSeq[0] {
						twoStep = false
						break
					}
					if i+1 < len(note.freqSeq) && note.freqSeq[i+1] != note.freqSeq[1] {
						twoStep = false
						break
					}
				}

				if twoStep {
					fmt.Printf("  Pattern type: TWO-STEP repeat (%d, %d)\n", note.freqSeq[0], note.freqSeq[1])
				}
			}
		}
	}

	fmt.Printf("\nPer-frame changes: %d patterns\n", perFrameCount)
	fmt.Printf("Unique signatures: %d\n", len(uniquePatterns))

	// Show most common patterns
	fmt.Printf("\nMost common signatures:\n")
	type patCount struct {
		pattern string
		count   int
	}
	counts := []patCount{}
	for pat, cnt := range uniquePatterns {
		counts = append(counts, patCount{pat, cnt})
	}

	// Simple bubble sort
	for i := 0; i < len(counts); i++ {
		for j := i + 1; j < len(counts); j++ {
			if counts[j].count > counts[i].count {
				counts[i], counts[j] = counts[j], counts[i]
			}
		}
	}

	for i := 0; i < min(10, len(counts)); i++ {
		fmt.Printf("  %s: %d occurrences\n", counts[i].pattern, counts[i].count)
	}
}

// DetectRepeatingPatterns finds repeating subsequences
func DetectRepeatingPatterns(notes []NoteWithMetadata) {
	fmt.Printf("\n=== Repeating Subsequence Detection ===\n")

	analyzed := 0
	for _, note := range notes {
		if note.isRest || len(note.freqSeq) <= 2 {
			continue
		}

		// Check if frequency changes every frame initially
		allDifferent := true
		for i := 1; i < min(3, len(note.freqSeq)); i++ {
			if note.freqSeq[i] == note.freqSeq[i-1] {
				allDifferent = false
				break
			}
		}
		if !allDifferent {
			continue
		}

		analyzed++
		if analyzed > 5 {
			break
		}

		fmt.Printf("\nAnalyzing pattern #%d:\n", analyzed)
		fmt.Printf("  Length: %d\n", len(note.freqSeq))

		// Try different period lengths
		for period := 2; period <= 5; period++ {
			matches := true
			for i := period; i < len(note.freqSeq); i++ {
				if note.freqSeq[i] != note.freqSeq[i%period] {
					matches = false
					break
				}
			}

			if matches {
				fmt.Printf("  REPEATS with period %d: ", period)
				for i := 0; i < period; i++ {
					fmt.Printf("%d ", note.freqSeq[i])
				}
				fmt.Printf("\n")
				break
			}
		}
	}
}
