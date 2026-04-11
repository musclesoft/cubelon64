// AY-3-8912 to SID 6581 Audio Optimizer
//
// Translates ZX Spectrum (AY-3-8912) audio to Commodore 64 (SID 6581) by analyzing
// volume envelopes and fitting optimal ADSR parameters.
//
// The AY chip uses software-controlled per-frame volume changes, while the SID uses
// hardware ADSR envelopes. This tool bridges the gap by:
//  1. Emulating AY audio from logged register commands
//  2. Computing RMS envelope per frame (880 samples @ 44.1kHz = 19.95ms PAL)
//  3. Detecting individual notes from RMS envelope (zero threshold)
//  4. Finding "hold" regions (longest constant-volume segment in each note)
//  5. Fitting SID ADSR curves by trying all 16^4 combinations and minimizing RMS error
//  6. Using median ADSR parameters across all notes per channel
//  7. Generating SID audio with continuous frequency tracking and RMS-based gating
//
// Usage:
//
//	# First, log AY commands from the emulator:
//	cd ../emulator
//	go run run_cubelon.go motion_analyzer.go assertions.go \
//	  --log-ay-commands=ay_commands.bin --max-frames=9650 --no-frames
//
//	# Then optimize:
//	cd ../tools
//	go run optimize_volume.go sid_emulator.go \
//	  --input=../emulator/ay_commands.bin \
//	  --output=output.wav \
//	  --verbose
//
// Output: 6-channel WAV file
//   - Channels 0-1: AY Channel A, SID Voice 1
//   - Channels 2-3: AY Channel B, SID Voice 2
//   - Channels 4-5: AY Channel C, SID Voice 3
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"sync"
	"time"

	"cubelon-emu/spectrum"
)

// PAL timing constants
const (
	sidClockHz        = 985248.0 // PAL SID clock frequency
	cyclesPerFrame    = 19656    // PAL: 985248 / 50.124542 Hz
	palFrameRate      = 50.124542124542 // 985248 / 19656
)

// Note represents a detected note from RMS
type Note struct {
	StartFrame      int
	EndFrame        int
	VolumeCurve        []float64
	ADSR            ADSRParams
	GateOffFrame    int     // Frame (relative to StartFrame) when gate turns off
	UseNoise        bool    // If true, use noise waveform (0x08)
	Waveform        byte    // SID waveform: 0x01=triangle, 0x02=sawtooth, 0x04=pulse, 0x08=noise
	EnvelopeMode    bool    // If true, note uses AY envelope mode (bit 4 of volume register)
	EnvelopeTypeID  int     // ID of the envelope type this note belongs to
	ReleasePerFrame []uint8 // Optimal R value per frame (for dynamic envelope matching)
}

// ADSRParams for a channel
type ADSRParams struct {
	Attack  uint8
	Decay   uint8
	Sustain uint8
	Release uint8
}

// EnvelopeType represents a cluster of similar envelope shapes
type EnvelopeType struct {
	ID              int
	RepresentCurve  []float64  // Representative normalized curve
	ADSR            ADSRParams // Fitted ADSR for this type
	GateOffFrame    int        // Frame when gate turns off (for release phase)
	NoteIndices     []int      // Indices of notes using this type (per-channel when used per-channel)
	ChannelID       int        // Which channel (0=A, 1=B, 2=C) - used during per-channel analysis
	ReleasePerFrame []uint8    // Optimal R value per frame (computed from representative curve)
	ChannelNotes    []struct { // For merged cross-channel types
		Channel int
		Indices []int
	}
}

// candidateCurve represents a candidate envelope curve for clustering
type candidateCurve struct {
	noteIdx    int
	normalized []float64
	isComplete bool
}

// SID ADSR rate tables (cycles per step at 1MHz)
var adsrRates = [16]uint32{
	8, 31, 62, 94, 148, 219, 266, 312,
	391, 976, 1953, 3125, 3906, 11719, 19531, 31250,
}

func main() {
	inputFile := flag.String("input", "", "Input AY command log (.bin)")
	minNoteFrames := flag.Int("min-note", 3, "Minimum note length in frames")
	verbose := flag.Bool("verbose", false, "Show detailed analysis")
	shapesOnly := flag.Bool("shapes-only", false, "Only analyze envelope shapes (skip WAV generation)")

	flag.Parse()

	if *inputFile == "" {
		fmt.Fprintf(os.Stderr, "Error: --input required\n")
		os.Exit(1)
	}

	outputWav := "optimized.wav" // Always write to same location

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

	// Show which AY volume values are used (diagnostic)
	volumeValuesUsed := make(map[uint8]int)
	volumeInEnvMode := make(map[uint8]int)
	volumeInDirectMode := make(map[uint8]int)
	for _, cmd := range commands {
		// Volume registers: R8=A, R9=B, R10=C
		if cmd.Register >= 8 && cmd.Register <= 10 {
			vol := cmd.Value & 0x0F      // Lower 4 bits = volume (0-15)
			envMode := (cmd.Value & 0x10) != 0 // Bit 4 = envelope mode
			volumeValuesUsed[vol]++
			if envMode {
				volumeInEnvMode[vol]++
			} else {
				volumeInDirectMode[vol]++
			}
		}
	}
	fmt.Printf("AY volume values used in song: ")
	for vol := uint8(0); vol <= 15; vol++ {
		if count, used := volumeValuesUsed[vol]; used {
			fmt.Printf("%d(%d) ", vol, count)
		}
	}
	fmt.Printf("\n")
	fmt.Printf("  In envelope mode: ")
	for vol := uint8(0); vol <= 15; vol++ {
		if count := volumeInEnvMode[vol]; count > 0 {
			fmt.Printf("%d(%d) ", vol, count)
		}
	}
	fmt.Printf("\n")
	fmt.Printf("  In direct mode: ")
	for vol := uint8(0); vol <= 15; vol++ {
		if count := volumeInDirectMode[vol]; count > 0 {
			fmt.Printf("%d(%d) ", vol, count)
		}
	}
	fmt.Printf("\n\n")

	// Detect notes from AY volume commands (parallel)
	fmt.Printf("\nAnalyzing volume commands (3 channels in parallel)...\n")
	notes := [3][]Note{}
	var wg sync.WaitGroup
	for ch := 0; ch < 3; ch++ {
		wg.Add(1)
		go func(channel int) {
			defer wg.Done()
			notes[channel] = detectNotesFromAYVolume(commands, channel, maxFrame, *minNoteFrames, *verbose)
		}(ch)
	}
	wg.Wait()
	for ch := 0; ch < 3; ch++ {
		fmt.Printf("  Channel %c: Found %d notes\n", 'A'+ch, len(notes[ch]))
	}

	// Phase 1: Extract envelope curves from all channels (no ADSR fitting yet, parallel)
	fmt.Printf("\n=== Analyzing Volume Phases ===\n")
	fmt.Printf("Extracting envelopes (3 channels in parallel)...\n")

	channelTypes := [3][]EnvelopeType{}
	for ch := 0; ch < 3; ch++ {
		wg.Add(1)
		go func(channel int) {
			defer wg.Done()
			// Use non-overlapping ID ranges: Ch A=0+, Ch B=1000+, Ch C=2000+
			typeIDOffset := channel * 1000
			channelTypes[channel] = extractAndCategorizeEnvelopes(notes[channel], false, typeIDOffset)

			// Mark channel and wrap note indices
			for i := range channelTypes[channel] {
				channelTypes[channel][i].ChannelID = channel
				channelTypes[channel][i].ChannelNotes = []struct {
					Channel int
					Indices []int
				}{{Channel: channel, Indices: channelTypes[channel][i].NoteIndices}}
			}
		}(ch)
	}
	wg.Wait()

	// Merge all channel types
	allTypes := []EnvelopeType{}
	for ch := 0; ch < 3; ch++ {
		fmt.Printf("  Channel %c: %d envelope types\n", 'A'+ch, len(channelTypes[ch]))
		allTypes = append(allTypes, channelTypes[ch]...)
	}

	totalTypesBeforeMerge := len(allTypes)
	fmt.Printf("  Total: %d envelope types across all channels\n", totalTypesBeforeMerge)

	// Phase 1.5: Merge types with identical AY volume curves across channels
	globalTypes := []EnvelopeType{}
	used := make([]bool, len(allTypes))

	for i := range allTypes {
		if used[i] {
			continue
		}

		merged := allTypes[i]
		used[i] = true

		// Find all other types that match this one
		for j := i + 1; j < len(allTypes); j++ {
			if used[j] {
				continue
			}

			similarity := curveSimilarity(allTypes[i].RepresentCurve, allTypes[j].RepresentCurve)
			if volumeCurvesEqual(allTypes[i].RepresentCurve, allTypes[j].RepresentCurve) {
				// Merge type j into this type
				merged.ChannelNotes = append(merged.ChannelNotes, allTypes[j].ChannelNotes...)
				used[j] = true
				if *verbose {
					fmt.Printf("  Merged TYPE %d (channel %c) into TYPE %d (channel %c), similarity=%.3f\n",
						allTypes[j].ID, 'A'+allTypes[j].ChannelID,
						allTypes[i].ID, 'A'+allTypes[i].ChannelID, similarity)
				}
			}
		}

		globalTypes = append(globalTypes, merged)
	}

	if len(globalTypes) < totalTypesBeforeMerge {
		fmt.Printf("  Merged %d types into %d unique global types\n", totalTypesBeforeMerge, len(globalTypes))
	}

	// Phase 2: Fit ADSR once per global type (not per-channel!)
	fmt.Printf("  Phase 2: Fitting ADSR for %d global envelope types...\n", len(globalTypes))
	for i := range globalTypes {
		curve := globalTypes[i].RepresentCurve
		if len(curve) == 0 {
			continue
		}

		// Find optimal Attack value to reach first frame's level after 1 frame
		// Gate turns OFF after 1 frame, so attack only lasts 1 frame
		// AY volume table (logarithmic)
		ayVolumeTable := [16]float64{
			0, 327, 473, 690, 1006, 1492, 2113, 3518,
			4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
		}

		// Get first frame and peak values
		firstVol := int(globalTypes[i].RepresentCurve[0] + 0.5)
		if firstVol < 0 {
			firstVol = 0
		}
		if firstVol > 15 {
			firstVol = 15
		}

		peakVal := 0.0
		for _, v := range globalTypes[i].RepresentCurve {
			if v > peakVal {
				peakVal = v
			}
		}
		peakVol := int(peakVal + 0.5)

		var attack uint8

		// If first frame is at peak, use A=0 (instant attack)
		// Otherwise use A=3 for rising attack (~24ms)
		if firstVol >= peakVol {
			attack = 0
		} else {
			// Has rising attack - use A=3 (only allowed non-zero attack value)
			attack = 3
		}

		// Get target amplitude for S parameter
		targetAmp := ayVolumeTable[firstVol] / 32767.0   // Linear amplitude (0-1)

		// S parameter: set to the level we reach after attack (for ADSR display purposes)
		// In practice it doesn't matter since gate turns off after 1 frame
		sustainParam := uint8((targetAmp * 15.0) + 0.5) // SID S parameter (0-15)

		// Set ADSR: A=0/3, D=0, S=calculated, R=11 (R will be dynamic per frame)
		globalTypes[i].ADSR = ADSRParams{Attack: attack, Decay: 0, Sustain: sustainParam, Release: 11}
		globalTypes[i].GateOffFrame = 1 // Gate OFF after 1 frame (attack)

		// Compute ReleasePerFrame from representative curve
		// Pass attack parameter so it knows where SID level starts for release
		globalTypes[i].ReleasePerFrame = computeReleaseForCurveWithAttack(globalTypes[i].RepresentCurve, attack, sustainParam)
	}

	// Phase 3: Apply global types to notes in each channel
	fmt.Printf("  Phase 3: Applying global types to notes...\n")
	for i := range globalTypes {
		for _, cn := range globalTypes[i].ChannelNotes {
			ch := cn.Channel
			for _, noteIdx := range cn.Indices {
				if noteIdx < len(notes[ch]) {
					notes[ch][noteIdx].ADSR = globalTypes[i].ADSR
					notes[ch][noteIdx].GateOffFrame = globalTypes[i].GateOffFrame
					notes[ch][noteIdx].EnvelopeTypeID = globalTypes[i].ID

					// Copy ReleasePerFrame
					noteFrames := len(notes[ch][noteIdx].VolumeCurve)
					notes[ch][noteIdx].ReleasePerFrame = make([]uint8, noteFrames)
					copyLen := min(noteFrames, len(globalTypes[i].ReleasePerFrame))
					copy(notes[ch][noteIdx].ReleasePerFrame, globalTypes[i].ReleasePerFrame[:copyLen])
					if noteFrames > len(globalTypes[i].ReleasePerFrame) && len(globalTypes[i].ReleasePerFrame) > 0 {
						lastR := globalTypes[i].ReleasePerFrame[len(globalTypes[i].ReleasePerFrame)-1]
						for j := len(globalTypes[i].ReleasePerFrame); j < noteFrames; j++ {
							notes[ch][noteIdx].ReleasePerFrame[j] = lastR
						}
					}
				}
			}
		}
	}

	// Phase 3.5: Envelope mode notes have empty VolumeCurve (all zeros) so they're skipped
	// during curve extraction and don't get ReleasePerFrame values. They don't need R values
	// because gate stays ON throughout (never enters release phase). Amplitude modulation
	// comes from waveform toggling, not SID envelope.
	// We still need to initialize ReleasePerFrame to prevent nil access.
	for ch := range notes {
		for i := range notes[ch] {
			if notes[ch][i].EnvelopeMode && len(notes[ch][i].ReleasePerFrame) == 0 {
				noteFrames := notes[ch][i].EndFrame - notes[ch][i].StartFrame + 1
				notes[ch][i].ReleasePerFrame = make([]uint8, noteFrames)
				// R values don't matter since gate stays ON, but initialize to 0 for consistency
				// (matches the initial ADSR.Release=0 set when note was created)
				for j := 0; j < noteFrames; j++ {
					notes[ch][i].ReleasePerFrame[j] = 0
				}
			}
		}
	}

	// Phase 3.6: Assign fallback ADSR to non-envelope notes not covered by global types
	// These are typically short notes (< 10 frames) skipped during envelope extraction
	fallbackCount := 0
	for ch := range notes {
		for i := range notes[ch] {
			// Skip envelope mode notes
			if notes[ch][i].EnvelopeMode {
				continue
			}
			// Check if note has no ADSR assigned (still has zero-initialized values)
			if notes[ch][i].ADSR.Attack == 0 && notes[ch][i].ADSR.Sustain == 0 {
				// Assign fallback ADSR with instant attack (A=0), medium sustain
				notes[ch][i].ADSR = ADSRParams{Attack: 0, Decay: 9, Sustain: 9, Release: 6}
				notes[ch][i].GateOffFrame = len(notes[ch][i].VolumeCurve)
				notes[ch][i].EnvelopeTypeID = -1

				// Initialize ReleasePerFrame with fallback R value
				noteFrames := len(notes[ch][i].VolumeCurve)
				notes[ch][i].ReleasePerFrame = make([]uint8, noteFrames)
				for j := 0; j < noteFrames; j++ {
					notes[ch][i].ReleasePerFrame[j] = 6
				}
				fallbackCount++
			}
		}
	}
	if fallbackCount > 0 {
		fmt.Printf("  Applied fallback ADSR to %d unmatched notes\n", fallbackCount)
	}

	// Show verbose envelope details for merged global types
	if *verbose {
		fmt.Printf("\n=== Global Envelope Type Details ===\n")
		for i, envType := range globalTypes {
			if i >= 10 {
				break // Limit verbose output
			}
			totalNotes := 0
			channelList := ""
			for _, cn := range envType.ChannelNotes {
				totalNotes += len(cn.Indices)
				if channelList != "" {
					channelList += ", "
				}
				channelList += fmt.Sprintf("%c:%d", 'A'+cn.Channel, len(cn.Indices))
			}
			fmt.Printf("\n      === TYPE %d (%d notes, %s) ===\n", envType.ID, totalNotes, channelList)

			// Show envelope visualization
			maxVal := 0.0
			uniqueVals := make(map[int]bool)
			for _, v := range envType.RepresentCurve {
				if v > maxVal {
					maxVal = v
				}
				uniqueVals[int(v+0.5)] = true
			}
			fmt.Printf("      [%d frames: %.1f → %.1f, max: %.1f]\n",
				len(envType.RepresentCurve), envType.RepresentCurve[0],
				envType.RepresentCurve[len(envType.RepresentCurve)-1], maxVal)

			// Show unique volume values in this envelope
			fmt.Printf("      Unique AY volumes in curve: ")
			for vol := 0; vol <= 15; vol++ {
				if uniqueVals[vol] {
					fmt.Printf("%d ", vol)
				}
			}
			fmt.Printf("\n")

			// AY volume table (logarithmic, from Ayumi - matches emulator)
			ayVolumeTable := [16]float64{
				0, 327, 473, 690, 1006, 1492, 2113, 3518,
				4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
			}

			// Convert raw AY volumes (0-15) to linear amplitude for visualization
			// RepresentCurve stores raw AY volume register values
			normalizedAY := make([]float64, len(envType.RepresentCurve))
			for j := range envType.RepresentCurve {
				ayVol := int(envType.RepresentCurve[j] + 0.5) // Round to nearest int
				if ayVol < 0 {
					ayVol = 0
				}
				if ayVol > 15 {
					ayVol = 15
				}
				// Convert to linear amplitude (0-1 scale)
				normalizedAY[j] = ayVolumeTable[ayVol] / 32767.0
			}

			// Simulate SID envelope with ADSR delay bug
			var simulated []float64
			if len(envType.ReleasePerFrame) > 0 {
				simulated = make([]float64, len(envType.RepresentCurve))

				// SID ADSR simulation:
				// Gate ON → Attack for 1 frame → Gate OFF at frame 1 → Release
				// Frame 0: attack phase
				// Frame 1+: release phase with ADSR bug simulation

				// Determine level after 1 frame of attack and initial rate counter
				var currentLevel uint8
				var rateCounter uint16
				if envType.ADSR.Attack > 0 {
					// Simulate exactly 1 frame of attack from 0
					attackSim := simulateAttackFromLevel(0, envType.ADSR.Attack, 1)
					endLevel := uint8(attackSim[0] * 255.0)
					currentLevel = endLevel

					// For visualization: show mean level during attack frame
					// Attack is linear from 0 to endLevel, so mean = (0 + endLevel) / 2
					meanLevel := float64(endLevel) / 2.0 / 255.0
					simulated[0] = meanLevel

					// Rate counter after attack frame
					attackPeriod := uint32(adsrRates[envType.ADSR.Attack])
					rateCounter = uint16(19656 % attackPeriod)
				} else {
					// A=0, D=0: very fast attack to 255, then very fast decay to sustain
					// By end of frame 0, we're at sustain level
					sustainLevel := uint8((float64(envType.ADSR.Sustain) / 15.0) * 255.0)
					currentLevel = sustainLevel
					// Show sustain level for frame 0
					simulated[0] = float64(sustainLevel) / 255.0

					// Rate counter after A=0 attack (period=8)
					rateCounter = uint16(19656 % 8) // ~4
				}

				// Start release from frame 1
				attackFrameIdx := 1

				// After attack completes: simulate release with dynamic R and ADSR bug
				for j := attackFrameIdx; j < len(simulated); j++ {
					if j < len(envType.ReleasePerFrame) {
						r := envType.ReleasePerFrame[j]
						// Use bug-aware simulation
						sim := simulateReleaseWithBug(currentLevel, r, 1, rateCounter)
						currentLevel = uint8(sim[0] * 255.0)
						simulated[j] = sim[0] // Already 0-1 normalized

						// Update rate counter for next frame
						ratePeriod := uint32(adsrRates[r])
						cyclesThisFrame := uint32(19656)
						for cyclesThisFrame > 0 {
							cyclesUntilStep := ratePeriod - uint32(rateCounter)
							if cyclesUntilStep > cyclesThisFrame {
								// Use uint32 to avoid overflow before checking 15-bit wraparound
								newCounter := uint32(rateCounter) + cyclesThisFrame
								if newCounter >= 0x8000 {
									rateCounter = uint16((newCounter + 1) & 0x7fff)
								} else {
									rateCounter = uint16(newCounter)
								}
								break
							}
							cyclesThisFrame -= cyclesUntilStep
							rateCounter = 0
						}
					}
				}
			}

			// Plot AY and SID side by side
			// Pass raw AY volumes for display, normalized AY for bar length
			fmt.Printf("      %s\n", plotEnvelopeSideBySide(envType.RepresentCurve, normalizedAY, simulated, envType.ReleasePerFrame, envType.ADSR.Attack))

			fmt.Printf("      → A=%d D=%d S=%d (dynamic R per frame)\n",
				envType.ADSR.Attack, envType.ADSR.Decay, envType.ADSR.Sustain)
		}
	}

	// Show all detected envelope types (global)
	fmt.Printf("\n=== Detected Global Envelope Types ===\n")
	fmt.Printf("Total: %d unique envelope types\n", len(globalTypes))
	for _, envType := range globalTypes {
		holdMs := float64(envType.GateOffFrame) * 20.0
		totalNotes := 0
		channelList := ""
		for _, cn := range envType.ChannelNotes {
			totalNotes += len(cn.Indices)
			if channelList != "" {
				channelList += ", "
			}
			channelList += fmt.Sprintf("%c:%d", 'A'+cn.Channel, len(cn.Indices))
		}
		fmt.Printf("  Type %d (%d notes, %s): A=%d D=%d S=%d H=%dms R=%d\n",
			envType.ID, totalNotes, channelList,
			envType.ADSR.Attack, envType.ADSR.Decay,
			envType.ADSR.Sustain, int(holdMs), envType.ADSR.Release)
	}

	// Compute median ADSR per channel
	fmt.Printf("\n=== Median ADSR per Channel ===\n")
	channelADSR := [3]ADSRParams{}
	for ch := 0; ch < 3; ch++ {
		channelADSR[ch] = computeMedianADSR(notes[ch])
		fmt.Printf("Channel %c: A=%d D=%d S=%d R=%d\n", 'A'+ch,
			channelADSR[ch].Attack, channelADSR[ch].Decay,
			channelADSR[ch].Sustain, channelADSR[ch].Release)
	}

	// Skip WAV generation if --shapes-only
	if *shapesOnly {
		// Analyze envelope modulation ratios
		analyzeModulationRatios(commands, notes)
		fmt.Printf("\n✓ Shape analysis complete (skipped WAV generation)\n")
		return
	}

	// Generate comparison WAV
	fmt.Printf("\n=== Generating Final WAV ===\n")
	ayAudio, sidAudio := generateComparisonWAV(commands, notes, outputWav)

	fmt.Printf("✓ Complete! Written to: %s\n", outputWav)
	fmt.Printf("\n7-channel WAV layout:\n")
	fmt.Printf("  0-1: AY Channel A, SID Voice 1\n")
	fmt.Printf("  2-3: AY Channel B, SID Voice 2\n")
	fmt.Printf("  4-5: AY Channel C, SID Voice 3\n")
	fmt.Printf("  6:   ADSR delay bug (-1=none, +1=A, +0.33=B, -0.33=C)\n")

	// Spectral and RMS analysis validation
	fmt.Printf("\n=== Conversion Quality Analysis ===\n")
	analyzeConversionQuality(ayAudio, sidAudio, notes, *verbose)
}

// isValidEnvelopeShape checks if a curve represents a valid ADSR shape:
// - Must rise once (or start at peak)
// - Then fall once (or stay at peak)
// - No rises after falling
func isValidEnvelopeShape(values []float64, tolerance float64) bool {
	if len(values) == 0 {
		return false
	}

	// Find peak
	peakIdx := 0
	peakValue := values[0]
	for j := 1; j < len(values); j++ {
		if values[j] > peakValue {
			peakValue = values[j]
			peakIdx = j
		}
	}

	// Check monotonicity: must be non-decreasing before peak, non-increasing after
	for j := 1; j <= peakIdx; j++ {
		if values[j] < values[j-1]-tolerance {
			return false
		}
	}
	for j := peakIdx + 1; j < len(values); j++ {
		if values[j] > values[j-1]+tolerance {
			return false
		}
	}

	return true
}

// analyzeVolumePhases analyzes notes using envelope type clustering
func analyzeVolumePhases(notes []Note, verbose bool) []EnvelopeType {
	return analyzeVolumePhasesWithOffset(notes, verbose, 0)
}

// analyzeVolumePhasesWithOffset analyzes notes with a global type ID offset
func analyzeVolumePhasesWithOffset(notes []Note, verbose bool, typeIDOffset int) []EnvelopeType {
	// Phase 1: Extract envelope shapes from all notes
	envelopeTypes := extractAndCategorizeEnvelopes(notes, verbose, typeIDOffset)

	// Phase 2: Fit ADSR once per envelope type
	fitADSRPerEnvelopeType(envelopeTypes, notes, verbose)

	// Phase 2.5: Merge types with identical ADSR values
	envelopeTypes = mergeIdenticalADSRTypes(envelopeTypes, verbose)

	// Phase 3: Apply precomputed ADSR to notes
	applyEnvelopeTypesToNotes(notes, envelopeTypes)

	return envelopeTypes
}

// extractAndCategorizeEnvelopes extracts all envelope curves and clusters them
func extractAndCategorizeEnvelopes(notes []Note, verbose bool, typeIDOffset int) []EnvelopeType {
	fmt.Printf("  Phase 1: Extracting envelope shapes...\n")

	// Extract normalized curves from non-envelope-mode notes
	var candidates []candidateCurve
	skippedEnvMode := 0
	skippedSilent := 0
	candidatesWithRising := 0
	shortNotesWithRising := 0

	for i := range notes {
		// Skip envelope mode notes (already have A=0 D=0 S=15 R=0)
		if notes[i].EnvelopeMode {
			skippedEnvMode++
			continue
		}

		// Include short notes in categorization (no minimum length requirement)
		// Track short notes with rising volume for diagnostics
		if len(notes[i].VolumeCurve) < 10 && len(notes[i].VolumeCurve) >= 2 {
			for j := 1; j < len(notes[i].VolumeCurve) && j < 5; j++ {
				if notes[i].VolumeCurve[j] > notes[i].VolumeCurve[j-1]+0.3 {
					shortNotesWithRising++
					if verbose && shortNotesWithRising <= 3 {
						fmt.Printf("  DEBUG: Short note %d (%d frames) has rising: [%.1f, %.1f, %.1f]\n",
							i, len(notes[i].VolumeCurve), notes[i].VolumeCurve[0], notes[i].VolumeCurve[1],
							notes[i].VolumeCurve[2])
					}
					break
				}
			}
		}

		// Use raw curve (0-15 scale, no normalization)
		raw := notes[i].VolumeCurve

		// Check if silent
		peakVal := 0.0
		for _, r := range raw {
			if r > peakVal {
				peakVal = r
			}
		}
		if peakVal < 0.01 {
			skippedSilent++
			continue // Silent note
		}

		// Validate: notes can only rise once then fall once
		if !isValidEnvelopeShape(raw, 0.75) { // Allow small fluctuations (~5% of 15)
			// Skip this note - invalid shape (multiple peaks or rises after fall)
			continue
		}

		// Find peak for rise detection
		peakIdx := 0
		peakValue := raw[0]
		for j := 1; j < len(raw); j++ {
			if raw[j] > peakValue {
				peakValue = raw[j]
				peakIdx = j
			}
		}

		// Check if note has rising volume at start
		hasRising := peakIdx > 0 && raw[peakIdx] > raw[0]+1.5 // ~10% of 15
		if hasRising {
			candidatesWithRising++
		}

		// Check if note is complete (ends near zero)
		endVal := raw[len(raw)-1]
		isComplete := endVal < 3.0 // ~20% of 15

		candidates = append(candidates, candidateCurve{
			noteIdx:    i,
			normalized: raw, // Store raw values (despite field name)
			isComplete: isComplete,
		})
	}

	if verbose {
		fmt.Printf("  Found %d candidate envelope curves (%d with rising attack, %d short)\n", len(candidates), candidatesWithRising, shortNotesWithRising)
		fmt.Printf("  Skipped: %d envelope mode, %d silent\n", skippedEnvMode, skippedSilent)
	} else {
		fmt.Printf("  Found %d candidate envelope curves\n", len(candidates))
	}

	// Cluster curves by similarity
	types := clusterEnvelopeCurves(candidates, verbose, typeIDOffset)

	fmt.Printf("  Categorized into %d envelope types\n", len(types))
	return types
}

// clusterEnvelopeCurves clusters similar envelope curves
func clusterEnvelopeCurves(candidates []candidateCurve, verbose bool, typeIDOffset int) []EnvelopeType {
	if len(candidates) == 0 {
		return nil
	}

	var types []EnvelopeType

	for _, cand := range candidates {
		// Try to find existing type that matches
		foundMatch := false
		for typeIdx := range types {
			similarity := curveSimilarity(cand.normalized, types[typeIdx].RepresentCurve)
			// Match if curves are very similar (accounting for floating point precision)
			// Envelopes with identical overlapping portions cluster together
			if similarity < 0.01 {
				// Nearly identical - definitely same type
				types[typeIdx].NoteIndices = append(types[typeIdx].NoteIndices, cand.noteIdx)
				foundMatch = true
				break
			}
		}

		if !foundMatch {
			// Create new type
			newType := EnvelopeType{
				ID:             typeIDOffset + len(types),
				RepresentCurve: cand.normalized,
				NoteIndices:    []int{cand.noteIdx},
			}
			types = append(types, newType)
		}
	}

	// Update representative curves to use the longest (complete if possible) note in each type
	for i := range types {
		bestIdx := -1
		bestLength := 0
		bestIsComplete := false

		for _, noteIdx := range types[i].NoteIndices {
			// Find this candidate
			for _, cand := range candidates {
				if cand.noteIdx == noteIdx {
					// Prefer: 1) complete over incomplete, 2) longer over shorter
					betterChoice := false
					if !bestIsComplete && cand.isComplete {
						betterChoice = true // First complete note always better
					} else if bestIsComplete == cand.isComplete {
						// Same completeness - prefer longer
						if len(cand.normalized) > bestLength {
							betterChoice = true
						}
					}

					if bestIdx == -1 || betterChoice {
						bestLength = len(cand.normalized)
						bestIdx = noteIdx
						bestIsComplete = cand.isComplete
					}
					break
				}
			}
		}

		// Update representative curve to the best one
		if bestIdx >= 0 {
			for _, cand := range candidates {
				if cand.noteIdx == bestIdx {
					types[i].RepresentCurve = cand.normalized
					break
				}
			}
		}
	}

	return types
}

// curveSimilarity computes RMS error between two curves (lower = more similar)
// Compares only the overlapping portion without resampling to avoid distortion
// Ignores trailing near-zero frames to allow same decay with different note-off times to match
func curveSimilarity(curve1, curve2 []float64) float64 {
	// Find effective length of each curve (ignoring trailing near-zero frames)
	effectiveLen1 := len(curve1)
	for i := len(curve1) - 1; i >= 0; i-- {
		if curve1[i] > 0.5 { // Ignore frames below 0.5 volume
			effectiveLen1 = i + 1
			break
		}
	}
	if effectiveLen1 == 0 {
		effectiveLen1 = len(curve1)
	}

	effectiveLen2 := len(curve2)
	for i := len(curve2) - 1; i >= 0; i-- {
		if curve2[i] > 0.5 {
			effectiveLen2 = i + 1
			break
		}
	}
	if effectiveLen2 == 0 {
		effectiveLen2 = len(curve2)
	}

	// Compare only the overlapping effective portion
	length := effectiveLen1
	if effectiveLen2 < length {
		length = effectiveLen2
	}
	if length < 2 {
		length = 2
	}

	// Compare directly without resampling
	sumSquaredError := 0.0
	for i := 0; i < length; i++ {
		diff := curve1[i] - curve2[i]
		sumSquaredError += diff * diff
	}
	return math.Sqrt(sumSquaredError / float64(length))
}

// findSustainEnd finds the end of the sustain phase (before release starts)
// Returns the index where release begins, or length if no release detected
func findSustainEnd(curve []float64) int {
	if len(curve) < 3 {
		return len(curve)
	}

	// Find peak
	peakIdx := 0
	peakValue := curve[0]
	for i := 1; i < len(curve); i++ {
		if curve[i] > peakValue {
			peakValue = curve[i]
			peakIdx = i
		}
	}

	// After peak, find where curve stays relatively stable (sustain)
	// then starts falling consistently (release)
	sustainLevel := curve[peakIdx]
	for i := peakIdx + 1; i < len(curve)-2; i++ {
		// Update sustain level as we decay
		if curve[i] < sustainLevel {
			sustainLevel = curve[i]
		}

		// Check if next 3 points show consistent falling (release phase)
		falling := 0
		for j := 0; j < 3 && i+j+1 < len(curve); j++ {
			if curve[i+j+1] < curve[i+j]-0.5 { // Falling by >0.5
				falling++
			}
		}

		// If 2+ out of 3 points are falling, this is the release start
		if falling >= 2 {
			return i
		}
	}

	// No clear release detected - use full curve
	return len(curve)
}

// resampleCurve resamples a curve to a target length using linear interpolation
func resampleCurve(curve []float64, targetLength int) []float64 {
	if len(curve) == targetLength {
		return curve
	}

	result := make([]float64, targetLength)
	scale := float64(len(curve)-1) / float64(targetLength-1)

	for i := 0; i < targetLength; i++ {
		pos := float64(i) * scale
		idx := int(pos)
		frac := pos - float64(idx)

		if idx >= len(curve)-1 {
			result[i] = curve[len(curve)-1]
		} else {
			result[i] = curve[idx]*(1-frac) + curve[idx+1]*frac
		}
	}

	return result
}

// fitADSRPerEnvelopeType fits ADSR parameters for each envelope type
func fitADSRPerEnvelopeType(types []EnvelopeType, notes []Note, verbose bool) {
	if len(types) == 0 {
		return
	}

	fmt.Printf("  Phase 2: Fitting ADSR for %d envelope types...\n", len(types))

	for i := range types {
		if len(types[i].RepresentCurve) == 0 {
			continue
		}

		// Print type header if verbose
		if verbose && i < 5 {
			fmt.Printf("\n      === TYPE %d (%d notes) ===\n",
				types[i].ID, len(types[i].NoteIndices))
		}

		// Compute ReleasePerFrame directly from representative curve
		// This ensures consistent fitting regardless of which note is first
		types[i].ReleasePerFrame = computeReleaseForCurve(types[i].RepresentCurve)

		// Fit ADSR for this type's representative curve
		types[i].ADSR, types[i].GateOffFrame = detectVolumePhasesDebug(types[i].RepresentCurve, types[i].ReleasePerFrame, verbose && i < 10)
	}

	fmt.Printf("  ADSR fitting complete\n")
}

// calculateGateOffFrame determines where gate should turn off for an envelope
func calculateGateOffFrame(volumeCurve []float64) int {
	if len(volumeCurve) == 0 {
		return 0
	}

	// Find max value and end value
	maxVal := 0.0
	for _, v := range volumeCurve {
		if v > maxVal {
			maxVal = v
		}
	}
	endValue := volumeCurve[len(volumeCurve)-1]

	// Default: gate on full duration (for sustained envelopes)
	gateOffFrame := len(volumeCurve)

	// Check if there's a long sustain period at the end
	// Count consecutive equal frames at the end
	sustainFrames := 1
	for i := len(volumeCurve) - 2; i >= 0; i-- {
		if volumeCurve[i] == endValue {
			sustainFrames++
		} else {
			break
		}
	}

	// If envelope has a long sustain (> 1/3 of length), decide how to model it
	// High sustain level → use sustain phase (gate on throughout)
	// Low sustain level → use release phase (gate off early, let release model the tail)
	if sustainFrames > len(volumeCurve)/3 {
		// If sustained level is high (> 25% of peak), use sustain phase
		if endValue > maxVal*0.25 {
			return len(volumeCurve) // Gate on full duration - use sustain phase
		}
		// If sustained level is low (<= 25% of peak), use release phase
		// Gate off after initial decay, let release model the long flat tail
		// Find where initial decay reaches the sustained level
		for i := 0; i < len(volumeCurve)-sustainFrames; i++ {
			if volumeCurve[i] <= endValue*1.5 {
				gateOffFrame = i
				break
			}
		}
		return gateOffFrame
	}

	// If envelope decays to near zero with no long sustain, use release phase
	if endValue < maxVal/4 && sustainFrames < 5 {
		// Find where envelope has decayed to 1/4 peak - gate off there
		for i := 0; i < len(volumeCurve); i++ {
			if volumeCurve[i] <= maxVal/4 {
				gateOffFrame = i
				break
			}
		}
	}

	return gateOffFrame
}

// detectVolumePhasesDebug is a debug version that shows envelope shapes
func detectVolumePhasesDebug(volumeCurve []float64, releasePerFrame []uint8, debug bool) (ADSRParams, int) {
	if !debug {
		return detectVolumePhases(volumeCurve)
	}

	if len(volumeCurve) == 0 {
		return ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}, 0
	}

	// Detect attack from RMS envelope
	attack := uint8(0)
	if len(volumeCurve) > 1 {
		startVal := volumeCurve[0]
		peakVal := startVal
		for _, v := range volumeCurve {
			if v > peakVal {
				peakVal = v
			}
		}
		// If starts below 80% of peak, use A=3 (rising attack)
		if startVal < peakVal*0.8 && peakVal > 0.1 {
			attack = 3
		}
	}

	// Fixed ADSR: A=0/3, D=0, S=15
	result := ADSRParams{Attack: attack, Decay: 0, Sustain: 15, Release: 11}
	gateOffFrame := len(volumeCurve) // Gate stays on throughout

	// Find max to check scale
	maxVal := 0.0
	for _, v := range volumeCurve {
		if v > maxVal {
			maxVal = v
		}
	}

	// Print summary
	fmt.Printf("      [%d frames: %.1f → %.1f, max: %.1f]\n",
		len(volumeCurve), volumeCurve[0], volumeCurve[len(volumeCurve)-1], maxVal)

	// Generate and print ASCII visualization
	visualLines := len(volumeCurve)
	if visualLines > 30 {
		visualLines = 30
	}
	if visualLines < 10 {
		visualLines = 10
	}

	// Normalize for visualization (0-15 range → 0-1 for plotEnvelope)
	normalized := make([]float64, len(volumeCurve))
	for i := range volumeCurve {
		normalized[i] = volumeCurve[i] / 15.0
	}

	fmt.Printf("      AY:\n")
	fmt.Printf("      %s\n", plotEnvelope(normalized, visualLines))

	// Simulate with dynamic R values if available
	var simulated []float64
	if len(releasePerFrame) > 0 {
		simulated = simulateDynamicR(attack, releasePerFrame, len(volumeCurve))
	} else {
		// Fallback: use R=0 (no decay)
		simulated = simulateFullADSR(attack, 0, 15, 0, len(volumeCurve), len(volumeCurve))
	}
	fmt.Printf("      SID:\n")
	fmt.Printf("      %s\n", plotEnvelope(simulated, visualLines))

	// Print ADSR info
	fmt.Printf("      → A=%d D=0 S=15 (dynamic R per frame)\n", attack)

	return result, gateOffFrame
}


// plotEnvelope creates an ASCII visualization of an envelope (0-1 range)
// Time runs vertically (each line is a time step), volume horizontally (bar length)
func plotEnvelope(curve []float64, maxHeight int) string {
	if len(curve) == 0 {
		return ""
	}

	// Resample curve to fit height
	resampled := resampleCurve(curve, maxHeight)

	// Plot vertically: each line is a time step, bar length shows volume
	var result string
	for i, v := range resampled {
		// Each line shows volume as horizontal bar (up to 40 chars)
		barLength := int(v * 40.0)
		if barLength > 40 {
			barLength = 40
		}

		line := "|"
		for j := 0; j < barLength; j++ {
			line += "█"
		}

		// Pad remaining space with ▁
		for j := barLength; j < 40; j++ {
			line += "▁"
		}

		// Add newline except for last line
		if i < len(resampled)-1 {
			result += line + "\n      "
		} else {
			result += line
		}
	}
	return result
}

// plotEnvelopeSideBySide plots AY and SID envelopes side by side
// ayRaw: raw AY volume values (0-15) for display
// ayNorm: normalized linear amplitude (0-1) for bar length
// sid: SID envelope (0-1) for bar length
// releasePerFrame: R values per frame
// attack: Attack rate (to calculate initial rate counter)
func plotEnvelopeSideBySide(ayRaw, ayNorm, sid []float64, releasePerFrame []uint8, attack uint8) string {
	// Ensure all have same length
	maxLen := len(ayRaw)
	if len(ayNorm) > maxLen {
		maxLen = len(ayNorm)
	}
	if len(sid) > maxLen {
		maxLen = len(sid)
	}

	// Track rate counter for ADSR bug detection
	attackPeriod := uint32(adsrRates[attack])
	rateCounter := uint16(19656 % attackPeriod) // Counter after attack frame

	var result string
	// Header
	result = "AY:                                          SID:\n      "

	for i := 0; i < maxLen; i++ {
		var ayRawVal, ayNormVal, sidVal float64
		if i < len(ayRaw) {
			ayRawVal = ayRaw[i]
		}
		if i < len(ayNorm) {
			ayNormVal = ayNorm[i]
		}
		if i < len(sid) {
			sidVal = sid[i]
		}

		// AY bar (40 chars) - use normalized (logarithmic) amplitude
		// Round up for non-zero values so they always show at least one block
		ayLength := int(ayNormVal * 40.0)
		if ayLength == 0 && ayNormVal > 0 {
			ayLength = 1
		}
		if ayLength > 40 {
			ayLength = 40
		}
		line := ""
		for j := 0; j < ayLength; j++ {
			line += "█"
		}
		for j := ayLength; j < 40; j++ {
			line += "▁"
		}

		// Add AY raw volume value (0-15 register value)
		ayAmplitude := int(ayRawVal + 0.5)
		line += fmt.Sprintf(" %2d", ayAmplitude)

		line += "  " // Spacing between columns

		// SID bar (40 chars)
		// Round up for non-zero values so they always show at least one block
		sidLength := int(sidVal * 40.0)
		if sidLength == 0 && sidVal > 0 {
			sidLength = 1
		}
		if sidLength > 40 {
			sidLength = 40
		}
		for j := 0; j < sidLength; j++ {
			line += "█"
		}
		for j := sidLength; j < 40; j++ {
			line += "▁"
		}

		// Add R value with bug severity indicator if available
		if i < len(releasePerFrame) {
			r := releasePerFrame[i]
			ratePeriod := uint32(adsrRates[r])

			// Check if ADSR bug triggers at this frame (skip frame 0 which is attack)
			bugSeverity := ""
			if i > 0 && ratePeriod < uint32(rateCounter) {
				// Bug active! Calculate frozen cycles
				frozenCycles := 0x8000 - uint32(rateCounter)
				cyclesPerFrame := uint32(19656)

				// Only mark bugs with >= 10ms freeze
				if frozenCycles < 9852 {
					// < 10ms freeze - acceptable, don't mark
				} else if frozenCycles >= cyclesPerFrame {
					bugSeverity = "!!!!" // Entire frame frozen
				} else if frozenCycles >= cyclesPerFrame*3/4 {
					bugSeverity = "!!!" // 75%+ frozen
				} else if frozenCycles >= cyclesPerFrame/2 {
					bugSeverity = "!!" // 50%+ frozen
				} else {
					bugSeverity = "!" // >= 10ms but < 50% frame
				}
			}

			if bugSeverity != "" {
				line += fmt.Sprintf(" R=%d%s", r, bugSeverity)
			} else {
				line += fmt.Sprintf(" R=%d", r)
			}

			// Update rate counter for next frame
			cyclesThisFrame := uint32(19656)
			for cyclesThisFrame > 0 {
				cyclesUntilStep := ratePeriod - uint32(rateCounter)
				if cyclesUntilStep > cyclesThisFrame {
					// Use uint32 to avoid overflow before checking 15-bit wraparound
					newCounter := uint32(rateCounter) + cyclesThisFrame
					if newCounter >= 0x8000 {
						rateCounter = uint16((newCounter + 1) & 0x7fff)
					} else {
						rateCounter = uint16(newCounter)
					}
					break
				}
				cyclesThisFrame -= cyclesUntilStep
				rateCounter = 0
			}
		}

		// Add newline except for last line
		if i < maxLen-1 {
			result += line + "\n      "
		} else {
			result += line
		}
	}

	return result
}

// mergeIdenticalADSRTypes merges envelope types with identical AY volume curves
// Types with the same AY envelope shape are merged (compared before SID conversion)
func mergeIdenticalADSRTypes(types []EnvelopeType, verbose bool) []EnvelopeType {
	if len(types) <= 1 {
		return types
	}

	merged := []EnvelopeType{}
	used := make([]bool, len(types))

	for i := range types {
		if used[i] {
			continue
		}

		// Start new merged type
		newType := types[i]
		used[i] = true

		// Find all types that should merge with this one
		for j := i + 1; j < len(types); j++ {
			if used[j] {
				continue
			}

			// Merge if AY volume curves are identical or similar
			similarity := curveSimilarity(types[i].RepresentCurve, types[j].RepresentCurve)
			if volumeCurvesEqual(types[i].RepresentCurve, types[j].RepresentCurve) {
				// Merge type j into current type
				newType.NoteIndices = append(newType.NoteIndices, types[j].NoteIndices...)
				used[j] = true
				if verbose {
					fmt.Printf("  Merged TYPE %d into TYPE %d (AY curve similarity=%.3f)\n", types[j].ID, types[i].ID, similarity)
				}
			} else if verbose && similarity < 1.0 {
				// Show near-misses for debugging
				fmt.Printf("  TYPE %d vs TYPE %d: similarity=%.3f (not merged, threshold=0.5)\n", types[i].ID, types[j].ID, similarity)
			}
		}

		merged = append(merged, newType)
	}

	if len(merged) < len(types) {
		fmt.Printf("  Merged %d types with identical envelopes into %d unique types\n", len(types), len(merged))
	}

	return merged
}

// volumeCurvesEqual checks if two AY volume curves are similar enough to merge
// Uses curveSimilarity with a lenient threshold to match old D±1 behavior
func volumeCurvesEqual(a, b []float64) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) == 0 || len(b) == 0 {
		return false
	}

	// Use curveSimilarity (RMS error) with lenient threshold
	// Old D±1 rule was quite lenient, so use 0.5 (on 0-15 scale)
	similarity := curveSimilarity(a, b)
	return similarity < 0.5
}

// abs returns absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// applyEnvelopeTypesToNotes applies precomputed ADSR to notes
func applyEnvelopeTypesToNotes(notes []Note, types []EnvelopeType) {
	fmt.Printf("  Phase 3: Applying envelope types to notes...\n")

	// Build map: noteIdx -> typeID
	noteToType := make(map[int]int)
	for _, envType := range types {
		for _, noteIdx := range envType.NoteIndices {
			noteToType[noteIdx] = envType.ID
		}
	}

	// Apply ADSR from types to notes
	appliedCount := 0
	for i := range notes {
		if notes[i].EnvelopeMode {
			continue // Already has A=0 D=0 S=15 R=0
		}

		if typeID, found := noteToType[i]; found {
			// Find the type
			for _, envType := range types {
				if envType.ID == typeID {
					notes[i].ADSR = envType.ADSR
					notes[i].GateOffFrame = envType.GateOffFrame
					notes[i].EnvelopeTypeID = typeID

					// Copy ReleasePerFrame from type (truncate or pad if needed)
					noteFrames := len(notes[i].VolumeCurve)
					notes[i].ReleasePerFrame = make([]uint8, noteFrames)
					copyLen := min(noteFrames, len(envType.ReleasePerFrame))
					copy(notes[i].ReleasePerFrame, envType.ReleasePerFrame[:copyLen])
					// Pad with last R value if note is longer than type curve
					if noteFrames > len(envType.ReleasePerFrame) && len(envType.ReleasePerFrame) > 0 {
						lastR := envType.ReleasePerFrame[len(envType.ReleasePerFrame)-1]
						for j := len(envType.ReleasePerFrame); j < noteFrames; j++ {
							notes[i].ReleasePerFrame[j] = lastR
						}
					}

					appliedCount++
					break
				}
			}
		} else {
			// Fallback: use instant attack (A=0)
			notes[i].ADSR = ADSRParams{Attack: 0, Decay: 9, Sustain: 1, Release: 0}
			notes[i].GateOffFrame = len(notes[i].VolumeCurve) // Default: gate on full duration
			notes[i].EnvelopeTypeID = -1
		}
	}

	fmt.Printf("  Applied ADSR to %d notes\n", appliedCount)
}

// Old analyzeVolumePhases implementation (now unused, keeping detectVolumePhases)

// detectVolumePhases detects ADSR by searching for best fit through all combinations
func detectVolumePhases(volumeCurve []float64) (ADSRParams, int) {
	if len(volumeCurve) == 0 {
		return ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}, 0
	}

	// Don't normalize - fit directly to raw AY volume levels (0-15)
	// This preserves absolute volume information for better fitting
	maxVal := 0.0
	for _, v := range volumeCurve {
		if v > maxVal {
			maxVal = v
		}
	}

	if maxVal < 0.001 {
		// Flat envelope - treat as sustain only
		return ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}, len(volumeCurve)
	}

	// Check if there's an actual attack phase (rising volume at start)
	// Find where the global peak occurs in the envelope
	hasAttack := false
	peakIdx := 0
	peakVal := volumeCurve[0]
	if len(volumeCurve) >= 2 {
		for i := 1; i < len(volumeCurve); i++ {
			if volumeCurve[i] > peakVal {
				peakVal = volumeCurve[i]
				peakIdx = i
			}
		}
		// If peak is after start and envelope rises to it, we have an attack phase
		hasAttack = peakIdx > 0 && peakVal > volumeCurve[0]
	}

	// Truncate long sustain tails from the target envelope
	// Only truncate if envelope ends at significant sustain level (not near zero)
	truncatedLen := len(volumeCurve)
	endValue := volumeCurve[len(volumeCurve)-1]

	// Only consider truncation if envelope ends well above zero (has sustain phase)
	// If envelope decays to near zero, keep full length (it's attack+decay+release, no sustain)
	if len(volumeCurve) > 10 && endValue > maxVal/2 {
		// Look for 5 consecutive frames with no change (stable sustain)
		for i := 5; i < len(volumeCurve)-5; i++ {
			allSame := true
			for j := 1; j < 5; j++ {
				if volumeCurve[i+j] != volumeCurve[i] {
					allSame = false
					break
				}
			}
			if allSame {
				// Found start of stable sustain - keep a few more frames then truncate
				truncatedLen = i + 5
				break
			}
		}
	}

	// Use truncated envelope for fitting to avoid sustain tail dominating error
	truncatedRms := volumeCurve[:truncatedLen]

	// Determine gate-off frame based on envelope shape
	gateOffFrame := truncatedLen

	// Check if there's a long sustain period at the end
	sustainFrames := 1
	for i := len(truncatedRms) - 2; i >= 0; i-- {
		if truncatedRms[i] == endValue {
			sustainFrames++
		} else {
			break
		}
	}

	// For envelopes with long flat sustain (> 1/3 of length)
	// Use sustain phase for high levels, release phase for low levels
	targetSustain := -1
	if sustainFrames > len(truncatedRms)/3 {
		// Long flat tail detected
		// If sustained level is audible (> 0.5), use sustain phase
		// If sustained level is near-zero (<= 0.5), it's decaying to zero - use release
		if endValue > 0.5 {
			gateOffFrame = truncatedLen // Gate on throughout, sustain models the flat tail
			// Convert sustained level (0-15 AY scale) to sustain parameter (0-15)
			targetSustain = int(endValue + 0.5) // Round to nearest integer
			if targetSustain > 15 {
				targetSustain = 15
			}
		} else {
			// Near-zero sustained level - treat as decay to zero, use release phase
			// Gate off after initial decay reaches halfway to zero
			for i := peakIdx + 1; i < len(truncatedRms); i++ {
				if truncatedRms[i] <= maxVal/4 {
					gateOffFrame = i
					break
				}
			}
		}
	} else if endValue < maxVal*0.1 {
		// Envelope decays to <10% of peak (continuously decaying, no flat sustain)
		// Use long decay phase to model entire decay curve, set S=0 to decay all the way
		// Gate on for full duration (no early gate-off)
		gateOffFrame = truncatedLen
		targetSustain = 0 // Force S=0 to allow decay all the way to zero
	} else if endValue < maxVal/2 {
		// Envelope decays to medium level (not near zero, not high sustain)
		// Use gate-off to model final decay with release phase
		// Find where envelope has decayed to half its peak
		for i := peakIdx + 1; i < len(truncatedRms); i++ {
			if truncatedRms[i] <= maxVal/2 {
				gateOffFrame = i
				break
			}
		}
	}

	// Search through all ADSR combinations to find best fit
	// Pass raw AY volumes (0-15 scale) directly
	result := searchBestADSR(truncatedRms, hasAttack, gateOffFrame, targetSustain)

	return result, gateOffFrame
}

// searchBestADSR searches all ADSR combinations to find the one with minimum error
// Uses a coarse-to-fine search strategy for speed
// If targetSustain >= 0, only search for that specific sustain level
func searchBestADSR(target []float64, hasAttack bool, gateOffFrame int, targetSustain int) ADSRParams {
	// Stage 1: Coarse search (step by 4)
	bestADSR := searchADSRWithStep(target, 4, hasAttack, gateOffFrame, targetSustain)

	// Stage 2: Fine search around best result (±4 range, step by 1)
	bestADSR = searchADSRAroundPoint(target, bestADSR, 4, hasAttack, gateOffFrame, targetSustain)

	// Stage 3: If attack was detected, do a dedicated attack-only fine search
	// This is critical for matching rising phases accurately
	if hasAttack {
		bestADSR = refineAttackValue(target, bestADSR, gateOffFrame)
	}

	return bestADSR
}

// refineAttackValue does a fine search over all attack values while keeping D/S/R fixed
func refineAttackValue(target []float64, current ADSRParams, gateOffFrame int) ADSRParams {
	bestADSR := current
	bestError := 1e9

	// Only allow A=0 (instant) or A=3 (~20ms)
	attackValues := []uint8{0, 3}
	for _, a := range attackValues {
		simulated := simulateFullADSR(a, current.Decay, current.Sustain, current.Release, len(target), gateOffFrame)

		// Scale to 0-15 to match AY
		simulatedScaled := make([]float64, len(simulated))
		for i := range simulated {
			simulatedScaled[i] = simulated[i] * 15.0
		}

		// Compute weighted error: 3x weight on first 25% of curve (attack phase)
		attackEnd := len(target) / 4
		if attackEnd < 3 {
			attackEnd = 3
		}

		attackError := 0.0
		tailError := 0.0

		for i := 0; i < len(target); i++ {
			diff := target[i] - simulatedScaled[i]
			sqError := diff * diff

			if i < attackEnd {
				attackError += sqError * 3.0 // 3x weight
			} else {
				tailError += sqError
			}
		}

		totalError := math.Sqrt((attackError + tailError) / float64(len(target)))

		if totalError < bestError {
			bestError = totalError
			bestADSR = ADSRParams{
				Attack:  a,
				Decay:   current.Decay,
				Sustain: current.Sustain,
				Release: current.Release,
			}
		}
	}

	return bestADSR
}

// searchADSRWithStep searches with a given step size
// If targetSustain >= 0, only search for that specific sustain level
func searchADSRWithStep(target []float64, step uint8, hasAttack bool, gateOffFrame int, targetSustain int) ADSRParams {
	bestADSR := ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 11}
	bestError := 1e9

	// Only allow A=0 (instant) or A=3 (~20ms)
	attackValues := []uint8{0}
	if hasAttack {
		attackValues = []uint8{0, 3}
	}

	// Constrain sustain search if target is specified
	sMin := uint8(0)
	sMax := uint8(15)
	if targetSustain >= 0 {
		sMin = uint8(targetSustain)
		sMax = uint8(targetSustain)
	}

	for _, a := range attackValues {
		for d := uint8(0); d <= 15; d += step {
			for s := sMin; s <= sMax; s += step {
				for r := uint8(0); r <= 15; r += step {
					// Simulate this ADSR (returns 0-1 scale)
					simulated := simulateFullADSR(a, d, s, r, len(target), gateOffFrame)

					// Scale simulated SID envelope to 0-15 to match AY scale
					simulatedScaled := make([]float64, len(simulated))
					for i := range simulated {
						simulatedScaled[i] = simulated[i] * 15.0
					}

					// Compute RMS error
					error := computeRMSError(target, simulatedScaled)

					if error < bestError {
						bestError = error
						bestADSR = ADSRParams{
							Attack:  a,
							Decay:   d,
							Sustain: s,
							Release: r,
						}
					}

					// Early termination if error is very small
					if error < 0.05 { // Adjusted threshold for 0-15 scale
						return bestADSR
					}
				}
			}
		}
	}

	return bestADSR
}

// searchADSRAroundPoint does a fine search around a given ADSR point
// If targetSustain >= 0, only search for that specific sustain level
func searchADSRAroundPoint(target []float64, center ADSRParams, radius uint8, hasAttack bool, gateOffFrame int, targetSustain int) ADSRParams {
	bestADSR := center
	bestError := 1e9

	// Helper to clamp values to 0-15
	clamp := func(val int) uint8 {
		if val < 0 {
			return 0
		}
		if val > 15 {
			return 15
		}
		return uint8(val)
	}

	// Only allow A=0 or A=3
	attackValues := []uint8{0}
	if hasAttack {
		attackValues = []uint8{0, 3}
	}

	dMin := clamp(int(center.Decay) - int(radius))
	dMax := clamp(int(center.Decay) + int(radius))
	sMin := clamp(int(center.Sustain) - int(radius))
	sMax := clamp(int(center.Sustain) + int(radius))

	// Constrain sustain search if target is specified
	if targetSustain >= 0 {
		sMin = uint8(targetSustain)
		sMax = uint8(targetSustain)
	}

	rMin := clamp(int(center.Release) - int(radius))
	rMax := clamp(int(center.Release) + int(radius))

	for _, a := range attackValues {
		for d := dMin; d <= dMax; d++ {
			for s := sMin; s <= sMax; s++ {
				for r := rMin; r <= rMax; r++ {
					simulated := simulateFullADSR(a, d, s, r, len(target), gateOffFrame)

					// Scale to 0-15 to match AY
					simulatedScaled := make([]float64, len(simulated))
					for i := range simulated {
						simulatedScaled[i] = simulated[i] * 15.0
					}

					error := computeRMSError(target, simulatedScaled)

					if error < bestError {
						bestError = error
						bestADSR = ADSRParams{
							Attack:  a,
							Decay:   d,
							Sustain: s,
							Release: r,
						}
					}
				}
			}
		}
	}

	return bestADSR
}

// computeRMSError computes root mean square error between two curves
func computeRMSError(a, b []float64) float64 {
	if len(a) != len(b) {
		return 1e9
	}

	sumSquaredError := 0.0
	for i := range a {
		diff := a[i] - b[i]
		sumSquaredError += diff * diff
	}

	return math.Sqrt(sumSquaredError / float64(len(a)))
}

// simulateADS simulates Attack-Decay-Sustain envelope (gate stays on)
// Uses frame-level approximation with exponential rate dividers for fast search
func simulateADS(attack, decay, sustain uint8, numFrames int) []float64 {
	envelope := make([]float64, numFrames)
	cyclesPerFrame := sidClockHz / palFrameRate

	envelopeLevel := uint8(0)
	envelopeState := 0 // 0=Attack, 1=Decay/Sustain
	envelopeCounter := 0.0

	for frame := 0; frame < numFrames; frame++ {
		// Process full frame's worth of cycles at once
		remainingCycles := cyclesPerFrame

		for remainingCycles > 0 {
			switch envelopeState {
			case 0: // Attack - exponential rate (slows as level increases)
				ratePeriod := float64(adsrRates[attack])
				effectiveRate := ratePeriod
				if envelopeLevel >= 157 {
					effectiveRate = ratePeriod * 9
				} else if envelopeLevel >= 93 {
					effectiveRate = ratePeriod * 3
				}

				cyclesNeeded := effectiveRate - envelopeCounter
				if cyclesNeeded <= remainingCycles {
					envelopeCounter = 0
					remainingCycles -= cyclesNeeded
					if envelopeLevel < 255 {
						envelopeLevel++
					}
					if envelopeLevel >= 255 {
						envelopeLevel = 255
						envelopeState = 1
					}
				} else {
					envelopeCounter += remainingCycles
					remainingCycles = 0
				}

			case 1: // Decay/Sustain - exponential rate (slows as level decreases)
				sustainTarget := uint8(sustain) * 17
				if envelopeLevel > sustainTarget {
					ratePeriod := float64(adsrRates[decay])
					effectiveRate := ratePeriod
					if envelopeLevel < 93 {
						effectiveRate = ratePeriod * 9
					} else if envelopeLevel < 157 {
						effectiveRate = ratePeriod * 3
					}

					cyclesNeeded := effectiveRate - envelopeCounter
					if cyclesNeeded <= remainingCycles {
						envelopeCounter = 0
						remainingCycles -= cyclesNeeded
						if envelopeLevel > 0 {
							envelopeLevel--
						}
					} else {
						envelopeCounter += remainingCycles
						remainingCycles = 0
					}
				} else {
					remainingCycles = 0
				}
			}
		}
		envelope[frame] = float64(envelopeLevel) / 255.0
	}

	return envelope
}

// simulateR simulates Release envelope (starting from sustain level)
// Uses frame-level approximation with exponential rate dividers for fast search
func simulateR(release, sustain uint8, numFrames int) []float64 {
	envelope := make([]float64, numFrames)
	cyclesPerFrame := sidClockHz / palFrameRate

	envelopeLevel := uint8(sustain) * 17 // Start at sustain level
	envelopeCounter := 0.0

	for frame := 0; frame < numFrames; frame++ {
		// Process full frame's worth of cycles at once
		remainingCycles := cyclesPerFrame

		for remainingCycles > 0 && envelopeLevel > 0 {
			ratePeriod := float64(adsrRates[release])

			// Exponential rate - slows as level decreases
			effectiveRate := ratePeriod
			if envelopeLevel < 93 {
				effectiveRate = ratePeriod * 9
			} else if envelopeLevel < 157 {
				effectiveRate = ratePeriod * 3
			}

			cyclesNeeded := effectiveRate - envelopeCounter
			if cyclesNeeded <= remainingCycles {
				envelopeCounter = 0
				remainingCycles -= cyclesNeeded
				if envelopeLevel > 0 {
					envelopeLevel--
				}
			} else {
				envelopeCounter += remainingCycles
				remainingCycles = 0
			}
		}
		envelope[frame] = float64(envelopeLevel) / 255.0
	}

	return envelope
}

// simulateAttack simulates Attack phase only (0 to peak)
func simulateAttack(attack uint8, numFrames int) []float64 {
	envelope := make([]float64, numFrames)
	cyclesPerFrame := sidClockHz / palFrameRate

	envelopeLevel := uint8(0)
	envelopeCounter := 0.0

	for frame := 0; frame < numFrames; frame++ {
		remainingCycles := cyclesPerFrame

		for remainingCycles > 0 && envelopeLevel < 255 {
			ratePeriod := float64(adsrRates[attack])
			effectiveRate := ratePeriod
			if envelopeLevel >= 157 {
				effectiveRate = ratePeriod * 9
			} else if envelopeLevel >= 93 {
				effectiveRate = ratePeriod * 3
			}

			cyclesNeeded := effectiveRate - envelopeCounter
			if cyclesNeeded <= remainingCycles {
				envelopeCounter = 0
				remainingCycles -= cyclesNeeded
				if envelopeLevel < 255 {
					envelopeLevel++
				}
			} else {
				envelopeCounter += remainingCycles
				remainingCycles = 0
			}
		}
		envelope[frame] = float64(envelopeLevel) / 255.0
	}

	return envelope
}

// simulateDecay simulates Decay phase only (peak to sustain level)
func simulateDecay(decay, sustain uint8, numFrames int) []float64 {
	envelope := make([]float64, numFrames)
	cyclesPerFrame := sidClockHz / palFrameRate

	envelopeLevel := uint8(255) // Start at peak
	envelopeCounter := 0.0
	sustainTarget := uint8(sustain) * 17

	for frame := 0; frame < numFrames; frame++ {
		remainingCycles := cyclesPerFrame

		for remainingCycles > 0 && envelopeLevel > sustainTarget {
			ratePeriod := float64(adsrRates[decay])
			effectiveRate := ratePeriod
			if envelopeLevel < 93 {
				effectiveRate = ratePeriod * 9
			} else if envelopeLevel < 157 {
				effectiveRate = ratePeriod * 3
			}

			cyclesNeeded := effectiveRate - envelopeCounter
			if cyclesNeeded <= remainingCycles {
				envelopeCounter = 0
				remainingCycles -= cyclesNeeded
				if envelopeLevel > 0 {
					envelopeLevel--
				}
			} else {
				envelopeCounter += remainingCycles
				remainingCycles = 0
			}
		}
		envelope[frame] = float64(envelopeLevel) / 255.0
	}

	return envelope
}

// amplitudeError computes RMS error between two envelopes
func amplitudeError(actual, simulated []float64) float64 {
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
	return amplitudeError(normalized, simulated)
}

// simulateFullADSR simulates full ADSR with gate on/off at specific frame
func simulateFullADSR(attack, decay, sustain, release uint8, numFrames, gateOffFrame int) []float64 {
	envelope := make([]float64, numFrames)
	cyclesPerFrame := sidClockHz / palFrameRate

	envelopeLevel := uint8(0)
	envelopeState := 0 // 0=Attack, 1=Decay/Sustain, 2=Release
	envelopeCounter := 0.0
	exponentialCounter := 0
	exponentialPeriod := 1

	// updateExponentialPeriod matches hardware behavior
	updateExponentialPeriod := func() {
		switch envelopeLevel {
		case 255:
			exponentialPeriod = 1
		case 93:
			exponentialPeriod = 2
		case 54:
			exponentialPeriod = 4
		case 26:
			exponentialPeriod = 8
		case 14:
			exponentialPeriod = 16
		case 6:
			exponentialPeriod = 30
		case 0:
			exponentialPeriod = 1
		}
	}

	for frame := 0; frame < numFrames; frame++ {
		// Gate off at specified frame
		if frame == gateOffFrame && envelopeState != 2 {
			envelopeState = 2
			envelopeCounter = 0
			exponentialCounter = 0
		}

		for cycle := 0.0; cycle < cyclesPerFrame; cycle++ {
			envelopeCounter++

			ratePeriod := float64(adsrRates[attack])
			switch envelopeState {
			case 0: // Attack
				ratePeriod = float64(adsrRates[attack])
			case 1: // Decay/Sustain
				ratePeriod = float64(adsrRates[decay])
			case 2: // Release
				ratePeriod = float64(adsrRates[release])
			}

			if envelopeCounter >= ratePeriod {
				envelopeCounter = 0

				shouldStep := false
				if envelopeState == 0 {
					// Attack: linear, always step and reset exponential counter
					exponentialCounter = 0
					shouldStep = true
				} else {
					// Decay/Release: exponential, step only when exponential counter expires
					exponentialCounter++
					if exponentialCounter >= exponentialPeriod {
						exponentialCounter = 0
						shouldStep = true
					}
				}

				if shouldStep {
					switch envelopeState {
					case 0: // Attack
						if envelopeLevel < 255 {
							envelopeLevel++
							updateExponentialPeriod()
						}
						if envelopeLevel >= 255 {
							envelopeLevel = 255
							envelopeState = 1
						}

					case 1: // Decay/Sustain
						sustainTarget := uint8(sustain) * 17
						if envelopeLevel > sustainTarget {
							if envelopeLevel > 0 {
								envelopeLevel--
								updateExponentialPeriod()
							}
						}

					case 2: // Release
						if envelopeLevel > 0 {
							envelopeLevel--
							updateExponentialPeriod()
						}
					}
				}
			}
		}
		envelope[frame] = float64(envelopeLevel) / 255.0
	}

	return envelope
}

// simulateDynamicR simulates gate ON for 1 frame (attack), then gate OFF with dynamic R per frame
// Includes ADSR delay bug simulation for accurate envelope display
func simulateDynamicR(attack uint8, releasePerFrame []uint8, numFrames int) []float64 {
	envelope := make([]float64, numFrames)

	// Frame 0: Attack to peak
	currentLevel := uint8(255) // Assume instant attack (A=0 or A=3 both reach 255 in ~1 frame)
	envelope[0] = 1.0

	// Rate counter after attack frame
	attackPeriod := uint32(adsrRates[attack])
	rateCounter := uint16(19656 % attackPeriod)

	// Frame 1+: Release with dynamic R and ADSR bug
	for frame := 1; frame < numFrames; frame++ {
		// Get R value for this frame
		currentR := uint8(11) // Default if out of range
		if frame < len(releasePerFrame) {
			currentR = releasePerFrame[frame]
		}

		// Simulate one frame of release with ADSR bug
		simulated := simulateReleaseWithBug(currentLevel, currentR, 1, rateCounter)
		envelope[frame] = simulated[0]

		// Update current level for next frame
		currentLevel = uint8(simulated[0] * 255.0)

		// Update rate counter for next frame
		ratePeriod := uint32(adsrRates[currentR])
		cyclesThisFrame := uint32(19656)
		for cyclesThisFrame > 0 {
			cyclesUntilStep := ratePeriod - uint32(rateCounter)
			if cyclesUntilStep > cyclesThisFrame {
				// Use uint32 to avoid overflow before checking 15-bit wraparound
				newCounter := uint32(rateCounter) + cyclesThisFrame
				if newCounter >= 0x8000 {
					rateCounter = uint16((newCounter + 1) & 0x7fff)
				} else {
					rateCounter = uint16(newCounter)
				}
				break
			}
			cyclesThisFrame -= cyclesUntilStep
			rateCounter = 0
		}
	}

	return envelope
}

// computeMedianADSR computes channel-wide median ADSR
func computeMedianADSR(notes []Note) ADSRParams {
	if len(notes) == 0 {
		return ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}
	}

	var adsrs []ADSRParams
	for _, note := range notes {
		// Skip envelope mode notes (they don't use ADSR)
		if note.EnvelopeMode {
			continue
		}
		adsrs = append(adsrs, note.ADSR)
	}

	// If no direct mode notes, return default
	if len(adsrs) == 0 {
		return ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}
	}

	return computeMedianADSRFromList(adsrs)
}

func computeMedianADSRFromList(adsrs []ADSRParams) ADSRParams {
	if len(adsrs) == 0 {
		return ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}
	}

	var attacks, decays, sustains, releases []uint8
	for _, adsr := range adsrs {
		attacks = append(attacks, adsr.Attack)
		decays = append(decays, adsr.Decay)
		sustains = append(sustains, adsr.Sustain)
		releases = append(releases, adsr.Release)
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

// ayEnvelopeToSIDWaveform maps AY envelope shape to SID waveform
// AY Envelope shapes (R13):
//
//	0x00-0x07: Single-cycle envelopes
//	0x08: \\\\\ Repeating decay (sawtooth down)
//	0x0A: \/\/\/ Repeating triangle
//	0x0C: ///// Repeating attack (sawtooth up)
//	0x0E: /\/\/\ Repeating triangle (inverted)
//
// SID waveforms: 0x01=triangle, 0x02=sawtooth, 0x04=pulse, 0x08=noise
// AY envelope shapes: R13 bits [3:0] define 16 shapes
func ayEnvelopeToSIDWaveform(ayShape byte) byte {
	switch ayShape & 0x0F {
	case 0x0A, 0x0E:
		// /\/\/\ repeating triangle (alternate + continue)
		return 0x01 // SID triangle
	case 0x04, 0x05, 0x06, 0x07, 0x0C, 0x0D, 0x0F:
		// / upward ramps: single or hold at max
		// 0x04-0x07: single ramp up
		// 0x0C: //// repeating ramp up (sawtooth)
		// 0x0D, 0x0F: ramp up then hold at max
		return 0x02 // SID sawtooth (upward ramp)
	case 0x00, 0x01, 0x02, 0x03, 0x08, 0x09, 0x0B:
		// \ downward ramps: can approximate with sawtooth + ADSR decay
		// 0x00-0x03: single ramp down
		// 0x08: \\\\ repeating ramp down (sawtooth)
		// 0x09: ramp down then hold at 0
		// 0x0B: ramp down then hold at max
		return 0x02 // SID sawtooth (closest approximation)
	default:
		return 0x04 // SID pulse (fallback)
	}
}

// detectNotesFromAYVolume detects notes from AY volume register writes
// For direct volume mode: each volume increase triggers a note
// detectNotesFromAYVolume detects notes from AY volume register writes
func detectNotesFromAYVolume(commands []spectrum.AYCommand, channel int, maxFrame int, minNoteFrames int, verbose bool) []Note {
	var notes []Note

	volReg := uint8(8 + channel) // R8=A, R9=B, R10=C
	numFrames := maxFrame + 1

	// Track envelope mode status per frame
	envelopeModeActive := make([]bool, numFrames)
	for _, cmd := range commands {
		if cmd.Register == volReg {
			frame := int(cmd.Frame)
			if frame < len(envelopeModeActive) {
				envelopeModeActive[frame] = (cmd.Value & 0x10) != 0
			}
		}
	}

	// Forward fill envelope mode status
	inEnvMode := false
	for i := range envelopeModeActive {
		if envelopeModeActive[i] {
			inEnvMode = true
		}
		envelopeModeActive[i] = inEnvMode
	}

	// Track if noise is audible per frame
	// Mixer bits 0-2: Tone enable for A/B/C (0=on, 1=off)
	// Mixer bits 3-5: Noise enable for A/B/C (0=on, 1=off)
	noiseBit := uint8(3 + channel) // Bit 3=A, 4=B, 5=C

	mixerWrites := make(map[int]byte)         // frame -> mixer value
	envelopeShapeWrites := make(map[int]byte) // frame -> envelope shape register (R13)

	for _, cmd := range commands {
		if cmd.Register == 7 { // Mixer register
			frame := int(cmd.Frame)
			if frame < numFrames {
				mixerWrites[frame] = cmd.Value
			}
		} else if cmd.Register == 13 { // Envelope shape register
			frame := int(cmd.Frame)
			if frame < numFrames {
				envelopeShapeWrites[frame] = cmd.Value
			}
		}
	}

	// Forward fill mixer and envelope shape, then determine waveform
	noiseEnabled := make([]bool, numFrames)
	envelopeShape := make([]byte, numFrames)
	currentMixer := byte(0xFF) // All disabled by default
	currentShape := byte(0x00)

	for i := range noiseEnabled {
		if val, hasWrite := mixerWrites[i]; hasWrite {
			currentMixer = val
		}
		if val, hasWrite := envelopeShapeWrites[i]; hasWrite {
			currentShape = val
		}
		envelopeShape[i] = currentShape

		// Use noise only when tone is OFF and noise is ON (noise-only mode)
		// AY can mix tone+noise, but SID cannot, so we only use SID noise for noise-only
		toneOn := (currentMixer & (1 << (channel))) == 0
		noiseOn := (currentMixer & (1 << noiseBit)) == 0

		noiseEnabled[i] = !toneOn && noiseOn
	}

	// Build array of volume level per frame (extract from volume register writes)
	volumePerFrame := make([]float64, numFrames)

	// First pass: record volume changes
	volumeChanges := make(map[int]uint8)
	for _, cmd := range commands {
		if cmd.Register == volReg {
			frame := int(cmd.Frame)
			if frame < len(volumePerFrame) {
				// Extract volume level (0-15) from bits 0-3
				volumeChanges[frame] = cmd.Value & 0x0F
			}
		}
	}

	// Second pass: forward fill - preserve raw AY volume register values (0-15)
	currentVolLevel := uint8(0)
	for i := 0; i < len(volumePerFrame); i++ {
		if newVol, hasChange := volumeChanges[i]; hasChange {
			currentVolLevel = newVol
		}
		// Store raw AY volume value (0-15), no conversion
		volumePerFrame[i] = float64(currentVolLevel)
	}

	// Unified note detection: detect from volume register writes
	// Then determine mode (envelope vs direct) per-note based on bit 4
	if verbose {
		fmt.Printf("  Using unified detection (volume register based)\n")
	}

	currentVol := uint8(0)
	noteStart := -1
	noteEnvMode := false
	volumeHistory := []float64{} // Track volume levels during current note

	for _, cmd := range commands {
		if cmd.Register != volReg {
			continue
		}

		frame := int(cmd.Frame)
		if frame >= numFrames {
			continue
		}

		newVol := cmd.Value & 0x1F // All 5 bits (bit 4 = envelope flag, bits 0-3 = volume)
		volLevel := newVol & 0x0F
		isEnvMode := (newVol & 0x10) != 0

		// Note starts when:
		// - Volume goes from 0 to non-zero (direct mode)
		// - Changes to envelope mode (bit 4 set, envelope provides volume)
		// - Volume rises after falling (detected using shared envelope validation)
		wasEnvMode := (currentVol & 0x10) != 0
		prevVolLevel := currentVol & 0x0F

		noteStarts := false
		if isEnvMode && !wasEnvMode {
			// Entering envelope mode
			noteStarts = true
		} else if isEnvMode && wasEnvMode {
			// Already in envelope mode: check for R13 write (envelope restart)
			// R13 write causes the AY envelope to restart from 0, making volume rise again
			if _, hasR13Write := envelopeShapeWrites[frame]; hasR13Write {
				noteStarts = true
			}
		} else if !isEnvMode && prevVolLevel == 0 && volLevel > 0 {
			// Direct mode: volume goes from 0 to non-zero
			noteStarts = true
		} else if !isEnvMode && volLevel > prevVolLevel && len(volumeHistory) > 2 {
			// Direct mode: volume increases - check if it's rising after falling
			// Add current rise to history and check if the pattern shows a fall then rise
			testHistory := append(volumeHistory, float64(volLevel))
			// Check if this forms an invalid shape (rising after falling = two cycles)
			if !isValidEnvelopeShape(testHistory, 0.75) {
				// Pattern shows rise-fall-rise, so start new note at this rise
				noteStarts = true
			}
		}

		if noteStarts {
			volumeHistory = []float64{float64(volLevel)}
		} else if noteStart >= 0 {
			volumeHistory = append(volumeHistory, float64(volLevel))
		}

		if noteStarts {
			// End previous note if exists
			if noteStart >= 0 {
				noteLength := frame - noteStart
				if noteLength >= minNoteFrames {
					curve := make([]float64, noteLength)
					copy(curve, volumePerFrame[noteStart:frame])

					var waveform byte
					var usesNoise bool
					var adsr ADSRParams

					if noteEnvMode {
						// Envelope mode: use R13 shape
						// Check for R13 write at this frame first (to handle same-frame writes)
						waveform = byte(0x04)
						if shapeVal, hasWrite := envelopeShapeWrites[noteStart]; hasWrite {
							waveform = ayEnvelopeToSIDWaveform(shapeVal)
						} else if noteStart < len(envelopeShape) {
							waveform = ayEnvelopeToSIDWaveform(envelopeShape[noteStart])
						}
						usesNoise = (waveform == 0x08)
						adsr = ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}
					} else {
						// Direct mode: check noise
						usesNoise = false
						for f := noteStart; f < frame; f++ {
							if noiseEnabled[f] {
								usesNoise = true
								break
							}
						}
						waveform = byte(0x04)
						if usesNoise {
							waveform = 0x08
						}
						adsr = ADSRParams{} // Will be fitted later
					}

					notes = append(notes, Note{
						StartFrame:   noteStart,
						EndFrame:     frame - 1,
						VolumeCurve:     curve,
						GateOffFrame: len(curve), // Default: gate on full duration
						UseNoise:     usesNoise,
						Waveform:     waveform,
						ADSR:         adsr,
						EnvelopeMode: noteEnvMode,
					})
				}
			}
			// Start new note
			noteStart = frame
			noteEnvMode = isEnvMode
		}

		// Note ends when:
		// - Direct mode: volume goes to 0
		// - Envelope mode: leaving envelope mode
		noteEnds := false
		if noteStart >= 0 {
			if noteEnvMode && !isEnvMode {
				// Was in envelope mode, now leaving
				noteEnds = true
			} else if !noteEnvMode && volLevel == 0 && prevVolLevel > 0 {
				// Direct mode: volume goes to 0
				noteEnds = true
			}
		}

		if noteEnds {
			if noteStart >= 0 {
				noteLength := frame - noteStart
				if noteLength >= minNoteFrames {
					curve := make([]float64, noteLength)
					copy(curve, volumePerFrame[noteStart:frame])

					var waveform byte
					var usesNoise bool
					var adsr ADSRParams

					if noteEnvMode {
						waveform = byte(0x04)
						if shapeVal, hasWrite := envelopeShapeWrites[noteStart]; hasWrite {
							waveform = ayEnvelopeToSIDWaveform(shapeVal)
						} else if noteStart < len(envelopeShape) {
							waveform = ayEnvelopeToSIDWaveform(envelopeShape[noteStart])
						}
						usesNoise = (waveform == 0x08)
						adsr = ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}
					} else {
						usesNoise = false
						for f := noteStart; f < frame; f++ {
							if noiseEnabled[f] {
								usesNoise = true
								break
							}
						}
						waveform = byte(0x04)
						if usesNoise {
							waveform = 0x08
						}
						adsr = ADSRParams{}
					}

					notes = append(notes, Note{
						StartFrame:   noteStart,
						EndFrame:     frame - 1,
						VolumeCurve:     curve,
						GateOffFrame: len(curve), // Default: gate on full duration
						UseNoise:     usesNoise,
						Waveform:     waveform,
						ADSR:         adsr,
						EnvelopeMode: noteEnvMode,
					})
				}
				noteStart = -1
			}
		}

		currentVol = newVol
	}

	// Close final note if exists
	if noteStart >= 0 && noteStart < numFrames {
		noteLength := numFrames - noteStart
		if noteLength >= minNoteFrames {
			curve := make([]float64, noteLength)
			copy(curve, volumePerFrame[noteStart:])

			var waveform byte
			var usesNoise bool
			var adsr ADSRParams

			if noteEnvMode {
				waveform = byte(0x04)
				if shapeVal, hasWrite := envelopeShapeWrites[noteStart]; hasWrite {
					waveform = ayEnvelopeToSIDWaveform(shapeVal)
				} else if noteStart < len(envelopeShape) {
					waveform = ayEnvelopeToSIDWaveform(envelopeShape[noteStart])
				}
				usesNoise = (waveform == 0x08)
				adsr = ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}
			} else {
				usesNoise = false
				for f := noteStart; f < numFrames; f++ {
					if noiseEnabled[f] {
						usesNoise = true
						break
					}
				}
				waveform = byte(0x04)
				if usesNoise {
					waveform = 0x08
				}
				adsr = ADSRParams{}
			}

			notes = append(notes, Note{
				StartFrame:   noteStart,
				EndFrame:     numFrames - 1,
				VolumeCurve:     curve,
				GateOffFrame: len(curve), // Default: gate on full duration
				UseNoise:     usesNoise,
				Waveform:     waveform,
				ADSR:         adsr,
			})
		}
	}

	if verbose {
		waveformCounts := make(map[byte]int)
		for _, note := range notes {
			waveformCounts[note.Waveform]++
		}
		waveformNames := map[byte]string{
			0x01: "triangle",
			0x02: "sawtooth",
			0x04: "pulse",
			0x08: "noise",
		}
		for wf, count := range waveformCounts {
			name := waveformNames[wf]
			if name == "" {
				name = fmt.Sprintf("unknown(0x%02X)", wf)
			}
			fmt.Printf("  %d/%d notes use %s waveform\n", count, len(notes), name)
		}
	}

	return notes
}


func generateComparisonWAV(commands []spectrum.AYCommand, notes [3][]Note, outputPath string) ([3][]int16, [3][]int16) {
	sampleRate := 44100
	durationFrames := int(commands[len(commands)-1].Frame)
	// Use correct PAL frame rate for SID sample count
	durationSamples := int(float64(durationFrames) * float64(sampleRate) / palFrameRate)

	fmt.Printf("Generating AY audio (3 channels)...\n")
	ayStart := time.Now()
	var wg sync.WaitGroup
	var ayAudio [3][]int16
	for ch := 0; ch < 3; ch++ {
		wg.Add(1)
		go func(channel int) {
			defer wg.Done()
			ayAudio[channel] = generateAYAudioChannel(commands, channel, 0, durationFrames)
		}(ch)
	}
	wg.Wait()
	ayDuration := time.Since(ayStart)
	fmt.Printf("  AY generation took %.1fs\n", ayDuration.Seconds())
	fmt.Printf("  AY sample counts: [%d, %d, %d] (target: %d)\n",
		len(ayAudio[0]), len(ayAudio[1]), len(ayAudio[2]), durationSamples)

	fmt.Printf("Generating SID audio (3 channels)...\n")
	sidStart := time.Now()
	var sidAudio [3][]int16
	var adsrBugDebug [3][]int16 // ADSR delay bug debug for all 3 channels
	for ch := 0; ch < 3; ch++ {
		wg.Add(1)
		go func(channel int) {
			defer wg.Done()
			audio, bugDebug := generateSIDPerNote(commands, channel, notes[channel], durationSamples)
			sidAudio[channel] = audio
			adsrBugDebug[channel] = bugDebug
		}(ch)
	}
	wg.Wait()
	sidDuration := time.Since(sidStart)
	fmt.Printf("  SID generation took %.1fs\n", sidDuration.Seconds())
	fmt.Printf("  SID sample counts: [%d, %d, %d]\n",
		len(sidAudio[0]), len(sidAudio[1]), len(sidAudio[2]))

	// Mix all ADSR bug debug channels into one with distinct values per channel
	mixedBugDebug := make([]int16, durationSamples)
	for i := 0; i < durationSamples; i++ {
		// Default: -1 (no bug active)
		mixedBugDebug[i] = -32767

		// Channel-specific values when bug is active
		if adsrBugDebug[0][i] != 0 {
			mixedBugDebug[i] = 32767 // Channel A: +1
		} else if adsrBugDebug[1][i] != 0 {
			mixedBugDebug[i] = 10922 // Channel B: +0.33
		} else if adsrBugDebug[2][i] != 0 {
			mixedBugDebug[i] = -10922 // Channel C: -0.33
		}
	}

	fmt.Printf("Writing WAV file...\n")
	writeStart := time.Now()
	writeSevenChannelWAV(outputPath, ayAudio[:], sidAudio[:], mixedBugDebug, sampleRate)
	writeDuration := time.Since(writeStart)
	fmt.Printf("  Write took %.1fs\n", writeDuration.Seconds())

	return ayAudio, sidAudio
}

// getAYNoisePeriod finds the noise period (R6) value at a given frame
func getAYNoisePeriod(commands []spectrum.AYCommand, frame int) byte {
	noisePeriod := byte(0)
	for _, cmd := range commands {
		if cmd.Register == 6 && int(cmd.Frame) <= frame {
			noisePeriod = cmd.Value & 0x1F
		}
		if int(cmd.Frame) > frame {
			break
		}
	}
	if noisePeriod == 0 {
		noisePeriod = 1 // Default to 1 if not set
	}
	return noisePeriod
}

// getAYEnvelopePeriod finds the envelope period (R11-R12) value at a given frame
func getAYEnvelopePeriod(commands []spectrum.AYCommand, frame int) uint16 {
	envLo := byte(0)
	envHi := byte(0)
	for _, cmd := range commands {
		if int(cmd.Frame) > frame {
			break
		}
		if cmd.Register == 11 && int(cmd.Frame) <= frame {
			envLo = cmd.Value
		}
		if cmd.Register == 12 && int(cmd.Frame) <= frame {
			envHi = cmd.Value
		}
	}
	period := uint16(envLo) | (uint16(envHi) << 8)
	if period == 0 {
		period = 1 // Default to 1 if not set
	}
	return period
}

// getAYEnvelopeShape finds the envelope shape (R13) value at a given frame
func getAYEnvelopeShape(commands []spectrum.AYCommand, frame int) byte {
	shape := byte(0)
	for _, cmd := range commands {
		if int(cmd.Frame) > frame {
			break
		}
		if cmd.Register == 13 && int(cmd.Frame) <= frame {
			shape = cmd.Value
		}
	}
	return shape
}

// computeEnvelopeFrequency converts AY envelope period to SID frequency
// AY envelope update rate: AY_clock / (period * 512) where period = R11 | (R12 << 8)
func computeEnvelopeFrequency(envPeriod uint16) uint16 {
	ayClock := 1773400.0
	sidClock := 985248.0
	// AY envelope rate: ayClock / (period * 512)
	// Previous /256 was 2x too high
	envFreqHz := ayClock / (float64(envPeriod) * 512.0)
	// SID frequency register value: freq_hz * 16777216 / sid_clock
	// Round to nearest integer for optimal frequency match
	sidFreq := uint16(envFreqHz*16777216.0/sidClock + 0.5)
	return sidFreq
}

// computeNoiseFrequency converts AY noise period to SID frequency
// AY noise update rate: AY_clock / (period * 16 * 2) where period = R6 & 0x1F
// SID oscillator freq: (SID_freq / 16777216) * SID_clock
// Match them: (SID_freq / 16777216) * 985248 = 886700 / (period * 32)
func computeNoiseFrequency(noisePeriod byte) uint16 {
	if noisePeriod == 0 {
		noisePeriod = 1
	}
	ayClock := 1773400.0 // AY clock (already CPU/2, same as tone conversion)
	sidClock := 985248.0
	// AY noise rate: ayClock / (period << 4) = ayClock / (period * 16)
	noiseRateHz := ayClock / (float64(noisePeriod) * 16.0)
	// SID noise LFSR is clocked by bit 19 rising edge
	// shift_rate = (freq_reg * sid_clock) / 2^20
	// Solving for freq_reg: freq_reg = shift_rate * 2^20 / sid_clock
	// Round to nearest integer for optimal frequency match
	sidFreq := uint16(noiseRateHz*1048576.0/sidClock + 0.5) // 1048576 = 2^20
	return sidFreq
}

// analyzeModulationRatios analyzes envelope-modulated notes and outputs ratio diagnostics
func analyzeModulationRatios(commands []spectrum.AYCommand, notes [3][]Note) {
	fmt.Printf("\n=== Envelope Modulation Ratio Analysis ===\n")

	// Build frequency tables for each channel
	numFrames := int(commands[len(commands)-1].Frame) + 100 // Add buffer
	freqPerFrame := [3][]uint16{}
	for ch := 0; ch < 3; ch++ {
		freqPerFrame[ch] = make([]uint16, numFrames)
		currentFreq := uint16(0)
		toneLoReg := uint8(ch * 2)
		toneHiReg := uint8(ch*2 + 1)

		for _, cmd := range commands {
			frame := int(cmd.Frame)
			if frame >= len(freqPerFrame[ch]) {
				continue
			}
			switch cmd.Register {
			case toneLoReg:
				currentFreq = (currentFreq & 0xFF00) | uint16(cmd.Value)
				freqPerFrame[ch][frame] = currentFreq
			case toneHiReg:
				currentFreq = (currentFreq & 0x00FF) | (uint16(cmd.Value&0x0F) << 8)
				freqPerFrame[ch][frame] = currentFreq
			}
		}

		// Forward-fill
		lastFreq := uint16(0)
		for i := range freqPerFrame[ch] {
			if freqPerFrame[ch][i] != 0 {
				lastFreq = freqPerFrame[ch][i]
			} else {
				freqPerFrame[ch][i] = lastFreq
			}
		}
	}

	ayClock := 1773400.0
	sidClock := 985248.0

	// Analyze each channel
	for ch := 0; ch < 3; ch++ {
		envModNotes := []Note{}
		for _, note := range notes[ch] {
			if note.EnvelopeMode {
				envModNotes = append(envModNotes, note)
			}
		}

		if len(envModNotes) == 0 {
			continue
		}

		fmt.Printf("\nChannel %c: %d envelope-modulated notes\n", 'A'+ch, len(envModNotes))

		// Sample first few envelope-modulated notes
		maxSamples := 5
		if len(envModNotes) < maxSamples {
			maxSamples = len(envModNotes)
		}

		for i := 0; i < maxSamples; i++ {
			note := envModNotes[i]
			frame := note.StartFrame

			// Get AY parameters
			ayTonePeriod := float64(freqPerFrame[ch][frame])
			if ayTonePeriod == 0 {
				continue
			}
			ayToneFreq := ayClock / (16.0 * ayTonePeriod)

			envPeriod := getAYEnvelopePeriod(commands, frame)
			ayEnvFreq := ayClock / (512.0 * float64(envPeriod))

			ayRatio := ayToneFreq / ayEnvFreq

			// Compute SID envelope frequency
			sidFreq := computeEnvelopeFrequency(envPeriod)
			if note.Waveform == 0x02 { // Sawtooth
				sidFreq *= 2
			}
			sidEnvFreq := (float64(sidFreq) * sidClock) / 16777216.0
			if note.Waveform == 0x02 {
				sidEnvFreq /= 2.0
			}

			// Calculate ideal toggle period and round to nearest integer
			idealToggleFreq := sidEnvFreq * ayRatio
			idealTogglePeriod := sidClock / (2.0 * idealToggleFreq)
			togglePeriod := int(idealTogglePeriod + 0.5)
			if togglePeriod < 1 {
				togglePeriod = 1
			}

			// Calculate actual ratio with rounded period
			actualToggleFreq := sidClock / (2.0 * float64(togglePeriod))
			actualRatio := actualToggleFreq / sidEnvFreq

			// Output diagnostics
			fmt.Printf("  Note %d (frame %d):\n", i+1, frame)
			fmt.Printf("    AY tone freq:      %.2f Hz\n", ayToneFreq)
			fmt.Printf("    AY envelope freq:  %.2f Hz\n", ayEnvFreq)
			fmt.Printf("    AY ratio:          %.4f\n", ayRatio)
			fmt.Printf("    SID envelope freq: %.2f Hz\n", sidEnvFreq)
			fmt.Printf("    Toggle period:     %d cycles (ideal: %.2f)\n", togglePeriod, idealTogglePeriod)
			fmt.Printf("    Actual ratio:      %.4f (error: %.2f%%)\n",
				actualRatio, 100.0*math.Abs(actualRatio-ayRatio)/ayRatio)
		}

		if len(envModNotes) > maxSamples {
			fmt.Printf("  ... (%d more envelope-modulated notes)\n", len(envModNotes)-maxSamples)
		}
	}
}

// fitReleaseValue finds the best R value by simulating SID release and comparing to target
// Includes ADSR delay bug simulation to avoid bug-prone R values
// currentLevel: current SID envelope level (0-255)
// targetCurve: remaining AY volume values (raw 0-15 register values)
// lookahead: number of frames to simulate ahead for fitting
// rateCounter: current 15-bit rate counter position (0-0x7fff)
func fitReleaseValue(currentLevel uint8, targetCurve []float64, lookahead int, rateCounter uint16) uint8 {
	if len(targetCurve) == 0 {
		return 15 // No data: slowest decay
	}

	// AY volume table (logarithmic)
	ayVolumeTable := [16]float64{
		0, 327, 473, 690, 1006, 1492, 2113, 3518,
		4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
	}

	// Check if there's a large drop coming in the next frame
	if len(targetCurve) >= 2 {
		immediateDrop := targetCurve[0] - targetCurve[1]
		if immediateDrop >= 3.0 {
			// Large drop coming (>= 3 levels) - use very fast R (0-6)
			bestR := uint8(6)
			bestError := 1e9

			for r := uint8(0); r <= 6; r++ {
				ratePeriod := uint32(adsrRates[r])

				// Skip R values that would trigger ADSR bug
				if ratePeriod < uint32(rateCounter) {
					continue
				}

				simulated := simulateReleaseWithBug(currentLevel, r, lookahead, rateCounter)

				// Calculate envelope mismatch error
				envelopeError := 0.0
				for i := 0; i < lookahead && i < len(targetCurve); i++ {
					tVol := int(targetCurve[i] + 0.5)
					if tVol < 0 {
						tVol = 0
					}
					if tVol > 15 {
						tVol = 15
					}
					tNorm := ayVolumeTable[tVol] / 32767.0
					diff := simulated[i] - tNorm
					envelopeError += diff * diff
				}
				envelopeError = envelopeError / float64(lookahead)

				if envelopeError < bestError {
					bestError = envelopeError
					bestR = r
				}
			}
			return bestR
		}
	}

	// Limit lookahead to available data
	if lookahead > len(targetCurve) {
		lookahead = len(targetCurve)
	}
	if lookahead < 1 {
		lookahead = 1
	}

	bestR := uint8(11)
	bestError := 1e9

	// Try R values 0-11 (R>=12 causes ADSR delay bug with frame-rate updates)
	for r := uint8(0); r <= 11; r++ {
		ratePeriod := uint32(adsrRates[r])

		// Skip R values that would trigger ADSR bug
		if ratePeriod < uint32(rateCounter) {
			continue
		}

		// Simulate with ADSR delay bug
		simulated := simulateReleaseWithBug(currentLevel, r, lookahead, rateCounter)

		// Compute envelope mismatch error
		envelopeError := 0.0
		for i := 0; i < lookahead && i < len(targetCurve); i++ {
			// Convert target AY volume to linear amplitude (0-1)
			targetVol := int(targetCurve[i] + 0.5)
			if targetVol < 0 {
				targetVol = 0
			}
			if targetVol > 15 {
				targetVol = 15
			}
			targetNorm := ayVolumeTable[targetVol] / 32767.0
			diff := simulated[i] - targetNorm
			envelopeError += diff * diff
		}
		envelopeError = envelopeError / float64(lookahead)

		if envelopeError < bestError {
			bestError = envelopeError
			bestR = r
		}
	}

	return bestR
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// simulateReleaseFromLevel simulates SID release from a given starting level
// Returns envelope values (0-1 scale) for the specified number of frames
// simulateAttackFromLevel simulates SID attack phase from a starting level
func simulateAttackFromLevel(startLevel uint8, attack uint8, numFrames int) []float64 {
	envelope := make([]float64, numFrames)
	cyclesPerFrame := sidClockHz / palFrameRate

	envelopeLevel := startLevel
	envelopeCounter := 0.0

	for frame := 0; frame < numFrames; frame++ {
		for cycle := 0.0; cycle < cyclesPerFrame; cycle++ {
			envelopeCounter++

			ratePeriod := float64(adsrRates[attack])

			if envelopeCounter >= ratePeriod {
				envelopeCounter = 0

				// Attack: linear increment
				if envelopeLevel < 255 {
					envelopeLevel++
				}
			}
		}
		envelope[frame] = float64(envelopeLevel) / 255.0
	}

	return envelope
}

func simulateReleaseFromLevel(startLevel uint8, release uint8, numFrames int) []float64 {
	envelope := make([]float64, numFrames)
	cyclesPerFrame := sidClockHz / palFrameRate

	envelopeLevel := startLevel
	envelopeCounter := 0.0
	exponentialCounter := 0
	exponentialPeriod := 1

	// updateExponentialPeriod matches hardware behavior
	updateExponentialPeriod := func() {
		switch envelopeLevel {
		case 255:
			exponentialPeriod = 1
		case 93:
			exponentialPeriod = 2
		case 54:
			exponentialPeriod = 4
		case 26:
			exponentialPeriod = 8
		case 14:
			exponentialPeriod = 16
		case 6:
			exponentialPeriod = 30
		case 0:
			exponentialPeriod = 1
		}
	}

	updateExponentialPeriod()

	for frame := 0; frame < numFrames; frame++ {
		for cycle := 0.0; cycle < cyclesPerFrame; cycle++ {
			envelopeCounter++

			ratePeriod := float64(adsrRates[release])

			if envelopeCounter >= ratePeriod {
				envelopeCounter = 0

				// Release: exponential decay
				exponentialCounter++
				if exponentialCounter >= exponentialPeriod {
					exponentialCounter = 0
					if envelopeLevel > 0 {
						envelopeLevel--
						updateExponentialPeriod()
					}
				}
			}
		}
		envelope[frame] = float64(envelopeLevel) / 255.0
	}

	return envelope
}

// simulateReleaseWithBug simulates SID release including ADSR delay bug effect
// If new rate period < current rate counter, envelope freezes until counter wraps at 0x8000
func simulateReleaseWithBug(startLevel uint8, release uint8, numFrames int, startCounter uint16) []float64 {
	envelope := make([]float64, numFrames)
	cyclesPerFrame := sidClockHz / palFrameRate
	ratePeriod := uint32(adsrRates[release])

	envelopeLevel := startLevel
	rateCounter := startCounter
	exponentialCounter := 0
	exponentialPeriod := 1

	// updateExponentialPeriod matches hardware behavior
	updateExponentialPeriod := func() {
		switch envelopeLevel {
		case 255:
			exponentialPeriod = 1
		case 93:
			exponentialPeriod = 2
		case 54:
			exponentialPeriod = 4
		case 26:
			exponentialPeriod = 8
		case 14:
			exponentialPeriod = 16
		case 6:
			exponentialPeriod = 30
		case 0:
			exponentialPeriod = 1
		}
	}

	updateExponentialPeriod()

	for frame := 0; frame < numFrames; frame++ {
		cyclesToSimulate := uint32(cyclesPerFrame)

		// Check for ADSR delay bug at frame start (when R changes)
		// Bug triggers if new rate_period < current rate_counter
		if ratePeriod < uint32(rateCounter) {
			// Bug active! Counter must wrap at 0x8000 before envelope can step
			// Calculate cycles until wrap
			cyclesUntilWrap := (0x8000 - uint32(rateCounter))
			if cyclesUntilWrap < cyclesToSimulate {
				// Wrap happens this frame - envelope frozen until then
				cyclesToSimulate -= cyclesUntilWrap
				rateCounter = 0 // Wrapped
			} else {
				// Still frozen entire frame
				rateCounter += uint16(cyclesToSimulate)
				envelope[frame] = float64(envelopeLevel) / 255.0
				continue
			}
		}

		// Simulate envelope for remaining cycles
		for cyclesToSimulate > 0 {
			// Calculate cycles until next envelope step
			cyclesUntilStep := ratePeriod - uint32(rateCounter)
			if cyclesUntilStep > cyclesToSimulate {
				// Not enough cycles to step - just advance counter
				rateCounter += uint16(cyclesToSimulate)
				break
			}

			// Step the envelope
			cyclesToSimulate -= cyclesUntilStep
			rateCounter = 0

			// Release: exponential decay
			exponentialCounter++
			if exponentialCounter >= exponentialPeriod {
				exponentialCounter = 0
				if envelopeLevel > 0 {
					envelopeLevel--
					updateExponentialPeriod()
				}
			}
		}

		envelope[frame] = float64(envelopeLevel) / 255.0
	}

	return envelope
}

// smoothAYCurve smooths small decreases (< 3 levels) in the AY curve
// Returns a new curve with smoothed transitions
func smoothAYCurve(volumeCurve []float64) []float64 {
	if len(volumeCurve) < 2 {
		return volumeCurve
	}

	smoothed := make([]float64, len(volumeCurve))
	copy(smoothed, volumeCurve)

	// Find the end of the initial plateau
	// Continue as long as we're at or near the peak
	peakIdx := 0
	peakVal := volumeCurve[0]
	for i := 1; i < len(volumeCurve); i++ {
		if volumeCurve[i] >= peakVal-0.1 {
			// Still at peak level, extend plateau
			if volumeCurve[i] > peakVal {
				peakVal = volumeCurve[i]
			}
			peakIdx = i
		} else {
			// Started decreasing significantly
			break
		}
	}

	// After the plateau, smooth small decreases with multiple passes
	// Multiple passes are needed to turn step+plateau sequences into smooth ramps
	for pass := 0; pass < 5; pass++ {
		for i := peakIdx + 1; i < len(smoothed)-1; i++ {
			prevDecrease := smoothed[i-1] - smoothed[i]
			nextDecrease := smoothed[i] - smoothed[i+1]

			// Only smooth if BOTH transitions are small (< 3 levels)
			// This prevents smoothing across large jumps (>= 3 levels)
			if prevDecrease > 0 && prevDecrease < 3.0 && nextDecrease >= 0 && nextDecrease < 3.0 {
				// Average current with previous and next
				smoothed[i] = (smoothed[i-1] + smoothed[i] + smoothed[i+1]) / 3.0
			}
		}
	}

	return smoothed
}

// enforceMonotonicRDecrease simulates the SID rate counter and avoids R changes that would trigger the ADSR delay bug.
// The ADSR delay bug occurs when: new_rate_period < current_rate_counter
// This causes the envelope to freeze while the counter wraps at 0x8000 (up to 33ms delay).
// By simulating the rate counter frame-by-frame, we can predict exactly when the bug would trigger.
// This allows safe R changes when rate_counter is low, while preventing changes when high.
// Note: Simulation is approximate (doesn't model exponential counter divisions), but uses
// conservative jitter margin (60 cycles) to ensure safety on real hardware.
func enforceMonotonicRDecrease(releasePerFrame []uint8) []uint8 {
	if len(releasePerFrame) == 0 {
		return releasePerFrame
	}

	result := make([]uint8, len(releasePerFrame))
	result[0] = releasePerFrame[0]

	// Simulate rate counter (starts at 0 for new notes)
	rateCounter := uint16(0)
	currentR := releasePerFrame[0]
	cyclesPerFrame := uint32(19656) // 50 Hz = 20ms = 19656 cycles at 985248 Hz
	jitterMargin := uint32(60)      // Safety margin for timing jitter

	for i := 1; i < len(releasePerFrame); i++ {
		desiredR := releasePerFrame[i]
		currentPeriod := adsrRatePeriods[currentR]
		desiredPeriod := adsrRatePeriods[desiredR]

		// Simulate rate counter advancement over one frame
		// Note: This is approximate - doesn't account for exponential counter or envelope steps
		cyclesDelta := cyclesPerFrame + jitterMargin // Add jitter margin for safety
		newCounter := uint32(rateCounter) + cyclesDelta

		// Handle counter wrapping and reset when reaching period
		for newCounter >= currentPeriod {
			newCounter -= currentPeriod
			if newCounter >= 0x8000 {
				// Wrapped at 15-bit boundary
				newCounter = (newCounter - 0x7fff) & 0x7fff
			}
		}
		rateCounter = uint16(newCounter & 0x7fff)

		// Check if changing R would trigger ADSR delay bug
		if desiredPeriod < uint32(rateCounter) {
			// Bug would trigger - keep current R to avoid freeze
			result[i] = currentR
		} else {
			// Safe to change - new period is higher than current counter
			result[i] = desiredR
			currentR = desiredR
			// Note: rate counter continues from current position
		}
	}

	return result
}

// computeReleaseForCurveWithAttack computes optimal R values accounting for attack phase
// attackParam: A parameter (0-15), sustainParam: S parameter (0-15)
// Returns array of R values, one per frame
func computeReleaseForCurveWithAttack(volumeCurve []float64, attackParam uint8, sustainParam uint8) []uint8 {
	if len(volumeCurve) == 0 {
		return nil
	}

	// Smooth the AY curve for small decreases
	smoothedCurve := smoothAYCurve(volumeCurve)
	_ = smoothedCurve // Used in backtracking

	lookahead := 5
	releasePerFrame := make([]uint8, len(volumeCurve))

	// Determine starting level for release (after attack phase completes)
	var currentLevel uint8
	if attackParam > 0 {
		// Simulate 1 frame of attack to find where we actually start
		attackSim := simulateAttackFromLevel(0, attackParam, 1)
		currentLevel = uint8(attackSim[0] * 255.0)
	} else {
		// A=0, D=0: fast attack/decay to sustain level
		currentLevel = uint8((float64(sustainParam) / 15.0) * 255.0)
	}

	releasePerFrame[0] = 11 // First frame is attack phase (R ignored, but set to safe value)

	// Track rate counter (starts at ~4 after R=0 pre-flush from previous note)
	rateCounter := uint16(4)

	// Helper: advance rate counter by one frame
	advanceCounter := func(counter uint16, rPeriod uint32) uint16 {
		cyclesThisFrame := uint32(19656)
		for cyclesThisFrame > 0 {
			cyclesUntilStep := rPeriod - uint32(counter)
			if cyclesUntilStep > cyclesThisFrame {
				newCounter := uint32(counter) + cyclesThisFrame
				if newCounter >= 0x8000 {
					return uint16((newCounter + 1) & 0x7fff)
				}
				return uint16(newCounter)
			}
			cyclesThisFrame -= cyclesUntilStep
			counter = 0
		}
		return counter
	}

	// AY volume table
	ayVolumeTable := [16]float64{
		0, 327, 473, 690, 1006, 1492, 2113, 3518,
		4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
	}

	// Beam search with parallel path expansion
	type BeamPath struct {
		frameIdx      int
		currentLevel  uint8
		rateCounter   uint16
		path          []uint8
		cumulativeErr float64
	}

	beamWidth := 100
	beam := []BeamPath{{
		frameIdx:      1,
		currentLevel:  currentLevel,
		rateCounter:   rateCounter,
		path:          []uint8{},
		cumulativeErr: 0.0,
	}}

	// Process each frame
	for frameIdx := 1; frameIdx < len(volumeCurve); frameIdx++ {
		// Expand all beam paths in parallel
		type expandResult struct {
			children []BeamPath
		}
		resultChan := make(chan expandResult, len(beam))
		var wg sync.WaitGroup

		for _, parent := range beam {
			if parent.frameIdx != frameIdx {
				continue
			}
			wg.Add(1)
			go func(p BeamPath) {
				defer wg.Done()

				targetCurve := volumeCurve[p.frameIdx:]
				var children []BeamPath

				// Generate all valid R candidates
				for r := uint8(0); r <= 15; r++ {
					ratePeriod := uint32(adsrRatePeriods[r])
					if ratePeriod < uint32(p.rateCounter) {
						continue // Would trigger bug
					}

					// Calculate envelope error for this R
					simulated := simulateReleaseWithBug(p.currentLevel, r, lookahead, p.rateCounter)
					envelopeError := 0.0
					for i := 0; i < lookahead && i < len(targetCurve); i++ {
						tVol := int(targetCurve[i] + 0.5)
						if tVol < 0 {
							tVol = 0
						}
						if tVol > 15 {
							tVol = 15
						}
						tNorm := ayVolumeTable[tVol] / 32767.0
						diff := simulated[i] - tNorm
						envelopeError += diff * diff
					}
					envelopeError = envelopeError / float64(lookahead)

					// Simulate one frame
					simulated1 := simulateReleaseWithBug(p.currentLevel, r, 1, p.rateCounter)
					newLevel := uint8(simulated1[0] * 255.0)
					newCounter := advanceCounter(p.rateCounter, ratePeriod)

					// Create child path
					newPath := make([]uint8, len(p.path)+1)
					copy(newPath, p.path)
					newPath[len(p.path)] = r

					// Add counter penalty to encourage low counters
					counterPenalty := 0.01 * float64(newCounter) / 32768.0

					children = append(children, BeamPath{
						frameIdx:      p.frameIdx + 1,
						currentLevel:  newLevel,
						rateCounter:   newCounter,
						path:          newPath,
						cumulativeErr: p.cumulativeErr + envelopeError + counterPenalty,
					})
				}

				resultChan <- expandResult{children: children}
			}(parent)
		}

		// Wait for all goroutines
		go func() {
			wg.Wait()
			close(resultChan)
		}()

		// Collect all children
		var allChildren []BeamPath
		for result := range resultChan {
			allChildren = append(allChildren, result.children...)
		}

		if len(allChildren) == 0 {
			fmt.Printf("WARNING: Beam search found no valid paths at frame %d\n", frameIdx)
			break
		}

		// Sort by error with counter as tiebreaker
		sort.Slice(allChildren, func(i, j int) bool {
			errDiff := allChildren[i].cumulativeErr - allChildren[j].cumulativeErr
			if math.Abs(errDiff) < 0.0005 { // Very tight error tolerance
				return allChildren[i].rateCounter < allChildren[j].rateCounter
			}
			return allChildren[i].cumulativeErr < allChildren[j].cumulativeErr
		})

		// Keep top K
		if len(allChildren) > beamWidth {
			beam = allChildren[:beamWidth]
		} else {
			beam = allChildren
		}
	}

	// Select best path (lowest cumulative error)
	if len(beam) > 0 {
		best := beam[0]
		copy(releasePerFrame[1:], best.path)
	} else {
		fmt.Printf("WARNING: Beam search failed for envelope type\n")
	}

	return releasePerFrame
}

// convergeRValues tries to smooth out jaggy R chains for smoother envelopes
// Extends sequences of the same R value while preserving fast R for sudden drops
func convergeRValues(releasePerFrame []uint8, volumeCurve, smoothedCurve []float64, attackParam, sustainParam uint8) []uint8 {
	if len(releasePerFrame) < 3 {
		return releasePerFrame
	}

	// AY volume table
	ayVolumeTable := [16]float64{
		0, 327, 473, 690, 1006, 1492, 2113, 3518,
		4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
	}

	// Determine starting level for release (after attack phase completes)
	var startLevel uint8
	if attackParam > 0 {
		attackSim := simulateAttackFromLevel(0, attackParam, 1)
		startLevel = uint8(attackSim[0] * 255.0)
	} else {
		startLevel = uint8((float64(sustainParam) / 15.0) * 255.0)
	}

	// Find the peak value to distinguish initial plateau from mid-curve plateaus
	peakVal := volumeCurve[0]
	for i := 1; i < len(volumeCurve); i++ {
		if volumeCurve[i] > peakVal {
			peakVal = volumeCurve[i]
		}
	}

	// Identify frames with large drops (should keep their fast R values)
	isLargeDropFrame := make([]bool, len(volumeCurve))
	for i := 1; i < len(volumeCurve); i++ {
		drop := volumeCurve[i-1] - volumeCurve[i]
		if drop >= 3.0 {
			// Mark this frame and the next few as large drop region (>= 3 levels)
			isLargeDropFrame[i] = true
			if i+1 < len(volumeCurve) {
				isLargeDropFrame[i+1] = true
			}
		}
	}

	converged := make([]uint8, len(releasePerFrame))
	copy(converged, releasePerFrame)

	// Start after the initial plateau region (skip frames at peak with slow decay)
	startIdx := 1
	for startIdx < len(converged) && converged[startIdx] == 11 && volumeCurve[startIdx] >= peakVal-0.1 {
		startIdx++
	}

	if startIdx >= len(converged)-1 {
		return converged
	}

	// Multiple passes to smooth out jaggy R chains
	for pass := 0; pass < 3; pass++ {
		changed := false

		// Forward pass: try to extend each R value forward
		for segmentStart := startIdx; segmentStart < len(converged); segmentStart++ {
			// Skip if at initial peak plateau or in large drop region
			if (converged[segmentStart] == 11 && volumeCurve[segmentStart] >= peakVal-0.1) || isLargeDropFrame[segmentStart] {
				continue
			}

			// Try to extend the current R value as far forward as possible
			currentR := converged[segmentStart]

			// Simulate from the start to get the level at segmentStart
			currentLevel := startLevel
			for i := 1; i < segmentStart; i++ {
				if converged[i] == 11 && volumeCurve[i] >= peakVal-0.1 {
					// Plateau - no change
					continue
				}
				sim := simulateReleaseFromLevel(currentLevel, converged[i], 1)
				currentLevel = uint8(sim[0] * 255.0)
			}

			// Try to extend currentR forward
			testLevel := currentLevel
			maxExtend := segmentStart
			for i := segmentStart; i < len(converged); i++ {
				// Stop at large drops or initial plateau
				if isLargeDropFrame[i] || (converged[i] == 11 && volumeCurve[i] >= peakVal-0.1) {
					break
				}

				// Simulate one frame with currentR
				sim := simulateReleaseFromLevel(testLevel, currentR, 1)
				testLevel = uint8(sim[0] * 255.0)

				// Check error against target
				targetVol := int(smoothedCurve[i] + 0.5)
				if targetVol < 0 {
					targetVol = 0
				}
				if targetVol > 15 {
					targetVol = 15
				}
				targetAmp := ayVolumeTable[targetVol] / 32767.0
				simAmp := float64(testLevel) / 255.0
				error := math.Abs(simAmp - targetAmp)

				if error > 0.05 {
					// Error too large - stop here
					break
				}

				maxExtend = i
			}

			// Apply currentR to the extended range if it's an improvement
			if maxExtend > segmentStart {
				for i := segmentStart; i <= maxExtend; i++ {
					if converged[i] != currentR {
						converged[i] = currentR
						changed = true
					}
				}
				segmentStart = maxExtend // Skip ahead
			}
		}

		// Backward pass: try to extend each R value backward
		for segmentEnd := len(converged) - 1; segmentEnd > startIdx; segmentEnd-- {
			// Skip if at initial peak plateau or in large drop region
			if (converged[segmentEnd] == 11 && volumeCurve[segmentEnd] >= peakVal-0.1) || isLargeDropFrame[segmentEnd] {
				continue
			}

			// Try to extend the current R value backward
			currentR := converged[segmentEnd]

			// Try to extend currentR backward
			minExtend := segmentEnd
			for i := segmentEnd; i >= startIdx; i-- {
				// Stop at large drops or initial plateau
				if isLargeDropFrame[i] || (converged[i] == 11 && volumeCurve[i] >= peakVal-0.1) {
					break
				}

				// Simulate from start to check if using currentR at position i works
				testConverged := make([]uint8, len(converged))
				copy(testConverged, converged)
				testConverged[i] = currentR

				maxError := 0.0
				testLevel := startLevel
				for j := 1; j <= segmentEnd; j++ {
					if testConverged[j] == 11 && volumeCurve[j] >= peakVal-0.1 {
						// Plateau - no change
						continue
					}
					sim := simulateReleaseFromLevel(testLevel, testConverged[j], 1)
					testLevel = uint8(sim[0] * 255.0)

					// Check error
					targetVol := int(smoothedCurve[j] + 0.5)
					if targetVol < 0 {
						targetVol = 0
					}
					if targetVol > 15 {
						targetVol = 15
					}
					targetAmp := ayVolumeTable[targetVol] / 32767.0
					simAmp := float64(testLevel) / 255.0
					error := math.Abs(simAmp - targetAmp)
					if error > maxError {
						maxError = error
					}
				}

				if maxError > 0.05 {
					// Error too large
					break
				}

				minExtend = i
			}

			// Apply currentR to the extended range if it's an improvement
			if minExtend < segmentEnd {
				for i := minExtend; i <= segmentEnd; i++ {
					if converged[i] != currentR {
						converged[i] = currentR
						changed = true
					}
				}
				segmentEnd = minExtend // Skip backward
			}
		}

		if !changed {
			break
		}
	}

	// Save original values to detect true humps (not artifacts from previous flattening)
	original := make([]uint8, len(releasePerFrame))
	copy(original, releasePerFrame)

	// Final pass: avoid temporary R increases (humps) but allow permanent increases
	// Only prevent increases that later decrease - these create jaggy curves
	// Allow increases that persist to the end - these are needed for tails
	for pass := 0; pass < 3; pass++ {
		changed := false
		for i := startIdx + 1; i < len(converged); i++ {
			// Skip large drop regions and initial plateau
			if isLargeDropFrame[i] || (converged[i] == 11 && volumeCurve[i] >= peakVal-0.1) {
				continue
			}

			// Check if R increased from previous frame
			if converged[i] > converged[i-1] {
				// Find the extent of this plateau (how many frames stay at this R or higher)
				plateauEnd := i
				for j := i + 1; j < len(converged); j++ {
					if isLargeDropFrame[j] || (converged[j] == 11 && volumeCurve[j] >= peakVal-0.1) {
						break
					}
					if converged[j] >= converged[i] {
						plateauEnd = j
					} else {
						break
					}
				}

				// Look beyond the plateau: does R go back down in the ORIGINAL data?
				// This prevents flattening increases that are actually needed for tail sustain
				willDecrease := false
				for j := plateauEnd + 1; j < len(original); j++ {
					if isLargeDropFrame[j] || (original[j] == 11 && volumeCurve[j] >= peakVal-0.1) {
						break
					}
					if original[j] < converged[i] {
						willDecrease = true
						break
					}
				}

				// Only try to prevent the increase if it will decrease later (hump)
				if willDecrease {
					// Try to use the previous (lower) R value for the entire plateau
					testConverged := make([]uint8, len(converged))
					copy(testConverged, converged)
					for j := i; j <= plateauEnd; j++ {
						testConverged[j] = converged[i-1]
					}

					// Simulate entire envelope with this change
					testLevel := startLevel
					maxError := 0.0

					for j := 1; j < len(testConverged); j++ {
						if testConverged[j] == 11 && volumeCurve[j] >= peakVal-0.1 {
							continue
						}
						sim := simulateReleaseFromLevel(testLevel, testConverged[j], 1)
						testLevel = uint8(sim[0] * 255.0)

						targetVol := int(smoothedCurve[j] + 0.5)
						if targetVol < 0 {
							targetVol = 0
						}
						if targetVol > 15 {
							targetVol = 15
						}
						targetAmp := ayVolumeTable[targetVol] / 32767.0
						simAmp := float64(testLevel) / 255.0
						error := math.Abs(simAmp - targetAmp)
						if error > maxError {
							maxError = error
						}
					}

					// If error is acceptable, prevent the increase for the entire plateau
					// Use moderately relaxed threshold (8% vs 5%) to smooth obvious humps while preserving tail sustain
					if maxError <= 0.08 {
						for j := i; j <= plateauEnd; j++ {
							converged[j] = converged[i-1]
						}
						changed = true
					}
				}
			}
		}

		if !changed {
			break
		}
	}

	return converged
}

// computeReleaseForCurve computes optimal R values for a given volume curve
// Returns array of R values, one per frame
func computeReleaseForCurve(volumeCurve []float64) []uint8 {
	if len(volumeCurve) == 0 {
		return nil
	}

	// AY volume table (logarithmic)
	ayVolumeTable := [16]float64{
		0, 327, 473, 690, 1006, 1492, 2113, 3518,
		4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
	}

	lookahead := 5
	releasePerFrame := make([]uint8, len(volumeCurve))

	// Start at the first frame's level (sustain level after gate OFF)
	// Convert raw AY volume to linear amplitude, then to SID envelope level (0-255)
	firstVolInt := int(volumeCurve[0] + 0.5)
	if firstVolInt < 0 {
		firstVolInt = 0
	}
	if firstVolInt > 15 {
		firstVolInt = 15
	}
	firstAmp := ayVolumeTable[firstVolInt] / 32767.0 // Linear amplitude (0-1)
	currentLevel := uint8(firstAmp * 255.0)            // SID envelope level (0-255)
	releasePerFrame[0] = 11

	// Track rate counter (starts at ~4 after R=0 pre-flush from previous note)
	rateCounter := uint16(4)

	// Helper: advance rate counter by one frame
	advanceCounter := func(counter uint16, rPeriod uint32) uint16 {
		cyclesThisFrame := uint32(19656)
		for cyclesThisFrame > 0 {
			cyclesUntilStep := rPeriod - uint32(counter)
			if cyclesUntilStep > cyclesThisFrame {
				newCounter := uint32(counter) + cyclesThisFrame
				if newCounter >= 0x8000 {
					return uint16((newCounter + 1) & 0x7fff)
				}
				return uint16(newCounter)
			}
			cyclesThisFrame -= cyclesUntilStep
			counter = 0
		}
		return counter
	}

	// Beam search with parallel path expansion
	type BeamPath struct {
		frameIdx      int
		currentLevel  uint8
		rateCounter   uint16
		path          []uint8
		cumulativeErr float64
	}

	beamWidth := 100
	beam := []BeamPath{{
		frameIdx:      1,
		currentLevel:  currentLevel,
		rateCounter:   rateCounter,
		path:          []uint8{},
		cumulativeErr: 0.0,
	}}

	// Process each frame
	for frameIdx := 1; frameIdx < len(volumeCurve); frameIdx++ {
		// Expand all beam paths in parallel
		type expandResult struct {
			children []BeamPath
		}
		resultChan := make(chan expandResult, len(beam))
		var wg sync.WaitGroup

		for _, parent := range beam {
			if parent.frameIdx != frameIdx {
				continue
			}
			wg.Add(1)
			go func(p BeamPath) {
				defer wg.Done()

				targetCurve := volumeCurve[p.frameIdx:]
				var children []BeamPath

				// Generate all valid R candidates
				for r := uint8(0); r <= 15; r++ {
					ratePeriod := uint32(adsrRatePeriods[r])
					if ratePeriod < uint32(p.rateCounter) {
						continue // Would trigger bug
					}

					// Calculate envelope error for this R
					simulated := simulateReleaseWithBug(p.currentLevel, r, lookahead, p.rateCounter)
					envelopeError := 0.0
					for i := 0; i < lookahead && i < len(targetCurve); i++ {
						tVol := int(targetCurve[i] + 0.5)
						if tVol < 0 {
							tVol = 0
						}
						if tVol > 15 {
							tVol = 15
						}
						tNorm := ayVolumeTable[tVol] / 32767.0
						diff := simulated[i] - tNorm
						envelopeError += diff * diff
					}
					envelopeError = envelopeError / float64(lookahead)

					// Simulate one frame
					simulated1 := simulateReleaseWithBug(p.currentLevel, r, 1, p.rateCounter)
					newLevel := uint8(simulated1[0] * 255.0)
					newCounter := advanceCounter(p.rateCounter, ratePeriod)

					// Create child path
					newPath := make([]uint8, len(p.path)+1)
					copy(newPath, p.path)
					newPath[len(p.path)] = r

					// Add counter penalty to encourage low counters
					counterPenalty := 0.01 * float64(newCounter) / 32768.0

					children = append(children, BeamPath{
						frameIdx:      p.frameIdx + 1,
						currentLevel:  newLevel,
						rateCounter:   newCounter,
						path:          newPath,
						cumulativeErr: p.cumulativeErr + envelopeError + counterPenalty,
					})
				}

				resultChan <- expandResult{children: children}
			}(parent)
		}

		// Wait for all goroutines
		go func() {
			wg.Wait()
			close(resultChan)
		}()

		// Collect all children
		var allChildren []BeamPath
		for result := range resultChan {
			allChildren = append(allChildren, result.children...)
		}

		if len(allChildren) == 0 {
			fmt.Printf("WARNING: Beam search found no valid paths at frame %d\n", frameIdx)
			break
		}

		// Sort by error with counter as tiebreaker
		sort.Slice(allChildren, func(i, j int) bool {
			errDiff := allChildren[i].cumulativeErr - allChildren[j].cumulativeErr
			if math.Abs(errDiff) < 0.0005 { // Very tight error tolerance
				return allChildren[i].rateCounter < allChildren[j].rateCounter
			}
			return allChildren[i].cumulativeErr < allChildren[j].cumulativeErr
		})

		// Keep top K
		if len(allChildren) > beamWidth {
			beam = allChildren[:beamWidth]
		} else {
			beam = allChildren
		}
	}

	// Select best path (lowest cumulative error)
	if len(beam) > 0 {
		best := beam[0]
		copy(releasePerFrame[1:], best.path)
	} else {
		fmt.Printf("WARNING: Beam search failed for envelope type\n")
	}

	return releasePerFrame
}


// generateSIDAltEnvGate generates SID audio using alternative approach for envelope mode:
// - AY freq → SID note frequency (50% square wave)
// - AY env period → rapid gate on/off timing (4 cycle pulses)
// - Fast ADSR (A=0/1, D=0/1, S=0, R=0) to emulate triangle/saw

func generateSIDPerNote(commands []spectrum.AYCommand, channel int, notes []Note, totalSamples int) ([]int16, []int16) {
	sampleRate := 44100.0
	samplesPerFrame := int(sampleRate / palFrameRate)
	samples := make([]int16, totalSamples)

	// Debug channel: ADSR delay bug active state for this channel
	adsrBugDebug := make([]int16, totalSamples)

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

	// SID register offset for this channel (voice 0=0, voice 1=7, voice 2=14)
	regBase := byte(channel * 7)

	// Process each note with its specific ADSR
	currentNoteIdx := 0
	lastSIDFreq := uint16(0)
	gateOn := false
	currentWaveform := uint8(0x04) // Default to pulse
	inEnvelopeMode := false
	togglePeriodCycles := 0   // Toggle period in SID clock cycles (integer)
	cycleCounter := 0.0       // Accumulated SID clock cycles
	toneHigh := true          // Current state of tone (HIGH/LOW)
	lastToneState := true     // Last written tone state (persists across frames)
	lastToneFreq := uint16(0) // Track last tone frequency to detect changes

	for frame := 0; frame < numFrames; frame++ {
		// CRITICAL: Update R BEFORE gate OFF
		// The SID envelope locks in R value on gate OFF transition
		if currentNoteIdx > 0 && !inEnvelopeMode {
			currentNote := notes[currentNoteIdx-1]
			relFrame := frame - currentNote.StartFrame

			// At frame 1 (when gate will go OFF), set R for release phase
			// Frame 0: Use initial R=15 from ADSR setup (attack/sustain)
			// Frame 1: Set R to releasePerFrame[1] BEFORE gate goes OFF
			// Frame 2+: Update R per frame during release
			if relFrame >= 1 && relFrame <= currentNote.EndFrame-currentNote.StartFrame {
				if relFrame < len(currentNote.ReleasePerFrame) {
					newR := currentNote.ReleasePerFrame[relFrame]
					currentSustain := sid.Registers[regBase+6] >> 4
					sid.WriteRegister(regBase+6, (currentSustain<<4)|newR)
				}
			}
		}

		// Gate OFF logic: wait 1 frame after note starts
		// R value is already set above, so SID enters release with correct R
		if gateOn && currentNoteIdx > 0 && !inEnvelopeMode {
			currentNote := notes[currentNoteIdx-1]

			if frame == currentNote.StartFrame+1 {
				// Gate OFF - enters Release with R from releasePerFrame[1]
				sid.WriteRegister(regBase+4, currentWaveform<<4)
				gateOn = false
			}
		}

		// ADSR delay bug workaround: Set R=0 for 2 frames before next note
		// First frame wraps the counter from any position, second frame keeps it low
		// This guarantees rate_counter < 8 when next note's Gate ON with A=0 starts
		if currentNoteIdx > 0 && !inEnvelopeMode {
			currentNote := notes[currentNoteIdx-1]
			if frame == currentNote.EndFrame-1 || frame == currentNote.EndFrame {
				// Set R=0 for 2 frames to guarantee counter < 8
				currentSustain := sid.Registers[regBase+6] >> 4
				sid.WriteRegister(regBase+6, (currentSustain<<4)|0x00)
			}
		}

		// Check if we're starting a new note
		if currentNoteIdx < len(notes) && frame == notes[currentNoteIdx].StartFrame {
			note := notes[currentNoteIdx]

			// ALWAYS gate OFF before starting new note to ensure envelope restarts
			// Even if gateOn is false, we need to write gate=0 and wait for the
			// SID to process it before writing gate=1, so it sees the 0→1 transition
			// Set release to 0 for instant gate off
			prevADSR := sid.Registers[regBase+6]
			sid.WriteRegister(regBase+6, (prevADSR&0xF0)|0x00) // Keep sustain, set release=0
			sid.WriteRegister(regBase+4, currentWaveform<<4)   // Gate OFF

			// Generate multiple samples to give SID envelope time to fully process gate OFF
			// The envelope state machine needs several cycles to enter release and reset
			// Single sample (~22 cycles) wasn't enough - caused every-2nd-note failures
			for i := 0; i < 10; i++ {
				_ = sid.GenerateSample(channel)
			}

			// Use waveform from note
			currentWaveform = note.Waveform
			inEnvelopeMode = note.EnvelopeMode

			// Reset pulse width to 50% for non-envelope mode
			// (envelope mode will override this during waveform toggling)
			if !note.EnvelopeMode {
				sid.WriteRegister(regBase+2, 0x00) // Pulse width low byte = 0x00
				sid.WriteRegister(regBase+3, 0x08) // Pulse width high byte = 0x08 (12-bit = 0x800 = 50%)
			}

			// CRITICAL: Set ADSR values while gate is OFF (before gate ON)
			// Writing ADSR registers while gate=1 resets envelope to Attack
			// Use pre-computed ADSR (A=0/3, D=0, S=15)
			// R comes from ReleasePerFrame[0] for frame 0, then updated each frame
			initialR := note.ADSR.Release
			if len(note.ReleasePerFrame) > 0 {
				initialR = note.ReleasePerFrame[0]
			}
			sid.WriteRegister(regBase+5, (note.ADSR.Attack<<4)|note.ADSR.Decay)
			sid.WriteRegister(regBase+6, (note.ADSR.Sustain<<4)|initialR)

			// Set frequency based on waveform type and mode (MUST calculate this first for toggle period)
			var sidFreq uint16
			if note.Waveform == 0x08 {
				// Noise: compute frequency from AY noise period (R6)
				noisePeriod := getAYNoisePeriod(commands, frame)
				sidFreq = computeNoiseFrequency(noisePeriod)
			} else if note.EnvelopeMode {
				// Envelope mode: use envelope period frequency (R11-R12)
				envPeriod := getAYEnvelopePeriod(commands, frame)
				sidFreq = computeEnvelopeFrequency(envPeriod)
				// Sawtooth envelopes complete visual cycles 2x faster than triangle
				// Triangle: 64 envelope steps per cycle (32 up, 32 down)
				// Sawtooth: 32 envelope steps per cycle (32 up, then reset)
				if note.Waveform == 0x02 { // Sawtooth
					sidFreq *= 2
				}
			} else {
				// Direct mode: use tone frequency from R0-R5
				if frame < len(freqPerFrame) && freqPerFrame[frame] > 0 {
					ayClock := 1773400.0
					freqHz := ayClock / (16.0 * float64(freqPerFrame[frame]))
					// Round to nearest integer for optimal frequency match
					sidFreq = uint16(freqHz*16777216.0/985248.0 + 0.5)
				}
			}
			if sidFreq > 0 {
				sid.WriteRegister(regBase+0, byte(sidFreq&0xFF))
				sid.WriteRegister(regBase+1, byte(sidFreq>>8))
				lastSIDFreq = sidFreq
			}

			// Calculate toggle period in SID clock cycles for envelope mode (after sidFreq is set)
			if note.EnvelopeMode && frame < len(freqPerFrame) && freqPerFrame[frame] > 0 {
				ayClock := 1773400.0
				sidClock := 985248.0

				// Calculate AY frequencies
				ayTonePeriod := float64(freqPerFrame[frame])
				ayToneFreq := ayClock / (16.0 * ayTonePeriod)

				envPeriod := getAYEnvelopePeriod(commands, frame)
				ayEnvFreq := ayClock / (512.0 * float64(envPeriod))

				// Calculate ideal ratio
				ayRatio := ayToneFreq / ayEnvFreq

				// SID envelope frequency (after quantization) - use the NEW sidFreq we just calculated
				sidEnvFreq := (float64(sidFreq) * sidClock) / 16777216.0
				if currentWaveform == 0x02 { // Sawtooth was doubled
					sidEnvFreq /= 2.0
				}

				// Ideal toggle frequency to maintain ratio
				idealToggleFreq := sidEnvFreq * ayRatio

				// Toggle period in cycles (round to nearest integer)
				idealTogglePeriod := sidClock / (2.0 * idealToggleFreq)
				togglePeriodCycles = int(idealTogglePeriod + 0.5)
				if togglePeriodCycles < 1 {
					togglePeriodCycles = 1
				}
				// Don't reset cycleCounter - modulation oscillator runs continuously
			}

			// Reset pulse width to 50% for envelope mode
			if note.EnvelopeMode {
				sid.WriteRegister(regBase+2, 0x00) // Pulse width low byte = 0x00
				sid.WriteRegister(regBase+3, 0x08) // Pulse width high byte = 0x08 (0x800 = 50%)
			}

			// Gate on with selected waveform
			sid.WriteRegister(regBase+4, (currentWaveform<<4)|0x01)
			gateOn = true
			currentNoteIdx++
		} else if currentNoteIdx > 0 {
			// Update frequency and R during note (works whether gate is ON or OFF)
			var sidFreq uint16
			if currentWaveform == 0x08 {
				// Noise: update based on noise period changes
				noisePeriod := getAYNoisePeriod(commands, frame)
				sidFreq = computeNoiseFrequency(noisePeriod)
			} else if inEnvelopeMode {
				// Envelope mode: update based on envelope period changes (R11-R12)
				envPeriod := getAYEnvelopePeriod(commands, frame)
				sidFreq = computeEnvelopeFrequency(envPeriod)

				// Check for R13 (envelope shape) changes during note
				r13Shape := getAYEnvelopeShape(commands, frame)
				newWaveform := ayEnvelopeToSIDWaveform(r13Shape)
				if newWaveform != currentWaveform && newWaveform != 0x08 {
					currentWaveform = newWaveform
				}

				// Sawtooth envelopes complete visual cycles 2x faster than triangle
				if currentWaveform == 0x02 { // Sawtooth
					sidFreq *= 2
				}

				// Recalculate toggle period if tone frequency changed
				if frame < len(freqPerFrame) && freqPerFrame[frame] > 0 && freqPerFrame[frame] != lastToneFreq {
					lastToneFreq = freqPerFrame[frame]

					ayClock := 1773400.0
					sidClock := 985248.0

					// Calculate AY frequencies
					ayTonePeriod := float64(freqPerFrame[frame])
					ayToneFreq := ayClock / (16.0 * ayTonePeriod)

					envPeriod := getAYEnvelopePeriod(commands, frame)
					ayEnvFreq := ayClock / (512.0 * float64(envPeriod))

					ayRatio := ayToneFreq / ayEnvFreq

					// SID envelope frequency (after quantization)
					sidEnvFreq := (float64(sidFreq) * sidClock) / 16777216.0
					if currentWaveform == 0x02 {
						sidEnvFreq /= 2.0
					}

					// Ideal toggle frequency
					idealToggleFreq := sidEnvFreq * ayRatio
					idealTogglePeriod := sidClock / (2.0 * idealToggleFreq)

					// Round to nearest integer
					bestPeriod := int(idealTogglePeriod + 0.5)
					if bestPeriod < 1 {
						bestPeriod = 1
					}

					// Update period if changed (don't reset counter - oscillator should continue running)
					if bestPeriod != togglePeriodCycles {
						togglePeriodCycles = bestPeriod
					}
				}
			} else {
				// Direct mode: update based on tone frequency changes
				if frame < len(freqPerFrame) && freqPerFrame[frame] > 0 {
					ayClock := 1773400.0
					freqHz := ayClock / (16.0 * float64(freqPerFrame[frame]))
					// Round to nearest integer for optimal frequency match
					sidFreq = uint16(freqHz*16777216.0/985248.0 + 0.5)
				}
			}
			if sidFreq > 0 && sidFreq != lastSIDFreq {
				sid.WriteRegister(regBase+0, byte(sidFreq&0xFF))
				sid.WriteRegister(regBase+1, byte(sidFreq>>8))
				lastSIDFreq = sidFreq
			}

			// Check for note end
			currentNote := notes[currentNoteIdx-1]
			if frame > currentNote.EndFrame {
				hasAdjacentNote := currentNoteIdx < len(notes) && notes[currentNoteIdx].StartFrame <= frame

				if inEnvelopeMode {
					// Envelope mode: gate OFF when note ends (stops waveform toggling)
					if !hasAdjacentNote {
						sid.WriteRegister(regBase+4, currentWaveform<<4) // Gate OFF
					}
					gateOn = false
					inEnvelopeMode = false
				} else if !hasAdjacentNote {
					// Non-envelope mode: set R=0 for immediate silence after note ends
					currentSustain := sid.Registers[regBase+6] >> 4
					sid.WriteRegister(regBase+6, (currentSustain<<4)|0x00)
				}
			}
		}

		// Generate samples for this frame
		frameStart := frame * samplesPerFrame
		numSamples := samplesPerFrame
		if frameStart+numSamples > totalSamples {
			numSamples = totalSamples - frameStart
		}

		// Check if we need per-sample processing (dynamic waveform toggling)
		needsPerSample := channel == 0 && gateOn && inEnvelopeMode && currentWaveform != 0x08

		if needsPerSample {
			// Slow path: per-sample processing for dynamic waveform toggling
			sidClock := 985248.0
			sampleRate := 44100.0
			cyclesPerSample := sidClock / sampleRate // ~22.34 SID cycles per audio sample

			// Envelope mode: gate stays ON throughout entire note
			// Waveform toggling creates amplitude modulation, SID envelope stays in sustain
			gateBit := byte(0x01)

			for i := 0; i < numSamples; i++ {
				// Apply current waveform state (only write registers when state changes)
				if toneHigh != lastToneState {
					if toneHigh {
						sid.WriteRegister(regBase+4, (currentWaveform<<4)|gateBit) // Waveform with dynamic gate
					} else {
						// Use pulse waveform with pulse width 0xFFF (max) to produce minimum output
						sid.WriteRegister(regBase+2, 0xFF)           // Pulse width low byte = 0xFF
						sid.WriteRegister(regBase+3, 0x0F)           // Pulse width high byte = 0x0F (12-bit = 0xFFF)
						sid.WriteRegister(regBase+4, (0x04<<4)|gateBit) // Pulse waveform with dynamic gate
					}
					lastToneState = toneHigh
				}

				// Generate sample
				samples[frameStart+i] = sid.GenerateSample(channel)

				// Record bug state for this sample
				if sid.Voices[channel].ADSRDelayBugActive {
					adsrBugDebug[frameStart+i] = 32767
				} else {
					adsrBugDebug[frameStart+i] = 0
				}

				// Advance cycle counter and toggle if we've reached the period
				if togglePeriodCycles > 0 {
					cycleCounter += cyclesPerSample
					if cycleCounter >= float64(togglePeriodCycles) {
						toneHigh = !toneHigh
						cycleCounter -= float64(togglePeriodCycles)
					}
				}
			}
		} else {
			// Generate samples one at a time to track bug state accurately
			for i := 0; i < numSamples; i++ {
				samples[frameStart+i] = sid.GenerateSample(channel)

				// Record bug state for this sample (at audio sample rate)
				if sid.Voices[channel].ADSRDelayBugActive {
					adsrBugDebug[frameStart+i] = 32767
				} else {
					adsrBugDebug[frameStart+i] = 0
				}
			}
		}
	}

	return samples, adsrBugDebug
}

func generateSIDContinuous(commands []spectrum.AYCommand, channel int, volumeCurve []float64, adsr ADSRParams, totalSamples int) []int16 {
	sampleRate := 44100.0
	samplesPerFrame := int(sampleRate / palFrameRate)
	samples := make([]int16, totalSamples)

	sid := NewSIDEmulator(sampleRate)

	sid.WriteRegister(5, (adsr.Attack<<4)|adsr.Decay)
	sid.WriteRegister(6, (adsr.Sustain<<4)|adsr.Release)

	freqPerFrame := make([]uint16, len(volumeCurve))
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
	for _, r := range volumeCurve {
		if r > maxRMS {
			maxRMS = r
		}
	}
	threshold := maxRMS * 0.1

	gateOn := false
	lastSIDFreq := uint16(0)
	silentFrames := 0 // Count consecutive frames below threshold

	for frame := 0; frame < len(volumeCurve); frame++ {
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
		if volumeCurve[frame] <= threshold {
			silentFrames++
		} else {
			silentFrames = 0
		}

		// Gate ON: RMS rises above threshold (note starts)
		if volumeCurve[frame] > threshold && !gateOn {
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
		numSamples := samplesPerFrame
		if frameStart+numSamples > totalSamples {
			numSamples = totalSamples - frameStart
		}
		sid.GenerateSamplesBatch(0, samples[frameStart:frameStart+numSamples])
	}

	return samples
}

// resampleToLength resamples audio to exact target length using linear interpolation
func resampleToLength(input []int16, targetLen int) []int16 {
	if len(input) == targetLen {
		return input
	}

	output := make([]int16, targetLen)
	ratio := float64(len(input)) / float64(targetLen)

	for i := 0; i < targetLen; i++ {
		// Map output position to input position
		srcPos := float64(i) * ratio
		srcIdx := int(srcPos)
		frac := srcPos - float64(srcIdx)

		if srcIdx >= len(input)-1 {
			// At or past end of input
			output[i] = input[len(input)-1]
		} else {
			// Linear interpolation between samples
			sample1 := float64(input[srcIdx])
			sample2 := float64(input[srcIdx+1])
			output[i] = int16(sample1 + frac*(sample2-sample1))
		}
	}

	return output
}

// resampleAudio resamples audio by a given ratio using linear interpolation
func resampleAudio(input []int16, ratio float64) []int16 {
	if ratio == 1.0 {
		return input
	}

	newLen := int(float64(len(input)) * ratio)
	return resampleToLength(input, newLen)
}

func generateAYAudioChannel(commands []spectrum.AYCommand, channel, startFrame, endFrame int) []int16 {
	sampleRate := 44100
	// Use PAL frame rate for sample timing to match SID
	// The AY chip cycles (69888) remain the same, but frame timing uses PAL rate

	numFrames := endFrame - startFrame + 1
	// Calculate total samples using PAL frame rate to match SID frame alignment
	numSamples := int(float64(numFrames) * float64(sampleRate) / palFrameRate)

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

// qualityWarning holds analysis warning data
type qualityWarning struct {
	channel        int
	noteIdx        int
	timeSeconds    float64
	spectralError  float64
	amplitudeError float64
	waveformName   string
	note           Note
}

// analyzeConversionQuality compares AY and SID audio in frequency and time domains (highly parallel)
func analyzeConversionQuality(ayAudio [3][]int16, sidAudio [3][]int16, notes [3][]Note, verbose bool) {
	sampleRate := 44100
	windowSize := sampleRate / 25 // 1/25 second = 40ms windows
	numWorkers := 64

	// Build work queue with all notes across all channels
	type workItem struct {
		channel int
		noteIdx int
		note    Note
	}
	var workItems []workItem
	for ch := 0; ch < 3; ch++ {
		for noteIdx, note := range notes[ch] {
			workItems = append(workItems, workItem{
				channel: ch,
				noteIdx: noteIdx,
				note:    note,
			})
		}
	}

	workChan := make(chan workItem, len(workItems))
	for _, item := range workItems {
		workChan <- item
	}
	close(workChan)

	// Collect warnings from workers
	warningChan := make(chan qualityWarning, len(workItems))

	// Launch worker pool
	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for work := range workChan {
				ch := work.channel
				note := work.note

				// Both AY and SID are at palFrameRate after resampling
				startSample := int(float64(note.StartFrame) * float64(sampleRate) / palFrameRate)
				endSample := int(float64(note.EndFrame) * float64(sampleRate) / palFrameRate)

				if endSample > len(ayAudio[ch]) {
					endSample = len(ayAudio[ch])
				}
				if endSample > len(sidAudio[ch]) {
					endSample = len(sidAudio[ch])
				}

				// Use note duration or fixed window, whichever is smaller
				noteDuration := endSample - startSample
				analysisWindow := windowSize
				if noteDuration < windowSize && noteDuration > 100 {
					analysisWindow = noteDuration
				}
				if analysisWindow < 100 {
					continue // Skip very short notes
				}

				// Analyze the note window
				ayWindow := ayAudio[ch][startSample : startSample+analysisWindow]
				sidWindow := sidAudio[ch][startSample : startSample+analysisWindow]

				spectralError := compareSpectra(ayWindow, sidWindow)
				amplitudeError := compareAmplitude(ayWindow, sidWindow)

				if spectralError > 0.3 || amplitudeError > 0.3 {
					timeSeconds := float64(note.StartFrame) / palFrameRate
					waveformName := "unknown"
					switch note.Waveform {
					case 0x01:
						waveformName = "triangle"
					case 0x02:
						waveformName = "sawtooth"
					case 0x04:
						waveformName = "pulse"
					case 0x08:
						waveformName = "NOISE"
					}
					warningChan <- qualityWarning{
						channel:        ch,
						noteIdx:        work.noteIdx,
						timeSeconds:    timeSeconds,
						spectralError:  spectralError,
						amplitudeError: amplitudeError,
						waveformName:   waveformName,
						note:           note,
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(warningChan)
	}()

	// Collect warnings and organize by channel
	channelWarnings := make([][]qualityWarning, 3)
	for warning := range warningChan {
		channelWarnings[warning.channel] = append(channelWarnings[warning.channel], warning)
	}

	// Sort warnings by note index within each channel
	for ch := 0; ch < 3; ch++ {
		warnings := channelWarnings[ch]
		sort.Slice(warnings, func(i, j int) bool {
			return warnings[i].noteIdx < warnings[j].noteIdx
		})
	}

	// Print results in channel order
	for ch := 0; ch < 3; ch++ {
		channelName := string('A' + ch)
		fmt.Printf("Channel %s:\n", channelName)
		for _, warning := range channelWarnings[ch] {
			fmt.Printf("  WARNING: Note %d at %.2fs - spectral: %.3f, amplitude: %.3f - %s\n",
				warning.noteIdx+1, warning.timeSeconds, warning.spectralError,
				warning.amplitudeError, warning.waveformName)

			if verbose {
				fmt.Printf("    Note: frames %d-%d, waveform=0x%02X, ADSR: A=%d D=%d S=%d R=%d\n",
					warning.note.StartFrame, warning.note.EndFrame, warning.note.Waveform,
					warning.note.ADSR.Attack, warning.note.ADSR.Decay,
					warning.note.ADSR.Sustain, warning.note.ADSR.Release)
			}
		}
	}
}

// computeErrorChannels generates audio visualization of conversion errors (highly parallel)
func computeErrorChannels(ayAudio [3][]int16, sidAudio [3][]int16) [3][]int16 {
	var errorChannels [3][]int16
	windowSize := 882 // 20ms windows at 44.1kHz (one frame)
	numWorkers := 128

	for ch := 0; ch < 3; ch++ {
		minLen := len(ayAudio[ch])
		if len(sidAudio[ch]) < minLen {
			minLen = len(sidAudio[ch])
		}
		errorChannels[ch] = make([]int16, minLen)

		totalWindows := minLen / windowSize

		// Create work queue
		type workItem struct {
			idx int
			pos int
		}
		workChan := make(chan workItem, totalWindows)
		for i := 0; i+windowSize <= minLen; i += windowSize {
			workChan <- workItem{i / windowSize, i}
		}
		close(workChan)

		// Progress tracking
		var completed int
		var completedMu sync.Mutex

		// Launch workers
		var wg sync.WaitGroup
		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for work := range workChan {
					ayWindow := ayAudio[ch][work.pos : work.pos+windowSize]
					sidWindow := sidAudio[ch][work.pos : work.pos+windowSize]

					// Compute combined error metric
					spectralErr := compareSpectra(ayWindow, sidWindow)
					rmsErr := compareAmplitude(ayWindow, sidWindow)
					combinedErr := (spectralErr + rmsErr) / 2.0
					errorValue := int16(combinedErr * 16384.0)

					// Fill window
					for j := 0; j < windowSize; j++ {
						errorChannels[ch][work.pos+j] = errorValue
					}

					// Update progress
					completedMu.Lock()
					completed++
					if completed%500 == 0 || completed == totalWindows {
						pct := float64(completed) * 100.0 / float64(totalWindows)
						fmt.Printf("\r  Channel %d: %.1f%% (%d/%d windows)...", ch, pct, completed, totalWindows)
					}
					completedMu.Unlock()
				}
			}()
		}
		wg.Wait()
		fmt.Printf("\r  Channel %d: 100%% complete\n", ch)
	}

	return errorChannels
}

// compareAmplitude computes amplitude envelope difference between two audio windows
func compareAmplitude(ay []int16, sid []int16) float64 {
	if len(ay) == 0 || len(sid) == 0 {
		return 0.0
	}

	minLen := len(ay)
	if len(sid) < minLen {
		minLen = len(sid)
	}

	// Compute RMS for each
	ayRMS := 0.0
	sidRMS := 0.0
	for i := 0; i < minLen; i++ {
		ayVal := float64(ay[i]) / 32768.0
		sidVal := float64(sid[i]) / 32768.0
		ayRMS += ayVal * ayVal
		sidRMS += sidVal * sidVal
	}
	ayRMS = math.Sqrt(ayRMS / float64(minLen))
	sidRMS = math.Sqrt(sidRMS / float64(minLen))

	// Normalize and compare
	maxRMS := ayRMS
	if sidRMS > maxRMS {
		maxRMS = sidRMS
	}
	if maxRMS < 1e-10 {
		return 0.0
	}

	return math.Abs(ayRMS-sidRMS) / maxRMS
}

// compareSpectra computes spectral difference between two audio windows
func compareSpectra(ay []int16, sid []int16) float64 {
	if len(ay) == 0 || len(sid) == 0 {
		return 0.0
	}

	// Ensure same length
	minLen := len(ay)
	if len(sid) < minLen {
		minLen = len(sid)
	}

	// Compute magnitude spectrum for both
	aySpectrum := computeMagnitudeSpectrum(ay[:minLen])
	sidSpectrum := computeMagnitudeSpectrum(sid[:minLen])

	// Normalize both spectra to [0,1] for shape comparison
	ayNorm := normalizeSpectrum(aySpectrum)
	sidNorm := normalizeSpectrum(sidSpectrum)

	// Focus on lower frequency bins (fundamental + harmonics, up to ~5kHz)
	// At 44.1kHz sample rate with 1764 samples (40ms), each bin is ~25Hz
	maxBin := len(ayNorm) / 4 // Focus on lower quarter of spectrum
	if maxBin > 200 {
		maxBin = 200 // Cap at ~5kHz
	}

	// Compare normalized spectra using RMS error
	sumSquaredError := 0.0
	count := 0

	for i := 1; i < maxBin; i++ { // Skip DC bin
		diff := ayNorm[i] - sidNorm[i]
		sumSquaredError += diff * diff
		count++
	}

	if count == 0 {
		return 0.0
	}

	return math.Sqrt(sumSquaredError / float64(count))
}

// normalizeSpectrum normalizes a spectrum to [0,1] range
func normalizeSpectrum(spectrum []float64) []float64 {
	maxVal := 0.0
	for _, v := range spectrum {
		if v > maxVal {
			maxVal = v
		}
	}

	if maxVal < 1e-10 {
		return spectrum
	}

	normalized := make([]float64, len(spectrum))
	for i, v := range spectrum {
		normalized[i] = v / maxVal
	}
	return normalized
}

// computeMagnitudeSpectrum computes the magnitude spectrum using DFT
func computeMagnitudeSpectrum(samples []int16) []float64 {
	n := len(samples)
	// Only compute lower half of spectrum (Nyquist)
	numBins := n / 2
	spectrum := make([]float64, numBins)

	// Simple DFT (could be optimized with FFT for larger windows)
	for k := 0; k < numBins; k++ {
		real := 0.0
		imag := 0.0

		for t := 0; t < n; t++ {
			sample := float64(samples[t])
			angle := 2.0 * math.Pi * float64(k) * float64(t) / float64(n)
			real += sample * math.Cos(angle)
			imag += sample * math.Sin(angle)
		}

		// Magnitude
		magnitude := math.Sqrt(real*real + imag*imag)
		spectrum[k] = magnitude / float64(n)
	}

	return spectrum
}

func writeSevenChannelWAV(path string, ayChannels [][]int16, sidChannels [][]int16, adsrBugDebug []int16, sampleRate int) {
	numSamples := len(ayChannels[0])
	for ch := 1; ch < 3; ch++ {
		if len(ayChannels[ch]) < numSamples {
			numSamples = len(ayChannels[ch])
		}
		if len(sidChannels[ch]) < numSamples {
			numSamples = len(sidChannels[ch])
		}
	}
	if len(adsrBugDebug) < numSamples {
		numSamples = len(adsrBugDebug)
	}

	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating WAV: %v\n", err)
		return
	}
	defer f.Close()

	// Use buffered writer for performance
	w := bufio.NewWriterSize(f, 1024*1024) // 1MB buffer
	defer w.Flush()

	numChannels := 7 // 3 AY + 3 SID + 1 debug (ADSR bug)
	dataSize := uint32(numSamples * numChannels * 2)

	w.Write([]byte("RIFF"))
	writeUint32Buf(w, dataSize+36)
	w.Write([]byte("WAVE"))

	w.Write([]byte("fmt "))
	writeUint32Buf(w, 16)
	writeUint16Buf(w, 1)
	writeUint16Buf(w, uint16(numChannels))
	writeUint32Buf(w, uint32(sampleRate))
	writeUint32Buf(w, uint32(sampleRate*numChannels*2))
	writeUint16Buf(w, uint16(numChannels*2))
	writeUint16Buf(w, 16)

	w.Write([]byte("data"))
	writeUint32Buf(w, dataSize)

	// Interleave samples (AY, SID for each channel, then ADSR bug debug)
	for i := 0; i < numSamples; i++ {
		for ch := 0; ch < 3; ch++ {
			writeSampleBuf(w, ayChannels[ch][i])
			writeSampleBuf(w, sidChannels[ch][i])
		}
		// Debug channel 7: ADSR delay bug (-1=none, +1=A, +0.33=B, -0.33=C)
		writeSampleBuf(w, adsrBugDebug[i])
	}
}

func writeSixChannelWAV(path string, ayChannels [][]int16, sidChannels [][]int16, sampleRate int) {
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

	// Use buffered writer for performance
	w := bufio.NewWriterSize(f, 1024*1024) // 1MB buffer
	defer w.Flush()

	numChannels := 6 // 3 AY + 3 SID
	dataSize := uint32(numSamples * numChannels * 2)

	w.Write([]byte("RIFF"))
	writeUint32Buf(w, dataSize+36)
	w.Write([]byte("WAVE"))

	w.Write([]byte("fmt "))
	writeUint32Buf(w, 16)
	writeUint16Buf(w, 1)
	writeUint16Buf(w, uint16(numChannels))
	writeUint32Buf(w, uint32(sampleRate))
	writeUint32Buf(w, uint32(sampleRate*numChannels*2))
	writeUint16Buf(w, uint16(numChannels*2))
	writeUint16Buf(w, 16)

	w.Write([]byte("data"))
	writeUint32Buf(w, dataSize)

	// Interleave samples (AY, SID for each channel)
	for i := 0; i < numSamples; i++ {
		for ch := 0; ch < 3; ch++ {
			writeSampleBuf(w, ayChannels[ch][i])
			writeSampleBuf(w, sidChannels[ch][i])
		}
	}
}

func writeMultiChannelWAV(path string, ayChannels [][]int16, sidChannels [][]int16, errorChannels [][]int16, sampleRate int) {
	numSamples := len(ayChannels[0])
	for ch := 1; ch < 3; ch++ {
		if len(ayChannels[ch]) < numSamples {
			numSamples = len(ayChannels[ch])
		}
		if len(sidChannels[ch]) < numSamples {
			numSamples = len(sidChannels[ch])
		}
		if len(errorChannels[ch]) < numSamples {
			numSamples = len(errorChannels[ch])
		}
	}

	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating WAV: %v\n", err)
		return
	}
	defer f.Close()

	numChannels := 9 // 3 AY + 3 SID + 3 Error
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

	// Interleave samples (AY, SID, Error for each channel)
	for i := 0; i < numSamples; i++ {
		for ch := 0; ch < 3; ch++ {
			writeSample(f, ayChannels[ch][i])
			writeSample(f, sidChannels[ch][i])
			writeSample(f, errorChannels[ch][i])
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

// Buffered versions for performance
func writeUint16Buf(w *bufio.Writer, v uint16) {
	w.Write([]byte{byte(v), byte(v >> 8)})
}

func writeUint32Buf(w *bufio.Writer, v uint32) {
	w.Write([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
}

func writeSampleBuf(w *bufio.Writer, s int16) {
	w.Write([]byte{byte(s), byte(s >> 8)})
}
