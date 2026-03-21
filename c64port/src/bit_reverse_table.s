; Bit reversal lookup table - runtime initialization
; Generates 256-byte table at startup instead of storing in ROM
; Saves 256 bytes of data at cost of ~20 bytes of init code

.include "include/shared_imports.inc"

.segment "BSS"

; 256-byte table aligned to page boundary for fast access
.align 256
.export BIT_REVERSE_TABLE
BIT_REVERSE_TABLE:
    .res 256

.segment "CODE"

; Initialize bit reversal table
; Generates table by reversing bits 0-255
; Uses: A, X, Y (all clobbered)
; Cycles: ~2,304 (9 cycles/iteration × 256)
.export init_bit_reverse_table
.proc init_bit_reverse_table
    ldx #0                  ; X = 0 (table index)

@loop:
    txa                     ; Copy X → A (byte to reverse)
    ldy #8                  ; Loop counter (8 bits)

@rev_bit:
    asl                     ; Shift MSB → carry
    ror BIT_REVERSE_TABLE,x ; Rotate carry → table[X]
    dey                     ; Decrement loop counter
    bne @rev_bit            ; Repeat 8 times

    inx                     ; Next table index
    bne @loop               ; Loop until X wraps to 0

    rts
.endproc
