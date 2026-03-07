#!/usr/bin/env python3
"""
Analyze raw uncentered patterns to find the rotation pivot point.
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

def analyze_pattern(pixels):
    """Find bounding box and corners"""
    min_x, max_x = 24, -1
    min_y, max_y = 24, -1

    for y, row in enumerate(pixels):
        for x, pixel in enumerate(row):
            if pixel:
                min_x = min(min_x, x)
                max_x = max(max_x, x)
                min_y = min(min_y, y)
                max_y = max(max_y, y)

    # Check for wrapping
    left_edge = sum(1 for row in pixels for x, p in enumerate(row) if p and x < 3)
    right_edge = sum(1 for row in pixels for x, p in enumerate(row) if p and x >= 21)

    # Find corner pixels
    corners = {
        'TL': None,  # Top-left
        'TR': None,  # Top-right
        'BL': None,  # Bottom-left
        'BR': None,  # Bottom-right
    }

    # Top-left: first pixel from top-left
    for y in range(24):
        for x in range(24):
            if pixels[y][x]:
                corners['TL'] = (x, y)
                break
        if corners['TL']:
            break

    # Top-right: first pixel from top-right
    for y in range(24):
        for x in range(23, -1, -1):
            if pixels[y][x]:
                corners['TR'] = (x, y)
                break
        if corners['TR']:
            break

    # Bottom-left: first pixel from bottom-left
    for y in range(23, -1, -1):
        for x in range(24):
            if pixels[y][x]:
                corners['BL'] = (x, y)
                break
        if corners['BL']:
            break

    # Bottom-right: first pixel from bottom-right
    for y in range(23, -1, -1):
        for x in range(23, -1, -1):
            if pixels[y][x]:
                corners['BR'] = (x, y)
                break
        if corners['BR']:
            break

    return {
        'bbox': (min_x, min_y, max_x, max_y),
        'wrapped': left_edge > 0 and right_edge > 0,
        'corners': corners
    }

# Load all raw patterns
frames = [318, 320, 322, 324, 326, 328, 330, 332, 334, 336]
patterns = {}

print("Analyzing raw uncentered patterns")
print("=" * 70)

for frame in frames:
    filename = f"../emulator/frames/raw_frame{frame}.txt"
    pixels = load_raw_pattern(filename)
    info = analyze_pattern(pixels)
    patterns[frame] = {'pixels': pixels, 'info': info}

    bbox = info['bbox']
    corners = info['corners']
    wrapped = " (WRAPPED)" if info['wrapped'] else ""

    print(f"\nFrame {frame}{wrapped}:")
    print(f"  Bounding box: ({bbox[0]:2d},{bbox[1]:2d}) to ({bbox[2]:2d},{bbox[3]:2d})")
    print(f"  Corners: TL={corners['TL']}, TR={corners['TR']}, BL={corners['BL']}, BR={corners['BR']}")

# Check which corners stay relatively fixed
print("\n" + "=" * 70)
print("Checking for fixed corner pivot...")

# For each corner position, check variance across frames
for corner_name in ['TL', 'TR', 'BL', 'BR']:
    corners = [patterns[f]['info']['corners'][corner_name] for f in frames if patterns[f]['info']['corners'][corner_name]]

    if not corners:
        continue

    xs = [c[0] for c in corners]
    ys = [c[1] for c in corners]

    mean_x = sum(xs) / len(xs)
    mean_y = sum(ys) / len(ys)

    var_x = sum((x - mean_x)**2 for x in xs) / len(xs)
    var_y = sum((y - mean_y)**2 for y in ys) / len(ys)

    std_x = math.sqrt(var_x)
    std_y = math.sqrt(var_y)

    print(f"\n{corner_name} corner:")
    print(f"  X: {mean_x:.1f} ± {std_x:.1f}  (range: {min(xs)}-{max(xs)})")
    print(f"  Y: {mean_y:.1f} ± {std_y:.1f}  (range: {min(ys)}-{max(ys)})")
    print(f"  Positions: {corners}")

    if std_x < 2.0 and std_y < 2.0:
        print(f"  ⭐ LOW VARIANCE - possible pivot!")

# Check specifically for wrapped patterns
print("\n" + "=" * 70)
print("Wrapped patterns analysis:")

wrapped_frames = [f for f in frames if patterns[f]['info']['wrapped']]
print(f"\nWrapped frames: {wrapped_frames}")

for frame in wrapped_frames:
    print(f"\nFrame {frame} (raw, wrapped):")
    for y, row in enumerate(patterns[frame]['pixels']):
        line = ""
        for x, pixel in enumerate(row):
            line += "█" if pixel else "·"
        print(f"  {line}")
