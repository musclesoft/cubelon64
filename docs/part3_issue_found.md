# Part 3 Extraction Issue - Root Cause Found

## Problem

Extracted patterns don't match what's displayed on screen.

## Root Cause

**The screen displays a 4×4 GRID of 48×48 tiles (16 total), each using a different pattern from the pattern table.**

My extraction code only captures ONE pattern per frame (the first unique address encountered), but frame 920 actually uses MULTIPLE patterns:
- Pattern $C946
- Pattern $C706
- Possibly others ($C8B6, $C74E, etc.)

## Evidence

From code trace of frame 920:
```
85A7: EB F9 EB D9 | DE:C946 HL:84C2  ← Pattern table entry 0: $C946
85A7: EB F9 EB D9 | DE:C706 HL:84C4  ← Pattern table entry 1: $C706
85A7: EB F9 EB D9 | DE:C946 HL:84C6  ← Pattern table entry 2: $C946
85A7: EB F9 EB D9 | DE:C706 HL:84C8  ← Pattern table entry 3: $C706
```

The pattern addresses alternate! This means different tiles on screen use different patterns.

## Screen Layout

```
ZX Spectrum screen: 256×192 pixels
After cropping: 192×192 pixels
Tile size: 48×48 pixels
Grid: 192/48 = 4×4 = 16 tiles per screen
```

## Pattern Table Structure

Located at $84C0 in Bank 2:
- 16 entries (one per tile)
- 2 bytes per entry (little-endian address)
- Points to patterns in Bank 2

Example from frame 920:
```
Tile  0: $C946
Tile  1: $C706
Tile  2: $C946
Tile  3: $C706
Tile  4: $C946
...
```

## What Needs To Be Fixed

### 1. Capture All Patterns Per Frame

Current code:
```go
if _, exists := part3CapturedPatterns[addr]; !exists {
    // Only captures first occurrence of each address
}
```

Should capture:
- ALL 16 pattern addresses from pattern table
- Store as array indexed by tile position, not map by address

### 2. Export Full Screen Grid

Instead of exporting "one pattern per frame", export:
```json
{
  "frame": 920,
  "tiles": [
    {"position": 0, "pattern_addr": 0xC946, "pixels": [...]},
    {"position": 1, "pattern_addr": 0xC706, "pixels": [...]},
    ...
  ]
}
```

### 3. Render Full Screen

Visualizations should show the complete 4×4 grid, not individual tiles.

## Next Steps

1. Modify capture code to read entire pattern table (16 entries at $84C0)
2. Capture all 16 patterns used in each frame
3. Export as complete screen with tile positions
4. Update visualization to render 4×4 grid
5. Verify against actual screen frames

## Why This Matters

The "morphing" animation works by:
- Keeping most pattern data static
- Changing which patterns are assigned to which tile positions in the pattern table
- Creating the illusion of morphing by rearranging tiles

This is much more efficient than recomputing 16 full patterns every frame!

---

**Status**: Root cause identified, fix needed
**Estimated effort**: 2-3 hours to implement full grid capture and rendering
