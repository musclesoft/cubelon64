; cube_mux.s - VICE test: 8 bands of standard 21 line sprites with a pointer row per band, the
; simple sprite multiplexer of the cube scene
;
; The cube only uses the tile rows 3-20, so a standard sprite that starts on the band's first line
; (rows 0-20) is enough and the band has a gap of 3 lines (22-24) between two sprites. One raster
; interrupt per band fires on the first line of the gap, moves the 8 sprites to the next band and
; points them at the sprite data of the rings: the kind of a cell is its ring, 3 - min(band, 7 -
; band, column, 7 - column). Nothing in the handler is cycle exact: it only has to finish before the
; sprites' data fetch for the next band's first line, in cycle 58 of the gap's last line.
;
; Every kind shows a pattern in which each row differs from its neighbours, so a pointer that changes
; too early or too late shows as a row of the wrong kind:
;   kind 0  all pixels            kind 1  every other row
;   kind 2  every other column    kind 3  checkerboard
;
; The CIA timers of the music's waveform are on as in sprite24.s, so the raster interrupt is delayed by
; them the way it is in the demo.
;
; Result (VICE x64sc, 12 frames with different phases between the CIA timers and the raster): every
; pixel of the 8x8 cells is as expected. With extra NOPs at the start of the raster path the writes
; still land in time up to 40 extra cycles and fail from 60: sprite row 1 of the bands that follow a
; changed pointer row shows the wrong kind. So the handler has a margin of 40-60 cycles.
;
; Build:  go run -C asm . -o ../c64port/build/cube_mux.prg -C ../c64port/c64.cfg ../c64port/experiments/cube_mux.s
; Run:    x64sc -console -sound -warp -limitcycles 6000000 -exitscreenshot cube_mux.png ../c64port/build/cube_mux.prg
; Check:  the screenshot (384x272) shows the cells from x=96, y=39 in steps of 24; a cell shows its
;         kind's pattern in rows 0-20 and black in rows 21-23

.include "../src/include/vic.inc"

SCREEN = $4000
CHARSET = $6000
SPRITE_DATA = $6800
POINTERS = SCREEN + $3F8
SPRITE_POINTER = $A0            ; SPRITE_DATA / 64
CIA1_TA_LO = $DC04
CIA1_TA_HI = $DC05
CIA1_TB_LO = $DC06
CIA1_TB_HI = $DC07
CIA1_ICR = $DC0D
CIA1_CRA = $DC0E
CIA1_CRB = $DC0F
CIA2_PORTA = $DD00
TIMER_A_PERIOD = 63 * 3 - 1 - 1
TIMER_B_PERIOD = 63 * 2 + 1 - 1

SPRITE_Y = 54
BANDS = 8
BAND_HEIGHT = 24
IRQ_OFFSET = 22                 ; the first line of the gap, relative to the sprite Y
IRQ_VECTOR = $FFFE

.segment "ZEROPAGE"

CPU_PORT = $01
ZP_BAND: .res 1

.segment "LOADADDR"
.word $0801

.segment "CODE"
    .byte $0B, $08
    .byte $0A, $00
    .byte $9E
    .byte $20
    .byte $32, $30, $36, $32
    .byte $00
    .byte $00, $00

.export main
main:
    jmp boot

irq:
    pha
    bit VIC_IRQ_STATUS
    bmi @raster

    bit CIA1_ICR

    nop
    inc VIC_BORDER
    nop
    dec VIC_BORDER
    nop

    bit VIC_IRQ_STATUS
    bmi @raster
    pla
    rti
@raster:
    ; STRESS
    txa
    pha
    ldx ZP_BAND
    lda band_y,x
    sta VIC_SPR0_Y+0
    sta VIC_SPR0_Y+2
    sta VIC_SPR0_Y+4
    sta VIC_SPR0_Y+6
    sta VIC_SPR0_Y+8
    sta VIC_SPR0_Y+10
    sta VIC_SPR0_Y+12
    sta VIC_SPR0_Y+14
    lda pointers_0,x
    sta POINTERS+0
    lda pointers_1,x
    sta POINTERS+1
    lda pointers_2,x
    sta POINTERS+2
    lda pointers_3,x
    sta POINTERS+3
    lda pointers_4,x
    sta POINTERS+4
    lda pointers_5,x
    sta POINTERS+5
    lda pointers_6,x
    sta POINTERS+6
    lda pointers_7,x
    sta POINTERS+7
    lda irq_line,x
    sta VIC_RASTER
    lda #1
    sta VIC_IRQ_STATUS         ; acknowledge the raster interrupt
    inx
    txa
    and #BANDS - 1
    sta ZP_BAND
    pla
    tax
    pla
    rti

boot:
    sei
    lda #$35
    sta CPU_PORT
    lda #$7F
    sta CIA1_ICR
    lda CIA1_ICR

    lda CIA2_PORTA
    and #$FC
    ora #$02
    sta CIA2_PORTA

    ldx #0
@clear:
    lda #$0
    sta SCREEN,x
    sta SCREEN+$100,x
    sta SCREEN+$200,x
    sta SCREEN+$2E8,x
    sta CHARSET,x
    inx
    bne @clear

    ldx #0
@copy:
    lda kind_data,x
    sta SPRITE_DATA,x
    inx
    bne @copy

    ldx #7
@pointers:
    lda pointers_0_first,x
    sta POINTERS,x
    dex
    bpl @pointers

    lda #$17
    sta VIC_CTRL1
    lda #$00
    sta VIC_CTRL2
    lda #$08
    sta VIC_MEMPTR
    lda #COLOR_DARK_GREY
    sta VIC_BORDER
    lda #COLOR_BLACK
    sta VIC_BG0
    lda #0
    sta VIC_SPR_EXPAND_Y
    sta VIC_SPR_EXPAND_X
    sta VIC_SPR_MULTICOLOR
    sta VIC_SPR_HI_X
    ldx #7
@sprites:
    lda #COLOR_WHITE
    sta VIC_SPR0_COLOR,x
    txa
    asl
    tay
    lda sprite_x,x
    sta VIC_SPR0_X,y
    lda #SPRITE_Y
    sta VIC_SPR0_Y,y
    dex
    bpl @sprites
    lda #$80
    sta VIC_SPR_HI_X
    lda #$FF
    sta VIC_SPR_ENABLE

    lda #1
    sta ZP_BAND

    lda #<irq
    sta IRQ_VECTOR
    lda #>irq
    sta IRQ_VECTOR+1
    lda #SPRITE_Y + IRQ_OFFSET
    sta VIC_RASTER
    lda #$01
    sta VIC_IRQ_STATUS
    sta VIC_IRQ_ENABLE

    lda #<TIMER_A_PERIOD
    sta CIA1_TA_LO
    lda #>TIMER_A_PERIOD
    sta CIA1_TA_HI
    lda #<TIMER_B_PERIOD
    sta CIA1_TB_LO
    lda #>TIMER_B_PERIOD
    sta CIA1_TB_HI
    lda #$81
    sta CIA1_ICR                ; enable timer A interrupt
    lda #$82
    sta CIA1_ICR                ; enable timer B interrupt
    lda #$11
    sta CIA1_CRA                ; force load, start timer A continuous
    sta CIA1_CRB                ; force load, start timer B continuous
    cli
    ldx #$f6
@forever:
    inc VIC_BORDER-$f6,x
    dec VIC_BORDER
    nop
    jmp @forever

.segment "RODATA"

sprite_x:
    .byte 88, 112, 136, 160, 184, 208, 232, 0

band_y:
    .byte 54, 78, 102, 126, 150, 174, 198, 222

irq_line:
    .byte 76, 100, 124, 148, 172, 196, 220, 244

; The sprite pointers of band 0, which the sprites show before the first interrupt
pointers_0_first:
    .byte $A3, $A3, $A3, $A3, $A3, $A3, $A3, $A3

; The pointer of every sprite by band: the ring of the cell, kind k at SPRITE_DATA + 64 * k
pointers_0:
    .byte $A3, $A3, $A3, $A3, $A3, $A3, $A3, $A3
pointers_1:
    .byte $A3, $A2, $A2, $A2, $A2, $A2, $A2, $A3
pointers_2:
    .byte $A3, $A2, $A1, $A1, $A1, $A1, $A2, $A3
pointers_3:
    .byte $A3, $A2, $A1, $A0, $A0, $A1, $A2, $A3
pointers_4:
    .byte $A3, $A2, $A1, $A0, $A0, $A1, $A2, $A3
pointers_5:
    .byte $A3, $A2, $A1, $A1, $A1, $A1, $A2, $A3
pointers_6:
    .byte $A3, $A2, $A2, $A2, $A2, $A2, $A2, $A3
pointers_7:
    .byte $A3, $A3, $A3, $A3, $A3, $A3, $A3, $A3

kind_data:
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $00
    .byte $FF, $FF, $FF, $00, $00, $00, $FF, $FF, $FF
    .byte $00, $00, $00, $FF, $FF, $FF, $00, $00, $00
    .byte $FF, $FF, $FF, $00, $00, $00, $FF, $FF, $FF
    .byte $00, $00, $00, $FF, $FF, $FF, $00, $00, $00
    .byte $FF, $FF, $FF, $00, $00, $00, $FF, $FF, $FF
    .byte $00, $00, $00, $FF, $FF, $FF, $00, $00, $00
    .byte $FF, $FF, $FF, $00, $00, $00, $FF, $FF, $FF
    .byte $00
    .byte $AA, $AA, $AA, $AA, $AA, $AA, $AA, $AA, $AA
    .byte $AA, $AA, $AA, $AA, $AA, $AA, $AA, $AA, $AA
    .byte $AA, $AA, $AA, $AA, $AA, $AA, $AA, $AA, $AA
    .byte $AA, $AA, $AA, $AA, $AA, $AA, $AA, $AA, $AA
    .byte $AA, $AA, $AA, $AA, $AA, $AA, $AA, $AA, $AA
    .byte $AA, $AA, $AA, $AA, $AA, $AA, $AA, $AA, $AA
    .byte $AA, $AA, $AA, $AA, $AA, $AA, $AA, $AA, $AA
    .byte $00
    .byte $AA, $AA, $AA, $55, $55, $55, $AA, $AA, $AA
    .byte $55, $55, $55, $AA, $AA, $AA, $55, $55, $55
    .byte $AA, $AA, $AA, $55, $55, $55, $AA, $AA, $AA
    .byte $55, $55, $55, $AA, $AA, $AA, $55, $55, $55
    .byte $AA, $AA, $AA, $55, $55, $55, $AA, $AA, $AA
    .byte $55, $55, $55, $AA, $AA, $AA, $55, $55, $55
    .byte $AA, $AA, $AA, $55, $55, $55, $AA, $AA, $AA
    .byte $00
