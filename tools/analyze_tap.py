#!/usr/bin/env python3
"""
Analyze ZX Spectrum TAP file structure
Shows blocks, types, load addresses, and lengths
"""

import sys
import struct

def analyze_tap(filename):
    with open(filename, 'rb') as f:
        data = f.read()

    offset = 0
    block_num = 0

    print(f"TAP File: {filename}")
    print(f"Total size: {len(data)} bytes")
    print("=" * 80)

    while offset < len(data):
        # Read block length (2 bytes, little endian)
        if offset + 2 > len(data):
            break

        block_len = struct.unpack('<H', data[offset:offset+2])[0]
        offset += 2

        if offset + block_len > len(data):
            print(f"ERROR: Block {block_num} extends past end of file")
            break

        block_data = data[offset:offset+block_len]
        offset += block_len

        if block_len == 0:
            continue

        flag = block_data[0]

        print(f"\nBlock {block_num}: {block_len} bytes (0x{block_len:04x})")
        print(f"  Flag: 0x{flag:02x} ", end="")

        if flag == 0x00:
            print("(Header)")
            if len(block_data) >= 19:
                block_type = block_data[1]
                type_names = {0: "Program", 1: "Number array", 2: "Character array", 3: "Code"}
                print(f"  Type: {block_type} - {type_names.get(block_type, 'Unknown')}")

                filename_bytes = block_data[2:12]
                filename_str = ''.join(chr(b) if 32 <= b < 127 else '.' for b in filename_bytes)
                print(f"  Filename: '{filename_str}'")

                data_len = struct.unpack('<H', block_data[12:14])[0]
                print(f"  Data length: {data_len} bytes (0x{data_len:04x})")

                if block_type == 0:  # Program (BASIC)
                    autostart = struct.unpack('<H', block_data[14:16])[0]
                    prog_len = struct.unpack('<H', block_data[16:18])[0]
                    print(f"  Autostart line: {autostart}")
                    print(f"  Program length: {prog_len}")
                elif block_type == 3:  # Code
                    start_addr = struct.unpack('<H', block_data[14:16])[0]
                    print(f"  Load address: 0x{start_addr:04x} ({start_addr})")

        elif flag == 0xff:
            print("(Data)")
            print(f"  Data size: {block_len - 1} bytes")
            # Show first few bytes
            preview = block_data[1:min(17, len(block_data))]
            hex_str = ' '.join(f'{b:02x}' for b in preview)
            print(f"  First bytes: {hex_str}...")

        block_num += 1

    print("\n" + "=" * 80)
    print(f"Total blocks: {block_num}")

if __name__ == '__main__':
    if len(sys.argv) < 2:
        print("Usage: analyze_tap.py <tap_file>")
        sys.exit(1)

    analyze_tap(sys.argv[1])
