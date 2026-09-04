# What comes after the front end

The six packages in this repository are done in the sense that matters:
they are held to Swift by oracles rather than by opinion, and their
shape will not change. `token`, `scanner`, `ast`, `parser`, `types`,
`analyzer`. This document is about everything after them, and it
replaces the sketch in the README, which was drawn before any of it
was built.

The question it exists to answer: **is one `lower` package enough?**

No. There is an ownership IR above it — `vil`, a clone of Swift's SIL
— and [proposed_vil.md](proposed_vil.md) is its design. This document
is the argument that got there and the layout around it.

The audit below is worth keeping even though the decision went the
other way, because it says what `vil` is *for*: two thirds of SIL is
VIR's job, and the third that is not is the whole reason to build it.

---

## What of SIL is actually ours to build

SIL is Apple's, and a good deal of it is Apple's problem. Lowering a
small program with a class, a struct, an enum, a protocol and a
generic produces 37 distinct SIL opcodes. Twenty-five of them have a
direct VIR equivalent:

> `apply` `function_ref` `load` `store` `struct` `struct_extract`
> `tuple` `br` `return` `switch_enum` `integer_literal` `alloc_ref`
> `alloc_box` `project_box` `ref_element_addr` `metatype`
> `witness_method` `unchecked_ref_cast` `dealloc_ref` …

Calls, memory, aggregates, branches, constants, switches. VIR has all
of it, and `vcc` already lowers C straight into it in one flat
package. Building a second IR to hold the same concepts would be
duplication, and the fact that Swift has one is not a reason: swiftc
needs SIL to be a *complete* IR because it hands off to LLVM, which
knows nothing about Swift. We hand off to VIR, which is ours.

Twelve opcodes have no VIR equivalent, and they are all the same
thing:

> `copy_value` `destroy_value` `begin_borrow` `end_borrow`
> `move_value` `begin_access` `end_access` `mark_uninitialized`
> `end_lifetime` `extend_lifetime` `unchecked_ownership_conversion`
> `debug_value`

Ownership, lifetime, and exclusive access. That is the residue, and it
is about 38% of the instruction lines in a lowered function.

A second slice of SIL's job is already the analyzer's, or could be:

| SIL does it there | but it is a question about the source |
|---|---|
| the type of every value | `Info.Types` already holds it |
| which function a call names | `Info.Uses` and overload resolution |
| which witness a protocol call uses | conformance is already nominal here |
| definite initialization | Java specifies definite assignment on the *source* language; it needs a CFG, not an IR |
| exclusive access | dataflow over the same CFG |
| `~Copyable` and `consuming` | dataflow over the same CFG |
| generic specialization | a substitution map and a second lowering of the AST |

None of those needs an instruction set. They need a **control-flow
graph over language-level values** — which is the honest irreducible
core of what SIL is doing for us, and it is much smaller than SIL.

---

## Where the ownership work lives

Three places it could have gone, and the costs were the whole
decision.

**In `analyzer`, as a CFG.** A control-flow graph whose nodes are AST
expressions, with the dataflow passes over it. No new IR, no
instruction set, no text form, no verifier, no parser. Diagnostics
come out naming source constructs because the nodes *are* source
constructs. The smallest thing that could work.

**In an IR of its own.** Costs an IR that duplicates two thirds of
VIR, plus its text form, verifier and parser — every one of them a
thing to keep correct forever. Buys an SSA form to do dataflow on, and
buys the `swiftc -emit-sil` diff.

**In VIR.** Cheapest to imagine and the worst idea here, for a reason
that has nothing to do with Swift: **VIR is shared with `vcc` and
`v++`.** Teaching it ownership means teaching a C compiler's IR a
concept C does not have, and every pass in `ir/lower` would carry it.
An IR shared by three languages should hold what all three mean.

**It went to the second, and the reason is the oracle.** The CFG would
have been smaller and would have been ours to get right alone. A
faithful clone of SIL is bigger and is checked by `swiftc -emit-sil`
line for line: every ownership question the middle end has to answer
already has an answer that can be diffed rather than argued about.
That is the same trade the parser and the checker took, and it has
paid twice.

What did not change is the third option. Ownership does not go into
VIR. VIL is where Vertex's ownership lives, and VIR stays the
machine-level IR all three compilers lower into.

---

## The shape

```
  ast + types + analyzer          the checked program        (done)
            │
            │  vil/gen
            ▼
      vil  (raw, ossa)             classes are classes,
            │                      values are owned or borrowed,
            │  vil/pass            generics are still generic
            ▼
      vil  (canonical)             checked, ARC placed,
            │                      specialized, host/device split
            │  lower
            ▼
      vir                          typed SSA, machine-level     (exists)
            │
            │  ir/lower  (in the ir repository)
            ▼
      isel → regalloc → object → link                          (exists)
```

---

## The packages

### `core/` — the built-in module

`Int`, `Bool`, `Double`, `String`, `Array`, `Optional`, their
operators and their conformances, as declarations the analyzer reads
the way it reads any other. Today `Int` has no members and `+` is not
declared anywhere, which is why any program touching a `String` stops
at the checker.

Its own package because it is *input*, not code: the analyzer imports
it, `lower` needs to name its entry points, and the runtime
implements some of them. It is also the one package a user might one
day replace.

**Build this first.** It is what widens the subset everything else is
tested against.

### `vil/` — the ownership IR

Swift's SIL, cloned. The instructions under their own names, the text
form, the ownership model, the two stages, and the passes that move
between them. It is the whole middle end and it has its own document:
[proposed_vil.md](proposed_vil.md).

The diagnostics that a CFG would have carried — definite
initialization, `~Copyable`, exclusivity, reachability — are passes
over VIL in `vil/pass`, which is where Swift runs them and therefore
where `swiftc -emit-sil` can be asked whether we got them right.

### `lower/` — canonical VIL to VIR

One package. By the time it runs, ownership is resolved, ARC is
placed, generics are specialized and every type is concrete, so this
is a translation rather than a decision: classes become pointers,
existentials become boxes, enums become tags and payloads, and the
layout `types` computed becomes offsets.

### `runtime/` — what ARC calls

Retain, release, allocation, deallocation, class metadata,
existential boxes, error propagation. Something has to implement them.

The real question is whether it is Vertex source compiled by `vsc`
itself or a small set of VIR intrinsics the backends know. Swift chose
a C++ runtime library; we have no C++ and no libc to lean on, so
intrinsics are more likely right. Either way it is a package, and it
is on the critical path for the first program that allocates.

### `module/` — serialized interfaces

For `import` to mean anything across compilations, a module has to be
writable and readable: its declarations, its types, its inlinable
bodies. Not needed for a single-file compiler, needed for the second
one. Worth naming now so that `analyzer` and `lower` do not grow the
assumption that everything is in one compilation.

### The root, `cli/`, `cmd/vsc/`

Unchanged: the root package composes the phases and holds the target
tables, `cli` is verb dispatch over it, `cmd/vsc` is the executable.
The rule that keeps this honest is that the root package is the only
place that knows the order of the phases, and every command runs the
same ones.

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

## What the oracles are now

Three, and each phase has one.

```console
$ swiftc -typecheck   f.swift     # is this program Swift?          → analyzer
$ swiftc -emit-silgen f.swift     # what does lowering produce?     → vil/gen
$ swiftc -emit-sil    f.swift     # where do the retains belong?    → vil/pass
```

Plus the one that needs no swiftc: every module interface in every
installed SDK must parse.

`-emit-silgen` is the one that pays for VIL. It answers, mechanically,
every question lowering has to make a decision about — whether a
parameter is `@owned` or `@guaranteed`, where the release of a
temporary belongs, what a `defer` extends the lifetime of, how
`switch_enum` delivers a payload. Without a matching text form those
are questions you read and interpret; with one they are a diff.

And once a program runs there is a fourth, better than all of them:
compile it with `vsc`, compile it with `swiftc`, run both, compare
what they print. An ARC bug is a leak or a crash, and neither is
subtle.

---

## Sequencing

1. **`core/`** — the built-in module. Widens the subset everything
   else is tested against, and needs nothing new.
2. **`vil/` + `vil/text`** — the IR and its text form. The printer and
   parser first, because they are the instrument everything after is
   built with.
3. **`vil/verify`** — SSA, dominance, then the two ownership rules.
4. **`vil/gen`** for the subset `core/` covers, diffed against
   `swiftc -emit-silgen` from the first function.
5. **`lower/`** — canonical VIL to VIR, plus the root package far
   enough to run `fib.vs` end to end. The spine, before any pass
   exists.
6. **`vil/pass`** — definite initialization first, then ARC, against
   `swiftc -emit-sil`.
7. **`runtime/`**, then **`module/`**, then **`vil/opt`**.

The thing to resist is building the pass pipeline before the spine
runs. A compiler that cannot yet produce a program has no way to tell
a correct optimization from a plausible one.

---

## Open questions

Forks where Swift's answer may not be ours:

- **Is specialization mandatory?** Swift can run generics
  unspecialized through witness tables and a runtime. That needs
  metadata and a runtime we may not want. Mandatory specialization is
  simpler and closes off separate compilation of generic code — and it
  decides how much of `vil`'s generic machinery has to survive into
  canonical stage.
- **Is the runtime Vertex or intrinsics?** Above.
- **Does `vil` keep addresses?** Swift's SIL has both value and
  address instructions, with a late pass lowering one to the other.
  Cloning it means cloning both. Starting value-only does not survive
  contact with `inout`, so the answer is probably yes from the start,
  which is also what keeps the diff honest.
- **Where does the host/device split go?** `kernel` and `graph` bodies
  need to become their own module. As a `vil/pass` it happens where
  the language semantics are still visible; as part of `lower` it
  happens where the device backend's input is. Probably the former,
  since a kernel's captures are an ownership question.
