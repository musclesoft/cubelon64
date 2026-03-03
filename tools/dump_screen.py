#!/usr/bin/env python3
"""
Dump screen RAM from a VICE snapshot or memory dump
"""
import sys

def dump_screen_from_prg(prg_file):
    """Analyze the PRG file to understand what it will write"""
    with open(prg_file, 'rb') as f:
        data = f.read()

    # PRG format: first 2 bytes are load address
    load_addr = data[0] | (data[1] << 8)
    print(f"Load address: ${load_addr:04X}")
    print(f"Program size: {len(data) - 2} bytes")

    # Look for pattern in the code that writes to screen
    # Search for STA $0400 patterns
    for i in range(len(data) - 3):
        if data[i] == 0x8D:  # STA absolute
            addr = data[i+1] | (data[i+2] << 8)
            if 0x0400 <= addr < 0x0800:
                print(f"Found STA ${addr:04X} at offset ${i:04X}")

def create_memory_inspector():
    """Create an assembly program that dumps screen RAM"""
    code = """; Memory inspector - dumps screen RAM to see what's displayed
.include "include/vic.inc"
.include "include/kernal.inc"

; BASIC stub: 10 SYS 2061
.segment "LOADADDR"
        .word $0801

.segment "EXEHDR"
        .word @end
        .word 10
        .byte $9E
        .byte "2061"
        .byte 0
@end:   .word 0

.segment "STARTUP"
_main:
        ; Display first 3 rows of screen RAM in hex
        lda #14
        jsr CHROUT      ; Switch to lowercase

        ldx #0
@row_loop:
        ; Print row header
        txa
        jsr print_hex_byte
        lda #':'
        jsr CHROUT
        lda #' '
        jsr CHROUT

        ; Print 40 bytes
        ldy #0
@col_loop:
        lda $0400,x
        jsr print_hex_byte
        lda #' '
        jsr CHROUT

        inx
        iny
        cpy #40
        bne @col_loop

        ; New line
        lda #13
        jsr CHROUT

        ; Stop after 3 rows
        cpx #120
        bne @row_loop

@done:
        jmp @done

print_hex_byte:
        pha
        lsr
        lsr
        lsr
        lsr
        jsr print_hex_nybble
        pla
        and #$0F
        jsr print_hex_nybble
        rts

print_hex_nybble:
        cmp #10
        bcc @digit
        ; A-F
        adc #'A'-11
        jmp CHROUT
@digit:
        adc #'0'
        jmp CHROUT
"""

    with open('src/inspector.s', 'w') as f:
        f.write(code)
    print("Created src/inspector.s - build and run to see screen RAM contents")

if __name__ == '__main__':
    if len(sys.argv) > 1:
        dump_screen_from_prg(sys.argv[1])
    else:
        create_memory_inspector()
