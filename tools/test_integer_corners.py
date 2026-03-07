#!/usr/bin/env python3
"""
Test if the original algorithm uses integer-only corner coordinates.
Brute force search for integer corner positions that match perfectly.
"""

import math
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

def render_from_corners(corners: List[Tuple[int, int]], width: int = 24, height: int = 24) -> List[List[bool]]:
    """Render directly from integer corner coordinates"""
    edges = []
    for i in range(4):
        x1, y1 = corners[i]
        x2, y2 = corners[(i + 1) % 4]
        edges.append(Edge(float(x1), float(y1), float(x2), float(y2)))

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

def parse_frame(lines: List[str]) -> Tuple[List[List[bool]], int]:
    frame_num = None
    for line in lines:
        if "Frame" in line and "(" in line:
            parts = line.split()
            for i, part in enumerate(parts):
                if i > 0 and "Frame" in parts[i-1]:
                    try:
                        frame_num = int(part)
                        break
                    except ValueError:
                        pass

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

    return bitmap, frame_num

def compare_bitmaps(a: List[List[bool]], b: List[List[bool]]) -> int:
    matches = 0
    for y in range(len(a)):
        for x in range(len(a[y])):
            if a[y][x] == b[y][x]:
                matches += 1
    return matches

# Load frame 336 (the perfect match)
with open("../emulator/frames/animation_318-336.txt", "r") as f:
    content = f.read()

frame_pattern = re.compile(r'=== Frame (\d+)')
lines = content.split('\n')
current_frame_lines = []
current_frame_num = None

target_frame = 336
target_bitmap = None

for line in lines:
    match = frame_pattern.search(line)
    if match:
        # Process previous frame
        if current_frame_num == target_frame and current_frame_lines:
            target_bitmap, _ = parse_frame(current_frame_lines)
            break
        # Start new frame
        current_frame_num = int(match.group(1))
        current_frame_lines = [line]
    else:
        current_frame_lines.append(line)

# Don't forget the last frame
if current_frame_num == target_frame and current_frame_lines and not target_bitmap:
    target_bitmap, _ = parse_frame(current_frame_lines)

print(f"Loaded frame {target_frame}: {len(target_bitmap) if target_bitmap else 0} rows")

if target_bitmap and len(target_bitmap) == 24:
    print(f"Searching for integer corners that match Frame {target_frame}")
    print("=" * 60)
    print("Testing axis-aligned squares...")

    # Frame 336 is the "horizontal bars" position
    # This should be a square with sides parallel to axes
    # Try squares centered around (12, 12) with width ~12-14

    best_match = 0
    best_corners = None

    # Try axis-aligned squares first (horizontal/vertical edges)
    for cx in range(10, 14):
        for cy in range(10, 14):
            for half_width in range(5, 9):
                # Square with sides parallel to axes
                corners = [
                    (cx - half_width, cy - half_width),  # Top-left
                    (cx + half_width, cy - half_width),  # Top-right
                    (cx + half_width, cy + half_width),  # Bottom-right
                    (cx - half_width, cy + half_width),  # Bottom-left
                ]

                rendered = render_from_corners(corners, 24, 24)
                matches = compare_bitmaps(target_bitmap, rendered)

                if matches > best_match:
                    best_match = matches
                    best_corners = corners
                    if matches == 576:
                        print(f"✓ PERFECT MATCH! Corners: {corners}")
                        print(f"  Center: ({cx}, {cy}), half_width: {half_width}")
                        break

            if best_match == 576:
                break
        if best_match == 576:
            break

    if best_match < 576:
        print(f"Best axis-aligned match: {best_match}/576 ({100*best_match/576:.2f}%)")
        print(f"  Corners: {best_corners}")

        # Try slightly rotated squares
        print("\nTrying slightly rotated squares...")

        for cx in range(10, 14):
            for cy in range(10, 14):
                for size in range(6, 10):
                    # Try small integer offsets for rotation
                    for dx in range(-2, 3):
                        for dy in range(-2, 3):
                            if dx == 0 and dy == 0:
                                continue

                            # Four corners with small rotational offset
                            corners = [
                                (cx - size + dx, cy - size + dy),
                                (cx + size - dy, cy - size + dx),
                                (cx + size - dx, cy + size - dy),
                                (cx - size + dy, cy + size - dx),
                            ]

                            rendered = render_from_corners(corners, 24, 24)
                            matches = compare_bitmaps(target_bitmap, rendered)

                            if matches > best_match:
                                best_match = matches
                                best_corners = corners
                                if matches == 576:
                                    print(f"✓ PERFECT MATCH! Corners: {corners}")
                                    print(f"  Approximate center: ({cx}, {cy}), size: {size}, offset: ({dx}, {dy})")
                                    break

                        if best_match == 576:
                            break
                    if best_match == 576:
                        break
                if best_match == 576:
                    break
            if best_match == 576:
                break

        if best_match < 576:
            print(f"\nBest rotated match: {best_match}/576 ({100*best_match/576:.2f}%)")
            print(f"  Corners: {best_corners}")
