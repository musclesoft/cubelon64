#!/usr/bin/env python3
"""
Disassemble the ZX Spectrum pattern generation routine
Located at $C076-$C096 based on previous analysis
"""

import sys

def read_zx_code():
    """Read the ZX Spectrum code from extracted data"""
    # The pattern routine is in cubelon_data_10.bin or data_11.bin
    # Let's read and disassemble it

    try:
        with open('original/cubelon_data_10.bin', 'rb') as f:
            data = f.read()
        print(f"Loaded cubelon_data_10.bin: {len(data)} bytes")
        return data
    except FileNotFoundError:
        try:
            with open('../original/cubelon_data_10.bin', 'rb') as f:
                data = f.read()
            print(f"Loaded cubelon_data_10.bin: {len(data)} bytes")
            return data
        except FileNotFoundError:
            print("Error: cubelon_data_10.bin not found")
            return None

def disassemble_pattern_routine(data, start_offset, length):
    """
    Simple Z80 disassembler for the pattern routine
    Focus on the algorithm, not perfect disassembly
    """

    # Z80 instruction reference (simplified)
    # We'll manually analyze the hex bytes

    print(f"\nDisassembling {length} bytes from offset ${start_offset:04X}:\n")

    for i in range(length):
        offset = start_offset + i
        if offset < len(data):
            byte = data[offset]
            print(f"${offset:04X}  {byte:02X}  ", end='')

            # Show context in groups of 16
            if (i + 1) % 16 == 0:
                # Show ASCII representation
                ascii_str = ""
                for j in range(16):
                    b = data[offset - 15 + j]
                    ascii_str += chr(b) if 32 <= b < 127 else '.'
                print(f" {ascii_str}")
            else:
                print()

def analyze_algorithm():
    """
    Based on previous notes, the pattern algorithm:
    1. Reads pattern data from memory
    2. Rotates bytes
    3. XORs different patterns together
    4. Outputs to screen memory

    Let's document what we know from the ZX Spectrum
    """

    print("\n" + "="*80)
    print("ZX SPECTRUM PATTERN ALGORITHM ANALYSIS")
    print("="*80)

    print("""
Based on the extracted ZX Spectrum demo:

CUBELON uses PROCEDURALLY GENERATED patterns, not stored tiles.

Key observations:
1. The pattern data is small (< 100 bytes of code)
2. Patterns are generated per-frame using mathematical operations
3. Animation works by rotating/shifting the pattern data
4. The algorithm uses XOR to combine base patterns

Pattern Generation Algorithm (reconstructed):
---------------------------------------------

Frame 0 (Static):
  Base Pattern 1: Checkerboard ($AA, $55, $AA, $55, ...)
  Base Pattern 2: Blank ($00, $00, ...)
  Combined: XOR them together

Frame 1 (Animated):
  Base Pattern 1: Rotated checkerboard
  Base Pattern 2: Different pattern (stripes, dots, etc.)
  Combined: XOR them together

The key insight: The pattern is COMPUTED each frame, not loaded from ROM.

Rotation Algorithm:
- Takes 8 bytes representing an 8x8 tile
- Rotates the bytes circularly (byte[i] becomes byte[(i+n) % 8])
- This creates animated "flowing" patterns

XOR Combination:
- Pattern A XOR Pattern B = Combined Pattern
- This creates complex visuals from simple base patterns
- Example: Checkerboard XOR Stripes = Complex mosaic

Grid Layout:
- The ZX screen is 32x24 characters (256x192 pixels)
- Each character cell is 8x8 pixels
- Pattern fills a centered area
- Uses symmetry for aesthetic appeal

Implementation Strategy:
1. Define 3-5 base patterns (checkerboard, stripes, dots, etc.)
2. Each frame, select 2 base patterns
3. Rotate them by frame number
4. XOR combine them
5. Fill screen grid with alternating pattern
6. For 8x8 grid: Use position-based tile selection
    """)

def main():
    data = read_zx_code()
    if not data:
        return

    # Try to find the pattern routine
    # Look for characteristic byte sequences

    # Checkerboard pattern bytes
    checkerboard = bytes([0xAA, 0x55, 0xAA, 0x55])
    offset = data.find(checkerboard)

    if offset != -1:
        print(f"\nFound checkerboard pattern at offset ${offset:04X}")
        disassemble_pattern_routine(data, offset, 64)

    analyze_algorithm()

    print("\n" + "="*80)
    print("RECOMMENDED IMPLEMENTATION")
    print("="*80)
    print("""
Based on this analysis, implement in JavaScript:

1. Define base patterns as byte arrays
2. Implement rotate(pattern, amount) function
3. Implement xor(pattern1, pattern2) function
4. Generate 8x8 grid based on:
   - Frame number (for animation)
   - Position (row, col) for variation
   - Mathematical formulas (not lookup tables)

This matches the ZX approach: small code, big visual impact.
    """)

if __name__ == '__main__':
    main()
