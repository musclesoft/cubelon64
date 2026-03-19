package main

import (
	"fmt"
	"strings"
)

// 6502 opcode table
type Opcode struct {
	Code uint8
	Mode AddressingMode
	Size int
}

var opcodeTable = map[string][]Opcode{
	// ADC - Add with Carry
	"ADC": {
		{0x69, Immediate, 2},
		{0x65, ZeroPage, 2},
		{0x75, ZeroPageX, 2},
		{0x6D, Absolute, 3},
		{0x7D, AbsoluteX, 3},
		{0x79, AbsoluteY, 3},
		{0x61, IndirectX, 2},
		{0x71, IndirectY, 2},
	},
	// AND - Logical AND
	"AND": {
		{0x29, Immediate, 2},
		{0x25, ZeroPage, 2},
		{0x35, ZeroPageX, 2},
		{0x2D, Absolute, 3},
		{0x3D, AbsoluteX, 3},
		{0x39, AbsoluteY, 3},
		{0x21, IndirectX, 2},
		{0x31, IndirectY, 2},
	},
	// ASL - Arithmetic Shift Left
	"ASL": {
		{0x0A, Accumulator, 1},
		{0x06, ZeroPage, 2},
		{0x16, ZeroPageX, 2},
		{0x0E, Absolute, 3},
		{0x1E, AbsoluteX, 3},
	},
	// Branch instructions
	"BCC": {{0x90, Relative, 2}},
	"BCS": {{0xB0, Relative, 2}},
	"BEQ": {{0xF0, Relative, 2}},
	"BMI": {{0x30, Relative, 2}},
	"BNE": {{0xD0, Relative, 2}},
	"BPL": {{0x10, Relative, 2}},
	"BVC": {{0x50, Relative, 2}},
	"BVS": {{0x70, Relative, 2}},
	// BIT - Bit Test
	"BIT": {
		{0x24, ZeroPage, 2},
		{0x2C, Absolute, 3},
	},
	// BRK - Force Interrupt
	"BRK": {{0x00, Implied, 1}},
	// Clear flags
	"CLC": {{0x18, Implied, 1}},
	"CLD": {{0xD8, Implied, 1}},
	"CLI": {{0x58, Implied, 1}},
	"CLV": {{0xB8, Implied, 1}},
	// CMP - Compare Accumulator
	"CMP": {
		{0xC9, Immediate, 2},
		{0xC5, ZeroPage, 2},
		{0xD5, ZeroPageX, 2},
		{0xCD, Absolute, 3},
		{0xDD, AbsoluteX, 3},
		{0xD9, AbsoluteY, 3},
		{0xC1, IndirectX, 2},
		{0xD1, IndirectY, 2},
	},
	// CPX - Compare X Register
	"CPX": {
		{0xE0, Immediate, 2},
		{0xE4, ZeroPage, 2},
		{0xEC, Absolute, 3},
	},
	// CPY - Compare Y Register
	"CPY": {
		{0xC0, Immediate, 2},
		{0xC4, ZeroPage, 2},
		{0xCC, Absolute, 3},
	},
	// DEC - Decrement Memory
	"DEC": {
		{0xC6, ZeroPage, 2},
		{0xD6, ZeroPageX, 2},
		{0xCE, Absolute, 3},
		{0xDE, AbsoluteX, 3},
	},
	// DEX - Decrement X
	"DEX": {{0xCA, Implied, 1}},
	// DEY - Decrement Y
	"DEY": {{0x88, Implied, 1}},
	// EOR - Exclusive OR
	"EOR": {
		{0x49, Immediate, 2},
		{0x45, ZeroPage, 2},
		{0x55, ZeroPageX, 2},
		{0x4D, Absolute, 3},
		{0x5D, AbsoluteX, 3},
		{0x59, AbsoluteY, 3},
		{0x41, IndirectX, 2},
		{0x51, IndirectY, 2},
	},
	// INC - Increment Memory
	"INC": {
		{0xE6, ZeroPage, 2},
		{0xF6, ZeroPageX, 2},
		{0xEE, Absolute, 3},
		{0xFE, AbsoluteX, 3},
	},
	// INX - Increment X
	"INX": {{0xE8, Implied, 1}},
	// INY - Increment Y
	"INY": {{0xC8, Implied, 1}},
	// JMP - Jump
	"JMP": {
		{0x4C, Absolute, 3},
		{0x6C, Indirect, 3},
	},
	// JSR - Jump to Subroutine
	"JSR": {{0x20, Absolute, 3}},
	// LDA - Load Accumulator
	"LDA": {
		{0xA9, Immediate, 2},
		{0xA5, ZeroPage, 2},
		{0xB5, ZeroPageX, 2},
		{0xAD, Absolute, 3},
		{0xBD, AbsoluteX, 3},
		{0xB9, AbsoluteY, 3},
		{0xA1, IndirectX, 2},
		{0xB1, IndirectY, 2},
	},
	// LDX - Load X Register
	"LDX": {
		{0xA2, Immediate, 2},
		{0xA6, ZeroPage, 2},
		{0xB6, ZeroPageY, 2},
		{0xAE, Absolute, 3},
		{0xBE, AbsoluteY, 3},
	},
	// LDY - Load Y Register
	"LDY": {
		{0xA0, Immediate, 2},
		{0xA4, ZeroPage, 2},
		{0xB4, ZeroPageX, 2},
		{0xAC, Absolute, 3},
		{0xBC, AbsoluteX, 3},
	},
	// LSR - Logical Shift Right
	"LSR": {
		{0x4A, Accumulator, 1},
		{0x46, ZeroPage, 2},
		{0x56, ZeroPageX, 2},
		{0x4E, Absolute, 3},
		{0x5E, AbsoluteX, 3},
	},
	// NOP - No Operation
	"NOP": {{0xEA, Implied, 1}},
	// ORA - Logical OR
	"ORA": {
		{0x09, Immediate, 2},
		{0x05, ZeroPage, 2},
		{0x15, ZeroPageX, 2},
		{0x0D, Absolute, 3},
		{0x1D, AbsoluteX, 3},
		{0x19, AbsoluteY, 3},
		{0x01, IndirectX, 2},
		{0x11, IndirectY, 2},
	},
	// Stack operations
	"PHA": {{0x48, Implied, 1}},
	"PHP": {{0x08, Implied, 1}},
	"PLA": {{0x68, Implied, 1}},
	"PLP": {{0x28, Implied, 1}},
	// ROL - Rotate Left
	"ROL": {
		{0x2A, Accumulator, 1},
		{0x26, ZeroPage, 2},
		{0x36, ZeroPageX, 2},
		{0x2E, Absolute, 3},
		{0x3E, AbsoluteX, 3},
	},
	// ROR - Rotate Right
	"ROR": {
		{0x6A, Accumulator, 1},
		{0x66, ZeroPage, 2},
		{0x76, ZeroPageX, 2},
		{0x6E, Absolute, 3},
		{0x7E, AbsoluteX, 3},
	},
	// RTI - Return from Interrupt
	"RTI": {{0x40, Implied, 1}},
	// RTS - Return from Subroutine
	"RTS": {{0x60, Implied, 1}},
	// SBC - Subtract with Carry
	"SBC": {
		{0xE9, Immediate, 2},
		{0xE5, ZeroPage, 2},
		{0xF5, ZeroPageX, 2},
		{0xED, Absolute, 3},
		{0xFD, AbsoluteX, 3},
		{0xF9, AbsoluteY, 3},
		{0xE1, IndirectX, 2},
		{0xF1, IndirectY, 2},
	},
	// Set flags
	"SEC": {{0x38, Implied, 1}},
	"SED": {{0xF8, Implied, 1}},
	"SEI": {{0x78, Implied, 1}},
	// STA - Store Accumulator
	"STA": {
		{0x85, ZeroPage, 2},
		{0x95, ZeroPageX, 2},
		{0x8D, Absolute, 3},
		{0x9D, AbsoluteX, 3},
		{0x99, AbsoluteY, 3},
		{0x81, IndirectX, 2},
		{0x91, IndirectY, 2},
	},
	// STX - Store X Register
	"STX": {
		{0x86, ZeroPage, 2},
		{0x96, ZeroPageY, 2},
		{0x8E, Absolute, 3},
	},
	// STY - Store Y Register
	"STY": {
		{0x84, ZeroPage, 2},
		{0x94, ZeroPageX, 2},
		{0x8C, Absolute, 3},
	},
	// Register transfers
	"TAX": {{0xAA, Implied, 1}},
	"TAY": {{0xA8, Implied, 1}},
	"TSX": {{0xBA, Implied, 1}},
	"TXA": {{0x8A, Implied, 1}},
	"TXS": {{0x9A, Implied, 1}},
	"TYA": {{0x98, Implied, 1}},
}

func (a *Assembler) instructionSize(inst *InstructionStmt) (int, error) {
	opcodes, ok := opcodeTable[inst.Mnemonic]
	if !ok {
		return 0, fmt.Errorf("unknown instruction: %s", inst.Mnemonic)
	}

	// Determine addressing mode
	mode := inst.AddressMode

	// Branch instructions always use relative addressing
	isBranch := inst.Mnemonic == "BCC" || inst.Mnemonic == "BCS" ||
		inst.Mnemonic == "BEQ" || inst.Mnemonic == "BMI" ||
		inst.Mnemonic == "BNE" || inst.Mnemonic == "BPL" ||
		inst.Mnemonic == "BVC" || inst.Mnemonic == "BVS"
	if isBranch {
		return 2, nil // opcode + relative offset
	}

	// For Absolute/AbsoluteX/AbsoluteY, check if operand fits in zero page
	if mode == Absolute || mode == AbsoluteX || mode == AbsoluteY {
		if inst.Operand != "" {
			val, err := a.evalExpr(inst.Operand)
			if err == nil && val >= 0 && val <= 0xFF {
				// Can use zero page
				if mode == Absolute {
					mode = ZeroPage
				} else if mode == AbsoluteX {
					mode = ZeroPageX
				} else if mode == AbsoluteY {
					mode = ZeroPageY
				}
			}
		}
	}

	// Find matching opcode
	for _, op := range opcodes {
		if op.Mode == mode {
			return op.Size, nil
		}
	}

	return 0, fmt.Errorf("unsupported addressing mode for %s", inst.Mnemonic)
}

func (a *Assembler) emitInstruction(inst *InstructionStmt) error {
	opcodes, ok := opcodeTable[inst.Mnemonic]
	if !ok {
		return fmt.Errorf("unknown instruction: %s", inst.Mnemonic)
	}

	// Determine addressing mode and resolve operand
	mode := inst.AddressMode
	operandVal := 0
	var err error

	if inst.Operand != "" {
		// Strip immediate mode prefix (#, #<, #>) before evaluating
		operand := inst.Operand
		if strings.HasPrefix(operand, "#<") || strings.HasPrefix(operand, "#>") {
			// Keep the operator for evalExpr to handle
		} else if strings.HasPrefix(operand, "#") {
			operand = operand[1:] // Strip just the #
		}
		operandVal, err = a.evalExpr(operand)
		if err != nil {
			return err
		}
	}

	// For branch instructions, calculate relative offset
	isBranch := mode == Relative || inst.Mnemonic == "BCC" || inst.Mnemonic == "BCS" ||
		inst.Mnemonic == "BEQ" || inst.Mnemonic == "BMI" || inst.Mnemonic == "BNE" ||
		inst.Mnemonic == "BPL" || inst.Mnemonic == "BVC" || inst.Mnemonic == "BVS"

	if isBranch {
		mode = Relative
		// Calculate offset: target - (PC + 2)
		currentPC := a.currentSeg.Start + len(a.currentSeg.Data)
		offset := operandVal - (currentPC + 2)

		if offset < -128 || offset > 127 {
			return fmt.Errorf("branch %s %s offset out of range: %d bytes (PC=$%04X, target=$%04X). Consider using JMP instead",
				inst.Mnemonic, inst.Operand, offset, currentPC, operandVal)
		}
		operandVal = offset & 0xFF
	}

	// For Absolute/AbsoluteX/AbsoluteY, check if operand fits in zero page
	if mode == Absolute || mode == AbsoluteX || mode == AbsoluteY {
		if operandVal >= 0 && operandVal <= 0xFF {
			// Check if zero page mode is available
			zpMode := ZeroPage
			if mode == AbsoluteX {
				zpMode = ZeroPageX
			} else if mode == AbsoluteY {
				zpMode = ZeroPageY
			}

			// Check if instruction supports zero page mode
			for _, op := range opcodes {
				if op.Mode == zpMode {
					mode = zpMode
					break
				}
			}
		}
	}

	// Find matching opcode
	var selectedOp *Opcode
	for i := range opcodes {
		if opcodes[i].Mode == mode {
			selectedOp = &opcodes[i]
			break
		}
	}

	if selectedOp == nil {
		return fmt.Errorf("unsupported addressing mode %d for %s", mode, inst.Mnemonic)
	}

	// Emit opcode
	a.emitByte(selectedOp.Code)

	// Emit operand bytes
	switch selectedOp.Size {
	case 1:
		// No operand
	case 2:
		// 8-bit operand
		a.emitByte(uint8(operandVal))
	case 3:
		// 16-bit operand (little-endian)
		a.emitByte(uint8(operandVal))
		a.emitByte(uint8(operandVal >> 8))
	}

	return nil
}
