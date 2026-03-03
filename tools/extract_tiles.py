#!/usr/bin/env python3
"""
Extract 8x8 tiles from ZX Spectrum screen data
Converts to C64-compatible format
"""

import sys
import os

def extract_zx_tiles(bitmap_data, tile_width=8, tile_height=8):
    """
    Extract 8x8 tiles from ZX Spectrum bitmap
    ZX Screen layout is complex - organized in thirds
    """
    tiles = []

    # ZX Spectrum screen is 32x24 characters (256x192 pixels)
    chars_x = 32
    chars_y = 24

    for char_y in range(chars_y):
        for char_x in range(chars_x):
            tile = []

            for py in range(8):
                y = char_y * 8 + py

                # ZX Spectrum screen memory layout:
                # Organized in thirds (lines 0-63, 64-127, 128-191)
                # Within each third: scan line interleave
                third = y // 64  # Which third (0, 1, 2)
                y_in_third = y % 64
                line_in_char = y_in_third % 8
                char_row = y_in_third // 8

                offset = (third * 2048) + (line_in_char * 256) + (char_row * 32) + char_x

                if offset < len(bitmap_data):
                    tile.append(bitmap_data[offset])
                else:
                    tile.append(0)

            tiles.append((char_x, char_y, tile))

    return tiles

def tiles_to_c64_charset(tiles):
    """
    Convert tiles to C64 character set format
    Each char is 8 bytes (8x8 pixels, 1 bit per pixel)
    """
    charset = []
    for char_x, char_y, tile_data in tiles:
        charset.extend(tile_data)
    return charset

def find_unique_tiles(tiles):
    """Find unique tiles and create a mapping"""
    unique = {}
    tile_map = []
    next_index = 0

    for char_x, char_y, tile_data in tiles:
        tile_bytes = bytes(tile_data)

        if tile_bytes not in unique:
            unique[tile_bytes] = next_index
            next_index += 1

        tile_map.append((char_x, char_y, unique[tile_bytes]))

    return list(unique.keys()), tile_map

def save_c64_charset(charset_data, filename):
    """Save charset in C64 format"""
    with open(filename, 'wb') as f:
        f.write(bytes(charset_data))
    print(f"Saved C64 charset: {filename} ({len(charset_data)} bytes)")

def generate_asm_charset(unique_tiles, filename):
    """Generate assembly file with charset data"""
    with open(filename, 'w') as f:
        f.write("; Auto-generated charset data from ZX Spectrum graphics\n")
        f.write("; Each character is 8 bytes (8x8 pixels)\n\n")
        f.write(".export charset_data\n\n")
        f.write("charset_data:\n")

        for i, tile_bytes in enumerate(unique_tiles):
            if i % 4 == 0:
                f.write(f"\n; Characters {i}-{min(i+3, len(unique_tiles)-1)}\n")

            f.write(f".byte ")
            f.write(", ".join(f"${b:02X}" for b in tile_bytes))
            f.write(f"  ; char {i}\n")

        f.write(f"\n; Total: {len(unique_tiles)} unique characters\n")

    print(f"Generated ASM charset: {filename}")

def generate_screen_map(tile_map, filename):
    """Generate screen map (which char at each position)"""
    with open(filename, 'w') as f:
        f.write("; Screen layout map\n")
        f.write("; 32x24 character positions\n\n")
        f.write(".export screen_map\n\n")
        f.write("screen_map:\n")

        for y in range(24):
            f.write(".byte ")
            chars = []
            for x in range(32):
                for cx, cy, char_idx in tile_map:
                    if cx == x and cy == y:
                        chars.append(f"${char_idx:02X}")
                        break
            f.write(", ".join(chars))
            f.write(f"  ; row {y}\n")

    print(f"Generated screen map: {filename}")

def main():
    if len(sys.argv) < 2:
        print("Usage: extract_tiles.py <screen_file> [output_prefix]")
        print("\nExtracts 8x8 tiles from ZX Spectrum screen data")
        print("Generates C64-compatible charset and screen map")
        sys.exit(1)

    filename = sys.argv[1]
    output_prefix = sys.argv[2] if len(sys.argv) >= 3 else "charset"

    if not os.path.exists(filename):
        print(f"Error: File '{filename}' not found")
        sys.exit(1)

    with open(filename, 'rb') as f:
        data = f.read()

    # Extract bitmap (first 6144 bytes)
    bitmap = data[:6144] if len(data) >= 6144 else data

    print(f"Extracting tiles from {filename}...")
    print(f"Bitmap size: {len(bitmap)} bytes\n")

    # Extract all tiles
    tiles = extract_zx_tiles(bitmap)
    print(f"Extracted {len(tiles)} tiles (32x24 screen)\n")

    # Find unique tiles
    unique_tiles, tile_map = find_unique_tiles(tiles)
    print(f"Unique tiles: {len(unique_tiles)}")
    print(f"Compression ratio: {len(unique_tiles)}/{len(tiles)} = {len(unique_tiles)*100//len(tiles)}%\n")

    # Generate outputs
    output_dir = "src/data"
    os.makedirs(output_dir, exist_ok=True)

    # Generate ASM files
    charset_file = f"{output_dir}/{output_prefix}.s"
    generate_asm_charset(unique_tiles, charset_file)

    # Screen map shows which character to use at each position
    # (Only useful if we have unique tiles, otherwise just use sequential)
    if len(unique_tiles) < len(tiles):
        map_file = f"{output_dir}/{output_prefix}_map.s"
        generate_screen_map(tile_map, map_file)

    # Also save raw binary for reference
    charset_data = []
    for tile_bytes in unique_tiles:
        charset_data.extend(tile_bytes)
    save_c64_charset(charset_data, f"{output_dir}/{output_prefix}.bin")

    print("\nDone!")

if __name__ == '__main__':
    main()
