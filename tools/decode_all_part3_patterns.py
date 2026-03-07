#!/usr/bin/env python3
"""
Decode and visualize all 12 unique Part 3 patterns (48×48 pixels each).
"""

import json
from PIL import Image
import sys

# Load the pattern data
with open('../emulator/part3_tiles_900-960.json') as f:
    tiles = json.load(f)

# Extract unique patterns
unique_patterns = {}
for tile in tiles:
    addr = tile['pattern_addr']
    if addr not in unique_patterns:
        unique_patterns[addr] = {
            'frame': tile['frame'],
            'pixels': tile['pixels']
        }

# Sort by address
sorted_addrs = sorted(unique_patterns.keys())

print(f"Found {len(sorted_addrs)} unique patterns:")
for addr in sorted_addrs:
    frame = unique_patterns[addr]['frame']
    print(f"  ${addr:04X} (first seen frame {frame})")

# Create sprite sheet: 4 columns × 3 rows
cols = 4
rows = (len(sorted_addrs) + cols - 1) // cols
tile_size = 48
scale = 4
scaled_size = tile_size * scale

img_width = cols * scaled_size
img_height = rows * scaled_size

img = Image.new('L', (img_width, img_height), color=255)

for idx, addr in enumerate(sorted_addrs):
    col = idx % cols
    row = idx // cols

    x_offset = col * scaled_size
    y_offset = row * scaled_size

    pixels = unique_patterns[addr]['pixels']

    # Draw scaled tile
    for y in range(tile_size):
        for x in range(tile_size):
            pixel_val = 0 if pixels[y][x] == 1 else 255
            for sy in range(scale):
                for sx in range(scale):
                    img.putpixel(
                        (x_offset + x * scale + sx, y_offset + y * scale + sy),
                        pixel_val
                    )

output_file = 'part3_all_12_patterns.png'
img.save(output_file)
print(f"\n✅ Saved sprite sheet to {output_file}")
print(f"   Size: {img_width}×{img_height} ({cols}×{rows} grid, {scaled_size}×{scaled_size} per tile)")

# Also save individual patterns
for idx, addr in enumerate(sorted_addrs):
    frame = unique_patterns[addr]['frame']
    pixels = unique_patterns[addr]['pixels']

    tile_img = Image.new('L', (scaled_size, scaled_size), color=255)

    for y in range(tile_size):
        for x in range(tile_size):
            pixel_val = 0 if pixels[y][x] == 1 else 255
            for sy in range(scale):
                for sx in range(scale):
                    tile_img.putpixel((x * scale + sx, y * scale + sy), pixel_val)

    filename = f'part3_pattern_{addr:04X}_frame{frame}.png'
    tile_img.save(filename)
    print(f"   Saved ${addr:04X} → {filename}")

print("\n✅ All patterns decoded and saved!")
