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
// Block lookup uses the blockLengths table for O(N) offset sum (no body scan).
// Always returns a resolved (pid, isRest, restDurIdx) — block entry is handled internally.
func fetchNext(rom []byte, ch, blockBase, blockLenBase, blockCount int) (pid byte, isRest bool, restDurIdx byte) {
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
		// Seek to block b by summing lengths from the blockLengths table
		p := blockBase
		for i := 0; i < int(b); i++ {
			p += int(rom[blockLenBase+i])
		}
		bLen := rom[p] // length byte at block start
		setRamU16(zpReturnPtrLo, ch, ptr)
		setRamU16(zpStreamPtrLo, ch, p+1)
		ram[zpBlockCtr+ch] = bLen
		return fetchNext(rom, ch, blockBase, blockLenBase, blockCount)
	}
	return b - byte(blockCount), false, 0
}

// SimulateTrackerFromROM replays the tracker format from raw ROM bytes.
// Returns one SIDFrame per song frame, for all three channels simultaneously.
//
// All per-frame state is held in chanState fields that advance one step per frame,
// mirroring 6502 ZP registers — no O(frame) rewinds.
func SimulateTrackerFromROM(rom []byte) []SIDFrame {
	fmt.Printf("\n=== Simulating Tracker Replay ===\n")

	const numSections = 20
	sectionOff := [numSections]int{}
	for i := range sectionOff {
		sectionOff[i] = int(uint16(rom[i*2]) | uint16(rom[i*2+1])<<8)
	}
	freqModCount := int(rom[40])
	sectionEnd := func(i int) int {
		if i+1 < numSections {
			return sectionOff[i+1]
		}
		return len(rom)
	}

	freqTableBase      := sectionOff[0]
	toggleValBase      := sectionOff[1]
	toggleIdxBase      := sectionOff[2]
	freqModBase        := sectionOff[3]
	sustainBase        := sectionOff[6]
	instrumentBase     := sectionOff[7]
	durTableBase       := sectionOff[8]
	restDurBase        := sectionOff[9]
	notePatBase        := sectionOff[10]
	blockBase          := sectionOff[12]
	fmOffsetsBase      := sectionOff[16]
	blockLenBase       := sectionOff[17]
	nptWaveSeqBase     := sectionOff[18]
	nptWaveOverrideBase := sectionOff[19]

	notePatCount       := (sectionEnd(10) - notePatBase - freqModCount) / 2
	notePatFreqIdxBase := notePatBase
	notePatPackedBase  := notePatBase + notePatCount
	notePatFreqModBase := notePatBase + notePatCount*2
	blockCount         := sectionEnd(17) - blockLenBase

	// Build nptWaveOverride lookup: patternIdx → ROM addresses of wave and toggle seq data.
	// Section 18 entry layout: waveByteLen(1) + wave_packed_bytes + frameCount(1) + uint16×frameCount;
	// frameCount 0xFF marks a per-note list of uint16 pairs instead. Section 19 holds each entry's byte offset.
	nptWaveOverrideCount := (sectionEnd(19) - nptWaveOverrideBase) / 2
	nptWaveSeqFor   := make(map[byte]int, nptWaveOverrideCount) // pid → ROM addr of waveByteLen byte
	nptTogglePtrFor := make(map[byte]int, nptWaveOverrideCount) // pid → ROM addr of frameCount byte
	for i := 0; i < nptWaveOverrideCount; i++ {
		base := nptWaveOverrideBase + i*2
		pid := rom[base]
		p := nptWaveSeqBase + int(rom[base+1])
		nptWaveSeqFor[pid]   = p                                   // → waveByteLen byte
		nptTogglePtrFor[pid] = p + 1 + int(rom[p])                 // → frameCount byte
	}

	sustainStartValues := [4]byte{10, 12, 14, 15}
	decayTable         := [2]byte{0, 9}

	type instrData struct {
		wave, flags, tail, attack, decay, sustain, sustainDefaultIdx byte
	}
	instrAt := func(i byte) instrData {
		base := instrumentBase + int(i)*4
		merged := rom[base]
		ds := rom[base+1]
		return instrData{
			wave:              merged >> 4,
			flags:             (merged>>1)&1 | (merged>>2)&1<<1,
			tail:              rom[base+2],
			attack:            (ds >> 4) & 1,
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

	durAt      := func(i byte) uint16 { return uint16(rom[durTableBase+int(i)]) }
	restDurAt  := func(i byte) uint16 { return uint16(rom[restDurBase+int(i)]) }

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

	type fmData struct {
		compactType byte
		nibble      int
		dataPtr     int
	}
	freqModAt := func(i byte) fmData {
		base := freqModBase + int(rom[fmOffsetsBase+int(i)])
		hdr := rom[base]
		return fmData{compactType: hdr >> 4, nibble: int(hdr & 0x0F), dataPtr: base + 1}
	}

	// sustainPtr points at the start of the bitstream body for a given sustain pattern byte offset.
	// Returns (bodyBase, bodyEnd, startValue). Caller owns byteIdx/bitPos state.
	sustainPtrFor := func(sustainIdx byte) (bodyBase, bodyEnd int, startVal byte) {
		p := sustainBase + int(sustainIdx)
		hdr := rom[p]
		startVal = sustainStartValues[hdr>>6]
		bodyBase = p + 1
		bodyEnd = bodyBase + int(hdr&0x3F)
		return
	}

	// Per-channel state — all fields map directly to 6502 ZP/BSS variables.
	type chanState struct {
		// Stream decode (mirrored in ZP by fetchNext)
		framesLeft int
		isRest     bool
		noteFrame  int // counts up each frame within the current note

		// Note parameters (set at note start)
		baseFreq      uint16
		baseToggle    uint16
		wave          byte
		envelopeMode  bool
		noisePlusTone bool
		attack        byte
		decay         byte
		initialDecay  byte
		sustain       byte // static S nibble from instrument
		tailStartFrame int
		tailFired      bool

		// FM state — incremental, one step per frame (all = ZP byte or uint16 pair)
		fm        fmData
		dvPhase   byte   // DV: current phase 0-7 within the 8-step cycle
		glissFreq uint16 // holdGliss: accumulated current frequency
		glissPtr  int    // holdGliss: ROM pointer to next unused delta (stops at glissEnd)
		glissEnd  int    // holdGliss: ROM pointer one past last delta byte
		seqFreq   uint16 // seq/headCycle: current output frequency
		seqPtr    int    // seq/headCycle: ROM pointer to current entry (uint16 pair)
		seqEnd    int    // seq: ROM pointer one past last entry (clamp boundary)
		cycleBase int    // headCycle: ROM pointer to start of cycle section
		cycleEnd  int    // headCycle: ROM pointer one past last cycle entry

		// Sustain bitstream state — incremental
		sustainByteIdx int
		sustainBitPos  uint
		sustainEnd     int
		sustainVal     byte // current S nibble output

		// NPT waveform state + toggle period sequences (0 if no override — use default mixed state)
		nptWavePtr   int // ROM addr of waveByteLen byte (0 = no override)
		nptTogglePtr int // ROM addr of frameCount byte (toggle periods follow, uint16 per frame)
		nptNoteToggle [2]uint16 // first-frame and held toggle periods (per-note lists)

		// Gate transition tracking
		prevEnvelopeMode bool
		hasPrevNote      bool
	}

	states := [3]chanState{}
	for ch := 0; ch < 3; ch++ {
		setRamU16(zpStreamPtrLo, ch, sectionOff[13+ch])
		setRamU16(zpReturnPtrLo, ch, 0)
		ram[zpBlockCtr+ch] = 0
		states[ch].framesLeft = 0
		states[ch].isRest = true
	}

	// initNote populates all incremental state for a new note.
	initNote := func(st *chanState, np notePatData, ins instrData, dur uint16) {
		st.framesLeft = int(dur)
		st.noteFrame = 0
		st.isRest = false
		st.baseFreq = freqAt(np.freqIdx)
		st.baseToggle = togglePeriodFor(np.freqIdx)
		st.wave = ins.wave
		st.envelopeMode = ins.flags&1 != 0
		st.noisePlusTone = ins.flags&2 != 0
		st.attack = ins.attack
		st.initialDecay = ins.attack
		st.decay = ins.decay
		st.sustain = ins.sustain
		st.tailStartFrame = int(ins.tail)
		if ins.tail == 0xFF {
			st.tailStartFrame = int(dur)
		}
		st.tailFired = false

		// FM incremental init — all pointer fields are ROM addresses (uint16 on 6502)
		st.dvPhase = 0
		st.glissFreq = st.baseFreq
		st.glissPtr = 0
		st.glissEnd = 0
		st.seqFreq = st.baseFreq
		st.seqPtr = 0
		st.seqEnd = 0
		st.cycleBase = 0
		st.cycleEnd = 0
		if np.freqModIdx == 0xFF || np.freqModIdx == 0xFE {
			st.fm = fmData{compactType: 0xFF}
		} else {
			st.fm = freqModAt(np.freqModIdx)
			switch st.fm.compactType {
			case 0: // seq: seqPtr starts at first entry, seqEnd one past last
				st.seqPtr = st.fm.dataPtr
				st.seqEnd = st.fm.dataPtr + st.fm.nibble*2
				st.seqFreq = uint16(rom[st.seqPtr]) | uint16(rom[st.seqPtr+1])<<8
			case 3: // holdGliss: glissPtr at first delta, glissEnd past last
				st.glissPtr = st.fm.dataPtr
				st.glissEnd = st.fm.dataPtr + st.fm.nibble // nibble = byte count
			case 4: // headCycle: seqPtr at head[0], cycleBase/cycleEnd for wrap
				headLen := int(rom[st.fm.dataPtr])
				cycleLen := int(rom[st.fm.dataPtr+1])
				st.seqPtr = st.fm.dataPtr + 2
				st.cycleBase = st.fm.dataPtr + 2 + headLen*2
				st.cycleEnd = st.cycleBase + cycleLen*2
				st.seqFreq = uint16(rom[st.seqPtr]) | uint16(rom[st.seqPtr+1])<<8
			}
		}

		// Sustain bitstream init
		sustainIdx := ins.sustainDefaultIdx
		if sustainIdx == 0xFF || ins.flags&1 != 0 {
			st.sustainByteIdx = 0
			st.sustainBitPos = 0
			st.sustainEnd = 0
			st.sustainVal = ins.sustain
		} else {
			bodyBase, bodyEnd, startVal := sustainPtrFor(sustainIdx)
			st.sustainVal = startVal
			st.sustainByteIdx = bodyBase
			st.sustainBitPos = 0
			st.sustainEnd = bodyEnd
		}
	}

	// stepSustain advances the sustain bitstream by one output frame.
	stepSustain := func(st *chanState) {
		if st.sustainByteIdx >= st.sustainEnd {
			return // exhausted: hold last value
		}
		for {
			if st.sustainByteIdx >= st.sustainEnd {
				return
			}
			bit := (rom[st.sustainByteIdx] >> st.sustainBitPos) & 1
			st.sustainBitPos++
			if st.sustainBitPos == 8 {
				st.sustainBitPos = 0
				st.sustainByteIdx++
			}
			if bit == 1 {
				if st.sustainVal > 0 {
					st.sustainVal--
				}
			} else {
				return // consumed one output frame
			}
		}
	}

	// stepFM advances all incremental FM state by one frame and returns the SID frequency.
	// All state is in chanState fields (byte/uint16) — no frame-number indexing, no modulo.
	// holdFrames=12 is the shared constant for DV and holdGliss.
	stepFM := func(st *chanState, nf int) uint16 {
		const holdFrames = 12
		fm := &st.fm
		switch fm.compactType {
		case 0xFF, 0xFE, 2: // none / toggle-only / noMod
			return st.baseFreq

		case 0: // seq: seqPtr advances to next entry each frame, holds at last
			// seqFreq and seqPtr initialised in initNote; advance after frame 0.
			if nf > 0 {
				next := st.seqPtr + 2
				if next < st.seqEnd {
					st.seqPtr = next
				}
				st.seqFreq = uint16(rom[st.seqPtr]) | uint16(rom[st.seqPtr+1])<<8
			}
			return st.seqFreq

		case 1: // DV: hold baseFreq for holdFrames, then advance 8-phase cycle each frame
			if nf <= holdFrames {
				st.dvPhase = 0
				return st.baseFreq
			}
			phase := st.dvPhase
			st.dvPhase = (st.dvPhase + 1) & 7
			return uint16(int32(st.baseFreq) + delayedVibratoMultiples[phase]*delayedVibratoStep(st.baseFreq))

		case 3: // holdGliss: hold baseFreq for holdFrames, then glissPtr steps one delta per frame
			if nf <= holdFrames {
				return st.baseFreq
			}
			// Advance glissPtr by one delta step if not exhausted.
			if st.glissPtr < st.glissEnd {
				delta := int16(rom[st.glissPtr]) | int16(rom[st.glissPtr+1])<<8
				st.glissFreq = uint16(int32(st.glissFreq) + int32(delta))
				st.glissPtr += 2
			}
			return st.glissFreq

		case 4: // headCycle: seqPtr walks head then wraps at cycle boundary
			// seqPtr and cycleBase/cycleEnd initialised in initNote; advance each frame.
			if nf > 0 {
				st.seqPtr += 2
				if st.seqPtr >= st.cycleEnd {
					st.seqPtr = st.cycleBase // wrap back to cycle start
				}
				st.seqFreq = uint16(rom[st.seqPtr]) | uint16(rom[st.seqPtr+1])<<8
			}
			return st.seqFreq
		}
		return st.baseFreq
	}

	// nptTogglePeriodAt reads the per-frame toggle period of a note's NPT sequence.
	nptTogglePeriodAt := func(st *chanState, nf int) uint16 {
		frameCount := int(rom[st.nptTogglePtr])
		if frameCount == 0xFF {
			return st.nptNoteToggle[min(nf, 1)]
		}
		frame := min(nf, frameCount-1)
		tpBase := st.nptTogglePtr + 1 + frame*2
		return uint16(rom[tpBase]) | uint16(rom[tpBase+1])<<8
	}

	// togglePeriodForSIDFreq scales the base CIA timer period when FM shifts the frequency.
	// On 6502: baseToggle is always 0 for NPT/env notes that have FM (verified from ROM data),
	// so the multiply path never fires in practice; the integer multiply is retained for
	// correctness and as a model for a 16×16→16 unsigned multiply+divide routine if needed.
	togglePeriodForSIDFreq := func(baseFreq, currentFreq, baseTogglePeriod uint16) uint16 {
		if baseFreq == 0 || currentFreq == 0 || baseTogglePeriod == 0 {
			return baseTogglePeriod
		}
		// Integer: (baseTogglePeriod * baseFreq) / currentFreq
		// Intermediate product fits in uint32.
		return uint16(uint32(baseTogglePeriod) * uint32(baseFreq) / uint32(currentFreq))
	}

	var frames []SIDFrame
	done := [3]bool{}
	var nptOccurrence [3]int

	for !done[0] || !done[1] || !done[2] {
		frame := SIDFrame{}

		for ch := 0; ch < 3; ch++ {
			st := &states[ch]

			// Fetch next note/rest when current one is exhausted
			for st.framesLeft == 0 {
				streamEnd := sectionEnd(13 + ch)
				if ramU16(zpStreamPtrLo, ch) >= streamEnd && ram[zpBlockCtr+ch] == 0 {
					done[ch] = true
					break
				}
				pid, isRest, restDurIdx := fetchNext(rom, ch, blockBase, blockLenBase, blockCount)
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
				initNote(st, np, ins, dur)

				if seqAddr, ok := nptWaveSeqFor[pid]; ok {
					st.nptWavePtr   = seqAddr
					st.nptTogglePtr = nptTogglePtrFor[pid]
					if rom[st.nptTogglePtr] == 0xFF {
						listStart := st.nptTogglePtr + 1
						entry := listStart + int(rom[listStart+nptOccurrence[ch]])
						st.nptNoteToggle[0] = uint16(rom[entry]) | uint16(rom[entry+1])<<8
						st.nptNoteToggle[1] = uint16(rom[entry+2]) | uint16(rom[entry+3])<<8
						nptOccurrence[ch]++
					}
				} else {
					st.nptWavePtr   = 0
					st.nptTogglePtr = 0
				}
			}

			if done[ch] {
				frame.Voices[ch][VFCtrlA] = 0
				continue
			}

			vf := &frame.Voices[ch]

			if st.isRest {
				vf[VFCtrlA] = 0
				frame.TogglePeriod[ch] = 0
				st.framesLeft--
				st.noteFrame++
				continue
			}

			nf := st.noteFrame

			// Frequency (incremental FM step)
			sidFreq := stepFM(st, nf)
			vf[VFFreqLo] = byte(sidFreq)
			vf[VFFreqHi] = byte(sidFreq >> 8)

			gateBit := byte(0x01)
			if st.envelopeMode {
				vf[VFCtrlA] = (st.wave << 4) | gateBit
				vf[VFCtrlB] = (0x05 << 4) | gateBit
			} else if st.noisePlusTone {
				nptState := byte(1) // default: mixed (noise+pulse)
				if st.nptWavePtr != 0 {
					byteLen := int(rom[st.nptWavePtr])
					nptState = (rom[st.nptWavePtr+1+nf/4] >> uint(2*(nf%4))) & 3
					if nf/4 >= byteLen {
						nptState = 1 // past end: hold mixed
					}
				}
				switch nptState {
				case 0: // tone-only
					vf[VFCtrlA] = (0x04 << 4) | gateBit
					vf[VFCtrlB] = (0x04 << 4) | gateBit
				case 2: // noise-only
					vf[VFCtrlA] = (0x08 << 4) | gateBit
					vf[VFCtrlB] = (0x08 << 4) | gateBit
				default: // mixed (1)
					vf[VFCtrlA] = (0x08 << 4) | gateBit
					vf[VFCtrlB] = (0x05 << 4) | gateBit
				}
			} else {
				vf[VFCtrlA] = (st.wave << 4) | gateBit
				vf[VFCtrlB] = 0
			}

			// Pulse width: 0x800 (50% duty) only while CtrlA's waveform is
			// pure pulse ($40); 0xFFF for every other waveform (matches the
			// player, which only needs 0x800 for an actual square wave).
			if vf[VFCtrlA]&0xF0 == 0x40 {
				vf[VFPWLo] = 0x00
				vf[VFPWHi] = 0x08
			} else {
				vf[VFPWLo] = 0xFF
				vf[VFPWHi] = 0x0F
			}

			if nf == 0 && st.hasPrevNote {
				if st.envelopeMode && st.prevEnvelopeMode {
					vf[VFGateTransition] = 1
				} else {
					vf[VFGateTransition] = 2
				}
			}

			// ADSR
			if st.tailFired {
				vf[VFAttDec] = (st.attack << 4) | st.decay
			} else {
				vf[VFAttDec] = (st.attack << 4) | st.initialDecay
				if nf == 1 && st.initialDecay > 0 {
					vf[VFAttDec] = st.attack << 4
				}
			}
			if st.tailStartFrame > 0 && nf == st.tailStartFrame {
				st.tailFired = true
				vf[VFAttDec] = (st.attack << 4) | st.decay
			}
			if nf > 0 && st.framesLeft <= framesBeforeNoteOff {
				vf[VFAttDec] &= 0xF0
			}

			// Sustain: advance bitstream by one frame (incremental)
			if nf > 0 && st.sustainEnd > 0 {
				stepSustain(st)
			}
			vf[VFSusRel] = st.sustainVal << 4

			// CIA toggle period
			if st.envelopeMode {
				togglePeriod := togglePeriodForSIDFreq(st.baseFreq, sidFreq, st.baseToggle)
				if st.nptTogglePtr != 0 {
					togglePeriod = nptTogglePeriodAt(st, nf)
				}
				frame.TogglePeriod[ch] = togglePeriod
			} else if st.noisePlusTone {
				// CIA period is only audible during mixed frames (ctrlA!=ctrlB),
				// but non-mixed frames now carry forward the last mixed period
				// (ROM never stores a literal 0), so read the same way regardless.
				togglePeriod := togglePeriodForSIDFreq(st.baseFreq, sidFreq, st.baseToggle)
				if st.nptTogglePtr != 0 {
					togglePeriod = nptTogglePeriodAt(st, nf)
				}
				frame.TogglePeriod[ch] = togglePeriod
			} else if ch != 2 {
				// Plain instrument: CtrlA==CtrlB, so the CIA period is
				// immaterial to sound — but the player now always arms
				// the timer with the real ROM-stored period (never a
				// literal 0), so the reference must report the same.
				// ch2 has no CIA hardware at all (only CIA1 Timer A/B
				// exist, for ch0/ch1), so it stays unarmed regardless.
				frame.TogglePeriod[ch] = st.baseToggle
			}

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
			// The CIA period is not overridden here: SimulateTrackerFromROM already reads
			// the per-frame CIA periods from the ROM's NPT toggle table, which matches
			// what the 6502 player reads.
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
	const numSections = 20
	sectionOff := [numSections]int{}
	for i := range sectionOff {
		sectionOff[i] = int(uint16(rom[i*2]) | uint16(rom[i*2+1])<<8)
	}
	freqModCount := int(rom[40])
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
	sustainBase    := sectionOff[6]
	instrumentBase := sectionOff[7]
	durTableBase   := sectionOff[8]
	restDurBase    := sectionOff[9]
	notePatBase    := sectionOff[10]
	blockBase      := sectionOff[12]
	fmOffsetsBase  := sectionOff[16]
	blockLenBase   := sectionOff[17]

	notePatCount       := (sectionEnd(10) - notePatBase - freqModCount) / 2
	notePatFreqIdxBase := notePatBase
	notePatPackedBase  := notePatBase + notePatCount
	notePatFreqModBase := notePatBase + notePatCount*2

	blockCount := sectionEnd(17) - blockLenBase

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
			wave:              merged >> 4,
			flags:             (merged>>1)&1 | (merged>>2)&1<<1,
			tail:              rom[base+2],
			attack:            (ds >> 4) & 1,
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
		return freqModBase + int(rom[fmOffsetsBase+int(i)])
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
	dvFor := func(freq uint16) DelayedVibratoTemplate {
		var t DelayedVibratoTemplate
		for j, m := range delayedVibratoMultiples { t.Deltas[j] = int16(m * delayedVibratoStep(freq)) }
		return t
	}
	nptBase := sectionOff[18]
	toneSeqFor := func(pid byte) (FreqSequenceTemplate, bool) {
		for p := sectionOff[19]; p < sectionEnd(19); p += 2 {
			if rom[p] != pid { continue }
			countAddr := nptBase + int(rom[p+1])
			countAddr += 1 + int(rom[countAddr])
			frameCount := int(rom[countAddr])
			if frameCount == 0xFF { return FreqSequenceTemplate{}, false }
			freqs := make([]uint16, frameCount)
			for f := range freqs { freqs[f] = uint16(rom[countAddr+1+f*2]) | uint16(rom[countAddr+2+f*2])<<8 }
			return FreqSequenceTemplate{Frequencies: freqs}, true
		}
		return FreqSequenceTemplate{}, false
	}
	loadSustainVolumes := func(note *SimulatedNote, sustainIdx byte) {
		if sustainIdx == 0xFF { return }
		p := sustainBase + int(sustainIdx)
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
			pid, isRest, restDurIdx := fetchNext(rom, ch, blockBase, blockLenBase, blockCount)
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
				if fm.Type == 7 { dvTmpl = dvFor(baseFreq) } else if fm.Type == 5 { seqTmpl = seqTemplateFromFM(fm) }
			}
			var toneFreqMod FreqModPattern
			var toneSeqTmpl FreqSequenceTemplate
			if tmpl, ok := toneSeqFor(pid); ok && ins.flags&1 != 0 {
				toneSeqTmpl = tmpl; toneFreqMod = FreqModPattern{Type: 5}
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

// delayedVibratoMultiples scales delayedVibratoStep into the delta from the base
// frequency at each of the eight vibrato phases.
var delayedVibratoMultiples = [8]int32{1, 2, 1, 0, -1, -2, -1, 0}

// delayedVibratoStep is the vibrato depth for a base frequency: a fixed AY period
// swing makes the SID frequency swing grow with the square of the frequency. The
// player computes the same value from the top eight bits of the frequency.
func delayedVibratoStep(freq uint16) int32 {
	top := uint16(byte(freq >> 7))
	scaled := top + top>>4
	return int32(scaled * scaled >> 6)
}
