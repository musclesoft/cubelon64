// Effect parameter analysis - find mathematical functions behind frequency patterns
package main

import (
	"fmt"
	"math"
)

// EffectType represents different mathematical functions
type EffectType int

const (
	EffectLinear       EffectType = 0 // Linear interpolation (portamento)
	EffectExponential  EffectType = 1 // Exponential decay/growth
	EffectLogarithmic  EffectType = 2 // Logarithmic curve
	EffectConstant     EffectType = 3 // No change (hold)
	EffectVibrato      EffectType = 4 // Sinusoidal oscillation
	EffectComplex      EffectType = 5 // No simple function
)

type EffectParams struct {
	Type       EffectType
	StartFreq  uint16
	EndFreq    uint16
	Duration   int
	Delay      int    // Frames before effect starts
	Amplitude  int    // For vibrato/oscillation
	Period     int    // For vibrato/oscillation
	Curve      float64 // For exponential (e.g., 2.0 = quadratic)
}

// AnalyzeFrequencyPattern determines the underlying mathematical function
func AnalyzeFrequencyPattern(freqs []uint16) EffectParams {
	if len(freqs) == 0 {
		return EffectParams{Type: EffectConstant}
	}

	// Check for constant (no change)
	isConstant := true
	for _, f := range freqs {
		if f != freqs[0] {
			isConstant = false
			break
		}
	}
	if isConstant {
		return EffectParams{
			Type:      EffectConstant,
			StartFreq: freqs[0],
			EndFreq:   freqs[0],
			Duration:  len(freqs),
		}
	}

	// Count leading zeros (delay)
	delay := 0
	for i := 1; i < len(freqs) && freqs[i] == freqs[0]; i++ {
		delay++
	}

	activeFreqs := freqs[delay:]
	if len(activeFreqs) < 2 {
		return EffectParams{
			Type:      EffectConstant,
			StartFreq: freqs[0],
			EndFreq:   freqs[0],
			Duration:  len(freqs),
			Delay:     delay,
		}
	}

	startFreq := activeFreqs[0]
	endFreq := activeFreqs[len(activeFreqs)-1]

	// Check for monotonic (always increasing or always decreasing)
	isMonotonic := true
	lastDir := 0
	for i := 1; i < len(activeFreqs); i++ {
		delta := int(activeFreqs[i]) - int(activeFreqs[i-1])
		if delta != 0 {
			dir := 1
			if delta < 0 {
				dir = -1
			}
			if lastDir != 0 && dir != lastDir {
				isMonotonic = false
				break
			}
			lastDir = dir
		}
	}

	// If monotonic, check if linear, exponential, or logarithmic
	if isMonotonic {
		linearError := checkLinearFit(activeFreqs)
		expError := checkExponentialFit(activeFreqs)
		logError := checkLogarithmicFit(activeFreqs)

		// Choose best fit (lowest error)
		minError := linearError
		effectType := EffectLinear
		curve := 1.0

		if expError < minError {
			minError = expError
			effectType = EffectExponential
			curve = estimateExponentialCurve(activeFreqs)
		}

		if logError < minError {
			minError = logError
			effectType = EffectLogarithmic
		}

		// If error is low enough, use the simple function
		if minError < 0.05 { // 5% error threshold
			return EffectParams{
				Type:      effectType,
				StartFreq: startFreq,
				EndFreq:   endFreq,
				Duration:  len(activeFreqs),
				Delay:     delay,
				Curve:     curve,
			}
		}
	}

	// Check for vibrato/oscillation (returns to start)
	if activeFreqs[0] == activeFreqs[len(activeFreqs)-1] {
		vibratoParams := detectVibrato(activeFreqs)
		if vibratoParams != nil {
			vibratoParams.Delay = delay
			return *vibratoParams
		}
	}

	// Complex pattern - no simple function
	return EffectParams{
		Type:      EffectComplex,
		StartFreq: startFreq,
		EndFreq:   endFreq,
		Duration:  len(activeFreqs),
		Delay:     delay,
	}
}

// checkLinearFit calculates error for linear interpolation
func checkLinearFit(freqs []uint16) float64 {
	if len(freqs) < 2 {
		return 0.0
	}

	start := float64(freqs[0])
	end := float64(freqs[len(freqs)-1])
	duration := float64(len(freqs) - 1)

	totalError := 0.0
	for i, actual := range freqs {
		expected := start + (end-start)*float64(i)/duration
		error := math.Abs(float64(actual) - expected)
		totalError += error
	}

	avgError := totalError / float64(len(freqs))
	maxValue := math.Max(math.Abs(start), math.Abs(end))
	if maxValue == 0 {
		return 0.0
	}
	return avgError / maxValue
}

// checkExponentialFit calculates error for exponential curve
func checkExponentialFit(freqs []uint16) float64 {
	if len(freqs) < 3 {
		return 1.0 // Need at least 3 points
	}

	start := float64(freqs[0])
	end := float64(freqs[len(freqs)-1])

	if start == 0 || end == 0 {
		return 1.0 // Can't do exponential with zero
	}

	duration := float64(len(freqs) - 1)
	ratio := end / start

	totalError := 0.0
	for i, actual := range freqs {
		t := float64(i) / duration
		expected := start * math.Pow(ratio, t)
		error := math.Abs(float64(actual) - expected)
		totalError += error
	}

	avgError := totalError / float64(len(freqs))
	maxValue := math.Max(math.Abs(start), math.Abs(end))
	return avgError / maxValue
}

// checkLogarithmicFit calculates error for logarithmic curve
func checkLogarithmicFit(freqs []uint16) float64 {
	if len(freqs) < 3 {
		return 1.0
	}

	start := float64(freqs[0])
	end := float64(freqs[len(freqs)-1])
	duration := float64(len(freqs) - 1)

	totalError := 0.0
	for i, actual := range freqs {
		t := float64(i) / duration
		// Logarithmic: y = a + b*log(1+ct)
		logT := math.Log(1.0 + 9.0*t) / math.Log(10.0) // 0 to 1
		expected := start + (end-start)*logT
		error := math.Abs(float64(actual) - expected)
		totalError += error
	}

	avgError := totalError / float64(len(freqs))
	maxValue := math.Max(math.Abs(start), math.Abs(end))
	if maxValue == 0 {
		return 0.0
	}
	return avgError / maxValue
}

// estimateExponentialCurve estimates the curve parameter for exponential fit
func estimateExponentialCurve(freqs []uint16) float64 {
	if len(freqs) < 3 {
		return 2.0
	}

	start := float64(freqs[0])
	end := float64(freqs[len(freqs)-1])

	if start == 0 || end == 0 || start == end {
		return 2.0
	}

	// Sample middle point to estimate curve
	mid := freqs[len(freqs)/2]
	ratio := end / start
	t := 0.5

	// Solve: mid = start * ratio^(t^curve) for curve
	actualRatio := float64(mid) / start
	if actualRatio <= 0 || ratio <= 0 {
		return 2.0
	}

	curve := math.Log(math.Log(actualRatio)/math.Log(ratio)) / math.Log(t)

	// Clamp to reasonable range
	if curve < 0.5 || curve > 4.0 || math.IsNaN(curve) || math.IsInf(curve, 0) {
		return 2.0
	}

	return curve
}

// detectVibrato checks if pattern is sinusoidal oscillation
func detectVibrato(freqs []uint16) *EffectParams {
	if len(freqs) < 4 {
		return nil
	}

	// Find center frequency (average)
	sum := 0
	for _, f := range freqs {
		sum += int(f)
	}
	center := uint16(sum / len(freqs))

	// Find amplitude (max deviation from center)
	maxDev := 0
	for _, f := range freqs {
		dev := int(f) - int(center)
		if dev < 0 {
			dev = -dev
		}
		if dev > maxDev {
			maxDev = dev
		}
	}

	// Try to detect period
	period := detectPeriod(freqs, center)
	if period == 0 {
		return nil
	}

	// Check if sinusoidal fit is good
	totalError := 0.0
	for i, actual := range freqs {
		phase := 2.0 * math.Pi * float64(i) / float64(period)
		expected := float64(center) + float64(maxDev)*math.Sin(phase)
		error := math.Abs(float64(actual) - expected)
		totalError += error
	}

	avgError := totalError / float64(len(freqs))
	if avgError/float64(maxDev) > 0.3 { // 30% error threshold
		return nil
	}

	return &EffectParams{
		Type:      EffectVibrato,
		StartFreq: center,
		Duration:  len(freqs),
		Amplitude: maxDev,
		Period:    period,
	}
}

// detectPeriod finds the period of oscillation
func detectPeriod(freqs []uint16, center uint16) int {
	// Look for zero crossings
	crossings := []int{}
	lastAbove := int(freqs[0]) > int(center)

	for i := 1; i < len(freqs); i++ {
		above := int(freqs[i]) > int(center)
		if above != lastAbove {
			crossings = append(crossings, i)
			lastAbove = above
		}
	}

	if len(crossings) < 2 {
		return 0
	}

	// Period is 2× average distance between crossings
	totalDist := 0
	for i := 1; i < len(crossings); i++ {
		totalDist += crossings[i] - crossings[i-1]
	}
	avgDist := totalDist / (len(crossings) - 1)

	return avgDist * 2
}

// AnalyzeAllPatterns analyzes all frequency patterns in the music
func AnalyzeAllPatterns(notes []NoteWithMetadata) {
	fmt.Printf("\n=== Effect Parameter Analysis ===\n")

	effectCounts := make(map[EffectType]int)
	effectExamples := make(map[EffectType][]EffectParams)

	for _, note := range notes {
		if note.isRest || len(note.freqSeq) <= 1 {
			continue
		}

		params := AnalyzeFrequencyPattern(note.freqSeq)
		effectCounts[params.Type]++

		// Keep first 3 examples of each type
		if len(effectExamples[params.Type]) < 3 {
			effectExamples[params.Type] = append(effectExamples[params.Type], params)
		}
	}

	// Print summary
	fmt.Printf("\nEffect type distribution:\n")
	for effectType, count := range effectCounts {
		name := "Unknown"
		switch effectType {
		case EffectLinear:
			name = "Linear"
		case EffectExponential:
			name = "Exponential"
		case EffectLogarithmic:
			name = "Logarithmic"
		case EffectConstant:
			name = "Constant"
		case EffectVibrato:
			name = "Vibrato"
		case EffectComplex:
			name = "Complex"
		}
		fmt.Printf("  %s: %d patterns\n", name, count)
	}

	// Print examples
	fmt.Printf("\nExample parameters:\n")
	for effectType, examples := range effectExamples {
		if len(examples) == 0 {
			continue
		}

		name := "Unknown"
		switch effectType {
		case EffectLinear:
			name = "Linear"
		case EffectExponential:
			name = "Exponential"
		case EffectLogarithmic:
			name = "Logarithmic"
		case EffectConstant:
			name = "Constant"
		case EffectVibrato:
			name = "Vibrato"
		case EffectComplex:
			name = "Complex"
		}

		fmt.Printf("\n%s examples:\n", name)
		for i, ex := range examples {
			fmt.Printf("  %d: start=%d end=%d dur=%d delay=%d", i+1, ex.StartFreq, ex.EndFreq, ex.Duration, ex.Delay)
			if ex.Type == EffectExponential {
				fmt.Printf(" curve=%.2f", ex.Curve)
			} else if ex.Type == EffectVibrato {
				fmt.Printf(" amp=%d period=%d", ex.Amplitude, ex.Period)
			}
			fmt.Printf("\n")
		}
	}
}
