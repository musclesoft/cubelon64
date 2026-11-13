; sprite24.s - VICE test: 21 row sprites that show 24 distinct raster lines, 8 bands back to back, 8 sprites,
; driven by one raster interrupt per band with exact write times
;
; Rows 19-21 are Y-expanded, so they last 2 lines each (18 + 3*2 = 24 lines). The screen base in $D018 is
; switched so the repeated line of each of those rows is fetched through the other sprite pointer table.
; Every sprite shows a diagonal whose column is the line number minus 1 (1-24): a correct result is clean
; diagonals; a repeated or skipped line shows as a step. Neighbouring sprites alternate between a solid
; line (3 pixels) and a double line (the outer 2 of those pixels, hollow middle), and each band flips them,
; so a line fetched through the wrong screen pair shows as the other style. Both lines spill one pixel into
; the neighbouring sprite at lines 1 and 24, so every sprite has those two pixels and both lines continue
; across sprite and band borders.
;
; Line plan (R1 = first displayed line, a bad line with YSCROLL=7):
;   lines 1-18  rows 1-18 from screen A
;   lines 19-24 rows 19,19,20,20,21,21 from screens B,A,A,B,B,A
; Bands follow each other without a gap.
;
; Odd and even bands alternate between two screen pairs: A,B and C,D. A band uses its pair as main and
; partner screen (lines 19-24 are fetched from the partner,main,main,partner,partner,main), so the only per
; band state is the pair of $D018 values in ZP_MEMPTR_A and ZP_MEMPTR_B. The setup fills the sprite pointers
; of both pairs from pointer_table (8 per pair, the partner screen gets the pointer with bit 0 set).
;
; Build:  go run -C asm . -o ../c64port/build/sprite24.prg -C ../c64port/c64.cfg ../c64port/experiments/sprite24.s

.include "../src/include/vic.inc"

SCREEN_A = $4000
SCREEN_B = $4400
SCREEN_C = $4800
SCREEN_D = $4C00
CHARSET = $6000
SPRITE_A = $6800
SPRITE_B = $6840
SPRITE_HOLLOW_A = $6880
SPRITE_HOLLOW_B = $68C0
POINTERS_A = SCREEN_A + $3F8
POINTERS_B = SCREEN_B + $3F8
POINTERS_C = SCREEN_C + $3F8
POINTERS_D = SCREEN_D + $3F8
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
IRQ_OFFSET = 18 ; offset into sprite line
SCREEN_PARTNER_EOR = $10 ; $D018 screen bits, A differs from B by this
SCREEN_PAIR_EOR = $20  ; $D018 screen bits, A and B differ from C and D by this
IRQ_VECTOR = $FFFE

.segment "ZEROPAGE"

CPU_PORT = $01
ZP_MEMPTR_A: .res 1
ZP_MEMPTR_B: .res 1

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

    nop ; 18 cycles available
    inc VIC_BORDER
    nop
    dec VIC_BORDER
    nop

    bit VIC_IRQ_STATUS
    bmi @raster
    pla
    rti
@raster:  ; timing: depending on instructions here, jitter is reduced by 1 more
    lda ZP_MEMPTR_B
    sta VIC_MEMPTR
    dec VIC_SPR_EXPAND_Y       ; Y expand all sprites
    lda #1
@reduce_jitter:
    bit VIC_RASTER
    beq @reduce_jitter

    sta VIC_IRQ_STATUS         ; acknowledge the raster interrupt

    lda ZP_MEMPTR_A
    sta VIC_MEMPTR

    ; idle: cycles available, e.g. for CIA based SID update
    lda #15
@idle_loop:
    sec
    sbc #1
    bne @idle_loop

    lda ZP_MEMPTR_B
    sta VIC_MEMPTR

    lda VIC_SPR0_Y
    clc
    adc #24
    cmp #SPRITE_Y + 24 * 8
    bcc @no_y_wrap
    lda #SPRITE_Y
    clc
@no_y_wrap:
    sta VIC_SPR0_Y+0
    sta VIC_SPR0_Y+2
    sta VIC_SPR0_Y+4
    sta VIC_SPR0_Y+6
    sta VIC_SPR0_Y+8
    sta VIC_SPR0_Y+10
    sta VIC_SPR0_Y+12
    sta VIC_SPR0_Y+14
    adc #IRQ_OFFSET
    sta VIC_RASTER

    lda ZP_MEMPTR_A
    sta VIC_MEMPTR
    inc VIC_SPR_EXPAND_Y       ; Y expand off

    nop
    nop
    nop
    nop
    nop
    nop
    nop

    lda ZP_MEMPTR_B            ; only needed if odd and even bands differ
    eor #SCREEN_PAIR_EOR
    sta ZP_MEMPTR_B
    eor #SCREEN_PARTNER_EOR
    sta ZP_MEMPTR_A
    sta VIC_MEMPTR             ; line 25 and the next band's first 18 lines: its main screen

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
    sta SCREEN_A,x
    sta SCREEN_A+$100,x
    sta SCREEN_A+$200,x
    sta SCREEN_A+$2E8,x
    sta SCREEN_B,x
    sta SCREEN_B+$100,x
    sta SCREEN_B+$200,x
    sta SCREEN_B+$2E8,x
    sta SCREEN_C,x
    sta SCREEN_C+$100,x
    sta SCREEN_C+$200,x
    sta SCREEN_C+$2E8,x
    sta SCREEN_D,x
    sta SCREEN_D+$100,x
    sta SCREEN_D+$200,x
    sta SCREEN_D+$2E8,x
    sta CHARSET,x
    inx
    bne @clear
    lda #$55
    sta CHARSET

    ldx #0
@copy:
    lda data_a,x
    sta SPRITE_A,x
    inx
    bne @copy

    ldx #7
@pointers:
    lda pointer_table,x
    sta POINTERS_A,x
    ora #1
    sta POINTERS_B,x
    lda pointer_table+8,x
    sta POINTERS_C,x
    ora #1
    sta POINTERS_D,x
    dex
    bpl @pointers

    lda #$17
    sta VIC_CTRL1
    lda #$00
    sta VIC_CTRL2
    lda #$08
    sta VIC_MEMPTR
    sta ZP_MEMPTR_A
    eor #SCREEN_PARTNER_EOR
    sta ZP_MEMPTR_B
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
    lda #COLOR_GREEN
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

    lda #<irq
    sta IRQ_VECTOR
    lda #>irq
    sta IRQ_VECTOR+1
    lda #SPRITE_Y
    clc
    adc #IRQ_OFFSET
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

pointer_table:
    .byte $A0, $A2, $A0, $A2, $A0, $A2, $A0, $A2
    .byte $A2, $A0, $A2, $A0, $A2, $A0, $A2, $A0

data_a:
    .byte $C0, $00, $01
    .byte $E0, $00, $00
    .byte $70, $00, $00
    .byte $38, $00, $00
    .byte $1C, $00, $00
    .byte $0E, $00, $00
    .byte $07, $00, $00
    .byte $03, $80, $00
    .byte $01, $C0, $00
    .byte $00, $E0, $00
    .byte $00, $70, $00
    .byte $00, $38, $00
    .byte $00, $1C, $00
    .byte $00, $0E, $00
    .byte $00, $07, $00
    .byte $00, $03, $80
    .byte $00, $01, $C0
    .byte $00, $00, $E0
    .byte $00, $00, $38
    .byte $00, $00, $1C
    .byte $80, $00, $03
    .byte $00
data_b:
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $70
    .byte $00, $00, $0E
    .byte $00, $00, $07
    .byte $00
data_hollow_a:
    .byte $40, $00, $01
    .byte $A0, $00, $00
    .byte $50, $00, $00
    .byte $28, $00, $00
    .byte $14, $00, $00
    .byte $0A, $00, $00
    .byte $05, $00, $00
    .byte $02, $80, $00
    .byte $01, $40, $00
    .byte $00, $A0, $00
    .byte $00, $50, $00
    .byte $00, $28, $00
    .byte $00, $14, $00
    .byte $00, $0A, $00
    .byte $00, $05, $00
    .byte $00, $02, $80
    .byte $00, $01, $40
    .byte $00, $00, $A0
    .byte $00, $00, $28
    .byte $00, $00, $14
    .byte $80, $00, $02
    .byte $00
data_hollow_b:
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $00
    .byte $00, $00, $50
    .byte $00, $00, $0A
    .byte $00, $00, $05
    .byte $00
