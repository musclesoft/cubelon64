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
//     --output=output.wav \
//     --verbose
//
// Output: 6-channel WAV file
//   - Channels 0-1: AY Channel A, SID Voice 1
//   - Channels 2-3: AY Channel B, SID Voice 2
//   - Channels 4-5: AY Channel C, SID Voice 3
//
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"cubelon-emu/spectrum"
)

// Note represents a detected note from RMS
type Note struct {
	StartFrame     int
	EndFrame       int
	RMSCurve       []float64
	ADSR           ADSRParams
	UseNoise       bool // If true, use noise waveform (0x08)
	Waveform       byte // SID waveform: 0x01=triangle, 0x02=sawtooth, 0x04=pulse, 0x08=noise
	EnvelopeMode   bool // If true, note uses AY envelope mode (bit 4 of volume register)
	EnvelopeTypeID int  // ID of the envelope type this note belongs to
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
	RepresentCurve  []float64    // Representative normalized curve
	ADSR            ADSRParams   // Fitted ADSR for this type
	NoteIndices     []int        // Indices of notes using this type
	ChannelID       int          // Which channel (0=A, 1=B, 2=C)
}

// candidateCurve represents a candidate envelope curve for clustering
type candidateCurve struct {
	noteIdx    int
	normalized []float64
	isComplete bool
}

// SID ADSR rate tables (cycles per step at 1MHz)
var adsrRates = [16]uint32{
	9, 32, 63, 95, 149, 220, 267, 313,
	392, 977, 1954, 3126, 3907, 11720, 19532, 31251,
}

func main() {
	inputFile := flag.String("input", "", "Input AY command log (.bin)")
	outputWav := flag.String("output", "output.wav", "Output 6-channel WAV")
	minNoteFrames := flag.Int("min-note", 3, "Minimum note length in frames")
	verbose := flag.Bool("verbose", false, "Show detailed analysis")
	shapesOnly := flag.Bool("shapes-only", false, "Only analyze envelope shapes (skip WAV generation)")

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

	// Detect notes from AY volume commands (no thresholds)
	notes := [3][]Note{}
	for ch := 0; ch < 3; ch++ {
		fmt.Printf("\nAnalyzing Channel %c volume commands...\n", 'A'+ch)
		notes[ch] = detectNotesFromAYVolume(commands, ch, rmsPerFrame[ch], *minNoteFrames, *verbose)
		fmt.Printf("  Found %d notes\n", len(notes[ch]))
	}

	// Analyze volume phases in each note
	fmt.Printf("\n=== Analyzing Volume Phases ===\n")
	channelEnvelopeTypes := [3][]EnvelopeType{}
	globalTypeID := 0
	for ch := 0; ch < 3; ch++ {
		fmt.Printf("Channel %c: Analyzing %d notes...\n", 'A'+ch, len(notes[ch]))
		channelEnvelopeTypes[ch] = analyzeVolumePhasesWithOffset(notes[ch], *verbose, globalTypeID)

		// Update global offset
		globalTypeID += len(channelEnvelopeTypes[ch])
	}

	// Show all detected envelope types
	fmt.Printf("\n=== Detected Envelope Types per Channel ===\n")
	for ch := 0; ch < 3; ch++ {
		fmt.Printf("Channel %c: %d envelope types\n", 'A'+ch, len(channelEnvelopeTypes[ch]))
		for _, envType := range channelEnvelopeTypes[ch] {
			fmt.Printf("  Type %d (%d notes): A=%d D=%d S=%d R=%d\n",
				envType.ID, len(envType.NoteIndices),
				envType.ADSR.Attack, envType.ADSR.Decay,
				envType.ADSR.Sustain, envType.ADSR.Release)
		}
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
		fmt.Printf("\n✓ Shape analysis complete (skipped WAV generation)\n")
		return
	}

	// Generate comparison WAV
	fmt.Printf("\n=== Generating Final WAV ===\n")
	ayAudio, sidAudio := generateComparisonWAV(commands, rmsPerFrame, notes, channelADSR, outputWav)

	fmt.Printf("✓ Complete! Written to: %s\n", *outputWav)
	fmt.Printf("\n6-channel WAV layout:\n")
	fmt.Printf("  0-1: AY Channel A, SID Voice 1\n")
	fmt.Printf("  2-3: AY Channel B, SID Voice 2\n")
	fmt.Printf("  4-5: AY Channel C, SID Voice 3\n")

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
	fitADSRPerEnvelopeType(envelopeTypes, verbose)

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
	skippedTooShort := 0
	skippedSilent := 0
	candidatesWithRising := 0
	shortNotesWithRising := 0

	for i := range notes {
		// Skip envelope mode notes (already have A=0 D=0 S=15 R=0)
		if notes[i].EnvelopeMode {
			skippedEnvMode++
			continue
		}

		// Check if short notes have rising volume
		if len(notes[i].RMSCurve) < 10 {
			if len(notes[i].RMSCurve) >= 2 {
				for j := 1; j < len(notes[i].RMSCurve) && j < 5; j++ {
					if notes[i].RMSCurve[j] > notes[i].RMSCurve[j-1]+0.3 {
						shortNotesWithRising++
						if verbose && shortNotesWithRising <= 3 {
							fmt.Printf("  DEBUG: Short note %d (%d frames) has rising: [%.1f, %.1f, %.1f]\n",
								i, len(notes[i].RMSCurve), notes[i].RMSCurve[0], notes[i].RMSCurve[1],
								notes[i].RMSCurve[2])
						}
						break
					}
				}
			}
			skippedTooShort++
			continue
		}

		// Use raw curve (0-15 scale, no normalization)
		raw := notes[i].RMSCurve

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
		fmt.Printf("  Found %d candidate envelope curves (%d with rising attack)\n", len(candidates), candidatesWithRising)
		fmt.Printf("  Skipped: %d envelope mode, %d too short, %d silent\n", skippedEnvMode, skippedTooShort, skippedSilent)
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
	similarityThreshold := 0.15 // RMS error threshold for considering curves similar

	for _, cand := range candidates {
		// Try to find existing type that matches
		foundMatch := false
		for typeIdx := range types {
			similarity := curveSimilarity(cand.normalized, types[typeIdx].RepresentCurve)
			if similarity < similarityThreshold {
				// Add to this type
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
func curveSimilarity(curve1, curve2 []float64) float64 {
	// Resample to common length (use shorter length)
	length := len(curve1)
	if len(curve2) < length {
		length = len(curve2)
	}

	resample1 := resampleCurve(curve1, length)
	resample2 := resampleCurve(curve2, length)

	// Compute RMS error
	sumSquaredError := 0.0
	for i := 0; i < length; i++ {
		diff := resample1[i] - resample2[i]
		sumSquaredError += diff * diff
	}
	return math.Sqrt(sumSquaredError / float64(length))
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
func fitADSRPerEnvelopeType(types []EnvelopeType, verbose bool) {
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

		// Fit ADSR for this type's representative curve
		types[i].ADSR = detectVolumePhasesDebug(types[i].RepresentCurve, verbose && i < 5)

		if verbose && i < 5 {
			fmt.Printf("      → A=%d D=%d S=%d R=%d\n",
				types[i].ADSR.Attack, types[i].ADSR.Decay,
				types[i].ADSR.Sustain, types[i].ADSR.Release)
		}
	}

	fmt.Printf("  ADSR fitting complete\n")
}

// detectVolumePhasesDebug is a debug version that shows envelope shapes
func detectVolumePhasesDebug(rms []float64, debug bool) ADSRParams {
	if !debug {
		return detectVolumePhases(rms)
	}

	if len(rms) == 0 {
		return ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}
	}

	// Fit ADSR directly to raw AY volumes (no normalization)
	result := detectVolumePhases(rms)

	// Find max to check scale
	maxVal := 0.0
	for _, v := range rms {
		if v > maxVal {
			maxVal = v
		}
	}

	// Print summary
	fmt.Printf("      [%d frames: %.1f → %.1f, max: %.1f]\n",
		len(rms), rms[0], rms[len(rms)-1], maxVal)

	// Generate and print ASCII visualization (use actual frame count for better resolution)
	visualLines := len(rms)
	if visualLines > 30 {
		visualLines = 30 // Cap at 30 lines for readability
	}
	if visualLines < 10 {
		visualLines = 10 // Minimum 10 lines
	}

	// Normalize for visualization (0-15 range → 0-1 for plotEnvelope)
	normalized := make([]float64, len(rms))
	for i := range rms {
		normalized[i] = rms[i] / 15.0
	}

	fmt.Printf("      AY:\n")
	fmt.Printf("      %s\n", plotEnvelope(normalized, visualLines))

	// Simulate fitted SID ADSR for comparison (returns 0-1 scale, same as normalized)
	simulated := simulateFullADSR(result.Attack, result.Decay, result.Sustain, result.Release, len(rms), len(rms))
	fmt.Printf("      SID:\n")
	fmt.Printf("      %s\n", plotEnvelope(simulated, visualLines))

	return result
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

		// Add newline except for last line
		if i < len(resampled)-1 {
			result += line + "\n      "
		} else {
			result += line
		}
	}
	return result
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
					notes[i].EnvelopeTypeID = typeID
					appliedCount++
					break
				}
			}
		} else {
			// Fallback: use default ADSR
			notes[i].ADSR = ADSRParams{Attack: 1, Decay: 9, Sustain: 1, Release: 0}
			notes[i].EnvelopeTypeID = -1
		}
	}

	fmt.Printf("  Applied ADSR to %d notes\n", appliedCount)
}

// Old analyzeVolumePhases implementation (now unused, keeping detectVolumePhases)

// detectVolumePhases detects ADSR by searching for best fit through all combinations
func detectVolumePhases(rms []float64) ADSRParams {
	if len(rms) == 0 {
		return ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}
	}

	// Don't normalize - fit directly to raw AY volume levels (0-15)
	// This preserves absolute volume information for better fitting
	maxVal := 0.0
	for _, v := range rms {
		if v > maxVal {
			maxVal = v
		}
	}

	if maxVal < 0.001 {
		// Flat envelope - treat as sustain only
		return ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}
	}

	// Check if there's an actual attack phase (rising volume at start)
	// Look for increasing volume in the first few frames
	hasAttack := false
	if len(rms) >= 2 {
		// Check if the first few frames show increasing volume (0.5+ volume increase threshold)
		for i := 1; i < len(rms) && i < 5; i++ {
			if rms[i] > rms[i-1]+0.5 {
				hasAttack = true
				break
			}
		}
	}

	// Search through all ADSR combinations to find best fit
	// Pass raw AY volumes (0-15 scale) directly
	result := searchBestADSR(rms, hasAttack)

	return result
}

// searchBestADSR searches all ADSR combinations to find the one with minimum error
// Uses a coarse-to-fine search strategy for speed
func searchBestADSR(target []float64, hasAttack bool) ADSRParams {
	// Stage 1: Coarse search (step by 4)
	bestADSR := searchADSRWithStep(target, 4, hasAttack)

	// Stage 2: Fine search around best result (±4 range, step by 1)
	bestADSR = searchADSRAroundPoint(target, bestADSR, 4, hasAttack)

	// Stage 3: If attack was detected, do a dedicated attack-only fine search
	// This is critical for matching rising phases accurately
	if hasAttack {
		bestADSR = refineAttackValue(target, bestADSR)
	}

	return bestADSR
}

// refineAttackValue does a fine search over all attack values while keeping D/S/R fixed
func refineAttackValue(target []float64, current ADSRParams) ADSRParams {
	bestADSR := current
	bestError := 1e9

	gateOffFrame := int(float64(len(target)) * 0.8)

	// Try all attack values from 0-15, but weight the early part of the curve more heavily
	// This ensures we match the attack phase accurately
	for a := uint8(0); a <= 15; a++ {
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
func searchADSRWithStep(target []float64, step uint8, hasAttack bool) ADSRParams {
	bestADSR := ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}
	bestError := 1e9

	// Assume gate turns off at 80% of note (most common for music)
	gateOffFrame := int(float64(len(target)) * 0.8)

	// Only search A>0 if the AY curve has an actual rising attack phase
	aMax := uint8(0)
	if hasAttack {
		aMax = 15
	}

	for a := uint8(0); a <= aMax; a += step {
		for d := uint8(0); d <= 15; d += step {
			for s := uint8(0); s <= 15; s += step {
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
func searchADSRAroundPoint(target []float64, center ADSRParams, radius uint8, hasAttack bool) ADSRParams {
	bestADSR := center
	bestError := 1e9

	gateOffFrame := int(float64(len(target)) * 0.8)

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

	aMin := clamp(int(center.Attack) - int(radius))
	aMax := clamp(int(center.Attack) + int(radius))

	// Force A=0 if no attack phase detected
	if !hasAttack {
		aMin = 0
		aMax = 0
	}

	dMin := clamp(int(center.Decay) - int(radius))
	dMax := clamp(int(center.Decay) + int(radius))
	sMin := clamp(int(center.Sustain) - int(radius))
	sMax := clamp(int(center.Sustain) + int(radius))
	rMin := clamp(int(center.Release) - int(radius))
	rMax := clamp(int(center.Release) + int(radius))

	for a := aMin; a <= aMax; a++ {
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

// detectVolumePhasesOld detects ADSR from volume curve by directly measuring time characteristics
func detectVolumePhasesOld(rms []float64) ADSRParams {
	if len(rms) == 0 {
		return ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}
	}

	// Normalize to 0-1 range (min->max becomes 0->1)
	minVal := rms[0]
	maxVal := rms[0]
	for _, v := range rms {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	rangeVal := maxVal - minVal
	if rangeVal < 0.001 {
		// Flat envelope - treat as sustain only
		return ADSRParams{Attack: 0, Decay: 0, Sustain: 15, Release: 0}
	}

	normalized := make([]float64, len(rms))
	for i := range rms {
		normalized[i] = (rms[i] - minVal) / rangeVal
	}

	// ATTACK: Measure frames to reach peak (no threshold)
	// Find the peak value in the curve
	peakValue := 0.0
	peakFrame := 0
	for i, v := range normalized {
		if v > peakValue {
			peakValue = v
			peakFrame = i
		}
	}

	// Attack time is the number of frames to reach peak
	attackFrames := peakFrame

	// Map frames directly to SID attack rates
	// At 50Hz: 1 frame = 20ms
	// SID rates: A=0:2ms, A=1:8ms, A=2:16ms, A=3:24ms, A=4:38ms, A=5:56ms, A=6:68ms, A=7:80ms, A=8:100ms
	var attack uint8
	if attackFrames == 0 {
		attack = 0 // Instant
	} else if attackFrames == 1 {
		attack = 2 // ~20ms
	} else if attackFrames == 2 {
		attack = 4 // ~40ms
	} else if attackFrames == 3 {
		attack = 5 // ~60ms
	} else if attackFrames == 4 {
		attack = 7 // ~80ms
	} else {
		attack = 8 // 100ms+
	}

	// SUSTAIN: Use the average value of the last 20% of the note
	// This represents the stable level the note settles at
	sustainStart := int(float64(len(normalized)) * 0.8)
	if sustainStart >= len(normalized) {
		sustainStart = len(normalized) - 1
	}
	if sustainStart < 0 {
		sustainStart = 0
	}

	// Average the tail to get sustain level
	sustainSum := 0.0
	sustainCount := 0
	for i := sustainStart; i < len(normalized); i++ {
		sustainSum += normalized[i]
		sustainCount++
	}

	sustainLevel := 0.0
	if sustainCount > 0 {
		sustainLevel = sustainSum / float64(sustainCount)
	}

	// Map directly to SID sustain (0-15), no capping
	sustain := uint8(sustainLevel * 15.0 + 0.5) // +0.5 for rounding
	if sustain > 15 {
		sustain = 15
	}

	// DECAY: Measure frames from peak to sustain level
	// Find where curve reaches the sustain level
	decayFrames := 0
	actualSustainLevel := float64(sustain) / 15.0

	// Find where we reach the sustain level after peak
	for i := peakFrame + 1; i < len(normalized); i++ {
		if normalized[i] <= actualSustainLevel {
			decayFrames = i - peakFrame
			break
		}
	}

	// Map measured decay frames to SID decay rates
	// D=0: 6ms, D=1: 24ms, D=2: 48ms, D=3: 72ms, D=4: 114ms, D=5: 168ms, D=6: 204ms, D=7: 240ms, D=8: 300ms
	// At 50Hz: 1 frame = 20ms
	var decay uint8
	if decayFrames <= 1 {
		decay = 0 // 0-20ms
	} else if decayFrames <= 2 {
		decay = 1 // ~40ms
	} else if decayFrames <= 3 {
		decay = 2 // ~60ms
	} else if decayFrames <= 4 {
		decay = 3 // ~80ms
	} else if decayFrames <= 6 {
		decay = 4 // ~120ms
	} else if decayFrames <= 8 {
		decay = 5 // ~160ms
	} else if decayFrames <= 10 {
		decay = 6 // ~200ms
	} else if decayFrames <= 12 {
		decay = 7 // ~240ms
	} else {
		decay = 8 // 300ms+
	}

	return ADSRParams{
		Attack:  attack,
		Decay:   decay,
		Sustain: sustain,
		Release: 0, // Not used in most cases
	}
}

// simulateADS simulates Attack-Decay-Sustain envelope (gate stays on)
// Uses frame-level approximation with exponential rate dividers for fast search
func simulateADS(attack, decay, sustain uint8, numFrames int) []float64 {
	envelope := make([]float64, numFrames)
	cyclesPerFrame := 985248.0 / 50.0

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
	cyclesPerFrame := 985248.0 / 50.0

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
	cyclesPerFrame := 985248.0 / 50.0

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
	cyclesPerFrame := 985248.0 / 50.0

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
				// Exponential rate: slower as envelope increases
				effectiveRate := ratePeriod
				if envelopeLevel >= 157 {
					effectiveRate = ratePeriod * 9
				} else if envelopeLevel >= 93 {
					effectiveRate = ratePeriod * 3
				}

				if envelopeCounter >= effectiveRate {
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
					// Exponential rate: slower as envelope decreases
					effectiveRate := ratePeriod
					if envelopeLevel < 93 {
						effectiveRate = ratePeriod * 9
					} else if envelopeLevel < 157 {
						effectiveRate = ratePeriod * 3
					}

					if envelopeCounter >= effectiveRate {
						envelopeCounter = 0
						if envelopeLevel > 0 {
							envelopeLevel--
						}
					}
				}

			case 2: // Release
				ratePeriod := float64(adsrRates[release])
				// Exponential rate: slower as envelope decreases
				effectiveRate := ratePeriod
				if envelopeLevel < 93 {
					effectiveRate = ratePeriod * 9
				} else if envelopeLevel < 157 {
					effectiveRate = ratePeriod * 3
				}

				if envelopeCounter >= effectiveRate {
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
//   0x00-0x07: Single-cycle envelopes
//   0x08: \\\\\ Repeating decay (sawtooth down)
//   0x0A: \/\/\/ Repeating triangle
//   0x0C: ///// Repeating attack (sawtooth up)
//   0x0E: /\/\/\ Repeating triangle (inverted)
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

// detectNotesFromAYVolume detects notes from AY volume register writes
// For direct volume mode: each volume increase triggers a note
// For envelope mode: detect from RMS (silence to sound transitions)
func detectNotesFromAYVolume(commands []spectrum.AYCommand, channel int, rms []float64, minNoteFrames int, verbose bool) []Note {
	var notes []Note

	volReg := uint8(8 + channel) // R8=A, R9=B, R10=C

	// Track envelope mode status per frame
	envelopeModeActive := make([]bool, len(rms))
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

	mixerWrites := make(map[int]byte)       // frame -> mixer value
	envelopeShapeWrites := make(map[int]byte) // frame -> envelope shape register (R13)

	for _, cmd := range commands {
		if cmd.Register == 7 { // Mixer register
			frame := int(cmd.Frame)
			if frame < len(rms) {
				mixerWrites[frame] = cmd.Value
			}
		} else if cmd.Register == 13 { // Envelope shape register
			frame := int(cmd.Frame)
			if frame < len(rms) {
				envelopeShapeWrites[frame] = cmd.Value
			}
		}
	}

	// Forward fill mixer and envelope shape, then determine waveform
	noiseEnabled := make([]bool, len(rms))
	envelopeShape := make([]byte, len(rms))
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
		hasAudio := i < len(rms) && rms[i] > 0.0

		noiseEnabled[i] = !toneOn && noiseOn && hasAudio
	}

	// Build array of volume level per frame (extract from volume register writes)
	volumePerFrame := make([]float64, len(rms))

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

	// Second pass: forward fill
	currentVolLevel := uint8(0)
	for i := 0; i < len(volumePerFrame); i++ {
		if newVol, hasChange := volumeChanges[i]; hasChange {
			currentVolLevel = newVol
		}
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
		if frame >= len(rms) {
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
						RMSCurve:     curve,
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
						RMSCurve:     curve,
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
	if noteStart >= 0 && noteStart < len(rms) {
		noteLength := len(rms) - noteStart
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
				for f := noteStart; f < len(rms); f++ {
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
				StartFrame: noteStart,
				EndFrame:   len(rms) - 1,
				RMSCurve:   curve,
				UseNoise:   usesNoise,
				Waveform:   waveform,
				ADSR:       adsr,
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

func generateComparisonWAV(commands []spectrum.AYCommand, rmsPerFrame [3][]float64, notes [3][]Note, channelADSR [3]ADSRParams, outputPath *string) ([3][]int16, [3][]int16) {
	sampleRate := 44100
	durationFrames := int(commands[len(commands)-1].Frame)
	durationSamples := durationFrames * sampleRate / 50

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

	fmt.Printf("Generating SID audio (3 channels)...\n")
	sidStart := time.Now()
	var sidAudio [3][]int16
	for ch := 0; ch < 3; ch++ {
		wg.Add(1)
		go func(channel int) {
			defer wg.Done()
			sidAudio[channel] = generateSIDPerNote(commands, channel, notes[channel], channelADSR[channel], durationSamples)
		}(ch)
	}
	wg.Wait()
	sidDuration := time.Since(sidStart)
	fmt.Printf("  SID generation took %.1fs\n", sidDuration.Seconds())

	fmt.Printf("Writing WAV file...\n")
	writeStart := time.Now()
	writeSixChannelWAV(*outputPath, ayAudio[:], sidAudio[:], sampleRate)
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

	// SID register offset for this channel (voice 0=0, voice 1=7, voice 2=14)
	regBase := byte(channel * 7)

	// Process each note with its specific ADSR
	currentNoteIdx := 0
	lastSIDFreq := uint16(0)
	gateOn := false
	currentWaveform := uint8(0x04) // Default to pulse
	inEnvelopeMode := false
	tonePhase := 0.0 // AY tone phase for envelope modulation (0.0-1.0)

	for frame := 0; frame < numFrames; frame++ {
		// Check if we need to gate OFF (note ended, no upcoming note)
		if gateOn && currentNoteIdx > 0 && frame > notes[currentNoteIdx-1].EndFrame {
			// Check if there's an adjacent note coming
			hasAdjacentNote := currentNoteIdx < len(notes) && notes[currentNoteIdx].StartFrame <= frame+1
			if !hasAdjacentNote {
				sid.WriteRegister(regBase+4, currentWaveform<<4)
				gateOn = false
				inEnvelopeMode = false
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

			// Process at least 4 SID CPU cycles (generate 1 sample = ~22 cycles at 44.1kHz)
			// This ensures the SID envelope processes the gate-off before gate-on
			dummy := sid.GenerateSample(channel)
			_ = dummy

			// Use waveform from note
			currentWaveform = note.Waveform
			inEnvelopeMode = note.EnvelopeMode
			tonePhase = 0.0 // Reset tone phase for new note

			// Set this note's specific ADSR
			sid.WriteRegister(regBase+5, (note.ADSR.Attack<<4)|note.ADSR.Decay)
			sid.WriteRegister(regBase+6, (note.ADSR.Sustain<<4)|note.ADSR.Release)

			// Reset pulse width to 50% for non-envelope mode
			// (envelope mode will override this during waveform toggling)
			if !note.EnvelopeMode {
				sid.WriteRegister(regBase+2, 0x00) // Pulse width low byte = 0x00
				sid.WriteRegister(regBase+3, 0x08) // Pulse width high byte = 0x08 (12-bit = 0x800 = 50%)
			}

			// Set frequency based on waveform type and mode
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

			// Gate on with selected waveform
			sid.WriteRegister(regBase+4, (currentWaveform<<4)|0x01)
			gateOn = true
			currentNoteIdx++
		} else if gateOn && currentNoteIdx > 0 {
			// Update frequency during note based on current waveform and mode
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
			for i := 0; i < numSamples; i++ {
				// Calculate AY tone state (HIGH when phase < 0.5, LOW when phase >= 0.5)
				toneHigh := tonePhase < 0.5

				// Toggle SID waveform: full waveform when tone HIGH, pulse with width 0xFFF when tone LOW
				if toneHigh {
					sid.WriteRegister(regBase+4, (currentWaveform<<4)|0x01) // Gate ON with waveform
				} else {
					// Use pulse waveform with pulse width 0xFFF (max) to produce minimum output
					// PW 0xFFF means pulse is LOW (output -1.0) for nearly the entire cycle
					sid.WriteRegister(regBase+2, 0xFF) // Pulse width low byte = 0xFF
					sid.WriteRegister(regBase+3, 0x0F) // Pulse width high byte = 0x0F (12-bit = 0xFFF)
					sid.WriteRegister(regBase+4, (0x04<<4)|0x01) // Gate ON with pulse waveform
				}

				// Advance tone phase preserving the ratio relative to SID envelope frequency
				if frame < len(freqPerFrame) && freqPerFrame[frame] > 0 && lastSIDFreq > 0 {
					ayClock := 1773400.0
					sidClock := 985248.0

					// Calculate AY frequencies per datasheet spec
					// Tone generator: Clock / (16 × Period)
					// Envelope generator: Clock / (512 × Period)
					ayTonePeriod := float64(freqPerFrame[frame])
					ayToneFreq := ayClock / (16.0 * ayTonePeriod)

					envPeriod := getAYEnvelopePeriod(commands, frame)
					ayEnvFreq := ayClock / (512.0 * float64(envPeriod))

					// Calculate ratio of AY tone to envelope frequency
					ayRatio := ayToneFreq / ayEnvFreq

					// Apply same ratio to actual SID frequency (after rounding)
					// For sawtooth, lastSIDFreq was doubled, so divide by 2 to get base envelope freq
					baseSIDFreq := float64(lastSIDFreq)
					if currentWaveform == 0x02 { // Sawtooth
						baseSIDFreq /= 2.0
					}
					sidEnvFreq := (baseSIDFreq * sidClock) / 16777216.0
					toggleFreq := sidEnvFreq * ayRatio

					// Phase increment per sample
					phaseIncrement := toggleFreq / 44100.0
					tonePhase += phaseIncrement
					// Keep phase in 0.0-1.0 range
					if tonePhase >= 1.0 {
						tonePhase -= 1.0
					}
				}

				samples[frameStart+i] = sid.GenerateSample(channel)
			}
		} else {
			// Fast path: batch processing
			sid.GenerateSamplesBatch(channel, samples[frameStart:frameStart+numSamples])
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
		numSamples := samplesPerFrame
		if frameStart+numSamples > totalSamples {
			numSamples = totalSamples - frameStart
		}
		sid.GenerateSamplesBatch(0, samples[frameStart:frameStart+numSamples])
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

// analyzeConversionQuality compares AY and SID audio in frequency and time domains
func analyzeConversionQuality(ayAudio [3][]int16, sidAudio [3][]int16, notes [3][]Note, verbose bool) {
	sampleRate := 44100
	windowSize := sampleRate / 25 // 1/25 second = 40ms windows

	for ch := 0; ch < 3; ch++ {
		channelName := string('A' + ch)
		fmt.Printf("Channel %s:\n", channelName)

		// Analyze each note separately
		for noteIdx, note := range notes[ch] {
			startSample := note.StartFrame * sampleRate / 50
			endSample := note.EndFrame * sampleRate / 50

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
			ayWindow := ayAudio[ch][startSample:startSample+analysisWindow]
			sidWindow := sidAudio[ch][startSample:startSample+analysisWindow]

			spectralError := compareSpectra(ayWindow, sidWindow)
			rmsError := compareRMS(ayWindow, sidWindow)

			if spectralError > 0.3 || rmsError > 0.3 {
				timeSeconds := float64(note.StartFrame) / 50.0
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
				fmt.Printf("  WARNING: Note %d at %.2fs - spectral: %.3f, RMS: %.3f - %s\n",
					noteIdx+1, timeSeconds, spectralError, rmsError, waveformName)

				if verbose {
					// Show more details
					fmt.Printf("    Note: frames %d-%d, waveform=0x%02X, ADSR: A=%d D=%d S=%d R=%d\n",
						note.StartFrame, note.EndFrame, note.Waveform,
						note.ADSR.Attack, note.ADSR.Decay, note.ADSR.Sustain, note.ADSR.Release)
				}
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
					rmsErr := compareRMS(ayWindow, sidWindow)
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

// compareRMS computes RMS envelope difference between two audio windows
func compareRMS(ay []int16, sid []int16) float64 {
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
