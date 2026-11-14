; simple_sprite_mux.s - The sprite multiplexer for tiles that fit the 21 rows of a sprite
;
; The marching cubes' squares use the tile rows 1-17, so a standard sprite that starts on the band's
; first line is enough: its 21 rows show the tile rows 0-20, and the three lines left in the 24 of
; the band are the gap in which one raster interrupt per band moves the 8 sprites to the next band
; and sets their pointers (see simple_sprite_mux.inc). There is no Y expansion, no partner sprite
; data and no switching of the screens: every band is shown from screen A, whose chars must be
; those of all bands, and the sprite pointer of each band is written into its pointer row.
;
; The other scenes keep the gapless multiplexer of square_sprite_mux.s, which shows all 24 rows of a
; tile. mux_select switches between the two before each frame of the main loop.

.include "include/vic.inc"
.include "simple_sprite_mux.inc"

.segment "BSS"

SIMPLE_BAND: .res 1                 ; The band the sprites move to at the next interrupt
SIMPLE_POINTERS: .res BANDS * 8     ; The pointer of sprite k in band b: SIMPLE_POINTERS + 8 * k + b
MUX_SIMPLE: .res 1                  ; Nonzero while the simple multiplexer runs

.segment "CODE"

.import sequencer_shows_cubes: absolute
.import sequencer_shows_cube3d: absolute
.import CUBE_SQUARE_POINTER: absolute
.import irq_use_simple: absolute
.import irq_use_gapless: absolute

.export mux_select
.export SIMPLE_POINTERS
.export simple_mux_init

; ─── Start with the gapless multiplexer, which the caller sets up ────────────
; Modifies: A
.proc simple_mux_init
    lda #0
    sta MUX_SIMPLE
    sta SIMPLE_BAND
    rts
.endproc

; ─── Run the multiplexer the tiles of the frame to come need ─────────────────
; The main loop calls this at the start of its step, in the vertical blank: the sprites wait
; at band 0 then, and no sprite is shown until its first line.
; Modifies: A, X, Y
.proc mux_select
    jsr sequencer_shows_cubes
    beq @gapless
    lda MUX_SIMPLE
    bne @done
    jmp enter_simple_mux
@gapless:
    lda MUX_SIMPLE
    beq @done
    jmp enter_gapless_mux
@done:
    rts
.endproc

; ─── Switch to the simple multiplexer ────────────────────────────────────────
; The marching cubes' bands alternate between the normal and the mirrored sprite.
.proc enter_simple_mux
    sei
    jsr irq_use_simple
    jsr fill_simple_pointers

    ldx #7
@pointer:
    txa
    asl
    asl
    asl
    tay
    lda SIMPLE_POINTERS,y
    sta POINTERS_A,x
    dex
    bpl @pointer

    lda #SPRITE_Y
    sta VIC_SPR0_Y+0
    sta VIC_SPR0_Y+2
    sta VIC_SPR0_Y+4
    sta VIC_SPR0_Y+6
    sta VIC_SPR0_Y+8
    sta VIC_SPR0_Y+10
    sta VIC_SPR0_Y+12
    sta VIC_SPR0_Y+14
    lda #0
    sta VIC_SPR_EXPAND_Y
    lda #FIRST_MEMPTR
    sta VIC_MEMPTR
    lda #1
    sta SIMPLE_BAND
    sta MUX_SIMPLE
    sta VIC_IRQ_STATUS
    lda #SPRITE_Y + SIMPLE_IRQ_OFFSET
    sta VIC_RASTER
    cli
    rts
.endproc

; ─── The pointers of all bands and sprites: the rotating cubes' square, the marching cubes' alternating ──
; Modifies: A, X
.proc fill_simple_pointers
    jsr sequencer_shows_cube3d
    bne @rings
    ldx #BANDS - 1
@band:
    txa
    and #1
    asl
    ora #SPRITE_POINTER_NORMAL
    sta SIMPLE_POINTERS+0,x
    sta SIMPLE_POINTERS+8,x
    sta SIMPLE_POINTERS+16,x
    sta SIMPLE_POINTERS+24,x
    sta SIMPLE_POINTERS+32,x
    sta SIMPLE_POINTERS+40,x
    sta SIMPLE_POINTERS+48,x
    sta SIMPLE_POINTERS+56,x
    dex
    bpl @band

    rts
@rings:
    ldx #BANDS * 8 - 1
    lda #CUBE_SQUARE_POINTER
@ring:
    sta SIMPLE_POINTERS,x
    dex
    bpl @ring
    rts
.endproc

; ─── Switch back to the gapless multiplexer ──────────────────────────────────
.proc enter_gapless_mux
    sei
    jsr irq_use_gapless
    jsr sprite_multiplex_init
    lda #0
    sta MUX_SIMPLE
    cli
    rts
.endproc

.segment "RODATA"

simple_band_y:
    .byte SPRITE_Y, SPRITE_Y + BAND_HEIGHT, SPRITE_Y + 2 * BAND_HEIGHT, SPRITE_Y + 3 * BAND_HEIGHT
    .byte SPRITE_Y + 4 * BAND_HEIGHT, SPRITE_Y + 5 * BAND_HEIGHT, SPRITE_Y + 6 * BAND_HEIGHT, SPRITE_Y + 7 * BAND_HEIGHT

simple_irq_line:
    .byte SPRITE_Y + SIMPLE_IRQ_OFFSET, SPRITE_Y + BAND_HEIGHT + SIMPLE_IRQ_OFFSET
    .byte SPRITE_Y + 2 * BAND_HEIGHT + SIMPLE_IRQ_OFFSET, SPRITE_Y + 3 * BAND_HEIGHT + SIMPLE_IRQ_OFFSET
    .byte SPRITE_Y + 4 * BAND_HEIGHT + SIMPLE_IRQ_OFFSET, SPRITE_Y + 5 * BAND_HEIGHT + SIMPLE_IRQ_OFFSET
    .byte SPRITE_Y + 6 * BAND_HEIGHT + SIMPLE_IRQ_OFFSET, SPRITE_Y + 7 * BAND_HEIGHT + SIMPLE_IRQ_OFFSET
