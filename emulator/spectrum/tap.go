package spectrum

import (
	"encoding/binary"
	"fmt"
	"os"
)

// TAPBlock represents a TAP file block
type TAPBlock struct {
	Flag byte
	Data []byte
}

// LoadTAP loads a TAP file
func LoadTAP(filename string) ([]TAPBlock, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	
	var blocks []TAPBlock
	offset := 0
	
	for offset < len(data) {
		if offset+2 > len(data) {
			break
		}
		
		length := binary.LittleEndian.Uint16(data[offset : offset+2])
		offset += 2
		
		if offset+int(length) > len(data) {
			break
		}
		
		blockData := data[offset : offset+int(length)]
		offset += int(length)
		
		if len(blockData) > 0 {
			blocks = append(blocks, TAPBlock{
				Flag: blockData[0],
				Data: blockData[1:],
			})
		}
	}
	
	fmt.Printf("Loaded %d TAP blocks\n", len(blocks))
	return blocks, nil
}

// LoaderData represents CUBELON loader structure
type LoaderData struct {
	Bank   byte
	Length uint16
	Dest   uint16
}

// ParseLoaderData parses the loader data structure
func ParseLoaderData(loaderCode []byte) []LoaderData {
	// Data structure starts at offset 0x4D in the loader
	// Structure: bank(1), length(2 LE), dest(2 LE) = 5 bytes per entry
	offset := 0x4D
	var entries []LoaderData

	for offset+4 < len(loaderCode) {
		bank := loaderCode[offset]

		if bank == 0x08 {
			// Jump to entry point marker
			break
		}

		// Length and dest are little-endian
		length := binary.LittleEndian.Uint16(loaderCode[offset+1 : offset+3])
		dest := binary.LittleEndian.Uint16(loaderCode[offset+3 : offset+5])

		entries = append(entries, LoaderData{
			Bank:   bank,
			Length: length,  // Note: may be larger than actual TAP block
			Dest:   dest,
		})

		offset += 5
	}

	return entries
}
