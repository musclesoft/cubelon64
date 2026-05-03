// Check how channel pointers advance - maybe some channels skip bytes
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

func main() {
	mem, err := os.ReadFile("zx_music_complete_memory.bin")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	ayData, err := os.ReadFile("../emulator/ay_commands.bin")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("=== Tracing Channel Advancement ===\n")

	// Let's trace Channel 2 specifically
	// At frame 172: R2=$00, Channel 2 byte=$1C
	// At frame 182: R2=$1C, Channel 2 byte should still be $1C

	ch2Ptr := uint16(0xEA37)
	ch2Offset := int(ch2Ptr - 0xC000)

	fmt.Printf("Channel 2 data stream:\n")
	fmt.Printf("Offset $%04X: ", ch2Ptr)
	for i := 0; i < 32; i++ {
		if ch2Offset+i < len(mem) {
			fmt.Printf("%02X ", mem[ch2Offset+i])
		}
		if i == 15 {
			fmt.Printf("\n                ")
		}
	}
	fmt.Println()

	// Parse AY log for register 2
	fmt.Println("\nRegister 2 values over frames 170-190:")

	pos := 4
	count := binary.LittleEndian.Uint32(ayData[0:4])

	for i := uint32(0); i < count; i++ {
		frame := binary.LittleEndian.Uint32(ayData[pos : pos+4])
		reg := ayData[pos+4]
		val := ayData[pos+5]
		pos += 6

		if frame >= 170 && frame <= 190 && reg == 2 {
			fmt.Printf("  Frame %d: R2 = $%02X\n", frame, val)
		}
		if frame > 190 {
			break
		}
	}

	fmt.Println("\n=== Hypothesis ===")
	fmt.Println("If R2 is $00 at frame 172 but channel has $1C,")
	fmt.Println("then channel might:")
	fmt.Println("  1. Not output yet (waiting for condition)")
	fmt.Println("  2. Skip bytes until a marker")
	fmt.Println("  3. Use different timing")

	fmt.Println("\nLet me check if channel bytes have timing markers...")

	// Look for patterns in channel 2 data
	fmt.Println("\nChannel 2 detailed bytes:")
	for i := 0; i < 16; i++ {
		if ch2Offset+i < len(mem) {
			val := mem[ch2Offset+i]
			fmt.Printf("  [%2d] $%02X = %08b", i, val, val)

			// Check bit patterns
			if (val & 0x80) != 0 {
				fmt.Print(" (bit7=1)")
			}
			if (val & 0x40) != 0 {
				fmt.Print(" (bit6=1)")
			}
			if (val & 0x20) != 0 {
				fmt.Print(" (bit5=1 END)")
			}
			fmt.Println()
		}
	}

	fmt.Println("\n=== Key Observation ===")
	fmt.Println("Channel 2 bytes: 1C 1C 1C 1C 1C 1C 1C 3E")
	fmt.Println("Byte $3E = 00111110 has bits 5,4,3,2,1 set")
	fmt.Println("This is likely an END marker (bit 5)")
	fmt.Println()
	fmt.Println("So channel 2 outputs $1C for 7 frames, then ends")
	fmt.Println("But at frame 172 it outputs $00...")
	fmt.Println("This means the channel hasn't started yet at frame 172!")
	fmt.Println()
	fmt.Println("Pattern 8 runs frames 160-179 (20 frames)")
	fmt.Println("Channel 2 starts outputting at frame 182 (pattern 9)")
}
