# Runtime vs. Package Bridges

`stdlib/runtime`'s core unit is unconditionally linked into every Vertex binary across all targets. It contains only what compiled code and core types strictly require to exist. A language feature that not every program uses gets a **runtime unit of its own**, linked only into the programs that use it (§1.1). Everything else—especially OS-facing functionality—belongs in a **package native bridge** linked only when imported.

---

## 1. The Membership Rule

Code belongs in the runtime if and only if:

1. **The compiler emits direct calls to it** (ARC, allocations, errors, generics metadata, task scheduling, `async main`).
2. **The core module cannot link or typecheck without it** (`String`, `Array`, `Dictionary`, `print`, float parsing/formatting, hashing seeds).
3. **Only the task executor can perform it** (parking tasks on descriptors, waking workers, executor-local storage).

| Question | Runtime | Package |
| --- | --- | --- |
| Does `vsc/lower` or `sil/gen` emit calls to it? | **Yes** | — |
| Would basic language types fail to link without it? | **Yes** | — |
| Does it require direct task/executor/worker state? | **Yes** | — |
| Is it OS interaction (files, sockets, env, processes)? | — | **Yes** |

### 1.1 Runtime units linked on demand

Some of what the compiler emits calls to belongs to a feature most programs never use. It still belongs to the compiler, by rule 1, but it doesn't belong in every binary. It gets its own unit beside the core one, and the build adds that unit to the link only when the program uses the feature.

* **The precedent:** `SwiftBridge`, linked only into programs that use Swift interop.
* **The first real one:** the built-in `gpu` module (`proposed_vertex_kernel.md` §3). Its device half is compiler intrinsics that lower straight to VIR and need no runtime at all. Its host half, which the compiler-written `Launch`, `Enqueue` and `Map` call, is `stdlib/runtime/gpu/`: the one binding to the Metal, CUDA-driver and HIP APIs, linked only into programs that import `gpu`. Vendor drivers are opened at run time, so a binary runs on a machine that lacks one.
* **Not a way around §3.** An on-demand unit is for code the compiler itself calls. A library that merely needs the OS is still a package with a bridge.

---

## 2. The Platform Abstraction Layer (`vertex_pal_*`) is Private

`include/vertex/platform.h` exists exclusively for the runtime. Packages must never bind `vertex_pal_*` functions via `@_silgen_name`.

* **Allowed package-to-runtime entry points:** The public executor ABI (`vertex_task_wait_fd`, `vertex_task_sleep`, `vertex_task_yield`, `vertex_task_workers`).
* **Everything else:** If a package needs OS functionality, it implements its own native bridge. If it requires executor features, add an explicit `vertex_task_*` function to `ABI.md`.

---

## 3. Native Bridge Architecture

A package's native code is a **C++ named module in the package's own folder**. vsc reads the module's `export`s through vcx and gives them to the package's Vertex as ordinary declarations. There is no manifest, no C header, no `bindings.vs` and no `@_silgen_name`. The design is `~/Desktop/proposed_vsc_import_v2.md`.

### File Layout

```
net/tcp/                  ← import "net/tcp"
├── sock.cpp              # export module net.tcp;   the interface unit (exactly one)
├── sock_posix.cpp        # module net.tcp;          implementation units, any number
├── sock_windows.cpp      # module net.tcp;
├── window_darwin.mm      # module ui.window;        Objective-C++ for Apple frameworks, with ARC
└── *.vs                  # package tcp: the public Vertex API, types, policy, validation
```

* **The module name is the import path with dots:** `net/tcp` is `net.tcp`, `github.com/you/thing/x` is `thing.x`. vsc checks it.
* **Platform files use Go's suffixes:** `_darwin`, `_posix` (every platform but Windows), `_windows`, `_linux`, `_android`, `_ios`, and `_arm64` / `_amd64`, alone or as `_linux_amd64`. A file named for another target is not built. This replaces `#if` around whole files.
* **System headers go in the global module fragment** (`module;` … `export module net.tcp;`), so their macros never reach the module's users.
* **Visibility:** in a folder that has `.vs` files, the C++ exports are package-internal: the `.vs` files call them unqualified, and importers see only the `public` Vertex API. A folder of C++ alone is a package whose exports *are* its API (`import "math"`, `math.add(1, 2)`).
* **Programs** are `cmd/<name>/main.vs` (`vsc run <name>`), tests included.
* **Linking:** a unit names what a program using the package must link with a pragma, in its global module fragment: `#pragma vertex framework("AppKit")`, `#pragma vertex library("m")`, or clang's `#pragma comment(lib, "ws2_32")`. Framework headers are found as clang finds them (`<CoreFoundation/CoreFoundation.h>`).

### What crosses

vsc generates one `extern "C"` thunk per exported function and calls that, so every C++ ABI decision (how a `std::string_view` is passed, what a symbol is called) stays vcx's.

| C++ export | Vertex sees |
| --- | --- |
| `int32_t`, `uint16_t`, `double`, `bool`, `char` | `int32`, `uint16`, `float64`, `bool`, `CChar` |
| `T*`, `const T*`, `void*`, `T**` | `UnsafeMutablePointer<T>?`, `UnsafePointer<T>?`, `UnsafeMutableRawPointer?`, … |
| `std::string_view` parameter | `string` |
| `std::span<T>` / `std::span<const T>` parameter | `UnsafeMutableBufferPointer<T>` / `UnsafeBufferPointer<T>` (waits on vcx's span, see the gaps) |
| `enum class E : int32_t { … }` | `enum E: int32 { case … }` |
| unscoped `enum E : int32_t { … }` | `enum E { static let …: int32 }` |
| `constexpr` integer | `let` (`static let` in a namespace) |
| `namespace <module> { … }` (`math` for `math`, `net::tcp` or `tcp` for `net.tcp`) | the package itself |
| any other namespace | a Vertex namespace: `detail::f` is `detail.f` |

Not yet: classes, templates, `std::expected` / `std::optional`, references. vsc says what it left out and why when it builds.

### Toolchain & Language Selection

All native code compiles in-process through vcx (no external toolchain):

| Extension | Compiler | Usage |
| --- | --- | --- |
| `.cpp`, `.cc`, `.cxx`, `.cppm` | **vcx** / `v++` (C++23) | **Default.** Every bridge. |
| `.mm` | **vcx** (Objective-C++, ARC) | Apple-only frameworks (`ui/window/window_darwin.mm`). |
| `.cu`, `.cuh`, `.hip`, `.metal` | **vcx** | Only in the `gpu` repository's function packages, for kernels that need vendor tuning. Everywhere else a kernel is `.vs`. |

`.c` and `.m` are errors in any package, `Package.swift` ones included: vsc builds `.vs` and C++, and nothing else. C belongs to vcc and Objective-C to objv, which are separate compilers.

**Known gaps:**
* **`std::span`:** vcx can't construct one from a pointer and a length yet, nor iterate one with range-`for`. Pass a pointer and a count.
* **Constants:** only integer `constexpr`s import; a `constexpr double` is left out. Export a function returning it.

---

## 4. Bridge Design Contract

* **Types at the boundary:** fixed-width integers, pointers and buffers, `std::string_view`, enums and integer constants. No classes, exceptions or Objective-C objects cross. Exported functions should be `noexcept`: the thunks are, so an escaping exception terminates.
* **Namespace Isolation:** the module is the namespace. Never name anything `vertex_*`.
* **Zero Blocking:** Native bridges must never block an OS thread. Descriptors are non-blocking. Return a would-block code on pending I/O and let Vertex await readiness via `vertex_task_wait_fd`. When the OS only offers a blocking call (a child's exit, a signal), turn it into a readable descriptor (a pidfd, kqueue, or self-pipe) instead of blocking in native code.
* **Caller-Owned Memory:** Buffers are allocated and owned by the caller. If the native layer must allocate, export a matching free function.
* **Return Conventions:** Return `0` or byte counts on success, and negative error codes on failure. Export a `last_error()` to surface underlying OS error numbers (`errno`, `GetLastError()`).
* **Universal Target Support:** Every exported function must be implemented on every supported target (`aarch64-macos`, `x86_64-windows`, `aarch64-android`). An unsupported operation returns an "unsupported" code rather than being omitted or panicking.
* **Thin Adapters:** The bridge handles OS call translation only. Parsing, business logic, default values, and data structures belong in Vertex.
* **Privileged Packages Only:** Native bridges are restricted to core platform modules (`os`, `sync`, `fs`, `time`, `net/*`, `ui/window`, `db/sqlite`) and the `gpu` repository's function packages (`gpu/*`). `gpu` itself isn't a package: it's built into the compiler (§1.1).

---

## 5. Adding to the Runtime

If a feature meets the runtime criteria:

1. **Declare:** Add the `vertex_*` function to `include/vertex/abi.h` (public compiler ABI) or `include/vertex/platform.h` (internal PAL).
2. **Implement Everywhere:** Provide implementations in `platform/darwin.h`, `platform/windows.h`, and `platform/android.h`. Where a platform can't do it, degrade honestly: Windows' `vertex_pal_io_register` returns -1, and the executor falls back to a thread wait.
3. **Bind:** If generated by the compiler, register the symbol in `stdlib.go`.
4. **Document & Test:** Add the definition to `ABI.md` and add automated coverage under `stdlib/tests`.

Keep runtime additions small. Every one in the core unit ships in every Vertex binary and has to be maintained on every target.

For an on-demand unit (§1.1), the same steps apply, plus: put it in its own directory under `stdlib/runtime/` with its own `vertex_<feature>_*` prefix; have `vsc/build` add it to the link only when the program uses the feature, as it does `SwiftBridge`; and load any vendor or system library it needs at run time, so a binary still starts on a machine without it.
