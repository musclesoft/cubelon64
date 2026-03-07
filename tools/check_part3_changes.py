#!/usr/bin/env python3
import json
import hashlib
import base64

with open('part3_tiles_900-910.json', 'r') as f:
    tiles = json.load(f)

print('Frame -> Pattern Hash (first 96 bytes):')
for tile in tiles[:11]:
    # Decode base64 pattern bytes
    pattern_bytes = base64.b64decode(tile['pattern_bytes'])
    # Hash first 96 bytes to see if pattern changes
    pattern_hash = hashlib.md5(pattern_bytes[:96]).hexdigest()[:8]
    print(f"Frame {tile['frame']}: {pattern_hash}")

# Check if all frames are identical
if len(tiles) >= 2:
    print()
    frame1_bytes = base64.b64decode(tiles[0]['pattern_bytes'])
    frame2_bytes = base64.b64decode(tiles[1]['pattern_bytes'])
    if frame1_bytes == frame2_bytes:
        print("✓ Frames 900 and 901 have IDENTICAL pattern data")
    else:
        diffs = sum(1 for a, b in zip(frame1_bytes, frame2_bytes) if a != b)
        print(f"✗ Frames 900 and 901 differ in {diffs}/288 bytes")
