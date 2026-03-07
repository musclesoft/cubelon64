#!/usr/bin/env python3
"""
Test all frames with smart 8.8 optimization and create visualizations.
"""

import math
import re
from rotating_square import parse_frame, compare_bitmaps
from test_8bit_precision import render_square_8bit
from visualize_diff import create_diff_visualization

# Load animation file
with open("../emulator/frames/animation_318-336.txt", 'r') as f:
    content = f.read()

# Test all unique frames
frames_to_test = {
    320: (12.0, 11.0, 8.0, 61.1),
    322: (12.5, 11.5, 8.0, 70.9),
    324: (11.5, 11.0, 8.0, 77.3),
    326: (12.0, 11.0, 7.5, -4.2),
    328: (12.5, 11.0, 8.0, 9.1),
    330: (11.5, 11.5, 8.0, 17.3),
    332: (12.0, 11.0, 8.0, 22.0),
    334: (12.0, 11.5, 8.0, 35.2),
    336: (12.0, 11.5, 8.0, 43.2),
}

frame_pattern = re.compile(r'=== Frame (\d+)')

print("Testing all frames with 8.8 optimization")
print("=" * 70)

results = []
step = 1.0 / 256.0

for target_frame, (base_cx, base_cy, base_size, base_angle) in frames_to_test.items():
    # Load pattern
    lines = content.split('\n')
    current_frame_lines = []
    current_frame_num = None
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
        continue

    # Quick optimization
    cx, cy, size, angle = base_cx, base_cy, base_size, base_angle

    def test_params(cx, cy, size, angle):
        rendered = render_square_8bit(cx, cy, size, math.radians(angle), 24, 24)
        matches, total = compare_bitmaps(target_bitmap, rendered)
        return matches

    # Optimize cx
    best_cx, best_matches = cx, test_params(cx, cy, size, angle)
    for offset in range(-32, 33):
        test_cx = cx + offset * step
        matches = test_params(test_cx, cy, size, angle)
        if matches > best_matches:
            best_matches, best_cx = matches, test_cx

    cx = best_cx

    # Optimize cy
    best_cy, best_matches = cy, test_params(cx, cy, size, angle)
    for offset in range(-32, 33):
        test_cy = cy + offset * step
        matches = test_params(cx, test_cy, size, angle)
        if matches > best_matches:
            best_matches, best_cy = matches, test_cy

    cy = best_cy

    # Final match
    rendered = render_square_8bit(cx, cy, size, math.radians(angle), 24, 24)
    matches, total = compare_bitmaps(target_bitmap, rendered)
    match_pct = 100 * matches / total

    results.append({
        'frame': target_frame,
        'matches': matches,
        'match_pct': match_pct,
        'diff': 576 - matches,
        'params': (cx, cy, size, angle)
    })

    print(f"Frame {target_frame}: {match_pct:.2f}% ({matches}/576, diff={576-matches})")

    # Create visualization
    img = create_diff_visualization(target_bitmap, rendered, (cx, cy, size, angle), target_frame)
    img.save(f"diff_frame{target_frame}.png")

print("\n" + "=" * 70)
print("Summary:")
total_matches = sum(r['matches'] for r in results)
total_pixels = len(results) * 576
avg_match = 100 * total_matches / total_pixels
print(f"Average: {avg_match:.2f}%")
print(f"Total differences: {total_pixels - total_matches} pixels across {len(results)} frames")
print(f"Average per frame: {(total_pixels - total_matches) / len(results):.1f} pixels")

perfect_frames = [r['frame'] for r in results if r['matches'] == 576]
if perfect_frames:
    print(f"\nPerfect matches: {perfect_frames}")
else:
    print(f"\nNo perfect matches - confirms patterns are pre-rendered bitmaps")

print(f"\nVisualization PNGs saved for all frames")
