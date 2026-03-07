#!/usr/bin/env python3
"""
Trace the exact write pattern of the Part 3 drawing routine.

From $85AB onwards, trace each POP BC and LD (HL),r to see
how the 36 bytes map to screen positions.
"""

# Starting HL position (example: $4004 for top-left tile)
H = 0x40
L = 0x04

writes = []

# Helper to record a write
def write(byte_idx, is_C, h, l):
    writes.append({
        'byte': byte_idx,
        'reg': 'C' if is_C else 'B',
        'addr': (h << 8) | l,
        'h': h,
        'l': l
    })

# Trace first chunk (36 bytes = 18 POP BC)
byte_idx = 0

# Pattern from disassembly:
# POP BC, LD (HL),C, INC L, LD (HL),B, INC L
# POP BC, LD (HL),C, INC H, LD (HL),B, DEC L
# POP BC, LD (HL),C, DEC L, LD (HL),B, INC H
# ... repeat pattern ...

operations = [
    # Bytes 0-1
    ('POP', None), ('LD_C', None), ('INC_L', None), ('LD_B', None), ('INC_L', None),
    # Bytes 2-3
    ('POP', None), ('LD_C', None), ('INC_H', None), ('LD_B', None), ('DEC_L', None),
    # Bytes 4-5
    ('POP', None), ('LD_C', None), ('DEC_L', None), ('LD_B', None), ('INC_H', None),
    # Bytes 6-7
    ('POP', None), ('LD_C', None), ('INC_L', None), ('LD_B', None), ('INC_L', None),
    # Bytes 8-9
    ('POP', None), ('LD_C', None), ('INC_H', None), ('LD_B', None), ('DEC_L', None),
    # Bytes 10-11
    ('POP', None), ('LD_C', None), ('DEC_L', None), ('LD_B', None), ('INC_H', None),
    # Bytes 12-13
    ('POP', None), ('LD_C', None), ('INC_L', None), ('LD_B', None), ('INC_L', None),
    # Bytes 14-15
    ('POP', None), ('LD_C', None), ('INC_H', None), ('LD_B', None), ('DEC_L', None),
    # Bytes 16-17
    ('POP', None), ('LD_C', None), ('DEC_L', None), ('LD_B', None), ('INC_H', None),
    # Bytes 18-19
    ('POP', None), ('LD_C', None), ('INC_L', None), ('LD_B', None), ('INC_L', None),
    # Bytes 20-21
    ('POP', None), ('LD_C', None), ('INC_H', None), ('LD_B', None), ('DEC_L', None),
    # Bytes 22-23
    ('POP', None), ('LD_C', None), ('DEC_L', None), ('LD_B', None), ('INC_H', None),
    # Bytes 24-25
    ('POP', None), ('LD_C', None), ('INC_L', None), ('LD_B', None), ('INC_L', None),
    # Bytes 26-27
    ('POP', None), ('LD_C', None), ('INC_H', None), ('LD_B', None), ('DEC_L', None),
    # Bytes 28-29
    ('POP', None), ('LD_C', None), ('DEC_L', None), ('LD_B', None), ('INC_H', None),
    # Bytes 30-31
    ('POP', None), ('LD_C', None), ('INC_L', None), ('LD_B', None), ('INC_L', None),
    # Bytes 32-33
    ('POP', None), ('LD_C', None), ('INC_H', None), ('LD_B', None), ('DEC_L', None),
    # Bytes 34-35
    ('POP', None), ('LD_C', None), ('DEC_L', None), ('LD_B', None), ('INC_H', None),
]

for op, _ in operations:
    if op == 'POP':
        pass  # Just advance byte index
    elif op == 'LD_C':
        write(byte_idx, True, H, L)
        byte_idx += 1
    elif op == 'LD_B':
        write(byte_idx, False, H, L)
        byte_idx += 1
    elif op == 'INC_L':
        L = (L + 1) & 0xFF
    elif op == 'DEC_L':
        L = (L - 1) & 0xFF
    elif op == 'INC_H':
        H = (H + 1) & 0xFF

print("First 36-byte chunk write pattern:")
print(f"{'Byte':<6} {'Reg':<4} {'Addr':<8} {'H':<6} {'L':<6} | Screen Y")
print("-" * 60)

def zx_y_from_addr(addr):
    """Calculate Y coordinate from ZX Spectrum screen address."""
    if addr < 0x4000 or addr >= 0x5800:
        return "N/A"
    offset = addr - 0x4000
    y7y6 = (offset >> 11) & 0x03
    y2y1y0 = (offset >> 8) & 0x07
    y5y4y3 = (offset >> 5) & 0x07
    x_byte = offset & 0x1F
    y = (y7y6 << 6) | (y5y4y3 << 3) | y2y1y0
    return f"y={y:2d} x_byte={x_byte}"

for w in writes[:36]:
    y_info = zx_y_from_addr(w['addr'])
    print(f"{w['byte']:<6} {w['reg']:<4} ${w['addr']:04X}    ${w['h']:02X}    ${w['l']:02X}   | {y_info}")

print(f"\nFinal HL after 36 bytes: ${H:02X}{L:02X}")
print("\nPattern repeats 8 times (DJNZ) with HL advancing through screen")
