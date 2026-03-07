#!/usr/bin/env python3
"""
Calculate the exact corner coordinates for the perfect-matching frame 336.
"""

import math

# Frame 336 perfect match parameters
cx, cy, size, angle_deg = 12.0, 11.5, 8.0, 43.2

angle_rad = math.radians(angle_deg)

print(f"Frame 336 perfect match parameters:")
print(f"  Center: ({cx}, {cy})")
print(f"  Size (corner distance): {size}")
print(f"  Angle: {angle_deg}° ({angle_rad:.6f} rad)")
print("\nCalculated corners:")

corners = []
for i in range(4):
    corner_angle = angle_rad + (i * math.pi / 2)
    x = cx + size * math.cos(corner_angle)
    y = cy + size * math.sin(corner_angle)
    corners.append((x, y))
    print(f"  Corner {i}: ({x:.6f}, {y:.6f})")

# Check if they're close to half-integers or integers
print("\nRounded to nearest 0.5:")
for i, (x, y) in enumerate(corners):
    x_rounded = round(x * 2) / 2
    y_rounded = round(y * 2) / 2
    print(f"  Corner {i}: ({x_rounded}, {y_rounded})")

print("\nRounded to nearest integer:")
for i, (x, y) in enumerate(corners):
    print(f"  Corner {i}: ({round(x)}, {round(y)})")

# Calculate what angle would give "perfect" 45 degrees
perfect_45_angle = math.radians(45.0)
print(f"\n\nFor comparison, perfect 45° would be:")
for i in range(4):
    corner_angle = perfect_45_angle + (i * math.pi / 2)
    x = cx + size * math.cos(corner_angle)
    y = cy + size * math.sin(corner_angle)
    print(f"  Corner {i}: ({x:.6f}, {y:.6f})")
