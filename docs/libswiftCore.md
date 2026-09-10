# libswiftCore

What it is, why `print` drags it in, and what Vertex would have to
write to stop needing it.

Written 2026-09-10, after `x86_64-windows` landed in vsc and `print`
was the one thing that would not build for it.

---

## 1. It is two libraries wearing one name

`libswiftCore` is a single shared object — `libswiftCore.dylib` on
macOS, `swiftCore.dll` on Windows, `libswiftCore.so` on Linux — and
inside it are two things built by two different compilers.

| Half | Language | Swift repo | What it holds |
| --- | --- | --- | --- |
| **The runtime** | C++ | `stdlib/public/runtime/` | `swift_retain`, `swift_release`, `swift_allocObject`, the type-metadata system, dynamic casting, existential boxing, error objects |
| **The standard library** | Swift | `stdlib/public/core/` | `Int`, `String`, `Array`, `Optional`, `print`, the protocol conformances |

Between them sits a third, smaller layer: `stdlib/public/stubs/` and
`stdlib/public/SwiftShims/` — C and C++ shims whose only job is to
reach the platform's libc, plus C headers that describe libc's shapes
to Swift in terms Swift can import.

The split is not arbitrary. The C++ half is **everything Swift cannot
say about itself**. A reference count lives in an object header that
has no Swift type; a metadata record describes what a Swift type *is*,
so it cannot be described by one; boxing an `Any` means writing bytes
whose layout is only known at runtime. Those are operations beneath the
language, so they are written beneath the language.

Everything else — and it is most of the library by volume — is
ordinary Swift, compiled by `swiftc`.

---

## 2. The bootstrap: what you guessed, and the actual shape

You guessed that Swift had to bootstrap and that the first rung was a
C/C++ library that worked across platforms. That is right in
substance, with one correction to the story.

There was never a C standard library for Swift that was later replaced
by a Swift one. The shape has been the same since the beginning:

1. **`swiftc` itself is C++**, built on LLVM, and still is today. It
   is compiled by a C++ compiler like any other C++ program. This is
   the rung everything else stands on, and it never went away.
2. **The C++ runtime half of libswiftCore is compiled by clang**, as
   plain C++. It depends on nothing Swift.
3. **The Swift half of the standard library is compiled by the
   `swiftc` just built in step 1.** This is the genuine bootstrap: the
   library that defines `Int` is compiled by a compiler that already
   has to know what `Int` is. Swift resolves that by having the
   compiler know a small set of `Builtin.*` primitives intrinsically —
   `Builtin.Int64`, `Builtin.RawPointer` — and `stdlib/public/core`
   wraps those in the types everyone actually uses. `Int` is a struct
   with one field of type `Builtin.Int64`.
4. Both halves link into one shared object.

vsc does the same thing in the same place, which is worth noticing:
`core/core.swift` declares the operators on the primitives with no
bodies, and `core/core.go` says which machine instruction each body is.
Swift writes that as a `@_transparent` wrapper the first mandatory pass
inlines away, leaving a builtin. Same arrangement, different spelling.

So the honest version of your hypothesis: **the C++ is not a
bootstrap that was outgrown — it is the permanent floor.** It is there
for the operations that are beneath the type system, not for the ones
that came first chronologically.

---

## 3. The kernel argument: right conclusion, different mechanism

You said the OS kernel holds all the keys — console, I/O, `print` —
so the bootstrap had to be C. The conclusion is right. The mechanism
is not quite "because kernel", it is **because the OS publishes its
ABI as a C-ABI dynamic library, and nothing else.**

The distinction matters because it is not uniform across platforms:

| Platform | Is the raw syscall ABI stable? | What is the actual contract |
| --- | --- | --- |
| **Linux** | **Yes.** Syscall numbers are a kernel promise. | You may bypass libc entirely. Go does exactly this. |
| **macOS** | **No.** Numbers change between releases, deliberately. | `libSystem.dylib`. Apple has broken raw-syscall binaries on purpose. |
| **Windows** | **No.** Numbers change between *builds*. | `ntdll.dll` at the lowest documented layer, and in practice `kernel32` / `ucrt`. |

So on two of the three platforms Vertex targets, there is no legal way
to reach the kernel except through a C-ABI dynamic library the vendor
ships. That is not a limitation of Swift or of Vertex. It is the
interface the operating system offers, and it is offered in C because
C is the lingua franca of ABIs — not because C is privileged, but
because the C ABI is the one every language already knows how to
speak.

This is precisely why vsc's `-freestanding` flag and its hosted mode
are different things, and why the hosted link still needs
`libSystem.tbd` on macOS and `libucrt` on Windows even though vsc
emits every instruction itself. **The compiler needs no toolchain. The
program still needs the platform.**

---

## 4. What `print("hi")` actually does

Following one call all the way down, on macOS:

```
print("hi")
  │
  ├─ $ss5print_9separator10terminatoryypd_S2StF        [Swift, stdlib/public/core/Print.swift]
  │    the variadic list is boxed into an Array<Any>
  │      ├─ $ss27_allocateUninitializedArray…           [Swift] storage for the array
  │      │    └─ swift_allocObject                      [C++] heap object with a refcount header
  │      └─ each element boxed as Any, carrying metadata
  │           └─ $sSiMa, $ss5Int32VMa, $sypN            [C++/data] the metadata records
  │
  ├─ _print(_:separator:terminator:to:)                 [Swift]
  │    each item's description via reflection or protocol dispatch
  │
  ├─ _Stdout.write(_:)                                  [Swift] a TextOutputStream
  │    └─ _swift_stdlib_* shim                          [C++, stdlib/public/stubs/]
  │         └─ fwrite / putc                            [C]  libSystem
  │              └─ write(2)                            [syscall]
  └─
```

The whole apparatus above `fwrite` exists to answer one question:
*what is in this `Any`?* Swift's `print` takes `Any...`, so every
argument is type-erased at the call and must carry a metadata pointer
to be un-erased inside. That single signature decision is what pulls
in the array allocator, the boxing machinery, the metadata records and
the C++ runtime that owns them.

**Roughly four lines of that chain do the work. The rest is the cost
of `Any`.**

---

## 5. What vsc reaches into libswiftCore for today

Read out of the code rather than remembered. These are the exact
symbols a Vertex program can end up naming:

| Symbol | What it is | Where vsc names it |
| --- | --- | --- |
| `$ss5print_9separator10terminatoryypd_S2StF` | `print(_:separator:terminator:)` | `core/core.swift:371` |
| `$ss27_allocateUninitializedArrayySayxG_BptBwlF` | array-literal storage, returns `(Array<T>, RawPointer)` | `vil/gen/array.go:48` |
| `$sSiMa`, `$ss5Int32VMa`, `$sSdMa`, `$sSSMa`, … | type-metadata accessors, one per primitive | `vil/gen/array.go:213` |
| `$sypN` | the metadata record for `Any` | `vil/gen/array.go:203` |
| `swift_release` | ARC release for a Swift-made object | `lower/inst.go:1467` |
| `swift_bridgeObjectRetain` / `…Release` | ARC for `String` and `Array`, whose storage is a `_BridgeStorage` that may be native, tagged or immortal | `lower/string.go:175` |
| `Array.subscript.getter` | element read — and the bounds check traps *inside* libswiftCore | `vil/gen/array.go:~235` |

Note the last three rows. Even a program that never says `print` is
tied to libswiftCore the moment it holds a `String` or an `Array`,
because vsc deliberately does *not* refcount those with
`vertex_release` — what they hold is a bridge object, and only Swift's
own release knows whether the word is a pointer, a tagged value or
something immortal. Decrementing it would corrupt memory.

`_finalizeUninitializedArray` is the interesting exception: vsc does
not call it, because libswiftCore does not export it —
`@_alwaysEmitIntoClient` means `swiftc` emits a private copy into
every module that writes an array literal, and the body is a load and
a store of one word that only a checked runtime reads.

---

## 6. Why Windows is blocked twice

`print` is refused for `x86_64-windows`, and it would be refused twice
over:

**Blocker one — the symbol does not exist.** On macOS, libswiftCore is
part of the operating system: it lives in `/usr/lib/swift`, it is in
the dyld shared cache, and its stub `libswiftCore.tbd` sits in the SDK
where vsc's linker already picks it up. Nothing is redistributed and
nothing is versioned. On Windows there is no such thing. `swiftCore.dll`
exists only if someone installed a swift.org toolchain, and it must be
shipped beside every binary that uses it.

**Blocker two — lowering refuses it first.**
`_allocateUninitializedArray` returns `(Array<T>, Builtin.RawPointer)`
— two words. The Microsoft x64 calling convention returns **one**
value in `RAX`; anything larger comes back through caller-allocated
memory whose address is passed as a hidden first argument. That is
`sret`, and `ir/lower/amd64` has not written it for the MS convention
yet. So the program dies in instruction selection, before the linker
ever gets a chance to complain about the missing symbol:

```
vsc: build: lower: main: call @$ss27_allocateUninitializedArray…:
  the Microsoft calling convention returns one value; a second comes
  back in memory, which is sret and is not written yet
```

**These are not independent.** Blocker two exists *because of* the
`Any` boxing in §4. If `print` did not build an array of `Any`, there
would be no two-word tuple return, and the MS-ABI sret gap would not
be on the path at all.

There is a third consideration that argues against ever fixing this by
binding to Swift's Windows runtime: **Swift is ABI-stable on Apple
platforms only.** Swift 5.0 froze the ABI there and moved the runtime
into the OS. Everywhere else it is explicitly not frozen — a Vertex
binary linked against one `swiftCore.dll` would not be guaranteed to
run against the next. Vertex would be inheriting a version dependency
it has no way to control, on a platform where it gets nothing back for
it.

---

## 7. What a Vertex standard library would need instead

This is the payoff, and it is smaller than it looks.

**For `print` of a concrete type**, the entire requirement is:

1. `fwrite` or `write` — already linked. `libucrt` on Windows,
   `libSystem` on macOS. vsc's hosted link brings both in today
   without being asked, because `vertex_alloc` already calls `malloc`.
2. Integer-to-decimal and float-to-decimal formatting — a few dozen
   instructions, or a call to `snprintf`, which is in the same libc
   already on the link line.

Nothing else. No metadata, no boxing, no bridge objects, no C++
runtime, no ARC that is not already `vertex_retain` / `vertex_release`.

**The design decision that buys all of this** is the signature. Swift's

```swift
func print(_ items: Any..., separator: String = " ", terminator: String = "\n")
```

is what forces the whole apparatus. An overload set —

```swift
func print(_ x: Int32)
func print(_ x: Int64)
func print(_ x: Double)
func print(_ x: String)
```

— or a `Printable`-style protocol resolved statically, needs none of
it. Each call knows its own type at compile time, so there is nothing
to erase and nothing to un-erase.

Vertex already has the precedent for how to do this. The
`vertex_alloc` / `vertex_retain` / `vertex_release` runtime is not a C
library anyone has to compile ahead of time — `runtime/runtime.go`
emits it **as a VIR module**, which goes through the same instruction
selection and the same object writer as the program that calls it. A
target that can compile a program can build its runtime by
construction. A Vertex `print` belongs in exactly that place.

**The order the pieces would come in:**

1. `print` for the concrete primitives, written as VIR or as Vertex
   source, bottoming out in `fwrite`. Removes the single most common
   reason a program needs libswiftCore. Unblocks Windows immediately —
   and note that it does *not* require fixing the MS-ABI sret gap,
   because the two-word return is only on the `Any` path.
2. A Vertex `String` that is not `_BridgeStorage`. Removes
   `swift_bridgeObjectRetain` / `…Release`, and with them the last
   reason a program that never says `print` still touches Swift's
   runtime.
3. A Vertex `Array` with its own storage and its own bounds check.
   Removes `_allocateUninitializedArray`, the metadata accessors, and
   the array subscript getter.
4. Only then, if Swift interop is still wanted, keep the existing
   libswiftCore path as an *opt-in* for `aarch64-macos` — where it is
   free, because it is part of the OS — rather than as the only way to
   print a number.

After step 3, libswiftCore is a compatibility feature rather than a
dependency, which is the difference between a compiler that can target
Windows and one that targets Windows for programs that do not print.

---

## 8. Availability, at a glance

| | macOS | Windows | Linux |
| --- | --- | --- | --- |
| Ships with the OS | **Yes** — `/usr/lib/swift`, in the dyld shared cache | No | No |
| Stub available to link against | `libswiftCore.tbd` in the SDK | Only with a swift.org toolchain | Only with a swift.org toolchain |
| ABI stable | **Yes**, since Swift 5.0 | No | No |
| Must be redistributed with your binary | No | Yes | Yes |
| Static linking possible | Not really — it is in the shared cache | `-static-stdlib` | `-static-stdlib`, Static Linux SDK |
| What vsc does today | links the stub whether or not it is used | refuses the program in lowering | no target yet |

The row that decides the architecture is the first one. On macOS,
leaning on libswiftCore costs nothing — a load command, and the code
is already resident. Everywhere else it costs a redistributable,
version-locked, several-megabyte dependency on a toolchain that is not
Vertex's.

That asymmetry is the whole argument for writing the Vertex library.
