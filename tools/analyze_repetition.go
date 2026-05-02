// Find the REAL repetition in the music at multiple levels
package main

import (
	"fmt"
)

func AnalyzeRealRepetition(notes []NoteWithMetadata) {
	fmt.Printf("\n=== Finding Real Musical Repetition ===\n\n")

	// Separate by channel
	channels := [3][]NoteWithMetadata{}
	for _, note := range notes {
		channels[note.Channel] = append(channels[note.Channel], note)
	}

	for ch := 0; ch < 3; ch++ {
		fmt.Printf("=== CHANNEL %d ===\n", ch)
		channelNotes := channels[ch]

		// Level 1: Base frequency sequences (melody/pitch)
		fmt.Printf("\nLevel 1: Base Frequency Sequences (melody)\n")
		freqSeqs := make(map[string][]int) // freq sequence -> note indices
		for i, note := range channelNotes {
			if note.isRest {
				continue
			}
			key := fmt.Sprintf("%d", note.freq)
			freqSeqs[key] = append(freqSeqs[key], i)
		}

		// Find most common frequencies
		type freqCount struct {
			freq  uint16
			count int
		}
		freqCounts := []freqCount{}
		for freqStr, indices := range freqSeqs {
			var freq uint16
			fmt.Sscanf(freqStr, "%d", &freq)
			freqCounts = append(freqCounts, freqCount{freq, len(indices)})
		}

		// Sort by count
		for i := 0; i < len(freqCounts); i++ {
			for j := i + 1; j < len(freqCounts); j++ {
				if freqCounts[j].count > freqCounts[i].count {
					freqCounts[i], freqCounts[j] = freqCounts[j], freqCounts[i]
				}
			}
		}

		fmt.Printf("  Top frequencies (base notes):\n")
		for i := 0; i < min(10, len(freqCounts)); i++ {
			fmt.Printf("    Freq %5d: %3d occurrences (%.1f%%)\n",
				freqCounts[i].freq, freqCounts[i].count,
				float64(freqCounts[i].count)*100.0/float64(len(channelNotes)))
		}

		// Level 2: Note sequences (melody patterns, ignoring timing)
		fmt.Printf("\nLevel 2: Melodic Sequences (ignoring exact timing)\n")

		// Extract sequences of 4-8 consecutive base frequencies
		for seqLen := 4; seqLen <= 8; seqLen++ {
			seqCount := make(map[string]int)

			for i := 0; i+seqLen <= len(channelNotes); i++ {
				seq := ""
				validSeq := true
				for j := 0; j < seqLen; j++ {
					if channelNotes[i+j].isRest {
						validSeq = false
						break
					}
					if j > 0 {
						seq += ","
					}
					seq += fmt.Sprintf("%d", channelNotes[i+j].freq)
				}
				if validSeq {
					seqCount[seq]++
				}
			}

			// Find sequences that repeat
			repeatingSeqs := []struct {
				seq   string
				count int
			}{}
			for seq, count := range seqCount {
				if count >= 2 {
					repeatingSeqs = append(repeatingSeqs, struct {
						seq   string
						count int
					}{seq, count})
				}
			}

			// Sort by count
			for i := 0; i < len(repeatingSeqs); i++ {
				for j := i + 1; j < len(repeatingSeqs); j++ {
					if repeatingSeqs[j].count > repeatingSeqs[i].count {
						repeatingSeqs[i], repeatingSeqs[j] = repeatingSeqs[j], repeatingSeqs[i]
					}
				}
			}

			if len(repeatingSeqs) > 0 {
				fmt.Printf("  Sequences of length %d:\n", seqLen)
				for i := 0; i < min(5, len(repeatingSeqs)); i++ {
					fmt.Printf("    %s: %d times\n", repeatingSeqs[i].seq, repeatingSeqs[i].count)
				}
			}
		}

		// Level 3: Rhythm patterns (duration sequences)
		fmt.Printf("\nLevel 3: Rhythm Patterns\n")
		for seqLen := 4; seqLen <= 8; seqLen++ {
			rhythmCount := make(map[string]int)

			for i := 0; i+seqLen <= len(channelNotes); i++ {
				seq := ""
				for j := 0; j < seqLen; j++ {
					if j > 0 {
						seq += ","
					}
					seq += fmt.Sprintf("%d", channelNotes[i+j].dur)
				}
				rhythmCount[seq]++
			}

			// Find repeating rhythms
			repeatingRhythms := []struct {
				seq   string
				count int
			}{}
			for seq, count := range rhythmCount {
				if count >= 3 {
					repeatingRhythms = append(repeatingRhythms, struct {
						seq   string
						count int
					}{seq, count})
				}
			}

			// Sort by count
			for i := 0; i < len(repeatingRhythms); i++ {
				for j := i + 1; j < len(repeatingRhythms); j++ {
					if repeatingRhythms[j].count > repeatingRhythms[i].count {
						repeatingRhythms[i], repeatingRhythms[j] = repeatingRhythms[j], repeatingRhythms[i]
					}
				}
			}

			if len(repeatingRhythms) > 0 {
				fmt.Printf("  Rhythm sequences of length %d:\n", seqLen)
				for i := 0; i < min(3, len(repeatingRhythms)); i++ {
					fmt.Printf("    Durations [%s]: %d times\n", repeatingRhythms[i].seq, repeatingRhythms[i].count)
				}
			}
		}

		// Level 4: Instrument usage
		fmt.Printf("\nLevel 4: Instrument Usage\n")
		instrCount := make(map[int]int)
		for _, note := range channelNotes {
			instrCount[note.instrumentID]++
		}
		fmt.Printf("  Instruments used: %d\n", len(instrCount))
		for instr, count := range instrCount {
			fmt.Printf("    Instrument %d: %d notes (%.1f%%)\n",
				instr, count, float64(count)*100.0/float64(len(channelNotes)))
		}

		fmt.Printf("\n")
	}

	// Cross-channel analysis
	fmt.Printf("=== CROSS-CHANNEL PATTERNS ===\n")

	// Check for call-and-response or harmony patterns
	fmt.Printf("Looking for synchronized events across channels...\n")

	// Group notes by start frame
	frameNotes := make(map[int][]NoteWithMetadata)
	for _, note := range notes {
		frameNotes[note.StartFrame] = append(frameNotes[note.StartFrame], note)
	}

	// Count simultaneous notes
	simultaneous := 0
	for _, notesAtFrame := range frameNotes {
		if len(notesAtFrame) > 1 {
			simultaneous++
		}
	}

	fmt.Printf("  Frames with simultaneous notes: %d\n", simultaneous)
	fmt.Printf("  Total unique start frames: %d\n", len(frameNotes))
	fmt.Printf("  Simultaneity: %.1f%%\n", float64(simultaneous)*100.0/float64(len(frameNotes)))
}
