#!/usr/bin/env python3
"""
Final comprehensive optimization with all parameters including larger sizes.
"""

import math
import re
from rotating_square import parse_frame, compare_bitmaps
from test_8bit_precision import render_square_8bit
from visualize_diff import create_diff_visualization

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

print("Final comprehensive optimization")
print("Testing: cx, cy, size (including LARGER), angle")
print("=" * 70)

step = 1.0 / 256.0
results = []

for frame_num, base_cx, base_cy, base_size, base_angle in test_configs:
    bitmap = load_frame(frame_num)
    if not bitmap:
        continue

    cx, cy, size, angle = base_cx, base_cy, base_size, base_angle

    print(f"\nFrame {frame_num}:")

    # Multi-pass optimization
    for iteration in range(3):
        improved = False

        # Optimize size (including larger)
        best_size, best_matches = size, 0
        for s_offset in range(-128, 257):  # -0.5 to +1.0
            test_size = base_size + s_offset * step
            if test_size < 1.0:
                continue
            rendered = render_square_8bit(cx, cy, test_size, math.radians(angle), 24, 24)
            matches, _ = compare_bitmaps(bitmap, rendered)
            if matches > best_matches:
                best_matches, best_size = matches, test_size

        if best_size != size:
            size = best_size
            improved = True

        # Optimize cx
        best_cx, best_matches = cx, 0
        for c_offset in range(-64, 65):  # ±0.25
            test_cx = base_cx + c_offset * step
            rendered = render_square_8bit(test_cx, cy, size, math.radians(angle), 24, 24)
            matches, _ = compare_bitmaps(bitmap, rendered)
            if matches > best_matches:
                best_matches, best_cx = matches, test_cx

        if best_cx != cx:
            cx = best_cx
            improved = True

        # Optimize cy
        best_cy, best_matches = cy, 0
        for c_offset in range(-64, 65):
            test_cy = base_cy + c_offset * step
            rendered = render_square_8bit(cx, test_cy, size, math.radians(angle), 24, 24)
            matches, _ = compare_bitmaps(bitmap, rendered)
            if matches > best_matches:
                best_matches, best_cy = matches, test_cy

        if best_cy != cy:
            cy = best_cy
            improved = True

        # Optimize angle
        best_angle, best_matches = angle, 0
        for a_offset in range(-100, 101):  # ±10° in 0.1° steps
            test_angle = base_angle + a_offset * 0.1
            rendered = render_square_8bit(cx, cy, size, math.radians(test_angle), 24, 24)
            matches, _ = compare_bitmaps(bitmap, rendered)
            if matches > best_matches:
                best_matches, best_angle = matches, test_angle

        if best_angle != angle:
            angle = best_angle
            improved = True

        if not improved:
            break

    # Final test
    rendered = render_square_8bit(cx, cy, size, math.radians(angle), 24, 24)
    matches, _ = compare_bitmaps(bitmap, rendered)
    match_pct = 100 * matches / 576
    diff = 576 - matches

    results.append({
        'frame': frame_num,
        'matches': matches,
        'params': (cx, cy, size, angle)
    })

    print(f"  Final: {match_pct:.3f}% ({matches}/576, diff={diff})")
    print(f"  cx={cx:.6f}, cy={cy:.6f}, size={size:.6f}, angle={angle:.2f}°")

    # Create updated visualization
    img = create_diff_visualization(bitmap, rendered, (cx, cy, size, angle), frame_num)
    img.save(f"diff_final_frame{frame_num}.png")

    if matches == 576:
        print(f"  ✓ PERFECT 100%!")

print("\n" + "=" * 70)
print("Final Summary:")

total_matches = sum(r['matches'] for r in results)
avg_match = 100 * total_matches / (len(results) * 576)
avg_diff = (len(results) * 576 - total_matches) / len(results)

print(f"Average: {avg_match:.3f}%")
print(f"Average diff: {avg_diff:.1f} pixels/frame")

perfect = sum(1 for r in results if r['matches'] == 576)
if perfect > 0:
    print(f"\n✓ {perfect} PERFECT matches!")

print(f"\nBest per frame:")
for r in sorted(results, key=lambda x: x['matches'], reverse=True):
    cx, cy, size, angle = r['params']
    match_pct = 100 * r['matches'] / 576
    diff = 576 - r['matches']
    print(f"  Frame {r['frame']}: {match_pct:.3f}% (diff={diff})")

print(f"\nUpdated visualizations saved as diff_final_frame*.png")
