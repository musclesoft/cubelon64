#!/usr/bin/env python3
"""
Final brute force at 8.8 precision using known-working pattern loader.
"""

import math
import re
from rotating_square import parse_frame, compare_bitmaps
from test_8bit_precision import render_square_8bit

# Load animation file
with open("../emulator/frames/animation_318-336.txt", 'r') as f:
    content = f.read()

# Parse frame 324
frame_pattern = re.compile(r'=== Frame (\d+)')
lines = content.split('\n')
current_frame_lines = []
current_frame_num = None

target_frame = 324
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

if not target_bitmap:
    print("Failed to load frame 324")
    exit(1)

print(f"Brute force at 8.8 fixed-point precision for Frame 324")
print("=" * 70)

# Known best from previous search
base_cx, base_cy, base_size, base_angle = 11.5, 11.0, 8.0, 77.3

# 8.8 precision = 1/256
precision = 1.0 / 256.0

best_match = 0
best_params = None
tested = 0

print(f"Base: cx={base_cx}, cy={base_cy}, size={base_size}, angle={base_angle}°")
print(f"Searching ±1.0 (step=4/256=0.015625), angle ±5° (step=0.2°)")
print()

for cx_offset in range(-256, 257, 4):  # Step by 4/256
    cx = base_cx + cx_offset * precision

    for cy_offset in range(-256, 257, 4):
        cy = base_cy + cy_offset * precision

        for size_offset in range(-256, 257, 4):
            size = base_size + size_offset * precision

            if size <= 1.0:
                continue

            for angle_offset_05 in range(-25, 26):  # ±5° in 0.2° steps
                angle = base_angle + angle_offset_05 * 0.2

                rendered = render_square_8bit(cx, cy, size, math.radians(angle), 24, 24)
                matches, total = compare_bitmaps(target_bitmap, rendered)

                tested += 1

                if tested % 50000 == 0:
                    print(f"  Tested {tested:,}... best: {100*best_match/576:.2f}%")

                if matches > best_match:
                    best_match = matches
                    best_params = (cx, cy, size, angle)

                    match_pct = 100 * matches / 576
                    if match_pct > 99.0:
                        print(f"  {match_pct:.3f}%: cx={cx:.6f}, cy={cy:.6f}, size={size:.6f}, angle={angle:.2f}°")

                    if matches == 576:
                        print(f"\n✓ PERFECT MATCH!")
                        print(f"  cx={cx:.6f} (8.8={int(cx*256)})")
                        print(f"  cy={cy:.6f} (8.8={int(cy*256)})")
                        print(f"  size={size:.6f} (8.8={int(size*256)})")
                        print(f"  angle={angle:.2f}°")
                        print(f"  Tested {tested:,} combinations")
                        exit(0)

match_pct = 100 * best_match / 576
print(f"\nBest: {match_pct:.3f}% ({best_match}/576)")
if best_params:
    cx, cy, size, angle = best_params
    print(f"  cx={cx:.6f} (8.8={int(cx*256)})")
    print(f"  cy={cy:.6f} (8.8={int(cy*256)})")
    print(f"  size={size:.6f} (8.8={int(size*256)})")
    print(f"  angle={angle:.2f}°")
print(f"Tested {tested:,} combinations")
print(f"\nStill {576-best_match} pixels different - patterns are pre-rendered bitmaps")
