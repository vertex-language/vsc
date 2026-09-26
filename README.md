# vsc

[![compiler: vsc](https://img.shields.io/badge/compiler-vsc-f4f4f5?style=flat-square&labelColor=e4e4e7&color=18181b)](https://github.com/vertex-language/vsc)
[![targets: macOS | Android | Windows](https://img.shields.io/badge/targets-macOS%20%7C%20Android%20%7C%20Windows-f4f4f5?style=flat-square&labelColor=e4e4e7&color=18181b)](https://github.com/vertex-language/vsc)
[![kernels: Metal | CPU](https://img.shields.io/badge/kernels-Metal%20%7C%20CPU-f4f4f5?style=flat-square&labelColor=e4e4e7&color=18181b)](https://github.com/vertex-language/vsc)
[![native: C++ | Objective-C++](https://img.shields.io/badge/native-C%2B%2B%20%7C%20Objective--C%2B%2B-f4f4f5?style=flat-square&labelColor=e4e4e7&color=18181b)](https://github.com/vertex-language/vcx)

The Vertex compiler. `vsc` takes `.vs` source, and the C++ a package carries
with it, to a native executable in one process. There is no system compiler,
assembler or linker involved: scanning, checking, ownership, lowering, object
emission and linking are all vsc's, and a package's C++ is compiled by
[vcx](https://github.com/vertex-language/vcx), the in-process C++ compiler.

GPU kernels are part of the language. A `kernel` function is checked for the
device, compiled for it, and launched with a typed call the compiler writes,
from the same file as the host code that uses it.

---

## Quick Start

Build the compiler (Go 1.23 or later):

```bash
cd cmd && go build -o ~/go/bin/vsc ./vsc
```

Run a file:

```swift
// hello.vs
let name = "Vertex"
print("hello from \(name)")
```

```bash
vsc run hello.vs
```

Or build a binary:

```bash
vsc build -o hello hello.vs
./hello
```

Top-level statements are the program. `func main()` works too, as does
`async` code at the top level.

---

## The Language

A short tour of what compiles today. Every snippet in this section builds and
runs with the current vsc.

### Types

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

### Receiver methods

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

### Labels are optional where the call is clear

```swift
func add(a: int32, b: int32) -> int32 { a + b }

add(a: 1, b: 2)
add(1, 2)                          // the same call
```

A label is required only when leaving it out would make the call ambiguous.

### Enums and matching

```swift
enum Shape {
    case circle(radius: float64)
    case rect(w: float64, h: float64)
}

func area(_ s: Shape) -> float64 {
    switch s {
    case .circle(let r): return 3.14159 * r * r
    case .rect(let w, let h): return w * h
    }
}
```

### Concurrency

`async`/`await`, `async let`, task groups and actors run on vsc's own task
executor, which is linked into every program:

```swift
func work(_ n: int) async -> int { return n * n }

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

### Imports and conditional compilation

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
[Packages](#packages)).

---

## Kernels

`kernel` goes where `async` goes: `func f(...) kernel`. A kernel that returns
nothing is a **grid kernel**, called once per work-item. A kernel that returns
a value is an **element kernel**, called once per element and applied with
`Map`. `import "gpu"` gives both the device API and the host API. `gpu` is
built into the compiler, so there's nothing to install.

```swift
import "gpu"

// A grid kernel: one call per work-item.
func saxpy(_ a: float32, _ x: gpu.Span<float32>, _ y: gpu.MutableSpan<float32>) kernel {
    let i = gpu.Index.x
    if i < y.count {
        y[i] = a * x[i] + y[i]
    }
}

// An element kernel: one output element per call.
func gelu(_ x: float32) kernel -> float32 {
    let t = 0.7978845608 * (x + 0.044715 * x * x * x)
    return 0.5 * x * (1 + tanh(t))
}

// An ordinary function: compiled for the device because a kernel calls it.
func tanh(_ t: float32) -> float32 {
    let t2 = t * t
    return t * (27 + t2) / (27 + 9 * t2)
}

// A group reduction: shared storage and barriers, then one wave in registers.
func sum(_ x: gpu.Span<float32>, _ out: gpu.MutableSpan<float32>) kernel {
    let partial = gpu.Shared<float32>(count: 256)
    let me = gpu.LocalIndex.x
    let i = gpu.Index.x
    partial[me] = i < x.count ? x[i] : 0
    gpu.Barrier()
    var stride = gpu.GroupSize.x / 2
    while stride >= gpu.Wave.Size {
        if me < stride {
            partial[me] += partial[me + stride]
        }
        gpu.Barrier()
        stride /= 2
    }
    if me < gpu.Wave.Size {
        let s = gpu.Wave.Sum(partial[me])
        if gpu.Wave.Lane == 0 {
            _ = gpu.Atomic.Add(out.Address(0), s)
        }
    }
}

// A generic kernel: built for each element type it is launched with.
func scale<T: Numeric>(_ y: gpu.MutableSpan<T>, _ k: T) kernel {
    let i = gpu.Index.x
    if i < y.count {
        y[i] = y[i] * k
    }
}

let device = gpu.Default()                                   // Metal here, or the CPU
let n = 1 << 16
let x = try await device.Upload([float32](repeating: 1, count: n))
let y = try await device.Upload([float32](repeating: 2, count: n))

try await saxpy.Launch(3.0, x, y, over: n)                   // the grid is the work
let acts = try await gelu.Map(y)                             // a buffer in, a new buffer out

let total = try await device.Upload([float32(0)])
try await sum.Launch(y, total, over: n, workgroup: 256)
print(try await total.Download()[0])                         // 327680.0

let ints = try await device.Upload([int32(1), 2, 3])
try await scale.Launch(ints, 10, over: 3)                    // [10, 20, 30]

// The CPU device is always there: the oracle a GPU result is checked against.
let input: [float32] = [-2, -0.5, 0, 0.5, 2]
let onGPU = try await gelu.Map(try await device.Upload(input)).Download()
let onCPU = try await gelu.Map(try await gpu.CPU().Upload(input)).Download()
print(onGPU == onCPU)                                        // true
```

### What the compiler does for you

- **Typed launches.** Every kernel gets a `Launch` method with the kernel's own
  parameters, and every element kernel gets `Map`. A `gpu.Span<T>` parameter
  takes a `gpu.Buffer<T>`. A wrong argument is a compile error, not a crash on
  the device.
- **No device annotations.** Anything a kernel calls is compiled for the device
  and checked there: plain functions, generic functions, other packages'
  `@inlinable` functions, and element kernels called directly.
- **The rules are checked, with the chain.** A kernel can't reach the heap,
  `print`, `throws`, `async`, or a global. The error says how it got there:
  `kernel 'noisy' cannot run on a device: noisy calls log`. Calling a kernel
  like a function is refused too: `'fill' is a kernel: launch it with
  fill.Launch`.
- **Memory spaces are inferred.** Spans are device memory, scalars are
  constants, `gpu.Shared` is workgroup memory, and locals are private. You
  never write an address space.
- **Bounds are checked.** `span[i]` traps outside the span, and the
  `if i < y.count` guard lets the compiler drop the check beneath it.
  `span.Unchecked[i]` removes it visibly at one access.

### The device API

| API | Is |
| :--- | :--- |
| `gpu.Index`, `LocalIndex`, `GroupIndex` | where this work-item is: in the grid, in its group, and which group (`.x`, `.y`, `.z`) |
| `gpu.GroupSize`, `GroupCount`, `GridSize` | work-items per group, groups in the grid, work-items in the grid |
| `gpu.Span<T>`, `gpu.MutableSpan<T>` | views of device memory: `count`, a subscript, `Slice`, `Unchecked`, `Address` |
| `gpu.Shared<T>(count:)` | one allocation per workgroup, written where it's used |
| `gpu.Barrier()` | every work-item in the group arrives, and memory is visible after it |
| `gpu.Wave` | `Lane`, `Size`, `Shuffle…`, `Any`, `All`, `Ballot`, `First`, `Sum`, `Min`, `Max` |
| `gpu.Atomic` | `Add`, `Sub`, `Min`, `Max`, `And`, `Or`, `Xor`, `Exchange`, `CompareExchange`, on integers and floats |

On the host: `gpu.Default()`, `gpu.CPU()`, `gpu.Devices()`,
`device.Upload`, `device.CreateBuffer`, `device.Wrap` (existing host memory,
no copy), and on a buffer `Download`, `Copy`, `Fill`, `Slice` and `View(as:)`.

### Devices

| Device | State |
| :--- | :--- |
| **Metal** (Apple GPUs) | Runs. Kernels are lowered to AIR and embedded as Metal libraries. |
| **CPU** | Runs everywhere. Groups run on a worker per core, and a group that uses barriers runs as fibers on arm64, so a barrier is a stack switch. A `float64` kernel, which Apple GPUs can't run, falls back to it. |
| **NVIDIA**, **AMD** | Kernel lowering to PTX and AMDGPU exists in `ir`. The driver runtime doesn't load them yet. |

`vsc/tests/kernel` is the kernel test suite: 80 programs, each run on the CPU
device and on Metal and compared.

---

## Packages

A package is a folder. Every `.vs` file in it is the package, and a `.cpp`
file in it is the package's native half. Nothing lists the files, and there
is no manifest to keep in step with the folder.

```
geo/                         ← repository: module github.com/you/geo
├── vs.mod
├── shape/                   ← import "github.com/you/geo/shape"
│   ├── shape.vs             ←   package shape
│   ├── shape.cpp            ←   export module geo.shape;   (its native half)
│   └── shape_windows.cpp    ←   only built for Windows
├── fastmath/                ← import "github.com/you/geo/fastmath"
│   └── fastmath.cpp         ←   a package of C++ alone
├── testdata/                ← files the programs read; never built
└── cmd/
    └── area/main.vs         ← vsc run area
```

### Creating a package

1. **Make the folder and a `vs.mod`.** The module path is where the repository
   lives, and `platform` is the oldest OS it runs on:

   ```
   module github.com/you/geo

   vertex 0.9
   platform macos 14
   ```

2. **Write the package.** One `package` name per folder, the folder's own
   name. `public` is the API; everything else stays inside the package:

   ```swift
   // shape/shape.vs
   package shape

   /// A circle, by its radius.
   public struct Circle {
       public var Radius: float64

       public init(radius: float64) {
           Radius = radius
       }
   }

   public func (c: borrowing Circle) Area() -> float64 {
       return 3.141592653589793 * c.Radius * c.Radius
   }
   ```

3. **Add programs in `cmd/<name>/main.vs`,** tests included, and run them by
   name from anywhere in the repository:

   ```swift
   // cmd/area/main.vs
   package main

   import "github.com/you/geo/shape"

   print(shape.Circle(radius: 2).Area())
   ```

   ```bash
   vsc run area
   ```

   A bare name is always `cmd/<name>`, even when a package folder has the same
   name. `./shape` names the folder.

4. **Commit `vs.sum`** once a dependency has been fetched. vsc records each
   dependency's hash there and checks it on every later fetch.

### Best practices

- **Name exported API in `UpperCamelCase`:** `fs.Open`, `png.Decode`,
  `window.Create`. Keep internals `lowerCamelCase`. A reader can tell from the
  call site what is part of the package's contract.
- **Share across the repository with `package`,** not `public`: a helper
  `db/sqlite` needs from `db/sql` is `package func`, so it isn't part of
  either package's API. Anything unmarked is internal, and another package
  that names it gets "inaccessible due to 'internal' protection level".
- **Keep one idea per folder.** Subfolders are separate packages
  (`rdp/codec/planar`), so a large package splits into small ones that import
  each other, and each is testable alone.
- **Put tests and tools in `cmd/`.** A check program prints `ok` lines and
  exits non-zero on failure (`vsc run check`). A program that talks to the
  network or a device gets its own name (`hub-live-test`), so the offline
  checks stay fast.
- **Put sample input in `testdata/`.** vsc never builds it.
- **Use file suffixes for platform code, not `#if` around whole files:**
  `_darwin`, `_linux`, `_windows`, `_android`, `_ios`, `_posix` (everything
  but Windows), `_arm64`, `_amd64`, or both (`_linux_amd64`). This works for
  `.vs` and `.cpp` alike.
- **Reach for C++ last.** Parsing, validation, defaults and data structures
  belong in Vertex. Native code is for the OS call that Vertex can't make.

### Standard packages

A short path imports a package from `github.com/vertex-language`, so
`import "net/http"` is the `http` folder of the `net` repository. The standard
packages are `archive`, `cli`, `compress`, `crypto`, `db`, `encoding`, `fs`,
`hash`, `image`, `io`, `log`, `math`, `model`, `net`, `nn`, `os`, `remote`,
`tensor`, `text`, `time` and `ui`, plus the function packages of the `gpu`
repository. Any other package is imported by its full path
(`"github.com/you/geo/shape"`).

The first build that needs a package fetches it into the package cache
(`VERTEXCACHE`, or `vertex` in this machine's cache directory). Cloning is
built in, so there's no need to have git installed. Later builds read the
cache.

| Flag | Does |
| :--- | :--- |
| `-replace path=dir` | build a package from a local checkout |
| `-offline` | never fetch; use the cache as it is |
| `-update` | fetch again, even if the cache has the package |

### Working on several repositories

A `vs.work` above the checkouts makes every build use them in place of the
cache:

```
vertex 0.9

use (
    ./fs
    ./io
    ./net
    ./geo
)
```

---

## Native C++ in a Package

When a package needs the OS (a syscall, a framework, a driver), its folder
holds a C++ **named module**. vsc reads the module's `export`s through vcx and
gives them to the package's Vertex as ordinary declarations. There's no C
header, no binding file and no symbol attribute to write.

```cpp
// shape/shape.cpp
module;
#include <stdint.h>                      // system headers: the global module fragment
export module geo.shape;                 // the import path, with dots

// Package-internal: shape.vs calls it by name.
export double sumOfSquares(const double* xs, int64_t n) noexcept {
    double s = 0;
    for (int64_t i = 0; i < n; i++) s += xs[i] * xs[i];
    return s;
}
```

```swift
// shape/shape.vs
public func TotalArea(_ circles: [Circle]) -> float64 {
    let radii = circles.map { $0.Radius }
    return radii.withUnsafeBufferPointer { sumOfSquares($0.baseAddress, int64($0.count)) } * 3.141592653589793
}
```

A folder of C++ alone is a package too, and its exports *are* its API:

```cpp
// fastmath/fastmath.cpp
module;
#include <math.h>
#pragma vertex library("m")              // what a program using this links with
export module geo.fastmath;

export namespace fastmath {              // the module's own namespace is unwrapped
    double Hypot(double x, double y) noexcept { return hypot(x, y); }
}
```

```swift
import "github.com/you/geo/fastmath"

print(fastmath.Hypot(3, 4))              // 5.0
```

### The rules

- **The module name is the import path with dots**, without the host and owner:
  `github.com/you/geo/shape` is `geo.shape`, and `net/tcp` is `net.tcp`. vsc
  checks it.
- **One interface unit per folder** (`export module geo.shape;`). Any number of
  implementation units (`module geo.shape;`) can go beside it, usually one per
  platform: `shape_posix.cpp`, `shape_windows.cpp`.
- **System headers go in the global module fragment,** between `module;` and
  `export module`. Their macros stay there and never reach Vertex.
- **Links are declared in the source:** `#pragma vertex framework("AppKit")`,
  `#pragma vertex library("m")`, or `#pragma comment(lib, "ws2_32")`.
- **One package's C++ can import another's:** `import net.tcp;` resolves to
  that package's folder and links it. `import vertex.task;` gives the task
  executor's public entry points.
- **Apple frameworks use Objective-C++.** A `_darwin.mm` implementation unit is
  compiled by vcx with ARC, like any other unit of the module. `ui/window`
  drives Cocoa this way.
- **C++ only.** `.c` and `.m` files are errors. Rename them `.cpp` and `.mm`
  (C code usually needs only casts on `malloc`'s result).

### What crosses

| C++ export | Vertex sees |
| :--- | :--- |
| `int32_t`, `uint16_t`, `int64_t`, `double`, `bool` | `int32`, `uint16`, `int64`, `float64`, `bool` |
| `size_t` | `uint` |
| `T*`, `const T*`, `void*` | `UnsafeMutablePointer<T>?`, `UnsafePointer<T>?`, `UnsafeMutableRawPointer?` |
| `std::string_view` parameter | `string` |
| `enum class E : int32_t { … }` | `enum E: int32` with its cases |
| `export namespace Code { constexpr int32_t ok = 0, … }` | `enum Code { static let ok: int32 }` |
| `constexpr` integer | `let` |
| `namespace <module>` | the package itself |
| any other namespace | a nested namespace: `detail::f` is `detail.f` |

Classes, templates, references, `std::optional`, `std::expected` and non-integer
constants don't cross yet. vsc lists what it left out, and why, when it
builds.

### Best practices for native code

- **Keep it thin.** Translate one OS call and return. Everything else is
  Vertex.
- **Mark exports `noexcept`.** An exception that escapes to Vertex terminates
  the program.
- **Never block a thread.** Use non-blocking descriptors, return a
  "would block" code, and let the Vertex side wait with the task executor.
  When the OS only offers a blocking call, turn it into a readable descriptor.
- **Let the caller own memory.** Take buffers as pointer and length. If the
  native side must allocate, export the matching free.
- **Return `0` or a count on success and a negative code on failure.** Export
  the codes as a constant namespace, and a `last_error()` for `errno` or
  `GetLastError()`.
- **Implement every export on every target.** Where a platform can't do
  something, return an "unsupported" code rather than leaving the function out.
- **Don't export unscoped enums.** Their enumerators (`read`, `write`) collide
  with POSIX functions in the same module. Use `enum class` or a constant
  namespace.

---

## Command Line

| Command | Does |
| :--- | :--- |
| `vsc run [name \| files]` | build to a temporary path and run it; a name is `cmd/<name>` |
| `vsc build [name \| files]` | compile and link; with no arguments, the folder here |
| `vsc check [files]` | parse and type-check; print diagnostics |
| `vsc ast [file]` | print the syntax tree |
| `vsc tokens [file]` | print the token stream |
| `vsc env` | print the target, the SDK, the package cache and the known targets |

A file of `-` (or no file) reads standard input. Arguments after `--` go to
the program: `vsc run hub -- resolve hf.co/Qwen/Qwen3-0.6B`.

| Flag | Does |
| :--- | :--- |
| `-target T` | `aarch64-macos`, `aarch64-android` or `x86_64-windows` (default: this machine) |
| `-o file` | where to write the output (`-` is standard output, for `vir` and `sil`) |
| `-module name` | the module being compiled (default `main`, whose `main` is the entry point) |
| `-P dir` | a package root for string imports (repeatable) |
| `-I dir` | a directory of `.vinterface` files for named imports (repeatable) |
| `-replace path=dir`, `-offline`, `-update` | see [Standard packages](#standard-packages) |
| `-freestanding` | link no platform libraries |
| `-entry sym` | the program's entry symbol |

`--emit` stops at an earlier stage:

| `--emit` | Writes |
| :--- | :--- |
| `exe` | a linked program (the default) |
| `lib` | a shared library (`aarch64-android`) |
| `obj` | an object file |
| `interface` | the module's public API as a `.vinterface`, with `@inlinable` bodies kept |
| `sil` | the ownership IR after the mandatory passes |
| `vir` | the machine IR |

Exit codes: `0` success, `1` compile errors, `2` usage or I/O errors.

---

## Targets

| Target | Output | Notes |
| :--- | :--- | :--- |
| `aarch64-macos` | Mach-O | Finds the SDK through `SDKROOT` or `xcrun`. Kernels run on Metal and the CPU. |
| `aarch64-android` | ELF shared library | `--emit lib`; `ui/window` supplies the NativeActivity entry. |
| `x86_64-windows` | PE/COFF | Links against the MSVC and UCRT libraries, found through `LIB` or a Visual Studio install. `async` code doesn't build for this target yet. |

---

## How It Works

```
 .vs source              .cpp / .mm in the package
     │                              │
 scanner, parser                    vcx: parse, check
     │                              │
 analyzer ◄──── exports ────────────┤  (a thunk per export)
     │                              │
 SIL: generate, definite initialization, ownership
     │                              │
 lower ──► VIR ──┬─► arm64 / amd64 ─┼─► object files ──► macho / pe / elf linker ──► program
                 │                  │
                 └─► kernels: AIR (Metal), host code (CPU device); PTX, AMDGPU
```

| Package | Does |
| :--- | :--- |
| `token`, `scanner`, `parser`, `ast` | source positions, tokens, the recursive-descent parser and its tree |
| `ifconfig` | `#if` resolved for the target being built |
| `types`, `analyzer`, `core` | types, name lookup, inference, overloads, the built-in module |
| `internal/sil/…` | SIL generation, definite initialization, ownership passes, the verifier |
| `lower` | SIL to VIR: layouts, calling conventions, witness tables, the kernel lowering |
| `mangle`, `iface` | symbol names, and `.vinterface` reading and writing |
| `pkg`, `importer` | package folders, file suffixes, `vs.mod` / `vs.sum` / `vs.work`, fetching and the cache |
| `build` | the native half (vcx), objects, and links |
| `stdlib` | the runtime: ARC, strings, collections, generics metadata, the task executor, and the `gpu` device runtime, which is linked only into programs that use it |
| `cmd/vsc` | the command line |

vsc also compiles `.swift` files and reads `.swiftinterface` modules, and it
builds `Package.swift` packages, so existing code in that language links into
a Vertex program.

---

## As a Go Library

```go
package main

import (
	"fmt"
	"os"

	"github.com/vertex-language/vsc"
)

func main() {
	target, ok := vsc.HostTarget()
	if !ok {
		fmt.Fprintln(os.Stderr, "vsc has no target for this machine")
		os.Exit(1)
	}
	src := []byte(`
func add(_ a: int32, _ b: int32) -> int32 { a + b }
print(add(40, 2))
`)
	unit, diags := vsc.Compile([]vsc.Source{{Name: "main.vs", Text: src}},
		vsc.Options{Module: "main", Target: target, Stop: vsc.Checked})
	for _, d := range diags {
		fmt.Fprintln(os.Stderr, d)
	}
	if vsc.Errors(diags) {
		os.Exit(1)
	}
	fmt.Println(unit.Info != nil)
}
```

`Options.Stop` ends compilation early: `Parsed`, `Checked`, `Raw`,
`Canonical`, `Lowered`, or `All`.

---

## Testing

| Suite | Checks |
| :--- | :--- |
| `tests/` | the language ladder: 281 small programs, from an empty one up, each built and run and its output compared with the reference compiler's |
| `tests/kernel` | 80 kernel programs, run on the CPU device and on Metal, plus the errors the kernel checker must give |
| `build/testdata/packages` | package builds: mixed `.vs` and C++ folders, Objective-C++ frameworks |
| `build/testdata/interop`, `cinterop` | linking with other compilers' objects, and C calling conventions |
| `parser/testdata`, `analyzer/testdata` | syntax, recovery, and type-checking diagnostics |

```bash
go test ./...                               # front end, lowering, CLI
cd build && go test ./...                   # the ladders, packages, interop
```

---

## The Toolchain

| Repository | Is |
| :--- | :--- |
| **vsc** | this compiler |
| **[vcx](https://github.com/vertex-language/vcx)** (`v++`) | the C++ and Objective-C++ compiler: the runtime, and every package's native module |
| **ir** | VIR, the machine IR, and its lowerings: arm64, amd64, AIR, PTX, AMDGPU |
| **arm64**, **amd64**, **i386** | instruction encoders |
| **macho**, **pe**, **elf** | object writers and linkers |

---

## License

[MIT](LICENSE)
