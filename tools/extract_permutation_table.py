#!/usr/bin/env python3
"""
Extract the exact permutation table by comparing pattern bytes to screen pixels.

Once we have this table, we can decode any pattern purely from source data.
"""

import json
import base64

with open('../emulator/part3_tiles_900-960.json') as f:
    tiles = json.load(f)

# Use frame 900 (uses addresses BE06, C046 alternating)
frame900 = [t for t in tiles if t['frame'] == 900][0]
pattern_bytes = base64.b64decode(frame900['pattern_bytes'])
screen_pixels = frame900['pixels']

print(f"Building permutation table from frame 900...")
print(f"Pattern bytes: {len(pattern_bytes)}")
print(f"Screen pixels: {len(screen_pixels)}×{len(screen_pixels[0])}")
print()

# Convert screen pixels to bytes
screen_bytes = []
for row in range(48):
    for byte_idx in range(6):
        byte_val = 0
        for bit in range(8):
            col = byte_idx * 8 + bit
            if screen_pixels[row][col] == 1:
                byte_val |= (1 << (7 - bit))
        screen_bytes.append(byte_val)

print(f"Screen bytes: {len(screen_bytes)}")
print()

# Build permutation: pattern_byte[i] maps to screen position (row, col)
# We need to find which screen byte each pattern byte becomes

permutation = []  # List of (pattern_idx, screen_idx)

for pattern_idx in range(len(pattern_bytes)):
    pattern_byte = pattern_bytes[pattern_idx]

    # Find this byte in screen_bytes
    # Note: same byte value might appear multiple times, so we need to track which ones we've used
    found = False
    for screen_idx in range(len(screen_bytes)):
        if screen_bytes[screen_idx] == pattern_byte:
            # Check if this screen_idx already used
            if screen_idx not in [p[1] for p in permutation]:
                screen_row = screen_idx // 6
                screen_byte_in_row = screen_idx % 6
                permutation.append((pattern_idx, screen_idx, screen_row, screen_byte_in_row, pattern_byte))
                found = True
                break

    if not found:
        print(f"Warning: pattern byte {pattern_idx} (value 0x{pattern_byte:02X}) not found uniquely in screen")

print(f"Built permutation table with {len(permutation)} entries")
print()

# Verify: decode using permutation and compare to screen
decoded_bytes = [0] * 288
for pattern_idx, screen_idx, _, _, _ in permutation:
    decoded_bytes[screen_idx] = pattern_bytes[pattern_idx]

matches = sum(1 for i in range(288) if decoded_bytes[i] == screen_bytes[i])
print(f"Verification: {matches}/288 bytes match")

if matches == 288:
    print("✅ Perfect match! Permutation table is correct.")

    # Save permutation table
    with open('permutation_table_part3.json', 'w') as f:
        json.dump({
            'description': 'Part 3 pattern byte to screen byte permutation',
            'source': 'Extracted from frame 900 comparison',
            'permutation': [{'pattern_idx': p[0], 'screen_idx': p[1],
                           'screen_row': p[2], 'screen_byte': p[3]}
                          for p in permutation]
        }, f, indent=2)

    print("✅ Saved permutation_table_part3.json")
    print()
    print("This table can now be used to decode ANY Part 3 pattern")
    print("purely from source data, without emulation!")
else:
    print(f"❌ Only {matches}/288 match - permutation extraction failed")
    print("This might mean bytes are transformed, not just permuted")
