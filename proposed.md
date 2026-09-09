# Proposed

The Vertex additions: what each means, and the reasoning a later
change to one has to agree with. Additions only — Swift the compiler
does not do yet is `TODO.md`, and the rule that sorts one from the
other is in the README: every addition is written down here, and
anything else that differs from Swift is a bug.

| Addition | State | Where it lives |
| --- | --- | --- |
| Lowercase primitives | **done** | `types/universe.go` |
| `kernel` / `graph` | **done** | `parser/decl.go`, refused in `vil/gen` |
| Optional argument labels | **done** | `analyzer/call.go` |
| Packages and folder imports | **done** | `vsc.go` importer, driver in `internal/cli` |
| Receiver methods | not built | `parser`, desugaring to an extension |

Two through-lines. A module's identity is a single identifier, because
that is what a symbol is mangled with. And Swift's semantics stay
Swift's: what these add is grammar and where files are found, not a
second model of what a module or a method is.

## 1. Lowercase primitives

Lowercase spellings — `int32`, `float64`, `bool` — each an alias in
`types/universe.go` for the same `types.Basic` as its capitalised
counterpart. `LookupUniverse` reads Swift's names first, and
`LookupSwiftUniverse` is the Swift-only view for the places that need
one.

The pattern the others follow: **an addition is an alias onto an
existing thing, not a parallel implementation of it.** `int32` and
`Int32` are one type, so layout, mangling and lowering never learn
there were two spellings, and the feature costs nothing after the
table.

## 2. `kernel` and `graph`

Reserved modifiers for a data-parallel unit and a graph node:

```swift
func add(_ a: float32) kernel -> float32 { return a }
```

They sit between the throws clause and the result arrow, where nothing
else in the grammar may stand — after the closing paren only `async`,
`throws` and the arrow appear. So both stay contextual, and a
function, parameter or variable may still be called either.

**They are refused, not ignored.** The signature and body typecheck,
and `vil/gen` reports `cannot lower a kernel function yet`. The
alternative was to lower one as an ordinary function, which would run
on the CPU and return a right-looking answer — and would pass the
corpus, since a file lands in `tests/compiler/` once it builds and
runs. A reserved word that silently compiles to something else is
worse than one that does not compile.

## 3. Optional argument labels

Labels are carried for compatibility, so writing them is optional in
both directions: `_` on a parameter may be left off, and a label may
be written where the declaration says `_`.

```swift
func addUp(a: int32, b: int32) -> int32 { return a + b }

addUp(1, 2)         // no labels
addUp(a: 1, b: 2)   // labels
```

**Strict first, lax as a fallback.** A call whose labels are written
the way the declaration asks resolves exactly as it always did; only a
call the strict rule rejects is tried again with labels optional. No
program that compiled before can reach the second pass, so the change
cannot alter an existing meaning.

**Labels still disambiguate.** `tests/check/ok-34-overloads.swift`
declares `label(a:)` and `label(b:)` — two functions differing in
nothing else, mangling to two symbols. `label(1)` names neither, so it
is `ambiguous use of 'label': 'a:', 'b:'` rather than a guess. That
also keeps interop intact: a `swiftc`-built library may export two
functions differing only by label, and a call has to reach both.

**Two places keep requiring them**, because there a label is what says
*which* parameter is meant rather than decoration:

- A short argument list, where defaults are being skipped. With
  `func f(_ a: Int, b: Int = 2, _ c: Int)`, making labels optional
  would silently reread `f(1, 3)` as filling `b` instead of `c`.
- A variadic. `variadicParams` uses the label to decide where the list
  *stops* — it takes every unlabelled argument, and a labelled one
  after that belongs to the parameter of that name.

So the rule is: optional wherever the argument count matches the
parameter count, required where it does not.

## 4. Packages and folder imports

> **Add grammar. Reuse Swift's semantics.** No second model of what a
> module is, what a namespace is, or how a name resolves. Swift has
> answers and this compiler implements them; what was missing was a
> way to *write* the thing in source.

Go is a reference where its solution to a shared problem is
instructive. It is not the model.

### The grammar

```swift
package geometry          // the module this file belongs to

import Foundation         // unchanged: a module, found the usual way
import "std/fmt"          // a folder, found in the package directory
import "./geometry"       // a folder, relative to this file
import acmefmt "acme/fmt" // bound under another name
import (                  // punctuation: one import each
    "std/math"
    "./units"
)
```

`package` is contextual against Swift's access level of the same name:
followed by a plain identifier it is the clause, followed by a
declaration keyword or another modifier it is the modifier. Nothing is
newly reserved and no valid Swift file changes meaning.

### Why nothing below the importer changed

Swift has exactly one namespace and it is the **module** — no
`namespace` keyword, no submodules, and the caseless `enum` as the
in-language substitute. This compiler already models it: `c.modules`
is a scope per module, `recordModule` fills it, `isModuleRef` decides
a bare name is a module and not a variable.

And a Swift file never says which module it belongs to; the build
system does, with `-module-name`. So a folder import needed no new
resolution rule, no new scope kind and no new qualified-name syntax —
only a way to say which files are compiled together, which is the one
thing Swift never put in the language.

SwiftPM has had the convention all along: a target is a directory
under `Sources/`, and the directory's name is the module's. This is
that convention moved into the language, rather than Go's packages
moved into Swift.

### Naming, and casing

A path's last segment names the module — `"std/fmt"` gives `fmt` —
unless the folder's files say otherwise with `package`. The folder
name is the default because SwiftPM already works that way and a
rename cannot fall out of sync with a declaration that does not exist;
the clause is the override for a directory whose name is not an
identifier, and it must be one, since `mangle.module` writes it
length-prefixed into every symbol.

Either case is legal, and already was: the entry module is `main`,
lowercase, in every symbol of every program built. `$s8geometry…` and
`$s8Geometry…` differ only where you would expect.

One consequence, unresolved. Modules and values share a namespace at
the point of use — `isModuleRef` treats a bare name as a module only
where the local scope does not already have it — so a local shadows a
module of the same name, silently:

```swift
import geometry

func main() -> int32 {
    let geometry: int32 = 7    // shadows the module; checks clean
    return geometry
}
```

Lowercase makes that likelier, since lowercase is what locals look
like; Swift's capitalised-module convention is what keeps the two
populations apart. A warning when a local shadows an imported module
would cost little.

### Where a path resolves

| Written | Found in |
| --- | --- |
| `"std/fmt"`, `"acme/geometry"` | each package root: `-P`, then `VERTEXPATH` |
| `"./geometry"`, `"../shared"` | relative to the importing file |

The first is the ordinary case and is where a package manager writes
what it downloads — so fetching never becomes a language feature, since
a downloaded repository is just a folder once it lands. The second is
the marked form, for local work and tests: `./` means "not from the
package directory".

The root is configurable for a concrete reason: tests need to point at
a directory they populated, and a build reading a developer's home
directory is not reproducible.

`-I` stays separate. It finds *built* modules — a `.vertexinterface`
or `.swiftinterface` — which is how a `swiftc`-built library is
reached. The package roots hold *source*. Whether they should merge is
open.

### What an imported folder is

Both halves shipped. The importer reads the folder's `.vs` files and
hands them to the checker as an `analyzer.Import`, exactly as it hands
over an interface — because an interface *is* source with the bodies
taken out, so the same passes run over both and neither has its bodies
checked. Then `Unit.Packages` reports the folders a compilation
imported, in dependency order, and a loop in `internal/cli/build.go`
compiles each with its own module name and adds its object to the
link.

The prediction held: nothing in `analyzer`, `vil/gen`, `lower`,
`mangle` or `build` changed. The one analyzer change was for the
rename — `Import.As` is what this program calls the module, separate
from `Name`, which is what its symbols mangle with.

### Still open

- **A rename does not fix a symbol collision.** `import acmefmt
  "acme/fmt"` changes what this file calls the module, not what the
  module's symbols are mangled with. Linking two modules both named
  `fmt` is a duplicate-symbol error either way; the `package` clause
  is the real escape hatch, because it changes identity rather than
  reference. Worth stating, since the rename looks like it solves
  this. (Go puts the path in the symbol. Not available here: the
  mangling is shared with `swiftc`.)
- **Two imported modules cannot share a declaration name.** Predates
  folder imports and matters more now — see `TODO.md`.
- **Source or builds in the package directory?** Source is assumed,
  so a program builds its dependencies. Caching objects beside the
  source is where something has to decide what is stale, and where
  this stops being a language question.
- **One folder, or nested?** SwiftPM stops at the target directory:
  subdirectories are the same module, not submodules. Matching that is
  free and avoids inventing submodules, which Swift lacks.

## 5. Receiver methods

Methods written outside the type's body, with the receiver named. In
Swift they are an **extension member** — that is the whole of it, and
saying so is what keeps the feature from growing a second method model
beside the one Swift has:

```swift
func (v: borrowing vec2) length() -> float32 { ... }
func (v: inout vec2) scale(k: float32)       { ... }
```

```swift
extension vec2 {
    func length() -> float32 { ... }
    mutating func scale(k: float32) { ... }
}
```

### Which types, and what the receiver means on each

Anywhere an extension goes, which is any nominal type. What the
ownership word means is not uniform across them, which is the reason
to write them out rather than say "it works on types":

```swift
func (v: borrowing vec2) length() -> float32 { ... }  // reads a value
func (v: inout vec2) scale(k: float32)       { ... }  // mutating func
func (b: borrowing Box) doubled() -> int32   { ... }  // reads through a reference
func (b: borrowing Box) bump()               { b.v += 1 }  // still borrowing
func (d: borrowing Dir) code() -> int32      { ... }  // reads a value
```

| Receiver kind | `borrowing` | `inout` | `consuming` |
| --- | --- | --- | --- |
| `struct`, `enum` | ordinary method | `mutating func` | `consuming func` |
| `class` | ordinary method | **no meaning** | `consuming func` |

The class row is the one worth stating. A class receiver is a
reference, so a method that changes a property changes the object
without the receiver itself being mutable — `bump()` is `borrowing`
and still assigns to `b.v`, which is how Swift behaves and how this
compiler already behaves for a class extension. Swift has no
`mutating` on a class method at all, and `inout` there would have to
mean rebinding the reference, which no Swift method can do. So an
`inout` receiver on a class is refused by name, not quietly treated as
`borrowing` — and that refusal has to be *written*, because the
compiler currently accepts `mutating func` inside a `class` where
Swift rejects it (`TODO.md`).

**Protocols are the fourth case and are blocked.** A receiver method
on a protocol would be a protocol extension — a default implementation
— and protocol extensions do not work here yet: a member declared in
one cannot see the protocol's requirements, and conforming types do
not gain it, so there is nothing to desugar into. When they arrive
they bring Swift's sharpest dispatch rule with them: a method in a
protocol extension that is not *also* a requirement is statically
dispatched, so a conforming type's own version is not called through
the protocol.

Verified today: extensions on `struct`, `class` and `enum` all work,
including a class extension assigning to a property with no `mutating`
anywhere. An extension on a protocol does not.

### What it inherits from being an extension member

None of this is a choice the language gets to make differently:

- **Statically dispatched**, and not in the type's table. A live
  distinction rather than a theoretical one — this compiler models
  vtables and an override through a base reference already dispatches
  dynamically — so a receiver method has to be kept *out* of the table
  on purpose.
- **No stored properties.** Extensions add methods, computed
  properties and initializers, never storage. A receiver method is a
  method, so the question does not arise; the rule is what stops the
  syntax being read as a second place to declare a field.
- **Reaches imported types**, since extensions do. What it cannot do
  is change a type's layout or its table, which is the first two
  points again.
- **May satisfy a conformance**, as an extension's method can.

### The one thing Swift does not have

The name. Swift's receiver is always `self`; here it is whatever the
declaration calls it. So the desugaring is an extension member whose
body has one extra name in scope, bound to the receiver — a scope
entry, not a parameter, since the method already receives `self` and
`v` is another way to say it rather than a second thing to pass.

### Where the compiler already is

Most of the way, which is why this is small:

- `resolveExtensions` resolves an extension's type and hands its
  members to `readMembers` with the type's own sinks, so an extension
  member is already an ordinary method of the type.
- `vil/gen` lowers a method as a function with the receiver as a
  parameter, called through a `function_ref`.
- `mangle` spells a method as a member of its type.

What is left: a receiver clause in the parser, building the
`ExtensionDecl` the rest of the pipeline already understands, and
binding the receiver's name in the body's scope.

### Staging: `borrowing` first

The type kind is not what makes this big — the parser cannot tell an
enum from a struct, `sinksOf` routes all three to the same method
sink, and enum is a value type behaving exactly as struct does, which
is why the table above has two rows and not three. Excluding a kind
would mean *adding* a check after type resolution, a diagnostic, and a
documented divergence from Swift, where extending an enum is ordinary.

The axis that shrinks it is ownership. A `borrowing` receiver needs
nothing that does not already work:

- a struct or enum method that reads — ordinary today
- a class method that reads, and one that *writes a property*, since
  the receiver is a reference and needs no mutability of its own

That covers most of the value and hits neither blocker below, because
both are about assigning through a mutable *value* receiver. So
`borrowing` can ship on the machinery that exists, and `inout` and
`consuming` follow once the two are fixed.

### What is in the way

Two gaps an `inout` receiver would hit immediately, both of which
`mutating` hits today. They are `TODO.md` items and should be fixed
first, or a receiver method built on top will look broken for reasons
that are not its own:

- Assignment to an implicit-`self` property in a `mutating` method is
  rejected, with a message saying to declare it `mutating` — which it
  is. Reading a bare `x` works; assigning to one does not.
- Assignment through explicit `self` is not lowered: `self.x = self.x
  * k` typechecks, then `cannot lower an assignment to this
  expression yet`.

### Still open

- **Where may one be written?** Swift lets an extension sit anywhere
  in the module, and matching that is simplest. Same-file-as-the-type
  would be a smaller feature and a different one, and would not serve
  the flatter layout this is for.
- **`self` as well as the name?** Allowing both keeps a mixed file
  readable. Allowing only the name is stricter, and makes a receiver
  method impossible to move into a type body unedited.
- **Generic receivers.** `func (s: borrowing Stack<T>) peek() -> T`
  needs its generic parameters bound, which `extension Stack` does
  implicitly. Worth settling with the syntax rather than after it.
- **Does the receiver clause admit a label?** It reads as a parameter
  and is not one. Label-free — a name, a colon, an ownership word, a
  type — is the narrower grammar and cannot be confused with the
  parameter list that follows.
