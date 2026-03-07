#!/usr/bin/env python3
"""
Visualize Part 3 tiles extracted from ZX Spectrum memory.
Part 3 uses 48×48 pixel tiles (288 bytes each).
"""

import json
import base64
from PIL import Image
import sys

def visualize_tiles(json_file):
    # Load tile data from JSON
    with open(json_file, 'r') as f:
        tiles = json.load(f)

    print(f'Loaded {len(tiles)} tiles from {json_file}')

    # Create a grid showing all tiles
    tile_width = 48
    tile_height = 48

    # Grid layout: 4 columns
    cols = 4
    rows = (len(tiles) + cols - 1) // cols
    scale = 4  # 4x scale for visibility
    padding = 2 * scale

    sheet_width = (tile_width * scale + padding) * cols - padding
    sheet_height = (tile_height * scale + padding) * rows - padding

    # Create sprite sheet
    sprite_sheet = Image.new('RGB', (sheet_width, sheet_height), (0, 0, 0))

    for idx, tile in enumerate(tiles):
        col = idx % cols
        row = idx // cols

        x_offset = col * (tile_width * scale + padding)
        y_offset = row * (tile_height * scale + padding)

        # Draw tile pixels (already decoded in JSON)
        for y, row_pixels in enumerate(tile['pixels']):
            for x, pixel in enumerate(row_pixels):
                if pixel:
                    for sy in range(scale):
                        for sx in range(scale):
                            sprite_sheet.putpixel(
                                (x_offset + x * scale + sx, y_offset + y * scale + sy),
                                (255, 255, 255)
                            )

    # Save sprite sheet
    output_file = json_file.replace('.json', '_sheet.png')
    sprite_sheet.save(output_file)
    print(f"✅ Created sprite sheet: {sheet_width}×{sheet_height} pixels")
    print(f"   {len(tiles)} tiles in {cols}×{rows} grid, each tile {tile_width}×{tile_height} pixels at {scale}x scale")
    print(f"   Saved to: {output_file}")

    # Also show first tile as ASCII
    if tiles:
        print(f"\nFirst tile (frame {tiles[0]['frame']}) visualization:")
        for row in tiles[0]['pixels']:
            print('  ' + ''.join(['█' if p else '·' for p in row]))

if __name__ == '__main__':
    if len(sys.argv) > 1:
        visualize_tiles(sys.argv[1])
    else:
        # Default to the exported file
        visualize_tiles('part3_tiles_900-910.json')
