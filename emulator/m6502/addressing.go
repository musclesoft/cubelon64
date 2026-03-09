package m6502

// Addressing mode functions return (address, page_boundary_crossed)
// Page boundary crossing adds an extra cycle for certain instructions

// Immediate: operand is the next byte
func (c *CPU) addrImmediate() (uint16, bool) {
	addr := c.PC
	c.PC++
	return addr, false
}

// Zero Page: address is in page 0 (0x00XX)
func (c *CPU) addrZeroPage() (uint16, bool) {
	addr := uint16(c.fetch())
	return addr, false
}

// Zero Page,X: address is in page 0, indexed by X
func (c *CPU) addrZeroPageX() (uint16, bool) {
	addr := uint16(c.fetch()+c.X) & 0xFF
	return addr, false
}

// Zero Page,Y: address is in page 0, indexed by Y
func (c *CPU) addrZeroPageY() (uint16, bool) {
	addr := uint16(c.fetch()+c.Y) & 0xFF
	return addr, false
}

// Absolute: full 16-bit address
func (c *CPU) addrAbsolute() (uint16, bool) {
	lo := c.fetch()
	hi := c.fetch()
	return uint16(hi)<<8 | uint16(lo), false
}

// Absolute,X: 16-bit address indexed by X
func (c *CPU) addrAbsoluteX() (uint16, bool) {
	lo := c.fetch()
	hi := c.fetch()
	addr := uint16(hi)<<8 | uint16(lo)
	finalAddr := addr + uint16(c.X)
	// Page boundary crossed if high byte changed
	pageCrossed := (addr & 0xFF00) != (finalAddr & 0xFF00)
	return finalAddr, pageCrossed
}

// Absolute,Y: 16-bit address indexed by Y
func (c *CPU) addrAbsoluteY() (uint16, bool) {
	lo := c.fetch()
	hi := c.fetch()
	addr := uint16(hi)<<8 | uint16(lo)
	finalAddr := addr + uint16(c.Y)
	// Page boundary crossed if high byte changed
	pageCrossed := (addr & 0xFF00) != (finalAddr & 0xFF00)
	return finalAddr, pageCrossed
}

// Indirect: used only by JMP - reads address from memory
func (c *CPU) addrIndirect() (uint16, bool) {
	ptrLo := c.fetch()
	ptrHi := c.fetch()
	ptr := uint16(ptrHi)<<8 | uint16(ptrLo)

	// 6502 bug: if low byte is 0xFF, high byte wraps within same page
	var addr uint16
	if ptrLo == 0xFF {
		lo := c.Mem.Read(ptr)
		hi := c.Mem.Read(ptr & 0xFF00) // Wrap to start of page
		addr = uint16(hi)<<8 | uint16(lo)
	} else {
		addr = c.read16(ptr)
	}
	return addr, false
}

// Indexed Indirect (X): (Zero Page,X)
// Used as (addr,X) - add X to zero page address, then read 16-bit address from there
func (c *CPU) addrIndexedIndirect() (uint16, bool) {
	zp := c.fetch()
	ptr := uint16(zp+c.X) & 0xFF // Wrap within zero page

	lo := c.Mem.Read(ptr)
	hi := c.Mem.Read((ptr + 1) & 0xFF) // Wrap within zero page
	addr := uint16(hi)<<8 | uint16(lo)
	return addr, false
}

// Indirect Indexed (Y): (Zero Page),Y
// Used as (addr),Y - read 16-bit address from zero page, then add Y
func (c *CPU) addrIndirectIndexed() (uint16, bool) {
	zp := c.fetch()

	lo := c.Mem.Read(uint16(zp))
	hi := c.Mem.Read(uint16(zp+1) & 0xFF) // Wrap within zero page
	addr := uint16(hi)<<8 | uint16(lo)
	finalAddr := addr + uint16(c.Y)

	// Page boundary crossed if high byte changed
	pageCrossed := (addr & 0xFF00) != (finalAddr & 0xFF00)
	return finalAddr, pageCrossed
}

// Relative: used by branch instructions
// Returns the target address and whether a page boundary was crossed
func (c *CPU) addrRelative() (uint16, bool) {
	offset := int8(c.fetch())
	oldPC := c.PC
	newPC := uint16(int32(c.PC) + int32(offset))

	// Page boundary crossed if high byte changed
	pageCrossed := (oldPC & 0xFF00) != (newPC & 0xFF00)
	return newPC, pageCrossed
}
