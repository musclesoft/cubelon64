; shared.inc - Variables shared between modules
; ABSOLUTE ADDRESSING ONLY - No zero page
; Base address: $0200

; ═══════════════════════════════════════════════════════════
; ASSERTION MACROS - Empty for ca65, parsed by custom assembler
; ═══════════════════════════════════════════════════════════
.macro assert_if cond, val, symbol, file, length, offset
    ; Runtime assertion - parsed by custom assembler, ignored by ca65
.endmacro

.macro assert symbol, file, length, offset
    ; Runtime assertion - parsed by custom assembler, ignored by ca65
.endmacro

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

; Animation state
FRAME = $0210            ; Current animation frame (0-40 with 4-phase optimization, angles 0-20)
RENDER_PHASE = $0211     ; Render phase: 0=compute, 1=display (for 2-frame rendering)
