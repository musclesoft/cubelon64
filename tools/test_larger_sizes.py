#!/usr/bin/env python3
"""
Test if LARGER sizes improve match (user observation: more red than blue pixels).
Quick focused test without full brute force.
"""

import math
import re
from rotating_square import parse_frame, compare_bitmaps
from test_8bit_precision import render_square_8bit

# Load animation file
with open("../emulator/frames/animation_318-336.txt", 'r') as f:
    content = f.read()

frame_pattern = re.compile(r'=== Frame (\d+)')

def load_frame(target_frame):
    lines = content.split('\n')
    current_frame_lines = []
    current_frame_num = None

    for line in lines:
        match = frame_pattern.search(line)
        if match:
            if current_frame_num == target_frame and current_frame_lines:
                bitmap, _ = parse_frame(current_frame_lines)
                return bitmap
            current_frame_num = int(match.group(1))
            current_frame_lines = [line]
        else:
            current_frame_lines.append(line)

    if current_frame_num == target_frame:
        bitmap, _ = parse_frame(current_frame_lines)
        return bitmap
    return None

# Test configs with current best
test_configs = [
    (320, 12.0, 11.0, 8.0, 61.1),
    (322, 12.5, 11.5, 8.0, 70.9),
    (324, 11.5, 11.0, 8.0, 77.3),
    (326, 12.0, 11.0, 7.5, -4.2),
    (328, 12.5, 11.0, 8.0, 9.1),
    (330, 11.5, 11.5, 8.0, 17.3),
    (332, 12.0, 11.0, 8.0, 22.0),
    (334, 12.0, 11.5, 8.0, 35.2),
]

print("Testing if LARGER sizes improve match")
print("User observation: more red (missing) than blue (extra) pixels")
print("=" * 70)

step = 1.0 / 256.0  # 8.8 precision

for frame_num, base_cx, base_cy, base_size, base_angle in test_configs:
    bitmap = load_frame(frame_num)
    if not bitmap:
        continue

    # Test current size
    current_rendered = render_square_8bit(base_cx, base_cy, base_size, math.radians(base_angle), 24, 24)
    current_matches, _ = compare_bitmaps(bitmap, current_rendered)
    current_pct = 100 * current_matches / 576

    print(f"\nFrame {frame_num}:")
    print(f"  Current (size={base_size:.3f}): {current_pct:.3f}% ({current_matches}/576)")

    # Test systematically larger sizes
    best_size = base_size
    best_matches = current_matches

    # Test +0.0 to +1.0 in 8.8 steps
    for size_offset in range(0, 257, 1):  # 0 to +1.0
        test_size = base_size + size_offset * step

        rendered = render_square_8bit(base_cx, base_cy, test_size, math.radians(base_angle), 24, 24)
        matches, _ = compare_bitmaps(bitmap, rendered)

        if matches > best_matches:
            best_matches = matches
            best_size = test_size

            match_pct = 100 * matches / 576
            improvement = matches - current_matches
            print(f"    ↑ size={test_size:.6f}: {match_pct:.3f}% ({matches}/576, +{improvement})")

            if matches == 576:
                print(f"      ✓ PERFECT 100%!")
                break

    if best_matches > current_matches:
        print(f"  Best: size={best_size:.6f} improved by {best_matches - current_matches} pixels")
    elif best_matches == current_matches:
        print(f"  No improvement with larger sizes")

print("\n" + "=" * 70)
print("Summary:")
print("If larger sizes consistently improve, hypothesis is confirmed.")
print("If not, the issue is elsewhere (angle/position or algorithm difference).")
