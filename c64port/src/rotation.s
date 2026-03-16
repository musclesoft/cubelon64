; Rotation and rendering for C64
; Implements fixed-point 8.8 rotation with lookup tables

.include "include/shared_imports.inc"
.include "include/rotation_private.inc"

; Import sine table (cosine computed as sine[20-frame])
.import sine_table: absolute

; Constants
SQUARE_SIZE = 6       ; Half-size of square in pixels (12x12 total) - matches reference

.segment "CODE"

; Calculate rotated square corners using pre-multiplied sin/cos table.
; Coordinates are directly in 48×48 supersample space (no scaling needed).
; Table contains pre-multiplied values: sin(angle) * 256 * 24.
; Outputs corner coordinates in Fixed88 supersample format
.export calc_rotation
calc_rotation:
    ; Look up pre-multiplied sin/cos values from table based on FRAME
    ; Each table entry is 2 bytes (16-bit little-endian): sin*256*24
    ; Cosine uses reversed index: cos(frame) = sin(20-frame)

    ; Calculate table offset: FRAME * 2 (each entry is 2 bytes)
    lda FRAME
    asl                     ; Multiply by 2
    tax                     ; X = table offset

    ; Load sin24 directly from sine_table
    lda sine_table,x        ; sin24Lo
    sta ROT_MUL_RESULT+9
    lda sine_table+1,x      ; sin24Hi
    sta ROT_MUL_RESULT+10

    ; Calculate cos table offset: (20-FRAME) * 2
    lda #20
    sec
    sbc FRAME
    asl                     ; Multiply by 2
    tax                     ; X = cos table offset

    ; Load cos24 directly from sine_table (reversed index)
    lda sine_table,x        ; cos24Lo
    sta ROT_MUL_RESULT+11
    lda sine_table+1,x      ; cos24Hi
    sta ROT_MUL_RESULT+12

    ; Corner 1: (0, -24) relative to pivot
    ; x' = 0*cos - (-24)*sin = 24*sin
    ; y' = 0*sin + (-24)*cos = -24*cos

    ; rx1 = sin24 (already in ROT_MUL_RESULT+9,+10)
    lda ROT_MUL_RESULT+9      ; sin24Lo
    sta ROT_MUL_RESULT        ; rx1Lo
    lda ROT_MUL_RESULT+10     ; sin24Hi
    sta ROT_MUL_RESULT+1      ; rx1Hi

    ; ry1 = -cos24 (two's complement)
    lda #0
    sec
    sbc ROT_MUL_RESULT+11     ; cos24Lo
    sta ROT_MUL_RESULT+2      ; ry1Lo
    lda #0
    sbc ROT_MUL_RESULT+12     ; cos24Hi
    sta ROT_MUL_RESULT+3      ; ry1Hi

    ; Add pivot to corner 1: (28, 36) for -4px storage
    lda ROT_MUL_RESULT        ; rx1Lo
    clc
    adc #$00
    sta CORNER1_X
    lda ROT_MUL_RESULT+1      ; rx1Hi
    adc #$24                  ; Pivot X = $1C00 = 28*256 (-4px offset)
    sta CORNER1_X+1
    lda ROT_MUL_RESULT+2      ; ry1Lo
    clc
    adc #$00
    sta CORNER1_Y
    lda ROT_MUL_RESULT+3      ; ry1Hi
    adc #$24
    sta CORNER1_Y+1

    ; Corner 3: 90° rotation of corner 1's RELATIVE result
    ; (x1_rot, y1_rot) → (y1_rot, -x1_rot) then add pivot
    ; x3_rot = y1_rot (ry1 from ROT_MUL_RESULT+2,+3)
    lda ROT_MUL_RESULT+2      ; ry1Lo
    sta ROT_MUL_RESULT+4      ; rx3Lo
    lda ROT_MUL_RESULT+3      ; ry1Hi
    sta ROT_MUL_RESULT+5      ; rx3Hi

    ; y3_rot = -x1_rot = -(rx1) (two's complement)
    lda #0
    sec
    sbc ROT_MUL_RESULT        ; rx1Lo
    sta ROT_MUL_RESULT+6      ; ry3Lo
    lda #0
    sbc ROT_MUL_RESULT+1      ; rx1Hi
    sta ROT_MUL_RESULT+7      ; ry3Hi

    ; Add pivot to corner 3
    lda ROT_MUL_RESULT+4      ; rx3Lo
    clc
    adc #$00
    sta CORNER3_X
    lda ROT_MUL_RESULT+5      ; rx3Hi
    adc #$24                  ; Pivot X = $1C00 = 28*256 (-4px offset)
    sta CORNER3_X+1
    lda ROT_MUL_RESULT+6      ; ry3Lo
    clc
    adc #$00
    sta CORNER3_Y
    lda ROT_MUL_RESULT+7      ; ry3Hi
    adc #$24
    sta CORNER3_Y+1

    ; Corner 0: Computed from Corner 1 + Corner 3 - Pivot
    ; Since rotation is linear: Corner0(rel) = Corner1(rel) + Corner3(rel)
    ; In absolute coords: Corner0 = Corner1 + Corner3 - Pivot

    ; x0 = x1 + x3 - pivot
    ; First: compute sum = x1 + x3 (16-bit)
    lda CORNER1_X
    clc
    adc CORNER3_X
    sta CORNER0_X             ; Store low byte of sum temporarily
    lda CORNER1_X+1
    adc CORNER3_X+1
    ; Now A = high byte of sum, CORNER0_X = low byte of sum
    ; Second: subtract pivot from sum (16-bit)
    pha                       ; Save high byte of sum
    lda CORNER0_X             ; Get low byte of sum
    sec
    sbc #$00                  ; Subtract pivotLo
    sta CORNER0_X
    pla                       ; Restore high byte of sum
    sbc #$24                  ; Subtract pivotHi = $1C (-4px offset)
    sta CORNER0_X+1

    ; y0 = y1 + y3 - pivot
    ; First: compute sum = y1 + y3 (16-bit)
    lda CORNER1_Y
    clc
    adc CORNER3_Y
    sta CORNER0_Y             ; Store low byte of sum temporarily
    lda CORNER1_Y+1
    adc CORNER3_Y+1
    ; Now A = high byte of sum, CORNER0_Y = low byte of sum
    ; Second: subtract pivot from sum (16-bit)
    pha                       ; Save high byte of sum
    lda CORNER0_Y             ; Get low byte of sum
    sec
    sbc #$00                  ; Subtract pivotLo
    sta CORNER0_Y
    pla                       ; Restore high byte of sum
    sbc #$24                  ; Subtract pivotHi (carry already propagated)
    sta CORNER0_Y+1

    ; Corner 2: pivot itself (28, 36), no rotation
    lda #$00
    sta CORNER2_X
    lda #$24                  ; Pivot X = $1C00 = 28*256 (-4px offset)
    sta CORNER2_X+1
    lda #$00
    sta CORNER2_Y
    lda #$24
    sta CORNER2_Y+1

    ; Column offset for frames >= 8 to eliminate wrapping
    ; Shift all corners left by 16 supersample pixels (8 output pixels)
    lda FRAME
    cmp #8
    bcc @no_offset          ; Skip if frame < 8

    ; Subtract 16 from each corner's X high byte
    lda CORNER0_X+1
    sec
    sbc #16
    sta CORNER0_X+1

    lda CORNER1_X+1
    sec
    sbc #16
    sta CORNER1_X+1

    lda CORNER2_X+1
    sec
    sbc #16
    sta CORNER2_X+1

    lda CORNER3_X+1
    sec
    sbc #16
    sta CORNER3_X+1

    ; Store offset flag for write rotation
    lda #1
    sta USE_OFFSET
    jmp @done

@no_offset:
    lda #0
    sta USE_OFFSET

@done:
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
