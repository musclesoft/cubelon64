package z80

import "fmt"

// Common Z80 instructions for CUBELON

func (z *CPU) executeExtended(opcode byte) {
	switch opcode {
	case 0xED: // ED prefix
		op2 := z.fetch()
		z.executeED(op2)
	case 0xCB: // CB prefix (bit operations)
		op2 := z.fetch()
		z.executeCB(op2)
	case 0xDD: // DD prefix (IX)
		op2 := z.fetch()
		z.executeDD(op2)
	case 0xFD: // FD prefix (IY)
		op2 := z.fetch()
		z.executeFD(op2)
	default:
		z.executeMain(opcode)
	}
}

func (z *CPU) executeED(opcode byte) {
	switch opcode {
	case 0x47: // LD I, A
		z.I = z.A
		z.Cycles += 9
	case 0x4F: // LD R, A
		z.R = z.A
		z.Cycles += 9
	case 0x56, 0x76: // IM 1, IM 2
		z.IM = 1
		if opcode == 0x76 {
			z.IM = 2
		}
		z.Cycles += 8
	case 0x5E: // IM 2
		z.IM = 2
		z.Cycles += 8

	// IN r,(C) - port is full 16-bit BC
	case 0x40: z.B = z.IOIn16(z.BC()); z.Cycles += 12  // IN B,(C)
	case 0x48: z.C = z.IOIn16(z.BC()); z.Cycles += 12  // IN C,(C)
	case 0x50: z.D = z.IOIn16(z.BC()); z.Cycles += 12  // IN D,(C)
	case 0x58: z.E = z.IOIn16(z.BC()); z.Cycles += 12  // IN E,(C)
	case 0x60: z.H = z.IOIn16(z.BC()); z.Cycles += 12  // IN H,(C)
	case 0x68: z.L = z.IOIn16(z.BC()); z.Cycles += 12  // IN L,(C)
	case 0x78: z.A = z.IOIn16(z.BC()); z.Cycles += 12  // IN A,(C)

	// OUT (C), r - port is full 16-bit BC
	case 0x41: z.IOOut16(z.BC(), z.B); z.Cycles += 12  // OUT (C), B
	case 0x49: z.IOOut16(z.BC(), z.C); z.Cycles += 12  // OUT (C), C
	case 0x51: z.IOOut16(z.BC(), z.D); z.Cycles += 12  // OUT (C), D
	case 0x59: z.IOOut16(z.BC(), z.E); z.Cycles += 12  // OUT (C), E
	case 0x61: z.IOOut16(z.BC(), z.H); z.Cycles += 12  // OUT (C), H
	case 0x69: z.IOOut16(z.BC(), z.L); z.Cycles += 12  // OUT (C), L
	case 0x79: z.IOOut16(z.BC(), z.A); z.Cycles += 12  // OUT (C), A

	// 16-bit LD (nn),rr
	case 0x43: // LD (nn),BC
		addr := z.fetch16()
		z.Mem.Write(addr, z.C)
		z.Mem.Write(addr+1, z.B)
		z.Cycles += 20
	case 0x53: // LD (nn),DE
		addr := z.fetch16()
		z.Mem.Write(addr, z.E)
		z.Mem.Write(addr+1, z.D)
		z.Cycles += 20
	case 0x63: // LD (nn),HL
		addr := z.fetch16()
		z.Mem.Write(addr, z.L)
		z.Mem.Write(addr+1, z.H)
		z.Cycles += 20
	case 0x73: // LD (nn),SP
		addr := z.fetch16()
		z.Mem.Write(addr, byte(z.SP))
		z.Mem.Write(addr+1, byte(z.SP>>8))
		z.Cycles += 20

	// 16-bit LD rr,(nn)
	case 0x4B: // LD BC,(nn)
		addr := z.fetch16()
		z.C = z.Mem.Read(addr)
		z.B = z.Mem.Read(addr + 1)
		z.Cycles += 20
	case 0x5B: // LD DE,(nn)
		addr := z.fetch16()
		z.E = z.Mem.Read(addr)
		z.D = z.Mem.Read(addr + 1)
		z.Cycles += 20
	case 0x6B: // LD HL,(nn)
		addr := z.fetch16()
		z.L = z.Mem.Read(addr)
		z.H = z.Mem.Read(addr + 1)
		z.Cycles += 20
	case 0x7B: // LD SP,(nn)
		addr := z.fetch16()
		lo := z.Mem.Read(addr)
		hi := z.Mem.Read(addr + 1)
		z.SP = uint16(lo) | (uint16(hi) << 8)
		z.Cycles += 20

	// 16-bit ADC HL,rr
	case 0x4A: z.setHL(z.adc16(z.HL(), z.BC())); z.Cycles += 15  // ADC HL,BC
	case 0x5A: z.setHL(z.adc16(z.HL(), z.DE())); z.Cycles += 15  // ADC HL,DE
	case 0x6A: z.setHL(z.adc16(z.HL(), z.HL())); z.Cycles += 15  // ADC HL,HL
	case 0x7A: z.setHL(z.adc16(z.HL(), z.SP)); z.Cycles += 15    // ADC HL,SP

	// 16-bit SBC HL,rr
	case 0x42: z.setHL(z.sbc16(z.HL(), z.BC())); z.Cycles += 15  // SBC HL,BC
	case 0x52: z.setHL(z.sbc16(z.HL(), z.DE())); z.Cycles += 15  // SBC HL,DE
	case 0x62: z.setHL(z.sbc16(z.HL(), z.HL())); z.Cycles += 15  // SBC HL,HL
	case 0x72: z.setHL(z.sbc16(z.HL(), z.SP)); z.Cycles += 15    // SBC HL,SP

	// Block transfer instructions
	case 0xA0: // LDI - load and increment
		z.Mem.Write(z.DE(), z.Mem.Read(z.HL()))
		z.setHL(z.HL() + 1)
		z.setDE(z.DE() + 1)
		z.setBC(z.BC() - 1)
		z.F = (z.F & 0xC1) // Keep S, Z, C
		if z.BC() != 0 { z.F |= 0x04 } // P/V set if BC != 0
		z.Cycles += 16

	case 0xB0: // LDIR - load, increment, repeat
		for z.BC() > 0 {
			z.Mem.Write(z.DE(), z.Mem.Read(z.HL()))
			z.setHL(z.HL() + 1)
			z.setDE(z.DE() + 1)
			z.setBC(z.BC() - 1)
		}
		z.F = (z.F & 0xC1) // Keep S, Z, C; clear P/V (BC is 0)
		z.Cycles += 21

	case 0xA8: // LDD - load and decrement
		z.Mem.Write(z.DE(), z.Mem.Read(z.HL()))
		z.setHL(z.HL() - 1)
		z.setDE(z.DE() - 1)
		z.setBC(z.BC() - 1)
		z.F = (z.F & 0xC1) // Keep S, Z, C
		if z.BC() != 0 { z.F |= 0x04 } // P/V set if BC != 0
		z.Cycles += 16

	case 0xB8: // LDDR - load, decrement, repeat
		for z.BC() > 0 {
			z.Mem.Write(z.DE(), z.Mem.Read(z.HL()))
			z.setHL(z.HL() - 1)
			z.setDE(z.DE() - 1)
			z.setBC(z.BC() - 1)
		}
		z.F = (z.F & 0xC1) // Keep S, Z, C; clear P/V (BC is 0)
		z.Cycles += 21

	case 0xA2: // INI - input and increment
		val := z.IO.In(z.C)
		z.Mem.Write(z.HL(), val)
		z.setHL(z.HL() + 1)
		z.B--
		z.F = (z.F & 0xC1) // Keep S, Z, C
		if z.B == 0 { z.F |= 0x40 } // Set Z if B=0
		z.Cycles += 16

	case 0xA3: // OUTI - output and increment
		val := z.Mem.Read(z.HL())
		z.B-- // Decrement B BEFORE the OUT
		bc := z.BC()
		z.IOOut16(bc, val) // Use full 16-bit port address
		z.setHL(z.HL() + 1)
		z.F = (z.F & 0xC1) // Keep S, Z, C
		if z.B == 0 { z.F |= 0x40 } // Set Z if B=0
		z.Cycles += 16

	case 0xAA: // IND - input and decrement
		val := z.IO.In(z.C)
		z.Mem.Write(z.HL(), val)
		z.setHL(z.HL() - 1)
		z.B--
		z.F = (z.F & 0xC1) // Keep S, Z, C
		if z.B == 0 { z.F |= 0x40 } // Set Z if B=0
		z.Cycles += 16

	case 0xAB: // OUTD - output and decrement
		val := z.Mem.Read(z.HL())
		z.B-- // Decrement B BEFORE the OUT
		bc := z.BC()
		z.IOOut16(bc, val) // Use full 16-bit port address
		z.setHL(z.HL() - 1)
		z.F = (z.F & 0xC1) // Keep S, Z, C
		if z.B == 0 { z.F |= 0x40 } // Set Z if B=0
		z.Cycles += 16

	case 0xB2: // INIR - input and increment repeat
		for z.B > 0 {
			val := z.IO.In(z.C)
			z.Mem.Write(z.HL(), val)
			z.setHL(z.HL() + 1)
			z.B--
		}
		z.F = (z.F & 0xC1) | 0x40 // Keep S, Z, C; set Z (B is 0)
		z.Cycles += 21

	case 0xB3: // OTIR - output and increment repeat
		for z.B > 0 {
			val := z.Mem.Read(z.HL())
			z.B-- // Decrement B BEFORE the OUT
			bc := z.BC()
			z.IOOut16(bc, val) // Use full 16-bit port address
			z.setHL(z.HL() + 1)
		}
		z.F = (z.F & 0xC1) | 0x40 // Keep S, Z, C; set Z (B is 0)
		z.Cycles += 21

	case 0xBA: // INDR - input and decrement repeat
		for z.B > 0 {
			val := z.IO.In(z.C)
			z.Mem.Write(z.HL(), val)
			z.setHL(z.HL() - 1)
			z.B--
		}
		z.F = (z.F & 0xC1) | 0x40 // Keep S, Z, C; set Z (B is 0)
		z.Cycles += 21

	case 0xBB: // OTDR - output and decrement repeat
		for z.B > 0 {
			val := z.Mem.Read(z.HL())
			z.B-- // Decrement B BEFORE the OUT
			bc := z.BC()
			z.IOOut16(bc, val) // Use full 16-bit port address
			z.setHL(z.HL() - 1)
		}
		z.F = (z.F & 0xC1) | 0x40 // Keep S, Z, C; set Z (B is 0)
		z.Cycles += 21

	default:
		panic(fmt.Sprintf("Unimplemented ED instruction: ED %02X at PC=%04X", opcode, z.PC-2))
	}
}

func (z *CPU) executeCB(opcode byte) {
	// CB prefix: bit operations
	bit := (opcode >> 3) & 0x07
	reg := opcode & 0x07

	switch {
	case opcode >= 0x00 && opcode <= 0x3F: // Shift/rotate instructions
		var val byte
		var addr uint16

		// Get value from register or memory
		switch reg {
		case 0: val = z.B
		case 1: val = z.C
		case 2: val = z.D
		case 3: val = z.E
		case 4: val = z.H
		case 5: val = z.L
		case 6: addr = z.HL(); val = z.Mem.Read(addr); z.Cycles += 7
		case 7: val = z.A
		}

		// Perform operation based on opcode range
		var result byte
		switch {
		case opcode >= 0x00 && opcode <= 0x07: // RLC - rotate left circular
			result = (val << 1) | (val >> 7)
			z.F = 0
			if result == 0 { z.F |= 0x40 }
			if result&0x80 != 0 { z.F |= 0x80 }
			if val&0x80 != 0 { z.F |= 0x01 } // Carry = bit 7

		case opcode >= 0x08 && opcode <= 0x0F: // RRC - rotate right circular
			result = (val >> 1) | (val << 7)
			z.F = 0
			if result == 0 { z.F |= 0x40 }
			if result&0x80 != 0 { z.F |= 0x80 }
			if val&0x01 != 0 { z.F |= 0x01 } // Carry = bit 0

		case opcode >= 0x10 && opcode <= 0x17: // RL - rotate left through carry
			carry := z.F & 0x01
			result = (val << 1) | carry
			z.F = 0
			if result == 0 { z.F |= 0x40 }
			if result&0x80 != 0 { z.F |= 0x80 }
			if val&0x80 != 0 { z.F |= 0x01 }

		case opcode >= 0x18 && opcode <= 0x1F: // RR - rotate right through carry
			carry := (z.F & 0x01) << 7
			result = (val >> 1) | carry
			z.F = 0
			if result == 0 { z.F |= 0x40 }
			if result&0x80 != 0 { z.F |= 0x80 }
			if val&0x01 != 0 { z.F |= 0x01 }

		case opcode >= 0x20 && opcode <= 0x27: // SLA - shift left arithmetic
			result = val << 1
			z.F = 0
			if result == 0 { z.F |= 0x40 }
			if result&0x80 != 0 { z.F |= 0x80 }
			if val&0x80 != 0 { z.F |= 0x01 }

		case opcode >= 0x28 && opcode <= 0x2F: // SRA - shift right arithmetic
			result = (val >> 1) | (val & 0x80) // Keep sign bit
			z.F = 0
			if result == 0 { z.F |= 0x40 }
			if result&0x80 != 0 { z.F |= 0x80 }
			if val&0x01 != 0 { z.F |= 0x01 }

		case opcode >= 0x30 && opcode <= 0x37: // SLL - shift left logical (undocumented)
			result = (val << 1) | 0x01
			z.F = 0
			if result == 0 { z.F |= 0x40 }
			if result&0x80 != 0 { z.F |= 0x80 }
			if val&0x80 != 0 { z.F |= 0x01 }

		case opcode >= 0x38 && opcode <= 0x3F: // SRL - shift right logical
			result = val >> 1
			z.F = 0
			if result == 0 { z.F |= 0x40 }
			if result&0x80 != 0 { z.F |= 0x80 }
			if val&0x01 != 0 { z.F |= 0x01 }
		}

		// Parity flag
		parity := byte(0)
		for i := 0; i < 8; i++ {
			if result&(1<<i) != 0 { parity++ }
		}
		if parity&1 == 0 { z.F |= 0x04 }

		// Write result back
		switch reg {
		case 0: z.B = result
		case 1: z.C = result
		case 2: z.D = result
		case 3: z.E = result
		case 4: z.H = result
		case 5: z.L = result
		case 6: z.Mem.Write(addr, result)
		case 7: z.A = result
		}
		z.Cycles += 8

	case opcode >= 0x40 && opcode <= 0x7F: // BIT b, r
		var val byte
		switch reg {
		case 0: val = z.B
		case 1: val = z.C
		case 2: val = z.D
		case 3: val = z.E
		case 4: val = z.H
		case 5: val = z.L
		case 6: val = z.Mem.Read(z.HL()); z.Cycles += 4
		case 7: val = z.A
		}
		z.F = (z.F & 0x01) | 0x10 // Keep C, set H
		if val&(1<<bit) == 0 {
			z.F |= 0x44 // Z and PV
		}
		if bit == 7 && val&0x80 != 0 {
			z.F |= 0x80 // S
		}
		z.Cycles += 8

	case opcode >= 0x80 && opcode <= 0xBF: // RES b, r
		mask := ^(byte(1) << bit)
		switch reg {
		case 0: z.B &= mask
		case 1: z.C &= mask
		case 2: z.D &= mask
		case 3: z.E &= mask
		case 4: z.H &= mask
		case 5: z.L &= mask
		case 6:
			addr := z.HL()
			z.Mem.Write(addr, z.Mem.Read(addr)&mask)
			z.Cycles += 7
		case 7: z.A &= mask
		}
		z.Cycles += 8

	case opcode >= 0xC0 && opcode <= 0xFF: // SET b, r
		mask := byte(1) << bit
		switch reg {
		case 0: z.B |= mask
		case 1: z.C |= mask
		case 2: z.D |= mask
		case 3: z.E |= mask
		case 4: z.H |= mask
		case 5: z.L |= mask
		case 6:
			addr := z.HL()
			z.Mem.Write(addr, z.Mem.Read(addr)|mask)
			z.Cycles += 7
		case 7: z.A |= mask
		}
		z.Cycles += 8
	}
}

func (z *CPU) executeDD(opcode byte) {
	// DD prefix: IX register operations
	switch opcode {
	case 0x21: // LD IX,nn
		z.IX = z.fetch16()
		z.Cycles += 14

	case 0x22: // LD (nn),IX
		addr := z.fetch16()
		z.Mem.Write(addr, byte(z.IX))
		z.Mem.Write(addr+1, byte(z.IX>>8))
		z.Cycles += 20

	case 0x2A: // LD IX,(nn)
		addr := z.fetch16()
		lo := z.Mem.Read(addr)
		hi := z.Mem.Read(addr + 1)
		z.IX = uint16(lo) | (uint16(hi) << 8)
		z.Cycles += 20

	case 0x23: // INC IX
		z.IX++
		z.Cycles += 10

	case 0x2B: // DEC IX
		z.IX--
		z.Cycles += 10

	// ADD IX,rr (16-bit add)
	case 0x09: z.addIX(z.BC()); z.Cycles += 15
	case 0x19: z.addIX(z.DE()); z.Cycles += 15
	case 0x29: z.addIX(z.IX); z.Cycles += 15
	case 0x39: z.addIX(z.SP); z.Cycles += 15

	case 0xE1: // POP IX - used by the loader at 0x781E
		lo := z.Mem.Read(z.SP)
		z.SP++
		hi := z.Mem.Read(z.SP)
		z.SP++
		z.IX = uint16(lo) | (uint16(hi) << 8)
		z.Cycles += 14

	case 0xE5: // PUSH IX
		z.SP--
		z.Mem.Write(z.SP, byte(z.IX>>8))
		z.SP--
		z.Mem.Write(z.SP, byte(z.IX))
		z.Cycles += 15

	case 0xE9: // JP (IX)
		z.PC = z.IX
		z.Cycles += 8

	case 0x46: // LD B,(IX+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.B = z.Mem.Read(addr)
		z.Cycles += 19

	case 0x4E: // LD C,(IX+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.C = z.Mem.Read(addr)
		z.Cycles += 19

	case 0x56: // LD D,(IX+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.D = z.Mem.Read(addr)
		z.Cycles += 19

	case 0x5E: // LD E,(IX+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.E = z.Mem.Read(addr)
		z.Cycles += 19

	case 0x66: // LD H,(IX+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.H = z.Mem.Read(addr)
		z.Cycles += 19

	case 0x6E: // LD L,(IX+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.L = z.Mem.Read(addr)
		z.Cycles += 19

	case 0x7E: // LD A,(IX+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.A = z.Mem.Read(addr)
		z.Cycles += 19

	// Store to (IX+d)
	case 0x70: // LD (IX+d),B
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.Mem.Write(addr, z.B)
		z.Cycles += 19

	case 0x71: // LD (IX+d),C
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.Mem.Write(addr, z.C)
		z.Cycles += 19

	case 0x72: // LD (IX+d),D
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.Mem.Write(addr, z.D)
		z.Cycles += 19

	case 0x73: // LD (IX+d),E
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.Mem.Write(addr, z.E)
		z.Cycles += 19

	case 0x74: // LD (IX+d),H
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.Mem.Write(addr, z.H)
		z.Cycles += 19

	case 0x75: // LD (IX+d),L
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.Mem.Write(addr, z.L)
		z.Cycles += 19

	case 0x77: // LD (IX+d),A
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.Mem.Write(addr, z.A)
		z.Cycles += 19

	case 0x36: // LD (IX+d),n
		offset := int8(z.fetch())
		val := z.fetch()
		addr := uint16(int32(z.IX) + int32(offset))
		z.Mem.Write(addr, val)
		z.Cycles += 19

	case 0xF9: // LD SP,IX
		z.SP = z.IX
		z.Cycles += 10

	// Arithmetic operations on (IX+d)
	case 0x86: // ADD A,(IX+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.add(z.Mem.Read(addr))
		z.Cycles += 19

	case 0x8E: // ADC A,(IX+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.adc(z.Mem.Read(addr))
		z.Cycles += 19

	case 0x96: // SUB (IX+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.sub(z.Mem.Read(addr))
		z.Cycles += 19

	case 0x9E: // SBC A,(IX+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.sbc(z.Mem.Read(addr))
		z.Cycles += 19

	case 0xA6: // AND (IX+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.and(z.Mem.Read(addr))
		z.Cycles += 19

	case 0xAE: // XOR (IX+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.xor(z.Mem.Read(addr))
		z.Cycles += 19

	case 0xB6: // OR (IX+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.or(z.Mem.Read(addr))
		z.Cycles += 19

	case 0xBE: // CP (IX+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IX) + int32(offset))
		z.cp(z.Mem.Read(addr))
		z.Cycles += 19

	// IX half-register operations (undocumented)
	case 0x26: // LD IXH, n
		z.IX = (z.IX & 0x00FF) | (uint16(z.fetch()) << 8)
		z.Cycles += 11
	case 0x2E: // LD IXL, n
		z.IX = (z.IX & 0xFF00) | uint16(z.fetch())
		z.Cycles += 11
	case 0x24: // INC IXH
		ixh := byte(z.IX >> 8)
		ixh = z.inc8(ixh)
		z.IX = (z.IX & 0x00FF) | (uint16(ixh) << 8)
		z.Cycles += 8
	case 0x25: // DEC IXH
		ixh := byte(z.IX >> 8)
		ixh = z.dec8(ixh)
		z.IX = (z.IX & 0x00FF) | (uint16(ixh) << 8)
		z.Cycles += 8
	case 0x2C: // INC IXL
		ixl := byte(z.IX)
		ixl = z.inc8(ixl)
		z.IX = (z.IX & 0xFF00) | uint16(ixl)
		z.Cycles += 8
	case 0x2D: // DEC IXL
		ixl := byte(z.IX)
		ixl = z.dec8(ixl)
		z.IX = (z.IX & 0xFF00) | uint16(ixl)
		z.Cycles += 8
	case 0x44: // LD B, IXH
		z.B = byte(z.IX >> 8)
		z.Cycles += 8
	case 0x45: // LD B, IXL
		z.B = byte(z.IX)
		z.Cycles += 8
	case 0x4C: // LD C, IXH
		z.C = byte(z.IX >> 8)
		z.Cycles += 8
	case 0x4D: // LD C, IXL
		z.C = byte(z.IX)
		z.Cycles += 8
	case 0x54: // LD D, IXH
		z.D = byte(z.IX >> 8)
		z.Cycles += 8
	case 0x55: // LD D, IXL
		z.D = byte(z.IX)
		z.Cycles += 8
	case 0x5C: // LD E, IXH
		z.E = byte(z.IX >> 8)
		z.Cycles += 8
	case 0x5D: // LD E, IXL
		z.E = byte(z.IX)
		z.Cycles += 8
	case 0x60: // LD IXH, B
		z.IX = (z.IX & 0x00FF) | (uint16(z.B) << 8)
		z.Cycles += 8
	case 0x61: // LD IXH, C
		z.IX = (z.IX & 0x00FF) | (uint16(z.C) << 8)
		z.Cycles += 8
	case 0x62: // LD IXH, D
		z.IX = (z.IX & 0x00FF) | (uint16(z.D) << 8)
		z.Cycles += 8
	case 0x63: // LD IXH, E
		z.IX = (z.IX & 0x00FF) | (uint16(z.E) << 8)
		z.Cycles += 8
	case 0x64: // LD IXH, IXH (NOP)
		z.Cycles += 8
	case 0x65: // LD IXH, IXL
		z.IX = (z.IX & 0x00FF) | (uint16(byte(z.IX)) << 8)
		z.Cycles += 8
	case 0x67: // LD IXH, A
		z.IX = (z.IX & 0x00FF) | (uint16(z.A) << 8)
		z.Cycles += 8
	case 0x68: // LD IXL, B
		z.IX = (z.IX & 0xFF00) | uint16(z.B)
		z.Cycles += 8
	case 0x69: // LD IXL, C
		z.IX = (z.IX & 0xFF00) | uint16(z.C)
		z.Cycles += 8
	case 0x6A: // LD IXL, D
		z.IX = (z.IX & 0xFF00) | uint16(z.D)
		z.Cycles += 8
	case 0x6B: // LD IXL, E
		z.IX = (z.IX & 0xFF00) | uint16(z.E)
		z.Cycles += 8
	case 0x6C: // LD IXL, IXH
		z.IX = (z.IX & 0xFF00) | uint16(byte(z.IX>>8))
		z.Cycles += 8
	case 0x6D: // LD IXL, IXL (NOP)
		z.Cycles += 8
	case 0x6F: // LD IXL, A
		z.IX = (z.IX & 0xFF00) | uint16(z.A)
		z.Cycles += 8
	case 0x7C: // LD A, IXH
		z.A = byte(z.IX >> 8)
		z.Cycles += 8
	case 0x7D: // LD A, IXL
		z.A = byte(z.IX)
		z.Cycles += 8
	case 0x84: // ADD A, IXH
		z.add(byte(z.IX >> 8))
		z.Cycles += 8
	case 0x85: // ADD A, IXL
		z.add(byte(z.IX))
		z.Cycles += 8
	case 0x8C: // ADC A, IXH
		z.adc(byte(z.IX >> 8))
		z.Cycles += 8
	case 0x8D: // ADC A, IXL
		z.adc(byte(z.IX))
		z.Cycles += 8
	case 0x94: // SUB IXH
		z.sub(byte(z.IX >> 8))
		z.Cycles += 8
	case 0x95: // SUB IXL
		z.sub(byte(z.IX))
		z.Cycles += 8
	case 0x9C: // SBC A, IXH
		z.sbc(byte(z.IX >> 8))
		z.Cycles += 8
	case 0x9D: // SBC A, IXL
		z.sbc(byte(z.IX))
		z.Cycles += 8
	case 0xA4: // AND IXH
		z.and(byte(z.IX >> 8))
		z.Cycles += 8
	case 0xA5: // AND IXL
		z.and(byte(z.IX))
		z.Cycles += 8
	case 0xAC: // XOR IXH
		z.xor(byte(z.IX >> 8))
		z.Cycles += 8
	case 0xAD: // XOR IXL
		z.xor(byte(z.IX))
		z.Cycles += 8
	case 0xB4: // OR IXH
		z.or(byte(z.IX >> 8))
		z.Cycles += 8
	case 0xB5: // OR IXL
		z.or(byte(z.IX))
		z.Cycles += 8
	case 0xBC: // CP IXH
		z.cp(byte(z.IX >> 8))
		z.Cycles += 8
	case 0xBD: // CP IXL
		z.cp(byte(z.IX))
		z.Cycles += 8

	default:
		// Unknown DD instruction
		panic(fmt.Sprintf("Unimplemented DD instruction: DD %02X at PC=%04X", opcode, z.PC-2))
	}
}

func (z *CPU) executeFD(opcode byte) {
	// FD prefix: IY register operations
	switch opcode {
	case 0x21: // LD IY,nn
		z.IY = z.fetch16()
		z.Cycles += 14

	case 0x22: // LD (nn),IY
		addr := z.fetch16()
		z.Mem.Write(addr, byte(z.IY))
		z.Mem.Write(addr+1, byte(z.IY>>8))
		z.Cycles += 20

	case 0x2A: // LD IY,(nn)
		addr := z.fetch16()
		lo := z.Mem.Read(addr)
		hi := z.Mem.Read(addr + 1)
		z.IY = uint16(lo) | (uint16(hi) << 8)
		z.Cycles += 20

	case 0x23: // INC IY
		z.IY++
		z.Cycles += 10

	case 0x2B: // DEC IY
		z.IY--
		z.Cycles += 10

	case 0x2D: // DEC IYL (undocumented)
		iyl := byte(z.IY)
		iyl = z.dec8(iyl)
		z.IY = (z.IY & 0xFF00) | uint16(iyl)
		z.Cycles += 8

	case 0x2E: // LD IYL,n (undocumented)
		iyl := z.fetch()
		z.IY = (z.IY & 0xFF00) | uint16(iyl)
		z.Cycles += 11

	case 0x34: // INC (IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		val := z.Mem.Read(addr)
		z.Mem.Write(addr, z.inc8(val))
		z.Cycles += 23

	case 0x35: // DEC (IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		val := z.Mem.Read(addr)
		z.Mem.Write(addr, z.dec8(val))
		z.Cycles += 23

	case 0x36: // LD (IY+d),n
		offset := int8(z.fetch())
		n := z.fetch()
		addr := uint16(int32(z.IY) + int32(offset))
		z.Mem.Write(addr, n)
		z.Cycles += 19

	// LD r,(IY+d) instructions
	case 0x46: // LD B,(IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.B = z.Mem.Read(addr)
		z.Cycles += 19
	case 0x4E: // LD C,(IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.C = z.Mem.Read(addr)
		z.Cycles += 19
	case 0x56: // LD D,(IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.D = z.Mem.Read(addr)
		z.Cycles += 19
	case 0x5E: // LD E,(IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.E = z.Mem.Read(addr)
		z.Cycles += 19
	case 0x66: // LD H,(IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.H = z.Mem.Read(addr)
		z.Cycles += 19
	case 0x6E: // LD L,(IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.L = z.Mem.Read(addr)
		z.Cycles += 19
	case 0x7E: // LD A,(IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.A = z.Mem.Read(addr)
		z.Cycles += 19

	// Arithmetic operations on (IY+d)
	case 0x86: // ADD A,(IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.add(z.Mem.Read(addr))
		z.Cycles += 19
	case 0x8E: // ADC A,(IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.adc(z.Mem.Read(addr))
		z.Cycles += 19
	case 0x96: // SUB (IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.sub(z.Mem.Read(addr))
		z.Cycles += 19
	case 0x9E: // SBC A,(IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.sbc(z.Mem.Read(addr))
		z.Cycles += 19
	case 0xA6: // AND (IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.and(z.Mem.Read(addr))
		z.Cycles += 19
	case 0xAE: // XOR (IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.xor(z.Mem.Read(addr))
		z.Cycles += 19
	case 0xB6: // OR (IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.or(z.Mem.Read(addr))
		z.Cycles += 19
	case 0xBE: // CP (IY+d)
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.cp(z.Mem.Read(addr))
		z.Cycles += 19

	// LD (IY+d),r instructions
	case 0x70: // LD (IY+d),B
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.Mem.Write(addr, z.B)
		z.Cycles += 19
	case 0x71: // LD (IY+d),C
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.Mem.Write(addr, z.C)
		z.Cycles += 19
	case 0x72: // LD (IY+d),D
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.Mem.Write(addr, z.D)
		z.Cycles += 19
	case 0x73: // LD (IY+d),E
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.Mem.Write(addr, z.E)
		z.Cycles += 19
	case 0x74: // LD (IY+d),H
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.Mem.Write(addr, z.H)
		z.Cycles += 19
	case 0x75: // LD (IY+d),L
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.Mem.Write(addr, z.L)
		z.Cycles += 19
	case 0x77: // LD (IY+d),A
		offset := int8(z.fetch())
		addr := uint16(int32(z.IY) + int32(offset))
		z.Mem.Write(addr, z.A)
		z.Cycles += 19

	case 0xE1: // POP IY
		lo := z.Mem.Read(z.SP)
		z.SP++
		hi := z.Mem.Read(z.SP)
		z.SP++
		z.IY = uint16(lo) | (uint16(hi) << 8)
		z.Cycles += 14

	case 0xE5: // PUSH IY
		z.SP--
		z.Mem.Write(z.SP, byte(z.IY>>8))
		z.SP--
		z.Mem.Write(z.SP, byte(z.IY))
		z.Cycles += 15

	case 0xE9: // JP (IY)
		z.PC = z.IY
		z.Cycles += 8

	case 0xCB: // FD CB - bit operations on (IY+d)
		offset := int8(z.fetch())
		bitOp := z.fetch()
		addr := uint16(int32(z.IY) + int32(offset))

		// Simple implementation for common operations
		bit := (bitOp >> 3) & 0x07

		switch {
		case bitOp >= 0x40 && bitOp <= 0x7F: // BIT b,(IY+d)
			val := z.Mem.Read(addr)
			z.F = (z.F & 0x01) | 0x10 // Keep C, set H
			if val&(1<<bit) == 0 {
				z.F |= 0x44 // Z and PV
			}
			if bit == 7 && val&0x80 != 0 {
				z.F |= 0x80 // S
			}
			z.Cycles += 20

		case bitOp >= 0x80 && bitOp <= 0xBF: // RES b,(IY+d)
			val := z.Mem.Read(addr)
			val &= ^(byte(1) << bit)
			z.Mem.Write(addr, val)
			z.Cycles += 23

		case bitOp >= 0xC0 && bitOp <= 0xFF: // SET b,(IY+d)
			val := z.Mem.Read(addr)
			val |= byte(1) << bit
			z.Mem.Write(addr, val)
			z.Cycles += 23

		default:
			panic(fmt.Sprintf("Unimplemented FD CB instruction: FD CB %02X %02X at PC=%04X", offset, bitOp, z.PC-4))
		}

	case 0xF9: // LD SP,IY
		z.SP = z.IY
		z.Cycles += 10

	default:
		// Unknown FD instruction
		panic(fmt.Sprintf("Unimplemented FD instruction: FD %02X at PC=%04X", opcode, z.PC-2))
	}
}

func (z *CPU) executeMain(opcode byte) {
	switch opcode {
	case 0x00: // NOP
		z.Cycles += 4

	case 0x76: // HALT
		z.Halted = true
		z.Cycles += 4

	// 8-bit loads
	case 0x02: z.Mem.Write(z.BC(), z.A); z.Cycles += 7  // LD (BC),A
	case 0x12: z.Mem.Write(z.DE(), z.A); z.Cycles += 7  // LD (DE),A
	case 0x0A: z.A = z.Mem.Read(z.BC()); z.Cycles += 7  // LD A,(BC)
	case 0x1A: z.A = z.Mem.Read(z.DE()); z.Cycles += 7  // LD A,(DE)

	case 0x06: z.B = z.fetch(); z.Cycles += 7
	case 0x0E: z.C = z.fetch(); z.Cycles += 7
	case 0x16: z.D = z.fetch(); z.Cycles += 7
	case 0x1E: z.E = z.fetch(); z.Cycles += 7
	case 0x26: z.H = z.fetch(); z.Cycles += 7
	case 0x2E: z.L = z.fetch(); z.Cycles += 7
	case 0x3E: z.A = z.fetch(); z.Cycles += 7

	// LD r, r (register copies)
	case 0x40: z.B = z.B; z.Cycles += 4
	case 0x41: z.B = z.C; z.Cycles += 4
	case 0x42: z.B = z.D; z.Cycles += 4
	case 0x43: z.B = z.E; z.Cycles += 4
	case 0x44: z.B = z.H; z.Cycles += 4
	case 0x45: z.B = z.L; z.Cycles += 4
	case 0x47: z.B = z.A; z.Cycles += 4

	case 0x48: z.C = z.B; z.Cycles += 4
	case 0x49: z.C = z.C; z.Cycles += 4
	case 0x4A: z.C = z.D; z.Cycles += 4
	case 0x4B: z.C = z.E; z.Cycles += 4
	case 0x4C: z.C = z.H; z.Cycles += 4
	case 0x4D: z.C = z.L; z.Cycles += 4
	case 0x4F: z.C = z.A; z.Cycles += 4

	case 0x50: z.D = z.B; z.Cycles += 4
	case 0x51: z.D = z.C; z.Cycles += 4
	case 0x52: z.D = z.D; z.Cycles += 4
	case 0x53: z.D = z.E; z.Cycles += 4
	case 0x54: z.D = z.H; z.Cycles += 4
	case 0x55: z.D = z.L; z.Cycles += 4
	case 0x57: z.D = z.A; z.Cycles += 4

	case 0x58: z.E = z.B; z.Cycles += 4
	case 0x59: z.E = z.C; z.Cycles += 4
	case 0x5A: z.E = z.D; z.Cycles += 4
	case 0x5B: z.E = z.E; z.Cycles += 4
	case 0x5C: z.E = z.H; z.Cycles += 4
	case 0x5D: z.E = z.L; z.Cycles += 4
	case 0x5F: z.E = z.A; z.Cycles += 4

	case 0x60: z.H = z.B; z.Cycles += 4
	case 0x61: z.H = z.C; z.Cycles += 4
	case 0x62: z.H = z.D; z.Cycles += 4
	case 0x63: z.H = z.E; z.Cycles += 4
	case 0x64: z.H = z.H; z.Cycles += 4
	case 0x65: z.H = z.L; z.Cycles += 4
	case 0x67: z.H = z.A; z.Cycles += 4

	case 0x68: z.L = z.B; z.Cycles += 4
	case 0x69: z.L = z.C; z.Cycles += 4
	case 0x6A: z.L = z.D; z.Cycles += 4
	case 0x6B: z.L = z.E; z.Cycles += 4
	case 0x6C: z.L = z.H; z.Cycles += 4
	case 0x6D: z.L = z.L; z.Cycles += 4
	case 0x6F: z.L = z.A; z.Cycles += 4

	case 0x78: z.A = z.B; z.Cycles += 4
	case 0x79: z.A = z.C; z.Cycles += 4
	case 0x7A: z.A = z.D; z.Cycles += 4
	case 0x7B: z.A = z.E; z.Cycles += 4
	case 0x7C: z.A = z.H; z.Cycles += 4
	case 0x7D: z.A = z.L; z.Cycles += 4
	case 0x7F: z.A = z.A; z.Cycles += 4

	// LD r, (HL)
	case 0x46: z.B = z.Mem.Read(z.HL()); z.Cycles += 7
	case 0x4E: z.C = z.Mem.Read(z.HL()); z.Cycles += 7
	case 0x56: z.D = z.Mem.Read(z.HL()); z.Cycles += 7
	case 0x5E: z.E = z.Mem.Read(z.HL()); z.Cycles += 7
	case 0x66: z.H = z.Mem.Read(z.HL()); z.Cycles += 7
	case 0x6E: z.L = z.Mem.Read(z.HL()); z.Cycles += 7
	case 0x7E: z.A = z.Mem.Read(z.HL()); z.Cycles += 7

	// LD (HL), r
	case 0x70: z.Mem.Write(z.HL(), z.B); z.Cycles += 7
	case 0x71: z.Mem.Write(z.HL(), z.C); z.Cycles += 7
	case 0x72: z.Mem.Write(z.HL(), z.D); z.Cycles += 7
	case 0x73: z.Mem.Write(z.HL(), z.E); z.Cycles += 7
	case 0x74: z.Mem.Write(z.HL(), z.H); z.Cycles += 7
	case 0x75: z.Mem.Write(z.HL(), z.L); z.Cycles += 7
	case 0x77: z.Mem.Write(z.HL(), z.A); z.Cycles += 7
	case 0x36: // LD (HL), n
		z.Mem.Write(z.HL(), z.fetch())
		z.Cycles += 10

	// 16-bit loads
	case 0x01: z.setBC(z.fetch16()); z.Cycles += 10
	case 0x11: z.setDE(z.fetch16()); z.Cycles += 10
	case 0x21: z.setHL(z.fetch16()); z.Cycles += 10
	case 0x31: z.SP = z.fetch16(); z.Cycles += 10

	// Memory loads with 16-bit address
	case 0x22: // LD (nn),HL
		addr := z.fetch16()
		z.Mem.Write(addr, z.L)
		z.Mem.Write(addr+1, z.H)
		z.Cycles += 16

	case 0x2A: // LD HL,(nn)
		addr := z.fetch16()
		z.L = z.Mem.Read(addr)
		z.H = z.Mem.Read(addr + 1)
		z.Cycles += 16

	case 0x32: // LD (nn),A
		addr := z.fetch16()
		z.Mem.Write(addr, z.A)
		z.Cycles += 13

	case 0x3A: // LD A,(nn)
		addr := z.fetch16()
		z.A = z.Mem.Read(addr)
		z.Cycles += 13

	// INC/DEC 8-bit
	case 0x04: z.B = z.inc8(z.B); z.Cycles += 4
	case 0x0C: z.C = z.inc8(z.C); z.Cycles += 4
	case 0x14: z.D = z.inc8(z.D); z.Cycles += 4
	case 0x1C: z.E = z.inc8(z.E); z.Cycles += 4
	case 0x24: z.H = z.inc8(z.H); z.Cycles += 4
	case 0x2C: z.L = z.inc8(z.L); z.Cycles += 4
	case 0x3C: z.A = z.inc8(z.A); z.Cycles += 4

	case 0x05: z.B = z.dec8(z.B); z.Cycles += 4
	case 0x0D: z.C = z.dec8(z.C); z.Cycles += 4
	case 0x15: z.D = z.dec8(z.D); z.Cycles += 4
	case 0x1D: z.E = z.dec8(z.E); z.Cycles += 4
	case 0x25: z.H = z.dec8(z.H); z.Cycles += 4
	case 0x2D: z.L = z.dec8(z.L); z.Cycles += 4
	case 0x34: // INC (HL)
		addr := z.HL()
		val := z.Mem.Read(addr)
		z.Mem.Write(addr, z.inc8(val))
		z.Cycles += 11
	case 0x35: // DEC (HL)
		addr := z.HL()
		val := z.Mem.Read(addr)
		z.Mem.Write(addr, z.dec8(val))
		z.Cycles += 11
	case 0x3D: z.A = z.dec8(z.A); z.Cycles += 4

	// INC/DEC 16-bit
	case 0x03: z.setBC(z.BC() + 1); z.Cycles += 6
	case 0x13: z.setDE(z.DE() + 1); z.Cycles += 6
	case 0x23: z.setHL(z.HL() + 1); z.Cycles += 6
	case 0x33: z.SP++; z.Cycles += 6

	case 0x0B: z.setBC(z.BC() - 1); z.Cycles += 6
	case 0x1B: z.setDE(z.DE() - 1); z.Cycles += 6
	case 0x2B: z.setHL(z.HL() - 1); z.Cycles += 6
	case 0x3B: z.SP--; z.Cycles += 6

	// ADD HL,rr (16-bit add)
	case 0x09: z.addHL(z.BC()); z.Cycles += 11
	case 0x19: z.addHL(z.DE()); z.Cycles += 11
	case 0x29: z.addHL(z.HL()); z.Cycles += 11
	case 0x39: z.addHL(z.SP); z.Cycles += 11

	// Arithmetic
	case 0x80: z.add(z.B); z.Cycles += 4
	case 0x81: z.add(z.C); z.Cycles += 4
	case 0x82: z.add(z.D); z.Cycles += 4
	case 0x83: z.add(z.E); z.Cycles += 4
	case 0x84: z.add(z.H); z.Cycles += 4
	case 0x85: z.add(z.L); z.Cycles += 4
	case 0x86: z.add(z.Mem.Read(z.HL())); z.Cycles += 7
	case 0x87: z.add(z.A); z.Cycles += 4
	case 0xC6: z.add(z.fetch()); z.Cycles += 7

	// ADC (add with carry)
	case 0x88: z.adc(z.B); z.Cycles += 4
	case 0x89: z.adc(z.C); z.Cycles += 4
	case 0x8A: z.adc(z.D); z.Cycles += 4
	case 0x8B: z.adc(z.E); z.Cycles += 4
	case 0x8C: z.adc(z.H); z.Cycles += 4
	case 0x8D: z.adc(z.L); z.Cycles += 4
	case 0x8E: z.adc(z.Mem.Read(z.HL())); z.Cycles += 7
	case 0x8F: z.adc(z.A); z.Cycles += 4
	case 0xCE: z.adc(z.fetch()); z.Cycles += 7

	case 0x90: z.sub(z.B); z.Cycles += 4
	case 0x91: z.sub(z.C); z.Cycles += 4
	case 0x92: z.sub(z.D); z.Cycles += 4
	case 0x93: z.sub(z.E); z.Cycles += 4
	case 0x94: z.sub(z.H); z.Cycles += 4
	case 0x95: z.sub(z.L); z.Cycles += 4
	case 0x96: z.sub(z.Mem.Read(z.HL())); z.Cycles += 7
	case 0x97: z.sub(z.A); z.Cycles += 4
	case 0xD6: z.sub(z.fetch()); z.Cycles += 7

	// SBC (subtract with carry)
	case 0x98: z.sbc(z.B); z.Cycles += 4
	case 0x99: z.sbc(z.C); z.Cycles += 4
	case 0x9A: z.sbc(z.D); z.Cycles += 4
	case 0x9B: z.sbc(z.E); z.Cycles += 4
	case 0x9C: z.sbc(z.H); z.Cycles += 4
	case 0x9D: z.sbc(z.L); z.Cycles += 4
	case 0x9E: z.sbc(z.Mem.Read(z.HL())); z.Cycles += 7
	case 0x9F: z.sbc(z.A); z.Cycles += 4
	case 0xDE: z.sbc(z.fetch()); z.Cycles += 7

	// Logic
	case 0xA0: z.and(z.B); z.Cycles += 4
	case 0xA1: z.and(z.C); z.Cycles += 4
	case 0xA2: z.and(z.D); z.Cycles += 4
	case 0xA3: z.and(z.E); z.Cycles += 4
	case 0xA4: z.and(z.H); z.Cycles += 4
	case 0xA5: z.and(z.L); z.Cycles += 4
	case 0xA6: z.and(z.Mem.Read(z.HL())); z.Cycles += 7
	case 0xA7: z.and(z.A); z.Cycles += 4
	case 0xE6: z.and(z.fetch()); z.Cycles += 7

	case 0xB0: z.or(z.B); z.Cycles += 4
	case 0xB1: z.or(z.C); z.Cycles += 4
	case 0xB2: z.or(z.D); z.Cycles += 4
	case 0xB3: z.or(z.E); z.Cycles += 4
	case 0xB4: z.or(z.H); z.Cycles += 4
	case 0xB5: z.or(z.L); z.Cycles += 4
	case 0xB6: z.or(z.Mem.Read(z.HL())); z.Cycles += 7
	case 0xB7: z.or(z.A); z.Cycles += 4
	case 0xF6: z.or(z.fetch()); z.Cycles += 7

	case 0xA8: z.xor(z.B); z.Cycles += 4
	case 0xA9: z.xor(z.C); z.Cycles += 4
	case 0xAA: z.xor(z.D); z.Cycles += 4
	case 0xAB: z.xor(z.E); z.Cycles += 4
	case 0xAC: z.xor(z.H); z.Cycles += 4
	case 0xAD: z.xor(z.L); z.Cycles += 4
	case 0xAE: z.xor(z.Mem.Read(z.HL())); z.Cycles += 7
	case 0xAF: z.xor(z.A); z.Cycles += 4
	case 0xEE: z.xor(z.fetch()); z.Cycles += 7

	// Compare
	case 0xB8: z.cp(z.B); z.Cycles += 4
	case 0xB9: z.cp(z.C); z.Cycles += 4
	case 0xBA: z.cp(z.D); z.Cycles += 4
	case 0xBB: z.cp(z.E); z.Cycles += 4
	case 0xBC: z.cp(z.H); z.Cycles += 4
	case 0xBD: z.cp(z.L); z.Cycles += 4
	case 0xBE: z.cp(z.Mem.Read(z.HL())); z.Cycles += 7
	case 0xBF: z.cp(z.A); z.Cycles += 4
	case 0xFE: z.cp(z.fetch()); z.Cycles += 7

	// Jumps
	case 0xC3: z.PC = z.fetch16(); z.Cycles += 10
	case 0xC2:
		addr := z.fetch16()
		if z.F&0x40 == 0 { z.PC = addr }
		z.Cycles += 10
	case 0xCA:
		addr := z.fetch16()
		if z.F&0x40 != 0 { z.PC = addr }
		z.Cycles += 10
	case 0xD2:
		addr := z.fetch16()
		if z.F&0x01 == 0 { z.PC = addr }
		z.Cycles += 10
	case 0xDA:
		addr := z.fetch16()
		if z.F&0x01 != 0 { z.PC = addr }
		z.Cycles += 10
	case 0xE2:
		addr := z.fetch16()
		if z.F&0x04 == 0 { z.PC = addr } // JP PO (parity odd)
		z.Cycles += 10
	case 0xEA:
		addr := z.fetch16()
		if z.F&0x04 != 0 { z.PC = addr } // JP PE (parity even)
		z.Cycles += 10
	case 0xF2:
		addr := z.fetch16()
		if z.F&0x80 == 0 { z.PC = addr } // JP P (positive/plus)
		z.Cycles += 10
	case 0xFA:
		addr := z.fetch16()
		if z.F&0x80 != 0 { z.PC = addr } // JP M (minus/negative)
		z.Cycles += 10
	case 0xE9: z.PC = z.HL(); z.Cycles += 4

	// Relative jumps
	case 0x18:
		offset := int8(z.fetch())
		z.PC = uint16(int32(z.PC) + int32(offset))
		z.Cycles += 12
	case 0x20:
		offset := int8(z.fetch())
		if z.F&0x40 == 0 {
			z.PC = uint16(int32(z.PC) + int32(offset))
			z.Cycles += 12
		} else {
			z.Cycles += 7
		}
	case 0x28:
		offset := int8(z.fetch())
		if z.F&0x40 != 0 {
			z.PC = uint16(int32(z.PC) + int32(offset))
			z.Cycles += 12
		} else {
			z.Cycles += 7
		}
	case 0x30:
		offset := int8(z.fetch())
		if z.F&0x01 == 0 {
			z.PC = uint16(int32(z.PC) + int32(offset))
			z.Cycles += 12
		} else {
			z.Cycles += 7
		}
	case 0x38:
		offset := int8(z.fetch())
		if z.F&0x01 != 0 {
			z.PC = uint16(int32(z.PC) + int32(offset))
			z.Cycles += 12
		} else {
			z.Cycles += 7
		}

	// Calls and returns
	case 0xCD:
		addr := z.fetch16()
		z.push(z.PC)
		z.PC = addr
		z.Cycles += 17
	case 0xC9: z.PC = z.pop(); z.Cycles += 10

	// Conditional RETs
	case 0xC0: // RET NZ
		if z.F&0x40 == 0 {
			z.PC = z.pop()
			z.Cycles += 11
		} else {
			z.Cycles += 5
		}
	case 0xC8: // RET Z
		if z.F&0x40 != 0 {
			z.PC = z.pop()
			z.Cycles += 11
		} else {
			z.Cycles += 5
		}
	case 0xD0: // RET NC
		if z.F&0x01 == 0 {
			z.PC = z.pop()
			z.Cycles += 11
		} else {
			z.Cycles += 5
		}
	case 0xD8: // RET C
		if z.F&0x01 != 0 {
			z.PC = z.pop()
			z.Cycles += 11
		} else {
			z.Cycles += 5
		}
	case 0xE0: // RET PO
		if z.F&0x04 == 0 {
			z.PC = z.pop()
			z.Cycles += 11
		} else {
			z.Cycles += 5
		}
	case 0xE8: // RET PE
		if z.F&0x04 != 0 {
			z.PC = z.pop()
			z.Cycles += 11
		} else {
			z.Cycles += 5
		}
	case 0xF0: // RET P
		if z.F&0x80 == 0 {
			z.PC = z.pop()
			z.Cycles += 11
		} else {
			z.Cycles += 5
		}
	case 0xF8: // RET M
		if z.F&0x80 != 0 {
			z.PC = z.pop()
			z.Cycles += 11
		} else {
			z.Cycles += 5
		}

	// Conditional CALLs
	case 0xC4: // CALL NZ,nnnn
		addr := z.fetch16()
		if z.F&0x40 == 0 { // Z flag not set
			z.push(z.PC)
			z.PC = addr
			z.Cycles += 17
		} else {
			z.Cycles += 10
		}
	case 0xCC: // CALL Z,nnnn
		addr := z.fetch16()
		if z.F&0x40 != 0 { // Z flag set
			z.push(z.PC)
			z.PC = addr
			z.Cycles += 17
		} else {
			z.Cycles += 10
		}
	case 0xD4: // CALL NC,nnnn
		addr := z.fetch16()
		if z.F&0x01 == 0 { // C flag not set
			z.push(z.PC)
			z.PC = addr
			z.Cycles += 17
		} else {
			z.Cycles += 10
		}
	case 0xDC: // CALL C,nnnn
		addr := z.fetch16()
		if z.F&0x01 != 0 { // C flag set
			z.push(z.PC)
			z.PC = addr
			z.Cycles += 17
		} else {
			z.Cycles += 10
		}
	case 0xE4: // CALL PO,nnnn
		addr := z.fetch16()
		if z.F&0x04 == 0 { // P/V flag not set
			z.push(z.PC)
			z.PC = addr
			z.Cycles += 17
		} else {
			z.Cycles += 10
		}
	case 0xEC: // CALL PE,nnnn
		addr := z.fetch16()
		if z.F&0x04 != 0 { // P/V flag set
			z.push(z.PC)
			z.PC = addr
			z.Cycles += 17
		} else {
			z.Cycles += 10
		}
	case 0xF4: // CALL P,nnnn
		addr := z.fetch16()
		if z.F&0x80 == 0 { // S flag not set
			z.push(z.PC)
			z.PC = addr
			z.Cycles += 17
		} else {
			z.Cycles += 10
		}
	case 0xFC: // CALL M,nnnn
		addr := z.fetch16()
		if z.F&0x80 != 0 { // S flag set
			z.push(z.PC)
			z.PC = addr
			z.Cycles += 17
		} else {
			z.Cycles += 10
		}

	// RST (restart) instructions
	case 0xC7: z.push(z.PC); z.PC = 0x0000; z.Cycles += 11
	case 0xCF: z.push(z.PC); z.PC = 0x0008; z.Cycles += 11
	case 0xD7: z.push(z.PC); z.PC = 0x0010; z.Cycles += 11
	case 0xDF: z.push(z.PC); z.PC = 0x0018; z.Cycles += 11
	case 0xE7: z.push(z.PC); z.PC = 0x0020; z.Cycles += 11
	case 0xEF: z.push(z.PC); z.PC = 0x0028; z.Cycles += 11
	case 0xF7: z.push(z.PC); z.PC = 0x0030; z.Cycles += 11
	case 0xFF: z.push(z.PC); z.PC = 0x0038; z.Cycles += 11

	// Stack
	case 0xC5: z.push(z.BC()); z.Cycles += 11
	case 0xD5: z.push(z.DE()); z.Cycles += 11
	case 0xE5: z.push(z.HL()); z.Cycles += 11
	case 0xF5: z.push(uint16(z.A)<<8|uint16(z.F)); z.Cycles += 11

	case 0xC1: z.setBC(z.pop()); z.Cycles += 10
	case 0xD1: z.setDE(z.pop()); z.Cycles += 10
	case 0xE1: z.setHL(z.pop()); z.Cycles += 10
	case 0xF1:
		val := z.pop()
		z.A = byte(val >> 8)
		z.F = byte(val)
		z.Cycles += 10

	// I/O
	case 0xD3: z.IO.Out(z.fetch(), z.A); z.Cycles += 11
	case 0xDB: z.A = z.IO.In(z.fetch()); z.Cycles += 11

	// Interrupts
	case 0xF3: z.IFF1 = false; z.IFF2 = false; z.Cycles += 4
	case 0xF9: z.SP = z.HL(); z.Cycles += 6
	case 0xFB: z.IFF1 = true; z.IFF2 = true; z.Cycles += 4

	// Exchange
	case 0xE3: // EX (SP),HL
		l := z.Mem.Read(z.SP)
		h := z.Mem.Read(z.SP + 1)
		z.Mem.Write(z.SP, z.L)
		z.Mem.Write(z.SP+1, z.H)
		z.L = l
		z.H = h
		z.Cycles += 19
	case 0xEB: z.D, z.H = z.H, z.D; z.E, z.L = z.L, z.E; z.Cycles += 4
	case 0x08: z.A, z.A_ = z.A_, z.A; z.F, z.F_ = z.F_, z.F; z.Cycles += 4
	case 0xD9:
		z.B, z.B_ = z.B_, z.B
		z.C, z.C_ = z.C_, z.C
		z.D, z.D_ = z.D_, z.D
		z.E, z.E_ = z.E_, z.E
		z.H, z.H_ = z.H_, z.H
		z.L, z.L_ = z.L_, z.L
		z.Cycles += 4

	// Rotates
	case 0x07:
		carry := z.A >> 7
		z.A = (z.A << 1) | carry
		z.F = (z.F & 0xC4) | carry
		z.Cycles += 4
	case 0x0F:
		carry := z.A & 1
		z.A = (z.A >> 1) | (carry << 7)
		z.F = (z.F & 0xC4) | carry
		z.Cycles += 4
	case 0x17:
		carry := z.F & 1
		newCarry := z.A >> 7
		z.A = (z.A << 1) | carry
		z.F = (z.F & 0xC4) | newCarry
		z.Cycles += 4
	case 0x1F:
		carry := z.F & 1
		newCarry := z.A & 1
		z.A = (z.A >> 1) | (carry << 7)
		z.F = (z.F & 0xC4) | newCarry
		z.Cycles += 4

	// DAA, CPL, SCF, CCF
	case 0x27: z.daa(); z.Cycles += 4
	case 0x2F: z.A = ^z.A; z.F |= 0x12; z.Cycles += 4
	case 0x37: z.F = (z.F & 0xC4) | 0x01; z.Cycles += 4
	case 0x3F: z.F ^= 0x01; z.Cycles += 4

	// DJNZ
	case 0x10:
		z.B--
		offset := int8(z.fetch())
		if z.B != 0 {
			z.PC = uint16(int32(z.PC) + int32(offset))
			z.Cycles += 13
		} else {
			z.Cycles += 8
		}

	default:
		panic(fmt.Sprintf("Unimplemented instruction: %02X at PC=%04X", opcode, z.PC-1))
	}
}

// Helper functions for 16-bit registers
func (z *CPU) BC() uint16 { return uint16(z.B)<<8 | uint16(z.C) }
func (z *CPU) DE() uint16 { return uint16(z.D)<<8 | uint16(z.E) }
func (z *CPU) HL() uint16 { return uint16(z.H)<<8 | uint16(z.L) }

func (z *CPU) setBC(val uint16) { z.B = byte(val >> 8); z.C = byte(val) }
func (z *CPU) setDE(val uint16) { z.D = byte(val >> 8); z.E = byte(val) }
func (z *CPU) setHL(val uint16) { z.H = byte(val >> 8); z.L = byte(val) }

// fetch16 reads a 16-bit little-endian value
func (z *CPU) fetch16() uint16 {
	low := z.fetch()
	high := z.fetch()
	return uint16(high)<<8 | uint16(low)
}

// Arithmetic and logic helpers with proper flag handling
// Z80 flags: S(7) Z(6) Y(5) H(4) X(3) P/V(2) N(1) C(0)

func (z *CPU) add(val byte) {
	result := uint16(z.A) + uint16(val)
	z.F = 0
	if result&0xFF == 0 { z.F |= 0x40 } // Z
	if result&0x80 != 0 { z.F |= 0x80 } // S
	if result > 0xFF { z.F |= 0x01 }    // C
	if (z.A^val^byte(result))&0x10 != 0 { z.F |= 0x10 } // H
	if ((z.A^val^0x80)&(val^byte(result))&0x80) != 0 { z.F |= 0x04 } // V
	z.A = byte(result)
}

func (z *CPU) adc(val byte) {
	carry := z.F & 0x01
	result := uint16(z.A) + uint16(val) + uint16(carry)
	z.F = 0
	if result&0xFF == 0 { z.F |= 0x40 } // Z
	if result&0x80 != 0 { z.F |= 0x80 } // S
	if result > 0xFF { z.F |= 0x01 }    // C
	if (z.A^val^byte(result))&0x10 != 0 { z.F |= 0x10 } // H
	if ((z.A^val^0x80)&(val^byte(result))&0x80) != 0 { z.F |= 0x04 } // V
	z.A = byte(result)
}

func (z *CPU) sub(val byte) {
	result := uint16(z.A) - uint16(val)
	z.F = 0x02 // N flag
	if result&0xFF == 0 { z.F |= 0x40 } // Z
	if result&0x80 != 0 { z.F |= 0x80 } // S
	if result > 0xFF { z.F |= 0x01 }    // C (borrow)
	if (z.A^val)&0x10 != 0 { z.F |= 0x10 } // H
	if ((z.A^val)&(z.A^byte(result))&0x80) != 0 { z.F |= 0x04 } // V
	z.A = byte(result)
}

func (z *CPU) sbc(val byte) {
	carry := z.F & 0x01
	result := uint16(z.A) - uint16(val) - uint16(carry)
	z.F = 0x02 // N flag
	if result&0xFF == 0 { z.F |= 0x40 } // Z
	if result&0x80 != 0 { z.F |= 0x80 } // S
	if result > 0xFF { z.F |= 0x01 }    // C (borrow)
	if (z.A^val^byte(result))&0x10 != 0 { z.F |= 0x10 } // H
	if ((z.A^val)&(z.A^byte(result))&0x80) != 0 { z.F |= 0x04 } // V
	z.A = byte(result)
}

func (z *CPU) and(val byte) {
	z.A &= val
	z.F = 0x10 // H flag always set
	if z.A == 0 { z.F |= 0x40 } // Z
	if z.A&0x80 != 0 { z.F |= 0x80 } // S
	// P/V is parity
	parity := byte(0)
	for i := 0; i < 8; i++ {
		if z.A&(1<<i) != 0 { parity++ }
	}
	if parity&1 == 0 { z.F |= 0x04 }
}

func (z *CPU) or(val byte) {
	z.A |= val
	z.F = 0
	if z.A == 0 { z.F |= 0x40 } // Z
	if z.A&0x80 != 0 { z.F |= 0x80 } // S
	// P/V is parity
	parity := byte(0)
	for i := 0; i < 8; i++ {
		if z.A&(1<<i) != 0 { parity++ }
	}
	if parity&1 == 0 { z.F |= 0x04 }
}

func (z *CPU) xor(val byte) {
	z.A ^= val
	z.F = 0
	if z.A == 0 { z.F |= 0x40 } // Z
	if z.A&0x80 != 0 { z.F |= 0x80 } // S
	// P/V is parity
	parity := byte(0)
	for i := 0; i < 8; i++ {
		if z.A&(1<<i) != 0 { parity++ }
	}
	if parity&1 == 0 { z.F |= 0x04 }
}

func (z *CPU) cp(val byte) {
	// Match jsspeccy's CP implementation exactly
	a := uint32(z.A)
	cptemp := a - uint32(val)

	// Lookup index for H and V flags
	// lookup = ((a & 0x88) >> 3) | ((val & 0x88) >> 2) | ((cptemp & 0x88) >> 1)
	lookup := ((a & 0x88) >> 3) | ((uint32(val) & 0x88) >> 2) | ((cptemp & 0x88) >> 1)

	// Build F register:
	// - Carry or Zero from result
	// - N flag (always 0x02 for CP)
	// - H flag from lookup table (lower 3 bits)
	// - V flag from lookup table (upper bits)
	// - Bits 3 and 5 copied from val (NOT result!)
	// - S flag from result bit 7

	var f byte
	if cptemp&0x100 != 0 {
		f = 0x01 // Carry
	} else if cptemp&0xFF == 0 {
		f = 0x40 // Zero
	}

	f |= 0x02                              // N flag
	f |= HalfCarrySubTable[lookup&0x07]   // H flag
	f |= OverflowSubTable[lookup>>4]       // V flag
	f |= val & (0x08 | 0x20)               // Bits 3 and 5 from val
	f |= byte(cptemp) & 0x80               // S flag from result

	z.F = f
	// Note: A is not modified in CP
}

func (z *CPU) inc8(val byte) byte {
	result := val + 1
	z.F = (z.F & 0x01) // Keep carry flag
	if result == 0 { z.F |= 0x40 } // Z
	if result&0x80 != 0 { z.F |= 0x80 } // S
	if val == 0x7F { z.F |= 0x04 } // V (overflow from 0x7F to 0x80)
	if (val&0x0F) == 0x0F { z.F |= 0x10 } // H
	return result
}

func (z *CPU) dec8(val byte) byte {
	result := val - 1
	z.F = (z.F & 0x01) | 0x02 // Keep carry, set N flag
	if result == 0 { z.F |= 0x40 } // Z
	if result&0x80 != 0 { z.F |= 0x80 } // S
	if val == 0x80 { z.F |= 0x04 } // V (overflow from 0x80 to 0x7F)
	if (val&0x0F) == 0 { z.F |= 0x10 } // H
	return result
}

func (z *CPU) addHL(val uint16) {
	hl := z.HL()
	result := uint32(hl) + uint32(val)
	z.F = (z.F & 0xC4) // Keep S, Z, P/V
	if result > 0xFFFF { z.F |= 0x01 } // C
	if ((hl^val^uint16(result))&0x1000) != 0 { z.F |= 0x10 } // H
	z.setHL(uint16(result))
}

func (z *CPU) addIX(val uint16) {
	result := uint32(z.IX) + uint32(val)
	z.F = (z.F & 0xC4) // Keep S, Z, P/V
	if result > 0xFFFF { z.F |= 0x01 } // C
	if ((z.IX^val^uint16(result))&0x1000) != 0 { z.F |= 0x10 } // H
	z.IX = uint16(result)
}

func (z *CPU) daa() {
	// Decimal Adjust Accumulator - complex BCD operation
	correction := byte(0)
	carry := false

	if z.F&0x10 != 0 || (z.A&0x0F) > 9 {
		correction = 0x06
	}

	if z.F&0x01 != 0 || z.A > 0x99 {
		correction |= 0x60
		carry = true
	}

	if z.F&0x02 != 0 { // Subtract flag
		z.A -= correction
	} else {
		z.A += correction
	}

	z.F = (z.F & 0x02) // Keep N flag
	if z.A == 0 { z.F |= 0x40 } // Z
	if z.A&0x80 != 0 { z.F |= 0x80 } // S
	if carry { z.F |= 0x01 } // C

	// Parity
	parity := byte(0)
	for i := 0; i < 8; i++ {
		if z.A&(1<<i) != 0 { parity++ }
	}
	if parity&1 == 0 { z.F |= 0x04 }
}

// adc16 performs 16-bit add with carry: HL = HL + rr + C
func (z *CPU) adc16(hl, rr uint16) uint16 {
	carry := uint32(z.F & 0x01)
	result := uint32(hl) + uint32(rr) + carry

	z.F = 0
	if result&0xFFFF == 0 { z.F |= 0x40 } // Z
	if result&0x8000 != 0 { z.F |= 0x80 } // S
	if result > 0xFFFF { z.F |= 0x01 }    // C
	if ((hl^rr)&0x8000) == 0 && ((hl^uint16(result))&0x8000) != 0 { z.F |= 0x04 } // V
	if ((hl&0x0FFF)+(rr&0x0FFF)+uint16(carry)) > 0x0FFF { z.F |= 0x10 } // H

	return uint16(result)
}

// sbc16 performs 16-bit subtract with carry: HL = HL - rr - C
func (z *CPU) sbc16(hl, rr uint16) uint16 {
	carry := uint32(z.F & 0x01)
	result := uint32(hl) - uint32(rr) - carry

	z.F = 0x02 // Set N flag (subtract)
	if result&0xFFFF == 0 { z.F |= 0x40 } // Z
	if result&0x8000 != 0 { z.F |= 0x80 } // S
	if result > 0xFFFF { z.F |= 0x01 }    // C (borrow)
	if ((hl^rr)&0x8000) != 0 && ((hl^uint16(result))&0x8000) != 0 { z.F |= 0x04 } // V
	if (int32(hl&0x0FFF) - int32(rr&0x0FFF) - int32(carry)) < 0 { z.F |= 0x10 } // H

	return uint16(result)
}
