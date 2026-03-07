# Part 3: HL Advancement Constant - FOUND

## Summary

Successfully extracted the missing HL advancement constant from disassembly: **$F8C3** (= -1853 decimal)

## Disassembly Evidence

From `full_disassembly.asm` (frame 900 trace):

```asm
$8660: POP BC               ; executed 24 times
$8661: LD (HL),C            ; executed 24 times
$8662: DEC L                ; executed 24 times
$8663: LD (HL),B            ; executed 24 times
$8664: LD DE,$F8C3          ; executed 24 times  ← THE CONSTANT!
$8667: ADD HL,DE            ; executed 24 times
$8668: EXX                  ; executed 24 times
$8669: DEC B                ; executed 24 times
$866A: JP NZ,$85A3          ; executed 24 times
```

## Context

This instruction appears at the **end of each DJNZ iteration** in the Part 3 drawing routine:

1. **18 × POP BC** writes 36 bytes to screen with complex navigation
2. **LD DE,$F8C3** loads the advancement constant
3. **ADD HL,DE** advances HL by -1853 bytes
4. **EXX** switches register sets (HL ↔ HL')
5. **DJNZ** loops back to $85A3 for next iteration

## Value Analysis

**Hex**: $F8C3
**Little-endian bytes**: C3 F8 (at addresses $8665-$8666)
**Unsigned**: 63,683
**Signed (two's complement)**: **-1,853**

The negative value means HL moves **backwards** through screen memory between iterations.

## ZX Spectrum Screen Memory Layout

- Screen base: $4000
- Screen size: 6,144 bytes (192 lines × 32 bytes)
- Y-coordinate interleaving: `y = (y7y6 << 6) | (y5y4y3 << 3) | y2y1y0`

Moving backwards by 1,853 bytes navigates through the interleaved screen structure to position HL for the next 6-row strip.

## Execution Count Analysis

**Executed 24 times** = 8 DJNZ iterations × 3 tiles per frame

Each frame draws 3 tiles:
- Square 0 (left)
- Square 1 (middle)
- Square 2 (right)

Each tile uses 8 DJNZ iterations (8 chunks of 36 bytes each).

## Implications for Source-Only Decoding

### Previously Blocked

We couldn't build the complete 288-byte write trace because we didn't know how HL advanced between DJNZ iterations.

### Now Unblocked

With the constant $F8C3, we can:

1. **Simulate HL navigation** through all 8 DJNZ iterations
2. **Record all 288 screen addresses** where bytes are written
3. **Build permutation table** mapping `pattern_byte[i]` → `screen(row, col)`
4. **Decode patterns from source** without screen extraction

## Next Steps

### Build Complete Write Trace

```python
# Starting position for top-left tile
HL = 0x4004  # x_byte=4, y=0

write_map = []
byte_idx = 0

for djnz_iteration in range(8):
    # Write 36 bytes with complex navigation
    for pop_iteration in range(18):
        # Write C register
        write_map.append((byte_idx, HL))
        byte_idx += 1
        HL = navigate_after_write_C(HL)  # INC L, INC H, or DEC L

        # Write B register
        write_map.append((byte_idx, HL))
        byte_idx += 1
        HL = navigate_after_write_B(HL)  # INC L, INC H, or DEC L

    # Advance HL for next iteration
    HL = (HL + 0xF8C3) & 0xFFFF  # Add -1853 with 16-bit wrap
```

### Generate Permutation Table

Map each pattern byte index to screen coordinates:

```json
{
  "description": "Part 3 pattern byte to screen position mapping",
  "hl_start": "0x4004",
  "hl_advancement": "0xF8C3",
  "mapping": [
    {"pattern_idx": 0, "screen_addr": "0x4004", "row": 0, "col": 32},
    {"pattern_idx": 1, "screen_addr": "0x4005", "row": 0, "col": 40},
    ...
  ]
}
```

### Implement Source-Only Decoder

```python
def decode_pattern_from_source(pattern_bytes, permutation_table):
    """Decode 288-byte pattern to 48×48 pixels using permutation table."""
    pixels = [[0]*48 for _ in range(48)]

    for entry in permutation_table:
        pattern_idx = entry['pattern_idx']
        screen_addr = entry['screen_addr']

        # Extract (row, col) from ZX screen address
        row, x_byte = zx_coords_from_addr(screen_addr)

        # Get pattern byte and unpack 8 pixels
        byte_val = pattern_bytes[pattern_idx]
        for bit in range(8):
            col = x_byte * 8 + bit
            pixels[row][col] = (byte_val >> (7 - bit)) & 1

    return pixels
```

## Technical Details

### Why Negative?

The drawing routine writes in a forward pattern within each 36-byte chunk but must reposition HL to start the next chunk. The negative offset efficiently navigates back through the interleaved screen memory.

### Why 1,853?

This specific value positions HL correctly for the next 6-row strip in the ZX Spectrum's interleaved screen memory layout. The calculation accounts for:
- Y-coordinate interleaving (3 bits scrambled)
- X-coordinate offset (tile position)
- Previous navigation (accumulated INC H, INC L, DEC L)

## Conclusion

**Blocker removed!** We now have all information needed to build a complete source-only decoder:

1. ✅ Starting HL: $4004
2. ✅ Navigation pattern: INC H, INC L, DEC L (from disassembly)
3. ✅ HL advancement: $F8C3 (= -1853)
4. ✅ DJNZ iterations: 8 (each 36 bytes)

The permutation table can now be generated by simulating the complete write trace.

---

**Status**: ✅ COMPLETE - Missing constant found!
**Date**: 2026-03-07
**Method**: Full disassembly with unknown opcode suppression
**Location**: Bank 2, addresses $8665-$8666 = bytes C3 F8
**Value**: $F8C3 (hex) = -1853 (decimal signed 16-bit)
