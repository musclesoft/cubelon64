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
		header := rb()           // (compactType<<4 | dataLen)
		dataLen := int(header & 0x0F)
		_ = header               // compactType decoded in freqModAt
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
		pos += 8 // 4 × int16 V params (bell only, no shape byte)
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
		pos++ // startValue
		if !isConstant {
			valLen := int(rb())
			sustainValBase[i] = pos
			sustainValEnd[i] = pos + valLen
			pos += valLen
		}
	}

	// --- Parse instruments ---
	instrumentBase := pos
	instrumentCount := int(rb())
	instrumentDataBase := pos // base of instr[0] wave byte
	pos += instrumentCount * 7

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

	// --- Parse note patterns (SoA layout) ---
	notePatBase := pos
	notePatCount := int(rb())
	notePatFreqModCount := int(rb())             // patterns 0..freqModCount-1 have freqModIdx
	notePatFreqIdxBase  := pos                   // freqIdx[0..n-1]
	notePatPackedBase   := pos + notePatCount    // packed[0..n-1]
	notePatSustainBase  := pos + notePatCount*2  // sustainIdx[0..n-1]
	notePatFreqModBase  := pos + notePatCount*3  // freqModIdx[0..freqModCount-1]
	pos += notePatCount*3 + notePatFreqModCount

	// --- Parse tone seq overrides ---
	toneSeqOverrideCount := int(rb())
	toneSeqOverrideBase := pos
	pos += toneSeqOverrideCount * 2

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
	_ = toneSeqOverrideBase; _ = blockBase; _ = streamBase

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
		base := instrumentDataBase + int(i)*7
		flags := rom[base+1]
		return instrData{
			wave:         rom[base],
			flags:        flags,
			togglePeriod: uint16(rom[base+2]) | uint16(rom[base+3])<<8,
			tail:         rom[base+4],
			attack:       (flags >> 2) & 1, // bit 2 of flags
			decay:        rom[base+5],
			sustain:      rom[base+6],
		}
	}

	durAt := func(i byte) uint16 {
		return uint16(rom[durTableDataBase+int(i)])
	}

	restDurAt := func(i byte) uint16 {
		return uint16(rom[restDurDataBase+int(i)])
	}

	// Build tone seq override map: patternIdx → templateIdx
	toneSeqOverrideMap := make(map[byte]byte, toneSeqOverrideCount)
	for i := 0; i < toneSeqOverrideCount; i++ {
		base := toneSeqOverrideBase + i*2
		toneSeqOverrideMap[rom[base]] = rom[base+1]
	}

	type notePatData struct {
		freqIdx, instrID, durIdx, sustainIdx, freqModIdx byte
	}
	notePatAt := func(pid byte) notePatData {
		packed := rom[notePatPackedBase+int(pid)]
		fmIdx := byte(0xFF)
		if int(pid) < notePatFreqModCount {
			fmIdx = rom[notePatFreqModBase+int(pid)]
		}
		return notePatData{
			freqIdx:    rom[notePatFreqIdxBase+int(pid)],
			instrID:    packed >> 4,
			durIdx:     packed & 0x0F,
			sustainIdx: rom[notePatSustainBase+int(pid)],
			freqModIdx: fmIdx,
		}
	}

	freqModAt := func(i byte) FreqModPattern {
		base := freqModOffsets[i]
		header := rom[base]
		compactType := header >> 4
		dataLen := int(header & 0x0F)
		// Map compact type (0–4) back to internal type (5,7,8,11,13)
		internalType := [5]byte{5, 7, 8, 11, 13}[compactType]
		return FreqModPattern{
			Type: internalType,
			Data: rom[base+1 : base+1+dataLen],
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
		var t DelayedVibratoTemplate
		t.Shape = 0
		for j := 0; j < 4; j++ {
			b := base + j*2
			t.V[j] = int16(uint16(rom[b]) | uint16(rom[b+1])<<8)
		}
		t.Deltas = [8]int16{t.V[0], t.V[1], t.V[0], 0, -t.V[2], -t.V[3], -t.V[2], 0}
		return t
	}

	loadSustainVolumes := func(note *SimulatedNote, sustainIdx byte) {
		if sustainIdx == 0xFF || int(sustainIdx) >= sustainCount {
			return
		}
		base := sustainOffsets[sustainIdx]
		flags := rom[base]
		isConstant := flags&1 != 0
		dur := note.dur
		note.SustainPerFrame = make([]uint8, dur)

		if isConstant {
			startVal := rom[base+1]
			for i := uint16(0); i < dur; i++ {
				note.SustainPerFrame[i] = startVal
			}
		} else {
			// Unary bit-stream decoder: 1-bit = -1, 0-bit = emit frame and advance.
			// Stops early if value reaches 0; remaining frames stay 0.
			current := rom[base+1]
			note.SustainPerFrame[0] = current
			vBase := sustainValBase[sustainIdx]
			vEnd := sustainValEnd[sustainIdx]
			byteIdx := vBase
			bitPos := uint(0)
			readBit := func() uint8 {
				if byteIdx >= vEnd {
					return 0
				}
				b := (rom[byteIdx] >> bitPos) & 1
				bitPos++
				if bitPos == 8 {
					bitPos = 0
					byteIdx++
				}
				return b
			}
			outIdx := 1
		outer:
			for outIdx < int(dur) {
				if byteIdx >= vEnd {
					note.SustainPerFrame[outIdx] = current
					break outer
				}
				bit := readBit()
				if bit == 1 {
					current--
				} else {
					note.SustainPerFrame[outIdx] = current
					outIdx++
					if current == 0 {
						break outer
					}
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
			tailStartFrame := int(ins.tail)
			if ins.tail == 0xFF {
				tailStartFrame = int(frameDuration)
			}

			freqSeqIdx := np.freqModIdx
			var fm FreqModPattern
			var dvTmpl DelayedVibratoTemplate
			var seqTmpl FreqSequenceTemplate
			if freqSeqIdx == 0xFE {
				fm = FreqModPattern{Type: 0xFE}
			} else if freqSeqIdx != 0xFF {
				fm = freqModAt(freqSeqIdx)
				if fm.Type == 7 {
					dvTmpl = dvTemplates[int(fm.Data[0])]
				} else if fm.Type == 5 {
					seqTmpl = seqTemplates[int(fm.Data[0])]
				}
			} else {
				fm = FreqModPattern{Type: 0xFF}
			}

			var toneFreqMod FreqModPattern
			var toneSeqTmpl FreqSequenceTemplate
			if tmplIdx, ok := toneSeqOverrideMap[pid]; ok {
				toneFreqMod = FreqModPattern{Type: 5, Data: []byte{tmplIdx}}
				toneSeqTmpl = seqTemplates[int(tmplIdx)]
			} else {
				toneFreqMod = FreqModPattern{Type: 0xFF}
			}

			note := SimulatedNote{
				freq:            baseFreq,
				freqMod:         fm,
				dvTemplate:      dvTmpl,
				seqTemplate:     seqTmpl,
				toneFreqMod:     toneFreqMod,
				toneSeqTemplate: toneSeqTmpl,
				Waveform:        ins.wave,
				dur:             frameDuration,
				EnvelopeMode:    ins.flags&1 != 0,
				NoisePlusTone:   ins.flags&2 != 0,
				TogglePeriod:   togglePeriod,
				TailStartFrame: tailStartFrame,
				ADSR: ADSRParams{
					Attack:  ins.attack,
					Decay:   ins.decay,
					Sustain: ins.sustain,
				},
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

