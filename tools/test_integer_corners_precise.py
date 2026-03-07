#!/usr/bin/env python3
"""
Test if corners are at integer positions.
Since we got 100% on frame 336, let's work backwards from that perfect match.
"""

import math
import re
from rotating_square import render_square, parse_frame, compare_bitmaps

# Frame 336 parameters that gave 100%
perfect_frame = 336
cx, cy, size, angle_deg = 12.0, 11.5, 8.0, 44.0  # Using 44° (middle of range)

# Calculate corners
angle_rad = math.radians(angle_deg)
corners = []
for i in range(4):
    corner_angle = angle_rad + (i * math.pi / 2)
    x = cx + size * math.cos(corner_angle)
    y = cy + size * math.sin(corner_angle)
    corners.append((x, y))

print(f"Frame {perfect_frame} perfect match analysis")
print("=" * 60)
print(f"Parameters: cx={cx}, cy={cy}, size={size}, angle={angle_deg}°")
print("\nCalculated corners:")
for i, (x, y) in enumerate(corners):
    print(f"  Corner {i}: ({x:.6f}, {y:.6f})")

print("\nRounded to integer:")
int_corners = [(round(x), round(y)) for x, y in corners]
for i, (x, y) in enumerate(int_corners):
    print(f"  Corner {i}: ({x}, {y})")

# Test if integer corners give 100%
with open("../emulator/frames/animation_318-336.txt", "r") as f:
    content = f.read()

frame_pattern = re.compile(r'=== Frame (\d+)')
lines = content.split('\n')
current_frame_lines = []
current_frame_num = None

target_bitmap = None

for line in lines:
    match = frame_pattern.search(line)
    if match:
        if current_frame_num == perfect_frame and current_frame_lines:
            target_bitmap = parse_frame(current_frame_lines)
            break
        current_frame_num = int(match.group(1))
        current_frame_lines = [line]
    else:
        current_frame_lines.append(line)

if current_frame_num == perfect_frame and current_frame_lines and not target_bitmap:
    target_bitmap, _ = parse_frame(current_frame_lines)

if target_bitmap and len(target_bitmap) == 24:
    # Test with floating point corners (known to work)
    rendered_float = render_square(cx, cy, size, angle_rad, 24, 24)
    matches_float, _ = compare_bitmaps(target_bitmap, rendered_float)
    print(f"\nFloating point corners: {matches_float}/576 ({100*matches_float/576:.2f}%)")

    # Now test if we can derive a pattern for other frames
    print("\n" + "=" * 60)
    print("Testing if pattern holds for other frames...")

    # Load all frames
    frames_data = {}
    current_frame_lines = []
    current_frame_num = None

    lines = content.split('\n')
    for line in lines:
        match = frame_pattern.search(line)
        if match:
            if current_frame_num in [320, 322, 324, 326, 328, 330, 332, 334, 336]:
                if current_frame_lines:
                    bitmap, _ = parse_frame(current_frame_lines)
                    if bitmap:
                        frames_data[current_frame_num] = bitmap

            current_frame_num = int(match.group(1))
            current_frame_lines = [line]
        else:
            current_frame_lines.append(line)

    if current_frame_num in [320, 322, 324, 326, 328, 330, 332, 334, 336]:
        if current_frame_lines:
            bitmap, _ = parse_frame(current_frame_lines)
            if bitmap:
                frames_data[current_frame_num] = bitmap

    # Test theory: Maybe they use integer center with floating size/angle?
    print("\nTesting integer center (11, 11):")

    for frame_num in sorted(frames_data.keys()):
        bitmap = frames_data[frame_num]

        # Try different sizes and angles with integer center
        best_match = 0
        best_params = None

        for test_size in [i * 0.5 for i in range(14, 18)]:  # 7.0 to 8.5
            for test_angle in [i * 0.5 for i in range(-20, 180)]:  # -10° to 90°
                rendered = render_square(11.0, 11.0, test_size, math.radians(test_angle), 24, 24)
                matches, _ = compare_bitmaps(bitmap, rendered)

                if matches > best_match:
                    best_match = matches
                    best_params = (test_size, test_angle)

        match_pct = 100 * best_match / 576
        size, angle = best_params
        print(f"  Frame {frame_num}: {match_pct:.2f}% (size={size:.1f}, angle={angle:.1f}°)")

        if match_pct == 100.0:
            print(f"    ✓ PERFECT!")
