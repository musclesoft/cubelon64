package main

import (
	"fmt"
	"strings"
)

// Statement types
type Statement interface {
	statement()
}

type LabelStmt struct {
	Name string
}

func (*LabelStmt) statement() {}

type DirectiveStmt struct {
	Name string
	Args []string
}

func (*DirectiveStmt) statement() {}

type InstructionStmt struct {
	Mnemonic    string
	AddressMode AddressingMode
	Operand     string
}

func (*InstructionStmt) statement() {}

type ConstStmt struct {
	Name  string
	Value string
}

func (*ConstStmt) statement() {}

type AssertStmt struct {
	Symbol       string
	ExpectedFile string // for memory assertions
	Length       int    // for memory assertions
	FileOffset   int    // offset in expected file
	ExpectedVal  *int   // for value assertions (nil = memory assertion)
	CondSymbol   string // conditional check: only fire if this symbol...
	CondValue    *int   // ...equals this value
}

func (*AssertStmt) statement() {}

type AddressingMode int

const (
	Implied AddressingMode = iota
	Accumulator
	Immediate
	ZeroPage
	ZeroPageX
	ZeroPageY
	Absolute
	AbsoluteX
	AbsoluteY
	Indirect
	IndirectX
	IndirectY
	Relative
)

type Parser struct {
	lexer   *Lexer
	current *Token
	peek    *Token
}

func NewParser(lexer *Lexer) *Parser {
	p := &Parser{lexer: lexer}
	// Read two tokens to initialize current and peek
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) nextToken() error {
	var err error
	p.current = p.peek
	p.peek, err = p.lexer.NextToken()
	return err
}

func (p *Parser) NextStatement() (Statement, error) {
	// Skip empty lines and comments (but check for assertion comments first)
	for p.current != nil && (p.current.Type == NEWLINE || p.current.Type == COMMENT) {
		// Check if this is an assertion comment: ; @assert or ; @assert_if
		if p.current.Type == COMMENT {
			trimmed := strings.TrimSpace(p.current.Value)
			if strings.HasPrefix(trimmed, "; @assert_if") || strings.HasPrefix(trimmed, "; @assert") {
				// Parse as assertion
				return p.parseAssertionComment(p.current.Value)
			}
		}
		if err := p.nextToken(); err != nil {
			return nil, err
		}
	}

	if p.current == nil || p.current.Type == EOF {
		return nil, nil
	}

	var stmt Statement
	var err error

	switch p.current.Type {
	case DIRECTIVE:
		stmt, err = p.parseDirective()
	case IDENT:
		// Check for assertion macros first
		if p.current.Value == "assert_if" {
			stmt, err = p.parseAssertMacro(true)
		} else if p.current.Value == "assert" {
			stmt, err = p.parseAssertMacro(false)
		} else if p.peek != nil && p.peek.Type == COLON {
			stmt, err = p.parseLabel()
		} else if p.peek != nil && p.peek.Type == EQUALS {
			stmt, err = p.parseConst()
		} else {
			stmt, err = p.parseInstruction()
		}
	case COLON:
		// Handle ::label: (cheap local label in ca65)
		// Skip first colon
		p.nextToken()
		if p.current != nil && p.current.Type == COLON {
			// Skip second colon
			p.nextToken()
		}
		if p.current != nil && p.current.Type == IDENT {
			stmt, err = p.parseLabel()
		} else {
			return nil, fmt.Errorf("unexpected token after :: at line %d", p.current.Line)
		}
	default:
		return nil, fmt.Errorf("unexpected token %v at line %d", p.current.Type, p.current.Line)
	}

	// Consume trailing newline
	if p.current != nil && p.current.Type == NEWLINE {
		p.nextToken()
	}

	return stmt, err
}

func (p *Parser) parseLabel() (*LabelStmt, error) {
	name := p.current.Value
	p.nextToken() // consume identifier
	p.nextToken() // consume colon
	return &LabelStmt{Name: name}, nil
}

func (p *Parser) parseConst() (*ConstStmt, error) {
	name := p.current.Value
	p.nextToken() // consume identifier
	p.nextToken() // consume equals

	// Read the entire expression (until newline/EOF/comment)
	value := p.parseOperand()

	return &ConstStmt{Name: name, Value: value}, nil
}

func (p *Parser) parseDirective() (Statement, error) {
	name := p.current.Value
	p.nextToken()

	// Special handling for .assert and .assert_if
	if name == ".assert" {
		return p.parseAssert(false)
	}
	if name == ".assert_if" {
		return p.parseAssert(true)
	}

	var args []string

	// Parse arguments - for directives, read entire expressions as single arguments
	for p.current != nil && p.current.Type != NEWLINE && p.current.Type != EOF && p.current.Type != COMMENT {
		switch p.current.Type {
		case STRING:
			args = append(args, p.current.Value)
			p.nextToken()
		case IDENT, NUMBER:
			// Start of an expression - read until comma or end
			exprParts := []string{p.current.Value}
			p.nextToken()
			// Continue reading expression tokens
			for p.current != nil && p.current.Type != NEWLINE && p.current.Type != EOF &&
				p.current.Type != COMMENT && p.current.Type != COMMA && p.current.Type != COLON {
				switch p.current.Type {
				case STAR:
					exprParts = append(exprParts, "*")
				case PLUS:
					exprParts = append(exprParts, "+")
				case MINUS:
					exprParts = append(exprParts, "-")
				case SLASH:
					exprParts = append(exprParts, "/")
				case LPAREN:
					exprParts = append(exprParts, "(")
				case RPAREN:
					exprParts = append(exprParts, ")")
				case IDENT, NUMBER:
					exprParts = append(exprParts, p.current.Value)
				default:
					return nil, fmt.Errorf("unexpected token in directive expression at line %d: type=%v", p.current.Line, p.current.Type)
				}
				p.nextToken()
			}
			args = append(args, strings.Join(exprParts, " "))
		case COMMA:
			p.nextToken() // skip commas
		case COLON:
			// Handle ".import symbol: absolute"
			p.nextToken()
			if p.current.Type == IDENT {
				p.nextToken() // skip "absolute"
			}
		default:
			return nil, fmt.Errorf("unexpected token in directive at line %d: type=%v value=%q", p.current.Line, p.current.Type, p.current.Value)
		}
	}

	return &DirectiveStmt{Name: name, Args: args}, nil
}

func (p *Parser) parseAssertionComment(comment string) (*AssertStmt, error) {
	// Parse assertion from comment: ; @assert_if COND VAL SYMBOL "FILE" LEN OFFSET
	trimmed := strings.TrimSpace(comment)
	if !strings.HasPrefix(trimmed, "; @assert") {
		return nil, fmt.Errorf("not an assertion comment")
	}

	// Remove "; @" prefix
	content := strings.TrimPrefix(trimmed, "; @")

	// Split into fields
	fields := strings.Fields(content)
	if len(fields) < 3 {
		return nil, fmt.Errorf("invalid assertion comment: too few fields")
	}

	hasCondition := fields[0] == "assert_if"
	fieldIdx := 1

	var condSymbol string
	var condValue *int

	if hasCondition {
		if len(fields) < 5 {
			return nil, fmt.Errorf("invalid assert_if comment: too few fields")
		}
		condSymbol = fields[fieldIdx]
		fieldIdx++

		val, err := ParseNumber(fields[fieldIdx])
		if err != nil {
			return nil, fmt.Errorf("invalid condition value: %w", err)
		}
		condValue = &val
		fieldIdx++
	}

	// Symbol
	symbol := fields[fieldIdx]
	fieldIdx++

	// Second arg - need to handle quoted strings
	// Reconstruct the original line to properly parse quoted strings
	remaining := strings.TrimSpace(strings.TrimPrefix(content, strings.Join(fields[:fieldIdx], " ")))

	// Check if it starts with a quote (file path)
	if strings.HasPrefix(remaining, "\"") {
		// Extract quoted string
		endQuote := strings.Index(remaining[1:], "\"")
		if endQuote == -1 {
			return nil, fmt.Errorf("unterminated string in assertion comment")
		}
		file := remaining[1 : endQuote+1]
		remaining = strings.TrimSpace(remaining[endQuote+2:])

		// Parse length and optional offset
		parts := strings.Fields(remaining)
		if len(parts) < 1 {
			return nil, fmt.Errorf("expected length after file in assertion")
		}

		length, err := ParseNumber(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid length: %w", err)
		}

		fileOffset := 0
		if len(parts) >= 2 {
			fileOffset, err = ParseNumber(parts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid file offset: %w", err)
			}
		}

		// Advance past this statement
		p.nextToken()

		return &AssertStmt{
			Symbol:       symbol,
			ExpectedFile: file,
			Length:       length,
			FileOffset:   fileOffset,
			CondSymbol:   condSymbol,
			CondValue:    condValue,
		}, nil
	}

	// Value assertion
	val, err := ParseNumber(fields[fieldIdx])
	if err != nil {
		return nil, fmt.Errorf("invalid value: %w", err)
	}

	// Advance past this statement
	p.nextToken()

	return &AssertStmt{
		Symbol:     symbol,
		ExpectedVal: &val,
		CondSymbol: condSymbol,
		CondValue:  condValue,
	}, nil
}

func (p *Parser) parseAssertMacro(hasCondition bool) (*AssertStmt, error) {
	// assert_if COND, VAL, SYMBOL, "FILE", LENGTH, OFFSET
	// assert SYMBOL, "FILE", LENGTH, OFFSET
	// Parse comma-separated arguments
	p.nextToken() // skip "assert_if" or "assert"

	var args []string
	for p.current != nil && p.current.Type != NEWLINE && p.current.Type != EOF {
		if p.current.Type == COMMA {
			p.nextToken()
			continue
		}

		switch p.current.Type {
		case STRING:
			args = append(args, p.current.Value)
			p.nextToken()
		case IDENT, NUMBER:
			args = append(args, p.current.Value)
			p.nextToken()
		default:
			p.nextToken()
		}
	}

	// Parse based on hasCondition
	var condSymbol string
	var condValue *int
	argIdx := 0

	if hasCondition {
		if len(args) < 4 {
			return nil, fmt.Errorf("assert_if requires at least 4 arguments")
		}
		condSymbol = args[argIdx]
		argIdx++

		val, err := ParseNumber(args[argIdx])
		if err != nil {
			return nil, fmt.Errorf("invalid condition value: %w", err)
		}
		condValue = &val
		argIdx++
	}

	if argIdx >= len(args) {
		return nil, fmt.Errorf("missing symbol argument")
	}
	symbol := args[argIdx]
	argIdx++

	// Check if next arg is a file (string) or value (number)
	if argIdx >= len(args) {
		return nil, fmt.Errorf("missing expected file or value")
	}

	// If it starts with quote or contains /, it's a file path
	nextArg := args[argIdx]
	if strings.HasPrefix(nextArg, "\"") || strings.Contains(nextArg, "/") || strings.HasSuffix(nextArg, ".bin") {
		// Memory assertion
		file := strings.Trim(nextArg, "\"")
		argIdx++

		if argIdx >= len(args) {
			return nil, fmt.Errorf("missing length argument")
		}
		length, err := ParseNumber(args[argIdx])
		if err != nil {
			return nil, fmt.Errorf("invalid length: %w", err)
		}
		argIdx++

		fileOffset := 0
		if argIdx < len(args) {
			fileOffset, err = ParseNumber(args[argIdx])
			if err != nil {
				return nil, fmt.Errorf("invalid file offset: %w", err)
			}
		}

		return &AssertStmt{
			Symbol:       symbol,
			ExpectedFile: file,
			Length:       length,
			FileOffset:   fileOffset,
			CondSymbol:   condSymbol,
			CondValue:    condValue,
		}, nil
	}

	// Value assertion
	val, err := ParseNumber(nextArg)
	if err != nil {
		return nil, fmt.Errorf("invalid value: %w", err)
	}

	return &AssertStmt{
		Symbol:      symbol,
		ExpectedVal: &val,
		CondSymbol:  condSymbol,
		CondValue:   condValue,
	}, nil
}

func (p *Parser) parseAssert(hasCondition bool) (*AssertStmt, error) {
	// .assert_if COND_SYMBOL COND_VALUE SYMBOL FILE LENGTH [OFFSET]  (conditional memory assertion)
	// .assert_if COND_SYMBOL COND_VALUE SYMBOL VALUE                  (conditional value assertion)
	// .assert SYMBOL FILE LENGTH [OFFSET]                             (memory assertion)
	// .assert SYMBOL VALUE                                             (value assertion)

	var condSymbol string
	var condValue *int

	// Parse condition if present
	if hasCondition {
		if p.current == nil || (p.current.Type != IDENT && p.current.Type != NUMBER) {
			return nil, fmt.Errorf("expected condition symbol after .assert_if at line %d", p.current.Line)
		}
		condSymbol = p.current.Value
		p.nextToken()

		if p.current == nil || p.current.Type != NUMBER {
			return nil, fmt.Errorf("expected condition value after symbol in .assert_if at line %d", p.current.Line)
		}
		val, err := ParseNumber(p.current.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid condition value in .assert_if: %w", err)
		}
		condValue = &val
		p.nextToken()
	}

	if p.current == nil || (p.current.Type != IDENT && p.current.Type != NUMBER) {
		return nil, fmt.Errorf("expected symbol after .assert at line %d", p.current.Line)
	}

	symbol := p.current.Value
	p.nextToken()

	// Check second argument
	if p.current == nil || (p.current.Type != STRING && p.current.Type != NUMBER && p.current.Type != IDENT) {
		return nil, fmt.Errorf("expected file or value after symbol in .assert at line %d", p.current.Line)
	}

	// If it's a string, it's a file path (memory assertion)
	if p.current.Type == STRING {
		file := p.current.Value
		p.nextToken()

		// Expect length
		if p.current == nil || p.current.Type != NUMBER {
			return nil, fmt.Errorf("expected length after file in .assert at line %d", p.current.Line)
		}

		length, err := ParseNumber(p.current.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid length in .assert: %w", err)
		}
		p.nextToken()

		// Optional file offset
		fileOffset := 0
		if p.current != nil && p.current.Type == NUMBER {
			fileOffset, err = ParseNumber(p.current.Value)
			if err != nil {
				return nil, fmt.Errorf("invalid file offset in .assert: %w", err)
			}
			p.nextToken()
		}

		return &AssertStmt{
			Symbol:       symbol,
			ExpectedFile: file,
			Length:       length,
			FileOffset:   fileOffset,
			CondSymbol:   condSymbol,
			CondValue:    condValue,
		}, nil
	}

	// Otherwise it's a value assertion
	valueStr := p.current.Value
	p.nextToken()

	// Try to parse as number (will be resolved later if it's a symbol)
	value, err := ParseNumber(valueStr)
	if err != nil {
		// Might be a symbol reference, store as string for now
		// For simplicity, we'll require numeric literals in assertions for now
		return nil, fmt.Errorf("expected numeric value in .assert at line %d (got %q)", p.current.Line, valueStr)
	}

	return &AssertStmt{
		Symbol:      symbol,
		ExpectedVal: &value,
		CondSymbol:  condSymbol,
		CondValue:   condValue,
	}, nil
}

func (p *Parser) parseInstruction() (*InstructionStmt, error) {
	mnemonic := strings.ToUpper(p.current.Value)
	p.nextToken()

	inst := &InstructionStmt{
		Mnemonic: mnemonic,
	}

	// Parse addressing mode based on next tokens
	if p.current == nil || p.current.Type == NEWLINE || p.current.Type == EOF || p.current.Type == COMMENT {
		// Implied or accumulator - some instructions use accumulator mode when no operand
		// ASL, LSR, ROL, ROR without operand means operate on accumulator
		if mnemonic == "ASL" || mnemonic == "LSR" || mnemonic == "ROL" || mnemonic == "ROR" {
			inst.AddressMode = Accumulator
		} else {
			inst.AddressMode = Implied
		}
		return inst, nil
	}

	// Immediate: #value or #<value or #>value
	if p.current.Type == HASH {
		hashValue := p.current.Value // Could be "#", "#<", or "#>"
		if err := p.nextToken(); err != nil {
			return nil, fmt.Errorf("error reading token after # in instruction %s: %w", mnemonic, err)
		}
		if p.current == nil || (p.current.Type != NUMBER && p.current.Type != IDENT) {
			if p.current == nil {
				return nil, fmt.Errorf("unexpected end of file after # in instruction %s", mnemonic)
			} else {
				return nil, fmt.Errorf("expected number or identifier after # in instruction %s, got %v", mnemonic, p.current.Type)
			}
		}
		inst.AddressMode = Immediate
		inst.Operand = hashValue + p.current.Value
		if err := p.nextToken(); err != nil {
			return nil, err
		}
		return inst, nil
	}

	// Indirect: (value) or (value,X) or (value),Y
	if p.current.Type == LPAREN {
		p.nextToken()
		if p.current == nil {
			return nil, fmt.Errorf("unexpected end of file after ( in instruction %s", mnemonic)
		}
		operand := p.current.Value
		p.nextToken()

		if p.current != nil && p.current.Type == COMMA {
			// (value,X) - indexed indirect
			p.nextToken()
			if p.current.Type == IDENT && strings.ToUpper(p.current.Value) == "X" {
				inst.AddressMode = IndirectX
				inst.Operand = operand
				p.nextToken() // consume X
				p.nextToken() // consume )
				return inst, nil
			}
		} else if p.current.Type == RPAREN {
			p.nextToken()
			if p.current.Type == COMMA {
				// (value),Y - indirect indexed
				p.nextToken()
				if p.current.Type == IDENT && strings.ToUpper(p.current.Value) == "Y" {
					inst.AddressMode = IndirectY
					inst.Operand = operand
					p.nextToken()
					return inst, nil
				}
			} else {
				// (value) - indirect
				inst.AddressMode = Indirect
				inst.Operand = operand
				return inst, nil
			}
		}
	}

	// Absolute or zero page: value or value,X or value,Y
	operand := p.parseOperand()
	inst.Operand = operand

	// Check for indexed
	if p.current != nil && p.current.Type == COMMA {
		p.nextToken()
		if p.current.Type == IDENT {
			index := strings.ToUpper(p.current.Value)
			p.nextToken()

			// Determine if zero page or absolute
			// We'll decide in code generation based on operand value
			if index == "X" {
				inst.AddressMode = AbsoluteX // or ZeroPageX, decided later
			} else if index == "Y" {
				inst.AddressMode = AbsoluteY // or ZeroPageY, decided later
			}
		}
	} else {
		// Non-indexed
		inst.AddressMode = Absolute // or ZeroPage, decided later
	}

	return inst, nil
}

func (p *Parser) parseOperand() string {
	var parts []string

	for p.current != nil && p.current.Type != NEWLINE && p.current.Type != EOF &&
		p.current.Type != COMMENT && p.current.Type != COMMA {
		val := p.current.Value
		// For operators, use their symbol
		switch p.current.Type {
		case PLUS:
			val = "+"
		case MINUS:
			val = "-"
		case STAR:
			val = "*"
		case SLASH:
			val = "/"
		case LPAREN:
			val = "("
		case RPAREN:
			val = ")"
		}
		parts = append(parts, val)
		if err := p.nextToken(); err != nil {
			break
		}
	}

	return strings.Join(parts, " ")
}
