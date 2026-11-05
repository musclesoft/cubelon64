package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// RunIRQPlayer assembles and runs the IRQ-driven harness (player_harness_irq.s)
// under IRQMachine for totalFrames frames — the only path that actually
// executes music_player_write_ctrl via real CIA1 timer underflow
// interrupts, since the plain sid_update/step-only harness (Run6502Player) never
// fires them. Frames holds one SIDFrame per song frame, the same shape
// Run6502Player returns, so validation can treat the two harnesses
// interchangeably; Writes is the cycle-stamped SID write stream for audio
// rendering.
type IRQRun struct {
	Frames      []SIDFrame
	Writes      []SIDWrite
	RasterFires int
	CIAAFires   int
	CIABFires   int

	HandlerCyclesMax   int64
	HandlerCyclesTotal int64
}

func RunIRQPlayer(totalFrames int) IRQRun {
	if err := assembleIRQHarness(); err != nil {
		fmt.Fprintf(os.Stderr, "assembly failed: %v\n", err)
		os.Exit(1)
	}
	prgPath := "build/player_harness_irq.prg"
	metaPath := prgPath + ".meta.json"

	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading meta: %v\n", err)
		os.Exit(1)
	}
	var meta playerMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		fmt.Fprintf(os.Stderr, "parsing meta: %v\n", err)
		os.Exit(1)
	}
	sym := func(name string) uint16 {
		v, ok := meta.Symbols[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "symbol %q not found\n", name)
			os.Exit(1)
		}
		return v
	}
	addrInit := sym("sid_setup")
	addrTogLo := sym("PLAYER_TOGGLE_LO")
	addrTogHi := sym("PLAYER_TOGGLE_HI")
	addrCtrlA := sym("ZP_FRAME_CTRL_A")
	addrCtrlB := sym("ZP_FRAME_CTRL_B")

	prgBytes, err := os.ReadFile(prgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading PRG: %v\n", err)
		os.Exit(1)
	}
	loadAddr := uint16(prgBytes[0]) | uint16(prgBytes[1])<<8

	cpu := newCPU6502()
	copy(cpu.Memory[loadAddr:], prgBytes[2:])

	// Install I/O hooks before running sid_setup: sid_setup (and the
	// music_player_init it calls) writes CIA1_ICR/VIC_IMR/etc., which must be
	// captured by IRQMachine's model, not raw memory.
	irq := NewIRQMachine(cpu, int(sidCyclesPerFrame))
	cpu.cpu6502Call(addrInit)
	irq.Begin()

	playerFrames := make([]SIDFrame, totalFrames)

	totalCIAA, totalCIAB, totalRaster := 0, 0, 0
	var handlerMax, handlerTotal int64
	for f := 0; f < totalFrames; f++ {
		cpu.CurrentFrame = f
		var sf SIDFrame
		for ch := 0; ch < 2; ch++ {
			sf.TogglePeriod[ch] = uint16(cpu.Memory[addrTogLo+uint16(ch)]) |
				uint16(cpu.Memory[addrTogHi+uint16(ch)])<<8
		}
		irq.RunFrame()
		handlerTotal += irq.HandlerCycles
		if irq.HandlerCycles > handlerMax {
			handlerMax = irq.HandlerCycles
		}
		totalCIAA += irq.CIAAFires
		totalCIAB += irq.CIABFires
		totalRaster += irq.RasterFires
		for ch := 0; ch < 3; ch++ {
			base := uint16(ch * 7)
			vf := &sf.Voices[ch]
			vf[VFFreqLo] = cpu.Memory[0xD400+base+0]
			vf[VFFreqHi] = cpu.Memory[0xD400+base+1]
			vf[VFPWLo] = cpu.Memory[0xD400+base+2]
			vf[VFPWHi] = cpu.Memory[0xD400+base+3]
			vf[VFCtrlA] = cpu.Memory[addrCtrlA+uint16(ch)]
			vf[VFCtrlB] = cpu.Memory[addrCtrlB+uint16(ch)]
			vf[VFAttDec] = cpu.Memory[0xD400+base+5]
			vf[VFSusRel] = cpu.Memory[0xD400+base+6]
		}
		playerFrames[f] = sf
	}
	fmt.Printf("Player:    %d frames, %d SID writes\n", totalFrames, len(cpu.SIDWrites))
	fmt.Printf("Totals:    raster=%d ciaA=%d ciaB=%d (expect raster=%d, i.e. 1/frame)\n\n",
		totalRaster, totalCIAA, totalCIAB, totalFrames)

	return IRQRun{
		Frames:      playerFrames,
		Writes:      cpu.SIDWrites,
		RasterFires: totalRaster,
		CIAAFires:   totalCIAA,
		CIABFires:   totalCIAB,

		HandlerCyclesMax:   handlerMax,
		HandlerCyclesTotal: handlerTotal,
	}
}

// runValidateIRQ reads tracker_format.rom, runs the Go simulation as
// reference, then runs the IRQ-driven harness via RunIRQPlayer and compares
// against the reference. Exits 0 on pass, 1 on fail.
func runValidateIRQ() {
	fmt.Printf("=== Validate IRQ-driven harness against tracker_format.rom ===\n\n")

	rom, err := os.ReadFile("tracker_format.rom")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read tracker_format.rom: %v\n", err)
		os.Exit(1)
	}

	refFrames := SimulateTrackerFromROM(rom)
	fmt.Printf("Reference: %d frames\n", len(refFrames))

	writeTrackerASM(rom, "../c64port/src/music_data.inc")

	CompareSIDFrames(refFrames, RunIRQPlayer(len(refFrames)).Frames, 0, 0)
}
