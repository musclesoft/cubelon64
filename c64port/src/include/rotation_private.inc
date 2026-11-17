; rotation_private.inc - Private variables for rotation.s ONLY
; ABSOLUTE ADDRESSING ONLY - No zero page, No temps
; Base address: $0220

; ═══════════════════════════════════════════════════════════
; ROTATION MODULE PRIVATE - Do NOT use in other modules
; ═══════════════════════════════════════════════════════════

; Workspace of calc_rotation: the corner 1 offset (rx, ry) and -rx, 16 bits each
ROT_RX = $022C
ROT_RY = $022E
ROT_NEGATIVE_RX = $0230
