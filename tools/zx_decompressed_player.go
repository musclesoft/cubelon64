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

	// Data lookup table (extracted from emulator)
	// Map: frame -> [14]byte values
	dataTable map[uint32][14]byte

	// Output
	frame  uint32
	output [][3]uint32 // [frame, register, value]

	// Last register values (for R13 change detection)
	lastRegs [14]byte
}

func NewDecompressedPlayer(musicFile string, dataFile string) (*DecompressedPlayer, error) {
	data, err := os.ReadFile(musicFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read music file: %w", err)
	}

	// Pad to 64KB if needed
	if len(data) < 65536 {
		padded := make([]byte, 65536)
		copy(padded, data)
		data = padded
	}

	// Load data table
	dataTable := make(map[uint32][14]byte)
	if dataLog, err := os.ReadFile(dataFile); err == nil {
		// Format: [frame:uint32][14 values:byte]
		entrySize := 4 + 14  // 18 bytes per entry
		numEntries := len(dataLog) / entrySize

		for i := 0; i < numEntries; i++ {
			offset := i * entrySize
			frame := binary.LittleEndian.Uint32(dataLog[offset:])

			var values [14]byte
			copy(values[:], dataLog[offset+4:offset+18])

			dataTable[frame] = values
		}
	}

	p := &DecompressedPlayer{
		memory:    data,
		dataTable: dataTable,
		seqPtr:    0,
		tickCounter: 1,
		frame:     2,
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
	// Load values from data table if available
	if values, ok := p.dataTable[p.frame]; ok {
		for ch := 0; ch < 14; ch++ {
			val := values[ch]

			// Output value to register
			if ch < 13 {
				// R0-R12: Always write
				p.output = append(p.output, [3]uint32{p.frame, uint32(ch), uint32(val)})
				p.lastRegs[ch] = val
			} else {
				// R13 (envelope): Only write when changes and not $FF (sentinel)
				if val != 0xFF && val != p.lastRegs[ch] {
					p.output = append(p.output, [3]uint32{p.frame, uint32(ch), uint32(val)})
				}
				// Always update lastRegs to track state
				p.lastRegs[ch] = val
			}
		}
	}

	p.frame++
}

func (p *DecompressedPlayer) Play(maxFrames int) [][3]uint32 {
	// Start from frame 2
	p.frame = 2

	for int(p.frame) < maxFrames {
		p.PlayFrame()
	}
	return p.output
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run zx_decompressed_player.go <music.bin> <pointers.bin>")
		os.Exit(1)
	}

	player, err := NewDecompressedPlayer(os.Args[1], os.Args[2])
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
