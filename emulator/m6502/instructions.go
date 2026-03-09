package m6502

// execute dispatches the opcode to the appropriate instruction handler
func (c *CPU) execute(opcode byte) {
	// Instruction implementations with cycle-accurate timing
	switch opcode {
	// LDA - Load Accumulator
	case 0xA9: // LDA #imm
		addr, _ := c.addrImmediate()
		c.A = c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 2
	case 0xA5: // LDA zp
		addr, _ := c.addrZeroPage()
		c.A = c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 3
	case 0xB5: // LDA zp,X
		addr, _ := c.addrZeroPageX()
		c.A = c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
	case 0xAD: // LDA abs
		addr, _ := c.addrAbsolute()
		c.A = c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
	case 0xBD: // LDA abs,X
		addr, pageCrossed := c.addrAbsoluteX()
		c.A = c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}
	case 0xB9: // LDA abs,Y
		addr, pageCrossed := c.addrAbsoluteY()
		c.A = c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}
	case 0xA1: // LDA (ind,X)
		addr, _ := c.addrIndexedIndirect()
		c.A = c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 6
	case 0xB1: // LDA (ind),Y
		addr, pageCrossed := c.addrIndirectIndexed()
		c.A = c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 5
		if pageCrossed {
			c.Cycles++
		}

	// LDX - Load X Register
	case 0xA2: // LDX #imm
		addr, _ := c.addrImmediate()
		c.X = c.Mem.Read(addr)
		c.updateZN(c.X)
		c.Cycles += 2
	case 0xA6: // LDX zp
		addr, _ := c.addrZeroPage()
		c.X = c.Mem.Read(addr)
		c.updateZN(c.X)
		c.Cycles += 3
	case 0xB6: // LDX zp,Y
		addr, _ := c.addrZeroPageY()
		c.X = c.Mem.Read(addr)
		c.updateZN(c.X)
		c.Cycles += 4
	case 0xAE: // LDX abs
		addr, _ := c.addrAbsolute()
		c.X = c.Mem.Read(addr)
		c.updateZN(c.X)
		c.Cycles += 4
	case 0xBE: // LDX abs,Y
		addr, pageCrossed := c.addrAbsoluteY()
		c.X = c.Mem.Read(addr)
		c.updateZN(c.X)
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}

	// LDY - Load Y Register
	case 0xA0: // LDY #imm
		addr, _ := c.addrImmediate()
		c.Y = c.Mem.Read(addr)
		c.updateZN(c.Y)
		c.Cycles += 2
	case 0xA4: // LDY zp
		addr, _ := c.addrZeroPage()
		c.Y = c.Mem.Read(addr)
		c.updateZN(c.Y)
		c.Cycles += 3
	case 0xB4: // LDY zp,X
		addr, _ := c.addrZeroPageX()
		c.Y = c.Mem.Read(addr)
		c.updateZN(c.Y)
		c.Cycles += 4
	case 0xAC: // LDY abs
		addr, _ := c.addrAbsolute()
		c.Y = c.Mem.Read(addr)
		c.updateZN(c.Y)
		c.Cycles += 4
	case 0xBC: // LDY abs,X
		addr, pageCrossed := c.addrAbsoluteX()
		c.Y = c.Mem.Read(addr)
		c.updateZN(c.Y)
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}

	// STA - Store Accumulator
	case 0x85: // STA zp
		addr, _ := c.addrZeroPage()
		c.Mem.Write(addr, c.A)
		c.Cycles += 3
	case 0x95: // STA zp,X
		addr, _ := c.addrZeroPageX()
		c.Mem.Write(addr, c.A)
		c.Cycles += 4
	case 0x8D: // STA abs
		addr, _ := c.addrAbsolute()
		c.Mem.Write(addr, c.A)
		c.Cycles += 4
	case 0x9D: // STA abs,X
		addr, _ := c.addrAbsoluteX()
		c.Mem.Write(addr, c.A)
		c.Cycles += 5
	case 0x99: // STA abs,Y
		addr, _ := c.addrAbsoluteY()
		c.Mem.Write(addr, c.A)
		c.Cycles += 5
	case 0x81: // STA (ind,X)
		addr, _ := c.addrIndexedIndirect()
		c.Mem.Write(addr, c.A)
		c.Cycles += 6
	case 0x91: // STA (ind),Y
		addr, _ := c.addrIndirectIndexed()
		c.Mem.Write(addr, c.A)
		c.Cycles += 6

	// STX - Store X Register
	case 0x86: // STX zp
		addr, _ := c.addrZeroPage()
		c.Mem.Write(addr, c.X)
		c.Cycles += 3
	case 0x96: // STX zp,Y
		addr, _ := c.addrZeroPageY()
		c.Mem.Write(addr, c.X)
		c.Cycles += 4
	case 0x8E: // STX abs
		addr, _ := c.addrAbsolute()
		c.Mem.Write(addr, c.X)
		c.Cycles += 4

	// STY - Store Y Register
	case 0x84: // STY zp
		addr, _ := c.addrZeroPage()
		c.Mem.Write(addr, c.Y)
		c.Cycles += 3
	case 0x94: // STY zp,X
		addr, _ := c.addrZeroPageX()
		c.Mem.Write(addr, c.Y)
		c.Cycles += 4
	case 0x8C: // STY abs
		addr, _ := c.addrAbsolute()
		c.Mem.Write(addr, c.Y)
		c.Cycles += 4

	// Register transfers
	case 0xAA: // TAX
		c.X = c.A
		c.updateZN(c.X)
		c.Cycles += 2
	case 0xA8: // TAY
		c.Y = c.A
		c.updateZN(c.Y)
		c.Cycles += 2
	case 0x8A: // TXA
		c.A = c.X
		c.updateZN(c.A)
		c.Cycles += 2
	case 0x98: // TYA
		c.A = c.Y
		c.updateZN(c.A)
		c.Cycles += 2

	// Stack operations
	case 0xBA: // TSX
		c.X = c.SP
		c.updateZN(c.X)
		c.Cycles += 2
	case 0x9A: // TXS
		c.SP = c.X
		c.Cycles += 2
	case 0x48: // PHA
		c.push(c.A)
		c.Cycles += 3
	case 0x68: // PLA
		c.A = c.pop()
		c.updateZN(c.A)
		c.Cycles += 4
	case 0x08: // PHP
		c.push(c.P | FlagB | FlagU) // B and U flags set when pushed
		c.Cycles += 3
	case 0x28: // PLP
		c.P = (c.pop() &^ FlagB) | FlagU // B cleared, U always set
		c.Cycles += 4

	// Logical operations - AND
	case 0x29: // AND #imm
		addr, _ := c.addrImmediate()
		c.A &= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 2
	case 0x25: // AND zp
		addr, _ := c.addrZeroPage()
		c.A &= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 3
	case 0x35: // AND zp,X
		addr, _ := c.addrZeroPageX()
		c.A &= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
	case 0x2D: // AND abs
		addr, _ := c.addrAbsolute()
		c.A &= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
	case 0x3D: // AND abs,X
		addr, pageCrossed := c.addrAbsoluteX()
		c.A &= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}
	case 0x39: // AND abs,Y
		addr, pageCrossed := c.addrAbsoluteY()
		c.A &= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}
	case 0x21: // AND (ind,X)
		addr, _ := c.addrIndexedIndirect()
		c.A &= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 6
	case 0x31: // AND (ind),Y
		addr, pageCrossed := c.addrIndirectIndexed()
		c.A &= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 5
		if pageCrossed {
			c.Cycles++
		}

	// Logical operations - EOR (XOR)
	case 0x49: // EOR #imm
		addr, _ := c.addrImmediate()
		c.A ^= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 2
	case 0x45: // EOR zp
		addr, _ := c.addrZeroPage()
		c.A ^= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 3
	case 0x55: // EOR zp,X
		addr, _ := c.addrZeroPageX()
		c.A ^= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
	case 0x4D: // EOR abs
		addr, _ := c.addrAbsolute()
		c.A ^= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
	case 0x5D: // EOR abs,X
		addr, pageCrossed := c.addrAbsoluteX()
		c.A ^= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}
	case 0x59: // EOR abs,Y
		addr, pageCrossed := c.addrAbsoluteY()
		c.A ^= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}
	case 0x41: // EOR (ind,X)
		addr, _ := c.addrIndexedIndirect()
		c.A ^= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 6
	case 0x51: // EOR (ind),Y
		addr, pageCrossed := c.addrIndirectIndexed()
		c.A ^= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 5
		if pageCrossed {
			c.Cycles++
		}

	// Logical operations - ORA
	case 0x09: // ORA #imm
		addr, _ := c.addrImmediate()
		c.A |= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 2
	case 0x05: // ORA zp
		addr, _ := c.addrZeroPage()
		c.A |= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 3
	case 0x15: // ORA zp,X
		addr, _ := c.addrZeroPageX()
		c.A |= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
	case 0x0D: // ORA abs
		addr, _ := c.addrAbsolute()
		c.A |= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
	case 0x1D: // ORA abs,X
		addr, pageCrossed := c.addrAbsoluteX()
		c.A |= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}
	case 0x19: // ORA abs,Y
		addr, pageCrossed := c.addrAbsoluteY()
		c.A |= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}
	case 0x01: // ORA (ind,X)
		addr, _ := c.addrIndexedIndirect()
		c.A |= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 6
	case 0x11: // ORA (ind),Y
		addr, pageCrossed := c.addrIndirectIndexed()
		c.A |= c.Mem.Read(addr)
		c.updateZN(c.A)
		c.Cycles += 5
		if pageCrossed {
			c.Cycles++
		}

	// BIT - Test bits
	case 0x24: // BIT zp
		addr, _ := c.addrZeroPage()
		val := c.Mem.Read(addr)
		c.setFlag(FlagZ, (c.A&val) == 0)
		c.setFlag(FlagV, (val&0x40) != 0)
		c.setFlag(FlagN, (val&0x80) != 0)
		c.Cycles += 3
	case 0x2C: // BIT abs
		addr, _ := c.addrAbsolute()
		val := c.Mem.Read(addr)
		c.setFlag(FlagZ, (c.A&val) == 0)
		c.setFlag(FlagV, (val&0x40) != 0)
		c.setFlag(FlagN, (val&0x80) != 0)
		c.Cycles += 4

	// ADC - Add with Carry
	case 0x69: // ADC #imm
		addr, _ := c.addrImmediate()
		c.adc(c.Mem.Read(addr))
		c.Cycles += 2
	case 0x65: // ADC zp
		addr, _ := c.addrZeroPage()
		c.adc(c.Mem.Read(addr))
		c.Cycles += 3
	case 0x75: // ADC zp,X
		addr, _ := c.addrZeroPageX()
		c.adc(c.Mem.Read(addr))
		c.Cycles += 4
	case 0x6D: // ADC abs
		addr, _ := c.addrAbsolute()
		c.adc(c.Mem.Read(addr))
		c.Cycles += 4
	case 0x7D: // ADC abs,X
		addr, pageCrossed := c.addrAbsoluteX()
		c.adc(c.Mem.Read(addr))
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}
	case 0x79: // ADC abs,Y
		addr, pageCrossed := c.addrAbsoluteY()
		c.adc(c.Mem.Read(addr))
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}
	case 0x61: // ADC (ind,X)
		addr, _ := c.addrIndexedIndirect()
		c.adc(c.Mem.Read(addr))
		c.Cycles += 6
	case 0x71: // ADC (ind),Y
		addr, pageCrossed := c.addrIndirectIndexed()
		c.adc(c.Mem.Read(addr))
		c.Cycles += 5
		if pageCrossed {
			c.Cycles++
		}

	// SBC - Subtract with Carry
	case 0xE9: // SBC #imm
		addr, _ := c.addrImmediate()
		c.sbc(c.Mem.Read(addr))
		c.Cycles += 2
	case 0xE5: // SBC zp
		addr, _ := c.addrZeroPage()
		c.sbc(c.Mem.Read(addr))
		c.Cycles += 3
	case 0xF5: // SBC zp,X
		addr, _ := c.addrZeroPageX()
		c.sbc(c.Mem.Read(addr))
		c.Cycles += 4
	case 0xED: // SBC abs
		addr, _ := c.addrAbsolute()
		c.sbc(c.Mem.Read(addr))
		c.Cycles += 4
	case 0xFD: // SBC abs,X
		addr, pageCrossed := c.addrAbsoluteX()
		c.sbc(c.Mem.Read(addr))
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}
	case 0xF9: // SBC abs,Y
		addr, pageCrossed := c.addrAbsoluteY()
		c.sbc(c.Mem.Read(addr))
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}
	case 0xE1: // SBC (ind,X)
		addr, _ := c.addrIndexedIndirect()
		c.sbc(c.Mem.Read(addr))
		c.Cycles += 6
	case 0xF1: // SBC (ind),Y
		addr, pageCrossed := c.addrIndirectIndexed()
		c.sbc(c.Mem.Read(addr))
		c.Cycles += 5
		if pageCrossed {
			c.Cycles++
		}

	// CMP - Compare Accumulator
	case 0xC9: // CMP #imm
		addr, _ := c.addrImmediate()
		c.compare(c.A, c.Mem.Read(addr))
		c.Cycles += 2
	case 0xC5: // CMP zp
		addr, _ := c.addrZeroPage()
		c.compare(c.A, c.Mem.Read(addr))
		c.Cycles += 3
	case 0xD5: // CMP zp,X
		addr, _ := c.addrZeroPageX()
		c.compare(c.A, c.Mem.Read(addr))
		c.Cycles += 4
	case 0xCD: // CMP abs
		addr, _ := c.addrAbsolute()
		c.compare(c.A, c.Mem.Read(addr))
		c.Cycles += 4
	case 0xDD: // CMP abs,X
		addr, pageCrossed := c.addrAbsoluteX()
		c.compare(c.A, c.Mem.Read(addr))
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}
	case 0xD9: // CMP abs,Y
		addr, pageCrossed := c.addrAbsoluteY()
		c.compare(c.A, c.Mem.Read(addr))
		c.Cycles += 4
		if pageCrossed {
			c.Cycles++
		}
	case 0xC1: // CMP (ind,X)
		addr, _ := c.addrIndexedIndirect()
		c.compare(c.A, c.Mem.Read(addr))
		c.Cycles += 6
	case 0xD1: // CMP (ind),Y
		addr, pageCrossed := c.addrIndirectIndexed()
		c.compare(c.A, c.Mem.Read(addr))
		c.Cycles += 5
		if pageCrossed {
			c.Cycles++
		}

	// CPX - Compare X Register
	case 0xE0: // CPX #imm
		addr, _ := c.addrImmediate()
		c.compare(c.X, c.Mem.Read(addr))
		c.Cycles += 2
	case 0xE4: // CPX zp
		addr, _ := c.addrZeroPage()
		c.compare(c.X, c.Mem.Read(addr))
		c.Cycles += 3
	case 0xEC: // CPX abs
		addr, _ := c.addrAbsolute()
		c.compare(c.X, c.Mem.Read(addr))
		c.Cycles += 4

	// CPY - Compare Y Register
	case 0xC0: // CPY #imm
		addr, _ := c.addrImmediate()
		c.compare(c.Y, c.Mem.Read(addr))
		c.Cycles += 2
	case 0xC4: // CPY zp
		addr, _ := c.addrZeroPage()
		c.compare(c.Y, c.Mem.Read(addr))
		c.Cycles += 3
	case 0xCC: // CPY abs
		addr, _ := c.addrAbsolute()
		c.compare(c.Y, c.Mem.Read(addr))
		c.Cycles += 4

	// INC - Increment Memory
	case 0xE6: // INC zp
		addr, _ := c.addrZeroPage()
		val := c.Mem.Read(addr) + 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 5
	case 0xF6: // INC zp,X
		addr, _ := c.addrZeroPageX()
		val := c.Mem.Read(addr) + 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 6
	case 0xEE: // INC abs
		addr, _ := c.addrAbsolute()
		val := c.Mem.Read(addr) + 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 6
	case 0xFE: // INC abs,X
		addr, _ := c.addrAbsoluteX()
		val := c.Mem.Read(addr) + 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 7

	// INX, INY - Increment Registers
	case 0xE8: // INX
		c.X++
		c.updateZN(c.X)
		c.Cycles += 2
	case 0xC8: // INY
		c.Y++
		c.updateZN(c.Y)
		c.Cycles += 2

	// DEC - Decrement Memory
	case 0xC6: // DEC zp
		addr, _ := c.addrZeroPage()
		val := c.Mem.Read(addr) - 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 5
	case 0xD6: // DEC zp,X
		addr, _ := c.addrZeroPageX()
		val := c.Mem.Read(addr) - 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 6
	case 0xCE: // DEC abs
		addr, _ := c.addrAbsolute()
		val := c.Mem.Read(addr) - 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 6
	case 0xDE: // DEC abs,X
		addr, _ := c.addrAbsoluteX()
		val := c.Mem.Read(addr) - 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 7

	// DEX, DEY - Decrement Registers
	case 0xCA: // DEX
		c.X--
		c.updateZN(c.X)
		c.Cycles += 2
	case 0x88: // DEY
		c.Y--
		c.updateZN(c.Y)
		c.Cycles += 2

	// ASL - Arithmetic Shift Left
	case 0x0A: // ASL A
		c.setFlag(FlagC, (c.A&0x80) != 0)
		c.A <<= 1
		c.updateZN(c.A)
		c.Cycles += 2
	case 0x06: // ASL zp
		addr, _ := c.addrZeroPage()
		val := c.Mem.Read(addr)
		c.setFlag(FlagC, (val&0x80) != 0)
		val <<= 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 5
	case 0x16: // ASL zp,X
		addr, _ := c.addrZeroPageX()
		val := c.Mem.Read(addr)
		c.setFlag(FlagC, (val&0x80) != 0)
		val <<= 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 6
	case 0x0E: // ASL abs
		addr, _ := c.addrAbsolute()
		val := c.Mem.Read(addr)
		c.setFlag(FlagC, (val&0x80) != 0)
		val <<= 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 6
	case 0x1E: // ASL abs,X
		addr, _ := c.addrAbsoluteX()
		val := c.Mem.Read(addr)
		c.setFlag(FlagC, (val&0x80) != 0)
		val <<= 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 7

	// LSR - Logical Shift Right
	case 0x4A: // LSR A
		c.setFlag(FlagC, (c.A&0x01) != 0)
		c.A >>= 1
		c.updateZN(c.A)
		c.Cycles += 2
	case 0x46: // LSR zp
		addr, _ := c.addrZeroPage()
		val := c.Mem.Read(addr)
		c.setFlag(FlagC, (val&0x01) != 0)
		val >>= 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 5
	case 0x56: // LSR zp,X
		addr, _ := c.addrZeroPageX()
		val := c.Mem.Read(addr)
		c.setFlag(FlagC, (val&0x01) != 0)
		val >>= 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 6
	case 0x4E: // LSR abs
		addr, _ := c.addrAbsolute()
		val := c.Mem.Read(addr)
		c.setFlag(FlagC, (val&0x01) != 0)
		val >>= 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 6
	case 0x5E: // LSR abs,X
		addr, _ := c.addrAbsoluteX()
		val := c.Mem.Read(addr)
		c.setFlag(FlagC, (val&0x01) != 0)
		val >>= 1
		c.Mem.Write(addr, val)
		c.updateZN(val)
		c.Cycles += 7

	// ROL - Rotate Left
	case 0x2A: // ROL A
		carry := c.A & 0x80
		c.A = (c.A << 1) | (c.P & FlagC)
		c.setFlag(FlagC, carry != 0)
		c.updateZN(c.A)
		c.Cycles += 2
	case 0x26: // ROL zp
		addr, _ := c.addrZeroPage()
		val := c.Mem.Read(addr)
		carry := val & 0x80
		val = (val << 1) | (c.P & FlagC)
		c.Mem.Write(addr, val)
		c.setFlag(FlagC, carry != 0)
		c.updateZN(val)
		c.Cycles += 5
	case 0x36: // ROL zp,X
		addr, _ := c.addrZeroPageX()
		val := c.Mem.Read(addr)
		carry := val & 0x80
		val = (val << 1) | (c.P & FlagC)
		c.Mem.Write(addr, val)
		c.setFlag(FlagC, carry != 0)
		c.updateZN(val)
		c.Cycles += 6
	case 0x2E: // ROL abs
		addr, _ := c.addrAbsolute()
		val := c.Mem.Read(addr)
		carry := val & 0x80
		val = (val << 1) | (c.P & FlagC)
		c.Mem.Write(addr, val)
		c.setFlag(FlagC, carry != 0)
		c.updateZN(val)
		c.Cycles += 6
	case 0x3E: // ROL abs,X
		addr, _ := c.addrAbsoluteX()
		val := c.Mem.Read(addr)
		carry := val & 0x80
		val = (val << 1) | (c.P & FlagC)
		c.Mem.Write(addr, val)
		c.setFlag(FlagC, carry != 0)
		c.updateZN(val)
		c.Cycles += 7

	// ROR - Rotate Right
	case 0x6A: // ROR A
		carry := c.A & 0x01
		c.A = (c.A >> 1) | ((c.P & FlagC) << 7)
		c.setFlag(FlagC, carry != 0)
		c.updateZN(c.A)
		c.Cycles += 2
	case 0x66: // ROR zp
		addr, _ := c.addrZeroPage()
		val := c.Mem.Read(addr)
		carry := val & 0x01
		val = (val >> 1) | ((c.P & FlagC) << 7)
		c.Mem.Write(addr, val)
		c.setFlag(FlagC, carry != 0)
		c.updateZN(val)
		c.Cycles += 5
	case 0x76: // ROR zp,X
		addr, _ := c.addrZeroPageX()
		val := c.Mem.Read(addr)
		carry := val & 0x01
		val = (val >> 1) | ((c.P & FlagC) << 7)
		c.Mem.Write(addr, val)
		c.setFlag(FlagC, carry != 0)
		c.updateZN(val)
		c.Cycles += 6
	case 0x6E: // ROR abs
		addr, _ := c.addrAbsolute()
		val := c.Mem.Read(addr)
		carry := val & 0x01
		val = (val >> 1) | ((c.P & FlagC) << 7)
		c.Mem.Write(addr, val)
		c.setFlag(FlagC, carry != 0)
		c.updateZN(val)
		c.Cycles += 6
	case 0x7E: // ROR abs,X
		addr, _ := c.addrAbsoluteX()
		val := c.Mem.Read(addr)
		carry := val & 0x01
		val = (val >> 1) | ((c.P & FlagC) << 7)
		c.Mem.Write(addr, val)
		c.setFlag(FlagC, carry != 0)
		c.updateZN(val)
		c.Cycles += 7

	// JMP - Jump
	case 0x4C: // JMP abs
		c.PC, _ = c.addrAbsolute()
		c.Cycles += 3
	case 0x6C: // JMP ind
		c.PC, _ = c.addrIndirect()
		c.Cycles += 5

	// JSR - Jump to Subroutine
	case 0x20: // JSR abs
		addr, _ := c.addrAbsolute()
		c.push16(c.PC - 1) // Push return address - 1
		c.PC = addr
		c.Cycles += 6

	// RTS - Return from Subroutine
	case 0x60: // RTS
		c.PC = c.pop16() + 1
		c.Cycles += 6

	// Branch instructions
	case 0x10: // BPL - Branch if Plus (N=0)
		c.branch(!c.getFlag(FlagN))
	case 0x30: // BMI - Branch if Minus (N=1)
		c.branch(c.getFlag(FlagN))
	case 0x50: // BVC - Branch if Overflow Clear (V=0)
		c.branch(!c.getFlag(FlagV))
	case 0x70: // BVS - Branch if Overflow Set (V=1)
		c.branch(c.getFlag(FlagV))
	case 0x90: // BCC - Branch if Carry Clear (C=0)
		c.branch(!c.getFlag(FlagC))
	case 0xB0: // BCS - Branch if Carry Set (C=1)
		c.branch(c.getFlag(FlagC))
	case 0xD0: // BNE - Branch if Not Equal (Z=0)
		c.branch(!c.getFlag(FlagZ))
	case 0xF0: // BEQ - Branch if Equal (Z=1)
		c.branch(c.getFlag(FlagZ))

	// Flag operations
	case 0x18: // CLC - Clear Carry
		c.setFlag(FlagC, false)
		c.Cycles += 2
	case 0x38: // SEC - Set Carry
		c.setFlag(FlagC, true)
		c.Cycles += 2
	case 0x58: // CLI - Clear Interrupt Disable
		c.setFlag(FlagI, false)
		c.Cycles += 2
	case 0x78: // SEI - Set Interrupt Disable
		c.setFlag(FlagI, true)
		c.Cycles += 2
	case 0xB8: // CLV - Clear Overflow
		c.setFlag(FlagV, false)
		c.Cycles += 2
	case 0xD8: // CLD - Clear Decimal
		c.setFlag(FlagD, false)
		c.Cycles += 2
	case 0xF8: // SED - Set Decimal
		c.setFlag(FlagD, true)
		c.Cycles += 2

	// System
	case 0x00: // BRK - Break
		c.PC++ // BRK advances PC by 2
		c.push16(c.PC)
		c.push(c.P | FlagB | FlagU)
		c.setFlag(FlagI, true)
		c.PC = c.read16(IRQVector)
		c.Cycles += 7

	case 0x40: // RTI - Return from Interrupt
		c.P = (c.pop() &^ FlagB) | FlagU // B cleared, U always set
		c.PC = c.pop16()
		c.Cycles += 6

	case 0xEA: // NOP
		c.Cycles += 2

	default:
		// Unknown opcode - could implement illegal opcodes here
		c.Cycles += 2
	}
}

// ADC helper
func (c *CPU) adc(val byte) {
	carry := byte(0)
	if c.getFlag(FlagC) {
		carry = 1
	}

	sum := uint16(c.A) + uint16(val) + uint16(carry)
	c.setFlag(FlagC, sum > 0xFF)
	c.setFlag(FlagV, ((c.A^val)&0x80) == 0 && ((c.A^byte(sum))&0x80) != 0)
	c.A = byte(sum)
	c.updateZN(c.A)
}

// SBC helper
func (c *CPU) sbc(val byte) {
	c.adc(^val) // SBC is ADC with inverted operand
}

// Compare helper
func (c *CPU) compare(reg, val byte) {
	result := reg - val
	c.setFlag(FlagC, reg >= val)
	c.updateZN(result)
}

// Branch helper
func (c *CPU) branch(condition bool) {
	addr, pageCrossed := c.addrRelative()
	c.Cycles += 2
	if condition {
		c.PC = addr
		c.Cycles++ // Branch taken costs 1 extra cycle
		if pageCrossed {
			c.Cycles++ // Page boundary crossed costs another cycle
		}
	}
}
