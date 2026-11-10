# CUBELON Emulator

ZX Spectrum 128K emulator optimized for capturing the CUBELON demo frame-by-frame with cycle-accurate timing.

## Features

- Cycle-accurate Z80 emulation with memory contention
- Progressive scanline capture (simulates CRT beam racing)
- 3-channel AY-3-8912 audio capture
- C64 mode (`--c64 <prg>`) with a SID model (`sidemu/`) fed by the C64's $D400 writes
- Per-frame audio traces (`--audio-trace`) and a ZX vs C64 compare (`compare_audio.go`)
- Automatic demo frame detection
- Cropped frame output (192×192 or 180×180 for analysis)

## Quick Start

The directory holds several standalone tools that each define `main`, so `go run .` does not
work: name the runner's files (`run_cubelon.go assertions.go motion_analyzer.go`) as below.

```bash
# Automatically finds cubelon_final.tap in ../original/ or ./
go run run_cubelon.go assertions.go motion_analyzer.go

# Or build once
go build -o run_cubelon run_cubelon.go assertions.go motion_analyzer.go
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
go run run_cubelon.go assertions.go motion_analyzer.go --no-frames --no-audio --stop-at=1000

# At this point, add breakpoint or memory inspection code
```

### Save Only Specific Frames

```bash
# Save one frame for inspection
go run run_cubelon.go assertions.go motion_analyzer.go --save-frames=900

# Save a range (e.g., Part 3: frames 802-1404)
go run run_cubelon.go assertions.go motion_analyzer.go --save-frames=802-1404

# Save multiple specific frames for comparison
go run run_cubelon.go assertions.go motion_analyzer.go --save-frames=800,900,1000,1100
```

### Only Changed Frames Are Saved

A frame PNG is written only if the screen differs from the previous captured frame.
A missing `frame_NNNNN_zx.png` means that frame is identical to the closest lower-numbered
file, not that it was skipped or lost. For example, there is no `frame_00780_zx.png` to
`frame_00800_zx.png` because the screen stays unchanged after `frame_00779_zx.png`. All saved
frames are cropped to 192×192 at x=32.

### Disable Audio (Moderate speedup)

```bash
# Skip audio if you don't need it
go run run_cubelon.go assertions.go motion_analyzer.go --no-audio
```

## Audio Trace and Sync Check

`--audio-trace=file.csv` writes one row per frame: `frame,a_level,a_hz,b_level,b_hz,c_level,c_hz`.
It works in both modes and needs the audio model, so not with `--no-audio` on the ZX side.

- **ZX:** keyed by demo frame, sampled at the end of the frame. Level is the AY volume register
  (or the envelope level when the envelope bit is set), 0 to 1, gated by the mixer. Pitch is the
  tone period in Hz, 0 for noise-only channels.
- **C64:** keyed by the sequencer counter (`SEQ_FRAME_LO`), from the frames' SID writes replayed
  through `sidemu` at cycle accuracy. Level is envelope times master volume, 0 to 1. Pitch is
  the voice frequency in Hz, 0 for noise.

`compare_audio.go` compares the two CSVs:

```bash
go run compare_audio.go zx.csv c64.csv
```

It lists the activity mismatches for each C64 lag from -4 to +4 and picks the best one, then
reports per channel (A, B, C) the activity mismatches, the pitch mismatches (limit 50 cents,
octaves ignored) and the first diverging frame. The best lag should be 0.

## Frame Ranges by Demo Part

Use these ranges with `--save-frames` to capture specific effects:

| Part | Frames | Description |
|------|--------|-------------|
| E0 | 2-167 | Initialization |
| E1 | 168-779 | Marching cubes (still of plain squares from 779, unchanged until 800) |
| E2 | 801-819+ | Morph: squares become diamonds, then petals, then inverted grid (801-819 verified); geometric pattern continues to 1404 |
| E3 | 1405-2304 | Grid pulse/rotation |
| E4 | 2305-3999 | Pattern cycling |
| E5 | 4000-7005 | Plasma & morphing |
| E6 | 7006-7249 | Grid fade-out |
| E7 | 7250-9420 | Credits text |

The part boundaries other than E1/E2 are rough labels and have not been re-verified.

Examples:
```bash
# Morph scene (779 is the last still, 801 the first changed frame)
go run run_cubelon.go assertions.go motion_analyzer.go --save-frames=779-819

# Compare start and end of Part 2
go run run_cubelon.go assertions.go motion_analyzer.go --save-frames=168,400,600,801

# Analyze plasma effect transitions
go run run_cubelon.go assertions.go motion_analyzer.go --save-frames=4000-4050,5000-5050
```

## Output Files

- `frames/frame_XXXXX_zx.png` - Captured frames (cropped to 192×192 at x=32)
- `cubelon_audio.wav` - 3-channel audio (unless --no-audio)
- `--audio-trace` CSV - per-frame channel levels and pitches (see above)

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
- Only saves frames that differ from the previous frame (pixel-by-pixel comparison of the full frame buffer before cropping)
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
go run run_cubelon.go assertions.go motion_analyzer.go --no-frames --no-audio --stop-at=900
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
- 50Hz (70,908 T-states per frame on the 128K)
- Frame counter at 0x8184 increments every frame, right after the AY music tick in the
  interrupt handler, so it equals the number of ticks played (the 0-based tick index plus 1)
- Capture frames are numbered from the emulator's first frame; the demo counter is the capture
  frame minus 1 (`demoFrameOffset`). The AY log (`--log-ay-commands`) uses capture frames; PNGs
  and `--audio-trace` use the demo counter. Example: the first sound is capture frame 162,
  demo frame 161
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
- All saved frames are cropped to 192×192 at offset (32, 0)
- The change detection runs on the full frame buffer, before cropping

## Troubleshooting

**Error: "stat run_cubelon.go: no such file or directory"**
- Run from `emulator/` directory: `cd emulator && go run run_cubelon.go assertions.go motion_analyzer.go`

**Frames are missing/incomplete**
- Don't interrupt emulation - frame counter detection requires initial frames
- Use `--stop-at` instead of Ctrl-C

**Emulation is slow**
- Use `--no-frames` for analysis
- Use `--no-audio` if you don't need audio
- Use `--save-frames` to save only what you need

**Out of disk space**
- 9,420 frames ≈ 50-100MB (depends on complexity)
- Use selective saving: `--save-frames=779-819` for the morph scene only
