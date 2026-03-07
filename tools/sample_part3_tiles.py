#!/usr/bin/env python3
"""
Create a sampled visualization of Part 3 tiles (every Nth frame).
"""

import json
import sys

def sample_tiles(input_file, output_file, step=10):
    with open(input_file, 'r') as f:
        tiles = json.load(f)

    # Sample every Nth tile
    sampled = [tiles[i] for i in range(0, len(tiles), step)]

    print(f'Sampled {len(sampled)} tiles from {len(tiles)} (every {step} frames)')

    with open(output_file, 'w') as f:
        json.dump(sampled, f, indent=2)

    print(f'Saved to {output_file}')

if __name__ == '__main__':
    if len(sys.argv) > 1:
        input_file = sys.argv[1]
        step = int(sys.argv[2]) if len(sys.argv) > 2 else 10
        output_file = input_file.replace('.json', f'_every{step}.json')
        sample_tiles(input_file, output_file, step)
    else:
        sample_tiles('part3_tiles_900-1100.json', 'part3_tiles_900-1100_every10.json', 10)
