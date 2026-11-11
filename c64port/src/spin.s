; spin.s - The bowties spin a quarter turn
;
; After the split every tile corner is the centre of a bowtie of two kites, and the bowties
; spin 90 degrees in 20 states of 4.5 degrees: the upright ones one way and the lying ones
; the other, so the neighbour of a tile is its transpose and not its mirror image. A
; tile has 180 degree symmetry, and the tile of state 20 - N is the transpose of the tile
; of state N. proto renders the states 0-10 from the rotation of the split's bowtie and fits
; them to the ZX frames; the C64 keeps rows 0-11 of each (spin_data.inc), rebuilds the
; other rows by the symmetry and the states 11-20 by transposing. Tile N lives in page N of
; SPIN_TILES. A spin step shows the tile of state N and that of state 20 - N.

.include "include/shared_imports.inc"
.include "include/tile.inc"

HALF_TILE_STATES = 11
HALF_TILE_DATA_BYTES = 12 * TILE_ROW_BYTES
LAST_ROW_OFFSET_SPIN = TILE_BYTES - TILE_ROW_BYTES
MASK_BYTES = 3
SPIN_PAGES = SPIN_STATES + 1

.segment "ZEROPAGE"

SPIN_DATA_PTR: .res 2
SPIN_TILE_PTR: .res 2
SPIN_FROM_PTR: .res 2

.segment "BSS"

SPIN_STATE: .res 1
SPIN_ROW_OFFSET: .res 1
SPIN_MIRROR_OFFSET: .res 1
SPIN_MASK_OFFSET: .res 1
SPIN_ROW: .res 1
SPIN_COLUMNS_LEFT: .res 1
SPIN_ROW_BYTES: .res MASK_BYTES
SPIN_STAGE: .res TILE_BYTES

.segment "SPIN"

.export SPIN_TILES
SPIN_TILES: .res SPIN_PAGES * 256
SPIN_FIRST_PAGE = SPIN_TILES / 256

.segment "CODE"

.import show_tile_pair: absolute
.import mirror_column: absolute
.import BIT_REVERSE_TABLE: absolute
.import spin_half_tiles: absolute

.export spin_precompute
.export spin_step

; ─── Build the tiles of all spin states ──────────────────────────────────────
.proc spin_precompute
    lda #<spin_half_tiles
    sta SPIN_DATA_PTR
    lda #>spin_half_tiles
    sta SPIN_DATA_PTR + 1
    lda #0
    sta SPIN_STATE
@half:
    jsr expand_half_tile
    inc SPIN_STATE
    lda SPIN_STATE
    cmp #HALF_TILE_STATES
    bcc @half
@transposed:
    jsr transpose_into_state
    inc SPIN_STATE
    lda SPIN_STATE
    cmp #SPIN_STATES + 1
    bcc @transposed
    assert_if SPIN_STATE, 21, SPIN_TILES, "../proto/golden/spin_tiles.bin", 5376
    rts
.endproc

; ─── Build the tile of state SPIN_STATE from its rows 0-11 at SPIN_DATA_PTR ──
; Row 23 - r is row r turned by 180 degrees: its bytes in reverse order, bit-reversed.
.proc expand_half_tile
    lda SPIN_STATE
    clc
    adc #SPIN_FIRST_PAGE
    sta SPIN_TILE_PTR + 1
    lda #0
    sta SPIN_TILE_PTR
    ldy #HALF_TILE_DATA_BYTES - 1
@copy:
    lda (SPIN_DATA_PTR),y
    sta (SPIN_TILE_PTR),y
    dey
    bpl @copy

    lda SPIN_DATA_PTR
    clc
    adc #HALF_TILE_DATA_BYTES
    sta SPIN_DATA_PTR
    bcc @pointer_ready
    inc SPIN_DATA_PTR + 1
@pointer_ready:
    lda #0
    sta SPIN_ROW_OFFSET
    lda #LAST_ROW_OFFSET_SPIN
    sta SPIN_MIRROR_OFFSET
@row:
    ldx #TILE_ROW_BYTES - 1
@byte:
    lda mirror_column,x
    clc
    adc SPIN_ROW_OFFSET
    tay
    lda (SPIN_TILE_PTR),y
    tay
    lda BIT_REVERSE_TABLE,y
    pha
    txa
    clc
    adc SPIN_MIRROR_OFFSET
    tay
    pla
    sta (SPIN_TILE_PTR),y
    dex
    bpl @byte

    lda SPIN_ROW_OFFSET
    clc
    adc #TILE_ROW_BYTES
    sta SPIN_ROW_OFFSET
    lda SPIN_MIRROR_OFFSET
    sec
    sbc #TILE_ROW_BYTES
    sta SPIN_MIRROR_OFFSET
    lda SPIN_ROW_OFFSET
    cmp #HALF_TILE_DATA_BYTES
    bne @row
    rts
.endproc

; ─── Build the tile of state SPIN_STATE as the transpose of the tile of 20 - SPIN_STATE ─
; Row r of the source gives bit r of every row of the result: its 24 bits shift out left to
; right, one into each result row, which collects the bits of 8 source rows per byte.
.proc transpose_into_state
    lda #SPIN_FIRST_PAGE + SPIN_STATES
    sec
    sbc SPIN_STATE
    sta SPIN_FROM_PTR + 1
    lda #0
    sta SPIN_FROM_PTR
    ldx #TILE_BYTES - 1
@clear:
    sta SPIN_STAGE,x
    dex
    cpx #$FF
    bne @clear

    lda #0
    sta SPIN_MASK_OFFSET
@mask:
    lda #0
    sta SPIN_ROW
@row:
    jsr load_row_bytes
    lda SPIN_ROW
    lsr
    lsr
    lsr
    clc
    adc SPIN_MASK_OFFSET
    tax
    lda #TILE_ROWS
    sta SPIN_COLUMNS_LEFT
@column:
    asl SPIN_ROW_BYTES + 2
    rol SPIN_ROW_BYTES + 1
    rol SPIN_ROW_BYTES
    rol SPIN_STAGE,x
    txa
    clc
    adc #TILE_ROW_BYTES
    tax
    dec SPIN_COLUMNS_LEFT
    bne @column
    inc SPIN_ROW
    lda SPIN_ROW
    cmp #TILE_ROWS
    bne @row
    lda SPIN_MASK_OFFSET
    clc
    adc #MASK_BYTES
    sta SPIN_MASK_OFFSET
    cmp #2 * MASK_BYTES
    bne @mask

    lda SPIN_STATE
    clc
    adc #SPIN_FIRST_PAGE
    sta SPIN_TILE_PTR + 1
    lda #0
    sta SPIN_TILE_PTR
    ldy #TILE_BYTES - 1
@store:
    lda SPIN_STAGE,y
    sta (SPIN_TILE_PTR),y
    dey
    cpy #$FF
    bne @store
    rts
.endproc

; ─── Load the 3 bytes of the current mask of the source row SPIN_ROW ─────────
.proc load_row_bytes
    lda SPIN_ROW
    asl
    sta SPIN_ROW_OFFSET
    asl
    clc
    adc SPIN_ROW_OFFSET
    sta SPIN_ROW_OFFSET
    lda SPIN_ROW_OFFSET
    clc
    adc SPIN_MASK_OFFSET
    tay
    lda (SPIN_FROM_PTR),y
    sta SPIN_ROW_BYTES
    iny
    lda (SPIN_FROM_PTR),y
    sta SPIN_ROW_BYTES + 1
    iny
    lda (SPIN_FROM_PTR),y
    sta SPIN_ROW_BYTES + 2
    rts
.endproc

; ─── Show the tiles of state SEQ_SCENE_STATE and of 20 - SEQ_SCENE_STATE ─────
.proc spin_step
    lda #SPIN_FIRST_PAGE + SPIN_STATES
    sec
    sbc SEQ_SCENE_STATE
    tax
    lda SEQ_SCENE_STATE
    clc
    adc #SPIN_FIRST_PAGE
    jmp show_tile_pair
.endproc
