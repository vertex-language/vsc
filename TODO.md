# TODO

Swift the compiler does not do yet. Distinct from
`docs/vertex_spec.md`, which defines the additions Vertex makes on
top; everything here is a gap in the compatibility layer — Swift a
program may reasonably write that this compiler gets wrong, refuses,
or has not reached.

Ordered by how it fails rather than by size, because the compiler's
own rule is that where it does not know, it says nothing and never
invents an answer. The first two sections are where that rule would be
broken, and both are empty.

## Silently wrong

Nothing known. This section is for programs that typecheck clean and
then do something other than what the source says — the failure the
compiler's own rule exists to prevent — so it is the one to keep
empty.

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
| a float-to-integer conversion | `what Swift traps on is a bound in the source's own arithmetic` |
| a global `let` or `var` | `cannot lower this expression yet`, where it is read |
| top-level code | `top-level code is not supported` |

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
- **A lowercase primitive could not be called.** `int32(x)` reached
  lowering as a constructor call with no type behind it while
  `Int32(x)` was a conversion, because the aliases lived only in
  `LookupUniverse` and had no symbol — a type in type position and
  nothing in expression position. They are in the universe scope now,
  which is what the spec's "the two are one type" requires.
- **`??` was not lowered.** It is the conditional operator's shape
  asked of the case rather than of a bit: `switch_enum` says which
  case the optional holds, the some arm hands its payload to the
  join, and the none arm evaluates the right operand — so only the
  arm that runs evaluates, which is what the `@autoclosure` on
  Swift's right operand promises.
- **No `mutating` method could write to its receiver.** `self` crossed
  the call by value and never `@inout`, so there was nothing to write
  through: an assignment was refused rather than lowered, and an
  implicit-`self` one was refused with a message saying to declare the
  method `mutating`, which it already was. A mutating method on a
  value type is handed the receiver's storage now — the caller passes
  an address, the callee writes through it, and one mutating method
  calling another passes on what it was given. It is also what
  unblocked `inout` receivers.
- **Two imported modules could not share a declaration name.** Every
  import was declared into one shared scope, and `Scope.Insert` keeps
  the first symbol of a name — so the second module's `width` existed
  nowhere at all, and `B.width` could not be resolved. Each module is
  now read into a staging scope of its own and keeps every symbol it
  declared, in a scope with no parent so a qualified name is exact;
  the shared scope still keeps the first of a name, which is what an
  unqualified reference finds.
