; reveal.s - Uncovers the 8×4 cube grid column by column as the cubes march
;
; The screen starts out filled with the fully opaque char (sprites sit behind
; it). A reveal step replaces one opaque column per grid row with the animated
; chars. Rows are 3 chars high (half a cube, matching the sprite bands):
;   rows 0, 2, 4, 6 (odd rows counting from 1) reveal left to right,
;   rows 1, 3, 5, 7 (even rows) reveal right to left, using the mirrored chars.
; The cubes march 3 chars per animation cycle (FRAME 0-39). The front advances
; one column on the 3 frames of the cycle where that column of a cube cell is
; empty, so the opaque char is replaced without a visible pop.

.include "include/shared.inc"

SCREEN = $4400
GRID_FIRST_COLUMN = 8
GRID_ORIGIN = SCREEN+GRID_FIRST_COLUMN
GRID_COLUMNS = 24
GRID_ROWS = 8
GRID_ROWS_SCREEN = 24
OPAQUE_CHAR = 63

.segment "ZEROPAGE"

SCREEN_PTR: .res 2

.segment "BSS"

REVEAL_STEP: .res 1         ; Char columns revealed per grid row so far

.segment "CODE"

.export reveal_init
.export reveal_step

; ─── Cover the whole grid with the opaque char and restart the reveal ────────
; Can be called again at any time to repeat the effect.
.proc reveal_init
    lda #0
    sta REVEAL_STEP
    lda #<GRID_ORIGIN
    sta SCREEN_PTR
    lda #>GRID_ORIGIN
    sta SCREEN_PTR+1
    ldx #GRID_ROWS_SCREEN
@screen_row:
    ldy #GRID_COLUMNS-1
    lda #OPAQUE_CHAR
@column:
    sta (SCREEN_PTR),y
    dey
    bpl @column
    lda SCREEN_PTR
    clc
    adc #40
    sta SCREEN_PTR
    bcc @same_page
    inc SCREEN_PTR+1
@same_page:
    dex
    bne @screen_row
    rts
.endproc

; ─── Reveal the next char column of every grid row on the cycle's trigger frames
; Call once per animation frame, after FRAME is set; does nothing once the grid
; is fully revealed.
.proc reveal_step
    lda FRAME
    cmp #7
    beq @trigger
    cmp #21
    beq @trigger
    cmp #33
    bne @done
@trigger:
    lda REVEAL_STEP
    cmp #GRID_COLUMNS
    bcs @done

    ldx #GRID_ROWS-1
@grid_row:
    txa
    lsr                     ; Carry set on odd indices = even rows from 1
    lda REVEAL_STEP
    bcc @column_ready
    eor #$FF
    clc
    adc #GRID_COLUMNS       ; 255-step+24 = GRID_COLUMNS-1-step
@column_ready:
    tay                     ; Y = grid column index
    clc
    adc grid_row_start_lo,x
    sta SCREEN_PTR
    lda grid_row_start_hi,x
    adc #0
    sta SCREEN_PTR+1

    lda half_char_offset,x  ; Carry is clear from here on: chars stay below 24
    adc column_char_base,y
    ldy #0
    sta (SCREEN_PTR),y
    adc #1
    ldy #40
    sta (SCREEN_PTR),y
    adc #1
    ldy #80
    sta (SCREEN_PTR),y

    dex
    bpl @grid_row

    inc REVEAL_STEP
@done:
    rts
.endproc

; Screen address of the top-left char of each grid row
grid_row_start_lo:
    .byte <(SCREEN+GRID_FIRST_COLUMN+0), <(SCREEN+GRID_FIRST_COLUMN+120), <(SCREEN+GRID_FIRST_COLUMN+240), <(SCREEN+GRID_FIRST_COLUMN+360)
    .byte <(SCREEN+GRID_FIRST_COLUMN+480), <(SCREEN+GRID_FIRST_COLUMN+600), <(SCREEN+GRID_FIRST_COLUMN+720), <(SCREEN+GRID_FIRST_COLUMN+840)
grid_row_start_hi:
    .byte >(SCREEN+GRID_FIRST_COLUMN+0), >(SCREEN+GRID_FIRST_COLUMN+120), >(SCREEN+GRID_FIRST_COLUMN+240), >(SCREEN+GRID_FIRST_COLUMN+360)
    .byte >(SCREEN+GRID_FIRST_COLUMN+480), >(SCREEN+GRID_FIRST_COLUMN+600), >(SCREEN+GRID_FIRST_COLUMN+720), >(SCREEN+GRID_FIRST_COLUMN+840)

; Odd rows show the lower half of the cube chars
half_char_offset:
    .byte 0, 3, 0, 3, 0, 3, 0, 3

; First char of each grid column: the three columns of a cube are 8 chars apart
column_char_base:
    .byte 0, 8, 16, 0, 8, 16, 0, 8, 16, 0, 8, 16
    .byte 0, 8, 16, 0, 8, 16, 0, 8, 16, 0, 8, 16
