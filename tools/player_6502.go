package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// playerMeta holds the symbol table from the assembler meta.json output.
type playerMeta struct {
	Symbols map[string]uint16 `json:"symbols"`
}

// SIDWrite records a single SID register write with frame number.
// Frame is uint32 to match the CPU6502 CurrentFrame type.
type SIDWrite struct {
	Frame uint32
	Addr  uint16
	Value byte
}

// Run6502Player assembles the player harness, loads it into the CPU6502 emulator,
// drives player_init once then player_tick once per frame, and returns one
// SIDFrame per song frame plus the full ordered SID write log.
func Run6502Player(trackerFormat *TrackerFormat, totalFrames int) ([]SIDFrame, []SIDWrite) {
	fmt.Printf("\n=== 6502 Player Execution ===\n")

	prgPath, metaPath, err := assemblePlayerHarness()
	if err != nil {
		panic(fmt.Sprintf("player harness assembly: %v", err))
	}

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

	addrInit   := sym("player_init")
	addrTick   := sym("player_tick")
	addrTogLo  := sym("PLAYER_TOGGLE_LO")
	addrTogHi  := sym("PLAYER_TOGGLE_HI")
	addrFrame  := sym("PLAYER_FRAME")

	prgBytes, err := os.ReadFile(prgPath)
	if err != nil {
		panic(fmt.Sprintf("reading player PRG: %v", err))
	}
	if len(prgBytes) < 3 {
		panic("player PRG too short")
	}
	loadAddr := uint16(prgBytes[0]) | uint16(prgBytes[1])<<8

	cpu := newCPU6502()
	copy(cpu.Memory[loadAddr:], prgBytes[2:])

	fmt.Printf("  Running %d frames...\n", totalFrames)
	cpu.cpu6502Call(addrInit)

	frames := make([]SIDFrame, totalFrames)
	for f := 0; f < totalFrames; f++ {
		cpu.CurrentFrame = f
		cpu.cpu6502Call(addrTick)

		var sf SIDFrame
		for ch := 0; ch < 3; ch++ {
			base := uint16(ch * 7)
			vf := &sf.Voices[ch]
			vf[VFFreqLo] = cpu.Memory[0xD400+base+0]
			vf[VFFreqHi] = cpu.Memory[0xD400+base+1]
			vf[VFPWLo]   = cpu.Memory[0xD400+base+2]
			vf[VFPWHi]   = cpu.Memory[0xD400+base+3]
			vf[VFCtrlA]  = cpu.Memory[0xD400+base+4]
			vf[VFCtrlB]  = cpu.Memory[addrFrame+uint16(ch*9+5)]
			vf[VFAttDec] = cpu.Memory[0xD400+base+5]
			vf[VFSusRel] = cpu.Memory[0xD400+base+6]
		}
		for ch := 0; ch < 2; ch++ {
			sf.TogglePeriod[ch] = uint16(cpu.Memory[addrTogLo+uint16(ch)]) |
				uint16(cpu.Memory[addrTogHi+uint16(ch)])<<8
		}
		frames[f] = sf
	}

	fmt.Printf("  6502 player done (%d frames, %d SID writes)\n", totalFrames, len(cpu.SIDWrites))
	return frames, cpu.SIDWrites
}

// assemblePlayerHarness runs the asm tool on player_harness.s.
// Files are written under tools/build/.
func assemblePlayerHarness() (prgPath, metaPath string, err error) {
	buildDir := "build"
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return "", "", fmt.Errorf("mkdir build: %w", err)
	}
	prgPath  = filepath.Join(buildDir, "player_harness.prg")
	metaPath = prgPath + ".meta.json"
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
