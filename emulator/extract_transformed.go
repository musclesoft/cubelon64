package main

import (
	"fmt"
	"os"
	"strconv"

	"cubelon-emu/c64"
	"cubelon-emu/m6502"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: extract_transformed <prg_file> <frame_num> <output_charset.bin> <output_sprite.bin>")
		os.Exit(1)
	}

	prgPath := os.Args[1]
	frameNum, _ := strconv.Atoi(os.Args[2])
	outputCharset := os.Args[3]
	outputSprite := os.Args[4]

	// Load PRG
	data, err := os.ReadFile(prgPath)
	if err != nil {
		fmt.Printf("Error reading PRG: %v\n", err)
		os.Exit(1)
	}

	// Initialize emulator
	mem := c64.NewMemory()
	mem.LoadPRG(data)
	cpu := m6502.New(mem)

	// Load symbols
	symbols := make(map[string]uint16)
	symbols["CHARSET_RAM"] = 0x6000
	symbols["SPRITE_DATA"] = 0x6800
	symbols["FRAME"] = 0x0210
	symbols["test_copy_frame"] = 0x0A12

	// Skip BASIC stub - start at machine code
	cpu.PC = 0x080E

	// Run until test_copy_frame is hit for the target frame
	targetFrame := byte(frameNum)
	checkpointAddr := symbols["test_copy_frame"]

	hitCount := 0
	for i := 0; i < 10000000; i++ {
		if cpu.PC == checkpointAddr {
			frame := mem.Read(symbols["FRAME"])
			if frame == targetFrame {
				hitCount++
				// After first hit, copy_frame_to_charset has run for this frame
				if hitCount == 1 {
					// Extract charset data (192 bytes)
					charsetData := make([]byte, 192)
					for j := 0; j < 192; j++ {
						charsetData[j] = mem.Read(symbols["CHARSET_RAM"] + uint16(j))
					}

					// Extract sprite data (63 bytes = 21 rows × 3 bytes)
					spriteData := make([]byte, 63)
					for j := 0; j < 63; j++ {
						spriteData[j] = mem.Read(symbols["SPRITE_DATA"] + uint16(j))
					}

					// Write files
					if err := os.WriteFile(outputCharset, charsetData, 0644); err != nil {
						fmt.Printf("Error writing charset: %v\n", err)
						os.Exit(1)
					}
					if err := os.WriteFile(outputSprite, spriteData, 0644); err != nil {
						fmt.Printf("Error writing sprite: %v\n", err)
						os.Exit(1)
					}

					fmt.Printf("Frame %d extracted:\n", frame)
					fmt.Printf("  Charset: %s (192 bytes)\n", outputCharset)
					fmt.Printf("  Sprite:  %s (63 bytes)\n", outputSprite)
					return
				}
			}
		}

		cpu.Step()
	}

	fmt.Println("Timeout: checkpoint not reached")
	os.Exit(1)
}
