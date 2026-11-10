; split.s - The large square splits into two kites
;
; After the morph the grid is a checkerboard: every other tile is the mirror image of
; its neighbours, so the squares split into kites that point up and down in one
; tile and left and right in the next. Each tile corner is the centre of a bowtie of
; two kites, standing at the top left and bottom right corner and lying at the
; top right and bottom left corner. A bowtie is symmetric, so one quadrant of it
; (O=(0,0) W=(waist,0) B=(wide,wide_y) T=(0,tip), see split_table.inc) renders all of it:
; the cube rasterizer renders the standing and the transposed lying quadrant of every
; state at init, and they are mirrored into the four corners of the tile. A tile has the
; format of the morph tiles and tile N lives in page N-1 of SPLIT_TILES. A split
; step copies one tile to the charset and the sprites.

.include "include/shared_imports.inc"
.include "include/tile.inc"

SPLIT_SHIFT_WORD = $0800
SPLIT_Y_SHIFT_WORD = SPLIT_SHIFT_WORD - Y_SAMPLE_BIAS
LAST_ROW_OFFSET_SPLIT = TILE_BYTES - TILE_ROW_BYTES

SCREEN = $4000
SCREEN_C = $4800
GRID_FIRST_COLUMN = 8
GRID_ORIGIN = SCREEN + GRID_FIRST_COLUMN
GRID_ORIGIN_C = SCREEN_C + GRID_FIRST_COLUMN
SCREEN_ROWS = 24
SCREEN_ROW_BYTES = 40
ODD_TILE_CHAR_COLUMNS = 12

STANDING = 0
LYING = 4
SPLIT_WORD_ZERO = 0
SPLIT_WORD_WAIST = 2
SPLIT_WORD_WIDE = 4
SPLIT_WORD_WIDE_Y = 6
SPLIT_WORD_TIP = 8

.segment "ZEROPAGE"

SPLIT_TILE_PTR: .res 2
SPLIT_QUAD_PTR: .res 2
SPLIT_SCREEN_PTR: .res 2
SPLIT_SCREEN_C_PTR: .res 2

.segment "BSS"

SPLIT_BUILD_STATE: .res 1
SPLIT_WORDS: .res 10
SPLIT_QUAD: .res 1
SPLIT_CORNER: .res 1
SPLIT_CORNER_OFFSET: .res 1
SPLIT_ROW_OFFSET: .res 1
SPLIT_MIRROR_OFFSET: .res 1
SPLIT_ACCUMULATOR: .res 1
SPLIT_SCREEN_ROW: .res 1
SPLIT_ROW_CHAR: .res 1
SPLIT_STANDING: .res TILE_BYTES
SPLIT_LYING: .res TILE_BYTES

.segment "SPLIT"

.export SPLIT_TILES
SPLIT_TILES: .res SPLIT_STATES * 256

.segment "CODE"

.import render_quad: absolute
.import show_tile: absolute
.import load_petal_row: absolute
.import mirror_column: absolute
.import sprite_checker_pointers: absolute
.import BIT_REVERSE_TABLE: absolute
.import MORPH_ROW: absolute
.import MORPH_ROW_INDEX: absolute

.export split_precompute
.export split_step
.export checker_init

; ─── Build the tiles of all split states ─────────────────────────────────────
.proc split_precompute
    lda #PETAL_SLOT
    sta FRAME
    lda #1
    sta SPLIT_BUILD_STATE
@state:
    jsr load_state_words
    lda #STANDING
    jsr render_quadrant
    lda #<SPLIT_STANDING
    sta SPLIT_QUAD_PTR
    lda #>SPLIT_STANDING
    sta SPLIT_QUAD_PTR + 1
    jsr unshift_petal
    lda #LYING
    jsr render_quadrant
    lda #<SPLIT_LYING
    sta SPLIT_QUAD_PTR
    lda #>SPLIT_LYING
    sta SPLIT_QUAD_PTR + 1
    jsr unshift_petal
    jsr mirror_into_tile
    assert_if SPLIT_BUILD_STATE, 1, SPLIT_TILES, "../proto/golden/split_tiles.bin", 144
    assert_if SPLIT_BUILD_STATE, 2, SPLIT_TILES + 256, "../proto/golden/split_tiles.bin" + 144, 144
    assert_if SPLIT_BUILD_STATE, 3, SPLIT_TILES + 512, "../proto/golden/split_tiles.bin" + 288, 144
    assert_if SPLIT_BUILD_STATE, 4, SPLIT_TILES + 768, "../proto/golden/split_tiles.bin" + 432, 144
    assert_if SPLIT_BUILD_STATE, 5, SPLIT_TILES + 1024, "../proto/golden/split_tiles.bin" + 576, 144
    assert_if SPLIT_BUILD_STATE, 6, SPLIT_TILES + 1280, "../proto/golden/split_tiles.bin" + 720, 144
    assert_if SPLIT_BUILD_STATE, 7, SPLIT_TILES + 1536, "../proto/golden/split_tiles.bin" + 864, 144
    assert_if SPLIT_BUILD_STATE, 8, SPLIT_TILES + 1792, "../proto/golden/split_tiles.bin" + 1008, 144
    assert_if SPLIT_BUILD_STATE, 9, SPLIT_TILES + 2048, "../proto/golden/split_tiles.bin" + 1152, 144
    assert_if SPLIT_BUILD_STATE, 10, SPLIT_TILES + 2304, "../proto/golden/split_tiles.bin" + 1296, 144
    assert_if SPLIT_BUILD_STATE, 11, SPLIT_TILES + 2560, "../proto/golden/split_tiles.bin" + 1440, 144
    assert_if SPLIT_BUILD_STATE, 12, SPLIT_TILES + 2816, "../proto/golden/split_tiles.bin" + 1584, 144
    assert_if SPLIT_BUILD_STATE, 13, SPLIT_TILES + 3072, "../proto/golden/split_tiles.bin" + 1728, 144
    assert_if SPLIT_BUILD_STATE, 14, SPLIT_TILES + 3328, "../proto/golden/split_tiles.bin" + 1872, 144
    assert_if SPLIT_BUILD_STATE, 15, SPLIT_TILES + 3584, "../proto/golden/split_tiles.bin" + 2016, 144
    assert_if SPLIT_BUILD_STATE, 16, SPLIT_TILES + 3840, "../proto/golden/split_tiles.bin" + 2160, 144
    assert_if SPLIT_BUILD_STATE, 17, SPLIT_TILES + 4096, "../proto/golden/split_tiles.bin" + 2304, 144
    assert_if SPLIT_BUILD_STATE, 18, SPLIT_TILES + 4352, "../proto/golden/split_tiles.bin" + 2448, 144
    assert_if SPLIT_BUILD_STATE, 19, SPLIT_TILES + 4608, "../proto/golden/split_tiles.bin" + 2592, 144
    assert_if SPLIT_BUILD_STATE, 20, SPLIT_TILES + 4864, "../proto/golden/split_tiles.bin" + 2736, 144
    inc SPLIT_BUILD_STATE
    lda SPLIT_BUILD_STATE
    cmp #SPLIT_STATES + 1
    bcc @state
    lda #0
    sta FRAME
    rts
.endproc

; ─── Load the quadrant words of state SPLIT_BUILD_STATE into SPLIT_WORDS ─────
.proc load_state_words
    lda SPLIT_BUILD_STATE
    asl
    tax
    dex
    dex
    lda #0
    sta SPLIT_WORDS + SPLIT_WORD_ZERO
    sta SPLIT_WORDS + SPLIT_WORD_ZERO + 1
    lda split_waist_table,x
    sta SPLIT_WORDS + SPLIT_WORD_WAIST
    lda split_waist_table + 1,x
    sta SPLIT_WORDS + SPLIT_WORD_WAIST + 1
    lda split_wide_table,x
    sta SPLIT_WORDS + SPLIT_WORD_WIDE
    lda split_wide_table + 1,x
    sta SPLIT_WORDS + SPLIT_WORD_WIDE + 1
    lda split_wide_y_table,x
    sta SPLIT_WORDS + SPLIT_WORD_WIDE_Y
    lda split_wide_y_table + 1,x
    sta SPLIT_WORDS + SPLIT_WORD_WIDE_Y + 1
    lda split_tip_table,x
    sta SPLIT_WORDS + SPLIT_WORD_TIP
    lda split_tip_table + 1,x
    sta SPLIT_WORDS + SPLIT_WORD_TIP + 1
    rts
.endproc

; ─── Render the standing (A = STANDING) or lying (A = LYING) quadrant ────────
; Corner k takes its X and Y words from the tables below. The lying quadrant is
; the standing one with X and Y swapped. Both are shifted into the petal.
.proc render_quadrant
    sta SPLIT_QUAD
    lda #0
    sta SPLIT_CORNER
    sta SPLIT_CORNER_OFFSET
@corner:
    lda SPLIT_QUAD
    clc
    adc SPLIT_CORNER
    tax
    lda quadrant_x_word,x
    tay
    ldx SPLIT_CORNER_OFFSET
    clc
    lda SPLIT_WORDS,y
    adc #<SPLIT_SHIFT_WORD
    sta CORNER0_X,x
    lda SPLIT_WORDS + 1,y
    adc #>SPLIT_SHIFT_WORD
    sta CORNER0_X + 1,x

    lda SPLIT_QUAD
    clc
    adc SPLIT_CORNER
    tax
    lda quadrant_y_word,x
    tay
    ldx SPLIT_CORNER_OFFSET
    clc
    lda SPLIT_WORDS,y
    adc #<SPLIT_Y_SHIFT_WORD
    sta CORNER0_Y,x
    lda SPLIT_WORDS + 1,y
    adc #>SPLIT_Y_SHIFT_WORD
    sta CORNER0_Y + 1,x

    lda SPLIT_CORNER_OFFSET
    clc
    adc #4
    sta SPLIT_CORNER_OFFSET
    inc SPLIT_CORNER
    lda SPLIT_CORNER
    cmp #4
    bne @corner
    jmp render_quad
.endproc

; Offsets in SPLIT_WORDS of the X and Y word of each corner, standing then lying
quadrant_x_word:
    .byte SPLIT_WORD_ZERO, SPLIT_WORD_WAIST, SPLIT_WORD_WIDE, SPLIT_WORD_ZERO
    .byte SPLIT_WORD_ZERO, SPLIT_WORD_ZERO, SPLIT_WORD_WIDE_Y, SPLIT_WORD_TIP
quadrant_y_word:
    .byte SPLIT_WORD_ZERO, SPLIT_WORD_ZERO, SPLIT_WORD_WIDE_Y, SPLIT_WORD_TIP
    .byte SPLIT_WORD_ZERO, SPLIT_WORD_WAIST, SPLIT_WORD_WIDE, SPLIT_WORD_ZERO

; ─── Copy the petal as a tile at SPLIT_QUAD_PTR, rotated back by 4 pixels ────
.proc unshift_petal
    lda #0
    sta MORPH_ROW_INDEX
    sta SPLIT_ROW_OFFSET
@row:
    jsr load_petal_row
    ldy SPLIT_ROW_OFFSET
    ldx #0
@byte:
    lda MORPH_ROW,x
    sta (SPLIT_QUAD_PTR),y
    iny
    inx
    cpx #TILE_ROW_BYTES
    bne @byte
    sty SPLIT_ROW_OFFSET
    inc MORPH_ROW_INDEX
    lda MORPH_ROW_INDEX
    cmp #TILE_ROWS
    bne @row
    rts
.endproc

; ─── Mirror the quadrants into the corners of the tile of the current state ──
; Tile row r is the standing quadrant in row r, the standing quadrant mirrored in
; both axes (row 23 - r, mirrored), the lying quadrant mirrored horizontally (row r,
; mirrored) and the lying quadrant mirrored vertically (row 23 - r). A mask bit is
; set where every part is set.
.proc mirror_into_tile
    lda SPLIT_BUILD_STATE
    clc
    adc #>SPLIT_TILES - 1
    sta SPLIT_TILE_PTR + 1
    lda #0
    sta SPLIT_TILE_PTR
    sta SPLIT_ROW_OFFSET
    lda #LAST_ROW_OFFSET_SPLIT
    sta SPLIT_MIRROR_OFFSET
@row:
    ldx #TILE_ROW_BYTES - 1
@byte:
    txa
    clc
    adc SPLIT_ROW_OFFSET
    tay
    lda SPLIT_STANDING,y
    sta SPLIT_ACCUMULATOR

    lda mirror_column,x
    clc
    adc SPLIT_MIRROR_OFFSET
    tay
    lda SPLIT_STANDING,y
    tay
    lda BIT_REVERSE_TABLE,y
    and SPLIT_ACCUMULATOR
    sta SPLIT_ACCUMULATOR

    lda mirror_column,x
    clc
    adc SPLIT_ROW_OFFSET
    tay
    lda SPLIT_LYING,y
    tay
    lda BIT_REVERSE_TABLE,y
    and SPLIT_ACCUMULATOR
    sta SPLIT_ACCUMULATOR

    txa
    clc
    adc SPLIT_MIRROR_OFFSET
    tay
    lda SPLIT_LYING,y
    and SPLIT_ACCUMULATOR
    pha
    txa
    clc
    adc SPLIT_ROW_OFFSET
    tay
    pla
    sta (SPLIT_TILE_PTR),y
    dex
    bpl @byte

    lda SPLIT_ROW_OFFSET
    clc
    adc #TILE_ROW_BYTES
    sta SPLIT_ROW_OFFSET
    lda SPLIT_MIRROR_OFFSET
    sec
    sbc #TILE_ROW_BYTES
    sta SPLIT_MIRROR_OFFSET
    lda SPLIT_ROW_OFFSET
    cmp #TILE_BYTES
    bne @row
    rts
.endproc

; ─── Show the tile of state SEQ_SPLIT ─────────────────────────────────────────
.proc split_step
    lda SEQ_SPLIT
    clc
    adc #>SPLIT_TILES - 1
    jmp show_tile
.endproc

; ─── Turn the grid into a checkerboard of the tile and its mirror image ──────
; Until now a whole band shows the tile or its mirror image. Every odd tile column
; of the screens swaps to the other kind of chars, and in the sprite pointers every
; other sprite does. The tile of the morph is symmetric, so nothing changes on screen.
.proc checker_init
    jsr sprite_checker_pointers
    lda #<GRID_ORIGIN
    sta SPLIT_SCREEN_PTR
    sta SPLIT_SCREEN_C_PTR
    lda #>GRID_ORIGIN
    sta SPLIT_SCREEN_PTR + 1
    lda #>GRID_ORIGIN_C
    sta SPLIT_SCREEN_C_PTR + 1
    lda #0
    sta SPLIT_SCREEN_ROW
@screen_row:
    ldx SPLIT_SCREEN_ROW
    lda swapped_row_char,x
    sta SPLIT_ROW_CHAR
    ldx #ODD_TILE_CHAR_COLUMNS - 1
@column:
    ldy odd_tile_screen_column,x
    lda odd_tile_char_base,x
    clc
    adc SPLIT_ROW_CHAR
    sta (SPLIT_SCREEN_PTR),y
    sta (SPLIT_SCREEN_C_PTR),y
    dex
    bpl @column

    lda SPLIT_SCREEN_PTR
    clc
    adc #SCREEN_ROW_BYTES
    sta SPLIT_SCREEN_PTR
    sta SPLIT_SCREEN_C_PTR
    bcc @same_page
    inc SPLIT_SCREEN_PTR + 1
    inc SPLIT_SCREEN_C_PTR + 1
@same_page:
    inc SPLIT_SCREEN_ROW
    lda SPLIT_SCREEN_ROW
    cmp #SCREEN_ROWS
    bne @screen_row
    rts
.endproc

odd_tile_screen_column:
    .byte 3, 4, 5, 9, 10, 11, 15, 16, 17, 21, 22, 23
odd_tile_char_base:
    .byte 0, 8, 16, 0, 8, 16, 0, 8, 16, 0, 8, 16
swapped_row_char:
    .byte 3, 4, 5, 0, 1, 2, 3, 4, 5, 0, 1, 2
    .byte 3, 4, 5, 0, 1, 2, 3, 4, 5, 0, 1, 2
