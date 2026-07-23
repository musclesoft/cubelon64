package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// fieldName maps a VFxxx index to a human-readable name.
var sidFieldName = [9]string{
	"FreqLo", "FreqHi", "PWLo", "PWHi", "CtrlA", "CtrlB", "AttDec", "SusRel", "GateTrans",
}

// CompareSIDFrames compares the Go-simulated sidFrames against the 6502-emulated
// playerFrames field by field. Fields that are known to diverge legitimately between
// the two paths (CtrlB, GateTransition — CIA phase is not tracked in playerFrames yet)
// are skipped and noted. Reports pass/fail per channel; panics on first comparison error
// that exceeds maxMismatches total.
func CompareSIDFrames(sidFrames, playerFrames []SIDFrame, startOffset, maxMismatches int) {
	fmt.Printf("\n=== Comparing sidFrames vs 6502 playerFrames ===\n")

	// Fields to compare (indices into SIDVoiceFrame), excluding CtrlA which
	// gets its own alternation-aware check below.
	// CtrlB and GateTransition are Go-only fields, not written to SID hardware.
	compareFields := []int{VFFreqLo, VFFreqHi, VFPWLo, VFPWHi, VFAttDec, VFSusRel}

	total := len(sidFrames)
	if len(playerFrames) < total {
		total = len(playerFrames)
	}

	// Fields to skip during rests (CtrlA==0): SID hardware retains stale freq/PW/ADSR.
	restSkipFields := map[int]bool{VFFreqLo: true, VFFreqHi: true, VFPWLo: true, VFPWHi: true, VFAttDec: true, VFSusRel: true}

	totalMismatches := 0
	for ch := 0; ch < 3; ch++ {
		chMismatches := 0
		firstMiss := -1
		for f := startOffset; f < total; f++ {
			// Registers compare against the PREVIOUS reference frame: the
			// player's one-tick pipeline means SID hardware at frame f
			// reflects what compute_next_frame staged during frame f-1,
			// not frame f's own values (which only land on hardware at
			// the start of frame f+1). Frame 0 has nothing staged yet —
			// compare against silence, matching post-player_init state.
			var sf SIDVoiceFrame
			if f > 0 {
				sf = sidFrames[f-1].Voices[ch]
			}
			pf := playerFrames[f].Voices[ch]
			simRest := sf[VFCtrlA] == 0
			for _, fi := range compareFields {
				if simRest && restSkipFields[fi] {
					continue
				}
				if sf[fi] != pf[fi] {
					chMismatches++
					totalMismatches++
					if firstMiss < 0 {
						firstMiss = f
					}
					if chMismatches <= 8 {
						fmt.Printf("  ch%d frame %d %s: sim=$%02X 6502=$%02X\n",
							ch, f, sidFieldName[fi], sf[fi], pf[fi])
					}
				}
			}
			// CtrlA check: when CIA alternation is active this frame (CtrlB set
			// and differs from CtrlA), the 6502 player now picks whichever phase
			// the live CIA toggle bit is actually in — this per-frame reference
			// model has no visibility into that sub-frame timing, so either
			// value is a legitimate pass. Only flag a mismatch if the player's
			// CtrlA write matches neither phase.
			if !simRest {
				alternating := sf[VFCtrlB] != 0 && sf[VFCtrlB] != sf[VFCtrlA]
				var ok bool
				if alternating {
					ok = pf[VFCtrlA] == sf[VFCtrlA] || pf[VFCtrlA] == sf[VFCtrlB]
				} else {
					ok = pf[VFCtrlA] == sf[VFCtrlA]
				}
				if !ok {
					chMismatches++
					totalMismatches++
					if firstMiss < 0 {
						firstMiss = f
					}
					if chMismatches <= 8 {
						fmt.Printf("  ch%d frame %d CtrlA: sim=$%02X (altB=$%02X) 6502=$%02X\n",
							ch, f, sf[VFCtrlA], sf[VFCtrlB], pf[VFCtrlA])
					}
				}
			}
			// TogglePeriod comparison (skip during rests — CIA is not
			// running). NOT shifted like Voices above: player_6502.go
			// samples PLAYER_TOGGLE_LO/HI right after player_tick returns,
			// which is compute_next_frame's THIS-tick staged value (the
			// period apply_toggle_period will commit to hardware next
			// tick) — i.e. it's already aligned with sidFrames[f], the
			// same-tick reference value, not sidFrames[f-1].
			curRest := sidFrames[f].Voices[ch][VFCtrlA] == 0
			if !curRest && sidFrames[f].TogglePeriod[ch] != playerFrames[f].TogglePeriod[ch] {
				chMismatches++
				totalMismatches++
				if firstMiss < 0 {
					firstMiss = f
				}
				if chMismatches <= 8 {
					fmt.Printf("  ch%d frame %d TogglePeriod: sim=%d 6502=%d\n",
						ch, f, sidFrames[f].TogglePeriod[ch], playerFrames[f].TogglePeriod[ch])
				}
			}
		}
		if chMismatches == 0 {
			fmt.Printf("  ch%d: ✓ %d frames match\n", ch, total-startOffset)
		} else {
			fmt.Printf("  ch%d: ✗ %d mismatches (first at frame %d)\n", ch, chMismatches, firstMiss)
		}
	}

	if totalMismatches == 0 {
		fmt.Printf("  All channels: ✓ pass\n")
	} else if totalMismatches > maxMismatches {
		panic(fmt.Sprintf("CompareSIDFrames: %d mismatches exceed threshold %d", totalMismatches, maxMismatches))
	} else {
		fmt.Printf("  Total mismatches: %d (within threshold %d)\n", totalMismatches, maxMismatches)
	}
}

// CompareSIDWrites compares two SID write logs byte-for-byte and prints the
// first divergence with surrounding context. Returns true if they match.
func CompareSIDWrites(refWrites, gotWrites []SIDWrite) bool {
	if len(refWrites) != len(gotWrites) {
		fmt.Printf("  SID write count mismatch: ref=%d got=%d\n", len(refWrites), len(gotWrites))
	}
	limit := len(refWrites)
	if len(gotWrites) < limit {
		limit = len(gotWrites)
	}
	for i := 0; i < limit; i++ {
		r, g := refWrites[i], gotWrites[i]
		if r.Addr != g.Addr || r.Value != g.Value || r.Frame != g.Frame {
			start := i - 8
			if start < 0 {
				start = 0
			}
			end := i + 8
			if end > limit {
				end = limit
			}
			fmt.Printf("  SID write mismatch at index %d:\n", i)
			for j := start; j < end; j++ {
				mark := " "
				if j == i {
					mark = "*"
				}
				rj, gj := refWrites[j], gotWrites[j]
				fmt.Printf("    %s[%d] f%d: ref $%04X=$%02X  got $%04X=$%02X\n",
					mark, j, rj.Frame, rj.Addr, rj.Value, gj.Addr, gj.Value)
			}
			return false
		}
	}
	if len(refWrites) != len(gotWrites) {
		return false
	}
	return true
}

// ValidateGateResetSequence checks that the 6502 player actually performs the
// sub-frame gate-off/gate-on (GateReset) or test-bit pulse (TestPulse) cycle
// that sidFrames' GateTransition field records — a check CompareSIDFrames
// cannot make, since it only samples one final register snapshot per frame
// and can't see whether a reset happened before that snapshot was taken.
func ValidateGateResetSequence(sidFrames []SIDFrame, playerWrites []SIDWrite, startOffset int) {
	// Index player SID_CTRL writes by (channel, frame) for quick lookup.
	type key struct {
		ch    int
		frame uint32
	}
	ctrlWritesByFrame := make(map[key][]byte)
	for _, w := range playerWrites {
		for ch := 0; ch < 3; ch++ {
			if w.Addr == 0xD400+uint16(ch*7)+4 {
				k := key{ch, w.Frame}
				ctrlWritesByFrame[k] = append(ctrlWritesByFrame[k], w.Value)
			}
		}
	}

	totalViolations := 0
	for ch := 0; ch < 3; ch++ {
		violations := 0
		checked := 0
		for f := startOffset; f < len(sidFrames); f++ {
			transition := sidFrames[f].Voices[ch][VFGateTransition]
			if transition == 0 {
				continue
			}
			checked++
			// The actual SID_CTRL writes land one tick later than the
			// Go reference's transition marker: apply_prev_frame replays
			// the queued gate-off/test-bit byte(s) plus the note's own
			// final ctrl value at the START of the tick AFTER the note
			// was fetched — see player.s's PLAYER_CTRL_PRE0/1 queue.
			writes := ctrlWritesByFrame[key{ch, uint32(f + 1)}]
			ok := false
			switch transition {
			case 2: // GateReset: expect gate 1→0→1 within this frame
				sawGateOff := false
				for i, v := range writes {
					if v&0x01 == 0 {
						sawGateOff = true
					}
					if i == len(writes)-1 && v&0x01 == 1 && sawGateOff {
						ok = true
					}
				}
			case 1: // TestPulse: test bit pulsed 0→1→0 on the *current* register
				// (gate untouched — may be 0 if a rest preceded this note, since
				// the reference model doesn't distinguish that case either),
				// followed by the new note's own ctrl write (last, gate=1).
				if len(writes) >= 3 {
					pulse := writes[:len(writes)-1]
					last := writes[len(writes)-1]
					gate := pulse[0] & 0x01
					sawTestOn := false
					consistent := true
					for _, v := range pulse {
						if v&0x01 != gate {
							consistent = false // gate changed mid-pulse: invalid
							break
						}
						if v&0x08 != 0 {
							sawTestOn = true
						}
					}
					lastHadNoTestBit := last&0x08 == 0
					lastGateOn := last&0x01 == 1
					ok = consistent && sawTestOn && lastHadNoTestBit && lastGateOn
				}
			}
			if !ok {
				violations++
				if violations <= 5 {
					fmt.Printf("  ch%d frame %d: expected transition=%d sequence, got writes=% 02X\n",
						ch, f, transition, writes)
				}
			}
		}
		if violations > 0 {
			fmt.Printf("Channel %d: ✗ %d/%d gate reset sequence violations\n", ch, violations, checked)
			totalViolations += violations
		} else if checked > 0 {
			fmt.Printf("Channel %d: ✓ %d gate reset sequences validated\n", ch, checked)
		}
	}
	if totalViolations > 0 {
		panic(fmt.Sprintf("gate reset sequence validation failed: %d violations", totalViolations))
	}
}

// runValidate is the -validate mode: reads tracker_format.rom, runs the Go
// simulation as reference, assembles and runs the 6502 player, then compares
// frame-by-frame. Exits 0 on pass, 1 on fail.
func runValidate() {
	fmt.Printf("=== Validate 6502 player against tracker_format.rom ===\n\n")

	rom, err := os.ReadFile("tracker_format.rom")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read tracker_format.rom: %v\n", err)
		os.Exit(1)
	}

	// Reference: Go simulation of the tracker format.
	refFrames := SimulateTrackerFromROM(rom)
	fmt.Printf("Reference: %d frames\n", len(refFrames))

	// Regenerate music_data.inc from the ROM so it matches the player's section list.
	writeTrackerASM(rom, "../c64port/src/music_data.inc")

	// Player: assemble + run 6502 player.
	prgPath, metaPath, err := assemblePlayerHarness()
	if err != nil {
		fmt.Fprintf(os.Stderr, "assembly failed: %v\n", err)
		os.Exit(1)
	}
	_ = metaPath

	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading meta: %v\n", err)
		os.Exit(1)
	}
	var meta playerMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		fmt.Fprintf(os.Stderr, "parsing meta: %v\n", err)
		os.Exit(1)
	}

	sym := func(name string) uint16 {
		v, ok := meta.Symbols[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "symbol %q not found\n", name)
			os.Exit(1)
		}
		return v
	}
	addrInit  := sym("player_init")
	addrTick  := sym("player_tick")
	addrTogLo := sym("PLAYER_TOGGLE_LO")
	addrTogHi := sym("PLAYER_TOGGLE_HI")
	addrFrame := sym("PLAYER_FRAME")

	prgBytes, err := os.ReadFile(prgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading PRG: %v\n", err)
		os.Exit(1)
	}
	loadAddr := uint16(prgBytes[0]) | uint16(prgBytes[1])<<8

	cpu := newCPU6502()
	copy(cpu.Memory[loadAddr:], prgBytes[2:])
	cpu.cpu6502Call(addrInit)

	totalFrames := len(refFrames)
	playerFrames := make([]SIDFrame, totalFrames)

	for f := 0; f < totalFrames; f++ {
		cpu.CurrentFrame = f
		cpu.cpu6502Call(addrTick)
		var sf SIDFrame
		for ch := 0; ch < 3; ch++ {
			base := uint16(ch * 7)
			vf := &sf.Voices[ch]
			vf[VFFreqLo] = cpu.Memory[0xD400+base+0]
			vf[VFFreqHi] = cpu.Memory[0xD400+base+1]
			vf[VFPWLo]   = cpu.Memory[0xD400+base+2]
			vf[VFPWHi]   = cpu.Memory[0xD400+base+3]
			vf[VFCtrlA]  = cpu.Memory[0xD400+base+4]
			vf[VFCtrlB]  = cpu.Memory[addrFrame+uint16(ch*9+5)]
			vf[VFAttDec] = cpu.Memory[0xD400+base+5]
			vf[VFSusRel] = cpu.Memory[0xD400+base+6]
		}
		for ch := 0; ch < 2; ch++ {
			sf.TogglePeriod[ch] = uint16(cpu.Memory[addrTogLo+uint16(ch)]) |
				uint16(cpu.Memory[addrTogHi+uint16(ch)])<<8
		}
		playerFrames[f] = sf
	}
	fmt.Printf("Player:    %d frames, %d SID writes\n\n", totalFrames, len(cpu.SIDWrites))

	CompareSIDFrames(refFrames, playerFrames, 0, 0)
}
