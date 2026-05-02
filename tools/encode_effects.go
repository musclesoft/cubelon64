// Effect-based encoding - store mathematical parameters instead of frame deltas
package main

import (
	"fmt"
	"math"
)

// EffectPattern stores effect as mathematical function parameters
type EffectPattern struct {
	Type      byte   // 0=Linear, 1=Exponential, 2=Logarithmic, 3=Vibrato, 4=Complex
	Delay     byte   // Frames before effect starts
	Duration  byte   // Total frames including delay
	StartFreq uint16 // Starting frequency
	EndFreq   uint16 // Ending frequency (or center for vibrato)
	Param1    byte   // Curve for exp/log, amplitude for vibrato
	Param2    byte   // Period for vibrato
}

// EncodeAsEffect converts frequency sequence to effect parameters
func EncodeAsEffect(freqs []uint16, threshold float64) EffectPattern {
	if len(freqs) == 0 {
		return EffectPattern{Type: 4} // Complex fallback
	}

	params := AnalyzeFrequencyPattern(freqs)

	// Map EffectType to encoding type
	var encType byte
	switch params.Type {
	case EffectLinear:
		encType = 0
	case EffectExponential:
		encType = 1
	case EffectLogarithmic:
		encType = 2
	case EffectVibrato:
		encType = 3
	case EffectConstant, EffectComplex:
		encType = 4 // Complex fallback
	default:
		encType = 4
	}

	// Encode parameters
	pattern := EffectPattern{
		Type:      encType,
		Delay:     byte(params.Delay),
		Duration:  byte(params.Duration + params.Delay),
		StartFreq: params.StartFreq,
		EndFreq:   params.EndFreq,
	}

	if params.Type == EffectExponential || params.Type == EffectLogarithmic {
		// Encode curve as byte (0-255 maps to 0.5-4.0)
		curve := params.Curve
		if curve < 0.5 {
			curve = 0.5
		}
		if curve > 4.0 {
			curve = 4.0
		}
		pattern.Param1 = byte((curve - 0.5) * 255.0 / 3.5)
	} else if params.Type == EffectVibrato {
		pattern.Param1 = byte(params.Amplitude)
		pattern.Param2 = byte(params.Period)
	}

	return pattern
}

// DecodeEffect expands effect parameters into frequency sequence
func DecodeEffect(pattern EffectPattern) []uint16 {
	result := make([]uint16, pattern.Duration)

	// Hold at start frequency during delay
	for i := 0; i < int(pattern.Delay); i++ {
		result[i] = pattern.StartFreq
	}

	activeDur := int(pattern.Duration) - int(pattern.Delay)
	if activeDur <= 0 {
		return result
	}

	start := float64(pattern.StartFreq)
	end := float64(pattern.EndFreq)

	switch pattern.Type {
	case 0: // Linear
		for i := 0; i < activeDur; i++ {
			t := float64(i) / float64(activeDur-1)
			freq := start + (end-start)*t
			result[int(pattern.Delay)+i] = uint16(freq)
		}

	case 1: // Exponential
		if start == 0 || end == 0 {
			// Fallback to linear
			for i := 0; i < activeDur; i++ {
				t := float64(i) / float64(activeDur-1)
				freq := start + (end-start)*t
				result[int(pattern.Delay)+i] = uint16(freq)
			}
		} else {
			// Decode curve from byte
			curve := 0.5 + float64(pattern.Param1)*3.5/255.0
			ratio := end / start

			for i := 0; i < activeDur; i++ {
				t := float64(i) / float64(activeDur-1)
				tCurved := math.Pow(t, curve)
				freq := start * math.Pow(ratio, tCurved)
				result[int(pattern.Delay)+i] = uint16(freq)
			}
		}

	case 2: // Logarithmic
		for i := 0; i < activeDur; i++ {
			t := float64(i) / float64(activeDur-1)
			logT := math.Log(1.0+9.0*t) / math.Log(10.0) // 0 to 1
			freq := start + (end-start)*logT
			result[int(pattern.Delay)+i] = uint16(freq)
		}

	case 3: // Vibrato
		amplitude := float64(pattern.Param1)
		period := float64(pattern.Param2)
		center := float64(pattern.StartFreq)

		for i := 0; i < activeDur; i++ {
			phase := 2.0 * math.Pi * float64(i) / period
			freq := center + amplitude*math.Sin(phase)
			result[int(pattern.Delay)+i] = uint16(freq)
		}

	case 4: // Complex - should not use this, fallback to constant
		for i := 0; i < activeDur; i++ {
			result[int(pattern.Delay)+i] = pattern.StartFreq
		}
	}

	return result
}

// BuildEffectPatterns creates effect-based patterns
func BuildEffectPatterns(notes []NoteWithMetadata, freqSeqMap map[string]int) ([]EffectPattern, map[string]int, int, int) {
	effectMap := make(map[string]int)
	patterns := []EffectPattern{}
	effectCount := 0  // Patterns that fit effect model
	complexCount := 0 // Patterns that need Complex encoding

	for _, note := range notes {
		if note.isRest || len(note.freqSeq) <= 1 {
			continue
		}

		// Check if frequency actually changes
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

		// Try to encode as effect
		pattern := EncodeAsEffect(note.freqSeq, 0.05)

		// Verify by decoding
		decoded := DecodeEffect(pattern)

		// Check reconstruction error
		maxError := 0
		for i := 0; i < min(len(note.freqSeq), len(decoded)); i++ {
			error := int(note.freqSeq[i]) - int(decoded[i])
			if error < 0 {
				error = -error
			}
			if error > maxError {
				maxError = error
			}
		}

		// If error is acceptable, use effect encoding
		maxFreq := note.freqSeq[0]
		for _, f := range note.freqSeq {
			if f > maxFreq {
				maxFreq = f
			}
		}
		relativeError := float64(maxError) / float64(maxFreq)

		if pattern.Type != 4 && relativeError < 0.05 { // 5% error threshold
			// Use effect encoding
			effectCount++

			// Create key from effect parameters
			key := fmt.Sprintf("effect_%d_%d_%d_%d_%d_%d_%d",
				pattern.Type, pattern.Delay, pattern.Duration,
				pattern.StartFreq, pattern.EndFreq, pattern.Param1, pattern.Param2)

			if _, exists := effectMap[key]; !exists {
				effectMap[key] = len(patterns)
				patterns = append(patterns, pattern)
			}

			// Also add to freqSeqMap with the original key format
			origKey := fmt.Sprintf("%d", note.freqSeq[0])
			for j := 1; j < len(note.freqSeq); j++ {
				delta := int16(note.freqSeq[j]) - int16(note.freqSeq[j-1])
				origKey += fmt.Sprintf(",%d", delta)
			}
			freqSeqMap[origKey] = effectMap[key]
		} else {
			// Use Complex encoding (frame-by-frame deltas)
			complexCount++
		}
	}

	fmt.Printf("  Effect analysis: %d patterns fit effect model (%.1f%%), %d need Complex encoding\n",
		effectCount, float64(effectCount)*100.0/float64(effectCount+complexCount), complexCount)

	return patterns, effectMap, effectCount, complexCount
}

// EstimateEffectPatternSize calculates byte size of effect pattern
func EstimateEffectPatternSize(pattern EffectPattern) int {
	// Header: type(1) + delay(1) + duration(1) = 3 bytes
	// Frequencies: start(2) + end(2) = 4 bytes
	// Parameters: param1(1) + param2(1) = 2 bytes (if needed)

	baseSize := 7 // type + delay + duration + start + end

	switch pattern.Type {
	case 0, 2: // Linear, Logarithmic - no extra params
		return baseSize
	case 1, 3: // Exponential (curve), Vibrato (amp + period)
		return baseSize + 2 // param1 + param2
	case 4: // Complex - not effect-based
		return 0 // Will be handled by Complex encoding
	default:
		return baseSize
	}
}
