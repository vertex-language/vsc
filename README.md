# Vertex Source Compiler (`vsc`)

A modern, self-contained native compiler and toolchain for the Vertex programming language. `vsc` transforms source code directly into standalone native executables (Mach-O and PE/COFF) without external platform toolchains—no system C compiler, assembler, or external linker required.

---

## Overview

Traditional language toolchains offload code generation, assembly, and linking to system tools such as `clang`, `gcc`, `as`, and `ld`. `vsc` takes a different approach: every stage of the pipeline—from lexical scanning and semantic analysis to SIL ownership verification, target machine lowering, object emission, and native binary linking—is implemented in pure Go within the Vertex toolchain family.

### Key Highlights

- **Zero External Dependencies**: Produces runnable Mach-O (ARM64 macOS) and PE/COFF (x86-64 Windows) binaries out of the box with built-in object emitters and linkers.
- **Full Swift Compatibility**: Conforms to Swift grammar, calling conventions, symbol mangling, and ABI layouts. Parses `.swiftinterface` and `.vinterface` files natively.
- **Embedded C++ Runtime**: Includes an in-tree standard library runtime (`stdlib/`) providing reference counting (ARC), Unicode-compliant UTF-8 strings, and dynamic arrays, compiled in-process via `vcx` and linked statically.
- **Multi-Stage Intermediate Representation**: Features Swift Intermediate Language (`internal/sil`) for ownership validation and Definite Initialization, lowered into target-independent Vertex IR (`ir`).
- **Flexible Workspace & Package Support**: Compiles single-file scripts, multi-module directories, and `Package.swift` workspaces.

---

## Quick Start

### 1. Build the Compiler

Ensure Go 1.23+ is installed, then build the CLI from the `cmd` module:

```bash
cd cmd
go build -o bin/vsc ./vsc
export PATH="$PWD/bin:$PATH"
```

### 2. Write a Program

Save the following program to `demo.vs`:

```swift
struct Vector2 {
    var x: float32
    var y: float32
}

func (v: borrowing Vector2) magnitudeSquared() -> float32 {
    return v.x * v.x + v.y * v.y
}

func main() -> int32 {
    let point = Vector2(x: 3.0, y: 4.0)
    return int32(point.magnitudeSquared()) // Returns 25
}
```

### 3. Compile and Run

Compile and execute immediately via the built-in scratch runner:

```bash
vsc run demo.vs
echo $?
# 25
```

Or build a standalone native executable:

```bash
vsc build -o demo demo.vs
./demo
echo $?
# 25
```

---

## CLI Reference

### Subcommands

| Command | Action | Typical Invocation |
| --- | --- | --- |
| `vsc build` | Compile source files and link native executable or object | `vsc build -o app main.vs` |
| `vsc run` | Compile to a temporary directory and execute immediately | `vsc run script.vs` |
| `vsc check` | Run parser and semantic typechecker without codegen | `vsc check src/*.vs` |
| `vsc ast` | Parse and print formatted Abstract Syntax Tree | `vsc ast main.vs` |
| `vsc tokens` | Tokenize input and stream tokens with source coordinates | `vsc tokens main.vs` |
| `vsc env` | Print resolved target architecture, OS, and SDK search paths | `vsc env` |

Inputs can be individual files, directory paths, or standard input (`-`).

### Options and Flags

```text
Flags:
  -target <triple>       Target platform triple: aarch64-macos | x86_64-windows (default: host)
  -module <name>         Module identity name (default: main)
  -o <path>              Output destination file path ("-" for stdout)
  -I <dir>               Module search path for .vinterface / .swiftinterface (repeatable)
  -P, --package-path <d> Package root directory for folder imports and Package.swift manifests
  -entry <symbol>        Custom binary entry point symbol (default: platform entry)
  -freestanding          Compile without linking system SDK libraries or C runtime
  --emit <phase>         Stop compilation and emit intermediate representation
```

### Intermediate Emission Targets (`--emit`)

| Stage | Target Flag | Output Extension | Description |
| --- | --- | --- | --- |
| Executable | `--emit exe` | *(none)* / `.exe` | Complete native linked binary (default) |
| Object File | `--emit obj` | `.o` / `.obj` | Assembled relocatable machine object |
| Machine IR | `--emit vir` | `.vir` | Vertex Intermediate Representation instructions |
| Ownership IR | `--emit sil` | `.sil` | Canonical SIL with explicit memory and ARC operations |
| Interface | `--emit interface` | `.vinterface` | Public module interface with stripped function bodies |

### Exit Status

- `0`: Operation succeeded cleanly.
- `1`: Diagnostic errors encountered during compilation.
- `2`: Command-line usage error or file I/O failure.

---

## Language Features

Vertex supports standard Swift syntax while adding ergonomic primitive aliases, receiver method definitions, directory module imports, and compute execution annotations.

### Sized Primitive Types
All built-in types have clean lowercase spellings alongside their standard capitalized Swift counterparts:

```swift
let flag: bool     = true              // Bool
let byte: uint8    = 255               // UInt8
let count: int32   = 42                // Int32
let wide: int      = 100_000           // Int (64-bit word)
let ratio: float32 = 3.14159           // Float
let precise: double = 2.718281828       // Double
let name: string   = "Vertex"          // String
let ch: char       = "V"               // Character
```

### Receiver Methods
In addition to standard extensions and type bodies, Vertex allows receiver-style function declarations with explicit ownership semantics:

```swift
struct Point {
    var x: float32
    var y: float32
}

// Immutable borrow receiver:
func (p: borrowing Point) distanceSquared() -> float32 {
    return p.x * p.x + p.y * p.y
}

// Mutating inout receiver:
func (p: inout Point) translate(dx: float32, dy: float32) {
    p.x += dx
    p.y += dy
}
```

### Module and Package Imports
Vertex supports both nominal modules and path-based source directory imports:

```swift
package app

import "std/fmt"            // Package root import (resolved via -P or VERTEXPATH)
import "./geometry"         // Relative filesystem directory import
import algebra "math/linear" // Aliased directory import
```

Imported directories are compiled on-demand as distinct modules and linked directly into the target binary.

### Hardware & Dataflow Modifiers
The `kernel` and `graph` modifiers annotate declarations intended for compute and dataflow dispatch:

```swift
func computeVector(_ x: float32) kernel -> float32 {
    return x * 2.0
}

func transformNode(_ x: float32) graph -> float32 {
    return x + 1.0
}
```

---

## Compiler Architecture

`vsc` translates source code through a series of strongly typed, verifiable representation layers:

```
                  ┌───────────────────────────────┐
                  │      Source (.vs / .swift)    │
                  └──────────────┬────────────────┘
                                 │
                   [scanner] & [token] Lexing
                                 ▼
                  ┌───────────────────────────────┐
                  │       Token Stream            │
                  └──────────────┬────────────────┘
                                 │
                     [parser] Parsing (ast)
                                 ▼
                  ┌───────────────────────────────┐
                  │    Abstract Syntax Tree       │
                  └──────────────┬────────────────┘
                                 │
                   [analyzer] Type Checking (types)
                                 ▼
                  ┌───────────────────────────────┐
                  │      Typechecked AST          │
                  └──────────────┬────────────────┘
                                 │
                 [internal/sil/gen] Lowering
                                 ▼
                  ┌───────────────────────────────┐
                  │         Raw SIL               │
                  └──────────────┬────────────────┘
                                 │
               [internal/sil/pass] DI & Boxing
             [internal/sil/verify] Invariant Check
                                 ▼
                  ┌───────────────────────────────┐
                  │       Canonical SIL           │
                  └──────────────┬────────────────┘
                                 │
                    [lower] Machine Lowering
                                 ▼
                  ┌───────────────────────────────┐
                  │    Vertex Machine IR (VIR)    │
                  └──────────────┬────────────────┘
                                 │
                 [build] Object & Native Linking
                                 ▼
                  ┌───────────────────────────────┐
                  │  Native Mach-O / PE Binary    │
                  └───────────────────────────────┘
```

### Package Directory

| Package | Purpose |
| --- | --- |
| `token` | Source positions, token kind constants, and diagnostic reporting structures |
| `scanner` | Lexical scanner, Unicode identifier decoding, and numeric/string literal parsing |
| `ast` | Syntax tree data structures and AST walk routines |
| `parser` | Recursive-descent parser with error recovery and attribute parsing |
| `types` | Type representations, substitutions, type layout, and unification predicates |
| `analyzer` | Scoping, type inference, member lookup, and operator precedence |
| `core` | Built-in standard module declarations, operators, and intrinsic symbols |
| `internal/sil` | SIL module structures, basic blocks, instructions, and ownership values |
| `internal/sil/gen` | AST-to-SIL generator emitting explicit retain/release and memory allocations |
| `internal/sil/pass` | Mandatory SIL transformations: Definite Initialization (DI) and boxing |
| `internal/sil/verify` | SSA dominance verification and ownership invariant validation |
| `lower` | SIL lowering to VIR: ABI register packing, calling conventions, and vtables |
| `mangle` | Swift-compatible symbol mangling for functions, types, and conformances |
| `iface` | Module interface parser, validator, and serializer (`.vinterface`) |
| `pkg` | `Package.swift` manifest parsing and target dependency graph resolution |
| `build` | Compilation driver, native object generation, sysroot discovery, and linking |
| `stdlib` | In-tree C++ runtime implementing ARC, String, Array, and Unicode tables |

---

## Supported Target Platforms

| Target | Architecture | OS | Executable Format | Platform System Link |
| --- | --- | --- | --- | --- |
| `aarch64-macos` | ARM64 | macOS | Mach-O | macOS SDK `libSystem.tbd` |
| `x86_64-windows` | x86-64 | Windows | PE/COFF | MSVC CRT (`libcmt`, `libucrt`) |

System libraries and SDK roots are located automatically:
- On macOS, `vsc` detects Command Line Tools or Xcode SDKs via `$SDKROOT` or `xcrun`.
- On Windows, `vsc` discovers MSVC toolsets and Windows SDKs via `%LIB%` or Visual Studio installation paths.
- Running with `-freestanding` produces zero-dependency standalone binaries without system libraries.

---

## Embedding the Go Compiler API

`vsc` can be used directly as a Go library:

```go
package main

import (
	"fmt"
	"os"

	"github.com/vertex-language/vsc"
)

func main() {
	sourceCode := []byte(`
		func add(_ a: int32, _ b: int32) -> int32 { return a + b }
		func main() -> int32 { return add(40, 2) }
	`)

	target, err := vsc.HostTarget()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unsupported host: %v\n", err)
		os.Exit(1)
	}

	unit, diags := vsc.Compile(
		[]vsc.Source{{Name: "main.vs", Text: sourceCode}},
		vsc.Options{
			Module: "main",
			Target: target,
			Stop:   vsc.All,
		},
	)

	if vsc.Errors(diags) {
		for _, d := range diags {
			fmt.Fprintf(os.Stderr, "%s\n", d.Format(nil))
		}
		os.Exit(1)
	}

	fmt.Printf("Compiled module %s (SIL functions: %d)\n",
		unit.Info.Module, len(unit.SIL.Functions))
}
```

Use `vsc.Options.Stop` to halt compilation at earlier phases (`Parsed`, `Checked`, `Raw`, `Canonical`, `Lowered`, `All`).

---

## Verification & Testing

The compiler's correctness is validated across five automated test suites:

- `tests/syntax`: Validates parsing grammar and recovery across syntax forms.
- `tests/check`: Validates typechecking, semantic constraints, and diagnostic error output.
- `tests/compiler`: Contains 194 complete programs compiled, linked, and run natively.
- `tests/interop`: Differential testing comparing binary output and behavior against `swiftc`.
- `tests/cinterop`: Tests C calling conventions and interoperability (`@_cdecl`, `@_silgen_name`).

### Running Tests

Run frontend, lowering, and CLI tests:

```bash
go test ./...
```

Run integration, cross-module, and execution tests:

```bash
cd build && go test ./...
```

---

## Toolchain Ecosystem

`vsc` is part of the Vertex compiler family:

- **`vsc`**: Vertex Source Compiler frontend, SIL, and lowering.
- **`vcx`**: In-process C++ compiler used to compile the Vertex runtime.
- **`vcc`**: Standalone C compiler sharing header scanning with `vcx`.
- **`ir`**: Target-independent machine intermediate representation (VIR).
- **`arm64`** / **`amd64`** / **`i386`**: Native architecture assemblers and machine instruction encoders.
- **`macho`** / **`pe`** / **`elf`**: Pure Go binary container readers, writers, and linkers.

---

## License

MIT License. Copyright (c) 2026 Netangular Technologies.
