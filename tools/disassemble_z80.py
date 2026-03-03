#!/usr/bin/env python3
"""
Z80 disassembly helper
Provides basic Z80 code analysis and can invoke external disassemblers
"""

import sys
import os
import subprocess

# Common Z80 instruction patterns
Z80_PATTERNS = {
    0xF3: "DI",
    0xFB: "EI",
    0xC9: "RET",
    0xC3: "JP",
    0xCD: "CALL",
    0x00: "NOP",
    0x76: "HALT",
    0x3E: "LD A,",
    0x06: "LD B,",
    0x0E: "LD C,",
    0x16: "LD D,",
    0x1E: "LD E,",
    0x26: "LD H,",
    0x2E: "LD L,",
}

def analyze_z80_code(data, start_addr=0x7800):
    """Basic analysis of Z80 code"""
    print(f"Z80 Code Analysis")
    print(f"=" * 70)
    print(f"Start address: ${start_addr:04X}")
    print(f"Size: {len(data)} bytes")
    print()

    # Look for common patterns
    print("Instruction distribution (first occurrences):")
    found_instructions = set()
    for i, byte in enumerate(data):
        if byte in Z80_PATTERNS and byte not in found_instructions:
            found_instructions.add(byte)
            print(f"  ${start_addr + i:04X}: {Z80_PATTERNS[byte]}")

    # Look for potential addresses (16-bit values in reasonable ranges)
    print("\nPotential address references (0x5000-0xFFFF):")
    for i in range(len(data) - 1):
        addr = data[i] | (data[i+1] << 8)
        if 0x5000 <= addr <= 0xFFFF and i < 50:  # First 50 bytes only
            print(f"  ${start_addr + i:04X}: ${addr:04X}")

    # Look for text strings
    print("\nPotential text strings:")
    current_string = []
    string_start = 0
    for i, byte in enumerate(data):
        if 32 <= byte <= 126:  # Printable ASCII
            if not current_string:
                string_start = i
            current_string.append(chr(byte))
        else:
            if len(current_string) >= 4:  # At least 4 chars
                print(f"  ${start_addr + string_start:04X}: \"{''.join(current_string)}\"")
            current_string = []

def try_external_disassembler(filename, start_addr=0x7800):
    """Try to use external Z80 disassembler if available"""
    print("\nTrying external disassemblers...")

    # Try z80dasm
    try:
        result = subprocess.run(['z80dasm', '--version'],
                                capture_output=True, text=True, timeout=2)
        if result.returncode == 0 or 'z80dasm' in result.stdout.lower():
            print("Found z80dasm!")
            print(f"\nTo disassemble, run:")
            print(f"  z80dasm -g {start_addr:04x} -a {filename} > disassembly.asm")
            return True
    except (subprocess.TimeoutExpired, FileNotFoundError):
        pass

    # Try z80dis
    try:
        result = subprocess.run(['z80dis', '-h'],
                                capture_output=True, text=True, timeout=2)
        if result.returncode == 0 or result.returncode == 1:
            print("Found z80dis!")
            print(f"\nTo disassemble, run:")
            print(f"  z80dis -o {start_addr:04x} {filename} > disassembly.asm")
            return True
    except (subprocess.TimeoutExpired, FileNotFoundError):
        pass

    print("No external disassembler found.")
    print("\nRecommended tools:")
    print("  - z80dasm: apt-get install z80dasm (Linux)")
    print("  - z80dis: Available online")
    print("  - Online: https://onlinedisassembler.com/")
    return False

def main():
    if len(sys.argv) < 2:
        print("Usage: disassemble_z80.py <code_file> [start_addr_hex]")
        print("\nAnalyzes Z80 machine code and helps with disassembly")
        print("Example: disassemble_z80.py cubelon_code.bin 7800")
        sys.exit(1)

    filename = sys.argv[1]
    start_addr = 0x7800

    if len(sys.argv) >= 3:
        try:
            start_addr = int(sys.argv[2], 16)
        except ValueError:
            print(f"Warning: Invalid hex address '{sys.argv[2]}', using default 0x7800")

    if not os.path.exists(filename):
        print(f"Error: File '{filename}' not found")
        sys.exit(1)

    with open(filename, 'rb') as f:
        data = f.read()

    analyze_z80_code(data, start_addr)
    print()
    try_external_disassembler(filename, start_addr)

    # Output first 256 bytes as hex dump
    print("\n\nHex dump (first 256 bytes):")
    print("-" * 70)
    for i in range(0, min(256, len(data)), 16):
        hex_str = ' '.join(f'{b:02X}' for b in data[i:i+16])
        ascii_str = ''.join(chr(b) if 32 <= b <= 126 else '.' for b in data[i:i+16])
        print(f"{start_addr + i:04X}: {hex_str:48s} {ascii_str}")

if __name__ == '__main__':
    main()
