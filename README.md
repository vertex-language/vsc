# vsc

The Vertex Source Compiler. It takes Vertex source and produces a native
Mach-O executable — no assembler, no linker, no `cc`, nothing from a
platform toolchain. The instruction selector, the object writer and
the linker are all libraries in this project's family, so a machine
with none of that installed still builds and runs a program.

For interop with Swift's ecosystem, see [Compatibility](#compatibility).

## The language

A program is functions. `main` returning `int32` is the entry point,
and what it returns is the process exit status. There is no top-level
code yet — see `TODO.md`.

```swift
func fib(_ n: int32) -> int32 {
    if n < 2 { return n }
    return fib(n - 1) + fib(n - 2)
}

func main() -> int32 {
    return fib(10)
}
```

### Primitives

Types are spelled in lowercase:

| Vertex | What it is |
| --- | --- |
| `bool` | true or false |
| `int`, `int8`, `int16`, `int32`, `int64` | signed integers; `int` is 64-bit |
| `uint`, `uint8`, `uint16`, `uint32`, `uint64` | unsigned; `uint` is 64-bit |
| `float`, `float32` | 32-bit binary float |
| `double`, `float64` | 64-bit binary float |
| `string`, `char` | text, and one character of it |
| `void`, `never`, `any` | no value, no return, any value |

Each aliases a capitalised counterpart — see [Grammar](#grammar).

### Designed, not yet built

On record, but not built: all of this is a syntax error today.

**Packages and path-based imports.** File-level package declarations
and path imports, resolving underneath to the module and ownership
semantics described under [Modules](#modules):

```swift
package main

import (
    "github.com/username/repo1"
    "github.com/username/repo2"
)

func main() -> int32 {
    yourpackage.Something()
    return 0
}
```

**Receiver methods.** Methods declared outside the host type's body,
across `struct`, `enum` and `class`, to keep large packages flatter
while preserving the ownership conventions:

```swift
struct vec2 {
    var x: float32
    var y: float32
}

func (v: borrowing vec2) length() -> float32 {
    return (v.x * v.x + v.y * v.y).squareRoot()
}

func (v: inout vec2) scale(k: float32) {
    v.x *= k
    v.y *= k
}
```

**`kernel` and `graph`.** Two reserved function modifiers for compute
and dataflow targets — a data-parallel unit and a graph node.
Reserved is all they are: no syntax, no semantics, no backend.

```swift
func add() kernel -> float32 { }   // accelerated execution
func add() graph  -> float32 { }   // graph-based pipelines
```

## Status

One target: `aarch64-macos`. Apple silicon, macOS.

What the language can express today is defined by `tests/compiler/` —
156 whole programs the compiler builds and runs. A program goes in
there once it works, and a refusal fails the suite rather than being
skipped, so the corpus is a statement of what works rather than a
wishlist. Roughly:

- functions, recursion, argument labels, default arguments, `inout`
- `struct`, `class`, `enum` with payloads, inheritance, initializers
- generics, constraints, `where` clauses, associated types
- protocols and existentials, including dispatch through them
- optionals, tuples, closures, computed properties, nested types
- `switch` and pattern matching, operators and precedence groups
- the integer and floating-point widths, and conversions between them

Not there yet: `async`/`await` and actors; `throws` past the interface
boundary — it typechecks and it can call a throwing imported function,
but no compiled program here raises one; the `weak` and `unowned`
semantics that break reference cycles, which parse but carry no
meaning yet; and reflection.

## Install

The repositories are separate Go modules and find each other by
relative path, so they must be checked out beside one another:

```
parent/
  vsc/     this repository
  ir/      the machine IR, and lower/ inside it
  arm64/   the AArch64 assembler, with asm/ beside it
  macho/   the Mach-O reader, writer and linker
```

Then build the command (Go 1.23 or later), and put it on your `PATH`:

```bash
cd vsc/cmd && go build -o bin/vsc ./vsc
```

`cmd` is its own module, so `go build ./...` from the repository root
will not build the command — building it needs the backend.

## Quick start

Put the program above in `fib.vs` and:

```bash
vsc run fib.vs; echo $?    # 55
vsc check fib.vs           # typecheck only, no build
```

Errors come with the source line and a caret:

```
bad.vs:2:18: error: cannot convert value of type 'String' to specified type 'Int'
        let x: Int = "hello"
                     ^
```

Exit codes: `0` no errors, `1` diagnostics with errors, `2` a bad
invocation or an I/O failure — so `vsc check f.vs && echo ok` means
what it looks like.

## The command

```
vsc build  [flags] [files...]   compile and link; with --emit, stop earlier
vsc run    [flags] [files...]   build to a temporary path and run it
vsc check  [flags] [files...]   parse and typecheck; print diagnostics
vsc ast    [flags] [file]       parse and dump the syntax tree
vsc tokens [flags] [file]       dump the token stream
vsc env    [flags]              print the resolved target and SDK
```

A file of `-`, or no file at all, reads standard input.

| Flag | Meaning |
| --- | --- |
| `-target T` | target to build for (default: this host) |
| `-module name` | the module being compiled (default: `main`) |
| `-o file` | write output here; `-` is standard output |
| `-I dir` | look for imported modules here (repeatable) |
| `-entry sym` | the program's entry symbol |
| `-freestanding` | link no platform libraries |

`build` and `run` also take `--emit`:

| `--emit` | Stops after |
| --- | --- |
| `exe` | compile and link (the default) |
| `obj` | an object file |
| `vil` | the ownership IR |
| `vir` | the machine IR |
| `interface` | the module's public face, for another module to compile against |

Not yet: `--emit asm`, `--emit device`, `-L`, `-l`, `-static`, and
cross-target builds. The target table has one row.

## Modules

A module is imported by name. `import Geometry` looks for
`Geometry.vertexinterface` in each `-I` directory, in order, and takes
the first.

An interface is *source* — the language with the bodies taken out —
which is why compiling against one needs no separate binary module
format:

```swift
// vertex-interface-format-version: 1.0
// vertex-module-name: Metrics

public func mean(_ a: int32, _ b: int32) -> int32
```

`vsc build --emit interface` writes one.

The module name decides the entry point: `main` in module `main` is
the program's, every other module's `main` is an ordinary function.
That is why `-module` defaults to `main`, and why building a library
means saying so.

## Compatibility

Vertex looks to support as much of Swift's grammar as it can, for
interop-related tasks of Swift's ecosystem, when possible.

### Grammar

A valid Swift file is a valid Vertex file. Every Vertex addition is an
opt-in on top, and none of them changes the meaning of a program that
does not use them.

That is why the primitives have capitalised counterparts: `int32` and
`Int32` denote one type. Swift's own names resolve first, so a program
that writes only those reads exactly the universe `swiftc` does, and
the checker keeps a Swift-only view of the universe for the times that
distinction matters.

### Argument labels

*Designed, not yet built — an unlabelled call is an error today.*

Labels are part of what Vertex carries for compatibility. Write them
or leave them out, in either direction: `_` on a parameter is
optional, and so is the label at the call.

```swift
func addUp(a: int32, b: int32) -> int32 { return a + b }

addUp(1, 2)         // no labels
addUp(a: 1, b: 2)   // labels, the Swift spelling
```

Both declaration forms accept both call forms. The one place a label
still decides something is overloading: where declarations differ only
by label, an unlabelled call is an error naming the candidates rather
than a guess, since `label(a:)` and `label(b:)` mangle to different
symbols and picking one would call a function the caller never named.

### The ecosystem

Vertex has no standard library of its own. A compiler does not have to
implement a library to call it — it has to agree with it about names
and calling conventions. So `String`, `Array`, `Codable` and
Foundation come from Swift's own library, built by `swiftc` and linked
against code this compiler produced.

A `.swiftinterface` that `swiftc` emitted can be read unedited, which
`tests/interop/006-swiftc-emitted-interface` holds the compiler to.

### VIL and SIL

The Vertex Intermediate Language mirrors SIL's grammar, its passes and
its naming, so `--emit vil` prints something a compiler engineer can
read without a legend:

```
sil_stage lowered

sil hidden @$s4main3fibyS2iF : $@convention(thin) (Int) -> Int {
bb0(%0 : $Int):
  debug_value %0, let, name "n", argno 1
  %1 = integer_literal $Builtin.Int64, 2
  ...
```

Sharing the vocabulary is also what makes the symbols line up: a name
this compiler mangles is the name `swiftc` mangles, which is the whole
of what a linker needs from both of them.

### How it is checked

`swiftc` is used as the oracle for the corpora. Nothing in them writes
down an expected value: a number beside a program is a hand-maintained
claim, wrong the moment it drifts, while `swiftc`'s answer cannot. So
the runners compile twice, run twice, and compare — exit status, or
the same signal for a program that traps.

## Go API

The compiler is a library first; the command is a thin wrapper over it.

```go
import "github.com/vertex-language/vsc"

target, _ := vsc.HostTarget()
unit, diags := vsc.Compile(
    []vsc.Source{{Name: "fib.vs", Text: src}},
    vsc.Options{Module: "main", Target: target},
)
if vsc.Errors(diags) {
    // report and stop
}
// unit.Files, unit.Info, unit.VIL, unit.VIR
```

`Compile` stops at the first phase that reports an error — at the
*end* of that phase, so a file with three type errors reports three
rather than one and its consequences. Diagnostics are returned, not
printed: how to show them is the caller's business.

`Options.Stop` takes it partway, through `Parsed`, `Checked`, `Raw`,
`Canonical` and `Lowered`; the zero value runs all of them, and a
`Unit` field is nil where its phase did not run.

Turning a `Unit` into an object or an executable is the separate
`vsc/build` module, so typechecking alone pulls in no backend.

## Architecture

```
source → scanner → parser → analyzer → vil/gen → vil/pass → lower → VIR → build
         tokens    AST      types      VIL       ownership  machine  obj + link
```

The seam is `lower`. Above it everything is the language — formal
types, ownership, the vocabulary a diagnostic is written in. Below it
is machine, and VIR is shared with the family's C and C++ compilers,
so nothing language-shaped may cross.

| Package | Lines | Does |
| --- | ---: | --- |
| `token` | 714 | positions, kinds, diagnostics |
| `scanner` | 1531 | source to tokens |
| `ast` | 2227 | the syntax tree; nodes hold no text |
| `parser` | 4477 | tokens to tree, with error recovery |
| `types` | 2137 | the type model |
| `analyzer` | 6150 | names, then types, then bodies |
| `core` | 430 | the built-in module: operators and layout |
| `vil` | 1990 | the ownership IR |
| `vil/gen` | 8864 | checked tree to raw VIL |
| `vil/pass` | 503 | the passes that must run |
| `vil/verify` | 1043 | checks the ownership rules |
| `lower` | 6018 | VIL to VIR |
| `mangle` | 1424 | declarations to symbol names |
| `iface` | 426 | reading and writing module interfaces |
| `build` | 420 | VIR to object, objects to executable |

Two rules the front end is built on, both worth knowing before reading
it:

**Where the checker does not know, it says nothing.** An invented type
is worse than no type, so a type the analyzer cannot work out is
`Invalid`, a diagnostic about an `Invalid` is not reported, and one
mistake in the source is one diagnostic in the output.

**Ownership is emitted, not inferred.** `vil/gen` writes down every
copy and destroy as it goes, so `vil/verify` checks the rules against
what was emitted rather than what a later pass hopes to work out.

## Tests

Four corpora, each asking one question. A file belongs to exactly one.

| Corpus | Size | Question |
| --- | ---: | --- |
| `tests/syntax/` | 94 files | Does it parse? |
| `tests/check/` | 62 files | Does it typecheck, and say the right thing when it does not? |
| `tests/compiler/` | 156 programs | Does the program do what it says? |
| `tests/interop/` | 20 cases | Does what this builds agree with the ecosystem it links against? |

```bash
go test ./...              # front end and CLI; ~20s
cd build && go test ./...  # the corpora; ~3 minutes
```

The `build` suite needs an Apple-silicon Mac, plus `swiftc` and
`clang` on the `PATH` for its half of every comparison. Without them
it skips rather than fails — there is no oracle to compare against.

## License

MIT. Copyright (c) 2026 Netangular Technologies.
