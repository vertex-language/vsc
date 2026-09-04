# What comes after the front end

The six packages in this repository are done in the sense that matters:
they are held to Swift by oracles rather than by opinion, and their
shape will not change. `token`, `scanner`, `ast`, `parser`, `types`,
`analyzer`. This document is about everything after them, and it
replaces the sketch in the README, which was drawn before any of it
was built.

The question it exists to answer: **is one `lower` package enough?**

No. And the reason is worth stating first, because every other
decision here follows from it.

---

## Why one lowering is not enough

VIR is a good IR. It is also, deliberately, a low-level one: its
types are `i1`, `i32`, `i64`, `f32`, `f64`, pointers and aggregates,
and its own documentation says a module is "everything a frontend
decided and nothing a backend will decide". In VIR a class instance is
a pointer. An existential is bytes. A generic function does not exist,
because a generic function has no machine code.

That is exactly right for VIR and exactly wrong for the middle of a
Swift compiler, because the things this language must decide in the
middle all need the information VIR has already thrown away:

- **ARC.** Where a retain and a release belong is a question about
  values with lifetimes. In VIR there are no values with lifetimes,
  only pointers being copied.
- **`~Copyable` and `consuming`.** Checking that an owned value is
  consumed exactly once needs an IR that knows which values are owned.
  A pointer is not owned; it is a number.
- **Definite initialization.** `self.n` must be assigned before `self`
  escapes an initializer. After lowering there is no `self`, only an
  allocation.
- **Generics.** Specializing `Stack<Int>` from `Stack<Element>` needs
  the generic function still to be there.
- **Exclusive access.** Two overlapping `inout` accesses are a fact
  about variables, not about addresses.

This is not a guess about our compiler. It is what Swift's own
compiler does and why: `swiftc` does not lower a type-checked AST to
LLVM IR. It lowers it to **SIL**, runs the ownership checks and ARC
and specialization there, and only then goes to LLVM. The whole middle
of `swiftc` exists because of the ownership model, and we have adopted
the ownership model.

`vcc` gets away with one flat `lower/` package because C has no
ownership model, no generics and no protocols — there is nothing to
decide between the AST and the IR. `mocha` gets away with one because
the JVM decides lifetime itself. We do not get away with it. That is
the price of the dialect, and it is a price worth paying, because the
same choice is what gives us `swiftc` as an oracle for the middle end
as well as the front.

---

## The shape

```
  ast + types + analyzer          the checked program        (done)
            │
            │  sil/gen
            ▼
      sil  (raw, ownership form)   classes are classes,
            │                      values are owned or borrowed,
            │  sil/pass            generics are still generic
            ▼
      sil  (canonical)             checked, ARC inserted,
            │                      specialized, host/device split
            │  lower
            ▼
      vir                          typed SSA, machine-level     (exists)
            │
            │  ir/lower  (in the ir repository)
            ▼
      isel → regalloc → object → link                          (exists)
```

Two lowerings, one IR between them. Everything below VIR is already
built and shared with `vcc`; nothing here proposes touching it.

---

## The packages

### `core/` — the built-in module

`Int`, `Bool`, `Double`, `String`, `Array`, `Optional`, their
operators and their conformances, as declarations the analyzer reads
the way it reads any other. Today `Int` has no members and `+` is not
declared anywhere, which is why any program touching a `String` stops
at the checker.

Its own package because it is *input*, not code: the analyzer imports
it, `sil/gen` needs to name its entry points, and the runtime needs to
implement some of them. It is also the one package a user might one
day replace.

**Build this first.** It is what widens the subset everything else is
tested against.

### `sil/` — the ownership IR

The instruction set, values, blocks, functions, and the ownership kind
on every value: *owned*, *guaranteed*, *unowned*, *none*. Mirrors what
`ir` is for VIR, and should mirror its shape:

| | |
|---|---|
| `sil/` | the IR: values, instructions, blocks, functions, modules |
| `sil/text` | the text form — `--emit sil`, and a parser for it, so tests can be written in it |
| `sil/verify` | the verifier, including the ownership rules |
| `sil/gen` | checked AST → raw SIL |
| `sil/pass` | the mandatory passes |
| `sil/opt` | the optional ones (later) |

The single most valuable thing to copy from Swift is that **the
verifier enforces ownership**: every owned value is consumed exactly
once on every path, every borrow is enclosed by the lifetime it
borrows from. Get that right and ARC insertion becomes mechanical
rather than clever, `~Copyable` becomes a verifier rule rather than a
special case, and a whole class of miscompilation stops being
expressible.

### `sil/gen` — AST to SIL

Where decisions are made rather than checked: which accessor a
property reference calls, where a temporary lives, how a closure
captures, what a `defer` becomes, how an `enum` with payloads is laid
out as a switch.

Separate from `analyzer` because the two have opposite jobs. The
analyzer answers questions about the program as written and must not
change it; `sil/gen` rewrites it into something simpler and must not
reject it. Mixing them gives you a phase that both diagnoses and
mutates, which is the thing that makes a compiler hard to reason about.

### `sil/pass` — the mandatory passes

The ones that can **reject a program**:

- definite initialization
- ownership verification (the OSSA rules)
- `~Copyable` / `consuming` checking
- exclusive access to memory
- escaping-closure capture diagnostics
- unreachable-code and missing-return diagnostics

Then the ones that must run for the program to be lowerable at all:

- ARC insertion — retain, release, and destroy placed on the ownership
  the verifier already proved
- closure lifetime fixup and capture lowering
- generic specialization
- the host/device split for `kernel` and `graph`

Its own package, separate from `sil/opt`, for one reason: **a
mandatory pass may change whether a program compiles; an optimization
may never.** Keep them in one pipeline and `-O0` and `-O` stop
agreeing about which programs are valid, which is a bug class you
cannot test your way out of. Keeping them apart also gives `vsc check`
an exact meaning — front end plus mandatory passes, nothing else.

### `lower/` — SIL to VIR

Here classes become pointers, existentials become boxes, enums become
tags and payloads, and the layout `types` computed becomes offsets.
By the time this runs, every ownership question has been answered and
every generic has been specialized, so this package is a translation
rather than a decision — which is what makes it testable.

One package is enough *here*, which is the README's original instinct;
it was only wrong about what feeds it.

### `runtime/` — what ARC calls

Retain, release, allocation, deallocation, class metadata, existential
boxes, error propagation. Something has to implement them.

The real question this package has to answer is whether it is Vertex
source compiled by `vsc` itself, or a small set of VIR intrinsics the
backends know. Swift chose a C++ runtime library; we have no C++ and
no libc dependency to lean on, so the intrinsic route is more likely
right. Either way it is a package, and it is on the critical path for
the first program that allocates.

### `module/` — serialized interfaces

For `import` to mean anything across compilations, a module has to be
writable and readable: its declarations, its types, its inlinable
bodies. Swift writes `.swiftmodule` and `.swiftinterface`; we would
write one file that serves both purposes.

Not needed for a single-file compiler, needed for the second one.
Worth naming now so that `analyzer` and `sil` do not grow assumptions
that everything is in one compilation.

### The root, `cli/`, `cmd/vsc/`

Unchanged from the README's plan: the root package composes the phases
and holds the target tables, `cli` is verb dispatch over it, `cmd/vsc`
is the executable. The rule that keeps this honest is that the root
package is the only place that knows the order of the phases, and
every command runs the same ones.

---

## What does not move

The six packages here stay as they are. Two things that might look
like they belong later still belong in `analyzer`:

- **Protocol conformance checking.** Whether a type actually satisfies
  what it declared is a question about the program as written.
- **Effects checking** — `throws`, `async`, actor isolation. Swift
  checks these in Sema, and they are diagnostics about source.

And one thing that looks like it belongs in `analyzer` does not:
**overload resolution's ranking**. Picking among candidates that all
fit is a constraint problem, and when it grows past what
`call.go` does today it should become its own file, or its own package
under `analyzer`, rather than more branches in the checker.

---

## The oracle continues

The reason to follow Swift's phase structure rather than invent one is
not deference. It is that `swiftc` can be asked what the answer is at
every stage we share with it:

```console
$ swiftc -emit-silgen f.swift     # raw SIL, ownership form
$ swiftc -emit-sil    f.swift     # canonical SIL, after the mandatory passes
$ swiftc -emit-ir     f.swift     # LLVM IR
```

The first two are the ones that matter. `-emit-silgen` says what
lowering a construct is *supposed* to produce, down to which values
are `@owned` and which are `@guaranteed`; `-emit-sil` says where the
retains and releases belong after the mandatory passes have run. That
is a differential oracle for the middle end as strong as the one the
parser has, and it exists only if our middle end has the same joints
as Swift's.

It is also a reason to build `sil/text` early rather than late: an IR
you can print and parse is an IR you can write tests in, and the
comparison above is only mechanical if both sides are text.

---

## Sequencing

1. **`core/`** — the built-in module. Widens the subset everything
   else is tested against, and needs nothing new.
2. **`sil/` + `sil/text` + `sil/verify`** — the IR, its text form, and
   the ownership rules. Nothing generates it yet; the verifier and the
   parser are written against hand-written SIL.
3. **`sil/gen`** — enough of it to lower the subset `core/` covers.
   Diff against `swiftc -emit-silgen` from the first function.
4. **`lower/`** — SIL to VIR for the same subset, and the root package
   far enough to run `fib.vs` end to end. This is the spine: once a
   program runs, every later change is testable by running it.
5. **`sil/pass`** — the mandatory passes, ARC first, against
   `swiftc -emit-sil`.
6. **`runtime/`**, then **`module/`**, then **`sil/opt`**.

The thing to resist is building the pass pipeline before the spine
runs. A compiler that cannot yet produce a program has no way to tell
a correct optimization from a plausible one.

---

## Open questions

These are forks where Swift's answer may not be ours:

- **Is specialization mandatory?** Swift can run generics unspecialized
  through witness tables and a runtime; that needs metadata and a
  runtime we may not want. Making specialization mandatory is simpler
  and closes off separate compilation of generic code.
- **Is the runtime Vertex or intrinsics?** Above.
- **Does SIL keep addresses?** Swift's SIL has both value and address
  instructions, and a late pass lowers one to the other. Starting
  address-only is simpler; starting value-only does not survive
  contact with `inout`.
- **How much does `sil/opt` need to exist at all**, given that
  `ir/lower` already optimizes at the machine level and the backends
  are shared with `vcc`? The answer is probably "only the ones that
  need language semantics" — ARC elimination, devirtualization,
  specialization cleanup — and nothing that VIR can do for itself.
