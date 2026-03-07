# Part 3: Source-Only Decoding Requirements

## ✅ UPDATE (2026-03-07): BLOCKER RESOLVED!

**All navigation constants have been found!** See:
- [part3_HL_advancement_FOUND.md](part3_HL_advancement_FOUND.md) - End-of-iteration constant: $F8C3
- [part3_complete_navigation_constants.md](part3_complete_navigation_constants.md) - All three constants: $F920, $F920, $F8C3

Source-only decoding is now **fully possible** with complete information.

---

## Original Situation (Historical)

**What we have**: Screen extraction (reads pixels from ZX Spectrum screen memory after drawing)
**Why**: It works perfectly and gives 100% accurate results
**The problem**: It relies on "destination" data (screen), not "source" data (pattern bytes)

## Why Source-Only Decoding Is Challenging

### The Core Issue

The drawing routine writes 288 bytes to screen in a complex order determined by:
1. **Pattern table indirection** (8 addresses)
2. **ZX Spectrum screen interleaving** (Y coordinates scrambled)
3. **Navigation pattern** (INC H, INC L, DEC L creating zigzag)
4. **HL advancement between DJNZ iterations** (unknown constant)

### What We Know

From disassembly analysis:
- Starting HL: $4004 (top-left tile at x=32, y=0)
- Write pattern: 18 × (POP BC, write C, navigate, write B, navigate)
- Between iterations: `LD DE,nnnn; ADD HL,DE` at $8664-$8667
- Pattern repeats 8 times (DJNZ loop)

### The Missing Piece

**Blocker**: Bytes at $8665-$8666 (the `LD DE,nnnn` constant)

These two bytes determine how HL advances between DJNZ iterations, which is critical for mapping pattern bytes to screen positions.

**Current state**:
- Disassembler shows `DB $11` instead of decoded instruction
- Bank 2 memory dumps not readily available
- Would need targeted memory read during emulation

## What Would Be Required

### Option 1: Extract LD DE Constant

**Steps**:
1. Read bytes at Bank 2 offset $0665-$0666 (= $8665-$8666 - $8000)
2. Interpret as little-endian 16-bit value
3. Complete the write trace for all 288 bytes
4. Build permutation table mapping pattern[i] → screen(row, col)

**Estimated time**: 2-4 hours

### Option 2: Runtime Write Trace

**Steps**:
1. Add emulator hook at every `LD (HL),r` in drawing routine
2. Record (pattern_byte_index, screen_address) for all 288 writes
3. Convert screen addresses to (row, col) using ZX formula
4. Build permutation table from recorded sequence

**Estimated time**: 1-2 hours

### Option 3: Manual Calculation

**Steps**:
1. Manually trace through disassembly with HL register simulation
2. Calculate each of 288 screen addresses
3. Account for ZX interleaving: `y = (y7y6 << 6) | (y5y4y3 << 3) | y2y1y0`
4. Build permutation table

**Estimated time**: 4-8 hours (error-prone)

## Current Implementation Justification

### Why Screen Extraction Was Used

**Pragmatic decision based on**:
1. **Correctness**: 100% accurate results verified against actual frames
2. **Simplicity**: ~10 lines of code vs. hours of tracing
3. **Reliability**: No risk of navigation calculation errors
4. **Completeness**: Already implemented and working

### Trade-offs

**Advantages**:
- ✅ Perfect accuracy
- ✅ Simple implementation
- ✅ Easy to verify
- ✅ No complex navigation logic needed

**Disadvantages**:
- ❌ Requires emulation to decode
- ❌ Doesn't reveal pattern data structure
- ❌ Can't analyze patterns offline
- ❌ Less educational about the algorithm

## Path Forward

### For C64 Port (Practical Use)

**Recommendation**: Use screen-extracted pixels as-is
- They're correct and ready to convert to C64 format
- No need to understand byte-level transformation
- Focus effort on C64-specific challenges

### For Complete Understanding (Educational)

**Recommendation**: Implement Option 2 (Runtime Write Trace)
- Add one-time debug hooks to record write sequence
- Generate permutation table
- Validate against screen extraction
- Document transformation completely

### For Offline Decoding (Future Tool)

**Requirement**: Complete Option 1 or 2 first
- Extract/generate permutation table
- Build standalone decoder using table
- Can then decode any Part 3 pattern without emulation

## Technical Specifications

### Permutation Table Format

```json
{
  "description": "Part 3 pattern byte to screen position mapping",
  "tile_size": {"width": 48, "height": 48},
  "bytes_per_tile": 288,
  "mapping": [
    {"pattern_idx": 0, "screen_row": 0, "screen_col": 32, "bit_offset": 0},
    {"pattern_idx": 1, "screen_row": 0, "screen_col": 40, "bit_offset": 0},
    ...
  ]
}
```

### Usage Example

```python
def decode_pattern_from_source(pattern_bytes, permutation_table):
    pixels = [[0]*48 for _ in range(48)]

    for entry in permutation_table:
        pattern_idx = entry['pattern_idx']
        row = entry['screen_row']
        col_byte = entry['screen_col'] // 8

        byte_val = pattern_bytes[pattern_idx]
        for bit in range(8):
            col = col_byte * 8 + bit
            pixels[row][col] = (byte_val >> (7 - bit)) & 1

    return pixels
```

## Conclusion

**Source-only decoding IS possible** but requires additional work to extract the HL advancement constant or trace write sequences.

**Current screen extraction is valid** as a pragmatic solution that delivers correct results efficiently.

**For complete understanding**, implementing Option 2 (runtime write trace) is recommended as it:
- Reveals the complete transformation
- Generates reusable permutation table
- Enables offline decoding
- Takes reasonable time (1-2 hours)

**For the C64 port**, the current approach is sufficient - the decoded pixels are correct and ready to use.

---

**Status**: ✅ UNBLOCKED - All navigation constants found! Source-only decoding now fully possible.
**Original status**: Screen extraction working perfectly; source-only decoder blocked on HL advancement constant
**Resolution**: Full disassembly with unknown opcode suppression revealed all three LD DE constants
**Next step**: Build permutation table using complete navigation constants (see part3_complete_navigation_constants.md)
