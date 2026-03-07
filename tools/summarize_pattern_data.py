#!/usr/bin/env python3
import json
import base64

with open('part3_tiles_900-960.json', 'r') as f:
    tiles = json.load(f)

print('=== PART 3 PATTERN DATA SUMMARY ===\n')
print(f'Total frames extracted: {len(tiles)}\n')

# Group by pattern address
addr_map = {}
for tile in tiles:
    addr = tile['pattern_addr']
    if addr not in addr_map:
        addr_map[addr] = []
    addr_map[addr].append(tile['frame'])

print(f'Unique pattern addresses: {len(addr_map)}\n')
print('Pattern Address -> Frame List:')
for addr in sorted(addr_map.keys()):
    frames = addr_map[addr]
    print(f'  ${addr:04X} ({addr:5d}): frames {min(frames)}-{max(frames)} [{len(frames)} frames]')
    # Show first 16 bytes of pattern
    for tile in tiles:
        if tile['pattern_addr'] == addr and tile['frame'] == min(frames):
            pattern = base64.b64decode(tile['pattern_bytes'])
            hex_str = ' '.join(f'{b:02X}' for b in pattern[:16])
            print(f'            Data: {hex_str}...')
            break

addrs = sorted(addr_map.keys())
if len(addrs) > 1:
    spacing = addrs[1] - addrs[0]
    print(f'\nPattern spacing: ${spacing:04X} bytes ({spacing} decimal)')
    if spacing != 288:
        print(f'  Note: Spacing is ${288-spacing:04X} bytes less than pattern size (288 bytes)')
        print(f'        Patterns may overlap or use compressed storage')
