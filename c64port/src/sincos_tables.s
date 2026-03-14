; Generated sine/cosine lookup tables for 21 animation frames
; sin(i * 4.5°) * 256, stored as single byte (value-1)
; On load: add 1 to recover value in range [1,256]

.segment "RODATA"

.export sine_table
sine_table:
    .byte $00  ; frame  0:   0.0° (stored: 0, actual: 1)
    .byte $13  ; frame  1:   4.5° (stored: 19, actual: 20)
    .byte $27  ; frame  2:   9.0° (stored: 39, actual: 40)
    .byte $3B  ; frame  3:  13.5° (stored: 59, actual: 60)
    .byte $4E  ; frame  4:  18.0° (stored: 78, actual: 79)
    .byte $61  ; frame  5:  22.5° (stored: 97, actual: 98)
    .byte $73  ; frame  6:  27.0° (stored: 115, actual: 116)
    .byte $85  ; frame  7:  31.5° (stored: 133, actual: 134)
    .byte $95  ; frame  8:  36.0° (stored: 149, actual: 150)
    .byte $A5  ; frame  9:  40.5° (stored: 165, actual: 166)
    .byte $B4  ; frame 10:  45.0° (stored: 180, actual: 181)
    .byte $C2  ; frame 11:  49.5° (stored: 194, actual: 195)
    .byte $CE  ; frame 12:  54.0° (stored: 206, actual: 207)
    .byte $D9  ; frame 13:  58.5° (stored: 217, actual: 218)
    .byte $E3  ; frame 14:  63.0° (stored: 227, actual: 228)
    .byte $EC  ; frame 15:  67.5° (stored: 236, actual: 237)
    .byte $F2  ; frame 16:  72.0° (stored: 242, actual: 243)
    .byte $F8  ; frame 17:  76.5° (stored: 248, actual: 249)
    .byte $FC  ; frame 18:  81.0° (stored: 252, actual: 253)
    .byte $FE  ; frame 19:  85.5° (stored: 254, actual: 255)
    .byte $FF  ; frame 20:  90.0° (stored: 255, actual: 256)

.export cosine_table
cosine_table:
    .byte $FF  ; frame  0:   0.0° (stored: 255, actual: 256)
    .byte $FE  ; frame  1:   4.5° (stored: 254, actual: 255)
    .byte $FC  ; frame  2:   9.0° (stored: 252, actual: 253)
    .byte $F8  ; frame  3:  13.5° (stored: 248, actual: 249)
    .byte $F2  ; frame  4:  18.0° (stored: 242, actual: 243)
    .byte $EC  ; frame  5:  22.5° (stored: 236, actual: 237)
    .byte $E3  ; frame  6:  27.0° (stored: 227, actual: 228)
    .byte $D9  ; frame  7:  31.5° (stored: 217, actual: 218)
    .byte $CE  ; frame  8:  36.0° (stored: 206, actual: 207)
    .byte $C2  ; frame  9:  40.5° (stored: 194, actual: 195)
    .byte $B4  ; frame 10:  45.0° (stored: 180, actual: 181)
    .byte $A5  ; frame 11:  49.5° (stored: 165, actual: 166)
    .byte $95  ; frame 12:  54.0° (stored: 149, actual: 150)
    .byte $85  ; frame 13:  58.5° (stored: 133, actual: 134)
    .byte $73  ; frame 14:  63.0° (stored: 115, actual: 116)
    .byte $61  ; frame 15:  67.5° (stored: 97, actual: 98)
    .byte $4E  ; frame 16:  72.0° (stored: 78, actual: 79)
    .byte $3B  ; frame 17:  76.5° (stored: 59, actual: 60)
    .byte $27  ; frame 18:  81.0° (stored: 39, actual: 40)
    .byte $13  ; frame 19:  85.5° (stored: 19, actual: 20)
    .byte $00  ; frame 20:  90.0° (stored: 0, actual: 1)
