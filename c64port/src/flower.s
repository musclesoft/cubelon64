; flower.s - The square turns into a flower of six petals, and the flower walks
;
; The last square of the marching cubes becomes the sunflower the marching flowers start from, and the scene loops
; through cycles of 40 frames (demo frames 3359-3958, the sequencer's phases 80-94): the square turns into the flower,
; the flower marches twice and turns back into the square, a state of the cycle every second frame like the ZX demo.
; proto fits sweeps to the ZX frames and turns each state into a record of 18 bytes (go run -C proto . flower,
; flower_data.inc): the square's half size, centre row and shade, three petal shapes (half height, shear, width) and the
; offset of one petal of each shape; the other three petals lie opposite. Lengths are in 1/16 pixel.
;
; The renderer draws a record in subpixels: 48 sample rows of 48 sample columns, one bit each. A petal is an
; ellipse, drawn as chords: for each sample row the chord's centre moves by the shear, its half width is
; the half circle (flower_circle) times the shape's width, and the samples within it are set. Without a shift the
; second three petals are the first three turned by half a turn, which the pixel pass adds from the reversed rows
; (with FLOWER_SHIFT, for a flower that is not centred in the tile, all six are drawn and wrap around the tile). The petals and the
; solid square merge before the grey level of a pixel is chosen, the square fading from light grey to black in the
; shade 4-0 with a checkerboard in between. The tile of the even rows of cells is packed like the other tiles
; (see tile.inc), the tile of the odd rows is its flip about row 11.
;
; What the cycles show (go run -C proto . flower cycles checks all of it against the ZX frames). The ZX holds each of its
; pictures for two frames; the C64 draws the 50 Hz state between two pictures as well, so that no frame repeats another:
;   kind 0  the morph: the even states 2-18 (FLOWER_IMAGES) and the odd states 1-17 (FLOWER_ODD_IMAGES), a pair of
;           images each: the tile of the even rows of cells and its flip for the odd rows. The square is no render:
;           flower_square_tile fills its tile
;   kind 3  the morph backwards: the same pairs mirrored (FLOWER_MIRRORED_IMAGES, FLOWER_ODD_MIRRORED_IMAGES), the
;           mirror image of an image made by flower_mirror_image
;   kinds 1 and 2  the flower marching in a straight line at half pixel steps: the states 1-10 of a kick are rendered once
;           each at their own shift, the states 11-20 are those mirrored about x = 6, the second kick is the first with
;           the rows swapped, and the odd rows' tile is the even rows' tile mirrored, which gives the 20 steps and the
;           20 states between them of a pair of kicks (FLOWER_MARCH_IMAGES, the states between in two arrays)
; and the even rows and the odd rows swap their tiles every four cycles. The images are checked by
; proto/golden/flower_*images.bin and flower_*tiles.bin.
;
; Memory and time: the job builds in the order the scene needs: first what the first morph needs (the images of the
; even and odd states, the turns' tiles; the tile area is free once the diamonds are over), then, from frame 3190 when
; the rotating cubes' memory is free, the marching pairs (FLOWER_MARCH_IMAGES; FLOWER_BETWEEN_HIGH under the KERNAL and
; FLOWER_BETWEEN_LOW in the cubes' ring buffer, with the backwards morph's pairs), mirroring the backwards morph from
; the forward one one pair before it is needed. The state 1 is the last to be rendered. The work arrays, the
; variables and the turns' tiles lie on the morph's images. The code lies in the gaps of the VIC bank that c64.cfg lists
; as the pool of the FLOWERnn segments: the assembler packs them, largest first, and each piece must end with a
; jump or a return (the branches between pieces would be out of range).

.include "include/shared_imports.inc"
.include "include/tile.inc"

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
FLOWER_SQUARE_PAIR = 9
FLOWER_FIRST_PHASE = 80
FLOWER_LAST_MORPH_BETWEEN = 8
FLOWER_MARCH_STEPS = 20
FLOWER_BETWEEN_HIGH_STEPS = 16
FLOWER_RENDERED_STATES = 5
FLOWER_PAIR_BYTES = 2 * IMAGE_BYTES
FLOWER_WORK = MORPH_IMAGES
FLOWER_MASK = FLOWER_WORK
FLOWER_MASK_ROWS = FLOWER_SUB_ROWS + 2
FLOWER_FIRST_TILE_ROW = FLOWER_MASK + 2 * 6
FLOWER_TURNED_ROW = FLOWER_MASK + 46 * 6
FLOWER_TILE = FLOWER_MASK + FLOWER_MASK_ROWS * 6
FLOWER_ODD_TILE = FLOWER_TILE + TILE_BYTES
FLOWER_ROW_WIDTHS = FLOWER_ODD_TILE + TILE_BYTES
FLOWER_W16 = FLOWER_ROW_WIDTHS + 3 * FLOWER_ROW_WIDTH_BYTES
FLOWER_SQUARE = FLOWER_W16 + 16
FLOWER_VARS = FLOWER_SQUARE + 6
FLOWER_TURN_TILES = FLOWER_VARS + 160
FLOWER_ODD_TURN_TILES = $93E0
FLOWER_IMAGES = FLOWER_ODD_TURN_TILES + 10 * TILE_BYTES
FLOWER_ODD_IMAGES = FLOWER_IMAGES + 10 * FLOWER_PAIR_BYTES
FLOWER_MARCH_IMAGES = FLOWER_ODD_IMAGES + 9 * FLOWER_PAIR_BYTES
FLOWER_ODD_MIRRORED_IMAGES = FLOWER_MARCH_IMAGES + FLOWER_MARCH_STEPS * FLOWER_PAIR_BYTES
FLOWER_BETWEEN_HIGH = $E000
FLOWER_BETWEEN_LOW = $6E00
FLOWER_MIRRORED_IMAGES = FLOWER_BETWEEN_LOW + 4 * FLOWER_PAIR_BYTES

.segment "ZEROPAGE"

FLOWER_REC: .res 2
FLOWER_PTR: .res 2
FLOWER_PTR2: .res 2
FLOWER_P0: .res 1
FLOWER_P1: .res 1
FLOWER_Q0: .res 1
FLOWER_Q1: .res 1

; The work arrays and the variables lie on the morph's images, which are over after frame 2019

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
FLOWER_SCENE_STEP = FLOWER_VARS + 130
FLOWER_CYCLE = FLOWER_VARS + 131
FLOWER_SWAP = FLOWER_VARS + 132
FLOWER_MIRROR = FLOWER_VARS + 133
FLOWER_BETWEEN = FLOWER_VARS + 134
FLOWER_TURN_BASE = FLOWER_VARS + 135
FLOWER_IMAGE_BASE = FLOWER_VARS + 142
FLOWER_IMAGE_INDEX = FLOWER_VARS + 144
FLOWER_KIND = FLOWER_VARS + 146
FLOWER_TURN = FLOWER_VARS + 137
FLOWER_TURN_INDEX = FLOWER_VARS + 138
FLOWER_J = FLOWER_VARS + 139
FLOWER_SOURCE = FLOWER_VARS + 147
FLOWER_SWAPPED = FLOWER_VARS + 148
FLOWER_RENDERED_SLOT = FLOWER_VARS + 149
FLOWER_MIRRORED_SLOT = FLOWER_VARS + 150



.import store_image: absolute
.import image_address: absolute
.import show_images: absolute
.import store_image: absolute
.import sprite_image_index: absolute
.import SHOW_IMAGE_A: absolute
.import SHOW_IMAGE_B: absolute
.import TILE_SRC_PTR: absolute
.import flower_circle: absolute
.import flower_records: absolute
.import flower_march_records: absolute
.import flower_between_records: absolute
.import flower_march_between_records: absolute
.import flower_march_between_shifts: absolute
.import flower_march_shifts: absolute
.import flower_march_sources: absolute
.import BIT_REVERSE_TABLE: absolute

.export flower_precompute
.export flower_step
.export flower_between_precompute
.export flower_march_render
.export flower_render
.export flower_flip
.export FLOWER_REC
.export FLOWER_SHIFT
.export FLOWER_PHASE


; ─── Build the images of the even states: the pairs 0-8 are the states 2-18, the pair 9 is the square ──
; The backwards morph shows the same tiles mirrored, which are stored as the pairs 0-8 of their own array: the
; pair r of the state 16 - 2 * r, the last one the square.
.segment "FLOWER01"

.proc flower_precompute
    ldx #<flower_records
    ldy #>flower_records
    jsr flower_aim_records
    lda #0
    sta FLOWER_SHIFT
    sta FLOWER_SHIFT + 1
    sta FLOWER_PHASE
    sta FLOWER_STATE
@state:
    jsr flower_build_pair
    jsr flower_next_record
    inc FLOWER_STATE
    lda FLOWER_STATE
    cmp #FLOWER_SQUARE_PAIR
    bcc @state
    lda #<FLOWER_TURN_TILES
    sta FLOWER_TURN_BASE
    lda #>FLOWER_TURN_TILES
    sta FLOWER_TURN_BASE + 1
    lda #0
    sta FLOWER_TURN_INDEX
    jsr flower_keep_turn
    jsr flower_square_tile
    jsr flower_store_tiles
    assert_if FLOWER_STATE, 9, FLOWER_IMAGES, "../proto/golden/flower_images.bin", 2880
    ldx #<flower_between_records
    ldy #>flower_between_records
    jsr flower_aim_records
    jsr flower_next_record
    lda #1
    sta FLOWER_PHASE
    sta FLOWER_STATE
@odd:
    jsr flower_build_odd_pair
    jsr flower_next_record
    inc FLOWER_STATE
    lda FLOWER_STATE
    cmp #FLOWER_SQUARE_PAIR
    bcc @odd
    rts
.endproc

; The state 1 is only needed for the backwards morph, which starts much later
.proc flower_late_pair
    lda #0
    sta FLOWER_STATE
    ldx #<flower_between_records
    ldy #>flower_between_records
    jsr flower_aim_records
    lda #1
    sta FLOWER_PHASE
    jsr flower_build_odd_pair
    assert_if FLOWER_STATE, 0, FLOWER_ODD_IMAGES, "../proto/golden/flower_odd_images.bin", 2592
    rts
.endproc


.segment "FLOWER02"

.proc flower_aim_records
    stx FLOWER_REC
    sty FLOWER_REC + 1
    rts
.endproc

.proc flower_next_record
    clc
    lda FLOWER_REC
    adc #FLOWER_RECORD_BYTES
    sta FLOWER_REC
    bcc @done
    inc FLOWER_REC + 1
@done:
    rts
.endproc

; Renders the record and stores its tile and the flipped tile as the pair FLOWER_STATE of the images at X, Y
.proc flower_build_pair
    ldx #<FLOWER_IMAGES
    ldy #>FLOWER_IMAGES
    bne flower_build_in
.endproc

.proc flower_build_odd_pair
    ldx #<FLOWER_ODD_IMAGES
    ldy #>FLOWER_ODD_IMAGES
.endproc

.proc flower_build_in
    stx FLOWER_IMAGE_BASE
    sty FLOWER_IMAGE_BASE + 1
    jsr flower_render
.endproc

; Stores FLOWER_TILE and its flip as the pair FLOWER_STATE of the images at FLOWER_IMAGE_BASE
.proc flower_store_tiles
    jsr flower_flip
    ldx #<FLOWER_TILE
    ldy #>FLOWER_TILE
    lda FLOWER_STATE
    asl
    jsr flower_store_one
    ldx #<FLOWER_ODD_TILE
    ldy #>FLOWER_ODD_TILE
    lda FLOWER_STATE
    asl
    ora #1
.endproc

; Stores the tile at X, Y as the image A of the array at FLOWER_IMAGE_BASE
.proc flower_store_one
    stx TILE_SRC_PTR
    sty TILE_SRC_PTR + 1
    ldx FLOWER_IMAGE_BASE
    ldy FLOWER_IMAGE_BASE + 1
    jmp store_image
.endproc

; FLOWER_TILE = the square the marching cubes end with: the rows 6-17 are black from the pixel 6 to 17
.proc flower_square_tile
    ldx #0
    lda #$FF
@empty:
    sta FLOWER_TILE,x
    inx
    cpx #TILE_BYTES
    bne @empty
    ldx #0
    ldy #6 * TILE_ROW_BYTES
@fill:
    lda flower_square_row,x
    sta FLOWER_TILE,y
    iny
    inx
    cpx #3
    bcc @next
    ldx #0
@next:
    cpy #18 * TILE_ROW_BYTES
    bcc @fill
    rts
.endproc

flower_square_row:
    .byte $FC, $00, $3F


; ─── Show the tiles of the state of the cycle ────────────────────────────────
; The scene is a loop of cycles of 40 frames from frame 3359 (cycle 0 of the sequencer's phase 80): the square
; turns into the flower (kind 0), it marches twice (kinds 1 and 2) and turns back into the square (kind 3), a state
; of the cycle every second frame like the ZX demo, and the states between them. The kinds of rows swap their tiles
; every four cycles, from the cycle before the first march; the backwards morph shows the mirrored images, the other swap.
.segment "FLOWER05"

.proc flower_step
    lda SEQ_SCENE_STATE
    lsr
    sta FLOWER_SCENE_STEP
    lda #0
    rol
    sta FLOWER_BETWEEN
    lda FLOWER_SCENE_STEP
    beq @held
    jsr flower_select
    lda FLOWER_BETWEEN
    beq @show
    lda FLOWER_SCENE_STEP
    cmp #FLOWER_LAST_MORPH_BETWEEN + 1
    bcc @show
    bne @held
    lda FLOWER_CYCLE
    clc
    adc #1
    and #2
    beq @held
@show:
    jmp show_images
@held:
    rts
.endproc

; Picks the images of the step FLOWER_SCENE_STEP of the cycle into SHOW_IMAGE_A and SHOW_IMAGE_B, those of the
; state after it when FLOWER_BETWEEN is 1
.segment "FLOWER06"

.proc flower_select
    lda SEQ_PHASE
    sec
    sbc #FLOWER_FIRST_PHASE
    sta FLOWER_CYCLE
    clc
    adc #1
    lsr
    lsr
    and #1
    sta FLOWER_SWAP
    lda FLOWER_CYCLE
    and #3
    beq @morph
    cmp #3
    beq @mirrored
    cmp #2
    lda FLOWER_SCENE_STEP
    bcc @march
    clc
    adc #10
    cmp #20
    bcc @march
    sbc #20
@march:
    ldx FLOWER_BETWEEN
    jsr flower_march_base
    jmp flower_pair_images
@mirrored:
    lda FLOWER_SWAP
    eor #1
    sta FLOWER_SWAP
    lda #2
    jmp @pair
@morph:
    lda #0
@pair:
    ora FLOWER_BETWEEN
    tax
    jsr flower_morph_pair
    ldy flower_arrays_high,x
    pha
    lda flower_arrays_low,x
    tax
    pla
    jmp flower_pair_images
.endproc

; A = the pair of the state of the cycle in the morph's arrays: the step 1-9 are the pairs 0-8, the last step holds;
; the state after a step is the pair of the step in the arrays of the states between, which hold the states 1, 3 ... 17 of
; the forward morph (the step k is the pair k) and the states 15, 13 ... 1 of the backwards morph (the step k is the pair k - 1)
.segment "FLOWER07"

.proc flower_morph_pair
    lda FLOWER_SCENE_STEP
    ldy FLOWER_BETWEEN
    bne @between
    sec
    sbc #1
    cmp #FLOWER_SQUARE_PAIR
    bcc @done
    lda #FLOWER_SQUARE_PAIR - 1
@done:
    rts
@between:
    cpx #1
    beq @done
    sec
    sbc #1
    rts
.endproc

flower_arrays_low:
    .byte <FLOWER_IMAGES, <FLOWER_ODD_IMAGES, <FLOWER_MIRRORED_IMAGES, <FLOWER_ODD_MIRRORED_IMAGES
flower_arrays_high:
    .byte >FLOWER_IMAGES, >FLOWER_ODD_IMAGES, >FLOWER_MIRRORED_IMAGES, >FLOWER_ODD_MIRRORED_IMAGES

; ─── Pick the pair A of the array at X, Y; the pair's two images swap places when FLOWER_SWAP is 1 ─
.segment "FLOWER08"

.proc flower_pair_images
    asl
    jsr image_address
    sta SHOW_IMAGE_A
    sty SHOW_IMAGE_A + 1
    clc
    adc #IMAGE_BYTES
    sta SHOW_IMAGE_B
    tya
    adc #0
    sta SHOW_IMAGE_B + 1
    lsr FLOWER_SWAP
    bcc @shown
    ldx SHOW_IMAGE_A
    lda SHOW_IMAGE_B
    sta SHOW_IMAGE_A
    stx SHOW_IMAGE_B
    ldx SHOW_IMAGE_A + 1
    lda SHOW_IMAGE_B + 1
    sta SHOW_IMAGE_A + 1
    stx SHOW_IMAGE_B + 1
@shown:
    rts
.endproc

; ─── Render the record at FLOWER_REC, shifted by FLOWER_SHIFT, into FLOWER_TILE ─
; FLOWER_PHASE is the parity of the state, which moves the checkerboard of the fading square.
.segment "FLOWER09"

.proc flower_render
    jsr flower_clear_masks
    jsr flower_build_all_widths
    jsr flower_draw_petals
    jmp flower_pack_tile
.endproc

.segment "FLOWER10"

.proc flower_clear_masks
    lda #<FLOWER_MASK
    sta FLOWER_PTR
    lda #>FLOWER_MASK
    sta FLOWER_PTR + 1
    ldx #5
@chunk:
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
.segment "FLOWER11"

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
.segment "FLOWER12"

.proc flower_build_widths
    jsr flower_load_shape
    ldx #15
@circle:
    txa
    and #3
    bne @same_group
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
.segment "FLOWER13"

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
.segment "FLOWER14"

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
.segment "FLOWER15"

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
    jsr flower_aim_petal
    jsr flower_draw_petal_rows
    inc FLOWER_PETAL
    lda FLOWER_PETAL
    cmp FLOWER_PETAL_LIMIT
    bcc @petal
    jmp flower_draw_square
.endproc

; Loads the shape of the petal and its centre: the offset of the first petal of the shape, negated for the opposite one
.segment "FLOWER16"

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
.segment "FLOWER17"

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
.segment "FLOWER18"

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


; FLOWER_ACC = shear * dy for the first row, FLOWER_STEP = 8 * shear, both signed words
.segment "FLOWER19"

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


; The chords of the row's petal at its wraps: the shear is acc / 32, halves rounded away from zero
.segment "FLOWER20"

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
.segment "FLOWER21"

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
.segment "FLOWER22"

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


; Sets the bits FLOWER_FIRST to FLOWER_LAST of the 6 bytes at FLOWER_PTR, bit 7 of the first byte being 0
.segment "FLOWER23"

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
.segment "FLOWER24"

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


; ─── Turn the masks into the packed tile of FLOWER_TILE ──────────────────────
; A pixel has 4 subpixels. A pixel is full with 3 or more petal subpixels, medium with 1 or 2, and the
; square adds its own grey level on the subpixels the petals leave (see flower_levels).
.segment "FLOWER25"

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
.segment "FLOWER26"

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
.segment "FLOWER27"

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
.segment "FLOWER28"

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
.segment "FLOWER29"

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


; FLOWER_AN, FLOWER_AL = the planes of the pixels of the rows FLOWER_X and FLOWER_Y
.segment "FLOWER30"

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



; ─── FLOWER_ODD_TILE = FLOWER_TILE flipped about row 11 ──────────────────────
; Row 22 - r of the odd tile is row r of the tile; row 23 is empty.
.segment "FLOWER31"

.proc flower_flip
    ldx #0
    ldy #FLOWER_ODD_FLIP * TILE_ROW_BYTES
@row:
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

; ─── Render the marching flowers' states 1-10 ─────────────────────────────────
; The flower marches in a straight line (0.6 pixel a state) and turns by a constant angle, so a state of a kick is
; drawn at its own shift. Mirroring gives the rest: the state 20 - m of the first kick is the state m mirrored
; about x = 6, the second kick is the first with the rows swapped, the odd rows' tile is the mirror (see
; flower_march_sources, proto's flowerMarchSource). So the states 2, 4 ... 10 and 1, 3 ... 9 are rendered once each,
; at half pixel shifts, and kept as the tiles 1-5 of FLOWER_TURN_TILES and 0-4 of FLOWER_ODD_TURN_TILES (the tile 0 of
; the first is the morph's last state, the flower at the state 0). They are rendered in the idle time before the
; rotating cubes' images are over.
.segment "FLOWER32"

.proc flower_march_render
    lda #0
    sta FLOWER_SHIFT + 1
    sta FLOWER_PHASE
    lda #<FLOWER_TURN_TILES
    sta FLOWER_TURN_BASE
    lda #>FLOWER_TURN_TILES
    sta FLOWER_TURN_BASE + 1
    ldx #<flower_march_records
    ldy #>flower_march_records
    jsr flower_aim_records
    lda #1
    jsr flower_render_states
    assert_if FLOWER_TURN, 5, FLOWER_TURN_TILES + 144, "../proto/golden/flower_march_tiles.bin", 720
    lda #<FLOWER_ODD_TURN_TILES
    sta FLOWER_TURN_BASE
    lda #>FLOWER_ODD_TURN_TILES
    sta FLOWER_TURN_BASE + 1
    ldx #<flower_march_between_records
    ldy #>flower_march_between_records
    jsr flower_aim_records
    lda #0
    jsr flower_render_states
    assert_if FLOWER_TURN, 5, FLOWER_ODD_TURN_TILES, "../proto/golden/flower_odd_turn_tiles.bin", 720
    lda #0
    sta FLOWER_SHIFT
    rts
.endproc

; Renders the five records from FLOWER_REC, each at its shift (the table of the even states when A is 1, of the odd
; ones when it is 0), and keeps their tiles as the tiles A, A + 1 ... of the array at FLOWER_TURN_BASE
.proc flower_render_states
    sta FLOWER_KIND
    lda #0
    sta FLOWER_TURN
@state:
    ldx FLOWER_TURN
    lda FLOWER_KIND
    beq @between
    lda flower_march_shifts,x
    jmp @shifted
@between:
    lda flower_march_between_shifts,x
@shifted:
    sta FLOWER_SHIFT
    jsr flower_render
    lda FLOWER_TURN
    clc
    adc FLOWER_KIND
    sta FLOWER_TURN_INDEX
    jsr flower_keep_turn
    jsr flower_next_record
    inc FLOWER_TURN
    lda FLOWER_TURN
    cmp #FLOWER_RENDERED_STATES
    bcc @state
    rts
.endproc

; ─── Build the images of the states between the ones the ZX shows ─────────────
; The ZX holds a picture for two frames, the C64 draws the state in between: the odd states 1-17 of the morph (3-17
; forwards, 15-1 mirrored backwards) and the marching flower's odd states 1-9, rendered once each (flower_march_render) and
; mirrored like the even ones. The images lie on the rotating cubes' images, which are over from frame 3190.
.segment "FLOWER34"

.proc flower_between_precompute
    lda #0
    sta FLOWER_TURN
@step:
    lda #0
    sta FLOWER_KIND
@kind:
    lda FLOWER_TURN
    jsr flower_march_step
    inc FLOWER_KIND
    lda FLOWER_KIND
    cmp #2
    bcc @kind
    inc FLOWER_TURN
    lda FLOWER_TURN
    cmp #FLOWER_MARCH_STEPS
    bcc @step
    assert_if FLOWER_KIND, 2, FLOWER_MARCH_IMAGES, "../proto/golden/flower_march_images.bin", 5760
    assert_if FLOWER_KIND, 2, FLOWER_BETWEEN_HIGH, "../proto/golden/flower_march_images.bin" + 5760, 4608
    assert_if FLOWER_KIND, 2, FLOWER_BETWEEN_LOW, "../proto/golden/flower_march_images.bin" + 10368, 1152
    lda #0
    sta FLOWER_TURN
@mirror:
    lda #<FLOWER_MIRRORED_IMAGES
    sta FLOWER_IMAGE_BASE
    lda #>FLOWER_MIRRORED_IMAGES
    sta FLOWER_IMAGE_BASE + 1
    ldx #<FLOWER_IMAGES
    ldy #>FLOWER_IMAGES
    lda #FLOWER_SQUARE_PAIR - 2
    sec
    sbc FLOWER_TURN
    bpl @source
    lda #FLOWER_SQUARE_PAIR
@source:
    jsr flower_mirror_pair_from
    lda FLOWER_TURN
    cmp #FLOWER_SQUARE_PAIR - 1
    bcs @next
    cmp #FLOWER_SQUARE_PAIR - 2
    bne @odd_pair
    jsr flower_late_pair
@odd_pair:
    lda #<FLOWER_ODD_MIRRORED_IMAGES
    sta FLOWER_IMAGE_BASE
    lda #>FLOWER_ODD_MIRRORED_IMAGES
    sta FLOWER_IMAGE_BASE + 1
    ldx #<FLOWER_ODD_IMAGES
    ldy #>FLOWER_ODD_IMAGES
    lda #FLOWER_SQUARE_PAIR - 2
    sec
    sbc FLOWER_TURN
    jsr flower_mirror_pair_from
@next:
    inc FLOWER_TURN
    lda FLOWER_TURN
    cmp #FLOWER_SQUARE_PAIR
    bcc @mirror
    assert_if FLOWER_TURN, 9, FLOWER_MIRRORED_IMAGES, "../proto/golden/flower_mirrored_images.bin", 2592
    assert_if FLOWER_TURN, 9, FLOWER_ODD_MIRRORED_IMAGES, "../proto/golden/flower_odd_mirrored_images.bin", 2304
    rts
.endproc

; The record of the turn A: the turn 0 is the morph's last state, the others have their own records
.segment "FLOWER36"


; ─── Store the pair of the marching flower's step FLOWER_J (0-19) of the kind A ────────────────────────
; Stored in the arrays of the steps or of the states between two steps (FLOWER_BETWEEN): the byte of
; flower_march_sources names the rendered tile (bits 0-2), whether the image is that tile mirrored about x = 6
; (bit 6) and whether the pair's two images swap places (bit 7). The odd rows' image is the even rows' image
; mirrored.
.segment "FLOWER37"

.proc flower_march_step
    sta FLOWER_J
    ldx FLOWER_KIND
    jsr flower_march_base
    asl
    sta FLOWER_IMAGE_INDEX
    stx FLOWER_IMAGE_BASE
    sty FLOWER_IMAGE_BASE + 1
    ldx FLOWER_J
    lda FLOWER_KIND
    bne @between
    lda flower_march_sources,x
    jmp @source
@between:
    lda flower_march_sources + FLOWER_MARCH_STEPS,x
@source:
    sta FLOWER_SOURCE
    and #7
    sta FLOWER_TURN_INDEX
    ldx #<FLOWER_TURN_TILES
    ldy #>FLOWER_TURN_TILES
    lda FLOWER_KIND
    beq @base
    ldx #<FLOWER_ODD_TURN_TILES
    ldy #>FLOWER_ODD_TURN_TILES
@base:
    stx FLOWER_TURN_BASE
    sty FLOWER_TURN_BASE + 1
    jsr flower_load_turn
    lda FLOWER_SOURCE
    asl
    lda #0
    rol
    sta FLOWER_SWAPPED
    ora FLOWER_IMAGE_INDEX
    sta FLOWER_RENDERED_SLOT
    lda FLOWER_SWAPPED
    eor #1
    ora FLOWER_IMAGE_INDEX
    sta FLOWER_MIRRORED_SLOT
    lda FLOWER_SOURCE
    and #$40
    bne @quarter
    ldx #<FLOWER_TILE
    ldy #>FLOWER_TILE
    lda FLOWER_RENDERED_SLOT
    jsr flower_store_one
    jmp @mirror
@quarter:
    ldx #<FLOWER_TILE
    ldy #>FLOWER_TILE
    lda FLOWER_MIRRORED_SLOT
    jsr flower_store_one
    lda FLOWER_MIRRORED_SLOT
    jsr flower_image_pointer
    sta FLOWER_PTR
    sty FLOWER_PTR + 1
    lda FLOWER_RENDERED_SLOT
    jsr flower_image_pointer
    sta FLOWER_PTR2
    sty FLOWER_PTR2 + 1
    jsr flower_quarter_image
@mirror:
    lda FLOWER_RENDERED_SLOT
    jsr flower_image_pointer
    sta FLOWER_PTR
    sty FLOWER_PTR + 1
    lda FLOWER_MIRRORED_SLOT
    jsr flower_image_pointer
    sta FLOWER_PTR2
    sty FLOWER_PTR2 + 1
    jmp flower_mirror_image
.endproc

; A, Y = the address of the image A of the array at FLOWER_IMAGE_BASE
.proc flower_image_pointer
    ldx FLOWER_IMAGE_BASE
    ldy FLOWER_IMAGE_BASE + 1
    jmp image_address
.endproc

; Mirrors the pair A of the array at X, Y into the pair FLOWER_TURN of the array at FLOWER_IMAGE_BASE
.proc flower_mirror_pair_from
    asl
    jsr image_address
    sta FLOWER_PTR
    sty FLOWER_PTR + 1
    lda FLOWER_TURN
    asl
    jsr flower_image_pointer
    sta FLOWER_PTR2
    sty FLOWER_PTR2 + 1
    jsr flower_mirror_image
    ldx #0
@advance:
    lda FLOWER_PTR,x
    clc
    adc #IMAGE_BYTES
    sta FLOWER_PTR,x
    bcc @next
    inc FLOWER_PTR + 1,x
@next:
    inx
    inx
    cpx #4
    bne @advance
    beq flower_mirror_image
.endproc

; The image at FLOWER_PTR mirrored into the image at FLOWER_PTR2: the three bytes of each mask (the bytes 0-2 of a
; row of the tile at the row, + 24 and + 48 in the image, the bytes 3-5 at sprite_image_index) swap their ends and are
; bit-reversed
.proc flower_mirror_image
    ldx #TILE_ROWS - 1
@row:
    stx FLOWER_ROW_INDEX
    lda #24
    sta FLOWER_YO
    txa
    jsr flower_mirror_bytes
    lda #1
    sta FLOWER_YO
    ldx FLOWER_ROW_INDEX
    lda sprite_image_index,x
    jsr flower_mirror_bytes
    ldx FLOWER_ROW_INDEX
    dex
    bpl @row
    rts
.endproc

; Mirrors the three bytes at A, A + FLOWER_YO and A + 2 * FLOWER_YO of the image at FLOWER_PTR into the image at
; FLOWER_PTR2
.proc flower_mirror_bytes
    sta FLOWER_XO
    clc
    adc FLOWER_YO
    sta FLOWER_XA
    clc
    adc FLOWER_YO
    sta FLOWER_YA
    ldy FLOWER_XO
    lda (FLOWER_PTR),y
    tax
    lda BIT_REVERSE_TABLE,x
    ldy FLOWER_YA
    sta (FLOWER_PTR2),y
    lda (FLOWER_PTR),y
    tax
    lda BIT_REVERSE_TABLE,x
    ldy FLOWER_XO
    sta (FLOWER_PTR2),y
    ldy FLOWER_XA
    lda (FLOWER_PTR),y
    tax
    lda BIT_REVERSE_TABLE,x
    sta (FLOWER_PTR2),y
    rts
.endproc

; The image at FLOWER_PTR mirrored about x = 6 (the pixel x goes to 11 - x, wrapped at the tile) into the image at
; FLOWER_PTR2: like flower_mirror_image, but the three bytes of a mask b0, b1, b2 become the bit-reversed
; (b0 << 4 | b1 >> 4), (b2 << 4 | b0 >> 4) and (b1 << 4 | b2 >> 4)
.segment "FLOWER38"

.proc flower_quarter_image
    ldx #TILE_ROWS - 1
@row:
    stx FLOWER_ROW_INDEX
    lda #24
    sta FLOWER_YO
    txa
    jsr flower_quarter_bytes
    lda #1
    sta FLOWER_YO
    ldx FLOWER_ROW_INDEX
    lda sprite_image_index,x
    jsr flower_quarter_bytes
    ldx FLOWER_ROW_INDEX
    dex
    bpl @row
    rts
.endproc

.proc flower_quarter_bytes
    sta FLOWER_XO
    clc
    adc FLOWER_YO
    sta FLOWER_XA
    clc
    adc FLOWER_YO
    sta FLOWER_YA
    ldy FLOWER_XO
    lda (FLOWER_PTR),y
    sta FLOWER_ANY
    ldy FLOWER_XA
    lda (FLOWER_PTR),y
    sta FLOWER_ALL
    ldy FLOWER_YA
    lda (FLOWER_PTR),y
    sta FLOWER_FULL
    lda FLOWER_ANY
    ldx FLOWER_ALL
    jsr flower_nibbles
    ldy FLOWER_XO
    sta (FLOWER_PTR2),y
    lda FLOWER_FULL
    ldx FLOWER_ANY
    jsr flower_nibbles
    ldy FLOWER_XA
    sta (FLOWER_PTR2),y
    lda FLOWER_ALL
    ldx FLOWER_FULL
    jsr flower_nibbles
    ldy FLOWER_YA
    sta (FLOWER_PTR2),y
    rts
.endproc

; A = the bit reversal of (A << 4 | X >> 4)
.proc flower_nibbles
    asl
    asl
    asl
    asl
    sta FLOWER_MEDIUM
    txa
    lsr
    lsr
    lsr
    lsr
    ora FLOWER_MEDIUM
    tax
    lda BIT_REVERSE_TABLE,x
    rts
.endproc


; Keeps FLOWER_TILE as the tile of the turn FLOWER_TURN_INDEX
.segment "FLOWER39"

.proc flower_keep_turn
    jsr flower_aim_turn_tile
    lda #<FLOWER_TILE
    ldx #>FLOWER_TILE
    bne flower_keep_from
.endproc

; Copies the tile at X, A to the tile at FLOWER_PTR
.proc flower_keep_from
    sta FLOWER_PTR2
    stx FLOWER_PTR2 + 1
    ldy #0
@byte:
    cpy #TILE_BYTES / 2
    bne @copy
@copy:
    lda (FLOWER_PTR2),y
    sta (FLOWER_PTR),y
    iny
    cpy #TILE_BYTES
    bne @byte
    rts
.endproc

; Loads FLOWER_TILE from the tile of the turn FLOWER_TURN_INDEX
.segment "FLOWER40"

.proc flower_load_turn
    jsr flower_aim_turn_tile
    ldy #0
@byte:
    cpy #TILE_BYTES / 2
    bne @copy
@copy:
    lda (FLOWER_PTR),y
    sta FLOWER_TILE,y
    iny
    cpy #TILE_BYTES
    bne @byte
    rts
.endproc


.segment "FLOWER41"

.proc flower_aim_turn_tile
    lda FLOWER_TURN_INDEX
    ldx FLOWER_TURN_BASE
    ldy FLOWER_TURN_BASE + 1
    jsr image_address
    sta FLOWER_PTR
    sty FLOWER_PTR + 1
    rts
.endproc


; A = the pair of the marching flower's step A (0-19) in the array at X, Y: the steps lie in one array, the states
; between them in two (the first FLOWER_BETWEEN_HIGH_STEPS pairs under the KERNAL, the rest in the free cube RAM)
.segment "FLOWER42"

.proc flower_march_base
    cpx #0
    bne @between
    ldx #<FLOWER_MARCH_IMAGES
    ldy #>FLOWER_MARCH_IMAGES
    rts
@between:
    cmp #FLOWER_BETWEEN_HIGH_STEPS
    bcs @low
    ldx #<FLOWER_BETWEEN_HIGH
    ldy #>FLOWER_BETWEEN_HIGH
    rts
@low:
    sbc #FLOWER_BETWEEN_HIGH_STEPS
    ldx #<FLOWER_BETWEEN_LOW
    ldy #>FLOWER_BETWEEN_LOW
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
