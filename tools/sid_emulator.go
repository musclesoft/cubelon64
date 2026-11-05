// SID 6581 Emulator
//
// Hardware-accurate emulation of the Commodore 64's SID (Sound Interface Device) chip
// for validating code that will run on real C64 hardware.
//
// This emulator implements:
//   - 3 independent voices with triangle, sawtooth, pulse, and noise waveforms
//   - Hardware ADSR envelopes with exponential rate behavior
//   - Cycle-accurate envelope timing using real SID rate tables
//   - Register-based control interface (write-only, like real hardware)
//   - ADSR delay bug (rate counter wraparound when rate_period decreases)
//   - Correct gate OFF behavior (always enters Release state)
//
// The SID runs at 985248 Hz (PAL). Each ADSR rate (0-15) defines the number of
// cycles between envelope level changes, from 9 cycles (rate 0, ~2ms) to 31251
// cycles (rate 15, ~8 seconds). The envelope uses an exponential rate divider
// that slows the rate at certain thresholds (255, 93, 54, 26, 14, 6) to create
// exponential curves.
//
// ADSR delay bug: If rate_period decreases (R value decreases), the 15-bit rate
// counter must wrap at 0x8000 before stepping, freezing the envelope for 0-33ms.
// Verified by ENV3 sampling on real hardware. Our conversion code works around this
// by limiting R decreases to max 2 steps per frame (gradual descent).
//
// Hardware behaviors NOT implemented:
//   - Pipeline delays (2-3 cycle delays for envelope/state transitions)
//     These cycle-level timing quirks are negligible at 44.1kHz audio sample rate.
//   - "Decay accidentally activated" bug (first 2-3 cycles of Attack use Decay rate)
//     Negligible effect: ~0.1% of attack phase duration at 44.1kHz.
//
// Reference: http://www.sidmusic.org/sid/sidtech2.html
// Reference: reSID (http://www.zimmers.net/anonftp/pub/cbm/crossplatform/emulators/resid/)
//
package main

import (
	"math"
	"math/rand"
)

// SIDEmulator is a simplified SID chip emulator for audio comparison
// Focuses on voice generation and ADSR envelopes
type SIDEmulator struct {
	// SID registers (write-only in real hardware)
	Registers [29]byte

	// Voice state
	Voices [3]SIDVoice

	// Global state
	SampleRate      float64
	ClocksPerSample float64
	ClockCounter    float64

	// Current frame (for logging)
	CurrentFrame uint32
	Logger       SIDWriteLogger
}

// SIDWriteLogger interface for logging register writes
type SIDWriteLogger interface {
	LogWrite(frame uint32, register byte, value byte)
}

// SIDVoice represents one of the three SID voices
type SIDVoice struct {
	// Oscillator
	Frequency      float64 // Current frequency in Hz
	Phase          float64 // Current phase (0.0 to 1.0)
	PhaseIncrement float64 // Phase increment per sample

	// ADSR envelope (cycle-accurate)
	EnvelopeLevel      uint8   // Current envelope level (0-255)
	EnvelopeState      int     // 0=Attack, 1=Decay/Sustain, 2=Release
	AttackRate         uint8   // 0-15
	DecayRate          uint8   // 0-15
	SustainLevel       uint8   // 0-15 (maps to 0-255)
	ReleaseRate        uint8   // 0-15
	RateCounter        uint16  // 15-bit rate counter (0-0x7fff), counts up to rate_period
	EnvelopeCounter    float64 // Fractional accumulator for sub-cycle timing
	Gate               bool    // Gate bit
	PrevGate           bool    // Previous gate state
	ADSRDelayBugActive bool    // True when rate counter is wrapping (ADSR delay bug)

	// Exponential counter (for exponential ADSR decay/release)
	ExponentialCounter       uint8 // Counter for exponential rate divider
	ExponentialCounterPeriod uint8 // Period (1, 2, 4, 8, 16, 30)
	HoldZero                 bool  // Envelope frozen at zero

	// Waveform
	TestBit         bool    // Control register bit 3: freezes oscillator phase at 0
	Waveform        byte    // 1=Triangle, 2=Sawtooth, 4=Pulse, 8=Noise
	PrevWaveform    byte    // Previous waveform (to detect transition to 0)
	PulseWidth      uint16  // 12-bit pulse width (0-4095), controls duty cycle
	WaveformOutput  float64 // Last waveform output (for floating DAC)
	FloatingOutputTTL float64 // Time-to-live for floating DAC output (in cycles)

	// Noise generator (23-bit LFSR)
	NoiseShift           uint32  // 23-bit shift register
	NoiseOutput          uint16  // Current noise output (8 bits extracted from shift register)
	NoiseFractionalShift float64 // Fractional LFSR shift accumulator

	// Noise optimization: pre-computed cache
	NoiseCache      [1024]float64 // Pre-generated noise samples
	NoiseCacheIndex int           // Current position in cache
	NoiseCacheValid bool          // Whether cache contains valid data
}

// NewSIDEmulator creates a new SID emulator
func NewSIDEmulator(sampleRate float64) *SIDEmulator {
	sid := &SIDEmulator{
		SampleRate:      sampleRate,
		ClocksPerSample: sidClockHz / sampleRate,
	}

	// Initialize voices
	for i := 0; i < 3; i++ {
		sid.Voices[i].NoiseShift = 0x7FFFF8              // Initial LFSR seed
		sid.Voices[i].EnvelopeState = 2                  // Release
		sid.Voices[i].EnvelopeLevel = 0xAA               // Odd bits high on power-up (hardware behavior)
		sid.Voices[i].ExponentialCounterPeriod = 1       // Start with no division
		sid.Voices[i].ExponentialCounter = 0
		sid.Voices[i].PulseWidth = 0x800                 // 50% duty cycle (2048/4096)
	}

	return sid
}

// Real SID ADSR rate tables (cycles per step at 1MHz)
// These are the actual rates from the 6581 chip, verified by resid
// Rate counter period is the number of cycles between envelope steps
var adsrRatePeriods = [16]uint32{
	8, 31, 62, 94, 148, 219, 266, 312,
	391, 976, 1953, 3125, 3906, 11719, 19531, 31250,
}

// Floating DAC output behavior (6581 chip model)
// When waveform is set to 0, the DAC holds its last value and gradually fades
const (
	floatingOutputTTLStart = 182000.0 // ~200ms at 1MHz (6581)
	floatingOutputTTLBit   = 1500.0   // Per-bit fade time (6581)
)

// WriteRegister writes a value to a SID register
func (sid *SIDEmulator) WriteRegister(register byte, value byte) {
	if register >= 29 {
		return
	}

	sid.Registers[register] = value

	// Log write if logger is set
	if sid.Logger != nil {
		sid.Logger.LogWrite(sid.CurrentFrame, register, value)
	}

	// Update voice parameters based on register
	voice := int(register / 7)
	if voice >= 3 {
		return
	}

	regOffset := register % 7

	switch regOffset {
	case 0, 1: // Frequency low/high
		sid.updateFrequency(voice)
	case 2, 3: // Pulse width low/high
		sid.updatePulseWidth(voice)
	case 4: // Control register
		sid.updateControl(voice)
	case 5, 6: // Attack/Decay, Sustain/Release
		sid.updateEnvelope(voice)
	}
}

func (sid *SIDEmulator) updateFrequency(voice int) {
	base := voice * 7
	freqLow := sid.Registers[base+0]
	freqHigh := sid.Registers[base+1]
	freq16 := uint16(freqLow) | (uint16(freqHigh) << 8)

	// Convert SID frequency register to Hz
	// Formula: freq_hz = freq_reg * clock / 2^24
	sid.Voices[voice].Frequency = float64(freq16) * sidClockHz / sidFreqDivisor
	sid.Voices[voice].PhaseIncrement = sid.Voices[voice].Frequency / sid.SampleRate

	// Invalidate noise cache when frequency changes
	sid.Voices[voice].NoiseCacheValid = false
}

func (sid *SIDEmulator) updatePulseWidth(voice int) {
	base := voice * 7
	pwLow := sid.Registers[base+2]
	pwHigh := sid.Registers[base+3]
	// Pulse width is 12-bit (only lower 4 bits of high byte)
	sid.Voices[voice].PulseWidth = uint16(pwLow) | (uint16(pwHigh&0x0F) << 8)
}

func (sid *SIDEmulator) updateControl(voice int) {
	base := voice * 7
	ctrl := sid.Registers[base+4]

	v := &sid.Voices[voice]
	v.Waveform = (ctrl >> 4) & 0x0F

	// Gate bit
	newGate := (ctrl & 0x01) != 0

	if newGate && !v.PrevGate {
		// Gate on: start attack
		v.EnvelopeState = 0 // Attack
		v.HoldZero = false  // Unfreeze envelope
		v.Gate = true
		// Note: rate counter is NOT reset (hardware behavior)
		// This causes a delay before first envelope step
	} else if !newGate && v.PrevGate {
		// Gate OFF always enters Release state (after pipeline delay in real hardware)
		// We don't model the 2-3 cycle pipeline delay, so transition immediately
		v.EnvelopeState = 2 // Release
		v.Gate = false
		// Note: rate counter is NOT reset (hardware behavior)
	}

	v.PrevGate = newGate

	// Test bit: freeze oscillator phase at 0 while held, reset LFSR on assertion
	newTest := (ctrl & 0x08) != 0
	if newTest && !v.TestBit {
		v.Phase = 0
		v.NoiseShift = 0x7FFFF8
	}
	v.TestBit = newTest
}

func (sid *SIDEmulator) updateEnvelope(voice int) {
	base := voice * 7
	ad := sid.Registers[base+5]
	sr := sid.Registers[base+6]

	v := &sid.Voices[voice]

	// Attack rate (4 bits, 0-15)
	v.AttackRate = (ad >> 4) & 0x0F
	// Decay rate (4 bits, 0-15)
	v.DecayRate = ad & 0x0F
	// Sustain level (4 bits, 0-15) - maps to 0-255 in steps of 17
	v.SustainLevel = (sr >> 4) & 0x0F
	// Release rate (4 bits, 0-15)
	v.ReleaseRate = sr & 0x0F

	// Note: In reSID, writing ADSR registers updates the rate_period if in the
	// matching state. Our implementation recalculates rate period each sample
	// in updateEnvelopeState, so the new values take effect immediately.
}

// GenerateSample generates one audio sample from a specific voice
func (sid *SIDEmulator) GenerateSample(voice int) int16 {
	return sid.generateVoice(voice)
}

// GenerateSamplesBatch generates multiple audio samples from a specific voice in one call
// This is much faster than calling GenerateSample in a loop
func (sid *SIDEmulator) GenerateSamplesBatch(voice int, buffer []int16) {
	v := &sid.Voices[voice]

	for i := 0; i < len(buffer); i++ {
		// Update envelope (optimized version checks counter)
		sid.updateEnvelopeState(v)

		// Detect waveform transition to 0 (floating DAC behavior)
		if v.Waveform == 0 && v.PrevWaveform != 0 {
			v.FloatingOutputTTL = floatingOutputTTLStart
		}
		v.PrevWaveform = v.Waveform

		// Generate waveform
		var sample float64

		if v.Waveform == 0 {
			// No waveform selected - use floating DAC output
			if v.FloatingOutputTTL > 0 {
				sample = v.WaveformOutput
				v.FloatingOutputTTL -= sid.ClocksPerSample
				if v.FloatingOutputTTL <= 0 {
					v.WaveformOutput *= 0.5
					if v.WaveformOutput > 0.001 {
						v.FloatingOutputTTL = floatingOutputTTLBit
					} else {
						v.WaveformOutput = 0
						v.FloatingOutputTTL = 0
					}
				}
			} else {
				sample = 0
			}
		} else {
			// Waveform active: generate and save output
			if v.Waveform == 0x05 {
				// Pulse+Triangle waveform (0x05)
				// ERROR DETECTION: If pulse width < 0xFFF, output triangle-modulated noise to make error audible
				if v.PulseWidth < 0xFFF {
					// INCORRECT pulse width - output "filled triangle" with white noise
					// Noise range is always [-1.0, triangle_value], creating a filled triangle shape
					triangle := sid.generateTriangle(v)
					random := rand.Float64()                    // Random value in [0.0, 1.0]
					sample = -1.0 + random*(triangle-(-1.0))    // Maps to range [-1.0, triangle]
				} else {
					// CORRECT pulse width (0xFFF) - output pulse-gated triangle
					threshold := float64(v.PulseWidth) / 4096.0
					if v.Phase >= threshold {
						// Pulse is HIGH: output triangle
						sample = sid.generateTriangle(v)
					} else {
						// Pulse is LOW: output -1.0
						sample = -1.0
					}
				}
			} else if v.Waveform == 0x07 {
				// Combined waveforms: 0x7 (pulse+sawtooth+triangle)
				// Output -1.0 when pulse is LOW (simulates gating behavior)
				threshold := float64(v.PulseWidth) / 4096.0
				if v.Phase >= threshold {
					// Pulse is HIGH: output combined waveform (use triangle as approximation)
					sample = sid.generateTriangle(v)
				} else {
					// Pulse is LOW: output -1.0
					sample = -1.0
				}
			} else if v.Waveform&0x08 != 0 {
				sample = sid.generateNoise(v)
			} else if v.Waveform&0x04 != 0 {
				sample = sid.generatePulse(v)
			} else if v.Waveform&0x02 != 0 {
				sample = sid.generateSawtooth(v)
			} else if v.Waveform&0x01 != 0 {
				sample = sid.generateTriangle(v)
			}
			v.WaveformOutput = sample
		}

		// Apply envelope
		envelopeVolume := float64(v.EnvelopeLevel) / 255.0
		sample *= envelopeVolume

		// Convert to int16
		buffer[i] = int16(sample * 32767.0)

		// Update phase; frozen at 0 while test bit is held
		if !v.TestBit {
			v.Phase += v.PhaseIncrement
			if v.Phase >= 1.0 {
				v.Phase = math.Mod(v.Phase, 1.0)
			}
		}
	}
}



func (sid *SIDEmulator) generateVoice(voice int) int16 {
	v := &sid.Voices[voice]

	// Update envelope
	sid.updateEnvelopeState(v)

	// Detect waveform transition to 0 (floating DAC behavior)
	if v.Waveform == 0 && v.PrevWaveform != 0 {
		// Waveform changed from non-zero to 0: start floating DAC timer
		v.FloatingOutputTTL = floatingOutputTTLStart
		// WaveformOutput already contains last generated sample
	}
	v.PrevWaveform = v.Waveform

	// Generate waveform
	var sample float64

	if v.Waveform == 0 {
		// No waveform selected - use floating DAC output
		if v.FloatingOutputTTL > 0 {
			// DAC is still holding/fading the last value
			sample = v.WaveformOutput

			// Count down floating output timer
			v.FloatingOutputTTL -= sid.ClocksPerSample
			if v.FloatingOutputTTL <= 0 {
				// Timer expired: apply bit fade (equivalent to waveform_output >> 1)
				v.WaveformOutput *= 0.5
				// Reset timer for next bit fade (if output still significant)
				if v.WaveformOutput > 0.001 { // Threshold to avoid float precision issues
					v.FloatingOutputTTL = floatingOutputTTLBit
				} else {
					v.WaveformOutput = 0
					v.FloatingOutputTTL = 0
				}
			}
		} else {
			// Fully faded to zero
			sample = 0
		}
	} else {
		// Waveform active: generate and save output
		if v.Waveform == 0x05 {
			// Pulse+Triangle waveform (0x05)
			// ERROR DETECTION: If pulse width < 0xFFF, output triangle-modulated noise to make error audible
			if v.PulseWidth < 0xFFF {
				// INCORRECT pulse width - output "filled triangle" with white noise
				// Noise range is always [-1.0, triangle_value], creating a filled triangle shape
				triangle := sid.generateTriangle(v)
				random := rand.Float64()                    // Random value in [0.0, 1.0]
				sample = -1.0 + random*(triangle-(-1.0))    // Maps to range [-1.0, triangle]
			} else {
				// CORRECT pulse width (0xFFF) - output pulse-gated triangle
				threshold := float64(v.PulseWidth) / 4096.0
				if v.Phase >= threshold {
					// Pulse is HIGH: output triangle
					sample = sid.generateTriangle(v)
				} else {
					// Pulse is LOW: output -1.0
					sample = -1.0
				}
			}
		} else if v.Waveform == 0x07 {
			// Combined waveforms: 0x7 (pulse+sawtooth+triangle)
			// Output -1.0 when pulse is LOW (simulates gating behavior)
			threshold := float64(v.PulseWidth) / 4096.0
			if v.Phase >= threshold {
				// Pulse is HIGH: output combined waveform (use triangle as approximation)
				sample = sid.generateTriangle(v)
			} else {
				// Pulse is LOW: output -1.0
				sample = -1.0
			}
		} else if v.Waveform&0x08 != 0 {
			// Noise
			sample = sid.generateNoise(v)
		} else if v.Waveform&0x04 != 0 {
			// Pulse (check before sawtooth)
			sample = sid.generatePulse(v)
		} else if v.Waveform&0x02 != 0 {
			// Sawtooth
			sample = sid.generateSawtooth(v)
		} else if v.Waveform&0x01 != 0 {
			// Triangle
			sample = sid.generateTriangle(v)
		}
		// Save waveform output for potential floating DAC use
		v.WaveformOutput = sample
	}

	// Debug disabled
	//if debugSampleCount < 5 && voice == 0 {
	//	fmt.Printf("DEBUG SID voice 0: waveform=0x%02X, raw_sample=%.3f, env_level=%d, env_state=%d, gate=%v, phase=%.3f\n",
	//		v.Waveform, sample, v.EnvelopeLevel, v.EnvelopeState, v.Gate, v.Phase)
	//	debugSampleCount++
	//}

	// Apply per-voice envelope volume (0-255 maps to 0.0-1.0)
	envelopeVolume := float64(v.EnvelopeLevel) / 255.0
	sample *= envelopeVolume

	// Convert to int16 (-32768 to 32767)
	output := int16(sample * 32767.0)

	// Update phase (use modulo to handle PhaseIncrement >= 1.0)
	v.Phase += v.PhaseIncrement
	if v.Phase >= 1.0 {
		v.Phase = math.Mod(v.Phase, 1.0)
	}

	return output
}

// updateExponentialCounterPeriod sets the exponential counter period based on envelope level
// The SID uses a separate exponential counter to create exponential decay curves
func (sid *SIDEmulator) updateExponentialCounterPeriod(v *SIDVoice) {
	switch v.EnvelopeLevel {
	case 0xFF:
		v.ExponentialCounterPeriod = 1
	case 0x5D: // 93
		v.ExponentialCounterPeriod = 2
	case 0x36: // 54
		v.ExponentialCounterPeriod = 4
	case 0x1A: // 26
		v.ExponentialCounterPeriod = 8
	case 0x0E: // 14
		v.ExponentialCounterPeriod = 16
	case 0x06: // 6
		v.ExponentialCounterPeriod = 30
	case 0x00:
		v.ExponentialCounterPeriod = 1
		// When envelope reaches zero, it freezes (verified by hardware sampling)
		v.HoldZero = true
	}
}

func (sid *SIDEmulator) updateEnvelopeState(v *SIDVoice) {
	// Accumulate fractional cycles (reSID's clock(delta_t) approach)
	v.EnvelopeCounter += sid.ClocksPerSample

	// Process integer cycles (reSID algorithm with ADSR delay bug)
	for v.EnvelopeCounter >= 1.0 {
		deltaT := uint32(v.EnvelopeCounter)
		v.EnvelopeCounter -= float64(deltaT)

		// Get current rate period based on state
		// IMPORTANT: Must recalculate on each iteration to reflect:
		// 1. State changes from natural transitions (A→D, D→S)
		// 2. Rate changes from register writes (updateEnvelope calls)
		var ratePeriod uint32
		switch v.EnvelopeState {
		case 0: // Attack
			ratePeriod = adsrRatePeriods[v.AttackRate]
		case 1: // Decay/Sustain
			ratePeriod = adsrRatePeriods[v.DecayRate]
		case 2: // Release
			ratePeriod = adsrRatePeriods[v.ReleaseRate]
		}

		// ADSR delay bug: If rate_period is reduced below current rate_counter,
		// the counter must wrap at 0x8000 before the envelope can step.
		// This is real hardware behavior verified by ENV3 sampling.
		// Calculate cycles until next envelope step (reSID: NB! requires two's complement).
		rateStep := int32(ratePeriod) - int32(v.RateCounter)
		if rateStep <= 0 {
			rateStep += 0x7fff
			v.ADSRDelayBugActive = true // Flag: envelope is frozen, waiting for wraparound
		} else {
			v.ADSRDelayBugActive = false
		}

		for deltaT > 0 {
			if deltaT < uint32(rateStep) {
				// Not enough cycles to reach rate_period
				// Handle 15-bit wraparound correctly (matches reSID behavior)
				// When counter increments to 0x8000, it wraps to 0x0001
				newCounter := uint32(v.RateCounter) + deltaT
				if newCounter >= 0x8000 {
					// Wrapped: subtract 0x7FFF to get final position after wrap
					// Example: 0x7000 + 0x2000 = 0x9000, wrapped = 0x9000 - 0x7FFF = 0x1001
					newCounter = (newCounter - 0x7FFF) & 0x7FFF
				}
				v.RateCounter = uint16(newCounter)
				break
			}

			// Reached rate_period - reset counter and step envelope
			deltaT -= uint32(rateStep)
			v.RateCounter = 0

			// The first envelope step in attack resets exponential counter (reSID comment)
			if v.EnvelopeState == 0 || v.ExponentialCounter+1 == v.ExponentialCounterPeriod {
				v.ExponentialCounter = 0

				// Check whether envelope is frozen at zero
				if v.HoldZero {
					// reSID envelope.h line 255-256
					rateStep = int32(ratePeriod)
					continue
				}

				switch v.EnvelopeState {
				case 0: // Attack (linear)
					v.EnvelopeLevel++
					v.EnvelopeLevel &= 0xff
					if v.EnvelopeLevel == 0xff {
						v.EnvelopeLevel = 255
						v.EnvelopeState = 1 // -> Decay/Sustain
						// reSID envelope.h line 268-269: rate_period = rate_counter_period[decay]
						// Update rate period immediately on natural state transition
						ratePeriod = adsrRatePeriods[v.DecayRate]
					}
					sid.updateExponentialCounterPeriod(v)

				case 1: // Decay/Sustain (exponential)
					sustainTarget := uint8(v.SustainLevel) * 17 // Maps 0-15 to 0-255
					if v.EnvelopeLevel != sustainTarget {
						v.EnvelopeLevel--
						sid.updateExponentialCounterPeriod(v)
					}

				case 2: // Release (exponential)
					// The envelope counter can flip from 0x00 to 0xff by changing state to
					// attack, then to release. NB! requires two's complement (reSID comment)
					v.EnvelopeLevel--
					v.EnvelopeLevel &= 0xff
					sid.updateExponentialCounterPeriod(v)
				}
			} else {
				v.ExponentialCounter++
			}

			// reSID envelope.h line 302: rate_step = rate_period
			// Recalculate after envelope step to pick up state changes
			rateStep = int32(ratePeriod)
		}
	}
}

func (sid *SIDEmulator) generateTriangle(v *SIDVoice) float64 {
	// Triangle wave: /\/\/\
	if v.Phase < 0.5 {
		return v.Phase * 4.0 - 1.0 // Rising: -1 to 1
	}
	return 3.0 - v.Phase*4.0 // Falling: 1 to -1
}

func (sid *SIDEmulator) generateSawtooth(v *SIDVoice) float64 {
	// Sawtooth wave: /|/|/|
	return v.Phase*2.0 - 1.0 // Range: -1 to 1
}

func (sid *SIDEmulator) generatePulse(v *SIDVoice) float64 {
	// Pulse wave with variable duty cycle
	// PulseWidth is 12-bit (0-4095), determines when pulse goes high
	// Hardware: pulse HIGH when (accumulator >> 12) >= pw
	// In phase terms: pulse HIGH when phase >= (pw/4096)
	threshold := float64(v.PulseWidth) / 4096.0

	if v.Phase >= threshold {
		return 1.0
	}
	return -1.0
}

func (sid *SIDEmulator) generateNoise(v *SIDVoice) float64 {
	// OPTIMIZATION: Use pre-computed noise cache
	if !v.NoiseCacheValid {
		// Generate noise cache
		sid.fillNoiseCache(v)
		v.NoiseCacheValid = true
		v.NoiseCacheIndex = 0
	}

	// Get cached noise sample
	sample := v.NoiseCache[v.NoiseCacheIndex]

	// Advance cache index
	v.NoiseCacheIndex++
	if v.NoiseCacheIndex >= len(v.NoiseCache) {
		v.NoiseCacheIndex = 0
		// Mark cache as invalid to regenerate next time through
		v.NoiseCacheValid = false
	}

	return sample
}

func (sid *SIDEmulator) fillNoiseCache(v *SIDVoice) {
	// Pre-generate noise samples for the cache
	// This does the same LFSR computation but in batch
	shiftsPerSecond := v.Frequency * 16.0
	shiftsPerSample := shiftsPerSecond / sid.SampleRate

	for i := 0; i < len(v.NoiseCache); i++ {
		// Accumulate fractional shifts
		v.NoiseFractionalShift += shiftsPerSample
		numShifts := int(v.NoiseFractionalShift)
		v.NoiseFractionalShift -= float64(numShifts)

		// Perform the shifts
		for j := 0; j < numShifts; j++ {
			// Calculate feedback: bit 17 XOR bit 22
			bit17 := (v.NoiseShift >> 17) & 1
			bit22 := (v.NoiseShift >> 22) & 1
			feedback := bit17 ^ bit22

			// Shift left and insert feedback at bit 0 (matches resid)
			v.NoiseShift = ((v.NoiseShift << 1) | feedback) & 0x7FFFFF

			// Extract noise output from specific bits of shift register (resid algorithm)
			v.NoiseOutput = uint16(
				((v.NoiseShift & 0x100000) >> 9) | // Bit 20 -> output bit 11
					((v.NoiseShift & 0x040000) >> 8) | // Bit 18 -> output bit 10
					((v.NoiseShift & 0x004000) >> 5) | // Bit 14 -> output bit 9
					((v.NoiseShift & 0x000800) >> 3) | // Bit 11 -> output bit 8
					((v.NoiseShift & 0x000200) >> 2) | // Bit 9 -> output bit 7
					((v.NoiseShift & 0x000020) << 1) | // Bit 5 -> output bit 6
					((v.NoiseShift & 0x000004) << 3) | // Bit 2 -> output bit 5
					((v.NoiseShift & 0x000001) << 4)) // Bit 0 -> output bit 4
		}

		// Convert 8-bit noise value (bits 4-11, range 0-255) to -1.0 to 1.0
		v.NoiseCache[i] = (float64(v.NoiseOutput>>4)/255.0)*2.0 - 1.0
	}
}
