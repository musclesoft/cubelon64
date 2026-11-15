; flower.s - The square turns into a flower of six petals
;
; The last square of the marching cubes becomes the sunflower the marching flowers start from, in 18 states at
; 50 Hz (demo frames 3360-3377; the ZX demo shows the even ones). proto fits sweeps to the ZX frames and
; turns each state into a record of 18 bytes (go run -C proto . flower, flower_data.inc): the square's half size,
; centre row and shade, three petal shapes (half height, shear, width) and the offset of one petal of each
; shape; the other three petals lie opposite. Lengths are in 1/16 pixel.
;
; The renderer draws a record in subpixels: 48 sample rows of 48 sample columns, one bit each. A petal is an
; ellipse, drawn as chords: for each sample row the chord's centre moves by the shear, its half width is
; the half circle (flower_circle) times the shape's width, and the samples within it are set. The records and the
; routines are the ones the marching flowers need: they only add the shift of the whole flower
; (FLOWER_SHIFT, in 1/16 pixel), which wraps the petals around the tile. The petals and the solid square merge
; before the grey level of a pixel is chosen, the square fading from light grey to black in the shade 4-0
; with a checkerboard in between. The tile of the even rows of cells is packed like the other tiles (see tile.inc),
; the tile of the odd rows is its flip about row 11. proto/golden/flower_images.bin checks the images: the
; even and odd image of each state, state N at image 2 * N - 2 and 2 * N - 1 of FLOWER_IMAGES.
;
; Memory: the code lies where the VIC bank has room (c64.cfg): the screens B and D, which only hold sprite pointers
; (FLOWERCODE, FLOWERHELP), the free page at $6900 (FLOWERMORE) and RAM2 (FLOWERTAIL). The images lie on
; those of the spin, which are over after the diamonds, with the renderer's work arrays behind them, and the
; variables behind the rotating cubes' images.

.include "include/shared_imports.inc"
.include "include/tile.inc"

FLOWER_LAST_STATE = 18
FLOWER_IMAGES = SPIN_IMAGES
FLOWER_BASE = FLOWER_IMAGES - 2 * IMAGE_BYTES
FLOWER_RECORD_BYTES = 18
FLOWER_SUB_ROWS = 48
FLOWER_ROW_WIDTH_BYTES = 64
FLOWER_CENTRE_X = 192
FLOWER_CENTRE_Y = 184
FLOWER_HALF_OFFSET = 3
FLOWER_OFFSETS = 12
FLOWER_SQUARE_HALF = 0
FLOWER_SQUARE_Y = 1
FLOWER_SHADE_BYTE = 2
FLOWER_ODD_FLIP = 22
FLOWER_WORK = FLOWER_IMAGES + 2 * FLOWER_LAST_STATE * IMAGE_BYTES
FLOWER_MASK = FLOWER_WORK
FLOWER_MASK_ROWS = FLOWER_SUB_ROWS + 2
FLOWER_FIRST_TILE_ROW = FLOWER_MASK + 2 * 6
FLOWER_TURNED_ROW = FLOWER_MASK + 46 * 6
FLOWER_TILE = FLOWER_MASK + FLOWER_MASK_ROWS * 6
FLOWER_ODD_TILE = FLOWER_TILE + TILE_BYTES
FLOWER_ROW_WIDTHS = FLOWER_ODD_TILE + TILE_BYTES
FLOWER_W16 = FLOWER_ROW_WIDTHS + 3 * FLOWER_ROW_WIDTH_BYTES
FLOWER_SQUARE = FLOWER_W16 + 16

.segment "ZEROPAGE"

FLOWER_REC: .res 2
FLOWER_PTR: .res 2
FLOWER_PTR2: .res 2
FLOWER_P0: .res 1
FLOWER_P1: .res 1
FLOWER_Q0: .res 1
FLOWER_Q1: .res 1

; The variables lie after the rotating cubes' 3536 bytes of images, which move onto the split's images at frame 2700
FLOWER_VARS = SPLIT_IMAGES + 3584

FLOWER_STATE = FLOWER_VARS + 0
FLOWER_PHASE = FLOWER_VARS + 1
FLOWER_SHIFT = FLOWER_VARS + 2
FLOWER_WRAPS = FLOWER_VARS + 4
FLOWER_ROTATE = FLOWER_VARS + 5
FLOWER_PETAL_LIMIT = FLOWER_VARS + 6
FLOWER_SHAPE = FLOWER_VARS + 7
FLOWER_HALF = FLOWER_VARS + 8
FLOWER_WIDTH = FLOWER_VARS + 9
FLOWER_SHEAR = FLOWER_VARS + 10
FLOWER_ROW_BASE = FLOWER_VARS + 11
FLOWER_IDX = FLOWER_VARS + 12
FLOWER_REM = FLOWER_VARS + 13
FLOWER_MUL_A = FLOWER_VARS + 14
FLOWER_N = FLOWER_VARS + 15
FLOWER_PETAL = FLOWER_VARS + 17
FLOWER_NEG = FLOWER_VARS + 18
FLOWER_VAL = FLOWER_VARS + 19
FLOWER_CX = FLOWER_VARS + 21
FLOWER_CY = FLOWER_VARS + 23
FLOWER_DY = FLOWER_VARS + 25
FLOWER_AD = FLOWER_VARS + 27
FLOWER_ACC = FLOWER_VARS + 29
FLOWER_STEP = FLOWER_VARS + 31
FLOWER_ROW_FIRST = FLOWER_VARS + 33
FLOWER_ROW_COUNT = FLOWER_VARS + 34
FLOWER_CENTRE = FLOWER_VARS + 35
FLOWER_WRAP_INDEX = FLOWER_VARS + 37
FLOWER_WD = FLOWER_VARS + 38
FLOWER_SPAN_C = FLOWER_VARS + 39
FLOWER_T = FLOWER_VARS + 41
FLOWER_FIRST = FLOWER_VARS + 43
FLOWER_LAST = FLOWER_VARS + 44
FLOWER_BYTE_FIRST = FLOWER_VARS + 45
FLOWER_BYTE_LAST = FLOWER_VARS + 46
FLOWER_BITS = FLOWER_VARS + 47
FLOWER_SQ_DY = FLOWER_VARS + 48
FLOWER_SQ_ACTIVE0 = FLOWER_VARS + 50
FLOWER_SQ_ACTIVE1 = FLOWER_VARS + 51
FLOWER_PIXEL_ROW = FLOWER_VARS + 52
FLOWER_BYTE = FLOWER_VARS + 53
FLOWER_LIT = FLOWER_VARS + 54
FLOWER_ANY = FLOWER_VARS + 55
FLOWER_ALL = FLOWER_VARS + 56
FLOWER_X = FLOWER_VARS + 57
FLOWER_Y = FLOWER_VARS + 58
FLOWER_XO = FLOWER_VARS + 59
FLOWER_XA = FLOWER_VARS + 60
FLOWER_YO = FLOWER_VARS + 61
FLOWER_YA = FLOWER_VARS + 62
FLOWER_AN = FLOWER_VARS + 63
FLOWER_AL = FLOWER_VARS + 64
FLOWER_ROW_INDEX = FLOWER_VARS + 69
FLOWER_USED = FLOWER_VARS + 70
FLOWER_STRIDE = FLOWER_VARS + 70 + FLOWER_MASK_ROWS
FLOWER_FULL = FLOWER_VARS + 65
FLOWER_MEDIUM = FLOWER_VARS + 66
FLOWER_ROW_OFFSET = FLOWER_VARS + 67
FLOWER_SHADE = FLOWER_VARS + 68


.segment "FLOWERCODE"

.import store_image: absolute
.import show_state_pair: absolute
.import job_checkpoint: absolute
.import TILE_SRC_PTR: absolute
.import flower_circle: absolute
.import flower_records: absolute
.import BIT_REVERSE_TABLE: absolute

.export flower_precompute
.export flower_step
.export flower_render
.export flower_flip
.export FLOWER_REC
.export FLOWER_SHIFT
.export FLOWER_PHASE

; ─── Build the images of all flower states ───────────────────────────────────
.proc flower_precompute
    lda #<flower_records
    sta FLOWER_REC
    lda #>flower_records
    sta FLOWER_REC + 1
    lda #0
    sta FLOWER_SHIFT
    sta FLOWER_SHIFT + 1
    lda #1
    sta FLOWER_STATE
@state:
    and #1
    sta FLOWER_PHASE
    jsr flower_render
    lda #<FLOWER_TILE
    sta TILE_SRC_PTR
    lda #>FLOWER_TILE
    sta TILE_SRC_PTR + 1
    lda FLOWER_STATE
    asl
    ldx #<FLOWER_BASE
    ldy #>FLOWER_BASE
    jsr store_image
    jsr flower_flip
    lda #<FLOWER_ODD_TILE
    sta TILE_SRC_PTR
    lda #>FLOWER_ODD_TILE
    sta TILE_SRC_PTR + 1
    lda FLOWER_STATE
    asl
    ora #1
    ldx #<FLOWER_BASE
    ldy #>FLOWER_BASE
    jsr store_image
    clc
    lda FLOWER_REC
    adc #FLOWER_RECORD_BYTES
    sta FLOWER_REC
    bcc @next
    inc FLOWER_REC + 1
@next:
    inc FLOWER_STATE
    lda FLOWER_STATE
    cmp #FLOWER_LAST_STATE + 1
    bcc @state
    assert_if FLOWER_STATE, 19, FLOWER_IMAGES, "../proto/golden/flower_images.bin", 5184
    rts
.endproc

; ─── Show the pair of the state SEQ_SCENE_STATE; the flower holds its last state ─
.proc flower_step
    lda SEQ_SCENE_STATE
    cmp #FLOWER_LAST_STATE + 1
    bcs @held
    clc
    ldx #<FLOWER_BASE
    ldy #>FLOWER_BASE
    jmp show_state_pair
@held:
    rts
.endproc

; ─── Render the record at FLOWER_REC, shifted by FLOWER_SHIFT, into FLOWER_TILE ─
; FLOWER_PHASE is the parity of the state, which moves the checkerboard of the fading square.
.proc flower_render
    jsr flower_clear_masks
    jsr flower_build_all_widths
    jsr flower_draw_petals
    jsr flower_draw_square
    jmp flower_pack_tile
.endproc

.proc flower_clear_masks
    lda #<FLOWER_MASK
    sta FLOWER_PTR
    lda #>FLOWER_MASK
    sta FLOWER_PTR + 1
    ldx #5
@chunk:
    jsr job_checkpoint
    lda #0
    tay
@byte:
    sta (FLOWER_PTR),y
    iny
    cpy #64
    bne @byte
    clc
    lda FLOWER_PTR
    adc #64
    sta FLOWER_PTR
    bcc @same_page
    inc FLOWER_PTR + 1
@same_page:
    dex
    bne @chunk
    ldx #5
    lda #0
@square:
    sta FLOWER_SQUARE,x
    dex
    bpl @square
    ldx #FLOWER_MASK_ROWS - 1
@used:
    sta FLOWER_USED,x
    dex
    bpl @used
    rts
.endproc

; ─── The width of the chord at each distance from the centre, for the three shapes ─
.proc flower_build_all_widths
    lda #0
    sta FLOWER_SHAPE
@shape:
    jsr flower_build_widths
    inc FLOWER_SHAPE
    lda FLOWER_SHAPE
    cmp #3
    bcc @shape
    rts
.endproc

; The sixteen widths of the half circle times the shape's width, rounded: (width * circle + 127) / 255,
; then one for each distance below the half height, the distance times 16 divided by the half height
; picking the width
.proc flower_build_widths
    jsr flower_load_shape
    ldx #15
@circle:
    txa
    and #3
    bne @same_group
    jsr job_checkpoint
@same_group:
    lda FLOWER_WIDTH
    sta FLOWER_MUL_A
    lda flower_circle,x
    jsr flower_multiply
    clc
    lda FLOWER_N
    adc #127
    sta FLOWER_N
    bcc @rounded
    inc FLOWER_N + 1
@rounded:
    sec
    lda FLOWER_N
    adc FLOWER_N + 1
    lda FLOWER_N + 1
    adc #0
    sta FLOWER_W16,x
    dex
    bpl @circle

    lda FLOWER_HALF
    beq @done
    ldx FLOWER_SHAPE
    ldy flower_row_base,x
    lda #0
    sta FLOWER_IDX
    sta FLOWER_REM
    lda #4
    sta FLOWER_STRIDE
@distance:
    ldx FLOWER_IDX
    lda FLOWER_W16,x
    sta FLOWER_ROW_WIDTHS,y
    iny
    clc
    lda FLOWER_REM
    adc #16
@reduce:
    cmp FLOWER_HALF
    bcc @remainder
    sbc FLOWER_HALF
    inc FLOWER_IDX
    dec FLOWER_STRIDE
    bne @reduce
    ldx #4
    stx FLOWER_STRIDE
    jsr job_checkpoint
    jmp @reduce
@remainder:
    sta FLOWER_REM
    tya
    ldx FLOWER_SHAPE
    sec
    sbc flower_row_base,x
    cmp FLOWER_HALF
    bcc @distance
@done:
    rts
.endproc

; Loads the half height, width and shear of shape FLOWER_SHAPE and its first row in FLOWER_ROW_WIDTHS
.proc flower_load_shape
    ldx FLOWER_SHAPE
    lda flower_row_base,x
    sta FLOWER_ROW_BASE
    txa
    asl
    clc
    adc FLOWER_SHAPE
    clc
    adc #FLOWER_HALF_OFFSET
    tay
    lda (FLOWER_REC),y
    sta FLOWER_HALF
    iny
    lda (FLOWER_REC),y
    sta FLOWER_SHEAR
    iny
    lda (FLOWER_REC),y
    sta FLOWER_WIDTH
    rts
.endproc

; ─── FLOWER_N = A * FLOWER_MUL_A ─────────────────────────────────────────────
; Modifies: A, Y
.proc flower_multiply
    sta FLOWER_N
    lda #0
    ldy #8
    lsr FLOWER_N
@bit:
    bcc @shift
    clc
    adc FLOWER_MUL_A
@shift:
    ror
    ror FLOWER_N
    dey
    bne @bit
    sta FLOWER_N + 1
    rts
.endproc

; ─── Draw the petals into FLOWER_MASK ────────────────────────────────────────
; Without a shift the second three petals are the first three turned by half a turn about the centre of
; the flower, which the pixel pass adds (see flower_load_byte_rows); with a shift all six are drawn.
; The mask has two rows above the tile, which the turned rows 46 and 47 come from.
.proc flower_draw_petals
    lda #1
    sta FLOWER_WRAPS
    sta FLOWER_ROTATE
    lda #3
    sta FLOWER_PETAL_LIMIT
    lda FLOWER_SHIFT
    ora FLOWER_SHIFT + 1
    beq @petals_ready
    lda #3
    sta FLOWER_WRAPS
    lda #0
    sta FLOWER_ROTATE
    lda #6
    sta FLOWER_PETAL_LIMIT
@petals_ready:
    lda #0
    sta FLOWER_PETAL
@petal:
    jsr job_checkpoint
    jsr flower_aim_petal
    jsr flower_draw_petal_rows
    inc FLOWER_PETAL
    lda FLOWER_PETAL
    cmp FLOWER_PETAL_LIMIT
    bcc @petal
    rts
.endproc

; Loads the shape of the petal and its centre: the offset of the first petal of the shape, negated for the opposite one
.proc flower_aim_petal
    lda FLOWER_PETAL
    ldx #0
    cmp #3
    bcc @shape_ready
    sbc #3
    ldx #1
@shape_ready:
    stx FLOWER_NEG
    sta FLOWER_SHAPE
    jsr flower_load_shape

    lda FLOWER_SHAPE
    asl
    clc
    adc #FLOWER_OFFSETS
    tay
    lda (FLOWER_REC),y
    jsr flower_signed_offset
    clc
    lda FLOWER_VAL
    adc FLOWER_SHIFT
    sta FLOWER_CX
    lda FLOWER_VAL + 1
    adc FLOWER_SHIFT + 1
    sta FLOWER_CX + 1
    clc
    lda FLOWER_CX
    adc #<FLOWER_CENTRE_X
    sta FLOWER_CX
    lda FLOWER_CX + 1
    adc #>FLOWER_CENTRE_X
    sta FLOWER_CX + 1

    iny
    lda (FLOWER_REC),y
    jsr flower_signed_offset
    clc
    lda FLOWER_VAL
    adc #<FLOWER_CENTRE_Y
    sta FLOWER_CY
    lda FLOWER_VAL + 1
    adc #>FLOWER_CENTRE_Y
    sta FLOWER_CY + 1
    rts
.endproc

; FLOWER_VAL = the signed byte A as a word, negated when FLOWER_NEG is set. Keeps Y
.proc flower_signed_offset
    sta FLOWER_VAL
    ldx #0
    cmp #$80
    bcc @extended
    ldx #$FF
@extended:
    stx FLOWER_VAL + 1
    lda FLOWER_NEG
    beq @done
    sec
    lda #0
    sbc FLOWER_VAL
    sta FLOWER_VAL
    lda #0
    sbc FLOWER_VAL + 1
    sta FLOWER_VAL + 1
@done:
    rts
.endproc

; The sample rows of the petal in FLOWER_SHAPE that lie within the half height of its centre row: the
; row r of the mask (k + 2 for the tile's sample row k) has dy = 8 * r - 12 - cy, so the first is
; (cy - half - 4) / 8 + 3 and the last (cy + half - 5) / 8 + 2, cut to 0-49. Within them |dy| is below 64, so dy is a signed byte. The shear 32 * (shear * dy + 16) moves by
; 8 * shear from row to row, so only the first row needs a multiplication.
.proc flower_draw_petal_rows
    sec
    lda FLOWER_CY
    sbc FLOWER_HALF
    sta FLOWER_T
    lda FLOWER_CY + 1
    sbc #0
    sta FLOWER_T + 1
    sec
    lda FLOWER_T
    sbc #4
    sta FLOWER_T
    lda FLOWER_T + 1
    sbc #0
    sta FLOWER_T + 1
    jsr flower_shift_down
    lda FLOWER_T + 1
    clc
    lda FLOWER_T
    adc #3
    sta FLOWER_T
    bcc @shifted_up
    inc FLOWER_T + 1
@shifted_up:
    lda FLOWER_T + 1
    bmi @from_top
    bne @below
    lda FLOWER_T
    cmp #FLOWER_MASK_ROWS
    bcc @first_ready
@below:
    rts
@from_top:
    lda #0
@first_ready:
    sta FLOWER_ROW_FIRST

    clc
    lda FLOWER_CY
    adc FLOWER_HALF
    sta FLOWER_T
    lda FLOWER_CY + 1
    adc #0
    sta FLOWER_T + 1
    sec
    lda FLOWER_T
    sbc #5
    sta FLOWER_T
    lda FLOWER_T + 1
    sbc #0
    sta FLOWER_T + 1
    jsr flower_shift_down
    clc
    lda FLOWER_T
    adc #2
    sta FLOWER_T
    bcc @shifted_down
    inc FLOWER_T + 1
@shifted_down:
    lda FLOWER_T + 1
    bpl @not_above
    rts
@not_above:
    bne @bottom
    lda FLOWER_T
    cmp #FLOWER_MASK_ROWS
    bcc @last_ready
@bottom:
    lda #FLOWER_MASK_ROWS - 1
@last_ready:
    sec
    sbc FLOWER_ROW_FIRST
    bcs @rows
    rts
@rows:
    clc
    adc #1
    sta FLOWER_ROW_COUNT

    lda FLOWER_ROW_FIRST
    sta FLOWER_PTR
    lda #0
    sta FLOWER_PTR + 1
    asl FLOWER_PTR
    rol FLOWER_PTR + 1
    lda FLOWER_PTR
    sta FLOWER_T
    lda FLOWER_PTR + 1
    sta FLOWER_T + 1
    asl FLOWER_PTR
    rol FLOWER_PTR + 1
    clc
    lda FLOWER_PTR
    adc FLOWER_T
    sta FLOWER_PTR
    lda FLOWER_PTR + 1
    adc FLOWER_T + 1
    sta FLOWER_PTR + 1
    clc
    lda FLOWER_PTR
    adc #<FLOWER_MASK
    sta FLOWER_PTR
    lda FLOWER_PTR + 1
    adc #>FLOWER_MASK
    sta FLOWER_PTR + 1

    lda FLOWER_ROW_FIRST
    asl
    asl
    asl
    sec
    sbc #12
    sec
    sbc FLOWER_CY
    sta FLOWER_DY
    jsr flower_start_shear
    lda FLOWER_ROW_FIRST
    sta FLOWER_ROW_INDEX
@row:
    jsr job_checkpoint
    lda FLOWER_DY
    bpl @distance
    eor #$FF
    clc
    adc #1
@distance:
    sta FLOWER_AD
    jsr flower_draw_chords
    clc
    lda FLOWER_DY
    adc #8
    sta FLOWER_DY
    clc
    lda FLOWER_ACC
    adc FLOWER_STEP
    sta FLOWER_ACC
    lda FLOWER_ACC + 1
    adc FLOWER_STEP + 1
    sta FLOWER_ACC + 1
    clc
    lda FLOWER_PTR
    adc #6
    sta FLOWER_PTR
    bcc @same_page
    inc FLOWER_PTR + 1
@same_page:
    inc FLOWER_ROW_INDEX
    dec FLOWER_ROW_COUNT
    beq @done
    jmp @row
@done:
    rts
.endproc

.segment "FLOWERMORE"

; FLOWER_ACC = shear * dy for the first row, FLOWER_STEP = 8 * shear, both signed words
.proc flower_start_shear
    lda FLOWER_SHEAR
    ldx #0
    cmp #$80
    bcc @positive
    ldx #$FF
@positive:
    sta FLOWER_STEP
    stx FLOWER_STEP + 1
    ldy #3
@times_eight:
    asl FLOWER_STEP
    rol FLOWER_STEP + 1
    dey
    bne @times_eight

    lda FLOWER_SHEAR
    ldx #0
    cmp #$80
    bcc @shear_magnitude
    eor #$FF
    clc
    adc #1
    ldx #1
@shear_magnitude:
    sta FLOWER_MUL_A
    stx FLOWER_NEG
    lda FLOWER_DY
    ldx #0
    cmp #$80
    bcc @distance_magnitude
    eor #$FF
    clc
    adc #1
    ldx #1
@distance_magnitude:
    pha
    txa
    eor FLOWER_NEG
    sta FLOWER_NEG
    pla
    jsr flower_multiply
    lda FLOWER_N
    sta FLOWER_ACC
    lda FLOWER_N + 1
    sta FLOWER_ACC + 1
    lda FLOWER_NEG
    beq @done
    sec
    lda #0
    sbc FLOWER_ACC
    sta FLOWER_ACC
    lda #0
    sbc FLOWER_ACC + 1
    sta FLOWER_ACC + 1
@done:
    rts
.endproc

.segment "FLOWERHELP"

; The chords of the row's petal at its wraps: the shear is acc / 32, halves rounded away from zero
.proc flower_draw_chords
    ldx FLOWER_ROW_INDEX
    lda #1
    sta FLOWER_USED,x
    clc
    lda FLOWER_ROW_BASE
    adc FLOWER_AD
    tay
    lda FLOWER_ROW_WIDTHS,y
    sta FLOWER_WD
    lda #0
    sta FLOWER_NEG
    lda FLOWER_ACC
    sta FLOWER_T
    lda FLOWER_ACC + 1
    sta FLOWER_T + 1
    bpl @rounded
    inc FLOWER_NEG
    sec
    lda #0
    sbc FLOWER_ACC
    sta FLOWER_T
    lda #0
    sbc FLOWER_ACC + 1
    sta FLOWER_T + 1
@rounded:
    clc
    lda FLOWER_T
    adc #16
    sta FLOWER_T
    bcc @half_added
    inc FLOWER_T + 1
@half_added:
    ldx #5
@shear:
    lsr FLOWER_T + 1
    ror FLOWER_T
    dex
    bne @shear
    lda FLOWER_NEG
    beq @moved
    sec
    lda #0
    sbc FLOWER_T
    sta FLOWER_T
    lda #0
    sbc FLOWER_T + 1
    sta FLOWER_T + 1
@moved:
    clc
    lda FLOWER_CX
    adc FLOWER_T
    sta FLOWER_CENTRE
    lda FLOWER_CX + 1
    adc FLOWER_T + 1
    sta FLOWER_CENTRE + 1
    ldx #0
    stx FLOWER_WRAP_INDEX
@wrap:
    ldx FLOWER_WRAP_INDEX
    clc
    lda FLOWER_CENTRE
    adc flower_wraps_low,x
    sta FLOWER_SPAN_C
    lda FLOWER_CENTRE + 1
    adc flower_wraps_high,x
    sta FLOWER_SPAN_C + 1
    jsr flower_set_span
    inc FLOWER_WRAP_INDEX
    lda FLOWER_WRAP_INDEX
    cmp FLOWER_WRAPS
    bcc @wrap
    rts
.endproc

; ─── Set the subpixels of FLOWER_SPAN_C +- FLOWER_WD in the row at FLOWER_PTR ───
; The sample of subpixel j is at 8 * j + 4, so the first one is (c - w + 3) / 8 and the last (c + w - 4) / 8,
; both rounded down and cut to 0-47.
.proc flower_set_span
    sec
    lda FLOWER_SPAN_C
    sbc FLOWER_WD
    sta FLOWER_T
    lda FLOWER_SPAN_C + 1
    sbc #0
    sta FLOWER_T + 1
    clc
    lda FLOWER_T
    adc #3
    sta FLOWER_T
    bcc @first_added
    inc FLOWER_T + 1
@first_added:
    jsr flower_shift_down
    lda FLOWER_T + 1
    bmi @first_zero
    bne @none
    lda FLOWER_T
    cmp #48
    bcs @none
    bcc @first_ready
@first_zero:
    lda #0
@first_ready:
    sta FLOWER_FIRST

    clc
    lda FLOWER_SPAN_C
    adc FLOWER_WD
    sta FLOWER_T
    lda FLOWER_SPAN_C + 1
    adc #0
    sta FLOWER_T + 1
    sec
    lda FLOWER_T
    sbc #4
    sta FLOWER_T
    lda FLOWER_T + 1
    sbc #0
    sta FLOWER_T + 1
    jsr flower_shift_down
    lda FLOWER_T + 1
    bmi @none
    bne @top
    lda FLOWER_T
    cmp #48
    bcc @last_ready
@top:
    lda #47
@last_ready:
    sta FLOWER_LAST
    cmp FLOWER_FIRST
    bcc @none
    jmp flower_set_bits
@none:
    rts
.endproc

; FLOWER_T = FLOWER_T / 8 rounded down
.proc flower_shift_down
    ldx #3
@bit:
    lda FLOWER_T + 1
    cmp #$80
    ror FLOWER_T + 1
    ror FLOWER_T
    dex
    bne @bit
    rts
.endproc

.segment "FLOWERHELP"

; Sets the bits FLOWER_FIRST to FLOWER_LAST of the 6 bytes at FLOWER_PTR, bit 7 of the first byte being 0
.proc flower_set_bits
    lda FLOWER_FIRST
    lsr
    lsr
    lsr
    sta FLOWER_BYTE_FIRST
    tay
    lda FLOWER_LAST
    lsr
    lsr
    lsr
    sta FLOWER_BYTE_LAST
@byte:
    lda #0
    cpy FLOWER_BYTE_FIRST
    bne @head
    lda FLOWER_FIRST
    and #7
@head:
    tax
    lda flower_head,x
    sta FLOWER_BITS
    lda #7
    cpy FLOWER_BYTE_LAST
    bne @tail
    lda FLOWER_LAST
    and #7
@tail:
    tax
    lda FLOWER_BITS
    and flower_tail,x
    ora (FLOWER_PTR),y
    sta (FLOWER_PTR),y
    cpy FLOWER_BYTE_LAST
    beq @done
    iny
    bne @byte
@done:
    rts
.endproc

; ─── Draw the solid square into FLOWER_SQUARE ────────────────────────────────
.proc flower_draw_square
    lda #<FLOWER_SQUARE
    sta FLOWER_PTR
    lda #>FLOWER_SQUARE
    sta FLOWER_PTR + 1
    lda #<FLOWER_CENTRE_X
    sta FLOWER_SPAN_C
    lda #>FLOWER_CENTRE_X
    sta FLOWER_SPAN_C + 1
    ldy #FLOWER_SQUARE_HALF
    lda (FLOWER_REC),y
    sta FLOWER_WD
    jmp flower_set_span
.endproc

.segment "FLOWERHELP"

; ─── Turn the masks into the packed tile of FLOWER_TILE ──────────────────────
; A pixel has 4 subpixels. A pixel is full with 3 or more petal subpixels, medium with 1 or 2, and the
; square adds its own grey level on the subpixels the petals leave (see flower_levels).
.proc flower_pack_tile
    ldy #FLOWER_SHADE_BYTE
    lda (FLOWER_REC),y
    sta FLOWER_SHADE
    ldy #FLOWER_SQUARE_Y
    sec
    lda #4
    sbc (FLOWER_REC),y
    sta FLOWER_SQ_DY
    lda #0
    sbc #0
    sta FLOWER_SQ_DY + 1
    lda #<FLOWER_FIRST_TILE_ROW
    sta FLOWER_PTR
    lda #>FLOWER_FIRST_TILE_ROW
    sta FLOWER_PTR + 1
    lda #<FLOWER_TURNED_ROW
    sta FLOWER_PTR2
    lda #>FLOWER_TURNED_ROW
    sta FLOWER_PTR2 + 1
    lda #0
    sta FLOWER_PIXEL_ROW
    sta FLOWER_ROW_OFFSET
@row:
    jsr flower_square_active
    sta FLOWER_SQ_ACTIVE0
    jsr flower_square_active
    sta FLOWER_SQ_ACTIVE1
    lda FLOWER_PIXEL_ROW
    clc
    adc FLOWER_PHASE
    and #1
    tax
    lda flower_lit,x
    sta FLOWER_LIT
    jsr job_checkpoint
    lda FLOWER_PIXEL_ROW
    asl
    tax
    lda FLOWER_USED + 2,x
    ora FLOWER_USED + 3,x
    ora FLOWER_SQ_ACTIVE0
    ora FLOWER_SQ_ACTIVE1
    sta FLOWER_T
    stx FLOWER_T + 1
    lda #FLOWER_SUB_ROWS - 1
    sec
    sbc FLOWER_T + 1
    tay
    lda FLOWER_USED,y
    ora FLOWER_USED - 1,y
    ora FLOWER_T
    bne @drawn_row
    lda #$FF
    ldx FLOWER_ROW_OFFSET
    ldy #TILE_ROW_BYTES
@empty:
    sta FLOWER_TILE,x
    inx
    dey
    bne @empty
    jmp @advance
@drawn_row:
    lda #0
    sta FLOWER_BYTE
@byte:
    lda FLOWER_BYTE
    and #1
    bne @same_group
    jsr job_checkpoint
@same_group:
    jsr flower_load_byte_rows
    lda #0
    sta FLOWER_ANY
    sta FLOWER_ALL
    lda FLOWER_P0
    ora FLOWER_P1
    ora FLOWER_Q0
    ora FLOWER_Q1
    beq @levels_ready
    jsr flower_levels
@levels_ready:
    lda FLOWER_ANY
    asl
    rol FLOWER_FULL
    asl
    asl
    rol FLOWER_FULL
    asl
    asl
    rol FLOWER_FULL
    asl
    asl
    rol FLOWER_FULL
    lda FLOWER_ALL
    asl
    rol FLOWER_MEDIUM
    asl
    asl
    rol FLOWER_MEDIUM
    asl
    asl
    rol FLOWER_MEDIUM
    asl
    asl
    rol FLOWER_MEDIUM
    lda FLOWER_BYTE
    and #1
    beq @half_done
    jsr flower_store_row_bytes
@half_done:
    inc FLOWER_BYTE
    lda FLOWER_BYTE
    cmp #6
    bcs @advance
    jmp @byte
@advance:
    clc
    lda FLOWER_ROW_OFFSET
    adc #TILE_ROW_BYTES
    sta FLOWER_ROW_OFFSET
    clc
    lda FLOWER_PTR
    adc #12
    sta FLOWER_PTR
    bcc @same_page
    inc FLOWER_PTR + 1
@same_page:
    sec
    lda FLOWER_PTR2
    sbc #12
    sta FLOWER_PTR2
    bcs @same_page2
    dec FLOWER_PTR2 + 1
@same_page2:
    inc FLOWER_PIXEL_ROW
    lda FLOWER_PIXEL_ROW
    cmp #TILE_ROWS
    bcs @done
    jmp @row
@done:
    rts
.endproc

; A = $FF when the square covers the sample row at FLOWER_SQ_DY, otherwise 0; moves on to the next sample row
.proc flower_square_active
    lda FLOWER_SQ_DY
    ldx FLOWER_SQ_DY + 1
    bpl @magnitude
    sec
    lda #0
    sbc FLOWER_SQ_DY
    pha
    lda #0
    sbc FLOWER_SQ_DY + 1
    tax
    pla
@magnitude:
    stx FLOWER_T + 1
    sta FLOWER_T
    clc
    lda FLOWER_SQ_DY
    adc #8
    sta FLOWER_SQ_DY
    bcc @moved
    inc FLOWER_SQ_DY + 1
@moved:
    lda FLOWER_T + 1
    bne @inactive
    ldy #FLOWER_SQUARE_HALF
    lda (FLOWER_REC),y
    cmp FLOWER_T
    bcc @inactive
    lda #$FF
    rts
@inactive:
    lda #0
    rts
.endproc

; The two sample rows of the byte FLOWER_BYTE: FLOWER_P0 and FLOWER_P1 hold the petals', FLOWER_Q0 and
; FLOWER_Q1 the square's where the petals are not
.proc flower_load_byte_rows
    ldy FLOWER_BYTE
    lda (FLOWER_PTR),y
    sta FLOWER_P0
    tya
    clc
    adc #6
    tay
    lda (FLOWER_PTR),y
    sta FLOWER_P1
    lda FLOWER_ROTATE
    beq @merged
    lda #5
    sec
    sbc FLOWER_BYTE
    tay
    lda (FLOWER_PTR2),y
    tax
    lda BIT_REVERSE_TABLE,x
    ora FLOWER_P1
    sta FLOWER_P1
    tya
    clc
    adc #6
    tay
    lda (FLOWER_PTR2),y
    tax
    lda BIT_REVERSE_TABLE,x
    ora FLOWER_P0
    sta FLOWER_P0
@merged:
    ldy FLOWER_BYTE
    lda FLOWER_P0
    eor #$FF
    and FLOWER_SQUARE,y
    and FLOWER_SQ_ACTIVE0
    sta FLOWER_Q0
    lda FLOWER_P1
    eor #$FF
    and FLOWER_SQUARE,y
    and FLOWER_SQ_ACTIVE1
    sta FLOWER_Q1
    rts
.endproc

; Stores the 8 pixels shifted into FLOWER_FULL and FLOWER_MEDIUM (set bits: the pixel is not lit) as byte
; FLOWER_BYTE / 2 of the row
.proc flower_store_row_bytes
    lda FLOWER_BYTE
    lsr
    clc
    adc FLOWER_ROW_OFFSET
    tay
    lda FLOWER_FULL
    eor #$FF
    sta FLOWER_TILE,y
    lda FLOWER_MEDIUM
    eor #$FF
    sta FLOWER_TILE + 3,y
    rts
.endproc

; ─── The levels of the 4 pixels of the byte, as two planes: FLOWER_ANY has a bit (7, 5, 3, 1 for the pixels
; left to right) for each pixel with a subpixel, FLOWER_ALL for each with 3 or more, which is light grey ─────
; A pair of subpixels is one pixel's row: with the two rows x and y, a pixel has 3 or more when one row has both
; and the other at least one. The square only adds what the petals leave: a fading square is its own level,
; light grey, dark grey or the checkerboard of them (FLOWER_LIT marks the pixels the checkerboard lights).
.proc flower_levels
    lda FLOWER_SHADE
    cmp #4
    bne @fading
    lda FLOWER_P0
    ora FLOWER_Q0
    sta FLOWER_X
    lda FLOWER_P1
    ora FLOWER_Q1
    sta FLOWER_Y
    jsr flower_planes
    lda FLOWER_AN
    sta FLOWER_ANY
    lda FLOWER_AL
    sta FLOWER_ALL
    rts
@fading:
    lda FLOWER_P0
    sta FLOWER_X
    lda FLOWER_P1
    sta FLOWER_Y
    jsr flower_planes
    lda FLOWER_AN
    sta FLOWER_ANY
    lda FLOWER_AL
    sta FLOWER_ALL
    lda FLOWER_SHADE
    beq @done
    lda FLOWER_Q0
    sta FLOWER_X
    lda FLOWER_Q1
    sta FLOWER_Y
    jsr flower_planes
    lda FLOWER_SHADE
    cmp #3
    bne @not_three
    lda FLOWER_AL
    and FLOWER_LIT
    ora FLOWER_ALL
    sta FLOWER_ALL
    lda FLOWER_AN
    ora FLOWER_ANY
    sta FLOWER_ANY
    rts
@not_three:
    lda FLOWER_SHADE
    cmp #2
    bne @one
    lda FLOWER_AN
    ora FLOWER_ANY
    sta FLOWER_ANY
    rts
@one:
    lda FLOWER_AN
    and FLOWER_LIT
    ora FLOWER_ANY
    sta FLOWER_ANY
@done:
    rts
.endproc

.segment "FLOWERMORE"

; FLOWER_AN, FLOWER_AL = the planes of the pixels of the rows FLOWER_X and FLOWER_Y
.proc flower_planes
    lda FLOWER_X
    asl
    ora FLOWER_X
    sta FLOWER_XO
    lda FLOWER_X
    asl
    and FLOWER_X
    sta FLOWER_XA
    lda FLOWER_Y
    asl
    ora FLOWER_Y
    sta FLOWER_YO
    lda FLOWER_Y
    asl
    and FLOWER_Y
    sta FLOWER_YA
    lda FLOWER_XO
    ora FLOWER_YO
    and #$AA
    sta FLOWER_AN
    lda FLOWER_XA
    and FLOWER_YO
    sta FLOWER_XA
    lda FLOWER_YA
    and FLOWER_XO
    ora FLOWER_XA
    and #$AA
    sta FLOWER_AL
    rts
.endproc

.segment "FLOWERHELP"

.segment "FLOWERTAIL"

; ─── FLOWER_ODD_TILE = FLOWER_TILE flipped about row 11 ──────────────────────
; Row 22 - r of the odd tile is row r of the tile; row 23 is empty.
.proc flower_flip
    ldx #0
    ldy #FLOWER_ODD_FLIP * TILE_ROW_BYTES
@row:
    jsr job_checkpoint
    lda #TILE_ROW_BYTES
    sta FLOWER_BYTE
@byte:
    lda FLOWER_TILE,x
    sta FLOWER_ODD_TILE,y
    inx
    iny
    dec FLOWER_BYTE
    bne @byte
    tya
    sec
    sbc #2 * TILE_ROW_BYTES
    tay
    cpx #FLOWER_ODD_FLIP * TILE_ROW_BYTES + TILE_ROW_BYTES
    bcc @row
    lda #$FF
    ldy #TILE_ROW_BYTES - 1
@empty:
    sta FLOWER_ODD_TILE + FLOWER_ODD_FLIP * TILE_ROW_BYTES + TILE_ROW_BYTES,y
    dey
    bpl @empty
    rts
.endproc

flower_lit:
    .byte $22, $88
flower_row_base:
    .byte 0, FLOWER_ROW_WIDTH_BYTES, 2 * FLOWER_ROW_WIDTH_BYTES
flower_head:
    .byte $FF, $7F, $3F, $1F, $0F, $07, $03, $01
flower_tail:
    .byte $80, $C0, $E0, $F0, $F8, $FC, $FE, $FF
flower_wraps_low:
    .byte 0, $80, $80
flower_wraps_high:
    .byte 0, $FE, $01
