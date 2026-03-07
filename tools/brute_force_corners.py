#!/usr/bin/env python3
"""
Brute force search for exact corner coordinates (integer or half-integer).
Try all combinations to find 100% matches.
"""

import re
from typing import List, Tuple

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

def render_from_corners(corners: List[Tuple[float, float]], width: int = 24, height: int = 24) -> List[List[bool]]:
    """Render from 4 corner coordinates"""
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

            x1 = max(0, x1)
            x2 = min(width - 1, x2)
            for x in range(x1, x2 + 1):
                bitmap[y][x] = True

    return bitmap

def parse_frame(lines: List[str]) -> List[List[bool]]:
    bitmap = []
    for line in lines:
        dots = line.count("·")
        blocks = line.count("█")
        if dots + blocks == 24:
            row = []
            for char in line:
                if char == "█":
                    row.append(True)
                elif char == "·":
                    row.append(False)
            if len(row) == 24:
                bitmap.append(row)
    return bitmap

def compare_bitmaps(a: List[List[bool]], b: List[List[bool]]) -> int:
    matches = 0
    for y in range(len(a)):
        for x in range(len(a[y])):
            if a[y][x] == b[y][x]:
                matches += 1
    return matches

# Load frame 324 (typical case with 7 pixel difference)
with open("../emulator/frames/animation_318-336.txt", "r") as f:
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
            target_bitmap = parse_frame(current_frame_lines)
            break
        current_frame_num = int(match.group(1))
        current_frame_lines = [line]
    else:
        current_frame_lines.append(line)

if current_frame_num == target_frame and current_frame_lines and not target_bitmap:
    target_bitmap = parse_frame(current_frame_lines)

if target_bitmap and len(target_bitmap) == 24:
    print(f"Brute forcing corner positions for Frame {target_frame}")
    print("=" * 60)
    print("Testing half-integer grid positions...")

    # Generate half-integer positions (0, 0.5, 1, 1.5, ...)
    positions = [i * 0.5 for i in range(0, 48)]  # 0 to 23.5

    best_match = 0
    best_corners = None

    # Try all combinations of 4 corners
    # Constrain search to reasonable bounds
    import itertools

    # For a square-ish shape, corners should be in certain ranges
    # Based on bounding box (4, 3) to (19, 19), try around those areas

    x_range = [i * 0.5 for i in range(0, 48)]  # 0 to 23.5
    y_range = [i * 0.5 for i in range(0, 48)]

    # Constrain to smaller search space around expected values
    x_range = [i * 0.5 for i in range(6, 42)]  # 3 to 20.5
    y_range = [i * 0.5 for i in range(4, 42)]  # 2 to 20.5

    total_tests = 0
    print(f"Search space: {len(x_range)} x positions × {len(y_range)} y positions")
    print(f"This will test many combinations - may take a while...")
    print()

    # Smarter approach: For a square, if we know one corner and the angle,
    # we can derive the other corners. So search over:
    # - One corner position (x, y)
    # - Square side length
    # - Rotation angle

    # Even smarter: search for axis-aligned rectangles first (simpler)
    print("Testing axis-aligned rectangles...")

    for x1 in x_range[::2]:  # Step by 1.0
        for y1 in y_range[::2]:
            for x2 in x_range[::2]:
                if x2 <= x1:
                    continue
                for y2 in y_range[::2]:
                    if y2 <= y1:
                        continue

                    # Rectangle corners
                    corners = [
                        (x1, y1), (x2, y1),
                        (x2, y2), (x1, y2)
                    ]

                    rendered = render_from_corners(corners, 24, 24)
                    matches = compare_bitmaps(target_bitmap, rendered)

                    total_tests += 1
                    if matches > best_match:
                        best_match = matches
                        best_corners = corners

                        match_pct = 100 * matches / 576
                        if match_pct > 99.5:
                            print(f"  {match_pct:.2f}%: corners={corners}")

                        if matches == 576:
                            print(f"\n✓ PERFECT MATCH!")
                            print(f"  Corners: {corners}")
                            print(f"  Total tests: {total_tests}")
                            break

                if best_match == 576:
                    break
            if best_match == 576:
                break
        if best_match == 576:
            break

    if best_match < 576:
        match_pct = 100 * best_match / 576
        print(f"\nBest axis-aligned: {match_pct:.2f}% ({best_match}/576)")
        print(f"  Corners: {best_corners}")
        print(f"  Total tests: {total_tests}")
        print(f"\nAxis-aligned rectangles don't give 100%. The square must be rotated.")
else:
    print(f"Failed to load frame {target_frame}")
