; Generated sine/cosine lookup tables for 21 animation frames
; sin(i * 4.5°) * 256, stored as 16-bit little-endian

.segment "RODATA"

.export sine_table
sine_table:
    .byte $00, $00  ; frame  0:   0.0°
    .byte $14, $00  ; frame  1:   4.5°
    .byte $28, $00  ; frame  2:   9.0°
    .byte $3C, $00  ; frame  3:  13.5°
    .byte $4F, $00  ; frame  4:  18.0°
    .byte $62, $00  ; frame  5:  22.5°
    .byte $74, $00  ; frame  6:  27.0°
    .byte $86, $00  ; frame  7:  31.5°
    .byte $96, $00  ; frame  8:  36.0°
    .byte $A6, $00  ; frame  9:  40.5°
    .byte $B5, $00  ; frame 10:  45.0°
    .byte $C3, $00  ; frame 11:  49.5°
    .byte $CF, $00  ; frame 12:  54.0°
    .byte $DA, $00  ; frame 13:  58.5°
    .byte $E4, $00  ; frame 14:  63.0°
    .byte $ED, $00  ; frame 15:  67.5°
    .byte $F3, $00  ; frame 16:  72.0°
    .byte $F9, $00  ; frame 17:  76.5°
    .byte $FD, $00  ; frame 18:  81.0°
    .byte $FF, $00  ; frame 19:  85.5°
    .byte $00, $01  ; frame 20:  90.0°

.export cosine_table
cosine_table:
    .byte $00, $01  ; frame  0:   0.0°
    .byte $FF, $00  ; frame  1:   4.5°
    .byte $FD, $00  ; frame  2:   9.0°
    .byte $F9, $00  ; frame  3:  13.5°
    .byte $F3, $00  ; frame  4:  18.0°
    .byte $ED, $00  ; frame  5:  22.5°
    .byte $E4, $00  ; frame  6:  27.0°
    .byte $DA, $00  ; frame  7:  31.5°
    .byte $CF, $00  ; frame  8:  36.0°
    .byte $C3, $00  ; frame  9:  40.5°
    .byte $B5, $00  ; frame 10:  45.0°
    .byte $A6, $00  ; frame 11:  49.5°
    .byte $96, $00  ; frame 12:  54.0°
    .byte $86, $00  ; frame 13:  58.5°
    .byte $74, $00  ; frame 14:  63.0°
    .byte $62, $00  ; frame 15:  67.5°
    .byte $4F, $00  ; frame 16:  72.0°
    .byte $3C, $00  ; frame 17:  76.5°
    .byte $28, $00  ; frame 18:  81.0°
    .byte $14, $00  ; frame 19:  85.5°
    .byte $00, $00  ; frame 20:  90.0°
