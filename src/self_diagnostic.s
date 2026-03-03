; Self-diagnostic - program reports its own state to screen
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
        ; Standard init
        sei
        lda #$35
        sta $01

        ; Blue border, black background (easy to see)
        lda #6
        sta VIC_BORDER
        lda #0
        sta VIC_BG0

        ; Clear screen
        ldx #0
        lda #32
@clear:
        sta $0400,x
        sta $0500,x
        sta $0600,x
        sta $06E8,x
        inx
        bne @clear

        ; White color
        ldx #0
        lda #1
@color:
        sta $D800,x
        sta $D900,x
        sta $DA00,x
        sta $DAE8,x
        inx
        bne @color

        ; Test 1: Can we write to screen?
        lda #84  ; 'T'
        sta $0400
        lda #69  ; 'E'
        sta $0401
        lda #83  ; 'S'
        sta $0402
        lda #84  ; 'T'
        sta $0403
        lda #49  ; '1'
        sta $0404

        ; Test 2: Copy charset to $2000
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

        ; Indicate charset loaded
        lda #79  ; 'O'
        sta $0405
        lda #75  ; 'K'
        sta $0406

        ; Test 3: Switch to custom charset
        lda VIC_ADDR
        and #$F0
        ora #$08
        sta VIC_ADDR

        ; Write known char codes to test charset
        ; Row 5: Fill with character 0 (should be blank)
        ldx #10
@test3:
        lda #0
        sta $0400 + 40*5,x
        dex
        bpl @test3

        ; Row 6: Fill with character 1 (should be checkerboard)
        ldx #10
@test4:
        lda #1
        sta $0400 + 40*6,x
        dex
        bpl @test4

        ; Row 7: Fill with character 160 (should be solid)
        ldx #10
@test5:
        lda #160
        sta $0400 + 40*7,x
        dex
        bpl @test5

        cli
@loop:
        jmp @loop
