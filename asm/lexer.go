package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
)

type TokenType int

const (
	// Special tokens
	EOF TokenType = iota
	NEWLINE
	COMMENT

	// Identifiers and literals
	IDENT     // label, instruction, symbol
	NUMBER    // decimal or hex number
	STRING    // quoted string

	// Operators
	COLON     // :
	COMMA     // ,
	HASH      // #
	LPAREN    // (
	RPAREN    // )
	PLUS      // +
	MINUS     // -
	STAR      // *
	SLASH     // /
	EQUALS    // =

	// Directives (start with .)
	DIRECTIVE // .include, .import, .export, .segment, .byte, .word
)

type Token struct {
	Type   TokenType
	Value  string
	Line   int
	Column int
}

type Lexer struct {
	reader *bufio.Reader
	line   int
	column int
	ch     rune
	eof    bool
}

func NewLexer(r io.Reader) *Lexer {
	l := &Lexer{
		reader: bufio.NewReader(r),
		line:   1,
		column: 0,
	}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	ch, _, err := l.reader.ReadRune()
	if err != nil {
		l.eof = true
		l.ch = 0
		return
	}
	l.ch = ch
	l.column++
}

func (l *Lexer) peekChar() rune {
	bytes, err := l.reader.Peek(1)
	if err != nil || len(bytes) == 0 {
		return 0
	}
	return rune(bytes[0])
}

func (l *Lexer) skipWhitespace() {
	for !l.eof && (l.ch == ' ' || l.ch == '\t' || l.ch == '\r') {
		l.readChar()
	}
}

func (l *Lexer) readNumber() string {
	var sb strings.Builder

	// Check for hex prefix $
	if l.ch == '$' {
		sb.WriteRune(l.ch)
		l.readChar()
		for !l.eof && isHexDigit(l.ch) {
			sb.WriteRune(l.ch)
			l.readChar()
		}
	} else {
		// Decimal number
		for !l.eof && unicode.IsDigit(l.ch) {
			sb.WriteRune(l.ch)
			l.readChar()
		}
	}

	return sb.String()
}

func (l *Lexer) readIdentifier() string {
	var sb strings.Builder

	// Identifiers can start with letter, _, or @
	// @ is for local labels
	for !l.eof && (unicode.IsLetter(l.ch) || unicode.IsDigit(l.ch) || l.ch == '_' || l.ch == '@') {
		sb.WriteRune(l.ch)
		l.readChar()
	}

	return sb.String()
}

func (l *Lexer) readString() (string, error) {
	var sb strings.Builder
	quote := l.ch
	l.readChar() // skip opening quote

	for !l.eof && l.ch != quote {
		if l.ch == '\n' {
			return "", fmt.Errorf("unterminated string at line %d", l.line)
		}
		if l.ch == '\\' {
			l.readChar()
			if l.eof {
				return "", fmt.Errorf("unterminated string at line %d", l.line)
			}
			// Simple escape sequences
			switch l.ch {
			case 'n':
				sb.WriteRune('\n')
			case 't':
				sb.WriteRune('\t')
			case '\\':
				sb.WriteRune('\\')
			case '"':
				sb.WriteRune('"')
			case '\'':
				sb.WriteRune('\'')
			default:
				sb.WriteRune(l.ch)
			}
		} else {
			sb.WriteRune(l.ch)
		}
		l.readChar()
	}

	if l.eof {
		return "", fmt.Errorf("unterminated string at line %d", l.line)
	}

	l.readChar() // skip closing quote
	return sb.String(), nil
}

func (l *Lexer) NextToken() (*Token, error) {
	l.skipWhitespace()

	tok := &Token{
		Line:   l.line,
		Column: l.column,
	}

	if l.eof {
		tok.Type = EOF
		return tok, nil
	}

	switch l.ch {
	case '\n':
		tok.Type = NEWLINE
		l.readChar()
		l.line++
		l.column = 0
		return tok, nil

	case ';':
		// Read comment until end of line
		var sb strings.Builder
		for !l.eof && l.ch != '\n' {
			sb.WriteRune(l.ch)
			l.readChar()
		}
		tok.Type = COMMENT
		tok.Value = sb.String()
		return tok, nil

	case ':':
		tok.Type = COLON
		l.readChar()
		return tok, nil

	case ',':
		tok.Type = COMMA
		l.readChar()
		return tok, nil

	case '#':
		tok.Type = HASH
		tok.Value = "#"
		l.readChar()
		// Check for #< or #> (low/high byte operators)
		if l.ch == '<' || l.ch == '>' {
			tok.Value += string(l.ch)
			l.readChar()
		}
		return tok, nil

	case '(':
		tok.Type = LPAREN
		l.readChar()
		return tok, nil

	case ')':
		tok.Type = RPAREN
		l.readChar()
		return tok, nil

	case '+':
		tok.Type = PLUS
		tok.Value = "+"
		l.readChar()
		return tok, nil

	case '-':
		tok.Type = MINUS
		tok.Value = "-"
		l.readChar()
		return tok, nil

	case '*':
		tok.Type = STAR
		l.readChar()
		return tok, nil

	case '/':
		tok.Type = SLASH
		l.readChar()
		return tok, nil

	case '=':
		tok.Type = EQUALS
		l.readChar()
		return tok, nil

	case '"', '\'':
		str, err := l.readString()
		if err != nil {
			return nil, err
		}
		tok.Type = STRING
		tok.Value = str
		return tok, nil

	case '.':
		// Directive
		l.readChar() // skip the '.'
		tok.Type = DIRECTIVE
		tok.Value = "." + l.readIdentifier()
		return tok, nil

	case '$':
		// Hex number
		tok.Type = NUMBER
		tok.Value = l.readNumber()
		return tok, nil

	default:
		if unicode.IsDigit(l.ch) {
			tok.Type = NUMBER
			tok.Value = l.readNumber()
			return tok, nil
		} else if unicode.IsLetter(l.ch) || l.ch == '_' || l.ch == '@' {
			tok.Type = IDENT
			tok.Value = l.readIdentifier()
			return tok, nil
		} else {
			return nil, fmt.Errorf("unexpected character '%c' at line %d column %d", l.ch, l.line, l.column)
		}
	}
}

func isHexDigit(ch rune) bool {
	return unicode.IsDigit(ch) || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

// ParseNumber parses a number token value (decimal or hex with $ prefix)
func ParseNumber(s string) (int, error) {
	if strings.HasPrefix(s, "$") {
		// Hex number
		val, err := strconv.ParseInt(s[1:], 16, 64)
		return int(val), err
	}
	// Decimal number
	val, err := strconv.ParseInt(s, 10, 64)
	return int(val), err
}
