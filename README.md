# Nova — A Statically Typed, Compiled Language

Nova is a general-purpose, statically typed programming language that compiles to native **AArch64 (ARM64) assembly**. Clean syntax, no semicolons, no curly braces.

---

## Build the Compiler

Requires **Go 1.21+**

```bash
bash build.sh
# → produces ./nova binary
```

---

## Compile a Nova Program

```bash
./nova examples/hello.nv        # compile → native binary
./nova -S examples/hello.nv     # emit assembly only (hello.s)
./nova -tokens examples/hello.nv # debug: dump token stream
./nova -ast examples/hello.nv    # debug: dump AST
```

---

## Language Reference

### Types
| Type     | Description          |
|----------|----------------------|
| `int`    | 64-bit signed integer |
| `float`  | 64-bit float (double-precision) |
| `double` | alias for float      |
| `str`    | string               |
| `bool`   | true / false         |
| `void`   | no return value      |

---

### Entry Point
Every program starts at `main()`. Main may return `void` or `int`.

```nova
func main() -> void:
    print("hello world")
end
```

---

### Variables
Declared as `name: type = value`. No semicolons.

```nova
x: int = 42
name: str = "Alice"
flag: bool = true
pi: float = 3.14159
```

Reassignment:
```nova
x = x + 1
```

Delete a variable from scope:
```nova
undef x
```

---

### Functions

```nova
func add(a: int, b: int) -> int:
    return a + b
end

func greet(name: str) -> void:
    print("Hello, " + name)
end
```

---

### If / Elif / Else

```nova
if (x > 0):
    print("positive")
elif (x == 0):
    print("zero")
else:
    print("negative")
endif
```

---

### While Loop

```nova
i: int = 0
while (i < 10):
    print(i)
    i = i + 1
endwhile
```

---

### For Loop

```nova
for (i: int = 0, i < 10, i = i + 1):
    print(i)
endfor
```

---

### Built-in Functions

```nova
print(x)           # print any value (auto newline)
name: str = input()         # read a line from stdin
name: str = input("Enter: ")  # with optional prompt
```

---

### Operators

| Category    | Operators                          |
|-------------|------------------------------------|
| Arithmetic  | `+`, `-`, `*`, `/`                 |
| Comparison  | `<`, `>`, `>=`, `=<`, `==`, `!=`  |
| Logical     | `and`, `or`                        |

---

### Keywords

```
void int str bool float double
func return if elif else while for
print input undef
end endif endfor endwhile
true false and or
```

---

## Architecture

```
Source (.nv)
    │
    ▼
  Lexer          lexer/lexer.go      → []Token
    │
    ▼
  Parser         parser/parser.go    → AST
    │
    ▼
  Type Checker   checker/checker.go  → errors or OK
    │
    ▼
  Code Generator codegen/codegen.go  → AArch64 assembly (.s)
    │
    ▼
  clang/gcc                          → native binary
```

---

## Roadmap
- v0.1: Core language, AArch64 output, clang linkage
- v0.2: Arrays, imports, standard library
- v0.3: Self-hosting (Nova compiler written in Nova)
