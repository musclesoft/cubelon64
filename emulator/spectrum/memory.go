package spectrum

import "fmt"

// Memory implements jsspeccy-style page-mapped memory
type Memory struct {
	// 8 RAM banks + 2 ROM banks + 2 dummy banks = 12 total
	// Banks 0-7: RAM
	// Banks 8-9: ROM (8=ROM0, 9=ROM1)
	// Banks 10-11: Dummy (for ROM writes)
	Banks [12][0x4000]byte

	// Track which bytes have been written (for debugging)
	Written [12][0x4000]bool

	// Page maps for 4 regions (0x0000, 0x4000, 0x8000, 0xC000)
	PageReadMap  [4]byte
	PageWriteMap [4]byte

	// Screen bank (5 or 7)
	ScreenPageIndex byte

	// Paging locked?
	PagingLocked bool

	// CPU cycle counter (pointer to CPU's Cycles field for contention)
	CPUCycles *uint64
}

func NewMemoryV2() *Memory {
	m := &Memory{}
	// Initialize for 128K mode (reset() in jsspeccy)
	m.PageReadMap = [4]byte{8, 5, 2, 0}   // ROM0, RAM5, RAM2, RAM0
	m.PageWriteMap = [4]byte{11, 5, 2, 0} // Dummy, RAM5, RAM2, RAM0
	m.ScreenPageIndex = 5
	m.PagingLocked = false
	return m
}

func (m *Memory) Read(addr uint16) byte {
	// Apply contention if accessing screen memory
	if m.CPUCycles != nil && m.isScreenMemory(addr) {
		delay := m.getContentionDelay()
		*m.CPUCycles += delay
	}

	region := addr >> 14           // Which 16KB region (0-3)?
	page := m.PageReadMap[region]  // Which bank?
	offset := addr & 0x3FFF        // Offset within bank

	// Always panic if reading uninitialized memory
	if !m.Written[page][offset] {
		panic(fmt.Sprintf("Reading uninitialized memory at 0x%04x (bank %d offset 0x%04x)", addr, page, offset))
	}

	return m.Banks[page][offset]
}

func (m *Memory) Write(addr uint16, val byte) {
	// Apply contention if accessing screen memory
	if m.CPUCycles != nil && m.isScreenMemory(addr) {
		delay := m.getContentionDelay()
		*m.CPUCycles += delay
	}

	region := addr >> 14            // Which 16KB region (0-3)?
	page := m.PageWriteMap[region]  // Which bank?
	offset := addr & 0x3FFF         // Offset within bank

	m.Banks[page][offset] = val
	m.Written[page][offset] = true
}

// SetPaging handles port 0x7FFD writes
func (m *Memory) SetPaging(val byte) {
	if m.PagingLocked {
		return
	}

	// Bits 0-2: RAM page at 0xC000
	ramPage := val & 0x07
	m.PageReadMap[3] = ramPage
	m.PageWriteMap[3] = ramPage

	// Bit 3: Screen select (0=bank 5, 1=bank 7)
	if val&0x08 != 0 {
		m.ScreenPageIndex = 7
	} else {
		m.ScreenPageIndex = 5
	}

	// Bit 4: ROM select (0=ROM0/bank 8, 1=ROM1/bank 9)
	if val&0x10 != 0 {
		m.PageReadMap[0] = 9
	} else {
		m.PageReadMap[0] = 8
	}

	// Bit 5: Paging lock
	if val&0x20 != 0 {
		m.PagingLocked = true
	}
}

// LoadROM loads ROM data into bank 8 (16KB ROM)
func (m *Memory) LoadROM(data []byte) {
	copy(m.Banks[8][:], data)
	// Mark ROM bytes as written
	for i := range m.Banks[8] {
		m.Written[8][i] = true
	}
}

// Load128ROM loads 128K ROM data into banks 8 and 9
func (m *Memory) Load128ROM(rom0, rom1 []byte) {
	copy(m.Banks[8][:], rom0)  // ROM 0
	copy(m.Banks[9][:], rom1)  // ROM 1
	// Mark ROM bytes as written
	for i := range m.Banks[8] {
		m.Written[8][i] = true
	}
	for i := range m.Banks[9] {
		m.Written[9][i] = true
	}
}

// isScreenMemory checks if address is in the active screen bank
func (m *Memory) isScreenMemory(addr uint16) bool {
	// Screen memory is in the $4000-$7FFF region (region 1)
	if addr < 0x4000 || addr >= 0x8000 {
		return false
	}
	// Check if region 1 is mapped to the active screen bank
	return m.PageReadMap[1] == m.ScreenPageIndex
}

// getContentionDelay returns the contention delay for the current frame cycle
// ZX Spectrum 128K contention: matches the documented ULA timing
func (m *Memory) getContentionDelay() uint64 {
	if m.CPUCycles == nil {
		return 0
	}

	// ZX Spectrum 128K frame structure:
	// - Total: 69,888 T-states per frame (50Hz)
	// - 312 scanlines × 224 T-states/line
	// - Interrupt fires at T-state 0 (start of frame)
	// - Visible area: scanlines 64-255 (192 lines)
	// - Screen memory contention: scanlines 64-255, first 128 T-states of each line

	frameCycle := *m.CPUCycles % 69888

	// Contention range: T-states 14336 to 57343
	// (224 * 64 = 14336 start, 224 * 256 = 57344 end)
	if frameCycle < 14336 || frameCycle >= 57344 {
		return 0 // Outside visible area
	}

	// Within visible area - check position in scanline
	linePos := frameCycle % 224

	// Contention only in first 128 T-states of each scanline (pixel fetch time)
	if linePos >= 128 {
		return 0
	}

	// Contention pattern repeats every 8 T-states
	// When ULA fetches video data, CPU must wait
	// Pattern: [6, 5, 4, 3, 2, 1, 0, 0]
	// This means: at T-state 0 of pattern, delay 6 cycles; at T-state 1, delay 5, etc.
	offset := linePos % 8
	contentionPattern := []uint64{6, 5, 4, 3, 2, 1, 0, 0}

	return contentionPattern[offset]
}
