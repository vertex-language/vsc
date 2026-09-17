# libswiftCore

An architectural overview of Swift's core runtime and standard library: internal composition, build mechanics, subsystem boundaries, and platform ABI contracts.

---

## 1. Architecture: Two Libraries Wearing One Name

`libswiftCore` is packaged as a single shared object across platforms:
* `libswiftCore.dylib` (macOS)
* `swiftCore.dll` (Windows)
* `libswiftCore.so` (Linux)

Beneath that single binary interface, it consists of two distinct subsystems compiled by two different toolchains:

| Subsystem | Source Language | Swift Repository Path | Primary Responsibilities |
| :--- | :--- | :--- | :--- |
| **The Runtime** | C++ | `stdlib/public/runtime/` | Object lifecycle (`swift_retain`, `swift_release`, `swift_allocObject`), type metadata tables, existential boxing (`Any`), dynamic casting, reflection records, error handling |
| **The Standard Library** | Swift | `stdlib/public/core/` | Fundamental types (`Int`, `String`, `Array`, `Optional`), protocols (`Sequence`, `Equatable`), collection algorithms, console I/O (`print`) |

### The Shim Layer
Connecting these halves is an internal abstraction boundary located in `stdlib/public/stubs/` and `stdlib/public/SwiftShims/`. These contain C and C++ wrappers that expose underlying platform C libraries (libc, `libSystem`, `ucrt`) to the Swift standard library using C headers that the compiler can import directly.

### Why the C++ Floor Exists
The C++ runtime is not legacy code waiting to be rewritten in Swift; it represents **operations beneath the language's own type system**:
* A reference count resides in an internal object header that has no nominal Swift type.
* Type metadata records describe what a Swift type *is*, meaning they cannot be self-described by pure Swift types without circular definitions.
* Boxing a value into an existential container (`Any`) involves copying arbitrary memory whose layout and alignment are determined dynamically at runtime.

---

## 2. Build Pipeline and Bootstrap Mechanism

`libswiftCore` is **not** built using Swift Package Manager (`Package.swift`). It is built via **CMake and Ninja**, orchestrated by the project's Python build runner (`utils/build-script`).

### Why `Package.swift` Cannot Build the Core Library
1. **Circular Bootstrap Dependency:** SwiftPM is an executable program written in high-level Swift. Parsing a `Package.swift` manifest and scheduling build tasks requires an already functioning Swift compiler, runtime, and standard library. SwiftPM cannot compile its own prerequisites.
2. **Compiler Intrinsics:** The Swift half of `libswiftCore` relies on compiler builtins (`Builtin.RawPointer`, `Builtin.Int64`, `Builtin.Word`) and compiler flags like `-parse-stdlib`. These builtins bypass standard name resolution and module imports, which standard SwiftPM builds do not support.

### The Actual Build Sequence


```

```
            ┌──────────────────────────────┐
            │   Host C++ Compiler (Clang)  │
            └──────────────┬───────────────┘
                           │
        Compiles CMake C++ targets into binaries
                           │
        ┌──────────────────┴──────────────────┐
        ▼                                     ▼

```

┌───────────────────────┐             ┌───────────────────────┐
│  Stage 1: `swiftc`    │             │  C++ Runtime Objects  │
│   Compiler Binary     │             │ (`stdlib/.../runtime`)│
└───────────┬───────────┘             └───────────┬───────────┘
│                                     │
Compiles Swift stdlib                         │
with `-parse-stdlib`                          │
│                                     │
▼                                     │
┌───────────────────────┐                         │
│ Standard Library Objs │                         │
│  (`stdlib/.../core`)  │                         │
└───────────┬───────────┘                         │
│                                     │
└──────────────────┬──────────────────┘
│
Platform Linker
│
▼
┌─────────────────────┐
│    libswiftCore     │
│ (.dylib / .so / .dll)│
└─────────────────────┘

```

1. **Stage 1 Compiler:** Clang compiles the Swift compiler itself (`swiftc`) from LLVM/C++ sources.
2. **C++ Runtime Compilation:** Clang or MSVC compiles the C++ runtime files in `stdlib/public/runtime/` and platform shims in `stdlib/public/stubs/` into native object files.
3. **Standard Library Compilation:** The newly built `swiftc` compiles the Swift source files in `stdlib/public/core/` into object files. The types in `core` wrap low-level `Builtin.*` types (e.g., `struct Int { var _value: Builtin.Int64 }`).
4. **Final Link:** The platform linker merges the C++ runtime objects and Swift standard library objects into the final dynamic library (`libswiftCore`).

---

## 3. Subsystem Boundaries: `libswiftCore` vs. `Foundation`

A frequent point of confusion in Swift development is what belongs to the standard library (`libswiftCore`) versus higher-level system libraries (`Foundation`, `System`, or OS frameworks).

### The Boundary Rule
`libswiftCore` contains only what is required to make the core language execute, manipulate memory, and manage basic console I/O. It has **no awareness of file systems, networks, clocks, or operating system service daemons**.

### Direct Comparison: I/O and System Operations

* **Does `print()` belong in `libswiftCore`?**  
  **Yes.** `print()` is implemented in `stdlib/public/core/Print.swift`. It formats arguments, writes output via internal TextOutputStreams, and terminates in C `fwrite` calls directed to `stdout`.
* **Does `writeFile` / File I/O belong in `libswiftCore`?**  
  **No.** `libswiftCore` has zero concept of a file system, directory paths, or file descriptors. File operations live in:
  * **`Foundation`**: `FileManager.default.createFile`, `Data.write(to:)`, `FileHandle`
  * **`System` (`swift-system`)**: POSIX system-call wrappers (`FileDescriptor.open`, `write`)
  * **Platform C Libs**: Explicitly importing Darwin/Glibc/UCRT to call POSIX `open`/`write` directly.

### Detailed Scope Division

| Subsystem / Feature | Library Location | Rationale |
| :--- | :--- | :--- |
| **Scalar Primitives** (`Int`, `Double`, `Bool`) | `libswiftCore` | Core language types mapping to machine registers. |
| **Standard Collections** (`Array`, `Dictionary`, `Set`) | `libswiftCore` | Language-level collection types. |
| **Pointers & Memory** (`UnsafePointer`, `MemoryLayout`) | `libswiftCore` | Direct memory manipulation and layout inspection. |
| **Basic Terminal I/O** (`print`, `readLine`) | `libswiftCore` | Essential CLI interaction routed to `stdout`/`stdin`. |
| **Assertions & Traps** (`assert`, `precondition`, `fatalError`) | `libswiftCore` | Safety invariants and runtime abort mechanisms. |
| **File System & Paths** (`FileHandle`, `URL`, `FileManager`) | **`Foundation`** | Requires platform-specific I/O, file descriptor abstractions, and event loops. |
| **Networking & Sockets** (`URLSession`, HTTP, TCP) | **`Foundation` / `Network`** | Involves asynchronous sockets, state machines, and security protocols (TLS). |
| **Calendar & Dates** (`Date`, `Calendar`, `TimeZone`) | **`Foundation`** | Depends on timezone databases and calendar computation logic. |
| **Serialization Engines** (`JSONEncoder`, `PropertyListEncoder`) | **`Foundation`** | While `libswiftCore` defines the `Codable` protocol interfaces, concrete serialization engines live outside Core. |
| **Structured Concurrency** (`Task`, `Actor`) | **`libswift_Concurrency`** | Thread-pool scheduling and asynchronous continuations are separated into their own dedicated runtime library. |

---

## 4. The OS Interface: The C ABI Floor

To perform primitive actions like writing to the console or allocating heap space, `libswiftCore` must reach the operating system kernel. Operating systems do not publish uniform system-call contracts:

| Platform | Kernel Interface Stability | Contract Mechanism |
| :--- | :--- | :--- |
| **macOS** | **Unstable.** Raw syscall numbers deliberately change between OS releases. | Must route through `libSystem.dylib`. Apple enforces dynamic linking against this library. |
| **Windows** | **Unstable.** System-call IDs vary across build revisions. | Must route through documented user-mode subsystems (`kernel32.dll`, `libucrt.lib`, `ntdll.dll`). |
| **Linux** | **Stable.** Syscall numbers constitute a permanent kernel contract. | While direct syscalls are technically possible, standard toolchains route through `libc.so.6`. |

Because the lowest stable public interface on all major desktop platforms is exposed as a C-ABI dynamic library, `libswiftCore` connects to the host OS strictly via C-compatible symbols (`malloc`, `free`, `fwrite`, `write`).

---

## 5. The Anatomy of `print("hi")`

Although `print` is conceptually a standard console write, Swift's general-purpose signature introduces significant runtime machinery:

```swift
public func print(_ items: Any..., separator: String = " ", terminator: String = "\n")

```

Because `items` accepts `Any...`, passing even a simple literal traverses dynamic existential and collection layers:

```
print("hi")
  │
  ├─ $ss5print_9separator10terminatoryypd_S2StF        [Swift: stdlib/public/core/Print.swift]
  │    Variadic arguments are boxed into an Array<Any>
  │      ├─ $ss27_allocateUninitializedArray…           [Swift: stdlib/public/core/Array.swift]
  │      │    └─ swift_allocObject                      [C++ Runtime: HeapObject allocation]
  │      └─ Each argument is boxed into an existential container (Any)
  │           └─ $sSSMa / $sypN                         [Data: Metadata records for String and Any]
  │
  ├─ _print(_:separator:terminator:to:)                 [Swift: Text formatting pass]
  │    Resolves element description via reflection or CustomStringConvertible dispatch
  │
  ├─ _Stdout.write(_:)                                  [Swift: TextOutputStream implementation]
  │    └─ _swift_stdlib_fwrite                          [C++ Stubs: stdlib/public/stubs/Stubs.cpp]
  │         └─ fwrite / fputs                           [Platform C-ABI: libSystem / ucrt / libc]
  │              └─ write(2) / WriteFile                [Kernel System Call]

```

### The Cost of `Any`

The operations between `print("hi")` and `fwrite` exist primarily to resolve type erasure:

1. **Dynamic Boxing:** Values are copied into existential containers with inline buffers or indirect heap boxes.
2. **Metadata Lookup:** Every argument carries a pointer to its runtime metadata record (`$s...Ma`) so the runtime can dynamically locate string representation conformances.
3. **Array Allocation:** Passing variadic parameters dynamically constructs a temporary heap-allocated `Array<Any>`.

---

## 6. Key ABI Symbols and Runtime Entry Points

Compilers that lower Swift code to machine instructions emit direct references to runtime hooks provided by `libswiftCore`. Key entry points include:

| Symbol Pattern | Domain | Description |
| --- | --- | --- |
| `swift_retain` | Memory / ARC | Increments the strong reference count in an object's `HeapObject` header. |
| `swift_release` | Memory / ARC | Decrements reference count; invokes deinitializer and `swift_slowAlloc` free when 0. |
| `swift_allocObject` | Memory / Heap | Allocates heap memory with a standard reference-counted `HeapObject` prefix. |
| `swift_bridgeObjectRetain` | Collections / String | Retains an object that may be a native heap pointer, a tagged pointer, or immortal storage. |
| `swift_bridgeObjectRelease` | Collections / String | Releases bridge-object storage based on bitmask flags. |
| `$ss27_allocateUninitializedArray...` | Arrays | Core allocator for array literals; returns a tuple `(Array<T>, RawPointer)`. |
| `$s...Ma` | Metadata | Type metadata accessors (e.g., `$sSiMa` for `Int`, `$sSSMa` for `String`). |
| `$sypN` | Metadata | Direct metadata descriptor address for the `Any` type. |

---

## 7. Platform Distribution & ABI Stability

The operational characteristics of `libswiftCore` vary significantly across host environments:

| Characteristic | macOS | Windows | Linux |
| --- | --- | --- | --- |
| **Distribution** | Bundled in OS dyld shared cache (`/usr/lib/swift/`) | Bundled with swift.org toolchain or redistributed alongside binary | Bundled with swift.org toolchain or redistributed alongside binary |
| **ABI Stability** | **Stable.** Frozen as of Swift 5.0 in the OS ABI. | **Unfrozen.** Binaries must target specific runtime toolchain revisions. | **Unfrozen.** Binaries must target specific runtime toolchain revisions. |
| **Linkage** | Dynamic against OS stub (`libswiftCore.tbd`). | Dynamic (`swiftCore.dll`) or static (`-static-stdlib`). | Dynamic (`libswiftCore.so`) or static (`-static-stdlib`). |
| **Redistribution Needs** | None (OS managed). | Required (DLLs must ship with application). | Required (shared objects must ship with application or container). |