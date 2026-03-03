; Memory dumper - saves screen RAM to a file via KERNAL
; This version just exits immediately so we can examine memory in VICE monitor
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
.import pattern_charset, pattern_test, init_display, clear_screen, draw_border
.importzp ptr1, ptr2

_main:
        sei
        lda #$35
        sta $01

        ; Run the actual program
        jsr clear_screen
        jsr init_display

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

        jsr draw_border
        jsr pattern_test

        cli

        ; Just sit here - examine with VICE monitor
@loop:
        jmp @loop
