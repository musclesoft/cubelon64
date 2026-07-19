package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// runValidateIRQ reads tracker_format.rom, runs the Go simulation as
// reference, then assembles and runs the IRQ-driven harness
// (player_harness_irq.s) under IRQMachine — the only path that actually
// executes player_cia1a_fire/player_cia1b_fire, since the plain -validate
// mode only ever calls player_tick directly. Exits 0 on pass, 1 on fail.
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
	addrInit := sym("sid_init")
	addrTogLo := sym("PLAYER_TOGGLE_LO")
	addrTogHi := sym("PLAYER_TOGGLE_HI")
	addrFrame := sym("PLAYER_FRAME")

	prgBytes, err := os.ReadFile(prgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading PRG: %v\n", err)
		os.Exit(1)
	}
	loadAddr := uint16(prgBytes[0]) | uint16(prgBytes[1])<<8

	cpu := newCPU6502()
	copy(cpu.Memory[loadAddr:], prgBytes[2:])
	installKernalIRQStub(cpu)

	// Install I/O hooks before running sid_init: sid_init (and the
	// player_init it calls) writes CIA1_ICR/VIC_IMR/etc., which must be
	// captured by IRQMachine's model, not raw memory.
	irq := NewIRQMachine(cpu, int(sidCyclesPerFrame))
	cpu.cpu6502Call(addrInit)

	totalFrames := len(refFrames)
	playerFrames := make([]SIDFrame, totalFrames)

	totalCIAA, totalCIAB, totalRaster := 0, 0, 0
	for f := 0; f < totalFrames; f++ {
		cpu.CurrentFrame = f
		irq.RunFrame()
		totalCIAA += irq.CIAAFires
		totalCIAB += irq.CIABFires
		totalRaster += irq.RasterFires
		var sf SIDFrame
		for ch := 0; ch < 3; ch++ {
			base := uint16(ch * 7)
			vf := &sf.Voices[ch]
			vf[VFFreqLo] = cpu.Memory[0xD400+base+0]
			vf[VFFreqHi] = cpu.Memory[0xD400+base+1]
			vf[VFPWLo] = cpu.Memory[0xD400+base+2]
			vf[VFPWHi] = cpu.Memory[0xD400+base+3]
			vf[VFCtrlA] = cpu.Memory[0xD400+base+4]
			vf[VFCtrlB] = cpu.Memory[addrFrame+uint16(ch*9+5)]
			vf[VFAttDec] = cpu.Memory[0xD400+base+5]
			vf[VFSusRel] = cpu.Memory[0xD400+base+6]
		}
		for ch := 0; ch < 2; ch++ {
			sf.TogglePeriod[ch] = uint16(cpu.Memory[addrTogLo+uint16(ch)]) |
				uint16(cpu.Memory[addrTogHi+uint16(ch)])<<8
		}
		playerFrames[f] = sf
	}
	fmt.Printf("Player:    %d frames, %d SID writes\n", totalFrames, len(cpu.SIDWrites))
	fmt.Printf("Totals:    raster=%d ciaA=%d ciaB=%d (expect raster=%d, i.e. 1/frame)\n\n",
		totalRaster, totalCIAA, totalCIAB, totalFrames)

	CompareSIDFrames(refFrames, playerFrames, 0, 0)
}
