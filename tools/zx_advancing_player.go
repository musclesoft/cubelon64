// Player where channels advance their pointers independently
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

type Channel struct {
	basePtr uint16 // Starting address from pattern
	offset  int    // Current offset from base
	active  bool
}

type Player struct {
	memory    []byte
	seq       []byte
	seqPtr    int
	ticks     int
	frame     uint32
	channels  [14]Channel
	lastRegs  [14]byte // Track last value written to each register
	output    [][3]uint32
}

func NewPlayer() (*Player, error) {
	mem, err := os.ReadFile("zx_music_complete_memory.bin")
	if err != nil {
		return nil, err
	}

	p := &Player{
		memory: mem,
		seq:    mem[0:432],
	}

	// Initialize lastRegs to $FF so first writes are detected as changes
	// Except R13 which starts at $00
	for i := range p.lastRegs {
		if i == 13 {
			p.lastRegs[i] = 0x00
		} else {
			p.lastRegs[i] = 0xFF
		}
	}

	return p, nil
}

func (p *Player) LoadPattern() {
	// Loop sequence
	if p.seqPtr >= len(p.seq) {
		p.seqPtr = 0
	}

	if p.seq[p.seqPtr] == 0xFF {
		p.seqPtr = 0 // Loop on FF marker
	}

	patIdx := p.seq[p.seqPtr]
	p.seqPtr++

	// RRCA x3
	rotated := patIdx
	for i := 0; i < 3; i++ {
		carry := rotated & 1
		rotated = (rotated >> 1) | (carry << 7)
	}
	hi := (rotated & 0x1F) + 0xC4
	lo := rotated & 0xE0
	absAddr := (uint16(hi) << 8) | uint16(lo)
	offset := int(absAddr - 0xC000)

	// Read tick
	p.ticks = int(p.memory[offset])
	offset++

	// Load channel base pointers (but keep current offsets!)
	for ch := 0; ch < 14; ch++ {
		lo := p.memory[offset]
		hi := p.memory[offset+1]
		ptr := uint16(lo) | (uint16(hi) << 8)
		offset += 2

		p.channels[ch].basePtr = ptr
		p.channels[ch].active = (ptr >= 0xC000)
		// Keep offset from previous pattern
	}
}

func (p *Player) PlayFrame() {
	// Don't load patterns during init phase
	// Music starts at frame 162
	if p.frame >= 162 {
		// Load new pattern if needed
		if p.ticks <= 0 {
			p.LoadPattern()
		}
	}

	// Process each channel
	for ch := 0; ch < 14; ch++ { // 14 registers (0-13)
		var val byte = 0x00

		// Before frame 162: initialization phase
		if p.frame < 162 {
			// Initialization phase - only R7 has non-zero value
			if ch == 7 {
				val = 0x40 // Mixer init value
			} else {
				val = 0x00
			}
		} else if p.frame >= 162 {
			// Normal playback from channel data
			if p.channels[ch].active {
				ptr := p.channels[ch].basePtr
				if ptr >= 0xC000 {
					memOffset := int(ptr-0xC000) + p.channels[ch].offset
					if memOffset < len(p.memory) {
						val = p.memory[memOffset]

						// Channel 13 (envelope shape) special handling
						if ch == 13 {
							// $FF means no output (skip)
							if val == 0xFF {
								p.channels[ch].offset++
								continue // Don't output, keep last value
							}
							val = val >> 1
						}

						// Advance channel pointer
						p.channels[ch].offset++
					} else {
						p.channels[ch].active = false
					}
				}
			}
		}

		// Write register
		// Registers 0-12 are written every frame
		// Register 13 is only written when value changes
		if ch < 13 {
			p.output = append(p.output, [3]uint32{p.frame, uint32(ch), uint32(val)})
			p.lastRegs[ch] = val
		} else {
			// Register 13: only write on change
			if val != p.lastRegs[ch] {
				p.output = append(p.output, [3]uint32{p.frame, uint32(ch), uint32(val)})
				p.lastRegs[ch] = val
			}
		}
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

	count := uint32(len(p.output))
	binary.Write(f, binary.LittleEndian, count)

	for _, cmd := range p.output {
		binary.Write(f, binary.LittleEndian, cmd[0])
		f.Write([]byte{byte(cmd[1]), byte(cmd[2])})
	}

	return nil
}

func main() {
	fmt.Println("=== Advancing Channel Player ===")
	fmt.Println("Channels advance independently, persist across patterns\n")

	player, err := NewPlayer()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Skip first 2 frames to match original (starts at frame 2)
	player.frame = 2

	// Pattern will load at frame 162 when music starts
	// No pre-loading needed - pattern loads on first tick expiry
	player.ticks = 0 // Force pattern load at frame 162

	// Play (sequence will loop automatically)
	maxFrames := 10000
	for player.frame < uint32(maxFrames) {
		player.PlayFrame()
	}

	fmt.Printf("Generated %d commands over %d frames\n", len(player.output), player.frame)

	// Save
	outputFile := "zx_advancing_output.bin"
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

	// Compare commands
	matches := 0
	mismatches := 0
	origPos, ourPos := 4, 4

	// Compare first 10000 commands
	compareCount := 10000
	for i := 0; i < compareCount && origPos+5 < len(origData) && ourPos+5 < len(ourData); i++ {
		origFrame := binary.LittleEndian.Uint32(origData[origPos : origPos+4])
		origReg := origData[origPos+4]
		origVal := origData[origPos+5]

		ourFrame := binary.LittleEndian.Uint32(ourData[ourPos : ourPos+4])
		ourReg := ourData[ourPos+4]
		ourVal := ourData[ourPos+5]

		if origFrame == ourFrame && origReg == ourReg && origVal == ourVal {
			matches++
			if matches <= 5 {
				fmt.Printf("  ✓ [%d] f=%d r=%d v=$%02X\n", i, origFrame, origReg, origVal)
			}
		} else {
			mismatches++
			if mismatches <= 20 {
				fmt.Printf("  ✗ [%d] orig(f=%d r=%d v=$%02X) ours(f=%d r=%d v=$%02X)\n",
					i, origFrame, origReg, origVal, ourFrame, ourReg, ourVal)
			}
		}

		origPos += 6
		ourPos += 6
	}

	pct := float64(matches) / float64(compareCount) * 100.0
	fmt.Printf("\nFirst %d commands: %d matches (%.1f%%)\n", compareCount, matches, pct)

	if matches > 400 {
		fmt.Println("\n✓✓✓ EXCELLENT! Format decoded correctly!")
	} else if matches > 200 {
		fmt.Println("\n✓ Good progress!")
	}
}
