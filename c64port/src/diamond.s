; diamond.s - The bowties of the split turn into diamonds
;
; The animation starts from the large square, the morph's last tile, and goes on from the split: the
; two kites of every bowtie move apart and turn into diamonds, in 20 states at 50 Hz. proto fits one
; formula to the ZX frames, in which the corners of the bowtie's quadrant move linearly (go run -C
; proto . diamond), and renders the states 1-20; the ZX demo shows the even ones. The C64 keeps rows
; 0-11 of each tile (diamond_data.inc), rebuilds the other rows by the 180 degree symmetry and converts
; the tile and its mirror image to images (see tiles.s), which proto/golden/diamond_images.bin checks.
; The even cells of the checkerboard show the mirror image and the odd cells the tile. The images
; lie in four arrays of ten, the tiles of the even and of the odd cells of the even states and then
; those of the odd states, which do not fit in one place.

.include "include/shared_imports.inc"
.include "include/tile.inc"

DIAMOND_LAST_STATE = 20
DIAMOND_ARRAY_IMAGES = 10

.segment "SCRATCH"

DIAMOND_BUILD_STATE: .res 1
DIAMOND_IMAGE: .res 1
DIAMOND_ARRAY: .res 1

.segment "DIAMOND"

DIAMOND_EVEN_STATES_EVEN_CELLS: .res DIAMOND_ARRAY_IMAGES * IMAGE_BYTES
DIAMOND_EVEN_STATES_ODD_CELLS: .res DIAMOND_ARRAY_IMAGES * IMAGE_BYTES
DIAMOND_ODD_STATES_ODD_CELLS: .res DIAMOND_ARRAY_IMAGES * IMAGE_BYTES

.segment "DIAMOND_TILES"

DIAMOND_ODD_STATES_EVEN_CELLS: .res DIAMOND_ARRAY_IMAGES * IMAGE_BYTES

.segment "CODE"

.import store_image: absolute
.import store_mirrored_image: absolute
.import image_address: absolute
.import show_images: absolute
.import expand_half_tile: absolute
.import SPIN_DATA_PTR: absolute
.import SHOW_IMAGE_A: absolute
.import SHOW_IMAGE_B: absolute
.import diamond_half_tiles: absolute

.export diamond_precompute
.export diamond_step

; ─── Build the images of all diamond states ──────────────────────────────────
.proc diamond_precompute
    lda #<diamond_half_tiles
    sta SPIN_DATA_PTR
    lda #>diamond_half_tiles
    sta SPIN_DATA_PTR + 1
    lda #1
    sta DIAMOND_BUILD_STATE
@state:
    jsr expand_half_tile
    lda DIAMOND_BUILD_STATE
    jsr aim_state
    ldx DIAMOND_ARRAY
    inx
    lda DIAMOND_IMAGE
    jsr array_base
    jsr store_image
    ldx DIAMOND_ARRAY
    lda DIAMOND_IMAGE
    jsr array_base
    jsr store_mirrored_image
    inc DIAMOND_BUILD_STATE
    lda DIAMOND_BUILD_STATE
    cmp #DIAMOND_LAST_STATE + 1
    bcc @state
    assert_if DIAMOND_BUILD_STATE, 21, DIAMOND_EVEN_STATES_EVEN_CELLS, "../proto/golden/diamond_images.bin", 1440
    assert_if DIAMOND_BUILD_STATE, 21, DIAMOND_EVEN_STATES_ODD_CELLS, "../proto/golden/diamond_images.bin" + 1440, 1440
    assert_if DIAMOND_BUILD_STATE, 21, DIAMOND_ODD_STATES_EVEN_CELLS, "../proto/golden/diamond_images.bin" + 2880, 1440
    assert_if DIAMOND_BUILD_STATE, 21, DIAMOND_ODD_STATES_ODD_CELLS, "../proto/golden/diamond_images.bin" + 4320, 1440
    rts
.endproc

; ─── Show the tiles of state SEQ_SCENE_STATE ─────────────────────────────────
.proc diamond_step
    lda SEQ_SCENE_STATE
    jsr aim_state
    ldx DIAMOND_ARRAY
    lda DIAMOND_IMAGE
    jsr array_base
    jsr image_address
    sta SHOW_IMAGE_A
    sty SHOW_IMAGE_A + 1
    ldx DIAMOND_ARRAY
    inx
    lda DIAMOND_IMAGE
    jsr array_base
    jsr image_address
    sta SHOW_IMAGE_B
    sty SHOW_IMAGE_B + 1
    jmp show_images
.endproc

; ─── Where the images of the state A (1-20) lie ──────────────────────────────
; DIAMOND_ARRAY is the array of the even cells' images, the next one the odd cells', and
; DIAMOND_IMAGE the number of the image in it: the states 1 and 2 are image 0, 3 and 4 image 1, ...
.proc aim_state
    pha
    and #1
    asl
    sta DIAMOND_ARRAY
    pla
    sec
    sbc #1
    lsr
    sta DIAMOND_IMAGE
    rts
.endproc

; ─── The base of the array of images X: X low, Y high, A kept ───────────────
.proc array_base
    pha
    lda array_high,x
    tay
    lda array_low,x
    tax
    pla
    rts
.endproc

array_low:
    .byte <DIAMOND_EVEN_STATES_EVEN_CELLS, <DIAMOND_EVEN_STATES_ODD_CELLS
    .byte <DIAMOND_ODD_STATES_EVEN_CELLS, <DIAMOND_ODD_STATES_ODD_CELLS
array_high:
    .byte >DIAMOND_EVEN_STATES_EVEN_CELLS, >DIAMOND_EVEN_STATES_ODD_CELLS
    .byte >DIAMOND_ODD_STATES_EVEN_CELLS, >DIAMOND_ODD_STATES_ODD_CELLS
