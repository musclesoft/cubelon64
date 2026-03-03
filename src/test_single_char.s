; Ultra-simple test - just fill screen with character code 65 ('A' in ROM charset)
; NO custom charset at all
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
        ; Don't disable ROMs - use standard config

        ; Set colors
        lda #6              ; Blue
        sta VIC_BORDER
        lda #0              ; Black
        sta VIC_BG0

        ; Fill screen with 'A' (code 65 = $41)
        ldx #0
        lda #65
@loop:
        sta $0400,x
        sta $0500,x
        sta $0600,x
        sta $06E8,x
        inx
        bne @loop

        ; Set all colors to white
        ldx #0
        lda #1
@color:
        sta $D800,x
        sta $D900,x
        sta $DA00,x
        sta $DAE8,x
        inx
        bne @color

@forever:
        jmp @forever
