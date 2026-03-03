// Package codegen emits Linux AArch64 assembly — NO libc.
//
// All I/O is done via raw Linux syscalls:
//   sys_read  = 63   (fd=0, buf, len)
//   sys_write = 64   (fd=1, buf, len)
//   sys_exit  = 93   (code)
//
// We ship our own integer/float/bool→string conversion routines as
// hand-emitted assembly helpers at the bottom of every output file.
//
// Register conventions (AAPCS64):
//   x0-x7   arguments / return values
//   x8      syscall number
//   x9-x15  caller-saved temporaries
//   x19-x28 callee-saved (we use x19-x20 for scratch in helpers)
//   x29     frame pointer
//   x30     link register
//   sp      stack pointer (must stay 16-byte aligned)
//
// ELF/GAS notes:
//   - Symbols have NO leading underscore (Linux convention)
//   - Local labels start with .L
//   - adrp + add :lo12: for PC-relative data access

package codegen

import (
	"fmt"
	"math"
	"novalang/ast"
	"strings"
)

const ptrSize = 8

type variable struct {
	offset  int
	varType ast.TypeKind
}

type funcCtx struct {
	name       string
	vars       map[string]*variable
	frameSize  int
	labelCount int
}

// Codegen holds all compilation state for one translation unit
type Codegen struct {
	out     strings.Builder
	funcs   map[string]*ast.FuncDecl
	strLits []string
	ctx     *funcCtx
}

func New() *Codegen {
	return &Codegen{funcs: make(map[string]*ast.FuncDecl)}
}

// ─── Top-level ────────────────────────────────────────────────────────────────

func (g *Codegen) Generate(prog *ast.Program) string {
	for _, fn := range prog.Functions {
		g.funcs[fn.Name] = fn
	}

	// Text section header
	g.line(".section .text")
	g.line(".globl main")
	g.line(".align 2")
	g.blank()

	// Emit all user functions
	for _, fn := range prog.Functions {
		g.genFunc(fn)
		g.blank()
	}

	// Emit built-in runtime helpers (no libc)
	g.emitRuntime()

	// Read-only data section
	g.line(".section .rodata")
	for i, s := range g.strLits {
		g.linef(".Lstr%d:", i)
		// Emit each byte explicitly so we handle escapes correctly,
		// then NUL-terminate with .Lstr%d_end for length calculation
		g.linef("    .asciz %q", s)
		g.linef(".Lstr%d_end:", i)
	}

	return g.out.String()
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

	// Round frame to 16 bytes, plus 16 for saved fp+lr
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

	// Spill params (x0..xN → stack)
	for i, p := range fn.Params {
		v := g.ctx.vars[p.Name]
		g.linef("    str x%d, [x29, #%d]   // param %s", i, v.offset, p.Name)
	}

	g.genBlock(fn.Body)

	// Default return path
	g.linef(".L%s_ret:", fn.Name)
	if fn.Name == "main" {
		// Exit via syscall — don't fall through
		g.line("    mov x0, #0")
		g.line("    mov x8, #93       // sys_exit")
		g.line("    svc #0")
	} else {
		if fn.ReturnType == ast.TypeVoid {
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

func (g *Codegen) allocVar(name string, t ast.TypeKind) *variable {
	if v, ok := g.ctx.vars[name]; ok {
		return v
	}
	g.ctx.frameSize += ptrSize
	v := &variable{offset: -g.ctx.frameSize, varType: t}
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
		g.genExpr(s.Value)
		v := g.ctx.vars[s.Name]
		g.linef("    str x0, [x29, #%d]   // var %s", v.offset, s.Name)

	case *ast.Assign:
		g.genExpr(s.Value)
		v := g.ctx.vars[s.Name]
		g.linef("    str x0, [x29, #%d]   // assign %s", v.offset, s.Name)

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
	}
}

// genPrint emits a syscall write for any Nova type.
// For each type we call the appropriate __nova_print_* helper
// which writes directly to fd 1 using sys_write.
func (g *Codegen) genPrint(arg ast.Expr) {
	t := g.inferType(arg)
	g.genExpr(arg) // result in x0

	switch t {
	case ast.TypeStr:
		g.line("    bl __nova_print_str")
	case ast.TypeBool:
		g.line("    bl __nova_print_bool")
	case ast.TypeInt:
		g.line("    bl __nova_print_int")
	case ast.TypeFloat, ast.TypeDouble:
		// x0 holds raw IEEE 754 bits; helper expects that in x0
		g.line("    bl __nova_print_float")
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
	g.linef("%s:", top)
	g.genExpr(s.Condition)
	g.linef("    cbz x0, %s", end)
	g.genBlock(s.Body)
	g.linef("    b %s", top)
	g.linef("%s:", end)
}

func (g *Codegen) genFor(s *ast.ForStmt) {
	top := g.label("fortop")
	end := g.label("forend")
	g.genStmt(s.Init)
	g.linef("%s:", top)
	g.genExpr(s.Condition)
	g.linef("    cbz x0, %s", end)
	g.genBlock(s.Body)
	g.genStmt(s.Post)
	g.linef("    b %s", top)
	g.linef("%s:", end)
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

	case *ast.Ident:
		v, ok := g.ctx.vars[e.Name]
		if !ok {
			panic(fmt.Sprintf("codegen: undefined var %q", e.Name))
		}
		g.linef("    ldr x0, [x29, #%d]   // %s", v.offset, e.Name)

	case *ast.UnaryOp:
		g.genExpr(e.Operand)
		if e.Op == "-" {
			g.line("    neg x0, x0")
		}

	case *ast.BinOp:
		g.genBinOp(e)

	case *ast.CallExpr:
		g.genCall(e)

	case *ast.InputExpr:
		g.genInput(e)
	}
}

func (g *Codegen) genBinOp(e *ast.BinOp) {
	g.genExpr(e.Left)
	g.line("    str x0, [sp, #-16]!") // push left
	g.genExpr(e.Right)
	g.line("    mov x1, x0")          // x1 = right
	g.line("    ldr x0, [sp], #16")   // pop left → x0

	switch e.Op {
	case "+":
		g.line("    add x0, x0, x1")
	case "-":
		g.line("    sub x0, x0, x1")
	case "*":
		g.line("    mul x0, x0, x1")
	case "/":
		g.line("    sdiv x0, x0, x1")
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
	// If there's a prompt, print it first
	if e.Prompt != nil {
		g.genExpr(e.Prompt)
		g.line("    bl __nova_print_str")
	}
	// Read a line from stdin into a stack buffer
	// Returns pointer to buffer in x0
	g.line("    bl __nova_read_str")
}

// ─── Runtime helpers (no libc) ────────────────────────────────────────────────

// emitRuntime emits all the __nova_* helper functions in assembly.
// These are linked into every Nova binary and replace libc I/O entirely.
func (g *Codegen) emitRuntime() {
	g.line("// ═══════════════════════════════════════════════")
	g.line("// Nova runtime — pure Linux syscalls, no libc")
	g.line("// ═══════════════════════════════════════════════")
	g.blank()

	// ── __nova_write_raw(x0=buf, x1=len) ─────────────────────────────────────
	// Raw sys_write to stdout. Clobbers x8. Returns bytes written in x0.
	g.line("__nova_write_raw:")
	g.line("    mov x2, x1          // len")
	g.line("    mov x1, x0          // buf")
	g.line("    mov x0, #1          // fd = stdout")
	g.line("    mov x8, #64         // sys_write")
	g.line("    svc #0")
	g.line("    ret")
	g.blank()

	// ── __nova_print_str(x0=ptr) ──────────────────────────────────────────────
	// Prints a NUL-terminated string followed by newline.
	// Computes length by scanning for NUL, then calls sys_write twice.
	g.line("__nova_print_str:")
	g.line("    stp x29, x30, [sp, #-32]!")
	g.line("    mov x29, sp")
	g.line("    str x19, [sp, #16]")
	g.line("    mov x19, x0         // save ptr")
	// strlen
	g.line("    mov x1, x0")
	g.line(".Lps_loop:")
	g.line("    ldrb w2, [x1], #1")
	g.line("    cbnz w2, .Lps_loop")
	g.line("    sub x1, x1, x19")
	g.line("    sub x1, x1, #1     // length (excl NUL)")
	// write string
	g.line("    mov x0, x19")
	g.line("    bl __nova_write_raw")
	// write newline
	g.line("    adrp x0, .Lnewline")
	g.line("    add x0, x0, :lo12:.Lnewline")
	g.line("    mov x1, #1")
	g.line("    bl __nova_write_raw")
	g.line("    ldr x19, [sp, #16]")
	g.line("    ldp x29, x30, [sp], #32")
	g.line("    ret")
	g.blank()

	// ── __nova_print_bool(x0=0|1) ─────────────────────────────────────────────
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

	// ── __nova_print_int(x0=int64) ────────────────────────────────────────────
	// Converts signed 64-bit integer to decimal ASCII on the stack, then
	// writes it + newline via sys_write.
	g.line("__nova_print_int:")
	g.line("    stp x29, x30, [sp, #-64]!")
	g.line("    mov x29, sp")
	// x9 = digit buffer pointer (grows downward from sp+56)
	g.line("    add x9, sp, #56")
	g.line("    mov x10, x0         // value")
	g.line("    mov x11, #0         // negative flag")
	// handle negative
	g.line("    cmp x10, #0")
	g.line("    b.ge .Lpi_pos")
	g.line("    mov x11, #1")
	g.line("    neg x10, x10")
	g.line(".Lpi_pos:")
	// convert digits
	g.line("    mov x12, #0         // digit count")
	g.line("    mov x13, #10")
	g.line(".Lpi_loop:")
	g.line("    udiv x14, x10, x13")
	g.line("    msub x15, x14, x13, x10  // x15 = x10 mod 10")
	g.line("    add x15, x15, #48   // ASCII '0'")
	g.line("    strb w15, [x9, #-1]!")
	g.line("    add x12, x12, #1")
	g.line("    mov x10, x14")
	g.line("    cbnz x10, .Lpi_loop")
	// prepend '-' if negative
	g.line("    cbz x11, .Lpi_write")
	g.line("    mov x15, #45        // '-'")
	g.line("    strb w15, [x9, #-1]!")
	g.line("    add x12, x12, #1")
	g.line(".Lpi_write:")
	g.line("    mov x0, x9")
	g.line("    mov x1, x12")
	g.line("    bl __nova_write_raw")
	// newline
	g.line("    adrp x0, .Lnewline")
	g.line("    add x0, x0, :lo12:.Lnewline")
	g.line("    mov x1, #1")
	g.line("    bl __nova_write_raw")
	g.line("    ldp x29, x30, [sp], #64")
	g.line("    ret")
	g.blank()

	// ── __nova_print_float(x0=IEEE754 bits) ──────────────────────────────────
	// Very simple float printer: prints up to 6 decimal places.
	// Algorithm: separate integer and fractional parts, print both.
	// x0 holds raw float64 bits — move to d0 first.
	g.line("__nova_print_float:")
	g.line("    stp x29, x30, [sp, #-64]!")
	g.line("    mov x29, sp")
	g.line("    str x19, [sp, #16]")
	g.line("    str x20, [sp, #24]")
	g.line("    fmov d0, x0         // bits → fp register")
	// check negative
	g.line("    fcmp d0, #0.0")
	g.line("    b.ge .Lpf_pos")
	g.line("    // print '-'")
	g.line("    add x0, sp, #48")
	g.line("    mov w1, #45")
	g.line("    strb w1, [x0]")
	g.line("    mov x0, sp")
	g.line("    add x0, x0, #48")
	g.line("    mov x1, #1")
	g.line("    bl __nova_write_raw")
	g.line("    fneg d0, d0")
	g.line(".Lpf_pos:")
	// integer part via fcvtzs
	g.line("    fcvtzs x19, d0      // integer part")
	g.line("    scvtf d1, x19       // back to float")
	g.line("    fsub d0, d0, d1     // fractional part")
	// print integer part
	g.line("    mov x0, x19")
	g.line("    bl __nova_print_int_nonl  // int without newline")
	// print '.'
	g.line("    adrp x0, .Ldot")
	g.line("    add x0, x0, :lo12:.Ldot")
	g.line("    mov x1, #1")
	g.line("    bl __nova_write_raw")
	// print 6 fractional digits
	g.line("    mov x20, #6         // digit count")
	g.line("    mov x19, #10")
	g.line("    scvtf d2, x19       // d2 = 10.0")
	g.line(".Lpf_frac:")
	g.line("    fmul d0, d0, d2     // shift left one decimal")
	g.line("    fcvtzs x9, d0       // digit")
	g.line("    scvtf d1, x9")
	g.line("    fsub d0, d0, d1     // remove digit")
	g.line("    add x9, x9, #48     // ASCII")
	g.line("    add x0, sp, #48")
	g.line("    strb w9, [x0]")
	g.line("    mov x0, sp")
	g.line("    add x0, x0, #48")
	g.line("    mov x1, #1")
	g.line("    bl __nova_write_raw")
	g.line("    subs x20, x20, #1")
	g.line("    b.ne .Lpf_frac")
	// newline
	g.line("    adrp x0, .Lnewline")
	g.line("    add x0, x0, :lo12:.Lnewline")
	g.line("    mov x1, #1")
	g.line("    bl __nova_write_raw")
	g.line("    ldr x19, [sp, #16]")
	g.line("    ldr x20, [sp, #24]")
	g.line("    ldp x29, x30, [sp], #64")
	g.line("    ret")
	g.blank()

	// ── __nova_print_int_nonl(x0=int64) ──────────────────────────────────────
	// Same as __nova_print_int but NO trailing newline. Used by float printer.
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

	// ── __nova_read_str() ─────────────────────────────────────────────────────
	// Reads a line from stdin via sys_read.
	// Allocates 256 bytes on the CALLER's stack (caller must reserve space).
	// Returns pointer to buffer in x0. Strips trailing newline.
	// NOTE: for v1 we read into a static buffer — good enough for bootstrapping.
	g.line("__nova_read_str:")
	g.line("    stp x29, x30, [sp, #-16]!")
	g.line("    mov x29, sp")
	g.line("    adrp x0, .Linput_buf")
	g.line("    add x0, x0, :lo12:.Linput_buf")
	g.line("    mov x1, x0          // buf")
	g.line("    mov x2, #255        // max bytes")
	g.line("    mov x0, #0          // fd = stdin")
	g.line("    mov x8, #63         // sys_read")
	g.line("    svc #0              // x0 = bytes read")
	// strip trailing newline
	g.line("    adrp x9, .Linput_buf")
	g.line("    add x9, x9, :lo12:.Linput_buf")
	g.line("    cbz x0, .Lrs_done")
	g.line("    add x10, x9, x0")
	g.line("    sub x10, x10, #1    // last byte")
	g.line("    ldrb w11, [x10]")
	g.line("    cmp w11, #10        // newline?")
	g.line("    b.ne .Lrs_done")
	g.line("    strb wzr, [x10]     // NUL-terminate")
	g.line(".Lrs_done:")
	g.line("    mov x0, x9          // return buf ptr")
	g.line("    ldp x29, x30, [sp], #16")
	g.line("    ret")
	g.blank()

	// Runtime data (in .bss / .data sections)
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

func (g *Codegen) inferType(expr ast.Expr) ast.TypeKind {
	switch e := expr.(type) {
	case *ast.IntLit:
		return ast.TypeInt
	case *ast.FloatLit:
		return ast.TypeFloat
	case *ast.StrLit:
		return ast.TypeStr
	case *ast.BoolLit:
		return ast.TypeBool
	case *ast.Ident:
		if v, ok := g.ctx.vars[e.Name]; ok {
			return v.varType
		}
	case *ast.InputExpr:
		return ast.TypeStr
	case *ast.BinOp:
		switch e.Op {
		case "<", ">", ">=", "<=", "==", "!=", "and", "or":
			return ast.TypeBool
		default:
			return g.inferType(e.Left)
		}
	}
	return ast.TypeInt
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
