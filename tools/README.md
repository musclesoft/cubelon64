# Pattern Generation Tools

This directory contains tools for analyzing and optimizing the CUBELON Part 2 marching square animation.

## Final Results

**RECOMMENDED: Improved Truncation-Optimized (best balance):**
```
Size:    3064 (11.97 pixels) - $0BF8
CenterX: 3064 (11.97) - $0BF8
CenterY: 2960 (11.56) - $0B90
PivotX:  4612 (18.02) - $1204
PivotY:  4488 (17.53) - $1188
Angle:   0° to 90° in 9° steps (π/20 radians)
```
**Accuracy:** 99.86% (7/11 frames pixel-perfect)
**Advantage:** No rounding needed - just use high byte after multiply!
**Fixes:** Isolated pixel anomaly at frame 6 (45°)

**Alternative 1: Simple Truncation (clean values but has glitch):**
```
Size:    3072 (12.00) - $0C00
CenterX: 3072 (12.00) - $0C00
CenterY: 2944 (11.50) - $0B80
PivotX:  4608 (18.00) - $1200
PivotY:  4480 (17.50) - $1180
```
**Accuracy:** 99.67% (7/11 frames pixel-perfect)
**Issue:** Has isolated extra pixel at frame 6, position (12,6)

**Alternative 2: Maximum Accuracy (requires rounding):**
```
Size:    3065 (11.973 pixels)
CenterX: 3010 (11.758)
CenterY: 2954 (11.539)
PivotX:  4578 (17.883)
PivotY:  4552 (17.781)
```
**Accuracy:** 99.73% (8/11 frames pixel-perfect)
**Verification:** 85,766,121 parameter combinations tested
**Trade-off:** Only +0.06% gain, requires rounding logic

**Conclusion:** Improved truncation is the sweet spot - nearly optimal accuracy with simplest code

## Key Tools

### Verification and Testing

- **verify_params.go** - Verifies optimal parameters, generates comparison images
- **verify_improved_params.go** - Verifies improved truncation parameters with frame 6 fix
- **final_comparison.go** - Side-by-side comparison of parameter sets
- **test_scanline.go** - Tests scanline rendering methods
- **test_rectangle.go** - Tests if rectangle (non-square) improves accuracy
- **compare_implementations.go** - Compares optimal vs clean implementations
- **find_clean_params.go** - Finds clean rounded parameters
- **test_truncation.go** - Tests truncation vs rounding, finds optimal params for truncation
- **test_hybrid_pivot.go** - Tests different pivot Y values
- **analyze_frame6.go** - Pixel-level analysis of frame 6 isolated pixel anomaly
- **analyze_scanline6.go** - Detailed scanline intersection calculation for frame 6
- **test_ceiling_left.go** - Tests ceiling edge rule (rejected - degrades other frames)
- **optimize_frame6_pixel.go** - Searches parameter space to fix frame 6 pixel

### Optimization Tools (Used to Find Solution)

- **exhaustive_search.go** - Exhaustive grid search (85M+ combinations)
  - Definitive proof that 8/11 perfect is maximum
  - Parallel search using all CPU cores
  - ±10 units in spatial params, ±0.01 rad in angle

- **optimize_fixed88.go** - PSO optimizer in 8.8 fixed-point space
  - 256 particles, 500 iterations
  - Result: 99.70% accuracy

- **optimize_float.go** - Floating-point PSO optimizer
  - Unconstrained float64 optimization
  - Result: 99.68-99.98% accuracy
  - Problem: Doesn't quantize well to 8.8 fixed-point

- **optimize_per_frame_angles.go** - PSO with per-frame angles
  - Each frame can have independent angle
  - Result: 99.89% accuracy (only +0.16% vs fixed steps)
  - Conclusion: Not worth the complexity

- **optimize_angles_only.go** - Greedy per-frame angle optimizer
  - Keeps spatial params fixed, optimizes angles only
  - Searches ±2° per frame with 0.1° steps
  - Result: 99.89% accuracy, 8/11 perfect
  - Finding: Most frames (8/11) perfect at exactly 9° steps

- **find_pivot.go** - Grid search for optimal pivot point
  - Searches pivot position space
  - Led to pivot at (17.883, 17.781)

### Animation Extraction

- **show_11frames_grid.go** - Displays all 11 frames in grid
- **extract_marching_cycle.go** - Extracts 11-frame animation cycle
- **create_sprite_sheet_from_source.py** - Decodes patterns with interleaved byte order

### Animation Viewer

Open `animation.html` in your browser to see the complete 52-frame animation.

**Features:**
- Slider: Step through all 52 frames (frames 319-370)
- Play button: Auto-play the animation at 25 fps
- Keyboard controls: Arrow keys to step, Space to play/pause
- Frame info: Shows current frame number

**Data:**
- `animation_frames.json` - 52 frames of 24×12 pixel data extracted from ZX Spectrum screen memory

### Early Exploration (Historical)

These tools were used during initial exploration and parameter search:
- optimize.go, optimize_frame320_fine.go, optimize_334.go, optimize_332.go
- global_search.go, fast_global_search.go
- optimize_corners.go, ultrafine_corners.go
- visualize_pivots.go, analyze_ground_contact.go, optimize_ground_pivot.go
- find_fixed_corner.go, find_fixed_edge.go
- Various other analysis and visualization tools

## Running the Tools

### Verify Final Parameters
```bash
cd tools
go run verify_params.go
```

### Run Exhaustive Search (takes hours)
```bash
cd tools
go run exhaustive_search.go
```

### Test Per-Frame Angles
```bash
cd tools
go run optimize_angles_only.go
```

## Algorithm

The animation is a square rotating around a pivot point:

1. **Square**: 11.973 × 11.973 pixels
2. **Center**: (11.758, 11.539) in 24×24 grid
3. **Pivot**: (17.883, 17.781) - ~6.1 pixels right and down from center
4. **Rotation**: 0° to 90° in exactly 9° steps
5. **Method**: 8.8 fixed-point arithmetic with rounding

**Key findings:**
- Not exactly 12×12 pixels
- Center not at (12, 12)
- Initial angle exactly 0° (no offset)
- Per-frame angle adjustments don't help significantly
- 11/11 perfect match is mathematically impossible
- 99.73% accuracy is visually indistinguishable in motion

See [../docs/part2_pattern_generation_FINAL.md](../docs/part2_pattern_generation_FINAL.md) for complete analysis.
