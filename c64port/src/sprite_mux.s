; sprite_mux.s - Sprite multiplexer for 8×4 grid (64 sprite positions total)
; Reuses 8 hardware sprites across 8 vertical positions by repositioning during raster scan
; Each position is spaced 24 pixels (3 chars) apart
; Alternates between two sprite pointers for visual variety

.include "include/vic.inc"

.segment "CODE"

; Sprite Y positions spaced 24 pixels apart (adjusted for YSCROLL=7)
; Sprites are 21 pixels tall, need 8 positions to cover 24 chars (192 pixels)
SPR_Y_ROW0 = 54   ; chars 0-2   → Y = 50 + 0*8 + 4
SPR_Y_ROW1 = 78   ; chars 3-5   → Y = 50 + 3*8 + 4
SPR_Y_ROW2 = 102  ; chars 6-8   → Y = 50 + 6*8 + 4
SPR_Y_ROW3 = 126  ; chars 9-11  → Y = 50 + 9*8 + 4
SPR_Y_ROW4 = 150  ; chars 12-14 → Y = 50 + 12*8 + 4
SPR_Y_ROW5 = 174  ; chars 15-17 → Y = 50 + 15*8 + 4
SPR_Y_ROW6 = 198  ; chars 18-20 → Y = 50 + 18*8 + 4
SPR_Y_ROW7 = 222  ; chars 21-23 → Y = 50 + 21*8 + 4

; Sprite pointers (at end of screen memory)
SCREEN = $4400
SPRITE_PTRS = SCREEN + $3F8

; Sprite data pointers ($2800 / 64 = $A0, $2840 / 64 = $A1)
SPRITE_PTR_A = $A0
SPRITE_PTR_B = $A1

; Update sprite positions for all 8 rows during raster scan
; Called from wait_next_frame after raster synchronization
; Modifies: A
.export sprite_multiplex
sprite_multiplex:
    lda VIC_RASTER
    bmi sprite_multiplex

    ; Row 1: wait 21 lines after row 0 (right after previous sprite finishes)
    lda #SPR_Y_ROW0+21
@wait_row1:
    cmp VIC_RASTER
    bne @wait_row1

    lda #SPR_Y_ROW1
    sta VIC_SPR0_Y
    sta VIC_SPR0_Y+2
    sta VIC_SPR0_Y+4
    sta VIC_SPR0_Y+6
    sta VIC_SPR0_Y+8
    sta VIC_SPR0_Y+10
    sta VIC_SPR0_Y+12
    sta VIC_SPR0_Y+14

    ; Switch to sprite pointer B (odd row)
    lda #SPRITE_PTR_B
    sta SPRITE_PTRS+0
    sta SPRITE_PTRS+1
    sta SPRITE_PTRS+2
    sta SPRITE_PTRS+3
    sta SPRITE_PTRS+4
    sta SPRITE_PTRS+5
    sta SPRITE_PTRS+6
    sta SPRITE_PTRS+7

    ; Row 2: wait at row 0 + 45 (21 lines after row 1)
    lda #SPR_Y_ROW1+21
@wait_row2:
    cmp VIC_RASTER
    bne @wait_row2

    lda #SPR_Y_ROW2
    sta VIC_SPR0_Y
    sta VIC_SPR0_Y+2
    sta VIC_SPR0_Y+4
    sta VIC_SPR0_Y+6
    sta VIC_SPR0_Y+8
    sta VIC_SPR0_Y+10
    sta VIC_SPR0_Y+12
    sta VIC_SPR0_Y+14

    ; Switch to sprite pointer A (even row)
    lda #SPRITE_PTR_A
    sta SPRITE_PTRS+0
    sta SPRITE_PTRS+1
    sta SPRITE_PTRS+2
    sta SPRITE_PTRS+3
    sta SPRITE_PTRS+4
    sta SPRITE_PTRS+5
    sta SPRITE_PTRS+6
    sta SPRITE_PTRS+7

    ; Row 3: wait at row 0 + 69 (21 lines after row 2)
    lda #SPR_Y_ROW2+21
@wait_row3:
    cmp VIC_RASTER
    bne @wait_row3

    lda #SPR_Y_ROW3
    sta VIC_SPR0_Y
    sta VIC_SPR0_Y+2
    sta VIC_SPR0_Y+4
    sta VIC_SPR0_Y+6
    sta VIC_SPR0_Y+8
    sta VIC_SPR0_Y+10
    sta VIC_SPR0_Y+12
    sta VIC_SPR0_Y+14

    ; Switch to sprite pointer B (odd row)
    lda #SPRITE_PTR_B
    sta SPRITE_PTRS+0
    sta SPRITE_PTRS+1
    sta SPRITE_PTRS+2
    sta SPRITE_PTRS+3
    sta SPRITE_PTRS+4
    sta SPRITE_PTRS+5
    sta SPRITE_PTRS+6
    sta SPRITE_PTRS+7

    ; Row 4: wait at row 0 + 93 (21 lines after row 3)
    lda #SPR_Y_ROW3+21
@wait_row4:
    cmp VIC_RASTER
    bne @wait_row4

    lda #SPR_Y_ROW4
    sta VIC_SPR0_Y
    sta VIC_SPR0_Y+2
    sta VIC_SPR0_Y+4
    sta VIC_SPR0_Y+6
    sta VIC_SPR0_Y+8
    sta VIC_SPR0_Y+10
    sta VIC_SPR0_Y+12
    sta VIC_SPR0_Y+14

    ; Switch to sprite pointer A (even row)
    lda #SPRITE_PTR_A
    sta SPRITE_PTRS+0
    sta SPRITE_PTRS+1
    sta SPRITE_PTRS+2
    sta SPRITE_PTRS+3
    sta SPRITE_PTRS+4
    sta SPRITE_PTRS+5
    sta SPRITE_PTRS+6
    sta SPRITE_PTRS+7

    ; Row 5: wait at row 0 + 117 (21 lines after row 4)
    lda #SPR_Y_ROW4+21
@wait_row5:
    cmp VIC_RASTER
    bne @wait_row5

    lda #SPR_Y_ROW5
    sta VIC_SPR0_Y
    sta VIC_SPR0_Y+2
    sta VIC_SPR0_Y+4
    sta VIC_SPR0_Y+6
    sta VIC_SPR0_Y+8
    sta VIC_SPR0_Y+10
    sta VIC_SPR0_Y+12
    sta VIC_SPR0_Y+14

    ; Switch to sprite pointer B (odd row)
    lda #SPRITE_PTR_B
    sta SPRITE_PTRS+0
    sta SPRITE_PTRS+1
    sta SPRITE_PTRS+2
    sta SPRITE_PTRS+3
    sta SPRITE_PTRS+4
    sta SPRITE_PTRS+5
    sta SPRITE_PTRS+6
    sta SPRITE_PTRS+7

    ; Row 6: wait at row 0 + 141 (21 lines after row 5)
    lda #SPR_Y_ROW5+21
@wait_row6:
    cmp VIC_RASTER
    bne @wait_row6

    lda #SPR_Y_ROW6
    sta VIC_SPR0_Y
    sta VIC_SPR0_Y+2
    sta VIC_SPR0_Y+4
    sta VIC_SPR0_Y+6
    sta VIC_SPR0_Y+8
    sta VIC_SPR0_Y+10
    sta VIC_SPR0_Y+12
    sta VIC_SPR0_Y+14

    ; Switch to sprite pointer A (even row)
    lda #SPRITE_PTR_A
    sta SPRITE_PTRS+0
    sta SPRITE_PTRS+1
    sta SPRITE_PTRS+2
    sta SPRITE_PTRS+3
    sta SPRITE_PTRS+4
    sta SPRITE_PTRS+5
    sta SPRITE_PTRS+6
    sta SPRITE_PTRS+7

    ; Row 7: wait at row 0 + 165 (21 lines after row 6)
    lda #SPR_Y_ROW6+21
@wait_row7:
    cmp VIC_RASTER
    bne @wait_row7

    lda #SPR_Y_ROW7
    sta VIC_SPR0_Y
    sta VIC_SPR0_Y+2
    sta VIC_SPR0_Y+4
    sta VIC_SPR0_Y+6
    sta VIC_SPR0_Y+8
    sta VIC_SPR0_Y+10
    sta VIC_SPR0_Y+12
    sta VIC_SPR0_Y+14

    ; Switch to sprite pointer B (odd row)
    lda #SPRITE_PTR_B
    sta SPRITE_PTRS+0
    sta SPRITE_PTRS+1
    sta SPRITE_PTRS+2
    sta SPRITE_PTRS+3
    sta SPRITE_PTRS+4
    sta SPRITE_PTRS+5
    sta SPRITE_PTRS+6
    sta SPRITE_PTRS+7

    rts

; Reset sprites to row 0 for next frame
; Called after wait_frame to prepare for next display cycle
; Modifies: A
.export sprite_multiplex_reset
sprite_multiplex_reset:
    lda #SPR_Y_ROW0
    sta VIC_SPR0_Y
    sta VIC_SPR0_Y+2
    sta VIC_SPR0_Y+4
    sta VIC_SPR0_Y+6
    sta VIC_SPR0_Y+8
    sta VIC_SPR0_Y+10
    sta VIC_SPR0_Y+12
    sta VIC_SPR0_Y+14

    ; Reset to sprite pointer A (row 0 is even)
    lda #SPRITE_PTR_A
    sta SPRITE_PTRS+0
    sta SPRITE_PTRS+1
    sta SPRITE_PTRS+2
    sta SPRITE_PTRS+3
    sta SPRITE_PTRS+4
    sta SPRITE_PTRS+5
    sta SPRITE_PTRS+6
    sta SPRITE_PTRS+7

    rts
