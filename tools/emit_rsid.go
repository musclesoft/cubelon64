package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// EmitPSID builds a PSID v2 file from the plain player harness.
// The SID player calls player_init once, then calls player_tick at 50 Hz directly.
// PSID is more widely compatible than RSID because it does not require VIC raster simulation.
func EmitPSID(outputPath string) {
	prgPath := filepath.Join("build", "player_harness.prg")
	loadAddr, initAddr, playAddr, prgBytes := loadHarnessPRG(prgPath, "player_init", "player_tick")

	hdr := buildSIDHeader("PSID", loadAddr, initAddr, playAddr)
	writeSIDFile(outputPath, hdr, prgBytes)
	fmt.Printf("✓ PSID written to: %s (load=$%04X init=$%04X play=$%04X)\n", outputPath, loadAddr, initAddr, playAddr)
}

// EmitRSID builds a true RSID v2 file from the IRQ-driven player harness.
// sid_init sets up VIC raster and CIA1 IRQs; the SID player calls it once then
// lets hardware drive the music. playAddress is 0 (IRQ-driven).
func EmitRSID(outputPath string) {
	if err := assembleRSIDHarness(); err != nil {
		panic(fmt.Sprintf("RSID harness assembly failed: %v", err))
	}
	prgPath := filepath.Join("build", "player_harness_rsid.prg")
	loadAddr, initAddr, _, prgBytes := loadHarnessPRG(prgPath, "sid_init")

	hdr := buildSIDHeader("RSID", loadAddr, initAddr, 0x0000)
	writeSIDFile(outputPath, hdr, prgBytes)
	fmt.Printf("✓ RSID written to: %s (load=$%04X init=$%04X play=$0000 IRQ-driven)\n", outputPath, loadAddr, initAddr)
}

// loadHarnessPRG reads a PRG file and its meta.json, resolves the named symbols,
// and returns (loadAddr, sym0, sym1, prgBody). sym1 is 0 if only one name given.
func loadHarnessPRG(prgPath string, symNames ...string) (loadAddr, sym0, sym1 uint16, body []byte) {
	metaBytes, err := os.ReadFile(prgPath + ".meta.json")
	if err != nil {
		panic(fmt.Sprintf("reading harness meta %s: %v", prgPath, err))
	}
	var meta playerMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		panic(fmt.Sprintf("parsing harness meta: %v", err))
	}
	sym := func(name string) uint16 {
		v, ok := meta.Symbols[name]
		if !ok {
			panic(fmt.Sprintf("harness symbol %q not found", name))
		}
		return v
	}

	prgBytes, err := os.ReadFile(prgPath)
	if err != nil {
		panic(fmt.Sprintf("reading harness PRG %s: %v", prgPath, err))
	}
	if len(prgBytes) < 3 {
		panic("harness PRG too short")
	}

	loadAddr = uint16(prgBytes[0]) | uint16(prgBytes[1])<<8
	body = prgBytes[2:]

	if len(symNames) > 0 {
		sym0 = sym(symNames[0])
	}
	if len(symNames) > 1 {
		sym1 = sym(symNames[1])
	}
	return
}

// buildSIDHeader constructs a PSID/RSID v2 header (124 bytes).
// magic must be "PSID" or "RSID". For RSID, playAddr must be 0.
func buildSIDHeader(magic string, loadAddr, initAddr, playAddr uint16) []byte {
	hdr := make([]byte, 124)
	copy(hdr[0:4], magic)
	binary.BigEndian.PutUint16(hdr[4:], 2)         // version 2
	binary.BigEndian.PutUint16(hdr[6:], 124)        // data offset
	binary.BigEndian.PutUint16(hdr[8:], loadAddr)
	binary.BigEndian.PutUint16(hdr[10:], initAddr)
	binary.BigEndian.PutUint16(hdr[12:], playAddr)
	binary.BigEndian.PutUint16(hdr[14:], 1)         // songs
	binary.BigEndian.PutUint16(hdr[16:], 1)         // default song
	binary.BigEndian.PutUint32(hdr[18:], 0)         // speed: 0 = VIC/IRQ-driven (50 Hz PAL)
	copy(hdr[22:54], "Converted from ZX Spectrum")
	copy(hdr[54:86], "<author>")
	copy(hdr[86:118], "<year>")
	binary.BigEndian.PutUint16(hdr[118:], 0x14)     // flags: PAL (bits 2-3=01), MOS6581 (bits 4-5=01)
	// hdr[120] startPage = 0 (unspecified), hdr[121] pageLength = 0
	return hdr
}

// writeSIDFile creates outputPath and writes hdr + body.
func writeSIDFile(outputPath string, hdr, body []byte) {
	out, err := os.Create(outputPath)
	if err != nil {
		panic(fmt.Sprintf("creating SID output %s: %v", outputPath, err))
	}
	defer out.Close()
	out.Write(hdr)
	out.Write(body)
}

// assembleRSIDHarness assembles player_harness_rsid.s into build/player_harness_rsid.prg.
func assembleRSIDHarness() error {
	buildDir := "build"
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return fmt.Errorf("mkdir build: %w", err)
	}
	prgPath := filepath.Join(buildDir, "player_harness_rsid.prg")
	cmd := exec.Command("go", "run", "-C", "../asm", ".",
		"-C", "../c64port/c64.cfg",
		"-o", "../tools/"+prgPath,
		"../c64port/src/player_harness_rsid.s",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("asm tool: %w", err)
	}
	return nil
}
