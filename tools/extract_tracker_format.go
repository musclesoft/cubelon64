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

	// Build frequency sequences table for glissando notes and vibrato pattern
	freqSequences, freqSeqMap, vibratoPattern := buildFrequencySequences(allNotes)

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
						// Glissando: store pattern reference
						startFreq := allNotes[i].freqSeq[0]
						endFreq := allNotes[i].freqSeq[len(allNotes[i].freqSeq)-1]
						key := fmt.Sprintf("%d→%d", startFreq, endFreq)
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

	// Decode freqSequences into delta tables and glissando patterns
	// Format: sequences[0] = [0xFFFF, numDeltaTables]
	//         sequences[1..numDeltaTables] = delta tables
	//         sequences[numDeltaTables+1..] = patterns (deltaTableID, startFreq)

	var deltaTables []DeltaTable
	var glissandoPatterns []GlissandoPattern

	if len(freqSequences) > 0 && len(freqSequences[0]) == 2 && freqSequences[0][0] == 0xFFFF {
		numDeltaTables := int(freqSequences[0][1])

		// Extract delta tables with encoding
		for i := 1; i <= numDeltaTables && i < len(freqSequences); i++ {
			seq := freqSequences[i]
			if len(seq) < 2 {
				continue
			}

			encoding := DeltaTableEncoding(seq[0])
			length := int(seq[1])

			switch encoding {
			case EncodingTickwise:
				tickDeltas := make([]int16, len(seq)-2)
				for j := 0; j < len(tickDeltas); j++ {
					tickDeltas[j] = int16(seq[2+j])
				}
				deltaTables = append(deltaTables, DeltaTable{
					Encoding:   EncodingTickwise,
					Length:     length,
					TickDeltas: tickDeltas,
				})
			case EncodingRLE:
				numRuns := (len(seq) - 2) / 2
				runs := make([]RLERun, numRuns)
				for j := 0; j < numRuns; j++ {
					runs[j] = RLERun{
						Value: int16(seq[2+j*2]),
						Count: byte(seq[2+j*2+1]),
					}
				}
				deltaTables = append(deltaTables, DeltaTable{
					Encoding: EncodingRLE,
					Length:   length,
					RLERuns:  runs,
				})
			case EncodingFramewise:
				deltas := make([]int16, len(seq)-2)
				for j := 0; j < len(deltas); j++ {
					deltas[j] = int16(seq[2+j])
				}
				deltaTables = append(deltaTables, DeltaTable{
					Encoding: EncodingFramewise,
					Length:   length,
					Deltas:   deltas,
				})
			}
		}

		// Extract patterns
		for i := numDeltaTables + 1; i < len(freqSequences); i++ {
			seq := freqSequences[i]
			if len(seq) == 2 {
				glissandoPatterns = append(glissandoPatterns, GlissandoPattern{
					DeltaTableID: byte(seq[0]),
					StartFreq:    seq[1],
				})
			}
		}
	}

	format := &TrackerFormat{
		StartOffset:       startOffset,
		TickRate:          tickRate,
		FreqTable:         freqTable,
		DeltaTables:       deltaTables,
		GlissandoPatterns: glissandoPatterns,
		VibratoPattern:    vibratoPattern,
		Instruments:       instruments,
		NotePatterns:      notePatterns,
		Blocks:            blocks,
		Streams:           streams,
	}

	// Analyze instrument → delta table mapping
	fmt.Printf("\n  Instrument → Delta Table Mapping:\n")
	instrumentTables := make(map[byte]map[byte]bool) // instrument ID → set of delta table IDs
	for _, pattern := range notePatterns {
		if pattern.FreqSeqIndex >= 0 && int(pattern.FreqSeqIndex) < len(glissandoPatterns) {
			// This pattern uses a glissando
			gliss := glissandoPatterns[pattern.FreqSeqIndex]
			if instrumentTables[pattern.InstrumentID] == nil {
				instrumentTables[pattern.InstrumentID] = make(map[byte]bool)
			}
			instrumentTables[pattern.InstrumentID][gliss.DeltaTableID] = true
		}
	}

	// Print mapping sorted by instrument ID
	for instID := byte(0); instID < 10; instID++ {
		tables := instrumentTables[instID]
		if len(tables) > 0 {
			tableIDs := []int{}
			for tableID := range tables {
				tableIDs = append(tableIDs, int(tableID))
			}
			sort.Ints(tableIDs)
			fmt.Printf("    Instrument %d: delta tables %v (%d unique)\n", instID, tableIDs, len(tableIDs))
		}
	}

	// Check if each instrument uses only one delta table
	singleTableInstruments := 0
	for instID := byte(0); instID < 10; instID++ {
		if len(instrumentTables[instID]) == 1 {
			singleTableInstruments++
		}
	}
	if singleTableInstruments == len(instrumentTables) && len(instrumentTables) > 0 {
		fmt.Printf("    ✓ All %d instruments with glissando use exactly 1 delta table each\n", len(instrumentTables))
		fmt.Printf("    → Delta tables CAN be made instrument properties!\n")
	} else if len(instrumentTables) > 0 {
		fmt.Printf("    ✗ %d/%d instruments use multiple delta tables\n",
			len(instrumentTables)-singleTableInstruments, len(instrumentTables))
		fmt.Printf("    → Delta tables should remain global (shared across instruments)\n")
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


func createCompressedDeltaTable(deltas []int16) DeltaTable {
	length := len(deltas)

	// Try RLE encoding
	runs := []RLERun{}
	if length > 0 {
		currentVal := deltas[0]
		currentCount := byte(1)
		for i := 1; i < length; i++ {
			if deltas[i] == currentVal && currentCount < 255 {
				currentCount++
			} else {
				runs = append(runs, RLERun{Value: currentVal, Count: currentCount})
				currentVal = deltas[i]
				currentCount = 1
			}
		}
		runs = append(runs, RLERun{Value: currentVal, Count: currentCount})
	}

	// Try tick-wise encoding (sum of deltas per tick)
	tickDeltas := []int16{}
	for tickStart := 0; tickStart < length; tickStart += 5 {
		tickEnd := tickStart + 5
		if tickEnd > length {
			tickEnd = length
		}
		// Sum deltas for this tick
		tickSum := int16(0)
		for i := tickStart; i < tickEnd; i++ {
			tickSum += deltas[i]
		}
		tickDeltas = append(tickDeltas, tickSum)
	}

	// Validate tick-wise encoding by reconstructing and comparing
	tickwiseError := 0
	if len(tickDeltas) > 0 {
		tickIdx := 0
		tickSum := int16(0)
		for i := 0; i < length; i++ {
			if i > 0 && i%5 == 0 {
				tickIdx++
			}
			if tickIdx < len(tickDeltas) {
				tickSum += tickDeltas[tickIdx]
			}
			// Compare against cumulative sum of original deltas
			originalSum := int16(0)
			for j := 0; j <= i; j++ {
				originalSum += deltas[j]
			}
			error := abs(int(originalSum - tickSum))
			if error > tickwiseError {
				tickwiseError = error
			}
		}
	}

	// Calculate sizes
	framewiseSize := 1 + 1 + length*2 // encoding(1) + length(1) + deltas
	rleSize := 1 + 1 + len(runs)*3      // encoding(1) + length(1) + runs(value:2 + count:1)
	tickwiseSize := 1 + 1 + len(tickDeltas)*2 // encoding(1) + length(1) + tickDeltas

	// Choose best encoding (but avoid tick-wise if error is too high)
	// Allow up to 10 AY period units of error (about 0.5% frequency error)
	if tickwiseError <= 10 && tickwiseSize < rleSize && tickwiseSize < framewiseSize {
		return DeltaTable{
			Encoding:   EncodingTickwise,
			Length:     length,
			TickDeltas: tickDeltas,
		}
	} else if rleSize < framewiseSize {
		return DeltaTable{
			Encoding: EncodingRLE,
			Length:   length,
			RLERuns:  runs,
		}
	} else {
		return DeltaTable{
			Encoding: EncodingFramewise,
			Length:   length,
			Deltas:   deltas,
		}
	}
}

func buildFrequencySequences(notes []NoteWithMetadata) ([][]uint16, map[string]int, VibratoPattern) {
	// Analyze glissando patterns (monotonic frequency changes)
	glissandoPatterns := make(map[string]int) // key: "start→end" -> count
	glissandoCounts := make(map[string][]int) // key -> note durations

	// Analyze vibrato patterns (oscillating frequency changes)
	type vibratoExample struct {
		baseFreq uint16
		minFreq  uint16
		maxFreq  uint16
		depth    int // max deviation from base
		period   int // frames per cycle
	}
	vibratoExamples := []vibratoExample{}

	totalNotesWithChanges := 0
	monotonicCount := 0
	oscillatingCount := 0

	for _, note := range notes {
		if note.isRest || len(note.freqSeq) <= 1 {
			continue
		}

		// Check if frequency actually changes
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

		totalNotesWithChanges++

		// Check if monotonic (glissando)
		isMonotonic := true
		lastNonZeroDir := 0
		for i := 1; i < len(note.freqSeq); i++ {
			delta := int(note.freqSeq[i]) - int(note.freqSeq[i-1])
			if delta != 0 {
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

		if isMonotonic {
			monotonicCount++
			// Track glissando pattern
			startFreq := note.freqSeq[0]
			endFreq := note.freqSeq[len(note.freqSeq)-1]
			key := fmt.Sprintf("%d→%d", startFreq, endFreq)
			glissandoPatterns[key]++
			glissandoCounts[key] = append(glissandoCounts[key], len(note.freqSeq))

			// Analyze linearity for first few examples
			if monotonicCount <= 8 {
				// Check if linear in AY period space
				duration := len(note.freqSeq)
				expectedLinear := make([]uint16, duration)
				for i := 0; i < duration; i++ {
					t := float64(i) / float64(duration-1)
					if duration == 1 {
						t = 0
					}
					expectedLinear[i] = uint16(float64(startFreq) + t*float64(endFreq-startFreq))
				}

				// Compare actual vs linear
				maxDiff := 0
				for i := 0; i < duration; i++ {
					diff := int(note.freqSeq[i]) - int(expectedLinear[i])
					if diff < 0 {
						diff = -diff
					}
					if diff > maxDiff {
						maxDiff = diff
					}
				}

				// Calculate actual deltas
				deltas := make([]int, duration-1)
				for i := 1; i < duration; i++ {
					deltas[i-1] = int(note.freqSeq[i]) - int(note.freqSeq[i-1])
				}

				fmt.Printf("  DEBUG glissando #%d: %d→%d over %d frames, max deviation from linear: %d\n",
					monotonicCount, startFreq, endFreq, duration, maxDiff)
				fmt.Printf("    Actual AY periods: ")
				for i := 0; i < duration && i < 12; i++ {
					fmt.Printf("%d,", note.freqSeq[i])
				}
				if duration > 12 {
					fmt.Printf("...")
				}
				fmt.Printf("\n    Deltas (AY period): ")
				for i := 0; i < len(deltas) && i < 12; i++ {
					fmt.Printf("%+d,", deltas[i])
				}
				if len(deltas) > 12 {
					fmt.Printf("...")
				}
				fmt.Printf("\n")
			}
		} else {
			oscillatingCount++
			// Analyze vibrato pattern
			minFreq, maxFreq := note.freqSeq[0], note.freqSeq[0]
			for _, f := range note.freqSeq {
				if f < minFreq {
					minFreq = f
				}
				if f > maxFreq {
					maxFreq = f
				}
			}

			// Estimate period (frames between peaks)
			period := 0
			if len(note.freqSeq) > 4 {
				// Find first two peaks
				peaks := []int{}
				for i := 1; i < len(note.freqSeq)-1 && len(peaks) < 2; i++ {
					if note.freqSeq[i] >= note.freqSeq[i-1] && note.freqSeq[i] >= note.freqSeq[i+1] {
						peaks = append(peaks, i)
					}
				}
				if len(peaks) == 2 {
					period = peaks[1] - peaks[0]
				}
			}

			if len(vibratoExamples) < 5 {
				// Check the actual AY tone periods (before SID conversion)
				// The freqSeq contains AY tone periods, not SID frequencies
				ayMin, ayMax := note.freqSeq[0], note.freqSeq[0]
				for _, f := range note.freqSeq {
					if f < ayMin {
						ayMin = f
					}
					if f > ayMax {
						ayMax = f
					}
				}
				ayDepth := int(ayMax - ayMin)

				vibratoExamples = append(vibratoExamples, vibratoExample{
					baseFreq: note.freqSeq[0],
					minFreq:  ayMin,
					maxFreq:  ayMax,
					depth:    ayDepth,
					period:   period,
				})

				// Print full sequence for first few examples to analyze patterns
				if len(vibratoExamples) <= 3 {
					fmt.Printf("  DEBUG vibrato #%d: AY periods %d-%d (Δ%d), ch=%d dur=%d frames\n",
						len(vibratoExamples), ayMin, ayMax, ayDepth, note.Channel, note.dur)
					// Show all AY values to see the pattern
					fmt.Printf("    Full sequence: ")
					for i := 0; i < len(note.freqSeq); i++ {
						fmt.Printf("%d,", note.freqSeq[i])
					}
					fmt.Printf("\n")
				}
			}
		}
	}

	fmt.Printf("  Total notes with frequency changes: %d (%d monotonic glissando, %d oscillating vibrato)\n",
		totalNotesWithChanges, monotonicCount, oscillatingCount)

	// Print unique glissando patterns with duration statistics
	fmt.Printf("  Unique glissando patterns: %d\n", len(glissandoPatterns))

	// Compute duration statistics
	minDur := 99999
	maxDur := 0
	totalDur := 0
	totalCount := 0

	for pattern := range glissandoPatterns {
		durations := glissandoCounts[pattern]
		for _, d := range durations {
			if d < minDur {
				minDur = d
			}
			if d > maxDur {
				maxDur = d
			}
			totalDur += d
			totalCount++
		}
	}

	if totalCount > 0 {
		avgDur := totalDur / totalCount
		fmt.Printf("    Duration range: %d-%d frames (avg %d frames, %d notes)\n", minDur, maxDur, avgDur, totalCount)
	}

	// Print vibrato examples
	if len(vibratoExamples) > 0 {
		fmt.Printf("  Vibrato examples (AY tone periods):\n")
		for i, vib := range vibratoExamples {
			if i < 5 {
				fmt.Printf("    AY period %d-%d (Δ%d), period=%d frames\n",
					vib.minFreq, vib.maxFreq, vib.depth, vib.period)
			}
		}
	}

	// Build glissando pattern table with shared delta sequences
	patterns := []GlissandoPattern{}
	patternMap := make(map[string]int)

	// Extract unique delta sequences
	type deltaSeqInfo struct {
		deltas    []int16
		startFreq uint16 // First occurrence's start freq (for reference)
	}
	uniqueDeltaSeqs := make(map[string]int) // delta sequence -> deltaTableID
	deltaTables := []DeltaTable{}

	// Collect all glissando notes with their delta sequences
	type glissandoNote struct {
		startFreq uint16
		deltas    []int16
		key       string // "start→end" for patternMap
	}
	allGlissandos := []glissandoNote{}

	for _, note := range notes {
		if note.isRest || len(note.freqSeq) <= 1 {
			continue
		}

		// Check if frequency actually changes
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

		// Check if monotonic
		isMonotonic := true
		lastNonZeroDir := 0
		for i := 1; i < len(note.freqSeq); i++ {
			delta := int(note.freqSeq[i]) - int(note.freqSeq[i-1])
			if delta != 0 {
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

		if !isMonotonic {
			continue
		}

		// Extract delta sequence
		startFreq := note.freqSeq[0]
		endFreq := note.freqSeq[len(note.freqSeq)-1]
		deltas := make([]int16, len(note.freqSeq)-1)
		for i := 1; i < len(note.freqSeq); i++ {
			deltas[i-1] = int16(int(note.freqSeq[i]) - int(note.freqSeq[i-1]))
		}

		allGlissandos = append(allGlissandos, glissandoNote{
			startFreq: startFreq,
			deltas:    deltas,
			key:       fmt.Sprintf("%d→%d", startFreq, endFreq),
		})
	}

	// Build unique delta table with compression
	for _, gliss := range allGlissandos {
		// Create key from deltas only
		deltaKey := ""
		for _, d := range gliss.deltas {
			deltaKey += fmt.Sprintf("%d,", d)
		}

		// Add to delta table if not exists
		if _, exists := uniqueDeltaSeqs[deltaKey]; !exists {
			uniqueDeltaSeqs[deltaKey] = len(deltaTables)

			// Determine best encoding
			table := createCompressedDeltaTable(gliss.deltas)
			deltaTables = append(deltaTables, table)
		}
	}

	// Build pattern table (deltaTableID, startFreq pairs)
	for _, gliss := range allGlissandos {
		if _, exists := patternMap[gliss.key]; exists {
			continue // Already have this pattern
		}

		// Find delta table ID
		deltaKey := ""
		for _, d := range gliss.deltas {
			deltaKey += fmt.Sprintf("%d,", d)
		}
		deltaTableID := uniqueDeltaSeqs[deltaKey]

		patternMap[gliss.key] = len(patterns)
		patterns = append(patterns, GlissandoPattern{
			DeltaTableID: byte(deltaTableID),
			StartFreq:    gliss.startFreq,
		})
	}

	// Calculate sizes and analyze delta tables
	deltaTableBytes := 1 // count
	totalDeltas := 0
	minDelta := int16(32767)
	maxDelta := int16(-32768)
	zeroCount := 0

	for _, dt := range deltaTables {
		// Calculate size based on encoding
		switch dt.Encoding {
		case EncodingTickwise:
			deltaTableBytes += 1 + 1 + len(dt.TickDeltas)*2 // encoding + length + tickDeltas
			totalDeltas += dt.Length
			for _, d := range dt.TickDeltas {
				if d < minDelta {
					minDelta = d
				}
				if d > maxDelta {
					maxDelta = d
				}
			}
		case EncodingRLE:
			deltaTableBytes += 1 + 1 + len(dt.RLERuns)*3 // encoding + length + runs
			totalDeltas += dt.Length
			for _, run := range dt.RLERuns {
				if run.Value == 0 {
					zeroCount += int(run.Count)
				}
				if run.Value < minDelta {
					minDelta = run.Value
				}
				if run.Value > maxDelta {
					maxDelta = run.Value
				}
			}
		case EncodingFramewise:
			deltaTableBytes += 1 + 1 + len(dt.Deltas)*2 // encoding + length + deltas
			totalDeltas += len(dt.Deltas)
			for _, d := range dt.Deltas {
				if d == 0 {
					zeroCount++
				}
				if d < minDelta {
					minDelta = d
				}
				if d > maxDelta {
					maxDelta = d
				}
			}
		}
	}
	patternBytes := 1 + len(patterns)*3 // count + (deltaTableID + startFreq) per pattern
	totalSize := deltaTableBytes + patternBytes

	// Count how many patterns use each delta table
	tableUsage := make([]int, len(deltaTables))
	for _, p := range patterns {
		tableUsage[p.DeltaTableID]++
	}

	fmt.Printf("  Glissando: %d unique delta tables (%d bytes), %d patterns (%d bytes), total %d bytes\n",
		len(deltaTables), deltaTableBytes, len(patterns), patternBytes, totalSize)
	fmt.Printf("    Delta range: %d to %d, %d/%d are zero (%.1f%%)\n",
		minDelta, maxDelta, zeroCount, totalDeltas, float64(zeroCount)*100/float64(totalDeltas))

	// Analyze glissando note durations for tick alignment
	glissDurations := make(map[int]int)       // duration -> count
	tableByAlignment := make(map[byte]string) // deltaTableID -> "aligned" or "mixed"
	tableTickCounts := make(map[byte]int)     // deltaTableID -> tick-aligned count
	tableTotalCounts := make(map[byte]int)    // deltaTableID -> total count

	for _, note := range notes {
		if !note.isRest && len(note.freqSeq) > 1 {
			// Check if this has actual frequency change
			hasChange := false
			isMonotonic := true
			lastNonZeroDir := 0
			for i := 1; i < len(note.freqSeq); i++ {
				if note.freqSeq[i] != note.freqSeq[i-1] {
					hasChange = true
					delta := int(note.freqSeq[i]) - int(note.freqSeq[i-1])
					if delta != 0 {
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
			}
			if hasChange && isMonotonic {
				glissDurations[int(note.dur)]++

				// Find which delta table this note uses
				startFreq := note.freqSeq[0]
				endFreq := note.freqSeq[len(note.freqSeq)-1]
				key := fmt.Sprintf("%d→%d", startFreq, endFreq)
				if patternIdx, exists := patternMap[key]; exists {
					tableID := patterns[patternIdx].DeltaTableID
					tableTotalCounts[tableID]++
					if note.dur%5 == 0 {
						tableTickCounts[tableID]++
					}
				}
			}
		}
	}

	// Categorize tables
	for tableID := byte(0); tableID < byte(len(deltaTables)); tableID++ {
		total := tableTotalCounts[tableID]
		tickAligned := tableTickCounts[tableID]
		if total > 0 {
			if tickAligned == total {
				tableByAlignment[tableID] = "100% aligned"
			} else if tickAligned == 0 {
				tableByAlignment[tableID] = "0% aligned"
			} else {
				tableByAlignment[tableID] = fmt.Sprintf("%d%%", tickAligned*100/total)
			}
		}
	}

	tickAligned := 0
	totalGliss := 0
	for dur, count := range glissDurations {
		totalGliss += count
		if dur%5 == 0 {
			tickAligned += count
		}
	}
	fmt.Printf("    Glissando notes: %d total, %d (%.1f%%) have tick-aligned duration (multiple of 5 frames)\n",
		totalGliss, tickAligned, float64(tickAligned)*100/float64(totalGliss))
	fmt.Printf("    Per-table alignment: ")
	for i := byte(0); i < byte(len(deltaTables)); i++ {
		if align, ok := tableByAlignment[i]; ok {
			fmt.Printf("[%d]=%s ", i, align)
		}
	}
	fmt.Printf("\n")

	// Show all delta tables with encoding info
	fmt.Printf("    All %d delta tables:\n", len(deltaTables))
	encodingCounts := make(map[DeltaTableEncoding]int)
	for i := 0; i < len(deltaTables); i++ {
		dt := deltaTables[i]
		encodingCounts[dt.Encoding]++

		var actualSize int
		switch dt.Encoding {
		case EncodingTickwise:
			actualSize = 1 + 1 + len(dt.TickDeltas)*2
			fmt.Printf("      [%d] len=%d, encoding=TICK, used %dx, %d bytes: ", i, dt.Length, tableUsage[i], actualSize)
			for j := 0; j < len(dt.TickDeltas) && j < 10; j++ {
				fmt.Printf("%+d", dt.TickDeltas[j])
				if j < len(dt.TickDeltas)-1 {
					fmt.Printf(",")
				}
			}
			if len(dt.TickDeltas) > 10 {
				fmt.Printf("...")
			}
			fmt.Printf(" (1 per tick)\n")
		case EncodingRLE:
			actualSize = 1 + 1 + len(dt.RLERuns)*3
			fmt.Printf("      [%d] len=%d, encoding=RLE, used %dx, %d bytes: ", i, dt.Length, tableUsage[i], actualSize)
			for j := 0; j < len(dt.RLERuns) && j < 5; j++ {
				fmt.Printf("(%+d×%d)", dt.RLERuns[j].Value, dt.RLERuns[j].Count)
				if j < len(dt.RLERuns)-1 {
					fmt.Printf(",")
				}
			}
			if len(dt.RLERuns) > 5 {
				fmt.Printf("...")
			}
			fmt.Printf("\n")
		case EncodingFramewise:
			actualSize = 1 + 1 + len(dt.Deltas)*2
			fmt.Printf("      [%d] len=%d, encoding=FRAME, used %dx, %d bytes: ", i, dt.Length, tableUsage[i], actualSize)
			for j := 0; j < len(dt.Deltas) && j < 10; j++ {
				fmt.Printf("%+d", dt.Deltas[j])
				if j < len(dt.Deltas)-1 {
					fmt.Printf(",")
				}
			}
			if len(dt.Deltas) > 10 {
				fmt.Printf("...")
			}
			fmt.Printf("\n")
		}
	}
	fmt.Printf("    Encoding summary: TICK=%d, RLE=%d, FRAME=%d\n",
		encodingCounts[EncodingTickwise], encodingCounts[EncodingRLE], encodingCounts[EncodingFramewise])

	fmt.Printf("    Table usage (top 5): ")
	type usage struct {
		id    int
		count int
	}
	usages := []usage{}
	for i, count := range tableUsage {
		usages = append(usages, usage{i, count})
	}
	sort.Slice(usages, func(i, j int) bool {
		return usages[i].count > usages[j].count
	})
	for i := 0; i < len(usages) && i < 5; i++ {
		fmt.Printf("[%d]:%d ", usages[i].id, usages[i].count)
	}
	fmt.Printf("\n")

	// Extract global vibrato pattern from oscillating notes
	// Based on analysis: 13 frames delay, then 8-frame cycle [-2, -4, -2, 0, +2, +4, +2, 0]
	vibratoPattern := VibratoPattern{
		DelayFrames: 13,
		OffsetTable: []int8{-2, -4, -2, 0, +2, +4, +2, 0},
	}
	vibratoSize := 1 + 1 + len(vibratoPattern.OffsetTable) // delay(1) + count(1) + offsets
	fmt.Printf("  Vibrato pattern: delay=%d frames, cycle=%d frames, %d bytes\n",
		vibratoPattern.DelayFrames, len(vibratoPattern.OffsetTable), vibratoSize)

	// Return delta tables and patterns encoded as [][]uint16 for compatibility
	// Encoding format per table:
	// [encoding_type, length, data...]
	// - TICK: [0, length, delta0, delta1, ...]
	// - RLE: [1, length, value0, count0, value1, count1, ...]
	// - FRAME: [2, length, delta0, delta1, ...]
	deltaTableSeqs := make([][]uint16, len(deltaTables))
	for i, dt := range deltaTables {
		switch dt.Encoding {
		case EncodingTickwise:
			seq := make([]uint16, 2+len(dt.TickDeltas))
			seq[0] = uint16(EncodingTickwise)
			seq[1] = uint16(dt.Length)
			for j, d := range dt.TickDeltas {
				seq[2+j] = uint16(d) // int16 -> uint16 (reinterpret bits)
			}
			deltaTableSeqs[i] = seq
		case EncodingRLE:
			seq := make([]uint16, 2+len(dt.RLERuns)*2)
			seq[0] = uint16(EncodingRLE)
			seq[1] = uint16(dt.Length)
			for j, run := range dt.RLERuns {
				seq[2+j*2] = uint16(run.Value)
				seq[2+j*2+1] = uint16(run.Count)
			}
			deltaTableSeqs[i] = seq
		case EncodingFramewise:
			seq := make([]uint16, 2+len(dt.Deltas))
			seq[0] = uint16(EncodingFramewise)
			seq[1] = uint16(dt.Length)
			for j, d := range dt.Deltas {
				seq[2+j] = uint16(d)
			}
			deltaTableSeqs[i] = seq
		}
	}

	patternSeqs := make([][]uint16, len(patterns))
	for i, p := range patterns {
		patternSeqs[i] = []uint16{uint16(p.DeltaTableID), p.StartFreq}
	}

	// Combine: first element is delta tables, rest are patterns
	// Actually, we need to return both separately. Let's encode as:
	// sequences[0] = marker [0xFFFF, numDeltaTables]
	// sequences[1..numDeltaTables] = delta tables
	// sequences[numDeltaTables+1..] = patterns
	sequences := make([][]uint16, 1+len(deltaTableSeqs)+len(patternSeqs))
	sequences[0] = []uint16{0xFFFF, uint16(len(deltaTableSeqs))}
	copy(sequences[1:], deltaTableSeqs)
	copy(sequences[1+len(deltaTableSeqs):], patternSeqs)

	return sequences, patternMap, vibratoPattern
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

	// Delta tables: 1 byte count + (per table: encoding-specific format)
	size += 1 // table count
	for _, dt := range format.DeltaTables {
		switch dt.Encoding {
		case EncodingTickwise:
			size += 1 + 1 + len(dt.TickDeltas)*2 // encoding + length + tickDeltas
		case EncodingRLE:
			size += 1 + 1 + len(dt.RLERuns)*3 // encoding + length + runs (value:2 + count:1)
		case EncodingFramewise:
			size += 1 + 1 + len(dt.Deltas)*2 // encoding + length + deltas
		}
	}

	// Glissando patterns: 1 byte count + (per pattern: 1 byte deltaTableID + 2 bytes startFreq)
	size += 1 + len(format.GlissandoPatterns)*3

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
	deltaTableSize := 1
	for _, dt := range format.DeltaTables {
		switch dt.Encoding {
		case EncodingTickwise:
			deltaTableSize += 1 + 1 + len(dt.TickDeltas)*2
		case EncodingRLE:
			deltaTableSize += 1 + 1 + len(dt.RLERuns)*3
		case EncodingFramewise:
			deltaTableSize += 1 + 1 + len(dt.Deltas)*2
		}
	}
	glissandoSize := 1 + len(format.GlissandoPatterns)*3
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
	fmt.Printf("    Delta tables:     %4d bytes (%d tables)\n", deltaTableSize, len(format.DeltaTables))
	fmt.Printf("    Glissando:        %4d bytes (%d patterns × 3)\n", glissandoSize, len(format.GlissandoPatterns))
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
