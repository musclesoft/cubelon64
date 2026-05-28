// Tracker format extraction from optimized Note structures
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

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
	SustainTable             []byte                     // Flat lookup: sustainTable[instr.SustainTableOffset+durIdx] = sustainIdx
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
	StartValue uint8
	Values     []uint8 // unary bit-stream of -1 steps; nil/empty = constant (StartValue for all frames)
}

type TrackerInstrument struct {
	BaseWaveform       byte   // Triangle, sawtooth, pulse, noise (NOT 0x05)
	EnvelopeMode       bool   // If true, player toggles between 0x05 and BaseWaveform
	NoisePlusTone      bool   // If true, mixer state determines waveform (noise vs pulse)
	TogglePeriod       uint16 // SID cycles for waveform toggle (0 if no toggle needed)
	TailStartFrame     byte   // 0=no tail, 0xFF=tail==duration, else constant frame offset
	ADSR               ADSRParams
	SustainTableOffset byte   // offset into SustainTable: sustainTable[offset+durIdx] = sustainIdx
	SustainMinDurIdx   byte   // minimum durIdx this instrument uses (baked into offset)
	SustainMaxDurIdx   byte   // maximum durIdx this instrument uses (inclusive)
	// VibratoDepth/Speed/Delay omitted — all zero in this song
}

type TrackerPattern struct {
	FreqIdx      byte
	Duration     byte
	InstrumentID byte
	FreqModIdx   byte // 0xFF=none, 0xFE=inline vibrato, 0-N=freqmod pattern index
}

type TrackerBlock struct {
	Patterns []byte // Pattern IDs (variable length)
}

type TrackerStream []byte // Bytecode: 0x00-0x7F = play block, 0x80+ = commands

// instrDurSnapshotKey is the key for sustain table snapshots: (instrID, origDur).
type instrDurSnapshotKey struct{ instr int; dur byte }

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
	origDur         byte // non-zero = pre-absorption dur (set by insertAndAbsorbRests)
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
// freqSeqKey builds the canonical glissando map key for a frequency sequence:
// startFreq followed by comma-separated int16 deltas.
func freqSeqKey(freqs []uint16) string {
	key := fmt.Sprintf("%d", freqs[0])
	for j := 1; j < len(freqs); j++ {
		key += fmt.Sprintf(",%d", int16(freqs[j])-int16(freqs[j-1]))
	}
	return key
}

// notePatternKey builds the composite dedup key for a note pattern entry.
func notePatternKey(freqIdx byte, dur byte, instrumentID int, freqSeqIndex byte, isRest bool, toneSeqKey string) string {
	return fmt.Sprintf("%d_%d_%d_%d_%v_%s", freqIdx, dur, instrumentID, freqSeqIndex, isRest, toneSeqKey)
}

// activeSPF returns the portion of SustainPerFrame to encode.
// S is written every frame including tail frames (D>0 makes the envelope decay toward S*17
// during the tail phase), so the full slice is always needed.
func activeSPF(note Note) []uint8 {
	return note.SustainPerFrame
}

// lookupSustainIndex returns the sustain pattern index for a per-frame sustain slice,
// or 0xFF if the slice is empty or not found in sustainPatternMap.
func lookupSustainIndex(sustain []uint8, sustainPatternMap map[string]int) byte {
	if len(sustain) == 0 {
		return 0xFF
	}
	if idx, exists := sustainPatternMap[sustainMapKey(sustain)]; exists {
		return byte(idx)
	}
	return 0xFF
}

// freqSeqIndex resolves a frequency sequence to its glissando map index (or 0xFE for vibrato,
// 0xFF for no modulation). freqSeqMap keys use freqSeqKey format.
func resolveFreqSeqIndex(freqSeq []uint16, freqSeqMap map[string]int) byte {
	if len(freqSeq) <= 1 {
		return 0xFF
	}
	hasChange := false
	isMonotonic := true
	lastNonZeroDir := 0
	for i := 1; i < len(freqSeq); i++ {
		delta := int(freqSeq[i]) - int(freqSeq[i-1])
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
	if !hasChange {
		return 0xFF
	}
	key := freqSeqKey(freqSeq)
	if idx, exists := freqSeqMap[key]; exists {
		return byte(idx)
	}
	if !isMonotonic {
		return 0xFE
	}
	return 0xFF
}

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

	// Two-phase sustain table approach:
	// Phase 1: buildSustainTable on original notes → correct instrument splits, snapshot.
	// Phase 2: absorb rests (only notes silent at end), rebuild durToIdx, rebuild sustain table.

	buildDurToIdx := func(notes []NoteWithMetadata) map[byte]byte {
		durSet := make(map[byte]struct{})
		for _, n := range notes {
			if !n.isRest { durSet[n.dur] = struct{}{} }
		}
		durInts := make([]int, 0, len(durSet))
		for d := range durSet { durInts = append(durInts, int(d)) }
		sort.Ints(durInts)
		m := make(map[byte]byte, len(durInts))
		for i, d := range durInts { m[byte(d)] = byte(i) }
		return m
	}

	// Phase 1.
	durToIdx := buildDurToIdx(allNotes)
	var sustainTable []byte
	sustainTable, instruments, allNotes = buildSustainTable(allNotes, instruments, instrumentMap, sustainPatternMap, durToIdx)

	// Snapshot (instrID, dur) → sustainIdx from the phase-1 table.
	sustainSnapshot := make(map[instrDurSnapshotKey]byte)
	for _, n := range allNotes {
		if n.isRest || len(n.Note.SustainPerFrame) == 0 { continue }
		ins := instruments[n.instrumentID]
		if ins.SustainTableOffset == 0xFF { continue }
		di, ok := durToIdx[n.dur]
		if !ok { continue }
		tableIdx := int(ins.SustainTableOffset) + int(di)
		if tableIdx >= 0 && tableIdx < len(sustainTable) {
			if si := sustainTable[tableIdx]; si != 0xFF {
				sustainSnapshot[instrDurSnapshotKey{n.instrumentID, n.dur}] = si
			}
		}
	}

	// Phase 2: absorb rests into preceding notes where safe (SID silent at note-end).
	allNotes = insertAndAbsorbRests(allNotes)

	// Phase 2: rebuild durToIdx and sustain table.
	durToIdx = buildDurToIdx(allNotes)
	sustainTable, instruments, allNotes = rebuildSustainTableFromSnapshot(allNotes, instruments, sustainSnapshot, durToIdx)

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

	// Build note patterns (sustain no longer part of key — determined by instr+dur)
	notePatterns, notePatternMap := buildNotePatternsFromMetadata(allNotes, freqIndex, freqSeqMap)

	// Assign pattern IDs
	for i := range allNotes {
		var freqIdx byte
		if allNotes[i].isRest {
			freqIdx = 255
		} else {
			freqIdx = freqIndex[allNotes[i].freq]
		}
		freqSeqIndex := resolveFreqSeqIndex(allNotes[i].freqSeq, freqSeqMap)
		key := notePatternKey(freqIdx, allNotes[i].dur, allNotes[i].instrumentID, freqSeqIndex, allNotes[i].isRest, allNotes[i].toneSeqKey)
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
		freqSeqIdx := resolveFreqSeqIndex(allNotes[i].freqSeq, freqSeqMap)
		patKey := notePatternKey(freqIndex[allNotes[i].freq], allNotes[i].dur, allNotes[i].instrumentID, freqSeqIdx, allNotes[i].isRest, allNotes[i].toneSeqKey)
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
	var durationTable []byte
	{
		durSetFinal := make(map[byte]struct{})
		for _, p := range notePatterns {
			if p.FreqIdx != 255 { durSetFinal[p.Duration] = struct{}{} }
		}
		durIntsFinal := make([]int, 0, len(durSetFinal))
		for d := range durSetFinal { durIntsFinal = append(durIntsFinal, int(d)) }
		sort.Ints(durIntsFinal)
		durationTable = make([]byte, len(durIntsFinal))
		durToIdx = make(map[byte]byte, len(durIntsFinal))
		for i, d := range durIntsFinal {
			durationTable[i] = byte(d)
			durToIdx[byte(d)] = byte(i)
		}
		if len(durationTable) > 16 {
			panic(fmt.Sprintf("too many unique durations (%d) for 4-bit index", len(durationTable)))
		}
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
		SustainTable:            sustainTable,
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

// buildSustainTable builds the flat sustain lookup table and assigns SustainTableOffset to each
// instrument. If any (instr, durIdx) pair maps to multiple sustain indices, a new instrument is
// created for the minority case and affected notes are reassigned. Returns the table and updated
// instruments slice.
func buildSustainTable(notes []NoteWithMetadata, instruments []TrackerInstrument, instrumentMap map[string]int, sustainPatternMap map[string]int, durToIdx map[byte]byte) ([]byte, []TrackerInstrument, []NoteWithMetadata) {
	type key struct{ instr, durIdx byte }

	// First pass: collect (instr, durIdx) → sustainIdx mapping, detecting conflicts.
	// splitFor maps (origInstr, durIdx, si) → newInstrID to reuse splits for the same minority si.
	type splitKey struct{ instr, durIdx, si byte }
	first := make(map[key]byte)
	splitFor := make(map[splitKey]byte)
	for i, n := range notes {
		if n.isRest || len(n.Note.SustainPerFrame) == 0 {
			continue
		}
		di := durToIdx[n.dur]
		si := lookupSustainIndex(activeSPF(n.Note), sustainPatternMap)
		if si == 0xFF {
			continue
		}
		k := key{byte(n.instrumentID), di}
		if existing, ok := first[k]; !ok {
			first[k] = si
		} else if existing != si {
			// Conflict: reuse an existing split for this (origInstr, durIdx, si) or create one.
			sk := splitKey{byte(n.instrumentID), di, si}
			if newID, exists := splitFor[sk]; exists {
				notes[i].instrumentID = int(newID)
			} else {
				newID := byte(len(instruments))
				instruments = append(instruments, instruments[n.instrumentID])
				notes[i].instrumentID = int(newID)
				first[key{newID, di}] = si
				splitFor[sk] = newID
			}
		}
	}

	// Second pass: rebuild clean (instr, durIdx) → sustainIdx map after split.
	clean := make(map[key]byte)
	for _, n := range notes {
		if n.isRest || len(n.Note.SustainPerFrame) == 0 {
			continue
		}
		di := durToIdx[n.dur]
		si := lookupSustainIndex(activeSPF(n.Note), sustainPatternMap)
		if si == 0xFF {
			continue
		}
		clean[key{byte(n.instrumentID), di}] = si
	}

	// For each instrument, find min/max durIdx, allocate table slice.
	numInstrs := len(instruments)
	minDurIdx := make([]byte, numInstrs)
	maxDurIdx := make([]byte, numInstrs)
	hasEntry := make([]bool, numInstrs)
	for k := range clean {
		id := int(k.instr)
		if !hasEntry[id] {
			minDurIdx[id] = k.durIdx
			maxDurIdx[id] = k.durIdx
			hasEntry[id] = true
		} else {
			if k.durIdx < minDurIdx[id] { minDurIdx[id] = k.durIdx }
			if k.durIdx > maxDurIdx[id] { maxDurIdx[id] = k.durIdx }
		}
	}

	// Build flat table, assign offsets.
	// Pad table so len(table) >= minDurIdx[id] before computing offset, preventing byte-wrap.
	var table []byte
	for id := range instruments {
		if !hasEntry[id] {
			instruments[id].SustainTableOffset = 0xFF
			instruments[id].SustainMinDurIdx = 0
			continue
		}
		for len(table) < int(minDurIdx[id]) {
			table = append(table, 0xFF)
		}
		offset := byte(len(table)) - minDurIdx[id] // so table[offset+durIdx] is correct
		instruments[id].SustainTableOffset = offset
		instruments[id].SustainMinDurIdx = minDurIdx[id]
		instruments[id].SustainMaxDurIdx = maxDurIdx[id]
		sliceLen := int(maxDurIdx[id]) - int(minDurIdx[id]) + 1
		for j := 0; j < sliceLen; j++ {
			di := minDurIdx[id] + byte(j)
			if si, ok := clean[key{byte(id), di}]; ok {
				table = append(table, si)
			} else {
				table = append(table, 0xFF) // unused slot
			}
		}
	}

	fmt.Printf("  Sustain table: %d bytes (%d instruments)\n", len(table), numInstrs)
	return table, instruments, notes
}

// insertAndAbsorbRests folds each gap rest between notes into the preceding note when:
//   (a) the combined duration already exists in the note duration set, and
//   (b) the note's SustainPerFrame ends at 0 — SID is already silent at note-end.
// The gap rests are computed from StartFrame differences (same as buildBlocksFromMetadata does).
// Returns notes-only (no rest entries); absorbed notes have origDur set, dur extended.
// buildBlocksFromMetadata continues to handle rest pattern creation from StartFrame gaps.
func insertAndAbsorbRests(allNotes []NoteWithMetadata) []NoteWithMetadata {
	byChannel := [3][]NoteWithMetadata{}
	for _, n := range allNotes {
		byChannel[n.Channel] = append(byChannel[n.Channel], n)
	}

	// Build set of existing note durations for the constraint check.
	noteDurSet := make(map[byte]bool)
	for _, n := range allNotes {
		if !n.isRest { noteDurSet[n.dur] = true }
	}

	// Absorb: for each note followed by a gap rest (from StartFrame difference),
	// fold the rest into the note when both conditions are met.
	for ch := 0; ch < 3; ch++ {
		src := byChannel[ch]
		for i := range src {
			note := &src[i]
			if note.isRest { continue }
			// Compute the gap after this note.
			var gapDur int
			if i+1 < len(src) {
				nextStart := src[i+1].StartFrame
				noteEnd := note.StartFrame + int(note.dur)
				gapDur = nextStart - noteEnd
			}
			if gapDur <= 0 || gapDur > 255 { continue }
			if int(note.dur)+gapDur > 255 { continue }
			combined := note.dur + byte(gapDur)
			spf := note.Note.SustainPerFrame
			// Safe to absorb only when: (1) combined dur already exists, (2) no decay tail
			// (TailStartFrame==0 means purely S-driven volume — when S=0, level=0 immediately),
			// and (3) S=0 at note-end.
			noDecayTail := note.Note.TailStartFrame == 0
			silentAtEnd := len(spf) > 0 && spf[len(spf)-1] == 0
			if noteDurSet[combined] && noDecayTail && silentAtEnd {
				if note.origDur == 0 {
					note.origDur = note.dur
				}
				note.dur = combined
			}
		}
		byChannel[ch] = src
	}

	var result []NoteWithMetadata
	for ch := 0; ch < 3; ch++ {
		result = append(result, byChannel[ch]...)
	}
	return result
}

// rebuildSustainTableFromSnapshot rebuilds the sustain table after rest absorption using a
// snapshot of (instrID, origDur) → sustainIdx. Non-absorbed notes take priority; absorbed notes
// that would conflict with an existing genuine entry adopt it without triggering a split.
func rebuildSustainTableFromSnapshot(notes []NoteWithMetadata, instruments []TrackerInstrument, snapshot map[instrDurSnapshotKey]byte, durToIdx map[byte]byte) ([]byte, []TrackerInstrument, []NoteWithMetadata) {
	type key struct{ instr, durIdx byte }
	type splitKey struct{ instr, durIdx, si byte }

	lookupDur := func(n NoteWithMetadata) byte {
		if n.origDur != 0 { return n.origDur }
		return n.dur
	}

	first := make(map[key]byte)
	splitFor := make(map[splitKey]byte)

	processNote := func(i int, n NoteWithMetadata, isAbsorbed bool) {
		si, hasSI := snapshot[instrDurSnapshotKey{n.instrumentID, lookupDur(n)}]
		if !hasSI { return }
		di, ok := durToIdx[n.dur]
		if !ok { return }
		k := key{byte(n.instrumentID), di}
		if existing, ok2 := first[k]; !ok2 {
			first[k] = si
		} else if existing != si {
			if isAbsorbed {
				return // adopt existing — no split
			}
			sk := splitKey{byte(n.instrumentID), di, si}
			if newID, exists := splitFor[sk]; exists {
				notes[i].instrumentID = int(newID)
			} else {
				newID := byte(len(instruments))
				instruments = append(instruments, instruments[n.instrumentID])
				notes[i].instrumentID = int(newID)
				first[key{newID, di}] = si
				splitFor[sk] = newID
			}
		}
	}

	for i, n := range notes {
		if n.isRest || n.origDur != 0 { continue }
		processNote(i, n, false)
	}
	for i, n := range notes {
		if n.isRest || n.origDur == 0 { continue }
		processNote(i, n, true)
	}

	// Rebuild clean map.
	clean := make(map[key]byte)
	for _, n := range notes {
		if n.isRest { continue }
		si, hasSI := snapshot[instrDurSnapshotKey{n.instrumentID, lookupDur(n)}]
		if !hasSI { continue }
		di, ok := durToIdx[n.dur]
		if !ok { continue }
		k := key{byte(n.instrumentID), di}
		if _, exists := clean[k]; !exists {
			clean[k] = si
		}
	}

	numInstrs := len(instruments)
	minDurIdx := make([]byte, numInstrs)
	maxDurIdx := make([]byte, numInstrs)
	hasEntry := make([]bool, numInstrs)
	for k := range clean {
		id := int(k.instr)
		if !hasEntry[id] {
			minDurIdx[id] = k.durIdx
			maxDurIdx[id] = k.durIdx
			hasEntry[id] = true
		} else {
			if k.durIdx < minDurIdx[id] { minDurIdx[id] = k.durIdx }
			if k.durIdx > maxDurIdx[id] { maxDurIdx[id] = k.durIdx }
		}
	}

	var table []byte
	for id := range instruments {
		if !hasEntry[id] {
			instruments[id].SustainTableOffset = 0xFF
			instruments[id].SustainMinDurIdx = 0
			continue
		}
		for len(table) < int(minDurIdx[id]) {
			table = append(table, 0xFF)
		}
		offset := byte(len(table)) - minDurIdx[id]
		instruments[id].SustainTableOffset = offset
		instruments[id].SustainMinDurIdx = minDurIdx[id]
		instruments[id].SustainMaxDurIdx = maxDurIdx[id]
		sliceLen := int(maxDurIdx[id]) - int(minDurIdx[id]) + 1
		for j := 0; j < sliceLen; j++ {
			di := minDurIdx[id] + byte(j)
			if si, ok := clean[key{byte(id), di}]; ok {
				table = append(table, si)
			} else {
				table = append(table, 0xFF)
			}
		}
	}

	fmt.Printf("  Sustain table (rebuilt): %d bytes (%d instruments)\n", len(table), numInstrs)
	return table, instruments, notes
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
			continue
		}

		key := sustainMapKey(activeSPF(note.Note))

		// Add pattern if not seen before
		if _, exists := sustainMap[key]; !exists {
			sustainMap[key] = len(patterns)
			sustain := activeSPF(note.Note)
			sp := SustainPattern{StartValue: sustain[0]}
			if len(key) <= 6 || key[:6] != "const:" {
				// Unary bit-stream, LSB-first: one 1-bit per -1 step, 0-bit = next frame.
				// Sustain curves are non-increasing; panic if an increase is encountered.
				for i := 1; i < len(sustain); i++ {
					if int8(sustain[i])-int8(sustain[i-1]) > 0 {
						panic(fmt.Sprintf("sustain curve increases at frame %d: %d -> %d", i, sustain[i-1], sustain[i]))
					}
				}
				// Trim trailing frames once value reaches 0.
				trimmed := sustain
				for i, v := range sustain {
					if v == 0 {
						trimmed = sustain[:i+1]
						break
					}
				}
				var packed []uint8
				var cur uint8
				var bitPos uint
				emitBit := func(b uint8) {
					cur |= b << bitPos
					bitPos++
					if bitPos == 8 {
						packed = append(packed, cur)
						cur = 0
						bitPos = 0
					}
				}
				for i := 1; i < len(trimmed); i++ {
					drop := int(trimmed[i-1]) - int(trimmed[i])
					for j := 0; j < drop; j++ {
						emitBit(1) // -1
					}
					if i < len(trimmed)-1 {
						emitBit(0) // advance to next frame (omit for last frame)
					}
				}
				if bitPos > 0 {
					packed = append(packed, cur)
				}
				sp.Values = packed
			}
			patterns = append(patterns, sp)
		}
	}

	fmt.Printf("  Sustain patterns: %d unique sequences\n", len(patterns))

	// Analyze sustain patterns for compression opportunities
	if len(patterns) > 0 {
		fmt.Printf("\n=== Sustain Pattern Analysis ===\n")
		const chartW = 64
		const volMin, volMax = 0, 15
		bars := " .,:;+=xX#"
		for i, p := range patterns {
			vals := []byte{p.StartValue}
			cur := p.StartValue
			byteIdx, bitPos := 0, uint(0)
			readBit := func() uint8 {
				if byteIdx >= len(p.Values) { return 0 }
				b := (p.Values[byteIdx] >> bitPos) & 1
				bitPos++
				if bitPos == 8 { bitPos = 0; byteIdx++ }
				return b
			}
			for byteIdx < len(p.Values) {
				bit := readBit()
				if bit == 1 {
					cur--
				} else {
					vals = append(vals, cur)
					if cur == 0 { break }
				}
			}
			tag := "DELTA"
			if len(p.Values) == 0 {
				tag = "CONST"
			}
			row := make([]byte, chartW)
			for col := 0; col < chartW; col++ {
				srcIdx := col * len(vals) / chartW
				v := int(vals[srcIdx])
				if v < volMin { v = volMin }
				if v > volMax { v = volMax }
				barIdx := v * (len(bars) - 1) / volMax
				row[col] = bars[barIdx]
			}
			fmt.Printf("  [%2d] %s len=%3d start=%2d  %s\n",
				i, tag, len(vals), p.StartValue, string(row))
		}
	}

	// Shape deduplication: patterns with the same decoded value sequence can share one index.
	// The player naturally truncates at note duration, so a shorter note pointing at a longer
	// pattern is correct. This also handles exact duplicates (same length, same values).
	expandPat := func(i int) []byte {
		p := patterns[i]
		out := []byte{p.StartValue}
		cur := p.StartValue
		byteIdx, bitPos := 0, uint(0)
		readBit := func() uint8 {
			if byteIdx >= len(p.Values) { return 0 }
			b := (p.Values[byteIdx] >> bitPos) & 1
			bitPos++
			if bitPos == 8 { bitPos = 0; byteIdx++ }
			return b
		}
		for byteIdx < len(p.Values) {
			bit := readBit()
			if bit == 1 {
				cur--
			} else {
				out = append(out, cur)
				if cur == 0 { break }
			}
		}
		return out
	}

	expanded := make([][]byte, len(patterns))
	for i := range patterns {
		expanded[i] = expandPat(i)
	}

	// For each pattern, find the longest pattern whose decoded values start with this one's
	// values (or are equal). The player truncates at note duration, so a shorter pattern
	// pointing at a longer one is correct.
	shapeRemap := make([]int, len(patterns))
	for i := range shapeRemap {
		shapeRemap[i] = i
	}
	for i := range patterns {
		for j := range patterns {
			if i == j {
				continue
			}
			a, b := expanded[i], expanded[j]
			// j is a valid target if a is a prefix-or-equal of b, and b is longer than current best
			if len(b) < len(a) {
				continue
			}
			match := true
			for k := range a {
				if a[k] != b[k] {
					match = false
					break
				}
			}
			// prefer longer base; break ties by lower index for determinism
			curBest := expanded[shapeRemap[i]]
			if match && (len(b) > len(curBest) || (len(b) == len(curBest) && j < shapeRemap[i])) {
				shapeRemap[i] = j
			}
		}
	}

	needShapeRemap := false
	for i, s := range shapeRemap {
		if s != i {
			needShapeRemap = true
			break
		}
	}
	if needShapeRemap {
		for key, idx := range sustainMap {
			sustainMap[key] = shapeRemap[idx]
		}
		// Compact: remove patterns that are now aliased
		compact2 := make([]int, len(patterns))
		newPatterns2 := patterns[:0:len(patterns)]
		newPatterns2 = newPatterns2[:0]
		for i := range patterns {
			if shapeRemap[i] == i {
				compact2[i] = len(newPatterns2)
				newPatterns2 = append(newPatterns2, patterns[i])
			} else {
				compact2[i] = -1 // will be resolved after
			}
		}
		// Resolve compacted indices through the shapeRemap chain
		for i := range patterns {
			if compact2[i] == -1 {
				compact2[i] = compact2[shapeRemap[i]]
			}
		}
		for key, idx := range sustainMap {
			sustainMap[key] = compact2[idx]
		}
		fmt.Printf("  Sustain shape dedup: %d → %d patterns (%d merged)\n",
			len(patterns), len(newPatterns2), len(patterns)-len(newPatterns2))
		patterns = newPatterns2
	}

	return patterns, sustainMap
}

func buildNotePatternsFromMetadata(notes []NoteWithMetadata, freqIndex map[uint16]byte, freqSeqMap map[string]int) ([]TrackerPattern, map[string]int) {
	patternMap := make(map[string]int)
	patterns := []TrackerPattern{}

	glissandoAttempts := 0
	glissandoMatches := 0

	for _, note := range notes {
		var freqIdx byte
		if note.isRest {
			freqIdx = 255
		} else {
			freqIdx = freqIndex[note.freq]
		}
		freqSeqIndex := resolveFreqSeqIndex(note.freqSeq, freqSeqMap)
		if len(note.freqSeq) > 1 {
			hasChange := false
			for i := 1; i < len(note.freqSeq); i++ {
				if note.freqSeq[i] != note.freqSeq[i-1] {
					hasChange = true
					break
				}
			}
			if hasChange {
				glissandoAttempts++
				if freqSeqIndex != 0xFF && freqSeqIndex != 0xFE {
					glissandoMatches++
				} else if freqSeqIndex == 0xFF {
					k := freqSeqKey(note.freqSeq)
					fmt.Printf("⚠ Failed to find glissando pattern start=%d len=%d key=%s\n", note.StartFrame, len(note.freqSeq), k[:min(50, len(k))])
				}
			}
		}
		key := notePatternKey(freqIdx, note.dur, note.instrumentID, freqSeqIndex, note.isRest, note.toneSeqKey)
		if _, exists := patternMap[key]; !exists {
			patternMap[key] = len(patterns)
			patterns = append(patterns, TrackerPattern{
				FreqIdx:      freqIdx,
				Duration:     note.dur,
				InstrumentID: byte(note.instrumentID),
				FreqModIdx:   freqSeqIndex,
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

	// Sort note patterns: freqMod patterns first, then plain patterns.
	// This allows the serializer to omit byte 3 for the plain suffix.
	// Apply a final remap to blocks, streams, and finalRemap.
	{
		pats := *notePatterns
		n := len(pats)
		fmRemap := make([]byte, n)
		newID := 0
		// Pass 1: freqMod patterns
		for i, p := range pats {
			if p.FreqModIdx != 0xFF {
				fmRemap[i] = byte(newID); newID++
			}
		}
		freqModCount := newID
		// Pass 2: plain patterns
		for i, p := range pats {
			if p.FreqModIdx == 0xFF {
				fmRemap[i] = byte(newID); newID++
			}
		}
		if freqModCount < len(pats) { // only reshuffle if ordering changes anything
			reordered := make([]TrackerPattern, n)
			for old, nw := range fmRemap { reordered[nw] = pats[old] }
			*notePatterns = reordered
			// Remap block content
			for bi := range blocks {
				for j, b := range blocks[bi].Patterns {
					if int(b) < n { blocks[bi].Patterns[j] = fmRemap[b] }
				}
			}
			// Remap stream direct-refs
			for ch := 0; ch < 3; ch++ {
				for i, b := range streams[ch] {
					if b >= directRefBase && b < 0xEE {
						streams[ch][i] = directRefBase + fmRemap[b-directRefBase]
					}
				}
			}
			// Compose into finalRemap
			for orig := range finalRemap {
				if finalRemap[orig] != 0xFF {
					finalRemap[orig] = fmRemap[finalRemap[orig]]
				}
			}
		}
		fmt.Printf("  FreqMod patterns: %d, plain patterns: %d\n", freqModCount, len(pats)-freqModCount)
	}

	return blocks, streams, restDurations, finalRemap
}

func absorbRests(format *TrackerFormat) {
	blockCount := len(format.Blocks)

	existingDurs := make(map[byte]bool, len(format.DurationTable))
	for _, d := range format.DurationTable { existingDurs[d] = true }

	type entry struct { isRest bool; pid, dur byte }
	expand := func(bytecodes []byte) []entry {
		var seq []entry
		for _, b := range bytecodes {
			if int(b) < blockCount {
				for _, bb := range format.Blocks[b].Patterns {
					if bb >= 0xEE {
						seq = append(seq, entry{true, 0xFF, format.RestDurations[0xFF-bb]})
					} else {
						seq = append(seq, entry{false, bb, 0})
					}
				}
			} else if b >= 0xEE {
				seq = append(seq, entry{true, 0xFF, format.RestDurations[b-0xEE]})
			} else {
				seq = append(seq, entry{false, b - byte(blockCount), 0})
			}
		}
		return seq
	}

	type followMap = map[byte]map[byte]int
	pidFollowing := make(followMap)
	for ch := 0; ch < 3; ch++ {
		for i, e := range expand(format.Streams[ch]) {
			if e.isRest { continue }
			if pidFollowing[e.pid] == nil { pidFollowing[e.pid] = make(map[byte]int) }
			key := byte(255)
			if i+1 < len(expand(format.Streams[ch])) && expand(format.Streams[ch])[i+1].isRest {
				key = expand(format.Streams[ch])[i+1].dur
			}
			pidFollowing[e.pid][key]++
		}
	}

	pidAbsorb := make(map[byte]byte)
	for pid := 0; pid < len(format.NotePatterns); pid++ {
		following := pidFollowing[byte(pid)]
		if following == nil || following[255] > 0 { continue }
		notePat := format.NotePatterns[pid]
		var restKeys []byte
		for k := range following {
			if k != 255 && existingDurs[notePat.Duration+k] { restKeys = append(restKeys, k) }
		}
		if len(restKeys) == 1 {
			pidAbsorb[byte(pid)] = restKeys[0]
		}
	}

	type instrDurKey struct{ instr, dur byte }
	sustainSnapshot := make(map[instrDurKey]byte)
	{
		snapDurIdx := make(map[byte]byte, len(format.DurationTable))
		for i, d := range format.DurationTable { snapDurIdx[d] = byte(i) }
		for _, p := range format.NotePatterns {
			if p.FreqIdx == 0xFF { continue }
			ins := format.Instruments[p.InstrumentID]
			if ins.SustainTableOffset == 0xFF && ins.SustainMinDurIdx == 0 { continue }
			di, ok := snapDurIdx[p.Duration]
			if !ok { continue }
			tableIdx := int(ins.SustainTableOffset + byte(di))
			if tableIdx >= len(format.SustainTable) {
				panic(fmt.Sprintf("absorbRests: sustain table OOB: instr=%d dur=%d di=%d offset=%d tableIdx=%d len=%d",
					p.InstrumentID, p.Duration, di, ins.SustainTableOffset, tableIdx, len(format.SustainTable)))
			}
			if si := format.SustainTable[tableIdx]; si != 0xFF {
				sustainSnapshot[instrDurKey{p.InstrumentID, p.Duration}] = si
			}
		}
	}

	for pid, rd := range pidAbsorb {
		format.NotePatterns[pid].Duration += rd
	}

	rewriteStream := func(bytecodes TrackerStream) TrackerStream {
		out := TrackerStream{}
		for i := 0; i < len(bytecodes); {
			b := bytecodes[i]
			if int(b) < blockCount || b >= 0xEE {
				out = append(out, b)
				i++
			} else {
				pid := b - byte(blockCount)
				if i+1 < len(bytecodes) && bytecodes[i+1] >= 0xEE {
					if _, ok := pidAbsorb[pid]; ok {
						out = append(out, b)
						i += 2
						continue
					}
				}
				out = append(out, b)
				i++
			}
		}
		return out
	}

	rewriteBlock := func(patterns []byte) []byte {
		out := []byte{}
		for i := 0; i < len(patterns); {
			b := patterns[i]
			if b >= 0xEE {
				out = append(out, b)
				i++
			} else {
				if i+1 < len(patterns) && patterns[i+1] >= 0xEE {
					if _, ok := pidAbsorb[b]; ok {
						out = append(out, b)
						i += 2
						continue
					}
				}
				out = append(out, b)
				i++
			}
		}
		return out
	}

	for i := range format.Blocks {
		format.Blocks[i].Patterns = rewriteBlock(format.Blocks[i].Patterns)
	}
	for ch := 0; ch < 3; ch++ {
		format.Streams[ch] = rewriteStream(format.Streams[ch])
	}

	{
		usedPIDs := make(map[byte]bool)
		for _, block := range format.Blocks {
			for _, b := range block.Patterns {
				if b < 0xEE { usedPIDs[b] = true }
			}
		}
		for ch := 0; ch < 3; ch++ {
			for _, b := range format.Streams[ch] {
				if b >= byte(blockCount) && b < 0xEE { usedPIDs[b-byte(blockCount)] = true }
			}
		}
		oldToNew := make(map[byte]byte)
		newPats := []TrackerPattern{}
		for i, p := range format.NotePatterns {
			if usedPIDs[byte(i)] {
				oldToNew[byte(i)] = byte(len(newPats))
				newPats = append(newPats, p)
			}
		}
		format.NotePatterns = newPats
		for i := range format.Blocks {
			for j, b := range format.Blocks[i].Patterns {
				if b < 0xEE { format.Blocks[i].Patterns[j] = oldToNew[b] }
			}
		}
		for ch := 0; ch < 3; ch++ {
			for j, b := range format.Streams[ch] {
				if b >= byte(blockCount) && b < 0xEE {
					format.Streams[ch][j] = byte(blockCount) + oldToNew[b-byte(blockCount)]
				}
			}
		}
	}

	durSet := make(map[byte]struct{})
	for _, p := range format.NotePatterns {
		if p.FreqIdx != 0xFF { durSet[p.Duration] = struct{}{} }
	}
	durInts := make([]int, 0, len(durSet))
	for d := range durSet { durInts = append(durInts, int(d)) }
	sort.Ints(durInts)
	if len(durInts) > 16 {
		panic(fmt.Sprintf("too many unique durations after rest absorption (%d) for 4-bit index", len(durInts)))
	}
	format.DurationTable = make([]byte, len(durInts))
	for i, d := range durInts { format.DurationTable[i] = byte(d) }
	newDurIdx := make(map[byte]byte, len(format.DurationTable))
	for i, d := range format.DurationTable { newDurIdx[d] = byte(i) }

	type siKey struct{ instr, durIdx byte }
	type splitKey2 struct{ instr, durIdx, si byte }
	clean := make(map[siKey]byte)
	splitFor2 := make(map[splitKey2]byte)
	processPattern := func(i int, p TrackerPattern, isAbsorbed bool) {
		lookupDur := p.Duration
		if isAbsorbed {
			lookupDur = p.Duration - pidAbsorb[byte(i)]
		}
		si, hasSI := sustainSnapshot[instrDurKey{p.InstrumentID, lookupDur}]
		if !hasSI { return }
		di, ok := newDurIdx[p.Duration]
		if !ok { return }
		k := siKey{p.InstrumentID, di}
		if existing, exists := clean[k]; exists && existing != si {
			if isAbsorbed { return }
			sk := splitKey2{p.InstrumentID, di, si}
			if newID, exists2 := splitFor2[sk]; exists2 {
				format.NotePatterns[i].InstrumentID = newID
			} else {
				newID := byte(len(format.Instruments))
				format.Instruments = append(format.Instruments, format.Instruments[p.InstrumentID])
				format.NotePatterns[i].InstrumentID = newID
				clean[siKey{newID, di}] = si
				splitFor2[sk] = newID
			}
		} else {
			clean[k] = si
		}
	}
	for i, p := range format.NotePatterns {
		if p.FreqIdx == 0xFF { continue }
		if _, absorbed := pidAbsorb[byte(i)]; !absorbed {
			processPattern(i, p, false)
		}
	}
	for i, p := range format.NotePatterns {
		if p.FreqIdx == 0xFF { continue }
		if _, absorbed := pidAbsorb[byte(i)]; absorbed {
			processPattern(i, p, true)
		}
	}
	numInstrs := len(format.Instruments)
	minDurIdx2 := make([]byte, numInstrs)
	maxDurIdx2 := make([]byte, numInstrs)
	hasEntry2 := make([]bool, numInstrs)
	for k := range clean {
		id := int(k.instr)
		if !hasEntry2[id] {
			minDurIdx2[id] = k.durIdx; maxDurIdx2[id] = k.durIdx; hasEntry2[id] = true
		} else {
			if k.durIdx < minDurIdx2[id] { minDurIdx2[id] = k.durIdx }
			if k.durIdx > maxDurIdx2[id] { maxDurIdx2[id] = k.durIdx }
		}
	}
	var newTable []byte
	for id := 0; id < numInstrs; id++ {
		if !hasEntry2[id] {
			format.Instruments[id].SustainTableOffset = 0xFF
			format.Instruments[id].SustainMinDurIdx = 0
			continue
		}
		for len(newTable) < int(minDurIdx2[id]) { newTable = append(newTable, 0xFF) }
		offset := byte(len(newTable)) - minDurIdx2[id]
		format.Instruments[id].SustainTableOffset = offset
		format.Instruments[id].SustainMinDurIdx = minDurIdx2[id]
		sliceLen := int(maxDurIdx2[id]) - int(minDurIdx2[id]) + 1
		for j := 0; j < sliceLen; j++ {
			di := minDurIdx2[id] + byte(j)
			if si, ok := clean[siKey{byte(id), di}]; ok {
				newTable = append(newTable, si)
			} else {
				newTable = append(newTable, 0xFF)
			}
		}
	}
	format.SustainTable = newTable

	restUsed := make(map[byte]bool)
	for _, block := range format.Blocks {
		for _, b := range block.Patterns {
			if b >= 0xEE { restUsed[format.RestDurations[0xFF-b]] = true }
		}
	}
	for ch := 0; ch < 3; ch++ {
		for _, b := range format.Streams[ch] {
			if b >= 0xEE { restUsed[format.RestDurations[b-0xEE]] = true }
		}
	}
	newRestDurs := []int{}
	for d := range restUsed { newRestDurs = append(newRestDurs, int(d)) }
	sort.Ints(newRestDurs)
	newRestDurSlice := make([]byte, len(newRestDurs))
	for i, d := range newRestDurs { newRestDurSlice[i] = byte(d) }
	oldToNew2 := make(map[byte]byte)
	for i, d := range format.RestDurations {
		for j, nd := range newRestDurSlice {
			if d == nd { oldToNew2[byte(i)] = byte(j) }
		}
	}
	for i := range format.Blocks {
		for j, b := range format.Blocks[i].Patterns {
			if b >= 0xEE { format.Blocks[i].Patterns[j] = 0xFF - oldToNew2[0xFF-b] }
		}
	}
	for ch := 0; ch < 3; ch++ {
		for j, b := range format.Streams[ch] {
			if b >= 0xEE { format.Streams[ch][j] = 0xEE + oldToNew2[b-0xEE] }
		}
	}
	format.RestDurations = newRestDurSlice

	{
		used := make(map[byte]bool)
		for _, si := range newTable {
			if si != 0xFF { used[si] = true }
		}
		remap := make(map[byte]byte)
		newSPs := []SustainPattern{}
		for i, sp := range format.SustainPatterns {
			if used[byte(i)] {
				remap[byte(i)] = byte(len(newSPs))
				newSPs = append(newSPs, sp)
			}
		}
		for i, si := range newTable {
			if si != 0xFF { newTable[i] = remap[si] }
		}
		format.SustainTable = newTable
		format.SustainPatterns = newSPs
	}
}

func printTrackerStats(format *TrackerFormat) {
	absorbRests(format)

	fmt.Printf("\nTracker Format Statistics:\n")
	fmt.Printf("  Frequencies: %d entries\n", len(format.FreqTable))
	fmt.Printf("  FreqMod patterns: %d entries\n", len(format.FreqModPatterns))
	fmt.Printf("  Instruments: %d entries\n", len(format.Instruments))
	fmt.Printf("  Note patterns: %d entries\n", len(format.NotePatterns))

	// Calculate ROM size — each section computed once and summed.

	freqTableSize := len(format.FreqTable)*2 +
		len(format.TogglePeriodValues)*2 +
		(len(format.FreqTable)+1)/2

	freqModSize := 0
	for _, pattern := range format.FreqModPatterns {
		if pattern.Type == 5 {
			tmpl := format.FreqSequenceTemplates[int(pattern.Data[0])]
			freqModSize += 1 + len(tmpl.Frequencies)*2 // header byte + freq×2 (no length prefix)
		} else {
			freqModSize += 1 + len(pattern.Data)
		}
	}

	delayedVibratoSize := len(format.DelayedVibratoTemplates) * 8

	sustainSize := 0
	for _, pattern := range format.SustainPatterns {
		sustainSize += 1 + len(pattern.Values)
	}

	instrumentSize := len(format.Instruments) * 4
	_, sustainExceptions := sustainDefaultsAndExceptions(format.Instruments, format.SustainTable)
	sustainTableSize := 1 + len(sustainExceptions)*3 // count(1) + exceptions×(instrID,durIdx,sustainIdx)
	restDurSize := len(format.RestDurations)
	durTableSize := len(format.DurationTable)
	fmCount := 0
	for _, p := range format.NotePatterns { if p.FreqModIdx != 0xFF { fmCount++ } }
	notePatternSize := len(format.NotePatterns)*2 + fmCount // 2 arrays + fm array (fmCount in header)

	blockSize := 0
	for _, block := range format.Blocks {
		blockSize += 1 + len(block.Patterns)
	}

	streamSize := 0
	for ch := 0; ch < 3; ch++ {
		streamSize += len(format.Streams[ch])
	}

	toneSeqSize := 0
	toneSeqSeen := make(map[byte]bool)
	for _, o := range format.ToneSeqOverrides {
		if !toneSeqSeen[o.TemplateIdx] {
			toneSeqSeen[o.TemplateIdx] = true
			tmpl := format.FreqSequenceTemplates[int(o.TemplateIdx)]
			steps := toneSeqDeltaSteps(tmpl.Frequencies, format.TogglePeriodValues)
			toneSeqSize += 1 + len(steps)*2 // initialToggleIdx(1) + steps×(delta:u8,hold:u8) including first step with delta=0
		}
	}
	toneOverrideSize := len(format.ToneSeqOverrides) * 2

	const sectionOffsetTableSize = 36 // 17 × uint16 LE section offsets + freqModCount(1) + pad(1)
	size := sectionOffsetTableSize + freqTableSize + freqModSize + toneSeqSize +
		delayedVibratoSize + sustainSize + instrumentSize + sustainTableSize +
		durTableSize + restDurSize + notePatternSize + toneOverrideSize +
		blockSize + streamSize

	fmt.Printf("\nROM size: %d bytes (%.1f KB)\n", size, float64(size)/1024.0)
	fmt.Printf("  Breakdown:\n")
	fmt.Printf("    Header:           %4d bytes (17 × uint16 LE section offsets + freqModCount + pad)\n", sectionOffsetTableSize)
	fmt.Printf("    Freq table:       %4d bytes (%d entries × 2 + %d toggle values × 2 + %d nibble-index bytes)\n",
		freqTableSize, len(format.FreqTable), len(format.TogglePeriodValues), (len(format.FreqTable)+1)/2)
	fmt.Printf("    FreqMod patterns: %4d bytes (%d patterns, type-5 with inline data)\n", freqModSize, len(format.FreqModPatterns))
	fmt.Printf("    Tone seqs:        %4d bytes (%d templates for tone overrides)\n", toneSeqSize, len(toneSeqSeen))
	fmt.Printf("    Tone overrides:   %4d bytes (%d entries)\n", toneOverrideSize, len(format.ToneSeqOverrides))
	fmt.Printf("    Delayed vibrato:  %4d bytes (%d templates × 8, bell only)\n",
		delayedVibratoSize, len(format.DelayedVibratoTemplates))
	fmt.Printf("    Sustain patterns: %4d bytes (%d patterns, packed header)\n", sustainSize, len(format.SustainPatterns))
	fmt.Printf("    Instruments:      %4d bytes (%d × 4)\n", instrumentSize, len(format.Instruments))
	fmt.Printf("    Sustain table:    %4d bytes (%d entries)\n", sustainTableSize, len(format.SustainTable))
	fmt.Printf("    Rest durations:   %4d bytes (%d unique durations, inline in stream)\n", restDurSize, len(format.RestDurations))
	fmt.Printf("    Duration table:   %4d bytes (%d unique durations)\n", durTableSize, len(format.DurationTable))
	fmt.Printf("    Note patterns:    %4d bytes (%d total, %d with freqMod, SoA)\n", notePatternSize, len(format.NotePatterns), fmCount)
	fmt.Printf("    Blocks:           %4d bytes (%d blocks)\n", blockSize, len(format.Blocks))
	fmt.Printf("    Streams:          %4d bytes (3 channels, no length prefix)\n", streamSize)

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

	// Histogram: rest duration immediately following a note, across all channels.
	// Also analyses per-pattern: pids that ALWAYS precede a 5 or 10 frame rest could
	// absorb that rest into the note duration, eliminating the rest opcode entirely.
	{
		type entry struct {
			isRest bool
			pid    byte // valid when !isRest
			dur    byte // valid when isRest
		}
		blockCount := len(format.Blocks)
		restAfterNote := make(map[byte]int)
		// pidFollowing[pid] maps rest_dur (or 255=note/end) to occurrence count
		pidFollowing := make(map[byte]map[byte]int)

		for ch := 0; ch < 3; ch++ {
			var seq []entry
			for _, b := range format.Streams[ch] {
				if int(b) < blockCount {
					for _, bb := range format.Blocks[b].Patterns {
						if bb >= 0xEE {
							seq = append(seq, entry{isRest: true, dur: format.RestDurations[0xFF-bb]})
						} else {
							seq = append(seq, entry{isRest: false, pid: bb})
						}
					}
				} else if b >= 0xEE {
					seq = append(seq, entry{isRest: true, dur: format.RestDurations[b-0xEE]})
				} else {
					seq = append(seq, entry{isRest: false, pid: b - byte(blockCount)})
				}
			}
			for i, e := range seq {
				if e.isRest { continue }
				pid := e.pid
				if pidFollowing[pid] == nil {
					pidFollowing[pid] = make(map[byte]int)
				}
				var followKey byte = 255 // sentinel = note/end
				if i+1 < len(seq) && seq[i+1].isRest {
					followKey = seq[i+1].dur
					restAfterNote[followKey]++
				}
				pidFollowing[pid][followKey]++
			}
		}

		if len(restAfterNote) > 0 {
			durs := []int{}
			for d := range restAfterNote { durs = append(durs, int(d)) }
			sort.Ints(durs)
			fmt.Printf("  Rest duration after note (frames):\n")
			for _, d := range durs {
				fmt.Printf("    %3d: %d\n", d, restAfterNote[byte(d)])
			}
		}

		// Per-pattern analysis for rest absorption (informational only — already applied above)
		target := make(map[byte]bool, len(format.RestDurations))
		for _, d := range format.RestDurations { target[d] = true }
		existingDurs := make(map[byte]bool, len(format.DurationTable))
		for _, d := range format.DurationTable { existingDurs[d] = true }
		type absorbInfo struct{ restDur, count byte; noteDur byte }
		var alwaysAbsorb []absorbInfo
		var sometimesCount int
		durIdx2 := make(map[byte]int, len(format.DurationTable))
		for i, d := range format.DurationTable { durIdx2[d] = i }
		for _, p := range format.NotePatterns {
			if pidFollowing[p.FreqIdx] == nil { continue } // unused pid
		}
		pids := make([]int, len(format.NotePatterns))
		for i := range pids { pids[i] = i }
		sort.Ints(pids)
		for _, pid := range pids {
			following := pidFollowing[byte(pid)]
			if following == nil { continue }
			p := format.NotePatterns[pid]
			restKeys := []byte{}
			for k := range following { if k != 255 && target[k] && existingDurs[p.Duration+k] { restKeys = append(restKeys, k) } }
			if len(restKeys) == 0 { continue }
			hasNonRest := following[255] > 0
			if len(restKeys) == 1 && !hasNonRest {
				rd := restKeys[0]
				alwaysAbsorb = append(alwaysAbsorb, absorbInfo{rd, byte(following[rd]), p.Duration})
			} else {
				for _, k := range restKeys { sometimesCount += following[k] }
			}
		}
		if len(alwaysAbsorb) > 0 || sometimesCount > 0 {
			fmt.Printf("  Rest absorption analysis (all rests):\n")
			totalSaved := 0
			for _, a := range alwaysAbsorb {
				fmt.Printf("    always:    dur=%2d + rest=%2d (%dx) -> new dur=%d, saves %d opcodes\n",
					a.noteDur, a.restDur, a.count, int(a.noteDur)+int(a.restDur), a.count)
				totalSaved += int(a.count)
			}
			if sometimesCount > 0 {
				fmt.Printf("    sometimes: %d rest opcodes need new pattern variants to absorb\n", sometimesCount)
			}
			fmt.Printf("    total free savings: %d opcodes\n", totalSaved)
		}
	}

	// Count envelope mode instruments
	// Show example instruments
	fmt.Printf("\n  Instruments:\n")
	for i, inst := range format.Instruments {
		toggleStr := ""
		if inst.TogglePeriod > 0 {
			toggleStr = fmt.Sprintf(" toggle=%d", inst.TogglePeriod)
		}
		sustainStr := " sustain=none"
		if inst.SustainTableOffset != 0xFF || inst.SustainMinDurIdx != 0 {
			sustainStr = fmt.Sprintf(" sustainOffset=%d", inst.SustainTableOffset)
		}
		fmt.Printf("    %d: wave=0x%02X env=%v npt=%v tail=%d%s%s ADSR=%d/%d/%d\n",
			i, inst.BaseWaveform, inst.EnvelopeMode, inst.NoisePlusTone,
			inst.TailStartFrame, toggleStr, sustainStr,
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

	// Helper: look up sustainIdx for a note pattern via the instrument's sustain table offset.
	// Uses byte arithmetic for the index since offset may wrap (tableStart < minDurIdx).
	patSustainIdx := func(p TrackerPattern) byte {
		if int(p.InstrumentID) >= len(format.Instruments) { return 0xFF }
		ins := format.Instruments[p.InstrumentID]
		if ins.SustainTableOffset == 0xFF && ins.SustainMinDurIdx == 0 { return 0xFF }
		durIdxLocal := byte(0)
		for i, d := range format.DurationTable { if d == p.Duration { durIdxLocal = byte(i); break } }
		idx := int(ins.SustainTableOffset + durIdxLocal)
		if idx < 0 || idx >= len(format.SustainTable) { return 0xFF }
		return format.SustainTable[idx]
	}

	var npFreq, npDur, npInstr, npSustain, npTail []int
	for _, p := range format.NotePatterns {
		if p.FreqIdx != 255 { npFreq = append(npFreq, int(p.FreqIdx)) }
		npDur    = append(npDur,    int(p.Duration))
		npInstr  = append(npInstr,  int(p.InstrumentID))
		si := patSustainIdx(p)
		if si != 0xFF { npSustain = append(npSustain, int(si)) }
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
		si := patSustainIdx(p)
		k := instrDurKey{p.InstrumentID, p.Duration}
		if instrDurToSustain[k] == nil { instrDurToSustain[k] = make(map[byte]int) }
		instrDurToSustain[k][si]++
		if instrToSustain[p.InstrumentID] == nil { instrToSustain[p.InstrumentID] = make(map[byte]int) }
		instrToSustain[p.InstrumentID][si]++
		if durToSustain[p.Duration] == nil { durToSustain[p.Duration] = make(map[byte]int) }
		durToSustain[p.Duration][si]++
	}
	ambigInstrDur, ambigInstr, ambigDur := 0, 0, 0
	for _, m := range instrDurToSustain { if len(m) > 1 { ambigInstrDur++ } }
	for _, m := range instrToSustain    { if len(m) > 1 { ambigInstr++ } }
	for _, m := range durToSustain      { if len(m) > 1 { ambigDur++ } }
	fmt.Printf("  SustainIdx determined by (instr+dur): %d/%d pairs ambiguous\n", ambigInstrDur, len(instrDurToSustain))
	for k, m := range instrDurToSustain {
		if len(m) > 1 {
			sidxs := make([]int, 0, len(m))
			for sidx := range m { sidxs = append(sidxs, int(sidx)) }
			sort.Ints(sidxs)
			parts := make([]string, len(sidxs))
			for i, sidx := range sidxs { parts[i] = fmt.Sprintf("%d(×%d)", sidx, m[byte(sidx)]) }
			fmt.Printf("    instr %2d dur %2d: [%s]\n", k.instr, k.dur, strings.Join(parts, " "))
		}
	}
	fmt.Printf("  SustainIdx determined by instr alone: %d/%d instrs ambiguous\n", ambigInstr, len(instrToSustain))
	fmt.Printf("  SustainIdx determined by dur alone:   %d/%d durs ambiguous\n", ambigDur, len(durToSustain))

	// Per-duration breakdown: which sustain indices does each duration use?
	{
		durs := make([]byte, 0, len(durToSustain))
		for d := range durToSustain { durs = append(durs, d) }
		sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
		fmt.Printf("\n  Per-duration sustain index usage:\n")
		for _, d := range durs {
			m := durToSustain[d]
			sidxs := make([]int, 0, len(m))
			for sidx := range m { sidxs = append(sidxs, int(sidx)) }
			sort.Ints(sidxs)
			parts := make([]string, len(sidxs))
			for i, sidx := range sidxs {
				parts[i] = fmt.Sprintf("%d(×%d)", sidx, m[byte(sidx)])
			}
			uniq := "✓"
			if len(m) > 1 { uniq = "✗" }
			fmt.Printf("    dur %2d: %s  [%s]\n", d, uniq, strings.Join(parts, " "))
		}
	}

	// Per-instrument breakdown: which sustain indices does each instrument use?
	{
		type instrID = byte
		instrs := make([]instrID, 0, len(instrToSustain))
		for id := range instrToSustain { instrs = append(instrs, id) }
		sort.Slice(instrs, func(i, j int) bool { return instrs[i] < instrs[j] })
		fmt.Printf("\n  Per-instrument sustain index usage:\n")
		for _, id := range instrs {
			m := instrToSustain[id]
			sidxs := make([]int, 0, len(m))
			for sidx := range m { sidxs = append(sidxs, int(sidx)) }
			sort.Ints(sidxs)
			parts := make([]string, len(sidxs))
			for i, sidx := range sidxs {
				parts[i] = fmt.Sprintf("%d(×%d)", sidx, m[byte(sidx)])
			}
			uniq := "✓"
			if len(m) > 1 { uniq = "✗" }
			fmt.Printf("    instr %2d: %s  [%s]\n", id, uniq, strings.Join(parts, " "))
		}
	}

	// Check Duration correlation with InstrumentID
	instrToDur := make(map[byte]map[byte]int)
	for _, p := range format.NotePatterns {
		if instrToDur[p.InstrumentID] == nil { instrToDur[p.InstrumentID] = make(map[byte]int) }
		instrToDur[p.InstrumentID][p.Duration]++
	}
	ambigInstrDur2 := 0
	for _, m := range instrToDur { if len(m) > 1 { ambigInstrDur2++ } }
	fmt.Printf("  Duration determined by instr alone:   %d/%d instrs ambiguous\n", ambigInstrDur2, len(instrToDur))

	// Per-pattern usage count (across blocks + streams)
	{
		patUsage := make(map[byte]int, len(format.NotePatterns))
		blockCount := len(format.Blocks)
		// Block content bytes are raw pattern IDs (no offset).
		// Stream opcodes: 0..blockCount-1 = block ref, blockCount..0xED = patternID+blockCount direct ref.
		for ch := 0; ch < 3; ch++ {
			for _, opcode := range format.Streams[ch] {
				if int(opcode) < blockCount {
					for _, pid := range format.Blocks[opcode].Patterns {
						if pid < 0xEE {
							patUsage[pid]++
						}
					}
				} else if opcode < 0xEE {
					patUsage[opcode-byte(blockCount)]++
				}
			}
		}
		// Find drum patterns (instruments with exactly 1 unique freq/FM/dur)
		drumPats := []string{}
		for pid, p := range format.NotePatterns {
			if p.FreqIdx == 255 { continue }
			// Check if this instrument has only 1 unique value across all its patterns
			instrID := p.InstrumentID
			uniqFreq, uniqFM, uniqDur := true, true, true
			for _, q := range format.NotePatterns {
				if q.InstrumentID != instrID || q.FreqIdx == 255 { continue }
				if q.FreqIdx != p.FreqIdx { uniqFreq = false }
				if q.FreqModIdx != p.FreqModIdx { uniqFM = false }
				if q.Duration != p.Duration { uniqDur = false }
			}
			if uniqFreq && uniqFM && uniqDur {
				drumPats = append(drumPats, fmt.Sprintf("pat%d(instr%d, uses×%d)", pid, instrID, patUsage[byte(pid)]))
			}
		}
		fmt.Printf("\n  Drum patterns (const freq+FM+dur per instrument): %s\n", strings.Join(drumPats, "  "))
	}

	// Per-instrument breakdown of freqIdx, freqModIdx, durIdx
	{
		type instrStats struct {
			freqs    map[byte]int
			fmIdxs   map[byte]int
			durs     map[byte]int
		}
		iStats := make(map[byte]*instrStats)
		durIdxMap2 := make(map[byte]byte, len(format.DurationTable))
		for i, d := range format.DurationTable { durIdxMap2[d] = byte(i) }
		for _, p := range format.NotePatterns {
			if p.FreqIdx == 255 { continue }
			id := p.InstrumentID
			if iStats[id] == nil { iStats[id] = &instrStats{make(map[byte]int), make(map[byte]int), make(map[byte]int)} }
			iStats[id].freqs[p.FreqIdx]++
			iStats[id].fmIdxs[p.FreqModIdx]++
			iStats[id].durs[durIdxMap2[p.Duration]]++
		}
		ids2 := make([]byte, 0, len(iStats))
		for id := range iStats { ids2 = append(ids2, id) }
		sort.Slice(ids2, func(i, j int) bool { return ids2[i] < ids2[j] })
		fmt.Printf("\n  Per-instrument pattern fields:\n")
		fmt.Printf("  %-8s %-6s %-6s %-6s  freqs  fmIdxs  durIdxs\n", "instr", "#pats", "uFreq", "uFM")
		for _, id := range ids2 {
			s := iStats[id]
			freqList := make([]int, 0, len(s.freqs))
			for f := range s.freqs { freqList = append(freqList, int(f)) }
			sort.Ints(freqList)
			fmList := make([]int, 0, len(s.fmIdxs))
			for f := range s.fmIdxs { fmList = append(fmList, int(f)) }
			sort.Ints(fmList)
			durList := make([]int, 0, len(s.durs))
			for d := range s.durs { durList = append(durList, int(d)) }
			sort.Ints(durList)
			totalPats := 0
			for _, c := range s.freqs { totalPats += c }
			fmStrs := make([]string, len(fmList))
			for i, f := range fmList {
				if f == 255 { fmStrs[i] = "none" } else { fmStrs[i] = fmt.Sprintf("%d(×%d)", f, s.fmIdxs[byte(f)]) }
			}
			durStrs := make([]string, len(durList))
			for i, d := range durList { durStrs[i] = fmt.Sprintf("%d(×%d)", d, s.durs[byte(d)]) }
			freqStr := fmt.Sprintf("%d values", len(freqList))
			if len(freqList) == 1 { freqStr = fmt.Sprintf("const=%d", freqList[0]) }
			fmt.Printf("  instr %2d  %4d   %2d    %2d    %-20s  [%s]  [%s]\n",
				id, totalPats, len(freqList), len(s.fmIdxs), freqStr,
				strings.Join(fmStrs, " "), strings.Join(durStrs, " "))
		}
	}

	// Direct-indexed sustain table: sparseTable[instr.tableStart + durIdx] = sustainIdx
	// Each instrument gets a contiguous slice from durIdx=0..maxDurIdx, 0xFF for unused slots.
	// After splitting instr 2 into two instruments (one per sustain of the ambiguous dur-5 pair).
	{
		// Build durIdx map
		durIdxMap := make(map[byte]byte, len(format.DurationTable))
		for i, d := range format.DurationTable { durIdxMap[d] = byte(i) }

		// Collect unique instruments after split (instr 2 → instr 2a and 2b, each with 1 dur-5 entry)
		type instrEntry struct {
			id      byte
			durSust map[byte]byte // durIdx → sustainIdx
		}
		instrMap := make(map[byte]*instrEntry)
		splitID := byte(11) // new instrument gets id 11
		for _, p := range format.NotePatterns {
			id := p.InstrumentID
			di := durIdxMap[p.Duration]
			si := patSustainIdx(p)
			if instrMap[id] == nil {
				instrMap[id] = &instrEntry{id: id, durSust: make(map[byte]byte)}
			}
			if existing, ok := instrMap[id].durSust[di]; ok && existing != si {
				// ambiguous: split off new instrument for this (durIdx, sustainIdx) pair
				if instrMap[splitID] == nil {
					instrMap[splitID] = &instrEntry{id: splitID, durSust: make(map[byte]byte)}
				}
				instrMap[splitID].durSust[di] = si
			} else {
				instrMap[id].durSust[di] = si
			}
		}

		ids := make([]byte, 0, len(instrMap))
		for id := range instrMap { ids = append(ids, id) }
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

		totalTableBytes := 0
		fmt.Printf("\n  Direct-indexed sustain table (sustainTable[instr.offset+durIdx], offset=tableStart-minDurIdx):\n")
		for _, id := range ids {
			e := instrMap[id]
			minDurIdx, maxDurIdx := byte(255), byte(0)
			for di := range e.durSust {
				if di < minDurIdx { minDurIdx = di }
				if di > maxDurIdx { maxDurIdx = di }
			}
			sliceLen := int(maxDurIdx) - int(minDurIdx) + 1
			totalTableBytes += sliceLen
			durs := make([]int, 0, len(e.durSust))
			for di := range e.durSust { durs = append(durs, int(di)) }
			sort.Ints(durs)
			parts := make([]string, len(durs))
			for i, di := range durs { parts[i] = fmt.Sprintf("di%d→s%d", di, e.durSust[byte(di)]) }
			fmt.Printf("    instr %2d: durIdx=%d..%d → slice=%d bytes  [%s]\n",
				id, minDurIdx, maxDurIdx, sliceLen, strings.Join(parts, " "))
		}

		fmt.Printf("    Total table: %d bytes\n", totalTableBytes)
	}

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
		for _, v := range p.Values { spVal = append(spVal, int(v)) }
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

	// --- Dead entry reachability check ---
	// Trace reachability from streams upward through blocks → note patterns → instruments/freqs/freqMods.
	fmt.Printf("\n=== Dead Entry Check ===\n")
	{
		blockCount := len(format.Blocks)
		usedBlocks   := make(map[int]bool)
		usedPatterns := make(map[int]bool)
		usedRestDurs := make(map[int]bool)

		// Build duration → index map for rest durations
		restDurIdx := make(map[byte]int, len(format.RestDurations))
		for i, d := range format.RestDurations { restDurIdx[d] = i }

		durIdx := make(map[byte]int, len(format.DurationTable))
		for i, d := range format.DurationTable { durIdx[d] = i }

		processEntry := func(b byte) {
			if int(b) < blockCount {
				usedBlocks[int(b)] = true
			} else if b >= 0xEE {
				usedRestDurs[int(b-0xEE)] = true
			} else {
				usedPatterns[int(b)-blockCount] = true
			}
		}
		for ch := 0; ch < 3; ch++ {
			for _, b := range format.Streams[ch] { processEntry(b) }
		}
		// Expand blocks: entries inside a used block also add patterns/rest durs
		for bi := range usedBlocks {
			for _, b := range format.Blocks[bi].Patterns {
				if b >= 0xEE {
					usedRestDurs[int(0xFF-b)] = true
				} else {
					usedPatterns[int(b)] = true
				}
			}
		}

		// From used note patterns, gather used instruments, freqs, freqMods, durs, sustain patterns
		usedInstrs   := make(map[int]bool)
		usedFreqs    := make(map[int]bool)
		usedFreqMods := make(map[int]bool)
		usedDurIdxs  := make(map[int]bool)
		usedSustains := make(map[int]bool)
		for pid := range usedPatterns {
			if pid >= len(format.NotePatterns) { continue }
			p := format.NotePatterns[pid]
			if p.FreqIdx != 0xFF { usedFreqs[int(p.FreqIdx)] = true }
			usedInstrs[int(p.InstrumentID)] = true
			if di, ok := durIdx[p.Duration]; ok { usedDurIdxs[di] = true }
			if p.FreqModIdx != 0xFF { usedFreqMods[int(p.FreqModIdx)] = true }
			// Sustain pattern reachable via instrument sustain table.
			// offset may wrap as byte (when tableStart < minDurIdx), so use byte arithmetic.
			if int(p.InstrumentID) < len(format.Instruments) {
				ins := format.Instruments[p.InstrumentID]
				if ins.SustainTableOffset != 0xFF || ins.SustainMinDurIdx != 0 {
					if di, ok := durIdx[p.Duration]; ok {
						idx := int(ins.SustainTableOffset + byte(di))
						if idx >= 0 && idx < len(format.SustainTable) {
							si := int(format.SustainTable[idx])
							if si != 0xFF { usedSustains[si] = true }
						}
					}
				}
			}
		}
		dead := false
		check := func(label string, used map[int]bool, total int) {
			var deadList []int
			for i := 0; i < total; i++ {
				if !used[i] { deadList = append(deadList, i) }
			}
			if len(deadList) > 0 {
				fmt.Printf("  DEAD %-20s %v\n", label+":", deadList)
				dead = true
			} else {
				fmt.Printf("  ✓ %-20s all %d used\n", label, total)
			}
		}
		check("blocks",        usedBlocks,   len(format.Blocks))
		check("note patterns", usedPatterns, len(format.NotePatterns))
		check("instruments",   usedInstrs,   len(format.Instruments))
		check("freqs",         usedFreqs,    len(format.FreqTable))
		check("freqMods",      usedFreqMods, len(format.FreqModPatterns))
		check("dur table",     usedDurIdxs,  len(format.DurationTable))
		check("sustain pats",  usedSustains, len(format.SustainPatterns))

		// Rest durations: used set is indexed by (opcode - 0xEE) from streams, or (0xFF - b) from blocks
		usedRestDurBytes := make(map[int]bool)
		for idx := range usedRestDurs {
			if idx < len(format.RestDurations) { usedRestDurBytes[idx] = true }
		}
		check("rest durs",     usedRestDurBytes, len(format.RestDurations))

		if dead {
			panic("dead entries found — remove them before serializing")
		}
		fmt.Printf("  No dead entries found\n")
	}

	// Serialize and validate
	sectionNames := [17]string{
		"freqTable", "toggleValues", "toggleIdx", "freqMod", "toneSeqs",
		"delayedVibrato", "sustainPatterns", "instruments", "sustainTable",
		"durationTable", "restDurations", "notePatterns", "toneOverrides",
		"blocks", "streamA", "streamB", "streamC",
	}
	rom := serializeTrackerFormat(format)
	fmt.Printf("  Section offsets:\n")
	for i := 0; i < 17; i++ {
		off := uint16(rom[i*2]) | uint16(rom[i*2+1])<<8
		fmt.Printf("    %2d %-18s 0x%04X (%d)\n", i, sectionNames[i], off, off)
	}
	if len(rom) != size {
		panic(fmt.Sprintf("ROM size mismatch: calculated %d bytes but serialized %d bytes", size, len(rom)))
	}
	if err := os.WriteFile("tracker_format.rom", rom, 0644); err != nil {
		panic(fmt.Sprintf("failed to write ROM: %v", err))
	}
	fmt.Printf("  Written to tracker_format.rom (%d bytes)\n", len(rom))
}

type sustainException struct{ instrID, durIdx, sustainIdx byte }

// sustainDefaultsAndExceptions derives a per-instrument default sustainIdx and a small exception
// list from the flat SustainTable. The default is the most common sustainIdx for each instrument
// (0xFF if no entries). Exceptions cover the minority durIdx values that differ from the default.
func sustainDefaultsAndExceptions(instruments []TrackerInstrument, sustainTable []byte) ([]byte, []sustainException) {
	defaults := make([]byte, len(instruments))
	var exceptions []sustainException
	for id, ins := range instruments {
		if ins.SustainTableOffset == 0xFF {
			defaults[id] = 0xFF
			continue
		}
		counts := make(map[byte]int)
		for di := ins.SustainMinDurIdx; di <= ins.SustainMaxDurIdx; di++ {
			idx := int(ins.SustainTableOffset + di)
			if idx < 0 || idx >= len(sustainTable) { break }
			si := sustainTable[idx]
			if si != 0xFF { counts[si]++ }
		}
		if len(counts) == 0 {
			defaults[id] = 0xFF
			continue
		}
		best, bestCount := byte(0xFF), 0
		for si, c := range counts {
			if c > bestCount { best, bestCount = si, c }
		}
		defaults[id] = best
		for di := ins.SustainMinDurIdx; di <= ins.SustainMaxDurIdx; di++ {
			idx := int(ins.SustainTableOffset + di)
			if idx < 0 || idx >= len(sustainTable) { break }
			si := sustainTable[idx]
			if si == 0xFF || si == best { continue }
			exceptions = append(exceptions, sustainException{byte(id), di, si})
		}
	}
	return defaults, exceptions
}

type toneSeqStep struct {
	toggleIdx byte // only used for step[0]: index into toggleValues
	delta     byte // delta added to current period (steps[1:])
	hold      byte // frames to hold this value
}

func toneSeqDeltaSteps(freqs []uint16, toggleValues []uint16) []toneSeqStep {
	toggleIdx := byte(0)
	for i, v := range toggleValues {
		if v == freqs[0] {
			toggleIdx = byte(i)
			break
		}
	}
	steps := []toneSeqStep{{toggleIdx: toggleIdx, hold: 0}}
	prev := freqs[0]
	run := 1
	for i := 1; i < len(freqs); i++ {
		if freqs[i] == prev {
			run++
		} else {
			steps[len(steps)-1].hold = byte(run)
			delta := freqs[i] - prev
			if delta > 255 {
				panic(fmt.Sprintf("toneSeq delta %d exceeds u8 at index %d", delta, i))
			}
			steps = append(steps, toneSeqStep{delta: byte(delta), hold: 1})
			prev = freqs[i]
			run = 1
		}
	}
	steps[len(steps)-1].hold = byte(run)
	return steps
}

func serializeTrackerFormat(format *TrackerFormat) []byte {
	// Header: 16 × uint16 LE section offsets (bytes 0–31) + freqModCount 1 byte (byte 32) + 1 pad = 34 bytes.
	// Section order:
	//  0: freqTable  1: toggleValues  2: toggleIdx  3: freqMod  4: toneSeqs
	//  5: delayedVibrato  6: sustainPatterns  7: instruments  8: sustainTable
	//  9: durationTable  10: restDurations  11: notePatterns  12: toneOverrides
	//  13: blocks  14: streamA  15: streamB  16: streamC
	//
	// vs prior layout: seqTemplates section removed; drum templates inlined into type-5 FreqMod records.
	// toneSeqs section (new) holds only the tone-period sequences for envelope note overrides.
	// toneOverrides retained (only 1 entry, references toneSeqs[0]).
	const numSections = 17
	const headerSize = numSections*2 + 2 // 34 bytes offsets + freqModCount(1) + pad(1)

	w := []byte{}
	pu16 := func(v uint16) { w = append(w, byte(v), byte(v>>8)) }
	pb  := func(v byte)   { w = append(w, v) }
	pbs := func(v []byte) { w = append(w, v...) }

	sectionPos := [numSections]int{}
	mark := func(i int) { sectionPos[i] = len(w) }

	// Freq table: freq entries (count from section size / 2)
	mark(0)
	for _, f := range format.FreqTable {
		pu16(f)
	}
	// Toggle period values: uint16 entries (count from section size / 2)
	mark(1)
	for _, v := range format.TogglePeriodValues {
		pu16(v)
	}
	// Toggle period nibble-packed indices
	mark(2)
	pbs(format.TogglePeriodIdx)

	// FreqMod patterns: (compactType<<4|dataLen)(1) + data each (count from section scan)
	// Type 5 (SequenceLookup): data is now inline freq sequence: length(1) + freq×2
	//   Previously type 5 stored a 1-byte template index; seqTemplates section is removed.
	// compactType: 5→0, 7→1, 8→2, 11→3, 13→4
	mark(3)
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
	// For type 5 (SequenceLookup), the 4-bit dataLen nibble is repurposed as the
	// template entry count (1–15), followed by freq×2 inline. The scanner reads
	// dataLen = count, then skips count*2 bytes.
	// For all other types, dataLen is the raw byte count of Data as before.
	for _, p := range format.FreqModPatterns {
		if p.Type == 5 {
			tmplIdx := int(p.Data[0])
			tmpl := format.FreqSequenceTemplates[tmplIdx]
			count := byte(len(tmpl.Frequencies))
			pb(compactType(5)<<4 | count) // nibble = entry count
			for _, f := range tmpl.Frequencies {
				pu16(f)
			}
		} else {
			dataLen := byte(len(p.Data))
			pb(compactType(p.Type)<<4 | dataLen)
			pbs(p.Data)
		}
	}

	// Tone sequences: (length(1) + period×2 each) for envelope note tone overrides only.
	// Drum templates are inlined into their type-5 FreqMod records; this section holds
	// only the sequences referenced by toneOverrides entries.
	mark(4)
	toneSeqRemap := make(map[int]byte) // old tmplIdx → new toneSeq index
	for _, o := range format.ToneSeqOverrides {
		oldIdx := int(o.TemplateIdx)
		if _, exists := toneSeqRemap[oldIdx]; !exists {
			newIdx := byte(len(toneSeqRemap))
			toneSeqRemap[oldIdx] = newIdx
			tmpl := format.FreqSequenceTemplates[oldIdx]
			steps := toneSeqDeltaSteps(tmpl.Frequencies, format.TogglePeriodValues)
			pb(steps[0].toggleIdx) // initial period as index into toggleValues table
			for _, s := range steps {
				pb(s.delta)
				pb(s.hold)
			}
		}
	}

	// Delayed vibrato templates: (4 × int16 V params, bell only) (count = sectionSize / 8)
	mark(5)
	// All templates in this song are bell shape: Deltas=[v0,v1,v0,0,-v2,-v3,-v2,0]
	for _, t := range format.DelayedVibratoTemplates {
		if t.Shape != 0 {
			panic(fmt.Sprintf("unexpected DV template shape %d (only bell=0 supported)", t.Shape))
		}
		for i := 0; i < 4; i++ {
			pu16(uint16(t.V[i]))
		}
	}

	// Sustain patterns: (header(1) + values) (count from section scan)
	// header = (startValue_idx<<6) | valLen  where startValue_idx indexes sustainStartValues[]
	// valLen=0 means constant: startValue repeated for entire note duration.
	// startValue must be one of sustainStartValues (panic otherwise).
	sustainStartValues := [4]byte{10, 12, 14, 15}
	sustainStartIdx := func(v byte) byte {
		for i, sv := range sustainStartValues {
			if sv == v { return byte(i) }
		}
		panic(fmt.Sprintf("sustain StartValue %d not in table %v", v, sustainStartValues))
	}
	mark(6)
	for _, sp := range format.SustainPatterns {
		pb(sustainStartIdx(sp.StartValue)<<6 | byte(len(sp.Values)))
		pbs(sp.Values)
	}

	// Instruments: 4 bytes each: merged(1), DS(1), tail(1), sustainOffset(1)
	// merged = (waveCompact<<5) | flags3   waveCompact: 1→0 2→1 4→2 8→3 (2 bits, bits 6–5)
	//                                      flags3: bit0=EnvelopeMode bit1=NoisePlusTone bit2=Attack≠0
	// DS     = (decayBit<<3) | (sustainIdx<<1) | attackBit
	//   decayBit: 0→D=0, 1→D=9   (only 2 values in this song)
	//   sustainIdx: 2-bit index into sustainStartValues[] (same table as sustain pattern headers)
	//   attackBit: 0→A=0, 1→A=1
	// TogglePeriod is not stored — player reads it from the freq table (sections 1/2).
	sustainDefaults, sustainExceptions2 := sustainDefaultsAndExceptions(format.Instruments, format.SustainTable)
	instrIdx := 0
	mark(7)
	waveCompact := func(w byte) byte {
		switch w {
		case 1:  return 0
		case 2:  return 1
		case 4:  return 2
		case 8:  return 3
		default: panic(fmt.Sprintf("unexpected BaseWaveform 0x%02X", w))
		}
	}
	decayBit := func(d byte) byte {
		switch d {
		case 0:  return 0
		case 9:  return 1
		default: panic(fmt.Sprintf("unexpected Decay %d (only 0 or 9)", d))
		}
	}
	for instrIdx < len(format.Instruments) {
		ins := format.Instruments[instrIdx]
		flags := byte(0)
		if ins.EnvelopeMode  { flags |= 1 }
		if ins.NoisePlusTone { flags |= 2 }
		if ins.ADSR.Attack != 0 { flags |= 4 }
		pb(waveCompact(ins.BaseWaveform)<<5 | flags)
		pb(decayBit(ins.ADSR.Decay)<<3 | sustainStartIdx(ins.ADSR.Sustain)<<1 | (flags>>2)&1)
		pb(ins.TailStartFrame)
		pb(sustainDefaults[instrIdx])
		instrIdx++
	}

	// Sustain exceptions: count(1) + exceptions×(instrID:u8, durIdx:u8, sustainIdx:u8)
	// The default sustainIdx per instrument is stored in the instrument record (byte 4).
	mark(8)
	pb(byte(len(sustainExceptions2)))
	for _, e := range sustainExceptions2 {
		pb(e.instrID)
		pb(e.durIdx)
		pb(e.sustainIdx)
	}

	// Duration table (sec 9): 1 byte per entry, sequential ascending.
	durToIdx := make(map[byte]byte, len(format.DurationTable))
	for i, d := range format.DurationTable { durToIdx[d] = byte(i) }
	mark(9)
	pbs(format.DurationTable)

	// Rest durations (sec 10): 1 byte per entry, sequential ascending.
	restToIdx := make(map[byte]byte, len(format.RestDurations))
	for i, d := range format.RestDurations { restToIdx[d] = byte(i) }
	mark(10)
	pbs(format.RestDurations)

	// Note patterns: SoA layout.
	mark(11)
	// 3 separate arrays: freqIdx[0..n-1], packed[0..n-1], freqModIdx[0..fm-1]
	// notePatCount = (sectionSize - freqModCount) / 2  (freqModCount from header byte 34)
	freqModCount := 0
	for _, p := range format.NotePatterns {
		if p.FreqModIdx != 0xFF { freqModCount++ }
	}
	for _, p := range format.NotePatterns { pb(p.FreqIdx) }
	for _, p := range format.NotePatterns { pb((p.InstrumentID << 4) | durToIdx[p.Duration]) }
	for i, p := range format.NotePatterns {
		if i >= freqModCount { break }
		pb(p.FreqModIdx)
	}

	// Tone seq overrides: 2 bytes each: patternIdx, toneSeqIdx (count = sectionSize / 2)
	// toneSeqIdx indexes into toneSeqs section (section 4), not the old freqSequenceTemplates.
	mark(12)
	for _, o := range format.ToneSeqOverrides {
		pb(o.PatternIdx)
		pb(toneSeqRemap[int(o.TemplateIdx)])
	}

	// Blocks: (length(1) + patternIDs each) (count from section scan)
	mark(13)
	for _, block := range format.Blocks {
		pb(byte(len(block.Patterns)))
		pbs(block.Patterns)
	}

	// Streams: raw bytes (length = sectionSize from offset table)
	for ch := 0; ch < 3; ch++ {
		mark(14 + ch)
		pbs(format.Streams[ch])
	}

	// Prepend header: 17 × uint16 LE section offsets + freqModCount (1 byte) + 1 pad byte = 36 bytes.
	header := make([]byte, headerSize)
	for i, pos := range sectionPos {
		off := uint16(headerSize + pos)
		header[i*2] = byte(off)
		header[i*2+1] = byte(off >> 8)
	}
	header[34] = byte(freqModCount)
	// header[35] = 0 (pad, already zero from make)
	return append(header, w...)
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

// buildFreqModPatterns6502 creates 6502-friendly frequency modulation patterns
// Focus: byte operations, table lookups, simple loops - no multiply/divide
func buildFreqModPatterns6502(notes []NoteWithMetadata, freqTable []uint16) ([]FreqModPattern, map[string]int, []DelayedVibratoTemplate) {
	freqModMap := make(map[string]int)
	patterns := []FreqModPattern{}

	// addPattern deduplicates by type-specific key and also registers a legacy delta key.
	addPattern := func(p FreqModPattern, key, legacyKey string) {
		if _, exists := freqModMap[key]; !exists {
			idx := len(patterns)
			freqModMap[key] = idx
			freqModMap[legacyKey] = idx
			patterns = append(patterns, p)
		} else if _, exists := freqModMap[legacyKey]; !exists {
			freqModMap[legacyKey] = freqModMap[key]
		}
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
			addPattern(pattern, key, freqSeqKey(freqs))
			continue
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
			addPattern(pattern, key, freqSeqKey(freqs))
			continue
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
			addPattern(pattern, key, freqSeqKey(freqs))
			continue
		}

		// Classify pattern (only types 11 and 13 occur in this song)
		if classified, fmType, fmData := classifyFreqModPattern(freqs[0], deltas); classified {
			pattern = FreqModPattern{
				Type:   fmType,
				Delay:  delay,
				Length: byte(len(freqs)),
				Data:   fmData,
			}
			key = freqSeqKey(freqs)
		} else {
			panic(fmt.Sprintf("unclassified freq pattern: startFreq=%d deltas=%v", freqs[0], deltas))
		}
		addPattern(pattern, key, key) // key == legacyKey for classified patterns
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
