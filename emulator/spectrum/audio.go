package spectrum

import (
	"encoding/binary"
	"os"
)

// AudioBuffer generates audio samples from AY chip state
type AudioBuffer struct {
	SampleRate   int
	Samples      []int16
	Position     int
	CyclesPerSample float64
	CycleCounter float64

	// AY state per channel
	ToneCounter  [3]uint16
	NoiseCounter uint16
	NoiseRNG     uint32
	EnvCounter   uint16
	EnvPhase     byte
	EnvVolume    byte
}

func NewAudioBuffer(sampleRate int, durationSeconds float64) *AudioBuffer {
	numSamples := int(float64(sampleRate) * durationSeconds)
	// ZX Spectrum 128K runs at 3.5469MHz
	cpuFreq := 3546900.0
	cyclesPerSample := cpuFreq / float64(sampleRate)

	return &AudioBuffer{
		SampleRate:      sampleRate,
		Samples:         make([]int16, numSamples*2), // Stereo
		CyclesPerSample: cyclesPerSample,
		NoiseRNG:        1,
	}
}

// Update generates audio samples up to the given CPU cycle
func (ab *AudioBuffer) Update(ay *AY38912, cycles uint64) {
	ab.CycleCounter += float64(cycles)

	for ab.CycleCounter >= ab.CyclesPerSample && ab.Position < len(ab.Samples)/2 {
		sample := ab.generateSample(ay)

		// Write stereo (same sample to both channels)
		ab.Samples[ab.Position*2] = sample
		ab.Samples[ab.Position*2+1] = sample
		ab.Position++

		ab.CycleCounter -= ab.CyclesPerSample
	}
}

func (ab *AudioBuffer) generateSample(ay *AY38912) int16 {
	var mixedSample int32

	// Volume table (logarithmic, similar to real AY chip)
	// Scaled to fit int16 range
	volumeTable := [16]int16{
		0, 256, 414, 620, 962, 1619, 2463, 4555,
		5172, 8938, 12341, 15221, 19422, 23635, 28201, 32767,
	}

	for ch := 0; ch < 3; ch++ {
		var amplitude int16

		// Get tone period
		tonePeriod := ay.GetTone(ch)
		if tonePeriod < 1 {
			tonePeriod = 1
		}

		// Get volume
		volume := ay.GetVolume(ch)

		// Generate tone
		toneOn := (ay.Registers[7] & (1 << ch)) == 0
		if toneOn && tonePeriod > 0 {
			// Update tone counter
			ab.ToneCounter[ch]++
			if ab.ToneCounter[ch] >= tonePeriod {
				ab.ToneCounter[ch] = 0
			}

			// Square wave
			if ab.ToneCounter[ch] < tonePeriod/2 {
				amplitude = volumeTable[volume]
			} else {
				amplitude = -volumeTable[volume]
			}
		}

		mixedSample += int32(amplitude) / 3
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
