# First Pattern Implementation Plan

## What We Know

### Core Pattern Display Algorithm (ZX Spectrum)

```z80
; Pattern display at $C076-$C096
$C076: LD A,($90FB)      ; Load animation frame (0 or 1)
$C079: XOR $01           ; Toggle: 0↔1
$C07B: LD ($90FB),A      ; Store back

$C07E: LD A,($90E3)      ; Load pattern index
$C081: RRCA              ; Rotate right (creates variation)
$C082: LD HL,$E63A       ; Lookup table base
$C089: LD DE,$9300       ; Destination address
$C08C: ADD HL,DE         ; Calculate final address

; Write 32 bytes (4 tiles × 8 bytes)
$C090: LD B,$20          ; Counter = 32
$C092: LD (HL),E         ; Write pattern data
$C093: INC HL
$C094: LD (HL),D
$C095: INC HL
$C096: DJNZ $C092        ; Loop
```

### Key Insights

1. **Animation State**: Single bit toggle (0↔1)
2. **Pattern Transformation**: RRCA (rotate right) on pattern index
3. **Tile Combination**: XOR/OR operations combine seed tiles
4. **Output**: 32 bytes per pattern (4 tiles of 8 bytes each)
5. **Display Grid**: 8×8 patterns, each pattern is 24×24 pixels (3×3 tiles)

### Seed Data Available

- 873 tiles extracted (8×8 pixels each)
- 4 tile sets: pattern_set_1.s through pattern_set_4.s
- Already in C64 charset format

## Simplified First Test

Since the helper routines ($90A0, $9115) are in compressed overlays, let's create a simplified version that demonstrates the core concept:

### Test Pattern Generator (C64)

```assembly
; Simplified pattern generator for first test
; Generates one animated 24x24 pixel pattern

pattern_test:
    ; Animation state (ZP variable)
    lda anim_frame
    eor #$01          ; Toggle bit 0
    sta anim_frame

    ; Select base tile based on animation
    lda anim_frame
    beq @frame0
@frame1:
    lda #8            ; Tile 8 for frame 1
    jmp @generate
@frame0:
    lda #0            ; Tile 0 for frame 0

@generate:
    ; Multiply by 8 to get tile offset
    asl
    asl
    asl
    tax               ; X = tile offset

    ; Generate 3x3 tile pattern (24x24 pixels)
    ; Using XOR combination of tiles
    ldy #0
@tile_loop:
    lda pattern_charset,x
    eor pattern_charset+8,x   ; XOR with next tile
    ror                       ; Rotate (ZX uses RRCA)
    sta screen_buffer,y
    inx
    iny
    cpy #72               ; 9 tiles * 8 bytes = 72
    bne @tile_loop

    rts

; Variables
anim_frame:  .byte 0
```

## Implementation Steps

### Phase 1: Single Static Pattern
1. Display one 24×24 pattern (3×3 tiles) in center of screen
2. Use hardcoded tile indices
3. No animation yet
4. Goal: Prove display system works

### Phase 2: Add Simple Animation
1. Implement frame toggle (anim_frame 0↔1)
2. Switch between two tile sets
3. Add rotation (ROR equivalent to RRCA)
4. Goal: See animated pattern change

### Phase 3: Add XOR Combination
1. XOR multiple tiles together
2. Try different combinations
3. Match visual style to ZX original
4. Goal: Get closer to authentic patterns

### Phase 4: Full 8×8 Grid
1. Generate all 64 patterns
2. Position correctly in display area
3. Synchronize animation
4. Goal: Full pattern field like original

## C64 Memory Map

```
$0400-$07E7: Screen RAM (1000 bytes, 40×25)
$0800-$0FFF: Pattern charset (2KB, 256 chars)
$1000-$13FF: Screen buffer for patterns
$D800-$DBE7: Color RAM

Pattern display area: 32×24 characters (centered)
  Start: Row 0, Column 4 ($0400 + 4)
  Each pattern: 3×3 characters = 9 bytes
  8×8 grid = 64 patterns total
```

## Test Pattern Structure

For first test, create one 24×24 pattern manually:

```
Pattern composed of 9 tiles (3×3):
  [T0][T1][T0]
  [T1][T0^T1][T1]  ; Center is XOR of T0 and T1
  [T0][T1][T0]
```

Each tile: 8 bytes from pattern_charset
Total: 72 bytes for one pattern

## Expected Visual

```
 ╔════════════════════════════╗
 ║                            ║
 ║  ┌────┬────┬────┐          ║
 ║  │ ░░ │▒▒▒▒│ ░░ │          ║
 ║  │ ░░ │▒▒▒▒│ ░░ │          ║
 ║  ├────┼────┼────┤          ║
 ║  │▒▒▒▒│████│▒▒▒▒│          ║
 ║  │▒▒▒▒│████│▒▒▒▒│          ║
 ║  ├────┼────┼────┤          ║
 ║  │ ░░ │▒▒▒▒│ ░░ │          ║
 ║  │ ░░ │▒▒▒▒│ ░░ │          ║
 ║  └────┴────┴────┘          ║
 ║                            ║
 ╚════════════════════════════╝
```

## Success Criteria

✅ Phase 1: One static 24×24 pattern displays correctly
✅ Phase 2: Pattern animates with frame toggle and rotation
✅ Phase 3: XOR combination creates interesting visual
✅ Phase 4: Full 8×8 grid of animated patterns

## Next Steps

1. Implement Phase 1 in C64 assembly
2. Test on VICE emulator
3. Iterate until first pattern displays
4. Then proceed to animation

This gives us a concrete path forward without needing to fully reverse engineer the compressed ZX code!
