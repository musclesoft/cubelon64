// Decode first pattern manually to understand format
package main

import (
	"fmt"
	"os"
)

func main() {
	// Load complete memory
	mem, err := os.ReadFile("zx_music_complete_memory.bin")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("=== Decoding First Pattern ===\n")

	// Pattern sequence starts at $C000
	// From trace: Frame 179 reads sequence byte $02 from $C005
	// Let's decode pattern index $02

	patternIdx := byte(0x02)
	fmt.Printf("Pattern index: $%02X\n", patternIdx)

	// Calculate offset using formula from disassembly
	// From Z80 code at $7EF9-$7F06:
	// RRCA x3 means: rotate right circular 3 times (each bit moves right, wrapping around)
	// This is equivalent to: value rotated right by 3 bits

	// RRCA (Rotate Right Circular Accumulator)
	rotated := patternIdx
	for i := 0; i < 3; i++ {
		carry := rotated & 1
		rotated = (rotated >> 1) | (carry << 7)
	}

	hi := (rotated & 0x1F) + 0xC4
	lo := rotated & 0xE0
	absAddr := (uint16(hi) << 8) | uint16(lo)
	offset := int(absAddr - 0xC000)

	fmt.Printf("Pattern address: $%04X (offset $%04X)\n", absAddr, offset)

	if offset >= len(mem) {
		fmt.Println("Offset out of range")
		return
	}

	// Read pattern structure
	tickCount := mem[offset]
	fmt.Printf("Tick count: %d\n\n", tickCount)
	offset++

	// Read 14 channel pointers
	fmt.Println("Channel pointers:")
	channelPtrs := make([]uint16, 14)
	for i := 0; i < 14; i++ {
		lo := mem[offset]
		hi := mem[offset+1]
		ptr := uint16(lo) | (uint16(hi) << 8)
		channelPtrs[i] = ptr
		fmt.Printf("  Channel %2d: $%04X\n", i, ptr)
		offset += 2
	}

	// Now decode first few commands from channel 0
	fmt.Println("\n=== Channel 0 Commands ===")

	ch0Ptr := channelPtrs[0]
	if ch0Ptr < 0xC000 {
		fmt.Println("Channel 0 pointer invalid")
		return
	}

	ch0Offset := int(ch0Ptr - 0xC000)
	fmt.Printf("Channel 0 data at offset $%04X (absolute $%04X)\n\n", ch0Offset, ch0Ptr)

	// Decode first 10 commands
	for cmd := 0; cmd < 10 && ch0Offset < len(mem); cmd++ {
		cmdByte := mem[ch0Offset]
		fmt.Printf("Cmd %d [@$%04X]: $%02X ", cmd, ch0Ptr+uint16(cmd), cmdByte)
		fmt.Printf("(bit7=%d bit6=%d bit5=%d bits0-2=%d)\n",
			(cmdByte>>7)&1, (cmdByte>>6)&1, (cmdByte>>5)&1, cmdByte&0x07)

		ch0Offset++

		// Decode based on bits
		if (cmdByte & 0x20) != 0 {
			fmt.Println("  → END MARKER")
			break
		}

		if (cmdByte & 0x80) != 0 {
			// Timing command
			if ch0Offset+1 >= len(mem) {
				break
			}
			lo := mem[ch0Offset]
			hi := mem[ch0Offset+1]
			wait := uint16(lo) | (uint16(hi) << 8)
			fmt.Printf("  → TIMING: wait %d frames\n", wait)
			ch0Offset += 2
			continue
		}

		if (cmdByte & 0x40) != 0 {
			// Effect command
			if ch0Offset+1 >= len(mem) {
				break
			}
			lo := mem[ch0Offset]
			hi := mem[ch0Offset+1]
			param := uint16(lo) | (uint16(hi) << 8)
			fmt.Printf("  → EFFECT type=%d param=$%04X\n", cmdByte&0x07, param)
			ch0Offset += 2
			continue
		}

		// Regular note/register command
		if ch0Offset+1 >= len(mem) {
			break
		}
		val1 := mem[ch0Offset]
		val2 := mem[ch0Offset+1]
		ch0Offset += 2

		// High nibble E or F indicates note data
		if (cmdByte&0xF0) == 0xE0 || (cmdByte&0xF0) == 0xF0 {
			fmt.Printf("  → NOTE: note=$%02X val1=$%02X val2=$%02X\n", cmdByte, val1, val2)
		} else {
			fmt.Printf("  → COMMAND: reg=%d? val1=$%02X val2=$%02X\n", cmdByte&0x07, val1, val2)
		}
	}

	// Compare with actual AY output from emulator
	fmt.Println("\n=== Expected AY Output (from emulator log) ===")
	fmt.Println("Frame 172 first writes:")
	fmt.Println("  Reg 0: $E0")
	fmt.Println("  Reg 1: $08")
	fmt.Println("  Reg 4: $8E")
	fmt.Println("  Reg 7: $68")
	fmt.Println("  Reg 8: $10")
	fmt.Println("  Reg 10: $0D")
	fmt.Println("  Reg 11: $24")
}
