package main

// cpu6502Cycles is the base cycle count of every opcode CPU6502 implements
// (0 for opcodes it does not). Extra cycles for taken branches and for page
// crossings are added by CPU6502 itself.
var cpu6502Cycles = [256]uint8{
	7, 6, 0, 0, 0, 3, 5, 0, 3, 2, 2, 0, 0, 4, 6, 0,
	2, 5, 0, 0, 0, 4, 6, 0, 2, 4, 0, 0, 0, 4, 7, 0,
	6, 6, 0, 0, 3, 3, 5, 0, 4, 2, 2, 0, 4, 4, 6, 0,
	2, 5, 0, 0, 0, 4, 6, 0, 2, 4, 0, 0, 0, 4, 7, 0,
	6, 6, 0, 0, 0, 3, 5, 0, 3, 2, 2, 0, 3, 4, 6, 0,
	2, 5, 0, 0, 0, 4, 6, 0, 2, 4, 0, 0, 0, 4, 7, 0,
	6, 6, 0, 0, 0, 3, 5, 0, 4, 2, 2, 0, 5, 4, 6, 0,
	2, 5, 0, 0, 0, 4, 6, 0, 2, 4, 0, 0, 0, 4, 7, 0,
	0, 6, 0, 0, 3, 3, 3, 0, 2, 0, 2, 0, 4, 4, 4, 0,
	2, 6, 0, 0, 4, 4, 4, 0, 2, 5, 2, 0, 0, 5, 0, 0,
	2, 6, 2, 0, 3, 3, 3, 0, 2, 2, 2, 0, 4, 4, 4, 0,
	2, 5, 0, 0, 4, 4, 4, 0, 2, 4, 2, 0, 4, 4, 4, 0,
	2, 6, 0, 0, 3, 3, 5, 0, 2, 2, 2, 0, 4, 4, 6, 0,
	2, 5, 0, 0, 0, 4, 6, 0, 2, 4, 0, 0, 0, 4, 7, 0,
	2, 6, 0, 0, 3, 3, 5, 0, 2, 2, 2, 0, 4, 4, 6, 0,
	2, 5, 0, 0, 0, 4, 6, 0, 2, 4, 0, 0, 0, 4, 7, 0,
}

// cpu6502PageCrossPenalty marks the read instructions that take one extra
// cycle when indexing crosses a page boundary (abs,X / abs,Y / (zp),Y).
var cpu6502PageCrossPenalty = pageCrossPenaltyOpcodes(
	0x1D, 0x19, 0x11,
	0x3D, 0x39, 0x31,
	0x5D, 0x59, 0x51,
	0x7D, 0x79, 0x71,
	0xBD, 0xB9, 0xB1, 0xBE, 0xBC,
	0xDD, 0xD9, 0xD1,
	0xFD, 0xF9, 0xF1,
)

func pageCrossPenaltyOpcodes(opcodes ...byte) [256]bool {
	var penalized [256]bool
	for _, op := range opcodes {
		penalized[op] = true
	}
	return penalized
}
