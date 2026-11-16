# CUBELON C64 Port

Port of the CUBELON ZX Spectrum 128K demo to the Commodore 64.

## Validate After Every Change (humans and AI agents)

`make -C c64port` only assembles (about 0.1 s). **After every change to the C64 sources,
the emulator or the music, run `make -C c64port validate`** (about 2 s) and do not report a change
as done unless it passes. It assembles, then:

1. runs the assertions (`assert_if` lines compare memory with `proto/golden/*.bin` at given frames);
2. compares the C64 frames with the ZX reference frames in `emulator/frames/` and requires the last
   matching frame to equal `ExpectedLastMatchingFrame` (see [Verifying Sync](#verifying-sync)). A frame
   with a ZX reference one frame before and one after (an interpolation) must not look exactly like the C64
   frame before or after it.
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
  rules or limits (`MaxBlockMismatches`) needs the baseline checked the same way.

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
  The 45 degree frames of the full grid have 8 mismatched pixels in each block of 24x24 pixels, so the limit is 8 (`MaxBlockMismatches`)
- The bowties turn into diamonds (demo frames 2039-2059, `diamond.s`): starting from the large square, the two kites of every bowtie of
  the split move apart and become 12 by 12 pixel diamonds, in one cycle of 20 states. One formula covers the whole animation: the corner B
  of a bowtie's quadrant, its tip on the axis and the angle of the edge from B to the corner, which turns by exactly 45 degrees (it decides where the waist is, and once it has
  passed the corner, where the separated kite's near vertex is)) move linearly, and `go run -C proto . diamond fit` fits the start and end
  values of B and the tip to the ZX frames (24 pixels differ in total over the 11 even states; the last state's tile is the ZX frame's, a constant, and the formula only has to fit the states between it and the large square). proto renders the 20 states at 50 Hz (the ZX demo shows the even ones, the first and last are fixed and the states next to them lie halfway) and exports their top left quarters (`diamond_data.inc`; the tile has quarter turn symmetry);
  state 0 is the morph's last tile. The 40 images lie on the spin's images, which the job overwrites once the spin is over. The frames match up to 2059.
- The diamonds drift (demo frames 2079-2099, `drift.s`): every kite of the diamonds keeps moving, rigidly, diagonally in equidistant steps
  (0.6 pixels per state), the two kites of a bowtie in opposite directions, until each is where another one was and the picture is
  the same again, in 20 states at 50 Hz. `go run -C proto . drift` fits the diamond (its vertices from the corner) and the speed
  to the ZX frames (16 pixels differ in total over the 10 even states). The tile of the odd cells of the checkerboard is the
  transpose of the tile of the even cells in both diamond scenes, so the C64 keeps one tile per state and builds the pair
  with `store_pair` (the transpose is the spin's). The drift's 40 images lie on the split's, which are dead from frame 1379
  on, and the job builds them after the diamonds'. The frames match up to 2099. The diamonds then turn back into the large square (demo frames 2119-2139, `EVENT_DIAMONDS + SCENE_BACKWARDS`, the first cycle's states 19-0 with state 0 the morph's last tile); the frames match up to 2139.
  The ZX demo then plays the three scenes again with the tiles mirrored (demo frames 2159-2259, `EVENT_DIAMONDS_MIRRORED`,
  `EVENT_DRIFT_MIRRORED`: the second run; proto checks the mirrored tiles against its frames, 0 frame mismatches). The diamonds' tile and its
  transpose are each other's mirror image, so the diamonds show their image pairs swapped (the carry of `show_state_pair`). The drift's tiles
  are different images: `drift_precompute_mirrored` converts the same tiles as mirror images (`store_pair` with the carry set), which
  the job builds after the first drift (event at frame 2100) over the images that are dead by then. The frames match up to 2259.
  The six scenes (first run, second run) are then played once more and the first run a third time (demo frames 2279-2619, scene table entries only,
  and one rebuild of the drift's images before each drift, alternately of the plain and the mirrored ones: the events at frames 2220, 2340 and 2460,
  the states 21 of the cycles 51, 54 and 57). The frames match up to 2619.
- The squares merge (demo frames 2679-2699, `merge.s`): the large square, a square of 8 pixels in each corner of a cell, divides into four
  squares in every cell, which shrink by half a pixel a state and move on straight lines in a pinwheel around the cell's centre, until
  they are a pixel large; then four squares grow at the same places, a pixel a state, and merge into the 12 pixel square of the marching cubes'
  last picture (the cycle before is empty: `SCENE_PAUSE`). The squares are drawn from their centres and sizes, so `go run -C proto . merge` needs
  no fit (0 pixels differ in all 11 even states); all cells show the same tile, which has quarter turn symmetry, so the C64 keeps a quarter
  (`merge_data.inc`, in the `TILEDATA` segment above the VIC bank) and builds the tile with the diamonds' routine. The 20 images lie on the
  split's, and the job builds them after the last drift (event at frame 2580). The scene ends at frame 2699, where the frames are identical; the next scene of the ZX demo starts at 2721, where the C64 still holds the picture and the check that the C64 picture must change flags it
- The rotating cubes (demo frames 2721-3189, `cube_render.s`, `cube_build.s`, `cube_show.s`; the ZX scene's 8x8 blocks each play the same
  220 frame loop, 10 frames later per ring from the centre): `go run -C proto . cube` writes the 8 vertices of 56 states (`cube_data.inc`), the unrolled and tabled parts of `cube_show.s` (`cube_show_data.inc`), the memory areas of the images with their assertions (`cube_layout.inc`, `cube_asserts.inc`) and the 110 images
  of the loop's first half (`proto/golden/cube_images.bin`, 104 bytes each: only the rows that are ever not black, 16, 18 and 16 bytes of the
  three char columns and 18 sprite rows); the second half is the same turned by 180 degrees. The job renders the images
  (`cube_render_image`: slope edges with an 8.8 slope per edge, spans per subpixel row, pattern counts per pixel summed over the faces, then 3-4 light grey,
  1-2 dark grey) in its idle time, about 3 frames each, so they are built long before the scene, in the order the scene needs them (0, 2-109, 1), and the assertions
  in `cube_build_next` compare them all with the golden file. The job cannot keep up with the scene itself, so the images wait in memory no scene uses:
  free RAM under the KERNAL, the end of the tile pages, the unused chars, and the scene's own ring buffer, whose images `cube_move` copies onto the
  split's images on frames 2700-2709, once the merge is over. The code, the vertices and the scene's variables live in free VIC bank RAM
  (`CUBECODE`, `CUBEDATA`, `CUBEBSS` in `c64.cfg`, loaded as part of the PRG). The scene runs the simple sprite multiplexer: kind
  k = ring k has its chars at `CUBE_CHARS`, and `cube_prepare` (frame 2710, invisible: the chars are new) fills them and stages the first image, `cube_setup` (frames 2719 and 2720, 4 tile rows each, invisible because every block shows the same square) writes the screen's codes. Every step
  (`cube_step`) copies the chars of the four rings from a ring buffer of 32 slots, 10 frames apart, and points the sprites of each ring at the sprite block
  of its slot (`point_sprites`); then it stages the next frame's image into the buffer, turned in the second half of the loop
  (`stage_turned`). 32 slots, one more than the rings reach back, so that the slot a step writes is never one the sprites are showing. A ring that has not
  started shows image 0. The frames match up to 3189.
- The marching cubes come back (demo frames 3199-3339, four cycles: `SCENE_CUBES` twice forwards, then twice backwards). The rotating cubes end as the grid of squares the marching cubes start from, whose chars and sprite blocks are still in memory, so the 8 steps of `EVENT_CUBES_RETURN` (the freeze frames 3190-3197, one grid row each) only write the row's screen codes with the reveal's `write_cube_column` and, in the last step, the marching cubes' `SIMPLE_POINTERS` with `fill_marching_pointers` (the multiplexer's own); the simple multiplexer keeps running until the cycle starts at 3199. The frames match up to 3339, where the ZX demo goes on with the morph's petals (cycle 80). The in-between limit of 8 pixels only holds for the rotating cubes (`CubeScenesEndFrame`)
- The flowers (demo frames 3359-3939, `flower.s`, `EVENT_FLOWER` in the cycles 80-94, the entries after the four marching cube cycles of the scene table):
  a loop of cycles of 40 frames, a state of each every second frame like the ZX demo, and a rendered state in between (the ZX holds a picture for two
  frames, but the frame comparison requires that no frame is exactly the C64 frame before or after it where the ZX has a picture before and after):
  the square turns into the flower (the morph), the flower marches twice, and turns back into the square. `go run -C proto . flower` fits sweeps of the
  petals' ellipses to the ZX frames and writes one record of 18 bytes per state (`flower_data.inc`: the fading square's size, row and shade, three petal
  shapes and the offset of one petal of each) and the golden images; `go run -C proto . flower cycles` checks the whole loop against the 143 ZX frames
  (at most 4 mismatches in a tile) and that every state in between is made of what the states around it show. The kinds of cycle: the morph (the images
  of the even and the odd states), the morph backwards (the same images mirrored), and the two marching kicks. The flower repeats every 60 degrees and the
  flower turned the other way is its flip, so the marching kicks have ten turns of which five are rendered (without a shift, `flower_march_data.inc` has
  their records and shifts) and rolled to the shift of each step, the even rows' tile to the right and the odd rows' tile its mirror image (exact);
  the kinds of rows swap their tiles every four cycles (the carry of the pair show). The ZX demo's dissolve of the flowers after frame 3957 is not done
  yet: the last flower holds. The C64 renders the images in the job with a subpixel renderer: a petal is a row of chords (centre moving by the
  shear, half width from the half circle table), set into 48 bit masks per sample row, merged with the square and packed by bit planes
  (`flower_levels`). The second three petals are the first three turned by half a turn (the model rounds the shear symmetrically, so the turned rows
  are exact), which the packing ORs in from the reversed rows; with a shift of the whole flower (`FLOWER_SHIFT`) all six petals are drawn and wrap
  around the tile. The renderer takes about 340000 cycles a tile, so the job builds in the order of need: the images of the even and odd states
  and the turns' tiles from frame 2660 on (`EVENT_BUILD` in the cycle 62, once the diamonds' images are over), and from frame 3198 on (the last
  `EVENT_BUILD`, when the rotating cubes' memory is free: the tile area above the split's images, the RAM under the KERNAL and the cubes' ring buffer)
  the marching pairs and the backwards morph's pairs, mirrored from the forward ones just before the first backwards cycle needs them. The job runs until raster
  line `JOB_DEADLINE_LINE` of each frame, which was raised for this (all frames must stay within the main loop's budget: `validate` panics on an overrun).
  The tile scenes' scratch variables (`CONVERT_*`, `IMAGE_BASE` of tiles.s) lie in the middle of the tile area, so the turns' tiles start after them.
  The code does not fit the free RAM1, so it lies in the gaps of the VIC bank (the screens B and D, whose only used bytes are the sprite pointers, `$6900`,
  the free end of the cube code, the free end of the cube vertices) and in the end of RAM1: `c64.cfg` lists the regions as the pool of the segments
  `FLOWER01` and up (`load = A|B|C`), which the assembler fills, the largest first. Every piece (a segment, one or a few procedures, in flower.s) must end with
  `rts` or `jmp` and branch only within itself. A frame the main loop cannot afford
  twice stays safe by keeping the steps of the job short (`job_checkpoint` every few hundred cycles).
- Startup precomputes only the 11 cube frames (about 1 s); the images of the morph, split, spin and diamonds
  are built by a background job (`job.s`) that runs in the idle time of the main loop, as fast as that allows
  (the morph takes about 175 frames, the split 340, the spin 80 and the diamonds 70). The sequencer's event table
  asks for each build in time (`EVENT_BUILD`, handled by `build_request`, which counts the requests; the job
  builds the next of `build_routines` for each): the morph, split and spin at the start of the timeline, long before their scenes, the diamonds
  at frame 1389, once the spin is over, because the diamonds' images lie on the spin's. A scene's images have a home, and the job
  overwrites what is not needed any more. The job is a coroutine with its own stack
  (`$0100-$017F`); the main loop resumes it while it waits for `FRAME_TICK`. Routines of the job call
  `job_checkpoint` between steps; it hands control back once the raster has passed
  `JOB_DEADLINE_LINE` or the tick has come, so the main loop's work starts on time. Keep the steps
  between checkpoints short enough to end before line 240, and keep the job's variables away from the
  main loop's (the rasterizer renders into `BUILD_SLOT`, not `FRAME`). A build that is late shows as wrong frames in
  `validate`.
- All three tile scenes share `tiles.s`: a tile is converted once at init into an image (144
  bytes: the three char columns, the sprite's main block and its partner block, in the order
  the hardware wants them), so showing two tiles is two block copies of about 3.9k cycles. The
  morph shows one image as both kinds of tile (it is its own mirror image), the split its
  image and the mirror image, the spin the images of state N and 20 - N. Copying the tile
  row by row through buffers took 11k cycles, which overran the vertical blank and halved the
  frame rate, so profile a new scene (see [Profiling](#profiling)) and keep a step well under
  7k cycles. The images, 11.7 KB, and the init scratch buffers live in the TILES region of
  `c64.cfg`, scene data in RAM2; low RAM has about 500 bytes free below the VIC bank
- Two sprite multiplexers share the raster interrupt chain. The gapless one (`square_sprite_mux.s`) shows all 24
  rows of a tile: Y-expanded last rows, a partner sprite and four screens switched in exact cycles. The simple one
  (`simple_sprite_mux.s`, `irq_entry_simple`) is for tiles that fit the 21 rows of a standard sprite, such as the marching cubes'
  squares (rows 1-17): one interrupt per band on the sprite's last line moves the sprites and sets their pointers from a
  table by band and sprite (`SIMPLE_POINTERS`), every band is shown from screen A, and the music's waveform is written between
  the stores. The writes end about 100 cycles before the data fetch they must precede (`c64port/experiments/cube_mux.s`
  measures it in VICE). `mux_select` switches before each step of the main loop, by `sequencer_shows_cubes`
- The C64 emulator models the VIC-II at cycle level where raster effects need it: sprite DMA
  (MCBASE and the Y expansion flip-flop), the cycles the VIC takes from the CPU on bad lines and
  for sprite DMA, and VIC register writes on the cycle they happen. A probe of the sprite
  multiplexer renders pixel for pixel like VICE (x64sc)

### Memory map

| Range         | Use                                                                                                                                                                             |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `$0801-$3FFF` | `RAM1`: BASIC stub, main code, the music (about 200 bytes free)                                                                                                                 |
| `$4000-$4FFF` | screens A-D (B and D only hold sprite pointers while the gapless multiplexer runs; the flower renderer lies in their free bytes)                                                |
| `$5000-$5CFF` | `CUBECODE`: the cube scene's code and tables, then flower pieces                                                                                                                |
| `$6000-$61E0` | chars of the scenes; the cubes' chars from `$60C0` (codes 24-59)                                                                                                                |
| `$6200-$66DF` | 12 cube images until the scene, then free                                                                                                                                       |
| `$6700`       | the cubes' sprite block for image 0 (pointer `$9C`)                                                                                                                             |
| `$6800-$68FF` | the sprites of the tile scenes                                                                                                                                                  |
| `$5D00-$5FFF`, `$6900-$69FF` | pieces of the flower code (the pool of `c64.cfg`)                                                                                                                  |
| `$6A00-$6D7F` | `CUBEDATA`: the cube vertices                                                                                                                                                   |
| `$6E00-$77FF` | `CUBEBSS`: counts, edge rows, image table, the ring buffer's chars (15 images until the scene)                                                                                  |
| `$7800-$7FFF` | `CUBESPRITES`: the ring buffer's sprite blocks (19 images until the scene)                                                                                                      |
| `$8000-$87FF` | `RAM2`: scene data (about 500 bytes free)                                                                                                                                       |
| `$8800-$CFFF` | `TILES`: the tile scenes' images; from frame 2700 the cube images moved out of the ring buffer lie on the split's; 20 cube images at `$C780`; the flower's images on the spin's |
| `$E000-$F1E0` | 44 cube images (`CUBEHIGH`), then `BSS` (bit reverse table, marching cubes' frames, variables)                                                                                  |

The assembler stops with "label moved between the passes" when an instruction's operand is a zero page symbol that is
defined further down in the source: define zero page variables before their first use.

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
- Per pixel, C64 black must be ZX black and C64 light grey must be ZX white, or it counts as a mismatch. A darker grey
  pixel is an antialiased edge: it matches when its 3x3 ZX neighbourhood holds both black and white pixels, and
  counts as half a mismatch when it does not.
- A frame matches when none of its blocks of 24x24 pixels, the cells of the grid, has more than
  `MaxBlockMismatches` (8, what the marching cubes at 45 degrees need) mismatches, and when it
  differs from the C64 frame compared before it: the ZX run saves a frame only when its picture changed,
  so the C64 picture must change too. The last matching frame is the
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
- **Testing**: VICE emulator (x64sc). The project's emulator starts with RAM of zeros, VICE and a real C64 do not, so a
  variable that is read before anything wrote it can pass `validate` and still crash the demo. Run the PRG in VICE with
  different RAM patterns after adding or moving variables (`x64sc -raminitstartvalue 255 -raminitvalueinvert 65536
-raminitpatterninvert 65536 -raminitrandomchance 0 -warp -limitcycles 16000000 -exitscreenshot out.png build/cubelon64.prg`
  with the start value 0 and 255 as well as the default pattern); a black screenshot means it crashed at boot. The rasterizer's
  `job_checkpoint` reads the job's state, so `job_init` runs before `precompute_frames`
