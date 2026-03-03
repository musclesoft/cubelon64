# CUBELON Reference Implementation

This directory contains high-level reference implementations of the CUBELON pattern generation algorithm. These tools allow you to verify the algorithm visually before porting to C64 assembly.

## Purpose

**Why this approach?**
- ✅ Visual verification - See patterns immediately
- ✅ Fast iteration - Test changes in browser instantly
- ✅ Algorithm debugging - Much easier than assembly
- ✅ Frame-by-frame comparison - Compare with ZX Spectrum original
- ✅ Export ready data - Generate C64-ready hex directly

**The Problem:** Debugging C64 assembly blindly is extremely difficult. With these tools, you can verify the algorithm works correctly in a high-level language first, then port with confidence.

## Files

### pattern_generator.js
Core pattern generation algorithms in JavaScript.

**Functions:**
- `initTiles()` - Initialize all 11 base tiles
- `generatePattern(frame, tiles)` - 3x3 simple pattern
- `generatePattern8x8(frame, tiles, rotation)` - Full 8x8 grid
- `generatePatternZXAlgorithm(frame, tiles)` - ZX Spectrum faithful reproduction
- `xorTiles(tile1, tile2)` - XOR combine two tiles
- `rotateTileLeft/Right(tile, n)` - Circular byte rotation

**Usage:**
```bash
node pattern_generator.js
```

### pattern_viewer.html
Basic 3x3 pattern viewer with animation.

**Features:**
- 3x3 grid display
- Frame toggle (0 ↔ 1)
- Auto-play mode
- Individual tile preview
- Hex value display

**Usage:**
```bash
open pattern_viewer.html
```

### pattern_viewer_8x8.html
Advanced viewer with full features.

**Features:**
- **3 Algorithm Modes:**
  - 3x3 Simple - Basic pattern
  - 8x8 Pattern - Full grid with rotation
  - ZX Algorithm - Faithful ZX reproduction

- **Rotation Control:**
  - Slider (0-7 positions)
  - Left/Right buttons
  - Circular byte-level rotation

- **Animation:**
  - Frame toggle
  - Auto-play (200ms intervals)
  - Prev/Next frame buttons
  - 10 frames support

- **Export:**
  - C64 hex assembly format
  - Binary format (8 bits per line)
  - ASCII art visualization
  - Copy to clipboard

- **Display:**
  - Large scaled view (10x)
  - Pixel-perfect 1x view (actual 64x64 output)
  - Side-by-side comparison

**Usage:**
```bash
open pattern_viewer_8x8.html
```

### frame_compare.html
Side-by-side comparison with ZX Spectrum original.

**Features:**
- **Load Reference:**
  - Upload ZX Spectrum screenshot (PNG/GIF)
  - Paste hex data from memory dump
  - Automatic tile parsing

- **Auto-Match:**
  - Tests all frame/rotation combinations
  - Finds best match automatically
  - Shows match percentage

- **Comparison:**
  - Pixel-by-pixel difference map
  - Red highlights show mismatches
  - Statistics (match %, pixel counts)
  - Export difference image

**Usage:**
```bash
open frame_compare.html
```

## Workflow

### 1. Basic Pattern Verification
```bash
# Open basic viewer
open pattern_viewer.html

# Verify:
- Frame 0 shows blank + checkerboard
- Frame 1 shows cross + border
- Center tile is XOR combination
- Animation toggles smoothly
```

### 2. Full Algorithm Testing
```bash
# Open advanced viewer
open pattern_viewer_8x8.html

# Test each mode:
1. Click "3x3 Simple" - verify basic pattern
2. Click "8x8 Pattern" - test full grid
3. Click "ZX Algorithm" - test ZX reproduction

# Test rotation:
1. Move rotation slider (0-7)
2. Verify patterns rotate correctly
3. Try different frames + rotations

# Export C64 data:
1. Set desired frame/rotation
2. Click "Export as Hex (C64)"
3. Copy assembly code
4. Paste into C64 .s file
```

### 3. Compare with Original
```bash
# Open comparison tool
open frame_compare.html

# Load ZX Spectrum reference:
Option A: Load screenshot
  - Click "Choose File"
  - Select ZX Spectrum screenshot

Option B: Paste hex data
  - Copy memory dump from ZX emulator
  - Paste into textarea
  - Click "Load Hex Data"

# Find best match:
1. Click "Auto-Match Parameters"
2. Wait for analysis (tests 240 combinations)
3. Review match percentage
4. Examine difference map (red = mismatch)

# Manual tuning:
1. Adjust frame/rotation sliders
2. Click "Generate Reference Pattern"
3. Compare visually
4. Export when satisfied
```

### 4. Port to C64 Assembly

Once verified correct:

```bash
# From pattern_viewer_8x8.html:
1. Set correct frame/rotation
2. Click "Export as Hex (C64)"
3. Copy assembly code

# In C64 project:
1. Create/edit src/data/pattern_data.s
2. Paste exported hex data
3. Add appropriate labels and comments
4. Reference in main code
```

## Algorithm Notes

### Tile Format
- Each tile is 8x8 pixels
- Stored as 8 bytes (one byte per row)
- Each bit represents one pixel (1=on, 0=off)
- MSB is leftmost pixel

### Frame Selection
- **Frame 0:** Static patterns (tiles 0-7)
- **Frame 1:** Animated patterns (tiles 8-10)
- Frame determines which tiles are used as base

### Rotation
- Rotates tile bytes circularly
- Amount 0-7 (number of positions)
- Example: [A,B,C,D,E,F,G,H] rotated 2 → [C,D,E,F,G,H,A,B]

### XOR Combination
- Combines two tiles with exclusive-OR
- Creates complex patterns from simple ones
- Result[i] = Tile1[i] ^ Tile2[i]

### Grid Patterns
- **3x3:** Simple alternating pattern with XOR center
- **8x8:** Full grid with alternating and combined tiles
- **ZX Algorithm:** Faithful reproduction of ZX Spectrum $C076-$C096

## Example Output

### Frame 0, Rotation 0, 3x3 Mode:
```
        █ █ █ █
█ █ █ █ █ █ █ █
        █ █ █ █
█ █ █ █ █ █ █ █
        █ █ █ █
█ █ █ █ █ █ █ █
        █ █ █ █
█ █ █ █ █ █ █ █

█ █ █ █         █ █ █ █
█ █ █ █ █ █ █ █ █ █ █ █
█ █ █ █         █ █ █ █
█ █ █ █ █ █ █ █ █ █ █ █
█ █ █ █         █ █ █ █
█ █ █ █ █ █ █ █ █ █ █ █
█ █ █ █         █ █ █ █
█ █ █ █ █ █ █ █ █ █ █ █

        █ █ █ █
█ █ █ █ █ █ █ █
        █ █ █ █
█ █ █ █ █ █ █ █
        █ █ █ █
█ █ █ █ █ █ █ █
        █ █ █ █
█ █ █ █ █ █ █ █
```

### C64 Export Format:
```assembly
; Pattern data for C64 assembly
; Mode: 3x3, Frame: 0, Rotation: 0

; Tile [0,0]
.byte $00, $00, $00, $00, $00, $00, $00, $00

; Tile [0,1]
.byte $AA, $55, $AA, $55, $AA, $55, $AA, $55

; Tile [0,2]
.byte $00, $00, $00, $00, $00, $00, $00, $00
```

## Tips

1. **Start Simple:** Begin with 3x3 mode to understand basics
2. **Compare Early:** Use frame_compare.html to verify against ZX original
3. **Export Often:** Generate C64 data regularly during development
4. **Test Rotation:** Try all 8 rotations to find best visual effect
5. **Animation:** Use auto-play to see smooth frame transitions

## Next Steps

Once patterns are verified:

1. ✅ **Verified Pattern** - Working in reference implementation
2. 📝 **Export Hex** - Get C64 assembly data
3. 🎯 **Port to Assembly** - Translate algorithm to 6502
4. ✅ **Verify on C64** - Test in VICE emulator
5. 🎮 **Compare** - Use frame_compare.html to confirm match

## Benefits Over Assembly-First Approach

| Aspect | Assembly-First | Reference-First |
|--------|----------------|-----------------|
| Debug Time | Hours/Days | Minutes |
| Visual Feedback | None (blind) | Immediate |
| Iteration Speed | Very Slow | Instant |
| Error Detection | Difficult | Easy |
| Algorithm Verification | Manual/Guess | Visual Proof |
| Data Export | Manual typing | Copy/Paste |
| Comparison | Impossible | Built-in |

**Result:** Reference-first approach is 10-100x faster for algorithm development.
