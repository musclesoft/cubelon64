package c64

import (
	"cubelon-emu/m6502"
)

// Memory implements the C64 memory map with banking
type Memory struct {
	RAM     [65536]byte // 64KB RAM
	BASIC   [8192]byte  // BASIC ROM ($A000-$BFFF)
	KERNAL  [8192]byte  // KERNAL ROM ($E000-$FFFF)
	CHARSET [4096]byte  // Character ROM ($D000-$DFFF when banked)

	// Memory banking control
	// Processor port at $0001 controls ROM/IO visibility
	ProcessorPort byte
	DDR           byte // Data direction register at $0000

	// VIC bank selection (controlled by CIA2 port A bits 0-1)
	// 0=bank 3 ($C000), 1=bank 2 ($8000), 2=bank 1 ($4000), 3=bank 0 ($0000)
	VICBank byte

	// I/O devices
	VIC        *VIC
	CIA1       *CIA1
	ColorRAM   [1024]byte // Color RAM is separate 4-bit RAM
	OnSIDWrite func(register byte, val byte)
}

func NewMemory() *Memory {
	m := &Memory{
		ProcessorPort: 0x37, // Default: All ROMs visible, IO visible
		DDR:           0xFF,
		VICBank:       3, // Default: bank 0 ($0000-$3FFF)
		CIA1:          NewCIA1(),
	}

	// Initialize default ROM content (for now, just placeholder)
	// Real C64 would load actual ROM files
	m.initDefaultROMs()

	return m
}

func (m *Memory) initDefaultROMs() {
	// KERNAL reset vector points to a simple infinite loop
	// $FFFC-$FFFD: Reset vector
	m.KERNAL[0x1FFC] = 0x00 // Low byte of $E000
	m.KERNAL[0x1FFD] = 0xE0 // High byte

	// Simple initialization code at $E000
	// Just an infinite loop for now: JMP $E000
	m.KERNAL[0] = 0x4C // JMP absolute
	m.KERNAL[1] = 0x00
	m.KERNAL[2] = 0xE0
}

// Read implements m6502.Memory interface
func (m *Memory) Read(addr uint16) byte {
	// Special handling for processor port
	if addr == 0x0000 {
		return m.DDR
	}
	if addr == 0x0001 {
		return m.ProcessorPort
	}

	// Decode memory banking based on processor port
	loram := (m.ProcessorPort & 0x01) != 0
	hiram := (m.ProcessorPort & 0x02) != 0
	charen := (m.ProcessorPort & 0x04) != 0

	// I/O area ($D000-$DFFF)
	if addr >= 0xD000 && addr < 0xE000 {
		if charen {
			// I/O visible
			return m.readIO(addr)
		} else if hiram {
			// Character ROM visible
			return m.CHARSET[addr-0xD000]
		}
		// Otherwise RAM
		return m.RAM[addr]
	}

	// BASIC ROM area ($A000-$BFFF)
	if addr >= 0xA000 && addr < 0xC000 {
		if loram && hiram {
			return m.BASIC[addr-0xA000]
		}
		return m.RAM[addr]
	}

	// KERNAL ROM area ($E000-$FFFF)
	if addr >= 0xE000 {
		if hiram {
			return m.KERNAL[addr-0xE000]
		}
		return m.RAM[addr]
	}

	// Everything else is RAM
	return m.RAM[addr]
}

// Write implements m6502.Memory interface
func (m *Memory) Write(addr uint16, val byte) {
	// Special handling for processor port
	if addr == 0x0000 {
		m.DDR = val
		return
	}
	if addr == 0x0001 {
		m.ProcessorPort = val
		return
	}

	charen := (m.ProcessorPort & 0x04) != 0

	// I/O area - writes always go to I/O regardless of banking
	if addr >= 0xD000 && addr < 0xE000 {
		if charen {
			m.writeIO(addr, val)
		} else {
			// If Character ROM visible, writes go to underlying RAM
			m.RAM[addr] = val
		}
		return
	}

	// ROM areas are not writable, writes go to underlying RAM
	if (addr >= 0xA000 && addr < 0xC000) || addr >= 0xE000 {
		m.RAM[addr] = val
		return
	}

	// Everything else is RAM
	m.RAM[addr] = val
}

// readIO handles I/O area reads
func (m *Memory) readIO(addr uint16) byte {
	// VIC-II registers ($D000-$D3FF, repeated every $40 bytes)
	if addr >= 0xD000 && addr < 0xD400 {
		if m.VIC != nil {
			return m.VIC.ReadRegister(byte(addr & 0x3F))
		}
		return 0xFF
	}

	// SID registers ($D400-$D7FF)
	if addr >= 0xD400 && addr < 0xD800 {
		// TODO: SID chip emulation
		return 0xFF
	}

	// Color RAM ($D800-$DBFF, only 1000 bytes, repeated)
	if addr >= 0xD800 && addr < 0xDC00 {
		offset := (addr - 0xD800) % 1000
		return m.ColorRAM[offset] | 0xF0 // Upper 4 bits always 1
	}

	// CIA #1 ($DC00-$DCFF)
	if addr >= 0xDC00 && addr < 0xDD00 {
		return m.CIA1.Read(byte(addr & 0x0F))
	}

	// CIA #2 ($DD00-$DDFF)
	if addr >= 0xDD00 && addr < 0xDE00 {
		// TODO: CIA #2 emulation
		return 0xFF
	}

	// Unmapped I/O
	return 0xFF
}

// writeIO handles I/O area writes
func (m *Memory) writeIO(addr uint16, val byte) {
	// VIC-II registers ($D000-$D3FF)
	if addr >= 0xD000 && addr < 0xD400 {
		if m.VIC != nil {
			m.VIC.WriteRegister(byte(addr&0x3F), val)
		}
		return
	}

	// SID registers ($D400-$D7FF)
	if addr >= 0xD400 && addr < 0xD800 {
		if m.OnSIDWrite != nil && addr&0x1F <= 0x18 {
			m.OnSIDWrite(byte(addr&0x1F), val)
		}
		return
	}

	// Color RAM ($D800-$DBFF)
	if addr >= 0xD800 && addr < 0xDC00 {
		offset := (addr - 0xD800) % 1000
		m.ColorRAM[offset] = val & 0x0F // Only lower 4 bits writable
		return
	}

	// CIA #1 ($DC00-$DCFF)
	if addr >= 0xDC00 && addr < 0xDD00 {
		m.CIA1.Write(byte(addr&0x0F), val)
		return
	}

	// CIA #2 ($DD00-$DDFF)
	if addr >= 0xDD00 && addr < 0xDE00 {
		// CIA #2 Port A ($DD00) controls VIC bank selection (bits 0-1, inverted)
		// Bits 0-1: VIC bank select (inverted)
		//   %11 (3) -> bank 0 ($0000-$3FFF)
		//   %10 (2) -> bank 1 ($4000-$7FFF)
		//   %01 (1) -> bank 2 ($8000-$BFFF)
		//   %00 (0) -> bank 3 ($C000-$FFFF)
		if addr == 0xDD00 {
			m.VICBank = val & 0x03
		}
		// TODO: Full CIA #2 emulation
		return
	}
}

// LoadPRG loads a PRG file into memory at the specified address
func (m *Memory) LoadPRG(data []byte) {
	if len(data) < 2 {
		return
	}

	// PRG files start with 2-byte load address
	loadAddr := uint16(data[0]) | uint16(data[1])<<8

	// Copy data to memory
	for i := 2; i < len(data); i++ {
		m.RAM[loadAddr+uint16(i-2)] = data[i]
	}
}

// Verify Memory implements m6502.Memory interface
var _ m6502.Memory = (*Memory)(nil)
