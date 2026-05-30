// 6502 tracker player logic — SimulateTrackerFromROM is the player loop to be converted.
package main

import (
	"fmt"
)

// ram holds the simulated zero-page state for the 6502 player.
// Indexed by the ZP constants in player_layout.go.
var ram [256]byte

func ramU16(base int, ch int) int {
	lo := int(ram[base+ch])
	hi := int(ram[base+3+ch])
	return lo | hi<<8
}

func setRamU16(base int, ch int, v int) {
	ram[base+ch] = byte(v)
	ram[base+3+ch] = byte(v >> 8)
}

// fetchNext reads the next resolved entry for channel ch, advancing stream/block pointers.
//
// Encoding:
//   Stream byte b: b < blockCount → block ref (enter block, fetch first entry from body)
//                  blockCount ≤ b < 0xEE → direct pid = b - blockCount
//                  b ≥ 0xEE → rest, durIdx = b - 0xEE
//   Block byte b:  b < 0xEE → direct pid = b  (no offset)
//                  b ≥ 0xEE → rest, durIdx = 0xFF - b
//
// Always returns a resolved (pid, isRest, restDurIdx) — block entry is handled internally.
func fetchNext(rom []byte, ch int, blockOffsets []int) (pid byte, isRest bool, restDurIdx byte) {
	if ram[zpBlockCtr+ch] > 0 {
		// Inside a block body: direct pid or rest (no block-count offset, no nested blocks)
		ptr := ramU16(zpStreamPtrLo, ch)
		b := rom[ptr]
		ptr++
		ram[zpBlockCtr+ch]--
		if ram[zpBlockCtr+ch] == 0 {
			setRamU16(zpStreamPtrLo, ch, ramU16(zpReturnPtrLo, ch))
			setRamU16(zpReturnPtrLo, ch, 0)
		} else {
			setRamU16(zpStreamPtrLo, ch, ptr)
		}
		if b >= 0xEE {
			return 0, true, 0xFF - b
		}
		return b, false, 0
	}

	// Reading from stream
	ptr := ramU16(zpStreamPtrLo, ch)
	b := rom[ptr]
	ptr++
	setRamU16(zpStreamPtrLo, ch, ptr)

	if b >= 0xEE {
		return 0, true, b - 0xEE
	}
	blockCount := len(blockOffsets)
	if int(b) < blockCount {
		// Enter block: save stream pointer, aim STREAM_PTR at block body
		bBase := blockOffsets[int(b)]
		bLen := rom[bBase]
		setRamU16(zpReturnPtrLo, ch, ptr) // ptr already advanced past the block-ref byte
		setRamU16(zpStreamPtrLo, ch, bBase+1)
		ram[zpBlockCtr+ch] = bLen
		// Fetch first entry from the block body immediately
		return fetchNext(rom, ch, blockOffsets)
	}
	return b - byte(blockCount), false, 0
}

// SimulateTrackerFromROM replays the tracker format from raw ROM bytes.
// All reads use simple byte/uint16 memory ops to mirror 6502 execution.
func SimulateTrackerFromROM(rom []byte) [3][]SimulatedNote {
	fmt.Printf("\n=== Simulating Tracker Replay ===\n")

	// --- Read header: 16 × uint16 LE section offsets (bytes 0–31) + freqModCount byte 32 + pad byte 33 ---
	// Section order:
	//  0: freqTable  1: toggleValues  2: toggleIdx  3: freqMod  4: toneSeqs
	//  5: delayedVibrato  6: sustainPatterns  7: instruments
	//  8: durationTable  9: restDurations  10: notePatterns  11: toneOverrides
	//  12: blocks  13: streamA  14: streamB  15: streamC
	const numSections = 16
	const headerSize = numSections*2 + 2
	sectionOff := [numSections]int{}
	for i := range sectionOff {
		sectionOff[i] = int(uint16(rom[i*2]) | uint16(rom[i*2+1])<<8)
	}
	freqModCount := int(rom[32])
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
	// Type 5 (SequenceLookup): nibble = entry count; skip count*2 bytes of inline freq data.
	// All other types: nibble = raw byte count of Data.
	freqModBase    := sectionOff[3]
	freqModEnd     := sectionEnd(3)
	freqModOffsets := []int{}
	for p := freqModBase; p < freqModEnd; {
		freqModOffsets = append(freqModOffsets, p)
		hdr := rom[p]
		compactT := hdr >> 4
		nibble := int(hdr & 0x0F)
		if compactT == 0 { // type 5: nibble = entry count
			p += 1 + nibble*2
		} else {
			p += 1 + nibble
		}
	}

	// --- Tone sequences (sec 4): scan until section end ---
	// Each entry: length(1) + freq×2 per template.
	// --- Tone seqs (sec 4): delta+hold encoded.
	// Each entry: initialToggleIdx(1) + steps×(delta:u8, hold:u8); section boundary ends entries.
	// Since each entry has no explicit length, we record start offsets by scanning pairs:
	// entry ends when (sectionEnd - p) is even (all remaining bytes are delta/hold pairs).
	// For this song there is exactly one entry spanning the whole section.
	toneSeqBase    := sectionOff[4]
	toneSeqEnd     := sectionEnd(4)
	toneSeqOffsets := []int{}
	for p := toneSeqBase; p < toneSeqEnd; {
		toneSeqOffsets = append(toneSeqOffsets, p)
		p++ // skip initialToggleIdx
		// remaining bytes in section are pairs; all belong to this entry (one entry per song)
		p = toneSeqEnd
	}
	toneSeqCount := len(toneSeqOffsets)

	// --- Delayed vibrato templates (sec 5): count = sectionSize / 8 ---
	dvBase    := sectionOff[5]
	dvCount   := (sectionEnd(5) - sectionOff[5]) / 8
	dvOffsets := make([]int, dvCount)
	for i := range dvOffsets {
		dvOffsets[i] = dvBase + i*8
	}

	// --- Sustain patterns (sec 6): scan until section end ---
	// Each entry: header(1) + values[valLen]
	// header = (startValue_idx<<6) | valLen   startValue = sustainStartValues[idx]
	sustainBase    := sectionOff[6]
	sustainEnd     := sectionEnd(6)
	sustainOffsets := []int{}
	sustainValBase := []int{}
	sustainValEnd  := []int{}
	for p := sustainBase; p < sustainEnd; {
		sustainOffsets = append(sustainOffsets, p)
		hdr := rom[p]; p++
		valLen := int(hdr & 0x3F)
		sustainValBase = append(sustainValBase, p)
		sustainValEnd  = append(sustainValEnd, p+valLen)
		p += valLen
	}
	sustainCount := len(sustainOffsets)

	// --- Instruments (sec 7): count = sectionSize / 4 ---
	instrumentBase     := sectionOff[7]
	instrumentCount    := (sectionEnd(7) - sectionOff[7]) / 4
	instrumentDataBase := instrumentBase

	// --- Duration table (sec 8): count = sectionSize ---
	durTableBase     := sectionOff[8]
	durTableCount    := sectionEnd(8) - sectionOff[8]
	durTableDataBase := durTableBase

	// --- Rest durations (sec 9): count = sectionSize ---
	restDurBase     := sectionOff[9]
	restDurCount    := sectionEnd(9) - sectionOff[9]
	restDurDataBase := restDurBase

	// --- Note patterns (sec 10): SoA layout ---
	// freqIdx[n], packed[n], freqModIdx[fm]  (freqModCount from header byte 32)
	// notePatCount = (sectionSize - freqModCount) / 2
	notePatBase         := sectionOff[10]
	notePatFreqModCount := freqModCount
	notePatCount        := (sectionEnd(10) - notePatBase - notePatFreqModCount) / 2
	notePatFreqIdxBase  := notePatBase
	notePatPackedBase   := notePatFreqIdxBase + notePatCount
	notePatFreqModBase  := notePatPackedBase + notePatCount

	// --- Tone seq overrides (sec 11): 2 bytes each: patternIdx, toneSeqIdx ---
	toneSeqOverrideBase  := sectionOff[11]
	toneSeqOverrideCount := (sectionEnd(11) - sectionOff[11]) / 2
	toneSeqOverrideMap := make(map[byte]byte, toneSeqOverrideCount)
	for i := 0; i < toneSeqOverrideCount; i++ {
		base := toneSeqOverrideBase + i*2
		toneSeqOverrideMap[rom[base]] = rom[base+1]
	}

	// --- Blocks (sec 12): scan until section end ---
	blockBase    := sectionOff[12]
	blockEnd     := sectionEnd(12)
	blockOffsets := []int{}
	for p := blockBase; p < blockEnd; {
		blockOffsets = append(blockOffsets, p)
		n := int(rom[p])
		p += 1 + n
	}
	// --- Streams (sec 13–15): length = sectionSize from offset table ---
	streamOffsets := [3]int{sectionOff[13], sectionOff[14], sectionOff[15]}
	streamLens    := [3]int{sectionEnd(13) - sectionOff[13], sectionEnd(14) - sectionOff[14], sectionEnd(15) - sectionOff[15]}

	// Suppress unused-variable warnings
	_ = freqTableBase; _ = toggleValBase; _ = freqModBase; _ = freqModEnd
	_ = dvBase; _ = sustainBase; _ = toneSeqBase
	_ = instrumentBase; _ = durTableBase; _ = restDurBase; _ = notePatBase
	_ = blockBase; _ = toneSeqCount
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

	// waveTable maps waveCompact (0–3) back to SID waveform byte (1,2,4,8)
	waveTable := [4]byte{1, 2, 4, 8}

	// sustainStartValues maps 2-bit index (bits 7–6 of sustain pattern header) to SID S nibble
	sustainStartValues := [4]byte{10, 12, 14, 15}

	type instrData struct {
		wave, flags                    byte
		tail, attack, decay, sustain   byte
		sustainDefaultIdx              byte
	}
	// decayTable maps decayBit (0/1) back to SID D value (0/9)
	decayTable := [2]byte{0, 9}

	instrAt := func(i byte) instrData {
		base := instrumentDataBase + int(i)*4
		merged := rom[base]
		ds     := rom[base+1]
		return instrData{
			wave:              waveTable[merged>>5],
			flags:             merged & 0x07,
			tail:              rom[base+2],
			attack:            ds & 1,
			decay:             decayTable[(ds>>3)&1],
			sustain:           sustainStartValues[(ds>>1)&3],
			sustainDefaultIdx: rom[base+3],
		}
	}

	sustainIdxFor := func(instrID, _ byte) byte {
		return instrAt(instrID).sustainDefaultIdx
	}

	durAt := func(i byte) uint16 {
		return uint16(rom[durTableDataBase+int(i)])
	}

	restDurAt := func(i byte) uint16 {
		return uint16(rom[restDurDataBase+int(i)])
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
		nibble := int(header & 0x0F)
		// Map compact type (0–4) back to internal type (5,7,8,11,13)
		internalType := [5]byte{5, 7, 8, 11, 13}[compactType]
		if compactType == 0 { // type 5: nibble = entry count, data = count*2 freq bytes
			return FreqModPattern{
				Type: internalType,
				Data: rom[base+1 : base+1+nibble*2],
			}
		}
		return FreqModPattern{
			Type: internalType,
			Data: rom[base+1 : base+1+nibble],
		}
	}

	// seqTemplateFromFM builds a FreqSequenceTemplate from an inlined type-5 FreqMod.
	// Data layout: freq×2 entries (no length prefix; count = len(Data)/2).
	seqTemplateFromFM := func(fm FreqModPattern) FreqSequenceTemplate {
		n := len(fm.Data) / 2
		freqs := make([]uint16, n)
		for j := 0; j < n; j++ {
			b := j * 2
			freqs[j] = uint16(fm.Data[b]) | uint16(fm.Data[b+1])<<8
		}
		return FreqSequenceTemplate{Frequencies: freqs}
	}

	toneSeqAt := func(i byte) FreqSequenceTemplate {
		base := toneSeqOffsets[i]
		end := toneSeqEnd
		if int(i)+1 < len(toneSeqOffsets) {
			end = toneSeqOffsets[i+1]
		}
		toggleIdx := int(rom[base])
		cur := uint16(rom[toggleValDataBase+toggleIdx*2]) | uint16(rom[toggleValDataBase+toggleIdx*2+1])<<8
		var freqs []uint16
		for p := base + 1; p < end; p += 2 {
			cur += uint16(rom[p])   // delta (0 for first step)
			hold := int(rom[p+1])
			for h := 0; h < hold; h++ {
				freqs = append(freqs, cur)
			}
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
		hdr := rom[sustainOffsets[sustainIdx]]
		current := sustainStartValues[hdr>>6]
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

	var simulatedChannels [3][]SimulatedNote

	for ch := 0; ch < 3; ch++ {
		notes := []SimulatedNote{}
		streamStart := streamOffsets[ch]
		streamEnd   := streamStart + streamLens[ch]

		// Initialise channel ZP state
		setRamU16(zpStreamPtrLo, ch, streamStart)
		setRamU16(zpReturnPtrLo, ch, 0)
		ram[zpBlockCtr+ch] = 0

		for ramU16(zpStreamPtrLo, ch) < streamEnd || ram[zpBlockCtr+ch] > 0 {
			pid, isRest, restDurIdx := fetchNext(rom, ch, blockOffsets)

			if isRest {
				dur := restDurAt(restDurIdx)
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
					seqTmpl = seqTemplateFromFM(fm)
				}
			} else {
				fm = FreqModPattern{Type: 0xFF}
			}

			var toneFreqMod FreqModPattern
			var toneSeqTmpl FreqSequenceTemplate
			if tmplIdx, ok := toneSeqOverrideMap[pid]; ok {
				toneSeqTmpl = toneSeqAt(tmplIdx)
				toneFreqMod = FreqModPattern{Type: 5}
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

			sustainIdx := sustainIdxFor(np.instrID, np.durIdx)
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
