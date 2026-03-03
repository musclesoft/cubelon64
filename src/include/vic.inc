; VIC-II register definitions for C64
; $D000-$D02E

; Sprite positions
VIC_SPR0_X      = $D000
VIC_SPR0_Y      = $D001
VIC_SPR1_X      = $D002
VIC_SPR1_Y      = $D003
VIC_SPR2_X      = $D004
VIC_SPR2_Y      = $D005
VIC_SPR3_X      = $D006
VIC_SPR3_Y      = $D007
VIC_SPR4_X      = $D008
VIC_SPR4_Y      = $D009
VIC_SPR5_X      = $D00A
VIC_SPR5_Y      = $D00B
VIC_SPR6_X      = $D00C
VIC_SPR6_Y      = $D00D
VIC_SPR7_X      = $D00E
VIC_SPR7_Y      = $D00F

; Sprite X MSB
VIC_SPR_HI_X    = $D010

; Control registers
VIC_CTRL1       = $D011  ; Vertical fine scroll and control
VIC_RASTER      = $D012  ; Raster counter
VIC_LPEN_X      = $D013  ; Light pen X
VIC_LPEN_Y      = $D014  ; Light pen Y
VIC_SPR_ENA     = $D015  ; Sprite enable
VIC_CTRL2       = $D016  ; Horizontal fine scroll and control
VIC_SPR_EXP_Y   = $D017  ; Sprite Y expansion
VIC_ADDR        = $D018  ; Memory pointers
VIC_IRR         = $D019  ; Interrupt register
VIC_IMR         = $D01A  ; Interrupt mask register

; Sprite properties
VIC_SPR_PRIO    = $D01B  ; Sprite priority
VIC_SPR_MCOLOR  = $D01C  ; Sprite multicolor mode
VIC_SPR_EXP_X   = $D01D  ; Sprite X expansion
VIC_SPR_COLL    = $D01E  ; Sprite-sprite collision
VIC_SPR_BG      = $D01F  ; Sprite-background collision

; Border and background
VIC_BORDER      = $D020  ; Border color
VIC_BG0         = $D021  ; Background color 0
VIC_BG1         = $D022  ; Background color 1
VIC_BG2         = $D023  ; Background color 2
VIC_BG3         = $D024  ; Background color 3
VIC_SPR_MCOLOR0 = $D025  ; Sprite multicolor 0
VIC_SPR_MCOLOR1 = $D026  ; Sprite multicolor 1

; Sprite colors
VIC_SPR0_COLOR  = $D027
VIC_SPR1_COLOR  = $D028
VIC_SPR2_COLOR  = $D029
VIC_SPR3_COLOR  = $D02A
VIC_SPR4_COLOR  = $D02B
VIC_SPR5_COLOR  = $D02C
VIC_SPR6_COLOR  = $D02D
VIC_SPR7_COLOR  = $D02E

; Control register 1 bits
VIC_CTRL1_RST8  = $80    ; 9th raster bit
VIC_CTRL1_ECM   = $40    ; Extended color mode
VIC_CTRL1_BMM   = $20    ; Bitmap mode
VIC_CTRL1_DEN   = $10    ; Display enable
VIC_CTRL1_RSEL  = $08    ; Row select (25/24 rows)

; Control register 2 bits
VIC_CTRL2_MCM   = $10    ; Multicolor mode
VIC_CTRL2_CSEL  = $08    ; Column select (40/38 columns)

; C64 color values
COLOR_BLACK     = 0
COLOR_WHITE     = 1
COLOR_RED       = 2
COLOR_CYAN      = 3
COLOR_PURPLE    = 4
COLOR_GREEN     = 5
COLOR_BLUE      = 6
COLOR_YELLOW    = 7
COLOR_ORANGE    = 8
COLOR_BROWN     = 9
COLOR_LTRED     = 10
COLOR_GRAY1     = 11
COLOR_GRAY2     = 12
COLOR_LTGREEN   = 13
COLOR_LTBLUE    = 14
COLOR_GRAY3     = 15
