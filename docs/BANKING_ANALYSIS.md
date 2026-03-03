# 128K Spectrum Banking Analysis

## Critical Discovery

CUBELON uses **128K ZX Spectrum** with RAM bank switching, not 48K!

## Loader Analysis

The loader at $7800 performs these operations:

1. **Disable interrupts** (DI)
2. **Set stack** to $784D
3. **Read data table** starting at $784D
4. **For each entry**:
   - Read bank number
   - Switch to that bank (OUT to port $7FFD)
   - Load data from source address
   - Copy to destination address
5. **Jump to $7D00** when bank $08 encountered (end marker)

## Memory Map After Loading

| Destination | Bank | Source  | Size Est | Contents |
|-------------|------|---------|----------|----------|
| $6000       | 0    | $1800   | ~6 KB    | TBD |
| $7900       | 0    | $1D33   | ~6 KB    | TBD |
| $9640       | 0    | $09BE   | ~11 KB   | TBD |
| $C000       | 0    | $01C8   | ~16 KB   | Main code? |
| $D006       | 0    | $0746   | ~10 KB   | TBD |
| $F988       | 0    | $007D   | ~1.5 KB  | TBD |
| $FA06       | 0    | $05FA   | ~1.5 KB  | TBD |
| $C000       | 1    | $045C   | ~16 KB   | Code overlay? |
| $F815       | 1    | $07D5   | ~2 KB    | TBD |
| $C000       | 6    | $11F2   | ~16 KB   | Code overlay? |
| $D200       | 6    | $2DE6   | ~11 KB   | TBD |
| $DB00       | 7    | $24E5   | ~9 KB    | TBD |

**Note**: $C000 is loaded multiple times from different banks, suggesting code overlay system.

## 128K Spectrum Banking

### Memory Layout
- **Banks 0-7**: 16KB RAM banks switchable at $C000-$FFFF
- **Port $7FFD**: Bank control port
  - Bits 0-2: RAM bank selection (0-7)
  - Bit 4: ROM select
  - Bit 5: Disable paging lock

### Bank Switching
```z80
LD A, bank_number      ; 0-7
OR $10                 ; Keep bit 4 set
LD BC, $7FFD           ; Port address
OUT (C), A             ; Switch bank
```

## TAP File Block Mapping

The TAP file blocks correspond to data in different banks:

| TAP Block | Likely Bank | Contains |
|-----------|-------------|----------|
| data_01   | ?           | Screen data (title?) |
| data_02   | ?           | Graphics/screen |
| data_03   | ?           | Tiles (bank 0?) |
| data_04   | ?           | Lookup tables |
| data_05   | ?           | Tiles (bank 1?) |
| data_06   | ?           | Small data |
| data_07   | ?           | Tiles (bank 6?) |
| data_08   | ?           | Tiles (bank 7?) |
| data_09   | ?           | Address tables |
| data_10   | 0           | Main code at $C000 |
| data_11   | 1 or 6      | Code overlay |
| data_12   | 6 or 7      | Code overlay |

## C64 Port Implications

### Challenges
1. **Memory reorganization**: C64 has flat 64K, no banking
2. **Code overlays**: Need to identify which code runs when
3. **Data extraction**: Must determine actual loaded addresses

### Solutions
1. **Trace execution**: Follow from $7D00 to understand code flow
2. **Map memory usage**: Identify what's active when
3. **Flatten structure**: Reorganize for C64's memory model
4. **Possible compression**: C64 has 64K vs effective 80K on 128K Spectrum

## Next Steps

1. ✅ Understand loader mechanism
2. ⏳ Disassemble code at $7D00 (entry point)
3. ⏳ Trace pattern generation routine
4. ⏳ Identify which overlays contain pattern code
5. ⏳ Extract and reorganize data for C64

## Tools Needed

- Z80 disassembler (created)
- Memory map tracer
- Bank/overlay analyzer
- Code flow analyzer

## Progress

**Phase**: Reverse Engineering - Bank Analysis Complete
**Status**: Ready to trace execution from entry point
**Blocker**: None - good progress!
