// Trace ZX Spectrum music player - Find and disassemble the actual player code
package main

import (
	"fmt"
	"os"
	"sort"

	"cubelon-emu/spectrum"
	"cubelon-emu/z80"
)

func main() {
	fmt.Println("=== ZX Spectrum Music Player Tracer ===\n")

	// Load the TAP file
	tapFile := "../original/cubelon_final.tap"
	if len(os.Args) > 1 {
		tapFile = os.Args[1]
	}

	tap, err := spectrum.LoadTAP(tapFile)
	if err != nil {
		fmt.Printf("Error loading TAP: %v\n", err)
		return
	}

	// Create ZX Spectrum 128K
	mem := spectrum.NewMemory128K()
	cpu := z80.NewZ80(mem)

	// Load TAP into memory
	if err := spectrum.LoadTAPIntoMemory(tap, mem); err != nil {
		fmt.Printf("Error loading TAP into memory: %v\n", err)
		return
	}

	// Set PC to start
	cpu.PC = 0x8000 // Typical start address for ZX demos

	// Track execution
	pcCounts := make(map[uint16]int)
	callTargets := make(map[uint16]int)
	memReads := make(map[uint16]int)
	memWrites := make(map[uint16]int)

	// Music player regions we know about
	musicPlayerStart := uint16(0x9800)
	musicPlayerEnd := uint16(0x9900)

	interruptHandler := uint16(0x7D26)

	// Track when we're in music code
	inMusicCode := false
	musicPCCounts := make(map[uint16]int)

	// Track memory accesses during music code
	musicDataReads := make(map[uint16]int)

	fmt.Println("Running emulation to trace music player...")
	fmt.Println("This will run for ~30 seconds of emulated time")

	maxFrames := 1500 // 30 seconds at 50Hz
	frameCount := 0

	for frameCount < maxFrames {
		// Run one frame worth of cycles (70908 cycles per frame at 3.546895 MHz)
		cyclesPerFrame := 70908
		cyclesRun := 0

		for cyclesRun < cyclesPerFrame {
			pc := cpu.PC

			// Track execution
			pcCounts[pc]++

			// Check if we're in music player code
			if pc >= musicPlayerStart && pc <= musicPlayerEnd {
				inMusicCode = true
				musicPCCounts[pc]++
			} else if pc == interruptHandler {
				// Entering interrupt - music might be called from here
				inMusicCode = true
			}

			// Get instruction
			opcode := mem.Read(pc)

			// Track CALL instructions
			if opcode == 0xCD { // CALL nn
				target := uint16(mem.Read(pc+1)) | (uint16(mem.Read(pc+2)) << 8)
				callTargets[target]++

				if target >= musicPlayerStart && target <= musicPlayerEnd {
					fmt.Printf("Frame %d: CALL to music player at $%04X from $%04X\n",
						frameCount, target, pc)
				}
			}

			// Execute one instruction
			cycles := cpu.Step()
			cyclesRun += cycles

			// If we just executed in music code, track any memory reads
			if inMusicCode {
				// This is simplified - would need to track actual memory accesses
				// during instruction execution
				inMusicCode = false
			}
		}

		frameCount++

		if frameCount % 500 == 0 {
			fmt.Printf("  Frame %d/%d...\n", frameCount, maxFrames)
		}
	}

	fmt.Printf("\n=== Execution Analysis ===\n")
	fmt.Printf("Total unique PCs executed: %d\n", len(pcCounts))
	fmt.Printf("Total CALL targets: %d\n", len(callTargets))

	// Find most-called addresses
	type callStat struct {
		addr  uint16
		count int
	}

	calls := []callStat{}
	for addr, count := range callTargets {
		calls = append(calls, callStat{addr, count})
	}
	sort.Slice(calls, func(i, j int) bool {
		return calls[i].count > calls[j].count
	})

	fmt.Println("\nMost frequently called addresses:")
	for i := 0; i < 20 && i < len(calls); i++ {
		fmt.Printf("  $%04X: %d calls\n", calls[i].addr, calls[i].count)
	}

	// Find code in music player region
	fmt.Println("\nExecution in music player region ($9800-$9900):")
	musicPCs := []callStat{}
	for pc, count := range musicPCCounts {
		musicPCs = append(musicPCs, callStat{pc, count})
	}
	sort.Slice(musicPCs, func(i, j int) bool {
		return musicPCs[i].addr < musicPCs[j].addr
	})

	if len(musicPCs) > 0 {
		fmt.Printf("Found %d instructions executed in music player region\n", len(musicPCs))
		for _, stat := range musicPCs {
			opcode := mem.Read(stat.addr)
			fmt.Printf("  $%04X: %02X (executed %d times)\n", stat.addr, opcode, stat.count)
		}
	} else {
		fmt.Println("No execution found in $9800-$9900 region")
		fmt.Println("Music player might be at a different address")
	}

	// Look for interrupt handler execution
	fmt.Println("\nInterrupt handler region ($7D00-$7E00):")
	irqStart := uint16(0x7D00)
	irqEnd := uint16(0x7E00)
	irqPCs := []callStat{}
	for pc, count := range pcCounts {
		if pc >= irqStart && pc <= irqEnd {
			irqPCs = append(irqPCs, callStat{pc, count})
		}
	}
	sort.Slice(irqPCs, func(i, j int) bool {
		return irqPCs[i].addr < irqPCs[j].addr
	})

	if len(irqPCs) > 0 {
		fmt.Printf("Found %d instructions executed in interrupt region\n", len(irqPCs))
		// Show first 20
		for i := 0; i < 20 && i < len(irqPCs); i++ {
			opcode := mem.Read(irqPCs[i].addr)
			fmt.Printf("  $%04X: %02X (executed %d times)\n",
				irqPCs[i].addr, opcode, irqPCs[i].count)
		}
	}

	fmt.Println("\n✓ Trace complete")
	fmt.Println("\nNext: Examine the called addresses to find music player")
	fmt.Println("Look for addresses with regular call patterns (every N frames)")
}
