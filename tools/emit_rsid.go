package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// EmitPSID builds a PSID v2 file from the plain player harness.
// The SID player calls player_init once, then calls player_tick at 50 Hz directly.
// PSID is more widely compatible than RSID because it does not require VIC raster simulation.
func EmitPSID(outputPath string) {
	// Re-use the plain harness built for the 6502 emulator.
	prgPath := filepath.Join("build", "player_harness.prg")
	metaPath := prgPath + ".meta.json"

	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		panic(fmt.Sprintf("reading player meta for SID: %v", err))
	}
	var meta playerMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		panic(fmt.Sprintf("parsing player meta for SID: %v", err))
	}
	sym := func(name string) uint16 {
		v, ok := meta.Symbols[name]
		if !ok {
			panic(fmt.Sprintf("SID symbol %q not found", name))
		}
		return v
	}

	prgBytes, err := os.ReadFile(prgPath)
	if err != nil {
		panic(fmt.Sprintf("reading player PRG for SID: %v", err))
	}
	if len(prgBytes) < 3 {
		panic("player PRG too short")
	}
	loadAddr := uint16(prgBytes[0]) | uint16(prgBytes[1])<<8
	initAddr := sym("player_init")
	playAddr := sym("player_tick")

	// PSID v2 header: 124 bytes.
	hdr := make([]byte, 124)
	copy(hdr[0:4], "PSID")
	binary.BigEndian.PutUint16(hdr[4:], 2)      // version 2
	binary.BigEndian.PutUint16(hdr[6:], 124)    // data offset
	binary.BigEndian.PutUint16(hdr[8:], loadAddr)
	binary.BigEndian.PutUint16(hdr[10:], initAddr)
	binary.BigEndian.PutUint16(hdr[12:], playAddr)
	binary.BigEndian.PutUint16(hdr[14:], 1)     // songs
	binary.BigEndian.PutUint16(hdr[16:], 1)     // default song
	binary.BigEndian.PutUint32(hdr[18:], 0)     // speed bit 0 = CIA1A; 0 = VIC (50 Hz)
	copy(hdr[22:54], "Converted from ZX Spectrum")
	copy(hdr[54:86], "<author>")
	copy(hdr[86:118], "<year>")
	binary.BigEndian.PutUint16(hdr[118:], 0)    // flags: PAL, no SID2/3
	hdr[120] = 0                                 // start page (0 = reserved/unspecified)
	hdr[121] = 0                                 // page length
	binary.BigEndian.PutUint16(hdr[122:], 0)    // reserved

	out, err := os.Create(outputPath)
	if err != nil {
		panic(fmt.Sprintf("creating SID output: %v", err))
	}
	defer out.Close()
	out.Write(hdr)
	out.Write(prgBytes[2:]) // skip 2-byte load-address prefix
	fmt.Printf("✓ PSID written to: %s (load=$%04X init=$%04X play=$%04X)\n", outputPath, loadAddr, initAddr, playAddr)
}
