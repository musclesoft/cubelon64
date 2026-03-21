; VIC-II initialization module
; Initializes VIC-II chip to known state for cubelon64

.include "include/vic.inc"

; C64 system addresses
COLOR_RAM = $D800           ; Color RAM (not in VIC bank)

.segment "CODE"

; ═══════════════════════════════════════════════════════════════════════════
; Initialize VIC-II chip to known state
; ═══════════════════════════════════════════════════════════════════════════
.export init_vic
.proc init_vic
    ; $D011 (VIC_CTRL1): Control register 1
    ; Bit 7: RST8 (9th bit of raster compare) = 0
    ; Bit 6: ECM (extended color mode) = 0
    ; Bit 5: BMM (bitmap mode) = 0
    ; Bit 4: DEN (display enable) = 1
    ; Bit 3: RSEL (25/24 row select) = 0 (24 rows, borders grow 4px inward)
    ; Bits 2-0: YSCROLL = 7 (shift display to show rows 0-23)
    ; Result: %00010111 = $17 (character mode, display on, 24 rows, scroll 7)
    lda #$17
    sta VIC_CTRL1

    ; $D016 (VIC_CTRL2): Control register 2
    ; Bits 7-6: unused
    ; Bit 5: RES (reset) = 0
    ; Bit 4: MCM (multicolor mode) = 0
    ; Bit 3: CSEL (40/38 column select) = 0 (38 columns)
    ; Bits 2-0: XSCROLL = 0
    ; Result: %00000000 = $00 (hires mode, 38 columns)
    lda #$00
    sta VIC_CTRL2

    ; $D018 (VIC_MEMPTR): Memory pointers
    ; Bits 7-4: Video matrix base (screen RAM): $4400 = bank + $0400 = offset 1
    ; Bits 3-1: Character pattern base: $6000 = bank + $2000 = offset 4
    ; Bit 0: unused
    ; Result: %00011000 = $18 (screen at +$0400, charset at +$2000)
    lda #$18
    sta VIC_MEMPTR

    ; $D020 (VIC_BORDER): Border color
    lda #COLOR_BLACK
    sta VIC_BORDER

    ; $D021 (VIC_BG0): Background color 0
    lda #COLOR_LIGHT_GREY
    sta VIC_BG0

    ; $D022-$D023: Background colors 1-2 (multicolor mode, not used)
    lda #COLOR_BLACK
    sta VIC_BG1
    sta VIC_BG2

    ; $D024: Background color 3 (extended color mode, not used)
    sta VIC_BG3

    ; $D012 (VIC_RASTER): Raster compare (for interrupts, not used)
    lda #$00
    sta VIC_RASTER

    ; $D01A (VIC_IRQ_ENABLE): Interrupt enable - disable all VIC interrupts
    lda #$00
    sta VIC_IRQ_ENABLE

    ; $D019 (VIC_IRQ_STATUS): Interrupt status - acknowledge any pending interrupts
    lda #$FF
    sta VIC_IRQ_STATUS

    ; $D01B (VIC_SPR_PRIORITY): Sprite-to-background priority
    ; 1 = sprite behind background, 0 = sprite in front
    ; Set all sprites (bits 0-7) behind background
    lda #$FF
    sta VIC_SPR_PRIORITY

    ; $D01C (VIC_SPR_MULTICOLOR): Sprite multicolor mode
    ; 0 = hires, 1 = multicolor
    ; Set all sprites to hires mode
    lda #$00
    sta VIC_SPR_MULTICOLOR

    ; $D01D (VIC_SPR_EXPAND_X): Sprite X expansion (double width)
    ; 0 = normal, 1 = double width
    sta VIC_SPR_EXPAND_X

    ; $D01E (VIC_SPR_SPR_COLL): Sprite-to-sprite collision (read-only, clear by reading)
    lda VIC_SPR_SPR_COLL

    ; $D01F (VIC_SPR_BG_COLL): Sprite-to-background collision (read-only, clear by reading)
    lda VIC_SPR_BG_COLL

    ; $D017 (VIC_SPR_EXPAND_Y): Sprite Y expansion (double height)
    ; 0 = normal, 1 = double height
    lda #$00
    sta VIC_SPR_EXPAND_Y

    ; $D010 (VIC_SPR_HI_X): Sprite X MSB (9th bit for X > 255)
    ; Will be set per-sprite during sprite positioning
    sta VIC_SPR_HI_X

    ; $D015 (VIC_SPR_ENABLE): Sprite enable register
    ; Will be set to $FF after sprite setup
    sta VIC_SPR_ENABLE

    ; $D027-$D02E: Sprite colors
    lda #COLOR_DARK_GREY
    ldx #8
@init_sprite_colors:
    dex
    sta VIC_SPR0_COLOR,x
    bne @init_sprite_colors

    ; $D800-$DBE7: Color RAM (1000 bytes for 40×25 screen)
    ; Set all screen colors to black
    ldx #0
    lda #COLOR_BLACK
@init_color_ram:
    sta COLOR_RAM,x
    sta COLOR_RAM+$100,x
    sta COLOR_RAM+$200,x
    sta COLOR_RAM+$2E8,x
    inx
    bne @init_color_ram

    rts
.endproc
