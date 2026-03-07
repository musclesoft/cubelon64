#!/usr/bin/env python3
"""
Test if frame 336 is actually two separate vertical bars.
"""

from test_pivot_18_15 import load_raw_pattern, compare_bitmaps, bitmap_to_string

frame = 336
filename = f"../emulator/frames/raw_frame{frame}.txt"
original = load_raw_pattern(filename)

print(f"Testing if Frame {frame} is two separate rectangles")
print("=" * 70)

# Try rendering two vertical bars
def render_two_bars(left_x1, left_x2, right_x1, right_x2, y_top, y_bottom):
    bitmap = [[False] * 24 for _ in range(24)]

    for y in range(y_top, y_bottom + 1):
        # Left bar
        for x in range(left_x1, left_x2 + 1):
            if 0 <= x < 24 and 0 <= y < 24:
                bitmap[y][x] = True

        # Right bar
        for x in range(right_x1, right_x2 + 1):
            if 0 <= x < 24 and 0 <= y < 24:
                bitmap[y][x] = True

    return bitmap

# From observation: left bar is X=0-5, right bar is X=18-23, Y=6-17
best_match = 0
best_params = None

for left_x1 in range(0, 3):
    for left_x2 in range(3, 8):
        for right_x1 in range(16, 21):
            for right_x2 in range(21, 24):
                for y_top in range(4, 8):
                    for y_bottom in range(15, 20):
                        rendered = render_two_bars(left_x1, left_x2, right_x1, right_x2, y_top, y_bottom)
                        matches = compare_bitmaps(original, rendered)

                        if matches > best_match:
                            best_match = matches
                            best_params = (left_x1, left_x2, right_x1, right_x2, y_top, y_bottom)

match_pct = 100 * best_match / 576
print(f"Best match: {match_pct:.2f}% ({best_match}/576)")
print(f"  Parameters: left_x={best_params[0]}-{best_params[1]}, right_x={best_params[2]}-{best_params[3]}, y={best_params[4]}-{best_params[5]}")

if match_pct == 100.0:
    print("\n✓ PERFECT MATCH - It's two separate bars!")
    left_x1, left_x2, right_x1, right_x2, y_top, y_bottom = best_params
    rendered = render_two_bars(left_x1, left_x2, right_x1, right_x2, y_top, y_bottom)
    print("\nRendered:")
    print(bitmap_to_string(rendered))
else:
    print(f"\nNot two separate bars (only {match_pct:.2f}% match)")
    print("Must be a rotated filled polygon with specific edge algorithm")
