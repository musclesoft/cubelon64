; Diagnostic test with clear visual markers
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
_main:
        ; Border RED, Background BLUE
        lda #2              ; RED
        sta VIC_BORDER
        lda #6              ; BLUE
        sta VIC_BG0

        ; Clear screen with spaces
        ldx #0
        lda #32
@clear:
        sta $0400,x
        sta $0500,x
        sta $0600,x
        sta $06E8,x
        inx
        bne @clear

        ; Top-left corner: Write "1234" in YELLOW
        lda #49             ; '1'
        sta $0400
        lda #50             ; '2'
        sta $0401
        lda #51             ; '3'
        sta $0402
        lda #52             ; '4'
        sta $0403

        ldx #0
@yellow:
        lda #7              ; YELLOW
        sta $D800,x
        inx
        cpx #4
        bne @yellow

        ; Center: Write "TEST" in WHITE
        lda #84             ; 'T'
        sta $0400 + 40*12 + 18
        lda #69             ; 'E'
        sta $0400 + 40*12 + 19
        lda #83             ; 'S'
        sta $0400 + 40*12 + 20
        lda #84             ; 'T'
        sta $0400 + 40*12 + 21

        ldx #0
@white:
        lda #1              ; WHITE
        sta $D800 + 40*12 + 18,x
        inx
        cpx #4
        bne @white

@loop:
        jmp @loop
