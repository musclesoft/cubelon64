#!/usr/bin/env python3
"""
Create a focused visualization showing the morphing animation.
Shows key frames: static, all morph frames, final.
"""
import json

with open('part3_tiles_900-960.json', 'r') as f:
    tiles = json.load(f)

# Select key frames:
# - Frame 900 (initial static)
# - Frames 920, 922, 924, 926, 928, 930, 932, 934, 936 (each morph step)
# - Frame 938 (final)
# - Frame 960 (transition)

key_frames = [900, 920, 922, 924, 926, 928, 930, 932, 934, 936, 938, 960]
sampled = [t for t in tiles if t['frame'] in key_frames]

print(f'Selected {len(sampled)} key morph frames from {len(tiles)}')

with open('part3_tiles_morphing.json', 'w') as f:
    json.dump(sampled, f, indent=2)

print('Saved to part3_tiles_morphing.json')
