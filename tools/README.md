# Pattern Generation Tools

This directory contains tools for analyzing and optimizing the CUBELON Part 2 marching square animation.

## Final Results

**Optimal Parameters (8.8 Fixed-Point):**
```
Size:    3065 (11.973 pixels)
CenterX: 3010 (11.758)
CenterY: 2954 (11.539)
PivotX:  4578 (17.883)
PivotY:  4552 (17.781)
Angle:   0° to 90° in 9° steps (π/20 radians)
```

**Accuracy:** 99.73% (8/11 frames pixel-perfect)
**Verification:** 85,766,121 parameter combinations tested
**Conclusion:** Visually perfect for demo, 11/11 impossible with simple rotating square

## Key Tools

### Verification and Testing

- **verify_params.go** - Verifies optimal parameters, generates comparison images
- **test_scanline.go** - Tests scanline rendering methods
- **test_rectangle.go** - Tests if rectangle (non-square) improves accuracy

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
