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
	Type *ast.TypeRef
	Line int
}

type FuncSig struct {
	Params     []ast.Param
	ReturnType *ast.TypeRef
}

type StructDef struct {
	Fields []ast.StructField
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
	structs    map[string]*StructDef
	typedefs   map[string]*ast.TypeRef
	scope      *Scope
	returnType *ast.TypeRef
	inLoop     bool
}

func New() *Checker {
	return &Checker{
		funcs:    make(map[string]FuncSig),
		structs:  make(map[string]*StructDef),
		typedefs: make(map[string]*ast.TypeRef),
	}
}

func (c *Checker) Check(prog *ast.Program) error {
	// Register typedefs
	for _, td := range prog.Typedefs {
		c.typedefs[td.Name] = td.Aliased
	}

	// Register structs
	for _, s := range prog.Structs {
		c.structs[s.Name] = &StructDef{Fields: s.Fields}
	}

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
	if mainSig.ReturnType.Kind != ast.TypeVoid && mainSig.ReturnType.Kind != ast.TypeInt {
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
	if len(fn.Params) > 8 {
		return &TypeError{Line: fn.Line, Message: fmt.Sprintf("function %q has %d params; max 8 supported", fn.Name, len(fn.Params))}
	}
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

func (c *Checker) resolveType(tr *ast.TypeRef) *ast.TypeRef {
	if tr == nil {
		return tr
	}
	if tr.Kind == ast.TypeStruct || tr.Kind == ast.TypeAlias {
		// check if it's a typedef alias first
		if aliased, ok := c.typedefs[tr.Name]; ok {
			return c.resolveType(aliased)
		}
		// else it's a struct
	}
	return tr
}

func (c *Checker) checkStmt(stmt ast.Stmt) error {
	switch s := stmt.(type) {
	case *ast.FuncDecl:
		c.funcs[s.Name] = FuncSig{Params: s.Params, ReturnType: s.ReturnType}
		return c.checkFunc(s)

	case *ast.StructDecl:
		c.structs[s.Name] = &StructDef{Fields: s.Fields}
		return nil

	case *ast.TypedefDecl:
		c.typedefs[s.Name] = s.Aliased
		return nil

	case *ast.VarDecl:
		resolved := c.resolveType(s.Type)
		// For struct-typed vars, allow `p: Point = Point` (sentinel initialization)
		if resolved != nil && resolved.Kind == ast.TypeStruct {
			if ident, ok := s.Value.(*ast.Ident); ok && ident.Name == resolved.Name {
				c.scope.define(s.Name, Symbol{Type: s.Type, Line: s.Line})
				return nil
			}
		}
		valType, err := c.checkExpr(s.Value)
		if err != nil {
			return err
		}
		if !c.typesCompatible(resolved, valType) {
			return &TypeError{
				Line:    s.Line,
				Message: fmt.Sprintf("cannot assign %s to variable %q of type %s", valType, s.Name, s.Type),
			}
		}
		c.scope.define(s.Name, Symbol{Type: s.Type, Line: s.Line})
		return nil

	case *ast.Assign:
		valType, err := c.checkExpr(s.Value)
		if err != nil {
			return err
		}
		targetType, err := c.checkLValue(s.Target)
		if err != nil {
			return err
		}
		// For compound assignment, check operator makes sense
		if s.Op != "=" {
			if !isNumeric(targetType) {
				return &TypeError{Line: s.Line, Message: fmt.Sprintf("compound assignment %s requires numeric type", s.Op)}
			}
		}
		if !c.typesCompatible(targetType, valType) {
			return &TypeError{
				Line:    s.Line,
				Message: fmt.Sprintf("cannot assign %s to %s target", valType, targetType),
			}
		}
		return nil

	case *ast.ReturnStmt:
		if s.Value == nil {
			if c.returnType.Kind != ast.TypeVoid {
				return &TypeError{Line: s.Line, Message: "missing return value"}
			}
			return nil
		}
		valType, err := c.checkExpr(s.Value)
		if err != nil {
			return err
		}
		if !c.typesCompatible(c.returnType, valType) {
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

	case *ast.BreakStmt:
		if !c.inLoop {
			return &TypeError{Line: s.Line, Message: "'break' outside of loop"}
		}
		return nil

	case *ast.ContinueStmt:
		if !c.inLoop {
			return &TypeError{Line: s.Line, Message: "'continue' outside of loop"}
		}
		return nil

	case *ast.IfStmt:
		condType, err := c.checkExpr(s.Condition)
		if err != nil {
			return err
		}
		if condType.Kind != ast.TypeBool {
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
			if et.Kind != ast.TypeBool {
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
		if condType.Kind != ast.TypeBool {
			return &TypeError{Line: s.Line, Message: fmt.Sprintf("while condition must be bool, got %s", condType)}
		}
		savedScope := c.scope
		c.scope = newScope(savedScope)
		savedLoop := c.inLoop
		c.inLoop = true
		if err := c.checkBlock(s.Body); err != nil {
			return err
		}
		c.inLoop = savedLoop
		c.scope = savedScope
		return nil

	case *ast.ForStmt:
		savedScope := c.scope
		c.scope = newScope(savedScope)
		savedLoop := c.inLoop
		c.inLoop = true
		if err := c.checkStmt(s.Init); err != nil {
			return err
		}
		condType, err := c.checkExpr(s.Condition)
		if err != nil {
			return err
		}
		if condType.Kind != ast.TypeBool {
			return &TypeError{Line: s.Line, Message: "for condition must be bool"}
		}
		if err := c.checkStmt(s.Post); err != nil {
			return err
		}
		if err := c.checkBlock(s.Body); err != nil {
			return err
		}
		c.inLoop = savedLoop
		c.scope = savedScope
		return nil

	case *ast.ExprStmt:
		_, err := c.checkExpr(s.Expr)
		return err
	}

	return fmt.Errorf("unknown statement type %T", stmt)
}

func (c *Checker) checkLValue(lv ast.LValue) (*ast.TypeRef, error) {
	switch l := lv.(type) {
	case *ast.LValueIdent:
		sym, ok := c.scope.lookup(l.Name)
		if !ok {
			return nil, &TypeError{Line: l.Line, Message: fmt.Sprintf("undefined variable %q", l.Name)}
		}
		return sym.Type, nil

	case *ast.LValueIndex:
		objType, err := c.checkExpr(l.Object)
		if err != nil {
			return nil, err
		}
		if objType.Kind != ast.TypeArray {
			return nil, &TypeError{Line: l.Line, Message: fmt.Sprintf("indexing non-array type %s", objType)}
		}
		idxType, err := c.checkExpr(l.Index)
		if err != nil {
			return nil, err
		}
		if idxType.Kind != ast.TypeInt {
			return nil, &TypeError{Line: l.Line, Message: "array index must be int"}
		}
		return objType.ElemType, nil

	case *ast.LValueField:
		objType, err := c.checkExpr(l.Object)
		if err != nil {
			return nil, err
		}
		resolved := c.resolveType(objType)
		if resolved.Kind != ast.TypeStruct {
			return nil, &TypeError{Line: l.Line, Message: fmt.Sprintf("field access on non-struct type %s", resolved)}
		}
		sd, ok := c.structs[resolved.Name]
		if !ok {
			return nil, &TypeError{Line: l.Line, Message: fmt.Sprintf("unknown struct type %q", resolved.Name)}
		}
		for _, f := range sd.Fields {
			if f.Name == l.Field {
				return f.Type, nil
			}
		}
		return nil, &TypeError{Line: l.Line, Message: fmt.Sprintf("struct %q has no field %q", resolved.Name, l.Field)}
	}
	return nil, fmt.Errorf("unknown lvalue type %T", lv)
}

func (c *Checker) checkExpr(expr ast.Expr) (*ast.TypeRef, error) {
	switch e := expr.(type) {
	case *ast.IntLit:
		return &ast.TypeRef{Kind: ast.TypeInt}, nil
	case *ast.FloatLit:
		return &ast.TypeRef{Kind: ast.TypeFloat}, nil
	case *ast.StrLit:
		return &ast.TypeRef{Kind: ast.TypeStr}, nil
	case *ast.BoolLit:
		return &ast.TypeRef{Kind: ast.TypeBool}, nil

	case *ast.ArrayLit:
		if len(e.Elements) == 0 {
			return &ast.TypeRef{Kind: ast.TypeArray, ElemType: &ast.TypeRef{Kind: ast.TypeInt}, Size: -1}, nil
		}
		elemType, err := c.checkExpr(e.Elements[0])
		if err != nil {
			return nil, err
		}
		for _, el := range e.Elements[1:] {
			et, err := c.checkExpr(el)
			if err != nil {
				return nil, err
			}
			if !c.typesCompatible(elemType, et) {
				return nil, &TypeError{Line: e.Line, Message: fmt.Sprintf("array element type mismatch: %s vs %s", elemType, et)}
			}
		}
		return &ast.TypeRef{Kind: ast.TypeArray, ElemType: elemType, Size: len(e.Elements)}, nil

	case *ast.Ident:
		sym, ok := c.scope.lookup(e.Name)
		if !ok {
			return nil, &TypeError{Line: e.Line, Message: fmt.Sprintf("undefined variable %q", e.Name)}
		}
		return sym.Type, nil

	case *ast.IndexExpr:
		objType, err := c.checkExpr(e.Object)
		if err != nil {
			return nil, err
		}
		if objType.Kind != ast.TypeArray {
			return nil, &TypeError{Line: e.Line, Message: fmt.Sprintf("indexing non-array type %s", objType)}
		}
		idxType, err := c.checkExpr(e.Index)
		if err != nil {
			return nil, err
		}
		if idxType.Kind != ast.TypeInt {
			return nil, &TypeError{Line: e.Line, Message: "array index must be int"}
		}
		return objType.ElemType, nil

	case *ast.FieldExpr:
		objType, err := c.checkExpr(e.Object)
		if err != nil {
			return nil, err
		}
		resolved := c.resolveType(objType)
		if resolved.Kind != ast.TypeStruct {
			return nil, &TypeError{Line: e.Line, Message: fmt.Sprintf("field access on non-struct type %s", resolved)}
		}
		sd, ok := c.structs[resolved.Name]
		if !ok {
			return nil, &TypeError{Line: e.Line, Message: fmt.Sprintf("unknown struct type %q", resolved.Name)}
		}
		for _, f := range sd.Fields {
			if f.Name == e.Field {
				return f.Type, nil
			}
		}
		return nil, &TypeError{Line: e.Line, Message: fmt.Sprintf("struct %q has no field %q", resolved.Name, e.Field)}

	case *ast.UnaryOp:
		t, err := c.checkExpr(e.Operand)
		if err != nil {
			return nil, err
		}
		if e.Op == "-" && isNumeric(t) {
			return t, nil
		}
		if e.Op == "not" && t.Kind == ast.TypeBool {
			return &ast.TypeRef{Kind: ast.TypeBool}, nil
		}
		return nil, &TypeError{Line: e.Line, Message: fmt.Sprintf("invalid unary op %s on %s", e.Op, t)}

	case *ast.BinOp:
		return c.checkBinOp(e)

	case *ast.CallExpr:
		sig, ok := c.funcs[e.Callee]
		if !ok {
			return nil, &TypeError{Line: e.Line, Message: fmt.Sprintf("undefined function %q", e.Callee)}
		}
		if len(e.Args) != len(sig.Params) {
			return nil, &TypeError{
				Line:    e.Line,
				Message: fmt.Sprintf("function %q expects %d args, got %d", e.Callee, len(sig.Params), len(e.Args)),
			}
		}
		for i, arg := range e.Args {
			argType, err := c.checkExpr(arg)
			if err != nil {
				return nil, err
			}
			if !c.typesCompatible(sig.Params[i].Type, argType) {
				return nil, &TypeError{
					Line:    e.Line,
					Message: fmt.Sprintf("argument %d of %q: expected %s, got %s", i+1, e.Callee, sig.Params[i].Type, argType),
				}
			}
		}
		return sig.ReturnType, nil

	case *ast.MethodCallExpr:
		// For now, treat method calls permissively (could be extended for typed methods)
		_, err := c.checkExpr(e.Object)
		if err != nil {
			return nil, err
		}
		for _, arg := range e.Args {
			if _, err := c.checkExpr(arg); err != nil {
				return nil, err
			}
		}
		return &ast.TypeRef{Kind: ast.TypeVoid}, nil

	case *ast.InputExpr:
		return &ast.TypeRef{Kind: ast.TypeStr}, nil

	case *ast.LenExpr:
		t, err := c.checkExpr(e.Arg)
		if err != nil {
			return nil, err
		}
		if t.Kind != ast.TypeArray && t.Kind != ast.TypeStr {
			return nil, &TypeError{Line: e.Line, Message: fmt.Sprintf("len() requires array or str, got %s", t)}
		}
		return &ast.TypeRef{Kind: ast.TypeInt}, nil

	case *ast.AppendExpr:
		arrType, err := c.checkExpr(e.Array)
		if err != nil {
			return nil, err
		}
		if arrType.Kind != ast.TypeArray {
			return nil, &TypeError{Line: e.Line, Message: "append() requires array as first argument"}
		}
		valType, err := c.checkExpr(e.Value)
		if err != nil {
			return nil, err
		}
		if !c.typesCompatible(arrType.ElemType, valType) {
			return nil, &TypeError{Line: e.Line, Message: fmt.Sprintf("append(): element type mismatch %s vs %s", arrType.ElemType, valType)}
		}
		return arrType, nil
	}

	return nil, fmt.Errorf("unknown expression type %T", expr)
}

func (c *Checker) checkBinOp(e *ast.BinOp) (*ast.TypeRef, error) {
	lt, err := c.checkExpr(e.Left)
	if err != nil {
		return nil, err
	}
	rt, err := c.checkExpr(e.Right)
	if err != nil {
		return nil, err
	}

	switch e.Op {
	case "+", "-", "*", "/", "%":
		if isNumeric(lt) && isNumeric(rt) {
			if lt.Kind == ast.TypeDouble || rt.Kind == ast.TypeDouble {
				return &ast.TypeRef{Kind: ast.TypeDouble}, nil
			}
			if lt.Kind == ast.TypeFloat || rt.Kind == ast.TypeFloat {
				return &ast.TypeRef{Kind: ast.TypeFloat}, nil
			}
			return &ast.TypeRef{Kind: ast.TypeInt}, nil
		}
		if e.Op == "+" && lt.Kind == ast.TypeStr && rt.Kind == ast.TypeStr {
			return &ast.TypeRef{Kind: ast.TypeStr}, nil
		}
		return nil, &TypeError{Line: e.Line, Message: fmt.Sprintf("invalid operands for %s: %s and %s", e.Op, lt, rt)}

	case "<", ">", ">=", "<=":
		if (isNumeric(lt) && isNumeric(rt)) || (lt.Kind == ast.TypeStr && rt.Kind == ast.TypeStr) {
			return &ast.TypeRef{Kind: ast.TypeBool}, nil
		}
		return nil, &TypeError{Line: e.Line, Message: fmt.Sprintf("cannot compare %s and %s", lt, rt)}

	case "==", "!=":
		if c.typesCompatible(lt, rt) || c.typesCompatible(rt, lt) {
			return &ast.TypeRef{Kind: ast.TypeBool}, nil
		}
		return nil, &TypeError{Line: e.Line, Message: fmt.Sprintf("cannot compare %s and %s", lt, rt)}

	case "and", "or":
		if lt.Kind == ast.TypeBool && rt.Kind == ast.TypeBool {
			return &ast.TypeRef{Kind: ast.TypeBool}, nil
		}
		return nil, &TypeError{Line: e.Line, Message: fmt.Sprintf("logical operators require bool, got %s and %s", lt, rt)}
	}

	return nil, &TypeError{Line: e.Line, Message: fmt.Sprintf("unknown operator %q", e.Op)}
}

func isNumeric(t *ast.TypeRef) bool {
	if t == nil {
		return false
	}
	return t.Kind == ast.TypeInt || t.Kind == ast.TypeFloat || t.Kind == ast.TypeDouble
}

func (c *Checker) typesCompatible(expected, got *ast.TypeRef) bool {
	if expected == nil || got == nil {
		return expected == got
	}
	// Resolve aliases
	expected = c.resolveType(expected)
	got = c.resolveType(got)

	if expected.Kind == got.Kind {
		if expected.Kind == ast.TypeArray {
			return c.typesCompatible(expected.ElemType, got.ElemType)
		}
		if expected.Kind == ast.TypeStruct {
			return expected.Name == got.Name
		}
		return true
	}
	// Numeric promotions
	if expected.Kind == ast.TypeFloat && got.Kind == ast.TypeInt {
		return true
	}
	if expected.Kind == ast.TypeDouble && (got.Kind == ast.TypeInt || got.Kind == ast.TypeFloat) {
		return true
	}
	return false
}
