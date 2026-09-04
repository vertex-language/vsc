# What comes after the front end

The six packages in this repository are done in the sense that matters:
they are held to Swift by oracles rather than by opinion, and their
shape will not change. `token`, `scanner`, `ast`, `parser`, `types`,
`analyzer`. This document is about everything after them, and it
replaces the sketch in the README, which was drawn before any of it
was built.

The question it exists to answer: **is one `lower` package enough?**

Nearly. One lowering, and one thing above it that does not exist yet
and is not an IR. The first draft of this document said no and pointed
at Swift's SIL; the numbers below are why that was too quick.

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

## Where the CFG lives

Three places it could go, and the costs are the whole decision.

**In `analyzer`.** A CFG whose nodes are AST expressions, with the
dataflow passes over it. No new IR, no instruction set, no text form,
no verifier, no parser. Diagnostics come out naming source constructs
because the nodes *are* source constructs — which is the thing every
mandatory pass has to do and the thing a lowered form makes hardest.
Lowering then emits retain and release conservatively, the way SILGen
does before anything optimizes them.

**In a SIL of our own.** Buys an SSA form to do dataflow on, which is
genuinely easier than an AST-shaped CFG, and buys the `swiftc
-emit-sil` diff oracle. Costs an IR that duplicates two thirds of VIR,
plus its text form, verifier and parser — and every one of those is a
thing to keep correct forever.

**In VIR.** Cheapest to imagine and the worst idea here, for one
reason that has nothing to do with Swift: **VIR is shared with `vcc`
and `v++`.** Teaching it ownership means teaching a C compiler's IR a
concept C does not have, and every pass in `ir/lower` would carry it.
An IR shared by three languages should hold what all three mean.

**The recommendation is the first.** Build the CFG in `analyzer`, keep
`lower` as one package, emit conservative ARC on the way down, and do
not build a second IR until something forces it.

What would force it, concretely: an ARC optimizer good enough to
matter, or specialization that needs to clone bodies rather than
re-lower them. Both are optimizations. Neither is due before a program
runs end to end. And if that day comes, the cheap version is not SIL —
it is teaching VIR to recognise `retain` and `release` as calls with a
known meaning, which is metadata on a call rather than an ownership
model in the type system.

---

## The shape

```
  ast + types + analyzer          the checked program        (done)
     └── analyzer/cfg             definite init, exclusivity,
            │                     move-only, reachability
            │  lower              ARC emitted conservatively,
            ▼                     generics specialized by re-lowering
      vir                          typed SSA, machine-level     (exists)
            │
            │  ir/lower  (in the ir repository)
            ▼
      isel → regalloc → object → link                          (exists)
```

One lowering. The middle end is a graph and some dataflow, not a
second compiler.

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

### `analyzer/cfg` — control flow, and the passes over it

A control-flow graph over the checked tree: blocks of AST statements,
edges for every way control moves — branches, loops, `guard`, `defer`,
`throw`, `break` to a label, the arms of a `switch`. Then the dataflow
passes that can reject a program:

- definite initialization, including `self` in an initializer
- `~Copyable` and `consuming`: every owned value consumed once
- exclusive access to memory
- unreachable code and missing returns
- escaping-closure capture

A subpackage of `analyzer` rather than its own top-level package,
because it answers the same kind of question the rest of `analyzer`
does — is this program legal — and reports it the same way, against
source positions, in the same `Info`. Its output is diagnostics plus
one thing `lower` needs: **where each value's last use is**, which is
what makes conservative ARC placeable without a second IR.

This is the package that does not exist yet and is the whole middle
end. It is a few thousand lines, not a compiler.

### `lower/` — checked AST to VIR

One package, as the README always said. It walks the tree with
`Info` and the CFG's answers in hand and emits VIR: classes become
pointers, existentials become boxes, enums become tags and payloads,
the layout `types` computed becomes offsets, generic functions are
lowered once per instantiation from the same AST with a substitution
map, and retain/release go where the CFG said the lifetimes end.

The reason this can be one package is that everything it needs to
*decide* was decided above it. It is a translation.

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

### `sil/` — if and when

Not now. If an ARC optimizer or a cloning specializer ever earns it,
what to build is an SSA form of the twelve ownership opcodes above and
nothing else — the other twenty-five are VIR's, and duplicating them
is how a middle end becomes a second compiler.

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

## What the oracle can still tell us

Not having a SIL costs one thing, and it is worth being precise about
what.

```console
$ swiftc -emit-silgen f.swift     # raw SIL, ownership form
$ swiftc -emit-sil    f.swift     # canonical SIL, after the mandatory passes
```

What is lost: a mechanical diff. Without a matching text form there is
nothing to compare line against line, the way the parser compares
verdicts.

What is kept, and it is most of the value: **swiftc will tell you the
answer to any ownership question you can phrase as a program.** Is
this parameter `@owned` or `@guaranteed`? Where does the release of
this temporary belong — before or after the call? Does this `defer`
run before or after the return value is copied? Read the SIL, and the
answer is there, in a form that names the source constructs.

That is how the ownership rules in `analyzer/cfg` should be written:
one small program per rule, its SIL read once to learn what Swift
decided, and the rule then written and tested against the *behaviour*
— which values must be released, which must not — rather than against
Swift's text.

The other two oracles do not change. Every module interface in every
installed SDK must still parse, and `swiftc -typecheck` must still
agree with the checker about which programs are Swift.

And when a program finally runs, there is a third and better one:
compile it with `vsc`, compile it with `swiftc`, run both, compare
what they print. An ARC bug is a leak or a crash, and neither is
subtle.

---

## Sequencing

1. **`core/`** — the built-in module. Widens the subset everything
   else is tested against, and needs nothing new.
2. **`analyzer/cfg`** — the graph and the reachability pass, then
   definite initialization on it. Both are testable against
   `swiftc -typecheck` the day they are written, because both are
   diagnostics.
3. **`lower/`** — enough to lower the subset `core/` covers, plus the
   root package far enough to run `fib.vs` end to end. This is the
   spine: once a program runs, every later change is testable by
   running it.
4. **ARC**, on the lifetimes the CFG already computes, with
   `runtime/` behind it. The first program that allocates and frees
   correctly is the milestone that matters.
5. **`~Copyable`, exclusivity, the rest of the mandatory passes** —
   each one a pass on a graph that already exists.
6. **`module/`**, then optimization, then a `sil/` only if something
   forces it.

The thing to resist is building a middle end before the spine runs. A
compiler that cannot yet produce a program has no way to tell a
correct optimization from a plausible one.

---

## Open questions

Forks where Swift's answer may not be ours:

- **Is specialization mandatory?** Swift can run generics
  unspecialized through witness tables and a runtime. That needs
  metadata and a runtime we may not want. Mandatory specialization is
  simpler and closes off separate compilation of generic code.
- **Is the runtime Vertex or intrinsics?** Above.
- **Does the CFG need SSA?** Dataflow over an AST-shaped graph is
  fiddlier than over SSA — no value numbering, no phi nodes, aliasing
  answered by hand. If definite initialization and move-only checking
  turn out to want SSA badly enough, that is the thing that would
  justify a small ownership IR, and it is the honest trigger to watch
  for rather than a decision to make now.
- **Where does the host/device split go?** `kernel` and `graph` bodies
  need to become their own module. On the tree it is a walk; after
  lowering it is two VIR modules. Probably the latter, since VIR is
  what a device backend consumes — which would make it `lower`'s job
  and not a phase of its own.
