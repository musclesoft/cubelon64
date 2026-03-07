#!/usr/bin/env python3
"""
Extract raw patterns WITHOUT centering/wrapping to see the original geometry.
"""

# Read the raw 72-byte patterns directly
import struct

# Pattern addresses from animation_318-336.txt
pattern_sources = {
    318: 0xA73E,
    320: 0xAF66,
    322: 0xAF1E,
    324: 0xAED6,
    326: 0xAE8E,
    328: 0xAE46,
    330: 0xADFE,
    332: 0xADB6,
    334: 0xAD6E,
    336: 0xAD26,
}

# These addresses point to the END of 72-byte patterns
# So actual start = address - 72 + 1

# Load the Z80 memory dump (we need the actual binary data)
# First, let me just decode the patterns raw without centering

def decode_pattern_raw(data: bytes) -> list:
    """Decode 72-byte pattern to 24×24 pixels WITHOUT any centering"""
    pixels = []
    for row in range(24):
        row_pixels = []
        if row % 2 == 0:
            # Even row: normal byte order [b0, b1, b2]
            b0 = data[row * 3 + 0]
            b1 = data[row * 3 + 1]
            b2 = data[row * 3 + 2]
        else:
            # Odd row: reversed byte order [b2, b1, b0]
            b0 = data[row * 3 + 2]
            b1 = data[row * 3 + 1]
            b2 = data[row * 3 + 0]

        # Convert bytes to pixels
        for byte_val in [b0, b1, b2]:
            for bit in range(7, -1, -1):
                row_pixels.append((byte_val >> bit) & 1)

        pixels.append(row_pixels)

    return pixels

def visualize_pattern(pixels: list) -> str:
    """Convert to visual string"""
    lines = []
    for row in pixels:
        line = ""
        for pixel in row:
            line += "█" if pixel else "·"
        lines.append(line)
    return "\n".join(lines)

# We need to read the actual binary data
# Let me check what files we have

import os
print("Looking for binary pattern data...")
print()

# Check if we saved the patterns
pattern_file = "../emulator/pattern_frame337_square0_72bytes.bin"
if os.path.exists(pattern_file):
    print(f"Found saved pattern: {pattern_file}")
    with open(pattern_file, "rb") as f:
        data = f.read()

    print(f"Pattern size: {len(data)} bytes")
    print("\nRaw pattern (no centering):")
    pixels = decode_pattern_raw(data)
    print(visualize_pattern(pixels))

    # Analyze geometry
    min_x, max_x = 24, -1
    min_y, max_y = 24, -1

    for y, row in enumerate(pixels):
        for x, pixel in enumerate(row):
            if pixel:
                min_x = min(min_x, x)
                max_x = max(max_x, x)
                min_y = min(min_y, y)
                max_y = max(max_y, y)

    print(f"\nBounding box: ({min_x}, {min_y}) to ({max_x}, {max_y})")
    print(f"Width: {max_x - min_x + 1}, Height: {max_y - min_y + 1}")

    # Check for wrapping
    left_pixels = sum(1 for row in pixels for x, p in enumerate(row) if p and x < 4)
    right_pixels = sum(1 for row in pixels for x, p in enumerate(row) if p and x >= 20)

    print(f"\nEdge distribution:")
    print(f"  Left edge (x<4): {left_pixels} pixels")
    print(f"  Right edge (x>=20): {right_pixels} pixels")

    if left_pixels > 0 and right_pixels > 0:
        print("  → Pattern appears SPLIT (wraps around)")
    else:
        print("  → Pattern is COMPACT (no wrapping)")

else:
    print(f"Pattern file not found: {pattern_file}")
    print("\nLet's extract it from the emulator directly...")
    print("(Need to modify run_cubelon.go to save raw patterns before centering)")
