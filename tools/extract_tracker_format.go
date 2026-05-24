// Tracker format extraction from optimized Note structures
package main

import (
	"fmt"
	"os"
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
	InstrumentIndex     byte // Index into Instruments
	FreqSeqIndex        byte // Index into FreqModPatterns (0xFF = none)
	SustainPatternIndex byte // Index into SustainPatterns (0xFF = none)
}

// TrackerFormat contains the complete music data for the player
type TrackerFormat struct {
	StartOffset              int                        // Frame offset before first note (e.g., 162)
	TickRate                 int                        // Frames per tick (e.g., 5 for 10Hz updates)
	FreqTable                []uint16
	TogglePeriodValues       []uint16                   // Unique toggle period values (max 4, indexed by TogglePeriodIdx)
	TogglePeriodIdx          []byte                     // 2-bit index per freq entry, packed 4 per byte
	FreqSequenceTemplates    []FreqSequenceTemplate     // Frequency sequence templates for drum patterns
	DelayedVibratoTemplates  []DelayedVibratoTemplate   // Oscillation pattern templates for delayed vibrato
	FreqModPatterns          []FreqModPattern           // 6502-optimized frequency modulation patterns
	SustainPatterns          []SustainPattern           // Dynamic S value sequences for volume-following
	Instruments              []TrackerInstrument
	DurationTable            []byte                     // Unique note durations in frames; patterns store DurIdx
	NotePatterns             []TrackerPattern           // Bundled patterns (melody + effects)
	ToneSeqOverrides         []ToneSeqOverride          // Sparse: patterns with non-constant AY tone glissando
	RestDurations            []byte                     // Unique rest durations; stream/block rests encoded as 0xEE+durIdx (stream) or 0xFF-durIdx (block content)
	Blocks                   []TrackerBlock             // Reusable note sequences
	Streams                  [3]TrackerStream           // Per-channel bytecode
}

// ToneSeqOverride maps a note pattern index to a FreqSequenceTemplate index for envelope notes
// whose AY tone period glissandos independently of their pitch (CIA timer source).
type ToneSeqOverride struct {
	PatternIdx  byte
	TemplateIdx byte
}

type FreqSequenceTemplate struct {
	Frequencies []uint16 // Frequency sequence (e.g., drum sweep pattern)
}

// FreqModPattern - 6502-friendly frequency modulation encoding
type FreqModPattern struct {
	Type   byte   // 5=SequenceLookup, 7=DelayedVibrato, 8=Hold, 11=HoldGliss, 13=HeadCycle; serialized as compact 0–4
	Delay  byte   // Frames to hold before modulation starts (0-255)
	Length byte   // Total pattern length in frames (not serialized)
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
// DelayedVibrato (Type 7):
//   Data[0] = template index
//   holdFrames = 12 (constant, all notes in this song); baseFreq from note pattern
//   6502: hold 12 frames, then apply vibrato pattern from template
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
	Shape  byte     // 0=bell [v0,v1,v0,0,-v2,-v3,-v2,0], 1=sine [v0,v1,-v1,-v0,-v2,-v3,v3,v2], 2=raw
	V      [4]int16 // 4 parameters for bell/sine shapes
}


type SustainPattern struct {
	IsConstant     bool    // If true, StartValue is repeated for the entire note duration
	IsDelta        bool    // If true, use delta encoding (2-bit deltas)
	IsPrefix       bool    // If true, this is a prefix of BasePatternIdx (use first note-duration bytes)
	BasePatternIdx int     // For IsPrefix: index of the base pattern to slice
	StartValue     uint8   // Starting value (for delta encoding)
	Values         []uint8 // S parameter per frame (0-15), or delta-encoded (4 deltas per byte)
}

type TrackerInstrument struct {
	BaseWaveform   byte   // Triangle, sawtooth, pulse, noise (NOT 0x05)
	EnvelopeMode   bool   // If true, player toggles between 0x05 and BaseWaveform
	NoisePlusTone  bool   // If true, mixer state determines waveform (noise vs pulse)
	TogglePeriod   uint16 // SID cycles for waveform toggle (0 if no toggle needed)
	TailStartFrame byte   // 0=no tail, 0xFF=tail==duration, else constant frame offset
	ADSR           ADSRParams
	// VibratoDepth/Speed/Delay omitted — all zero in this song
}

type TrackerPattern struct {
	FreqIdx             byte
	Duration            byte
	InstrumentID        byte
	SustainPatternIndex byte
	FreqModIdx          byte // 0xFF=none, 0xFE=inline vibrato, 0-N=freqmod pattern index
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
	toneSeqKey      string   // Non-empty key for varying envelope tone period sequences
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
	freqTable, togglePeriodValues, togglePeriodIdx := buildFrequencyTableFromMetadata(allNotes)
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
	freqModPatterns, freqSeqMap, delayedVibratoTemplates := buildFreqModPatterns6502(allNotes, freqTable)

	// Build sustain patterns (for volume-following ADSR)
	sustainPatterns, sustainPatternMap := buildSustainPatterns(allNotes)

	// Analyze toggle periods for envelope/NoisePlusTone notes
	analyzeTogglePeriods(allNotes)

	// Compute toneSeqKey for each note: non-empty only for envelope notes with varying toggle periods.
	// Used in pattern key to ensure notes with different tone sequences get distinct patterns.
	for i := range allNotes {
		seq := allNotes[i].togglePeriodSeq
		if len(seq) == 0 || allNotes[i].isRest || !allNotes[i].Note.EnvelopeMode {
			continue
		}
		isConst := true
		for _, v := range seq[1:] {
			if v != seq[0] {
				isConst = false
				break
			}
		}
		if !isConst {
			allNotes[i].toneSeqKey = fmt.Sprintf("%v", seq)
		}
	}

	// Build note patterns
	notePatterns, notePatternMap := buildNotePatternsFromMetadata(allNotes, freqIndex, freqSeqMap, sustainPatternMap)

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
			sustainKey := sustainMapKey(allNotes[i].Note.SustainPerFrame)
			if idx, exists := sustainPatternMap[sustainKey]; exists {
				sustainPatternIndex = byte(idx)
			}
		}

		key := fmt.Sprintf("%d_%d_%d_%d_%d_%v_%s", freqIdx, allNotes[i].dur, allNotes[i].instrumentID, freqSeqIndex, sustainPatternIndex, allNotes[i].isRest, allNotes[i].toneSeqKey)
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

	// Build frequency sequence templates from drum sequence templates
	freqSequenceTemplates := make([]FreqSequenceTemplate, len(drumSequenceTemplates))
	for i, template := range drumSequenceTemplates {
		freqSequenceTemplates[i] = FreqSequenceTemplate{
			Frequencies: template,
		}
	}

	// Build tone-period FreqSequenceTemplates for envelope notes with varying toggle periods.
	// Stored as a sparse override table (ToneSeqOverrides) rather than a per-pattern byte,
	// since only a small number of patterns need this. Must happen BEFORE buildBlocksFromMetadata,
	// which reorders/filters the notePatterns slice.
	toneSeqMap := make(map[string]int)     // togglePeriodSeq key → FreqSequenceTemplate index
	assignedToneSeq := make(map[int]bool)  // patternIdx → already assigned
	var toneSeqOverrides []ToneSeqOverride
	for i, note := range allNotes {
		if note.isRest || !note.Note.EnvelopeMode || len(note.togglePeriodSeq) == 0 || note.toneSeqKey == "" {
			continue
		}
		key := note.toneSeqKey
		if _, exists := toneSeqMap[key]; !exists {
			toneSeqMap[key] = len(freqSequenceTemplates)
			freqs := make([]uint16, len(note.togglePeriodSeq))
			copy(freqs, note.togglePeriodSeq)
			freqSequenceTemplates = append(freqSequenceTemplates, FreqSequenceTemplate{Frequencies: freqs})
		}
		tmplIdx := toneSeqMap[key]
		freqSeqIdx := byte(0xFF)
		if len(allNotes[i].freqSeq) > 1 {
			startFreq := allNotes[i].freqSeq[0]
			k := fmt.Sprintf("%d", startFreq)
			for j := 1; j < len(allNotes[i].freqSeq); j++ {
				k += fmt.Sprintf(",%d", int16(allNotes[i].freqSeq[j])-int16(allNotes[i].freqSeq[j-1]))
			}
			if idx, exists := freqSeqMap[k]; exists {
				freqSeqIdx = byte(idx)
			}
		}
		sustainIdx := byte(0xFF)
		if len(allNotes[i].Note.SustainPerFrame) > 0 {
			if idx, exists := sustainPatternMap[sustainMapKey(allNotes[i].Note.SustainPerFrame)]; exists {
				sustainIdx = byte(idx)
			}
		}
		patKey := fmt.Sprintf("%d_%d_%d_%d_%d_%v_%s", freqIndex[allNotes[i].freq], allNotes[i].dur, allNotes[i].instrumentID, freqSeqIdx, sustainIdx, allNotes[i].isRest, allNotes[i].toneSeqKey)
		if pid, exists := notePatternMap[patKey]; exists && !assignedToneSeq[pid] {
			toneSeqOverrides = append(toneSeqOverrides, ToneSeqOverride{PatternIdx: byte(pid), TemplateIdx: byte(tmplIdx)})
			assignedToneSeq[pid] = true
		}
	}

	// Build dynamic-length blocks and bytecode streams.
	// patternRemap[origIdx] = finalIdx after reordering and rest-pattern removal.
	blocks, streams, restDurations, patternRemap := buildBlocksFromMetadata(allNotes, notesByChannel, &notePatterns, notePatternMap, tickRate)

	// Remap tone seq override pattern indices to post-reorder indices.
	remappedOverrides := toneSeqOverrides[:0]
	for _, o := range toneSeqOverrides {
		if int(o.PatternIdx) < len(patternRemap) && patternRemap[o.PatternIdx] != 0xFF {
			remappedOverrides = append(remappedOverrides, ToneSeqOverride{
				PatternIdx:  patternRemap[o.PatternIdx],
				TemplateIdx: o.TemplateIdx,
			})
		}
	}
	toneSeqOverrides = remappedOverrides

	// Build duration table: unique non-rest note durations, sorted ascending.
	durSet := make(map[byte]struct{})
	for _, p := range notePatterns {
		if p.FreqIdx != 255 {
			durSet[p.Duration] = struct{}{}
		}
	}
	durInts := make([]int, 0, len(durSet))
	for d := range durSet {
		durInts = append(durInts, int(d))
	}
	sort.Ints(durInts)
	durationTable := make([]byte, len(durInts))
	durToIdx := make(map[byte]byte, len(durInts))
	for i, d := range durInts {
		durationTable[i] = byte(d)
		durToIdx[byte(d)] = byte(i)
	}
	if len(durationTable) > 16 {
		panic(fmt.Sprintf("too many unique durations (%d) for 4-bit index", len(durationTable)))
	}

	// Build drum sweep pattern (incremental addition algorithm)
	// Pattern: 1.0, 1.0, 1.5, 1.0, 1.0, 1.0, 2.0, 1.0, 2.0
	// Encoded as nibbles: 0=1.0x, 1=1.5x, 2=2.0x
	format := &TrackerFormat{
		StartOffset:             startOffset,
		TickRate:                tickRate,
		FreqTable:               freqTable,
		TogglePeriodValues:      togglePeriodValues,
		TogglePeriodIdx:         togglePeriodIdx,
		FreqSequenceTemplates:   freqSequenceTemplates,
		DelayedVibratoTemplates: delayedVibratoTemplates,
		FreqModPatterns:         freqModPatterns,
		SustainPatterns:         sustainPatterns,
		Instruments:             instruments,
		DurationTable:           durationTable,
		NotePatterns:            notePatterns,
		ToneSeqOverrides:        toneSeqOverrides,
		RestDurations:           restDurations,
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
					InitialDecay: note.Note.ADSR.InitialDecay,
				},
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

	// Assign TailStartFrame per instrument from note data.
	// 0x00 = no tail, 0xFF = tail equals note duration, else constant frame value.
	instrTailValues := make(map[int]map[byte]int) // instID -> tailValue -> count
	for _, note := range notes {
		if note.isRest {
			continue
		}
		key := makeInstrumentKeyFromMetadata(note, nil)
		instID := instrumentMap[key]
		tail := byte(0)
		if note.Note.TailStartFrame > 0 && note.Note.TailStartFrame < 256 {
			tail = byte(note.Note.TailStartFrame)
		}
		if instrTailValues[instID] == nil {
			instrTailValues[instID] = make(map[byte]int)
		}
		// Normalise: if tail == duration, record as 0xFF
		dur := note.dur
		if tail > 0 && int(tail) == int(dur) {
			instrTailValues[instID][0xFF]++
		} else {
			instrTailValues[instID][tail]++
		}
	}
	for instID, vals := range instrTailValues {
		if len(vals) != 1 {
			panic(fmt.Sprintf("instrument %d has multiple tail values %v — tail is not instrument-constant", instID, vals))
		}
		for v := range vals {
			instruments[instID].TailStartFrame = v
		}
	}

	return instruments, instrumentMap
}

func makeInstrumentKeyFromMetadata(note NoteWithMetadata, instruments []TrackerInstrument) string {
	// Vibrato disabled - only use waveform, envelope mode, noise+tone, and ADSR
	return fmt.Sprintf("%d_%v_%v_%d_%d_%d_%d",
		note.Note.Waveform,
		note.Note.EnvelopeMode,
		note.Note.NoisePlusTone,
		note.Note.ADSR.Attack,
		note.Note.ADSR.Decay,
		note.Note.ADSR.Sustain,
		note.Note.ADSR.InitialDecay)
}

func sustainMapKey(sustain []uint8) string {
	isConstant := true
	for _, v := range sustain[1:] {
		if v != sustain[0] {
			isConstant = false
			break
		}
	}
	if isConstant {
		return fmt.Sprintf("const:%d", sustain[0])
	}
	key := ""
	for _, s := range sustain {
		if key != "" {
			key += ","
		}
		key += fmt.Sprintf("%d", s)
	}
	return key
}

func buildSustainPatterns(notes []NoteWithMetadata) ([]SustainPattern, map[string]int) {
	sustainMap := make(map[string]int)
	patterns := []SustainPattern{}

	for _, note := range notes {
		if note.isRest || len(note.Note.SustainPerFrame) == 0 {
			continue // Skip rests and notes without sustain data
		}

		key := sustainMapKey(note.Note.SustainPerFrame)

		// Add pattern if not seen before
		if _, exists := sustainMap[key]; !exists {
			sustainMap[key] = len(patterns)
			sustain := note.Note.SustainPerFrame
			isConstant := len(key) > 6 && key[:6] == "const:"

			if isConstant {
				// Constant pattern - just store the single value
				patterns = append(patterns, SustainPattern{
					IsConstant: true,
					StartValue: sustain[0],
					Values:     []uint8{sustain[0]},
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
						IsDelta:    true,
						StartValue: sustain[0],
						Values:     packed,
					})
				} else {
					// Store full sequence
					patterns = append(patterns, SustainPattern{
						StartValue: sustain[0],
						Values:     append([]uint8(nil), sustain...),
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
		// Check for raw (non-delta, non-constant) long patterns that share prefixes
		type rawPat struct {
			idx    int
			length int
			values []uint8
		}
		var rawLong []rawPat
		for i, p := range patterns {
			if !p.IsConstant && !p.IsDelta && len(p.Values) >= 10 {
				rawLong = append(rawLong, rawPat{idx: i, length: len(p.Values), values: p.Values})
			}
		}
		if len(rawLong) > 1 {
			// Find longest pattern and check if others are prefixes
			maxLen := 0
			maxIdx := 0
			for i, r := range rawLong {
				if r.length > maxLen {
					maxLen = r.length
					maxIdx = i
				}
			}
			base := rawLong[maxIdx].values
			prefixCount := 0
			savedBytes := 0
			for _, r := range rawLong {
				if r.idx == rawLong[maxIdx].idx {
					continue
				}
				isPrefixOf := true
				for j := 0; j < r.length && j < len(base); j++ {
					if r.values[j] != base[j] {
						isPrefixOf = false
						break
					}
				}
				if isPrefixOf {
					prefixCount++
					savedBytes += r.length - 2 // save sequence, pay 2 bytes for offset reference
					fmt.Printf("  Pattern %d (len=%d) is prefix of pattern %d (len=%d): saves %d bytes\n",
						r.idx, r.length, rawLong[maxIdx].idx, maxLen, r.length-2)
				}
			}
			if prefixCount > 0 {
				fmt.Printf("  Total: %d raw patterns are prefixes → potential saving: ~%d bytes\n", prefixCount, savedBytes)
			}
		}
	}

	// Mark prefix patterns: for each raw (non-delta, non-constant) pair where
	// shorter is a prefix of longer, mark the shorter as IsPrefix.
	type rawEntry struct {
		idx    int
		values []uint8
	}
	var rawEntries []rawEntry
	for i, p := range patterns {
		if !p.IsConstant && !p.IsDelta {
			rawEntries = append(rawEntries, rawEntry{idx: i, values: p.Values})
		}
	}
	// Sort by descending length so we always find the longest base first.
	sort.Slice(rawEntries, func(a, b int) bool {
		return len(rawEntries[a].values) > len(rawEntries[b].values)
	})
	for i, base := range rawEntries {
		for _, cand := range rawEntries[i+1:] {
			if patterns[cand.idx].IsPrefix {
				continue // already marked
			}
			if len(cand.values) >= len(base.values) {
				continue // same length or longer — not a prefix
			}
			isPfx := true
			for j, v := range cand.values {
				if base.values[j] != v {
					isPfx = false
					break
				}
			}
			if isPfx {
				patterns[cand.idx].IsPrefix = true
				patterns[cand.idx].BasePatternIdx = base.idx
				patterns[cand.idx].Values = nil // not stored inline
			}
		}
	}

	return patterns, sustainMap
}

func buildNotePatternsFromMetadata(notes []NoteWithMetadata, freqIndex map[uint16]byte, freqSeqMap map[string]int, sustainPatternMap map[string]int) ([]TrackerPattern, map[string]int) {
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
			sustainKey := sustainMapKey(note.Note.SustainPerFrame)
			if idx, exists := sustainPatternMap[sustainKey]; exists {
				sustainPatternIndex = byte(idx)
			}
		}

		key := fmt.Sprintf("%d_%d_%d_%d_%d_%v_%s", freqIdx, note.dur, note.instrumentID, freqSeqIndex, sustainPatternIndex, note.isRest, note.toneSeqKey)
		if _, exists := patternMap[key]; !exists {
			patternMap[key] = len(patterns)
			patterns = append(patterns, TrackerPattern{
				FreqIdx:             freqIdx,
				Duration:            note.dur,
				InstrumentID:        byte(note.instrumentID),
				SustainPatternIndex: sustainPatternIndex,
				FreqModIdx:          freqSeqIndex,
			})
		}
	}

	fmt.Printf("  Glissando pattern matching: %d/%d successful (%.1f%%)\n",
		glissandoMatches, glissandoAttempts, float64(glissandoMatches)*100.0/float64(max(1, glissandoAttempts)))

	return patterns, patternMap
}

func buildBlocksFromMetadata(allNotes []NoteWithMetadata, notesByChannel [3][]Note,
	notePatterns *[]TrackerPattern, notePatternMap map[string]int, tickRate int) ([]TrackerBlock, [3]TrackerStream, []byte, []byte) {

	// Rest pattern lookup tables — populated before stream encoding and block search.
	restPIDSet := make(map[byte]bool)
	restDurIndex := make(map[byte]byte)

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

	// Populate rest lookup tables now that all rest patterns have been inserted.
	for pid, p := range *notePatterns {
		if p.FreqIdx == 255 {
			restPIDSet[byte(pid)] = true
		}
	}
	{
		restDurSetPre := make(map[byte]struct{})
		for pid := range restPIDSet { restDurSetPre[(*notePatterns)[pid].Duration] = struct{}{} }
		restDursSorted := make([]byte, 0, len(restDurSetPre))
		for d := range restDurSetPre { restDursSorted = append(restDursSorted, d) }
		sort.Slice(restDursSorted, func(a, b int) bool { return restDursSorted[a] < restDursSorted[b] })
		for i, d := range restDursSorted { restDurIndex[d] = byte(i) }
	}

	// Build pattern ID streams per channel.
	// Rests are encoded as (256 - 1 - durIdx) = 0xFF - durIdx (values 0xEE-0xFF for 18 durations).
	// This is safely above all original pattern IDs (max = len(*notePatterns)-1 ≤ 161 < 0xEE=238).
	const restMarkerBase = byte(0xFF) // decreasing: restMarkerBase - durIdx
	channelStreams := [3][]byte{}
	for ch := 0; ch < 3; ch++ {
		stream := []byte{}
		for _, note := range notesByChannelWithMeta[ch] {
			pid := byte(note.patternID)
			if restPIDSet[pid] {
				dur := (*notePatterns)[pid].Duration
				stream = append(stream, restMarkerBase-restDurIndex[dur])
			} else {
				stream = append(stream, pid)
			}
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

		// Live-mutation encoding: store loop 1 in RAM, then apply a delta
		// patch list before each subsequent play. Cost = 61 (initial loop) +
		// sum of (changed positions between consecutive loops) × 2 bytes each
		// + 13 end-of-patch sentinels.
		fmt.Printf("=== Live-mutation (consecutive-delta) encoding ===\n")
		loopData := make([][]int, numLoops)
		for loopNum := 0; loopNum < numLoops; loopNum++ {
			loopData[loopNum] = make([]int, blockLen)
			for pos := 0; pos < blockLen; pos++ {
				idx := loopStart + loopNum*blockLen + pos
				if idx < len(notesByChannelWithMeta[ch]) {
					loopData[loopNum][pos] = notesByChannelWithMeta[ch][idx].patternID
				}
			}
		}
		totalDeltas := 0
		for loopNum := 1; loopNum < numLoops; loopNum++ {
			changed := 0
			for pos := 0; pos < blockLen; pos++ {
				if loopData[loopNum][pos] != loopData[loopNum-1][pos] {
					changed++
				}
			}
			fmt.Printf("  Loop %2d→%2d: %d positions changed\n", loopNum-1, loopNum, changed)
			totalDeltas += changed
		}
		fmt.Printf("  Total delta entries (×2 bytes each): %d = %d bytes\n", totalDeltas, totalDeltas*2)
		fmt.Printf("  Sentinels (end-of-patch per transition): %d bytes\n", numLoops-1)
		streamCost := blockLen + totalDeltas*2 + (numLoops - 1) + loopStart
		fmt.Printf("  Total stream: %d (initial) + %d (deltas) + %d (sentinels) + %d (intro) = %d bytes\n",
			blockLen, totalDeltas*2, numLoops-1, loopStart, streamCost)
		fmt.Printf("  Current stream: 488 bytes → live-mutation: %d bytes (saves %d)\n\n",
			streamCost, 488-streamCost)

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

	// Global greedy selection: iteratively pick the block with highest actual savings,
	// re-evaluating after each selection. Stop when no candidate has positive savings.
	// directRefBase = len(blocks) is computed after selection; the opcode space enforces
	// blocks + directPatterns + 18 rests <= 256.
	minSavings := 1
	maxBlocks := int(0xEE) - 1 // hard upper ceiling; real limit enforced by direct-ref space check

	// Sort initially by savings desc, length desc for tie-break, then seq for determinism.
	sort.Slice(seqList, func(i, j int) bool {
		if seqList[i].savings != seqList[j].savings {
			return seqList[i].savings > seqList[j].savings
		}
		if seqList[i].length != seqList[j].length {
			return seqList[i].length > seqList[j].length
		}
		return seqList[i].seq < seqList[j].seq
	})

	// consumedPos[ch] tracks which stream positions are already covered by a selected block.
	consumedPos := [3]map[int]bool{
		make(map[int]bool),
		make(map[int]bool),
		make(map[int]bool),
	}

	// findNonOverlap returns the non-overlapping positions from candidate after applying
	// already-consumed positions. Uses leftmost-first greedy.
	findNonOverlap := func(candidate seqInfo) []int {
		var result []int
		for _, pos := range candidate.positions {
			// check none of pos..pos+length-1 are consumed
			ok := true
			for k := 0; k < candidate.length; k++ {
				if consumedPos[candidate.channel][pos+k] {
					ok = false
					break
				}
			}
			if ok {
				result = append(result, pos)
			}
		}
		return result
	}

	computeSavings := func(seqLen, count int) int {
		blockCost := 1 + seqLen
		return count*seqLen - count*1 - blockCost
	}

	var selectedSeqList []seqInfo
	selected := make(map[string]bool) // deduplicate by sequence content

	for len(selectedSeqList) < maxBlocks {
		bestIdx := -1
		bestSavings := minSavings - 1
		var bestPositions []int

		for i, candidate := range seqList {
			if selected[candidate.seq] {
				continue
			}
			positions := findNonOverlap(candidate)
			if len(positions) < 2 {
				continue
			}
			actualSavings := computeSavings(candidate.length, len(positions))
			if actualSavings > bestSavings ||
				(actualSavings == bestSavings && (bestIdx < 0 ||
					candidate.length > seqList[bestIdx].length ||
					(candidate.length == seqList[bestIdx].length && candidate.seq < seqList[bestIdx].seq))) {
				bestIdx = i
				bestSavings = actualSavings
				bestPositions = positions
			}
		}

		if bestIdx < 0 {
			break
		}

		// Select this block.
		sel := seqList[bestIdx]
		sel.positions = bestPositions
		sel.savings = bestSavings
		sel.count = len(bestPositions)
		selectedSeqList = append(selectedSeqList, sel)
		selected[sel.seq] = true

		// Mark positions consumed (non-overlapping leftmost-first).
		for _, pos := range bestPositions {
			for k := 0; k < sel.length; k++ {
				consumedPos[sel.channel][pos+k] = true
			}
		}
	}

	seqList = selectedSeqList

	// Build block table (note-only content, no rest opcodes).
	blockDict := make(map[string]byte)
	blocks := []TrackerBlock{}
	for _, seqInf := range seqList {
		blockDict[seqInf.seq] = byte(len(blocks))
		patterns := make([]byte, len(seqInf.seq))
		copy(patterns, []byte(seqInf.seq))
		blocks = append(blocks, TrackerBlock{Patterns: patterns})
	}

	fmt.Printf("  Selected %d blocks (from %d candidates)\n", len(blocks), len(allSeqCandidates))

	fmt.Printf("  Building %d blocks (lengths: ", len(blocks))
	for i := 0; i < len(blocks) && i < 5; i++ {
		fmt.Printf("%d(ch%d) ", len(blocks[i].Patterns), seqList[i].channel)
	}
	fmt.Printf("...)\n")

	// Assign pattern IDs such that direct-ref patterns get low IDs (frequent ones first).
	// Use the same consumedPos maps from block selection to determine which positions are direct refs.
	var pidRemap2 []byte // declared here; assigned inside the block below
	var directRefBase byte // block refs: 0..directRefBase-1, direct refs: directRefBase..0xED
	{
		// Count direct-ref usage per original pattern ID.
		directUsageEarly := make([]int, len(*notePatterns))
		for ch := 0; ch < 3; ch++ {
			stream := channelStreams[ch]
			i := 0
			for i < len(stream) {
				// Check if this position is covered by a selected block.
				matchedLen := 0
				for seq := range blockDict {
					seqLen := len(seq)
					if i+seqLen <= len(stream) && string(stream[i:i+seqLen]) == seq {
						if seqLen > matchedLen {
							matchedLen = seqLen
						}
					}
				}
				if matchedLen > 0 {
					i += matchedLen
					continue
				}
				b := stream[i]
				if int(b) < len(directUsageEarly) { // rest markers (0xEE+) are outside this range
					directUsageEarly[b]++
				}
				i++
			}
		}

		// Build remap: direct-ref patterns (sorted by freq desc) get IDs 0..(n-1),
		// then block-only patterns, then rest patterns last.
		type pidEntryE struct{ oldID, count int }
		var directPIDsE, otherPIDsE []pidEntryE
		for i, n := range directUsageEarly {
			if n > 0 { directPIDsE = append(directPIDsE, pidEntryE{i, n}) } else { otherPIDsE = append(otherPIDsE, pidEntryE{i, 0}) }
		}
		sort.Slice(directPIDsE, func(a, b int) bool { return directPIDsE[a].count > directPIDsE[b].count })
		// Block refs: 0x00..(nBlocks-1). Direct refs: nBlocks..0xED. Rests: 0xEE..0xFF.
		directRefBase = byte(len(blocks))
		maxDirectRefs := int(0xEE) - int(directRefBase)
		if len(directPIDsE) > maxDirectRefs {
			panic(fmt.Sprintf("too many direct-ref patterns: %d > %d (need to reduce blocks)", len(directPIDsE), maxDirectRefs))
		}
		pidRemapEarly := make([]byte, len(*notePatterns))
		for i := range pidRemapEarly { pidRemapEarly[i] = 0xFF }
		newIDx := 0
		for _, e := range directPIDsE { pidRemapEarly[e.oldID] = byte(newIDx); newIDx++ }
		for _, e := range otherPIDsE { pidRemapEarly[e.oldID] = byte(newIDx); newIDx++ }
		fmt.Printf("  Distinct direct-ref pattern IDs: %d, block base=0x%02X, max direct encoding=0x%02X\n",
			len(directPIDsE), directRefBase, int(directRefBase)+len(directPIDsE)-1)

		// Apply remapEarly to channelStreams (pattern IDs) and block content.
		// Rest markers (0xEE-0xFF) are outside len(pidRemapEarly) range and unchanged.
		for ch := 0; ch < 3; ch++ {
			for i, b := range channelStreams[ch] {
				if int(b) < len(pidRemapEarly) {
					channelStreams[ch][i] = pidRemapEarly[b]
				}
			}
		}
		for bi := range blocks {
			for j, b := range blocks[bi].Patterns {
				if int(b) < len(pidRemapEarly) {
					blocks[bi].Patterns[j] = pidRemapEarly[b]
				}
			}
		}
		// Apply to *notePatterns (reorder).
		oldPatsE := *notePatterns
		reorderedE := make([]TrackerPattern, len(oldPatsE))
		for oldID, newID := range pidRemapEarly {
			if int(newID) < len(reorderedE) { reorderedE[newID] = oldPatsE[oldID] }
		}
		*notePatterns = reorderedE
		// Store for later use in composing the final remap.
		pidRemap2 = pidRemapEarly

		// Rebuild blockDict with remapped sequences so the bytecode encoder can match
		// against the now-remapped channelStreams.
		blockDict = make(map[string]byte)
		for id, block := range blocks {
			blockDict[string(block.Patterns)] = byte(id)
		}
	}

	// Generate bytecode streams using blockDict for O(1) lookup.
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
			matchedBlock := byte(0xFF)
			matchedLen := 0

			for seq, blockID := range blockDict {
				seqLen := len(seq)
				if i+seqLen <= len(stream) && string(stream[i:i+seqLen]) == seq {
					if seqLen > matchedLen {
						matchedBlock = blockID
						matchedLen = seqLen
					}
				}
			}

			if matchedBlock != 0xFF {
				// Emit block reference (0x00-0x0F)
				bytecode = append(bytecode, matchedBlock)
				i += matchedLen
				chBlockRefs++
			} else if stream[i] >= 0xEE {
				// Inline rest marker (0xFF-durIdx in channelStreams) → encode as 0xEE+durIdx
				durIdx := restMarkerBase - stream[i]
				bytecode = append(bytecode, 0xEE+durIdx)
				i++
				chDirectRefs++
			} else {
				// Direct pattern reference: encode as directRefBase+patternID
				bytecode = append(bytecode, directRefBase+stream[i])
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
	// (Pattern ID remapping was already applied to channelStreams and blocks before bytecode generation.)

	// Build RestDurations slice from restDurIndex (already populated).
	restDurations := make([]byte, len(restDurIndex))
	for dur, idx := range restDurIndex { restDurations[idx] = dur }

	// Remove rest patterns from NotePatterns and remap non-rest IDs in blocks and streams.
	oldPatterns := *notePatterns
	newPatterns := []TrackerPattern{}
	pidRemap := make([]byte, len(oldPatterns))
	for i := range pidRemap { pidRemap[i] = 0xFF }
	for i, p := range oldPatterns {
		if p.FreqIdx != 255 {
			pidRemap[i] = byte(len(newPatterns))
			newPatterns = append(newPatterns, p)
		}
	}
	*notePatterns = newPatterns

	// Remap block content: all bytes are note pattern IDs (no rest opcodes in blocks).
	for bi := range blocks {
		for j, b := range blocks[bi].Patterns {
			if int(b) < len(pidRemap) {
				blocks[bi].Patterns[j] = pidRemap[b]
			}
		}
	}
	// Remap stream direct-refs; block refs (< directRefBase) and rest opcodes (>= 0xEE) unchanged.
	for ch := 0; ch < 3; ch++ {
		for i, b := range streams[ch] {
			if b >= directRefBase && b < 0xEE {
				streams[ch][i] = directRefBase + pidRemap[b-directRefBase]
			}
		}
	}

	fmt.Printf("  Inlined %d rest patterns as stream opcodes 0x80-0x%02X (%d unique durations)\n",
		len(restPIDSet), 0xEE+len(restDurations)-1, len(restDurations))
	fmt.Printf("  Note patterns after removing rests: %d\n", len(*notePatterns))

	// Compose full remap: originalPatternID → finalPatternID (after freq-sort + rest removal).
	// pidRemap2 maps original → intermediate; pidRemap maps intermediate → final.
	origLen := len(pidRemap2)
	finalRemap := make([]byte, origLen)
	for i := range finalRemap { finalRemap[i] = 0xFF }
	for orig, mid := range pidRemap2 {
		if int(mid) < len(pidRemap) && pidRemap[mid] != 0xFF {
			finalRemap[orig] = pidRemap[mid]
		}
	}

	return blocks, streams, restDurations, finalRemap
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

	// Calculate ROM size — each section computed once and summed.

	freqTableSize := 1 + len(format.FreqTable)*2 +
		1 + len(format.TogglePeriodValues)*2 +
		(len(format.FreqTable)+1)/2 // freq table + toggle value table + packed 4-bit indices

	freqModSize := 1 // count
	for _, pattern := range format.FreqModPatterns {
		freqModSize += 1 + len(pattern.Data) // (compactType<<4|dataLen) + data
	}

	seqTemplateSize := 1 // count
	for _, template := range format.FreqSequenceTemplates {
		seqTemplateSize += 1 + len(template.Frequencies)*2
	}

	rawTemplates, bellTemplates, sineTemplates := 0, 0, 0
	delayedVibratoSize := 1 // count
	for _, tmpl := range format.DelayedVibratoTemplates {
		_ = tmpl
		bellTemplates++
		delayedVibratoSize += 8 // 4×int16 V params (no shape byte — bell only)
	}
	_ = rawTemplates; _ = sineTemplates

	sustainSize := 1 // count
	for _, pattern := range format.SustainPatterns {
		if pattern.IsPrefix {
			sustainSize += 2 // flags(1) + base_idx(1); length implicit from note duration
		} else {
			sustainSize += 2 // flags(1) + startValue(1)
			if !pattern.IsConstant {
				sustainSize += 1 + len(pattern.Values) // valuesLen(1) + values
			}
		}
	}

	instrumentSize := 1 + len(format.Instruments)*7 // count + 7 bytes: wave, flags(+attack), togglePeriod[2], tail, D, S
	restDurSize := 1 + len(format.RestDurations)
	durTableSize := 1 + len(format.DurationTable) // count + 1 byte per duration
	notePatternSize := 1 + len(format.NotePatterns)*4 // count + 4 bytes each
	toneSeqOverrideSize := 1 + len(format.ToneSeqOverrides)*2 // count + 2 bytes each

	blockSize := 1 // count
	for _, block := range format.Blocks {
		blockSize += 1 + len(block.Patterns)
	}

	streamSize := 0
	for ch := 0; ch < 3; ch++ {
		streamSize += 2 + len(format.Streams[ch])
	}

	size := freqTableSize + freqModSize + seqTemplateSize +
		delayedVibratoSize + sustainSize + instrumentSize +
		durTableSize + restDurSize + notePatternSize + toneSeqOverrideSize +
		blockSize + streamSize

	fmt.Printf("\nROM size: %d bytes (%.1f KB)\n", size, float64(size)/1024.0)
	fmt.Printf("  Breakdown:\n")
	fmt.Printf("    Freq table:       %4d bytes (%d entries × 2 + %d toggle values × 2 + %d nibble-index bytes)\n",
		freqTableSize, len(format.FreqTable), len(format.TogglePeriodValues), (len(format.FreqTable)+1)/2)
	fmt.Printf("    FreqMod patterns: %4d bytes (%d patterns, 6502-optimized)\n", freqModSize, len(format.FreqModPatterns))
	fmt.Printf("    Seq templates:    %4d bytes (%d templates)\n", seqTemplateSize, len(format.FreqSequenceTemplates))
	fmt.Printf("    Delayed vibrato:  %4d bytes (%d templates × 8, bell only)\n",
		delayedVibratoSize, len(format.DelayedVibratoTemplates))
	fmt.Printf("    Sustain patterns: %4d bytes (%d patterns)\n", sustainSize, len(format.SustainPatterns))
	fmt.Printf("    Instruments:      %4d bytes (%d × 7)\n", instrumentSize, len(format.Instruments))
	fmt.Printf("    Rest durations:   %4d bytes (%d unique durations, inline in stream)\n", restDurSize, len(format.RestDurations))
	fmt.Printf("    Duration table:   %4d bytes (%d unique durations)\n", durTableSize, len(format.DurationTable))
	fmt.Printf("    Note patterns:    %4d bytes (%d × 4)\n", notePatternSize, len(format.NotePatterns))
	fmt.Printf("    Tone overrides:   %4d bytes (%d entries)\n", toneSeqOverrideSize, len(format.ToneSeqOverrides))
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
	fmt.Printf("\n  Instruments:\n")
	for i, inst := range format.Instruments {
		toggleStr := ""
		if inst.TogglePeriod > 0 {
			toggleStr = fmt.Sprintf(" toggle=%d", inst.TogglePeriod)
		}
		fmt.Printf("    %d: wave=0x%02X env=%v npt=%v tail=%d%s ADSR=%d/%d/%d\n",
			i, inst.BaseWaveform, inst.EnvelopeMode, inst.NoisePlusTone,
			inst.TailStartFrame, toggleStr,
			inst.ADSR.Attack, inst.ADSR.Decay, inst.ADSR.Sustain)
	}

	// Per-instrument FreqMod and Sustain mappings

	// Value range analysis — looking for fields that fit in fewer bits
	fmt.Printf("\n=== Value Range Analysis ===\n")
	type rangeInfo struct {
		label  string
		vals   []int
	}
	printRangeInfo := func(ri rangeInfo) {
		if len(ri.vals) == 0 { return }
		lo, hi := ri.vals[0], ri.vals[0]
		uniq := make(map[int]struct{})
		for _, v := range ri.vals {
			if v < lo { lo = v }
			if v > hi { hi = v }
			uniq[v] = struct{}{}
		}
		span := hi - lo
		bits := 0
		for (1 << bits) <= span { bits++ }
		if bits == 0 { bits = 1 }
		uniqueList := make([]int, 0, len(uniq))
		for k := range uniq { uniqueList = append(uniqueList, k) }
		sort.Ints(uniqueList)
		display := fmt.Sprintf("%v", uniqueList)
		if len(uniqueList) > 10 { display = fmt.Sprintf("%d values", len(uniqueList)) }
		fmt.Printf("  %-38s n=%-4d  %3d..%-3d  %2d uniq  %d bits  %s\n",
			ri.label, len(ri.vals), lo, hi, len(uniq), bits, display)
	}

	var npFreq, npDur, npInstr, npSustain, npTail []int
	for _, p := range format.NotePatterns {
		if p.FreqIdx != 255 { npFreq = append(npFreq, int(p.FreqIdx)) }
		npDur    = append(npDur,    int(p.Duration))
		npInstr  = append(npInstr,  int(p.InstrumentID))
		npSustain = append(npSustain, int(p.SustainPatternIndex))
		if int(p.InstrumentID) < len(format.Instruments) {
			npTail = append(npTail, int(format.Instruments[p.InstrumentID].TailStartFrame))
		}
	}
	printRangeInfo(rangeInfo{"NotePattern.FreqIdx (excl 255)", npFreq})
	printRangeInfo(rangeInfo{"NotePattern.Duration", npDur})
	printRangeInfo(rangeInfo{"NotePattern.InstrumentID", npInstr})
	printRangeInfo(rangeInfo{"NotePattern.SustainIdx (excl 255)", npSustain})
	printRangeInfo(rangeInfo{"NotePattern.TailStartFrame", npTail})

	// Check if SustainPatternIndex is determined by (InstrumentID, Duration) pair
	fmt.Printf("\n=== Note Pattern Field Correlation ===\n")
	type instrDurKey struct{ instr, dur byte }
	instrDurToSustain := make(map[instrDurKey]map[byte]int)
	instrToSustain    := make(map[byte]map[byte]int)
	durToSustain      := make(map[byte]map[byte]int)
	for _, p := range format.NotePatterns {
		k := instrDurKey{p.InstrumentID, p.Duration}
		if instrDurToSustain[k] == nil { instrDurToSustain[k] = make(map[byte]int) }
		instrDurToSustain[k][p.SustainPatternIndex]++
		if instrToSustain[p.InstrumentID] == nil { instrToSustain[p.InstrumentID] = make(map[byte]int) }
		instrToSustain[p.InstrumentID][p.SustainPatternIndex]++
		if durToSustain[p.Duration] == nil { durToSustain[p.Duration] = make(map[byte]int) }
		durToSustain[p.Duration][p.SustainPatternIndex]++
	}
	ambigInstrDur, ambigInstr, ambigDur := 0, 0, 0
	for _, m := range instrDurToSustain { if len(m) > 1 { ambigInstrDur++ } }
	for _, m := range instrToSustain    { if len(m) > 1 { ambigInstr++ } }
	for _, m := range durToSustain      { if len(m) > 1 { ambigDur++ } }
	fmt.Printf("  SustainIdx determined by (instr+dur): %d/%d pairs ambiguous\n", ambigInstrDur, len(instrDurToSustain))
	fmt.Printf("  SustainIdx determined by instr alone: %d/%d instrs ambiguous\n", ambigInstr, len(instrToSustain))
	fmt.Printf("  SustainIdx determined by dur alone:   %d/%d durs ambiguous\n", ambigDur, len(durToSustain))

	// Check Duration correlation with InstrumentID
	instrToDur := make(map[byte]map[byte]int)
	for _, p := range format.NotePatterns {
		if instrToDur[p.InstrumentID] == nil { instrToDur[p.InstrumentID] = make(map[byte]int) }
		instrToDur[p.InstrumentID][p.Duration]++
	}
	ambigInstrDur2 := 0
	for _, m := range instrToDur { if len(m) > 1 { ambigInstrDur2++ } }
	fmt.Printf("  Duration determined by instr alone:   %d/%d instrs ambiguous\n", ambigInstrDur2, len(instrToDur))

	var fmType []int
	for _, p := range format.FreqModPatterns {
		fmType = append(fmType, int(p.Type))
	}
	printRangeInfo(rangeInfo{"FreqMod.Type", fmType})

	var iA, iD, iS, iWave, iTog []int
	for _, ins := range format.Instruments {
		iA    = append(iA,    int(ins.ADSR.Attack))
		iD    = append(iD,    int(ins.ADSR.Decay))
		iS    = append(iS,    int(ins.ADSR.Sustain))
		iWave = append(iWave, int(ins.BaseWaveform))
		iTog  = append(iTog,  int(ins.TogglePeriod))
	}
	printRangeInfo(rangeInfo{"Instrument.Attack", iA})
	printRangeInfo(rangeInfo{"Instrument.Decay", iD})
	printRangeInfo(rangeInfo{"Instrument.Sustain", iS})
	printRangeInfo(rangeInfo{"Instrument.BaseWaveform", iWave})
	printRangeInfo(rangeInfo{"Instrument.TogglePeriod (uint16)", iTog})

	var spStart, spVal []int
	for _, p := range format.SustainPatterns {
		spStart = append(spStart, int(p.StartValue))
		if !p.IsPrefix {
			for _, v := range p.Values { spVal = append(spVal, int(v)) }
		}
	}
	printRangeInfo(rangeInfo{"SustainPattern.StartValue", spStart})
	printRangeInfo(rangeInfo{"SustainPattern.Values[]", spVal})

	var vtV []int
	for _, t := range format.DelayedVibratoTemplates {
		for _, v := range t.V { vtV = append(vtV, int(v)) }
	}
	printRangeInfo(rangeInfo{"DelayedVibratoTemplate.V[] (int16)", vtV})

	var ftVal []int
	for _, f := range format.FreqTable { ftVal = append(ftVal, int(f)) }
	printRangeInfo(rangeInfo{"FreqTable[] (uint16)", ftVal})

	var npFreqMod []int
	for _, p := range format.NotePatterns { npFreqMod = append(npFreqMod, int(p.FreqModIdx)) }
	printRangeInfo(rangeInfo{"NotePattern.FreqModIdx", npFreqMod})

	var blkPID []int
	for _, b := range format.Blocks {
		for _, pid := range b.Patterns { blkPID = append(blkPID, int(pid)) }
	}
	printRangeInfo(rangeInfo{"Block.PatternIDs[]", blkPID})

	for ch := 0; ch < 3; ch++ {
		var sv []int
		for _, b := range format.Streams[ch] { sv = append(sv, int(b)) }
		printRangeInfo(rangeInfo{fmt.Sprintf("Stream[%d] bytes", ch), sv})
	}

	// Serialize and validate
	rom := serializeTrackerFormat(format)
	if len(rom) != size {
		panic(fmt.Sprintf("ROM size mismatch: calculated %d bytes but serialized %d bytes", size, len(rom)))
	}
	if err := os.WriteFile("tracker_format.rom", rom, 0644); err != nil {
		panic(fmt.Sprintf("failed to write ROM: %v", err))
	}
	fmt.Printf("  Written to tracker_format.rom (%d bytes)\n", len(rom))
}

func serializeTrackerFormat(format *TrackerFormat) []byte {
	w := []byte{}
	pu16 := func(v uint16) { w = append(w, byte(v), byte(v>>8)) }
	pb := func(v byte) { w = append(w, v) }
	pbs := func(v []byte) { w = append(w, v...) }

	// Freq table: count + freq entries
	pb(byte(len(format.FreqTable)))
	for _, f := range format.FreqTable {
		pu16(f)
	}
	// Toggle period values: count + uint16 entries
	pb(byte(len(format.TogglePeriodValues)))
	for _, v := range format.TogglePeriodValues {
		pu16(v)
	}
	// Toggle period nibble-packed indices
	pbs(format.TogglePeriodIdx)

	// FreqMod patterns: count + (compactType<<4|dataLen)(1) + data each
	// compactType: 5→0, 7→1, 8→2, 11→3, 13→4
	compactType := func(t byte) byte {
		switch t {
		case 5:  return 0
		case 7:  return 1
		case 8:  return 2
		case 11: return 3
		case 13: return 4
		default: panic(fmt.Sprintf("unexpected FreqMod type %d", t))
		}
	}
	pb(byte(len(format.FreqModPatterns)))
	for _, p := range format.FreqModPatterns {
		dataLen := byte(len(p.Data))
		pb(compactType(p.Type)<<4 | dataLen)
		pbs(p.Data)
	}

	// FreqSequence templates: count + (length(1) + freq×2 each)
	pb(byte(len(format.FreqSequenceTemplates)))
	for _, t := range format.FreqSequenceTemplates {
		pb(byte(len(t.Frequencies)))
		for _, f := range t.Frequencies {
			pu16(f)
		}
	}

	// Delayed vibrato templates: count + (4 × int16 V params, bell only)
	// All templates in this song are bell shape: Deltas=[v0,v1,v0,0,-v2,-v3,-v2,0]
	pb(byte(len(format.DelayedVibratoTemplates)))
	for _, t := range format.DelayedVibratoTemplates {
		if t.Shape != 0 {
			panic(fmt.Sprintf("unexpected DV template shape %d (only bell=0 supported)", t.Shape))
		}
		for i := 0; i < 4; i++ {
			pu16(uint16(t.V[i]))
		}
	}

	// Sustain patterns: count + (flags(1) + startValue(1) [+ values])
	pb(byte(len(format.SustainPatterns)))
	for _, sp := range format.SustainPatterns {
		flags := byte(0)
		if sp.IsConstant { flags |= 1 }
		if sp.IsDelta    { flags |= 2 }
		if sp.IsPrefix   { flags |= 4 }
		pb(flags)
		if sp.IsPrefix {
			pb(byte(sp.BasePatternIdx))
		} else {
			pb(sp.StartValue)
			if !sp.IsConstant {
				pb(byte(len(sp.Values)))
				pbs(sp.Values)
			}
		}
	}

	// Instruments: count + 7 bytes each: wave, flags, togglePeriod(2), tail, D, S
	// flags: bit0=EnvelopeMode, bit1=NoisePlusTone, bit2=Attack (0 or 1)
	pb(byte(len(format.Instruments)))
	for _, ins := range format.Instruments {
		pb(ins.BaseWaveform)
		flags := byte(0)
		if ins.EnvelopeMode  { flags |= 1 }
		if ins.NoisePlusTone { flags |= 2 }
		if ins.ADSR.Attack != 0 { flags |= 4 }
		pb(flags)
		pu16(ins.TogglePeriod)
		pb(ins.TailStartFrame)
		pb(ins.ADSR.Decay)
		pb(ins.ADSR.Sustain)
	}

	// Duration table: count + 1 byte each
	pb(byte(len(format.DurationTable)))
	pbs(format.DurationTable)

	// Rest durations: count + 1 byte each
	pb(byte(len(format.RestDurations)))
	pbs(format.RestDurations)

	// Note patterns: count + 4 bytes each: freq, (instr<<4|durIdx), sustainIdx, freqModIdx
	durToIdx := make(map[byte]byte, len(format.DurationTable))
	for i, d := range format.DurationTable {
		durToIdx[d] = byte(i)
	}
	pb(byte(len(format.NotePatterns)))
	for _, p := range format.NotePatterns {
		pb(p.FreqIdx)
		pb((p.InstrumentID << 4) | durToIdx[p.Duration])
		pb(p.SustainPatternIndex)
		pb(p.FreqModIdx)
	}

	// Tone seq overrides: count + 2 bytes each: patternIdx, templateIdx
	pb(byte(len(format.ToneSeqOverrides)))
	for _, o := range format.ToneSeqOverrides {
		pb(o.PatternIdx)
		pb(o.TemplateIdx)
	}

	// Blocks: count + (length(1) + patternIDs each)
	pb(byte(len(format.Blocks)))
	for _, block := range format.Blocks {
		pb(byte(len(block.Patterns)))
		pbs(block.Patterns)
	}

	// Streams: length(2 LE) + bytes each
	for ch := 0; ch < 3; ch++ {
		n := len(format.Streams[ch])
		pb(byte(n)); pb(byte(n >> 8))
		pbs(format.Streams[ch])
	}

	return w
}

func buildFrequencyTableFromMetadata(notes []NoteWithMetadata) ([]uint16, []uint16, []byte) {
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

	// Build unique toggle period value table and packed 2-bit index array
	uniqueToggle := []uint16{}
	uniqueToggleIdx := make(map[uint16]byte)
	for _, f := range freqs {
		tp := uint16(0)
		if v, exists := freqToToggle[f]; exists {
			tp = v
		}
		if _, seen := uniqueToggleIdx[tp]; !seen {
			if len(uniqueToggle) >= 16 {
				fmt.Printf("Toggle period overflow: unique values so far: %v, new value: %d\n", uniqueToggle, tp)
				panic("more than 16 unique toggle period values — 4-bit index insufficient")
			}
			uniqueToggleIdx[tp] = byte(len(uniqueToggle))
			uniqueToggle = append(uniqueToggle, tp)
		}
	}

	// Pack 2 indices per byte (4 bits each, lo nibble first)
	packed := make([]byte, (len(freqs)+1)/2)
	for i, f := range freqs {
		tp := uint16(0)
		if v, exists := freqToToggle[f]; exists {
			tp = v
		}
		idx := uniqueToggleIdx[tp]
		packed[i/2] |= idx << (uint(i%2) * 4)
	}

	return freqs, uniqueToggle, packed
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
	// Template 4: Drum sweep 3 (2 occurrences)
	{2552, 1897, 1509, 1155, 999, 880, 786, 648, 596, 513},
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
func buildFreqModPatterns6502(notes []NoteWithMetadata, freqTable []uint16) ([]FreqModPattern, map[string]int, []DelayedVibratoTemplate) {
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

		// Classify pattern (only types 11 and 13 occur in this song)
		if classified, fmType, fmData := classifyFreqModPattern(freqs[0], deltas); classified {
			pattern = FreqModPattern{
				Type:   fmType,
				Delay:  delay,
				Length: byte(len(freqs)),
				Data:   fmData,
			}
			key = fmt.Sprintf("%d", freqs[0])
			for _, d := range deltas {
				key += fmt.Sprintf(",%d", d)
			}
		} else {
			panic(fmt.Sprintf("unclassified freq pattern: startFreq=%d deltas=%v", freqs[0], deltas))
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
		totalBytes += 1 + len(p.Data) // (compactType<<4|dataLen) + data
	}

	fmt.Printf("  FreqMod patterns: %d total, %d bytes\n", len(patterns), totalBytes)
	fmt.Printf("    Type  5 (SequenceLookup): %d patterns\n", stats[5])
	fmt.Printf("    Type  7 (DelayedVibrato): %d patterns\n", stats[7])
	fmt.Printf("    Type 11 (HoldGliss):      %d patterns\n", stats[11])
	fmt.Printf("    Type 13 (HeadCycle):      %d patterns\n", stats[13])
	// Build delayed vibrato template table from collected patterns
	templateMap := make(map[string]int) // pattern signature -> template index
	templates := []DelayedVibratoTemplate{}

	for _, v := range vibratoPatterns {
		sig := fmt.Sprintf("%d,%d,%d,%d,%d,%d,%d,%d",
			v.pattern[0], v.pattern[1], v.pattern[2], v.pattern[3],
			v.pattern[4], v.pattern[5], v.pattern[6], v.pattern[7])

		if _, exists := templateMap[sig]; !exists {
			templateMap[sig] = len(templates)
			tmpl := DelayedVibratoTemplate{Deltas: v.pattern}
			classifyVibratoTemplate(&tmpl)
			templates = append(templates, tmpl)
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

			patterns[i].Data = []byte{templateIdx}
		}
	}

	// Second dedup pass: after type-7 patterns are compacted to 1-byte template index,
	// many that previously differed only by base freq now have identical serialized bytes.
	dedupMap := make(map[string]int) // serialized key → first index
	remap := make([]int, len(patterns))
	dedupedPatterns := patterns[:0:len(patterns)]
	dedupedPatterns = dedupedPatterns[:0]
	for i, p := range patterns {
		key := fmt.Sprintf("%d_%x", p.Type, p.Data)
		if j, exists := dedupMap[key]; exists {
			remap[i] = j
		} else {
			remap[i] = len(dedupedPatterns)
			dedupMap[key] = len(dedupedPatterns)
			dedupedPatterns = append(dedupedPatterns, p)
		}
	}
	if len(dedupedPatterns) < len(patterns) {
		fmt.Printf("  FreqMod dedup: %d → %d patterns (%d removed)\n",
			len(patterns), len(dedupedPatterns), len(patterns)-len(dedupedPatterns))
		// Remap freqModMap values
		for k, v := range freqModMap {
			freqModMap[k] = remap[v]
		}
		patterns = dedupedPatterns
	}

	// Convert Type 10 (HoldVib with raw deltas) to Type 7 (template-based)
	// Type 10 stores: startFreq(2) + holdFrames(1) + vibratoDeltas(2 each)
	// Type 7 stores:  baseFreq(2) + holdFrames(1) + templateIdx(1)
	const type10TemplateTolerance = int16(5)
	type10Converted := 0
	for i := range patterns {
		if patterns[i].Type == 10 && len(patterns[i].Data) >= 5 {
			numVibratoDeltas := (len(patterns[i].Data) - 3) / 2
			if numVibratoDeltas < 1 {
				continue
			}
			// Type10 vibratoDeltas are cumulative (frame-to-frame). Convert to
			// absolute offsets from baseFreq to match Type7 template format.
			cumulativeDeltas := make([]int16, numVibratoDeltas)
			for j := 0; j < numVibratoDeltas; j++ {
				cumulativeDeltas[j] = int16(patterns[i].Data[3+j*2]) | (int16(patterns[i].Data[3+j*2+1]) << 8)
			}
			absOffsets := make([]int16, numVibratoDeltas)
			running := int16(0)
			for j := 0; j < numVibratoDeltas; j++ {
				running += cumulativeDeltas[j]
				absOffsets[j] = running
			}
			// Match first 8 absolute offsets against all templates
			bestTemplate := -1
			for tidx, tmpl := range templates {
				match := true
				for j := 0; j < 8 && j < numVibratoDeltas; j++ {
					diff := absOffsets[j] - tmpl.Deltas[j]
					if diff < 0 {
						diff = -diff
					}
					if diff > type10TemplateTolerance {
						match = false
						break
					}
				}
				if match {
					bestTemplate = tidx
					break
				}
			}
			if bestTemplate < 0 {
				// No matching template — add a new one from first 8 absolute offsets
				var newPattern [8]int16
				for j := 0; j < 8; j++ {
					if j < numVibratoDeltas {
						newPattern[j] = absOffsets[j]
					}
				}
				sig := fmt.Sprintf("%d,%d,%d,%d,%d,%d,%d,%d",
					newPattern[0], newPattern[1], newPattern[2], newPattern[3],
					newPattern[4], newPattern[5], newPattern[6], newPattern[7])
				if tidx, exists := templateMap[sig]; exists {
					bestTemplate = tidx
				} else {
					bestTemplate = len(templates)
					templateMap[sig] = bestTemplate
					newTmpl := DelayedVibratoTemplate{Deltas: newPattern}
					classifyVibratoTemplate(&newTmpl)
					templates = append(templates, newTmpl)
				}
			}
			patterns[i].Type = 7
			patterns[i].Data = []byte{byte(bestTemplate)}
			type10Converted++
		}
	}
	if type10Converted > 0 {
		fmt.Printf("  Converted %d Type10 patterns to Type7 (template-based)\n", type10Converted)
	}

	fmt.Printf("\n[FreqMod] Total notes: %d, freq changes: %d, drum: %d, vibrato success: %d\n",
		totalNotes, notesWithFreqChange, drumMatches, vibratoSuccess)


	return patterns, freqModMap, templates
}

// classifyVibratoTemplate determines if a template fits a compact shape.
//
// Bell shape (Shape=0): [v0, v1, v0, 0, -v2, -v3, -v2, 0]
// Sine shape (Shape=1): [v0, v1, -v1, -v0, -v2, -v3, v3, v2]
// Raw  shape (Shape=2): full 8×int16 stored as-is
func classifyVibratoTemplate(t *DelayedVibratoTemplate) {
	d := t.Deltas
	// Try bell: d[2]==d[0], d[3]==0, d[6]==d[4], d[7]==0, d[4]==-d[0] approx
	if d[2] == d[0] && d[3] == 0 && d[6] == d[4] && d[7] == 0 {
		t.Shape = 0
		t.V = [4]int16{d[0], d[1], -d[4], -d[5]}
		return
	}
	// Try sine: d[2]==-d[1], d[3]==-d[0], d[6]==-d[5], d[7]==d[2+4-offset]
	// Pattern: [v0, v1, -v1, -v0, -v2, -v3, v3, v2]
	if d[2] == -d[1] && d[3] == -d[0] && d[6] == -d[5] {
		// d[7] should be d[4] negated ... wait: sine[7] = v2 = -d[4]
		// So d[7] == -d[4]
		if d[7] == -d[4] {
			t.Shape = 1
			t.V = [4]int16{d[0], d[1], -d[4], -d[5]}
			return
		}
		// Allow d[7]==0 (truncated last byte seen in some templates)
		if d[7] == 0 {
			t.Shape = 1
			t.V = [4]int16{d[0], d[1], -d[4], -d[5]}
			return
		}
	}
	t.Shape = 2 // Raw
}

// classifyFreqModPattern classifies a freq delta sequence (only types 11 and 13 occur in this song)
func classifyFreqModPattern(startFreq uint16, deltas []int16) (classified bool, fmType byte, fmData []byte) {
	if len(deltas) == 0 {
		return false, 0, nil
	}

	// Type 10 (intermediate): hold then vibrato — converted to type 7 by the caller
	if len(deltas) >= 10 {
		zeroRun := 0
		for i := 0; i < len(deltas); i++ {
			if deltas[i] == 0 {
				zeroRun++
			} else {
				break
			}
		}
		if zeroRun >= 8 {
			vibratoStart := zeroRun
			vibratoLen := len(deltas) - vibratoStart
			if vibratoLen > 0 {
				signChanges := 0
				for i := vibratoStart + 1; i < len(deltas); i++ {
					if (deltas[i] > 0 && deltas[i-1] < 0) || (deltas[i] < 0 && deltas[i-1] > 0) {
						signChanges++
					}
				}
				if vibratoLen > 1 && float64(signChanges)/float64(vibratoLen-1) > 0.2 {
					vibratoDeltas := deltas[vibratoStart:]
					data := make([]byte, 3+len(vibratoDeltas)*2)
					data[0] = byte(startFreq)
					data[1] = byte(startFreq >> 8)
					data[2] = byte(zeroRun)
					for i, d := range vibratoDeltas {
						data[3+i*2] = byte(d)
						data[3+i*2+1] = byte(d >> 8)
					}
					return true, 10, data
				}
			}
		}
	}

	// Type 11: GLISSANDO — semitone-quantized pitch steps (12-frame hold + N steps)
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
			err := ratio - float64(nearest)
			if err < 0 {
				err = -err
			}
			if err < 0.15 {
				quantized++
			}
		}
		if float64(quantized)/float64(len(deltas)) > 0.7 {
			holdCount := 0
			for _, d := range deltas {
				if d == 0 {
					holdCount++
				} else {
					break
				}
			}
			steps := []int16{}
			for _, d := range deltas[holdCount:] {
				if d != 0 {
					steps = append(steps, d)
				}
			}
			data := make([]byte, len(steps)*2)
			for i, d := range steps {
				data[i*2] = byte(d)
				data[i*2+1] = byte(d >> 8)
			}
			return true, 11, data
		}
	}

	// Type 13: HEAD+CYCLE — non-repeating head followed by repeating cycle
	if len(deltas) >= 4 {
		freqs := make([]uint16, len(deltas)+1)
		freqs[0] = startFreq
		for i, d := range deltas {
			freqs[i+1] = uint16(int32(freqs[i]) + int32(d))
		}
		for cycleLen := 2; cycleLen <= 4; cycleLen++ {
			if len(freqs) <= cycleLen {
				continue
			}
			for headLen := 1; headLen < len(freqs)-cycleLen; headLen++ {
				cycle := freqs[headLen : headLen+cycleLen]
				allMatch := true
				for i := headLen + cycleLen; i < len(freqs); i++ {
					if freqs[i] != cycle[(i-headLen)%cycleLen] {
						allMatch = false
						break
					}
				}
				if allMatch {
					data := make([]byte, 2+headLen*2+cycleLen*2)
					data[0] = byte(headLen)
					data[1] = byte(cycleLen)
					for i := 0; i < headLen; i++ {
						data[2+i*2] = byte(freqs[i])
						data[2+i*2+1] = byte(freqs[i] >> 8)
					}
					for i := 0; i < cycleLen; i++ {
						data[2+headLen*2+i*2] = byte(cycle[i])
						data[2+headLen*2+i*2+1] = byte(cycle[i] >> 8)
					}
					return true, 13, data
				}
			}
		}
	}

	return false, 0, nil
}
