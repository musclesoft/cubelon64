// Build phrase-based format (separated melody from effects)
package main

import (
	"fmt"
	"sort"
)

// buildPhrases extracts reusable musical phrases and their rendering parameters
func buildPhrases(allNotes []NoteWithMetadata, freqIndex map[uint16]byte) ([]Phrase, []PhraseInstance, map[string]int, map[string]int) {
	fmt.Printf("\n=== Building Phrase-Based Format ===\n")

	// Group notes by channel
	channels := [3][]NoteWithMetadata{}
	for _, note := range allNotes {
		channels[note.Channel] = append(channels[note.Channel], note)
	}

	// Find repeating (pitch, duration) sequences across all channels
	type seqInfo struct {
		seq       string
		length    int
		count     int
		savings   int
		notes     []PhraseNote
		positions []int // where this sequence appears in the full note stream
	}

	seqCounts := make(map[string]int)
	seqLengths := make(map[string]int)
	seqNotes := make(map[string][]PhraseNote)
	seqPositions := make(map[string][]int)

	// Find sequences in each channel
	for ch := 0; ch < 3; ch++ {
		channelNotes := channels[ch]

		for seqLen := 4; seqLen <= 12; seqLen++ { // Extended to length 12
			for i := 0; i+seqLen <= len(channelNotes); i++ {
				// Build sequence key from (pitch, duration) pairs
				seq := ""
				notes := []PhraseNote{}
				validSeq := true

				for j := 0; j < seqLen; j++ {
					note := channelNotes[i+j]

					// Get freq index
					var freqIdx byte
					if note.isRest {
						freqIdx = 0xFF // Rest marker
					} else {
						idx, ok := freqIndex[note.freq]
						if !ok {
							validSeq = false
							break
						}
						freqIdx = idx
					}

					if j > 0 {
						seq += ","
					}
					seq += fmt.Sprintf("%d:%d", freqIdx, note.dur)

					notes = append(notes, PhraseNote{
						FreqIndex: freqIdx,
						Duration:  note.dur,
					})
				}

				if validSeq {
					if seqCounts[seq] == 0 {
						seqLengths[seq] = seqLen
						seqNotes[seq] = notes
					}
					seqCounts[seq]++
					// Store position in full note stream for later reference
					seqPositions[seq] = append(seqPositions[seq], i)
				}
			}
		}
	}

	// Calculate savings for each sequence
	seqList := []seqInfo{}
	for seq, count := range seqCounts {
		if count <= 1 {
			continue // Only interested in repeating sequences
		}

		seqLen := seqLengths[seq]
		// Phrase table cost: 1 byte length + seqLen × 2 bytes (freqIdx + duration)
		phraseCost := 1 + seqLen*2
		// Reference cost: 1 byte per use (phrase ID)
		refCost := count * 1
		// Current cost without phrases: count × seqLen bytes (each note = 1 byte reference)
		currentCost := count * seqLen * 1
		// Savings = what we save - what it costs
		savings := currentCost - (phraseCost + refCost)

		if savings > 0 {
			seqList = append(seqList, seqInfo{
				seq:       seq,
				length:    seqLen,
				count:     count,
				savings:   savings,
				notes:     seqNotes[seq],
				positions: seqPositions[seq],
			})
		}
	}

	// Sort by savings (greedy selection)
	sort.Slice(seqList, func(i, j int) bool {
		return seqList[i].savings > seqList[j].savings
	})

	// Select non-overlapping phrases (greedy algorithm)
	maxPhrases := 128 // Max phrase table size
	selectedPhrases := []seqInfo{}
	usedPositions := make(map[int]bool)

	for _, seq := range seqList {
		if len(selectedPhrases) >= maxPhrases {
			break
		}

		// Check if this phrase overlaps with already selected phrases
		overlaps := false
		for _, pos := range seq.positions {
			for i := pos; i < pos+seq.length; i++ {
				if usedPositions[i] {
					overlaps = true
					break
				}
			}
			if overlaps {
				break
			}
		}

		if !overlaps {
			selectedPhrases = append(selectedPhrases, seq)
			// Mark positions as used
			for _, pos := range seq.positions {
				for i := pos; i < pos+seq.length; i++ {
					usedPositions[i] = true
				}
			}
		}
	}

	fmt.Printf("  Found %d repeating sequences\n", len(seqList))
	fmt.Printf("  Selected %d non-overlapping phrases (max savings)\n", len(selectedPhrases))

	// Build phrase table
	phrases := []Phrase{}
	phraseMap := make(map[string]int) // sequence -> phrase ID
	totalPhraseRefs := 0

	for _, seqInf := range selectedPhrases {
		phraseID := len(phrases)
		phraseMap[seqInf.seq] = phraseID
		phrases = append(phrases, Phrase{Notes: seqInf.notes})
		totalPhraseRefs += seqInf.count

		if len(phrases) <= 10 {
			fmt.Printf("    Phrase %d: len=%d, count=%d, saves=%d bytes\n",
				phraseID, seqInf.length, seqInf.count, seqInf.savings)
		}
	}

	// Calculate total savings
	totalSavings := 0
	for _, seq := range selectedPhrases {
		totalSavings += seq.savings
	}

	phraseTableBytes := 0
	for _, phrase := range phrases {
		phraseTableBytes += 1 + len(phrase.Notes)*2
	}

	fmt.Printf("  Phrase table: %d phrases, %d bytes\n", len(phrases), phraseTableBytes)
	fmt.Printf("  Phrase references: %d total\n", totalPhraseRefs)
	fmt.Printf("  Estimated savings: %d bytes\n", totalSavings)

	// Build phrase instances (effect combinations)
	// For now, build from existing note patterns
	phraseInstances := []PhraseInstance{}
	phraseInstanceMap := make(map[string]int)

	// We'll populate phrase instances during stream building
	// Return empty for now - will be built incrementally

	return phrases, phraseInstances, phraseMap, phraseInstanceMap
}
