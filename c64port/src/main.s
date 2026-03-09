.include "zeropage.inc"

; C64 system addresses
SCREEN_RAM = $0400
COLOR_RAM = $D800
VIC_BASE = $D000
VIC_CTRL1 = VIC_BASE + $11
VIC_CTRL2 = VIC_BASE + $16
VIC_MEMPTR = VIC_BASE + $18
VIC_BORDER = VIC_BASE + $20
VIC_BG = VIC_BASE + $21

; Bitmap mode addresses
BITMAP_RAM = $2000
SCREEN_DATA = $0400

; Colors
COLOR_BLACK = 0
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
    ; Initialize VIC-II for bitmap mode
    jsr init_screen

    ; Initialize rotation state
    lda #0
    sta frame
    sta angle

main_loop:
    ; Calculate rotated square corners
    jsr calc_rotation

    ; Render the square
    jsr render_square

    ; Update angle for next frame
    inc angle
    inc frame

    ; Simple delay
    jsr wait_frame

    ; Continue indefinitely
    jmp main_loop

; Initialize screen for bitmap mode
init_screen:
    ; Set bitmap mode: BMM=1, DEN=1, RSEL=1, YSCROLL=3
    lda #$3B
    sta VIC_CTRL1

    ; Set multicolor mode off: MCM=0, CSEL=1, XSCROLL=0
    lda #$08
    sta VIC_CTRL2

    ; Set video matrix at $0400, bitmap at $2000
    ; MEMPTR = (screen/1024) << 4 | (bitmap/8192) << 3
    ; = (1 << 4) | (1 << 3) = $18
    lda #$18
    sta VIC_MEMPTR

    ; Set border and background
    lda #COLOR_BLACK
    sta VIC_BORDER
    sta VIC_BG

    ; Clear bitmap memory
    ldx #0
    lda #0
@clear_loop:
    sta BITMAP_RAM,x
    sta BITMAP_RAM+$0100,x
    sta BITMAP_RAM+$0200,x
    sta BITMAP_RAM+$0300,x
    sta BITMAP_RAM+$0400,x
    sta BITMAP_RAM+$0500,x
    sta BITMAP_RAM+$0600,x
    sta BITMAP_RAM+$0700,x
    sta BITMAP_RAM+$0800,x
    sta BITMAP_RAM+$0900,x
    sta BITMAP_RAM+$0A00,x
    sta BITMAP_RAM+$0B00,x
    sta BITMAP_RAM+$0C00,x
    sta BITMAP_RAM+$0D00,x
    sta BITMAP_RAM+$0E00,x
    sta BITMAP_RAM+$0F00,x
    sta BITMAP_RAM+$1000,x
    sta BITMAP_RAM+$1100,x
    sta BITMAP_RAM+$1200,x
    sta BITMAP_RAM+$1300,x
    sta BITMAP_RAM+$1400,x
    sta BITMAP_RAM+$1500,x
    sta BITMAP_RAM+$1600,x
    sta BITMAP_RAM+$1700,x
    sta BITMAP_RAM+$1800,x
    sta BITMAP_RAM+$1900,x
    sta BITMAP_RAM+$1A00,x
    sta BITMAP_RAM+$1B00,x
    sta BITMAP_RAM+$1C00,x
    sta BITMAP_RAM+$1D00,x
    sta BITMAP_RAM+$1E00,x
    sta BITMAP_RAM+$1F00,x
    inx
    bne @clear_loop

    ; Set screen colors (foreground=white, background=black for each 8x8 cell)
    ldx #0
    lda #$01              ; White on black
@color_loop:
    sta SCREEN_DATA,x
    sta SCREEN_DATA+$0100,x
    sta SCREEN_DATA+$0200,x
    sta SCREEN_DATA+$02E8,x
    inx
    cpx #232              ; 40*25 = 1000 bytes, but only need to clear visible area
    bne @color_loop

    rts

; Calculate rotated square corners
; TODO: Implement rotation calculation
calc_rotation:
    rts

; Render the rotated square
; TODO: Implement square rendering
render_square:
    rts

; Wait for one frame
wait_frame:
    ldx #0
@delay:
    dex
    bne @delay
    rts
