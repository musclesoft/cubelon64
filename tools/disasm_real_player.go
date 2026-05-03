// Disassemble actual music player from correct memory location
package main

import (
	"fmt"
	"os"
)

func main() {
	mem, _ := os.ReadFile("../emulator/memory_frame300.bin")

	fmt.Println("=== ZX Music Player ($7EC5) ===\n")

	// Key addresses from trace
	fmt.Println("Entry: $7EC5 (called from IRQ at $7E0E)")
	fmt.Println("Tick counter: $7EC4")
	fmt.Println()

	// Disassemble first 100 bytes to understand structure
	pc := 0x7EC5
	end := pc + 150

	for pc < end {
		op := mem[pc]
		fmt.Printf("$%04X: %02X ", pc, op)

		switch op {
		case 0x21:
			val := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
			fmt.Printf("LD HL,$%04X\n", val)
			pc += 2
		case 0x35:
			fmt.Println("DEC (HL)")
		case 0xC0:
			fmt.Println("RET NZ")
		case 0x3A:
			val := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
			fmt.Printf("LD A,($%04X)\n", val)
			pc += 2
		case 0x4F:
			fmt.Println("LD C,A")
		case 0x86:
			fmt.Println("ADD A,(HL)")
		case 0xB1:
			fmt.Println("OR C")
		case 0x01:
			val := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
			fmt.Printf("LD BC,$%04X\n", val)
			pc += 2
		case 0xED:
			op2 := mem[pc+1]
			pc++
			switch op2 {
			case 0x79:
				fmt.Println("OUT (C),A")
			case 0x5B:
				val := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
				fmt.Printf("LD DE,($%04X)\n", val)
				pc += 2
			case 0x78:
				fmt.Println("IN A,(C)")
			default:
				fmt.Printf("ED %02X\n", op2)
			}
		case 0x23:
			fmt.Println("INC HL")
		case 0x7E:
			fmt.Println("LD A,(HL)")
		case 0xFE:
			fmt.Printf("CP $%02X\n", mem[pc+1])
			pc++
		case 0x20:
			ofs := int8(mem[pc+1])
			fmt.Printf("JR NZ,$%02X (→ $%04X)\n", mem[pc+1], pc+2+int(ofs))
			pc++
		case 0x28:
			ofs := int8(mem[pc+1])
			fmt.Printf("JR Z,$%02X (→ $%04X)\n", mem[pc+1], pc+2+int(ofs))
			pc++
		case 0xEE:
			fmt.Printf("XOR $%02X\n", mem[pc+1])
			pc++
		case 0x77:
			fmt.Println("LD (HL),A")
		case 0x06:
			fmt.Printf("LD B,$%02X\n", mem[pc+1])
			pc++
		case 0xC5:
			fmt.Println("PUSH BC")
		case 0x11:
			val := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
			fmt.Printf("LD DE,$%04X\n", val)
			pc += 2
		case 0xCD:
			val := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
			fmt.Printf("CALL $%04X\n", val)
			pc += 2
		case 0x1A:
			fmt.Println("LD A,(DE)")
		case 0x13:
			fmt.Println("INC DE")
		case 0xF5:
			fmt.Println("PUSH AF")
		case 0x12:
			fmt.Println("LD (DE),A")
		case 0xAF:
			fmt.Println("XOR A")
		case 0xF1:
			fmt.Println("POP AF")
		case 0xE6:
			fmt.Printf("AND $%02X\n", mem[pc+1])
			pc++
		case 0xCA:
			val := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
			fmt.Printf("JP Z,$%04X\n", val)
			pc += 2
		case 0x32:
			val := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
			fmt.Printf("LD ($%04X),A\n", val)
			pc += 2
		case 0x0F:
			fmt.Println("RRCA")
		case 0x47:
			fmt.Println("LD B,A")
		case 0xE5:
			fmt.Println("PUSH HL")
		case 0x19:
			fmt.Println("ADD HL,DE")
		case 0x5E:
			fmt.Println("LD E,(HL)")
		case 0x56:
			fmt.Println("LD D,(HL)")
		case 0xE1:
			fmt.Println("POP HL")
		case 0xEB:
			fmt.Println("EX DE,HL")
		case 0xC1:
			fmt.Println("POP BC")
		case 0x10:
			ofs := int8(mem[pc+1])
			fmt.Printf("DJNZ $%02X (→ $%04X)\n", mem[pc+1], pc+2+int(ofs))
			pc++
		case 0xC9:
			fmt.Println("RET")
		case 0x2A:
			val := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
			fmt.Printf("LD HL,($%04X)\n", val)
			pc += 2
		case 0x7B:
			fmt.Println("LD A,E")
		case 0x7A:
			fmt.Println("LD A,D")
		case 0x18:
			ofs := int8(mem[pc+1])
			fmt.Printf("JR $%02X (→ $%04X)\n", mem[pc+1], pc+2+int(ofs))
			pc++
		case 0x22:
			val := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
			fmt.Printf("LD ($%04X),HL\n", val)
			pc += 2
		default:
			fmt.Printf("??\n")
		}
		pc++
	}

	fmt.Println("\n=== Key Observations ===")
	fmt.Println("$7EC5: LD HL,$7EC4 - loads tick counter address")
	fmt.Println("$7EC8: DEC (HL) - decrements tick counter")
	fmt.Println("$7EC9: RET NZ - returns if ticks > 0")
	fmt.Println("$7ECA: LD A,($7D35) - loads sequence pointer")
	fmt.Println()
	fmt.Println("Pattern loading starts after tick counter expires")
}
