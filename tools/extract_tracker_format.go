// Tracker format extraction from optimized Note structures
package main

import (
	"fmt"
	"sort"

	"cubelon-emu/spectrum"
)

// TrackerFormat contains the complete music data for the player
type TrackerFormat struct {
	StartOffset     int                 // Frame offset before first note (e.g., 162)
	TickRate        int                 // Frames per tick (e.g., 5 for 10Hz updates)
	FreqTable       []uint16
	FreqSequences   [][]uint16          // Frequency sequences for glissando notes
	Instruments     []TrackerInstrument
	NotePatterns    []TrackerPattern
	Phrases         []TrackerPhrase
	Orders          [3][]int // One order list per channel
}

type TrackerInstrument struct {
	BaseWaveform byte // Triangle, sawtooth, pulse, noise (NOT 0x05)
	EnvelopeMode bool // If true, player toggles between 0x05 and BaseWaveform
	ADSR         ADSRParams
	VibratoDepth byte // Frequency delta in Hz
	VibratoSpeed byte // Frames per cycle
	VibratoDelay byte // Frames before vibrato starts
}

// ADSRParams is defined in optimize_volume.go

type TrackerPattern struct {
	FreqIdx         byte
	Duration        byte
	InstrumentID    byte
	FreqSeqIndex    int16 // Index into frequency sequence table, -1 if not used
}

type TrackerPhrase struct {
	Notes []byte // Pattern IDs
}

// NoteWithMetadata extends Note with tracker-specific metadata
type NoteWithMetadata struct {
	Note         Note
	Channel      int
	StartFrame   int    // Absolute frame position
	freq         uint16
	freqSeq      []uint16
	dur          byte
	instrumentID int
	patternID    int
	isRest       bool   // True for silence/rest patterns
}

// ExtractTrackerFormat converts Note structures and AY commands to tracker format
func ExtractTrackerFormat(commands []spectrum.AYCommand, notesByChannel [3][]Note) *TrackerFormat {
	fmt.Printf("\n=== Extracting Tracker Format ===\n")

	// Find the first note to determine global offset
	firstNoteFrame := 999999
	for ch := 0; ch < 3; ch++ {
		if len(notesByChannel[ch]) > 0 {
			chFirstFrame := notesByChannel[ch][0].StartFrame
			fmt.Printf("Channel %d first note at frame %d\n", ch, chFirstFrame)
			if chFirstFrame < firstNoteFrame {
				firstNoteFrame = chFirstFrame
			}
		}
	}

	// Use the first note frame as the global offset
	// This removes the initial silence and allows notes to start at row 0
	startOffset := firstNoteFrame
	tickRate := 5 // 5 frames per tick = 10Hz update rate

	fmt.Printf("Global offset: %d frames (%.2f seconds)\n", startOffset, float64(startOffset)/50.0)
	fmt.Printf("Tick rate: %d frames = %.1f Hz\n", tickRate, 50.0/float64(tickRate))

	// Build frequency sequences for all notes from AY commands
	// Adjust positions relative to startOffset and convert to ticks
	allNotes := buildNotesWithMetadata(commands, notesByChannel, startOffset, tickRate)

	// Build frequency table from all unique base frequencies
	freqTable := buildFrequencyTableFromMetadata(allNotes)
	freqIndex := make(map[uint16]byte)
	for i, f := range freqTable {
		freqIndex[f] = byte(i)
	}

	// Build instruments with envelope mode information
	instruments, instrumentMap := buildInstrumentsFromMetadata(allNotes)

	// Assign instrument IDs to notes
	for i := range allNotes {
		key := makeInstrumentKeyFromMetadata(allNotes[i], instruments)
		allNotes[i].instrumentID = instrumentMap[key]
	}

	// Build frequency sequences table for glissando notes
	freqSequences, freqSeqMap := buildFrequencySequences(allNotes)

	// Build note patterns
	notePatterns, notePatternMap := buildNotePatternsFromMetadata(allNotes, freqIndex, freqSeqMap)

	// Assign pattern IDs
	for i := range allNotes {
		baseFreq := allNotes[i].freq
		var freqIdx byte
		freqSeqIndex := int16(-1)

		if allNotes[i].isRest {
			freqIdx = 255
		} else {
			freqIdx = freqIndex[baseFreq]

			// Check for frequency sequence
			if len(allNotes[i].freqSeq) > 1 {
				// Check if frequency actually changes
				hasChange := false
				for j := 1; j < len(allNotes[i].freqSeq); j++ {
					if allNotes[i].freqSeq[j] != allNotes[i].freq {
						hasChange = true
						break
					}
				}

				if hasChange {
					// Find frequency sequence index
					key := ""
					for _, freq := range allNotes[i].freqSeq {
						key += fmt.Sprintf("%d,", freq)
					}
					if idx, exists := freqSeqMap[key]; exists {
						freqSeqIndex = int16(idx)
					}
				}
			}
		}

		key := fmt.Sprintf("%d_%d_%d_%d_%v", freqIdx, allNotes[i].dur, allNotes[i].instrumentID, freqSeqIndex, allNotes[i].isRest)
		allNotes[i].patternID = notePatternMap[key]
	}

	// Try different phrase lengths
	bestSize := 99999
	bestLen := 8
	for phraseLen := 6; phraseLen <= 16; phraseLen++ {
		size := estimateTrackerSize(allNotes, phraseLen, len(freqTable), len(instruments), len(notePatterns))
		if size < bestSize {
			bestSize = size
			bestLen = phraseLen
		}
	}

	// Build phrases with optimal length (this will insert rest patterns and add them to notePatterns)
	phrases, channelOrders := buildPhrasesFromMetadata(allNotes, bestLen, notesByChannel, &notePatterns, notePatternMap, tickRate)

	format := &TrackerFormat{
		StartOffset:   startOffset,
		TickRate:      tickRate,
		FreqTable:     freqTable,
		FreqSequences: freqSequences,
		Instruments:   instruments,
		NotePatterns:  notePatterns,
		Phrases:       phrases,
		Orders:        channelOrders,
	}

	// Analyze frame alignment and check original AY volume commands
	fmt.Printf("\nFrame Alignment Analysis:\n")

	// Check AY volume commands alignment
	volumeCommandFrames := make(map[int]bool)
	for _, cmd := range commands {
		if cmd.Register >= 8 && cmd.Register <= 10 { // Volume registers
			volumeCommandFrames[int(cmd.Frame)] = true
		}
	}

	alignedAYCommands := 0
	for frame := range volumeCommandFrames {
		if frame%5 == 0 {
			alignedAYCommands++
		}
	}
	fmt.Printf("  Original AY volume commands: %d/%d (%.1f%%) on 5-frame boundaries\n",
		alignedAYCommands, len(volumeCommandFrames),
		float64(alignedAYCommands)*100/float64(len(volumeCommandFrames)))

	// Find the first note across all channels to determine offset
	// (firstNoteFrame already calculated above, reuse it)
	globalOffset := firstNoteFrame % 5

	fmt.Printf("  First note starts at frame %d (offset %d from 5-frame boundary)\n", firstNoteFrame, globalOffset)

	// Check detected notes alignment (after removing global offset)
	alignedNotes := 0
	alignedDurations := 0
	offByOne := 0
	offByTwo := 0
	for ch := 0; ch < 3; ch++ {
		for _, note := range notesByChannel[ch] {
			// Check alignment after removing global offset
			adjustedFrame := note.StartFrame - globalOffset
			mod := adjustedFrame % 5
			if mod == 0 {
				alignedNotes++
			} else if mod == 1 || mod == 4 {
				offByOne++
			} else if mod == 2 || mod == 3 {
				offByTwo++
			}

			dur := note.EndFrame - note.StartFrame + 1
			if dur%5 == 0 {
				alignedDurations++
			}
		}
	}
	totalNotes := len(notesByChannel[0]) + len(notesByChannel[1]) + len(notesByChannel[2])
	fmt.Printf("  Note starts (adjusted by -%d): %d/%d (%.1f%%) on 5-frame boundaries\n",
		globalOffset, alignedNotes, totalNotes, float64(alignedNotes)*100/float64(totalNotes))
	if offByOne > 0 {
		fmt.Printf("    Off by ±1 frame: %d (%.1f%%)\n", offByOne, float64(offByOne)*100/float64(totalNotes))
	}
	if offByTwo > 0 {
		fmt.Printf("    Off by ±2 frames: %d (%.1f%%)\n", offByTwo, float64(offByTwo)*100/float64(totalNotes))
	}
	fmt.Printf("  Note durations: %d/%d (%.1f%%) multiples of 5 frames\n",
		alignedDurations, totalNotes, float64(alignedDurations)*100/float64(totalNotes))

	// Print statistics
	printTrackerStats(format)

	return format
}

// buildNotesWithMetadata enriches notes with frequency and duration from AY commands
func buildNotesWithMetadata(commands []spectrum.AYCommand, notesByChannel [3][]Note, startOffset int, tickRate int) []NoteWithMetadata {
	allNotes := []NoteWithMetadata{}

	for ch := 0; ch < 3; ch++ {
		// Build frequency table for this channel from AY commands
		toneLoReg := uint8(ch * 2)
		toneHiReg := uint8(ch*2 + 1)

		freqPerFrame := make(map[int]uint16)
		currentFreq := uint16(0)

		for _, cmd := range commands {
			frame := int(cmd.Frame)
			switch cmd.Register {
			case toneLoReg:
				currentFreq = (currentFreq & 0xFF00) | uint16(cmd.Value)
				freqPerFrame[frame] = currentFreq
			case toneHiReg:
				currentFreq = (currentFreq & 0x00FF) | (uint16(cmd.Value&0x0F) << 8)
				freqPerFrame[frame] = currentFreq
			}
		}

		// Process each note on this channel
		for _, note := range notesByChannel[ch] {
			// Extract frequency sequence for this note
			freqSeq := []uint16{}
			baseFreq := uint16(0)

			for frame := note.StartFrame; frame <= note.EndFrame; frame++ {
				if freq, ok := freqPerFrame[frame]; ok && freq > 0 {
					freqSeq = append(freqSeq, freq)
					if baseFreq == 0 {
						baseFreq = freq
					}
				} else if len(freqSeq) > 0 {
					// Repeat last frequency if missing
					freqSeq = append(freqSeq, freqSeq[len(freqSeq)-1])
				}
			}

			if baseFreq == 0 {
				baseFreq = 1 // Default
			}

			// Adjust position relative to startOffset and convert to ticks
			adjustedStartFrame := note.StartFrame - startOffset
			startRow := adjustedStartFrame / tickRate

			// Store duration in frames (not quantized to ticks)
			// This preserves exact timing for notes that don't align to tick boundaries
			durationFrames := note.EndFrame - note.StartFrame + 1
			if durationFrames > 255 {
				durationFrames = 255
			}

			allNotes = append(allNotes, NoteWithMetadata{
				Note:       note,
				Channel:    ch,
				StartFrame: startRow, // Now in row units (ticks)
				freq:       baseFreq,
				freqSeq:    freqSeq,
				dur:        byte(durationFrames), // Store in frames for accuracy
				isRest:     false,
			})
		}
	}

	return allNotes
}

func buildInstrumentsFromMetadata(notes []NoteWithMetadata) ([]TrackerInstrument, map[string]int) {
	instrumentMap := make(map[string]int)
	instruments := []TrackerInstrument{}

	for _, note := range notes {
		key := makeInstrumentKeyFromMetadata(note, nil)

		if _, exists := instrumentMap[key]; !exists {
			instrumentMap[key] = len(instruments)

			inst := TrackerInstrument{
				BaseWaveform: note.Note.Waveform,
				EnvelopeMode: note.Note.EnvelopeMode,
				ADSR: ADSRParams{
					Attack:       note.Note.ADSR.Attack,
					Decay:        note.Note.ADSR.Decay,
					Sustain:      note.Note.ADSR.Sustain,
					Release:      note.Note.ADSR.Release,
					InitialDecay: note.Note.ADSR.InitialDecay,
				},
				// Vibrato disabled - all frequency changes stored as sequences
				VibratoDepth: 0,
				VibratoSpeed: 0,
				VibratoDelay: 0,
			}

			instruments = append(instruments, inst)
		}
	}

	return instruments, instrumentMap
}

func makeInstrumentKeyFromMetadata(note NoteWithMetadata, instruments []TrackerInstrument) string {
	// Vibrato disabled - only use waveform, envelope mode, and ADSR
	return fmt.Sprintf("%d_%v_%d_%d_%d_%d_%d",
		note.Note.Waveform,
		note.Note.EnvelopeMode,
		note.Note.ADSR.Attack,
		note.Note.ADSR.Decay,
		note.Note.ADSR.Sustain,
		note.Note.ADSR.Release,
		note.Note.ADSR.InitialDecay)
}


func buildFrequencySequences(notes []NoteWithMetadata) ([][]uint16, map[string]int) {
	freqSeqMap := make(map[string]int)
	freqSequences := [][]uint16{}

	for _, note := range notes {
		if note.isRest || len(note.freqSeq) <= 1 {
			continue
		}

		// Check if frequency actually changes (not just single value repeated)
		hasChange := false
		baseFreq := note.freqSeq[0]
		for i := 1; i < len(note.freqSeq); i++ {
			if note.freqSeq[i] != baseFreq {
				hasChange = true
				break
			}
		}

		if !hasChange {
			continue
		}

		// Store full frequency sequence for any note with changing frequencies
		// Create key from frequency sequence
		key := ""
		for _, freq := range note.freqSeq {
			key += fmt.Sprintf("%d,", freq)
		}

		if _, exists := freqSeqMap[key]; !exists {
			freqSeqMap[key] = len(freqSequences)
			freqSequences = append(freqSequences, note.freqSeq)
		}
	}

	return freqSequences, freqSeqMap
}

func buildNotePatternsFromMetadata(notes []NoteWithMetadata, freqIndex map[uint16]byte, freqSeqMap map[string]int) ([]TrackerPattern, map[string]int) {
	patternMap := make(map[string]int)
	patterns := []TrackerPattern{}

	for _, note := range notes {
		var freqIdx byte
		freqSeqIndex := int16(-1)

		if note.isRest {
			// Rest pattern: use FreqIdx=255 as special marker for silence
			freqIdx = 255
		} else {
			baseFreq := note.freq
			freqIdx = freqIndex[baseFreq]

			// Check if this note has a frequency sequence
			if len(note.freqSeq) > 1 {
				// Check if frequency actually changes
				hasChange := false
				for i := 1; i < len(note.freqSeq); i++ {
					if note.freqSeq[i] != note.freq {
						hasChange = true
						break
					}
				}

				if hasChange {
					// Find frequency sequence index
					key := ""
					for _, freq := range note.freqSeq {
						key += fmt.Sprintf("%d,", freq)
					}
					if idx, exists := freqSeqMap[key]; exists {
						freqSeqIndex = int16(idx)
					}
				}
			}
		}

		key := fmt.Sprintf("%d_%d_%d_%d_%v", freqIdx, note.dur, note.instrumentID, freqSeqIndex, note.isRest)
		if _, exists := patternMap[key]; !exists {
			patternMap[key] = len(patterns)
			patterns = append(patterns, TrackerPattern{
				FreqIdx:      freqIdx,
				Duration:     note.dur,
				InstrumentID: byte(note.instrumentID),
				FreqSeqIndex: freqSeqIndex,
			})
		}
	}

	return patterns, patternMap
}

func buildPhrasesFromMetadata(allNotes []NoteWithMetadata, phraseLen int, notesByChannel [3][]Note,
	notePatterns *[]TrackerPattern, notePatternMap map[string]int, tickRate int) ([]TrackerPhrase, [3][]int) {
	phrases := []TrackerPhrase{}
	phraseMap := make(map[string]int)
	var channelOrders [3][]int

	// Group allNotes by channel
	notesByChannelWithMeta := [3][]NoteWithMetadata{}
	for _, note := range allNotes {
		notesByChannelWithMeta[note.Channel] = append(notesByChannelWithMeta[note.Channel], note)
	}

	// Insert rest patterns for gaps between notes
	// Work in frames for linear sequencing (convert row positions to frames)
	for ch := 0; ch < 3; ch++ {
		channelNotes := notesByChannelWithMeta[ch]
		notesWithRests := []NoteWithMetadata{}

		for i, note := range channelNotes {
			// Convert row position to frame position
			noteFrameStart := note.StartFrame * tickRate

			// Calculate expected frame position after previous note
			expectedFrameStart := 0
			if i > 0 {
				prevNote := channelNotes[i-1]
				prevFrameStart := prevNote.StartFrame * tickRate
				expectedFrameStart = prevFrameStart + int(prevNote.dur)
			}

			gapFrames := noteFrameStart - expectedFrameStart
			if gapFrames > 0 {
				if i == 0 {
					fmt.Printf("  Channel %d: inserting %d frame initial rest (first note at row %d)\n",
						ch, gapFrames, note.StartFrame)
				}
				// Insert rest patterns for the gap (in frames)
				restFramesRemaining := gapFrames

				for restFramesRemaining > 0 {
					restDur := byte(restFramesRemaining)
					if restFramesRemaining > 255 {
						restDur = 255
					}

					// Create or reuse rest pattern
					restKey := fmt.Sprintf("rest_%d", restDur)
					restPatternID, exists := notePatternMap[restKey]
					if !exists {
						// Create new rest pattern
						restPatternID = len(*notePatterns)
						notePatternMap[restKey] = restPatternID
						*notePatterns = append(*notePatterns, TrackerPattern{
							FreqIdx:      255, // Special marker for rest
							Duration:     restDur,
							InstrumentID: 0,
						})
					}

					notesWithRests = append(notesWithRests, NoteWithMetadata{
						Channel:    ch,
						StartFrame: 0, // Not used for rests in linear sequence
						freq:       0,
						dur:        restDur,
						isRest:     true,
						patternID:  restPatternID,
					})

					restFramesRemaining -= int(restDur)
				}
			}

			notesWithRests = append(notesWithRests, note)
		}

		notesByChannelWithMeta[ch] = notesWithRests
	}

	for ch := 0; ch < 3; ch++ {
		channelNotes := notesByChannelWithMeta[ch]
		order := []int{}

		for i := 0; i < len(channelNotes); i += phraseLen {
			end := i + phraseLen
			if end > len(channelNotes) {
				end = len(channelNotes)
			}

			key := ""
			phraseNotes := []byte{}
			for j := i; j < end; j++ {
				phraseNotes = append(phraseNotes, byte(channelNotes[j].patternID))
				key += fmt.Sprintf("%d,", channelNotes[j].patternID)
			}

			if _, exists := phraseMap[key]; !exists {
				phraseMap[key] = len(phrases)
				phrases = append(phrases, TrackerPhrase{Notes: phraseNotes})
			}

			order = append(order, phraseMap[key])
		}

		channelOrders[ch] = order
	}

	return phrases, channelOrders
}

func estimateTrackerSize(notes []NoteWithMetadata, phraseLen int, freqTableSize, instrumentCount, notePatternCount int) int {
	// Estimate size in bytes
	size := 1 + freqTableSize*2                 // freq table
	size += 1 + instrumentCount*8               // instruments (waveform + envelope flag + ADSR + vibrato)
	size += 1 + notePatternCount*3              // note patterns
	size += 1                                   // phrase count
	size += phraseLen * notePatternCount / 4    // estimated phrase size
	size += 3 + len(notes)/phraseLen*3          // orders

	return size
}

func printTrackerStats(format *TrackerFormat) {
	fmt.Printf("\nTracker Format Statistics:\n")
	fmt.Printf("  Frequencies: %d entries\n", len(format.FreqTable))
	fmt.Printf("  Frequency sequences (glissando): %d entries\n", len(format.FreqSequences))
	fmt.Printf("  Instruments: %d entries\n", len(format.Instruments))
	fmt.Printf("  Note patterns: %d entries\n", len(format.NotePatterns))

	// Calculate estimated ROM size
	size := 0

	// Frequency table: 1 byte count + (2 bytes per entry)
	size += 1 + len(format.FreqTable)*2

	// Frequency sequences: 1 byte count + (1 byte length + N*2 bytes per sequence)
	size += 1
	for _, seq := range format.FreqSequences {
		size += 1 + len(seq)*2
	}

	// Instruments: 1 byte count + (9 bytes per instrument: wave, envMode, A, D, S, R, ID, vibratoD, vibratoS, vibratoDelay)
	size += 1 + len(format.Instruments)*9

	// Note patterns: 1 byte count + (5 bytes per pattern: freqIdx, duration, instrumentID, freqSeqIndex(2))
	size += 1 + len(format.NotePatterns)*5

	// Phrases: 1 byte count + (1 byte length + N bytes per phrase)
	size += 1
	for _, phrase := range format.Phrases {
		size += 1 + len(phrase.Notes)
	}

	// Orders: 3 channels * (1 byte count + N bytes)
	for ch := 0; ch < 3; ch++ {
		size += 1 + len(format.Orders[ch])
	}

	fmt.Printf("\nEstimated ROM size: %d bytes (%.1f KB)\n", size, float64(size)/1024.0)

	// Count rest patterns
	restPatterns := 0
	for _, pattern := range format.NotePatterns {
		if pattern.FreqIdx == 255 {
			restPatterns++
		}
	}
	if restPatterns > 0 {
		fmt.Printf("    Rest/silence patterns: %d\n", restPatterns)
	}
	fmt.Printf("  Phrases: %d entries\n", len(format.Phrases))

	// Count envelope mode instruments
	envelopeInsts := 0
	for _, inst := range format.Instruments {
		if inst.EnvelopeMode {
			envelopeInsts++
		}
	}
	fmt.Printf("    Envelope mode instruments: %d\n", envelopeInsts)

	// Show example instruments
	fmt.Printf("\n  Instrument examples:\n")
	for i := 0; i < len(format.Instruments) && i < 5; i++ {
		inst := format.Instruments[i]
		fmt.Printf("    %d: wave=0x%02X envelope=%v ADSR=%d/%d/%d/%d",
			i, inst.BaseWaveform, inst.EnvelopeMode,
			inst.ADSR.Attack, inst.ADSR.Decay, inst.ADSR.Sustain, inst.ADSR.Release)
		if inst.VibratoDepth > 0 {
			fmt.Printf(" vibrato=%d/%d/%d", inst.VibratoDepth, inst.VibratoSpeed, inst.VibratoDelay)
		}
		fmt.Printf("\n")
	}
}

func buildFrequencyTableFromMetadata(notes []NoteWithMetadata) []uint16 {
	freqMap := make(map[uint16]bool)
	for _, n := range notes {
		if n.freq > 0 {
			freqMap[n.freq] = true
		}
	}

	freqs := []uint16{}
	for f := range freqMap {
		freqs = append(freqs, f)
	}
	sort.Slice(freqs, func(i, j int) bool {
		return freqs[i] < freqs[j]
	})

	return freqs
}
