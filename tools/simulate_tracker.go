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
//   - Contains final SID frequency register values (ready to write)
//   - Extractor converts AY periods to SID frequencies
//   - Player just writes: SID[freq_lo] = freqSeq[frame] & 0xFF
//   - May include glissando/vibrato patterns
//
// WAVEFORM DATA (waveformSeq):
//   - Envelope mode: contains waveform changes from R13 (envelope shape) register
//   - NoisePlusTone mode: contains waveform changes from R7 (mixer) register
//   - Each entry is 0x01 (triangle), 0x02 (sawtooth), 0x04 (pulse), or 0x08 (noise)
//
// TODO: Extractor must populate waveformSeq from R13 and R7 register changes
type SimulatedNote struct {
	freq              uint16
	freqSeq           []uint16 // Per-frame frequencies (AY periods: envelope/noise/tone)
	Waveform          byte     // Initial waveform for note
	waveformSeq       []byte   // Per-frame waveforms (from R13/mixer changes)
	dur               uint16
	ADSR              ADSRParams
	SustainPerFrame   []uint8  // Dynamic S parameter per frame (volume-following)
	GateOffFrame      int      // Frame to switch D from 0 to ADSR.Decay (0 = no tail decay)
	TogglePeriod      uint16   // SID cycles for waveform toggle (constant, from freq table)
	TogglePeriodSeq   []uint16 // Per-frame toggle periods (for tone glissando, 0 = use constant)
	EnvelopeMode      bool
	NoisePlusTone     bool // If true, note uses noise+tone mixing (waveform switches based on mixer)
	isRest            bool // True for silence/rest patterns
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

				// Get toggle period (constant from frequency table or sequence for tone glissando)
				togglePeriod := uint16(0)
				var togglePeriodSeq []uint16

				if pattern.TogglePeriodSeqIndex != 0xFF && int(pattern.TogglePeriodSeqIndex) < len(format.TogglePeriodSequences) {
					// Varying toggle period (tone glissando): use sequence
					seq := format.TogglePeriodSequences[pattern.TogglePeriodSeqIndex]
					togglePeriodSeq = make([]uint16, frameDuration)
					for i := uint16(0); i < frameDuration; i++ {
						if int(i) < len(seq.Periods) {
							togglePeriodSeq[i] = seq.Periods[i]
						} else if len(seq.Periods) > 0 {
							// Pad with last value
							togglePeriodSeq[i] = seq.Periods[len(seq.Periods)-1]
						}
					}
				} else if int(pattern.FreqIdx) < len(format.TogglePeriods) {
					// Constant toggle period: use frequency table lookup
					togglePeriod = format.TogglePeriods[pattern.FreqIdx]
				}

				// Build the simulated note
				note := SimulatedNote{
					freq:            baseFreq,
					Waveform:        instrument.BaseWaveform,
					dur:             frameDuration,
					EnvelopeMode:    instrument.EnvelopeMode,
					NoisePlusTone:   instrument.NoisePlusTone,
					TogglePeriod:    togglePeriod,
					TogglePeriodSeq: togglePeriodSeq,
					GateOffFrame:    int(pattern.GateOffFrame),
					isRest:          false,
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

				if pattern.FreqSeqIndex != 0xFF && pattern.FreqSeqIndex != 0xFE {
					// Use 6502-optimized FreqModPatterns
					if int(pattern.FreqSeqIndex) < len(format.FreqModPatterns) {
						// Expand 6502-optimized pattern
						freqModPattern := format.FreqModPatterns[pattern.FreqSeqIndex]
						expandedSeq := expandFreqModPattern6502(freqModPattern, format, format.FreqTable, format.FreqSequenceTemplates, format.DrumSweepPattern)
						for i := uint16(0); i < frameDuration && int(i) < len(expandedSeq); i++ {
							note.freqSeq[i] = expandedSeq[i]
						}
						// Fill remaining frames
						if len(expandedSeq) > 0 {
							// Type 7 (DelayedVibrato) should cycle the oscillation
							if freqModPattern.Type == 7 && len(freqModPattern.Data) >= 4 {
								// Continue cycling the vibrato pattern
								baseFreq := uint16(freqModPattern.Data[0]) | (uint16(freqModPattern.Data[1]) << 8)
								holdFrames := int(freqModPattern.Data[2])
								templateIdx := int(freqModPattern.Data[3])
								if templateIdx < len(format.DelayedVibratoTemplates) {
									template := format.DelayedVibratoTemplates[templateIdx]
									for i := uint16(len(expandedSeq)); i < frameDuration; i++ {
										frameIdx := int(i)
										if frameIdx <= holdFrames {
											note.freqSeq[i] = baseFreq
										} else {
											vibratoIdx := (frameIdx - holdFrames - 1) % 8
											delta := template.Deltas[vibratoIdx]
											note.freqSeq[i] = uint16(int32(baseFreq) + int32(delta))
										}
									}
								} else {
									// Fallback: hold last frequency
									lastFreq := expandedSeq[len(expandedSeq)-1]
									for i := uint16(len(expandedSeq)); i < frameDuration; i++ {
										note.freqSeq[i] = lastFreq
									}
								}
							} else {
								// For other types: hold last frequency
								lastFreq := expandedSeq[len(expandedSeq)-1]
								for i := uint16(len(expandedSeq)); i < frameDuration; i++ {
									note.freqSeq[i] = lastFreq
								}
							}
						}
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

				// Load waveform pattern (for R13 envelope shape and R7 mixer changes)
				if pattern.WaveformPatternIndex != 0xFF && int(pattern.WaveformPatternIndex) < len(format.WaveformPatterns) {
					waveformPattern := format.WaveformPatterns[pattern.WaveformPatternIndex]
					note.waveformSeq = make([]byte, frameDuration)

					// Copy waveform values, repeat last value if pattern is shorter than note duration
					for i := uint16(0); i < frameDuration; i++ {
						if int(i) < len(waveformPattern.Waveforms) {
							note.waveformSeq[i] = waveformPattern.Waveforms[i]
						} else if len(waveformPattern.Waveforms) > 0 {
							// Pad with last value
							note.waveformSeq[i] = waveformPattern.Waveforms[len(waveformPattern.Waveforms)-1]
						}
					}
				} else {
					// No waveform pattern - use constant base waveform
					note.waveformSeq = generateWaveformSequence(
						instrument.BaseWaveform,
						instrument.EnvelopeMode,
						frameDuration,
					)
				}

				// Load sustain pattern (dynamic S values for volume-following)
				if pattern.SustainPatternIndex != 0xFF && int(pattern.SustainPatternIndex) < len(format.SustainPatterns) {
					sustainPattern := format.SustainPatterns[pattern.SustainPatternIndex]
					note.SustainPerFrame = make([]uint8, frameDuration)

					if sustainPattern.IsConstant && len(sustainPattern.Values) > 0 {
						// Constant pattern - repeat single value for entire duration
						constantValue := sustainPattern.Values[0]
						for i := uint16(0); i < frameDuration; i++ {
							note.SustainPerFrame[i] = constantValue
						}
					} else {
						// Variable pattern - copy sustain values, repeat last value if pattern is shorter
						for i := uint16(0); i < frameDuration; i++ {
							if int(i) < len(sustainPattern.Values) {
								note.SustainPerFrame[i] = sustainPattern.Values[i]
							} else if len(sustainPattern.Values) > 0 {
								// Pad with last value
								note.SustainPerFrame[i] = sustainPattern.Values[len(sustainPattern.Values)-1]
							}
						}
					}
				}

			notes = append(notes, note)
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

// ============================================================================
// 6502-Optimized FreqMod Pattern Decoder
// ============================================================================

// expandFreqModPattern6502 expands a 6502-optimized pattern into frequency sequence
// Mimics what the 6502 player will do
func expandFreqModPattern6502(pattern FreqModPattern, trackerFormat *TrackerFormat, freqTable []uint16, seqTemplates []FreqSequenceTemplate, drumSweep DrumSweepPattern) []uint16 {
	switch pattern.Type {
	case 0, 1, 2, 3:
		// Effect-based encoding disabled - fallback to base frequency
		if len(pattern.Data) >= 2 {
			baseFreq := uint16(pattern.Data[0]) | (uint16(pattern.Data[1]) << 8)
			result := make([]uint16, pattern.Length)
			for i := range result {
				result[i] = baseFreq
			}
			return result
		}
		return []uint16{freqTable[0]}
	case 4:
		return expandComplex6502(pattern)
	case 5:
		// SequenceLookup - use template index
		if len(pattern.Data) >= 1 {
			templateIdx := int(pattern.Data[0])
			if templateIdx >= 0 && templateIdx < len(seqTemplates) {
				template := seqTemplates[templateIdx].Frequencies
				result := make([]uint16, pattern.Length)

				// Copy template frequencies
				for i := 0; i < int(pattern.Length); i++ {
					if i < len(template) {
						result[i] = template[i]
					} else if len(template) > 0 {
						// Pad with last template value
						result[i] = template[len(template)-1]
					}
				}

				return result
			}
		}
		// Fallback
		return []uint16{freqTable[0]}
	case 6:
		// DrumSweep - apply decay multiplier table to AY periods
		// Data[0,1] = start AY period
		if len(pattern.Data) >= 2 {
			startAYPeriod := uint16(pattern.Data[0]) | (uint16(pattern.Data[1]) << 8)
			result := make([]uint16, pattern.Length)

			// Decay multiplier table (same as templates 0 and 2)
			// Normalized: 1.00, 0.72, 0.57, 0.43, 0.37, 0.32, 0.29, 0.23, 0.22, 0.19
			decayTable := []float64{1.000, 0.723, 0.566, 0.427, 0.367, 0.322, 0.286, 0.235, 0.215, 0.185}

			// Generate AY period sequence by applying decay table
			for i := 0; i < int(pattern.Length); i++ {
				result[i] = uint16(float64(startAYPeriod) * decayTable[i])
			}

			return result
		}
		// Fallback
		return []uint16{freqTable[0]}
	case 7:
		// DelayedVibrato - hold + oscillation (table-based)
		// Data[0,1] = base AY period
		// Data[2] = holdFrames
		// Data[3] = template index
		if len(pattern.Data) >= 4 {
			baseFreq := uint16(pattern.Data[0]) | (uint16(pattern.Data[1]) << 8)
			holdFrames := int(pattern.Data[2])
			templateIdx := int(pattern.Data[3])

			// Look up oscillation pattern from template table
			if templateIdx >= len(trackerFormat.DelayedVibratoTemplates) {
				// Invalid template index
				fmt.Printf("ERROR: Type 7 invalid templateIdx=%d (only %d templates)\n", templateIdx, len(trackerFormat.DelayedVibratoTemplates))
				return []uint16{baseFreq}
			}

			template := trackerFormat.DelayedVibratoTemplates[templateIdx]
			result := make([]uint16, pattern.Length)

			debugType7 := false // Set to true to debug Type 7 decoding
			if debugType7 {
				fmt.Printf("  Type 7: base=%d hold=%d template=%d len=%d\n", baseFreq, holdFrames, templateIdx, pattern.Length)
				fmt.Printf("    Template deltas: %v\n", template.Deltas)
			}

			for i := 0; i < int(pattern.Length); i++ {
				if i <= holdFrames {
					// Hold at base frequency
					result[i] = baseFreq
					if debugType7 && i < 5 {
						fmt.Printf("    Frame %d: hold at %d\n", i, result[i])
					}
				} else {
					// Apply vibrato pattern from template (period 8)
					vibratoIdx := (i - holdFrames - 1) % 8
					delta := template.Deltas[vibratoIdx]
					result[i] = uint16(int32(baseFreq) + int32(delta))
					if debugType7 && i < holdFrames+5 {
						fmt.Printf("    Frame %d: base=%d + delta[%d]=%d = %d\n", i, baseFreq, vibratoIdx, delta, result[i])
					}
				}
			}

			return result
		}
		// Fallback
		return []uint16{freqTable[0]}
	case 8:
		// Hold - constant frequency for entire duration
		// Data[0,1] = base AY period
		if len(pattern.Data) >= 2 {
			baseFreq := uint16(pattern.Data[0]) | (uint16(pattern.Data[1]) << 8)
			result := make([]uint16, pattern.Length)
			for i := range result {
				result[i] = baseFreq
			}
			return result
		}
		return []uint16{freqTable[0]}
	case 9:
		// LinearRamp - constant delta per frame (portamento/pitch bend)
		// Data[0,1] = start frequency
		// Data[2,3] = delta per frame (signed 16-bit)
		if len(pattern.Data) >= 4 {
			startFreq := uint16(pattern.Data[0]) | (uint16(pattern.Data[1]) << 8)
			delta := int16(pattern.Data[2]) | (int16(pattern.Data[3]) << 8)
			result := make([]uint16, pattern.Length)

			currentFreq := int32(startFreq)
			for i := 0; i < int(pattern.Length); i++ {
				result[i] = uint16(currentFreq)
				currentFreq += int32(delta)
			}
			return result
		}
		return []uint16{freqTable[0]}
	default:
		fmt.Printf("ERROR: Unknown FreqMod type %d\n", pattern.Type)
		return []uint16{freqTable[0]}
	}
}

// expandDelayedArpeggio6502 - Type 0
// Simulates: LDA freq_table,X with varying X
func expandDelayedArpeggio6502(p FreqModPattern, freqTable []uint16) []uint16 {
	if len(p.Data) < 2 {
		return []uint16{freqTable[0]}
	}

	baseNote := p.Data[0]
	patternAndCount := p.Data[1]
	patternType := (patternAndCount >> 4) & 0x0F
	repeatCount := patternAndCount & 0x0F

	result := make([]uint16, 0, int(p.Length))

	// Hold at base frequency during delay
	if baseNote < byte(len(freqTable)) {
		for i := byte(0); i < p.Delay; i++ {
			result = append(result, freqTable[baseNote])
		}
	}

	// Apply arpeggio pattern
	if patternType == 0xFF {
		// Custom pattern: note indices stored directly in Data[2:]
		for i := 2; i < len(p.Data); i++ {
			noteIdx := p.Data[i]
			if int(noteIdx) < len(freqTable) {
				result = append(result, freqTable[noteIdx])
			}
		}
	} else {
		// Known pattern: apply offsets to base note
		var offsets []int8
		switch patternType {
		case 0:
			offsets = []int8{0, 1, 2, 1} // Most common from analysis
		case 1:
			offsets = []int8{0, -1, -2, -1}
		case 2:
			offsets = []int8{0, 2, 0, -2}
		default:
			offsets = []int8{0}
		}

		// Generate arpeggio sequence
		for rep := byte(0); rep < repeatCount; rep++ {
			for _, offset := range offsets {
				noteIdx := int(baseNote) + int(offset)
				if noteIdx >= 0 && noteIdx < len(freqTable) {
					result = append(result, freqTable[noteIdx])
				} else if int(baseNote) < len(freqTable) {
					result = append(result, freqTable[baseNote])
				}
			}
		}
	}

	// Pad or truncate to exact length
	for len(result) < int(p.Length) {
		// Pad with last frequency
		if len(result) > 0 {
			result = append(result, result[len(result)-1])
		} else if int(baseNote) < len(freqTable) {
			result = append(result, freqTable[baseNote])
		}
	}
	if len(result) > int(p.Length) {
		result = result[:int(p.Length)]
	}

	return result
}

// expandDelayedBend6502 - Type 1
// Simulates: CLC; ADC or SEC; SBC
func expandDelayedBend6502(p FreqModPattern, freqTable []uint16) []uint16 {
	if len(p.Data) < 3 {
		return []uint16{freqTable[0]}
	}

	result := make([]uint16, 0, int(p.Length))

	// Read starting frequency (16-bit)
	currentFreq := uint16(p.Data[0]) | (uint16(p.Data[1]) << 8)

	// Hold during delay
	for i := byte(0); i < p.Delay; i++ {
		result = append(result, currentFreq)
	}

	// Apply deltas
	deltaType := p.Data[2]
	dataIdx := 3

	switch deltaType {
	case 0x01: // Byte deltas
		for dataIdx < len(p.Data) {
			delta := int16(int8(p.Data[dataIdx]))
			currentFreq = uint16(int32(currentFreq) + int32(delta))
			result = append(result, currentFreq)
			dataIdx++
		}

	case 0x02: // Word deltas
		for dataIdx+1 < len(p.Data) {
			delta := int16(p.Data[dataIdx]) | (int16(p.Data[dataIdx+1]) << 8)
			currentFreq = uint16(int32(currentFreq) + int32(delta))
			result = append(result, currentFreq)
			dataIdx += 2
		}
	}

	return result
}

// expandTrill6502 - Type 2
// Simulates: ADD offset, flip sign with EOR #$FF; CLC; ADC #1
func expandTrill6502(p FreqModPattern, freqTable []uint16) []uint16 {
	if len(p.Data) < 3 {
		return []uint16{freqTable[0]}
	}

	baseNote := p.Data[0]
	oscillationAmount := int8(p.Data[1])
	repeatCount := p.Data[2]

	result := make([]uint16, 0, int(repeatCount))

	if int(baseNote) >= len(freqTable) {
		return result
	}

	// Oscillate: base, base+offset, base, base-offset, repeat
	sign := int8(1)
	for i := byte(0); i < repeatCount; i++ {
		offset := sign * oscillationAmount
		sign = -sign // Flip for next iteration

		noteIdx := int(baseNote) + int(offset)
		if noteIdx >= 0 && noteIdx < len(freqTable) {
			result = append(result, freqTable[noteIdx])
		} else {
			result = append(result, freqTable[baseNote])
		}
	}

	return result
}

// expandPortamento6502 - Type 3
// Simulates: Bresenham line algorithm (no division)
func expandPortamento6502(p FreqModPattern, freqTable []uint16) []uint16 {
	if len(p.Data) < 3 {
		return []uint16{freqTable[0]}
	}

	startNote := int(p.Data[0])
	endNote := int(p.Data[1])
	stepCount := int(p.Data[2])

	if startNote >= len(freqTable) || endNote >= len(freqTable) {
		return []uint16{freqTable[0]}
	}

	result := make([]uint16, 0, stepCount)

	// Bresenham algorithm for linear interpolation
	// Accumulate the difference, step when accumulated >= duration
	dy := endNote - startNote
	currentNote := startNote
	accumulator := 0

	for step := 0; step < stepCount; step++ {
		if currentNote >= 0 && currentNote < len(freqTable) {
			result = append(result, freqTable[currentNote])
		}

		// Bresenham step
		accumulator += abs(dy)
		if accumulator >= stepCount/2 {
			if dy > 0 {
				currentNote++
			} else if dy < 0 {
				currentNote--
			}
			accumulator -= stepCount
		}
	}

	return result
}

// expandComplex6502 - Type 4
// Full 16-bit frequency arithmetic
func expandComplex6502(p FreqModPattern) []uint16 {
	if len(p.Data) < 2 {
		return []uint16{0}
	}

	startFreq := uint16(p.Data[0]) | (uint16(p.Data[1]) << 8)
	result := []uint16{startFreq}

	currentFreq := startFreq
	dataIdx := 2

	for dataIdx+1 < len(p.Data) {
		delta := int16(p.Data[dataIdx]) | (int16(p.Data[dataIdx+1]) << 8)
		currentFreq = uint16(int32(currentFreq) + int32(delta))
		result = append(result, currentFreq)
		dataIdx += 2
	}

	return result
}
