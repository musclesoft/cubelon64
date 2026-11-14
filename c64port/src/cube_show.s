; cube_show.s - The rotating cubes of the last scene on the screen
;
; Every block of the 8x8 grid plays the same loop of 220 images, 10 frames later the farther it is
; from the centre (ring 0-3). The loop is the 110 images cube_build.s renders and the same turned by 180
; degrees. A block of ring k is drawn from the chars and the sprite of kind k:
;   chars of kind k, tile column j   CUBE_CHARS + 96 * j + 24 * k, screen code 24 + 12 * j + 3 * k + m for tile row m
;   sprite of the ring                the sprite block of the image it shows, which the simple multiplexer's
;                                     pointers choose (SIMPLE_POINTERS)
; The images only keep the rows that are ever not black (see cube_render.s), the rest of the chars is black,
; set once. The chars lie where the scenes before this one keep nothing, so they and the first image are
; prepared while those scenes still run, and the screen's codes are the only thing the switch has to write.
;
; The ring r shows the image of the frame n - 10 r, so every frame's image is made once, into a ring buffer
; of 32 slots (one more than the rings need: the slot a step stages into is not one the sprites show) (50 bytes of chars, a sprite block), and the four rings take the slots 10 frames apart: they copy the
; chars and point their sprites at the block. Before its start a ring shows image 0, the square the scene before it
; ends with, which has a sprite block of its own in the unused part of the charset.

.include "include/shared_imports.inc"

CUBE_FIRST_FRAME = 2721
CUBE_STEP_LIMIT = 471
CUBE_LOOP_FRAMES = 220
CUBE_HALF_LOOP = 110
CUBE_LOOP_START = 112
CUBE_ENDED_FROM = 439
CUBE_DELAY_SLOTS = 32
CUBE_RING_DELAY = 10
CUBE_RINGS = 4
CUBE_CHAR_PART = 50
CUBE_CHAR_AREA = 288
CUBE_CHARS = CHARSET + 192
CUBE_SPRITE_ROW_OFFSET = 9
CUBE_SPRITE_PART = 54
CUBE_SQUARE_BLOCK = $6700
CUBE_SQUARE_SPRITE = CUBE_SQUARE_BLOCK + CUBE_SPRITE_ROW_OFFSET
CUBE_SQUARE_POINTER = $9C
CUBE_SPRITE_POINTER = $E0
CUBE_BLACK = $FF

.segment "ZEROPAGE"

CUBE_IMAGE_PTR: .res 2
CUBE_CHAR_DST: .res 2
CUBE_SPRITE_DST: .res 2

.segment "CUBESPRITES"

.align 64
CUBE_SPRITE_BLOCKS: .res CUBE_DELAY_SLOTS * 64

.segment "CUBEBSS"

CUBE_CHAR_SLOTS: .res CUBE_DELAY_SLOTS * CUBE_CHAR_PART
CUBE_N: .res 2
CUBE_STAGE: .res 2
CUBE_SLOT: .res 1
CUBE_LOOP: .res 1
CUBE_RING: .res 1
CUBE_RING_SLOT: .res 1
CUBE_RING_POINTER: .res CUBE_RINGS

.segment "CUBECODE"

.import CUBE_IMAGE_LOW: absolute
.import CUBE_IMAGE_HIGH: absolute
.import BIT_REVERSE_TABLE: absolute
.import SIMPLE_POINTERS: absolute

.export cube_prepare
.export cube_setup
.export cube_step
.export CUBE_SQUARE_POINTER

; ─── Black chars, and image 0 in all four rings and in the square's sprite block ─
; The handler of EVENT_CUBE_PREPARE, a few frames before the scene: nothing it touches is shown yet.
.proc cube_prepare
    lda #CUBE_BLACK
    ldx #0
@black:
    sta CUBE_CHARS,x
    inx
    bne @black
    ldx #CUBE_CHAR_AREA - 256 - 1
@black_rest:
    sta CUBE_CHARS + 256,x
    dex
    bpl @black_rest

    lda CUBE_IMAGE_LOW
    sta CUBE_SHOW_SRC
    lda CUBE_IMAGE_HIGH
    sta CUBE_SHOW_SRC + 1
    lda #0
    sta CUBE_RING
@ring:
    jsr copy_chars
    inc CUBE_RING
    lda CUBE_RING
    cmp #CUBE_RINGS
    bne @ring

    lda CUBE_SHOW_SRC
    clc
    adc #CUBE_CHAR_PART
    sta CUBE_SHOW_SRC
    bcc @sprite
    inc CUBE_SHOW_SRC + 1
@sprite:
    lda #<CUBE_SQUARE_SPRITE
    sta CUBE_SHOW_DST
    lda #>CUBE_SQUARE_SPRITE
    sta CUBE_SHOW_DST + 1
    ldy #CUBE_SPRITE_PART - 1
@square:
    lda (CUBE_SHOW_SRC),y
    sta (CUBE_SHOW_DST),y
    dey
    bpl @square
    rts
.endproc

; ─── Switch the screen to the new chars and make the first frame's image ─────
; The handler of EVENT_CUBE_SETUP, the frame before the first one of the scene.
.proc cube_setup
    ldx #23
@column:
    lda cube_screen_codes + 0,x
    sta GRID_ORIGIN + 0,x
    lda cube_screen_codes + 24,x
    sta GRID_ORIGIN + 40,x
    lda cube_screen_codes + 48,x
    sta GRID_ORIGIN + 80,x
    lda cube_screen_codes + 72,x
    sta GRID_ORIGIN + 120,x
    lda cube_screen_codes + 96,x
    sta GRID_ORIGIN + 160,x
    lda cube_screen_codes + 120,x
    sta GRID_ORIGIN + 200,x
    lda cube_screen_codes + 144,x
    sta GRID_ORIGIN + 240,x
    lda cube_screen_codes + 168,x
    sta GRID_ORIGIN + 280,x
    lda cube_screen_codes + 192,x
    sta GRID_ORIGIN + 320,x
    lda cube_screen_codes + 216,x
    sta GRID_ORIGIN + 360,x
    lda cube_screen_codes + 240,x
    sta GRID_ORIGIN + 400,x
    lda cube_screen_codes + 264,x
    sta GRID_ORIGIN + 440,x
    lda cube_screen_codes + 288,x
    sta GRID_ORIGIN + 480,x
    lda cube_screen_codes + 312,x
    sta GRID_ORIGIN + 520,x
    lda cube_screen_codes + 336,x
    sta GRID_ORIGIN + 560,x
    lda cube_screen_codes + 360,x
    sta GRID_ORIGIN + 600,x
    lda cube_screen_codes + 384,x
    sta GRID_ORIGIN + 640,x
    lda cube_screen_codes + 408,x
    sta GRID_ORIGIN + 680,x
    lda cube_screen_codes + 432,x
    sta GRID_ORIGIN + 720,x
    lda cube_screen_codes + 456,x
    sta GRID_ORIGIN + 760,x
    lda cube_screen_codes + 480,x
    sta GRID_ORIGIN + 800,x
    lda cube_screen_codes + 504,x
    sta GRID_ORIGIN + 840,x
    lda cube_screen_codes + 528,x
    sta GRID_ORIGIN + 880,x
    lda cube_screen_codes + 552,x
    sta GRID_ORIGIN + 920,x
    dex
    bmi @columns_done
    jmp @column
@columns_done:

    lda #0
    sta CUBE_SLOT
    sta CUBE_STAGE
    sta CUBE_STAGE + 1
    lda #CUBE_LOOP_START
    sta CUBE_LOOP
    jmp stage_frame
.endproc

; ─── Show the images of this frame in the four rings and make the next frame's image ─
; The handler of EVENT_CUBES3D, every frame from the first one of the scene.
.proc cube_step
    lda SEQ_FRAME_LO
    sec
    sbc #<CUBE_FIRST_FRAME
    sta CUBE_N
    lda SEQ_FRAME_HI
    sbc #>CUBE_FIRST_FRAME
    sta CUBE_N + 1
    cmp #>CUBE_STEP_LIMIT
    bcc @running
    bne @finished
    lda CUBE_N
    cmp #<CUBE_STEP_LIMIT
    bcc @running
@finished:
    rts
@running:
    lda CUBE_SLOT
    sta CUBE_RING_SLOT
    lda #0
    sta CUBE_RING
@ring:
    ldx CUBE_RING
    lda CUBE_N + 1
    bne @started
    lda CUBE_N
    cmp ring_start,x
    bcs @started
    lda CUBE_IMAGE_LOW
    sta CUBE_SHOW_SRC
    lda CUBE_IMAGE_HIGH
    sta CUBE_SHOW_SRC + 1
    lda #CUBE_SQUARE_POINTER
    sta CUBE_RING_POINTER,x
    jmp @copy
@started:
    lda CUBE_RING_SLOT
    asl
    tay
    lda char_slot_address,y
    sta CUBE_SHOW_SRC
    lda char_slot_address + 1,y
    sta CUBE_SHOW_SRC + 1
    lda CUBE_RING_SLOT
    clc
    adc #CUBE_SPRITE_POINTER
    sta CUBE_RING_POINTER,x
@copy:
    jsr copy_chars
    lda CUBE_RING_SLOT
    sec
    sbc #CUBE_RING_DELAY
    bcs @slot_kept
    adc #CUBE_DELAY_SLOTS
@slot_kept:
    sta CUBE_RING_SLOT
    inc CUBE_RING
    lda CUBE_RING
    cmp #CUBE_RINGS
    bne @ring

    jsr point_sprites

    inc CUBE_SLOT
    lda CUBE_SLOT
    cmp #CUBE_DELAY_SLOTS
    bcc @slot_ready
    lda #0
    sta CUBE_SLOT
@slot_ready:
    inc CUBE_LOOP
    lda CUBE_LOOP
    cmp #CUBE_LOOP_FRAMES
    bcc @stage
    lda #0
    sta CUBE_LOOP
@stage:
    lda CUBE_N
    clc
    adc #1
    sta CUBE_STAGE
    lda CUBE_N + 1
    adc #0
    sta CUBE_STAGE + 1
    jmp stage_frame
.endproc

; ─── Point the sprites of every band at the sprite block of their ring ───────
; The pointers of the first band are also written into screen A: the interrupt that sets them for the
; next frame came before this step.
.proc point_sprites
    lda CUBE_RING_POINTER + 0
    sta SIMPLE_POINTERS + 27
    sta SIMPLE_POINTERS + 28
    sta SIMPLE_POINTERS + 35
    sta SIMPLE_POINTERS + 36
    lda CUBE_RING_POINTER + 1
    sta SIMPLE_POINTERS + 18
    sta SIMPLE_POINTERS + 19
    sta SIMPLE_POINTERS + 20
    sta SIMPLE_POINTERS + 21
    sta SIMPLE_POINTERS + 26
    sta SIMPLE_POINTERS + 29
    sta SIMPLE_POINTERS + 34
    sta SIMPLE_POINTERS + 37
    sta SIMPLE_POINTERS + 42
    sta SIMPLE_POINTERS + 43
    sta SIMPLE_POINTERS + 44
    sta SIMPLE_POINTERS + 45
    lda CUBE_RING_POINTER + 2
    sta SIMPLE_POINTERS + 9
    sta SIMPLE_POINTERS + 10
    sta SIMPLE_POINTERS + 11
    sta SIMPLE_POINTERS + 12
    sta SIMPLE_POINTERS + 13
    sta SIMPLE_POINTERS + 14
    sta SIMPLE_POINTERS + 17
    sta SIMPLE_POINTERS + 22
    sta SIMPLE_POINTERS + 25
    sta SIMPLE_POINTERS + 30
    sta SIMPLE_POINTERS + 33
    sta SIMPLE_POINTERS + 38
    sta SIMPLE_POINTERS + 41
    sta SIMPLE_POINTERS + 46
    sta SIMPLE_POINTERS + 49
    sta SIMPLE_POINTERS + 50
    sta SIMPLE_POINTERS + 51
    sta SIMPLE_POINTERS + 52
    sta SIMPLE_POINTERS + 53
    sta SIMPLE_POINTERS + 54
    lda CUBE_RING_POINTER + 3
    sta SIMPLE_POINTERS + 0
    sta SIMPLE_POINTERS + 1
    sta SIMPLE_POINTERS + 2
    sta SIMPLE_POINTERS + 3
    sta SIMPLE_POINTERS + 4
    sta SIMPLE_POINTERS + 5
    sta SIMPLE_POINTERS + 6
    sta SIMPLE_POINTERS + 7
    sta SIMPLE_POINTERS + 8
    sta SIMPLE_POINTERS + 15
    sta SIMPLE_POINTERS + 16
    sta SIMPLE_POINTERS + 23
    sta SIMPLE_POINTERS + 24
    sta SIMPLE_POINTERS + 31
    sta SIMPLE_POINTERS + 32
    sta SIMPLE_POINTERS + 39
    sta SIMPLE_POINTERS + 40
    sta SIMPLE_POINTERS + 47
    sta SIMPLE_POINTERS + 48
    sta SIMPLE_POINTERS + 55
    sta SIMPLE_POINTERS + 56
    sta SIMPLE_POINTERS + 57
    sta SIMPLE_POINTERS + 58
    sta SIMPLE_POINTERS + 59
    sta SIMPLE_POINTERS + 60
    sta SIMPLE_POINTERS + 61
    sta SIMPLE_POINTERS + 62
    sta SIMPLE_POINTERS + 63
    sta POINTERS_A + 0
    sta POINTERS_A + 1
    sta POINTERS_A + 2
    sta POINTERS_A + 3
    sta POINTERS_A + 4
    sta POINTERS_A + 5
    sta POINTERS_A + 6
    sta POINTERS_A + 7
    rts
.endproc

; ─── Make the image of the frame CUBE_STAGE in slot CUBE_SLOT ────────────────
; After the last animated frame it is image 0 again. Before that, loop frame CUBE_LOOP shows image
; CUBE_LOOP, or in the second half of the loop image CUBE_LOOP - 110 turned.
.proc stage_frame
    lda CUBE_SLOT
    asl
    tay
    lda char_slot_address,y
    sta CUBE_CHAR_DST
    lda char_slot_address + 1,y
    sta CUBE_CHAR_DST + 1
    lda sprite_slot_address,y
    sta CUBE_SPRITE_DST
    lda sprite_slot_address + 1,y
    sta CUBE_SPRITE_DST + 1
    lda CUBE_STAGE + 1
    cmp #2
    bcs @ended
    cmp #1
    bne @animated
    lda CUBE_STAGE
    cmp #<CUBE_ENDED_FROM
    bcs @ended
@animated:
    ldx CUBE_LOOP
    cpx #CUBE_HALF_LOOP
    bcs @turned
    lda CUBE_IMAGE_LOW,x
    sta CUBE_IMAGE_PTR
    lda CUBE_IMAGE_HIGH,x
    sta CUBE_IMAGE_PTR + 1
    jmp stage_plain
@turned:
    txa
    sec
    sbc #CUBE_HALF_LOOP
    tax
    lda CUBE_IMAGE_LOW,x
    sta CUBE_IMAGE_PTR
    lda CUBE_IMAGE_HIGH,x
    sta CUBE_IMAGE_PTR + 1
    jmp stage_turned
@ended:
    lda CUBE_IMAGE_LOW
    sta CUBE_IMAGE_PTR
    lda CUBE_IMAGE_HIGH
    sta CUBE_IMAGE_PTR + 1
    jmp stage_plain
.endproc

; ─── The image at CUBE_IMAGE_PTR into the slot's chars and sprite block ──────
.proc stage_plain
    ldy #CUBE_CHAR_PART - 1
@chars:
    lda (CUBE_IMAGE_PTR),y
    sta (CUBE_CHAR_DST),y
    dey
    bpl @chars
    jsr image_sprite_part
    ldy #CUBE_SPRITE_PART - 1
@sprite:
    lda (CUBE_SHOW_SRC),y
    sta (CUBE_SPRITE_DST),y
    dey
    bpl @sprite
    rts
.endproc

; CUBE_SHOW_SRC = the sprite part of the image at CUBE_IMAGE_PTR
.proc image_sprite_part
    lda CUBE_IMAGE_PTR
    clc
    adc #CUBE_CHAR_PART
    sta CUBE_SHOW_SRC
    lda CUBE_IMAGE_PTR + 1
    adc #0
    sta CUBE_SHOW_SRC + 1
    rts
.endproc

; ─── The same turned by 180 degrees ──────────────────────────────────────────
; Each piece (a char column, the sprite) is its bytes in reverse order, bit-reversed, and the left
; and right char column swap places.
.proc stage_turned
    ldx #0
@piece:
    stx CUBE_RING
    lda turned_source_offset,x
    clc
    adc CUBE_IMAGE_PTR
    sta CUBE_SHOW_SRC
    lda CUBE_IMAGE_PTR + 1
    adc #0
    sta CUBE_SHOW_SRC + 1
    lda turned_destination_offset,x
    clc
    adc CUBE_CHAR_DST
    sta CUBE_SHOW_DST
    lda CUBE_CHAR_DST + 1
    adc #0
    sta CUBE_SHOW_DST + 1
    cpx #3
    bne @bytes
    lda CUBE_SPRITE_DST
    sta CUBE_SHOW_DST
    lda CUBE_SPRITE_DST + 1
    sta CUBE_SHOW_DST + 1
@bytes:
    lda turned_piece_last,x
    tax
    jsr turn_piece
    ldx CUBE_RING
    inx
    cpx #4
    bne @piece
    rts
.endproc

; ─── Turn the X + 1 bytes at CUBE_SHOW_SRC into those at CUBE_SHOW_DST ───────
; The bytes in reverse order, bit-reversed. The bit reversal reads the table through an address
; whose low byte is the byte itself.
.proc turn_piece
    lda CUBE_SHOW_SRC
    sta @read + 1
    lda CUBE_SHOW_SRC + 1
    sta @read + 2
    ldy #0
@byte:
@read:
    lda $FFFF,x
    sta @lookup + 1
@lookup:
    lda BIT_REVERSE_TABLE
    sta (CUBE_SHOW_DST),y
    iny
    dex
    bpl @byte
    rts
.endproc

; ─── Copy the 50 bytes of chars at CUBE_SHOW_SRC to the chars of kind CUBE_RING ─
.proc copy_chars
    lda CUBE_RING
    asl
    tax
    lda copy_chars_entries + 1,x
    pha
    lda copy_chars_entries,x
    pha
    rts
.endproc
.proc copy_chars_0
    lda CUBE_SHOW_SRC
    sta @p0 + 1
    lda CUBE_SHOW_SRC + 1
    sta @p0 + 2
    ldy #15
@p0:
    lda $FFFF,y
    sta $60C4,y
    dey
    bpl @p0
    lda CUBE_SHOW_SRC
    clc
    adc #16
    sta @p1 + 1
    lda CUBE_SHOW_SRC + 1
    adc #0
    sta @p1 + 2
    ldy #17
@p1:
    lda $FFFF,y
    sta $6123,y
    dey
    bpl @p1
    lda CUBE_SHOW_SRC
    clc
    adc #34
    sta @p2 + 1
    lda CUBE_SHOW_SRC + 1
    adc #0
    sta @p2 + 2
    ldy #15
@p2:
    lda $FFFF,y
    sta $6184,y
    dey
    bpl @p2
    rts
.endproc

.proc copy_chars_1
    lda CUBE_SHOW_SRC
    sta @p0 + 1
    lda CUBE_SHOW_SRC + 1
    sta @p0 + 2
    ldy #15
@p0:
    lda $FFFF,y
    sta $60DC,y
    dey
    bpl @p0
    lda CUBE_SHOW_SRC
    clc
    adc #16
    sta @p1 + 1
    lda CUBE_SHOW_SRC + 1
    adc #0
    sta @p1 + 2
    ldy #17
@p1:
    lda $FFFF,y
    sta $613B,y
    dey
    bpl @p1
    lda CUBE_SHOW_SRC
    clc
    adc #34
    sta @p2 + 1
    lda CUBE_SHOW_SRC + 1
    adc #0
    sta @p2 + 2
    ldy #15
@p2:
    lda $FFFF,y
    sta $619C,y
    dey
    bpl @p2
    rts
.endproc

.proc copy_chars_2
    lda CUBE_SHOW_SRC
    sta @p0 + 1
    lda CUBE_SHOW_SRC + 1
    sta @p0 + 2
    ldy #15
@p0:
    lda $FFFF,y
    sta $60F4,y
    dey
    bpl @p0
    lda CUBE_SHOW_SRC
    clc
    adc #16
    sta @p1 + 1
    lda CUBE_SHOW_SRC + 1
    adc #0
    sta @p1 + 2
    ldy #17
@p1:
    lda $FFFF,y
    sta $6153,y
    dey
    bpl @p1
    lda CUBE_SHOW_SRC
    clc
    adc #34
    sta @p2 + 1
    lda CUBE_SHOW_SRC + 1
    adc #0
    sta @p2 + 2
    ldy #15
@p2:
    lda $FFFF,y
    sta $61B4,y
    dey
    bpl @p2
    rts
.endproc

.proc copy_chars_3
    lda CUBE_SHOW_SRC
    sta @p0 + 1
    lda CUBE_SHOW_SRC + 1
    sta @p0 + 2
    ldy #15
@p0:
    lda $FFFF,y
    sta $610C,y
    dey
    bpl @p0
    lda CUBE_SHOW_SRC
    clc
    adc #16
    sta @p1 + 1
    lda CUBE_SHOW_SRC + 1
    adc #0
    sta @p1 + 2
    ldy #17
@p1:
    lda $FFFF,y
    sta $616B,y
    dey
    bpl @p1
    lda CUBE_SHOW_SRC
    clc
    adc #34
    sta @p2 + 1
    lda CUBE_SHOW_SRC + 1
    adc #0
    sta @p2 + 2
    ldy #15
@p2:
    lda $FFFF,y
    sta $61CC,y
    dey
    bpl @p2
    rts
.endproc

ring_start:
    .byte 0, 10, 20, 30

copy_chars_entries:
    .word copy_chars_0 - 1, copy_chars_1 - 1, copy_chars_2 - 1, copy_chars_3 - 1

; The pieces of a turned image: the left and right char column swap, the middle one and the sprite stay
turned_source_offset:
    .byte 34, 0, 16, 50
turned_destination_offset:
    .byte 0, 34, 16, 0
turned_piece_last:
    .byte 15, 15, 17, 53

char_slot_address:
    .word CUBE_CHAR_SLOTS + 0, CUBE_CHAR_SLOTS + 50, CUBE_CHAR_SLOTS + 100, CUBE_CHAR_SLOTS + 150, CUBE_CHAR_SLOTS + 200, CUBE_CHAR_SLOTS + 250, CUBE_CHAR_SLOTS + 300, CUBE_CHAR_SLOTS + 350, CUBE_CHAR_SLOTS + 400, CUBE_CHAR_SLOTS + 450, CUBE_CHAR_SLOTS + 500, CUBE_CHAR_SLOTS + 550, CUBE_CHAR_SLOTS + 600, CUBE_CHAR_SLOTS + 650, CUBE_CHAR_SLOTS + 700, CUBE_CHAR_SLOTS + 750, CUBE_CHAR_SLOTS + 800, CUBE_CHAR_SLOTS + 850, CUBE_CHAR_SLOTS + 900, CUBE_CHAR_SLOTS + 950, CUBE_CHAR_SLOTS + 1000, CUBE_CHAR_SLOTS + 1050, CUBE_CHAR_SLOTS + 1100, CUBE_CHAR_SLOTS + 1150, CUBE_CHAR_SLOTS + 1200, CUBE_CHAR_SLOTS + 1250, CUBE_CHAR_SLOTS + 1300, CUBE_CHAR_SLOTS + 1350, CUBE_CHAR_SLOTS + 1400, CUBE_CHAR_SLOTS + 1450, CUBE_CHAR_SLOTS + 1500, CUBE_CHAR_SLOTS + 1550
sprite_slot_address:
    .word CUBE_SPRITE_BLOCKS + 9, CUBE_SPRITE_BLOCKS + 73, CUBE_SPRITE_BLOCKS + 137, CUBE_SPRITE_BLOCKS + 201, CUBE_SPRITE_BLOCKS + 265, CUBE_SPRITE_BLOCKS + 329, CUBE_SPRITE_BLOCKS + 393, CUBE_SPRITE_BLOCKS + 457, CUBE_SPRITE_BLOCKS + 521, CUBE_SPRITE_BLOCKS + 585, CUBE_SPRITE_BLOCKS + 649, CUBE_SPRITE_BLOCKS + 713, CUBE_SPRITE_BLOCKS + 777, CUBE_SPRITE_BLOCKS + 841, CUBE_SPRITE_BLOCKS + 905, CUBE_SPRITE_BLOCKS + 969, CUBE_SPRITE_BLOCKS + 1033, CUBE_SPRITE_BLOCKS + 1097, CUBE_SPRITE_BLOCKS + 1161, CUBE_SPRITE_BLOCKS + 1225, CUBE_SPRITE_BLOCKS + 1289, CUBE_SPRITE_BLOCKS + 1353, CUBE_SPRITE_BLOCKS + 1417, CUBE_SPRITE_BLOCKS + 1481, CUBE_SPRITE_BLOCKS + 1545, CUBE_SPRITE_BLOCKS + 1609, CUBE_SPRITE_BLOCKS + 1673, CUBE_SPRITE_BLOCKS + 1737, CUBE_SPRITE_BLOCKS + 1801, CUBE_SPRITE_BLOCKS + 1865, CUBE_SPRITE_BLOCKS + 1929, CUBE_SPRITE_BLOCKS + 1993

cube_screen_codes:
    .byte 33, 45, 57, 33, 45, 57, 33, 45, 57, 33, 45, 57, 33, 45, 57, 33, 45, 57, 33, 45, 57, 33, 45, 57
    .byte 34, 46, 58, 34, 46, 58, 34, 46, 58, 34, 46, 58, 34, 46, 58, 34, 46, 58, 34, 46, 58, 34, 46, 58
    .byte 35, 47, 59, 35, 47, 59, 35, 47, 59, 35, 47, 59, 35, 47, 59, 35, 47, 59, 35, 47, 59, 35, 47, 59
    .byte 33, 45, 57, 30, 42, 54, 30, 42, 54, 30, 42, 54, 30, 42, 54, 30, 42, 54, 30, 42, 54, 33, 45, 57
    .byte 34, 46, 58, 31, 43, 55, 31, 43, 55, 31, 43, 55, 31, 43, 55, 31, 43, 55, 31, 43, 55, 34, 46, 58
    .byte 35, 47, 59, 32, 44, 56, 32, 44, 56, 32, 44, 56, 32, 44, 56, 32, 44, 56, 32, 44, 56, 35, 47, 59
    .byte 33, 45, 57, 30, 42, 54, 27, 39, 51, 27, 39, 51, 27, 39, 51, 27, 39, 51, 30, 42, 54, 33, 45, 57
    .byte 34, 46, 58, 31, 43, 55, 28, 40, 52, 28, 40, 52, 28, 40, 52, 28, 40, 52, 31, 43, 55, 34, 46, 58
    .byte 35, 47, 59, 32, 44, 56, 29, 41, 53, 29, 41, 53, 29, 41, 53, 29, 41, 53, 32, 44, 56, 35, 47, 59
    .byte 33, 45, 57, 30, 42, 54, 27, 39, 51, 24, 36, 48, 24, 36, 48, 27, 39, 51, 30, 42, 54, 33, 45, 57
    .byte 34, 46, 58, 31, 43, 55, 28, 40, 52, 25, 37, 49, 25, 37, 49, 28, 40, 52, 31, 43, 55, 34, 46, 58
    .byte 35, 47, 59, 32, 44, 56, 29, 41, 53, 26, 38, 50, 26, 38, 50, 29, 41, 53, 32, 44, 56, 35, 47, 59
    .byte 33, 45, 57, 30, 42, 54, 27, 39, 51, 24, 36, 48, 24, 36, 48, 27, 39, 51, 30, 42, 54, 33, 45, 57
    .byte 34, 46, 58, 31, 43, 55, 28, 40, 52, 25, 37, 49, 25, 37, 49, 28, 40, 52, 31, 43, 55, 34, 46, 58
    .byte 35, 47, 59, 32, 44, 56, 29, 41, 53, 26, 38, 50, 26, 38, 50, 29, 41, 53, 32, 44, 56, 35, 47, 59
    .byte 33, 45, 57, 30, 42, 54, 27, 39, 51, 27, 39, 51, 27, 39, 51, 27, 39, 51, 30, 42, 54, 33, 45, 57
    .byte 34, 46, 58, 31, 43, 55, 28, 40, 52, 28, 40, 52, 28, 40, 52, 28, 40, 52, 31, 43, 55, 34, 46, 58
    .byte 35, 47, 59, 32, 44, 56, 29, 41, 53, 29, 41, 53, 29, 41, 53, 29, 41, 53, 32, 44, 56, 35, 47, 59
    .byte 33, 45, 57, 30, 42, 54, 30, 42, 54, 30, 42, 54, 30, 42, 54, 30, 42, 54, 30, 42, 54, 33, 45, 57
    .byte 34, 46, 58, 31, 43, 55, 31, 43, 55, 31, 43, 55, 31, 43, 55, 31, 43, 55, 31, 43, 55, 34, 46, 58
    .byte 35, 47, 59, 32, 44, 56, 32, 44, 56, 32, 44, 56, 32, 44, 56, 32, 44, 56, 32, 44, 56, 35, 47, 59
    .byte 33, 45, 57, 33, 45, 57, 33, 45, 57, 33, 45, 57, 33, 45, 57, 33, 45, 57, 33, 45, 57, 33, 45, 57
    .byte 34, 46, 58, 34, 46, 58, 34, 46, 58, 34, 46, 58, 34, 46, 58, 34, 46, 58, 34, 46, 58, 34, 46, 58
    .byte 35, 47, 59, 35, 47, 59, 35, 47, 59, 35, 47, 59, 35, 47, 59, 35, 47, 59, 35, 47, 59, 35, 47, 59
