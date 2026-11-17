; flower_dissolve_show.s - The flower's dissolve on the screen
;
; The ZX frames 3951-3977 show every tile of the 8 by 8 grid as one and the same sequence of 25 pictures (flower_dissolve.s), the
; tile of the even rows DELAY frames ahead of the screen's frame, the tile of an odd row the mirror image of it, with one of 4
; delays (-2, 0, 2, 4) for each tile (flower_dissolve_kinds): 8 kinds of tiles, which a frame shows 8 pictures for. They are drawn by
; the simple multiplexer from screen A, whose chars are those of the 8 kinds (tile column j, tile row m of the kind k is the char
; 24 + 24 * j + 3 * k + m), and the sprite of each tile is the block of its picture (the pointers are written into
; SIMPLE_POINTERS). The blocks and the mirror images are made by the job (flower_dissolve_convert) once the morph's arrays are
; free; a block holds the sprite's 21 rows, the tile rows 1-21, as the tiles of the dissolve have nothing else.
;
; A frame has to change the chars of all 8 kinds, which takes 8000 cycles: too many for the vertical blank. So there are two
; charsets, shown on every other frame, and the job's main loop copies the chars of the frame to come into the one that is not
; shown (flower_dissolve_copy, which the frame's work requests by DD_COPY); the frame's work itself only switches the charset
; and writes the sprite pointers. The frames before the switch prepare the first charset and write the screen's codes, a tile
; row on each frame, while the flower's own multiplexer still shows the held flower, which these chars show the same way.

.include "include/shared_imports.inc"
.include "include/vic.inc"
.include "include/layout.inc"
.include "include/tile.inc"

DD_PREPARE_T = 20                ; The state of the cycle 14 in which the first charset is made, once the march has shown its last pictures
DD_ROWS_T = 23                   ; and from which the rows of the screen's codes are written, once the job has copied the chars
DD_BASE_T = 36                   ; The picture the kind with the delay d shows in the state t is t - 36 + d (0 is the held flower)
DD_BLOCKS = $6640                ; The sprite blocks: the pictures, then their mirror images, in two runs: 7 blocks here, where the first
DD_RUN_BLOCKS = 7                ; charset's kinds end (the chars 64-135) and the cube scene's leftovers lie, and the others from
DD_RUN_OFFSET = $0E40            ; $7640, where the second charset's kinds end
DD_BLOCK_POINTER = $99           ; (DD_BLOCKS - $4000) / 64
DD_RUN_POINTERS = $39            ; the pointer of the second run's blocks lead by this
DD_MIRRORED = FLOWER_IMAGES      ; The mirror images of the pictures, in the image format
DD_MEMPTR = $08                  ; Screen A and the charset at $6000; $0C for the charset at $7000, which the marching flowers' states between
DD_CHARSET_B = $70               ; (FLOWER_BETWEEN_LOW, to $7280) leave free from the state 20 of the cycle 14
DD_KIND_CHARS = 512              ; The first char of the kinds is the char 64: the char 63, which fills the sides of the screen, stays
DD_SPRITE_ROWS = 21
DD_FIRST_CODE = 64

DD_T = DISSOLVE_VARS
DD_N = DISSOLVE_VARS + 1
DD_NEXT = DISSOLVE_VARS + 5
DD_P = DISSOLVE_VARS + 9
DD_COPY_HI = DISSOLVE_VARS + 17
DD_KIND = DISSOLVE_VARS + 19
DD_COL = DISSOLVE_VARS + 20
DD_ROW = DISSOLVE_VARS + 21
DD_LINE = DISSOLVE_VARS + 22
DD_IDX = DISSOLVE_VARS + 23
DD_TMP = DISSOLVE_VARS + 24
DD_BASE = DISSOLVE_VARS + 25

.segment "ZEROPAGE"

DD_SRC: .res 2
DD_DST: .res 2

.segment "BSS"

DD_COPY: .res 1                  ; Nonzero while the chars of a frame wait to be copied (job_init clears it)
.export DD_COPY

.import flower_dissolve_kinds: absolute
.import MUX_DISSOLVE: absolute

.export flower_dissolve_tick
.export flower_dissolve_copy
.export flower_dissolve_convert

.segment "FLOWER47"

; ─── The work of a frame of the dissolve, after the multiplexer's ────────────
; T is the state of the cycle 14 (the frame 3919 + T): the first charset is made in the state 16, the rows of codes follow, and
; from the state 28 on, when the simple multiplexer shows the screen, every frame writes its pointers
.proc flower_dissolve_tick
    lda SEQ_PHASE
    cmp #DISSOLVE_PHASE
    beq @cycle_14
    cmp #DISSOLVE_PHASE + 1
    bne @done
    lda SEQ_STATE
    clc
    adc #40
    bne @state
@cycle_14:
    lda SEQ_STATE
@state:
    sta DD_T
    cmp #DD_PREPARE_T
    bcc @done
    beq flower_dissolve_prepare
    cmp #DISSOLVE_SWITCH_STATE
    bcs flower_dissolve_frame
    cmp #DISSOLVE_SWITCH_STATE - 1
    bne @rows
    inc MUX_DISSOLVE
@rows:
    sec
    sbc #DD_ROWS_T
    cmp #8
    bcs @done
    jmp flower_dissolve_write_row
@done:
    rts
.endproc

; The first charset with the held picture in all kinds, and the pointers of its sprite blocks
.proc flower_dissolve_prepare
    ldx #3
    lda #0
@held:
    sta DD_N,x
    sta DD_NEXT,x
    dex
    bpl @held
    lda #>CHARSET
    sta DD_COPY_HI
    lda #2
    sta DD_COPY
    jmp flower_dissolve_pointers
.endproc

.segment "FLOWER52"
; The block of the mirror image of the picture 0 (the image SHOW_IMAGE_A, shown by the odd rows) and of the picture 0 (SHOW_IMAGE_B, the
; even rows')
flower_dissolve_hold_blocks:
    .byte DISSOLVE_IMAGE_COUNT, 0
.segment "FLOWER47"

; The pictures of the frame's state T, the sprite pointers and the charset of the frame; the chars of the frame after it are asked for
.proc flower_dissolve_frame
    ldx #3
@current:
    lda DD_NEXT,x
    sta DD_N,x
    dex
    bpl @current
    jsr flower_dissolve_pointers

    lda DD_T
    eor #1
    and #1
    asl
    asl
    ora #DD_MEMPTR
    sta VIC_MEMPTR

    ldx #3
@next:
    txa
    asl
    sec
    adc DD_T
    sec
    sbc #DD_BASE_T
    jsr flower_dissolve_clamp
    sta DD_NEXT,x
    dex
    bpl @next
    lda DD_T
    and #1
    beq @charset_a
    lda #DD_CHARSET_B
    bne @hi
@charset_a:
    lda #>CHARSET
@hi:
    sta DD_COPY_HI
    lda #1
    sta DD_COPY
    rts
.endproc

; A = the signed picture number limited to 0 - the last
.proc flower_dissolve_clamp
    bmi @first
    cmp #DISSOLVE_IMAGE_COUNT
    bcc @done
    lda #DISSOLVE_IMAGE_COUNT - 1
    rts
@first:
    lda #0
@done:
    rts
.endproc

; A = the number of a sprite block: the pointer to it
.proc flower_dissolve_block_pointer
    cmp #DD_RUN_BLOCKS
    bcc @first_run
    adc #DD_RUN_POINTERS - 1
@first_run:
    clc
    adc #DD_BLOCK_POINTER
    rts
.endproc

; The sprite pointers of the 8 kinds for the pictures DD_N, and of every tile for its kind; the first band's are written at once, as the
; interrupt that moves the sprites there has just run
.proc flower_dissolve_pointers
    ldx #3
@kind:
    lda DD_N,x
    jsr flower_dissolve_block_pointer
    sta DD_P,x
    lda DD_N,x
    clc
    adc #DISSOLVE_IMAGE_COUNT
    jsr flower_dissolve_block_pointer
    sta DD_P + 4,x
    dex
    bpl @kind
    ldy #63
@tile:
    ldx flower_dissolve_kinds,y
    lda DD_P,x
    sta SIMPLE_POINTERS,y
    dey
    bpl @tile
    ldx #7
@band_0:
    txa
    asl
    asl
    asl
    tay
    lda SIMPLE_POINTERS,y
    sta POINTERS_A,x
    dex
    bpl @band_0
    rts
.endproc

; ─── Write the codes of the tile row A into screen A ─────────────────────────
.segment "FLOWER48"

.proc flower_dissolve_write_row
    sta DD_ROW
    tax
    lda #<GRID_ORIGIN
    sta DD_DST
    lda #>GRID_ORIGIN
    sta DD_DST + 1
    txa
    beq @ready
@row:
    clc
    lda DD_DST
    adc #120
    sta DD_DST
    bcc @same_page
    inc DD_DST + 1
@same_page:
    dex
    bne @row
@ready:
    lda #0
    sta DD_LINE
@line:
    lda DD_ROW
    sta DD_IDX
    ldy #0
@tile:
    ldx DD_IDX
    lda flower_dissolve_kinds,x
    sta DD_TMP
    asl
    clc
    adc DD_TMP
    clc
    adc DD_LINE
    adc #DD_FIRST_CODE
    sta (DD_DST),y
    iny
    clc
    adc #24
    sta (DD_DST),y
    iny
    clc
    adc #24
    sta (DD_DST),y
    iny
    lda DD_IDX
    clc
    adc #8
    sta DD_IDX
    cpy #24
    bcc @tile
    clc
    lda DD_DST
    adc #40
    sta DD_DST
    bcc @next_line
    inc DD_DST + 1
@next_line:
    inc DD_LINE
    lda DD_LINE
    cmp #3
    bcc @line
    rts
.endproc

; ─── Copy the chars of the frame to come into the charset that is not shown ──
; DD_NEXT = the pictures of the 4 delays, DD_COPY_HI = the high byte of the charset, DD_COPY = 2 for the first request, which makes
; the held pictures first. The main loop calls this between the interrupts.
.segment "FLOWER49"

.proc flower_dissolve_copy
    lda DD_COPY
    bne @work
    rts
@work:
    cmp #2
    bne @kinds
    jsr flower_dissolve_hold
@kinds:
    jsr flower_dissolve_copy_kinds
    lda #0
    sta DD_COPY
    rts
.endproc

; The pictures 0 are the images the flower's multiplexer shows now (the marching flowers' last), whose edges differ in some pixels from
; the proto's flower: the addresses of the pictures 0 and their mirror images point at them and their blocks are made from them. The kinds'
; chars of both charsets are made black, as the copy leaves the rows 0, 22 and 23 of each column alone, and the char 63 of the second
; charset, which fills the sides of the screen (the first charset's is still black), with the chars 56-62
.proc flower_dissolve_hold
    lda #$FF
    ldy #0
@black:
    sta CHARSET + DD_KIND_CHARS,y
    sta CHARSET + DD_KIND_CHARS + 256,y
    sta DD_CHARSET_B * 256 + DD_KIND_CHARS,y
    sta DD_CHARSET_B * 256 + DD_KIND_CHARS + 256,y
    iny
    bne @black
    ldy #63
@black_rest:
    sta CHARSET + DD_KIND_CHARS + 512,y
    sta DD_CHARSET_B * 256 + DD_KIND_CHARS + 512,y
    sta DD_CHARSET_B * 256 + 448,y
    dey
    bpl @black_rest
    ldx #2
@hold:
    txa
    lsr
    tay
    lda flower_dissolve_hold_blocks,y
    pha
    asl
    tay
    lda SHOW_IMAGE_A,x
    sta FLOWER_PTR
    sta flower_dissolve_addresses,y
    lda SHOW_IMAGE_A + 1,x
    sta FLOWER_PTR + 1
    sta flower_dissolve_addresses + 1,y
    stx DD_COL
    pla
    jsr flower_dissolve_block
    ldx DD_COL
    dex
    dex
    bpl @hold
    rts
.endproc

.proc flower_dissolve_copy_kinds
    lda #0
    sta DD_KIND
@kind:
    lda DD_KIND
    and #3
    tax
    lda DD_NEXT,x
    ldy DD_KIND
    cpy #4
    bcs @mirrored
    jsr flower_dissolve_plain_image
    jmp @source
@mirrored:
    jsr flower_dissolve_mirror_image
@source:
    sta DD_SRC
    sty DD_SRC + 1
    lda DD_KIND
    asl
    asl
    asl
    sta DD_TMP
    asl
    clc
    adc DD_TMP
    sta DD_DST
    lda DD_COPY_HI
    adc #>DD_KIND_CHARS
    sta DD_DST + 1
    lda #3
    sta DD_COL
@column:
    ldy #21
@bytes:
    lda (DD_SRC),y
    sta (DD_DST),y
    dey
    lda (DD_SRC),y
    sta (DD_DST),y
    dey
    lda (DD_SRC),y
    sta (DD_DST),y
    dey
    bne @bytes
    clc
    lda DD_SRC
    adc #24
    sta DD_SRC
    bcc @source_page
    inc DD_SRC + 1
@source_page:
    clc
    lda DD_DST
    adc #192
    sta DD_DST
    bcc @destination_page
    inc DD_DST + 1
@destination_page:
    dec DD_COL
    bne @column
    inc DD_KIND
    lda DD_KIND
    cmp #8
    bcc @kind
    rts
.endproc

; A = the picture number: the address of its image in A (low) and Y (high), of its mirror image after flower_dissolve_mirror_image
.proc flower_dissolve_plain_image
    asl
    jmp flower_dissolve_address
.endproc

.proc flower_dissolve_mirror_image
    asl
    clc
    adc #2 * DISSOLVE_IMAGE_COUNT
.endproc

.proc flower_dissolve_address
    tax
    lda flower_dissolve_addresses + 1,x
    tay
    lda flower_dissolve_addresses,x
    rts
.endproc

; ─── Make the mirror images and the sprite blocks of the 25 pictures ─────────
; The job's build once the morph's arrays are free (the mirror images lie on the morph's even and odd states, the blocks on the
; mirrored morph's): the picture n has the block n and the mirror image the block 25 + n. A block holds the tile rows 1-21 of
; the image's sprite, 3 bytes each.
.segment "FLOWER50"

.proc flower_dissolve_convert
    lda #0
    sta DS_IMAGE
@image:
    lda DS_IMAGE
    jsr flower_dissolve_plain_image
    sta FLOWER_PTR
    sty FLOWER_PTR + 1
    lda DS_IMAGE
    jsr flower_dissolve_mirror_image
    sta FLOWER_PTR2
    sty FLOWER_PTR2 + 1
    jsr flower_mirror_image
    lda DS_IMAGE
    jsr flower_dissolve_block
    lda DS_IMAGE
    jsr flower_dissolve_mirror_image
    sta FLOWER_PTR
    sty FLOWER_PTR + 1
    lda DS_IMAGE
    clc
    adc #DISSOLVE_IMAGE_COUNT
    jsr flower_dissolve_block
    inc DS_IMAGE
    lda DS_IMAGE
    cmp #DISSOLVE_IMAGE_COUNT
    bcc @image
    rts
.endproc

; The block A of the sprite blocks from the sprite of the image at FLOWER_PTR
.proc flower_dissolve_block
    sta DD_TMP
    lsr
    lsr
    sta DD_IDX
    lda DD_TMP
    asl
    asl
    asl
    asl
    asl
    asl
    clc
    adc #<DD_BLOCKS
    sta FLOWER_PTR2
    lda DD_IDX
    adc #>DD_BLOCKS
    sta FLOWER_PTR2 + 1
    lda DD_TMP
    cmp #DD_RUN_BLOCKS
    bcc @address_ready
    lda FLOWER_PTR2
    clc
    adc #<DD_RUN_OFFSET
    sta FLOWER_PTR2
    lda FLOWER_PTR2 + 1
    adc #>DD_RUN_OFFSET
    sta FLOWER_PTR2 + 1
@address_ready:
    lda #0
    sta DD_LINE
@row:
    ldx DD_LINE
    inx
    ldy sprite_image_index,x
    lda (FLOWER_PTR),y
    sta DD_TMP
    iny
    lda (FLOWER_PTR),y
    sta DD_IDX
    iny
    lda (FLOWER_PTR),y
    sta DD_BASE
    lda DD_LINE
    asl
    clc
    adc DD_LINE
    tay
    lda DD_TMP
    sta (FLOWER_PTR2),y
    iny
    lda DD_IDX
    sta (FLOWER_PTR2),y
    iny
    lda DD_BASE
    sta (FLOWER_PTR2),y
    inc DD_LINE
    lda DD_LINE
    cmp #DD_SPRITE_ROWS
    bcc @row
    rts
.endproc

; The addresses of the 25 pictures' images and then of their mirror images
flower_dissolve_addresses:
    .word early_start + 0
    .word early_start + 144
    .word early_start + 288
    .word early_start + 432
    .word early_start + 576
    .word early_start + 720
    .word early_start + 864
    .word early_start + 1008
    .word early_start + 1152
    .word early_start + 1296
    .word early_start + 1440
    .word early_start + 1584
    .word early_start + 1728
    .word early_start + 1872
    .word early_start + 2016
    .word early_start + 2160
    .word early_start + 2304
    .word early_start + 2448
    .word early_start + 2592
    .word early_start + 2736
    .word early_start + 2880
    .word early_start + 3024
    .word spin_half_tiles + 0
    .word DD_MIRRORED + 0
    .word DD_MIRRORED + 144
    .word DD_MIRRORED + 288
    .word DD_MIRRORED + 432
    .word DD_MIRRORED + 576
    .word DD_MIRRORED + 720
    .word DD_MIRRORED + 864
    .word DD_MIRRORED + 1008
    .word DD_MIRRORED + 1152
    .word DD_MIRRORED + 1296
    .word DD_MIRRORED + 1440
    .word DD_MIRRORED + 1584
    .word DD_MIRRORED + 1728
    .word DD_MIRRORED + 1872
    .word DD_MIRRORED + 2016
    .word DD_MIRRORED + 2160
    .word DD_MIRRORED + 2304
    .word DD_MIRRORED + 2448
    .word DD_MIRRORED + 2592
    .word DD_MIRRORED + 2736
    .word DD_MIRRORED + 2880
    .word DD_MIRRORED + 3024
    .word DD_MIRRORED + 3168
