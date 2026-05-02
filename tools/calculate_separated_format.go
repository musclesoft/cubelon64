// Calculate optimal separated format size
package main

import (
	"fmt"
)

func CalculateSeparatedFormat(notes []NoteWithMetadata) {
	fmt.Printf("\n=== Calculating Separated Format Savings ===\n\n")

	// Separate by channel
	channels := [3][]NoteWithMetadata{}
	for _, note := range notes {
		channels[note.Channel] = append(channels[note.Channel], note)
	}

	totalCurrentBytes := 0
	totalOptimalBytes := 0

	for ch := 0; ch < 3; ch++ {
		fmt.Printf("Channel %d:\n", ch)
		channelNotes := channels[ch]

		// Current encoding: each note needs a pattern reference (1 byte)
		// Plus unique patterns need storage (~6 bytes each)
		currentBytes := len(channelNotes) // stream references

		// Count unique patterns
		uniquePatterns := make(map[string]bool)
		for _, note := range channelNotes {
			key := fmt.Sprintf("%d_%d_%d_%d", note.freq, note.dur, note.instrumentID, note.patternID)
			uniquePatterns[key] = true
		}
		currentBytes += len(uniquePatterns) * 6 // approximate pattern storage
		fmt.Printf("  Current: %d note refs + %d unique patterns × 6 = %d bytes\n",
			len(channelNotes), len(uniquePatterns), currentBytes)

		// Optimal encoding: separate melody from instrument
		// Melody sequence: (pitch, duration) pairs
		// Instrument: references to ADSR+effects

		// Count unique (pitch, duration) pairs
		uniqueMelody := make(map[string]int)
		for _, note := range channelNotes {
			if note.isRest {
				key := "rest"
				uniqueMelody[key]++
			} else {
				key := fmt.Sprintf("%d,%d", note.freq, note.dur)
				uniqueMelody[key]++
			}
		}

		// Find melody sequences (length 4-8)
		melodySeqs := make(map[string]int)
		for seqLen := 4; seqLen <= 8; seqLen++ {
			for i := 0; i+seqLen <= len(channelNotes); i++ {
				seq := ""
				for j := 0; j < seqLen; j++ {
					if j > 0 {
						seq += ";"
					}
					if channelNotes[i+j].isRest {
						seq += "rest"
					} else {
						seq += fmt.Sprintf("%d,%d", channelNotes[i+j].freq, channelNotes[i+j].dur)
					}
				}
				melodySeqs[seq]++
			}
		}

		// Find best melody sequences (repeating 3+ times, save bytes)
		type seqSavings struct {
			seq     string
			len     int
			count   int
			savings int
		}
		sequences := []seqSavings{}

		for seqLen := 8; seqLen >= 4; seqLen-- {
			for seq, count := range melodySeqs {
				// Check if this sequence is length seqLen
				parts := 0
				for _, c := range seq {
					if c == ';' {
						parts++
					}
				}
				parts++ // last part

				if parts == seqLen && count >= 3 {
					// Storage cost: seqLen × 3 bytes (freq:2 + dur:1) = table entry
					// Reference cost: 1 byte per use
					// Current cost: seqLen × 1 byte per use (if not deduplicated)
					tableCost := seqLen * 3
					refCost := count * 1
					currentCost := count * seqLen * 1
					savings := currentCost - (tableCost + refCost)
					if savings > 0 {
						sequences = append(sequences, seqSavings{seq, seqLen, count, savings})
					}
				}
			}
		}

		// Sort by savings
		for i := 0; i < len(sequences); i++ {
			for j := i + 1; j < len(sequences); j++ {
				if sequences[j].savings > sequences[i].savings {
					sequences[i], sequences[j] = sequences[j], sequences[i]
				}
			}
		}

		// Show top sequences
		fmt.Printf("  Top repeating melody sequences:\n")
		totalMelodySavings := 0
		for i := 0; i < min(5, len(sequences)); i++ {
			fmt.Printf("    Len %d × %d times: saves %d bytes\n",
				sequences[i].len, sequences[i].count, sequences[i].savings)
			totalMelodySavings += sequences[i].savings
		}

		// Optimal bytes = current - melody savings
		optimalBytes := currentBytes - totalMelodySavings
		fmt.Printf("  Optimal: %d bytes (saves %d bytes, %.1f%% reduction)\n\n",
			optimalBytes, totalMelodySavings,
			float64(totalMelodySavings)*100.0/float64(currentBytes))

		totalCurrentBytes += currentBytes
		totalOptimalBytes += optimalBytes
	}

	fmt.Printf("=== TOTAL ===\n")
	fmt.Printf("Current format: %d bytes\n", totalCurrentBytes)
	fmt.Printf("Separated format: %d bytes\n", totalOptimalBytes)
	fmt.Printf("Savings: %d bytes (%.1f%% reduction)\n",
		totalCurrentBytes-totalOptimalBytes,
		float64(totalCurrentBytes-totalOptimalBytes)*100.0/float64(totalCurrentBytes))

	// Add analysis of what else can be saved
	fmt.Printf("\n=== Additional Optimization Opportunities ===\n")

	// Channel 1 is mostly two notes - could use run-length encoding
	ch1Notes := channels[1]
	if len(ch1Notes) > 0 {
		fmt.Printf("Channel 1 (Bass):\n")
		freq6660Count := 0
		freq5298Count := 0
		for _, note := range ch1Notes {
			if note.freq == 6660 {
				freq6660Count++
			} else if note.freq == 5298 {
				freq5298Count++
			}
		}
		fmt.Printf("  Note 6660: %d times (%.1f%%)\n", freq6660Count,
			float64(freq6660Count)*100.0/float64(len(ch1Notes)))
		fmt.Printf("  Note 5298: %d times (%.1f%%)\n", freq5298Count,
			float64(freq5298Count)*100.0/float64(len(ch1Notes)))
		fmt.Printf("  → Could use 2-note alternation encoding\n")
		fmt.Printf("  → Current: %d notes × 1 byte = %d bytes\n", len(ch1Notes), len(ch1Notes))
		fmt.Printf("  → Optimal: ~50 bytes (run-length or pattern encoding)\n")
		fmt.Printf("  → Saves: ~%d bytes\n\n", len(ch1Notes)-50)
	}
}
