# TODO

Swift the compiler does not do yet. Distinct from
`docs/vertex_spec.md`, which defines the additions Vertex makes on
top; everything here is a gap in the compatibility layer — Swift a
program may reasonably write that this compiler gets wrong, refuses,
or has not reached.

Ordered by how it fails rather than by size, because the compiler's
own rule is that where it does not know, it says nothing and never
invents an answer. The first two sections are where that rule is
broken.

## Silently wrong

Nothing known. This section is for programs that typecheck clean and
then do something other than what the source says — the failure the
compiler's own rule exists to prevent — so it is the one to keep
empty.

## Rejected with the wrong reason

### `mutating` is not seen for an implicit-`self` assignment

```swift
struct vec2 {
    var x: int32
    mutating func scale(_ k: int32) { x = x * k }
}
```

```
error: cannot assign to 'x': the receiver is a value, and a method
that changes one has to be declared 'mutating'
```

The method *is* declared `mutating`. Reading a bare `x` in the same
body works, and writing `self.x` instead gets past the checker — so
what is missed is the connection between an implicit-`self` assignment
and the receiver's mutability, not the modifier itself.

Worse than a refusal, because the message names the fix and the fix is
already applied. Anyone who hits this re-reads their own correct code
looking for the mistake.

## Accepted where Swift refuses

The compiler is quiet about a program Swift would reject, so the first
report comes from `swiftc` or from a reader.

- **Protocol extensions.** `extension P { func f() { ... } }` neither
  sees `P`'s own requirements from inside nor reaches conforming
  types, so a default implementation is not one. Nothing is reported;
  the member simply is not there. It is also what blocks a receiver
  method on a protocol — see `docs/vertex_spec.md` §3.3.

## Refused honestly

Unimplemented, and they say so at the point of use. The behaviour is
already correct; only the feature is missing.

| Swift | What happens now |
| --- | --- |
| `throw`, `do`/`catch` | `cannot lower a throw yet`, `cannot lower a do block yet` |
| `int32(x)` conversions | `cannot lower a constructor call yet` |
| `a ?? b` | `cannot lower this expression yet` |
| a global `let` or `var` | `cannot lower this expression yet`, where it is read |
| `self.x = …` in a method | `cannot lower an assignment to this expression yet` |
| top-level code | `top-level code is not supported` |

Two are worth more than their line.

**No `mutating` method can write to its receiver.** `selfConvention`
in `vil/gen` returns `@unowned` or `@guaranteed` and never `@inout`,
so `self` is passed by value and there is nothing to write through. An
ordinary `inout` *parameter* works — `func scale(_ s: inout S, _ k:
int32) { s.x = s.x * k }` runs — so the machinery exists and is not
reaching `self`. Fixing it means passing `self` as `@inout` for a
mutating method, which changes the method convention at every call
site rather than in one place. It is also what blocks `inout`
receivers, which typecheck and stop here.

**Top-level code is refused, not run.** Swift runs statements at file
scope; here they are reported rather than discarded, which is the
honest half. Whether they should become the entry point the way
Swift's do is still open — an explicit `func main` is a defensible
rule, but it is now *stated* rather than left to the linker.

## Not started

- **`async`/`await` and actors.** Not modelled. The scanner
  recognises the contextual keywords, which is as far as it goes.
- **`weak` and `unowned`.** They parse — as capture specifiers in
  `parser/literal.go` and attributes in `parser/attr.go` — and carry
  no semantics, so a reference cycle leaks. Parsing without meaning is
  the same silent-acceptance shape as the first section, with a
  consequence that only shows up in memory use.
- **Most of the standard library**, by design rather than omission: it
  is linked from Swift's own, and `tests/interop/` is what holds that
  together.

## Fixed

Kept briefly, because each was a wrong answer rather than a missing
feature and the shape is worth remembering.

- **`??` returned the wrong type** when the wrapped type was narrower
  than a literal's default. `a ?? 0` against an `int32?` left the
  literal at `Int`, failed the assignability test, and fell out of the
  case still optional. The right operand is now read in the wrapped
  type's context, and operands that genuinely disagree are a
  diagnostic rather than a quietly optional result.
- **Top-level statements were discarded silently.** They were checked,
  then dropped, and the only symptom was an undefined `_main` at the
  link. Now reported, and a module with no entry point is reported by
  the compiler in its own words.
- **`mutating` was accepted on a class method**, where Swift rejects
  it outright.
- **Two imported modules could not share a declaration name.** Every
  import was declared into one shared scope, and `Scope.Insert` keeps
  the first symbol of a name — so the second module's `width` existed
  nowhere at all, and `B.width` could not be resolved. Each module is
  now read into a staging scope of its own and keeps every symbol it
  declared, in a scope with no parent so a qualified name is exact;
  the shared scope still keeps the first of a name, which is what an
  unqualified reference finds.
