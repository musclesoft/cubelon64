; Math helper functions for 16-bit Bresenham DDA
; Implements division and multiplication primitives from proto/cpu8.go

.include "include/shared_imports.inc"
.include "include/math_helpers_private.inc"

.segment "CODE"

; div16x16: unsigned 16÷16 → (quotient byte, remainder 16-bit)
; Input: MATH_DIV_LO/HI = dividend (16-bit)
;        MATH_DIV2_LO/HI = divisor (16-bit)
; Output: MATH_QUOT_LO = quotient (fits in byte: max 48 for our geometry)
;         MATH_REM_LO/HI = remainder (16-bit)
; Uses shift-and-subtract algorithm: 16 iterations
; Mirrors div16x16 from proto/cpu8.go lines 175-209
.export div16x16
div16x16:
    ; Clear quotient and remainder
    lda #0
    sta MATH_QUOT_LO
    sta MATH_REM_LO
    sta MATH_REM_HI

    ; Guard: check for divisor == 0
    lda MATH_DIV2_LO
    ora MATH_DIV2_HI
    bne @start
    lda #$FF
    sta MATH_QUOT_LO
    rts

@start:
    ; 16 iterations of shift-and-subtract
    ldx #16
@loop:
    ; Shift dividend left 16-bit, MSB feeds into remainder via carry
    asl MATH_DIV_LO         ; Shift lo, MSB → carry
    rol MATH_DIV_HI         ; Rotate hi, carry in, MSB → carry

    ; ROL remainder 16-bit: carry from dividend MSB becomes remainder LSB
    rol MATH_REM_LO         ; Rotate lo, carry in from dividend
    rol MATH_REM_HI         ; Rotate hi, carry from lo

    ; ASL quotient (make room for new bit)
    asl MATH_QUOT_LO

    ; Compare remainder with divisor: remainder >= divisor?
    ; Use SEC/SBC to test: if carry is set after subtraction, no borrow occurred
    lda MATH_REM_LO
    sec
    sbc MATH_DIV2_LO
    sta MATH_SHIFT_LO       ; Save tentative result lo
    lda MATH_REM_HI
    sbc MATH_DIV2_HI
    sta MATH_SHIFT_HI       ; Save tentative result hi
    bcc @no_subtract        ; Carry clear = borrow = remainder < divisor

    ; remainder >= divisor: commit subtraction and set quotient bit
    lda MATH_SHIFT_LO
    sta MATH_REM_LO
    lda MATH_SHIFT_HI
    sta MATH_REM_HI
    inc MATH_QUOT_LO

@no_subtract:
    dex
    bne @loop
div16_done:
    rts

; mul16x8: unsigned 16×8 → 24-bit result
; Input: MATH_MUL_A_LO/HI = 16-bit multiplicand
;        MATH_MUL_B = 8-bit multiplier
; Output: MATH_PROD_B0/B1/B2 = 24-bit product
; Uses shift-and-add algorithm with early exit
; Mirrors mul16x8 from proto/cpu8.go lines 215-254
.export mul16x8
mul16x8:
    ; Clear result
    lda #0
    sta MATH_PROD_B0
    sta MATH_PROD_B1
    sta MATH_PROD_B2

    ; Check for zero multiplier
    lda MATH_MUL_B
    beq mul16_done

    ; Initialize shifted multiplicand (24-bit): starts at (aLo, aHi, 0)
    lda MATH_MUL_A_LO
    sta MATH_SHIFT_LO
    lda MATH_MUL_A_HI
    sta MATH_SHIFT_HI
    lda #0
    sta MATH_SHIFT_TOP

    ; 8 iterations
    ldx #8
@loop:
    ; Test LSB of multiplier (already in A from previous iteration or init)
    lsr MATH_MUL_B          ; Shift right, LSB → carry
    bcc @skip_add           ; If bit was 0, skip addition

    ; Add shifted multiplicand (24-bit) to result
    lda MATH_PROD_B0
    clc
    adc MATH_SHIFT_LO
    sta MATH_PROD_B0
    lda MATH_PROD_B1
    adc MATH_SHIFT_HI
    sta MATH_PROD_B1
    lda MATH_PROD_B2
    adc MATH_SHIFT_TOP
    sta MATH_PROD_B2

@skip_add:
    ; Check for early exit: if multiplier is now 0, we're done
    lda MATH_MUL_B
    beq mul16_done

    ; Shift multiplicand left (24-bit)
    asl MATH_SHIFT_LO
    rol MATH_SHIFT_HI
    rol MATH_SHIFT_TOP

    dex
    bne @loop

mul16_done:
    rts

; div24x16: unsigned 24÷16 → (quotient 16-bit, remainder 16-bit)
; Input: MATH_DIV_B2/DIV_HI/DIV_LO = dividend (24-bit, high to low)
;        MATH_DIV2_LO/HI = divisor (16-bit)
; Output: MATH_QUOT_LO/HI = quotient (16-bit)
;         MATH_REM_LO/HI = remainder (16-bit)
; Uses shift-and-subtract algorithm: 24 iterations
; Mirrors div24x16 from proto/cpu8.go lines 260-297
.export div24x16
div24x16:
    ; Clear quotient and remainder
    lda #0
    sta MATH_QUOT_LO
    sta MATH_QUOT_HI
    sta MATH_REM_LO
    sta MATH_REM_HI

    ; Guard: check for divisor == 0
    lda MATH_DIV2_LO
    ora MATH_DIV2_HI
    bne @start
    lda #$FF
    sta MATH_QUOT_LO
    sta MATH_QUOT_HI
    rts

@start:
    ; 24 iterations of shift-and-subtract
    ldx #24
@loop:
    ; Shift dividend left 24-bit, MSB feeds into remainder via carry
    asl MATH_DIV_LO         ; Shift b0, MSB → carry
    rol MATH_DIV_HI         ; Rotate b1
    rol MATH_DIV_B2         ; Rotate b2, MSB → carry

    ; ROL remainder 16-bit
    rol MATH_REM_LO
    rol MATH_REM_HI

    ; ASL quotient (16-bit)
    asl MATH_QUOT_LO
    rol MATH_QUOT_HI

    ; Compare remainder with divisor
    lda MATH_REM_LO
    sec
    sbc MATH_DIV2_LO
    sta MATH_SHIFT_LO       ; Save tentative result lo
    lda MATH_REM_HI
    sbc MATH_DIV2_HI
    sta MATH_SHIFT_HI       ; Save tentative result hi
    bcc @no_subtract

    ; remainder >= divisor: commit subtraction and increment quotient
    lda MATH_SHIFT_LO
    sta MATH_REM_LO
    lda MATH_SHIFT_HI
    sta MATH_REM_HI
    inc MATH_QUOT_LO
    bne @no_subtract
    inc MATH_QUOT_HI

@no_subtract:
    dex
    bne @loop
div24_done:
    rts
