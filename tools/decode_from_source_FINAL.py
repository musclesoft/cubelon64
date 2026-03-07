#!/usr/bin/env python3
"""
FINAL: Source-only decoder for Part 3 patterns.

Uses the complete write sequence extracted from disassembly to decode
576 pattern bytes into 48×48 pixels WITHOUT screen extraction!

This is the complete solution to the user's requirement:
"I would have preferred an extraction method that 100% relies on source data"
"""

import json
import base64


def decode_pattern_from_source(pattern_bytes_576, write_sequence):
    """
    Decode 576 pattern bytes to 48×48 pixels using write sequence.

    Args:
        pattern_bytes_576: All 576 bytes from 8 chunks (8 × 72 bytes)
        write_sequence: Write map from extract_navigation_from_disasm.py

    Returns:
        48×48 pixel array (0 or 1)
    """
    # Initialize 48×48 tile (48 rows × 6 bytes/row = 288 bytes)
    pixels = [[0] * 48 for _ in range(48)]

    # Track which screen bytes have been written
    screen_bytes_written = {}

    # Simulate all 8 DJNZ iterations
    for iteration in range(8):
        pattern_offset = iteration * 72  # Each iteration uses 72 bytes

        for write_idx in range(72):
            # Get write info from trace
            write_info = write_sequence[write_idx]
            pattern_byte_in_chunk = write_info['pattern_byte']
            screen_addr = write_info['screen_addr']
            y = write_info['y']
            x_byte = write_info['x_byte']

            # Get actual pattern byte
            pattern_byte = pattern_bytes_576[pattern_offset + pattern_byte_in_chunk]

            # Track last write to each screen address (last write wins)
            screen_bytes_written[screen_addr] = pattern_byte

    # Convert screen bytes to pixels
    for screen_addr, byte_val in screen_bytes_written.items():
        # Calculate (y, x_byte) from screen address
        if screen_addr < 0x4000 or screen_addr >= 0x5800:
            continue

        offset = screen_addr - 0x4000
        y7y6 = (offset >> 11) & 0x03
        y2y1y0 = (offset >> 8) & 0x07
        y5y4y3 = (offset >> 5) & 0x07
        x_byte = offset & 0x1F

        y = (y7y6 << 6) | (y5y4y3 << 3) | y2y1y0

        # Extract 8 pixels from byte
        for bit in range(8):
            col = (x_byte - 4) * 8 + bit  # x_byte=4 is column 0
            if 0 <= col < 48 and 0 <= y < 48:
                pixels[y][col] = (byte_val >> (7 - bit)) & 1

    return pixels


def main():
    print("=== Part 3: Source-Only Decoder ===")
    print()
    print("This decoder uses ONLY source data (pattern bytes), not screen extraction!")
    print()

    # Load write sequence
    with open('part3_write_sequence.json') as f:
        trace_data = json.load(f)
        write_sequence = trace_data['writes']

    print(f"✅ Loaded write sequence: {len(write_sequence)} writes per iteration")

    # Load pattern data with ALL 576 bytes
    with open('../emulator/part3_tiles_900-900.json') as f:
        tiles = json.load(f)

    frame900 = tiles[0]
    pattern_bytes = base64.b64decode(frame900['pattern_bytes'])
    screen_pixels = frame900['pixels']  # For verification only

    print(f"✅ Loaded pattern data: {len(pattern_bytes)} bytes")
    print(f"   (8 iterations × 72 bytes/iteration)")
    print()

    # Decode from source
    print("Decoding from source data...")
    decoded_pixels = decode_pattern_from_source(pattern_bytes, write_sequence)

    # Verify against screen extraction
    print("Verifying against screen extraction...")
    matches = 0
    mismatches = 0

    for y in range(48):
        for x in range(48):
            if decoded_pixels[y][x] == screen_pixels[y][x]:
                matches += 1
            else:
                mismatches += 1

    total_pixels = 48 * 48
    print(f"  Matches: {matches}/{total_pixels}")
    print(f"  Mismatches: {mismatches}/{total_pixels}")
    print()

    if mismatches == 0:
        print("✅✅✅ PERFECT MATCH! Source-only decoding works 100%!")
        print()
        print("User requirement satisfied:")
        print('  "I would have preferred an extraction method that 100%')
        print('   relies on source data instead on \'destination\' data"')
        print()
        print("This decoder:")
        print("  ✅ Uses ONLY pattern bytes (source data)")
        print("  ✅ Uses disassembly-extracted navigation (no screen reads)")
        print("  ✅ Produces identical results to screen extraction")
        print("  ✅ Can decode offline without emulation")
    else:
        print(f"❌ {mismatches} mismatches - debugging needed")

        # Show first few mismatches
        print("\nFirst 10 mismatches:")
        shown = 0
        for y in range(48):
            for x in range(48):
                if decoded_pixels[y][x] != screen_pixels[y][x]:
                    print(f"  ({y},{x}): decoded={decoded_pixels[y][x]}, screen={screen_pixels[y][x]}")
                    shown += 1
                    if shown >= 10:
                        break
            if shown >= 10:
                break


if __name__ == '__main__':
    main()
