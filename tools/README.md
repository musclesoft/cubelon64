# Tools

## AY-3-8912 to SID 6581 Audio Optimizer

**optimize_volume.go** - Translates ZX Spectrum audio to C64 SID using RMS envelope analysis and ADSR curve fitting.

```bash
# 1. Generate AY command log from emulator
go run -C emulator run_cubelon.go motion_analyzer.go assertions.go \
  --log-ay-commands=ay_commands.bin --max-frames=9650 --no-frames

# 2. Optimize to SID
go run -C tools . --input=../emulator/ay_commands.bin
```

Output is a 17-channel WAV file `optimized.wav` (44100 Hz, Int16, interleaved). Channels are 1-based:

| Channels | Contents                                                                     |
| -------- | ---------------------------------------------------------------------------- |
| 1-5      | Voice A: AY emulation, direct SID, tracker SID, 6502 player, SID-file replay |
| 6-10     | Voice B: same five sources                                                   |
| 11-15    | Voice C: same five sources                                                   |
| 16       | ADSR delay bug debug (+1=A, +0.33=B, -0.33=C, -1=none)                       |
| 17       | Tracker vs AY error, all voices combined (-1=0% error, +1=100% error)        |

The 6502 player and SID-file replay tracks run one tick behind the tracker SID track, because the player stages each frame one tick before applying it. Their CtrlA is read from the player's `PLAYER_FRAME` state rather than `$D404`, which holds whichever CIA phase was written last.

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
