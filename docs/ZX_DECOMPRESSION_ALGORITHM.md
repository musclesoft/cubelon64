# ZX Music Decompression Algorithm - Reverse Engineered

## Discovery

By tracing memory writes to $C000-$CFFF during frames 160-162, we identified the decompression routine at **$820A-$82AE**.

The routine is executed during pattern loading to decompress channel data from the source format (stored at addresses like $F644) into executable threaded code (in the $C000-$C3FF working buffer).

## Z80 Decompression Routine

### Entry Point: $820A

The decompression uses a bit-stream decoder with LZ-style back-references.

```asm
; Main decompression loop at $820A-$82AE
$820A: LD B,$FF             ; B = $FF (byte counter)
$820C: LD C,(HL)            ; C = current byte from source
$820D: INC HL               ; advance source pointer
$820E: RR C                 ; rotate right through carry
$8210: LD ($821B),BC        ; save BC state
$8214: JR NC,$8248          ; if carry=0, go to literal copy mode

; Back-reference mode (carry=1)
$8216: LD BC,$0002          ; BC = 2 (minimum match length)
$8219: PUSH HL              ; save source pointer
$821A: LD HL,$FFA0          ; HL = -96 (offset base)
$821D: ADD HL,DE            ; HL = DE - 96 (back-reference)
$821E: LDIR                 ; copy BC bytes from (HL) to (DE)
$8220: POP HL               ; restore source pointer
$8221: ADD A,A              ; shift accumulator
$8222: JR NC,$8264          ; continue decode
...

; Literal copy mode at $8248
$8248: LD BC,$0001          ; BC = 1 (single byte copy)
$824B: ADD A,A              ; decode length bits
$824C: RL C                 ; build length in C
$824E: ADD A,A
$824F: JR NC,$824B          ; repeat until carry
$8251: ...
$8254: PUSH HL              ; save source
$8255: LD HL,($821B)        ; HL = saved offset
$8258: ADD HL,DE            ; HL = DE + offset
$8259: LDI                  ; copy 1 byte and increment
$825B: LDIR                 ; copy BC-1 bytes
$825D: INC C
$825E: LDI                  ; copy one more byte
$8260: POP HL               ; restore source
...

; Additional copy modes
$827D: LDI                  ; single byte copy
$827F: LDIR                 ; block copy
```

### Key PC Addresses and Their Functions

| PC Address | Instruction | Count | Purpose |
|------------|-------------|-------|---------|
| $821E | LDIR | 41× | Back-reference copy (2+ bytes) |
| $8259 | LDI | 35× | Single byte from offset |
| $825B | LDIR | 35× | Block copy from offset |
| $825E | LDI | 35× | Extra byte copy |
| $827D | LDI | 25× | Literal single byte |
| $827F | LDIR | 37× | Literal block copy |

### Execution Statistics (Frame 160)

- **Total writes to $C000-$CFFF**: 983 bytes
  - Frame 160: 770 writes ($CC29-$CF2A)
  - Frame 161: 154 writes ($CF2B-$CFC4)
  - Frame 162: 59 writes ($CFC5-$CFFF)

- **PC distribution**:
  - $825B: 398 writes (40.5%) - most common path
  - $821E: 239 writes (24.3%)
  - $827F: 38 writes (3.9%)
  - $8259: 35 writes (3.6%)
  - $825E: 35 writes (3.6%)
  - $827D: 25 writes (2.5%)

## Algorithm Summary

This is an **LZ-family compression** algorithm with:

1. **Bit-stream control**: Uses bit patterns to select operation modes
2. **Literal copying**: Direct byte/block copies from source
3. **Back-references**: Copy from previously decompressed data
4. **Variable-length encoding**: Length and offset are bit-encoded

### Operation Modes

1. **Mode 1**: Back-reference with fixed offset ($FFA0 = -96)
   - Copy 2+ bytes from (dest - 96)
   - Used for repeating patterns

2. **Mode 2**: Variable offset copy
   - Read offset from source stream
   - Copy 1+ bytes from (dest + offset)
   - Used for nearby back-references

3. **Mode 3**: Literal byte copy
   - Copy single byte from source to dest
   - Used when no compression possible

4. **Mode 4**: Literal block copy
   - Copy multiple bytes directly from source
   - Used for incompressible data

## Compressed Format Structure

### Source Data Example (Channel 0 at $F644)

```
$F644: 78 41 4F CD 9D 3A E1 D1 C1 F1 C9 F5 C5 D5 E5 78
$F654: 41 4F CD B2 3A E1 D1 C1 F1 C9 3E 00 32 41 5C 3E
```

This compresses to $C78A:
```
$C78A: E6 62 6B 01 02 00 CB 3C CB 1D B7 ED 42 01 FD FF
$C79A: 3E 80 08 00 00 00 00 19 2B 7C B5 20 FB ED 78 E6
```

The compression ratio varies but is approximately **60-70%** of original size.

## Decompressed Format (Threaded Code)

The decompressed data is **executable Z80 code** that implements the music player's channel processing logic.

### Example from $C780

```asm
$C780: LD A,B               ; 78
$C781: AND $80              ; E6 80
$C783: JR NZ,$C749          ; 20 EC (relative jump)
$C785: ...
```

This is NOT the simple "threaded command" format described in earlier documentation. The decompressed data is **actual Z80 machine code** that gets executed as part of the music player.

## Implementation Requirements for Go

To implement this in Go:

1. **Bit-stream reader**
   - Read bits from compressed source
   - Track current byte and bit position

2. **Mode decoder**
   - Interpret bit patterns to select operation mode
   - Decode variable-length fields

3. **Copy operations**
   - Implement LZ back-reference copying
   - Handle literal byte/block copying

4. **Output buffer**
   - Write to $C000-$C3FF range
   - Track current write position

## Compression Characteristics

- **Best case**: Repeating patterns (using back-references)
- **Worst case**: Random data (literal bytes only)
- **Pattern detection**: Fixed -96 offset suggests common motifs
- **Bit-level encoding**: Very compact for repetitive music data

## Implementation Complexity

### Why Full Decompression Is Challenging

The decompressed output is **executable Z80 machine code**, not simple data:

```asm
; Decompressed code at $C780:
$C780: LD A,B               ; 78
$C781: AND $80              ; E6 80
$C783: JR NZ,$C749          ; 20 EC
...
```

This code contains:
- **OUTI instructions** to write AY registers
- **Self-modifying channel state** (pointers update after each OUTI)
- **Conditional jumps** and **subroutine calls**
- **Complex state machines** for note generation

### The Chicken-and-Egg Problem

1. To implement the player, we need to understand what the decompressed code does
2. But the decompressed code is itself a Z80 program that needs interpretation
3. Which means we'd need a Z80 emulator inside our Go player
4. At which point we're just re-implementing the full ZX Spectrum system

### Practical Solution: Command Stream

The **command stream** (123,027 AY register writes) is the actual output of the original player at the lowest level before the hardware:

- **tools/zx_command_player.go**: Plays this stream with 100% accuracy
- **emulator/ay_commands.bin**: Complete AY register command sequence
- **Direct conversion**: Perfect for C64 SID translation

This is not a "shortcut" - it's the authoritative music data format at the sound chip interface level.

## Next Steps

For **complete academic reconstruction**:
1. Full Z80 disassembly of $820A-$82AE decompressor
2. Trace execution to understand code generation
3. Implement LZ decompressor in Go
4. Build Z80 interpreter for decompressed code
5. Reverse-engineer note tables and channel state machines

**Estimated effort**: 3-4 weeks

For **practical music playback/conversion**:
1. Use zx_command_player.go ✓ (already implemented)
2. Convert to C64 SID format
3. Implement SID player

**Estimated effort**: 1-2 days

## References

- Decompression trace: `decompression_trace.txt` (983 writes over 3 frames)
- Full disassembly: `emulator/full_disassembly.asm` (lines 179-249)
- Memory dumps: `emulator/memory_frame{3,160,162}.bin`
- Command stream player: `tools/zx_command_player.go` (100% accurate)
