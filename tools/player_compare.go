package main

import "fmt"

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
