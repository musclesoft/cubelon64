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

	// Fields to compare (indices into SIDVoiceFrame).
	// CtrlA is included — both paths should agree on the final ctrl value written per frame.
	// CtrlB and GateTransition are Go-only fields, not written to SID hardware.
	compareFields := []int{VFFreqLo, VFFreqHi, VFPWLo, VFPWHi, VFCtrlA, VFAttDec, VFSusRel}

	total := len(sidFrames)
	if len(playerFrames) < total {
		total = len(playerFrames)
	}

	totalMismatches := 0
	for ch := 0; ch < 3; ch++ {
		chMismatches := 0
		firstMiss := -1
		for f := startOffset; f < total; f++ {
			sf := sidFrames[f].Voices[ch]
			pf := playerFrames[f].Voices[ch]
			for _, fi := range compareFields {
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
			// TogglePeriod comparison
			if sidFrames[f].TogglePeriod[ch] != playerFrames[f].TogglePeriod[ch] {
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

	// Player: assemble + run 6502 player.
	// We need a TrackerFormat for writeTrackerASM to have run already — if
	// tracker_format.rom exists, music_data.inc was written in the same run.
	// Assemble the existing player_harness.s (music_data.inc is in c64port/src/).
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
