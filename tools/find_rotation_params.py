#!/usr/bin/env python3
"""
Fine-tune rotation parameters to get pixel-perfect matches.
Based on the coarse search, refine around the best parameters found.
"""

import math
import re
from rotating_square import render_square, compare_bitmaps, parse_frame, bitmap_to_string

# Read animation file
with open("../emulator/frames/animation_318-336.txt", "r") as f:
    content = f.read()

# Parse all unique pattern frames
frames_to_analyze = [320, 322, 324, 326, 328, 330, 332, 334, 336]

# Coarse search results as starting points
coarse_results = {
    320: (12.0, 11.0, 8.0, 62.0),
    322: (12.0, 11.5, 8.0, 70.0),
    324: (11.5, 11.0, 8.0, 78.0),
    326: (12.0, 11.0, 7.5, 0.0),   # Probably needs negative angle
    328: (12.0, 11.0, 8.0, 2.0),
    330: (11.5, 11.5, 8.0, 18.0),
    332: (12.0, 11.0, 8.0, 22.0),
    334: (12.0, 11.5, 8.0, 34.0),
    336: (12.0, 11.5, 8.0, 44.0),  # 100% match!
}

frame_pattern = re.compile(r'=== Frame (\d+)')
lines = content.split('\n')
current_frame_lines = []
current_frame_num = None

print("Fine-tuning rotation parameters")
print("=" * 60)

all_results = []

for line in lines:
    match = frame_pattern.search(line)
    if match:
        # Process previous frame
        if current_frame_num in frames_to_analyze and current_frame_lines:
            bitmap, _ = parse_frame(current_frame_lines)

            if bitmap and len(bitmap) == 24:
                # Get coarse result as starting point
                coarse_cx, coarse_cy, coarse_size, coarse_angle = coarse_results[current_frame_num]

                best_match = 0
                best_params = None

                # Fine search around coarse result
                # Try angles from -10 to +10 degrees around coarse angle
                # Also try angles 360° away (for wraparound)
                angle_ranges = [
                    (coarse_angle - 10, coarse_angle + 10),
                    (coarse_angle - 370, coarse_angle - 350),  # Wraparound negative
                ]

                for angle_min, angle_max in angle_ranges:
                    for angle_deg in range(int(angle_min * 10), int(angle_max * 10) + 1):
                        angle_deg_f = angle_deg / 10.0

                        # Try small variations in center and size
                        for cx_offset in [-0.5, 0.0, 0.5]:
                            for cy_offset in [-0.5, 0.0, 0.5]:
                                for size_offset in [-0.5, 0.0, 0.5]:
                                    cx = coarse_cx + cx_offset
                                    cy = coarse_cy + cy_offset
                                    size = coarse_size + size_offset
                                    angle_rad = math.radians(angle_deg_f)

                                    rendered = render_square(cx, cy, size, angle_rad, 24, 24)
                                    matches, total = compare_bitmaps(bitmap, rendered)

                                    if matches > best_match:
                                        best_match = matches
                                        best_params = (cx, cy, size, angle_deg_f)

                if best_params:
                    match_pct = 100 * best_match / (24 * 24)
                    cx, cy, size, angle_deg = best_params

                    print(f"\nFrame {current_frame_num}:")
                    print(f"  Params: cx={cx:.1f}, cy={cy:.1f}, size={size:.1f}, angle={angle_deg:.1f}°")
                    print(f"  Match: {match_pct:.2f}%")

                    all_results.append({
                        'frame': current_frame_num,
                        'cx': cx,
                        'cy': cy,
                        'size': size,
                        'angle': angle_deg,
                        'match': match_pct
                    })

                    if match_pct == 100.0:
                        print("  ✓ PERFECT MATCH!")

        # Start new frame
        current_frame_num = int(match.group(1))
        current_frame_lines = [line]
    else:
        current_frame_lines.append(line)

# Process last frame
if current_frame_num in frames_to_analyze and current_frame_lines:
    bitmap, _ = parse_frame(current_frame_lines)

    if bitmap and len(bitmap) == 24:
        coarse_cx, coarse_cy, coarse_size, coarse_angle = coarse_results[current_frame_num]

        best_match = 0
        best_params = None

        angle_ranges = [
            (coarse_angle - 10, coarse_angle + 10),
            (coarse_angle - 370, coarse_angle - 350),
        ]

        for angle_min, angle_max in angle_ranges:
            for angle_deg in range(int(angle_min * 10), int(angle_max * 10) + 1):
                angle_deg_f = angle_deg / 10.0

                for cx_offset in [-0.5, 0.0, 0.5]:
                    for cy_offset in [-0.5, 0.0, 0.5]:
                        for size_offset in [-0.5, 0.0, 0.5]:
                            cx = coarse_cx + cx_offset
                            cy = coarse_cy + cy_offset
                            size = coarse_size + size_offset
                            angle_rad = math.radians(angle_deg_f)

                            rendered = render_square(cx, cy, size, angle_rad, 24, 24)
                            matches, total = compare_bitmaps(bitmap, rendered)

                            if matches > best_match:
                                best_match = matches
                                best_params = (cx, cy, size, angle_deg_f)

        if best_params:
            match_pct = 100 * best_match / (24 * 24)
            cx, cy, size, angle_deg = best_params

            print(f"\nFrame {current_frame_num}:")
            print(f"  Params: cx={cx:.1f}, cy={cy:.1f}, size={size:.1f}, angle={angle_deg:.1f}°")
            print(f"  Match: {match_pct:.2f}%")

            all_results.append({
                'frame': current_frame_num,
                'cx': cx,
                'cy': cy,
                'size': size,
                'angle': angle_deg,
                'match': match_pct
            })

            if match_pct == 100.0:
                print("  ✓ PERFECT MATCH!")

# Summary
print("\n" + "=" * 60)
print("Summary:")
print(f"Perfect matches: {sum(1 for r in all_results if r['match'] == 100.0)}/{len(all_results)}")
print(f"Average match: {sum(r['match'] for r in all_results) / len(all_results):.2f}%")

# Check if rotation is monotonic
print("\nRotation sequence:")
for r in all_results:
    print(f"  Frame {r['frame']}: {r['angle']:6.1f}°")
