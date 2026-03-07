#!/usr/bin/env python3
"""
Generate Part 3 permutation table from complete navigation constants.

Now that all three LD DE constants have been found ($F920, $F920, $F8C3),
we can simulate the exact 288-byte write sequence and build a complete
permutation table for source-only decoding.

Constants from disassembly (frame 900 trace):
- Section 1→2 adjustment: LD DE,$F920 at $85E6 (-1,760 bytes)
- Section 2→3 adjustment: LD DE,$F920 at $8625 (-1,760 bytes)
- End-of-iteration:       LD DE,$F8C3 at $8664 (-1,853 bytes)
"""

import json


def zx_coords_from_addr(addr):
    """Convert ZX Spectrum screen address to (y, x_byte)."""
    if addr < 0x4000 or addr >= 0x5800:
        return None, None

    offset = addr - 0x4000
    y7y6 = (offset >> 11) & 0x03
    y2y1y0 = (offset >> 8) & 0x07
    y5y4y3 = (offset >> 5) & 0x07
    x_byte = offset & 0x1F

    # Reconstruct Y from interleaved bits
    y = (y7y6 << 6) | (y5y4y3 << 3) | y2y1y0

    return y, x_byte


def signed_16bit(value):
    """Convert 16-bit unsigned to signed (two's complement)."""
    if value & 0x8000:
        return value - 0x10000
    return value


def simulate_write_sequence():
    """
    Simulate complete 288-byte write sequence through 8 DJNZ iterations.

    Returns list of (pattern_byte_index, screen_address, y, x_byte) tuples.
    """

    # Navigation pattern within each section (from disassembly)
    # Each section has 6 POP BC operations = 12 bytes
    # Pattern: [nav_after_C, nav_after_B] for each POP BC

    section_nav_pattern = [
        # Bytes 0-1: INC L, INC L
        (['INC_L'], ['INC_L']),
        # Bytes 2-3: INC H, DEC L
        (['INC_H'], ['DEC_L']),
        # Bytes 4-5: DEC L, INC H
        (['DEC_L'], ['INC_H']),
        # Bytes 6-7: INC L, INC L
        (['INC_L'], ['INC_L']),
        # Bytes 8-9: INC H, DEC L
        (['INC_H'], ['DEC_L']),
        # Bytes 10-11: DEC L, INC H
        (['DEC_L'], ['INC_H']),
    ]

    def apply_navigation(HL, nav_ops):
        """Apply navigation operations to HL."""
        H = (HL >> 8) & 0xFF
        L = HL & 0xFF

        for op in nav_ops:
            if op == 'INC_L':
                L = (L + 1) & 0xFF
            elif op == 'DEC_L':
                L = (L - 1) & 0xFF
            elif op == 'INC_H':
                H = (H + 1) & 0xFF
            elif op == 'DEC_H':
                H = (H - 1) & 0xFF

        return (H << 8) | L

    # Starting position: top-left tile at x=32 pixels, y=0
    HL = 0x4004

    write_map = []
    byte_idx = 0

    # 8 DJNZ iterations
    for djnz_iter in range(8):
        # Section 1: Bytes 0-11 (6 POP BC = 12 bytes)
        for pop_idx in range(6):
            nav_after_C, nav_after_B = section_nav_pattern[pop_idx]

            # Write C register
            y, x_byte = zx_coords_from_addr(HL)
            write_map.append({
                'pattern_byte': byte_idx,
                'screen_addr': HL,
                'y': y,
                'x_byte': x_byte,
                'djnz_iter': djnz_iter,
                'section': 0,
                'register': 'C'
            })
            byte_idx += 1
            HL = apply_navigation(HL, nav_after_C)

            # Write B register
            y, x_byte = zx_coords_from_addr(HL)
            write_map.append({
                'pattern_byte': byte_idx,
                'screen_addr': HL,
                'y': y,
                'x_byte': x_byte,
                'djnz_iter': djnz_iter,
                'section': 0,
                'register': 'B'
            })
            byte_idx += 1
            HL = apply_navigation(HL, nav_after_B)

        # Adjustment: LD DE,$F920; ADD HL,DE
        HL = (HL + signed_16bit(0xF920)) & 0xFFFF

        # Section 2: Bytes 12-23 (6 POP BC = 12 bytes)
        for pop_idx in range(6):
            nav_after_C, nav_after_B = section_nav_pattern[pop_idx]

            # Write C register
            y, x_byte = zx_coords_from_addr(HL)
            write_map.append({
                'pattern_byte': byte_idx,
                'screen_addr': HL,
                'y': y,
                'x_byte': x_byte,
                'djnz_iter': djnz_iter,
                'section': 1,
                'register': 'C'
            })
            byte_idx += 1
            HL = apply_navigation(HL, nav_after_C)

            # Write B register
            y, x_byte = zx_coords_from_addr(HL)
            write_map.append({
                'pattern_byte': byte_idx,
                'screen_addr': HL,
                'y': y,
                'x_byte': x_byte,
                'djnz_iter': djnz_iter,
                'section': 1,
                'register': 'B'
            })
            byte_idx += 1
            HL = apply_navigation(HL, nav_after_B)

        # Adjustment: LD DE,$F920; ADD HL,DE
        HL = (HL + signed_16bit(0xF920)) & 0xFFFF

        # Section 3: Bytes 24-35 (6 POP BC = 12 bytes)
        for pop_idx in range(6):
            nav_after_C, nav_after_B = section_nav_pattern[pop_idx]

            # Write C register
            y, x_byte = zx_coords_from_addr(HL)
            write_map.append({
                'pattern_byte': byte_idx,
                'screen_addr': HL,
                'y': y,
                'x_byte': x_byte,
                'djnz_iter': djnz_iter,
                'section': 2,
                'register': 'C'
            })
            byte_idx += 1
            HL = apply_navigation(HL, nav_after_C)

            # Write B register
            y, x_byte = zx_coords_from_addr(HL)
            write_map.append({
                'pattern_byte': byte_idx,
                'screen_addr': HL,
                'y': y,
                'x_byte': x_byte,
                'djnz_iter': djnz_iter,
                'section': 2,
                'register': 'B'
            })
            byte_idx += 1
            HL = apply_navigation(HL, nav_after_B)

        # End-of-iteration adjustment: LD DE,$F8C3; ADD HL,DE
        HL = (HL + signed_16bit(0xF8C3)) & 0xFFFF

    return write_map


def main():
    print("Generating Part 3 permutation table from navigation constants...")
    print()

    # Simulate complete write sequence
    write_map = simulate_write_sequence()

    print(f"✅ Generated {len(write_map)} write entries (expected 288)")

    if len(write_map) != 288:
        print(f"❌ ERROR: Expected 288 bytes but got {len(write_map)}")
        return

    # Verify all pattern bytes are present and unique
    pattern_bytes_seen = set()
    for entry in write_map:
        pattern_bytes_seen.add(entry['pattern_byte'])

    if len(pattern_bytes_seen) != 288:
        print(f"❌ ERROR: Expected 288 unique pattern bytes but got {len(pattern_bytes_seen)}")
        return

    if pattern_bytes_seen != set(range(288)):
        print(f"❌ ERROR: Pattern byte indices are not 0-287")
        return

    print("✅ All 288 pattern bytes present and sequential")
    print()

    # Show first 20 entries
    print("First 20 write operations:")
    print(f"{'Byte':<5} {'Addr':<8} {'Y':<4} {'XByte':<6} {'DJNZ':<5} {'Sect':<5} {'Reg'}")
    print("-" * 60)
    for i in range(20):
        entry = write_map[i]
        print(f"{entry['pattern_byte']:<5} ${entry['screen_addr']:04X}    "
              f"{entry['y']:<4} {entry['x_byte']:<6} {entry['djnz_iter']:<5} "
              f"{entry['section']:<5} {entry['register']}")

    print()
    print("Last 20 write operations:")
    for i in range(len(write_map) - 20, len(write_map)):
        entry = write_map[i]
        print(f"{entry['pattern_byte']:<5} ${entry['screen_addr']:04X}    "
              f"{entry['y']:<4} {entry['x_byte']:<6} {entry['djnz_iter']:<5} "
              f"{entry['section']:<5} {entry['register']}")

    print()

    # Save permutation table
    output = {
        'description': 'Part 3 pattern byte to screen position permutation table',
        'generated_from': 'Navigation constants: $F920, $F920, $F8C3',
        'start_hl': 0x4004,
        'tile_size': {'width': 48, 'height': 48},
        'bytes_per_tile': 288,
        'djnz_iterations': 8,
        'constants': {
            'section1_to_2': {'hex': '0xF920', 'decimal': -1760},
            'section2_to_3': {'hex': '0xF920', 'decimal': -1760},
            'end_of_iteration': {'hex': '0xF8C3', 'decimal': -1853}
        },
        'permutation': write_map
    }

    with open('part3_permutation_table.json', 'w') as f:
        json.dump(output, f, indent=2)

    print("✅ Saved part3_permutation_table.json")
    print()
    print("This table enables source-only decoding of any Part 3 pattern")
    print("without emulation or screen extraction!")


if __name__ == '__main__':
    main()
