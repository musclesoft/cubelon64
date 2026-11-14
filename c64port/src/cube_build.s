; cube_build.s - The background build of the images of the rotating cube
;
; The loop of the cube scene shows 110 images and their turned copies (see cube_render.s). The
; job renders them whenever it has nothing else to do, in the order the scene needs them: image 0, the
; square the scene starts with, then image 2 up to 109, then 1. A rendering takes about 3 frames of idle time, and
; the job is busy with the other scenes' builds until the cube scene, so they are built long before it, where
; nothing else needs the memory until it:
;   CUBE_HIGH_IMAGES     the free RAM under the KERNAL
;   CUBE_TILE_FREE       the end of the tile pages
;   CUBE_CHARSET_FREE    chars no scene uses
;   CUBE_CHAR_SLOTS, CUBE_SPRITE_BLOCKS  the ring buffer of the scene, which only starts to be written by its first step
; The images of the ring buffer's memory move to the tile pages once the last scene before has ended (cube_move).

.include "include/shared_imports.inc"

CUBE_IMAGES = 110
CUBE_IMAGE_SIZE = 104
CUBE_HIGH_SLOTS = 44
CUBE_TILE_FREE_SLOTS = 20
CUBE_CHARSET_SLOTS = 12
CUBE_CHAR_SLOT_IMAGES = 15
CUBE_SPRITE_SLOT_IMAGES = 19
CUBE_TILE_FREE = $C780
CUBE_CHARSET_FREE = $6200
CUBE_MOVE_FIRST_SLOT = CUBE_HIGH_SLOTS + CUBE_TILE_FREE_SLOTS + CUBE_CHARSET_SLOTS
CUBE_MOVE_SLOTS = CUBE_CHAR_SLOT_IMAGES + CUBE_SPRITE_SLOT_IMAGES
CUBE_MOVE_PER_STEP = 4
CUBE_MOVE_BASE = $8800

.segment "ZEROPAGE"

.export CUBE_SHOW_SRC
CUBE_SHOW_SRC: .res 2
.export CUBE_SHOW_DST
CUBE_SHOW_DST: .res 2

.segment "CUBEHIGH"

.export CUBE_HIGH_IMAGES
CUBE_HIGH_IMAGES: .res CUBE_HIGH_SLOTS * CUBE_IMAGE_SIZE

.segment "CUBEBSS"

.export CUBE_BUILT
CUBE_BUILT: .res 1
CUBE_AREA: .res 1
CUBE_MOVED: .res 1
CUBE_MOVE_LEFT: .res 1
CUBE_MOVE_DST: .res 2
.export CUBE_IMAGE_LOW
CUBE_IMAGE_LOW: .res CUBE_IMAGES
.export CUBE_IMAGE_HIGH
CUBE_IMAGE_HIGH: .res CUBE_IMAGES

.segment "CUBECODE"

.import cube_render_init: absolute
.import cube_render_image: absolute
.import CUBE_OUT: absolute
.import CUBE_CHAR_SLOTS: absolute
.import CUBE_SPRITE_BLOCKS: absolute

.export cube_build_init
.export cube_build_next
.export cube_move

; ─── Work out where every image lives and start with none built ──────────────
.proc cube_build_init
    lda #0
    sta CUBE_BUILT
    sta CUBE_AREA
    sta CUBE_MOVED
    jsr cube_render_init
    lda #<CUBE_MOVE_BASE
    sta CUBE_MOVE_DST
    lda #>CUBE_MOVE_BASE
    sta CUBE_MOVE_DST + 1
    ldx #0
@slot:
    ldy CUBE_AREA
    txa
    cmp area_first,y
    bne @area_kept
    tya
    asl
    tay
    lda area_address,y
    sta CUBE_OUT
    lda area_address + 1,y
    sta CUBE_OUT + 1
    inc CUBE_AREA
@area_kept:
    jsr slot_image
    tay
    lda CUBE_OUT
    sta CUBE_IMAGE_LOW,y
    lda CUBE_OUT + 1
    sta CUBE_IMAGE_HIGH,y
    lda CUBE_OUT
    clc
    adc #CUBE_IMAGE_SIZE
    sta CUBE_OUT
    bcc @next
    inc CUBE_OUT + 1
@next:
    inx
    cpx #CUBE_IMAGES
    bne @slot
    rts
.endproc

; ─── The image the build slot X holds: 0, then 2-109, then 1 ─────────────────
.proc slot_image
    txa
    beq @square
    cmp #CUBE_IMAGES - 1
    bcs @last
    clc
    adc #1
    rts
@last:
    lda #1
@square:
    rts
.endproc

; ─── Render the next image ───────────────────────────────────────────────────
.proc cube_build_next
    ldx CUBE_BUILT
    cpx #CUBE_IMAGES
    bcs @done
    jsr slot_image
    tay
    lda CUBE_IMAGE_LOW,y
    sta CUBE_OUT
    lda CUBE_IMAGE_HIGH,y
    sta CUBE_OUT + 1
    tya
    jsr cube_render_image
    inc CUBE_BUILT
    assert_if CUBE_BUILT, 44, CUBE_HIGH_IMAGES, "../proto/golden/cube_images.bin", 104
    assert_if CUBE_BUILT, 44, CUBE_HIGH_IMAGES + 104, "../proto/golden/cube_images.bin" + 208, 4472
    assert_if CUBE_BUILT, 110, CUBE_TILE_FREE, "../proto/golden/cube_images.bin" + 4680, 2080
    assert_if CUBE_BUILT, 110, CUBE_CHARSET_FREE, "../proto/golden/cube_images.bin" + 6760, 1248
    assert_if CUBE_BUILT, 110, CUBE_CHAR_SLOTS, "../proto/golden/cube_images.bin" + 8008, 1560
    assert_if CUBE_BUILT, 110, CUBE_SPRITE_BLOCKS, "../proto/golden/cube_images.bin" + 9568, 1872
    assert_if CUBE_BUILT, 110, CUBE_SPRITE_BLOCKS + 1872, "../proto/golden/cube_images.bin" + 104, 104
@done:
    rts
.endproc

; ─── Move the images of the ring buffer's memory to the tile pages ───────────
; The handler of EVENT_CUBE_MOVE, on the frames after the last scene before has ended and before the
; first step of the ring buffer. Each step moves a few and the image table follows.
.proc cube_move
    lda #CUBE_MOVE_PER_STEP
    sta CUBE_MOVE_LEFT
@image:
    lda CUBE_MOVED
    cmp #CUBE_MOVE_SLOTS
    bcs @done
    clc
    adc #CUBE_MOVE_FIRST_SLOT
    tax
    jsr slot_image
    tay
    lda CUBE_IMAGE_LOW,y
    sta CUBE_SHOW_SRC
    lda CUBE_IMAGE_HIGH,y
    sta CUBE_SHOW_SRC + 1
    lda CUBE_MOVE_DST
    sta CUBE_IMAGE_LOW,y
    sta CUBE_SHOW_DST
    lda CUBE_MOVE_DST + 1
    sta CUBE_IMAGE_HIGH,y
    sta CUBE_SHOW_DST + 1
    ldy #CUBE_IMAGE_SIZE - 1
@byte:
    lda (CUBE_SHOW_SRC),y
    sta (CUBE_SHOW_DST),y
    dey
    bpl @byte
    lda CUBE_MOVE_DST
    clc
    adc #CUBE_IMAGE_SIZE
    sta CUBE_MOVE_DST
    bcc @moved
    inc CUBE_MOVE_DST + 1
@moved:
    inc CUBE_MOVED
    dec CUBE_MOVE_LEFT
    bne @image
@done:
    rts
.endproc

area_first:
    .byte 0, CUBE_HIGH_SLOTS, CUBE_HIGH_SLOTS + CUBE_TILE_FREE_SLOTS
    .byte CUBE_MOVE_FIRST_SLOT, CUBE_MOVE_FIRST_SLOT + CUBE_CHAR_SLOT_IMAGES
    .byte 255, 255
area_address:
    .word CUBE_HIGH_IMAGES, CUBE_TILE_FREE, CUBE_CHARSET_FREE, CUBE_CHAR_SLOTS, CUBE_SPRITE_BLOCKS
