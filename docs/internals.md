# Internals

## Targets

| Target | Output | Notes |
| :--- | :--- | :--- |
| `aarch64-macos` | Mach-O | Finds the SDK through `SDKROOT` or `xcrun`. Kernels run on Metal and the CPU. |
| `aarch64-android` | ELF shared library | `--emit lib`; `ui/window` supplies the NativeActivity entry. |
| `x86_64-linux` | ELF | |
| `x86_64-windows` | PE/COFF | Links against the MSVC and UCRT libraries, found through `LIB` or a Visual Studio install. `async` code doesn't build for this target yet. |

# How It Works

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

# As a Go Library

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
func add(a: int32, b: int32) -> int32 { a + b }
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

# Testing

| Suite | Checks |
| :--- | :--- |
| `tests/` | the language ladder: over 450 small programs (`001`–`454`), climbing from an empty one up, each built and run and its output compared with the reference compiler's |
| `tests/kernel` | 80 kernel programs, run on the CPU device and on Metal, plus the errors the kernel checker must give |
| `build/testdata/packages` | package builds: mixed `.vs` and C++ folders, Objective-C++ frameworks |
| `build/testdata/interop`, `cinterop` | linking with other compilers' objects, and C calling conventions |
| `parser/testdata`, `analyzer/testdata` | syntax, recovery, and type-checking diagnostics |

```bash
go test ./...                               # front end, lowering, CLI
cd build && go test ./...                   # the ladders, packages, interop
```
