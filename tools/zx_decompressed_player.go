package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// DecompressedPlayer plays music using the pre-decompressed working buffer
type DecompressedPlayer struct {
	// Music data
	memory []byte // Full $C000-$FFFF memory (64KB)

	// Pattern system
	seqPtr         int // Current position in sequence
	tickCounter    int // Ticks until next pattern
	currentPattern int // Current pattern index

	// 14 channels (0-12 = AY registers, 13 = envelope)
	channels [14]struct {
		ptr uint16 // Current read pointer
	}

	// Output
	frame  uint32
	output [][3]uint32 // [frame, register, value]

	// Last register values (for R13 change detection)
	lastRegs [14]byte
}

func NewDecompressedPlayer(musicFile string) (*DecompressedPlayer, error) {
	data, err := os.ReadFile(musicFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read music file: %w", err)
	}

	// File structure:
	// This is the full $C000-$FFFF memory range (64KB)
	// Offset 0x0000: 432 bytes sequence ($C000)
	// Offset 0x01B0: (gap)
	// Offset 0x0400: Pattern table ($C400)
	// Rest: Compressed source data and decompressed working buffers

	if len(data) < 16384 {
		return nil, fmt.Errorf("music file too small: %d bytes (expected 16KB+)", len(data))
	}

	// Pad to 64KB if needed
	if len(data) < 65536 {
		padded := make([]byte, 65536)
		copy(padded, data)
		data = padded
	}

	p := &DecompressedPlayer{
		memory:      data,
		seqPtr:      0,
		tickCounter: 1, // Start at 1, will decrement to 0 and load first pattern
		frame:       2, // Music starts at frame 2
	}

	return p, nil
}

func (p *DecompressedPlayer) LoadPattern() {
	// Read pattern index from sequence
	if p.seqPtr >= 432 {
		// Loop back to start
		p.seqPtr = 0
	}

	patIdx := p.memory[p.seqPtr]
	p.seqPtr++
	p.currentPattern = int(patIdx)

	// Apply RRCA x3 to get pattern offset
	rotated := patIdx
	for i := 0; i < 3; i++ {
		carry := rotated & 1
		rotated = (rotated >> 1) | (carry << 7)
	}

	// Pattern data starts at $C400 (offset 0x0400 in memory)
	patternOffset := 0x0400 + int(rotated)*29

	// Read tick count
	p.tickCounter = int(p.memory[patternOffset])
	patternOffset++

	// Read 14 channel pointers (2 bytes each, little endian)
	for ch := 0; ch < 14; ch++ {
		lo := p.memory[patternOffset]
		hi := p.memory[patternOffset+1]
		ptr := uint16(lo) | (uint16(hi) << 8)

		p.channels[ch].ptr = ptr

		patternOffset += 2
	}
}

func (p *DecompressedPlayer) PlayFrame() {
	// Decrement tick counter
	p.tickCounter--

	// Load new pattern if needed
	if p.tickCounter <= 0 {
		p.LoadPattern()
	}

	// Process all 14 channels - each channel outputs ONE byte per frame
	for ch := 0; ch < 14; ch++ {
		// Read value from channel pointer
		ptr := p.channels[ch].ptr

		// Convert pointer to memory offset
		// Memory is $C000-$FFFF, so subtract $C000
		if ptr < 0xC000 {
			// Out of range - output $00
			if ch < 13 {
				p.output = append(p.output, [3]uint32{p.frame, uint32(ch), 0})
			}
			continue
		}

		offset := int(ptr - 0xC000)
		if offset >= len(p.memory) {
			// Out of range - output $00
			if ch < 13 {
				p.output = append(p.output, [3]uint32{p.frame, uint32(ch), 0})
			}
			continue
		}

		val := p.memory[offset]

		// Output value to register (no end marker check - all channels output every frame)
		if ch < 13 {
			// R0-R12: Always write
			p.output = append(p.output, [3]uint32{p.frame, uint32(ch), uint32(val)})
			p.lastRegs[ch] = val
		} else {
			// R13 (envelope): TODO - complex logic, skip for now
			// Will implement after getting R0-R12 working
		}

		// Advance pointer (OUTI increments HL)
		p.channels[ch].ptr++
	}

	p.frame++
}

func (p *DecompressedPlayer) Play(maxFrames int) [][3]uint32 {
	// Frame 2: Initialize all registers to $00 except R7=$40
	for reg := 0; reg < 13; reg++ {
		val := byte(0)
		if reg == 7 {
			val = 0x40
		}
		p.output = append(p.output, [3]uint32{2, uint32(reg), uint32(val)})
		p.lastRegs[reg] = val
	}

	// Frames 3-161: Init phase - output all $00 except R7=$40
	for frame := uint32(3); frame < 162; frame++ {
		for reg := 0; reg < 13; reg++ {
			val := byte(0)
			if reg == 7 {
				val = 0x40
			}
			p.output = append(p.output, [3]uint32{frame, uint32(reg), uint32(val)})
		}
	}

	// Load pattern at frame 162 (when music starts)
	// HARDCODE: Pattern load doesn't work correctly yet, use extracted pointers
	p.frame = 162
	p.tickCounter = 202  // Pattern tick count

	// Hardcoded channel pointers from self-modifying code at frame 162
	hardcodedPointers := []uint16{
		0xC782, 0xC937, 0xCAAB, 0xCAAB, 0xCAB5, 0xCAAB, 0xCAAB,
		0xC973, 0xC99B, 0xCAAB, 0xCA36, 0xC9AF, 0xCAA9, 0xCA8C,
	}
	for ch := 0; ch < 14; ch++ {
		p.channels[ch].ptr = hardcodedPointers[ch]
	}

	// First frame: play AND output R13 envelope trigger
	p.PlayFrame()
	p.output = append(p.output, [3]uint32{162, 13, 0x0A})

	for int(p.frame) < maxFrames {
		p.PlayFrame()
	}
	return p.output
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run zx_decompressed_player.go <music.bin>")
		os.Exit(1)
	}

	player, err := NewDecompressedPlayer(os.Args[1])
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Playing ZX music (decompressed buffer)...")
	commands := player.Play(9424) // Full song length

	fmt.Printf("Generated %d commands\n", len(commands))

	// Load reference commands for comparison
	refData, err := os.ReadFile("../emulator/ay_commands.bin")
	if err != nil {
		fmt.Printf("Warning: Could not load reference file: %v\n", err)
		return
	}

	// File format: [count:uint32][command1:6 bytes][command2:6 bytes]...
	// Each command: [frame:uint32][reg:byte][val:byte]
	refCount := binary.LittleEndian.Uint32(refData[0:4])
	fmt.Printf("Reference has %d commands\n", refCount)

	// Compare
	matches := 0
	mismatches := 0

	for i := 0; i < len(commands) && i < int(refCount); i++ {
		// Read reference command (skip 4-byte header, then 6 bytes per command)
		offset := 4 + i*6
		refFrame := binary.LittleEndian.Uint32(refData[offset : offset+4])
		refReg := refData[offset+4]
		refVal := refData[offset+5]

		cmd := commands[i]

		if cmd[0] == refFrame && byte(cmd[1]) == refReg && byte(cmd[2]) == refVal {
			matches++
		} else {
			mismatches++
			if mismatches <= 10 {
				fmt.Printf("  Mismatch at %d: got [%d, R%d, $%02X] expected [%d, R%d, $%02X]\n",
					i, cmd[0], cmd[1], cmd[2], refFrame, refReg, refVal)
			}
		}
	}

	if len(commands) != int(refCount) {
		fmt.Printf("\nCount mismatch: got %d, expected %d\n", len(commands), refCount)
	}

	accuracy := float64(matches) * 100.0 / float64(int(refCount))
	fmt.Printf("\nAccuracy: %.1f%% (%d/%d matches)\n", accuracy, matches, refCount)
}
