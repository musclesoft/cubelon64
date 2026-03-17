package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"cubelon-emu/c64"
	"cubelon-emu/m6502"
)

// TestConfig represents the test configuration
type TestConfig struct {
	GoldenDir   string       `json:"golden_dir"`
	Breakpoints []Breakpoint `json:"breakpoints"`
}

// Breakpoint represents a test checkpoint
type Breakpoint struct {
	Symbol      string      `json:"symbol"`
	Type        string      `json:"type"` // "entry" or "return"
	Description string      `json:"description"`
	Assertions  []Assertion `json:"assertions"`
}

// Assertion represents a memory comparison
type Assertion struct {
	MemorySymbol  string `json:"memory_symbol,omitempty"`
	MemoryAddress string `json:"memory_address,omitempty"`
	Length        int    `json:"length"`
	ExpectedFile  string `json:"expected_file"`
	Description   string `json:"description"`
}

// Symbol represents a symbol from the .map file
type Symbol struct {
	Name    string
	Address uint16
}

// SymbolTable maps symbol names to addresses
type SymbolTable map[string]uint16

// TestHook implements breakpoint testing
type TestHook struct {
	Config      *TestConfig
	Symbols     SymbolTable
	Memory      *c64.Memory
	GoldenDir   string
	Passed      int
	Failed      int
	BreakpointHit map[uint16]bool
}

func main() {
	configPath := flag.String("config", "../c64port/test_config.json", "Test configuration file")
	prgPath := flag.String("prg", "../c64port/build/cubelon64.prg", "PRG file to test")
	mapPath := flag.String("map", "../c64port/build/cubelon64.map", "MAP file for symbols")
	rebuild := flag.Bool("rebuild", true, "Rebuild if sources are newer")
	flag.Parse()

	// Load test configuration
	config, err := loadTestConfig(*configPath)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Check if rebuild needed
	if *rebuild {
		if needsRebuild(*prgPath) {
			fmt.Println("Building c64port/build/cubelon64.prg...")
			if err := buildPRG(); err != nil {
				fmt.Printf("Build failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("✓ Build complete")
		}
	}

	// Parse symbol table from .map file
	fmt.Printf("\nLoading symbols from %s\n", *mapPath)
	symbols, err := parseMapFile(*mapPath)
	if err != nil {
		fmt.Printf("Error parsing .map file: %v\n", err)
		os.Exit(1)
	}

	// Resolve golden directory
	goldenDir := config.GoldenDir
	if !filepath.IsAbs(goldenDir) {
		baseDir := filepath.Dir(*configPath)
		goldenDir = filepath.Join(baseDir, goldenDir)
	}

	// Print memory regions and symbols
	fmt.Println("\nSymbols loaded:")
	for name, addr := range symbols {
		fmt.Printf("  %s = $%04X\n", name, addr)
	}
	fmt.Printf("  CHARSET_RAM        = $6000 (hardcoded)\n")

	// Load and run PRG
	fmt.Printf("\nLoading %s\n", *prgPath)
	prgData, err := os.ReadFile(*prgPath)
	if err != nil {
		fmt.Printf("Error reading PRG: %v\n", err)
		os.Exit(1)
	}

	// Create C64 system
	mem := c64.NewMemory()
	mem.LoadPRG(prgData)
	cpu := m6502.New(mem)

	// Get load address from PRG (first 2 bytes)
	_ = uint16(prgData[0]) | uint16(prgData[1])<<8

	// Skip BASIC stub - PRG files typically have "SYS xxxx" at the start
	// For c64port, the actual code starts at $080E (after BASIC stub)
	// TODO: Parse BASIC stub to find actual start address
	cpu.PC = 0x080E  // Skip BASIC stub, start at machine code

	// Create test hook
	hook := &TestHook{
		Config:        config,
		Symbols:       symbols,
		Memory:        mem,
		GoldenDir:     goldenDir,
		BreakpointHit: make(map[uint16]bool),
	}

	// Run emulator with breakpoints
	fmt.Println("\nRunning emulator...\n")
	fmt.Printf("Start PC: $%04X\n", cpu.PC)
	fmt.Printf("Waiting for %d breakpoint(s)...\n\n", len(config.Breakpoints))

	maxCycles := uint64(10000000) // 10M cycles timeout
	debugEvery := uint64(100000) // Print progress every 100K cycles

	for cpu.Cycles < maxCycles {
		// Check for breakpoints
		hook.checkBreakpoints(cpu)

		// Execute instruction
		cpu.Step()

		// Debug output
		if cpu.Cycles%debugEvery == 0 {
			fmt.Printf("  Cycles: %d, PC: $%04X\n", cpu.Cycles, cpu.PC)
		}

		// Stop if all breakpoints hit
		if len(hook.BreakpointHit) >= len(config.Breakpoints) {
			break
		}
	}

	if len(hook.BreakpointHit) < len(config.Breakpoints) {
		fmt.Printf("\nWarning: Only %d/%d breakpoints hit after %d cycles\n",
			len(hook.BreakpointHit), len(config.Breakpoints), cpu.Cycles)
	}

	// Report results
	fmt.Println("\n" + strings.Repeat("=", 60))
	if hook.Failed == 0 {
		fmt.Printf("✓ PASSED: %d/%d checkpoints passed\n", hook.Passed, hook.Passed)
		os.Exit(0)
	} else {
		fmt.Printf("✗ FAILED: %d/%d checkpoints passed\n", hook.Passed, hook.Passed+hook.Failed)
		os.Exit(1)
	}
}

func (h *TestHook) checkBreakpoints(cpu *m6502.CPU) {
	for _, bp := range h.Config.Breakpoints {
		addr, ok := h.Symbols[bp.Symbol]
		if !ok {
			continue
		}

		// Check if we're at this breakpoint
		var atBreakpoint bool
		if bp.Type == "return" {
			// Check if next instruction is RTS (0x60)
			if cpu.PC == addr {
				nextByte := h.Memory.Read(cpu.PC)
				atBreakpoint = nextByte == 0x60
			}
		} else {
			atBreakpoint = cpu.PC == addr
		}

		if !atBreakpoint || h.BreakpointHit[addr] {
			continue
		}

		// Mark as hit
		h.BreakpointHit[addr] = true

		// Run assertions
		fmt.Printf("✓ %s @ $%04X\n", bp.Symbol, addr)
		if bp.Description != "" {
			fmt.Printf("  %s\n", bp.Description)
		}

		for _, assert := range bp.Assertions {
			h.runAssertion(assert)
		}
		fmt.Println()
	}
}

func (h *TestHook) runAssertion(assert Assertion) {
	// Resolve memory address
	var startAddr uint16
	if assert.MemorySymbol != "" {
		addr, ok := h.Symbols[assert.MemorySymbol]
		if !ok {
			fmt.Printf("  ✗ Symbol not found: %s\n", assert.MemorySymbol)
			h.Failed++
			return
		}
		startAddr = addr
	} else if assert.MemoryAddress != "" {
		// Parse hex address
		addrStr := strings.TrimPrefix(assert.MemoryAddress, "0x")
		addr, err := strconv.ParseUint(addrStr, 16, 16)
		if err != nil {
			fmt.Printf("  ✗ Invalid address: %s\n", assert.MemoryAddress)
			h.Failed++
			return
		}
		startAddr = uint16(addr)
	} else {
		fmt.Printf("  ✗ No memory address specified\n")
		h.Failed++
		return
	}

	// Read actual memory
	actual := make([]byte, assert.Length)
	for i := 0; i < assert.Length; i++ {
		actual[i] = h.Memory.Read(startAddr + uint16(i))
	}

	// Load expected data
	expectedPath := filepath.Join(h.GoldenDir, assert.ExpectedFile)
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		fmt.Printf("  ✗ Failed to load golden file: %v\n", err)
		h.Failed++
		return
	}

	if len(expected) != assert.Length {
		fmt.Printf("  ✗ Golden file size mismatch: expected %d bytes, got %d\n",
			assert.Length, len(expected))
		h.Failed++
		return
	}

	// Compare
	if bytes.Equal(actual, expected) {
		fmt.Printf("  ✓ %s: %d bytes match\n", assert.Description, assert.Length)
		h.Passed++
	} else {
		fmt.Printf("  ✗ %s: mismatch\n", assert.Description)
		h.Failed++

		// Show first few differences
		diffCount := 0
		for i := 0; i < len(actual) && diffCount < 10; i++ {
			if actual[i] != expected[i] {
				fmt.Printf("    +$%04X: expected %02X, got %02X\n",
					i, expected[i], actual[i])
				diffCount++
			}
		}
		if diffCount == 10 {
			fmt.Printf("    ... and more differences\n")
		}
	}
}

func loadTestConfig(path string) (*TestConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config TestConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func parseMapFile(path string) (SymbolTable, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	symbols := make(SymbolTable)
	lines := strings.Split(string(data), "\n")

	// Look for "Exports list by name:" section
	inExports := false
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Exports list by name:") {
			inExports = true
			continue
		}

		if inExports && line == "" {
			break
		}

		if inExports {
			// Parse lines like: "precompute_frames             001A9F RLA"
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				name := fields[0]
				addrStr := fields[1]
				if addr, err := strconv.ParseUint(addrStr, 16, 16); err == nil {
					symbols[name] = uint16(addr)
				}
			}
		}
	}

	// Also look for DATA segment symbols
	inData := false
	for _, line := range lines {
		if strings.Contains(line, "DATA") {
			inData = true
			continue
		}
		if inData && strings.TrimSpace(line) == "" {
			break
		}
	}

	return symbols, nil
}

func needsRebuild(prgPath string) bool {
	prgInfo, err := os.Stat(prgPath)
	if err != nil {
		return true // PRG doesn't exist
	}

	// Check if any .s file is newer
	srcDir := "../c64port/src"
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".s") {
			continue
		}

		srcPath := filepath.Join(srcDir, entry.Name())
		srcInfo, err := os.Stat(srcPath)
		if err != nil {
			continue
		}

		if srcInfo.ModTime().After(prgInfo.ModTime()) {
			return true
		}
	}

	return false
}

func buildPRG() error {
	cmd := exec.Command("make", "-C", "../c64port")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
