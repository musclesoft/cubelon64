// AY-3-8912 to SID 6581 Audio Optimizer
//
// Translates ZX Spectrum (AY-3-8912) audio to Commodore 64 (SID 6581) by analyzing
// volume envelopes and fitting optimal ADSR parameters.
//
// The AY chip uses software-controlled per-frame volume changes, while the SID uses
// hardware ADSR envelopes. This tool bridges the gap by:
//   1. Emulating AY audio from logged register commands
//   2. Computing RMS envelope per frame (882 samples @ 44.1kHz = 1/50s)
//   3. Detecting individual notes from RMS envelope (zero threshold)
//   4. Finding "hold" regions (longest constant-volume segment in each note)
//   5. Fitting SID ADSR curves by trying all 16^4 combinations and minimizing RMS error
//   6. Using median ADSR parameters across all notes per channel
//   7. Generating SID audio with continuous frequency tracking and RMS-based gating
//
// Usage:
//   # First, log AY commands from the emulator:
//   cd ../emulator
//   go run run_cubelon.go motion_analyzer.go assertions.go \
//     --log-ay-commands=ay_commands.bin --max-frames=9650 --no-frames
//
//   # Then optimize:
//   cd ../tools
//   go run optimize_volume.go sid_emulator.go \
//     --input=../emulator/ay_commands.bin \
//     --output=optimized.wav \
//     --verbose
//
// Output: 6-channel WAV file
//   - Channels 1-3: Original AY audio (for comparison)
//   - Channels 4-6: Optimized SID audio
//
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"sync"

	"cubelon-emu/spectrum"
)

// Note represents a detected note from RMS
type Note struct {
	StartFrame int
	EndFrame   int
	RMSCurve   []float64
	ADSR       ADSRParams
}

// ADSRParams for a channel
type ADSRParams struct {
	Attack   uint8
	Decay    uint8
	Sustain  uint8
	Release  uint8
}

// SID ADSR rate tables (cycles per step at 1MHz)
var adsrRates = [16]uint32{
	9, 32, 63, 95, 149, 220, 267, 313,
	392, 977, 1954, 3126, 3907, 11720, 19532, 31251,
}

func main() {
	inputFile := flag.String("input", "", "Input AY command log (.bin)")
	outputWav := flag.String("output", "volume_optimized.wav", "Output 6-channel WAV")
	minNoteFrames := flag.Int("min-note", 3, "Minimum note length in frames")
	verbose := flag.Bool("verbose", false, "Show detailed analysis")

	flag.Parse()

	if *inputFile == "" {
		fmt.Fprintf(os.Stderr, "Error: --input required\n")
		os.Exit(1)
	}

	fmt.Printf("=== VOLUME-FOLLOWING ADSR ===\n\n")
	fmt.Printf("Loading AY commands from %s...\n", *inputFile)

	commands, err := spectrum.LoadAYCommands(*inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading commands: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loaded %d commands\n", len(commands))
	fmt.Printf("Minimum note length: %d frames\n\n", *minNoteFrames)

	maxFrame := int(commands[len(commands)-1].Frame)

	// Generate AY audio and compute RMS per frame for each channel
	rmsPerFrame := [3][]float64{}
	var wg sync.WaitGroup

	for ch := 0; ch < 3; ch++ {
		wg.Add(1)
		go func(channel int) {
			defer wg.Done()
			fmt.Printf("Generating Channel %c audio...\n", 'A'+channel)
			ayAudio := generateAYAudioChannel(commands, channel, 0, maxFrame)
			fmt.Printf("Computing RMS per frame for Channel %c...\n", 'A'+channel)
			rmsPerFrame[channel] = computeRMSPerFrame(ayAudio)
		}(ch)
	}
	wg.Wait()

	// Detect notes from RMS envelopes
	notes := [3][]Note{}
	for ch := 0; ch < 3; ch++ {
		fmt.Printf("\nAnalyzing Channel %c RMS envelope...\n", 'A'+ch)
		notes[ch] = detectNotesFromRMS(rmsPerFrame[ch], *minNoteFrames, *verbose)
		fmt.Printf("  Found %d notes\n", len(notes[ch]))
	}

	// Analyze volume phases in each note
	fmt.Printf("\n=== Analyzing Volume Phases ===\n")
	for ch := 0; ch < 3; ch++ {
		fmt.Printf("Channel %c: Analyzing %d notes...\n", 'A'+ch, len(notes[ch]))
		analyzeVolumePhases(notes[ch], *verbose)
	}

	// Compute median ADSR per channel
	fmt.Printf("\n=== Computing Median ADSR per Channel ===\n")
	channelADSR := [3]ADSRParams{}
	for ch := 0; ch < 3; ch++ {
		channelADSR[ch] = computeMedianADSR(notes[ch])
		fmt.Printf("Channel %c: A=%d D=%d S=%d R=%d\n", 'A'+ch,
			channelADSR[ch].Attack, channelADSR[ch].Decay,
			channelADSR[ch].Sustain, channelADSR[ch].Release)
	}

	// Generate comparison WAV
	fmt.Printf("\n=== Generating Final WAV ===\n")
	generateComparisonWAV(commands, rmsPerFrame, notes, channelADSR, outputWav)

	fmt.Printf("✓ Complete! Written to: %s\n", *outputWav)
	fmt.Printf("\nWAV channels (paired for easy comparison):\n")
	fmt.Printf("  1: AY Channel A\n")
	fmt.Printf("  2: SID Voice 1 (A)\n")
	fmt.Printf("  3: AY Channel B\n")
	fmt.Printf("  4: SID Voice 2 (B)\n")
	fmt.Printf("  5: AY Channel C\n")
	fmt.Printf("  6: SID Voice 3 (C)\n")
}

// analyzeVolumePhases analyzes each note to find A/D/S/R phases
func analyzeVolumePhases(notes []Note, verbose bool) {
	// Count notes by length
	longNotes := 0
	for _, note := range notes {
		if len(note.RMSCurve) >= 20 {
			longNotes++
		}
	}
	if longNotes > 0 {
		fmt.Printf("  %d notes with length >= 20 frames (>= 0.4s)\n", longNotes)
	}

	for i := range notes {
		notes[i].ADSR = detectVolumePhases(notes[i].RMSCurve)

		// Validate the computed ADSR against actual curve
		error := validateADSR(notes[i].RMSCurve, notes[i].ADSR)
		if error > 0.25 {
			fmt.Printf("  WARNING: Note %d ADSR fit error: %.3f (A=%d D=%d S=%d R=%d)\n",
				i+1, error,
				notes[i].ADSR.Attack, notes[i].ADSR.Decay,
				notes[i].ADSR.Sustain, notes[i].ADSR.Release)

			if verbose && i < 3 {
				// Show actual vs simulated curve for debugging
				fmt.Printf("    Actual RMS: ")
				for j := 0; j < 10 && j < len(notes[i].RMSCurve); j++ {
					fmt.Printf("%.3f ", notes[i].RMSCurve[j])
				}
				fmt.Printf("... (len=%d)\n", len(notes[i].RMSCurve))
			}
		}

		if verbose && i < 5 {
			fmt.Printf("  Note %d (len=%d): A=%d D=%d S=%d R=%d (error=%.3f)\n",
				i+1, len(notes[i].RMSCurve),
				notes[i].ADSR.Attack, notes[i].ADSR.Decay,
				notes[i].ADSR.Sustain, notes[i].ADSR.Release, error)
		}
	}
}

// detectVolumePhases detects ADSR phases from volume curve using curve fitting
func detectVolumePhases(rms []float64) ADSRParams {
	if len(rms) == 0 {
		return ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}
	}

	peakRMS := 0.0
	for _, r := range rms {
		if r > peakRMS {
			peakRMS = r
		}
	}

	if peakRMS < 0.001 {
		return ADSRParams{Attack: 0, Decay: 0, Sustain: 0, Release: 0}
	}

	// Normalize curve to 0-1
	normalized := make([]float64, len(rms))
	for i, r := range rms {
		normalized[i] = r / peakRMS
	}

	// Step 1: Detect hold region (longest constant-volume region, any level)
	holdStart := -1
	holdEnd := -1
	tolerance := 0.05 // 5% variation allowed

	for i := 0; i < len(normalized); i++ {
		baseLevel := normalized[i]
		j := i
		for j < len(normalized) && math.Abs(normalized[j]-baseLevel) <= tolerance {
			j++
		}
		length := j - i
		if length >= 3 && length > (holdEnd - holdStart + 1) {
			holdStart = i
			holdEnd = j - 1
		}
	}

	// Step 2: Split into phases
	var adsPhase, rPhase []float64
	hasHold := holdStart >= 0 && holdEnd >= holdStart

	if hasHold {
		// ADS phase: start to hold end (inclusive)
		adsPhase = normalized[:holdEnd+1]
		// R phase: hold end to note end
		if holdEnd+1 < len(normalized) {
			rPhase = normalized[holdEnd:]
		}
	} else {
		// No clear hold - entire note is ADS (attack+decay to zero)
		adsPhase = normalized
		rPhase = nil
	}

	// Step 3: Fit ADS by trying all combinations
	bestADS := struct{ A, D, S uint8 }{0, 0, 0}
	bestADSError := 1e9

	// Coarse search first
	for a := uint8(0); a < 16; a += 4 {
		for d := uint8(0); d < 16; d += 4 {
			for s := uint8(0); s <= 15; s += 5 {
				simulated := simulateADS(a, d, s, len(adsPhase))
				error := rmsError(adsPhase, simulated)
				if error < bestADSError {
					bestADSError = error
					bestADS = struct{ A, D, S uint8 }{a, d, s}
				}
			}
		}
	}

	// Fine search around best
	for a := int(bestADS.A) - 3; a <= int(bestADS.A)+3; a++ {
		if a < 0 || a > 15 {
			continue
		}
		for d := int(bestADS.D) - 3; d <= int(bestADS.D)+3; d++ {
			if d < 0 || d > 15 {
				continue
			}
			for s := int(bestADS.S) - 2; s <= int(bestADS.S)+2; s++ {
				if s < 0 || s > 15 {
					continue
				}
				simulated := simulateADS(uint8(a), uint8(d), uint8(s), len(adsPhase))
				error := rmsError(adsPhase, simulated)
				if error < bestADSError {
					bestADSError = error
					bestADS = struct{ A, D, S uint8 }{uint8(a), uint8(d), uint8(s)}
				}
			}
		}
	}

	// Step 4: Fit R if we have release phase
	bestR := uint8(0)
	bestRError := 0.0

	if rPhase != nil && len(rPhase) > 2 {
		bestRError = 1e9
		// Coarse search
		for r := uint8(0); r < 16; r += 4 {
			simulated := simulateR(r, bestADS.S, len(rPhase))
			error := rmsError(rPhase, simulated)
			if error < bestRError {
				bestRError = error
				bestR = r
			}
		}
		// Fine search
		for r := int(bestR) - 3; r <= int(bestR)+3; r++ {
			if r < 0 || r > 15 {
				continue
			}
			simulated := simulateR(uint8(r), bestADS.S, len(rPhase))
			error := rmsError(rPhase, simulated)
			if error < bestRError {
				bestRError = error
				bestR = uint8(r)
			}
		}
	}

	return ADSRParams{
		Attack:  bestADS.A,
		Decay:   bestADS.D,
		Sustain: bestADS.S,
		Release: bestR,
	}
}

// simulateADS simulates Attack-Decay-Sustain envelope (gate stays on)
func simulateADS(attack, decay, sustain uint8, numFrames int) []float64 {
	envelope := make([]float64, numFrames)
	cyclesPerFrame := 985248.0 / 50.0

	envelopeLevel := uint8(0)
	envelopeState := 0 // 0=Attack, 1=Decay/Sustain
	envelopeCounter := 0.0

	for frame := 0; frame < numFrames; frame++ {
		for cycle := 0.0; cycle < cyclesPerFrame; cycle++ {
			envelopeCounter++

			switch envelopeState {
			case 0: // Attack
				ratePeriod := float64(adsrRates[attack])
				if envelopeCounter >= ratePeriod {
					envelopeCounter = 0
					if envelopeLevel < 255 {
						envelopeLevel++
					}
					if envelopeLevel >= 255 {
						envelopeLevel = 255
						envelopeState = 1
					}
				}

			case 1: // Decay/Sustain
				sustainTarget := uint8(sustain) * 17
				if envelopeLevel > sustainTarget {
					ratePeriod := float64(adsrRates[decay])
					if envelopeCounter >= ratePeriod {
						envelopeCounter = 0
						if envelopeLevel > 0 {
							envelopeLevel--
						}
					}
				}
			}
		}
		envelope[frame] = float64(envelopeLevel) / 255.0
	}

	return envelope
}

// simulateR simulates Release envelope (starting from sustain level)
func simulateR(release, sustain uint8, numFrames int) []float64 {
	envelope := make([]float64, numFrames)
	cyclesPerFrame := 985248.0 / 50.0

	envelopeLevel := uint8(sustain) * 17 // Start at sustain level
	envelopeCounter := 0.0

	for frame := 0; frame < numFrames; frame++ {
		for cycle := 0.0; cycle < cyclesPerFrame; cycle++ {
			envelopeCounter++
			ratePeriod := float64(adsrRates[release])
			if envelopeCounter >= ratePeriod {
				envelopeCounter = 0
				if envelopeLevel > 0 {
					envelopeLevel--
				}
			}
		}
		envelope[frame] = float64(envelopeLevel) / 255.0
	}

	return envelope
}

// rmsError computes RMS error between two envelopes
func rmsError(actual, simulated []float64) float64 {
	if len(actual) != len(simulated) || len(actual) == 0 {
		return 1e9
	}
	sumSquares := 0.0
	for i := 0; i < len(actual); i++ {
		diff := actual[i] - simulated[i]
		sumSquares += diff * diff
	}
	return math.Sqrt(sumSquares / float64(len(actual)))
}

// validateADSR simulates the ADSR and compares to actual RMS curve
func validateADSR(rmsCurve []float64, adsr ADSRParams) float64 {
	if len(rmsCurve) == 0 {
		return 0.0
	}

	// Normalize curve to 0-1
	maxRMS := 0.0
	for _, r := range rmsCurve {
		if r > maxRMS {
			maxRMS = r
		}
	}
	normalized := make([]float64, len(rmsCurve))
	for i, r := range rmsCurve {
		normalized[i] = r / maxRMS
	}

	// Detect hold region to determine gate off point
	holdEnd := len(rmsCurve) - 1
	holdMinLevel := 0.85
	for i := 0; i < len(normalized); i++ {
		if normalized[i] >= holdMinLevel {
			j := i
			for j < len(normalized) && normalized[j] >= holdMinLevel {
				j++
			}
			if j-i >= 3 {
				holdEnd = j - 1
				break
			}
		}
	}

	// Simulate ADSR: gate on until holdEnd, then gate off
	simulated := simulateFullADSR(adsr.Attack, adsr.Decay, adsr.Sustain, adsr.Release, len(rmsCurve), holdEnd+1)

	// Compute RMS error
	return rmsError(normalized, simulated)
}

// simulateFullADSR simulates full ADSR with gate on/off at specific frame
func simulateFullADSR(attack, decay, sustain, release uint8, numFrames, gateOffFrame int) []float64 {
	envelope := make([]float64, numFrames)
	cyclesPerFrame := 985248.0 / 50.0

	envelopeLevel := uint8(0)
	envelopeState := 0 // 0=Attack, 1=Decay/Sustain, 2=Release
	envelopeCounter := 0.0

	for frame := 0; frame < numFrames; frame++ {
		// Gate off at specified frame
		if frame == gateOffFrame && envelopeState != 2 {
			envelopeState = 2
			envelopeCounter = 0
		}

		for cycle := 0.0; cycle < cyclesPerFrame; cycle++ {
			envelopeCounter++

			switch envelopeState {
			case 0: // Attack
				ratePeriod := float64(adsrRates[attack])
				if envelopeCounter >= ratePeriod {
					envelopeCounter = 0
					if envelopeLevel < 255 {
						envelopeLevel++
					}
					if envelopeLevel >= 255 {
						envelopeLevel = 255
						envelopeState = 1
					}
				}

			case 1: // Decay/Sustain
				sustainTarget := uint8(sustain) * 17
				if envelopeLevel > sustainTarget {
					ratePeriod := float64(adsrRates[decay])
					if envelopeCounter >= ratePeriod {
						envelopeCounter = 0
						if envelopeLevel > 0 {
							envelopeLevel--
						}
					}
				}

			case 2: // Release
				ratePeriod := float64(adsrRates[release])
				if envelopeCounter >= ratePeriod {
					envelopeCounter = 0
					if envelopeLevel > 0 {
						envelopeLevel--
					}
				}
			}
		}
		envelope[frame] = float64(envelopeLevel) / 255.0
	}

	return envelope
}

// computeMedianADSR computes channel-wide median ADSR
func computeMedianADSR(notes []Note) ADSRParams {
	if len(notes) == 0 {
		return ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}
	}

	var attacks, decays, sustains, releases []uint8
	for _, note := range notes {
		attacks = append(attacks, note.ADSR.Attack)
		decays = append(decays, note.ADSR.Decay)
		sustains = append(sustains, note.ADSR.Sustain)
		releases = append(releases, note.ADSR.Release)
	}

	return ADSRParams{
		Attack:  median(attacks),
		Decay:   median(decays),
		Sustain: median(sustains),
		Release: median(releases),
	}
}

func median(values []uint8) uint8 {
	if len(values) == 0 {
		return 0
	}

	sorted := make([]uint8, len(values))
	copy(sorted, values)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted[len(sorted)/2]
}

// computeRMSPerFrame computes RMS over 882-sample windows
func computeRMSPerFrame(audio []int16) []float64 {
	samplesPerFrame := 882
	numFrames := len(audio) / samplesPerFrame

	rms := make([]float64, numFrames)

	for frame := 0; frame < numFrames; frame++ {
		start := frame * samplesPerFrame
		end := start + samplesPerFrame
		if end > len(audio) {
			end = len(audio)
		}

		sumSquares := 0.0
		count := 0
		for i := start; i < end; i++ {
			val := float64(audio[i]) / 32768.0
			sumSquares += val * val
			count++
		}

		if count > 0 {
			rms[frame] = math.Sqrt(sumSquares / float64(count))
		}
	}

	return rms
}

// detectNotesFromRMS detects notes from RMS envelope
func detectNotesFromRMS(rms []float64, minNoteFrames int, verbose bool) []Note {
	var notes []Note

	maxRMS := 0.0
	for _, r := range rms {
		if r > maxRMS {
			maxRMS = r
		}
	}
	threshold := 0.0

	if verbose {
		fmt.Printf("  Max RMS: %.4f, threshold: %.4f\n", maxRMS, threshold)
	}

	inNote := false
	noteStart := 0

	for frame := 0; frame < len(rms); frame++ {
		if !inNote && rms[frame] > threshold {
			inNote = true
			noteStart = frame
		} else if inNote && rms[frame] <= threshold {
			noteLength := frame - noteStart
			if noteLength >= minNoteFrames {
				curve := make([]float64, noteLength)
				copy(curve, rms[noteStart:frame])

				notes = append(notes, Note{
					StartFrame: noteStart,
					EndFrame:   frame - 1,
					RMSCurve:   curve,
				})
			}
			inNote = false
		}
	}

	if inNote {
		noteLength := len(rms) - noteStart
		if noteLength >= minNoteFrames {
			curve := make([]float64, noteLength)
			copy(curve, rms[noteStart:])

			notes = append(notes, Note{
				StartFrame: noteStart,
				EndFrame:   len(rms) - 1,
				RMSCurve:   curve,
			})
		}
	}

	return notes
}

func generateComparisonWAV(commands []spectrum.AYCommand, rmsPerFrame [3][]float64, notes [3][]Note, channelADSR [3]ADSRParams, outputPath *string) {
	sampleRate := 44100
	durationFrames := int(commands[len(commands)-1].Frame)
	durationSamples := durationFrames * sampleRate / 50

	var wg sync.WaitGroup
	ayAudio := make([][]int16, 3)
	for ch := 0; ch < 3; ch++ {
		wg.Add(1)
		go func(channel int) {
			defer wg.Done()
			ayAudio[channel] = generateAYAudioChannel(commands, channel, 0, durationFrames)
		}(ch)
	}
	wg.Wait()

	sidAudio := make([][]int16, 3)
	for ch := 0; ch < 3; ch++ {
		wg.Add(1)
		go func(channel int) {
			defer wg.Done()
			sidAudio[channel] = generateSIDPerNote(commands, channel, notes[channel], channelADSR[channel], durationSamples)
		}(ch)
	}
	wg.Wait()

	writeMultiChannelWAV(*outputPath, ayAudio, sidAudio, sampleRate)
}

func generateSIDPerNote(commands []spectrum.AYCommand, channel int, notes []Note, fallbackADSR ADSRParams, totalSamples int) []int16 {
	sampleRate := 44100.0
	samplesPerFrame := int(sampleRate / 50.0)
	samples := make([]int16, totalSamples)

	sid := NewSIDEmulator(sampleRate)

	// Build frequency table from AY commands
	numFrames := totalSamples / samplesPerFrame
	freqPerFrame := make([]uint16, numFrames)
	currentFreq := uint16(0)

	toneLoReg := uint8(channel * 2)
	toneHiReg := uint8(channel*2 + 1)

	for _, cmd := range commands {
		frame := int(cmd.Frame)
		if frame >= len(freqPerFrame) {
			continue
		}

		switch cmd.Register {
		case toneLoReg:
			currentFreq = (currentFreq & 0xFF00) | uint16(cmd.Value)
			freqPerFrame[frame] = currentFreq
		case toneHiReg:
			currentFreq = (currentFreq & 0x00FF) | (uint16(cmd.Value&0x0F) << 8)
			freqPerFrame[frame] = currentFreq
		}
	}

	// Forward-fill frequency array
	lastFreq := uint16(0)
	for i := range freqPerFrame {
		if freqPerFrame[i] != 0 {
			lastFreq = freqPerFrame[i]
		} else {
			freqPerFrame[i] = lastFreq
		}
	}

	// Process each note with its specific ADSR
	currentNoteIdx := 0
	lastSIDFreq := uint16(0)
	gateOn := false

	for frame := 0; frame < numFrames; frame++ {
		// Check if we need to gate OFF (note ended)
		if gateOn && currentNoteIdx > 0 && frame > notes[currentNoteIdx-1].EndFrame {
			sid.WriteRegister(4, 0x04<<4)
			gateOn = false
		}

		// Check if we're starting a new note
		if currentNoteIdx < len(notes) && frame == notes[currentNoteIdx].StartFrame {
			note := notes[currentNoteIdx]

			// Gate OFF first if already on (retrigger for adjacent notes)
			if gateOn {
				sid.WriteRegister(4, 0x04<<4)
			}

			// Set this note's specific ADSR
			sid.WriteRegister(5, (note.ADSR.Attack<<4)|note.ADSR.Decay)
			sid.WriteRegister(6, (note.ADSR.Sustain<<4)|note.ADSR.Release)

			// Set frequency
			if frame < len(freqPerFrame) && freqPerFrame[frame] > 0 {
				ayClock := 1773400.0
				freqHz := ayClock / (16.0 * float64(freqPerFrame[frame]))
				sidFreq := uint16(freqHz * 16777216.0 / 985248.0)
				sid.WriteRegister(0, byte(sidFreq&0xFF))
				sid.WriteRegister(1, byte(sidFreq>>8))
				lastSIDFreq = sidFreq
			}

			// Gate on
			sid.WriteRegister(4, (0x04<<4)|0x01)
			gateOn = true
			currentNoteIdx++
		} else if frame < len(freqPerFrame) && freqPerFrame[frame] > 0 {
			// Update frequency during note
			ayClock := 1773400.0
			freqHz := ayClock / (16.0 * float64(freqPerFrame[frame]))
			sidFreq := uint16(freqHz * 16777216.0 / 985248.0)

			if sidFreq != lastSIDFreq {
				sid.WriteRegister(0, byte(sidFreq&0xFF))
				sid.WriteRegister(1, byte(sidFreq>>8))
				lastSIDFreq = sidFreq
			}
		}

		// Generate samples for this frame
		frameStart := frame * samplesPerFrame
		for i := 0; i < samplesPerFrame && frameStart+i < totalSamples; i++ {
			voice0, _, _ := sid.GenerateSample()
			samples[frameStart+i] = voice0
		}
	}

	return samples
}

func generateSIDContinuous(commands []spectrum.AYCommand, channel int, rms []float64, adsr ADSRParams, totalSamples int) []int16 {
	sampleRate := 44100.0
	samplesPerFrame := int(sampleRate / 50.0)
	samples := make([]int16, totalSamples)

	sid := NewSIDEmulator(sampleRate)

	sid.WriteRegister(5, (adsr.Attack<<4)|adsr.Decay)
	sid.WriteRegister(6, (adsr.Sustain<<4)|adsr.Release)

	freqPerFrame := make([]uint16, len(rms))
	currentFreq := uint16(0)

	toneLoReg := uint8(channel * 2)
	toneHiReg := uint8(channel*2 + 1)

	for _, cmd := range commands {
		frame := int(cmd.Frame)
		if frame >= len(freqPerFrame) {
			continue
		}

		switch cmd.Register {
		case toneLoReg:
			currentFreq = (currentFreq & 0xFF00) | uint16(cmd.Value)
			freqPerFrame[frame] = currentFreq
		case toneHiReg:
			currentFreq = (currentFreq & 0x00FF) | (uint16(cmd.Value&0x0F) << 8)
			freqPerFrame[frame] = currentFreq
		}
	}

	lastFreq := uint16(0)
	for i := range freqPerFrame {
		if freqPerFrame[i] != 0 {
			lastFreq = freqPerFrame[i]
		} else {
			freqPerFrame[i] = lastFreq
		}
	}

	maxRMS := 0.0
	for _, r := range rms {
		if r > maxRMS {
			maxRMS = r
		}
	}
	threshold := maxRMS * 0.1

	gateOn := false
	lastSIDFreq := uint16(0)
	silentFrames := 0 // Count consecutive frames below threshold

	for frame := 0; frame < len(rms); frame++ {
		if freqPerFrame[frame] > 0 {
			ayClock := 1773400.0
			freqHz := ayClock / (16.0 * float64(freqPerFrame[frame]))
			sidFreq := uint16(freqHz * 16777216.0 / 985248.0)

			if sidFreq != lastSIDFreq {
				sid.WriteRegister(0, byte(sidFreq&0xFF))
				sid.WriteRegister(1, byte(sidFreq>>8))
				lastSIDFreq = sidFreq
			}
		}

		// Track silence duration
		if rms[frame] <= threshold {
			silentFrames++
		} else {
			silentFrames = 0
		}

		// Gate ON: RMS rises above threshold (note starts)
		if rms[frame] > threshold && !gateOn {
			sid.WriteRegister(4, (0x04<<4)|0x01)
			gateOn = true
			silentFrames = 0
		} else if silentFrames >= 3 && gateOn {
			// Gate OFF: 3+ consecutive silent frames (note truly ends)
			// This allows ADSR decay to complete instead of cutting off early
			sid.WriteRegister(4, 0x04<<4)
			gateOn = false
		}

		frameStart := frame * samplesPerFrame
		for i := 0; i < samplesPerFrame && frameStart+i < totalSamples; i++ {
			voice0, _, _ := sid.GenerateSample()
			samples[frameStart+i] = voice0
		}
	}

	return samples
}

func generateAYAudioChannel(commands []spectrum.AYCommand, channel, startFrame, endFrame int) []int16 {
	sampleRate := 44100
	samplesPerFrame := sampleRate / 50

	numFrames := endFrame - startFrame + 1
	numSamples := numFrames * samplesPerFrame

	durationSeconds := float64(numSamples) / float64(sampleRate)
	ay := spectrum.NewAudioBuffer(sampleRate, durationSeconds)
	ayChip := spectrum.NewAY38912()

	cyclesPerFrame := uint64(69888)
	startCycle := uint64(startFrame) * cyclesPerFrame

	for _, cmd := range commands {
		if int(cmd.Frame) >= startFrame && int(cmd.Frame) <= endFrame {
			ayChip.SelectRegister(cmd.Register)
			ayChip.WriteRegister(cmd.Value)
			cycle := uint64(cmd.Frame)*cyclesPerFrame - startCycle
			ay.Update(ayChip, cycle)
		}
	}

	finalCycle := uint64(endFrame+1)*cyclesPerFrame - startCycle
	ay.Update(ayChip, finalCycle)

	samples := make([]int16, numSamples)
	for i := 0; i < numSamples && i < ay.Position; i++ {
		samples[i] = ay.Samples[i*6+channel]
	}

	return samples
}

func writeMultiChannelWAV(path string, ayChannels [][]int16, sidChannels [][]int16, sampleRate int) {
	numSamples := len(ayChannels[0])
	for ch := 1; ch < 3; ch++ {
		if len(ayChannels[ch]) < numSamples {
			numSamples = len(ayChannels[ch])
		}
		if len(sidChannels[ch]) < numSamples {
			numSamples = len(sidChannels[ch])
		}
	}

	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating WAV: %v\n", err)
		return
	}
	defer f.Close()

	numChannels := 6
	dataSize := uint32(numSamples * numChannels * 2)

	f.Write([]byte("RIFF"))
	writeUint32(f, dataSize+36)
	f.Write([]byte("WAVE"))

	f.Write([]byte("fmt "))
	writeUint32(f, 16)
	writeUint16(f, 1)
	writeUint16(f, uint16(numChannels))
	writeUint32(f, uint32(sampleRate))
	writeUint32(f, uint32(sampleRate*numChannels*2))
	writeUint16(f, uint16(numChannels*2))
	writeUint16(f, 16)

	f.Write([]byte("data"))
	writeUint32(f, dataSize)

	// Interleave samples (pair each AY channel with its SID voice for easy comparison)
	for i := 0; i < numSamples; i++ {
		for ch := 0; ch < 3; ch++ {
			writeSample(f, ayChannels[ch][i])
			writeSample(f, sidChannels[ch][i])
		}
	}
}

func writeUint16(f *os.File, v uint16) {
	f.Write([]byte{byte(v), byte(v >> 8)})
}

func writeUint32(f *os.File, v uint32) {
	f.Write([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
}

func writeSample(f *os.File, s int16) {
	f.Write([]byte{byte(s), byte(s >> 8)})
}
