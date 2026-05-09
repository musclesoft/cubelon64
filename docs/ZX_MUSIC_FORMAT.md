# ZX Spectrum Music Format — CUBELON

Reverse-engineered from the CUBELON ZX Spectrum 128K demo via emulator execution tracing.
All details verified against 123,027 AY register writes captured from a full playback run.

---

## Hardware

- **CPU**: Z80 @ 3.546895 MHz
- **Sound chip**: AY-3-8912
- **Frame rate**: 50 Hz (PAL interrupt)

**AY-3-8912 registers:**

| Reg | Name | Notes |
|-----|------|-------|
| R0–R1 | Channel A tone period | 12-bit |
| R2–R3 | Channel B tone period | 12-bit |
| R4–R5 | Channel C tone period | 12-bit |
| R6 | Noise period | 5-bit |
| R7 | Mixer control | tone/noise enable per channel |
| R8–R10 | Volume | 4-bit per channel |
| R11–R12 | Envelope period | 16-bit |
| R13 | Envelope shape | written with RRCA encoding (see below) |

---

## Player Architecture

The interrupt handler fires at 50 Hz. Two routines run each frame:

**Channel processor** (`$7D30–$7DFC`, runs every frame):
1. Bank-switches to the active music bank (`$7D35` → port `$7FFD`)
2. Runs 14 channel processors — one per AY register:
   - Ch 0–12: `OUTI` reads one byte from bank memory → writes to AY R0–R12
   - Ch 13: `LD A,(HL); INC HL; RRCA; JR C,skip; OUT (C),A` — writes RRCA(byte) to R13 only when the original byte is even

**Tick routine** (`$7EC5`, runs every frame, acts on tick=0):
1. Decrements tick counter at `$7EC4`; returns early if non-zero
2. Reads next byte from the sequence at `$C000` in the active bank
3. If `$FF`: XORs `$7D35` with 2 (toggles bank 1↔3), resets sequence pointer to `$C000`
4. Otherwise: computes pattern address via RRCA×3, loads tick count and 14 channel pointers

### OUTI mechanism

`OUTI` semantics: `val = Mem.Read(HL); B--; IOOut16(BC, val); HL++`

Before the tick routine fires, `HL` for each channel is pre-loaded from a self-modifying
`LD HL,nn` at `$7D44`. After `OUTI`, the incremented `HL` is saved back to `$7D45` for
the next frame. This makes each channel pointer auto-advance one byte per frame.

### Ch13 RRCA encoding

Raw byte `b` → written value `(b >> 1) | ((b & 1) << 7)`.
Write is skipped when `b & 1 == 1` (odd byte). This gives the composer a way to pad the
R13 stream without triggering envelope resets.

### Pattern address formula

Given a sequence byte `idx`, the pattern address is computed by rotating `idx` right
three times (RRCA×3), then:

```
addr = ((rotated & 0x1F) + 0xC4) << 8 | (rotated & 0xE0)
```

This creates a sparse 32-entry × 32-byte layout within `$C400–$CFFF`.

### Pattern record (29 bytes)

```
[0]      tick_count       frames this pattern runs (written to $7EC4)
[1–2]    ch0  ptr lo/hi   initial ZX address for channel 0
[3–4]    ch1  ptr lo/hi
...
[27–28]  ch13 ptr lo/hi
```

Each channel pointer is a 16-bit address within the active bank (`$C000–$FFFF`).
The 14 pointers are loaded using a `POP HL` sequence with SP temporarily set to HL+1.

---

## Song Structure

The song runs from frame 2 to frame 9422 (~185 seconds). Bank 1 is physically reused:
at frame 2 the decompressor has only filled a 4-pattern silence block; by frame 802 it
has expanded all 432 full-song patterns into the same bank.

| Phase | Frames | Bank | Patterns | Notes |
|-------|--------|------|----------|-------|
| 1 | 2–81 | 1 (early) | 4 silence | Bank 1 pre-decompression |
| 2 | 82–801 | 3 | 36 (4 silence + 32 music) | Intro music begins at frame 162 |
| 3 | 802–9422 | 1 (full) | 432 | Complete song |

Frames 2–161 are entirely silent: both bank 1 and the opening 4 patterns of bank 3
write `R7=0x40` (all channels disabled) with zero tone periods and zero volumes.
The player is running the whole time — the silence is composed, not an absence of playback.

### Bank switching

`$7D35` in bank 5 holds the active music bank number (1 or 3).
Each interrupt: `LD A,($7D35); OR mask; OUT ($7FFD),A` maps that bank to `$C000–$FFFF`.
On sequence `$FF`: `XOR 2` toggles 1↔3 and the sequence pointer resets to `$C000`.

---

## Data Files

Three compact sparse binary files hold the actually-read byte ranges from each phase.
Bytes that were never read during playback are excluded — gaps within a bank remain zero.

| File | Size | Runs | Raw bytes | Notes |
|------|------|------|-----------|-------|
| `emulator/music_bank1_intro.bin` | 108 B | 3 | 94 B | Phase 1 |
| `emulator/music_bank3_mid.bin` | 1,810 B | 30 | 1,688 B | Phase 2 |
| `emulator/music_bank1_main.bin` | 14,016 B | 257 | 12,986 B | Phase 3 |

Total: 15,934 B vs 49,152 B for three raw 16 KB dumps (32.4%).

### Sparse binary format

```
[num_runs : u16 LE]
for each run:
    [zx_addr : u16 LE]   address in ZX address space ($C000–$FFFF)
    [len     : u16 LE]   byte count
    [data    : len bytes]
```

Bytes not covered by any run are zero (never read, never matter for playback).

---

## Go Player

`tools/zx_player/zx_music_format.go` implements a complete standalone player that
reproduces all 123,027 AY commands with 100% accuracy against the emulator log.

It loads the three sparse files, runs each phase in sequence using `ZXPlayer.Play()`,
and validates against `emulator/ay_commands.bin`.

```
✓ ZX player reproduces original music 100%
```

---

## Music Archaeology — Unused Patterns

### bank1_intro (frames 2–81)

Patterns 0x01 and 0x02 exist in the table but are never referenced. Their channel
pointers land at `$4040` (RAM5, not music bank) — the decompressor had not written
real data there at frame 2. Not real music.

### bank3_mid (frames 82–801)

25 patterns (0x1C–0x36) are unreferenced. Most have channel pointers where both bytes
equal the index byte (e.g. `$1C1C`, `$6565`) — the signature of uninitialized memory
that the decompressor filled with pattern-table bytes but never wrote actual pointer data
into. Musical garbage.

### bank1_main (frames 802–9422)

Three unreferenced patterns with real-looking content:

| idx | addr | tick | status |
|-----|------|------|--------|
| 0xFF | $C3E0 | 189 (3.8s) | **Structurally unreachable** — the Z80 `CP $FF; JR NZ` check at `$7EDC` consumes this value as an end-of-sequence sentinel before it can be used as a pattern index |
| 0xFE | $C3C0 | 96 (1.9s) | Several valid channel pointers into the main data area; ch3 data: `14 44 F6 C6 EC 37 EA 7B` — likely cut/leftover content |
| 0xFD | $C3A0 | 248 (5.0s) | ch9 at `$C994` has `B0 B2 B4 B6 B4 BD BD…`; channels 7–8 have out-of-bank pointers suggesting partial authoring |

0xFE is the most interesting: multiple channels point into valid music data with note-like
byte values, suggesting it was composed and later removed from the sequence.

---

## Pattern Reuse Statistics

Every pattern has a fixed tick count of 20 frames (0.4s). Variation comes entirely from
having many distinct patterns rather than combining fewer patterns at different durations.

| Phase | Sequence entries | Unique patterns | Reuse factor | Duration |
|-------|-----------------|-----------------|--------------|----------|
| bank1_intro | 4 | 1 | 4.00× | 1.6s |
| bank3_mid | 36 | 28 | 1.29× | 14.4s |
| bank1_main | 432 | 253 | 1.71× | 172.8s |

**bank1_main breakdown:**

- 158 patterns used exactly once (62%)
- 63 patterns used twice
- 32 patterns used 3 or more times
- Pattern 0xFC is the most reused: appears **12 times** (4.8s total) — likely a recurring chorus or riff anchor
- Patterns 0x1D–0x27 each appear exactly 5 times, suggesting a repeating section that cycles through the song

**bank3_mid:** mostly linear — 22 of 28 patterns are one-shots. The silence pattern 0x00
reappears 4 times as the opening buffer before the intro music begins.

---

## Memory Map

```
$7D35         Active music bank number (1 or 3), in bank 5
$7D44–$7D46   Self-modifying LD HL,nn for ch0 pointer
$7D45         Ch0 current pointer (lo byte of LD HL operand)
$7D52         Ch1 current pointer
$7D5F         Ch2 current pointer
$7D6C         Ch3 current pointer
$7D79         Ch4 current pointer
$7D86         Ch5 current pointer
$7D93         Ch6 current pointer
$7DA0         Ch7 current pointer
$7DAD         Ch8 current pointer
$7DBA         Ch9 current pointer
$7DC7         Ch10 current pointer
$7DD4         Ch11 current pointer
$7DE1         Ch12 current pointer
$7DED         Ch13 current pointer
$7EC4         Tick counter
$7EC5–$7F9E   Tick routine (218 bytes)
$8186         Bank control mask (ORed with $7D35 before port write)
$C000–$C1AF   Pattern sequence (up to 432 bytes, ends with $FF)
$C400–$CFFF   Pattern table (3,072 bytes, 32-entry sparse layout)
```
