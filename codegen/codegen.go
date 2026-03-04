// Package codegen emits Linux AArch64 assembly — NO libc.
//
// Structs: laid out as consecutive 8-byte slots on the stack.
// Arrays:  heap-allocated via sys_mmap; each array is a pointer to:
//          [int64 length][int64 capacity][elem0][elem1]...
//          All elements are 8 bytes (padded).
//
// Register conventions (AAPCS64):
//   x0-x7   arguments / return values
//   x8      syscall number
//   x9-x15  caller-saved temporaries
//   x19-x28 callee-saved (we use x19-x20 in helpers)
//   x29     frame pointer
//   x30     link register
//   sp      stack pointer (must stay 16-byte aligned)

package codegen

import (
	"fmt"
	"math"
	"novalang/ast"
	"strings"
)

const ptrSize = 8

type variable struct {
	offset   int       // from x29 (negative = on stack)
	typeRef  *ast.TypeRef
	size     int       // total bytes this var occupies on stack
}

type structLayout struct {
	fieldOffset map[string]int // field name -> offset within struct
	totalSize   int
}

type funcCtx struct {
	name        string
	vars        map[string]*variable
	frameSize   int
	labelCount  int
	breakLabel  string
	contLabel   string
}

// Codegen holds all compilation state for one translation unit
type Codegen struct {
	out      strings.Builder
	funcs    map[string]*ast.FuncDecl
	structs  map[string]*ast.StructDecl
	layouts  map[string]*structLayout
	typedefs map[string]*ast.TypeRef
	strLits  []string
	ctx      *funcCtx
}

func New() *Codegen {
	return &Codegen{
		funcs:    make(map[string]*ast.FuncDecl),
		structs:  make(map[string]*ast.StructDecl),
		layouts:  make(map[string]*structLayout),
		typedefs: make(map[string]*ast.TypeRef),
	}
}

// ─── Top-level ────────────────────────────────────────────────────────────────

func (g *Codegen) Generate(prog *ast.Program) string {
	// Register typedefs
	for _, td := range prog.Typedefs {
		g.typedefs[td.Name] = td.Aliased
	}

	// Compute struct layouts
	for _, s := range prog.Structs {
		g.structs[s.Name] = s
		g.computeLayout(s)
	}

	// Register functions
	for _, fn := range prog.Functions {
		g.funcs[fn.Name] = fn
	}

	g.line(".section .text")
	g.line(".globl main")
	g.line(".align 2")
	g.blank()

	for _, fn := range prog.Functions {
		g.genFunc(fn)
		g.blank()
	}

	g.emitRuntime()

	g.line(".section .rodata")
	for i, s := range g.strLits {
		g.linef(".Lstr%d:", i)
		g.linef("    .asciz %q", s)
		g.linef(".Lstr%d_end:", i)
	}

	return g.out.String()
}

func (g *Codegen) computeLayout(s *ast.StructDecl) {
	layout := &structLayout{fieldOffset: make(map[string]int)}
	off := 0
	for _, f := range s.Fields {
		layout.fieldOffset[f.Name] = off
		off += g.sizeOf(f.Type)
	}
	layout.totalSize = off
	g.layouts[s.Name] = layout
}

// sizeOf returns the stack size in bytes for a type (all 8-byte aligned)
func (g *Codegen) sizeOf(tr *ast.TypeRef) int {
	if tr == nil {
		return ptrSize
	}
	tr = g.resolveType(tr)
	switch tr.Kind {
	case ast.TypeStruct:
		if layout, ok := g.layouts[tr.Name]; ok {
			return layout.totalSize
		}
		return ptrSize
	case ast.TypeArray:
		return ptrSize // pointer
	default:
		return ptrSize
	}
}

func (g *Codegen) resolveType(tr *ast.TypeRef) *ast.TypeRef {
	if tr == nil {
		return tr
	}
	if tr.Kind == ast.TypeStruct || tr.Kind == ast.TypeAlias {
		if aliased, ok := g.typedefs[tr.Name]; ok {
			return g.resolveType(aliased)
		}
	}
	return tr
}

// ─── Function generation ──────────────────────────────────────────────────────

func (g *Codegen) genFunc(fn *ast.FuncDecl) {
	g.ctx = &funcCtx{
		name: fn.Name,
		vars: make(map[string]*variable),
	}

	for _, p := range fn.Params {
		g.allocVar(p.Name, p.Type)
	}
	g.prescanStmts(fn.Body)

	locals := g.ctx.frameSize
	if locals%16 != 0 {
		locals += 16 - (locals % 16)
	}
	frame := locals + 16
	if frame%16 != 0 {
		frame += 16 - (frame % 16)
	}

	g.linef("%s:", fn.Name)
	g.linef("    stp x29, x30, [sp, #-%d]!", frame)
	g.line("    mov x29, sp")

	for i, p := range fn.Params {
		v := g.ctx.vars[p.Name]
		sz := g.sizeOf(p.Type)
		if sz <= ptrSize {
			g.linef("    str x%d, [x29, #%d]   // param %s", i, v.offset, p.Name)
		} else {
			// struct param: copy from x0..xN (multi-register passing simplified to pointer)
			g.linef("    str x%d, [x29, #%d]   // param %s (ptr)", i, v.offset, p.Name)
		}
	}

	g.genBlock(fn.Body)

	g.linef(".L%s_ret:", fn.Name)
	if fn.Name == "main" {
		g.line("    mov x0, #0")
		g.line("    mov x8, #93       // sys_exit")
		g.line("    svc #0")
	} else {
		if fn.ReturnType != nil && fn.ReturnType.Kind == ast.TypeVoid {
			g.line("    mov x0, #0")
		}
		g.linef("    ldp x29, x30, [sp], #%d", frame)
		g.line("    ret")
	}
}

func (g *Codegen) prescanStmts(stmts []ast.Stmt) {
	for _, s := range stmts {
		g.prescanStmt(s)
	}
}

func (g *Codegen) prescanStmt(s ast.Stmt) {
	switch st := s.(type) {
	case *ast.VarDecl:
		g.allocVar(st.Name, st.Type)
	case *ast.IfStmt:
		g.prescanStmts(st.Then)
		for _, e := range st.ElseIfs {
			g.prescanStmts(e.Body)
		}
		g.prescanStmts(st.Else)
	case *ast.WhileStmt:
		g.prescanStmts(st.Body)
	case *ast.ForStmt:
		g.prescanStmt(st.Init)
		g.prescanStmts(st.Body)
	}
}

func (g *Codegen) allocVar(name string, t *ast.TypeRef) *variable {
	if v, ok := g.ctx.vars[name]; ok {
		return v
	}
	sz := g.sizeOf(t)
	if sz < ptrSize {
		sz = ptrSize
	}
	// Round up to multiple of 8
	if sz%ptrSize != 0 {
		sz += ptrSize - (sz % ptrSize)
	}
	g.ctx.frameSize += sz
	v := &variable{offset: -g.ctx.frameSize, typeRef: t, size: sz}
	g.ctx.vars[name] = v
	return v
}

// ─── Statements ───────────────────────────────────────────────────────────────

func (g *Codegen) genBlock(stmts []ast.Stmt) {
	for _, s := range stmts {
		g.genStmt(s)
	}
}

func (g *Codegen) genStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {

	case *ast.VarDecl:
		v := g.ctx.vars[s.Name]
		resolvedT := g.resolveType(s.Type)
		if resolvedT != nil && resolvedT.Kind == ast.TypeStruct {
			// Struct vars are stack-allocated by allocVar; zero-initialize the fields
			if layout, ok := g.layouts[resolvedT.Name]; ok {
				for i := 0; i < layout.totalSize/8; i++ {
					g.linef("    str xzr, [x29, #%d]   // zero struct field %d", v.offset+i*8, i)
				}
			}
		} else {
			g.genExpr(s.Value)
			g.linef("    str x0, [x29, #%d]   // var %s", v.offset, s.Name)
		}

	case *ast.Assign:
		g.genExpr(s.Value)
		// If compound assignment, load current value and compute
		if s.Op != "=" {
			// Push rhs to stack so genLValueLoad cannot clobber it
			g.line("    str x0, [sp, #-16]!  // push compound rhs")
			g.genLValueLoad(s.Target)        // x0 = current lhs value
			g.line("    ldr x1, [sp], #16    // pop rhs into x1")
			switch s.Op {
			case "+=":
				g.line("    add x0, x0, x1")
			case "-=":
				g.line("    sub x0, x0, x1")
			case "*=":
				g.line("    mul x0, x0, x1")
			case "/=":
				g.line("    sdiv x0, x0, x1")
			}
		}
		g.genLValueStore(s.Target)

	case *ast.ReturnStmt:
		if s.Value != nil {
			g.genExpr(s.Value)
		} else {
			g.line("    mov x0, #0")
		}
		g.linef("    b .L%s_ret", g.ctx.name)

	case *ast.PrintStmt:
		for _, arg := range s.Args {
			g.genPrint(arg)
		}

	case *ast.UndefStmt:
		if v, ok := g.ctx.vars[s.Name]; ok {
			g.linef("    str xzr, [x29, #%d]   // undef %s", v.offset, s.Name)
		}

	case *ast.BreakStmt:
		g.linef("    b %s", g.ctx.breakLabel)

	case *ast.ContinueStmt:
		g.linef("    b %s", g.ctx.contLabel)

	case *ast.IfStmt:
		g.genIf(s)

	case *ast.WhileStmt:
		g.genWhile(s)

	case *ast.ForStmt:
		g.genFor(s)

	case *ast.ExprStmt:
		g.genExpr(s.Expr)

	case *ast.FuncDecl:
		// already emitted at top level

	case *ast.StructDecl:
		// already handled in layout pass

	case *ast.TypedefDecl:
		// no code needed

	case *ast.ImportDecl:
		// handled at program level
	}
}

// genLValueLoad loads the value at lvalue into x0
func (g *Codegen) genLValueLoad(lv ast.LValue) {
	switch l := lv.(type) {
	case *ast.LValueIdent:
		v, ok := g.ctx.vars[l.Name]
		if !ok {
			panic(fmt.Sprintf("codegen: undefined var %q", l.Name))
		}
		g.linef("    ldr x0, [x29, #%d]   // load %s", v.offset, l.Name)

	case *ast.LValueIndex:
		// compute array[index] address in x0
		g.genExpr(l.Object)
		g.line("    str x0, [sp, #-16]!")   // push array ptr
		g.genExpr(l.Index)
		g.line("    mov x1, x0")             // x1 = index
		g.line("    ldr x0, [sp], #16")      // pop array ptr
		// array header: [len int64][cap int64][elems...]
		g.line("    add x0, x0, #16")        // skip header
		g.line("    lsl x1, x1, #3")         // index * 8
		g.line("    add x0, x0, x1")
		g.line("    ldr x0, [x0]")

	case *ast.LValueField:
		g.genExpr(l.Object)
		// x0 = pointer to struct base
		fieldOff := g.fieldOffset(l.Object, l.Field)
		if fieldOff != 0 {
			g.linef("    add x0, x0, #%d", fieldOff)
		}
		g.line("    ldr x0, [x0]")
	}
}

// genLValueStore stores x0 into lvalue location
func (g *Codegen) genLValueStore(lv ast.LValue) {
	switch l := lv.(type) {
	case *ast.LValueIdent:
		v, ok := g.ctx.vars[l.Name]
		if !ok {
			panic(fmt.Sprintf("codegen: undefined var %q", l.Name))
		}
		g.linef("    str x0, [x29, #%d]   // store %s", v.offset, l.Name)

	case *ast.LValueIndex:
		// x0 = value to store; compute address and store
		g.line("    str x0, [sp, #-16]!")  // push value
		g.genExpr(l.Object)
		g.line("    str x0, [sp, #-16]!")  // push array ptr
		g.genExpr(l.Index)
		g.line("    mov x1, x0")            // x1 = index
		g.line("    ldr x0, [sp], #16")     // pop array ptr
		g.line("    add x0, x0, #16")       // skip header
		g.line("    lsl x1, x1, #3")        // index * 8
		g.line("    add x0, x0, x1")        // address
		g.line("    ldr x2, [sp], #16")     // pop value
		g.line("    str x2, [x0]")

	case *ast.LValueField:
		g.line("    str x0, [sp, #-16]!")  // push value
		g.genExpr(l.Object)
		fieldOff := g.fieldOffset(l.Object, l.Field)
		if fieldOff != 0 {
			g.linef("    add x0, x0, #%d", fieldOff)
		}
		g.line("    ldr x1, [sp], #16")    // pop value
		g.line("    str x1, [x0]")
	}
}

// fieldOffset returns the byte offset of a field within a struct expression
func (g *Codegen) fieldOffset(obj ast.Expr, field string) int {
	// Try to infer struct type from the expression
	typeName := g.inferStructName(obj)
	if typeName != "" {
		if layout, ok := g.layouts[typeName]; ok {
			return layout.fieldOffset[field]
		}
	}
	return 0
}

func (g *Codegen) inferStructName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		if v, ok := g.ctx.vars[e.Name]; ok {
			tr := g.resolveType(v.typeRef)
			if tr != nil && tr.Kind == ast.TypeStruct {
				return tr.Name
			}
		}
	}
	return ""
}

func (g *Codegen) genPrint(arg ast.Expr) {
	t := g.inferType(arg)
	g.genExpr(arg)

	resolved := g.resolveType(t)
	if resolved == nil {
		// unknown type: fall back to integer print
		g.line("    bl __nova_print_int")
		return
	}
	switch resolved.Kind {
	case ast.TypeStr:
		g.line("    bl __nova_print_str")
	case ast.TypeBool:
		g.line("    bl __nova_print_bool")
	case ast.TypeInt:
		g.line("    bl __nova_print_int")
	case ast.TypeFloat, ast.TypeDouble:
		g.line("    bl __nova_print_float")
	default:
		// fallback: print as int
		g.line("    bl __nova_print_int")
	}
}

func (g *Codegen) genIf(s *ast.IfStmt) {
	endLbl := g.label("ifend")
	nextLbl := endLbl
	if len(s.ElseIfs) > 0 || len(s.Else) > 0 {
		nextLbl = g.label("ifnext")
	}

	g.genExpr(s.Condition)
	g.linef("    cbz x0, %s", nextLbl)
	g.genBlock(s.Then)
	g.linef("    b %s", endLbl)

	for i, eif := range s.ElseIfs {
		g.linef("%s:", nextLbl)
		g.genExpr(eif.Condition)
		if i < len(s.ElseIfs)-1 || len(s.Else) > 0 {
			nextLbl = g.label("elifnext")
		} else {
			nextLbl = endLbl
		}
		g.linef("    cbz x0, %s", nextLbl)
		g.genBlock(eif.Body)
		g.linef("    b %s", endLbl)
	}

	if len(s.Else) > 0 {
		g.linef("%s:", nextLbl)
		g.genBlock(s.Else)
	}
	g.linef("%s:", endLbl)
}

func (g *Codegen) genWhile(s *ast.WhileStmt) {
	top := g.label("whtop")
	end := g.label("whend")

	savedBreak := g.ctx.breakLabel
	savedCont := g.ctx.contLabel
	g.ctx.breakLabel = end
	g.ctx.contLabel = top

	g.linef("%s:", top)
	g.genExpr(s.Condition)
	g.linef("    cbz x0, %s", end)
	g.genBlock(s.Body)
	g.linef("    b %s", top)
	g.linef("%s:", end)

	g.ctx.breakLabel = savedBreak
	g.ctx.contLabel = savedCont
}

func (g *Codegen) genFor(s *ast.ForStmt) {
	top := g.label("fortop")
	end := g.label("forend")
	cont := g.label("forcont")

	savedBreak := g.ctx.breakLabel
	savedCont := g.ctx.contLabel
	g.ctx.breakLabel = end
	g.ctx.contLabel = cont

	g.genStmt(s.Init)
	g.linef("%s:", top)
	g.genExpr(s.Condition)
	g.linef("    cbz x0, %s", end)
	g.genBlock(s.Body)
	g.linef("%s:", cont)
	g.genStmt(s.Post)
	g.linef("    b %s", top)
	g.linef("%s:", end)

	g.ctx.breakLabel = savedBreak
	g.ctx.contLabel = savedCont
}

// ─── Expressions ──────────────────────────────────────────────────────────────

func (g *Codegen) genExpr(expr ast.Expr) {
	switch e := expr.(type) {

	case *ast.IntLit:
		g.emitMovImm(e.Value)

	case *ast.FloatLit:
		g.emitMovImmU(math.Float64bits(e.Value))

	case *ast.StrLit:
		idx := g.internStr(e.Value)
		g.linef("    adrp x0, .Lstr%d", idx)
		g.linef("    add x0, x0, :lo12:.Lstr%d", idx)

	case *ast.BoolLit:
		if e.Value {
			g.line("    mov x0, #1")
		} else {
			g.line("    mov x0, #0")
		}

	case *ast.ArrayLit:
		g.genArrayLit(e)

	case *ast.Ident:
		v, ok := g.ctx.vars[e.Name]
		if !ok {
			panic(fmt.Sprintf("codegen: undefined var %q", e.Name))
		}
		tr := g.resolveType(v.typeRef)
		if tr != nil && tr.Kind == ast.TypeStruct {
			// Return pointer to struct on stack
			g.linef("    add x0, x29, #%d   // addr of struct %s", v.offset, e.Name)
		} else {
			g.linef("    ldr x0, [x29, #%d]   // %s", v.offset, e.Name)
		}

	case *ast.IndexExpr:
		g.genExpr(e.Object)
		g.line("    str x0, [sp, #-16]!")  // push array ptr
		g.genExpr(e.Index)
		g.line("    mov x1, x0")           // x1 = index
		g.line("    ldr x0, [sp], #16")    // pop array ptr
		g.line("    add x0, x0, #16")      // skip [len][cap] header
		g.line("    lsl x1, x1, #3")       // index * 8
		g.line("    add x0, x0, x1")
		g.line("    ldr x0, [x0]")

	case *ast.FieldExpr:
		g.genExpr(e.Object)
		// x0 = pointer to struct
		fieldOff := g.fieldOffset(e.Object, e.Field)
		if fieldOff != 0 {
			g.linef("    add x0, x0, #%d", fieldOff)
		}
		g.line("    ldr x0, [x0]")

	case *ast.UnaryOp:
		g.genExpr(e.Operand)
		switch e.Op {
		case "-":
			g.line("    neg x0, x0")
		case "not":
			g.line("    cmp x0, #0")
			g.line("    cset x0, eq")
		}

	case *ast.BinOp:
		g.genBinOp(e)

	case *ast.CallExpr:
		g.genCall(e)

	case *ast.MethodCallExpr:
		g.genMethodCall(e)

	case *ast.InputExpr:
		g.genInput(e)

	case *ast.LenExpr:
		g.genExpr(e.Arg)
		// x0 = array ptr or str ptr
		// For arrays: load length from header[0]
		// For str: compute strlen
		t := g.inferType(e.Arg)
		if t != nil && t.Kind == ast.TypeStr {
			g.line("    bl __nova_strlen")
		} else {
			g.line("    ldr x0, [x0]   // array length")
		}

	case *ast.AppendExpr:
		g.genAppend(e)
	}
}

func (g *Codegen) genArrayLit(e *ast.ArrayLit) {
	n := len(e.Elements)
	// Allocate array: [len][cap][elem0..elemN-1]
	// Using mmap syscall (sys_mmap = 222)
	totalBytes := (n+2)*ptrSize // 2 header words + n elements
	g.emitMovImm(0)                  // addr hint = 0
	g.line("    str x0, [sp, #-16]!")
	g.emitMovImm(int64(totalBytes))
	g.line("    str x0, [sp, #-16]!")
	g.emitMovImm(3)                  // PROT_READ|PROT_WRITE
	g.line("    str x0, [sp, #-16]!")
	g.emitMovImm(0x22)               // MAP_PRIVATE|MAP_ANONYMOUS
	g.line("    str x0, [sp, #-16]!")
	g.emitMovImm(-1)                 // fd = -1
	g.line("    str x0, [sp, #-16]!")
	g.emitMovImm(0)                  // offset = 0
	g.line("    str x0, [sp, #-16]!")
	// AArch64 mmap args: x0=addr, x1=len, x2=prot, x3=flags, x4=fd, x5=offset
	g.line("    ldr x5, [sp], #16")
	g.line("    ldr x4, [sp], #16")
	g.line("    ldr x3, [sp], #16")
	g.line("    ldr x2, [sp], #16")
	g.line("    ldr x1, [sp], #16")
	g.line("    ldr x0, [sp], #16")
	g.line("    mov x8, #222         // sys_mmap")
	g.line("    svc #0               // x0 = pointer to array memory")
	g.line("    str x0, [sp, #-16]! // save array ptr")

	// Store length and capacity
	g.emitMovImm(int64(n))
	g.line("    ldr x9, [sp]        // array ptr")
	g.line("    str x0, [x9]        // length")
	g.emitMovImm(int64(n))
	g.line("    str x0, [x9, #8]    // capacity")

	// Store elements
	for i, elem := range e.Elements {
		g.genExpr(elem)
		g.line("    ldr x9, [sp]    // array ptr")
		g.linef("    str x0, [x9, #%d]  // elem %d", 16+i*8, i)
	}

	g.line("    ldr x0, [sp], #16   // return array ptr")
}

func (g *Codegen) genAppend(e *ast.AppendExpr) {
	noexpand := g.label("app_noexpand")
	copyLbl := g.label("app_copy")
	copyDone := g.label("app_copy_done")

	// Save callee-saved registers we use (x19, x20)
	g.line("    stp x19, x20, [sp, #-16]!")
	// Load array ptr, extend if needed, store element, return new ptr
	g.genExpr(e.Array)
	g.line("    str x0, [sp, #-16]!  // push old array ptr")
	g.genExpr(e.Value)
	g.line("    mov x19, x0          // x19 = value to append")
	g.line("    ldr x20, [sp], #16   // x20 = array ptr")
	// Load length and capacity
	g.line("    ldr x9, [x20]        // length")
	g.line("    ldr x10, [x20, #8]   // capacity")
	g.line("    cmp x9, x10")
	g.linef("    b.lt %s", noexpand)
	// Need to expand: new cap = cap*2+4, new mmap
	g.line("    add x10, x10, x10    // cap * 2")
	g.line("    add x10, x10, #4     // +4 extra")
	// compute new total bytes = (cap+2)*8
	g.line("    add x1, x10, #2")
	g.line("    lsl x1, x1, #3")
	g.line("    mov x0, #0")
	g.line("    mov x2, #3")
	g.line("    mov x3, #0x22")
	g.line("    mov x4, #-1")
	g.line("    mov x5, #0")
	g.line("    mov x8, #222")
	g.line("    svc #0               // x0 = new array ptr")
	g.line("    mov x11, x0          // x11 = new ptr")
	g.line("    str x9, [x11]        // copy length")
	g.line("    str x10, [x11, #8]   // copy new cap")
	// copy elements (length * 8 bytes starting at old+16)
	g.line("    mov x12, #0          // i=0")
	g.linef("%s:", copyLbl)
	g.line("    cmp x12, x9")
	g.linef("    b.ge %s", copyDone)
	g.line("    lsl x13, x12, #3")
	g.line("    add x13, x13, #16")
	g.line("    ldr x14, [x20, x13]")
	g.line("    str x14, [x11, x13]")
	g.line("    add x12, x12, #1")
	g.linef("    b %s", copyLbl)
	g.linef("%s:", copyDone)
	g.line("    mov x20, x11         // update array ptr to new allocation")
	g.linef("%s:", noexpand)
	// append element at index [length]
	g.line("    ldr x9, [x20]        // length")
	g.line("    lsl x12, x9, #3")
	g.line("    add x12, x12, #16")
	g.line("    str x19, [x20, x12]  // store value")
	g.line("    add x9, x9, #1")
	g.line("    str x9, [x20]        // update length")
	g.line("    mov x0, x20          // return new/same array ptr")
	// Restore callee-saved registers
	g.line("    ldp x19, x20, [sp], #16")
}

func (g *Codegen) genMethodCall(e *ast.MethodCallExpr) {
	// Built-in methods: arr.length, str.length (same as len())
	// For now: emit object + treat as property
	g.genExpr(e.Object)
	t := g.inferType(e.Object)
	if t != nil && t.Kind == ast.TypeArray {
		switch e.Method {
		case "length", "len":
			g.line("    ldr x0, [x0]   // array.length")
			return
		}
	}
	if t != nil && t.Kind == ast.TypeStr {
		switch e.Method {
		case "length", "len":
			g.line("    bl __nova_strlen")
			return
		}
	}
	// Generic: no-op for unknown methods
}

func (g *Codegen) genBinOp(e *ast.BinOp) {
	// String concatenation needs a runtime helper, not integer add
	if e.Op == "+" {
		lt := g.inferType(e.Left)
		if lt != nil && lt.Kind == ast.TypeStr {
			g.genExpr(e.Left)
			g.line("    str x0, [sp, #-16]!")
			g.genExpr(e.Right)
			g.line("    mov x1, x0")
			g.line("    ldr x0, [sp], #16")
			g.line("    bl __nova_str_concat")
			return
		}
	}

	g.genExpr(e.Left)
	g.line("    str x0, [sp, #-16]!") // push left
	g.genExpr(e.Right)
	g.line("    mov x1, x0")           // x1 = right
	g.line("    ldr x0, [sp], #16")    // pop left -> x0

	switch e.Op {
	case "+":
		g.line("    add x0, x0, x1")
	case "-":
		g.line("    sub x0, x0, x1")
	case "*":
		g.line("    mul x0, x0, x1")
	case "/":
		g.line("    sdiv x0, x0, x1")
	case "%":
		g.line("    sdiv x2, x0, x1")
		g.line("    msub x0, x2, x1, x0")
	case "<":
		g.line("    cmp x0, x1"); g.line("    cset x0, lt")
	case ">":
		g.line("    cmp x0, x1"); g.line("    cset x0, gt")
	case ">=":
		g.line("    cmp x0, x1"); g.line("    cset x0, ge")
	case "<=", "=<":
		g.line("    cmp x0, x1"); g.line("    cset x0, le")
	case "==":
		g.line("    cmp x0, x1"); g.line("    cset x0, eq")
	case "!=":
		g.line("    cmp x0, x1"); g.line("    cset x0, ne")
	case "and":
		g.line("    and x0, x0, x1")
		g.line("    cmp x0, #0"); g.line("    cset x0, ne")
	case "or":
		g.line("    orr x0, x0, x1")
		g.line("    cmp x0, #0"); g.line("    cset x0, ne")
	}
}

func (g *Codegen) genCall(e *ast.CallExpr) {
	if len(e.Args) > 8 {
		panic(fmt.Sprintf("codegen: function %q called with %d args; max 8 supported", e.Callee, len(e.Args)))
	}
	for _, arg := range e.Args {
		g.genExpr(arg)
		g.line("    str x0, [sp, #-16]!")
	}
	for i := len(e.Args) - 1; i >= 0; i-- {
		g.linef("    ldr x%d, [sp], #16", i)
	}
	g.linef("    bl %s", e.Callee)
}

func (g *Codegen) genInput(e *ast.InputExpr) {
	if e.Prompt != nil {
		g.genExpr(e.Prompt)
		g.line("    bl __nova_print_str")
	}
	g.line("    bl __nova_read_str")
}

// ─── Runtime helpers (no libc) ────────────────────────────────────────────────

func (g *Codegen) emitRuntime() {
	g.line("// ═══════════════════════════════════════════════")
	g.line("// Nova runtime — pure Linux syscalls, no libc")
	g.line("// ═══════════════════════════════════════════════")
	g.blank()

	g.line("__nova_write_raw:")
	g.line("    mov x2, x1")
	g.line("    mov x1, x0")
	g.line("    mov x0, #1")
	g.line("    mov x8, #64")
	g.line("    svc #0")
	g.line("    ret")
	g.blank()

	g.line("__nova_strlen:")
	g.line("    mov x1, x0")
	g.line(".Lsl_loop:")
	g.line("    ldrb w2, [x1], #1")
	g.line("    cbnz w2, .Lsl_loop")
	g.line("    sub x0, x1, x0")
	g.line("    sub x0, x0, #1")
	g.line("    ret")
	g.blank()

	g.line("__nova_str_concat:")
	g.line("    stp x29, x30, [sp, #-64]!")
	g.line("    mov x29, sp")
	g.line("    str x19, [sp, #16]")
	g.line("    str x20, [sp, #24]")
	g.line("    str x21, [sp, #32]")
	g.line("    str x22, [sp, #40]")
	g.line("    mov x19, x0         // left ptr")
	g.line("    mov x20, x1         // right ptr")
	g.line("    mov x0, x19")
	g.line("    bl __nova_strlen")
	g.line("    mov x21, x0         // len_left")
	g.line("    mov x0, x20")
	g.line("    bl __nova_strlen")
	g.line("    mov x22, x0         // len_right")
	g.line("    add x1, x21, x22")
	g.line("    add x1, x1, #1      // +1 for NUL")
	g.line("    mov x0, #0          // mmap addr hint")
	g.line("    mov x2, #3          // PROT_READ|WRITE")
	g.line("    mov x3, #0x22       // MAP_PRIVATE|ANON")
	g.line("    mov x4, #-1         // fd=-1")
	g.line("    mov x5, #0          // offset=0")
	g.line("    mov x8, #222        // sys_mmap")
	g.line("    svc #0              // x0 = new buffer base")
	g.line("    str x0, [sp, #48]   // save buffer base")
	g.line("    mov x9, x0          // dst write cursor")
	g.line("    mov x10, x19        // src = left")
	g.line(".Lsc_left:")
	g.line("    ldrb w11, [x10], #1")
	g.line("    cbz w11, .Lsc_left_done")
	g.line("    strb w11, [x9], #1")
	g.line("    b .Lsc_left")
	g.line(".Lsc_left_done:")
	g.line("    mov x10, x20        // src = right")
	g.line(".Lsc_right:")
	g.line("    ldrb w11, [x10], #1")
	g.line("    cbz w11, .Lsc_right_done")
	g.line("    strb w11, [x9], #1")
	g.line("    b .Lsc_right")
	g.line(".Lsc_right_done:")
	g.line("    strb wzr, [x9]      // NUL-terminate")
	g.line("    ldr x0, [sp, #48]   // return buffer base")
	g.line("    ldr x19, [sp, #16]")
	g.line("    ldr x20, [sp, #24]")
	g.line("    ldr x21, [sp, #32]")
	g.line("    ldr x22, [sp, #40]")
	g.line("    ldp x29, x30, [sp], #64")
	g.line("    ret")
	g.blank()

	g.line("__nova_print_str:")
	g.line("    stp x29, x30, [sp, #-32]!")
	g.line("    mov x29, sp")
	g.line("    str x19, [sp, #16]")
	g.line("    mov x19, x0")
	g.line("    mov x1, x0")
	g.line(".Lps_loop:")
	g.line("    ldrb w2, [x1], #1")
	g.line("    cbnz w2, .Lps_loop")
	g.line("    sub x1, x1, x19")
	g.line("    sub x1, x1, #1")
	g.line("    mov x0, x19")
	g.line("    bl __nova_write_raw")
	g.line("    adrp x0, .Lnewline")
	g.line("    add x0, x0, :lo12:.Lnewline")
	g.line("    mov x1, #1")
	g.line("    bl __nova_write_raw")
	g.line("    ldr x19, [sp, #16]")
	g.line("    ldp x29, x30, [sp], #32")
	g.line("    ret")
	g.blank()

	g.line("__nova_print_bool:")
	g.line("    stp x29, x30, [sp, #-16]!")
	g.line("    mov x29, sp")
	g.line("    cbnz x0, .Lpb_true")
	g.line("    adrp x0, .Lbool_false")
	g.line("    add x0, x0, :lo12:.Lbool_false")
	g.line("    bl __nova_print_str")
	g.line("    b .Lpb_end")
	g.line(".Lpb_true:")
	g.line("    adrp x0, .Lbool_true")
	g.line("    add x0, x0, :lo12:.Lbool_true")
	g.line("    bl __nova_print_str")
	g.line(".Lpb_end:")
	g.line("    ldp x29, x30, [sp], #16")
	g.line("    ret")
	g.blank()

	g.line("__nova_print_int:")
	g.line("    stp x29, x30, [sp, #-64]!")
	g.line("    mov x29, sp")
	g.line("    add x9, sp, #56")
	g.line("    mov x10, x0")
	g.line("    mov x11, #0")
	g.line("    cmp x10, #0")
	g.line("    b.ge .Lpi_pos")
	g.line("    mov x11, #1")
	g.line("    neg x10, x10")
	g.line(".Lpi_pos:")
	g.line("    mov x12, #0")
	g.line("    mov x13, #10")
	g.line(".Lpi_loop:")
	g.line("    udiv x14, x10, x13")
	g.line("    msub x15, x14, x13, x10")
	g.line("    add x15, x15, #48")
	g.line("    strb w15, [x9, #-1]!")
	g.line("    add x12, x12, #1")
	g.line("    mov x10, x14")
	g.line("    cbnz x10, .Lpi_loop")
	g.line("    cbz x11, .Lpi_write")
	g.line("    mov x15, #45")
	g.line("    strb w15, [x9, #-1]!")
	g.line("    add x12, x12, #1")
	g.line(".Lpi_write:")
	g.line("    mov x0, x9")
	g.line("    mov x1, x12")
	g.line("    bl __nova_write_raw")
	g.line("    adrp x0, .Lnewline")
	g.line("    add x0, x0, :lo12:.Lnewline")
	g.line("    mov x1, #1")
	g.line("    bl __nova_write_raw")
	g.line("    ldp x29, x30, [sp], #64")
	g.line("    ret")
	g.blank()

	g.line("__nova_print_int_nonl:")
	g.line("    stp x29, x30, [sp, #-64]!")
	g.line("    mov x29, sp")
	g.line("    add x9, sp, #56")
	g.line("    mov x10, x0")
	g.line("    mov x12, #0")
	g.line("    mov x13, #10")
	g.line("    cbz x10, .Lpin_zero")
	g.line(".Lpin_loop:")
	g.line("    udiv x14, x10, x13")
	g.line("    msub x15, x14, x13, x10")
	g.line("    add x15, x15, #48")
	g.line("    strb w15, [x9, #-1]!")
	g.line("    add x12, x12, #1")
	g.line("    mov x10, x14")
	g.line("    cbnz x10, .Lpin_loop")
	g.line("    b .Lpin_write")
	g.line(".Lpin_zero:")
	g.line("    mov x15, #48")
	g.line("    strb w15, [x9, #-1]!")
	g.line("    mov x12, #1")
	g.line(".Lpin_write:")
	g.line("    mov x0, x9")
	g.line("    mov x1, x12")
	g.line("    bl __nova_write_raw")
	g.line("    ldp x29, x30, [sp], #64")
	g.line("    ret")
	g.blank()

	g.line("__nova_print_float:")
	g.line("    stp x29, x30, [sp, #-64]!")
	g.line("    mov x29, sp")
	g.line("    str x19, [sp, #16]")
	g.line("    str x20, [sp, #24]")
	g.line("    fmov d0, x0")
	g.line("    fcmp d0, #0.0")
	g.line("    b.ge .Lpf_pos")
	g.line("    add x0, sp, #48")
	g.line("    mov w1, #45")
	g.line("    strb w1, [x0]")
	g.line("    add x0, sp, #48")
	g.line("    mov x1, #1")
	g.line("    bl __nova_write_raw")
	g.line("    fneg d0, d0")
	g.line(".Lpf_pos:")
	g.line("    fcvtzs x19, d0")
	g.line("    scvtf d1, x19")
	g.line("    fsub d0, d0, d1")
	g.line("    mov x0, x19")
	g.line("    bl __nova_print_int_nonl")
	g.line("    adrp x0, .Ldot")
	g.line("    add x0, x0, :lo12:.Ldot")
	g.line("    mov x1, #1")
	g.line("    bl __nova_write_raw")
	g.line("    mov x20, #6")
	g.line("    mov x19, #10")
	g.line("    scvtf d2, x19")
	g.line(".Lpf_frac:")
	g.line("    fmul d0, d0, d2")
	g.line("    fcvtzs x9, d0")
	g.line("    scvtf d1, x9")
	g.line("    fsub d0, d0, d1")
	g.line("    add x9, x9, #48")
	g.line("    add x0, sp, #48")
	g.line("    strb w9, [x0]")
	g.line("    add x0, sp, #48")
	g.line("    mov x1, #1")
	g.line("    bl __nova_write_raw")
	g.line("    subs x20, x20, #1")
	g.line("    b.ne .Lpf_frac")
	g.line("    adrp x0, .Lnewline")
	g.line("    add x0, x0, :lo12:.Lnewline")
	g.line("    mov x1, #1")
	g.line("    bl __nova_write_raw")
	g.line("    ldr x19, [sp, #16]")
	g.line("    ldr x20, [sp, #24]")
	g.line("    ldp x29, x30, [sp], #64")
	g.line("    ret")
	g.blank()

	g.line("__nova_read_str:")
	g.line("    stp x29, x30, [sp, #-16]!")
	g.line("    mov x29, sp")
	g.line("    adrp x0, .Linput_buf")
	g.line("    add x0, x0, :lo12:.Linput_buf")
	g.line("    mov x1, x0")
	g.line("    mov x2, #255")
	g.line("    mov x0, #0")
	g.line("    mov x8, #63")
	g.line("    svc #0")
	g.line("    adrp x9, .Linput_buf")
	g.line("    add x9, x9, :lo12:.Linput_buf")
	g.line("    cbz x0, .Lrs_done")
	g.line("    add x10, x9, x0")
	g.line("    sub x10, x10, #1")
	g.line("    ldrb w11, [x10]")
	g.line("    cmp w11, #10")
	g.line("    b.ne .Lrs_done")
	g.line("    strb wzr, [x10]")
	g.line(".Lrs_done:")
	g.line("    mov x0, x9")
	g.line("    ldp x29, x30, [sp], #16")
	g.line("    ret")
	g.blank()

	g.line(".section .rodata")
	g.line(".Lnewline:  .byte 10")
	g.line(".Ldot:      .byte 46")
	g.line(".Lbool_true:  .asciz \"true\"")
	g.line(".Lbool_false: .asciz \"false\"")
	g.blank()
	g.line(".section .bss")
	g.line(".Linput_buf: .space 256")
	g.blank()
	g.line(".section .text")
	g.blank()
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (g *Codegen) emitMovImm(val int64) {
	g.emitMovImmU(uint64(val))
}

func (g *Codegen) emitMovImmU(val uint64) {
	g.linef("    mov x0, #0x%x", val&0xFFFF)
	if (val>>16)&0xFFFF != 0 {
		g.linef("    movk x0, #0x%x, lsl #16", (val>>16)&0xFFFF)
	}
	if (val>>32)&0xFFFF != 0 {
		g.linef("    movk x0, #0x%x, lsl #32", (val>>32)&0xFFFF)
	}
	if (val>>48)&0xFFFF != 0 {
		g.linef("    movk x0, #0x%x, lsl #48", (val>>48)&0xFFFF)
	}
}

func (g *Codegen) internStr(s string) int {
	for i, lit := range g.strLits {
		if lit == s {
			return i
		}
	}
	idx := len(g.strLits)
	g.strLits = append(g.strLits, s)
	return idx
}

func (g *Codegen) label(prefix string) string {
	g.ctx.labelCount++
	return fmt.Sprintf(".L%s_%s_%d", g.ctx.name, prefix, g.ctx.labelCount)
}

func (g *Codegen) inferLValueType(lv ast.LValue) *ast.TypeRef {
	switch l := lv.(type) {
	case *ast.LValueIdent:
		if v, ok := g.ctx.vars[l.Name]; ok {
			return v.typeRef
		}
	case *ast.LValueIndex:
		t := g.inferType(l.Object)
		if t != nil && t.Kind == ast.TypeArray {
			return t.ElemType
		}
	case *ast.LValueField:
		typeName := g.inferStructName(l.Object)
		if typeName != "" {
			if sd, ok := g.structs[typeName]; ok {
				for _, f := range sd.Fields {
					if f.Name == l.Field {
						return f.Type
					}
				}
			}
		}
	}
	return nil
}

func (g *Codegen) inferType(expr ast.Expr) *ast.TypeRef {
	switch e := expr.(type) {
	case *ast.IntLit:
		return &ast.TypeRef{Kind: ast.TypeInt}
	case *ast.FloatLit:
		return &ast.TypeRef{Kind: ast.TypeFloat}
	case *ast.StrLit:
		return &ast.TypeRef{Kind: ast.TypeStr}
	case *ast.BoolLit:
		return &ast.TypeRef{Kind: ast.TypeBool}
	case *ast.ArrayLit:
		return &ast.TypeRef{Kind: ast.TypeArray}
	case *ast.Ident:
		if v, ok := g.ctx.vars[e.Name]; ok {
			return v.typeRef
		}
	case *ast.InputExpr:
		return &ast.TypeRef{Kind: ast.TypeStr}
	case *ast.LenExpr:
		return &ast.TypeRef{Kind: ast.TypeInt}
	case *ast.BinOp:
		switch e.Op {
		case "<", ">", ">=", "<=", "==", "!=", "and", "or":
			return &ast.TypeRef{Kind: ast.TypeBool}
		default:
			return g.inferType(e.Left)
		}
	case *ast.IndexExpr:
		t := g.inferType(e.Object)
		if t != nil && t.Kind == ast.TypeArray && t.ElemType != nil {
			return t.ElemType
		}
	case *ast.FieldExpr:
		typeName := g.inferStructName(e.Object)
		if typeName != "" {
			if sd, ok := g.structs[typeName]; ok {
				for _, f := range sd.Fields {
					if f.Name == e.Field {
						return f.Type
					}
				}
			}
		}
	case *ast.CallExpr:
		if fn, ok := g.funcs[e.Callee]; ok {
			return fn.ReturnType
		}
	case *ast.UnaryOp:
		if e.Op == "not" {
			return &ast.TypeRef{Kind: ast.TypeBool}
		}
		return g.inferType(e.Operand)
	case *ast.AppendExpr:
		return g.inferType(e.Array)
	}
	return &ast.TypeRef{Kind: ast.TypeInt}
}

func (g *Codegen) line(s string) {
	g.out.WriteString(s)
	g.out.WriteByte('\n')
}

func (g *Codegen) linef(f string, args ...interface{}) {
	g.out.WriteString(fmt.Sprintf(f, args...))
	g.out.WriteByte('\n')
}

func (g *Codegen) blank() {
	g.out.WriteByte('\n')
}
