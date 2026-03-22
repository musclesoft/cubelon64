; External functions
.import calc_rotation: absolute
.import render_square: absolute
.import precompute_frames: absolute
.import copy_frame_to_charset: absolute
.import PRECOMPUTED_FRAMES: absolute
.import sequencer_init: absolute
.import sequencer_tick: absolute
.import sequencer_get_state: absolute
.import init_bit_reverse_table: absolute
.import init_vic: absolute
.import sprite_multiplex: absolute
.import sprite_multiplex_reset: absolute

; Include shared zero page variables
.include "include/shared.inc"

; Include VIC-II register definitions
.include "include/vic.inc"

; C64 system addresses
SCREEN = $4400              ; VIC bank 1 + $0400
COLOR = $D800               ; Color RAM (not in VIC bank)
CHARSET = $6000             ; VIC bank 1 + $2000
SPRITE = $6800              ; VIC bank 1 + $2800
SPRITE_PTRS = SCREEN + $3F8 ; Sprite pointers at end of screen
CIA2_PORTA = $DD00          ; CIA2 port A (VIC bank selection)

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
.export FRAME
main:
    sei
    jsr clear_vic

    ; Select VIC bank 1 ($4000-$7FFF) via CIA2 port A
    ; Bits 1-0: %10 (2) = bank 1, bits 7-2: unchanged (for keyboard/joystick)
    lda CIA2_PORTA
    and #$FC               ; Clear bits 1-0
    ora #$02               ; Set bits 1-0 to %10 for bank 1
    sta CIA2_PORTA

    ; Fill screen, charset, and sprite data
    ldx #0
@fill:
    lda #63
    sta SCREEN,x
    sta SCREEN+$100,x
    sta SCREEN+$200,x
    sta SCREEN+$2E8,x
    lda #$FF
    sta CHARSET,x
    sta CHARSET+$100,x
    sta CHARSET+$200,x
    sta CHARSET+$300,x
    sta CHARSET+$400,x
    sta CHARSET+$500,x
    sta CHARSET+$600,x
    sta CHARSET+$700,x
    sta SPRITE,x
    inx
    bne @fill

    ; Initialize VIC-II chip
    jsr init_vic


    ; Set up 8×4 grid of rotating squares (32 total)
    ; Each square is 3×6 chars, uses chars 0-21 (column-major layout)
    ; Grid layout: 8 squares across × 4 down = 24×24 chars at columns 8-31, rows 0-23
    ; Strategy: Fill first 6 rows with loops, then copy to remaining rows

    ; Fill rows 0-5 (first square row: 8 squares × 3 chars wide × 6 chars tall)
    ldx #0                  ; Row counter (0-5)
@row_loop:
    ; Write column 0 of each square (chars 0+row)
    txa                     ; A = row (0, 1, 2, 3, 4, 5)
    ldy #8                  ; Start at column 8
@col0_loop:
    sta SCREEN,y
    iny
    iny
    iny                     ; Skip 3 columns to next square
    cpy #32                 ; Done all 8 squares?
    bne @col0_loop

    ; Write column 1 of each square (chars 8+row)
    txa
    clc
    adc #8                  ; A = 8 + row
    ldy #9                  ; Start at column 9
@col1_loop:
    sta SCREEN,y
    iny
    iny
    iny                     ; Skip 3 columns to next square
    cpy #33
    bne @col1_loop

    ; Write column 2 of each square (chars 16+row)
    txa
    clc
    adc #16                 ; A = 16 + row
    ldy #10                 ; Start at column 10
@col2_loop:
    sta SCREEN,y
    iny
    iny
    iny                     ; Skip 3 columns to next square
    cpy #34
    bne @col2_loop

    ; Move SCREEN pointer to next row (add 40 to all store addresses)
    lda @col0_loop+1
    clc
    adc #40
    sta @col0_loop+1
    sta @col1_loop+1
    sta @col2_loop+1
    lda @col0_loop+2
    adc #0
    sta @col0_loop+2
    sta @col1_loop+2
    sta @col2_loop+2

    inx
    cpx #6                  ; 6 rows per square
    bne @row_loop

    ; Copy rows 0-5 to rows 6-23 using overlapping copy (240 bytes × 3 times)
    ; Each copy uses the previously-written data as source
    ldx #0                 ; X = 0
    ldy #0                 ; Y = byte offset within 240-byte block
@copy_outer:
    lda SCREEN,y
    sta SCREEN + 6*40,y
    iny
    cpy #240               ; 6 rows × 40 bytes = 240
    bne @copy_outer

    inx
    cpx #3                 ; 3 copies total (rows 6-11, 12-17, 18-23)
    beq @copy_done

    ; Shift addresses forward by 240 bytes for next iteration
    lda @copy_outer+1
    clc
    adc #240
    sta @copy_outer+1
    lda @copy_outer+2
    adc #0
    sta @copy_outer+2
    lda @copy_outer+4
    clc
    adc #240
    sta @copy_outer+4
    lda @copy_outer+5
    adc #0
    sta @copy_outer+5

    ldy #0
    jmp @copy_outer
@copy_done:

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
    ; Row 0, columns 8, 11, 14, 17, 20, 23, 26, 29
    ; Y coordinate: 50 + 0*8 + 4 = 54 (adjusted for YSCROLL=7)
    lda #54
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
    sta VIC_SPR_HI_X

    ; Enable sprites 0-7 (now that positions are set)
    lda #$FF
    sta VIC_SPR_ENABLE

    ; Initialize bit reversal table (generates 256 bytes at runtime)
    jsr init_bit_reverse_table

    ; Precompute frames
    jsr precompute_frames

    ; Initialize sequencer (state=0, frame=0, mode=motion)
    jsr sequencer_init

main_loop:
    ; Derive FRAME from sequencer state for 2-phase animation loop (82 frames)
    ; SEQ_STATE 0-20: motion phase (21 states)
    ; SEQ_STATE 21-39: freeze phase (19 states)
    ; FRAME = (SEQ_PHASE & 1) * 20 + SEQ_STATE (when motion)
    ;   Even phases (0,2,4...): FRAME 0-20 (21 frames)
    ;   Odd phases (1,3,5...): FRAME 20-40 (21 frames, uses all 41 frames)
    ;   Animation loops every 2 phases (82 frames total)

    lda SEQ_STATE
    cmp #21
    bcs @freeze_phase      ; If SEQ_STATE >= 21, skip to freeze

@motion_phase:
    ; Calculate FRAME = (SEQ_PHASE & 1) * 20 + SEQ_STATE
    lda SEQ_PHASE
    and #$01               ; Keep bit 0 (even=0, odd=1)
    beq @even_phase

@odd_phase:
    ; FRAME = 20 + SEQ_STATE (gives 20-40)
    lda SEQ_STATE
    clc
    adc #20
    sta FRAME
    jmp @update_screen

@even_phase:
    ; FRAME = SEQ_STATE (gives 0-20)
    lda SEQ_STATE
    sta FRAME
    jmp @update_screen

@freeze_phase:
    ; FRAME unchanged, skip screen update
    jmp @wait_next_frame

@update_screen:
    ; Copy and transform frame using precomputed data (only during motion)
    jsr copy_frame_to_charset
    ; Assertions for charset and sprite data (checked once per frame during playback)
    assert_if FRAME, 0, CHARSET, "../proto/golden/frame0_charset.bin", 192
    assert_if FRAME, 0, SPRITE, "../proto/golden/frame0_sprite.bin", 63
    assert_if FRAME, 1, CHARSET, "../proto/golden/frame1_charset.bin", 192
    assert_if FRAME, 1, SPRITE, "../proto/golden/frame1_sprite.bin", 63
    assert_if FRAME, 2, CHARSET, "../proto/golden/frame2_charset.bin", 192
    assert_if FRAME, 2, SPRITE, "../proto/golden/frame2_sprite.bin", 63
    assert_if FRAME, 3, CHARSET, "../proto/golden/frame3_charset.bin", 192
    assert_if FRAME, 3, SPRITE, "../proto/golden/frame3_sprite.bin", 63
    assert_if FRAME, 4, CHARSET, "../proto/golden/frame4_charset.bin", 192
    assert_if FRAME, 4, SPRITE, "../proto/golden/frame4_sprite.bin", 63
    assert_if FRAME, 5, CHARSET, "../proto/golden/frame5_charset.bin", 192
    assert_if FRAME, 5, SPRITE, "../proto/golden/frame5_sprite.bin", 63
    assert_if FRAME, 6, CHARSET, "../proto/golden/frame6_charset.bin", 192
    assert_if FRAME, 6, SPRITE, "../proto/golden/frame6_sprite.bin", 63
    assert_if FRAME, 7, CHARSET, "../proto/golden/frame7_charset.bin", 192
    assert_if FRAME, 7, SPRITE, "../proto/golden/frame7_sprite.bin", 63
    assert_if FRAME, 8, CHARSET, "../proto/golden/frame8_charset.bin", 192
    assert_if FRAME, 8, SPRITE, "../proto/golden/frame8_sprite.bin", 63
    assert_if FRAME, 9, CHARSET, "../proto/golden/frame9_charset.bin", 192
    assert_if FRAME, 9, SPRITE, "../proto/golden/frame9_sprite.bin", 63
    assert_if FRAME, 10, CHARSET, "../proto/golden/frame10_charset.bin", 192
    assert_if FRAME, 10, SPRITE, "../proto/golden/frame10_sprite.bin", 63
    assert_if FRAME, 11, CHARSET, "../proto/golden/frame11_charset.bin", 192
    assert_if FRAME, 11, SPRITE, "../proto/golden/frame11_sprite.bin", 63
    assert_if FRAME, 12, CHARSET, "../proto/golden/frame12_charset.bin", 192
    assert_if FRAME, 12, SPRITE, "../proto/golden/frame12_sprite.bin", 63
    assert_if FRAME, 13, CHARSET, "../proto/golden/frame13_charset.bin", 192
    assert_if FRAME, 13, SPRITE, "../proto/golden/frame13_sprite.bin", 63
    assert_if FRAME, 14, CHARSET, "../proto/golden/frame14_charset.bin", 192
    assert_if FRAME, 14, SPRITE, "../proto/golden/frame14_sprite.bin", 63
    assert_if FRAME, 15, CHARSET, "../proto/golden/frame15_charset.bin", 192
    assert_if FRAME, 15, SPRITE, "../proto/golden/frame15_sprite.bin", 63
    assert_if FRAME, 16, CHARSET, "../proto/golden/frame16_charset.bin", 192
    assert_if FRAME, 16, SPRITE, "../proto/golden/frame16_sprite.bin", 63
    assert_if FRAME, 17, CHARSET, "../proto/golden/frame17_charset.bin", 192
    assert_if FRAME, 17, SPRITE, "../proto/golden/frame17_sprite.bin", 63
    assert_if FRAME, 18, CHARSET, "../proto/golden/frame18_charset.bin", 192
    assert_if FRAME, 18, SPRITE, "../proto/golden/frame18_sprite.bin", 63
    assert_if FRAME, 19, CHARSET, "../proto/golden/frame19_charset.bin", 192
    assert_if FRAME, 19, SPRITE, "../proto/golden/frame19_sprite.bin", 63
    assert_if FRAME, 20, CHARSET, "../proto/golden/frame20_charset.bin", 192
    assert_if FRAME, 20, SPRITE, "../proto/golden/frame20_sprite.bin", 63
    assert_if FRAME, 21, CHARSET, "../proto/golden/frame21_charset.bin", 192
    assert_if FRAME, 21, SPRITE, "../proto/golden/frame21_sprite.bin", 63
    assert_if FRAME, 22, CHARSET, "../proto/golden/frame22_charset.bin", 192
    assert_if FRAME, 22, SPRITE, "../proto/golden/frame22_sprite.bin", 63
    assert_if FRAME, 23, CHARSET, "../proto/golden/frame23_charset.bin", 192
    assert_if FRAME, 23, SPRITE, "../proto/golden/frame23_sprite.bin", 63
    assert_if FRAME, 24, CHARSET, "../proto/golden/frame24_charset.bin", 192
    assert_if FRAME, 24, SPRITE, "../proto/golden/frame24_sprite.bin", 63
    assert_if FRAME, 25, CHARSET, "../proto/golden/frame25_charset.bin", 192
    assert_if FRAME, 25, SPRITE, "../proto/golden/frame25_sprite.bin", 63
    assert_if FRAME, 26, CHARSET, "../proto/golden/frame26_charset.bin", 192
    assert_if FRAME, 26, SPRITE, "../proto/golden/frame26_sprite.bin", 63
    assert_if FRAME, 27, CHARSET, "../proto/golden/frame27_charset.bin", 192
    assert_if FRAME, 27, SPRITE, "../proto/golden/frame27_sprite.bin", 63
    assert_if FRAME, 28, CHARSET, "../proto/golden/frame28_charset.bin", 192
    assert_if FRAME, 28, SPRITE, "../proto/golden/frame28_sprite.bin", 63
    assert_if FRAME, 29, CHARSET, "../proto/golden/frame29_charset.bin", 192
    assert_if FRAME, 29, SPRITE, "../proto/golden/frame29_sprite.bin", 63
    assert_if FRAME, 30, CHARSET, "../proto/golden/frame30_charset.bin", 192
    assert_if FRAME, 30, SPRITE, "../proto/golden/frame30_sprite.bin", 63
    assert_if FRAME, 31, CHARSET, "../proto/golden/frame31_charset.bin", 192
    assert_if FRAME, 31, SPRITE, "../proto/golden/frame31_sprite.bin", 63
    assert_if FRAME, 32, CHARSET, "../proto/golden/frame32_charset.bin", 192
    assert_if FRAME, 32, SPRITE, "../proto/golden/frame32_sprite.bin", 63
    assert_if FRAME, 33, CHARSET, "../proto/golden/frame33_charset.bin", 192
    assert_if FRAME, 33, SPRITE, "../proto/golden/frame33_sprite.bin", 63
    assert_if FRAME, 34, CHARSET, "../proto/golden/frame34_charset.bin", 192
    assert_if FRAME, 34, SPRITE, "../proto/golden/frame34_sprite.bin", 63
    assert_if FRAME, 35, CHARSET, "../proto/golden/frame35_charset.bin", 192
    assert_if FRAME, 35, SPRITE, "../proto/golden/frame35_sprite.bin", 63
    assert_if FRAME, 36, CHARSET, "../proto/golden/frame36_charset.bin", 192
    assert_if FRAME, 36, SPRITE, "../proto/golden/frame36_sprite.bin", 63
    assert_if FRAME, 37, CHARSET, "../proto/golden/frame37_charset.bin", 192
    assert_if FRAME, 37, SPRITE, "../proto/golden/frame37_sprite.bin", 63
    assert_if FRAME, 38, CHARSET, "../proto/golden/frame38_charset.bin", 192
    assert_if FRAME, 38, SPRITE, "../proto/golden/frame38_sprite.bin", 63
    assert_if FRAME, 39, CHARSET, "../proto/golden/frame39_charset.bin", 192
    assert_if FRAME, 39, SPRITE, "../proto/golden/frame39_sprite.bin", 63
    assert_if FRAME, 40, CHARSET, "../proto/golden/frame40_charset.bin", 192
    assert_if FRAME, 40, SPRITE, "../proto/golden/frame40_sprite.bin", 63

@wait_next_frame:
    lda #$09
    sta VIC_BORDER

    jsr sprite_multiplex

    ; Wait for frame
    jsr wait_frame

    ; Reset sprites to row 0 for next frame
    jsr sprite_multiplex_reset

    ; Advance sequencer state (0→1→2...→39→0)
    jsr sequencer_tick

    lda #$08
    sta VIC_BORDER

    jmp main_loop

; Wait for one frame - synchronize to VIC-II raster beam
; Waits for raster line 250+ (bottom border), then 0-50 (top of screen)
; This ensures one full frame has passed with no tearing
wait_frame:
    ; Wait for raster >= 250 (bottom border area)
@wait_bottom:
    lda #245
    cmp VIC_RASTER          ; VIC-II raster line register
    bne @wait_bottom        ; Loop if raster < 250
@wait_top:
    cmp VIC_RASTER
    beq @wait_top           ; Loop if bit 7 set ($D012 >= 128)

    rts
