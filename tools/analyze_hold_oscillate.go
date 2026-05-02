// Analyze hold+oscillate patterns (delayed vibrato)
package main

import (
	"fmt"
)

func AnalyzeHoldOscillate(notes []NoteWithMetadata) {
	fmt.Printf("\n=== Analyzing Hold+Oscillate Patterns ===\n")

	holdOscillatePatterns := []struct {
		pattern     []uint16
		holdFrames  int
		baseFreq    uint16
		oscPattern  []int16
	}{}

	for _, note := range notes {
		if note.isRest || len(note.freqSeq) <= 10 {
			continue
		}

		// Check if pattern starts with multiple frames at same frequency
		holdFrames := 0
		if len(note.freqSeq) > 1 {
			baseFreq := note.freqSeq[0]
			for i := 1; i < len(note.freqSeq); i++ {
				if note.freqSeq[i] == baseFreq {
					holdFrames++
				} else {
					break
				}
			}
		}

		// Need at least 10 frames of hold
		if holdFrames < 10 {
			continue
		}

		// After hold, check if it oscillates
		baseFreq := note.freqSeq[0]
		oscStart := holdFrames + 1

		if oscStart >= len(note.freqSeq) {
			continue
		}

		// Extract oscillation pattern (deltas from base)
		oscPattern := []int16{}
		for i := oscStart; i < len(note.freqSeq) && i < oscStart+20; i++ {
			delta := int16(note.freqSeq[i]) - int16(baseFreq)
			oscPattern = append(oscPattern, delta)
		}

		// Check if oscillation is periodic
		if len(oscPattern) >= 8 {
			holdOscillatePatterns = append(holdOscillatePatterns, struct {
				pattern     []uint16
				holdFrames  int
				baseFreq    uint16
				oscPattern  []int16
			}{
				pattern:    note.freqSeq,
				holdFrames: holdFrames,
				baseFreq:   baseFreq,
				oscPattern: oscPattern,
			})

			if len(holdOscillatePatterns) <= 10 {
				fmt.Printf("\nPattern %d: hold %d frames at %d, then oscillate\n",
					len(holdOscillatePatterns), holdFrames, baseFreq)
				fmt.Printf("  Oscillation pattern (deltas from base): ")
				for i := 0; i < min(16, len(oscPattern)); i++ {
					fmt.Printf("%+d ", oscPattern[i])
				}
				fmt.Printf("\n")

				// Check if periodic
				period := detectOscillationPeriod(oscPattern)
				if period > 0 {
					fmt.Printf("  → Periodic with period %d: ", period)
					for i := 0; i < period; i++ {
						fmt.Printf("%+d ", oscPattern[i])
					}
					fmt.Printf("(repeats)\n")
				}
			}
		}
	}

	fmt.Printf("\nFound %d hold+oscillate patterns\n", len(holdOscillatePatterns))

	// Analyze common oscillation patterns
	if len(holdOscillatePatterns) > 0 {
		fmt.Printf("\n=== Common Oscillation Patterns ===\n")
		analyzeOscillationPatterns(holdOscillatePatterns)
	}

	// Encoding recommendation
	if len(holdOscillatePatterns) > 0 {
		fmt.Printf("\n=== Encoding Recommendation ===\n")
		fmt.Printf("Pattern: Hold + Delayed Vibrato\n")
		fmt.Printf("  Encoding: baseFreq (2) + holdFrames (1) + vibratoPattern (4-8 bytes)\n")
		fmt.Printf("  Total: ~7-11 bytes per pattern\n")
		fmt.Printf("  Current: ~30-50 bytes per pattern (Complex encoding)\n")
		fmt.Printf("  Savings: ~25-40 bytes × %d patterns = %d-%d bytes\n",
			len(holdOscillatePatterns),
			25*len(holdOscillatePatterns),
			40*len(holdOscillatePatterns))
	}
}

func detectOscillationPeriod(deltas []int16) int {
	// Try periods 2-16
	for period := 2; period <= 16 && period < len(deltas)/2; period++ {
		matches := true
		for i := period; i < len(deltas); i++ {
			if deltas[i] != deltas[i%period] {
				matches = false
				break
			}
		}
		if matches {
			return period
		}
	}
	return 0
}

func analyzeOscillationPatterns(patterns []struct {
	pattern     []uint16
	holdFrames  int
	baseFreq    uint16
	oscPattern  []int16
}) {
	// Group by oscillation signature (first 8 deltas)
	signatures := make(map[string]int)

	for _, p := range patterns {
		if len(p.oscPattern) < 8 {
			continue
		}

		sig := fmt.Sprintf("%d,%d,%d,%d,%d,%d,%d,%d",
			p.oscPattern[0], p.oscPattern[1], p.oscPattern[2], p.oscPattern[3],
			p.oscPattern[4], p.oscPattern[5], p.oscPattern[6], p.oscPattern[7])
		signatures[sig]++
	}

	// Show most common signatures
	type sigCount struct {
		sig   string
		count int
	}
	counts := []sigCount{}
	for sig, count := range signatures {
		counts = append(counts, sigCount{sig, count})
	}

	// Sort by count
	for i := 0; i < len(counts); i++ {
		for j := i + 1; j < len(counts); j++ {
			if counts[j].count > counts[i].count {
				counts[i], counts[j] = counts[j], counts[i]
			}
		}
	}

	fmt.Printf("Most common oscillation patterns (first 8 deltas):\n")
	for i := 0; i < min(5, len(counts)); i++ {
		fmt.Printf("  %s: %d patterns\n", counts[i].sig, counts[i].count)
	}

	if len(counts) > 0 {
		fmt.Printf("\nUnique oscillation patterns: %d\n", len(counts))
		if len(counts) <= 3 {
			fmt.Printf("→ Can store %d oscillation templates (4-8 bytes each)\n", len(counts))
		}
	}
}
