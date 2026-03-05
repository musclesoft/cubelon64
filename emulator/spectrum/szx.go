package spectrum

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"cubelon-emu/z80"
)

// SZX file format: https://github.com/tomaz/zx-spectrum/blob/master/docs/szx_format.txt

type SZXSnapshot struct {
	MachineType byte
	Registers   z80.Registers
	Memory      map[int][]byte // page number -> 16KB data
	ULAState    SZXULAState
}

type SZXULAState struct {
	BorderColor  byte
	PagingFlags  byte
	PagingLocked bool
}

func LoadSZX(filename string) (*SZXSnapshot, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	if len(data) < 8 || string(data[0:4]) != "ZXST" {
		return nil, fmt.Errorf("not a valid SZX file")
	}

	majorVer := data[4]
	minorVer := data[5]
	_ = majorVer // version 1.x supported
	_ = minorVer

	snapshot := &SZXSnapshot{
		Memory: make(map[int][]byte),
	}

	// Parse chunks
	pos := 8
	for pos < len(data) {
		if pos+8 > len(data) {
			break
		}

		chunkID := string(data[pos : pos+4])
		chunkSize := binary.LittleEndian.Uint32(data[pos+4 : pos+8])
		pos += 8

		if pos+int(chunkSize) > len(data) {
			return nil, fmt.Errorf("chunk size exceeds file size")
		}

		chunkData := data[pos : pos+int(chunkSize)]
		pos += int(chunkSize)

		switch chunkID {
		case "Z80R":
			if err := parseZ80R(snapshot, chunkData); err != nil {
				return nil, err
			}
		case "SPCR":
			if err := parseSPCR(snapshot, chunkData); err != nil {
				return nil, err
			}
		case "RAMP":
			if err := parseRAMP(snapshot, chunkData); err != nil {
				return nil, err
			}
		}
	}

	return snapshot, nil
}

func parseZ80R(snap *SZXSnapshot, data []byte) error {
	if len(data) < 37 {
		return fmt.Errorf("Z80R chunk too small")
	}

	snap.Registers.AF = binary.LittleEndian.Uint16(data[0:2])
	snap.Registers.BC = binary.LittleEndian.Uint16(data[2:4])
	snap.Registers.DE = binary.LittleEndian.Uint16(data[4:6])
	snap.Registers.HL = binary.LittleEndian.Uint16(data[6:8])
	snap.Registers.AF_ = binary.LittleEndian.Uint16(data[8:10])
	snap.Registers.BC_ = binary.LittleEndian.Uint16(data[10:12])
	snap.Registers.DE_ = binary.LittleEndian.Uint16(data[12:14])
	snap.Registers.HL_ = binary.LittleEndian.Uint16(data[14:16])
	snap.Registers.IX = binary.LittleEndian.Uint16(data[16:18])
	snap.Registers.IY = binary.LittleEndian.Uint16(data[18:20])
	snap.Registers.SP = binary.LittleEndian.Uint16(data[20:22])
	snap.Registers.PC = binary.LittleEndian.Uint16(data[22:24])
	snap.Registers.I = data[24]
	snap.Registers.R = data[25]
	snap.Registers.IFF1 = data[26] != 0
	snap.Registers.IFF2 = data[27] != 0
	snap.Registers.IM = data[28]

	// Bytes 29-36: additional flags, not critical for basic loading

	return nil
}

func parseSPCR(snap *SZXSnapshot, data []byte) error {
	if len(data) < 8 {
		return fmt.Errorf("SPCR chunk too small")
	}

	snap.ULAState.BorderColor = data[0]
	snap.ULAState.PagingFlags = data[1]
	// data[2] is unused
	// data[3-7] are additional state

	return nil
}

func parseRAMP(snap *SZXSnapshot, data []byte) error {
	if len(data) < 3 {
		return fmt.Errorf("RAMP chunk too small")
	}

	flags := binary.LittleEndian.Uint16(data[0:2])
	pageNum := int(data[2])

	compressed := (flags & 0x01) != 0
	pageData := data[3:]

	var uncompressed []byte
	if compressed {
		// zlib compressed
		reader, err := zlib.NewReader(bytes.NewReader(pageData))
		if err != nil {
			return fmt.Errorf("failed to create zlib reader: %v", err)
		}
		defer reader.Close()

		uncompressed, err = io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("failed to decompress: %v", err)
		}
	} else {
		uncompressed = pageData
	}

	// Ensure page is exactly 16KB
	if len(uncompressed) > 0x4000 {
		uncompressed = uncompressed[:0x4000]
	} else if len(uncompressed) < 0x4000 {
		// Pad with zeros
		padded := make([]byte, 0x4000)
		copy(padded, uncompressed)
		uncompressed = padded
	}

	snap.Memory[pageNum] = uncompressed
	return nil
}

func (snap *SZXSnapshot) ApplyToSystem(mem *Memory, cpu interface{}) error {
	// Apply memory pages
	for pageNum, pageData := range snap.Memory {
		if pageNum >= 0 && pageNum < 8 {
			// RAM pages 0-7
			copy(mem.Banks[pageNum][:], pageData)
			// Mark all bytes as written
			for i := 0; i < 0x4000; i++ {
				mem.Written[pageNum][i] = true
			}
		}
	}

	// Apply paging
	mem.SetPaging(snap.ULAState.PagingFlags)

	// Apply registers - directly type assert to *z80.CPU
	if z80CPU, ok := cpu.(*z80.CPU); ok {
		z80CPU.SetRegisters(snap.Registers)
	} else {
		return fmt.Errorf("CPU is not *z80.CPU")
	}

	return nil
}
