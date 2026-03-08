# Frame 6 Single Pixel Anomaly

## Summary

Frame 6 (45° rotation) contains one isolated pixel mismatch at position (12, 6) that appears to be an edge case in scanline rasterization. This pixel is filled by our algorithm but not by the original ZX Spectrum version.

## Technical Details

### The Pixel
- **Location**: (12, 6)
- **Type**: Extra pixel (rendered black, should be white)
- **Status**: Only isolated mismatch in frame 6 (other 16 mismatches are clustered edge pixels)

### Root Cause

At scanline y=6, the left edge of the rotated square intersects at:
- **x_raw = 3199** (in 8.8 fixed-point)
- **x_float = 12.496094** pixels
- **Fractional part = 127/256 = 0.496** (just below 0.5)

Our scanline fill algorithm uses:
```go
x1 := (intersections[i] + 128) >> 8  // Round to nearest
// x1 = (3199 + 128) >> 8 = 3327 >> 8 = 12
```

This fills pixels starting from x=12. However:
- Pixel 12 spans [12.0, 13.0)
- The quad edge is at 12.496
- Only **50.4%** of pixel 12 is covered by the quad

The original ZX Spectrum algorithm does NOT fill this pixel, suggesting it uses different edge inclusion rules.

## Attempted Fix: Ceiling on Left Edge

Testing showed that using ceiling for the left edge intersection:
```go
x1 := (intersections[i] + 255) >> 8  // Ceiling
// x1 = (3199 + 255) >> 8 = 3454 >> 8 = 13
```

Would fix this pixel, but causes **significant degradation** across other frames:

| Metric | Rounding (current) | Ceiling Left | Change |
|--------|-------------------|--------------|---------|
| Overall accuracy | 99.67% | 98.69% | **-0.98%** |
| Perfect frames | 7/11 | 2/11 | **-5 frames** |
| Frame 6 accuracy | 97.05% | 97.22% | +0.17% |

**Result**: Fixing this one pixel breaks many others.

## Possible Explanations

Why the original doesn't fill this pixel:

1. **Different edge rules**: May use asymmetric rules (e.g., ceiling left, floor right)
2. **Per-frame micro-adjustments**: Spatial parameters might vary slightly per frame
3. **Alternative rasterization**: Different algorithm entirely (not standard scanline)
4. **Coverage-based filling**: May require >50% pixel coverage threshold
5. **Coordinate system differences**: Subtle pixel center vs edge conventions

## Conclusion

This single pixel anomaly:
- Represents a 0.17% error (1 pixel out of 576) in frame 6 only
- Is imperceptible in motion at 50Hz demo playback
- Cannot be fixed without degrading overall accuracy by 0.98%
- Is an acceptable limitation of the simplified rotation model

**Recommendation**: Accept this minor imperfection. The truncation-optimized parameters remain the best choice at 99.67% overall accuracy.

## Investigation Files

- `analyze_frame6.go` - Pixel-level analysis identifying the isolated mismatch
- `analyze_scanline6.go` - Detailed scanline intersection calculation
- `test_ceiling_left.go` - Testing ceiling edge rule (rejected due to poor results)
- `frame6_analysis.png` - Visual comparison showing the isolated pixel

## Visual Reference

Frame 6 comparison:
- Target: ZX Spectrum original
- Rendered: Our truncation-optimized algorithm
- Difference: Red = missing, Green = extra

The isolated extra pixel at (12, 6) is visible as a single green pixel away from the main cluster of edge mismatches along the diagonal.
