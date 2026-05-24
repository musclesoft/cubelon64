// 6502 tracker player logic — SimulateTrackerFromROM is the player loop to be converted.
package main

import (
	"fmt"
)

// SimulateTrackerFromROM replays the tracker format from raw ROM bytes.
// All reads use simple byte/uint16 memory ops to mirror 6502 execution.
func SimulateTrackerFromROM(rom []byte) [3][]SimulatedNote {
	fmt.Printf("\n=== Simulating Tracker Replay ===\n")

	// --- Read section offset table ---
	const numSections = 17
	const headerSize = numSections * 2
	sectionOff := [numSections]int{}
	for i := range sectionOff {
		sectionOff[i] = int(uint16(rom[i*2]) | uint16(rom[i*2+1])<<8)
	}
	sectionEnd := func(i int) int {
		if i+1 < numSections {
			return sectionOff[i+1]
		}
		return len(rom)
	}
	_ = headerSize

	// --- Freq table (sec 0): freqCount = sectionSize / 2 ---
	freqTableBase     := sectionOff[0]
	freqCount         := (sectionOff[1] - sectionOff[0]) / 2
	freqTableDataBase := freqTableBase

	// --- Toggle period values (sec 1): toggleValCount = sectionSize / 2 ---
	toggleValBase     := sectionOff[1]
	toggleValCount    := (sectionOff[2] - sectionOff[1]) / 2
	toggleValDataBase := toggleValBase

	// --- Toggle period nibble-packed indices (sec 2) ---
	toggleIdxBase := sectionOff[2]

	// --- FreqMod patterns (sec 3): scan until section end ---
	freqModBase   := sectionOff[3]
	freqModEnd    := sectionEnd(3)
	freqModOffsets := []int{}
	for p := freqModBase; p < freqModEnd; {
		freqModOffsets = append(freqModOffsets, p)
		dataLen := int(rom[p] & 0x0F)
		p += 1 + dataLen
	}
	freqModCount := len(freqModOffsets)

	// --- FreqSequence templates (sec 4): scan until section end ---
	seqTemplateBase    := sectionOff[4]
	seqTemplateEnd     := sectionEnd(4)
	seqTemplateOffsets := []int{}
	for p := seqTemplateBase; p < seqTemplateEnd; {
		seqTemplateOffsets = append(seqTemplateOffsets, p)
		n := int(rom[p])
		p += 1 + n*2
	}
	seqTemplateCount := len(seqTemplateOffsets)

	// --- Delayed vibrato templates (sec 5): count = sectionSize / 8 ---
	dvBase   := sectionOff[5]
	dvCount  := (sectionEnd(5) - sectionOff[5]) / 8
	dvOffsets := make([]int, dvCount)
	for i := range dvOffsets {
		dvOffsets[i] = dvBase + i*8
	}

	// --- Sustain patterns (sec 6): scan until section end ---
	sustainBase    := sectionOff[6]
	sustainEnd     := sectionEnd(6)
	sustainOffsets := []int{}
	sustainValBase := []int{}
	sustainValEnd  := []int{}
	for p := sustainBase; p < sustainEnd; {
		sustainOffsets = append(sustainOffsets, p)
		p++ // startValue
		valLen := int(rom[p]); p++
		sustainValBase = append(sustainValBase, p)
		sustainValEnd  = append(sustainValEnd, p+valLen)
		p += valLen
	}
	sustainCount := len(sustainOffsets)

	// --- Instruments (sec 7): count = sectionSize / 8 ---
	instrumentBase     := sectionOff[7]
	instrumentCount    := (sectionEnd(7) - sectionOff[7]) / 8
	instrumentDataBase := instrumentBase

	// --- Sustain table (sec 8): count = sectionSize ---
	sustainTableBase  := sectionOff[8]
	sustainTableCount := sectionEnd(8) - sectionOff[8]

	// --- Duration table (sec 9): count = sectionSize ---
	durTableBase     := sectionOff[9]
	durTableCount    := sectionEnd(9) - sectionOff[9]
	durTableDataBase := durTableBase

	// --- Rest durations (sec 10): count = sectionSize ---
	restDurBase     := sectionOff[10]
	restDurCount    := sectionEnd(10) - sectionOff[10]
	restDurDataBase := restDurBase

	// --- Note patterns (sec 11): SoA layout ---
	// Byte 0: freqModCount; then freqIdx[n], packed[n], freqModIdx[fm]
	// notePatCount = (sectionSize - 1 - freqModCount) / 2
	notePatBase         := sectionOff[11]
	notePatFreqModCount := int(rom[notePatBase])
	notePatCount        := (sectionEnd(11) - notePatBase - 1 - notePatFreqModCount) / 2
	notePatFreqIdxBase  := notePatBase + 1
	notePatPackedBase   := notePatFreqIdxBase + notePatCount
	notePatFreqModBase  := notePatPackedBase + notePatCount

	// --- Tone seq overrides (sec 12): count = sectionSize / 2 ---
	toneSeqOverrideBase  := sectionOff[12]
	toneSeqOverrideCount := (sectionEnd(12) - sectionOff[12]) / 2

	// --- Blocks (sec 13): scan until section end ---
	blockBase    := sectionOff[13]
	blockEnd     := sectionEnd(13)
	blockOffsets := []int{}
	for p := blockBase; p < blockEnd; {
		blockOffsets = append(blockOffsets, p)
		n := int(rom[p])
		p += 1 + n
	}
	blockCount := len(blockOffsets)

	// --- Streams (sec 14–16): length = sectionSize from offset table ---
	streamOffsets := [3]int{sectionOff[14], sectionOff[15], sectionOff[16]}
	streamLens    := [3]int{sectionEnd(14) - sectionOff[14], sectionEnd(15) - sectionOff[15], sectionEnd(16) - sectionOff[16]}

	// Suppress unused-variable warnings
	_ = freqTableBase; _ = toggleValBase; _ = freqModBase; _ = freqModEnd
	_ = seqTemplateBase; _ = dvBase; _ = sustainBase
	_ = instrumentBase; _ = durTableBase; _ = restDurBase; _ = notePatBase
	_ = toneSeqOverrideBase; _ = blockBase; _ = sustainTableCount
	_ = toggleValCount; _ = durTableCount; _ = restDurCount
	_ = freqCount; _ = freqModCount; _ = instrumentCount

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

	sustainTable := rom[sustainTableBase : sustainTableBase+sustainTableCount]

	type instrData struct {
		wave, flags              byte
		togglePeriod             uint16
		tail, attack, decay, sustain, sustainOffset byte
	}
	instrAt := func(i byte) instrData {
		base := instrumentDataBase + int(i)*8
		flags := rom[base+1]
		return instrData{
			wave:          rom[base],
			flags:         flags,
			togglePeriod:  uint16(rom[base+2]) | uint16(rom[base+3])<<8,
			tail:          rom[base+4],
			attack:        (flags >> 2) & 1, // bit 2 of flags
			decay:         rom[base+5],
			sustain:       rom[base+6],
			sustainOffset: rom[base+7],
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
		freqIdx, instrID, durIdx, freqModIdx byte
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
		dur := note.dur
		note.SustainPerFrame = make([]uint8, dur)
		current := rom[sustainOffsets[sustainIdx]]
		note.SustainPerFrame[0] = current
		vBase := sustainValBase[sustainIdx]
		vEnd := sustainValEnd[sustainIdx]
		if vBase == vEnd {
			// valLen=0: constant, fill remaining frames
			for i := uint16(1); i < dur; i++ {
				note.SustainPerFrame[i] = current
			}
			return
		}
		// Unary bit-stream decoder: 1-bit = -1, 0-bit = emit frame and advance.
		// Stops early if value reaches 0; remaining frames stay 0.
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

			sustainIdx := byte(0xFF)
			if ins.sustainOffset != 0xFF {
				tableIdx := int(ins.sustainOffset) + int(np.durIdx)
				if tableIdx >= 0 && tableIdx < len(sustainTable) {
					sustainIdx = sustainTable[tableIdx]
				}
			}
			loadSustainVolumes(&note, sustainIdx)

			notes = append(notes, note)
		}

		simulatedChannels[ch] = notes
	}

	fmt.Printf("Generated %d + %d + %d = %d notes across 3 channels\n",
		len(simulatedChannels[0]), len(simulatedChannels[1]), len(simulatedChannels[2]),
		len(simulatedChannels[0])+len(simulatedChannels[1])+len(simulatedChannels[2]))

	return simulatedChannels
}

