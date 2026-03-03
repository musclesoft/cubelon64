#!/usr/bin/env python3
"""
Extract pattern tiles from CUBELON data blocks
Converts ZX Spectrum 8x8 tiles to C64 charset format
"""

import sys
import os

def extract_tiles(data):
    """Extract 8x8 tiles from data"""
    tiles = []
    offset = 0

    while offset + 8 <= len(data):
        tile = data[offset:offset+8]
        tiles.append(tile)
        offset += 8

    return tiles

def visualize_tile(tile_bytes):
    """Convert tile bytes to ASCII visualization"""
    lines = []
    for byte in tile_bytes:
        line = format(byte, '08b').replace('0', '.').replace('1', '#')
        lines.append(line)
    return '\n'.join(lines)

def generate_c64_charset_asm(tiles, output_file, label="pattern_tiles"):
    """Generate CA65 assembly file with charset data"""

    with open(output_file, 'w') as f:
        f.write(f"; Auto-generated pattern tiles from ZX Spectrum data\n")
        f.write(f"; Total tiles: {len(tiles)}\n\n")
        f.write(f".export {label}\n\n")
        f.write(f"{label}:\n")

        for i, tile in enumerate(tiles):
            if i % 8 == 0:
                f.write(f"\n; Tiles {i}-{min(i+7, len(tiles)-1)}\n")

            f.write(f".byte ")
            f.write(", ".join(f"${b:02X}" for b in tile))
            f.write(f"  ; tile {i}\n")

        f.write(f"\n; End of {label} ({len(tiles)} tiles, {len(tiles)*8} bytes)\n")

    print(f"Generated: {output_file} ({len(tiles)} tiles)")

def generate_tile_viewer_html(tiles, output_file, title="Pattern Tiles"):
    """Generate HTML file to visualize tiles"""

    with open(output_file, 'w') as f:
        f.write(f"""<!DOCTYPE html>
<html>
<head>
    <title>{title}</title>
    <style>
        body {{ font-family: monospace; background: #000; color: #fff; }}
        .tile {{
            display: inline-block;
            margin: 2px;
            background: #000;
            border: 1px solid #333;
            padding: 2px;
        }}
        .tile-row {{ white-space: nowrap; font-size: 8px; line-height: 8px; }}
        .pixel {{ display: inline-block; width: 8px; height: 8px; }}
        .on {{ background: #fff; }}
        .off {{ background: #000; }}
        .tile-num {{ font-size: 10px; color: #666; }}
        h1 {{ color: #0f0; }}
    </style>
</head>
<body>
    <h1>{title}</h1>
    <p>Total tiles: {len(tiles)}</p>
    <div>
""")

        for i, tile in enumerate(tiles):
            f.write(f'<div class="tile">\n')
            f.write(f'<div class="tile-num">#{i}</div>\n')
            for byte in tile:
                f.write('<div class="tile-row">')
                for bit in range(8):
                    pixel_class = 'on' if (byte & (0x80 >> bit)) else 'off'
                    f.write(f'<span class="pixel {pixel_class}"></span>')
                f.write('</div>\n')
            f.write('</div>\n')

            if (i + 1) % 16 == 0:
                f.write('<br>\n')

        f.write("""
    </div>
</body>
</html>
""")

    print(f"Generated: {output_file} (HTML tile viewer)")

def main():
    print("CUBELON Pattern Tile Extractor\n")
    print("="*60)

    # Extract tiles from each data block
    tile_blocks = [
        ('original/cubelon_data_03.bin', 'pattern_set_1'),
        ('original/cubelon_data_05.bin', 'pattern_set_2'),
        ('original/cubelon_data_07.bin', 'pattern_set_3'),
        ('original/cubelon_data_08.bin', 'pattern_set_4'),
    ]

    all_tiles = []

    for filename, label in tile_blocks:
        print(f"\nProcessing {filename}...")

        if not os.path.exists(filename):
            print(f"  File not found, skipping")
            continue

        with open(filename, 'rb') as f:
            data = f.read()

        tiles = extract_tiles(data)
        print(f"  Extracted: {len(tiles)} tiles")

        # Show first tile as sample
        if tiles:
            print(f"  First tile:")
            for line in visualize_tile(tiles[0]).split('\n'):
                print(f"    {line}")

        all_tiles.extend(tiles)

        # Generate assembly file for this set
        os.makedirs('src/data', exist_ok=True)
        output_asm = f'src/data/{label}.s'
        generate_c64_charset_asm(tiles, output_asm, label)

    print(f"\n{'='*60}")
    print(f"Total tiles extracted: {len(all_tiles)}")
    print(f"Total data size: {len(all_tiles) * 8} bytes")

    # Generate combined tile set (first 256 tiles for C64 charset)
    if len(all_tiles) > 0:
        charset_tiles = all_tiles[:256]
        print(f"\nGenerating C64 charset with first {len(charset_tiles)} tiles...")
        generate_c64_charset_asm(charset_tiles, 'src/data/pattern_charset.s', 'pattern_charset')

        # Generate HTML viewer
        generate_tile_viewer_html(charset_tiles, 'build/tiles_preview.html', 'CUBELON Pattern Tiles')

        print(f"\nTo view tiles: open build/tiles_preview.html in a browser")

    print("\nDone!")

if __name__ == '__main__':
    main()
