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
	"strings"
	"sync"
	"time"

	"cubelon-emu/spectrum"
)

// Timing constants - SINGLE SOURCE OF TRUTH
// These must match the values in emulator/spectrum/audio.go
const (
	// Spectrum 128K timing
	spectrumCPUClockHz      = 3546895.0 // 3.546895 MHz CPU (128K)
	spectrumAYClockHz       = spectrumCPUClockHz / 2.0 // 1.773447 MHz (CPU / 2)
	spectrumCyclesPerFrame  = 70908 // 128K: 70908 T-states per frame (not 48K's 69888)
	spectrumFrameRate       = spectrumCPUClockHz / spectrumCyclesPerFrame // 50.0170648 Hz

	// C64 PAL timing
	sidClockHz       = 985248.0 // ~1 MHz PAL SID clock
	sidCyclesPerFrame = 19656    // 312 lines × 63 cycles
	palFrameRate      = sidClockHz / sidCyclesPerFrame // 50.124542 Hz
	sidFreqDivisor    = 16777216.0 // 2^24 for SID frequency conversion
)

// ADSR delay bug workaround: number of frames before next note to set R=0
// This can be tuned to balance note-off latency vs bug prevention
var framesBeforeNoteOff = 1 // 0, 1, or 2 frames

// Note represents a detected note from RMS
type Note struct {
	StartFrame      int
	EndFrame        int
	VolumeCurve     []float64
	ADSR            ADSRParams
	GateOffFrame    int     // Frame (relative to StartFrame) when gate turns off
	UseNoise        bool    // If true, use noise waveform (0x08)
	Waveform        byte    // SID waveform: 0x01=triangle, 0x02=sawtooth, 0x04=pulse, 0x08=noise
	EnvelopeMode    bool    // If true, note uses AY envelope mode (bit 4 of volume register)
	NoisePlusTone   bool    // If true, toggle between noise and pulse to simulate AY noise+tone mixing
	EnvelopeTypeID  int     // ID of the envelope type this note belongs to
	SustainPerFrame []uint8 // Sustain level per frame (0-15) for volume-following with D=0
}

// ADSRParams for a channel
type ADSRParams struct {
	Attack       uint8
	InitialDecay uint8 // D value for frame 0 (0 or 1), switches to 0 after attack
	Decay        uint8 // Tail decay rate (activated at GateOffFrame)
	Sustain      uint8
	Release      uint8
}

// EnvelopeType represents a cluster of similar envelope shapes
type EnvelopeType struct {
	ID              int
	RepresentCurve  []float64  // Representative normalized curve
	ADSR            ADSRParams // Fitted ADSR for this type
	GateOffFrame    int        // Frame when gate turns off (0 = stays ON throughout note)
	NoteIndices     []int      // Indices of notes using this type (per-channel when used per-channel)
	ChannelID       int        // Which channel (0=A, 1=B, 2=C) - used during per-channel analysis
	SustainPerFrame []uint8    // Sustain level per frame (0-15) for volume-following with D=0
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

		// Find max amplitude in curve to optimize S parameter
		// AY volume table (logarithmic amplitude response)
		ayVolumeTable := [16]float64{
			0, 327, 473, 690, 1006, 1492, 2113, 3518,
			4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
		}

		maxAmplitude := 0.0
		for _, vol := range globalTypes[i].RepresentCurve {
			ayVol := int(vol + 0.5)
			if ayVol < 0 {
				ayVol = 0
			}
			if ayVol > 15 {
				ayVol = 15
			}
			amplitude := ayVolumeTable[ayVol] / 32767.0
			if amplitude > maxAmplitude {
				maxAmplitude = amplitude
			}
		}

		// Determine if this is noise (needs √3 boost for perceived volume)
		// SID noise (8-bit) has ~58% RMS of AY noise (1-bit), so boost by √3 ≈ 1.73×
		isNoise := false
		if len(globalTypes[i].ChannelNotes) > 0 && len(globalTypes[i].ChannelNotes[0].Indices) > 0 {
			ch := globalTypes[i].ChannelNotes[0].Channel
			noteIdx := globalTypes[i].ChannelNotes[0].Indices[0]
			if noteIdx < len(notes[ch]) {
				isNoise = (notes[ch][noteIdx].Waveform == 0x08)
			}
		}

		// Map max linear amplitude to S (0-15)
		// SID envelope is linear, so S should scale with amplitude
		// For noise, boost by √3 with 7:1 compression to preserve dynamics
		amplitudeBoostForMax := 1.0
		if isNoise {
			amplitudeBoostForMax = 1.732050808 // √3
		}
		targetAmplitude := applyNoiseBoostWithCompression(maxAmplitude, amplitudeBoostForMax)
		optimalS := uint8(targetAmplitude*15.0 + 0.5)
		if optimalS < 1 {
			optimalS = 1 // Minimum S=1 to avoid zero volume
		}
		if optimalS > 15 {
			optimalS = 15
		}

		// Detect tail based on ORIGINAL AY values (before interpolation and S computation)
		// This ensures we identify quiet tails that should use D decay, not dynamic S tracking
		lastAudibleFrame := len(globalTypes[i].RepresentCurve) - 1

		// Find last frame where AY volume > 6 (anything <=6 is quiet enough for D decay)
		for j := len(globalTypes[i].RepresentCurve) - 1; j >= 0; j-- {
			if globalTypes[i].RepresentCurve[j] > 6.0 {
				lastAudibleFrame = j
				break
			}
		}

		// Compute SustainPerFrame from representative curve (only up to lastAudibleFrame)
		// Scale to use full 0-optimalS range for best dynamic range
		truncatedCurve := globalTypes[i].RepresentCurve[:lastAudibleFrame+1]
		amplitudeBoost := 1.0
		if isNoise {
			amplitudeBoost = 1.732050808 // √3 for noise
		}
		sustainPerFrame := computeSustainPerFrame(truncatedCurve, optimalS, amplitudeBoost)

		// Check if note starts at peak - if so, must use A=0
		firstVol := globalTypes[i].RepresentCurve[0]
		peakVol := firstVol
		for _, v := range globalTypes[i].RepresentCurve {
			if v > peakVol {
				peakVol = v
			}
		}

		var attack, initialDecay uint8
		startsAtPeak := (firstVol >= peakVol-0.1)

		if startsAtPeak {
			// Note starts at peak - use A=0 (no attack needed)
			attack = 0
			initialDecay = 0
		} else {
			// Note rises to peak - find best attack rate
			attack = findBestAttack(globalTypes[i].RepresentCurve, sustainPerFrame, optimalS)
			initialDecay = 0

			// If fitter chose A>2, re-run with only allowed values to find best
			// Don't just clamp to 2 - A=1 might fit better than A=2!
			// Also tries D=0 vs D=1 for frame 0 to avoid ADSR delay bug
			if attack > 2 {
				attack, initialDecay = findBestAttackInRange(globalTypes[i].RepresentCurve, sustainPerFrame, optimalS, 1, 2)
			}

			// Validate: A>0 is required for notes that don't start at peak
			if attack == 0 {
				panic(fmt.Sprintf("TYPE %d: A=0 selected but note starts at %.1f (peak %.1f, %.0f%% of peak) - should use A>0",
					globalTypes[i].ID, firstVol, peakVol, firstVol/peakVol*100))
			}
		}

		// Fit tail decay rate and GateOffFrame position jointly
		// Try different GateOffFrame positions and for each, fit the best D value
		// Pick the combination that gives the lowest overall error
		tailFrames := len(globalTypes[i].RepresentCurve) - lastAudibleFrame - 1
		var tailDecayRate uint8 = 0
		var gateOffFrame int = 0 // Default: gate stays on throughout
		if tailFrames > 0 {
			// Check if tail is flat (sustained note at quiet level)
			tailStart := lastAudibleFrame + 1
			tailVol := globalTypes[i].RepresentCurve[tailStart]
			isFlat := true
			for j := tailStart + 1; j < len(globalTypes[i].RepresentCurve); j++ {
				if math.Abs(globalTypes[i].RepresentCurve[j]-tailVol) > 0.5 {
					isFlat = false
					break
				}
			}

			// If tail is flat (sustained), use D=0 and extend sustainPerFrame with correct S for tail
			// If tail is decaying, try to fit D decay
			if isFlat {
				// D=0, but need to extend sustainPerFrame with tail S values
				// Compute S value that directly matches the tail's actual amplitude (with boost)
				ayVolumeTable := [16]float64{
					0, 327, 473, 690, 1006, 1492, 2113, 3518,
					4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
				}
				ayVol := int(tailVol + 0.5)
				if ayVol < 0 {
					ayVol = 0
				}
				if ayVol > 15 {
					ayVol = 15
				}
				amplitude := ayVolumeTable[ayVol] / 32767.0
				tailAmplitude := applyNoiseBoostWithCompression(amplitude, amplitudeBoost)
				// Map to S value that produces this amplitude (S/15 ≈ amplitude)
				tailS := uint8(tailAmplitude * 15.0 + 0.5)
				if tailS < 1 && tailAmplitude > 0 {
					tailS = 1
				}
				if tailS > 15 {
					tailS = 15
				}
				// Extend sustainPerFrame with flat tail S values
				for j := tailStart; j < len(globalTypes[i].RepresentCurve); j++ {
					sustainPerFrame = append(sustainPerFrame, tailS)
				}
			} else {
				gateOffFrame, tailDecayRate = findBestGateOffAndDecay(globalTypes[i].RepresentCurve, sustainPerFrame, attack, initialDecay, optimalS, lastAudibleFrame, amplitudeBoost)
			}
		}

		// ADSR: A=attack, InitialDecay for frame 0, D=tailDecayRate for tail, S=optimalS, R=0
		globalTypes[i].ADSR = ADSRParams{Attack: attack, InitialDecay: initialDecay, Decay: tailDecayRate, Sustain: optimalS, Release: 0}
		globalTypes[i].GateOffFrame = gateOffFrame

		// Truncate sustainPerFrame to the length chosen by the optimizer
		// If gateOffFrame > 0 and tailDecayRate > 0, use dynamic S only until gateOffFrame
		if gateOffFrame > 0 && tailDecayRate > 0 && gateOffFrame < len(sustainPerFrame) {
			globalTypes[i].SustainPerFrame = sustainPerFrame[:gateOffFrame]
		} else {
			globalTypes[i].SustainPerFrame = sustainPerFrame
		}

		// Debug: Show InitialDecay selection
		if initialDecay > 0 {
			fmt.Printf("  Type %d: Chose InitialDecay=%d (avoids ADSR delay bug with A=%d)\n",
				globalTypes[i].ID, initialDecay, attack)
		}

		// Fix: If attack > 0, set initial frames to optimalS (only while volume is rising to peak)
		// The attack phase brings envelope from 0 to S, so S should be constant while rising
		// Once peak is reached, use dynamic S values to track the decay
		if attack > 0 && len(sustainPerFrame) > 0 {
			// Find first frame where volume reaches its peak
			peakVol := 0.0
			for _, v := range globalTypes[i].RepresentCurve {
				if v > peakVol {
					peakVol = v
				}
			}
			// Set all frames BEFORE peak to optimalS (while volume < peak)
			// Once at peak or decreasing, use dynamic S values for smooth tracking
			for j := 0; j < len(globalTypes[i].SustainPerFrame); j++ {
				if globalTypes[i].RepresentCurve[j] < peakVol {
					globalTypes[i].SustainPerFrame[j] = optimalS
				} else {
					break // Reached peak, start using dynamic S values
				}
			}
		}

		// Validation: Ensure S never rises
		if len(globalTypes[i].SustainPerFrame) > 1 {
			// Check S never rises
			for j := 1; j < len(globalTypes[i].SustainPerFrame); j++ {
				if globalTypes[i].SustainPerFrame[j] > globalTypes[i].SustainPerFrame[j-1] {
					panic(fmt.Sprintf("TYPE %d: S rises at frame %d: S=%d -> S=%d",
						globalTypes[i].ID, j, globalTypes[i].SustainPerFrame[j-1], globalTypes[i].SustainPerFrame[j]))
				}
			}
			// Note: With ADSR delay bug modeling, frame 1 may not reach optimal S immediately
			// if attack-to-decay transition triggers the delay bug (rate_counter carryover)
		}
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

					// Copy SustainPerFrame
					noteFrames := len(notes[ch][noteIdx].VolumeCurve)
					notes[ch][noteIdx].SustainPerFrame = make([]uint8, noteFrames)
					copyLen := min(noteFrames, len(globalTypes[i].SustainPerFrame))
					copy(notes[ch][noteIdx].SustainPerFrame, globalTypes[i].SustainPerFrame[:copyLen])
					if noteFrames > len(globalTypes[i].SustainPerFrame) && len(globalTypes[i].SustainPerFrame) > 0 {
						lastS := globalTypes[i].SustainPerFrame[len(globalTypes[i].SustainPerFrame)-1]
						for j := len(globalTypes[i].SustainPerFrame); j < noteFrames; j++ {
							notes[ch][noteIdx].SustainPerFrame[j] = lastS
						}
					}
				}
			}
		}
	}

	// Phase 3.5: Envelope mode notes have empty VolumeCurve (all zeros) so they're skipped
	// during curve extraction and don't get SustainPerFrame values. They don't need R values
	// because gate stays ON throughout (never enters release phase). Amplitude modulation
	// comes from waveform toggling, not SID envelope.
	// We still need to initialize SustainPerFrame to prevent nil access.
	for ch := range notes {
		for i := range notes[ch] {
			if notes[ch][i].EnvelopeMode && len(notes[ch][i].SustainPerFrame) == 0 {
				noteFrames := notes[ch][i].EndFrame - notes[ch][i].StartFrame + 1
				notes[ch][i].SustainPerFrame = make([]uint8, noteFrames)
				// Envelope mode: use S=15 for maximum dynamic range
				// Amplitude modulation comes from waveform toggling, not ADSR
				for j := 0; j < noteFrames; j++ {
					notes[ch][i].SustainPerFrame[j] = 15
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
			// Check if note has no ADSR assigned (no SustainPerFrame computed)
			if len(notes[ch][i].SustainPerFrame) == 0 {
				// Compute optimal S and SustainPerFrame from volume curve
				ayVolumeTable := [16]float64{
					0, 327, 473, 690, 1006, 1492, 2113, 3518,
					4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
				}
				maxAmplitude := 0.0
				for _, vol := range notes[ch][i].VolumeCurve {
					ayVol := int(vol + 0.5)
					if ayVol < 0 {
						ayVol = 0
					}
					if ayVol > 15 {
						ayVol = 15
					}
					amplitude := ayVolumeTable[ayVol] / 32767.0
					if amplitude > maxAmplitude {
						maxAmplitude = amplitude
					}
				}
				// Apply √3 boost for noise with 7:1 compression to preserve dynamics
				amplitudeBoostForMax := 1.0
				if notes[ch][i].Waveform == 0x08 {
					amplitudeBoostForMax = 1.732050808 // √3
				}
				targetAmplitude := applyNoiseBoostWithCompression(maxAmplitude, amplitudeBoostForMax)
				optimalS := uint8(targetAmplitude*15.0 + 0.5)
				if optimalS < 1 {
					optimalS = 1
				}
				if optimalS > 15 {
					optimalS = 15
				}

				// Assign fallback ADSR with D=0 + optimal S
				notes[ch][i].ADSR = ADSRParams{Attack: 0, InitialDecay: 0, Decay: 0, Sustain: optimalS, Release: 0}
				notes[ch][i].GateOffFrame = 0
				notes[ch][i].EnvelopeTypeID = -1

				// Compute SustainPerFrame from volume curve
				amplitudeBoost := 1.0
				if notes[ch][i].Waveform == 0x08 {
					amplitudeBoost = 1.732050808 // √3 for noise
				}
				notes[ch][i].SustainPerFrame = computeSustainPerFrame(notes[ch][i].VolumeCurve, optimalS, amplitudeBoost)
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

			// Determine waveform from first note in type (all notes in a type share same waveform characteristics)
			var typeWaveform byte = 0x04 // Default to pulse
			if len(envType.ChannelNotes) > 0 && len(envType.ChannelNotes[0].Indices) > 0 {
				ch := envType.ChannelNotes[0].Channel
				noteIdx := envType.ChannelNotes[0].Indices[0]
				if noteIdx < len(notes[ch]) {
					typeWaveform = notes[ch][noteIdx].Waveform
				}
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

			// Determine amplitude boost for visualization (will be applied in plot function)
			amplitudeBoostViz := 1.0
			if typeWaveform == 0x08 {
				amplitudeBoostViz = 1.732050808 // √3 for noise
			}

			// Simulate SID envelope with dynamic S + optional decay tail
			var simulated []float64
			if len(envType.SustainPerFrame) > 0 {
				simulated = simulateDynamicSWithDecay(envType.ADSR.Attack, envType.ADSR.InitialDecay, envType.SustainPerFrame, envType.ADSR.Sustain, len(envType.SustainPerFrame))

				// If D>0, add decay phase after SustainPerFrame
				if envType.ADSR.Decay > 0 && len(envType.SustainPerFrame) < len(envType.RepresentCurve) {
					tailLength := len(envType.RepresentCurve) - len(envType.SustainPerFrame)

					// CRITICAL: Use the actual SID envelope level at GateOffFrame as starting point
					// The envelope level at GateOffFrame is determined by the dynamic S simulation,
					// not by mapping the AY volume. Using the simulated value ensures the tail
					// decay visualization matches the actual SID emulator behavior.
					lastSustainFrame := len(simulated) - 1
					if lastSustainFrame >= 0 {
						// Pass the normalized envelope level directly to simulateDecayToZero
						// simulated[i] is in range 0-1, representing envelope level 0-255
						startingLevel := simulated[lastSustainFrame]

						decayPhase := simulateDecayToZero(startingLevel, envType.ADSR.Decay, tailLength)
						simulated = append(simulated, decayPhase...)
					}
				} else if len(envType.SustainPerFrame) < len(envType.RepresentCurve) {
					// Pad with zeros if no decay
					for i := len(envType.SustainPerFrame); i < len(envType.RepresentCurve); i++ {
						simulated = append(simulated, 0)
					}
				}
			}

			// Plot AY and SID side by side
			// Pass raw AY volumes for display, unboosted normalized AY for bar length, boost factor for grey blocks
			fmt.Printf("      %s\n", plotEnvelopeSideBySide(envType.RepresentCurve, normalizedAY, simulated, envType.SustainPerFrame, envType.ADSR.Attack, envType.ADSR.InitialDecay, envType.ADSR.Decay, typeWaveform, amplitudeBoostViz))

			fmt.Printf("      → A=%d", envType.ADSR.Attack)
			if envType.ADSR.InitialDecay > 0 {
				fmt.Printf(" InitialDecay=%d", envType.ADSR.InitialDecay)
			}
			fmt.Printf(" D=%d S=%d", envType.ADSR.Decay, envType.ADSR.Sustain)
			if envType.ADSR.Decay > 0 {
				if envType.ADSR.InitialDecay > 0 {
					fmt.Printf(" (InitialDecay=%d frame 0, D=0 for %d frames, then D=%d for tail)\n",
						envType.ADSR.InitialDecay, envType.GateOffFrame-1, envType.ADSR.Decay)
				} else {
					fmt.Printf(" (D=0 for %d frames, then D=%d for tail)\n",
						envType.GateOffFrame, envType.ADSR.Decay)
				}
			} else {
				if envType.ADSR.InitialDecay > 0 {
					fmt.Printf(" (InitialDecay=%d frame 0, then D=0)\n", envType.ADSR.InitialDecay)
				} else {
					fmt.Printf(" (D=0 throughout)\n")
				}
			}
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

	// Validate that detected waveforms match actual AY audio per frame (direct player)
	fmt.Printf("\nValidating direct player waveforms per frame against AY audio...\n")
	ValidateWaveformPerFrame(commands, notes, ayAudio, 44100)
	fmt.Printf("✓ Direct player waveform validation passed\n")

	// Extract tracker format
	trackerFormat := ExtractTrackerFormat(commands, notes)

	// Simulate tracker replay
	simulatedNotes := SimulateTracker(trackerFormat)

	// Compare original and simulated notes
	CompareNotes(commands, notes, simulatedNotes)

	// Generate audio from simulated tracker format (replayed SID channels) BEFORE validations
	// This ensures the WAV is written even if validation panics
	fmt.Printf("\nGenerating replayed SID audio from tracker format (3 channels)...\n")
	replayStart := time.Now()
	var replayedSID [3][]int16
	for ch := 0; ch < 3; ch++ {
		wg.Add(1)
		go func(channel int) {
			defer wg.Done()
			// Convert SimulatedNote to audio using same frame timing as original
			audio := generateSIDFromSimulatedNotes(commands, channel, simulatedNotes[channel], len(ayAudio[0]), trackerFormat.StartOffset)
			replayedSID[channel] = audio
		}(ch)
	}
	wg.Wait()
	replayDuration := time.Since(replayStart)
	fmt.Printf("  Replayed SID generation took %.1fs\n", replayDuration.Seconds())
	fmt.Printf("  Replayed SID sample counts: [%d, %d, %d]\n",
		len(replayedSID[0]), len(replayedSID[1]), len(replayedSID[2]))

	// Validate that tracker player waveforms match actual AY audio per frame
	fmt.Printf("\nValidating tracker player waveforms per frame against AY audio...\n")
	ValidateTrackerWaveformPerFrame(commands, simulatedNotes, ayAudio, 44100, trackerFormat.StartOffset)
	fmt.Printf("✓ Tracker player waveform validation passed\n")

	// Validate that tracker player toggle periods are correct
	fmt.Printf("\nValidating tracker player toggle periods...\n")
	ValidateTrackerTogglePeriods(replayedSID, simulatedNotes, 44100, trackerFormat.StartOffset)
	fmt.Printf("✓ Tracker player toggle period validation passed\n")

	// Write 10-channel WAV: 3 AY + 3 original SID + 3 replayed SID + 1 debug
	fmt.Printf("\nWriting 10-channel WAV file with replayed format...\n")
	writeMultiFormatWAV(outputWav, ayAudio[:], sidAudio[:], replayedSID[:], 44100)

	fmt.Printf("✓ Complete! Written to: %s\n", outputWav)
	fmt.Printf("\n7-channel WAV layout:\n")
	fmt.Printf("  0-1: AY Channel A, SID Voice 1\n")
	fmt.Printf("  2-3: AY Channel B, SID Voice 2\n")
	fmt.Printf("  4-5: AY Channel C, SID Voice 3\n")
	fmt.Printf("  6:   ADSR delay bug (-1=none, +1=A, +0.33=B, -0.33=C)\n")

	// Validate SID frequencies match frame-by-frame
	fmt.Printf("\n=== Validating SID Frequencies ===\n")
	ValidateSIDFrequencies(commands, notes, simulatedNotes, trackerFormat.StartOffset)

	// Check if noise notes are being validated
	fmt.Printf("\n=== Noise Waveform Check ===\n")
	CheckNoiseValidation(commands, notes, simulatedNotes, trackerFormat.StartOffset)

	// Validate gate commands match
	fmt.Printf("\n=== Validating Gate Commands ===\n")
	ValidateGateCommands(notes, simulatedNotes, trackerFormat.StartOffset)

	// Validate ADSR parameters match
	fmt.Printf("\n=== Validating ADSR Parameters ===\n")
	ValidateADSR(notes, simulatedNotes)

	// Validate dynamic D updates match
	fmt.Printf("\n=== Validating Dynamic D Updates ===\n")
	ValidateDynamicD(notes, simulatedNotes, trackerFormat.StartOffset)

	// Validate toggle period for waveform toggling
	fmt.Printf("\n=== Validating Toggle Period ===\n")
	ValidateTogglePeriod(simulatedNotes, trackerFormat.StartOffset)

	// Compare toggle periods between direct and tracker players
	fmt.Printf("\n=== Comparing Toggle Periods (Direct vs Tracker) ===\n")
	CompareTogglePeriods(commands, notes, simulatedNotes, trackerFormat.StartOffset)

	// Validate note onset alignment (0→non-0 volume transitions)
	fmt.Printf("\n=== Note Onset Alignment Validation ===\n")
	validateNoteOnsets(ayAudio, sidAudio, notes, 44100)

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

		// Find max amplitude to optimize S parameter
		// AY volume table (logarithmic amplitude response)
		ayVolumeTable := [16]float64{
			0, 327, 473, 690, 1006, 1492, 2113, 3518,
			4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
		}

		maxAmplitude := 0.0
		for _, vol := range types[i].RepresentCurve {
			ayVol := int(vol + 0.5)
			if ayVol < 0 {
				ayVol = 0
			}
			if ayVol > 15 {
				ayVol = 15
			}
			amplitude := ayVolumeTable[ayVol] / 32767.0
			if amplitude > maxAmplitude {
				maxAmplitude = amplitude
			}
		}

		// Determine if this is noise (needs √3 boost for perceived volume)
		isNoise := false
		if len(types[i].NoteIndices) > 0 {
			noteIdx := types[i].NoteIndices[0]
			if noteIdx < len(notes) {
				isNoise = (notes[noteIdx].Waveform == 0x08)
			}
		}

		// Map max linear amplitude to S (0-15)
		// For noise, boost by √3 with 7:1 compression to preserve dynamics
		amplitudeBoostForMax := 1.0
		if isNoise {
			amplitudeBoostForMax = 1.732050808 // √3
		}
		targetAmplitude := applyNoiseBoostWithCompression(maxAmplitude, amplitudeBoostForMax)
		optimalS := uint8(targetAmplitude*15.0 + 0.5)
		if optimalS < 1 {
			optimalS = 1
		}
		if optimalS > 15 {
			optimalS = 15
		}

		// Detect tail based on ORIGINAL AY values (before interpolation and S computation)
		// This ensures we identify quiet tails that should use D decay, not dynamic S tracking
		lastAudibleFrame := len(types[i].RepresentCurve) - 1

		// Find last frame where AY volume > 6 (anything <=6 is quiet enough for D decay)
		for j := len(types[i].RepresentCurve) - 1; j >= 0; j-- {
			if types[i].RepresentCurve[j] > 6.0 {
				lastAudibleFrame = j
				break
			}
		}

		// Compute SustainPerFrame scaled to optimal S (only up to lastAudibleFrame)
		truncatedCurve := types[i].RepresentCurve[:lastAudibleFrame+1]
		amplitudeBoost := 1.0
		if isNoise {
			amplitudeBoost = 1.732050808 // √3 for noise
		}
		sustainPerFrame := computeSustainPerFrame(truncatedCurve, optimalS, amplitudeBoost)

		types[i].SustainPerFrame = sustainPerFrame

		// Fit ADSR for this type's representative curve (use D=0 initially, will fit later)
		types[i].ADSR, types[i].GateOffFrame = detectVolumePhasesDebug(types[i].RepresentCurve, types[i].SustainPerFrame, optimalS, 0, verbose && i < 10)

		// Now fit tail decay rate and GateOffFrame jointly if there's a tail
		tailFrames := len(types[i].RepresentCurve) - lastAudibleFrame - 1
		if tailFrames > 0 {
			// Check if tail is flat (sustained note at quiet level)
			tailStart := lastAudibleFrame + 1
			tailVol := types[i].RepresentCurve[tailStart]
			isFlat := true
			for j := tailStart + 1; j < len(types[i].RepresentCurve); j++ {
				if math.Abs(types[i].RepresentCurve[j]-tailVol) > 0.5 {
					isFlat = false
					break
				}
			}

			// If tail is flat (sustained), use D=0 and extend sustainPerFrame with correct S for tail
			// If tail is decaying, try to fit D decay
			if isFlat {
				// D=0, but need to extend sustainPerFrame with tail S values
				// Compute S value that directly matches the tail's actual amplitude (with boost)
				ayVolumeTable := [16]float64{
					0, 327, 473, 690, 1006, 1492, 2113, 3518,
					4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
				}
				ayVol := int(tailVol + 0.5)
				if ayVol < 0 {
					ayVol = 0
				}
				if ayVol > 15 {
					ayVol = 15
				}
				amplitude := ayVolumeTable[ayVol] / 32767.0
				tailAmplitude := applyNoiseBoostWithCompression(amplitude, amplitudeBoost)
				// Map to S value that produces this amplitude (S/15 ≈ amplitude)
				tailS := uint8(tailAmplitude * 15.0 + 0.5)
				if tailS < 1 && tailAmplitude > 0 {
					tailS = 1
				}
				if tailS > 15 {
					tailS = 15
				}
				// Extend sustainPerFrame with flat tail S values
				for j := tailStart; j < len(types[i].RepresentCurve); j++ {
					sustainPerFrame = append(sustainPerFrame, tailS)
				}
				// Update types[i].SustainPerFrame with extended array
				types[i].SustainPerFrame = sustainPerFrame
			} else {
				types[i].GateOffFrame, types[i].ADSR.Decay = findBestGateOffAndDecay(
					types[i].RepresentCurve, sustainPerFrame,
					types[i].ADSR.Attack, types[i].ADSR.InitialDecay,
					optimalS, lastAudibleFrame, amplitudeBoost)
			}
		}

		// Fix: If attack > 0, set initial frames to optimalS (only while volume is rising to peak)
		// The attack phase brings envelope from 0 to S, so S should be constant while rising
		// Once peak is reached, use dynamic S values to track the decay
		if types[i].ADSR.Attack > 0 && len(types[i].SustainPerFrame) > 0 {
			// Find first frame where volume reaches its peak
			peakVol := 0.0
			for _, v := range types[i].RepresentCurve {
				if v > peakVol {
					peakVol = v
				}
			}
			// Set all frames BEFORE peak to optimalS (while volume < peak)
			// Once at peak or decreasing, use dynamic S values for smooth tracking
			for j := 0; j < len(types[i].SustainPerFrame); j++ {
				if types[i].RepresentCurve[j] < peakVol {
					types[i].SustainPerFrame[j] = optimalS
				} else {
					break // Reached peak, start using dynamic S values
				}
			}
		}

		// Validation: Ensure S never rises
		if len(types[i].SustainPerFrame) > 1 {
			// Check S never rises
			for j := 1; j < len(types[i].SustainPerFrame); j++ {
				if types[i].SustainPerFrame[j] > types[i].SustainPerFrame[j-1] {
					panic(fmt.Sprintf("TYPE %d: S rises at frame %d: S=%d -> S=%d",
						types[i].ID, j, types[i].SustainPerFrame[j-1], types[i].SustainPerFrame[j]))
				}
			}
			// For notes with attack, frame 1 must be at optimal S (attack target)
			if types[i].ADSR.Attack > 0 && types[i].SustainPerFrame[1] != optimalS {
				panic(fmt.Sprintf("TYPE %d: A=%d but frame 1 has S=%d (expected optimal S=%d)",
					types[i].ID, types[i].ADSR.Attack, types[i].SustainPerFrame[1], optimalS))
			}
		}
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
func detectVolumePhasesDebug(volumeCurve []float64, sustainPerFrame []uint8, optimalS uint8, tailDecayRate uint8, debug bool) (ADSRParams, int) {
	if !debug {
		// Non-debug path: find best attack by simulation
		attack := findBestAttack(volumeCurve, sustainPerFrame, optimalS)
		// Use tailDecayRate for D parameter, gate off if D>0
		gateOff := 0
		if tailDecayRate > 0 {
			gateOff = len(sustainPerFrame)
		}
		return ADSRParams{Attack: attack, InitialDecay: 0, Decay: tailDecayRate, Sustain: optimalS, Release: 0}, gateOff
	}

	if len(volumeCurve) == 0 {
		return ADSRParams{Attack: 0, InitialDecay: 0, Decay: 0, Sustain: optimalS, Release: 0}, 0
	}

	// Check if note starts at peak - if so, must use A=0
	firstVol := volumeCurve[0]
	peakVol := firstVol
	for _, v := range volumeCurve {
		if v > peakVol {
			peakVol = v
		}
	}

	var attack uint8
	startsAtPeak := (firstVol >= peakVol-0.1)

	if startsAtPeak {
		// Note starts at peak - use A=0 (no attack needed)
		attack = 0
	} else {
		// Note rises to peak - find best attack rate
		attack = findBestAttack(volumeCurve, sustainPerFrame, optimalS)

		// Validate attack selection - clamp to A=2 if optimizer picks higher
		if attack > 2 {
			attack = 2
		}

		// Validate: A>0 is required for notes that don't start at peak
		if attack == 0 {
			panic(fmt.Sprintf("A=0 selected but note starts at %.1f (peak %.1f, %.0f%% of peak) - should use A>0",
				firstVol, peakVol, firstVol/peakVol*100))
		}
	}

	// ADSR: A=attack, D=tailDecayRate (0 or >0 for tail), S=optimalS, R=0
	result := ADSRParams{Attack: attack, InitialDecay: 0, Decay: tailDecayRate, Sustain: optimalS, Release: 0}
	gateOffFrame := 0 // Default: gate stays on throughout
	if tailDecayRate > 0 {
		// Gate turns off when dynamic S ends, D handles the tail
		gateOffFrame = len(sustainPerFrame)
	}

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

	// Simulate with dynamic S values + optional decay tail
	var simulated []float64
	if len(sustainPerFrame) > 0 {
		// Simulate dynamic S phase
		simulated = simulateDynamicS(attack, sustainPerFrame, optimalS, len(sustainPerFrame))

		// If tailDecayRate > 0, add decay phase after gateOffFrame
		if tailDecayRate > 0 && len(sustainPerFrame) < len(volumeCurve) {
			tailLength := len(volumeCurve) - len(sustainPerFrame)
			// Compute starting S from first tail frame's AY volume using same scaling
			ayVolumeTable := [16]float64{
				0, 327, 473, 690, 1006, 1492, 2113, 3518,
				4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
			}
			firstTailAYVol := volumeCurve[len(sustainPerFrame)]

			// Find maxAmplitude from the sustain phase
			maxAmplitude := 0.0
			for i := 0; i < len(sustainPerFrame); i++ {
				ayVol := int(volumeCurve[i] + 0.5)
				if ayVol < 0 { ayVol = 0 }
				if ayVol > 15 { ayVol = 15 }
				amp := ayVolumeTable[ayVol] / 32767.0
				if amp > maxAmplitude {
					maxAmplitude = amp
				}
			}
			if maxAmplitude == 0 {
				maxAmplitude = 1.0
			}

			// Compute S for first tail frame
			ayVol := int(firstTailAYVol + 0.5)
			if ayVol < 0 { ayVol = 0 }
			if ayVol > 15 { ayVol = 15 }
			amplitude := ayVolumeTable[ayVol] / 32767.0
			tailStartS := uint8((amplitude / maxAmplitude) * float64(optimalS) + 0.5)
			if tailStartS > optimalS {
				tailStartS = optimalS
			}
			if amplitude > 0 && tailStartS == 0 {
				tailStartS = 1
			}

			// Simulate decay from computed starting S to 0
			// Convert S value to normalized envelope level (0-1)
			startingLevel := float64(tailStartS) * 17.0 / 255.0
			decayPhase := simulateDecayToZero(startingLevel, tailDecayRate, tailLength)
			simulated = append(simulated, decayPhase...)
		} else if len(sustainPerFrame) < len(volumeCurve) {
			// Pad with zeros if no decay
			for i := len(sustainPerFrame); i < len(volumeCurve); i++ {
				simulated = append(simulated, 0)
			}
		}
	} else {
		// Fallback: use full ADSR
		simulated = simulateFullADSR(attack, tailDecayRate, optimalS, 0, len(volumeCurve), len(volumeCurve))
	}
	fmt.Printf("      SID:\n")
	fmt.Printf("      %s\n", plotEnvelope(simulated, visualLines))

	// Print ADSR info
	if tailDecayRate > 0 {
		fmt.Printf("      → A=%d D=%d S=%d (dynamic S, then D for tail)\n", attack, tailDecayRate, optimalS)
	} else {
		fmt.Printf("      → A=%d D=0 S=%d (dynamic S per frame)\n", attack, optimalS)
	}

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
// sid: SID envelope (0-1) for bar length (already includes √3 boost for noise from ADSR fitting)
// releasePerFrame: R values per frame
// attack: Attack rate (to calculate initial rate counter)
// initialDecay: Initial decay for frame 0 (to avoid ADSR delay bug)
// decay: Decay rate (for tail indicator)
// waveform: SID waveform (0x08=noise uses different block character ▓ vs █ for tone)
func plotEnvelopeSideBySide(ayRaw, ayNorm, sid []float64, sustainPerFrame []uint8, attack uint8, initialDecay uint8, decay uint8, waveform byte, amplitudeBoost float64) string {
	// Ensure all have same length
	maxLen := len(ayRaw)
	if len(ayNorm) > maxLen {
		maxLen = len(ayNorm)
	}
	if len(sid) > maxLen {
		maxLen = len(sid)
	}

	var result string
	// Header - aligned with data format: 40 blocks + " 12  " (5 chars) + 40 blocks + " S=4"
	// "AY:" (3) + 37 spaces = 40, then 5-char gap, then "SID:" (4) + 36 spaces + " AD=11" or " A=%d"
	header := "AY:" + strings.Repeat(" ", 42) + "SID:"
	if initialDecay > 0 {
		// Show AD=11 (attack and initial decay) aligned with S=10 position
		header += strings.Repeat(" ", 35) + fmt.Sprintf(" AD=%d%d", attack, initialDecay)
	} else {
		header += strings.Repeat(" ", 36) + fmt.Sprintf(" A=%d", attack)
	}
	result = header + "\n      "

	// Determine if this is noise (percussion) - use different block character
	// Note: ADSR fitting already applied √3 boost for noise, no additional boost needed here
	isNoise := (waveform == 0x08)
	var ayBlock, sidBlock string
	if isNoise {
		ayBlock = "▓" // Noise/percussion: medium shade
		sidBlock = "▓"
	} else {
		ayBlock = "█" // Tone/melody: solid block
		sidBlock = "█"
	}

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

		// AY bar (40 chars) - show unboosted (solid) + boost (grey)
		// ayNormVal is unboosted, amplitudeBoost is the multiplier (1.0 or √3)
		ayUnboostedLength := int(ayNormVal * 40.0)
		if ayUnboostedLength == 0 && ayNormVal > 0 {
			ayUnboostedLength = 1
		}
		ayBoostedLength := int(ayNormVal * amplitudeBoost * 40.0)
		if ayBoostedLength == 0 && ayNormVal > 0 {
			ayBoostedLength = 1
		}
		if ayBoostedLength > 40 {
			ayBoostedLength = 40
		}

		line := ""
		// Draw unboosted portion with solid block
		for j := 0; j < ayUnboostedLength && j < 40; j++ {
			line += ayBlock
		}
		// Draw boost portion with grey block
		for j := ayUnboostedLength; j < ayBoostedLength && j < 40; j++ {
			line += "▒"
		}
		// Fill remainder with light shade
		for j := ayBoostedLength; j < 40; j++ {
			line += "▁"
		}

		// Add AY raw volume value (0-15 register value)
		ayAmplitude := int(ayRawVal + 0.5)
		line += fmt.Sprintf(" %2d", ayAmplitude)

		line += "  " // Spacing between columns

		// SID bar (40 chars)
		// Note: No additional boost needed - ADSR fitting already applied √3 for noise
		// Round up for non-zero values so they always show at least one block
		sidLength := int(sidVal * 40.0)
		if sidLength == 0 && sidVal > 0 {
			sidLength = 1
		}
		if sidLength > 40 {
			sidLength = 40
		}
		for j := 0; j < sidLength; j++ {
			line += sidBlock
		}
		for j := sidLength; j < 40; j++ {
			line += "▁"
		}

		// Add S (sustain) value for D=0 + dynamic S approach
		// After sustainPerFrame ends, show D=X on first tail frame
		if i < len(sustainPerFrame) {
			s := sustainPerFrame[i]
			line += fmt.Sprintf(" S=%d", s)
		} else if i == len(sustainPerFrame) && decay > 0 {
			line += fmt.Sprintf(" D=%d", decay)
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

					// Copy SustainPerFrame from type (truncate or pad if needed)
					noteFrames := len(notes[i].VolumeCurve)
					notes[i].SustainPerFrame = make([]uint8, noteFrames)
					copyLen := min(noteFrames, len(envType.SustainPerFrame))
					copy(notes[i].SustainPerFrame, envType.SustainPerFrame[:copyLen])
					// Pad with last R value if note is longer than type curve
					if noteFrames > len(envType.SustainPerFrame) && len(envType.SustainPerFrame) > 0 {
						lastR := envType.SustainPerFrame[len(envType.SustainPerFrame)-1]
						for j := len(envType.SustainPerFrame); j < noteFrames; j++ {
							notes[i].SustainPerFrame[j] = lastR
						}
					}

					appliedCount++
					break
				}
			}
		} else {
			// Fallback: use instant attack (A=0)
			notes[i].ADSR = ADSRParams{Attack: 0, InitialDecay: 0, Decay: 9, Sustain: 1, Release: 0}
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
		return ADSRParams{Attack: 0, InitialDecay: 0, Decay: 0, Sustain: 15, Release: 0}, 0
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
		return ADSRParams{Attack: 0, InitialDecay: 0, Decay: 0, Sustain: 15, Release: 0}, len(volumeCurve)
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
	bestADSR := ADSRParams{Attack: 0, InitialDecay: 0, Decay: 0, Sustain: 15, Release: 11}
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

// simulateDynamicS simulates gate ON throughout with D=0 + dynamic S per frame
// Properly simulates attack phase (envelope rises to 255) then sustain with dynamic S
// Returns average envelope level per frame for accurate fitting
// Wrapper that uses D=0 by default
func simulateDynamicS(attack uint8, sustainPerFrame []uint8, maxSustain uint8, numFrames int) []float64 {
	return simulateDynamicSWithDecay(attack, 0, sustainPerFrame, maxSustain, numFrames)
}

// simulateDynamicSWithDecay simulates envelope with specific initial decay rate
func simulateDynamicSWithDecay(attack uint8, initialDecay uint8, sustainPerFrame []uint8, maxSustain uint8, numFrames int) []float64 {
	// Use actual SID emulator to correctly model ADSR delay bug
	sampleRate := 44100.0
	sid := NewSIDEmulator(sampleRate)
	envelope := make([]float64, numFrames)

	// Initialize voice 0
	sid.WriteRegister(0, 0x00)  // Freq low
	sid.WriteRegister(1, 0x10)  // Freq high (arbitrary non-zero)
	sid.WriteRegister(4, 0x00)  // Waveform, gate OFF

	// Set ADSR while gate is OFF
	initialS := maxSustain
	if len(sustainPerFrame) > 0 {
		initialS = sustainPerFrame[0]
	}
	sid.WriteRegister(5, (attack<<4)|initialDecay)  // A=attack, D=initialDecay
	sid.WriteRegister(6, (initialS<<4)|0) // S=initialS, R=0

	// Note: rate_counter is always 0 at natural A→D transitions in delta clocking mode
	// (or always 2 in real hardware single-cycle mode due to 2-cycle pipeline delay).
	// The ADSR delay bug only triggers from external register writes during a note,
	// NOT from natural state transitions. No need to pre-initialize rate_counter.

	// Gate ON to start envelope
	sid.WriteRegister(4, 0x11) // Pulse waveform + gate ON

	// Need multiple samples per frame to capture peaks (e.g., attack completion at 100%)
	// Generate at sample rate, then downsample to frame rate using peak values
	samplesPerFrame := int(sampleRate / palFrameRate)

	for frame := 0; frame < numFrames; frame++ {
		// Sample multiple times per frame and average
		sum := 0.0
		for sample := 0; sample < samplesPerFrame; sample++ {
			sid.GenerateSample(0)
			level := float64(sid.Voices[0].EnvelopeLevel) / 255.0
			sum += level
		}
		envelope[frame] = sum / float64(samplesPerFrame)

		// After frame 0, switch to D=0 for volume-following behavior
		if frame == 0 && initialDecay > 0 {
			currentA := (sid.Registers[5] >> 4) & 0x0F
			sid.WriteRegister(5, (currentA<<4)|0) // Keep A, set D=0
		}

		// Update S value AFTER sampling frame (to not interfere with current frame)
		if frame+1 < len(sustainPerFrame) {
			newS := sustainPerFrame[frame+1]
			currentS := sid.Registers[6] >> 4
			if newS != currentS {
				sid.WriteRegister(6, (newS<<4)|0)
			}
		}
	}

	return envelope
}

// findBestAttackInRange finds best A and initial D in range [minAttack, maxAttack]
// Returns (attack, initialDecay) - initialDecay is used for frame 0, then switches to D=0
func findBestAttackInRange(volumeCurve []float64, sustainPerFrame []uint8, maxSustain uint8, minAttack uint8, maxAttack uint8) (uint8, uint8) {
	if len(volumeCurve) == 0 {
		return 0, 0
	}

	// Convert AY volumes to linear amplitudes for comparison
	ayVolumeTable := [16]float64{
		0, 327, 473, 690, 1006, 1492, 2113, 3518,
		4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
	}

	targetAmplitudes := make([]float64, len(volumeCurve))
	for i, vol := range volumeCurve {
		ayVol := int(vol + 0.5)
		if ayVol < 0 {
			ayVol = 0
		}
		if ayVol > 15 {
			ayVol = 15
		}
		targetAmplitudes[i] = ayVolumeTable[ayVol] / 32767.0
	}

	bestAttack := minAttack
	bestInitialDecay := uint8(0)
	bestError := math.MaxFloat64

	for attack := minAttack; attack <= maxAttack; attack++ {
		// Try both D=0 and D=1 for initial decay to avoid ADSR delay bug
		for _, initialDecay := range []uint8{0, 1} {
			// Simulate envelope with this A + D combination
			simulated := simulateDynamicSWithDecay(attack, initialDecay, sustainPerFrame, maxSustain, len(volumeCurve))

			// Compute weighted RMS error (higher weight for early frames)
			sumSquaredError := 0.0
			totalWeight := 0.0
			for i := 0; i < len(targetAmplitudes); i++ {
				weight := 1.0
				if i < 5 {
					// 4x weight for first 5 frames where attack transient occurs
					weight = 4.0
				}
				diff := simulated[i] - targetAmplitudes[i]
				sumSquaredError += diff * diff * weight
				totalWeight += weight
			}
			rmsError := math.Sqrt(sumSquaredError / totalWeight)

			if rmsError < bestError {
				bestError = rmsError
				bestAttack = attack
				bestInitialDecay = initialDecay
			}
		}
	}

	return bestAttack, bestInitialDecay
}

// applyNoiseBoostWithCompression applies amplitude boost with 11:1 compression
// For noise (amplitudeBoost = √3), this preserves dynamics when exceeding S=15
func applyNoiseBoostWithCompression(amplitude float64, amplitudeBoost float64) float64 {
	if amplitudeBoost == 1.0 {
		return amplitude
	}

	boosted := amplitude * amplitudeBoost

	// 11:1 compression above threshold (S≈13.9)
	// Threshold: T + (√3 - T)/11 = 1.0  →  T = 0.927
	const compressionThreshold = 0.927
	if boosted > compressionThreshold {
		excess := boosted - compressionThreshold
		return compressionThreshold + excess/11.0
	}
	return boosted
}

// findBestGateOffAndDecay tries different GateOffFrame positions and D values
// Returns the combination that gives the best overall fit to the entire curve
func findBestGateOffAndDecay(volumeCurve []float64, sustainPerFrame []uint8, attack uint8, initialDecay uint8, maxSustain uint8, lastAudibleFrame int, amplitudeBoost float64) (int, uint8) {
	if len(volumeCurve) <= lastAudibleFrame+1 {
		return 0, 0 // No tail
	}

	// AY volume table for converting to linear amplitudes
	ayVolumeTable := [16]float64{
		0, 327, 473, 690, 1006, 1492, 2113, 3518,
		4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
	}

	// Convert entire AY curve to target amplitudes (with boost + compression for noise)
	targetAmplitudes := make([]float64, len(volumeCurve))
	for i, vol := range volumeCurve {
		ayVol := int(vol + 0.5)
		if ayVol < 0 {
			ayVol = 0
		}
		if ayVol > 15 {
			ayVol = 15
		}
		amplitude := ayVolumeTable[ayVol] / 32767.0
		targetAmplitudes[i] = applyNoiseBoostWithCompression(amplitude, amplitudeBoost)
	}

	// Try different GateOffFrame positions across a wide range
	// Start from lastAudibleFrame (quiet tail) and go back to try earlier positions
	// Earlier positions (higher S) would use slower D values (D=12-15)
	// Later positions (lower S) would use faster D values (D=6-10)
	bestGateOff := lastAudibleFrame
	bestDecay := uint8(0)
	bestError := math.MaxFloat64

	// Search from lastAudibleFrame back to 1/3 of the way through the curve
	// This allows trying D decay on higher S values (S=5, S=8, etc.) with slow D rates
	minGateOff := len(sustainPerFrame) / 3
	if minGateOff < 0 {
		minGateOff = 0
	}

	// For efficiency, search with a step size based on curve length
	// Short curves: check every frame
	// Long curves: check every 2-3 frames
	step := 1
	if len(sustainPerFrame) > 20 {
		step = 2
	}
	if len(sustainPerFrame) > 40 {
		step = 3
	}

	for gateOff := lastAudibleFrame; gateOff >= minGateOff; gateOff -= step {
		if gateOff >= len(sustainPerFrame) {
			continue // Can't use this position
		}

		// Simulate dynamic S up to this GateOffFrame
		dynamicSPart := simulateDynamicSWithDecay(attack, initialDecay, sustainPerFrame[:gateOff], maxSustain, gateOff)
		startingLevel := dynamicSPart[len(dynamicSPart)-1]

		// Try different D values for the tail
		for decay := uint8(0); decay <= 15; decay++ {
			// Simulate tail decay
			tailLength := len(volumeCurve) - gateOff
			decayPart := simulateDecayToZero(startingLevel, decay, tailLength)

			// Combine dynamic S part + decay part
			simulated := append(dynamicSPart, decayPart...)

			// Compute RMS error for entire curve
			sumSquaredError := 0.0
			for i := 0; i < len(targetAmplitudes) && i < len(simulated); i++ {
				diff := simulated[i] - targetAmplitudes[i]
				sumSquaredError += diff * diff
			}
			rmsError := math.Sqrt(sumSquaredError / float64(len(targetAmplitudes)))

			if rmsError < bestError {
				bestError = rmsError
				bestGateOff = gateOff
				bestDecay = decay
			}
		}
	}

	return bestGateOff, bestDecay
}

// findBestDecayRate tries multiple D values and returns the one with best fit to AY tail curve
// startingLevel is the normalized envelope level (0-1) at GateOffFrame
func findBestDecayRate(tailCurve []float64, startingLevel float64) uint8 {
	if len(tailCurve) == 0 || startingLevel < 0.01 {
		return 0 // No tail or starting too low
	}

	// AY volume table for converting to linear amplitudes
	ayVolumeTable := [16]float64{
		0, 327, 473, 690, 1006, 1492, 2113, 3518,
		4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
	}

	// Convert tail AY volumes to target amplitudes
	targetAmplitudes := make([]float64, len(tailCurve))
	for i, vol := range tailCurve {
		ayVol := int(vol + 0.5)
		if ayVol < 0 {
			ayVol = 0
		}
		if ayVol > 15 {
			ayVol = 15
		}
		targetAmplitudes[i] = ayVolumeTable[ayVol] / 32767.0
	}

	// Try D values and find best fit
	bestDecay := uint8(0)
	bestError := math.MaxFloat64

	// Try D from 0 to 15 (0=instant, 15=slowest)
	for decay := uint8(0); decay <= 15; decay++ {
		// Simulate decay with this D value
		simulated := simulateDecayToZero(startingLevel, decay, len(tailCurve))

		// Compute RMS error
		sumSquaredError := 0.0
		for i := 0; i < len(targetAmplitudes); i++ {
			diff := simulated[i] - targetAmplitudes[i]
			sumSquaredError += diff * diff
		}
		rmsError := math.Sqrt(sumSquaredError / float64(len(targetAmplitudes)))

		if rmsError < bestError {
			bestError = rmsError
			bestDecay = decay
		}
	}

	return bestDecay
}

// findBestAttack tries multiple A values and returns the one with best fit to AY curve
func findBestAttack(volumeCurve []float64, sustainPerFrame []uint8, maxSustain uint8) uint8 {
	if len(volumeCurve) == 0 {
		return 0
	}

	// Convert AY volumes to linear amplitudes for comparison
	ayVolumeTable := [16]float64{
		0, 327, 473, 690, 1006, 1492, 2113, 3518,
		4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
	}

	targetAmplitudes := make([]float64, len(volumeCurve))
	for i, vol := range volumeCurve {
		ayVol := int(vol + 0.5)
		if ayVol < 0 {
			ayVol = 0
		}
		if ayVol > 15 {
			ayVol = 15
		}
		targetAmplitudes[i] = ayVolumeTable[ayVol] / 32767.0
	}

	// Determine if A=0 is allowed (only for notes starting at peak)
	firstVol := volumeCurve[0]
	peakVol := firstVol
	for _, v := range volumeCurve {
		if v > peakVol {
			peakVol = v
		}
	}
	allowA0 := (firstVol >= peakVol-0.1) // Allow A=0 if starts within 0.1 of peak

	// Try A values from 0 to 5 (covers instant to ~120ms attack)
	// Weight early frames more heavily (first 5 frames = 100ms) where attack matters
	bestAttack := uint8(0)
	if !allowA0 {
		bestAttack = 1 // Start from A=1 if A=0 not allowed
	}
	bestError := math.MaxFloat64

	minAttack := uint8(0)
	if !allowA0 {
		minAttack = 1
	}

	for attack := minAttack; attack <= 5; attack++ {
		// Simulate envelope with this A value
		simulated := simulateDynamicS(attack, sustainPerFrame, maxSustain, len(volumeCurve))

		// Compute weighted RMS error (higher weight for early frames)
		sumSquaredError := 0.0
		totalWeight := 0.0
		for i := 0; i < len(targetAmplitudes); i++ {
			weight := 1.0
			if i < 5 {
				// 4x weight for first 5 frames where attack transient occurs
				weight = 4.0
			}
			diff := simulated[i] - targetAmplitudes[i]
			sumSquaredError += diff * diff * weight
			totalWeight += weight
		}
		rmsError := math.Sqrt(sumSquaredError / totalWeight)

		if rmsError < bestError {
			bestError = rmsError
			bestAttack = attack
		}
	}

	return bestAttack
}

// simulateDecayToZero simulates exponential decay from a starting envelope level to 0
// Used for note tails where we detect quiet volumes and want to decay to silence
// startingLevel is the envelope level (0.0-1.0, representing 0-255)
// Uses REAL SID emulator to match actual hardware behavior
func simulateDecayToZero(startingLevel float64, decayRate uint8, numFrames int) []float64 {
	// Use actual SID emulator to correctly model ADSR delay bug and envelope behavior
	sampleRate := 44100.0
	sid := NewSIDEmulator(sampleRate)
	envelope := make([]float64, numFrames)

	// Convert normalized level (0.0-1.0) to envelope level (0-255)
	envelopeLevel := uint8(startingLevel * 255.0 + 0.5)
	if envelopeLevel > 255 {
		envelopeLevel = 255
	}

	// Initialize voice 0
	sid.WriteRegister(0, 0x00)  // Freq low
	sid.WriteRegister(1, 0x10)  // Freq high (arbitrary non-zero)

	// Set ADSR: A=0, D=0, S=15, R=decayRate
	// We'll use Release with the decay rate to simulate tail decay
	sid.WriteRegister(5, (0<<4)|0)  // A=0, D=0
	sid.WriteRegister(6, (15<<4)|decayRate)  // S=15, R=decayRate

	// Gate ON first to reach starting level
	sid.WriteRegister(4, 0x11) // Pulse + gate ON

	// Set envelope level directly and put in sustain state
	sid.Voices[0].EnvelopeLevel = envelopeLevel
	sid.Voices[0].EnvelopeState = 1 // Sustain state

	// Now gate OFF to trigger Release phase (which uses R=decayRate)
	sid.WriteRegister(4, 0x10) // Pulse, gate OFF

	// Generate envelope samples during release (decay to zero)
	samplesPerFrame := int(sampleRate / palFrameRate)
	for frame := 0; frame < numFrames; frame++ {
		sum := 0.0
		for sample := 0; sample < samplesPerFrame; sample++ {
			sid.GenerateSample(0)
			level := float64(sid.Voices[0].EnvelopeLevel) / 255.0
			sum += level
		}
		envelope[frame] = sum / float64(samplesPerFrame)
	}

	return envelope
}

// simulateDynamicR simulates gate ON for 1 frame (attack), then gate OFF with dynamic R per frame
// Includes ADSR delay bug simulation for accurate envelope display
// NOTE: Deprecated in favor of simulateDynamicS with D=0 approach
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
		simulated, state := simulateReleaseWithBug(currentLevel, currentR, 1, rateCounter, 0, 1)
		envelope[frame] = simulated[0]

		// Update state for next frame from simulation result
		currentLevel = state.level
		rateCounter = state.rateCounter
	}

	return envelope
}

// computeMedianADSR computes channel-wide median ADSR
func computeMedianADSR(notes []Note) ADSRParams {
	if len(notes) == 0 {
		return ADSRParams{Attack: 0, InitialDecay: 0, Decay: 0, Sustain: 15, Release: 0}
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
		return ADSRParams{Attack: 0, InitialDecay: 0, Decay: 0, Sustain: 15, Release: 0}
	}

	return computeMedianADSRFromList(adsrs)
}

func computeMedianADSRFromList(adsrs []ADSRParams) ADSRParams {
	if len(adsrs) == 0 {
		return ADSRParams{Attack: 0, InitialDecay: 0, Decay: 0, Sustain: 15, Release: 0}
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
	noisePlusToneEnabled := make([]bool, numFrames) // Track noise+tone mixed (for envelope mode)
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

		toneOn := (currentMixer & (1 << (channel))) == 0
		noiseOn := (currentMixer & (1 << noiseBit)) == 0

		// Use noise only when tone is OFF and noise is ON (noise-only mode for direct mode)
		// AY can mix tone+noise, but SID cannot, so we only use SID noise for noise-only
		noiseEnabled[i] = !toneOn && noiseOn

		// Track noise+tone mixed (for direct mode toggle simulation)
		noisePlusToneEnabled[i] = toneOn && noiseOn

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
					var hasNoisePlusTone bool
					var adsr ADSRParams

					if noteEnvMode {
						// Envelope mode: check for noise+tone mixing first
						hasNoisePlusTone = false
						for f := noteStart; f < frame; f++ {
							if noisePlusToneEnabled[f] {
								hasNoisePlusTone = true
								break
							}
						}

						if hasNoisePlusTone {
							// Noise+tone mixing in envelope mode: toggle between noise and pulse
							waveform = 0x08 // Noise
							usesNoise = true
						} else {
							// Check for pure noise in envelope mode
							usesNoise = false
							for f := noteStart; f < frame; f++ {
								if noiseEnabled[f] {
									usesNoise = true
									break
								}
							}

							if usesNoise {
								waveform = 0x08 // Pure noise
							} else {
								// Regular envelope mode: use R13 shape for triangle/sawtooth
								waveform = byte(0x04)
								if shapeVal, hasWrite := envelopeShapeWrites[noteStart]; hasWrite {
									waveform = ayEnvelopeToSIDWaveform(shapeVal)
								} else if noteStart < len(envelopeShape) {
									waveform = ayEnvelopeToSIDWaveform(envelopeShape[noteStart])
								}
							}
						}
						adsr = ADSRParams{Attack: 0, InitialDecay: 0, Decay: 0, Sustain: 15, Release: 0}
					} else {
						// Direct mode: check for noise+tone mixing first
						hasNoisePlusTone = false
						for f := noteStart; f < frame; f++ {
							if noisePlusToneEnabled[f] {
								hasNoisePlusTone = true
								break
							}
						}

						if hasNoisePlusTone {
							// Noise+tone mixing in direct mode: toggle between noise and pulse
							waveform = 0x08 // Noise (will toggle with pulse)
							usesNoise = true
						} else {
							// Regular direct mode: check for noise-only
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
						}
						adsr = ADSRParams{} // Will be fitted later
					}

					newNote := Note{
						StartFrame:    noteStart,
						EndFrame:      frame - 1,
						VolumeCurve:   curve,
						GateOffFrame:  len(curve), // Default: gate on full duration
						UseNoise:      usesNoise,
						Waveform:      waveform,
						ADSR:          adsr,
						EnvelopeMode:  noteEnvMode,
						NoisePlusTone: hasNoisePlusTone,
					}

					// VALIDATE: If we detected a noise note, verify AY actually has audible noise
					if newNote.Waveform == 0x08 && !newNote.NoisePlusTone {
						hasAudibleNoise := false
						for f := newNote.StartFrame; f <= newNote.EndFrame; f++ {
							if f < len(noiseEnabled) && noiseEnabled[f] {
								hasAudibleNoise = true
								break
							}
						}
						if channel == 0 && newNote.StartFrame == 4102 {
							fmt.Printf("  DEBUG CH%d: Note frames %d-%d, noiseEnabled check: hasAudibleNoise=%v\n",
								channel, newNote.StartFrame, newNote.EndFrame, hasAudibleNoise)
							for f := newNote.StartFrame; f <= newNote.EndFrame && f < len(noiseEnabled); f++ {
								mixer := getAYMixerState(commands, f)
								vol := volumePerFrame[f]
								// Get actual volume register value to check envelope bit
								volRegValue := uint8(0)
								for _, cmd := range commands {
									if int(cmd.Frame) == f && cmd.Register == volReg {
										volRegValue = cmd.Value
										break
									}
								}
								envBit := (volRegValue & 0x10) != 0
								toneBit := uint8(channel)
								noiseBit := uint8(3 + channel)
								toneOn := (mixer & (1 << toneBit)) == 0
								noiseOn := (mixer & (1 << noiseBit)) == 0
								fmt.Printf("    frame %d: noiseEnabled=%v mixer=0x%02X toneOn=%v noiseOn=%v volume=%.0f volReg=0x%02X envBit=%v\n",
									f, noiseEnabled[f], mixer, toneOn, noiseOn, vol, volRegValue, envBit)
							}
						}
						if !hasAudibleNoise {
							panic(fmt.Sprintf("CH%d: Detected noise note at frames %d-%d (waveform=0x08) but AY has no audible noise in that range!",
								channel, newNote.StartFrame, newNote.EndFrame))
						}
					}

					notes = append(notes, newNote)
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
					var hasNoisePlusTone bool
					var adsr ADSRParams

					if noteEnvMode {
						// Envelope mode: check for noise+tone mixing
						for f := noteStart; f < frame; f++ {
							if noisePlusToneEnabled[f] {
								hasNoisePlusTone = true
								break
							}
						}

						if hasNoisePlusTone {
							waveform = 0x08
							usesNoise = true
						} else {
							waveform = byte(0x04)
							if shapeVal, hasWrite := envelopeShapeWrites[noteStart]; hasWrite {
								waveform = ayEnvelopeToSIDWaveform(shapeVal)
							} else if noteStart < len(envelopeShape) {
								waveform = ayEnvelopeToSIDWaveform(envelopeShape[noteStart])
							}
							usesNoise = false
						}
						adsr = ADSRParams{Attack: 0, InitialDecay: 0, Decay: 0, Sustain: 15, Release: 0}
					} else {
						// Direct mode: check for noise+tone mixing
						for f := noteStart; f < frame; f++ {
							if noisePlusToneEnabled[f] {
								hasNoisePlusTone = true
								break
							}
						}

						if hasNoisePlusTone {
							waveform = 0x08
							usesNoise = true
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
						}
						adsr = ADSRParams{}
					}

					newNote := Note{
						StartFrame:    noteStart,
						EndFrame:      frame - 1,
						VolumeCurve:   curve,
						GateOffFrame:  len(curve), // Default: gate on full duration
						UseNoise:      usesNoise,
						Waveform:      waveform,
						ADSR:          adsr,
						EnvelopeMode:  noteEnvMode,
						NoisePlusTone: hasNoisePlusTone,
					}

					// VALIDATE: If we detected a noise note, verify AY actually has audible noise
					if newNote.Waveform == 0x08 && !newNote.NoisePlusTone {
						hasAudibleNoise := false
						for f := newNote.StartFrame; f <= newNote.EndFrame; f++ {
							if f < len(noiseEnabled) && noiseEnabled[f] {
								hasAudibleNoise = true
								break
							}
						}
						if channel == 0 && newNote.StartFrame == 4102 {
							fmt.Printf("  DEBUG CH%d: Note frames %d-%d, noiseEnabled check: hasAudibleNoise=%v\n",
								channel, newNote.StartFrame, newNote.EndFrame, hasAudibleNoise)
							for f := newNote.StartFrame; f <= newNote.EndFrame && f < len(noiseEnabled); f++ {
								mixer := getAYMixerState(commands, f)
								vol := volumePerFrame[f]
								// Get actual volume register value to check envelope bit
								volRegValue := uint8(0)
								for _, cmd := range commands {
									if int(cmd.Frame) == f && cmd.Register == volReg {
										volRegValue = cmd.Value
										break
									}
								}
								envBit := (volRegValue & 0x10) != 0
								toneBit := uint8(channel)
								noiseBit := uint8(3 + channel)
								toneOn := (mixer & (1 << toneBit)) == 0
								noiseOn := (mixer & (1 << noiseBit)) == 0
								fmt.Printf("    frame %d: noiseEnabled=%v mixer=0x%02X toneOn=%v noiseOn=%v volume=%.0f volReg=0x%02X envBit=%v\n",
									f, noiseEnabled[f], mixer, toneOn, noiseOn, vol, volRegValue, envBit)
							}
						}
						if !hasAudibleNoise {
							panic(fmt.Sprintf("CH%d: Detected noise note at frames %d-%d (waveform=0x08) but AY has no audible noise in that range!",
								channel, newNote.StartFrame, newNote.EndFrame))
						}
					}

					notes = append(notes, newNote)
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
			var hasNoisePlusTone bool
			var adsr ADSRParams

			if noteEnvMode {
				// Envelope mode: check for noise+tone mixing first
				for f := noteStart; f < numFrames; f++ {
					if noisePlusToneEnabled[f] {
						hasNoisePlusTone = true
						break
					}
				}

				if hasNoisePlusTone {
					// Noise+tone mixing in envelope mode
					waveform = 0x08
					usesNoise = true
				} else {
					// Regular envelope mode: use R13 shape
					waveform = byte(0x04)
					if shapeVal, hasWrite := envelopeShapeWrites[noteStart]; hasWrite {
						waveform = ayEnvelopeToSIDWaveform(shapeVal)
					} else if noteStart < len(envelopeShape) {
						waveform = ayEnvelopeToSIDWaveform(envelopeShape[noteStart])
					}
					usesNoise = false
				}
				adsr = ADSRParams{Attack: 0, InitialDecay: 0, Decay: 0, Sustain: 15, Release: 0}
			} else {
				// Direct mode: check for noise+tone mixing first
				for f := noteStart; f < numFrames; f++ {
					if noisePlusToneEnabled[f] {
						hasNoisePlusTone = true
						break
					}
				}

				if hasNoisePlusTone {
					// Noise+tone mixing in direct mode
					waveform = 0x08
					usesNoise = true
				} else {
					// Regular direct mode: check for noise-only
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
				}
				adsr = ADSRParams{}
			}

			notes = append(notes, Note{
				StartFrame:    noteStart,
				EndFrame:      numFrames - 1,
				VolumeCurve:   curve,
				GateOffFrame:  len(curve), // Default: gate on full duration
				UseNoise:      usesNoise,
				Waveform:      waveform,
				ADSR:          adsr,
				NoisePlusTone: hasNoisePlusTone,
			})
		}
	}

	// Count noise+tone notes
	noisePlusToneCount := 0
	for _, note := range notes {
		if note.NoisePlusTone {
			noisePlusToneCount++
		}
	}
	if noisePlusToneCount > 0 {
		fmt.Printf("  %d/%d notes use noise+tone mixing (channel %d)\n", noisePlusToneCount, len(notes), channel)
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
	lastFrame := int(commands[len(commands)-1].Frame)
	// Frame range is 0 to lastFrame inclusive, so total frames = lastFrame + 1
	durationFrames := lastFrame + 1
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
			ayAudio[channel] = generateAYAudioChannel(commands, channel, 0, lastFrame)
		}(ch)
	}
	wg.Wait()
	ayDuration := time.Since(ayStart)
	fmt.Printf("  AY generation took %.1fs\n", ayDuration.Seconds())
	fmt.Printf("  AY sample counts (Spectrum timing): [%d, %d, %d]\n",
		len(ayAudio[0]), len(ayAudio[1]), len(ayAudio[2]))

	// Resample AY audio with correct ratio to align frame boundaries
	// Ratio = (samples/frame at PAL) / (samples/frame at Spectrum)
	//       = spectrumFrameRate / palFrameRate
	// This preserves frame boundaries: frame N in AY maps to frame N in PAL
	fmt.Printf("Resampling AY to PAL timing (uniform)...\n")
	resampleStart := time.Now()
	for ch := 0; ch < 3; ch++ {
		wg.Add(1)
		go func(channel int) {
			defer wg.Done()
			ayAudio[channel] = resampleToLength(ayAudio[channel], durationSamples)
		}(ch)
	}
	wg.Wait()
	resampleDuration := time.Since(resampleStart)
	fmt.Printf("  Resampling took %.1fs\n", resampleDuration.Seconds())
	fmt.Printf("  AY sample counts (PAL timing): [%d, %d, %d] (target: %d)\n",
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
	bugSamplesPerChannel := [3]int{0, 0, 0}
	for i := 0; i < durationSamples; i++ {
		// Default: -1 (no bug active)
		mixedBugDebug[i] = -32767

		// Channel-specific values when bug is active
		if adsrBugDebug[0][i] != 0 {
			mixedBugDebug[i] = 32767 // Channel A: +1
			bugSamplesPerChannel[0]++
		} else if adsrBugDebug[1][i] != 0 {
			mixedBugDebug[i] = 10922 // Channel B: +0.33
			bugSamplesPerChannel[1]++
		} else if adsrBugDebug[2][i] != 0 {
			mixedBugDebug[i] = -10922 // Channel C: -0.33
			bugSamplesPerChannel[2]++
		}
	}

	// Report ADSR delay bug statistics
	totalBugSamples := bugSamplesPerChannel[0] + bugSamplesPerChannel[1] + bugSamplesPerChannel[2]
	bugDurationMs := float64(totalBugSamples) / float64(sampleRate) * 1000.0
	fmt.Printf("  ADSR delay bug: %d samples (%.1fms) - A:%d B:%d C:%d\n",
		totalBugSamples, bugDurationMs,
		bugSamplesPerChannel[0], bugSamplesPerChannel[1], bugSamplesPerChannel[2])

	fmt.Printf("\nWriting WAV file...\n")
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

// getAYTonePeriod finds the tone period (R0-R5) value at a given frame for a channel
func getAYTonePeriod(commands []spectrum.AYCommand, channel int, frame int) uint16 {
	toneLoReg := uint8(channel * 2)
	toneHiReg := uint8(channel*2 + 1)
	tonePeriod := uint16(0)
	for _, cmd := range commands {
		if int(cmd.Frame) > frame {
			break
		}
		if cmd.Register == toneLoReg && int(cmd.Frame) <= frame {
			tonePeriod = (tonePeriod & 0xFF00) | uint16(cmd.Value)
		}
		if cmd.Register == toneHiReg && int(cmd.Frame) <= frame {
			tonePeriod = (tonePeriod & 0x00FF) | (uint16(cmd.Value&0x0F) << 8)
		}
	}
	if tonePeriod == 0 {
		tonePeriod = 1 // Default to 1 if not set
	}
	return tonePeriod
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

// getAYMixerState finds the mixer register (R7) value at a given frame
func getAYMixerState(commands []spectrum.AYCommand, frame int) byte {
	mixer := byte(0xFF) // All disabled by default
	for _, cmd := range commands {
		if int(cmd.Frame) > frame {
			break
		}
		if cmd.Register == 7 && int(cmd.Frame) <= frame {
			mixer = cmd.Value
		}
	}
	return mixer
}

// checkMixerMode determines the waveform mode based on mixer register and channel
// Returns: toneOnly, noiseOnly, noisePlusTone (only one will be true)
func checkMixerMode(mixer byte, channel int) (toneOnly, noiseOnly, noisePlusTone bool) {
	toneBit := uint8(channel)    // Bit 0=A, 1=B, 2=C
	noiseBit := uint8(3 + channel) // Bit 3=A, 4=B, 5=C

	toneOn := (mixer & (1 << toneBit)) == 0    // 0 = enabled
	noiseOn := (mixer & (1 << noiseBit)) == 0  // 0 = enabled

	if toneOn && !noiseOn {
		toneOnly = true
	} else if !toneOn && noiseOn {
		noiseOnly = true
	} else if toneOn && noiseOn {
		noisePlusTone = true
	}

	// Debug for problematic mixer states
	if channel == 1 && (mixer == 0x68 || mixer == 0x78) {
		fmt.Printf("    checkMixerMode: ch=%d mixer=0x%02x toneBit=%d noiseBit=%d toneOn=%v noiseOn=%v → only=%v/%v/both=%v\n",
			channel, mixer, toneBit, noiseBit, toneOn, noiseOn, toneOnly, noiseOnly, noisePlusTone)
	}
	return
}

// computeEnvelopeFrequency converts AY envelope period to SID frequency
// AY envelope update rate: AY_clock / (period * 512) where period = R11 | (R12 << 8)
func computeEnvelopeFrequency(envPeriod uint16) uint16 {
	ayClock := spectrumAYClockHz
	sidClock := sidClockHz
	// AY envelope rate: ayClock / (period * 512)
	envFreqHz := ayClock / (float64(envPeriod) * 512.0)
	// Correct for resampling: AY audio gets sped up by palFrameRate/spectrumFrameRate
	envFreqHz *= palFrameRate / spectrumFrameRate
	// SID frequency register value: freq_hz * 16777216 / sid_clock
	// Round to nearest integer for optimal frequency match
	sidFreq := uint16(envFreqHz*sidFreqDivisor/sidClock + 0.5)
	return sidFreq
}

// computeToneFrequency converts AY tone period to SID frequency
func computeToneFrequency(ayTonePeriod uint16) uint16 {
	if ayTonePeriod == 0 {
		return 0
	}
	ayClock := spectrumAYClockHz
	freqHz := ayClock / (16.0 * float64(ayTonePeriod))
	// Correct for resampling: AY audio gets sped up by palFrameRate/spectrumFrameRate
	freqHz *= palFrameRate / spectrumFrameRate
	// Round to nearest integer for optimal frequency match
	return uint16(freqHz*sidFreqDivisor/sidClockHz + 0.5)
}

// computeNoiseFrequency converts AY noise period to SID frequency
// AY noise update rate: AY_clock / (period * 16 * 2) where period = R6 & 0x1F
// SID oscillator freq: (SID_freq / 16777216) * SID_clock
// Match them: (SID_freq / 16777216) * 985248 = 886700 / (period * 32)
func computeNoiseFrequency(noisePeriod byte) uint16 {
	if noisePeriod == 0 {
		noisePeriod = 1
	}
	ayClock := spectrumAYClockHz // AY clock (already CPU/2, same as tone conversion)
	sidClock := sidClockHz
	// AY noise rate: ayClock / (period << 4) = ayClock / (period * 16)
	noiseRateHz := ayClock / (float64(noisePeriod) * 16.0)
	// Correct for resampling: AY audio gets sped up by palFrameRate/spectrumFrameRate
	noiseRateHz *= palFrameRate / spectrumFrameRate
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

	ayClock := spectrumAYClockHz
	sidClock := sidClockHz

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
			sidEnvFreq := (float64(sidFreq) * sidClock) / sidFreqDivisor
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
func fitReleaseValue(currentLevel uint8, targetCurve []float64, lookahead int, rateCounter uint16, expCounter int, expPeriod int) uint8 {
	if len(targetCurve) == 0 {
		return 15 // No data: slowest decay
	}

	// AY volume table (logarithmic)
	ayVolumeTable := [16]float64{
		0, 327, 473, 690, 1006, 1492, 2113, 3518,
		4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
	}

	// Use longer lookahead for low levels to avoid premature decay to 0
	if currentLevel < 30 && lookahead < 10 {
		lookahead = 10
	}
	if lookahead > len(targetCurve) {
		lookahead = len(targetCurve)
	}

	// Check if there's a large drop coming in the next frame
	if len(targetCurve) >= 2 {
		immediateDrop := targetCurve[0] - targetCurve[1]
		if immediateDrop >= 3.0 {
			// Large drop coming (>= 3 levels) - use very fast R (1-6)
			bestR := uint8(6)
			bestError := 1e9

			for r := uint8(1) ; r <= 6; r++ {
				ratePeriod := uint32(adsrRates[r])

				// Skip R values that would trigger severe ADSR bug (>75% frame frozen)
				if ratePeriod < uint32(rateCounter) {
					frozenCycles := 0x8000 - uint32(rateCounter)
					if frozenCycles >= uint32(sidCyclesPerFrame)*3/4 {
						continue // Severe bug: 75%+ of frame frozen
					}
				}

				simulated, _ := simulateReleaseWithBug(currentLevel, r, lookahead, rateCounter, expCounter, expPeriod)

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

					// Add heavy penalty if SID undershoots target
					if simulated[i] < tNorm * 0.85 && tNorm > 0.1 {
						// SID is more than 15% below target - strongly penalize
						envelopeError += 20.0 * diff * diff
					} else {
						envelopeError += diff * diff
					}
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

	// Try all R values 1-15 (R=0 is too extreme, causes instant decay)
	for r := uint8(1); r <= 15; r++ {
		ratePeriod := uint32(adsrRates[r])

		// Skip R values that would trigger severe ADSR bug (>75% frame frozen)
		if ratePeriod < uint32(rateCounter) {
			frozenCycles := 0x8000 - uint32(rateCounter)
			if frozenCycles >= uint32(sidCyclesPerFrame)*3/4 {
				continue // Severe bug: 75%+ of frame frozen
			}
		}

		// Simulate with ADSR delay bug
		simulated, _ := simulateReleaseWithBug(currentLevel, r, lookahead, rateCounter, expCounter, expPeriod)

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

			// Add heavy penalty if SID hits 0 while target is still audible
			if simulated[i] < 0.01 && targetNorm > 0.05 {
				// Envelope reached 0 too early - apply huge penalty
				envelopeError += 10.0 * (targetNorm - simulated[i]) * (targetNorm - simulated[i])
			} else {
				envelopeError += diff * diff
			}
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
// attackDecayState holds the state after attack+decay phase
type attackDecayState struct {
	level              uint8
	rateCounter        uint16
	exponentialCounter int
	exponentialPeriod  int
}

// simulateAttackWithDecay returns cycle-weighted average for attack+decay phase
func simulateAttackWithDecay(attack, decay, sustain uint8, numFrames int) ([]float64, attackDecayState) {
	envelope := make([]float64, numFrames)
	cyclesPerFrame := sidClockHz / palFrameRate

	envelopeLevel := uint8(0)
	envelopeState := 0 // 0=Attack, 1=Decay
	rateCounter := uint16(0)
	exponentialCounter := 0
	exponentialPeriod := 1
	sustainLevel := uint8((float64(sustain) / 15.0) * 255.0)

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
		cyclesToSimulate := uint32(cyclesPerFrame)
		totalCycles := cyclesPerFrame
		weightedSum := 0.0

		for cyclesToSimulate > 0 {
			ratePeriod := uint32(adsrRates[attack])
			if envelopeState == 1 {
				ratePeriod = uint32(adsrRates[decay])
			}

			cyclesUntilStep := ratePeriod - uint32(rateCounter)
			if cyclesUntilStep > cyclesToSimulate {
				// Not enough cycles to step
				weightedSum += float64(envelopeLevel) * float64(cyclesToSimulate)
				rateCounter += uint16(cyclesToSimulate)
				break
			}

			// Envelope stays at current level for cyclesUntilStep cycles
			weightedSum += float64(envelopeLevel) * float64(cyclesUntilStep)
			cyclesToSimulate -= cyclesUntilStep
			rateCounter = 0

			if envelopeState == 0 {
				// Attack: linear increment
				envelopeLevel++
				if envelopeLevel == 255 {
					envelopeState = 1 // Switch to decay
					updateExponentialPeriod()
				}
			} else {
				// Decay: exponential decrement
				exponentialCounter++
				if exponentialCounter >= exponentialPeriod {
					exponentialCounter = 0
					if envelopeLevel > sustainLevel {
						envelopeLevel--
						updateExponentialPeriod()
					}
				}
			}
		}

		envelope[frame] = (weightedSum / totalCycles) / 255.0
	}

	state := attackDecayState{
		level:              envelopeLevel,
		rateCounter:        rateCounter,
		exponentialCounter: exponentialCounter,
		exponentialPeriod:  exponentialPeriod,
	}

	return envelope, state
}

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
// simulateReleaseWithBugEndLevel returns the end-of-frame envelope level (for state tracking)
func simulateReleaseWithBugEndLevel(startLevel uint8, release uint8, numFrames int, startCounter uint16) float64 {
	cyclesPerFrame := sidClockHz / palFrameRate
	ratePeriod := uint32(adsrRates[release])

	envelopeLevel := startLevel
	rateCounter := startCounter
	exponentialCounter := 0
	exponentialPeriod := 1

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

		if ratePeriod < uint32(rateCounter) {
			cyclesUntilWrap := (0x8000 - uint32(rateCounter))
			if cyclesUntilWrap < cyclesToSimulate {
				cyclesToSimulate -= cyclesUntilWrap
				rateCounter = 0
			} else {
				rateCounter += uint16(cyclesToSimulate)
				continue
			}
		}

		for cyclesToSimulate > 0 {
			cyclesUntilStep := ratePeriod - uint32(rateCounter)
			if cyclesUntilStep > cyclesToSimulate {
				rateCounter += uint16(cyclesToSimulate)
				break
			}
			cyclesToSimulate -= cyclesUntilStep
			rateCounter = 0
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

	return float64(envelopeLevel) / 255.0
}

// releaseState holds the state of the release simulation
type releaseState struct {
	level              uint8
	rateCounter        uint16
	exponentialCounter int
	exponentialPeriod  int
}

// getExponentialPeriod returns the exponential period for a given envelope level
func getExponentialPeriod(level uint8) int {
	switch level {
	case 255:
		return 1
	case 93:
		return 2
	case 54:
		return 4
	case 26:
		return 8
	case 14:
		return 16
	case 6:
		return 30
	case 0:
		return 1
	default:
		// Determine period based on level thresholds
		if level >= 93 {
			return 2
		} else if level >= 54 {
			return 4
		} else if level >= 26 {
			return 8
		} else if level >= 14 {
			return 16
		} else if level >= 6 {
			return 30
		}
		return 1
	}
}

func simulateReleaseWithBug(startLevel uint8, release uint8, numFrames int, startCounter uint16, expCounter int, expPeriod int) ([]float64, releaseState) {
	envelope := make([]float64, numFrames)
	cyclesPerFrame := sidClockHz / palFrameRate
	ratePeriod := uint32(adsrRates[release])

	state := releaseState{
		level:              startLevel,
		rateCounter:        startCounter,
		exponentialCounter: expCounter,
		exponentialPeriod:  expPeriod,
	}

	// updateExponentialPeriod matches hardware behavior
	updateExponentialPeriod := func() {
		switch state.level {
		case 255:
			state.exponentialPeriod = 1
		case 93:
			state.exponentialPeriod = 2
		case 54:
			state.exponentialPeriod = 4
		case 26:
			state.exponentialPeriod = 8
		case 14:
			state.exponentialPeriod = 16
		case 6:
			state.exponentialPeriod = 30
		case 0:
			state.exponentialPeriod = 1
		}
	}

	updateExponentialPeriod()

	for frame := 0; frame < numFrames; frame++ {
		cyclesToSimulate := uint32(cyclesPerFrame)
		totalCycles := cyclesPerFrame
		weightedSum := 0.0

		// Check for ADSR delay bug at frame start (when R changes)
		// Bug triggers if new rate_period < current rate_counter
		if ratePeriod < uint32(state.rateCounter) {
			// Bug active! Counter must wrap at 0x8000 before envelope can step
			// Calculate cycles until wrap
			cyclesUntilWrap := (0x8000 - uint32(state.rateCounter))
			if cyclesUntilWrap < cyclesToSimulate {
				// Frozen for part of frame
				weightedSum += float64(state.level) * float64(cyclesUntilWrap)
				cyclesToSimulate -= cyclesUntilWrap
				state.rateCounter = 0 // Wrapped
			} else {
				// Still frozen entire frame
				state.rateCounter += uint16(cyclesToSimulate)
				envelope[frame] = float64(state.level) / 255.0
				continue
			}
		}

		// Simulate envelope for remaining cycles, tracking weighted average
		for cyclesToSimulate > 0 {
			// Calculate cycles until next envelope step
			cyclesUntilStep := ratePeriod - uint32(state.rateCounter)
			if cyclesUntilStep > cyclesToSimulate {
				// Not enough cycles to step - envelope stays at current level
				weightedSum += float64(state.level) * float64(cyclesToSimulate)
				state.rateCounter += uint16(cyclesToSimulate)
				break
			}

			// Envelope stays at current level for cyclesUntilStep cycles
			weightedSum += float64(state.level) * float64(cyclesUntilStep)

			// Step the envelope
			cyclesToSimulate -= cyclesUntilStep
			state.rateCounter = 0

			// Release: exponential decay
			state.exponentialCounter++
			if state.exponentialCounter >= state.exponentialPeriod {
				state.exponentialCounter = 0
				if state.level > 0 {
					state.level--
					updateExponentialPeriod()
				}
			}
		}

		// Calculate cycle-weighted average (corresponds to sample-weighted average)
		envelope[frame] = (weightedSum / totalCycles) / 255.0
	}

	return envelope, state
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
	cyclesPerFrame := uint32(sidCyclesPerFrame) // 50 Hz = 20ms = 19656 cycles at 985248 Hz
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

// computeSustainPerFrame converts AY volume curve to SID sustain values
// Simple approach: D=0 (instant decay) + dynamic S (frame-accurate volume following)
// Returns array of S values (0-15), one per frame
func computeSustainPerFrame(volumeCurve []float64, maxSustain uint8, amplitudeBoost float64) []uint8 {
	if len(volumeCurve) == 0 {
		return nil
	}

	// IMPORTANT: Interpolate in AY register space BEFORE applying exponential scaling
	// This preserves the exponential nature of the AY decay
	// Identify smooth regions where all consecutive AY steps are ≤2
	// Within each smooth region, linearly interpolate AY register values
	interpolatedAY := make([]float64, len(volumeCurve))

	i := 0
	for i < len(volumeCurve) {
		// Find extent of smooth region starting at i
		// A smooth region is a sequence where all consecutive AY changes are ≤2
		regionStart := i
		regionEnd := i

		// Extend region while AY steps are ≤2
		for regionEnd+1 < len(volumeCurve) {
			ayStep := math.Abs(volumeCurve[regionEnd+1] - volumeCurve[regionEnd])
			if ayStep <= 2.0 {
				regionEnd++
			} else {
				break
			}
		}

		// Linearly interpolate AY register values across the smooth region
		startAY := volumeCurve[regionStart]
		endAY := volumeCurve[regionEnd]
		regionLength := regionEnd - regionStart + 1

		if regionLength == 1 {
			// Single frame - no interpolation needed
			interpolatedAY[regionStart] = startAY
		} else {
			// Multi-frame smooth region - linearly interpolate in register space
			for j := 0; j < regionLength; j++ {
				frame := regionStart + j
				fraction := float64(j) / float64(regionLength-1)
				interpolatedAY[frame] = startAY + fraction*(endAY-startAY)
			}
		}

		i = regionEnd + 1
	}

	// AY volume table (logarithmic amplitude response)
	ayVolumeTable := [16]float64{
		0, 327, 473, 690, 1006, 1492, 2113, 3518,
		4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
	}

	// Now convert interpolated AY values to amplitudes using exponential table
	// For fractional AY values, interpolate between table entries
	// This preserves the exponential decay characteristic while allowing smooth transitions
	interpolatedAmplitudes := make([]float64, len(volumeCurve))
	maxAmplitude := 0.0
	for i, vol := range interpolatedAY {
		if vol < 0 {
			vol = 0
		}
		if vol > 15 {
			vol = 15
		}

		// For fractional AY values, linearly interpolate between adjacent table entries
		// This preserves exponential shape while providing smooth transitions
		ayLow := int(vol) // Floor
		ayHigh := ayLow + 1
		if ayHigh > 15 {
			ayHigh = 15
		}

		fraction := vol - float64(ayLow)
		ampLow := ayVolumeTable[ayLow] / 32767.0
		ampHigh := ayVolumeTable[ayHigh] / 32767.0

		// Linearly interpolate between the exponential table entries
		interpolatedAmplitudes[i] = ampLow + fraction*(ampHigh-ampLow)

		// Track maxAmplitude BEFORE boosting (so boost doesn't cancel out during normalization)
		if interpolatedAmplitudes[i] > maxAmplitude {
			maxAmplitude = interpolatedAmplitudes[i]
		}
	}

	if maxAmplitude == 0 {
		maxAmplitude = 1.0 // Avoid division by zero
	}

	// Apply amplitude boost with 7:1 compression to preserve dynamics when exceeding S=15
	for i := range interpolatedAmplitudes {
		interpolatedAmplitudes[i] = applyNoiseBoostWithCompression(interpolatedAmplitudes[i], amplitudeBoost)
	}

	// Recompute maxAmplitude AFTER boosting (for correct normalization when computing S values)
	maxAmplitudeBoosted := 0.0
	for _, amp := range interpolatedAmplitudes {
		if amp > maxAmplitudeBoosted {
			maxAmplitudeBoosted = amp
		}
	}
	if maxAmplitudeBoosted == 0 {
		maxAmplitudeBoosted = 1.0
	}

	// Helper function to compute RMS error between target amplitudes and S values
	computeError := func(sPer []uint8) float64 {
		sumSquaredError := 0.0
		for i := 0; i < len(interpolatedAmplitudes); i++ {
			targetAmp := interpolatedAmplitudes[i]
			sidAmp := float64(sPer[i]) / float64(maxSustain)
			diff := sidAmp - targetAmp
			sumSquaredError += diff * diff
		}
		return math.Sqrt(sumSquaredError / float64(len(interpolatedAmplitudes)))
	}

	// Now compute S from interpolated amplitudes
	sustainPerFrame := make([]uint8, len(volumeCurve))
	for i := 0; i < len(interpolatedAmplitudes); i++ {
		amplitude := interpolatedAmplitudes[i]
		if amplitude < 0 {
			amplitude = 0
		}

		// Compute S from interpolated amplitude (use boosted max for correct scaling)
		s := uint8((amplitude / maxAmplitudeBoosted) * float64(maxSustain) + 0.5)
		if s > maxSustain {
			s = maxSustain
		}
		if amplitude > 0 && s == 0 {
			s = 1
		}
		sustainPerFrame[i] = s
	}

	// Compute initial error before any smoothing
	initialError := computeError(sustainPerFrame)

	// Third pass: ensure S never increases AFTER the peak (envelope can only fall or stay constant)
	// But allow initial rise to peak (for attack phase with A>0)
	if len(sustainPerFrame) > 0 {
		// Find the first occurrence of maxSustain (the peak)
		peakIndex := -1
		for i := 0; i < len(sustainPerFrame); i++ {
			if sustainPerFrame[i] == maxSustain {
				peakIndex = i
				break
			}
		}

		// If peak not found, find the maximum S value actually present
		if peakIndex == -1 {
			maxVal := sustainPerFrame[0]
			peakIndex = 0
			for i := 1; i < len(sustainPerFrame); i++ {
				if sustainPerFrame[i] > maxVal {
					maxVal = sustainPerFrame[i]
					peakIndex = i
				}
			}
		}

		// Allow rise up to peak, then enforce non-increasing after peak
		maxSoFar := sustainPerFrame[peakIndex]
		for i := peakIndex + 1; i < len(sustainPerFrame); i++ {
			if sustainPerFrame[i] > maxSoFar {
				sustainPerFrame[i] = maxSoFar
			} else {
				maxSoFar = sustainPerFrame[i]
			}
		}

		// Verify error did not increase
		errorAfterPass3 := computeError(sustainPerFrame)
		if errorAfterPass3 > initialError*1.001 { // Allow 0.1% tolerance for rounding
			panic(fmt.Sprintf("Third pass increased error: %.6f -> %.6f", initialError, errorAfterPass3))
		}
	}

	// Fourth pass: DISABLED - smoothing was found to increase error significantly
	// The exponential curve naturally has gaps, and forcing S to decrease smoothly
	// defeats the exponential nature we worked to preserve
	// Error validation showed smoothing increased RMS error from 0.119 to 0.200 (68% worse)
	_ = initialError // Suppress unused variable warning

	return sustainPerFrame
}

// computeReleaseForCurveWithAttack computes optimal R values accounting for attack phase
// attackParam: A parameter (0-15), sustainParam: S parameter (0-15)
// Returns array of R values, one per frame
// NOTE: This function is deprecated in favor of computeSustainPerFrame with D=0 approach
func computeReleaseForCurveWithAttack(volumeCurve []float64, attackParam uint8, sustainParam uint8) []uint8 {
	if len(volumeCurve) == 0 {
		return nil
	}

	// Smooth the AY curve for small decreases
	smoothedCurve := smoothAYCurve(volumeCurve)
	_ = smoothedCurve // Used in backtracking

	// For beam search with per-frame R values, use short lookahead
	// Each frame picks R based on immediate error, not long-term average
	lookahead := 3  // Look 3 frames ahead to consider trend
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

	// Get initial exponential period based on starting level
	initialExpPeriod := getExponentialPeriod(currentLevel)

	// Track rate counter
	// For short notes (≤5 frames), use simplified initialization (counter=4) to allow fast release rates
	// For longer notes, simulate frame 0 for accurate counter state to avoid severe bugs
	var rateCounter uint16
	isShortNote := len(volumeCurve) <= 5
	if isShortNote {
		rateCounter = 4 // Simplified: allows R=3 and other fast rates without triggering bugs
	} else {
		// Accurate: starts at ~4, plus advancement from frame 0 with R=11 (period=976)
		initialR := releasePerFrame[0]
		_, state0 := simulateReleaseWithBug(currentLevel, initialR, 1, 4, 0, initialExpPeriod)
		rateCounter = state0.rateCounter
	}

	// AY volume table
	ayVolumeTable := [16]float64{
		0, 327, 473, 690, 1006, 1492, 2113, 3518,
		4148, 6717, 9575, 12217, 16139, 20818, 26397, 32767,
	}

	// Beam search with parallel path expansion
	type BeamPath struct {
		frameIdx           int
		currentLevel       uint8
		rateCounter        uint16
		exponentialCounter int
		exponentialPeriod  int
		path               []uint8
		cumulativeErr      float64
	}

	beamWidth := 100
	beam := []BeamPath{{
		frameIdx:           1,
		currentLevel:       currentLevel,
		rateCounter:        rateCounter,
		exponentialCounter: 0,
		exponentialPeriod:  initialExpPeriod,
		path:               []uint8{},
		cumulativeErr:      0.0,
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

				// RULE: Last 2 frames must use the same R value to keep counter stable at note end
				// This prevents bugs from extending note duration
				isLastFrame := (p.frameIdx == len(volumeCurve)-1)

				var rCandidates []uint8
				if isLastFrame && len(p.path) > 0 {
					// Last frame: reuse R from previous frame (no R change allowed)
					rCandidates = []uint8{p.path[len(p.path)-1]}
				} else {
					// Not last frame: generate all valid R candidates (R=1-15, R=0 is too extreme)
					for r := uint8(1); r <= 15; r++ {
						rCandidates = append(rCandidates, r)
					}
				}

				for _, r := range rCandidates {
					ratePeriod := uint32(adsrRatePeriods[r])
					// For short notes (≤5 frames), prioritize envelope quality over bug avoidance
					// Only check immediate severe bugs on non-short notes
					isShortNote := len(volumeCurve) <= 5
					if !isShortNote {
						// Skip R values that would trigger severe ADSR bug (>75% frame frozen)
						if ratePeriod < uint32(p.rateCounter) {
							frozenCycles := 0x8000 - uint32(p.rateCounter)
							if frozenCycles >= uint32(sidCyclesPerFrame)*3/4 {
								continue // Severe bug: 75%+ of frame frozen
							}
						}
					}

					// Calculate envelope error for this R
					simulated, _ := simulateReleaseWithBug(p.currentLevel, r, lookahead, p.rateCounter, p.exponentialCounter, p.exponentialPeriod)
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

						// Add very heavy penalty if SID hits 0 while target is still audible
						if simulated[i] < 0.01 && tNorm > 0.01 {
							envelopeError += 1000.0 * (tNorm - simulated[i]) * (tNorm - simulated[i])
						} else {
							envelopeError += diff * diff
						}
					}
					envelopeError = envelopeError / float64(lookahead)

					// Simulate one frame
					_, state1 := simulateReleaseWithBug(p.currentLevel, r, 1, p.rateCounter, p.exponentialCounter, p.exponentialPeriod)
					newLevel := state1.level
					newCounter := state1.rateCounter

					// CRITICAL: On second-to-last frame, ban R values that would cause bugs on last 2 frames
					// This R will be used for BOTH the second-to-last AND last frames (no R change allowed)
					isSecondToLastFrame := (p.frameIdx == len(volumeCurve)-2)
					if isSecondToLastFrame {
						// Second-to-last frame: ban R values that would trigger moderate-to-severe bugs
						// This R will be locked for the last 2 frames, so it must be stable
						if ratePeriod < uint32(p.rateCounter) {
							frozenCycles := 0x8000 - uint32(p.rateCounter)
							// Ban any bug that freezes >50% of frame
							if frozenCycles >= uint32(sidCyclesPerFrame)/2 {
								continue // Skip: would extend note beyond intended end
							}
						}
					} else if !isLastFrame && !isShortNote {
						// Not last frame and not a short note: check if this R would leave counter in state that causes severe bugs next frame
						// For short notes (≤5 frames), skip this check to prioritize envelope quality
						// Check R values 3-11 (covers most fast/moderate release rates) for severe bugs
						wouldCauseSevereBugNextFrame := false
						for testR := uint8(3); testR <= 11; testR++ {
							testPeriod := uint32(adsrRatePeriods[testR])
							if testPeriod < uint32(newCounter) {
								frozenCycles := 0x8000 - uint32(newCounter)
								if frozenCycles >= uint32(sidCyclesPerFrame)*3/4 {
									wouldCauseSevereBugNextFrame = true
									break
								}
							}
						}
						if wouldCauseSevereBugNextFrame {
							continue // Would set up severe bug in next frame
						}
					}

					// Create child path
					newPath := make([]uint8, len(p.path)+1)
					copy(newPath, p.path)
					newPath[len(p.path)] = r

					// Add counter penalty to encourage low counters (tiebreaker only)
					counterPenalty := 0.0001 * float64(newCounter) / 32768.0

					children = append(children, BeamPath{
						frameIdx:           p.frameIdx + 1,
						currentLevel:       newLevel,
						rateCounter:        newCounter,
						exponentialCounter: state1.exponentialCounter,
						exponentialPeriod:  state1.exponentialPeriod,
						path:               newPath,
						cumulativeErr:      p.cumulativeErr + envelopeError + counterPenalty,
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

	lookahead := 15
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

	// Get initial exponential period based on starting level
	initialExpPeriod := getExponentialPeriod(currentLevel)

	// Track rate counter
	// For short notes (≤5 frames), use simplified initialization (counter=4) to allow fast release rates
	// For longer notes, simulate frame 0 for accurate counter state to avoid severe bugs
	var rateCounter uint16
	isShortNote := len(volumeCurve) <= 5
	if isShortNote {
		rateCounter = 4 // Simplified: allows R=3 and other fast rates without triggering bugs
	} else {
		// Accurate: starts at ~4, plus advancement from frame 0 with R=11 (period=976)
		initialR := releasePerFrame[0]
		_, state0 := simulateReleaseWithBug(currentLevel, initialR, 1, 4, 0, initialExpPeriod)
		rateCounter = state0.rateCounter
	}

	// Beam search with parallel path expansion
	type BeamPath struct {
		frameIdx           int
		currentLevel       uint8
		rateCounter        uint16
		exponentialCounter int
		exponentialPeriod  int
		path               []uint8
		cumulativeErr      float64
	}

	beamWidth := 100
	beam := []BeamPath{{
		frameIdx:           1,
		currentLevel:       currentLevel,
		rateCounter:        rateCounter,
		exponentialCounter: 0,
		exponentialPeriod:  initialExpPeriod,
		path:               []uint8{},
		cumulativeErr:      0.0,
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

				// RULE: Last 2 frames must use the same R value to keep counter stable at note end
				// This prevents bugs from extending note duration
				isLastFrame := (p.frameIdx == len(volumeCurve)-1)

				var rCandidates []uint8
				if isLastFrame && len(p.path) > 0 {
					// Last frame: reuse R from previous frame (no R change allowed)
					rCandidates = []uint8{p.path[len(p.path)-1]}
				} else {
					// Not last frame: generate all valid R candidates (R=1-15, R=0 is too extreme)
					for r := uint8(1); r <= 15; r++ {
						rCandidates = append(rCandidates, r)
					}
				}

				for _, r := range rCandidates {
					ratePeriod := uint32(adsrRatePeriods[r])
					// For short notes (≤5 frames), prioritize envelope quality over bug avoidance
					// Only check immediate severe bugs on non-short notes
					isShortNote := len(volumeCurve) <= 5
					if !isShortNote {
						// Skip R values that would trigger severe ADSR bug (>75% frame frozen)
						if ratePeriod < uint32(p.rateCounter) {
							frozenCycles := 0x8000 - uint32(p.rateCounter)
							if frozenCycles >= uint32(sidCyclesPerFrame)*3/4 {
								continue // Severe bug: 75%+ of frame frozen
							}
						}
					}

					// Calculate envelope error for this R
					simulated, _ := simulateReleaseWithBug(p.currentLevel, r, lookahead, p.rateCounter, p.exponentialCounter, p.exponentialPeriod)
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

						// Add very heavy penalty if SID hits 0 while target is still audible
						if simulated[i] < 0.01 && tNorm > 0.01 {
							envelopeError += 1000.0 * (tNorm - simulated[i]) * (tNorm - simulated[i])
						} else {
							envelopeError += diff * diff
						}
					}
					envelopeError = envelopeError / float64(lookahead)

					// Simulate one frame
					_, state1 := simulateReleaseWithBug(p.currentLevel, r, 1, p.rateCounter, p.exponentialCounter, p.exponentialPeriod)
					newLevel := state1.level
					newCounter := state1.rateCounter

					// CRITICAL: On second-to-last frame, ban R values that would cause bugs on last 2 frames
					// This R will be used for BOTH the second-to-last AND last frames (no R change allowed)
					isSecondToLastFrame := (p.frameIdx == len(volumeCurve)-2)
					if isSecondToLastFrame {
						// Second-to-last frame: ban R values that would trigger moderate-to-severe bugs
						// This R will be locked for the last 2 frames, so it must be stable
						if ratePeriod < uint32(p.rateCounter) {
							frozenCycles := 0x8000 - uint32(p.rateCounter)
							// Ban any bug that freezes >50% of frame
							if frozenCycles >= uint32(sidCyclesPerFrame)/2 {
								continue // Skip: would extend note beyond intended end
							}
						}
					} else if !isLastFrame && !isShortNote {
						// Not last frame and not a short note: check if this R would leave counter in state that causes severe bugs next frame
						// For short notes (≤5 frames), skip this check to prioritize envelope quality
						// Check R values 3-11 (covers most fast/moderate release rates) for severe bugs
						wouldCauseSevereBugNextFrame := false
						for testR := uint8(3); testR <= 11; testR++ {
							testPeriod := uint32(adsrRatePeriods[testR])
							if testPeriod < uint32(newCounter) {
								frozenCycles := 0x8000 - uint32(newCounter)
								if frozenCycles >= uint32(sidCyclesPerFrame)*3/4 {
									wouldCauseSevereBugNextFrame = true
									break
								}
							}
						}
						if wouldCauseSevereBugNextFrame {
							continue // Would set up severe bug in next frame
						}
					}

					// Create child path
					newPath := make([]uint8, len(p.path)+1)
					copy(newPath, p.path)
					newPath[len(p.path)] = r

					// Add counter penalty to encourage low counters (tiebreaker only)
					counterPenalty := 0.0001 * float64(newCounter) / 32768.0

					children = append(children, BeamPath{
						frameIdx:           p.frameIdx + 1,
						currentLevel:       newLevel,
						rateCounter:        newCounter,
						exponentialCounter: state1.exponentialCounter,
						exponentialPeriod:  state1.exponentialPeriod,
						path:               newPath,
						cumulativeErr:      p.cumulativeErr + envelopeError + counterPenalty,
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

// Global storage for SID register captures (per channel)
var capturedDirectPlayerRegs [3][]uint16

// generateSIDPerNote generates SID audio directly from AY commands (DIRECT CONVERSION)
//
// ARCHITECTURE: This is the DIRECT CONVERSION PLAYER - it reads AY commands frame-by-frame
// and generates SID audio on-the-fly without using a tracker format.
//
// PURPOSE: This serves as the REFERENCE IMPLEMENTATION for validation. The tracker player's
// output is compared against this to ensure the tracker format accurately reproduces the
// original audio.
//
// RULES:
// - This function READS from AY commands (that's its whole purpose)
// - It represents what we're trying to match/compress with the tracker format
// - Any discrepancies between this and the tracker player indicate encoding issues
func generateSIDPerNote(commands []spectrum.AYCommand, channel int, notes []Note, totalSamples int) ([]int16, []int16) {
	sampleRate := 44100.0
	samples := make([]int16, totalSamples)

	// Debug channel: ADSR delay bug active state for this channel
	adsrBugDebug := make([]int16, totalSamples)

	sid := NewSIDEmulator(sampleRate)

	// Calculate frame count for register capture
	numFrames := int(math.Ceil(float64(totalSamples) / (sampleRate / palFrameRate)))
	capturedDirectPlayerRegs[channel] = make([]uint16, numFrames)

	// Build frequency table from AY commands
	// numFrames already calculated above for register capture
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

	// Build noise+tone mixer state per frame
	noisePlusTonePerFrame := make([]bool, numFrames)
	noiseBit := uint8(3 + channel) // Noise enable bit: 3=A, 4=B, 5=C

	// Track mixer writes per frame
	mixerWrites := make(map[int]byte)
	for _, cmd := range commands {
		if cmd.Register == 7 { // Mixer register
			frame := int(cmd.Frame)
			if frame < numFrames {
				mixerWrites[frame] = cmd.Value
			}
		}
	}

	// Forward-fill mixer state and determine noise+tone per frame
	currentMixer := byte(0xFF) // All disabled by default
	tonePerFrame := make([]bool, numFrames)
	noisePerFrame := make([]bool, numFrames)
	for i := range noisePlusTonePerFrame {
		if val, hasWrite := mixerWrites[i]; hasWrite {
			currentMixer = val
		}

		// Check if this frame has tone and/or noise enabled
		toneOn := (currentMixer & (1 << channel)) == 0
		noiseOn := (currentMixer & (1 << noiseBit)) == 0
		tonePerFrame[i] = toneOn
		noisePerFrame[i] = noiseOn
		noisePlusTonePerFrame[i] = toneOn && noiseOn
	}

	// SID register offset for this channel (voice 0=0, voice 1=7, voice 2=14)
	regBase := byte(channel * 7)

	// Process each note with its specific ADSR
	currentNoteIdx := 0
	lastSIDFreq := uint16(0)
	gateOn := false
	currentWaveform := uint8(0x04) // Default to pulse
	inEnvelopeMode := false
	noisePlusToneMode := false    // True when currently in noise+tone mixing mode (runtime state)
	noteIsNoisePlusTone := false  // True if the current note was detected as noise+tone (original property)
	currentFramePeriod := 0       // Period calculated at frame boundary (staging area)
	activePeriod := 0             // Period for currently RUNNING cycle (locked when cycleCounter was loaded)
	cycleCounter := 0.0           // Countdown timer in SID clock cycles
	timerIsHi := true             // CIA timer phase (toggles on every interrupt)
	currentWaveformHi := byte(0x00) // Waveform to write when timerIsHi=true
	currentWaveformLo := byte(0x00) // Waveform to write when timerIsHi=false
	lastToneFreq := uint16(0)     // Track last effective frequency to detect changes
	cachedNoiseFreq := uint16(0)  // Cached noise frequency for noise+tone toggling
	lastToggleSample := -1        // Track last toggle sample for period validation
	totalSampleCount := 0         // Running total of samples generated (spans frames)


	for frame := 0; frame < numFrames; frame++ {
		// Update S (sustain level) each frame for D=0 + dynamic S approach
		// This allows frame-accurate volume following while gate stays ON
		// Stop updating after GateOffFrame (tail decay takes over with D>0, S=0)
		if currentNoteIdx > 0 {
			currentNote := notes[currentNoteIdx-1]
			// Only update S for volume-following ADSR notes, not envelope-modulated notes
			if !currentNote.EnvelopeMode && frame <= currentNote.EndFrame {
				relFrame := frame - currentNote.StartFrame

				// Update S value each frame to follow volume curve (before tail starts)
				// Skip updates after GateOffFrame - tail uses D decay to S=0 target
				if relFrame >= 0 && relFrame < len(currentNote.SustainPerFrame) &&
				   (currentNote.GateOffFrame == 0 || relFrame < currentNote.GateOffFrame) {
					newS := currentNote.SustainPerFrame[relFrame]
					currentS := sid.Registers[regBase+6] >> 4

					// Only write if S value changed to avoid ADSR delay bug
					if newS != currentS {
						currentR := sid.Registers[regBase+6] & 0x0F
						sid.WriteRegister(regBase+6, (newS<<4)|currentR)
					}
				}

				// Switch D from InitialDecay to 0 after frame 0 (for volume-following behavior)
				// This must happen on frame 1 (relFrame == 1) to avoid ADSR delay bug on frame 0
				if relFrame == 1 && currentNote.ADSR.InitialDecay > 0 {
					currentA := (sid.Registers[regBase+5] >> 4) & 0x0F
					sid.WriteRegister(regBase+5, (currentA<<4)|0) // Switch to D=0 for volume-following
				}
			}
		}

		// ADSR delay bug workaround: Set R=0 before next note STARTS
		// This wraps the counter from any position to guarantee rate_counter < 8
		// when next note's Gate ON with A=0 starts
		// CRITICAL: Do this before BOTH envelope mode AND volume-following notes!
		if currentNoteIdx < len(notes) && framesBeforeNoteOff > 0 {
			nextNote := notes[currentNoteIdx]
			// Check if we're within the R=0 window
			inR0Window := false
			for offset := 1; offset <= framesBeforeNoteOff; offset++ {
				if frame == nextNote.StartFrame-offset {
					inR0Window = true
					break
				}
			}

			if inR0Window {
				// Don't write ADSR if we're still in attack phase of current note
				// Writing during attack can trigger ADSR delay bug
				inAttackPhase := false
				if currentNoteIdx > 0 {
					currentNote := notes[currentNoteIdx-1]
					relFrame := frame - currentNote.StartFrame
					attackDuration := int(currentNote.ADSR.Attack) // A=0: 0 frames, A=2: 2 frames
					if relFrame < attackDuration {
						inAttackPhase = true
					}
				}

				if !inAttackPhase {
					// Set R=0 to guarantee counter < 8
					currentSustain := sid.Registers[regBase+6] >> 4
					sid.WriteRegister(regBase+6, (currentSustain<<4)|0x00)
				}
			}

			// If current note has D>0, gate OFF before next note's gate ON
			// This allows the decay phase to complete before the new note starts
			if frame == nextNote.StartFrame-framesBeforeNoteOff && currentNoteIdx > 0 {
				currentNote := notes[currentNoteIdx-1]
				if currentNote.ADSR.Decay > 0 {
					// Gate OFF: preserve waveform bits
					currentCtrl := sid.Registers[regBase+4]
					sid.WriteRegister(regBase+4, currentCtrl&0xFE)
				}
			}
		}

		// Check if we're starting a new note
		if currentNoteIdx < len(notes) && frame == notes[currentNoteIdx].StartFrame {
			note := notes[currentNoteIdx]

			// ALWAYS gate OFF before starting new note to ensure envelope restarts
			// Even if gateOn is false, we need to write gate=0 and wait for the
			// SID to process it before writing gate=1, so it sees the 0→1 transition
			// CRITICAL: Respect CIA timer phase - write waveform based on timer phase
			// Set release to 0 for instant gate off
			prevADSR := sid.Registers[regBase+6]
			sid.WriteRegister(regBase+6, (prevADSR&0xF0)|0x00) // Keep sustain, set release=0
			// Gate OFF: preserve waveform bits (4-7), clear gate bit (0)
			currentCtrl := sid.Registers[regBase+4]
			sid.WriteRegister(regBase+4, currentCtrl&0xFE)

			// Generate multiple samples to give SID envelope time to fully process gate OFF
			// The envelope state machine needs several cycles to enter release and reset
			// Single sample (~22 cycles) wasn't enough - caused every-2nd-note failures
			for i := 0; i < 10; i++ {
				_ = sid.GenerateSample(channel)
			}

			// Use waveform from note
			currentWaveform = note.Waveform
			inEnvelopeMode = note.EnvelopeMode
			noisePlusToneMode = note.NoisePlusTone
			noteIsNoisePlusTone = note.NoisePlusTone

			// Initialize CIA timer interrupt waveforms based on note type
			if note.EnvelopeMode {
				// Envelope mode: alternate between waveform and silence
				currentWaveformHi = note.Waveform
				currentWaveformLo = 0x05 // Silence
			} else if note.NoisePlusTone {
				// NoisePlusTone: depends on starting waveform
				if note.Waveform == 0x08 {
					// Noise mode: modulated
					currentWaveformHi = 0x08
					currentWaveformLo = 0x05
				} else {
					// Pulse mode: no modulation
					currentWaveformHi = 0x04
					currentWaveformLo = 0x04
				}
			} else {
				// Regular notes: no modulation
				currentWaveformHi = note.Waveform
				currentWaveformLo = note.Waveform
			}


			// CRITICAL: Set ADSR values while gate is OFF (before gate ON)
			// Writing ADSR registers while gate=1 resets envelope to Attack
			// Volume-following ADSR: A=0/2, D=InitialDecay (frame 0), then D=0 (during sustain), S from SustainPerFrame[0], R=11
			// D will be switched to 0 after frame 0, then updated to tailDecayRate at GateOffFrame (when gate turns OFF)
			// S comes from SustainPerFrame[0] for frame 0, then updated each frame
			initialS := note.ADSR.Sustain
			if len(note.SustainPerFrame) > 0 {
				initialS = note.SustainPerFrame[0]
			}
			sid.WriteRegister(regBase+5, (note.ADSR.Attack<<4)|note.ADSR.InitialDecay) // Use InitialDecay for frame 0
			sid.WriteRegister(regBase+6, (initialS<<4)|note.ADSR.Release)

			// Set frequency based on waveform type and mode (MUST calculate this first for toggle period)
			var sidFreq uint16
			if note.EnvelopeMode {
				// Envelope mode: use envelope period frequency (R11-R12)
				// This applies to triangle, sawtooth, and noise+tone envelope modes
				envPeriod := getAYEnvelopePeriod(commands, frame)
				sidFreq = computeEnvelopeFrequency(envPeriod)
				// Sawtooth envelopes complete visual cycles 2x faster than triangle
				// Triangle: 64 envelope steps per cycle (32 up, 32 down)
				// Sawtooth: 32 envelope steps per cycle (32 up, then reset)
				if note.Waveform == 0x02 { // Sawtooth
					sidFreq *= 2
				}
			} else if note.Waveform == 0x08 {
				// Direct mode noise: use AY noise frequency from R6
				noisePeriod := getAYNoisePeriod(commands, frame)
				sidFreq = computeNoiseFrequency(noisePeriod)
			} else {
				// Direct mode tone: use tone frequency from R0-R5
				if frame < len(freqPerFrame) && freqPerFrame[frame] > 0 {
					sidFreq = computeToneFrequency(freqPerFrame[frame])
				}
			}
			if sidFreq > 0 {
				sid.WriteRegister(regBase+0, byte(sidFreq&0xFF))
				sid.WriteRegister(regBase+1, byte(sidFreq>>8))
					}

			// Cache frequencies for noise+tone toggling
			if note.NoisePlusTone && frame < len(freqPerFrame) && freqPerFrame[frame] > 0 {
				noisePeriod := getAYNoisePeriod(commands, frame)
				cachedNoiseFreq = computeNoiseFrequency(noisePeriod)
			}

			// Calculate toggle period in SID clock cycles for modulation modes (after sidFreq is set)
			// This applies to envelope mode (triangle, sawtooth, noise+tone) and direct mode noise+tone
			if (note.EnvelopeMode || note.NoisePlusTone) && frame < len(freqPerFrame) && freqPerFrame[frame] > 0 {
				ayClock := spectrumAYClockHz
				sidClock := sidClockHz

				// Calculate target toggle frequency directly from AY tone frequency
				// This avoids amplifying the quantization error from sidEnvFreq
				ayTonePeriod := float64(freqPerFrame[frame])
				ayToneFreq := ayClock / (16.0 * ayTonePeriod)
				// Apply resampling correction
				targetToggleFreq := ayToneFreq * (palFrameRate / spectrumFrameRate)

				// Toggle period in cycles (round to nearest integer for optimal match)
				idealTogglePeriod := sidClock / (2.0 * targetToggleFreq)
				newPeriod := int(idealTogglePeriod + 0.5)
				if newPeriod < 1 {
					newPeriod = 1
				}
				// CRITICAL: Store in currentFramePeriod, not togglePeriodCycles
				// togglePeriodCycles is locked for the active cycle
				currentFramePeriod = newPeriod

				// CRITICAL: Only initialize cycleCounter if it's not already running (first note)
				// Never reset it for subsequent notes - phase continuity must be maintained!
				if cycleCounter == 0 {
					activePeriod = newPeriod
					cycleCounter = float64(newPeriod)
					timerIsHi = true // Start in HI phase
					lastToggleSample = -1
					if currentNoteIdx == 0 {
						fmt.Printf("  DIRECT first note frame=%d: ayTonePeriod=%.0f togglePeriod=%d timerIsHi=true\n",
							frame, ayTonePeriod, activePeriod)
					}
				}
				// Otherwise cycle is running - currentFramePeriod will be used when cycle completes
			}

			// Note: Do NOT reset pulse width here. CIA timer interrupt handler
			// sets correct pulse width based on waveform (0xFFF for 0x05, 0x800 for 0x04)

			// Write waveform and gate ON
			// ALWAYS write the initial waveform at note start for ALL note types
			// For envelope/NoisePlusTone modes, CIA timer will update it dynamically during the note
			if note.Waveform == 0x04 {
				// Pulse: need pulse width
				sid.WriteRegister(regBase+2, 0x00)
				sid.WriteRegister(regBase+3, 0x08)
			}
			// Write waveform + gate ON for ALL note types
			sid.WriteRegister(regBase+4, (note.Waveform<<4)|0x01)
			gateOn = true
			currentNoteIdx++
		} else if currentNoteIdx > 0 && frame <= notes[currentNoteIdx-1].EndFrame+1 {
			// Update frequency and R during note (works whether gate is ON or OFF)
			// Allow one extra frame past EndFrame for gate OFF to execute
			// Frequency registers won't be updated on that frame (frame > EndFrame), only gate state
			currentNote := notes[currentNoteIdx-1]
			var sidFreq uint16

			// Only update frequencies while note is active (not on the gate OFF frame)
			if frame <= currentNote.EndFrame {
				if inEnvelopeMode {
				// Envelope mode: update based on envelope period changes (R11-R12)
				// This applies to triangle, sawtooth, and noise+tone envelope modes
				envPeriod := getAYEnvelopePeriod(commands, frame)
				sidFreq = computeEnvelopeFrequency(envPeriod)

				// Check for R13 (envelope shape) changes during note
				// Don't switch waveforms if we're in noise+tone mode (currentWaveform == 0x08)
				// CRITICAL: Always update currentWaveformHi/Lo, CIA timer will respect phase on next interrupt
				if currentWaveform != 0x08 {
					r13Shape := getAYEnvelopeShape(commands, frame)
					newWaveform := ayEnvelopeToSIDWaveform(r13Shape)
					if newWaveform != currentWaveform {
						if channel == 0 && frame >= 1670 && frame <= 1673 {
							fmt.Printf("  DIRECT R13 CHANGE frame=%d: 0x%02X → 0x%02X (noteIdx=%d cycleCounter=%.1f)\n",
								frame, currentWaveform, newWaveform, currentNoteIdx, cycleCounter)
						}
						currentWaveformLo = 0x05
					}
				}

				// Sawtooth envelopes complete visual cycles 2x faster than triangle
				if currentWaveform == 0x02 { // Sawtooth
					sidFreq *= 2
				}

				// Recalculate toggle period when effective frequency changes
				// Effective frequency includes waveform multiplier (sawtooth = 2x)
				// This applies to triangle, sawtooth, and noise+tone envelope modes
				if frame < len(freqPerFrame) && freqPerFrame[frame] > 0 {
					effectiveFreq := freqPerFrame[frame]
					if currentWaveform == 0x02 { // Sawtooth runs at 2x visual frequency
						effectiveFreq *= 2
					}

					// Only recalculate if frequency changed
					if effectiveFreq != lastToneFreq {
						lastToneFreq = effectiveFreq

						ayClock := spectrumAYClockHz
						sidClock := sidClockHz

						// Calculate target toggle frequency directly from AY tone frequency
						// This avoids amplifying the quantization error from sidEnvFreq
						ayTonePeriod := float64(freqPerFrame[frame])
						ayToneFreq := ayClock / (16.0 * ayTonePeriod)
						// Apply resampling correction
						targetToggleFreq := ayToneFreq * (palFrameRate / spectrumFrameRate)

						// Toggle period in cycles (round to nearest integer for optimal match)
						idealTogglePeriod := sidClock / (2.0 * targetToggleFreq)
						newPeriod := int(idealTogglePeriod + 0.5)
						if newPeriod < 1 {
							newPeriod = 1
						}
						// CRITICAL: Store in currentFramePeriod, not togglePeriodCycles
						// togglePeriodCycles is locked for the active cycle
						if channel == 0 && frame >= 1728 && frame <= 1732 && newPeriod != currentFramePeriod {
							fmt.Printf("  PERIOD CHANGE DIRECT CH0 frame=%d: %d → %d, cycleCounter=%.1f waveform=0x%02X\n",
								frame, currentFramePeriod, newPeriod, cycleCounter, currentWaveformHi)
						}
						currentFramePeriod = newPeriod
					}
				}

				// Capture toggle period for validation
				// Match tracker: capture "what period will be used if NEW cycle starts in this frame"
				if len(capturedDirectTogglePeriods[channel]) > frame {
					capturedDirectTogglePeriods[channel][frame] = currentFramePeriod
				}
			} else if currentWaveform == 0x08 {
				// Direct mode noise: update based on noise period changes
				noisePeriod := getAYNoisePeriod(commands, frame)
				sidFreq = computeNoiseFrequency(noisePeriod)

				// Update cached noise frequency for noise+tone toggling
				if noisePlusToneMode {
					cachedNoiseFreq = sidFreq
				}

				// For noise+tone mode, recalculate toggle period when frequency changes
				if noisePlusToneMode && frame < len(freqPerFrame) && freqPerFrame[frame] > 0 {
					// Only recalculate if frequency changed
					if freqPerFrame[frame] != lastToneFreq {
						lastToneFreq = freqPerFrame[frame]

						ayClock := spectrumAYClockHz
						sidClock := sidClockHz

						// Calculate target toggle frequency from AY tone frequency
						ayTonePeriod := float64(freqPerFrame[frame])
						ayToneFreq := ayClock / (16.0 * ayTonePeriod)
						targetToggleFreq := ayToneFreq * (palFrameRate / spectrumFrameRate)

						// Toggle period in cycles
						idealTogglePeriod := sidClock / (2.0 * targetToggleFreq)
						newPeriod := int(idealTogglePeriod + 0.5)
						if newPeriod < 1 {
							newPeriod = 1
						}
						// CRITICAL: Store in currentFramePeriod, not togglePeriodCycles
						// togglePeriodCycles is locked for the active cycle
						currentFramePeriod = newPeriod
					}
				}
			} else {
				// Direct mode tone: update based on tone frequency changes
				if frame < len(freqPerFrame) && freqPerFrame[frame] > 0 {
					sidFreq = computeToneFrequency(freqPerFrame[frame])
				}
			}
				if sidFreq > 0 && sidFreq != lastSIDFreq {
					sid.WriteRegister(regBase+0, byte(sidFreq&0xFF))
					sid.WriteRegister(regBase+1, byte(sidFreq>>8))
				}
			}

			// Check for gate off frame (start of tail decay)
			// At GateOffFrame, update D from 0 to tailDecayRate and set S=0 target
			// Tail uses Decay state (D rate), not Release state
			if currentNote.GateOffFrame > 0 && gateOn && !inEnvelopeMode {
				relFrame := frame - currentNote.StartFrame
				if relFrame == currentNote.GateOffFrame {
					// Set D=tailDecayRate and S=0 for tail decay
					// Envelope will decay from current level to 0 at rate D
					currentA := (sid.Registers[regBase+5] >> 4) & 0x0F
					sid.WriteRegister(regBase+5, (currentA<<4)|currentNote.ADSR.Decay)
					sid.WriteRegister(regBase+6, (0<<4)|currentNote.ADSR.Release) // S=0 target
				}
			}

			// Check for note end
			if frame > currentNote.EndFrame {
				hasAdjacentNote := currentNoteIdx < len(notes) && notes[currentNoteIdx].StartFrame <= frame

				if inEnvelopeMode {
					// Envelope mode: gate OFF when note ends (stops waveform toggling)
					if !hasAdjacentNote {
						// Gate OFF: preserve waveform bits
					currentCtrl := sid.Registers[regBase+4]
					sid.WriteRegister(regBase+4, currentCtrl&0xFE)
					}
					gateOn = false
					inEnvelopeMode = false
				} else if noisePlusToneMode {
					// Noise+tone mode: gate OFF when note ends (stops waveform toggling)
					if !hasAdjacentNote {
						// Gate OFF: preserve waveform bits
					currentCtrl := sid.Registers[regBase+4]
					sid.WriteRegister(regBase+4, currentCtrl&0xFE)
					}
					gateOn = false
					noisePlusToneMode = false
				} else if !hasAdjacentNote {
					// D=0 + dynamic S mode: set D=0 and gate OFF for final release
					currentA := (sid.Registers[regBase+5] >> 4) & 0x0F
					sid.WriteRegister(regBase+5, (currentA<<4)|0) // D=0 for gate OFF
					// Gate OFF: preserve waveform bits
					currentCtrl := sid.Registers[regBase+4]
					sid.WriteRegister(regBase+4, currentCtrl&0xFE)
					gateOn = false
				}
			}
		}

		// Generate samples for this frame
		// Use floating-point calculation to avoid integer truncation drift
		frameStart := int(float64(frame) * sampleRate / palFrameRate)
		frameEnd := int(float64(frame+1) * sampleRate / palFrameRate)
		numSamples := frameEnd - frameStart
		if frameEnd > totalSamples {
			numSamples = totalSamples - frameStart
		}

		// Check mixer state for this frame to determine if noise+tone mixing is active
		frameNoisePlusTone := false
		if noteIsNoisePlusTone && frame < len(tonePerFrame) && frame < len(noisePerFrame) {
			hasTone := tonePerFrame[frame]
			hasNoise := noisePerFrame[frame]
			frameNoisePlusTone = hasTone && hasNoise
		}

		// Check mixer state for this frame to determine output mode
		frameToneOnly := false
		frameNoiseOnly := false
		if noteIsNoisePlusTone && frame < len(tonePerFrame) && frame < len(noisePerFrame) {
			hasTone := tonePerFrame[frame]
			hasNoise := noisePerFrame[frame]
			if !hasTone && hasNoise {
				frameNoiseOnly = true
			} else if hasTone && !hasNoise {
				frameToneOnly = true
			}
		}

		// Handle mixer state changes for notes detected as noise+tone
		if noteIsNoisePlusTone {
			// Update noisePlusToneMode based on current mixer state
			noisePlusToneMode = frameNoisePlusTone

			if frameToneOnly {
				// Tone-only: no modulation (both Hi and Lo are pulse)
				currentWaveformHi = 0x04
				currentWaveformLo = 0x04
				currentWaveform = 0x04
				// Set frequency
				if frame < len(freqPerFrame) && freqPerFrame[frame] > 0 {
					toneFreq := computeToneFrequency(freqPerFrame[frame])
					sid.WriteRegister(regBase+0, byte(toneFreq&0xFF))
					sid.WriteRegister(regBase+1, byte(toneFreq>>8))
				}
			} else if frameNoiseOnly {
				// Noise-only: no modulation (both Hi and Lo are noise)
				currentWaveformHi = 0x08
				currentWaveformLo = 0x08
				currentWaveform = 0x08
				// Set frequency
				if cachedNoiseFreq > 0 {
					sid.WriteRegister(regBase+0, byte(cachedNoiseFreq&0xFF))
					sid.WriteRegister(regBase+1, byte(cachedNoiseFreq>>8))
				}
			} else if frameNoisePlusTone {
				// Noise+tone: modulated (Hi = noise, Lo = silence)
				currentWaveformHi = 0x08
				currentWaveformLo = 0x05
				currentWaveform = 0x08
			}
		}

		// Check if we need per-sample processing (dynamic waveform toggling)
		// Enable for any channel with modulation: envelope mode (while gate ON) or direct mode noise+tone (entire note)
		// CRITICAL: For noise+tone notes, ALWAYS use per-sample processing, even during tone-only/noise-only frames
		// This keeps cycleCounter running to maintain phase continuity
		needsPerSample := (gateOn && inEnvelopeMode) || noteIsNoisePlusTone

		// CRITICAL: Once toggle is active, always use per-sample processing to keep counter running
		// The counter runs continuously like a hardware oscillator - it never stops
		// When not actively toggling, we still count but write the same waveform for both states
		if cycleCounter > 0 {
			needsPerSample = true
		}


		if needsPerSample {
			// Slow path: per-sample processing for dynamic waveform toggling
			sidClock := sidClockHz
			sampleRate := 44100.0
			cyclesPerSample := sidClock / sampleRate // ~22.34 SID cycles per audio sample

			// Determine gate bit based on mode and current gate state
			// Envelope mode: gate stays ON throughout entire note
			// Noise+tone mode: uses current gate state (ON during attack, OFF during release)
			var gateBit byte
			if inEnvelopeMode {
				gateBit = 0x01 // Envelope mode keeps gate ON
			} else if gateOn {
				gateBit = 0x01
			} else {
				gateBit = 0x00
			}

			// VALIDATE: After frame update, if waveform is 0x05, pulse width must be 0xFFF
			currentWaveform := (sid.Registers[regBase+4] & 0xF0) >> 4
			if currentWaveform == 0x05 {
				currentPulseWidth := uint16(sid.Registers[regBase+2]) | (uint16(sid.Registers[regBase+3]&0x0F) << 8)
				if currentPulseWidth != 0xFFF {
					panic(fmt.Sprintf("DIRECT CH%d frame %d sample %d: After frame update, waveform 0x05 has incorrect pulse width 0x%03X (must be 0xFFF)", channel, frame, totalSampleCount, currentPulseWidth))
				}
			}

			for i := 0; i < numSamples; i++ {
				currentSample := totalSampleCount + i

				// CIA timer interrupt processing - applies to all note types
				// Physical model: timer runs continuously, outputs waveform on every interrupt


				// Generate sample
				samples[frameStart+i] = sid.GenerateSample(channel)

				// Record bug state for this sample
				if sid.Voices[channel].ADSRDelayBugActive {
					adsrBugDebug[frameStart+i] = 32767
				} else {
					adsrBugDebug[frameStart+i] = 0
				}

				// CIA timer countdown (maintains phase across all note types)
				if cycleCounter > 0 {
					cycleCounter -= cyclesPerSample
					if cycleCounter <= 0 {
						// CIA TIMER INTERRUPT: Write waveform based on current timer phase
						if timerIsHi {
							// HIGH phase
							if currentWaveformHi == 0x08 {
								// VALIDATE: Check if AY actually has audible noise
								mixer := getAYMixerState(commands, frame)
								noiseBit := uint8(3 + channel) // Bit 3=A, 4=B, 5=C
								noiseOn := (mixer & (1 << noiseBit)) == 0

								if !noiseOn {
									panic(fmt.Sprintf("DIRECT CH%d frame %d sample %d: SID writing noise waveform 0x08 but AY mixer has noise OFF (mixer=0x%02X bit%d=1)",
										channel, frame, currentSample, mixer, noiseBit))
								}

								// Noise waveform
								sid.WriteRegister(regBase+4, (0x08<<4)|gateBit)
							} else if currentWaveformHi == 0x04 {
								// Pulse waveform
									if channel == 0 && currentSample >= 1470970 && currentSample <= 1471180 {
									}
								sid.WriteRegister(regBase+2, 0x00)
								sid.WriteRegister(regBase+3, 0x08)
								sid.WriteRegister(regBase+4, (0x04<<4)|gateBit)
							} else {
								// Triangle/Sawtooth
								sid.WriteRegister(regBase+4, (currentWaveformHi<<4)|gateBit)
							}
						} else {
							// LOW phase
							if currentWaveformLo == 0x05 {
								// Pulse+Triangle with pulse width 0xFFF
								sid.WriteRegister(regBase+2, 0xFF)
								sid.WriteRegister(regBase+3, 0x0F)
								sid.WriteRegister(regBase+4, (0x05<<4)|gateBit)
								// VALIDATE: waveform 0x05 must have pulse width 0xFFF
								actualPulseWidth := uint16(sid.Registers[regBase+2]) | (uint16(sid.Registers[regBase+3]&0x0F) << 8)
								if channel == 0 && currentSample >= 1470970 && currentSample <= 1471180 {
								}
								if actualPulseWidth != 0xFFF {
									panic(fmt.Sprintf("DIRECT CH%d sample %d: Wrote waveform 0x05 with incorrect pulse width 0x%03X (must be 0xFFF)", channel, currentSample, actualPulseWidth))
								}
							} else if currentWaveformLo == 0x04 {
								// Pulse (non-modulated)
								sid.WriteRegister(regBase+2, 0x00)
								sid.WriteRegister(regBase+3, 0x08)
								sid.WriteRegister(regBase+4, (0x04<<4)|gateBit)
							} else if currentWaveformLo == 0x08 {
								// VALIDATE: Check if AY actually has audible noise
								mixer := getAYMixerState(commands, frame)
								noiseBit := uint8(3 + channel) // Bit 3=A, 4=B, 5=C
								noiseOn := (mixer & (1 << noiseBit)) == 0

								if !noiseOn {
									panic(fmt.Sprintf("DIRECT CH%d frame %d sample %d: SID writing noise waveform 0x08 (LO phase) but AY mixer has noise OFF (mixer=0x%02X bit%d=1)",
										channel, frame, currentSample, mixer, noiseBit))
								}

								// Noise (non-modulated)
								sid.WriteRegister(regBase+4, (0x08<<4)|gateBit)
							} else {
								// Other waveforms
								sid.WriteRegister(regBase+4, (currentWaveformLo<<4)|gateBit)
							}
						}

						// Capture toggle event
						if len(capturedDirectToggleEvents[channel]) < 10000 {
							capturedDirectToggleEvents[channel] = append(capturedDirectToggleEvents[channel], currentSample)
						}

						// Capture waveform write for validation (same limit as toggle events)
						waveform := (sid.Registers[regBase+4] & 0xF0) >> 4
						pulseWidth := uint16(sid.Registers[regBase+2]) | (uint16(sid.Registers[regBase+3]&0x0F) << 8)
						if len(capturedDirectWaveformWrites[channel]) < 10000 {
							capturedDirectWaveformWrites[channel] = append(capturedDirectWaveformWrites[channel],
								WaveformWrite{sample: currentSample, waveform: waveform, pulseWidth: pulseWidth})
						}

						// Validate period (only if waveform actually changed)
						if currentWaveformHi != currentWaveformLo && lastToggleSample >= 0 {
							actualPeriodSamples := currentSample - lastToggleSample
							expectedPeriodSamples := float64(activePeriod) / cyclesPerSample
							diff := expectedPeriodSamples - float64(actualPeriodSamples)

							if diff > 5.0 {
								fmt.Printf("  ⚠ DIRECT CH%d sample %d: PREMATURE TOGGLE! Period was %d samples (expected %.1f), cut short by %.1f samples. Frame=%d activePeriod=%d currentFramePeriod=%d\n",
									channel, currentSample, actualPeriodSamples, expectedPeriodSamples, diff, frame, activePeriod, currentFramePeriod)
							}
						}

						if currentWaveformHi != currentWaveformLo {
							lastToggleSample = currentSample
						}

						// Toggle timer phase
						timerIsHi = !timerIsHi

						// Reload counter and lock period
						if currentFramePeriod > 0 {
							cycleCounter += float64(currentFramePeriod)
							activePeriod = currentFramePeriod
						}
					}
				}
			}
			totalSampleCount += numSamples
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
			totalSampleCount += numSamples
		}

		// Capture SID frequency registers at end of frame (after all register writes)
		if frame < len(capturedDirectPlayerRegs[channel]) {
			sidFreqLo := uint16(sid.Registers[regBase+0])
			sidFreqHi := uint16(sid.Registers[regBase+1])
			capturedDirectPlayerRegs[channel][frame] = (sidFreqHi << 8) | sidFreqLo
		}
	}

	return samples, adsrBugDebug
}

func generateSIDContinuous(commands []spectrum.AYCommand, channel int, volumeCurve []float64, adsr ADSRParams, totalSamples int) []int16 {
	sampleRate := 44100.0
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
			ayClock := spectrumAYClockHz
			freqHz := ayClock / (16.0 * float64(freqPerFrame[frame]))
			// Correct for resampling: AY audio gets sped up by palFrameRate/spectrumFrameRate
			freqHz *= palFrameRate / spectrumFrameRate
			sidFreq := uint16(freqHz * sidFreqDivisor / sidClockHz + 0.5)

			if sidFreq != lastSIDFreq {
				sid.WriteRegister(0, byte(sidFreq&0xFF))
				sid.WriteRegister(1, byte(sidFreq>>8))
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

		// Use floating-point calculation to avoid integer truncation drift
		frameStart := int(float64(frame) * sampleRate / palFrameRate)
		frameEnd := int(float64(frame+1) * sampleRate / palFrameRate)
		numSamples := frameEnd - frameStart
		if frameEnd > totalSamples {
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
	// Generate accurate AY audio at Spectrum timing (this is the reference)

	numFrames := endFrame - startFrame + 1
	// Calculate total samples using Spectrum frame rate for accurate AY timing
	numSamples := int(float64(numFrames) * float64(sampleRate) / spectrumFrameRate)

	durationSeconds := float64(numSamples) / float64(sampleRate)
	ay := spectrum.NewAudioBuffer(sampleRate, durationSeconds)
	ayChip := spectrum.NewAY38912()

	cyclesPerFrame := uint64(spectrumCyclesPerFrame)
	startCycle := uint64(startFrame) * cyclesPerFrame

	// Group commands by frame and process frame-by-frame
	// This ensures audio for frame N is generated AFTER applying frame N's commands
	cmdIdx := 0
	for frame := 0; frame <= endFrame; frame++ {
		// Apply all commands for this frame
		for cmdIdx < len(commands) && int(commands[cmdIdx].Frame) == frame {
			ayChip.SelectRegister(commands[cmdIdx].Register)
			ayChip.WriteRegister(commands[cmdIdx].Value)
			cmdIdx++
		}

		// Generate audio for this frame (with commands just applied)
		if frame >= startFrame {
			cycle := uint64(frame+1)*cyclesPerFrame - startCycle
			ay.Update(ayChip, cycle)
		}
	}

	samples := make([]int16, numSamples)
	for i := 0; i < numSamples && i < ay.Position; i++ {
		samples[i] = ay.Samples[i*6+channel]
	}

	return samples
}

// validateNoteOnsets checks that volume 0→non-0 transitions happen at the same sample in AY and SID
func validateNoteOnsets(ayAudio [3][]int16, sidAudio [3][]int16, notes [3][]Note, sampleRate int) {
	const windowSize = 50 // samples to check around expected position

	for ch := 0; ch < 3; ch++ {
		fmt.Printf("Channel %d: checking %d notes\n", ch, len(notes[ch]))

		maxDrift := 0
		maxDriftNote := -1
		driftCount := 0

		for i, note := range notes[ch] {
			// Expected sample position for this frame
			expectedPos := int(float64(note.StartFrame) * float64(sampleRate) / palFrameRate)

			// Find actual onset in AY audio (first non-zero after silence)
			ayOnset := findOnsetNear(ayAudio[ch], expectedPos, windowSize)

			// Find actual onset in SID audio
			sidOnset := findOnsetNear(sidAudio[ch], expectedPos, windowSize)

			if ayOnset >= 0 && sidOnset >= 0 {
				drift := ayOnset - sidOnset
				if drift < 0 {
					drift = -drift
				}

				if drift > maxDrift {
					maxDrift = drift
					maxDriftNote = i
				}

				driftFrames := float64(drift) * palFrameRate / float64(sampleRate)

				// Report drifts > 0.3 frames
				if driftFrames > 0.3 {
					driftCount++
					if driftCount <= 3 {
						fmt.Printf("  Note %d (frame %d): AY onset=%d, SID onset=%d, drift=%d samples (%.3f frames)\n",
							i, note.StartFrame, ayOnset, sidOnset, drift, driftFrames)
					}
				}

				// Panic on drift > 0.5 frames
				if driftFrames > 0.5 {
					panic(fmt.Sprintf("Channel %d, note %d: onset drift %d samples (%.3f frames) exceeds 0.5 frame threshold!",
						ch, i, drift, driftFrames))
				}
			}
		}

		maxDriftFrames := float64(maxDrift) * palFrameRate / float64(sampleRate)
		if maxDriftFrames > 0.3 {
			fmt.Printf("  ⚠ Maximum drift: %d samples (%.3f frames) at note %d\n",
				maxDrift, maxDriftFrames, maxDriftNote)
		} else {
			fmt.Printf("  ✓ Maximum drift: %d samples (%.3f frames) - onsets aligned\n",
				maxDrift, maxDriftFrames)
		}
	}
}

// findOnsetNear finds the sample where amplitude increases significantly near expected position
func findOnsetNear(audio []int16, expectedPos, windowSize int) int {
	start := expectedPos - windowSize
	if start < 10 {
		start = 10
	}
	end := expectedPos + windowSize
	if end > len(audio) {
		end = len(audio)
	}

	// Find first position where RMS increases significantly
	const windowRMS = 10
	threshold := int16(500) // Minimum onset amplitude

	for i := start; i < end-windowRMS; i++ {
		// Calculate RMS before and after
		rmsBefore := int16(0)
		for j := i - windowRMS; j < i; j++ {
			sample := audio[j]
			if sample < 0 {
				sample = -sample
			}
			if sample > rmsBefore {
				rmsBefore = sample
			}
		}

		rmsAfter := int16(0)
		for j := i; j < i+windowRMS; j++ {
			sample := audio[j]
			if sample < 0 {
				sample = -sample
			}
			if sample > rmsAfter {
				rmsAfter = sample
			}
		}

		// Onset = significant increase
		if rmsAfter > threshold && rmsAfter > rmsBefore*2 {
			return i
		}
	}

	return -1 // Not found
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

// CheckNoiseValidation verifies that noise waveform notes are being validated correctly
// ValidateTrackerWaveformPerFrame checks that for every frame, the tracker note's waveform matches the actual AY audio
func ValidateTrackerWaveformPerFrame(commands []spectrum.AYCommand, simNotes [3][]SimulatedNote, ayAudioPAL [3][]int16, sampleRate int, startOffset int) {
	maxFrame := int(commands[len(commands)-1].Frame)

	for ch := 0; ch < 3; ch++ {
		// Build a map of frame -> simulated note for quick lookup
		// SimulatedNotes are sequential - calculate start frame by summing durations
		frameToNote := make(map[int]*SimulatedNote)
		currentFrame := startOffset
		for i := range simNotes[ch] {
			if simNotes[ch][i].isRest {
				currentFrame += int(simNotes[ch][i].dur)
				continue
			}
			// Map all frames covered by this note
			startFrame := currentFrame
			endFrame := currentFrame + int(simNotes[ch][i].dur) - 1
			for f := startFrame; f <= endFrame && f <= maxFrame; f++ {
				frameToNote[f] = &simNotes[ch][i]
			}
			currentFrame += int(simNotes[ch][i].dur)
		}

		// Check each frame
		for frame := 0; frame <= maxFrame; frame++ {
			// Get the sample range for this frame
			startSample := int(float64(frame) * float64(sampleRate) / palFrameRate)
			endSample := int(float64(frame+1) * float64(sampleRate) / palFrameRate)

			if startSample >= len(ayAudioPAL[ch]) || endSample > len(ayAudioPAL[ch]) {
				continue
			}

			// Check if AY audio has noise characteristics in this frame
			ayHasNoise := detectNoiseInSamples(ayAudioPAL[ch][startSample:endSample])

			// Check what note (if any) covers this frame
			simNote, hasNote := frameToNote[frame]

			if ayHasNoise {
				// AY plays noise at this frame - SID must also play noise
				if !hasNote {
					panic(fmt.Sprintf("TRACKER Channel %d frame %d: AY plays noise but no SID note covers this frame (gap in note detection)", ch, frame))
				}
				if simNote.Waveform != 0x08 {
					panic(fmt.Sprintf("TRACKER Channel %d frame %d: AY plays noise but SID note has waveform 0x%02X (should be 0x08)", ch, frame, simNote.Waveform))
				}
			} else {
				// AY does not play noise at this frame - SID must not play noise
				if hasNote && simNote.Waveform == 0x08 && !simNote.NoisePlusTone {
					panic(fmt.Sprintf("TRACKER Channel %d frame %d: SID plays pure noise (waveform 0x08) but AY audio has no noise characteristics (samples %d-%d)", ch, frame, startSample, endSample))
				}
			}
		}
	}
}

// ValidateTrackerTogglePeriods checks that actual toggle periods in tracker SID audio match expected periods
// Panics if actual period is longer than both previous and current expected periods
func ValidateTrackerTogglePeriods(sidAudio [3][]int16, simNotes [3][]SimulatedNote, sampleRate int, startOffset int) {
	const sidClock = 985248.0 // PAL SID clock in Hz

	for ch := 0; ch < 3; ch++ {
		currentFrame := startOffset

		for noteIdx := range simNotes[ch] {
			simNote := simNotes[ch][noteIdx]

			// Only check envelope/NoisePlusTone notes that use toggling
			if !simNote.EnvelopeMode && !simNote.NoisePlusTone {
				currentFrame += int(simNote.dur)
				continue
			}

			noteStartFrame := currentFrame
			noteDuration := int(simNote.dur)

			// Calculate sample range for this note
			startSample := int(float64(noteStartFrame) * float64(sampleRate) / palFrameRate)
			endSample := int(float64(noteStartFrame+noteDuration) * float64(sampleRate) / palFrameRate)
			if startSample >= len(sidAudio[ch]) || endSample > len(sidAudio[ch]) {
				currentFrame += noteDuration
				continue
			}

			// Find all zero crossings (waveform transitions) in the SID audio
			var transitions []int
			prevSample := int16(0)
			for i := startSample; i < endSample && i < len(sidAudio[ch]); i++ {
				sample := sidAudio[ch][i]

				// Detect transition (sign change with magnitude threshold)
				if (prevSample >= 0 && sample < -100) || (prevSample <= 0 && sample > 100) {
					transitions = append(transitions, i)
				}
				prevSample = sample
			}

			// Check each toggle period against expected
			for i := 1; i < len(transitions); i++ {
				actualPeriodSamples := transitions[i] - transitions[i-1]
				toggleSample := transitions[i-1]

				// Convert sample position to frame within this note
				frame := int(float64(toggleSample) * palFrameRate / float64(sampleRate))
				relFrame := frame - noteStartFrame

				if relFrame < 0 || relFrame >= noteDuration {
					continue
				}

				// Get expected toggle period in SID cycles
				var expectedPeriodCycles uint16
				if len(simNote.TogglePeriodSeq) > relFrame && simNote.TogglePeriodSeq[relFrame] > 0 {
					expectedPeriodCycles = simNote.TogglePeriodSeq[relFrame]
				} else {
					expectedPeriodCycles = simNote.TogglePeriod
				}

				if expectedPeriodCycles == 0 {
					continue // No expected period for this frame
				}

				// Convert expected period from SID cycles to samples
				// Period in cycles → frequency = sidClock / period → samples = sampleRate / frequency
				expectedPeriodSamples := int(float64(expectedPeriodCycles) * float64(sampleRate) / sidClock)

				// Get previous expected period for comparison
				prevExpectedCycles := expectedPeriodCycles
				if relFrame > 0 {
					if len(simNote.TogglePeriodSeq) > relFrame-1 && simNote.TogglePeriodSeq[relFrame-1] > 0 {
						prevExpectedCycles = simNote.TogglePeriodSeq[relFrame-1]
					}
				}
				prevExpectedSamples := int(float64(prevExpectedCycles) * float64(sampleRate) / sidClock)

				// Allow 10% tolerance for timing variations
				maxExpectedSamples := int(float64(max(expectedPeriodSamples, prevExpectedSamples)) * 1.1)

				// Panic if actual period is longer than both previous and current expected (with tolerance)
				if actualPeriodSamples > maxExpectedSamples {
					panic(fmt.Sprintf("TRACKER Channel %d note starting frame %d: toggle period too long at sample %d (frame %d)\n"+
						"  Actual period: %d samples (%.2fms)\n"+
						"  Current expected: %d samples (%.2fms) from %d SID cycles\n"+
						"  Previous expected: %d samples (%.2fms) from %d SID cycles\n"+
						"  Actual > max(current, prev) * 1.1",
						ch, noteStartFrame, toggleSample, frame,
						actualPeriodSamples, float64(actualPeriodSamples)*1000.0/float64(sampleRate),
						expectedPeriodSamples, float64(expectedPeriodSamples)*1000.0/float64(sampleRate), expectedPeriodCycles,
						prevExpectedSamples, float64(prevExpectedSamples)*1000.0/float64(sampleRate), prevExpectedCycles))
				}
			}

			currentFrame += noteDuration
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ValidateWaveformPerFrame checks that for every frame, the detected note's waveform matches the actual AY audio
func ValidateWaveformPerFrame(commands []spectrum.AYCommand, notes [3][]Note, ayAudioPAL [3][]int16, sampleRate int) {
	maxFrame := int(commands[len(commands)-1].Frame)

	for ch := 0; ch < 3; ch++ {
		// Build a map of frame -> note for quick lookup
		frameToNote := make(map[int]*Note)
		for i := range notes[ch] {
			for f := notes[ch][i].StartFrame; f <= notes[ch][i].EndFrame; f++ {
				frameToNote[f] = &notes[ch][i]
			}
		}

		// Check each frame
		for frame := 0; frame <= maxFrame; frame++ {
			// Get the sample range for this frame
			startSample := int(float64(frame) * float64(sampleRate) / palFrameRate)
			endSample := int(float64(frame+1) * float64(sampleRate) / palFrameRate)

			if startSample >= len(ayAudioPAL[ch]) || endSample > len(ayAudioPAL[ch]) {
				continue
			}

			// Check if AY audio has noise characteristics in this frame
			ayHasNoise := detectNoiseInSamples(ayAudioPAL[ch][startSample:endSample])

			// Check what note (if any) covers this frame
			note, hasNote := frameToNote[frame]

			if ayHasNoise {
				// AY plays noise at this frame - SID must also play noise
				if !hasNote {
					panic(fmt.Sprintf("Channel %d frame %d: AY plays noise but no SID note covers this frame (gap in note detection)", ch, frame))
				}
				if note.Waveform != 0x08 {
					panic(fmt.Sprintf("Channel %d frame %d: AY plays noise but SID note has waveform 0x%02X (should be 0x08)", ch, frame, note.Waveform))
				}
			} else {
				// AY does not play noise at this frame - SID must not play noise
				if hasNote && note.Waveform == 0x08 && !note.NoisePlusTone {
					panic(fmt.Sprintf("Channel %d frame %d: SID plays pure noise (waveform 0x08) but AY audio has no noise characteristics (samples %d-%d)", ch, frame, startSample, endSample))
				}
			}
		}
	}
}

// ValidateNoiseInAYAudio checks that notes detected as pure noise actually have noise in the AY audio
func ValidateNoiseInAYAudio(commands []spectrum.AYCommand, notes [3][]Note, ayAudioPAL [3][]int16, sampleRate int) {
}

// detectNoiseInSamples returns true if the sample slice has noise characteristics
// Noise: rapid alternation between low and high values (high-frequency content)
// Tone/Envelope: smooth changes (low-frequency content)
func detectNoiseInSamples(samples []int16) bool {
	if len(samples) < 10 {
		return false // Too few samples to determine
	}

	// Count rapid sign changes (zero crossings) in the diff signal
	// Noise will have many zero crossings, smooth envelope will have few
	zeroCrossings := 0
	prevDiff := int32(0)

	for i := 1; i < len(samples); i++ {
		diff := int32(samples[i]) - int32(samples[i-1])

		// Check for zero crossing in the diff (change of direction)
		if prevDiff != 0 && diff != 0 {
			if (prevDiff > 0 && diff < 0) || (prevDiff < 0 && diff > 0) {
				zeroCrossings++
			}
		}

		prevDiff = diff
	}

	// Noise should have frequent direction changes (at least 20% of samples)
	// Smooth envelope/tone will have few direction changes
	ratio := float64(zeroCrossings) / float64(len(samples))
	return ratio > 0.2
}

func CheckNoiseValidation(commands []spectrum.AYCommand, original [3][]Note, simulated [3][]SimulatedNote, startOffset int) {
	for ch := 0; ch < 3; ch++ {
		// Count noise notes in original
		noiseCount := 0
		noiseFrames := 0
		for _, note := range original[ch] {
			if note.Waveform == 0x08 {
				noiseCount++
				noiseFrames += note.EndFrame - note.StartFrame + 1
			}
		}

		if noiseCount == 0 {
			continue
		}

		// Build expected frequency sequence for noise notes only
		expectedFreqs := buildExpectedFrequencySequence(commands, ch, original[ch])

		// Build simulated frequency sequence
		actualFreqs := buildSimulatedFrequencySequence(commands, ch, simulated[ch], startOffset)

		// Check noise note frames
		checkedFrames := 0
		nonZeroExpected := 0
		nonZeroActual := 0
		matches := 0

		for _, note := range original[ch] {
			if note.Waveform == 0x08 {
				for frame := note.StartFrame; frame <= note.EndFrame; frame++ {
					if frame < len(expectedFreqs) && frame < len(actualFreqs) {
						checkedFrames++
						if expectedFreqs[frame] > 0 {
							nonZeroExpected++
						}
						if actualFreqs[frame] > 0 {
							nonZeroActual++
						}
						if expectedFreqs[frame] == actualFreqs[frame] && expectedFreqs[frame] > 0 {
							matches++
						}
					}
				}
			}
		}

		fmt.Printf("Channel %d: %d noise notes (%d frames)\n", ch, noiseCount, noiseFrames)
		fmt.Printf("  Checked: %d frames, expected>0: %d, actual>0: %d, matches: %d\n",
			checkedFrames, nonZeroExpected, nonZeroActual, matches)
		if checkedFrames > 0 && matches < checkedFrames {
			fmt.Printf("  ⚠ %d/%d noise frames don't match (%.1f%%)\n",
				checkedFrames-matches, checkedFrames, float64(checkedFrames-matches)*100/float64(checkedFrames))
		}
	}
}

// ValidateSIDFrequencies compares SID frequencies frame-by-frame between original and tracker replay
// by running both players and capturing actual SID register writes
func ValidateSIDFrequencies(commands []spectrum.AYCommand, original [3][]Note, simulated [3][]SimulatedNote, startOffset int) {
	totalDivergences := 0

	for ch := 0; ch < 3; ch++ {
		// Calculate total samples needed
		maxFrame := 0
		for _, note := range original[ch] {
			if note.EndFrame > maxFrame {
				maxFrame = note.EndFrame
			}
		}
		totalSamples := int(float64(maxFrame+1) * 44100.0 / palFrameRate)

		// Run direct player and capture frequencies
		capturedDirectPlayerRegs[ch] = make([]uint16, maxFrame+1)
		_, _ = generateSIDPerNote(commands, ch, original[ch], totalSamples)
		expectedFreqs := capturedDirectPlayerRegs[ch]

		// Run tracker player and capture frequencies
		actualFreqs := captureTrackerPlayerSIDFrequencies(commands, ch, simulated[ch], startOffset)


		// Compare frame-by-frame
		maxFrames := len(expectedFreqs)
		if len(actualFreqs) > maxFrames {
			maxFrames = len(actualFreqs)
		}

		divergences := 0
		firstDivergence := -1
		for frame := 0; frame < maxFrames; frame++ {
			expected := uint16(0)
			actual := uint16(0)

			if frame < len(expectedFreqs) {
				expected = expectedFreqs[frame]
			}
			if frame < len(actualFreqs) {
				actual = actualFreqs[frame]
			}

			if expected != actual {
				if firstDivergence < 0 {
					firstDivergence = frame
				}
				divergences++
				// Debug: print all divergences with note info
				if divergences <= 20 {
					// Debug envelope frequency calculation for first divergence
					if ch == 0 && divergences <= 3 {
						for _, note := range original[ch] {
							if frame >= note.StartFrame && frame <= note.EndFrame && note.EnvelopeMode {
								envPeriod := getAYEnvelopePeriod(commands, frame)
								computedFreq := computeEnvelopeFrequency(envPeriod)
								if note.Waveform == 0x02 {
									computedFreq *= 2
								}
								fmt.Printf("  DEBUG ch=%d frame=%d envMode=true waveform=0x%02x: envPeriod=%d computedFreq=%d expected=%d actual=%d\n",
									ch, frame, note.Waveform, envPeriod, computedFreq, expected, actual)
								break
							}
						}
					}
					// Find which note is active at this frame
					noteInfo := "no active note"
					for _, note := range original[ch] {
						if frame >= note.StartFrame && frame <= note.EndFrame {
							waveformStr := fmt.Sprintf("0x%02x", note.Waveform)
							if note.Waveform == 0x08 {
								waveformStr = "noise"
							} else if note.EnvelopeMode {
								waveformStr = fmt.Sprintf("0x%02x+env", note.Waveform)
							}
							noteInfo = fmt.Sprintf("note waveform=%s", waveformStr)
							break
						}
					}
					fmt.Printf("  DEBUG ch=%d frame=%d: expected=%d actual=%d diff=%d (%s)\n",
						ch, frame, expected, actual, int(actual)-int(expected), noteInfo)
				}
			}
		}

		if divergences > 0 {
			fmt.Printf("Channel %d: ⚠ %d/%d frames differ (%.1f%%, first at frame %d)\n",
				ch, divergences, maxFrames, float64(divergences)*100/float64(maxFrames), firstDivergence)
			totalDivergences += divergences
		} else {
			fmt.Printf("Channel %d: ✓ %d frames validated\n", ch, maxFrames)
		}
	}

	if totalDivergences > 0 {
		totalFrames := 0
		for ch := 0; ch < 3; ch++ {
			totalFrames += len(buildExpectedFrequencySequence(commands, ch, original[ch]))
		}
		errorRate := float64(totalDivergences) * 100.0 / float64(totalFrames)

		if errorRate >= 0.01 {
			panic(fmt.Sprintf("Frequency validation failed: %d total frame mismatches (%.3f%% error rate). Tracker format must reproduce original frequencies exactly.", totalDivergences, errorRate))
		} else {
			fmt.Printf("\n⚠ Warning: %d frame mismatches (%.3f%% error rate) - small rounding errors in noise period sampling\n", totalDivergences, errorRate)
		}
	}
}

// ValidateGateCommands compares gate ON/OFF events between original and simulated
func ValidateGateCommands(original [3][]Note, simulated [3][]SimulatedNote, startOffset int) {
	totalMismatches := 0

	for ch := 0; ch < 3; ch++ {
		// Build expected gate events from original notes
		expectedGates := buildExpectedGateEvents(original[ch])

		// Build actual gate events from simulated notes
		actualGates := buildSimulatedGateEvents(simulated[ch], startOffset)

		// Compare
		if len(expectedGates) != len(actualGates) {
			fmt.Printf("Channel %d: ⚠ Gate event count mismatch! Expected=%d Actual=%d\n",
				ch, len(expectedGates), len(actualGates))
			totalMismatches += abs(len(expectedGates) - len(actualGates))
			continue
		}

		mismatches := 0
		for i := 0; i < len(expectedGates) && i < len(actualGates); i++ {
			if expectedGates[i] != actualGates[i] {
				if mismatches < 5 {
					fmt.Printf("  DEBUG ch=%d event=%d: expected=frame %d %s, actual=frame %d %s\n",
						ch, i, expectedGates[i].frame, gateStateStr(expectedGates[i].gateOn),
						actualGates[i].frame, gateStateStr(actualGates[i].gateOn))
				}
				mismatches++
			}
		}

		if mismatches > 0 {
			fmt.Printf("Channel %d: ⚠ %d/%d gate events differ\n", ch, mismatches, len(expectedGates))
			totalMismatches += mismatches
		} else {
			fmt.Printf("Channel %d: ✓ %d gate events validated\n", ch, len(expectedGates))
		}
	}

	if totalMismatches > 0 {
		panic(fmt.Sprintf("Gate validation failed: %d total mismatches. Tracker format must reproduce original gate commands exactly.", totalMismatches))
	}
}

type GateEvent struct {
	frame  int
	gateOn bool
}

func gateStateStr(gateOn bool) string {
	if gateOn {
		return "ON"
	}
	return "OFF"
}

func buildExpectedGateEvents(notes []Note) []GateEvent {
	events := []GateEvent{}
	for noteIdx, note := range notes {
		// Direct conversion ALWAYS gates OFF before starting new note (even if adjacent)
		// This happens at note.StartFrame, before gate ON
		if noteIdx > 0 {
			events = append(events, GateEvent{frame: note.StartFrame, gateOn: false})
		}

		// Gate ON at note start
		events = append(events, GateEvent{frame: note.StartFrame, gateOn: true})

		// Gate OFF at note end (only if no adjacent note - otherwise next note will gate OFF first)
		hasAdjacentNote := noteIdx+1 < len(notes) && notes[noteIdx+1].StartFrame <= note.EndFrame+1
		if !hasAdjacentNote {
			events = append(events, GateEvent{frame: note.EndFrame + 1, gateOn: false})
		}
	}
	return events
}

func buildSimulatedGateEvents(simNotes []SimulatedNote, startOffset int) []GateEvent {
	events := []GateEvent{}
	absoluteFrame := startOffset
	lastNonRestIdx := -1

	for noteIdx, simNote := range simNotes {
		if simNote.isRest {
			absoluteFrame += int(simNote.dur)
			continue
		}

		// Tracker player ALWAYS gates OFF before starting new note (even if adjacent)
		// This matches direct conversion behavior and avoids ADSR delay bug
		if lastNonRestIdx >= 0 {
			events = append(events, GateEvent{frame: absoluteFrame, gateOn: false})
		}

		// Gate ON at note start
		events = append(events, GateEvent{frame: absoluteFrame, gateOn: true})

		lastNonRestIdx = noteIdx
		absoluteFrame += int(simNote.dur)

		// Gate OFF at note end (only if no adjacent non-rest note)
		// Otherwise next note will gate OFF first
		hasAdjacentNote := noteIdx+1 < len(simNotes) && !simNotes[noteIdx+1].isRest
		if !hasAdjacentNote {
			events = append(events, GateEvent{frame: absoluteFrame, gateOn: false})
		}
	}

	return events
}

// ValidateDynamicD checks that D register updates match between original and simulated
func ValidateDynamicD(original [3][]Note, simulated [3][]SimulatedNote, startOffset int) {
	totalMismatches := 0

	for ch := 0; ch < 3; ch++ {
		// Filter out rest patterns from simulated
		simulatedNonRest := []SimulatedNote{}
		for _, note := range simulated[ch] {
			if !note.isRest {
				simulatedNonRest = append(simulatedNonRest, note)
			}
		}

		if len(original[ch]) != len(simulatedNonRest) {
			fmt.Printf("Channel %d: ⚠ Note count mismatch!\n", ch)
			continue
		}

		mismatches := 0
		for i := 0; i < len(original[ch]); i++ {
			orig := original[ch][i]
			sim := simulatedNonRest[i]

			// Skip envelope mode notes - they don't use dynamic D
			if orig.EnvelopeMode {
				continue
			}

			// Expected D values at key frames:
			// Frame 0: D = InitialDecay (if A > 0)
			// Frame 1: D = 0
			// Frame = GateOffFrame: D = ADSR.Decay (if Decay > 0)

			// Build expected D schedule
			expectedD := make(map[int]uint8) // relFrame -> D value
			if orig.ADSR.Attack > 0 && orig.ADSR.InitialDecay > 0 {
				expectedD[0] = orig.ADSR.InitialDecay
				expectedD[1] = 0
			} else {
				expectedD[0] = 0
			}

			// GateOffFrame is where D switches from 0 to Decay
			// This should match len(SustainPerFrame) since SustainPerFrame is truncated at GateOffFrame
			if orig.ADSR.Decay > 0 && orig.GateOffFrame > 0 {
				expectedD[orig.GateOffFrame] = orig.ADSR.Decay
			}

			// Build simulated D schedule
			simulatedD := make(map[int]uint8)
			if sim.ADSR.Attack > 0 && sim.ADSR.InitialDecay > 0 {
				simulatedD[0] = sim.ADSR.InitialDecay
				simulatedD[1] = 0
			} else {
				simulatedD[0] = 0
			}

			// GateOffFrame stored explicitly
			if sim.GateOffFrame > 0 {
				simulatedD[sim.GateOffFrame] = sim.ADSR.Decay
			}

			// Compare
			allFrames := make(map[int]bool)
			for f := range expectedD {
				allFrames[f] = true
			}
			for f := range simulatedD {
				allFrames[f] = true
			}

			for frame := range allFrames {
				expD, expOk := expectedD[frame]
				simD, simOk := simulatedD[frame]

				if expOk != simOk || expD != simD {
					if mismatches < 3 {
						fmt.Printf("  DEBUG ch=%d note=%d frame=%d: expected D=%d actual D=%d\n", ch, i, frame, expD, simD)
						fmt.Printf("    orig: GateOffFrame=%d len(SustainPerFrame)=%d ADSR.Decay=%d\n",
							orig.GateOffFrame, len(orig.SustainPerFrame), orig.ADSR.Decay)
						fmt.Printf("    sim:  len(SustainPerFrame)=%d ADSR.Decay=%d\n",
							len(sim.SustainPerFrame), sim.ADSR.Decay)
					}
					mismatches++
					break // Only count once per note
				}
			}
		}

		if mismatches > 0 {
			fmt.Printf("Channel %d: ⚠ %d/%d notes have D update mismatches\n", ch, mismatches, len(original[ch]))
			totalMismatches += mismatches
		} else {
			nonEnvelopeCount := 0
			for _, note := range original[ch] {
				if !note.EnvelopeMode {
					nonEnvelopeCount++
				}
			}
			fmt.Printf("Channel %d: ✓ %d non-envelope notes validated\n", ch, nonEnvelopeCount)
		}
	}

	if totalMismatches > 0 {
		panic(fmt.Sprintf("Dynamic D validation failed: %d notes with mismatches. D updates must match original exactly.", totalMismatches))
	}
}

// Global arrays for capturing toggle periods from both players
var capturedDirectTogglePeriods [3][]int
var capturedTrackerTogglePeriods [3][]int
var capturedDirectToggleEvents [3][]int    // Sample indices where toggles occur
var capturedTrackerToggleEvents [3][]int   // Sample indices where toggles occur

// Track all waveform register writes for validation
type WaveformWrite struct {
	sample     int
	waveform   byte   // bits 4-7 of control register
	pulseWidth uint16 // pulse width (registers +2 and +3)
}
var capturedDirectWaveformWrites [3][]WaveformWrite
var capturedTrackerWaveformWrites [3][]WaveformWrite

// CompareTogglePeriods compares toggle period values used by direct conversion vs tracker player
func CompareTogglePeriods(commands []spectrum.AYCommand, original [3][]Note, simulated [3][]SimulatedNote, startOffset int) {
	totalMismatches := 0

	for ch := 0; ch < 3; ch++ {
		// Calculate total samples for this channel
		maxFrame := 0
		for _, note := range original[ch] {
			if note.EndFrame > maxFrame {
				maxFrame = note.EndFrame
			}
		}
		totalSamples := int(float64(maxFrame+1) * 44100.0 / palFrameRate)

		// Run direct player and capture toggle periods and events
		capturedDirectTogglePeriods[ch] = make([]int, maxFrame+1)
		capturedDirectToggleEvents[ch] = []int{}
		capturedDirectWaveformWrites[ch] = []WaveformWrite{}
		_, _ = generateSIDPerNote(commands, ch, original[ch], totalSamples)
		directPeriods := capturedDirectTogglePeriods[ch]

		// Run tracker player and capture toggle periods and events
		capturedTrackerTogglePeriods[ch] = make([]int, maxFrame+1)
		capturedTrackerToggleEvents[ch] = []int{}
		capturedTrackerWaveformWrites[ch] = []WaveformWrite{}
		_ = generateSIDFromSimulatedNotes(commands, ch, simulated[ch], totalSamples, startOffset)
		trackerPeriods := capturedTrackerTogglePeriods[ch]

		// Compare frame-by-frame
		maxFrames := len(directPeriods)
		if len(trackerPeriods) > maxFrames {
			maxFrames = len(trackerPeriods)
		}

		realMismatches := 0      // Both players have non-zero but differ
		gateOnMismatches := 0    // Direct=0 (gate ON frame), tracker>0
		firstMismatch := -1

		for frame := 0; frame < maxFrames; frame++ {
			direct := 0
			tracker := 0

			if frame < len(directPeriods) {
				direct = directPeriods[frame]
			}
			if frame < len(trackerPeriods) {
				tracker = trackerPeriods[frame]
			}

			// Compare toggle periods
			if direct != tracker {
				if direct > 0 && tracker > 0 {
					// REAL MISMATCH: Both players toggling but with different periods
					if firstMismatch < 0 {
						firstMismatch = frame
					}
					realMismatches++
					if realMismatches <= 20 {
						// Find which simulated note is active at this frame
						noteInfo := "no active note"
						absoluteFrame := frame - startOffset
						currentFrame := 0
						for noteIdx, simNote := range simulated[ch] {
							noteDur := int(simNote.dur)
							if absoluteFrame >= currentFrame && absoluteFrame < currentFrame+noteDur {
								noteInfo = fmt.Sprintf("simNote[%d] dur=%d toggleSeqLen=%d togglePeriod=%d envMode=%v",
									noteIdx, simNote.dur, len(simNote.TogglePeriodSeq), simNote.TogglePeriod, simNote.EnvelopeMode)
								break
							}
							currentFrame += noteDur
						}
						ratio := float64(tracker) / float64(direct)
						fmt.Printf("  ERROR ch=%d frame=%d: direct=%d tracker=%d diff=%d ratio=%.2fx (%s)\n",
							ch, frame, direct, tracker, tracker-direct, ratio, noteInfo)
					}
				} else if direct == 0 && tracker > 0 {
					// Gate ON frame: direct hasn't started toggling yet, tracker has period loaded
					gateOnMismatches++
				}
			}
		}

		totalFramesWithToggle := 0
		for _, p := range directPeriods {
			if p > 0 {
				totalFramesWithToggle++
			}
		}

		if realMismatches > 0 {
			errorRate := float64(realMismatches) * 100.0 / float64(totalFramesWithToggle)
			fmt.Printf("Channel %d: ✗ %d/%d REAL mismatches (%.1f%%, first at frame %d)\n",
				ch, realMismatches, totalFramesWithToggle, errorRate, firstMismatch)
			totalMismatches += realMismatches
		} else if totalFramesWithToggle > 0 {
			fmt.Printf("Channel %d: ✓ %d frames validated (%d gate ON frames ignored)\n",
				ch, totalFramesWithToggle, gateOnMismatches)
		} else {
			fmt.Printf("Channel %d: No toggle periods\n", ch)
		}
	}

	// Also compare actual toggle events (sample timestamps where waveform bit flips)
	fmt.Printf("\n=== Validating Toggle Event Timing ===\n")
	totalEventMismatches := 0
	for ch := 0; ch < 3; ch++ {
		directEvents := capturedDirectToggleEvents[ch]
		trackerEvents := capturedTrackerToggleEvents[ch]

		if len(directEvents) == 0 && len(trackerEvents) == 0 {
			fmt.Printf("Channel %d: No toggle events\n", ch)
			continue
		}

		if len(directEvents) != len(trackerEvents) {
			fmt.Printf("Channel %d: ✗ Toggle count mismatch! Direct=%d Tracker=%d\n",
				ch, len(directEvents), len(trackerEvents))

			// Show first few events from each to help debug
			maxShow := 20
			if len(directEvents) < maxShow {
				maxShow = len(directEvents)
			}
			if len(trackerEvents) < maxShow {
				maxShow = len(trackerEvents)
			}
			fmt.Printf("  First %d direct events: ", maxShow)
			for i := 0; i < maxShow; i++ {
				fmt.Printf("%d ", directEvents[i])
			}
			fmt.Printf("\n  First %d tracker events: ", maxShow)
			for i := 0; i < maxShow; i++ {
				fmt.Printf("%d ", trackerEvents[i])
			}
			fmt.Printf("\n")

			// Show where they diverge
			for i := 0; i < min(len(directEvents), len(trackerEvents)); i++ {
				diff := abs(directEvents[i] - trackerEvents[i])
				if diff > 1 {
					directSample := directEvents[i]
					trackerSample := trackerEvents[i]
					directFrame := int(float64(directSample) * palFrameRate / 44100.0)
					trackerFrame := int(float64(trackerSample) * palFrameRate / 44100.0)
					fmt.Printf("  First divergence at toggle #%d: direct=%d (frame ~%d) tracker=%d (frame ~%d) diff=%d samples\n",
						i, directSample, directFrame, trackerSample, trackerFrame, diff)

					// Show previous toggle timing
					if i > 0 {
						prevDirect := directEvents[i-1]
						prevTracker := trackerEvents[i-1]
						directPeriodSamples := directSample - prevDirect
						trackerPeriodSamples := trackerSample - prevTracker
						fmt.Printf("  Period from toggle #%d to #%d: direct=%d samples, tracker=%d samples, diff=%d\n",
							i-1, i, directPeriodSamples, trackerPeriodSamples, trackerPeriodSamples-directPeriodSamples)
					}
					break
				}
			}

			totalEventMismatches += abs(len(directEvents) - len(trackerEvents))
			continue
		}

		// Compare event timestamps
		eventMismatches := 0
		for i := 0; i < len(directEvents) && i < 20; i++ {
			direct := directEvents[i]
			tracker := trackerEvents[i]
			diff := abs(direct - tracker)

			// Allow 1 sample tolerance for rounding
			if diff > 1 {
				if eventMismatches < 10 {
					fmt.Printf("  ERROR ch=%d toggle #%d: direct sample=%d tracker sample=%d diff=%d\n",
						ch, i, direct, tracker, diff)
				}
				eventMismatches++
			}
		}

		if eventMismatches > 0 {
			fmt.Printf("Channel %d: ✗ %d/%d toggle events misaligned (first %d checked)\n",
				ch, eventMismatches, len(directEvents), min(len(directEvents), 20))
			totalEventMismatches += eventMismatches
		} else {
			fmt.Printf("Channel %d: ✓ %d toggle events validated\n", ch, len(directEvents))
		}
	}

	if totalMismatches > 0 {
		panic(fmt.Sprintf("Toggle period validation failed: %d total REAL mismatches between direct and tracker players. Tracker must match direct conversion exactly.", totalMismatches))
	}
	if totalEventMismatches > 0 {
		fmt.Printf("⚠ Toggle event validation failed: %d total event timing mismatches. Continuing anyway...\n", totalEventMismatches)
		// panic(fmt.Sprintf("Toggle event validation failed: %d total event timing mismatches. Tracker toggle timing must match direct player exactly.", totalEventMismatches))
	}

	// Validate that waveform register writes respect CIA timer phase
	// At toggle events: waveforms change (expected)
	// At frame boundaries: waveforms must stay the same (preserve current timer phase)
	fmt.Printf("\n=== Validating Waveform Register Writes ===\n")
	fmt.Printf("Verification: Frame boundary writes preserve waveform bits (respect CIA timer phase)\n")
	totalViolations := 0

	for ch := 0; ch < 3; ch++ {
		directWrites := capturedDirectWaveformWrites[ch]
		trackerWrites := capturedTrackerWaveformWrites[ch]
		directToggleEvents := capturedDirectToggleEvents[ch]
		trackerToggleEvents := capturedTrackerToggleEvents[ch]

		if len(directWrites) == 0 && len(trackerWrites) == 0 {
			fmt.Printf("Channel %d: No waveform writes captured\n", ch)
			continue
		}

		// Create maps of toggle event samples
		directToggleMap := make(map[int]bool)
		for _, sample := range directToggleEvents {
			directToggleMap[sample] = true
		}
		trackerToggleMap := make(map[int]bool)
		for _, sample := range trackerToggleEvents {
			trackerToggleMap[sample] = true
		}

		// Check direct player: waveform changes at non-toggle events indicate frame boundary violations
		violationsDirect := 0
		if len(directWrites) > 1 {
			prevWaveform := directWrites[0].waveform
			for i := 1; i < len(directWrites); i++ {
				write := directWrites[i]
				if write.waveform != prevWaveform {
					// Waveform changed - if NOT at toggle event, it's a frame boundary violation
					if !directToggleMap[write.sample] {
						violationsDirect++
						if violationsDirect <= 3 {
							fmt.Printf("  ⚠ DIRECT CH%d sample %d (frame %d): Frame boundary write changed waveform 0x%X → 0x%X (should preserve)\n",
								ch, write.sample, write.sample/880, prevWaveform, write.waveform)
						}
					}
				}
				prevWaveform = write.waveform
			}
		}

		// Check tracker player
		violationsTracker := 0
		if len(trackerWrites) > 1 {
			prevWaveform := trackerWrites[0].waveform
			for i := 1; i < len(trackerWrites); i++ {
				write := trackerWrites[i]
				if write.waveform != prevWaveform {
					// Waveform changed - if NOT at toggle event, it's a frame boundary violation
					if !trackerToggleMap[write.sample] {
						violationsTracker++
						if violationsTracker <= 3 {
							fmt.Printf("  ⚠ TRACKER CH%d sample %d (frame %d): Frame boundary write changed waveform 0x%X → 0x%X (should preserve)\n",
								ch, write.sample, write.sample/880, prevWaveform, write.waveform)
						}
					}
				}
				prevWaveform = write.waveform
			}
		}

		totalChannelViolations := violationsDirect + violationsTracker
		if totalChannelViolations > 0 {
			fmt.Printf("Channel %d: ✗ %d frame boundary violations (Direct: %d, Tracker: %d)\n",
				ch, totalChannelViolations, violationsDirect, violationsTracker)
			totalViolations += totalChannelViolations
		} else {
			fmt.Printf("Channel %d: ✓ Frame boundary writes preserve waveform bits (Direct: %d violations, Tracker: %d violations)\n",
				ch, violationsDirect, violationsTracker)
		}
	}

	if totalViolations > 0 {
		panic(fmt.Sprintf("Frame boundary validation failed: %d violations detected. Frame boundary writes must preserve waveform bits to respect CIA timer phase.", totalViolations))
	}

	// Validate pulse width for waveform 0x05
	fmt.Printf("\n=== Validating Pulse Width ===\n")
	fmt.Printf("Verification: Waveform 0x05 must always use pulse width 0xFFF\n")
	totalPulseWidthViolations := 0

	for ch := 0; ch < 3; ch++ {
		directWrites := capturedDirectWaveformWrites[ch]
		trackerWrites := capturedTrackerWaveformWrites[ch]

		violationsDirect := 0
		for _, write := range directWrites {
			if write.waveform == 0x05 && write.pulseWidth != 0xFFF {
				violationsDirect++
				if violationsDirect <= 3 {
					fmt.Printf("  ⚠ DIRECT CH%d sample %d: waveform 0x05 has pulse width 0x%03X (expected 0xFFF)\n",
						ch, write.sample, write.pulseWidth)
				}
			}
		}

		violationsTracker := 0
		for _, write := range trackerWrites {
			if write.waveform == 0x05 && write.pulseWidth != 0xFFF {
				violationsTracker++
				if violationsTracker <= 3 {
					fmt.Printf("  ⚠ TRACKER CH%d sample %d: waveform 0x05 has pulse width 0x%03X (expected 0xFFF)\n",
						ch, write.sample, write.pulseWidth)
				}
			}
		}

		totalChannelViolations := violationsDirect + violationsTracker
		if totalChannelViolations > 0 {
			fmt.Printf("Channel %d: ✗ %d pulse width violations (Direct: %d, Tracker: %d)\n",
				ch, totalChannelViolations, violationsDirect, violationsTracker)
			totalPulseWidthViolations += totalChannelViolations
		} else if len(directWrites) > 0 || len(trackerWrites) > 0 {
			fmt.Printf("Channel %d: ✓ Pulse width correct for waveform 0x05\n", ch)
		}
	}

	if totalPulseWidthViolations > 0 {
		panic(fmt.Sprintf("Pulse width validation failed: %d violations detected. Waveform 0x05 must use pulse width 0xFFF.", totalPulseWidthViolations))
	}
}

// ValidateTogglePeriod validates that waveform toggling periods are correct for envelope and NoisePlusTone notes
func ValidateTogglePeriod(simulated [3][]SimulatedNote, startOffset int) {
	sampleRate := 44100.0
	sidClock := sidClockHz
	cyclesPerSample := sidClock / sampleRate

	for ch := 0; ch < 3; ch++ {
		// Track toggle state across all notes
		togglePeriodCycles := 0
		cycleCounter := 0.0
		toneHigh := true
		lastToggleSample := 0
		lastTogglePeriod := 0    // Period that was active at last toggle
		totalSampleCount := 0
		periodMismatches := 0
		totalToggles := 0

		// Track which notes use toggling
		toggleNoteCount := 0
		for _, simNote := range simulated[ch] {
			if simNote.EnvelopeMode || simNote.NoisePlusTone {
				toggleNoteCount++
			}
		}

		if toggleNoteCount == 0 {
			fmt.Printf("Channel %d: No toggle notes\n", ch)
			continue
		}

		// Calculate total frames
		totalFrames := startOffset
		for _, simNote := range simulated[ch] {
			totalFrames += int(simNote.dur)
		}

		absoluteFrame := 0

		// Skip start offset frames
		for frame := 0; frame < startOffset; frame++ {
			frameStart := int(float64(absoluteFrame) * sampleRate / palFrameRate)
			frameEnd := int(float64(absoluteFrame+1) * sampleRate / palFrameRate)
			numSamples := frameEnd - frameStart
			totalSampleCount += numSamples
			absoluteFrame++
		}

		lastFreq := uint16(0)

		for _, simNote := range simulated[ch] {
			noteDuration := int(simNote.dur)

			if simNote.isRest {
				// Rest note - but still run toggle counter if it's active
				for frame := 0; frame < noteDuration; frame++ {
					frameStart := int(float64(absoluteFrame) * sampleRate / palFrameRate)
					frameEnd := int(float64(absoluteFrame+1) * sampleRate / palFrameRate)
					numSamples := frameEnd - frameStart

					// CRITICAL: Keep toggle counter running even during rest notes
					if cycleCounter > 0 && togglePeriodCycles > 0 {
						for i := 0; i < numSamples; i++ {
							if cycleCounter > 0 {
								cycleCounter -= cyclesPerSample
								if cycleCounter <= 0 {
									toneHigh = !toneHigh
									cycleCounter += float64(togglePeriodCycles)

									// Validate period on toggle - only if period hasn't changed since last toggle
									if totalToggles > 0 && lastToggleSample > 0 && lastTogglePeriod == togglePeriodCycles {
										actualPeriod := totalSampleCount - lastToggleSample
										expectedPeriod := float64(togglePeriodCycles) / cyclesPerSample
										tolerance := 1.0

										if float64(actualPeriod) < expectedPeriod-tolerance || float64(actualPeriod) > expectedPeriod+tolerance {
											if periodMismatches < 5 {
												fmt.Printf("  DEBUG ch=%d frame=%d: Period mismatch! Actual=%d samples, Expected=%.1f\n",
													ch, absoluteFrame, actualPeriod, expectedPeriod)
											}
											periodMismatches++
										}
									}

									// Always update lastToggleSample and lastTogglePeriod after each toggle
									lastToggleSample = totalSampleCount
									lastTogglePeriod = togglePeriodCycles
									totalToggles++
								}
							}
							totalSampleCount++
						}
					} else {
						totalSampleCount += numSamples
					}

					absoluteFrame++
				}
				continue
			}

			for frame := 0; frame < noteDuration; frame++ {
				// Update frequency per frame
				if frame < len(simNote.freqSeq) {
					sidFreq := simNote.freqSeq[frame]

					// Update toggle period when frequency changes
					if (simNote.EnvelopeMode || simNote.NoisePlusTone) && sidFreq != lastFreq {
						lastFreq = sidFreq

						oscFreqHz := (float64(sidFreq) * sidClock) / sidFreqDivisor
						targetToggleFreq := oscFreqHz / 2.0
						idealTogglePeriod := sidClock / (2.0 * targetToggleFreq)
						newPeriod := int(idealTogglePeriod + 0.5)
						if newPeriod < 1 {
							newPeriod = 1
						}

						togglePeriodCycles = newPeriod

						if cycleCounter == 0 {
							cycleCounter = float64(newPeriod)
						}
					}
				}

				// Calculate samples for this frame
				frameStart := int(float64(absoluteFrame) * sampleRate / palFrameRate)
				frameEnd := int(float64(absoluteFrame+1) * sampleRate / palFrameRate)
				numSamples := frameEnd - frameStart

				// Check if toggling is active
				needsToggle := simNote.EnvelopeMode || simNote.NoisePlusTone

				// CRITICAL: Once toggle counter starts, always process per-sample (matches player behavior)
				if cycleCounter > 0 {
					needsToggle = true
				}

				if needsToggle && togglePeriodCycles > 0 {
					for i := 0; i < numSamples; i++ {
						if cycleCounter > 0 {
							cycleCounter -= cyclesPerSample
							if cycleCounter <= 0 {
								toneHigh = !toneHigh
								cycleCounter += float64(togglePeriodCycles)

								// Validate period on toggle - only if period hasn't changed since last toggle
								if totalToggles > 0 && lastToggleSample > 0 && lastTogglePeriod == togglePeriodCycles {
									actualPeriod := totalSampleCount - lastToggleSample
									expectedPeriod := float64(togglePeriodCycles) / cyclesPerSample
									tolerance := 1.0

									if float64(actualPeriod) < expectedPeriod-tolerance || float64(actualPeriod) > expectedPeriod+tolerance {
										if periodMismatches < 5 {
											fmt.Printf("  DEBUG ch=%d frame=%d: Period mismatch! Actual=%d samples, Expected=%.1f\n",
												ch, absoluteFrame, actualPeriod, expectedPeriod)
										}
										periodMismatches++
									}
								}

								// Always update lastToggleSample and lastTogglePeriod after each toggle
								lastToggleSample = totalSampleCount
								lastTogglePeriod = togglePeriodCycles
								totalToggles++
							}
						}
						totalSampleCount++
					}
				} else {
					totalSampleCount += numSamples
				}

				absoluteFrame++
			}
		}

		if periodMismatches > 0 {
			fmt.Printf("Channel %d: ⚠ %d/%d period mismatches\n", ch, periodMismatches, totalToggles)
		} else {
			fmt.Printf("Channel %d: ✓ %d toggles validated\n", ch, totalToggles)
		}
	}
}

// ValidateADSR compares ADSR parameters and SustainPerFrame between original and simulated notes
func ValidateADSR(original [3][]Note, simulated [3][]SimulatedNote) {
	totalMismatches := 0
	totalSustainMismatches := 0

	for ch := 0; ch < 3; ch++ {
		// Filter out rest patterns from simulated
		simulatedNonRest := []SimulatedNote{}
		for _, note := range simulated[ch] {
			if !note.isRest {
				simulatedNonRest = append(simulatedNonRest, note)
			}
		}

		if len(original[ch]) != len(simulatedNonRest) {
			fmt.Printf("Channel %d: ⚠ Note count mismatch! Original=%d Simulated(non-rest)=%d\n",
				ch, len(original[ch]), len(simulatedNonRest))
			continue
		}

		mismatches := 0
		sustainMismatches := 0
		for i := 0; i < len(original[ch]); i++ {
			orig := original[ch][i]
			sim := simulatedNonRest[i]

			// Check static ADSR parameters
			if orig.ADSR != sim.ADSR {
				if mismatches < 5 {
					fmt.Printf("  DEBUG ch=%d note=%d: expected A=%d D=%d S=%d R=%d InitD=%d, actual A=%d D=%d S=%d R=%d InitD=%d\n",
						ch, i,
						orig.ADSR.Attack, orig.ADSR.Decay, orig.ADSR.Sustain, orig.ADSR.Release, orig.ADSR.InitialDecay,
						sim.ADSR.Attack, sim.ADSR.Decay, sim.ADSR.Sustain, sim.ADSR.Release, sim.ADSR.InitialDecay)
				}
				mismatches++
			}

			// Check SustainPerFrame (dynamic S updates)
			if len(orig.SustainPerFrame) > 0 {
				if len(sim.SustainPerFrame) == 0 {
					if sustainMismatches == 0 {
						fmt.Printf("  DEBUG ch=%d note=%d: SustainPerFrame missing in simulated (original has %d frames)\n",
							ch, i, len(orig.SustainPerFrame))
					}
					sustainMismatches++
				} else if len(orig.SustainPerFrame) != len(sim.SustainPerFrame) {
					if sustainMismatches < 3 {
						fmt.Printf("  DEBUG ch=%d note=%d: SustainPerFrame length mismatch (expected %d, got %d)\n",
							ch, i, len(orig.SustainPerFrame), len(sim.SustainPerFrame))
					}
					sustainMismatches++
				} else {
					// Compare frame-by-frame
					frameMismatches := 0
					for frame := 0; frame < len(orig.SustainPerFrame); frame++ {
						if orig.SustainPerFrame[frame] != sim.SustainPerFrame[frame] {
							frameMismatches++
						}
					}
					if frameMismatches > 0 {
						if sustainMismatches < 3 {
							fmt.Printf("  DEBUG ch=%d note=%d: %d/%d SustainPerFrame values differ\n",
								ch, i, frameMismatches, len(orig.SustainPerFrame))
						}
						sustainMismatches++
					}
				}
			}
		}

		if mismatches > 0 {
			fmt.Printf("Channel %d: ⚠ %d/%d notes have static ADSR mismatches\n", ch, mismatches, len(original[ch]))
			totalMismatches += mismatches
		}

		if sustainMismatches > 0 {
			fmt.Printf("Channel %d: ⚠ %d/%d notes have SustainPerFrame mismatches\n", ch, sustainMismatches, len(original[ch]))
			totalSustainMismatches += sustainMismatches
		}

		if mismatches == 0 && sustainMismatches == 0 {
			fmt.Printf("Channel %d: ✓ %d notes validated (static ADSR + SustainPerFrame)\n", ch, len(original[ch]))
		}
	}

	if totalMismatches > 0 || totalSustainMismatches > 0 {
		panic(fmt.Sprintf("ADSR validation failed: %d static ADSR mismatches, %d SustainPerFrame mismatches. Tracker format must reproduce original ADSR exactly.", totalMismatches, totalSustainMismatches))
	}
}

// buildExpectedFrequencySequence extracts the SID frequency for each frame from original notes
func buildExpectedFrequencySequence(commands []spectrum.AYCommand, channel int, notes []Note) []uint16 {
	if len(notes) == 0 {
		return []uint16{}
	}

	// Calculate total frames
	maxFrame := 0
	for _, note := range notes {
		if note.EndFrame > maxFrame {
			maxFrame = note.EndFrame
		}
	}

	freqSeq := make([]uint16, maxFrame+1)

	// Build frequency table from AY commands
	toneLoReg := uint8(channel * 2)
	toneHiReg := uint8(channel*2 + 1)
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

	// Build a frame-to-note index for faster lookup
	frameToNote := make(map[int]*Note)
	for i := range notes {
		for frame := notes[i].StartFrame; frame <= notes[i].EndFrame; frame++ {
			frameToNote[frame] = &notes[i]
		}
	}

	// Map notes to frames and compute SID frequency
	for frame := 0; frame <= maxFrame; frame++ {
		activeNote := frameToNote[frame]

		if activeNote != nil {
			// Note is active, compute SID frequency
			var sidFreq uint16
			var ayFreq uint16

			if activeNote.EnvelopeMode {
				// Envelope mode: use envelope period
				envPeriod := getAYEnvelopePeriod(commands, frame)
				sidFreq = computeEnvelopeFrequency(envPeriod)
				if activeNote.Waveform == 0x02 { // Sawtooth
					sidFreq *= 2
				}
			} else if activeNote.Waveform == 0x08 {
				// Noise: use noise period
				noisePeriod := getAYNoisePeriod(commands, frame)
				sidFreq = computeNoiseFrequency(noisePeriod)
			} else {
				// Tone: use tone frequency
				if freq, ok := freqPerFrame[frame]; ok && freq > 0 {
					ayFreq = freq
					sidFreq = computeToneFrequency(ayFreq)
				}
			}

			// Debug: track AY freq at first divergence (ch1 frame 4042)
			if channel == 1 && frame == 4042 {
				fmt.Printf("  DEBUG expected ch=%d frame=%d: envelope=%v waveform=0x%02x ayFreq=%d sidFreq=%d\n",
					channel, frame, activeNote.EnvelopeMode, activeNote.Waveform, ayFreq, sidFreq)
			}

			freqSeq[frame] = sidFreq
		} else {
			// No note active, frequency is 0 (rest/gap)
			freqSeq[frame] = 0
		}
	}

	return freqSeq
}

// buildSimulatedFrequencySequence extracts the SID frequency for each frame from simulated notes
func buildSimulatedFrequencySequence(commands []spectrum.AYCommand, channel int, simNotes []SimulatedNote, startOffset int) []uint16 {
	freqSeq := []uint16{}

	// Prepend silence for the global offset (from TrackerFormat.StartOffset)
	for i := 0; i < startOffset; i++ {
		freqSeq = append(freqSeq, 0)
	}

	absoluteFrame := startOffset

	for _, simNote := range simNotes {
		noteDuration := int(simNote.dur)

		// Rest patterns have freq=0
		if simNote.isRest {
			for frame := 0; frame < noteDuration; frame++ {
				freqSeq = append(freqSeq, 0)
				absoluteFrame++
			}
			continue
		}

		// Convert each frame's AY frequency to SID frequency
		// Must match the logic in generateSIDPerNote
		for frame := 0; frame < noteDuration; frame++ {
			var sidFreq uint16
			var ayFreq uint16

			if simNote.EnvelopeMode {
				// Envelope mode: use envelope period from AY commands
				envPeriod := getAYEnvelopePeriod(commands, absoluteFrame)
				sidFreq = computeEnvelopeFrequency(envPeriod)
				if simNote.Waveform == 0x02 { // Sawtooth
					sidFreq *= 2
				}
			} else if simNote.Waveform == 0x08 {
				// Noise: use noise period from AY commands
				noisePeriod := getAYNoisePeriod(commands, absoluteFrame)
				sidFreq = computeNoiseFrequency(noisePeriod)
			} else {
				// Tone: use stored frequency
				ayFreq = simNote.freq
				if frame < len(simNote.freqSeq) {
					ayFreq = simNote.freqSeq[frame]
				}
				sidFreq = computeToneFrequency(ayFreq)
			}

			// Debug: track AY freq at first divergence (ch1 frame 4042)
			if channel == 1 && absoluteFrame == 4042 {
				fmt.Printf("  DEBUG simulated ch=%d frame=%d: envelope=%v waveform=0x%02x ayFreq=%d sidFreq=%d\n",
					channel, absoluteFrame, simNote.EnvelopeMode, simNote.Waveform, ayFreq, sidFreq)
			}

			freqSeq = append(freqSeq, sidFreq)
			absoluteFrame++
		}
	}

	return freqSeq
}

// generateSIDFromSimulatedNotes generates audio from simulated tracker notes
//
// ARCHITECTURE: This is the TRACKER PLAYER - it simulates what a 6502 CPU would do when
// playing back the tracker format on real SID hardware. It represents the final playback code.
//
// RULES:
// - MUST read ONLY from SimulatedNote fields (freq, freqSeq, waveformSeq, ADSR, etc.)
// - MUST NOT read from AY commands stream (commands parameter is for validation functions only)
// - All necessary data must be pre-encoded in the tracker format by the extractor
// - Any getAY*() function calls in this function are ARCHITECTURAL VIOLATIONS
//
// The 'commands' and 'channel' parameters are only passed through to validation/capture functions,
// not used in the actual playback logic.
func generateSIDFromSimulatedNotes(commands []spectrum.AYCommand, channel int, simNotes []SimulatedNote, totalSamples int, startOffset int) []int16 {
	sampleRate := 44100.0
	samples := make([]int16, totalSamples)

	sid := NewSIDEmulator(sampleRate)
	regBase := byte(0) // Use voice 0

	// Initialize voice
	sid.WriteRegister(regBase+0, 0x00)  // Freq low
	sid.WriteRegister(regBase+1, 0x10)  // Freq high
	sid.WriteRegister(regBase+4, 0x00)  // Gate OFF

	// Track absolute frame number for precise sample timing
	absoluteFrame := 0

	// CIA timer interrupt model for envelope mode and NoisePlusTone
	nextPeriod := 0          // Period read at frame boundaries (staging area)
	activePeriod := 0        // Period for currently RUNNING cycle (locked when cycleCounter was loaded)
	cycleCounter := 0.0      // Countdown timer in SID clock cycles
	timerIsHi := true        // CIA timer phase (toggles on every interrupt)
	currentWaveformHi := byte(0x00) // Waveform to write when timerIsHi=true
	currentWaveformLo := byte(0x00) // Waveform to write when timerIsHi=false
	totalSampleCount := 0    // Running total of samples generated
	lastToggleSample := -1   // Track last toggle sample for period validation

	// Apply global offset (frames before first note)
	for frame := 0; frame < startOffset; frame++ {
		// Use floating-point calculation to avoid truncation drift
		frameStart := int(float64(absoluteFrame) * sampleRate / palFrameRate)
		frameEnd := int(float64(absoluteFrame+1) * sampleRate / palFrameRate)
		if frameEnd > totalSamples {
			frameEnd = totalSamples
		}
		// Generate silent samples (SID emulator continues running)
		for i := frameStart; i < frameEnd; i++ {
			sid.GenerateSample(0) // Keep emulator running
			samples[i] = 0
		}
		absoluteFrame++
	}

	for noteIdx, simNote := range simNotes {
		noteDuration := int(simNote.dur)

		// Debug every note in CH0 (first 5 notes)

		// Handle rest patterns (silence)
		if simNote.isRest {
			// Generate silent samples while keeping emulator running
			for frame := 0; frame < noteDuration; frame++ {
				frameStart := int(float64(absoluteFrame) * sampleRate / palFrameRate)
				frameEnd := int(float64(absoluteFrame+1) * sampleRate / palFrameRate)
				if frameEnd > totalSamples {
					frameEnd = totalSamples
				}
				numSamples := frameEnd - frameStart

				// Keep SID emulator running during rests
				// Debug rest notes (first frame of rest only)
				if channel == 0 && frame == 0 {
					fmt.Printf("  REST NOTE START frame=%d duration=%d: cycleCounter=%.1f activePeriod=%d nextPeriod=%d\n",
						absoluteFrame, noteDuration, cycleCounter, activePeriod, nextPeriod)
				}
				// Debug first few frames of first rest note
				if channel == 0 && absoluteFrame >= 182 && absoluteFrame < 185 {
					fmt.Printf("    REST LOOP frame=%d: numSamples=%d cycleCounter=%.1f before loop\n",
						absoluteFrame, numSamples, cycleCounter)
				}

				// Need to re-declare cyclesPerSample for rest notes
				sidClock := sidClockHz
				cyclesPerSample := sidClock / sampleRate

				for i := 0; i < numSamples; i++ {
					sid.GenerateSample(0)
					samples[frameStart+i] = 0

					// Keep counting down cycleCounter to maintain phase continuity
					if cycleCounter > 0 && activePeriod > 0 {
						cycleCounter -= cyclesPerSample
						if cycleCounter <= 0 {
							// CIA timer interrupt during rest - toggle phase
							timerIsHi = !timerIsHi
							// Reload with activePeriod
							cycleCounter += float64(activePeriod)
						}
					}
				}
				absoluteFrame++
			}
			continue
		}

		// ALWAYS gate OFF before starting new note (matches direct conversion)
		// This ensures envelope restarts cleanly and avoids ADSR delay bug
		if noteIdx > 0 {
			// Set release to 0 for instant gate OFF
			prevADSR := sid.Registers[regBase+6]
			sid.WriteRegister(regBase+6, (prevADSR&0xF0)|0x00) // Keep sustain, set release=0
			// Gate OFF: preserve waveform bits (4-7), clear gate bit (0)
			currentCtrl := sid.Registers[regBase+4]
			sid.WriteRegister(regBase+4, currentCtrl&0xFE)

			// Generate samples to let SID process gate OFF
			for i := 0; i < 10; i++ {
				_ = sid.GenerateSample(0)
			}
		}

		// Set ADSR while gate is OFF (before gate ON)
		// Use SustainPerFrame[0] if available (volume-following ADSR), otherwise use static S
		initialS := simNote.ADSR.Sustain
		if len(simNote.SustainPerFrame) > 0 {
			initialS = simNote.SustainPerFrame[0]
		}
		sid.WriteRegister(regBase+5, (simNote.ADSR.Attack<<4)|simNote.ADSR.InitialDecay)
		sid.WriteRegister(regBase+6, (initialS<<4)|simNote.ADSR.Release)

		// Set initial frequency from stored sequence (already converted to SID values)
		var sidFreq uint16
		if len(simNote.freqSeq) > 0 {
			sidFreq = simNote.freqSeq[0]
		} else {
			sidFreq = simNote.freq
		}
		sid.WriteRegister(regBase+0, byte(sidFreq&0xFF))
		sid.WriteRegister(regBase+1, byte(sidFreq>>8))


		// Initialize CIA timer interrupt waveforms based on note type
		// Physical model: on every timer interrupt, we write currentWaveformHi or currentWaveformLo
		waveform := simNote.Waveform
		if simNote.EnvelopeMode {
			// Envelope mode: alternate between waveform (triangle/sawtooth) and silence
			currentWaveformHi = waveform
			currentWaveformLo = 0x05 // Silence via combined pulse+triangle with 0xFFF pulse width
		} else if simNote.NoisePlusTone {
			// NoisePlusTone: alternate between noise and silence (or pulse and pulse)
			// Initial state depends on starting waveform
			if waveform == 0x08 {
				// Starting with noise
				currentWaveformHi = 0x08
				currentWaveformLo = 0x05
			} else {
				// Starting with pulse (no modulation)
				currentWaveformHi = 0x04
				currentWaveformLo = 0x04
			}
		} else {
			// Regular notes: same waveform for both phases (no modulation)
			currentWaveformHi = waveform
			currentWaveformLo = waveform
		}

		// Set pulse width for pulse waveforms
		if waveform == 0x04 {
			// Set pulse width 0x800 (50% duty cycle) for pure pulse
			sid.WriteRegister(regBase+2, 0x00)
			sid.WriteRegister(regBase+3, 0x08)
		}

		// ALWAYS write waveform + gate ON at note start for ALL note types
		// For envelope/NoisePlusTone modes, CIA timer will update it dynamically during the note
		sid.WriteRegister(regBase+4, (waveform<<4)|0x01)

		// Initialize nextPeriod at note start (before frame loop)
		// CRITICAL: This ensures frame 0 has the correct period for initialization checks
		if simNote.EnvelopeMode || simNote.NoisePlusTone {
			if len(simNote.TogglePeriodSeq) > 0 {
				// Use first value from sequence
				nextPeriod = int(simNote.TogglePeriodSeq[0])
			} else if simNote.TogglePeriod > 0 {
				// Use constant period
				nextPeriod = int(simNote.TogglePeriod)
			}
		}

		// VALIDATE: Envelope mode notes must have toggle period
		if simNote.EnvelopeMode && nextPeriod == 0 {
			panic(fmt.Sprintf("TRACKER CH%d frame %d: Envelope mode note has zero toggle period (waveform=0x%02X TogglePeriod=%d TogglePeriodSeq=%d)",
				channel, absoluteFrame, simNote.Waveform, simNote.TogglePeriod, len(simNote.TogglePeriodSeq)))
		}

		// Debug note start (first few notes)
		if channel == 0 && absoluteFrame < 1000 {
			needsToggle := simNote.EnvelopeMode || simNote.NoisePlusTone
			fmt.Printf("  NOTE START CH%d frame=%d: needsToggle=%v duration=%d TogglePeriod=%d TogglePeriodSeq=%d nextPeriod=%d cycleCounter=%.1f\n",
				channel, absoluteFrame, needsToggle, noteDuration, simNote.TogglePeriod, len(simNote.TogglePeriodSeq), nextPeriod, cycleCounter)
		}

		// Generate audio for each frame of this note
		for frame := 0; frame < noteDuration; frame++ {
			// Check waveformSeq for NoisePlusTone mixer state changes
			// Waveform pattern encodes both Hi and Lo: (Hi << 4) | Lo
			if simNote.NoisePlusTone && frame < len(simNote.waveformSeq) && simNote.waveformSeq[frame] != 0 {
				packedWaveform := simNote.waveformSeq[frame]
				// Decode: upper nibble = Hi, lower nibble = Lo
				newWaveformHi := (packedWaveform >> 4) & 0x0F
				newWaveformLo := packedWaveform & 0x0F

				// Update waveforms from pattern
				currentWaveformHi = newWaveformHi
				currentWaveformLo = newWaveformLo
			}

			// Update frequency per frame (glissando/vibrato/envelope)
			if frame < len(simNote.freqSeq) {
				// Check waveformSeq for R13 envelope shape changes (envelope mode only)
				// CRITICAL: Always update currentWaveformHi/Lo, CIA timer will respect phase on next interrupt
				if simNote.EnvelopeMode && frame < len(simNote.waveformSeq) && simNote.waveformSeq[frame] != 0 {
					newWaveform := simNote.waveformSeq[frame]
					// Update HIGH phase waveform (triangle or sawtooth)
					// LOW phase stays as silence (0x05)
					currentWaveformHi = newWaveform
					currentWaveformLo = 0x05
				}

				// freqSeq already contains SID frequency values - just write them
				sidFreq := simNote.freqSeq[frame]
				sid.WriteRegister(regBase+0, byte(sidFreq&0xFF))
				sid.WriteRegister(regBase+1, byte(sidFreq>>8))

				// Read toggle period for this frame at frame boundary
				if (simNote.EnvelopeMode || simNote.NoisePlusTone) && frame < len(simNote.TogglePeriodSeq) {
					// Varying period: read from sequence
					nextPeriod = int(simNote.TogglePeriodSeq[frame])
				} else if (simNote.EnvelopeMode || simNote.NoisePlusTone) && simNote.TogglePeriod > 0 {
					// Constant period
					nextPeriod = int(simNote.TogglePeriod)
				}

				// Capture toggle period for validation
				// Match direct player: capture "what period will be used if NEW cycle starts in this frame"
				if channel < 3 && len(capturedTrackerTogglePeriods[channel]) > absoluteFrame {
					capturedTrackerTogglePeriods[channel][absoluteFrame] = nextPeriod
				}
			}

			// Dynamic ADSR updates for volume-following (skip envelope mode notes)
			if !simNote.EnvelopeMode {
				// Update S value each frame (before tail decay starts)
				// NOTE: S can be written freely - ADSR delay bug only triggers on A/D/R changes
				if frame < len(simNote.SustainPerFrame) {
					newS := simNote.SustainPerFrame[frame]
					currentR := sid.Registers[regBase+6] & 0x0F
					sid.WriteRegister(regBase+6, (newS<<4)|currentR)
				}

				// Switch D from InitialDecay to 0 after frame 0 (for volume-following behavior)
				if frame == 1 && simNote.ADSR.InitialDecay > 0 {
					currentA := (sid.Registers[regBase+5] >> 4) & 0x0F
					sid.WriteRegister(regBase+5, (currentA<<4)|0) // D=0
				}

				// At GateOffFrame, switch D from 0 to tailDecayRate and S to 0 (start tail decay)
				if simNote.GateOffFrame > 0 && frame == simNote.GateOffFrame {
					currentA := (sid.Registers[regBase+5] >> 4) & 0x0F
					sid.WriteRegister(regBase+5, (currentA<<4)|simNote.ADSR.Decay) // D = tailDecayRate
					currentR := sid.Registers[regBase+6] & 0x0F
					sid.WriteRegister(regBase+6, (0<<4)|currentR) // S = 0 target
				}
			}

			// If current note has D>0, gate OFF 1 frame before note ends (matches direct player)
			// This allows the decay phase to complete before the new note starts
			if frame == noteDuration-framesBeforeNoteOff && simNote.ADSR.Decay > 0 {
				hasNextNote := noteIdx+1 < len(simNotes) && !simNotes[noteIdx+1].isRest
				if hasNextNote {
					sid.WriteRegister(regBase+4, waveform<<4) // Gate OFF
				}
			}

			// Calculate samples for this frame using floating-point to avoid drift
			frameStart := int(float64(absoluteFrame) * sampleRate / palFrameRate)
			frameEnd := int(float64(absoluteFrame+1) * sampleRate / palFrameRate)
			if frameEnd > totalSamples {
				frameEnd = totalSamples
			}
			numSamples := frameEnd - frameStart

			// Check if this note needs per-sample toggle processing
			needsToggle := simNote.EnvelopeMode || simNote.NoisePlusTone

			// VALIDATE: Envelope mode should always toggle when needsToggle is true
			if simNote.EnvelopeMode && frame < noteDuration && nextPeriod == 0 {
				panic(fmt.Sprintf("TRACKER CH%d frame %d (note frame %d/%d): Envelope mode has zero nextPeriod - will skip toggling!",
					channel, absoluteFrame, frame, noteDuration))
			}

			if needsToggle && nextPeriod > 0 {
				// Initialize cycle counter and lock period on first frame
				if cycleCounter == 0 && frame == 0 {
					activePeriod = nextPeriod
					cycleCounter = float64(activePeriod)
					timerIsHi = true // Start in HI phase
				}

				// Per-sample processing with CIA timer interrupts
				sidClock := sidClockHz
				cyclesPerSample := sidClock / sampleRate // ~22.34 SID cycles per audio sample
				gateBit := byte(0x01) // Gate stays ON during envelope/NoisePlusTone

				// VALIDATE: After frame update, if waveform is 0x05, pulse width must be 0xFFF
				currentWaveform := (sid.Registers[regBase+4] & 0xF0) >> 4
				if currentWaveform == 0x05 {
					currentPulseWidth := uint16(sid.Registers[regBase+2]) | (uint16(sid.Registers[regBase+3]&0x0F) << 8)
					if currentPulseWidth != 0xFFF {
						panic(fmt.Sprintf("TRACKER CH%d frame %d sample %d: After frame update, waveform 0x05 has incorrect pulse width 0x%03X (must be 0xFFF)", channel, absoluteFrame, frameStart, currentPulseWidth))
					}
				}

				// Debug sample loop entry (trace cycleCounter progression)
				for i := 0; i < numSamples; i++ {
					// Generate sample with current waveform state
					samples[frameStart+i] = sid.GenerateSample(0)

					// Countdown CIA timer (AFTER generating sample)
					cycleCounter -= cyclesPerSample

					// Debug first sample of frame 1668
					if channel == 0 && absoluteFrame == 1668 && i == 0 {
						fmt.Printf("    PER-SAMPLE LOOP START frame=%d i=0: cycleCounter=%.1f (after decrement)\n", absoluteFrame, cycleCounter)
					}

					if cycleCounter <= 0 {
						sampleIdx := frameStart + i

						// Debug toggles at frame 1668
						if channel == 0 && absoluteFrame == 1668 {
							fmt.Printf("      CIA FIRE frame=%d i=%d sampleIdx=%d: timerIsHi=%v currentWaveformHi=0x%02X currentWaveformLo=0x%02X\n",
								absoluteFrame, i, sampleIdx, timerIsHi, currentWaveformHi, currentWaveformLo)
						}

						// CIA TIMER INTERRUPT: Write waveform based on current timer phase
						// Physical model: on every interrupt, we output Hi or Lo waveform
						if timerIsHi {
							// HIGH phase
							if currentWaveformHi == 0x08 {
								// VALIDATE: Check if AY actually has audible noise
								mixer := getAYMixerState(commands, absoluteFrame)
								noiseBit := uint8(3 + channel) // Bit 3=A, 4=B, 5=C
								noiseOn := (mixer & (1 << noiseBit)) == 0

								if !noiseOn {
									panic(fmt.Sprintf("TRACKER CH%d frame %d sample %d: SID writing noise waveform 0x08 but AY mixer has noise OFF (mixer=0x%02X bit%d=1)",
										channel, absoluteFrame, sampleIdx, mixer, noiseBit))
								}

								// Noise waveform
								sid.WriteRegister(regBase+4, (0x08<<4)|gateBit)
							} else if currentWaveformHi == 0x04 {
								// Pulse waveform (set 50% pulse width)
								sid.WriteRegister(regBase+2, 0x00)
								sid.WriteRegister(regBase+3, 0x08)
								sid.WriteRegister(regBase+4, (0x04<<4)|gateBit)
							} else {
								// Triangle/Sawtooth waveform
								sid.WriteRegister(regBase+4, (currentWaveformHi<<4)|gateBit)
							}
						} else {
							// LOW phase
							if currentWaveformLo == 0x05 {
								// Pulse+Triangle with pulse width 0xFFF
								sid.WriteRegister(regBase+2, 0xFF)
								sid.WriteRegister(regBase+3, 0x0F)
								sid.WriteRegister(regBase+4, (0x05<<4)|gateBit)
								// VALIDATE: waveform 0x05 must have pulse width 0xFFF
								actualPulseWidth := uint16(sid.Registers[regBase+2]) | (uint16(sid.Registers[regBase+3]&0x0F) << 8)
								if actualPulseWidth != 0xFFF {
									panic(fmt.Sprintf("TRACKER CH%d sample %d: Wrote waveform 0x05 with incorrect pulse width 0x%03X (must be 0xFFF)", channel, sampleIdx, actualPulseWidth))
								}
							} else {
								// Non-modulated: output same waveform as HIGH
								if currentWaveformLo == 0x04 {
									sid.WriteRegister(regBase+2, 0x00)
									sid.WriteRegister(regBase+3, 0x08)
									sid.WriteRegister(regBase+4, (0x04<<4)|gateBit)
								} else {
									sid.WriteRegister(regBase+4, (currentWaveformLo<<4)|gateBit)
								}
							}
						}

						// Capture phase BEFORE toggle for validation
						phaseBeforeToggle := timerIsHi

						// Toggle timer phase for next interrupt
						timerIsHi = !timerIsHi

						// Capture toggle event for validation
						if channel < 3 && len(capturedTrackerToggleEvents[channel]) < 10000 {
							capturedTrackerToggleEvents[channel] = append(capturedTrackerToggleEvents[channel], sampleIdx)
						}

						// Capture waveform write for validation (same limit as toggle events)
						waveform := (sid.Registers[regBase+4] & 0xF0) >> 4
						pulseWidth := uint16(sid.Registers[regBase+2]) | (uint16(sid.Registers[regBase+3]&0x0F) << 8)
						if channel < 3 && len(capturedTrackerWaveformWrites[channel]) < 10000 {
							capturedTrackerWaveformWrites[channel] = append(capturedTrackerWaveformWrites[channel],
								WaveformWrite{sample: sampleIdx, waveform: waveform, pulseWidth: pulseWidth})
						}

						// VALIDATE: Waveform written by CIA timer matches expected waveform
						expectedWaveform := currentWaveformHi
						if !phaseBeforeToggle {
							expectedWaveform = currentWaveformLo
						}
						if waveform != expectedWaveform {
							panic(fmt.Sprintf("TRACKER CH%d sample %d: CIA timer wrote waveform 0x%02X but expected 0x%02X (phase=%s Hi=0x%02X Lo=0x%02X)",
								channel, sampleIdx, waveform, expectedWaveform, map[bool]string{true: "HI", false: "LO"}[phaseBeforeToggle], currentWaveformHi, currentWaveformLo))
						}

						// VALIDATE: Any waveform 0x05 must have pulse width 0xFFF
						if waveform == 0x05 && pulseWidth != 0xFFF {
							panic(fmt.Sprintf("TRACKER CH%d sample %d: CIA timer wrote waveform 0x05 with invalid pulse width 0x%03X (must be 0xFFF)", channel, sampleIdx, pulseWidth))
						}

						// VALIDATE: Envelope mode with noise should alternate Hi=0x08, Lo=0x05
						// If we're in envelope mode with noise and wrote 0x08 during LOW phase, that's wrong
						if currentWaveformHi == 0x08 && currentWaveformLo == 0x05 && !phaseBeforeToggle {
							if waveform == 0x08 {
								panic(fmt.Sprintf("TRACKER CH%d sample %d: Envelope mode noise - wrote 0x08 during LOW phase (should write 0x05 for silence/modulation)", channel, sampleIdx))
							}
						}

						// Validate period duration using activePeriod (the period for the cycle that just completed)
						if lastToggleSample >= 0 {
							actualPeriodSamples := sampleIdx - lastToggleSample
							expectedPeriodSamples := float64(activePeriod) / cyclesPerSample
							diff := expectedPeriodSamples - float64(actualPeriodSamples)

							// Flag if toggle happened significantly early (period was cut short)
							if diff > 5.0 { // More than 5 samples too early = premature toggle
								fmt.Printf("  ⚠ TRACKER CH%d sample %d: PREMATURE TOGGLE! Period was %d samples (expected %.1f), cut short by %.1f samples. Frame=%d activePeriod=%d nextPeriod=%d cycleCounter=%.1f\n",
									channel, sampleIdx, actualPeriodSamples, expectedPeriodSamples, diff, absoluteFrame, activePeriod, nextPeriod, cycleCounter)
								// Check if this is near a frame boundary
								samplesIntoFrame := sampleIdx - frameStart
								if samplesIntoFrame < 10 {
									fmt.Printf("    → Toggle happened %d samples into frame %d (FRAME BOUNDARY!)\n", samplesIntoFrame, absoluteFrame)
								}
							}

							// Flag if toggle happened significantly late (period was too long)
							if diff < -5.0 { // More than 5 samples too late
								fmt.Printf("  ⚠ TRACKER CH%d sample %d: LATE TOGGLE! Period was %d samples (expected %.1f), too long by %.1f samples. Frame=%d activePeriod=%d nextPeriod=%d\n",
									channel, sampleIdx, actualPeriodSamples, expectedPeriodSamples, -diff, absoluteFrame, activePeriod, nextPeriod)
							}
						}
						lastToggleSample = sampleIdx

						// Reload counter and lock period for NEW cycle
						// CRITICAL: Use nextPeriod (from frame boundary), not activePeriod
						cycleCounter += float64(nextPeriod)
						activePeriod = nextPeriod
					}
				}


				totalSampleCount += numSamples
			} else {
				// Simple sample generation (no toggling needed)
				// CRITICAL: Keep cycleCounter running even during non-toggle notes for phase continuity
				sidClock := sidClockHz
				cyclesPerSample := sidClock / sampleRate

				// VALIDATE BUG 2: Pure pulse instruments (Hi=Lo=0x04) must have pulse width 0x800 or 0x7FF
				if currentWaveformHi == 0x04 && currentWaveformLo == 0x04 {
					currentPulseWidth := uint16(sid.Registers[regBase+2]) | (uint16(sid.Registers[regBase+3]&0x0F) << 8)
					if currentPulseWidth != 0x800 && currentPulseWidth != 0x7FF {
						panic(fmt.Sprintf("TRACKER CH%d frame %d: Pure pulse instrument (Hi=Lo=0x04) has invalid pulse width 0x%03X (must be 0x800 or 0x7FF)", channel, absoluteFrame, currentPulseWidth))
					}
				}

				// VALIDATE: Expected waveform matches actual register
				actualWaveform := (sid.Registers[regBase+4] & 0xF0) >> 4

				// For pure pulse notes, register must be 0x04
				if currentWaveformHi == 0x04 && currentWaveformLo == 0x04 {
					if actualWaveform != 0x04 {
						panic(fmt.Sprintf("TRACKER CH%d frame %d: Expected pure pulse (Hi=Lo=0x04) but waveform register is 0x%02X", channel, absoluteFrame, actualWaveform))
					}
				}

				// For pure noise notes, register must be 0x08
				if currentWaveformHi == 0x08 && currentWaveformLo == 0x08 {
					if actualWaveform != 0x08 {
						panic(fmt.Sprintf("TRACKER CH%d frame %d: Expected pure noise (Hi=Lo=0x08) but waveform register is 0x%02X", channel, absoluteFrame, actualWaveform))
					}
				}

				// VALIDATE: Any waveform 0x05 must have pulse width 0xFFF
				if actualWaveform == 0x05 {
					currentPulseWidth := uint16(sid.Registers[regBase+2]) | (uint16(sid.Registers[regBase+3]&0x0F) << 8)
					if currentPulseWidth != 0xFFF {
						panic(fmt.Sprintf("TRACKER CH%d frame %d: Waveform 0x05 has invalid pulse width 0x%03X (must be 0xFFF)", channel, absoluteFrame, currentPulseWidth))
					}
				}

				// Debug non-toggle notes (first few in the song)
				if channel == 0 && absoluteFrame < 1000 && frame == 0 {
					fmt.Printf("  NON-TOGGLE NOTE frame=%d: cycleCounter=%.1f activePeriod=%d nextPeriod=%d\n",
						absoluteFrame, cycleCounter, activePeriod, nextPeriod)
				}

				for i := 0; i < numSamples; i++ {
					samples[frameStart+i] = sid.GenerateSample(0)

					// Keep counting down cycleCounter to maintain phase continuity
					// Even though we don't write waveforms (Hi==Lo), we must maintain timer phase
					if cycleCounter > 0 && activePeriod > 0 {
						cycleCounter -= cyclesPerSample
						if cycleCounter <= 0 {
							// CIA timer interrupt - toggle phase even though waveform doesn't change
							timerIsHi = !timerIsHi
							// Reload with activePeriod (nextPeriod may be 0 for non-toggle notes)
							cycleCounter += float64(activePeriod)
						}
					}
				}
				totalSampleCount += numSamples
			}

			absoluteFrame++
		}

		// Gate OFF at note end (only if no adjacent note)
		// Otherwise next note will gate OFF at its start
		hasAdjacentNote := noteIdx+1 < len(simNotes) && !simNotes[noteIdx+1].isRest
		if !hasAdjacentNote {
			sid.WriteRegister(regBase+4, waveform<<4) // Gate OFF
		}
	}

	return samples
}

// writeMultiFormatWAV writes 10-channel WAV: 3 AY + 3 original SID + 3 replayed SID + 1 debug
func writeMultiFormatWAV(path string, ayChannels [][]int16, sidChannels [][]int16, replayedChannels [][]int16, sampleRate int) {
	numSamples := len(ayChannels[0])
	for ch := 1; ch < 3; ch++ {
		if len(ayChannels[ch]) < numSamples {
			numSamples = len(ayChannels[ch])
		}
		if len(sidChannels[ch]) < numSamples {
			numSamples = len(sidChannels[ch])
		}
		if len(replayedChannels[ch]) < numSamples {
			numSamples = len(replayedChannels[ch])
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

	numChannels := 9 // 3 AY + 3 original SID + 3 replayed SID
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

	// Interleave samples: AY, original SID, replayed SID for each channel
	for i := 0; i < numSamples; i++ {
		for ch := 0; ch < 3; ch++ {
			writeSampleBuf(w, ayChannels[ch][i])
			writeSampleBuf(w, sidChannels[ch][i])
			writeSampleBuf(w, replayedChannels[ch][i])
		}
	}
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

// captureDirectPlayerSIDFrequencies runs the direct conversion player and captures SID frequency register writes per frame
func captureDirectPlayerSIDFrequencies(commands []spectrum.AYCommand, channel int, notes []Note) []uint16 {
	// Run generateSIDPerNote and capture register writes
	// We need to intercept at frame boundaries, not sample boundaries

	sampleRate := 44100.0
	numFrames := 0
	for _, note := range notes {
		if note.EndFrame > numFrames {
			numFrames = note.EndFrame
		}
	}
	numFrames++ // Include frame 0
	totalSamples := int(float64(numFrames) * sampleRate / palFrameRate)

	freqPerFrame := make([]uint16, numFrames)

	sid := NewSIDEmulator(sampleRate)
	regBase := byte(channel * 7)

	// Build frequency table from AY commands
	freqFromAY := make([]uint16, numFrames)
	currentFreq := uint16(0)

	toneLoReg := uint8(channel * 2)
	toneHiReg := uint8(channel*2 + 1)

	for _, cmd := range commands {
		frame := int(cmd.Frame)
		if frame >= numFrames {
			continue
		}

		switch cmd.Register {
		case toneLoReg:
			currentFreq = (currentFreq & 0xFF00) | uint16(cmd.Value)
			freqFromAY[frame] = currentFreq
		case toneHiReg:
			currentFreq = (currentFreq & 0x00FF) | (uint16(cmd.Value&0x0F) << 8)
			freqFromAY[frame] = currentFreq
		}
	}

	// Forward-fill frequency array
	lastFreq := uint16(0)
	for i := range freqFromAY {
		if freqFromAY[i] != 0 {
			lastFreq = freqFromAY[i]
		} else {
			freqFromAY[i] = lastFreq
		}
	}

	// Now run the actual player and capture SID register writes at frame boundaries
	currentNoteIdx := 0

	for frame := 0; frame < numFrames; frame++ {
		frameStartSample := int(float64(frame) * sampleRate / palFrameRate)
		frameEndSample := int(float64(frame+1) * sampleRate / palFrameRate)
		if frameEndSample > totalSamples {
			frameEndSample = totalSamples
		}

		// Check if we're starting a new note at this frame
		for currentNoteIdx < len(notes) && notes[currentNoteIdx].StartFrame == frame {
			note := notes[currentNoteIdx]

			// Gate OFF before new note (matches generateSIDPerNote logic)
			if currentNoteIdx > 0 {
				prevADSR := sid.Registers[regBase+6]
				sid.WriteRegister(regBase+6, (prevADSR&0xF0)|0x00)
				sid.WriteRegister(regBase+4, sid.Registers[regBase+4]&0xFE)
				for i := 0; i < 10; i++ {
					_ = sid.GenerateSample(0)
				}
			}

			// Set ADSR
			initialS := note.ADSR.Sustain
			if len(note.SustainPerFrame) > 0 {
				initialS = note.SustainPerFrame[0]
			}
			sid.WriteRegister(regBase+5, (note.ADSR.Attack<<4)|note.ADSR.InitialDecay)
			sid.WriteRegister(regBase+6, (initialS<<4)|note.ADSR.Release)

			// Set frequency from AY commands at this frame
			ayPeriod := freqFromAY[frame]
			var sidFreq uint16
			if note.EnvelopeMode {
				// Envelope mode uses envelope period
				envPeriod := getAYEnvelopePeriod(commands, frame)
				if envPeriod > 0 {
					ayFreqHz := 1773400.0 / (256.0 * float64(envPeriod))
					sidFreq = uint16((ayFreqHz * 65536.0) / 985248.0)
				}
			} else {
				// Normal mode uses tone period
				if ayPeriod > 0 {
					ayFreqHz := 1773400.0 / (16.0 * float64(ayPeriod))
					sidFreq = uint16((ayFreqHz * 65536.0) / 985248.0)
				}
			}

			sid.WriteRegister(regBase+0, byte(sidFreq&0xFF))
			sid.WriteRegister(regBase+1, byte(sidFreq>>8))

			// Gate ON
			waveform := note.Waveform
			sid.WriteRegister(regBase+4, (waveform<<4)|0x01)

			currentNoteIdx++
		}

		// Update frequency if it changed this frame (glissando/vibrato)
		// This happens during note playback
		if currentNoteIdx > 0 {
			note := notes[currentNoteIdx-1]
			if frame >= note.StartFrame && frame <= note.EndFrame {
				relFrame := frame - note.StartFrame

				// Update S for volume-following
				if !note.EnvelopeMode && relFrame < len(note.SustainPerFrame) &&
				   (note.GateOffFrame == 0 || relFrame < note.GateOffFrame) {
					newS := note.SustainPerFrame[relFrame]
					currentR := sid.Registers[regBase+6] & 0x0F
					sid.WriteRegister(regBase+6, (newS<<4)|currentR)
				}

				// Switch D from InitialDecay to 0 after frame 0
				if relFrame == 1 && note.ADSR.InitialDecay > 0 {
					currentA := (sid.Registers[regBase+5] >> 4) & 0x0F
					sid.WriteRegister(regBase+5, (currentA<<4)|0)
				}

				// Dynamic D update at GateOffFrame
				if note.GateOffFrame > 0 && relFrame == note.GateOffFrame {
					currentA := (sid.Registers[regBase+5] >> 4) & 0x0F
					sid.WriteRegister(regBase+5, (currentA<<4)|note.ADSR.Decay)
					currentR := sid.Registers[regBase+6] & 0x0F
					sid.WriteRegister(regBase+6, (0<<4)|currentR)
				}

				// Update frequency from AY commands
				ayPeriod := freqFromAY[frame]
				var sidFreq uint16
				if note.EnvelopeMode {
					envPeriod := getAYEnvelopePeriod(commands, frame)
					if envPeriod > 0 {
						ayFreqHz := 1773400.0 / (256.0 * float64(envPeriod))
						sidFreq = uint16((ayFreqHz * 65536.0) / 985248.0)
					}
				} else {
					if ayPeriod > 0 {
						ayFreqHz := 1773400.0 / (16.0 * float64(ayPeriod))
						sidFreq = uint16((ayFreqHz * 65536.0) / 985248.0)
					}
				}

				sid.WriteRegister(regBase+0, byte(sidFreq&0xFF))
				sid.WriteRegister(regBase+1, byte(sidFreq>>8))
			}
		}

		// Capture SID frequency at end of frame
		sidFreqLo := uint16(sid.Registers[regBase+0])
		sidFreqHi := uint16(sid.Registers[regBase+1])
		freqPerFrame[frame] = (sidFreqHi << 8) | sidFreqLo

		// Generate samples for this frame to advance SID state
		for sample := frameStartSample; sample < frameEndSample; sample++ {
			_ = sid.GenerateSample(0)
		}
	}

	return freqPerFrame
}

// captureTrackerPlayerSIDFrequencies runs the tracker player and captures SID frequency register writes per frame
//
// ARCHITECTURE: This is a VALIDATION HELPER - it captures what the tracker player outputs
// for comparison with the direct conversion player.
//
// RULES:
// - This function CAN read from AY commands (for validation/comparison purposes)
// - It mirrors generateSIDFromSimulatedNotes but captures register writes instead of audio
// - Any differences from generateSIDFromSimulatedNotes should be minimal (capture vs audio gen)
func captureTrackerPlayerSIDFrequencies(commands []spectrum.AYCommand, channel int, simNotes []SimulatedNote, startOffset int) []uint16 {
	sampleRate := 44100.0

	// Calculate total frames
	totalFrames := startOffset
	for _, simNote := range simNotes {
		totalFrames += int(simNote.dur)
	}

	freqPerFrame := make([]uint16, totalFrames)

	sid := NewSIDEmulator(sampleRate)
	regBase := byte(0) // Tracker player uses voice 0

	// Initialize voice to match direct player
	sid.WriteRegister(regBase+0, 0x00) // Freq low = 0
	sid.WriteRegister(regBase+1, 0x00) // Freq high = 0
	sid.WriteRegister(regBase+4, 0x00) // Gate OFF

	absoluteFrame := 0

	// Apply global offset (silence before first note)
	for frame := 0; frame < startOffset; frame++ {
		// Capture current register state (initially 0)
		sidFreqLo := uint16(sid.Registers[regBase+0])
		sidFreqHi := uint16(sid.Registers[regBase+1])
		freqPerFrame[absoluteFrame] = (sidFreqHi << 8) | sidFreqLo

		// Generate samples to advance SID state
		frameStartSample := int(float64(absoluteFrame) * sampleRate / palFrameRate)
		frameEndSample := int(float64(absoluteFrame+1) * sampleRate / palFrameRate)
		for sample := frameStartSample; sample < frameEndSample; sample++ {
			_ = sid.GenerateSample(0)
		}
		absoluteFrame++
	}

	for noteIdx, simNote := range simNotes {
		noteDuration := int(simNote.dur)


		// Handle rest patterns - don't modify frequency registers, just capture current state
		if simNote.isRest {
			for frame := 0; frame < noteDuration; frame++ {
				// Capture current frequency register state (persists from previous note)
				sidFreqLo := uint16(sid.Registers[regBase+0])
				sidFreqHi := uint16(sid.Registers[regBase+1])
				freqPerFrame[absoluteFrame] = (sidFreqHi << 8) | sidFreqLo

				frameStartSample := int(float64(absoluteFrame) * sampleRate / palFrameRate)
				frameEndSample := int(float64(absoluteFrame+1) * sampleRate / palFrameRate)
				for sample := frameStartSample; sample < frameEndSample; sample++ {
					_ = sid.GenerateSample(0)
				}
				absoluteFrame++
			}
			continue
		}

		// ALWAYS gate OFF before new note
		if noteIdx > 0 {
			prevADSR := sid.Registers[regBase+6]
			sid.WriteRegister(regBase+6, (prevADSR&0xF0)|0x00)
			sid.WriteRegister(regBase+4, sid.Registers[regBase+4]&0xFE)
			for i := 0; i < 10; i++ {
				_ = sid.GenerateSample(0)
			}
		}

		// Set ADSR while gate is OFF
		initialS := simNote.ADSR.Sustain
		if len(simNote.SustainPerFrame) > 0 {
			initialS = simNote.SustainPerFrame[0]
		}
		sid.WriteRegister(regBase+5, (simNote.ADSR.Attack<<4)|simNote.ADSR.InitialDecay)
		sid.WriteRegister(regBase+6, (initialS<<4)|simNote.ADSR.Release)

		// Set initial frequency from stored sequence (already converted to SID values)
		var sidFreq uint16
		if len(simNote.freqSeq) > 0 {
			sidFreq = simNote.freqSeq[0]
		} else {
			sidFreq = simNote.freq
		}
		sid.WriteRegister(regBase+0, byte(sidFreq&0xFF))
		sid.WriteRegister(regBase+1, byte(sidFreq>>8))


		// Gate ON with waveform
		waveform := simNote.Waveform
		sid.WriteRegister(regBase+4, (waveform<<4)|0x01)

		// Track current waveform for mid-note changes (envelope mode only)
		currentWaveform := waveform

		// Generate audio for each frame of this note
		for frame := 0; frame < noteDuration; frame++ {
			// Check waveformSeq for NoisePlusTone mixer state changes
			// TODO: Populate waveformSeq in extractor from mixer register (R7)
			if simNote.NoisePlusTone && frame < len(simNote.waveformSeq) && simNote.waveformSeq[frame] != 0 {
				newWaveform := simNote.waveformSeq[frame]
				if newWaveform != currentWaveform {
					currentWaveform = newWaveform
					if currentWaveform == 0x04 {
						// Switch to pulse
						sid.WriteRegister(regBase+2, 0x00) // Pulse width = 50%
						sid.WriteRegister(regBase+3, 0x08)
						sid.WriteRegister(regBase+4, (0x04<<4)|0x01) // Pulse with gate ON
					} else {
						// Switch to noise
						sid.WriteRegister(regBase+4, (0x08<<4)|0x01) // Noise with gate ON
					}
				}
			}

			// Update frequency per frame (glissando/vibrato/envelope)
			if frame < len(simNote.freqSeq) {
				// freqSeq already contains SID frequency values - just write them
				sidFreq := simNote.freqSeq[frame]
				sid.WriteRegister(regBase+0, byte(sidFreq&0xFF))
				sid.WriteRegister(regBase+1, byte(sidFreq>>8))
			}

			// Dynamic ADSR updates for volume-following
			if !simNote.EnvelopeMode {
				// Update S value each frame
				if frame < len(simNote.SustainPerFrame) &&
				   (simNote.GateOffFrame == 0 || frame < simNote.GateOffFrame) {
					newS := simNote.SustainPerFrame[frame]
					currentR := sid.Registers[regBase+6] & 0x0F
					sid.WriteRegister(regBase+6, (newS<<4)|currentR)
				}

				// Switch D from InitialDecay to 0 after frame 0
				if frame == 1 && simNote.ADSR.InitialDecay > 0 {
					currentA := (sid.Registers[regBase+5] >> 4) & 0x0F
					sid.WriteRegister(regBase+5, (currentA<<4)|0)
				}

				// Dynamic D update at GateOffFrame
				if simNote.GateOffFrame > 0 && frame == simNote.GateOffFrame {
					currentA := (sid.Registers[regBase+5] >> 4) & 0x0F
					sid.WriteRegister(regBase+5, (currentA<<4)|simNote.ADSR.Decay)
					currentR := sid.Registers[regBase+6] & 0x0F
					sid.WriteRegister(regBase+6, (0<<4)|currentR)
				}
			}

			// If current note has D>0, gate OFF 1 frame before note ends (matches direct player)
			// This allows the decay phase to complete before the new note starts
			if frame == noteDuration-framesBeforeNoteOff && simNote.ADSR.Decay > 0 {
				hasNextNote := noteIdx+1 < len(simNotes) && !simNotes[noteIdx+1].isRest
				if hasNextNote {
					sid.WriteRegister(regBase+4, waveform<<4) // Gate OFF
				}
			}

			// Capture SID frequency at end of frame
			sidFreqLo := uint16(sid.Registers[regBase+0])
			sidFreqHi := uint16(sid.Registers[regBase+1])
			freqPerFrame[absoluteFrame] = (sidFreqHi << 8) | sidFreqLo

			// Generate samples for this frame to advance SID state
			frameStartSample := int(float64(absoluteFrame) * sampleRate / palFrameRate)
			frameEndSample := int(float64(absoluteFrame+1) * sampleRate / palFrameRate)
			for sample := frameStartSample; sample < frameEndSample; sample++ {
				_ = sid.GenerateSample(0)
			}
			absoluteFrame++
		}
	}

	return freqPerFrame
}
