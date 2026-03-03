; Display routines for CUBELON 64

.include "include/vic.inc"
.include "include/kernal.inc"

.export init_display
.export clear_screen
.export draw_border

; Display area dimensions (centered square on screen)
DISPLAY_WIDTH  = 32  ; Characters wide
DISPLAY_HEIGHT = 24  ; Characters high
DISPLAY_X      = 4   ; Start column (40-32)/2 = 4
DISPLAY_Y      = 0   ; Start row

.segment "CODE"

; Initialize display system
init_display:
        ; Set border and background to black
        lda #COLOR_BLACK
        sta VIC_BORDER
        sta VIC_BG0

        ; Standard text mode, 40 columns, 25 rows
        lda #VIC_CTRL1_DEN | VIC_CTRL1_RSEL | $03
        sta VIC_CTRL1

        lda #VIC_CTRL2_CSEL
        sta VIC_CTRL2

        ; Screen at $0400, charset at default
        lda #$18
        sta VIC_ADDR

        rts

; Clear entire screen
clear_screen:
        ldx #0
        lda #$20  ; Space character
@loop1:
        sta SCREEN_RAM,x
        sta SCREEN_RAM+$100,x
        sta SCREEN_RAM+$200,x
        sta SCREEN_RAM+$2E8,x
        inx
        bne @loop1

        ; Set color to white
        ldx #0
        lda #COLOR_WHITE
@loop2:
        sta COLOR_RAM,x
        sta COLOR_RAM+$100,x
        sta COLOR_RAM+$200,x
        sta COLOR_RAM+$2E8,x
        inx
        bne @loop2

        rts

; Draw border around display area
draw_border:
        ; Fill screen with solid blocks and set border color
        ldx #0
        lda #$A0  ; Inverted space = solid block
@loop:
        sta SCREEN_RAM,x
        sta SCREEN_RAM+$100,x
        sta SCREEN_RAM+$200,x
        sta SCREEN_RAM+$2E8,x
        inx
        bne @loop

        ; Set border area color to gray
        ldx #0
        lda #COLOR_GRAY2
@color_loop:
        sta COLOR_RAM,x
        sta COLOR_RAM+$100,x
        sta COLOR_RAM+$200,x
        sta COLOR_RAM+$2E8,x
        inx
        bne @color_loop

        ; Clear center 32x24 area
        ; Row counter in A
        lda #0
@row_loop:
        pha  ; Save row counter

        ; Calculate screen offset (row * 40) + DISPLAY_X
        ; A has row number
        asl
        asl
        asl  ; A = row * 8
        sta @temp
        pla
        pha
        asl
        asl
        asl
        asl
        asl  ; A = row * 32
        clc
        adc @temp  ; A = row * 40
        clc
        adc #DISPLAY_X  ; Add starting column
        tay

        ; Clear this row - Y is base offset, X is column counter
        ldx #0
@clear_col:
        lda #$20  ; Space
        sta SCREEN_RAM,y
        lda #COLOR_WHITE
        sta COLOR_RAM,y
        iny
        inx
        cpx #DISPLAY_WIDTH
        bne @clear_col

        pla  ; Restore row counter
        clc
        adc #1
        cmp #DISPLAY_HEIGHT
        bne @row_loop

        rts

@temp:  .byte 0

; Removed fill_pattern for now - will reimplement properly later
