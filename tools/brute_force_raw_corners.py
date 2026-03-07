#!/usr/bin/env python3
"""
Brute force search for exact integer/half-integer corner coordinates
that match the RAW (uncentered) patterns.
"""

from test_pivot_18_15 import render_square_around_pivot, load_raw_pattern, compare_bitmaps, bitmap_to_string

# Pick frame 336 (horizontal bars) - should be easiest
frame = 336
filename = f"../emulator/frames/raw_frame{frame}.txt"
original = load_raw_pattern(filename)

print(f"Brute force search for Frame {frame} (raw pattern)")
print("=" * 70)
print("Original pattern:")
print(bitmap_to_string(original))
print()

# Since (18, 15) is always lit, try it and nearby pixels as pivot
# Also try the 4-corner direct approach

from typing import List, Tuple
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

    def get_x_at_y(self, y: float) -> int:
        if self.y_end - self.y_start < 0.001:
            return int(self.x_start)

        dy = y - self.y_start
        dx = FixedPoint.multiply(FixedPoint.from_float(dy), self.dx_dy)
        x = FixedPoint.from_float(self.x_start) + dx
        return x >> FixedPoint.SHIFT

def render_from_corners_wrapped(corners: List[Tuple[float, float]], width: int = 24, height: int = 24) -> List[List[bool]]:
    """
    Render directly from 4 corner coordinates with X wrapping.
    """
    edges = []
    for i in range(4):
        x1, y1 = corners[i]
        x2, y2 = corners[(i + 1) % 4]
        edges.append(Edge(x1, y1, x2, y2))

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

            # Fill with wrapping
            if x2 - x1 < width:
                # Normal fill
                for x in range(x1, x2 + 1):
                    bitmap[y][x % width] = True
            else:
                # Wrapped fill - this shouldn't happen with correct corners
                for x in range(x1, x2 + 1):
                    bitmap[y][x % width] = True

    return bitmap

# Try all combinations of integer corner positions
# Frame 336 is horizontal bars, so should have 2 horizontal edges
# Based on raw pattern: Y range is 6-17, X wraps at 0-5 and 18-23

print("Testing integer corner combinations...")
print("(Frame 336 should be nearly square/horizontal)")
print()

best_match = 0
best_corners = None

# For horizontal bars that wrap, we expect something like:
# Top-left around (0, 6), Top-right around (23, 6)
# Bottom-left around (0, 17), Bottom-right around (23, 17)

# Test with half-integer precision
positions = [i * 0.5 for i in range(0, 48)]  # 0, 0.5, 1.0, ..., 23.5

# Constrain search based on observed bounding box
x_candidates = [i * 0.5 for i in range(0, 48)]
y_top_candidates = [i * 0.5 for i in range(10, 16)]  # Y around 5-7
y_bottom_candidates = [i * 0.5 for i in range(32, 38)]  # Y around 16-18

tested = 0
for y_top in y_top_candidates:
    for y_bottom in y_bottom_candidates:
        # For a square that wraps, try various X positions
        # Left edge positions
        for x_left1 in range(0, 8):
            for x_left2 in range(0, 8):
                # Right edge positions
                for x_right1 in range(16, 24):
                    for x_right2 in range(16, 24):
                        # Form rectangle
                        corners = [
                            (x_left1, y_top),
                            (x_right1, y_top),
                            (x_right2, y_bottom),
                            (x_left2, y_bottom),
                        ]

                        rendered = render_from_corners_wrapped(corners, 24, 24)
                        matches = compare_bitmaps(original, rendered)

                        tested += 1

                        if matches > best_match:
                            best_match = matches
                            best_corners = corners

                            match_pct = 100 * matches / 576
                            if match_pct > 99.5:
                                print(f"  {match_pct:.2f}%: corners={corners}")

                            if matches == 576:
                                print(f"\n✓ PERFECT MATCH!")
                                print(f"  Corners: {corners}")
                                print(f"  Tested {tested} combinations")
                                break

                    if best_match == 576:
                        break
                if best_match == 576:
                    break
            if best_match == 576:
                break
        if best_match == 576:
            break
    if best_match == 576:
        break

match_pct = 100 * best_match / 576
print(f"\nBest match: {match_pct:.2f}% ({best_match}/576)")
print(f"  Corners: {best_corners}")
print(f"  Tested: {tested} combinations")

if best_match == 576:
    print("\nRendered:")
    rendered = render_from_corners_wrapped(best_corners, 24, 24)
    print(bitmap_to_string(rendered))
