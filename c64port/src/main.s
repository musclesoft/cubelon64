.include "zeropage.inc"

; External functions
.import calc_rotation: absolute
.import render_to_charset: absolute
.import init_charset: absolute

; C64 system addresses
SCREEN_RAM = $0400
COLOR_RAM = $D800
VIC_BASE = $D000
VIC_CTRL1 = VIC_BASE + $11
VIC_CTRL2 = VIC_BASE + $16
VIC_MEMPTR = VIC_BASE + $18
VIC_BORDER = VIC_BASE + $20
VIC_BG = VIC_BASE + $21

; Screen addresses
SCREEN_DATA = $0400

; Colors
COLOR_BLACK = 0
COLOR_WHITE = 1
COLOR_DARK_GREY = 11
COLOR_MID_GREY = 12
COLOR_LIGHT_GREY = 15

.segment "CODE"

; Entry point - BASIC stub: 2016 SYS 2062
.word $0801           ; Load address for BASIC
.word @next_line
.word 2016            ; Line number
.byte $9E             ; SYS token
.byte " 2062", 0
@next_line:
.word 0               ; End of BASIC program

; Main program starts here
main:
    ; Initialize VIC-II for character mode with custom charset
    jsr init_screen

    ; Initialize custom charset
    jsr init_charset

    ; Configure VIC to use charset at $3000
    ; VM13-VM10 bits in $D018 control charset location
    ; Charset address = (VM13-VM11) * $0800
    ; $3000 / $0800 = 6, so we need VM13-VM11 = 110 = 6
    lda $D018
    and #$F0               ; Keep video matrix bits
    ora #$0C               ; Set charset to $3000 (bits 3-1 = 110)
    sta $D018

    ; Initialize rotation state
    lda #0
    sta frame
    sta angle

main_loop:
    ; Calculate rotated square corners
    jsr calc_rotation

    ; Render the square to charset
    jsr render_to_charset

    ; Update angle for next frame
    inc angle
    inc frame

    ; Simple delay
    jsr wait_frame

    ; Continue indefinitely
    jmp main_loop

; Initialize screen for character mode
init_screen:
    ; Set character mode: BMM=0, DEN=1, RSEL=1, YSCROLL=3
    lda #$1B
    sta VIC_CTRL1

    ; Set multicolor mode off: MCM=0, CSEL=1, XSCROLL=0
    lda #$08
    sta VIC_CTRL2

    ; Set border and background
    lda #COLOR_BLACK
    sta VIC_BORDER
    sta VIC_BG

    ; Clear screen
    ldx #0
    lda #$20              ; Space character
@clear_screen:
    sta SCREEN_DATA,x
    sta SCREEN_DATA+$0100,x
    sta SCREEN_DATA+$0200,x
    sta SCREEN_DATA+$02E8,x
    inx
    bne @clear_screen

    ; Set all screen colors to white
    ldx #0
    lda #COLOR_WHITE
@color_loop:
    sta COLOR_RAM,x
    sta COLOR_RAM+$0100,x
    sta COLOR_RAM+$0200,x
    sta COLOR_RAM+$02E8,x
    inx
    bne @color_loop

    rts

; Calculate rotated square corners (implemented in rotation.s)
; Render the rotated square (implemented in rotation.s)

; Wait for one frame
wait_frame:
    ldx #0
@delay:
    dex
    bne @delay
    rts
