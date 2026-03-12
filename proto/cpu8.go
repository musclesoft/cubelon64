package main

// 8-bit primitive operations that map directly to 6502 instructions.
//
// Convention:
//   - All values are byte (uint8), mirroring 8-bit registers/memory.
//   - Carry flag C is an explicit byte (0 or 1), passed in and returned.
//   - 16-bit values are two named bytes: lo (low) and hi (high), little-endian.
//   - 32-bit values are four named bytes: b0 (lowest) .. b3 (highest).
//
// Each function name matches the 6502 mnemonic it implements.

// Global cycle counter for performance estimation.
var cycles int64

func resetCycles() { cycles = 0 }
func getCycles() int64 { return cycles }

// Memory access helpers (for accurate cycle counting in 6502 translation).
func load(val byte) byte {
	cycles += 3 // LDA zeropage: 3 cycles (absolute: 4)
	return val
}

func store(val byte) {
	cycles += 3 // STA zeropage: 3 cycles (absolute: 4)
}

func loopOverhead() {
	cycles += 5 // INX/INY (2) + CMP (2) + BNE (3 when taken, 2 when not - avg 2.5)
}

// adc: A + B + C → (result, carryOut).
// Mirrors: CLC / ADC or with carry already set from prior op.
func adc(a, b, c byte) (byte, byte) {
	cycles += 2 // ADC immediate: 2 cycles
	r := uint16(a) + uint16(b) + uint16(c)
	return byte(r), byte(r >> 8)
}

// sbc: A − B − (1−C) → (result, carryOut). Carry=1 means no borrow (SEC before SBC).
// Mirrors: SEC / SBC (or chained SBC with carry from prior op).
func sbc(a, b, c byte) (byte, byte) {
	cycles += 2 // SBC immediate: 2 cycles
	r := uint16(a) - uint16(b) - uint16(1-c)
	return byte(r), byte(1-(r>>8)&1)
}

// asl: arithmetic/logical shift left → (result, carryOut=old bit 7).
func asl(a byte) (byte, byte) {
	cycles += 2 // ASL accumulator: 2 cycles
	return a << 1, a >> 7
}

// lsr: logical shift right → (result, carryOut=old bit 0).
func lsr(a byte) (byte, byte) {
	cycles += 2 // LSR accumulator: 2 cycles
	return a >> 1, a & 1
}

// rol: rotate left through carry → (result, carryOut=old bit 7).
func rol(a, c byte) (byte, byte) {
	cycles += 2 // ROL accumulator: 2 cycles
	return (a << 1) | c, a >> 7
}

// ror: rotate right through carry → (result, carryOut=old bit 0).
func ror(a, c byte) (byte, byte) {
	cycles += 2 // ROR accumulator: 2 cycles
	return (a >> 1) | (c << 7), a & 1
}

// neg16: negate a 16-bit little-endian pair. Mirrors: SEC / LDA #0 / SBC lo / LDA #0 / SBC hi.
func neg16(lo, hi byte) (byte, byte) {
	r, c := sbc(0, lo, 1)
	h, _ := sbc(0, hi, c)
	return r, h
}

// neg32: negate a 32-bit little-endian value.
func neg32(b0, b1, b2, b3 byte) (byte, byte, byte, byte) {
	r0, c := sbc(0, b0, 1)
	r1, c := sbc(0, b1, c)
	r2, c := sbc(0, b2, c)
	r3, _ := sbc(0, b3, c)
	return r0, r1, r2, r3
}

// isNeg returns true if bit 7 is set (N flag after LDA/STA/arithmetic).
// Mirrors: BMI / BPL decision.
func isNeg(a byte) bool {
	cycles += 2 // Branch instruction: 2-3 cycles (assume 2 avg)
	return a&0x80 != 0
}

// cmpGE returns true if a >= b (unsigned). Mirrors: CMP b / BCS.
func cmpGE(a, b byte) bool {
	cycles += 4 // CMP immediate (2) + BCS (2)
	return a >= b
}

// cmp16GE returns true if (aHi:aLo) >= (bHi:bLo) (unsigned 16-bit).
// Mirrors: SEC / SBC bLo → carry / SBC bHi → BCS (carry set = no borrow = >=).
func cmp16GE(aLo, aHi, bLo, bHi byte) bool {
	cycles += 2 // SEC/CLC + branch overhead
	_, c := sbc(aLo, bLo, 1)
	_, c = sbc(aHi, bHi, c)
	return c != 0
}

// div8: unsigned 8÷8 → (quotient, remainder) via shift-and-subtract.
// 8 fixed iterations: ROL remainder, ASL dividend, CMP divisor, BCC skip, SBC.
// Mirrors the standard @divide_8x8 helper used in 6502 assembly.
func div8(dividend, divisor byte) (quotient, remainder byte) {
	if divisor == 0 {
		return 0xFF, 0 // guard: caller should never pass 0
	}
	for i := 0; i < 8; i++ {
		var c byte
		remainder, c = rol(remainder, dividend>>7) // ROL remainder (bit in from dividend MSB)
		dividend, _ = asl(dividend)                // ASL dividend (shift next bit out)
		quotient, _ = asl(quotient)                // ASL quotient (make room for new bit)
		if cmpGE(remainder, divisor) {             // CMP divisor / BCC skip
			remainder, _ = sbc(remainder, divisor, 1) // SEC / SBC
			quotient, _ = adc(quotient, 1, 0)         // set bit 0 of quotient
		}
		_ = c
	}
	return
}

// mul8x8: unsigned 8×8 → (lo, hi) using shift-and-add.
// Test each bit of multiplier b, add shifted a if bit is set.
// Early exit when multiplier becomes 0 (saves iterations for small values).
// Mirrors optimized @multiply_8x8 helper in rotation.s.
func mul8x8(a, b byte) (lo, hi byte) {
	cycles += 6 // Setup overhead: LDA/LDX/LDY init
	if a == 0 || b == 0 {
		return 0, 0
	}

	// Shifted multiplicand (16-bit): starts at a, shifts left each iteration
	shiftLo, shiftHi := a, byte(0)

	for i := 0; i < 8; i++ {
		cycles += 2 // LSR multiplier (2 cycles) - test LSB
		if b == 0 {
			cycles += 2 // BEQ done (branch taken: 3, but we add base 2)
			break       // Early exit when multiplier exhausted
		}
		if b&1 != 0 { // Bit is set, add shifted value (16-bit) to result
			cycles += 8 // CLC/ADC lo (4) + ADC hi (4)
			var carry byte
			lo, carry = adc(lo, shiftLo, 0)
			hi, _ = adc(hi, shiftHi, carry)
			cycles -= 4 // ADC ops already counted 4, we added 8
		} else {
			cycles += 2 // BCC skip_add (branch taken)
		}
		b >>= 1 // Shift multiplier right (already counted in LSR above)

		cycles += 4 // ASL shiftLo (2) + ROL shiftHi (2)
		var carry byte
		shiftLo, carry = asl(shiftLo)
		shiftHi, _ = rol(shiftHi, carry)
		cycles -= 4 // ASL/ROL already counted cycles, subtract them
	}
	return
}
