; Absolutely minimal test - just fill screen with character 1 (checkerboard)
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

        ; Black border/background
        lda #0
        sta VIC_BORDER
        sta VIC_BG0

        ; Copy charset to $2000
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

        ; Point VIC to charset at $2000
        lda VIC_ADDR
        and #$F0
        ora #$08
        sta VIC_ADDR

        ; Fill entire screen with character 1 (checkerboard pattern)
        lda #1
        ldx #0
@fill:
        sta $0400,x
        sta $0500,x
        sta $0600,x
        sta $06E8,x
        inx
        bne @fill

        ; Set all to white
        lda #1
        ldx #0
@color:
        sta $D800,x
        sta $D900,x
        sta $DA00,x
        sta $DAE8,x
        inx
        bne @color

        cli
@loop:
        jmp @loop
