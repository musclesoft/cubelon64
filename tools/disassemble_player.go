// Disassemble the ZX music player to understand channel processing
package main

import (
	"fmt"
	"os"
)

func main() {
	// Load player code
	mem, err := os.ReadFile("../emulator/memory_frame300.bin")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Music player at $7EC5-$7F9E
	start := 0x7EC5 - 0x4000
	end := 0x7F9E - 0x4000

	fmt.Println("=== ZX Music Player Disassembly ($7EC5-$7F9E) ===\n")

	pc := start
	addr := 0x7EC5

	for pc < end {
		op := mem[pc]
		fmt.Printf("$%04X: %02X ", addr, op)

		switch op {
		case 0xC9:
			fmt.Println("RET")
		case 0xFE:
			fmt.Printf("CP $%02X\n", mem[pc+1])
			pc++
		case 0xA3:
			fmt.Println("AND E")
		case 0x30:
			fmt.Printf("JR NC, $%02X (→ $%04X)\n", mem[pc+1], addr+2+int(int8(mem[pc+1])))
			pc++
		case 0x24:
			fmt.Println("INC H")
		case 0x7A:
			fmt.Println("LD A,D")
		case 0xE6:
			fmt.Printf("AND $%02X\n", mem[pc+1])
			pc++
		case 0x20:
			fmt.Printf("JR NZ, $%02X (→ $%04X)\n", mem[pc+1], addr+2+int(int8(mem[pc+1])))
			pc++
		case 0x32:
			fmt.Printf("LD ($%04X),A\n", uint16(mem[pc+1])|uint16(mem[pc+2])<<8)
			pc += 2
		case 0x7B:
			fmt.Println("LD A,E")
		case 0xCD:
			fmt.Printf("CALL $%04X\n", uint16(mem[pc+1])|uint16(mem[pc+2])<<8)
			pc += 2
		case 0xD5:
			fmt.Println("PUSH DE")
		case 0x3E:
			fmt.Printf("LD A,$%02X\n", mem[pc+1])
			pc++
		case 0xD1:
			fmt.Println("POP DE")
		case 0xF6:
			fmt.Printf("OR $%02X\n", mem[pc+1])
			pc++
		case 0x2A:
			fmt.Printf("LD HL,($%04X)\n", uint16(mem[pc+1])|uint16(mem[pc+2])<<8)
			pc += 2
		case 0x23:
			fmt.Println("INC HL")
		case 0x22:
			fmt.Printf("LD ($%04X),HL\n", uint16(mem[pc+1])|uint16(mem[pc+2])<<8)
			pc += 2
		case 0x47:
			fmt.Println("LD B,A")
		case 0x3A:
			fmt.Printf("LD A,($%04X)\n", uint16(mem[pc+1])|uint16(mem[pc+2])<<8)
			pc += 2
		case 0x78:
			fmt.Println("LD A,B")
		case 0x28:
			fmt.Printf("JR Z, $%02X (→ $%04X)\n", mem[pc+1], addr+2+int(int8(mem[pc+1])))
			pc++
		case 0x25:
			fmt.Println("DEC H")
		case 0xED:
			op2 := mem[pc+1]
			pc++
			switch op2 {
			case 0x5B:
				fmt.Printf("LD DE,($%04X)\n", uint16(mem[pc+1])|uint16(mem[pc+2])<<8)
				pc += 2
			case 0x4B:
				fmt.Printf("LD BC,($%04X)\n", uint16(mem[pc+1])|uint16(mem[pc+2])<<8)
				pc += 2
			case 0x42:
				fmt.Println("SBC HL,BC")
			case 0xB8:
				fmt.Println("LDDR")
			case 0x53:
				fmt.Printf("LD ($%04X),DE\n", uint16(mem[pc+1])|uint16(mem[pc+2])<<8)
				pc += 2
			case 0x72:
				fmt.Println("SBC HL,SP")
			default:
				fmt.Printf("ED %02X (unknown)\n", op2)
			}
		case 0x7C:
			fmt.Println("LD A,H")
		case 0xBA:
			fmt.Println("CP D")
		case 0x7D:
			fmt.Println("LD A,L")
		case 0xBB:
			fmt.Println("CP E")
		case 0xA7:
			fmt.Println("AND A")
		case 0xF9:
			fmt.Println("LD SP,HL")
		case 0x37:
			fmt.Println("SCF")
		case 0x18:
			fmt.Printf("JR $%02X (→ $%04X)\n", mem[pc+1], addr+2+int(int8(mem[pc+1])))
			pc++
		case 0x3F:
			fmt.Println("CCF")
		case 0x13:
			fmt.Println("INC DE")
		case 0xF5:
			fmt.Println("PUSH AF")
		case 0x01:
			fmt.Printf("LD BC,$%04X\n", uint16(mem[pc+1])|uint16(mem[pc+2])<<8)
			pc += 2
		case 0xE5:
			fmt.Println("PUSH HL")
		case 0xEF:
			fmt.Printf("RST $%02X\n", 0x28)
		case 0x16:
			fmt.Printf("LD D,$%02X\n", mem[pc+1])
			pc++
		case 0xF1:
			fmt.Println("POP AF")
		case 0x12:
			fmt.Println("LD (DE),A")
		case 0x45:
			fmt.Println("LD B,L")
		case 0x1F:
			fmt.Println("RRA")
		case 0x09:
			fmt.Println("ADD HL,BC")
		case 0x38:
			fmt.Printf("JR C, $%02X (→ $%04X)\n", mem[pc+1], addr+2+int(int8(mem[pc+1])))
			pc++
		case 0xD8:
			fmt.Println("RET C")
		case 0x21:
			fmt.Printf("LD HL,$%04X\n", uint16(mem[pc+1])|uint16(mem[pc+2])<<8)
			pc += 2
		case 0x19:
			fmt.Println("ADD HL,DE")
		case 0xC3:
			fmt.Printf("JP $%04X\n", uint16(mem[pc+1])|uint16(mem[pc+2])<<8)
			pc += 2
		case 0x2E:
			fmt.Printf("LD L,$%02X\n", mem[pc+1])
			pc++
		case 0x06:
			fmt.Printf("LD B,$%02X\n", mem[pc+1])
			pc++
		case 0x11:
			fmt.Printf("LD DE,$%04X\n", uint16(mem[pc+1])|uint16(mem[pc+2])<<8)
			pc += 2
		case 0x35:
			fmt.Println("DEC (HL)")
		case 0x3B:
			fmt.Println("DEC SP")
		case 0xD0:
			fmt.Println("RET NC")
		case 0x04:
			fmt.Println("INC B")
		case 0x7E:
			fmt.Println("LD A,(HL)")
		case 0xEB:
			fmt.Println("EX DE,HL")
		default:
			fmt.Printf("?? (unknown opcode)\n")
		}

		pc++
		addr++
	}
}
