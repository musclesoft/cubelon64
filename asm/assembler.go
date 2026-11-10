package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	floating      []string // segments not in the config, laid out after DATA in first-use order
	currentSeg    *Segment
	pc            int    // program counter
	lastLabel     string // for local labels
	procDepth     int    // open .proc scopes: their plain labels leave the local label scope alone
	processed     map[string]bool
	assertions    []Assertion // assertions with their trigger addresses
	files         []string    // list of files to assemble
	inMacro       bool        // tracking if inside a .macro definition
	macroName     string      // upper case name of the macro being defined
	macroBody     []Statement // statements of the macro being defined
	macros        map[string][]Statement
	expansions    int // macro expansions so far in this pass, numbers their local label scopes
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
	a.macros = make(map[string][]Statement)
	a.expansions = 0

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
	if err := a.resolveSegmentAddresses(); err != nil {
		return err
	}

	// Update symbol addresses to reflect new segment starts
	a.updateSymbolAddresses()

	// Reset PC for pass 2
	if a.currentSeg != nil {
		a.pc = a.currentSeg.Start
	}
	a.lastLabel = ""
	a.procDepth = 0
	a.inMacro = false // Reset macro state for pass 2
	a.expansions = 0

	// Reset processed map for pass 2 so we process all files again
	a.processed = make(map[string]bool)

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
	// Check if already processed (prevent duplicate code emission)
	absPath, _ := filepath.Abs(path)
	if a.processed[absPath] {
		return nil
	}
	a.processed[absPath] = true

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
		if name == "CODE" {
			seg.Pass1PC = int(mem.Start)
			seg.Pass1Start = int(mem.Start)
		}
		seg.Alignment = 0 // No alignment requirement yet
	}
}

// resolveSegmentAddresses calculates final addresses after pass1 (sizes known)
// Segments in CODE's memory region are laid out sequentially; one that no
// longer fits spills into the next file-backed region above (skipping any gap
// between the regions). Other segments are laid out within their own region.
// Respects alignment requirements from .align directives
func (a *Assembler) resolveSegmentAddresses() error {
	chain := a.spillChain()
	chainIndex := 0
	chainOffset := 0
	if len(chain) > 0 {
		chainOffset = int(a.config.Memory[chain[0]].Start)
	}
	otherOffsets := make(map[string]int)

	for _, name := range a.layoutOrder() {
		seg, ok := a.segments[name]
		if !ok {
			continue
		}
		cfg, ok := a.config.Segments[name]
		if !ok {
			continue
		}
		mem, ok := a.config.Memory[cfg.Load]
		if !ok {
			continue
		}

		size := seg.Pass1PC - seg.Pass1Start

		if len(chain) > 0 && cfg.Load == chain[0] {
			for {
				region := a.config.Memory[chain[chainIndex]]
				start := alignUp(chainOffset, lcm(cfg.Align, seg.Alignment))
				if start+size <= int(region.Start)+int(region.Size) {
					seg.Start = start
					chainOffset = start + size
					break
				}
				if chainIndex+1 == len(chain) {
					return fmt.Errorf("memory region %s overflows by %d bytes (segment %s ends at $%04X, limit $%04X)",
						chain[chainIndex], start+size-int(region.Start)-int(region.Size), name, start+size, int(region.Start)+int(region.Size))
				}
				chainIndex++
				chainOffset = int(a.config.Memory[chain[chainIndex]].Start)
			}
			continue
		}

		offset, ok := otherOffsets[cfg.Load]
		if !ok {
			offset = int(mem.Start)
		}
		start := alignUp(offset, lcm(cfg.Align, seg.Alignment))
		limit := int(mem.Start) + int(mem.Size)
		if start+size > limit {
			return fmt.Errorf("memory region %s overflows by %d bytes (segment %s ends at $%04X, limit $%04X)",
				cfg.Load, start+size-limit, name, start+size, limit)
		}
		seg.Start = start
		otherOffsets[cfg.Load] = start + size
	}
	return nil
}

// spillChain lists CODE's memory region followed by the file-backed regions
// above it, in address order
func (a *Assembler) spillChain() []string {
	codeRegion := a.config.Segments["CODE"].Load
	code, ok := a.config.Memory[codeRegion]
	if !ok {
		return nil
	}
	var above []string
	for name, mem := range a.config.Memory {
		if name != codeRegion && mem.File && mem.Start > code.Start {
			above = append(above, name)
		}
	}
	sort.Slice(above, func(i, j int) bool {
		return a.config.Memory[above[i]].Start < a.config.Memory[above[j]].Start
	})
	return append([]string{codeRegion}, above...)
}

func lcm(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	x, y := a, b
	for y != 0 {
		x, y = y, x%y
	}
	return a / x * b
}

func alignUp(offset, align int) int {
	if align <= 0 {
		return offset
	}
	return offset + (align-offset%align)%align
}

// declareFloatingSegment registers a segment the config doesn't know. It is
// placed in CODE's memory region, so its address follows from the sizes of the
// segments before it.
func (a *Assembler) declareFloatingSegment(name string) {
	if _, ok := a.segments[name]; ok {
		return
	}
	codeConfig := a.config.Segments["CODE"]
	a.config.Segments[name] = SegmentConfig{Load: codeConfig.Load, Type: "ro"}
	a.segments[name] = &Segment{Name: name, Data: []byte{}}
	if mem, ok := a.config.Memory[codeConfig.Load]; ok {
		a.segments[name].Start = int(mem.Start)
	}
	a.floating = append(a.floating, name)
}

// layoutOrder lists the segments in address order: the built-in ones, with
// floating segments between DATA and BSS
func (a *Assembler) layoutOrder() []string {
	order := []string{"ZEROPAGE", "LOADADDR", "CODE", "RODATA", "DATA"}
	order = append(order, a.floating...)
	order = append(order, a.configuredSegments(order)...)
	return append(order, "BSS")
}

// configuredSegments lists the used segments the config defines beyond the built-in
// ones, by name; each is laid out within its own region
func (a *Assembler) configuredSegments(laidOut []string) []string {
	placed := map[string]bool{"BSS": true}
	for _, name := range laidOut {
		placed[name] = true
	}
	var names []string
	for name := range a.segments {
		if _, configured := a.config.Segments[name]; configured && !placed[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
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
	return a.pass1Statements(path, parser.NextStatement)
}

func (a *Assembler) pass1Statements(path string, next func() (Statement, error)) error {
	for {
		stmt, err := next()
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
				a.macros[a.macroName] = a.macroBody
				continue
			}
			a.macroBody = append(a.macroBody, stmt)
			continue
		}

		if inst, ok := stmt.(*InstructionStmt); ok && inst.AddressMode == Implied {
			if body, isMacro := a.macros[inst.Mnemonic]; isMacro {
				if err := a.expandMacro(inst.Mnemonic, body, func(next func() (Statement, error)) error {
					return a.pass1Statements(path, next)
				}); err != nil {
					return err
				}
				continue
			}
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
			} else if a.procDepth == 0 {
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
				a.declareFloatingSegment(segName)
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
				// Add padding to align PC within segment
				if len(s.Args) > 0 {
					alignment, err := a.evalExpr(s.Args[0])
					if err != nil {
						return fmt.Errorf("evaluating .align value: %w", err)
					}
					if alignment <= 0 {
						return fmt.Errorf(".align value must be positive, got %d", alignment)
					}
					// Calculate padding needed: (alignment - (pc % alignment)) % alignment
					padding := (alignment - (a.pc % alignment)) % alignment
					a.pc += padding
					if a.currentSeg != nil && a.currentSeg.Pass1Start == 0 {
						a.currentSeg.Alignment = lcm(a.currentSeg.Alignment, alignment)
					}
				}
			case ".proc":
				// .proc creates a label and sets scope for local labels
				if len(s.Args) > 0 {
					procName := s.Args[0]
					a.symbols[procName] = a.pc
					a.lastLabel = procName
					a.procDepth++
					// Track segment
					if a.currentSeg != nil {
						a.symbolSegment[procName] = a.currentSeg.Name
					}
				}
			case ".endproc":
				a.procDepth--
			case ".macro":
				a.inMacro = true
				a.macroName = strings.ToUpper(s.Args[0])
				a.macroBody = nil
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
	return a.pass2Statements(path, parser.NextStatement)
}

func (a *Assembler) pass2Statements(path string, next func() (Statement, error)) error {
	for {
		stmt, err := next()
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
				a.macros[a.macroName] = a.macroBody
				continue
			}
			a.macroBody = append(a.macroBody, stmt)
			continue
		}

		if inst, ok := stmt.(*InstructionStmt); ok && inst.AddressMode == Implied {
			if body, isMacro := a.macros[inst.Mnemonic]; isMacro {
				if err := a.expandMacro(inst.Mnemonic, body, func(next func() (Statement, error)) error {
					return a.pass2Statements(path, next)
				}); err != nil {
					return err
				}
				continue
			}
		}

		switch s := stmt.(type) {
		case *LabelStmt:
			if !strings.HasPrefix(s.Name, "@") && a.procDepth == 0 {
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
				// Reserve bytes - emit zeros for initialized segments (CODE, DATA, RODATA)
				// Skip emission for uninitialized segments (BSS, ZEROPAGE)
				if len(s.Args) > 0 {
					count, err := a.evalExpr(s.Args[0])
					if err != nil {
						return fmt.Errorf("evaluating .res count: %w", err)
					}

					// Check if current segment is uninitialized (BSS or ZEROPAGE)
					segCfg, ok := a.config.Segments[a.currentSeg.Name]
					isUninit := ok && (segCfg.Type == "bss" || segCfg.Type == "zp")

					if !isUninit {
						// Emit zeros for initialized segments (matches ca65 behavior)
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
					// For uninit segments, don't emit (space reserved in pass1)
				}

			case ".align":
				// Emit padding bytes to align to boundary (only for initialized segments)
				if len(s.Args) > 0 {
					alignment, err := a.evalExpr(s.Args[0])
					if err != nil {
						return fmt.Errorf("evaluating .align value: %w", err)
					}
					if alignment <= 0 {
						return fmt.Errorf(".align value must be positive, got %d", alignment)
					}

					// Check if current segment is uninitialized (BSS or ZEROPAGE)
					segCfg, ok := a.config.Segments[a.currentSeg.Name]
					isUninit := ok && (segCfg.Type == "bss" || segCfg.Type == "zp")

					if !isUninit {
						// Only emit padding for initialized segments
						currentPos := a.currentSeg.Start + len(a.currentSeg.Data)
						padding := (alignment - (currentPos % alignment)) % alignment
						for i := 0; i < padding; i++ {
							a.emitByte(0)
						}
						a.pc += padding
					}
					// For uninit segments, alignment is handled in pass1/segment layout
				}

			case ".proc":
				// .proc creates a label and sets scope for local labels
				if len(s.Args) > 0 {
					procName := s.Args[0]
					// Symbol already defined in pass1, just set lastLabel for local label scope
					a.lastLabel = procName
					a.procDepth++
				}
			case ".endproc":
				a.procDepth--
			case ".import", ".export":
				// Already processed in pass1, ignore in pass2
			case ".macro":
				a.inMacro = true
				a.macroName = strings.ToUpper(s.Args[0])
				a.macroBody = nil
			case ".endmacro":
				// Should be handled by inMacro check above
				return fmt.Errorf(".endmacro without matching .macro")
			default:
				return fmt.Errorf("unknown directive %s (missing implementation?)", s.Name)
			}

		case *ConstStmt:
			// Re-evaluate in pass 2 so that constants referencing labels get the
			// final relocated address, not the pass-1 segment-relative value.
			val, err := a.evalExpr(s.Value)
			if err != nil {
				return err
			}
			a.symbols[s.Name] = val

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

// expandMacro runs the macro body through the current pass. Its local labels
// get a scope of their own per expansion.
func (a *Assembler) expandMacro(name string, body []Statement, run func(func() (Statement, error)) error) error {
	outerLabel := a.lastLabel
	a.expansions++
	a.lastLabel = fmt.Sprintf("%s#%d", name, a.expansions)
	index := 0
	next := func() (Statement, error) {
		if index == len(body) {
			return nil, nil
		}
		index++
		return body[index-1], nil
	}
	err := run(next)
	a.lastLabel = outerLabel
	return err
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

	// Handle #< and #> operators (low/high byte) — immediate mode: "#<SYMBOL"
	if strings.HasPrefix(expr, "#<") {
		val, err := a.evalByteOperand(expr, expr[2:])
		if err != nil {
			return 0, err
		}
		return val & 0xFF, nil
	}
	if strings.HasPrefix(expr, "#>") {
		val, err := a.evalByteOperand(expr, expr[2:])
		if err != nil {
			return 0, err
		}
		return (val >> 8) & 0xFF, nil
	}
	// Handle bare < and > operators (low/high byte) — used in .byte directives
	if strings.HasPrefix(expr, "<") {
		val, err := a.evalByteOperand(expr, expr[1:])
		if err != nil {
			return 0, err
		}
		return val & 0xFF, nil
	}
	if strings.HasPrefix(expr, ">") {
		val, err := a.evalByteOperand(expr, expr[1:])
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
	// Find the load address (typically from LOADADDR segment or first CODE byte)
	loadAddr := uint16(0x0801)
	if loadSeg, ok := a.segments["LOADADDR"]; ok && len(loadSeg.Data) >= 2 {
		loadAddr = uint16(loadSeg.Data[0]) | uint16(loadSeg.Data[1])<<8
	}

	// PRG header (load address)
	image := []byte{uint8(loadAddr), uint8(loadAddr >> 8)}

	// Write segments in order, zero-filling the gaps left by reserved ranges
	for _, name := range a.layoutOrder() {
		seg, ok := a.segments[name]
		if !ok || len(seg.Data) == 0 {
			continue
		}
		// Skip LOADADDR as it's just metadata for the load address
		if name == "LOADADDR" {
			continue
		}
		// Skip BSS and ZEROPAGE segments - they're uninitialized data allocated at runtime
		if cfg, ok := a.config.Segments[name]; ok && (cfg.Type == "bss" || cfg.Type == "zp") {
			continue
		}
		next := int(loadAddr) + len(image) - 2
		if seg.Start < next {
			return fmt.Errorf("segment %s at $%04X overlaps earlier output ending at $%04X", name, seg.Start, next)
		}
		image = append(image, make([]byte, seg.Start-next)...)
		image = append(image, seg.Data...)
	}

	return os.WriteFile(path, image, 0644)
}

func (a *Assembler) WriteMeta(path string) error {

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

// evalByteOperand evaluates the operand of a low or high byte operator, which covers the
// whole rest of the expression. An operand with an operator of its own is rejected: `>A + 1`
// and `>A + >B` would otherwise silently mean `>(A + 1)` and `>(A + >B)`.
func (a *Assembler) evalByteOperand(expr, operand string) (int, error) {
	depth := 0
	for i, r := range strings.TrimSpace(operand) {
		switch {
		case r == '(':
			depth++
		case r == ')':
			depth--
		case depth == 0 && i > 0 && strings.ContainsRune("+-*/<>", r):
			return 0, fmt.Errorf("ambiguous byte operator in %q: parenthesize its operand or use a constant", expr)
		}
	}
	return a.evalExpr(operand)
}
