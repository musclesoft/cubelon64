package main

import (
	"fmt"
	"os"

	"cubelon-emu/c64"
	"cubelon-emu/m6502"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: extract_frame <prg_file> <frame_num> <output.bin>")
		os.Exit(1)
	}

	prgPath := os.Args[1]
	frameNum := os.Args[2]
	outputPath := os.Args[3]

	// Load PRG
	data, err := os.ReadFile(prgPath)
	if err != nil {
		fmt.Printf("Error reading PRG: %v\n", err)
		os.Exit(1)
	}

	if len(data) < 2 {
		fmt.Println("Invalid PRG file")
		os.Exit(1)
	}

	// Initialize emulator
	mem := c64.NewMemory()
	mem.LoadPRG(data)
	cpu := m6502.New(mem)

	// Load symbols
	symbols := make(map[string]uint16)
	symbols["PRECOMPUTED_FRAMES"] = 0x1F90 // From map file
	symbols["FRAME"] = 0x0210
	symbols["test_checkpoint_frame"] = 0x1B2D

	// Skip BASIC stub - start at machine code
	cpu.PC = 0x080E

	// Run until breakpoint
	targetFrame := frameNum[0] - '0'
	checkpointAddr := symbols["test_checkpoint_frame"]

	for i := 0; i < 10000000; i++ {
		if cpu.PC == checkpointAddr {
			frame := mem.Read(symbols["FRAME"])
			if frame == byte(targetFrame) {
				// Extract frame data
				frameData := make([]byte, 128)
				frameOffset := uint16(frame) * 128
				for j := 0; j < 128; j++ {
					frameData[j] = mem.Read(symbols["PRECOMPUTED_FRAMES"] + frameOffset + uint16(j))
				}

				// Write to file
				if err := os.WriteFile(outputPath, frameData, 0644); err != nil {
					fmt.Printf("Error writing output: %v\n", err)
					os.Exit(1)
				}

				fmt.Printf("Frame %d extracted to %s\n", frame, outputPath)
				return
			}
		}

		cpu.Step()
	}

	fmt.Println("Timeout: checkpoint not reached")
	os.Exit(1)
}
