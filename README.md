# CUBELON C64 Port

Port of the CUBELON ZX Spectrum 128K demo to the Commodore 64.

## Project Structure

```
cubelon64/
├── original/          # Original ZX Spectrum TAP file and extracted data
│   ├── cubelon_final.tap
│   └── cubelon_*.bin  # Extracted code and data blocks
├── emulator/          # C64 emulator + ZX Spectrum reference emulator
│   ├── run_cubelon.go # Main runner (--c64 flag for C64 mode)
│   ├── c64/           # C64 hardware emulation (6502, VIC-II, memory)
│   ├── m6502/         # 6502 CPU emulation
│   ├── z80/           # Z80 CPU emulation (ZX reference)
│   ├── spectrum/      # ZX Spectrum hardware emulation (reference)
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
├── tools/             # Reference tools (see tools/README.md)
│   ├── generate_comparison_with_reference.go  # THE rendering reference/ground truth
│   └── generate_reference_24x24.go            # Generate reference PNG at given angle
└── docs/              # Algorithm documentation
```

## C64 Port

### Rendering Reference (Ground Truth)

```bash
go run tools/generate_comparison_with_reference.go
# Output: emulator/frames/comparison_with_reference.png
```

13-column PNG: col 0 = 16×16 supersampled float (blue border), cols 1-12 = 12 rendering
variants sorted by quality. Tests 11 frames at 9° steps (0°–90°). This is the authoritative
reference for verifying C64 output correctness.

### Current Status

- Rotation math (fixed 8.8 sin/cos multiply) working correctly
- Scanline DDA edge-walker + 2x supersampled fill working
- Charset-mode output (3x3 chars = 24x24 pixels) working
- Angles 0, 1, 2 verified pixel-perfect via assertions in `emulator/run_cubelon.go`

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
make -C c64port new   # Custom assembler (produces identical output to ca65)
make -C c64port       # Legacy ca65/ld65 toolchain
```

Requires:
- Custom assembler: Go 1.21+
- Legacy: cc65 toolchain (ca65, ld65)

### Running (emulator)

```bash
make -C c64port run
```

Or directly:
```bash
go run -C emulator run_cubelon.go assertions.go -c64 ../c64port/build/cubelon64.prg
```

Flags:
- `-c64`: C64 mode (default is ZX Spectrum)
- `-no-frames`: Skip PNG frame output
- `-max-frames=N`: Run N frames total (default 20000)

## ZX Spectrum Reference

The `emulator/` directory also contains a cycle-accurate ZX Spectrum 128K emulator
used as a reference for understanding the original demo.

```bash
go run -C emulator run_cubelon.go assertions.go ../original/cubelon_final.tap
```

Outputs `frames/frame_XXXXX.png` and `cubelon_audio.wav`.

## Original Demo

- **Platform**: ZX Spectrum 128K
- **Duration**: ~193 seconds (3m 13s)
- **Size**: 49KB of code and data
- **Audio**: AY-3-8912 PSG (3 channels + envelope + noise)

## Build Tools

- **C64 Build**: Custom 6502 assembler (asm/asm64) or cc65 toolchain (ca65, ld65)
- **Emulator/Tools**: Go 1.21+
- **Testing**: VICE emulator (x64sc)
