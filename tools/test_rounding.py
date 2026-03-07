#!/usr/bin/env python3
"""
Test different rounding and edge inclusion modes to find exact algorithm.
"""

import math
import re
from typing import List, Tuple

class FixedPoint:
    """Fixed-point math for 8-bit systems"""
    SHIFT = 16
    SCALE = 1 << SHIFT

    @staticmethod
    def from_float(f: float) -> int:
        return int(f * FixedPoint.SCALE)

    @staticmethod
    def to_float(fp: int) -> float:
        return fp / FixedPoint.SCALE

    @staticmethod
    def multiply(a: int, b: int) -> int:
        return (a * b) >> FixedPoint.SHIFT

class Edge:
    """Edge with configurable rounding"""
    def __init__(self, x1: float, y1: float, x2: float, y2: float):
        if y1 > y2:
            x1, y1, x2, y2 = x2, y2, x1, y1

        self.y_start = y1
        self.y_end = y2
        self.x_start = x1
        self.x_end = x2

        dy = y2 - y1
        if dy > 0.001:
            self.dx_dy = FixedPoint.from_float((x2 - x1) / dy)
        else:
            self.dx_dy = 0

    def get_x_at_y(self, y: float, round_mode: str) -> int:
        """Get X position with specified rounding mode"""
        if self.y_end - self.y_start < 0.001:
            return int(self.x_start)

        dy = y - self.y_start
        dx = FixedPoint.multiply(FixedPoint.from_float(dy), self.dx_dy)
        x = FixedPoint.from_float(self.x_start) + dx
        x_float = x / FixedPoint.SCALE

        if round_mode == 'floor':
            return int(math.floor(x_float))
        elif round_mode == 'ceil':
            return int(math.ceil(x_float))
        elif round_mode == 'round':
            return int(round(x_float))
        else:  # 'trunc' or default
            return int(x_float)

def render_square_variant(center_x: float, center_y: float, size: float, angle: float,
                          round_mode: str = 'trunc',
                          edge_rule: str = 'standard',
                          width: int = 24, height: int = 24) -> List[List[bool]]:
    """
    Render square with configurable rounding and edge rules.

    round_mode: 'trunc', 'floor', 'ceil', 'round'
    edge_rule:
      'standard' - y_start <= y < y_end
      'inclusive' - y_start <= y <= y_end
      'exclusive' - y_start < y < y_end
      'left_inclusive' - y_start <= y < y_end (current)
      'right_inclusive' - y_start < y <= y_end
    """
    # Calculate corners
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
        edges.append(Edge(x1, y1, x2, y2))

    bitmap = [[False] * width for _ in range(height)]

    y_min = int(max(0, min(edge.y_start for edge in edges)))
    y_max = int(min(height, max(edge.y_end for edge in edges) + 1))

    for y in range(y_min, y_max):
        y_float = float(y)

        x_intersections = []
        for edge in edges:
            # Apply edge rule
            active = False
            if edge_rule == 'standard' or edge_rule == 'left_inclusive':
                active = edge.y_start <= y_float < edge.y_end
            elif edge_rule == 'inclusive':
                active = edge.y_start <= y_float <= edge.y_end
            elif edge_rule == 'exclusive':
                active = edge.y_start < y_float < edge.y_end
            elif edge_rule == 'right_inclusive':
                active = edge.y_start < y_float <= edge.y_end

            if active:
                x = edge.get_x_at_y(y_float, round_mode)
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
    """Parse frame from animation text"""
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

def compare_bitmaps(a: List[List[bool]], b: List[List[bool]]) -> Tuple[int, int]:
    matches = 0
    total = 0
    for y in range(len(a)):
        for x in range(len(a[y])):
            if a[y][x] == b[y][x]:
                matches += 1
            total += 1
    return matches, total

# Load frame 324 (typical case)
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
            target_bitmap, _ = parse_frame(current_frame_lines)
            break
        current_frame_num = int(match.group(1))
        current_frame_lines = [line]
    else:
        current_frame_lines.append(line)

if target_bitmap and len(target_bitmap) == 24:
    # Test parameters from fine-tuning
    cx, cy, size, angle_deg = 11.5, 11.0, 8.0, 77.3

    print(f"Testing rounding modes for Frame {target_frame}")
    print("=" * 60)

    results = []

    for round_mode in ['trunc', 'floor', 'ceil', 'round']:
        for edge_rule in ['standard', 'inclusive', 'exclusive', 'right_inclusive']:
            rendered = render_square_variant(cx, cy, size, math.radians(angle_deg),
                                            round_mode, edge_rule, 24, 24)
            matches, total = compare_bitmaps(target_bitmap, rendered)
            match_pct = 100 * matches / total

            results.append({
                'round': round_mode,
                'edge': edge_rule,
                'match': match_pct,
                'matches': matches
            })

    # Sort by best match
    results.sort(key=lambda r: r['matches'], reverse=True)

    print("\nTop 10 configurations:")
    for i, r in enumerate(results[:10]):
        print(f"{i+1}. {r['match']:.2f}% - round={r['round']:<5} edge={r['edge']:<15} ({r['matches']}/576)")

    # Show best result
    best = results[0]
    if best['match'] == 100.0:
        print(f"\n✓ PERFECT MATCH FOUND!")
        print(f"  Rounding: {best['round']}")
        print(f"  Edge rule: {best['edge']}")
    else:
        print(f"\nBest match: {best['match']:.2f}% (still {576 - best['matches']} pixels different)")
