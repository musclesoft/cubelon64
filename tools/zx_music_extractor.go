// ZX Music Extractor - Extract music data from CUBELON binary
//
// This tool analyzes the CUBELON ZX Spectrum binary and AY command logs
// to reconstruct the original music format used by the player.
//
// APPROACH:
// 1. Load AY command log (already captured by emulator)
// 2. Analyze command sequences to identify patterns
// 3. Reconstruct frequency table, instruments, and patterns
// 4. Build ZXMusicData structure for replay
//
package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
)

// ========== AY COMMAND LOADING ==========

type AYCommand struct {
	Frame    uint32
	Register uint8
	Value    uint8
}

func LoadAYCommands(filename string) ([]AYCommand, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	var count uint32
	if err := binary.Read(f, binary.LittleEndian, &count); err != nil {
		return nil, fmt.Errorf("read count: %w", err)
	}

	commands := make([]AYCommand, 0, count)
	for i := uint32(0); i < count; i++ {
		var cmd AYCommand
		if err := binary.Read(f, binary.LittleEndian, &cmd.Frame); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("read frame: %w", err)
		}
		if err := binary.Read(f, binary.LittleEndian, &cmd.Register); err != nil {
			return nil, fmt.Errorf("read register: %w", err)
		}
		if err := binary.Read(f, binary.LittleEndian, &cmd.Value); err != nil {
			return nil, fmt.Errorf("read value: %w", err)
		}
		commands = append(commands, cmd)
	}

	return commands, nil
}

// ========== MUSIC ANALYSIS ==========

type NoteEvent struct {
	Frame      uint32
	Channel    int
	Frequency  uint16
	Volume     uint8
	Duration   uint32
}

// ExtractNotesFromCommands analyzes AY commands and extracts note events
func ExtractNotesFromCommands(commands []AYCommand) []NoteEvent {
	fmt.Println("\n=== Extracting Notes from AY Commands ===")

	notes := []NoteEvent{}

	// Track current state per channel
	type ChannelState struct {
		Frequency  uint16
		Volume     uint8
		StartFrame uint32
		Active     bool
	}
	channels := [3]ChannelState{}

	// Process commands in order
	for _, cmd := range commands {
		// Determine which channel this affects
		ch := -1
		switch cmd.Register {
		case 0, 1: // Tone A
			ch = 0
		case 2, 3: // Tone B
			ch = 1
		case 4, 5: // Tone C
			ch = 2
		case 8: // Volume A
			ch = 0
		case 9: // Volume B
			ch = 1
		case 10: // Volume C
			ch = 2
		}

		if ch == -1 {
			continue
		}

		// Update channel state
		state := &channels[ch]

		// Handle register writes
		switch cmd.Register {
		case 0, 2, 4: // Tone low byte
			state.Frequency = (state.Frequency & 0xFF00) | uint16(cmd.Value)
		case 1, 3, 5: // Tone high byte
			state.Frequency = (state.Frequency & 0x00FF) | (uint16(cmd.Value&0x0F) << 8)
		case 8, 9, 10: // Volume
			oldVol := state.Volume
			state.Volume = cmd.Value & 0x0F

			// Detect note start (volume goes from 0 to non-zero)
			if oldVol == 0 && state.Volume > 0 && !state.Active {
				state.Active = true
				state.StartFrame = cmd.Frame
			}

			// Detect note end (volume goes to 0)
			if state.Volume == 0 && state.Active {
				// Save completed note
				duration := cmd.Frame - state.StartFrame
				if duration > 0 {
					notes = append(notes, NoteEvent{
						Frame:     state.StartFrame,
						Channel:   ch,
						Frequency: state.Frequency,
						Volume:    oldVol,
						Duration:  duration,
					})
				}
				state.Active = false
			}
		}
	}

	// Close any active notes at end
	for ch, state := range channels {
		if state.Active && len(commands) > 0 {
			lastFrame := commands[len(commands)-1].Frame
			duration := lastFrame - state.StartFrame
			if duration > 0 {
				notes = append(notes, NoteEvent{
					Frame:     state.StartFrame,
					Channel:   ch,
					Frequency: state.Frequency,
					Volume:    state.Volume,
					Duration:  duration,
				})
			}
		}
	}

	fmt.Printf("Extracted %d notes from %d commands\n", len(notes), len(commands))

	return notes
}

// BuildFrequencyTable creates a sorted table of unique frequencies
func BuildFrequencyTable(notes []NoteEvent) []uint16 {
	freqSet := make(map[uint16]bool)
	for _, note := range notes {
		if note.Frequency > 0 {
			freqSet[note.Frequency] = true
		}
	}

	freqs := make([]uint16, 0, len(freqSet))
	for freq := range freqSet {
		freqs = append(freqs, freq)
	}

	// Sort from low to high (high period = low pitch, low period = high pitch)
	sort.Slice(freqs, func(i, j int) bool {
		return freqs[i] > freqs[j] // Descending order (low pitch to high pitch)
	})

	fmt.Printf("\n=== Frequency Table ===\n")
	fmt.Printf("Total unique frequencies: %d\n", len(freqs))
	if len(freqs) <= 20 {
		fmt.Println("Frequencies (AY periods):")
		for i, freq := range freqs {
			hz := 1773447.0 / (16.0 * float64(freq))
			fmt.Printf("  [%2d] Period=%4d (%.1f Hz)\n", i, freq, hz)
		}
	}

	return freqs
}

// AnalyzeNoteDurations analyzes the distribution of note durations
func AnalyzeNoteDurations(notes []NoteEvent) {
	fmt.Println("\n=== Note Duration Analysis ===")

	durationCounts := make(map[uint32]int)
	for _, note := range notes {
		durationCounts[note.Duration]++
	}

	durations := make([]uint32, 0, len(durationCounts))
	for dur := range durationCounts {
		durations = append(durations, dur)
	}
	sort.Slice(durations, func(i, j int) bool {
		return durationCounts[durations[i]] > durationCounts[durations[j]]
	})

	fmt.Println("Most common durations (frames):")
	for i := 0; i < len(durations) && i < 10; i++ {
		dur := durations[i]
		count := durationCounts[dur]
		fmt.Printf("  %3d frames (%.2fs): %4d occurrences\n", dur, float64(dur)/50.0, count)
	}

	// Find greatest common divisor (likely tick rate)
	if len(durations) > 0 {
		gcd := durations[0]
		for _, dur := range durations[1:] {
			gcd = gcdUint32(gcd, dur)
		}
		fmt.Printf("\nGCD of durations: %d frames (likely tick rate)\n", gcd)
	}
}

func gcdUint32(a, b uint32) uint32 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// AnalyzeChannelActivity analyzes per-channel note distribution
func AnalyzeChannelActivity(notes []NoteEvent) {
	fmt.Println("\n=== Channel Activity ===")

	channelCounts := [3]int{}
	for _, note := range notes {
		if note.Channel >= 0 && note.Channel < 3 {
			channelCounts[note.Channel]++
		}
	}

	for ch := 0; ch < 3; ch++ {
		fmt.Printf("Channel %d: %4d notes\n", ch, channelCounts[ch])
	}
}

// FindMusicStartOffset finds the frame where music begins
func FindMusicStartOffset(notes []NoteEvent) uint32 {
	if len(notes) == 0 {
		return 0
	}

	minFrame := notes[0].Frame
	for _, note := range notes {
		if note.Frame < minFrame {
			minFrame = note.Frame
		}
	}

	return minFrame
}

// ========== MAIN ==========

func main() {
	fmt.Println("ZX Spectrum Music Extractor - CUBELON")
	fmt.Println("=====================================")

	// Check if AY command file exists
	ayCommandFile := "emulator/ay_commands.bin"
	if len(os.Args) > 1 {
		ayCommandFile = os.Args[1]
	}

	// Load AY commands
	commands, err := LoadAYCommands(ayCommandFile)
	if err != nil {
		fmt.Printf("Error loading AY commands: %v\n", err)
		fmt.Println("\nTo generate AY commands, run:")
		fmt.Println("  go run emulator/run_cubelon.go --log-ay-commands")
		return
	}

	fmt.Printf("Loaded %d AY commands\n", len(commands))

	// Extract notes
	notes := ExtractNotesFromCommands(commands)

	// Analyze music data
	startOffset := FindMusicStartOffset(notes)
	fmt.Printf("\nMusic starts at frame %d (%.2f seconds)\n", startOffset, float64(startOffset)/50.0)

	freqTable := BuildFrequencyTable(notes)
	AnalyzeNoteDurations(notes)
	AnalyzeChannelActivity(notes)

	// Statistics
	fmt.Println("\n=== Summary ===")
	fmt.Printf("Total notes: %d\n", len(notes))
	fmt.Printf("Unique frequencies: %d\n", len(freqTable))
	fmt.Printf("Start offset: %d frames\n", startOffset)

	if len(commands) > 0 {
		lastFrame := commands[len(commands)-1].Frame
		duration := float64(lastFrame-startOffset) / 50.0
		fmt.Printf("Music duration: %.1f seconds\n", duration)
	}

	fmt.Println("\n✓ Analysis complete")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Use extracted frequency table in ZXMusicData")
	fmt.Println("  2. Identify repeating patterns for pattern table")
	fmt.Println("  3. Analyze volume envelopes for instrument table")
	fmt.Println("  4. Build stream sequencer data")
}
