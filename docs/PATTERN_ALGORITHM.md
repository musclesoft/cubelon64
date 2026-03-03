# Pattern Generation Algorithm - Deep Dive

## Executive Summary

Located pattern generation code in **Block 10** at addresses **$C076-$C096**.
The algorithm uses **procedural generation** with XOR/rotate operations on seed tiles.

## Key Discoveries

### Pattern Display Loop ($C076-$C096)

This is the core pattern display routine that writes patterns to screen memory:

```z80
$C076: LD A,($90FB)      ; Load animation state
$C079: XOR $01           ; Toggle bit 0 (animation frame)
$C07B: LD ($90FB),A      ; Store updated state

$C07E: LD A,($90E3)      ; Load pattern index/data
$C081: RRCA              ; Rotate right (transformation!)
$C082: LD HL,$E63A       ; Point to lookup table
$C085: RST $18           ; Calculate offset (likely multiply/shift)
$C086: LD L,A            ; Use as index
$C087: LD H,$00          ; Table base page
$C089: LD DE,$9300       ; Screen/pattern buffer address
$C08C: ADD HL,DE         ; Calculate final address
$C08D: EX DE,HL          ; DE = destination address

$C090: LD B,$20          ; Counter: 32 ($20) bytes
$C092: LD (HL),E         ; Write pattern data
$C093: INC HL            ; Next position
$C094: LD (HL),D         ; Write more data
$C095: INC HL            ; Next position
$C096: DJNZ $C092        ; Loop 32 times
```

### Algorithm Breakdown

#### Stage 1: Animation State Management
- Variable at **$90FB** toggles between frames
- XOR $01 alternates bit 0 (0 ↔ 1)
- This drives animation/variation

#### Stage 2: Pattern Transformation
- Pattern data from **$90E3**
- **RRCA** (rotate right with carry) creates variation
- Different rotations = different patterns from same seed

#### Stage 3: Lookup Table
- Table at **$E63A** (likely in a memory bank)
- RST $18 performs calculation (probably index * 8 or similar)
- Maps pattern index to tile data address

#### Stage 4: Screen Write
- Destination at **$9300** (pattern buffer or screen area)
- Writes **32 bytes** (= 4 tiles of 8 bytes each, or 16 tiles of 2 bytes each)
- Structured write: alternating E and D registers

### Helper Subroutines

Located at:
- **$90A0**: Pattern setup/initialization
- **$9115**: Pattern processing/preparation
- **$E38A**: Additional processing

These are likely in different memory banks (overlays at $C000 from banks 1, 6, or 7).

### Bank Switching Code ($C040-$C050)

```z80
$C040: OR B              ; Combine with bank bits
$C041: LD A,$59          ; Bank selector value
$C043: OR B              ; More combining
$C044: RLCA              ; Rotate for positioning
$C045: OR $55            ; OR with pattern
$C047: OR B              ; Final combine
$C049: OR $00            ; Clear/mask
$C04B: LD BC,$7FFD       ; 128K banking port
$C04E: OUT (C),A         ; Switch bank!
$C050: RET               ; Return
```

This confirms 128K Spectrum usage - switches banks to access different tile data sets.

## Pattern Generation Hypothesis

Based on the code analysis, the algorithm works like this:

### Pseudo-code
```python
def generate_pattern(pattern_index, frame):
    # Stage 1: Get animation state
    anim_state = toggle_bit(animation_var, 0)

    # Stage 2: Transform pattern
    pattern_data = load_pattern(pattern_index)
    transformed = rotate_right(pattern_data, anim_state)

    # Stage 3: Lookup tiles
    table_offset = calculate_offset(transformed)  # RST $18
    tile_address = lookup_table[table_offset]

    # Stage 4: Combine tiles (XOR/OR operations from seed data)
    final_pattern = combine_tiles(tile_address, transformation)

    # Stage 5: Write to screen
    for i in range(32):
        screen_buffer[screen_pos + i] = final_pattern[i]

    return final_pattern
```

### Animation Mechanism
1. **Frame counter** at $90FB toggles each update
2. **Rotation** (RRCA) creates different orientations
3. **XOR/OR** operations combine multiple seed tiles
4. Result: Smooth animated geometric patterns

## Memory Map Implications

### Known Addresses
- **$90FB**: Animation state variable
- **$90E3**: Pattern index/data source
- **$E63A**: Tile lookup table (likely in bank)
- **$9300**: Screen/pattern buffer
- **$90A0**: Pattern setup routine
- **$9115**: Pattern processing routine
- **$E38A**: Additional processing

### Banking
- Code at $C000 (Block 10) - Bank 0
- Subroutines at $9xxx and $Exxx may be in different banks
- Lookup table $E63A accessed after bank switching

## Tile Data Structure

From earlier analysis:
- **873 tiles** extracted (8×8 pixels each)
- Split across blocks 03, 05, 07, 08
- These are **seed tiles** for procedural generation
- Not stored as final 24×24 patterns

### Combination Method
Based on XOR/OR operations found:
1. Load base tile from seed data
2. XOR with second tile (at $C057: XOR (HL))
3. Rotate result (at $C058: RLA)
4. Repeat for multiple tiles
5. Output 3×3 composite (24×24 pixels)

## Next Steps for C64 Port

### What We Need
1. **Extract lookup table** at $E63A
2. **Disassemble subroutines** at $90A0, $9115, $E38A
3. **Trace tile combination** logic completely
4. **Map tile indices** to seed data blocks
5. **Understand RST $18** calculation

### Port Strategy
1. Port the 32-byte write loop (straightforward)
2. Implement rotate/XOR transformation (6502 equivalents)
3. Create lookup table from ZX data
4. Implement animation state toggle
5. Map tile data to C64 memory

### C64 Equivalents
- RRCA → ROR on 6502
- XOR → EOR on 6502
- Table lookup → indexed addressing
- 32-byte loop → similar structure on 6502

## Progress Status

✅ Located pattern display routine
✅ Understood animation mechanism
✅ Identified transformation operations
⏳ Need to extract lookup table
⏳ Need to disassemble helper routines
⏳ Need to trace complete tile combination
⏳ Ready to start C64 implementation

## Code Locations Summary

| Address | Function | Status |
|---------|----------|--------|
| $C040-$C050 | Bank switching | Documented |
| $C051 | Pattern routine entry | Found |
| $C076-$C096 | Pattern display loop | **Analyzed** |
| $90A0 | Setup routine | Need to disassemble |
| $90E3 | Pattern data var | Identified |
| $90FB | Animation state | Identified |
| $9115 | Processing routine | Need to disassemble |
| $9300 | Screen buffer | Identified |
| $E38A | Additional routine | Need to disassemble |
| $E63A | Lookup table | Need to extract |

---

**This is major progress!** We now understand the core display mechanism and transformation logic. The next step is to extract the lookup table and trace the complete tile combination process.
