package parser

import (
	"fmt"
	"novalang/ast"
	"novalang/lexer"
	"strconv"
)

type ParseError struct {
	Line    int
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("ParseError at line %d: %s", e.Line, e.Message)
}

type Parser struct {
	tokens []lexer.Token
	pos    int
}

func New(tokens []lexer.Token) *Parser {
	return &Parser{tokens: tokens, pos: 0}
}

func (p *Parser) peek() lexer.Token         { return p.tokens[p.pos] }
func (p *Parser) peekType() lexer.TokenType { return p.tokens[p.pos].Type }

func (p *Parser) advance() lexer.Token {
	t := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return t
}

func (p *Parser) check(tt lexer.TokenType) bool { return p.peekType() == tt }
func (p *Parser) match(tt lexer.TokenType) bool {
	if p.check(tt) {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) expect(tt lexer.TokenType) (lexer.Token, error) {
	if p.check(tt) {
		return p.advance(), nil
	}
	t := p.peek()
	return t, &ParseError{Line: t.Line, Message: fmt.Sprintf("expected %s, got %s (%q)", tt, t.Type, t.Value)}
}

func (p *Parser) skipNewlines() {
	for p.check(lexer.TT_NEWLINE) {
		p.advance()
	}
}

func (p *Parser) expectNewline() error {
	if p.check(lexer.TT_EOF) {
		return nil
	}
	if p.check(lexer.TT_NEWLINE) {
		p.advance()
		return nil
	}
	t := p.peek()
	return &ParseError{Line: t.Line, Message: fmt.Sprintf("expected newline, got %s (%q)", t.Type, t.Value)}
}

// ─── Type parsing ─────────────────────────────────────────────────────────────

// parseTypeRef parses a full type reference.
// Syntax: int | str | bool | float | double | void | IDENT | [ElemType] | [ElemType, Size]
func (p *Parser) parseTypeRef() (*ast.TypeRef, error) {
	t := p.peek()
	switch t.Type {
	case lexer.TT_VOID:
		p.advance()
		return &ast.TypeRef{Kind: ast.TypeVoid}, nil
	case lexer.TT_INT:
		p.advance()
		return &ast.TypeRef{Kind: ast.TypeInt}, nil
	case lexer.TT_STR:
		p.advance()
		return &ast.TypeRef{Kind: ast.TypeStr}, nil
	case lexer.TT_BOOL:
		p.advance()
		return &ast.TypeRef{Kind: ast.TypeBool}, nil
	case lexer.TT_FLOAT:
		p.advance()
		return &ast.TypeRef{Kind: ast.TypeFloat}, nil
	case lexer.TT_DOUBLE:
		p.advance()
		return &ast.TypeRef{Kind: ast.TypeDouble}, nil
	case lexer.TT_IDENT:
		// user-defined struct or typedef alias
		p.advance()
		return &ast.TypeRef{Kind: ast.TypeStruct, Name: t.Value}, nil
	case lexer.TT_LBRACKET:
		// Array type: [ElemType] or [ElemType, Size]
		p.advance()
		elemType, err := p.parseTypeRef()
		if err != nil {
			return nil, err
		}
		size := -1
		if p.match(lexer.TT_COMMA) {
			sizeTok, err := p.expect(lexer.TT_INT_LIT)
			if err != nil {
				return nil, err
			}
			sz, _ := strconv.Atoi(sizeTok.Value)
			size = sz
		}
		if _, err := p.expect(lexer.TT_RBRACKET); err != nil {
			return nil, err
		}
		return &ast.TypeRef{Kind: ast.TypeArray, ElemType: elemType, Size: size}, nil
	}
	return nil, &ParseError{Line: t.Line, Message: fmt.Sprintf("expected type, got %s (%q)", t.Type, t.Value)}
}

// ─── Top-level ────────────────────────────────────────────────────────────────

func (p *Parser) Parse() (*ast.Program, error) {
	prog := &ast.Program{}
	p.skipNewlines()
	for !p.check(lexer.TT_EOF) {
		switch p.peekType() {
		case lexer.TT_IMPORT:
			decl, err := p.parseImport()
			if err != nil {
				return nil, err
			}
			prog.Imports = append(prog.Imports, decl)
			prog.TopLevel = append(prog.TopLevel, decl)

		case lexer.TT_TYPEDEF:
			decl, err := p.parseTypedef()
			if err != nil {
				return nil, err
			}
			prog.Typedefs = append(prog.Typedefs, decl)
			prog.TopLevel = append(prog.TopLevel, decl)

		case lexer.TT_STRUCT:
			decl, err := p.parseStructDecl()
			if err != nil {
				return nil, err
			}
			prog.Structs = append(prog.Structs, decl)
			prog.TopLevel = append(prog.TopLevel, decl)

		case lexer.TT_FUNC:
			fn, err := p.parseFuncDecl()
			if err != nil {
				return nil, err
			}
			prog.Functions = append(prog.Functions, fn)
			prog.TopLevel = append(prog.TopLevel, fn)

		default:
			t := p.peek()
			return nil, &ParseError{Line: t.Line, Message: fmt.Sprintf("unexpected top-level token %s (%q)", t.Type, t.Value)}
		}
		p.skipNewlines()
	}
	return prog, nil
}

func (p *Parser) parseImport() (*ast.ImportDecl, error) {
	line := p.peek().Line
	p.advance() // consume 'import'
	pathTok, err := p.expect(lexer.TT_STR_LIT)
	if err != nil {
		return nil, err
	}
	p.expectNewline()
	return &ast.ImportDecl{Path: pathTok.Value, Line: line}, nil
}

func (p *Parser) parseTypedef() (*ast.TypedefDecl, error) {
	line := p.peek().Line
	p.advance() // consume 'typedef'
	nameTok, err := p.expect(lexer.TT_IDENT)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_ASSIGN); err != nil {
		return nil, err
	}
	aliased, err := p.parseTypeRef()
	if err != nil {
		return nil, err
	}
	p.expectNewline()
	return &ast.TypedefDecl{Name: nameTok.Value, Aliased: aliased, Line: line}, nil
}

func (p *Parser) parseStructDecl() (*ast.StructDecl, error) {
	line := p.peek().Line
	p.advance() // consume 'struct'
	nameTok, err := p.expect(lexer.TT_IDENT)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_COLON); err != nil {
		return nil, err
	}
	p.expectNewline()
	p.skipNewlines()

	var fields []ast.StructField
	for !p.check(lexer.TT_ENDSTRUCT) && !p.check(lexer.TT_EOF) {
		fldName, err := p.expect(lexer.TT_IDENT)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.TT_COLON); err != nil {
			return nil, err
		}
		fldType, err := p.parseTypeRef()
		if err != nil {
			return nil, err
		}
		fields = append(fields, ast.StructField{Name: fldName.Value, Type: fldType, Line: fldName.Line})
		p.expectNewline()
		p.skipNewlines()
	}
	if _, err := p.expect(lexer.TT_ENDSTRUCT); err != nil {
		return nil, err
	}
	p.expectNewline()
	return &ast.StructDecl{Name: nameTok.Value, Fields: fields, Line: line}, nil
}

func (p *Parser) parseFuncDecl() (*ast.FuncDecl, error) {
	line := p.peek().Line
	p.advance() // consume 'func'

	nameTok, err := p.expect(lexer.TT_IDENT)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(lexer.TT_LPAREN); err != nil {
		return nil, err
	}

	var params []ast.Param
	for !p.check(lexer.TT_RPAREN) && !p.check(lexer.TT_EOF) {
		paramName, err := p.expect(lexer.TT_IDENT)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.TT_COLON); err != nil {
			return nil, err
		}
		paramType, err := p.parseTypeRef()
		if err != nil {
			return nil, err
		}
		params = append(params, ast.Param{Name: paramName.Value, Type: paramType})
		if !p.match(lexer.TT_COMMA) {
			break
		}
	}
	if _, err := p.expect(lexer.TT_RPAREN); err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_ARROW); err != nil {
		return nil, err
	}
	retType, err := p.parseTypeRef()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_COLON); err != nil {
		return nil, err
	}
	p.expectNewline()
	p.skipNewlines()

	body, err := p.parseBlock(lexer.TT_END)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_END); err != nil {
		return nil, err
	}
	p.expectNewline()

	return &ast.FuncDecl{
		Name: nameTok.Value, Params: params, ReturnType: retType, Body: body, Line: line,
	}, nil
}

func (p *Parser) parseBlock(stopAt ...lexer.TokenType) ([]ast.Stmt, error) {
	var stmts []ast.Stmt
	p.skipNewlines()
	for !p.check(lexer.TT_EOF) {
		stop := false
		for _, tt := range stopAt {
			if p.check(tt) {
				stop = true
				break
			}
		}
		if stop {
			break
		}
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			stmts = append(stmts, stmt)
		}
		p.skipNewlines()
	}
	return stmts, nil
}

// ─── Statements ───────────────────────────────────────────────────────────────

func (p *Parser) parseStmt() (ast.Stmt, error) {
	t := p.peek()
	switch t.Type {
	case lexer.TT_FUNC:
		return p.parseFuncDecl()
	case lexer.TT_STRUCT:
		return p.parseStructDecl()
	case lexer.TT_TYPEDEF:
		return p.parseTypedef()
	case lexer.TT_RETURN:
		return p.parseReturn()
	case lexer.TT_IF:
		return p.parseIf()
	case lexer.TT_WHILE:
		return p.parseWhile()
	case lexer.TT_FOR:
		return p.parseFor()
	case lexer.TT_PRINT:
		return p.parsePrint()
	case lexer.TT_UNDEF:
		return p.parseUndef()
	case lexer.TT_BREAK:
		p.advance()
		p.expectNewline()
		return &ast.BreakStmt{Line: t.Line}, nil
	case lexer.TT_CONTINUE:
		p.advance()
		p.expectNewline()
		return &ast.ContinueStmt{Line: t.Line}, nil
	case lexer.TT_IDENT:
		return p.parseIdentStmt()
	case lexer.TT_NEWLINE:
		p.advance()
		return nil, nil
	default:
		return nil, &ParseError{Line: t.Line, Message: fmt.Sprintf("unexpected token %s (%q)", t.Type, t.Value)}
	}
}

func (p *Parser) parseReturn() (ast.Stmt, error) {
	line := p.peek().Line
	p.advance()
	var val ast.Expr
	if !p.check(lexer.TT_NEWLINE) && !p.check(lexer.TT_EOF) {
		var err error
		val, err = p.parseExpr()
		if err != nil {
			return nil, err
		}
	}
	p.expectNewline()
	return &ast.ReturnStmt{Value: val, Line: line}, nil
}

func (p *Parser) parsePrint() (ast.Stmt, error) {
	line := p.peek().Line
	p.advance()
	if _, err := p.expect(lexer.TT_LPAREN); err != nil {
		return nil, err
	}
	var args []ast.Expr
	for !p.check(lexer.TT_RPAREN) && !p.check(lexer.TT_EOF) {
		arg, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		if !p.match(lexer.TT_COMMA) {
			break
		}
	}
	if _, err := p.expect(lexer.TT_RPAREN); err != nil {
		return nil, err
	}
	p.expectNewline()
	return &ast.PrintStmt{Args: args, Line: line}, nil
}

func (p *Parser) parseUndef() (ast.Stmt, error) {
	line := p.peek().Line
	p.advance()
	name, err := p.expect(lexer.TT_IDENT)
	if err != nil {
		return nil, err
	}
	p.expectNewline()
	return &ast.UndefStmt{Name: name.Value, Line: line}, nil
}

func (p *Parser) parseIf() (ast.Stmt, error) {
	line := p.peek().Line
	p.advance()
	if _, err := p.expect(lexer.TT_LPAREN); err != nil {
		return nil, err
	}
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_RPAREN); err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_COLON); err != nil {
		return nil, err
	}
	p.expectNewline()
	p.skipNewlines()

	then, err := p.parseBlock(lexer.TT_ENDIF, lexer.TT_ELIF, lexer.TT_ELSE)
	if err != nil {
		return nil, err
	}

	var elseIfs []ast.ElseIfClause
	var elseBody []ast.Stmt

	for p.check(lexer.TT_ELIF) {
		p.advance()
		if _, err := p.expect(lexer.TT_LPAREN); err != nil {
			return nil, err
		}
		eic, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.TT_RPAREN); err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.TT_COLON); err != nil {
			return nil, err
		}
		p.expectNewline()
		p.skipNewlines()
		eib, err := p.parseBlock(lexer.TT_ENDIF, lexer.TT_ELIF, lexer.TT_ELSE)
		if err != nil {
			return nil, err
		}
		elseIfs = append(elseIfs, ast.ElseIfClause{Condition: eic, Body: eib})
	}
	if p.check(lexer.TT_ELSE) {
		p.advance()
		if _, err := p.expect(lexer.TT_COLON); err != nil {
			return nil, err
		}
		p.expectNewline()
		p.skipNewlines()
		elseBody, err = p.parseBlock(lexer.TT_ENDIF)
		if err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.TT_ENDIF); err != nil {
		return nil, err
	}
	p.expectNewline()
	return &ast.IfStmt{Condition: cond, Then: then, ElseIfs: elseIfs, Else: elseBody, Line: line}, nil
}

func (p *Parser) parseWhile() (ast.Stmt, error) {
	line := p.peek().Line
	p.advance()
	if _, err := p.expect(lexer.TT_LPAREN); err != nil {
		return nil, err
	}
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_RPAREN); err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_COLON); err != nil {
		return nil, err
	}
	p.expectNewline()
	p.skipNewlines()
	body, err := p.parseBlock(lexer.TT_ENDWHILE)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_ENDWHILE); err != nil {
		return nil, err
	}
	p.expectNewline()
	return &ast.WhileStmt{Condition: cond, Body: body, Line: line}, nil
}

func (p *Parser) parseFor() (ast.Stmt, error) {
	line := p.peek().Line
	p.advance()
	if _, err := p.expect(lexer.TT_LPAREN); err != nil {
		return nil, err
	}
	init, err := p.parseIdentStmt()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_COMMA); err != nil {
		return nil, err
	}
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_COMMA); err != nil {
		return nil, err
	}
	post, err := p.parseIdentStmt()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_RPAREN); err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_COLON); err != nil {
		return nil, err
	}
	p.expectNewline()
	p.skipNewlines()
	body, err := p.parseBlock(lexer.TT_ENDFOR)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_ENDFOR); err != nil {
		return nil, err
	}
	p.expectNewline()
	return &ast.ForStmt{Init: init, Condition: cond, Post: post, Body: body, Line: line}, nil
}

// parseIdentStmt handles: var decl, assignment (including compound and lvalue chains), or call stmt
func (p *Parser) parseIdentStmt() (ast.Stmt, error) {
	name, err := p.expect(lexer.TT_IDENT)
	if err != nil {
		return nil, err
	}
	line := name.Line

	// var decl: name: type = expr
	if p.check(lexer.TT_COLON) {
		p.advance()
		varType, err := p.parseTypeRef()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.TT_ASSIGN); err != nil {
			return nil, err
		}
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		p.expectNewline()
		return &ast.VarDecl{Name: name.Value, Type: varType, Value: val, Line: line}, nil
	}

	// Build lvalue + expr chains (for index/field access chains before assignment)
	var lval ast.LValue = &ast.LValueIdent{Name: name.Value, Line: line}
	var baseExpr ast.Expr = &ast.Ident{Name: name.Value, Line: line}

	for p.check(lexer.TT_LBRACKET) || p.check(lexer.TT_DOT) {
		if p.check(lexer.TT_LBRACKET) {
			p.advance()
			idx, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.TT_RBRACKET); err != nil {
				return nil, err
			}
			lval = &ast.LValueIndex{Object: baseExpr, Index: idx, Line: line}
			baseExpr = &ast.IndexExpr{Object: baseExpr, Index: idx, Line: line}
		} else {
			p.advance()
			fld, err := p.expect(lexer.TT_IDENT)
			if err != nil {
				return nil, err
			}
			lval = &ast.LValueField{Object: baseExpr, Field: fld.Value, Line: fld.Line}
			baseExpr = &ast.FieldExpr{Object: baseExpr, Field: fld.Value, Line: fld.Line}
		}
	}

	// Check for assignment operators
	op := ""
	switch p.peekType() {
	case lexer.TT_ASSIGN:
		op = "="
	case lexer.TT_PLUS_ASSIGN:
		op = "+="
	case lexer.TT_MINUS_ASSIGN:
		op = "-="
	case lexer.TT_STAR_ASSIGN:
		op = "*="
	case lexer.TT_SLASH_ASSIGN:
		op = "/="
	}
	if op != "" {
		p.advance()
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		p.expectNewline()
		return &ast.Assign{Target: lval, Value: val, Op: op, Line: line}, nil
	}

	// function call used as statement (name must be plain ident)
	if p.check(lexer.TT_LPAREN) {
		p.advance()
		var args []ast.Expr
		for !p.check(lexer.TT_RPAREN) && !p.check(lexer.TT_EOF) {
			arg, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			if !p.match(lexer.TT_COMMA) {
				break
			}
		}
		if _, err := p.expect(lexer.TT_RPAREN); err != nil {
			return nil, err
		}
		p.expectNewline()
		return &ast.ExprStmt{Expr: &ast.CallExpr{Callee: name.Value, Args: args, Line: line}, Line: line}, nil
	}

	return nil, &ParseError{Line: line, Message: fmt.Sprintf("expected ':' or assignment operator after %q", name.Value)}
}

// ─── Expressions ──────────────────────────────────────────────────────────────

func (p *Parser) parseExpr() (ast.Expr, error) { return p.parseOr() }

func (p *Parser) parseOr() (ast.Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.TT_OR) {
		op := p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &ast.BinOp{Op: op.Value, Left: left, Right: right, Line: op.Line}
	}
	return left, nil
}

func (p *Parser) parseAnd() (ast.Expr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.TT_AND) {
		op := p.advance()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &ast.BinOp{Op: op.Value, Left: left, Right: right, Line: op.Line}
	}
	return left, nil
}

func (p *Parser) parseNot() (ast.Expr, error) {
	if p.check(lexer.TT_NOT) {
		op := p.advance()
		operand, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryOp{Op: "not", Operand: operand, Line: op.Line}, nil
	}
	return p.parseComparison()
}

func (p *Parser) parseComparison() (ast.Expr, error) {
	left, err := p.parseAddSub()
	if err != nil {
		return nil, err
	}
	for {
		tt := p.peekType()
		if tt != lexer.TT_LT && tt != lexer.TT_GT && tt != lexer.TT_GTE &&
			tt != lexer.TT_LTE && tt != lexer.TT_EQ && tt != lexer.TT_NEQ {
			break
		}
		op := p.advance()
		right, err := p.parseAddSub()
		if err != nil {
			return nil, err
		}
		left = &ast.BinOp{Op: op.Value, Left: left, Right: right, Line: op.Line}
	}
	return left, nil
}

func (p *Parser) parseAddSub() (ast.Expr, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.TT_PLUS) || p.check(lexer.TT_MINUS) {
		op := p.advance()
		right, err := p.parseMulDiv()
		if err != nil {
			return nil, err
		}
		left = &ast.BinOp{Op: op.Value, Left: left, Right: right, Line: op.Line}
	}
	return left, nil
}

func (p *Parser) parseMulDiv() (ast.Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.TT_STAR) || p.check(lexer.TT_SLASH) || p.check(lexer.TT_MOD) {
		op := p.advance()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &ast.BinOp{Op: op.Value, Left: left, Right: right, Line: op.Line}
	}
	return left, nil
}

func (p *Parser) parseUnary() (ast.Expr, error) {
	if p.check(lexer.TT_MINUS) {
		op := p.advance()
		operand, err := p.parsePostfix()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryOp{Op: "-", Operand: operand, Line: op.Line}, nil
	}
	return p.parsePostfix()
}

// parsePostfix handles arr[i], obj.field, obj.method() chains
func (p *Parser) parsePostfix() (ast.Expr, error) {
	expr, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}

	for {
		if p.check(lexer.TT_LBRACKET) {
			bracketLine := p.peek().Line
			p.advance()
			idx, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.TT_RBRACKET); err != nil {
				return nil, err
			}
			expr = &ast.IndexExpr{Object: expr, Index: idx, Line: bracketLine}
		} else if p.check(lexer.TT_DOT) {
			p.advance()
			fld, err := p.expect(lexer.TT_IDENT)
			if err != nil {
				return nil, err
			}
			if p.check(lexer.TT_LPAREN) {
				p.advance()
				var args []ast.Expr
				for !p.check(lexer.TT_RPAREN) && !p.check(lexer.TT_EOF) {
					arg, err := p.parseExpr()
					if err != nil {
						return nil, err
					}
					args = append(args, arg)
					if !p.match(lexer.TT_COMMA) {
						break
					}
				}
				if _, err := p.expect(lexer.TT_RPAREN); err != nil {
					return nil, err
				}
				expr = &ast.MethodCallExpr{Object: expr, Method: fld.Value, Args: args, Line: fld.Line}
			} else {
				expr = &ast.FieldExpr{Object: expr, Field: fld.Value, Line: fld.Line}
			}
		} else {
			break
		}
	}
	return expr, nil
}

func (p *Parser) parsePrimary() (ast.Expr, error) {
	t := p.peek()
	switch t.Type {
	case lexer.TT_INT_LIT:
		p.advance()
		v, _ := strconv.ParseInt(t.Value, 10, 64)
		return &ast.IntLit{Value: v, Line: t.Line}, nil

	case lexer.TT_FLOAT_LIT:
		p.advance()
		v, _ := strconv.ParseFloat(t.Value, 64)
		return &ast.FloatLit{Value: v, Line: t.Line}, nil

	case lexer.TT_STR_LIT:
		p.advance()
		return &ast.StrLit{Value: t.Value, Line: t.Line}, nil

	case lexer.TT_TRUE:
		p.advance()
		return &ast.BoolLit{Value: true, Line: t.Line}, nil

	case lexer.TT_FALSE:
		p.advance()
		return &ast.BoolLit{Value: false, Line: t.Line}, nil

	case lexer.TT_INPUT:
		p.advance()
		if _, err := p.expect(lexer.TT_LPAREN); err != nil {
			return nil, err
		}
		var prompt ast.Expr
		if !p.check(lexer.TT_RPAREN) {
			var err error
			prompt, err = p.parseExpr()
			if err != nil {
				return nil, err
			}
		}
		if _, err := p.expect(lexer.TT_RPAREN); err != nil {
			return nil, err
		}
		return &ast.InputExpr{Prompt: prompt, Line: t.Line}, nil

	case lexer.TT_LBRACKET:
		// Array literal: [e1, e2, ...]
		p.advance()
		var elems []ast.Expr
		for !p.check(lexer.TT_RBRACKET) && !p.check(lexer.TT_EOF) {
			e, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			elems = append(elems, e)
			if !p.match(lexer.TT_COMMA) {
				break
			}
		}
		if _, err := p.expect(lexer.TT_RBRACKET); err != nil {
			return nil, err
		}
		return &ast.ArrayLit{Elements: elems, Line: t.Line}, nil

	case lexer.TT_IDENT:
		p.advance()

		// Built-in: len(expr)
		if t.Value == "len" && p.check(lexer.TT_LPAREN) {
			p.advance()
			arg, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.TT_RPAREN); err != nil {
				return nil, err
			}
			return &ast.LenExpr{Arg: arg, Line: t.Line}, nil
		}

		// Built-in: append(arr, val)
		if t.Value == "append" && p.check(lexer.TT_LPAREN) {
			p.advance()
			arr, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.TT_COMMA); err != nil {
				return nil, err
			}
			val, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.TT_RPAREN); err != nil {
				return nil, err
			}
			return &ast.AppendExpr{Array: arr, Value: val, Line: t.Line}, nil
		}

		// Function call
		if p.check(lexer.TT_LPAREN) {
			p.advance()
			var args []ast.Expr
			for !p.check(lexer.TT_RPAREN) && !p.check(lexer.TT_EOF) {
				arg, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
				if !p.match(lexer.TT_COMMA) {
					break
				}
			}
			if _, err := p.expect(lexer.TT_RPAREN); err != nil {
				return nil, err
			}
			return &ast.CallExpr{Callee: t.Value, Args: args, Line: t.Line}, nil
		}

		return &ast.Ident{Name: t.Value, Line: t.Line}, nil

	case lexer.TT_LPAREN:
		p.advance()
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.TT_RPAREN); err != nil {
			return nil, err
		}
		return inner, nil
	}

	return nil, &ParseError{Line: t.Line, Message: fmt.Sprintf("unexpected token in expression: %s (%q)", t.Type, t.Value)}
}
