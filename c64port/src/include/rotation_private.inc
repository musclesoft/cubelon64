; rotation_private.inc - Private variables for rotation.s ONLY
; ABSOLUTE ADDRESSING ONLY - No zero page, No temps
; Base address: $0220

; ═══════════════════════════════════════════════════════════
; ROTATION MODULE PRIVATE - Do NOT use in other modules
; ═══════════════════════════════════════════════════════════

; Trigonometric values (cos256/sin256 scaled by 256)
ROT_COS_VAL = $0220      ; Cosine value (16-bit)
ROT_SIN_VAL = $0222      ; Sine value (16-bit)

; Point being rotated (Fixed 8.8)
ROT_POINT_X = $0224      ; X coordinate of point (16-bit)
ROT_POINT_Y = $0226      ; Y coordinate of point (16-bit)

; Signed 16x16 multiplication inputs/outputs
ROT_MUL_A = $0228        ; Multiplicand A (16-bit signed)
ROT_MUL_B = $022A        ; Multiplicand B (16-bit signed)
ROT_MUL_RESULT = $022C   ; 32-bit result + workspace (bytes 0-11, 12 bytes total)
ROT_MUL_SIGN = $0238     ; Sign of result (0=positive, non-zero=negative)
ROT_MUL_TEMP = $0239     ; Temp for 8x8 helper multiply

; Final rotated coordinates (before adding pivot)
ROT_RESULT_X = $0244     ; Rotated X (16-bit Fixed 8.8)
ROT_RESULT_Y = $0246     ; Rotated Y (16-bit Fixed 8.8)
