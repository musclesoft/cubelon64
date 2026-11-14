; diamond.s - The bowties of the split turn into diamonds
;
; The animation starts from the large square, the morph's last tile, and goes on from the split: the
; two kites of every bowtie move apart and turn into diamonds, in 20 states at 50 Hz. proto fits one
; formula to the ZX frames, in which the corners of the bowtie's quadrant move linearly (go run -C
; proto . diamond), and renders the states 1-20; the ZX demo shows the even ones. The C64 keeps the top left
; quarter of each tile (diamond_data.inc), which has quarter turn symmetry, turns it into the top right
; quarter, rebuilds the lower half by the half turn, and converts the tile and its transpose to images
; (see spin.s and tiles.s), which proto/golden/diamond_images.bin checks. The tile is for the even cells of
; the checkerboard, its transpose for the odd cells: the images of state N are image 2 * N - 2 and
; 2 * N - 1 of DIAMOND_IMAGES (image 2 * N and 2 * N + 1 of DIAMOND_BASE), which lie on the spin's
; images, over by the time the job builds them.

.include "include/shared_imports.inc"
.include "include/tile.inc"

DIAMOND_LAST_STATE = 20
DIAMOND_IMAGES = SPIN_IMAGES
DIAMOND_BASE = DIAMOND_IMAGES - 2 * IMAGE_BYTES

.segment "SCRATCH"

DIAMOND_BUILD_STATE: .res 1

.segment "CODE"

.import store_pair: absolute
.import show_state_pair: absolute
.import split_step: absolute
.import fill_lower_half: absolute
.import job_checkpoint: absolute
.import transpose_tile: absolute
.import mirror_column: absolute
.import BIT_REVERSE_TABLE: absolute
.import TILE_SRC_PTR: absolute
.import SPIN_DATA_PTR: absolute
.import SPIN_MASK_OFFSET: absolute
.import SPIN_BLOCK: absolute
.import SPIN_TILE: absolute
.import SPIN_TRANSPOSED: absolute
.import diamond_quarter_tiles: absolute

.export diamond_precompute
.export expand_quarter_tile
.export diamond_step

; ─── Build the images of all diamond states ──────────────────────────────────
.proc diamond_precompute
    lda #<diamond_quarter_tiles
    sta SPIN_DATA_PTR
    lda #>diamond_quarter_tiles
    sta SPIN_DATA_PTR + 1
    lda #1
    sta DIAMOND_BUILD_STATE
@state:
    jsr expand_quarter_tile
    lda DIAMOND_BUILD_STATE
    ldx #<DIAMOND_BASE
    ldy #>DIAMOND_BASE
    clc
    jsr store_pair
    inc DIAMOND_BUILD_STATE
    lda DIAMOND_BUILD_STATE
    cmp #DIAMOND_LAST_STATE + 1
    bcc @state
    assert_if DIAMOND_BUILD_STATE, 21, DIAMOND_IMAGES, "../proto/golden/diamond_images.bin", 5760
    rts
.endproc

; ─── Build the tile in SPIN_TILE from the quarter at SPIN_DATA_PTR ───────────
; A set bit of a mask is a pixel that is not covered. The 12 rows of 3 bytes are unpacked into the
; rows of a tile that is empty elsewhere, the transposed tile mirrored is the top right quarter, and
; the half turn makes the lower half.
.proc expand_quarter_tile
    ldy #TILE_BYTES
    lda #$FF
@empty:
    sta SPIN_TILE - 1,y
    dey
    bne @empty
    ldx #0
    ldy #0
@unpack:
    jsr job_checkpoint
    lda (SPIN_DATA_PTR),y
    sta SPIN_TILE,x
    iny
    lda (SPIN_DATA_PTR),y
    sta SPIN_TILE + 3,x
    iny
    lda (SPIN_DATA_PTR),y
    iny
    pha
    ora #$0F
    sta SPIN_TILE + 1,x
    pla
    asl
    asl
    asl
    asl
    ora #$0F
    sta SPIN_TILE + 4,x
    txa
    clc
    adc #TILE_ROW_BYTES
    tax
    cpx #HALF_TILE_BYTES
    bne @unpack
    tya
    clc
    adc SPIN_DATA_PTR
    sta SPIN_DATA_PTR
    bcc @quarter_read
    inc SPIN_DATA_PTR + 1
@quarter_read:

    jsr transpose_tile
    lda #0
    sta SPIN_MASK_OFFSET
@mirror_row:
    jsr job_checkpoint
    ldx #TILE_ROW_BYTES - 1
@mirror_byte:
    lda mirror_column,x
    clc
    adc SPIN_MASK_OFFSET
    tay
    lda SPIN_TRANSPOSED,y
    tay
    lda BIT_REVERSE_TABLE,y
    sta SPIN_BLOCK
    txa
    clc
    adc SPIN_MASK_OFFSET
    tay
    lda SPIN_BLOCK
    and SPIN_TILE,y
    sta SPIN_TILE,y
    dex
    bpl @mirror_byte
    lda SPIN_MASK_OFFSET
    clc
    adc #TILE_ROW_BYTES
    sta SPIN_MASK_OFFSET
    cmp #HALF_TILE_BYTES
    bne @mirror_row

    lda #<SPIN_TILE
    sta TILE_SRC_PTR
    lda #>SPIN_TILE
    sta TILE_SRC_PTR + 1
    jmp fill_lower_half
.endproc

; ─── Show the tiles of state SEQ_SCENE_STATE ─────────────────────────────────
; The backwards animation ends with the state 0, the large square, which is the morph's last tile. In
; the second run of the ZX demo, with the tiles mirrored, the diamonds' tile and its transpose, which is its
; mirror image, swap their places.
.proc diamond_step
    lda SEQ_SCENE_STATE
    beq @squares
    lda SEQ_EVENT
    cmp #EVENT_DIAMONDS_MIRRORED
    ldx #<DIAMOND_BASE
    ldy #>DIAMOND_BASE
    jmp show_state_pair
@squares:
    jmp split_step
.endproc
