package main

import "fmt"

// replayToggleMismatchBaseline is the committed upper bound on audible frames whose number of
// CIA waveform toggles in the replay differs from the direct path's by more than one. It must
// not increase; lower it when the cause is fixed.
const replayToggleMismatchBaseline = 0

// ValidateReplayToggleCounts compares, on every frame where the ROM alternates two waveforms,
// how often the waveform actually toggled in the replay (the IRQ-driven player and its CIA
// timers) against the direct path. A count insensitive to the free-running timer phase still
// exposes wrong periods and late timer reloads. It needs capturedDirectToggleEvents.
func ValidateReplayToggleCounts(romFrames []SIDFrame, replayWrites []SIDWrite, startOffset int) {
	total := 0
	for ch := 0; ch < 2; ch++ {
		direct := map[int]int{}
		for _, sample := range capturedDirectToggleEvents[ch] {
			direct[int(float64(sample)*palFrameRate/44100.0)]++
		}
		replay := map[int]int{}
		ctrlAddr := uint16(0xD404 + ch*7)
		lastCtrl := byte(0)
		for _, write := range replayWrites {
			if write.Addr != ctrlAddr {
				continue
			}
			previous := lastCtrl
			lastCtrl = write.Value
			if previous&0x01 == 0 || write.Value&0x01 == 0 || previous == write.Value {
				continue
			}
			frame := int(write.Cycle/int64(sidCyclesPerFrame)) + startOffset
			songFrame := frame - startOffset
			if write.Cycle < 0 || songFrame >= len(romFrames) {
				continue
			}
			voice := romFrames[songFrame].Voices[ch]
			if write.Value == voice[VFCtrlA] || write.Value == voice[VFCtrlB] {
				replay[frame]++
			}
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
		total += mismatches
	}
	if total > replayToggleMismatchBaseline {
		panic(fmt.Sprintf("ValidateReplayToggleCounts: %d mismatching frames (baseline %d)", total, replayToggleMismatchBaseline))
	}
}
