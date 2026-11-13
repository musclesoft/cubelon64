# CUBELON C64 Port

Port of the CUBELON ZX Spectrum 128K demo to the Commodore 64.

## Validate After Every Change (humans and AI agents)

`make -C c64port` only assembles (about 0.1 s). **After every change to the C64 sources,
the emulator or the music, run `make -C c64port validate`** (about 2 s) and do not report a change
as done unless it passes. It assembles, then:

1. runs the assertions (`assert_if` lines compare memory with `proto/golden/*.bin` at given frames);
2. compares the C64 frames with the ZX reference frames in `emulator/frames/` and requires the last
   matching frame to equal `ExpectedLastMatchingFrame` (see [Verifying Sync](#verifying-sync)).
3. panics on a frame overrun: once the sequencer runs, its counter must advance at the start of
   every PAL frame. If the main loop needs two frames for a step, the demo and the music run at half
   speed even though the frames still match; find the slow routine with [Profiling](#profiling).

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
- Morph scene (squares to diamonds, demo frames 800-819) implemented: `morph.s` renders the kite
  of each of the 20 states with the cube rasterizer (`render_quad`, a division per edge) at init,
  folds it into a 24x24 tile (checked against `proto/golden/morph_tiles.bin`) and copies one tile
  per frame to the charset and sprites. The marching cubes stop at frame 780. The morph frames
  match the ZX frames up to 819
- Split scene (the large square splits into two kites, demo frames 840-859) implemented. The
  scenes are named after what they do: the marching cubes, the `morph` (`morph.s`, kites in
  the corners of a tile) and the `split` (`split.s`). From the split on the grid is a
  checkerboard of the tile and its mirror image (`checker_step`, four steps from frame 820, swaps every odd
  tile column's chars and every other sprite pointer); each tile corner is the centre of a
  bowtie of two kites. `split.s` renders the bowtie's quadrant, standing and transposed, with
  the cube rasterizer at init and mirrors the two into the four corners of a tile (checked
  against `proto/golden/split_tiles.bin`; `go run -C proto . split` regenerates it and
  `split_table.inc`). The split frames match the ZX frames up to 859
- Spin scene (the bowties spin a quarter turn, demo frames 880-899) implemented. The bowtie of
  the split rotates rigidly 4.5 degrees per state, the upright ones one way and the lying ones
  the other, so a tile's neighbour is its transpose. The tile of state 20 - N is the transpose of
  state N and every tile has 180 degree symmetry, so `go run -C proto . spin` renders only the
  states 0-10, fitted to the ZX frames, and ships rows 0-11 of each (`spin_data.inc`, 792
  bytes); `spin.s` rebuilds the rest at init (checked against `proto/golden/spin_tiles.bin`).
  The spin frames match the ZX frames up to 899.
- The spin goes on (demo frames 920-1099): three more quarter turns, each a mirror of the first (`spin_step_mirrored`,
  `SPIN_MIRRORED_IMAGES` built from every spin tile with `store_mirrored_image`), the first and the repeats
  identical. After a pause everything plays backwards (frames 1120-1419): the mirrored and plain quarters, then
  the split and the morph, down to the marching cubes' last frame, which `keep_squares` saves as the morph's state 0 at
  frame 780. The sequencer's scene table has one byte per 40 frame cycle from the morph's: the event, `SCENE_BACKWARDS` (states 19-0), `SCENE_CHECKER` and `SCENE_UNCHECKER`. The frames match up to 1419
- The marching cubes come back (demo frames 1439-1759, `SCENE_CUBES`, eight cycles, the plain animation without the reveal). The backwards morph's cycle undoes the checkerboard in its
  held states 21-24 (`checker_step` run as `EVENT_UNCHECKER`: bands of one kind of tile again, the sprite pointers reset). The frames match up to 1739.
  The cubes then walk backwards (frames 1759-1999, six cycles, `SCENE_CUBES + SCENE_BACKWARDS`: the animation frame is 40 - state in even phases, 20 - state in odd ones),
  matching up to 1979. The morph plays again from 1999 (`EVENT_MORPH + SCENE_CHECKER`, frames identical to the first morph's), matching up to 2019.
  The 45 degree frames of the full grid have 512 mismatched pixels, so `MaxMismatchedPixels` is 512
- The demo starts over on the first 40 frame cycle start after the last scene (the sequencer
  publishes `EVENT_RESTART` when its scene table has run out): the sprite data and the sprite pointers
  are initialized again and the sequencer starts its timeline over (`sequencer_restart`: phase, state,
  events and scenes). The frame counter goes on and never goes back, so every frame has its own
  number in all loops. The precomputed data stays and the music plays on
  (`EVENT_MUSIC` starts it only once, `MUSIC_ON`). The restart takes more than a frame, but
  `FRAME_TICK` counts the frames passed, so the main loop catches up and the demo stays in step
  with the music (the validator does not report the restart frame, the frame after it and the first timeline frame as overruns).
- Startup precomputes only the 11 cube frames (about 1 s); the images of the morph, split and spin
  are built by a background job (`job.s`) that runs in the idle time of the main loop and is done
  around frame 770, before the morph at 800. The job is a coroutine with its own stack
  (`$0100-$017F`); the main loop resumes it while it waits for `FRAME_TICK`. Routines of the job call
  `job_checkpoint` between steps; it hands control back once the raster has passed
  `JOB_DEADLINE_LINE` or the tick has come, so the main loop's work starts on time. Keep the steps
  between checkpoints short enough to end before line 240, and keep the job's variables away from the
  main loop's (the rasterizer renders into `BUILD_SLOT`, not `FRAME`).
- All three tile scenes share `tiles.s`: a tile is converted once at init into an image (144
  bytes: the three char columns, the sprite's main block and its partner block, in the order
  the hardware wants them), so showing two tiles is two block copies of about 3.9k cycles. The
  morph shows one image as both kinds of tile (it is its own mirror image), the split its
  image and the mirror image, the spin the images of state N and 20 - N. Copying the tile
  row by row through buffers took 11k cycles, which overran the vertical blank and halved the
  frame rate, so profile a new scene (see [Profiling](#profiling)) and keep a step well under
  7k cycles. The images, 11.7 KB, and the init scratch buffers live in the TILES region of
  `c64.cfg`, scene data in RAM2; low RAM has about 500 bytes free below the VIC bank
- The C64 emulator models the VIC-II at cycle level where raster effects need it: sprite DMA
  (MCBASE and the Y expansion flip-flop), the cycles the VIC takes from the CPU on bad lines and
  for sprite DMA, and VIC register writes on the cycle they happen. A probe of the sprite
  multiplexer renders pixel for pixel like VICE (x64sc)

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

To see where a frame differs, save the C64 frames and run `framediff`; it counts mismatches by the
validator's rule, in total and per 24x24 tile, and can write an image of the C64 frame, the ZX frame
and the mismatches side by side:

```bash
go run -C emulator run_cubelon.go assertions.go motion_analyzer.go -c64 -save-frames=855-860 ../c64port/build/cubelon64.prg
go run -C emulator ./framecompare/cmd/framediff -image=diff.png 855 857 859
```

To analyse a new scene, `go run -C proto . analyze 881 883` describes the periods and symmetries of
ZX frames and whether they are a checkerboard of a tile and its mirror image or transpose.

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

### Profiling

`-profile=FROM-TO` (C64 mode) prints the cycles per frame of each routine over the given
sequencer frames, including the cycles the VIC takes from the CPU. A frame has 19656 cycles;
the main loop waits for `FRAME_TICK`, which the raster interrupt of the last sprite band (line 240)
counts up, and must finish its work before the next one or a sequencer step takes two frames (check
that `Profile over N frames` is N for N frames; the main loop catches up, but the validator panics). The same chain of raster interrupts runs the music: the
step after the first band's interrupt (line 72), the SID update after the next to last (line 216).
Each of these tasks must end before the next interrupt, 24 lines later.

```bash
go run -C emulator run_cubelon.go assertions.go motion_analyzer.go -c64 -no-frames -max-frames=1100 -profile=845 ../c64port/build/cubelon64.prg
```

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
