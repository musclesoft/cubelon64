# CUBELON Minimal Emulator

A focused ZX Spectrum 128K emulator specifically for CUBELON analysis.

## Purpose

- **Automated frame capture** - Capture every frame for analysis
- **Music extraction** - Log AY-3-8912 register writes for SID conversion
- **Headless operation** - No GUI, runs in terminal
- **Instrumentation** - Add hooks for debugging and analysis

## Building

```bash
cd emulator
go build -o cubelon-emu
```

## Usage

```bash
./cubelon-emu ../original/cubelon_final.tap
```

Output:
- `frames/frame_0000.png` - Screenshot captures
- `stdout` - AY music register writes

## Architecture

### Minimal Implementation

Only implements what CUBELON needs:
- Core Z80 instructions (add more as needed)
- ZX Spectrum 128K memory banking
- TAP file loading
- Screen rendering (256×192)
- AY-3-8912 sound chip (logging only)

### Package Structure

```
z80/        - Z80 CPU core
spectrum/   - ZX Spectrum hardware
  memory.go - 128K memory system
  io.go     - I/O ports and AY chip
  screen.go - Screen capture
  tap.go    - TAP file loading
main.go     - Main program
```

## Extending

### Adding Z80 Instructions

Edit `z80/cpu.go`, add case to `execute()`:

```go
case 0xXX: // INSTRUCTION
    // Implementation
    z.Cycles += N
```

### Capturing More Data

Modify `spectrum/io.go` to log port writes:

```go
func (io *IO) Out(port byte, val byte) {
    fmt.Printf("OUT 0x%02x, 0x%02x\n", port, val)
    // ... existing code
}
```

### Music Analysis

AY register writes are logged to stdout. Pipe to file:

```bash
./cubelon-emu cubelon_final.tap > music.log 2>&1
```

Format: `AY[reg] = 0x##`

## Status

🚧 **In Development** - Basic functionality works, but:
- Entry point needs to be found (currently tries 0x8800)
- More Z80 opcodes needed
- Timing is approximate

## Comparison to Web Emulator

**Web emulator** (`emulator/web/`):
- ✅ Complete, accurate emulation
- ✅ Real-time playback with sound
- ✅ Manual frame capture
- ❌ Hard to automate

**Go emulator** (this):
- ✅ Automated frame capture
- ✅ Headless operation
- ✅ Music data extraction
- ✅ Easy to instrument
- ❌ Incomplete, needs work

Use web emulator for watching, Go emulator for analysis.
