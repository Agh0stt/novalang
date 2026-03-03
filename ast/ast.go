package ast

import "fmt"

// TypeKind represents a Nova type
type TypeKind int

const (
	TypeVoid TypeKind = iota
	TypeInt
	TypeStr
	TypeBool
	TypeFloat
	TypeDouble
)

func (t TypeKind) String() string {
	switch t {
	case TypeVoid:
		return "void"
	case TypeInt:
		return "int"
	case TypeStr:
		return "str"
	case TypeBool:
		return "bool"
	case TypeFloat:
		return "float"
	case TypeDouble:
		return "double"
	}
	return "unknown"
}

// Node is the base interface for all AST nodes
type Node interface {
	nodeTag()
	String() string
}

// Statements
type Stmt interface {
	Node
	stmtTag()
}

// Expressions
type Expr interface {
	Node
	exprTag()
}

// ─── Statements ───────────────────────────────────────────────────────────────

type Program struct {
	Functions []*FuncDecl
}

func (p *Program) nodeTag()    {}
func (p *Program) String() string { return fmt.Sprintf("Program(%d funcs)", len(p.Functions)) }

type FuncDecl struct {
	Name       string
	Params     []Param
	ReturnType TypeKind
	Body       []Stmt
	Line       int
}

func (f *FuncDecl) nodeTag()       {}
func (f *FuncDecl) stmtTag()       {}
func (f *FuncDecl) String() string { return fmt.Sprintf("FuncDecl(%s)", f.Name) }

type Param struct {
	Name string
	Type TypeKind
}

type VarDecl struct {
	Name  string
	Type  TypeKind
	Value Expr
	Line  int
}

func (v *VarDecl) nodeTag()       {}
func (v *VarDecl) stmtTag()       {}
func (v *VarDecl) String() string { return fmt.Sprintf("VarDecl(%s: %s)", v.Name, v.Type) }

type Assign struct {
	Name  string
	Value Expr
	Line  int
}

func (a *Assign) nodeTag()       {}
func (a *Assign) stmtTag()       {}
func (a *Assign) String() string { return fmt.Sprintf("Assign(%s)", a.Name) }

type ReturnStmt struct {
	Value Expr
	Line  int
}

func (r *ReturnStmt) nodeTag()       {}
func (r *ReturnStmt) stmtTag()       {}
func (r *ReturnStmt) String() string { return "Return" }

type PrintStmt struct {
	Args []Expr
	Line int
}

func (p *PrintStmt) nodeTag()       {}
func (p *PrintStmt) stmtTag()       {}
func (p *PrintStmt) String() string { return "Print" }

type UndefStmt struct {
	Name string
	Line int
}

func (u *UndefStmt) nodeTag()       {}
func (u *UndefStmt) stmtTag()       {}
func (u *UndefStmt) String() string { return fmt.Sprintf("Undef(%s)", u.Name) }

type IfStmt struct {
	Condition Expr
	Then      []Stmt
	ElseIfs   []ElseIfClause
	Else      []Stmt
	Line      int
}

type ElseIfClause struct {
	Condition Expr
	Body      []Stmt
}

func (i *IfStmt) nodeTag()       {}
func (i *IfStmt) stmtTag()       {}
func (i *IfStmt) String() string { return "If" }

type WhileStmt struct {
	Condition Expr
	Body      []Stmt
	Line      int
}

func (w *WhileStmt) nodeTag()       {}
func (w *WhileStmt) stmtTag()       {}
func (w *WhileStmt) String() string { return "While" }

type ForStmt struct {
	Init      Stmt
	Condition Expr
	Post      Stmt
	Body      []Stmt
	Line      int
}

func (f *ForStmt) nodeTag()       {}
func (f *ForStmt) stmtTag()       {}
func (f *ForStmt) String() string { return "For" }

type ExprStmt struct {
	Expr Expr
	Line int
}

func (e *ExprStmt) nodeTag()       {}
func (e *ExprStmt) stmtTag()       {}
func (e *ExprStmt) String() string { return fmt.Sprintf("ExprStmt(%s)", e.Expr) }

// ─── Expressions ──────────────────────────────────────────────────────────────

type IntLit struct {
	Value int64
	Line  int
}

func (i *IntLit) nodeTag()       {}
func (i *IntLit) exprTag()       {}
func (i *IntLit) String() string { return fmt.Sprintf("Int(%d)", i.Value) }

type FloatLit struct {
	Value float64
	Line  int
}

func (f *FloatLit) nodeTag()       {}
func (f *FloatLit) exprTag()       {}
func (f *FloatLit) String() string { return fmt.Sprintf("Float(%f)", f.Value) }

type StrLit struct {
	Value string
	Line  int
}

func (s *StrLit) nodeTag()       {}
func (s *StrLit) exprTag()       {}
func (s *StrLit) String() string { return fmt.Sprintf("Str(%q)", s.Value) }

type BoolLit struct {
	Value bool
	Line  int
}

func (b *BoolLit) nodeTag()       {}
func (b *BoolLit) exprTag()       {}
func (b *BoolLit) String() string { return fmt.Sprintf("Bool(%v)", b.Value) }

type Ident struct {
	Name string
	Line int
}

func (id *Ident) nodeTag()       {}
func (id *Ident) exprTag()       {}
func (id *Ident) String() string { return fmt.Sprintf("Ident(%s)", id.Name) }

type BinOp struct {
	Op    string
	Left  Expr
	Right Expr
	Line  int
}

func (b *BinOp) nodeTag()       {}
func (b *BinOp) exprTag()       {}
func (b *BinOp) String() string { return fmt.Sprintf("BinOp(%s)", b.Op) }

type UnaryOp struct {
	Op      string
	Operand Expr
	Line    int
}

func (u *UnaryOp) nodeTag()       {}
func (u *UnaryOp) exprTag()       {}
func (u *UnaryOp) String() string { return fmt.Sprintf("UnaryOp(%s)", u.Op) }

type CallExpr struct {
	Callee string
	Args   []Expr
	Line   int
}

func (c *CallExpr) nodeTag()       {}
func (c *CallExpr) exprTag()       {}
func (c *CallExpr) String() string { return fmt.Sprintf("Call(%s)", c.Callee) }

type InputExpr struct {
	Prompt Expr // optional
	Line   int
}

func (inp *InputExpr) nodeTag()       {}
func (inp *InputExpr) exprTag()       {}
func (inp *InputExpr) String() string { return "Input()" }
