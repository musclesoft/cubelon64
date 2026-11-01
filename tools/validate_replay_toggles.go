package main

import "fmt"

// replayToggleMismatchBaseline is the committed upper bound on audible frames whose number of
// CIA waveform toggles in the replay differs from the direct path's by more than one. It must
// not increase; lower it when the cause is fixed.
const replayToggleMismatchBaseline = 0

// replayToggleGapBaseline is the committed upper bound on replay waveform toggles that follow
// the previous toggle by less than replayToggleGapMin or more than replayToggleGapMax of the
// frame's toggle period. It must not increase; lower it when the cause is fixed.
const replayToggleGapBaseline = 0

const (
	replayToggleGapMax = 1.5
	replayToggleGapMin = 0.5
)

// ValidateReplayToggleCounts compares, on every frame where the ROM alternates two waveforms,
// how often the waveform actually toggled in the replay (the IRQ-driven player and its CIA
// timers) against the direct path. A count insensitive to the free-running timer phase still
// exposes wrong periods and late timer reloads. The count tolerates one toggle of difference, so
// a spurious toggle pair plus a dropped toggle goes unnoticed by it; the spacing of consecutive
// toggles, which must stay near the toggle period, catches that. It needs
// capturedDirectToggleEvents.
func ValidateReplayToggleCounts(romFrames []SIDFrame, replayWrites []SIDWrite, startOffset int) {
	total := 0
	totalGapViolations := 0
	for ch := 0; ch < 2; ch++ {
		direct := map[int]int{}
		for _, sample := range capturedDirectToggleEvents[ch] {
			direct[int(float64(sample)*palFrameRate/44100.0)]++
		}
		replay := map[int]int{}
		ctrlAddr := uint16(0xD404 + ch*7)
		lastCtrl := byte(0)
		lastToggleCycle := int64(-1)
		gapViolations := 0
		for _, write := range replayWrites {
			if write.Addr != ctrlAddr {
				continue
			}
			previous := lastCtrl
			lastCtrl = write.Value
			frame := int(write.Cycle/int64(sidCyclesPerFrame)) + startOffset
			songFrame := frame - startOffset
			if write.Cycle < 0 || songFrame >= len(romFrames) {
				continue
			}
			voice := romFrames[songFrame].Voices[ch]
			if !alternatesWaveforms(voice) {
				lastToggleCycle = -1
			}
			if previous&0x01 == 0 || write.Value&0x01 == 0 || previous == write.Value {
				continue
			}
			if write.Value != voice[VFCtrlA] && write.Value != voice[VFCtrlB] {
				lastToggleCycle = -1
				continue
			}
			replay[frame]++
			if lastToggleCycle >= 0 && voice[VFGateTransition] == 0 && alternatesWaveforms(voice) {
				gapRatio := float64(write.Cycle-lastToggleCycle) / float64(romFrames[songFrame].TogglePeriod[ch])
				if gapRatio < replayToggleGapMin || gapRatio > replayToggleGapMax {
					gapViolations++
					if gapViolations <= 5 {
						fmt.Printf("  ch%d abs frame %d: toggle %d cycles after the previous, period %d\n",
							ch, frame, write.Cycle-lastToggleCycle, romFrames[songFrame].TogglePeriod[ch])
					}
				}
			}
			lastToggleCycle = write.Cycle
		}

		compared, mismatches, shown := 0, 0, 0
		for songFrame, frame := range romFrames {
			voice := frame.Voices[ch]
			if voice[VFCtrlA]&0x01 == 0 || !alternatesWaveforms(voice) {
				continue
			}
			abs := songFrame + startOffset
			compared++
			// A test-pulse note start also ends with its own waveform write.
			expectedExtra := 0
			if voice[VFGateTransition] == 1 {
				expectedExtra = 1
			}
			difference := direct[abs] + expectedExtra - replay[abs]
			if difference >= -1 && difference <= 1 {
				continue
			}
			mismatches++
			if shown < 5 {
				shown++
				fmt.Printf("  ch%d abs frame %d: direct=%d toggles, replay=%d (ctrl %02X/%02X, period %d)\n",
					ch, abs, direct[abs], replay[abs], voice[VFCtrlA], voice[VFCtrlB], frame.TogglePeriod[ch])
			}
		}
		fmt.Printf("Channel %d: %d/%d audible alternating frames differ by more than one toggle\n", ch, mismatches, compared)
		fmt.Printf("Channel %d: %d replay toggles spaced outside %.1f-%.1f of the toggle period\n", ch, gapViolations, replayToggleGapMin, replayToggleGapMax)
		total += mismatches
		totalGapViolations += gapViolations
	}
	if total > replayToggleMismatchBaseline {
		panic(fmt.Sprintf("ValidateReplayToggleCounts: %d mismatching frames (baseline %d)", total, replayToggleMismatchBaseline))
	}
	if totalGapViolations > replayToggleGapBaseline {
		panic(fmt.Sprintf("ValidateReplayToggleCounts: %d badly spaced toggles (baseline %d)", totalGapViolations, replayToggleGapBaseline))
	}
}
