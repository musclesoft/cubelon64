; cube_render.s - Renders one image of the rotating cube
;
; The corners of the cube on the screen come from cube_data.inc, 8 vertices in 1/8 pixel per state; the
; frame between two states is the mean of both. Every face the corners show counterclockwise (clockwise
; for the faces at -1) is drawn with slope edges into one span per row of the 48x48 subpixel grid,
; and the subpixels its pattern draws are added per pixel (see proto/cube_render.go, which this follows
; step by step). A pixel with 3 or more subpixels is light grey, 1 or 2 dark grey, none black. The image
; is only the rows the cube ever touches: 16, 18 and 16 bytes of the three char columns, where a set bit
; is a black pixel, and 18 rows of 3 bytes for the sprite, where a set bit is a pixel that is not light
; grey (proto/golden/cube_images.bin).

.include "include/shared_imports.inc"

CUBE_FACES = 6
CUBE_SUBPIXEL_ROWS = 48
CUBE_ROWS = 18
CUBE_SPRITE_OFFSET = 50
CUBE_COLUMN1_OFFSET = 16
CUBE_COLUMN2_OFFSET = 34

.segment "ZEROPAGE"

CUBE_OUT: .res 2
CUBE_ROW_PTR: .res 2
CUBE_PTR_A: .res 2
CUBE_FACE: .res 1
CUBE_ROW: .res 1
CUBE_ROW_MIN: .res 1
CUBE_ROW_MAX: .res 1
CR_X: .res 4
CR_Y: .res 4
C88_XL: .res 4
C88_XH: .res 4
C88_YL: .res 4
C88_YH: .res 4
FROM_BLOCK: .res 4
TO_BLOCK: .res 4
EDGE_X: .res 2
EDGE_SLOPE: .res 2
EDGE_DIST: .res 2
EDGE_HEIGHT: .res 2
EDGE_ADVANCE: .res 2
EDGE_LIMIT: .res 3
EDGE_REMAINDER: .res 2
EDGE_QUOTIENT: .res 2
EDGE_BITS: .res 1
EDGE_LEFT: .res 1
EDGE_START: .res 1
EDGE_END: .res 1
EDGE_INDEX: .res 1
CUBE_TEMP: .res 1
CUBE_PATTERN_BASE: .res 1
SPAN_START: .res 1
SPAN_END: .res 1
SPAN_FIRST: .res 1
SPAN_LAST: .res 1
SPAN_KIND: .res 1
SPAN_PARITY: .res 1
AREA_A: .res 1
AREA_B: .res 1
AREA_C: .res 1
AREA_D: .res 1
AREA_SIGN: .res 1
AREA_FIRST_SIGN: .res 1
PRODUCT_LOW: .res 1
PRODUCT_HIGH: .res 1
FIRST_LOW: .res 1
FIRST_HIGH: .res 1
MULTIPLIER: .res 1
MULTIPLICAND: .res 1
PACK_ROW: .res 1
PACK_SPRITE: .res 1
PACK_COLUMN: .res 1
PACK_CHARS: .res 1
PACK_LIGHT: .res 1

.segment "CUBEBSS"

.align 256
.export CUBE_COUNTS
CUBE_COUNTS: .res CUBE_ROWS * 24
CUBE_FOUND: .res CUBE_SUBPIXEL_ROWS
CUBE_X1: .res CUBE_SUBPIXEL_ROWS
CUBE_X2: .res CUBE_SUBPIXEL_ROWS
CUBE_VERTICES: .res 16
CUBE_STATE: .res 16
CUBE_DELTA: .res 16

.segment "CUBECODE"

.import cube_changes: absolute
.import cube_start_state: absolute

.export cube_render_init
.export cube_render_image

; ─── Clear the counts, once before the first image ───────────────────────────
.proc cube_render_init
    lda #0
    tax
@clear:
    sta CUBE_COUNTS,x
    sta CUBE_COUNTS + $100,x
    inx
    bne @clear
    rts
.endproc

; ─── Render the image of frame A (0-109 of the loop) to CUBE_OUT ─────────────
.proc cube_render_image
    pha
    and #1
    beq @pattern
    lda #12
@pattern:
    sta CUBE_PATTERN_BASE
    pla
    jsr load_vertices
    lda #0
    sta CUBE_FACE
@face:
    jsr face_seen
    bcc @next
    jsr draw_face
@next:
    inc CUBE_FACE
    lda CUBE_FACE
    cmp #CUBE_FACES
    bne @face
    jmp pack_image
.endproc

; ─── The corners of frame A: a state's, or the mean of two states' ───────────
; The states are walked in the order the build renders them: the state and its difference to the next
; start over for the first two images and advance with the changes of the difference on every even frame.
.proc load_vertices
    pha
    cmp #2
    bcs @running
    jsr start_state
    jmp @frame
@running:
    and #1
    bne @frame
    jsr advance_state
@frame:
    pla
    lsr
    ldy #15
    bcs @mean
@copy:
    lda CUBE_STATE,y
    sta CUBE_VERTICES,y
    dey
    bpl @copy
    rts
@mean:
    lda CUBE_DELTA,y
    clc
    adc #1
    cmp #$80
    ror
    clc
    adc CUBE_STATE,y
    sta CUBE_VERTICES,y
    dey
    bpl @mean
    rts
.endproc

.proc start_state
    ldx #31
@copy:
    lda cube_start_state,x
    sta CUBE_STATE,x
    dex
    bpl @copy
    lda #<cube_changes
    sta CUBE_PTR_A
    lda #>cube_changes
    sta CUBE_PTR_A + 1
    rts
.endproc

.proc advance_state
    ldx #15
@state:
    lda CUBE_STATE,x
    clc
    adc CUBE_DELTA,x
    sta CUBE_STATE,x
    dex
    bpl @state
    ldy #7
@pair:
    tya
    asl
    tax
    lda (CUBE_PTR_A),y
    pha
    and #$0F
    jsr change_delta
    pla
    lsr
    lsr
    lsr
    lsr
    inx
    jsr change_delta
    dey
    bpl @pair
    lda CUBE_PTR_A
    clc
    adc #8
    sta CUBE_PTR_A
    bcc @done
    inc CUBE_PTR_A + 1
@done:
    rts
.endproc

.proc change_delta
    clc
    adc CUBE_DELTA,x
    sec
    sbc #2
    sta CUBE_DELTA,x
    rts
.endproc

; ─── Load the corners of face CUBE_FACE ──────────────────────────────────────
.proc load_corners
    lda CUBE_FACE
    asl
    asl
    sta CUBE_TEMP
    ldx #0
@corner:
    ldy CUBE_TEMP
    lda face_vertices,y
    tay
    lda CUBE_VERTICES,y
    sta CR_X,x
    lda CUBE_VERTICES + 1,y
    sta CR_Y,x
    inc CUBE_TEMP
    inx
    cpx #4
    bne @corner
    rts
.endproc

; ─── Whether face CUBE_FACE is seen: the carry is set when it is ─────────────
; The corners run counterclockwise when the cross product of the diagonals is positive. The faces
; at +1 (odd faces) are seen then, the faces at -1 when it is negative.
.proc face_seen
    jsr load_corners
    lda CR_X + 2
    sta AREA_A
    lda CR_X
    sta AREA_B
    lda CR_Y + 3
    sta AREA_C
    lda CR_Y + 1
    sta AREA_D
    jsr signed_product
    lda AREA_SIGN
    sta AREA_FIRST_SIGN
    lda PRODUCT_LOW
    sta FIRST_LOW
    lda PRODUCT_HIGH
    sta FIRST_HIGH

    lda CR_Y + 2
    sta AREA_A
    lda CR_Y
    sta AREA_B
    lda CR_X + 3
    sta AREA_C
    lda CR_X + 1
    sta AREA_D
    jsr signed_product

    lda AREA_FIRST_SIGN
    bmi @first_negative
    lda AREA_SIGN
    bmi @greater
    jsr compare_magnitudes
    jmp @compared
@first_negative:
    lda AREA_SIGN
    bpl @less
    jsr compare_magnitudes
    eor #$FF
    clc
    adc #1
    jmp @compared
@greater:
    lda #1
    jmp @compared
@less:
    lda #$FF
@compared:
    ldx CUBE_FACE
    pha
    txa
    lsr
    pla
    bcc @minus
    cmp #1
    beq @seen
    clc
    rts
@minus:
    cmp #$FF
    beq @seen
    clc
    rts
@seen:
    sec
    rts
.endproc

; ─── The first product against the second: A = 1 greater, 0 equal, $FF less ──
.proc compare_magnitudes
    lda FIRST_HIGH
    cmp PRODUCT_HIGH
    bne @decide
    lda FIRST_LOW
    cmp PRODUCT_LOW
    beq @equal
@decide:
    bcc @less
    lda #1
    rts
@less:
    lda #$FF
    rts
@equal:
    lda #0
    rts
.endproc

; ─── (A - B) * (C - D) as a sign (bit 7 set: negative, never for zero) and a magnitude ─
.proc signed_product
    lda #0
    sta AREA_SIGN
    lda AREA_A
    sec
    sbc AREA_B
    bcs @first
    eor #$FF
    adc #1
    pha
    lda AREA_SIGN
    eor #$80
    sta AREA_SIGN
    pla
@first:
    sta MULTIPLICAND
    lda AREA_C
    sec
    sbc AREA_D
    bcs @second
    eor #$FF
    adc #1
    pha
    lda AREA_SIGN
    eor #$80
    sta AREA_SIGN
    pla
@second:
    sta MULTIPLIER
    jsr multiply
    sta PRODUCT_HIGH
    lda MULTIPLIER
    sta PRODUCT_LOW
    ora PRODUCT_HIGH
    bne @done
    sta AREA_SIGN
@done:
    rts
.endproc

; ─── MULTIPLICAND * MULTIPLIER: A = high byte, MULTIPLIER = low byte ─────────
.proc multiply
    lda #0
    ldx #8
    lsr MULTIPLIER
@bit:
    bcc @shift
    clc
    adc MULTIPLICAND
@shift:
    ror
    ror MULTIPLIER
    dex
    bne @bit
    rts
.endproc

; ─── Draw face CUBE_FACE: the spans of its four edges, then its pattern ──────
.proc draw_face
    ldx #3
@convert:
    lda CR_X,x
    lsr
    lsr
    sta C88_XH,x
    lda CR_X,x
    and #3
    lsr
    ror
    ror
    sta C88_XL,x
    lda CR_Y,x
    sec
    sbc #2
    bcs @above
    lda #0
@above:
    sta CUBE_TEMP
    lsr
    lsr
    sta C88_YH,x
    lda CUBE_TEMP
    and #3
    lsr
    ror
    ror
    sta C88_YL,x
    dex
    bpl @convert

    lda #CUBE_SUBPIXEL_ROWS
    sta CUBE_ROW_MIN
    lda #0
    sta CUBE_ROW_MAX
    sta EDGE_INDEX
@edge:
    jsr step_edge
    inc EDGE_INDEX
    lda EDGE_INDEX
    cmp #4
    bne @edge
    jmp draw_spans
.endproc

; ─── Walk edge EDGE_INDEX (from its corner to the next) down the rows ────────
; Its x on each row it crosses, rounded to a subpixel column, becomes the first or second end of the
; row's span.
.proc step_edge
    ldx EDGE_INDEX
    lda C88_XL,x
    sta FROM_BLOCK
    lda C88_XH,x
    sta FROM_BLOCK + 1
    lda C88_YL,x
    sta FROM_BLOCK + 2
    lda C88_YH,x
    sta FROM_BLOCK + 3
    inx
    txa
    and #3
    tax
    lda C88_XL,x
    sta TO_BLOCK
    lda C88_XH,x
    sta TO_BLOCK + 1
    lda C88_YL,x
    sta TO_BLOCK + 2
    lda C88_YH,x
    sta TO_BLOCK + 3

    lda TO_BLOCK + 2
    cmp FROM_BLOCK + 2
    lda TO_BLOCK + 3
    sbc FROM_BLOCK + 3
    bcs @ordered
    ldx #3
@swap:
    lda FROM_BLOCK,x
    sta CUBE_TEMP
    lda TO_BLOCK,x
    sta FROM_BLOCK,x
    lda CUBE_TEMP
    sta TO_BLOCK,x
    dex
    bpl @swap

@ordered:
    lda FROM_BLOCK + 3
    ldx FROM_BLOCK + 2
    beq @start_ready
    clc
    adc #1
@start_ready:
    sta EDGE_START
    lda TO_BLOCK + 3
    ldx TO_BLOCK + 2
    beq @end_ready
    clc
    adc #1
@end_ready:
    cmp #CUBE_SUBPIXEL_ROWS + 1
    bcc @end_clamped
    lda #CUBE_SUBPIXEL_ROWS
@end_clamped:
    sta EDGE_END
    lda EDGE_START
    cmp EDGE_END
    bcc @rows
    rts

@rows:
    cmp CUBE_ROW_MIN
    bcs @min_kept
    sta CUBE_ROW_MIN
@min_kept:
    lda EDGE_END
    cmp CUBE_ROW_MAX
    bcc @max_kept
    sta CUBE_ROW_MAX
@max_kept:

    lda TO_BLOCK
    sec
    sbc FROM_BLOCK
    sta EDGE_DIST
    lda TO_BLOCK + 1
    sbc FROM_BLOCK + 1
    sta EDGE_DIST + 1
    bcs @right
    lda #0
    sec
    sbc EDGE_DIST
    sta EDGE_DIST
    lda #0
    sbc EDGE_DIST + 1
    sta EDGE_DIST + 1
    lda #$80
    bne @direction
@right:
    lda #0
@direction:
    sta EDGE_LEFT
    lda TO_BLOCK + 2
    sec
    sbc FROM_BLOCK + 2
    sta EDGE_HEIGHT
    lda TO_BLOCK + 3
    sbc FROM_BLOCK + 3
    sta EDGE_HEIGHT + 1

    lda #0
    sta EDGE_SLOPE
    sta EDGE_SLOPE + 1
    lda EDGE_DIST
    ora EDGE_DIST + 1
    beq @slope_ready

    lda EDGE_HEIGHT + 1
    lsr
    sta EDGE_LIMIT + 2
    lda EDGE_HEIGHT
    ror
    sta EDGE_LIMIT + 1
    lda #0
    ror
    sta EDGE_LIMIT
    lda EDGE_DIST
    cmp EDGE_LIMIT
    lda EDGE_DIST + 1
    sbc EDGE_LIMIT + 1
    lda #0
    sbc EDGE_LIMIT + 2
    bcc @divide
    lda #$FF
    sta EDGE_SLOPE
    lda #$7F
    sta EDGE_SLOPE + 1
    jmp @slope_ready

@divide:
    lda EDGE_DIST
    asl
    sta EDGE_BITS
    lda EDGE_DIST + 1
    rol
    sta EDGE_REMAINDER
    lda #0
    sta EDGE_REMAINDER + 1
    sta EDGE_QUOTIENT
    sta EDGE_QUOTIENT + 1
    ldx #15
@quotient_bit:
    asl EDGE_BITS
    rol EDGE_REMAINDER
    rol EDGE_REMAINDER + 1
    lda EDGE_REMAINDER
    sec
    sbc EDGE_HEIGHT
    tay
    lda EDGE_REMAINDER + 1
    sbc EDGE_HEIGHT + 1
    bcc @remainder_kept
    sta EDGE_REMAINDER + 1
    sty EDGE_REMAINDER
@remainder_kept:
    rol EDGE_QUOTIENT
    rol EDGE_QUOTIENT + 1
    dex
    bne @quotient_bit
    lda EDGE_QUOTIENT
    sta EDGE_SLOPE
    lda EDGE_QUOTIENT + 1
    sta EDGE_SLOPE + 1

@slope_ready:
    lda #0
    sta EDGE_ADVANCE
    sta EDGE_ADVANCE + 1
    lda FROM_BLOCK + 2
    beq @advance_ready
    cmp #128
    beq @half
    bcs @quarter
    lda EDGE_SLOPE
    clc
    adc #3
    sta EDGE_ADVANCE
    lda EDGE_SLOPE + 1
    adc #0
    lsr
    ror EDGE_ADVANCE
    lsr
    ror EDGE_ADVANCE
    sta EDGE_ADVANCE + 1
    lda EDGE_SLOPE
    sec
    sbc EDGE_ADVANCE
    sta EDGE_ADVANCE
    lda EDGE_SLOPE + 1
    sbc EDGE_ADVANCE + 1
    sta EDGE_ADVANCE + 1
    jmp @advance_ready
@half:
    lda EDGE_SLOPE + 1
    lsr
    sta EDGE_ADVANCE + 1
    lda EDGE_SLOPE
    ror
    sta EDGE_ADVANCE
    jmp @advance_ready
@quarter:
    lda EDGE_SLOPE + 1
    lsr
    sta EDGE_ADVANCE + 1
    lda EDGE_SLOPE
    ror
    lsr EDGE_ADVANCE + 1
    ror
    sta EDGE_ADVANCE
@advance_ready:

    lda EDGE_LEFT
    bmi @leftwards
    lda FROM_BLOCK
    clc
    adc EDGE_ADVANCE
    sta EDGE_X
    lda FROM_BLOCK + 1
    adc EDGE_ADVANCE + 1
    sta EDGE_X + 1
    jmp @walk
@leftwards:
    lda FROM_BLOCK
    sec
    sbc EDGE_ADVANCE
    sta EDGE_X
    lda FROM_BLOCK + 1
    sbc EDGE_ADVANCE + 1
    sta EDGE_X + 1
    lda #0
    sec
    sbc EDGE_SLOPE
    sta EDGE_SLOPE
    lda #0
    sbc EDGE_SLOPE + 1
    sta EDGE_SLOPE + 1

@walk:
    ldx EDGE_START
@row:
    lda EDGE_X
    clc
    adc #128
    lda EDGE_X + 1
    adc #0
    ldy CUBE_FOUND,x
    bne @second
    sta CUBE_X1,x
    lda #1
    bne @found
@second:
    sta CUBE_X2,x
    lda #2
@found:
    sta CUBE_FOUND,x
    lda EDGE_X
    clc
    adc EDGE_SLOPE
    sta EDGE_X
    lda EDGE_X + 1
    adc EDGE_SLOPE + 1
    sta EDGE_X + 1
    inx
    cpx EDGE_END
    bne @row
    rts
.endproc

; ─── Add the pattern of the face to the counts of every row with a span ──────
.proc draw_spans
    ldx CUBE_ROW_MIN
    cpx CUBE_ROW_MAX
    bcs @none
@row:
    ldy CUBE_FOUND,x
    lda #0
    sta CUBE_FOUND,x
    cpy #2
    bne @next
    lda CUBE_X1,x
    sta SPAN_START
    lda CUBE_X2,x
    sta SPAN_END
    cmp SPAN_START
    bcs @ordered
    ldy SPAN_START
    sta SPAN_START
    sty SPAN_END
@ordered:
    lda SPAN_END
    cmp #CUBE_SUBPIXEL_ROWS + 1
    bcc @clamped
    lda #CUBE_SUBPIXEL_ROWS
    sta SPAN_END
@clamped:
    lda SPAN_START
    cmp SPAN_END
    bcs @next
    stx CUBE_ROW
    jsr add_span
    ldx CUBE_ROW
@next:
    inx
    cpx CUBE_ROW_MAX
    bne @row
@none:
    rts
.endproc

; ─── Add what the pattern draws in subpixel row CUBE_ROW from SPAN_START to SPAN_END ─
; Per pixel, the two columns of each of its two rows: the solid rows draw both, the others one
; column of every second pixel, which kind of pixels depends on the row (see span_patterns).
.proc add_span
    lda CUBE_ROW
    lsr
    tax
    lda pixel_row_low,x
    sta CUBE_ROW_PTR
    lda pixel_row_high,x
    clc
    adc #>CUBE_COUNTS
    sta CUBE_ROW_PTR + 1

    lda CUBE_FACE
    lsr
    asl
    asl
    sta CUBE_TEMP
    lda CUBE_ROW
    and #3
    ora CUBE_TEMP
    clc
    adc CUBE_PATTERN_BASE
    tax
    lda span_patterns,x
    sta SPAN_KIND
    beq @left
    sec
    sbc #1
    and #1
    sta SPAN_PARITY

@left:
    lda SPAN_START
    lsr
    bcc @right
    tay
    lda SPAN_KIND
    beq @left_add
    cmp #3
    bcs @right
    tya
    eor SPAN_KIND
    and #1
    beq @right
@left_add:
    lda (CUBE_ROW_PTR),y
    clc
    adc #1
    sta (CUBE_ROW_PTR),y

@right:
    lda SPAN_END
    lsr
    bcc @interior
    tay
    lda SPAN_KIND
    beq @right_add
    cmp #3
    bcc @interior
    tya
    eor SPAN_KIND
    and #1
    beq @interior
@right_add:
    lda (CUBE_ROW_PTR),y
    clc
    adc #1
    sta (CUBE_ROW_PTR),y

@interior:
    lda SPAN_START
    clc
    adc #1
    lsr
    sta SPAN_FIRST
    lda SPAN_END
    lsr
    sta SPAN_LAST
    lda SPAN_FIRST
    cmp SPAN_LAST
    bcs @done
    ldy SPAN_FIRST
    lda SPAN_KIND
    bne @alternating
@solid:
    lda (CUBE_ROW_PTR),y
    clc
    adc #2
    sta (CUBE_ROW_PTR),y
    iny
    cpy SPAN_LAST
    bne @solid
@done:
    rts
@alternating:
    tya
    eor SPAN_PARITY
    and #1
    beq @aligned
    iny
@aligned:
    cpy SPAN_LAST
    bcs @done
    lda (CUBE_ROW_PTR),y
    clc
    adc #1
    sta (CUBE_ROW_PTR),y
    iny
    iny
    jmp @aligned
.endproc

; ─── Turn the counts into the image at CUBE_OUT and clear them ───────────────
.proc pack_image
    lda #<CUBE_COUNTS
    sta CUBE_ROW_PTR
    lda #>CUBE_COUNTS
    sta CUBE_ROW_PTR + 1
    lda #0
    sta PACK_ROW
    lda #CUBE_SPRITE_OFFSET
    sta PACK_SPRITE
@row:
    ldy #0
    jsr classify_byte
    sty PACK_COLUMN
    lda PACK_ROW
    beq @skip_column0
    cmp #CUBE_ROWS - 1
    beq @skip_column0
    lda PACK_CHARS
    eor #$FF
    ldy PACK_ROW
    dey
    sta (CUBE_OUT),y
@skip_column0:
    lda PACK_LIGHT
    eor #$FF
    ldy PACK_SPRITE
    sta (CUBE_OUT),y
    ldy PACK_COLUMN

    jsr classify_byte
    sty PACK_COLUMN
    lda PACK_CHARS
    eor #$FF
    pha
    lda PACK_ROW
    clc
    adc #CUBE_COLUMN1_OFFSET
    tay
    pla
    sta (CUBE_OUT),y
    lda PACK_LIGHT
    eor #$FF
    ldy PACK_SPRITE
    iny
    sta (CUBE_OUT),y
    ldy PACK_COLUMN

    jsr classify_byte
    lda PACK_ROW
    beq @skip_column2
    cmp #CUBE_ROWS - 1
    beq @skip_column2
    lda PACK_CHARS
    eor #$FF
    pha
    lda PACK_ROW
    clc
    adc #CUBE_COLUMN2_OFFSET - 1
    tay
    pla
    sta (CUBE_OUT),y
@skip_column2:
    lda PACK_LIGHT
    eor #$FF
    ldy PACK_SPRITE
    iny
    iny
    sta (CUBE_OUT),y

    lda PACK_SPRITE
    clc
    adc #3
    sta PACK_SPRITE
    lda CUBE_ROW_PTR
    clc
    adc #24
    sta CUBE_ROW_PTR
    bcc @same_page
    inc CUBE_ROW_PTR + 1
@same_page:
    inc PACK_ROW
    lda PACK_ROW
    cmp #CUBE_ROWS
    beq @clear
    jmp @row

@clear:
    jmp cube_render_init
.endproc

; ─── Classify the 8 pixels at CUBE_ROW_PTR + Y ───────────────────────────────
; PACK_CHARS gets a bit for each pixel that is not black, PACK_LIGHT one for each that is light grey, the
; first pixel in bit 7. Y moves on by 8.
.proc classify_byte
    lda #0
    sta PACK_CHARS
    sta PACK_LIGHT
    ldx #8
@pixel:
    lda (CUBE_ROW_PTR),y
    iny
    cmp #1
    rol PACK_CHARS
    cmp #3
    rol PACK_LIGHT
    dex
    bne @pixel
    rts
.endproc

; Where each row of the counts starts: the 18 pixel rows from the 4th, of 24 counts (the others are never drawn)
pixel_row_low:
    .byte 0, 0, 0, 0, 24, 48, 72, 96, 120, 144, 168, 192, 216, 240, 8, 32, 56, 80, 104, 128, 152, 0, 0, 0
pixel_row_high:
    .byte 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0

; The corners of each face, as byte offsets into the vertices: faces at -1 and +1 of the x, y and z axes
face_vertices:
    .byte 0, 4, 12, 8
    .byte 2, 6, 14, 10
    .byte 0, 8, 10, 2
    .byte 4, 12, 14, 6
    .byte 0, 2, 6, 4
    .byte 8, 10, 14, 12

; What the pattern draws in a row, by axis (dots, checker, solid) and the row's number modulo 4:
; 0 both subpixels of every pixel, 1 and 2 the odd subpixel of the even and the odd pixels,
; 3 and 4 the even subpixel of the even and the odd pixels
span_patterns:
    .byte 1, 4, 2, 3
    .byte 1, 0, 2, 0
    .byte 0, 0, 0, 0
    .byte 2, 3, 1, 4
    .byte 2, 0, 1, 0
    .byte 0, 0, 0, 0
