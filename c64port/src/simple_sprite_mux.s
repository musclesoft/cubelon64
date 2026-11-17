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
MUX_DISSOLVE: .res 1                ; Nonzero from the frame before the flower's dissolve (flower_dissolve_tick sets it)
simple_band_y: .res BANDS           ; The Y of the sprites in each band and the line of its interrupt: SPRITE_Y + 24 * b,
simple_irq_line: .res BANDS         ; + SIMPLE_IRQ_OFFSET; the dissolve's tiles start one line lower

.segment "CODE"

.import sequencer_shows_cubes: absolute
.import sequencer_shows_cube3d: absolute
.import CUBE_SQUARE_POINTER: absolute
.import irq_use_simple: absolute
.import irq_use_gapless: absolute

.export mux_select
.export SIMPLE_POINTERS
.export MUX_DISSOLVE
.export simple_mux_init
.export fill_marching_pointers

; ─── Start with the gapless multiplexer, which the caller sets up ────────────
; Modifies: A

.proc simple_mux_init
    lda #0
    sta MUX_DISSOLVE
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
    bne @simple
    lda MUX_DISSOLVE
    beq @gapless
@simple:
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
    jsr fill_simple_tables
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

    lda simple_band_y
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
    lda simple_irq_line
    sta VIC_RASTER
    cli
    rts
.endproc

; ─── The Y of the sprites and the interrupt line of each band ────────────────
; The tiles of the dissolve use the rows 1-21, so their sprites start one line below the band's
; Modifies: A, X
.proc fill_simple_tables
    lda MUX_DISSOLVE
    clc
    adc #SPRITE_Y
    ldx #0
@band:
    sta simple_band_y,x
    clc
    adc #SIMPLE_IRQ_OFFSET
    sta simple_irq_line,x
    clc
    adc #BAND_HEIGHT - SIMPLE_IRQ_OFFSET
    inx
    cpx #BANDS
    bcc @band
    rts
.endproc

; ─── The pointers of all bands and sprites: the rotating cubes' square, the marching cubes' alternating ──
; Modifies: A, X
.proc fill_simple_pointers
    lda MUX_DISSOLVE
    beq @cubes
    rts
@cubes:
    jsr sequencer_shows_cube3d
    bne @rings
    jmp fill_marching_pointers
@rings:
    ldx #BANDS * 8 - 1
    lda #CUBE_SQUARE_POINTER
@ring:
    sta SIMPLE_POINTERS,x
    dex
    bpl @ring
    rts
.endproc

; ─── The marching cubes' pointers: the bands alternate between the normal and the mirrored sprite ──
; Modifies: A, X
.proc fill_marching_pointers
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
