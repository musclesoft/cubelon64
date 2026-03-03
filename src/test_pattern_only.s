; Test pattern drawing without border
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

        ; Clear entire screen with space (32)
        lda #32
        ldx #0
@clear:
        sta $0400,x
        sta $0500,x
        sta $0600,x
        sta $06E8,x
        inx
        bne @clear

        ; Set all colors to white
        lda #1
        ldx #0
@color:
        sta $D800,x
        sta $D900,x
        sta $DA00,x
        sta $DAE8,x
        inx
        bne @color

        ; Draw 3x3 pattern at row 10, col 15
        ; Row 1
        lda #1
        sta $0400 + 40*10 + 15
        lda #2
        sta $0400 + 40*10 + 16
        lda #1
        sta $0400 + 40*10 + 17

        ; Row 2
        lda #2
        sta $0400 + 40*11 + 15
        lda #3
        sta $0400 + 40*11 + 16
        lda #2
        sta $0400 + 40*11 + 17

        ; Row 3
        lda #1
        sta $0400 + 40*12 + 15
        lda #2
        sta $0400 + 40*12 + 16
        lda #1
        sta $0400 + 40*12 + 17

        cli
@loop:
        jmp @loop
