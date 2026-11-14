; merge.s - The large square divides into four squares that merge into the end square
;
; The last scene of the animation: every corner square of the large square divides into four squares, one
; for each of the cells around the corner, and in every cell the four of them shrink and move around the
; centre of the cell, until they are a pixel large, when four squares start to grow in the cell and merge into
; the square of the marching cubes' last picture, in 20 states at 50 Hz. proto draws the squares from their
; centres and sizes, which move linearly (go run -C proto . merge), and renders the states 1-20; the ZX demo
; shows the even ones. All cells show the same tile, which has quarter turn symmetry; the C64 keeps its top
; left quarter (merge_data.inc), builds the tile as the diamonds' (see diamond.s) and converts it to the image
; of its state, which proto/golden/merge_images.bin checks. The images lie on those of the split, which are
; over by then.

.include "include/shared_imports.inc"
.include "include/tile.inc"

MERGE_LAST_STATE = 20
MERGE_IMAGES = SPLIT_IMAGES
MERGE_BASE = MERGE_IMAGES - IMAGE_BYTES

.segment "SCRATCH"

MERGE_BUILD_STATE: .res 1

.segment "CODE"

.import store_image: absolute
.import image_address: absolute
.import show_images: absolute
.import expand_quarter_tile: absolute
.import SPIN_DATA_PTR: absolute
.import SHOW_IMAGE_A: absolute
.import SHOW_IMAGE_B: absolute
.import merge_quarter_tiles: absolute

.export merge_precompute
.export merge_step

; ─── Build the images of all merge states ────────────────────────────────────
.proc merge_precompute
    lda #<merge_quarter_tiles
    sta SPIN_DATA_PTR
    lda #>merge_quarter_tiles
    sta SPIN_DATA_PTR + 1
    lda #1
    sta MERGE_BUILD_STATE
@state:
    jsr expand_quarter_tile
    lda MERGE_BUILD_STATE
    ldx #<MERGE_BASE
    ldy #>MERGE_BASE
    jsr store_image
    inc MERGE_BUILD_STATE
    lda MERGE_BUILD_STATE
    cmp #MERGE_LAST_STATE + 1
    bcc @state
    assert_if MERGE_BUILD_STATE, 21, MERGE_IMAGES, "../proto/golden/merge_images.bin", 2880
    rts
.endproc

; ─── Show the tile of state SEQ_SCENE_STATE in all cells ─────────────────────
.proc merge_step
    lda SEQ_SCENE_STATE
    ldx #<MERGE_BASE
    ldy #>MERGE_BASE
    jsr image_address
    sta SHOW_IMAGE_A
    sta SHOW_IMAGE_B
    sty SHOW_IMAGE_A + 1
    sty SHOW_IMAGE_B + 1
    jmp show_images
.endproc
