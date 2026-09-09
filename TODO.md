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

Two have been found and both are gone: a getter emitted from the
setter's body, and property observers that never ran. Both compiled,
linked and answered wrongly with nothing said, which is why the
section is worth keeping in front of the rest.

## Accepted where Swift refuses

### `r?.value!` binds the `!` to the chain rather than the member

```swift
struct Reading { var value: Int32 }
func forced(_ r: Reading?) -> Int32 { return r?.value! }
```

swiftc reads that as `r?.(value!)` and refuses it — `value` is an
`Int32` and there is nothing to unwrap. This reads it as
`(r?.value)!` and accepts it. Both agree on the parenthesised form,
so the difference is only where a postfix `!` binds after a chained
member.

Found by writing it in a corpus program and having swiftc reject the
program rather than the compiler.







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
| a static computed property's setter | not emitted; static storage first |
| `defer` | `cannot lower a defer` |
| a closure that captures | `cannot lower a closure that captures 'k'` |
| a chain whose payload is more than a register, or owns what it holds | `a chain through an optional of B` |
| a failable initializer | `cannot lower a failable initializer` |
| a user-declared operator | `cannot lower this expression yet` |
| a String `rawValue` | `whose cases are not all numbers` |
| a tuple pattern in a `switch` | `a tuple pattern over (Int32, Int32), which is held in memory` |
| a subscript | `cannot lower a subscript of 'T'` |
| `super.method()` | `cannot lower this expression yet` |
| a computed property of a class with a subclass | `reached through the table the instance carries` |

**A tuple pattern needs the subject in a register.** Its elements are
taken out of the subject, and a tuple of more than one word is memory
— its elements are separate leaves rather than parts of a register —
so there is nothing to extract from, and every tuple this compiler can
hold is one of those. Lowering one waits on the same layout work as a
wide struct and a payload enum wider than a word. The elements are
checked either way, so a mistake inside one is reported where it is
written.

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
- **An optional chain was not lowered.** Its type was fixed earlier;
  the lowering is a switch with the member read inside the some arm,
  which is what makes a chain a chain — the read only happens where
  there is something to read from, and both arms hand the join an
  optional. Limited to a payload that fits a register and owns
  nothing, which is the same limit a binding condition has: a class
  payload owns its reference, and a struct of two fields is memory.
- **`o!` and `o == nil` were not lowered.** A force unwrap had no
  case in lowering at all, and a comparison against nil none either:
  nil is not a value of a type core declares an operator over. Both
  are questions about which case the optional holds, so both are a
  switch — `!` traps on the none arm, which is what makes it an
  assertion rather than a conversion, and with the same message and
  the same signal swiftc gives. `nil == o` was also rejected outright,
  since nil is not comparable to anything on its own and that was
  asked of the left operand.
- **An enum's `rawValue` could not be read.** The raw type was never
  recorded, though `rawValueOf` existed to read it — so the property
  did not exist, and the case values were checked against nothing. It
  is a switch over the case now, each arm handing the join what the
  source wrote, because the value is not the tag: `case bad = 7` is
  the second case carrying a 7, and answering the tag answers 1.
  Cases the declaration leaves out are numbered Swift's way, from
  zero and continuing from the last that said a number. A String raw
  value is refused: answering one needs a string constant.
- **A custom operator over literals took the wrong type.** A literal
  has no type of its own to match a declaration with, and an
  operator's operands have no context until the operator is known —
  a circle. Both defaulted to Int, so an operator declared over Int32
  did not fit, and the result fell back to the *left operand's* type:
  wrong whatever the operator returns, and silently so where the two
  agree. The operands are read again in the parameters' types now,
  and only where the plain lookup found nothing.
- **`return` in an initializer did not compile.** It was checked
  against the type being made, so a bare `return` was Void where the
  type was wanted — the one return statement an initializer may write,
  and swiftc is exact about it: *"'nil' is the only return value
  permitted in an initializer"*. An initializer's return takes no
  value now, and lowering gives it the same thing the end of the body
  gives: the value built so far, with the box torn down. `init?` may
  return nil, which is the one thing it exists to do; lowering one is
  still refused, so it stops honestly at the backend rather than
  confusingly at the checker.
- **An optional chain lost its optionality.** `p?.x` answered
  `Invalid` — the lookup ran on a doubly optional type, since `p?`
  wraps whatever it followed and `p` was already optional, and found
  nothing without reporting it. So the chain was assignable to
  anything, which Swift refuses, and `??` after one saw a left side
  that was not an optional at all. A chain reaches through both
  layers now and answers an optional of the member, flattened rather
  than nested.
- **Property observers were parsed and dropped.** A write to a
  property with `willSet` or `didSet` stored the value and ran
  neither, so the program compiled, ran and did half of what its
  source says — `s.n = 5` where both observers touch a log left the
  log at 0 where swiftc leaves 11. A write is the store with the
  observers around it now: the old value read first, `willSet`, the
  store, `didSet`. The old value is read before the store because
  after it there is nothing left to read.
- **A memberwise initializer could not leave a property to its
  default.** `S()` where `S` declares `var n: int32 = 5` was refused —
  as ordinary as Swift gets. The default is an expression on the
  declaration rather than at the call, so nothing at the call had it
  to lower; the analyzer recorded *that* there was one and not what it
  was. It records the expression now, and construction walks the
  properties in order taking the argument where there is one and the
  default where there is not.
- **A tuple pattern checked every element against the whole tuple.**
  `case (0, 0)` over a pair of `int32` was two errors about a scalar
  that cannot match a tuple, and a binding in one got the tuple's type
  rather than its own. Each element is checked against the subject's
  element at that position now — which the enum-case branch beside it
  had been doing all along.
- **A `switch` could not bind or guard.** `case let k` was refused as
  "this pattern in a switch", and a `where` clause with it. A binding
  names the subject and matches whatever it is, so it is a default
  with a name — nothing after it is reachable, and the continuation is
  not made where nothing reaches it. A `where` clause is read after
  the binding, since it is written about the names the pattern
  declares, and the two are and-ed without a branch.
- **Every range pattern was an error.** `case 1...5` compared the
  range against the subject, and a `ClosedRange<Int>` is not an `Int`
  — so it failed at every subject type, not only where the literals
  needed adopting. What agrees with the subject is the range's
  *element*: Swift matches through `~=` over a RangeExpression, whose
  Bound is the subject's type. The bounds take the element's context
  now, so they do not default to Int, and lowering is two comparisons
  rather than one equality — closed or half-open as the operator says.
- **A `case let` binding was not in scope in its own `where` clause.**
  The condition was checked before the pattern declared anything, so
  `case let k where k < 0` reported the name it was about to declare.
- **A computed property could not be written.** Assigning to one took
  the address of storage it does not have, naming a field the type
  lacks — which the backend reported about a `struct_element_addr`,
  with no line to look at. Setters are emitted now and the assignment
  calls one: the new value first and self last, `@inout` on a value
  type so the write is the caller's, and the reference itself on a
  class. `mangle.Setter` is the one accessor kind the mangler was
  missing. A compound assignment is the getter, the operator and the
  setter — the base evaluated twice, which is what every other
  destination already does here.
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
