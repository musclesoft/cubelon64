#!/usr/bin/env python3
"""
Refine pivot search around (18, 15) with fractional coordinates.
"""

import math
from test_pivot_18_15 import render_square_around_pivot, load_raw_pattern, compare_bitmaps

# Test one frame (336 - the horizontal bars)
frame = 336
filename = f"../emulator/frames/raw_frame{frame}.txt"
original = load_raw_pattern(filename)

print(f"Refining pivot for Frame {frame}")
print("=" * 70)

best_match = 0
best_pivot = None
best_params = None

# Test fractional pivots around (18, 15)
for pivot_x in [i * 0.5 for i in range(32, 40)]:  # 16.0 to 19.5
    for pivot_y in [i * 0.5 for i in range(26, 34)]:  # 13.0 to 16.5
        for size in [i * 0.5 for i in range(18, 24)]:  # 9.0 to 11.5
            for angle_deg in range(-90, -70):
                angle_rad = math.radians(angle_deg)

                rendered = render_square_around_pivot(pivot_x, pivot_y, size, angle_rad, 24, 24)
                matches = compare_bitmaps(original, rendered)

                if matches > best_match:
                    best_match = matches
                    best_pivot = (pivot_x, pivot_y)
                    best_params = (size, angle_deg)

                    match_pct = 100 * matches / 576
                    if match_pct > 99.5:
                        print(f"  {match_pct:.2f}%: pivot=({pivot_x}, {pivot_y}), size={size}, angle={angle_deg}°")

match_pct = 100 * best_match / 576
pivot_x, pivot_y = best_pivot
size, angle = best_params

print(f"\nBest match: {match_pct:.2f}%")
print(f"  Pivot: ({pivot_x}, {pivot_y})")
print(f"  Size: {size}")
print(f"  Angle: {angle}°")

if match_pct == 100.0:
    print("\n✓ PERFECT MATCH!")
