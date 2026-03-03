; Memory dump test - writes known values we can verify
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
        ; Write marker pattern to screen RAM we can verify
        ; Pattern: $DE $AD $BE $EF at start of screen
        lda #$DE
        sta $0400
        lda #$AD
        sta $0401
        lda #$BE
        sta $0402
        lda #$EF
        sta $0403

        ; Write marker at where pattern should be (row 15, col 18)
        ; Offset = 15 * 40 + 18 = 618 = $26A
        lda #$CA
        sta $0400 + 618
        lda #$FE
        sta $0400 + 619
        lda #$BA
        sta $0400 + 620
        lda #$BE
        sta $0400 + 621

        ; Write to color RAM to verify it exists
        lda #7  ; Yellow
        sta $D800
        sta $D801
        sta $D802
        sta $D803

@loop:
        jmp @loop
