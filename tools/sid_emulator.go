// SID 6581 Emulator
//
// Cycle-accurate emulation of the Commodore 64's SID (Sound Interface Device) chip,
// focusing on the three voices and their ADSR envelope generators.
//
// This emulator implements:
//   - 3 independent voices with triangle, sawtooth, pulse, and noise waveforms
//   - Hardware ADSR envelopes with exponential rate behavior
//   - Cycle-accurate envelope timing using the SID rate tables
//   - Register-based control interface (write-only, like real hardware)
//
// The SID runs at 985248 Hz (PAL). Each ADSR rate (0-15) defines the number of
// cycles between envelope level changes, from 9 cycles (rate 0, ~2ms) to 31251
// cycles (rate 15, ~8 seconds). The envelope uses an exponential rate divider
// that slows the rate at certain thresholds (93 and 157) to create exponential curves.
//
// Reference: http://www.sidmusic.org/sid/sidtech2.html
//
package main

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
}

// SIDVoice represents one of the three SID voices
type SIDVoice struct {
	// Oscillator
	Frequency      float64 // Current frequency in Hz
	Phase          float64 // Current phase (0.0 to 1.0)
	PhaseIncrement float64 // Phase increment per sample

	// ADSR envelope (cycle-accurate)
	EnvelopeLevel    uint8   // Current envelope level (0-255)
	EnvelopeState    int     // 0=Attack, 1=Decay/Sustain, 2=Release
	AttackRate       uint8   // 0-15
	DecayRate        uint8   // 0-15
	SustainLevel     uint8   // 0-15 (maps to 0-255)
	ReleaseRate      uint8   // 0-15
	EnvelopeCounter  float64 // Counter for rate timing
	Gate             bool    // Gate bit
	PrevGate         bool    // Previous gate state

	// Exponential counter (for exponential ADSR decay/release)
	ExponentialCounter       uint8 // Counter for exponential rate divider
	ExponentialCounterPeriod uint8 // Period (1, 2, 4, 8, 16, 30)
	HoldZero                 bool  // Envelope frozen at zero

	// Waveform
	Waveform        byte    // 1=Triangle, 2=Sawtooth, 4=Pulse, 8=Noise
	PrevWaveform    byte    // Previous waveform (to detect transition to 0)
	PulseWidth      uint16  // 12-bit pulse width (0-4095), controls duty cycle
	WaveformOutput  float64 // Last waveform output (for floating DAC)
	FloatingOutputTTL float64 // Time-to-live for floating DAC output (in cycles)

	// Noise generator (23-bit LFSR)
	NoiseShift  uint32 // 23-bit shift register
	NoiseOutput uint16 // Current noise output (8 bits extracted from shift register)
}

// NewSIDEmulator creates a new SID emulator
func NewSIDEmulator(sampleRate float64) *SIDEmulator {
	// SID clock is ~1 MHz (PAL: 985248 Hz, NTSC: 1022727 Hz)
	// Using PAL frequency
	sidClock := 985248.0

	sid := &SIDEmulator{
		SampleRate:      sampleRate,
		ClocksPerSample: sidClock / sampleRate,
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
	// Formula: freq_hz = freq_reg * clock / 16777216
	sid.Voices[voice].Frequency = float64(freq16) * 985248.0 / 16777216.0
	sid.Voices[voice].PhaseIncrement = sid.Voices[voice].Frequency / sid.SampleRate
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
		// Gate off: ADSR bug!
		// If in attack phase, goes to decay/sustain (not release)
		// This is the famous ADSR bug in the real SID
		if v.EnvelopeState == 0 {
			v.EnvelopeState = 1 // Decay/Sustain (bug!)
		} else {
			v.EnvelopeState = 2 // Release
		}
		v.Gate = false
		// Note: rate counter is NOT reset (hardware behavior)
	}

	v.PrevGate = newGate

	// Test bit resets oscillator
	if (ctrl & 0x08) != 0 {
		v.Phase = 0
		// Test bit also resets noise LFSR
		v.NoiseShift = 0x7FFFF8
	}
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
}

// GenerateSample generates one audio sample from all voices
func (sid *SIDEmulator) GenerateSample() (int16, int16, int16) {
	var samples [3]int16

	for i := 0; i < 3; i++ {
		samples[i] = sid.generateVoice(i)
	}

	return samples[0], samples[1], samples[2]
}

var debugSampleCount = 0

func resetSIDDebug() {
	debugSampleCount = 0
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
		if v.Waveform&0x08 != 0 {
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

	// Update phase
	v.Phase += v.PhaseIncrement
	if v.Phase >= 1.0 {
		v.Phase -= 1.0
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
	// Convert SID clock cycles to sample clock
	// SID runs at ~985248 Hz, we're sampling at sid.SampleRate
	cyclesPerSample := sid.ClocksPerSample

	v.EnvelopeCounter += cyclesPerSample

	// Get current rate period based on state
	var ratePeriod uint32
	switch v.EnvelopeState {
	case 0: // Attack
		ratePeriod = adsrRatePeriods[v.AttackRate]
	case 1: // Decay/Sustain
		ratePeriod = adsrRatePeriods[v.DecayRate]
	case 2: // Release
		ratePeriod = adsrRatePeriods[v.ReleaseRate]
	}

	// Process envelope steps (may be multiple per sample at high sample rates)
	for v.EnvelopeCounter >= float64(ratePeriod) {
		v.EnvelopeCounter -= float64(ratePeriod)

		// Hardware behavior: Attack bypasses exponential counter (LINEAR)
		// Decay/Release use exponential counter (EXPONENTIAL)
		shouldStep := false

		if v.EnvelopeState == 0 {
			// Attack: always step (linear), and reset exponential counter
			v.ExponentialCounter = 0
			shouldStep = true
		} else {
			// Decay/Release: step only when exponential counter reaches period
			v.ExponentialCounter++
			if v.ExponentialCounter >= v.ExponentialCounterPeriod {
				v.ExponentialCounter = 0
				shouldStep = true
			}
		}

		// Step the envelope if conditions are met
		if shouldStep {
			// Hardware behavior: envelope can freeze at zero
			if v.HoldZero {
				continue
			}

			switch v.EnvelopeState {
			case 0: // Attack (linear)
				if v.EnvelopeLevel < 255 {
					v.EnvelopeLevel++
					sid.updateExponentialCounterPeriod(v)
				}
				if v.EnvelopeLevel >= 255 {
					v.EnvelopeLevel = 255
					v.EnvelopeState = 1 // -> Decay/Sustain
				}

			case 1: // Decay/Sustain (exponential)
				sustainTarget := uint8(v.SustainLevel) * 17 // Maps 0-15 to 0-255
				if v.EnvelopeLevel > sustainTarget {
					if v.EnvelopeLevel > 0 {
						v.EnvelopeLevel--
						sid.updateExponentialCounterPeriod(v)
					}
				}
				// Stay at sustain level while gate is on

			case 2: // Release (exponential)
				if v.EnvelopeLevel > 0 {
					v.EnvelopeLevel--
					sid.updateExponentialCounterPeriod(v)
				}
			}
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
	// Real SID noise uses a 23-bit LFSR with feedback from bits 17 and 22
	// The noise is clocked by the oscillator frequency

	// Update noise shift register when oscillator cycles
	if v.Phase < v.PhaseIncrement {
		// Calculate feedback: bit 17 XOR bit 22
		bit17 := (v.NoiseShift >> 17) & 1
		bit22 := (v.NoiseShift >> 22) & 1
		feedback := bit17 ^ bit22

		// Shift left and insert feedback at bit 0 (matches resid)
		v.NoiseShift = ((v.NoiseShift << 1) | feedback) & 0x7FFFFF

		// Extract noise output from specific bits of shift register (resid algorithm)
		// These specific bits are used by the SID hardware to create the noise waveform
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
	// Normalize from 0-255 to -1.0 to 1.0
	return (float64(v.NoiseOutput>>4)/255.0)*2.0 - 1.0
}
