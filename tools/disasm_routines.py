#!/usr/bin/env python3
"""
Disassemble specific helper routines for pattern generation
"""

import sys
sys.path.append('.')
from advanced_z80_disasm import AdvancedZ80Disassembler

def disasm_routine(code, start_addr, offset, length, name):
    """Disassemble a routine at a specific offset"""
    print("=" * 70)
    print(f"{name} - ${start_addr + offset:04X}")
    print("=" * 70)
    print()

    disasm = AdvancedZ80Disassembler(code, start_addr)
    disasm.pc = offset

    count = 0
    while disasm.pc < offset + length and disasm.pc < len(code) and count < 50:
        addr, mnem, size = disasm.disassemble_one()

        # Stop at RET
        print(f"${addr:04X}: {mnem}")

        if "RET" in mnem and not "RET " in mnem:  # RET but not RET cc
            break

        count += 1

    print()

def main():
    # Load block 02 - likely contains $7900-$9640 code
    with open('original/cubelon_data_02.bin', 'rb') as f:
        block02 = f.read()

    print(f"Block 02 size: {len(block02)} bytes")
    print(f"If loaded at $7900, covers $7900-${0x7900 + len(block02):04X}")
    print()

    # Disassemble key routines
    # These are at offsets from $7900:
    # $90A0 - $7900 = $17A0
    # $9115 - $7900 = $1815

    if 0x17A0 < len(block02):
        disasm_routine(block02, 0x7900, 0x17A0, 100, "Setup Routine at $90A0")

    if 0x1815 < len(block02):
        disasm_routine(block02, 0x7900, 0x1815, 100, "Process Routine at $9115")

    # Also check block 04 for the lookup table area
    print("=" * 70)
    print("Checking for Lookup Table Data")
    print("=" * 70)
    print()

    # Load block 04 - might contain $D006-$F988
    # But first, let's check the pattern data/state variables
    # $90E3 - $7900 = $17E3
    # $90FB - $7900 = $17FB

    if 0x17E3 < len(block02):
        print(f"Pattern data at offset $17E3 (address $90E3):")
        data = block02[0x17E3:0x17E3+32]
        print("  Hex: " + " ".join(f"{b:02X}" for b in data))
        print()

    if 0x17FB < len(block02):
        print(f"Animation state at offset $17FB (address $90FB):")
        data = block02[0x17FB:0x17FB+8]
        print("  Hex: " + " ".join(f"{b:02X}" for b in data))
        print()

    # Now try block 11 for $E63A
    # If loaded at $D006, then $E63A - $D006 = $1634
    print("=" * 70)
    print("Checking Block 11 for lookup table")
    print("=" * 70)
    print()

    try:
        with open('original/cubelon_data_11.bin', 'rb') as f:
            block11 = f.read()

        # Load 4: $D006 with source $0746
        # If block 11 is decompressed to this area
        # Offset $1634 might contain the table

        if 0x1634 < len(block11):
            print(f"Data at offset $1634 in block 11:")
            data = block11[0x1634:0x1634+64]
            print("  Hex: " + " ".join(f"{b:02X}" for b in data[:32]))
            print("       " + " ".join(f"{b:02X}" for b in data[32:]))
            print()
    except FileNotFoundError:
        print("Block 11 not found")


if __name__ == '__main__':
    main()
