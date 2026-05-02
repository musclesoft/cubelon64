// Analyze drum pattern sequences to find the generating algorithm
package main

import (
	"fmt"
	"math"
)

// Drum sequence templates from per-frame analysis
var drumTemplates = [][]uint16{
	{2831, 2047, 1603, 1209, 1039, 911, 811, 665, 610, 524},
	{39403, 59104, 52673, 52673, 52673, 52673, 52673, 52673, 52673, 52673},
	{2552, 1897, 1509, 1155, 999, 886, 793, 653, 599, 517},
	{14776, 2993, 2295, 13134, 14776},
}

func AnalyzeDrumAlgorithm(notes []NoteWithMetadata, freqTable []uint16) {
	fmt.Printf("\n=== Analyzing Drum Pattern Algorithms ===\n")

	// Build reverse frequency table lookup
	freqToNote := make(map[uint16]int)
	for i, freq := range freqTable {
		freqToNote[freq] = i
	}

	for templateIdx, template := range drumTemplates {
		fmt.Printf("\n--- Template %d ---\n", templateIdx)
		fmt.Printf("Sequence: ")
		for i, freq := range template {
			if i > 0 {
				fmt.Printf(" → ")
			}
			fmt.Printf("%d", freq)
		}
		fmt.Printf("\n")

		// Check if all frequencies are in note table
		allInTable := true
		noteIndices := []int{}
		for _, freq := range template {
			if idx, ok := freqToNote[freq]; ok {
				noteIndices = append(noteIndices, idx)
			} else {
				allInTable = false
				break
			}
		}

		if allInTable {
			fmt.Printf("Type: DISCRETE NOTES (all frequencies in note table)\n")
			fmt.Printf("Note indices: ")
			for i, idx := range noteIndices {
				if i > 0 {
					fmt.Printf(", ")
				}
				fmt.Printf("%d", idx)
			}
			fmt.Printf("\n")

			// Analyze note intervals
			if len(noteIndices) > 1 {
				fmt.Printf("Semitone steps: ")
				for i := 1; i < len(noteIndices); i++ {
					step := noteIndices[i] - noteIndices[i-1]
					fmt.Printf("%+d ", step)
				}
				fmt.Printf("\n")
			}
		} else {
			fmt.Printf("Type: CONTINUOUS SWEEP (not all in note table)\n")
		}

		// Analyze ratios (for exponential/geometric patterns)
		if len(template) > 1 {
			fmt.Printf("Frequency ratios: ")
			for i := 1; i < len(template) && i < 6; i++ {
				ratio := float64(template[i-1]) / float64(template[i])
				fmt.Printf("%.3f ", ratio)
			}
			fmt.Printf("\n")
		}

		// Analyze deltas (for linear or stepped patterns)
		if len(template) > 1 {
			fmt.Printf("Deltas: ")
			deltas := []int16{}
			for i := 1; i < len(template) && i < 6; i++ {
				delta := int16(template[i]) - int16(template[i-1])
				deltas = append(deltas, delta)
				fmt.Printf("%+d ", delta)
			}
			fmt.Printf("\n")

			// Check if deltas form a pattern
			if len(deltas) > 2 {
				// Check for constant acceleration (linear delta change)
				secondDeltas := []int16{}
				for i := 1; i < len(deltas); i++ {
					secondDeltas = append(secondDeltas, deltas[i]-deltas[i-1])
				}
				allSame := true
				if len(secondDeltas) > 0 {
					first := secondDeltas[0]
					for _, d := range secondDeltas[1:] {
						if d != first {
							allSame = false
							break
						}
					}
				}
				if allSame && len(secondDeltas) > 0 {
					fmt.Printf("  → Constant acceleration: %+d per frame\n", secondDeltas[0])
				}
			}
		}

		// Check for exponential decay
		if len(template) > 2 && template[0] > template[len(template)-1] {
			// Try fitting y = a * exp(-k * t)
			// Or y = a * r^t where r < 1
			startVal := float64(template[0])
			endVal := float64(template[len(template)-1])
			duration := float64(len(template) - 1)

			// Geometric: r = (end/start)^(1/duration)
			r := math.Pow(endVal/startVal, 1.0/duration)
			fmt.Printf("Geometric fit: r = %.4f (multiply by %.4f each frame)\n", r, r)

			// Check how well it fits
			maxError := 0.0
			for i := 0; i < len(template); i++ {
				predicted := startVal * math.Pow(r, float64(i))
				error := math.Abs(predicted - float64(template[i]))
				if error > maxError {
					maxError = error
				}
			}
			fmt.Printf("  → Max error: %.1f (%.1f%%)\n", maxError, maxError*100.0/startVal)

			// Try piecewise linear fit (two segments)
			if len(template) >= 5 {
				bestSplit := -1
				bestError := 999999.0

				for split := 2; split < len(template)-2; split++ {
					// Segment 1: 0 to split
					seg1Start := float64(template[0])
					seg1End := float64(template[split])
					seg1Dur := float64(split)

					// Segment 2: split to end
					seg2Start := float64(template[split])
					seg2End := float64(template[len(template)-1])
					seg2Dur := float64(len(template) - 1 - split)

					// Calculate error for this split
					maxErr := 0.0
					for i := 0; i < len(template); i++ {
						var predicted float64
						if i <= split {
							t := float64(i) / seg1Dur
							predicted = seg1Start + (seg1End-seg1Start)*t
						} else {
							t := float64(i-split) / seg2Dur
							predicted = seg2Start + (seg2End-seg2Start)*t
						}
						err := math.Abs(predicted - float64(template[i]))
						if err > maxErr {
							maxErr = err
						}
					}

					if maxErr < bestError {
						bestError = maxErr
						bestSplit = split
					}
				}

				if bestSplit > 0 {
					fmt.Printf("Piecewise linear fit (split at frame %d): max error %.1f (%.1f%%)\n",
						bestSplit, bestError, bestError*100.0/startVal)
				}
			}

			// Try fitting y = a / (b + t)^c (rational function)
			// This often fits drum envelopes better than exponential
			if len(template) >= 3 {
				// Simple case: y = a / (1 + kt) where k is constant
				// Rearrange: 1/y = (1/a) + (k/a)t
				// If we plot 1/y vs t, should be linear

				// Calculate reciprocals
				invSum := 0.0
				invTSum := 0.0
				for i := 0; i < len(template); i++ {
					if template[i] > 0 {
						invSum += 1.0 / float64(template[i])
						invTSum += float64(i) / float64(template[i])
					}
				}

				// Check linearity of reciprocals
				fmt.Printf("Reciprocal pattern check (for rational decay y=a/(1+kt)):\n")
				fmt.Printf("  1/y values: ")
				for i := 0; i < min(5, len(template)); i++ {
					fmt.Printf("%.6f ", 1.0/float64(template[i]))
				}
				fmt.Printf("\n")
			}
		}

		// Check for envelope attack-decay-sustain
		if len(template) >= 3 {
			// Find peak
			peakIdx := 0
			peakVal := template[0]
			for i := 1; i < len(template); i++ {
				if template[i] > peakVal {
					peakVal = template[i]
					peakIdx = i
				}
			}

			// Check if pattern is: rise → fall → sustain
			if peakIdx > 0 && peakIdx < len(template)-1 {
				// Find where sustain starts (values stop changing)
				sustainIdx := len(template)
				for i := peakIdx + 2; i < len(template); i++ {
					if template[i] == template[i-1] && template[i-1] == template[i-2] {
						sustainIdx = i - 2
						break
					}
				}

				if sustainIdx < len(template) {
					fmt.Printf("Envelope shape detected:\n")
					fmt.Printf("  Attack: frames 0-%d (→ %d)\n", peakIdx, peakVal)
					fmt.Printf("  Decay: frames %d-%d (→ %d)\n", peakIdx, sustainIdx, template[sustainIdx])
					fmt.Printf("  Sustain: frames %d-%d (hold %d)\n", sustainIdx, len(template)-1, template[sustainIdx])
				}
			}
		}
	}

	// Compare templates 0 and 2 to see if they share the same algorithm
	CompareDrumPatterns()

	// Find notes that use these templates to analyze AY command patterns
	fmt.Printf("\n=== Finding Source AY Commands ===\n")
	matchingNotes := []NoteWithMetadata{}
	for _, note := range notes {
		if note.isRest || len(note.freqSeq) < 5 {
			continue
		}

		// Check if this note matches a template
		for templateIdx, template := range drumTemplates {
			if len(note.freqSeq) < len(template) {
				continue
			}

			matches := true
			for i := 0; i < len(template); i++ {
				if note.freqSeq[i] != template[i] {
					matches = false
					break
				}
			}

			if matches {
				fmt.Printf("Template %d: Channel %d, frames %d-%d\n",
					templateIdx, note.Channel, note.Note.StartFrame, note.Note.EndFrame)
				matchingNotes = append(matchingNotes, note)
				if len(matchingNotes) >= 4 {
					return // Just analyze a few examples
				}
				break
			}
		}
	}
}
