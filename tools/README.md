# C64 Rendering Reference

## THE reference: generate_comparison_with_reference.go

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
