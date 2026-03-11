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

**`tools/generate_comparison_with_reference.go`** is the authoritative ground truth.

Run it to produce `emulator/frames/comparison_with_reference.png`:
```bash
go run tools/generate_comparison_with_reference.go
```

The PNG shows 13 columns:
- Column 0 (blue): 16×16 supersampled float rendering (ground truth)
- Columns 1-12 (green=best, red=worst): 12 rendering variants sorted by quality

Test angles: 0°, 9°, 18°, 27°, 36°, 45°, 54°, 63°, 72°, 81°, 90°

The reference uses identical math to the 6502 code (same pivot, same fixed 8.8
rotation with truncation), so the best-quality variant in column 1 is what we
target for the C64 implementation.

## Implementation Files

- `c64port/src/rotation.s` — Fixed-point 8.8 sin/cos multiply (`mul16x16_signed32`)
- `c64port/src/sincos_tables.s` — Sin/cos lookup tables
- `c64port/src/render.s` — DDA scanline walker, 2×2 supersample fill, charset output
- `c64port/src/charset.s` — Charset initialization

## Verification

Assertions in `emulator/run_cubelon.go` check C64 charset memory against expected
byte patterns at specific frames:

- `c64AssertAngle0` (frame 6): axis-aligned square, pixel-perfect
- `c64AssertAngle1` (frame 11): 1.4° rotation — identical to angle=0 (sin=6 too small
  to shift any pixel boundary in floor-based DDA)
- `c64AssertAngle2` (frame 16): 2.8° rotation — first visible difference; left edge
  shifts to X≈13.2 in supersample space, col 6 drops to medium for rows 6-16
