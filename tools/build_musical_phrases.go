// Extract musical phrases aligned to compositional structure
package main

import (
	"fmt"
	"sort"
)

// buildMusicalPhrases extracts phrases based on rhythm patterns and musical structure
// Unlike arbitrary sequence matching, this finds REAL compositional phrases
func buildMusicalPhrases(allNotes []NoteWithMetadata, freqIndex map[uint16]byte) ([]Phrase, map[string]int) {
	fmt.Printf("\n=== Extracting Musical Phrases (Rhythm-Aware) ===\n")

	// Separate by channel
	channels := [3][]NoteWithMetadata{}
	for _, note := range allNotes {
		channels[note.Channel] = append(channels[note.Channel], note)
	}

	allPhrases := []Phrase{}
	phraseMap := make(map[string]int)

	for ch := 0; ch < 3; ch++ {
		fmt.Printf("\nChannel %d:\n", ch)
		channelNotes := channels[ch]

		// Step 1: Find dominant rhythm patterns (duration sequences)
		rhythms := findRhythmPatterns(channelNotes, 3, 16) // Length 3-16

		fmt.Printf("  Dominant rhythm patterns:\n")
		for i := 0; i < min(5, len(rhythms)); i++ {
			fmt.Printf("    Rhythm len=%d: %v, count=%d\n", rhythms[i].length, rhythms[i].durations, rhythms[i].count)
		}

		// Step 2: Extract melodic phrases aligned to these rhythms
		// Track phrase occurrences
		phraseOccurrences := make(map[string]int)

		for _, rhythm := range rhythms {
			if rhythm.count < 3 {
				continue // Only interested in patterns that repeat 3+ times
			}

			// For each position where this rhythm appears, extract the melody
			for _, pos := range rhythm.positions {
				if pos+rhythm.length > len(channelNotes) {
					continue
				}

				// Build phrase (melody + rhythm)
				phraseNotes := []PhraseNote{}
				phraseKey := ""
				valid := true

				for j := 0; j < rhythm.length; j++ {
					note := channelNotes[pos+j]

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

					phraseNotes = append(phraseNotes, PhraseNote{
						FreqIndex: freqIdx,
						Duration:  note.dur,
					})

					if j > 0 {
						phraseKey += ","
					}
					phraseKey += fmt.Sprintf("%d:%d", freqIdx, note.dur)
				}

				if !valid {
					continue
				}

				// Track occurrence
				phraseOccurrences[phraseKey]++

				// Add phrase if not already in table
				if _, exists := phraseMap[phraseKey]; !exists {
					phraseID := len(allPhrases)
					phraseMap[phraseKey] = phraseID
					allPhrases = append(allPhrases, Phrase{Notes: phraseNotes})
				}
			}
		}

		// Filter to phrases that appear 2+ times
		for key, count := range phraseOccurrences {
			if count >= 2 {
				phraseID, exists := phraseMap[key]
				if exists && phraseID < len(allPhrases) {
					// This phrase is worth keeping
					// (will filter in next step)
				}
			}
		}
	}

	// Step 3: Collect phrase occurrence counts across all channels
	allPhraseOccurrences := make(map[string]int)

	for ch := 0; ch < 3; ch++ {
		channelNotes := channels[ch]

		// Find all rhythm patterns for this channel
		rhythms := findRhythmPatterns(channelNotes, 3, 16)

		for _, rhythm := range rhythms {
			if rhythm.count < 2 {
				continue
			}

			// For each rhythm occurrence, track the melodic phrase
			for _, pos := range rhythm.positions {
				if pos+rhythm.length > len(channelNotes) {
					continue
				}

				// Build phrase key
				phraseKey := ""
				valid := true
				for j := 0; j < rhythm.length; j++ {
					note := channelNotes[pos+j]
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
						phraseKey += ","
					}
					phraseKey += fmt.Sprintf("%d:%d", freqIdx, note.dur)
				}

				if valid {
					allPhraseOccurrences[phraseKey]++
				}
			}
		}
	}

	// Calculate savings for each phrase and select only profitable ones
	type phraseSavings struct {
		key     string
		phraseID int
		length  int
		count   int
		savings int
	}

	savings := []phraseSavings{}
	for key, phraseID := range phraseMap {
		count := allPhraseOccurrences[key]
		if count < 3 || phraseID >= len(allPhrases) {
			continue // Must appear 3+ times
		}

		phrase := allPhrases[phraseID]
		length := len(phrase.Notes)

		// Table cost: 1 byte length + length × 2 bytes per note
		tableCost := 1 + length*2
		// Reference cost: 2 bytes per use (phraseID + instanceID)
		refCost := count * 2
		// Current cost: 1 byte per note reference
		currentCost := count * length * 1
		// Net savings
		netSavings := currentCost - (tableCost + refCost)

		if netSavings > 0 {
			savings = append(savings, phraseSavings{
				key:      key,
				phraseID: phraseID,
				length:   length,
				count:    count,
				savings:  netSavings,
			})
		}
	}

	// Sort by savings (greedy selection)
	sort.Slice(savings, func(i, j int) bool {
		return savings[i].savings > savings[j].savings
	})

	// Select top phrases (limit to reasonable number)
	maxPhrases := 200 // Limit phrase table size
	usedPhrases := []Phrase{}
	newPhraseMap := make(map[string]int)

	for i := 0; i < len(savings) && i < maxPhrases; i++ {
		phrase := allPhrases[savings[i].phraseID]
		newID := len(usedPhrases)
		newPhraseMap[savings[i].key] = newID
		usedPhrases = append(usedPhrases, phrase)
	}

	fmt.Printf("\n=== Musical Phrase Extraction Results ===\n")
	fmt.Printf("  Candidate phrases: %d\n", len(allPhrases))
	fmt.Printf("  Profitable phrases: %d\n", len(savings))
	fmt.Printf("  Selected phrases: %d\n", len(usedPhrases))

	// Show top selected phrases
	fmt.Printf("  Top phrases by savings:\n")
	for i := 0; i < min(10, len(savings)) && i < len(usedPhrases); i++ {
		fmt.Printf("    Phrase len=%d × %d times: saves %d bytes\n",
			savings[i].length, savings[i].count, savings[i].savings)
	}

	// Calculate total savings
	totalSavings := 0
	for i := 0; i < len(savings) && i < len(usedPhrases); i++ {
		totalSavings += savings[i].savings
	}

	// Calculate sizes
	phraseTableBytes := 0
	totalPhraseNotes := 0
	totalPhraseRefs := 0
	for i := 0; i < len(usedPhrases) && i < len(savings); i++ {
		phrase := usedPhrases[i]
		phraseTableBytes += 1 + len(phrase.Notes)*2
		totalPhraseNotes += len(phrase.Notes)
		totalPhraseRefs += savings[i].count
	}

	fmt.Printf("  Phrase table: %d bytes\n", phraseTableBytes)
	fmt.Printf("  Phrase references: %d\n", totalPhraseRefs)
	fmt.Printf("  Total projected savings: %d bytes\n", totalSavings)
	if len(usedPhrases) > 0 {
		fmt.Printf("  Average phrase length: %.1f notes\n", float64(totalPhraseNotes)/float64(len(usedPhrases)))
	}

	return usedPhrases, newPhraseMap
}

type rhythmPattern struct {
	durations []byte
	length    int
	count     int
	positions []int
}

// findRhythmPatterns extracts dominant rhythm patterns (duration sequences)
func findRhythmPatterns(notes []NoteWithMetadata, minLen, maxLen int) []rhythmPattern {
	rhythmCounts := make(map[string]*rhythmPattern)

	// Try different pattern lengths
	for length := minLen; length <= maxLen && length <= len(notes); length++ {
		for i := 0; i+length <= len(notes); i++ {
			// Build rhythm key
			key := ""
			durations := []byte{}
			for j := 0; j < length; j++ {
				if j > 0 {
					key += ","
				}
				key += fmt.Sprintf("%d", notes[i+j].dur)
				durations = append(durations, notes[i+j].dur)
			}

			if _, exists := rhythmCounts[key]; !exists {
				rhythmCounts[key] = &rhythmPattern{
					durations: durations,
					length:    length,
					count:     0,
					positions: []int{},
				}
			}
			rhythmCounts[key].count++
			rhythmCounts[key].positions = append(rhythmCounts[key].positions, i)
		}
	}

	// Convert to slice and sort by savings (count × length)
	patterns := []rhythmPattern{}
	for _, pattern := range rhythmCounts {
		if pattern.count >= 3 { // Only patterns that repeat 3+ times
			patterns = append(patterns, *pattern)
		}
	}

	sort.Slice(patterns, func(i, j int) bool {
		savingsI := patterns[i].count * patterns[i].length
		savingsJ := patterns[j].count * patterns[j].length
		return savingsI > savingsJ
	})

	return patterns
}

// findRhythmPositions finds all positions where a rhythm pattern appears
func findRhythmPositions(notes []NoteWithMetadata, durations []byte) []int {
	positions := []int{}

	for i := 0; i+len(durations) <= len(notes); i++ {
		matches := true
		for j := 0; j < len(durations); j++ {
			if notes[i+j].dur != durations[j] {
				matches = false
				break
			}
		}
		if matches {
			positions = append(positions, i)
		}
	}

	return positions
}
