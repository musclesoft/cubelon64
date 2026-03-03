; Test the border drawing and see if clear_center works
.include "include/vic.inc"

.segment "LOADADDR"
        .word $0801

.segment "EXEHDR"
        .word @end
        .word 10
        .byte $9E
        .byte "2061"
        .byte 0
@end:   .word 0

.segment "STARTUP"
.import pattern_charset
.importzp ptr1, ptr2

_main:
        sei
        lda #$35
        sta $01

        lda #0
        sta VIC_BORDER
        sta VIC_BG0

        ; Copy charset
        lda #<pattern_charset
        sta ptr1
        lda #>pattern_charset
        sta ptr1+1
        lda #<$2000
        sta ptr2
        lda #>$2000
        sta ptr2+1

        ldx #8
        ldy #0
@copy:
        lda (ptr1),y
        sta (ptr2),y
        iny
        bne @copy
        inc ptr1+1
        inc ptr2+1
        dex
        bne @copy

        lda VIC_ADDR
        and #$F0
        ora #$08
        sta VIC_ADDR

        ; Fill screen with solid blocks (160)
        lda #160
        ldx #0
@fill:
        sta $0400,x
        sta $0500,x
        sta $0600,x
        sta $06E8,x
        inx
        bne @fill

        ; Set colors to grey
        lda #12  ; GRAY2
        ldx #0
@color:
        sta $D800,x
        sta $D900,x
        sta $DA00,x
        sta $DAE8,x
        inx
        bne @color

        ; Clear center - just do 5 rows manually for simplicity
        lda #32  ; Space
        ldx #20  ; 20 characters per row
@row10:
        sta $0400 + 40*10 + 10,x
        dex
        bpl @row10

        ldx #20
@row11:
        sta $0400 + 40*11 + 10,x
        dex
        bpl @row11

        ldx #20
@row12:
        sta $0400 + 40*12 + 10,x
        dex
        bpl @row12

        ldx #20
@row13:
        sta $0400 + 40*13 + 10,x
        dex
        bpl @row13

        ldx #20
@row14:
        sta $0400 + 40*14 + 10,x
        dex
        bpl @row14

        ; Draw 3 distinct test tiles in center of row 12
        lda #1  ; Checkerboard
        sta $0400 + 40*12 + 18
        lda #3  ; Horizontal stripes
        sta $0400 + 40*12 + 19
        lda #4  ; Vertical stripes
        sta $0400 + 40*12 + 20

        cli
@loop:
        jmp @loop

@row_count: .byte 0
@temp: .byte 0
