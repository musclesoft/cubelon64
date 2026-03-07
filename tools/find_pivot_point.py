#!/usr/bin/env python3
"""
Test if the square rotates around a fixed corner pivot point.
"""

import math
import re
from rotating_square import parse_frame, analyze_frame_geometry

# Load animation file
with open("../emulator/frames/animation_318-336.txt", "r") as f:
    content = f.read()

frame_pattern = re.compile(r'=== Frame (\d+)')
lines = content.split('\n')
current_frame_lines = []
current_frame_num = None

frames_to_analyze = [320, 322, 324, 326, 328, 330, 332, 334, 336]
frame_data = {}

print("Extracting frame geometries...")
print("=" * 60)

for line in lines:
    match = frame_pattern.search(line)
    if match:
        if current_frame_num in frames_to_analyze and current_frame_lines:
            bitmap, _ = parse_frame(current_frame_lines)
            if bitmap and len(bitmap) == 24:
                geo = analyze_frame_geometry(bitmap)
                if geo:
                    frame_data[current_frame_num] = {
                        'bitmap': bitmap,
                        'geo': geo
                    }

        current_frame_num = int(match.group(1))
        current_frame_lines = [line]
    else:
        current_frame_lines.append(line)

# Process last frame
if current_frame_num in frames_to_analyze and current_frame_lines:
    bitmap, _ = parse_frame(current_frame_lines)
    if bitmap and len(bitmap) == 24:
        geo = analyze_frame_geometry(bitmap)
        if geo:
            frame_data[current_frame_num] = {
                'bitmap': bitmap,
                'geo': geo
            }

# Analyze each frame to find corner positions
print("\nFrame geometries:")
for frame_num in sorted(frame_data.keys()):
    geo = frame_data[frame_num]['geo']
    bbox = geo['bbox']
    print(f"Frame {frame_num}:")
    print(f"  Bounding box: ({bbox[0]}, {bbox[1]}) to ({bbox[2]}, {bbox[3]})")
    print(f"  Center of mass: ({geo['center_x']:.2f}, {geo['center_y']:.2f})")

    # Calculate approximate corners from bounding box
    # These are the extreme pixels, not exact geometric corners
    corners = [
        (bbox[0], bbox[1]),  # Top-left extreme
        (bbox[2], bbox[1]),  # Top-right extreme
        (bbox[2], bbox[3]),  # Bottom-right extreme
        (bbox[0], bbox[3]),  # Bottom-left extreme
    ]
    print(f"  Extreme corners: TL={corners[0]}, TR={corners[1]}, BR={corners[2]}, BL={corners[3]}")

# Check if any corner stays relatively fixed
print("\n" + "=" * 60)
print("Checking for fixed pivot point...")

# Test common pivot candidates
pivot_candidates = [
    (6, 6), (6, 17), (17, 6), (17, 17),  # Bounding box corners
    (5, 5), (5, 18), (18, 5), (18, 18),  # Slightly outside
    (4, 4), (4, 19), (19, 4), (19, 19),  # Further outside
    (12, 12), (11, 11), (12, 11), (11, 12),  # Near center
]

for pivot_x, pivot_y in pivot_candidates:
    # Calculate distance from pivot to center of mass for each frame
    distances = []
    for frame_num in sorted(frame_data.keys()):
        geo = frame_data[frame_num]['geo']
        cx, cy = geo['center_x'], geo['center_y']
        dist = math.sqrt((cx - pivot_x)**2 + (cy - pivot_y)**2)
        distances.append(dist)

    # If rotating around a corner, distance should be relatively constant
    # (it's the distance from pivot to geometric center)
    mean_dist = sum(distances) / len(distances)
    variance = sum((d - mean_dist)**2 for d in distances) / len(distances)
    std_dev = math.sqrt(variance)

    if std_dev < 1.0:  # Low variance suggests fixed pivot
        print(f"\nCandidate pivot: ({pivot_x}, {pivot_y})")
        print(f"  Distance to centers: {mean_dist:.2f} ± {std_dev:.2f}")
        print(f"  Distances: {[f'{d:.2f}' for d in distances]}")

# Also check lower corners specifically
print("\n" + "=" * 60)
print("Checking lower corners as pivot candidates...")

# For each frame, find the lowest pixels
for frame_num in sorted(frame_data.keys()):
    bitmap = frame_data[frame_num]['bitmap']

    # Find lowest row with pixels
    lowest_y = -1
    lowest_pixels = []
    for y in range(23, -1, -1):
        row_has_pixel = any(bitmap[y])
        if row_has_pixel:
            lowest_y = y
            # Find all pixels in this row
            for x in range(24):
                if bitmap[y][x]:
                    lowest_pixels.append((x, y))
            break

    print(f"Frame {frame_num}: lowest pixels at y={lowest_y}: {lowest_pixels[:5]}")
