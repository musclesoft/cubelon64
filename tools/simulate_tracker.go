// 6502 tracker player logic — SimulateTrackerFromROM is the player loop to be converted.
package main

import (
	"fmt"
)

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
	pos += freqCount * 2

	// --- Parse toggle period values ---
	toggleValBase := pos
	toggleValCount := int(rb())
	toggleValDataBase := pos
	pos += toggleValCount * 2

	// --- Parse toggle period nibble-packed indices ---
	toggleIdxBase := pos
	pos += (freqCount + 1) / 2

	// --- Parse freqmod patterns ---
	freqModBase := pos
	freqModCount := int(rb())
	freqModOffsets := make([]int, freqModCount)
	for i := 0; i < freqModCount; i++ {
		freqModOffsets[i] = pos
		pos++                    // type
		pos++                    // patternLength
		dataLen := int(rb())     // dataLen byte
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
	sustainValBase := make([]int, sustainCount)
	sustainValEnd := make([]int, sustainCount)
	for i := 0; i < sustainCount; i++ {
		sustainOffsets[i] = pos
		flags := rb()
		isConstant := flags&1 != 0
		isPrefix := flags&4 != 0
		if isPrefix {
			pos++ // base_idx
		} else {
			pos++ // startValue
			if !isConstant {
				valLen := int(rb())
				sustainValBase[i] = pos
				sustainValEnd[i] = pos + valLen
				pos += valLen
			}
		}
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
	pos += notePatCount * 4

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
		streamOffsets[ch] = pos
		streamLens[ch] = n
		pos += n
	}

	// Suppress unused-variable warnings for section bases not directly indexed
	_ = freqTableBase; _ = toggleValBase; _ = freqModBase
	_ = seqTemplateBase; _ = dvBase; _ = sustainBase
	_ = instrumentBase; _ = durTableBase; _ = restDurBase; _ = notePatBase
	_ = blockBase; _ = streamBase

	freqAt := func(i byte) uint16 {
		base := freqTableDataBase + int(i)*2
		return uint16(rom[base]) | uint16(rom[base+1])<<8
	}

	togglePeriodFor := func(freqIdx byte) uint16 {
		nibbleByte := rom[toggleIdxBase+int(freqIdx)/2]
		nibble := (nibbleByte >> (freqIdx % 2 * 4)) & 0x0F
		base := toggleValDataBase + int(nibble)*2
		return uint16(rom[base]) | uint16(rom[base+1])<<8
	}

	type instrData struct {
		wave, flags              byte
		togglePeriod             uint16
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

	durAt := func(i byte) uint16 {
		return uint16(rom[durTableDataBase+int(i)])
	}

	restDurAt := func(i byte) uint16 {
		return uint16(rom[restDurDataBase+int(i)])
	}

	type notePatData struct {
		freqIdx, instrID, durIdx, sustainIdx, freqModIdx byte
	}
	notePatAt := func(pid byte) notePatData {
		base := notePatDataBase + int(pid)*4
		packed := rom[base+1]
		return notePatData{
			freqIdx:    rom[base],
			instrID:    packed >> 4,
			durIdx:     packed & 0x0F,
			sustainIdx: rom[base+2],
			freqModIdx: rom[base+3],
		}
	}

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

	dvAt := func(i byte) DelayedVibratoTemplate {
		base := dvOffsets[i]
		shape := rom[base]
		var t DelayedVibratoTemplate
		t.Shape = shape
		switch shape {
		case 0: // bell: V=[v0,v1,v2,v3] → Deltas=[v0,v1,v0,0,-v2,-v3,-v2,0]
			for j := 0; j < 4; j++ {
				b := base + 1 + j*2
				t.V[j] = int16(uint16(rom[b]) | uint16(rom[b+1])<<8)
			}
			t.Deltas = [8]int16{t.V[0], t.V[1], t.V[0], 0, -t.V[2], -t.V[3], -t.V[2], 0}
		case 1: // sine: V=[v0,v1,v2,v3] → Deltas=[v0,v1,-v1,-v0,-v2,-v3,v3,v2]
			for j := 0; j < 4; j++ {
				b := base + 1 + j*2
				t.V[j] = int16(uint16(rom[b]) | uint16(rom[b+1])<<8)
			}
			t.Deltas = [8]int16{t.V[0], t.V[1], -t.V[1], -t.V[0], -t.V[2], -t.V[3], t.V[3], t.V[2]}
		default: // raw: 8 × int16 Deltas
			for j := 0; j < 8; j++ {
				b := base + 1 + j*2
				t.Deltas[j] = int16(uint16(rom[b]) | uint16(rom[b+1])<<8)
			}
		}
		return t
	}

	loadSustainVolumes := func(note *SimulatedNote, sustainIdx byte) {
		if sustainIdx == 0xFF || int(sustainIdx) >= sustainCount {
			return
		}
		base := sustainOffsets[sustainIdx]
		flags := rom[base]
		isConstant := flags&1 != 0
		isDelta := flags&2 != 0
		isPrefix := flags&4 != 0
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
			vEnd := sustainValEnd[sustainIdx]
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
			vEnd := sustainValEnd[sustainIdx]
			vLen := vEnd - vBase
			for i := uint16(0); i < dur; i++ {
				if int(i) < vLen {
					note.SustainPerFrame[i] = rom[vBase+int(i)]
				} else if vLen > 0 {
					note.SustainPerFrame[i] = rom[vEnd-1]
				}
			}
		}
	}

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
		streamEnd := streamStart + streamLens[ch]

		patternIDs := []byte{}
		restDursSeq := []byte{}
		blockRefs, directRefs, restRefs := 0, 0, 0

		for sp := streamStart; sp < streamEnd; {
			opcode := rom[sp]; sp++
			if int(opcode) < blockCount {
				bBase := blockOffsets[opcode]
				bLen := int(rom[bBase])
				for k := 1; k <= bLen; k++ {
					b := rom[bBase+k]
					if b >= 0xEE {
						patternIDs = append(patternIDs, 0xFF)
						restDursSeq = append(restDursSeq, 0xFF-b)
					} else {
						patternIDs = append(patternIDs, b)
					}
				}
				blockRefs++
			} else if opcode >= 0xEE {
				patternIDs = append(patternIDs, 0xFF)
				restDursSeq = append(restDursSeq, opcode-0xEE)
				restRefs++
			} else {
				patternIDs = append(patternIDs, opcode-byte(blockCount))
				directRefs++
			}
		}

		fmt.Printf("  Channel %d: decoded %d block refs + %d direct refs + %d rests = %d entries from %d bytecode bytes\n",
			ch, blockRefs, directRefs, restRefs, len(patternIDs), streamLens[ch])

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

			togglePeriod := togglePeriodFor(np.freqIdx)
			var togglePeriodSeq []uint16

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

			note.freqSeq = make([]uint16, frameDuration)
			freqSeqIdx := np.freqModIdx
			if freqSeqIdx != 0xFF && freqSeqIdx != 0xFE {
				fm := freqModAt(freqSeqIdx)
				expandedSeq := expandFreqModPattern6502(fm, dvTemplates, seqTemplates, baseFreq)
				last := baseFreq
				for i := uint16(0); i < frameDuration; i++ {
					if int(i) < len(expandedSeq) {
						last = expandedSeq[i]
					} else if fm.Type == 7 {
						// DelayedVibrato cycles indefinitely after the hold period
						const holdFrames = 12
						templateIdx := int(fm.Data[0])
						vibratoIdx := (int(i) - holdFrames - 1) % 8
						last = uint16(int32(baseFreq) + int32(dvTemplates[templateIdx].Deltas[vibratoIdx]))
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

			note.waveformSeq = make([]byte, frameDuration)
			for i := range note.waveformSeq {
				note.waveformSeq[i] = ins.wave
			}

			loadSustainVolumes(&note, np.sustainIdx)

			notes = append(notes, note)
		}

		simulatedChannels[ch] = notes
	}

	fmt.Printf("Generated %d + %d + %d = %d notes across 3 channels\n",
		len(simulatedChannels[0]), len(simulatedChannels[1]), len(simulatedChannels[2]),
		len(simulatedChannels[0])+len(simulatedChannels[1])+len(simulatedChannels[2]))

	return simulatedChannels
}

// expandFreqModPattern6502 computes SID frequencies for a freqmod pattern.
// Only types used in this song: 5, 7, 11, 13.
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
		// DelayedVibrato: hold 12 frames at baseFreq, then apply 8-entry delta cycle
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
		// HoldGliss: hold 12 frames, apply step deltas, hold at final freq
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
