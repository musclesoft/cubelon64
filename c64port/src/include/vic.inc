; vic.inc - VIC-II chip register definitions for C64
; Commodore 64 Video Interface Controller (VIC-II) memory-mapped registers

; ═══════════════════════════════════════════════════════════════════════════
; VIC-II Base Address
; ═══════════════════════════════════════════════════════════════════════════
VIC_BASE = $D000

; ═══════════════════════════════════════════════════════════════════════════
; Sprite Position Registers ($D000-$D00F)
; ═══════════════════════════════════════════════════════════════════════════
VIC_SPR0_X   = VIC_BASE + $00   ; Sprite 0 X position (low 8 bits)
VIC_SPR0_Y   = VIC_BASE + $01   ; Sprite 0 Y position
VIC_SPR1_X   = VIC_BASE + $02   ; Sprite 1 X position
VIC_SPR1_Y   = VIC_BASE + $03   ; Sprite 1 Y position
VIC_SPR2_X   = VIC_BASE + $04   ; Sprite 2 X position
VIC_SPR2_Y   = VIC_BASE + $05   ; Sprite 2 Y position
VIC_SPR3_X   = VIC_BASE + $06   ; Sprite 3 X position
VIC_SPR3_Y   = VIC_BASE + $07   ; Sprite 3 Y position
VIC_SPR4_X   = VIC_BASE + $08   ; Sprite 4 X position
VIC_SPR4_Y   = VIC_BASE + $09   ; Sprite 4 Y position
VIC_SPR5_X   = VIC_BASE + $0A   ; Sprite 5 X position
VIC_SPR5_Y   = VIC_BASE + $0B   ; Sprite 5 Y position
VIC_SPR6_X   = VIC_BASE + $0C   ; Sprite 6 X position
VIC_SPR6_Y   = VIC_BASE + $0D   ; Sprite 6 Y position
VIC_SPR7_X   = VIC_BASE + $0E   ; Sprite 7 X position
VIC_SPR7_Y   = VIC_BASE + $0F   ; Sprite 7 Y position

VIC_SPR_HI_X = VIC_BASE + $10   ; Sprites 0-7 X MSB (9th bit for X > 255)

; ═══════════════════════════════════════════════════════════════════════════
; Control Registers ($D011-$D012, $D016)
; ═══════════════════════════════════════════════════════════════════════════
VIC_CTRL1    = VIC_BASE + $11   ; Control register 1
                                ; Bit 7: RST8 (raster compare bit 8)
                                ; Bit 6: ECM (extended color mode)
                                ; Bit 5: BMM (bitmap mode)
                                ; Bit 4: DEN (display enable)
                                ; Bit 3: RSEL (25/24 row select)
                                ; Bits 2-0: YSCROLL (vertical scroll)

VIC_RASTER   = VIC_BASE + $12   ; Raster counter (read) / compare (write)

VIC_CTRL2    = VIC_BASE + $16   ; Control register 2
                                ; Bit 5: RES (reset)
                                ; Bit 4: MCM (multicolor mode)
                                ; Bit 3: CSEL (40/38 column select)
                                ; Bits 2-0: XSCROLL (horizontal scroll)

; ═══════════════════════════════════════════════════════════════════════════
; Light Pen Registers ($D013-$D014)
; ═══════════════════════════════════════════════════════════════════════════
VIC_LIGHTPEN_X = VIC_BASE + $13 ; Light pen X position
VIC_LIGHTPEN_Y = VIC_BASE + $14 ; Light pen Y position

; ═══════════════════════════════════════════════════════════════════════════
; Sprite Enable and Control ($D015, $D017, $D01B-$D01D)
; ═══════════════════════════════════════════════════════════════════════════
VIC_SPR_ENABLE     = VIC_BASE + $15 ; Sprite enable (1 bit per sprite)
VIC_SPR_EXPAND_Y   = VIC_BASE + $17 ; Sprite Y expansion (double height)
VIC_SPR_PRIORITY   = VIC_BASE + $1B ; Sprite-background priority (1=behind)
VIC_SPR_MULTICOLOR = VIC_BASE + $1C ; Sprite multicolor mode (0=hires, 1=MC)
VIC_SPR_EXPAND_X   = VIC_BASE + $1D ; Sprite X expansion (double width)

; ═══════════════════════════════════════════════════════════════════════════
; Memory Pointers ($D018)
; ═══════════════════════════════════════════════════════════════════════════
VIC_MEMPTR   = VIC_BASE + $18   ; Memory pointers
                                ; Bits 7-4: Video matrix base (screen RAM)
                                ; Bits 3-1: Character pattern base (charset)
                                ; Bit 0: unused

; ═══════════════════════════════════════════════════════════════════════════
; Interrupt Control ($D019-$D01A)
; ═══════════════════════════════════════════════════════════════════════════
VIC_IRQ_STATUS = VIC_BASE + $19 ; Interrupt status (read) / acknowledge (write)
                                ; Bit 7: IRQ occurred (any source)
                                ; Bit 3: Light pen triggered
                                ; Bit 2: Sprite-sprite collision
                                ; Bit 1: Sprite-background collision
                                ; Bit 0: Raster compare match

VIC_IRQ_ENABLE = VIC_BASE + $1A ; Interrupt enable
                                ; Same bit layout as $D019

; ═══════════════════════════════════════════════════════════════════════════
; Collision Detection ($D01E-$D01F) - Read Only
; ═══════════════════════════════════════════════════════════════════════════
VIC_SPR_SPR_COLL = VIC_BASE + $1E ; Sprite-sprite collision (cleared by read)
VIC_SPR_BG_COLL  = VIC_BASE + $1F ; Sprite-background collision (cleared by read)

; ═══════════════════════════════════════════════════════════════════════════
; Color Registers ($D020-$D02E)
; ═══════════════════════════════════════════════════════════════════════════
VIC_BORDER   = VIC_BASE + $20   ; Border color
VIC_BG0      = VIC_BASE + $21   ; Background color 0 (main background)
VIC_BG1      = VIC_BASE + $22   ; Background color 1 (multicolor/ECM)
VIC_BG2      = VIC_BASE + $23   ; Background color 2 (multicolor/ECM)
VIC_BG3      = VIC_BASE + $24   ; Background color 3 (ECM only)

; Sprite colors ($D027-$D02E)
VIC_SPR0_COLOR = VIC_BASE + $27 ; Sprite 0 color
VIC_SPR1_COLOR = VIC_BASE + $28 ; Sprite 1 color
VIC_SPR2_COLOR = VIC_BASE + $29 ; Sprite 2 color
VIC_SPR3_COLOR = VIC_BASE + $2A ; Sprite 3 color
VIC_SPR4_COLOR = VIC_BASE + $2B ; Sprite 4 color
VIC_SPR5_COLOR = VIC_BASE + $2C ; Sprite 5 color
VIC_SPR6_COLOR = VIC_BASE + $2D ; Sprite 6 color
VIC_SPR7_COLOR = VIC_BASE + $2E ; Sprite 7 color

; Aliases for common names
VIC_BG = VIC_BG0                ; Main background color

; ═══════════════════════════════════════════════════════════════════════════
; C64 Color Codes (for use with color registers)
; ═══════════════════════════════════════════════════════════════════════════
COLOR_BLACK       = 0
COLOR_WHITE       = 1
COLOR_RED         = 2
COLOR_CYAN        = 3
COLOR_PURPLE      = 4
COLOR_GREEN       = 5
COLOR_BLUE        = 6
COLOR_YELLOW      = 7
COLOR_ORANGE      = 8
COLOR_BROWN       = 9
COLOR_LIGHT_RED   = 10
COLOR_DARK_GREY   = 11
COLOR_MID_GREY    = 12
COLOR_LIGHT_GREEN = 13
COLOR_LIGHT_BLUE  = 14
COLOR_LIGHT_GREY  = 15
