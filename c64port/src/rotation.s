; Rotation and rendering for C64
; Implements fixed-point 8.8 rotation with lookup tables

.include "include/shared_imports.inc"
.include "include/rotation_private.inc"

; Import sine/cosine tables
.import sine_table: absolute
.import cosine_table: absolute

; Constants
SQUARE_SIZE = 6       ; Half-size of square in pixels (12x12 total) - matches reference

.segment "CODE"

; Calculate rotated square corners
; Uses the angle stored in zero page
; Outputs corner coordinates in Fixed88 format
.export calc_rotation
calc_rotation:
    ; Look up sin/cos values from tables based on ANGLE
    ; Each table entry is 2 bytes (16-bit little-endian)
    ; Table index = ANGLE * 2
    lda ANGLE
    asl                     ; Multiply by 2 (each entry is 2 bytes)
    tax                     ; X = table offset

    ; Load sin(angle) from sine_table
    lda sine_table,x
    sta ROT_SIN_VAL
    lda sine_table+1,x
    sta ROT_SIN_VAL+1

    ; Load cos(angle) from cosine_table
    lda cosine_table,x
    sta ROT_COS_VAL
    lda cosine_table+1,x
    sta ROT_COS_VAL+1

    ; Calculate the 4 corners of the square
    ; Square size: 12x12 pixels, centered at (12, 12) - MATCHES REFERENCE
    ; Corners span from (6,6) to (18,18) in absolute coordinates
    ; Relative to pivot (18,18): (-12,-12), (+0,-12), (+0,+0), (-12,+0)
    ; Wait no, pivot is at (18,18) but center is at (12,12), so relative to center:
    ; Corners: (-6,-6), (+6,-6), (+6,+6), (-6,+6)
    ; Absolute: (6,6), (18,6), (18,18), (6,18)
    ; But rotation is around pivot (18,18), so relative to pivot:
    ; (6,6) - (18,18) = (-12,-12)
    ; (18,6) - (18,18) = (0,-12)
    ; (18,18) - (18,18) = (0,0)
    ; (6,18) - (18,18) = (-12,0)

    ; Corner 0: (-12, -12) relative to pivot = ($F400, $F400)
    lda #$00
    sta ROT_POINT_X
    lda #$F4
    sta ROT_POINT_X+1
    lda #$00
    sta ROT_POINT_Y
    lda #$F4
    sta ROT_POINT_Y+1
    jsr rotate_point
    ; Add pivot (18,18) = $1200
    lda ROT_MUL_RESULT
    clc
    adc #$00
    sta CORNER0_X
    lda ROT_MUL_RESULT+1
    adc #$12
    sta CORNER0_X+1
    lda ROT_MUL_RESULT+2
    clc
    adc #$00
    sta CORNER0_Y
    lda ROT_MUL_RESULT+3
    adc #$12
    sta CORNER0_Y+1

    ; Corner 1: (0, -12) relative to pivot = ($0000, $F400)
    lda #$00
    sta ROT_POINT_X
    lda #$00
    sta ROT_POINT_X+1
    lda #$00
    sta ROT_POINT_Y
    lda #$F4
    sta ROT_POINT_Y+1
    jsr rotate_point
    lda ROT_MUL_RESULT
    clc
    adc #$00
    sta CORNER1_X
    lda ROT_MUL_RESULT+1
    adc #$12
    sta CORNER1_X+1
    lda ROT_MUL_RESULT+2
    clc
    adc #$00
    sta CORNER1_Y
    lda ROT_MUL_RESULT+3
    adc #$12
    sta CORNER1_Y+1

    ; Corner 2: (0, 0) relative to pivot = ($0000, $0000)
    lda #$00
    sta ROT_POINT_X
    sta ROT_POINT_Y
    sta ROT_POINT_X+1
    sta ROT_POINT_Y+1
    jsr rotate_point
    lda ROT_MUL_RESULT
    clc
    adc #$00
    sta CORNER2_X
    lda ROT_MUL_RESULT+1
    adc #$12
    sta CORNER2_X+1
    lda ROT_MUL_RESULT+2
    clc
    adc #$00
    sta CORNER2_Y
    lda ROT_MUL_RESULT+3
    adc #$12
    sta CORNER2_Y+1

    ; Corner 3: (-12, 0) relative to pivot = ($F400, $0000)
    lda #$00
    sta ROT_POINT_X
    sta ROT_POINT_Y
    lda #$F4
    sta ROT_POINT_X+1
    lda #$00
    sta ROT_POINT_Y+1
    jsr rotate_point
    lda ROT_MUL_RESULT
    clc
    adc #$00
    sta CORNER3_X
    lda ROT_MUL_RESULT+1
    adc #$12
    sta CORNER3_X+1
    lda ROT_MUL_RESULT+2
    clc
    adc #$00
    sta CORNER3_Y
    lda ROT_MUL_RESULT+3
    adc #$12
    sta CORNER3_Y+1

    rts

; Rotate a point (ROT_POINT_X, ROT_POINT_Y) by angle
; Input: ROT_POINT_X = x coordinate (Fixed88), ROT_POINT_Y = y coordinate (Fixed88)
;        ROT_COS_VAL, ROT_SIN_VAL = 16-bit rotation values (cos256/sin256)
; Output: ROT_MUL_RESULT = rotated X, ROT_MUL_RESULT+2 = rotated Y (both Fixed88)
; Algorithm matches Go: x' = (x*cos256 - y*sin256) >> 8
rotate_point:
    ; x' = (x * cos256 - y * sin256) >> 8

    ; Calculate x * cos256 -> 32-bit result
    lda ROT_POINT_X
    sta ROT_MUL_A
    lda ROT_POINT_X+1
    sta ROT_MUL_A+1
    lda ROT_COS_VAL
    sta ROT_MUL_B
    lda ROT_COS_VAL+1
    sta ROT_MUL_B+1
    jsr mul16x16_signed32
    ; Result in ROT_MUL_RESULT+0..3, save on stack (mul16x16 uses ROT_MUL_RESULT+4!)
    lda ROT_MUL_RESULT+3
    pha
    lda ROT_MUL_RESULT+2
    pha
    lda ROT_MUL_RESULT+1
    pha
    lda ROT_MUL_RESULT
    pha

    ; Calculate y * sin256 -> 32-bit result
    lda ROT_POINT_Y
    sta ROT_MUL_A
    lda ROT_POINT_Y+1
    sta ROT_MUL_A+1
    lda ROT_SIN_VAL
    sta ROT_MUL_B
    lda ROT_SIN_VAL+1
    sta ROT_MUL_B+1
    jsr mul16x16_signed32
    ; Result in ROT_MUL_RESULT+0..3

    ; Subtract: (x*cos256) - (y*sin256) as 32-bit
    ; Stack has x*cos256 (pushed low byte first), ROT_MUL_RESULT has y*sin256
    ; Pull x*cos256 into ROT_MUL_RESULT+4..7
    pla
    sta ROT_MUL_RESULT+4
    pla
    sta ROT_MUL_RESULT+5
    pla
    sta ROT_MUL_RESULT+6
    pla
    sta ROT_MUL_RESULT+7

    ; Subtract: ROT_MUL_RESULT+4..7 - ROT_MUL_RESULT+0..3 -> ROT_MUL_RESULT+8..11
    lda ROT_MUL_RESULT+4
    sec
    sbc ROT_MUL_RESULT
    sta ROT_MUL_RESULT+8
    lda ROT_MUL_RESULT+5
    sbc ROT_MUL_RESULT+1
    sta ROT_MUL_RESULT+9
    lda ROT_MUL_RESULT+6
    sbc ROT_MUL_RESULT+2
    sta ROT_MUL_RESULT+10
    lda ROT_MUL_RESULT+7
    sbc ROT_MUL_RESULT+3
    sta ROT_MUL_RESULT+11

    ; Shift right by 8 bits: keep bytes 9..10 (discard byte 8)
    ; Save X on stack before calculating Y (mul16x16 will overwrite ROT_MUL_RESULT!)
    lda ROT_MUL_RESULT+10
    pha                    ; Save x' high byte
    lda ROT_MUL_RESULT+9
    pha                    ; Save x' low byte

    ; y' = (x * sin256 + y * cos256) >> 8

    ; Calculate x * sin256 -> 32-bit result
    lda ROT_POINT_X
    sta ROT_MUL_A
    lda ROT_POINT_X+1
    sta ROT_MUL_A+1
    lda ROT_SIN_VAL
    sta ROT_MUL_B
    lda ROT_SIN_VAL+1
    sta ROT_MUL_B+1
    jsr mul16x16_signed32
    ; Save x*sin256 on stack
    lda ROT_MUL_RESULT+3
    pha
    lda ROT_MUL_RESULT+2
    pha
    lda ROT_MUL_RESULT+1
    pha
    lda ROT_MUL_RESULT
    pha

    ; Calculate y * cos256 -> 32-bit result
    lda ROT_POINT_Y
    sta ROT_MUL_A
    lda ROT_POINT_Y+1
    sta ROT_MUL_A+1
    lda ROT_COS_VAL
    sta ROT_MUL_B
    lda ROT_COS_VAL+1
    sta ROT_MUL_B+1
    jsr mul16x16_signed32

    ; Add: (x*sin256) + (y*cos256) as 32-bit
    ; Pull x*sin256 from stack into ROT_MUL_RESULT+4..7
    pla
    sta ROT_MUL_RESULT+4
    pla
    sta ROT_MUL_RESULT+5
    pla
    sta ROT_MUL_RESULT+6
    pla
    sta ROT_MUL_RESULT+7

    ; Add: ROT_MUL_RESULT+4..7 + ROT_MUL_RESULT+0..3 -> ROT_MUL_RESULT+8..11
    lda ROT_MUL_RESULT+4
    clc
    adc ROT_MUL_RESULT
    sta ROT_MUL_RESULT+8
    lda ROT_MUL_RESULT+5
    adc ROT_MUL_RESULT+1
    sta ROT_MUL_RESULT+9
    lda ROT_MUL_RESULT+6
    adc ROT_MUL_RESULT+2
    sta ROT_MUL_RESULT+10
    lda ROT_MUL_RESULT+7
    adc ROT_MUL_RESULT+3
    sta ROT_MUL_RESULT+11

    ; Shift right by 8 bits: keep bytes 9..10
    lda ROT_MUL_RESULT+9
    sta ROT_MUL_RESULT+2        ; y' low byte
    lda ROT_MUL_RESULT+10
    sta ROT_MUL_RESULT+3        ; y' high byte

    ; Restore X from stack
    pla
    sta ROT_MUL_RESULT          ; x' low byte
    pla
    sta ROT_MUL_RESULT+1        ; x' high byte

    rts

; Signed 16-bit × 16-bit multiplication -> 32-bit result
; Input: ROT_MUL_A (16-bit signed), ROT_MUL_B (16-bit signed)
; Output: ROT_MUL_RESULT+0..3 (32-bit signed result, little-endian)
; Algorithm: Use unsigned multiply, then adjust for signs
mul16x16_signed32:
    ; Save signs
    lda ROT_MUL_A+1
    and #$80
    sta ROT_MUL_RESULT+4        ; Sign of ROT_MUL_A
    lda ROT_MUL_B+1
    and #$80
    eor ROT_MUL_RESULT+4
    sta ROT_MUL_RESULT+4        ; XOR of signs = result sign

    ; Convert ROT_MUL_A to absolute value if negative
    lda ROT_MUL_A+1
    bpl @mul_a_pos
    ; Negate ROT_MUL_A
    lda #0
    sec
    sbc ROT_MUL_A
    sta ROT_MUL_A
    lda #0
    sbc ROT_MUL_A+1
    sta ROT_MUL_A+1
@mul_a_pos:

    ; Convert ROT_MUL_B to absolute value if negative
    lda ROT_MUL_B+1
    bpl @mul_b_pos
    ; Negate ROT_MUL_B
    lda #0
    sec
    sbc ROT_MUL_B
    sta ROT_MUL_B
    lda #0
    sbc ROT_MUL_B+1
    sta ROT_MUL_B+1
@mul_b_pos:

    ; Now do unsigned 16×16 multiplication
    ; Result = ROT_MUL_A * ROT_MUL_B (both positive)
    ; Using standard algorithm: (AH*256 + AL) * (BH*256 + BL)
    ;   = AH*BH*65536 + AH*BL*256 + AL*BH*256 + AL*BL

    ; Clear result
    lda #0
    sta ROT_MUL_RESULT
    sta ROT_MUL_RESULT+1
    sta ROT_MUL_RESULT+2
    sta ROT_MUL_RESULT+3

    ; AL * BL -> bytes 0,1
    lda ROT_MUL_A              ; AL
    ldx ROT_MUL_B              ; BL
    beq @skip_al_bl
    cmp #0                 ; Check if AL is zero
    beq @skip_al_bl
    jsr @multiply_8x8
    ; Result in A (low) and Y (high)
    sta ROT_MUL_RESULT
    sty ROT_MUL_RESULT+1
@skip_al_bl:

    ; AL * BH -> bytes 1,2
    lda ROT_MUL_A              ; AL
    ldx ROT_MUL_B+1            ; BH
    beq @skip_al_bh
    jsr @multiply_8x8
    ; Add to bytes 1,2
    clc
    adc ROT_MUL_RESULT+1
    sta ROT_MUL_RESULT+1
    tya
    adc ROT_MUL_RESULT+2
    sta ROT_MUL_RESULT+2
@skip_al_bh:

    ; AH * BL -> bytes 1,2
    lda ROT_MUL_A+1            ; AH
    ldx ROT_MUL_B              ; BL
    beq @skip_ah_bl
    jsr @multiply_8x8
    ; Add to bytes 1,2
    clc
    adc ROT_MUL_RESULT+1
    sta ROT_MUL_RESULT+1
    tya
    adc ROT_MUL_RESULT+2
    sta ROT_MUL_RESULT+2
    bcc @skip_ah_bl
    inc ROT_MUL_RESULT+3
@skip_ah_bl:

    ; AH * BH -> bytes 2,3
    lda ROT_MUL_A+1            ; AH
    ldx ROT_MUL_B+1            ; BH
    beq @skip_ah_bh
    jsr @multiply_8x8
    ; Add to bytes 2,3
    clc
    adc ROT_MUL_RESULT+2
    sta ROT_MUL_RESULT+2
    tya
    adc ROT_MUL_RESULT+3
    sta ROT_MUL_RESULT+3
@skip_ah_bh:

    ; Negate result if signs were different
    lda ROT_MUL_RESULT+4
    beq @done
    ; Negate 32-bit result
    lda #0
    sec
    sbc ROT_MUL_RESULT
    sta ROT_MUL_RESULT
    lda #0
    sbc ROT_MUL_RESULT+1
    sta ROT_MUL_RESULT+1
    lda #0
    sbc ROT_MUL_RESULT+2
    sta ROT_MUL_RESULT+2
    lda #0
    sbc ROT_MUL_RESULT+3
    sta ROT_MUL_RESULT+3
@done:
    rts

; Helper: 8-bit × 8-bit unsigned multiplication
; Input: A = multiplicand, X = multiplier
; Output: A = low byte, Y = high byte
; Uses repeated addition (simple but slow)
@multiply_8x8:
    sta ROT_MUL_RESULT+5        ; Save multiplicand
    cpx #0
    beq @mul8_zero
    ldy #0                 ; High byte = 0
    lda #0                 ; Result low byte = 0
@mul8_loop:
    clc
    adc ROT_MUL_RESULT+5
    bcc @mul8_no_carry
    iny                    ; Increment high byte on carry
@mul8_no_carry:
    dex
    bne @mul8_loop
    ; A = low byte, Y = high byte
    rts
@mul8_zero:
    lda #0
    ldy #0
    rts
