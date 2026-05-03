// Simple Z80 disassembler for music player analysis
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: disasm_z80 <memory.bin> <start_addr_hex> <length>")
		fmt.Println("Example: disasm_z80 memory_frame300.bin 7EC5 200")
		return
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	var startAddr int
	fmt.Sscanf(os.Args[2], "%x", &startAddr)

	var length int
	fmt.Sscanf(os.Args[3], "%d", &length)

	fmt.Printf("Disassembling from $%04X:\n\n", startAddr)

	pc := startAddr
	end := startAddr + length

	for pc < end && pc < len(data) {
		opcode := data[pc]
		addr := pc
		line := fmt.Sprintf("%04X  ", addr)

		bytes := ""
		mnemonic := ""

		switch opcode {
		case 0x00:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "NOP"
			pc++

		case 0x01:
			bytes = fmt.Sprintf("%02X %02X %02X", opcode, data[pc+1], data[pc+2])
			mnemonic = fmt.Sprintf("LD BC,$%04X", uint16(data[pc+1])|uint16(data[pc+2])<<8)
			pc += 3

		case 0x0F:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "RRCA"
			pc++

		case 0x18:
			bytes = fmt.Sprintf("%02X %02X", opcode, data[pc+1])
			offset := int8(data[pc+1])
			target := pc + 2 + int(offset)
			mnemonic = fmt.Sprintf("JR $%04X", target)
			pc += 2

		case 0x20:
			bytes = fmt.Sprintf("%02X %02X", opcode, data[pc+1])
			offset := int8(data[pc+1])
			target := pc + 2 + int(offset)
			mnemonic = fmt.Sprintf("JR NZ,$%04X", target)
			pc += 2

		case 0x21:
			bytes = fmt.Sprintf("%02X %02X %02X", opcode, data[pc+1], data[pc+2])
			mnemonic = fmt.Sprintf("LD HL,$%04X", uint16(data[pc+1])|uint16(data[pc+2])<<8)
			pc += 3

		case 0x22:
			bytes = fmt.Sprintf("%02X %02X %02X", opcode, data[pc+1], data[pc+2])
			mnemonic = fmt.Sprintf("LD ($%04X),HL", uint16(data[pc+1])|uint16(data[pc+2])<<8)
			pc += 3

		case 0x23:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "INC HL"
			pc++

		case 0x28:
			bytes = fmt.Sprintf("%02X %02X", opcode, data[pc+1])
			offset := int8(data[pc+1])
			target := pc + 2 + int(offset)
			mnemonic = fmt.Sprintf("JR Z,$%04X", target)
			pc += 2

		case 0x2A:
			bytes = fmt.Sprintf("%02X %02X %02X", opcode, data[pc+1], data[pc+2])
			mnemonic = fmt.Sprintf("LD HL,($%04X)", uint16(data[pc+1])|uint16(data[pc+2])<<8)
			pc += 3

		case 0x2C:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "INC L"
			pc++

		case 0x31:
			bytes = fmt.Sprintf("%02X %02X %02X", opcode, data[pc+1], data[pc+2])
			mnemonic = fmt.Sprintf("LD SP,$%04X", uint16(data[pc+1])|uint16(data[pc+2])<<8)
			pc += 3

		case 0x32:
			bytes = fmt.Sprintf("%02X %02X %02X", opcode, data[pc+1], data[pc+2])
			mnemonic = fmt.Sprintf("LD ($%04X),A", uint16(data[pc+1])|uint16(data[pc+2])<<8)
			pc += 3

		case 0x35:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "DEC (HL)"
			pc++

		case 0x3A:
			bytes = fmt.Sprintf("%02X %02X %02X", opcode, data[pc+1], data[pc+2])
			mnemonic = fmt.Sprintf("LD A,($%04X)", uint16(data[pc+1])|uint16(data[pc+2])<<8)
			pc += 3

		case 0x46:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "LD B,(HL)"
			pc++

		case 0x4E:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "LD C,(HL)"
			pc++

		case 0x4F:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "LD C,A"
			pc++

		case 0x56:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "LD D,(HL)"
			pc++

		case 0x5E:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "LD E,(HL)"
			pc++

		case 0x67:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "LD H,A"
			pc++

		case 0x6F:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "LD L,A"
			pc++

		case 0x76:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "HALT"
			pc++

		case 0x7D:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "LD A,L"
			pc++

		case 0x7E:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "LD A,(HL)"
			pc++

		case 0xB1:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "OR C"
			pc++

		case 0xB7:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "OR A"
			pc++

		case 0xC0:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "RET NZ"
			pc++

		case 0xC3:
			bytes = fmt.Sprintf("%02X %02X %02X", opcode, data[pc+1], data[pc+2])
			mnemonic = fmt.Sprintf("JP $%04X", uint16(data[pc+1])|uint16(data[pc+2])<<8)
			pc += 3

		case 0xC6:
			bytes = fmt.Sprintf("%02X %02X", opcode, data[pc+1])
			mnemonic = fmt.Sprintf("ADD A,$%02X", data[pc+1])
			pc += 2

		case 0xC9:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "RET"
			pc++

		case 0xCB:
			bytes = fmt.Sprintf("%02X %02X", opcode, data[pc+1])
			cb := data[pc+1]
			switch cb {
			case 0x6F:
				mnemonic = "BIT 5,A"
			case 0x77:
				mnemonic = "BIT 6,(HL)"
			case 0x7F:
				mnemonic = "BIT 7,A"
			default:
				mnemonic = fmt.Sprintf("CB %02X", cb)
			}
			pc += 2

		case 0xCD:
			bytes = fmt.Sprintf("%02X %02X %02X", opcode, data[pc+1], data[pc+2])
			mnemonic = fmt.Sprintf("CALL $%04X", uint16(data[pc+1])|uint16(data[pc+2])<<8)
			pc += 3

		case 0xE1:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "POP HL"
			pc++

		case 0xE5:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "PUSH HL"
			pc++

		case 0xE6:
			bytes = fmt.Sprintf("%02X %02X", opcode, data[pc+1])
			mnemonic = fmt.Sprintf("AND $%02X", data[pc+1])
			pc += 2

		case 0xE9:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "JP (HL)"
			pc++

		case 0xEB:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "EX DE,HL"
			pc++

		case 0xED:
			bytes = fmt.Sprintf("%02X %02X", opcode, data[pc+1])
			ed := data[pc+1]
			switch ed {
			case 0x42:
				mnemonic = "SBC HL,BC"
			case 0x73:
				bytes = fmt.Sprintf("%02X %02X %02X %02X", opcode, ed, data[pc+2], data[pc+3])
				mnemonic = fmt.Sprintf("LD ($%04X),SP", uint16(data[pc+2])|uint16(data[pc+3])<<8)
				pc += 2
			case 0x79:
				mnemonic = "OUT (C),A"
			default:
				mnemonic = fmt.Sprintf("ED %02X", ed)
			}
			pc += 2

		case 0xEE:
			bytes = fmt.Sprintf("%02X %02X", opcode, data[pc+1])
			mnemonic = fmt.Sprintf("XOR $%02X", data[pc+1])
			pc += 2

		case 0xF9:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = "LD SP,HL"
			pc++

		case 0xFA:
			bytes = fmt.Sprintf("%02X %02X %02X", opcode, data[pc+1], data[pc+2])
			mnemonic = fmt.Sprintf("JP M,$%04X", uint16(data[pc+1])|uint16(data[pc+2])<<8)
			pc += 3

		case 0xFE:
			bytes = fmt.Sprintf("%02X %02X", opcode, data[pc+1])
			mnemonic = fmt.Sprintf("CP $%02X", data[pc+1])
			pc += 2

		default:
			bytes = fmt.Sprintf("%02X", opcode)
			mnemonic = fmt.Sprintf("??? (%02X)", opcode)
			pc++
		}

		fmt.Printf("%s%-12s %s\n", line, bytes, mnemonic)
	}
}
