// Build streams using phrase-based format
package main

import (
	"fmt"
)

// buildPhrasedStreams creates channel streams that reference phrases instead of bundled note patterns
func buildPhrasedStreams(allNotes []NoteWithMetadata, phrases []Phrase, phraseMap map[string]int,
	freqIndex map[uint16]byte, instrumentMap map[string]int,
	freqSeqMap map[string]int, sustainPatternMap map[string]int,
	waveformPatternMap map[string]int, togglePeriodSeqMap map[string]int) ([]PhraseInstance, [3]TrackerStream) {

	fmt.Printf("\n=== Building Phrase-Based Streams ===\n")

	// Group notes by channel
	channels := [3][]NoteWithMetadata{}
	for _, note := range allNotes {
		channels[note.Channel] = append(channels[note.Channel], note)
	}

	// Build phrase instance table and streams
	phraseInstances := []PhraseInstance{}
	phraseInstanceMap := make(map[string]int)

	var streams [3]TrackerStream
	phraseHits := 0
	totalNotes := 0
	phraseMatchesOnMelody := 0
	phraseRejectedDueToEffects := 0

	for ch := 0; ch < 3; ch++ {
		channelNotes := channels[ch]
		stream := []byte{}
		i := 0

		for i < len(channelNotes) {
			totalNotes++

			// Try to match a phrase starting at position i
			matchedPhrase := false
			var bestPhraseID int
			var bestPhraseLen int

			// Try phrases from longest to shortest
			for phraseID := 0; phraseID < len(phrases); phraseID++ {
				phrase := phrases[phraseID]

				// Check if phrase matches at position i
				if i+len(phrase.Notes) <= len(channelNotes) {
					matches := true
					for j := 0; j < len(phrase.Notes); j++ {
						note := channelNotes[i+j]

						// Get expected freq index
						var expectedFreqIdx byte
						if note.isRest {
							expectedFreqIdx = 0xFF
						} else {
							idx, ok := freqIndex[note.freq]
							if !ok {
								matches = false
								break
							}
							expectedFreqIdx = idx
						}

						// Check if note matches phrase
						if phrase.Notes[j].FreqIndex != expectedFreqIdx || phrase.Notes[j].Duration != note.dur {
							matches = false
							break
						}
					}

					if matches && len(phrase.Notes) > bestPhraseLen {
						bestPhraseID = phraseID
						bestPhraseLen = len(phrase.Notes)
						matchedPhrase = true
					}
				}
			}

			// Declare firstNote here so it's in scope for both validation and encoding
			var firstNote NoteWithMetadata

			if matchedPhrase {
				// Found a matching phrase on melody/rhythm
				phraseMatchesOnMelody++

				// Verify all notes in phrase have identical effects
				// (if they don't, we can't use a single PhraseInstance)
				firstNote = channelNotes[i]
				effectsMatch := true

				for j := 1; j < bestPhraseLen; j++ {
					note := channelNotes[i+j]

					// Check if ADSR/waveform effects match firstNote
					if note.Note.Waveform != firstNote.Note.Waveform ||
						note.Note.EnvelopeMode != firstNote.Note.EnvelopeMode ||
						note.Note.NoisePlusTone != firstNote.Note.NoisePlusTone ||
						note.Note.ADSR.Attack != firstNote.Note.ADSR.Attack ||
						note.Note.ADSR.InitialDecay != firstNote.Note.ADSR.InitialDecay ||
						note.Note.ADSR.Decay != firstNote.Note.ADSR.Decay ||
						note.Note.ADSR.Sustain != firstNote.Note.ADSR.Sustain ||
						note.Note.ADSR.Release != firstNote.Note.ADSR.Release {
						effectsMatch = false
						break
					}

					// Check sustain patterns match
					if len(note.Note.SustainPerFrame) != len(firstNote.Note.SustainPerFrame) {
						effectsMatch = false
						break
					}
					for k := 0; k < len(note.Note.SustainPerFrame); k++ {
						if note.Note.SustainPerFrame[k] != firstNote.Note.SustainPerFrame[k] {
							effectsMatch = false
							break
						}
					}
					if !effectsMatch {
						break
					}

					// Check freqSeq match
					if len(note.freqSeq) != len(firstNote.freqSeq) {
						effectsMatch = false
						break
					}
					for k := 0; k < len(note.freqSeq); k++ {
						if note.freqSeq[k] != firstNote.freqSeq[k] {
							effectsMatch = false
							break
						}
					}
					if !effectsMatch {
						break
					}

					// Check waveformSeq match
					if len(note.waveformSeq) != len(firstNote.waveformSeq) {
						effectsMatch = false
						break
					}
					for k := 0; k < len(note.waveformSeq); k++ {
						if note.waveformSeq[k] != firstNote.waveformSeq[k] {
							effectsMatch = false
							break
						}
					}
					if !effectsMatch {
						break
					}

					// Check togglePeriodSeq match
					if len(note.togglePeriodSeq) != len(firstNote.togglePeriodSeq) {
						effectsMatch = false
						break
					}
					for k := 0; k < len(note.togglePeriodSeq); k++ {
						if note.togglePeriodSeq[k] != firstNote.togglePeriodSeq[k] {
							effectsMatch = false
							break
						}
					}
					if !effectsMatch {
						break
					}
				}

				if !effectsMatch {
					// Effects don't match - can't use phrase here
					phraseRejectedDueToEffects++
					matchedPhrase = false
				}
			}

			if matchedPhrase {
				// Found a valid phrase with matching effects!
				phraseHits++

				// Create phrase instance key
				instrKey := makeInstrumentKeyFromMetadata(firstNote, []TrackerInstrument{}) // dummy instruments
				instrumentID := instrumentMap[instrKey]

				freqSeqKey := fmt.Sprintf("%d", firstNote.freq)
				for j := 1; j < len(firstNote.freqSeq); j++ {
					delta := int16(firstNote.freqSeq[j]) - int16(firstNote.freqSeq[j-1])
					freqSeqKey += fmt.Sprintf(",%d", delta)
				}
				freqSeqIndex := byte(0xFF)
				if idx, exists := freqSeqMap[freqSeqKey]; exists {
					freqSeqIndex = byte(idx)
				}

				sustainKey := ""
				for _, s := range firstNote.Note.SustainPerFrame {
					if sustainKey != "" {
						sustainKey += ","
					}
					sustainKey += fmt.Sprintf("%d", s)
				}
				sustainIndex := byte(0xFF)
				if idx, exists := sustainPatternMap[sustainKey]; exists {
					sustainIndex = byte(idx)
				}

				waveformKey := ""
				for _, w := range firstNote.waveformSeq {
					if waveformKey != "" {
						waveformKey += ","
					}
					waveformKey += fmt.Sprintf("%d", w)
				}
				waveformIndex := byte(0xFF)
				if idx, exists := waveformPatternMap[waveformKey]; exists {
					waveformIndex = byte(idx)
				}

				toggleKey := ""
				for _, p := range firstNote.togglePeriodSeq {
					if toggleKey != "" {
						toggleKey += ","
					}
					toggleKey += fmt.Sprintf("%d", p)
				}
				toggleIndex := byte(0xFF)
				if idx, exists := togglePeriodSeqMap[toggleKey]; exists {
					toggleIndex = byte(idx)
				}

				// Create or find phrase instance
				instanceKey := fmt.Sprintf("%d_%d_%d_%d_%d_%d",
					bestPhraseID, instrumentID, freqSeqIndex, sustainIndex, waveformIndex, toggleIndex)

				instanceID, exists := phraseInstanceMap[instanceKey]
				if !exists {
					instanceID = len(phraseInstances)
					phraseInstanceMap[instanceKey] = instanceID
					phraseInstances = append(phraseInstances, PhraseInstance{
						InstrumentIndex:      byte(instrumentID),
						FreqSeqIndex:         freqSeqIndex,
						SustainPatternIndex:  sustainIndex,
						WaveformPatternIndex: waveformIndex,
						TogglePeriodIndex:    toggleIndex,
					})
				}

				// Encode in stream: phraseID + instanceID
				// Use high bit to distinguish phrase refs from legacy note pattern refs
				// Format: 0x80 | phraseID for first byte, then instanceID
				stream = append(stream, byte(0x80|bestPhraseID))
				stream = append(stream, byte(instanceID))

				// Skip all notes covered by this phrase
				i += bestPhraseLen

			} else {
				// No phrase match - encode as single note (legacy format)
				// This is a fallback for notes not covered by phrases
				// For now, just use note pattern ID (will be 0 since we're not using legacy)
				// TODO: Build single-note pattern or use different encoding
				stream = append(stream, 0x00) // Placeholder
				i++
			}
		}

		streams[ch] = TrackerStream(stream)
		fmt.Printf("  Channel %d: %d bytes\n", ch, len(stream))
	}

	fmt.Printf("  Phrase matches (melody/rhythm): %d\n", phraseMatchesOnMelody)
	fmt.Printf("  Rejected due to effect mismatch: %d (%.1f%%)\n",
		phraseRejectedDueToEffects,
		float64(phraseRejectedDueToEffects)*100.0/float64(phraseMatchesOnMelody))
	fmt.Printf("  Phrase coverage: %d/%d notes (%.1f%%)\n",
		phraseHits, totalNotes, float64(phraseHits)*100.0/float64(totalNotes))
	fmt.Printf("  Phrase instances: %d unique effect combinations\n", len(phraseInstances))

	return phraseInstances, streams
}
