// Tracker replay simulation - generates SID register writes from tracker format
package main

import (
	"fmt"

	"cubelon-emu/spectrum"
)

// SimulatedNote represents the SID register state for one note
type SimulatedNote struct {
	freq         uint16
	freqSeq      []uint16 // Per-frame frequencies (including vibrato)
	Waveform     byte
	waveformSeq  []byte   // Per-frame waveforms (for envelope mode toggling)
	dur          uint16
	ADSR         ADSRParams
	EnvelopeMode bool
	isRest       bool     // True for silence/rest patterns
}

// SimulateTracker replays the tracker format and generates notes per channel
func SimulateTracker(format *TrackerFormat) [3][]SimulatedNote {
	fmt.Printf("\n=== Simulating Tracker Replay ===\n")

	var simulatedChannels [3][]SimulatedNote

	for ch := 0; ch < 3; ch++ {
		notes := []SimulatedNote{}
		orderList := format.Orders[ch]

		for _, phraseIdx := range orderList {
			phrase := format.Phrases[phraseIdx]

			for _, patternID := range phrase.Notes {
				pattern := format.NotePatterns[patternID]

				// Check if this is a rest pattern (FreqIdx == 255)
				if pattern.FreqIdx == 255 {
					// Create rest note (silence)
					note := SimulatedNote{
						freq:     0,
						Waveform: 0,
						dur:      uint16(pattern.Duration),
						isRest:   true,
					}
					notes = append(notes, note)
					continue
				}

				instrument := format.Instruments[pattern.InstrumentID]
				baseFreq := format.FreqTable[pattern.FreqIdx]

				// Build the simulated note
				note := SimulatedNote{
					freq:         baseFreq,
					Waveform:     instrument.BaseWaveform,
					dur:          uint16(pattern.Duration),
					EnvelopeMode: instrument.EnvelopeMode,
					isRest:       false,
					ADSR: ADSRParams{
						Attack:       instrument.ADSR.Attack,
						Decay:        instrument.ADSR.Decay,
						Sustain:      instrument.ADSR.Sustain,
						Release:      instrument.ADSR.Release,
						InitialDecay: instrument.ADSR.InitialDecay,
					},
				}

				// Check if this pattern has a frequency sequence
				if pattern.FreqSeqIndex >= 0 && int(pattern.FreqSeqIndex) < len(format.FreqSequences) {
					// Use stored frequency sequence
					note.freqSeq = format.FreqSequences[pattern.FreqSeqIndex]
				} else {
					// Constant frequency (no vibrato or glissando)
					note.freqSeq = make([]uint16, pattern.Duration)
					for i := range note.freqSeq {
						note.freqSeq[i] = baseFreq
					}
				}

				// Generate per-frame waveform sequence (for envelope mode)
				note.waveformSeq = generateWaveformSequence(
					instrument.BaseWaveform,
					instrument.EnvelopeMode,
					pattern.Duration,
				)

				notes = append(notes, note)
			}
		}

		simulatedChannels[ch] = notes
	}

	fmt.Printf("Generated %d + %d + %d = %d notes across 3 channels\n",
		len(simulatedChannels[0]), len(simulatedChannels[1]), len(simulatedChannels[2]),
		len(simulatedChannels[0])+len(simulatedChannels[1])+len(simulatedChannels[2]))

	return simulatedChannels
}

func generateWaveformSequence(baseWaveform byte, envelopeMode bool, duration byte) []byte {
	waveformSeq := make([]byte, duration)

	if !envelopeMode {
		// No envelope mode - all frames use base waveform
		for i := range waveformSeq {
			waveformSeq[i] = baseWaveform
		}
		return waveformSeq
	}

	// Envelope mode: alternate between 0x05 (pulse+triangle) and base waveform
	// This creates the envelope gating effect
	// The actual toggle happens at audio rate in the player, but we mark
	// frames where toggling is active with alternating waveforms
	for frame := 0; frame < int(duration); frame++ {
		// Alternate every frame (the actual toggling is per-sample in the player)
		if frame%2 == 0 {
			waveformSeq[frame] = 0x05 // pulse+triangle (modulation waveform)
		} else {
			waveformSeq[frame] = baseWaveform
		}
	}

	return waveformSeq
}

// CompareNotes compares original Note array and simulated SimulatedNote array for validation
func CompareNotes(commands []spectrum.AYCommand, original [3][]Note, simulated [3][]SimulatedNote) {
	fmt.Printf("\n=== Comparing Original vs Simulated ===\n")

	totalMismatches := 0

	for ch := 0; ch < 3; ch++ {
		// Filter out rest patterns from simulated notes for comparison
		simulatedNonRest := []SimulatedNote{}
		totalRestFrames := 0
		for _, note := range simulated[ch] {
			if !note.isRest {
				simulatedNonRest = append(simulatedNonRest, note)
			} else {
				totalRestFrames += int(note.dur)
			}
		}

		if len(original[ch]) != len(simulatedNonRest) {
			fmt.Printf("Channel %d: Note count mismatch! Original=%d Simulated(non-rest)=%d\n",
				ch, len(original[ch]), len(simulatedNonRest))
			totalMismatches++
			continue
		}

		simulated[ch] = simulatedNonRest // Use filtered list for comparison

		// Build frequency table for this channel from AY commands
		toneLoReg := uint8(ch * 2)
		toneHiReg := uint8(ch*2 + 1)
		freqPerFrame := make(map[int]uint16)
		currentFreq := uint16(0)

		for _, cmd := range commands {
			frame := int(cmd.Frame)
			switch cmd.Register {
			case toneLoReg:
				currentFreq = (currentFreq & 0xFF00) | uint16(cmd.Value)
				freqPerFrame[frame] = currentFreq
			case toneHiReg:
				currentFreq = (currentFreq & 0x00FF) | (uint16(cmd.Value&0x0F) << 8)
				freqPerFrame[frame] = currentFreq
			}
		}

		channelMismatches := 0
		for i := 0; i < len(original[ch]); i++ {
			orig := original[ch][i]
			sim := simulated[ch][i]

			// Get original note base frequency
			origFreq := uint16(0)
			for frame := orig.StartFrame; frame <= orig.EndFrame; frame++ {
				if freq, ok := freqPerFrame[frame]; ok && freq > 0 {
					origFreq = freq
					break
				}
			}

			// Compare basic properties (only report if significant mismatch)
			if origFreq != sim.freq && origFreq != 0 && sim.freq != 0 {
				channelMismatches++
			}
			expectedDur := uint16(orig.EndFrame - orig.StartFrame + 1)
			if expectedDur != sim.dur {
				channelMismatches++
			}
			if orig.Waveform != sim.Waveform {
				channelMismatches++
			}
			if orig.EnvelopeMode != sim.EnvelopeMode {
				channelMismatches++
			}
			if orig.ADSR != sim.ADSR {
				channelMismatches++
			}
		}

		if channelMismatches > 0 {
			fmt.Printf("Channel %d: %d mismatches\n", ch, channelMismatches)
			totalMismatches += channelMismatches
		}
	}

	if totalMismatches == 0 {
		fmt.Printf("\n✓ All channels match perfectly!\n")
	} else {
		fmt.Printf("\n✗ Total mismatches: %d\n", totalMismatches)
	}
}
