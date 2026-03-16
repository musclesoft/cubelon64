package cpu8

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
//
// IMPORTANT: This counter ONLY tracks explicit 6502 operations (adc, sbc, asl, cmp, etc.)
// It does NOT count the actual computational work done by Go operations:
//   - Bitwise ops: & (AND), ^ (EOR), | (ORA) → uncounted, but are 2-4 cycles each in 6502
//   - Comparisons: <, >, ==, != → uncounted, but map to CMP (2-4 cycles)
//   - Array indexing: arr[i] → uncounted, but is LDA/STA (2-6 cycles depending on addressing)
//   - Conditionals: if/else → uncounted branches (2-3 cycles)
//
// As a result, the cycle count here is vastly underestimated (typically 5-6× too low).
// Use empirical emulator measurements for realistic performance estimates.
var cycles int64

func ResetCycles() { cycles = 0 }
func GetCycles() int64 { return cycles }

// Memory access helpers (for accurate cycle counting in 6502 translation).
func Load(val byte) byte {
	cycles += 3 // LDA zeropage: 3 cycles (absolute: 4)
	return val
}

func Store(val byte) {
	cycles += 3 // STA zeropage: 3 cycles (absolute: 4)
}

func LoopOverhead() {
	cycles += 5 // INX/INY (2) + CMP (2) + BNE (3 when taken, 2 when not - avg 2.5)
}

func TableLookup(val byte) byte {
	cycles += 4 // LDA abs,X table lookup
	return val
}

// Adc: A + B + C → (result, carryOut).
// Mirrors: CLC / ADC or with carry already set from prior op.
func Adc(a, b, c byte) (byte, byte) {
	cycles += 2 // ADC immediate: 2 cycles
	r := uint16(a) + uint16(b) + uint16(c)
	return byte(r), byte(r >> 8)
}

// Sbc: A − B − (1−C) → (result, carryOut). Carry=1 means no borrow (SEC before SBC).
// Mirrors: SEC / SBC (or chained SBC with carry from prior op).
func Sbc(a, b, c byte) (byte, byte) {
	cycles += 2 // SBC immediate: 2 cycles
	r := uint16(a) - uint16(b) - uint16(1-c)
	return byte(r), byte(1-(r>>8)&1)
}

// Asl: arithmetic/logical shift left → (result, carryOut=old bit 7).
func Asl(a byte) (byte, byte) {
	cycles += 2 // ASL accumulator: 2 cycles
	return a << 1, a >> 7
}

// Lsr: logical shift right → (result, carryOut=old bit 0).
func Lsr(a byte) (byte, byte) {
	cycles += 2 // LSR accumulator: 2 cycles
	return a >> 1, a & 1
}

// Rol: rotate left through carry → (result, carryOut=old bit 7).
func Rol(a, c byte) (byte, byte) {
	cycles += 2 // ROL accumulator: 2 cycles
	return (a << 1) | c, a >> 7
}

// Ror: rotate right through carry → (result, carryOut=old bit 0).
func Ror(a, c byte) (byte, byte) {
	cycles += 2 // ROR accumulator: 2 cycles
	return (a >> 1) | (c << 7), a & 1
}

// Neg16: negate a 16-bit little-endian pair. Mirrors: SEC / LDA #0 / SBC lo / LDA #0 / SBC hi.
func Neg16(lo, hi byte) (byte, byte) {
	r, c := Sbc(0, lo, 1)
	h, _ := Sbc(0, hi, c)
	return r, h
}

// Neg32: negate a 32-bit little-endian value.
func Neg32(b0, b1, b2, b3 byte) (byte, byte, byte, byte) {
	r0, c := Sbc(0, b0, 1)
	r1, c := Sbc(0, b1, c)
	r2, c := Sbc(0, b2, c)
	r3, _ := Sbc(0, b3, c)
	return r0, r1, r2, r3
}

// IsNeg returns true if bit 7 is set (N flag after LDA/STA/arithmetic).
// Mirrors: BMI / BPL decision.
func IsNeg(a byte) bool {
	cycles += 2 // Branch instruction: 2-3 cycles (assume 2 avg)
	return a&0x80 != 0
}

// CmpGE returns true if a >= b (unsigned). Mirrors: CMP b / BCS.
func CmpGE(a, b byte) bool {
	cycles += 4 // CMP immediate (2) + BCS (2)
	return a >= b
}

// Cmp16GE returns true if (aHi:aLo) >= (bHi:bLo) (unsigned 16-bit).
// Mirrors: SEC / SBC bLo → carry / SBC bHi → BCS (carry set = no borrow = >=).
func Cmp16GE(aLo, aHi, bLo, bHi byte) bool {
	cycles += 2 // SEC/CLC + branch overhead
	_, c := Sbc(aLo, bLo, 1)
	_, c = Sbc(aHi, bHi, c)
	return c != 0
}

// Div8: unsigned 8÷8 → (quotient, remainder) via shift-and-subtract.
// 8 fixed iterations: ROL remainder, ASL dividend, CMP divisor, BCC skip, SBC.
// Mirrors the standard @divide_8x8 helper used in 6502 assembly.
func Div8(dividend, divisor byte) (quotient, remainder byte) {
	if divisor == 0 {
		return 0xFF, 0 // guard: caller should never pass 0
	}
	for i := 0; i < 8; i++ {
		var c byte
		remainder, c = Rol(remainder, dividend>>7) // ROL remainder (bit in from dividend MSB)
		dividend, _ = Asl(dividend)                // ASL dividend (shift next bit out)
		quotient, _ = Asl(quotient)                // ASL quotient (make room for new bit)
		if CmpGE(remainder, divisor) {             // CMP divisor / BCC skip
			remainder, _ = Sbc(remainder, divisor, 1) // SEC / SBC
			quotient, _ = Adc(quotient, 1, 0)         // set bit 0 of quotient
		}
		_ = c
	}
	return
}

// Mul8x8: unsigned 8×8 → (lo, hi) using shift-and-add.
// Test each bit of multiplier b, add shifted a if bit is set.
// Early exit when multiplier becomes 0 (saves iterations for small values).
// Mirrors optimized @multiply_8x8 helper in rotation.s.
func Mul8x8(a, b byte) (lo, hi byte) {
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
			lo, carry = Adc(lo, shiftLo, 0)
			hi, _ = Adc(hi, shiftHi, carry)
			cycles -= 4 // ADC ops already counted 4, we added 8
		} else {
			cycles += 2 // BCC skip_add (branch taken)
		}
		b >>= 1 // Shift multiplier right (already counted in LSR above)

		cycles += 4 // ASL shiftLo (2) + ROL shiftHi (2)
		var carry byte
		shiftLo, carry = Asl(shiftLo)
		shiftHi, _ = Rol(shiftHi, carry)
		cycles -= 4 // ASL/ROL already counted cycles, subtract them
	}
	return
}

// Div16x16: unsigned 16÷16 → (quotient byte, remainder 16-bit) using shift-and-subtract.
// Input: dividend (lo, hi), divisor (lo, hi)
// Output: quotient (fits in byte for our geometry: max 48), remainder (lo, hi)
// Mirrors div16x16 in dda_proto.s - 16 iterations.
func Div16x16(divLo, divHi, divisorLo, divisorHi byte) (quotient byte, remLo, remHi byte) {
	cycles += 6 // Setup overhead
	if divisorHi == 0 && divisorLo == 0 {
		return 0xFF, 0, 0 // guard
	}

	// Shift-and-subtract: 16 iterations
	for i := 0; i < 16; i++ {
		cycles += 10 // ASL/ROL dividend (4) + ROL remainder (4) + ASL quotient (2)

		// Shift dividend left 16-bit, MSB feeds into remainder via carry flag
		var carry byte
		divLo, carry = Asl(divLo)           // Shift lo, MSB → carry
		divHi, carry = Rol(divHi, carry)    // Rotate hi, carry in, MSB → carry

		// ROL remainder 16-bit: carry from dividend MSB becomes remainder LSB
		remLo, carry = Rol(remLo, carry)    // Rotate lo, carry in from dividend
		remHi, _ = Rol(remHi, carry)        // Rotate hi, carry from lo

		// ASL quotient (make room for new bit)
		quotient, _ = Asl(quotient)

		// Compare remainder with divisor: if remainder >= divisor, subtract and set quotient bit
		cycles += 6 // CMP + branch overhead
		_, c := Sbc(remLo, divisorLo, 1)
		_, c = Sbc(remHi, divisorHi, c)
		if c != 0 { // No borrow = remainder >= divisor
			cycles += 6 // Subtract + increment
			remLo, c = Sbc(remLo, divisorLo, 1)
			remHi, _ = Sbc(remHi, divisorHi, c)
			quotient, _ = Adc(quotient, 1, 0)
		}
	}
	return
}

// Mul16x8: unsigned 16×8 → 24-bit result (b0, b1, b2) using shift-and-add.
// Input: 16-bit multiplicand (lo, hi), 8-bit multiplier
// Output: 24-bit product (b0, b1, b2)
// Mirrors mul16x8 in dda_proto.s.
func Mul16x8(aLo, aHi, b byte) (b0, b1, b2 byte) {
	cycles += 6 // Setup overhead
	if b == 0 {
		return 0, 0, 0
	}

	// Result: 24-bit accumulator
	var r0, r1, r2 byte

	// Shifted multiplicand: starts at (aLo, aHi), shifts left each iteration
	shiftLo, shiftHi, shiftTop := aLo, aHi, byte(0)

	for i := 0; i < 8; i++ {
		cycles += 2 // LSR multiplier
		if b == 0 {
			cycles += 2 // BEQ done
			break
		}
		if b&1 != 0 { // Add shifted multiplicand (24-bit) to result
			cycles += 12 // CLC/ADC (3 bytes × 4 cycles each)
			var carry byte
			r0, carry = Adc(r0, shiftLo, 0)
			r1, carry = Adc(r1, shiftHi, carry)
			r2, _ = Adc(r2, shiftTop, carry)
			cycles -= 6 // ADC already counted
		} else {
			cycles += 2 // BCC skip
		}
		b >>= 1

		// Shift multiplicand left (24-bit)
		cycles += 6 // ASL lo + ROL hi + ROL top
		var carry byte
		shiftLo, carry = Asl(shiftLo)
		shiftHi, carry = Rol(shiftHi, carry)
		shiftTop, _ = Rol(shiftTop, carry)
		cycles -= 6 // Already counted
	}
	return r0, r1, r2
}

// Div24x16: unsigned 24÷16 → (quotient 16-bit, remainder 16-bit) using shift-and-subtract.
// Input: dividend (b0, b1, b2), divisor (lo, hi)
// Output: quotient (lo, hi), remainder (lo, hi)
// Mirrors div24x16 in dda_proto.s - 24 iterations.
func Div24x16(divB0, divB1, divB2, divisorLo, divisorHi byte) (quotLo, quotHi, remLo, remHi byte) {
	cycles += 6 // Setup overhead
	if divisorHi == 0 && divisorLo == 0 {
		return 0xFF, 0xFF, 0, 0 // guard
	}

	// Shift-and-subtract: 24 iterations
	for i := 0; i < 24; i++ {
		cycles += 12 // ASL/ROL dividend (6) + ROL remainder (4) + ASL quotient (2)

		// Shift dividend left 24-bit, MSB feeds into remainder via carry
		var carry byte
		divB0, carry = Asl(divB0)           // Shift b0, MSB → carry
		divB1, carry = Rol(divB1, carry)    // Rotate b1
		divB2, carry = Rol(divB2, carry)    // Rotate b2, MSB → carry

		// ROL remainder 16-bit
		remLo, carry = Rol(remLo, carry)
		remHi, _ = Rol(remHi, carry)

		// ASL quotient (16-bit)
		quotLo, carry = Asl(quotLo)
		quotHi, _ = Rol(quotHi, carry)

		// Compare remainder with divisor
		cycles += 6
		_, c := Sbc(remLo, divisorLo, 1)
		_, c = Sbc(remHi, divisorHi, c)
		if c != 0 { // remainder >= divisor
			cycles += 6
			remLo, c = Sbc(remLo, divisorLo, 1)
			remHi, _ = Sbc(remHi, divisorHi, c)
			quotLo, c = Adc(quotLo, 1, 0)
			quotHi, _ = Adc(quotHi, 0, c)
		}
	}
	return
}
