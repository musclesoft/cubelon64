#!/usr/bin/env python3
"""
ZX Spectrum Screen Viewer
Converts ZX Spectrum screen data to PNG images
"""

import sys
from PIL import Image

# ZX Spectrum colors (RGB)
ZX_COLORS = [
    (0, 0, 0),       # 0: Black
    (0, 0, 215),     # 1: Blue
    (215, 0, 0),     # 2: Red
    (215, 0, 215),   # 3: Magenta
    (0, 215, 0),     # 4: Green
    (0, 215, 215),   # 5: Cyan
    (215, 215, 0),   # 6: Yellow
    (215, 215, 215), # 7: White
]

# Bright variants
ZX_COLORS_BRIGHT = [
    (0, 0, 0),       # 0: Black (same)
    (0, 0, 255),     # 1: Bright Blue
    (255, 0, 0),     # 2: Bright Red
    (255, 0, 255),   # 3: Bright Magenta
    (0, 255, 0),     # 4: Bright Green
    (0, 255, 255),   # 5: Bright Cyan
    (255, 255, 0),   # 6: Bright Yellow
    (255, 255, 255), # 7: Bright White
]

def extract_screen(data, offset=0):
    """Extract a ZX Spectrum screen from data"""
    if len(data) < offset + 6912:
        print(f"Warning: Data too small for full screen (need 6912 bytes, have {len(data)-offset})")
        data = data + b'\x00' * (6912 - (len(data) - offset))

    bitmap_data = data[offset:offset+6144]
    attr_data = data[offset+6144:offset+6912]

    img = Image.new('RGB', (256, 192))
    pixels = img.load()

    for y in range(192):
        section = y // 64
        row = (y % 64) // 8
        line = y % 8
        line_offset = section * 2048 + line * 256 + row * 32
        attr_offset = (y // 8) * 32

        for x in range(32):
            byte_val = bitmap_data[line_offset + x]
            attr_val = attr_data[attr_offset + x]
            ink = attr_val & 0x07
            paper = (attr_val >> 3) & 0x07
            bright = (attr_val >> 6) & 0x01
            colors = ZX_COLORS_BRIGHT if bright else ZX_COLORS
            ink_color = colors[ink]
            paper_color = colors[paper]

            for bit in range(8):
                pixel_on = (byte_val >> (7 - bit)) & 1
                color = ink_color if pixel_on else paper_color
                pixels[x * 8 + bit, y] = color

    return img

if __name__ == '__main__':
    if len(sys.argv) < 2:
        print("Usage: view_zx_screen.py <input_file> [output_png] [offset]")
        sys.exit(1)

    input_file = sys.argv[1]
    output_file = sys.argv[2] if len(sys.argv) > 2 else input_file.replace('.bin', '.png')
    offset = int(sys.argv[3]) if len(sys.argv) > 3 else 0

    with open(input_file, 'rb') as f:
        data = f.read()

    img = extract_screen(data, offset)
    img.save(output_file)
    print(f"Saved: {output_file}")

    img_2x = img.resize((512, 384), Image.NEAREST)
    img_2x.save(output_file.replace('.png', '_2x.png'))
    print(f"Saved 2x: {output_file.replace('.png', '_2x.png')}")
