package main

import (
	"fmt"
	"os"
)

// LZDecompressor implements the Z80 LZ decompression algorithm from $820A-$82AE
type LZDecompressor struct {
	source []byte // Compressed source data
	dest   []byte // Decompressed destination buffer

	srcPtr  int  // HL register (source pointer)
	destPtr int  // DE register (dest pointer)
	acc     byte // A register (bit accumulator)
	carry   bool // Carry flag

	// Saved state at $821B
	savedBC uint16
}

func NewLZDecompressor(source []byte, destSize int) *LZDecompressor {
	return &LZDecompressor{
		source: source,
		dest:   make([]byte, destSize),
	}
}

// getBit reads one bit from the accumulator, refilling from source when needed
func (d *LZDecompressor) getBit() bool {
	// If accumulator is empty (all bits used), refill
	if d.acc == 0 {
		if d.srcPtr >= len(d.source) {
			return false
		}
		d.acc = d.source[d.srcPtr]
		d.srcPtr++
		// RLA (rotate left through carry) - sets carry to bit 7
		d.carry = (d.acc & 0x80) != 0
		d.acc = (d.acc << 1) | 0x01 // Set bit 0 as marker
	}

	// ADD A,A - shift left, bit 7 → carry
	d.carry = (d.acc & 0x80) != 0
	d.acc = d.acc << 1

	return d.carry
}

// readBits reads n bits as an integer (MSB first)
func (d *LZDecompressor) readBits(n int) int {
	val := 0
	for i := 0; i < n; i++ {
		val = (val << 1)
		if d.getBit() {
			val |= 1
		}
	}
	return val
}

// readVariableLength reads a variable-length value using unary encoding
// Reads bits until a 1 is encountered, returns count of 0s
func (d *LZDecompressor) readVariableLength() int {
	length := 0
	for !d.getBit() {
		length++
	}
	return length
}

// copyLiteral copies literal bytes from source to dest
func (d *LZDecompressor) copyLiteral(count int) {
	for i := 0; i < count; i++ {
		if d.srcPtr >= len(d.source) || d.destPtr >= len(d.dest) {
			return
		}
		d.dest[d.destPtr] = d.source[d.srcPtr]
		d.destPtr++
		d.srcPtr++
	}
}

// copyBackReference copies bytes from earlier in dest buffer
func (d *LZDecompressor) copyBackReference(offset, length int) {
	srcPos := d.destPtr - offset
	for i := 0; i < length; i++ {
		if srcPos < 0 || srcPos >= len(d.dest) || d.destPtr >= len(d.dest) {
			return
		}
		d.dest[d.destPtr] = d.dest[srcPos]
		d.destPtr++
		srcPos++
	}
}

// Decompress performs LZ decompression
func (d *LZDecompressor) Decompress() []byte {
	// Initialize
	d.srcPtr = 0
	d.destPtr = 0
	d.acc = 0

	for d.destPtr < len(d.dest) {
		// Read control bit
		if !d.getBit() {
			// Bit = 0: Literal byte
			if d.srcPtr >= len(d.source) {
				break
			}
			d.dest[d.destPtr] = d.source[d.srcPtr]
			d.destPtr++
			d.srcPtr++
		} else {
			// Bit = 1: Back-reference
			// Read offset and length

			// Simple model: Read offset (12 bits) and length (4 bits)
			offset := d.readBits(12)
			if offset == 0 {
				break // End marker
			}

			length := d.readBits(4)
			if length == 0 {
				length = 1
			}

			// Copy from back-reference
			d.copyBackReference(offset, length+2) // Minimum length 2
		}
	}

	return d.dest
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run zx_lz_decompressor.go <input.bin> <output.bin>")
		os.Exit(1)
	}

	// Load compressed source
	source, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Printf("Error loading source: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loaded %d bytes of compressed data\n", len(source))

	// Decompress (4KB working buffer)
	dec := NewLZDecompressor(source, 4096)
	decompressed := dec.Decompress()

	fmt.Printf("Decompressed to %d bytes\n", len(decompressed))

	// Save
	err = os.WriteFile(os.Args[2], decompressed, 0644)
	if err != nil {
		fmt.Printf("Error saving output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Saved to %s\n", os.Args[2])
}
