// Simple ZX player based on discovered format:
// 14 channels = 14 byte streams, one byte per frame per channel
// Each channel writes to a specific AY register
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// Channel-to-register mapping (hypothesis)
var channelToReg = []int{
	0,  // Ch 0 → Reg 0 (Tone A fine)
	1,  // Ch 1 → Reg 1 (Tone A coarse)
	2,  // Ch 2 → Reg 2 (Tone B fine)
	3,  // Ch 3 → Reg 3 (Tone B coarse)
	4,  // Ch 4 → Reg 4 (Tone C fine)
	5,  // Ch 5 → Reg 5 (Tone C coarse)
	6,  // Ch 6 → Reg 6 (Noise period)
	7,  // Ch 7 → Reg 7 (Mixer)
	8,  // Ch 8 → Reg 8 (Volume A)
	9,  // Ch 9 → Reg 9 (Volume B)
	10, // Ch 10 → Reg 10 (Volume C)
	11, // Ch 11 → Reg 11 (Envelope fine)
	12, // Ch 12 → Reg 12 (Envelope coarse)
	13, // Ch 13 → Reg 13 (Envelope shape)
}

type Player struct {
	memory          []byte
	sequence        []byte
	sequencePtr     int
	tickCounter     int
	channelPtrs     [14]int
	channelActive   [14]bool
	frame           uint32
	commands        [][2]byte // [reg, val]
}

func NewPlayer() (*Player, error) {
	p := &Player{}

	// Load complete memory
	mem, err := os.ReadFile("zx_music_complete_memory.bin")
	if err != nil {
		return nil, err
	}
	p.memory = mem

	// Sequence is at offset 0 ($C000)
	p.sequence = mem[0:432]

	return p, nil
}

func (p *Player) PlayFrame() {
	// Decrement tick counter
	if p.tickCounter > 0 {
		p.tickCounter--
		p.ProcessChannels()
		p.frame++
		return
	}

	// Load next pattern
	if p.sequencePtr >= len(p.sequence) || p.sequence[p.sequencePtr] == 0xFF {
		// End or loop
		return
	}

	patIdx := p.sequence[p.sequencePtr]
	p.sequencePtr++

	// Calculate pattern address
	rotated := patIdx
	for i := 0; i < 3; i++ {
		carry := rotated & 1
		rotated = (rotated >> 1) | (carry << 7)
	}
	hi := (rotated & 0x1F) + 0xC4
	lo := rotated & 0xE0
	absAddr := (uint16(hi) << 8) | uint16(lo)
	offset := int(absAddr - 0xC000)

	if offset >= len(p.memory) {
		return
	}

	// Read tick count
	p.tickCounter = int(p.memory[offset])
	offset++

	// Read 14 channel pointers
	for i := 0; i < 14; i++ {
		lo := p.memory[offset]
		hi := p.memory[offset+1]
		ptr := uint16(lo) | (uint16(hi) << 8)
		offset += 2

		if ptr >= 0xC000 {
			p.channelPtrs[i] = int(ptr - 0xC000)
			p.channelActive[i] = true
		} else {
			p.channelActive[i] = false
		}
	}

	p.ProcessChannels()
	p.frame++
}

func (p *Player) ProcessChannels() {
	// Read one byte from each active channel and write to AY
	for ch := 0; ch < 14; ch++ {
		if !p.channelActive[ch] {
			continue
		}

		ptr := p.channelPtrs[ch]
		if ptr >= len(p.memory) {
			p.channelActive[ch] = false
			continue
		}

		val := p.memory[ptr]

		// Check for end marker (bit 5 set)
		if (val & 0x20) != 0 {
			p.channelActive[ch] = false
			continue
		}

		// Write to AY register
		reg := channelToReg[ch]
		p.commands = append(p.commands, [2]byte{byte(reg), val})

		// Advance pointer
		p.channelPtrs[ch]++
	}
}

func (p *Player) SaveBinary(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// Convert commands to frame-based format
	// Group by frame
	type Command struct {
		frame uint32
		reg   byte
		val   byte
	}

	var allCmds []Command
	cmdIdx := 0
	for frame := uint32(0); frame < p.frame; frame++ {
		// Each frame should have up to 14 commands (one per active channel)
		for ch := 0; ch < 14 && cmdIdx < len(p.commands); ch++ {
			allCmds = append(allCmds, Command{
				frame: frame,
				reg:   p.commands[cmdIdx][0],
				val:   p.commands[cmdIdx][1],
			})
			cmdIdx++
		}
	}

	// Write count
	count := uint32(len(allCmds))
	binary.Write(f, binary.LittleEndian, count)

	// Write commands
	for _, cmd := range allCmds {
		binary.Write(f, binary.LittleEndian, cmd.frame)
		f.Write([]byte{cmd.reg, cmd.val})
	}

	return nil
}

func main() {
	fmt.Println("=== Simple ZX Music Player ===\n")

	player, err := NewPlayer()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Playing music...")

	maxFrames := 1000
	for player.frame < uint32(maxFrames) {
		player.PlayFrame()

		if player.sequencePtr >= len(player.sequence) {
			break
		}
	}

	fmt.Printf("Generated %d commands over %d frames\n", len(player.commands), player.frame)

	// Save output
	outputFile := "zx_simple_output.bin"
	fmt.Printf("\nSaving to %s...\n", outputFile)
	if err := player.SaveBinary(outputFile); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Compare with original
	fmt.Println("\nComparing with original...")

	origData, _ := os.ReadFile("../emulator/ay_commands.bin")
	ourData, _ := os.ReadFile(outputFile)

	origCount := binary.LittleEndian.Uint32(origData[0:4])
	ourCount := binary.LittleEndian.Uint32(ourData[0:4])

	fmt.Printf("  Original: %d commands\n", origCount)
	fmt.Printf("  Ours:     %d commands\n", ourCount)

	// Compare first 100 commands
	matches := 0
	mismatches := 0

	origPos, ourPos := 4, 4
	for i := 0; i < 100 && origPos+5 < len(origData) && ourPos+5 < len(ourData); i++ {
		origFrame := binary.LittleEndian.Uint32(origData[origPos : origPos+4])
		origReg := origData[origPos+4]
		origVal := origData[origPos+5]

		ourFrame := binary.LittleEndian.Uint32(ourData[ourPos : ourPos+4])
		ourReg := ourData[ourPos+4]
		ourVal := ourData[ourPos+5]

		if origFrame == ourFrame && origReg == ourReg && origVal == ourVal {
			matches++
		} else {
			mismatches++
			if mismatches <= 10 {
				fmt.Printf("  Cmd %d: orig(f=%d r=%d v=$%02X) ours(f=%d r=%d v=$%02X)\n",
					i, origFrame, origReg, origVal, ourFrame, ourReg, ourVal)
			}
		}

		origPos += 6
		ourPos += 6
	}

	fmt.Printf("\nFirst 100: %d matches, %d mismatches\n", matches, mismatches)

	if matches > 50 {
		fmt.Println("✓ Good match rate - format understanding is correct!")
	}
}
