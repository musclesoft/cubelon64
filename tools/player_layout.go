package main

// Zero-page memory layout for the 6502 tracker player.
// Per-channel fields use SoA layout: field[ch] = ZP_FIELD + ch, ch ∈ {0,1,2}.
// This mirrors LDA ZP_FIELD,X in the 6502 player with X = channel index.

const (
	zpStreamPtrLo = 0x00 // +ch: stream (or block body) read pointer, lo byte
	zpStreamPtrHi = 0x03 // +ch: stream (or block body) read pointer, hi byte
	zpReturnPtrLo = 0x06 // +ch: stream resume pointer lo (0 = reading from stream)
	zpReturnPtrHi = 0x09 // +ch: stream resume pointer hi
	zpBlockCtr    = 0x0C // +ch: opcodes remaining in current block (0 = reading from stream)
)
