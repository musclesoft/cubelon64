// Investigate "complex" patterns by looking at source AY commands
package main

import (
	"fmt"
	"cubelon-emu/spectrum"
)

// InvestigateComplexPatterns analyzes what AY hardware is generating "complex" patterns
func InvestigateComplexPatterns(commands []spectrum.AYCommand, notes []NoteWithMetadata) {
	fmt.Printf("\n=== Investigating 'Complex' Patterns ===\n")

	// Analyze first few complex patterns
	complexCount := 0
	for _, note := range notes {
		if note.isRest || len(note.freqSeq) <= 1 {
			continue
		}

		// Check if frequency changes
		hasChange := false
		for i := 1; i < len(note.freqSeq); i++ {
			if note.freqSeq[i] != note.freqSeq[0] {
				hasChange = true
				break
			}
		}
		if !hasChange {
			continue
		}

		// Check if it's categorized as complex
		params := AnalyzeFrequencyPattern(note.freqSeq)
		if params.Type != EffectComplex {
			continue
		}

		complexCount++
		if complexCount > 10 {
			break // Just analyze first 10
		}

		fmt.Printf("\nComplex pattern #%d (channel %d):\n", complexCount, note.Channel)
		fmt.Printf("  Frames: %d-%d (%d frames)\n", note.Note.StartFrame, note.Note.EndFrame, note.Note.EndFrame-note.Note.StartFrame+1)
		fmt.Printf("  Frequencies: %d -> %d\n", note.freqSeq[0], note.freqSeq[len(note.freqSeq)-1])

		// Show first/last few frequencies
		fmt.Printf("  Sequence: ")
		for i := 0; i < min(5, len(note.freqSeq)); i++ {
			fmt.Printf("%d ", note.freqSeq[i])
		}
		if len(note.freqSeq) > 10 {
			fmt.Printf("... ")
			for i := max(0, len(note.freqSeq)-5); i < len(note.freqSeq); i++ {
				fmt.Printf("%d ", note.freqSeq[i])
			}
		}
		fmt.Printf("\n")

		// Find AY commands that generated this
		ch := note.Channel
		toneLoReg := uint8(ch * 2)
		toneHiReg := uint8(ch*2 + 1)
		envPeriodLoReg := uint8(11)
		envPeriodHiReg := uint8(12)
		envShapeReg := uint8(13)
		noiseReg := uint8(6)

		fmt.Printf("  AY commands during this note:\n")

		// Check for envelope mode
		volReg := uint8(8 + ch)
		isEnvelopeMode := false
		for _, cmd := range commands {
			if int(cmd.Frame) >= note.Note.StartFrame && int(cmd.Frame) <= note.Note.EndFrame {
				if cmd.Register == volReg && (cmd.Value&0x10) != 0 {
					isEnvelopeMode = true
					break
				}
			}
		}

		if isEnvelopeMode {
			fmt.Printf("    ENVELOPE MODE: Using R11-R13 envelope generator\n")
			// Show envelope period changes
			envChanges := 0
			for _, cmd := range commands {
				if int(cmd.Frame) >= note.Note.StartFrame && int(cmd.Frame) <= note.Note.EndFrame {
					if cmd.Register == envPeriodLoReg || cmd.Register == envPeriodHiReg || cmd.Register == envShapeReg {
						fmt.Printf("      Frame %d: R%d = 0x%02X\n", cmd.Frame, cmd.Register, cmd.Value)
						envChanges++
						if envChanges > 5 {
							fmt.Printf("      ... (%d more envelope changes)\n", countEnvelopeChanges(commands, note.Note.StartFrame, note.Note.EndFrame)-5)
							break
						}
					}
				}
			}
		} else {
			fmt.Printf("    TONE MODE: Using R0-R1 tone period\n")
			// Show tone period changes
			toneChanges := 0
			for _, cmd := range commands {
				if int(cmd.Frame) >= note.Note.StartFrame && int(cmd.Frame) <= note.Note.EndFrame {
					if cmd.Register == toneLoReg || cmd.Register == toneHiReg {
						fmt.Printf("      Frame %d: R%d = 0x%02X\n", cmd.Frame, cmd.Register, cmd.Value)
						toneChanges++
						if toneChanges > 5 {
							fmt.Printf("      ... (%d more tone changes)\n", countToneChanges(commands, note.Note.StartFrame, note.Note.EndFrame, ch)-5)
							break
						}
					}
				}
			}
		}

		// Check for noise
		for _, cmd := range commands {
			if int(cmd.Frame) >= note.Note.StartFrame && int(cmd.Frame) <= note.Note.EndFrame {
				if cmd.Register == noiseReg {
					fmt.Printf("    NOISE: R6 = 0x%02X at frame %d\n", cmd.Value, cmd.Frame)
					break
				}
			}
		}
	}

	fmt.Printf("\nTotal complex patterns analyzed: %d\n", min(complexCount, 10))
}

func countEnvelopeChanges(commands []spectrum.AYCommand, startFrame, endFrame int) int {
	count := 0
	for _, cmd := range commands {
		if int(cmd.Frame) >= startFrame && int(cmd.Frame) <= endFrame {
			if cmd.Register == 11 || cmd.Register == 12 || cmd.Register == 13 {
				count++
			}
		}
	}
	return count
}

func countToneChanges(commands []spectrum.AYCommand, startFrame, endFrame int, channel int) int {
	count := 0
	toneLoReg := uint8(channel * 2)
	toneHiReg := uint8(channel*2 + 1)
	for _, cmd := range commands {
		if int(cmd.Frame) >= startFrame && int(cmd.Frame) <= endFrame {
			if cmd.Register == toneLoReg || cmd.Register == toneHiReg {
				count++
			}
		}
	}
	return count
}

// AnalyzeComplexPatternSources categorizes complex patterns by their AY source
func AnalyzeComplexPatternSources(commands []spectrum.AYCommand, notes []NoteWithMetadata) {
	fmt.Printf("\n=== Complex Pattern Sources ===\n")

	envelopeBased := 0
	toneBased := 0
	noiseBased := 0
	mixed := 0

	for _, note := range notes {
		if note.isRest || len(note.freqSeq) <= 1 {
			continue
		}

		// Check if frequency changes
		hasChange := false
		for i := 1; i < len(note.freqSeq); i++ {
			if note.freqSeq[i] != note.freqSeq[0] {
				hasChange = true
				break
			}
		}
		if !hasChange {
			continue
		}

		// Check if it's categorized as complex
		params := AnalyzeFrequencyPattern(note.freqSeq)
		if params.Type != EffectComplex {
			continue
		}

		// Determine source
		ch := note.Channel
		volReg := uint8(8 + ch)

		isEnvelope := false
		hasToneChanges := false
		hasNoiseChanges := false

		for _, cmd := range commands {
			if int(cmd.Frame) >= note.Note.StartFrame && int(cmd.Frame) <= note.Note.EndFrame {
				if cmd.Register == volReg && (cmd.Value&0x10) != 0 {
					isEnvelope = true
				}
				if cmd.Register == uint8(ch*2) || cmd.Register == uint8(ch*2+1) {
					hasToneChanges = true
				}
				if cmd.Register == 6 {
					hasNoiseChanges = true
				}
			}
		}

		if isEnvelope {
			envelopeBased++
		} else if hasNoiseChanges && hasToneChanges {
			mixed++
		} else if hasNoiseChanges {
			noiseBased++
		} else if hasToneChanges {
			toneBased++
		}
	}

	fmt.Printf("  Envelope-based: %d patterns\n", envelopeBased)
	fmt.Printf("  Tone-based: %d patterns\n", toneBased)
	fmt.Printf("  Noise-based: %d patterns\n", noiseBased)
	fmt.Printf("  Mixed: %d patterns\n", mixed)
}
