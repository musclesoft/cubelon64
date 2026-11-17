; flower_dissolve.s - The images of the flower's dissolve
;
; The petals of the final flower vanish one after the other, each by its own steps (proto/flower_dissolve.go, the data in
; flower_dissolve_data.inc): a step is a mask of 4 by 4 subpixels of the square's ladder (flower_square_masks) that
; lets part of the petal's subpixels through, or of the ring of the petal, which is the petal without a smaller one
; of the same centre (which lies inside it). The chords of the petals come from the flower's renderer, but instead of
; setting bits they add to the count of each pixel the subpixels of the chord that the petal's mask lets through, and a
; ring subtracts the subpixels of the smaller petal's chords: the count is what the pixel's grey comes from (3 or more
; light grey, 1 or 2 dark grey), so no row of bits and no pack is needed.

.include "include/shared_imports.inc"
.include "include/tile.inc"

.import flower_square_masks: absolute

.export flower_count_span

; ─── Render the images 0-22 of the dissolve (the frames 3952-3974) ───────────
.segment "FLOWER43"

.proc flower_dissolve_precompute
    lda #0
    sta DS_IMAGE
@image:
    jsr flower_dissolve_image
    inc DS_IMAGE
    lda DS_IMAGE
    cmp #DISSOLVE_IMAGE_COUNT
    bcc @image
    assert_if DS_IMAGE, 23, early_start, "../proto/golden/flower_dissolve_images.bin", 3168
    assert_if DS_IMAGE, 23, spin_half_tiles, "../proto/golden/flower_dissolve_images.bin" + 3168, 144
    rts
.endproc

; Renders the image DS_IMAGE and stores it as the image of that number at FLOWER_IMAGE_BASE
.segment "FLOWER44"

.proc flower_dissolve_image
    jsr flower_dissolve_aim
    jsr flower_dissolve_inner_record
    jsr flower_dissolve_clear
    lda #1
    sta FLOWER_WRAPS
    sta FLOWER_DISSOLVE
    lda #0
    sta FLOWER_ROTATE
    sta FLOWER_SHIFT
    sta FLOWER_SHIFT + 1
    sta DS_SHAPE
@shape:
    ldx DS_SHAPE
    lda DS_STEP,x
    ora DS_STEP + 3,x
    beq @next_shape
    jsr flower_dissolve_draw_outer
    ldx DS_SHAPE
    lda DS_STEP,x
    ora DS_STEP + 3,x
    and #2
    beq @next_shape
    jsr flower_dissolve_draw_inner
@next_shape:
    inc DS_SHAPE
    lda DS_SHAPE
    cmp #3
    bcc @shape
    lda #0
    sta FLOWER_DISSOLVE
    jsr flower_dissolve_tile
    lda DS_IMAGE
    ldx #<early_start
    ldy #>early_start
    cmp #DISSOLVE_EARLY_IMAGES
    bcc @home
    sbc #DISSOLVE_EARLY_IMAGES
    ldx #<spin_half_tiles
    ldy #>spin_half_tiles
@home:
    stx FLOWER_IMAGE_BASE
    sty FLOWER_IMAGE_BASE + 1
    ldx #<FLOWER_TILE
    ldy #>FLOWER_TILE
    jmp flower_store_one
.endproc

; FLOWER_REC = the record of the image's turn (the images 0 and 1 have the turn 0) and DS_STEP = its 6 steps
.proc flower_dissolve_aim
    lda DS_IMAGE
    sec
    sbc #1
    bcs @turn
    lda #0
@turn:
    ldx #FLOWER_RECORD_BYTES
    stx FLOWER_MUL_A
    jsr flower_multiply
    clc
    lda FLOWER_N
    adc #<flower_dissolve_records
    sta FLOWER_REC
    lda FLOWER_N + 1
    adc #>flower_dissolve_records
    sta FLOWER_REC + 1
    lda #6
    sta FLOWER_MUL_A
    lda DS_IMAGE
    jsr flower_multiply
    clc
    lda FLOWER_N
    adc #<flower_dissolve_steps
    sta FLOWER_PTR
    lda FLOWER_N + 1
    adc #>flower_dissolve_steps
    sta FLOWER_PTR + 1
    ldy #5
@steps:
    lda (FLOWER_PTR),y
    sta DS_STEP,y
    dey
    bpl @steps
    rts
.endproc

; Adds the chords of the petals of the shape DS_SHAPE (the petals DS_SHAPE and DS_SHAPE + 3) that show something
.proc flower_dissolve_draw_outer
    lda DS_SHAPE
    sta FLOWER_SHAPE
    jsr flower_build_widths
    lda #0
    sta DS_POPBASE
    sta DS_RING_ONLY
    jmp flower_dissolve_petals
.endproc

; Takes away the chords of the smaller petals of the rings of the shape; FLOWER_REC is the record again afterwards
.proc flower_dissolve_draw_inner
    lda FLOWER_REC
    sta DS_OUTER
    lda FLOWER_REC + 1
    sta DS_OUTER + 1
    lda #<DS_INNER_REC
    sta FLOWER_REC
    lda #>DS_INNER_REC
    sta FLOWER_REC + 1
    lda DS_SHAPE
    sta FLOWER_SHAPE
    jsr flower_build_widths
    lda #4
    sta DS_POPBASE
    lda #2
    sta DS_RING_ONLY
    jsr flower_dissolve_petals
    lda DS_OUTER
    sta FLOWER_REC
    lda DS_OUTER + 1
    sta FLOWER_REC + 1
    rts
.endproc

; Draws the petal DS_SHAPE and then the petal DS_SHAPE + 3, each with the mask of its step, if it shows something (and,
; when DS_RING_ONLY is 2, a ring)
.proc flower_dissolve_petals
    lda DS_SHAPE
    sta DS_PETAL
@petal:
    ldx DS_PETAL
    lda DS_STEP,x
    beq @next
    and DS_RING_ONLY
    cmp DS_RING_ONLY
    bne @next
    jsr flower_dissolve_masks
    lda DS_PETAL
    sta FLOWER_PETAL
    jsr flower_aim_petal
    jsr flower_draw_petal_rows
@next:
    lda DS_PETAL
    cmp #3
    bcs @done
    clc
    adc #3
    sta DS_PETAL
    bne @petal
@done:
    rts
.endproc

; DS_MP = for each of the 4 rows of the mask of the step of the petal DS_PETAL (8 bytes a shade, 4 a phase) the subpixels the mask
; lets through in a pixel of an even column and in one of an odd column, as 2 bits (the left subpixel is bit 1)
.proc flower_dissolve_masks
    ldx DS_PETAL
    lda DS_STEP,x
    pha
    and #1
    asl
    asl
    sta DS_K
    pla
    lsr
    lsr
    asl
    asl
    asl
    ora DS_K
    tax
    ldy #0
@row:
    lda flower_square_masks,x
    pha
    lsr
    lsr
    lsr
    lsr
    lsr
    lsr
    sta DS_MP,y
    pla
    lsr
    lsr
    lsr
    lsr
    and #3
    sta DS_MP + 1,y
    inx
    iny
    iny
    cpy #8
    bcc @row
    rts
.endproc

; The record of the smaller petals: the half heights and widths of the shapes times 169 / 256, rounded; the shears and the
; offsets stay
.proc flower_dissolve_inner_record
    ldy #FLOWER_RECORD_BYTES - 1
@copy:
    lda (FLOWER_REC),y
    sta DS_INNER_REC,y
    dey
    bpl @copy
    ldy #FLOWER_HALF_OFFSET
@shape:
    sty DS_SAVED
    lda DS_INNER_REC,y
    jsr flower_dissolve_scale
    ldy DS_SAVED
    sta DS_INNER_REC,y
    lda DS_INNER_REC + 2,y
    jsr flower_dissolve_scale
    ldy DS_SAVED
    sta DS_INNER_REC + 2,y
    iny
    iny
    iny
    cpy #FLOWER_HALF_OFFSET + 9
    bcc @shape
    rts
.endproc

; A = (A * 169 + 128) / 256
.proc flower_dissolve_scale
    ldx #169
    stx FLOWER_MUL_A
    jsr flower_multiply
    clc
    lda FLOWER_N
    adc #128
    lda FLOWER_N + 1
    adc #0
    rts
.endproc

.proc flower_dissolve_clear
    lda #0
    tay
@byte:
    sta DISSOLVE_COUNTS,y
    sta DISSOLVE_COUNTS + 192,y
    sta DISSOLVE_COUNTS + 384,y
    iny
    cpy #192
    bne @byte
    rts
.endproc

; ─── Count the subpixels of the span FLOWER_FIRST - FLOWER_LAST of the sample row FLOWER_ROW_INDEX ─
; The mask lets through the subpixels whose column has its bit in the mask's row, which repeats every 4 columns; the
; row of the mask is the sample row's number modulo 4. The rows above the tile are not counted.
.segment "FLOWER45"

.proc flower_count_span
    lda FLOWER_ROW_INDEX
    sec
    sbc #2
    bcs @counted
    rts
@counted:
    sta DS_K
    and #3
    asl
    sta DS_ROW2
    lda DS_K
    lsr
    sta FLOWER_PTR2
    asl
    adc FLOWER_PTR2
    sta FLOWER_PTR2
    lda #0
    sta FLOWER_PTR2 + 1
    asl FLOWER_PTR2
    rol FLOWER_PTR2 + 1
    asl FLOWER_PTR2
    rol FLOWER_PTR2 + 1
    asl FLOWER_PTR2
    rol FLOWER_PTR2 + 1
    clc
    lda FLOWER_PTR2
    adc #<DISSOLVE_COUNTS
    sta FLOWER_PTR2
    lda FLOWER_PTR2 + 1
    adc #>DISSOLVE_COUNTS
    sta FLOWER_PTR2 + 1
    lda FLOWER_LAST
    lsr
    sta DS_PX1
    lda FLOWER_FIRST
    lsr
    tay
    cpy DS_PX1
    beq @single
    lda FLOWER_FIRST
    and #1
    beq @first_whole
    lda #1
    bne @first
@first_whole:
    lda #3
@first:
    jsr flower_count_pixel
    iny
    ldx DS_ROW2
    lda DS_MP,x
    clc
    adc DS_POPBASE
    tax
    lda flower_pop,x
    sta DS_V
    ldx DS_ROW2
    lda DS_MP + 1,x
    clc
    adc DS_POPBASE
    tax
    lda flower_pop,x
    sta DS_V + 1
@middle:
    cpy DS_PX1
    bcs @last
    tya
    and #1
    tax
    lda (FLOWER_PTR2),y
    clc
    adc DS_V,x
    sta (FLOWER_PTR2),y
    iny
    bne @middle
@last:
    lda FLOWER_LAST
    and #1
    beq @last_half
    lda #3
    bne @finish
@last_half:
    lda #2
@finish:
    jmp flower_count_pixel
@single:
    lda FLOWER_FIRST
    and #1
    beq @single_left
    lda #1
    bne @single_cut
@single_left:
    lda #3
@single_cut:
    sta DS_COV
    lda FLOWER_LAST
    and #1
    bne @single_ready
    lda DS_COV
    and #2
    sta DS_COV
@single_ready:
    lda DS_COV
    jmp flower_count_pixel
.endproc

; Adds to the pixel Y of the row at FLOWER_PTR2 the subpixels the mask lets through among the A (a pair as 2 bits) it covers
.proc flower_count_pixel
    sta DS_COV
    tya
    and #1
    ora DS_ROW2
    tax
    lda DS_MP,x
    and DS_COV
    clc
    adc DS_POPBASE
    tax
    lda flower_pop,x
    clc
    adc (FLOWER_PTR2),y
    sta (FLOWER_PTR2),y
    rts
.endproc

; The subpixels of a pair (0-3 of its bits set) as an added or subtracted count
flower_pop:
    .byte 0, 1, 1, 2, 0, $FF, $FF, $FE

; ─── FLOWER_TILE = the tile of the counts: light grey from 3 on, dark grey from 1 ──
; The tile holds a row as 3 bytes of the pixels that are black (bit 7 is the leftmost pixel) and 3 of those that are not light
; grey
.segment "FLOWER46"

.proc flower_dissolve_tile
    lda #<DISSOLVE_COUNTS
    sta FLOWER_PTR2
    lda #>DISSOLVE_COUNTS
    sta FLOWER_PTR2 + 1
    ldx #0
@row:
    ldy #0
@byte:
    lda #8
    sta DS_CNT
@pixel:
    lda (FLOWER_PTR2),y
    cmp #1
    rol DS_ANYB
    cmp #3
    rol DS_FULLB
    iny
    dec DS_CNT
    bne @pixel
    lda DS_ANYB
    eor #$FF
    sta FLOWER_TILE,x
    lda DS_FULLB
    eor #$FF
    sta FLOWER_TILE + 3,x
    inx
    cpy #24
    bcc @byte
    txa
    clc
    adc #3
    tax
    clc
    lda FLOWER_PTR2
    adc #24
    sta FLOWER_PTR2
    bcc @same_page
    inc FLOWER_PTR2 + 1
@same_page:
    cpx #TILE_BYTES
    bcc @row
    rts
.endproc
