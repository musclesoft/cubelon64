// ZX Spectrum Music Player - CUBELON
//
// Implements the interrupt-driven AY player from the CUBELON ZX Spectrum 128K demo.
// Requires three bank dumps captured from the running emulator:
//
//   ram_bank1_frame2.bin   - bank 1 at demo frame 2 (silent intro, 4 patterns)
//   ram_bank3_frame162.bin - bank 3 at demo frame 162 (intro + 32 music patterns)
//   ram_bank1_frame802.bin - bank 1 at demo frame 802 (full 432-pattern song)
//
// Architecture (from emulator execution tracing):
//
//   The channel processor code at $7D30-$7DFC runs every interrupt (50Hz).
//   It bank-switches to a music bank ($7D35 = active bank number), then runs
//   14 channel processors — one per AY register:
//     - Ch 0-12: OUTI reads one byte from bank → writes to AY R0-R12
//     - Ch 13:   LD A,(HL); RRCA; writes RRCA(byte) to AY R13 only when byte is even
//
//   The music player tick routine at $7EC5 runs every frame; on tick=0 it:
//     - Reads next pattern index from the sequence at $C000 in the current bank
//     - If $FF: toggles active bank (1↔3) via XOR 2, resets sequence pointer
//     - Otherwise: calculates pattern address via RRCA×3, loads channel pointers
//
// Song structure (AY frames 2-9422):
//
//   Phase 1 bank1_frame2:  frames   2- 81 (4 silence patterns × 20 frames)
//   Phase 2 bank3_frame162: frames  82-801 (36 patterns: 4 silence + 32 music)
//   Phase 3 bank1_frame802: frames 802-9422 (432 patterns of full song)
//
// Pattern record (29 bytes at bank[$C400+] via RRCA×3 addressing):
//   [0]      tick_count     - frames this pattern runs
//   [1-2]    ch0 ptr lo/hi  - initial ZX address in current bank
//   [3-28]   ch1..ch13 ptrs
//
// Ch13 encoding: byte b → R13 = RRCA(b) = (b>>1)|(bit0<<7); write only when b is even.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

const memBase = 0xC000 // bank data covers $C000-$FFFF

// AYCommand is one write to an AY register at a specific frame.
type AYCommand struct {
	Frame    uint32
	Register byte
	Value    byte
}

// ZXMemory holds one 16KB bank ($C000-$FFFF).
type ZXMemory struct {
	data [0x4000]byte
}

func (m *ZXMemory) read(addr uint16) byte {
	return m.data[addr-memBase]
}

func (m *ZXMemory) readLE16(addr uint16) uint16 {
	return uint16(m.data[addr-memBase]) | uint16(m.data[addr-memBase+1])<<8
}

// patternAddr converts a sequence byte to its bank address using RRCA×3.
// Replicates Z80 code at $7EF9-$7F06 in the original player.
func patternAddr(idx byte) uint16 {
	r := idx
	for i := 0; i < 3; i++ {
		r = (r >> 1) | ((r & 1) << 7)
	}
	return (uint16(r&0x1F)+0xC4)<<8 | uint16(r&0xE0)
}

const ticksPerPattern = 20

// ZXPlayer plays one bank's pattern sequence from a given start frame.
type ZXPlayer struct {
	mem       *ZXMemory
	seqPtr    int
	ticksLeft int
	chPtrs    [14]uint16
}

// NewZXPlayer creates a player ready to start from the first sequence entry.
func NewZXPlayer(mem *ZXMemory) *ZXPlayer {
	return &ZXPlayer{mem: mem}
}

// Play runs the player starting at startFrame until the sequence ends or
// maxFrames frames have been played. Returns (commands, next frame number).
func (p *ZXPlayer) Play(startFrame, maxFrames int) ([]AYCommand, int) {
	var out []AYCommand
	p.seqPtr = 0
	p.ticksLeft = 0

	frame := startFrame
	endFrame := startFrame + maxFrames

	for frame < endFrame {
		if p.ticksLeft == 0 {
			if p.seqPtr >= 0x1B1 || p.mem.data[p.seqPtr] == 0xFF {
				break
			}
			seqByte := p.mem.data[p.seqPtr]
			p.seqPtr++
			addr := patternAddr(seqByte)
			p.ticksLeft = ticksPerPattern
			for ch := 0; ch < 12; ch++ {
				p.chPtrs[ch] = p.mem.readLE16(addr + 1 + uint16(ch)*2)
			}
			// ch12 (R12 envelope period hi) is always zero — pointer not loaded
			p.chPtrs[13] = p.mem.readLE16(addr + 1 + 13*2)
		}
		p.ticksLeft--

		// Ch 0-11: raw byte → AY register (OUTI mechanism)
		for ch := 0; ch < 12; ch++ {
			val := p.mem.data[p.chPtrs[ch]-memBase]
			p.chPtrs[ch]++
			out = append(out, AYCommand{Frame: uint32(frame), Register: byte(ch), Value: val})
		}
		// Ch 12 (R12 envelope period hi) is always 0x00 — hardcoded, not read from data
		out = append(out, AYCommand{Frame: uint32(frame), Register: 12, Value: 0})

		// Ch 13: RRCA encoding; write only when original byte is even
		b13 := p.mem.data[p.chPtrs[13]-memBase]
		p.chPtrs[13]++
		if b13&1 == 0 {
			out = append(out, AYCommand{
				Frame:    uint32(frame),
				Register: 13,
				Value:    (b13 >> 1) | ((b13 & 1) << 7),
			})
		}

		frame++
	}
	return out, frame
}

// PlaySong plays the full CUBELON song using three bank dumps.
// Phase 1: bank1_frame2   (frames 2-81)
// Phase 2: bank3_frame162 (frames 82-801)
// Phase 3: bank1_frame802 (frames 802 through lastFrame)
func PlaySong(bank1Early, bank3, bank1Music *ZXMemory, lastFrame int) []AYCommand {
	var all []AYCommand

	p := NewZXPlayer(bank1Early)
	cmds, nextFrame := p.Play(2, 9999)
	all = append(all, cmds...)

	p = NewZXPlayer(bank3)
	cmds, nextFrame = p.Play(nextFrame, 9999)
	all = append(all, cmds...)

	p = NewZXPlayer(bank1Music)
	cmds, _ = p.Play(nextFrame, lastFrame+1-nextFrame)
	all = append(all, cmds...)

	return all
}

// ── File loaders ──────────────────────────────────────────────────────────────

// loadBank loads a compact sparse binary file produced by trimming to only
// the actually-read byte ranges. Format: [num_runs:u16] then per run:
// [zx_addr:u16][len:u16][data...]. Gaps between runs are left as zero.
func loadBank(path string) (*ZXMemory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 2 {
		return nil, fmt.Errorf("file too short")
	}
	m := &ZXMemory{}
	numRuns := binary.LittleEndian.Uint16(data[0:2])
	off := 2
	for i := uint16(0); i < numRuns; i++ {
		if off+4 > len(data) {
			return nil, fmt.Errorf("truncated run header at run %d", i)
		}
		addr := binary.LittleEndian.Uint16(data[off:])
		runLen := int(binary.LittleEndian.Uint16(data[off+2:]))
		off += 4
		if off+runLen > len(data) {
			return nil, fmt.Errorf("truncated run data at run %d", i)
		}
		if addr < memBase || int(addr-memBase)+runLen > 0x4000 {
			return nil, fmt.Errorf("run %d address $%04X out of range", i, addr)
		}
		copy(m.data[addr-memBase:], data[off:off+runLen])
		off += runLen
	}
	return m, nil
}

func loadAYCommands(path string) ([]AYCommand, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 {
		return nil, fmt.Errorf("file too short")
	}
	count := binary.LittleEndian.Uint32(data[0:4])
	cmds := make([]AYCommand, 0, count)
	for i := uint32(0); i < count; i++ {
		off := 4 + i*6
		if int(off)+6 > len(data) {
			break
		}
		cmds = append(cmds, AYCommand{
			Frame:    binary.LittleEndian.Uint32(data[off:]),
			Register: data[off+4],
			Value:    data[off+5],
		})
	}
	return cmds, nil
}

// ── Validation ────────────────────────────────────────────────────────────────

func validate(original, replayed []AYCommand) (matched, total int, firstMiss string) {
	type key struct {
		frame uint32
		reg   byte
	}
	orig := make(map[key]byte, len(original))
	for _, c := range original {
		orig[key{c.Frame, c.Register}] = c.Value
	}
	for _, c := range replayed {
		k := key{c.Frame, c.Register}
		if v, ok := orig[k]; ok && v == c.Value {
			matched++
		} else if firstMiss == "" {
			if ok {
				firstMiss = fmt.Sprintf("frame %d R%d: got 0x%02X want 0x%02X",
					c.Frame, c.Register, c.Value, v)
			} else {
				firstMiss = fmt.Sprintf("frame %d R%d: 0x%02X not in original",
					c.Frame, c.Register, c.Value)
			}
		}
	}
	total = len(replayed)
	return
}

func analyzeDiscrepancies(original, replayed []AYCommand) {
	type key struct {
		frame uint32
		reg   byte
	}
	origMap := make(map[key]byte, len(original))
	for _, c := range original {
		origMap[key{c.Frame, c.Register}] = c.Value
	}
	replayMap := make(map[key]byte, len(replayed))
	for _, c := range replayed {
		replayMap[key{c.Frame, c.Register}] = c.Value
	}

	regMiss := make(map[byte]int)
	regExtra := make(map[byte]int)
	shown := 0
	for _, c := range replayed {
		k := key{c.Frame, c.Register}
		if v, ok := origMap[k]; !ok {
			regExtra[c.Register]++
			if shown < 12 {
				fmt.Printf("  Extra:    frame %d R%d = 0x%02X\n", c.Frame, c.Register, c.Value)
				shown++
			}
		} else if v != c.Value {
			regMiss[c.Register]++
			if shown < 12 {
				fmt.Printf("  Mismatch: frame %d R%d got 0x%02X want 0x%02X\n",
					c.Frame, c.Register, c.Value, v)
				shown++
			}
		}
	}

	missing := 0
	for _, c := range original {
		if _, ok := replayMap[key{c.Frame, c.Register}]; !ok {
			missing++
		}
	}

	fmt.Printf("\nPer-register mismatches:\n")
	regNames := []string{"ToneALo", "ToneAHi", "ToneBLo", "ToneBHi", "ToneCLo", "ToneCHi",
		"Noise", "Mixer", "VolA", "VolB", "VolC", "EnvLo", "EnvHi", "EnvShape"}
	for r := 0; r < 14; r++ {
		if n := regMiss[byte(r)]; n > 0 {
			fmt.Printf("  R%-2d %-9s mismatch: %d\n", r, regNames[r], n)
		}
	}
	fmt.Printf("Per-register extras (not in original):\n")
	for r := 0; r < 14; r++ {
		if n := regExtra[byte(r)]; n > 0 {
			fmt.Printf("  R%-2d %-9s extra:    %d\n", r, regNames[r], n)
		}
	}
	fmt.Printf("Original commands missing from player: %d\n", missing)
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	dir := "../../emulator/"
	if len(os.Args) > 1 {
		dir = os.Args[1]
		if dir[len(dir)-1] != '/' {
			dir += "/"
		}
	}
	ayPath := dir + "ay_commands.bin"
	if len(os.Args) > 2 {
		ayPath = os.Args[2]
	}

	fmt.Printf("Loading bank dumps from %s\n", dir)
	bank1Early, err := loadBank(dir + "music_bank1_intro.bin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	bank3, err := loadBank(dir + "music_bank3_mid.bin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	bank1Music, err := loadBank(dir + "music_bank1_main.bin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded music_bank1_intro, music_bank3_mid, music_bank1_main\n")

	fmt.Printf("Loading AY command log from %s\n", ayPath)
	cmds, err := loadAYCommands(ayPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	lastFrame := 0
	for _, c := range cmds {
		if int(c.Frame) > lastFrame {
			lastFrame = int(c.Frame)
		}
	}
	fmt.Printf("Original: %d commands (last frame: %d)\n\n", len(cmds), lastFrame)

	fmt.Printf("Running ZX player...\n")
	replayed := PlaySong(bank1Early, bank3, bank1Music, lastFrame)
	fmt.Printf("Player emitted %d commands\n\n", len(replayed))

	fmt.Printf("Validating against original...\n")
	matched, total, firstMiss := validate(cmds, replayed)
	pct := 0.0
	if total > 0 {
		pct = 100.0 * float64(matched) / float64(total)
	}
	fmt.Printf("Player vs original: %d / %d (%.6f%%)\n", matched, total, pct)
	if firstMiss != "" {
		fmt.Printf("First mismatch: %s\n", firstMiss)
	}

	covMatched, covTotal, _ := validate(replayed, cmds)
	covPct := 0.0
	if covTotal > 0 {
		covPct = 100.0 * float64(covMatched) / float64(covTotal)
	}
	fmt.Printf("Coverage:        %d / %d original reproduced (%.6f%%)\n",
		covMatched, covTotal, covPct)

	if pct >= 99.9999 && covPct >= 99.9999 {
		fmt.Printf("\n✓ ZX player reproduces original music 100%%\n")
	} else {
		fmt.Printf("\n✗ Validation incomplete\n")
		analyzeDiscrepancies(cmds, replayed)
	}
}
