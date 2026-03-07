#!/usr/bin/env python3
"""
FINAL CORRECT Part 3 pattern decoder.

Now that we have pattern_addrs in the JSON (all 8 addresses),
we can decode the patterns correctly by using the actual data
as assembled by the drawing routine.

The pixels array in the JSON is already correctly decoded from
the assembled 288 bytes, so we just need to visualize it!
"""

import json
from PIL import Image

# Load the pattern data
with open('../emulator/part3_tiles_900-960.json') as f:
    tiles = json.load(f)

# Extract unique pattern combinations (based on pattern_addrs signature)
unique_patterns = {}
for tile in tiles:
    # Use the pattern_addrs as a key (convert to tuple for hashing)
    addrs = tuple(tile.get('pattern_addrs', [tile['pattern_addr']]))
    if addrs not in unique_patterns:
        unique_patterns[addrs] = {
            'frame': tile['frame'],
            'pixels': tile['pixels'],
            'pattern_addr': tile['pattern_addr'],
            'pattern_addrs': tile.get('pattern_addrs', [])
        }

# Sort by first address
sorted_keys = sorted(unique_patterns.keys(), key=lambda k: k[0] if k else 0)

print(f"Found {len(sorted_keys)} unique pattern combinations:")
for addrs in sorted_keys:
    frame = unique_patterns[addrs]['frame']
    if len(addrs) > 1:
        print(f"  Frame {frame}: {', '.join(f'${a:04X}' for a in addrs[:3])}{'...' if len(addrs) > 3 else ''}")
    else:
        print(f"  Frame {frame}: ${addrs[0]:04X}")

# Create sprite sheet: 4 columns × N rows
cols = 4
rows = (len(sorted_keys) + cols - 1) // cols
tile_size = 48
scale = 4
scaled_size = tile_size * scale

img_width = cols * scaled_size
img_height = rows * scaled_size

img = Image.new('L', (img_width, img_height), color=255)

for idx, addrs in enumerate(sorted_keys):
    col = idx % cols
    row = idx // cols

    x_offset = col * scaled_size
    y_offset = row * scaled_size

    pixels = unique_patterns[addrs]['pixels']

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

output_file = 'part3_all_patterns_CORRECT.png'
img.save(output_file)
print(f"\n✅ Saved sprite sheet to {output_file}")
print(f"   Size: {img_width}×{img_height} ({cols}×{rows} grid, {scaled_size}×{scaled_size} per tile)")

# Also save individual patterns
for idx, addrs in enumerate(sorted_keys):
    frame = unique_patterns[addrs]['frame']
    pixels = unique_patterns[addrs]['pixels']

    tile_img = Image.new('L', (scaled_size, scaled_size), color=255)

    for y in range(tile_size):
        for x in range(tile_size):
            pixel_val = 0 if pixels[y][x] == 1 else 255
            for sy in range(scale):
                for sx in range(scale):
                    tile_img.putpixel((x * scale + sx, y * scale + sy), pixel_val)

    # Create safe filename
    if len(addrs) > 1:
        filename = f'part3_tile_frame{frame}_{addrs[0]:04X}_multi.png'
    else:
        filename = f'part3_tile_frame{frame}_{addrs[0]:04X}.png'

    tile_img.save(filename)
    print(f"   Saved frame {frame} → {filename}")

print("\n✅ All patterns decoded and saved!")
