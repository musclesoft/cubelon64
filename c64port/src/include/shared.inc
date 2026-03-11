; shared.inc - Variables shared between modules
; ABSOLUTE ADDRESSING ONLY - No zero page
; Base address: $0200

; ═══════════════════════════════════════════════════════════
; SHARED VARIABLES - Accessible to all modules
; ═══════════════════════════════════════════════════════════

; Rotated square corner coordinates (Fixed 8.8 format)
; Written by: rotation.s, Read by: render.s
CORNER0_X = $0200        ; Corner 0 X coordinate (16-bit little-endian)
CORNER0_Y = $0202        ; Corner 0 Y coordinate (16-bit)
CORNER1_X = $0204        ; Corner 1 X coordinate (16-bit)
CORNER1_Y = $0206        ; Corner 1 Y coordinate (16-bit)
CORNER2_X = $0208        ; Corner 2 X coordinate (16-bit)
CORNER2_Y = $020A        ; Corner 2 Y coordinate (16-bit)
CORNER3_X = $020C        ; Corner 3 X coordinate (16-bit)
CORNER3_Y = $020E        ; Corner 3 Y coordinate (16-bit)

; Rotation state
ANGLE = $0210            ; Current rotation angle (8-bit)
FRAME = $0211            ; Current frame number (8-bit)
RENDER_PHASE = $0212     ; Render phase: 0=compute, 1=display (for 2-frame rendering)
