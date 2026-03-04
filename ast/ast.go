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
	TypeArray  // arr[ElementType]
	TypeStruct // user-defined struct
	TypeAlias  // typedef alias
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
	case TypeArray:
		return "array"
	case TypeStruct:
		return "struct"
	case TypeAlias:
		return "alias"
	}
	return "unknown"
}

// TypeRef is a full type reference (may include element type or struct name)
type TypeRef struct {
	Kind     TypeKind
	Name     string   // for struct / typedef alias
	ElemType *TypeRef // for array element type
	Size     int      // for fixed-size arrays (-1 = dynamic)
}

func (tr *TypeRef) String() string {
	if tr == nil {
		return "nil"
	}
	switch tr.Kind {
	case TypeArray:
		if tr.Size >= 0 {
			return fmt.Sprintf("arr[%s; %d]", tr.ElemType, tr.Size)
		}
		return fmt.Sprintf("arr[%s]", tr.ElemType)
	case TypeStruct, TypeAlias:
		return tr.Name
	default:
		return tr.Kind.String()
	}
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

// ─── Top-level declarations ───────────────────────────────────────────────────

type Program struct {
	Imports   []*ImportDecl
	Typedefs  []*TypedefDecl
	Structs   []*StructDecl
	Functions []*FuncDecl
	TopLevel  []Node
}

func (p *Program) nodeTag() {}
func (p *Program) String() string {
	return fmt.Sprintf("Program(%d funcs, %d structs, %d typedefs, %d imports)",
		len(p.Functions), len(p.Structs), len(p.Typedefs), len(p.Imports))
}

// ImportDecl: import "path/to/file.nv"
type ImportDecl struct {
	Path string
	Line int
}

func (i *ImportDecl) nodeTag()       {}
func (i *ImportDecl) stmtTag()       {}
func (i *ImportDecl) String() string { return fmt.Sprintf("Import(%q)", i.Path) }

// TypedefDecl: typedef NewName = ExistingType
type TypedefDecl struct {
	Name    string
	Aliased *TypeRef
	Line    int
}

func (t *TypedefDecl) nodeTag()       {}
func (t *TypedefDecl) stmtTag()       {}
func (t *TypedefDecl) String() string { return fmt.Sprintf("Typedef(%s = %s)", t.Name, t.Aliased) }

// StructDecl: struct Name: ... endstruct
type StructDecl struct {
	Name   string
	Fields []StructField
	Line   int
}

type StructField struct {
	Name string
	Type *TypeRef
	Line int
}

func (s *StructDecl) nodeTag()       {}
func (s *StructDecl) stmtTag()       {}
func (s *StructDecl) String() string { return fmt.Sprintf("Struct(%s)", s.Name) }

// ─── Statements ───────────────────────────────────────────────────────────────

type FuncDecl struct {
	Name       string
	Params     []Param
	ReturnType *TypeRef
	Body       []Stmt
	Line       int
}

func (f *FuncDecl) nodeTag()       {}
func (f *FuncDecl) stmtTag()       {}
func (f *FuncDecl) String() string { return fmt.Sprintf("FuncDecl(%s)", f.Name) }

type Param struct {
	Name string
	Type *TypeRef
}

// VarDecl: name: type = expr
type VarDecl struct {
	Name  string
	Type  *TypeRef
	Value Expr
	Line  int
}

func (v *VarDecl) nodeTag()       {}
func (v *VarDecl) stmtTag()       {}
func (v *VarDecl) String() string { return fmt.Sprintf("VarDecl(%s: %s)", v.Name, v.Type) }

// LValue represents an assignable location
type LValue interface {
	Node
	lvalueTag()
}

type LValueIdent struct {
	Name string
	Line int
}

func (l *LValueIdent) nodeTag()       {}
func (l *LValueIdent) lvalueTag()     {}
func (l *LValueIdent) String() string { return l.Name }

type LValueIndex struct {
	Object Expr
	Index  Expr
	Line   int
}

func (l *LValueIndex) nodeTag()       {}
func (l *LValueIndex) lvalueTag()     {}
func (l *LValueIndex) String() string { return fmt.Sprintf("%s[...]", l.Object) }

type LValueField struct {
	Object Expr
	Field  string
	Line   int
}

func (l *LValueField) nodeTag()       {}
func (l *LValueField) lvalueTag()     {}
func (l *LValueField) String() string { return fmt.Sprintf("%s.%s", l.Object, l.Field) }

// Assign: target op= expr  (op can be "=" "+=" "-=" "*=" "/=")
type Assign struct {
	Target LValue
	Value  Expr
	Op     string
	Line   int
}

func (a *Assign) nodeTag()       {}
func (a *Assign) stmtTag()       {}
func (a *Assign) String() string { return fmt.Sprintf("Assign(%s %s)", a.Target, a.Op) }

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

type BreakStmt struct{ Line int }

func (b *BreakStmt) nodeTag()       {}
func (b *BreakStmt) stmtTag()       {}
func (b *BreakStmt) String() string { return "Break" }

type ContinueStmt struct{ Line int }

func (c *ContinueStmt) nodeTag()       {}
func (c *ContinueStmt) stmtTag()       {}
func (c *ContinueStmt) String() string { return "Continue" }

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

// ArrayLit: [expr, expr, ...]
type ArrayLit struct {
	Elements []Expr
	Line     int
}

func (a *ArrayLit) nodeTag()       {}
func (a *ArrayLit) exprTag()       {}
func (a *ArrayLit) String() string { return fmt.Sprintf("Array(%d elems)", len(a.Elements)) }

// StructLit: TypeName { field: val, ... }
type StructLit struct {
	TypeName string
	Fields   []StructFieldInit
	Line     int
}

type StructFieldInit struct {
	Name  string
	Value Expr
}

func (s *StructLit) nodeTag()       {}
func (s *StructLit) exprTag()       {}
func (s *StructLit) String() string { return fmt.Sprintf("StructLit(%s)", s.TypeName) }

type Ident struct {
	Name string
	Line int
}

func (id *Ident) nodeTag()       {}
func (id *Ident) exprTag()       {}
func (id *Ident) String() string { return fmt.Sprintf("Ident(%s)", id.Name) }

// IndexExpr: expr[expr]
type IndexExpr struct {
	Object Expr
	Index  Expr
	Line   int
}

func (i *IndexExpr) nodeTag()       {}
func (i *IndexExpr) exprTag()       {}
func (i *IndexExpr) String() string { return fmt.Sprintf("Index(%s)", i.Object) }

// FieldExpr: expr.field
type FieldExpr struct {
	Object Expr
	Field  string
	Line   int
}

func (f *FieldExpr) nodeTag()       {}
func (f *FieldExpr) exprTag()       {}
func (f *FieldExpr) String() string { return fmt.Sprintf("Field(%s.%s)", f.Object, f.Field) }

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

// MethodCallExpr: expr.method(args)
type MethodCallExpr struct {
	Object Expr
	Method string
	Args   []Expr
	Line   int
}

func (m *MethodCallExpr) nodeTag()       {}
func (m *MethodCallExpr) exprTag()       {}
func (m *MethodCallExpr) String() string { return fmt.Sprintf("MethodCall(.%s)", m.Method) }

type InputExpr struct {
	Prompt Expr
	Line   int
}

func (inp *InputExpr) nodeTag()       {}
func (inp *InputExpr) exprTag()       {}
func (inp *InputExpr) String() string { return "Input()" }

// LenExpr: len(expr)
type LenExpr struct {
	Arg  Expr
	Line int
}

func (l *LenExpr) nodeTag()       {}
func (l *LenExpr) exprTag()       {}
func (l *LenExpr) String() string { return "Len()" }

// AppendExpr: append(arr, val)
type AppendExpr struct {
	Array Expr
	Value Expr
	Line  int
}

func (a *AppendExpr) nodeTag()       {}
func (a *AppendExpr) exprTag()       {}
func (a *AppendExpr) String() string { return "Append()" }
