# CUBELON C64 Port

Port of the CUBELON ZX Spectrum 128K demo to the Commodore 64.

## Project Structure

```
cubelon64/
├── original/          # Original ZX Spectrum TAP file and extracted data
│   ├── cubelon_final.tap
│   └── cubelon_*.bin  # Extracted code and data blocks
├── emulator/          # ZX Spectrum emulator (reference implementation)
│   ├── run_cubelon.go
│   ├── z80/           # Z80 CPU emulation
│   ├── spectrum/      # ZX Spectrum hardware emulation
│   └── web/           # ZX Spectrum ROMs and snapshots
├── src/               # C64 source code (to be created)
├── tools/             # Conversion and build tools (to be created)
├── Makefile           # C64 build system
└── c64.cfg            # cc65 linker configuration
```

## ZX Spectrum Emulator (Reference)

The `emulator/` directory contains a cycle-accurate ZX Spectrum 128K emulator in Go, used as a reference for understanding the original demo:

- **Z80 CPU**: Full instruction set with cycle-accurate timing
- **Memory**: 128K banking, ROM loading
- **Audio**: Cycle-exact AY-3-8912 sound chip emulation
- **Display**: Screen capture every 200 frames

### Running the Emulator

```bash
cd emulator
go run run_cubelon.go ../original/cubelon_final.tap
```

Outputs:
- `frames/frame_XXXXX.png` - Screen captures
- `cubelon_audio.wav` - 3-channel audio (AY channels A, B, C)

The emulator auto-detects demo end (5 seconds of no AY activity).

## Original Demo

- **Platform**: ZX Spectrum 128K
- **Duration**: ~193 seconds (3m 13s)
- **Size**: 49KB of code and data
- **Audio**: AY-3-8912 PSG (3 channels + envelope + noise)

## C64 Port (To Do)

See the implementation plan in `~/.claude/plans/` for detailed porting strategy.

## Build Tools

- **Emulator**: Go 1.21+
- **C64 Build**: cc65 toolchain (ca65, ld65)
- **Testing**: VICE emulator
