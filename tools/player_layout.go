package main

// Zero-page memory layout for the 6502 tracker player.
// Per-channel fields use SoA layout: field[ch] = ZP_FIELD + ch, ch ∈ {0,1,2}.
// This mirrors LDA ZP_FIELD,X in the 6502 player with X = channel index.

const (
	zpStreamPtrLo = 0x02 // +ch: stream (or block body) read pointer, lo byte (ZP starts at $02)
	zpStreamPtrHi = 0x05 // +ch: stream (or block body) read pointer, hi byte
	zpReturnPtrLo = 0x08 // +ch: stream resume pointer lo (0 = reading from stream)
	zpReturnPtrHi = 0x0B // +ch: stream resume pointer hi
	zpBlockCtr    = 0x0E // +ch: opcodes remaining in current block (0 = reading from stream)
	zpDurCtr      = 0x11 // +ch: frames remaining in current note/rest (0 = fetch next)
)

// SID voice register offsets relative to voice base (voice N base = N*7).
// Used for sid.WriteRegister(regBase+RegXxx, value).
const (
	RegFreqLo = 0
	RegFreqHi = 1
	RegPWLo   = 2
	RegPWHi   = 3
	RegCtrl   = 4
	RegAttDec = 5
	RegSusRel = 6
)

// SIDVoiceFrame byte indices — NOT the same as RegXxx because CtrlB sits at index 5.
const (
	VFFreqLo        = 0
	VFFreqHi        = 1
	VFPWLo          = 2
	VFPWHi          = 3
	VFCtrlA         = 4 // CtrlA = hi-phase control (= SID ctrl register)
	VFCtrlB         = 5 // CtrlB = lo-phase control (0 = no alternation)
	VFAttDec        = 6
	VFSusRel        = 7
	VFGateTransition = 8
)

// SIDVoiceFrame holds the register state for one SID voice for one song frame,
// plus CIA toggle data and a test-pulse flag.
//
// Layout:
//
//	[0] FreqLo
//	[1] FreqHi
//	[2] PWLo
//	[3] PWHi
//	[4] CtrlA     — primary control byte (gate | waveform for CIA hi-phase)
//	[5] CtrlB     — alternate control byte for CIA lo-phase (0 = no alternation, same as CtrlA)
//	[6] AttDec
//	[7] SusRel
//	[8] GateTransition — 1=TestPulse (envelope adjacent notes): pulse test bit on prev ctrl byte
//	                     2=GateReset (all other note starts): gate=0 + 10 samples + gate=1
type SIDVoiceFrame [9]byte

// SIDFrame holds per-frame output for all three SID voices plus CIA toggle periods.
// TogglePeriod[ch] is the CIA timer reload value for voice ch (0 = CIA not running).
type SIDFrame struct {
	Voices       [3]SIDVoiceFrame
	TogglePeriod [3]uint16
}
