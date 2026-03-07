#!/usr/bin/env python3
"""
Hypothesis: The 8 pattern addresses alternate between LEFT and RIGHT halves.
- Even entries (0,2,4,6): LEFT 24 pixels
- Odd entries (1,3,5,7): RIGHT 24 pixels

Each chunk is 36 bytes = 6 rows × 3 bytes = 6 rows × 24 pixels
8 chunks = 8 × 6 rows = 48 rows

So the structure is:
- Chunk 0 (left): rows 0-5, columns 0-23
- Chunk 1 (right): rows 0-5, columns 24-47
- Chunk 2 (left): rows 6-11, columns 0-23
- Chunk 3 (right): rows 6-11, columns 24-47
... etc
"""

import json
import base64
from PIL import Image

# Load the data
with open('../emulator/part3_tiles_900-960.json') as f:
    tiles = json.load(f)

def decode_chunk_zigzag(chunk_bytes):
    """
    Decode 36 bytes with zigzag pattern into 6 rows × 24 pixels.
    """
    pixels = []
    byte_idx = 0

    for row in range(6):
        row_pixels = []

        if row % 2 == 0:
            # Even rows: left-to-right
            for _ in range(3):
                b = chunk_bytes[byte_idx]
                byte_idx += 1
                for bit in range(7, -1, -1):
                    row_pixels.append((b >> bit) & 1)
        else:
            # Odd rows: right-to-left (need to reverse)
            bytes_for_row = []
            for _ in range(3):
                bytes_for_row.append(chunk_bytes[byte_idx])
                byte_idx += 1
            # Reverse byte order
            bytes_for_row.reverse()
            for b in bytes_for_row:
                for bit in range(7, -1, -1):
                    row_pixels.append((b >> bit) & 1)
            row_pixels.reverse()  # Reverse pixel order too

        pixels.append(row_pixels)

    return pixels

def decode_tile_left_right(pattern_bytes_base64):
    """
    Decode 48×48 tile assuming left/right alternation.
    """
    pattern_bytes = base64.b64decode(pattern_bytes_base64)
    if len(pattern_bytes) != 288:
        return None

    full_tile = [[0]*48 for _ in range(48)]

    for chunk_idx in range(8):
        chunk_start = chunk_idx * 36
        chunk_end = chunk_start + 36
        chunk_bytes = pattern_bytes[chunk_start:chunk_end]

        chunk_pixels = decode_chunk_zigzag(chunk_bytes)

        # Determine which 6-row strip this is
        strip_idx = chunk_idx // 2  # 0,0,1,1,2,2,3,3 -> 0,1,2,3
        is_right_half = (chunk_idx % 2 == 1)

        row_offset = strip_idx * 6

        for local_row in range(6):
            global_row = row_offset + local_row
            col_offset = 24 if is_right_half else 0

            for local_col in range(24):
                global_col = col_offset + local_col
                full_tile[global_row][global_col] = chunk_pixels[local_row][local_col]

    return full_tile

# Test with frame 920
frame920 = [t for t in tiles if t['frame'] == 920][0]
print("Decoding frame 920 with left/right hypothesis...")
print(f"  Pattern addrs: {[f'${a:04X}' for a in frame920['pattern_addrs']]}")

pixels = decode_tile_left_right(frame920['pattern_bytes'])

if pixels:
    # Save as image
    scale = 4
    img = Image.new('L', (48 * scale, 48 * scale), color=255)

    for y in range(48):
        for x in range(48):
            pixel_val = 0 if pixels[y][x] == 1 else 255
            for sy in range(scale):
                for sx in range(scale):
                    img.putpixel((x * scale + sx, y * scale + sy), pixel_val)

    img.save('frame920_left_right_decode.png')
    print(f"✅ Saved frame920_left_right_decode.png")

    # Show first 8 rows
    print("\nFirst 8 rows:")
    for y in range(8):
        row = ''.join('█' if pixels[y][x] == 1 else '·' for x in range(48))
        print(f'{y}: {row}')
else:
    print("❌ Decoding failed")
