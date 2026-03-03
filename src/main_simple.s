; Simple test - just display characters using ROM charset
; No custom charset, no patterns - just verify display works

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
.export _main
_main:
        ; Set border and background to black
        lda #COLOR_BLACK
        sta VIC_BORDER
        sta VIC_BG0

        ; Clear screen with spaces
        ldx #0
@clear:
        lda #$20          ; Space
        sta SCREEN_RAM,x
        sta SCREEN_RAM+$100,x
        sta SCREEN_RAM+$200,x
        sta SCREEN_RAM+$2E8,x
        lda #COLOR_WHITE
        sta COLOR_RAM,x
        sta COLOR_RAM+$100,x
        sta COLOR_RAM+$200,x
        sta COLOR_RAM+$2E8,x
        inx
        bne @clear

        ; Write test message
        ldx #0
@msg:
        lda message,x
        beq @done
        sta SCREEN_RAM + 40*12 + 10,x
        inx
        bne @msg

@done:
        ; Loop forever
        jmp @done

message:
        .byte "HELLO WORLD!"
        .byte 0
