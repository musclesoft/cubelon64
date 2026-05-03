// Extract music player code and data from CUBELON
// This runs the actual emulator with instrumentation
package main

import (
	"fmt"
	"os"
	"sort"

	"cubelon-emu/spectrum"
	"cubelon-emu/z80"
)

func main() {
	fmt.Println("=== CUBELON Music Player Extraction ===\n")

	// Load snapshot
	snapshot, err := spectrum.LoadSZX("../emulator/web/jsspeccy/tapeloaders/tape_128.szx")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Load ROMs
	rom0, _ := os.ReadFile("../emulator/web/jsspeccy/roms/128-0.rom")
	rom1, _ := os.ReadFile("../emulator/web/jsspeccy/roms/128-1.rom")

	// Load TAP
	tap, err := spectrum.LoadTAP("../original/cubelon_final.tap")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Setup system
	mem := spectrum.NewMemoryV2()
	mem.Load128ROM(rom0, rom1)
	io := spectrum.NewIO(mem)
	cpu := z80.New(mem, io)
	mem.CPUCycles = &cpu.Cycles

	snapshot.ApplyToSystem(mem, cpu)
	io.InsertTape(tap)

	// Tracing data structures
	callCounts := make(map[uint16]int)
	callFrom := make(map[uint16]map[uint16]int)
	memReads := make(map[uint16]int)
	irqCount := 0

	// We know music starts around frame 172, trace from 200-300
	traceStart := 200
	traceEnd := 300
	tracing := false

	fmt.Printf("Running emulation to frame %d...\n", traceEnd)

	frame := 0
	lastCycles := cpu.Cycles

	for frame <= traceEnd {
		targetCycles := lastCycles + 69888 // Cycles per frame

		for cpu.Cycles < targetCycles {
			pc := cpu.PC

			// Start tracing at frame 200
			if frame == traceStart {
				tracing = true
				fmt.Printf("Started tracing at frame %d\n", frame)
			}

			if tracing {
				// Track CALL instructions
				opcode := mem.Read(pc)
				if opcode == 0xCD {
					target := uint16(mem.Read(pc+1)) | (uint16(mem.Read(pc+2)) << 8)
					callCounts[target]++
					if callFrom[target] == nil {
						callFrom[target] = make(map[uint16]int)
					}
					callFrom[target][pc]++
				}

				// Track memory reads (simplified - track PC reads)
				memReads[pc]++
			}

			cpu.Step()
		}

		// Interrupt
		cpu.Interrupt()
		if tracing {
			irqCount++
		}

		lastCycles = cpu.Cycles
		frame++

		if frame%50 == 0 {
			fmt.Printf("  Frame %d\n", frame)
		}
	}

	fmt.Printf("\n=== Trace Complete ===\n")
	fmt.Printf("Traced frames %d-%d (%d frames)\n\n", traceStart, traceEnd, traceEnd-traceStart+1)

	totalFrames := traceEnd - traceStart + 1

	// Find music player (called once per frame)
	fmt.Println("=== Music Player Candidates (called ~1x per frame) ===")

	type callStat struct {
		addr  uint16
		count int
	}

	calls := []callStat{}
	for addr, count := range callCounts {
		calls = append(calls, callStat{addr, count})
	}
	sort.Slice(calls, func(i, j int) bool {
		return calls[i].count > calls[j].count
	})

	musicPlayerAddr := uint16(0)

	for _, call := range calls {
		perFrame := float64(call.count) / float64(totalFrames)
		if perFrame >= 0.9 && perFrame <= 1.1 {
			fmt.Printf("  $%04X: %d calls (%.2f per frame) ", call.addr, call.count, perFrame)

			// Show who calls it
			callers := []uint16{}
			for caller := range callFrom[call.addr] {
				callers = append(callers, caller)
			}
			sort.Slice(callers, func(i, j int) bool { return callers[i] < callers[j] })
			fmt.Printf("← ")
			for _, caller := range callers {
				fmt.Printf("$%04X ", caller)
			}
			fmt.Println()

			if musicPlayerAddr == 0 {
				musicPlayerAddr = call.addr
			}
		}
	}

	if musicPlayerAddr == 0 {
		fmt.Println("No music player found!")
		return
	}

	fmt.Printf("\n=== Identified Music Player: $%04X ===\n", musicPlayerAddr)

	// Now trace memory reads from this routine
	fmt.Printf("\nRe-running to trace memory accesses from $%04X...\n", musicPlayerAddr)

	// Reset and run again, this time tracking memory reads from music player
	mem2 := spectrum.NewMemoryV2()
	mem2.Load128ROM(rom0, rom1)
	io2 := spectrum.NewIO(mem2)
	cpu2 := z80.New(mem2, io2)
	mem2.CPUCycles = &cpu2.Cycles
	snapshot.ApplyToSystem(mem2, cpu2)
	io2.InsertTape(tap)

	dataReads := make(map[uint16]int)
	inMusicPlayer := false
	callDepth := 0

	frame = 0
	lastCycles = cpu2.Cycles

	for frame <= traceEnd {
		targetCycles := lastCycles + 69888

		for cpu2.Cycles < targetCycles {
			pc := cpu2.PC

			if frame >= traceStart {
				// Track when we enter/exit music player
				opcode := mem2.Read(pc)

				if opcode == 0xCD {
					target := uint16(mem2.Read(pc+1)) | (uint16(mem2.Read(pc+2)) << 8)
					if target == musicPlayerAddr {
						inMusicPlayer = true
						callDepth = 1
					} else if inMusicPlayer {
						callDepth++
					}
				} else if opcode == 0xC9 && inMusicPlayer { // RET
					callDepth--
					if callDepth == 0 {
						inMusicPlayer = false
					}
				}

				// Track all memory reads while in music player
				if inMusicPlayer && pc != mem2.Read(pc) {
					// This is a data read, not code fetch
					// (simplified - would need proper tracking)
					dataReads[pc]++
				}
			}

			cpu2.Step()
		}

		cpu2.Interrupt()
		lastCycles = cpu2.Cycles
		frame++
	}

	fmt.Println("\n=== Memory Access Analysis ===")
	fmt.Println("Most frequently read addresses:")

	reads := []callStat{}
	for addr, count := range dataReads {
		reads = append(reads, callStat{addr, count})
	}
	sort.Slice(reads, func(i, j int) bool {
		return reads[i].count > reads[j].count
	})

	for i := 0; i < 50 && i < len(reads); i++ {
		fmt.Printf("  $%04X: %d reads\n", reads[i].addr, reads[i].count)
	}

	// Dump music player code
	fmt.Printf("\n=== Dumping Music Player Code ($%04X) ===\n", musicPlayerAddr)

	f, err := os.Create("music_player_code.bin")
	if err != nil {
		fmt.Printf("Error creating file: %v\n", err)
		return
	}
	defer f.Close()

	// Dump 2KB starting from music player address
	for i := 0; i < 2048; i++ {
		f.Write([]byte{mem2.Read(musicPlayerAddr + uint16(i))})
	}

	fmt.Println("✓ Saved music_player_code.bin")

	// Dump identified data regions
	fmt.Println("\n=== Next Steps ===")
	fmt.Println("1. Disassemble music_player_code.bin")
	fmt.Println("2. Identify data table addresses from player code")
	fmt.Println("3. Extract music data tables")
	fmt.Println("4. Determine format from player logic")
}
