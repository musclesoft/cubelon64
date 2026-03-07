#!/usr/bin/env python3
"""
Analyze the Part 3 drawing routine to understand the byte layout.

From the disassembly at $859D:
- LD B,$08 (loop 8 times)
- Each iteration: read pattern address from table (INC L twice = next entry)
- Each iteration: 18 × POP BC = 36 bytes read from pattern
- Total: 8 × 36 = 288 bytes

Key observation: Pattern addresses are $48 (72) bytes apart:
  $C946 → $C8FE → $C8B6 (each -$48)

72 bytes is exactly 2 × 36 bytes. So patterns might be stored in a
different layout than we're assuming!

Let's trace the write pattern:
POP BC, LD (HL),C, INC L, LD (HL),B, INC L  (bytes 0-1, horiz)
POP BC, LD (HL),C, INC H, LD (HL),B, DEC L  (bytes 2-3, INC H = next scan line)
POP BC, LD (HL),C, DEC L, LD (HL),B, INC H  (bytes 4-5)
... (18 POP BC total = 36 bytes)

INC H moves through ZX Spectrum's interleaved screen lines!
"""

# ZX Spectrum screen layout:
# Address bits: 010 Y7Y6 Y2Y1Y0 Y5Y4Y3 XXXXX
# Y7Y6 = character row (0-2, each 64 lines)
# Y5Y4Y3 = scan line within 8-line block
# Y2Y1Y0 = which 8-line block within character row

def zx_screen_addr(x, y):
    """Calculate ZX Spectrum screen address for pixel at (x, y)."""
    # y bits: Y7 Y6 Y5 Y4 Y3 Y2 Y1 Y0
    # addr bits: 010 Y7Y6 Y2Y1Y0 Y5Y4Y3 XXXXX
    y7y6 = (y >> 6) & 0x03  # Top 2 bits (character row)
    y5y4y3 = (y >> 3) & 0x07  # Middle 3 bits (scan line in block)
    y2y1y0 = y & 0x07  # Bottom 3 bits (which block)
    x_byte = x // 8

    addr = 0x4000
    addr |= (y7y6 << 11)  # Bits 12-11
    addr |= (y2y1y0 << 8)  # Bits 10-8
    addr |= (y5y4y3 << 5)  # Bits 7-5
    addr |= x_byte  # Bits 4-0

    return addr

# Test: trace what the INC H pattern does
print("INC H navigation pattern:")
print("Starting at $4000 (x=0, y=0):")
addr = 0x4000
for i in range(10):
    y = None
    x = None
    # Reverse calculate y from address
    y7y6 = (addr >> 11) & 0x03
    y2y1y0 = (addr >> 8) & 0x07
    y5y4y3 = (addr >> 5) & 0x07
    y = (y7y6 << 6) | (y5y4y3 << 3) | y2y1y0
    x = (addr & 0x1F) * 8
    print(f"  ${addr:04X} → y={y:3d} (Y7Y6={y7y6:02b} Y5Y4Y3={y5y4y3:03b} Y2Y1Y0={y2y1y0:03b})")
    addr += 0x0100  # INC H

print("\nPattern addresses are spaced $48 (72) bytes apart")
print("72 bytes = 12 rows × 6 bytes/row")
print("So each pattern might be 12 rows of a 48-pixel-wide strip!")
print("\nIf the tile is 48×48 pixels:")
print("  48 rows ÷ 12 rows per pattern = 4 patterns needed")
print("  But DJNZ loops 8 times, reading 8 pattern addresses...")
print("\nAlternatively, with 8 iterations × 36 bytes:")
print("  36 bytes = 6 rows × 6 bytes/row")
print("  8 iterations × 6 rows = 48 rows ✓")
print("\nSo each DJNZ iteration draws 6 rows!")
