; Zero page allocations for C64 port

.segment "ZEROPAGE"

; Fixed-point coordinates (8.8 format - 256 units per pixel)
corner0_x: .res 2
corner0_y: .res 2
corner1_x: .res 2
corner1_y: .res 2
corner2_x: .res 2
corner2_y: .res 2
corner3_x: .res 2
corner3_y: .res 2

; Rotation state
angle: .res 1          ; Current rotation angle (0-255)
cos_val: .res 1        ; Cosine value (-128 to +127)
sin_val: .res 1        ; Sine value (-128 to +127)

; Frame counter
frame: .res 1

; Temporary variables for calculations
temp1: .res 2
temp2: .res 2
temp3: .res 2
