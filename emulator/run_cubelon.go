package main

import (
	"fmt"
	"os"
	"path/filepath"
	"cubelon-emu/spectrum"
	"cubelon-emu/z80"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: run_cubelon <cubelon_final.tap> [max_frames]")
		os.Exit(1)
	}

	tapFile := os.Args[1]
	maxFrames := 20000 // Safety limit (400 seconds = 6.7 minutes), demo will auto-detect end
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &maxFrames)
	}

	blocks, err := spectrum.LoadTAP(tapFile)
	if err != nil {
		fmt.Printf("Error loading TAP: %v\n", err)
		os.Exit(1)
	}

	// Load snapshot (system ready to load tape)
	snapshot, err := spectrum.LoadSZX("web/jsspeccy/tapeloaders/tape_128.szx")
	if err != nil {
		fmt.Printf("Error loading snapshot: %v\n", err)
		os.Exit(1)
	}

	rom0Data, _ := os.ReadFile("web/jsspeccy/roms/128-0.rom")
	rom1Data, _ := os.ReadFile("web/jsspeccy/roms/128-1.rom")

	mem := spectrum.NewMemoryV2()
	mem.Load128ROM(rom0Data, rom1Data)
	io := spectrum.NewIO(mem)
	cpu := z80.New(mem, io)

	// Apply snapshot state
	snapshot.ApplyToSystem(mem, cpu)

	fmt.Printf("ZX Spectrum 128K loaded from snapshot\n")
	fmt.Printf("Starting at PC=0x%04x\n\n", cpu.PC)

	os.MkdirAll("frames", 0755)

	// Create audio buffer (44.1kHz, duration based on max frames)
	durationSeconds := float64(maxFrames) / 50.0
	audio := spectrum.NewAudioBuffer(44100, durationSeconds)

	// Connect audio to IO for cycle-accurate updates
	io.Audio = audio

	frameCount := 0
	cyclesPerFrame := uint64(69888)
	tapeBlockIdx := 0 // Start from block 0 (BASIC loader)
	interruptCount := 0

	// Track AY writes to detect when demo ends (no AY activity for extended period)
	lastAYWriteCount := 0
	noActivityCount := 0
	const noActivityThreshold = 250 // If no AY writes for 250 frames (5 seconds), demo has ended

	for frameCount < maxFrames {
		// Fire interrupt at start of frame
		if cpu.IFF1 {
			cpu.Interrupt()
			interruptCount++
		}

		targetCycles := cpu.Cycles + cyclesPerFrame

		for cpu.Cycles < targetCycles {
			// Tape trap
			if (cpu.PC == 0x056b || cpu.PC == 0x0111) && tapeBlockIdx < len(blocks) {
				block := blocks[tapeBlockIdx]

				// Load the block
				dest := cpu.IX
				for i := 0; i < len(block.Data); i++ {
					mem.Write((dest+uint16(i))&0xFFFF, block.Data[i])
				}

				if frameCount < 100 {
					fmt.Printf("Frame %d: Loaded tape block %d (%d bytes) to 0x%04x\n",
						frameCount, tapeBlockIdx, len(block.Data), dest)
				}

				tapeBlockIdx++
				cpu.F |= 0x01  // Set carry flag (success)
				cpu.PC = 0x05E2  // Return from tape routine
				cpu.Cycles += 100
				continue
			}

			cpu.Step()

			// Update audio after each instruction to catch beeper toggles
			audio.Update(io.AY, cpu.Cycles)
		}

		cpu.Cycles -= cyclesPerFrame
		audio.LastCycle -= cyclesPerFrame  // Keep LastCycle in sync!

		// Capture visible screen
		if frameCount%200 == 0 {
			filename := filepath.Join("frames", fmt.Sprintf("frame_%05d.png", frameCount))
			spectrum.CaptureScreen(mem, filename)

			elapsed := float64(frameCount) / 50.0
			percent := float64(frameCount) / float64(maxFrames) * 100
			fmt.Printf("\rFrame %d/%d (%.1f%%) - %.1fs - PC: 0x%04x - IRQs: %d",
				frameCount, maxFrames, percent, elapsed, cpu.PC, interruptCount)
		}

		// Check if demo has ended (no AY activity for extended period)
		if io.AYWriteCount == lastAYWriteCount {
			noActivityCount++
			if noActivityCount >= noActivityThreshold {
				fmt.Printf("\n\n🏁 Demo ended at frame %d (%.1fs) - No AY activity for %d frames\n",
					frameCount, float64(frameCount)/50.0, noActivityThreshold)
				break
			}
		} else {
			lastAYWriteCount = io.AYWriteCount
			noActivityCount = 0
		}

		frameCount++
	}

	actualDuration := float64(frameCount) / 50.0
	fmt.Printf("\n\n✅ Captured %d frames (%.1fs)\n", frameCount, actualDuration)
	fmt.Printf("🎬 Loaded %d/%d tape blocks\n", tapeBlockIdx, len(blocks))

	// Write audio to WAV file (3 channels: A, B, C)
	fmt.Printf("🎵 Writing 3-channel audio to cubelon_audio.wav...\n")
	if err := audio.WriteWAV("cubelon_audio.wav"); err != nil {
		fmt.Printf("Error writing audio: %v\n", err)
	} else {
		fmt.Printf("✅ Audio saved (%.1fs, %d samples, 3 channels: A, B, C)\n", actualDuration, audio.Position)
	}
}
