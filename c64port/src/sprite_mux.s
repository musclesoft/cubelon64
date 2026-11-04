; sprite_mux.s - Sprite multiplexer for 8×4 grid (64 sprite positions total)
; Reuses 8 hardware sprites across 8 vertical positions by repositioning from raster interrupts
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
SPRITE_PTR = $A0
SPRITE_PTR_REVERSE = $A1

MUX_ROW = $0217
SPRITE_IRQ_BORDER = 1

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
    lda #1
    sta MUX_ROW
    lda row_irq_line+1
    sta VIC_RASTER

    lda #$01
    sta VIC_IRQ_STATUS
    sta VIC_IRQ_ENABLE
    rts

; Moves all 8 sprites to the next row, then schedules the interrupt for the
; row after it. Row 0 is restored right after row 7, below the visible sprites.
; Preserves X and Y; clobbers A. The IRQ handler acknowledges the VIC.
; The border shows the time spent here.
.export sprite_multiplex_row
sprite_multiplex_row:
    txa
    pha
    tya
    pha
    ldy VIC_BORDER
    lda #SPRITE_IRQ_BORDER
    sta VIC_BORDER
    ldx MUX_ROW
    lda row_y,x
    sta VIC_SPR0_Y
    sta VIC_SPR0_Y+2
    sta VIC_SPR0_Y+4
    sta VIC_SPR0_Y+6
    sta VIC_SPR0_Y+8
    sta VIC_SPR0_Y+10
    sta VIC_SPR0_Y+12
    sta VIC_SPR0_Y+14

    lda row_pointer,x
    sta SPRITE_PTRS+0
    sta SPRITE_PTRS+1
    sta SPRITE_PTRS+2
    sta SPRITE_PTRS+3
    sta SPRITE_PTRS+4
    sta SPRITE_PTRS+5
    sta SPRITE_PTRS+6
    sta SPRITE_PTRS+7

    inx
    txa
    and #7
    sta MUX_ROW
    tax
    lda row_irq_line,x
    sta VIC_RASTER

    sty VIC_BORDER
    pla
    tay
    pla
    tax
    rts
