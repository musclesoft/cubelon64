#!/usr/bin/env python3
"""
Show pixel-by-pixel differences between extracted and rendered frames.
"""

import math
import re
from rotating_square import render_square, parse_frame, bitmap_to_string

# Read animation file
with open("../emulator/frames/animation_318-336.txt", "r") as f:
    content = f.read()

# Best parameters found from fine-tuning
best_params = {
    320: (12.0, 11.0, 8.0, 61.1),
    322: (12.5, 11.5, 8.0, 70.9),
    324: (11.5, 11.0, 8.0, 77.3),
    326: (12.0, 11.0, 7.5, -4.2),
    328: (12.5, 11.0, 8.0, 9.1),
    330: (11.5, 11.5, 8.0, 17.3),
    332: (12.0, 11.0, 8.0, 22.0),
    334: (12.0, 11.5, 8.0, 35.2),
    336: (12.0, 11.5, 8.0, 43.2),  # Perfect!
}

# Pick frame 324 to examine (typical ~98.8% match)
target_frame = 324

frame_pattern = re.compile(r'=== Frame (\d+)')
lines = content.split('\n')
current_frame_lines = []
current_frame_num = None

for line in lines:
    match = frame_pattern.search(line)
    if match:
        if current_frame_num == target_frame and current_frame_lines:
            bitmap, _ = parse_frame(current_frame_lines)

            if bitmap and len(bitmap) == 24:
                # Render with best params
                cx, cy, size, angle_deg = best_params[target_frame]
                rendered = render_square(cx, cy, size, math.radians(angle_deg), 24, 24)

                print(f"Frame {target_frame} comparison")
                print("=" * 50)

                # Count differences
                diffs = []
                matches = 0
                for y in range(24):
                    for x in range(24):
                        if bitmap[y][x] == rendered[y][x]:
                            matches += 1
                        else:
                            diffs.append((x, y, bitmap[y][x], rendered[y][x]))

                print(f"Matches: {matches}/576 ({100*matches/576:.2f}%)")
                print(f"Differences: {len(diffs)}")

                if len(diffs) > 0:
                    print("\nDifference locations:")
                    for x, y, orig, rend in diffs[:20]:  # Show first 20
                        orig_char = "█" if orig else "·"
                        rend_char = "█" if rend else "·"
                        print(f"  ({x:2d},{y:2d}): original={orig_char} rendered={rend_char}")

                    if len(diffs) > 20:
                        print(f"  ... and {len(diffs) - 20} more")

                print("\nOriginal:")
                print(bitmap_to_string(bitmap))

                print("\nRendered:")
                print(bitmap_to_string(rendered))

                print("\nDifferences (X marks diff location):")
                diff_vis = []
                for y in range(24):
                    row = ""
                    for x in range(24):
                        if (x, y) in [(d[0], d[1]) for d in diffs]:
                            row += "X"
                        else:
                            row += "·"
                    diff_vis.append(row)
                print("\n".join(diff_vis))

            break

        current_frame_num = int(match.group(1))
        current_frame_lines = [line]
    else:
        current_frame_lines.append(line)
