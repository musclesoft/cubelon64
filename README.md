# CUBELON 64

A Commodore 64 port of the ZX Spectrum demo CUBELON.

## Status

🚧 **Work in Progress** - Currently in Phase 1: Analysis and Understanding

✅ **Working ZX Spectrum Emulation**: The demo runs correctly at http://localhost:8080
- Plays for 3 minutes with continuous music
- All graphics display properly
- TAP file confirmed working

### Progress

- [x] Extract ZX Spectrum TAP file
- [x] Verify demo works in emulator
- [x] Extract all data blocks from TAP
- [x] Create graphics extraction tool
- [x] Extract initial graphics to PNG
- [ ] Watch and document all demo effects
- [ ] Disassemble Z80 code
- [ ] Analyze music format
- [ ] Set up C64 build system
- [ ] Implement demo framework
- [ ] Port visual effects
- [ ] Convert music/sound
- [ ] Polish and testing

## Reference Emulator 🎮

**For pixel-perfect conversion reference**, use the integrated emulator:

```bash
cd emulator && ./start.sh
```

Then open: **http://localhost:8888/cubelon_reference.html**

### Features
- ✅ One-click frame capture (press SPACE)
- ✅ Pause/resume demo (press P)
- ✅ Frame counter and timing display
- ✅ Save bookmarked frames with labels
- ✅ Download 256×192 PNG frames

**Perfect for:**
- Side-by-side comparison during C64 development
- Capturing specific effects for analysis
- Getting exact pixel data for conversion
- Frame-accurate timing reference (50 Hz)

## Viewing Extracted Graphics

Graphics have been extracted from the demo as PNG files:

```bash
# View 2x scaled graphics (easier to see)
open original/cubelon_data_01_2x.png
open original/cubelon_data_02_2x.png
```

Extract more graphics:
```bash
python3 tools/view_zx_screen.py original/cubelon_data_03.bin
```

## Building

### Requirements

- **cc65** - 6502 C compiler and assembler toolchain
- **VICE** - C64 emulator for testing
- **Python 3** - For conversion tools
- **Make** - Build automation

### Build Instructions

```bash
make
```

The built demo will be in `build/cubelon.prg`

### Running

```bash
# In VICE
x64sc build/cubelon.prg

# Or use make target
make run
```

## Project Structure

```
cubelon64/
├── original/       # Original ZX Spectrum extracted data
├── docs/          # Documentation and analysis
├── tools/         # Conversion and build tools
├── src/           # C64 source code
│   ├── effects/   # Effect implementations
│   ├── data/      # Data includes
│   ├── music/     # Music files
│   └── include/   # Common definitions
└── build/         # Build artifacts
```

## Credits

### Original ZX Spectrum Demo
- CUBELON by [Original Authors]

### C64 Port
- Dave (2026)

## License

[TBD - Respect original demo license]

## Development Notes

See [docs/PORTING_NOTES.md](docs/PORTING_NOTES.md) for technical details about the porting process.
