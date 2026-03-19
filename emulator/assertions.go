package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cubelon-emu/c64"
)

var errConditionNotMet = errors.New("condition not met")

// MetaAssertion represents an assertion from the .meta.json file
type MetaAssertion struct {
	Address      int    `json:"address"`
	Symbol       string `json:"symbol"`
	Type         string `json:"type"` // "memory" or "value"
	ExpectedFile string `json:"expected_file,omitempty"`
	Length       int    `json:"length,omitempty"`
	FileOffset   int    `json:"file_offset,omitempty"`
	ExpectedVal  *int   `json:"expected_value,omitempty"`
	CondSymbol   string `json:"cond_symbol,omitempty"`
	CondValue    *int   `json:"cond_value,omitempty"`
}

// Metadata holds assertion and symbol data from .meta.json
type Metadata struct {
	Assertions map[string][]MetaAssertion `json:"assertions"`
	Symbols    map[string]int             `json:"symbols"`
}

// AssertionChecker manages runtime assertion checking
type AssertionChecker struct {
	Meta        *Metadata
	BaseDir     string
	PassCount   int
	FailCount   int
	CheckedPCs  map[uint16]bool // Track which PCs we've already checked
}

// LoadMetadata loads a .meta.json file
func LoadMetadata(prgPath string) (*Metadata, error) {
	metaPath := prgPath + ".meta.json"

	// Check if metadata file exists
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		return nil, nil // No metadata file, not an error
	}

	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("reading metadata: %w", err)
	}

	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing metadata: %w", err)
	}

	return &meta, nil
}

// NewAssertionChecker creates a new assertion checker
func NewAssertionChecker(meta *Metadata, prgDir string) *AssertionChecker {
	return &AssertionChecker{
		Meta:       meta,
		BaseDir:    prgDir,
		CheckedPCs: make(map[uint16]bool),
	}
}

// Check runs assertions for the current PC
func (ac *AssertionChecker) Check(pc uint16, mem *c64.Memory) {
	if ac.Meta == nil {
		return
	}

	// Check if we've already checked this PC
	if ac.CheckedPCs[pc] {
		return
	}

	// Look up assertions for this PC
	addrKey := fmt.Sprintf("0x%04X", pc)
	assertions, ok := ac.Meta.Assertions[addrKey]
	if !ok {
		return
	}

	// Mark as checked
	ac.CheckedPCs[pc] = true

	fmt.Printf("\n🔍 Checking assertions at PC=$%04X\n", pc)

	// Run each assertion
	for _, assertion := range assertions {
		result := ac.runAssertion(&assertion, mem)
		if result == errConditionNotMet {
			// Condition not met, skip silently
			continue
		} else if result != nil {
			fmt.Printf("  ✗ FAIL: %v\n", result)
			ac.FailCount++
		} else {
			fmt.Printf("  ✓ PASS: %s\n", ac.formatAssertion(&assertion))
			ac.PassCount++
		}
	}
}

func (ac *AssertionChecker) runAssertion(assertion *MetaAssertion, mem *c64.Memory) error {
	// Check condition first (if present)
	if assertion.CondSymbol != "" && assertion.CondValue != nil {
		condAddr, err := ac.resolveSymbol(assertion.CondSymbol)
		if err != nil {
			return fmt.Errorf("resolving condition symbol %s: %w", assertion.CondSymbol, err)
		}

		condActual := mem.Read(uint16(condAddr))
		condExpected := uint8(*assertion.CondValue)

		if condActual != condExpected {
			// Condition not met, skip silently
			return errConditionNotMet
		}
	}

	// Resolve symbol to address
	symbolAddr, err := ac.resolveSymbol(assertion.Symbol)
	if err != nil {
		return fmt.Errorf("resolving symbol %s: %w", assertion.Symbol, err)
	}

	switch assertion.Type {
	case "memory":
		return ac.checkMemory(symbolAddr, assertion, mem)
	case "value":
		return ac.checkValue(symbolAddr, assertion, mem)
	default:
		return fmt.Errorf("unknown assertion type: %s", assertion.Type)
	}
}

func (ac *AssertionChecker) checkMemory(addr int, assertion *MetaAssertion, mem *c64.Memory) error {
	// Load expected data from file
	expectedPath := filepath.Join(ac.BaseDir, assertion.ExpectedFile)
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		return fmt.Errorf("loading expected file %s: %w", assertion.ExpectedFile, err)
	}

	// Check file has enough data at offset
	fileOffset := assertion.FileOffset
	if len(expected) < fileOffset+assertion.Length {
		return fmt.Errorf("expected file too small: need %d bytes at offset %d, file has %d bytes",
			assertion.Length, fileOffset, len(expected))
	}

	// Read actual memory
	actual := make([]byte, assertion.Length)
	for i := 0; i < assertion.Length; i++ {
		actual[i] = mem.Read(uint16(addr + i))
	}

	// Compare with data at file offset
	expectedSlice := expected[fileOffset : fileOffset+assertion.Length]
	if !bytes.Equal(actual, expectedSlice) {
		return fmt.Errorf("memory mismatch at $%04X (%d bytes, file offset %d)", addr, assertion.Length, fileOffset)
	}

	return nil
}

func (ac *AssertionChecker) checkValue(addr int, assertion *MetaAssertion, mem *c64.Memory) error {
	if assertion.ExpectedVal == nil {
		return fmt.Errorf("value assertion missing expected_value")
	}

	actual := mem.Read(uint16(addr))
	expected := uint8(*assertion.ExpectedVal)

	if actual != expected {
		return fmt.Errorf("value mismatch at $%04X: expected $%02X, got $%02X", addr, expected, actual)
	}

	return nil
}

func (ac *AssertionChecker) resolveSymbol(symbol string) (int, error) {
	// Try direct hex address
	if strings.HasPrefix(symbol, "$") {
		addr, err := strconv.ParseInt(symbol[1:], 16, 32)
		if err == nil {
			return int(addr), nil
		}
	}

	// Try symbol table
	if addr, ok := ac.Meta.Symbols[symbol]; ok {
		return addr, nil
	}

	return 0, fmt.Errorf("unknown symbol: %s", symbol)
}

func (ac *AssertionChecker) formatAssertion(assertion *MetaAssertion) string {
	switch assertion.Type {
	case "memory":
		return fmt.Sprintf("%s matches %s (%d bytes)", assertion.Symbol, assertion.ExpectedFile, assertion.Length)
	case "value":
		if assertion.ExpectedVal != nil {
			return fmt.Sprintf("%s == %d", assertion.Symbol, *assertion.ExpectedVal)
		}
		return fmt.Sprintf("%s == ???", assertion.Symbol)
	default:
		return fmt.Sprintf("unknown type: %s", assertion.Type)
	}
}

// Summary prints assertion results
func (ac *AssertionChecker) Summary() {
	if ac.Meta == nil || (ac.PassCount == 0 && ac.FailCount == 0) {
		return
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Printf("Assertion Summary: ")
	if ac.FailCount == 0 {
		fmt.Printf("✅ All %d assertions passed\n", ac.PassCount)
	} else {
		fmt.Printf("❌ %d passed, %d failed\n", ac.PassCount, ac.FailCount)
	}
	fmt.Println(strings.Repeat("=", 60))
}
