#!/usr/bin/env python3
"""
Verify the 100% match claim for frame 336 and visualize the differences.
"""

import math
import re
from rotating_square import render_square, parse_frame, bitmap_to_string, compare_bitmaps

# Load frame 336
with open("../emulator/frames/animation_318-336.txt", "r") as f:
    content = f.read()

frame_pattern = re.compile(r'=== Frame (\d+)')
lines = content.split('\n')
current_frame_lines = []
current_frame_num = None

target_frame = 336
target_bitmap = None

for line in lines:
    match = frame_pattern.search(line)
    if match:
        if current_frame_num == target_frame and current_frame_lines:
            target_bitmap, _ = parse_frame(current_frame_lines)
            break
        current_frame_num = int(match.group(1))
        current_frame_lines = [line]
    else:
        current_frame_lines.append(line)

if current_frame_num == target_frame and current_frame_lines and not target_bitmap:
    target_bitmap, _ = parse_frame(current_frame_lines)

if target_bitmap and len(target_bitmap) == 24:
    # Test the "perfect" parameters
    cx, cy, size, angle_deg = 12.0, 11.5, 8.0, 43.2

    rendered = render_square(cx, cy, size, math.radians(angle_deg), 24, 24)
    matches, total = compare_bitmaps(target_bitmap, rendered)
    match_pct = 100 * matches / total

    print(f"Frame 336 with parameters: cx={cx}, cy={cy}, size={size}, angle={angle_deg}°")
    print(f"Match: {matches}/{total} = {match_pct:.2f}%")

    if matches == total:
        print("✓ CONFIRMED PERFECT MATCH!")
    else:
        print(f"✗ NOT PERFECT - {total - matches} pixels different")

        # Show differences
        print("\nOriginal:")
        print(bitmap_to_string(target_bitmap))

        print("\nRendered:")
        print(bitmap_to_string(rendered))

        print("\nDifferences:")
        for y in range(24):
            for x in range(24):
                if target_bitmap[y][x] != rendered[y][x]:
                    orig = "█" if target_bitmap[y][x] else "·"
                    rend = "█" if rendered[y][x] else "·"
                    print(f"  ({x:2d},{y:2d}): original={orig} rendered={rend}")

    # Try nearby angles to see if 100% is achievable
    print("\n" + "=" * 60)
    print("Testing nearby angles:")

    best_match = matches
    best_angle = angle_deg

    for test_angle in range(int((angle_deg - 2) * 10), int((angle_deg + 2) * 10) + 1):
        test_angle_f = test_angle / 10.0
        rendered = render_square(cx, cy, size, math.radians(test_angle_f), 24, 24)
        matches, total = compare_bitmaps(target_bitmap, rendered)

        if matches > best_match:
            best_match = matches
            best_angle = test_angle_f

        if matches == total:
            print(f"✓ 100% match at angle={test_angle_f:.1f}°")

    if best_match < total:
        print(f"\nBest match: {best_match}/{total} ({100*best_match/total:.2f}%) at angle={best_angle:.1f}°")
else:
    print(f"Failed to load frame {target_frame}")
