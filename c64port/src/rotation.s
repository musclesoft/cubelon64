; Rotation and rendering for C64
; Implements fixed-point 8.8 rotation with lookup tables

.include "include/shared_imports.inc"
.include "include/rotation_private.inc"

; Import sine and cosine tables
.import sine_table: absolute
.import cosine_table: absolute

PIVOT_X_HI = $1C
PIVOT_Y_HI = $24

.segment "CODE"

; Calculate rotated square corners using pre-multiplied sin/cos table.
; Coordinates are directly in 48×48 supersample space (no scaling needed).
; Table contains pre-multiplied values: sin(angle) * 256 * 24.
; Outputs corner coordinates in Fixed88 supersample format
.export calc_rotation
; The pivot is (28, 36) pixels (-4 px offset). With rx = sin24 and ry = -cos24 the corners are
; corner 1 = (rx, ry), corner 3 = (ry, -rx), corner 0 = corner 1 + corner 3 - pivot, corner 2 = pivot
calc_rotation:
    lda BUILD_SLOT
    asl
    tax
    lda sine_table,x
    sta ROT_RX
    lda sine_table+1,x
    sta ROT_RX+1
    sec
    lda #0
    sbc cosine_table,x
    sta ROT_RY
    lda #0
    sbc cosine_table+1,x
    sta ROT_RY+1
    sec
    lda #0
    sbc ROT_RX
    sta ROT_NEGATIVE_RX
    lda #0
    sbc ROT_RX+1
    sta ROT_NEGATIVE_RX+1

    lda ROT_RX
    sta CORNER1_X
    lda ROT_RX+1
    clc
    adc #PIVOT_X_HI
    sta CORNER1_X+1
    lda ROT_RY
    sta CORNER1_Y
    lda ROT_RY+1
    clc
    adc #PIVOT_Y_HI
    sta CORNER1_Y+1

    lda ROT_RY
    sta CORNER3_X
    lda ROT_RY+1
    clc
    adc #PIVOT_X_HI
    sta CORNER3_X+1
    lda ROT_NEGATIVE_RX
    sta CORNER3_Y
    lda ROT_NEGATIVE_RX+1
    clc
    adc #PIVOT_Y_HI
    sta CORNER3_Y+1

    lda ROT_RX
    clc
    adc ROT_RY
    sta CORNER0_X
    lda ROT_RX+1
    adc ROT_RY+1
    clc
    adc #PIVOT_X_HI
    sta CORNER0_X+1
    lda ROT_RY
    clc
    adc ROT_NEGATIVE_RX
    sta CORNER0_Y
    lda ROT_RY+1
    adc ROT_NEGATIVE_RX+1
    clc
    adc #PIVOT_Y_HI
    sta CORNER0_Y+1

    lda #0
    sta CORNER2_X
    sta CORNER2_Y
    lda #PIVOT_X_HI
    sta CORNER2_X+1
    lda #PIVOT_Y_HI
    sta CORNER2_Y+1
    rts
