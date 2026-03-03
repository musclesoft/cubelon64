#!/usr/bin/env python3
"""
Comprehensive Z80 disassembler for CUBELON
Focuses on tracing execution and understanding algorithms
"""

import sys

class Z80Disassembler:
    def __init__(self, code, start_addr=0x7800):
        self.code = code
        self.start_addr = start_addr
        self.labels = {}
        self.comments = {}

    def get_byte(self, offset):
        return self.code[offset] if offset < len(self.code) else 0

    def get_word(self, offset):
        if offset + 1 < len(self.code):
            return self.code[offset] | (self.code[offset+1] << 8)
        return 0

    def disassemble_instruction(self, offset):
        """Disassemble single instruction, return (mnemonic, size, target_addr)"""
        addr = self.start_addr + offset
        opcode = self.get_byte(offset)

        # Handle ED prefix (extended instructions)
        if opcode == 0xED:
            if offset + 1 >= len(self.code):
                return "DB $ED", 1, None
            opcode2 = self.get_byte(offset + 1)
            if opcode2 == 0x79:
                return "OUT (C),A", 2, None
            elif opcode2 == 0x78:
                return "IN A,(C)", 2, None
            elif opcode2 == 0xB0:
                return "LDIR", 2, None
            else:
                return f"DB $ED, ${opcode2:02X}", 2, None

        # Handle DD/FD prefixes (IX/IY)
        if opcode == 0xDD:
            if offset + 1 >= len(self.code):
                return "DB $DD", 1, None
            opcode2 = self.get_byte(offset + 1)
            if opcode2 == 0xE1:
                return "POP IX", 2, None
            else:
                return f"DB $DD, ${opcode2:02X}", 2, None

        if opcode == 0xFD:
            return "DB $FD", 1, None

        # Standard opcodes
        opcodes = {
            0x00: ("NOP", 1, None),
            0x01: ("LD BC,${:04X}", 3, None),
            0x06: ("LD B,${:02X}", 2, None),
            0x0E: ("LD C,${:02X}", 2, None),
            0x11: ("LD DE,${:04X}", 3, None),
            0x16: ("LD D,${:02X}", 2, None),
            0x18: ("JR ${:04X}", 2, "rel"),
            0x1E: ("LD E,${:02X}", 2, None),
            0x21: ("LD HL,${:04X}", 3, None),
            0x23: ("INC HL", 1, None),
            0x26: ("LD H,${:02X}", 2, None),
            0x2E: ("LD L,${:02X}", 2, None),
            0x32: ("LD (${:04X}),A", 3, None),
            0x36: ("LD (HL),${:02X}", 2, None),
            0x37: ("SCF", 1, None),
            0x3E: ("LD A,${:02X}", 2, None),
            0x46: ("LD B,(HL)", 1, None),
            0x4E: ("LD C,(HL)", 1, None),
            0x56: ("LD D,(HL)", 1, None),
            0x5E: ("LD E,(HL)", 1, None),
            0x76: ("HALT", 1, None),
            0x77: ("LD (HL),A", 1, None),
            0x7E: ("LD A,(HL)", 1, None),
            0x80: ("ADD A,B", 1, None),
            0x86: ("ADD A,(HL)", 1, None),
            0x87: ("ADD A,A", 1, None),
            0xA7: ("AND A", 1, None),
            0xAF: ("XOR A", 1, None),
            0xB7: ("OR A", 1, None),
            0xC1: ("POP BC", 1, None),
            0xC3: ("JP ${:04X}", 3, "abs"),
            0xC5: ("PUSH BC", 1, None),
            0xC9: ("RET", 1, None),
            0xCA: ("JP Z,${:04X}", 3, "abs"),
            0xCD: ("CALL ${:04X}", 3, "abs"),
            0xD5: ("PUSH DE", 1, None),
            0xE1: ("POP HL", 1, None),
            0xE5: ("PUSH HL", 1, None),
            0xF3: ("DI", 1, None),
            0xF6: ("OR ${:02X}", 2, None),
            0xFB: ("EI", 1, None),
            0xFE: ("CP ${:02X}", 2, None),
        }

        # DJNZ special case
        if opcode == 0x10:
            offset_byte = self.get_byte(offset + 1)
            if offset_byte & 0x80:
                offset_val = offset_byte - 256
            else:
                offset_val = offset_byte
            target = addr + 2 + offset_val
            return f"DJNZ ${target:04X}", 2, target

        if opcode in opcodes:
            template, size, target_type = opcodes[opcode]

            if size == 1:
                return template, size, None
            elif size == 2:
                val = self.get_byte(offset + 1)
                if target_type == "rel":
                    if val & 0x80:
                        val = val - 256
                    target = addr + 2 + val
                    return template.format(target), size, target
                return template.format(val), size, None
            elif size == 3:
                val = self.get_word(offset + 1)
                return template.format(val), size, val if target_type == "abs" else None

        return f"DB ${opcode:02X}", 1, None

    def disassemble_range(self, start_offset, length):
        """Disassemble a range of code"""
        offset = start_offset
        end = min(start_offset + length, len(self.code))

        output = []
        while offset < end:
            addr = self.start_addr + offset

            # Get bytes for display
            bytes_str = ""
            mnemonic, size, target = self.disassemble_instruction(offset)

            for i in range(min(size, 4)):
                if offset + i < len(self.code):
                    bytes_str += f"{self.code[offset+i]:02X} "
            bytes_str = bytes_str.ljust(12)

            # Add label if this address is a jump target
            label = self.labels.get(addr, "")
            if label:
                output.append(f"\n{label}:")

            comment = self.comments.get(addr, "")
            if comment:
                mnemonic = f"{mnemonic:30s} ; {comment}"

            output.append(f"${addr:04X}:  {bytes_str} {mnemonic}")
            offset += size

        return "\n".join(output)

def main():
    print("Z80 Disassembler for CUBELON")
    print("="*70)

    # Disassemble loader
    print("\n=== LOADER CODE (cubelon_code.bin) at $7800 ===\n")
    with open('original/cubelon_code.bin', 'rb') as f:
        loader = f.read()

    disasm = Z80Disassembler(loader, 0x7800)

    # Add known comments
    disasm.comments[0x7800] = "Disable interrupts"
    disasm.comments[0x7801] = "Set stack pointer"
    disasm.comments[0x7804] = "Load data pointer"
    disasm.comments[0x780A] = "Check for value 08"
    disasm.comments[0x780D] = "Jump if zero"
    disasm.comments[0x7810] = "OR with 10"
    disasm.comments[0x7812] = "Store to $5B5C (system var)"
    disasm.comments[0x7815] = "Load BC with $7FFD (128K port)"
    disasm.comments[0x7818] = "OUT to port (bank switch!)"

    print(disasm.disassemble_range(0, len(loader)))

    # Disassemble start of block 10
    print("\n\n=== BLOCK 10 (Main Code) - First 256 bytes ===\n")
    with open('original/cubelon_data_10.bin', 'rb') as f:
        code10 = f.read()

    disasm10 = Z80Disassembler(code10, 0xC000)  # Assume loaded at $C000
    print(disasm10.disassemble_range(0, 256))

    print("\n" + "="*70)
    print("Disassembly complete. Key findings:")
    print("  - Loader uses 128K bank switching (OUT to $7FFD)")
    print("  - Multiple memory banks accessed")
    print("  - Need to trace which banks contain what data")

if __name__ == '__main__':
    main()
