#!/usr/bin/env python3
"""
Test if the square rotates around pivot (18, 15) with X wrapping.
"""

import math

class FixedPoint:
    SHIFT = 16
    SCALE = 1 << SHIFT

    @staticmethod
    def from_float(f: float) -> int:
        return int(f * FixedPoint.SCALE)

    @staticmethod
    def multiply(a: int, b: int) -> int:
        return (a * b) >> FixedPoint.SHIFT

class Edge:
    def __init__(self, x1: float, y1: float, x2: float, y2: float):
        if y1 > y2:
            x1, y1, x2, y2 = x2, y2, x1, y1

        self.y_start = y1
        self.y_end = y2
        self.x_start = x1

        dy = y2 - y1
        if dy > 0.001:
            self.dx_dy = FixedPoint.from_float((x2 - x1) / dy)
        else:
            self.dx_dy = 0

    def get_x_at_y(self, y: float) -> float:
        if self.y_end - self.y_start < 0.001:
            return self.x_start

        dy = y - self.y_start
        dx = FixedPoint.multiply(FixedPoint.from_float(dy), self.dx_dy)
        x = FixedPoint.from_float(self.x_start) + dx
        return x / FixedPoint.SCALE

def render_square_around_pivot(pivot_x: float, pivot_y: float, size: float, angle: float,
                                width: int = 24, height: int = 24) -> list:
    """
    Render square rotating around a fixed pivot point with X wrapping.

    Args:
        pivot_x, pivot_y: Fixed pivot point (one corner of the square)
        size: Side length of the square
        angle: Rotation angle in radians
        width, height: Output dimensions
    """
    # Calculate 4 corners relative to pivot
    # Pivot is at one corner, others are relative to it
    corners = []

    # Corner 0 is the pivot itself
    corners.append((pivot_x, pivot_y))

    # Other corners form a square from the pivot
    # Corner 1: along first edge
    x1 = pivot_x + size * math.cos(angle)
    y1 = pivot_y + size * math.sin(angle)
    corners.append((x1, y1))

    # Corner 2: opposite diagonal from pivot
    x2 = pivot_x + size * math.cos(angle) + size * math.cos(angle + math.pi/2)
    y2 = pivot_y + size * math.sin(angle) + size * math.sin(angle + math.pi/2)
    corners.append((x2, y2))

    # Corner 3: along second edge
    x3 = pivot_x + size * math.cos(angle + math.pi/2)
    y3 = pivot_y + size * math.sin(angle + math.pi/2)
    corners.append((x3, y3))

    # Create edges
    edges = []
    for i in range(4):
        x1, y1 = corners[i]
        x2, y2 = corners[(i + 1) % 4]
        edges.append(Edge(x1, y1, x2, y2))

    # Render with X wrapping
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

            # Fill with X wrapping (modulo 24)
            x1_int = int(x1)
            x2_int = int(x2)

            # Check if span wraps around
            if x2_int - x1_int > width:
                # Wraps around - fill two segments
                for x in range(int(x1), width):
                    bitmap[y][x % width] = True
                for x in range(0, int(x2 % width) + 1):
                    bitmap[y][x] = True
            else:
                # Normal span
                for x in range(x1_int, x2_int + 1):
                    bitmap[y][x % width] = True

    return bitmap

def load_raw_pattern(filename):
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

def compare_bitmaps(a: list, b: list) -> int:
    matches = 0
    for y in range(len(a)):
        for x in range(len(a[y])):
            if a[y][x] == b[y][x]:
                matches += 1
    return matches

def bitmap_to_string(bitmap: list) -> str:
    lines = []
    for row in bitmap:
        line = ""
        for pixel in row:
            line += "█" if pixel else "·"
        lines.append(line)
    return "\n".join(lines)

# Test with pivot at (18, 15)
frames = [318, 320, 322, 324, 326, 328, 330, 332, 334, 336]

print("Testing square rotating around pivot (18, 15)")
print("=" * 70)

for frame in frames:
    filename = f"../emulator/frames/raw_frame{frame}.txt"
    original = load_raw_pattern(filename)

    # Try different sizes and angles
    best_match = 0
    best_params = None

    for size in [i * 0.5 for i in range(16, 28)]:  # 8.0 to 13.5
        for angle_deg in range(-180, 180, 2):
            angle_rad = math.radians(angle_deg)

            rendered = render_square_around_pivot(18, 15, size, angle_rad, 24, 24)
            matches = compare_bitmaps(original, rendered)

            if matches > best_match:
                best_match = matches
                best_params = (size, angle_deg)

    match_pct = 100 * best_match / 576
    size, angle = best_params

    print(f"Frame {frame}: {match_pct:.2f}% (size={size:.1f}, angle={angle:4d}°)")

    if match_pct == 100.0:
        print(f"  ✓ PERFECT MATCH!")

        # Show it
        rendered = render_square_around_pivot(18, 15, size, math.radians(angle), 24, 24)
        print("\nOriginal:")
        print(bitmap_to_string(original))
        print("\nRendered:")
        print(bitmap_to_string(rendered))
        print()
