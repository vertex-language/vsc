# VIL — the ownership IR

VIL is Swift's SIL, cloned: the same instructions under the same
names, the same text form, the same ownership model, the same two
stages. Where Swift writes `copy_value`, VIL writes `copy_value`.
Where Swift writes `bb0(%0 : @guaranteed $Box)`, so does VIL.

This document sketched the package before it was written. `vil/` and
`vil/text/` now exist: the types, values, instructions, blocks,
functions and modules, the builder, and the printer. What remains of
the sequence at the end is `vil/verify`, `vil/gen` and `vil/pass`.

## Why cloning exactly is the point

A near-copy would be worse than either a clone or an original design,
because the value of the clone is entirely in one thing:

```console
$ swiftc -emit-silgen f.swift    # what Swift lowered this to
$ vsc build --emit vil f.vs      # what we lowered it to
$ diff                           # the whole test suite
```

Every question the middle end has to answer — is this parameter
`@owned` or `@guaranteed`, does the release go before or after the
call, which values does this `defer` extend the lifetime of, where
does `switch_enum` put its payload — has an answer already written
down by a compiler that has been shipping for a decade. Cloning to a
tee turns that from a reference you read into a test you run.

It is the same method as the rest of this repository. `parser` is held
to every module interface in every SDK; `analyzer` is held to
`swiftc -typecheck`. VIL is held to `swiftc -emit-sil`.

## Shape of the package

Mirrors `ir`, so that someone who knows the Vertex IR knows this one:

| | |
|---|---|
| `vil/` | the IR: values, instructions, blocks, functions, modules, the builder |
| `vil/spec/` | the normative grammar and instruction index, as `ir/spec` is for VIR |
| `vil/text/` | the printer and the parser — `.vil`, `--emit vil` |
| `vil/verify/` | the verifier, ownership rules included |
| `vil/gen/` | checked AST → raw VIL (Swift calls this SILGen) |
| `vil/pass/` | the mandatory passes: raw → canonical |
| `vil/opt/` | later, and only what needs language semantics |

`lower/` then becomes canonical VIL → VIR, and is the smaller job it
should have been all along: by then ownership is resolved, generics
are specialized, and every type is concrete.

## The model

### Modules

A module is a list of items, in SIL's own vocabulary:

```
sil_stage raw | canonical

sil [linkage] [attrs] @name : $type { ... }
sil_global [linkage] @name : $type
sil_vtable Class { #Class.method: @impl, ... }
sil_witness_table [linkage] Type: Protocol module M { method #P.f: @impl }
sil_default_witness_table Protocol { ... }
```

Linkage is Swift's: `public`, `hidden`, `private`, `shared`, with the
keyword omitted for public. Attributes are Swift's: `[ossa]`,
`[transparent]`, `[serialized]`, `[exact_self_class]`, `[thunk]`.

### Types

Two facts about a VIL type, and both are Swift's.

**Object or address.** `$T` is a value; `$*T` is the address of one.
Loadable types move as values, address-only types are manipulated
through their address, and `alloc_stack` / `load` / `store` /
`copy_addr` are how the second kind is worked with.

**Lowered, not formal.** The AST's `(Int) -> Int` becomes
`$@convention(thin) (Int) -> Int`, and the conventions are on the
parameters:

| | |
|---|---|
| `@owned` | the callee takes ownership |
| `@guaranteed` | the caller keeps it alive across the call |
| `@in`, `@in_guaranteed`, `@inout`, `@out` | the address-based forms |
| `@error` | the throw result |
| `@yields` | what a coroutine yields |

and the calling convention is on the function: `@convention(thin)`,
`@convention(method)`, `@convention(c)`,
`@convention(witness_method: P)`.

Metatypes are `@thin` or `@thick`; an opened existential is
`@opened("<uuid>", any P) Self`. Generic signatures print as Swift
prints them, `<τ_0_0 where τ_0_0 : Drawable>`.

### Ownership

The reason the whole thing exists. Every value in an OSSA function has
an ownership kind — **owned**, **guaranteed**, **unowned**, **none** —
and the verifier enforces two rules:

1. An owned value is consumed exactly once on every path out of its
   definition. `destroy_value`, `return`, a `@owned` argument, a
   `store` all consume.
2. A guaranteed value is used only inside the borrow scope that
   produced it — `begin_borrow` … `end_borrow`, or the extent of a
   `@guaranteed` parameter.

Get these right and three things stop being special cases. ARC
insertion becomes bookkeeping over a property the verifier already
proved. `~Copyable` becomes rule 1 with copying forbidden.
`consuming` and `borrowing` become the conventions they already are in
the source.

Block arguments carry their ownership, exactly as in Swift:

```
bb0(%0 : @guaranteed $Box):
bb2(%8 : $Int):
```

### The instruction set

Cloned wholesale. The first cut — what the subset `core/` covers
actually needs — is roughly:

*Values and memory*
`alloc_stack` `alloc_box` `alloc_ref` `dealloc_stack` `dealloc_box`
`dealloc_ref` `project_box` `load` `store` `copy_addr` `destroy_addr`
`begin_access` `end_access` `mark_uninitialized`

*Ownership*
`copy_value` `destroy_value` `begin_borrow` `end_borrow` `move_value`
`extend_lifetime` `end_lifetime` `mark_dependence`

*Aggregates*
`struct` `struct_extract` `struct_element_addr` `tuple`
`tuple_extract` `destructure_tuple` `enum` `unchecked_enum_data`
`init_enum_data_addr` `inject_enum_addr`

*References and dispatch*
`ref_element_addr` `class_method` `witness_method` `function_ref`
`partial_apply` `apply` `try_apply` `metatype` `value_metatype`

*Existentials*
`init_existential_addr` `open_existential_addr`
`init_existential_ref` `open_existential_ref` `alloc_existential_box`

*Control flow*
`br` `cond_br` `switch_enum` `switch_value` `return` `throw`
`unreachable` `yield` `unwind`

*Literals and casts*
`integer_literal` `float_literal` `string_literal`
`unchecked_ref_cast` `unchecked_trivial_bit_cast` `upcast`

*Debug*
`debug_value`

Everything else is added when a construct needs it, under Swift's
name for it, never under a new one.

### The Go surface

`ir`'s style, because the toolchain already has one: build by calling
methods, blocks declared then filled, no phi nodes because block
arguments are the phis.

```go
m := vil.NewModule("app", vil.StageRaw)
fn := m.Func("$s3app3fooyS2iF").Hidden().OSSA()
fn.Convention(vil.Thin)
n := fn.Param(vil.Int, vil.Guaranteed)
fn.Returns(vil.Int)

bb := fn.Entry()
b := bb.CopyValue(n)
bb.DestroyValue(b)
bb.Return(n)
```

One Go method per instruction, named as the instruction is named with
the underscores removed — `copy_value` is `CopyValue`. A reader with
SIL open should never have to translate.

## The text form comes first

Not last. `vil/text` is the oracle's other half, and an IR you can
print *and parse* is one you can write tests in:

```
sil hidden [ossa] @$s1a7matchesySiAA5ShapeOF : $@convention(thin) (Shape) -> Int {
bb0(%0 : $Shape):
  switch_enum %0, case #Shape.dot!enumelt: bb1, case #Shape.line!enumelt: bb2

bb1:
  %1 = integer_literal $Builtin.IntLiteral, 0
  br bb3(%1)

bb2(%2 : $Int):
  br bb3(%2)

bb3(%3 : $Int):
  return %3
}
```

Print exactly this, including the `// user:` and `// Preds:` trailing
comments, because a diff that has to ignore half the line is a diff
that stops catching things.

## The one place a literal clone is impossible

**Symbol names.** Swift's mangling encodes Swift's module names,
declaration kinds and generic signatures: `$s1a3BoxC1nSivg` is
`a.Box.n.getter`. Cloning the *scheme* would mean encoding Vertex
declarations in a grammar designed for Swift's, and the result would
be a name that is neither.

So: our own mangling, and the differential harness normalizes. Both
sides get their symbols replaced by position — the first function
becomes `@f0`, the second `@f1` — before comparing. The same
normalization handles the two other things that differ without meaning
anything: `%`-numbering, and the UUID in `@opened(...)`.

That normalization is the only licence taken. Everything else is
compared literally.

## Stages, and what runs between them

```
vil/gen  ──▶  raw VIL  ──▶  vil/pass  ──▶  canonical VIL  ──▶  lower
                                                                 │
                                                                 ▼
                                                                VIR
```

`sil_stage raw` is what `vil/gen` emits: ownership is explicit but
nothing has been checked, `mark_uninitialized` is still there, and the
program may still be rejected.

`vil/pass` is the mandatory pipeline, in Swift's order because the
order is load-bearing:

1. diagnose invalid escaping captures
2. definite initialization — and drop `mark_uninitialized`
3. mandatory inlining of `[transparent]`
4. closure lifetime fixup
5. `~Copyable` / move-only checking
6. exclusivity diagnostics
7. diagnose unreachable code and missing returns
8. **ownership verification** — the gate to canonical

`sil_stage canonical` is what `lower` consumes: checked, ARC placed,
generics specialized, and — for `kernel` and `graph` — split into host
and device modules.

The rule that keeps `vil/opt` from ever becoming a correctness
dependency: a pass in `vil/pass` may reject a program; a pass in
`vil/opt` may not change whether one compiles.

## The harness

`vil/gen` gets the treatment `parser` got:

```
tests/vil/NN-name.swift        the program
tests/vil/NN-name.vil          what we expect to emit
```

and a test that, when `swiftc` is on the machine, lowers the same
program with `-emit-silgen`, normalizes both sides, and requires them
to agree. When they do not, the expectation file is what changes —
after reading Swift's and deciding it is right.

The same for `vil/pass` against `-emit-sil`, which is where ARC
placement gets checked.

`vil/verify` needs no oracle: it is checked by construction, against
hand-written `.vil` that the parser reads and the verifier must
reject.

## Sequencing

1. **`vil/` + `vil/text`** — types, values, blocks, functions, the
   printer, the parser. Nothing generates VIL yet; the parser is
   tested against SIL text copied out of `swiftc` and re-printed.
2. **`vil/verify`** — SSA, dominance, then the two ownership rules.
   Written against hand-written `.vil`.
3. **`vil/gen`** for the subset `core/` covers — literals, calls,
   structs, control flow — diffed against `-emit-silgen` from the
   first function.
4. **`lower/`** — canonical VIL → VIR, far enough to run `fib.vs`. The
   spine, before any pass exists.
5. **`vil/pass`** — definite initialization first, then ARC, against
   `-emit-sil`.
6. Everything else.

Step 1 before step 3 is the important ordering: the text form is not a
debugging convenience here, it is the instrument the rest is built
with.

## What is deliberately not cloned

Not out of taste — these are SIL constructs for language features
Vertex does not have, and emitting them would mean inventing meanings:

- **Resilience.** No `@opaque` layouts, no
  `sil_default_witness_table` for library evolution, no
  `@convention(method)` ABI-stability machinery beyond what a direct
  call needs.
- **Objective-C interop.** No `objc_method`, no `bridge_object`, no
  `@convention(objc_method)`, no ObjC thunks.
- **Differentiable programming.** No `differentiable_function`.
- **Distributed actors**, until the language has them.

They stay unimplemented rather than renamed. If Vertex grows the
feature, the construct arrives under Swift's name for it.
