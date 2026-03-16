; External functions
.import calc_rotation: absolute
.import render_square: absolute
.import init_charset: absolute
.import precompute_frames: absolute
.import copy_frame_to_charset: absolute

; Include shared zero page variables
.include "include/shared.inc"

; C64 system addresses
SCREEN_RAM = $4400          ; VIC bank 1 + $0400
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
VIC_SPR_PRIORITY = VIC_BASE + $1B
SPRITE_PTRS = SCREEN_RAM + $3F8
CIA2_PORTA = $DD00

; Screen addresses
SCREEN_DATA = $4400         ; VIC bank 1 + $0400

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
    ; Select VIC bank 1 ($4000-$7FFF) via CIA2 port A
    ; Bits 1-0: %10 (2) = bank 1, bits 7-2: unchanged (for keyboard/joystick)
    lda CIA2_PORTA
    and #$FC               ; Clear bits 1-0
    ora #$02               ; Set bits 1-0 to %10 for bank 1
    sta CIA2_PORTA

    ; Set VIC for character mode
    lda #$1B
    sta VIC_CTRL1

    lda #$08
    sta VIC_CTRL2

    ; Set colors
    lda #COLOR_LIGHT_GREY  ; Light grey background ($F)
    sta VIC_BG
    lda #COLOR_BLACK       ; Black border
    sta VIC_BORDER

    ; Clear charset at $6000 (VIC bank 1 + $2000)
    ldx #0
    lda #$00
@fillcharset:
    sta $6000,x
    sta $6100,x
    sta $6200,x
    sta $6300,x
    sta $6400,x
    sta $6500,x
    sta $6600,x
    sta $6700,x
    inx
    bne @fillcharset

    ; Configure VIC to use charset at bank offset $2000
    lda $D018
    and #$F1
    ora #$08               ; Bits 3-1 = 100 binary = charset at offset $2000 in bank
    sta $D018

    ; Fill char 63 with $FF (fully opaque black background)
    ldx #0
    lda #$FF
@fill_char63:
    sta $61F8,x            ; Char 63 at $6000 + 63*8 = $61F8
    inx
    cpx #8
    bne @fill_char63

    ; Clear screen with char 63 (opaque background)
    ldx #0
    lda #63
@fillscreen:
    sta SCREEN_DATA,x
    sta SCREEN_DATA+$100,x
    sta SCREEN_DATA+$200,x
    sta SCREEN_DATA+$2E8,x
    inx
    bne @fillscreen

    ; Set up 8×(3x3) grids for rotating squares across top center
    ; 8 squares × 3 chars wide = 24 chars, centered: columns 8-31, rows 1-3
    ; All squares use same chars 0-23 (column-major layout)
    ; Row 1
    lda #0
    sta SCREEN_RAM + 1*40 + 8
    sta SCREEN_RAM + 1*40 + 11
    sta SCREEN_RAM + 1*40 + 14
    sta SCREEN_RAM + 1*40 + 17
    sta SCREEN_RAM + 1*40 + 20
    sta SCREEN_RAM + 1*40 + 23
    sta SCREEN_RAM + 1*40 + 26
    sta SCREEN_RAM + 1*40 + 29
    lda #8
    sta SCREEN_RAM + 1*40 + 9
    sta SCREEN_RAM + 1*40 + 12
    sta SCREEN_RAM + 1*40 + 15
    sta SCREEN_RAM + 1*40 + 18
    sta SCREEN_RAM + 1*40 + 21
    sta SCREEN_RAM + 1*40 + 24
    sta SCREEN_RAM + 1*40 + 27
    sta SCREEN_RAM + 1*40 + 30
    lda #16
    sta SCREEN_RAM + 1*40 + 10
    sta SCREEN_RAM + 1*40 + 13
    sta SCREEN_RAM + 1*40 + 16
    sta SCREEN_RAM + 1*40 + 19
    sta SCREEN_RAM + 1*40 + 22
    sta SCREEN_RAM + 1*40 + 25
    sta SCREEN_RAM + 1*40 + 28
    sta SCREEN_RAM + 1*40 + 31
    ; Row 2
    lda #1
    sta SCREEN_RAM + 2*40 + 8
    sta SCREEN_RAM + 2*40 + 11
    sta SCREEN_RAM + 2*40 + 14
    sta SCREEN_RAM + 2*40 + 17
    sta SCREEN_RAM + 2*40 + 20
    sta SCREEN_RAM + 2*40 + 23
    sta SCREEN_RAM + 2*40 + 26
    sta SCREEN_RAM + 2*40 + 29
    lda #9
    sta SCREEN_RAM + 2*40 + 9
    sta SCREEN_RAM + 2*40 + 12
    sta SCREEN_RAM + 2*40 + 15
    sta SCREEN_RAM + 2*40 + 18
    sta SCREEN_RAM + 2*40 + 21
    sta SCREEN_RAM + 2*40 + 24
    sta SCREEN_RAM + 2*40 + 27
    sta SCREEN_RAM + 2*40 + 30
    lda #17
    sta SCREEN_RAM + 2*40 + 10
    sta SCREEN_RAM + 2*40 + 13
    sta SCREEN_RAM + 2*40 + 16
    sta SCREEN_RAM + 2*40 + 19
    sta SCREEN_RAM + 2*40 + 22
    sta SCREEN_RAM + 2*40 + 25
    sta SCREEN_RAM + 2*40 + 28
    sta SCREEN_RAM + 2*40 + 31
    ; Row 3
    lda #2
    sta SCREEN_RAM + 3*40 + 8
    sta SCREEN_RAM + 3*40 + 11
    sta SCREEN_RAM + 3*40 + 14
    sta SCREEN_RAM + 3*40 + 17
    sta SCREEN_RAM + 3*40 + 20
    sta SCREEN_RAM + 3*40 + 23
    sta SCREEN_RAM + 3*40 + 26
    sta SCREEN_RAM + 3*40 + 29
    lda #10
    sta SCREEN_RAM + 3*40 + 9
    sta SCREEN_RAM + 3*40 + 12
    sta SCREEN_RAM + 3*40 + 15
    sta SCREEN_RAM + 3*40 + 18
    sta SCREEN_RAM + 3*40 + 21
    sta SCREEN_RAM + 3*40 + 24
    sta SCREEN_RAM + 3*40 + 27
    sta SCREEN_RAM + 3*40 + 30
    lda #18
    sta SCREEN_RAM + 3*40 + 10
    sta SCREEN_RAM + 3*40 + 13
    sta SCREEN_RAM + 3*40 + 16
    sta SCREEN_RAM + 3*40 + 19
    sta SCREEN_RAM + 3*40 + 22
    sta SCREEN_RAM + 3*40 + 25
    sta SCREEN_RAM + 3*40 + 28
    sta SCREEN_RAM + 3*40 + 31

    ; Set all colors to black
    ldx #0
    lda #COLOR_BLACK
@color:
    sta COLOR_RAM,x
    sta COLOR_RAM+$100,x
    sta COLOR_RAM+$200,x
    sta COLOR_RAM+$2E8,x
    inx
    bne @color

    ; Clear sprite data at $6800 (VIC bank 1 + $2800)
    ldx #0
    lda #$00
@clear_sprite:
    sta $6800,x
    inx
    cpx #64
    bne @clear_sprite

    ; Set up sprite pointers 0-7 (all use same sprite data at $2800)
    ; Sprite pointer: offset $2800 / 64 = $A0 (160)
    lda #$A0
    sta SPRITE_PTRS+0
    sta SPRITE_PTRS+1
    sta SPRITE_PTRS+2
    sta SPRITE_PTRS+3
    sta SPRITE_PTRS+4
    sta SPRITE_PTRS+5
    sta SPRITE_PTRS+6
    sta SPRITE_PTRS+7

    ; Position sprites 0-7 at character positions across top center
    ; Row 1, columns 8, 11, 14, 17, 20, 23, 26, 29
    ; Y coordinate: 50 + 1*8 = 58 (all sprites same Y)
    lda #58
    sta VIC_SPR0_Y
    sta VIC_SPR0_Y+2    ; Sprite 1
    sta VIC_SPR0_Y+4    ; Sprite 2
    sta VIC_SPR0_Y+6    ; Sprite 3
    sta VIC_SPR0_Y+8    ; Sprite 4
    sta VIC_SPR0_Y+10   ; Sprite 5
    sta VIC_SPR0_Y+12   ; Sprite 6
    sta VIC_SPR0_Y+14   ; Sprite 7

    ; X coordinates: 88, 112, 136, 160, 184, 208, 232, 256
    lda #88
    sta VIC_SPR0_X      ; Sprite 0: col 8
    lda #112
    sta VIC_SPR0_X+2    ; Sprite 1: col 11
    lda #136
    sta VIC_SPR0_X+4    ; Sprite 2: col 14
    lda #160
    sta VIC_SPR0_X+6    ; Sprite 3: col 17
    lda #184
    sta VIC_SPR0_X+8    ; Sprite 4: col 20
    lda #208
    sta VIC_SPR0_X+10   ; Sprite 5: col 23
    lda #232
    sta VIC_SPR0_X+12   ; Sprite 6: col 26
    lda #0              ; Sprite 7: col 29 = 256 (need MSB set)
    sta VIC_SPR0_X+14
    ; Set X MSB for sprite 7 (bit 7)
    lda #$80
    sta VIC_BASE+$10    ; VIC_SPR_HI_X

    ; Set all sprite colors to dark grey ($B)
    lda #COLOR_DARK_GREY
    sta VIC_SPR0_COLOR
    sta VIC_SPR0_COLOR+1
    sta VIC_SPR0_COLOR+2
    sta VIC_SPR0_COLOR+3
    sta VIC_SPR0_COLOR+4
    sta VIC_SPR0_COLOR+5
    sta VIC_SPR0_COLOR+6
    sta VIC_SPR0_COLOR+7

    ; Disable multicolor for all sprites (use hires mode)
    lda #$00
    sta VIC_SPR_MULTICOLOR

    ; Set all sprites behind foreground (priority bits 0-7)
    lda #$FF
    sta VIC_SPR_PRIORITY

    ; Enable sprites 0-7
    lda #$FF                ; Bits 0-7 = sprites 0-7
    sta VIC_SPR_ENABLE

    ; Precompute frames 0-10 at -4px offset for animation optimization
    jsr precompute_frames

    ; Initialize animation frame (41 discrete frames: 0-40)
    lda #0
    sta FRAME

main_loop:
    ; Calculate rotated square corners (for frames 0-20, mirrors for 21-40)
    ; Map frame 0-40 to angle 0-20
    lda FRAME
    cmp #21
    bcc @use_frame          ; If frame < 21, use it directly
    ; For frames 21-40, mirror: angle = 40 - frame
    lda #40
    sec
    sbc FRAME
@use_frame:
    sta @angle_frame
    ; Temporarily set FRAME to angle for calc_rotation
    lda FRAME
    pha                     ; Save actual frame
    lda @angle_frame
    sta FRAME
    jsr calc_rotation
    pla
    sta FRAME               ; Restore actual frame

    ; Copy/transform precomputed frame to charset (4-phase optimization)
    jsr copy_frame_to_charset

    ; Increment frame counter (0→1→2...→40→0)
    lda FRAME
    clc
    adc #1
    cmp #41                ; Check if we've reached frame 41
    bcc @frame_ok          ; If < 41, keep the value
    lda #0                 ; Wrap back to frame 0
@frame_ok:
    sta FRAME

    ; Wait for frame
    jsr wait_frame
    jmp main_loop

@angle_frame: .res 1

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
