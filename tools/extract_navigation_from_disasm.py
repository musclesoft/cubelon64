#!/usr/bin/env python3
"""
Extract exact navigation pattern directly from disassembly.

This reads full_disassembly.asm and traces the exact sequence of:
- POP BC (loads 2 bytes from pattern)
- LD (HL),C (writes C to screen at HL)
- [navigation] (INC L, INC H, DEC L, or LD DE+ADD)
- LD (HL),B (writes B to screen at HL)
- [navigation]

This gives us the exact write sequence without guessing.
"""

import re


def parse_disassembly(filename):
    """Parse disassembly and extract DJNZ loop instructions."""
    with open(filename) as f:
        lines = f.readlines()

    # Find the DJNZ loop: from $85AB (first POP BC) to $8668 (EXX before DEC B)
    in_loop = False
    instructions = []

    for line in lines:
        # Match instructions like "$85AB: POP BC               ; executed 24 times"
        match = re.match(r'\$([0-9A-F]{4}):\s+(.+?)\s+;', line)
        if not match:
            continue

        addr = int(match.group(1), 16)
        instr = match.group(2).strip()

        # Start of loop
        if addr == 0x85AB:
            in_loop = True

        if in_loop:
            instructions.append((addr, instr))

        # End of loop (before EXX)
        if addr == 0x8667:  # ADD HL,DE (last instruction before EXX)
            break

    return instructions


def trace_writes(instructions):
    """Trace through instructions and record write sequence."""
    HL = 0x4004  # Starting position
    byte_idx = 0
    write_map = []

    i = 0
    while i < len(instructions):
        addr, instr = instructions[i]

        if instr.startswith('POP BC'):
            # Next two writes are C and B
            # Skip to LD (HL),C
            i += 1
            assert instructions[i][1].startswith('LD (HL),C')

            # Record write of C
            write_map.append({
                'pattern_byte': byte_idx,
                'screen_addr': HL,
                'register': 'C',
                'instr_addr': instructions[i][0]
            })
            byte_idx += 1

            # Apply navigation after C
            i += 1
            nav_instr = instructions[i][1]
            if nav_instr.startswith('INC L'):
                HL = (HL & 0xFF00) | ((HL + 1) & 0xFF)
            elif nav_instr.startswith('DEC L'):
                HL = (HL & 0xFF00) | ((HL - 1) & 0xFF)
            elif nav_instr.startswith('INC H'):
                HL = ((HL + 0x100) & 0xFFFF)
            elif nav_instr.startswith('DEC H'):
                HL = ((HL - 0x100) & 0xFFFF)

            # Next is LD (HL),B
            i += 1
            assert instructions[i][1].startswith('LD (HL),B')

            # Record write of B
            write_map.append({
                'pattern_byte': byte_idx,
                'screen_addr': HL,
                'register': 'B',
                'instr_addr': instructions[i][0]
            })
            byte_idx += 1

            # Apply navigation after B (might be LD DE+ADD HL,DE)
            i += 1
            if i < len(instructions):
                nav_instr = instructions[i][1]
                if nav_instr.startswith('INC L'):
                    HL = (HL & 0xFF00) | ((HL + 1) & 0xFF)
                elif nav_instr.startswith('DEC L'):
                    HL = (HL & 0xFF00) | ((HL - 1) & 0xFF)
                elif nav_instr.startswith('INC H'):
                    HL = ((HL + 0x100) & 0xFFFF)
                elif nav_instr.startswith('DEC H'):
                    HL = ((HL - 0x100) & 0xFFFF)
                elif nav_instr.startswith('LD DE'):
                    # Extract constant
                    match = re.search(r'\$([0-9A-F]+)', nav_instr)
                    if match:
                        de_val = int(match.group(1), 16)
                        # Convert to signed
                        if de_val & 0x8000:
                            de_val -= 0x10000
                        # Next instruction should be ADD HL,DE
                        i += 1
                        assert instructions[i][1].startswith('ADD HL,DE')
                        HL = (HL + de_val) & 0xFFFF
        else:
            i += 1

    return write_map


def zx_coords_from_addr(addr):
    """Convert ZX Spectrum screen address to (y, x_byte)."""
    if addr < 0x4000 or addr >= 0x5800:
        return None, None

    offset = addr - 0x4000
    y7y6 = (offset >> 11) & 0x03
    y2y1y0 = (offset >> 8) & 0x07
    y5y4y3 = (offset >> 5) & 0x07
    x_byte = offset & 0x1F

    y = (y7y6 << 6) | (y5y4y3 << 3) | y2y1y0

    return y, x_byte


def main():
    print("Extracting navigation pattern from disassembly...")
    print()

    # Parse disassembly
    instructions = parse_disassembly('../emulator/full_disassembly.asm')
    print(f"✅ Parsed {len(instructions)} instructions from DJNZ loop")

    # Count POP BC
    pop_count = sum(1 for addr, instr in instructions if instr.startswith('POP BC'))
    print(f"✅ Found {pop_count} POP BC operations")
    print(f"   → {pop_count * 2} pattern bytes per iteration")
    print()

    # Trace writes
    write_map = trace_writes(instructions)
    print(f"✅ Traced {len(write_map)} writes")
    print()

    # Add coordinates
    for entry in write_map:
        y, x_byte = zx_coords_from_addr(entry['screen_addr'])
        entry['y'] = y
        entry['x_byte'] = x_byte

    # Show first 40
    print("First 40 writes:")
    print(f"{'Byte':<5} {'Addr':<8} {'Y':<5} {'XByte':<7} {'Reg'}")
    print("-" * 50)
    for i in range(min(40, len(write_map))):
        entry = write_map[i]
        y_str = str(entry['y']) if entry['y'] is not None else "???"
        x_str = str(entry['x_byte']) if entry['x_byte'] is not None else "?"
        print(f"{entry['pattern_byte']:<5} ${entry['screen_addr']:04X}    "
              f"{y_str:<5} {x_str:<7} {entry['register']}")

    print(f"\n... ({len(write_map)} total writes)")

    # Save
    import json
    output = {
        'description': 'Part 3 write sequence extracted from disassembly',
        'source': 'full_disassembly.asm DJNZ loop $85AB-$8667',
        'pop_bc_count': pop_count,
        'bytes_per_iteration': pop_count * 2,
        'writes': write_map
    }

    with open('part3_write_sequence.json', 'w') as f:
        json.dump(output, f, indent=2)

    print(f"\n✅ Saved part3_write_sequence.json")


if __name__ == '__main__':
    main()
