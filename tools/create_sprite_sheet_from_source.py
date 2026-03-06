#!/usr/bin/env python3
"""
Create sprite sheet from source-extracted patterns.
Uses animation_frames_319-370.json extracted directly from ZX Spectrum memory.
"""

import json
import base64
from PIL import Image

def decode_pattern(pattern_bytes):
    """Decode 72-byte pattern into 24×24 pixel grid.

    The ZX Spectrum drawing routine uses an interleaved byte order:
    - Even rows (0,2,4...): read bytes in order [b0, b1, b2]
    - Odd rows (1,3,5...): read bytes in reverse [b2, b1, b0]

    This is an optimization for the Z80 POP-based drawing routine.

    Pattern structure (72 bytes = 24 rows × 3 bytes):
    - Bytes 0-17: Top padding (6 rows)
    - Bytes 18-53: Visible area (12 rows)
    - Bytes 54-71: Bottom padding (6 rows)
    """
    pixels = []

    # Decode all 24 rows (72 bytes / 3 bytes per row)
    for row in range(24):
        offset = row * 3

        # Odd rows read bytes in reverse order
        if row % 2 == 0:
            b0 = pattern_bytes[offset]
            b1 = pattern_bytes[offset + 1]
            b2 = pattern_bytes[offset + 2]
        else:
            b0 = pattern_bytes[offset + 2]
            b1 = pattern_bytes[offset + 1]
            b2 = pattern_bytes[offset]

        # Decode 3 bytes into 24 pixels
        row_pixels = []
        for byte_val in [b0, b1, b2]:
            for bit in range(7, -1, -1):
                row_pixels.append(1 if (byte_val >> bit) & 1 else 0)

        pixels.append(row_pixels)

    return pixels

def create_sprite_sheet():
    # Load animation data from source
    with open('animation_frames_319-370.json', 'r') as f:
        frames = json.load(f)

    print(f'Loaded {len(frames)} frames from source')

    # Frame dimensions (24×24 pixels - full pattern data)
    frame_width = 24
    frame_height = 24

    # Grid layout: 13 columns × 4 rows = 52 frames
    cols = 13
    rows = 4
    scale = 4  # 4x scale for visibility
    padding = 2 * scale  # scaled padding

    # Calculate sprite sheet dimensions
    sheet_width = (frame_width * scale + padding) * cols - padding
    sheet_height = (frame_height * scale + padding) * rows - padding

    # Create sprite sheet (RGB for better visibility)
    sprite_sheet = Image.new('RGB', (sheet_width, sheet_height), (0, 0, 0))

    # Draw each frame
    for idx, frame in enumerate(frames):
        col = idx % cols
        row = idx // cols

        x_offset = col * (frame_width * scale + padding)
        y_offset = row * (frame_height * scale + padding)

        # Decode pattern from source bytes
        pattern_bytes = base64.b64decode(frame['pattern_bytes'])
        pixels = decode_pattern(pattern_bytes)

        # Draw pixels (scaled)
        for y, row_pixels in enumerate(pixels):
            for x, pixel in enumerate(row_pixels):
                if pixel:
                    # Draw a scaled pixel block
                    for sy in range(scale):
                        for sx in range(scale):
                            sprite_sheet.putpixel(
                                (x_offset + x * scale + sx, y_offset + y * scale + sy),
                                (255, 255, 255)
                            )

    # Save as PNG
    sprite_sheet.save('animation_sprite_sheet_source.png')
    print(f"✅ Created sprite sheet: {sheet_width}×{sheet_height} pixels")
    print(f"   52 frames in 13×4 grid, each frame {frame_width}×{frame_height} pixels at {scale}x scale")

if __name__ == '__main__':
    create_sprite_sheet()
