; Character set management for C64
; Manages a 3x3 character area (24x24 pixels) for rendering the rotated square

.include "zeropage.inc"

; Charset location in RAM
CHARSET_RAM = $3000    ; Custom charset at $3000-$37FF
SQUARE_CHARS = CHARSET_RAM + (64 * 8)  ; Start at character 64 (9 chars: 64-72)

.segment "CODE"

; Initialize charset - copy ROM charset and clear square area
.export init_charset
init_charset:
    ; Copy ROM charset to RAM
    ; First, enable ROM at $D000
    lda #$33               ; BASIC off, KERNAL on, I/O off (expose char ROM)
    sta $01

    ; Copy charset from $D000-$D7FF to $3000-$37FF
    ldx #$00
@copy_loop:
    lda $D000,x
    sta CHARSET_RAM,x
    lda $D100,x
    sta CHARSET_RAM+$100,x
    lda $D200,x
    sta CHARSET_RAM+$200,x
    lda $D300,x
    sta CHARSET_RAM+$300,x
    lda $D400,x
    sta CHARSET_RAM+$400,x
    lda $D500,x
    sta CHARSET_RAM+$500,x
    lda $D600,x
    sta CHARSET_RAM+$600,x
    lda $D700,x
    sta CHARSET_RAM+$700,x
    inx
    bne @copy_loop

    ; Restore normal memory map
    lda #$37               ; BASIC on, KERNAL on, I/O on
    sta $01

    ; Clear the 9 characters used for the square
    lda #$00
    ldx #72                ; 9 characters * 8 bytes = 72 bytes
@clear_loop:
    sta SQUARE_CHARS-1,x
    dex
    bne @clear_loop

    ; Set up screen to display the square characters
    ; Place characters 64-72 in a 3x3 grid at screen position (8,11)
    ; Screen position = row*40 + col = 11*40 + 8 = 448
    ldx #0
    ldy #0
@setup_screen:
    txa
    clc
    adc #64                ; Character 64 + offset
    sta $0400 + 11*40 + 8,x  ; Row 0
    sta $0400 + 12*40 + 8,x  ; Row 1
    sta $0400 + 13*40 + 8,x  ; Row 2
    inx
    cpx #3
    bne @setup_screen

    ; Actually we need to properly layout 3x3
    lda #64
    sta $0400 + 11*40 + 8
    lda #65
    sta $0400 + 11*40 + 9
    lda #66
    sta $0400 + 11*40 + 10

    lda #67
    sta $0400 + 12*40 + 8
    lda #68
    sta $0400 + 12*40 + 9
    lda #69
    sta $0400 + 12*40 + 10

    lda #70
    sta $0400 + 13*40 + 8
    lda #71
    sta $0400 + 13*40 + 9
    lda #72
    sta $0400 + 13*40 + 10

    ; Set colors to white on black
    ldx #0
@color_loop:
    lda #$01               ; White foreground
    sta $D800 + 11*40 + 8,x
    sta $D800 + 12*40 + 8,x
    sta $D800 + 13*40 + 8,x
    inx
    cpx #3
    bne @color_loop

    rts

; Render a filled square to the 3x3 character area
; For now, just draw a simple test pattern
.export render_to_charset
render_to_charset:
    ; Clear all 9 characters
    lda #$00
    ldx #72
@clear:
    sta SQUARE_CHARS-1,x
    dex
    bne @clear

    ; Draw a simple 12x12 filled square centered in the 24x24 area
    ; Center pixels: x=6..17, y=6..17

    ldy #0                 ; Y = pixel row counter (0..23)
@row_loop:
    ; Check if this row is in the square (y >= 6 and y < 18)
    cpy #6
    bcc @next_row
    cpy #18
    bcs @next_row

    ; This row is part of the square
    ; Calculate character row (0, 1, or 2) = Y / 8
    tya
    lsr
    lsr
    lsr
    tax                    ; X = character row

    ; Calculate line within character = Y & 7
    tya
    and #$07
    sta temp2              ; temp2 = line within character

    ; For this row, draw pixels 6-17 across 3 characters
    ; Character 0: pixels 0-7, we want pixels 6-7 = bits 1-0 = $03
    ; Character 1: pixels 8-15, we want pixels 8-15 (all) = $FF
    ; Character 2: pixels 16-23, we want pixels 16-17 = bits 7-6 = $C0

    ; Character offset for row X: X * 3 * 8 (3 chars per row, 8 bytes per char)
    ; = X * 24
    txa
    asl                    ; * 2
    asl                    ; * 4
    asl                    ; * 8
    sta temp3              ; temp3 = X * 8
    txa
    asl                    ; * 2
    asl                    ; * 4
    clc
    adc temp3
    sta temp3              ; temp3 = X * 8 + X * 4 = X * 12
    asl                    ; * 24
    clc
    adc temp2              ; Add line offset
    tax                    ; X = byte offset for char 0

    ; Write to character 0
    lda #$03               ; Bits 1-0 set (pixels 6-7)
    sta SQUARE_CHARS,x

    ; Write to character 1 (8 bytes later)
    lda #$FF               ; All bits set
    sta SQUARE_CHARS+8,x

    ; Write to character 2 (16 bytes later)
    lda #$C0               ; Bits 7-6 set (pixels 16-17)
    sta SQUARE_CHARS+16,x

@next_row:
    iny
    cpy #24
    bne @row_loop

    rts
