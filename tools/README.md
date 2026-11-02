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

The run writes two files:

- `optimized.wav`: a 13-channel WAV file (44100 Hz, Int16, interleaved)
- `cubelon.sid`: the playable RSID file, built from `player_harness_irq.s`

The WAV channels are 1-based:

| Channels | Contents                                                              |
| -------- | --------------------------------------------------------------------- |
| 1-4      | Voice A: AY emulation, direct SID, tracker SID, SID-file replay       |
| 5-8      | Voice B: same four sources                                            |
| 9-12     | Voice C: same four sources                                            |
| 13       | Tracker vs AY error, all voices combined (-1=0% error, +1=100% error) |

The tracker SID track is rendered from per-frame register snapshots. The SID-file replay track is rendered from the raw SID write stream of the IRQ-driven harness (`player_harness_irq.s`, the same code `cubelon.sid` runs): a cycle-counted 6502 executes the real handlers while CIA1 timers count each cycle, interrupts are taken between instructions (so a CIA fire can preempt the raster tick), and every SID write is stamped with its cycle and played into the SID emulator at that moment. The VIC raster IRQ is modeled at the first cycle of each frame.

The player stages each frame one tick before applying it, so the harness runs one priming tick at init to stay frame-aligned with the tracker SID track. The replay audio trails the tracker by the tick's execution time (a few thousand cycles).

The replay has no Go-side ADSR bug masking, so it shows how the player really behaves. A write that lowers the SID's envelope rate period below its rate counter freezes the envelope for up to ~1.7 frames (the ADSR delay bug), which would delay a note's onset if the previous note's gate-off/gate-on hit it. The player prevents this by running the last `framesBeforeNoteOff` (2) frames of every note with D=0 (decided from its own duration counter, with no lookahead and no extra music data); the tracker simulation applies the same rule, and the constant reaches the player through `music_data.inc` as `FRAMES_BEFORE_NOTE_OFF`. `optimize_volume.go` panics if a note onset in the replay is still frozen by the bug, or if the replay's true warning count exceeds its committed baseline.

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
