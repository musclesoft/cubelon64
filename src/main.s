; CUBELON 64 - Main Entry Point
; Commodore 64 port of ZX Spectrum demo

.include "include/vic.inc"
.include "include/sid.inc"
.include "include/kernal.inc"

; Import display routines
.import init_display, clear_screen, draw_border

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

; Main code entry point
.segment "STARTUP"
.export _main
_main:
        ; Disable interrupts during init
        sei

        ; Turn off CIA interrupts
        lda #$7F
        sta CIA1_ICR
        sta CIA2_ICR
        lda CIA1_ICR
        lda CIA2_ICR

        ; Disable KERNAL and BASIC ROMs
        lda #$35
        sta $01

        ; Clear screen with black
        jsr clear_screen

        ; Initialize VIC-II for demo
        jsr init_display

        ; Initialize SID (silence)
        jsr init_sid

        ; Show test message
        jsr show_test_message

        ; Enable interrupts
        cli

        ; Main demo loop
main_loop:
        ; Wait for vertical blank
        jsr wait_frame

        ; TODO: Update demo part

        ; TODO: Check for exit key

        jmp main_loop

; Show test message (temporary)
show_test_message:
        ldx #0
@loop:
        lda test_msg,x
        beq @done
        sta SCREEN_RAM + 40*12 + 8,x  ; Center of screen
        lda #COLOR_WHITE
        sta COLOR_RAM + 40*12 + 8,x
        inx
        bne @loop
@done:
        rts

test_msg:
        .byte "CUBELON 64 - INIT OK"
        .byte 0

; Initialize SID (all voices off, volume 0)
init_sid:
        ldx #0
@loop:
        lda #0
        sta SID_V1_FREQ_LO,x
        inx
        cpx #25
        bne @loop

        ; Set volume to 15
        lda #15
        sta SID_MODE_VOL

        rts

; Wait for next frame (raster line 0)
wait_frame:
@wait:
        lda VIC_RASTER
        bne @wait
        rts
