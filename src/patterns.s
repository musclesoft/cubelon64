;
; Pattern generation for CUBELON C64 port
; Phase 1: Single static 24x24 pattern test
;

.export pattern_test
.export anim_frame

.import pattern_charset
.importzp ptr1, ptr2

.segment "CODE"

;-----------------------------------------------------------------------------
; pattern_test - Generate and display one test pattern
;
; Based on ZX Spectrum algorithm at $C076-$C096:
;   - Toggle animation frame
;   - Rotate pattern data
;   - XOR combine tiles
;   - Display as 3x3 tile grid (24x24 pixels)
;-----------------------------------------------------------------------------
pattern_test:
    ; Toggle animation frame (0 <-> 1)
    lda anim_frame
    eor #$01
    sta anim_frame

    ; Generate 3x3 tile pattern (9 tiles total)
    ; Each tile is 8 bytes, so 72 bytes total
    ; Pattern structure:
    ;   [T0][T1][T0]      Tile indices:  [0][1][0]
    ;   [T1][T2][T1]                     [1][2][1]
    ;   [T0][T1][T0]                     [0][1][0]
    ; Where T2 = T0 XOR T1 (combined tile)

    ; Calculate screen position for centered pattern
    ; Screen is 40x25, we want pattern at column 18, row 15 (below test message)
    ; Screen offset = (15 * 40) + 18 = 618 = $26A
    lda #<($0400 + 618)
    sta ptr1
    lda #>($0400 + 618)
    sta ptr1+1

    ; Row 1: T0, T1, T0
    jsr draw_tile_0
    jsr draw_tile_1
    jsr draw_tile_0

    ; Reset to start column and move to next screen row
    ; ptr1 is now at column 21, we need column 18 of next row
    ; Subtract 3 to get back to column 18, then add 40 for next row = add 37
    lda ptr1
    clc
    adc #37           ; 40 - 3 = 37
    sta ptr1
    lda ptr1+1
    adc #0
    sta ptr1+1

    ; Row 2: T1, T2, T1
    jsr draw_tile_1
    jsr draw_tile_combined
    jsr draw_tile_1

    ; Reset to start column and move to next screen row
    lda ptr1
    clc
    adc #37           ; 40 - 3 = 37
    sta ptr1
    lda ptr1+1
    adc #0
    sta ptr1+1

    ; Row 3: T0, T1, T0
    jsr draw_tile_0
    jsr draw_tile_1
    jsr draw_tile_0

    rts

;-----------------------------------------------------------------------------
; Draw individual tiles
; Each tile is 8 bytes of graphics data
;-----------------------------------------------------------------------------
draw_tile_0:
    ; Select tile index based on animation frame
    lda anim_frame
    beq @static
    lda #$08          ; Animated: use tile 8
    jmp @draw
@static:
    lda #$00          ; Static: use tile 0
@draw:
    ; Draw 3 characters (tile is 8x8, char is also 8x8)
    ; So one tile = 1 character
    pha               ; Save tile index
    jsr set_char_at_ptr
    pla
    inc ptr1          ; Next column
    bne :+
    inc ptr1+1
:   rts

draw_tile_1:
    lda anim_frame
    beq @static
    lda #$09          ; Animated: use tile 9
    jmp @draw
@static:
    lda #$01          ; Static: use tile 1
@draw:
    pha
    jsr set_char_at_ptr
    pla
    inc ptr1
    bne :+
    inc ptr1+1
:   rts

draw_tile_combined:
    ; This should be T0 XOR T1, but for Phase 1 just use tile 2
    lda anim_frame
    beq @static
    lda #$0A          ; Animated: use tile 10
    jmp @draw
@static:
    lda #$02          ; Static: use tile 2
@draw:
    jsr set_char_at_ptr
    inc ptr1
    bne :+
    inc ptr1+1
:   rts

;-----------------------------------------------------------------------------
; set_char_at_ptr - Write character code to screen at ptr1
; Input: A = character code
;-----------------------------------------------------------------------------
set_char_at_ptr:
    ldy #0
    sta (ptr1),y

    ; Also set color to white in color RAM
    ; Color RAM = $D800 + (screen offset from $0400)
    ; Calculate: $D800 + (ptr1 - $0400) = $D400 + ptr1
    pha
    lda ptr1
    sta ptr2
    lda ptr1+1
    clc
    adc #$D0          ; $D400 + ptr1 = color RAM offset
    sta ptr2+1

    lda #$01          ; White
    sta (ptr2),y
    pla
    rts

;-----------------------------------------------------------------------------
; Data
;-----------------------------------------------------------------------------
.segment "DATA"

anim_frame:
    .byte 0           ; Animation frame counter (0 or 1)

