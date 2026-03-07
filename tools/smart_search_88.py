#!/usr/bin/env python3
"""
Smart parameter search: optimize one parameter at a time.
Much faster than full brute force.
"""

import math
import re
from rotating_square import parse_frame, compare_bitmaps
from test_8bit_precision import render_square_8bit

# Load frame 324
with open("../emulator/frames/animation_318-336.txt", 'r') as f:
    content = f.read()

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

print(f"Smart parameter search for Frame {target_frame}")
print("=" * 70)

# Start with known best
cx, cy, size, angle = 11.5, 11.0, 8.0, 77.3

# 8.8 precision
step = 1.0 / 256.0

def test_params(cx, cy, size, angle):
    rendered = render_square_8bit(cx, cy, size, math.radians(angle), 24, 24)
    matches, total = compare_bitmaps(target_bitmap, rendered)
    return matches

print(f"Starting: cx={cx}, cy={cy}, size={size}, angle={angle}°")
initial_matches = test_params(cx, cy, size, angle)
print(f"Initial match: {100*initial_matches/576:.3f}% ({initial_matches}/576)")
print()

# Optimize each parameter separately
for iteration in range(5):  # Multiple passes
    print(f"Pass {iteration + 1}:")

    # Optimize cx
    best_cx = cx
    best_matches = test_params(cx, cy, size, angle)

    for offset in range(-64, 65):  # ±0.25 in 8.8 steps
        test_cx = cx + offset * step
        matches = test_params(test_cx, cy, size, angle)
        if matches > best_matches:
            best_matches = matches
            best_cx = test_cx

    if best_cx != cx:
        cx = best_cx
        print(f"  cx improved to {cx:.6f} ({100*best_matches/576:.3f}%)")

    # Optimize cy
    best_cy = cy
    best_matches = test_params(cx, cy, size, angle)

    for offset in range(-64, 65):
        test_cy = cy + offset * step
        matches = test_params(cx, test_cy, size, angle)
        if matches > best_matches:
            best_matches = matches
            best_cy = test_cy

    if best_cy != cy:
        cy = best_cy
        print(f"  cy improved to {cy:.6f} ({100*best_matches/576:.3f}%)")

    # Optimize size
    best_size = size
    best_matches = test_params(cx, cy, size, angle)

    for offset in range(-64, 65):
        test_size = size + offset * step
        if test_size <= 1.0:
            continue
        matches = test_params(cx, cy, test_size, angle)
        if matches > best_matches:
            best_matches = matches
            best_size = test_size

    if best_size != size:
        size = best_size
        print(f"  size improved to {size:.6f} ({100*best_matches/576:.3f}%)")

    # Optimize angle (finer steps)
    best_angle = angle
    best_matches = test_params(cx, cy, size, angle)

    for offset_hundredths in range(-100, 101):  # ±1° in 0.01° steps
        test_angle = angle + offset_hundredths * 0.01
        matches = test_params(cx, cy, size, test_angle)
        if matches > best_matches:
            best_matches = matches
            best_angle = test_angle

    if best_angle != angle:
        angle = best_angle
        print(f"  angle improved to {angle:.4f}° ({100*best_matches/576:.3f}%)")

    # Check if we improved
    current_matches = test_params(cx, cy, size, angle)
    if current_matches == best_matches and best_matches == initial_matches:
        print(f"  No improvement in pass {iteration + 1}, stopping")
        break

    initial_matches = current_matches

print()
print("=" * 70)
print(f"Final best parameters:")
print(f"  cx     = {cx:.6f}  (8.8 = {int(cx * 256)})")
print(f"  cy     = {cy:.6f}  (8.8 = {int(cy * 256)})")
print(f"  size   = {size:.6f}  (8.8 = {int(size * 256)})")
print(f"  angle  = {angle:.4f}°")

final_matches = test_params(cx, cy, size, angle)
match_pct = 100 * final_matches / 576
print(f"  Match: {match_pct:.3f}% ({final_matches}/576)")
print(f"  Diff:  {576 - final_matches} pixels")

if final_matches == 576:
    print("\n✓ PERFECT 100% MATCH!")
else:
    print(f"\nNot perfect - patterns are likely pre-rendered bitmaps")

# Save params for visualization
with open("best_params_frame324.txt", "w") as f:
    f.write(f"{target_frame} {cx} {cy} {size} {angle}\n")

print(f"\nSaved to best_params_frame324.txt")
