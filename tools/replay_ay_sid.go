// AY Command Replay Tool
//
// Plays back logged AY-3-8912 register commands through both AY and SID emulators
// for comparison and debugging. Useful for testing and analyzing specific sections
// of audio.
//
// This is a simpler tool than optimize_volume.go - it just replays commands
// without any ADSR optimization.
//
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"os"

	"cubelon-emu/spectrum"
)

func main() {
	// Command-line flags
	inputFile := flag.String("input", "", "Input AY command log file (.bin)")
	outputWAV := flag.String("output", "comparison.wav", "Output WAV file")
	sampleRate := flag.Int("sample-rate", 44100, "Sample rate (Hz)")
	duration := flag.Float64("duration", 0, "Duration in seconds (0 = auto from commands)")
	showStats := flag.Bool("stats", false, "Show statistics about AY commands")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Replays AY register commands through AY and SID emulators for comparison.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Replay AY commands and generate comparison WAV\n")
		fmt.Fprintf(os.Stderr, "  %s --input ay_commands.bin --output comparison.wav\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Show statistics about AY usage\n")
		fmt.Fprintf(os.Stderr, "  %s --input ay_commands.bin --stats\n\n", os.Args[0])
	}

	flag.Parse()

	if *inputFile == "" {
		fmt.Fprintf(os.Stderr, "Error: --input required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// Load AY commands
	fmt.Printf("Loading AY commands from %s...\n", *inputFile)
	commands, err := spectrum.LoadAYCommands(*inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading commands: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded %d AY commands\n", len(commands))

	if len(commands) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no commands in file\n")
		os.Exit(1)
	}

	// Show statistics if requested
	if *showStats {
		showCommandStats(commands)
		if *duration == 0 {
			return
		}
	}

	// Calculate duration if not specified
	durationSeconds := *duration
	if durationSeconds == 0 {
		// Use last command frame to estimate duration
		// ZX Spectrum: 50 frames/sec
		lastFrame := commands[len(commands)-1].Frame
		durationSeconds = float64(lastFrame) / 50.0
		fmt.Printf("Auto-detected duration: %.2f seconds (from frame %d)\n", durationSeconds, lastFrame)
	}

	// Create emulators
	fmt.Printf("Creating emulators (sample rate: %d Hz)...\n", *sampleRate)
	ay := createAYEmulator(*sampleRate, durationSeconds)
	sid := NewSIDEmulator(float64(*sampleRate))

	// Replay commands and generate audio
	fmt.Printf("Replaying commands and generating audio...\n")
	samples := replayCommands(commands, ay, sid, *sampleRate, durationSeconds)

	// Write WAV file
	fmt.Printf("Writing comparison WAV to %s...\n", *outputWAV)
	if err := writeComparisonWAV(*outputWAV, samples, *sampleRate); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing WAV: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Done! Generated %d samples (%.2fs)\n", len(samples)/6, float64(len(samples)/6)/float64(*sampleRate))
	fmt.Printf("\nWAV channels:\n")
	fmt.Printf("  1: AY Channel A\n")
	fmt.Printf("  2: AY Channel B\n")
	fmt.Printf("  3: AY Channel C\n")
	fmt.Printf("  4: SID Voice 1\n")
	fmt.Printf("  5: SID Voice 2\n")
	fmt.Printf("  6: SID Voice 3\n")
}

// createAYEmulator creates an AY emulator with audio buffer
func createAYEmulator(sampleRate int, durationSeconds float64) *spectrum.AudioBuffer {
	return spectrum.NewAudioBuffer(sampleRate, durationSeconds)
}

// replayCommands replays AY commands through both emulators and generates samples
func replayCommands(commands []spectrum.AYCommand, ay *spectrum.AudioBuffer, sid *SIDEmulator, sampleRate int, durationSeconds float64) []int16 {
	numSamples := int(float64(sampleRate) * durationSeconds)
	samples := make([]int16, numSamples*6) // 6 channels: AY A/B/C, SID 1/2/3

	// Create AY chip state (needed by audio buffer)
	ayChip := spectrum.NewAY38912()

	// Process all AY commands through the AudioBuffer
	// Convert frame numbers to cycles (69888 cycles per frame)
	cyclesPerFrame := uint64(69888)
	fmt.Printf("Processing %d AY commands...\n", len(commands))
	for i, cmd := range commands {
		// Apply to AY chip
		ayChip.SelectRegister(cmd.Register)
		ayChip.WriteRegister(cmd.Value)

		// Convert frame to cycle
		cycle := uint64(cmd.Frame) * cyclesPerFrame

		// Update audio buffer to this cycle (generates samples)
		ay.Update(ayChip, cycle)

		if i%10000 == 0 && i > 0 {
			fmt.Printf("  Processed %d/%d commands (AY generated %d samples)\n",
				i, len(commands), ay.Position)
		}
	}

	// Final update to generate any remaining samples
	finalFrame := commands[len(commands)-1].Frame + 10 // Extra frames
	finalCycle := uint64(finalFrame) * cyclesPerFrame
	ay.Update(ayChip, finalCycle)

	fmt.Printf("AY emulator generated %d samples\n", ay.Position)

	// Generate SID samples at same rate by replaying commands
	fmt.Printf("Generating SID samples...\n")
	samplesPerFrame := float64(sampleRate) / 50.0 // 50 fps
	commandIndex := 0

	// Track gate state per channel for proper retriggering
	gateState := [3]bool{false, false, false}

	for sampleIndex := 0; sampleIndex < numSamples; sampleIndex++ {
		// Calculate which frame this sample belongs to
		currentFrame := uint32(float64(sampleIndex) / samplesPerFrame)

		// Process all commands that should happen before this frame
		for commandIndex < len(commands) && commands[commandIndex].Frame <= currentFrame {
			translateAYToSID(commands[commandIndex], sid, &gateState)
			commandIndex++
		}

		// Generate SID sample
		sid1, sid2, sid3 := sid.GenerateSample()

		// Get AY samples (buffer already generated)
		ayIndex := sampleIndex
		if ayIndex >= ay.Position {
			ayIndex = ay.Position - 1
		}

		ay0 := ay.Samples[ayIndex*6+0]
		ay1 := ay.Samples[ayIndex*6+1]
		ay2 := ay.Samples[ayIndex*6+2]

		// Store interleaved samples
		samples[sampleIndex*6+0] = ay0
		samples[sampleIndex*6+1] = ay1
		samples[sampleIndex*6+2] = ay2
		samples[sampleIndex*6+3] = sid1
		samples[sampleIndex*6+4] = sid2
		samples[sampleIndex*6+5] = sid3
	}

	return samples
}

// translateAYToSID translates AY register writes to SID register writes
func translateAYToSID(cmd spectrum.AYCommand, sid *SIDEmulator, gateState *[3]bool) {
	reg := cmd.Register
	val := cmd.Value

	switch reg {
	case 0, 1: // AY Tone A (channel 0 -> SID voice 0)
		translateTone(cmd, sid, 0, 0)
	case 2, 3: // AY Tone B (channel 1 -> SID voice 1)
		translateTone(cmd, sid, 1, 7)
	case 4, 5: // AY Tone C (channel 2 -> SID voice 2)
		translateTone(cmd, sid, 2, 14)

	case 6: // AY Noise period
		// Map noise to all SID voices (can be refined)
		// For now, we'll handle noise in the mixer

	case 7: // AY Mixer control
		// Bits 0-2: Tone enable (0=on, 1=off) for channels A,B,C
		// Bits 3-5: Noise enable (0=on, 1=off) for channels A,B,C
		// Translate to SID waveform selection
		for ch := 0; ch < 3; ch++ {
			toneOn := (val & (1 << ch)) == 0
			noiseOn := (val & (0x08 << ch)) == 0

			sidBase := byte(ch * 7)
			var waveform byte
			if noiseOn {
				waveform = 0x80 // Noise
			} else if toneOn {
				waveform = 0x10 // Triangle (default)
			} else {
				waveform = 0x00 // Silent
			}

			// Read current control register to preserve gate bit
			ctrl := sid.Registers[sidBase+4]
			gate := ctrl & 0x01
			sid.WriteRegister(sidBase+4, waveform|gate)
		}

	case 8, 9, 10: // AY Volume A/B/C
		ch := reg - 8
		sidBase := byte(ch * 7)

		// Check if envelope mode (bit 4)
		if val&0x10 != 0 {
			// Envelope mode - use fast attack with sustain
			sid.WriteRegister(sidBase+5, 0x09) // A=0 (2ms), D=9 (800ms)
			sid.WriteRegister(sidBase+6, 0xF5) // S=15 (full), R=5 (114ms)
		} else {
			// Direct volume mode
			volume := val & 0x0F

			if volume > 0 {
				// Gate OFF first if already on (retrigger for adjacent notes)
				// The SID needs a 0→1 transition on the gate bit to restart attack
				if gateState[ch] {
					ctrl := sid.Registers[sidBase+4]
					sid.WriteRegister(sidBase+4, ctrl&0xFE)
				}

				// Note on: use fast attack, medium release for natural sound
				// A=0 (2ms), D=0 (6ms), S=15 (full), R=5 (114ms)
				sid.WriteRegister(sidBase+5, 0x00) // A=0, D=0
				sid.WriteRegister(sidBase+6, 0xF5) // S=15, R=5

				// Gate on
				ctrl := sid.Registers[sidBase+4]
				sid.WriteRegister(sidBase+4, ctrl|0x01)
				gateState[ch] = true
			} else {
				// Note off: gate off for release
				ctrl := sid.Registers[sidBase+4]
				sid.WriteRegister(sidBase+4, ctrl&0xFE)
				gateState[ch] = false
			}
		}

	case 11, 12: // AY Envelope period
		// Could map to SID ADSR rates

	case 13: // AY Envelope shape
		// Could map to SID ADSR curves

	// Registers 14, 15 are I/O ports - ignore for audio
	}
}

// translateTone translates AY tone period to SID frequency
func translateTone(cmd spectrum.AYCommand, sid *SIDEmulator, ayChannel int, sidBase byte) {
	// Get the full 12-bit period from AY
	// This requires both low and high bytes
	// For now, just write the register and let SID update when both bytes are written

	regOffset := cmd.Register % 2 // 0 for low, 1 for high
	sidReg := sidBase + byte(regOffset)

	// AY tone period is 12-bit, SID frequency is 16-bit
	// AY frequency = Clock / (16 * Period)
	// Clock = 1.7734 MHz (ZX Spectrum)
	// Need to convert to SID frequency register
	// SID frequency = freq_hz * 16777216 / 985248

	// For direct register mapping, we need both bytes
	// Just write the register for now
	sid.WriteRegister(sidReg, cmd.Value)

	// If this is the high byte, calculate and set the full frequency
	if regOffset == 1 {
		periodLow := sid.Registers[sidBase+0]
		periodHigh := cmd.Value & 0x0F

		ayPeriod := uint16(periodLow) | (uint16(periodHigh) << 8)
		if ayPeriod == 0 {
			ayPeriod = 1
		}

		// Convert AY period to frequency
		ayClock := 1773400.0 // ZX Spectrum AY clock (CPU clock / 2)
		freqHz := ayClock / (16.0 * float64(ayPeriod))

		// Convert to SID frequency register
		sidFreq := uint16(freqHz * 16777216.0 / 985248.0)

		// Write both bytes
		sid.WriteRegister(sidBase+0, byte(sidFreq&0xFF))
		sid.WriteRegister(sidBase+1, byte(sidFreq>>8))
	}
}

// writeComparisonWAV writes a 6-channel WAV file
func writeComparisonWAV(filename string, samples []int16, sampleRate int) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	numChannels := 6
	dataSize := uint32(len(samples) * 2)

	// WAV header
	f.Write([]byte("RIFF"))
	binary.Write(f, binary.LittleEndian, uint32(36+dataSize))
	f.Write([]byte("WAVE"))

	// fmt chunk
	f.Write([]byte("fmt "))
	binary.Write(f, binary.LittleEndian, uint32(16))                  // chunk size
	binary.Write(f, binary.LittleEndian, uint16(1))                   // PCM
	binary.Write(f, binary.LittleEndian, uint16(numChannels))         // 6 channels
	binary.Write(f, binary.LittleEndian, uint32(sampleRate))          // sample rate
	binary.Write(f, binary.LittleEndian, uint32(sampleRate*12))       // byte rate (6 ch * 2 bytes)
	binary.Write(f, binary.LittleEndian, uint16(numChannels*2))       // block align
	binary.Write(f, binary.LittleEndian, uint16(16))                  // bits per sample

	// data chunk
	f.Write([]byte("data"))
	binary.Write(f, binary.LittleEndian, dataSize)
	binary.Write(f, binary.LittleEndian, samples)

	return nil
}

// showCommandStats displays statistics about AY command usage
func showCommandStats(commands []spectrum.AYCommand) {
	regCounts := make(map[byte]int)
	regNames := []string{
		"R0_TONE_A_LOW", "R1_TONE_A_HIGH",
		"R2_TONE_B_LOW", "R3_TONE_B_HIGH",
		"R4_TONE_C_LOW", "R5_TONE_C_HIGH",
		"R6_NOISE_PERIOD",
		"R7_MIXER",
		"R8_VOL_A", "R9_VOL_B", "R10_VOL_C",
		"R11_ENV_LOW", "R12_ENV_HIGH",
		"R13_ENV_SHAPE",
		"R14_IO_A", "R15_IO_B",
	}

	for _, cmd := range commands {
		regCounts[cmd.Register]++
	}

	fmt.Printf("\n=== AY Register Usage ===\n")
	for reg := byte(0); reg < 16; reg++ {
		count := regCounts[reg]
		if count > 0 {
			fmt.Printf("  R%-2d %-16s: %6d writes\n", reg, regNames[reg], count)
		}
	}

	// Analyze command density over time
	if len(commands) > 0 {
		firstFrame := commands[0].Frame
		lastFrame := commands[len(commands)-1].Frame
		durationSeconds := float64(lastFrame-firstFrame) / 50.0

		fmt.Printf("\n=== Timing ===\n")
		fmt.Printf("  First frame: %d\n", firstFrame)
		fmt.Printf("  Last frame:  %d\n", lastFrame)
		fmt.Printf("  Duration:    %.2f seconds\n", durationSeconds)
		if durationSeconds > 0 {
			fmt.Printf("  Commands/sec: %.1f\n", float64(len(commands))/durationSeconds)
		}
	}

	// Analyze tone frequencies used
	fmt.Printf("\n=== Frequency Analysis ===\n")
	analyzeFrequencies(commands)
}

// analyzeFrequencies analyzes tone frequencies used in the demo
func analyzeFrequencies(commands []spectrum.AYCommand) {
	// Track tone periods for each channel
	tonePeriods := make(map[int]map[uint16]int) // channel -> period -> count
	for i := 0; i < 3; i++ {
		tonePeriods[i] = make(map[uint16]int)
	}

	currentPeriod := [3]uint16{0, 0, 0}

	for _, cmd := range commands {
		if cmd.Register <= 5 {
			// Tone register
			ch := int(cmd.Register / 2)
			isHigh := (cmd.Register % 2) == 1

			if isHigh {
				currentPeriod[ch] = (currentPeriod[ch] & 0x00FF) | (uint16(cmd.Value&0x0F) << 8)
				if currentPeriod[ch] > 0 {
					tonePeriods[ch][currentPeriod[ch]]++
				}
			} else {
				currentPeriod[ch] = (currentPeriod[ch] & 0xFF00) | uint16(cmd.Value)
			}
		}
	}

	ayClock := 1773400.0
	for ch := 0; ch < 3; ch++ {
		if len(tonePeriods[ch]) == 0 {
			continue
		}

		fmt.Printf("\nChannel %c:\n", 'A'+ch)

		// Find most common periods
		type periodCount struct {
			period uint16
			count  int
		}
		var periods []periodCount
		for period, count := range tonePeriods[ch] {
			periods = append(periods, periodCount{period, count})
		}

		// Sort by count
		for i := 0; i < len(periods); i++ {
			for j := i + 1; j < len(periods); j++ {
				if periods[j].count > periods[i].count {
					periods[i], periods[j] = periods[j], periods[i]
				}
			}
		}

		// Show top 5
		numShow := 5
		if len(periods) < numShow {
			numShow = len(periods)
		}

		fmt.Printf("  Top %d periods:\n", numShow)
		for i := 0; i < numShow; i++ {
			p := periods[i]
			freq := ayClock / (16.0 * float64(p.period))
			note := frequencyToNote(freq)
			fmt.Printf("    Period %4d: %7.2f Hz (%s) - %d times\n",
				p.period, freq, note, p.count)
		}
	}
}

// frequencyToNote converts frequency to nearest musical note
func frequencyToNote(freq float64) string {
	if freq < 20 || freq > 20000 {
		return "---"
	}

	// A4 = 440 Hz
	a4 := 440.0
	c0 := a4 * math.Pow(2, -4.75) // C0 frequency

	halfSteps := math.Round(12 * math.Log2(freq/c0))
	octave := int(halfSteps / 12)
	note := int(halfSteps) % 12

	noteNames := []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

	return fmt.Sprintf("%s%d", noteNames[note], octave)
}
