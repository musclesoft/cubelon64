; square_sprite_mux.s - Gapless sprite multiplexer for the 8x4 cube grid
;
; 8 hardware sprites are reused for 8 bands of 24 raster lines each, one raster
; interrupt per band with exact write times. A sprite is 21 rows tall, so rows
; 19-21 are Y-expanded (18 + 3*2 = 24 lines). The screen base in $D018 is
; switched so the repeated line of each expanded row is fetched through the
; partner screen's sprite pointer.
;
; Line plan of a band (line 1 is a bad line with YSCROLL=7):
;   lines 1-18   rows 1-18 from the main screen
;   lines 19-24  rows 19,19,20,20,21,21 from the partner,main,main,partner,
;                partner,main screens
;
; Odd and even bands alternate between two screen pairs: A,B and C,D. A band
; uses its pair as main and partner screen, so the only per band state is the
; pair of $D018 values in MEMPTR_MAIN and MEMPTR_PARTNER. Only the main screens
; (A and C) show chars; the partner screens (B and D) only hold sprite pointers.
;
; Sprite data: SPRITE_NORMAL_MAIN/PARTNER hold the normal sprite, the mirror
; pair the mirrored one. Even bands use the normal pair, odd bands the mirror.

.include "include/vic.inc"
.include "square_sprite_mux.inc"

.segment "ZEROPAGE"

MEMPTR_MAIN: .res 1
MEMPTR_PARTNER: .res 1

.segment "CODE"

POINTERS_A = SCREEN_A + $3F8
POINTERS_B = SCREEN_B + $3F8
POINTERS_C = SCREEN_C + $3F8
POINTERS_D = SCREEN_D + $3F8


FIRST_MEMPTR = $08              ; screen A, charset $6000

; Makes the sprites a checkerboard of the normal and the mirrored sprite: even bands
; use the normal one on even columns and the mirrored one on odd columns, odd bands
; the other way round.
; Modifies: A, X
.export sprite_checker_pointers
sprite_checker_pointers:
    ldx #7
@pointers:
    txa
    and #1
    asl
    ora #SPRITE_POINTER_NORMAL
    sta POINTERS_A,x
    ora #1
    sta POINTERS_B,x
    lda POINTERS_A,x
    eor #SPRITE_POINTER_MIRROR - SPRITE_POINTER_NORMAL
    sta POINTERS_C,x
    ora #1
    sta POINTERS_D,x
    dex
    bpl @pointers
    rts

; Points all sprites at the normal sprite again. Safe while the interrupts run.
; Modifies: A, X
.export sprite_pointers_reset
sprite_pointers_reset:
    ldx #7
@pointers:
    lda #SPRITE_POINTER_NORMAL
    sta POINTERS_A,x
    ora #1
    sta POINTERS_B,x
    lda #SPRITE_POINTER_MIRROR
    sta POINTERS_C,x
    ora #1
    sta POINTERS_D,x
    dex
    bpl @pointers
    rts

; Installs the raster interrupt chain. Sprites must already be at band 0 with
; X positions set and Y expansion off. The caller installs the IRQ handler and
; enables interrupts.
; Modifies: A, X
.export sprite_multiplex_init
sprite_multiplex_init:
    jsr sprite_pointers_reset

    lda #FIRST_MEMPTR
    sta VIC_MEMPTR
    sta MEMPTR_MAIN
    eor #SCREEN_PARTNER_EOR
    sta MEMPTR_PARTNER

    lda #SPRITE_Y + IRQ_OFFSET
    sta VIC_RASTER
    lda #$01
    sta VIC_IRQ_STATUS
    sta VIC_IRQ_ENABLE
    rts
