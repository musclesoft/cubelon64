#!/usr/bin/env python3
"""
ZX Spectrum screen viewer
Converts ZX Spectrum screen data to text representation
"""

import sys
import os

# ZX Spectrum colors
ZX_COLORS = [
    'BLACK', 'BLUE', 'RED', 'MAGENTA',
    'GREEN', 'CYAN', 'YELLOW', 'WHITE'
]

def decode_zx_screen(screen_data):
    """
    Decode ZX Spectrum screen format
    Screen: 6144 bytes bitmap + 768 bytes attributes = 6912 bytes
    Bitmap: 256x192 pixels, organized in thirds (0-63, 64-127, 128-191)
    Attributes: 8x8 cells, 32x24 grid
    """
    if len(screen_data) < 6144:
        print(f"Warning: Screen data is only {len(screen_data)} bytes (expected 6144+)")

    bitmap_size = min(6144, len(screen_data))
    bitmap = screen_data[:bitmap_size]

    # Check if we have attribute data
    if len(screen_data) >= 6912:
        attrs = screen_data[6144:6912]
    else:
        attrs = None

    return bitmap, attrs

def display_screen_summary(bitmap, attrs):
    """Display summary of screen contents"""
    print("ZX Spectrum Screen Analysis")
    print("=" * 60)
    print(f"Bitmap size: {len(bitmap)} bytes")
    if attrs:
        print(f"Attributes: {len(attrs)} bytes")
    print()

    # Count non-zero bytes
    non_zero = sum(1 for b in bitmap if b != 0)
    print(f"Non-zero bitmap bytes: {non_zero}/{len(bitmap)} ({non_zero*100//len(bitmap)}%)")
    print()

    if attrs:
        # Analyze attributes
        attr_counts = {}
        for attr in attrs:
            ink = attr & 0x07
            paper = (attr >> 3) & 0x07
            bright = (attr >> 6) & 0x01
            flash = (attr >> 7) & 0x01

            key = (ink, paper, bright, flash)
            attr_counts[key] = attr_counts.get(key, 0) + 1

        print("Most common attribute combinations:")
        sorted_attrs = sorted(attr_counts.items(), key=lambda x: x[1], reverse=True)
        for (ink, paper, bright, flash), count in sorted_attrs[:10]:
            bright_str = " BRIGHT" if bright else ""
            flash_str = " FLASH" if flash else ""
            print(f"  INK {ink} ({ZX_COLORS[ink]}), PAPER {paper} ({ZX_COLORS[paper]}){bright_str}{flash_str}: {count} cells")

def display_screen_ascii(bitmap, width=32, height=24):
    """Display screen as ASCII art (simplified)"""
    print("\nASCII representation (8x8 char grid, # = any pixel set):")
    print("-" * (width + 2))

    for char_y in range(height):
        line = "|"
        for char_x in range(width):
            # Check if any pixel is set in this 8x8 character
            has_pixel = False
            for py in range(8):
                y = char_y * 8 + py
                if y >= 192:
                    break

                # ZX Spectrum screen layout is complex
                # Simplified check
                byte_offset = (y // 64) * 2048 + (y % 8) * 256 + ((y // 8) % 8) * 32 + char_x
                if byte_offset < len(bitmap) and bitmap[byte_offset] != 0:
                    has_pixel = True
                    break

            line += "#" if has_pixel else " "
        line += "|"
        print(line)

    print("-" * (width + 2))

def main():
    if len(sys.argv) < 2:
        print("Usage: view_zx_screen.py <screen_file>")
        print("\nDisplays ZX Spectrum screen data analysis")
        sys.exit(1)

    filename = sys.argv[1]

    if not os.path.exists(filename):
        print(f"Error: File '{filename}' not found")
        sys.exit(1)

    with open(filename, 'rb') as f:
        data = f.read()

    bitmap, attrs = decode_zx_screen(data)
    display_screen_summary(bitmap, attrs)
    display_screen_ascii(bitmap)

    print(f"\nFile: {filename}")
    print(f"Total size: {len(data)} bytes")

if __name__ == '__main__':
    main()
