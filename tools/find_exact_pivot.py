#!/usr/bin/env python3
"""
Find the exact pivot point for the rotating square.
We know Y≈17 is stable, now find the X coordinate.
"""

import math

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

def find_center_of_mass(pixels):
    """Find center of mass of the pattern (handling wrapping)"""
    # First unwrap if needed
    left_pixels = sum(1 for row in pixels for x, p in enumerate(row) if p and x < 4)
    right_pixels = sum(1 for row in pixels for x, p in enumerate(row) if p and x >= 20)

    if left_pixels > 0 and right_pixels > 0:
        # Pattern is wrapped, unwrap it by rolling
        # Find best roll offset
        best_roll = 0
        min_width = 24

        for roll in range(24):
            min_x, max_x = 24, -1
            for y, row in enumerate(pixels):
                for x in range(24):
                    src_x = (x - roll) % 24
                    if row[src_x]:
                        min_x = min(min_x, x)
                        max_x = max(max_x, x)

            if min_x <= max_x:
                width = max_x - min_x + 1
                if width < min_width:
                    min_width = width
                    best_roll = roll

        # Apply roll
        unwrapped = []
        for row in pixels:
            new_row = []
            for x in range(24):
                src_x = (x - best_roll) % 24
                new_row.append(row[src_x])
            unwrapped.append(new_row)

        pixels = unwrapped

    # Calculate center of mass
    total_pixels = 0
    sum_x = 0
    sum_y = 0

    for y, row in enumerate(pixels):
        for x, pixel in enumerate(row):
            if pixel:
                total_pixels += 1
                sum_x += x
                sum_y += y

    if total_pixels == 0:
        return None, None

    return sum_x / total_pixels, sum_y / total_pixels

# Load all raw patterns
frames = [318, 320, 322, 324, 326, 328, 330, 332, 334, 336]
patterns = {}

print("Finding exact pivot point")
print("=" * 70)

for frame in frames:
    filename = f"../emulator/frames/raw_frame{frame}.txt"
    pixels = load_raw_pattern(filename)
    patterns[frame] = pixels

# Test various pivot candidates
print("\nTesting pivot candidates...")
print("(Looking for stable distance from pivot to center of mass)")

best_pivot = None
best_variance = float('inf')

# Test grid of positions
for pivot_x in range(24):
    for pivot_y in range(24):
        distances = []

        for frame in frames:
            pixels = patterns[frame]
            cx, cy = find_center_of_mass(pixels)

            if cx is None:
                continue

            # Calculate distance (handling X wrapping)
            dx1 = abs(cx - pivot_x)
            dx2 = 24 - dx1  # Distance via wraparound
            dx = min(dx1, dx2)

            dy = cy - pivot_y
            dist = math.sqrt(dx**2 + dy**2)
            distances.append(dist)

        if len(distances) < len(frames):
            continue

        # Calculate variance
        mean_dist = sum(distances) / len(distances)
        variance = sum((d - mean_dist)**2 for d in distances) / len(distances)
        std_dev = math.sqrt(variance)

        if std_dev < best_variance:
            best_variance = std_dev
            best_pivot = (pivot_x, pivot_y)

            if std_dev < 0.5:  # Very stable
                print(f"  ({pivot_x:2d}, {pivot_y:2d}): distance = {mean_dist:.2f} ± {std_dev:.3f}")

print(f"\n✓ Best pivot candidate: {best_pivot} (variance: {best_variance:.3f})")

# Test with the best pivot
pivot_x, pivot_y = best_pivot
print(f"\nDistance from pivot ({pivot_x}, {pivot_y}) to each frame:")

for frame in frames:
    pixels = patterns[frame]
    cx, cy = find_center_of_mass(pixels)

    if cx is None:
        continue

    # Calculate distance (handling X wrapping)
    dx1 = abs(cx - pivot_x)
    dx2 = 24 - dx1
    dx = min(dx1, dx2)

    dy = cy - pivot_y
    dist = math.sqrt(dx**2 + dy**2)

    print(f"  Frame {frame}: center=({cx:5.1f}, {cy:4.1f}), distance={dist:.3f}")

# Also check specific corner positions
print("\n" + "=" * 70)
print("Testing specific bottom corners as pivots:")

for test_pivot in [(0, 17), (6, 17), (12, 17), (17, 17), (23, 17), (11, 17)]:
    pivot_x, pivot_y = test_pivot
    distances = []

    for frame in frames:
        pixels = patterns[frame]
        cx, cy = find_center_of_mass(pixels)

        if cx is None:
            continue

        dx1 = abs(cx - pivot_x)
        dx2 = 24 - dx1
        dx = min(dx1, dx2)

        dy = cy - pivot_y
        dist = math.sqrt(dx**2 + dy**2)
        distances.append(dist)

    mean_dist = sum(distances) / len(distances)
    variance = sum((d - mean_dist)**2 for d in distances) / len(distances)
    std_dev = math.sqrt(variance)

    print(f"  Pivot ({pivot_x:2d}, {pivot_y:2d}): {mean_dist:.2f} ± {std_dev:.3f}")
