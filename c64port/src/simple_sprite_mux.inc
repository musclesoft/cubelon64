; simple_sprite_mux.inc - Raster interrupt code of simple_sprite_mux.s, inlined into the shared
; IRQ handler (the module's variables and tables and the constants of square_sprite_mux.inc are
; shared through the single assembly unit)

SIMPLE_IRQ_OFFSET = 21          ; The band's interrupt line relative to its sprite Y: the sprite's last line

; Raster interrupt of one band: moves the sprites to the next band, points them at its tiles and
; acknowledges the VIC. The sprites are 21 lines tall and a band has 24, so after the sprite's last
; line nothing is fetched until the data fetch for the next band's first line, in cycle 58 of the
; line before it; the writes must end before that, which they do with room to spare (see
; experiments/cube_mux.s). They are not cycle exact, but the interrupts are masked, so the music's
; waveform is written between them. Clobbers A and X, the caller saves them. The handler enters
; this after `bit VIC_IRQ_STATUS` / `bmi` / `pha` and the saving of X and Y.
.macro simple_sprite_irq
    ldx SIMPLE_BAND
    lda simple_band_y,x
    sta VIC_SPR0_Y+0
    sta VIC_SPR0_Y+2
    sta VIC_SPR0_Y+4
    sta VIC_SPR0_Y+6
    sta VIC_SPR0_Y+8
    sta VIC_SPR0_Y+10
    sta VIC_SPR0_Y+12
    sta VIC_SPR0_Y+14
    write_ctrl_ch0

    lda SIMPLE_POINTERS+0,x
    sta POINTERS_A+0
    lda SIMPLE_POINTERS+8,x
    sta POINTERS_A+1
    lda SIMPLE_POINTERS+16,x
    sta POINTERS_A+2
    lda SIMPLE_POINTERS+24,x
    sta POINTERS_A+3
    write_ctrl_ch0

    lda SIMPLE_POINTERS+32,x
    sta POINTERS_A+4
    lda SIMPLE_POINTERS+40,x
    sta POINTERS_A+5
    lda SIMPLE_POINTERS+48,x
    sta POINTERS_A+6
    lda SIMPLE_POINTERS+56,x
    sta POINTERS_A+7
    write_ctrl_ch0

    lda simple_irq_line,x
    sta VIC_RASTER
    lda #1
    sta VIC_IRQ_STATUS
    inx
    txa
    and #BANDS - 1
    sta SIMPLE_BAND
.endmacro
