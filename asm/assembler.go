package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Assembler struct {
	config        *Config
	baseDir       string
	symbols       map[string]int    // symbol name -> address
	symbolSegment map[string]string // symbol name -> segment name
	imports       map[string]bool
	exports       map[string]bool
	segments      map[string]*Segment
	currentSeg    *Segment
	pc            int    // program counter
	lastLabel     string // for local labels
	processed     map[string]bool
	assertions    []Assertion // assertions with their trigger addresses
	files         []string    // list of files to assemble
	inMacro       bool        // tracking if inside a .macro definition
}

type Assertion struct {
	Address      int    `json:"address"`
	MemoryAddr   int    `json:"memory_addr"`           // Evaluated address to check
	Type         string `json:"type"`                  // "memory" or "value"
	ExpectedFile string `json:"expected_file,omitempty"`
	Length       int    `json:"length,omitempty"`
	FileOffset   int    `json:"file_offset,omitempty"`
	ExpectedVal  *int   `json:"expected_value,omitempty"`
	CondSymbol   string `json:"cond_symbol,omitempty"`
	CondValue    *int   `json:"cond_value,omitempty"`
}

type Segment struct {
	Name        string
	Data        []byte
	Start       int // resolved start address
	Pass1PC     int // PC tracking during pass1 (independent of Data.len)
	Pass1Start  int // Start address used during pass1 (for size calculation)
	Alignment   int // Required alignment for this segment (0 = no requirement)
}

func NewAssembler(config *Config, baseDir string) *Assembler {
	asm := &Assembler{
		config:        config,
		baseDir:       baseDir,
		symbols:       make(map[string]int),
		symbolSegment: make(map[string]string),
		imports:       make(map[string]bool),
		exports:       make(map[string]bool),
		segments:      make(map[string]*Segment),
		processed:     make(map[string]bool),
	}

	// Create segments from config
	for name := range config.Segments {
		asm.segments[name] = &Segment{
			Name: name,
			Data: []byte{},
		}
	}

	// Default to CODE segment
	asm.currentSeg = asm.segments["CODE"]

	return asm
}

func (a *Assembler) AssembleFile(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}

	// Reset macro state
	a.inMacro = false

	// Initialize segment addresses for pass1
	a.initSegmentAddresses()

	// Initialize PC for pass1 - use segment-relative addressing
	if a.currentSeg != nil {
		a.pc = a.currentSeg.Pass1PC
	}

	// Two-pass assembly
	// Pass 1: Build symbol table and calculate addresses
	if err := a.pass1WithIncludes(absPath, data); err != nil {
		return fmt.Errorf("pass 1: %w", err)
	}

	// After pass1, resolve final addresses based on actual segment sizes
	a.resolveSegmentAddresses()

	// Update symbol addresses to reflect new segment starts
	a.updateSymbolAddresses()

	// Reset PC for pass 2
	if a.currentSeg != nil {
		a.pc = a.currentSeg.Start
	}
	a.lastLabel = ""
	a.inMacro = false // Reset macro state for pass 2

	// Pass 2: Generate code
	if err := a.pass2WithIncludes(absPath, data); err != nil {
		return fmt.Errorf("pass 2: %w", err)
	}

	return nil
}

// pass1WithIncludes handles .include directives inline
func (a *Assembler) pass1WithIncludes(path string, data []byte) error {
	// Check if already processed (prevent circular includes)
	absPath, _ := filepath.Abs(path)
	if a.processed[absPath] {
		return nil
	}
	a.processed[absPath] = true

	return a.pass1(path, data)
}

// pass2WithIncludes handles .include directives inline
func (a *Assembler) pass2WithIncludes(path string, data []byte) error {
	return a.pass2(path, data)
}

// initSegmentAddresses sets initial addresses for pass1 (before sizes are known)
func (a *Assembler) initSegmentAddresses() {
	for name, seg := range a.segments {
		cfg, ok := a.config.Segments[name]
		if !ok {
			continue
		}
		mem, ok := a.config.Memory[cfg.Load]
		if !ok {
			continue
		}
		seg.Start = int(mem.Start)
		// For pass1, each segment tracks its own size independently starting from 0
		seg.Pass1PC = 0
		seg.Pass1Start = 0
		seg.Alignment = 0 // No alignment requirement yet
	}
}

// resolveSegmentAddresses calculates final addresses after pass1 (sizes known)
// Segments sharing the same memory region are laid out sequentially
// Respects alignment requirements from .align directives
func (a *Assembler) resolveSegmentAddresses() {
	// Group segments by their Load memory region
	type SegInfo struct {
		name string
		seg  *Segment
		cfg  SegmentConfig
	}

	memoryGroups := make(map[string][]SegInfo)

	for name, seg := range a.segments {
		cfg, ok := a.config.Segments[name]
		if !ok {
			continue
		}
		memoryGroups[cfg.Load] = append(memoryGroups[cfg.Load], SegInfo{
			name: name,
			seg:  seg,
			cfg:  cfg,
		})
	}

	// For each memory region, lay out segments sequentially
	// Order: LOADADDR, CODE, RODATA, DATA, BSS
	segmentOrder := []string{"LOADADDR", "CODE", "RODATA", "DATA", "BSS"}

	for memName, group := range memoryGroups {
		mem, ok := a.config.Memory[memName]
		if !ok {
			continue
		}

		offset := int(mem.Start)

		// Process segments in defined order
		for _, segName := range segmentOrder {
			for _, info := range group {
				if info.name == segName {
					// Align offset if segment has alignment requirement
					if info.seg.Alignment > 0 {
						padding := (info.seg.Alignment - (offset % info.seg.Alignment)) % info.seg.Alignment
						offset += padding
					}

					// Calculate size from pass1: Pass1PC - Pass1Start
					size := info.seg.Pass1PC - info.seg.Pass1Start

					// Set final start address
					info.seg.Start = offset

					// Advance offset by segment size
					offset += size
					break
				}
			}
		}
	}
}

// updateSymbolAddresses adjusts symbol addresses after segment relocation
func (a *Assembler) updateSymbolAddresses() {
	// For each symbol, use its tracked segment to adjust its address
	for name, addr := range a.symbols {
		segName, ok := a.symbolSegment[name]
		if !ok {
			// Symbol has no segment tracking (e.g., constants), skip
			continue
		}

		seg, ok := a.segments[segName]
		if !ok {
			continue
		}

		// Calculate offset within segment
		offset := addr - seg.Pass1Start
		// Update to new segment start + offset
		a.symbols[name] = seg.Start + offset
	}
}

func (a *Assembler) pass1(path string, data []byte) error {
	lexer := NewLexer(strings.NewReader(string(data)))
	parser := NewParser(lexer)

	for {
		stmt, err := parser.NextStatement()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if stmt == nil {
			break
		}

		// Track current file for better error messages
		_ = path

		// Check if we're inside a macro definition
		if a.inMacro {
			// Only allow .endmacro inside macro definitions
			if dir, ok := stmt.(*DirectiveStmt); ok && dir.Name == ".endmacro" {
				a.inMacro = false
				continue
			}
			return fmt.Errorf("%s: statements not allowed inside .macro (macro expansion not implemented - only empty macros for assertions)", path)
		}

		switch s := stmt.(type) {
		case *LabelStmt:
			// Register label with current PC
			label := s.Name
			if strings.HasPrefix(label, "@") {
				// Local label - prefix with last global label
				if a.lastLabel == "" {
					return fmt.Errorf("local label %s without preceding global label", label)
				}
				label = a.lastLabel + label
			} else {
				a.lastLabel = label
			}
			a.symbols[label] = a.pc
			// Track which segment this symbol belongs to
			if a.currentSeg != nil {
				a.symbolSegment[label] = a.currentSeg.Name
			}

		case *DirectiveStmt:
			switch s.Name {
			case ".include":
				// Process include inline during pass 1
				if len(s.Args) > 0 {
					includePath := s.Args[0]
					if !filepath.IsAbs(includePath) {
						includePath = filepath.Join(a.baseDir, includePath)
					}
					includeData, err := os.ReadFile(includePath)
					if err != nil {
						return fmt.Errorf("reading include %s: %w", includePath, err)
					}
					if err := a.pass1WithIncludes(includePath, includeData); err != nil {
						return err
					}
				}
			case ".import":
				a.imports[s.Args[0]] = true
			case ".export":
				a.exports[s.Args[0]] = true
			case ".segment":
				segName := s.Args[0]
				if seg, ok := a.segments[segName]; ok {
					// Save current segment's PC
					if a.currentSeg != nil {
						a.currentSeg.Pass1PC = a.pc
					}
					// Switch to new segment and restore its PC
					a.currentSeg = seg
					a.pc = seg.Pass1PC
				}
			case ".byte":
				a.pc += len(s.Args)
			case ".word":
				a.pc += len(s.Args) * 2
			case ".res", ".resb":
				// Reserve bytes - first arg is count
				if len(s.Args) > 0 {
					count, err := a.evalExpr(s.Args[0])
					if err != nil {
						return fmt.Errorf("evaluating .res count: %w", err)
					}
					a.pc += count
				}
			case ".align":
				// Track segment alignment requirement for first .align in segment
				if len(s.Args) > 0 {
					alignment, err := a.evalExpr(s.Args[0])
					if err != nil {
						return fmt.Errorf("evaluating .align value: %w", err)
					}
					if alignment <= 0 {
						return fmt.Errorf(".align value must be positive, got %d", alignment)
					}
					// If this is the first .align at the start of the segment, track it
					if a.currentSeg != nil && a.pc == 0 && a.currentSeg.Alignment == 0 {
						a.currentSeg.Alignment = alignment
					}
					// Calculate padding needed: (alignment - (pc % alignment)) % alignment
					padding := (alignment - (a.pc % alignment)) % alignment
					a.pc += padding
				}
			case ".proc":
				// .proc creates a label and sets scope for local labels
				if len(s.Args) > 0 {
					procName := s.Args[0]
					a.symbols[procName] = a.pc
					a.lastLabel = procName
					// Track segment
					if a.currentSeg != nil {
						a.symbolSegment[procName] = a.currentSeg.Name
					}
				}
			case ".endproc":
				// .endproc ends the scope
			case ".macro":
				// Enter macro definition (only empty macros allowed)
				a.inMacro = true
			case ".endmacro":
				// Should be handled by inMacro check above
				return fmt.Errorf(".endmacro without matching .macro")
			default:
				return fmt.Errorf("unknown directive %s (missing implementation?)", s.Name)
			}

		case *ConstStmt:
			// Register constant
			val, err := a.evalExpr(s.Value)
			if err != nil {
				return err
			}
			a.symbols[s.Name] = val

		case *InstructionStmt:
			// Estimate instruction size
			size, err := a.instructionSize(s)
			if err != nil {
				return err
			}
			a.pc += size

		case *AssertStmt:
			// Assertions don't generate code, just note them
			// We'll record them in pass2 with resolved PC
		}
	}

	// Save final PC for current segment
	if a.currentSeg != nil {
		a.currentSeg.Pass1PC = a.pc
	}

	return nil
}

func (a *Assembler) pass2(path string, data []byte) error {
	lexer := NewLexer(strings.NewReader(string(data)))
	parser := NewParser(lexer)
	a.lastLabel = ""

	for {
		stmt, err := parser.NextStatement()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if stmt == nil {
			break
		}

		// Check if we're inside a macro definition
		if a.inMacro {
			// Only allow .endmacro inside macro definitions
			if dir, ok := stmt.(*DirectiveStmt); ok && dir.Name == ".endmacro" {
				a.inMacro = false
				continue
			}
			return fmt.Errorf("%s: statements not allowed inside .macro (macro expansion not implemented - only empty macros for assertions)", path)
		}

		switch s := stmt.(type) {
		case *LabelStmt:
			if !strings.HasPrefix(s.Name, "@") {
				a.lastLabel = s.Name
			}
			// Labels don't emit code

		case *DirectiveStmt:
			switch s.Name {
			case ".include":
				// Process include inline during pass 2
				if len(s.Args) > 0 {
					includePath := s.Args[0]
					if !filepath.IsAbs(includePath) {
						includePath = filepath.Join(a.baseDir, includePath)
					}
					includeData, err := os.ReadFile(includePath)
					if err != nil {
						return fmt.Errorf("reading include %s: %w", includePath, err)
					}
					if err := a.pass2WithIncludes(includePath, includeData); err != nil {
						return err
					}
				}

			case ".segment":
				segName := s.Args[0]
				if seg, ok := a.segments[segName]; ok {
					a.currentSeg = seg
					// Set PC to current position in this segment
					a.pc = seg.Start + len(seg.Data)
				}

			case ".byte":
				for _, arg := range s.Args {
					val, err := a.evalExpr(arg)
					if err != nil {
						return err
					}
					a.emitByte(uint8(val))
				}

			case ".word":
				for _, arg := range s.Args {
					val, err := a.evalExpr(arg)
					if err != nil {
						return err
					}
					a.emitWord(uint16(val))
				}

			case ".res", ".resb":
				// Reserve bytes - emit zeros
				if len(s.Args) > 0 {
					count, err := a.evalExpr(s.Args[0])
					if err != nil {
						return fmt.Errorf("evaluating .res count: %w", err)
					}
					// Optionally fill with value (second arg)
					fillValue := uint8(0)
					if len(s.Args) > 1 {
						val, err := a.evalExpr(s.Args[1])
						if err != nil {
							return fmt.Errorf("evaluating .res fill value: %w", err)
						}
						fillValue = uint8(val)
					}
					for i := 0; i < count; i++ {
						a.emitByte(fillValue)
					}
				}

			case ".align":
				// Emit padding bytes to align to boundary
				if len(s.Args) > 0 {
					alignment, err := a.evalExpr(s.Args[0])
					if err != nil {
						return fmt.Errorf("evaluating .align value: %w", err)
					}
					if alignment <= 0 {
						return fmt.Errorf(".align value must be positive, got %d", alignment)
					}
					// Calculate padding needed
					currentPos := a.currentSeg.Start + len(a.currentSeg.Data)
					padding := (alignment - (currentPos % alignment)) % alignment
					for i := 0; i < padding; i++ {
						a.emitByte(0)
					}
					a.pc += padding
				}

			case ".proc":
				// .proc creates a label and sets scope for local labels
				if len(s.Args) > 0 {
					procName := s.Args[0]
					// Symbol already defined in pass1, just set lastLabel for local label scope
					a.lastLabel = procName
				}
			case ".endproc":
				// .endproc ends the scope
			case ".import", ".export":
				// Already processed in pass1, ignore in pass2
			case ".macro":
				// Enter macro definition (only empty macros allowed)
				a.inMacro = true
			case ".endmacro":
				// Should be handled by inMacro check above
				return fmt.Errorf(".endmacro without matching .macro")
			default:
				return fmt.Errorf("unknown directive %s (missing implementation?)", s.Name)
			}

		case *InstructionStmt:
			if err := a.emitInstruction(s); err != nil {
				return err
			}

		case *AssertStmt:
			// Record assertion at current PC
			// The assertion will fire BEFORE the next instruction executes
			currentPC := a.currentSeg.Start + len(a.currentSeg.Data)

			// Evaluate symbol expression to get memory address
			memoryAddr, err := a.evalExpr(s.SymbolExpr)
			if err != nil {
				return fmt.Errorf("evaluating assertion symbol expression %s: %w", s.SymbolExpr, err)
			}

			assertion := Assertion{
				Address:    currentPC,
				MemoryAddr: memoryAddr,
				CondSymbol: s.CondSymbol,
				CondValue:  s.CondValue,
			}

			if s.ExpectedFile != "" {
				// Memory assertion
				assertion.Type = "memory"
				assertion.ExpectedFile = s.ExpectedFile
				assertion.Length = s.Length
				assertion.FileOffset = s.FileOffset
			} else {
				// Value assertion
				assertion.Type = "value"
				assertion.ExpectedVal = s.ExpectedVal
			}

			a.assertions = append(a.assertions, assertion)
		}
	}

	return nil
}

func (a *Assembler) emitByte(b uint8) {
	a.currentSeg.Data = append(a.currentSeg.Data, b)
}

func (a *Assembler) emitWord(w uint16) {
	a.emitByte(uint8(w))
	a.emitByte(uint8(w >> 8))
}

func (a *Assembler) evalExpr(expr string) (int, error) {
	expr = strings.TrimSpace(expr)

	// Handle #< and #> operators (low/high byte)
	if strings.HasPrefix(expr, "#<") {
		val, err := a.evalExpr(expr[2:])
		if err != nil {
			return 0, err
		}
		return val & 0xFF, nil
	}
	if strings.HasPrefix(expr, "#>") {
		val, err := a.evalExpr(expr[2:])
		if err != nil {
			return 0, err
		}
		return (val >> 8) & 0xFF, nil
	}

	// Remove parentheses
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return a.evalExpr(expr[1 : len(expr)-1])
	}

	// Handle addition (lowest precedence)
	if idx := strings.LastIndex(expr, "+"); idx > 0 {
		left, err := a.evalExpr(strings.TrimSpace(expr[:idx]))
		if err != nil {
			return 0, err
		}
		right, err := a.evalExpr(strings.TrimSpace(expr[idx+1:]))
		if err != nil {
			return 0, err
		}
		return left + right, nil
	}

	// Handle subtraction (but not negative numbers at the start)
	if idx := strings.LastIndex(expr, "-"); idx > 0 {
		left, err := a.evalExpr(strings.TrimSpace(expr[:idx]))
		if err != nil {
			return 0, err
		}
		right, err := a.evalExpr(strings.TrimSpace(expr[idx+1:]))
		if err != nil {
			return 0, err
		}
		return left - right, nil
	}

	// Handle multiplication (higher precedence)
	if idx := strings.Index(expr, "*"); idx > 0 {
		left, err := a.evalExpr(strings.TrimSpace(expr[:idx]))
		if err != nil {
			return 0, err
		}
		right, err := a.evalExpr(strings.TrimSpace(expr[idx+1:]))
		if err != nil {
			return 0, err
		}
		return left * right, nil
	}

	// Handle division
	if idx := strings.Index(expr, "/"); idx > 0 {
		left, err := a.evalExpr(strings.TrimSpace(expr[:idx]))
		if err != nil {
			return 0, err
		}
		right, err := a.evalExpr(strings.TrimSpace(expr[idx+1:]))
		if err != nil {
			return 0, err
		}
		if right == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return left / right, nil
	}

	// Check for hex/decimal number
	if strings.HasPrefix(expr, "$") || (len(expr) > 0 && expr[0] >= '0' && expr[0] <= '9') {
		return ParseNumber(expr)
	}

	// Resolve local labels
	symbol := expr
	if strings.HasPrefix(symbol, "@") {
		if a.lastLabel == "" {
			return 0, fmt.Errorf("local label %s without preceding global label", symbol)
		}
		symbol = a.lastLabel + symbol
	}

	// Check for symbol
	if val, ok := a.symbols[symbol]; ok {
		return val, nil
	}

	return 0, fmt.Errorf("undefined symbol or expression: %s", expr)
}

func (a *Assembler) WritePRG(path string) error {
	// Calculate segment addresses based on config
	for name, seg := range a.segments {
		cfg, ok := a.config.Segments[name]
		if !ok {
			continue
		}
		mem, ok := a.config.Memory[cfg.Load]
		if !ok {
			continue
		}
		seg.Start = int(mem.Start)
	}

	// Find the load address (typically from LOADADDR segment or first CODE byte)
	loadAddr := uint16(0x0801)
	if loadSeg, ok := a.segments["LOADADDR"]; ok && len(loadSeg.Data) >= 2 {
		loadAddr = uint16(loadSeg.Data[0]) | uint16(loadSeg.Data[1])<<8
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write PRG header (load address)
	f.Write([]byte{uint8(loadAddr), uint8(loadAddr >> 8)})

	// Write segments in order
	segmentOrder := []string{"LOADADDR", "CODE", "RODATA", "DATA", "BSS"}
	for _, name := range segmentOrder {
		if seg, ok := a.segments[name]; ok && len(seg.Data) > 0 {
			// Skip LOADADDR as it's just metadata for the load address
			if name == "LOADADDR" {
				continue
			}
			// Skip BSS segments - they're uninitialized data allocated at runtime
			if cfg, ok := a.config.Segments[name]; ok && cfg.Type == "bss" {
				continue
			}
			f.Write(seg.Data)
		}
	}

	return nil
}

func (a *Assembler) WriteMeta(path string) error {
	if len(a.assertions) == 0 {
		// No assertions, don't create file
		return nil
	}

	// Group assertions by address
	assertionsByAddr := make(map[string][]Assertion)
	for _, assertion := range a.assertions {
		addr := fmt.Sprintf("0x%04X", assertion.Address)
		assertionsByAddr[addr] = append(assertionsByAddr[addr], assertion)
	}

	// Create metadata structure
	meta := map[string]interface{}{
		"assertions": assertionsByAddr,
		"symbols":    a.symbols,
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
