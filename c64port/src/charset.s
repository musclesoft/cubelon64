; Character set management for C64
; Charset location in RAM
CHARSET_RAM = $6000    ; VIC bank 1 + $2000 = $6000-$67FF

; Import SQUARE_CHARS from render.s
.import SQUARE_CHARS: absolute

.segment "CODE"
