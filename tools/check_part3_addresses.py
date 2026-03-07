#!/usr/bin/env python3
import json

with open('part3_tiles_900-910.json', 'r') as f:
    tiles = json.load(f)

print('Frame -> Pattern Address:')
for tile in tiles[:11]:
    addr = tile['pattern_addr']
    print(f"Frame {tile['frame']}: 0x{addr:04X} ({addr})")
