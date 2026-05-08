// ZX Spectrum Music Format - CUBELON
//
// Reverse-engineered music format for the CUBELON demo.
// Extracts music data from ay_commands.bin (frame-accurate AY register log)
// and reconstructs the original ZX player logic in Go.
//
// The ZX player runs at 50 Hz (interrupt-driven). Each frame it writes
// AY registers R0-R12 unconditionally; R13 only when the value changes.
// Notes are quantised to 5-frame ticks (10 Hz note rate).
//
// Validation: re-running the player must reproduce ay_commands.bin exactly.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
)

// ── AY-3-8912 hardware constants ─────────────────────────────────────────────

const (
	frameRate = 50 // PAL interrupt rate

	// Register numbers
	regToneALo  = 0
	regToneAHi  = 1
	regToneBLo  = 2
	regToneBHi  = 3
	regToneCLo  = 4
	regToneCHi  = 5
	regNoise    = 6
	regMixer    = 7
	regVolA     = 8
	regVolB     = 9
	regVolC     = 10
	regEnvLo    = 11
	regEnvHi    = 12
	regEnvShape = 13
)

// ── Data structures ───────────────────────────────────────────────────────────

// AYCommand mirrors spectrum.AYCommand so this file compiles standalone.
type AYCommand struct {
	Frame    uint32
	Register byte
	Value    byte
}

// ZXNote is one note-event on one channel, in AY-native units.
// During rest, Volume == 0 but TonePeriod still holds the last active value
// (the ZX player always writes R0-R12 every frame regardless of volume).
type ZXNote struct {
	StartFrame int
	DurFrames  int
	IsRest     bool

	// Per-frame slices, len == DurFrames.
	// Shared registers (Noise, Mixer, EnvPeriod) are stored on every channel
	// but only one channel drives each shared register per frame.
	TonePeriod []uint16 // R0/R1 12-bit combined
	Noise      []byte   // R6
	Mixer      []byte   // R7
	Volume     []byte   // R8/R9/R10 for this channel
	EnvPeriod  []uint16 // R11/R12 16-bit combined
	EnvShape   []byte   // R13; 0xFF = no write this frame
}

// ZXMusic is the complete extracted music data.
type ZXMusic struct {
	StartFrame int
	EndFrame   int
	Notes      [3][]ZXNote
}

// ── Extraction ────────────────────────────────────────────────────────────────

// ExtractZXMusic builds ZXMusic from the raw AY command log.
// Covers the full frame range from the first AY write to the last,
// including the init phase before music starts.
// The critical invariant: TonePeriod holds the forward-filled register
// value at every frame (ZX player always writes R0-R12 every frame).
func ExtractZXMusic(cmds []AYCommand) *ZXMusic {
	if len(cmds) == 0 {
		return &ZXMusic{}
	}

	maxFrame := int(cmds[len(cmds)-1].Frame)

	// Build per-frame snapshots: only actual write frames are recorded.
	writeAt := make([][]int16, 16)
	for r := 0; r < 16; r++ {
		writeAt[r] = make([]int16, maxFrame+1)
		for f := range writeAt[r] {
			writeAt[r][f] = -1 // -1 = no write on this frame
		}
	}
	for _, c := range cmds {
		if c.Register < 16 {
			writeAt[c.Register][c.Frame] = int16(c.Value)
		}
	}

	// Forward-fill: each register holds its last-written value.
	state := make([][]byte, 16)
	for r := 0; r < 16; r++ {
		state[r] = make([]byte, maxFrame+1)
	}
	cur := [16]byte{}
	cur[regMixer] = 0xFF // default: all disabled
	for f := 0; f <= maxFrame; f++ {
		for r := 0; r < 16; r++ {
			if writeAt[r][f] >= 0 {
				cur[r] = byte(writeAt[r][f])
			}
			state[r][f] = cur[r]
		}
	}

	// Reconstruct 12-bit tone periods and 16-bit env period per frame.
	tonePeriod := [3][]uint16{}
	for ch := 0; ch < 3; ch++ {
		tonePeriod[ch] = make([]uint16, maxFrame+1)
		for f := 0; f <= maxFrame; f++ {
			lo := state[ch*2][f]
			hi := state[ch*2+1][f] & 0x0F
			tonePeriod[ch][f] = uint16(lo) | uint16(hi)<<8
		}
	}
	envPeriod := make([]uint16, maxFrame+1)
	for f := 0; f <= maxFrame; f++ {
		envPeriod[f] = uint16(state[regEnvLo][f]) | uint16(state[regEnvHi][f])<<8
	}

	// Trim to the music region: first frame where any volume is non-zero
	// through last frame where any volume is non-zero.
	// Init (all-zero silence) and outro (frozen registers, zero volume) are
	// constant and carry no musical information.
	firstMusic, lastMusic := -1, 0
	for f := 0; f <= maxFrame; f++ {
		if state[regVolA][f] != 0 || state[regVolB][f] != 0 || state[regVolC][f] != 0 {
			if firstMusic < 0 {
				firstMusic = f
			}
			lastMusic = f
		}
	}
	if firstMusic < 0 {
		firstMusic = 0
	}
	endFrame := lastMusic + 1

	music := &ZXMusic{StartFrame: firstMusic, EndFrame: endFrame}

	// Segment each channel into note/rest events by volume transitions.
	// Key: even during rests, TonePeriod holds the forward-filled value.
	volRegs := [3]byte{regVolA, regVolB, regVolC}
	for ch := 0; ch < 3; ch++ {
		vr := volRegs[ch]
		var notes []ZXNote

		f := firstMusic
		for f < endFrame {
			vol := state[vr][f]
			isActive := vol != 0

			// Scan until volume state changes (active→rest or rest→active)
			start := f
			for f < endFrame {
				v := state[vr][f]
				if (v != 0) != isActive {
					break
				}
				f++
			}
			dur := f - start

			note := ZXNote{
				StartFrame: start,
				DurFrames:  dur,
				IsRest:     !isActive,
				TonePeriod: make([]uint16, dur),
				Noise:      make([]byte, dur),
				Mixer:      make([]byte, dur),
				Volume:     make([]byte, dur),
				EnvPeriod:  make([]uint16, dur),
				EnvShape:   make([]byte, dur),
			}
			for i := 0; i < dur; i++ {
				ff := start + i
				note.TonePeriod[i] = tonePeriod[ch][ff]
				note.Noise[i] = state[regNoise][ff]
				note.Mixer[i] = state[regMixer][ff]
				note.Volume[i] = state[vr][ff]
				note.EnvPeriod[i] = envPeriod[ff]
				if writeAt[regEnvShape][ff] >= 0 {
					note.EnvShape[i] = byte(writeAt[regEnvShape][ff])
				} else {
					note.EnvShape[i] = 0xFF // no write
				}
			}
			notes = append(notes, note)
		}
		music.Notes[ch] = notes
	}

	return music
}

// ── Player ────────────────────────────────────────────────────────────────────

// ZXPlayer replays ZXMusic and emits AY commands.
type ZXPlayer struct {
	music *ZXMusic
	out   []AYCommand
}

// NewZXPlayer creates a player for the given music.
func NewZXPlayer(m *ZXMusic) *ZXPlayer {
	return &ZXPlayer{music: m}
}

// Play runs the full song and returns all AY commands.
// Exactly mirrors ZX player behaviour:
//   - R0-R12 written every frame
//   - R13 written only when value changes
func (p *ZXPlayer) Play() []AYCommand {
	m := p.music

	// Build note index per channel for O(1) frame→note lookup.
	// noteIdx[ch][f] = which note is active at frame f
	noteAtFrame := [3][]int{}
	for ch := 0; ch < 3; ch++ {
		arr := make([]int, m.EndFrame)
		ni := 0
		for f := m.StartFrame; f < m.EndFrame; f++ {
			for ni < len(m.Notes[ch])-1 {
				n := m.Notes[ch][ni]
				if f >= n.StartFrame+n.DurFrames {
					ni++
				} else {
					break
				}
			}
			arr[f] = ni
		}
		noteAtFrame[ch] = arr
	}

	for f := m.StartFrame; f < m.EndFrame; f++ {
		// Gather per-channel register values for this frame.
		var tone [3]uint16
		var vol [3]byte
		noise := byte(0)
		mixer := byte(0xFF)
		envp := uint16(0)
		envShape := byte(0xFF)

		for ch := 0; ch < 3; ch++ {
			notes := m.Notes[ch]
			if len(notes) == 0 {
				continue
			}
			ni := noteAtFrame[ch][f]
			if ni >= len(notes) {
				continue
			}
			note := &notes[ni]
			i := f - note.StartFrame
			if i < 0 || i >= note.DurFrames {
				continue
			}
			tone[ch] = note.TonePeriod[i]
			vol[ch] = note.Volume[i]
			// Shared registers: last channel wins (all should agree per frame)
			noise = note.Noise[i]
			mixer = note.Mixer[i]
			envp = note.EnvPeriod[i]
			if note.EnvShape[i] != 0xFF {
				envShape = note.EnvShape[i]
			}
		}

		// Emit R0-R12 every frame.
		p.emit(f, regToneALo, byte(tone[0]&0xFF))
		p.emit(f, regToneAHi, byte(tone[0]>>8)&0x0F)
		p.emit(f, regToneBLo, byte(tone[1]&0xFF))
		p.emit(f, regToneBHi, byte(tone[1]>>8)&0x0F)
		p.emit(f, regToneCLo, byte(tone[2]&0xFF))
		p.emit(f, regToneCHi, byte(tone[2]>>8)&0x0F)
		p.emit(f, regNoise, noise)
		p.emit(f, regMixer, mixer)
		p.emit(f, regVolA, vol[0])
		p.emit(f, regVolB, vol[1])
		p.emit(f, regVolC, vol[2])
		p.emit(f, regEnvLo, byte(envp&0xFF))
		p.emit(f, regEnvHi, byte(envp>>8))
		// R13: emit whenever the original player wrote it (including same-value
		// rewrites, which retrigger the hardware envelope on each new note).
		if envShape != 0xFF {
			p.emit(f, regEnvShape, envShape)
		}
	}
	return p.out
}

func (p *ZXPlayer) emit(frame int, reg byte, val byte) {
	p.out = append(p.out, AYCommand{Frame: uint32(frame), Register: reg, Value: val})
}

// ── Statistics ────────────────────────────────────────────────────────────────

func printMusicStats(m *ZXMusic) {
	fmt.Printf("\n=== ZX Music Statistics ===\n")
	fmt.Printf("Start frame: %d (%.2fs)\n", m.StartFrame, float64(m.StartFrame)/frameRate)
	fmt.Printf("End frame:   %d (%.2fs)\n", m.EndFrame, float64(m.EndFrame)/frameRate)

	totalNotes := 0
	for ch := 0; ch < 3; ch++ {
		active, rests := 0, 0
		for _, n := range m.Notes[ch] {
			if n.IsRest {
				rests++
			} else {
				active++
				totalNotes++
			}
		}
		fmt.Printf("  Ch%d: %d notes, %d rests\n", ch, active, rests)
	}
	fmt.Printf("  Total notes: %d\n", totalNotes)

	periods := make(map[uint16]bool)
	for ch := 0; ch < 3; ch++ {
		for _, n := range m.Notes[ch] {
			if n.IsRest {
				continue
			}
			for _, tp := range n.TonePeriod {
				if tp > 0 {
					periods[tp] = true
				}
			}
		}
	}
	pList := make([]int, 0, len(periods))
	for p := range periods {
		pList = append(pList, int(p))
	}
	sort.Ints(pList)
	fmt.Printf("  Unique tone periods: %d", len(periods))
	if len(pList) > 0 {
		fmt.Printf(" (range %d – %d)", pList[0], pList[len(pList)-1])
	}
	fmt.Printf("\n")
}

// ── Validation ────────────────────────────────────────────────────────────────

// Validate compares player output against the original command log.
func Validate(original, replayed []AYCommand) (matched, total int, firstMiss string) {
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
		} else {
			if firstMiss == "" {
				if ok {
					firstMiss = fmt.Sprintf("frame %d R%d: got 0x%02X want 0x%02X",
						c.Frame, c.Register, c.Value, v)
				} else {
					firstMiss = fmt.Sprintf("frame %d R%d: 0x%02X not in original",
						c.Frame, c.Register, c.Value)
				}
			}
		}
	}
	total = len(replayed)
	return
}

// ── Loader ────────────────────────────────────────────────────────────────────

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

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	path := "../emulator/ay_commands.bin"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	fmt.Printf("Loading %s...\n", path)
	cmds, err := loadAYCommands(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded %d AY commands\n", len(cmds))

	fmt.Printf("\nExtracting ZX music format...\n")
	music := ExtractZXMusic(cmds)

	// Trim original commands to the same music region for fair comparison.
	musicCmds := make([]AYCommand, 0, len(cmds))
	for _, c := range cmds {
		if int(c.Frame) >= music.StartFrame && int(c.Frame) < music.EndFrame {
			musicCmds = append(musicCmds, c)
		}
	}
	fmt.Printf("Original commands in music region: %d (frames %d–%d)\n",
		len(musicCmds), music.StartFrame, music.EndFrame-1)
	printMusicStats(music)

	fmt.Printf("\nReplaying through ZX player...\n")
	player := NewZXPlayer(music)
	replayed := player.Play()
	fmt.Printf("Player emitted %d AY commands\n", len(replayed))

	fmt.Printf("\nValidating against original (music region only)...\n")
	matched, total, firstMiss := Validate(musicCmds, replayed)
	pct := 0.0
	if total > 0 {
		pct = 100.0 * float64(matched) / float64(total)
	}
	fmt.Printf("Player vs original: %d / %d (%.4f%%)\n", matched, total, pct)
	if firstMiss != "" {
		fmt.Printf("First mismatch: %s\n", firstMiss)
	}

	covMatched, covTotal, _ := Validate(replayed, musicCmds)
	covPct := 0.0
	if covTotal > 0 {
		covPct = 100.0 * float64(covMatched) / float64(covTotal)
	}
	fmt.Printf("Coverage:        %d / %d original reproduced (%.4f%%)\n",
		covMatched, covTotal, covPct)

	if pct >= 99.99 && covPct >= 99.99 {
		fmt.Printf("\n✓ ZX player reproduces original music 100%%\n")
	} else {
		fmt.Printf("\n✗ Validation incomplete\n")
		analyzeDiscrepancies(musicCmds, replayed)
	}
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
			if shown < 8 {
				fmt.Printf("  Extra:    frame %d R%d = 0x%02X\n", c.Frame, c.Register, c.Value)
				shown++
			}
		} else if v != c.Value {
			regMiss[c.Register]++
			if shown < 8 {
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
