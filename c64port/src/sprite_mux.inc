; sprite_mux.inc - Raster interrupt code of sprite_mux.s, inlined into the
; shared IRQ handler (the module's tables are shared through the single
; assembly unit)

SCREEN = $4000
SPRITE_PTRS = SCREEN + $3F8

MUX_ROW = $0217

; Moves all 8 sprites to the next row, then schedules the interrupt for the
; row after it. Row 0 is restored right after row 7, below the visible sprites.
; Preserves X; clobbers A. Acknowledges the VIC raster interrupt.
; The border shows the time spent here.
.macro sprite_multiplex_irq
    stx @restore_x+1
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

    lda #$01
    sta VIC_IRQ_STATUS

@restore_x:
    ldx #0
.endmacro
