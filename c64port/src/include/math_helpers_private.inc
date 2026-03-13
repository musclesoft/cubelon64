; math_helpers_private.inc - Memory locations for math helper functions
; Shared between math_helpers.s and render.s

; Memory locations for math operations (using absolute addressing)
; Located after render private vars to avoid conflicts
MATH_DIV_LO = $02C2         ; Dividend/divisor lo byte
MATH_DIV_HI = $02C3         ; Dividend/divisor hi byte
MATH_DIV2_LO = $02C4        ; Second operand lo
MATH_DIV2_HI = $02C5        ; Second operand hi
MATH_QUOT_LO = $02C6        ; Quotient lo byte
MATH_QUOT_HI = $02C7        ; Quotient hi byte (for 16-bit results)
MATH_REM_LO = $02C8         ; Remainder lo byte
MATH_REM_HI = $02C9         ; Remainder hi byte
MATH_DIV_B2 = $02CA         ; Third dividend byte (for 24-bit)
MATH_MUL_A_LO = $02CB       ; Multiplicand lo
MATH_MUL_A_HI = $02CC       ; Multiplicand hi
MATH_MUL_B = $02CD          ; Multiplier (8-bit)
MATH_PROD_B0 = $02CE        ; Product byte 0
MATH_PROD_B1 = $02CF        ; Product byte 1
MATH_PROD_B2 = $02D0        ; Product byte 2
MATH_SHIFT_LO = $02D1       ; Shifted value lo
MATH_SHIFT_HI = $02D2       ; Shifted value hi
MATH_SHIFT_TOP = $02D3      ; Shifted value top byte
