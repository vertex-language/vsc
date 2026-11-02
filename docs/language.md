# The language

A short tour of what compiles today. Every snippet in this section builds and
runs with the current vsc.

## Types

Primitive types have short lowercase names:

| Type | Is |
| :--- | :--- |
| `bool` | `true` or `false` |
| `int`, `int8` … `int64` | signed integers (`int` is pointer width) |
| `uint`, `uint8` … `uint64` | unsigned integers |
| `float32` (`float`), `float64` (`double`) | IEEE 754 floating point |
| `float16`, `bfloat16` | half floats, on the CPU and in kernels |
| `string`, `char` | UTF-8 text, and one extended grapheme cluster |
| `void`, `never`, `any` | the empty tuple, a call that does not return, a type-erased value |

Structs, enums with payloads, classes, protocols, generics, closures,
optionals, `throws`, and ARC-managed references are all there.

## Receiver methods

A method can be declared outside its type, with the receiver's ownership
written out:

```swift
struct Vec2 {
    var x: float32
    var y: float32
}

func (v: borrowing Vec2) Length() -> float32 {
    return (v.x * v.x + v.y * v.y).squareRoot()
}

func (v: inout Vec2) Scale(by k: float32) {
    v.x *= k
    v.y *= k
}

var v = Vec2(x: 3, y: 4)
v.Scale(by: 2)
print(v.Length())                  // 10.0
```

`borrowing` reads, `inout` mutates, `consuming` takes ownership. Receiver
methods are statically dispatched.

## Labels are optional where the call is clear

```swift
func add(a: int32, b: int32) -> int32 { a + b }

add(a: 1, b: 2)
add(1, 2)                          // the same call
```

A label is required only when leaving it out would make the call ambiguous.

Because any argument can be passed by position, Vertex has no `_` label.
`func add(_ a: int32, _ b: int32)` in a `.vs` or `.vsx` file is an error
(`expected parameter name, found '_'`): write `func add(a: int32, b: int32)`.
`_` is accepted only in `.swift` files, the Swift-compatible layer vsc
compiles for testing and interop, where labels keep Swift's rules.

## Enums and matching

```swift
enum Shape {
    case circle(radius: float64)
    case rect(w: float64, h: float64)
}

func area(s: Shape) -> float64 {
    switch s {
    case .circle(let r): return 3.14159 * r * r
    case .rect(let w, let h): return w * h
    }
}
```

## Concurrency

`async`/`await`, `async let`, task groups and actors run on vsc's own task
executor, which is linked into every program:

```swift
func work(n: int) async -> int { return n * n }

async let a = work(3)
async let b = work(4)
print(await a + b)                 // 25

let total = await withTaskGroup(of: int.self) { group in
    for i in 1...4 { group.addTask { await work(i) } }
    var s = 0
    for await v in group { s += v }
    return s
}

actor Counter {
    var n = 0
    func Bump() -> int { n += 1; return n }
}
```

Native code never blocks a worker thread. A package's file or socket call
returns "would block", and the task waits on the descriptor.

## Imports and conditional compilation

```swift
import (
    "fs"
    "net/http"
    bin "encoding/binary"          // an alias
)

#if os(macOS)
let platform = "mac"
#elseif os(Windows)
let platform = "windows"
#else
let platform = "other"
#endif
```

A quoted import names a package: a folder of source (see
[Packages](packages.md)).
