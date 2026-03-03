; Display routines for CUBELON 64

.include "include/vic.inc"
.include "include/kernal.inc"

.export init_display
.export clear_screen
.export draw_border
.importzp ptr1, ptr2

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
        ; Screen is already cleared to spaces by clear_screen
        ; Just need to draw the border pattern
        ; For now, do nothing - rely on clear_screen
        ; TODO: Add actual border pattern drawing
        rts

; Removed fill_pattern for now - will reimplement properly later
