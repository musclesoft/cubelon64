# Tools

## AY-3-8912 to SID 6581 Audio Optimizer

**optimize_volume.go** - Translates ZX Spectrum audio to C64 SID using RMS envelope analysis and ADSR curve fitting.

```bash
# 1. Generate AY command log from emulator
cd emulator
go run run_cubelon.go motion_analyzer.go assertions.go \
  --log-ay-commands=ay_commands.bin --max-frames=9650 --no-frames

# 2. Optimize to SID
go run ../tools/optimize_volume.go ../tools/sid_emulator.go \
  --input=ay_commands.bin --output=optimized.wav
```

Output is a 6-channel WAV (paired for easy comparison):
- Channels 1+2: AY A and SID Voice 1
- Channels 3+4: AY B and SID Voice 2
- Channels 5+6: AY C and SID Voice 3

## C64 Rendering Reference

### THE reference: generate_comparison_with_reference.go

This is the authoritative source of truth for verifying C64 rendering correctness.

```bash
go run generate_comparison_with_reference.go
# Output: emulator/frames/comparison_with_reference.png
```

Produces a 13-column PNG:
- **Column 0** (blue border): 16×16 supersampled float ground truth
- **Columns 1-12** (green=best, red=worst): 12 rendering variants sorted by quality

Tests 11 animation frames at 0°, 9°, 18°, 27°, 36°, 45°, 54°, 63°, 72°, 81°, 90°.

Uses identical geometry and rotation math as the 6502 code:
- 12×12 pixel square
- Pivot at (18.0, 18.0) — bottom-right corner
- Fixed 8.8 rotation with truncation

## generate_reference_24x24.go

Generates a single reference PNG for a given angle (useful for visual inspection).

```bash
go run generate_reference_24x24.go 45 /tmp/ref_45deg.png
```
