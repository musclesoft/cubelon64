; Test custom charset - write specific characters to verify charset is working
.include "include/vic.inc"
.include "include/kernal.inc"

; BASIC stub: 10 SYS 2061
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

        ; Disable ROMs
        lda #$35
        sta $01

        ; Set border and background to black
        lda #COLOR_BLACK
        sta VIC_BORDER
        sta VIC_BG0

        ; Clear screen with spaces (char 32)
        ldx #0
        lda #32
@clear:
        sta SCREEN_RAM,x
        sta SCREEN_RAM+$100,x
        sta SCREEN_RAM+$200,x
        sta SCREEN_RAM+$2E8,x
        inx
        bne @clear

        ; Set all colors to white
        ldx #0
        lda #COLOR_WHITE
@color:
        sta COLOR_RAM,x
        sta COLOR_RAM+$100,x
        sta COLOR_RAM+$200,x
        sta COLOR_RAM+$2E8,x
        inx
        bne @color

        ; Copy charset to $2000
        lda #<pattern_charset
        sta ptr1
        lda #>pattern_charset
        sta ptr1+1
        lda #<$2000
        sta ptr2
        lda #>$2000
        sta ptr2+1

        ldx #8                ; 8 pages (2KB)
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
        ora #$08              ; Charset at $2000
        sta VIC_ADDR

        ; Draw a test pattern - write characters 0-15 in a row
        ldx #0
@draw_test:
        txa
        sta SCREEN_RAM + 40*12 + 12,x  ; Middle of screen
        inx
        cpx #16
        bne @draw_test

        ; Draw characters 160 and 32 to test space/solid
        lda #160
        sta SCREEN_RAM + 40*14 + 12
        sta SCREEN_RAM + 40*14 + 13
        sta SCREEN_RAM + 40*14 + 14
        lda #32
        sta SCREEN_RAM + 40*14 + 15
        sta SCREEN_RAM + 40*14 + 16
        sta SCREEN_RAM + 40*14 + 17

        cli

@loop:
        jmp @loop
