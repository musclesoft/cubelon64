// ZX Spectrum music player - recreates AY commands from extracted music data
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

// MusicPlayer recreates the ZX music player logic
type MusicPlayer struct {
	// Music data
	playerCode      []byte
	patternSequence []byte
	patternTable    []byte
	freqTables      []byte

	// Player state
	tickCounter     byte
	sequencePtr     int
	bankSwitch      byte
	channelPointers [14]uint16

	// Output
	commands []AYCommand
	frame    uint32
}

// LoadMusicData loads the extracted music bundle
func LoadMusicData(filename string) (*MusicPlayer, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	player := &MusicPlayer{
		frame:       0,
		sequencePtr: 0,
		bankSwitch:  0x03, // Start with bank 3
		commands:    make([]AYCommand, 0, 120000),
	}

	// Parse the bundle format
	// Skip header "CUBELON_MUSIC_V1\n"
	pos := 0
	for pos < len(data) && data[pos] != '\n' {
		pos++
	}
	pos++ // Skip newline

	// Parse sections
	for pos < len(data) {
		// Read section header: "TYPE:ADDR:SIZE\n"
		lineStart := pos
		for pos < len(data) && data[pos] != '\n' {
			pos++
		}
		if pos >= len(data) {
			break
		}

		header := string(data[lineStart:pos])
		pos++ // Skip newline

		var sectionType string
		var addr, size int
		fmt.Sscanf(header, "%[^:]:%x:%x", &sectionType, &addr, &size)

		if pos+size > len(data) {
			break
		}

		section := data[pos : pos+size]
		pos += size

		switch sectionType {
		case "PLAYER":
			player.playerCode = section
		case "SEQUENCE":
			player.patternSequence = section
		case "PATTERNS":
			player.patternTable = section
		case "FREQTAB":
			player.freqTables = section
		}
	}

	return player, nil
}

// PlayFrame executes one frame of music (called at 50Hz)
func (p *MusicPlayer) PlayFrame() {
	// Decrement tick counter
	if p.tickCounter > 0 {
		p.tickCounter--
		p.frame++
		return
	}

	// Advance to next pattern
	if p.sequencePtr >= len(p.patternSequence) {
		// End of sequence
		return
	}

	patternIndex := p.patternSequence[p.sequencePtr]
	p.sequencePtr++

	// Check for end marker
	if patternIndex == 0xFF {
		// Loop back to start (simplified - actual player does bank switching)
		p.sequencePtr = 0
		patternIndex = p.patternSequence[0]
		p.sequencePtr++
	}

	// Calculate pattern address using the formula from disassembly:
	// offset = ((index >> 3) & 0x1F) << 8 | (index & 0xE0)
	offset := ((int(patternIndex) >> 3) & 0x1F) << 8
	offset |= int(patternIndex) & 0xE0

	if offset >= len(p.patternTable) {
		fmt.Printf("Warning: Pattern index %d (offset $%04X) out of bounds\n", patternIndex, offset)
		p.frame++
		return
	}

	// Read pattern data
	// Format: [tick_count] [ptr0_lo] [ptr0_hi] ... [ptr13_hi] [data...]
	p.tickCounter = p.patternTable[offset]
	offset++

	// Load 14 channel pointers (28 bytes)
	for i := 0; i < 14; i++ {
		if offset+1 >= len(p.patternTable) {
			break
		}
		lo := p.patternTable[offset]
		hi := p.patternTable[offset+1]
		p.channelPointers[i] = uint16(lo) | (uint16(hi) << 8)
		offset += 2
	}

	// Process channel commands (simplified - real player does this per-frame)
	// For now, we'll just generate the initial register writes
	// Real implementation would need to track state per channel

	p.frame++
}

// ProcessChannelData reads commands from a channel pointer
// This is a simplified version - the real player is more complex
func (p *MusicPlayer) ProcessChannelData(channelIdx int) {
	_ = p.channelPointers[channelIdx]

	// Channel data is in the pattern table
	// Pattern format from disassembly analysis:
	// Commands with note data (high nibble $E or $F)
	// Followed by value and register bytes

	// This is a stub - full implementation requires understanding
	// the exact command format from the disassembly
}

// GetCommands returns all generated AY commands
func (p *MusicPlayer) GetCommands() []AYCommand {
	return p.commands
}

// WriteCommand adds an AY register write to the output
func (p *MusicPlayer) WriteCommand(reg, val byte) {
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
	fmt.Printf("Playing music for %d frames...\n", maxFrames)

	for frame := 0; frame < maxFrames; frame++ {
		p.PlayFrame()

		// Check if we've reached the end
		if p.sequencePtr >= len(p.patternSequence) {
			break
		}
	}

	fmt.Printf("Generated %d AY commands over %d frames\n", len(p.commands), p.frame)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: zx_music_player <zx_music_complete.bin> [output.bin]")
		fmt.Println("  Recreates AY commands from extracted ZX music data")
		return
	}

	inputFile := os.Args[1]
	outputFile := "zx_player_output.bin"
	if len(os.Args) >= 3 {
		outputFile = os.Args[2]
	}

	// Load music data
	fmt.Printf("Loading music data from %s...\n", inputFile)
	player, err := LoadMusicData(inputFile)
	if err != nil {
		fmt.Printf("Error loading music: %v\n", err)
		return
	}

	fmt.Printf("✓ Loaded music data:\n")
	fmt.Printf("  Player code: %d bytes\n", len(player.playerCode))
	fmt.Printf("  Pattern sequence: %d bytes\n", len(player.patternSequence))
	fmt.Printf("  Pattern table: %d bytes\n", len(player.patternTable))
	fmt.Printf("  Frequency tables: %d bytes\n\n", len(player.freqTables))

	// Play the music
	maxFrames := 9650 // Full CUBELON demo length
	player.PlayFullSong(maxFrames)

	// Save output
	fmt.Printf("\nSaving output to %s...\n", outputFile)
	if err := player.SaveBinary(outputFile); err != nil {
		fmt.Printf("Error saving output: %v\n", err)
		return
	}

	// Compare with original if available
	originalFile := "../emulator/ay_commands.bin"
	if _, err := os.Stat(originalFile); err == nil {
		fmt.Printf("\nComparing with original %s...\n", originalFile)
		compareAYCommands(outputFile, originalFile)
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

	// Compare first 100 commands
	matches := 0
	mismatches := 0
	pos1, pos2 := 4, 4

	for i := 0; i < 100 && i < int(count1) && i < int(count2); i++ {
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
			if mismatches <= 5 {
				fmt.Printf("  Mismatch at command %d:\n", i)
				fmt.Printf("    Generated: frame=%d reg=%d val=%02X\n", frame1, reg1, val1)
				fmt.Printf("    Original:  frame=%d reg=%d val=%02X\n", frame2, reg2, val2)
			}
		}

		pos1 += 6
		pos2 += 6
	}

	fmt.Printf("\n  First 100 commands: %d matches, %d mismatches\n", matches, mismatches)
}
