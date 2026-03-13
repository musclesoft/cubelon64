; Character set management for C64
; Manages a 3x3 character area (24x24 pixels) for rendering the rotated square

; Charset location in RAM
CHARSET_RAM = $6000    ; VIC bank 1 + $2000 = $6000-$67FF
SQUARE_CHARS = CHARSET_RAM + (64 * 8)  ; Start at character 64 (9 chars: 64-72)

.segment "CODE"

; Initialize charset - create simple pattern for testing
.export init_charset
init_charset:
    ; For now, just clear entire charset (emulator doesn't have ROM loaded)
    ; In a real C64 or complete emulator, we'd copy from $D000
    lda #$00
    tax
@clear_all:
    sta CHARSET_RAM,x
    sta CHARSET_RAM+$100,x
    sta CHARSET_RAM+$200,x
    sta CHARSET_RAM+$300,x
    sta CHARSET_RAM+$400,x
    sta CHARSET_RAM+$500,x
    sta CHARSET_RAM+$600,x
    sta CHARSET_RAM+$700,x
    inx
    bne @clear_all

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
