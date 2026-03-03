# CUBELON 64

A Commodore 64 port of the ZX Spectrum demo CUBELON.

## Status

🚧 **Work in Progress** - Currently in Phase 1: Analysis and Understanding

### Progress

- [x] Extract ZX Spectrum TAP file
- [ ] Watch and document original demo
- [ ] Disassemble Z80 code
- [ ] Extract graphics and assets
- [ ] Set up C64 build system
- [ ] Implement demo framework
- [ ] Port visual effects
- [ ] Convert music/sound
- [ ] Polish and testing

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
