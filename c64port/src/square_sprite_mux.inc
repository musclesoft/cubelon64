; square_sprite_mux.inc - Raster interrupt code of square_sprite_mux.s, inlined
; into the shared IRQ handler so that its cycle counts are exact
; (the module's zero page variables MEMPTR_MAIN and MEMPTR_PARTNER and the
; constants below are shared through the single assembly unit)

SPRITE_Y = 54
BANDS = 8
BAND_HEIGHT = 24
IRQ_OFFSET = 18                 ; raster line of the band's interrupt, relative to its sprite Y
SCREEN_PARTNER_EOR = $10        ; $D018 screen bits, A differs from B by this
SCREEN_PAIR_EOR = $20           ; $D018 screen bits, A and B differ from C and D by this

; Raster interrupt of one band: moves the sprites to the next band and
; acknowledges the VIC. Clobbers A, the caller saves it. The write times are
; cycle exact; the idle loop has room for other work. The handler enters this
; directly after `bit VIC_IRQ_STATUS` / `bmi` / `pha`.
.macro sprite_multiplex_irq
    lda MEMPTR_PARTNER
    sta VIC_MEMPTR
    dec VIC_SPR_EXPAND_Y
    lda #1
@reduce_jitter:
    bit VIC_RASTER
    beq @reduce_jitter

    sta VIC_IRQ_STATUS

    lda MEMPTR_MAIN
    sta VIC_MEMPTR

    write_ctrl_ch0
    lda #10
@idle_nop_loop:
    sec
    sbc #1
    bne @idle_nop_loop
    write_ctrl_ch0

    lda MEMPTR_PARTNER
    sta VIC_MEMPTR

    lda VIC_SPR0_Y
    clc
    adc #BAND_HEIGHT
    cmp #SPRITE_Y + BAND_HEIGHT * BANDS
    bcc @no_y_wrap
    lda #SPRITE_Y
    clc
@no_y_wrap:
    sta VIC_SPR0_Y+0
    sta VIC_SPR0_Y+2
    sta VIC_SPR0_Y+4
    sta VIC_SPR0_Y+6
    sta VIC_SPR0_Y+8
    sta VIC_SPR0_Y+10
    sta VIC_SPR0_Y+12
    sta VIC_SPR0_Y+14
    adc #IRQ_OFFSET
    sta VIC_RASTER

    lda MEMPTR_MAIN
    sta VIC_MEMPTR
    inc VIC_SPR_EXPAND_Y

    nop
    nop
    nop
    nop
    nop
    nop
    nop
    nop
    lda MEMPTR_PARTNER
    eor #SCREEN_PAIR_EOR
    sta MEMPTR_PARTNER
    eor #SCREEN_PARTNER_EOR
    sta MEMPTR_MAIN
    sta VIC_MEMPTR

.endmacro
