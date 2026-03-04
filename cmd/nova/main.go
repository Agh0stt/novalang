package main

import (
	"fmt"
	"novalang/ast"
	"novalang/checker"
	"novalang/codegen"
	"novalang/lexer"
	"novalang/parser"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const version = "0.2.0"

func usage() {
	fmt.Fprintf(os.Stderr, `Nova Language Compiler v%s

Usage:
  nova [options] <file.nv>

Options:
  -o <out>   Output binary name (default: input name without extension)
  -S         Emit assembly only (.s file), do not assemble/link
  -tokens    Dump lexer tokens then exit
  -ast       Dump parsed AST then exit
  -v         Show version
  -h         Show this help

Features:
  - Structs:    struct Point: x: int  y: int  endstruct
  - Typedef:    typedef Meters = int
  - Arrays:     arr: [int] = [1, 2, 3]  |  len(arr)  |  append(arr, 4)
  - Import:     import "other.nv"
  - Break/continue in loops
  - Compound assignment: +=  -=  *=  /=
  - Modulo:     %%
  - not keyword for boolean negation
  - Field access: point.x
  - Index access: arr[i]

Examples:
  nova hello.nv           # compile to native binary (AArch64 Linux)
  nova -S hello.nv        # emit hello.s assembly only
  nova -tokens hello.nv   # debug: show token stream
  nova -ast hello.nv      # debug: show parsed AST
`, version)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	var (
		inputFile  string
		outputFile string
		emitASM    bool
		dumpTokens bool
		dumpAST    bool
	)

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			usage()
		case "-v", "--version":
			fmt.Printf("Nova v%s\n", version)
			os.Exit(0)
		case "-S":
			emitASM = true
		case "-tokens":
			dumpTokens = true
		case "-ast":
			dumpAST = true
		case "-o":
			i++
			if i >= len(args) {
				fatal("-o requires an argument")
			}
			outputFile = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				fatal("unknown flag: %s", args[i])
			}
			if inputFile != "" {
				fatal("only one input file is supported")
			}
			inputFile = args[i]
		}
	}

	if inputFile == "" {
		usage()
	}

	// Parse the main file (and all imports recursively)
	prog, err := parseFile(inputFile, make(map[string]bool))
	if err != nil {
		fatal("%v", err)
	}

	if dumpTokens {
		src, _ := os.ReadFile(inputFile)
		tokens, lexErr := lexer.Tokenize(string(src))
		if lexErr != nil {
			fatal("%v", lexErr)
		}
		fmt.Println("=== TOKENS ===")
		for _, t := range tokens {
			fmt.Println(t)
		}
		os.Exit(0)
	}

	if dumpAST {
		fmt.Println("=== AST ===")
		printProgram(prog)
		os.Exit(0)
	}

	// Type check
	chk := checker.New()
	if typeErr := chk.Check(prog); typeErr != nil {
		fatal("%v", typeErr)
	}

	// Code generation
	gen := codegen.New()
	asm := gen.Generate(prog)

	base := strings.TrimSuffix(inputFile, filepath.Ext(inputFile))
	asmFile := base + ".s"
	objFile := base + ".o"
	if outputFile == "" {
		outputFile = base
	}

	if err := os.WriteFile(asmFile, []byte(asm), 0644); err != nil {
		fatal("cannot write assembly file %s: %v", asmFile, err)
	}
	fmt.Printf("[nova] assembly written: %s\n", asmFile)

	if emitASM {
		os.Exit(0)
	}

	// Assemble
	fmt.Printf("[nova] assembling: %s\n", objFile)
	asCmd := exec.Command("as", "-o", objFile, asmFile)
	asCmd.Stdout = os.Stdout
	asCmd.Stderr = os.Stderr
	if err := asCmd.Run(); err != nil {
		fatal("assembler failed: %v\n       assembly is at: %s", err, asmFile)
	}

	// Link
	fmt.Printf("[nova] linking: %s\n", outputFile)
	ldCmd := exec.Command("ld", "-o", outputFile, objFile, "-e", "main")
	ldCmd.Stdout = os.Stdout
	ldCmd.Stderr = os.Stderr
	if err := ldCmd.Run(); err != nil {
		fatal("linker failed: %v\n       object is at: %s\n       try: ld -o %s %s -e main",
			err, objFile, outputFile, objFile)
	}
	fmt.Printf("[nova] done: ./%s\n", outputFile)
}

// parseFile parses a .nv file and merges all imports into a single Program
func parseFile(path string, visited map[string]bool) (*ast.Program, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve path %s: %v", path, err)
	}
	if visited[absPath] {
		// Already imported — return empty program (no duplicates)
		return &ast.Program{}, nil
	}
	visited[absPath] = true

	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %v", path, err)
	}

	tokens, lexErr := lexer.Tokenize(string(src))
	if lexErr != nil {
		return nil, fmt.Errorf("%s: %v", path, lexErr)
	}

	p := parser.New(tokens)
	prog, parseErr := p.Parse()
	if parseErr != nil {
		return nil, fmt.Errorf("%s: %v", path, parseErr)
	}

	// Process imports: merge imported programs first
	merged := &ast.Program{}
	dir := filepath.Dir(absPath)

	for _, imp := range prog.Imports {
		importPath := filepath.Join(dir, imp.Path)
		imported, err := parseFile(importPath, visited)
		if err != nil {
			return nil, err
		}
		mergeProgram(merged, imported)
	}

	// Then merge this file's declarations
	mergeProgram(merged, prog)
	return merged, nil
}

// mergeProgram merges src into dst (imports are skipped — already resolved)
func mergeProgram(dst, src *ast.Program) {
	dst.Typedefs = append(dst.Typedefs, src.Typedefs...)
	dst.Structs = append(dst.Structs, src.Structs...)
	dst.Functions = append(dst.Functions, src.Functions...)
	// TopLevel is rebuilt implicitly; imports are resolved at this stage
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "nova error: "+format+"\n", args...)
	os.Exit(1)
}

func printProgram(prog *ast.Program) {
	fmt.Printf("Typedefs (%d):\n", len(prog.Typedefs))
	for _, td := range prog.Typedefs {
		fmt.Printf("  typedef %s = %s\n", td.Name, td.Aliased)
	}
	fmt.Printf("Structs (%d):\n", len(prog.Structs))
	for _, s := range prog.Structs {
		fmt.Printf("  struct %s:\n", s.Name)
		for _, f := range s.Fields {
			fmt.Printf("    %s: %s\n", f.Name, f.Type)
		}
	}
	fmt.Printf("Functions (%d):\n", len(prog.Functions))
	for _, fn := range prog.Functions {
		printFunc(fn, 0)
	}
}

func printFunc(fn *ast.FuncDecl, depth int) {
	indent := strings.Repeat("  ", depth)
	params := make([]string, len(fn.Params))
	for i, p := range fn.Params {
		params[i] = fmt.Sprintf("%s: %s", p.Name, p.Type)
	}
	fmt.Printf("%sfunc %s(%s) -> %s\n", indent, fn.Name, strings.Join(params, ", "), fn.ReturnType)
	printStmts(fn.Body, depth+1)
	fmt.Printf("%send\n", indent)
}

func printStmts(stmts []ast.Stmt, depth int) {
	for _, s := range stmts {
		printStmt(s, depth)
	}
}

func printStmt(stmt ast.Stmt, depth int) {
	indent := strings.Repeat("  ", depth)
	switch s := stmt.(type) {
	case *ast.FuncDecl:
		printFunc(s, depth)
	case *ast.StructDecl:
		fmt.Printf("%sstruct %s (%d fields)\n", indent, s.Name, len(s.Fields))
	case *ast.TypedefDecl:
		fmt.Printf("%stypedef %s = %s\n", indent, s.Name, s.Aliased)
	case *ast.VarDecl:
		fmt.Printf("%svar %s: %s = %s\n", indent, s.Name, s.Type, s.Value)
	case *ast.Assign:
		fmt.Printf("%s%s %s %s\n", indent, s.Target, s.Op, s.Value)
	case *ast.ReturnStmt:
		if s.Value != nil {
			fmt.Printf("%sreturn %s\n", indent, s.Value)
		} else {
			fmt.Printf("%sreturn\n", indent)
		}
	case *ast.PrintStmt:
		args := make([]string, len(s.Args))
		for i, a := range s.Args {
			args[i] = a.String()
		}
		fmt.Printf("%sprint(%s)\n", indent, strings.Join(args, ", "))
	case *ast.UndefStmt:
		fmt.Printf("%sundef %s\n", indent, s.Name)
	case *ast.BreakStmt:
		fmt.Printf("%sbreak\n", indent)
	case *ast.ContinueStmt:
		fmt.Printf("%scontinue\n", indent)
	case *ast.IfStmt:
		fmt.Printf("%sif (%s):\n", indent, s.Condition)
		printStmts(s.Then, depth+1)
		for _, eif := range s.ElseIfs {
			fmt.Printf("%selif (%s):\n", indent, eif.Condition)
			printStmts(eif.Body, depth+1)
		}
		if len(s.Else) > 0 {
			fmt.Printf("%selse:\n", indent)
			printStmts(s.Else, depth+1)
		}
		fmt.Printf("%sendif\n", indent)
	case *ast.WhileStmt:
		fmt.Printf("%swhile (%s):\n", indent, s.Condition)
		printStmts(s.Body, depth+1)
		fmt.Printf("%sendwhile\n", indent)
	case *ast.ForStmt:
		fmt.Printf("%sfor (...):\n", indent)
		printStmts(s.Body, depth+1)
		fmt.Printf("%sendfor\n", indent)
	case *ast.ExprStmt:
		fmt.Printf("%s%s\n", indent, s.Expr)
	}
}
