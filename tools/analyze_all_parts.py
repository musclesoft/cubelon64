#!/usr/bin/env python3
"""
Analyze and visualize all grid pattern parts from CUBELON.

Parts 2-5 all use grid-based patterns:
- Part 2 (E1): 24×24 patterns, frames 168-801
- Part 3 (E2): 48×48 patterns, frames 802-1404
- Part 4 (E3): 48×48 patterns, frames 1405-2304
- Part 5 (E4): 48×48 patterns, frames 2305-3999

DEPRECATED: This script uses the 'pixels' field from JSON exports, which has been
removed. Screen extraction is not a valid technique for understanding the algorithm.
The sprite sheets it generated show patterns from screen extraction, not source decoding.
"""

import json
import base64
from PIL import Image, ImageDraw


def pixels_to_image(pixels, scale=1):
    """Convert pixel array to PIL Image."""
    height = len(pixels)
    width = len(pixels[0]) if pixels else 0
    img = Image.new('1', (width, height), 0)
    for y in range(height):
        for x in range(width):
            if pixels[y][x] == 1:
                img.putpixel((x, y), 1)
    if scale > 1:
        img = img.resize((width * scale, height * scale), Image.NEAREST)
    return img


def create_sprite_sheet(patterns, output_file, tile_size, scale, cols=8):
    """Create sprite sheet from patterns."""
    unique_patterns = {}
    for tile in patterns:
        addrs_tuple = tuple(tile['pattern_addrs']) if 'pattern_addrs' in tile else tuple()
        if addrs_tuple not in unique_patterns:
            unique_patterns[addrs_tuple] = tile

    rows = (len(unique_patterns) + cols - 1) // cols
    tile_width = tile_size * scale
    tile_height = tile_size * scale
    margin = 10
    label_height = 20

    sheet_width = cols * (tile_width + margin) + margin
    sheet_height = rows * (tile_height + label_height + margin) + margin

    sprite_sheet = Image.new('RGB', (sheet_width, sheet_height), (40, 40, 40))
    draw = ImageDraw.Draw(sprite_sheet)

    for idx, (addrs, tile) in enumerate(sorted(unique_patterns.items(),
                                                 key=lambda x: x[1]['frame'])):
        col = idx % cols
        row = idx // cols

        x = margin + col * (tile_width + margin)
        y = margin + row * (tile_height + label_height + margin)

        img = pixels_to_image(tile['pixels'], scale=scale)
        sprite_sheet.paste(img, (x, y))

        label = f"F{tile['frame']}"
        draw.text((x, y + tile_height + 2), label, fill=(200, 200, 200))

    sprite_sheet.save(output_file)
    return len(unique_patterns), len(patterns)


def main():
    print("=" * 80)
    print("CUBELON: Complete Grid Pattern Analysis")
    print("=" * 80)
    print()

    parts = [
        {
            'name': 'Part 2 (E1): Square Emergence',
            'file': '../emulator/animation_frames_168-801.json',
            'frames': '168-801',
            'tile_size': 24,
            'scale': 6,
            'output': 'part2_complete_all_patterns.png'
        },
        {
            'name': 'Part 3 (E2): Geometric Pattern',
            'file': '../emulator/part3_tiles_802-1404.json',
            'frames': '802-1404',
            'tile_size': 48,
            'scale': 3,
            'output': 'part3_complete_all_patterns.png'
        },
        {
            'name': 'Part 4 (E3): Grid Pulse/Rotation',
            'file': '../emulator/part3_tiles_1405-2304.json',
            'frames': '1405-2304',
            'tile_size': 48,
            'scale': 3,
            'output': 'part4_complete_all_patterns.png'
        },
        {
            'name': 'Part 5 (E4): Pattern Cycling',
            'file': '../emulator/part3_tiles_2305-3999.json',
            'frames': '2305-3999',
            'tile_size': 48,
            'scale': 3,
            'output': 'part5_complete_all_patterns.png'
        }
    ]

    total_unique = 0
    total_frames = 0

    for part in parts:
        print(f"\n📦 {part['name']}")
        print(f"   Frames: {part['frames']}")

        try:
            with open(part['file']) as f:
                data = json.load(f)

            unique, frames = create_sprite_sheet(
                data,
                part['output'],
                part['tile_size'],
                part['scale']
            )

            print(f"   ✅ Total frames: {frames}")
            print(f"   ✅ Unique patterns: {unique}")
            print(f"   ✅ Repetition: {frames/unique:.1f}x")
            print(f"   ✅ Saved: {part['output']}")

            total_unique += unique
            total_frames += frames

        except FileNotFoundError:
            print(f"   ⚠️  File not found: {part['file']}")

    print()
    print("=" * 80)
    print(f"✅ COMPLETE: {total_unique} unique patterns from {total_frames} frames")
    print("=" * 80)
    print()
    print("Pattern Distribution:")
    print("  • Part 2 (24×24): Small emergence patterns")
    print("  • Part 3 (48×48): Complex geometric morphing")
    print("  • Part 4 (48×48): Pulse and rotation effects")
    print("  • Part 5 (48×48): Extended pattern cycling")
    print()


if __name__ == '__main__':
    main()
