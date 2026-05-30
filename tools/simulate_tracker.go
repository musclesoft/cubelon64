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
// Block lookup walks from blockBase each time (no pre-built index array).
// Always returns a resolved (pid, isRest, restDurIdx) — block entry is handled internally.
func fetchNext(rom []byte, ch, blockBase, blockCount int) (pid byte, isRest bool, restDurIdx byte) {
	if ram[zpBlockCtr+ch] > 0 {
		// Inside a block body: direct pid or rest, no block-count offset
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
	if int(b) < blockCount {
		// Walk to block b from blockBase
		p := blockBase
		for i := 0; i < int(b); i++ {
			p += 1 + int(rom[p])
		}
		bLen := rom[p]
		setRamU16(zpReturnPtrLo, ch, ptr)
		setRamU16(zpStreamPtrLo, ch, p+1)
		ram[zpBlockCtr+ch] = bLen
		return fetchNext(rom, ch, blockBase, blockCount)
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

	// Section base addresses (constants in final player — hardcoded instead of read from header)
	freqTableBase    := sectionOff[0]
	toggleValBase    := sectionOff[1]
	toggleIdxBase    := sectionOff[2]
	freqModBase      := sectionOff[3]
	dvBase           := sectionOff[5]
	sustainBase      := sectionOff[6]
	instrumentBase   := sectionOff[7]
	durTableBase     := sectionOff[8]
	restDurBase      := sectionOff[9]
	notePatBase      := sectionOff[10]
	blockBase        := sectionOff[12]

	// Derived counts (also constants in final player)
	notePatCount        := (sectionEnd(10) - notePatBase - freqModCount) / 2
	notePatFreqIdxBase  := notePatBase
	notePatPackedBase   := notePatBase + notePatCount
	notePatFreqModBase  := notePatBase + notePatCount*2

	// Block count: walk section 12 once to count entries
	blockCount := 0
	for p := blockBase; p < sectionEnd(12); {
		p += 1 + int(rom[p])
		blockCount++
	}

	// waveTable maps waveCompact (0–3) back to SID waveform byte (1,2,4,8)
	waveTable := [4]byte{1, 2, 4, 8}

	// sustainStartValues maps 2-bit index (bits 7–6 of sustain pattern header) to SID S nibble
	sustainStartValues := [4]byte{10, 12, 14, 15}

	// decayTable maps decayBit (0/1) back to SID D value (0/9)
	decayTable := [2]byte{0, 9}

	type instrData struct {
		wave, flags                  byte
		tail, attack, decay, sustain byte
		sustainDefaultIdx            byte
	}
	instrAt := func(i byte) instrData {
		base := instrumentBase + int(i)*4
		merged := rom[base]
		ds := rom[base+1]
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

	freqAt := func(i byte) uint16 {
		base := freqTableBase + int(i)*2
		return uint16(rom[base]) | uint16(rom[base+1])<<8
	}

	togglePeriodFor := func(freqIdx byte) uint16 {
		nibbleByte := rom[toggleIdxBase+int(freqIdx)/2]
		nibble := (nibbleByte >> (freqIdx % 2 * 4)) & 0x0F
		base := toggleValBase + int(nibble)*2
		return uint16(rom[base]) | uint16(rom[base+1])<<8
	}

	durAt := func(i byte) uint16 {
		return uint16(rom[durTableBase+int(i)])
	}

	restDurAt := func(i byte) uint16 {
		return uint16(rom[restDurBase+int(i)])
	}

	type notePatData struct {
		freqIdx, instrID, durIdx, freqModIdx byte
	}
	notePatAt := func(pid byte) notePatData {
		packed := rom[notePatPackedBase+int(pid)]
		fmIdx := byte(0xFF)
		if int(pid) < freqModCount {
			fmIdx = rom[notePatFreqModBase+int(pid)]
		}
		return notePatData{
			freqIdx:    rom[notePatFreqIdxBase+int(pid)],
			instrID:    packed >> 4,
			durIdx:     packed & 0x0F,
			freqModIdx: fmIdx,
		}
	}

	// freqModPtrFor returns the ROM offset of freqMod entry i by walking from freqModBase.
	freqModPtrFor := func(i byte) int {
		p := freqModBase
		for j := byte(0); j < i; j++ {
			hdr := rom[p]
			n := int(hdr & 0x0F)
			if hdr>>4 == 0 {
				p += 1 + n*2
			} else {
				p += 1 + n
			}
		}
		return p
	}

	freqModAt := func(i byte) FreqModPattern {
		base := freqModPtrFor(i)
		header := rom[base]
		compactType := header >> 4
		nibble := int(header & 0x0F)
		internalType := [5]byte{5, 7, 8, 11, 13}[compactType]
		if compactType == 0 {
			return FreqModPattern{Type: internalType, Data: rom[base+1 : base+1+nibble*2]}
		}
		return FreqModPattern{Type: internalType, Data: rom[base+1 : base+1+nibble]}
	}

	// seqTemplateFromFM builds a FreqSequenceTemplate from an inlined type-5 FreqMod.
	seqTemplateFromFM := func(fm FreqModPattern) FreqSequenceTemplate {
		n := len(fm.Data) / 2
		freqs := make([]uint16, n)
		for j := 0; j < n; j++ {
			freqs[j] = uint16(fm.Data[j*2]) | uint16(fm.Data[j*2+1])<<8
		}
		return FreqSequenceTemplate{Frequencies: freqs}
	}

	// toneSeqAt decodes the delta+hold encoded tone sequence at index i in section 4.
	toneSeqAt := func(i byte) FreqSequenceTemplate {
		// Walk to entry i from section base
		base := sectionOff[4]
		end := sectionEnd(4)
		for j := byte(0); j < i; j++ {
			base++ // skip initialToggleIdx
			// Each remaining byte-pair is a step; this entry spans to the next entry's start.
			// (Not needed for current data: only 1 entry exists.)
		}
		toggleIdx := int(rom[base])
		cur := uint16(rom[toggleValBase+toggleIdx*2]) | uint16(rom[toggleValBase+toggleIdx*2+1])<<8
		var freqs []uint16
		for p := base + 1; p < end; p += 2 {
			cur += uint16(rom[p])
			hold := int(rom[p+1])
			for h := 0; h < hold; h++ {
				freqs = append(freqs, cur)
			}
		}
		return FreqSequenceTemplate{Frequencies: freqs}
	}

	// dvAt reads delayed vibrato template i from section 5 (8 bytes per entry).
	dvAt := func(i byte) DelayedVibratoTemplate {
		base := dvBase + int(i)*8
		var t DelayedVibratoTemplate
		for j := 0; j < 4; j++ {
			t.V[j] = int16(uint16(rom[base+j*2]) | uint16(rom[base+j*2+1])<<8)
		}
		t.Deltas = [8]int16{t.V[0], t.V[1], t.V[0], 0, -t.V[2], -t.V[3], -t.V[2], 0}
		return t
	}

	// toneOverrideFor does a linear scan through section 11 for a tone seq index matching pid.
	toneOverrideFor := func(pid byte) (byte, bool) {
		for p := sectionOff[11]; p < sectionEnd(11); p += 2 {
			if rom[p] == pid {
				return rom[p+1], true
			}
		}
		return 0, false
	}

	// loadSustainVolumes fills note.SustainPerFrame by walking to entry sustainIdx in section 6.
	loadSustainVolumes := func(note *SimulatedNote, sustainIdx byte) {
		if sustainIdx == 0xFF {
			return
		}
		// Walk to entry sustainIdx
		p := sustainBase
		for i := byte(0); i < sustainIdx; i++ {
			p += 1 + int(rom[p]&0x3F)
		}
		hdr := rom[p]
		current := sustainStartValues[hdr>>6]
		vBase := p + 1
		vEnd := vBase + int(hdr&0x3F)

		dur := note.dur
		note.SustainPerFrame = make([]uint8, dur)
		note.SustainPerFrame[0] = current
		if vBase == vEnd {
			for i := uint16(1); i < dur; i++ {
				note.SustainPerFrame[i] = current
			}
			return
		}
		// Unary bit-stream decoder: 1-bit = -1, 0-bit = emit frame and advance.
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
			if readBit() == 1 {
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

	var simulatedChannels [3][]SimulatedNote

	for ch := 0; ch < 3; ch++ {
		notes := []SimulatedNote{}
		streamStart := sectionOff[13+ch]
		streamEnd   := sectionEnd(13+ch)

		// Initialise channel ZP state
		setRamU16(zpStreamPtrLo, ch, streamStart)
		setRamU16(zpReturnPtrLo, ch, 0)
		ram[zpBlockCtr+ch] = 0

		for ramU16(zpStreamPtrLo, ch) < streamEnd || ram[zpBlockCtr+ch] > 0 {
			pid, isRest, restDurIdx := fetchNext(rom, ch, blockBase, blockCount)

			if isRest {
				notes = append(notes, SimulatedNote{dur: restDurAt(restDurIdx), isRest: true})
				continue
			}
			if int(pid) >= notePatCount {
				continue
			}

			np := notePatAt(pid)
			ins := instrAt(np.instrID)
			frameDuration := durAt(np.durIdx)
			baseFreq := freqAt(np.freqIdx)
			togglePeriod := togglePeriodFor(np.freqIdx)
			tailStartFrame := int(ins.tail)
			if ins.tail == 0xFF {
				tailStartFrame = int(frameDuration)
			}

			var fm FreqModPattern
			var dvTmpl DelayedVibratoTemplate
			var seqTmpl FreqSequenceTemplate
			switch {
			case np.freqModIdx == 0xFE:
				fm = FreqModPattern{Type: 0xFE}
			case np.freqModIdx == 0xFF:
				fm = FreqModPattern{Type: 0xFF}
			default:
				fm = freqModAt(np.freqModIdx)
				if fm.Type == 7 {
					dvTmpl = dvAt(fm.Data[0])
				} else if fm.Type == 5 {
					seqTmpl = seqTemplateFromFM(fm)
				}
			}

			var toneFreqMod FreqModPattern
			var toneSeqTmpl FreqSequenceTemplate
			if tmplIdx, ok := toneOverrideFor(pid); ok {
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
				TogglePeriod:    togglePeriod,
				TailStartFrame:  tailStartFrame,
				ADSR: ADSRParams{
					Attack:  ins.attack,
					Decay:   ins.decay,
					Sustain: ins.sustain,
				},
			}
			loadSustainVolumes(&note, ins.sustainDefaultIdx)
			notes = append(notes, note)
		}

		simulatedChannels[ch] = notes
	}

	fmt.Printf("Generated %d + %d + %d = %d notes across 3 channels\n",
		len(simulatedChannels[0]), len(simulatedChannels[1]), len(simulatedChannels[2]),
		len(simulatedChannels[0])+len(simulatedChannels[1])+len(simulatedChannels[2]))

	return simulatedChannels
}
