; sprite_mux.s - Sprite multiplexer for 8×4 grid (64 sprite positions total)
; Reuses 8 hardware sprites across 8 vertical positions by repositioning during raster scan
; Each position is spaced 24 pixels (3 chars) apart

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

; Update sprite positions for all 8 rows during raster scan
; Called from wait_next_frame after raster synchronization
; Modifies: A
.export sprite_multiplex
sprite_multiplex:
    ; Row 1: wait 12 lines before sprite Y position
    lda #SPR_Y_ROW1-12
@wait_row1:
    cmp $D012
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

    ; Row 2: wait 12 lines before sprite Y position
    lda #SPR_Y_ROW2-12
@wait_row2:
    cmp $D012
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

    ; Row 3: wait 12 lines before sprite Y position
    lda #SPR_Y_ROW3-12
@wait_row3:
    cmp $D012
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

    ; Row 4: wait 12 lines before sprite Y position
    lda #SPR_Y_ROW4-12
@wait_row4:
    cmp $D012
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

    ; Row 5: wait 12 lines before sprite Y position
    lda #SPR_Y_ROW5-12
@wait_row5:
    cmp $D012
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

    ; Row 6: wait 12 lines before sprite Y position
    lda #SPR_Y_ROW6-12
@wait_row6:
    cmp $D012
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

    ; Row 7: wait 12 lines before sprite Y position
    lda #SPR_Y_ROW7-12
@wait_row7:
    cmp $D012
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

    rts
