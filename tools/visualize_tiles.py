#!/usr/bin/env python3
"""
Visualize C64 charset tiles as ASCII art
"""
import sys

def read_charset(filename):
    """Read the pattern_charset.s file and extract tile data"""
    tiles = []
    with open(filename, 'r') as f:
        for line in f:
            if line.strip().startswith('.byte'):
                # Extract hex bytes
                data_part = line.split('.byte')[1].split(';')[0]
                bytes_str = data_part.replace('$', '').replace(',', ' ').split()
                tile_bytes = [int(b, 16) for b in bytes_str]
                tiles.append(tile_bytes)
    return tiles

def visualize_tile(tile_data, tile_num):
    """Print a tile as ASCII art"""
    print(f"\n=== Tile {tile_num} ===")
    print("Hex: " + " ".join(f"{b:02X}" for b in tile_data))
    print()

    for byte in tile_data:
        line = ""
        for bit in range(7, -1, -1):
            if byte & (1 << bit):
                line += "██"
            else:
                line += "  "
        print(line)

def main():
    if len(sys.argv) < 2:
        print("Usage: visualize_tiles.py <tile_numbers>")
        print("Example: visualize_tiles.py 0 1 2 8 9 10 32 160")
        sys.exit(1)

    tiles = read_charset('src/data/pattern_charset.s')
    print(f"Loaded {len(tiles)} tiles")

    tile_nums = [int(x) for x in sys.argv[1:]]
    for tile_num in tile_nums:
        if tile_num < len(tiles):
            visualize_tile(tiles[tile_num], tile_num)
        else:
            print(f"\nTile {tile_num} not found (only {len(tiles)} tiles)")

if __name__ == '__main__':
    main()
