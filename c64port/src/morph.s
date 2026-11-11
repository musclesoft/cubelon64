; morph.s - Squares to diamonds morph
;
; The morph shows a kite in each corner of the 24x24 tile, rendered by the cube
; rasterizer. At init, every state's kite is rendered as a petal (rows 1-17, shifted
; 4 pixels), then folded into the full tile: rotated back by 4 pixels and mirrored into
; all four corners (see tile.inc for the tile). Every state's tile is converted to an image
; (see tiles.s), image N-1 of MORPH_IMAGES, which proto/golden/morph_images.bin checks.
; The tile is its own mirror image, so a morph step shows the image as both kinds of tile.

.include "include/shared_imports.inc"
.include "include/tile.inc"

HALF_TILE_ROWS = 12
LAST_ROW_OFFSET = TILE_BYTES - TILE_ROW_BYTES
HALF_TILE_BYTES = HALF_TILE_ROWS * TILE_ROW_BYTES

CORNER_COUNT = 4
WORD_A = 0
WORD_C = 2
WORD_P = 4
WORD_Q = 6

.segment "ZEROPAGE"

.segment "BSS"

MORPH_BUILD_STATE: .res 1
MORPH_CORNER: .res 1
MORPH_ROW_INDEX: .res 1
MORPH_ROW_OFFSET: .res 1
MORPH_WORD_LO: .res 1
MORPH_WORD_HI: .res 1
MORPH_WORDS: .res 8
MORPH_ROW: .res TILE_ROW_BYTES

.segment "SCRATCH"

MORPH_FOLDED: .res TILE_BYTES
MORPH_STAGE: .res TILE_BYTES

.segment "MORPH"

MORPH_IMAGES: .res MORPH_STATES * IMAGE_BYTES

.segment "CODE"

.import render_quad: absolute
.import job_checkpoint: absolute
.import MORPH_PETAL: absolute
.import BIT_REVERSE_TABLE: absolute
.import store_image: absolute
.import image_address: absolute
.import show_images: absolute
.import TILE_SRC_PTR: absolute
.import SHOW_IMAGE_A: absolute
.import SHOW_IMAGE_B: absolute

.export morph_precompute
.export morph_step
.export load_petal_row
.export mirror_column
.export MORPH_ROW
.export MORPH_ROW_INDEX

; ─── Build the tiles of all morph states ─────────────────────────────────────
.proc morph_precompute
    lda #PETAL_SLOT
    sta BUILD_SLOT
    lda #1
    sta MORPH_BUILD_STATE
@state:
    ldx MORPH_BUILD_STATE
    dex
    jsr set_kite_corners
    jsr render_quad
    jsr fold_petal
    jsr mirror_vertically
    inc MORPH_BUILD_STATE
    lda MORPH_BUILD_STATE
    cmp #MORPH_STATES + 1
    bcc @state
    assert_if MORPH_BUILD_STATE, 21, MORPH_IMAGES, "../proto/golden/morph_images.bin", 2880
    rts
.endproc

; ─── Write the corners of the kite of state X + 1 for the rasterizer ─────────
; Kite A=(a,a) B=(p,q) C=(c,c) D=(q,p), see morph_table.inc
.proc set_kite_corners
    lda morph_outer_table,x
    ldy #4
    jsr scale_word
    clc
    lda MORPH_WORD_LO
    adc #<MORPH_OUTER_BASE
    sta MORPH_WORDS + WORD_A
    lda MORPH_WORD_HI
    adc #>MORPH_OUTER_BASE
    sta MORPH_WORDS + WORD_A + 1

    lda morph_inner_table,x
    ldy #3
    jsr scale_word
    clc
    lda MORPH_WORD_LO
    adc #<MORPH_INNER_BASE
    sta MORPH_WORD_LO
    lda MORPH_WORD_HI
    adc #>MORPH_INNER_BASE
    sta MORPH_WORD_HI
    ldy #WORD_C
    jsr add_outer_word

    lda morph_near_table,x
    ldy #3
    jsr scale_word
    clc
    lda MORPH_WORD_LO
    adc #<MORPH_NEAR_BASE
    sta MORPH_WORD_LO
    lda MORPH_WORD_HI
    adc #>MORPH_NEAR_BASE
    sta MORPH_WORD_HI
    ldy #WORD_P
    jsr add_outer_word

    lda morph_far_table,x
    ldy #3
    jsr scale_word
    clc
    lda MORPH_WORD_LO
    adc #<MORPH_FAR_BASE
    sta MORPH_WORD_LO
    lda MORPH_WORD_HI
    adc #>MORPH_FAR_BASE
    sta MORPH_WORD_HI
    ldy #WORD_Q
    jsr add_outer_word

    lda #0
    sta MORPH_CORNER
@corner:
    lda MORPH_CORNER
    asl
    asl
    tay
    ldx MORPH_CORNER
    lda corner_x_word,x
    tax
    lda MORPH_WORDS,x
    sta CORNER0_X,y
    lda MORPH_WORDS+1,x
    sta CORNER0_X+1,y
    ldx MORPH_CORNER
    lda corner_y_word,x
    tax
    sec
    lda MORPH_WORDS,x
    sbc #Y_SAMPLE_BIAS
    sta CORNER0_Y,y
    lda MORPH_WORDS+1,x
    sbc #0
    sta CORNER0_Y+1,y
    inc MORPH_CORNER
    lda MORPH_CORNER
    cmp #CORNER_COUNT
    bne @corner
    rts
.endproc

; ─── MORPH_WORD = A << Y ─────────────────────────────────────────────────────
.proc scale_word
    sta MORPH_WORD_LO
    lda #0
    sta MORPH_WORD_HI
@shift:
    asl MORPH_WORD_LO
    rol MORPH_WORD_HI
    dey
    bne @shift
    rts
.endproc

; ─── MORPH_WORDS[Y] = MORPH_WORD + the outer word; keeps X ──────────────────
.proc add_outer_word
    clc
    lda MORPH_WORD_LO
    adc MORPH_WORDS + WORD_A
    sta MORPH_WORDS,y
    lda MORPH_WORD_HI
    adc MORPH_WORDS + WORD_A + 1
    sta MORPH_WORDS + 1,y
    rts
.endproc

; Which word each corner takes its X and Y from
corner_x_word:
    .byte WORD_A, WORD_P, WORD_C, WORD_Q
corner_y_word:
    .byte WORD_A, WORD_Q, WORD_C, WORD_P

; ─── Rotate the petal back by 4 pixels and mirror it horizontally ────────────
; Writes MORPH_FOLDED: per row the petal row, rotated left 4 pixels with wrap,
; combined with its mirror image. A mask bit is set where every part is set.
.proc fold_petal
    lda #0
    sta MORPH_ROW_INDEX
    sta MORPH_ROW_OFFSET
@row:
    jsr job_checkpoint
    jsr load_petal_row
    ldx #TILE_ROW_BYTES - 1
@byte:
    ldy mirror_column,x
    lda MORPH_ROW,y
    tay
    lda BIT_REVERSE_TABLE,y
    and MORPH_ROW,x
    pha
    txa
    clc
    adc MORPH_ROW_OFFSET
    tay
    pla
    sta MORPH_FOLDED,y
    dex
    bpl @byte

    lda MORPH_ROW_OFFSET
    clc
    adc #TILE_ROW_BYTES
    sta MORPH_ROW_OFFSET
    inc MORPH_ROW_INDEX
    lda MORPH_ROW_INDEX
    cmp #TILE_ROWS
    bne @row
    rts
.endproc

; ─── Load tile row MORPH_ROW_INDEX of the petal into MORPH_ROW ──────────────
; The petal row is rotated back by 4 pixels; a row the petal does not hold is empty.
.proc load_petal_row
    ldx MORPH_ROW_INDEX
    lda petal_row_offset,x
    bpl @petal_row
    ldx #TILE_ROW_BYTES - 1
    lda #$FF
@empty:
    sta MORPH_ROW,x
    dex
    bpl @empty
    rts

@petal_row:
    tay
    ldx #0
@copy:
    lda MORPH_PETAL,y
    sta MORPH_ROW,x
    iny
    inx
    cpx #TILE_ROW_BYTES
    bne @copy
    ldx #0
    jsr rotate_left_4
    ldx #3
    jmp rotate_left_4
.endproc

; ─── Rotate the 3 bytes of MORPH_ROW at X left by 4 pixels, with wrap ────────
.proc rotate_left_4
    ldy #4
@pixel:
    lda MORPH_ROW,x
    cmp #$80
    rol MORPH_ROW + 2,x
    rol MORPH_ROW + 1,x
    rol MORPH_ROW,x
    dey
    bne @pixel
    rts
.endproc

; Offset in the petal of the row each tile row takes: the petal holds rows 1-17,
; shifted down by 4, so tile rows 21-23 wrap around to petal rows 1-3. $FF: empty
petal_row_offset:
    .byte 18, 24, 30, 36, 42, 48, 54, 60, 66, 72, 78, 84, 90, 96
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte 0, 6, 12

; The byte holding the mirror image of each byte of a row
mirror_column:
    .byte 2, 1, 0, 5, 4, 3

; ─── Mirror the folded rows vertically into the tile of the current state ────
.proc mirror_vertically
    ldx #0
    ldy #LAST_ROW_OFFSET
@row_pair:
    lda MORPH_FOLDED,x
    and MORPH_FOLDED,y
    sta MORPH_STAGE,x
    sta MORPH_STAGE,y
    lda MORPH_FOLDED+1,x
    and MORPH_FOLDED+1,y
    sta MORPH_STAGE+1,x
    sta MORPH_STAGE+1,y
    lda MORPH_FOLDED+2,x
    and MORPH_FOLDED+2,y
    sta MORPH_STAGE+2,x
    sta MORPH_STAGE+2,y
    lda MORPH_FOLDED+3,x
    and MORPH_FOLDED+3,y
    sta MORPH_STAGE+3,x
    sta MORPH_STAGE+3,y
    lda MORPH_FOLDED+4,x
    and MORPH_FOLDED+4,y
    sta MORPH_STAGE+4,x
    sta MORPH_STAGE+4,y
    lda MORPH_FOLDED+5,x
    and MORPH_FOLDED+5,y
    sta MORPH_STAGE+5,x
    sta MORPH_STAGE+5,y
    tya
    sec
    sbc #TILE_ROW_BYTES
    tay
    txa
    clc
    adc #TILE_ROW_BYTES
    tax
    cpx #HALF_TILE_BYTES
    bne @row_pair

    lda #<MORPH_STAGE
    sta TILE_SRC_PTR
    lda #>MORPH_STAGE
    sta TILE_SRC_PTR + 1
    lda MORPH_BUILD_STATE
    sec
    sbc #1
    ldx #<MORPH_IMAGES
    ldy #>MORPH_IMAGES
    jmp store_image
.endproc

; ─── Show the tile of state SEQ_SCENE_STATE ─────────────────────────────────────────
.proc morph_step
    lda SEQ_SCENE_STATE
    sec
    sbc #1
    ldx #<MORPH_IMAGES
    ldy #>MORPH_IMAGES
    jsr image_address
    sta SHOW_IMAGE_A
    sta SHOW_IMAGE_B
    sty SHOW_IMAGE_A + 1
    sty SHOW_IMAGE_B + 1
    jmp show_images
.endproc
