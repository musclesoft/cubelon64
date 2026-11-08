package main

import (
	"encoding/binary"
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
	"cubelon-emu/c64"
	"cubelon-emu/m6502"
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

// writeSparseMusic simulates the ZX music player's reads against a raw 16KB bank
// and writes a compact sparse file containing only the accessed bytes.
// Tick bytes (first byte of each pattern record) are excluded — all patterns are
// fixed at 20 frames and the player does not read the stored tick value.
func writeSparseMusic(bank []byte, path string) {
	const memBase = uint16(0xC000)
	const ticksPerPattern = 20

	patternAddr := func(idx byte) uint16 {
		r := idx
		for i := 0; i < 3; i++ {
			r = (r >> 1) | ((r & 1) << 7)
		}
		return (uint16(r&0x1F)+0xC4)<<8 | uint16(r&0xE0)
	}

	readByte := func(reads map[uint16]struct{}, addr uint16) byte {
		reads[addr] = struct{}{}
		return bank[addr-memBase]
	}

	readLE16 := func(reads map[uint16]struct{}, addr uint16) uint16 {
		lo := readByte(reads, addr)
		hi := readByte(reads, addr+1)
		return uint16(lo) | uint16(hi)<<8
	}

	reads := make(map[uint16]struct{})
	var chPtrs [14]uint16
	seqPtr := uint16(0)

	for {
		if seqPtr >= 0x1B1 {
			break
		}
		b := readByte(reads, memBase+seqPtr)
		seqPtr++
		if b == 0xFF {
			break
		}
		pat := patternAddr(b)
		// skip tick byte at pat+0
		// read all 14 pointer slots (ch12 pointer is read to keep runs contiguous,
		// but ch12 stream bytes are not read — R12 is hardcoded 0x00 in the player)
		for ch := 0; ch < 13; ch++ {
			chPtrs[ch] = readLE16(reads, pat+1+uint16(ch)*2)
		}
		chPtrs[13] = readLE16(reads, pat+1+13*2)
		for tick := 0; tick < ticksPerPattern; tick++ {
			for ch := 0; ch < 12; ch++ {
				readByte(reads, chPtrs[ch])
				chPtrs[ch]++
			}
			readByte(reads, chPtrs[13])
			chPtrs[13]++
		}
	}

	// Sort addresses and pack into contiguous runs
	addrs := make([]uint16, 0, len(reads))
	for a := range reads {
		addrs = append(addrs, a)
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i] < addrs[j] })

	type run struct{ start, end uint16 }
	var runs []run
	if len(addrs) > 0 {
		cur := run{addrs[0], addrs[0]}
		for _, a := range addrs[1:] {
			if a == cur.end+1 {
				cur.end = a
			} else {
				runs = append(runs, cur)
				cur = run{a, a}
			}
		}
		runs = append(runs, cur)
	}

	var buf []byte
	buf = binary.LittleEndian.AppendUint16(buf, uint16(len(runs)))
	for _, r := range runs {
		rlen := r.end - r.start + 1
		buf = binary.LittleEndian.AppendUint16(buf, r.start)
		buf = binary.LittleEndian.AppendUint16(buf, rlen)
		buf = append(buf, bank[r.start-memBase:r.end-memBase+1]...)
	}

	if err := os.WriteFile(path, buf, 0644); err != nil {
		fmt.Printf("⚠️  writeSparseMusic: %v\n", err)
	} else {
		fmt.Printf("💾 Wrote sparse music data -> %s (%d B, %d runs)\n", path, len(buf), len(runs))
	}
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
	c64Mode := flag.Bool("c64", false, "Run C64 emulator instead of ZX Spectrum")
	noFrames := flag.Bool("no-frames", false, "Skip saving frames (faster)")
	validate := flag.Bool("validate", false, "C64 only: run until every assertion has passed, skipping frames; panics on any failure")
	noAudio := flag.Bool("no-audio", false, "Skip audio generation (faster)")
	saveFrames := flag.String("save-frames", "", "Only save specific frames (e.g. '1000' or '1000-2000' or '1000,2000,3000')")
	stopAt := flag.Int("stop-at", 0, "Stop emulation at specific frame number")
	traceScreen := flag.String("trace-screen", "", "Trace screen writes to file for frame range (e.g. '0-1000')")
	traceCode := flag.String("trace-code", "", "Trace all instructions for frame:pcrange (e.g. '900:8580-86FF')")
	traceAllCode := flag.String("trace-all-code", "", "Trace ALL executed code for frame range (e.g. '0-1000') - generates full disassembly")
	extractPattern := flag.String("extract-pattern", "", "Extract pattern data from source for frame range (e.g. '319-370')")
	extractCycle := flag.Bool("extract-cycle", false, "Extract the 11-frame marching cycle as PNGs (cycle_01.png - cycle_11.png)")
	analyzeMotion := flag.Bool("analyze-motion", false, "Analyze and log motion/freeze periods throughout the demo (uses threshold=4)")
	traceSequencer := flag.Bool("trace-sequencer", false, "Trace frame counter (0x8184) accesses to find sequencer logic")
	logAYCommands := flag.String("log-ay-commands", "", "Log AY register writes to file (e.g. 'ay_commands.bin')")

	maxFrames := flag.Int("max-frames", 9650, "Maximum frames to capture (default: 9650 = full CUBELON demo)")
	dumpMemory := flag.Int("dump-memory", 0, "Dump full 64K memory at specified frame to memory_frameNNN.bin")
	traceMusic := flag.String("trace-music", "", "Trace music player calls in frame range (e.g. '200-300')")
	traceDecomp := flag.String("trace-decomp", "", "Trace memory writes to $C000-$CFFF for frame range (e.g. '160-162')")
	dumpMusicBank := flag.Bool("dump-music-bank", false, "Dump music player state: active $C000 bank + per-pattern trace -> music_player_trace.txt + music_bank_frameNNN.bin")

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

	// Handle C64 mode
	if *c64Mode {
		runC64(flag.Args(), *maxFrames, *noFrames || *validate, *saveFrames, *validate)
		return
	}

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

	shouldSaveFrame := func(frame int) bool {
		return !*noFrames && frameSelected(*saveFrames, frame)
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

	// Parse trace-music flag
	var traceMusicStart, traceMusicEnd int
	var musicCallCounts map[uint16]int
	var musicCallFrom map[uint16]map[uint16]int
	var musicTraceFile *os.File
	var musicDetailTrace bool
	if *traceMusic != "" {
		parts := strings.Split(*traceMusic, "-")
		if len(parts) == 2 {
			traceMusicStart, _ = strconv.Atoi(parts[0])
			traceMusicEnd, _ = strconv.Atoi(parts[1])
			musicCallCounts = make(map[uint16]int)
			musicCallFrom = make(map[uint16]map[uint16]int)
			var err error
			musicTraceFile, err = os.Create("music_trace.txt")
			if err == nil {
				fmt.Fprintf(musicTraceFile, "=== Music Player Trace (Frames %d-%d) ===\n\n", traceMusicStart, traceMusicEnd)
			}
			// Enable detailed tracing
			musicDetailTrace = true
			fmt.Printf("🎵 Tracing music player for frames %d-%d\n", traceMusicStart, traceMusicEnd)
		}
	}

	// Music bank dump state
	var musicBankTraceFile *os.File
	var musicBankDumped bool
	var musicBankTraceActive bool   // true while tracing player code instruction-by-instruction
	var musicBankTraceFrames int    // how many pattern-load events to trace in detail
	if *dumpMusicBank {
		var err error
		musicBankTraceFile, err = os.Create("music_player_trace.txt")
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot create music_player_trace.txt: %v\n", err)
			os.Exit(1)
		}
		musicBankTraceFrames = 5 // trace first 5 pattern-load events in full detail
		fmt.Printf("🎵 Music bank dump enabled -> music_player_trace.txt + music_bank_frameNNN.bin\n")
	}

	// Parse trace-decomp flag
	var traceDecompStart, traceDecompEnd int
	if *traceDecomp != "" {
		parts := strings.Split(*traceDecomp, "-")
		if len(parts) == 2 {
			traceDecompStart, _ = strconv.Atoi(parts[0])
			traceDecompEnd, _ = strconv.Atoi(parts[1])
			fmt.Printf("🔧 Tracing decompression writes to $C000-$CFFF for frames %d-%d\n", traceDecompStart, traceDecompEnd)
		}
	}

	// Parse extract-pattern flag
	var extractPatternStart, extractPatternEnd int
	var extractedPatterns []struct {
		Frame       int
		PatternAddr uint16
		Data        []byte
	}
	extractedFrameSet := make(map[int]bool) // Track which frames we've already extracted
	if *extractPattern != "" {
		parts := strings.Split(*extractPattern, "-")
		if len(parts) == 2 {
			extractPatternStart, _ = strconv.Atoi(parts[0])
			extractPatternEnd, _ = strconv.Atoi(parts[1])
			fmt.Printf("📦 Extracting pattern data for frames %d-%d\n", extractPatternStart, extractPatternEnd)
		}
	}

	// Extract cycle flag: extract the 11 specific frames for the marching cycle
	var cycleFrames []int
	var cycleExtracted []struct {
		Frame       int
		PatternAddr uint16
		Data        []byte
	}
	if *extractCycle {
		// The 11 demo frames that make up one complete marching cycle (upright to upright)
		// Hook captures pattern during drawing, which happens one frame before display
		// Screen frames 319-337 show upright-to-upright, so extract from 318-337
		cycleFrames = []int{318, 319, 321, 323, 325, 327, 329, 331, 333, 335, 337}
		fmt.Println("📦 Extracting 11-frame marching cycle")
		fmt.Println("   Capturing at frames: 318, 319, 321, 323, 325, 327, 329, 331, 333, 335, 337")
		fmt.Println("   (Displayed at screen frames 319-337)")
		fmt.Println("   Output: cycle_01.png - cycle_11.png")
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

	// Wire AY write trace callback for music bank dump mode
	if *dumpMusicBank && musicBankTraceFile != nil {
		tf := musicBankTraceFile
		io.AYWriteTrace = func(frame uint32, reg byte, val byte, pc uint16) {
			if frame < 2 || frame > 5 {
				return
			}
			sp := cpu.SP
			var stack [6]uint16
			for i := range stack {
				lo := mem.Read(sp)
				hi := mem.Read(sp + 1)
				stack[i] = uint16(hi)<<8 | uint16(lo)
				sp += 2
			}
			fmt.Fprintf(tf,
				"F%d AY R%02d=0x%02X  PC=$%04X bank=%d  SP=$%04X  stack=[%04X %04X %04X %04X %04X %04X]  HL=$%04X AF=%04X\n",
				frame, reg, val, pc, mem.PageReadMap[3], cpu.SP,
				stack[0], stack[1], stack[2], stack[3], stack[4], stack[5],
				cpu.HL(), uint16(cpu.A)<<8|uint16(cpu.F))
		}
	}

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

	// Create AY command logger if requested
	var ayLog *spectrum.AYLogger
	if *logAYCommands != "" {
		ayLog = spectrum.NewAYLogger()
		ayLog.Enabled = true
		io.AYLog = ayLog
		fmt.Printf("AY command logging enabled (will save to %s)\n", *logAYCommands)
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
	cyclesPerFrame := uint64(70908) // Spectrum 128K: 70908 T-states per frame
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

	// Motion analyzer for detecting motion/freeze periods
	// var motionAnalyzer *MotionAnalyzer
	if *analyzeMotion {
		fmt.Printf("⚠ Motion analysis not available (commented out)\n")
		// const motionThreshold = 4 // Optimal threshold determined by analysis
		// motionAnalyzer = NewMotionAnalyzer(motionThreshold)
		// fmt.Printf("🎬 Motion analysis enabled (threshold: %d frames)\n", motionThreshold)
	}

	// Sequencer tracer for finding control logic
	var sequencerLog *os.File
	var lastTracedFrame = -1
	var memoryDumped = false
	if *traceSequencer {
		var err error
		sequencerLog, err = os.Create("sequencer_trace.log")
		if err != nil {
			fmt.Printf("Error creating sequencer trace: %v\n", err)
			os.Exit(1)
		}
		defer sequencerLog.Close()
		fmt.Fprintf(sequencerLog, "=== Frame Counter Access Trace ===\n")
		fmt.Fprintf(sequencerLog, "Tracking: 0x8184 (bank 2, offset 0x0184)\n\n")
		fmt.Printf("🔍 Sequencer trace enabled -> sequencer_trace.log\n")
	}

	// Track AY register activity
	type ayFrameStat struct {
		frame       int
		writesTotal int
		writesDelta int
	}
	var ayStats []ayFrameStat
	lastAYTotal := 0

	for frameCount < *maxFrames {
		// Update current frame for AY logging
		io.CurrentFrame = uint32(frameCount)

		// Enable decompression tracing if in range
		if traceDecompStart > 0 && currentDemoFrame >= traceDecompStart && currentDemoFrame <= traceDecompEnd {
			mem.TraceWrites = true
			mem.TraceFrame = uint32(currentDemoFrame)
		} else {
			mem.TraceWrites = false
		}

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
			// Update PC for decompression tracing
			if mem.TraceWrites {
				mem.TracePC = cpu.PC
			}


			// ── Music bank dump tracing ──────────────────────────────────────────
			if *dumpMusicBank && musicBankTraceFile != nil {
				pc := cpu.PC

				// Capture the PC + call stack of every AY data write ($BFFD).
				// OUT (C), A = ED 79; port = BC. Detect before Step().
				if musicBankDumped && currentDemoFrame >= 2 && currentDemoFrame <= 5 {
					op0 := mem.Read(pc)
					op1 := mem.Read(pc + 1)
					if op0 == 0xED && op1 == 0x79 { // OUT (C), A
						bcPort := cpu.BC()
						if (bcPort&0xC002) == 0x8000 { // AY data port $BFFD
							ayReg := io.AY.Selected
							sp := cpu.SP
							var callChain [6]uint16
							for i := range callChain {
								lo := mem.Read(sp)
								hi := mem.Read(sp + 1)
								callChain[i] = uint16(hi)<<8 | uint16(lo)
								sp += 2
							}
							fmt.Fprintf(musicBankTraceFile,
								"F%d AY R%02d=0x%02X  PC=$%04X bank=%d  SP=$%04X  stack=[%04X %04X %04X %04X %04X %04X]  HL=$%04X DE=$%04X\n",
								currentDemoFrame, ayReg, cpu.A, pc, mem.PageReadMap[3], cpu.SP,
								callChain[0], callChain[1], callChain[2],
								callChain[3], callChain[4], callChain[5],
								cpu.HL(), cpu.DE())
						}
					}
				}

				// Dump sparse music files at the three key frames
				if pc == 0x7D30 {
					switch currentDemoFrame {
					case 2:
						if _, e2 := os.Stat("music_bank1_intro.bin"); os.IsNotExist(e2) {
							writeSparseMusic(mem.Banks[1][:], "music_bank1_intro.bin")
						}
					case 162:
						if _, e2 := os.Stat("music_bank3_mid.bin"); os.IsNotExist(e2) {
							writeSparseMusic(mem.Banks[3][:], "music_bank3_mid.bin")
						}
					case 802:
						if _, e2 := os.Stat("music_bank1_main.bin"); os.IsNotExist(e2) {
							writeSparseMusic(mem.Banks[1][:], "music_bank1_main.bin")
						}
					}
				}
				if pc == 0x7D30 && !musicBankDumped && currentDemoFrame >= 2 {
					musicBankDumped = true
					activeBank := mem.PageReadMap[3]
					fmt.Fprintf(musicBankTraceFile,
						"=== Music Bank Dump at demo frame %d ===\n"+
							"Active bank at $C000: bank %d\n\n",
						currentDemoFrame, activeBank)

					// Dump the full 16KB bank to a binary file.
					bankFile := fmt.Sprintf("music_bank_frame%d.bin", currentDemoFrame)
					if err := os.WriteFile(bankFile, mem.Banks[activeBank][:], 0644); err == nil {
						fmt.Fprintf(musicBankTraceFile, "Bank data -> %s\n\n", bankFile)
						fmt.Printf("💾 Dumped music bank %d -> %s\n", activeBank, bankFile)
					}
					// Always dump all 8 banks for reference
					for b := 0; b < 8; b++ {
						bfile := fmt.Sprintf("ram_bank%d_frame%d.bin", b, currentDemoFrame)
						os.WriteFile(bfile, mem.Banks[b][:], 0644)
					}
					fmt.Fprintf(musicBankTraceFile, "All RAM banks dumped -> ram_bankN_frame%d.bin\n\n", currentDemoFrame)

					// Dump the channel processor code at $7D44-$7E10 (bank 5 = $4000 region)
					// Offset in bank 5: addr - $4000
					codeBank := mem.PageReadMap[1] // $4000 region
					fmt.Fprintf(musicBankTraceFile, "Channel processor code $7D44-$7E10 (bank %d):\n", codeBank)
					for addr := uint16(0x7D44); addr < 0x7E14; addr++ {
						off := addr - 0x4000
						b := mem.Banks[codeBank][off]
						if (addr-0x7D44)%16 == 0 {
							fmt.Fprintf(musicBankTraceFile, "\n  %04X:", addr)
						}
						fmt.Fprintf(musicBankTraceFile, " %02X", b)
					}
					fmt.Fprintf(musicBankTraceFile, "\n\n")

					// Dump the sequence ($C000-$C1B0) for reference.
					fmt.Fprintf(musicBankTraceFile, "Pattern sequence ($C000+):\n")
					for i := 0; i < 0x1B1; i++ {
						b := mem.Banks[activeBank][i]
						if b == 0xFF {
							fmt.Fprintf(musicBankTraceFile, " FF (end at offset %d)\n", i)
							break
						}
						if i%16 == 0 {
							fmt.Fprintf(musicBankTraceFile, "\n  %04X:", i)
						}
						fmt.Fprintf(musicBankTraceFile, " %02X", b)
					}
					fmt.Fprintf(musicBankTraceFile, "\n\n")
				}

				// Instruction-level trace of $7EC5-$7F9E (player code) for the
				// first musicBankTraceFrames pattern-load events.
				if musicBankTraceActive {
					op0 := mem.Read(pc)
					op1 := mem.Read(pc + 1)
					op2 := mem.Read(pc + 2)
					op3 := mem.Read(pc + 3)
					fmt.Fprintf(musicBankTraceFile,
						"  %04X: %02X %02X %02X %02X  AF:%04X BC:%04X DE:%04X HL:%04X SP:%04X IX:%04X\n",
						pc, op0, op1, op2, op3,
						uint16(cpu.A)<<8|uint16(cpu.F), cpu.BC(), cpu.DE(), cpu.HL(), cpu.SP, cpu.IX)
					// Stop tracing when player returns from $7EC5 scope
					if pc < 0x7EC5 || pc > 0x7FA0 {
						musicBankTraceActive = false
					}
				}

				// Per-frame entry log: always show tick counter and bank at $7EC5.
				if pc == 0x7EC5 && musicBankDumped {
					tickCtr := mem.Read(0x7EC4)
					seqPtrLo := mem.Read(0x7ED8)
					seqPtrHi := mem.Read(0x7ED9)
					seqPtr := uint16(seqPtrLo) | uint16(seqPtrHi)<<8
					activeBank := mem.PageReadMap[3]
					fmt.Fprintf(musicBankTraceFile,
						"\n=== $7EC5 entry frame=%d tick=%d seqPtr=$%04X bank=%d ===\n",
						currentDemoFrame, tickCtr, seqPtr, activeBank)
				}

				// Trace every port OUT that changes bank ($7FFD) inside $7EC5–$7FA0 range.
				if musicBankDumped && pc >= 0x7EC5 && pc <= 0x7FA0 {
					op0 := mem.Read(pc)
					op1 := mem.Read(pc + 1)
					if op0 == 0xED && (op1 == 0x79) { // OUT (C), A
						bcPort := cpu.BC()
						if (bcPort & 0x8002) == 0 { // port $7FFD - bank switch
							fmt.Fprintf(musicBankTraceFile,
								"  BANK SWITCH at $%04X: OUT($%04X)=0x%02X → bank=%d\n",
								pc, bcPort, cpu.A, cpu.A&0x07)
						}
						if (bcPort&0xC002) == 0x8000 { // AY data write
							fmt.Fprintf(musicBankTraceFile,
								"  AY DATA at $%04X: R%d=0x%02X (bank=%d)\n",
								pc, io.AY.Selected, cpu.A, mem.PageReadMap[3])
						}
					}
				}

				// Detailed instruction trace when tick=0 (pattern load frames).
				if pc == 0x7EC5 && musicBankDumped && musicBankTraceFrames > 0 {
					tickCtr := mem.Read(0x7EC4)
					if tickCtr == 0 || tickCtr == 1 { // About to load next pattern
						musicBankTraceFrames--
						musicBankTraceActive = true
					}
				}
			}
			// ── end music bank dump tracing ──────────────────────────────────────

			// Extract Ch13 data at PC=$7DEF (before RRCA at $7DF4)
			if cpu.PC == 0x7DEF {
				hl := cpu.HL()
				val := mem.Read(hl)

				if f, err := os.OpenFile("music_data_ch13.bin", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
					binary.Write(f, binary.LittleEndian, uint32(currentDemoFrame+2))
					binary.Write(f, binary.LittleEndian, val)
					f.Close()
				}
			}

			// Debug: Check what's happening during the gap frames
			if currentDemoFrame >= 8139 && currentDemoFrame <= 8155 {
				// Check if we hit ANY of the channel processing PCs
				if cpu.PC == 0x7D44 {
					fmt.Printf("Frame %d: Hit $7D44 (pattern load)\n", currentDemoFrame)
				} else if cpu.PC >= 0x7D44 && cpu.PC <= 0x7E00 {
					// We're in the channel processing area but not at the start
					if currentDemoFrame == 8139 || currentDemoFrame == 8141 || currentDemoFrame == 8150 || currentDemoFrame == 8153 {
						fmt.Printf("Frame %d: PC=$%04X (in channel area)\n", currentDemoFrame, cpu.PC)
					}
				}
			}

			// Extract channel pointers at every pattern load for Go player
			if cpu.PC == 0x7D44 {
				// This is the first channel processor - pattern just loaded
				// Extract pointers from self-modifying code
				pointers := make([]uint16, 14)

				// Channel 0-12 code addresses
				chAddrs := []uint16{
					0x7D44, 0x7D51, 0x7D5E, 0x7D6B, 0x7D78, 0x7D85, 0x7D92,
					0x7D9F, 0x7DAC, 0x7DB9, 0x7DC6, 0x7DD3, 0x7DE0,
				}

				for ch := 0; ch < 13; ch++ {
					addr := chAddrs[ch]
					opcode := mem.Read(addr)
					if opcode == 0x21 { // LD HL,nnnn
						lo := mem.Read(addr + 1)
						hi := mem.Read(addr + 2)
						pointers[ch] = uint16(lo) | (uint16(hi) << 8)
					}
				}

				// Ch13 pointer extraction (not actually used since we extract at PC=$7DEF)
				// But keep for pointer_log.bin compatibility
				for addr := uint16(0x7DE0); addr < 0x7E00; addr++ {
					if mem.Read(addr) == 0x21 {
						lo := mem.Read(addr + 1)
						hi := mem.Read(addr + 2)
						ptr := uint16(lo) | (uint16(hi) << 8)
						if ptr >= 0xC000 {
							pointers[13] = ptr
							break
						}
					}
				}

				// Append to data log file with VALUES at pointers
				// NOTE: Music player runs 2 frames ahead of when data is output
				// so we add +2 to frame number to match actual AY output timing
				if f, err := os.OpenFile("music_data_log.bin", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
					// Write: [frame:uint32][14 values:byte each]
					binary.Write(f, binary.LittleEndian, uint32(currentDemoFrame+2))
					for _, ptr := range pointers {
						val := byte(0)
						if ptr >= 0xC000 {
							val = mem.Read(ptr)
						}
						binary.Write(f, binary.LittleEndian, val)
					}
					f.Close()
				}
			}

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

			// Music player tracing
			if musicCallCounts != nil && currentDemoFrame >= traceMusicStart && currentDemoFrame <= traceMusicEnd {
				opcode := mem.Read(cpu.PC)
				if opcode == 0xCD { // CALL nn
					target := uint16(mem.Read(cpu.PC+1)) | (uint16(mem.Read(cpu.PC+2)) << 8)
					musicCallCounts[target]++
					if musicCallFrom[target] == nil {
						musicCallFrom[target] = make(map[uint16]int)
					}
					musicCallFrom[target][cpu.PC]++
				}

				// Detailed tracing for AY write routines and channel processing
				if musicDetailTrace && currentDemoFrame >= traceMusicStart && currentDemoFrame <= traceMusicEnd {
					pc := cpu.PC

					// Trace entry to main music player
					if pc == 0x7EC5 {
						fmt.Fprintf(musicTraceFile, "\n[Frame %d] === Music Player Entry $7EC5 ===\n", currentDemoFrame)
						tickCtr := mem.Read(0x7EC4)
						fmt.Fprintf(musicTraceFile, "  Tick counter = %d\n", tickCtr)
					}

					// Trace pattern load (reading sequence)
					if pc == 0x7EDB {
						seqPtr := cpu.HL()
						seqVal := mem.Read(seqPtr)
						fmt.Fprintf(musicTraceFile, "  Reading sequence [$%04X] = $%02X\n", seqPtr, seqVal)
					}

					// Trace pattern address calculation result
					if pc == 0x7F07 {
						fmt.Fprintf(musicTraceFile, "  Pattern data at $%04X, tick count = %d\n", cpu.HL(), cpu.A)
					}

					// Trace channel pointer loads (POP HL instructions)
					if (pc >= 0x7F11 && pc <= 0x7F46 && pc != 0x7F4C) {
						if (pc-0x7F11)%3 == 0 {
							chanIdx := (pc - 0x7F11) / 3
							ptr := cpu.HL()
							fmt.Fprintf(musicTraceFile, "  Channel %d pointer = $%04X\n", chanIdx, ptr)
						}
					}

					// Trace entry to channel processing routine at $7F5C
					if pc == 0x7F5C {
						chanPtr := uint16(mem.Read(0x7902)) | (uint16(mem.Read(0x7903)) << 8)
						fmt.Fprintf(musicTraceFile, "\n  [Channel process @$7F5C] ptr=$%04X\n", chanPtr)
					}

					// Trace command byte read
					if pc == 0x7F5F {
						hl := cpu.HL()
						if hl >= 0xC000 {
							cmd := mem.Read(hl)
							fmt.Fprintf(musicTraceFile, "    Read CMD [$%04X] = $%02X (bit7=%d bit6=%d bit5=%d bits2-0=%d)\n",
								hl, cmd, (cmd>>7)&1, (cmd>>6)&1, (cmd>>5)&1, cmd&0x07)
						}
					}

					// After incrementing HL at $7F60
					if pc == 0x7F61 {
						hl := cpu.HL()
						a := cpu.A
						fmt.Fprintf(musicTraceFile, "    After INC HL: HL=$%04X, A=$%02X\n", hl, a)
					}

					// Check bit 5 result at $7F63
					if pc == 0x7F63 {
						if cpu.F&0x40 != 0 { // Z flag set = bit 5 was 1
							fmt.Fprintf(musicTraceFile, "    Bit 5 SET → END MARKER, jumping to $7F9E\n")
						} else {
							fmt.Fprintf(musicTraceFile, "    Bit 5 CLEAR → continue processing\n")
						}
					}

					// Check bit 7 at $7F67
					if pc == 0x7F67 {
						if cpu.F&0x40 == 0 { // Z flag clear = bit 7 was 1
							fmt.Fprintf(musicTraceFile, "    Bit 7 SET → timing command, jump to $7F69\n")
						} else {
							fmt.Fprintf(musicTraceFile, "    Bit 7 CLEAR → skip to $7F78\n")
						}
					}

					// Read timing bytes at $7F69
					if pc == 0x7F69 {
						hl := cpu.HL()
						c := mem.Read(hl)
						fmt.Fprintf(musicTraceFile, "      Read timing low byte [$%04X] = $%02X\n", hl, c)
					}

					// Check bit 6 at $7F78
					if pc == 0x7F78 {
						hl := cpu.HL()
						val := mem.Read(hl)
						fmt.Fprintf(musicTraceFile, "    Check bit 6 at [$%04X] = $%02X\n", hl, val)
					}

					// Bit 6 check result at $7F7A
					if pc == 0x7F7A {
						if cpu.F&0x40 != 0 { // Z flag set = bit 6 was 1
							fmt.Fprintf(musicTraceFile, "    Bit 6 SET → effect command, jump to $7F8F\n")
						} else {
							fmt.Fprintf(musicTraceFile, "    Bit 6 CLEAR → normal command\n")
						}
					}

					// Normal command path: AND $07 at $7F7C
					if pc == 0x7F7C {
						a := cpu.A
						fmt.Fprintf(musicTraceFile, "      A = $%02X, AND $07 → reg = %d\n", a, a&0x07)
					}

					// About to call $819A
					if pc == 0x7F7E {
						a := cpu.A
						hl := cpu.HL()
						fmt.Fprintf(musicTraceFile, "      CALL $819A with A=$%02X (reg=%d), HL=$%04X\n", a, a&0x07, hl)
					}

					// Read data bytes at $7F81
					if pc == 0x7F81 {
						hl := cpu.HL()
						e := mem.Read(hl)
						fmt.Fprintf(musicTraceFile, "      Read E [$%04X] = $%02X\n", hl, e)
					}

					if pc == 0x7F83 {
						hl := cpu.HL()
						d := mem.Read(hl)
						fmt.Fprintf(musicTraceFile, "      Read D [$%04X] = $%02X, DE=$%04X\n", hl, d, cpu.DE())
					}

					// Call via JP (HL) at $7F87
					if pc == 0x7F87 {
						hl := cpu.HL()
						fmt.Fprintf(musicTraceFile, "      CALL via JP (HL) to $%04X\n", hl)
					}

					// Loop back at $7F8B
					if pc == 0x7F8B {
						hl := cpu.HL()
						fmt.Fprintf(musicTraceFile, "    Loop back to $7F5F with HL=$%04X\n", hl)
					}

					// Trace AY register write routine at $819A entry
					if pc == 0x819A {
						reg := cpu.A & 0x07
						hl := cpu.HL()
						fmt.Fprintf(musicTraceFile, "        → ENTER $819A: reg=%d HL=$%04X\n", reg, hl)
					}

					// Trace what $819A reads
					if pc == 0x819B { // LD C,(HL) at $819A+1
						hl := cpu.HL()
						val := mem.Read(hl)
						fmt.Fprintf(musicTraceFile, "          Read [$%04X] = $%02X → C\n", hl, val)
					}

					if pc == 0x819D { // LD B,(HL) at $819A+3
						hl := cpu.HL()
						val := mem.Read(hl)
						fmt.Fprintf(musicTraceFile, "          Read [$%04X] = $%02X → B, BC=$%04X\n", hl, val, cpu.BC())
					}

					// Trace channel processors at $7D44+ area
					if pc >= 0x7D44 && pc <= 0x7DFF {
						if pc == 0x7D44 || pc == 0x7D51 || pc == 0x7D5E || pc == 0x7D6B {
							chanIdx := (pc - 0x7D44) / 13
							fmt.Fprintf(musicTraceFile, "\n  [Channel %d processor @$%04X]\n", chanIdx, pc)
						}

						// Trace LD HL,addr at channel processor start
						opcode := mem.Read(pc)
						if opcode == 0x21 { // LD HL,nn
							addr := uint16(mem.Read(pc+1)) | (uint16(mem.Read(pc+2)) << 8)
							if addr >= 0xC000 {
								fmt.Fprintf(musicTraceFile, "    LD HL,$%04X (channel data pointer)\n", addr)
							}
						}
					}
				}
			}

			io.CurrentPC = cpu.PC
			cpu.Step()

			// Sequencer state machine tracing
			if sequencerLog != nil && currentDemoFrame >= 2 && currentDemoFrame <= 250 {
				switch cpu.PC {
				case 0x7F69:
					// About to load C from table - log table pointer
					fmt.Fprintf(sequencerLog, "  [Frame %04d] PC=$7F69 LD C,(HL) - Table ptr HL=$%04X, byte=$%02X\n",
						currentDemoFrame, cpu.HL(), mem.Read(cpu.HL()))
				case 0x7F6B:
					// About to load B from table - log current C and next byte
					fmt.Fprintf(sequencerLog, "  [Frame %04d] PC=$7F6B LD B,(HL) - Table ptr HL=$%04X, C=$%02X, byte=$%02X\n",
						currentDemoFrame, cpu.HL(), byte(cpu.BC()&0xFF), mem.Read(cpu.HL()))
				case 0x7F6C:
					// Just loaded BC threshold - log complete value
					fmt.Fprintf(sequencerLog, "  [Frame %04d] PC=$7F6C EX DE,HL - BC threshold=$%04X (%d)\n",
						currentDemoFrame, cpu.BC(), cpu.BC())
				case 0x7F76:
					// Threshold reached, about to restore HL from DE
					counterLow := mem.Banks[mem.PageReadMap[2]][0x0184]
					counterHigh := mem.Banks[mem.PageReadMap[2]][0x0185]
					demoCounter := uint16(counterLow) | (uint16(counterHigh) << 8)
					fmt.Fprintf(sequencerLog, "  [Frame %04d] PC=$7F76 EX DE,HL - Threshold reached! Counter=$%04X, BC=$%04X, DE=$%04X\n",
						currentDemoFrame, demoCounter, cpu.BC(), cpu.DE())
				}
			}

			// Hook into drawing routine to capture pattern address from HL register
			// PC 0x85A8 is "LD SP,HL" where HL contains the pattern address being drawn
			if *extractCycle && len(cycleFrames) > 0 && cpu.PC == 0x85A8 {
				for _, targetFrame := range cycleFrames {
					if currentDemoFrame == targetFrame && !extractedFrameSet[currentDemoFrame] {
						extractedFrameSet[currentDemoFrame] = true

						// HL register contains the pattern address at drawing time
						patternAddr := cpu.HL()

						// Read 72 bytes of pattern data
						patternData := make([]byte, 72)
						for i := 0; i < 72; i++ {
							patternData[i] = mem.Read(patternAddr + uint16(i))
						}

						cycleExtracted = append(cycleExtracted, struct {
							Frame       int
							PatternAddr uint16
							Data        []byte
						}{
							Frame:       currentDemoFrame,
							PatternAddr: patternAddr,
							Data:        patternData,
						})
						break
					}
				}
			}

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
				// Visible area: scanlines 64-255 (192 lines), starting at T-state 14592
				// Each scanline is 228 T-states (128K)
				frameCycle := cpu.Cycles - frameStartCycles
				if frameCycle >= 14592 && frameCycle < 58368 {
					currentScanline := int((frameCycle - 14592) / 228)
					if currentScanline < 192 && currentScanline > frameBuffer.LastCapturedLine {
						frameBuffer.CaptureUpToLine(currentScanline)
					}
				}
			}
		}

		// Extract pattern data from source at END of frame (before cycle reset)
		// This ensures we read the updated pattern table
		counterRegion := uint16(2)
		counterOffset := uint16(0x0184)
		if mem.Written[mem.PageReadMap[counterRegion]][counterOffset] {
			counterLow := mem.Banks[mem.PageReadMap[counterRegion]][counterOffset]
			counterHigh := mem.Banks[mem.PageReadMap[counterRegion]][counterOffset+1]
			demoFrame := int(counterLow) | (int(counterHigh) << 8)

			if extractPatternStart > 0 && demoFrame >= extractPatternStart && demoFrame <= extractPatternEnd {
				if !extractedFrameSet[demoFrame] {
					extractedFrameSet[currentDemoFrame] = true

					patternTable := uint16(0x84c0)

					// Part 2 uses square 3 for later frames (after transition at ~frame 370)
					// Earlier frames (318-336) used square 1
					squareIdx := 3
					if demoFrame < 370 {
						squareIdx = 1
					}
					addr := patternTable + uint16(squareIdx*2)
					lo := mem.Read(addr)
					hi := mem.Read(addr + 1)
					patternAddrEnd := uint16(lo) | (uint16(hi) << 8)

					// Pattern table points to END of 72-byte pattern, subtract 72 to get start
					patternAddr := patternAddrEnd - 72

					// Read 72 bytes of pattern data from source
					patternData := make([]byte, 72)
					for i := 0; i < 72; i++ {
						patternData[i] = mem.Read(patternAddr + uint16(i))
					}

					extractedPatterns = append(extractedPatterns, struct {
						Frame       int
						PatternAddr uint16
						Data        []byte
					}{
						Frame:       currentDemoFrame,
						PatternAddr: patternAddr,
						Data:        patternData,
					})
				}
			}

			// Extract cycle frames if requested
			if *extractCycle && len(cycleFrames) > 0 {
				// Check if current demo frame is in our cycle list
				for _, targetFrame := range cycleFrames {
					if currentDemoFrame == targetFrame && !extractedFrameSet[currentDemoFrame] {
						extractedFrameSet[currentDemoFrame] = true

						patternTable := uint16(0x84c0)
						squareIdx := 1 // All cycle frames are before frame 370, so use square 1
						addr := patternTable + uint16(squareIdx*2)
						lo := mem.Read(addr)
						hi := mem.Read(addr + 1)
						patternAddrEnd := uint16(lo) | (uint16(hi) << 8)
						patternAddr := patternAddrEnd - 72

						// Read 72 bytes of pattern data
						patternData := make([]byte, 72)
						for i := 0; i < 72; i++ {
							patternData[i] = mem.Read(patternAddr + uint16(i))
						}

						cycleExtracted = append(cycleExtracted, struct {
							Frame       int
							PatternAddr uint16
							Data        []byte
						}{
							Frame:       currentDemoFrame,
							PatternAddr: patternAddr,
							Data:        patternData,
						})
						break
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
		var frameNumber int

		// Check if demo counter exists and is valid
		if mem.Written[mem.PageReadMap[counterRegion]][counterOffset] {
			counterLow := mem.Banks[mem.PageReadMap[counterRegion]][counterOffset]
			counterHigh := mem.Banks[mem.PageReadMap[counterRegion]][counterOffset+1]
			demoFrame := int(counterLow) | (int(counterHigh) << 8)

			// Log frame counter changes for sequencer tracing
			if sequencerLog != nil && demoFrame != lastTracedFrame && demoFrame >= 2 && demoFrame <= 300 {
				fmt.Fprintf(sequencerLog, "Frame %04d: counter=$%04X  PC=$%04X  SP=$%04X\n",
					demoFrame, demoFrame, cpu.PC, cpu.SP)
				lastTracedFrame = demoFrame

				// Dump memory at frame 200 to see sequencer code and table
				if demoFrame == 200 && !memoryDumped {
					memoryDumped = true
					fmt.Fprintf(sequencerLog, "\n=== Memory Dump at Frame 200 ===\n")
					fmt.Fprintf(sequencerLog, "Sequencer code region 0x7F00-0x7F80:\n\n")

					// Dump sequencer code
					for addr := uint16(0x7F00); addr < 0x7F80; addr += 16 {
						fmt.Fprintf(sequencerLog, "%04X: ", addr)
						for i := uint16(0); i < 16 && addr+i < 0x7F80; i++ {
							val := mem.Read(addr + i)
							fmt.Fprintf(sequencerLog, "%02X ", val)
						}
						fmt.Fprintf(sequencerLog, "\n")
					}

					// Dump table pointer area
					fmt.Fprintf(sequencerLog, "\nTable pointer at 0x7900-0x7910:\n\n")
					for addr := uint16(0x7900); addr < 0x7910; addr += 16 {
						fmt.Fprintf(sequencerLog, "%04X: ", addr)
						for i := uint16(0); i < 16 && addr+i < 0x7910; i++ {
							val := mem.Read(addr + i)
							fmt.Fprintf(sequencerLog, "%02X ", val)
						}
						fmt.Fprintf(sequencerLog, "\n")
					}

					// Read table pointer
					tableAddrLo := mem.Read(0x7902)
					tableAddrHi := mem.Read(0x7903)
					tableAddr := uint16(tableAddrLo) | (uint16(tableAddrHi) << 8)

					fmt.Fprintf(sequencerLog, "\nTable pointer at $7902 = $%04X\n", tableAddr)
					fmt.Fprintf(sequencerLog, "\nSequence table at 0x%04X:\n\n", tableAddr)

					// Dump first 128 bytes of table
					endAddr := tableAddr + 128
					if endAddr < tableAddr { // Handle wraparound
						endAddr = 0xFFFF
					}
					for addr := tableAddr; addr < endAddr; addr += 16 {
						fmt.Fprintf(sequencerLog, "%04X: ", addr)
						for i := uint16(0); i < 16 && addr+i < endAddr; i++ {
							val := mem.Read(addr + i)
							fmt.Fprintf(sequencerLog, "%02X ", val)
						}
						fmt.Fprintf(sequencerLog, "\n")
					}

					fmt.Printf("💾 Memory dump saved to sequencer_trace.log at frame 200\n")
				}
			}
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

			// The demo's screen updates start at internal counter 2; 0 and 1 are the untouched initial screen and tape loading leftovers
			if demoFrame >= 2 && demoFrame < 100000 {
				// Auto-detect when demo counter starts
				if !demoStarted {
					demoStarted = true
					demoFrameOffset = frameCount - demoFrame
					fmt.Printf("\n🎬 Demo started: capture frame %d = demo frame %d\n", frameCount, demoFrame)
				}

				// Save frame using demo's internal frame counter
				frameNumber = demoFrame

				// Frame comparison for motion analysis (runs even with --no-frames)
				var frameChanged bool
				if !*noFrames {
					// Ensure all scanlines are captured
					frameBuffer.CaptureUpToLine(191)

					currentPix := frameBuffer.Image.Pix
					frameChanged = true

					if previousFramePixels != nil && len(previousFramePixels) == len(currentPix) {
						// Compare full frame buffer
						frameChanged = false
						for i := 0; i < len(currentPix); i++ {
							if currentPix[i] != previousFramePixels[i] {
								frameChanged = true
								break
							}
						}
					}

					// Store as previous frame for next comparison
					if frameChanged {
						if previousFramePixels == nil || len(previousFramePixels) != len(currentPix) {
							previousFramePixels = make([]byte, len(currentPix))
						}
						copy(previousFramePixels, currentPix)
					}

					// Track motion/freeze periods
					// if motionAnalyzer != nil {
					// 	// motionAnalyzer.OnFrame(demoFrame, frameChanged)
					// }
				}

				// Check if we should save this frame
				if shouldSaveFrame(demoFrame) && !*noFrames && frameChanged {
					// All frames cropped to 192×192 centered (active visual area)
					cropX, cropY, cropWidth, cropHeight := 32, 0, 192, 192
					currentPix := frameBuffer.Image.Pix
					filename = filepath.Join("frames", fmt.Sprintf("frame_%05d_zx.png", demoFrame))

					// Copy raw pixel data (fast) and let workers do cropping + encoding
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

				// Stop if we've reached the target frame
				if *stopAt > 0 && demoFrame >= *stopAt {
					fmt.Printf("\n\n🎯 Reached target frame %d, stopping\n", demoFrame)
					frameCount = *maxFrames // Break outer loop
					break
				}
			}
			// else: Skip counter 0-1 (initial screen, tape loading leftovers)
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

		// Dump memory if requested
		if *dumpMemory > 0 && frameCount == *dumpMemory {
			filename := fmt.Sprintf("memory_frame%d.bin", frameCount)
			f, err := os.Create(filename)
			if err == nil {
				for addr := 0; addr < 65536; addr++ {
					f.Write([]byte{mem.Read(uint16(addr))})
				}
				f.Close()
				fmt.Printf("\n💾 Dumped full 64K memory to %s\n", filename)
			}
		}
	}

	// Wait for all frame save workers to complete
	if !*noFrames {
		close(saveJobs)
		saveWg.Wait()
		fmt.Printf("\n✅ All frames saved to disk\n")
	}

	// Generate music trace report
	if musicTraceFile != nil {
		defer musicTraceFile.Close()

		totalFrames := traceMusicEnd - traceMusicStart + 1
		fmt.Fprintf(musicTraceFile, "Traced frames: %d-%d (%d total frames)\n\n", traceMusicStart, traceMusicEnd, totalFrames)

		// Sort calls by frequency
		type callStat struct {
			addr  uint16
			count int
		}
		calls := []callStat{}
		for addr, count := range musicCallCounts {
			calls = append(calls, callStat{addr, count})
		}
		sort.Slice(calls, func(i, j int) bool {
			return calls[i].count > calls[j].count
		})

		fmt.Fprintf(musicTraceFile, "=== Top 50 Called Routines ===\n\n")
		for i := 0; i < 50 && i < len(calls); i++ {
			perFrame := float64(calls[i].count) / float64(totalFrames)
			fmt.Fprintf(musicTraceFile, "$%04X: %5d calls (%.2f per frame)\n", calls[i].addr, calls[i].count, perFrame)

			if perFrame >= 0.9 && perFrame <= 1.1 {
				fmt.Fprintf(musicTraceFile, "  ← Called from: ")
				callers := []uint16{}
				for caller := range musicCallFrom[calls[i].addr] {
					callers = append(callers, caller)
				}
				sort.Slice(callers, func(i, j int) bool { return callers[i] < callers[j] })
				for _, caller := range callers {
					fmt.Fprintf(musicTraceFile, "$%04X ", caller)
				}
				fmt.Fprintf(musicTraceFile, "\n")
			}
		}

		fmt.Fprintf(musicTraceFile, "\n=== Music Player Candidates (called ~1x per frame) ===\n\n")
		for _, call := range calls {
			perFrame := float64(call.count) / float64(totalFrames)
			if perFrame >= 0.9 && perFrame <= 1.1 {
				fmt.Fprintf(musicTraceFile, "$%04X: %d calls (%.2f per frame)\n", call.addr, call.count, perFrame)
			}
		}

		fmt.Printf("\n🎵 Music trace saved to music_trace.txt\n")
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

	// Generate motion/freeze timeline
	if false { // motionAnalyzer != nil {
		fmt.Printf("\n🔍 Generating motion timeline (demoStarted=%v)...\n", demoStarted)

		finalFrame := frameCount - 1
		if demoStarted {
			// Read final demo frame counter
			counterRegion := uint16(2)
			counterOffset := uint16(0x0184)
			counterLow := mem.Banks[mem.PageReadMap[counterRegion]][counterOffset]
			counterHigh := mem.Banks[mem.PageReadMap[counterRegion]][counterOffset+1]
			finalFrame = int(counterLow) | (int(counterHigh) << 8)
			fmt.Printf("   Final demo frame: %d\n", finalFrame)
		}

		// motionAnalyzer.Finalize(finalFrame)
		// motionAnalyzer.PrintTimeline()
		// motionAnalyzer.VisualizeTimeline(120)

		// // Export script
		// if err := motionAnalyzer.ExportScript("motion_script.txt"); err != nil {
		// 	fmt.Printf("Warning: failed to export motion script: %v\n", err)
		// } else {
		// 	fmt.Printf("\n📝 Motion script exported to motion_script.txt\n")
		// }
	}

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

	// Save extracted pattern data
	if len(extractedPatterns) > 0 {
		fmt.Printf("\n📦 Decoding %d patterns (frames %d-%d)...\n",
			len(extractedPatterns), extractPatternStart, extractPatternEnd)

		// Decode pattern from 72-byte source to screen representation
		// Part 2 patterns: 72 bytes = 24 rows × 3 bytes
		// Interleaved byte order: even rows [b0,b1,b2], odd rows [b2,b1,b0]
		// Applies horizontal wrapping and centering to keep square together
		decodePattern := func(data []byte, frameNum int) [][]byte {
			// First decode the raw pattern
			rawPixels := make([][]byte, 24) // 24 rows
			for row := 0; row < 24; row++ {
				rowBytes := make([]byte, 3) // 3 bytes per row = 24 pixels
				if row%2 == 0 {
					// Even row: forward order
					rowBytes[0] = data[row*3+0]
					rowBytes[1] = data[row*3+1]
					rowBytes[2] = data[row*3+2]
				} else {
					// Odd row: reversed order
					rowBytes[0] = data[row*3+2]
					rowBytes[1] = data[row*3+1]
					rowBytes[2] = data[row*3+0]
				}
				rawPixels[row] = rowBytes
			}

			// Convert to pixel array for easier manipulation
			pixelArray := make([][]bool, 24)
			for row := 0; row < 24; row++ {
				pixelArray[row] = make([]bool, 24)
				for byteIdx := 0; byteIdx < 3; byteIdx++ {
					b := rawPixels[row][byteIdx]
					for bit := 0; bit < 8; bit++ {
						x := byteIdx*8 + (7 - bit)
						if x < 24 {
							pixelArray[row][x] = (b>>bit)&1 == 1
						}
					}
				}
			}

			// SAVE RAW PATTERN (before centering/wrapping) for analysis
			if *saveFrames != "" && frameNum >= 318 && frameNum <= 336 {
				rawFilename := fmt.Sprintf("frames/raw_frame%03d.txt", frameNum)
				rawFile, err := os.Create(rawFilename)
				if err == nil {
					fmt.Fprintf(rawFile, "=== RAW Frame %d (before centering) ===\n", frameNum)
					for row := 0; row < 24; row++ {
						for x := 0; x < 24; x++ {
							if pixelArray[row][x] {
								rawFile.WriteString("█")
							} else {
								rawFile.WriteString("·")
							}
						}
						rawFile.WriteString("\n")
					}
					rawFile.Close()
				}
			}

			// Find bounding box and check for split pattern
			minX, maxX := 24, -1
			minY, maxY := 24, -1
			hasContent := false

			for row := 0; row < 24; row++ {
				for x := 0; x < 24; x++ {
					if pixelArray[row][x] {
						hasContent = true
						if row < minY {
							minY = row
						}
						if row > maxY {
							maxY = row
						}
						if x < minX {
							minX = x
						}
						if x > maxX {
							maxX = x
						}
					}
				}
			}

			if !hasContent {
				return rawPixels
			}

			// Calculate center of mass to detect and fix wrapped patterns
			// Find horizontal center of mass
			totalPixels := 0
			weightedSum := 0

			for row := 0; row < 24; row++ {
				for x := 0; x < 24; x++ {
					if pixelArray[row][x] {
						totalPixels++
						weightedSum += x
					}
				}
			}

			// Find the best roll offset to minimize gaps (maximize compactness)
			// This gives smooth, monotone X offset across animation steps
			rollOffset := 0
			if totalPixels > 0 {
				// For each possible roll offset, calculate the "compactness"
				// (width of bounding box after rolling)
				bestRollOffset := 0
				minWidth := 24

				for testRoll := 0; testRoll < 24; testRoll++ {
					// Calculate bounding box for this roll offset
					testMinX, testMaxX := 24, -1
					for row := 0; row < 24; row++ {
						for x := 0; x < 24; x++ {
							srcX := (x - testRoll + 24) % 24
							if pixelArray[row][srcX] {
								if x < testMinX {
									testMinX = x
								}
								if x > testMaxX {
									testMaxX = x
								}
							}
						}
					}

					if testMinX <= testMaxX {
						testWidth := testMaxX - testMinX + 1
						if testWidth < minWidth {
							minWidth = testWidth
							bestRollOffset = testRoll
						}
					}
				}

				rollOffset = bestRollOffset
			}

			// Apply horizontal roll if needed
			if rollOffset > 0 {
				rolledArray := make([][]bool, 24)
				for row := 0; row < 24; row++ {
					rolledArray[row] = make([]bool, 24)
					for x := 0; x < 24; x++ {
						srcX := (x - rollOffset + 24) % 24
						rolledArray[row][x] = pixelArray[row][srcX]
					}
				}
				pixelArray = rolledArray

				// Recalculate bounding box after rolling
				minX, maxX = 24, -1
				minY, maxY = 24, -1
				for row := 0; row < 24; row++ {
					for x := 0; x < 24; x++ {
						if pixelArray[row][x] {
							if row < minY {
								minY = row
							}
							if row > maxY {
								maxY = row
							}
							if x < minX {
								minX = x
							}
							if x > maxX {
								maxX = x
							}
						}
					}
				}
			}

			// Center the pattern
			contentWidth := maxX - minX + 1
			contentHeight := maxY - minY + 1
			shiftX := (24 - contentWidth) / 2
			shiftY := (24 - contentHeight) / 2

			centeredArray := make([][]bool, 24)
			for row := 0; row < 24; row++ {
				centeredArray[row] = make([]bool, 24)
			}

			for srcRow := minY; srcRow <= maxY; srcRow++ {
				dstRow := srcRow - minY + shiftY
				if dstRow >= 0 && dstRow < 24 {
					for srcX := minX; srcX <= maxX; srcX++ {
						if pixelArray[srcRow][srcX] {
							dstX := srcX - minX + shiftX
							if dstX >= 0 && dstX < 24 {
								centeredArray[dstRow][dstX] = true
							}
						}
					}
				}
			}

			// Convert back to byte array
			centeredPixels := make([][]byte, 24)
			for row := 0; row < 24; row++ {
				centeredPixels[row] = make([]byte, 3)
				for x := 0; x < 24; x++ {
					if centeredArray[row][x] {
						byteIdx := x / 8
						bit := 7 - (x % 8)
						centeredPixels[row][byteIdx] |= (1 << bit)
					}
				}
			}

			return centeredPixels
		}

		// Track unique patterns and reuse
		uniquePatterns := make(map[uint16]int) // addr -> first frame number
		var patternReuseInfo []string

		for _, p := range extractedPatterns {
			if firstFrame, exists := uniquePatterns[p.PatternAddr]; exists {
				patternReuseInfo = append(patternReuseInfo, fmt.Sprintf("Frame %d: REUSED from frame %d (Source: $%04X)", p.Frame, firstFrame, p.PatternAddr))
			} else {
				uniquePatterns[p.PatternAddr] = p.Frame
				patternReuseInfo = append(patternReuseInfo, fmt.Sprintf("Frame %d: NEW pattern (Source: $%04X)", p.Frame, p.PatternAddr))
			}
		}

		// Save combined animation file
		animFile := filepath.Join("frames", fmt.Sprintf("animation_%d-%d.txt", extractPatternStart, extractPatternEnd))
		var animBuf strings.Builder
		animBuf.WriteString(fmt.Sprintf("CUBELON Part 2 Animation Cycle (Demo Frames %d-%d)\n", extractPatternStart, extractPatternEnd))
		animBuf.WriteString(fmt.Sprintf("Total frames: %d (24×24 pixels each)\n", len(extractedPatterns)))
		animBuf.WriteString(fmt.Sprintf("Unique patterns: %d\n", len(uniquePatterns)))
		animBuf.WriteString(fmt.Sprintf("Decoded from 72-byte source patterns (algorithm-based, not screen extraction)\n\n"))

		animBuf.WriteString("Pattern Reuse Summary:\n")
		animBuf.WriteString(strings.Repeat("=", 60) + "\n")
		for _, info := range patternReuseInfo {
			animBuf.WriteString(info + "\n")
		}
		animBuf.WriteString("\n\n")

		// Only show NEW patterns, skip reused ones
		prevAddr := uint16(0)
		for _, p := range extractedPatterns {
			if p.PatternAddr == prevAddr {
				continue // Skip duplicate pattern
			}
			prevAddr = p.PatternAddr

			pixels := decodePattern(p.Data, p.Frame)
			animBuf.WriteString(fmt.Sprintf("=== Frame %d (Source: $%04X) ===\n", p.Frame, p.PatternAddr))
			for row := 0; row < 24; row++ {
				for byteIdx := 0; byteIdx < 3; byteIdx++ {
					b := pixels[row][byteIdx]
					for bit := 7; bit >= 0; bit-- {
						if (b>>bit)&1 == 1 {
							animBuf.WriteString("█")
						} else {
							animBuf.WriteString("·")
						}
					}
				}
				animBuf.WriteString("\n")
			}
			animBuf.WriteString("\n")
		}

		if err := os.WriteFile(animFile, []byte(animBuf.String()), 0644); err != nil {
			fmt.Printf("Error writing animation file: %v\n", err)
		} else {
			fmt.Printf("✅ Decoded %d patterns (source-based algorithm)\n", len(extractedPatterns))
			fmt.Printf("   Animation file: %s\n", animFile)
		}
	}

	// Save cycle extraction as PNGs
	if *extractCycle && len(cycleExtracted) > 0 {
		fmt.Printf("\n📦 Saving 11-frame marching cycle as PNGs...\n")

		// Sort by frame number
		sort.Slice(cycleExtracted, func(i, j int) bool {
			return cycleExtracted[i].Frame < cycleExtracted[j].Frame
		})

		// Build frame index map (frame 336 appears twice, so we need to handle duplicates)
		frameIndex := make(map[int]int) // maps demo frame -> output index
		currentIdx := 0
		for _, targetFrame := range cycleFrames {
			if _, exists := frameIndex[targetFrame]; !exists {
				frameIndex[targetFrame] = currentIdx
			}
			currentIdx++
		}

		// Decode and save each pattern
		saved := make(map[int]bool) // Track which we've saved to avoid re-saving frame 336
		cycleNum := 1
		for _, targetFrame := range cycleFrames {
			if saved[targetFrame] {
				// For frame 336 appearing twice, just copy the file
				srcFile := fmt.Sprintf("frames/cycle_%02d.png", frameIndex[targetFrame]+1)
				dstFile := fmt.Sprintf("frames/cycle_%02d.png", cycleNum)
				if srcData, err := os.ReadFile(srcFile); err == nil {
					os.WriteFile(dstFile, srcData, 0644)
				}
				cycleNum++
				continue
			}

			// Find this frame in extracted data
			var patternData []byte
			for _, extracted := range cycleExtracted {
				if extracted.Frame == targetFrame {
					patternData = extracted.Data
					break
				}
			}

			if patternData == nil {
				fmt.Printf("   Frame %d: NOT FOUND\n", targetFrame)
				cycleNum++
				continue
			}

			// Decode 72-byte pattern to 24x24 bitmap with wrapping correction
			// Pattern storage: even rows [b0,b1,b2], odd rows [b2,b1,b0]
			pixelArray := make([][]bool, 24)
			for y := 0; y < 24; y++ {
				pixelArray[y] = make([]bool, 24)
				byteOffset := y * 3

				// Apply byte reversal for odd rows
				var rowBytes [3]byte
				if y%2 == 0 {
					// Even row: forward order
					rowBytes[0] = patternData[byteOffset+0]
					rowBytes[1] = patternData[byteOffset+1]
					rowBytes[2] = patternData[byteOffset+2]
				} else {
					// Odd row: reversed order
					rowBytes[0] = patternData[byteOffset+2]
					rowBytes[1] = patternData[byteOffset+1]
					rowBytes[2] = patternData[byteOffset+0]
				}

				// Decode bits from the (potentially reordered) row bytes
				for x := 0; x < 24; x++ {
					byteIdx := x / 8
					bitPos := uint(7 - (x % 8))
					if (rowBytes[byteIdx] & (1 << bitPos)) != 0 {
						pixelArray[y][x] = true
					}
				}
			}

			// Convert to bitmap for PNG rendering
			bitmap := make([][]byte, 24)
			for y := 0; y < 24; y++ {
				bitmap[y] = make([]byte, 24)
				for x := 0; x < 24; x++ {
					if pixelArray[y][x] {
						bitmap[y][x] = 1
					}
				}
			}

			// Save as PNG
			scale := 8
			img := image.NewGray(image.Rect(0, 0, 24*scale, 24*scale))
			for y := 0; y < 24; y++ {
				for x := 0; x < 24; x++ {
					val := uint8(0)
					if bitmap[y][x] == 0 {
						val = 255 // white for empty
					}
					for dy := 0; dy < scale; dy++ {
						for dx := 0; dx < scale; dx++ {
							img.SetGray(x*scale+dx, y*scale+dy, color.Gray{Y: val})
						}
					}
				}
			}

			filename := fmt.Sprintf("frames/cycle_%02d.png", cycleNum)
			f, err := os.Create(filename)
			if err != nil {
				fmt.Printf("   Frame %d -> cycle_%02d.png: ERROR creating file\n", targetFrame, cycleNum)
				cycleNum++
				continue
			}
			if err := png.Encode(f, img); err != nil {
				fmt.Printf("   Frame %d -> cycle_%02d.png: ERROR encoding\n", targetFrame, cycleNum)
			} else {
				fmt.Printf("   Frame %d -> cycle_%02d.png ✓\n", targetFrame, cycleNum)
			}
			f.Close()
			saved[targetFrame] = true
			cycleNum++
		}
		fmt.Printf("✅ Saved %d cycle PNGs to frames/\n", len(cycleFrames))
	}

	// Write audio to WAV file (6 channels: A, B, C, Envelope, AY-Write-Pulse, A-waveform)
	if audio != nil {
		fmt.Printf("\n🎵 Writing 6-channel audio to cubelon_audio.wav...\n")
		if err := audio.WriteWAV("cubelon_audio.wav"); err != nil {
			fmt.Printf("Error writing audio: %v\n", err)
		} else {
			fmt.Printf("✅ Audio saved (%.1fs, %d samples, 6 channels: A, B, C, Envelope, AY-Write-Pulse, A-waveform)\n", actualDuration, audio.Position)
		}
	}

	// Write AY command log if requested
	if ayLog != nil && *logAYCommands != "" {
		fmt.Printf("\n📝 Writing AY command log to %s...\n", *logAYCommands)
		if err := ayLog.SaveBinary(*logAYCommands); err != nil {
			fmt.Printf("Error writing AY log: %v\n", err)
		} else {
			fmt.Printf("✅ AY log saved (%d commands)\n", len(ayLog.Commands))
			// Also save text version for human inspection
			textFile := *logAYCommands + ".txt"
			if err := ayLog.SaveText(textFile); err != nil {
				fmt.Printf("Warning: could not save text log: %v\n", err)
			} else {
				fmt.Printf("✅ Text log saved to %s\n", textFile)
			}
		}
	}
}

// runC64 executes C64 emulation
func frameSelected(spec string, frame int) bool {
	if spec == "" {
		return true
	}

	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) == 2 {
				start, _ := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
				end, _ := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
				if frame >= start && frame <= end {
					return true
				}
			}
		} else {
			targetFrame, _ := strconv.Atoi(part)
			if frame == targetFrame {
				return true
			}
		}
	}
	return false
}

func runC64(args []string, maxFrames int, noFrames bool, saveFrames string, validate bool) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: PRG file path required in C64 mode\n")
		fmt.Fprintf(os.Stderr, "Usage: run_cubelon --c64 <prg-file>\n")
		os.Exit(1)
	}

	prgFile := args[0]
	fmt.Printf("🎮 C64 mode - loading PRG: %s\n", prgFile)

	// Load PRG file
	prgData, err := os.ReadFile(prgFile)
	if err != nil {
		fmt.Printf("Error loading PRG: %v\n", err)
		os.Exit(1)
	}

	metadata, err := LoadMetadata(prgFile)
	if err != nil {
		panic(fmt.Sprintf("loading metadata: %v", err))
	}
	if validate && metadata == nil {
		panic("no metadata found: " + prgFile + ".meta.json")
	}

	// Create C64 memory and load PRG
	mem := c64.NewMemory()
	mem.LoadPRG(prgData)

	// Create VIC-II chip
	vic := c64.NewVIC(mem)
	mem.VIC = vic

	// Create 6502 CPU
	cpu := m6502.New(mem)

	// Extract load address from PRG
	if len(prgData) < 2 {
		fmt.Printf("Error: PRG file too small\n")
		os.Exit(1)
	}
	loadAddr := uint16(prgData[0]) | uint16(prgData[1])<<8
	fmt.Printf("Load address: $%04X\n", loadAddr)
	fmt.Printf("Program size: %d bytes\n", len(prgData)-2)

	// Set PC to load address
	// If load address is $0801 (BASIC area), skip BASIC stub
	// Stub structure: 2-byte link + 2-byte line# + 1 SYS + 1 space + 4 digits + 1 end + 2 end = 13 bytes
	// So code starts at $0801 + 13 = $080E
	if loadAddr == 0x0801 {
		cpu.PC = 0x080E
	} else {
		cpu.PC = loadAddr
	}

	// Create assertion checker
	var assertChecker *AssertionChecker
	if metadata != nil {
		prgDir := filepath.Dir(filepath.Dir(prgFile))
		assertChecker = NewAssertionChecker(metadata, prgDir)
		fmt.Printf("📋 Loaded %d assertions\n", assertChecker.Total())
	}

	// Create frames directory
	os.MkdirAll("frames", 0755)

	// Emulation loop
	frameCount := 0
	renderCount := 0
	prevFrameVar := mem.RAM[0x0210] // FRAME variable at $0210; incremented after each render_square
	fmt.Printf("\n🚀 Starting C64 emulation...\n")
	fmt.Printf("Initial PC=$%04X, FRAME=$%02X\n", cpu.PC, mem.RAM[0x0210])

	// Track VIC timing
	nextVICCycle := cpu.Cycles + 63

	for frameCount < maxFrames {
		// Run one PAL frame (312 raster lines × 63 cycles = 19656 cycles)
		frameCycles := uint64(312 * 63)
		targetCycles := cpu.Cycles + frameCycles
		saveThisFrame := !noFrames && frameSelected(saveFrames, frameCount)
		vic.Recording = saveThisFrame

		for cpu.Cycles < targetCycles {
			// Check assertions before executing instruction
			if assertChecker != nil {
				assertChecker.Check(cpu.PC, mem)
			}

			cpu.Step()

			// Step VIC every 63 cycles (check if we've passed the threshold)
			if cpu.Cycles >= nextVICCycle {
				vic.StepLine()

				// Trigger IRQ if VIC raised raster interrupt
				if vic.RasterIRQ {
					cpu.IRQ()
				}

				nextVICCycle += 63
			}
		}

		// Save frame
		if saveThisFrame {
			filename := fmt.Sprintf("frames/frame_%05d_c64.png", frameCount)
			if err := c64.CaptureScreen(mem, filename); err != nil {
				fmt.Printf("Error saving frame %d: %v\n", frameCount, err)
			}
		}

		// Detect render completion: the 6502 increments FRAME after each render_square.
		if currentFrameVar := mem.RAM[0x0210]; currentFrameVar != prevFrameVar {
			renderCount++
			prevFrameVar = currentFrameVar
		}

		frameCount++
		if frameCount%100 == 0 {
			fmt.Printf("Frame %d / %d (PC=$%04X, FRAME=$%02X)\n",
				frameCount, maxFrames, cpu.PC, mem.RAM[0x0210])
		}

		if validate && assertChecker.AllPassed() {
			break
		}
	}

	fmt.Printf("\n✅ C64 emulation complete - %d PAL frames, %d renders\n", frameCount, renderCount)

	if assertChecker != nil {
		assertChecker.Summary()
		if validate && !assertChecker.AllPassed() {
			panic(fmt.Sprintf("only %d of %d assertions were reached", assertChecker.PassCount, assertChecker.Total()))
		}
	}
}

