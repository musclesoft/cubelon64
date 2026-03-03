; Simplest possible test - just draw some recognizable characters using ROM charset
.include "include/vic.inc"

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
_main:
        ; Set border and background
        lda #COLOR_BLUE
        sta VIC_BORDER
        lda #COLOR_BLACK
        sta VIC_BG0

        ; Clear screen
        ldx #0
        lda #32  ; Space
@clear:
        sta $0400,x
        sta $0500,x
        sta $0600,x
        sta $06E8,x
        inx
        bne @clear

        ; Set all colors to white
        ldx #0
        lda #COLOR_WHITE
@color:
        sta $D800,x
        sta $D900,x
        sta $DA00,x
        sta $DAE8,x
        inx
        bne @color

        ; Draw a diagonal line of asterisks (char 42)
        ldx #0
@diag:
        lda #42  ; Asterisk
        sta $0400,x
        sta $0400 + 41,x
        sta $0400 + 82,x
        sta $0400 + 123,x
        sta $0400 + 164,x
        inx
        cpx #20
        bne @diag

        ; Draw a box using PETSCII box characters
        ; Top line
        lda #$70  ; Horizontal line
        ldx #0
@top:
        sta $0400 + 40*10 + 10,x
        inx
        cpx #20
        bne @top

        ; Bottom line
        ldx #0
@bottom:
        sta $0400 + 40*20 + 10,x
        inx
        cpx #20
        bne @bottom

        ; Sides
        lda #$7D  ; Vertical line
        ldx #0
@sides:
        sta $0400 + 40*(11+x) + 10
        sta $0400 + 40*(11+x) + 29
        inx
        cpx #9
        bne @sides

@loop:
        jmp @loop
