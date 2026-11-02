# Native C++ in a package

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
public func TotalArea(circles: [Circle]) -> float64 {
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

## The rules

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

## What crosses

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

## Best practices for native code

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
