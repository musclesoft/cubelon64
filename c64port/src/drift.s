; drift.s - The diamonds move on diagonally
;
; After the bowties have turned into diamonds, the diamonds keep moving: every kite of a bowtie moves
; diagonally in equidistant steps, the two kites of a bowtie in opposite directions, until each is where
; another one was and the picture is the same again, in 20 states at 50 Hz. proto fits the diamond and
; its speed to the ZX frames (go run -C proto . drift) and renders the states 1-20; the ZX demo shows the
; even ones. The C64 keeps rows 0-11 of each tile (drift_data.inc), rebuilds the other rows by the 180
; degree symmetry and converts the tile and its transpose, the tile of the odd cells, to images (see
; spin.s and tiles.s), which proto/golden/drift_images.bin checks. The images lie on those of the
; split, which are over by then.

.include "include/shared_imports.inc"
.include "include/tile.inc"

DRIFT_LAST_STATE = 20
DRIFT_IMAGES = SPLIT_IMAGES
DRIFT_BASE = DRIFT_IMAGES - 2 * IMAGE_BYTES

.segment "SCRATCH"

DRIFT_BUILD_STATE: .res 1

.segment "CODE"

.import store_pair: absolute
.import show_state_pair: absolute
.import expand_half_tile: absolute
.import SPIN_DATA_PTR: absolute
.import drift_half_tiles: absolute

.export drift_precompute
.export drift_step

; ─── Build the images of all drift states ────────────────────────────────────
.proc drift_precompute
    lda #<drift_half_tiles
    sta SPIN_DATA_PTR
    lda #>drift_half_tiles
    sta SPIN_DATA_PTR + 1
    lda #1
    sta DRIFT_BUILD_STATE
@state:
    jsr expand_half_tile
    lda DRIFT_BUILD_STATE
    ldx #<DRIFT_BASE
    ldy #>DRIFT_BASE
    jsr store_pair
    inc DRIFT_BUILD_STATE
    lda DRIFT_BUILD_STATE
    cmp #DRIFT_LAST_STATE + 1
    bcc @state
    assert_if DRIFT_BUILD_STATE, 21, DRIFT_IMAGES, "../proto/golden/drift_images.bin", 5760
    rts
.endproc

; ─── Show the tiles of state SEQ_SCENE_STATE ─────────────────────────────────
.proc drift_step
    ldx #<DRIFT_BASE
    ldy #>DRIFT_BASE
    jmp show_state_pair
.endproc
