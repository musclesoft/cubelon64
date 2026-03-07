#!/usr/bin/env python3
"""
Find if any specific pixel position stays lit (or stays in same state) across all frames.
This would be the true pivot corner.
"""

def load_raw_pattern(filename):
    """Load a raw pattern file"""
    pixels = []
    with open(filename, 'r') as f:
        for line in f:
            if line.startswith('=') or not line.strip():
                continue
            row = []
            for char in line.strip():
                if char == '█':
                    row.append(True)
                elif char == '·':
                    row.append(False)
            if len(row) == 24:
                pixels.append(row)
    return pixels

# Load all raw patterns
frames = [318, 320, 322, 324, 326, 328, 330, 332, 334, 336]
patterns = {}

print("Finding pixels that stay lit across all frames")
print("=" * 70)

for frame in frames:
    filename = f"../emulator/frames/raw_frame{frame}.txt"
    patterns[frame] = load_raw_pattern(filename)

# Find pixels that are always lit
always_lit = []
for y in range(24):
    for x in range(24):
        lit_in_all = all(patterns[f][y][x] for f in frames)
        if lit_in_all:
            always_lit.append((x, y))

print(f"\nPixels always lit: {always_lit}")

# Find pixels that are lit in most frames
lit_counts = {}
for y in range(24):
    for x in range(24):
        count = sum(1 for f in frames if patterns[f][y][x])
        if count > 0:
            lit_counts[(x, y)] = count

# Sort by count
sorted_pixels = sorted(lit_counts.items(), key=lambda x: x[1], reverse=True)

print(f"\nPixels lit in most frames (top 20):")
for (x, y), count in sorted_pixels[:20]:
    print(f"  ({x:2d}, {y:2d}): {count}/{len(frames)} frames")

# Check the bottom edge more carefully
print("\n" + "=" * 70)
print("Bottom edge pixels (Y=16-18):")

for y in [16, 17, 18]:
    lit_pixels = []
    for x in range(24):
        count = sum(1 for f in frames if patterns[f][y][x])
        if count >= len(frames) // 2:  # Lit in at least half the frames
            lit_pixels.append((x, count))

    print(f"\nY={y}:")
    for x, count in lit_pixels:
        print(f"  X={x:2d}: {count}/{len(frames)} frames")

# Look at corners of each frame
print("\n" + "=" * 70)
print("Checking actual geometric corners (not just edge pixels):")

for frame in frames:
    pixels = patterns[frame]

    # Find bounding box
    min_x, max_x = 24, -1
    min_y, max_y = 24, -1

    for y, row in enumerate(pixels):
        for x, pixel in enumerate(row):
            if pixel:
                min_x = min(min_x, x)
                max_x = max(max_x, x)
                min_y = min(min_y, y)
                max_y = max(max_y, y)

    # Check if wrapped
    left = sum(1 for row in pixels for x, p in enumerate(row) if p and x < 4)
    right = sum(1 for row in pixels for x, p in enumerate(row) if p and x >= 20)
    wrapped = left > 0 and right > 0

    print(f"Frame {frame}: bbox=({min_x:2d},{min_y:2d}) to ({max_x:2d},{max_y:2d})", end="")
    if wrapped:
        print(" [WRAPPED - corners ambiguous]")
    else:
        # For non-wrapped, show actual corners
        corners = [(min_x, min_y), (max_x, min_y), (max_x, max_y), (min_x, max_y)]
        print(f" corners={corners}")

# Key insight: Maybe the pivot is BELOW the visible area?
print("\n" + "=" * 70)
print("Hypothesis: Pivot might be at Y > 17 (below visible square)")
print("\nTesting virtual pivot at (11, 20)...")

import math

pivot_x, pivot_y = 11, 20

for frame in frames:
    pixels = patterns[frame]

    # Find center of mass (without unwrapping for now)
    total = 0
    sum_x = 0
    sum_y = 0

    for y, row in enumerate(pixels):
        for x, pixel in enumerate(row):
            if pixel:
                total += 1
                sum_x += x
                sum_y += y

    if total > 0:
        cx = sum_x / total
        cy = sum_y / total

        # Distance to pivot
        dx = cx - pivot_x
        dy = cy - pivot_y
        dist = math.sqrt(dx**2 + dy**2)

        print(f"  Frame {frame}: center=({cx:5.1f}, {cy:5.1f}), distance={dist:.3f}")
