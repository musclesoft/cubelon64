# Rotating Square Algorithm

## C64 Implementation

The rotating square is rendered as a 24×24 pixel bitmap in a 3×3 grid of C64 characters
(chars 64-82 at `SQUARE_CHARS=$2200`). Column-major layout with 64-byte spacing.

### Pipeline

1. **Rotation**: Multiply corner offsets by sin/cos (fixed 8.8), add pivot
2. **Scale 2x**: Double corners for 48×48 supersample space
3. **Scanline fill**: DDA edge walking, fill spans row by row in 48×48 space
4. **Downsample**: 2×2 → 1 output pixel (0 sub-pixels=off, 1-2=medium, 3-4=full)
5. **Charset write**: Pack bits into character data (bit 7 = leftmost pixel, X=0)
6. **Restore corners**: Halve corners back to 24×24 space

### Geometry

- Square: 12×12 pixels
- Center: (12.0, 12.0)
- Pivot: bottom-right corner (18.0, 18.0)
- Rotation: 0° to 360° in steps driven by `ANGLE` variable

### Fixed 8.8 Format

16-bit little-endian: high byte = integer part, low byte = fractional.
All rotation math uses this format. Sin/cos values scaled by 256.

### Bit Ordering

Bit 7 = leftmost pixel (X=0), bit 0 = rightmost pixel (X=7).

## Reference / Source of Truth

The original ZX demo, run in the Go Z80 emulator, is the ground truth. `proto/` reimplements
the algorithm and writes `proto/golden/*.bin`, which the C64 build checks with `assert_if`
(`make -C c64port validate`, which also compares the C64 frames with the saved ZX frames). See
`tools/README.md` for the chain.

## Implementation Files

- `c64port/src/rotation.s` — Fixed-point 8.8 sin/cos multiply (`mul16x16_signed32`)
- `c64port/src/sincos_tables.s` — Sin/cos lookup tables
- `c64port/src/render.s` — DDA scanline walker, 2×2 supersample fill, charset output
- `c64port/src/charset.s` — Charset initialization

## Verification

`assert_if` lines in `c64port/src/main.s` and `animate.s` compare CHARSET, SPRITE and
PRECOMPUTED_FRAMES memory with `proto/golden/*.bin` at specific frames.
