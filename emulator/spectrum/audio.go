package spectrum

import (
	"encoding/binary"
	"fmt"
	"os"
)

// AudioBuffer generates audio samples from AY chip state
type AudioBuffer struct {
	SampleRate      int
	Samples         []int16
	Position        int
	CyclesPerSample float64
	CycleCounter    float64

	// AY chip runs at CPU_CLOCK / 2 (on ZX Spectrum 128K)
	AYClocksPerSample float64

	// AY state per channel
	ToneCounter [3]float64
	ToneOutput  [3]bool

	// Envelope generator
	EnvelopePeriod      float64
	EnvelopeCounter     float64
	EnvelopeRampCounter int
	EnvelopeOnFirstRamp bool
	EnvelopeValue       int
	// Envelope shape masks
	EnvelopeHoldMask       int
	EnvelopeAlternateMask  int
	EnvelopeAttackMask     int
	EnvelopeContinueMask   int
	EnvelopeAlternatePhase int
	LastEnvelopeShape      byte // Track R13 to detect changes

	// Noise generator (17-bit shift register)
	NoisePeriod  float64
	NoiseCounter float64
	NoiseSeed    uint32

	LastCycle uint64

	// AY write pulse tracking (for 5th channel)
	AYWritePulse bool
}

func NewAudioBuffer(sampleRate int, durationSeconds float64) *AudioBuffer {
	numSamples := int(float64(sampleRate) * durationSeconds)
	// ZX Spectrum 128K: 69888 T-states/frame * 50 frames/sec = 3.4944 MHz
	cpuFreq := 69888.0 * 50.0
	cyclesPerSample := cpuFreq / float64(sampleRate)

	// AY chip clock is CPU clock / 2 (matches JSSpeccy: ayCyclesPerSample = 0.5 * frameCycleCount / samplesPerFrame)
	ayClockFreq := cpuFreq / 2.0
	ayClocksPerSample := ayClockFreq / float64(sampleRate)

	return &AudioBuffer{
		SampleRate:          sampleRate,
		Samples:             make([]int16, numSamples*6), // 6 channels: A, B, C, Envelope, AY-Write-Pulse, A-waveform
		CyclesPerSample:     cyclesPerSample,
		AYClocksPerSample:   ayClocksPerSample,
		EnvelopePeriod:      256.0,
		EnvelopeRampCounter: 16,
		EnvelopeOnFirstRamp: true,
		NoisePeriod:         16.0,
		NoiseSeed:           1,
		AYWritePulse:        false,
	}
}

// TriggerEnvelope resets envelope generator (called when R13 is written)
func (ab *AudioBuffer) TriggerEnvelope(shape byte, period uint16) {
	ab.EnvelopeCounter = 0
	ab.EnvelopeRampCounter = 16
	ab.EnvelopeOnFirstRamp = true
	ab.EnvelopeAlternatePhase = 0

	// Set envelope period (16-bit value shifted left by 4)
	ab.EnvelopePeriod = float64(period << 4)
	if ab.EnvelopePeriod == 0 {
		ab.EnvelopePeriod = 16
	}

	// Set shape masks
	if shape&0x01 != 0 {
		ab.EnvelopeHoldMask = 0x0F
	} else {
		ab.EnvelopeHoldMask = 0x00
	}

	if shape&0x02 != 0 {
		ab.EnvelopeAlternateMask = 0x0F
	} else {
		ab.EnvelopeAlternateMask = 0x00
	}

	if shape&0x04 != 0 {
		ab.EnvelopeAttackMask = 0x0F
	} else {
		ab.EnvelopeAttackMask = 0x00
	}

	if shape&0x08 != 0 {
		ab.EnvelopeContinueMask = 0x0F
	} else {
		ab.EnvelopeContinueMask = 0x00
	}
}

// MarkAYWrite sets the AY write pulse flag (will be rendered in next sample)
func (ab *AudioBuffer) MarkAYWrite() {
	ab.AYWritePulse = true
}

// Update generates audio samples up to the given CPU cycle
func (ab *AudioBuffer) Update(ay *AY38912, currentCycle uint64) {
	// Check if envelope shape (R13) changed
	currentShape := ay.Registers[13]
	if currentShape != ab.LastEnvelopeShape {
		// Trigger envelope with new shape
		period := uint16(ay.Registers[11]) | (uint16(ay.Registers[12]) << 8)
		ab.TriggerEnvelope(currentShape, period)
		ab.LastEnvelopeShape = currentShape
	}

	// Calculate cycles elapsed since last update
	cyclesElapsed := currentCycle - ab.LastCycle
	ab.LastCycle = currentCycle

	ab.CycleCounter += float64(cyclesElapsed)

	for ab.CycleCounter >= ab.CyclesPerSample && ab.Position < len(ab.Samples)/6 {
		chanA, chanB, chanC, chanEnv, chanPulse, chanANoEnv := ab.generateSample(ay)

		// Write 6 channels: A, B, C, Envelope, AY-Write-Pulse, A-waveform
		ab.Samples[ab.Position*6+0] = chanA
		ab.Samples[ab.Position*6+1] = chanB
		ab.Samples[ab.Position*6+2] = chanC
		ab.Samples[ab.Position*6+3] = chanEnv
		ab.Samples[ab.Position*6+4] = chanPulse
		ab.Samples[ab.Position*6+5] = chanANoEnv
		ab.Position++

		ab.CycleCounter -= ab.CyclesPerSample
	}
}

var DebugAudio = false
var debugSampleCount = 0
var debugMaxSamples = 20 // Show more samples

func (ab *AudioBuffer) generateSample(ay *AY38912) (int16, int16, int16, int16, int16, int16) {
	var channelAmplitudes [3]int16

	// Volume table from JSSpeccy, scaled to use full int16 range
	// Scaled for individual channel analysis (not mixed output)
	// Original JSSpeccy values * 6.0x to maximize dynamic range
	volumeTable := [16]int16{
		0, 451, 671, 952, 1387, 2026, 2776, 4486,
		5542, 8672, 11557, 14743, 18690, 22520, 27792, 32767,
	}

	// Update noise period from R6 (JSSpeccy updates this on every register write)
	noisePeriod := float64((ay.Registers[6] & 0x1F) << 4)
	if noisePeriod == 0 {
		noisePeriod = 16
	}
	ab.NoisePeriod = noisePeriod

	// Update noise generator (17-bit LFSR with prescaler)
	// Hardware has a prescaler that divides the noise period by 2
	ab.NoiseCounter -= ab.AYClocksPerSample
	for ab.NoiseCounter < 0 {
		ab.NoiseCounter += ab.NoisePeriod * 2.0 // Prescaler: divide by 2

		// 17-bit Fibonacci LFSR: feedback from bit 0 XOR bit 3
		// This was verified on real AY-3-8910 and YM2149 chips
		bit0 := ab.NoiseSeed & 1
		bit3 := (ab.NoiseSeed >> 3) & 1
		feedback := bit0 ^ bit3
		ab.NoiseSeed = (ab.NoiseSeed >> 1) | (feedback << 16)
	}

	// Update envelope period from R11/R12 (JSSpeccy updates this on every register write)
	envPeriod := float64((uint16(ay.Registers[11]) | (uint16(ay.Registers[12]) << 8)) << 4)
	if envPeriod == 0 {
		envPeriod = 16
	}
	ab.EnvelopePeriod = envPeriod

	// Update envelope generator
	ab.EnvelopeCounter -= ab.AYClocksPerSample
	for ab.EnvelopeCounter < 0 {
		ab.EnvelopeCounter += ab.EnvelopePeriod

		ab.EnvelopeRampCounter--
		if ab.EnvelopeRampCounter < 0 { // Wraps to -1, simulating 0xff wrap
			ab.EnvelopeRampCounter = 15
			ab.EnvelopeOnFirstRamp = false
			ab.EnvelopeAlternatePhase ^= 0x0F
		}

		// Calculate envelope value (matches JSSpeccy exactly)
		// envelopeValue = envelopeRampCounter ^ (envelopeAlternatePhase && envelopeAlternateMask)
		alternateXor := 0
		if ab.EnvelopeAlternatePhase != 0 {
			alternateXor = ab.EnvelopeAlternateMask
		}
		ab.EnvelopeValue = ab.EnvelopeRampCounter ^ alternateXor

		// OR with the 'hold' bit if past the first ramp
		if !ab.EnvelopeOnFirstRamp {
			ab.EnvelopeValue |= ab.EnvelopeHoldMask
		}
		// XOR with the 'attack' bit
		ab.EnvelopeValue ^= ab.EnvelopeAttackMask
		// AND with the 'continue' bit if past the first ramp
		if !ab.EnvelopeOnFirstRamp {
			ab.EnvelopeValue &= ab.EnvelopeContinueMask
		}
	}

	// Add AY channels
	for ch := 0; ch < 3; ch++ {
		// Get tone period (in AY clock units, 16-bit)
		// JSSpeccy multiplies by 8: toneGeneratorAPeriod = f64(...register_value... * 8)
		tonePeriod := float64(ay.GetTone(ch)) * 8.0
		if tonePeriod < 8.0 {
			tonePeriod = 8.0
		}

		// Get volume register (full byte to check bit 4)
		volumeReg := ay.Registers[8+ch]

		// Determine final volume: envelope mode (bit 4) or direct volume (bits 0-3)
		var finalVolume int
		if volumeReg&0x10 != 0 {
			// Envelope mode
			finalVolume = ab.EnvelopeValue
		} else {
			// Direct volume
			finalVolume = int(volumeReg & 0x0F)
		}

		// Update tone counter by number of AY clocks per sample
		ab.ToneCounter[ch] += ab.AYClocksPerSample

		// Toggle output when counter exceeds period
		toggleCount := 0
		for ab.ToneCounter[ch] >= tonePeriod {
			ab.ToneCounter[ch] -= tonePeriod
			ab.ToneOutput[ch] = !ab.ToneOutput[ch]
			toggleCount++
		}

		if DebugAudio && debugSampleCount < debugMaxSamples && ch == 1 && toggleCount > 0 {
			fmt.Printf("  Ch%d toggled %d times, output now=%v\n", ch, toggleCount, ab.ToneOutput[ch])
		}

		// Calculate tone and noise masks (JSSpeccy style)
		// When bit is SET in R7, channel is DISABLED, mask=0xff (passes through)
		// When bit is CLEAR in R7, channel is ENABLED, mask=0x00 (uses generator)
		toneMask := 0
		if (ay.Registers[7] & (1 << ch)) != 0 {
			toneMask = 0xFF // Disabled - pass through
		}

		noiseMask := 0
		if (ay.Registers[7] & (0x08 << ch)) != 0 {
			noiseMask = 0xFF // Disabled - pass through
		}

		// Tone and noise generator phases (0x00 or 0xFF) - matches JSSpeccy
		tonePhase := 0
		if ab.ToneOutput[ch] {
			tonePhase = 0xFF
		}

		// Noise output: use LFSR bit 0 directly
		noisePhase := 0
		if ab.NoiseSeed&1 != 0 {
			noisePhase = 0xFF
		}

		// Apply mixer logic: (tonePhase OR toneMask) AND (noisePhase OR noiseMask)
		// Result is 0x00 (off) or 0xFF (on)
		mixerOutput := (tonePhase | toneMask) & (noisePhase | noiseMask)

		if DebugAudio && debugSampleCount < debugMaxSamples && ch == 1 {
			fmt.Printf("  Ch%d: period=%.0f counter=%.2f AYClocks/sample=%.2f toggles=%d output=%v\n",
				ch, tonePeriod, ab.ToneCounter[ch], ab.AYClocksPerSample, toggleCount, ab.ToneOutput[ch])
			fmt.Printf("  Ch%d mixer: R7=0x%02x volReg=0x%02x vol=%d tonePhase=0x%02x toneMask=0x%02x noisePhase=0x%02x noiseMask=0x%02x => mixer=0x%02x\n",
				ch, ay.Registers[7], volumeReg, finalVolume, tonePhase, toneMask, noisePhase, noiseMask, mixerOutput)
		}

		// Generate bipolar waveform: -volume to +volume (centered around 0)
		// When mixer is off (0x00): output negative
		// When mixer is on (0xFF): output positive
		amplitude := volumeTable[finalVolume]
		if mixerOutput == 0 {
			channelAmplitudes[ch] = -amplitude
		} else {
			channelAmplitudes[ch] = amplitude
		}

		if DebugAudio && debugSampleCount < debugMaxSamples {
			fmt.Printf("  Ch%d: period=%.0f counter=%.2f output=%v vol=%d mixer=0x%02x amp=%d\n",
				ch, tonePeriod, ab.ToneCounter[ch], ab.ToneOutput[ch], finalVolume, mixerOutput, channelAmplitudes[ch])
		}
	}

	if DebugAudio && debugSampleCount < debugMaxSamples {
		fmt.Printf("Sample %d: A=%d B=%d C=%d\n",
			debugSampleCount, channelAmplitudes[0], channelAmplitudes[1], channelAmplitudes[2])
		debugSampleCount++
	}

	// Channel 4: Envelope value (raw envelope generator output)
	// Output as unipolar since it's a control signal, not a waveform
	envelopeAmplitude := volumeTable[ab.EnvelopeValue]

	// Channel 5: AY write pulse (1-sample pulse on AY register write)
	// Bipolar for consistency with audio channels
	var pulseAmplitude int16
	const maxPulseVolume int16 = 32767 // volumeTable[15]
	if ab.AYWritePulse {
		pulseAmplitude = maxPulseVolume
		ab.AYWritePulse = false // Clear pulse after 1 sample
	} else {
		pulseAmplitude = -maxPulseVolume
	}

	// Channel 6: Voice A raw waveform (before envelope applied)
	// Shows the tone/noise state at constant volume, ignoring envelope modulation

	// Reuse channel A's tone output (already computed in main loop)
	tonePhaseA := 0
	if ab.ToneOutput[0] {
		tonePhaseA = 0xFF
	}

	// Calculate tone and noise masks for channel A (from R7)
	toneMaskA := 0
	if (ay.Registers[7] & (1 << 0)) != 0 {
		toneMaskA = 0xFF // Tone disabled
	}

	noiseMaskA := 0
	if (ay.Registers[7] & (0x08 << 0)) != 0 {
		noiseMaskA = 0xFF // Noise disabled
	}

	// Get the raw waveform state (tone AND noise mixer output)
	// Use LFSR bit 0 directly for noise output
	noisePhaseA := 0
	if ab.NoiseSeed&1 != 0 {
		noisePhaseA = 0xFF
	}
	waveformStateA := (tonePhaseA | toneMaskA) & (noisePhaseA | noiseMaskA)

	// Output raw waveform at constant volume (shows shape without envelope modulation)
	// Bipolar output: negative when off, positive when on
	const waveformVolume int16 = 18690 // Mid-high volume (volumeTable[12])
	var channelAWaveformAmplitude int16
	if waveformStateA != 0 {
		channelAWaveformAmplitude = waveformVolume
	} else {
		channelAWaveformAmplitude = -waveformVolume
	}

	// Return separate channels: A, B, C, Envelope, AY-Write-Pulse, A-waveform
	return channelAmplitudes[0], channelAmplitudes[1], channelAmplitudes[2], envelopeAmplitude, pulseAmplitude, channelAWaveformAmplitude
}

// WriteWAV writes the audio buffer to a WAV file (6 channels: A, B, C, Envelope, AY-Write-Pulse, A-waveform)
func (ab *AudioBuffer) WriteWAV(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// Trim to actual recorded samples (6 channels per sample)
	actualSamples := ab.Samples[:ab.Position*6]
	dataSize := uint32(len(actualSamples) * 2)

	// WAV header
	f.Write([]byte("RIFF"))
	binary.Write(f, binary.LittleEndian, uint32(36+dataSize))
	f.Write([]byte("WAVE"))

	// fmt chunk
	f.Write([]byte("fmt "))
	binary.Write(f, binary.LittleEndian, uint32(16)) // fmt chunk size
	binary.Write(f, binary.LittleEndian, uint16(1))  // PCM
	binary.Write(f, binary.LittleEndian, uint16(6))  // 6 channels: A, B, C, Envelope, AY-Write-Pulse, A-no-envelope
	binary.Write(f, binary.LittleEndian, uint32(ab.SampleRate))
	binary.Write(f, binary.LittleEndian, uint32(ab.SampleRate*12)) // byte rate (6 channels * 2 bytes)
	binary.Write(f, binary.LittleEndian, uint16(12))               // block align (6 channels * 2 bytes)
	binary.Write(f, binary.LittleEndian, uint16(16))               // bits per sample

	// data chunk
	f.Write([]byte("data"))
	binary.Write(f, binary.LittleEndian, dataSize)
	binary.Write(f, binary.LittleEndian, actualSamples)

	return nil
}
