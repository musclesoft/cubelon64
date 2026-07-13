package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"cubelon-emu/m6502"
)

// playerMeta holds the symbol table from the assembler meta.json output.
type playerMeta struct {
	Symbols map[string]uint16 `json:"symbols"`
}

// SIDWrite records a single SID register write with frame number.
type SIDWrite struct {
	Frame uint32
	Addr  uint16
	Value byte
}

// playerMemory is a flat 64K address space with SID and CIA write interception.
type playerMemory struct {
	ram [65536]byte

	// SID register snapshot — updated on every write to $D400–$D418
	sid [0x19]byte

	// SID write log — every write to $D400–$D418, in order, with frame number.
	// currentFrame must be set by the caller before each frame's calls.
	writes       []SIDWrite
	currentFrame uint32

	// CIA1 timer state
	ciaTA    uint16 // Timer A latch (written to $DC04/$DC05)
	ciaTB    uint16 // Timer B latch (written to $DC06/$DC07)
	ciaCRA   byte   // Control register A ($DC0E)
	ciaCRB   byte   // Control register B ($DC0F)
	ciaICR   byte   // ICR write mask ($DC0D)
}

func (m *playerMemory) Read(addr uint16) byte {
	// CIA1 ICR read: return pending flags and clear (simplified: always 0)
	if addr == 0xDC0D {
		return 0x00
	}
	return m.ram[addr]
}

func (m *playerMemory) Write(addr uint16, val byte) {
	m.ram[addr] = val
	switch {
	case addr >= 0xD400 && addr <= 0xD418:
		m.sid[addr-0xD400] = val
		m.writes = append(m.writes, SIDWrite{m.currentFrame, addr, val})
	case addr == 0xDC04:
		m.ciaTA = (m.ciaTA & 0xFF00) | uint16(val)
	case addr == 0xDC05:
		m.ciaTA = (m.ciaTA & 0x00FF) | uint16(val)<<8
	case addr == 0xDC06:
		m.ciaTB = (m.ciaTB & 0xFF00) | uint16(val)
	case addr == 0xDC07:
		m.ciaTB = (m.ciaTB & 0x00FF) | uint16(val)<<8
	case addr == 0xDC0D:
		m.ciaICR = val
	case addr == 0xDC0E:
		m.ciaCRA = val
	case addr == 0xDC0F:
		m.ciaCRB = val
	}
}

// Run6502Player assembles the player harness, loads it into a 6502 emulator,
// drives player_init + player_tick once per frame, fires CIA IRQs when timers
// expire, and returns one SIDFrame per song frame plus the full SID write log.
// totalFrames must be the song length in PAL frames (from SimulateTrackerFromROM).
func Run6502Player(trackerFormat *TrackerFormat, totalFrames int) ([]SIDFrame, []SIDWrite) {
	fmt.Printf("\n=== 6502 Player Execution ===\n")

	// 1. Assemble the player harness into a PRG.
	prgPath, metaPath, err := assemblePlayerHarness()
	if err != nil {
		panic(fmt.Sprintf("player harness assembly failed: %v", err))
	}

	// 2. Parse symbol table.
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		panic(fmt.Sprintf("reading player meta: %v", err))
	}
	var meta playerMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		panic(fmt.Sprintf("parsing player meta: %v", err))
	}
	sym := func(name string) uint16 {
		v, ok := meta.Symbols[name]
		if !ok {
			panic(fmt.Sprintf("symbol %q not found in player meta", name))
		}
		return v
	}

	addrInit    := sym("player_init")
	addrTick    := sym("player_tick")
	addrCIAA    := sym("player_cia1a_fire")
	addrCIAB    := sym("player_cia1b_fire")
	addrTogLo   := sym("PLAYER_TOGGLE_LO")
	addrTogHi   := sym("PLAYER_TOGGLE_HI")
	addrFrame   := sym("PLAYER_FRAME")

	// 3. Load PRG into flat memory (skip 2-byte load-address header).
	prgBytes, err := os.ReadFile(prgPath)
	if err != nil {
		panic(fmt.Sprintf("reading player PRG: %v", err))
	}
	if len(prgBytes) < 3 {
		panic("player PRG too short")
	}
	loadAddr := uint16(prgBytes[0]) | uint16(prgBytes[1])<<8
	mem := &playerMemory{}
	copy(mem.ram[loadAddr:], prgBytes[2:])

	// 4. Place an RTS sentinel at $FFFF so subroutine calls return cleanly.
	mem.ram[0xFFFF] = 0x60 // RTS

	cpu := m6502.New(mem)
	cpu.P = m6502.FlagU | m6502.FlagI
	cpu.SP = 0xFF

	// call runs a subroutine at addr and steps until it returns.
	// We push return address $FFFE (JSR convention: push PC-1), so RTS pulls it and
	// jumps to $FFFF where the RTS sentinel lives.
	// Push order matches the 6502: write hi byte first (write-then-decrement).
	call := func(addr uint16) {
		mem.ram[0x0100+uint16(cpu.SP)] = 0xFF // push hi byte of $FFFE
		cpu.SP--
		mem.ram[0x0100+uint16(cpu.SP)] = 0xFE // push lo byte of $FFFE
		cpu.SP--
		cpu.PC = addr
		for limit := 500000; cpu.PC != 0xFFFF; limit-- {
			if limit == 0 {
				panic(fmt.Sprintf("6502 call to $%04X did not return after 500000 steps (PC=$%04X)", addr, cpu.PC))
			}
			cpu.Step()
		}
	}

	// 5. Call player_init.
	call(addrInit)
	fmt.Printf("  player_init done\n")

	// CIA timer state: countdown in SID cycles, 0 = not running.
	ciaACounter := uint16(0)
	ciaBCounter := uint16(0)

	// 6. Run one player_tick per song frame, firing CIA IRQs as needed.
	fmt.Printf("  Running %d frames...\n", totalFrames)

	frames := make([]SIDFrame, 0, totalFrames)

	for f := 0; f < totalFrames; f++ {
		mem.currentFrame = uint32(f)
		// Fire CIA A if its timer has a pending reload (CRA bit 0 = start).
		if mem.ciaCRA&0x01 != 0 && mem.ciaTA > 0 {
			if ciaACounter == 0 {
				ciaACounter = mem.ciaTA
			} else if ciaACounter <= uint16(sidCyclesPerFrame) {
				// Timer fires this frame — call handler, reload counter.
				call(addrCIAA)
				ciaACounter = mem.ciaTA
			} else {
				ciaACounter -= uint16(sidCyclesPerFrame)
			}
		} else {
			ciaACounter = 0
		}

		if mem.ciaCRB&0x01 != 0 && mem.ciaTB > 0 {
			if ciaBCounter == 0 {
				ciaBCounter = mem.ciaTB
			} else if ciaBCounter <= uint16(sidCyclesPerFrame) {
				call(addrCIAB)
				ciaBCounter = mem.ciaTB
			} else {
				ciaBCounter -= uint16(sidCyclesPerFrame)
			}
		} else {
			ciaBCounter = 0
		}

		call(addrTick)

		// Build SIDFrame from register writes.
		// Voices at $D400, $D407, $D40E.
		var sf SIDFrame
		for ch := 0; ch < 3; ch++ {
			base := ch * 7
			vf := &sf.Voices[ch]
			vf[VFFreqLo] = mem.sid[base+0]
			vf[VFFreqHi] = mem.sid[base+1]
			vf[VFPWLo]   = mem.sid[base+2]
			vf[VFPWHi]   = mem.sid[base+3]
			vf[VFCtrlA]  = mem.sid[base+4]
			vf[VFCtrlB]  = mem.ram[addrFrame+uint16(ch*9+5)] // lo-phase ctrl from PLAYER_FRAME
			vf[VFAttDec] = mem.sid[base+5]
			vf[VFSusRel] = mem.sid[base+6]
		}


		// CIA toggle periods from BSS: PLAYER_TOGGLE_LO/HI
		for ch := 0; ch < 2; ch++ {
			sf.TogglePeriod[ch] = uint16(mem.ram[addrTogLo+uint16(ch)]) | uint16(mem.ram[addrTogHi+uint16(ch)])<<8
		}

		frames = append(frames, sf)
	}

	fmt.Printf("  6502 player execution complete (%d frames, %d SID writes)\n", len(frames), len(mem.writes))
	return frames, mem.writes
}

// assemblePlayerHarness runs the asm tool on player_harness.s and returns
// (prgPath, metaPath). Files are written under tools/build/.
func assemblePlayerHarness() (prgPath, metaPath string, err error) {
	buildDir := "build"
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return "", "", fmt.Errorf("mkdir build: %w", err)
	}

	// prgPath / metaPath are relative to tools/ (current working directory).
	prgPath  = filepath.Join(buildDir, "player_harness.prg")
	metaPath = prgPath + ".meta.json"

	// go run -C ../asm . runs the asm tool from the ../asm directory.
	// All path arguments to the asm tool are therefore relative to ../asm:
	//   config  ../c64port/c64.cfg            (../asm → ../c64port/c64.cfg)
	//   output  ../tools/build/player_harness.prg
	//   source  ../c64port/src/player_harness.s
	cmd := exec.Command("go", "run", "-C", "../asm", ".",
		"-C", "../c64port/c64.cfg",
		"-o", "../tools/"+prgPath,
		"../c64port/src/player_harness.s",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("asm tool: %w", err)
	}
	return prgPath, metaPath, nil
}
