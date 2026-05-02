// Analyze Complex patterns to find underlying tracker effects/mechanisms
package main

import (
	"fmt"
)

// Analyze what simple tracker mechanisms generate the "Complex" patterns
// Focus on exact parameter extraction for implementation
func AnalyzeTrackerMechanisms(notes []NoteWithMetadata) {
	fmt.Printf("\n=== Reverse-Engineering Tracker Parameters ===\n")
	fmt.Printf("Goal: Extract generation parameters for each pattern\n\n")

	complexPatterns := []struct {
		freqs    []uint16
		deltas   []int16
		noteIdx  int
		channel  int
		analysis MechanismAnalysis
	}{}

	// Collect all patterns that would be encoded as Complex (Type 4)
	for i, note := range notes {
		if note.isRest || len(note.freqSeq) <= 1 {
			continue
		}

		// Check if frequency changes
		hasChange := false
		for j := 1; j < len(note.freqSeq); j++ {
			if note.freqSeq[j] != note.freqSeq[0] {
				hasChange = true
				break
			}
		}
		if !hasChange {
			continue
		}

		// Skip drum patterns
		if _, matches := matchDrumSequence(note.freqSeq); matches {
			continue
		}

		// Skip delayed vibrato
		if _, _, matches := matchDelayedVibrato(note.freqSeq); matches {
			continue
		}

		// This would be encoded as Complex - analyze it
		deltas := make([]int16, len(note.freqSeq)-1)
		for j := 1; j < len(note.freqSeq); j++ {
			deltas[j-1] = int16(note.freqSeq[j]) - int16(note.freqSeq[j-1])
		}

		analysis := analyzeEffectMechanism(note.freqSeq, deltas)

		complexPatterns = append(complexPatterns, struct {
			freqs    []uint16
			deltas   []int16
			noteIdx  int
			channel  int
			analysis MechanismAnalysis
		}{
			freqs:    note.freqSeq,
			deltas:   deltas,
			noteIdx:  i,
			channel:  note.Channel,
			analysis: analysis,
		})
	}

	fmt.Printf("Found %d Complex patterns to analyze\n\n", len(complexPatterns))

	// Categorize by mechanism type
	linearRamps := 0
	acceleratingRamps := 0
	deceleratingRamps := 0
	stepFunctions := 0
	periodicOscillations := 0
	hybridPatterns := 0

	for i, p := range complexPatterns {
		if i < 30 { // Show first 30 in detail
			fmt.Printf("Pattern %d (ch%d note%d): len=%d, %s\n",
				i, p.channel, p.noteIdx, len(p.freqs), p.analysis.Description)
			fmt.Printf("  Freqs: ")
			for j := 0; j < min(10, len(p.freqs)); j++ {
				fmt.Printf("%d ", p.freqs[j])
			}
			if len(p.freqs) > 10 {
				fmt.Printf("... ")
			}
			fmt.Printf("\n")
			fmt.Printf("  Deltas: ")
			for j := 0; j < min(10, len(p.deltas)); j++ {
				fmt.Printf("%+d ", p.deltas[j])
			}
			if len(p.deltas) > 10 {
				fmt.Printf("... ")
			}
			fmt.Printf("\n")
			fmt.Printf("  → %s\n", p.analysis.EncodingRecommendation)
			if len(p.analysis.Parameters) > 0 {
				fmt.Printf("  → Parameters: %v\n", p.analysis.Parameters)
			}
			fmt.Printf("\n")
		}

		switch p.analysis.Type {
		case "linear":
			linearRamps++
		case "accelerating":
			acceleratingRamps++
		case "decelerating":
			deceleratingRamps++
		case "step":
			stepFunctions++
		case "periodic":
			periodicOscillations++
		case "hybrid":
			hybridPatterns++
		}
	}

	fmt.Printf("\n=== Mechanism Classification ===\n")
	total := len(complexPatterns)
	fmt.Printf("Linear ramps:       %3d (%.1f%%) - constant velocity\n", linearRamps, pct(linearRamps, total))
	fmt.Printf("Accelerating ramps: %3d (%.1f%%) - increasing velocity\n", acceleratingRamps, pct(acceleratingRamps, total))
	fmt.Printf("Decelerating ramps: %3d (%.1f%%) - decreasing velocity\n", deceleratingRamps, pct(deceleratingRamps, total))
	fmt.Printf("Step functions:     %3d (%.1f%%) - discrete value changes\n", stepFunctions, pct(stepFunctions, total))
	fmt.Printf("Periodic oscillate: %3d (%.1f%%) - repeating wave\n", periodicOscillations, pct(periodicOscillations, total))
	fmt.Printf("Hybrid/complex:     %3d (%.1f%%) - combination\n", hybridPatterns, pct(hybridPatterns, total))

	// Estimate encoding savings
	fmt.Printf("\n=== Encoding Efficiency ===\n")
	currentBytes := 0
	optimizedBytes := 0

	for _, p := range complexPatterns {
		analysis := analyzeEffectMechanism(p.freqs, p.deltas)
		currentBytes += 2 + len(p.deltas)*2 // current: startFreq + deltas
		optimizedBytes += analysis.EncodingBytes
	}

	fmt.Printf("Current Complex encoding: %d bytes\n", currentBytes)
	fmt.Printf("Optimized encoding:       %d bytes\n", optimizedBytes)
	fmt.Printf("Potential savings:        %d bytes (%.1f%% reduction)\n",
		currentBytes-optimizedBytes,
		float64(currentBytes-optimizedBytes)*100.0/float64(currentBytes))
}

type MechanismAnalysis struct {
	Type                    string
	Description             string
	EncodingRecommendation  string
	EncodingBytes           int
	Parameters              map[string]interface{}
}

func analyzeEffectMechanism(freqs []uint16, deltas []int16) MechanismAnalysis {
	if len(freqs) <= 1 {
		return MechanismAnalysis{
			Type:          "constant",
			Description:   "No change",
			EncodingBytes: 2,
		}
	}

	// Check for linear ramp (constant delta)
	if isConstantDelta(deltas) {
		return MechanismAnalysis{
			Type:                   "linear",
			Description:            fmt.Sprintf("Linear ramp: velocity=%+d", deltas[0]),
			EncodingRecommendation: fmt.Sprintf("LINEAR: start=%d, delta=%+d, frames=%d (4 bytes)", freqs[0], deltas[0], len(freqs)),
			EncodingBytes:          4, // startFreq(2) + delta(2)
			Parameters: map[string]interface{}{
				"start":  freqs[0],
				"delta":  deltas[0],
				"frames": len(freqs),
			},
		}
	}

	// Check for step function (values hold for multiple frames)
	if isStepFunction(deltas) {
		steps := countSteps(deltas)
		return MechanismAnalysis{
			Type:                   "step",
			Description:            fmt.Sprintf("Step function: %d steps", steps),
			EncodingRecommendation: fmt.Sprintf("STEP: %d unique values + durations (%d bytes)", steps, steps*3),
			EncodingBytes:          steps * 3, // value(2) + duration(1) per step
			Parameters: map[string]interface{}{
				"steps": steps,
			},
		}
	}

	// Check for constant acceleration (2nd derivative constant)
	if accel, isAccel := isConstantAcceleration(deltas); isAccel {
		direction := "accelerating"
		if accel < 0 {
			direction = "decelerating"
		}
		return MechanismAnalysis{
			Type:                   direction,
			Description:            fmt.Sprintf("%s: accel=%+d", direction, accel),
			EncodingRecommendation: fmt.Sprintf("ACCEL: start=%d, velocity=%+d, accel=%+d, frames=%d (6 bytes)", freqs[0], deltas[0], accel, len(freqs)),
			EncodingBytes:          6, // startFreq(2) + velocity(2) + accel(2)
			Parameters: map[string]interface{}{
				"start":    freqs[0],
				"velocity": deltas[0],
				"accel":    accel,
				"frames":   len(freqs),
			},
		}
	}

	// Check for periodic oscillation
	if period := findPeriod(deltas); period > 0 && period < len(deltas)/2 {
		return MechanismAnalysis{
			Type:                   "periodic",
			Description:            fmt.Sprintf("Periodic oscillation: period=%d", period),
			EncodingRecommendation: fmt.Sprintf("PERIODIC: base=%d, period=%d, pattern=%d bytes (%d bytes)", freqs[0], period, period*2, 2+1+period*2),
			EncodingBytes:          2 + 1 + period*2, // baseFreq(2) + period(1) + deltas
			Parameters: map[string]interface{}{
				"base":   freqs[0],
				"period": period,
			},
		}
	}

	// Check for segmented pattern (different mechanisms in different sections)
	segments := analyzeSegments(freqs, deltas)
	if len(segments) > 1 {
		totalBytes := 1 // segment count
		for _, seg := range segments {
			totalBytes += seg.encodingBytes + 1 // +1 for segment length
		}
		return MechanismAnalysis{
			Type:                   "hybrid",
			Description:            fmt.Sprintf("Hybrid: %d segments", len(segments)),
			EncodingRecommendation: fmt.Sprintf("HYBRID: %d segments (%d bytes)", len(segments), totalBytes),
			EncodingBytes:          totalBytes,
			Parameters: map[string]interface{}{
				"segments": len(segments),
			},
		}
	}

	// Fallback: use delta encoding
	return MechanismAnalysis{
		Type:                   "hybrid",
		Description:            "No clear mechanism - use delta encoding",
		EncodingRecommendation: fmt.Sprintf("COMPLEX: full delta encoding (%d bytes)", 2+len(deltas)*2),
		EncodingBytes:          2 + len(deltas)*2,
	}
}

func isConstantDelta(deltas []int16) bool {
	if len(deltas) == 0 {
		return true
	}
	first := deltas[0]
	for _, d := range deltas[1:] {
		if d != first {
			return false
		}
	}
	return true
}

func isStepFunction(deltas []int16) bool {
	// Count how many deltas are zero (holding values)
	zeros := 0
	for _, d := range deltas {
		if d == 0 {
			zeros++
		}
	}
	// If >50% of frames hold values, it's a step function
	return zeros > len(deltas)/2
}

func countSteps(deltas []int16) int {
	steps := 1
	for _, d := range deltas {
		if d != 0 {
			steps++
		}
	}
	return steps
}

func isConstantAcceleration(deltas []int16) (int16, bool) {
	if len(deltas) < 3 {
		return 0, false
	}

	// Compute 2nd derivative (delta of deltas)
	secondDerivs := make([]int16, len(deltas)-1)
	for i := 1; i < len(deltas); i++ {
		secondDerivs[i-1] = deltas[i] - deltas[i-1]
	}

	// Check if 2nd derivative is constant
	if len(secondDerivs) == 0 {
		return 0, false
	}

	accel := secondDerivs[0]
	if accel == 0 {
		return 0, false // this is linear, not accelerating
	}

	for _, a := range secondDerivs[1:] {
		if a != accel {
			return 0, false
		}
	}

	return accel, true
}

func findPeriod(deltas []int16) int {
	// Try periods from 2 to min(32, len/2)
	maxPeriod := min(32, len(deltas)/2)
	for period := 2; period <= maxPeriod; period++ {
		matches := true
		for i := period; i < len(deltas); i++ {
			if deltas[i] != deltas[i%period] {
				matches = false
				break
			}
		}
		if matches {
			return period
		}
	}
	return 0
}

type segment struct {
	start         int
	length        int
	mechanism     string
	encodingBytes int
}

func analyzeSegments(freqs []uint16, deltas []int16) []segment {
	// Try to split pattern into simpler segments
	// For now, just return empty if no obvious segmentation
	// TODO: implement segment detection
	return []segment{}
}

func pct(count, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(count) * 100.0 / float64(total)
}
