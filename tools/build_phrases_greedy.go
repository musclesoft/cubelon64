// Build phrases using greedy algorithm (optimized for savings)
package main

import (
	"fmt"
	"sort"
)

// buildPhrasesGreedy extracts phrases that maximize savings
// Key difference from buildPhrases: Phrases can overlap in TABLE, selection is greedy by savings
func buildPhrasesGreedy(allNotes []NoteWithMetadata, freqIndex map[uint16]byte) ([]Phrase, map[string]int) {
	fmt.Printf("\n=== Building Phrases (Greedy by Savings) ===\n")

	// Separate by channel
	channels := [3][]NoteWithMetadata{}
	for _, note := range allNotes {
		channels[note.Channel] = append(channels[note.Channel], note)
	}

	// Find all repeating sequences (allow overlaps)
	type seqInfo struct {
		key     string
		notes   []PhraseNote
		length  int
		count   int
		savings int
	}

	seqCounts := make(map[string]int)
	seqNotes := make(map[string][]PhraseNote)

	// Try sequences length 4-16
	for ch := 0; ch < 3; ch++ {
		channelNotes := channels[ch]

		for seqLen := 4; seqLen <= 16; seqLen++ {
			for i := 0; i+seqLen <= len(channelNotes); i++ {
				// Build sequence
				seq := ""
				notes := []PhraseNote{}
				valid := true

				for j := 0; j < seqLen; j++ {
					note := channelNotes[i+j]

					var freqIdx byte
					if note.isRest {
						freqIdx = 0xFF
					} else {
						idx, ok := freqIndex[note.freq]
						if !ok {
							valid = false
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

				if valid {
					seqCounts[seq]++
					if seqCounts[seq] == 1 {
						seqNotes[seq] = notes
					}
				}
			}
		}
	}

	// Calculate savings for each sequence
	sequences := []seqInfo{}
	for seq, count := range seqCounts {
		if count < 3 {
			continue // Must repeat 3+ times
		}

		notes := seqNotes[seq]
		length := len(notes)

		// Phrase table cost: 1 byte length + length × 2 bytes
		tableCost := 1 + length*2
		// Reference cost: 2 bytes per use (phraseID + instanceID)
		refCost := count * 2
		// Current cost: 1 byte per note
		currentCost := count * length * 1
		// Net savings
		netSavings := currentCost - (tableCost + refCost)

		if netSavings > 0 {
			sequences = append(sequences, seqInfo{
				key:     seq,
				notes:   notes,
				length:  length,
				count:   count,
				savings: netSavings,
			})
		}
	}

	// Sort by savings (greedy)
	sort.Slice(sequences, func(i, j int) bool {
		return sequences[i].savings > sequences[j].savings
	})

	// Select top 128 phrases (balance coverage vs table size)
	maxPhrases := 128
	if len(sequences) > maxPhrases {
		sequences = sequences[:maxPhrases]
	}

	// Build phrase table
	phrases := []Phrase{}
	phraseMap := make(map[string]int)
	totalSavings := 0

	for i, seq := range sequences {
		phraseMap[seq.key] = i
		phrases = append(phrases, Phrase{Notes: seq.notes})
		totalSavings += seq.savings
	}

	fmt.Printf("  Found %d repeating sequences\n", len(seqCounts))
	fmt.Printf("  Profitable sequences: %d\n", len(sequences))
	fmt.Printf("  Selected phrases: %d\n", len(phrases))

	// Show top phrases
	fmt.Printf("  Top phrases by savings:\n")
	for i := 0; i < min(10, len(sequences)); i++ {
		fmt.Printf("    Phrase %d: len=%d × %d times: saves %d bytes\n",
			i, sequences[i].length, sequences[i].count, sequences[i].savings)
	}

	// Calculate table size
	tableBytes := 0
	totalRefs := 0
	for _, seq := range sequences {
		tableBytes += 1 + len(seq.notes)*2
		totalRefs += seq.count
	}

	fmt.Printf("  Phrase table: %d bytes\n", tableBytes)
	fmt.Printf("  Phrase references: %d\n", totalRefs)
	fmt.Printf("  Projected savings: %d bytes\n", totalSavings)

	return phrases, phraseMap
}
