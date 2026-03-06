# CUBELON Effects Catalog

Quick reference for all visual effects in the demo, organized by complexity.

---

## Effect Index

| ID | Name | Frame Range | Duration | Complexity |
|----|------|-------------|----------|------------|
| E0 | Initialization/Fade-In | 2-150 | 3.0s | Trivial |
| E1 | Square Emergence | 151-800 | 13.0s | Medium |
| E2 | Geometric Interlocking Pattern | 801-1400 | 12.0s | High |
| E3 | Grid Pulse & Rotation | 1401-2300 | 18.0s | Medium |
| E4 | Pattern Cycling (Multi-effect) | 2301-4000 | 34.0s | High |
| E5 | Plasma/Dithered Organic | 4001-5000 | 20.0s | Very High |
| E6 | 3D Rotation & Morphing | 5001-7000 | 40.0s | Very High |
| E7 | Typewriter Credits | 7001-9419 | 48.4s | Low |

---

## E0: Initialization/Fade-In
**Frame Range**: 2-150 (3.0 seconds)

### Visual Description
- Pure black screen
- Music starts playing
- No visible animation

### Technical Requirements
- Clear screen to black
- Initialize music player
- Set up demo framework
- Wait for frame 150

---

## E1: Square Emergence
**Frame Range**: 151-800 (13.0 seconds, 650 frames)

### Visual Description
- Small white squares appear gradually
- Squares emerge from scattered positions
- Build into complete 8×8 grid
- Left-right symmetry
- Smooth animation over 650 frames

### Technical Requirements
- 64 squares total (8×8 grid)
- Animation system for position/visibility
- Symmetry logic (mirror left/right)
- Timing: each square appears at specific frame

### Sample Frames
- Frame 200: ~8 squares visible
- Frame 400: ~24 squares visible
- Frame 600: ~48 squares visible
- Frame 800: All 64 squares visible

### Likely Algorithm
```
For each frame F (151-800):
    visible_count = (F - 150) / 10  ; ~1 square every 10 frames
    For each square S (0-63):
        If appearance_order[S] < visible_count:
            Draw square at position[S]
```

### Data Structures Needed
```
appearance_order[64]:  ; Order squares appear (0-63 randomized?)
position_x[64]:        ; X coordinate for each square
position_y[64]:        ; Y coordinate for each square
```

### Z80 Analysis Tasks
1. Locate routine called around frame 150-200
2. Find appearance order table (64 bytes)
3. Identify position calculation or table
4. Check for symmetry implementation
5. Document timing algorithm

---

## E2: Geometric Interlocking Pattern
**Frame Range**: 801-1400 (12.0 seconds, 600 frames)

### Visual Description
- Complex organic/geometric pattern fills screen
- "Bone-like" or interlocking shapes
- High density black/white areas
- Horizontal and vertical symmetry
- Pattern appears to animate/shift

### Technical Requirements
- Full-screen pattern generation
- Symmetry (4-way or 2-way)
- Animation (pattern morphs or scrolls)
- Algorithmic or table-driven

### Visual Characteristics
- Repeating motifs
- Organic appearance (not grid-based)
- High visual complexity
- Smooth transitions

### Likely Algorithms
1. **XOR Pattern**:
   ```
   pixel[x,y] = pattern_table[x & mask] XOR pattern_table[y & mask]
   ```

2. **Symmetry**:
   ```
   Draw quadrant 1
   Mirror to quadrant 2 (flip X)
   Mirror to quadrant 3 (flip Y)
   Mirror to quadrant 4 (flip X+Y)
   ```

3. **Animation**:
   ```
   pattern_offset += 1 per frame
   Apply offset to coordinates before lookup
   ```

### Z80 Analysis Tasks (CRITICAL)
1. Locate pattern rendering routine (around frame 800-850)
2. Identify pattern generation method:
   - Check for XOR operations (very common)
   - Look for lookup tables
   - Find coordinate transformations
3. Document symmetry implementation
4. Analyze animation parameters
5. Extract any pattern data tables
6. Estimate CPU usage (performance check)

---

## E3: Grid Pulse & Rotation
**Frame Range**: 1401-2300 (18.0 seconds, 900 frames)

### Visual Description
- 8×8 grid of white squares returns
- Squares appear to pulse or animate
- Frame 2200: Squares rotated to diamond orientation
- Smooth transition between orientations

### Technical Requirements
- 8×8 grid rendering (same as E1)
- Rotation animation (squares → diamonds)
- Multiple frames of character graphics
- Timing system for animation

### Animation Timing
```
charset_frame = (current_frame - 1400) / animation_speed
charset_frame = charset_frame MOD 8
```

### Z80 Analysis Tasks
1. Locate grid rendering (should be similar to E1)
2. Find rotation animation routine
3. Check if using character redefinition or sprite rotation
4. Document rotation steps
5. Identify timing parameters

---

## E4: Pattern Cycling (Multi-Effect Section)
**Frame Range**: 2301-4000 (34.0 seconds, 1700 frames)

### Visual Description
- Multiple distinct patterns appear in sequence
- Frame 2500-3500: Grid patterns
- Frame 3900: Dot/circle pattern appears
- Frame 4000: Transition to black
- Longest single section of demo

### Technical Requirements
- Multiple sub-effects to implement
- Transition system between patterns
- Effect sequencer
- Timing management

### Known Sub-Effects
1. **Grid Pattern A** (2301-2500): Grid continuation
2. **Grid Pattern B** (2501-3500): Modified grid
3. **Dot/Circle Pattern** (3501-3900): New pattern type
4. **Fade to Black** (3901-4000): Transition out

### Z80 Analysis Tasks (HIGH PRIORITY)
1. **Detailed frame analysis** (watch frames 2300-4000 carefully)
2. Identify exact transition frames
3. Locate effect switching code
4. Document each sub-effect separately
5. Check for shared routines vs separate implementations

### Next Step Required
**Must watch frames 2300-4000 in detail before implementation**

---

## E5: Plasma/Dithered Organic
**Frame Range**: 4001-5000 (20.0 seconds, 1000 frames)

### Visual Description
- Complex organic/flowing pattern
- Heavy dithering (checkerboard/noise pattern)
- Circular/kaleidoscope elements
- 4-way or 8-way symmetry
- Pattern appears to move/animate
- Most visually complex effect in demo

### Technical Requirements
- Full-screen pattern generation
- Plasma generation algorithm
- Sine/cosine lookup tables
- Animation/scrolling parameters
- High performance (full screen update each frame)

### Visual Characteristics
- Frame 4100: Fuzzy circles with dithering
- Frame 4500: Dense geometric pattern
- Frame 5000: Kaleidoscope/mandala symmetry
- Organic, flowing appearance
- Pseudo-3D depth effect via dithering

### Data Structures
```
sine_table[256]:     ; Pre-calculated sine values (0-255)
cosine_table[256]:   ; Pre-calculated cosine values
dither_pattern[16]:  ; 4×4 dithering pattern
time_offset:         ; Animation frame counter
```

### Likely Algorithm (ZX Spectrum Plasma)
```assembly
; Pseudo-code
for y = 0 to 191:
    for x = 0 to 255:
        v1 = sine_table[(x + time) & 255]
        v2 = sine_table[(y + time) & 255]
        v3 = sine_table[((x + y) / 2 + time) & 255]
        value = (v1 + v2 + v3) / 3

        ; Apply dithering
        threshold = dither_pattern[x & 3, y & 3]
        pixel = (value > threshold) ? WHITE : BLACK

        plot_pixel(x, y, pixel)
```

### Z80 Analysis Tasks (CRITICAL - HIGHEST PRIORITY)
1. Locate plasma routine (around frame 4000-4050)
2. Document exact algorithm used
3. Measure performance (cycles per pixel)
4. Extract sine/cosine tables
5. Identify animation parameters
6. Check for optimization tricks
7. Determine if using shortcuts (not full plasma)

---

## E6: 3D Rotation & Morphing Squares
**Frame Range**: 5001-7000 (40.0 seconds, 2000 frames)

### Visual Description
- Squares rotating in 3D space
- Scaling (size changes)
- Perspective effects
- Shading via dithering (pseudo-3D)
- Multiple transformation states
- Morphing between configurations

### Technical Requirements
- 3D mathematics (rotation matrices)
- Fixed-point arithmetic
- Sine/cosine for rotation angles
- Perspective projection
- Shading/dithering system
- Animation sequencing

### Visual Breakdown
- Frame 5500: Tilted/rotated squares
- Frame 5800: Different rotation angle, dithered
- Frame 6000: Squares with 3D shading
- Frame 6300: Morphing between sizes
- Smooth continuous animation

### 3D Mathematics Required
```
Rotation around Y-axis:
    x' = x * cos(angle) - z * sin(angle)
    y' = y
    z' = x * sin(angle) + z * cos(angle)

Perspective projection:
    screen_x = (x * focal_length) / (z + distance)
    screen_y = (y * focal_length) / (z + distance)

3D Square vertices (8 points):
    (-1,-1,-1), (+1,-1,-1), (+1,+1,-1), (-1,+1,-1)  ; Front
    (-1,-1,+1), (+1,-1,+1), (+1,+1,+1), (-1,+1,+1)  ; Back
```

### Data Structures
```
sine_table[256]      ; Rotation angles
cosine_table[256]
vertex_x[8]          ; 3D coordinates (fixed-point)
vertex_y[8]
vertex_z[8]
screen_x[8]          ; 2D projected coordinates
screen_y[8]
rotation_angle_x     ; Current rotation angles
rotation_angle_y
rotation_angle_z
scale_factor         ; For morphing/scaling
```

### Likely Algorithm
```assembly
; Per frame:
update_rotation_angles()

for each vertex (8 vertices):
    ; Apply rotation matrix (Y-axis example)
    temp_x = vertex_x * cos(angle_y) - vertex_z * sin(angle_y)
    temp_z = vertex_x * sin(angle_y) + vertex_z * cos(angle_y)

    ; Perspective projection
    screen_x = (temp_x * focal) / (temp_z + distance)
    screen_y = (vertex_y * focal) / (temp_z + distance)

    ; Scale
    screen_x = screen_x * scale_factor
    screen_y = screen_y * scale_factor

    ; Store projected point
    save_screen_coords(screen_x, screen_y)

; Draw filled squares (connect vertices, fill)
draw_filled_quad(vertices 0-3, color)
draw_filled_quad(vertices 4-7, color)
; etc...

; Apply dithering for depth/shading
apply_dithering_by_z_depth()
```

### Z80 Analysis Tasks (CRITICAL)
1. Locate 3D transformation routine (frame 5000-5100)
2. Identify math precision (8-bit? 16-bit? fixed-point?)
3. Extract rotation matrices or calculation method
4. Find multiplication implementation
5. Document vertex count and geometry
6. Analyze frame-by-frame animation (rotation speed)
7. Check for simplifications vs full 3D

---

## E7: Typewriter Credits
**Frame Range**: 7001-9419 (48.4 seconds, 2419 frames)

### Visual Description
- Text appears character-by-character
- Monospaced font
- Upper-left screen area
- Clean, simple presentation

### Text Content
```
CUBELON
DARKLITE OFFENCE
NEON PAL RESPONSE
```

### Technical Requirements
- Text printing routine
- Frame-based timing (1 char every N frames)
- Character positioning
- Font (ZX or custom)

### Character Timing
- Total: ~40 characters (including spaces)
- Duration: 2419 frames
- Speed: ~60 frames per character (1.2 seconds per char)

### Likely Implementation
```assembly
credits_text:
    .byte "CUBELON", 13
    .byte "DARKLITE OFFENCE", 13
    .byte "NEON PAL RESPONSE", 0

credits_routine:
    ; Check if enough frames elapsed
    lda frame_counter
    cmp next_char_frame
    bcc :+

    ; Print next character
    ldx char_index
    lda credits_text,x
    beq done               ; 0 = end
    cmp #13
    beq newline

    jsr print_char
    inc char_index

    ; Set next character frame
    lda frame_counter
    clc
    adc #60                ; 60 frames delay
    sta next_char_frame
:   rts
```

### Z80 Analysis Tasks
1. Locate text printing routine (around frame 7000-7100)
2. Extract text strings
3. Document character timing
4. Check font (standard ZX or custom)
5. Verify text positioning

---

## Analysis Priorities

### Phase 1: Framework Analysis (Critical)
1. **Main demo loop**
   - Frame counter (0x8184 equivalent)
   - Effect sequencing logic
   - Transition triggers

2. **Frame timing system**
   - Interrupt handler
   - Frame synchronization
   - Music sync points

### Phase 2: Effect Routines (in order of complexity)
1. **E7: Credits** - Simple, validates framework
2. **E1: Square Emergence** - First visual effect
3. **E3: Grid Rotation** - Character animation
4. **E2: Geometric Pattern** - First bitmap effect
5. **E4: Pattern Cycling** - Multiple sub-effects
6. **E5: Plasma** - Most complex algorithm
7. **E6: 3D Rotation** - 3D mathematics

---

## Summary Statistics

- **Total Effects**: 8
- **Total Frames**: 9,419 (188.4 seconds)
- **Trivial Effects**: 1 (E0)
- **Low Complexity**: 1 (E7)
- **Medium Complexity**: 2 (E1, E3)
- **High Complexity**: 2 (E2, E4)
- **Very High Complexity**: 2 (E5, E6)

**Complexity Assessment**:
- Critical: E5 (plasma), E6 (3D) - most complex
- High: E2, E4 - algorithmic patterns
- Medium: E1, E3 - grid-based
- Low: E0, E7 - simple

---

End of Effects Catalog
