package lexer

import (
	"fmt"
	"strings"
)

// TokenType represents the type of a lexical token
type TokenType int

const (
	// Literals
	TT_INT_LIT TokenType = iota
	TT_FLOAT_LIT
	TT_STR_LIT
	TT_BOOL_LIT

	// Types
	TT_VOID
	TT_INT
	TT_STR
	TT_BOOL
	TT_FLOAT
	TT_DOUBLE

	// Keywords
	TT_FUNC
	TT_RETURN
	TT_IF
	TT_ELSE
	TT_ELIF
	TT_WHILE
	TT_FOR
	TT_UNDEF
	TT_AND
	TT_OR
	TT_TRUE
	TT_FALSE
	TT_PRINT
	TT_INPUT

	// Block terminators
	TT_END
	TT_ENDIF
	TT_ENDFOR
	TT_ENDWHILE
	TT_ENDSTRUCT

	// Struct / Typedef / Import
	TT_STRUCT
	TT_TYPEDEF
	TT_IMPORT

	// Array / index
	TT_LBRACKET
	TT_RBRACKET

	// Extra keywords
	TT_BREAK
	TT_CONTINUE
	TT_NOT
	TT_MOD // %
	TT_DOT // .

	// Compound assignment
	TT_PLUS_ASSIGN  // +=
	TT_MINUS_ASSIGN // -=
	TT_STAR_ASSIGN  // *=
	TT_SLASH_ASSIGN // /=

	// Symbols
	TT_LPAREN
	TT_RPAREN
	TT_COLON
	TT_COMMA
	TT_ARROW
	TT_ASSIGN

	// Comparison operators
	TT_LT
	TT_GT
	TT_GTE
	TT_LTE
	TT_EQ
	TT_NEQ

	// Arithmetic operators
	TT_PLUS
	TT_MINUS
	TT_STAR
	TT_SLASH

	// Special
	TT_IDENT
	TT_NEWLINE
	TT_EOF
)

var tokenNames = map[TokenType]string{
	TT_INT_LIT: "INT_LIT", TT_FLOAT_LIT: "FLOAT_LIT", TT_STR_LIT: "STR_LIT",
	TT_BOOL_LIT: "BOOL_LIT", TT_VOID: "void", TT_INT: "int", TT_STR: "str",
	TT_BOOL: "bool", TT_FLOAT: "float", TT_DOUBLE: "double", TT_FUNC: "func",
	TT_RETURN: "return", TT_IF: "if", TT_ELSE: "else", TT_ELIF: "elif",
	TT_WHILE: "while", TT_FOR: "for", TT_UNDEF: "undef", TT_AND: "and",
	TT_OR: "or", TT_NOT: "not", TT_TRUE: "true", TT_FALSE: "false",
	TT_PRINT: "print", TT_INPUT: "input",
	TT_END: "end", TT_ENDIF: "endif", TT_ENDFOR: "endfor",
	TT_ENDWHILE: "endwhile", TT_ENDSTRUCT: "endstruct",
	TT_STRUCT: "struct", TT_TYPEDEF: "typedef", TT_IMPORT: "import",
	TT_BREAK: "break", TT_CONTINUE: "continue",
	TT_LBRACKET: "[", TT_RBRACKET: "]", TT_DOT: ".", TT_MOD: "%",
	TT_PLUS_ASSIGN: "+=", TT_MINUS_ASSIGN: "-=", TT_STAR_ASSIGN: "*=", TT_SLASH_ASSIGN: "/=",
	TT_LPAREN: "(", TT_RPAREN: ")", TT_COLON: ":",
	TT_COMMA: ",", TT_ARROW: "->", TT_ASSIGN: "=", TT_LT: "<", TT_GT: ">",
	TT_GTE: ">=", TT_LTE: "<=", TT_EQ: "==", TT_NEQ: "!=", TT_PLUS: "+",
	TT_MINUS: "-", TT_STAR: "*", TT_SLASH: "/", TT_IDENT: "IDENT",
	TT_NEWLINE: "NEWLINE", TT_EOF: "EOF",
}

func (tt TokenType) String() string {
	if s, ok := tokenNames[tt]; ok {
		return s
	}
	return fmt.Sprintf("TT(%d)", int(tt))
}

var keywords = map[string]TokenType{
	"void": TT_VOID, "int": TT_INT, "str": TT_STR, "bool": TT_BOOL,
	"float": TT_FLOAT, "double": TT_DOUBLE, "func": TT_FUNC,
	"return": TT_RETURN, "if": TT_IF, "else": TT_ELSE, "elif": TT_ELIF,
	"while": TT_WHILE, "for": TT_FOR, "undef": TT_UNDEF,
	"and": TT_AND, "or": TT_OR, "not": TT_NOT,
	"true": TT_TRUE, "false": TT_FALSE,
	"print": TT_PRINT, "input": TT_INPUT,
	"end": TT_END, "endif": TT_ENDIF, "endfor": TT_ENDFOR, "endwhile": TT_ENDWHILE,
	"endstruct": TT_ENDSTRUCT,
	"struct":    TT_STRUCT,
	"typedef":   TT_TYPEDEF,
	"import":    TT_IMPORT,
	"break":     TT_BREAK,
	"continue":  TT_CONTINUE,
}

// Token is a lexical token with type, value, and source location
type Token struct {
	Type  TokenType
	Value string
	Line  int
}

func (t Token) String() string {
	return fmt.Sprintf("Token(%s, %q, line=%d)", t.Type, t.Value, t.Line)
}

// LexError represents a lexical error
type LexError struct {
	Line    int
	Message string
}

func (e *LexError) Error() string {
	return fmt.Sprintf("LexError at line %d: %s", e.Line, e.Message)
}

// Tokenize converts source code into a slice of tokens
func Tokenize(source string) ([]Token, error) {
	var tokens []Token
	runes := []rune(source)
	n := len(runes)
	i := 0
	line := 1

	emit := func(tt TokenType, val string) {
		tokens = append(tokens, Token{Type: tt, Value: val, Line: line})
	}
	emitNL := func() {
		if len(tokens) > 0 && tokens[len(tokens)-1].Type != TT_NEWLINE {
			emit(TT_NEWLINE, "\n")
		}
	}

	for i < n {
		ch := runes[i]

		// Skip spaces and tabs
		if ch == ' ' || ch == '\t' || ch == '\r' {
			i++
			continue
		}

		// Comments
		if ch == '#' {
			for i < n && runes[i] != '\n' {
				i++
			}
			continue
		}

		// Newline
		if ch == '\n' {
			emitNL()
			line++
			i++
			continue
		}

		// String literal
		if ch == '"' {
			i++
			var sb strings.Builder
			for i < n && runes[i] != '"' {
				if runes[i] == '\\' && i+1 < n {
					i++
					switch runes[i] {
					case 'n':
						sb.WriteRune('\n')
					case 't':
						sb.WriteRune('\t')
					case '"':
						sb.WriteRune('"')
					case '\\':
						sb.WriteRune('\\')
					default:
						sb.WriteRune('\\')
						sb.WriteRune(runes[i])
					}
				} else {
					sb.WriteRune(runes[i])
				}
				i++
			}
			if i >= n {
				return nil, &LexError{Line: line, Message: "unterminated string literal"}
			}
			i++ // consume closing "
			emit(TT_STR_LIT, sb.String())
			continue
		}

		// Number literal
		if ch >= '0' && ch <= '9' {
			j := i
			isFloat := false
			for j < n && runes[j] >= '0' && runes[j] <= '9' {
				j++
			}
			if j < n && runes[j] == '.' {
				isFloat = true
				j++
				for j < n && runes[j] >= '0' && runes[j] <= '9' {
					j++
				}
			}
			numStr := string(runes[i:j])
			if isFloat {
				emit(TT_FLOAT_LIT, numStr)
			} else {
				emit(TT_INT_LIT, numStr)
			}
			i = j
			continue
		}

		// Identifier or keyword
		if ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			j := i
			for j < n && (runes[j] == '_' || (runes[j] >= 'a' && runes[j] <= 'z') ||
				(runes[j] >= 'A' && runes[j] <= 'Z') || (runes[j] >= '0' && runes[j] <= '9')) {
				j++
			}
			word := string(runes[i:j])
			if tt, ok := keywords[word]; ok {
				emit(tt, word)
			} else {
				emit(TT_IDENT, word)
			}
			i = j
			continue
		}

		// Two-character operators
		if i+1 < n {
			two := string(runes[i : i+2])
			switch two {
			case "->":
				emit(TT_ARROW, "->"); i += 2; continue
			case ">=":
				emit(TT_GTE, ">="); i += 2; continue
			case "<=", "=<":
				emit(TT_LTE, two); i += 2; continue
			case "!=":
				emit(TT_NEQ, "!="); i += 2; continue
			case "==":
				emit(TT_EQ, "=="); i += 2; continue
			case "+=":
				emit(TT_PLUS_ASSIGN, "+="); i += 2; continue
			case "-=":
				emit(TT_MINUS_ASSIGN, "-="); i += 2; continue
			case "*=":
				emit(TT_STAR_ASSIGN, "*="); i += 2; continue
			case "/=":
				emit(TT_SLASH_ASSIGN, "/="); i += 2; continue
			}
		}

		// Single-character tokens
		switch ch {
		case '(':
			emit(TT_LPAREN, "(")
		case ')':
			emit(TT_RPAREN, ")")
		case '[':
			emit(TT_LBRACKET, "[")
		case ']':
			emit(TT_RBRACKET, "]")
		case ':':
			emit(TT_COLON, ":")
		case ',':
			emit(TT_COMMA, ",")
		case '=':
			emit(TT_ASSIGN, "=")
		case '+':
			emit(TT_PLUS, "+")
		case '-':
			emit(TT_MINUS, "-")
		case '*':
			emit(TT_STAR, "*")
		case '/':
			emit(TT_SLASH, "/")
		case '%':
			emit(TT_MOD, "%")
		case '<':
			emit(TT_LT, "<")
		case '>':
			emit(TT_GT, ">")
		case '.':
			emit(TT_DOT, ".")
		default:
			return nil, &LexError{Line: line, Message: fmt.Sprintf("unexpected character %q", ch)}
		}
		i++
	}

	emit(TT_EOF, "")
	return tokens, nil
}
