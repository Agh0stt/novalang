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

func (p *Parser) peek() lexer.Token {
	return p.tokens[p.pos]
}

func (p *Parser) peekType() lexer.TokenType {
	return p.tokens[p.pos].Type
}

func (p *Parser) advance() lexer.Token {
	t := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return t
}

func (p *Parser) check(tt lexer.TokenType) bool {
	return p.peekType() == tt
}

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
	return t, &ParseError{
		Line:    t.Line,
		Message: fmt.Sprintf("expected %s, got %s (%q)", tt, t.Type, t.Value),
	}
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

func (p *Parser) parseType() (ast.TypeKind, error) {
	t := p.peek()
	switch t.Type {
	case lexer.TT_VOID:
		p.advance()
		return ast.TypeVoid, nil
	case lexer.TT_INT:
		p.advance()
		return ast.TypeInt, nil
	case lexer.TT_STR:
		p.advance()
		return ast.TypeStr, nil
	case lexer.TT_BOOL:
		p.advance()
		return ast.TypeBool, nil
	case lexer.TT_FLOAT:
		p.advance()
		return ast.TypeFloat, nil
	case lexer.TT_DOUBLE:
		p.advance()
		return ast.TypeDouble, nil
	}
	return 0, &ParseError{Line: t.Line, Message: fmt.Sprintf("expected type, got %s (%q)", t.Type, t.Value)}
}

// ─── Top-level ────────────────────────────────────────────────────────────────

func (p *Parser) Parse() (*ast.Program, error) {
	prog := &ast.Program{}
	p.skipNewlines()
	for !p.check(lexer.TT_EOF) {
		fn, err := p.parseFuncDecl()
		if err != nil {
			return nil, err
		}
		prog.Functions = append(prog.Functions, fn)
		p.skipNewlines()
	}
	return prog, nil
}

func (p *Parser) parseFuncDecl() (*ast.FuncDecl, error) {
	line := p.peek().Line
	if _, err := p.expect(lexer.TT_FUNC); err != nil {
		return nil, err
	}

	nameTok, err := p.expect(lexer.TT_IDENT)
	if err != nil {
		// allow 'main' which is just an ident
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
		paramType, err := p.parseType()
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

	retType, err := p.parseType()
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(lexer.TT_COLON); err != nil {
		return nil, err
	}
	if err := p.expectNewline(); err != nil {
		return nil, err
	}
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
		Name:       nameTok.Value,
		Params:     params,
		ReturnType: retType,
		Body:       body,
		Line:       line,
	}, nil
}

// parseBlock reads statements until it sees one of the stop tokens
func (p *Parser) parseBlock(stopAt ...lexer.TokenType) ([]ast.Stmt, error) {
	var stmts []ast.Stmt
	p.skipNewlines()

	for !p.check(lexer.TT_EOF) {
		// Check stop tokens
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
	p.advance() // consume 'return'

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
	p.advance() // consume 'print'
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
	p.advance() // consume 'if'

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

	return &ast.IfStmt{
		Condition: cond,
		Then:      then,
		ElseIfs:   elseIfs,
		Else:      elseBody,
		Line:      line,
	}, nil
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

	// init
	init, err := p.parseIdentStmt()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_COMMA); err != nil {
		return nil, err
	}

	// condition
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TT_COMMA); err != nil {
		return nil, err
	}

	// post (assignment)
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

// parseIdentStmt handles: var decl (name: type = expr) OR assignment (name = expr) OR call
func (p *Parser) parseIdentStmt() (ast.Stmt, error) {
	name, err := p.expect(lexer.TT_IDENT)
	if err != nil {
		return nil, err
	}
	line := name.Line

	// var decl: name: type = expr
	if p.check(lexer.TT_COLON) {
		p.advance()
		varType, err := p.parseType()
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

	// assignment: name = expr
	if p.check(lexer.TT_ASSIGN) {
		p.advance()
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		p.expectNewline()
		return &ast.Assign{Name: name.Value, Value: val, Line: line}, nil
	}

	// function call used as statement
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

	return nil, &ParseError{Line: line, Message: fmt.Sprintf("expected ':' or '=' after identifier %q", name.Value)}
}

// ─── Expressions ──────────────────────────────────────────────────────────────

func (p *Parser) parseExpr() (ast.Expr, error) {
	return p.parseOr()
}

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
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.TT_AND) {
		op := p.advance()
		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		left = &ast.BinOp{Op: op.Value, Left: left, Right: right, Line: op.Line}
	}
	return left, nil
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
	for p.check(lexer.TT_STAR) || p.check(lexer.TT_SLASH) {
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
		operand, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryOp{Op: "-", Operand: operand, Line: op.Line}, nil
	}
	return p.parsePrimary()
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

	case lexer.TT_IDENT:
		p.advance()
		// function call
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
