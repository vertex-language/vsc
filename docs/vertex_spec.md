# Vertex Language Specification

Vertex is a statically typed systems programming language designed for performance, predictability, and modern ergonomics. Vertex is an independent language built on a shared core dialect, providing a dedicated **Swift compatibility layer** for zero-cost interoperability with Swift codebases, libraries, and ABIs.

---

## 1. Language Architecture & Dialect Model

Vertex separates language identity from low-level runtime interoperability:

- **Independent Language**: Vertex defines its own grammar productions, package model, receiver methods, compute execution modifiers, and relaxed call-site syntax.
- **Shared Core Dialect**: Vertex shares a foundational dialect with Swift—including the intermediate representation (SIL), Automatic Reference Counting (ARC) memory model, type layouts, calling conventions, and symbol mangling.
- **Swift Compatibility Layer**: The compiler (`vsc`) compiles Swift source files (`.swift`) and parses module interfaces (`.swiftinterface`) natively, mapping Swift declarations directly into the core dialect.
- **Vertex Source (`.vs`)**: Native Vertex files use the `.vs` extension and enable all language extensions specified in this document.

---

## 2. Types

### 2.1 Primitive Spellings

Vertex provides lowercase primitive type aliases mapped directly to the shared core dialect:

```text
VertexTypeName: (one of)
    bool char string void never any
    int int8 int16 int32 int64
    uint uint8 uint16 uint32 uint64
    float float32 double float64
```

| Vertex Type | Core / Swift Mapping | Description |
| --- | --- | --- |
| `bool` | `Bool` | Boolean type (`true`, `false`) |
| `int`, `int8`, `int16`, `int32`, `int64` | `Int`, `Int8` … `Int64` | Signed integers (pointer-width, 8, 16, 32, 64-bit) |
| `uint`, `uint8`, `uint16`, `uint32`, `uint64` | `UInt`, `UInt8` … `UInt64` | Unsigned integers (pointer-width, 8, 16, 32, 64-bit) |
| `float`, `float32` | `Float` | 32-bit IEEE 754 floating point |
| `double`, `float64` | `Double` | 64-bit IEEE 754 floating point |
| `string` | `String` | UTF-8 encoded string |
| `char` | `Character` | Extended grapheme cluster |
| `void` | `Void` | Unit type / empty tuple |
| `never` | `Never` | Divergent return type |
| `any` | `Any` | Type-erased container |

---

## 3. Declarations

### 3.1 Package Declarations

A package declaration assigns a module identity directly within source code:

```text
PackageDeclaration:
    package Identifier
```

- **Identity**: Sets the module identifier used for symbol mangling and linker resolution.
- **Resolution**: When omitted, the module name defaults to the enclosing folder name (§5.1) or the `-module` compiler flag.
- **Entry Point**: `func main()` in package `main` is the executable entry point.
- **Disambiguation**: `package` followed by an `Identifier` is a package declaration. `package` followed by a declaration keyword (e.g. `package func`) is parsed as an access modifier.

### 3.2 Directory & Path Imports

Vertex supports importing source directories via string literals:

```text
ImportDeclaration:
    [AttributeList] [AccessLevelModifier] import [ImportKind] ImportPath
    [AttributeList] [AccessLevelModifier] import ImportSpec
    [AttributeList] [AccessLevelModifier] import '(' {ImportSpec} ')'

ImportSpec:
    [Identifier] StringLiteral
```

- **Filesystem Paths**: String literals import directories containing `.vs` files:
  ```swift
  import "std/fmt"            // Package root import (resolved via -P or VERTEXPATH)
  import "./geometry"         // Relative directory import
  import geom "./geometry"    // Aliased directory import
  ```
- **Grouped Imports**: Imports can be grouped using parentheses:
  ```swift
  import (
      "std/fmt"
      "std/math"
      "./local_pkg"
  )
  ```
- **Disambiguation**: Relative paths (`./`, `../`) resolve against the importing file's directory. Bare paths resolve against package search roots (`-P`, `VERTEXPATH`). Prebuilt interfaces (`.vinterface`, `.swiftinterface`) resolve via `-I`.

### 3.3 Receiver Methods

Methods may be declared outside type declarations using receiver syntax:

```text
FunctionDeclaration:
    [AttributeList] [DeclarationModifiers] func [ReceiverClause] FunctionName
        [GenericParameterClause] FunctionSignature [GenericWhereClause]
        [FunctionBody]

ReceiverClause:
    '(' Identifier ':' [OwnershipModifier] Type ')'

OwnershipModifier: (one of)
    borrowing consuming inout __shared __owned
```

```swift
func (v: borrowing Vec2) length() -> float32 {
    return (v.x * v.x + v.y * v.y).squareRoot()
}

func (v: inout Vec2) scale(by factor: float32) {
    v.x *= factor
    v.y *= factor
}
```

- **Semantics**: Lowers to an extension method. Statically dispatched; cannot be overridden in class hierarchies.
- **Receiver Identifier**: In scope within the function body and denotes `self`. Implicit member access remains available.
- **Ownership Modifiers**:
  | Receiver Kind | `borrowing` (default) | `inout` | `consuming` |
  | --- | --- | --- | --- |
  | `struct`, `enum` | Standard method | `mutating func` | `consuming func` |
  | `class` | Reference method | *Disallowed* | `consuming func` |

### 3.4 Execution Modifiers (`kernel`, `graph`)

Annotate declarations intended for compute acceleration and dataflow execution:

```text
FunctionSignature:
    '(' [ParameterList] ')' [async] [ThrowsClause] [ExecutionModifier] [FunctionResult]

ExecutionModifier: (one of)
    kernel graph
```

```swift
func computeShader(data: [float32]) kernel -> [float32] { ... }
func pipelineNode(input: Stream) graph -> Stream { ... }
```

- **Placement**: Stood between the `throws` clause and the return arrow.
- **`kernel`**: compiled for the device, and launched with the `Launch` or `Map` method the compiler writes for it. The rules and the `gpu` API are in the README's Kernels section and `proposed_vertex_kernel.md`.
- **`graph`**: reserved. It parses and type-checks, and lowering refuses it.

---

## 4. Call Sites & Argument Labels

Argument labels are optional at call sites when invocation remains unambiguous:

```swift
func add(a: int32, b: int32) -> int32 { ... }

add(a: 1, b: 2)   // Explicit labels
add(1, 2)         // Omitted labels
```

- **Resolution**:
  1. Strict match against declared parameter labels.
  2. Fallback to positional resolution if exact label matching fails.
  3. Compile error if positional resolution produces multiple candidate overloads.
- **Mandatory Labels**: Required when disambiguating overloads, passing non-default arguments in short argument lists, or terminating variadic parameter lists.

---

## 5. Module & Package Model

### 5.1 Identity Resolution

A module's canonical identifier is determined in order:
1. `package <name>` declaration in source files.
2. Directory name of the imported path.
3. Compiler `-module <name>` flag (defaults to `main`).

### 5.2 Directory Semantics

- Every `*.vs` file within a directory constitutes that module.
- Subdirectories are not included automatically.
- Imported directories are compiled on-demand and linked directly into the binary.

---

## 6. Implementation Status

| Feature | Specification | Status |
| --- | --- | --- |
| Lowercase Primitives | §2.1 | Complete |
| Package Declarations | §3.1 | Complete |
| Directory & Grouped Imports | §3.2 | Complete |
| Receiver Methods | §3.3 | Complete |
| `kernel` | §3.4 | Complete (Metal and the CPU device) |
| `graph` | §3.4 | Reserved (parsed and checked; refused at lowering) |
| Relaxed Argument Labels | §4 | Complete |
| Folder Modules & Resolution | §5 | Complete |
