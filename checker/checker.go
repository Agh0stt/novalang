package checker

import (
	"fmt"
	"novalang/ast"
)

type TypeError struct {
	Line    int
	Message string
}

func (e *TypeError) Error() string {
	return fmt.Sprintf("TypeError at line %d: %s", e.Line, e.Message)
}

type Symbol struct {
	Type ast.TypeKind
	Line int
}

type FuncSig struct {
	Params     []ast.Param
	ReturnType ast.TypeKind
}

type Scope struct {
	vars   map[string]Symbol
	parent *Scope
}

func newScope(parent *Scope) *Scope {
	return &Scope{vars: make(map[string]Symbol), parent: parent}
}

func (s *Scope) define(name string, sym Symbol) {
	s.vars[name] = sym
}

func (s *Scope) lookup(name string) (Symbol, bool) {
	if sym, ok := s.vars[name]; ok {
		return sym, true
	}
	if s.parent != nil {
		return s.parent.lookup(name)
	}
	return Symbol{}, false
}

func (s *Scope) undefine(name string) {
	delete(s.vars, name)
}

type Checker struct {
	funcs      map[string]FuncSig
	scope      *Scope
	returnType ast.TypeKind
}

func New() *Checker {
	return &Checker{funcs: make(map[string]FuncSig)}
}

func (c *Checker) Check(prog *ast.Program) error {
	// First pass: collect function signatures
	for _, fn := range prog.Functions {
		c.funcs[fn.Name] = FuncSig{Params: fn.Params, ReturnType: fn.ReturnType}
	}

	// Check that main exists
	mainSig, ok := c.funcs["main"]
	if !ok {
		return &TypeError{Line: 0, Message: "program has no main() function"}
	}
	if len(mainSig.Params) != 0 {
		return &TypeError{Line: 0, Message: "main() must have no parameters"}
	}
	if mainSig.ReturnType != ast.TypeVoid && mainSig.ReturnType != ast.TypeInt {
		return &TypeError{Line: 0, Message: "main() must return void or int"}
	}

	// Second pass: check all function bodies
	for _, fn := range prog.Functions {
		if err := c.checkFunc(fn); err != nil {
			return err
		}
	}
	return nil
}

func (c *Checker) checkFunc(fn *ast.FuncDecl) error {
	c.scope = newScope(nil)
	c.returnType = fn.ReturnType

	for _, p := range fn.Params {
		c.scope.define(p.Name, Symbol{Type: p.Type, Line: fn.Line})
	}

	return c.checkBlock(fn.Body)
}

func (c *Checker) checkBlock(stmts []ast.Stmt) error {
	for _, stmt := range stmts {
		if err := c.checkStmt(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (c *Checker) checkStmt(stmt ast.Stmt) error {
	switch s := stmt.(type) {
	case *ast.FuncDecl:
		// nested func (just register signature)
		c.funcs[s.Name] = FuncSig{Params: s.Params, ReturnType: s.ReturnType}
		return c.checkFunc(s)

	case *ast.VarDecl:
		valType, err := c.checkExpr(s.Value)
		if err != nil {
			return err
		}
		if !typesCompatible(s.Type, valType) {
			return &TypeError{
				Line:    s.Line,
				Message: fmt.Sprintf("cannot assign %s to variable %q of type %s", valType, s.Name, s.Type),
			}
		}
		c.scope.define(s.Name, Symbol{Type: s.Type, Line: s.Line})
		return nil

	case *ast.Assign:
		sym, ok := c.scope.lookup(s.Name)
		if !ok {
			return &TypeError{Line: s.Line, Message: fmt.Sprintf("undefined variable %q", s.Name)}
		}
		valType, err := c.checkExpr(s.Value)
		if err != nil {
			return err
		}
		if !typesCompatible(sym.Type, valType) {
			return &TypeError{
				Line:    s.Line,
				Message: fmt.Sprintf("cannot assign %s to variable %q of type %s", valType, s.Name, sym.Type),
			}
		}
		return nil

	case *ast.ReturnStmt:
		if s.Value == nil {
			if c.returnType != ast.TypeVoid {
				return &TypeError{Line: s.Line, Message: "missing return value"}
			}
			return nil
		}
		valType, err := c.checkExpr(s.Value)
		if err != nil {
			return err
		}
		if !typesCompatible(c.returnType, valType) {
			return &TypeError{
				Line:    s.Line,
				Message: fmt.Sprintf("return type mismatch: expected %s, got %s", c.returnType, valType),
			}
		}
		return nil

	case *ast.PrintStmt:
		for _, arg := range s.Args {
			if _, err := c.checkExpr(arg); err != nil {
				return err
			}
		}
		return nil

	case *ast.UndefStmt:
		if _, ok := c.scope.lookup(s.Name); !ok {
			return &TypeError{Line: s.Line, Message: fmt.Sprintf("undef: %q is not defined", s.Name)}
		}
		c.scope.undefine(s.Name)
		return nil

	case *ast.IfStmt:
		condType, err := c.checkExpr(s.Condition)
		if err != nil {
			return err
		}
		if condType != ast.TypeBool {
			return &TypeError{Line: s.Line, Message: fmt.Sprintf("if condition must be bool, got %s", condType)}
		}
		savedScope := c.scope
		c.scope = newScope(savedScope)
		if err := c.checkBlock(s.Then); err != nil {
			return err
		}
		c.scope = savedScope
		for _, eif := range s.ElseIfs {
			et, err := c.checkExpr(eif.Condition)
			if err != nil {
				return err
			}
			if et != ast.TypeBool {
				return &TypeError{Line: s.Line, Message: "elif condition must be bool"}
			}
			c.scope = newScope(savedScope)
			if err := c.checkBlock(eif.Body); err != nil {
				return err
			}
			c.scope = savedScope
		}
		if len(s.Else) > 0 {
			c.scope = newScope(savedScope)
			if err := c.checkBlock(s.Else); err != nil {
				return err
			}
			c.scope = savedScope
		}
		return nil

	case *ast.WhileStmt:
		condType, err := c.checkExpr(s.Condition)
		if err != nil {
			return err
		}
		if condType != ast.TypeBool {
			return &TypeError{Line: s.Line, Message: fmt.Sprintf("while condition must be bool, got %s", condType)}
		}
		savedScope := c.scope
		c.scope = newScope(savedScope)
		if err := c.checkBlock(s.Body); err != nil {
			return err
		}
		c.scope = savedScope
		return nil

	case *ast.ForStmt:
		savedScope := c.scope
		c.scope = newScope(savedScope)
		if err := c.checkStmt(s.Init); err != nil {
			return err
		}
		condType, err := c.checkExpr(s.Condition)
		if err != nil {
			return err
		}
		if condType != ast.TypeBool {
			return &TypeError{Line: s.Line, Message: "for condition must be bool"}
		}
		if err := c.checkStmt(s.Post); err != nil {
			return err
		}
		if err := c.checkBlock(s.Body); err != nil {
			return err
		}
		c.scope = savedScope
		return nil

	case *ast.ExprStmt:
		_, err := c.checkExpr(s.Expr)
		return err
	}

	return fmt.Errorf("unknown statement type %T", stmt)
}

func (c *Checker) checkExpr(expr ast.Expr) (ast.TypeKind, error) {
	switch e := expr.(type) {
	case *ast.IntLit:
		return ast.TypeInt, nil
	case *ast.FloatLit:
		return ast.TypeFloat, nil
	case *ast.StrLit:
		return ast.TypeStr, nil
	case *ast.BoolLit:
		return ast.TypeBool, nil

	case *ast.Ident:
		sym, ok := c.scope.lookup(e.Name)
		if !ok {
			return 0, &TypeError{Line: e.Line, Message: fmt.Sprintf("undefined variable %q", e.Name)}
		}
		return sym.Type, nil

	case *ast.UnaryOp:
		t, err := c.checkExpr(e.Operand)
		if err != nil {
			return 0, err
		}
		if e.Op == "-" && (t == ast.TypeInt || t == ast.TypeFloat || t == ast.TypeDouble) {
			return t, nil
		}
		return 0, &TypeError{Line: e.Line, Message: fmt.Sprintf("invalid unary op %s on %s", e.Op, t)}

	case *ast.BinOp:
		return c.checkBinOp(e)

	case *ast.CallExpr:
		sig, ok := c.funcs[e.Callee]
		if !ok {
			return 0, &TypeError{Line: e.Line, Message: fmt.Sprintf("undefined function %q", e.Callee)}
		}
		if len(e.Args) != len(sig.Params) {
			return 0, &TypeError{
				Line:    e.Line,
				Message: fmt.Sprintf("function %q expects %d args, got %d", e.Callee, len(sig.Params), len(e.Args)),
			}
		}
		for i, arg := range e.Args {
			argType, err := c.checkExpr(arg)
			if err != nil {
				return 0, err
			}
			if !typesCompatible(sig.Params[i].Type, argType) {
				return 0, &TypeError{
					Line:    e.Line,
					Message: fmt.Sprintf("argument %d of %q: expected %s, got %s", i+1, e.Callee, sig.Params[i].Type, argType),
				}
			}
		}
		return sig.ReturnType, nil

	case *ast.InputExpr:
		// input() returns str
		return ast.TypeStr, nil
	}

	return 0, fmt.Errorf("unknown expression type %T", expr)
}

func (c *Checker) checkBinOp(e *ast.BinOp) (ast.TypeKind, error) {
	lt, err := c.checkExpr(e.Left)
	if err != nil {
		return 0, err
	}
	rt, err := c.checkExpr(e.Right)
	if err != nil {
		return 0, err
	}

	switch e.Op {
	case "+", "-", "*", "/":
		if isNumeric(lt) && isNumeric(rt) {
			if lt == ast.TypeDouble || rt == ast.TypeDouble {
				return ast.TypeDouble, nil
			}
			if lt == ast.TypeFloat || rt == ast.TypeFloat {
				return ast.TypeFloat, nil
			}
			return ast.TypeInt, nil
		}
		// String concatenation with +
		if e.Op == "+" && lt == ast.TypeStr && rt == ast.TypeStr {
			return ast.TypeStr, nil
		}
		return 0, &TypeError{Line: e.Line, Message: fmt.Sprintf("invalid operands for %s: %s and %s", e.Op, lt, rt)}

	case "<", ">", ">=", "<=":
		if (isNumeric(lt) && isNumeric(rt)) || (lt == ast.TypeStr && rt == ast.TypeStr) {
			return ast.TypeBool, nil
		}
		return 0, &TypeError{Line: e.Line, Message: fmt.Sprintf("cannot compare %s and %s", lt, rt)}

	case "==", "!=":
		if typesCompatible(lt, rt) || typesCompatible(rt, lt) {
			return ast.TypeBool, nil
		}
		return 0, &TypeError{Line: e.Line, Message: fmt.Sprintf("cannot compare %s and %s", lt, rt)}

	case "and", "or":
		if lt == ast.TypeBool && rt == ast.TypeBool {
			return ast.TypeBool, nil
		}
		return 0, &TypeError{Line: e.Line, Message: fmt.Sprintf("logical operators require bool, got %s and %s", lt, rt)}
	}

	return 0, &TypeError{Line: e.Line, Message: fmt.Sprintf("unknown operator %q", e.Op)}
}

func isNumeric(t ast.TypeKind) bool {
	return t == ast.TypeInt || t == ast.TypeFloat || t == ast.TypeDouble
}

func typesCompatible(expected, got ast.TypeKind) bool {
	if expected == got {
		return true
	}
	// Allow int -> float/double and float -> double promotions
	if expected == ast.TypeFloat && got == ast.TypeInt {
		return true
	}
	if expected == ast.TypeDouble && (got == ast.TypeInt || got == ast.TypeFloat) {
		return true
	}
	return false
}
