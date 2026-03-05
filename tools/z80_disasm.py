#!/usr/bin/env python3
"""
Proper Z80 disassembler for CUBELON pattern routine
"""

class Z80Disassembler:
    def __init__(self, data):
        self.data = data
        self.pc = 0

    def read_byte(self):
        if self.pc < len(self.data):
            b = self.data[self.pc]
            self.pc += 1
            return b
        return None

    def read_word(self):
        lo = self.read_byte()
        hi = self.read_byte()
        if lo is not None and hi is not None:
            return (hi << 8) | lo
        return None

    def disassemble_instruction(self, addr):
        """Disassemble one Z80 instruction at given address"""
        self.pc = addr
        opcode = self.read_byte()

        if opcode is None:
            return None

        # 0x00-0x0F
        if opcode == 0x00:
            return "NOP"
        elif opcode == 0x01:
            nn = self.read_word()
            return f"LD BC,${nn:04X}"
        elif opcode == 0x02:
            return "LD (BC),A"
        elif opcode == 0x03:
            return "INC BC"
        elif opcode == 0x04:
            return "INC B"
        elif opcode == 0x05:
            return "DEC B"
        elif opcode == 0x06:
            n = self.read_byte()
            return f"LD B,${n:02X}"
        elif opcode == 0x07:
            return "RLCA"
        elif opcode == 0x08:
            return "EX AF,AF'"
        elif opcode == 0x09:
            return "ADD HL,BC"
        elif opcode == 0x0A:
            return "LD A,(BC)"
        elif opcode == 0x0B:
            return "DEC BC"
        elif opcode == 0x0C:
            return "INC C"
        elif opcode == 0x0D:
            return "DEC C"
        elif opcode == 0x0E:
            n = self.read_byte()
            return f"LD C,${n:02X}"
        elif opcode == 0x0F:
            return "RRCA"

        # 0x10-0x1F
        elif opcode == 0x10:
            e = self.read_byte()
            offset = e if e < 128 else e - 256
            target = self.pc + offset
            return f"DJNZ ${target:04X}"
        elif opcode == 0x11:
            nn = self.read_word()
            return f"LD DE,${nn:04X}"
        elif opcode == 0x12:
            return "LD (DE),A"
        elif opcode == 0x13:
            return "INC DE"
        elif opcode == 0x14:
            return "INC D"
        elif opcode == 0x15:
            return "DEC D"
        elif opcode == 0x16:
            n = self.read_byte()
            return f"LD D,${n:02X}"
        elif opcode == 0x17:
            return "RLA"
        elif opcode == 0x18:
            e = self.read_byte()
            offset = e if e < 128 else e - 256
            target = self.pc + offset
            return f"JR ${target:04X}"
        elif opcode == 0x19:
            return "ADD HL,DE"
        elif opcode == 0x1A:
            return "LD A,(DE)"
        elif opcode == 0x1B:
            return "DEC DE"
        elif opcode == 0x1C:
            return "INC E"
        elif opcode == 0x1D:
            return "DEC E"
        elif opcode == 0x1E:
            n = self.read_byte()
            return f"LD E,${n:02X}"
        elif opcode == 0x1F:
            return "RRA"

        # 0x20-0x2F
        elif opcode == 0x20:
            e = self.read_byte()
            offset = e if e < 128 else e - 256
            target = self.pc + offset
            return f"JR NZ,${target:04X}"
        elif opcode == 0x21:
            nn = self.read_word()
            return f"LD HL,${nn:04X}"
        elif opcode == 0x22:
            nn = self.read_word()
            return f"LD (${nn:04X}),HL"
        elif opcode == 0x23:
            return "INC HL"
        elif opcode == 0x24:
            return "INC H"
        elif opcode == 0x25:
            return "DEC H"
        elif opcode == 0x26:
            n = self.read_byte()
            return f"LD H,${n:02X}"
        elif opcode == 0x27:
            return "DAA"
        elif opcode == 0x28:
            e = self.read_byte()
            offset = e if e < 128 else e - 256
            target = self.pc + offset
            return f"JR Z,${target:04X}"
        elif opcode == 0x29:
            return "ADD HL,HL"
        elif opcode == 0x2A:
            nn = self.read_word()
            return f"LD HL,(${nn:04X})"
        elif opcode == 0x2B:
            return "DEC HL"
        elif opcode == 0x2C:
            return "INC L"
        elif opcode == 0x2D:
            return "DEC L"
        elif opcode == 0x2E:
            n = self.read_byte()
            return f"LD L,${n:02X}"
        elif opcode == 0x2F:
            return "CPL"

        # 0x30-0x3F
        elif opcode == 0x30:
            e = self.read_byte()
            offset = e if e < 128 else e - 256
            target = self.pc + offset
            return f"JR NC,${target:04X}"
        elif opcode == 0x31:
            nn = self.read_word()
            return f"LD SP,${nn:04X}"
        elif opcode == 0x32:
            nn = self.read_word()
            return f"LD (${nn:04X}),A"
        elif opcode == 0x33:
            return "INC SP"
        elif opcode == 0x34:
            return "INC (HL)"
        elif opcode == 0x35:
            return "DEC (HL)"
        elif opcode == 0x36:
            n = self.read_byte()
            return f"LD (HL),${n:02X}"
        elif opcode == 0x37:
            return "SCF"
        elif opcode == 0x38:
            e = self.read_byte()
            offset = e if e < 128 else e - 256
            target = self.pc + offset
            return f"JR C,${target:04X}"
        elif opcode == 0x39:
            return "ADD HL,SP"
        elif opcode == 0x3A:
            nn = self.read_word()
            return f"LD A,(${nn:04X})"
        elif opcode == 0x3B:
            return "DEC SP"
        elif opcode == 0x3C:
            return "INC A"
        elif opcode == 0x3D:
            return "DEC A"
        elif opcode == 0x3E:
            n = self.read_byte()
            return f"LD A,${n:02X}"
        elif opcode == 0x3F:
            return "CCF"

        # 0x40-0x7F: LD r,r' instructions (8x8 matrix)
        elif 0x40 <= opcode <= 0x7F:
            if opcode == 0x76:
                return "HALT"
            src = opcode & 0x07
            dst = (opcode >> 3) & 0x07
            regs = ['B', 'C', 'D', 'E', 'H', 'L', '(HL)', 'A']
            return f"LD {regs[dst]},{regs[src]}"

        # 0x80-0xBF: ALU operations
        elif 0x80 <= opcode <= 0xBF:
            reg = opcode & 0x07
            op = (opcode >> 3) & 0x07
            regs = ['B', 'C', 'D', 'E', 'H', 'L', '(HL)', 'A']
            ops = ['ADD A,', 'ADC A,', 'SUB ', 'SBC A,', 'AND ', 'XOR ', 'OR ', 'CP ']
            return f"{ops[op]}{regs[reg]}"

        # 0xC0-0xFF: Control flow, stack, I/O
        elif opcode == 0xC0:
            return "RET NZ"
        elif opcode == 0xC1:
            return "POP BC"
        elif opcode == 0xC2:
            nn = self.read_word()
            return f"JP NZ,${nn:04X}"
        elif opcode == 0xC3:
            nn = self.read_word()
            return f"JP ${nn:04X}"
        elif opcode == 0xC4:
            nn = self.read_word()
            return f"CALL NZ,${nn:04X}"
        elif opcode == 0xC5:
            return "PUSH BC"
        elif opcode == 0xC6:
            n = self.read_byte()
            return f"ADD A,${n:02X}"
        elif opcode == 0xC7:
            return "RST $00"
        elif opcode == 0xC8:
            return "RET Z"
        elif opcode == 0xC9:
            return "RET"
        elif opcode == 0xCA:
            nn = self.read_word()
            return f"JP Z,${nn:04X}"
        elif opcode == 0xCB:
            # CB prefix - bit operations
            cb = self.read_byte()
            if cb is None:
                return "CB ??"
            bit_op = (cb >> 6) & 0x3
            bit_num = (cb >> 3) & 0x7
            reg = cb & 0x7
            regs = ['B', 'C', 'D', 'E', 'H', 'L', '(HL)', 'A']
            if bit_op == 1:  # BIT
                return f"BIT {bit_num},{regs[reg]}"
            elif bit_op == 2:  # RES
                return f"RES {bit_num},{regs[reg]}"
            elif bit_op == 3:  # SET
                return f"SET {bit_num},{regs[reg]}"
            else:  # Rotates/shifts
                ops = ['RLC', 'RRC', 'RL', 'RR', 'SLA', 'SRA', 'SLL', 'SRL']
                return f"{ops[bit_num]} {regs[reg]}"
        elif opcode == 0xCC:
            nn = self.read_word()
            return f"CALL Z,${nn:04X}"
        elif opcode == 0xCD:
            nn = self.read_word()
            return f"CALL ${nn:04X}"
        elif opcode == 0xCE:
            n = self.read_byte()
            return f"ADC A,${n:02X}"
        elif opcode == 0xCF:
            return "RST $08"

        elif opcode == 0xD0:
            return "RET NC"
        elif opcode == 0xD1:
            return "POP DE"
        elif opcode == 0xD2:
            nn = self.read_word()
            return f"JP NC,${nn:04X}"
        elif opcode == 0xD3:
            n = self.read_byte()
            return f"OUT (${n:02X}),A"
        elif opcode == 0xD4:
            nn = self.read_word()
            return f"CALL NC,${nn:04X}"
        elif opcode == 0xD5:
            return "PUSH DE"
        elif opcode == 0xD6:
            n = self.read_byte()
            return f"SUB ${n:02X}"
        elif opcode == 0xD7:
            return "RST $10"
        elif opcode == 0xD8:
            return "RET C"
        elif opcode == 0xD9:
            return "EXX"
        elif opcode == 0xDA:
            nn = self.read_word()
            return f"JP C,${nn:04X}"
        elif opcode == 0xDB:
            n = self.read_byte()
            return f"IN A,(${n:02X})"
        elif opcode == 0xDC:
            nn = self.read_word()
            return f"CALL C,${nn:04X}"
        elif opcode == 0xDE:
            n = self.read_byte()
            return f"SBC A,${n:02X}"
        elif opcode == 0xDF:
            return "RST $18"

        elif opcode == 0xE0:
            return "RET PO"
        elif opcode == 0xE1:
            return "POP HL"
        elif opcode == 0xE2:
            nn = self.read_word()
            return f"JP PO,${nn:04X}"
        elif opcode == 0xE3:
            return "EX (SP),HL"
        elif opcode == 0xE4:
            nn = self.read_word()
            return f"CALL PO,${nn:04X}"
        elif opcode == 0xE5:
            return "PUSH HL"
        elif opcode == 0xE6:
            n = self.read_byte()
            return f"AND ${n:02X}"
        elif opcode == 0xE7:
            return "RST $20"
        elif opcode == 0xE8:
            return "RET PE"
        elif opcode == 0xE9:
            return "JP (HL)"
        elif opcode == 0xEA:
            nn = self.read_word()
            return f"JP PE,${nn:04X}"
        elif opcode == 0xEB:
            return "EX DE,HL"
        elif opcode == 0xEC:
            nn = self.read_word()
            return f"CALL PE,${nn:04X}"
        elif opcode == 0xED:
            # ED prefix - extended instructions
            ed = self.read_byte()
            if ed is None:
                return "ED ??"
            # Block instructions and misc
            if ed == 0x42:
                return "SBC HL,BC"
            elif ed == 0x43:
                nn = self.read_word()
                return f"LD (${nn:04X}),BC"
            elif ed == 0x44:
                return "NEG"
            elif ed == 0x45:
                return "RETN"
            elif ed == 0x46:
                return "IM 0"
            elif ed == 0x47:
                return "LD I,A"
            elif ed == 0x4A:
                return "ADC HL,BC"
            elif ed == 0x4B:
                nn = self.read_word()
                return f"LD BC,(${nn:04X})"
            elif ed == 0x4D:
                return "RETI"
            elif ed == 0x4F:
                return "LD R,A"
            elif ed == 0x52:
                return "SBC HL,DE"
            elif ed == 0x53:
                nn = self.read_word()
                return f"LD (${nn:04X}),DE"
            elif ed == 0x56:
                return "IM 1"
            elif ed == 0x57:
                return "LD A,I"
            elif ed == 0x5A:
                return "ADC HL,DE"
            elif ed == 0x5B:
                nn = self.read_word()
                return f"LD DE,(${nn:04X})"
            elif ed == 0x5E:
                return "IM 2"
            elif ed == 0x5F:
                return "LD A,R"
            elif ed == 0x62:
                return "SBC HL,HL"
            elif ed == 0x67:
                return "RRD"
            elif ed == 0x6A:
                return "ADC HL,HL"
            elif ed == 0x6F:
                return "RLD"
            elif ed == 0x72:
                return "SBC HL,SP"
            elif ed == 0x73:
                nn = self.read_word()
                return f"LD (${nn:04X}),SP"
            elif ed == 0x7A:
                return "ADC HL,SP"
            elif ed == 0x7B:
                nn = self.read_word()
                return f"LD SP,(${nn:04X})"
            elif ed == 0xA0:
                return "LDI"
            elif ed == 0xA1:
                return "CPI"
            elif ed == 0xA2:
                return "INI"
            elif ed == 0xA3:
                return "OUTI"
            elif ed == 0xA8:
                return "LDD"
            elif ed == 0xA9:
                return "CPD"
            elif ed == 0xAA:
                return "IND"
            elif ed == 0xAB:
                return "OUTD"
            elif ed == 0xB0:
                return "LDIR"
            elif ed == 0xB1:
                return "CPIR"
            elif ed == 0xB2:
                return "INIR"
            elif ed == 0xB3:
                return "OTIR"
            elif ed == 0xB8:
                return "LDDR"
            elif ed == 0xB9:
                return "CPDR"
            elif ed == 0xBA:
                return "INDR"
            elif ed == 0xBB:
                return "OTDR"
            else:
                return f"ED {ed:02X}  ; Unknown"
        elif opcode == 0xEE:
            n = self.read_byte()
            return f"XOR ${n:02X}"
        elif opcode == 0xEF:
            return "RST $28"

        elif opcode == 0xF0:
            return "RET P"
        elif opcode == 0xF1:
            return "POP AF"
        elif opcode == 0xF2:
            nn = self.read_word()
            return f"JP P,${nn:04X}"
        elif opcode == 0xF3:
            return "DI"
        elif opcode == 0xF4:
            nn = self.read_word()
            return f"CALL P,${nn:04X}"
        elif opcode == 0xF5:
            return "PUSH AF"
        elif opcode == 0xF6:
            n = self.read_byte()
            return f"OR ${n:02X}"
        elif opcode == 0xF7:
            return "RST $30"
        elif opcode == 0xF8:
            return "RET M"
        elif opcode == 0xF9:
            return "LD SP,HL"
        elif opcode == 0xFA:
            nn = self.read_word()
            return f"JP M,${nn:04X}"
        elif opcode == 0xFB:
            return "EI"
        elif opcode == 0xFC:
            nn = self.read_word()
            return f"CALL M,${nn:04X}"
        elif opcode == 0xFE:
            n = self.read_byte()
            return f"CP ${n:02X}"
        elif opcode == 0xFF:
            return "RST $38"

        else:
            return f"DB ${opcode:02X}  ; Unknown"

def analyze_pattern_data():
    """Analyze all data files to find pattern generation code"""

    print("="*80)
    print("CUBELON Z80 CODE ANALYSIS")
    print("="*80)

    # Load all code/data files
    files_to_check = [
        ('original/cubelon_data_10.bin', 'Main code'),
        ('original/cubelon_data_11.bin', 'Additional code'),
        ('original/cubelon_data_12.bin', 'More code'),
    ]

    for filename, desc in files_to_check:
        try:
            with open(filename, 'rb') as f:
                data = f.read()
            print(f"\n{'='*80}")
            print(f"{desc}: {filename} ({len(data)} bytes)")
            print('='*80)

            # Look for interesting patterns
            # XOR operations (0xA8-0xAE, 0xEE)
            xor_positions = []
            for i, byte in enumerate(data):
                if byte in [0xA8, 0xA9, 0xAA, 0xAB, 0xAC, 0xAD, 0xAE, 0xEE]:
                    xor_positions.append(i)

            if xor_positions:
                print(f"\nFound {len(xor_positions)} XOR instructions")
                print("Locations:", ', '.join(f"${p:04X}" for p in xor_positions[:20]))

            # Look for screen memory writes (LD (HL),A = 0x77)
            screen_writes = [i for i, b in enumerate(data) if b == 0x77]
            if screen_writes:
                print(f"\nFound {len(screen_writes)} screen write instructions (LD (HL),A)")

            # Look for loops (JR instructions)
            jr_positions = [i for i, b in enumerate(data) if b in [0x18, 0x20, 0x28, 0x30, 0x38]]
            if jr_positions:
                print(f"\nFound {len(jr_positions)} jump/loop instructions")

            # Disassemble around interesting areas
            if xor_positions:
                print("\n" + "-"*80)
                print("Disassembling around first XOR instruction:")
                print("-"*80)
                start = max(0, xor_positions[0] - 20)
                disasm = Z80Disassembler(data)
                for addr in range(start, min(start + 100, len(data))):
                    if addr < len(data):
                        saved_pc = disasm.pc
                        instr = disasm.disassemble_instruction(addr)
                        instr_len = disasm.pc - addr

                        # Show hex bytes
                        hex_bytes = ' '.join(f'{data[addr+i]:02X}' for i in range(min(instr_len, 4)))
                        hex_bytes = hex_bytes.ljust(12)

                        mark = ">>>" if addr in xor_positions else "   "
                        print(f"{mark} ${addr:04X}  {hex_bytes} {instr}")

                        # Skip bytes we just disassembled
                        if instr_len > 1:
                            continue

        except FileNotFoundError:
            print(f"Not found: {filename}")

if __name__ == '__main__':
    analyze_pattern_data()
