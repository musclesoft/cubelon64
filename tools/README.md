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

The rendering reference is the original ZX demo itself, run in the Go Z80 emulator
(`emulator/`). The chain from original to C64 is:

1. `go run -C emulator run_cubelon.go assertions.go motion_analyzer.go --no-audio` writes
   `emulator/frames/frame_NNNNN.png` for every demo frame whose screen changed
   (missing numbers are identical to the previous file). These PNGs are checked visually.
2. `proto/` reimplements the renderer in Go with 8-bit primitives and writes
   `proto/golden/*.bin` (gitignored, regenerate locally).
3. `assert_if` lines in `c64port/src/*.s` compare C64 memory with the golden files at
   given frames. `make -C c64port validate` runs them and panics on any mismatch.

Nothing automatically compares step 1 with step 2.
