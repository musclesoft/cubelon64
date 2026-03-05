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

	// AY chip runs at CPU_CLOCK / 8
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
	EnvelopeHoldMask      int
	EnvelopeAlternateMask int
	EnvelopeAttackMask    int
	EnvelopeContinueMask  int
	EnvelopeAlternatePhase int
	LastEnvelopeShape      byte  // Track R13 to detect changes

	// Noise generator (17-bit shift register)
	NoisePeriod    float64
	NoiseCounter   float64
	NoiseSeed      uint32
	NoisePhase     int

	// Beeper state tracking
	BeeperState byte
	LastCycle   uint64
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
		Samples:             make([]int16, numSamples*2), // Stereo
		CyclesPerSample:     cyclesPerSample,
		AYClocksPerSample:   ayClocksPerSample,
		EnvelopePeriod:      256.0,
		EnvelopeRampCounter: 16,
		EnvelopeOnFirstRamp: true,
		NoisePeriod:         16.0,
		NoiseSeed:           1,
		NoisePhase:          0,
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

// Update generates audio samples up to the given CPU cycle
func (ab *AudioBuffer) Update(ay *AY38912, beeperState byte, currentCycle uint64) {
	// Update beeper state
	ab.BeeperState = beeperState

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

	for ab.CycleCounter >= ab.CyclesPerSample && ab.Position < len(ab.Samples)/2 {
		sample := ab.generateSample(ay)

		// Write stereo (same sample to both channels)
		ab.Samples[ab.Position*2] = sample
		ab.Samples[ab.Position*2+1] = sample
		ab.Position++

		ab.CycleCounter -= ab.CyclesPerSample
	}
}

var DebugAudio = false
var debugSampleCount = 0
var debugMaxSamples = 20 // Show more samples

func (ab *AudioBuffer) generateSample(ay *AY38912) int16 {
	var mixedSample int32

	// Volume table (logarithmic, similar to real AY chip)
	// Scaled to fit int16 range
	volumeTable := [16]int16{
		0, 256, 414, 620, 962, 1619, 2463, 4555,
		5172, 8938, 12341, 15221, 19422, 23635, 28201, 32767,
	}

	// Add beeper output (1-bit, fixed volume)
	if ab.BeeperState != 0 {
		mixedSample += 8000  // Moderate volume for beeper
	} else {
		mixedSample -= 8000
	}

	// Update noise period from R6 (JSSpeccy updates this on every register write)
	noisePeriod := float64((ay.Registers[6] & 0x1F) << 4)
	if noisePeriod == 0 {
		noisePeriod = 16
	}
	ab.NoisePeriod = noisePeriod

	// Update noise generator (17-bit shift register)
	ab.NoiseCounter -= ab.AYClocksPerSample
	for ab.NoiseCounter < 0 {
		ab.NoiseCounter += ab.NoisePeriod

		// Toggle noise phase based on RNG
		if (ab.NoiseSeed+1)&2 != 0 {
			ab.NoisePhase ^= 0xFF
		}

		// 17-bit shift register: bit 0 is output, input is bit 0 XOR bit 3
		if ab.NoiseSeed&1 != 0 {
			ab.NoiseSeed ^= 0x24000
		}
		ab.NoiseSeed >>= 1
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

		// Calculate envelope value
		ab.EnvelopeValue = ab.EnvelopeRampCounter
		if ab.EnvelopeAlternateMask != 0 && ab.EnvelopeAlternatePhase != 0 {
			ab.EnvelopeValue ^= ab.EnvelopeAlternateMask
		}
		if !ab.EnvelopeOnFirstRamp {
			ab.EnvelopeValue |= ab.EnvelopeHoldMask
		}
		ab.EnvelopeValue ^= ab.EnvelopeAttackMask
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

		// Noise generator phase (updated by noise generator above)
		noisePhase := ab.NoisePhase

		// Apply mixer logic: finalVolume AND (tonePhase OR toneMask) AND (noisePhase OR noiseMask)
		volBeforeMixer := finalVolume
		finalVolume = finalVolume & (tonePhase | toneMask) & (noisePhase | noiseMask)

		if DebugAudio && debugSampleCount < debugMaxSamples && ch == 1 {
			fmt.Printf("  Ch%d: period=%.0f counter=%.2f AYClocks/sample=%.2f toggles=%d output=%v\n",
				ch, tonePeriod, ab.ToneCounter[ch], ab.AYClocksPerSample, toggleCount, ab.ToneOutput[ch])
			fmt.Printf("  Ch%d mixer: R7=0x%02x volReg=0x%02x volBefore=%d tonePhase=0x%02x toneMask=0x%02x noisePhase=0x%02x noiseMask=0x%02x => volAfter=%d\n",
				ch, ay.Registers[7], volumeReg, volBeforeMixer, tonePhase, toneMask, noisePhase, noiseMask, finalVolume)
		}

		// Generate amplitude using final volume
		var amplitude int16
		if finalVolume > 0 {
			amplitude = volumeTable[finalVolume]
		} else {
			amplitude = -volumeTable[0]  // Silence
		}

		if DebugAudio && debugSampleCount < debugMaxSamples {
			fmt.Printf("  Ch%d: period=%.0f counter=%.2f output=%v vol=%d amp=%d\n",
				ch, tonePeriod, ab.ToneCounter[ch], ab.ToneOutput[ch], finalVolume, amplitude)
		}

		mixedSample += int32(amplitude) / 4
	}

	if DebugAudio && debugSampleCount < debugMaxSamples {
		fmt.Printf("Sample %d: beeper=%d mixed=%d result=%d\n",
			debugSampleCount, ab.BeeperState, mixedSample, mixedSample)
		debugSampleCount++
	}

	// Clamp to 16-bit range
	if mixedSample > 32767 {
		mixedSample = 32767
	} else if mixedSample < -32768 {
		mixedSample = -32768
	}

	return int16(mixedSample)
}

// WriteWAV writes the audio buffer to a WAV file
func (ab *AudioBuffer) WriteWAV(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// Trim to actual recorded samples
	actualSamples := ab.Samples[:ab.Position*2]
	dataSize := uint32(len(actualSamples) * 2)

	// WAV header
	f.Write([]byte("RIFF"))
	binary.Write(f, binary.LittleEndian, uint32(36+dataSize))
	f.Write([]byte("WAVE"))

	// fmt chunk
	f.Write([]byte("fmt "))
	binary.Write(f, binary.LittleEndian, uint32(16)) // fmt chunk size
	binary.Write(f, binary.LittleEndian, uint16(1))  // PCM
	binary.Write(f, binary.LittleEndian, uint16(2))  // Stereo
	binary.Write(f, binary.LittleEndian, uint32(ab.SampleRate))
	binary.Write(f, binary.LittleEndian, uint32(ab.SampleRate*4)) // byte rate
	binary.Write(f, binary.LittleEndian, uint16(4))  // block align
	binary.Write(f, binary.LittleEndian, uint16(16)) // bits per sample

	// data chunk
	f.Write([]byte("data"))
	binary.Write(f, binary.LittleEndian, dataSize)
	binary.Write(f, binary.LittleEndian, actualSamples)

	return nil
}
