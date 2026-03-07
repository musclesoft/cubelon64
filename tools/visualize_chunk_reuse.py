#!/usr/bin/env python3
"""
Visualize pattern chunk reuse by color-coding literally stored vs. reused chunks.

IMPORTANT: This script only shows chunk reuse patterns, not the actual pattern images.
Pattern pixels are not included in JSON exports (screen extraction is not a valid
technique for understanding the algorithm). To visualize actual patterns, use a
source-based decoder with the write sequences and navigation constants.

For 48×48 patterns with 8-chunk indirection:
- Each unique chunk address gets a unique color
- Repeated addresses show as the same color
- This reveals which parts are literally stored vs. transformed/reused
- A 4×2 grid represents the 8 chunks

For 24×24 patterns:
- No chunking, so single color block (all literally stored)
"""

import json
import base64
from PIL import Image, ImageDraw, ImageFont
from collections import defaultdict


def get_chunk_colors(pattern_addrs):
    """Assign colors to unique chunk addresses."""
    unique_addrs = []
    for addr in pattern_addrs:
        if addr not in unique_addrs:
            unique_addrs.append(addr)

    # Generate distinct colors for each unique chunk
    colors = [
        (255, 100, 100),  # Red
        (100, 255, 100),  # Green
        (100, 100, 255),  # Blue
        (255, 255, 100),  # Yellow
        (255, 100, 255),  # Magenta
        (100, 255, 255),  # Cyan
        (255, 150, 100),  # Orange
        (150, 100, 255),  # Purple
    ]

    # Map each chunk index to its color based on unique address
    chunk_colors = []
    for addr in pattern_addrs:
        unique_idx = unique_addrs.index(addr)
        chunk_colors.append(colors[unique_idx % len(colors)])

    return chunk_colors, len(unique_addrs)


def create_chunk_diagram(pattern_addrs, size=64):
    """Create chunk reuse diagram showing which chunks are unique vs. reused."""
    chunk_colors, num_unique = get_chunk_colors(pattern_addrs)
    diagram = Image.new('RGB', (size, size), (20, 20, 20))
    diagram_pixels = diagram.load()

    # Draw 4x2 grid representing 8 chunks
    cell_h = size // 4
    cell_w = size // 2

    for chunk_idx in range(8):
        chunk_row = chunk_idx // 2
        chunk_col = chunk_idx % 2
        color = chunk_colors[chunk_idx]

        # Fill cell with color
        for dy in range(cell_h):
            for dx in range(cell_w):
                y = chunk_row * cell_h + dy
                x = chunk_col * cell_w + dx
                if y < size and x < size:
                    diagram_pixels[x, y] = color

    return diagram, num_unique


def create_reuse_visualization(json_file, output_file, part_name):
    """Create sprite sheet showing chunk reuse with color coding."""
    print(f"\n{part_name}:")
    print(f"  Loading: {json_file}")

    with open(json_file) as f:
        data = json.load(f)

    # Find unique patterns
    unique_patterns = {}
    pattern_stats = []

    for pattern in data:
        if 'pattern_addrs' in pattern:
            # 48×48 chunked patterns
            addrs_tuple = tuple(pattern['pattern_addrs'])
            if addrs_tuple not in unique_patterns:
                unique_patterns[addrs_tuple] = pattern

                # Analyze chunk reuse
                unique_addrs = len(set(pattern['pattern_addrs']))
                pattern_stats.append({
                    'frame': pattern['frame'],
                    'pattern': pattern,
                    'unique_chunks': unique_addrs,
                    'total_chunks': 8,
                    'reuse_ratio': 8 / unique_addrs
                })
        else:
            # 24×24 direct patterns - no chunking
            pattern_bytes = base64.b64decode(pattern['pattern_bytes'])
            key = pattern_bytes
            if key not in unique_patterns:
                unique_patterns[key] = pattern
                pattern_stats.append({
                    'frame': pattern['frame'],
                    'pattern': pattern,
                    'unique_chunks': 1,
                    'total_chunks': 1,
                    'reuse_ratio': 1.0
                })

    print(f"  Total frames: {len(data)}")
    print(f"  Unique patterns: {len(unique_patterns)}")

    # Sort by frame number
    pattern_stats.sort(key=lambda x: x['frame'])

    # Analyze reuse statistics
    reuse_counts = defaultdict(int)
    for stat in pattern_stats:
        reuse_counts[stat['unique_chunks']] += 1

    print(f"\n  Chunk reuse breakdown:")
    for unique_chunks in sorted(reuse_counts.keys()):
        count = reuse_counts[unique_chunks]
        if unique_chunks == 1 and pattern_stats[0]['total_chunks'] == 1:
            print(f"    Direct storage (no chunking): {count} patterns")
        else:
            reuse_ratio = 8 / unique_chunks
            print(f"    {unique_chunks} unique chunks (×{reuse_ratio:.1f} reuse): {count} patterns")

    # Create sprite sheet
    is_small = pattern_stats[0]['total_chunks'] == 1  # 24×24 patterns
    tile_size = 24 if is_small else 48
    scale = 6 if is_small else 3

    cols = 8
    rows = (len(pattern_stats) + cols - 1) // cols

    # For chunked patterns, add space for chunk diagram
    if is_small:
        tile_width = tile_size * scale
    else:
        # 48 + 16 (diagram) + 2 (gap) = 66 pixels, scaled
        tile_width = (tile_size + 18) * scale
    tile_height = tile_size * scale
    margin = 10
    label_height = 30

    sheet_width = cols * (tile_width + margin) + margin
    sheet_height = rows * (tile_height + label_height + margin) + margin

    sprite_sheet = Image.new('RGB', (sheet_width, sheet_height), (40, 40, 40))
    draw = ImageDraw.Draw(sprite_sheet)

    for idx, stat in enumerate(pattern_stats):
        col = idx % cols
        row = idx // cols

        x = margin + col * (tile_width + margin)
        y = margin + row * (tile_height + label_height + margin)

        pattern = stat['pattern']

        # Create chunk reuse diagram
        if 'pattern_addrs' in pattern:
            # 48×48 chunked - show chunk reuse diagram
            img, num_unique = create_chunk_diagram(pattern['pattern_addrs'], size=tile_width//scale)
        else:
            # 24×24 direct - single color block (all literal storage)
            img = Image.new('RGB', (tile_size, tile_size), (100, 100, 100))
            num_unique = 1

        # Scale up
        img = img.resize((tile_width, tile_height), Image.NEAREST)
        sprite_sheet.paste(img, (x, y))

        # Draw label with chunk count
        if stat['total_chunks'] > 1:
            label = f"F{stat['frame']} ({stat['unique_chunks']}×{int(stat['reuse_ratio'])})"
        else:
            label = f"F{stat['frame']}"
        draw.text((x, y + tile_height + 2), label, fill=(200, 200, 200))

        # Draw legend showing reuse ratio
        if stat['total_chunks'] > 1:
            legend_y = y + tile_height + 16
            legend_text = f"{stat['unique_chunks']} unique"
            draw.text((x, legend_y), legend_text, fill=(150, 150, 150))

    sprite_sheet.save(output_file)
    print(f"  Saved: {output_file}")

    return len(unique_patterns), reuse_counts


def main():
    print("=" * 70)
    print("CUBELON: Pattern Chunk Reuse Visualization")
    print("=" * 70)
    print("\nIMPORTANT: Shows chunk reuse patterns only, not actual pattern images")
    print("(Pattern pixels require source-based decoder with write sequences)")
    print("\nColor-coding legend:")
    print("  • Each unique color = one unique 72-byte chunk")
    print("  • Same color appearing multiple times = chunk reused")
    print("  • More colors = more literal storage")
    print("  • Fewer colors = more memory-efficient reuse")

    all_stats = {}

    # Part 2: 24×24 patterns (no chunking)
    print("\n" + "=" * 70)
    count, stats = create_reuse_visualization(
        '../emulator/animation_frames_168-801.json',
        'part2_chunk_reuse.png',
        'Part 2 (24×24 direct)'
    )
    all_stats['Part 2'] = stats

    # Part 3: 48×48 patterns
    print("\n" + "=" * 70)
    count, stats = create_reuse_visualization(
        '../emulator/part3_tiles_802-1404.json',
        'part3_chunk_reuse.png',
        'Part 3 (48×48 chunked)'
    )
    all_stats['Part 3'] = stats

    # Part 4: 48×48 patterns
    print("\n" + "=" * 70)
    count, stats = create_reuse_visualization(
        '../emulator/part3_tiles_1405-2304.json',
        'part4_chunk_reuse.png',
        'Part 4 (48×48 chunked)'
    )
    all_stats['Part 4'] = stats

    # Part 5: 48×48 patterns
    print("\n" + "=" * 70)
    count, stats = create_reuse_visualization(
        '../emulator/part3_tiles_2305-3999.json',
        'part5_chunk_reuse.png',
        'Part 5 (48×48 chunked)'
    )
    all_stats['Part 5'] = stats

    print("\n" + "=" * 70)
    print("Summary: Chunk Reuse Patterns")
    print("=" * 70)

    for part_name, stats in all_stats.items():
        print(f"\n{part_name}:")
        for unique_chunks in sorted(stats.keys()):
            count = stats[unique_chunks]
            if unique_chunks == 1:
                print(f"  1 chunk (direct storage): {count} patterns")
            else:
                reuse_ratio = 8 / unique_chunks
                pct = count / sum(stats.values()) * 100
                print(f"  {unique_chunks} unique chunks (×{reuse_ratio:.1f} reuse): {count} patterns ({pct:.0f}%)")

    print("\n" + "=" * 70)
    print("Interpretation:")
    print("  • Part 2: All direct storage (no chunking system)")
    print("  • Parts 3-5: Look for patterns with 1-2 unique chunks")
    print("    - These show extreme memory efficiency")
    print("    - Symmetry/repetition achieved by reusing same data")
    print("  • More unique chunks = more unique data stored")
    print("\nMethodological Note:")
    print("  This analysis uses only SOURCE data (pattern addresses from $84C0)")
    print("  Screen extraction is not used - it does not reveal algorithm logic")
    print("=" * 70)
    print()


if __name__ == '__main__':
    main()
