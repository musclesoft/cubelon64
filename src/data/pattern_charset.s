; Clean test charset - only defined patterns, rest blank
.export pattern_charset

pattern_charset:

; Tile 0: Blank
.byte $00
.byte $00
.byte $00
.byte $00
.byte $00
.byte $00
.byte $00
.byte $00

; Tile 1: Checkerboard
.byte $AA, $55, $AA, $55
.byte $AA, $55, $AA, $55

; Tile 2: Inverse checkerboard
.byte $55, $AA, $55, $AA
.byte $55, $AA, $55, $AA

; Tile 3: Horizontal stripes
.byte $FF, $00, $FF, $00
.byte $FF, $00, $FF, $00

; Tile 4: Vertical stripes
.byte $CC, $CC, $CC, $CC
.byte $CC, $CC, $CC, $CC

; Tile 5: Diagonal
.byte $80, $40, $20, $10
.byte $08, $04, $02, $01

; Tile 6: Opposite diagonal
.byte $01, $02, $04, $08
.byte $10, $20, $40, $80

; Tile 7: Dots
.byte $88, $00, $22, $00
.byte $88, $00, $22, $00

; Tile 8: Cross (animation frame)
.byte $18, $18, $FF, $FF
.byte $18, $18, $18, $18

; Tile 9: Border (animation frame)
.byte $FF, $81, $81, $81
.byte $81, $81, $81, $FF

; Tile 10: Filled border (animation frame)
.byte $FF, $FF, $C3, $C3
.byte $C3, $C3, $FF, $FF

; Tiles 11-31: Blank
.repeat 21
.byte $00, $00, $00, $00
.byte $00, $00, $00, $00
.endrep

; Tile 32: Space (blank)
.byte $00, $00, $00, $00
.byte $00, $00, $00, $00

; Tiles 33-159: All blank
.repeat 127
.byte $00, $00, $00, $00
.byte $00, $00, $00, $00
.endrep

; Tile 160: Solid block
.byte $FF, $FF, $FF, $FF
.byte $FF, $FF, $FF, $FF

; Tiles 161-255: All blank
.repeat 95
.byte $00, $00, $00, $00
.byte $00, $00, $00, $00
.endrep
