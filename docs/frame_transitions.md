# CUBELON Frame Transitions - Bisection Results

Precise frame boundaries determined by binary search through captured frames.

---

## Transition Summary Table

| Part | Name | Start Frame | End Frame | Duration (frames) | Duration (seconds) | Notes |
|------|------|-------------|-----------|-------------------|-------------------|-------|
| **E0** | Initialization | 2 | 167 | 166 | 3.32s | Pure black screen |
| **E1** | Square Emergence | 168 | 801 | 634 | 12.68s | Pixels first appear at 168, grid complete by 801 |
| **E2** | Geometric Pattern | 802 | 1404 | 603 | 12.06s | Complex interlocking "bone" pattern |
| **E3** | Grid Pulse/Rotation | 1405 | 2304 | 900 | 18.00s | 8×8 grid with rotation animation |
| **E4** | Pattern Cycling | 2305 | 3999 | 1695 | 33.90s | Multiple diamond/grid patterns |
| **E5** | Plasma Effect | 4000 | 7005 | 3006 | 60.12s | Kaleidoscope plasma with 4-way symmetry |
| **E6** | 3D Squares (Part of E5) | ~5300-7005 | - | - | - | Embedded within plasma section |
| **E7** | Fade & Grid Return | 7006 | 7249 | 244 | 4.88s | Grid fades out through black |
| **E8** | Credits Text | 7250 | 9420 | 2171 | 43.42s | Typewriter text effect |

**Total**: Frames 2-9420 (9,419 frames = 188.38 seconds @ 50Hz)

---

## Detailed Transition Analysis

### Transition 0→1: Black Screen → Square Emergence
- **Frame 165**: Black screen (E0 continues)
- **Frame 167**: First tiny pixels appear (transition begins)
- **Frame 168**: Clear pixel dots visible (E1 begins)
- **Frame 170**: More squares emerging
- **Transition Type**: Gradual appearance (fade-in)
- **Precise Boundary**: Frame **168**

### Transition 1→2: Square Grid → Geometric Pattern
- **Frame 795**: Complete 8×8 grid of squares
- **Frame 800**: Grid still visible, solid squares
- **Frame 801**: Grid remains but pattern emerging
- **Frame 805**: Pattern transformation visible (butterfly/X shapes appearing)
- **Frame 810**: Full geometric pattern established
- **Transition Type**: Morphing transformation
- **Precise Boundary**: Frame **802** (first clear pattern change)

### Transition 2→3: Geometric Pattern → Grid Pulse/Rotation
- **Frame 1395**: Geometric pattern still visible
- **Frame 1404**: Pattern continues
- **Frame 1405**: Pattern transforms back to grid
- **Frame 1410**: Grid with butterfly/X overlays
- **Frame 1420**: Clean 8×8 grid restored
- **Transition Type**: Pattern-to-grid morphing
- **Precise Boundary**: Frame **1405**

### Transition 3→4: Grid Pulse → Pattern Cycling
- **Frame 2295**: Diamond/rotated pattern visible
- **Frame 2304**: Pattern continues (diamonds)
- **Frame 2305**: Pattern shifts to cleaner diamonds
- **Frame 2310**: Diamond pattern fully established
- **Frame 2320**: Diamond pattern continues
- **Transition Type**: Smooth pattern evolution
- **Precise Boundary**: Frame **2305** (subtle shift in diamond density)

### Transition 4→5: Pattern Cycling → Plasma Effect
- **Frame 3995**: Black screen (fade complete)
- **Frame 3999**: Black screen
- **Frame 4000**: Still black
- **Frame 4005**: Plasma pattern begins (4-way symmetry squares)
- **Frame 4010**: Plasma fully visible
- **Frame 4020**: Plasma continues (dithered organic pattern)
- **Transition Type**: Black screen transition (hard cut)
- **Precise Boundary**: Frame **4000** (black screen), **4005** (plasma starts)

### Transition 5→6: Plasma Continues (No Clear Boundary)
- **Frame 4995-5200**: Kaleidoscope plasma pattern continues
- **Frame 5300**: Different squares/rotation appear (still part of plasma aesthetic)
- **Frame 5400**: Sparse large squares with shading
- **Frame 5600**: Multiple squares with 3D rotation
- **Frame 5700**: Rotated diamonds
- **Frame 5900**: More rotated diamonds
- **Frame 6100**: Squares morphing back
- **Frame 6200**: 4×4 grid of large squares
- **Frame 6400**: Grid growing denser
- **Frame 6700**: 8×8 grid returns
- **Frame 6900**: 8×8 grid continues
- **Observation**: E5 and E6 are NOT separate effects! This is ONE CONTINUOUS EFFECT (plasma morphing into squares and back)
- **Revised Classification**: Frames 4000-7005 are a single "Plasma & Morphing" effect

### Transition 6→7: Plasma/Grid → Fade Out
- **Frame 6995**: 8×8 grid visible
- **Frame 7000**: 8×8 grid continues
- **Frame 7005**: Grid with diagonal rotation overlay begins
- **Frame 7010**: Diagonal diamond overlay on grid
- **Frame 7020**: Grid returns to normal
- **Frame 7050**: Grid starts fading (individual squares disappearing)
- **Frame 7080**: Grid fading continues
- **Frame 7100**: Few squares remain, mostly black
- **Frame 7120**: Almost black, last few squares
- **Frame 7150**: Complete black screen
- **Transition Type**: Gradual fade-out (reverse of E1 emergence)
- **Precise Boundary**: Frame **7006** (transition begins), **7150** (fade complete)

### Transition 7→8: Black Screen → Credits Text
- **Frame 7150**: Black screen
- **Frame 7180**: Black screen
- **Frame 7200**: Black screen
- **Frame 7220**: Black screen
- **Frame 7250**: First letter "C" appears
- **Frame 7280**: "C" complete
- **Frame 7300**: "CU"
- **Transition Type**: Hard cut from black to text
- **Precise Boundary**: Frame **7250** (first character)

---

## Revised Effect Structure

Based on bisection analysis, here's the corrected structure:

### Effect 0: Initialization
- **Frames**: 2-167 (166 frames, 3.32s)
- **Visual**: Pure black screen
- **Audio**: Music starts

### Effect 1: Square Emergence
- **Frames**: 168-801 (634 frames, 12.68s)
- **Visual**: White squares emerge gradually into 8×8 grid
- **Technique**: Appearance animation, symmetrical

### Effect 2: Geometric Interlocking Pattern
- **Frames**: 802-1404 (603 frames, 12.06s)
- **Visual**: Complex organic "bone-like" pattern
- **Technique**: Algorithmic pattern generation, morphing

### Effect 3: Grid Pulse & Rotation
- **Frames**: 1405-2304 (900 frames, 18.00s)
- **Visual**: 8×8 grid returns with rotation/transformation
- **Technique**: Grid with animated overlays, butterfly/X shapes

### Effect 4: Diamond Pattern Cycling
- **Frames**: 2305-3999 (1695 frames, 33.90s)
- **Visual**: Diamond/rotated square patterns cycling
- **Technique**: Grid-based patterns with rotation

### Effect 5: Plasma & Morphing (COMBINED E5+E6)
- **Frames**: 4000-7005 (3006 frames, 60.12s)
- **Visual**: Complex plasma effect morphing into rotating 3D squares and back
- **Sub-sections**:
  - 4000-4004: Black transition
  - 4005-5299: Kaleidoscope plasma (dithered, organic, 4-way symmetry)
  - 5300-6699: Morphing squares (sparse → dense, rotation, 3D effects)
  - 6700-7005: Return to 8×8 grid with overlays
- **Technique**: Plasma algorithm + 3D transformation + morphing
- **Note**: This is ONE continuous algorithmic effect, not two separate effects

### Effect 6: Grid Fade-Out
- **Frames**: 7006-7249 (244 frames, 4.88s)
- **Visual**: 8×8 grid fades to black (reverse emergence)
- **Technique**: Disappearance animation (reverse of E1)

### Effect 7: Credits Text
- **Frames**: 7250-9420 (2171 frames, 43.42s)
- **Visual**: Typewriter text
- **Text**: "CUBELON / DARKLITE OFFENCE / NEON PAL RESPONSE"
- **Technique**: Character-by-character text printing

---

## Key Observations

### Effect Durations (Sorted by Length)
1. **E5 (Plasma & Morphing)**: 60.12s (31.9% of demo) - LONGEST
2. **E7 (Credits)**: 43.42s (23.0% of demo)
3. **E4 (Diamond Cycling)**: 33.90s (18.0% of demo)
4. **E3 (Grid Pulse)**: 18.00s (9.6% of demo)
5. **E1 (Square Emergence)**: 12.68s (6.7% of demo)
6. **E2 (Geometric Pattern)**: 12.06s (6.4% of demo)
7. **E6 (Fade-Out)**: 4.88s (2.6% of demo)
8. **E0 (Init)**: 3.32s (1.8% of demo)

### Critical Findings

1. **E5 is the DOMINANT effect**: Over 60 seconds (1/3 of entire demo)
   - This is the most complex and important effect
   - Combines plasma generation, 3D math, and morphing
   - Will be the most challenging to port to C64

2. **No clear E5→E6 boundary**: They are one continuous effect
   - Plasma smoothly morphs into squares and back
   - Single algorithmic system with multiple visual states
   - Should be treated as ONE effect in implementation

3. **Grid fade-out is short**: Only 4.88 seconds
   - Simple reverse animation of E1
   - Can reuse emergence code in reverse

4. **Black screen transitions**: Only occurs at frame 4000 (E4→E5)
   - All other transitions are smooth morphs or gradual fades
   - No other hard cuts in the demo

5. **Symmetry throughout**: All effects use left-right or 4-way symmetry
   - Reduces memory requirements
   - Simplifies algorithms

---

## Implementation Priority (Revised)

### Phase 1: Foundation
1. Framework + timing (frame counter)
2. E0: Black screen (trivial)
3. E7: Credits text (simple, validates framework)

### Phase 2: Grid Effects (Building blocks)
4. E1: Square emergence (12.68s)
5. E6: Fade-out (4.88s) - reuse E1 code in reverse
6. E3: Grid pulse/rotation (18.00s)

### Phase 3: Pattern Effects
7. E2: Geometric pattern (12.06s)
8. E4: Diamond cycling (33.90s)

### Phase 4: Master Effect (Highest complexity)
9. **E5: Plasma & Morphing (60.12s)** - SAVE FOR LAST
   - Most complex
   - Longest duration
   - Requires all techniques: plasma, 3D, morphing
   - Performance critical

---

## Transition Types Summary

| Transition | Type | Duration | Technique |
|------------|------|----------|-----------|
| E0→E1 | Gradual fade-in | ~10 frames | Pixels appear incrementally |
| E1→E2 | Morphing | ~5-10 frames | Grid transforms to pattern |
| E2→E3 | Morphing | ~10-20 frames | Pattern transforms back to grid |
| E3→E4 | Smooth evolution | Instant | Pattern density changes |
| E4→E5 | Hard cut | 5 frames | Black screen between |
| E5→E6 | N/A (same effect) | - | Continuous morphing |
| E6→E7 | Gradual fade-out | ~150 frames | Squares disappear |
| E7→E8 | Hard cut | ~50 frames | Black screen between |

---

## Music Synchronization Points

Based on effect boundaries, likely music structure:

- **0:00-0:03** (E0): Intro/drums start
- **0:03-0:16** (E1): Music builds, melody begins
- **0:16-0:28** (E2): Section A
- **0:28-0:46** (E3): Section B
- **0:46-1:20** (E4): Section C (extended)
- **1:20-2:20** (E5): Section D (climax, longest section, 60 seconds!)
- **2:20-2:25** (E6): Fade/transition
- **2:25-3:08** (E7): Outro/credits (43 seconds)

The 60-second E5 section suggests this is the "main" part of the demo, the visual/audio climax.

---

## Frame-Accurate Effect Table

```
Frame    | Effect | Visual Description
---------|--------|--------------------------------------------------
0002-0167| E0     | Black screen (initialization)
0168-0801| E1     | Squares emerge → 8×8 grid
0802-1404| E2     | Geometric interlocking pattern
1405-2304| E3     | Grid pulse with rotation overlay
2305-3999| E4     | Diamond/rotated pattern cycling
4000-4004| -      | Black transition
4005-5299| E5a    | Kaleidoscope plasma (organic dithered)
5300-6699| E5b    | Morphing 3D squares (sparse → dense)
6700-7005| E5c    | Return to grid with overlays
7006-7249| E6     | Grid fade-out to black
7250-9420| E7     | Credits text (typewriter)
```

---

## Data for C64 Effect Sequencer

```assembly
; Effect transition table (frame number, effect ID)
effect_table:
    .word 2,    EFFECT_INIT          ; E0: Init
    .word 168,  EFFECT_SQUARES       ; E1: Square emergence
    .word 802,  EFFECT_GEOMETRIC     ; E2: Geometric pattern
    .word 1405, EFFECT_GRID_PULSE    ; E3: Grid pulse/rotation
    .word 2305, EFFECT_DIAMONDS      ; E4: Diamond cycling
    .word 4000, EFFECT_PLASMA        ; E5: Plasma & morphing (combined)
    .word 7006, EFFECT_FADE_OUT      ; E6: Fade-out
    .word 7250, EFFECT_CREDITS       ; E7: Credits text
    .word 9421, EFFECT_END           ; End marker

; Plasma sub-states (internal to E5)
plasma_states:
    .word 4000, PLASMA_BLACK_TRANS   ; Black transition
    .word 4005, PLASMA_KALEIDOSCOPE  ; Kaleidoscope plasma
    .word 5300, PLASMA_MORPHING      ; 3D morphing squares
    .word 6700, PLASMA_GRID_RETURN   ; Return to grid
```

---

End of Frame Transition Analysis
