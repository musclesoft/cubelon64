; tiles.s - Display images of the kite tiles
;
; A tile is built as 24 rows of 6 bytes (see tile.inc). To show one, its bytes go to
; the three char columns and to the sprite, whose rows lie in the order of the
; hardware, so a tile is converted once at init into an image in exactly that order:
;
;   0-23     char column 0, rows 0-23
;   24-47    char column 1
;   48-71    char column 2
;   72-134   the 63 bytes of the sprite's main block
;   135-143  the 9 bytes of its partner block (the last lines of the band, see below)
;
; Showing images is then block copies. The even kind of tile uses the chars at +0, +64, +128
; and the sprite, the odd kind the chars at +24, +88, +152 and the mirrored sprite.

.include "include/shared_imports.inc"
.include "include/tile.inc"

IMAGE_SPRITE = 72
IMAGE_PARTNER = 135
SPRITE_MAIN_BYTES = 63
SPRITE_PARTNER_BYTES = 9
SPRITE_PARTNER_OFFSET = $40 + 54
POINTERS_PER_IMAGE = 5

.segment "ZEROPAGE"

TILE_SRC_PTR: .res 2
IMAGE_PTR: .res 2
SHOW_IMAGE_A: .res 2
SHOW_IMAGE_B: .res 2
SHOW_A0: .res 2
SHOW_A1: .res 2
SHOW_A2: .res 2
SHOW_A_SPRITE: .res 2
SHOW_A_PARTNER: .res 2
SHOW_B0: .res 2
SHOW_B1: .res 2
SHOW_B2: .res 2
SHOW_B_SPRITE: .res 2
SHOW_B_PARTNER: .res 2

.segment "SCRATCH"

CONVERT_MIRROR: .res 1
CONVERT_ROW: .res 1
CONVERT_OFFSET: .res 1
CONVERT_BYTES: .res TILE_ROW_BYTES
TURN_ROW_OFFSET: .res 1
TURN_MIRROR_OFFSET: .res 1
IMAGE_BASE: .res 2
IMAGE_OFFSET: .res 2

.segment "CODE"

.import mirror_column: absolute
.import job_checkpoint: absolute
.import BIT_REVERSE_TABLE: absolute

.export fill_lower_half
.export store_image
.export store_tile_image
.export store_mirrored_image
.export image_address
.export show_state_pair
.export show_images
.export TILE_SRC_PTR
.export SHOW_IMAGE_A
.export SHOW_IMAGE_B

; ─── Fill rows 12-23 of the tile at TILE_SRC_PTR from its rows 0-11 ──────────
; Row 23 - r is row r turned by 180 degrees: its bytes in reverse order, bit-reversed.
.proc fill_lower_half
    lda #0
    sta TURN_ROW_OFFSET
    lda #LAST_ROW_OFFSET
    sta TURN_MIRROR_OFFSET
@row:
    jsr job_checkpoint
    ldx #TILE_ROW_BYTES - 1
@byte:
    lda mirror_column,x
    clc
    adc TURN_ROW_OFFSET
    tay
    lda (TILE_SRC_PTR),y
    tay
    lda BIT_REVERSE_TABLE,y
    pha
    txa
    clc
    adc TURN_MIRROR_OFFSET
    tay
    pla
    sta (TILE_SRC_PTR),y
    dex
    bpl @byte

    lda TURN_ROW_OFFSET
    clc
    adc #TILE_ROW_BYTES
    sta TURN_ROW_OFFSET
    lda TURN_MIRROR_OFFSET
    sec
    sbc #TILE_ROW_BYTES
    sta TURN_MIRROR_OFFSET
    lda TURN_ROW_OFFSET
    cmp #HALF_TILE_BYTES
    bne @row
    rts
.endproc

; ─── Store the tile at TILE_SRC_PTR as image A of the array of images at X (low), Y (high) ─
.proc store_image
    clc
    jmp store_tile_image
.endproc

; ─── Store the tile at TILE_SRC_PTR as image A of the array at X, Y: the carry set, its mirror image ─
.proc store_tile_image
    php
    jsr aim_image
    lda #0
    plp
    rol
    jmp convert_tile
.endproc

; ─── Store the mirror image of the tile at TILE_SRC_PTR the same way ─────────
.proc store_mirrored_image
    sec
    jmp store_tile_image
.endproc

.proc aim_image
    jsr image_address
    sta IMAGE_PTR
    sty IMAGE_PTR + 1
    rts
.endproc

; ─── Convert the tile at TILE_SRC_PTR into the image at IMAGE_PTR ────────────
; A = 0: the tile as it is, otherwise its mirror image
; (bytes in reverse order, bit-reversed)
.proc convert_tile
    sta CONVERT_MIRROR
    lda #0
    sta CONVERT_ROW
    sta CONVERT_OFFSET
@row:
    jsr job_checkpoint
    ldx #TILE_ROW_BYTES - 1
@load:
    txa
    ldy CONVERT_MIRROR
    beq @offset_ready
    lda mirror_column,x
@offset_ready:
    clc
    adc CONVERT_OFFSET
    tay
    lda (TILE_SRC_PTR),y
    ldy CONVERT_MIRROR
    beq @byte_ready
    tay
    lda BIT_REVERSE_TABLE,y
@byte_ready:
    sta CONVERT_BYTES,x
    dex
    bpl @load

    ldy CONVERT_ROW
    lda CONVERT_BYTES
    sta (IMAGE_PTR),y
    tya
    clc
    adc #24
    tay
    lda CONVERT_BYTES + 1
    sta (IMAGE_PTR),y
    tya
    clc
    adc #24
    tay
    lda CONVERT_BYTES + 2
    sta (IMAGE_PTR),y

    ldx CONVERT_ROW
    ldy sprite_image_index,x
    lda CONVERT_BYTES + 3
    sta (IMAGE_PTR),y
    iny
    lda CONVERT_BYTES + 4
    sta (IMAGE_PTR),y
    iny
    lda CONVERT_BYTES + 5
    sta (IMAGE_PTR),y

    lda CONVERT_OFFSET
    clc
    adc #TILE_ROW_BYTES
    sta CONVERT_OFFSET
    inc CONVERT_ROW
    lda CONVERT_ROW
    cmp #TILE_ROWS
    bne @row
    rts
.endproc

; Where the sprite bytes of each tile row lie in an image: rows 0-17 are the sprite's
; rows, rows 18-23 come from the main and partner block (see the header)
sprite_image_index:
    .byte 72, 75, 78, 81, 84, 87, 90, 93, 96, 99, 102, 105, 108, 111, 114, 117, 120, 123
    .byte 135, 126, 129, 138, 141, 132

; ─── The address of image A (0-255) of an array of images at X (low), Y (high) ─
; Keeps its own variables: the main loop calls it while the job is converting a tile
; Returns A = low, Y = high
.proc image_address
    stx IMAGE_BASE
    sty IMAGE_BASE + 1
    sta IMAGE_OFFSET
    pha
    lda #0
    sta IMAGE_OFFSET + 1
    ldy #3
@times_eight:
    asl IMAGE_OFFSET
    rol IMAGE_OFFSET + 1
    dey
    bne @times_eight
    pla
    clc
    adc IMAGE_OFFSET
    sta IMAGE_OFFSET
    bcc @times_nine
    inc IMAGE_OFFSET + 1
@times_nine:
    ldy #4
@times_sixteen:
    asl IMAGE_OFFSET
    rol IMAGE_OFFSET + 1
    dey
    bne @times_sixteen
    lda IMAGE_OFFSET
    clc
    adc IMAGE_BASE
    sta IMAGE_OFFSET
    lda IMAGE_OFFSET + 1
    adc IMAGE_BASE + 1
    tay
    lda IMAGE_OFFSET
    rts
.endproc

; ─── Show the pair of the state SEQ_SCENE_STATE of the array at X (low), Y (high) ─
; With the carry set the two images swap their places.
; A scene's images are pairs, the first for the even cells of the checkerboard, the second for the odd
; cells: the pair of the state N is image 2 * N and 2 * N + 1 of the array, so the array starts two
; images before the first pair, which is state 1.
.proc show_state_pair
    php
    lda SEQ_SCENE_STATE
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
    plp
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
    jmp show_images
.endproc

; ─── Show the images at SHOW_IMAGE_A and SHOW_IMAGE_B ────────────────────────
; The first goes to the even kind of tile, the second to the odd kind.
.proc show_images
    ldx #0
    ldy #0
@pointers_a:
    clc
    lda SHOW_IMAGE_A
    adc image_offsets,y
    sta SHOW_A0,x
    lda SHOW_IMAGE_A + 1
    adc #0
    sta SHOW_A0 + 1,x
    inx
    inx
    iny
    cpy #POINTERS_PER_IMAGE
    bne @pointers_a
    ldx #0
    ldy #0
@pointers_b:
    clc
    lda SHOW_IMAGE_B
    adc image_offsets,y
    sta SHOW_B0,x
    lda SHOW_IMAGE_B + 1
    adc #0
    sta SHOW_B0 + 1,x
    inx
    inx
    iny
    cpy #POINTERS_PER_IMAGE
    bne @pointers_b

    ldy #TILE_ROWS - 1
@chars:
    lda (SHOW_A0),y
    sta CHARSET,y
    lda (SHOW_A1),y
    sta CHARSET + 64,y
    lda (SHOW_A2),y
    sta CHARSET + 128,y
    lda (SHOW_B0),y
    sta CHARSET + 24,y
    lda (SHOW_B1),y
    sta CHARSET + 88,y
    lda (SHOW_B2),y
    sta CHARSET + 152,y
    dey
    bpl @chars

    ldy #SPRITE_MAIN_BYTES - 1
@sprite:
    lda (SHOW_A_SPRITE),y
    sta SPRITE,y
    lda (SHOW_B_SPRITE),y
    sta SPRITE_REVERSE,y
    dey
    bpl @sprite

    ldy #SPRITE_PARTNER_BYTES - 1
@partner:
    lda (SHOW_A_PARTNER),y
    sta SPRITE + SPRITE_PARTNER_OFFSET,y
    lda (SHOW_B_PARTNER),y
    sta SPRITE_REVERSE + SPRITE_PARTNER_OFFSET,y
    dey
    bpl @partner
    rts
.endproc

image_offsets:
    .byte 0, 24, 48, IMAGE_SPRITE, IMAGE_PARTNER
