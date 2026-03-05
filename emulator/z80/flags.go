package z80

// Flag lookup tables matching jsspeccy's implementation
// These are built in jsspeccy's initialization code

var (
	// SZ53Table: S, Z, bit 5, bit 3 flags (matches jsspeccy line 119)
	SZ53Table [256]byte

	// ParityTable: Parity flag (matches jsspeccy line 127)
	ParityTable [256]byte

	// SZ53PTable: Combined SZ53 + Parity (matches jsspeccy line 128)
	SZ53PTable [256]byte
)

func init() {
	// Build SZ53 table: store<u8>(288332 + (i), (u8(i) & ( 0x08 | 0x20 | 0x80 )));
	for i := 0; i < 256; i++ {
		SZ53Table[i] = byte(i) & (0x08 | 0x20 | 0x80)
	}

	// Build parity table
	for i := 0; i < 256; i++ {
		j := i
		parity := 0
		for k := 0; k < 8; k++ {
			parity ^= j & 1
			j >>= 1
		}
		if parity == 0 {
			ParityTable[i] = 0x04
		} else {
			ParityTable[i] = 0x00
		}
	}

	// Build combined table
	for i := 0; i < 256; i++ {
		SZ53PTable[i] = SZ53Table[i] | ParityTable[i]
	}

	// Special case: zero has Z flag set
	SZ53Table[0] |= 0x40
	SZ53PTable[0] |= 0x40
}

// HalfCarryAddTable and HalfCarrySubTable for H flag calculation
// jsspeccy uses lookup based on: ((a & 0x88) >> 3) | ((val & 0x88) >> 2) | ((result & 0x88) >> 1)
// The lower 3 bits determine half-carry for subtract operations
var HalfCarrySubTable = [8]byte{
	0,    // 000: no half-carry
	0x10, // 001: half-carry
	0x10, // 010: half-carry
	0x10, // 011: half-carry
	0,    // 100: no half-carry
	0,    // 101: no half-carry
	0,    // 110: no half-carry
	0x10, // 111: half-carry
}

// OverflowSubTable for V flag calculation in subtract operations
// The upper bits of the lookup determine overflow
var OverflowSubTable = [8]byte{
	0,    // no overflow
	0,    // no overflow
	0,    // no overflow
	0x04, // overflow
	0x04, // overflow
	0,    // no overflow
	0,    // no overflow
	0,    // no overflow
}
