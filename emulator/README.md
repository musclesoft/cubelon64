# CUBELON Emulator

ZX Spectrum 128K emulator optimized for capturing the CUBELON demo frame-by-frame with cycle-accurate timing.

## Features

- Cycle-accurate Z80 emulation with memory contention
- Progressive scanline capture (simulates CRT beam racing)
- 3-channel AY-3-8912 audio capture
- Automatic demo frame detection
- Cropped frame output (192×192 or 180×180 for analysis)

## Quick Start

```bash
# Automatically finds cubelon_final.tap in ../original/ or ./
go run .

# Or build once
go build -o run_cubelon ./run_cubelon.go
./run_cubelon
```

The emulator automatically looks for `cubelon_final.tap` in:
1. `../original/cubelon_final.tap`
2. `./cubelon_final.tap`

## Fast Analysis Modes

For algorithm analysis, you often need to quickly reach specific frames without saving thousands of PNGs:

### Skip to Specific Frame (Fastest)

```bash
# Reach frame 1000 in ~5-10 seconds (vs ~1-2 minutes normally)
go run . --no-frames --no-audio --stop-at=1000

# At this point, add breakpoint or memory inspection code
```

### Save Only Specific Frames

```bash
# Save one frame for inspection
go run . --save-frames=900

# Save a range (e.g., Part 3: frames 802-1404)
go run . --save-frames=802-1404

# Save multiple specific frames for comparison
go run . --save-frames=800,900,1000,1100
```

### Disable Audio (Moderate speedup)

```bash
# Skip audio if you don't need it
go run . --no-audio
```

## Frame Ranges by Demo Part

Use these ranges with `--save-frames` to capture specific effects:

| Part | Frames | Description | Crop Size |
|------|--------|-------------|-----------|
| E0 | 2-167 | Initialization | 256×192 |
| E1 | 168-801 | Square emergence | 180×180 |
| E2 | 802-1404 | Geometric pattern | 192×192 |
| E3 | 1405-2304 | Grid pulse/rotation | 256×192 |
| E4 | 2305-3999 | Pattern cycling | 256×192 |
| E5 | 4000-7005 | Plasma & morphing | 256×192 |
| E6 | 7006-7249 | Grid fade-out | 180×180 |
| E7 | 7250-9420 | Credits text | 256×192 |

Examples:
```bash
# Analyze Part 3 (geometric pattern) only
go run . --save-frames=802-1404

# Compare start and end of Part 2
go run . --save-frames=168,400,600,801

# Analyze plasma effect transitions
go run . --save-frames=4000-4050,5000-5050
```

## Output Files

- `frames/frame_XXXXX.png` - Captured frames (cropped to active areas for Parts 2-3)
- `cubelon_audio.wav` - 3-channel audio (unless --no-audio)

## Performance

| Mode | Speed | Use Case |
|------|-------|----------|
| Default | ~60-80 fps | Full demo capture with audio |
| `--no-frames` | ~200-300 fps | Quick algorithm analysis |
| `--no-frames --no-audio` | ~300-400 fps | Fastest, state inspection only |
| `--save-frames=N` | ~100-150 fps | Selective capture |

On a modern CPU:
- Full demo (9,420 frames): ~2-3 minutes
- Skip to frame 1000: ~5-10 seconds with `--no-frames --no-audio`

**Threading:**
- Frame saving uses a worker pool (128 workers) for massively parallel PNG encoding
- Offloads I/O bottleneck from main emulation thread
- Minimal memory overhead due to efficient job queue

**Frame Diff Detection:**
- Only saves frames that differ from the previous frame (pixel-by-pixel comparison in cropped region)
- Dramatically reduces file count for static or repeated frames
- Makes analysis more efficient by filtering out duplicate frames

## Adding Analysis Code

**CRITICAL**: Only **read** state, never **modify** execution. See [EMULATOR_ANALYSIS_PRINCIPLES.md](../docs/EMULATOR_ANALYSIS_PRINCIPLES.md) for details.

To inspect state at specific frames, add code at frame boundaries:

```go
// ✅ CORRECT: Read state at frame boundary (after frame completes)
if demoFrame == 900 {
    fmt.Printf("\n=== Frame 900 Analysis ===\n")

    // ✅ Read memory
    patternTable := uint16(0x84c0)
    addr := mem.Read(patternTable)

    // ✅ Read CPU state
    fmt.Printf("PC=%04x SP=%04x\n", cpu.PC, cpu.SP)

    // ✅ Read AY registers
    fmt.Printf("AY Registers: %v\n", io.AY.Registers)
}

// ❌ WRONG: Never run extra CPU steps!
// for i < maxSteps {
//     cpu.Step()  // NO! Breaks cycle-accurate timing
// }
```

Then run with `--stop-at` to stop right after:
```bash
go run . --no-frames --no-audio --stop-at=900
```

**Rule**: Observe, don't interfere. The emulator must run cycle-accurately.

## Memory Map

```
0x0000-0x3FFF: ROM (bank 8 or 9)
0x4000-0x7FFF: Bank 5 (default screen at 0x4000-0x5AFF)
0x8000-0xBFFF: Bank 2
0xC000-0xFFFF: Bankable (0-7)

Demo frame counter: 0x8184 (16-bit, little-endian)
```

## Key Technical Details

### Frame Timing
- 50Hz (69,888 T-states per frame)
- Frame counter at 0x8184 increments every frame
- Demo starts at frame 2 (frame 1 has tape loading artifacts)

### Memory Contention
- Implemented for screen memory access (bank 5, 0x4000-0x7FFF)
- Pattern: [6, 5, 4, 3, 2, 1, 0, 0] T-state delays per 8-pixel block
- Active during visible scanlines (64-255), first 128 T-states

### Progressive Scanline Capture
- Captures each scanline at correct moment during frame execution
- Matches CRT beam position based on T-state count
- Prevents screen tearing from "beam racing" effects

### Frame Cropping
- Parts 2-3 are cropped to active square areas
- Part 2: 180×180 at offset (38, 6)
- Part 3: 192×192 at offset (32, 0)
- Other parts: full 256×192 screen

## Troubleshooting

**Error: "stat run_cubelon.go: no such file or directory"**
- Run from `emulator/` directory: `cd emulator && go run .`

**Frames are missing/incomplete**
- Don't interrupt emulation - frame counter detection requires initial frames
- Use `--stop-at` instead of Ctrl-C

**Emulation is slow**
- Use `--no-frames` for analysis
- Use `--no-audio` if you don't need audio
- Use `--save-frames` to save only what you need

**Out of disk space**
- 9,420 frames ≈ 50-100MB (depends on complexity)
- Use selective saving: `--save-frames=802-1404` for Part 3 only
