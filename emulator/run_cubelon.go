package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"cubelon-emu/spectrum"
	"cubelon-emu/z80"
)

// PC trace for Part 2 algorithm analysis
var pcLog = make(map[uint16]int)

// SaveJob represents a frame save operation to be processed asynchronously
type SaveJob struct {
	SourcePix    []byte // Raw pixel data (full frame buffer)
	SourceStride int    // Stride of source image
	SourceWidth  int    // Width of source image
	SourceHeight int    // Height of source image
	CropX        int    // Crop region X
	CropY        int    // Crop region Y
	CropWidth    int    // Crop region width
	CropHeight   int    // Crop region height
	Filename     string
}

// frameSaveWorker processes frame save jobs from a channel
func frameSaveWorker(jobs <-chan SaveJob, wg *sync.WaitGroup) {
	defer wg.Done()

	// 1-bit monochrome palette (black and white)
	palette := []color.Color{
		color.Gray{Y: 0},   // 0: Black
		color.Gray{Y: 255}, // 1: White
	}

	for job := range jobs {
		// Create output image (either cropped or full) as paletted 1-bit
		var img *image.Paletted

		if job.CropWidth > 0 && job.CropHeight > 0 {
			// Crop the image in the worker thread (1 byte per pixel)
			img = image.NewPaletted(image.Rect(0, 0, job.CropWidth, job.CropHeight), palette)
			bytesPerRow := job.CropWidth

			for row := 0; row < job.CropHeight; row++ {
				srcOffset := (job.CropY+row)*job.SourceStride + job.CropX
				dstOffset := row * img.Stride
				copy(img.Pix[dstOffset:dstOffset+bytesPerRow],
				     job.SourcePix[srcOffset:srcOffset+bytesPerRow])
			}
		} else {
			// Full frame - reconstruct from raw data
			img = image.NewPaletted(image.Rect(0, 0, job.SourceWidth, job.SourceHeight), palette)
			copy(img.Pix, job.SourcePix)
		}

		// Encode PNG
		f, err := os.Create(job.Filename)
		if err != nil {
			fmt.Printf("Error creating %s: %v\n", job.Filename, err)
			continue
		}
		if err := png.Encode(f, img); err != nil {
			fmt.Printf("Error encoding %s: %v\n", job.Filename, err)
		}
		f.Close()
	}
}

type pcStat struct {
	pc    uint16
	count int
}

// writeDisasmSection writes a disassembled section of code
func writeDisasmSection(w io.Writer, stats []pcStat, mem *spectrum.Memory) {
	if len(stats) == 0 {
		return
	}

	startPC := stats[0].pc
	endPC := stats[len(stats)-1].pc

	fmt.Fprintf(w, "; Section $%04X-$%04X\n", startPC, endPC)
	fmt.Fprintf(w, "; -----------------------------\n")

	for _, stat := range stats {
		pc := stat.pc
		opcode := mem.Read(pc)
		op1 := mem.Read(pc + 1)
		op2 := mem.Read(pc + 2)
		op3 := mem.Read(pc + 3)

		// Simple disassembly (most common opcodes)
		var mnemonic string

		switch opcode {
		case 0x00:
			mnemonic = "NOP"
		case 0x01:
			mnemonic = fmt.Sprintf("LD BC,$%02X%02X", op2, op1)

		case 0x02:
			mnemonic = "LD (BC),A"
		case 0x03:
			mnemonic = "INC BC"
		case 0x04:
			mnemonic = "INC B"
		case 0x05:
			mnemonic = "DEC B"

		case 0x06:
			mnemonic = fmt.Sprintf("LD B,$%02X", op1)

		case 0x08:
			mnemonic = "EX AF,AF'"
		case 0x09:
			mnemonic = "ADD HL,BC"
		case 0x0A:
			mnemonic = "LD A,(BC)"
		case 0x0C:
			mnemonic = "INC C"
		case 0x0D:
			mnemonic = "DEC C"
			
		case 0x0E:
			mnemonic = fmt.Sprintf("LD C,$%02X", op1)
			
		case 0x10:
			offset := int8(op1)
			target := uint16(int(pc) + 2 + int(offset))
			mnemonic = fmt.Sprintf("DJNZ $%04X", target)

		case 0x11:
			mnemonic = fmt.Sprintf("LD DE,$%02X%02X", op2, op1)

		case 0x18:
			offset := int8(op1)
			target := uint16(int(pc) + 2 + int(offset))
			mnemonic = fmt.Sprintf("JR $%04X", target)

		case 0x19:
			mnemonic = "ADD HL,DE"

		case 0x20:
			offset := int8(op1)
			target := uint16(int(pc) + 2 + int(offset))
			mnemonic = fmt.Sprintf("JR NZ,$%04X", target)
			
		case 0x16:
			mnemonic = fmt.Sprintf("LD D,$%02X", op1)

		case 0x1A:
			mnemonic = "LD A,(DE)"
		case 0x1F:
			mnemonic = "RRA"

		case 0x21:
			mnemonic = fmt.Sprintf("LD HL,$%02X%02X", op2, op1)

		case 0x22:
			mnemonic = fmt.Sprintf("LD ($%02X%02X),HL", op2, op1)

		case 0x23:
			mnemonic = "INC HL"
		case 0x24:
			mnemonic = "INC H"
		case 0x27:
			mnemonic = "DAA"
		case 0x28:
			offset := int8(op1)
			target := uint16(int(pc) + 2 + int(offset))
			mnemonic = fmt.Sprintf("JR Z,$%04X", target)

		case 0x29:
			mnemonic = "ADD HL,HL"
		case 0x2B:
			mnemonic = "DEC HL"
		case 0x2C:
			mnemonic = "INC L"
		case 0x2D:
			mnemonic = "DEC L"

		case 0x30:
			offset := int8(op1)
			target := uint16(int(pc) + 2 + int(offset))
			mnemonic = fmt.Sprintf("JR NC,$%04X", target)

		case 0x34:
			mnemonic = "INC (HL)"
		case 0x35:
			mnemonic = "DEC (HL)"
		case 0x38:
			offset := int8(op1)
			target := uint16(int(pc) + 2 + int(offset))
			mnemonic = fmt.Sprintf("JR C,$%04X", target)

		case 0x3C:
			mnemonic = "INC A"
		case 0x3F:
			mnemonic = "CCF"
		case 0x2A:
			mnemonic = fmt.Sprintf("LD HL,($%02X%02X)", op2, op1)

		case 0x32:
			mnemonic = fmt.Sprintf("LD ($%02X%02X),A", op2, op1)

		case 0x36:
			mnemonic = fmt.Sprintf("LD (HL),$%02X", op1)

		case 0x3A:
			mnemonic = fmt.Sprintf("LD A,($%02X%02X)", op2, op1)

		case 0x3E:
			mnemonic = fmt.Sprintf("LD A,$%02X", op1)

		case 0x56:
			mnemonic = "LD D,(HL)"
		case 0x5E:
			mnemonic = "LD E,(HL)"

		case 0x66:
			mnemonic = "LD H,(HL)"
		case 0x67:
			mnemonic = "LD H,A"
		case 0x6F:
			mnemonic = "LD L,A"

		case 0x70:
			mnemonic = "LD (HL),B"
		case 0x71:
			mnemonic = "LD (HL),C"
		case 0x73:
			mnemonic = "LD (HL),E"
		case 0x75:
			mnemonic = "LD (HL),L"
		case 0x76:
			mnemonic = "HALT"
		case 0x77:
			mnemonic = "LD (HL),A"
		case 0x78:
			mnemonic = "LD A,B"
		case 0x79:
			mnemonic = "LD A,C"
		case 0x7A:
			mnemonic = "LD A,D"
		case 0x7C:
			mnemonic = "LD A,H"
		case 0x7E:
			mnemonic = "LD A,(HL)"

		case 0x80:
			mnemonic = "ADD A,B"
		case 0x87:
			mnemonic = "ADD A,A"
		case 0x90:
			mnemonic = "SUB B"

		case 0xA3:
			mnemonic = "AND E"

		case 0xAF:
			mnemonic = "XOR A"

		case 0xB1:
			mnemonic = "OR C"
		case 0xB5:
			mnemonic = "OR L"
		case 0xB7:
			mnemonic = "OR A"
		case 0xBA:
			mnemonic = "CP D"
		case 0xBE:
			mnemonic = "CP (HL)"
		case 0xBF:
			mnemonic = "CP A"

		case 0xC0:
			mnemonic = "RET NZ"
		case 0xC1:
			mnemonic = "POP BC"
		case 0xC2:
			mnemonic = fmt.Sprintf("JP NZ,$%02X%02X", op2, op1)

		case 0xC3:
			mnemonic = fmt.Sprintf("JP $%02X%02X", op2, op1)
			
		case 0xC5:
			mnemonic = "PUSH BC"
		case 0xC6:
			mnemonic = fmt.Sprintf("ADD A,$%02X", op1)

		case 0xC8:
			mnemonic = "RET Z"
		case 0xC9:
			mnemonic = "RET"
		case 0xCA:
			mnemonic = fmt.Sprintf("JP Z,$%02X%02X", op2, op1)

		case 0xCB:
			// CB prefix instructions
			switch op1 {
			case 0x27:
				mnemonic = "SLA A"
			case 0x3F:
				mnemonic = "SRL A"
			case 0x7E:
				mnemonic = "BIT 7,(HL)"
			default:
				// Generic CB instruction
				bit := (op1 >> 3) & 0x07
				reg := op1 & 0x07
				regName := []string{"B", "C", "D", "E", "H", "L", "(HL)", "A"}[reg]
				if op1 < 0x40 {
					// Shifts/rotates
					opName := []string{"RLC", "RRC", "RL", "RR", "SLA", "SRA", "SLL", "SRL"}[(op1>>3)&0x07]
					mnemonic = fmt.Sprintf("%s %s", opName, regName)
				} else if op1 < 0x80 {
					mnemonic = fmt.Sprintf("BIT %d,%s", bit, regName)
				} else if op1 < 0xC0 {
					mnemonic = fmt.Sprintf("RES %d,%s", bit-8, regName)
				} else {
					mnemonic = fmt.Sprintf("SET %d,%s", bit-16, regName)
				}
			}

		case 0xCD:
			mnemonic = fmt.Sprintf("CALL $%02X%02X", op2, op1)

		case 0xD0:
			mnemonic = "RET NC"
		case 0xD1:
			mnemonic = "POP DE"
		case 0xD2:
			mnemonic = fmt.Sprintf("JP NC,$%02X%02X", op2, op1)

		case 0xD5:
			mnemonic = "PUSH DE"
		case 0xD6:
			mnemonic = fmt.Sprintf("SUB $%02X", op1)

		case 0xD8:
			mnemonic = "RET C"
		case 0xD9:
			mnemonic = "EXX"
		case 0xDA:
			mnemonic = fmt.Sprintf("JP C,$%02X%02X", op2, op1)

		case 0xDD:
			// DD prefix (IX register) - simplified
			mnemonic = fmt.Sprintf("DD $%02X", op1)

		case 0xE1:
			mnemonic = "POP HL"
		case 0xE3:
			mnemonic = "EX (SP),HL"
		case 0xE5:
			mnemonic = "PUSH HL"
		case 0xEB:
			mnemonic = "EX DE,HL"
		case 0xEF:
			mnemonic = "RST $28"
		case 0xED:
			// ED prefix instructions
			switch op1 {
			case 0x42:
				mnemonic = "SBC HL,BC"
			case 0x43:
				mnemonic = fmt.Sprintf("LD ($%02X%02X),BC", op3, op2)

			case 0x52:
				mnemonic = "SBC HL,DE"

			case 0x73:
				mnemonic = fmt.Sprintf("LD ($%02X%02X),SP", op3, op2)

			case 0x78:
				mnemonic = "IN A,(C)"
			case 0x79:
				mnemonic = "OUT (C),A"
			case 0x7B:
				mnemonic = fmt.Sprintf("LD SP,($%02X%02X)", op3, op2)

			case 0xA0:
				mnemonic = "LDI"
			case 0xA3:
				mnemonic = "OUTI"
			case 0xB0:
				mnemonic = "LDIR"

			default:
				mnemonic = fmt.Sprintf("ED $%02X  ; UNKNOWN", op1)
			}
		case 0xF0:
			mnemonic = "RET P"
		case 0xF1:
			mnemonic = "POP AF"
		case 0xF3:
			mnemonic = "DI"
		case 0xF5:
			mnemonic = "PUSH AF"
		case 0xF9:
			mnemonic = "LD SP,HL"
		case 0xFB:
			mnemonic = "EI"
		case 0xFD:
			// FD prefix (IY register)
			mnemonic = fmt.Sprintf("FD $%02X", op1) // Simplified for now
		case 0xFE:
			mnemonic = fmt.Sprintf("CP $%02X", op1)

		case 0xFF:
			mnemonic = "RST $38"

		default:
			mnemonic = fmt.Sprintf("DB $%02X  ; UNKNOWN", opcode)
		}

		// Write with execution count
		fmt.Fprintf(w, "$%04X: %-20s ; executed %d times\n", pc, mnemonic, stat.count)
	}
}

func main() {
	// Command-line flags for fast analysis
	noFrames := flag.Bool("no-frames", false, "Skip saving frames (faster)")
	noAudio := flag.Bool("no-audio", false, "Skip audio generation (faster)")
	saveFrames := flag.String("save-frames", "", "Only save specific frames (e.g. '1000' or '1000-2000' or '1000,2000,3000')")
	stopAt := flag.Int("stop-at", 0, "Stop emulation at specific frame number")
	traceScreen := flag.String("trace-screen", "", "Trace screen writes to file for frame range (e.g. '0-1000')")
	traceCode := flag.String("trace-code", "", "Trace all instructions for frame:pcrange (e.g. '900:8580-86FF')")
	traceAllCode := flag.String("trace-all-code", "", "Trace ALL executed code for frame range (e.g. '0-1000') - generates full disassembly")

	maxFrames := flag.Int("max-frames", 20000, "Maximum frames to capture (default: 20000)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Automatically looks for cubelon_final.tap in:\n")
		fmt.Fprintf(os.Stderr, "  1. ../original/cubelon_final.tap\n")
		fmt.Fprintf(os.Stderr, "  2. ./cubelon_final.tap\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Run to frame 1000 without saving frames (fast analysis)\n")
		fmt.Fprintf(os.Stderr, "  %s --no-frames --stop-at=1000\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Save only frames 900-1000 for Part 3 analysis\n")
		fmt.Fprintf(os.Stderr, "  %s --save-frames=900-1000\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Save specific frames for comparison\n")
		fmt.Fprintf(os.Stderr, "  %s --save-frames=800,900,1000\n\n", os.Args[0])
	}

	flag.Parse()

	// Automatically find TAP file in default locations
	tapFile := ""
	defaultPaths := []string{
		"../original/cubelon_final.tap",
		"./cubelon_final.tap",
		"cubelon_final.tap",
	}
	for _, path := range defaultPaths {
		if _, err := os.Stat(path); err == nil {
			tapFile = path
			break
		}
	}
	if tapFile == "" {
		fmt.Fprintf(os.Stderr, "Error: cubelon_final.tap not found. Searched:\n")
		for _, path := range defaultPaths {
			fmt.Fprintf(os.Stderr, "  - %s\n", path)
		}
		os.Exit(1)
	}

	fmt.Printf("Using TAP file: %s\n", tapFile)

	// Override maxFrames if stop-at is specified
	if *stopAt > 0 && *stopAt < *maxFrames {
		*maxFrames = *stopAt + 50 // Add buffer for frame counter detection
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

	// Parse trace-screen specification
	traceStart := -1
	traceEnd := -1
	var traceFile *os.File
	if *traceScreen != "" {
		if strings.Contains(*traceScreen, "-") {
			rangeParts := strings.Split(*traceScreen, "-")
			if len(rangeParts) == 2 {
				traceStart, _ = strconv.Atoi(strings.TrimSpace(rangeParts[0]))
				traceEnd, _ = strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			}
		}
		// Create trace file
		var traceErr error
		traceFile, traceErr = os.Create("screen_trace.log")
		if traceErr != nil {
			fmt.Printf("Error creating trace file: %v\n", traceErr)
			os.Exit(1)
		}
		defer traceFile.Close()
		fmt.Fprintf(traceFile, "=== Screen Write Trace (Frames %d-%d) ===\n\n", traceStart, traceEnd)
	}

	// Parse trace-code specification (e.g., "900:8580-86FF")
	traceCodeFrame := -1
	traceCodePCStart := uint16(0)
	traceCodePCEnd := uint16(0)
	var traceCodeFile *os.File
	if *traceCode != "" {
		parts := strings.Split(*traceCode, ":")
		if len(parts) == 2 {
			traceCodeFrame, _ = strconv.Atoi(parts[0])
			pcRange := strings.Split(parts[1], "-")
			if len(pcRange) == 2 {
				pcStart, _ := strconv.ParseUint(strings.TrimSpace(pcRange[0]), 16, 16)
				pcEnd, _ := strconv.ParseUint(strings.TrimSpace(pcRange[1]), 16, 16)
				traceCodePCStart = uint16(pcStart)
				traceCodePCEnd = uint16(pcEnd)
			}
		}
		// Create code trace file
		var traceCodeErr error
		traceCodeFile, traceCodeErr = os.Create("code_trace.log")
		if traceCodeErr != nil {
			fmt.Printf("Error creating code trace file: %v\n", traceCodeErr)
			os.Exit(1)
		}
		defer traceCodeFile.Close()
		fmt.Fprintf(traceCodeFile, "=== Code Trace (Frame %d, PC $%04X-$%04X) ===\n\n", traceCodeFrame, traceCodePCStart, traceCodePCEnd)
	}

	// Parse trace-all-code flag
	var traceAllCodeStart, traceAllCodeEnd int
	var pcExecutionCount map[uint16]int
	if *traceAllCode != "" {
		parts := strings.Split(*traceAllCode, "-")
		if len(parts) == 2 {
			traceAllCodeStart, _ = strconv.Atoi(parts[0])
			traceAllCodeEnd, _ = strconv.Atoi(parts[1])
			pcExecutionCount = make(map[uint16]int)
			fmt.Printf("🔍 Tracing ALL code execution for frames %d-%d\n", traceAllCodeStart, traceAllCodeEnd)
		}
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
		durationSeconds := float64(*maxFrames) / 50.0
		audio = spectrum.NewAudioBuffer(44100, durationSeconds)
		// Connect audio to IO for cycle-accurate updates
		io.Audio = audio
	}

	// Set up frame save worker pool
	var saveJobs chan SaveJob
	var saveWg sync.WaitGroup
	const numWorkers = 128
	if !*noFrames {
		saveJobs = make(chan SaveJob, numWorkers*2) // Buffer for 2x workers
		for i := 0; i < numWorkers; i++ {
			saveWg.Add(1)
			go frameSaveWorker(saveJobs, &saveWg)
		}
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
	currentDemoFrame := 0

	// Track previous frame pixels for diff detection
	var previousFramePixels []byte

	// Track AY register activity
	type ayFrameStat struct {
		frame       int
		writesTotal int
		writesDelta int
	}
	var ayStats []ayFrameStat
	lastAYTotal := 0

	for frameCount < *maxFrames {
		// Create frame buffer for progressive scanline capture
		frameBuffer := spectrum.NewFrameBuffer(mem)
		frameStartCycles := cpu.Cycles

		// Fire interrupt at start of frame
		if cpu.IFF1 {
			cpu.Interrupt()
			interruptCount++
		}

		targetCycles := cpu.Cycles + cyclesPerFrame

		// SP trace state
		var spTraceEnabled bool
		var spTraceSquareCount int
		var spTracePatternBytes []byte
		var screenWrites []struct{ addr uint16; val byte; pc uint16 }
		var instructionLog []string

		for cpu.Cycles < targetCycles {
			// SP tracing for frame 337 - track POP instructions during drawing
			if currentDemoFrame == 337 {
				// Entering draw routine
				if cpu.PC == 0x859D && !spTraceEnabled {
					spTraceEnabled = true
					spTraceSquareCount++
					spTracePatternBytes = make([]byte, 0, 80)
					instructionLog = make([]string, 0, 200)
					screenWrites = screenWrites[:0]

					if spTraceSquareCount == 1 {  // Only trace first square
						fmt.Printf("\n=== Frame 337: Drawing Square 1 at PC=0x859D ===\n")
						fmt.Printf("HL=0x%04x, SP=0x%04x\n", cpu.HL(), cpu.SP)
					}
				}

				// Track ALL instructions during drawing, plus screen writes
				if spTraceEnabled && spTraceSquareCount == 1 {
					opcode := mem.Read(cpu.PC)

					// Log key instructions
					if cpu.PC >= 0x859D && cpu.PC <= 0x866D {
						instrDesc := ""
						switch opcode {
						case 0xC1: // POP BC
							sp := cpu.SP
							byte1 := mem.Read(sp)
							byte2 := mem.Read(sp + 1)
							spTracePatternBytes = append(spTracePatternBytes, byte1, byte2)
							instrDesc = fmt.Sprintf("POP BC (SP=%04x -> [%02X %02X])", sp, byte1, byte2)
						case 0xF9: // LD SP,HL
							instrDesc = fmt.Sprintf("LD SP,HL (HL=%04x)", cpu.HL())
						case 0x71: // LD (HL),C
							instrDesc = fmt.Sprintf("LD (HL),C (HL=%04x, C=%02X)", cpu.HL(), cpu.BC()&0xFF)
						case 0x70: // LD (HL),B
							instrDesc = fmt.Sprintf("LD (HL),B (HL=%04x, B=%02X)", cpu.HL(), cpu.BC()>>8)
						case 0x23: // INC HL
							instrDesc = fmt.Sprintf("INC HL (HL=%04x)", cpu.HL())
						case 0x24: // INC H
							instrDesc = "INC H"
						case 0x10: // DJNZ
							offset := int8(mem.Read(cpu.PC + 1))
							target := uint16(int(cpu.PC) + 2 + int(offset))
							instrDesc = fmt.Sprintf("DJNZ %04x (B=%02X)", target, cpu.BC()>>8)
						case 0x06: // LD B,n
							instrDesc = fmt.Sprintf("LD B,%02X", mem.Read(cpu.PC+1))
						case 0xC9: // RET
							instrDesc = "RET"
						}

						if instrDesc != "" && len(instructionLog) < 200 {
							logEntry := fmt.Sprintf("%04x: %s", cpu.PC, instrDesc)
							instructionLog = append(instructionLog, logEntry)
							if len(instructionLog) <= 50 {  // Print first 50
								fmt.Printf("  %s\n", logEntry)
							}
						}

						// Track screen writes (0x4000-0x5AFF = Bank 5 screen memory)
						// Check if next instruction will write to screen
						if opcode == 0x71 || opcode == 0x70 {  // LD (HL),C or LD (HL),B
							addr := cpu.HL()
							if addr >= 0x4000 && addr <= 0x5AFF {
								var val byte
								if opcode == 0x71 {
									val = byte(cpu.BC() & 0xFF)
								} else {
									val = byte(cpu.BC() >> 8)
								}
								screenWrites = append(screenWrites, struct{ addr uint16; val byte; pc uint16 }{addr, val, cpu.PC})
								fmt.Printf("  → Screen write: addr=%04x val=%02X\n", addr, val)
							}
						}
					}
				}

				// Exiting draw routine
				if spTraceEnabled && cpu.PC == 0x866D && spTraceSquareCount == 1 {
					fmt.Printf("\n=== Exit Draw Routine ===\n")
					fmt.Printf("Total instructions logged: %d\n", len(instructionLog))
					fmt.Printf("Total screen writes: %d\n", len(screenWrites))
					fmt.Printf("\n=== Pattern Data Captured (%d bytes) ===\n", len(spTracePatternBytes))

					fmt.Printf("Hex: ")
					for i, b := range spTracePatternBytes {
						fmt.Printf("%02X ", b)
						if (i+1)%16 == 0 {
							fmt.Printf("\n     ")
						}
					}
					fmt.Printf("\n\nVisual (2 bytes per row):\n")

					for row := 0; row < 12 && row*2+1 < len(spTracePatternBytes); row++ {
						b1 := spTracePatternBytes[row*2]
						b2 := spTracePatternBytes[row*2+1]

						bits := ""
						for bit := 7; bit >= 0; bit-- {
							if (b1 & (1 << bit)) != 0 {
								bits += "█"
							} else {
								bits += "·"
							}
						}
						for bit := 7; bit >= 4; bit-- {
							if (b2 & (1 << bit)) != 0 {
								bits += "█"
							} else {
								bits += "·"
							}
						}

						fmt.Printf("Row %2d: %02X %02X = %s\n", row, b1, b2, bits)
					}
					fmt.Printf("\nExpected: ········████ repeated\n\n")

					// Analyze screen writes
					fmt.Printf("=== Screen Write Analysis ===\n")
					fmt.Printf("Total writes to screen memory: %d\n\n", len(screenWrites))
					if len(screenWrites) > 0 {
						fmt.Printf("First 24 screen writes (showing address and value):\n")
						for i := 0; i < len(screenWrites) && i < 24; i++ {
							sw := screenWrites[i]
							// Calculate screen row from address
							// ZX Spectrum screen address formula: 0x4000 + (Y&7)*256 + (Y&56)*32 + X/8
							addr := sw.addr - 0x4000
							yLow := (addr >> 8) & 0x07
							yMid := (addr >> 5) & 0x07
							yHi := (addr >> 11) & 0x03
							y := (yHi << 6) | (yMid << 3) | yLow
							x := addr & 0x1F

							fmt.Printf("  %2d: addr=%04x (Y=%3d X=%2d) val=%02X", i, sw.addr, y, x, sw.val)

							// Show as bits
							bits := ""
							for bit := 7; bit >= 0; bit-- {
								if (sw.val & (1 << bit)) != 0 {
									bits += "█"
								} else {
									bits += "·"
								}
							}
							fmt.Printf(" [%s]\n", bits)
						}

						// Group writes by Y coordinate to see pattern
						fmt.Printf("\nWrites grouped by Y coordinate:\n")
						yGroups := make(map[int][]struct{x int; val byte})
						for _, sw := range screenWrites {
							addr := sw.addr - 0x4000
							yLow := (addr >> 8) & 0x07
							yMid := (addr >> 5) & 0x07
							yHi := (addr >> 11) & 0x03
							y := int((yHi << 6) | (yMid << 3) | yLow)
							x := int(addr & 0x1F)
							yGroups[y] = append(yGroups[y], struct{x int; val byte}{x, sw.val})
						}

						// Print in Y order
						for y := 0; y < 192; y++ {
							if writes, ok := yGroups[y]; ok && len(writes) > 0 {
								fmt.Printf("  Y=%3d: ", y)
								for _, w := range writes {
									fmt.Printf("X=%2d:%02X ", w.x, w.val)
								}
								fmt.Printf("\n")
							}
						}
					}
					fmt.Printf("\n")

					spTraceEnabled = false
				}
			}

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

			// Screen write tracing (if enabled)
			if traceFile != nil && currentDemoFrame >= traceStart && currentDemoFrame <= traceEnd {
				pc := cpu.PC
				op := mem.Read(pc)
				op2 := mem.Read(pc + 1)

				// Check if this instruction writes to screen memory
				writeAddr := uint16(0)
				writeVal := byte(0)
				writes := false
				instrName := ""

				// Decode write instructions
				if op >= 0x70 && op <= 0x77 && op != 0x76 { // LD (HL),r
					writeAddr = cpu.HL()
					instrName = fmt.Sprintf("LD (HL),%s", []string{"B","C","D","E","H","L","","A"}[op&7])
					writes = writeAddr >= 0x4000 && writeAddr < 0x5B00
					if writes {
						switch op & 7 {
						case 0: writeVal = byte(cpu.BC() >> 8)
						case 1: writeVal = byte(cpu.BC() & 0xFF)
						case 2: writeVal = byte(cpu.DE() >> 8)
						case 3: writeVal = byte(cpu.DE() & 0xFF)
						case 4: writeVal = byte(cpu.HL() >> 8)
						case 5: writeVal = byte(cpu.HL() & 0xFF)
						case 7: writeVal = cpu.A
						}
					}
				} else if op == 0x12 { // LD (DE),A
					writeAddr = cpu.DE()
					instrName = "LD (DE),A"
					writes = writeAddr >= 0x4000 && writeAddr < 0x5B00
					if writes {
						writeVal = cpu.A
					}
				} else if op == 0x02 { // LD (BC),A
					writeAddr = cpu.BC()
					instrName = "LD (BC),A"
					writes = writeAddr >= 0x4000 && writeAddr < 0x5B00
					if writes {
						writeVal = cpu.A
					}
				} else if op == 0x36 { // LD (HL),n
					writeAddr = cpu.HL()
					instrName = fmt.Sprintf("LD (HL),$%02X", op2)
					writes = writeAddr >= 0x4000 && writeAddr < 0x5B00
					if writes {
						writeVal = op2
					}
				}

				// Log screen writes
				if writes {
					af := (uint16(cpu.A) << 8) | uint16(cpu.F)
					fmt.Fprintf(traceFile, "F%04d PC:%04X | %02X %02X | %-16s | AF:%04X BC:%04X DE:%04X HL:%04X | -> [%04X]=%02X\n",
						currentDemoFrame, pc, op, op2, instrName,
						af, cpu.BC(), cpu.DE(), cpu.HL(),
						writeAddr, writeVal)
				}
			}

			// Full code tracing (if enabled for this frame and PC range)
			if traceCodeFile != nil && currentDemoFrame == traceCodeFrame {
				pc := cpu.PC
				if pc >= traceCodePCStart && pc <= traceCodePCEnd {
					op := mem.Read(pc)
					op2 := mem.Read(pc + 1)
					op3 := mem.Read(pc + 2)
					op4 := mem.Read(pc + 3)

					af := (uint16(cpu.A) << 8) | uint16(cpu.F)
					fmt.Fprintf(traceCodeFile, "%04X: %02X %02X %02X %02X | AF:%04X BC:%04X DE:%04X HL:%04X SP:%04X\n",
						pc, op, op2, op3, op4,
						af, cpu.BC(), cpu.DE(), cpu.HL(), cpu.SP)
				}
			}

			cpu.Step()

			// Track ALL executed PC addresses if traceAllCode is enabled
			if pcExecutionCount != nil && currentDemoFrame >= traceAllCodeStart && currentDemoFrame <= traceAllCodeEnd {
				pcExecutionCount[cpu.PC]++
			}

			// Track PC addresses during Part 2 for algorithm analysis
			if currentDemoFrame >= 168 && currentDemoFrame <= 180 {
				pcLog[cpu.PC]++
			}

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
			currentDemoFrame = demoFrame

			// Check at START of frame (before drawing)
			if demoFrame == 337 && cpu.Cycles < cyclesPerFrame/4 {
				fmt.Printf("\n=== Frame 337 START: Check pattern at $AD66 ===\n")
				patternStartAddr := uint16(0xAD66)
				offset := patternStartAddr - 0x8000
				bank := mem.PageReadMap[2]

				fmt.Printf("Bank %d, First 18 bytes at $AD66:\n  ", bank)
				for i := 0; i < 18; i++ {
					fmt.Printf("%02X ", mem.Banks[bank][offset+uint16(i)])
				}
				fmt.Printf("\n\n")
			}

			// Dump pattern data and addresses during Part 2
			if demoFrame == 337 && cpu.Cycles > cyclesPerFrame*3/4 {
				// Dump pattern table AND the actual pattern buffer contents
				fmt.Printf("\n\n=== Complete Pattern Analysis (Frame %d) ===\n", demoFrame)

				patternTable := uint16(0x84c0)
				fmt.Printf("\nPattern table entries:\n")
				uniqueAddrs := make(map[uint16]bool)

				for i := 0; i < 64; i++ {
					addr := patternTable + uint16(i*2)
					lo := mem.Read(addr)
					hi := mem.Read(addr + 1)
					patternAddr := uint16(lo) | (uint16(hi) << 8)
					fmt.Printf("Square %2d: 0x%04x\n", i, patternAddr)
					uniqueAddrs[patternAddr] = true
				}

				// Dump the actual pattern buffers
				fmt.Printf("\nUnique pattern buffer addresses and their contents:\n")
				for addr := range uniqueAddrs {
					fmt.Printf("\nPattern at 0x%04x:\n", addr)
					for row := 0; row < 12; row++ {
						byte1 := mem.Read(addr + uint16(row*3/2))
						byte2 := mem.Read(addr + uint16(row*3/2+1))
						fmt.Printf("  Row %2d: %02x %02x\n", row, byte1, byte2)
					}
				}

				// Dump complete pattern buffer range $AD00-$B000 (768 bytes)
				fmt.Printf("\n=== Dumping Pattern Buffer Range $AD00-$B000 ===\n")
				fmt.Printf("Current memory bank configuration:\n")
				fmt.Printf("  Bank at $0000-$3FFF: Bank %d (ROM)\n", mem.PageReadMap[0])
				fmt.Printf("  Bank at $4000-$7FFF: Bank %d (RAM)\n", mem.PageReadMap[1])
				fmt.Printf("  Bank at $8000-$BFFF: Bank %d (RAM) <-- PATTERN BUFFER\n", mem.PageReadMap[2])
				fmt.Printf("  Bank at $C000-$FFFF: Bank %d (RAM)\n", mem.PageReadMap[3])

				// The active bank at region 2 is what we should dump
				activeBank := mem.PageReadMap[2]
				fmt.Printf("\n*** Patterns will be read from Bank %d ***\n", activeBank)

				startAddr := uint16(0xA700)  // Start earlier to include 0xA786
				endAddr := uint16(0xB200)     // Extended range
				bufferSize := int(endAddr - startAddr)

				// Try reading from ALL banks to find where the pattern data actually is
				fmt.Printf("\nSearching for pattern data in all RAM banks...\n")

				// ALSO search for the actual screen pattern bytes
				// Frame 319: 03 FF C0, Frame 337: 00 F0 00 F0
				fmt.Printf("\nSearching all banks for screen pattern bytes (00 F0 00 F0 for frame 337)...\n")
				target := []byte{0x00, 0xF0, 0x00, 0xF0, 0x00, 0xF0}
				for bank := 0; bank < 8; bank++ {
					for offset := uint16(0); offset < 0x4000-6; offset++ {
						match := true
						for i := 0; i < len(target); i++ {
							if mem.Banks[bank][offset+uint16(i)] != target[i] {
								match = false
								break
							}
						}
						if match {
							fmt.Printf("  Bank %d: FOUND at offset 0x%04x (addr 0x%04x)\n",
								bank, offset, 0x8000+offset)
							// Show context
							fmt.Printf("    ")
							for i := 0; i < 18 && offset+uint16(i) < 0x4000; i++ {
								fmt.Printf("%02x ", mem.Banks[bank][offset+uint16(i)])
							}
							fmt.Printf("\n")
							break  // Only show first match per bank
						}
					}
				}
				fmt.Printf("\n")

				// Check pattern addresses from pattern table
				// At frame 319: Square 0 uses 0xA786
				// At frame 500: Square 0 uses 0xAD26
				// Both are in region 2 (0x8000-0xBFFF)
				sampleOffset := uint16(0x2786)  // 0xA786 - 0x8000

				for bank := 0; bank < 8; bank++ {
					// Read directly from bank
					hasData := false
					for i := 0; i < 18; i++ {
						if mem.Banks[bank][sampleOffset + uint16(i)] != 0 {
							hasData = true
							break
						}
					}

					if hasData {
						fmt.Printf("  Bank %d: HAS DATA at offset 0x%04x (addr 0xAD26)\n", bank, sampleOffset)
						// Dump first few bytes
						fmt.Printf("    ")
						for i := 0; i < 18; i++ {
							fmt.Printf("%02x ", mem.Banks[bank][sampleOffset+uint16(i)])
						}
						fmt.Printf("\n")
					}
				}

				// Binary dump from the currently active bank
				patternBank := mem.PageReadMap[2]  // Use actively mapped bank
				fmt.Printf("\nDumping from currently active Bank %d...\n", patternBank)
				binData := make([]byte, bufferSize)
				baseOffset := startAddr - 0x8000  // Offset within bank
				for i := 0; i < bufferSize; i++ {
					binData[i] = mem.Banks[patternBank][baseOffset + uint16(i)]
				}
				if err := os.WriteFile("pattern_buffer_dump.bin", binData, 0644); err != nil {
					fmt.Printf("Error writing binary dump: %v\n", err)
				} else {
					fmt.Printf("✅ Binary dump: pattern_buffer_dump.bin (%d bytes)\n", bufferSize)
				}

				// Text dump with hex and visual representation
				txtFile, err := os.Create("pattern_buffer_dump.txt")
				if err != nil {
					fmt.Printf("Error creating text dump: %v\n", err)
				} else {
					fmt.Fprintf(txtFile, "=== Pattern Buffer Memory Dump ($AD00-$B000) ===\n")
					fmt.Fprintf(txtFile, "Frame: %d\n", demoFrame)
					fmt.Fprintf(txtFile, "Size: %d bytes (0x%03X)\n\n", bufferSize, bufferSize)

					// Dump in 18-byte chunks (one pattern per chunk)
					for offset := 0; offset < bufferSize; offset += 18 {
						addr := startAddr + uint16(offset)
						fmt.Fprintf(txtFile, "\nPattern at 0x%04X:\n", addr)

						// Dump as hex
						fmt.Fprintf(txtFile, "  Hex: ")
						for i := 0; i < 18 && offset+i < bufferSize; i++ {
							fmt.Fprintf(txtFile, "%02X ", binData[offset+i])
							if i == 8 || i == 17 {
								fmt.Fprintf(txtFile, "\n       ")
							}
						}
						fmt.Fprintf(txtFile, "\n")

						// Visualize first row (first 3 bytes = 2 rows × 1.5 bytes)
						if offset+2 < bufferSize {
							bits := ""
							for byteIdx := 0; byteIdx < 3 && offset+byteIdx < bufferSize; byteIdx++ {
								b := binData[offset+byteIdx]
								for bit := 7; bit >= 0; bit-- {
									if (b & (1 << bit)) != 0 {
										bits += "█"
									} else {
										bits += "·"
									}
								}
							}
							// First 12 bits = first row
							if len(bits) >= 12 {
								fmt.Fprintf(txtFile, "  Row0: %s\n", bits[:12])
							}
						}
					}

					txtFile.Close()
					fmt.Printf("✅ Text dump: pattern_buffer_dump.txt\n")
				}

				// CRITICAL: Extract actual pattern from SCREEN MEMORY
				fmt.Printf("\n=== Extracting Pattern from Screen Memory ===\n")

				// First, find which bank has screen data
				fmt.Printf("Screen configuration: ScreenPageIndex = %d\n", mem.ScreenPageIndex)
				fmt.Printf("Searching all banks for non-zero screen data at offset 0x20...\n")
				for bank := 0; bank < 8; bank++ {
					if mem.Banks[bank][0x20] != 0 || mem.Banks[bank][0x21] != 0 {
						fmt.Printf("  Bank %d: has data (%02x %02x)\n", bank, mem.Banks[bank][0x20], mem.Banks[bank][0x21])
					}
				}
				fmt.Printf("\n")

				screenBank := uint16(mem.ScreenPageIndex)  // Use configured screen bank

				// Search for non-zero data in Bank 5 screen area (first 2KB should have visible squares)
				fmt.Printf("Searching Bank %d for non-zero screen data...\n", screenBank)
				found := false
				for offset := uint16(0); offset < 0x800 && !found; offset += 32 {
					hasData := false
					for i := uint16(0); i < 32; i++ {
						if mem.Banks[screenBank][offset+i] != 0 {
							hasData = true
							break
						}
					}
					if hasData {
						fmt.Printf("  Offset 0x%04x: ", offset)
						for i := uint16(0); i < 16; i++ {
							fmt.Printf("%02x ", mem.Banks[screenBank][offset+i])
						}
						fmt.Printf("\n")
						found = true
					}
				}

				if !found {
					fmt.Printf("  NO NON-ZERO DATA FOUND in Bank %d screen area!\n", screenBank)
					fmt.Printf("  This means the screen hasn't been drawn yet at frame 319.\n")
					fmt.Printf("  The PNG frame buffer must capture at a different time than memory inspection.\n")
				}
				fmt.Printf("\n")

				fmt.Printf("Extracting pattern at correct position (X=32 pixels = 4 bytes offset):\n\n")

				for row := 0; row < 12; row++ {
					// Calculate ZX Spectrum screen address for pixel (32, 8+row)
					y := 8 + row
					xByte := uint16(4)  // X=32 pixels = 4 bytes

					yLow := y & 0x07
					yMid := (y >> 3) & 0x07
					yHi := (y >> 6) & 0x03

					offset := uint16((yHi << 11) | (yLow << 8) | (yMid << 5)) + xByte

					// Read 2 bytes
					byte1 := mem.Banks[screenBank][offset]
					byte2 := mem.Banks[screenBank][offset+1]

					// Visualize
					bits := ""
					for b := 7; b >= 0; b-- {
						bits += string([]rune{[]rune("·█")[(byte1>>b)&1]})
					}
					for b := 7; b >= 4; b-- {
						bits += string([]rune{[]rune("·█")[(byte2>>b)&1]})
					}

					fmt.Printf("Row %2d: %02X %02X  %s\n", row, byte1, byte2, bits)
				}

				fmt.Printf("\n=== CRITICAL: Matching Pattern Buffer to Screen ===\n\n")

				// At frame 337, pattern table says Square 0 uses $AD6E
				// Screen shows: 00 F0 per row (········████)
				// Let's extract what's ACTUALLY at $AD6E in the active bank

				fmt.Printf("Reading pattern at $AD6E (where pattern table points):\n")
				patternAddr := uint16(0xAD6E)
				patternOffset := patternAddr - 0x8000  // Offset in bank

				// Read from the active bank at $8000-$BFFF
				patternBank2 := mem.PageReadMap[2]
				fmt.Printf("Active bank at $8000-$BFFF: %d\n", patternBank2)
				fmt.Printf("Pattern offset in bank: 0x%04X\n\n", patternOffset)

				// Read 20 bytes to see the pattern data
				fmt.Printf("Raw bytes at $AD6E:\n  ")
				for i := 0; i < 20; i++ {
					fmt.Printf("%02X ", mem.Banks[patternBank2][patternOffset+uint16(i)])
				}
				fmt.Printf("\n\n")

				// Now let's try to understand the format
				// If it's 2 bytes per row (like screen), that's 24 bytes for 12 rows
				// If it's 1.5 bytes per row (18 bytes), it's more compact

				fmt.Printf("Attempting decode as 2 bytes per row (24 bytes total):\n")
				for row := 0; row < 12; row++ {
					b1 := mem.Banks[patternBank2][patternOffset+uint16(row*2)]
					b2 := mem.Banks[patternBank2][patternOffset+uint16(row*2+1)]

					bits := ""
					for bit := 7; bit >= 0; bit-- {
						bits += string([]rune{[]rune("·█")[(b1>>bit)&1]})
					}
					for bit := 7; bit >= 4; bit-- {
						bits += string([]rune{[]rune("·█")[(b2>>bit)&1]})
					}

					fmt.Printf("  Row %2d: %02X %02X = %s\n", row, b1, b2, bits)
				}

				fmt.Printf("\nExpected from screen: 00 F0 00 F0 00 F0 ... (········████ repeated)\n")
				fmt.Printf("\n=== Pattern Analysis Complete ===\n\n")
			}

			if demoFrame == 900 || demoFrame == 925 || demoFrame == 950 || demoFrame == 975 || demoFrame == 1000 {
				fmt.Printf("\n\n=== Part 3 Analysis (Frame %d) ===\n", demoFrame)
				fmt.Printf("Looking for 48×48 tile data...\n\n")

				// Part 3 uses different code than Part 2
				// From PC trace: $8877, $882x-$888x range are hot
				// Let's dump memory regions to find tile data

				// Check pattern table at $84C0 (might still be used)
				patternTable := uint16(0x84c0)
				fmt.Printf("Pattern table at $84C0:\n")
				for i := 0; i < 4; i++ {
					addr := patternTable + uint16(i*2)
					lo := mem.Read(addr)
					hi := mem.Read(addr + 1)
					tileAddr := uint16(lo) | (uint16(hi) << 8)
					fmt.Printf("  Tile %d: 0x%04x\n", i, tileAddr)
				}
				fmt.Printf("\n")

				// Check for tile data in Bank 2
				// 48×48 pixels = 2304 pixels = 288 bytes per tile
				// Look for data that matches the screen pattern
				fmt.Printf("Searching for 48×48 tile data (288 bytes)...\n")
				fmt.Printf("Expected pattern size: 48 pixels/8 = 6 bytes per row × 48 rows = 288 bytes\n\n")

				// Read tile 0 data
				tile0Addr := uint16(0xbe06)
				fmt.Printf("Tile 0 at 0x%04X (first 48 bytes):\n", tile0Addr)
				for row := 0; row < 8; row++ {
					fmt.Printf("  ")
					for col := 0; col < 6; col++ {
						b := mem.Read(tile0Addr + uint16(row*6+col))
						fmt.Printf("%02X ", b)
					}
					fmt.Printf("\n")
				}
				fmt.Printf("  ...\n\n")

				// Read tile 1 data
				tile1Addr := uint16(0xc046)
				fmt.Printf("Tile 1 at 0x%04X (first 48 bytes):\n", tile1Addr)
				for row := 0; row < 8; row++ {
					fmt.Printf("  ")
					for col := 0; col < 6; col++ {
						b := mem.Read(tile1Addr + uint16(row*6+col))
						fmt.Printf("%02X ", b)
					}
					fmt.Printf("\n")
				}
				fmt.Printf("  ...\n\n")

				// Visualize first 16 rows of tile 0
				fmt.Printf("Tile 0 visualization (first 16 rows):\n")
				for row := 0; row < 16; row++ {
					fmt.Printf("  ")
					for col := 0; col < 6; col++ {
						b := mem.Read(tile0Addr + uint16(row*6+col))
						for bit := 7; bit >= 0; bit-- {
							if (b>>bit)&1 == 1 {
								fmt.Printf("█")
							} else {
								fmt.Printf("·")
							}
						}
					}
					fmt.Printf("\n")
				}
				fmt.Printf("\n")
			}

			if demoFrame == 400 {
				fmt.Printf("\n\n=== Pattern Table & Data Dump (Frame %d) ===\n", demoFrame)

				// Dump pattern table at $84c0 (64 entries × 2 bytes)
				patternTable := uint16(0x84c0)
				fmt.Printf("Pattern table at 0x%04x:\n", patternTable)

				for i := 0; i < 64; i++ {
					addr := patternTable + uint16(i*2)
					lo := mem.Read(addr)
					hi := mem.Read(addr + 1)
					patternAddr := uint16(lo) | (uint16(hi) << 8)
					fmt.Printf("Square %2d: 0x%04x -> ", i, patternAddr)

					// Dump first 18 bytes of pattern data
					for j := 0; j < 18; j++ {
						fmt.Printf("%02x ", mem.Read(patternAddr+uint16(j)))
					}
					fmt.Printf("\n")
				}

				fmt.Printf("\n=== Pattern Dump Complete ===\n\n")
			}

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
				// Frame diff detection is only enabled when audio is disabled
				// (comparing 200KB buffers adds overhead that can cause audio glitches)
				if shouldSaveFrame(demoFrame) && !*noFrames {
					// Ensure all scanlines are captured
					frameBuffer.CaptureUpToLine(191)

					// All frames cropped to 192×192 centered (active visual area)
					cropX, cropY, cropWidth, cropHeight := 32, 0, 192, 192

					// Only do frame diff when audio is disabled (no timing pressure)
					shouldSave := true
					if *noAudio {
						// Compare with previous frame (compare full buffer for simplicity)
						currentPix := frameBuffer.Image.Pix

						if previousFramePixels == nil || len(previousFramePixels) != len(currentPix) {
							// First frame or size changed, always save
							shouldSave = true
						} else {
							// Compare full frame buffer
							shouldSave = false
							for i := 0; i < len(currentPix); i++ {
								if currentPix[i] != previousFramePixels[i] {
									shouldSave = true
									break
								}
							}
						}

						// Store as previous frame for next comparison
						if shouldSave {
							if previousFramePixels == nil || len(previousFramePixels) != len(currentPix) {
								previousFramePixels = make([]byte, len(currentPix))
							}
							copy(previousFramePixels, currentPix)
						}
					}

					if shouldSave {
						filename = filepath.Join("frames", fmt.Sprintf("frame_%05d.png", demoFrame))

						// Copy raw pixel data (fast) and let workers do cropping + encoding
						currentPix := frameBuffer.Image.Pix
						pixCopy := make([]byte, len(currentPix))
						copy(pixCopy, currentPix)

						// Submit save job to worker pool
						saveJobs <- SaveJob{
							SourcePix:    pixCopy,
							SourceStride: frameBuffer.Image.Stride,
							SourceWidth:  frameBuffer.Image.Bounds().Dx(),
							SourceHeight: frameBuffer.Image.Bounds().Dy(),
							CropX:        cropX,
							CropY:        cropY,
							CropWidth:    cropWidth,
							CropHeight:   cropHeight,
							Filename:     filename,
						}
					}
				}

				// Stop if we've reached the target frame
				if *stopAt > 0 && demoFrame >= *stopAt {
					fmt.Printf("\n\n🎯 Reached target frame %d, stopping\n", demoFrame)
					frameCount = *maxFrames // Break outer loop
					break
				}
			}
			// else: Skip frames 0-1 (tape loading artifacts)
		}
		// else: Skip frames before demo counter exists (tape loading)

		// Print progress every 200 frames
		if frameCount%200 == 0 {
			elapsed := float64(frameCount) / 50.0
			percent := float64(frameCount) / float64(*maxFrames) * 100
			if demoStarted {
				fmt.Printf("\rDemo frame %d (capture %d/%d, %.1f%%) - %.1fs - PC: 0x%04x - IRQs: %d",
					frameNumber, frameCount, *maxFrames, percent, elapsed, cpu.PC, interruptCount)
			} else {
				fmt.Printf("\rFrame %d/%d (%.1f%%) - %.1fs - PC: 0x%04x - IRQs: %d",
					frameCount, *maxFrames, percent, elapsed, cpu.PC, interruptCount)
			}
		}

		// Track AY writes per frame (observe, don't interfere)
		if demoStarted && currentDemoFrame > 0 {
			ayWritesDelta := io.AYWriteCount - lastAYTotal
			if ayWritesDelta > 0 {
				ayStats = append(ayStats, ayFrameStat{
					frame:       currentDemoFrame,
					writesTotal: io.AYWriteCount,
					writesDelta: ayWritesDelta,
				})
			}
			lastAYTotal = io.AYWriteCount
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

	// Wait for all frame save workers to complete
	if !*noFrames {
		close(saveJobs)
		saveWg.Wait()
		fmt.Printf("\n✅ All frames saved to disk\n")
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

	// Dump PC trace for Part 2 algorithm analysis
	if len(pcLog) > 0 {
		type pcCount struct {
			pc    uint16
			count int
		}
		var pcs []pcCount
		for pc, count := range pcLog {
			pcs = append(pcs, pcCount{pc, count})
		}
		sort.Slice(pcs, func(i, j int) bool {
			return pcs[i].count > pcs[j].count
		})

		fmt.Println("\n=== Part 2 PC Trace (Top 100 most executed addresses) ===")
		for i := 0; i < 100 && i < len(pcs); i++ {
			fmt.Printf("0x%04x: %d times\n", pcs[i].pc, pcs[i].count)
		}
	}

	// Generate comprehensive disassembly if traceAllCode was enabled
	if pcExecutionCount != nil && len(pcExecutionCount) > 0 {
		fmt.Printf("\n🔍 Generating comprehensive disassembly from %d unique PC addresses...\n", len(pcExecutionCount))

		// Sort PCs by execution frequency
		var pcStats []pcStat
		for pc, count := range pcExecutionCount {
			pcStats = append(pcStats, pcStat{pc, count})
		}
		sort.Slice(pcStats, func(i, j int) bool {
			return pcStats[i].pc < pcStats[j].pc // Sort by address first
		})

		// Write disassembly to file
		disasmFile, err := os.Create("full_disassembly.asm")
		if err != nil {
			fmt.Printf("Error creating disassembly file: %v\n", err)
		} else {
			defer disasmFile.Close()

			fmt.Fprintf(disasmFile, "; CUBELON Full Disassembly\n")
			fmt.Fprintf(disasmFile, "; Traced frames %d-%d\n", traceAllCodeStart, traceAllCodeEnd)
			fmt.Fprintf(disasmFile, "; Total unique addresses: %d\n", len(pcExecutionCount))
			fmt.Fprintf(disasmFile, "; Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

			// Group addresses into contiguous sections
			var currentSection []pcStat
			for _, stat := range pcStats {
				if len(currentSection) == 0 {
					currentSection = append(currentSection, stat)
				} else {
					prevPC := currentSection[len(currentSection)-1].pc
					// Start new section if gap > 16 bytes
					if stat.pc-prevPC > 16 {
						// Write previous section
						writeDisasmSection(disasmFile, currentSection, mem)
						fmt.Fprintf(disasmFile, "\n")
						currentSection = []pcStat{stat}
					} else {
						currentSection = append(currentSection, stat)
					}
				}
			}
			// Write final section
			if len(currentSection) > 0 {
				writeDisasmSection(disasmFile, currentSection, mem)
			}

			fmt.Printf("✅ Disassembly saved to full_disassembly.asm\n")

			// Also write execution statistics
			statsFile, err := os.Create("execution_stats.txt")
			if err == nil {
				defer statsFile.Close()

				// Sort by execution count
				sort.Slice(pcStats, func(i, j int) bool {
					return pcStats[i].count > pcStats[j].count
				})

				fmt.Fprintf(statsFile, "Top 500 Most Executed Addresses\n")
				fmt.Fprintf(statsFile, "================================\n\n")
				for i := 0; i < 500 && i < len(pcStats); i++ {
					fmt.Fprintf(statsFile, "$%04X: %8d times\n", pcStats[i].pc, pcStats[i].count)
				}
				fmt.Printf("✅ Execution statistics saved to execution_stats.txt\n")
			}
		}
	}

	// Report AY register write statistics
	if len(ayStats) > 0 {
		fmt.Printf("\n\n🎵 === AY Register Write Statistics ===\n")
		fmt.Printf("Total AY register writes: %d\n", io.AYWriteCount)
		fmt.Printf("Total AY register selects: %d\n", io.AYSelectCount)
		fmt.Printf("Total frames with AY activity: %d\n", len(ayStats))
		fmt.Printf("Frames captured: %d\n", frameCount)
		fmt.Printf("Average writes per active frame: %.1f\n", float64(io.AYWriteCount)/float64(len(ayStats)))
		fmt.Printf("Average writes per total frame: %.1f\n", float64(io.AYWriteCount)/float64(frameCount))

		// Find frame with most/least writes
		maxWrites := 0
		minWrites := 999999
		maxFrame := 0
		minFrame := 0
		for _, stat := range ayStats {
			if stat.writesDelta > maxWrites {
				maxWrites = stat.writesDelta
				maxFrame = stat.frame
			}
			if stat.writesDelta < minWrites {
				minWrites = stat.writesDelta
				minFrame = stat.frame
			}
		}
		fmt.Printf("Busiest frame: %d (%d writes)\n", maxFrame, maxWrites)
		fmt.Printf("Quietest frame: %d (%d writes)\n", minFrame, minWrites)

		// Show first 20 frames of AY activity
		fmt.Printf("\nFirst 20 frames with AY writes:\n")
		for i := 0; i < 20 && i < len(ayStats); i++ {
			fmt.Printf("  Frame %4d: %3d writes (total: %d)\n",
				ayStats[i].frame, ayStats[i].writesDelta, ayStats[i].writesTotal)
		}

		// Distribution histogram
		fmt.Printf("\nAY writes per frame distribution:\n")
		writeBuckets := make(map[int]int)
		for _, stat := range ayStats {
			bucket := (stat.writesDelta / 10) * 10 // Round to nearest 10
			writeBuckets[bucket]++
		}
		var buckets []int
		for bucket := range writeBuckets {
			buckets = append(buckets, bucket)
		}
		sort.Ints(buckets)
		for _, bucket := range buckets {
			count := writeBuckets[bucket]
			percent := float64(count) / float64(len(ayStats)) * 100
			fmt.Printf("  %3d-%3d writes: %4d frames (%.1f%%)\n",
				bucket, bucket+9, count, percent)
		}
	}

	// Write audio to WAV file (5 channels: A, B, C, Envelope, AY-Write-Pulse)
	if audio != nil {
		fmt.Printf("\n🎵 Writing 5-channel audio to cubelon_audio.wav...\n")
		if err := audio.WriteWAV("cubelon_audio.wav"); err != nil {
			fmt.Printf("Error writing audio: %v\n", err)
		} else {
			fmt.Printf("✅ Audio saved (%.1fs, %d samples, 5 channels: A, B, C, Envelope, AY-Write-Pulse)\n", actualDuration, audio.Position)
		}
	}
}
