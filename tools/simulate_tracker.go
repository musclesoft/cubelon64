// Tracker replay simulation - generates SID register writes from tracker format
package main

import (
	"fmt"
)

// SimulatedNote represents the SID register state for one note
//
// ARCHITECTURE: This contains ALL data the tracker player needs for playback.
// Each field is populated by the simulator from the tracker format.
//
// FREQUENCY DATA (freqSeq):
//   - Contains final SID frequency register values (ready to write)
//   - Extractor converts AY periods to SID frequencies
//   - Player just writes: SID[freq_lo] = freqSeq[frame] & 0xFF
//   - May include glissando/vibrato patterns
//
// WAVEFORM DATA (waveformSeq):
//   - Envelope mode: contains waveform changes from R13 (envelope shape) register
//   - NoisePlusTone mode: contains waveform changes from R7 (mixer) register
//   - Each entry is 0x01 (triangle), 0x02 (sawtooth), 0x04 (pulse), or 0x08 (noise)
//
// TODO: Extractor must populate waveformSeq from R13 and R7 register changes
type SimulatedNote struct {
	freq              uint16
	freqSeq           []uint16 // Per-frame frequencies (AY periods: envelope/noise/tone)
	Waveform          byte     // Initial waveform for note
	waveformSeq       []byte   // Per-frame waveforms (from R13/mixer changes)
	dur               uint16
	ADSR              ADSRParams
	SustainPerFrame   []uint8  // Dynamic S parameter per frame (volume-following)
	TailStartFrame      int      // Frame to switch D from 0 to ADSR.Decay (0 = no tail decay)
	TogglePeriod      uint16   // SID cycles for waveform toggle (constant, from freq table)
	TogglePeriodSeq   []uint16 // Per-frame toggle periods (for tone glissando, 0 = use constant)
	EnvelopeMode      bool
	NoisePlusTone     bool // If true, note uses noise+tone mixing (waveform switches based on mixer)
	isRest            bool // True for silence/rest patterns
}

// SimulateTracker replays the tracker format and generates notes per channel
//
// ARCHITECTURE: This is the DECODER/SIMULATOR - it unpacks the compressed tracker format
// into expanded SimulatedNote structures that the player can easily process.
//
// RESPONSIBILITIES:
// - Decode bytecode streams (blocks + direct patterns) into note sequences
// - Expand patterns into full SimulatedNote structures with all per-frame data
// - Load frequency sequences, waveform sequences, sustain patterns from the format
// - Apply vibrato/glissando patterns to generate per-frame frequencies
//
// OUTPUT: SimulatedNote arrays containing ALL playback data (no AY commands needed)
func SimulateTracker(format *TrackerFormat) [3][]SimulatedNote {
	return SimulateTrackerFromROM(serializeTrackerFormat(format))
}

// SimulateTrackerFromROM replays the tracker format from raw ROM bytes.
// All reads use simple byte/uint16 memory ops to mirror 6502 execution.
func SimulateTrackerFromROM(rom []byte) [3][]SimulatedNote {
	fmt.Printf("\n=== Simulating Tracker Replay ===\n")

	// --- ROM cursor helpers ---
	pos := 0
	rb := func() byte { v := rom[pos]; pos++; return v }

	// --- Parse freq table ---
	freqTableBase := pos
	freqCount := int(rb())
	freqTableDataBase := pos // base of freq[0] lo byte
	pos += freqCount * 2    // skip freq entries

	// --- Parse toggle period values ---
	toggleValBase := pos
	toggleValCount := int(rb())
	toggleValDataBase := pos
	pos += toggleValCount * 2

	// --- Parse toggle period nibble-packed indices ---
	toggleIdxBase := pos
	pos += (freqCount + 1) / 2

	// --- Parse toggle period sequences ---
	toggleSeqBase := pos
	toggleSeqCount := int(rb())
	toggleSeqDataBase := pos
	pos += toggleSeqCount * 4

	// --- Parse freqmod patterns ---
	freqModBase := pos
	freqModCount := int(rb())
	freqModOffsets := make([]int, freqModCount)
	for i := 0; i < freqModCount; i++ {
		freqModOffsets[i] = pos
		pos++        // type
		pos++        // patternLength
		dataLen := int(rb()) // dataLen byte
		pos += dataLen
	}

	// --- Parse freq sequence templates ---
	seqTemplateBase := pos
	seqTemplateCount := int(rb())
	seqTemplateOffsets := make([]int, seqTemplateCount)
	for i := 0; i < seqTemplateCount; i++ {
		seqTemplateOffsets[i] = pos
		n := int(rb())
		pos += n * 2
	}

	// --- Parse delayed vibrato templates ---
	dvBase := pos
	dvCount := int(rb())
	dvOffsets := make([]int, dvCount)
	for i := 0; i < dvCount; i++ {
		dvOffsets[i] = pos
		shape := rb()
		switch shape {
		case 0, 1:
			pos += 8 // 4 × int16
		default:
			pos += 16 // 8 × int16
		}
	}

	// --- Parse sustain patterns ---
	sustainBase := pos
	sustainCount := int(rb())
	sustainOffsets := make([]int, sustainCount)
	sustainValBase := make([]int, sustainCount) // base of Values[] for non-const non-prefix
	sustainValEnd  := make([]int, sustainCount) // exclusive end
	for i := 0; i < sustainCount; i++ {
		sustainOffsets[i] = pos
		flags := rb()
		isConstant := flags&1 != 0
		isPrefix   := flags&4 != 0
		if isPrefix {
			pos++ // base_idx
		} else {
			pos++ // startValue
			if !isConstant {
				valLen := int(rb()) // valuesLen byte
				sustainValBase[i] = pos
				sustainValEnd[i] = pos + valLen
				pos += valLen
			}
		}
	}

	// --- Parse waveform patterns ---
	waveformBase := pos
	waveformCount := int(rb())
	waveformOffsets := make([]int, waveformCount)
	for i := 0; i < waveformCount; i++ {
		waveformOffsets[i] = pos
		n := int(rb())
		pos += n
	}

	// --- Parse instruments ---
	instrumentBase := pos
	instrumentCount := int(rb())
	instrumentDataBase := pos // base of instr[0] wave byte
	pos += instrumentCount * 8

	// --- Parse duration table ---
	durTableBase := pos
	durTableCount := int(rb())
	durTableDataBase := pos
	pos += durTableCount

	// --- Parse rest durations ---
	restDurBase := pos
	restDurCount := int(rb())
	restDurDataBase := pos
	pos += restDurCount

	// --- Parse note patterns ---
	notePatBase := pos
	notePatCount := int(rb())
	notePatDataBase := pos // base of pattern[0] byte 0
	pos += notePatCount * 3

	// --- Parse override tables (freqSeq, waveform, toggle) ---
	freqSeqOvBase := pos
	freqSeqOvCount := int(rb())
	freqSeqOvDataBase := pos
	pos += freqSeqOvCount * 2

	waveOvBase := pos
	waveOvCount := int(rb())
	waveOvDataBase := pos
	pos += waveOvCount * 2

	toggleOvBase := pos
	toggleOvCount := int(rb())
	toggleOvDataBase := pos
	pos += toggleOvCount * 2

	// --- Parse blocks ---
	blockBase := pos
	blockCount := int(rb())
	blockOffsets := make([]int, blockCount)
	for i := 0; i < blockCount; i++ {
		blockOffsets[i] = pos
		n := int(rb())
		pos += n
	}

	// --- Parse streams ---
	streamBase := pos
	streamOffsets := [3]int{}
	streamLens := [3]int{}
	for ch := 0; ch < 3; ch++ {
		lo := int(rb()); hi := int(rb())
		n := lo | hi<<8
		streamOffsets[ch] = pos  // points to first opcode byte, after the 2-byte length
		streamLens[ch] = n
		pos += n
	}

	// Suppress unused-variable warnings for section bases not directly indexed
	_ = freqTableBase; _ = toggleValBase; _ = toggleSeqBase; _ = freqModBase
	_ = seqTemplateBase; _ = dvBase; _ = sustainBase; _ = waveformBase
	_ = instrumentBase; _ = durTableBase; _ = restDurBase; _ = notePatBase
	_ = freqSeqOvBase; _ = waveOvBase; _ = toggleOvBase; _ = blockBase; _ = streamBase

	// --- Inline helpers that read from ROM by index ---

	// freqAt returns the SID frequency for freq-table entry i
	freqAt := func(i byte) uint16 {
		base := freqTableDataBase + int(i)*2
		return uint16(rom[base]) | uint16(rom[base+1])<<8
	}

	// togglePeriodFor returns the constant toggle period for freq-table entry freqIdx
	togglePeriodFor := func(freqIdx byte) uint16 {
		nibbleByte := rom[toggleIdxBase+int(freqIdx)/2]
		nibble := (nibbleByte >> (freqIdx % 2 * 4)) & 0x0F
		base := toggleValDataBase + int(nibble)*2
		return uint16(rom[base]) | uint16(rom[base+1])<<8
	}

	// instrAt reads the 8-byte instrument block for instrument i
	type instrData struct {
		wave, flags byte
		togglePeriod uint16
		tail, attack, decay, sustain byte
	}
	instrAt := func(i byte) instrData {
		base := instrumentDataBase + int(i)*8
		return instrData{
			wave:         rom[base],
			flags:        rom[base+1],
			togglePeriod: uint16(rom[base+2]) | uint16(rom[base+3])<<8,
			tail:         rom[base+4],
			attack:       rom[base+5],
			decay:        rom[base+6],
			sustain:      rom[base+7],
		}
	}

	// durAt returns the note duration in frames for dur-table index i
	durAt := func(i byte) uint16 {
		return uint16(rom[durTableDataBase+int(i)])
	}

	// restDurAt returns the rest duration for rest-dur-table index i
	restDurAt := func(i byte) uint16 {
		return uint16(rom[restDurDataBase+int(i)])
	}

	// notePatAt reads the 3-byte note pattern: (freqIdx, instrNibble|durIdx, sustainIdx)
	type notePatData struct {
		freqIdx, instrID, durIdx, sustainIdx byte
	}
	notePatAt := func(pid byte) notePatData {
		base := notePatDataBase + int(pid)*3
		packed := rom[base+1]
		return notePatData{
			freqIdx:    rom[base],
			instrID:    packed >> 4,
			durIdx:     packed & 0x0F,
			sustainIdx: rom[base+2],
		}
	}

	// ovLookup scans an override table for patternID, returns (seqIndex, found)
	ovLookup := func(dataBase, count int, patternID byte) (byte, bool) {
		for i := 0; i < count; i++ {
			if rom[dataBase+i*2] == patternID {
				return rom[dataBase+i*2+1], true
			}
		}
		return 0, false
	}

	// toggleSeqAt returns start/end periods for glissando sequence i
	toggleSeqAt := func(i byte) (start, end uint16) {
		base := toggleSeqDataBase + int(i)*4
		return uint16(rom[base]) | uint16(rom[base+1])<<8,
			uint16(rom[base+2]) | uint16(rom[base+3])<<8
	}

	// waveformAt returns the waveform bytes slice for pattern i
	waveformAt := func(i byte) []byte {
		base := waveformOffsets[i]
		n := int(rom[base])
		return rom[base+1 : base+1+n]
	}

	// freqModAt reads a freqmod pattern from ROM and reconstructs the FreqModPattern struct
	freqModAt := func(i byte) FreqModPattern {
		base := freqModOffsets[i]
		fmType := rom[base]
		fmLen := rom[base+1]
		dataLen := int(rom[base+2])
		return FreqModPattern{
			Type:   fmType,
			Length: fmLen,
			Data:   rom[base+3 : base+3+dataLen],
		}
	}

	// seqTemplateAt reconstructs a FreqSequenceTemplate from ROM
	seqTemplateAt := func(i byte) FreqSequenceTemplate {
		base := seqTemplateOffsets[i]
		n := int(rom[base])
		freqs := make([]uint16, n)
		for j := 0; j < n; j++ {
			b := base + 1 + j*2
			freqs[j] = uint16(rom[b]) | uint16(rom[b+1])<<8
		}
		return FreqSequenceTemplate{Frequencies: freqs}
	}

	// dvAt reconstructs a DelayedVibratoTemplate from ROM
	dvAt := func(i byte) DelayedVibratoTemplate {
		base := dvOffsets[i]
		shape := rom[base]
		var t DelayedVibratoTemplate
		t.Shape = shape
		switch shape {
		case 0, 1:
			for j := 0; j < 4; j++ {
				b := base + 1 + j*2
				t.V[j] = int16(uint16(rom[b]) | uint16(rom[b+1])<<8)
			}
		default:
			for j := 0; j < 8; j++ {
				b := base + 1 + j*2
				t.Deltas[j] = int16(uint16(rom[b]) | uint16(rom[b+1])<<8)
			}
		}
		return t
	}

	// expandSustain populates note.SustainPerFrame from ROM sustain pattern i
	expandSustain := func(note *SimulatedNote, sustainIdx byte) {
		if sustainIdx == 0xFF || int(sustainIdx) >= sustainCount {
			return
		}
		base := sustainOffsets[sustainIdx]
		flags := rom[base]
		isConstant := flags&1 != 0
		isDelta    := flags&2 != 0
		isPrefix   := flags&4 != 0
		dur := note.dur
		note.SustainPerFrame = make([]uint8, dur)

		if isPrefix {
			baseIdx := rom[base+1]
			vBase := sustainValBase[baseIdx]
			for i := uint16(0); i < dur; i++ {
				note.SustainPerFrame[i] = rom[vBase+int(i)]
			}
		} else if isConstant {
			startVal := rom[base+1]
			for i := uint16(0); i < dur; i++ {
				note.SustainPerFrame[i] = startVal
			}
		} else if isDelta {
			startVal := rom[base+1]
			current := startVal
			note.SustainPerFrame[0] = current
			outIdx := 1
			vBase := sustainValBase[sustainIdx]
			vEnd  := sustainValEnd[sustainIdx]
			for p := vBase; p < vEnd && outIdx < int(dur); p++ {
				packed := rom[p]
				for j := 0; j < 4 && outIdx < int(dur); j++ {
					code := (packed >> (j * 2)) & 0x03
					delta := [4]int8{-2, -1, 0, 1}[code]
					current = uint8(int8(current) + delta)
					note.SustainPerFrame[outIdx] = current
					outIdx++
				}
			}
			for outIdx < int(dur) {
				note.SustainPerFrame[outIdx] = note.SustainPerFrame[outIdx-1]
				outIdx++
			}
		} else {
			vBase := sustainValBase[sustainIdx]
			vEnd  := sustainValEnd[sustainIdx]
			vLen  := vEnd - vBase
			for i := uint16(0); i < dur; i++ {
				if int(i) < vLen {
					note.SustainPerFrame[i] = rom[vBase+int(i)]
				} else if vLen > 0 {
					note.SustainPerFrame[i] = rom[vEnd-1]
				}
			}
		}
	}

	// Build template slices once — passed to expandFreqModPattern6502
	dvTemplates := make([]DelayedVibratoTemplate, dvCount)
	for i := 0; i < dvCount; i++ {
		dvTemplates[i] = dvAt(byte(i))
	}
	seqTemplates := make([]FreqSequenceTemplate, seqTemplateCount)
	for i := 0; i < seqTemplateCount; i++ {
		seqTemplates[i] = seqTemplateAt(byte(i))
	}

	var simulatedChannels [3][]SimulatedNote

	for ch := 0; ch < 3; ch++ {
		notes := []SimulatedNote{}
		streamStart := streamOffsets[ch]
		streamEnd   := streamStart + streamLens[ch]

		// Decode bytecode stream into pattern ID sequence
		patternIDs := []byte{}
		restDursSeq := []byte{}
		blockRefs, directRefs, restRefs := 0, 0, 0

		for sp := streamStart; sp < streamEnd; {
			opcode := rom[sp]; sp++
			if opcode < 0x10 {
				// Block ref: expand inline
				if int(opcode) < blockCount {
					bBase := blockOffsets[opcode]
					bLen  := int(rom[bBase])
					for k := 1; k <= bLen; k++ {
						b := rom[bBase+k]
						if b >= 0xEE {
							durIdx := 0xFF - b
							patternIDs = append(patternIDs, 0xFF)
							restDursSeq = append(restDursSeq, byte(durIdx))
						} else {
							patternIDs = append(patternIDs, b)
						}
					}
					blockRefs++
				}
			} else if opcode >= 0xEE {
				// Inline rest
				patternIDs = append(patternIDs, 0xFF)
				restDursSeq = append(restDursSeq, opcode-0xEE)
				restRefs++
			} else {
				patternIDs = append(patternIDs, opcode-0x10)
				directRefs++
			}
		}

		fmt.Printf("  Channel %d: decoded %d block refs + %d direct refs + %d rests = %d entries from %d bytecode bytes\n",
			ch, blockRefs, directRefs, restRefs, len(patternIDs), streamLens[ch])

		// Expand each pattern ID into a SimulatedNote
		restIdx := 0
		for _, pid := range patternIDs {
			if pid == 0xFF {
				dur := restDurAt(restDursSeq[restIdx])
				restIdx++
				notes = append(notes, SimulatedNote{dur: dur, isRest: true})
				continue
			}
			if int(pid) >= notePatCount {
				continue
			}

			np := notePatAt(pid)
			frameDuration := durAt(np.durIdx)
			ins := instrAt(np.instrID)
			baseFreq := freqAt(np.freqIdx)

			// Toggle period: glissando seq or constant from freq table
			togglePeriod := uint16(0)
			var togglePeriodSeq []uint16
			if seqIdx, ok := ovLookup(toggleOvDataBase, toggleOvCount, pid); ok {
				start, end := toggleSeqAt(seqIdx)
				togglePeriodSeq = make([]uint16, frameDuration)
				for i := uint16(0); i < frameDuration; i++ {
					if i == 0 { togglePeriodSeq[i] = start } else { togglePeriodSeq[i] = end }
				}
			} else {
				togglePeriod = togglePeriodFor(np.freqIdx)
			}

			// Tail start frame
			tailStartFrame := int(ins.tail)
			if ins.tail == 0xFF {
				tailStartFrame = int(frameDuration)
			}

			note := SimulatedNote{
				freq:            baseFreq,
				Waveform:        ins.wave,
				dur:             frameDuration,
				EnvelopeMode:    ins.flags&1 != 0,
				NoisePlusTone:   ins.flags&2 != 0,
				TogglePeriod:    togglePeriod,
				TogglePeriodSeq: togglePeriodSeq,
				TailStartFrame:  tailStartFrame,
				ADSR: ADSRParams{
					Attack:  ins.attack,
					Decay:   ins.decay,
					Sustain: ins.sustain,
				},
			}

			// FreqMod
			note.freqSeq = make([]uint16, frameDuration)
			freqSeqIdx := byte(0xFF)
			if idx, ok := ovLookup(freqSeqOvDataBase, freqSeqOvCount, pid); ok {
				freqSeqIdx = idx
			}
			if freqSeqIdx != 0xFF && freqSeqIdx != 0xFE {
				fm := freqModAt(freqSeqIdx)
				expandedSeq := expandFreqModPattern6502(fm, dvTemplates, seqTemplates, baseFreq)
				last := baseFreq
				for i := uint16(0); i < frameDuration; i++ {
					if int(i) < len(expandedSeq) {
						last = expandedSeq[i]
					}
					note.freqSeq[i] = last
				}
			} else if freqSeqIdx == 0xFE {
				const vibratoDelay = 13
				vibratoOffsets := [8]int{-2, -4, -2, 0, 2, 4, 2, 0}
				for i := uint16(0); i < frameDuration; i++ {
					freq := baseFreq
					if int(i) >= vibratoDelay {
						freq = uint16(int(baseFreq) + vibratoOffsets[(int(i)-vibratoDelay)%8])
					}
					note.freqSeq[i] = freq
				}
			} else {
				for i := range note.freqSeq {
					note.freqSeq[i] = baseFreq
				}
			}

			// Waveform
			waveformIdx := byte(0xFF)
			if idx, ok := ovLookup(waveOvDataBase, waveOvCount, pid); ok {
				waveformIdx = idx
			}
			if waveformIdx != 0xFF && int(waveformIdx) < waveformCount {
				wf := waveformAt(waveformIdx)
				note.waveformSeq = make([]byte, frameDuration)
				for i := uint16(0); i < frameDuration; i++ {
					if int(i) < len(wf) {
						note.waveformSeq[i] = wf[i]
					} else if len(wf) > 0 {
						note.waveformSeq[i] = wf[len(wf)-1]
					}
				}
			} else {
				note.waveformSeq = make([]byte, frameDuration)
				for i := range note.waveformSeq {
					note.waveformSeq[i] = ins.wave
				}
			}

			// Sustain
			expandSustain(&note, np.sustainIdx)

			notes = append(notes, note)
		}

		simulatedChannels[ch] = notes
	}

	fmt.Printf("Generated %d + %d + %d = %d notes across 3 channels\n",
		len(simulatedChannels[0]), len(simulatedChannels[1]), len(simulatedChannels[2]),
		len(simulatedChannels[0])+len(simulatedChannels[1])+len(simulatedChannels[2]))

	return simulatedChannels
}


// expandFreqModPattern6502 expands a freqmod pattern into a frequency sequence.
// Only types actually used in this song are implemented: 5, 7, 11, 13.
func expandFreqModPattern6502(pattern FreqModPattern, dvTemplates []DelayedVibratoTemplate, seqTemplates []FreqSequenceTemplate, noteBaseFreq uint16) []uint16 {
	switch pattern.Type {
	case 5:
		// SequenceLookup: Data[0] = template index
		templateIdx := int(pattern.Data[0])
		template := seqTemplates[templateIdx].Frequencies
		result := make([]uint16, pattern.Length)
		for i := range result {
			if i < len(template) {
				result[i] = template[i]
			} else {
				result[i] = template[len(template)-1]
			}
		}
		return result
	case 7:
		// DelayedVibrato: Data[0] = template index; hold 12 frames at baseFreq then vibrato
		const holdFrames = 12
		templateIdx := int(pattern.Data[0])
		template := dvTemplates[templateIdx]
		result := make([]uint16, pattern.Length)
		for i := range result {
			if i <= holdFrames {
				result[i] = noteBaseFreq
			} else {
				result[i] = uint16(int32(noteBaseFreq) + int32(template.Deltas[(i-holdFrames-1)%8]))
			}
		}
		return result
	case 11:
		// HoldGliss: hold 12 frames at startFreq, apply step deltas, hold at final freq
		// Data[0,1] = startFreq, Data[2+] = steps (2 bytes each)
		const holdFrames = 12
		startFreq := uint16(pattern.Data[0]) | uint16(pattern.Data[1])<<8
		result := make([]uint16, 0, int(pattern.Length))
		for i := 0; i <= holdFrames; i++ {
			result = append(result, startFreq)
		}
		cur := startFreq
		for j := 0; 2+j*2+1 < len(pattern.Data); j++ {
			delta := int16(pattern.Data[2+j*2]) | int16(pattern.Data[2+j*2+1])<<8
			cur = uint16(int32(cur) + int32(delta))
			result = append(result, cur)
		}
		for len(result) < int(pattern.Length) {
			result = append(result, cur)
		}
		return result
	case 13:
		// HeadCycle: head_len(1) + cycle_len(1) + head_values(N×2) + cycle_values(M×2)
		headLen := int(pattern.Data[0])
		cycleLen := int(pattern.Data[1])
		result := make([]uint16, pattern.Length)
		for i := 0; i < headLen && i < int(pattern.Length); i++ {
			result[i] = uint16(pattern.Data[2+i*2]) | uint16(pattern.Data[2+i*2+1])<<8
		}
		cycleBase := 2 + headLen*2
		for i := headLen; i < int(pattern.Length); i++ {
			j := (i - headLen) % cycleLen
			result[i] = uint16(pattern.Data[cycleBase+j*2]) | uint16(pattern.Data[cycleBase+j*2+1])<<8
		}
		return result
	default:
		fmt.Printf("ERROR: Unknown FreqMod type %d\n", pattern.Type)
		return []uint16{noteBaseFreq}
	}
}

