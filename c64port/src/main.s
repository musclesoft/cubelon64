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

; Include shared zero page variables
.include "include/shared.inc"

; Include VIC-II register definitions
.include "include/vic.inc"

; C64 system addresses
SCREEN_RAM = $4400          ; VIC bank 1 + $0400
SCREEN_DATA = $4400         ; VIC bank 1 + $0400 (same as SCREEN_RAM)
COLOR_RAM = $D800           ; Color RAM (not in VIC bank)
CHARSET_RAM = $6000         ; VIC bank 1 + $2000
SPRITE_PTRS = SCREEN_RAM + $3F8 ; Sprite pointers at end of screen RAM
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
    ; Select VIC bank 1 ($4000-$7FFF) via CIA2 port A
    ; Bits 1-0: %10 (2) = bank 1, bits 7-2: unchanged (for keyboard/joystick)
    lda CIA2_PORTA
    and #$FC               ; Clear bits 1-0
    ora #$02               ; Set bits 1-0 to %10 for bank 1
    sta CIA2_PORTA

    ; Initialize VIC-II chip
    jsr init_vic

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

    ; Initialize charset margins for animation chars (column-major layout)
    ; Chars 0, 8, 16 (columns 0-2): set row 0 and rows 18-23 to $FF (background)
    lda #$FF
    sta CHARSET_RAM         ; Column 0, row 0
    sta CHARSET_RAM+64      ; Column 1, row 0
    sta CHARSET_RAM+128     ; Column 2, row 0
    ldx #18
@init_margins:
    sta CHARSET_RAM,x       ; Column 0, rows 18-23
    sta CHARSET_RAM+64,x    ; Column 1, rows 18-23
    sta CHARSET_RAM+128,x   ; Column 2, rows 18-23
    inx
    cpx #24
    bne @init_margins

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

    ; Set up 8×8 grid of rotating squares (64 total)
    ; Each square is 3×3 chars, all use same chars 0-23 (column-major layout)
    ; Grid layout: 8 squares across × 8 down = 24×24 chars at columns 8-31, rows 1-24
    ; Strategy: Fill first 3 rows, then copy to remaining rows (pattern repeats every 3 rows)

    ; Fill rows 1-3 (first square row: 8 squares × 3 chars wide each)
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

    ; Copy rows 1-3 to rows 4-24 using overlapping copy (120 bytes × 7 times)
    ; Each copy uses the previously-written data as source
    ldx #0                 ; X = 0
    ldy #0                 ; Y = byte offset within 120-byte block
@copy_outer:
    lda SCREEN_RAM + 1*40,y
    sta SCREEN_RAM + 4*40,y
    iny
    cpy #120               ; 3 rows × 40 bytes = 120
    bne @copy_outer

    inx
    cpx #7                 ; 7 copies total (rows 4-6, 7-9, ..., 22-24)
    beq @copy_done

    ; Shift addresses forward by 120 bytes for next iteration
    lda @copy_outer+1
    clc
    adc #120
    sta @copy_outer+1
    lda @copy_outer+2
    adc #0
    sta @copy_outer+2
    lda @copy_outer+4
    clc
    adc #120
    sta @copy_outer+4
    lda @copy_outer+5
    adc #0
    sta @copy_outer+5

    ldy #0
    jmp @copy_outer
@copy_done:

    ; Clear sprite data at $6800 (VIC bank 1 + $2800)
    ; Need 63 bytes (21 rows × 3 bytes) in row-major format
    ldx #0
    lda #$00
@clear_sprite:
    sta $6800,x
    inx
    cpx #63
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
    sta VIC_SPR_HI_X

    ; Set all sprite colors to red (2) for visual debugging
    lda #COLOR_RED
    sta VIC_SPR0_COLOR
    sta VIC_SPR0_COLOR+1
    sta VIC_SPR0_COLOR+2
    sta VIC_SPR0_COLOR+3
    sta VIC_SPR0_COLOR+4
    sta VIC_SPR0_COLOR+5
    sta VIC_SPR0_COLOR+6
    sta VIC_SPR0_COLOR+7

    ; Enable sprites 0-7 (now that positions and colors are set)
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
    assert_if FRAME, 0, CHARSET_RAM, "../proto/golden/frame0_charset.bin", 192
    assert_if FRAME, 0, SPRITE_DATA, "../proto/golden/frame0_sprite.bin", 63
    assert_if FRAME, 1, CHARSET_RAM, "../proto/golden/frame1_charset.bin", 192
    assert_if FRAME, 1, SPRITE_DATA, "../proto/golden/frame1_sprite.bin", 63
    assert_if FRAME, 2, CHARSET_RAM, "../proto/golden/frame2_charset.bin", 192
    assert_if FRAME, 2, SPRITE_DATA, "../proto/golden/frame2_sprite.bin", 63
    assert_if FRAME, 3, CHARSET_RAM, "../proto/golden/frame3_charset.bin", 192
    assert_if FRAME, 3, SPRITE_DATA, "../proto/golden/frame3_sprite.bin", 63
    assert_if FRAME, 4, CHARSET_RAM, "../proto/golden/frame4_charset.bin", 192
    assert_if FRAME, 4, SPRITE_DATA, "../proto/golden/frame4_sprite.bin", 63
    assert_if FRAME, 5, CHARSET_RAM, "../proto/golden/frame5_charset.bin", 192
    assert_if FRAME, 5, SPRITE_DATA, "../proto/golden/frame5_sprite.bin", 63
    assert_if FRAME, 6, CHARSET_RAM, "../proto/golden/frame6_charset.bin", 192
    assert_if FRAME, 6, SPRITE_DATA, "../proto/golden/frame6_sprite.bin", 63
    assert_if FRAME, 7, CHARSET_RAM, "../proto/golden/frame7_charset.bin", 192
    assert_if FRAME, 7, SPRITE_DATA, "../proto/golden/frame7_sprite.bin", 63
    assert_if FRAME, 8, CHARSET_RAM, "../proto/golden/frame8_charset.bin", 192
    assert_if FRAME, 8, SPRITE_DATA, "../proto/golden/frame8_sprite.bin", 63
    assert_if FRAME, 9, CHARSET_RAM, "../proto/golden/frame9_charset.bin", 192
    assert_if FRAME, 9, SPRITE_DATA, "../proto/golden/frame9_sprite.bin", 63
    assert_if FRAME, 10, CHARSET_RAM, "../proto/golden/frame10_charset.bin", 192
    assert_if FRAME, 10, SPRITE_DATA, "../proto/golden/frame10_sprite.bin", 63
    assert_if FRAME, 11, CHARSET_RAM, "../proto/golden/frame11_charset.bin", 192
    assert_if FRAME, 11, SPRITE_DATA, "../proto/golden/frame11_sprite.bin", 63
    assert_if FRAME, 12, CHARSET_RAM, "../proto/golden/frame12_charset.bin", 192
    assert_if FRAME, 12, SPRITE_DATA, "../proto/golden/frame12_sprite.bin", 63
    assert_if FRAME, 13, CHARSET_RAM, "../proto/golden/frame13_charset.bin", 192
    assert_if FRAME, 13, SPRITE_DATA, "../proto/golden/frame13_sprite.bin", 63
    assert_if FRAME, 14, CHARSET_RAM, "../proto/golden/frame14_charset.bin", 192
    assert_if FRAME, 14, SPRITE_DATA, "../proto/golden/frame14_sprite.bin", 63
    assert_if FRAME, 15, CHARSET_RAM, "../proto/golden/frame15_charset.bin", 192
    assert_if FRAME, 15, SPRITE_DATA, "../proto/golden/frame15_sprite.bin", 63
    assert_if FRAME, 16, CHARSET_RAM, "../proto/golden/frame16_charset.bin", 192
    assert_if FRAME, 16, SPRITE_DATA, "../proto/golden/frame16_sprite.bin", 63
    assert_if FRAME, 17, CHARSET_RAM, "../proto/golden/frame17_charset.bin", 192
    assert_if FRAME, 17, SPRITE_DATA, "../proto/golden/frame17_sprite.bin", 63
    assert_if FRAME, 18, CHARSET_RAM, "../proto/golden/frame18_charset.bin", 192
    assert_if FRAME, 18, SPRITE_DATA, "../proto/golden/frame18_sprite.bin", 63
    assert_if FRAME, 19, CHARSET_RAM, "../proto/golden/frame19_charset.bin", 192
    assert_if FRAME, 19, SPRITE_DATA, "../proto/golden/frame19_sprite.bin", 63
    assert_if FRAME, 20, CHARSET_RAM, "../proto/golden/frame20_charset.bin", 192
    assert_if FRAME, 20, SPRITE_DATA, "../proto/golden/frame20_sprite.bin", 63
    assert_if FRAME, 21, CHARSET_RAM, "../proto/golden/frame21_charset.bin", 192
    assert_if FRAME, 21, SPRITE_DATA, "../proto/golden/frame21_sprite.bin", 63
    assert_if FRAME, 22, CHARSET_RAM, "../proto/golden/frame22_charset.bin", 192
    assert_if FRAME, 22, SPRITE_DATA, "../proto/golden/frame22_sprite.bin", 63
    assert_if FRAME, 23, CHARSET_RAM, "../proto/golden/frame23_charset.bin", 192
    assert_if FRAME, 23, SPRITE_DATA, "../proto/golden/frame23_sprite.bin", 63
    assert_if FRAME, 24, CHARSET_RAM, "../proto/golden/frame24_charset.bin", 192
    assert_if FRAME, 24, SPRITE_DATA, "../proto/golden/frame24_sprite.bin", 63
    assert_if FRAME, 25, CHARSET_RAM, "../proto/golden/frame25_charset.bin", 192
    assert_if FRAME, 25, SPRITE_DATA, "../proto/golden/frame25_sprite.bin", 63
    assert_if FRAME, 26, CHARSET_RAM, "../proto/golden/frame26_charset.bin", 192
    assert_if FRAME, 26, SPRITE_DATA, "../proto/golden/frame26_sprite.bin", 63
    assert_if FRAME, 27, CHARSET_RAM, "../proto/golden/frame27_charset.bin", 192
    assert_if FRAME, 27, SPRITE_DATA, "../proto/golden/frame27_sprite.bin", 63
    assert_if FRAME, 28, CHARSET_RAM, "../proto/golden/frame28_charset.bin", 192
    assert_if FRAME, 28, SPRITE_DATA, "../proto/golden/frame28_sprite.bin", 63
    assert_if FRAME, 29, CHARSET_RAM, "../proto/golden/frame29_charset.bin", 192
    assert_if FRAME, 29, SPRITE_DATA, "../proto/golden/frame29_sprite.bin", 63
    assert_if FRAME, 30, CHARSET_RAM, "../proto/golden/frame30_charset.bin", 192
    assert_if FRAME, 30, SPRITE_DATA, "../proto/golden/frame30_sprite.bin", 63
    assert_if FRAME, 31, CHARSET_RAM, "../proto/golden/frame31_charset.bin", 192
    assert_if FRAME, 31, SPRITE_DATA, "../proto/golden/frame31_sprite.bin", 63
    assert_if FRAME, 32, CHARSET_RAM, "../proto/golden/frame32_charset.bin", 192
    assert_if FRAME, 32, SPRITE_DATA, "../proto/golden/frame32_sprite.bin", 63
    assert_if FRAME, 33, CHARSET_RAM, "../proto/golden/frame33_charset.bin", 192
    assert_if FRAME, 33, SPRITE_DATA, "../proto/golden/frame33_sprite.bin", 63
    assert_if FRAME, 34, CHARSET_RAM, "../proto/golden/frame34_charset.bin", 192
    assert_if FRAME, 34, SPRITE_DATA, "../proto/golden/frame34_sprite.bin", 63
    assert_if FRAME, 35, CHARSET_RAM, "../proto/golden/frame35_charset.bin", 192
    assert_if FRAME, 35, SPRITE_DATA, "../proto/golden/frame35_sprite.bin", 63
    assert_if FRAME, 36, CHARSET_RAM, "../proto/golden/frame36_charset.bin", 192
    assert_if FRAME, 36, SPRITE_DATA, "../proto/golden/frame36_sprite.bin", 63
    assert_if FRAME, 37, CHARSET_RAM, "../proto/golden/frame37_charset.bin", 192
    assert_if FRAME, 37, SPRITE_DATA, "../proto/golden/frame37_sprite.bin", 63
    assert_if FRAME, 38, CHARSET_RAM, "../proto/golden/frame38_charset.bin", 192
    assert_if FRAME, 38, SPRITE_DATA, "../proto/golden/frame38_sprite.bin", 63
    assert_if FRAME, 39, CHARSET_RAM, "../proto/golden/frame39_charset.bin", 192
    assert_if FRAME, 39, SPRITE_DATA, "../proto/golden/frame39_sprite.bin", 63
    assert_if FRAME, 40, CHARSET_RAM, "../proto/golden/frame40_charset.bin", 192
    assert_if FRAME, 40, SPRITE_DATA, "../proto/golden/frame40_sprite.bin", 63

@wait_next_frame:

    lda #$09
    sta $D020              ; visual debugging

    ; Wait for frame
    jsr wait_frame

    ; Advance sequencer state (0→1→2...→39→0)
    jsr sequencer_tick

    lda #$08
    sta $D020              ; visual debugging

    jmp main_loop

; Wait for one frame - synchronize to VIC-II raster beam
; Waits for raster line 250+ (bottom border), then 0-50 (top of screen)
; This ensures one full frame has passed with no tearing
wait_frame:
    ; Wait for raster >= 250 (bottom border area)
@wait_bottom:
    lda #249
    cmp $D012               ; VIC-II raster line register
    bne @wait_bottom        ; Loop if raster < 250
@wait_top:
    cmp $D012
    beq @wait_top           ; Loop if bit 7 set ($D012 >= 128)

    rts
