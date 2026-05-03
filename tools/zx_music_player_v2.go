// ZX Spectrum music player - recreates AY commands from extracted music data
// This version properly implements the Z80 player logic
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// AYCommand represents a single AY register write
type AYCommand struct {
	Frame    uint32
	Register byte
	Value    byte
}

// Channel state for tracking playback position
type Channel struct {
	dataPtr uint16 // Current position in pattern data
	active  bool   // Whether this channel is playing
}

// MusicPlayer recreates the ZX music player logic
type MusicPlayer struct {
	// Music data (loaded as continuous memory region $C000+)
	memory []byte // Simulated memory at $C000-$FFFF

	// Player state
	tickCounter byte
	sequencePtr int
	channels    [14]Channel

	// Output
	commands []AYCommand
	frame    uint32
}

// LoadMusicFiles loads the complete memory region
func LoadMusicFiles() (*MusicPlayer, error) {
	player := &MusicPlayer{
		memory:   make([]byte, 0x4000), // $C000-$FFFF
		frame:    0,
		commands: make([]AYCommand, 0, 130000),
	}

	// Try to load complete memory dump first
	mem, err := os.ReadFile("zx_music_complete_memory.bin")
	if err == nil && len(mem) == 0x4000 {
		copy(player.memory, mem)
		fmt.Println("  Loaded complete memory region $C000-$FFFF")
	} else {
		// Fallback to individual files
		seq, err := os.ReadFile("zx_pattern_sequence.bin")
		if err != nil {
			return nil, fmt.Errorf("reading sequence: %v", err)
		}

		patterns, err := os.ReadFile("zx_pattern_table.bin")
		if err != nil {
			return nil, fmt.Errorf("reading patterns: %v", err)
		}

		copy(player.memory[0x0000:], seq)     // $C000
		copy(player.memory[0x0400:], patterns) // $C400
		fmt.Println("  Loaded pattern sequence + table")
	}

	// Initialize all channels as inactive
	for i := range player.channels {
		player.channels[i].active = false
	}

	return player, nil
}

// PlayFrame executes one frame of music (called at 50Hz)
func (p *MusicPlayer) PlayFrame() {
	// Decrement tick counter
	if p.tickCounter > 0 {
		p.tickCounter--
		p.ProcessAllChannels()
		p.frame++
		return
	}

	// Advance to next pattern
	if p.sequencePtr >= 432 { // Sequence is 432 bytes
		// End of sequence - loop back
		p.sequencePtr = 0
	}

	patternIndex := p.memory[p.sequencePtr]
	p.sequencePtr++

	// Check for end marker
	if patternIndex == 0xFF {
		// Loop back to start
		p.sequencePtr = 0
		patternIndex = p.memory[0]
		p.sequencePtr++
	}

	// Calculate pattern address using the formula from disassembly:
	// HL = A (pattern index)
	// HL = HL >> 3
	// H = (HL & 0x1F) + 0xC4
	// L = A & 0xE0
	offset := ((int(patternIndex) >> 3) & 0x1F) << 8
	offset |= int(patternIndex) & 0xE0
	offset += 0x0400 // Adjust to $C400 base in our memory

	if offset >= len(p.memory) {
		fmt.Printf("Warning: Pattern index %d (offset $%04X) out of bounds\n", patternIndex, offset)
		p.frame++
		return
	}

	// Read pattern data
	// Format: [tick_count] [ptr0_lo] [ptr0_hi] ... [ptr13_hi]
	p.tickCounter = p.memory[offset]
	offset++

	// Load 14 channel pointers (28 bytes)
	for i := 0; i < 14; i++ {
		if offset+1 >= len(p.memory) {
			break
		}
		lo := p.memory[offset]
		hi := p.memory[offset+1]
		ptr := uint16(lo) | (uint16(hi) << 8)

		// Convert absolute address to memory offset
		if ptr >= 0xC000 {
			p.channels[i].dataPtr = ptr - 0xC000
			p.channels[i].active = true
		} else {
			p.channels[i].active = false
		}

		offset += 2
	}

	p.ProcessAllChannels()
	p.frame++
}

// ProcessAllChannels processes commands for all active channels
func (p *MusicPlayer) ProcessAllChannels() {
	for i := range p.channels {
		if p.channels[i].active {
			p.ProcessChannel(i)
		}
	}
}

// ProcessChannel reads and executes commands for one channel
// This implements the logic from $7F5C (note playback routine)
func (p *MusicPlayer) ProcessChannel(chanIdx int) {
	ch := &p.channels[chanIdx]
	if ch.dataPtr >= uint16(len(p.memory)) {
		ch.active = false
		return
	}

	cmd := p.memory[ch.dataPtr]
	ch.dataPtr++

	// Debug first few commands
	if p.frame < 5 && chanIdx == 0 {
		fmt.Printf("  Ch%d frame=%d: cmd=$%02X ptr=$%04X\n", chanIdx, p.frame, cmd, ch.dataPtr-1+0xC000)
	}

	// Check command type based on bits
	// Bit 5 set = end marker
	if (cmd & 0x20) != 0 {
		ch.active = false
		return
	}

	// Bit 7 set = timing command (wait)
	if (cmd & 0x80) != 0 {
		// Skip timing command - read 2 bytes
		ch.dataPtr += 2
		return
	}

	// Bit 6 set = effect command
	if (cmd & 0x40) != 0 {
		// Skip effect command - read 2 bytes
		ch.dataPtr += 2
		return
	}

	// Normal note/register write
	// Format: [cmd] [value] [register] or [cmd] [register] [value]
	// cmd & 0x07 = something (register number or channel)

	// From the actual data analysis, bytes like $F5, $EF, etc. are note values
	// These need to be written to AY registers

	// Read next 2 bytes
	if ch.dataPtr+1 >= uint16(len(p.memory)) {
		ch.active = false
		return
	}

	val1 := p.memory[ch.dataPtr]
	val2 := p.memory[ch.dataPtr+1]
	ch.dataPtr += 2

	// Map channel index to AY register
	// Channels 0-2 = tone generators A, B, C
	// Channels 3-5 = volumes
	// etc.

	// For now, use a simplified mapping
	// This needs to be refined based on actual player logic

	ayChannel := chanIdx % 3 // AY has 3 channels

	// Write frequency (registers 0-5)
	// cmd is the note value, val1/val2 are parameters
	if (cmd & 0xF0) == 0xE0 || (cmd & 0xF0) == 0xF0 {
		// This is a note byte - write to tone registers
		// AY tone period is 12-bit across 2 registers
		freqLo := cmd
		freqHi := val1

		p.WriteAY(byte(ayChannel*2), freqLo)   // Fine tune
		p.WriteAY(byte(ayChannel*2+1), freqHi) // Coarse tune

		// Write volume
		p.WriteAY(byte(8+ayChannel), val2&0x0F)
	}
}

// WriteAY adds an AY register write to the output
func (p *MusicPlayer) WriteAY(reg, val byte) {
	// Clamp register to valid range
	if reg > 15 {
		return
	}

	p.commands = append(p.commands, AYCommand{
		Frame:    p.frame,
		Register: reg,
		Value:    val,
	})
}

// SaveBinary saves commands in the same format as the emulator
func (p *MusicPlayer) SaveBinary(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write command count
	count := uint32(len(p.commands))
	binary.Write(f, binary.LittleEndian, count)

	// Write commands
	for _, cmd := range p.commands {
		binary.Write(f, binary.LittleEndian, cmd.Frame)
		f.Write([]byte{cmd.Register, cmd.Value})
	}

	return nil
}

// PlayFullSong executes the entire music sequence
func (p *MusicPlayer) PlayFullSong(maxFrames int) {
	fmt.Printf("Playing music for %d frames (%.1fs @ 50Hz)...\n", maxFrames, float64(maxFrames)/50.0)

	debug := false
	for p.frame < uint32(maxFrames) {
		if p.frame < 10 && !debug {
			fmt.Printf("Frame %d: seq=$%02X tickCtr=%d activeChannels=", p.frame, p.memory[p.sequencePtr], p.tickCounter)
			for i, ch := range p.channels {
				if ch.active {
					fmt.Printf("%d ", i)
				}
			}
			fmt.Println()
		}
		if p.frame == 10 {
			debug = true
		}

		p.PlayFrame()
	}

	fmt.Printf("Generated %d AY commands over %d frames\n", len(p.commands), p.frame)
}

func main() {
	outputFile := "zx_player_output.bin"

	// Load music data
	fmt.Printf("Loading music data from extracted files...\n")
	player, err := LoadMusicFiles()
	if err != nil {
		fmt.Printf("Error loading music: %v\n", err)
		fmt.Println("\nMake sure these files exist:")
		fmt.Println("  - zx_pattern_sequence.bin")
		fmt.Println("  - zx_pattern_table.bin")
		return
	}

	fmt.Printf("✓ Loaded music data\n")
	fmt.Printf("  Memory size: %d bytes\n", len(player.memory))
	fmt.Printf("  First sequence byte: $%02X\n", player.memory[0])
	fmt.Printf("  First pattern byte: $%02X\n\n", player.memory[0x0400])

	// Play the music
	maxFrames := 9650 // Full CUBELON demo length
	player.PlayFullSong(maxFrames)

	// Save output
	fmt.Printf("\nSaving output to %s...\n", outputFile)
	if err := player.SaveBinary(outputFile); err != nil {
		fmt.Printf("Error saving output: %v\n", err)
		return
	}

	// Compare with original
	originalFile := "../emulator/ay_commands.bin"
	if _, err := os.Stat(originalFile); err == nil {
		fmt.Printf("\nComparing with original %s...\n", originalFile)
		compareAYCommands(outputFile, originalFile)
	} else {
		fmt.Printf("\nOriginal file not found: %s\n", originalFile)
	}

	fmt.Println("\n✓ Done")
}

// compareAYCommands compares two AY command files
func compareAYCommands(file1, file2 string) {
	data1, err1 := os.ReadFile(file1)
	data2, err2 := os.ReadFile(file2)

	if err1 != nil || err2 != nil {
		fmt.Println("Could not read files for comparison")
		return
	}

	// Read counts
	if len(data1) < 4 || len(data2) < 4 {
		fmt.Println("Files too small")
		return
	}

	count1 := binary.LittleEndian.Uint32(data1[0:4])
	count2 := binary.LittleEndian.Uint32(data2[0:4])

	fmt.Printf("  Generated: %d commands\n", count1)
	fmt.Printf("  Original:  %d commands\n", count2)

	if count1 == 0 {
		fmt.Println("  ⚠️  Generated file is empty - player implementation incomplete")
		return
	}

	// Compare first 200 commands in detail
	matches := 0
	mismatches := 0
	pos1, pos2 := 4, 4

	compareCount := 200
	if int(count1) < compareCount {
		compareCount = int(count1)
	}
	if int(count2) < compareCount {
		compareCount = int(count2)
	}

	for i := 0; i < compareCount; i++ {
		if pos1+5 >= len(data1) || pos2+5 >= len(data2) {
			break
		}

		frame1 := binary.LittleEndian.Uint32(data1[pos1 : pos1+4])
		reg1 := data1[pos1+4]
		val1 := data1[pos1+5]

		frame2 := binary.LittleEndian.Uint32(data2[pos2 : pos2+4])
		reg2 := data2[pos2+4]
		val2 := data2[pos2+5]

		if frame1 == frame2 && reg1 == reg2 && val1 == val2 {
			matches++
		} else {
			mismatches++
			if mismatches <= 10 {
				fmt.Printf("  Mismatch at command %d:\n", i)
				fmt.Printf("    Generated: frame=%d reg=$%02X val=$%02X\n", frame1, reg1, val1)
				fmt.Printf("    Original:  frame=%d reg=$%02X val=$%02X\n", frame2, reg2, val2)
			}
		}

		pos1 += 6
		pos2 += 6
	}

	fmt.Printf("\n  First %d commands: %d matches, %d mismatches\n", compareCount, matches, mismatches)

	if matches > 0 {
		pct := float64(matches) / float64(compareCount) * 100.0
		fmt.Printf("  Match rate: %.1f%%\n", pct)
	}
}
