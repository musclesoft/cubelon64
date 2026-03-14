; Generated sine lookup table for 21 animation frames
; Pre-multiplied: sin(i * 4.5°) * 256 * 24, stored as 16-bit little-endian
; Note: cosine uses same table with reversed index: cos(frame) = sin(20-frame)

.segment "RODATA"

.export sine_table
sine_table:
    .byte $00, $00  ; frame  0:   0.0° (value: 0)
    .byte $E2, $01  ; frame  1:   4.5° (value: 482)
    .byte $C1, $03  ; frame  2:   9.0° (value: 961)
    .byte $9A, $05  ; frame  3:  13.5° (value: 1434)
    .byte $6B, $07  ; frame  4:  18.0° (value: 1899)
    .byte $2F, $09  ; frame  5:  22.5° (value: 2351)
    .byte $E5, $0A  ; frame  6:  27.0° (value: 2789)
    .byte $8A, $0C  ; frame  7:  31.5° (value: 3210)
    .byte $1B, $0E  ; frame  8:  36.0° (value: 3611)
    .byte $96, $0F  ; frame  9:  40.5° (value: 3990)
    .byte $F8, $10  ; frame 10:  45.0° (value: 4344)
    .byte $40, $12  ; frame 11:  49.5° (value: 4672)
    .byte $6B, $13  ; frame 12:  54.0° (value: 4971)
    .byte $77, $14  ; frame 13:  58.5° (value: 5239)
    .byte $62, $15  ; frame 14:  63.0° (value: 5474)
    .byte $2C, $16  ; frame 15:  67.5° (value: 5676)
    .byte $D3, $16  ; frame 16:  72.0° (value: 5843)
    .byte $56, $17  ; frame 17:  76.5° (value: 5974)
    .byte $B4, $17  ; frame 18:  81.0° (value: 6068)
    .byte $ED, $17  ; frame 19:  85.5° (value: 6125)
    .byte $00, $18  ; frame 20:  90.0° (value: 6144)
