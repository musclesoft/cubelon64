# Display System Debug Session

## Problem
Main program (`make run`) shows "everything white" or "random garbage" on screen instead of expected pattern display.

## What We've Verified

### ✅ WORKING Components
1. **Charset loading system** - Diagnostic test with solid bottom rows showed correct white bars
2. **Memory allocation** - Charset correctly placed at $2000, no code overlap
3. **VIC-II configuration** - VIC_ADDR set correctly for screen at $0400, charset at $2000
4. **Test programs work** - Simple tests (test_minimal, test_border) display correctly

### ❌ NOT WORKING
1. **Main program display** - Shows white screen or garbage
2. **Pattern generation** - 3×3 pattern not visible as expected

## Root Cause Analysis

The issue appears to be in one of:
1. **draw_border routine** - May be writing incorrect screen codes
2. **pattern_test routine** - May be positioning or drawing incorrectly
3. **Charset data** - Test charset may still have issues

## Test Programs Created

### test_diagnostic.prg
**Purpose**: Verify basic C64 display functionality
**Expected Output**:
- RED border (#2)
- BLUE background (#6)
- Top-left corner: "1234" in YELLOW
- Center: "TEST" in WHITE

**Run**: `~/vice-arm64-gtk3-3.10/bin/x64sc -autostartprgmode 1 build/test_diagnostic.prg`

### test_single_char.prg
**Purpose**: Test ROM charset display
**Expected**: Screen filled with letter 'A', blue border, black background

### test_border.prg
**Purpose**: Test border drawing with custom charset
**Expected**: Grey solid border, black center, 3 test patterns

### test_minimal.prg
**Purpose**: Minimal charset test with checkerboard fill
**Expected**: Entire screen filled with checkerboard pattern

## Current Charset
Located at: `src/data/pattern_charset.s`

Defined tiles:
- Tile 0: Blank
- Tile 1: Checkerboard ($AA/$55 pattern)
- Tile 2: Inverse checkerboard
- Tiles 3-10: Various test patterns (stripes, diagonals, etc.)
- Tile 32: Space (blank)
- Tile 160: Solid block ($FF bytes)
- All other tiles: Blank

## Next Steps

1. **Run test_diagnostic.prg** to verify basic C64 functionality
2. If diagnostic works:
   - Issue is in main program logic (draw_border or pattern_test)
   - Need to debug those specific routines
3. If diagnostic fails:
   - Basic C64 setup issue
   - May need to check VICE configuration or hardware

## Technical Notes

### VIC-II Register $D018 (VIC_ADDR)
```
Bits 7-4: Screen memory / 1024
Bits 3-1: Charset memory / 2048
Bit 0: Unused
```

Value $18 = %00011000:
- Screen: bits 7-4 = 1 → $0400
- Charset: bits 3-1 = 4 → $2000 (4 * $0800)

### Color RAM
- Location: $D800-$DBFF (fixed, not moveable)
- Offset from screen RAM: $D400
- 4 bits per location (16 colors)

### Charset Format
- 256 characters * 8 bytes each = 2048 bytes = $800
- Each byte represents one scan line of the 8×8 character
- Bit 7 = leftmost pixel, Bit 0 = rightmost pixel

## Files Modified This Session

- `src/main.s` - Disabled test message, fixed charset location
- `src/data/pattern_charset.s` - Multiple iterations to fix format
- `src/display.s` - Added gray border coloring
- `src/patterns.s` - Fixed color RAM offset bug ($D4 not $D0)
- Various test files created for diagnosis

## Known Issues Fixed

1. **Charset memory overlap** - Was at $0800, overlapping code at $080F
   - Fixed: Moved to $2000

2. **Color RAM corruption** - Writing to $D000-$D3FF instead of $D800+
   - Fixed: Changed offset from $D0 to $D4

3. **Charset data format** - `.segment "CODE"` directive caused issues
   - Fixed: Removed segment directive from charset file

## Critical Discovery

The charset system works correctly when tested in isolation. The issue is specifically with how the main program (`make run`) combines all components. Further debugging needed to identify exact failure point.
