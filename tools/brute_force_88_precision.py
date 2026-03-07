#!/usr/bin/env python3
"""
Brute force search at 8.8 fixed-point precision (1/256 increments).
This is the actual granularity an 8-bit system would use.
"""

import math
from test_8bit_precision import render_square_8bit, compare_bitmaps

def load_centered_pattern(content, frame):
    """Extract pattern from animation file"""
    frame_start = content.find(f"=== Frame {frame}")
    if frame_start == -1:
        return None

    frame_end = content.find("===", frame_start + 1)
    if frame_end == -1:
        frame_end = len(content)

    frame_text = content[frame_start:frame_end]
    lines = frame_text.split('\n')

    original = []
    for line in lines:
        row = []
        for char in line:
            if char == '█':
                row.append(True)
            elif char == '·':
                row.append(False)
        if len(row) == 24:
            original.append(row)
        if len(original) == 24:
            break

    return original if len(original) == 24 else None

# Load animation file
import os
animation_file = "../emulator/frames/animation_318-336.txt"
if not os.path.exists(animation_file):
    animation_file = "frames/animation_318-336.txt"

with open(animation_file, 'r') as f:
    content = f.read()

# Test frame 324 (typical ~98.78% case with 7 pixel difference)
test_frame = 324
original = load_centered_pattern(content, test_frame)

if not original:
    print(f"Failed to load frame {test_frame}")
    print(f"Tried loading from: {animation_file}")
    exit(1)

print(f"Brute force search at 8.8 fixed-point precision for Frame {test_frame}")
print("=" * 70)
print(f"Precision: 1/256 = {1/256:.6f}")
print()

# Known best parameters from previous search
base_cx, base_cy, base_size, base_angle = 11.5, 11.0, 8.0, 77.3

# Search around these with 8.8 precision
# 1/256 is the smallest representable value in 8.8 fixed-point
precision = 1.0 / 256.0

best_match = 0
best_params = None

print(f"Base parameters: cx={base_cx}, cy={base_cy}, size={base_size}, angle={base_angle}°")
print(f"Searching ±1.0 in x/y/size, ±5° in angle at {precision:.6f} precision...")
print()

# Convert ranges to 8.8 fixed-point steps
# ±1.0 = ±256 steps in 8.8
# Test every 4th value to make search tractable (1/64 precision = 0.015625)

tested = 0
progress_interval = 50000

# Step by 4 in 8.8 (= 4/256 = 0.015625 increments)
step_size = 4

for cx_steps in range(-256, 257, step_size):  # ±1.0 in steps of 4/256
    cx = base_cx + cx_steps * precision

    for cy_steps in range(-256, 257, step_size):
        cy = base_cy + cy_steps * precision

        for size_steps in range(-256, 257, step_size):
            size = base_size + size_steps * precision

            if size <= 0:
                continue

            # For angle, test ±5° in 0.2° steps
            for angle_deg_10 in range(int((base_angle - 5) * 5), int((base_angle + 5) * 5) + 1):
                angle = angle_deg_10 / 5.0

                rendered = render_square_8bit(cx, cy, size, math.radians(angle), 24, 24)
                matches = compare_bitmaps(original, rendered)

                tested += 1

                if tested % progress_interval == 0:
                    print(f"  Tested {tested:,} combinations... best so far: {100*best_match/576:.2f}%")

                if matches > best_match:
                    best_match = matches
                    best_params = (cx, cy, size, angle)

                    match_pct = 100 * matches / 576
                    if match_pct > 99.0:
                        print(f"  {match_pct:.2f}%: cx={cx:.6f}, cy={cy:.6f}, size={size:.6f}, angle={angle:.1f}°")

                    if matches == 576:
                        print(f"\n✓ PERFECT MATCH!")
                        print(f"  Tested {tested:,} combinations")
                        print(f"  Parameters:")
                        print(f"    cx     = {cx:.6f}")
                        print(f"    cy     = {cy:.6f}")
                        print(f"    size   = {size:.6f}")
                        print(f"    angle  = {angle:.1f}°")
                        break

            if best_match == 576:
                break
        if best_match == 576:
            break
    if best_match == 576:
        break

match_pct = 100 * best_match / 576
print(f"\nBest match: {match_pct:.2f}% ({best_match}/576)")
if best_params:
    cx, cy, size, angle = best_params
    print(f"  cx     = {cx:.6f}  (8.8 = {int(cx * 256)})")
    print(f"  cy     = {cy:.6f}  (8.8 = {int(cy * 256)})")
    print(f"  size   = {size:.6f}  (8.8 = {int(size * 256)})")
    print(f"  angle  = {angle:.1f}°")
print(f"\nTotal combinations tested: {tested:,}")

if best_match < 576:
    print(f"\nStill {576 - best_match} pixels different.")
    print("The original likely uses:")
    print("  - Different edge algorithm (Bresenham?)")
    print("  - Different rounding mode")
    print("  - Pre-rendered bitmaps (not real-time generation)")
