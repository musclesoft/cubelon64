package main

import "fmt"

// stripDerivedVibrato removes the delayed vibrato and glissando records: the player
// plays vibrato for every note of an instrument flagged Vibrato, with a depth derived
// from the note frequency. The flag is only valid when it reproduces every note of
// the instrument: vibrato notes keep their pattern, and the rest are too short for
// the 12 frame hold to end. A glissando stops after two steps, which a vibrato
// matches while the note lasts at most 15 frames.
func stripDerivedVibrato(notes []NoteWithMetadata, instruments []TrackerInstrument, patterns []FreqModPattern, freqSeqMap map[string]int) []FreqModPattern {
	const holdFrames = 12
	const glissandoSteps = 2
	isVibrato := func(note NoteWithMetadata) bool {
		index := resolveFreqSeqIndex(note.freqSeq, freqSeqMap)
		if index >= 0xFE {
			return false
		}
		return patterns[index].Type == 7 || patterns[index].Type == 11
	}
	for _, note := range notes {
		if note.isRest || !isVibrato(note) {
			continue
		}
		index := resolveFreqSeqIndex(note.freqSeq, freqSeqMap)
		if patterns[index].Type == 11 && int(note.dur) > holdFrames+glissandoSteps+1 {
			panic(fmt.Sprintf("glissando note of %d frames outlasts its two vibrato steps", note.dur))
		}
		instruments[note.instrumentID].Vibrato = true
	}
	for _, note := range notes {
		if note.isRest || !instruments[note.instrumentID].Vibrato || isVibrato(note) {
			continue
		}
		if int(note.dur) > holdFrames {
			panic(fmt.Sprintf("instrument %d note of %d frames has no vibrato pattern", note.instrumentID, note.dur))
		}
	}
	remap := make([]int, len(patterns))
	var kept []FreqModPattern
	for i, pattern := range patterns {
		if pattern.Type == 7 || pattern.Type == 11 {
			remap[i] = 0xFF
			continue
		}
		remap[i] = len(kept)
		kept = append(kept, pattern)
	}
	for key, index := range freqSeqMap {
		freqSeqMap[key] = remap[index]
	}
	return kept
}
