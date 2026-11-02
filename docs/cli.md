# Command line

| Command | Does |
| :--- | :--- |
| `vsc run [name \| files]` | build to a temporary path and run it; a name is `cmd/<name>` |
| `vsc build [name \| files]` | compile and link; with no arguments, the folder here |
| `vsc install [name \| folder \| path[@ref]]` | build programs into `$VERTEXBIN` (default `~/.vertex/bin`); with no arguments, every program in this checkout's `cmd/` |
| `vsc doc [-o file.html] [-all] [package]` | write a package's exported signatures and comments as Markdown, or HTML for `.html` / `-format html` |
| `vsc check [files]` | parse and type-check; print diagnostics |
| `vsc ast [file]` | print the syntax tree |
| `vsc tokens [file]` | print the token stream |
| `vsc env` | print the target, the SDK, the package cache and the known targets |

A file of `-` (or no file) reads standard input. Arguments after `--` go to
the program: `vsc run hub -- resolve hf.co/Qwen/Qwen3-0.6B`.

`vsc install` is `go install`: a name is `cmd/<name>`, a folder that is a
library (or has no `.vs` files) stands for every program in its `cmd/`, and
an import path is fetched first: `vsc install time/cmd/stopwatch`,
`vsc install github.com/you/tool/cmd/tool@v1.2`. Each program is named for its
folder; one built with `-target` for another machine lands in
`$VERTEXBIN/<target>/`.

`vsc doc` is `go doc` for a whole package: its overview (the `// Package x`
comment above the package clause), an index, and every `public` or `open`
declaration as written, body left out, with the `///` comment above it.
Receiver functions and extensions are listed under their type. The package
is a folder or an import path, found as a build finds it: `vsc doc`,
`vsc doc -o tcp.html net/tcp`.

| Flag | Does |
| :--- | :--- |
| `-target T` | `aarch64-macos`, `aarch64-android`, `x86_64-linux` or `x86_64-windows` (default: this machine) |
| `-o file` | where to write the output (`-` is standard output, for `vir` and `sil`) |
| `-module name` | the module being compiled (default `main`, whose `main` is the entry point) |
| `-P dir` | a package root for string imports (repeatable) |
| `-I dir` | a directory of `.vinterface` files for named imports (repeatable) |
| `-replace path=dir`, `-offline`, `-update` | see [Standard packages](packages.md#standard-packages) |
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
