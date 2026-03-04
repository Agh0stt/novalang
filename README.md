# Nova Language v0.2.2

A statically-typed, compiled language that targets Linux AArch64 (ARM64) with **zero libc dependency** — all I/O is via raw syscalls.

## Building

```bash
cd novalang
go build -o nova ./cmd/nova
```

## Usage

```bash
nova hello.nv              # compile to binary
nova -S hello.nv           # emit assembly only
nova -ast hello.nv         # dump AST
nova -tokens hello.nv      # dump token stream
nova -o out hello.nv       # custom output name
```

---

## Language Reference

### Primitive Types

| Type     | Description             |
|----------|-------------------------|
| `int`    | 64-bit signed integer   |
| `float`  | 64-bit float            |
| `double` | 64-bit double           |
| `str`    | NUL-terminated string   |
| `bool`   | `true` / `false`        |
| `void`   | No return value         |

---

### Functions

```nova
func add(a: int, b: int) -> int:
    return a + b
end

func main() -> void:
    result: int = add(3, 4)
    print(result)
end
```

---

### Variables & Assignment

```nova
x: int = 10          # declare + init
x = 20               # reassign
x += 5               # compound: += -= *= /=
rem: int = 17 % 5    # modulo
```

---

### Arrays

Arrays are dynamically allocated sequences. All elements are 8-byte aligned.

```nova
# Array literal
nums: [int] = [1, 2, 3, 4, 5]

# Fixed-size type annotation (optional)
fixed: [int, 10] = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0]

# Length
n: int = len(nums)

# Index access & assignment
print(nums[0])    # read
nums[2] = 99      # write

# Append (returns new or same array pointer)
nums = append(nums, 100)

# Pass to function
func sum(arr: [int], n: int) -> int:
    total: int = 0
    for (i: int = 0, i < n, i += 1):
        total += arr[i]
    endfor
    return total
end
```

---

### Structs

```nova
struct Point:
    x: int
    y: int
endstruct

struct Color:
    r: int
    g: int
    b: int
endstruct

func main() -> void:
    p: Point = Point
    p.x = 10
    p.y = 20
    print(p.x)    # 10
    print(p.y)    # 20
end
```

Structs are stack-allocated. Field access uses `.` notation. Struct variables hold a pointer to their stack allocation.

---

### Typedef

Create type aliases for any type, including arrays, structs, and primitives:

```nova
typedef Meters = int
typedef IntArray = [int]
typedef Score = int

dist: Meters = 42        # same as int
arr: IntArray = [1,2,3]  # same as [int]
```

---

### Import

Split code across multiple `.nv` files:

```nova
# math_utils.nv
func abs_val(n: int) -> int:
    if (n < 0): return -n endif
    return n
end
```

```nova
# main.nv
import "math_utils.nv"

func main() -> void:
    print(abs_val(-7))  # 7
end
```

Circular imports are automatically detected and skipped. Imports are resolved relative to the importing file's directory.

---

### Control Flow

```nova
# if / elif / else / endif
if (x > 10):
    print("big")
elif (x == 10):
    print("ten")
else:
    print("small")
endif

# while / endwhile  (supports break, continue)
i: int = 0
while (i < 5):
    if (i == 3): continue endif
    print(i)
    i += 1
endwhile

# for / endfor  (supports break, continue)
for (j: int = 0, j < 10, j += 1):
    if (j == 7): break endif
    print(j)
endfor
```

---

### Operators

| Category    | Operators                          |
|-------------|------------------------------------|
| Arithmetic  | `+` `-` `*` `/` `%`               |
| Comparison  | `<` `>` `<=` `>=` `==` `!=`       |
| Logical     | `and` `or` `not`                  |
| Assignment  | `=` `+=` `-=` `*=` `/=`          |
| String      | `+` (concatenation)               |

---

### Built-ins

| Function         | Description                              |
|------------------|------------------------------------------|
| `print(v)`       | Print any value (adds newline)           |
| `input("prompt")`| Read a line from stdin, returns `str`   |
| `len(arr)`       | Length of array or string               |
| `append(arr, v)` | Append value to array, return new array  |
| `undef name`     | Remove a variable from scope            |

---

### Full Example

```nova
# game.nv
import "math_utils.nv"

typedef HP = int

struct Enemy:
    health: int
    damage: int
endstruct

func attack(e: Enemy, dmg: int) -> int:
    new_hp: int = e.health - dmg
    return clamp(new_hp, 0, 9999)
end

func main() -> void:
    boss: Enemy = Enemy
    boss.health = 500
    boss.damage = 25

    remaining: HP = attack(boss, 120)
    print(remaining)    # 380

    scores: [int] = [100, 200, 300]
    scores = append(scores, 400)
    print(len(scores))  # 4
    print(scores[3])    # 400
end
```

---

## Architecture

```
lexer/     → tokenizes .nv source
parser/    → recursive-descent parser → AST
ast/       → AST node types
checker/   → static type checker
codegen/   → AArch64 assembly emitter (no libc)
cmd/nova/  → CLI driver, import resolver
```

The compiler outputs raw AArch64 assembly, assembled with GNU `as` and linked with `ld`. No C runtime required.
