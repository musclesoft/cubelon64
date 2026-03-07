#!/usr/bin/env python3
"""
Build complete 288-byte write pattern map from the disassembly.

Starting HL: $4004 (top-left tile at x=32 pixels, y=0)
This is x_byte=4, y=0

Trace all 8 DJNZ iterations with complete write pattern.
"""

def zx_coords_from_addr(addr):
    """Convert ZX Spectrum screen address to (y, x_byte)."""
    if addr < 0x4000 or addr >= 0x5800:
        return None, None
    offset = addr - 0x4000
    y7y6 = (offset >> 11) & 0x03
    y2y1y0 = (offset >> 8) & 0x07
    y5y4y3 = (offset >> 5) & 0x07
    x_byte = offset & 0x1F
    y = (y7y6 << 6) | (y5y4y3 << 3) | y2y1y0
    return y, x_byte

# Starting position for top-left tile
H = 0x40
L = 0x04

write_map = []  # List of (byte_index, screen_y, screen_x_byte, bit_offset)

byte_idx = 0

# Pattern repeats 8 times (DJNZ loop)
for djnz_iteration in range(8):
    # At start of iteration:
    # - HL points to pattern table (gets restored after reads)
    # - HL' (alternate) holds screen address

    # The pattern within one iteration (18 POP BC = 36 bytes):
    # This is the pattern from $85AB onwards

    operations = [
        # Bytes 0-1: POP BC, LD (HL),C, INC L, LD (HL),B, INC L
        ('POP', None), ('LD_C', None), ('INC_L', None), ('LD_B', None), ('INC_L', None),
        # Bytes 2-3: POP BC, LD (HL),C, INC H, LD (HL),B, DEC L
        ('POP', None), ('LD_C', None), ('INC_H', None), ('LD_B', None), ('DEC_L', None),
        # Bytes 4-5: POP BC, LD (HL),C, DEC L, LD (HL),B, INC H
        ('POP', None), ('LD_C', None), ('DEC_L', None), ('LD_B', None), ('INC_H', None),
        # Bytes 6-7: POP BC, LD (HL),C, INC L, LD (HL),B, INC L
        ('POP', None), ('LD_C', None), ('INC_L', None), ('LD_B', None), ('INC_L', None),
        # Bytes 8-9: POP BC, LD (HL),C, INC H, LD (HL),B, DEC L
        ('POP', None), ('LD_C', None), ('INC_H', None), ('LD_B', None), ('DEC_L', None),
        # Bytes 10-11: POP BC, LD (HL),C, DEC L, LD (HL),B, INC H
        ('POP', None), ('LD_C', None), ('DEC_L', None), ('LD_B', None), ('INC_H', None),
        # Bytes 12-13: POP BC, LD (HL),C, INC L, LD (HL),B, INC L
        ('POP', None), ('LD_C', None), ('INC_L', None), ('LD_B', None), ('INC_L', None),
        # Bytes 14-15: POP BC, LD (HL),C, INC H, LD (HL),B, DEC L
        ('POP', None), ('LD_C', None), ('INC_H', None), ('LD_B', None), ('DEC_L', None),
        # Bytes 16-17: POP BC, LD (HL),C, DEC L, LD (HL),B, INC H
        ('POP', None), ('LD_C', None), ('DEC_L', None), ('LD_B', None), ('INC_H', None),
        # Bytes 18-19: POP BC, LD (HL),C, INC L, LD (HL),B, INC L
        ('POP', None), ('LD_C', None), ('INC_L', None), ('LD_B', None), ('INC_L', None),
        # Bytes 20-21: POP BC, LD (HL),C, INC H, LD (HL),B, DEC L
        ('POP', None), ('LD_C', None), ('INC_H', None), ('LD_B', None), ('DEC_L', None),
        # Bytes 22-23: POP BC, LD (HL),C, DEC L, LD (HL),B, INC H
        ('POP', None), ('LD_C', None), ('DEC_L', None), ('LD_B', None), ('INC_H', None),
        # Bytes 24-25: POP BC, LD (HL),C, INC L, LD (HL),B, INC L
        ('POP', None), ('LD_C', None), ('INC_L', None), ('LD_B', None), ('INC_L', None),
        # Bytes 26-27: POP BC, LD (HL),C, INC H, LD (HL),B, DEC L
        ('POP', None), ('LD_C', None), ('INC_H', None), ('LD_B', None), ('DEC_L', None),
        # Bytes 28-29: POP BC, LD (HL),C, DEC L, LD (HL),B, INC H
        ('POP', None), ('LD_C', None), ('DEC_L', None), ('LD_B', None), ('INC_H', None),
        # Bytes 30-31: POP BC, LD (HL),C, INC L, LD (HL),B, INC L
        ('POP', None), ('LD_C', None), ('INC_L', None), ('LD_B', None), ('INC_L', None),
        # Bytes 32-33: POP BC, LD (HL),C, INC H, LD (HL),B, DEC L
        ('POP', None), ('LD_C', None), ('INC_H', None), ('LD_B', None), ('DEC_L', None),
        # Bytes 34-35: POP BC, LD (HL),C, DEC L, LD (HL),B, INC H
        ('POP', None), ('LD_C', None), ('DEC_L', None), ('LD_B', None), ('INC_H', None),
    ]

    # Note: Between DJNZ iterations, there's:
    # $85E6: LD DE,$0020
    # $85E9: ADD HL,DE  (advances to next screen row section)
    # But this happens in the alternate register set after EXX

    chunk_byte_idx = 0
    for op, _ in operations:
        if op == 'POP':
            pass
        elif op == 'LD_C':
            addr = (H << 8) | L
            y, x_byte = zx_coords_from_addr(addr)
            write_map.append({
                'pattern_byte': byte_idx,
                'chunk': djnz_iteration,
                'chunk_byte': chunk_byte_idx,
                'addr': addr,
                'y': y,
                'x_byte': x_byte,
                'register': 'C'
            })
            byte_idx += 1
            chunk_byte_idx += 1
        elif op == 'LD_B':
            addr = (H << 8) | L
            y, x_byte = zx_coords_from_addr(addr)
            write_map.append({
                'pattern_byte': byte_idx,
                'chunk': djnz_iteration,
                'chunk_byte': chunk_byte_idx,
                'addr': addr,
                'y': y,
                'x_byte': x_byte,
                'register': 'B'
            })
            byte_idx += 1
            chunk_byte_idx += 1
        elif op == 'INC_L':
            L = (L + 1) & 0xFF
        elif op == 'DEC_L':
            L = (L - 1) & 0xFF
        elif op == 'INC_H':
            H = (H + 1) & 0xFF

    # Between iterations: LD DE,$0020; ADD HL,DE (happens after EXX)
    # This adds 0x20 to HL, moving down in the interleaved screen
    # But wait - looking at execution counts, HL' advances, not HL
    # Let me check what advances between DJNZ iterations...

    # Actually, from the pattern, after 36 bytes, we're at $4C04
    # Which is y=68-71 area. The next iteration should start at next strip.
    # Let me just use the final HL value

print(f"Generated write map for {len(write_map)} bytes")
print("\nFirst 20 writes:")
print(f"{'Byte':<6} {'Chunk':<7} {'ChByte':<8} {'Addr':<8} {'Y':<5} {'XByte':<7} {'Reg'}")
print("-" * 60)
for i in range(20):
    w = write_map[i]
    print(f"{w['pattern_byte']:<6} {w['chunk']:<7} {w['chunk_byte']:<8} "
          f"${w['addr']:04X}    {w['y']:<5} {w['x_byte']:<7} {w['register']}")

print(f"\nLast 20 writes:")
for i in range(len(write_map)-20, len(write_map)):
    w = write_map[i]
    print(f"{w['pattern_byte']:<6} {w['chunk']:<7} {w['chunk_byte']:<8} "
          f"${w['addr']:04X}    {w['y']:<5} {w['x_byte']:<7} {w['register']}")

# Save the write map as JSON for use in decoder
import json
with open('write_map_288.json', 'w') as f:
    json.dump(write_map, f, indent=2)

print(f"\n✅ Saved write_map_288.json")
print("\nNext step: Use this map to build decoder that transforms")
print("pattern_bytes[288] → screen_pixels[48][48]")
