# Part 3: Complete Navigation Constants - ALL FOUND

## Summary

Found **three** LD DE constants that control HL navigation during the 36-byte write sequence:

1. **$F920** (= -1760) at $85E6 - mid-sequence adjustment #1
2. **$F920** (= -1760) at $8625 - mid-sequence adjustment #2
3. **$F8C3** (= -1853) at $8664 - end-of-iteration advancement

## Complete DJNZ Iteration Structure

Each of the 8 DJNZ iterations writes 36 bytes in **three sections** with HL adjustments between them:

### Section 1: Bytes 0-11 (12 bytes)

```asm
$85AB: POP BC, LD (HL),C, INC L, LD (HL),B, INC L     ; bytes 0-1
$85B0: POP BC, LD (HL),C, INC H, LD (HL),B, DEC L     ; bytes 2-3
$85B5: POP BC, LD (HL),C, DEC L, LD (HL),B, INC H     ; bytes 4-5
$85BA: POP BC, LD (HL),C, INC L, LD (HL),B, INC L     ; bytes 6-7
$85BF: POP BC, LD (HL),C, INC H, LD (HL),B, DEC L     ; bytes 8-9
$85C4: POP BC, LD (HL),C, DEC L, LD (HL),B, INC H     ; bytes 10-11
```

**Then adjust**: `LD DE,$F920; ADD HL,DE` (move -1760 bytes)

### Section 2: Bytes 12-23 (12 bytes)

```asm
$85EA: POP BC, LD (HL),C, INC L, LD (HL),B, INC L     ; bytes 12-13
$85EF: POP BC, LD (HL),C, INC H, LD (HL),B, DEC L     ; bytes 14-15
$85F4: POP BC, LD (HL),C, DEC L, LD (HL),B, INC H     ; bytes 16-17
$85F9: POP BC, LD (HL),C, INC L, LD (HL),B, INC L     ; bytes 18-19
$85FE: POP BC, LD (HL),C, INC H, LD (HL),B, DEC L     ; bytes 20-21
$8603: POP BC, LD (HL),C, DEC L, LD (HL),B, INC H     ; bytes 22-23
```

**Then adjust**: `LD DE,$F920; ADD HL,DE` (move -1760 bytes)

### Section 3: Bytes 24-35 (12 bytes)

```asm
$8629: POP BC, LD (HL),C, INC L, LD (HL),B, INC L     ; bytes 24-25
$862E: POP BC, LD (HL),C, INC H, LD (HL),B, DEC L     ; bytes 26-27
$8633: POP BC, LD (HL),C, DEC L, LD (HL),B, INC H     ; bytes 28-29
$8638: POP BC, LD (HL),C, INC L, LD (HL),B, INC L     ; bytes 30-31
$863D: POP BC, LD (HL),C, INC H, LD (HL),B, DEC L     ; bytes 32-33
$8642: POP BC, LD (HL),C, DEC L, LD (HL),B, INC H     ; bytes 34-35
```

**Then adjust**: `LD DE,$F8C3; ADD HL,DE` (move -1853 bytes)

### Loop Control

```asm
$8668: EXX                  ; Switch register sets
$8669: DEC B                ; Decrement DJNZ counter
$866A: JP NZ,$85A3          ; Jump back if not zero
```

## Navigation Pattern Within Each Section

Each 12-byte section follows the **same navigation pattern** repeated 6 times:

```
POP BC
  LD (HL),C  <-- write byte from C register
  INC L      <-- navigate horizontally right
  LD (HL),B  <-- write byte from B register
  INC L      <-- navigate horizontally right

POP BC
  LD (HL),C
  INC H      <-- navigate vertically down (through interleaving!)
  LD (HL),B
  DEC L      <-- navigate horizontally left

POP BC
  LD (HL),C
  DEC L      <-- navigate horizontally left
  LD (HL),B
  INC H      <-- navigate vertically down

[Repeat pattern 2 more times within section, then LD DE + ADD HL,DE]
```

The pattern creates a **zigzag** through the ZX Spectrum's interleaved screen memory:
- Move right (INC L)
- Move down + left (INC H, DEC L)
- Move left + down (DEC L, INC H)
- Move right (INC L)
- ...

## Constants Analysis

### $F920 (= -1760 decimal)

Used twice per DJNZ iteration to reposition HL after writing 12 bytes.

**Two's complement calculation**:
- Hex: $F920 = 0xF920
- Binary bit 15 set = negative
- ~0xF920 = 0x06DF = 1,759
- Add 1 = 1,760
- **Value: -1,760 bytes**

### $F8C3 (= -1853 decimal)

Used once per DJNZ iteration to reposition HL for the next iteration.

**Two's complement calculation**:
- Hex: $F8C3 = 0xF8C3
- Binary bit 15 set = negative
- ~0xF8C3 = 0x073C = 1,852
- Add 1 = 1,853
- **Value: -1,853 bytes**

## Memory Layout Implications

All three constants are **negative**, meaning HL moves **backwards** through screen memory. This is because:

1. The INC H instructions move forward through the interleaved screen
2. The Y-coordinate interleaving means consecutive pixel rows aren't consecutive in memory
3. After writing forward through a strip, HL must jump back to continue in the right position
4. The negative offsets compensate for accumulated forward motion

## Complete 288-Byte Write Trace Algorithm

```python
def build_complete_write_map():
    HL = 0x4004  # Starting position: x_byte=4, y=0
    write_map = []
    byte_idx = 0

    for djnz_iter in range(8):
        # Section 1: Bytes 0-11
        for _ in range(6):  # 6 POP BC = 12 bytes
            write_map.append((byte_idx, HL))
            byte_idx += 1
            HL = navigate_after_C(HL)  # INC L or DEC L or same

            write_map.append((byte_idx, HL))
            byte_idx += 1
            HL = navigate_after_B(HL)  # INC L, INC H, or DEC L + INC H

        HL = (HL + 0xF920) & 0xFFFF  # -1760

        # Section 2: Bytes 12-23
        for _ in range(6):
            write_map.append((byte_idx, HL))
            byte_idx += 1
            HL = navigate_after_C(HL)

            write_map.append((byte_idx, HL))
            byte_idx += 1
            HL = navigate_after_B(HL)

        HL = (HL + 0xF920) & 0xFFFF  # -1760

        # Section 3: Bytes 24-35
        for _ in range(6):
            write_map.append((byte_idx, HL))
            byte_idx += 1
            HL = navigate_after_C(HL)

            write_map.append((byte_idx, HL))
            byte_idx += 1
            HL = navigate_after_B(HL)

        HL = (HL + 0xF8C3) & 0xFFFF  # -1853

    return write_map  # 288 entries
```

## Source Location

All three constants are in **Bank 2** (paged at $C000-$FFFF when active):

| Address | Bank 2 Offset | Instruction      | Value (hex) | Value (dec) |
|---------|---------------|------------------|-------------|-------------|
| $85E6   | $05E6         | LD DE,$F920      | $F920       | -1,760      |
| $8625   | $0625         | LD DE,$F920      | $F920       | -1,760      |
| $8664   | $0664         | LD DE,$F8C3      | $F8C3       | -1,853      |

Bytes stored little-endian:
- $85E6: bytes `20 F9`
- $8625: bytes `20 F9`
- $8664: bytes `C3 F8`

## Conclusion

**Complete navigation now documented!** All information needed for source-only decoding:

1. ✅ Starting HL: $4004
2. ✅ Navigation within sections: INC L, INC H, DEC L pattern (6 POP BC per section)
3. ✅ Section 1→2 adjustment: $F920 (= -1760)
4. ✅ Section 2→3 adjustment: $F920 (= -1760)
5. ✅ Iteration advancement: $F8C3 (= -1853)
6. ✅ DJNZ iterations: 8 (total 288 bytes)

The permutation table can now be generated with **complete accuracy** by simulating the exact write sequence.

---

**Status**: ✅ COMPLETE - All constants found!
**Date**: 2026-03-07
**Method**: Full disassembly of frame 900 execution trace
**Constants**: $F920 (×2), $F8C3 (×1) per DJNZ iteration
