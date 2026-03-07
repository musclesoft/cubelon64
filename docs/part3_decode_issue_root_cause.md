# Part 3 Decode Issue - Root Cause Found

## Problem

The decoded 48×48 patterns don't match what's displayed on screen.

**Screen shows**: Clear symmetric cross/plus patterns
**Our decode shows**: Garbled noise

## Root Cause

**The drawing routine reads from 8 DIFFERENT addresses, not 1!**

From the disassembly execution trace:
- `$85A3` (LD E,(HL) - read pattern addr from table): executed **20,208 times**
- `$85AB` (POP BC - first pattern byte): executed **20,208 times**

Calculation: 20,208 = 2,526 calls × 8 DJNZ iterations

This proves the DJNZ loop **LOOPS BACK TO $85A3**, re-reading from the pattern table each iteration.

## Drawing Routine Structure

```asm
$859D: LD ($866E),SP      ; Save SP
$85A1: LD B,$08           ; Loop 8 times
; ↓ DJNZ loops back here!
$85A3: LD E,(HL)          ; Read pattern address LOW from table
$85A4: INC L              ; Next byte
$85A5: LD D,(HL)          ; Read pattern address HIGH
$85A6: INC L              ; Next entry (moves to next address)
$85A7: EX DE,HL           ; HL = pattern address
$85A8: LD SP,HL           ; SP = pattern address
$85A9: EX DE,HL           ; HL = pattern table again
$85AA: EXX                ; Switch to alt registers
$85AB: POP BC             ; ← Start of 18 POP BC (36 bytes)
...
(18 POP BC total = 36 bytes)
...
$8668: EXX                ; Back to main registers
$8669: DJNZ $85A3         ; Loop back! (B decrements, loop 8 times)
$866D: LD SP,($866E)      ; Restore SP
$8670: RET
```

## Pattern Storage Structure

**Pattern table at $84C0** (in Bank 2):
- 8 consecutive entries (16 bytes total)
- Each entry: 2 bytes (little-endian address)
- Each address points to 36 bytes of pattern data

**Pattern chunks**:
- Each chunk: 36 bytes = 6 rows × 6 bytes/row
- 8 chunks: 8 × 6 rows = 48 rows total
- Total per tile: 8 × 36 = 288 bytes
- BUT: Data is **non-contiguous**, scattered across 8 different addresses

## Example: Frame 920

Pattern table at $84C0 should contain 8 addresses like:
```
Offset  Address   Data
$84C0:  ??  ??    → 36 bytes (rows 0-5)
$84C2:  ??  ??    → 36 bytes (rows 6-11)
$84C4:  ??  ??    → 36 bytes (rows 12-17)
$84C6:  ??  ??    → 36 bytes (rows 18-23)
$84C8:  ??  ??    → 36 bytes (rows 24-29)
$84CA:  ??  ??    → 36 bytes (rows 30-35)
$84CC:  ??  ??    → 36 bytes (rows 36-41)
$84CE:  ??  ??    → 36 bytes (rows 42-47)
```

## What We Did Wrong

**Current export code**:
- Reads pattern table at $84C0
- Gets FIRST address only (2 bytes)
- Reads 288 bytes SEQUENTIALLY from that one address
- ❌ WRONG: Assumes pattern is contiguous

**What we should do**:
- Read pattern table at $84C0
- Get ALL 8 addresses (16 bytes)
- Read 36 bytes from EACH address
- Assemble them as: chunk0[36] + chunk1[36] + ... + chunk7[36]
- ✅ CORRECT: Respects the scattered storage

## Why Patterns Are Scattered

The $48 (72-byte) spacing between pattern addresses makes sense now:
- $C946, $C8FE, $C8B6, $C86E, $C826, $C7DE, $C796, $C74E
- Spacing: -$48 = -72 bytes

72 bytes = 2 × 36 bytes, suggesting these are interleaved chunks used by different tiles or different strips within tiles.

The scattered storage likely allows:
1. **Pattern reuse**: Different tiles share some chunks
2. **Efficient morphing**: Change which chunks are used without copying data
3. **Memory optimization**: Pack chunks tightly, reference them flexibly

## Fix Required

### Emulator Changes (run_cubelon.go)

Around line 1238-1310, modify Part 3 export:

```go
// OLD: Read single address and 288 bytes
patternAddr := uint16(lo) | (uint16(hi) << 8)
tileBytes := make([]byte, 288)
for i := 0; i < 288; i++ {
    tileBytes[i] = mem.Read(patternAddr + uint16(i))
}

// NEW: Read 8 addresses and assemble 8 chunks
patternAddrs := make([]uint16, 8)
for i := 0; i < 8; i++ {
    addr := patternTable + uint16(i*2)
    lo := mem.Read(addr)
    hi := mem.Read(addr + 1)
    patternAddrs[i] = uint16(lo) | (uint16(hi) << 8)
}

tileBytes := make([]byte, 288)
for i := 0; i < 8; i++ {
    // Use captured pattern data (respects banking)
    if capturedData, exists := part3CapturedPatterns[patternAddrs[i]]; exists {
        copy(tileBytes[i*36:(i+1)*36], capturedData[0:36])
    } else {
        // Fallback: read from memory
        for j := 0; j < 36; j++ {
            tileBytes[i*36+j] = mem.Read(patternAddrs[i] + uint16(j))
        }
    }
}
```

### Capture Changes

Modify pattern capture (around line 777-793) to capture 36-byte chunks:

```go
if cpu.PC == 0x85AB {  // Right after LD SP,HL
    // Get pattern address from DE (swapped back at $85A9)
    addr := (uint16(cpu.D) << 8) | uint16(cpu.E)

    if _, exists := part3CapturedPatterns[addr]; !exists {
        patternData := make([]byte, 36)  // Only 36 bytes per chunk!
        for i := 0; i < 36; i++ {
            patternData[i] = mem.Read(addr + uint16(i))
        }
        part3CapturedPatterns[addr] = patternData
    }
}
```

## Expected Outcome

After fix:
- ✅ Decoded patterns will match screen tiles exactly
- ✅ Clear symmetric cross/plus patterns will be visible
- ✅ Morphing sequence will show correct transformations
- ✅ All 12 unique patterns will be properly reconstructed

---

**Status**: Root cause identified, fix specifications documented
**Next step**: Implement emulator changes and re-export patterns
**Estimated time**: 30-60 minutes to fix and verify
