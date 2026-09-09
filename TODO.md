# TODO

Swift the compiler does not do yet. Distinct from `proposed.md`, which
is about additions Vertex makes on top; everything here is a gap in
the compatibility layer — Swift a program may reasonably write that
this compiler gets wrong, refuses, or has not reached.

Ordered by how it fails rather than by size, because the compiler's own
rule is that where it does not know, it says nothing and never invents
an answer. The first two sections are where that rule is broken: one
by acting on code it discarded, the other by naming a fix that is
already in place.

## Silently wrong

These typecheck clean and then do something other than what the source
says. They are the ones worth fixing first, whatever they cost.

### Top-level code is discarded

Swift runs statements at file scope — that is what `main.swift` is, and
a program need not declare `func main` at all. Here:

```swift
// toplevel.vs
let x: int32 = 7
print(x)
```

```
$ vsc check toplevel.vs          # exit 0, no diagnostic
$ vsc build -o out toplevel.vs
vsc: build: link: link: check undefined: link: undefined symbol _main
```

`--emit vil` on that file prints an empty module. So the checker
accepts the statements, `vil/gen` drops them without a word, and the
only sign anything is wrong is a missing symbol whose name says
nothing about top-level code.

The statements are not merely skipped — they are checked. Nonsense at
file scope is caught exactly as it would be inside a function:

```
$ vsc check tlbad.vs
tlbad.vs:1:16: error: cannot find 'undefinedName' in scope
tlbad.vs:2:16: error: cannot convert value of type 'String' to specified type 'Int32'
```

So the front end does the whole job and the result is thrown away,
which is the worst arrangement of the three: the compiler proves it
understood the code and then acts as though it had not been written.

Three things are wrong and they are worth separating:

1. The statements are discarded silently. Whatever the entry-point
   rule ends up being, dropping checked code without a diagnostic is
   the failure the "says nothing where it does not know" rule exists
   to prevent.
2. The diagnostic, when it comes, is from the linker and names
   `_main`. A program with no entry point should be told so by the
   compiler, in its own words.
3. Only then, the feature: whether file-scope statements become the
   entry point the way Swift's do.

(1) and (2) are worth doing even if (3) is never wanted — an explicit
`func main` requirement is a defensible rule, but it has to be
*stated*, not left to the linker.

### `??` returns the wrong type when the wrapped type is narrow

```swift
func main() -> int32 {
    let a: int32? = 5
    return a ?? 0
}
```

```
error: cannot convert return value of type 'Int32?' to expected return type 'Int32'
```

The rule in `analyzer/expr.go:566` is right — `T? ?? T -> T`. What
goes wrong is the operand: the literal `0` defaults to `Int`, so
`AssignableTo(Int, Int32)` is false, and the case falls through to
`return lhs`, which is the optional.

Two bugs, one line apart:

- The right operand is not given the wrapped type as its context, so
  an untyped literal defaults instead of adopting `Int32`.
- The fallthrough returns `lhs` rather than reporting a mismatch. A
  `??` whose operands genuinely disagree should be a diagnostic, not a
  quietly optional result.

Confirmed narrow: `a ?? z` with `z: int32` checks clean, and so does
`a ?? 0` when `a` is `int?`, because there the literal's default is
already the wrapped type.

### Two imported modules cannot share a name

```swift
import A   // public func width() -> Int32
import B   // public func width() -> Int32

func main() -> int32 { return A.width() + B.width() }
```

```
error: cannot find 'B.width' in scope: no such name in B
```

`A.width` resolves and `B.width` does not. Every import is declared
into one shared scope so that an unqualified name finds them all, and
`recordModule` then hands each symbol to the module that claimed it
first -- so the second module's same-named declaration never reaches a
scope of its own, and its qualified name cannot be looked up.

Predates folder imports and is not caused by them: the reproduction
above is two `.vertexinterface` files reached through `-I`. It matters
more now, because a package directory makes `fmt.width` and
`other.width` an ordinary pairing rather than a coincidence.

The fix is for `recordModule` to fill each module's own scope from
that module's declarations rather than from the shared scope's
leftovers.

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
body works, and writing `self.x` instead gets past the checker -- so
what is missed is the connection between an implicit-`self` assignment
and the receiver's mutability, not the modifier itself.

Worse than a refusal, because the message names the fix and the fix is
already applied. Anyone who hits this will re-read their own correct
code looking for the mistake.

## Refused honestly

Unimplemented, and they say so at the point of use. Nothing here is
urgent — the behaviour is already correct, only the feature is
missing.

| Swift | What happens now |
| --- | --- |
| `throw`, `do`/`catch` | `cannot lower a throw yet`, `cannot lower a do block yet` |
| `int32(x)` conversions | `cannot lower a constructor call yet` |
| `self.x = …` in a method | `cannot lower an assignment to this expression yet` |

Both typecheck first and refuse at lowering, which is the right shape:
the front end understands the program, and the back end admits what it
cannot build. `throws` in a signature is already read across the
interface boundary, so a `swiftc`-built throwing function can be
called; what is missing is raising and catching one here.

## Not started

- **`async`/`await` and actors.** Not modelled anywhere. Contextual
  keywords are recognised by the scanner, which is as far as it goes.
- **`weak` and `unowned`.** They parse — as capture specifiers in
  `parser/literal.go` and as attributes in `parser/attr.go` — and
  carry no semantics, so a reference cycle leaks. Worth noting that
  parsing without meaning is the same silent-acceptance shape as the
  first section, just with a consequence that only shows up in memory
  use.
- **Most of the standard library**, by design rather than by omission:
  it is linked from Swift's own, and `tests/interop/` is what holds
  that together.

## Documentation

`tests/README.md` says the interface flags line "is not read yet" and
that linking against a resilient library "is a bus error, not a
diagnostic". That is stale. `vsc.go:364` reads the header and refuses
a library built with `-enable-library-evolution`, naming the flag and
saying what to do about it. The note should be corrected so it stops
describing a hazard that is handled.
