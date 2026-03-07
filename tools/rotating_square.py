#!/usr/bin/env python3
"""
Rotating square renderer using 4-edge scanline fill algorithm.
Goal: Recreate CUBELON Part 2 animation pixel-perfectly using an 8-bit friendly approach.
"""

import math
from typing import List, Tuple

class FixedPoint:
    """Fixed-point math for 8-bit systems (16.16 format for prototyping)"""
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
        """Multiply two fixed-point numbers"""
        return (a * b) >> FixedPoint.SHIFT

class Edge:
    """Represents one edge of the polygon for scanline filling"""
    def __init__(self, x1: float, y1: float, x2: float, y2: float):
        # Sort vertices by Y (top to bottom)
        if y1 > y2:
            x1, y1, x2, y2 = x2, y2, x1, y1

        self.y_start = y1
        self.y_end = y2
        self.x_start = x1
        self.x_end = x2

        # Calculate slope (dx/dy) as fixed-point
        dy = y2 - y1
        if dy > 0.001:
            self.dx_dy = FixedPoint.from_float((x2 - x1) / dy)
        else:
            self.dx_dy = 0

    def get_x_at_y(self, y: float) -> int:
        """Get X position at given Y scanline"""
        if self.y_end - self.y_start < 0.001:
            return int(self.x_start)

        # Interpolate: x = x_start + (y - y_start) * dx/dy
        dy = y - self.y_start
        dx = FixedPoint.multiply(FixedPoint.from_float(dy), self.dx_dy)
        x = FixedPoint.from_float(self.x_start) + dx
        return x >> FixedPoint.SHIFT

def render_square(center_x: float, center_y: float, size: float, angle: float,
                  width: int = 24, height: int = 24) -> List[List[bool]]:
    """
    Render a filled square using 4-edge scanline fill.

    Args:
        center_x, center_y: Center of the square
        size: Distance from center to corner (half-diagonal)
        angle: Rotation angle in radians
        width, height: Output bitmap dimensions

    Returns:
        2D boolean array [y][x] representing pixels
    """
    # Calculate 4 corners of the square
    corners = []
    for i in range(4):
        corner_angle = angle + (i * math.pi / 2)
        x = center_x + size * math.cos(corner_angle)
        y = center_y + size * math.sin(corner_angle)
        corners.append((x, y))

    # Create 4 edges (wrapping around)
    edges = []
    for i in range(4):
        x1, y1 = corners[i]
        x2, y2 = corners[(i + 1) % 4]
        edges.append(Edge(x1, y1, x2, y2))

    # Initialize bitmap
    bitmap = [[False] * width for _ in range(height)]

    # Find Y range
    y_min = int(max(0, min(edge.y_start for edge in edges)))
    y_max = int(min(height, max(edge.y_end for edge in edges) + 1))

    # Scanline fill
    for y in range(y_min, y_max):
        y_float = float(y)

        # Find all edges that cross this scanline
        x_intersections = []
        for edge in edges:
            # Check if edge spans this Y coordinate
            if edge.y_start <= y_float < edge.y_end:
                x = edge.get_x_at_y(y_float)
                x_intersections.append(x)

        # Sort intersections left to right
        x_intersections.sort()

        # Fill between pairs of intersections
        # For convex polygon, should be exactly 2 intersections
        if len(x_intersections) >= 2:
            x1 = x_intersections[0]
            x2 = x_intersections[-1]

            # Fill span
            x1 = max(0, x1)
            x2 = min(width - 1, x2)
            for x in range(x1, x2 + 1):
                bitmap[y][x] = True

    return bitmap

def parse_frame(lines: List[str]) -> Tuple[List[List[bool]], int]:
    """
    Parse a frame from the animation text file.
    Returns (bitmap, frame_number)
    """
    # Find frame number
    frame_num = None
    for line in lines:
        if "Frame" in line and "(" in line:
            # Extract number from "Frame NNN (Source: $XXXX)"
            parts = line.split()
            for i, part in enumerate(parts):
                if "Frame" in parts[i-1] if i > 0 else False:
                    try:
                        frame_num = int(part)
                        break
                    except ValueError:
                        pass

    # Parse bitmap (skip header lines)
    bitmap = []
    for line in lines:
        # Count dots and blocks
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

def bitmap_to_string(bitmap: List[List[bool]]) -> str:
    """Convert bitmap to visual string representation"""
    lines = []
    for row in bitmap:
        line = ""
        for pixel in row:
            line += "█" if pixel else "·"
        lines.append(line)
    return "\n".join(lines)

def compare_bitmaps(a: List[List[bool]], b: List[List[bool]]) -> Tuple[int, int]:
    """
    Compare two bitmaps and return (matching_pixels, total_pixels)
    """
    matches = 0
    total = 0
    for y in range(len(a)):
        for x in range(len(a[y])):
            if a[y][x] == b[y][x]:
                matches += 1
            total += 1
    return matches, total

def analyze_frame_geometry(bitmap: List[List[bool]]) -> dict:
    """
    Analyze a frame to extract geometry parameters:
    - Bounding box
    - Center of mass
    - Apparent rotation angle
    """
    # Find all filled pixels
    pixels = []
    for y in range(len(bitmap)):
        for x in range(len(bitmap[y])):
            if bitmap[y][x]:
                pixels.append((x, y))

    if not pixels:
        return None

    # Calculate bounding box
    min_x = min(p[0] for p in pixels)
    max_x = max(p[0] for p in pixels)
    min_y = min(p[1] for p in pixels)
    max_y = max(p[1] for p in pixels)

    # Center of mass
    center_x = sum(p[0] for p in pixels) / len(pixels)
    center_y = sum(p[1] for p in pixels) / len(pixels)

    # Calculate second moment to estimate rotation
    # For a rotated square, the principal axes give rotation angle
    m20 = sum((p[0] - center_x) ** 2 for p in pixels) / len(pixels)
    m02 = sum((p[1] - center_y) ** 2 for p in pixels) / len(pixels)
    m11 = sum((p[0] - center_x) * (p[1] - center_y) for p in pixels) / len(pixels)

    # Rotation angle from moments
    if m20 != m02:
        angle = 0.5 * math.atan2(2 * m11, m20 - m02)
    else:
        angle = 0

    # Estimate size (distance from center to corner)
    # For a square, diagonal = sqrt(2) * side, so size = diagonal/2
    width = max_x - min_x + 1
    height = max_y - min_y + 1
    diagonal = math.sqrt(width**2 + height**2)
    size = diagonal / 2

    return {
        'center_x': center_x,
        'center_y': center_y,
        'size': size,
        'angle': angle,
        'bbox': (min_x, min_y, max_x, max_y),
        'pixels': len(pixels)
    }

if __name__ == "__main__":
    import sys

    if len(sys.argv) > 1 and sys.argv[1] == "--test":
        # Basic algorithm tests
        print("Testing rotating square renderer")
        print("=" * 50)

        # Note: angle=0 creates a diamond (corners at N/S/E/W)
        # For horizontal square, use angle=π/4 (45°)

        # Test 1: Diamond (angle = 0)
        print("\nTest 1: Diamond (angle = 0°)")
        bitmap = render_square(12, 12, 7, 0, 24, 24)
        print(bitmap_to_string(bitmap))

        # Test 2: Horizontal square (angle = 45°)
        print("\nTest 2: Horizontal square (angle = 45°)")
        bitmap = render_square(12, 12, 7, math.pi/4, 24, 24)
        print(bitmap_to_string(bitmap))

        # Test 3: Intermediate rotation (angle = 22.5°)
        print("\nTest 3: Rotated 22.5°")
        bitmap = render_square(12, 12, 7, math.pi/8, 24, 24)
        print(bitmap_to_string(bitmap))

        print("\n" + "=" * 50)
        print("Algorithm: 4-edge scanline fill")
        print("- Fixed-point math (16.16 format)")
        print("- Finds 2 active edges per scanline")
        print("- Fills horizontal span between intersections")
        print("- Ready for 8-bit implementation (reduce to 8.8 format)")

    else:
        # Analyze and match extracted frames
        print("Analyzing CUBELON Part 2 extracted frames")
        print("=" * 50)

        # Read animation file
        with open("../emulator/frames/animation_318-336.txt", "r") as f:
            content = f.read()

        # Split into individual frames by looking for frame markers
        import re
        frame_pattern = re.compile(r'=== Frame (\d+)')

        # Parse frames 320, 322, 324, 326, 328, 330, 332, 334, 336 (unique patterns)
        frames_to_analyze = [320, 322, 324, 326, 328, 330, 332, 334, 336]

        # Split content into lines and find frame boundaries
        lines = content.split('\n')
        current_frame_lines = []
        current_frame_num = None

        for line in lines:
            match = frame_pattern.search(line)
            if match:
                # Save previous frame
                if current_frame_num in frames_to_analyze and current_frame_lines:
                    bitmap, _ = parse_frame(current_frame_lines)

                    if bitmap and len(bitmap) == 24:
                        # Analyze geometry
                        geo = analyze_frame_geometry(bitmap)
                        if geo:
                            print(f"\n=== Frame {current_frame_num} ===")
                            print(f"Center: ({geo['center_x']:.2f}, {geo['center_y']:.2f})")
                            print(f"Size estimate: {geo['size']:.2f}")
                            print(f"Bounding box: {geo['bbox']}, Pixels: {geo['pixels']}")

                            # Try different parameters to find best match
                            best_match = 0
                            best_params = None

                            # Try different center positions and sizes
                            for cx in [11.0, 11.5, 12.0]:
                                for cy in [11.0, 11.5, 12.0]:
                                    for size in [6.5, 7.0, 7.5, 8.0]:
                                        for angle_deg in range(0, 360, 2):
                                            angle_rad = math.radians(angle_deg)
                                            rendered = render_square(cx, cy, size, angle_rad, 24, 24)
                                            matches, total = compare_bitmaps(bitmap, rendered)

                                            if matches > best_match:
                                                best_match = matches
                                                best_params = (cx, cy, size, angle_rad)

                            if best_params:
                                match_pct = 100 * best_match / (24 * 24)
                                cx, cy, size, angle = best_params
                                print(f"Best match: {match_pct:.1f}% with cx={cx}, cy={cy}, size={size}, angle={math.degrees(angle):.1f}°")

                                if match_pct == 100.0:
                                    print("PERFECT MATCH!")
                                    rendered = render_square(cx, cy, size, angle, 24, 24)
                                    print("\nOriginal:")
                                    print(bitmap_to_string(bitmap))
                                    print("\nRendered:")
                                    print(bitmap_to_string(rendered))

                # Start new frame
                current_frame_num = int(match.group(1))
                current_frame_lines = [line]
            else:
                current_frame_lines.append(line)

        # Don't forget last frame
        if current_frame_num in frames_to_analyze and current_frame_lines:
            bitmap, _ = parse_frame(current_frame_lines)
            if bitmap and len(bitmap) == 24:
                geo = analyze_frame_geometry(bitmap)
                if geo:
                    print(f"\n=== Frame {current_frame_num} ===")
                    print(f"Center: ({geo['center_x']:.2f}, {geo['center_y']:.2f})")
                    print(f"Size estimate: {geo['size']:.2f}")
                    print(f"Bounding box: {geo['bbox']}, Pixels: {geo['pixels']}")

                    best_match = 0
                    best_params = None

                    for cx in [11.0, 11.5, 12.0]:
                        for cy in [11.0, 11.5, 12.0]:
                            for size in [6.5, 7.0, 7.5, 8.0]:
                                for angle_deg in range(0, 360, 2):
                                    angle_rad = math.radians(angle_deg)
                                    rendered = render_square(cx, cy, size, angle_rad, 24, 24)
                                    matches, total = compare_bitmaps(bitmap, rendered)

                                    if matches > best_match:
                                        best_match = matches
                                        best_params = (cx, cy, size, angle_rad)

                    if best_params:
                        match_pct = 100 * best_match / (24 * 24)
                        cx, cy, size, angle = best_params
                        print(f"Best match: {match_pct:.1f}% with cx={cx}, cy={cy}, size={size}, angle={math.degrees(angle):.1f}°")
