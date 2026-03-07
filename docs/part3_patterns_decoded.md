# Part 3 Patterns - All 12 Decoded

## Summary

Successfully decoded all 12 unique 48×48 pixel patterns used in Part 3 of CUBELON (frames 900-960).

## Pattern Catalog

| Address | First Frame | Description |
|---------|-------------|-------------|
| `$BE06` | 900 | Initial static pattern - complex 3D diamond/cube structure |
| `$C946` | 920 | Morph step 1 - transition begins |
| `$C8FE` | 922 | Morph step 2 |
| `$C8B6` | 924 | Morph step 3 |
| `$C86E` | 926 | Morph step 4 |
| `$C826` | 928 | Morph step 5 |
| `$C7DE` | 930 | Morph step 6 - mostly empty |
| `$C796` | 932 | Morph step 7 - completely empty |
| `$C74E` | 934 | Morph step 8 - completely empty |
| `$C706` | 936 | Morph step 9 - final morph state |
| `$C046` | 938 | Final static pattern - different 3D structure |
| `$C98E` | 960 | Transition pattern (1 frame only) |

## Pattern Details

### Storage
- **Size**: 288 bytes each (48×48 pixels = 2,304 pixels ÷ 8 bits = 288 bytes)
- **Format**: Linear row-major order (6 bytes per row × 48 rows)
- **Location**: Bank 2 memory ($C000-$FFFF range)
- **Total size**: 12 patterns × 288 bytes = 3,456 bytes

### Pattern Sequence

**Frames 900-919 (20 frames, 400ms):**
- Pattern `$BE06` - Static display of complex 3D structure

**Frames 920-939 (20 frames, 400ms, 10 morph steps at 2 frames each):**
1. `$C946` → `$C8FE` → `$C8B6` → `$C86E` → `$C826` (dissolving)
2. `$C7DE` → `$C796` → `$C74E` (mostly/completely empty)
3. `$C706` (new structure emerging)

**Frames 938-959 (22 frames, 440ms):**
- Pattern `$C046` - Static display of final 3D structure

**Frame 960 (1 frame, 20ms):**
- Pattern `$C98E` - Brief transition pattern

## Visual Characteristics

The patterns show **abstract 3D geometric structures** with:
- Diamond/cube shapes
- Interlocking patterns
- Gradual dissolution in morphing sequence
- Empty frames in middle of morph (frames 932-934)
- Re-emergence of new structure

The morphing effect creates a smooth visual transition by:
1. Gradually removing pixels from first structure
2. Passing through blank frames
3. Gradually adding pixels for second structure

## Files Generated

### Individual Patterns
- `part3_pattern_BE06_frame900.png` - Initial pattern
- `part3_pattern_C046_frame938.png` - Final pattern
- `part3_pattern_C946_frame920.png` through `part3_pattern_C706_frame936.png` - Morph sequence
- `part3_pattern_C98E_frame960.png` - Transition

### Sprite Sheet
- `part3_all_12_patterns.png` - All 12 patterns in 4×3 grid (768×576 pixels)

## Technical Implementation

From disassembly analysis ([part3_pattern_analysis.md](part3_pattern_analysis.md)):
- Patterns are **pre-stored** in memory (not computed)
- Drawing routine uses fast stack-based copy (`POP BC` + `LD (HL),r`)
- Pattern table at `$84C0` maps tiles to patterns
- Morphing achieved by changing pattern table pointers, not modifying pattern data
- No arithmetic operations on pattern data - pure memory copy

## Usage for C64 Port

These decoded patterns can be:
1. Converted to C64 bitmap format (multicolor or hires)
2. Used as reference for recreating the visual effect
3. Analyzed for compression opportunities
4. Adapted for C64's different resolution (320×200 vs ZX 256×192)

The morphing sequence shows exactly how the visual transition works and can be replicated frame-by-frame on the C64.

---

**Generated**: 2026-03-07
**Source**: `emulator/part3_tiles_900-960.json` (61 frames, 12 unique patterns)
**Tool**: `tools/decode_all_part3_patterns.py`
