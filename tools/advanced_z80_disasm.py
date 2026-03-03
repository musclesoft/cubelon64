#!/usr/bin/env python3
"""
Advanced Z80 disassembler with full instruction set support
Focus on pattern generation: rotates, shifts, XOR, OR operations
"""

import sys

class AdvancedZ80Disassembler:
    def __init__(self, code, start_addr=0):
        self.code = code
        self.start_addr = start_addr
        self.pc = 0

    def get_byte(self):
        if self.pc < len(self.code):
            val = self.code[self.pc]
            self.pc += 1
            return val
        return 0

    def get_word(self):
        lo = self.get_byte()
        hi = self.get_byte()
        return lo | (hi << 8)

    def get_signed_byte(self):
        val = self.get_byte()
        return val if val < 128 else val - 256

    def disassemble_cb(self):
        """CB-prefixed instructions: bit operations, rotates, shifts"""
        op = self.get_byte()

        bit_ops = {
            0: ("RLC", "B"), 1: ("RLC", "C"), 2: ("RLC", "D"), 3: ("RLC", "E"),
            4: ("RLC", "H"), 5: ("RLC", "L"), 6: ("RLC", "(HL)"), 7: ("RLC", "A"),
            8: ("RRC", "B"), 9: ("RRC", "C"), 10: ("RRC", "D"), 11: ("RRC", "E"),
            12: ("RRC", "H"), 13: ("RRC", "L"), 14: ("RRC", "(HL)"), 15: ("RRC", "A"),
            16: ("RL", "B"), 17: ("RL", "C"), 18: ("RL", "D"), 19: ("RL", "E"),
            20: ("RL", "H"), 21: ("RL", "L"), 22: ("RL", "(HL)"), 23: ("RL", "A"),
            24: ("RR", "B"), 25: ("RR", "C"), 26: ("RR", "D"), 27: ("RR", "E"),
            28: ("RR", "H"), 29: ("RR", "L"), 30: ("RR", "(HL)"), 31: ("RR", "A"),
            32: ("SLA", "B"), 33: ("SLA", "C"), 34: ("SLA", "D"), 35: ("SLA", "E"),
            36: ("SLA", "H"), 37: ("SLA", "L"), 38: ("SLA", "(HL)"), 39: ("SLA", "A"),
            40: ("SRA", "B"), 41: ("SRA", "C"), 42: ("SRA", "D"), 43: ("SRA", "E"),
            44: ("SRA", "H"), 45: ("SRA", "L"), 46: ("SRA", "(HL)"), 47: ("SRA", "A"),
            48: ("SLL", "B"), 49: ("SLL", "C"), 50: ("SLL", "D"), 51: ("SLL", "E"),
            52: ("SLL", "H"), 53: ("SLL", "L"), 54: ("SLL", "(HL)"), 55: ("SLL", "A"),
            56: ("SRL", "B"), 57: ("SRL", "C"), 58: ("SRL", "D"), 59: ("SRL", "E"),
            60: ("SRL", "H"), 61: ("SRL", "L"), 62: ("SRL", "(HL)"), 63: ("SRL", "A"),
        }

        if op < 64:
            mnem, reg = bit_ops[op]
            return f"{mnem} {reg}", 2
        elif op < 128:
            bit = (op - 64) // 8
            reg = ["B", "C", "D", "E", "H", "L", "(HL)", "A"][op % 8]
            return f"BIT {bit},{reg}", 2
        elif op < 192:
            bit = (op - 128) // 8
            reg = ["B", "C", "D", "E", "H", "L", "(HL)", "A"][op % 8]
            return f"RES {bit},{reg}", 2
        else:
            bit = (op - 192) // 8
            reg = ["B", "C", "D", "E", "H", "L", "(HL)", "A"][op % 8]
            return f"SET {bit},{reg}", 2

    def disassemble_ed(self):
        """ED-prefixed instructions"""
        op = self.get_byte()

        ed_ops = {
            0x40: ("IN B,(C)", 2), 0x41: ("OUT (C),B", 2),
            0x42: ("SBC HL,BC", 2), 0x43: (None, 4),  # LD (nn),BC
            0x44: ("NEG", 2), 0x45: ("RETN", 2),
            0x46: ("IM 0", 2), 0x47: ("LD I,A", 2),
            0x48: ("IN C,(C)", 2), 0x49: ("OUT (C),C", 2),
            0x4A: ("ADC HL,BC", 2), 0x4B: (None, 4),  # LD BC,(nn)
            0x4D: ("RETI", 2), 0x4F: ("LD R,A", 2),
            0x50: ("IN D,(C)", 2), 0x51: ("OUT (C),D", 2),
            0x52: ("SBC HL,DE", 2), 0x53: (None, 4),  # LD (nn),DE
            0x56: ("IM 1", 2), 0x57: ("LD A,I", 2),
            0x58: ("IN E,(C)", 2), 0x59: ("OUT (C),E", 2),
            0x5A: ("ADC HL,DE", 2), 0x5B: (None, 4),  # LD DE,(nn)
            0x5E: ("IM 2", 2), 0x5F: ("LD A,R", 2),
            0x60: ("IN H,(C)", 2), 0x61: ("OUT (C),H", 2),
            0x62: ("SBC HL,HL", 2), 0x67: ("RRD", 2),
            0x68: ("IN L,(C)", 2), 0x69: ("OUT (C),L", 2),
            0x6A: ("ADC HL,HL", 2), 0x6F: ("RLD", 2),
            0x72: ("SBC HL,SP", 2), 0x73: (None, 4),  # LD (nn),SP
            0x78: ("IN A,(C)", 2), 0x79: ("OUT (C),A", 2),
            0x7A: ("ADC HL,SP", 2), 0x7B: (None, 4),  # LD SP,(nn)
            0xA0: ("LDI", 2), 0xA1: ("CPI", 2), 0xA2: ("INI", 2), 0xA3: ("OUTI", 2),
            0xA8: ("LDD", 2), 0xA9: ("CPD", 2), 0xAA: ("IND", 2), 0xAB: ("OUTD", 2),
            0xB0: ("LDIR", 2), 0xB1: ("CPIR", 2), 0xB2: ("INIR", 2), 0xB3: ("OTIR", 2),
            0xB8: ("LDDR", 2), 0xB9: ("CPDR", 2), 0xBA: ("INDR", 2), 0xBB: ("OTDR", 2),
        }

        if op in ed_ops:
            result = ed_ops[op]
            if result[0] is None:  # LD with 16-bit address
                nn = self.get_word()
                if op == 0x43: return f"LD (${nn:04X}),BC", 4
                if op == 0x4B: return f"LD BC,(${nn:04X})", 4
                if op == 0x53: return f"LD (${nn:04X}),DE", 4
                if op == 0x5B: return f"LD DE,(${nn:04X})", 4
                if op == 0x73: return f"LD (${nn:04X}),SP", 4
                if op == 0x7B: return f"LD SP,(${nn:04X})", 4
            return result

        return f"DB $ED,${op:02X}", 2

    def disassemble_one(self):
        """Disassemble one instruction"""
        addr = self.start_addr + self.pc
        start_pc = self.pc

        op = self.get_byte()

        # CB prefix - bit operations
        if op == 0xCB:
            mnem, size = self.disassemble_cb()
            return addr, mnem, size

        # ED prefix - extended instructions
        if op == 0xED:
            mnem, size = self.disassemble_ed()
            return addr, mnem, size

        # DD/FD prefixes - IX/IY (simplified)
        if op == 0xDD or op == 0xFD:
            prefix = "IX" if op == 0xDD else "IY"
            op2 = self.get_byte()
            # Just handle common ones
            if op2 == 0x21:
                nn = self.get_word()
                return addr, f"LD {prefix},${nn:04X}", 4
            if op2 == 0xE1:
                return addr, f"POP {prefix}", 2
            if op2 == 0xE5:
                return addr, f"PUSH {prefix}", 2
            return addr, f"DB ${op:02X},${op2:02X}", 2

        # Standard opcodes
        opcodes = {
            0x00: ("NOP", 1),
            0x01: (None, 3), 0x02: ("LD (BC),A", 1), 0x03: ("INC BC", 1),
            0x04: ("INC B", 1), 0x05: ("DEC B", 1), 0x06: (None, 2),
            0x07: ("RLCA", 1), 0x08: ("EX AF,AF'", 1), 0x09: ("ADD HL,BC", 1),
            0x0A: ("LD A,(BC)", 1), 0x0B: ("DEC BC", 1), 0x0C: ("INC C", 1),
            0x0D: ("DEC C", 1), 0x0E: (None, 2), 0x0F: ("RRCA", 1),
            0x10: (None, 2), 0x11: (None, 3), 0x12: ("LD (DE),A", 1),
            0x13: ("INC DE", 1), 0x14: ("INC D", 1), 0x15: ("DEC D", 1),
            0x16: (None, 2), 0x17: ("RLA", 1), 0x18: (None, 2),
            0x19: ("ADD HL,DE", 1), 0x1A: ("LD A,(DE)", 1), 0x1B: ("DEC DE", 1),
            0x1C: ("INC E", 1), 0x1D: ("DEC E", 1), 0x1E: (None, 2),
            0x1F: ("RRA", 1), 0x20: (None, 2), 0x21: (None, 3),
            0x22: (None, 3), 0x23: ("INC HL", 1), 0x24: ("INC H", 1),
            0x25: ("DEC H", 1), 0x26: (None, 2), 0x27: ("DAA", 1),
            0x28: (None, 2), 0x29: ("ADD HL,HL", 1), 0x2A: (None, 3),
            0x2B: ("DEC HL", 1), 0x2C: ("INC L", 1), 0x2D: ("DEC L", 1),
            0x2E: (None, 2), 0x2F: ("CPL", 1), 0x30: (None, 2),
            0x31: (None, 3), 0x32: (None, 3), 0x33: ("INC SP", 1),
            0x34: ("INC (HL)", 1), 0x35: ("DEC (HL)", 1), 0x36: (None, 2),
            0x37: ("SCF", 1), 0x38: (None, 2), 0x39: ("ADD HL,SP", 1),
            0x3A: (None, 3), 0x3B: ("DEC SP", 1), 0x3C: ("INC A", 1),
            0x3D: ("DEC A", 1), 0x3E: (None, 2), 0x3F: ("CCF", 1),
        }

        # 0x40-0x7F: LD r,r' and HALT
        if 0x40 <= op < 0x76:
            regs = ["B", "C", "D", "E", "H", "L", "(HL)", "A"]
            dst = regs[(op - 0x40) // 8]
            src = regs[op % 8]
            return addr, f"LD {dst},{src}", 1
        if op == 0x76:
            return addr, "HALT", 1
        if 0x77 <= op < 0x80:
            regs = ["B", "C", "D", "E", "H", "L", "(HL)", "A"]
            src = regs[op % 8]
            return addr, f"LD (HL),{src}" if op == 0x77 else f"LD {src},(HL)", 1

        # 0x80-0xBF: ALU operations
        if 0x80 <= op < 0xC0:
            alu_ops = ["ADD A,", "ADC A,", "SUB ", "SBC A,", "AND ", "XOR ", "OR ", "CP "]
            regs = ["B", "C", "D", "E", "H", "L", "(HL)", "A"]
            alu = alu_ops[(op - 0x80) // 8]
            reg = regs[op % 8]
            return addr, f"{alu}{reg}", 1

        # 0xC0+: Various
        if op in opcodes:
            result = opcodes[op]
            if result[0] is not None:
                return addr, result[0], result[1]

        # Handle instructions with immediate values
        if op == 0x01:
            nn = self.get_word()
            return addr, f"LD BC,${nn:04X}", 3
        if op == 0x06:
            n = self.get_byte()
            return addr, f"LD B,${n:02X}", 2
        if op == 0x0E:
            n = self.get_byte()
            return addr, f"LD C,${n:02X}", 2
        if op == 0x10:  # DJNZ
            e = self.get_signed_byte()
            target = addr + 2 + e
            return addr, f"DJNZ ${target:04X}", 2
        if op == 0x11:
            nn = self.get_word()
            return addr, f"LD DE,${nn:04X}", 3
        if op == 0x16:
            n = self.get_byte()
            return addr, f"LD D,${n:02X}", 2
        if op == 0x18:  # JR
            e = self.get_signed_byte()
            target = addr + 2 + e
            return addr, f"JR ${target:04X}", 2
        if op == 0x1E:
            n = self.get_byte()
            return addr, f"LD E,${n:02X}", 2
        if op == 0x20:  # JR NZ
            e = self.get_signed_byte()
            target = addr + 2 + e
            return addr, f"JR NZ,${target:04X}", 2
        if op == 0x21:
            nn = self.get_word()
            return addr, f"LD HL,${nn:04X}", 3
        if op == 0x22:
            nn = self.get_word()
            return addr, f"LD (${nn:04X}),HL", 3
        if op == 0x26:
            n = self.get_byte()
            return addr, f"LD H,${n:02X}", 2
        if op == 0x28:  # JR Z
            e = self.get_signed_byte()
            target = addr + 2 + e
            return addr, f"JR Z,${target:04X}", 2
        if op == 0x2A:
            nn = self.get_word()
            return addr, f"LD HL,(${nn:04X})", 3
        if op == 0x2E:
            n = self.get_byte()
            return addr, f"LD L,${n:02X}", 2
        if op == 0x30:  # JR NC
            e = self.get_signed_byte()
            target = addr + 2 + e
            return addr, f"JR NC,${target:04X}", 2
        if op == 0x31:
            nn = self.get_word()
            return addr, f"LD SP,${nn:04X}", 3
        if op == 0x32:
            nn = self.get_word()
            return addr, f"LD (${nn:04X}),A", 3
        if op == 0x36:
            n = self.get_byte()
            return addr, f"LD (HL),${n:02X}", 2
        if op == 0x38:  # JR C
            e = self.get_signed_byte()
            target = addr + 2 + e
            return addr, f"JR C,${target:04X}", 2
        if op == 0x3A:
            nn = self.get_word()
            return addr, f"LD A,(${nn:04X})", 3
        if op == 0x3E:
            n = self.get_byte()
            return addr, f"LD A,${n:02X}", 2

        # Control flow
        if op == 0xC0: return addr, "RET NZ", 1
        if op == 0xC1: return addr, "POP BC", 1
        if op == 0xC2:
            nn = self.get_word()
            return addr, f"JP NZ,${nn:04X}", 3
        if op == 0xC3:
            nn = self.get_word()
            return addr, f"JP ${nn:04X}", 3
        if op == 0xC4:
            nn = self.get_word()
            return addr, f"CALL NZ,${nn:04X}", 3
        if op == 0xC5: return addr, "PUSH BC", 1
        if op == 0xC6:
            n = self.get_byte()
            return addr, f"ADD A,${n:02X}", 2
        if op == 0xC7: return addr, "RST $00", 1
        if op == 0xC8: return addr, "RET Z", 1
        if op == 0xC9: return addr, "RET", 1
        if op == 0xCA:
            nn = self.get_word()
            return addr, f"JP Z,${nn:04X}", 3
        if op == 0xCC:
            nn = self.get_word()
            return addr, f"CALL Z,${nn:04X}", 3
        if op == 0xCD:
            nn = self.get_word()
            return addr, f"CALL ${nn:04X}", 3
        if op == 0xCE:
            n = self.get_byte()
            return addr, f"ADC A,${n:02X}", 2
        if op == 0xCF: return addr, "RST $08", 1
        if op == 0xD0: return addr, "RET NC", 1
        if op == 0xD1: return addr, "POP DE", 1
        if op == 0xD2:
            nn = self.get_word()
            return addr, f"JP NC,${nn:04X}", 3
        if op == 0xD3:
            n = self.get_byte()
            return addr, f"OUT (${n:02X}),A", 2
        if op == 0xD4:
            nn = self.get_word()
            return addr, f"CALL NC,${nn:04X}", 3
        if op == 0xD5: return addr, "PUSH DE", 1
        if op == 0xD6:
            n = self.get_byte()
            return addr, f"SUB ${n:02X}", 2
        if op == 0xD7: return addr, "RST $10", 1
        if op == 0xD8: return addr, "RET C", 1
        if op == 0xD9: return addr, "EXX", 1
        if op == 0xDA:
            nn = self.get_word()
            return addr, f"JP C,${nn:04X}", 3
        if op == 0xDB:
            n = self.get_byte()
            return addr, f"IN A,(${n:02X})", 2
        if op == 0xDC:
            nn = self.get_word()
            return addr, f"CALL C,${nn:04X}", 3
        if op == 0xDE:
            n = self.get_byte()
            return addr, f"SBC A,${n:02X}", 2
        if op == 0xDF: return addr, "RST $18", 1
        if op == 0xE0: return addr, "RET PO", 1
        if op == 0xE1: return addr, "POP HL", 1
        if op == 0xE2:
            nn = self.get_word()
            return addr, f"JP PO,${nn:04X}", 3
        if op == 0xE3: return addr, "EX (SP),HL", 1
        if op == 0xE4:
            nn = self.get_word()
            return addr, f"CALL PO,${nn:04X}", 3
        if op == 0xE5: return addr, "PUSH HL", 1
        if op == 0xE6:
            n = self.get_byte()
            return addr, f"AND ${n:02X}", 2
        if op == 0xE7: return addr, "RST $20", 1
        if op == 0xE8: return addr, "RET PE", 1
        if op == 0xE9: return addr, "JP (HL)", 1
        if op == 0xEA:
            nn = self.get_word()
            return addr, f"JP PE,${nn:04X}", 3
        if op == 0xEB: return addr, "EX DE,HL", 1
        if op == 0xEC:
            nn = self.get_word()
            return addr, f"CALL PE,${nn:04X}", 3
        if op == 0xEE:
            n = self.get_byte()
            return addr, f"XOR ${n:02X}", 2
        if op == 0xEF: return addr, "RST $28", 1
        if op == 0xF0: return addr, "RET P", 1
        if op == 0xF1: return addr, "POP AF", 1
        if op == 0xF2:
            nn = self.get_word()
            return addr, f"JP P,${nn:04X}", 3
        if op == 0xF3: return addr, "DI", 1
        if op == 0xF4:
            nn = self.get_word()
            return addr, f"CALL P,${nn:04X}", 3
        if op == 0xF5: return addr, "PUSH AF", 1
        if op == 0xF6:
            n = self.get_byte()
            return addr, f"OR ${n:02X}", 2
        if op == 0xF7: return addr, "RST $30", 1
        if op == 0xF8: return addr, "RET M", 1
        if op == 0xF9: return addr, "LD SP,HL", 1
        if op == 0xFA:
            nn = self.get_word()
            return addr, f"JP M,${nn:04X}", 3
        if op == 0xFB: return addr, "EI", 1
        if op == 0xFC:
            nn = self.get_word()
            return addr, f"CALL M,${nn:04X}", 3
        if op == 0xFE:
            n = self.get_byte()
            return addr, f"CP ${n:02X}", 2
        if op == 0xFF: return addr, "RST $38", 1

        # Unknown opcode
        self.pc = start_pc + 1
        return addr, f"DB ${op:02X}", 1

    def disassemble(self, max_bytes=512):
        """Disassemble code"""
        results = []
        while self.pc < min(len(self.code), max_bytes):
            addr, mnem, size = self.disassemble_one()
            results.append((addr, mnem))
        return results


def find_pattern_loops(code, start_addr=0):
    """Find loops that might generate patterns"""
    disasm = AdvancedZ80Disassembler(code, start_addr)
    instructions = disasm.disassemble(len(code))

    print("Searching for pattern generation loops...")
    print("=" * 70)

    # Find DJNZ instructions (loop markers)
    loops = []
    for i, (addr, mnem) in enumerate(instructions):
        if "DJNZ" in mnem:
            # Look at surrounding instructions
            start_idx = max(0, i - 20)
            end_idx = min(len(instructions), i + 5)

            window = instructions[start_idx:end_idx]

            # Count rotate/shift/XOR operations in window
            rotate_count = sum(1 for _, m in window if any(x in m for x in ["RL", "RR", "SLA", "SRA", "SRL"]))
            xor_count = sum(1 for _, m in window if "XOR" in m)
            or_count = sum(1 for _, m in window if "OR" in m)

            score = rotate_count + xor_count * 2 + or_count

            if score >= 2:
                loops.append((addr, i, score, rotate_count, xor_count, or_count))

    print(f"\nFound {len(loops)} interesting loops (with rotate/XOR/OR operations)")
    print()

    for addr, idx, score, rot, xor, or_ops in sorted(loops, key=lambda x: -x[2])[:10]:
        print(f"Loop at ${addr:04X} - Score: {score} (Rotates: {rot}, XOR: {xor}, OR: {or_ops})")

        # Show surrounding code
        start_idx = max(0, idx - 15)
        end_idx = min(len(instructions), idx + 5)

        for i in range(start_idx, end_idx):
            a, m = instructions[i]
            marker = " <-- LOOP" if i == idx else ""
            highlight = " ***" if any(x in m for x in ["XOR", "RL", "RR", "SLA", "SRA", "SRL", "OR"]) else ""
            print(f"  ${a:04X}: {m:30s}{highlight}{marker}")
        print()


def main():
    print("Advanced Z80 Pattern Analysis")
    print("=" * 70)

    # Analyze block 10
    with open('original/cubelon_data_10.bin', 'rb') as f:
        code = f.read()

    print(f"\nAnalyzing Block 10: {len(code)} bytes")
    print()

    find_pattern_loops(code, 0xC000)

    print("\n" + "=" * 70)
    print("Analysis complete. Pattern generation code should be in loops above.")


if __name__ == '__main__':
    main()
