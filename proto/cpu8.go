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

// adc: A + B + C → (result, carryOut).
// Mirrors: CLC / ADC or with carry already set from prior op.
func adc(a, b, c byte) (byte, byte) {
	r := uint16(a) + uint16(b) + uint16(c)
	return byte(r), byte(r >> 8)
}

// sbc: A − B − (1−C) → (result, carryOut). Carry=1 means no borrow (SEC before SBC).
// Mirrors: SEC / SBC (or chained SBC with carry from prior op).
func sbc(a, b, c byte) (byte, byte) {
	r := uint16(a) - uint16(b) - uint16(1-c)
	return byte(r), byte(1-(r>>8)&1)
}

// asl: arithmetic/logical shift left → (result, carryOut=old bit 7).
func asl(a byte) (byte, byte) { return a << 1, a >> 7 }

// lsr: logical shift right → (result, carryOut=old bit 0).
func lsr(a byte) (byte, byte) { return a >> 1, a & 1 }

// rol: rotate left through carry → (result, carryOut=old bit 7).
func rol(a, c byte) (byte, byte) { return (a << 1) | c, a >> 7 }

// ror: rotate right through carry → (result, carryOut=old bit 0).
func ror(a, c byte) (byte, byte) { return (a >> 1) | (c << 7), a & 1 }

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
func isNeg(a byte) bool { return a&0x80 != 0 }

// cmpGE returns true if a >= b (unsigned). Mirrors: CMP b / BCS.
func cmpGE(a, b byte) bool { return a >= b }

// cmp16GE returns true if (aHi:aLo) >= (bHi:bLo) (unsigned 16-bit).
// Mirrors: SEC / SBC bLo → carry / SBC bHi → BCS (carry set = no borrow = >=).
func cmp16GE(aLo, aHi, bLo, bHi byte) bool {
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

// mul8x8: unsigned 8×8 → (lo, hi) using repeated addition.
// Mirrors the @multiply_8x8 helper in rotation.s.
func mul8x8(a, b byte) (lo, hi byte) {
	if a == 0 || b == 0 {
		return 0, 0
	}
	for x := b; x != 0; x-- { // ldx b / @loop: ... dex / bne @loop
		var carry byte
		lo, carry = adc(lo, a, 0) // clc / adc
		if carry != 0 {           // bcc @no_carry / iny
			hi++
		}
	}
	return
}
