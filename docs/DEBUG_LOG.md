# CUBELON 64 - Debug Log

## Session: Pattern Display Implementation

### Issue 1: Color RAM Crash
**Symptom:** Program crashes when writing colors
**Cause:** Incorrect color RAM offset calculation in `set_char_at_ptr`
- Had: `adc #$D0` (offset $D000)
- Wrote to: $D000-$D3FF (VIC-II and SID registers)
- Should be: `adc #$D4` (offset $D400)
- Correct range: $D800-$DBFF (color RAM)

**Fix:** Changed to `adc #$D4` in [src/patterns.s:152](../src/patterns.s#L152)

### Issue 2: Charset Copy Crash (CRITICAL)
**Symptom:** Program crashes when copying custom charset
**Cause:** Memory overlap - overwriting own code
- Charset destination: $0800-$0FFF (2KB)
- Program location: $080F-$11E7 (per build/cubelon.map)
- Overlap: $080F-$0FFF (nearly 2KB of code overwritten!)

**Analysis:**
```
Memory Map:
$0801-$0802  LOADADDR (load address word)
$0803-$080E  EXEHDR (BASIC stub)
$080F-$08B0  STARTUP (main.s code)
$08B1-$11E7  CODE (all modules including pattern_charset)
$A000-$A000  DATA (anim_frame variable)

Charset at $0800 would overwrite:
$0800-$0FFF: Entire BASIC stub, STARTUP, and most of CODE segment!
```

**Fix:** Moved charset to $2000-$27FF
- Safe location (code ends at $11E7, DATA starts at $A000)
- Updated VIC_ADDR: `ora #$08` for charset at offset 4 ($2000 / $0800 = 4)
- Changed in [src/main.s:117-148](../src/main.s#L117-L148)

### Issue 3: White Screen
**Symptom:** Entire screen displayed white after border fix
**Cause:** `draw_border` filled screen with inverse spaces but inherited white color from `clear_screen`

**Fix:** Added gray color loop in `draw_border` ([src/display.s:76-85](../src/display.s#L76-L85))

### Issue 4: Pointer Arithmetic
**Symptom:** Pattern tiles displayed incorrectly (wrong positions)
**Cause:** Incorrect row offset calculation
- Need to account for 3 increments per row (3 tiles wide)
- Screen width is 40, so offset is 40 - 3 = 37

**Fix:** Changed row advance from `adc #40` to `adc #37` in patterns.s

## Current Status: ✅ WORKING

**Achieved:**
- Custom charset loaded at $2000
- Pattern tiles displaying from ZX Spectrum data
- 3×3 pattern grid (24×24 pixels)
- Animation frame toggle working
- Gray border around display area
- "CUBELON 64 - INIT OK" message at center

**Test Results:**
- Program runs without crashing
- VICE emulator stable
- Pattern visible on screen

## Next Steps

1. **Verify Pattern Accuracy**
   - Compare displayed pattern to ZX Spectrum original
   - Check tile animations (frame 0 vs frame 1)
   - Verify colors match intended palette

2. **Implement Full 8×8 Grid**
   - Currently: Single 3×3 pattern
   - Target: 8×8 grid of patterns (64 patterns total)
   - Each pattern is 3×3 tiles (24×24 pixels)

3. **Add Proper Tile Combination**
   - Currently: Using placeholder tiles for combined (T2)
   - Implement: XOR combination of T0 and T1
   - Add: RRCA rotation transformation

4. **Match ZX Timing**
   - Study original frame timing
   - Synchronize animation speed
   - Match visual effects timing

## Technical Notes

### VIC-II Charset Configuration
- VIC bank 0: $0000-$3FFF (default)
- Charset at $2000 within bank 0
- Charset bits in VIC_ADDR ($D018):
  - Bits 1-3: Charset offset / $0800
  - Value $08 = charset at $2000 ($2000 / $0800 = 4, binary 100)

### Memory Safety
- Always check memory map before assigning charset location
- Safe areas: Between code end and DATA start
- Check `build/cubelon.map` after every build
- Common safe locations: $2000, $3000, $3800

### Color RAM
- Color RAM: $D800-$DBFF (fixed location, not moveable)
- Offset from screen RAM: $D400
- Always calculate: screen_ptr + $D400 offset
- Never use: screen_ptr + $D000 (VIC-II registers!)
