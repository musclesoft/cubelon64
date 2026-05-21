// Tracker format extraction from optimized Note structures
package main

import (
	"fmt"
	"sort"

	"cubelon-emu/spectrum"
)

// Phrase: Reusable musical sequence (melody + rhythm, no effects)
type Phrase struct {
	Notes []PhraseNote
}

// PhraseNote: Single note in a phrase (pitch + duration only)
type PhraseNote struct {
	FreqIndex byte // Index into FreqTable (0xFF = rest)
	Duration  byte // Note duration in ticks
}

// PhraseInstance: How to render a phrase (effects + instrumentation)
type PhraseInstance struct {
	InstrumentIndex      byte // Index into Instruments
	FreqSeqIndex         byte // Index into FreqModPatterns (0xFF = none)
	SustainPatternIndex  byte // Index into SustainPatterns (0xFF = none)
	WaveformPatternIndex byte // Index into WaveformPatterns (0xFF = none)
	TogglePeriodIndex    byte // Index into TogglePeriodSequences (0xFF = none)
}

// TrackerFormat contains the complete music data for the player
type TrackerFormat struct {
	StartOffset              int                        // Frame offset before first note (e.g., 162)
	TickRate                 int                        // Frames per tick (e.g., 5 for 10Hz updates)
	FreqTable                []uint16
	TogglePeriods            []uint16                   // Toggle period per frequency (parallel to FreqTable)
	TogglePeriodSequences    []TogglePeriodSequence     // Per-frame toggle periods for glissando notes
	FreqSequenceTemplates    []FreqSequenceTemplate     // Frequency sequence templates for drum patterns
	DrumSweepPattern         DrumSweepPattern           // Incremental drum sweep (base delta + multipliers)
	DelayedVibratoTemplates  []DelayedVibratoTemplate   // Oscillation pattern templates for delayed vibrato
	FreqModPatterns          []FreqModPattern           // 6502-optimized frequency modulation patterns
	VibratoPattern           VibratoPattern             // Global vibrato pattern (applied to base freq)
	SustainPatterns          []SustainPattern           // Dynamic S value sequences for volume-following
	WaveformPatterns         []WaveformPattern          // Per-frame waveforms for mixer/envelope changes
	Instruments              []TrackerInstrument
	NotePatterns             []TrackerPattern           // Bundled patterns (melody + effects)
	FreqSeqOverrides         []TrackerOverride          // Sparse table: patternID → freqMod pattern index
	WaveformOverrides        []TrackerOverride          // Sparse table: patternID → waveform pattern index
	ToggleOverrides          []TrackerOverride          // Sparse table: patternID → toggle-period seq index
	Blocks                   []TrackerBlock             // Reusable note sequences
	Streams                  [3]TrackerStream           // Per-channel bytecode
}

type TogglePeriodSequence struct {
	Periods []uint16 // Toggle period per frame (for notes with tone glissando)
}

type FreqSequenceTemplate struct {
	Frequencies []uint16 // Frequency sequence (e.g., drum sweep pattern)
}

// FreqModPattern - 6502-friendly frequency modulation encoding
type FreqModPattern struct {
	Type   byte   // 0=DelayedArp, 1=DelayedBend, 2=Trill, 3=Portamento, 4=Complex, 5=SequenceLookup, 6=DrumSweep, 7=DelayedVibrato, 8=Hold, 9=LinearRamp
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
//
// DelayedVibrato (Type 7):
//   Data[0,1] = base AY period lo,hi
//   Data[2] = holdFrames (typically 12)
//   Data[3] = template index (0-11 for the 12 vibrato patterns)
//   6502: hold for N frames, then apply vibrato pattern from template
//
// Hold (Type 8):
//   Data[0,1] = constant frequency lo,hi
//   6502: output constant frequency for Length frames
//
// LinearRamp (Type 9):
//   Data[0,1] = start frequency lo,hi
//   Data[2,3] = delta per frame (signed 16-bit)
//   6502: freq[t] = freq[t-1] + delta (simple ADD instruction)
//   Encodes portamento/pitch bend with constant velocity

type DelayedVibratoTemplate struct {
	Deltas [8]int16 // 8-frame oscillation pattern (deltas from base frequency)
}

type DrumSweepPattern struct {
	BaseDelta  byte   // Base frequency delta (133 Hz for drum sweeps)
	Multipliers []byte // Multiplier nibbles: 0=1.0x, 1=1.5x, 2=2.0x (2 per byte)
}

type VibratoPattern struct {
	DelayFrames int     // Frames before vibrato starts (e.g., 13)
	OffsetTable []int8  // Per-frame offsets to base AY period (e.g., [-4,-2,0,+2,+4,+2,0,-2])
}

type SustainPattern struct {
	IsConstant  bool    // If true, Values[0] is repeated for the entire pattern
	IsDelta     bool    // If true, use delta encoding (2-bit deltas)
	StartValue  uint8   // Starting value (for delta encoding)
	Length      uint8   // Pattern length in frames
	Values      []uint8 // S parameter per frame (0-15), or delta-encoded (4 deltas per byte)
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
	FreqIdx             byte
	Duration            byte
	InstrumentID        byte
	SustainPatternIndex byte // 0xFF=none (use static S from ADSR), 0-N=sustain pattern index
	TailStartFrame      byte // 0=no tail decay, 1-255=frame to switch from dynamic-S to D-decay
}

// TrackerOverride maps a pattern ID to a sequence index.
// Used as a compact side table for sparse optional fields.
type TrackerOverride struct {
	PatternID byte
	SeqIndex  byte
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

	// Analysis: Find REAL repetition at multiple levels (disabled for production)
	// AnalyzeRealRepetition(allNotes)

	// Calculate optimal separated format (disabled for production)
	// CalculateSeparatedFormat(allNotes)

	// Analysis: Find underlying tracker mechanisms (disabled for production)
	// AnalyzeTrackerMechanisms(allNotes)

	// Optional: Analysis functions (commented out for production)
	// AnalyzeAllPatterns(allNotes)
	// InvestigateComplexPatterns(commands, allNotes)
	// AnalyzeComplexPatternSources(commands, allNotes)
	// AnalyzePerFramePatterns(allNotes)
	// DetectRepeatingPatterns(allNotes)
	// BuildEffectPatterns(allNotes, make(map[string]int))
	// AnalyzeDrumAlgorithm(allNotes, freqTable)
	// AnalyzeComplexFunctions(allNotes)
	// AnalyzeHoldOscillate(allNotes)

	// Optional: Analyze channel 0 loop structure
	// Requires: analyze_ch0_loop.go
	_ = startOffset // avoid unused warning

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

	// Build optimized frequency modulation patterns (6502-friendly)
	freqModPatterns, freqSeqMap, vibratoPattern, delayedVibratoTemplates := buildFreqModPatterns6502(allNotes, freqTable)

	// Build sustain patterns (for volume-following ADSR)
	sustainPatterns, sustainPatternMap := buildSustainPatterns(allNotes)

	// Build waveform patterns (for R13 envelope shape and R7 mixer changes)
	waveformPatterns, waveformPatternMap := buildWaveformPatterns(allNotes)

	// Build toggle period sequences (for envelope/NoisePlusTone notes with tone glissando)
	togglePeriodSequences, togglePeriodSeqMap := buildTogglePeriodSequences(allNotes)

	// Analyze toggle periods for envelope/NoisePlusTone notes
	analyzeTogglePeriods(allNotes)

	// Build note patterns
	notePatterns, notePatternMap, freqSeqOverrides, waveformOverrides, toggleOverrides := buildNotePatternsFromMetadata(allNotes, freqIndex, freqSeqMap, vibratoPattern, sustainPatternMap, waveformPatternMap, togglePeriodSeqMap)

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

		// TailStartFrame: when to switch D from 0 to ADSR.Decay
		gateOffFrame := byte(0)
		if allNotes[i].Note.TailStartFrame > 0 && allNotes[i].Note.TailStartFrame < 256 {
			gateOffFrame = byte(allNotes[i].Note.TailStartFrame)
		}

		key := fmt.Sprintf("%d_%d_%d_%d_%d_%d_%d_%d_%v", freqIdx, allNotes[i].dur, allNotes[i].instrumentID, freqSeqIndex, sustainPatternIndex, waveformPatternIndex, togglePeriodSeqIndex, gateOffFrame, allNotes[i].isRest)
		allNotes[i].patternID = notePatternMap[key]

	}

	// Analyze music structure to find patterns and loops
	//analyzeMusicStructure(allNotes, tickRate)

	// Find ALL repeated sequences (ignoring pattern boundaries)
	//findAllRepeatedSequences(allNotes, notePatterns, tickRate)

	// Design phrase-based format using actual repetition
	//designPhraseFormat(allNotes, notePatterns, tickRate)

	// Design tracker format from actual patterns
	//designTrackerFormat(allNotes, notePatterns, tickRate)

	// Analyze if patterns are redundant when ignoring instruments
	//analyzePatternRedundancy(allNotes, notePatterns, tickRate)

	// Analyze natural music structure (variable patterns, delta-time, phrases)
	//analyzeNaturalStructure(allNotes, notePatterns, tickRate)

	// Find transposed/key-changed patterns
	//findTransposedPatterns(allNotes, notePatterns, tickRate)

	// Calculate savings from rhythm/pitch separation
	//calculateRhythmSeparation(allNotes, notePatterns, tickRate)

	// Analyze remaining large tables for optimization
	//analyzeRemainingTables(allNotes, notePatterns, freqModPatterns)

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
		StartOffset:             startOffset,
		TickRate:                tickRate,
		FreqTable:               freqTable,
		TogglePeriods:           togglePeriods,
		TogglePeriodSequences:   togglePeriodSequences,
		FreqSequenceTemplates:   freqSequenceTemplates,
		DrumSweepPattern:        drumSweepPattern,
		DelayedVibratoTemplates: delayedVibratoTemplates,
		FreqModPatterns:         freqModPatterns,
		VibratoPattern:          vibratoPattern,
		SustainPatterns:         sustainPatterns,
		WaveformPatterns:        waveformPatterns,
		Instruments:             instruments,
		NotePatterns:            notePatterns,
		FreqSeqOverrides:        freqSeqOverrides,
		WaveformOverrides:       waveformOverrides,
		ToggleOverrides:         toggleOverrides,
		Blocks:                  blocks,
		Streams:                 streams,
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

	// Skip detailed analysis for now - focusing on structure
	_ = freqModPatterns
	_ = sustainPatterns

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

			sustain := note.Note.SustainPerFrame

			// Check if pattern is constant
			isConstant := true
			if len(sustain) > 1 {
				first := sustain[0]
				for _, v := range sustain[1:] {
					if v != first {
						isConstant = false
						break
					}
				}
			}

			if isConstant {
				// Constant pattern - just store the single value
				patterns = append(patterns, SustainPattern{
					IsConstant:  true,
					IsDelta:     false,
					StartValue:  sustain[0],
					Length:      uint8(len(sustain)),
					Values:      []uint8{sustain[0]},
				})
			} else {
				// Try delta encoding (2-bit deltas: -2, -1, 0, +1)
				// Pack 4 deltas per byte
				deltas := []int8{}
				canUseDelta := true

				for i := 1; i < len(sustain); i++ {
					delta := int8(sustain[i]) - int8(sustain[i-1])
					if delta < -2 || delta > 1 {
						canUseDelta = false
						break
					}
					deltas = append(deltas, delta)
				}

				if canUseDelta && len(sustain) > 4 {
					// Use delta encoding
					// Pack deltas: 00=-2, 01=-1, 10=0, 11=+1
					packed := []uint8{}
					for i := 0; i < len(deltas); i += 4 {
						b := uint8(0)
						for j := 0; j < 4 && i+j < len(deltas); j++ {
							delta := deltas[i+j]
							var code uint8
							switch delta {
							case -2:
								code = 0
							case -1:
								code = 1
							case 0:
								code = 2
							case 1:
								code = 3
							}
							b |= code << (j * 2)
						}
						packed = append(packed, b)
					}

					patterns = append(patterns, SustainPattern{
						IsConstant:  false,
						IsDelta:     true,
						StartValue:  sustain[0],
						Length:      uint8(len(sustain)),
						Values:      packed,
					})
				} else {
					// Store full sequence
					patterns = append(patterns, SustainPattern{
						IsConstant:  false,
						IsDelta:     false,
						StartValue:  sustain[0],
						Length:      uint8(len(sustain)),
						Values:      append([]uint8(nil), sustain...),
					})
				}
			}
		}
	}

	fmt.Printf("  Sustain patterns: %d unique sequences\n", len(patterns))

	// Analyze sustain patterns for compression opportunities
	if len(patterns) > 0 {
		fmt.Printf("\n=== Sustain Pattern Analysis ===\n")

		// Length distribution
		lengthDist := make(map[int]int)
		totalBytes := 0
		for _, pattern := range patterns {
			lengthDist[len(pattern.Values)]++
			totalBytes += len(pattern.Values)
		}

		fmt.Printf("Length distribution:\n")
		for length := 1; length <= 50; length++ {
			if count := lengthDist[length]; count > 0 {
				fmt.Printf("  len=%2d: %d patterns\n", length, count)
			}
		}
		fmt.Printf("Total sustain bytes: %d\n", totalBytes)

		// Check for constant patterns
		constantCount := 0
		linearCount := 0
		for _, pattern := range patterns {
			if len(pattern.Values) <= 1 {
				constantCount++
				continue
			}

			// Check if constant
			isConstant := true
			first := pattern.Values[0]
			for _, v := range pattern.Values[1:] {
				if v != first {
					isConstant = false
					break
				}
			}
			if isConstant {
				constantCount++
				continue
			}

			// Check if linear
			if len(pattern.Values) >= 2 {
				delta := int(pattern.Values[1]) - int(pattern.Values[0])
				isLinear := true
				for i := 2; i < len(pattern.Values); i++ {
					actualDelta := int(pattern.Values[i]) - int(pattern.Values[i-1])
					if actualDelta != delta {
						isLinear = false
						break
					}
				}
				if isLinear {
					linearCount++
				}
			}
		}

		fmt.Printf("Pattern types:\n")
		fmt.Printf("  Constant: %d/%d (%.1f%%)\n", constantCount, len(patterns), float64(constantCount)*100/float64(len(patterns)))
		fmt.Printf("  Linear:   %d/%d (%.1f%%)\n", linearCount, len(patterns), float64(linearCount)*100/float64(len(patterns)))
		fmt.Printf("  Other:    %d/%d (%.1f%%)\n", len(patterns)-constantCount-linearCount, len(patterns),
			float64(len(patterns)-constantCount-linearCount)*100/float64(len(patterns)))

		// Show samples
		fmt.Printf("\nSample patterns (first 5):\n")
		for i := 0; i < min(5, len(patterns)); i++ {
			fmt.Printf("  Pattern %d: len=%d values=[", i, len(patterns[i].Values))
			for j := 0; j < min(10, len(patterns[i].Values)); j++ {
				if j > 0 {
					fmt.Printf(" ")
				}
				fmt.Printf("%d", patterns[i].Values[j])
			}
			if len(patterns[i].Values) > 10 {
				fmt.Printf(" ...")
			}
			fmt.Printf("]\n")
		}
	}

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

func buildNotePatternsFromMetadata(notes []NoteWithMetadata, freqIndex map[uint16]byte, freqSeqMap map[string]int, vibratoPattern VibratoPattern, sustainPatternMap map[string]int, waveformPatternMap map[string]int, togglePeriodSeqMap map[string]int) ([]TrackerPattern, map[string]int, []TrackerOverride, []TrackerOverride, []TrackerOverride) {
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

		// TailStartFrame: when to switch D from 0 to ADSR.Decay
		gateOffFrame := byte(0)
		if note.Note.TailStartFrame > 0 && note.Note.TailStartFrame < 256 {
			gateOffFrame = byte(note.Note.TailStartFrame)
		}

		key := fmt.Sprintf("%d_%d_%d_%d_%d_%d_%d_%d_%v", freqIdx, note.dur, note.instrumentID, freqSeqIndex, sustainPatternIndex, waveformPatternIndex, togglePeriodSeqIndex, gateOffFrame, note.isRest)
		if _, exists := patternMap[key]; !exists {
			patternMap[key] = len(patterns)
			patterns = append(patterns, TrackerPattern{
				FreqIdx:             freqIdx,
				Duration:            note.dur,
				InstrumentID:        byte(note.instrumentID),
				SustainPatternIndex: sustainPatternIndex,
				TailStartFrame:      gateOffFrame,
			})
		}
	}

	// Build sparse override side tables from the pattern keys.
	freqSeqOverrides := []TrackerOverride{}
	waveformOverrides := []TrackerOverride{}
	toggleOverrides := []TrackerOverride{}
	for key, patternID := range patternMap {
		var freqIdx2, dur, instrID, freqSeqIdx, sustainIdx, waveformIdx, toggleIdx, gateOff int
		var isRest bool
		fmt.Sscanf(key, "%d_%d_%d_%d_%d_%d_%d_%d_%t", &freqIdx2, &dur, &instrID, &freqSeqIdx, &sustainIdx, &waveformIdx, &toggleIdx, &gateOff, &isRest)
		_ = freqIdx2
		if freqSeqIdx != 0xFF {
			freqSeqOverrides = append(freqSeqOverrides, TrackerOverride{PatternID: byte(patternID), SeqIndex: byte(freqSeqIdx)})
		}
		if waveformIdx != 0xFF {
			waveformOverrides = append(waveformOverrides, TrackerOverride{PatternID: byte(patternID), SeqIndex: byte(waveformIdx)})
		}
		if toggleIdx != 0xFF {
			toggleOverrides = append(toggleOverrides, TrackerOverride{PatternID: byte(patternID), SeqIndex: byte(toggleIdx)})
		}
	}
	sortOverrides := func(ovs []TrackerOverride) {
		sort.Slice(ovs, func(i, j int) bool { return ovs[i].PatternID < ovs[j].PatternID })
	}
	sortOverrides(freqSeqOverrides)
	sortOverrides(waveformOverrides)
	sortOverrides(toggleOverrides)

	fmt.Printf("  Glissando pattern matching: %d/%d successful (%.1f%%)\n",
		glissandoMatches, glissandoAttempts, float64(glissandoMatches)*100.0/float64(max(1, glissandoAttempts)))

	return patterns, patternMap, freqSeqOverrides, waveformOverrides, toggleOverrides
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

	// Debug: Find the ACTUAL loop structure
	if ch == 0 {
		fmt.Printf("\n=== Channel 0 Loop Detection ===\n")
		fmt.Printf("Total: %d patterns\n\n", len(stream))

		// Print ALL loops in tracker format
		// User confirmed sample positions:
		// Loop 0: sample 142525, Loop 1: 705605, Loop 2: 1268685, Loop 3: 1831765
		// This translates to: loop starts at pattern 15, length 61 patterns
		loopStart := 15
		blockLen := 61
		numLoops := (len(notesByChannelWithMeta[ch]) - loopStart) / blockLen

		fmt.Printf("Printing ALL loops in tracker format:\n")
		fmt.Printf("(897 patterns: 15-pattern intro + %d loops of 61 patterns)\n", numLoops)
		fmt.Printf("Loop starts at pattern %d (sample 142525, 3.23s)\n\n", loopStart)

		// Print in groups of 4 for readability
		for groupStart := 0; groupStart < numLoops; groupStart += 4 {
			groupEnd := groupStart + 4
			if groupEnd > numLoops {
				groupEnd = numLoops
			}

			// Print header
			fmt.Printf("Pos |")
			for loopNum := groupStart; loopNum < groupEnd; loopNum++ {
				fmt.Printf(" Loop %2d            |", loopNum)
			}
			fmt.Printf("\n----|")
			for loopNum := groupStart; loopNum < groupEnd; loopNum++ {
				fmt.Printf("-------------------|")
			}
			fmt.Printf("\n")

			// Print each position
			for offset := 0; offset < blockLen; offset++ {
				fmt.Printf("%3d |", offset)

				for loopNum := groupStart; loopNum < groupEnd; loopNum++ {
					blockStart := loopStart + loopNum * blockLen
					idx := blockStart + offset

					if idx < len(notesByChannelWithMeta[ch]) {
						note := notesByChannelWithMeta[ch][idx]
						if int(note.patternID) < len(*notePatterns) {
							pattern := (*notePatterns)[note.patternID]

							// Convert FreqIdx to note name
							noteName := "---"
							if pattern.FreqIdx > 0 && int(pattern.FreqIdx) < 60 {
								noteNames := []string{
									"---", // 0
									"C-1", "C#1", "D-1", "D#1", "E-1", "F-1", "F#1", "G-1", "G#1", "A-1", "A#1", "B-1", // 1-12
									"C-2", "C#2", "D-2", "D#2", "E-2", "F-2", "F#2", "G-2", "G#2", "A-2", "A#2", "B-2", // 13-24
									"C-3", "C#3", "D-3", "D#3", "E-3", "F-3", "F#3", "G-3", "G#3", "A-3", "A#3", "B-3", // 25-36
									"C-4", "C#4", "D-4", "D#4", "E-4", "F-4", "F#4", "G-4", "G#4", "A-4", "A#4", "B-4", // 37-48
									"C-5", "C#5", "D-5", "D#5", "E-5", "F-5", "F#5", "G-5", "G#5", "A-5", "A#5", "B-5", // 49-60
								}
								noteName = noteNames[pattern.FreqIdx]
							}

							fmt.Printf(" %-3s %2d I%d ", noteName, pattern.Duration, pattern.InstrumentID)
						} else {
							fmt.Printf(" ???            ")
						}
					} else {
						fmt.Printf(" END            ")
					}
					fmt.Printf("|")
				}
				fmt.Printf("\n")
			}
			fmt.Printf("\n")
		}
		fmt.Printf("\n")

		// Now let's check POSITION BY POSITION which ones match across loops
		// Skip loop 0 (it's muted/intro) and compare loops 1-14
		fmt.Printf("=== Position-by-position melody matching (loops 1-14, skipping loop 0) ===\n\n")

		matchingPositions := 0
		totalPositions := 0

		for pos := 0; pos < blockLen; pos++ {
			// Get the pitch at this position across loops 1-14
			pitches := []byte{}
			for loopNum := 1; loopNum < numLoops; loopNum++ {
				idx := loopStart + loopNum*blockLen + pos
				if idx < len(notesByChannelWithMeta[ch]) {
					note := notesByChannelWithMeta[ch][idx]
					if int(note.patternID) < len(*notePatterns) {
						pattern := (*notePatterns)[note.patternID]
						pitches = append(pitches, pattern.FreqIdx)
					}
				}
			}

			if len(pitches) >= 2 {
				// Check if all pitches are the same
				allSame := true
				first := pitches[0]
				for _, p := range pitches[1:] {
					if p != first {
						allSame = false
						break
					}
				}

				totalPositions++
				if allSame {
					matchingPositions++
				} else {
					// Count how many loops have the most common pitch
					pitchCounts := make(map[byte]int)
					for _, p := range pitches {
						pitchCounts[p]++
					}
					maxCount := 0
					var mostCommonPitch byte
					for p, count := range pitchCounts {
						if count > maxCount {
							maxCount = count
							mostCommonPitch = p
						}
					}

					// Show first few positions that DON'T match
					if totalPositions <= 10 {
						fmt.Printf("  Pos %2d: %d/%d loops match (pitch %d), all = %v\n",
							pos, maxCount, len(pitches), mostCommonPitch, pitches[:min(len(pitches), 14)])
					}
				}
			}
		}

		matchPct := float64(matchingPositions) / float64(totalPositions) * 100
		fmt.Printf("\nPositions where ALL loops have same pitch: %d/%d (%.1f%%)\n",
			matchingPositions, totalPositions, matchPct)

		// Now analyze: how many unique patterns exist at each position?
		fmt.Printf("\n=== Loop-based encoding analysis ===\n")
		fmt.Printf("For each position in the 61-pattern loop, count unique patterns across all loops:\n\n")

		totalUniquePatterns := 0
		maxVariants := 0

		for pos := 0; pos < blockLen; pos++ {
			// Get all patterns at this position across all loops
			patternsAtPos := make(map[int]bool)
			for loopNum := 0; loopNum < numLoops; loopNum++ {
				idx := loopStart + loopNum*blockLen + pos
				if idx < len(notesByChannelWithMeta[ch]) {
					note := notesByChannelWithMeta[ch][idx]
					patternsAtPos[note.patternID] = true
				}
			}

			numVariants := len(patternsAtPos)
			totalUniquePatterns += numVariants
			if numVariants > maxVariants {
				maxVariants = numVariants
			}

			if pos < 10 || numVariants > 3 {
				fmt.Printf("  Pos %2d: %d unique patterns\n", pos, numVariants)
			}
		}

		fmt.Printf("\nSummary:\n")
		fmt.Printf("  Total unique patterns across all 61 positions: %d\n", totalUniquePatterns)
		fmt.Printf("  Average variants per position: %.1f\n", float64(totalUniquePatterns)/float64(blockLen))
		fmt.Printf("  Max variants at any position: %d\n", maxVariants)
		fmt.Printf("\nEncoding scheme:\n")
		fmt.Printf("  Master loop: store %d unique pattern choices across 61 positions\n", totalUniquePatterns)
		fmt.Printf("  Per-loop selectors: 14 loops × 61 positions × log2(%d) bits\n", maxVariants)
		fmt.Printf("  If 4 variants per position: 14 × 61 × 2 bits = 214 bytes\n")
		fmt.Printf("\n  Current encoding: 897 patterns → 488 bytes (with blocks)\n")
		fmt.Printf("  Loop-based (all variants): 15 intro + (%d patterns + 214 selector bytes) = ~%d bytes\n",
			totalUniquePatterns, 15 + totalUniquePatterns + 214)

		// Better approach: store ONE master loop + per-loop OVERRIDES
		fmt.Printf("\n=== Master loop + overrides approach ===\n")

		// For each position, find the most common pattern
		masterLoop := make([]int, blockLen)
		for pos := 0; pos < blockLen; pos++ {
			patternCounts := make(map[int]int)
			for loopNum := 0; loopNum < numLoops; loopNum++ {
				idx := loopStart + loopNum*blockLen + pos
				if idx < len(notesByChannelWithMeta[ch]) {
					note := notesByChannelWithMeta[ch][idx]
					patternCounts[note.patternID]++
				}
			}

			// Find most common pattern
			maxCount := 0
			mostCommon := 0
			for patternID, count := range patternCounts {
				if count > maxCount {
					maxCount = count
					mostCommon = patternID
				}
			}
			masterLoop[pos] = mostCommon
		}

		// Now count how many overrides each loop needs
		totalOverrides := 0
		for loopNum := 0; loopNum < numLoops; loopNum++ {
			overrideCount := 0
			for pos := 0; pos < blockLen; pos++ {
				idx := loopStart + loopNum*blockLen + pos
				if idx < len(notesByChannelWithMeta[ch]) {
					note := notesByChannelWithMeta[ch][idx]
					if note.patternID != masterLoop[pos] {
						overrideCount++
					}
				}
			}
			totalOverrides += overrideCount
			if loopNum < 5 {
				fmt.Printf("  Loop %2d: %2d overrides (%.1f%%)\n", loopNum, overrideCount, float64(overrideCount)/float64(blockLen)*100)
			}
		}

		fmt.Printf("  ...\n")
		fmt.Printf("  Total overrides across all 14 loops: %d\n", totalOverrides)
		fmt.Printf("\nEncoding:\n")
		fmt.Printf("  Master loop: 61 patterns = 61 bytes\n")
		fmt.Printf("  Overrides: %d × 2 bytes (pos + patternID) = %d bytes\n", totalOverrides, totalOverrides*2)
		fmt.Printf("  Total: 61 + %d = %d bytes\n", totalOverrides*2, 61 + totalOverrides*2)
		fmt.Printf("  Plus 15-pattern intro = %d bytes\n", 15 + 61 + totalOverrides*2)
		fmt.Printf("\n  SAVINGS: 488 → %d bytes = %d bytes saved!\n\n",
			15 + 61 + totalOverrides*2, 488 - (15 + 61 + totalOverrides*2))

		// Let me look at this differently - show the actual NOTES (pitch + instrument) at a few positions
		fmt.Printf("=== Analyzing position structure ===\n")
		fmt.Printf("Looking at what actually appears at each position (first 5 positions):\n\n")

		for pos := 0; pos < 5; pos++ {
			fmt.Printf("Position %d:\n", pos)

			type noteKey struct {
				pitch byte
				inst  byte
			}
			notesAtPos := make(map[noteKey][]int) // which loops have this note

			for loopNum := 0; loopNum < numLoops; loopNum++ {
				idx := loopStart + loopNum*blockLen + pos
				if idx < len(notesByChannelWithMeta[ch]) {
					note := notesByChannelWithMeta[ch][idx]
					if int(note.patternID) < len(*notePatterns) {
						pattern := (*notePatterns)[note.patternID]
						key := noteKey{pattern.FreqIdx, pattern.InstrumentID}
						notesAtPos[key] = append(notesAtPos[key], loopNum)
					}
				}
			}

			// Show which notes appear
			noteNames := []string{
				"---", // 0
				"C-1", "C#1", "D-1", "D#1", "E-1", "F-1", "F#1", "G-1", "G#1", "A-1", "A#1", "B-1",
				"C-2", "C#2", "D-2", "D#2", "E-2", "F-2", "F#2", "G-2", "G#2", "A-2", "A#2", "B-2",
				"C-3", "C#3", "D-3", "D#3", "E-3", "F-3", "F#3", "G-3", "G#3", "A-3", "A#3", "B-3",
				"C-4", "C#4", "D-4", "D#4", "E-4", "F-4", "F#4", "G-4", "G#4", "A-4", "A#4", "B-4",
			}

			for key, loops := range notesAtPos {
				noteName := "?"
				if int(key.pitch) < len(noteNames) {
					noteName = noteNames[key.pitch]
				}
				fmt.Printf("  %s I%d: loops %v\n", noteName, key.inst, loops)
			}
			fmt.Printf("\n")
		}

		// Show the pattern table - what unique patterns exist
		fmt.Printf("=== Pattern Table for Channel 0 ===\n")
		fmt.Printf("Total unique patterns: %d\n\n", len(*notePatterns))

		// Count usage of each pattern in channel 0
		patternUsage := make(map[int]int)
		for _, note := range notesByChannelWithMeta[ch] {
			patternUsage[note.patternID]++
		}

		// Show all patterns sorted by usage
		type patternInfo struct {
			id    int
			count int
		}
		var patternsInfo []patternInfo
		for id, count := range patternUsage {
			patternsInfo = append(patternsInfo, patternInfo{id, count})
		}

		// Sort by usage descending
		for i := 0; i < len(patternsInfo); i++ {
			for j := i + 1; j < len(patternsInfo); j++ {
				if patternsInfo[j].count > patternsInfo[i].count {
					patternsInfo[i], patternsInfo[j] = patternsInfo[j], patternsInfo[i]
				}
			}
		}

		fmt.Printf("Top 30 most used patterns:\n")
		fmt.Printf("ID  | Note Dur Inst | Usage | Pattern details\n")
		fmt.Printf("----|---------------|-------|------------------\n")

		for i := 0; i < 30 && i < len(patternsInfo); i++ {
			p := patternsInfo[i]
			if p.id < len(*notePatterns) {
				pattern := (*notePatterns)[p.id]

				// Convert FreqIdx to note name
				noteName := "---"
				if pattern.FreqIdx > 0 && int(pattern.FreqIdx) < 60 {
					noteNames := []string{
						"---", // 0
						"C-1", "C#1", "D-1", "D#1", "E-1", "F-1", "F#1", "G-1", "G#1", "A-1", "A#1", "B-1", // 1-12
						"C-2", "C#2", "D-2", "D#2", "E-2", "F-2", "F#2", "G-2", "G#2", "A-2", "A#2", "B-2", // 13-24
						"C-3", "C#3", "D-3", "D#3", "E-3", "F-3", "F#3", "G-3", "G#3", "A-3", "A#3", "B-3", // 25-36
						"C-4", "C#4", "D-4", "D#4", "E-4", "F-4", "F#4", "G-4", "G#4", "A-4", "A#4", "B-4", // 37-48
						"C-5", "C#5", "D-5", "D#5", "E-5", "F-5", "F#5", "G-5", "G#5", "A-5", "A#5", "B-5", // 49-60
					}
					noteName = noteNames[pattern.FreqIdx]
				}

				fmt.Printf("%3d | %-3s %3d I%d   | %5d | Sustain=%d\n",
					p.id, noteName, pattern.Duration, pattern.InstrumentID, p.count,
					pattern.SustainPatternIndex)
			}
		}
		fmt.Printf("\n")

		// Now show pattern sequences - what patterns appear together
		fmt.Printf("Common pattern sequences (4-pattern phrases):\n")
		type seqKey struct {
			p1, p2, p3, p4 int
		}
		seqCounts := make(map[seqKey]int)

		for i := 0; i+4 <= len(notesByChannelWithMeta[ch]); i++ {
			key := seqKey{
				notesByChannelWithMeta[ch][i].patternID,
				notesByChannelWithMeta[ch][i+1].patternID,
				notesByChannelWithMeta[ch][i+2].patternID,
				notesByChannelWithMeta[ch][i+3].patternID,
			}
			seqCounts[key]++
		}

		type seqInfoType struct {
			key   seqKey
			count int
		}
		var sequences []seqInfoType
		for key, count := range seqCounts {
			if count > 1 { // Only show sequences that repeat
				sequences = append(sequences, seqInfoType{key, count})
			}
		}

		// Sort by count
		for i := 0; i < len(sequences); i++ {
			for j := i + 1; j < len(sequences); j++ {
				if sequences[j].count > sequences[i].count {
					sequences[i], sequences[j] = sequences[j], sequences[i]
				}
			}
		}

		fmt.Printf("Top 20 repeating 4-pattern sequences:\n")
		for i := 0; i < 20 && i < len(sequences); i++ {
			seq := sequences[i]
			fmt.Printf("  [%3d %3d %3d %3d] repeats %2d times\n",
				seq.key.p1, seq.key.p2, seq.key.p3, seq.key.p4, seq.count)
		}
		fmt.Printf("\n")

		// Now search for ALL occurrences of this 64-pattern block in the entire stream
		fmt.Printf("Searching for all occurrences of this 64-pattern block:\n")
		referenceBlock := 312
		blockLen64 := 64

		// Build the reference melody sequence
		type melodyItem struct {
			freq byte
			dur  byte
			inst byte
		}
		refMelody := []melodyItem{}
		for i := 0; i < blockLen64; i++ {
			idx := referenceBlock + i
			if idx < len(notesByChannelWithMeta[ch]) {
				note := notesByChannelWithMeta[ch][idx]
				if int(note.patternID) < len(*notePatterns) {
					pattern := (*notePatterns)[note.patternID]
					refMelody = append(refMelody, melodyItem{pattern.FreqIdx, pattern.Duration, pattern.InstrumentID})
				}
			}
		}

		// Search for this sequence throughout the stream
		var occurrences []int
		for startPos := 0; startPos+blockLen64 <= len(notesByChannelWithMeta[ch]); startPos++ {
			matches := 0
			for offset := 0; offset < blockLen64; offset++ {
				idx := startPos + offset
				if idx < len(notesByChannelWithMeta[ch]) {
					note := notesByChannelWithMeta[ch][idx]
					if int(note.patternID) < len(*notePatterns) {
						pattern := (*notePatterns)[note.patternID]
						if offset < len(refMelody) {
							refItem := refMelody[offset]
							if pattern.FreqIdx == refItem.freq && pattern.Duration == refItem.dur && pattern.InstrumentID == refItem.inst {
								matches++
							}
						}
					}
				}
			}

			if matches == blockLen64 {
				occurrences = append(occurrences, startPos)
			}
		}

		fmt.Printf("  Found %d exact occurrences at positions: %v\n", len(occurrences), occurrences)
		fmt.Printf("  Total patterns in these blocks: %d × 64 = %d patterns\n", len(occurrences), len(occurrences)*64)
		fmt.Printf("  Storage: 64 bytes (master) + %d bytes (refs) = %d bytes\n",
			len(occurrences)*2, 64+len(occurrences)*2)
		fmt.Printf("  vs current: %d patterns inline\n\n", len(occurrences)*64)

		// Now let's look at the OTHER 64-pattern windows to see if they're variations
		// Compare windows at 0, 64, 128, 192, 256 to the reference
		fmt.Printf("Comparing other 64-pattern windows to reference [312:376):\n\n")
		compareWindows := []int{0, 64, 128, 192, 256, 480, 576, 640, 704, 768}

		for _, winStart := range compareWindows {
			if winStart+blockLen64 > len(notesByChannelWithMeta[ch]) {
				continue
			}

			// Count exact matches, rhythm matches (ignore pitch), instrument matches
			exactMatches := 0
			rhythmMatches := 0
			instrMatches := 0

			for offset := 0; offset < blockLen64; offset++ {
				idx := winStart + offset
				if idx < len(notesByChannelWithMeta[ch]) && offset < len(refMelody) {
					note := notesByChannelWithMeta[ch][idx]
					if int(note.patternID) < len(*notePatterns) {
						pattern := (*notePatterns)[note.patternID]
						refItem := refMelody[offset]

						if pattern.FreqIdx == refItem.freq && pattern.Duration == refItem.dur && pattern.InstrumentID == refItem.inst {
							exactMatches++
						}
						if pattern.Duration == refItem.dur {
							rhythmMatches++
						}
						if pattern.InstrumentID == refItem.inst {
							instrMatches++
						}
					}
				}
			}

			exactPct := float64(exactMatches) / float64(blockLen64) * 100
			rhythmPct := float64(rhythmMatches) / float64(blockLen64) * 100
			instrPct := float64(instrMatches) / float64(blockLen64) * 100

			fmt.Printf("  [%3d:%3d): exact=%2d/%d (%.0f%%), rhythm=%2d/%d (%.0f%%), instr=%2d/%d (%.0f%%)\n",
				winStart, winStart+blockLen64, exactMatches, blockLen64, exactPct,
				rhythmMatches, blockLen64, rhythmPct, instrMatches, blockLen64, instrPct)
		}

		fmt.Printf("\n")

		// Strategy: Try loop length 64 with DIFFERENT STARTING OFFSETS
		// Maybe the loop doesn't start at position 0!
		// Try different loop lengths - user says ~11 seconds per loop
		// Song: 897 patterns, 188 seconds total
		// 188s / 14 loops = 13.4s per loop
		// 13.4s / (188s/897 patterns) = 64 patterns per loop
		//
		// Try lengths 50-70 to see which has best correlation
		bestLoopLen := -1
		bestLoopCorr := 0.0

		for _, loopLen := range []int{50, 52, 54, 56, 58, 60, 62, 64, 66, 68, 70} {
		bestOffset := -1
		bestCorrelation := 0.0
		bestMatches := 0
		bestTotal := 0
		bestReps := 0

		for startOffset := 0; startOffset < loopLen && startOffset < 10; startOffset += 1 { // Try first 10 offsets
			// Check how many complete loops we can fit
			availablePatterns := len(stream) - startOffset
			numReps := availablePatterns / loopLen

			if numReps < 3 {
				continue
			}

			// For each position in the loop, check consistency across repetitions
			matches := 0
			total := 0

			for pos := 0; pos < loopLen; pos++ {
				type melodyKey struct {
					freqIdx  byte
					duration byte
				}
				melodies := []melodyKey{}

				for rep := 0; rep < numReps; rep++ {
					idx := startOffset + rep*loopLen + pos
					if idx < len(notesByChannelWithMeta[ch]) {
						note := notesByChannelWithMeta[ch][idx]
						if int(note.patternID) < len(*notePatterns) {
							pattern := (*notePatterns)[note.patternID]
							melodies = append(melodies, melodyKey{
								freqIdx:  pattern.FreqIdx,
								duration: pattern.Duration,
							})
						}
					}
				}

				if len(melodies) < 2 {
					continue
				}

				// Count matches to most common melody
				melodyCounts := make(map[melodyKey]int)
				for _, melody := range melodies {
					melodyCounts[melody]++
				}

				maxCount := 0
				for _, count := range melodyCounts {
					if count > maxCount {
						maxCount = count
					}
				}

				// Count this position as matching if MOST (>50%) repetitions have the same melody
				total++
				if maxCount > len(melodies)/2 {
					matches += maxCount // Weight by how many actually matched
				}
			}

			if total > 0 {
				correlation := float64(matches) / float64(total) * 100
				if correlation > bestCorrelation {
					bestCorrelation = correlation
					bestOffset = startOffset
					bestMatches = matches
					bestTotal = total
					bestReps = numReps
				}
			}
		}

		if bestCorrelation > bestLoopCorr {
			bestLoopCorr = bestCorrelation
			bestLoopLen = loopLen
		}

		if bestCorrelation > 0 {
			fmt.Printf("  Len=%2d best: offset=%2d corr=%.1f%% (%d/%d), reps=%d\n",
				loopLen, bestOffset, bestCorrelation, bestMatches, bestTotal, bestReps)
		}
		} // end loop length loop

		if bestLoopLen > 0 {
			fmt.Printf("\nBest loop length: %d with %.1f%% correlation\n\n", bestLoopLen, bestLoopCorr)
		} else {
			fmt.Printf("\nNo repeating loop structure detected\n\n")
		}
	}
	}

	// Find recurring sequences PER CHANNEL using BAR-ALIGNED detection
	// 4/4 time: 1 pattern = 1 bar, look at 4-bar boundaries
	type seqInfo struct {
		seq      string
		length   int
		count    int
		savings  int
		channel  int
		positions []int // positions where this sequence appears (for non-overlap check)
	}

	allSeqCandidates := []seqInfo{}

	for ch := 0; ch < 3; ch++ {
		stream := channelStreams[ch]

		// Try all phrase lengths from 4 to 64 (inclusive).
		phraseLengths := make([]int, 0, 61)
		for l := 4; l <= 64; l++ {
			phraseLengths = append(phraseLengths, l)
		}

		for _, seqLen := range phraseLengths {
			seqCounts := make(map[string][]int) // sequence -> positions

			// Check at every position (no bar-alignment constraint)
			for i := 0; i+seqLen <= len(stream); i++ {
				seq := string(stream[i : i+seqLen])
				seqCounts[seq] = append(seqCounts[seq], i)
			}

			// For each sequence, find the maximum set of non-overlapping occurrences
			for seq, positions := range seqCounts {
				if len(positions) <= 1 {
					continue
				}

				// Greedy: pick non-overlapping occurrences
				nonOverlapPositions := []int{positions[0]}
				for j := 1; j < len(positions); j++ {
					lastPos := nonOverlapPositions[len(nonOverlapPositions)-1]
					if positions[j] >= lastPos+seqLen {
						nonOverlapPositions = append(nonOverlapPositions, positions[j])
					}
				}

				count := len(nonOverlapPositions)
				if count <= 1 {
					continue
				}

				blockCost := 1 + seqLen
				savings := count*seqLen - count*1 - blockCost // 1 byte per block ref
				if savings > 0 {
					allSeqCandidates = append(allSeqCandidates, seqInfo{
						seq:       seq,
						length:    seqLen,
						count:     count,
						savings:   savings,
						channel:   ch,
						positions: nonOverlapPositions,
					})
				}
			}
		}

		// Count candidates for this channel
		thisChCount := 0
		chCandidates := []seqInfo{}
		for _, candidate := range allSeqCandidates {
			if candidate.channel == ch {
				thisChCount++
				chCandidates = append(chCandidates, candidate)
			}
		}
		fmt.Printf("  Ch %d: found %d bar-aligned candidates\n", ch, thisChCount)

		// Show top 3 candidates for this channel
		if len(chCandidates) > 0 {
			// Sort this channel's candidates by savings
			sort.Slice(chCandidates, func(i, j int) bool {
				return chCandidates[i].savings > chCandidates[j].savings
			})
			fmt.Printf("    Top 3:\n")
			for i := 0; i < 3 && i < len(chCandidates); i++ {
				c := chCandidates[i]
				fmt.Printf("      len=%d count=%d positions=%v\n", c.length, c.count, c.positions)
			}
			fmt.Printf("\n")
		}
	}

	seqList := allSeqCandidates

	// Debug: show top candidates per channel
	fmt.Printf("  Found %d candidate sequences\n", len(seqList))
	if len(seqList) > 0 {
		// Sort to see best
		sort.Slice(seqList, func(i, j int) bool {
			return seqList[i].savings > seqList[j].savings
		})
		fmt.Printf("  Top 10 by savings:\n")
		for i := 0; i < 10 && i < len(seqList); i++ {
			fmt.Printf("    len=%d count=%d savings=%d\n", seqList[i].length, seqList[i].count, seqList[i].savings)
		}

		// Check for long sequences
		fmt.Printf("  Sequences length >= 32:\n")
		longCount := 0
		for i := 0; i < len(seqList) && longCount < 10; i++ {
			if seqList[i].length >= 32 {
				fmt.Printf("    len=%d count=%d savings=%d\n", seqList[i].length, seqList[i].count, seqList[i].savings)
				longCount++
			}
		}
		if longCount == 0 {
			fmt.Printf("    (none found)\n")
		}
	}

	// Sort candidates: long blocks first (bonus), then by raw savings, then by
	// sequence content for a fully deterministic ordering across runs.
	sort.Slice(seqList, func(i, j int) bool {
		si, sj := seqList[i].savings, seqList[j].savings
		if seqList[i].length >= 32 {
			si += seqList[i].length * 10
		}
		if seqList[j].length >= 32 {
			sj += seqList[j].length * 10
		}
		if si != sj {
			return si > sj
		}
		if seqList[i].length != seqList[j].length {
			return seqList[i].length > seqList[j].length
		}
		return seqList[i].seq < seqList[j].seq
	})

	// Greedy selection: take top blocks from EACH channel.
	// The bytecode reserves 0x00-0x1F (32 IDs) for block refs; cap per-channel at 32
	// so the total stays well within the 32-entry limit after the "used" pass.
	maxBlocksPerChannel := 32
	minSavings := 40 // must save at least 40 bytes to be worth it
	selectedBlocks := make(map[int][]seqInfo) // channel -> blocks

	for _, candidate := range seqList {
		ch := candidate.channel
		if candidate.savings >= minSavings && len(selectedBlocks[ch]) < maxBlocksPerChannel {
			selectedBlocks[ch] = append(selectedBlocks[ch], candidate)
		}
	}

	// Combine back into single list
	seqList = []seqInfo{}
	for ch := 0; ch < 3; ch++ {
		seqList = append(seqList, selectedBlocks[ch]...)
	}

	// Build ordered block table (seqList index = block ID).
	tempSeqs := make([]string, len(seqList))
	tempBlocks := make([]TrackerBlock, len(seqList))
	for i, seqInf := range seqList {
		tempSeqs[i] = seqInf.seq
		patterns := make([]byte, len(seqInf.seq))
		copy(patterns, []byte(seqInf.seq))
		tempBlocks[i] = TrackerBlock{Patterns: patterns}
	}

	// First pass: find which block IDs are actually matched in each stream.
	// Longest match wins; tie-break by lower index (deterministic).
	usedBlockIDs := make([]bool, len(tempSeqs))
	for ch := 0; ch < 3; ch++ {
		stream := channelStreams[ch]
		for i := 0; i < len(stream); {
			matchedID := -1
			matchedLen := 0
			for id, seq := range tempSeqs {
				seqLen := len(seq)
				if i+seqLen <= len(stream) && string(stream[i:i+seqLen]) == seq {
					if seqLen > matchedLen || (seqLen == matchedLen && id < matchedID) {
						matchedID = id
						matchedLen = seqLen
					}
				}
			}
			if matchedID >= 0 {
				usedBlockIDs[matchedID] = true
				i += matchedLen
				continue
			}
			i++
		}
	}

	// Build final block table preserving seqList order (deterministic).
	oldToNew := make([]byte, len(tempSeqs))
	blockDict := make(map[string]byte)
	blocks := []TrackerBlock{}
	for i, seq := range tempSeqs {
		if usedBlockIDs[i] {
			oldToNew[i] = byte(len(blocks))
			blockDict[seq] = byte(len(blocks))
			blocks = append(blocks, tempBlocks[i])
		}
	}

	if len(blocks) > 32 {
		panic(fmt.Sprintf("block table overflow: %d blocks exceed 32-entry bytecode limit (IDs 0x00-0x1F)", len(blocks)))
	}
	fmt.Printf("  Selected %d blocks (from %d candidates)\n", len(blocks), len(tempBlocks))

	fmt.Printf("  Building %d blocks (lengths: ", len(blocks))
	for i := 0; i < len(blocks) && i < 5; i++ {
		// Show which channel this block is from
		blockCh := -1
		for j := 0; j < len(seqList) && j < i+1; j++ {
			if j == i {
				blockCh = seqList[j].channel
			}
		}
		fmt.Printf("%d(ch%d) ", len(blocks[i].Patterns), blockCh)
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
			// Try to match longest block at current position.
			// Iterate tempSeqs (ordered) for deterministic tie-breaking.
			matchedBlock := byte(0xFF)
			matchedLen := 0

			for oldID, seq := range tempSeqs {
				if !usedBlockIDs[oldID] {
					continue
				}
				seqLen := len(seq)
				if i+seqLen <= len(stream) && string(stream[i:i+seqLen]) == seq {
					if seqLen > matchedLen {
						matchedBlock = oldToNew[oldID]
						matchedLen = seqLen
					}
				}
			}

			if matchedBlock != 0xFF {
				// Emit block reference
				bytecode = append(bytecode, matchedBlock)
				i += matchedLen
				chBlockRefs++
			} else {
				// No block match - emit pattern directly
				// Encoding: 0x20+ = pattern ID
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

	// DelayedVibratoTemplates: 1 byte count + N*16 bytes templates (8 deltas × 2 bytes each)
	size += 1 + len(format.DelayedVibratoTemplates)*16

	// Vibrato pattern: 1 byte delay + 1 byte count + N bytes offsets
	size += 1 + 1 + len(format.VibratoPattern.OffsetTable)

	// Sustain patterns: 1 byte count + (per pattern: 1 byte length + N bytes S values)
	size += 1
	for _, pattern := range format.SustainPatterns {
		size += 1 + len(pattern.Values)
	}

	// Instruments: 1 byte count + (11 bytes per instrument: wave, envMode, togglePeriod[2], A, D, S, R, ID, vibratoD, vibratoS, vibratoDelay)
	size += 1 + len(format.Instruments)*11

	// Note patterns: 1 byte count + (5 bytes per pattern: freqIdx, duration, instrumentID, sustainPatternIndex, gateOffFrame)
	size += 1 + len(format.NotePatterns)*5
	// FreqSeq overrides: 1 byte count + (2 bytes per entry: patternID, seqIndex)
	size += 1 + len(format.FreqSeqOverrides)*2
	// Waveform overrides: 1 byte count + (2 bytes per entry: patternID, seqIndex)
	size += 1 + len(format.WaveformOverrides)*2
	// Toggle overrides: 1 byte count + (2 bytes per entry: patternID, seqIndex)
	size += 1 + len(format.ToggleOverrides)*2

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
	delayedVibratoSize := 1 + len(format.DelayedVibratoTemplates)*16 // 1 count + (8 deltas × 2 bytes each)
	vibratoSize := 1 + 1 + len(format.VibratoPattern.OffsetTable)
	sustainSize := 1 // Count byte
	for _, pattern := range format.SustainPatterns {
		// Header: 1 byte (flags: bit7=IsConstant, bit6=IsDelta, bits5-0=unused) + StartValue + Length
		sustainSize += 3 // flags, startValue, length
		if !pattern.IsConstant {
			sustainSize += len(pattern.Values) // Delta-encoded or full values
		}
	}
	instrumentSize := 1 + len(format.Instruments)*11 // 11 bytes: wave, envMode, togglePeriod[2], ADSR[5], vibrato[3]
	notePatternSize := 1 + len(format.NotePatterns)*5
	freqSeqOverrideSize := 1 + len(format.FreqSeqOverrides)*2
	waveformOverrideSize := 1 + len(format.WaveformOverrides)*2
	toggleOverrideSize := 1 + len(format.ToggleOverrides)*2
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
	fmt.Printf("    Delayed vibrato:  %4d bytes (%d templates × 16)\n", delayedVibratoSize, len(format.DelayedVibratoTemplates))
	fmt.Printf("    Vibrato:          %4d bytes\n", vibratoSize)
	fmt.Printf("    Sustain patterns: %4d bytes (%d patterns)\n", sustainSize, len(format.SustainPatterns))
	fmt.Printf("    Instruments:      %4d bytes (%d × 11)\n", instrumentSize, len(format.Instruments))
	fmt.Printf("    Note patterns:    %4d bytes (%d × 5)\n", notePatternSize, len(format.NotePatterns))
	fmt.Printf("    FreqSeq overrides:%4d bytes (%d entries)\n", freqSeqOverrideSize, len(format.FreqSeqOverrides))
	fmt.Printf("    Waveform overrides:%3d bytes (%d entries)\n", waveformOverrideSize, len(format.WaveformOverrides))
	fmt.Printf("    Toggle overrides: %4d bytes (%d entries)\n", toggleOverrideSize, len(format.ToggleOverrides))
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
// Requires EXACT match - the tracker uses precise sequences, not fuzzy approximations
func matchDrumSequence(freqs []uint16) (int, bool) {
	for templateIdx, template := range drumSequenceTemplates {
		// Try exact match (full template or longer)
		if len(freqs) >= len(template) {
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

		// Try partial match (at least 5 frames of template)
		if len(freqs) >= 5 && len(freqs) < len(template) {
			matches := true
			for i := 0; i < len(freqs); i++ {
				if freqs[i] != template[i] {
					matches = false
					break
				}
			}

			if matches {
				return templateIdx, true
			}
		}
	}

	return -1, false
}

// matchDelayedVibrato checks if a sequence is hold + delayed vibrato
// Returns (holdFrames, oscillation pattern, matches)
var debugDelayedVibratoRejects = 0
var debugDelayedVibratoSuccess = 0
const maxDebugOutput = 0 // Set to > 0 to enable debug output

func matchDelayedVibrato(freqs []uint16) (byte, [8]int16, bool) {
	var emptyPattern [8]int16

	if len(freqs) < 20 {
		if debugDelayedVibratoRejects < maxDebugOutput {
			fmt.Printf("  [DelayedVibrato] REJECT: len=%d (need >= 20)\n", len(freqs))
			debugDelayedVibratoRejects++
		}
		return 0, emptyPattern, false // Need at least hold + one vibrato cycle
	}

	// Find how many frames hold the initial frequency
	baseFreq := freqs[0]
	holdFrames := 0
	for i := 1; i < len(freqs) && i < 30; i++ {
		if freqs[i] == baseFreq {
			holdFrames++
		} else {
			break
		}
	}

	// Need at least 10 frames of hold (match analysis criteria)
	if holdFrames < 10 {
		if debugDelayedVibratoRejects < maxDebugOutput {
			fmt.Printf("  [DelayedVibrato] REJECT: baseFreq=%d holdFrames=%d (need >= 10)\n", baseFreq, holdFrames)
			debugDelayedVibratoRejects++
		}
		return 0, emptyPattern, false
	}

	// After hold, check for oscillation pattern with period 8
	oscStart := holdFrames + 1
	oscFrames := len(freqs) - oscStart

	// For short notes (< 8 frames after hold), try to match against existing templates
	// by checking if the PREFIX of the note matches a known vibrato pattern
	if oscFrames < 8 {
		if oscFrames < 3 {
			if debugDelayedVibratoRejects < maxDebugOutput {
				fmt.Printf("  [DelayedVibrato] REJECT: oscFrames=%d (need >= 3 for prefix match)\n", oscFrames)
				debugDelayedVibratoRejects++
			}
			return 0, emptyPattern, false
		}

		// Can't extract full pattern, but we can match against known templates
		// For now, reject - we'll handle this in a second pass after building templates
		if debugDelayedVibratoRejects < maxDebugOutput {
			fmt.Printf("  [DelayedVibrato] SHORT NOTE: oscStart=%d len=%d oscFrames=%d (prefix match needed)\n",
				oscStart, len(freqs), oscFrames)
			debugDelayedVibratoRejects++
		}
		return 0, emptyPattern, false
	}

	// Extract 8-frame oscillation pattern (deltas from base frequency)
	var pattern [8]int16
	for i := 0; i < 8; i++ {
		pattern[i] = int16(freqs[oscStart+i]) - int16(baseFreq)
	}

	// Verify pattern repeats throughout the ENTIRE note
	// Allow small conversion errors (up to 3 AY units) - AY/SID conversion introduces rounding
	const tolerance = 3
	numCycles := (oscFrames + 7) / 8 // Round up to include partial cycles

	for cycle := 1; cycle < numCycles; cycle++ {
		cycleStart := oscStart + cycle*8
		// For the last partial cycle, only check available frames (PREFIX MATCH)
		framesToCheck := 8
		if cycleStart+framesToCheck > len(freqs) {
			framesToCheck = len(freqs) - cycleStart
		}

		for i := 0; i < framesToCheck; i++ {
			delta := int16(freqs[cycleStart+i]) - int16(baseFreq)
			diff := pattern[i] - delta
			if diff < 0 {
				diff = -diff
			}
			if diff > tolerance {
				// Pattern doesn't match throughout entire note
				if debugDelayedVibratoRejects < maxDebugOutput {
					fmt.Printf("  [DelayedVibrato] REJECT: cycle=%d i=%d pattern[%d]=%d delta=%d diff=%d (tolerance=%d)\n",
						cycle, i, i, pattern[i], delta, diff, tolerance)
					fmt.Printf("    baseFreq=%d holdFrames=%d oscStart=%d len=%d pattern=%v\n",
						baseFreq, holdFrames, oscStart, len(freqs), pattern)
					debugDelayedVibratoRejects++
				}
				return 0, emptyPattern, false
			}
		}
	}

	if debugDelayedVibratoSuccess < maxDebugOutput {
		fmt.Printf("  [DelayedVibrato] SUCCESS: holdFrames=%d len=%d baseFreq=%d pattern=%v\n",
			holdFrames, len(freqs), baseFreq, pattern)
		debugDelayedVibratoSuccess++
	}
	return byte(holdFrames), pattern, true
}

func absInt16(x int16) int16 {
	if x < 0 {
		return -x
	}
	return x
}

func withinTolerance(actual, expected int16, tolerance float64) bool {
	diff := float64(actual - expected)
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
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
func buildFreqModPatterns6502(notes []NoteWithMetadata, freqTable []uint16) ([]FreqModPattern, map[string]int, VibratoPattern, []DelayedVibratoTemplate) {
	freqModMap := make(map[string]int)
	patterns := []FreqModPattern{}

	// Build frequency index for note lookups (byte indices)
	freqIndex := make(map[uint16]byte)
	for i, f := range freqTable {
		freqIndex[f] = byte(i)
	}

	totalNotes := 0
	notesWithFreqChange := 0
	drumMatches := 0
	vibratoAttempts := 0
	vibratoSuccess := 0
	complexFallback := 0

	// First pass: collect all delayed vibrato oscillation patterns
	type vibratoInfo struct {
		baseFreq   uint16
		holdFrames byte
		pattern    [8]int16
		noteIdx    int
	}
	vibratoPatterns := []vibratoInfo{}

	for noteIdx, note := range notes {
		if note.isRest || len(note.freqSeq) <= 1 {
			continue
		}

		totalNotes++

		// Check if frequency actually changes
		hasChange := false
		firstFreq := note.freqSeq[0]
		for i := 1; i < len(note.freqSeq); i++ {
			if note.freqSeq[i] != firstFreq {
				hasChange = true
				break
			}
		}
		if !hasChange {
			// All frequencies are the same - this is a simple constant note
			// No frequency modulation pattern needed - player uses base frequency only
			continue
		}

		notesWithFreqChange++

		// Compute deltas
		freqs := note.freqSeq
		deltas := make([]int16, len(freqs)-1)
		nonZeroDeltas := 0
		for i := 1; i < len(freqs); i++ {
			deltas[i-1] = int16(freqs[i]) - int16(freqs[i-1])
			if deltas[i-1] != 0 {
				nonZeroDeltas++
			}
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

		// Check if pattern is pure hold (ALL deltas are zero)
		// Encode as Type 8 Hold pattern for efficiency
		if len(deltas) > 0 && nonZeroDeltas == 0 {
			// HOLD pattern - Type 8
			// Just store base frequency and length
			pattern = FreqModPattern{
				Type:   8,
				Delay:  0,
				Length: byte(len(freqs)),
				Data:   []byte{byte(freqs[0]), byte(freqs[0] >> 8)},
			}
			key = fmt.Sprintf("hold_%d_%d", freqs[0], len(freqs))

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

		// Check for drum sequence templates FIRST (exact match)
		if templateIdx, matches := matchDrumSequence(freqs); matches {
			drumMatches++
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

		// Check for delayed vibrato (hold + oscillation) - TABLE-BASED
		vibratoAttempts++
		if holdFrames, oscPattern, matches := matchDelayedVibrato(freqs); matches {
			vibratoSuccess++

			// Store vibrato info for template building
			vibratoPatterns = append(vibratoPatterns, vibratoInfo{
				baseFreq:   freqs[0],
				holdFrames: holdFrames,
				pattern:    oscPattern,
				noteIdx:    noteIdx,
			})

			// DELAYED VIBRATO - Type 7
			// Temporarily store the full oscillation pattern in Data (18 bytes)
			// We'll replace this with a template index (1 byte) after building the template table
			data := make([]byte, 2+1+16)
			data[0] = byte(freqs[0])
			data[1] = byte(freqs[0] >> 8)
			data[2] = holdFrames
			for i := 0; i < 8; i++ {
				data[3+i*2] = byte(oscPattern[i])
				data[3+i*2+1] = byte(oscPattern[i] >> 8)
			}

			pattern = FreqModPattern{
				Type:   7,
				Delay:  0,
				Length: byte(len(freqs)),
				Data:   data,
			}

			patternSig := fmt.Sprintf("%d,%d,%d,%d,%d,%d,%d,%d",
				oscPattern[0], oscPattern[1], oscPattern[2], oscPattern[3],
				oscPattern[4], oscPattern[5], oscPattern[6], oscPattern[7])
			key = fmt.Sprintf("delvib_%d_%d_%s", freqs[0], holdFrames, patternSig)

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
				// Frequencies not all in table (glissando) - try classification
				if classified, fmType, fmData := classifyFreqModPattern(freqs[0], deltas); classified {
					pattern = FreqModPattern{
						Type:   fmType,
						Delay:  delay,
						Length: byte(len(freqs)),
						Data:   fmData,
					}
					typeName := map[byte]string{
						10: "jumpholdvib",
						11: "glissando",
						12: "trill",
					}[fmType]
					key = fmt.Sprintf("%s_%d_%d_%d", typeName, delay, freqs[0], len(deltas))
				} else {
					complexFallback++
					pattern = encodeComplex6502(freqs[0], deltas)
					key = fmt.Sprintf("complex_%d_%d", freqs[0], len(deltas))
				}
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
				// Frequencies not in table - try classification first
				if classified, fmType, fmData := classifyFreqModPattern(freqs[0], deltas); classified {
					pattern = FreqModPattern{
						Type:   fmType,
						Delay:  0,
						Length: byte(len(freqs)),
						Data:   fmData,
					}
					typeName := map[byte]string{
						10: "jumpholdvib",
						11: "glissando",
						12: "trill",
					}[fmType]
					key = fmt.Sprintf("%s_%d_%d", typeName, freqs[0], len(deltas))
				} else {
					complexFallback++
					pattern = encodeComplex6502(freqs[0], deltas)
					key = fmt.Sprintf("complex_%d_%d", freqs[0], len(deltas))
				}
			}
		} else if isMonotonic {
			// Check for LINEAR RAMP (constant delta per frame)
			if len(deltas) > 0 {
				allSameDelta := true
				firstDelta := deltas[0]
				for _, d := range deltas[1:] {
					if d != firstDelta {
						allSameDelta = false
						break
					}
				}

				if allSameDelta {
					// LINEAR RAMP - Type 9 (portamento/pitch bend with constant velocity)
					pattern = FreqModPattern{
						Type:   9,
						Delay:  0,
						Length: byte(len(freqs)),
						Data: []byte{
							byte(freqs[0]), byte(freqs[0] >> 8), // start freq
							byte(firstDelta), byte(firstDelta >> 8), // delta per frame
						},
					}
					key = fmt.Sprintf("linearramp_%d_%d_%d", freqs[0], firstDelta, len(freqs))
				} else {
					// Not constant delta - try note-based portamento or fall back to complex
					startIdx, startOk := freqIndex[freqs[0]]
					endIdx, endOk := freqIndex[freqs[len(freqs)-1]]

					if startOk && endOk && startIdx != endIdx {
						// PORTAMENTO - linear slide between notes
						pattern = encodePortamento6502(startIdx, endIdx, byte(len(freqs)))
						key = fmt.Sprintf("port_%d_%d_%d", startIdx, endIdx, len(freqs))
					} else {
						// COMPLEX - fallback to delta encoding
						complexFallback++
						pattern = encodeComplex6502(freqs[0], deltas)
						key = fmt.Sprintf("complex_%d_%d", freqs[0], len(deltas))
					}
				}
			} else {
				// No deltas - shouldn't happen for monotonic with change
				// Try classification first
				if classified, fmType, fmData := classifyFreqModPattern(freqs[0], deltas); classified {
					pattern = FreqModPattern{
						Type:   fmType,
						Delay:  0,
						Length: byte(len(freqs)),
						Data:   fmData,
					}
					typeName := map[byte]string{
						10: "jumpholdvib",
						11: "glissando",
						12: "trill",
					}[fmType]
					key = fmt.Sprintf("%s_%d_%d", typeName, freqs[0], len(deltas))
				} else {
					complexFallback++
					pattern = encodeComplex6502(freqs[0], deltas)
					key = fmt.Sprintf("complex_%d_%d", freqs[0], len(deltas))
				}
			}
		} else {
			// Try to classify before falling back to Complex
			if classified, fmType, fmData := classifyFreqModPattern(freqs[0], deltas); classified {
				// Successfully classified as parameterized effect
				pattern = FreqModPattern{
					Type:   fmType,
					Delay:  0,
					Length: byte(len(freqs)),
					Data:   fmData,
				}
				typeName := map[byte]string{
					10: "jumpholdvib",
					11: "glissando",
					12: "trill",
				}[fmType]
				key = fmt.Sprintf("%s_%d_%d", typeName, freqs[0], len(deltas))
			} else {
				// Fallback to Complex encoding
				complexFallback++
				pattern = encodeComplex6502(freqs[0], deltas)
				key = fmt.Sprintf("complex_%d_%d", freqs[0], len(deltas))
			}
			/*
			// Effect-based encoding disabled for now
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
					complexFallback++
					pattern = encodeComplex6502(freqs[0], deltas)
					key = fmt.Sprintf("complex_%d_%d", freqs[0], len(deltas))
				}
			} else {
				// COMPLEX fallback (vibrato already checked above)
				complexFallback++
					pattern = encodeComplex6502(freqs[0], deltas)
				key = fmt.Sprintf("complex_%d_%d", freqs[0], len(deltas))
			}
			*/
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
	fmt.Printf("    Type 7 (DelayedVibrato): %d patterns\n", stats[7])
	fmt.Printf("    Type 8 (Hold): %d patterns\n", stats[8])
	fmt.Printf("    Type 9 (LinearRamp): %d patterns\n", stats[9])

	// Build delayed vibrato template table from collected patterns
	templateMap := make(map[string]int) // pattern signature -> template index
	templates := []DelayedVibratoTemplate{}

	for _, v := range vibratoPatterns {
		sig := fmt.Sprintf("%d,%d,%d,%d,%d,%d,%d,%d",
			v.pattern[0], v.pattern[1], v.pattern[2], v.pattern[3],
			v.pattern[4], v.pattern[5], v.pattern[6], v.pattern[7])

		if _, exists := templateMap[sig]; !exists {
			templateMap[sig] = len(templates)
			templates = append(templates, DelayedVibratoTemplate{Deltas: v.pattern})
		}
	}

	fmt.Printf("  Delayed vibrato templates: %d unique patterns from %d notes\n", len(templates), len(vibratoPatterns))

	// Update Type 7 patterns to use template indices instead of full pattern data
	for i := range patterns {
		if patterns[i].Type == 7 && len(patterns[i].Data) >= 19 {
			// Extract the stored oscillation pattern
			var oscPattern [8]int16
			for j := 0; j < 8; j++ {
				oscPattern[j] = int16(patterns[i].Data[3+j*2]) | (int16(patterns[i].Data[3+j*2+1]) << 8)
			}

			// Find template index
			sig := fmt.Sprintf("%d,%d,%d,%d,%d,%d,%d,%d",
				oscPattern[0], oscPattern[1], oscPattern[2], oscPattern[3],
				oscPattern[4], oscPattern[5], oscPattern[6], oscPattern[7])

			templateIdx := byte(templateMap[sig])

			// Update Data: baseFreq (2) + holdFrames (1) + templateIdx (1) = 4 bytes
			patterns[i].Data = []byte{
				patterns[i].Data[0], // baseFreq lo
				patterns[i].Data[1], // baseFreq hi
				patterns[i].Data[2], // holdFrames
				templateIdx,         // template index
			}
		}
	}

	// Second pass: Convert short Type 4 patterns to Type 7 if they match vibrato templates
	// These are patterns that were too short to validate during initial extraction
	fmt.Printf("\n  Second pass: checking %d patterns for short vibrato...\n", len(patterns))
	fmt.Printf("  Available templates: %d\n", len(templates))
	if len(templates) > 0 {
		fmt.Printf("  Template 0: %v\n", templates[0].Deltas)
	}
	convertedShortVibrato := 0
	type4Count := 0
	for i := range patterns {
		if patterns[i].Type == 4 {
			type4Count++
		}
		if patterns[i].Type == 4 && len(patterns[i].Data) >= 4 {
			// Parse Type 4: startFreq + deltas
			startFreq := uint16(patterns[i].Data[0]) | (uint16(patterns[i].Data[1]) << 8)
			numDeltas := (len(patterns[i].Data) - 2) / 2

			// Parse deltas
			deltas := make([]int16, numDeltas)
			for j := 0; j < numDeltas; j++ {
				deltas[j] = int16(patterns[i].Data[2+j*2]) | (int16(patterns[i].Data[2+j*2+1]) << 8)
			}

			// Check if this is a short delayed vibrato: 12 zeros followed by vibrato prefix
			if numDeltas >= 15 { // At least 12 hold + 3 vibrato frames
				holdFrames := 0
				for holdFrames < len(deltas) && deltas[holdFrames] == 0 {
					holdFrames++
				}

				if holdFrames == 12 && len(deltas)-holdFrames >= 3 {
					// Extract available vibrato frames
					vibratoFrames := deltas[holdFrames:]

					if convertedShortVibrato == 0 {
						fmt.Printf("    Pattern %d: Found candidate with hold=%d, %d vibrato frames\n",
							i, holdFrames, len(vibratoFrames))
					}

					// Try to match against existing templates
					const tolerance = 3
					bestMatch := -1
					bestMatchErrors := 1000
					for templateIdx, template := range templates {
						matches := true
						maxError := int16(0)
						for j := 0; j < len(vibratoFrames) && j < 8; j++ {
							diff := vibratoFrames[j] - template.Deltas[j]
							if diff < 0 {
								diff = -diff
							}
							if diff > maxError {
								maxError = diff
							}
							if diff > tolerance {
								matches = false
							}
						}

						if int(maxError) < bestMatchErrors {
							bestMatchErrors = int(maxError)
							if matches {
								bestMatch = templateIdx
							}
						}

						if matches {
							// Convert Type 4 to Type 7!
							patterns[i].Type = 7
							patterns[i].Data = []byte{
								byte(startFreq), byte(startFreq >> 8), // baseFreq
								byte(holdFrames),                       // holdFrames
								byte(templateIdx),                      // template index
							}
							convertedShortVibrato++

							if convertedShortVibrato <= 10 {
								fmt.Printf("  [Short Vibrato] Converted pattern %d: hold=%d, %d vibrato frames matched template %d\n",
									i, holdFrames, len(vibratoFrames), templateIdx)
							}
							break
						}
					}

					// If no match found, report best candidate
					if bestMatch == -1 && convertedShortVibrato < 5 {
						fmt.Printf("      NO MATCH: best error was %d (tolerance=%d)\n", bestMatchErrors, tolerance)
						fmt.Printf("      Vibrato frames: %v\n", vibratoFrames[:min(8, len(vibratoFrames))])
					}
				}
			}
		}
	}

	if convertedShortVibrato > 0 {
		fmt.Printf("  Converted %d short Type 4 patterns to Type 7 (prefix match)\n", convertedShortVibrato)
	}

	vibratoPattern := VibratoPattern{
		DelayFrames: 13,
		OffsetTable: []int8{-2, -4, -2, 0, 2, 4, 2, 0},
	}

	// Count pattern types and analyze sizes
	var type10Count, type10Bytes int
	var type11Count, type11Bytes int
	var type12Count int
	var type4Bytes int
	// type4Count already declared earlier
	for _, p := range patterns {
		switch p.Type {
		case 10:
			type10Count++
			type10Bytes += len(p.Data)
		case 11:
			type11Count++
			type11Bytes += len(p.Data)
		case 12:
			type12Count++
		case 4:
			type4Count++
			type4Bytes += len(p.Data)
		}
	}

	fmt.Printf("\n[FreqMod Debug] Total notes: %d, freq changes: %d, drum: %d, vibrato attempts: %d, vibrato success: %d, complex fallback: %d\n",
		totalNotes, notesWithFreqChange, drumMatches, vibratoAttempts, vibratoSuccess, complexFallback)
	fmt.Printf("[FreqMod Classification] Type10 (HoldVib): %d (%d bytes), Type11 (Gliss): %d (%d bytes), Type12 (Trill): %d\n",
		type10Count, type10Bytes, type11Count, type11Bytes, type12Count)
	fmt.Printf("[FreqMod Classification] Type4 (Complex): %d patterns (%d bytes)\n",
		type4Count, type4Bytes)

	return patterns, freqModMap, vibratoPattern, templates
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

// classifyFreqModPattern attempts to classify Type 4 patterns into parameterized effects
func classifyFreqModPattern(startFreq uint16, deltas []int16) (classified bool, fmType byte, fmData []byte) {
	if len(deltas) == 0 {
		return false, 0, nil
	}

	// Test 1: Hold+Vibrato (most common pattern)
	// Pattern: [0, 0, 0, ..., 0, vibrato_deltas...]
	// The "jump" is implicit in startFreq (vs note's base freq)
	if len(deltas) >= 10 {
		// Count leading zeros
		zeroRun := 0
		for i := 0; i < len(deltas); i++ {
			if deltas[i] == 0 {
				zeroRun++
			} else {
				break
			}
		}

		if zeroRun >= 8 {
			// Hold detected
			vibratoStart := zeroRun
			vibratoLen := len(deltas) - vibratoStart

			if vibratoLen > 0 {
				// Check if tail oscillates (vibrato)
				signChanges := 0
				for i := vibratoStart + 1; i < len(deltas); i++ {
					if (deltas[i] > 0 && deltas[i-1] < 0) || (deltas[i] < 0 && deltas[i-1] > 0) {
						signChanges++
					}
				}
				oscillationRatio := 0.0
				if vibratoLen > 1 {
					oscillationRatio = float64(signChanges) / float64(vibratoLen-1)
				}

				if oscillationRatio > 0.2 { // Lower threshold for slow vibrato
					// HOLD_VIBRATO pattern (Type 10)
					// Optimized: startFreq(2) + zeroCount(1) + vibratoDeltas (2 bytes each)
					// This saves storage for the long run of zeros
					vibratoDeltas := deltas[vibratoStart:]
					data := make([]byte, 3+len(vibratoDeltas)*2)
					data[0] = byte(startFreq)
					data[1] = byte(startFreq >> 8)
					data[2] = byte(zeroRun) // Number of zeros (hold frames)
					for i, d := range vibratoDeltas {
						data[3+i*2] = byte(d)
						data[3+i*2+1] = byte(d >> 8)
					}
					return true, 10, data
				}
			} else if zeroRun == len(deltas) {
				// Pure hold (all zeros) - should have been caught earlier as Type 8
				// But classify anyway
				data := []byte{
					byte(startFreq), byte(startFreq >> 8),
					byte(zeroRun),
					0, // no vibrato
					0,
				}
				return true, 10, data
			}
		}
	}

	// Test 2: GLISSANDO (quantized semitone steps: 14 patterns)
	// Check if deltas are multiples of ~59 (semitone)
	if len(deltas) >= 3 {
		semitone := 59.0
		quantized := 0
		for _, d := range deltas {
			if d == 0 {
				quantized++
				continue
			}
			ratio := float64(d)
			if d < 0 {
				ratio = -ratio
			}
			ratio /= semitone
			nearest := int(ratio + 0.5)
			error := ratio - float64(nearest)
			if error < 0 {
				error = -error
			}
			if error < 0.15 { // within 15% of semitone
				quantized++
			}
		}

		quantizationScore := float64(quantized) / float64(len(deltas))
		if quantizationScore > 0.7 {
			// GLISSANDO pattern (Type 11)
			// Data: startFreq(2) + deltas (2 bytes each, signed 16-bit)
			// Store exact deltas for now - can optimize later
			data := make([]byte, 2+len(deltas)*2)
			data[0] = byte(startFreq)
			data[1] = byte(startFreq >> 8)
			for i, d := range deltas {
				data[2+i*2] = byte(d)
				data[2+i*2+1] = byte(d >> 8)
			}
			return true, 11, data
		}
	}

	// Test 3: TRILL (alternating +N/-N: 1 pattern)
	if len(deltas) >= 4 {
		matches := 0
		for i := 1; i < len(deltas); i++ {
			expected := -deltas[i-1]
			actual := deltas[i]
			diff := expected - actual
			if diff < 0 {
				diff = -diff
			}
			if float64(diff) < float64(expected)*0.3 {
				if expected < 0 && float64(diff) < -float64(expected)*0.3 {
					matches++
				} else if expected >= 0 {
					matches++
				}
			}
		}

		alternationScore := float64(matches) / float64(len(deltas)-1)
		if alternationScore > 0.7 {
			// TRILL pattern (Type 12)
			// Data: amplitude(2), repeat_count(1)
			amplitude := deltas[0]
			if amplitude < 0 {
				amplitude = -amplitude
			}
			data := []byte{
				byte(amplitude), byte(amplitude >> 8),
				byte(len(deltas) / 2), // number of oscillations
			}
			return true, 12, data
		}
	}

	// No match - use Type 4 (Complex)
	return false, 0, nil
}
