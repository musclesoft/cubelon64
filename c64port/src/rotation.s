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

; mul24_signed16: compute 24 × value using shift-and-add (24 = 16 + 8)
; Input: ROT_MUL_A (16-bit signed lo, hi)
; Output: ROT_MUL_RESULT (16-bit signed result lo, hi)
; Replaces expensive mul16x16 with shifts and adds for the constant 24
mul24_signed16:
    ; Check sign and convert to positive if negative
    lda ROT_MUL_A+1
    bpl @positive
    ; Negative: negate the input
    lda #0
    sec
    sbc ROT_MUL_A
    sta ROT_MUL_A
    lda #0
    sbc ROT_MUL_A+1
    sta ROT_MUL_A+1
    lda #1
    sta ROT_MUL_RESULT+4      ; Save sign flag
    jmp @do_multiply
@positive:
    lda #0
    sta ROT_MUL_RESULT+4      ; Clear sign flag
@do_multiply:
    ; val << 4 (multiply by 16): 4 left shifts
    lda ROT_MUL_A
    sta ROT_MUL_RESULT+5      ; val16Lo
    lda ROT_MUL_A+1
    sta ROT_MUL_RESULT+6      ; val16Hi
    ; Shift 1
    asl ROT_MUL_RESULT+5
    rol ROT_MUL_RESULT+6
    ; Shift 2
    asl ROT_MUL_RESULT+5
    rol ROT_MUL_RESULT+6
    ; Shift 3
    asl ROT_MUL_RESULT+5
    rol ROT_MUL_RESULT+6
    ; Shift 4
    asl ROT_MUL_RESULT+5
    rol ROT_MUL_RESULT+6

    ; val << 3 (multiply by 8): 3 left shifts
    lda ROT_MUL_A
    sta ROT_MUL_RESULT+7      ; val8Lo
    lda ROT_MUL_A+1
    sta ROT_MUL_RESULT+8      ; val8Hi
    ; Shift 1
    asl ROT_MUL_RESULT+7
    rol ROT_MUL_RESULT+8
    ; Shift 2
    asl ROT_MUL_RESULT+7
    rol ROT_MUL_RESULT+8
    ; Shift 3
    asl ROT_MUL_RESULT+7
    rol ROT_MUL_RESULT+8

    ; Add: (val << 4) + (val << 3) = 24 * val
    lda ROT_MUL_RESULT+5      ; val16Lo
    clc
    adc ROT_MUL_RESULT+7      ; val8Lo
    sta ROT_MUL_RESULT        ; resultLo
    lda ROT_MUL_RESULT+6      ; val16Hi
    adc ROT_MUL_RESULT+8      ; val8Hi
    sta ROT_MUL_RESULT+1      ; resultHi

    ; Negate result if input was negative
    lda ROT_MUL_RESULT+4
    beq @done
    ; Negate: two's complement
    lda #0
    sec
    sbc ROT_MUL_RESULT
    sta ROT_MUL_RESULT
    lda #0
    sbc ROT_MUL_RESULT+1
    sta ROT_MUL_RESULT+1
@done:
    rts

; Calculate rotated square corners using shift-and-add optimization
; Coordinates are directly in 48×48 supersample space (no scaling needed).
; Since corners are at distances 0, ±24 from pivot, and 24 = 16 + 8,
; we replace expensive 16×16 multiplication with shift-and-add.
; Outputs corner coordinates in Fixed88 supersample format
.export calc_rotation
calc_rotation:
    ; Look up sin/cos values from tables based on FRAME
    ; Each table entry is 2 bytes (16-bit little-endian)
    ; Table index = FRAME * 2
    lda FRAME
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

    ; Precompute 24*sin using shift-and-add
    lda ROT_SIN_VAL
    sta ROT_MUL_A
    lda ROT_SIN_VAL+1
    sta ROT_MUL_A+1
    jsr mul24_signed16
    ; Save sin24 in temp storage
    lda ROT_MUL_RESULT
    sta ROT_MUL_RESULT+9      ; sin24Lo
    lda ROT_MUL_RESULT+1
    sta ROT_MUL_RESULT+10     ; sin24Hi

    ; Precompute 24*cos using shift-and-add
    lda ROT_COS_VAL
    sta ROT_MUL_A
    lda ROT_COS_VAL+1
    sta ROT_MUL_A+1
    jsr mul24_signed16
    ; Save cos24 in temp storage
    lda ROT_MUL_RESULT
    sta ROT_MUL_RESULT+11     ; cos24Lo
    lda ROT_MUL_RESULT+1
    sta ROT_MUL_RESULT+12     ; cos24Hi

    ; Corner 0: (-24, -24) relative to pivot
    ; x' = -24*cos - (-24)*sin = 24*sin - 24*cos
    ; y' = -24*sin + (-24)*cos = -(24*sin + 24*cos)

    ; rx0 = sin24 - cos24
    lda ROT_MUL_RESULT+9      ; sin24Lo
    sec
    sbc ROT_MUL_RESULT+11     ; cos24Lo
    sta ROT_MUL_RESULT        ; rx0Lo
    lda ROT_MUL_RESULT+10     ; sin24Hi
    sbc ROT_MUL_RESULT+12     ; cos24Hi
    sta ROT_MUL_RESULT+1      ; rx0Hi

    ; sum0 = sin24 + cos24
    lda ROT_MUL_RESULT+9      ; sin24Lo
    clc
    adc ROT_MUL_RESULT+11     ; cos24Lo
    sta ROT_MUL_RESULT+2      ; sum0Lo
    lda ROT_MUL_RESULT+10     ; sin24Hi
    adc ROT_MUL_RESULT+12     ; cos24Hi
    sta ROT_MUL_RESULT+3      ; sum0Hi

    ; ry0 = -sum0 (two's complement)
    lda #0
    sec
    sbc ROT_MUL_RESULT+2      ; sum0Lo
    sta ROT_MUL_RESULT+4      ; ry0Lo
    lda #0
    sbc ROT_MUL_RESULT+3      ; sum0Hi
    sta ROT_MUL_RESULT+5      ; ry0Hi

    ; Add pivot (36,36) = $2400 to corner 0
    lda ROT_MUL_RESULT        ; rx0Lo
    clc
    adc #$00                  ; pivotLo
    sta CORNER0_X
    lda ROT_MUL_RESULT+1      ; rx0Hi
    adc #$24                  ; pivotHi
    sta CORNER0_X+1
    lda ROT_MUL_RESULT+4      ; ry0Lo
    clc
    adc #$00                  ; pivotLo
    sta CORNER0_Y
    lda ROT_MUL_RESULT+5      ; ry0Hi
    adc #$24                  ; pivotHi
    sta CORNER0_Y+1

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

    ; Add pivot to corner 1
    lda ROT_MUL_RESULT        ; rx1Lo
    clc
    adc #$00
    sta CORNER1_X
    lda ROT_MUL_RESULT+1      ; rx1Hi
    adc #$24
    sta CORNER1_X+1
    lda ROT_MUL_RESULT+2      ; ry1Lo
    clc
    adc #$00
    sta CORNER1_Y
    lda ROT_MUL_RESULT+3      ; ry1Hi
    adc #$24
    sta CORNER1_Y+1

    ; Corner 2: (0, 0) - pivot itself, no rotation
    lda #$00
    sta CORNER2_X
    lda #$24
    sta CORNER2_X+1
    lda #$00
    sta CORNER2_Y
    lda #$24
    sta CORNER2_Y+1

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
    adc #$24
    sta CORNER3_X+1
    lda ROT_MUL_RESULT+6      ; ry3Lo
    clc
    adc #$00
    sta CORNER3_Y
    lda ROT_MUL_RESULT+7      ; ry3Hi
    adc #$24
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
