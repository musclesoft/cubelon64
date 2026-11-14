; spin.s - The bowties spin a quarter turn
;
; After the split every tile corner is the centre of a bowtie of two kites, and the bowties
; spin 90 degrees in 20 states of 4.5 degrees: the upright ones one way and the lying ones
; the other, so the neighbour of a tile is its transpose and not its mirror image. A
; tile has 180 degree symmetry, and the tile of state 20 - N is the transpose of the tile
; of state N. proto renders the states 0-10 from the rotation of the split's bowtie and fits
; them to the ZX frames; the C64 keeps rows 0-11 of each (spin_data.inc), rebuilds the
; other rows by the symmetry and the tile of state 20 - N by transposing, and converts
; every tile to an image (see tiles.s): image N of SPIN_IMAGES, which
; proto/golden/spin_images.bin checks. A spin step shows the images of state N and 20 - N.

.include "include/shared_imports.inc"
.include "include/tile.inc"

HALF_TILE_STATES = 11
MASK_BYTES = 3
BLOCKS_PER_MASK = 9
SPIN_IMAGE_COUNT = SPIN_STATES + 1

.segment "ZEROPAGE"

SPIN_DATA_PTR: .res 2
SPIN_TRANSPOSE_ROWS: .res 8
SPIN_MASK_OFFSET: .res 1
SPIN_BLOCK: .res 1

.segment "BSS"

SPIN_IMAGE_BASE: .res 2

.segment "SCRATCH"

SPIN_STATE: .res 1
PAIR_STATE: .res 1
PAIR_BASE: .res 2

.segment "SCRATCH"

SPIN_TILE: .res TILE_BYTES
SPIN_TRANSPOSED: .res TILE_BYTES

.segment "SPIN"

SPIN_IMAGES: .res SPIN_IMAGE_COUNT * IMAGE_BYTES
SPIN_MIRRORED_IMAGES: .res SPIN_IMAGE_COUNT * IMAGE_BYTES

.segment "CODE"

.import store_image: absolute
.import store_mirrored_image: absolute
.import fill_lower_half: absolute
.import job_checkpoint: absolute
.import image_address: absolute
.import show_images: absolute
.import TILE_SRC_PTR: absolute
.import SHOW_IMAGE_A: absolute
.import SHOW_IMAGE_B: absolute
.import mirror_column: absolute
.import BIT_REVERSE_TABLE: absolute
.import spin_half_tiles: absolute

.export spin_precompute
.export expand_half_tile
.export transpose_tile
.export store_pair
.export SPIN_DATA_PTR
.export SPIN_MASK_OFFSET
.export SPIN_BLOCK
.export SPIN_TILE
.export SPIN_TRANSPOSED
.export spin_step
.export spin_step_mirrored

; ─── Build the images of all spin states ─────────────────────────────────────
; State N comes from its data and state 20 - N from its transpose
.proc spin_precompute
    lda #<spin_half_tiles
    sta SPIN_DATA_PTR
    lda #>spin_half_tiles
    sta SPIN_DATA_PTR + 1
    lda #0
    sta SPIN_STATE
@state:
    jsr expand_half_tile
    lda SPIN_STATE
    jsr store_state_image
    lda SPIN_STATE
    cmp #HALF_TILE_STATES - 1
    beq @next
    jsr transpose_tile
    lda #<SPIN_TRANSPOSED
    sta TILE_SRC_PTR
    lda #>SPIN_TRANSPOSED
    sta TILE_SRC_PTR + 1
    lda #SPIN_STATES
    sec
    sbc SPIN_STATE
    jsr store_state_image
@next:
    inc SPIN_STATE
    lda SPIN_STATE
    cmp #HALF_TILE_STATES
    bcc @state
    assert_if SPIN_STATE, 11, SPIN_IMAGES, "../proto/golden/spin_images.bin", 3024
    rts
.endproc

.proc store_state_image
    pha
    ldx #<SPIN_IMAGES
    ldy #>SPIN_IMAGES
    jsr store_image
    pla
    ldx #<SPIN_MIRRORED_IMAGES
    ldy #>SPIN_MIRRORED_IMAGES
    jmp store_mirrored_image
.endproc

; ─── Build the tile of state SPIN_STATE in SPIN_TILE from its rows 0-11 at SPIN_DATA_PTR ──
; Leaves TILE_SRC_PTR at the tile.
.proc expand_half_tile
    lda #<SPIN_TILE
    sta TILE_SRC_PTR
    lda #>SPIN_TILE
    sta TILE_SRC_PTR + 1
    ldy #HALF_TILE_BYTES - 1
@copy:
    lda (SPIN_DATA_PTR),y
    sta (TILE_SRC_PTR),y
    dey
    bpl @copy

    lda SPIN_DATA_PTR
    clc
    adc #HALF_TILE_BYTES
    sta SPIN_DATA_PTR
    bcc @pointer_ready
    inc SPIN_DATA_PTR + 1
@pointer_ready:
    jmp fill_lower_half
.endproc

; ─── Transpose SPIN_TILE into SPIN_TRANSPOSED ────────────────────────────────
; Each of the 9 blocks of a mask (3 bytes per tile row) is 8 rows of one byte: its bits shift out
; left to right, one column per step, and every step collects the 8 bits into a byte of the
; block of the result that lies mirrored on the diagonal.
.proc transpose_tile
    lda #0
    sta SPIN_MASK_OFFSET
@mask:
    ldx #BLOCKS_PER_MASK - 1
@block:
    stx SPIN_BLOCK
    jsr job_checkpoint
    lda transpose_source,x
    clc
    adc SPIN_MASK_OFFSET
    tax
    ldy #0
@load:
    lda SPIN_TILE,x
    sta SPIN_TRANSPOSE_ROWS,y
    txa
    clc
    adc #TILE_ROW_BYTES
    tax
    iny
    cpy #8
    bne @load
    ldx SPIN_BLOCK
    lda transpose_target,x
    clc
    adc SPIN_MASK_OFFSET
    tay
    ldx #8
@column:
    asl SPIN_TRANSPOSE_ROWS + 0
    rol
    asl SPIN_TRANSPOSE_ROWS + 1
    rol
    asl SPIN_TRANSPOSE_ROWS + 2
    rol
    asl SPIN_TRANSPOSE_ROWS + 3
    rol
    asl SPIN_TRANSPOSE_ROWS + 4
    rol
    asl SPIN_TRANSPOSE_ROWS + 5
    rol
    asl SPIN_TRANSPOSE_ROWS + 6
    rol
    asl SPIN_TRANSPOSE_ROWS + 7
    rol
    sta SPIN_TRANSPOSED,y
    tya
    clc
    adc #TILE_ROW_BYTES
    tay
    dex
    bne @column
    ldx SPIN_BLOCK
    dex
    bpl @block
    lda SPIN_MASK_OFFSET
    clc
    adc #MASK_BYTES
    sta SPIN_MASK_OFFSET
    cmp #2 * MASK_BYTES
    bne @mask
    rts
.endproc

; ─── Store SPIN_TILE and its transpose as the pair of images of the state A of a scene ─
; The tile is for the even cells of the checkerboard and its transpose for the odd cells. The pair of
; the state N is image 2 * N and 2 * N + 1 of the array at X (low), Y (high); see show_state_pair.
.proc store_pair
    sta PAIR_STATE
    stx PAIR_BASE
    sty PAIR_BASE + 1
    asl
    jsr store_image
    jsr transpose_tile
    lda #<SPIN_TRANSPOSED
    sta TILE_SRC_PTR
    lda #>SPIN_TRANSPOSED
    sta TILE_SRC_PTR + 1
    lda PAIR_STATE
    asl
    ora #1
    ldx PAIR_BASE
    ldy PAIR_BASE + 1
    jmp store_image
.endproc

; Offsets in the tile of the first byte of each block of 8 rows, and in the result of the
; block it turns into: 48 * block row + column byte, and 48 * column byte + block row
transpose_source:
    .byte 0, 1, 2, 48, 49, 50, 96, 97, 98
transpose_target:
    .byte 0, 48, 96, 1, 49, 97, 2, 50, 98

; ─── Show the images of state SEQ_SCENE_STATE and of 20 - SEQ_SCENE_STATE ────
.proc spin_step
    ldx #<SPIN_IMAGES
    ldy #>SPIN_IMAGES
    jmp show_spin
.endproc

; ─── The same with every tile mirrored: the quarter turn after the first ─────
.proc spin_step_mirrored
    ldx #<SPIN_MIRRORED_IMAGES
    ldy #>SPIN_MIRRORED_IMAGES
    jmp show_spin
.endproc

.proc show_spin
    stx SPIN_IMAGE_BASE
    sty SPIN_IMAGE_BASE + 1
    lda SEQ_SCENE_STATE
    jsr image_address
    sta SHOW_IMAGE_A
    sty SHOW_IMAGE_A + 1
    lda #SPIN_STATES
    sec
    sbc SEQ_SCENE_STATE
    ldx SPIN_IMAGE_BASE
    ldy SPIN_IMAGE_BASE + 1
    jsr image_address
    sta SHOW_IMAGE_B
    sty SHOW_IMAGE_B + 1
    jmp show_images
.endproc
