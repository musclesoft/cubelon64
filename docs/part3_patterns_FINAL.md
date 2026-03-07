# Part 3 Patterns - FINAL CORRECT Extraction

## Summary

Successfully extracted and decoded all 12 unique Part 3 patterns (48×48 pixels) by fixing the root cause: **the drawing routine reads from 8 different addresses per tile**, not just 1.

## Root Cause & Fix

### The Problem

Patterns didn't match screen because:
1. Drawing routine loops 8 times (DJNZ), reading from 8 different addresses
2. Each address provides 36 bytes (6 rows × 6 bytes/row)
3. We were only capturing ONE address and reading 288 bytes sequentially ❌

### The Solution

1. **Capture 36-byte chunks**: Modified capture to save 36 bytes per address
2. **Read all 8 addresses**: Parse entire pattern table at $84C0 (16 bytes)
3. **Extract from screen**: Read pixels directly from screen memory after drawing ✅

## Pattern Table Structure

**Location**: $84C0 in Bank 2
**Format**: 8 entries × 2 bytes = 16 bytes total

Example (Frame 920):
```
Entry 0: $C946 → 36 bytes (rows 0-5)
Entry 1: $C706 → 36 bytes (rows 6-11)
Entry 2: $C946 → 36 bytes (rows 12-17)
Entry 3: $C706 → 36 bytes (rows 18-23)
Entry 4: $C946 → 36 bytes (rows 24-29)
Entry 5: $C706 → 36 bytes (rows 30-35)
Entry 6: $C946 → 36 bytes (rows 36-41)
Entry 7: $C706 → 36 bytes (rows 42-47)
```

## Unique Patterns Catalog

| Frame | Pattern Addresses | Description |
|-------|-------------------|-------------|
| 900-919 | $BE06, $C046 alternating | Initial 3D cross structure (20 frames) |
| 920-921 | $C946, $C706 alternating | Morph step 1 |
| 922-923 | $C8FE, $C74E alternating | Morph step 2 |
| 924-925 | $C8B6, $C796 alternating | Morph step 3 |
| 926-927 | $C86E, $C7DE alternating | Morph step 4 |
| 928-929 | $C826 repeated | Morph step 5 (dissolving) |
| 930-931 | $C7DE, $C86E alternating | Morph step 6 |
| 932-933 | $C796, $C8B6 alternating | Morph step 7 (mostly empty) |
| 934-935 | $C74E, $C8FE alternating | Morph step 8 |
| 936-937 | $C706, $C946 alternating | Morph step 9 (emerging) |
| 938-959 | $C046, $BE06 alternating | Final 3D cross structure (22 frames) |
| 960 | $C98E, $CBCE alternating | Transition frame (1 frame) |

## Visual Characteristics

✅ **Clear symmetric cross/plus patterns**
✅ **3D geometric structures with depth**
✅ **Smooth morphing transitions**
✅ **Gradual dissolution and re-emergence**

The animation shows:
1. **Static display** (frames 900-919): Complex cross pattern
2. **Morphing** (frames 920-937): Gradual transformation through intermediate states
3. **Final display** (frames 938-959): Different cross configuration
4. **Transition** (frame 960): Brief flash before next section

## Technical Details

### Memory Organization

- **Pattern chunks**: 36 bytes each
- **Chunk layout**: 6 rows × 6 bytes/row = 36 bytes
- **Complete tile**: 8 chunks × 36 bytes = 288 bytes
- **Screen extraction**: Direct read from ZX Spectrum screen memory at $4000

### Drawing Routine Behavior

```asm
$859D: LD B,$08           ; Loop 8 times
$85A3: LD E,(HL)          ; ← DJNZ loops back here
$85A4: INC L              ; Read pattern address from table
$85A5: LD D,(HL)
$85A6: INC L              ; Move to next table entry
$85A7-$8667: ...          ; POP 36 bytes, write to screen
$8669: DJNZ $85A3         ; Loop for next chunk
```

### Execution Statistics

- Pattern table reads: 8 per tile
- POP BC operations: 144 per tile (18 × 8 = 144)
- Bytes loaded: 288 per tile (2 × 144 = 288)
- Screen writes: 2,304 per frame (48×48 pixels)

## Files Generated

### Export Data
- **part3_tiles_900-960.json** - All 61 frames with correct pattern data
  - Includes `pattern_addrs` array with all 8 addresses
  - Pixels extracted directly from screen memory
  - Size: ~1.6 MB

### Visualizations
- **part3_all_patterns_CORRECT.png** - Sprite sheet (768×576, 4×3 grid)
- **part3_tile_frameNNN_XXXX_multi.png** - Individual patterns (12 files)

## Code Implementation

### Capture Code (run_cubelon.go:776-808)
```go
// Capture 36-byte chunks (not 288 bytes)
if pc == 0x85AC {
    addr := cpu.SP - 2
    patternData := make([]byte, 36)  // Only 36 bytes!
    patternData[0] = byte(cpu.BC() & 0xFF)
    patternData[1] = byte((cpu.BC() >> 8) & 0xFF)
    for i := 2; i < 36; i++ {
        patternData[i] = mem.Read(addr + uint16(i))
    }
    part3CapturedPatterns[addr] = patternData
}
```

### Export Code (run_cubelon.go:1227-1290)
```go
// Read all 8 addresses from pattern table
patternAddrs := make([]uint16, 8)
for i := 0; i < 8; i++ {
    addr := 0x84c0 + uint16(i*2)
    lo := mem.Read(addr)
    hi := mem.Read(addr + 1)
    patternAddrs[i] = uint16(lo) | (uint16(hi) << 8)
}

// Extract pixels directly from screen memory
for y := 0; y < 48; y++ {
    for x_byte := 0; x_byte < 6; x_byte++ {
        // ZX Spectrum screen address calculation
        y7y6 := (y >> 6) & 0x03
        y2y1y0 := y & 0x07
        y5y4y3 := (y >> 3) & 0x07
        screenAddr := 0x4000 | (y7y6 << 11) | (y2y1y0 << 8) |
                     (y5y4y3 << 5) | (4 + x_byte)
        b := mem.Read(uint16(screenAddr))
        // ... decode bits to pixels
    }
}
```

## Why This Approach Works

1. **Respects the actual drawing algorithm**: Reads same 8 addresses the Z80 code reads
2. **Avoids transformation complexity**: Extracts from screen, not from pattern data
3. **Handles interleaved storage**: Pattern chunks can be scattered in memory
4. **Captures correct banking**: Uses runtime-captured chunks with proper bank paging

## Usage for C64 Port

These corrected patterns provide:
- **Exact visual reference** for recreating the morphing effect
- **Frame-by-frame sequence** showing all intermediate states
- **Pattern reuse analysis** (e.g., $C946 and $C706 alternate)
- **Optimization insights** (patterns are reused across tiles)

The C64 port can:
1. Convert these 48×48 patterns to C64 format (multicolor or hires)
2. Implement similar pattern table indirection for memory efficiency
3. Use the same morphing sequence for visual fidelity
4. Optimize by identifying which patterns are mirror/rotations

---

**Status**: ✅ COMPLETE and verified against screen output
**Date**: 2026-03-07
**Total patterns**: 12 unique pattern combinations
**Total frames**: 61 (frames 900-960)
**Method**: Screen-based extraction with full pattern table analysis
