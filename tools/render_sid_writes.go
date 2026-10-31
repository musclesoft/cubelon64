package main

// renderSIDWrites plays a cycle-stamped SID write stream for one voice
// through a SIDEmulator and returns its samples, a per-sample track that is
// non-zero while the ADSR delay bug holds a note onset frozen (from the gate
// rising edge until the envelope first moves), and the song frames at which a
// gate rising edge left the envelope frozen by it.
// Each write lands on the first sample at or after its cycle; writes stamped
// before the song starts (init) land before the first sample. The first
// startOffset frames are silent, matching generateSIDFromFrames.
func renderSIDWrites(writes []SIDWrite, channel int, totalSamples int, startOffset int) ([]int16, []int16, []int) {
	const sampleRate = 44100.0
	cyclesPerSample := sidClockHz / sampleRate
	silentCycles := int64(startOffset) * int64(sidCyclesPerFrame)
	voiceBase := uint16(0xD400 + channel*7)

	samples := make([]int16, totalSamples)
	bugDebug := make([]int16, totalSamples)
	var onsetDelayFrames []int
	sid := NewSIDEmulator(sampleRate)
	next := 0
	checkOnset := false
	onsetOpen := false
	onsetLevel := uint8(0)

	for i := 0; i < totalSamples; i++ {
		sampleCycle := int64(float64(i) * cyclesPerSample)
		for next < len(writes) && writes[next].Cycle+silentCycles <= sampleCycle {
			write := writes[next]
			next++
			if write.Addr < voiceBase || write.Addr >= voiceBase+7 {
				continue
			}
			register := byte(write.Addr - voiceBase)
			wasGateOn := sid.Registers[RegCtrl]&0x01 != 0
			sid.WriteRegister(register, write.Value)
			if register == RegCtrl && !wasGateOn && write.Value&0x01 != 0 {
				checkOnset = true
				onsetOpen = true
				onsetLevel = sid.Voices[0].EnvelopeLevel
			}
		}
		sample := sid.GenerateSample(0)
		if onsetOpen {
			if sid.Registers[RegCtrl]&0x01 == 0 || sid.Voices[0].EnvelopeLevel != onsetLevel {
				onsetOpen = false
			} else if sid.Voices[0].ADSRDelayBugActive {
				bugDebug[i] = 32767
			}
		}
		if checkOnset {
			checkOnset = false
			if sid.Voices[0].ADSRDelayBugActive {
				onsetDelayFrames = append(onsetDelayFrames, int(sampleCycle/int64(sidCyclesPerFrame))-startOffset)
			}
		}
		if sampleCycle >= silentCycles {
			samples[i] = sample
		}
	}
	return samples, bugDebug, onsetDelayFrames
}
