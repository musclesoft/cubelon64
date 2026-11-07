; sprite_mux.s - Sprite multiplexer for 8×4 grid (64 sprite positions total)
; Reuses 8 hardware sprites across 8 vertical positions by repositioning from raster interrupts
; Each position is spaced 24 pixels (3 chars) apart
; Alternates between two sprite pointers for visual variety

.include "include/vic.inc"
.include "sprite_mux.inc"

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

; Sprite data pointers ($2800 / 64 = $A0, $2880 / 64 = $A2)
SPRITE_PTR = $A0
SPRITE_PTR_REVERSE = $A2

SPRITE_HEIGHT = 21

row_y:
    .byte SPR_Y_ROW0, SPR_Y_ROW1, SPR_Y_ROW2, SPR_Y_ROW3
    .byte SPR_Y_ROW4, SPR_Y_ROW5, SPR_Y_ROW6, SPR_Y_ROW7

row_pointer:
    .byte SPRITE_PTR, SPRITE_PTR_REVERSE, SPRITE_PTR, SPRITE_PTR_REVERSE
    .byte SPRITE_PTR, SPRITE_PTR_REVERSE, SPRITE_PTR, SPRITE_PTR_REVERSE

; Raster line at which the sprites of the row before this one have finished
row_irq_line:
    .byte SPR_Y_ROW7+SPRITE_HEIGHT, SPR_Y_ROW0+SPRITE_HEIGHT
    .byte SPR_Y_ROW1+SPRITE_HEIGHT, SPR_Y_ROW2+SPRITE_HEIGHT
    .byte SPR_Y_ROW3+SPRITE_HEIGHT, SPR_Y_ROW4+SPRITE_HEIGHT
    .byte SPR_Y_ROW5+SPRITE_HEIGHT, SPR_Y_ROW6+SPRITE_HEIGHT

; Installs the raster interrupt chain. Sprites must already be at row 0.
; The caller installs the IRQ handler and enables interrupts.
; Modifies: A
.export sprite_multiplex_init
sprite_multiplex_init:
    lda #SPRITE_PTR
    sta SPRITE_PTRS+0
    sta SPRITE_PTRS+1
    sta SPRITE_PTRS+2
    sta SPRITE_PTRS+3
    sta SPRITE_PTRS+4
    sta SPRITE_PTRS+5
    sta SPRITE_PTRS+6
    sta SPRITE_PTRS+7
    lda #1
    sta MUX_ROW
    lda row_irq_line+1
    sta VIC_RASTER

    lda #$01
    sta VIC_IRQ_STATUS
    sta VIC_IRQ_ENABLE
    rts
