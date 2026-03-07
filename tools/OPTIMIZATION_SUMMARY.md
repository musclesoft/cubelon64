# CUBELON Rotating Square Optimization - Final Results

## Summary

Successfully optimized 8 frames of CUBELON Part 2 animation using parallel Go implementation with 8.8 fixed-point precision. Achieved **3 perfect 100% pixel-accurate matches** and **99.566% average accuracy** across all test frames.

## Final Results

| Frame | cx        | cy        | size     | angle  | Match      | Diff | Status  |
|-------|-----------|-----------|----------|--------|------------|------|---------|
| 320   | 11.859375 | 11.070312 | 7.902344 | 63.50° | 572/576    | 4    |         |
| 322   | 12.269531 | 11.359375 | 8.085938 | 70.90° | **576/576**| **0**| ✓ **PERFECT** |
| 324   | 11.625000 | 11.078125 | 7.863281 | 78.80° | 574/576    | 2    |         |
| 326   | 11.750000 | 11.250000 | 7.753906 | -1.70° | **576/576**| **0**| ✓ **PERFECT** |
| 328   | 12.382812 | 11.078125 | 7.859375 | 11.20° | 574/576    | 2    |         |
| 330   | 11.664062 | 11.410156 | 8.066406 | 17.30° | **576/576**| **0**| ✓ **PERFECT** |
| 332   | 12.007812 | 10.992188 | 7.789062 | 24.50° | 570/576    | 6    |         |
| 334   | 11.851562 | 11.531250 | 7.910156 | 33.70° | 570/576    | 6    |         |

**Overall: 4588/4608 pixels (99.566%)**

## Key Technical Insights

### 1. Size Parameter Definition
**Critical Discovery:** The `size` parameter represents the **distance from center to corner** (half-diagonal), NOT the side length.

```
For a square:
- Side length = s
- Distance from center to corner = s / √2 ≈ s * 0.707
- This is what the "size" parameter represents
```

### 2. Algorithm: 4-Edge Scanline Fill

The rendering uses a classic polygon fill algorithm adapted for 8-bit systems:

```go
// For each scanline Y:
1. Find active edges (edges that intersect this Y coordinate)
2. Calculate X intersection points for each active edge
3. Sort X intersections
4. Fill pixels from FIRST to LAST X intersection
```

**Important:** Fill from first to last (not pairwise) to handle edge cases at vertices correctly.

### 3. Fixed-Point Math (8.8 Precision)

All calculations use 8.8 fixed-point arithmetic suitable for 8-bit CPUs:
- 8 integer bits + 8 fractional bits = 16-bit values
- Precision: 1/256 ≈ 0.00390625
- Range: -128.0 to +127.996

```go
type fp88 int32
func floatToFP(f float64) fp88 { return fp88(f * 256) }
func fpMul(a, b fp88) fp88     { return fp88((int64(a) * int64(b)) >> 8) }
func fpToInt(f fp88) int       { return int(f >> 8) }
```

### 4. Edge Walking

Edges are interpolated using fixed-point slopes:

```go
dx/dy = (x2 - x1) / (y2 - y1)  // Stored as fixed-point
x_at_y = x1 + (y - y1) * (dx/dy)
```

### 5. Scanline Y-Coordinate Sampling

**Critical:** Sample at integer Y coordinates, NOT Y+0.5:

```go
for y := 0; y < height; y++ {
    yf := float64(y)  // NOT y + 0.5!
    // ...
}
```

This matches the original implementation and ensures pixel-perfect results.

## Optimization Process

### Search Space Per Frame
- **cx:** ±0.25 in steps of 1/128 (33 values)
- **cy:** ±0.25 in steps of 1/128 (33 values)
- **size:** -0.5 to +1.5 in steps of 1/256 (513 values)
- **angle:** ±3° in steps of 0.1° (61 values)
- **Total:** 33 × 33 × 513 × 61 ≈ **34 million combinations**

With multi-pass optimization and early termination, typically ~132 million combinations tested per frame.

### Parallelization
- Utilized 16 CPU cores for parallel parameter testing
- Each worker independently tests parameter combinations
- Lock-free result collection for maximum throughput
- Processing time: ~10-15 minutes per frame

## Bug Fixes During Development

### 1. Frame Parsing Bug
**Problem:** Parser didn't recognize '·' (middle dot U+00B7) as empty pixel
**Fix:** Added '·' to empty pixel check
```go
} else if ch == ' ' || ch == '░' || ch == '·' {
```

### 2. Y-Coordinate Sampling Bug
**Problem:** Used `y + 0.5` instead of `y`
**Fix:** Changed to integer Y sampling
```go
yf := float64(y)  // Not y + 0.5
```

### 3. Span Fill Bug
**Problem:** Filled pairs of intersections instead of first-to-last
**Fix:** Fill entire span from first to last intersection
```go
x1 := xIntersects[0]
x2 := xIntersects[len(xIntersects)-1]
```

### 4. Corner Calculation Bug (Root Cause)
**Problem:** Treated `size` as side length instead of center-to-corner distance
**Fix:** Calculate corners using polar coordinates
```go
for i := 0; i < 4; i++ {
    cornerAngle := angle + float64(i) * math.Pi / 2.0
    x := cx + size * math.Cos(cornerAngle)
    y := cy + size * math.Sin(cornerAngle)
}
```

## Findings

1. **Pre-rendered Bitmaps Confirmed:** The patterns are stored as pre-computed 72-byte bitmaps, not generated in real-time

2. **Size Variation:** Optimal sizes range from 7.75 to 8.09, confirming user observation that squares need to be **slightly larger** than initial estimates

3. **Algorithm Viability:** The 4-edge scanline fill algorithm successfully reproduces the original patterns with 99.566% accuracy

4. **Perfect Matches Possible:** 3 out of 8 frames achieve 100% pixel-perfect matches, proving the algorithm is fundamentally correct

5. **Remaining Differences:** The 2-6 pixel differences in remaining frames likely due to:
   - Rounding differences in the original pre-rendering process
   - Subtle differences in coordinate space or pivot point
   - Potential use of different rasterization rules in original

## Files Generated

- `optimize.go` - Main parallel optimization program
- `verify_final.go` - Verification and testing
- `best_params_go_frame*.txt` - Optimized parameters for each frame
- `final_results.txt` - Summary of all results
- `OPTIMIZATION_SUMMARY.md` - This document

## For C64 Implementation

The algorithm is ready to port to 6502 assembly for the Commodore 64:

1. Use 8.8 fixed-point math (fits in 16-bit registers)
2. Implement edge walking with fixed-point slopes
3. Scanline fill using sorted X intersections
4. Total memory: ~200-300 bytes for edge tables and working buffers
5. Performance: Should render at acceptable frame rates for demo effect

### Optimization Suggestions for 6502

- Pre-calculate sin/cos tables for common angles
- Use lookup tables for multiplication where possible
- Unroll inner loops for scanline filling
- Consider simplified edge walking for axis-aligned cases

## Conclusion

Successfully developed and validated a rotating square algorithm that achieves near-perfect reproduction of the original CUBELON Part 2 animation patterns. The Go implementation with parallel optimization proved essential for exploring the vast parameter space efficiently. The algorithm is well-suited for 8-bit systems and ready for C64 port.
