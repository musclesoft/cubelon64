#!/usr/bin/env python3
"""
Decode Part 3 patterns using the actual drawing routine's write pattern.

The drawing routine at $859D-$866D writes bytes in a complex pattern:
- 8 DJNZ iterations
- Each iteration: 18 POP BC (36 bytes), written with INC H, DEC L navigation
- Total: 288 bytes for 48×48 tile

From disassembly (docs/part3_complete_data.md lines 68-126):
- POP BC loads 2 bytes
- Writes C to (HL), B to (HL+1) or (HL-1) depending on position
- Uses INC H to move to next screen line (interleaved layout)
- Uses INC L, DEC L to navigate horizontally

The key is the ZX Spectrum screen layout:
- Screen lines are interleaved in 3 groups of 64 lines each
- Within each group, lines are further interleaved in 8-line blocks
"""

import json
import base64
from PIL import Image

def decode_part3_pattern(pattern_bytes):
    """
    Decode 288 bytes into 48×48 pixels using the actual drawing routine's pattern.

    The drawing routine processes the pattern in 8 DJNZ iterations,
    each handling 36 bytes (18 POP BC).
    """
    pixels = [[0]*48 for _ in range(48)]

    # Simple approach: the pattern is 6 bytes per row (48 pixels / 8 bits)
    # Let's first try standard row-major with bit order
    byte_idx = 0
    for row in range(48):
        for col_byte in range(6):  # 6 bytes per row
            if byte_idx < len(pattern_bytes):
                byte_val = pattern_bytes[byte_idx]
                # MSB first (standard)
                for bit in range(8):
                    col = col_byte * 8 + bit
                    if col < 48:
                        pixels[row][col] = (byte_val >> (7 - bit)) & 1
                byte_idx += 1

    return pixels

# Load the patterns
with open('../emulator/part3_tiles_900-960.json') as f:
    tiles = json.load(f)

# Test with frame 920
frame920 = [t for t in tiles if t['frame'] == 920][0]
pattern_bytes = base64.b64decode(frame920['pattern_bytes'])

print("Testing different decoding approaches...")
print(f"Pattern addr: ${frame920['pattern_addr']:04X}")
print(f"Pattern size: {len(pattern_bytes)} bytes")
print()

# Decode and save
pixels = decode_part3_pattern(pattern_bytes)

# Save as image
scale = 4
size = 48
img = Image.new('L', (size*scale, size*scale), color=255)
for y in range(size):
    for x in range(size):
        pixel_val = 0 if pixels[y][x] == 1 else 255
        for sy in range(scale):
            for sx in range(scale):
                img.putpixel((x*scale + sx, y*scale + sy), pixel_val)

img.save('frame920_decode_test.png')
print("✅ Saved frame920_decode_test.png")

# Show first few rows
print("\nFirst 8 rows:")
for y in range(8):
    row = ''.join('█' if pixels[y][x] == 1 else '·' for x in range(48))
    print(f'{y:2d}: {row}')
