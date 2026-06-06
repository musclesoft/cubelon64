// 6502 tracker player — SimulateTrackerFromROM runs the player loop and emits per-frame SID state.
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
//
//	Stream byte b: b < blockCount → block ref (enter block, fetch first entry from body)
//	               blockCount ≤ b < 0xEE → direct pid = b - blockCount
//	               b ≥ 0xEE → rest, durIdx = b - 0xEE
//	Block byte b:  b < 0xEE → direct pid = b  (no offset)
//	               b ≥ 0xEE → rest, durIdx = 0xFF - b
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
// Returns one SIDFrame per song frame, for all three channels simultaneously.
func SimulateTrackerFromROM(rom []byte) []SIDFrame {
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
	freqTableBase  := sectionOff[0]
	toggleValBase  := sectionOff[1]
	toggleIdxBase  := sectionOff[2]
	freqModBase    := sectionOff[3]
	dvBase         := sectionOff[5]
	sustainBase    := sectionOff[6]
	instrumentBase := sectionOff[7]
	durTableBase   := sectionOff[8]
	restDurBase    := sectionOff[9]
	notePatBase    := sectionOff[10]
	blockBase      := sectionOff[12]

	// Derived counts (also constants in final player)
	notePatCount       := (sectionEnd(10) - notePatBase - freqModCount) / 2
	notePatFreqIdxBase := notePatBase
	notePatPackedBase  := notePatBase + notePatCount
	notePatFreqModBase := notePatBase + notePatCount*2

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

	type fmData struct {
		compactType byte
		nibble      int
		dataPtr     int // ROM offset of first data byte
	}
	freqModAt := func(i byte) fmData {
		base := freqModPtrFor(i)
		hdr := rom[base]
		return fmData{compactType: hdr >> 4, nibble: int(hdr & 0x0F), dataPtr: base + 1}
	}

	// freqAtFMFrame computes the SID frequency for a note at note-local frame, given FM data.
	// compactType mapping (see serializeTrackerFormat): 0=seq(5) 1=DV(7) 2=noMod(8) 3=holdGliss(11) 4=headCycle(13)
	// 0xFE=toggle(no freq mod) 0xFF=none; both have high nibble 0xF — handled as default (return baseFreq).
	freqAtFMFrame := func(baseFreq uint16, fm fmData, frame int, dvIdx byte) uint16 {
		if fm.compactType >= 5 { // sentinel: 0xFF>>4=15, 0xFE>>4=15
			return baseFreq
		}
		switch fm.compactType {
		case 0: // seq (type 5): nibble = entry count, data = count*2 freq bytes
			idx := frame
			if idx >= fm.nibble {
				idx = fm.nibble - 1
			}
			if idx < 0 {
				return baseFreq
			}
			b := fm.dataPtr + idx*2
			return uint16(rom[b]) | uint16(rom[b+1])<<8
		case 1: // DV (type 7): delayed vibrato, 8-entry delta table, 12-frame hold
			const holdFrames = 12
			if frame <= holdFrames {
				return baseFreq
			}
			dvOff := dvBase + int(dvIdx)*8
			phase := (frame - holdFrames - 1) % 8
			vIdx := [8]int{0, 1, 0, -1, 2, 3, 2, -1}[phase]
			negate := phase >= 4 && phase <= 6
			var delta int16
			if vIdx >= 0 {
				b := dvOff + vIdx*2
				delta = int16(uint16(rom[b]) | uint16(rom[b+1])<<8)
				if negate {
					delta = -delta
				}
			}
			return uint16(int32(baseFreq) + int32(delta))
		case 2: // noMod (type 8): no freq modulation
			return baseFreq
		case 3: // holdGliss (type 11): hold 12 frames then step deltas
			// fm.nibble = byte count of data; each delta is 2 bytes → max fm.nibble/2 steps.
			const holdFrames = 12
			cur := baseFreq
			if frame <= holdFrames {
				return cur
			}
			for j := 0; j < frame-holdFrames && j*2+1 < fm.nibble; j++ {
				b := fm.dataPtr + j*2
				delta := int16(rom[b]) | int16(rom[b+1])<<8
				cur = uint16(int32(cur) + int32(delta))
			}
			return cur
		case 4: // headCycle (type 13): head seq then repeating cycle
			headLen := int(rom[fm.dataPtr])
			cycleLen := int(rom[fm.dataPtr+1])
			cycleBase := fm.dataPtr + 2 + headLen*2
			if frame < headLen {
				b := fm.dataPtr + 2 + frame*2
				return uint16(rom[b]) | uint16(rom[b+1])<<8
			}
			j := (frame - headLen) % cycleLen
			b := cycleBase + j*2
			return uint16(rom[b]) | uint16(rom[b+1])<<8
		}
		return baseFreq
	}

	// toneSeqPeriodAt returns the CIA timer period at frame for tone seq entry i in section 4.
	toneSeqPeriodAt := func(i byte, frame int) uint16 {
		base := sectionOff[4]
		end := sectionEnd(4)
		for j := byte(0); j < i; j++ {
			base++
		}
		// Expand delta+hold into frame index
		toggleIdx := int(rom[base])
		cur := uint16(rom[toggleValBase+toggleIdx*2]) | uint16(rom[toggleValBase+toggleIdx*2+1])<<8
		f := 0
		for p := base + 1; p < end; p += 2 {
			cur += uint16(rom[p])
			hold := int(rom[p+1])
			last := f + hold - 1
			if frame <= last {
				return cur
			}
			f += hold
		}
		return cur
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

	// sustainAtFrame returns the SID S nibble for sustainIdx at note-local frame,
	// decoding the bitstream from section 6 on the fly.
	sustainAtFrame := func(sustainIdx byte, frame int, dur uint16) byte {
		if sustainIdx == 0xFF {
			return 0xFF // no sustain pattern
		}
		p := sustainBase
		for i := byte(0); i < sustainIdx; i++ {
			p += 1 + int(rom[p]&0x3F)
		}
		hdr := rom[p]
		current := sustainStartValues[hdr>>6]
		vBase := p + 1
		vEnd := vBase + int(hdr&0x3F)

		if vBase == vEnd {
			return current // constant
		}

		// Decode bitstream up to frame
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
		outIdx := 0
		for outIdx < frame {
			if byteIdx >= vEnd {
				return current
			}
			if readBit() == 1 {
				if current > 0 {
					current--
				}
			} else {
				outIdx++
				if current == 0 {
					return 0
				}
			}
		}
		return current
	}

	// togglePeriodForSIDFreq computes the CIA timer period that matches the SID oscillator frequency.
	// The AY toggles at the oscillator frequency; CIA fires at half that (one toggle = half period).
	togglePeriodForSIDFreq := func(baseFreq, currentFreq uint16, baseTogglePeriod uint16) uint16 {
		if baseFreq == 0 || currentFreq == 0 {
			return baseTogglePeriod
		}
		return uint16(float64(baseTogglePeriod) * float64(baseFreq) / float64(currentFreq))
	}

	// Per-channel state for the frame loop
	type chanState struct {
		// Note-level state (set at note start, consumed per frame)
		baseFreq       uint16
		baseToggle     uint16
		fm             fmData
		dvIdx          byte   // DV template index (only valid when fm.compactType==1)
		hasToneOverride bool
		toneSeqIdx     byte
		wave           byte   // base waveform byte
		envelopeMode   bool
		noisePlusTone  bool
		attack         byte
		decay          byte   // tail decay rate
		sustain        byte   // static S from instrument
		sustainIdx     byte
		tailStartFrame int
		isRest         bool
		framesLeft     int
		noteFrame      int    // frame within current note
		// Gate/transition state
		prevEnvelopeMode bool
		hasPrevNote      bool
		// For ADSR: InitialDecay (D at frame 0 when A>0)
		initialDecay byte
		tailFired    bool // set when tailStartFrame fires; locks D to decay for remainder of note
	}

	states := [3]chanState{}
	// Initialise channel ZP state
	for ch := 0; ch < 3; ch++ {
		streamStart := sectionOff[13+ch]
		streamEnd := sectionEnd(13 + ch)
		setRamU16(zpStreamPtrLo, ch, streamStart)
		setRamU16(zpReturnPtrLo, ch, 0)
		ram[zpBlockCtr+ch] = 0
		_ = streamEnd
		states[ch].framesLeft = 0
		states[ch].isRest = true
	}


	var frames []SIDFrame

	// ciaPhase[ch]: true = hi-phase (CtrlA active), false = lo-phase (CtrlB active)
	ciaPhase := [3]bool{true, true, true}

	done := [3]bool{}

	for !done[0] || !done[1] || !done[2] {
		frame := SIDFrame{}

		for ch := 0; ch < 3; ch++ {
			st := &states[ch]

			// Advance to next note if needed
			for st.framesLeft == 0 {
				streamEnd := sectionEnd(13 + ch)
				if ramU16(zpStreamPtrLo, ch) >= streamEnd && ram[zpBlockCtr+ch] == 0 {
					done[ch] = true
					break
				}
				pid, isRest, restDurIdx := fetchNext(rom, ch, blockBase, blockCount)

				if isRest {
					st.isRest = true
					st.framesLeft = int(restDurAt(restDurIdx))
					st.noteFrame = 0
					continue
				}
				if int(pid) >= notePatCount {
					st.isRest = true
					st.framesLeft = 1
					st.noteFrame = 0
					continue
				}

				np := notePatAt(pid)
				ins := instrAt(np.instrID)
				dur := durAt(np.durIdx)

				st.isRest = false
				st.framesLeft = int(dur)
				st.noteFrame = 0
				st.baseFreq = freqAt(np.freqIdx)
				st.baseToggle = togglePeriodFor(np.freqIdx)
				st.wave = ins.wave
				st.envelopeMode = ins.flags&1 != 0
				st.noisePlusTone = ins.flags&2 != 0
				st.attack = ins.attack
				st.initialDecay = ins.attack
				st.decay = ins.decay
				st.sustain = ins.sustain
				st.sustainIdx = ins.sustainDefaultIdx
				st.tailStartFrame = int(ins.tail)
				if ins.tail == 0xFF {
					st.tailStartFrame = int(dur)
				}
				st.tailFired = false

				// freqMod
				if np.freqModIdx == 0xFF {
					st.fm = fmData{compactType: 0xFF, nibble: 0, dataPtr: 0}
					st.dvIdx = 0
				} else if np.freqModIdx == 0xFE {
					st.fm = fmData{compactType: 0xFE, nibble: 0, dataPtr: 0}
					st.dvIdx = 0
				} else {
					st.fm = freqModAt(np.freqModIdx)
					if st.fm.compactType == 1 {
						st.dvIdx = rom[st.fm.dataPtr]
					}
				}

				// tone override
				if tmplIdx, ok := toneOverrideFor(pid); ok {
					st.hasToneOverride = true
					st.toneSeqIdx = tmplIdx
				} else {
					st.hasToneOverride = false
					st.toneSeqIdx = 0
				}
			}

			if done[ch] {
				// Channel finished: emit silence
				frame.Voices[ch][VFCtrlA] = 0
				continue
			}

			vf := &frame.Voices[ch]

			if st.isRest {
				// Rest: gate off, keep waveform from previous note, no freq update
				vf[VFCtrlA] = 0
				frame.TogglePeriod[ch] = 0
				st.framesLeft--
				st.noteFrame++
				continue
			}

			nf := st.noteFrame

			// Compute SID frequency for this frame
			sidFreq := freqAtFMFrame(st.baseFreq, st.fm, nf, st.dvIdx)
			vf[VFFreqLo] = byte(sidFreq)
			vf[VFFreqHi] = byte(sidFreq >> 8)

			// Pulse width
			if st.wave == 0x04 {
				vf[VFPWLo] = 0x00
				vf[VFPWHi] = 0x08 // 50% duty
			}

			// Control bytes: CtrlA = waveform hi-phase | gate; CtrlB = lo-phase | gate
			gateBit := byte(0x01)
			if st.envelopeMode {
				// Hi: waveform (triangle/sawtooth); Lo: silence (0x05 with PW=0xFFF)
				vf[VFCtrlA] = (st.wave << 4) | gateBit
				vf[VFCtrlB] = (0x05 << 4) | gateBit
			} else if st.noisePlusTone {
				// Hi: noise (0x08); Lo: silence (0x05)
				vf[VFCtrlA] = (0x08 << 4) | gateBit
				vf[VFCtrlB] = (0x05 << 4) | gateBit
			} else {
				vf[VFCtrlA] = (st.wave << 4) | gateBit
				vf[VFCtrlB] = 0 // no alternation
			}

			// Gate transition at note start
			if nf == 0 && st.hasPrevNote {
				if st.envelopeMode && st.prevEnvelopeMode {
					vf[VFGateTransition] = 1 // TestPulse: envelope adjacent notes
				} else {
					vf[VFGateTransition] = 2 // GateReset: all other note transitions
				}
			}

			// ADSR
			if st.tailFired {
				vf[VFAttDec] = (st.attack << 4) | st.decay
			} else {
				vf[VFAttDec] = (st.attack << 4) | st.initialDecay
				if nf == 1 && st.initialDecay > 0 {
					vf[VFAttDec] = st.attack << 4 // D → 0 after frame 0
				}
			}
			if st.tailStartFrame > 0 && nf == st.tailStartFrame {
				st.tailFired = true
				vf[VFAttDec] = (st.attack << 4) | st.decay
			}

			// SusRel: S nibble per frame from sustain bitstream, else static S.
			// Envelope-mode notes keep S fixed (the old renderer never updates S per-frame for these).
			s := st.sustain
			if st.sustainIdx != 0xFF && !st.envelopeMode {
				s = sustainAtFrame(st.sustainIdx, nf, uint16(st.framesLeft+nf))
			}
			vf[VFSusRel] = s << 4 // R=0

			// CIA toggle period
			if st.envelopeMode || st.noisePlusTone {
				var togglePeriod uint16
				if st.hasToneOverride {
					togglePeriod = toneSeqPeriodAt(st.toneSeqIdx, nf)
				} else {
					togglePeriod = togglePeriodForSIDFreq(st.baseFreq, sidFreq, st.baseToggle)
				}
				frame.TogglePeriod[ch] = togglePeriod
			} else {
				frame.TogglePeriod[ch] = 0
			}

			// Update CIA phase tracking (phase flips on each timer fire — handled by renderer)
			_ = ciaPhase[ch]

			st.prevEnvelopeMode = st.envelopeMode
			st.hasPrevNote = true
			st.framesLeft--
			st.noteFrame++
		}

		frames = append(frames, frame)
	}

	fmt.Printf("Generated %d song frames across 3 channels\n", len(frames))
	return frames
}

// NoisePlusToneCtrl holds per-frame CtrlA/CtrlB overrides for NoisePlusTone notes,
// computed from the live AY mixer state which isn't available inside SimulateTrackerFromROM.
type NoisePlusToneCtrl struct {
	CtrlA, CtrlB    byte
	SIDFreqLo, SIDFreqHi byte
	TogglePeriod    uint16
}

// SimulateTrackerFrames serializes the tracker format to ROM bytes then returns per-frame SID state.
// nptOverrides[ch][songFrame] overrides CtrlA/CtrlB/freq for NoisePlusTone frames on that channel.
// Pass nil to use the default static Hi=noise/Lo=silence behaviour.
func SimulateTrackerFrames(format *TrackerFormat, nptOverrides [3][]NoisePlusToneCtrl) []SIDFrame {
	frames := SimulateTrackerFromROM(serializeTrackerFormat(format))
	// Apply NoisePlusTone per-frame overrides where provided.
	for sfIdx := range frames {
		for ch := 0; ch < 3; ch++ {
			if nptOverrides[ch] == nil || sfIdx >= len(nptOverrides[ch]) {
				continue
			}
			ov := nptOverrides[ch][sfIdx]
			if ov.CtrlA == 0 {
				continue // no override for this frame
			}
			vf := &frames[sfIdx].Voices[ch]
			vf[VFCtrlA] = ov.CtrlA
			vf[VFCtrlB] = ov.CtrlB
			vf[VFFreqLo] = ov.SIDFreqLo
			vf[VFFreqHi] = ov.SIDFreqHi
			frames[sfIdx].TogglePeriod[ch] = ov.TogglePeriod
		}
	}
	return frames
}

// SimulateTrackerToNotes is a compatibility shim: runs the player and converts
// the SIDFrame stream back to per-channel SimulatedNote slices for the optimizer.
func SimulateTrackerToNotes(rom []byte) [3][]SimulatedNote {
	// TODO: replace with SIDFrame-based rendering once optimizer is updated
	return simulateTrackerLegacy(rom)
}

// simulateTrackerLegacy is the old SimulateTrackerFromROM kept temporarily for the optimizer.
func simulateTrackerLegacy(rom []byte) [3][]SimulatedNote {
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

	freqTableBase  := sectionOff[0]
	toggleValBase  := sectionOff[1]
	toggleIdxBase  := sectionOff[2]
	freqModBase    := sectionOff[3]
	dvBase         := sectionOff[5]
	sustainBase    := sectionOff[6]
	instrumentBase := sectionOff[7]
	durTableBase   := sectionOff[8]
	restDurBase    := sectionOff[9]
	notePatBase    := sectionOff[10]
	blockBase      := sectionOff[12]

	notePatCount       := (sectionEnd(10) - notePatBase - freqModCount) / 2
	notePatFreqIdxBase := notePatBase
	notePatPackedBase  := notePatBase + notePatCount
	notePatFreqModBase := notePatBase + notePatCount*2

	blockCount := 0
	for p := blockBase; p < sectionEnd(12); {
		p += 1 + int(rom[p])
		blockCount++
	}

	waveTable          := [4]byte{1, 2, 4, 8}
	sustainStartValues := [4]byte{10, 12, 14, 15}
	decayTable         := [2]byte{0, 9}

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
	durAt := func(i byte) uint16 { return uint16(rom[durTableBase+int(i)]) }
	restDurAt := func(i byte) uint16 { return uint16(rom[restDurBase+int(i)]) }

	type notePatData struct{ freqIdx, instrID, durIdx, freqModIdx byte }
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
	freqModPtrFor := func(i byte) int {
		p := freqModBase
		for j := byte(0); j < i; j++ {
			hdr := rom[p]; n := int(hdr & 0x0F)
			if hdr>>4 == 0 { p += 1 + n*2 } else { p += 1 + n }
		}
		return p
	}
	freqModAt := func(i byte) FreqModPattern {
		base := freqModPtrFor(i)
		hdr := rom[base]; compactType := hdr >> 4; nibble := int(hdr & 0x0F)
		internalType := [5]byte{5, 7, 8, 11, 13}[compactType]
		if compactType == 0 { return FreqModPattern{Type: internalType, Data: rom[base+1 : base+1+nibble*2]} }
		return FreqModPattern{Type: internalType, Data: rom[base+1 : base+1+nibble]}
	}
	seqTemplateFromFM := func(fm FreqModPattern) FreqSequenceTemplate {
		n := len(fm.Data) / 2; freqs := make([]uint16, n)
		for j := 0; j < n; j++ { freqs[j] = uint16(fm.Data[j*2]) | uint16(fm.Data[j*2+1])<<8 }
		return FreqSequenceTemplate{Frequencies: freqs}
	}
	toneSeqAt := func(i byte) FreqSequenceTemplate {
		base := sectionOff[4]; end := sectionEnd(4)
		for j := byte(0); j < i; j++ { base++ }
		toggleIdx := int(rom[base])
		cur := uint16(rom[toggleValBase+toggleIdx*2]) | uint16(rom[toggleValBase+toggleIdx*2+1])<<8
		var freqs []uint16
		for p := base + 1; p < end; p += 2 {
			cur += uint16(rom[p]); hold := int(rom[p+1])
			for h := 0; h < hold; h++ { freqs = append(freqs, cur) }
		}
		return FreqSequenceTemplate{Frequencies: freqs}
	}
	dvAt := func(i byte) DelayedVibratoTemplate {
		base := dvBase + int(i)*8; var t DelayedVibratoTemplate
		for j := 0; j < 4; j++ { t.V[j] = int16(uint16(rom[base+j*2]) | uint16(rom[base+j*2+1])<<8) }
		t.Deltas = [8]int16{t.V[0], t.V[1], t.V[0], 0, -t.V[2], -t.V[3], -t.V[2], 0}
		return t
	}
	toneOverrideFor := func(pid byte) (byte, bool) {
		for p := sectionOff[11]; p < sectionEnd(11); p += 2 {
			if rom[p] == pid { return rom[p+1], true }
		}
		return 0, false
	}
	loadSustainVolumes := func(note *SimulatedNote, sustainIdx byte) {
		if sustainIdx == 0xFF { return }
		p := sustainBase
		for i := byte(0); i < sustainIdx; i++ { p += 1 + int(rom[p]&0x3F) }
		hdr := rom[p]; current := sustainStartValues[hdr>>6]
		vBase := p + 1; vEnd := vBase + int(hdr&0x3F)
		dur := note.dur
		note.SustainPerFrame = make([]uint8, dur)
		note.SustainPerFrame[0] = current
		if vBase == vEnd {
			for i := uint16(1); i < dur; i++ { note.SustainPerFrame[i] = current }
			return
		}
		byteIdx := vBase; bitPos := uint(0)
		readBit := func() uint8 {
			if byteIdx >= vEnd { return 0 }
			b := (rom[byteIdx] >> bitPos) & 1; bitPos++
			if bitPos == 8 { bitPos = 0; byteIdx++ }
			return b
		}
		outIdx := 1
	outer:
		for outIdx < int(dur) {
			if byteIdx >= vEnd { note.SustainPerFrame[outIdx] = current; break outer }
			if readBit() == 1 { current-- } else {
				note.SustainPerFrame[outIdx] = current; outIdx++
				if current == 0 { break outer }
			}
		}
	}

	var simulatedChannels [3][]SimulatedNote
	for ch := 0; ch < 3; ch++ {
		notes := []SimulatedNote{}
		streamStart := sectionOff[13+ch]
		streamEnd := sectionEnd(13 + ch)
		setRamU16(zpStreamPtrLo, ch, streamStart)
		setRamU16(zpReturnPtrLo, ch, 0)
		ram[zpBlockCtr+ch] = 0

		for ramU16(zpStreamPtrLo, ch) < streamEnd || ram[zpBlockCtr+ch] > 0 {
			pid, isRest, restDurIdx := fetchNext(rom, ch, blockBase, blockCount)
			if isRest {
				notes = append(notes, SimulatedNote{dur: restDurAt(restDurIdx), isRest: true})
				continue
			}
			if int(pid) >= notePatCount { continue }
			np := notePatAt(pid)
			ins := instrAt(np.instrID)
			frameDuration := durAt(np.durIdx)
			baseFreq := freqAt(np.freqIdx)
			togglePeriod := togglePeriodFor(np.freqIdx)
			tailStartFrame := int(ins.tail)
			if ins.tail == 0xFF { tailStartFrame = int(frameDuration) }
			var fm FreqModPattern
			var dvTmpl DelayedVibratoTemplate
			var seqTmpl FreqSequenceTemplate
			switch {
			case np.freqModIdx == 0xFE: fm = FreqModPattern{Type: 0xFE}
			case np.freqModIdx == 0xFF: fm = FreqModPattern{Type: 0xFF}
			default:
				fm = freqModAt(np.freqModIdx)
				if fm.Type == 7 { dvTmpl = dvAt(fm.Data[0]) } else if fm.Type == 5 { seqTmpl = seqTemplateFromFM(fm) }
			}
			var toneFreqMod FreqModPattern
			var toneSeqTmpl FreqSequenceTemplate
			if tmplIdx, ok := toneOverrideFor(pid); ok {
				toneSeqTmpl = toneSeqAt(tmplIdx); toneFreqMod = FreqModPattern{Type: 5}
			} else {
				toneFreqMod = FreqModPattern{Type: 0xFF}
			}
			note := SimulatedNote{
				freq: baseFreq, freqMod: fm, dvTemplate: dvTmpl, seqTemplate: seqTmpl,
				toneFreqMod: toneFreqMod, toneSeqTemplate: toneSeqTmpl,
				Waveform: ins.wave, dur: frameDuration,
				EnvelopeMode: ins.flags&1 != 0, NoisePlusTone: ins.flags&2 != 0,
				TogglePeriod: togglePeriod, TailStartFrame: tailStartFrame,
				ADSR: ADSRParams{Attack: ins.attack, Decay: ins.decay, Sustain: ins.sustain},
			}
			loadSustainVolumes(&note, ins.sustainDefaultIdx)
			notes = append(notes, note)
		}
		simulatedChannels[ch] = notes
	}
	return simulatedChannels
}
