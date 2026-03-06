package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"cubelon-emu/spectrum"
	"cubelon-emu/z80"
)

func main() {
	// Command-line flags for fast analysis
	noFrames := flag.Bool("no-frames", false, "Skip saving frames (faster)")
	noAudio := flag.Bool("no-audio", false, "Skip audio generation (faster)")
	saveFrames := flag.String("save-frames", "", "Only save specific frames (e.g. '1000' or '1000-2000' or '1000,2000,3000')")
	stopAt := flag.Int("stop-at", 0, "Stop emulation at specific frame number")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <cubelon_final.tap> [max_frames]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Run to frame 1000 without saving frames (fast analysis)\n")
		fmt.Fprintf(os.Stderr, "  %s --no-frames --stop-at=1000 cubelon_final.tap\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Save only frames 900-1000 for Part 3 analysis\n")
		fmt.Fprintf(os.Stderr, "  %s --save-frames=900-1000 cubelon_final.tap\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Save specific frames for comparison\n")
		fmt.Fprintf(os.Stderr, "  %s --save-frames=800,900,1000 cubelon_final.tap\n\n", os.Args[0])
	}

	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	tapFile := flag.Arg(0)
	maxFrames := 20000 // Safety limit (400 seconds = 6.7 minutes), demo will auto-detect end
	if flag.NArg() > 1 {
		fmt.Sscanf(flag.Arg(1), "%d", &maxFrames)
	}

	// Override maxFrames if stop-at is specified
	if *stopAt > 0 && *stopAt < maxFrames {
		maxFrames = *stopAt + 50 // Add buffer for frame counter detection
	}

	// Parse save-frames specification
	shouldSaveFrame := func(frame int) bool {
		if *noFrames {
			return false
		}
		if *saveFrames == "" {
			return true // Save all frames
		}

		// Parse comma-separated list or ranges
		parts := strings.Split(*saveFrames, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.Contains(part, "-") {
				// Range: "1000-2000"
				rangeParts := strings.Split(part, "-")
				if len(rangeParts) == 2 {
					start, _ := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
					end, _ := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
					if frame >= start && frame <= end {
						return true
					}
				}
			} else {
				// Single frame: "1000"
				targetFrame, _ := strconv.Atoi(part)
				if frame == targetFrame {
					return true
				}
			}
		}
		return false
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

	// Connect memory contention to CPU cycle counter
	mem.CPUCycles = &cpu.Cycles

	// Apply snapshot state
	snapshot.ApplyToSystem(mem, cpu)

	fmt.Printf("ZX Spectrum 128K loaded from snapshot\n")
	fmt.Printf("Starting at PC=0x%04x\n", cpu.PC)

	// Performance options
	if *noFrames {
		fmt.Printf("⚡ Fast mode: Frame saving disabled\n")
	} else if *saveFrames != "" {
		fmt.Printf("⚡ Selective mode: Saving frames %s\n", *saveFrames)
	}
	if *noAudio {
		fmt.Printf("⚡ Fast mode: Audio disabled\n")
	}
	if *stopAt > 0 {
		fmt.Printf("🎯 Target: Stop at frame %d\n", *stopAt)
	}
	fmt.Println()

	// Create frames directory only if saving frames
	if !*noFrames {
		os.MkdirAll("frames", 0755)
	}

	// Create audio buffer (44.1kHz, duration based on max frames)
	var audio *spectrum.AudioBuffer
	if !*noAudio {
		durationSeconds := float64(maxFrames) / 50.0
		audio = spectrum.NewAudioBuffer(44100, durationSeconds)
		// Connect audio to IO for cycle-accurate updates
		io.Audio = audio
	}

	frameCount := 0
	cyclesPerFrame := uint64(69888)
	tapeBlockIdx := 0 // Start from block 0 (BASIC loader)
	interruptCount := 0

	// Track AY writes to detect when demo ends (no AY activity for extended period)
	lastAYWriteCount := 0
	noActivityCount := 0
	const noActivityThreshold = 250 // If no AY writes for 250 frames (5 seconds), demo has ended

	var filename string
	demoStarted := false
	demoFrameOffset := 0

	for frameCount < maxFrames {
		// Create frame buffer for progressive scanline capture
		frameBuffer := spectrum.NewFrameBuffer(mem)
		frameStartCycles := cpu.Cycles

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
			if audio != nil {
				audio.Update(io.AY, cpu.Cycles)
			}

			// Progressive scanline capture - capture lines as they're "scanned"
			// Skip this expensive operation if we're not saving frames
			if !*noFrames {
				// Visible area: scanlines 64-255 (192 lines), starting at T-state 14336
				// Each scanline is 224 T-states
				frameCycle := cpu.Cycles - frameStartCycles
				if frameCycle >= 14336 && frameCycle < 57344 {
					currentScanline := int((frameCycle - 14336) / 224)
					if currentScanline < 192 && currentScanline > frameBuffer.LastCapturedLine {
						frameBuffer.CaptureUpToLine(currentScanline)
					}
				}
			}
		}

		cpu.Cycles -= cyclesPerFrame
		if audio != nil {
			audio.LastCycle -= cyclesPerFrame  // Keep LastCycle in sync!
		}

		// Save the progressively captured frame using demo's frame counter
		// Address 0x8184: region=2, offset=0x0184
		counterRegion := uint16(2)
		counterOffset := uint16(0x0184)
		var frameNumber int

		// Check if demo counter exists and is valid
		if mem.Written[mem.PageReadMap[counterRegion]][counterOffset] {
			counterLow := mem.Banks[mem.PageReadMap[counterRegion]][counterOffset]
			counterHigh := mem.Banks[mem.PageReadMap[counterRegion]][counterOffset+1]
			demoFrame := int(counterLow) | (int(counterHigh) << 8)

			// Demo counter is running (>= 2, skip frame 1 with tape loading artifact)
			if demoFrame >= 2 && demoFrame < 100000 {
				// Auto-detect when demo counter starts
				if !demoStarted {
					demoStarted = true
					demoFrameOffset = frameCount - demoFrame
					fmt.Printf("\n🎬 Demo started: capture frame %d = demo frame %d\n", frameCount, demoFrame)
				}

				// Save frame using demo's internal frame counter
				frameNumber = demoFrame

				// Check if we should save this frame
				if shouldSaveFrame(demoFrame) {
					filename = filepath.Join("frames", fmt.Sprintf("frame_%05d.png", demoFrame))

					// Crop to active square areas to avoid confusion
					// Part 2 (E1): frames 168-801, 180×180 at (38, 6)
					// Part 3 (E2): frames 802-1404, 192×192 at (32, 0)
					// Default: save full frame for other parts
					if demoFrame >= 168 && demoFrame <= 801 {
						// Part 2: Square emergence - crop to 180×180
						frameBuffer.SaveCropped(filename, 38, 6, 180, 180)
					} else if demoFrame >= 802 && demoFrame <= 1404 {
						// Part 3: Geometric pattern - crop to 192×192
						frameBuffer.SaveCropped(filename, 32, 0, 192, 192)
					} else {
						// Other parts: save full frame
						frameBuffer.Save(filename)
					}
				}

				// Stop if we've reached the target frame
				if *stopAt > 0 && demoFrame >= *stopAt {
					fmt.Printf("\n\n🎯 Reached target frame %d, stopping\n", demoFrame)
					frameCount = maxFrames // Break outer loop
					break
				}
			}
			// else: Skip frames 0-1 (tape loading artifacts)
		}
		// else: Skip frames before demo counter exists (tape loading)

		// Print progress every 200 frames
		if frameCount%200 == 0 {
			elapsed := float64(frameCount) / 50.0
			percent := float64(frameCount) / float64(maxFrames) * 100
			if demoStarted {
				fmt.Printf("\rDemo frame %d (capture %d/%d, %.1f%%) - %.1fs - PC: 0x%04x - IRQs: %d",
					frameNumber, frameCount, maxFrames, percent, elapsed, cpu.PC, interruptCount)
			} else {
				fmt.Printf("\rFrame %d/%d (%.1f%%) - %.1fs - PC: 0x%04x - IRQs: %d",
					frameCount, maxFrames, percent, elapsed, cpu.PC, interruptCount)
			}
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
	if demoStarted {
		// Read final demo frame counter at 0x8184
		counterRegion := uint16(2)
		counterOffset := uint16(0x0184)
		counterLow := mem.Banks[mem.PageReadMap[counterRegion]][counterOffset]
		counterHigh := mem.Banks[mem.PageReadMap[counterRegion]][counterOffset+1]
		finalDemoFrame := int(counterLow) | (int(counterHigh) << 8)
		demoDuration := float64(finalDemoFrame) / 50.0

		fmt.Printf("\n\n✅ Captured %d total frames (%.1fs)\n", frameCount, actualDuration)
		fmt.Printf("🎬 Demo frames: 0-%d (%d frames, %.1fs)\n", finalDemoFrame, finalDemoFrame+1, demoDuration)
		fmt.Printf("📼 Pre-demo frames: %d (tape loading)\n", demoFrameOffset)
	} else {
		fmt.Printf("\n\n✅ Captured %d frames (%.1fs)\n", frameCount, actualDuration)
	}
	fmt.Printf("🎬 Loaded %d/%d tape blocks\n", tapeBlockIdx, len(blocks))

	// Write audio to WAV file (3 channels: A, B, C)
	if audio != nil {
		fmt.Printf("🎵 Writing 3-channel audio to cubelon_audio.wav...\n")
		if err := audio.WriteWAV("cubelon_audio.wav"); err != nil {
			fmt.Printf("Error writing audio: %v\n", err)
		} else {
			fmt.Printf("✅ Audio saved (%.1fs, %d samples, 3 channels: A, B, C)\n", actualDuration, audio.Position)
		}
	}
}
