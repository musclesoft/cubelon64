#!/usr/bin/env python3
"""
Visualize pixel differences between original and rendered as PNG.
"""

from PIL import Image, ImageDraw
import re
from rotating_square import parse_frame, compare_bitmaps
from test_8bit_precision import render_square_8bit
import math

def create_diff_visualization(original, rendered, best_params, frame_num):
    """
    Create a PNG showing original, rendered, and differences side by side.
    """
    # Scale factor for visibility
    scale = 16

    # Create image: 3 panels side by side (original, rendered, diff)
    width = 24 * 3 * scale
    height = 24 * scale
    img = Image.new('RGB', (width, height), 'white')
    draw = ImageDraw.Draw(img)

    # Panel 1: Original
    for y in range(24):
        for x in range(24):
            if original[y][x]:
                x1 = x * scale
                y1 = y * scale
                draw.rectangle([x1, y1, x1 + scale - 1, y1 + scale - 1], fill='black')

    # Panel 2: Rendered
    for y in range(24):
        for x in range(24):
            if rendered[y][x]:
                x1 = (24 + x) * scale
                y1 = y * scale
                draw.rectangle([x1, y1, x1 + scale - 1, y1 + scale - 1], fill='black')

    # Panel 3: Differences (red = missing in rendered, blue = extra in rendered)
    for y in range(24):
        for x in range(24):
            x1 = (48 + x) * scale
            y1 = y * scale

            if original[y][x] != rendered[y][x]:
                if original[y][x]:  # Original has pixel, rendered doesn't
                    color = 'red'
                else:  # Rendered has pixel, original doesn't
                    color = 'blue'
                draw.rectangle([x1, y1, x1 + scale - 1, y1 + scale - 1], fill=color)
            elif original[y][x]:  # Both have pixel
                draw.rectangle([x1, y1, x1 + scale - 1, y1 + scale - 1], fill='lightgray')

    # Add labels
    draw.text((5, height - 15), f"Original", fill='black')
    draw.text((24*scale + 5, height - 15), f"Rendered", fill='black')
    draw.text((48*scale + 5, height - 15), f"Diff (red=missing, blue=extra)", fill='black')

    # Add parameter info at bottom
    cx, cy, size, angle = best_params
    matches, total = compare_bitmaps(original, rendered)
    match_pct = 100 * matches / total

    # Expand image for text
    text_img = Image.new('RGB', (width, height + 60), 'white')
    text_img.paste(img, (0, 0))
    text_draw = ImageDraw.Draw(text_img)

    text_draw.text((5, height + 5), f"Frame {frame_num}", fill='black')
    text_draw.text((5, height + 20), f"Match: {match_pct:.3f}% ({matches}/{total} pixels)", fill='black')
    text_draw.text((5, height + 35), f"Params: cx={cx:.4f}, cy={cy:.4f}, size={size:.4f}, angle={angle:.2f}°", fill='black')

    return text_img

# Main
if __name__ == "__main__":
    import sys

    if len(sys.argv) < 6:
        print("Usage: python visualize_diff.py <frame> <cx> <cy> <size> <angle_deg>")
        sys.exit(1)

    frame_num = int(sys.argv[1])
    cx = float(sys.argv[2])
    cy = float(sys.argv[3])
    size = float(sys.argv[4])
    angle_deg = float(sys.argv[5])

    # Load original
    with open("../emulator/frames/animation_318-336.txt", 'r') as f:
        content = f.read()

    frame_pattern = re.compile(r'=== Frame (\d+)')
    lines = content.split('\n')
    current_frame_lines = []
    current_frame_num = None

    target_bitmap = None

    for line in lines:
        match = frame_pattern.search(line)
        if match:
            if current_frame_num == frame_num and current_frame_lines:
                target_bitmap, _ = parse_frame(current_frame_lines)
                break
            current_frame_num = int(match.group(1))
            current_frame_lines = [line]
        else:
            current_frame_lines.append(line)

    if not target_bitmap:
        print(f"Failed to load frame {frame_num}")
        sys.exit(1)

    # Render with best params
    rendered = render_square_8bit(cx, cy, size, math.radians(angle_deg), 24, 24)

    # Create visualization
    img = create_diff_visualization(target_bitmap, rendered, (cx, cy, size, angle_deg), frame_num)

    # Save
    output_file = f"diff_frame{frame_num}.png"
    img.save(output_file)
    print(f"Saved to {output_file}")
