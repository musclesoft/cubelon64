#!/usr/bin/env python3
"""
Trace and document the pattern generation routine
Focus on the high-scoring loop at $C05A
"""

import sys
sys.path.append('.')
from advanced_z80_disasm import AdvancedZ80Disassembler

def trace_routine(code, start_offset, length=200):
    """Disassemble and annotate a routine"""
    disasm = AdvancedZ80Disassembler(code, 0xC000)
    disasm.pc = start_offset

    results = []
    while disasm.pc < start_offset + length and disasm.pc < len(code):
        addr, mnem, size = disasm.disassemble_one()
        results.append((addr, mnem))

    return results

def main():
    with open('original/cubelon_data_10.bin', 'rb') as f:
        code = f.read()

    print("=" * 70)
    print("PATTERN GENERATION ROUTINE ANALYSIS")
    print("=" * 70)
    print()

    # Trace from before the high-scoring loop at $C05A
    # Start at $C000 to get full context
    print("Complete routine from $C000 to $C100:")
    print()

    instructions = trace_routine(code, 0x000, 256)

    for addr, mnem in instructions:
        # Highlight key operations
        highlight = ""
        if any(x in mnem for x in ["XOR", "OR ", "RL", "RR", "SLA", "SRA", "SRL"]):
            highlight = "  *** PATTERN OP"
        if "DJNZ" in mnem:
            highlight = "  *** LOOP ***"
        if "CALL" in mnem:
            highlight = f"  (subroutine)"

        print(f"${addr:04X}: {mnem:30s}{highlight}")

    print()
    print("=" * 70)
    print()

    # Now look at the region around $C05A more carefully
    print("Detailed analysis of loop at $C05A:")
    print()

    # Start from $C040 (a bit before the interesting part)
    instructions = trace_routine(code, 0x040, 100)

    print("This appears to be bank switching code:")
    print()

    for addr, mnem in instructions:
        comment = ""

        if addr == 0xC041:
            comment = "; Load pattern selector?"
        elif addr == 0xC04B:
            comment = "; Port $7FFD = 128K banking!"
        elif addr == 0xC04E:
            comment = "; Switch bank via OUT"
        elif addr == 0xC050:
            comment = "; Return from bank switch"
        elif addr == 0xC051:
            comment = "; *** START OF PATTERN ROUTINE? ***"
        elif addr == 0xC054:
            comment = "; Call subroutine at $9115"
        elif addr == 0xC057:
            comment = "; XOR with tile data!"
        elif addr == 0xC05A:
            comment = "; *** LOOP - Process multiple tiles ***"

        highlight = ""
        if any(x in mnem for x in ["XOR", "OR"]):
            highlight = "  <<< COMBINE"
        if any(x in mnem for x in ["RL", "RR"]):
            highlight = "  <<< ROTATE"

        print(f"${addr:04X}: {mnem:30s}{comment}{highlight}")

    print()
    print("=" * 70)
    print("KEY FINDINGS:")
    print("=" * 70)
    print()
    print("1. The code at $C04B-$C050 performs 128K bank switching")
    print("2. Pattern generation likely starts at $C051")
    print("3. Calls subroutine at $90A0 and $9115 (helper functions)")
    print("4. XOR operation at $C057 combines tiles")
    print("5. DJNZ loop at $C05A processes 32 ($20) iterations")
    print()
    print("NEXT STEPS:")
    print("- Disassemble subroutines at $90A0 and $9115")
    print("- Trace where tile data is loaded from")
    print("- Understand the loop counter (register B)")
    print("- Document the complete algorithm")


if __name__ == '__main__':
    main()
