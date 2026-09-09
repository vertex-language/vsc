# TODO

**Swift plus the additions in `docs/vertex_spec.md` are the source of
truth.** Those two together define the language: a program either of
them accepts is valid Vertex, and a program neither accepts is not.
Everything here is a place this compiler disagrees with that union.

Both halves are held to it. A gap in Swift's grammar and a defect in a
Vertex addition are the same kind of bug and are listed together — an
`int32(x)` that would not compile was no more acceptable than a
`mutating` that would not write.

There are three ways to disagree, and they are not equally bad:

| | The language says | This compiler |
| --- | --- | --- |
| **Wrong answer** | valid, means X | accepts it, does something else |
| **Wrongly accepted** | invalid | builds it |
| **Wrongly refused** | valid | will not build it |

Ordered that way rather than by size. The compiler's own rule is that
where it does not know, it says nothing and never invents an answer —
so a wrong answer is the failure that rule exists to prevent, and a
refusal that says why is the shape a gap is supposed to have.

`swiftc` is the oracle for the Swift half. `tests/compiler/` runs a
program under both and compares the answers, which is what separates a
program that builds from one that agrees; `tests/README.md` describes
that corpus and the three beside it.

## Wrong answers

None known.

Six have been found and all six are gone: a getter emitted from the
setter's body, property observers that never ran, a shift that
answered for counts in range and no others, a ternary arm that handed
its join the wrong width, an assignment into an optional that stored
four bytes over five, and a subclass whose inherited properties were
never initialized.

Every one of them compiled, linked and answered, and nothing was said
anywhere — which is why this section stays at the front, and why "it
builds" is never the test. Four of the six were found by writing a
program into `tests/compiler/` and comparing the two answers, and
none by reading the code.

## Wrongly accepted

### `r?.value!` binds the `!` to the chain rather than the member

```swift
struct Reading { var value: Int32 }
func forced(_ r: Reading?) -> Int32 { return r?.value! }
```

Swift reads that as `r?.(value!)` and refuses it: `value` is an
`Int32` and there is nothing to unwrap. This reads it as
`(r?.value)!` and builds it. Both agree on the parenthesised form, so
the difference is only where a postfix `!` lands after a chained
member.

Found by writing it into a corpus program and having *swiftc* reject
the program rather than the compiler.

### A constant that overflows is trapped rather than refused

```swift
func main() -> Int32 {
    let lo: Int32 = -2147483648
    return -lo
}
```

swiftc refuses that at compile time — "arithmetic operation
'0 - -2147483648' results in an overflow" — because both operands are
constants and it can see the answer. This builds it and traps at run
time, which is the same outcome said later.

The mildest kind: a program that never produces a wrong number, and a
diagnostic that arrives after the build rather than during it. The
same expression over a variable traps in both, identically.

### An interop attribute parses and means nothing

```swift
@_cdecl("vs_add")
func add(_ a: Int32, _ b: Int32) -> Int32 { return a + b }
```

That builds, and the object file exports no `vs_add`. The attribute
is read as a name and a balanced token list and then dropped, which
is what happens to every declaration attribute: `ast.Attr` is parsed
everywhere and consulted nowhere — the one `.Attrs` loop in the
checker is for precedence groups, which are not these.

Four attributes are wrong in this particular way, because each one's
whole purpose is a symbol or a dispatch that does not appear:

| Written | What it promises | What happens |
| --- | --- | --- |
| `@_cdecl("f")` | a C-callable symbol `f` | no such symbol |
| `@_silgen_name("f")` | this declaration *is* `f` | a body-less function that traps |
| `@objc` | an Objective-C entry point | nothing |
| `@convention(c)` on a value | a C function pointer | `@convention(thin)`, the ordinary one |

`@_silgen_name` is the sharpest: the program links, runs, and dies on
a trap where the call should have been. `@_cdecl` builds a library
nobody can link against. `@convention(c)` is the quietest, because a
value written that way and then called from Vertex answers correctly
— the convention is only wrong at the boundary it exists for.

Parsing without meaning is the shape `weak` and `unowned` have too,
and it is the wrong-answer shape wearing a different coat: the source
says one thing and the program does another with nothing said. These
should refuse until they work.

## Wrongly refused

Valid programs that do not build yet. Each says so where it is
written, and says which limit it met — the behaviour is right, the
feature is missing.

| Written | What it says |
| --- | --- |
| `defer` | `cannot lower a defer yet` |
| a failable initializer | `cannot lower a failable initializer yet` |
| a closure that captures | `cannot lower a closure that captures 'k' yet` |
| a subscript | `cannot lower a subscript of 'Box' yet` |
| `throw`, `do`/`catch` | `cannot lower a throw yet` |
| a *stored* static property | `it needs storage of its own and the one-time initializer that fills it` |
| `Int32.max`, `Int32.min` | `cannot lower this expression yet` |
| top-level code | `top-level code is not supported` |
| a global `let` or `var` | `cannot lower this expression yet`, at the read |
| a user-declared operator | `cannot lower this expression yet` |
| `super.method()` | `cannot lower this expression yet` |
| a computed property of a class with a subclass | `reached through the table the instance carries` |
| a static computed property's setter | not emitted; static storage comes first |
| a String `rawValue` | `whose cases are not all numbers` |
| an optional payload that owns what it holds | `which owns what it holds` |
| `Bool?` | `an optional with no tag byte whose payload is not a reference` |
| a tuple pattern in a `switch` | `held in memory rather than in a register` |
| a payload enum wider than one word | `an enum whose cases carry values … cannot cross a call` |
| a protocol extension | `value of type 'S' has no member 'twice'` |

Three of these entries want a note.

**`Bool?`** is a layout question rather than a missing feature. It is
one byte, not two: a Bool uses two of a byte's values and the empty
case takes a third, which Swift calls an extra inhabitant.
`types/layout.go` already knows that — `hasSpareValues` is where it
decides — and `lower`'s optionalImage does not, because it knows the
one shape where a tag byte follows the payload and no other. So the
size is right everywhere and the bytes cannot be built. Every optional
whose payload has spare values is the same entry; `Bool?` is the one
reachable without a library.

A tuple, a payload enum past a word, and an aggregate that owns
something are **one piece of work, not three**: each is a value this
compiler holds as separate leaves rather than as a register, and none
of them can be taken apart where a single value is wanted. Fixing that
retires all three.

**A protocol extension** is the one that misleads. It does report —
but it reports `cannot find 'n' in scope` inside the extension and
`no member` at the use, which describes a program that is wrong rather
than a compiler that is missing something. A member declared in an
extension neither sees the protocol's requirements nor reaches
conforming types, and the diagnostic blames the source for it.

**Top-level code** is reported rather than run. Whether file-scope
statements should become the entry point the way Swift's do is open —
requiring `func main` is a defensible rule, and it is at least
*stated* now rather than left to the linker to imply.

## Not started

- **`async`/`await` and actors.** Not modelled. The scanner
  recognises the contextual keywords, which is as far as it goes.
- **`weak` and `unowned`.** They parse — capture specifiers in
  `parser/literal.go`, attributes in `parser/attr.go` — and carry no
  semantics, so a reference cycle leaks. Parsing without meaning is
  the wrong-answer shape again, with a consequence that shows up only
  in memory use.
- **Most of the standard library**, by design rather than omission: it
  is linked from Swift's own, and `tests/interop/` is what holds that
  together.
- **C and Objective-C interoperability**, which is the whole axis
  described below.

## C and Objective-C

Swift calls this **C and Objective-C interoperability**, and the part
of the compiler that does it is the **Clang importer**: the unit it
imports is a *Clang module* — a `module.modulemap` naming headers —
or, for a mixed target, a *bridging header*. Nothing here does any of
it. `import Darwin` reports `no such module`, which is at least true:
there is no importer, so there is no way to name a C declaration at
all.

`tests/interop/` proves the Swift-to-Swift half — this compiler calls
libraries swiftc built, agreeing about mangled names and registers.
The C half has no equivalent, and would need its own corpus for the
same reason: nothing short of running the two together shows it.

It is not one feature, and the pieces are not the same size.

**Calling C** is the smallest and is mostly a type mapping. A header's
declarations become signatures, and C's types become Swift's: `int` is
`CInt`, `char *` is `UnsafeMutablePointer<CChar>!`, `void *` is
`UnsafeMutableRawPointer`, a pointer to an incomplete struct is
`OpaquePointer`, a function pointer is `@convention(c)`, an `NS_ENUM`
is an `enum` and an `NS_OPTIONS` an `OptionSet`. Nullability decides
which of those are optional: an audited header's `_Nullable` is a
`T?`, and an unaudited one's pointers are implicitly unwrapped.

The blocker under all of that is that **the pointer types do not
exist**. `UnsafePointer`, `UnsafeMutablePointer`, `UnsafeRawPointer`
and `OpaquePointer` are all `cannot find type … in scope`, and no C
signature can be spelled without them.

**Being called from C** is smaller still and is the other half of
`@_cdecl`: a symbol with the platform convention, and `@convention(c)`
function values so a callback can be handed over.

**Objective-C is much larger than either**, because it is a runtime
and not a calling convention. It needs `objc_msgSend` dispatch and the
`@convention(objc_method)` thunks around it; classes that are also
Objective-C classes, with metadata the runtime recognises; `@objc`
members, `dynamic`, `#selector` and `#keyPath`; ARC across the
boundary, which is `Unmanaged`, toll-free bridging and autorelease
pools; the error convention that turns a trailing `NSError**` into
`throws`; `@objc protocol` with optional requirements; and, for the
reverse direction, the generated `<Module>-Swift.h`. SIL has
instructions for most of this — `objc_method`, `objc_super_method`,
`objc_protocol`, `thick_to_objc_metatype`, `ref_to_unmanaged` — and
VIL has none of them.

**C++ interoperability** is a separate mode in Swift
(`-cxx-interoperability-mode`) and a separate question here.

Two things are worth saying about where this sits against the rest of
this file. Every other entry is a lowering this compiler does not
emit yet; the Clang importer is a front end for another language,
which is why Swift does not write one either and hands the job to
Clang. And the four attributes above are not waiting on any of it —
refusing them is a small change and is owed now, because a program
that says `@_cdecl` and exports nothing is the failure this file puts
first.

## Fixed

Kept short, because the shape of each is worth more than the detail.

**Answered wrongly:**

- **`<<` and `>>` answered only for counts in range.** They were
  lowered to `shl` and `ashr`, which say nothing about a count of the
  width or more; Swift's answer for every count there is, and a
  negative one shifts the other way. `1 << 40` on an Int32 gave a
  number where Swift gives zero.
- A **subclass's inherited stored properties were never
  initialized**. The default initializer stored what the class
  declared, and an instance holds what its superclass declared too,
  so those kept whatever the allocation left there.
- A **ternary arm** handed its join the arm's own type rather than
  the expression's, so `c ? nil : v` passed four bytes on one edge
  and five on the other.
- An **assignment into an optional did not inject**, so `d = 6 / 2`
  stored the three over the tag byte's neighbour and left d nil.
- A getter was emitted from the **setter's body**, wherever a property
  wrote `set` before `get`. Accessors were picked by position rather
  than by keyword.
- **`willSet` and `didSet` never ran.** They parsed and were dropped,
  so a write stored the value and did half of what the source says.
- A **custom operator over literals** took the *left operand's* type
  as its result — wrong whatever the operator returns, silently so
  where the two agree.
- **`??` answered an optional** where the wrapped type was narrower
  than a literal's default.
- **Top-level statements were discarded**, and the only symptom was an
  undefined `_main` at the link.
- An **optional chain answered `Invalid`**, so it was assignable to
  anything and `??` after one saw a left side that was not optional.

**Refused wrongly:**

- **`guard let` and `while let` did not bind**, and neither did a
  second condition after a binding. Each statement built its control
  flow from a bit, and a binding condition is a switch over which
  case an optional holds; only `if` had a special case for it, for
  exactly one condition.
- **Half the operator table was missing a precedence group**, and an
  operator with none falls back to a non-associative one, so
  `a & b == 8` folded as `a & (b == 8)`.
- The **masking operators** — `&+`, `&-`, `&*`, `&<<`, `&>>` — were
  neither declared nor lowered, and neither were their compound
  forms.
- An **operator written where an optional was wanted** read its
  operands as optionals: `let a: Int32? = 1 + 2` did not convert, and
  `-7` there reported that `-` cannot be applied to an `Int32?`.
- **Optionals could not be compared** — to each other, or to a value
  that is not one. `a == b` typed and would not lower; `a == 7` did
  not type.
- An **inherited property could not be named without `self`**, because
  the checker resolves an unqualified name through the scope chain
  and inheritance is not lexical.
- **`mutating` could not write to its receiver.** `self` crossed every
  call by value, so no `mutating func` on a value type could do the
  thing it exists for. Unblocked `inout` receivers too.
- An optional's payload **could only be one register** — the some arm
  was handed `parts[0]` where the payload is everything before the
  tag. Four guards cited a limit that only the edge imposed.
- **`o!`, `o == nil` and `p?.x`** had no lowering at all; all three are
  the same switch over which case an optional holds.
- An enum's **`rawValue`** could not be read — the raw type was never
  recorded, and the value is not the case's tag.
- A **memberwise initializer** could not leave a property to its
  default; the default lives on the declaration, not at the call.
- **`return` in an initializer** was checked against the type being
  made, so a bare `return` was Void where an `S` was wanted.
- An enum's **computed and static members** were dropped, because
  `readMembers` wanted all three sinks and an enum has no fields.
- A **payload enum of one word** could not cross a call, and a
  **float-to-integer conversion** was refused for want of a bound.
- A **lowercase primitive could not be called**: the aliases had no
  symbol, so `int32(x)` was a constructor call with no type behind it
  while `Int32(x)` was a conversion.
- **Two imported modules could not share a declaration name**, and a
  **range or a binding in a `case`** could not be matched.

**Accepted wrongly:**

- **`mutating` on a class method**, which Swift rejects outright.
- A **stored property in an enum**, which was ignored rather than
  refused.
