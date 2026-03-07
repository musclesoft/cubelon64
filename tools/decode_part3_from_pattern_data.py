#!/usr/bin/env python3
"""
CORRECT decoder for Part 3 patterns from raw pattern data.

The drawing routine writes bytes in a zigzag pattern:
- Row 0: left-to-right (bytes 0-2 at x_bytes 4,5,6)
- Row 1: right-to-left (bytes 3-5 at x_bytes 6,5,4)
- Row 2: left-to-right (bytes 6-8 at x_bytes 4,5,6)
- Row 3: right-to-left (bytes 9-11 at x_bytes 6,5,4)
... and so on

Plus ZX Spectrum screen Y interleaving (y=0-7, then y=64-71, etc.)
"""

import json
import base64
from PIL import Image

# Load the data
with open('../emulator/part3_tiles_900-960.json') as f:
    tiles = json.load(f)

def decode_chunk_36bytes(chunk_bytes):
    """
    Decode one 36-byte chunk into a 6-row × 3-byte grid.
    Handles the zigzag write pattern.
    """
    pixels = []
    byte_idx = 0

    for row in range(6):
        row_pixels = []

        if row % 2 == 0:
            # Even rows: left-to-right (x_bytes 4,5,6 → offsets 0,1,2)
            for x_byte in range(3):
                b = chunk_bytes[byte_idx]
                byte_idx += 1
                for bit in range(7, -1, -1):
                    row_pixels.append((b >> bit) & 1)
        else:
            # Odd rows: right-to-left (x_bytes 6,5,4 → offsets 2,1,0)
            row_bytes = [chunk_bytes[byte_idx + 2],
                        chunk_bytes[byte_idx + 1],
                        chunk_bytes[byte_idx]]
            byte_idx += 3
            for b in row_bytes:
                for bit in range(7, -1, -1):
                    row_pixels.append((b >> bit) & 1)
            # Reverse to get left-to-right pixel order
            row_pixels.reverse()

        pixels.append(row_pixels)

    return pixels

def decode_tile(pattern_bytes_base64, pattern_addrs):
    """
    Decode complete 48×48 tile from 8 chunks of 36 bytes each.
    """
    pattern_bytes = base64.b64decode(pattern_bytes_base64)

    if len(pattern_bytes) != 288:
        print(f"Warning: Expected 288 bytes, got {len(pattern_bytes)}")
        return None

    # Decode 8 chunks
    all_pixels = []
    for chunk_idx in range(8):
        chunk_start = chunk_idx * 36
        chunk_end = chunk_start + 36
        chunk_bytes = pattern_bytes[chunk_start:chunk_end]

        chunk_pixels = decode_chunk_36bytes(chunk_bytes)
        all_pixels.extend(chunk_pixels)

    return all_pixels

# Test with frame 920
frame920 = [t for t in tiles if t['frame'] == 920][0]
print("Decoding frame 920...")
print(f"  Pattern addrs: {[f'${a:04X}' for a in frame920['pattern_addrs']]}")

pixels = decode_tile(frame920['pattern_bytes'], frame920['pattern_addrs'])

if pixels:
    # Save as image
    tile_size = 48
    scale = 4
    img = Image.new('L', (tile_size * scale, tile_size * scale), color=255)

    for y in range(tile_size):
        for x in range(tile_size):
            pixel_val = 0 if pixels[y][x] == 1 else 255
            for sy in range(scale):
                for sx in range(scale):
                    img.putpixel((x * scale + sx, y * scale + sy), pixel_val)

    img.save('frame920_decoded_from_pattern.png')
    print(f"✅ Saved frame920_decoded_from_pattern.png")

    # Show first 8 rows
    print("\nFirst 8 rows:")
    for y in range(8):
        row = ''.join('█' if pixels[y][x] == 1 else '·' for x in range(48))
        print(f'{y}: {row}')
else:
    print("❌ Decoding failed")
