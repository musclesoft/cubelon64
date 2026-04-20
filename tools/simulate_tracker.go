// Tracker replay simulation - generates SID register writes from tracker format
package main

import (
	"fmt"

	"cubelon-emu/spectrum"
)

// SimulatedNote represents the SID register state for one note
//
// ARCHITECTURE: This contains ALL data the tracker player needs for playback.
// Each field is populated by the simulator from the tracker format.
//
// FREQUENCY DATA (freqSeq):
//   - Envelope mode: contains envelope periods (R11-R12) read by extractor
//   - Noise mode: contains noise periods (R6, 5-bit values) read by extractor
//   - Tone mode: contains tone periods (R0-R5) read by extractor
//   - May include glissando/vibrato patterns
//
// WAVEFORM DATA (waveformSeq):
//   - Envelope mode: contains waveform changes from R13 (envelope shape) register
//   - NoisePlusTone mode: contains waveform changes from R7 (mixer) register
//   - Each entry is 0x01 (triangle), 0x02 (sawtooth), 0x04 (pulse), or 0x08 (noise)
//
// TODO: Extractor must populate waveformSeq from R13 and R7 register changes
type SimulatedNote struct {
	freq            uint16
	freqSeq         []uint16 // Per-frame frequencies (AY periods: envelope/noise/tone)
	Waveform        byte     // Initial waveform for note
	waveformSeq     []byte   // Per-frame waveforms (from R13/mixer changes)
	dur             uint16
	ADSR            ADSRParams
	SustainPerFrame []uint8  // Dynamic S parameter per frame (volume-following)
	GateOffFrame    int      // Frame to switch D from 0 to ADSR.Decay (0 = no tail decay)
	EnvelopeMode    bool
	NoisePlusTone   bool     // If true, note uses noise+tone mixing (waveform switches based on mixer)
	isRest          bool     // True for silence/rest patterns
}

// SimulateTracker replays the tracker format and generates notes per channel
//
// ARCHITECTURE: This is the DECODER/SIMULATOR - it unpacks the compressed tracker format
// into expanded SimulatedNote structures that the player can easily process.
//
// RESPONSIBILITIES:
// - Decode bytecode streams (blocks + direct patterns) into note sequences
// - Expand patterns into full SimulatedNote structures with all per-frame data
// - Load frequency sequences, waveform sequences, sustain patterns from the format
// - Apply vibrato/glissando patterns to generate per-frame frequencies
//
// OUTPUT: SimulatedNote arrays containing ALL playback data (no AY commands needed)
func SimulateTracker(format *TrackerFormat) [3][]SimulatedNote {
	fmt.Printf("\n=== Simulating Tracker Replay ===\n")

	var simulatedChannels [3][]SimulatedNote

	for ch := 0; ch < 3; ch++ {
		notes := []SimulatedNote{}
		stream := format.Streams[ch]

		// Decode bytecode stream
		// Encoding: 0x00-0x1F = blocks 0-31, 0x20+ = direct patterns (patternID = opcode - 0x20)
		patternIDs := []byte{}
		i := 0
		blockRefs := 0
		directRefs := 0
		for i < len(stream) {
			opcode := stream[i]
			i++

			if opcode < 0x20 {
				// Block reference (0x00-0x1F)
				if int(opcode) < len(format.Blocks) {
					block := format.Blocks[opcode]
					patternIDs = append(patternIDs, block.Patterns...)
					blockRefs++
				}
			} else {
				// Direct pattern reference (0x20+ maps to pattern 0+)
				patternID := opcode - 0x20
				patternIDs = append(patternIDs, patternID)
				directRefs++
			}
		}

		if ch == 0 {
			fmt.Printf("  Channel %d: decoded %d block refs + %d direct refs = %d pattern IDs from %d bytecode bytes\n",
				ch, blockRefs, directRefs, len(patternIDs), len(stream))
		}

		// Process pattern IDs to create notes
		for _, patternID := range patternIDs {
			if int(patternID) >= len(format.NotePatterns) {
				continue
			}
			pattern := format.NotePatterns[patternID]

				// Duration is already in frames (not quantized to ticks)
				frameDuration := uint16(pattern.Duration)

				// Debug: track first few notes of each type
				if len(notes) < 3 {
					freqSeqType := "constant"
					if pattern.FreqSeqIndex == 0xFE {
						freqSeqType = "vibrato"
					} else if pattern.FreqSeqIndex != 0xFF {
						if int(pattern.FreqSeqIndex) < len(format.GlissandoPatterns) {
							gliss := format.GlissandoPatterns[pattern.FreqSeqIndex]
							freqSeqType = fmt.Sprintf("glissando[%d] start=%d deltas=%d", pattern.FreqSeqIndex, gliss.StartFreq, len(gliss.Deltas))
						}
					}
					fmt.Printf("DEBUG ch=%d note=%d: dur=%d freqIdx=%d type=%s\n",
						ch, len(notes), frameDuration, pattern.FreqIdx, freqSeqType)
				}

				// Check if this is a rest pattern (FreqIdx == 255)
				if pattern.FreqIdx == 255 {
					// Create rest note (silence)
					note := SimulatedNote{
						freq:     0,
						Waveform: 0,
						dur:      frameDuration,
						isRest:   true,
					}
					notes = append(notes, note)
					continue
				}

				instrument := format.Instruments[pattern.InstrumentID]
				baseFreq := format.FreqTable[pattern.FreqIdx]

				// Build the simulated note
				note := SimulatedNote{
					freq:          baseFreq,
					Waveform:      instrument.BaseWaveform,
					dur:           frameDuration,
					EnvelopeMode:  instrument.EnvelopeMode,
					NoisePlusTone: instrument.NoisePlusTone,
					GateOffFrame:  int(pattern.GateOffFrame),
					isRest:        false,
					ADSR: ADSRParams{
						Attack:       instrument.ADSR.Attack,
						Decay:        instrument.ADSR.Decay,
						Sustain:      instrument.ADSR.Sustain,
						Release:      instrument.ADSR.Release,
						InitialDecay: instrument.ADSR.InitialDecay,
					},
				}

				// Check frequency modulation type
				note.freqSeq = make([]uint16, frameDuration)

				if pattern.FreqSeqIndex != 0xFF && pattern.FreqSeqIndex != 0xFE && int(pattern.FreqSeqIndex) < len(format.GlissandoPatterns) {
					// Glissando: apply per-frame deltas sequentially
					gliss := format.GlissandoPatterns[pattern.FreqSeqIndex]

					currentPeriod := int(gliss.StartFreq)
					note.freqSeq[0] = uint16(currentPeriod)

					// Apply deltas frame-by-frame
					for i := uint16(1); i < frameDuration; i++ {
						if int(i-1) < len(gliss.Deltas) {
							currentPeriod += int(gliss.Deltas[i-1])
						}
						// Clamp to valid AY period range
						if currentPeriod < 1 {
							currentPeriod = 1
						}
						if currentPeriod > 4095 {
							currentPeriod = 4095
						}
						note.freqSeq[i] = uint16(currentPeriod)
					}
				} else if pattern.FreqSeqIndex == 0xFE {
					// Vibrato: apply global pattern to base frequency (FreqSeqIndex = 0xFE)
					vib := format.VibratoPattern

					for i := uint16(0); i < frameDuration; i++ {
						freq := baseFreq

						// Apply vibrato after delay
						if int(i) >= vib.DelayFrames {
							// Calculate offset from vibrato cycle
							cyclePos := (int(i) - vib.DelayFrames) % len(vib.OffsetTable)
							offset := int(vib.OffsetTable[cyclePos])
							// Apply offset to AY tone period
							freq = uint16(int(baseFreq) + offset)
						}

						note.freqSeq[i] = freq
					}
				} else {
					// Constant frequency (no modulation)
					for i := range note.freqSeq {
						note.freqSeq[i] = baseFreq
					}
				}

				// Generate per-frame waveform sequence (for envelope mode)
				note.waveformSeq = generateWaveformSequence(
					instrument.BaseWaveform,
					instrument.EnvelopeMode,
					frameDuration,
				)

				// Load sustain pattern (dynamic S values for volume-following)
				if pattern.SustainPatternIndex != 0xFF && int(pattern.SustainPatternIndex) < len(format.SustainPatterns) {
					sustainPattern := format.SustainPatterns[pattern.SustainPatternIndex]
					note.SustainPerFrame = make([]uint8, frameDuration)

					// Copy sustain values, repeat last value if pattern is shorter than note duration
					for i := uint16(0); i < frameDuration; i++ {
						if int(i) < len(sustainPattern.Values) {
							note.SustainPerFrame[i] = sustainPattern.Values[i]
						} else if len(sustainPattern.Values) > 0 {
							// Pad with last value
							note.SustainPerFrame[i] = sustainPattern.Values[len(sustainPattern.Values)-1]
						}
					}
				}

			notes = append(notes, note)
		}

		// Debug: print first real note details
		for i, note := range notes {
			if !note.isRest {
				fmt.Printf("DEBUG ch=%d first real note #%d: freq=%d freqSeqLen=%d waveform=0x%02x\n",
					ch, i, note.freq, len(note.freqSeq), note.Waveform)
				if len(note.freqSeq) > 0 {
					fmt.Printf("  freqSeq[0-4]: %d %d %d %d %d\n",
						note.freqSeq[0],
						note.freqSeq[min(1, len(note.freqSeq)-1)],
						note.freqSeq[min(2, len(note.freqSeq)-1)],
						note.freqSeq[min(3, len(note.freqSeq)-1)],
						note.freqSeq[min(4, len(note.freqSeq)-1)])
				}
				break
			}
		}

		simulatedChannels[ch] = notes
	}

	fmt.Printf("Generated %d + %d + %d = %d notes across 3 channels\n",
		len(simulatedChannels[0]), len(simulatedChannels[1]), len(simulatedChannels[2]),
		len(simulatedChannels[0])+len(simulatedChannels[1])+len(simulatedChannels[2]))

	return simulatedChannels
}

func generateWaveformSequence(baseWaveform byte, envelopeMode bool, duration uint16) []byte {
	waveformSeq := make([]byte, duration)

	// All frames use the base waveform
	// Envelope mode is a flag that tells the player to:
	// 1. Use envelope period for frequency (not tone period)
	// 2. Gate the note when envelope period changes (not every frame)
	// The player reads the AY envelope period register and gates only on changes
	for i := range waveformSeq {
		waveformSeq[i] = baseWaveform
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
