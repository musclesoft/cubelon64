; CUBELON 64 - Main Entry Point
; Commodore 64 port of ZX Spectrum demo

.include "include/vic.inc"
.include "include/sid.inc"
.include "include/kernal.inc"

; Import display routines
.import init_display, clear_screen, draw_border

; Import pattern generation
.import pattern_test, anim_frame

; Import zero page variables
.importzp ptr1, ptr2

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

        ; Load custom charset for patterns
        jsr setup_charset

        ; Draw border around display area
        jsr draw_border

        ; Show test message
        ; DISABLED - text characters show as random tiles with custom charset
        ; jsr show_test_message

        ; Enable interrupts
        cli

        ; Main demo loop
main_loop:
        ; Wait for vertical blank
        jsr wait_frame

        ; Generate and display pattern
        jsr pattern_test

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

; Setup custom charset from pattern data
; Copies 256 tiles (2KB) to $2000 (character RAM)
setup_charset:
        ; Copy 2KB from pattern_charset to $2000
        lda #<pattern_charset
        sta ptr1
        lda #>pattern_charset
        sta ptr1+1
        lda #<$2000
        sta ptr2
        lda #>$2000
        sta ptr2+1

        ldx #8                ; 8 pages to copy (2KB)
        ldy #0
@page_loop:
        lda (ptr1),y
        sta (ptr2),y
        iny
        bne @page_loop

        ; Move to next page
        inc ptr1+1
        inc ptr2+1
        dex
        bne @page_loop

        ; Point VIC to use charset at $2000
        ; VIC bank is at $0000 (default)
        ; Charset offset = $2000 / $0800 = 4
        lda VIC_ADDR
        and #$F0              ; Clear charset bits
        ora #$08              ; Set to $2000 (bits 1-3 = 100)
        sta VIC_ADDR

        rts

; Import pattern charset data
.import pattern_charset

; Wait for next frame (raster line 255)
wait_frame:
        ; First wait until we're NOT at line 255
@wait1:
        lda VIC_RASTER
        cmp #255
        beq @wait1
        ; Now wait until we reach line 255
@wait2:
        lda VIC_RASTER
        cmp #255
        bne @wait2
        rts
