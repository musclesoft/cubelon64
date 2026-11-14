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

CUBE_STEP_LIMIT = 471                ; $FFFF: the loop never stops (for debugging, together with CUBE_ENDED_FROM)
CUBE_LOOP_FRAMES = 220
CUBE_HALF_LOOP = 110
CUBE_LOOP_START = 112
CUBE_ENDED_FROM = 439                ; $FFFF: the loop never stops
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
CUBE_SCREEN_SECOND = GRID_ORIGIN + 480

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
CUBE_LINE: .res 1
CUBE_SCREEN_STEP: .res 1
CUBE_SCREEN_END: .res 1
CUBE_ROW_START: .res 1

.segment "CUBECODE"

.import CUBE_IMAGE_LOW: absolute
.import CUBE_IMAGE_HIGH: absolute
.import BIT_REVERSE_TABLE: absolute
.import SIMPLE_POINTERS: absolute

.export cube_prepare
.export cube_setup
.export cube_step
.export CUBE_SQUARE_POINTER

; ─── Black chars, image 0 in all four rings and in the square's sprite block, the first frame's image ─
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

    lda #0
    sta CUBE_SCREEN_STEP
    sta CUBE_SLOT
    sta CUBE_STAGE
    sta CUBE_STAGE + 1
    lda #CUBE_LOOP_START
    sta CUBE_LOOP
    jmp stage_frame
.endproc

; ─── Switch the screen to the new chars ──────────────────────────────────────
; The handler of EVENT_CUBE_SETUP, on the two frames before the first one of the scene, 4 tile rows each. Every
; block shows the same square before and after, so a frame in which the codes are only partly written looks the same.
.proc cube_setup
    jmp cube_write_screen
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
    cmp #>CUBE_ENDED_FROM
    bcc @animated
    bne @ended
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
ring_start:
    .byte 0, 10, 20, 30

; The pieces of a turned image: the left and right char column swap, the middle one and the sprite stay
turned_source_offset:
    .byte 34, 0, 16, 50
turned_destination_offset:
    .byte 0, 34, 16, 0
turned_piece_last:
    .byte 15, 15, 17, 53

.include "cube_show_data.inc"
