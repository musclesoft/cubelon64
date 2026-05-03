// Dump music player code and data from running ZX Spectrum emulator
package main

import (
	"fmt"
	"os"
	"sort"

	"cubelon-emu/spectrum"
	"cubelon-emu/z80"
)

func main() {
	fmt.Println("=== ZX Spectrum Music Memory Dumper ===\n")

	// Load snapshot
	snapshot, err := spectrum.LoadSZX("../emulator/web/jsspeccy/tapeloaders/tape_128.szx")
	if err != nil {
		fmt.Printf("Error loading snapshot: %v\n", err)
		return
	}

	// Load ROMs
	rom0Data, err := os.ReadFile("../emulator/web/jsspeccy/roms/128-0.rom")
	if err != nil {
		fmt.Printf("Error loading ROM 0: %v\n", err)
		return
	}
	rom1Data, err := os.ReadFile("../emulator/web/jsspeccy/roms/128-1.rom")
	if err != nil {
		fmt.Printf("Error loading ROM 1: %v\n", err)
		return
	}

	// Create system
	mem := spectrum.NewMemoryV2()
	mem.Load128ROM(rom0Data, rom1Data)
	io := spectrum.NewIO(mem)
	cpu := z80.New(mem, io)
	mem.CPUCycles = &cpu.Cycles

	// Apply snapshot
	snapshot.ApplyToSystem(mem, cpu)

	// TAP loading is handled by snapshot
	fmt.Println("Loading tape into memory via snapshot...")
	// Tape is loaded via LOAD command from snapshot, just wait for it

	fmt.Println("Running emulation to frame 1000 (20 seconds)...")
	fmt.Println("This will execute the music player and capture memory")

	// Track CALL instructions to find music player
	callCounts := make(map[uint16]int)
	lastCallPC := make(map[uint16]uint16) // target -> caller PC

	// Run until music is playing (frame 1000)
	maxFrames := 1000
	frameCount := 0
	lastCycles := cpu.Cycles

	for frameCount < maxFrames {
		// Run one frame (70908 T-states at 3.546895 MHz)
		targetCycles := lastCycles + 70908

		for cpu.Cycles < targetCycles {
			pc := cpu.PC

			// Check for CALL instruction
			opcode := mem.Read(pc)
			if opcode == 0xCD { // CALL nn
				target := uint16(mem.Read(pc+1)) | (uint16(mem.Read(pc+2)) << 8)
				callCounts[target]++
				lastCallPC[target] = pc
			}

			// Execute instruction
			cpu.Step()

			// Check for errors
			if cpu.PC == 0 {
				fmt.Printf("Error: PC reached 0 at frame %d\n", frameCount)
				return
			}
		}

		// Trigger interrupt at end of frame
		cpu.Interrupt()
		lastCycles = cpu.Cycles

		frameCount++
		if frameCount%250 == 0 {
			fmt.Printf("  Frame %d/%d...\n", frameCount, maxFrames)
		}
	}

	fmt.Printf("\n=== Analysis Complete ===\n")
	fmt.Printf("Ran %d frames\n\n", frameCount)

	// Sort by call count
	type callStat struct {
		addr   uint16
		count  int
		caller uint16
	}

	calls := []callStat{}
	for addr, count := range callCounts {
		calls = append(calls, callStat{addr, count, lastCallPC[addr]})
	}
	sort.Slice(calls, func(i, j int) bool {
		return calls[i].count > calls[j].count
	})

	fmt.Println("Most frequently called routines:")
	for i := 0; i < 30 && i < len(calls); i++ {
		fmt.Printf("  $%04X: %5d calls (last from $%04X)\n",
			calls[i].addr, calls[i].count, calls[i].caller)
	}

	// Look for patterns - music player is usually called 50 times/second
	fmt.Println("\nRoutines called approximately once per frame (music player candidates):")
	for _, call := range calls {
		if call.count >= frameCount-10 && call.count <= frameCount+10 {
			fmt.Printf("  $%04X: %d calls (%d frames)\n", call.addr, call.count, frameCount)
		}
	}

	// Dump memory regions that might contain music data
	fmt.Println("\n=== Memory Dumps ===")

	// Known addresses from disassembly
	regions := []struct {
		name  string
		start uint16
		end   uint16
	}{
		{"Interrupt handler", 0x7D26, 0x7E9D},
		{"Music player area", 0x9800, 0x9900},
		{"Potential data 1", 0xE500, 0xE600},
		{"Potential data 2", 0xEA00, 0xEB00},
		{"Potential data 3", 0xEC00, 0xED00},
		{"Potential data 4", 0xF000, 0xF600},
	}

	for _, region := range regions {
		fmt.Printf("\n%s ($%04X-$%04X):\n", region.name, region.start, region.end)

		// Dump first 64 bytes
		fmt.Printf("First 64 bytes:\n")
		for i := uint16(0); i < 64 && region.start+i <= region.end; i++ {
			if i%16 == 0 {
				fmt.Printf("  %04X: ", region.start+i)
			}
			fmt.Printf("%02X ", mem.Read(region.start+i))
			if i%16 == 15 {
				fmt.Println()
			}
		}
		if 64%16 != 0 {
			fmt.Println()
		}
	}

	// Try to identify patterns in memory
	fmt.Println("\n=== Pattern Analysis ===")

	// Look for frequency tables (sequences of 16-bit values in ascending/descending order)
	fmt.Println("\nSearching for frequency tables...")
	for addr := uint16(0x8000); addr < 0xFFFF-100; addr++ {
		// Check if this looks like a frequency table
		// (at least 20 16-bit values in roughly sorted order)
		values := []uint16{}
		for i := 0; i < 40; i++ {
			if addr+uint16(i*2)+1 > 0xFFFF {
				break
			}
			lo := mem.Read(addr + uint16(i*2))
			hi := mem.Read(addr + uint16(i*2) + 1)
			val := uint16(lo) | (uint16(hi) << 8)
			values = append(values, val)
		}

		// Check if values are roughly sorted (frequency table)
		if len(values) >= 20 {
			sorted := true
			ascending := values[1] > values[0]
			for i := 1; i < len(values); i++ {
				if ascending {
					if values[i] < values[i-1] {
						sorted = false
						break
					}
				} else {
					if values[i] > values[i-1] {
						sorted = false
						break
					}
				}
			}

			if sorted && values[0] > 100 && values[0] < 2000 {
				fmt.Printf("Possible frequency table at $%04X:\n", addr)
				fmt.Printf("  First 10 values: ")
				for i := 0; i < 10 && i < len(values); i++ {
					fmt.Printf("%d ", values[i])
				}
				fmt.Println()

				// Skip ahead to avoid finding same table twice
				addr += uint16(len(values) * 2)
			}
		}
	}

	fmt.Println("\n✓ Memory dump complete")
	fmt.Println("\nNext steps:")
	fmt.Println("1. Identify the music player routine from call patterns")
	fmt.Println("2. Disassemble the player code")
	fmt.Println("3. Find the music data tables")
	fmt.Println("4. Determine the format by analyzing player logic")
}
