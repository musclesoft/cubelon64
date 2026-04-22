// Tracker format extraction from optimized Note structures
package main

import (
	"fmt"
	"sort"

	"cubelon-emu/spectrum"
)

// TrackerFormat contains the complete music data for the player
type TrackerFormat struct {
	StartOffset           int                     // Frame offset before first note (e.g., 162)
	TickRate              int                     // Frames per tick (e.g., 5 for 10Hz updates)
	FreqTable             []uint16
	TogglePeriods         []uint16                // Toggle period per frequency (parallel to FreqTable)
	TogglePeriodSequences []TogglePeriodSequence  // Per-frame toggle periods for glissando notes
	GlissandoPatterns     []GlissandoPattern      // Frequency sweeps with per-frame deltas
	VibratoPattern        VibratoPattern          // Global vibrato pattern (applied to base freq)
	SustainPatterns       []SustainPattern        // Dynamic S value sequences for volume-following
	WaveformPatterns      []WaveformPattern       // Per-frame waveforms for mixer/envelope changes
	Instruments           []TrackerInstrument
	NotePatterns          []TrackerPattern
	Blocks                []TrackerBlock          // Reusable note sequences
	Streams               [3]TrackerStream        // Per-channel bytecode
}

type TogglePeriodSequence struct {
	Periods []uint16 // Toggle period per frame (for notes with tone glissando)
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

type SustainPattern struct {
	Values []uint8 // S parameter per frame (0-15) for volume-following ADSR
}

type WaveformPattern struct {
	Waveforms []byte // Waveform per frame (0x01/0x02/0x04/0x08) for mixer/envelope changes
}

type TrackerInstrument struct {
	BaseWaveform  byte   // Triangle, sawtooth, pulse, noise (NOT 0x05)
	EnvelopeMode  bool   // If true, player toggles between 0x05 and BaseWaveform
	NoisePlusTone bool   // If true, mixer state determines waveform (noise vs pulse)
	TogglePeriod  uint16 // SID cycles for waveform toggle (0 if no toggle needed)
	ADSR          ADSRParams
	VibratoDepth  byte // Frequency delta in Hz
	VibratoSpeed  byte // Frames per cycle
	VibratoDelay  byte // Frames before vibrato starts
}

type TrackerPattern struct {
	FreqIdx                 byte
	Duration                byte
	InstrumentID            byte
	FreqSeqIndex            byte // 0xFF=none, 0xFE=vibrato, 0-17=glissando pattern index
	SustainPatternIndex     byte // 0xFF=none (use static S from ADSR), 0-N=sustain pattern index
	WaveformPatternIndex    byte // 0xFF=none (use static waveform), 0-N=waveform pattern index
	TogglePeriodSeqIndex    byte // 0xFF=none (use constant from FreqTable), 0-N=toggle period sequence index
	GateOffFrame            byte // 0=no tail decay, 1-255=frame to switch D from 0 to ADSR.Decay
}

type TrackerBlock struct {
	Patterns []byte // Pattern IDs (variable length)
}

type TrackerStream []byte // Bytecode: 0x00-0x7F = play block, 0x80+ = commands

// NoteWithMetadata extends Note with tracker-specific metadata
type NoteWithMetadata struct {
	Note            Note
	Channel         int
	StartFrame      int      // Absolute frame position
	freq            uint16
	freqSeq         []uint16 // Per-frame frequencies (envelope/noise/tone periods)
	togglePeriodSeq []uint16 // Per-frame toggle periods in SID cycles (envelope/NoisePlusTone only)
	waveformSeq     []byte   // Per-frame waveforms (from R13/mixer changes)
	dur             byte
	instrumentID    int
	patternID       int
	isRest          bool     // True for silence/rest patterns
}

// ExtractTrackerFormat converts Note structures and AY commands to tracker format
//
// ARCHITECTURE: This is the EXTRACTOR/ENCODER - it analyzes the AY command stream and
// encodes all necessary playback data into a compact tracker format.
//
// RESPONSIBILITIES:
// - Read AY commands (R0-R13, mixer, volume registers) to extract all playback data
// - Store per-frame frequencies in freqSeq (envelope periods, noise periods, tone periods)
// - Store per-frame waveforms in waveformSeq (from R13 changes, mixer state changes)
// - Store per-frame sustain levels in SustainPatterns (for volume-following ADSR)
// - Deduplicate patterns and build compressed representation
//
// OUTPUT: A tracker format that contains ALL data needed for playback without AY commands
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

	// Build frequency table and toggle periods from all unique base frequencies
	freqTable, togglePeriods := buildFrequencyTableFromMetadata(allNotes)
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

	// Build sustain patterns (for volume-following ADSR)
	sustainPatterns, sustainPatternMap := buildSustainPatterns(allNotes)

	// Build waveform patterns (for R13 envelope shape and R7 mixer changes)
	waveformPatterns, waveformPatternMap := buildWaveformPatterns(allNotes)

	// Build toggle period sequences (for envelope/NoisePlusTone notes with tone glissando)
	togglePeriodSequences, togglePeriodSeqMap := buildTogglePeriodSequences(allNotes)

	// Analyze toggle periods for envelope/NoisePlusTone notes
	analyzeTogglePeriods(allNotes)

	// Build note patterns
	notePatterns, notePatternMap := buildNotePatternsFromMetadata(allNotes, freqIndex, freqSeqMap, vibratoPattern, sustainPatternMap, waveformPatternMap, togglePeriodSeqMap)

	// Assign pattern IDs
	for i := range allNotes {
		baseFreq := allNotes[i].freq
		var freqIdx byte
		freqSeqIndex := byte(0xFF) // 0xFF = no modulation (same as buildNotePatternsFromMetadata)

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
					// Try to match as glissando pattern first (works for both monotonic and non-monotonic)
					startFreq := allNotes[i].freqSeq[0]
					key := fmt.Sprintf("%d", startFreq)
					for j := 1; j < len(allNotes[i].freqSeq); j++ {
						delta := int16(allNotes[i].freqSeq[j]) - int16(allNotes[i].freqSeq[j-1])
						key += fmt.Sprintf(",%d", delta)
					}

					if idx, exists := freqSeqMap[key]; exists {
						freqSeqIndex = byte(idx)
					} else if !isMonotonic {
						// Non-monotonic and not in glissando map: must be vibrato
						freqSeqIndex = 0xFE
					}
				}
			}
		}

		// Find sustain pattern index
		sustainPatternIndex := byte(0xFF) // 0xFF = no pattern
		if len(allNotes[i].Note.SustainPerFrame) > 0 {
			sustainKey := ""
			for _, s := range allNotes[i].Note.SustainPerFrame {
				if sustainKey != "" {
					sustainKey += ","
				}
				sustainKey += fmt.Sprintf("%d", s)
			}
			if idx, exists := sustainPatternMap[sustainKey]; exists {
				sustainPatternIndex = byte(idx)
			}
		}

		// Find waveform pattern index
		waveformPatternIndex := byte(0xFF) // 0xFF = no pattern (use static waveform)
		if len(allNotes[i].waveformSeq) > 0 {
			waveformKey := ""
			for _, w := range allNotes[i].waveformSeq {
				if waveformKey != "" {
					waveformKey += ","
				}
				waveformKey += fmt.Sprintf("%d", w)
			}
			if idx, exists := waveformPatternMap[waveformKey]; exists {
				waveformPatternIndex = byte(idx)
			}
		}

		// Find toggle period sequence index (for varying toggle periods)
		togglePeriodSeqIndex := byte(0xFF) // 0xFF = use constant from FreqTable
		if len(allNotes[i].togglePeriodSeq) > 0 {
			// Check if toggle period varies
			isConstant := true
			firstPeriod := allNotes[i].togglePeriodSeq[0]
			for _, period := range allNotes[i].togglePeriodSeq[1:] {
				if period != firstPeriod {
					isConstant = false
					break
				}
			}

			if !isConstant {
				// Build key from toggle period sequence
				toggleKey := ""
				for _, p := range allNotes[i].togglePeriodSeq {
					if toggleKey != "" {
						toggleKey += ","
					}
					toggleKey += fmt.Sprintf("%d", p)
				}
				if idx, exists := togglePeriodSeqMap[toggleKey]; exists {
					togglePeriodSeqIndex = byte(idx)
				}
			}
		}

		// GateOffFrame: when to switch D from 0 to ADSR.Decay
		gateOffFrame := byte(0)
		if allNotes[i].Note.GateOffFrame > 0 && allNotes[i].Note.GateOffFrame < 256 {
			gateOffFrame = byte(allNotes[i].Note.GateOffFrame)
		}

		key := fmt.Sprintf("%d_%d_%d_%d_%d_%d_%d_%d_%v", freqIdx, allNotes[i].dur, allNotes[i].instrumentID, freqSeqIndex, sustainPatternIndex, waveformPatternIndex, togglePeriodSeqIndex, gateOffFrame, allNotes[i].isRest)
		allNotes[i].patternID = notePatternMap[key]

	}

	// Build dynamic-length blocks and bytecode streams
	blocks, streams := buildBlocksFromMetadata(allNotes, notesByChannel, &notePatterns, notePatternMap, tickRate)

	format := &TrackerFormat{
		StartOffset:           startOffset,
		TickRate:              tickRate,
		FreqTable:             freqTable,
		TogglePeriods:         togglePeriods,
		TogglePeriodSequences: togglePeriodSequences,
		GlissandoPatterns:     glissandoPatterns,
		VibratoPattern:        vibratoPattern,
		SustainPatterns:       sustainPatterns,
		WaveformPatterns:      waveformPatterns,
		Instruments:           instruments,
		NotePatterns:          notePatterns,
		Blocks:                blocks,
		Streams:               streams,
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

		toneFreqPerFrame := make(map[int]uint16)
		currentToneFreq := uint16(0)

		envFreqPerFrame := make(map[int]uint16)
		currentEnvFreq := uint16(0)

		noiseFreqPerFrame := make(map[int]uint16)
		currentNoiseFreq := uint16(0)

		// Build R13 (envelope shape) and R7 (mixer) tables for waveform changes
		envelopeShapePerFrame := make(map[int]byte)
		currentEnvShape := byte(0)

		mixerPerFrame := make(map[int]byte)
		currentMixer := byte(0xFF) // All disabled by default

		for _, cmd := range commands {
			frame := int(cmd.Frame)
			switch cmd.Register {
			case toneLoReg:
				currentToneFreq = (currentToneFreq & 0xFF00) | uint16(cmd.Value)
				toneFreqPerFrame[frame] = currentToneFreq
			case toneHiReg:
				currentToneFreq = (currentToneFreq & 0x00FF) | (uint16(cmd.Value&0x0F) << 8)
				toneFreqPerFrame[frame] = currentToneFreq
			case 6: // Noise period
				currentNoiseFreq = uint16(cmd.Value & 0x1F) // 5-bit value
				noiseFreqPerFrame[frame] = currentNoiseFreq
			case 7: // Mixer register
				currentMixer = cmd.Value
				mixerPerFrame[frame] = currentMixer
			case 11: // Envelope period low
				currentEnvFreq = (currentEnvFreq & 0xFF00) | uint16(cmd.Value)
				envFreqPerFrame[frame] = currentEnvFreq
			case 12: // Envelope period high
				currentEnvFreq = (currentEnvFreq & 0x00FF) | (uint16(cmd.Value) << 8)
				envFreqPerFrame[frame] = currentEnvFreq
			case 13: // Envelope shape
				currentEnvShape = cmd.Value
				envelopeShapePerFrame[frame] = currentEnvShape
			}
		}

		// Forward-fill envelope shape and mixer state
		lastEnvShape := byte(0)
		lastMixer := byte(0xFF)
		maxFrame := 0
		for _, note := range notesByChannel[ch] {
			if note.EndFrame > maxFrame {
				maxFrame = note.EndFrame
			}
		}
		for frame := 0; frame <= maxFrame; frame++ {
			if shape, ok := envelopeShapePerFrame[frame]; ok {
				lastEnvShape = shape
			} else if lastEnvShape != 0 {
				envelopeShapePerFrame[frame] = lastEnvShape
			}

			if mixer, ok := mixerPerFrame[frame]; ok {
				lastMixer = mixer
			} else {
				mixerPerFrame[frame] = lastMixer
			}
		}

		// Process each note on this channel
		for _, note := range notesByChannel[ch] {
			// Build waveform sequence first for NoisePlusTone notes (needed to select frequency source)
			waveformSeq := []byte{}
			if note.EnvelopeMode {
				// Envelope mode: waveform changes based on R13 (envelope shape) register
				for frame := note.StartFrame; frame <= note.EndFrame; frame++ {
					var waveform byte
					if shape, ok := envelopeShapePerFrame[frame]; ok && shape != 0 {
						waveform = ayEnvelopeToSIDWaveform(shape)
					} else {
						waveform = note.Waveform // Default to base waveform
					}
					waveformSeq = append(waveformSeq, waveform)
				}
			} else if note.NoisePlusTone {
				// NoisePlusTone: waveform changes based on R7 (mixer) register
				for frame := note.StartFrame; frame <= note.EndFrame; frame++ {
					mixer, ok := mixerPerFrame[frame]
					if !ok {
						mixer = 0xFF // Default: all disabled
					}

					toneBit := uint8(ch)        // Bit 0=A, 1=B, 2=C
					noiseBit := uint8(3 + ch)   // Bit 3=A, 4=B, 5=C
					toneOn := (mixer & (1 << toneBit)) == 0
					noiseOn := (mixer & (1 << noiseBit)) == 0

					var waveform byte
					if toneOn && !noiseOn {
						waveform = 0x04 // Tone-only: pulse
					} else {
						waveform = 0x08 // Noise-only or mixed: noise
					}
					waveformSeq = append(waveformSeq, waveform)
				}
			}
			// For other notes, waveformSeq remains empty (player uses static waveform)

			// Extract frequency sequence for this note
			// Convert AY periods to SID frequency register values immediately
			// For NoisePlusTone: switch between tone and noise frequency based on waveform per frame
			// For envelope mode: use envelope period
			// For static waveform: use tone or noise period based on base waveform
			freqSeq := []uint16{}
			baseFreq := uint16(0)

			for i, frame := 0, note.StartFrame; frame <= note.EndFrame; i, frame = i+1, frame+1 {
				var sidFreq uint16

				if note.NoisePlusTone && len(waveformSeq) > i {
					// NoisePlusTone: select frequency source based on waveform at this frame
					if waveformSeq[i] == 0x08 {
						// Noise: convert noise period to SID frequency
						if noisePeriod, ok := noiseFreqPerFrame[frame]; ok {
							sidFreq = computeNoiseFrequency(byte(noisePeriod & 0x1F))
						}
					} else {
						// Pulse: convert tone period to SID frequency
						if tonePeriod, ok := toneFreqPerFrame[frame]; ok {
							sidFreq = computeToneFrequency(tonePeriod)
						}
					}
				} else if note.EnvelopeMode {
					// Envelope mode: convert envelope period to SID frequency
					if envPeriod, ok := envFreqPerFrame[frame]; ok {
						sidFreq = computeEnvelopeFrequency(envPeriod)
						// Sawtooth envelopes run at 2x frequency
						if note.Waveform == 0x02 {
							sidFreq *= 2
						}
					}
				} else if note.Waveform == 0x08 {
					// Pure noise: convert noise period to SID frequency
					if noisePeriod, ok := noiseFreqPerFrame[frame]; ok {
						sidFreq = computeNoiseFrequency(byte(noisePeriod & 0x1F))
					}
				} else {
					// Tone: convert tone period to SID frequency
					if tonePeriod, ok := toneFreqPerFrame[frame]; ok {
						sidFreq = computeToneFrequency(tonePeriod)
					}
				}

				if sidFreq > 0 {
					freqSeq = append(freqSeq, sidFreq)
					if baseFreq == 0 {
						baseFreq = sidFreq
					}
				} else if len(freqSeq) > 0 {
					// Repeat last frequency if missing
					freqSeq = append(freqSeq, freqSeq[len(freqSeq)-1])
				}
			}

			if baseFreq == 0 {
				baseFreq = 1 // Default
			}

			// Calculate toggle periods for envelope/NoisePlusTone notes
			// Use the SAME calculation as direct player to ensure matching periods
			togglePeriodSeq := []uint16{}
			if note.EnvelopeMode || note.NoisePlusTone {
				ayClock := spectrumAYClockHz
				sidClock := sidClockHz

				for i, frame := 0, note.StartFrame; frame <= note.EndFrame; i, frame = i+1, frame+1 {
					var ayPeriod uint16

					// Get AY period (tone or noise) based on waveform
					if note.NoisePlusTone && len(waveformSeq) > i {
						if waveformSeq[i] == 0x08 {
							// Noise waveform
							if noisePeriod, ok := noiseFreqPerFrame[frame]; ok {
								// Noise uses different formula
								ayPeriod = noisePeriod
							}
						} else {
							// Pulse waveform
							if tonePeriod, ok := toneFreqPerFrame[frame]; ok {
								ayPeriod = tonePeriod
							}
						}
					} else if note.EnvelopeMode {
						// Envelope mode: use TONE period for toggle calculation (not envelope period)
						// Toggle happens at audio frequency (tone), envelope period only affects shape speed
						if tonePeriod, ok := toneFreqPerFrame[frame]; ok {
							ayPeriod = tonePeriod
						} else {
							// CRITICAL: Envelope mode must have tone period for toggle calculation
							// If tone period is missing, this is a data error
							panic(fmt.Sprintf("Envelope mode note at ch=%d frame=%d has no tone period (R0/R1 not set)", ch, frame))
						}
					}

					if ayPeriod > 0 {
						// Calculate toggle period from AY frequency (matches direct player)
						var ayFreq float64
						if note.NoisePlusTone && len(waveformSeq) > i && waveformSeq[i] == 0x08 {
							// Noise frequency calculation
							ayFreq = ayClock / (16.0 * float64(ayPeriod))
						} else {
							// Tone/envelope frequency calculation
							ayFreq = ayClock / (16.0 * float64(ayPeriod))
						}

						// NOTE: Sawtooth 2x frequency multiplier is NOT applied to toggle period
						// The direct player only uses 2x for SID frequency (visual), not toggle timing
						// Toggle happens at base audio frequency (tone period), regardless of waveform

						// Apply resampling correction
						targetToggleFreq := ayFreq * (palFrameRate / spectrumFrameRate)

						// Toggle period in SID clock cycles
						idealTogglePeriod := sidClock / (2.0 * targetToggleFreq)
						togglePeriod := uint16(idealTogglePeriod + 0.5)
						if togglePeriod < 1 {
							togglePeriod = 1
						}

						togglePeriodSeq = append(togglePeriodSeq, togglePeriod)
					} else if len(togglePeriodSeq) > 0 {
						// Repeat last period if missing
						togglePeriodSeq = append(togglePeriodSeq, togglePeriodSeq[len(togglePeriodSeq)-1])
					}
				}
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
				Note:            note,
				Channel:         ch,
				StartFrame:      adjustedStartFrame, // Keep in frames, not ticks
				freq:            baseFreq,
				freqSeq:         freqSeq,
				togglePeriodSeq: togglePeriodSeq,
				waveformSeq:     waveformSeq,
				dur:             byte(durationFrames), // Store in frames for accuracy
				isRest:          false,
			})
		}
	}

	return allNotes
}

func buildInstrumentsFromMetadata(notes []NoteWithMetadata) ([]TrackerInstrument, map[string]int) {
	instrumentMap := make(map[string]int)
	instruments := []TrackerInstrument{}

	// First pass: create instruments
	for _, note := range notes {
		key := makeInstrumentKeyFromMetadata(note, nil)

		if _, exists := instrumentMap[key]; !exists {
			instrumentMap[key] = len(instruments)

			inst := TrackerInstrument{
				BaseWaveform:  note.Note.Waveform,
				EnvelopeMode:  note.Note.EnvelopeMode,
				NoisePlusTone: note.Note.NoisePlusTone,
				TogglePeriod:  0, // Will be calculated in second pass
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

	// Second pass: calculate toggle periods for envelope/NoisePlusTone instruments
	// Collect toggle period counts per instrument (for constant-frequency notes only)
	instrumentTogglePeriods := make(map[int]map[uint16]int) // instrumentID -> togglePeriod -> count

	for _, note := range notes {
		if !note.Note.EnvelopeMode && !note.Note.NoisePlusTone {
			continue // Skip non-toggle instruments
		}

		if len(note.togglePeriodSeq) == 0 {
			continue // Skip notes without toggle periods
		}

		// Check if toggle period is constant
		isConstant := true
		firstPeriod := note.togglePeriodSeq[0]
		for _, period := range note.togglePeriodSeq[1:] {
			if period != firstPeriod {
				isConstant = false
				break
			}
		}

		if !isConstant {
			continue // Skip varying toggle periods (will calculate on-the-fly)
		}

		// Find instrument ID for this note
		key := makeInstrumentKeyFromMetadata(note, nil)
		instID := instrumentMap[key]

		// Record toggle period for this instrument
		if instrumentTogglePeriods[instID] == nil {
			instrumentTogglePeriods[instID] = make(map[uint16]int)
		}
		instrumentTogglePeriods[instID][firstPeriod]++
		if instID == 0 && len(instrumentTogglePeriods[instID]) == 1 {
			fmt.Printf("  INSTRUMENT 0: first toggle period = %d\n", firstPeriod)
		}
	}

	// Assign most common toggle period to each instrument
	for instID, periods := range instrumentTogglePeriods {
		maxCount := 0
		mostCommonPeriod := uint16(0)
		allPeriods := []uint16{}
		for period, count := range periods {
			allPeriods = append(allPeriods, period)
			if count > maxCount {
				maxCount = count
				mostCommonPeriod = period
			}
		}
		instruments[instID].TogglePeriod = mostCommonPeriod
		if instID == 0 {
			fmt.Printf("  INSTRUMENT 0: periods=%v counts=%v, chose %d (count=%d)\n",
				allPeriods, periods, mostCommonPeriod, maxCount)
		}
	}

	return instruments, instrumentMap
}

func makeInstrumentKeyFromMetadata(note NoteWithMetadata, instruments []TrackerInstrument) string {
	// Vibrato disabled - only use waveform, envelope mode, noise+tone, and ADSR
	return fmt.Sprintf("%d_%v_%v_%d_%d_%d_%d_%d",
		note.Note.Waveform,
		note.Note.EnvelopeMode,
		note.Note.NoisePlusTone,
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
			// Check if this oscillation matches the vibrato pattern EXACTLY
			matchesVibrato := false
			if len(note.freqSeq) >= 8 { // Need at least one full vibrato cycle
				baseFreq := note.freq
				vibratoOffsets := []int8{-2, -4, -2, 0, 2, 4, 2, 0}

				// Try to find vibrato pattern starting after delay
				for startFrame := 0; startFrame < min(20, len(note.freqSeq)-8); startFrame++ {
					matches := 0
					for i := 0; i < 8 && startFrame+i < len(note.freqSeq); i++ {
						expectedFreq := int(baseFreq) + int(vibratoOffsets[i])
						actualFreq := int(note.freqSeq[startFrame+i])
						if actualFreq == expectedFreq { // Exact match only (tolerance=0)
							matches++
						}
					}
					if matches == 8 { // All 8 frames must match exactly
						matchesVibrato = true
						break
					}
				}
			}

			if matchesVibrato {
				continue // Skip vibrato notes (handled globally)
			}
			// Fall through to create glissando pattern for non-matching oscillations
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

	// Debug: print first few glissando patterns
	for i := 0; i < min(10, len(patterns)); i++ {
		pattern := patterns[i]
		fmt.Printf("    Gliss #%d: start=%d deltas[0-9]=", i, pattern.StartFreq)
		for j := 0; j < min(10, len(pattern.Deltas)); j++ {
			fmt.Printf("%d ", pattern.Deltas[j])
		}
		fmt.Printf("(len=%d)\n", len(pattern.Deltas))
		// Check if this is truly constant (all deltas are 0)
		hasNonZero := false
		for _, d := range pattern.Deltas {
			if d != 0 {
				hasNonZero = true
				break
			}
		}
		if !hasNonZero && len(pattern.Deltas) > 0 {
			fmt.Printf("      ^ WARNING: Constant pattern stored as glissando!\n")
		}
	}

	return patterns, glissandoMap, vibratoPattern
}

func buildSustainPatterns(notes []NoteWithMetadata) ([]SustainPattern, map[string]int) {
	sustainMap := make(map[string]int)
	patterns := []SustainPattern{}

	for _, note := range notes {
		if note.isRest || len(note.Note.SustainPerFrame) == 0 {
			continue // Skip rests and notes without sustain data
		}

		// Create unique key from sustain sequence
		key := ""
		for _, s := range note.Note.SustainPerFrame {
			if key != "" {
				key += ","
			}
			key += fmt.Sprintf("%d", s)
		}

		// Add pattern if not seen before
		if _, exists := sustainMap[key]; !exists {
			sustainMap[key] = len(patterns)
			patterns = append(patterns, SustainPattern{
				Values: append([]uint8(nil), note.Note.SustainPerFrame...), // Copy slice
			})
		}
	}

	fmt.Printf("  Sustain patterns: %d unique sequences\n", len(patterns))

	return patterns, sustainMap
}

func buildTogglePeriodSequences(notes []NoteWithMetadata) ([]TogglePeriodSequence, map[string]int) {
	toggleMap := make(map[string]int)
	sequences := []TogglePeriodSequence{}

	for _, note := range notes {
		if note.isRest || len(note.togglePeriodSeq) == 0 {
			continue // Skip rests and notes without toggle periods
		}

		// Check if toggle period is constant
		isConstant := true
		firstPeriod := note.togglePeriodSeq[0]
		for _, period := range note.togglePeriodSeq[1:] {
			if period != firstPeriod {
				isConstant = false
				break
			}
		}

		if isConstant {
			continue // Skip constant toggle periods (handled by frequency table lookup)
		}

		// Create unique key from toggle period sequence
		key := ""
		for _, p := range note.togglePeriodSeq {
			if key != "" {
				key += ","
			}
			key += fmt.Sprintf("%d", p)
		}

		// Add sequence if not seen before
		if _, exists := toggleMap[key]; !exists {
			toggleMap[key] = len(sequences)
			sequences = append(sequences, TogglePeriodSequence{
				Periods: append([]uint16(nil), note.togglePeriodSeq...), // Copy slice
			})
		}
	}

	fmt.Printf("  Toggle period sequences: %d unique patterns (for tone glissando notes)\n", len(sequences))

	return sequences, toggleMap
}

func buildWaveformPatterns(notes []NoteWithMetadata) ([]WaveformPattern, map[string]int) {
	waveformMap := make(map[string]int)
	patterns := []WaveformPattern{}

	for _, note := range notes {
		if note.isRest || len(note.waveformSeq) == 0 {
			continue // Skip rests and notes without waveform changes
		}

		// Create unique key from waveform sequence
		key := ""
		for _, w := range note.waveformSeq {
			if key != "" {
				key += ","
			}
			key += fmt.Sprintf("%d", w)
		}

		// Add pattern if not seen before
		if _, exists := waveformMap[key]; !exists {
			waveformMap[key] = len(patterns)
			patterns = append(patterns, WaveformPattern{
				Waveforms: append([]byte(nil), note.waveformSeq...), // Copy slice
			})
		}
	}

	fmt.Printf("  Waveform patterns: %d unique sequences\n", len(patterns))

	return patterns, waveformMap
}

func buildNotePatternsFromMetadata(notes []NoteWithMetadata, freqIndex map[uint16]byte, freqSeqMap map[string]int, vibratoPattern VibratoPattern, sustainPatternMap map[string]int, waveformPatternMap map[string]int, togglePeriodSeqMap map[string]int) ([]TrackerPattern, map[string]int) {
	patternMap := make(map[string]int)
	patterns := []TrackerPattern{}

	glissandoAttempts := 0
	glissandoMatches := 0

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
					// Try to match as glissando pattern first (works for both monotonic and non-monotonic)
					glissandoAttempts++
					startFreq := note.freqSeq[0]
					key := fmt.Sprintf("%d", startFreq)
					for j := 1; j < len(note.freqSeq); j++ {
						delta := int16(note.freqSeq[j]) - int16(note.freqSeq[j-1])
						key += fmt.Sprintf(",%d", delta)
					}

					if idx, exists := freqSeqMap[key]; exists {
						freqSeqIndex = byte(idx)
						glissandoMatches++
					} else if !isMonotonic {
						// Non-monotonic and not in glissando map: must be vibrato
						freqSeqIndex = 0xFE
					} else {
						fmt.Printf("⚠ Failed to find glissando pattern start=%d len=%d key=%s\n", note.StartFrame, len(note.freqSeq), key[:min(50, len(key))])
					}
				}
			}
		}

		// Find sustain pattern index
		sustainPatternIndex := byte(0xFF) // 0xFF = no pattern (use static S from ADSR)
		if len(note.Note.SustainPerFrame) > 0 {
			// Build key from sustain sequence
			sustainKey := ""
			for _, s := range note.Note.SustainPerFrame {
				if sustainKey != "" {
					sustainKey += ","
				}
				sustainKey += fmt.Sprintf("%d", s)
			}
			if idx, exists := sustainPatternMap[sustainKey]; exists {
				sustainPatternIndex = byte(idx)
			}
		}

		waveformPatternIndex := byte(0xFF) // 0xFF = no pattern (use static waveform)
		if len(note.waveformSeq) > 0 {
			// Build key from waveform sequence
			waveformKey := ""
			for _, w := range note.waveformSeq {
				if waveformKey != "" {
					waveformKey += ","
				}
				waveformKey += fmt.Sprintf("%d", w)
			}
			if idx, exists := waveformPatternMap[waveformKey]; exists {
				waveformPatternIndex = byte(idx)
			}
		}

		// Find toggle period sequence index (for varying toggle periods)
		togglePeriodSeqIndex := byte(0xFF) // 0xFF = use constant from FreqTable
		if len(note.togglePeriodSeq) > 0 {
			// Check if toggle period varies
			isConstant := true
			firstPeriod := note.togglePeriodSeq[0]
			for _, period := range note.togglePeriodSeq[1:] {
				if period != firstPeriod {
					isConstant = false
					break
				}
			}

			if !isConstant {
				// Build key from toggle period sequence
				toggleKey := ""
				for _, p := range note.togglePeriodSeq {
					if toggleKey != "" {
						toggleKey += ","
					}
					toggleKey += fmt.Sprintf("%d", p)
				}
				if idx, exists := togglePeriodSeqMap[toggleKey]; exists {
					togglePeriodSeqIndex = byte(idx)
				}
			}
		}

		// GateOffFrame: when to switch D from 0 to ADSR.Decay
		gateOffFrame := byte(0)
		if note.Note.GateOffFrame > 0 && note.Note.GateOffFrame < 256 {
			gateOffFrame = byte(note.Note.GateOffFrame)
		}

		key := fmt.Sprintf("%d_%d_%d_%d_%d_%d_%d_%d_%v", freqIdx, note.dur, note.instrumentID, freqSeqIndex, sustainPatternIndex, waveformPatternIndex, togglePeriodSeqIndex, gateOffFrame, note.isRest)
		if _, exists := patternMap[key]; !exists {
			patternMap[key] = len(patterns)
			patterns = append(patterns, TrackerPattern{
				FreqIdx:              freqIdx,
				Duration:             note.dur,
				InstrumentID:         byte(note.instrumentID),
				FreqSeqIndex:         freqSeqIndex,
				SustainPatternIndex:  sustainPatternIndex,
				WaveformPatternIndex: waveformPatternIndex,
				TogglePeriodSeqIndex: togglePeriodSeqIndex,
				GateOffFrame:         gateOffFrame,
			})
		}
	}

	fmt.Printf("  Glissando pattern matching: %d/%d successful (%.1f%%)\n",
		glissandoMatches, glissandoAttempts, float64(glissandoMatches)*100.0/float64(max(1, glissandoAttempts)))

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

func analyzeTogglePeriods(allNotes []NoteWithMetadata) {
	fmt.Printf("\n=== Toggle Period Analysis ===\n")

	notesWithToggle := 0
	totalToggleFrames := 0
	constantPeriods := 0
	varyingPeriods := 0

	// Collect all toggle period sequences
	type TogglePeriodInfo struct {
		length   int
		constant bool
		value    uint16     // For constant sequences
		deltas   []int16    // For varying sequences
		minVal   uint16
		maxVal   uint16
	}

	sequences := []TogglePeriodInfo{}
	freqToTogglePeriod := make(map[uint16]map[uint16]int) // freq -> togglePeriod -> count

	for _, note := range allNotes {
		if len(note.togglePeriodSeq) == 0 {
			continue
		}

		notesWithToggle++
		totalToggleFrames += len(note.togglePeriodSeq)

		info := TogglePeriodInfo{
			length: len(note.togglePeriodSeq),
			minVal: note.togglePeriodSeq[0],
			maxVal: note.togglePeriodSeq[0],
		}

		// Check if constant
		info.constant = true
		info.value = note.togglePeriodSeq[0]
		for _, v := range note.togglePeriodSeq[1:] {
			if v != info.value {
				info.constant = false
			}
			if v < info.minVal {
				info.minVal = v
			}
			if v > info.maxVal {
				info.maxVal = v
			}
		}

		if info.constant {
			constantPeriods++
			// Track frequency -> toggle period mapping
			baseFreq := note.freq
			togglePeriod := note.togglePeriodSeq[0]
			if freqToTogglePeriod[baseFreq] == nil {
				freqToTogglePeriod[baseFreq] = make(map[uint16]int)
			}
			freqToTogglePeriod[baseFreq][togglePeriod]++
		} else {
			varyingPeriods++
			// Calculate deltas
			info.deltas = make([]int16, len(note.togglePeriodSeq)-1)
			for i := 1; i < len(note.togglePeriodSeq); i++ {
				info.deltas[i-1] = int16(note.togglePeriodSeq[i]) - int16(note.togglePeriodSeq[i-1])
			}
		}

		sequences = append(sequences, info)
	}

	fmt.Printf("  Notes with toggle periods: %d/%d (%.1f%%)\n", notesWithToggle, len(allNotes), float64(notesWithToggle)*100/float64(len(allNotes)))
	fmt.Printf("  Total toggle frames: %d\n", totalToggleFrames)
	fmt.Printf("  Constant periods: %d (%.1f%%)\n", constantPeriods, float64(constantPeriods)*100/float64(notesWithToggle))
	fmt.Printf("  Varying periods: %d (%.1f%%)\n", varyingPeriods, float64(varyingPeriods)*100/float64(notesWithToggle))

	// Show examples of varying sequences
	if varyingPeriods > 0 {
		fmt.Printf("\n  Varying period examples:\n")
		count := 0
		for i, seq := range sequences {
			if !seq.constant && count < 5 {
				fmt.Printf("    Note %d: len=%d range=%d-%d deltas[0-9]=", i, seq.length, seq.minVal, seq.maxVal)
				for j := 0; j < len(seq.deltas) && j < 10; j++ {
					fmt.Printf("%d ", seq.deltas[j])
				}
				fmt.Printf("\n")
				count++
			}
		}
	}

	// Show distribution of sequence lengths
	lengthDist := make(map[int]int)
	for _, seq := range sequences {
		lengthDist[seq.length]++
	}
	fmt.Printf("\n  Sequence length distribution:\n")
	lengths := make([]int, 0, len(lengthDist))
	for l := range lengthDist {
		lengths = append(lengths, l)
	}
	sort.Ints(lengths)
	for _, l := range lengths {
		if lengthDist[l] > 0 {
			fmt.Printf("    len=%d: %d notes (%.1f%%)\n", l, lengthDist[l], float64(lengthDist[l])*100/float64(notesWithToggle))
		}
		if len(lengths) > 10 {
			// Only show first 10
			break
		}
	}

	fmt.Printf("\n  Frequency → Toggle Period relationship:\n")
	freqsWithMultiplePeriods := 0
	for freq, periods := range freqToTogglePeriod {
		if len(periods) > 1 {
			freqsWithMultiplePeriods++
			if freqsWithMultiplePeriods <= 5 {
				fmt.Printf("    freq=%d: ", freq)
				for period, count := range periods {
					fmt.Printf("%d(%dx) ", period, count)
				}
				fmt.Printf("\n")
			}
		}
	}
	if freqsWithMultiplePeriods == 0 {
		fmt.Printf("    ✓ Each frequency maps to exactly ONE toggle period\n")
		fmt.Printf("    → Can store toggle periods in frequency table!\n")
	} else {
		fmt.Printf("    ✗ %d frequencies have multiple toggle periods\n", freqsWithMultiplePeriods)
	}

	// Estimate storage cost
	// Option 1: uint16 per pattern (all patterns)
	totalPatterns := len(allNotes)
	cost1 := totalPatterns * 2

	// Option 2: Store sequences (deduplicated)
	uniqueSequences := make(map[string]bool)
	for _, note := range allNotes {
		if len(note.togglePeriodSeq) > 0 {
			key := fmt.Sprintf("%v", note.togglePeriodSeq)
			uniqueSequences[key] = true
		}
	}
	// Rough estimate: average 2 bytes per value + overhead
	cost2 := len(uniqueSequences) * 10 // Pattern overhead
	cost2 += totalToggleFrames * 2      // Actual values
	cost2 += notesWithToggle * 1        // Pattern index in each note

	// Option 3: Store in frequency table
	cost3 := len(freqToTogglePeriod) * 2 // uint16 per frequency
	cost3 += varyingPeriods * 30         // Rough estimate for varying sequences

	fmt.Printf("\n  Storage estimates:\n")
	fmt.Printf("    Option 1 (uint16 per pattern):   %d bytes\n", cost1)
	fmt.Printf("    Option 2 (sequence patterns):    ~%d bytes (with dedup)\n", cost2)
	fmt.Printf("    Option 3 (frequency table):      ~%d bytes\n", cost3)
	if freqsWithMultiplePeriods == 0 {
		fmt.Printf("    → Option 3 is smallest!\n")
	}
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

	// Sustain patterns: 1 byte count + (per pattern: 1 byte length + N bytes S values)
	size += 1
	for _, pattern := range format.SustainPatterns {
		size += 1 + len(pattern.Values)
	}

	// Instruments: 1 byte count + (11 bytes per instrument: wave, envMode, togglePeriod[2], A, D, S, R, ID, vibratoD, vibratoS, vibratoDelay)
	size += 1 + len(format.Instruments)*11

	// Note patterns: 1 byte count + (6 bytes per pattern: freqIdx, duration, instrumentID, freqSeqIndex, sustainPatternIndex, gateOffFrame)
	size += 1 + len(format.NotePatterns)*6

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
	sustainSize := 1
	for _, pattern := range format.SustainPatterns {
		sustainSize += 1 + len(pattern.Values)
	}
	instrumentSize := 1 + len(format.Instruments)*11 // 11 bytes: wave, envMode, togglePeriod[2], ADSR[5], vibrato[3]
	notePatternSize := 1 + len(format.NotePatterns)*6  // 6 bytes per pattern (freqIdx, dur, instrID, freqSeqIdx, sustainIdx, gateOffFrame)
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
	fmt.Printf("    Sustain patterns: %4d bytes (%d patterns)\n", sustainSize, len(format.SustainPatterns))
	fmt.Printf("    Instruments:      %4d bytes (%d × 11)\n", instrumentSize, len(format.Instruments))
	fmt.Printf("    Note patterns:    %4d bytes (%d × 6)\n", notePatternSize, len(format.NotePatterns))
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
		toggleStr := ""
		if inst.TogglePeriod > 0 {
			toggleStr = fmt.Sprintf(" toggle=%d", inst.TogglePeriod)
		}
		fmt.Printf("    %d: wave=0x%02X envelope=%v%s ADSR=%d/%d/%d/%d\n",
			i, inst.BaseWaveform, inst.EnvelopeMode, toggleStr,
			inst.ADSR.Attack, inst.ADSR.Decay, inst.ADSR.Sustain, inst.ADSR.Release)
	}

	// Skip detailed analysis (was specific to old phrase/order format)
	return
}
func buildFrequencyTableFromMetadata(notes []NoteWithMetadata) ([]uint16, []uint16) {
	// Build frequency table (all unique frequencies)
	freqMap := make(map[uint16]bool)
	for _, n := range notes {
		if n.freq > 0 {
			freqMap[n.freq] = true
		}
	}

	// Build toggle period map (only for envelope/NoisePlusTone notes)
	freqToToggle := make(map[uint16]uint16)
	for _, n := range notes {
		if n.freq > 0 && len(n.togglePeriodSeq) > 0 {
			// Use first toggle period (all should be same for constant-freq notes)
			freqToToggle[n.freq] = n.togglePeriodSeq[0]
		}
	}

	// Sort frequencies
	freqs := []uint16{}
	for f := range freqMap {
		freqs = append(freqs, f)
	}
	sort.Slice(freqs, func(i, j int) bool {
		return freqs[i] < freqs[j]
	})

	// Build parallel toggle period array (0 for frequencies without toggle periods)
	togglePeriods := make([]uint16, len(freqs))
	for i, f := range freqs {
		if togglePeriod, exists := freqToToggle[f]; exists {
			togglePeriods[i] = togglePeriod
		} else {
			togglePeriods[i] = 0 // No toggle period (not envelope/NoisePlusTone)
		}
	}

	return freqs, togglePeriods
}
