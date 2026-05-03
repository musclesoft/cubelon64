// Direct player: each channel outputs one byte per frame to its register
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

type Player struct {
	memory []byte
	seq    []byte
	seqPtr int
	ticks  int

	channelPtrs [14]uint16
	frame       uint32

	output [][3]uint32 // [frame, reg, val]
}

func NewPlayer() (*Player, error) {
	mem, err := os.ReadFile("zx_music_complete_memory.bin")
	if err != nil {
		return nil, err
	}

	return &Player{
		memory: mem,
		seq:    mem[0:432],
	}, nil
}

func (p *Player) PlayFrame() {
	// Check if need new pattern
	if p.ticks == 0 {
		if p.seqPtr >= len(p.seq) || p.seq[p.seqPtr] == 0xFF {
			return // End
		}

		patIdx := p.seq[p.seqPtr]
		p.seqPtr++

		// Calculate pattern address (RRCA x3)
		rotated := patIdx
		for i := 0; i < 3; i++ {
			carry := rotated & 1
			rotated = (rotated >> 1) | (carry << 7)
		}
		hi := (rotated & 0x1F) + 0xC4
		lo := rotated & 0xE0
		absAddr := (uint16(hi) << 8) | uint16(lo)
		offset := int(absAddr - 0xC000)

		// Read tick count
		p.ticks = int(p.memory[offset])
		offset++

		// Read 14 channel pointers
		for ch := 0; ch < 14; ch++ {
			lo := p.memory[offset]
			hi := p.memory[offset+1]
			p.channelPtrs[ch] = uint16(lo) | (uint16(hi) << 8)
			offset += 2
		}
	}

	// Process each channel - output one byte to its register
	for ch := 0; ch < 14 && ch < 13; ch++ { // Only first 13 channels map to 13 AY regs
		ptr := p.channelPtrs[ch]
		if ptr < 0xC000 {
			continue
		}

		offset := int(ptr - 0xC000)
		if offset >= len(p.memory) {
			continue
		}

		val := p.memory[offset]

		// Write to AY register (ch maps directly to register)
		p.output = append(p.output, [3]uint32{p.frame, uint32(ch), uint32(val)})

		// Advance pointer
		p.channelPtrs[ch]++
	}

	p.ticks--
	p.frame++
}

func (p *Player) SaveBinary(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write count
	count := uint32(len(p.output))
	binary.Write(f, binary.LittleEndian, count)

	// Write commands
	for _, cmd := range p.output {
		binary.Write(f, binary.LittleEndian, cmd[0]) // frame
		f.Write([]byte{byte(cmd[1]), byte(cmd[2])}) // reg, val
	}

	return nil
}

func main() {
	fmt.Println("=== Direct ZX Player ===")
	fmt.Println("Model: Each channel outputs 1 byte/frame directly to register\n")

	player, err := NewPlayer()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Play
	for player.frame < 1000 {
		player.PlayFrame()
		if player.seqPtr >= len(player.seq) {
			break
		}
	}

	fmt.Printf("Generated %d commands over %d frames\n", len(player.output), player.frame)

	// Save
	outputFile := "zx_direct_output.bin"
	fmt.Printf("Saving to %s...\n", outputFile)
	player.SaveBinary(outputFile)

	// Compare
	fmt.Println("\nComparing with original...")

	origData, _ := os.ReadFile("../emulator/ay_commands.bin")
	ourData, _ := os.ReadFile(outputFile)

	origCount := binary.LittleEndian.Uint32(origData[0:4])
	ourCount := binary.LittleEndian.Uint32(ourData[0:4])

	fmt.Printf("  Original: %d commands\n", origCount)
	fmt.Printf("  Ours:     %d commands\n", ourCount)

	// Compare first 200 commands in detail
	matches := 0
	mismatches := 0
	origPos, ourPos := 4, 4

	for i := 0; i < 200 && origPos+5 < len(origData) && ourPos+5 < len(ourData); i++ {
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
			if mismatches <= 20 {
				fmt.Printf("  [%3d] orig(f=%d r=%d v=$%02X) ours(f=%d r=%d v=$%02X)\n",
					i, origFrame, origReg, origVal, ourFrame, ourReg, ourVal)
			}
		}

		origPos += 6
		ourPos += 6
	}

	fmt.Printf("\nFirst 200: %d matches, %d mismatches (%.1f%% match)\n",
		matches, mismatches, float64(matches)/2.0)

	if matches > 100 {
		fmt.Println("\n✓ Excellent! Format is correct!")
	} else if matches > 50 {
		fmt.Println("\n⚠ Good progress, needs refinement")
	} else {
		fmt.Println("\n✗ Model needs adjustment")
	}
}
