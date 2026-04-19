// Tracker format extraction from optimized Note structures
package main

import (
	"fmt"
	"sort"

	"cubelon-emu/spectrum"
)

// TrackerFormat contains the complete music data for the player
type TrackerFormat struct {
	StartOffset       int                 // Frame offset before first note (e.g., 162)
	TickRate          int                 // Frames per tick (e.g., 5 for 10Hz updates)
	FreqTable         []uint16
	GlissandoPatterns []GlissandoPattern  // Frequency sweeps with per-frame deltas
	VibratoPattern    VibratoPattern      // Global vibrato pattern (applied to base freq)
	Instruments       []TrackerInstrument
	NotePatterns      []TrackerPattern
	Blocks            []TrackerBlock      // Reusable note sequences
	Streams           [3]TrackerStream    // Per-channel bytecode
}

// GlissandoPattern stores a frequency sweep with simple per-frame deltas
type GlissandoPattern struct {
	StartFreq uint16   // Starting AY tone period
	Deltas    []int16  // Per-frame delta from previous frequency
}

type VibratoPattern struct {
	DelayFrames int     // Frames before vibrato starts (e.g., 13)
	OffsetTable []int8  // Per-frame offsets to base AY period (e.g., [-4,-2,0,+2,+4,+2,0,-2])
}

type TrackerInstrument struct {
	BaseWaveform byte // Triangle, sawtooth, pulse, noise (NOT 0x05)
	EnvelopeMode bool // If true, player toggles between 0x05 and BaseWaveform
	ADSR         ADSRParams
	VibratoDepth byte // Frequency delta in Hz
	VibratoSpeed byte // Frames per cycle
	VibratoDelay byte // Frames before vibrato starts
}

type TrackerPattern struct {
	FreqIdx      byte
	Duration     byte
	InstrumentID byte
	FreqSeqIndex byte // 0xFF=none, 0xFE=vibrato, 0-17=glissando pattern index
}

type TrackerBlock struct {
	Patterns []byte // Pattern IDs (variable length)
}

type TrackerStream []byte // Bytecode: 0x00-0x7F = play block, 0x80+ = commands

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

	// Build glissando patterns and vibrato pattern
	glissandoPatterns, freqSeqMap, vibratoPattern := buildFrequencySequences(allNotes)

	// Build note patterns
	notePatterns, notePatternMap := buildNotePatternsFromMetadata(allNotes, freqIndex, freqSeqMap, vibratoPattern)

	// Assign pattern IDs
	for i := range allNotes {
		baseFreq := allNotes[i].freq
		var freqIdx byte
		freqSeqIndex := int16(-1)

		if allNotes[i].isRest {
			freqIdx = 255
		} else {
			freqIdx = freqIndex[baseFreq]

			// Check for frequency modulation (glissando or vibrato)
			if len(allNotes[i].freqSeq) > 1 {
				// Check if frequency changes and is monotonic
				hasChange := false
				isMonotonic := true
				lastNonZeroDir := 0

				for j := 1; j < len(allNotes[i].freqSeq); j++ {
					delta := int(allNotes[i].freqSeq[j]) - int(allNotes[i].freqSeq[j-1])
					if delta != 0 {
						hasChange = true
						currDir := 1
						if delta < 0 {
							currDir = -1
						}
						if lastNonZeroDir != 0 && currDir != lastNonZeroDir {
							isMonotonic = false
							break
						}
						lastNonZeroDir = currDir
					}
				}

				if hasChange {
					if isMonotonic {
						// Glissando: build key from startFreq + deltas to match buildFrequencySequences
						startFreq := allNotes[i].freqSeq[0]
						key := fmt.Sprintf("%d", startFreq)
						for j := 1; j < len(allNotes[i].freqSeq); j++ {
							delta := int16(allNotes[i].freqSeq[j]) - int16(allNotes[i].freqSeq[j-1])
							key += fmt.Sprintf(",%d", delta)
						}
						if idx, exists := freqSeqMap[key]; exists {
							freqSeqIndex = int16(idx)
						}
					} else {
						// Vibrato: use global pattern
						freqSeqIndex = -2
					}
				}
			}
		}

		key := fmt.Sprintf("%d_%d_%d_%d_%v", freqIdx, allNotes[i].dur, allNotes[i].instrumentID, freqSeqIndex, allNotes[i].isRest)
		allNotes[i].patternID = notePatternMap[key]
	}

	// Build dynamic-length blocks and bytecode streams
	blocks, streams := buildBlocksFromMetadata(allNotes, notesByChannel, &notePatterns, notePatternMap, tickRate)

	format := &TrackerFormat{
		StartOffset:       startOffset,
		TickRate:          tickRate,
		FreqTable:         freqTable,
		GlissandoPatterns: glissandoPatterns,
		VibratoPattern:    vibratoPattern,
		Instruments:       instruments,
		NotePatterns:      notePatterns,
		Blocks:            blocks,
		Streams:           streams,
	}

	// TODO: Re-add instrument analysis once glissando patterns are rebuilt

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

			// Adjust position relative to startOffset (keep in frames, not ticks)
			adjustedStartFrame := note.StartFrame - startOffset

			// Store duration in frames (not quantized to ticks)
			// This preserves exact timing for notes that don't align to tick boundaries
			durationFrames := note.EndFrame - note.StartFrame + 1
			if durationFrames > 255 {
				durationFrames = 255
			}

			allNotes = append(allNotes, NoteWithMetadata{
				Note:       note,
				Channel:    ch,
				StartFrame: adjustedStartFrame, // Keep in frames, not ticks
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

func buildFrequencySequences(notes []NoteWithMetadata) ([]GlissandoPattern, map[string]int, VibratoPattern) {
	glissandoMap := make(map[string]int)
	patterns := []GlissandoPattern{}

	for _, note := range notes {
		if note.isRest || len(note.freqSeq) <= 1 {
			continue
		}

		// Check if frequency actually changes
		hasChange := false
		for i := 1; i < len(note.freqSeq); i++ {
			if note.freqSeq[i] != note.freqSeq[0] {
				hasChange = true
				break
			}
		}

		if !hasChange {
			continue
		}

		// Check if monotonic (glissando) vs oscillating (vibrato)
		isMonotonic := true
		lastDir := 0
		for i := 1; i < len(note.freqSeq); i++ {
			delta := int(note.freqSeq[i]) - int(note.freqSeq[i-1])
			if delta != 0 {
				dir := 1
				if delta < 0 {
					dir = -1
				}
				if lastDir != 0 && dir != lastDir {
					isMonotonic = false
					break
				}
				lastDir = dir
			}
		}

		if !isMonotonic {
			continue // Skip vibrato notes (handled globally)
		}

		// Build glissando pattern with start freq + per-frame deltas
		startFreq := note.freqSeq[0]
		deltas := make([]int16, len(note.freqSeq)-1)
		for i := 1; i < len(note.freqSeq); i++ {
			deltas[i-1] = int16(note.freqSeq[i]) - int16(note.freqSeq[i-1])
		}

		// Create unique key from full sequence
		key := fmt.Sprintf("%d", startFreq)
		for _, d := range deltas {
			key += fmt.Sprintf(",%d", d)
		}

		// Add pattern if not seen before
		if _, exists := glissandoMap[key]; !exists {
			glissandoMap[key] = len(patterns)
			patterns = append(patterns, GlissandoPattern{
				StartFreq: startFreq,
				Deltas:    deltas,
			})
		}
	}

	// Build vibrato pattern (hardcoded from analysis)
	vibratoPattern := VibratoPattern{
		DelayFrames: 13,
		OffsetTable: []int8{-2, -4, -2, 0, 2, 4, 2, 0},
	}

	fmt.Printf("  Glissando: %d unique patterns\n", len(patterns))

	return patterns, glissandoMap, vibratoPattern
}

func buildNotePatternsFromMetadata(notes []NoteWithMetadata, freqIndex map[uint16]byte, freqSeqMap map[string]int, vibratoPattern VibratoPattern) ([]TrackerPattern, map[string]int) {
	patternMap := make(map[string]int)
	patterns := []TrackerPattern{}

	for _, note := range notes {
		var freqIdx byte
		freqSeqIndex := byte(0xFF) // 0xFF = no modulation (was -1)

		if note.isRest {
			// Rest pattern: use FreqIdx=255 as special marker for silence
			freqIdx = 255
		} else {
			baseFreq := note.freq
			freqIdx = freqIndex[baseFreq]

			// Check for frequency modulation (glissando or vibrato)
			if len(note.freqSeq) > 1 {
				hasChange := false
				isMonotonic := true
				lastNonZeroDir := 0

				for i := 1; i < len(note.freqSeq); i++ {
					delta := int(note.freqSeq[i]) - int(note.freqSeq[i-1])
					if delta != 0 {
						hasChange = true
						currDir := 1
						if delta < 0 {
							currDir = -1
						}
						if lastNonZeroDir != 0 && currDir != lastNonZeroDir {
							isMonotonic = false
							break
						}
						lastNonZeroDir = currDir
					}
				}

				if hasChange {
					if isMonotonic {
						// Glissando: reference pattern (0-17)
						startFreq := note.freqSeq[0]
						endFreq := note.freqSeq[len(note.freqSeq)-1]
						key := fmt.Sprintf("%d→%d", startFreq, endFreq)
						if idx, exists := freqSeqMap[key]; exists {
							freqSeqIndex = byte(idx)
						}
					} else {
						// Vibrato: use global pattern (0xFE = was -2)
						freqSeqIndex = 0xFE
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

func buildBlocksFromMetadata(allNotes []NoteWithMetadata, notesByChannel [3][]Note,
	notePatterns *[]TrackerPattern, notePatternMap map[string]int, tickRate int) ([]TrackerBlock, [3]TrackerStream) {

	// Group notes by channel and build pattern ID streams
	notesByChannelWithMeta := [3][]NoteWithMetadata{}
	for _, note := range allNotes {
		notesByChannelWithMeta[note.Channel] = append(notesByChannelWithMeta[note.Channel], note)
	}

	// Insert rest patterns for gaps between notes
	for ch := 0; ch < 3; ch++ {
		channelNotes := notesByChannelWithMeta[ch]
		notesWithRests := []NoteWithMetadata{}

		for i, note := range channelNotes {
			noteFrameStart := note.StartFrame
			expectedFrameStart := 0
			if i > 0 {
				prevNote := channelNotes[i-1]
				prevFrameStart := prevNote.StartFrame
				expectedFrameStart = prevFrameStart + int(prevNote.dur)
			}

			gapFrames := noteFrameStart - expectedFrameStart
			if gapFrames > 0 {
				if i == 0 {
					fmt.Printf("  Channel %d: inserting %d frame initial rest (first note at frame %d)\n",
						ch, gapFrames, note.StartFrame)
				}
				restFramesRemaining := gapFrames

				for restFramesRemaining > 0 {
					restDur := byte(restFramesRemaining)
					if restFramesRemaining > 255 {
						restDur = 255
					}

					restKey := fmt.Sprintf("rest_%d", restDur)
					restPatternID, exists := notePatternMap[restKey]
					if !exists {
						restPatternID = len(*notePatterns)
						notePatternMap[restKey] = restPatternID
						*notePatterns = append(*notePatterns, TrackerPattern{
							FreqIdx:      255,
							Duration:     restDur,
							InstrumentID: 0,
							FreqSeqIndex: 0xFF,
						})
					}

					notesWithRests = append(notesWithRests, NoteWithMetadata{
						Channel:    ch,
						StartFrame: 0,
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

	// Build pattern ID streams per channel
	channelStreams := [3][]byte{}
	for ch := 0; ch < 3; ch++ {
		stream := []byte{}
		for _, note := range notesByChannelWithMeta[ch] {
			stream = append(stream, byte(note.patternID))
		}
		channelStreams[ch] = stream
	}

	// Find recurring sequences across all channels (lengths 3-16)
	type seqInfo struct {
		seq      string
		length   int
		count    int
		savings  int
	}

	seqCounts := make(map[string]int)
	seqLengths := make(map[string]int)

	for ch := 0; ch < 3; ch++ {
		stream := channelStreams[ch]
		for seqLen := 3; seqLen <= 16; seqLen++ {
			for i := 0; i+seqLen <= len(stream); i++ {
				seq := string(stream[i : i+seqLen])
				if seqCounts[seq] == 0 {
					seqLengths[seq] = seqLen
				}
				seqCounts[seq]++
			}
		}
	}

	// Calculate savings for each sequence
	seqList := []seqInfo{}
	for seq, count := range seqCounts {
		if count <= 1 {
			continue
		}
		seqLen := seqLengths[seq]
		// Block table cost: 1 byte length + seqLen bytes
		blockCost := 1 + seqLen
		// Stream savings: replace count × seqLen bytes with count × 1 byte
		savings := count*seqLen - count - blockCost
		if savings > 0 {
			seqList = append(seqList, seqInfo{seq, seqLen, count, savings})
		}
	}

	// Sort by savings
	sort.Slice(seqList, func(i, j int) bool {
		return seqList[i].savings > seqList[j].savings
	})

	// Select top 32 blocks (or fewer if not available)
	maxBlocks := 32
	if len(seqList) > maxBlocks {
		seqList = seqList[:maxBlocks]
	}

	// Create block dictionary
	blockDict := make(map[string]byte) // sequence -> block ID
	blocks := []TrackerBlock{}
	for i, seqInf := range seqList {
		blockDict[seqInf.seq] = byte(i)
		patterns := []byte{}
		for j := 0; j < len(seqInf.seq); j++ {
			patterns = append(patterns, seqInf.seq[j])
		}
		blocks = append(blocks, TrackerBlock{Patterns: patterns})
	}

	fmt.Printf("  Building %d blocks (lengths: ", len(blocks))
	for i := 0; i < len(blocks) && i < 5; i++ {
		fmt.Printf("%d ", len(blocks[i].Patterns))
	}
	fmt.Printf("...)\n")

	// Generate bytecode streams
	var streams [3]TrackerStream

	totalBlockRefs := 0
	totalDirectRefs := 0

	for ch := 0; ch < 3; ch++ {
		stream := channelStreams[ch]
		bytecode := []byte{}
		i := 0
		chBlockRefs := 0
		chDirectRefs := 0

		for i < len(stream) {
			// Try to match longest block at current position
			matchedBlock := byte(0xFF)
			matchedLen := 0

			for blockSeq, blockID := range blockDict {
				seqLen := len(blockSeq)
				if i+seqLen <= len(stream) && string(stream[i:i+seqLen]) == blockSeq {
					if seqLen > matchedLen {
						matchedBlock = blockID
						matchedLen = seqLen
					}
				}
			}

			if matchedBlock != 0xFF {
				// Emit block reference (0x00-0x1F for blocks 0-31)
				bytecode = append(bytecode, matchedBlock)
				i += matchedLen
				chBlockRefs++
			} else {
				// No block match - emit pattern directly
				// Encoding: 0x00-0x1F = blocks, 0x20+ = patterns (opcode = 0x20 + patternID)
				// This allows 166 patterns (0-165) to fit in opcodes 0x20-0xC5
				bytecode = append(bytecode, 0x20+stream[i])
				i++
				chDirectRefs++
			}
		}

		totalBlockRefs += chBlockRefs
		totalDirectRefs += chDirectRefs
		streams[ch] = TrackerStream(bytecode)
		fmt.Printf("    Ch %d: %d block refs, %d direct refs, stream = %d bytes\n",
			ch, chBlockRefs, chDirectRefs, len(bytecode))
	}

	fmt.Printf("  Total: %d block refs, %d direct refs\n", totalBlockRefs, totalDirectRefs)

	return blocks, streams
}

func printTrackerStats(format *TrackerFormat) {
	fmt.Printf("\nTracker Format Statistics:\n")
	fmt.Printf("  Frequencies: %d entries\n", len(format.FreqTable))
	fmt.Printf("  Glissando patterns: %d entries\n", len(format.GlissandoPatterns))
	fmt.Printf("  Instruments: %d entries\n", len(format.Instruments))
	fmt.Printf("  Note patterns: %d entries\n", len(format.NotePatterns))

	// Calculate estimated ROM size
	size := 0

	// Frequency table: 1 byte count + (2 bytes per entry)
	size += 1 + len(format.FreqTable)*2

	// Glissando patterns: 1 byte count + (per pattern: 1 byte length + 2 bytes startFreq + N*2 bytes deltas)
	size += 1 // count
	for _, pattern := range format.GlissandoPatterns {
		size += 1 + 2 + len(pattern.Deltas)*2 // length + startFreq + deltas
	}

	// Vibrato pattern: 1 byte delay + 1 byte count + N bytes offsets
	size += 1 + 1 + len(format.VibratoPattern.OffsetTable)

	// Instruments: 1 byte count + (9 bytes per instrument: wave, envMode, A, D, S, R, ID, vibratoD, vibratoS, vibratoDelay)
	size += 1 + len(format.Instruments)*9

	// Note patterns: 1 byte count + (4 bytes per pattern: freqIdx, duration, instrumentID, freqSeqIndex)
	size += 1 + len(format.NotePatterns)*4

	// Blocks: 1 byte count + (per block: 1 byte length + N pattern IDs)
	size += 1
	for _, block := range format.Blocks {
		size += 1 + len(block.Patterns)
	}

	// Streams: 3 channels * (1 byte count + N bytecode bytes)
	for ch := 0; ch < 3; ch++ {
		size += 1 + len(format.Streams[ch])
	}

	// Detailed breakdown
	freqTableSize := 1 + len(format.FreqTable)*2
	glissandoSize := 1
	for _, pattern := range format.GlissandoPatterns {
		glissandoSize += 1 + 2 + len(pattern.Deltas)*2
	}
	vibratoSize := 1 + 1 + len(format.VibratoPattern.OffsetTable)
	instrumentSize := 1 + len(format.Instruments)*9
	notePatternSize := 1 + len(format.NotePatterns)*4
	blockSize := 1
	for _, block := range format.Blocks {
		blockSize += 1 + len(block.Patterns)
	}
	streamSize := 0
	for ch := 0; ch < 3; ch++ {
		streamSize += 1 + len(format.Streams[ch])
	}

	fmt.Printf("\nEstimated ROM size: %d bytes (%.1f KB)\n", size, float64(size)/1024.0)
	fmt.Printf("  Breakdown:\n")
	fmt.Printf("    Frequency table:  %4d bytes (%d entries × 2)\n", freqTableSize, len(format.FreqTable))
	fmt.Printf("    Glissando:        %4d bytes (%d patterns)\n", glissandoSize, len(format.GlissandoPatterns))
	fmt.Printf("    Vibrato:          %4d bytes\n", vibratoSize)
	fmt.Printf("    Instruments:      %4d bytes (%d × 9)\n", instrumentSize, len(format.Instruments))
	fmt.Printf("    Note patterns:    %4d bytes (%d × 4)\n", notePatternSize, len(format.NotePatterns))
	fmt.Printf("    Blocks:           %4d bytes (%d blocks)\n", blockSize, len(format.Blocks))
	fmt.Printf("    Streams:          %4d bytes (3 channels)\n", streamSize)

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
	fmt.Printf("  Blocks: %d entries\n", len(format.Blocks))

	// Show block length distribution
	blockLenDist := make(map[int]int)
	for _, block := range format.Blocks {
		blockLenDist[len(block.Patterns)]++
	}
	if len(blockLenDist) > 0 {
		fmt.Printf("    Block lengths: ")
		lengths := []int{}
		for length := range blockLenDist {
			lengths = append(lengths, length)
		}
		sort.Ints(lengths)
		for _, length := range lengths {
			fmt.Printf("%d:%d ", length, blockLenDist[length])
		}
		fmt.Printf("\n")
	}

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
		fmt.Printf("    %d: wave=0x%02X envelope=%v ADSR=%d/%d/%d/%d\n",
			i, inst.BaseWaveform, inst.EnvelopeMode,
			inst.ADSR.Attack, inst.ADSR.Decay, inst.ADSR.Sustain, inst.ADSR.Release)
	}

	// Skip detailed analysis (was specific to old phrase/order format)
	return
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
