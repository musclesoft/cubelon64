#!/usr/bin/env python3
"""
Reverse-engineer the pattern byte to screen pixel transformation.

We have:
- pattern_bytes: 288 bytes (from 8 chunks of 36 bytes each)
- screen_pixels: 48×48 pixels (extracted from screen, known correct)

Goal: Figure out how to transform pattern_bytes → screen_pixels
"""

import json
import base64

with open('../emulator/part3_tiles_900-960.json') as f:
    tiles = json.load(f)

frame920 = [t for t in tiles if t['frame'] == 920][0]
pattern_bytes = base64.b64decode(frame920['pattern_bytes'])
screen_pixels = frame920['pixels']

print("Analyzing frame 920...")
print(f"Pattern bytes: {len(pattern_bytes)} bytes")
print(f"Screen pixels: {len(screen_pixels)}×{len(screen_pixels[0])}")
print(f"Pattern addresses: {[f'${a:04X}' for a in frame920['pattern_addrs']]}")
print()

# Convert screen pixels to bytes for comparison
screen_bytes = []
for row in range(48):
    for byte_idx in range(6):
        byte_val = 0
        for bit in range(8):
            col = byte_idx * 8 + bit
            if screen_pixels[row][col] == 1:
                byte_val |= (1 << (7 - bit))
        screen_bytes.append(byte_val)

print("Comparing pattern bytes to screen bytes...")
print("If they match in some permutation, we just need to find the mapping.")
print()

# Check if pattern bytes appear anywhere in screen bytes
matches_found = 0
for i in range(len(pattern_bytes)):
    pattern_byte = pattern_bytes[i]
    for j in range(len(screen_bytes)):
        if pattern_byte == screen_bytes[j]:
            if i < 10:  # Only print first 10 matches
                print(f"  pattern[{i}] = screen[{j}] = 0x{pattern_byte:02X}")
            matches_found += 1
            break

print(f"\nDirect byte matches: {matches_found}/{len(pattern_bytes)}")

if matches_found < len(pattern_bytes) // 2:
    print("\nNot many direct matches - bytes might be transformed/reordered")
    print("Let me check if bytes are bit-reversed or otherwise transformed...")

    # Check bit-reversed
    bit_reversed_matches = 0
    for i in range(len(pattern_bytes)):
        pattern_byte = pattern_bytes[i]
        reversed_byte = int('{:08b}'.format(pattern_byte)[::-1], 2)
        if reversed_byte in screen_bytes:
            bit_reversed_matches += 1

    print(f"Bit-reversed matches: {bit_reversed_matches}/{len(pattern_bytes)}")

# Try to find the permutation by analyzing structure
print("\nAnalyzing chunk structure...")
print("Pattern is 8 chunks of 36 bytes each")
print("Screen is 48 rows of 6 bytes each")
print()

# Check each chunk separately
for chunk_idx in range(8):
    chunk_start = chunk_idx * 36
    chunk_end = chunk_start + 36
    chunk_bytes = pattern_bytes[chunk_start:chunk_end]

    # This chunk should map to some 6-row strip in the screen
    # Try each possible 6-row strip
    for strip_idx in range(8):  # 48 rows / 6 = 8 strips
        strip_start = strip_idx * 6
        strip_bytes = screen_bytes[strip_start * 6:(strip_start + 6) * 6]

        # Count how many bytes match
        matches = sum(1 for b in chunk_bytes if b in strip_bytes)
        if matches > 30:  # If most bytes match
            print(f"  Chunk {chunk_idx} might map to screen strip {strip_idx} ({matches}/36 bytes present)")
