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
	FreqSequenceTemplates []FreqSequenceTemplate  // Frequency sequence templates for drum patterns
	DrumSweepPattern      DrumSweepPattern        // Incremental drum sweep (base delta + multipliers)
	FreqModPatterns       []FreqModPattern        // 6502-optimized frequency modulation patterns
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

type FreqSequenceTemplate struct {
	Frequencies []uint16 // Frequency sequence (e.g., drum sweep pattern)
}

// FreqModPattern - 6502-friendly frequency modulation encoding
type FreqModPattern struct {
	Type   byte   // 0=DelayedArp, 1=DelayedBend, 2=Trill, 3=Portamento, 4=Complex, 5=SequenceLookup, 6=DrumSweep
	Delay  byte   // Frames to hold before modulation starts (0-255)
	Length byte   // Total pattern length in frames
	Data   []byte // Type-specific data
}

// Encoding formats (6502-optimized):
//
// DelayedArpeggio (Type 0):
//   Data[0] = base note index (0-41)
//   Data[1] = arpeggio pattern (4-bit pattern index, 4-bit repeat count)
//   Pattern 0: +0,+1,+2,+1 (most common from analysis)
//   Pattern 1: +0,-1,-2,-1
//   Pattern 2-15: additional patterns as needed
//   6502: LDA base_note,X; ADC arpeggio_offset[pattern],Y
//
// DelayedBend (Type 1):
//   Data[0] = start note index
//   Data[1+] = signed byte deltas (can be applied with ADC/SBC)
//   6502: Simple ADC loop, no multiply needed
//
// Trill (Type 2):
//   Data[0] = base note index
//   Data[1] = offset count (number of +/- steps)
//   Data[2] = signed offset in semitones
//   6502: ADD/SUB in loop, alternating sign
//
// Portamento (Type 3):
//   Data[0] = start note index
//   Data[1] = end note index
//   Data[2] = step count
//   6502: Linear interpolation with fraction tracking (no division)
//
// Complex (Type 4 - fallback):
//   Data[0,1] = start freq lo,hi
//   Data[2+] = signed 16-bit deltas (2 bytes each)
//
// SequenceLookup (Type 5):
//   Data[0] = template index (0-3 for the 4 drum patterns)
//   6502: LDA template_lo,X; LDA template_hi,X; loop and copy
//
// DrumSweep (Type 6):
//   Data[0,1] = start SID freq lo,hi
//   6502: freq[t] = freq[t-1] + (base_delta × multiplier[t])
//   Uses global DrumSweepPattern with base_delta and multiplier nibbles

type DrumSweepPattern struct {
	BaseDelta  byte   // Base frequency delta (133 Hz for drum sweeps)
	Multipliers []byte // Multiplier nibbles: 0=1.0x, 1=1.5x, 2=2.0x (2 per byte)
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

	// Analyze underlying effect parameters
	AnalyzeAllPatterns(allNotes)

	// Investigate what's generating "complex" patterns
	InvestigateComplexPatterns(commands, allNotes)
	AnalyzeComplexPatternSources(commands, allNotes)

	// Analyze per-frame changes for encoding patterns
	AnalyzePerFramePatterns(allNotes)
	DetectRepeatingPatterns(allNotes)

	// Test effect-based encoding
	effectPatterns, _, effectCount, complexCount := BuildEffectPatterns(allNotes, make(map[string]int))

	totalEffectBytes := 0
	for _, pattern := range effectPatterns {
		totalEffectBytes += EstimateEffectPatternSize(pattern)
	}

	fmt.Printf("\n=== Effect-based encoding estimate ===\n")
	fmt.Printf("  Effect patterns: %d (%.1f%% of total)\n", effectCount, float64(effectCount)*100.0/float64(effectCount+complexCount))
	fmt.Printf("  Complex patterns: %d (%.1f%% of total)\n", complexCount, float64(complexCount)*100.0/float64(effectCount+complexCount))
	fmt.Printf("  Effect pattern bytes: %d bytes for %d patterns (avg %.1f bytes/pattern)\n",
		totalEffectBytes, len(effectPatterns), float64(totalEffectBytes)/float64(max(1, len(effectPatterns))))
	fmt.Printf("  Estimated total: %d effect bytes + (Complex encoding for %d patterns)\n", totalEffectBytes, complexCount)

	// Build frequency table and toggle periods from all unique base frequencies
	freqTable, togglePeriods := buildFrequencyTableFromMetadata(allNotes)
	freqIndex := make(map[uint16]byte)
	for i, f := range freqTable {
		freqIndex[f] = byte(i)
	}

	// Analyze drum pattern algorithms
	AnalyzeDrumAlgorithm(allNotes, freqTable)

	// Build instruments with envelope mode information
	instruments, instrumentMap := buildInstrumentsFromMetadata(allNotes)

	// Assign instrument IDs to notes
	for i := range allNotes {
		key := makeInstrumentKeyFromMetadata(allNotes[i], instruments)
		allNotes[i].instrumentID = instrumentMap[key]
	}

	// Build optimized frequency modulation patterns (6502-friendly)
	freqModPatterns, freqSeqMap, vibratoPattern := buildFreqModPatterns6502(allNotes, freqTable)

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

	// Build frequency sequence templates from drum sequence templates
	freqSequenceTemplates := make([]FreqSequenceTemplate, len(drumSequenceTemplates))
	for i, template := range drumSequenceTemplates {
		freqSequenceTemplates[i] = FreqSequenceTemplate{
			Frequencies: template,
		}
	}

	// Build drum sweep pattern (incremental addition algorithm)
	// Pattern: 1.0, 1.0, 1.5, 1.0, 1.0, 1.0, 2.0, 1.0, 2.0
	// Encoded as nibbles: 0=1.0x, 1=1.5x, 2=2.0x
	drumSweepPattern := DrumSweepPattern{
		BaseDelta:   133, // 133 Hz base delta
		Multipliers: []byte{0x00, 0x01, 0x00, 0x02, 0x02}, // Nibbles packed: [0,0], [1,0], [0,0], [2,0], [2,x]
	}
	// Nibble encoding: multipliers[0]=0 (1.0x), [1]=0 (1.0x), [2]=1 (1.5x), [3]=0 (1.0x),
	//                  [4]=0 (1.0x), [5]=0 (1.0x), [6]=2 (2.0x), [7]=0 (1.0x), [8]=2 (2.0x)

	format := &TrackerFormat{
		StartOffset:           startOffset,
		TickRate:              tickRate,
		FreqTable:             freqTable,
		TogglePeriods:         togglePeriods,
		TogglePeriodSequences: togglePeriodSequences,
		FreqSequenceTemplates: freqSequenceTemplates,
		DrumSweepPattern:      drumSweepPattern,
		FreqModPatterns:       freqModPatterns,
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
				// NoisePlusTone: encode both Hi and Lo waveforms in a single byte
				// Format: (Hi << 4) | Lo
				// This allows the player to directly set both phases without complex state logic
				for frame := note.StartFrame; frame <= note.EndFrame; frame++ {
					mixer, ok := mixerPerFrame[frame]
					if !ok {
						mixer = 0xFF // Default: all disabled
					}

					toneBit := uint8(ch)        // Bit 0=A, 1=B, 2=C
					noiseBit := uint8(3 + ch)   // Bit 3=A, 4=B, 5=C
					toneOn := (mixer & (1 << toneBit)) == 0
					noiseOn := (mixer & (1 << noiseBit)) == 0

					var waveformHi, waveformLo byte
					if toneOn && !noiseOn {
						// Tone-only: both Hi and Lo are pulse (no modulation)
						waveformHi = 0x04
						waveformLo = 0x04
					} else if toneOn && noiseOn {
						// Tone+noise mixed: toggle between noise and silence for pulsed noise
						waveformHi = 0x08
						waveformLo = 0x05
					} else if !toneOn && noiseOn {
						// Noise-only: both Hi and Lo are noise (no modulation = constant noise)
						waveformHi = 0x08
						waveformLo = 0x08
					} else {
						// Both off: default to pulse
						waveformHi = 0x04
						waveformLo = 0x04
					}
					// Pack both waveforms into single byte: Hi in upper nibble, Lo in lower nibble
					waveform := (waveformHi << 4) | waveformLo
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
					// NoisePlusTone: waveform is packed as (Hi << 4) | Lo
					// Use Hi waveform to determine frequency source
					packedWaveform := waveformSeq[i]
					waveformHi := (packedWaveform >> 4) & 0x0F

					if waveformHi == 0x08 {
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

					// Get AY period for toggle calculation
					// CRITICAL: For both envelope mode AND NoisePlusTone, ALWAYS use TONE period
					// The toggle timing is based on the audio frequency (R0/R1), not the waveform
					if note.EnvelopeMode || note.NoisePlusTone {
						// Use TONE period for toggle calculation
						// Toggle happens at audio frequency (tone period), envelope/waveform only affects shape
						if tonePeriod, ok := toneFreqPerFrame[frame]; ok {
							ayPeriod = tonePeriod
							// BUG DETECTION: Zero tone period means envelope mode won't toggle!
							if ayPeriod == 0 && note.EnvelopeMode {
								panic(fmt.Sprintf("Envelope mode note at ch=%d frame=%d has ZERO tone period - will not toggle!", ch, frame))
							}
						} else if note.EnvelopeMode {
							// CRITICAL: Envelope mode must have tone period for toggle calculation
							// If tone period is missing, this is a data error
							panic(fmt.Sprintf("Envelope mode note at ch=%d frame=%d has no tone period (R0/R1 not set)", ch, frame))
						} else if note.NoisePlusTone {
							// CRITICAL: NoisePlusTone must also have tone period for toggle calculation
							panic(fmt.Sprintf("NoisePlusTone note at ch=%d frame=%d has no tone period (R0/R1 not set) - will play as constant noise!", ch, frame))
						}
					}

					if ayPeriod > 0 {
						// Calculate toggle period from AY frequency (matches direct player)
						// Always use tone frequency formula (ayPeriod is always tone period now)
						ayFreq := ayClock / (16.0 * float64(ayPeriod))

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
				// DEBUG: Check if NoisePlusTone notes have toggle periods
				if note.NoisePlusTone && len(togglePeriodSeq) == 0 {
					fmt.Printf("  DEBUG: NoisePlusTone note at ch=%d frame=%d has EMPTY togglePeriodSeq!\n", ch, note.StartFrame)
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
	fmt.Printf("  FreqMod patterns: %d entries\n", len(format.FreqModPatterns))
	fmt.Printf("  Instruments: %d entries\n", len(format.Instruments))
	fmt.Printf("  Note patterns: %d entries\n", len(format.NotePatterns))

	// Calculate estimated ROM size
	size := 0

	// Frequency table: 1 byte count + (2 bytes per entry)
	size += 1 + len(format.FreqTable)*2

	// FreqMod patterns: 1 byte count + (per pattern: 1 type + 1 delay + 1 length + N data bytes)
	size += 1 // count
	for _, pattern := range format.FreqModPatterns {
		size += 3 + len(pattern.Data) // type + delay + length + data
	}

	// FreqSequenceTemplates: 1 byte count + (per template: 1 byte length + N*2 bytes frequencies)
	size += 1
	for _, template := range format.FreqSequenceTemplates {
		size += 1 + len(template.Frequencies)*2
	}

	// DrumSweepPattern: 1 byte baseDelta + 1 byte count + N bytes multipliers
	size += 1 + 1 + len(format.DrumSweepPattern.Multipliers)

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
	freqModSize := 1
	for _, pattern := range format.FreqModPatterns {
		freqModSize += 3 + len(pattern.Data)
	}
	seqTemplateSize := 1
	for _, template := range format.FreqSequenceTemplates {
		seqTemplateSize += 1 + len(template.Frequencies)*2
	}
	drumSweepSize := 1 + 1 + len(format.DrumSweepPattern.Multipliers)
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
	fmt.Printf("    FreqMod patterns: %4d bytes (%d patterns, 6502-optimized)\n", freqModSize, len(format.FreqModPatterns))
	fmt.Printf("    Seq templates:    %4d bytes (%d templates for drum patterns)\n", seqTemplateSize, len(format.FreqSequenceTemplates))
	fmt.Printf("    Drum sweep:       %4d bytes (incremental addition algorithm)\n", drumSweepSize)
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

// ============================================================================
// 6502-Optimized FreqMod Pattern Builder
// ============================================================================

// Drum sequence templates (discovered from per-frame analysis)
var drumSequenceTemplates = [][]uint16{
	// Template 0: Drum sweep 1 (26 occurrences)
	{2831, 2047, 1603, 1209, 1039, 911, 811, 665, 610, 524},
	// Template 1: Noise burst (36 occurrences)
	{39403, 59104, 52673, 52673, 52673, 52673, 52673, 52673, 52673, 52673},
	// Template 2: Drum sweep 2 (24 occurrences)
	{2552, 1897, 1509, 1155, 999, 886, 793, 653, 599, 517},
	// Template 3: Modulation (1 occurrence)
	{14776, 2993, 2295, 13134, 14776},
}

// matchDrumSequence checks if a frequency sequence matches one of the drum templates
func matchDrumSequence(freqs []uint16) (int, bool) {
	for templateIdx, template := range drumSequenceTemplates {
		if len(freqs) < len(template) {
			continue
		}

		// Check if first len(template) frequencies match
		matches := true
		for i := 0; i < len(template); i++ {
			if freqs[i] != template[i] {
				matches = false
				break
			}
		}

		if matches {
			// Check if remaining frequencies (if any) hold the last template value
			if len(freqs) > len(template) {
				lastValue := template[len(template)-1]
				for i := len(template); i < len(freqs); i++ {
					if freqs[i] != lastValue {
						matches = false
						break
					}
				}
			}

			if matches {
				return templateIdx, true
			}
		}
	}

	return -1, false
}

// matchDrumSweep checks if a sequence is a drum sweep (templates 0 or 2)
// Returns true if it matches the normalized drum sweep curve
func matchDrumSweep(freqs []uint16) bool {
	// Drum sweeps are 10 frames long
	if len(freqs) != 10 {
		return false
	}

	// Templates 0 and 2 follow the same normalized decay curve
	// Normalized: 1.00, 0.72, 0.57, 0.43, 0.37, 0.32, 0.29, 0.23, 0.22, 0.19
	decayTable := []float64{1.000, 0.723, 0.566, 0.427, 0.367, 0.322, 0.286, 0.235, 0.215, 0.185}

	startFreq := float64(freqs[0])
	maxError := 0.0

	for i := 0; i < len(freqs); i++ {
		expected := startFreq * decayTable[i]
		actual := float64(freqs[i])
		errorVal := expected - actual
		if errorVal < 0 {
			errorVal = -errorVal
		}
		if errorVal > maxError {
			maxError = errorVal
		}
	}

	// Accept if error is < 5% of start frequency
	threshold := startFreq * 0.05
	return maxError < threshold
}

// buildFreqModPatterns6502 creates 6502-friendly frequency modulation patterns
// Focus: byte operations, table lookups, simple loops - no multiply/divide
func buildFreqModPatterns6502(notes []NoteWithMetadata, freqTable []uint16) ([]FreqModPattern, map[string]int, VibratoPattern) {
	freqModMap := make(map[string]int)
	patterns := []FreqModPattern{}

	// Build frequency index for note lookups (byte indices)
	freqIndex := make(map[uint16]byte)
	for i, f := range freqTable {
		freqIndex[f] = byte(i)
	}

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

		// Compute deltas
		freqs := note.freqSeq
		deltas := make([]int16, len(freqs)-1)
		for i := 1; i < len(freqs); i++ {
			deltas[i-1] = int16(freqs[i]) - int16(freqs[i-1])
		}

		// Count leading zeros (delay)
		delay := byte(0)
		for _, d := range deltas {
			if d == 0 {
				delay++
			} else {
				break
			}
		}

		var pattern FreqModPattern
		var key string

		// Check for drum sequence templates FIRST (exact match)
		if templateIdx, matches := matchDrumSequence(freqs); matches {
			// SEQUENCE LOOKUP - use template index for all drum templates
			pattern = FreqModPattern{
				Type:   5,
				Delay:  0,
				Length: byte(len(freqs)),
				Data:   []byte{byte(templateIdx)},
			}
			key = fmt.Sprintf("seq_%d_%d", templateIdx, len(freqs))

			// Create legacy key for lookup compatibility
			legacyKey := fmt.Sprintf("%d", freqs[0])
			for _, d := range deltas {
				legacyKey += fmt.Sprintf(",%d", d)
			}

			// Deduplicate
			if _, exists := freqModMap[key]; !exists {
				patternIdx := len(patterns)
				freqModMap[key] = patternIdx
				freqModMap[legacyKey] = patternIdx
				patterns = append(patterns, pattern)
			} else if _, exists := freqModMap[legacyKey]; !exists {
				freqModMap[legacyKey] = freqModMap[key]
			}

			continue // Skip other categorizations
		}

		// Check for vibrato (oscillation around base frequency)
		matchesVibrato := checkVibrato(freqs, note.freq)
		if matchesVibrato {
			continue // Skip - handled by global vibrato pattern
		}

		// Check if returns to start (trill)
		isTrill := freqs[0] == freqs[len(freqs)-1]

		// Check if monotonic (portamento)
		isMonotonic := true
		lastDir := 0
		for _, delta := range deltas {
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

		// Categorize pattern for 6502-friendly encoding
		if delay > byte(len(deltas)/2) {
			// DELAYED ARPEGGIO or DELAYED BEND
			// Check if all frequencies after delay are in the note table
			allInTable := true
			for i := int(delay); i < len(freqs); i++ {
				if _, ok := freqIndex[freqs[i]]; !ok {
					allInTable = false
					break
				}
			}

			if allInTable && len(freqs) > int(delay) {
				// All frequencies are discrete notes - collect indices
				noteIndices := make([]byte, 0, len(freqs)-int(delay))
				for i := int(delay); i < len(freqs); i++ {
					noteIndices = append(noteIndices, freqIndex[freqs[i]])
				}

				// DELAYED ARPEGGIO - use note table indices
				pattern = encodeDelayedArpeggio6502(delay, noteIndices, freqIndex[note.freq])
				key = fmt.Sprintf("darp_%d_%d_%d", delay, freqIndex[note.freq], len(noteIndices))
			} else {
				// Frequencies not all in table (glissando) - use Complex
				pattern = encodeComplex6502(freqs[0], deltas)
				key = fmt.Sprintf("complex_%d_%d", freqs[0], len(deltas))
			}
		} else if isTrill && !isMonotonic && len(freqs) > 2 {
			// TRILL - oscillate around base note and return to start
			// Check if all frequencies are in the note table
			allInTable := true
			for _, freq := range freqs {
				if _, ok := freqIndex[freq]; !ok {
					allInTable = false
					break
				}
			}

			if allInTable {
				// TRILL - use note-based oscillation
				pattern = encodeTrill6502(freqs, deltas, freqIndex)
				key = fmt.Sprintf("trill_%d_%d", freqs[0], len(deltas))
			} else {
				// Frequencies not in table - use Complex
				pattern = encodeComplex6502(freqs[0], deltas)
				key = fmt.Sprintf("complex_%d_%d", freqs[0], len(deltas))
			}
		} else if isMonotonic {
			// Check for PORTAMENTO (note-to-note slide)
			startIdx, startOk := freqIndex[freqs[0]]
			endIdx, endOk := freqIndex[freqs[len(freqs)-1]]

			if startOk && endOk && startIdx != endIdx {
				// PORTAMENTO - linear slide between notes
				pattern = encodePortamento6502(startIdx, endIdx, byte(len(freqs)))
				key = fmt.Sprintf("port_%d_%d_%d", startIdx, endIdx, len(freqs))
			} else {
				// COMPLEX - fallback to delta encoding
				pattern = encodeComplex6502(freqs[0], deltas)
				key = fmt.Sprintf("complex_%d_%d", freqs[0], len(deltas))
			}
		} else {
			// Try effect-based encoding first
			effectPattern := EncodeAsEffect(freqs, 0.05)
			if effectPattern.Type != 4 {
				// Verify reconstruction quality
				decoded := DecodeEffect(effectPattern)
				maxError := 0
				for i := 0; i < min(len(freqs), len(decoded)); i++ {
					error := int(freqs[i]) - int(decoded[i])
					if error < 0 {
						error = -error
					}
					if error > maxError {
						maxError = error
					}
				}

				// Use effect encoding if error is acceptable
				maxFreq := freqs[0]
				for _, f := range freqs {
					if f > maxFreq {
						maxFreq = f
					}
				}
				relativeError := float64(maxError) / float64(maxFreq)

				if relativeError < 0.10 { // 10% error threshold
					// Convert EffectPattern to FreqModPattern
					pattern = FreqModPattern{
						Type:   effectPattern.Type,
						Delay:  effectPattern.Delay,
						Length: effectPattern.Duration,
						Data: []byte{
							byte(effectPattern.StartFreq),
							byte(effectPattern.StartFreq >> 8),
							byte(effectPattern.EndFreq),
							byte(effectPattern.EndFreq >> 8),
							effectPattern.Param1,
							effectPattern.Param2,
						},
					}
					effectName := []string{"Linear", "Exponential", "Logarithmic", "Vibrato"}[effectPattern.Type]
					key = fmt.Sprintf("effect_%s_%d_%d_%d", effectName, effectPattern.StartFreq, effectPattern.EndFreq, effectPattern.Duration)
				} else {
					// Effect encoding error too high - use Complex
					pattern = encodeComplex6502(freqs[0], deltas)
					key = fmt.Sprintf("complex_%d_%d", freqs[0], len(deltas))
				}
			} else {
				// COMPLEX fallback (vibrato already checked above)
				pattern = encodeComplex6502(freqs[0], deltas)
				key = fmt.Sprintf("complex_%d_%d", freqs[0], len(deltas))
			}
		}

		// Create legacy key for lookup compatibility (startFreq,delta1,delta2,...)
		legacyKey := fmt.Sprintf("%d", freqs[0])
		for _, d := range deltas {
			legacyKey += fmt.Sprintf(",%d", d)
		}

		// Deduplicate using type-specific key, but also store under legacy key
		if _, exists := freqModMap[key]; !exists {
			patternIdx := len(patterns)
			freqModMap[key] = patternIdx
			freqModMap[legacyKey] = patternIdx // Also store under legacy key for lookup
			patterns = append(patterns, pattern)
		} else if _, exists := freqModMap[legacyKey]; !exists {
			// Pattern exists under type-specific key, add legacy key alias
			freqModMap[legacyKey] = freqModMap[key]
		}
	}

	// Print statistics
	stats := make(map[byte]int)
	totalBytes := 0
	for _, p := range patterns {
		stats[p.Type]++
		totalBytes += 3 + len(p.Data) // type + delay + length + data
	}

	fmt.Printf("  FreqMod patterns (6502-optimized): %d total, %d bytes\n", len(patterns), totalBytes)
	fmt.Printf("    Type 0 (DelayedArp): %d patterns\n", stats[0])
	fmt.Printf("    Type 1 (DelayedBend): %d patterns\n", stats[1])
	fmt.Printf("    Type 2 (Trill): %d patterns\n", stats[2])
	fmt.Printf("    Type 3 (Portamento): %d patterns\n", stats[3])
	fmt.Printf("    Type 4 (Complex): %d patterns\n", stats[4])
	fmt.Printf("    Type 5 (SequenceLookup): %d patterns\n", stats[5])
	fmt.Printf("    Type 6 (DrumSweep): %d patterns\n", stats[6])

	vibratoPattern := VibratoPattern{
		DelayFrames: 13,
		OffsetTable: []int8{-2, -4, -2, 0, 2, 4, 2, 0},
	}

	return patterns, freqModMap, vibratoPattern
}

// encodeDelayedArpeggio6502 - Type 0
// 6502 player: LDA freq_table,X where X comes from base_note + arpeggio_offset
func encodeDelayedArpeggio6502(delay byte, noteIndices []byte, baseNote byte) FreqModPattern {
	// Detect arpeggio pattern
	if len(noteIndices) == 0 {
		noteIndices = []byte{baseNote}
	}

	// Try to find repeating pattern
	patternType := byte(0xFF) // 0xFF = no match (will use custom pattern)
	repeatCount := byte(len(noteIndices))

	// Check for common 4-step patterns (must repeat perfectly with no remainder)
	if len(noteIndices) >= 4 && len(noteIndices)%4 == 0 {
		// Pattern 0: +0,+1,+2,+1 repeating
		offsets := make([]int8, min(4, len(noteIndices)))
		for i := 0; i < len(offsets); i++ {
			offsets[i] = int8(noteIndices[i]) - int8(baseNote)
		}

		// Check if entire sequence matches pattern (not just first 4)
		matchesFullSequence := false
		if matchesPattern(offsets, []int8{0, 1, 2, 1}) {
			matchesFullSequence = true
			for i := 4; i < len(noteIndices); i++ {
				expectedOffset := []int8{0, 1, 2, 1}[i%4]
				actualOffset := int8(noteIndices[i]) - int8(baseNote)
				if actualOffset != expectedOffset {
					matchesFullSequence = false
					break
				}
			}
			if matchesFullSequence {
				patternType = 0
				repeatCount = byte(len(noteIndices) / 4)
			}
		}

		if !matchesFullSequence && matchesPattern(offsets, []int8{0, -1, -2, -1}) {
			matchesFullSequence = true
			for i := 4; i < len(noteIndices); i++ {
				expectedOffset := []int8{0, -1, -2, -1}[i%4]
				actualOffset := int8(noteIndices[i]) - int8(baseNote)
				if actualOffset != expectedOffset {
					matchesFullSequence = false
					break
				}
			}
			if matchesFullSequence {
				patternType = 1
				repeatCount = byte(len(noteIndices) / 4)
			}
		}

		if !matchesFullSequence && matchesPattern(offsets, []int8{0, 2, 0, -2}) {
			matchesFullSequence = true
			for i := 4; i < len(noteIndices); i++ {
				expectedOffset := []int8{0, 2, 0, -2}[i%4]
				actualOffset := int8(noteIndices[i]) - int8(baseNote)
				if actualOffset != expectedOffset {
					matchesFullSequence = false
					break
				}
			}
			if matchesFullSequence {
				patternType = 2
				repeatCount = byte(len(noteIndices) / 4)
			}
		}
	}

	// Encode: base_note, pattern_and_count, [full sequence if no pattern match]
	data := []byte{baseNote, (patternType << 4) | (repeatCount & 0x0F)}

	// If no pattern matched (patternType == 0xFF), store full sequence
	if patternType == 0xFF {
		data = append(data, noteIndices...)
	}

	return FreqModPattern{
		Type:   0,
		Delay:  delay,
		Length: delay + byte(len(noteIndices)),
		Data:   data,
	}
}

// encodeDelayedBend6502 - Type 1
// 6502 player: Simple ADC/SBC loop with signed bytes
func encodeDelayedBend6502(delay byte, startFreq uint16, deltas []int16) FreqModPattern {
	// Try to encode as signed bytes where possible
	data := make([]byte, 0, 2+len(deltas)*2)

	// Store start frequency (16-bit)
	data = append(data, byte(startFreq), byte(startFreq>>8))

	// Check if all deltas fit in signed bytes (-127 to +127)
	allBytes := true
	for _, d := range deltas {
		if d < -127 || d > 127 {
			allBytes = false
			break
		}
	}

	if allBytes {
		// Encode as single bytes (6502: simple CLC/ADC or SEC/SBC)
		data = append(data, 0x01) // flag: byte deltas
		for _, d := range deltas {
			data = append(data, byte(d))
		}
	} else {
		// Encode as 16-bit deltas
		data = append(data, 0x02) // flag: word deltas
		for _, d := range deltas {
			data = append(data, byte(d), byte(d>>8))
		}
	}

	return FreqModPattern{
		Type:   1,
		Delay:  delay,
		Length: delay + byte(len(deltas)),
		Data:   data,
	}
}

// encodeTrill6502 - Type 2
// 6502 player: ADD offset, flip sign, repeat
func encodeTrill6502(freqs []uint16, deltas []int16, freqIndex map[uint16]byte) FreqModPattern {
	baseIdx, ok := freqIndex[freqs[0]]
	if !ok {
		baseIdx = 0
	}

	// Find oscillation amount (in semitones if possible)
	maxOffset := int8(0)
	for i := 1; i < len(freqs); i++ {
		if idx, ok := freqIndex[freqs[i]]; ok {
			offset := int8(idx) - int8(baseIdx)
			if offset < 0 {
				offset = -offset
			}
			if offset > maxOffset {
				maxOffset = offset
			}
		}
	}

	// Data: base_note, oscillation_amount (in semitones), repeat_count
	data := []byte{
		baseIdx,
		byte(maxOffset),
		byte(len(deltas)),
	}

	return FreqModPattern{
		Type:   2,
		Delay:  0,
		Length: byte(len(freqs)),
		Data:   data,
	}
}

// encodePortamento6502 - Type 3
// 6502 player: Bresenham-style interpolation (no division)
func encodePortamento6502(startNote, endNote, duration byte) FreqModPattern {
	// Data: start_note, end_note, step_count
	// 6502 uses Bresenham algorithm: accumulate (end-start), when >= duration, step
	data := []byte{startNote, endNote, duration}

	return FreqModPattern{
		Type:   3,
		Delay:  0,
		Length: duration,
		Data:   data,
	}
}

// encodeComplex6502 - Type 4 (fallback)
// Full 16-bit delta encoding for patterns that don't fit simpler encodings
func encodeComplex6502(startFreq uint16, deltas []int16) FreqModPattern {
	data := make([]byte, 2+len(deltas)*2)
	data[0] = byte(startFreq)
	data[1] = byte(startFreq >> 8)

	for i, d := range deltas {
		data[2+i*2] = byte(d)
		data[2+i*2+1] = byte(d >> 8)
	}

	return FreqModPattern{
		Type:   4,
		Delay:  0,
		Length: byte(len(deltas) + 1),
		Data:   data,
	}
}

// Helper functions

func matchesPattern(offsets, pattern []int8) bool {
	if len(offsets) < len(pattern) {
		return false
	}
	for i := 0; i < len(pattern); i++ {
		if offsets[i] != pattern[i] {
			return false
		}
	}
	return true
}

func checkVibrato(freqs []uint16, baseFreq uint16) bool {
	if len(freqs) < 8 {
		return false
	}

	vibratoOffsets := []int8{-2, -4, -2, 0, 2, 4, 2, 0}
	for startFrame := 0; startFrame < min(20, len(freqs)-8); startFrame++ {
		matches := 0
		for i := 0; i < 8 && startFrame+i < len(freqs); i++ {
			expectedFreq := int(baseFreq) + int(vibratoOffsets[i])
			actualFreq := int(freqs[startFrame+i])
			if actualFreq == expectedFreq {
				matches++
			}
		}
		if matches == 8 {
			return true
		}
	}
	return false
}
