# CUBELON Pattern Extraction: Screen Extraction and JSON Export Removed

## Summary

**Screen extraction and JSON export code have been removed** from the emulator and visualization tools. This document explains why and what it means for pattern analysis.

## What Changed (2025-03-07)

1. **Screen extraction removed** - No longer reads from screen memory at $4000
2. **JSON export removed** - No longer generates `animation_frames_*.json` or `part3_tiles_*.json`
3. **Pattern capture removed** - No longer hooks POP BC instructions to capture pattern chunks
4. **Visualization deprecated** - Scripts that depend on JSON exports marked as deprecated
5. **Frame diff detection added** - Only saves frames that differ from previous frame

Pattern analysis now focuses on direct memory inspection and disassembly-based understanding.

## What Was Removed

### From Emulator (`run_cubelon.go`)

1. **`PatternFrame` struct** - Entirely removed
2. **`Pixels` field** - Removed from struct (screen-extracted pixel arrays)
3. **JSON export flags** - `--export-patterns` and `--export-part3-tiles` removed
4. **Pattern capture code** - POP BC hooking at $85AC removed
5. **Export arrays** - `exportedPatterns` and `exportedPart3Tiles` removed
6. **JSON marshaling** - No more `json.MarshalIndent()` or file writing
7. **Screen extraction** - ZX Spectrum screen address calculations reading from $4000 removed

**Result**: No JSON files are generated. Pattern analysis must use other approaches.

### From Visualization Scripts

Marked as **DEPRECATED**:
- `visualize_all_complete.py`
- `analyze_all_parts.py`
- `visualize_part2_patterns.py`

These scripts relied on the `pixels` field which no longer exists in JSON exports.

## Why Screen Extraction Was Removed

### The Problem with Screen Extraction

**Screen extraction does not help understand the algorithm.** It provides a pragmatic shortcut to visualize patterns but teaches nothing about:

1. **How the code works** - What instructions transform source data to screen
2. **The data structures** - Pattern tables, chunk indirection, navigation constants
3. **The algorithm** - Write sequences, permutation tables, memory optimization strategies
4. **Portability** - Cannot port to C64 without understanding the transformation

### The User's Requirement

> "I would have preferred an extraction method that 100% relies on source data instead on 'destination' data"

> "never extract from screen. this prevents us from understanding the underlying code and data"

Screen extraction reads **destination data** (pixels in screen memory at $4000). This is opaque and bypasses the algorithm we're trying to understand.

## What We Have Instead

### Complete Source Data

All pattern data is captured from **source memory locations**:

#### Part 2 (24×24 patterns)
- **72 bytes per pattern** read from addresses like $A786, $AD26
- Direct bitmap format
- Total: 33 unique patterns across 634 frames

#### Parts 3-5 (48×48 patterns)
- **576 bytes per pattern** (8 chunks × 72 bytes)
- Pattern table at **$84C0** with 8 addresses pointing to chunks
- Chunk addresses like $BE06, $C046, $C706, etc.
- Total: 394 unique patterns across 3,198 frames

### Navigation Constants

All constants needed for source-based decoding:
- **$F920** (-1,760): Section 1→2 adjustment
- **$F920** (-1,760): Section 2→3 adjustment
- **$F8C3** (-1,853): End-of-iteration advancement

### Write Sequences

Complete permutation table for one drawing routine ($859D):
- 72 writes per DJNZ iteration
- 8 iterations = 576 bytes consumed
- Maps source byte positions to screen addresses
- Documented in `part3_write_sequence.json`

## Current Limitations

### Pattern Visualization

The existing sprite sheets (`.png` files) were generated using screen extraction:
- `part2_complete_all_patterns.png`
- `part3_complete_all_patterns.png`
- `part4_complete_all_patterns.png`
- `part5_complete_all_patterns.png`

These images are **correct visually** but were produced using the **wrong methodology**. They show what the patterns look like, but don't teach us how the code generates them.

### To Properly Visualize Patterns

Requires **source-based decoder** using:
1. Complete write sequences for all 4 drawing routines
2. Navigation constants ($F920, $F920, $F8C3)
3. ZX Spectrum screen interleaving formula
4. 576-byte source data per pattern

**Estimated effort**: 4-6 hours to build complete multi-routine permutation table.

## What You CAN Do

### Analysis Approaches

✅ **Direct memory inspection** - Read pattern table at $84C0, pattern buffers at $BE06, $C046, etc.
✅ **Disassembly analysis** - Study complete disassembly in `full_disassembly.asm`
✅ **Trace execution** - Use `--trace-all-code` flag to trace specific frame ranges
✅ **Study navigation constants** - $F920, $F920, $F8C3 documented in write sequences
✅ **Understand algorithm** - All drawing routines and data structures documented
✅ **Extract from TAP** - Analyze original TAP file blocks directly

### What Requires Work

⏳ **Source-based decoder** - Build complete multi-routine permutation table (4-6 hours)
⏳ **Chunk reuse analysis** - Read pattern table and analyze address repetition patterns
⏳ **C64 port** - Requires understanding algorithm from disassembly, not from JSON
⏳ **Pattern visualization** - Requires source-based decoder, not screen extraction

## Files Status

### Still Valid (Disassembly and Analysis)

- ✅ `full_disassembly.asm` - Complete disassembly with navigation constants
- ✅ `part3_write_sequence.json` - Write permutation table (one routine)
- ✅ All documentation (`.md` files)
- ✅ Emulator frame capture (PNG screenshots)

### No Longer Generated (JSON Exports Removed)

- ❌ `animation_frames_168-801.json` - Part 2 JSON export (removed)
- ❌ `part3_tiles_802-1404.json` - Part 3 JSON export (removed)
- ❌ `part3_tiles_1405-2304.json` - Part 4 JSON export (removed)
- ❌ `part3_tiles_2305-3999.json` - Part 5 JSON export (removed)

### Deprecated (Screen-Based Visualizations)

- ⚠️ `part2_complete_all_patterns.png` - Generated from screen extraction (before removal)
- ⚠️ `part3_complete_all_patterns.png` - Generated from screen extraction (before removal)
- ⚠️ `part4_complete_all_patterns.png` - Generated from screen extraction (before removal)
- ⚠️ `part5_complete_all_patterns.png` - Generated from screen extraction (before removal)
- ⚠️ `visualize_all_complete.py` - Depends on pixels field (deprecated)
- ⚠️ `analyze_all_parts.py` - Depends on pixels field (deprecated)
- ⚠️ `visualize_chunk_reuse.py` - Depends on JSON exports (deprecated)

## Recommended Approaches for Pattern Analysis

### Approach 1: Direct Memory Reading (Simplest)

For each frame, read pattern data directly from memory:

```go
// Part 2 (24×24 patterns)
patternTable := uint16(0x84c0)
squareIdx := 0
addr := patternTable + uint16(squareIdx*2)
lo := mem.Read(addr)
hi := mem.Read(addr + 1)
patternAddrTable := uint16(lo) | (uint16(hi) << 8)
patternAddr := patternAddrTable - 72  // Table points to end, subtract 72

// Read 72 bytes
for i := 0; i < 72; i++ {
    patternData[i] = mem.Read(patternAddr + uint16(i))
}

// Part 3/4/5 (48×48 patterns)
for i := 0; i < 8; i++ {
    addr := patternTable + uint16(i*2)
    lo := mem.Read(addr)
    hi := mem.Read(addr + 1)
    chunkAddr := uint16(lo) | (uint16(hi) << 8)

    // Read 72 bytes per chunk
    for j := 0; j < 72; j++ {
        chunkData[j] = mem.Read(chunkAddr + uint16(j))
    }
}
```

### Approach 2: Disassembly-Based Understanding (Most Educational)

1. Read `full_disassembly.asm` to understand drawing routines
2. Trace write sequences using navigation constants
3. Build permutation table mapping source to destination
4. Implement decoder based on understanding

### Approach 3: TAP File Analysis (External)

Extract pattern data directly from TAP file blocks without running emulator.

## Conclusion

This project now **strictly adheres to source-based analysis**:
- ✅ Screen extraction **removed**
- ✅ JSON export **removed**
- ✅ Pattern capture hooks **removed**

Analysis must use:
- Direct memory reading from pattern tables and buffers
- Disassembly study and algorithm understanding
- TAP file analysis

This ensures the focus remains on understanding code and data structures, not taking shortcuts through destination memory.

---

**Date**: 2025-03-07
**Changes**:
- Removed all screen extraction code
- Removed all JSON export functionality
- Removed pattern capture hooks
**Reason**: Screen extraction and automated exports do not help understand algorithm logic
