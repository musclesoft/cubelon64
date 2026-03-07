#!/usr/bin/env python3
"""
Test with 8.8 fixed-point precision (8-bit appropriate).
"""

import math
from typing import List, Tuple

class FixedPoint88:
    """8.8 fixed-point (8-bit friendly)"""
    SHIFT = 8
    SCALE = 1 << SHIFT

    @staticmethod
    def from_float(f: float) -> int:
        return int(f * FixedPoint88.SCALE)

    @staticmethod
    def multiply(a: int, b: int) -> int:
        return (a * b) >> FixedPoint88.SHIFT

class Edge88:
    def __init__(self, x1: float, y1: float, x2: float, y2: float):
        if y1 > y2:
            x1, y1, x2, y2 = x2, y2, x1, y1

        self.y_start = y1
        self.y_end = y2
        self.x_start = x1

        dy = y2 - y1
        if dy > 0.001:
            self.dx_dy = FixedPoint88.from_float((x2 - x1) / dy)
        else:
            self.dx_dy = 0

    def get_x_at_y(self, y: float) -> int:
        if self.y_end - self.y_start < 0.001:
            return int(self.x_start)

        dy = y - self.y_start
        dx = FixedPoint88.multiply(FixedPoint88.from_float(dy), self.dx_dy)
        x = FixedPoint88.from_float(self.x_start) + dx
        return x >> FixedPoint88.SHIFT

def render_square_8bit(center_x: float, center_y: float, size: float, angle: float,
                       width: int = 24, height: int = 24) -> List[List[bool]]:
    """Render with 8.8 fixed-point precision"""
    # Calculate 4 corners
    corners = []
    for i in range(4):
        corner_angle = angle + (i * math.pi / 2)
        x = center_x + size * math.cos(corner_angle)
        y = center_y + size * math.sin(corner_angle)
        corners.append((x, y))

    # Create edges
    edges = []
    for i in range(4):
        x1, y1 = corners[i]
        x2, y2 = corners[(i + 1) % 4]
        edges.append(Edge88(x1, y1, x2, y2))

    bitmap = [[False] * width for _ in range(height)]

    y_min = int(max(0, min(edge.y_start for edge in edges)))
    y_max = int(min(height, max(edge.y_end for edge in edges) + 1))

    for y in range(y_min, y_max):
        y_float = float(y)

        x_intersections = []
        for edge in edges:
            if edge.y_start <= y_float < edge.y_end:
                x = edge.get_x_at_y(y_float)
                x_intersections.append(x)

        x_intersections.sort()

        if len(x_intersections) >= 2:
            x1 = x_intersections[0]
            x2 = x_intersections[-1]

            x1 = max(0, x1)
            x2 = min(width - 1, x2)
            for x in range(x1, x2 + 1):
                bitmap[y][x] = True

    return bitmap

def load_centered_pattern(filename):
    """Load CENTERED pattern (not raw)"""
    pixels = []
    with open(filename, 'r') as f:
        in_frame = False
        for line in f:
            if '===' in line and 'Frame' in line:
                in_frame = True
                continue
            if not in_frame:
                continue

            row = []
            for char in line:
                if char == '█':
                    row.append(True)
                elif char == '·':
                    row.append(False)

            if len(row) == 24:
                pixels.append(row)

            if len(pixels) == 24:
                break

    return pixels

def compare_bitmaps(a: List[List[bool]], b: List[List[bool]]) -> int:
    matches = 0
    for y in range(24):
        for x in range(24):
            if a[y][x] == b[y][x]:
                matches += 1
    return matches

# Test with CENTERED patterns (like animation_318-336.txt)
print("Testing with 8.8 fixed-point precision")
print("=" * 70)

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

with open("../emulator/frames/animation_318-336.txt", 'r') as f:
    content = f.read()

for frame, (cx, cy, size, angle_deg) in frames_to_test.items():
    # Extract pattern from file
    frame_start = content.find(f"=== Frame {frame}")
    if frame_start == -1:
        continue

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

    if len(original) != 24:
        continue

    # Render with 8.8 precision
    rendered = render_square_8bit(cx, cy, size, math.radians(angle_deg), 24, 24)
    matches = compare_bitmaps(original, rendered)
    match_pct = 100 * matches / 576

    print(f"Frame {frame}: {match_pct:.2f}% (was {match_pct:.2f}% with 16.16)")

    if match_pct == 100.0:
        print(f"  ✓ PERFECT MATCH with 8.8!")
