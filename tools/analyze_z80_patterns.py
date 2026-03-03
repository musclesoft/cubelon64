#!/usr/bin/env python3
"""
Analyze Z80 code for pattern generation algorithm
Focus on understanding how tiles are combined and transformed
"""

import sys

# Basic Z80 instruction decoder for key operations
Z80_OPS = {
    0x00: "NOP",
    0x01: "LD BC,nn",
    0x06: "LD B,n",
    0x0E: "LD C,n",
    0x10: "DJNZ e",
    0x11: "LD DE,nn",
    0x16: "LD D,n",
    0x1E: "LD E,n",
    0x21: "LD HL,nn",
    0x26: "LD H,n",
    0x2E: "LD L,n",
    0x3E: "LD A,n",
    0x76: "HALT",
    0x77: "LD (HL),A",
    0x7E: "LD A,(HL)",
    0x80: "ADD A,B",
    0x81: "ADD A,C",
    0x82: "ADD A,D",
    0x83: "ADD A,E",
    0x84: "ADD A,H",
    0x85: "ADD A,L",
    0x86: "ADD A,(HL)",
    0x87: "ADD A,A",
    0xA0: "AND B",
    0xA1: "AND C",
    0xA2: "AND D",
    0xA3: "AND E",
    0xA4: "AND H",
    0xA5: "AND L",
    0xA6: "AND (HL)",
    0xA7: "AND A",
    0xA8: "XOR B",
    0xA9: "XOR C",
    0xAA: "XOR D",
    0xAB: "XOR E",
    0xAC: "XOR H",
    0xAD: "XOR L",
    0xAE: "XOR (HL)",
    0xAF: "XOR A",
    0xB0: "OR B",
    0xB1: "OR C",
    0xB2: "OR D",
    0xB3: "OR E",
    0xB4: "OR H",
    0xB5: "OR L",
    0xB6: "OR (HL)",
    0xB7: "OR A",
    0xC3: "JP nn",
    0xC9: "RET",
    0xCD: "CALL nn",
    0xEE: "XOR n",
    0xF6: "OR n",
}

def disassemble_block(code, start_addr=0, max_instructions=100):
    """Simple Z80 disassembler focusing on key instructions"""
    pc = 0
    instructions = []

    while pc < len(code) and len(instructions) < max_instructions:
        addr = start_addr + pc
        opcode = code[pc]

        # Basic decoding
        if opcode in Z80_OPS:
            mnemonic = Z80_OPS[opcode]

            # Handle multi-byte instructions
            if 'nn' in mnemonic and pc + 2 < len(code):
                nn = code[pc+1] | (code[pc+2] << 8)
                instr = mnemonic.replace('nn', f'${nn:04X}')
                size = 3
            elif 'n' in mnemonic and pc + 1 < len(code):
                n = code[pc+1]
                instr = mnemonic.replace('n', f'${n:02X}')
                size = 2
            elif 'e' in mnemonic and pc + 1 < len(code):  # DJNZ
                offset = code[pc+1]
                if offset & 0x80:
                    offset = offset - 256
                target = addr + 2 + offset
                instr = mnemonic.replace('e', f'${target:04X}')
                size = 2
            else:
                instr = mnemonic
                size = 1

            instructions.append((addr, opcode, instr, size))
            pc += size
        else:
            # Unknown opcode, skip
            instructions.append((addr, opcode, f"DB ${opcode:02X}", 1))
            pc += 1

    return instructions

def find_pattern_routines(code):
    """Find routines that might generate patterns"""
    print("Analyzing Pattern Generation Routines")
    print("="*70)

    # Look for loops with XOR operations (likely pattern generation)
    loops_with_xor = []

    for i in range(len(code)-20):
        # Look for: DJNZ followed by XOR operations nearby
        if code[i] == 0x10:  # DJNZ
            # Check surrounding bytes for XOR
            window = code[max(0, i-15):min(len(code), i+15)]
            xor_count = sum(1 for b in window if b in [0xAF, 0xA8, 0xA9, 0xAA, 0xAB, 0xAC, 0xAD, 0xAE, 0xEE])

            if xor_count >= 2:
                loops_with_xor.append((i, xor_count))

    print(f"\nFound {len(loops_with_xor)} loops with XOR operations nearby")

    # Disassemble these interesting regions
    for offset, xor_count in loops_with_xor[:5]:  # First 5
        print(f"\n--- Loop at ${offset:04X} ({xor_count} XOR ops nearby) ---")

        # Disassemble 20 instructions before and after
        start = max(0, offset - 40)
        instructions = disassemble_block(code[start:start+100], start, 30)

        for addr, opcode, instr, size in instructions:
            marker = " <--" if addr == offset else ""
            print(f"  ${addr:04X}: {instr:20s} {marker}")

def analyze_data_access(code):
    """Find where code accesses tile data"""
    print("\n" + "="*70)
    print("Analyzing Data Access Patterns")
    print("="*70)

    # Look for LD HL,nn instructions (loading data pointers)
    data_loads = []

    for i in range(len(code)-2):
        if code[i] == 0x21:  # LD HL,nn
            addr = code[i+1] | (code[i+2] << 8)
            data_loads.append((i, addr))

    print(f"\nFound {len(data_loads)} data pointer loads (LD HL,nn)")
    print("First 10 addresses loaded:")

    for offset, addr in data_loads[:10]:
        print(f"  ${offset:04X}: LD HL,${addr:04X}")

    # Group by address range
    print("\nAddress ranges accessed:")
    ranges = {
        "Low ($0000-$3FFF)": sum(1 for _, a in data_loads if a < 0x4000),
        "Screen ($4000-$5FFF)": sum(1 for _, a in data_loads if 0x4000 <= a < 0x6000),
        "Data ($6000-$BFFF)": sum(1 for _, a in data_loads if 0x6000 <= a < 0xC000),
        "High ($C000-$FFFF)": sum(1 for _, a in data_loads if a >= 0xC000),
    }

    for name, count in ranges.items():
        if count > 0:
            print(f"  {name}: {count}")

def main():
    # Load code blocks
    with open('original/cubelon_data_10.bin', 'rb') as f:
        code10 = f.read()

    print(f"Loaded data_10: {len(code10)} bytes")
    print()

    find_pattern_routines(code10)
    analyze_data_access(code10)

    print("\n" + "="*70)
    print("Analysis complete. Key findings will be in output above.")
    print("Look for:")
    print("  - Loops with XOR/OR operations (pattern combination)")
    print("  - Rotate operations (pattern transformation)")
    print("  - Data pointer loads (accessing tile data)")

if __name__ == '__main__':
    main()
