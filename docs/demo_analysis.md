# CUBELON Demo Analysis

**Demo Duration**: 9,419 frames (188.4 seconds @ 50Hz)
**Audio**: 3-channel AY-3-8912 music throughout
**Resolution**: ZX Spectrum 256×192
**Color Scheme**: Monochrome (black and white) throughout entire demo
**Frame Counter**: 16-bit counter at memory address 0x8184

## Demo Structure Overview

The CUBELON demo is a visual effects showcase synchronized to AY chip music. It consists of approximately 7-8 distinct visual effects sections that morph and transition into each other, followed by text credits. All effects are monochrome (1-bit black/white).

---

## Part 1: Fade-In / Initialization
**Frame Range**: 2-150 (3.0 seconds)
**Visual**: Black screen

### Technical Analysis
- Pure black screen during initialization
- Music starts playing
- Likely setting up memory, initializing effect routines

---

## Part 2: Emerging Squares Animation
**Frame Range**: 151-800 (13 seconds)
**Visual**: Small white squares emerge and form into an 8×8 grid

### Technical Analysis
- Squares appear gradually from scattered positions
- Animation builds up to full 8×8 grid of white squares
- Smooth emergence effect, possibly sprite-based or character-based
- Frame 200: Few scattered squares in two vertical columns
- Frame 300: More squares appearing, clear left/right symmetry
- Frame 400-600: Continued emergence, squares growing in number
- Frame 800: Complete 8×8 grid established

### Pattern Structure
- Individual square: 12×12 pixels
- Spacing: 12 pixels between squares
- Repeating pattern: **24×48 pixels**
- Grid: 8×8 squares in 180×180 pixel area, centered at (38, 6)
- Alternating rows have horizontal offset (creates "marching" visual effect)

### Effects Identified
- **Square emergence animation**: Objects appearing/fading in
- **Symmetry**: Left-right mirroring suggests algorithmic generation
- **Grid formation**: Targets regular 8×8 pattern
- **Marching pattern**: Alternating row offsets create visual movement

---

## Part 3: Geometric Pattern Morphing
**Frame Range**: 801-1400 (12 seconds)
**Visual**: Complex interlocking geometric pattern

### Technical Analysis
- Frame 900-1200: "Bone-like" or interlocking shapes filling screen
- High visual complexity, dense black and white areas
- Pattern animates/morphs over time

### Pattern Structure
- Active area: **192×192 pixels** (centered with 32-pixel left border)
- Repeating tile: **48×48 pixels**
- Grid layout: 4×4 tiles (16 total, all identical at any given frame)
- Data size: Only 288 bytes per frame (1-bit black/white)
- Very memory efficient - single tile repeated in perfect grid

### Effects Identified
- **Tile-based pattern**: Single 48×48 tile repeated 4×4
- **Morphing/animation**: Tile changes over time (switching between different patterns)
- **Memory efficient**: Only 288 bytes needed per pattern frame

### Z80 Analysis Tasks
1. Locate pattern rendering routine (around frame 800-900)
2. Check if using tile switching (multiple 48×48 patterns) or algorithmic generation
3. Identify animation method (pre-rendered tiles vs generated)
4. Document tile data format and storage

---

## Part 4: Grid Cycling/Pulsing
**Frame Range**: 1401-2300 (18 seconds)
**Visual**: 8×8 grid returns with animation/pulsing

### Technical Analysis
- Frame 1500-2000: Clean 8×8 grid of white squares
- Grid appears static or subtly animating
- Frame 2200: Squares appear rotated/transformed (diagonal orientation)
- Possible rotation or XOR effect creating diamond shapes

### Effects Identified
- **Grid display**: 8×8 matrix of squares
- **Transformation**: Rotation or morphing of square shapes
- **Cycling**: Pattern may be cycling through states

---

## Part 5: Advanced Pattern Effects
**Frame Range**: 2301-4000 (34 seconds)
**Visual**: Multiple geometric patterns cycling

### Technical Analysis
- Frame 2500-3500: Grid patterns continue
- Frame 3900: Different pattern appears (dots/circles pattern)
- Frame 4000: Black screen (transition point)
- This section seems to cycle through multiple pattern variations
- Longer duration suggests multiple sub-effects

### Effects Identified
- **Pattern cycling**: Multiple distinct patterns
- **Transitions**: Clean transitions between patterns
- **Dot/circle patterns**: New visual element introduced

### Z80 Analysis Tasks
1. Locate effect switching logic
2. Identify how many distinct patterns exist in this range
3. Document transition frames precisely
4. Check if patterns are table-driven or algorithmic

---

## Part 6: Dithered/Plasma Effects
**Frame Range**: 4001-5000 (20 seconds)
**Visual**: Complex dithered/plasma-like patterns

### Technical Analysis
- Frame 4100: Fuzzy/dithered pattern with circular elements
- Frame 4200: Similar dithered effect
- Frame 4500: Complex geometric pattern with heavy dithering
- Frame 4800: Dithered fuzzy pattern
- Frame 5000: Kaleidoscope or mandala-like symmetrical pattern
- High visual density, organic appearance
- Likely uses XOR, plasma, or checkerboard dithering techniques

### Effects Identified
- **Plasma effect**: Organic flowing pattern
- **Dithering**: Black/white dithering creates "gray" appearance
- **Kaleidoscope symmetry**: 4-way or 8-way symmetry
- **Animation**: Pattern appears to move/flow

### Z80 Analysis Tasks
1. Locate plasma/pattern routine (around frame 4000-4100)
2. Check for sine/cosine tables
3. Identify coordinate system and formula
4. Look for XOR operations (common in ZX demos)
5. Document animation parameters

---

## Part 7: Morphing Squares/Rotation
**Frame Range**: 5001-7000 (40 seconds)
**Visual**: Squares with rotation, scaling, and morphing

### Technical Analysis
- Frame 5500: Rotated/tilted squares pattern
- Frame 5800: Different dithered/shaded pattern
- Frame 6000: Squares with apparent shading or 3D effect
- Frame 6300: Grid transitioning, mix of small and large squares
- Frame 6500-7000: 8×8 grid returns
- This section appears to be morphing between square-based effects
- Possible 3D transformation or perspective effects

### Effects Identified
- **Rotation**: Squares rotated at various angles
- **Scaling**: Squares changing size
- **Shading/dithering**: Pseudo-3D appearance
- **Morphing**: Smooth transitions between states

### Z80 Analysis Tasks
1. Locate square transformation routine
2. Check for rotation matrices or lookup tables
3. Identify if using fixed-point math
4. Document scaling/morphing algorithm
5. Analyze frame-by-frame to understand transition timing

---

## Part 8: Credits Sequence
**Frame Range**: 7001-9419 (48 seconds)
**Visual**: Text appearing letter-by-letter

### Technical Analysis
- Frame 7500: "CUBE" appears
- Frame 7800: "CUBELON"
- Frame 8000: "CUBELON DARK"
- Frame 8200: "CUBELON DARKLITE O"
- Frame 8500: "CUBELON DARKLITE OFFENCE N"
- Frame 9000: "CUBELON DARKLITE OFFENCE NEON PAL RESPON"
- Frame 9400: "CUBELON DARKLITE OFFENCE NEON PAL RESPONSE"
- Text appears to be typed out character-by-character
- Clean monospaced font
- Text positioned in upper-left area of screen

### Text Decoded
```
CUBELON
DARKLITE OFFENCE
NEON PAL RESPONSE
```

These appear to be group/handle names (demoscene credits).

### Effects Identified
- **Typewriter effect**: Characters appear one at a time
- **Monospaced font**: ZX Spectrum character set or custom font
- **Static display**: Text remains after typing completes

---

## Summary of Effects

### Critical Technical Effects (Require Z80 Analysis)
1. **Geometric Pattern Generator** (Part 3, frames 801-1400)
   - Complex interlocking shapes
   - Likely algorithmic, need to understand generation method

2. **Plasma/Dithered Effect** (Part 6, frames 4001-5000)
   - Most visually complex section
   - Requires understanding plasma generation or XOR patterns

3. **Rotation/Transformation Engine** (Part 7, frames 5001-7000)
   - Square rotation and morphing
   - Need transformation mathematics

### Medium Complexity Effects
4. **Square Emergence Animation** (Part 2, frames 151-800)
   - Appearance/fade-in algorithm
   - Timing synchronization

5. **Grid Transformation** (Part 4, frames 1401-2300)
   - Rotation of square elements
   - Character animation

6. **Pattern Cycling System** (Part 5, frames 2301-4000)
   - Multiple sub-effects to identify
   - Transition system

### Simple Effects
7. **Typewriter Credits** (Part 8, frames 7001-9419)
   - Text output with timing
   - Font rendering

---

## Z80 Disassembly Priorities

### Phase 1: Framework Analysis
1. **Main demo loop** (highest priority)
   - Frame counter at 0x8184 (16-bit)
   - Effect sequencing logic
   - Transition triggers

2. **Frame timing system**
   - Interrupt handler
   - Frame synchronization
   - Music sync points

### Phase 2: Effect Routines (in order of appearance)
1. **Square emergence** (frame 151-800)
   - Look for routines called around frame 150-200
   - Check for sprite/character setup

2. **Geometric pattern generator** (frame 801-1400)
   - Pattern calculation algorithm
   - Symmetry implementation

3. **Grid animation** (frame 1401-2300)
   - Rotation logic
   - Grid rendering

4. **Pattern cycling** (frame 2301-4000)
   - Effect table/array
   - Switching mechanism

5. **Plasma/dither effects** (frame 4001-5000)
   - Plasma algorithm
   - Lookup tables (sine/cosine)

6. **Transformation engine** (frame 5001-7000)
   - Rotation matrices
   - Scaling calculations

7. **Credits text** (frame 7001-9419)
   - Text printing routine
   - Typewriter timing

### Phase 3: Support Systems
- **Music player integration**
- **Transition effects**
- **Memory management**

---

## Music and Synchronization

### Music Analysis Needed
- Extract AY register writes from emulator
- Identify music patterns and structure
- Determine if music has distinct sections matching visual parts

### Sync Points (approximate)
- Frame 151: Music intensifies, visuals begin
- Frame 801: Pattern change, likely music section change
- Frame 4001: Major transition, plasma effect begins
- Frame 7001: Music may fade, credits begin

### Critical Timing
- Demo is timing-sensitive: effects must sync to music
- Use frame counter for synchronization (like original)

---

## Next Steps: Immediate Actions

### 1. Detailed Frame Analysis (4-8 hours)
- Watch frames 2300-4000 in detail (Part 5)
- Identify exact transition points
- Create frame-by-frame log of major changes
- Screenshot key frames for reference

### 2. Z80 Disassembly Setup (2-4 hours)
- Set up z80dasm or similar disassembler
- Disassemble all code blocks:
  - cubelon_data_10.bin (4595 bytes) - likely main code
  - cubelon_data_11.bin (11751 bytes) - major code section
  - cubelon_data_12.bin (9446 bytes) - additional code
- Create annotated disassembly file

### 3. Main Loop Analysis (8-12 hours)
- Locate main demo loop in Z80 code
- Find frame counter at 0x8184 usage
- Identify effect switching mechanism
- Document interrupt handler
- Map out effect table/sequence

### 4. Tool Development (4-8 hours)
- Create frame comparison tool (Python)
- Build effect extraction tool (view specific frame ranges)
- Create memory map visualization tool

---

## Critical Questions to Answer During Analysis

1. **Frame counter**: How is 0x8184 incremented? IRQ handler or main loop?
2. **Effect table**: Is there an effect list/table, or hardcoded switches?
3. **Pattern generation**: Are patterns algorithmic or pre-rendered?
4. **Music sync**: How are effects synchronized to music? Frame counts or music player events?
5. **Transitions**: Are transitions separate routines or part of effects?
6. **Memory layout**: Where are pattern tables, sine tables, character data?
7. **Screen updates**: Double buffering? Direct writes? Page flipping?
8. **Performance**: Are there wait loops? Frame skipping? Timing margins?

---

## Documentation Deliverables

1. **demo_analysis.md** (this document)
2. **z80_disassembly.txt** - Annotated Z80 disassembly
3. **memory_map.md** - ZX Spectrum memory layout
4. **effects_list.md** - Detailed effect catalog
5. **frame_transitions.md** - Frame-by-frame timing breakdown
6. **algorithm_notes.md** - Pattern generation algorithms documented

---

## Complexity Assessment

| Part | Description | Complexity | Analysis Priority |
|------|-------------|------------|-------------------|
| 1 | Black screen | Trivial | Low |
| 2 | Square emergence | Medium | Medium |
| 3 | Geometric pattern | High | High |
| 4 | Grid transform | Medium | Medium |
| 5 | Pattern cycling | High | High |
| 6 | Plasma/dither | Very High | Very High |
| 7 | Rotation/morph | Very High | Very High |
| 8 | Credits text | Low | Low |

---

## End of Analysis Document

This document will be updated as Z80 disassembly reveals more details about effect implementations and algorithms.
