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

A bridge translates platform APIs into an `extern "C"` ABI wrapped by Vertex bindings.

### File Layout

```
<pkg>/
├── c<pkg>/
│   ├── include/c<pkg>.h   # Public C ABI: extern "C", primitive types only
│   ├── c<pkg>.cpp         # Main implementation (#if defined(_WIN32), etc.)
│   ├── c<pkg>_darwin.m    # Optional: Objective-C for Apple frameworks
│   └── kernels.metal      # gpu/* only: vendor GPU kernels (elsewhere, kernels are .vs)
├── bindings.vs            # @_silgen_name definitions only
└── *.vs                   # Public Vertex API, types, policy, and validation
```

```swift
.target(name: "c<pkg>", path: "<pkg>/c<pkg>", publicHeadersPath: "include"),
.target(name: "<pkg>", dependencies: ["c<pkg>"], path: "<pkg>", exclude: ["c<pkg>"]),
```

A target compiles every source in it for **every** platform. Wrap a platform-only file (`c<pkg>_darwin.m`) in `#if defined(__APPLE__)`, and implement the same header functions in the `.cpp` under `#if !defined(__APPLE__)`.

### Toolchain & Language Selection

All native code compiles in-process using the Go toolchain (no external toolchain required):

| Extension | Compiler | Standard / Target | Usage |
| --- | --- | --- | --- |
| `.cpp`, `.cc`, `.cxx` | **vcx** / `v++` | the manifest's `cxxLanguageStandard` (up to C++23) | **Default.** Use for all POSIX and Win32 bridges. Enables RAII, clean string views, and templates. |
| `.c` | **vcc** | C11/C17 | Pristine upstream C code only. |
| `.m` | **objv** | Objective-C (ARC) | Apple-only frameworks (AppKit, Security, MPS/Accelerate in `gpu/blas`). Metal's host API belongs to the built-in `gpu`'s runtime unit (§1.1), not to packages. |
| `.cu`, `.cuh`, `.hip`, `.metal` | **vcx** | vendor GPU kernels | Only in the `gpu` repository's function packages (`gpu/blas`, `gpu/dnn`), for kernels that need vendor tuning. Everywhere else a GPU kernel is `.vs` (`func f(...) kernel`), which builds for every vendor and the CPU. Vendor sources load through `device.Library`. `.metal` builds a `.metallib` that's loaded at run time, not a linked object. |

**Known gaps:**
* **GPU sources in packages:** `vsc/pkg/layout.go` doesn't list `.cu`, `.cuh`, `.hip` or `.metal` yet, so a `package.vs` target won't pick them up.
* **Objective-C++ (`.mm`) and assembly (`.s`, `.S`):** not built. For Objective-C++, use a `.m` file and a `.cpp` file that talk through the shared header.
* **`std::span`:** vcx can't construct one from a pointer and a length yet. Pass a pointer and a count.

---

## 4. Bridge Design Contract

* **ABI Boundary:** Headers must use `extern "C"` and expose only fixed-width integers (`int32_t`, `uint64_t`), raw buffers (`const char*`, length pairs), and opaque pointers. No C++ types, Objective-C objects, or exceptions may cross the boundary.
* **Namespace Isolation:** Prefix all C symbols with `c<pkg>_*` and constants with `C<PKG>_*`. Never use `vertex_*`.
* **Zero Blocking:** Native bridges must never block an OS thread. Descriptors are non-blocking. Return `*_ERR_WOULD_BLOCK` on pending I/O and let Vertex await readiness via `vertex_task_wait_fd`. When the OS only offers a blocking call (a child's exit, a signal), turn it into a readable descriptor (a pidfd, kqueue, or self-pipe) instead of blocking in native code.
* **Caller-Owned Memory:** Buffers are allocated and owned by the caller. If the native layer must allocate, provide a corresponding `c<pkg>_free_*()` function.
* **Return Conventions:** Return `0` or byte counts on success; return negative error codes on failure. Provide `c<pkg>_last_error()` to surface underlying OS error numbers (`errno`, `GetLastError()`).
* **Universal Target Support:** Every header function must be implemented across all supported targets (`aarch64-macos`, `x86_64-windows`, `aarch64-android`). Unsupported operations must return `*_ERR_UNSUPPORTED` rather than being omitted or panicking.
* **Thin Adapters:** The bridge handles OS call translation only. Parsing, business logic, default values, and data structures belong in Vertex.
* **Privileged Packages Only:** Native bridges are restricted to core platform modules (`os`, `sync`, `fs`, `time`, `net/*`, `ui/window`, `db/sqlite`) and the `gpu` repository's function packages (`gpu/*`: vendor kernel sources, and `gpu/blas`'s MPS/Accelerate binding). `gpu` itself isn't a package: it's built into the compiler (§1.1).

---

## 5. Adding to the Runtime

If a feature meets the runtime criteria:

1. **Declare:** Add the `vertex_*` function to `include/vertex/abi.h` (public compiler ABI) or `include/vertex/platform.h` (internal PAL).
2. **Implement Everywhere:** Provide implementations in `platform/darwin.h`, `platform/windows.h`, and `platform/android.h`. Where a platform can't do it, degrade honestly: Windows' `vertex_pal_io_register` returns -1, and the executor falls back to a thread wait.
3. **Bind:** If generated by the compiler, register the symbol in `stdlib.go`.
4. **Document & Test:** Add the definition to `ABI.md` and add automated coverage under `stdlib/tests`.

Keep runtime additions small. Every one in the core unit ships in every Vertex binary and has to be maintained on every target.

For an on-demand unit (§1.1), the same steps apply, plus: put it in its own directory under `stdlib/runtime/` with its own `vertex_<feature>_*` prefix; have `vsc/build` add it to the link only when the program uses the feature, as it does `SwiftBridge`; and load any vendor or system library it needs at run time, so a binary still starts on a machine without it.
