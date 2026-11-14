; External functions
.import precompute_frames: absolute
.import copy_frame_to_charset: absolute
.import sequencer_init: absolute
.import sequencer_tick: absolute
.import init_bit_reverse_table: absolute
.import clear_cia: absolute
.import clear_vic: absolute
.import init_vic: absolute
.import sprite_multiplex_init: absolute
.import irq_init: absolute
.import reveal_init: absolute
.import reveal_step: absolute
.import morph_step: absolute
.import split_step: absolute
.import job_init: absolute
.import job_step: absolute
.import spin_step: absolute
.import spin_step_mirrored: absolute
.import keep_squares: absolute
.import checker_step: absolute
.import diamond_step: absolute
.import drift_step: absolute
.import build_request: absolute

; Include shared zero page variables
.include "include/shared.inc"

; Include VIC-II register definitions
.include "include/vic.inc"

; C64 system addresses
CPU_PORT = $01             ; Memory configuration
NMI_VECTOR = $FFFA
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
    lda #$35                ; I/O visible, KERNAL and BASIC ROMs banked out
    sta CPU_PORT
    lda #<ignore_nmi
    sta NMI_VECTOR
    lda #>ignore_nmi
    sta NMI_VECTOR+1
    lda #0
    sta MUSIC_ON
    jsr prepare_screen

    ; Initialize bit reversal table (generates 256 bytes at runtime)
    jsr init_bit_reverse_table

    ; The cubes need their frames from the first step on; the scenes' images
    ; are built by the job in the idle time of the main loop
    jsr precompute_frames
    jsr job_init

    ; Initialize sequencer (state=0, frame=0, idle until the timeline starts)
    jsr sequencer_init

    jsr sprite_multiplex_init
    jsr irq_init
    cli

main_loop:
    lda #$02
    sta VIC_BORDER

    jsr dispatch_event

    lda SEQ_MODE
    bne @wait_next_frame    ; Freeze: FRAME unchanged, skip screen update

    jsr reveal_step
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
    lda #$0b
    sta VIC_BORDER

@wait_tick:
    lda FRAME_TICK
    bne @tick
    jsr job_step
    jmp @wait_tick
@tick:
    dec FRAME_TICK

    lda #$01
    sta VIC_BORDER

    ; Advance sequencer state (0→1→2...→39→0)
    jsr sequencer_tick

    jmp main_loop

ignore_nmi:
    rti

; Puts the screen, the charset, the sprites and the VIC into their initial state
prepare_screen:
    jsr clear_cia
    jsr clear_vic

    ; Select VIC bank 1 ($4000-$7FFF) via CIA2 port A
    ; Bits 1-0: %10 (2) = bank 1, bits 7-2: unchanged (for keyboard/joystick)
    lda CIA2_PORTA
    and #$FC               ; Clear bits 1-0
    ora #$02               ; Set bits 1-0 to %10 for bank 1
    sta CIA2_PORTA

    jsr fill_screens_and_charset
    jsr fill_sprites

    ; Initialize VIC-II chip
    jsr init_vic


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
    rts

; Fills the screens and the charset
fill_screens_and_charset:
    ldx #0
@fill:
    lda #63
    sta SCREEN,x
    sta SCREEN+$100,x
    sta SCREEN+$200,x
    sta SCREEN+$2E8,x
    sta SCREEN_C,x
    sta SCREEN_C+$100,x
    sta SCREEN_C+$200,x
    sta SCREEN_C+$2E8,x
    lda #$FF
    sta CHARSET,x
    sta CHARSET+$100,x
    sta CHARSET+$200,x
    sta CHARSET+$300,x
    sta CHARSET+$400,x
    sta CHARSET+$500,x
    sta CHARSET+$600,x
    sta CHARSET+$700,x
    inx
    bne @fill
    rts

; Fills the sprite data
fill_sprites:
    ldx #0
@fill:
    lda #$FF
    sta SPRITE,x        ; Sprite pointer A data ($2800)
    sta SPRITE+$80,x    ; Mirror sprite data ($2880)
    lda #0
    sta SPRITE+$40,x    ; Partner halves ($2840, $28C0) stay empty
    sta SPRITE+$C0,x
    inx
    bne @fill
    rts

start_music:
    lda MUSIC_ON
    bne @running
    jsr music_player_init
    lda #1
    sta MUSIC_ON
@running:
    rts

; Runs the handler of the event SEQ_EVENT: its address - 1 is pushed, so the
; handler's rts returns to the caller
dispatch_event:
    lda SEQ_EVENT
    asl
    tax
    lda event_handlers + 1,x
    pha
    lda event_handlers,x
    pha
ignore_event:
    rts

; Handler of each event, indexed by EVENT_*
event_handlers:
    .word ignore_event - 1
    .word reveal_init - 1
    .word start_music - 1
    .word morph_step - 1
    .word checker_step - 1
    .word split_step - 1
    .word spin_step - 1
    .word spin_step_mirrored - 1
    .word keep_squares - 1
    .word checker_step - 1
    .word diamond_step - 1
    .word drift_step - 1
    .word build_request - 1
