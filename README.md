# CUBELON C64 Port

Port of the CUBELON ZX Spectrum 128K demo to the Commodore 64.

## Validate After Every Change (humans and AI agents)

`make -C c64port` only assembles (about 0.1 s). **After every change to the C64 sources,
the emulator or the music, run `make -C c64port validate`** (about 2 s) and do not report a change
as done unless it passes. It assembles, then:

1. runs the assertions (`assert_if` lines compare memory with `proto/golden/*.bin` at given frames);
2. compares the C64 frames with the ZX reference frames in `emulator/frames/` and requires the last
   matching frame to equal `ExpectedLastMatchingFrame` (see [Verifying Sync](#verifying-sync)).

Any failure panics. Two things to know:

- **The ZX frames are not committed** (`emulator/frames/` is gitignored). When it holds fewer than
  `ExpectedZXFrameCount` PNGs, `validate` runs the ZX emulator itself to save them (several
  seconds). To regenerate them, for example after a change to the ZX emulation, delete the folder.
  If a ZX change alters how many frames are saved, `validate` panics until you update
  `ExpectedZXFrameCount` in `emulator/framecompare/framecompare.go`.
- **Frame baseline:** if fewer frames match, fix the regression. If more match (for example once the
  morph scene is implemented), validate panics until you raise `ExpectedLastMatchingFrame` in
  `emulator/framecompare/framecompare.go` to the reported value. A deliberate change of the matching
  rules or limit (`MaxMismatchedPixels`) needs the baseline checked the same way.

Changes to the music also need the audio check (`compare_audio.go`, see
[Verifying Sync](#verifying-sync)); it is not part of `validate`. The best lag must stay 0, and
`tools/` panics on its own committed baselines (see `tools/README.md`).

## Project Structure

```
cubelon64/
├── original/          # Original ZX Spectrum TAP file and extracted data
│   ├── cubelon_final.tap
│   └── cubelon_*.bin  # Extracted code and data blocks
├── emulator/          # C64 emulator + ZX Spectrum reference emulator
│   ├── run_cubelon.go # Main runner (-c64 capture, -validate for C64 mode)
│   ├── c64/           # C64 hardware emulation (6502, VIC-II, memory)
│   ├── m6502/         # 6502 CPU emulation
│   ├── z80/           # Z80 CPU emulation (ZX reference)
│   ├── spectrum/      # ZX Spectrum hardware emulation (reference)
│   ├── sidemu/        # SID 6581 model (shared with tools/)
│   ├── audiotrace/    # Per-frame AY/SID channel traces (CSV)
│   ├── framecompare/  # C64 vs saved ZX frames check (the -validate baseline)
│   ├── compare_audio.go # ZX vs C64 audio sync check
│   └── web/           # ZX Spectrum ROMs and snapshots
├── c64port/           # C64 assembly source
│   ├── src/
│   │   ├── main.s             # Entry point, init, main loop
│   │   ├── rotation.s         # Corner rotation math (fixed 8.8)
│   │   ├── render.s           # Scanline fill + charset output
│   │   ├── charset.s          # Charset initialization
│   │   ├── sincos_tables.s    # Sin/cos lookup tables
│   │   └── include/
│   │       ├── shared.inc         # Shared variables ($0200+)
│   │       ├── render_private.inc # Render-local variables
│   │       └── zeropage.inc       # Zero page allocation
│   ├── build/
│   │   └── cubelon64.prg      # Compiled C64 program
│   ├── Makefile
│   └── c64.cfg                # cc65 linker config
├── proto/             # Go prototype of the renderer, writes proto/golden/
├── tools/             # Music/SID tools (see tools/README.md)
└── docs/              # Algorithm documentation
```

## C64 Port

### Rendering Reference (Ground Truth)

The reference is the original ZX demo run in the Go Z80 emulator:

```bash
go run -C emulator run_cubelon.go assertions.go motion_analyzer.go --no-audio
# Output: emulator/frames/frame_NNNNN_zx.png (only frames whose screen changed)
```

`proto/` reimplements the renderer and writes `proto/golden/*.bin`; `assert_if` lines in
the C64 sources compare memory against them. `make -C c64port validate` runs these assertions and
the comparison with the ZX frames. See `tools/README.md` for the full chain.

### Current Status

- Marching cubes scene (demo frames 168-779) implemented; validated against `proto/golden/` and
  matching the ZX frames up to 779
- Morph scene (squares to diamonds, demo frames 779-819) not implemented yet; the ZX
  reference frames are 801-819 with a still at 779

### Algorithm

The rotating square is rendered as a 24x24 pixel bitmap stored in a 3x3 grid of
C64 characters (chars 64-82). The render pipeline:

1. **Rotation**: Multiply corner offsets by sin/cos (fixed 8.8), add pivot
2. **Scale 2x**: Double corners for 48x48 supersample space
3. **Scanline fill**: DDA edge walking, fill spans row by row
4. **Downsample**: 2x2 -> 1 output pixel (0 subpixels=off, 1-2=medium, 3-4=full)
5. **Charset write**: Pack bits into character data (bit 7 = leftmost pixel)
6. **Restore corners**: Halve corners back to 24x24 space

### Building

**Using custom assembler (recommended):**
```bash
go run -C asm . -o c64port/build/cubelon64.prg -C c64port/c64.cfg c64port/src/all.s
```

**Or using Makefile:**
```bash
make -C c64port         # Custom assembler
make -C c64port legacy  # Legacy ca65/ld65 toolchain
```

Requires:
- Custom assembler: Go 1.21+
- Legacy: cc65 toolchain (ca65, ld65)

### Running (emulator)

```bash
make -C c64port run
```

`make -C c64port` only builds the PRG (about 0.1 s). After changes, run `make -C c64port validate`:
it builds, then runs the assertions and the comparison with the saved ZX frames (see
[Verifying Sync](#verifying-sync)), about 2 s.

Running the emulator directly:
```bash
go run -C emulator run_cubelon.go assertions.go motion_analyzer.go -validate ../c64port/build/cubelon64.prg
go run -C emulator run_cubelon.go assertions.go motion_analyzer.go -c64 ../c64port/build/cubelon64.prg
```

Flags:
- `-validate`: validate the PRG (implies `-c64`): run until every assertion has passed and compare
  with the saved ZX frames, panicking on any failure
- `-c64`: C64 capture run (default is ZX Spectrum): saves every frame as a PNG
- `-no-frames`: Skip PNG frame output
- `-save-frames=SPEC`: Only save these sequencer frames (e.g. `200-300`)
- `-audio-trace=FILE`: Write the per-frame audio CSV
- `-max-frames=N`: Run N frames total (default 9650 = full demo)

### Verifying Sync

Video: the ZX and C64 frame PNGs are named by the same frame counter (see
[Frame Numbering](#frame-numbering)), so `frame_NNNNN_zx.png` and `frame_NNNNN_c64.png` show the
same moment. `make -C c64port validate` checks this automatically against the ZX frames saved in
`emulator/frames/` (generated automatically when missing):

- Only frames that have their own ZX PNG are compared, so it does not matter how often the ZX
  updates its picture.
- Per pixel, C64 black must be ZX black and C64 light grey must be ZX white. A darker grey
  pixel is an antialiased edge: it matches when its 3x3 ZX neighbourhood holds both black and
  white pixels.
- A frame matches with at most `MaxMismatchedPixels` mismatches. The last matching frame is the
  last one before the first mismatching frame, and it must equal `ExpectedLastMatchingFrame` in
  `emulator/framecompare`. It panics when fewer frames match, and also when more match, to make
  you raise the baseline.

Audio: both emulators write a per-frame CSV of each channel's level and pitch, and
`compare_audio.go` compares them, with the AY as the golden master:

```bash
go run -C emulator run_cubelon.go assertions.go motion_analyzer.go --no-frames --audio-trace=zx.csv
go run -C emulator run_cubelon.go assertions.go motion_analyzer.go -c64 --no-frames --audio-trace=c64.csv ../c64port/build/cubelon64.prg
go run -C emulator compare_audio.go zx.csv c64.csv
```

It tries C64 lags of ±4 frames and reports the best one (0 when in sync), then per channel the
activity mismatches (sounding on one side only) and pitch mismatches (octaves ignored). Levels
are not compared: SID ADSR and AY envelopes differ, and one sample per frame cannot capture an
envelope-driven AY channel.

### Frame Numbering

A frame number is the original demo's frame counter at `$8184`. The ZX interrupt handler plays
the AY tick first and increments the counter after it, so the counter reads the number of ticks
played, one more than the 0-based tick index. Consequences:

- The song starts with 4 silent patterns of 40 ticks, so the first sound is tick 160, which
  reads as counter **161**. Bars start at 161 + 40n.
- The ZX emulator's capture frame is the counter plus 1 (one tape-loading frame), so the same
  event is **162** in the AY log and in older docs ("music begins at frame 162").
- The C64 sequencer counter matches the ZX counter. It ticks at the end of its frame, after the
  music step, so a C64 step at counter N matches the ZX step that reads N afterwards.
  `START_FRAME` (159, the original sequencer's threshold `$009F`) starts the timeline;
  `MUSIC_FRAME` (161) starts the music.
- The 40-frame animation cycle starts at 159 + 40n, 2 frames before the music bar at 161 + 40n.
  The ZX shows the same phase (motion blocks end on the same frames in both), so this looks
  inherited from the original, not introduced by the port.

## ZX Spectrum Reference

The `emulator/` directory also contains a cycle-accurate ZX Spectrum 128K emulator
used as a reference for understanding the original demo.

```bash
go run -C emulator run_cubelon.go assertions.go ../original/cubelon_final.tap
```

Outputs `frames/frame_XXXXX_zx.png` and `cubelon_audio.wav`.

## Original Demo

- **Platform**: ZX Spectrum 128K
- **Duration**: ~193 seconds (3m 13s)
- **Size**: 49KB of code and data
- **Audio**: AY-3-8912 PSG (3 channels + envelope + noise)

## Build Tools

- **C64 Build**: Custom 6502 assembler (asm/asm64) or cc65 toolchain (ca65, ld65)
- **Emulator/Tools**: Go 1.21+
- **Testing**: VICE emulator (x64sc)
