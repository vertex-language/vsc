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
| a global `let` or `var` | `cannot lower this expression yet`, where it is read |
| a *stored* static property | `it needs storage of its own and the one-time initializer` |
| binding an optional of a wide payload | `whose payload is more than one register` |
| top-level code | `top-level code is not supported` |
| compound assignment to a computed property | `a call to its setter and not a write to storage` |
| a static computed property's setter | not emitted; static storage first |
| a subscript | `cannot lower a subscript of 'T'` |
| `super.method()` | `cannot lower this expression yet` |
| a computed property of a class with a subclass | `reached through the table the instance carries` |

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
- **A computed property could not be written.** Assigning to one took
  the address of storage it does not have, naming a field the type
  lacks — which the backend reported about a `struct_element_addr`,
  with no line to look at. Setters are emitted now and the assignment
  calls one: the new value first and self last, `@inout` on a value
  type so the write is the caller's, and the reference itself on a
  class. `mangle.Setter` is the one accessor kind the mangler was
  missing.
- **A getter could be emitted from the setter's body.** Which
  accessor a block is was decided by which came first rather than by
  its keyword, so a property writing `set` before `get` had its setter
  emitted as the getter. It compiled and ran, answering whatever the
  setter's body left behind — a wrong answer with no diagnostic, and
  the only one of these found that reached a running program.
- **A computed property on a class emitted invalid VIL.** A getter's
  self is `@guaranteed` the way a method's is — the caller keeps the
  receiver alive across the call and the callee does not consume it —
  but the call handed over an owned copy, so on a class it left a
  reference nothing destroyed. The verifier caught it: an owned value
  not consumed on all paths. Worse than a refusal, because the
  compiler was producing IR it would not accept.
- **A bare computed name inside a member was not lowered.**
  `doubled` inside another member is `self.doubled`, the way a bare
  stored name is `self.n` — but it is a call rather than a field, and
  lowering had no case for it.
- **A getter did not clear self.** One emitted after a mutating method
  or an initializer inherited the storage that one was handed and read
  its properties through an address belonging to another function. The
  same slip as in `functionNamed`, in a second emission path;
  `witness.go` had it right all along.
- **An enum's computed and static members were never recorded.**
  `readMembers` took a var declaration only where the type had
  somewhere to put all three kinds, and an enum has no stored
  properties and so no field sink — which dropped its computed and
  static ones with them. `Dir.count` was "no member 'count'" for
  something the type plainly declares. Each kind is taken where there
  is somewhere to put it now, and a stored property in an enum is
  refused in swiftc's own words rather than ignored.
- **A bare static name inside a member was not lowered.** A type's
  statics are in scope unqualified inside its own members, the way its
  properties are through implicit `self`, and the analyzer resolved
  one — but lowering had no case for it and stopped at the name. It is
  the same call the qualified form makes.
- **A static computed property was refused as though it were stored.**
  A static may be stored or computed and the two shared one list, so
  a computed one was counted among the properties that need storage
  and a one-time initializer — which a getter with nothing behind it
  has no use for. It is a call now, to a getter with no receiver,
  named the way swiftc names one: the instance getter's symbol with Z
  after it, which `mangle.StaticGetter` already wrote.
- **A payload enum could not cross a call**, if it fitted in one
  word. Its memory image is words, and a multi-word one is passed as
  those words — which the ABI arranges out of its leaves. A one-word
  one has a single leaf, so that path did not apply, and `machineOf`
  refused every payload enum outright, so it had no register either:
  usable inside a function, and no machine type at a boundary. The
  word is what the value already was — `makePayloadEnum` defines the
  result as a single i64 when there is one — so saying so was the
  whole fix.
- **A float-to-integer conversion was refused**, because the bound
  Swift traps on was never computed. It is computed against the
  source's own type now — a signed destination of n bits holds
  [-2^(n-1), 2^(n-1)) and an unsigned one [0, 2^n), and every one of
  those powers of two is exact in binary floating point, so the test
  is the range rather than an approximation of it. A NaN compares
  false against both bounds and traps, and `fptosi`/`fptoui` reached
  the backend, which recognised the verbs but did not lower them.
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
