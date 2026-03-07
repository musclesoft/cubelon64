#!/usr/bin/env python3
"""
Simplest possible decoder - just read bytes in order and see what we get.
"""

import json
import base64
from PIL import Image

with open('../emulator/part3_tiles_900-960.json') as f:
    tiles = json.load(f)

frame920 = [t for t in tiles if t['frame'] == 920][0]
pattern_bytes = base64.b64decode(frame920['pattern_bytes'])

print(f"Pattern bytes length: {len(pattern_bytes)}")
print(f"First 36 bytes (chunk 0, addr ${frame920['pattern_addrs'][0]:04X}):")
for i in range(0, 36, 6):
    print(f"  {i:2d}: {' '.join(f'{b:02X}' for b in pattern_bytes[i:i+6])}")

# Simple decode: 288 bytes = 48 rows × 6 bytes/row
pixels = [[0]*48 for _ in range(48)]
for row in range(48):
    for byte_idx in range(6):
        b = pattern_bytes[row * 6 + byte_idx]
        for bit in range(8):
            col = byte_idx * 8 + bit
            pixels[row][col] = (b >> (7 - bit)) & 1

# Save
scale = 4
img = Image.new('L', (48 * scale, 48 * scale), color=255)
for y in range(48):
    for x in range(48):
        pixel_val = 0 if pixels[y][x] == 1 else 255
        for sy in range(scale):
            for sx in range(scale):
                img.putpixel((x * scale + sx, y * scale + sy), pixel_val)

img.save('frame920_simple_decode.png')
print(f"\n✅ Saved frame920_simple_decode.png (simple row-major)")

# Show first 12 rows
print("\nFirst 12 rows:")
for y in range(12):
    row = ''.join('█' if pixels[y][x] == 1 else '·' for x in range(48))
    print(f'{y:2d}: {row}')
