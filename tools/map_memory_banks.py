#!/usr/bin/env python3
"""
Map memory addresses to TAP blocks based on loader table
Uses the banking analysis to determine which block contains which address
"""

import struct

# From BANKING_ANALYSIS.md - loader table at $784D
# Format: bank, dest_low, dest_high, source_low, source_high
# Repeats until bank $08 (end marker)

def parse_loader_table():
    """Extract the load table from the loader binary"""
    with open('original/cubelon_code.bin', 'rb') as f:
        loader = f.read()

    # Table starts at offset $4D (from $7800)
    offset = 0x4D
    loads = []

    while offset < len(loader):
        bank = loader[offset]
        if bank == 0x08:  # End marker
            break

        if offset + 4 >= len(loader):
            break

        dest = struct.unpack('<H', loader[offset+1:offset+3])[0]
        source = struct.unpack('<H', loader[offset+3:offset+5])[0]

        loads.append({
            'bank': bank,
            'dest': dest,
            'source': source,
            'offset': offset
        })

        offset += 5  # Each entry is 5 bytes

    return loads

def find_block_for_address(address, loads):
    """Find which load operation covers a given address"""
    candidates = []

    for i, load in enumerate(loads):
        dest_start = load['dest']
        # Estimate size (next load's dest - this dest, or 16KB max)
        if i + 1 < len(loads) and loads[i+1]['bank'] == load['bank']:
            dest_end = loads[i+1]['dest']
        else:
            dest_end = dest_start + 0x4000  # Max 16KB

        if dest_start <= address < dest_end:
            candidates.append({
                'load_index': i,
                'bank': load['bank'],
                'dest_start': dest_start,
                'dest_end': dest_end,
                'source': load['source'],
                'offset_in_block': address - dest_start,
                'source_offset': load['source'] + (address - dest_start)
            })

    return candidates

def map_tap_blocks():
    """Map TAP blocks to load operations"""
    import os

    # Get all data blocks
    blocks = []
    for i in range(1, 13):
        filename = f'original/cubelon_data_{i:02d}.bin'
        if os.path.exists(filename):
            size = os.path.getsize(filename)
            blocks.append({
                'number': i,
                'filename': filename,
                'size': size
            })

    return blocks

def main():
    print("=" * 70)
    print("MEMORY ADDRESS TO TAP BLOCK MAPPER")
    print("=" * 70)
    print()

    loads = parse_loader_table()

    print(f"Found {len(loads)} load operations:\n")
    for i, load in enumerate(loads):
        print(f"{i:2d}. Bank {load['bank']} → ${load['dest']:04X} from source ${load['source']:04X}")

    print()
    print("=" * 70)
    print()

    # Key addresses we want to map
    addresses = [
        ('Pattern data', 0x90E3),
        ('Animation state', 0x90FB),
        ('Lookup table', 0xE63A),
        ('Setup routine', 0x90A0),
        ('Process routine', 0x9115),
        ('Additional routine', 0xE38A),
        ('Screen buffer', 0x9300),
        ('Pattern buffer', 0x9300),
    ]

    print("Mapping key addresses to load operations:\n")

    for name, addr in addresses:
        print(f"{name} at ${addr:04X}:")

        candidates = find_block_for_address(addr, loads)

        if not candidates:
            print(f"  ❌ Not found in any load operation")
        else:
            for c in candidates:
                print(f"  ✓ Load {c['load_index']}: Bank {c['bank']}, "
                      f"Dest ${c['dest_start']:04X}-${c['dest_end']:04X}, "
                      f"Source ${c['source']:04X}")
                print(f"    Offset in dest: ${c['offset_in_block']:04X}")
                print(f"    Source offset: ${c['source_offset']:04X}")

        print()

    print("=" * 70)
    print()

    # Try to extract the lookup table
    print("Attempting to extract lookup table at $E63A:\n")

    candidates = find_block_for_address(0xE63A, loads)

    if candidates:
        c = candidates[0]
        print(f"Found in load {c['load_index']}: Bank {c['bank']}")
        print(f"Source offset: ${c['source_offset']:04X}")

        # The source is relative to the decompressed data
        # We need to figure out which TAP block this corresponds to

        print()
        print("TAP block mapping (estimated):")
        blocks = map_tap_blocks()

        for block in blocks:
            print(f"  Block {block['number']:2d}: {block['size']:5d} bytes - {block['filename']}")

    print()
    print("=" * 70)
    print("Note: Source addresses are relative to decompressed data in each bank.")
    print("TAP blocks may be compressed, so direct offset mapping is complex.")


if __name__ == '__main__':
    main()
