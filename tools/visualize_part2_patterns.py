#!/usr/bin/env python3
"""
Visualize Part 2 (24×24) patterns from animation_frames.json.

This complements the Part 3 visualization and shows the smaller patterns
used in the square emergence section (E1, frames 168-801).
"""

import json
import base64
from PIL import Image, ImageDraw


def pixels_to_image(pixels, scale=4):
    """Convert pixel array to PIL Image."""
    height = len(pixels)
    width = len(pixels[0]) if pixels else 0

    img = Image.new('1', (width, height), 0)
    for y in range(height):
        for x in range(width):
            if pixels[y][x] == 1:
                img.putpixel((x, y), 1)

    # Scale up
    if scale > 1:
        img = img.resize((width * scale, height * scale), Image.NEAREST)

    return img


def main():
    print("=== Part 2: Pattern Visualization (24×24) ===")
    print()

    # Load patterns
    with open('../emulator/animation_frames_319-370.json') as f:
        data = json.load(f)

    print(f"✅ Loaded {len(data)} patterns from frames 319-370")

    # Check pattern byte size
    if data:
        first_pattern_bytes = base64.b64decode(data[0]['pattern_bytes'])
        print(f"✅ Pattern byte size: {len(first_pattern_bytes)} bytes")
        print(f"   24×24 pixels = 72 bytes (24 rows × 3 bytes/row)")
    print()

    # Find unique patterns
    unique_patterns = {}
    for pattern in data:
        pattern_bytes = base64.b64decode(pattern['pattern_bytes'])
        key = pattern_bytes  # Use bytes as key
        if key not in unique_patterns:
            unique_patterns[key] = pattern

    print(f"Found {len(unique_patterns)} unique patterns (out of {len(data)} frames)")
    print()

    # Create sprite sheet
    scale = 6
    cols = 8
    rows = (len(unique_patterns) + cols - 1) // cols

    tile_width = 24 * scale
    tile_height = 24 * scale
    margin = 10
    label_height = 20

    sheet_width = cols * (tile_width + margin) + margin
    sheet_height = rows * (tile_height + label_height + margin) + margin

    sprite_sheet = Image.new('RGB', (sheet_width, sheet_height), (40, 40, 40))
    draw = ImageDraw.Draw(sprite_sheet)

    # Draw each unique pattern
    for idx, (pattern_bytes, pattern) in enumerate(unique_patterns.items()):
        col = idx % cols
        row = idx // cols

        x = margin + col * (tile_width + margin)
        y = margin + row * (tile_height + label_height + margin)

        # Draw tile
        img = pixels_to_image(pattern['pixels'], scale=scale)
        sprite_sheet.paste(img, (x, y))

        # Draw label
        label = f"F{pattern['frame']}"
        draw.text((x, y + tile_height + 2), label, fill=(200, 200, 200))

    sprite_sheet.save('part2_all_unique_patterns.png')
    print(f"✅ Saved sprite sheet: part2_all_unique_patterns.png")
    print(f"   {len(unique_patterns)} unique patterns, {cols}×{rows} grid")
    print()

    # Stats
    print("Pattern statistics:")
    print(f"  Total frames exported: {len(data)}")
    print(f"  Unique patterns: {len(unique_patterns)}")
    print(f"  Repetition rate: {len(data) / len(unique_patterns):.1f}x average")
    print()

    # Check if patterns are using full 72 bytes or less
    all_pattern_bytes = [base64.b64decode(p['pattern_bytes']) for p in data]
    non_zero_counts = [sum(1 for b in pb if b != 0) for pb in all_pattern_bytes]

    avg_non_zero = sum(non_zero_counts) / len(non_zero_counts)
    print(f"  Average non-zero bytes: {avg_non_zero:.1f} / 72")
    print(f"  Pattern sparsity: {(72 - avg_non_zero) / 72 * 100:.1f}% empty")
    print()

    print("✅ Part 2 (24×24 patterns) complete!")
    print("   These are the smaller square patterns from the emergence section")


if __name__ == '__main__':
    main()
