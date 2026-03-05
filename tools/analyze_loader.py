#!/usr/bin/env python3
"""
Analyze CUBELON loader structure
Parses the data blocks to understand memory layout
"""

import sys
import struct

def analyze_loader_data(base_path):
    """Parse the loader data format"""

    print("CUBELON Loader Data Analysis")
    print("=" * 80)

    # The loader code is in cubelon_code.bin starting at offset 0x4D
    # It contains the unpacking data structure

    with open(f"{base_path}/cubelon_code.bin", 'rb') as f:
        loader_data = f.read()

    # Skip to the data structure (starts after the code)
    # The data structure starts at offset 0x4D in the file (0x784D in memory)
    offset = 0x4D

    print("\nLoader Data Structure:")
    print("-" * 80)

    block_idx = 4  # Data blocks start at block 4 in TAP file

    while offset < len(loader_data):
        if offset + 5 > len(loader_data):
            break

        bank = loader_data[offset]

        if bank == 0:  # End marker (or no data)
            break

        if bank == 8:  # Special: jump to start
            print(f"\nBank {bank}: JUMP TO START (0x7D00)")
            break

        offset += 1

        if offset + 4 > len(loader_data):
            break

        length = struct.unpack('<H', loader_data[offset:offset+2])[0]
        offset += 2

        dest_addr = struct.unpack('<H', loader_data[offset:offset+2])[0]
        offset += 2

        # The actual data comes from TAP blocks, not the loader
        print(f"\nBlock {block_idx}:")
        print(f"  Memory Bank: {bank & 0x07} (paging byte: 0x{(bank | 0x10):02x})")
        print(f"  Load Address: 0x{dest_addr:04x} ({dest_addr})")
        print(f"  Length: {length} bytes (0x{length:04x})")
        print(f"  TAP Block: Block {block_idx} (data block)")

        block_idx += 1

    print("\n" + "=" * 80)
    print("\nMemory Layout Summary:")
    print("-" * 80)
    print("ZX Spectrum 128K uses memory banking:")
    print("  Banks 0-7 can be paged into 0xC000-0xFFFF")
    print("  Port 0x7FFD controls paging")
    print("  Demo loads code/data into different banks")
    print("\nNote: The largest code blocks (data_10, data_11, data_12) likely")
    print("      contain the main demo code and should be disassembled at")
    print("      their respective load addresses.")

if __name__ == '__main__':
    if len(sys.argv) < 2:
        print("Usage: analyze_loader.py <path_to_original_dir>")
        sys.exit(1)

    analyze_loader_data(sys.argv[1])
