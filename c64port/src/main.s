; External functions
.import calc_rotation: absolute
.import render_square: absolute
.import init_charset: absolute

; Include shared zero page variables
.include "include/shared.inc"

; C64 system addresses
SCREEN_RAM = $0400
COLOR_RAM = $D800
VIC_BASE = $D000
VIC_CTRL1 = VIC_BASE + $11
VIC_CTRL2 = VIC_BASE + $16
VIC_MEMPTR = VIC_BASE + $18
VIC_BORDER = VIC_BASE + $20
VIC_BG = VIC_BASE + $21
VIC_SPR_ENABLE = VIC_BASE + $15
VIC_SPR0_X = VIC_BASE + $00
VIC_SPR0_Y = VIC_BASE + $01
VIC_SPR0_COLOR = VIC_BASE + $27
VIC_SPR_MULTICOLOR = VIC_BASE + $1C
SPRITE_PTRS = SCREEN_RAM + $3F8

; Screen addresses
SCREEN_DATA = $0400

; Colors
COLOR_BLACK = 0
COLOR_WHITE = 1
COLOR_DARK_GREY = 11
COLOR_MID_GREY = 12
COLOR_LIGHT_GREY = 15

; PRG load address (goes in file header, not loaded into memory)
.segment "LOADADDR"
.word $0801

.segment "CODE"

; BASIC stub: 10 SYS 2062
; Memory layout:
; $0801-$0802: $0B $08 (link to next line)
; $0803-$0804: $0A $00 (line number 10)
; $0805: $9E (SYS token)
; $0806: $20 (space)
; $0807-$080A: "2062" (4 ASCII digits)
; $080B: $00 (end of line)
; $080C-$080D: $00 $00 (end of program)
; Machine code starts at: $080E (2062 decimal)
    .byte $0B, $08          ; Link to next line
    .byte $0A, $00          ; Line number 10
    .byte $9E               ; SYS token
    .byte $20               ; Space
    .byte $32, $30, $36, $32  ; "2062" in ASCII
    .byte $00               ; End of BASIC line
    .byte $00, $00          ; End of BASIC program

; Entry point starts here (will be loaded at $080D)
.export main
main:
    ; Set VIC for character mode
    lda #$1B
    sta VIC_CTRL1

    lda #$08
    sta VIC_CTRL2

    ; Set colors for visibility
    lda #7                 ; Yellow background
    sta VIC_BG
    lda #5                 ; GREEN border to verify code is running
    sta VIC_BORDER

    ; Clear charset at $2000
    ldx #0
    lda #$00
@fillcharset:
    sta $2000,x
    sta $2100,x
    sta $2200,x
    sta $2300,x
    sta $2400,x
    sta $2500,x
    sta $2600,x
    sta $2700,x
    inx
    bne @fillcharset

    ; Configure VIC to use charset at $2000
    lda $D018
    and #$F1
    ora #$08               ; Bits 3-1 = 100 binary = charset at $2000
    sta $D018

    ; Clear screen with spaces
    ldx #0
    lda #$20
@fillscreen:
    sta SCREEN_DATA,x
    sta SCREEN_DATA+$100,x
    sta SCREEN_DATA+$200,x
    sta SCREEN_DATA+$2E8,x
    inx
    bne @fillscreen

    ; Set up 3x8 grid for rotating square at screen position (8,11)
    ; Column-major layout with power-of-2: offset = (column << 6) + y
    ; Column 0: chars 64-71 (only 64,65,66 visible)
    ; Column 1: chars 72-79 (only 72,73,74 visible)
    ; Column 2: chars 80-87 (only 80,81,82 visible)
    lda #64
    sta $0400 + 11*40 + 8
    lda #72
    sta $0400 + 11*40 + 9
    lda #80
    sta $0400 + 11*40 + 10
    lda #65
    sta $0400 + 12*40 + 8
    lda #73
    sta $0400 + 12*40 + 9
    lda #81
    sta $0400 + 12*40 + 10
    lda #66
    sta $0400 + 13*40 + 8
    lda #74
    sta $0400 + 13*40 + 9
    lda #82
    sta $0400 + 13*40 + 10

    ; Set all colors to white
    ldx #0
    lda #1
@color:
    sta COLOR_RAM,x
    sta COLOR_RAM+$100,x
    sta COLOR_RAM+$200,x
    sta COLOR_RAM+$2E8,x
    inx
    bne @color

    ; Explicitly set colors for the 3x3 grid to blue (6)
    lda #6
    sta $D800 + 11*40 + 8
    sta $D800 + 11*40 + 9
    sta $D800 + 11*40 + 10
    sta $D800 + 12*40 + 8
    sta $D800 + 12*40 + 9
    sta $D800 + 12*40 + 10
    sta $D800 + 13*40 + 8
    sta $D800 + 13*40 + 9
    sta $D800 + 13*40 + 10

    ; Clear sprite data
    ldx #0
    lda #$00
@clear_sprite:
    sta $2800,x
    inx
    cpx #64
    bne @clear_sprite

    ; Set up sprite 0 for antialiasing overlay
    ; Sprite pointer: $2800 / 64 = $A0 (160)
    lda #$A0
    sta SPRITE_PTRS

    ; Position sprite 0 at character position (8,11)
    ; PAL VIC-II: char col 0 starts at sprite X=24, char row 0 starts at raster Y=51
    ; Character (8,11) → X = 24 + 8*8 = 88, Y = 51 + 11*8 = 139
    lda #88
    sta VIC_SPR0_X
    lda #139
    sta VIC_SPR0_Y

    ; Set sprite 0 color to light gray (12 = medium brightness gray)
    lda #12
    sta VIC_SPR0_COLOR

    ; Disable multicolor for sprite 0 (use hires mode)
    lda VIC_SPR_MULTICOLOR
    and #$FE                ; Clear bit 0
    sta VIC_SPR_MULTICOLOR

    ; Enable sprite 0
    lda #$01                ; Bit 0 = sprite 0
    sta VIC_SPR_ENABLE

    ; Initialize animation frame (21 discrete frames: 0-20)
    lda #0
    sta FRAME

main_loop:
    ; Calculate rotated square corners
    jsr calc_rotation

    ; Render the rotated square to charset
    jsr render_square

    ; Increment frame counter (0→1→2...→20→0)
    lda FRAME
    clc
    adc #1
    cmp #21                ; Check if we've reached frame 21
    bcc @frame_ok          ; If < 21, keep the value
    lda #0                 ; Wrap back to frame 0
@frame_ok:
    sta FRAME

    ; Wait for frame
    jsr wait_frame
    jmp main_loop

; Initialize screen for character mode
init_screen:
    ; Set character mode: BMM=0, DEN=1, RSEL=1, YSCROLL=3
    lda #$1B
    sta VIC_CTRL1

    ; Set multicolor mode off: MCM=0, CSEL=1, XSCROLL=0
    lda #$08
    sta VIC_CTRL2

    ; Set border and background for visibility
    lda #COLOR_BLACK
    sta VIC_BORDER
    sta VIC_BG

    ; Clear screen with space characters
    ldx #0
    lda #$20              ; Space character
@clear_screen:
    sta SCREEN_DATA,x
    sta SCREEN_DATA+$0100,x
    sta SCREEN_DATA+$0200,x
    sta SCREEN_DATA+$02E8,x
    inx
    bne @clear_screen

    ; Set all screen colors to white (foreground)
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
