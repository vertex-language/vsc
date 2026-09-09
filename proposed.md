# Proposed

What the Vertex additions should do, and what has to change in the
compiler to make them do it. One section each, ordered by how much
design is left rather than by how much code.

Additions only. Swift the compiler does not do yet is `TODO.md`.

| Addition | State | Where it lives |
| --- | --- | --- |
| Lowercase primitives | **done** | `types/universe.go` |
| `kernel` / `graph` | **done** | `parser/decl.go`, refused in `vil/gen` |
| Optional argument labels | **done** | `analyzer/call.go` |
| Packages and folder imports | **done** | `vsc.go` importer, driver in `internal/cli` |
| Receiver methods | not built | `parser`, desugaring to an extension |

The first four are implemented. What follows is the design and the reasoning
behind it, kept because the reasoning is what the next change to any
of them has to agree with; where the built behaviour is narrower than
the design, the section says so.

Two through-lines. A module's identity is a single identifier, because
that is what a symbol is mangled with — every proposal either respects
that or has to say what replaces it. And Swift's semantics stay
Swift's: what is on the table is grammar and where files are found,
not a second model of what a module is.

## 1. Lowercase primitives

Already in, and worth writing down because it sets the pattern the
others should follow.

`types/universe.go` keeps two tables. `swiftTypes` is Swift's
universe; `vertexTypes` is the lowercase spellings, each mapped to the
same `Type` value as its capitalised counterpart. `LookupUniverse`
reads the first, then the second, so a program that writes only
Swift's names sees exactly Swift's universe — and
`LookupSwiftUniverse` exists for the places that need that view on
purpose.

The pattern: **an addition is an alias onto an existing thing, not a
parallel implementation of it.** `int32` and `Int32` are one
`types.Basic`, so nothing downstream — layout, mangling, lowering —
ever learns there were two spellings. Cost of the feature after the
table: zero.

## 2. `kernel` and `graph`

Reserved modifiers for a data-parallel unit and a graph node. The
backends do not exist, so the only question is what the compiler does
when it meets one.

```swift
func add() kernel -> float32 { }
```

**Parsing.** A modifier between the parameter list and the return
arrow. Nothing else in the grammar sits there, so it costs one
lookahead in the function-signature parser and a field on the AST
node. No ambiguity with Swift: `kernel` and `graph` are contextual —
a function or variable may still be called either one.

**What happens next is the actual decision**, and "do nothing" splits
into two options that behave very differently:

- *Parse and ignore.* The function compiles as an ordinary one. It
  runs on the CPU and returns a right-looking answer, so nobody
  discovers the modifier did nothing until they benchmark it. This is
  the option that lies.
- *Parse and refuse to lower.* The signature typechecks, the body
  typechecks, and `vil/gen` reports `kernel functions are not lowered
  yet` at the declaration. The program does not build, which is the
  truth.

Recommend the second. A reserved word that silently compiles to
something else is worse than one that does not compile at all — and
the corpus rule already says this: a file lands in `tests/compiler/`
once it works, and a refusal fails the suite rather than being
skipped. A `kernel` that quietly became a normal function would pass
that suite while being wrong.

Cost: a parser field, one diagnostic, no backend work.

## 3. Optional argument labels

Labels are carried for compatibility, so writing them is optional in
both directions: `_` on a parameter may be left off, and so may the
label at the call.

```swift
func addUp(a: int32, b: int32) -> int32 { return a + b }

addUp(1, 2)         // no labels
addUp(a: 1, b: 2)   // labels
```

**Where the enforcement lives.** `analyzer/call.go`, five places:

| Site | Does |
| --- | --- |
| `labelFits` (106) | overload resolution — which declaration a name means |
| the check loop (254) | reports `incorrect argument label` |
| `matchByLabel` (274) | short argument lists, defaults skipped |
| `labels` (312) | pairs arguments with parameters |
| `argLabelFits` (505) | the same question, for variadics |

Four of the five are the same predicate written for different callers.
The change is to make the predicate accept a missing label, and to
leave the fifth — overload resolution — able to fail.

**What shipped.** Strict first, lax as a fallback: a call whose labels
are written the way the declaration asks resolves exactly as it always
did, and only a call the strict rule rejects is tried again with
labels optional. No program that compiled before can reach the second
pass, so the change cannot alter an existing meaning. An unlabelled
call matching more than one overload is
`ambiguous use of 'label': 'a:', 'b:'`.

The lax rule is confined to calls whose argument count matches the
parameter count -- there, every argument is positional and a label
decorates rather than decides. A short list skipping defaults, and the
arguments of a variadic, still require their labels, for the reason
below.

**Variadics need more than a lax predicate.** `variadicParams` uses
the label to decide where the variadic list *stops*: it takes every
argument written without a label, and a labelled argument after that
belongs to the parameter of that name. Make labels optional and an
unlabelled argument after the list is ambiguous — it could be another
element or the next parameter. Swift avoids this by requiring the
label. So either the trailing parameters after a variadic keep
requiring theirs, or the list has to stop being greedy. The first is
smaller and easier to explain; it means "labels are optional" has one
footnote.

**The one case that cannot be made lax.** `tests/check/ok-34-overloads.swift`
holds this:

```swift
func label(a: Int) -> Int { return a }
func label(b: Int) -> Int { return b }
```

Two functions, differing in nothing but the label, mangling to two
different symbols. `label(1)` names neither. So the rule is: omitting
a label is allowed wherever it leaves exactly one candidate, and is an
error naming the candidates where it does not. Labels stop being
required and become the way to disambiguate — which is what they are
for anyway.

That also keeps the compatibility story intact: a `swiftc`-built
library may export two functions that differ only by label, and the
call has to be able to reach both.

## 4. Packages and folder imports

The interesting one, and the rule it is built on is worth stating
before the details:

> **Add grammar. Reuse Swift's semantics.** Nothing here introduces a
> second model of what a module is, what a namespace is, or how a name
> is resolved. Swift already has answers and this compiler already
> implements them. What is missing is a way to *write* the thing in
> source, which is grammar.

Go is a reference where its solution to a shared problem is
instructive, and nothing more. It is not the model.

```swift
import "std/fmt"

func main() -> int32 {
    fmt.Sprintf()
    return 0
}
```

Fetching stays out of scope. A package manager downloads a repository
into the global package directory, and what it leaves behind is a
folder of `.vs` files like any other. The language only has to know
what a folder means and where to look for one.

### The new grammar

All of it, in one place. Three forms, none of which changes the
meaning of anything already valid.

**A string import.** Today an import is followed by a dotted
identifier path. This adds a string literal, which names a folder:

```swift
import Foundation      // unchanged: a module, found the usual way
import "std/fmt"       // new: a folder, found in the package directory
import "./geometry"    // new: a folder, relative to this file
```

The two forms are distinguished at the token, so the parser knows
which it has before it resolves anything, and no existing import is
re-read. What binds is the last segment — `std/fmt` provides `fmt` —
which is the folder-name rule below, since the last segment *is* the
folder's name.

**The group form.** Punctuation only — a parenthesised list standing
for one `import` each:

```swift
import (
    "std/fmt"
    "./geometry"
)
```

**A rename.** An identifier before the string binds that name instead
of the last segment:

```swift
import acmefmt "acme/fmt"
```

Not decoration: a package directory makes last-segment collisions
ordinary, since `fmt`, `json` and `http` are names many packages end
in. Without a rename, two such imports in one file cannot both be
named. The form is reachable only through the string import, so no
Swift file can encounter it.

**A `package` clause**, naming the module a file belongs to:

```swift
package geometry
```

`package` is already a Swift access level — 5.9 took it, `package func
f()` is valid, and `parser/attr.go:111` lists it as a modifier. That
is why `package geometry` currently fails with `consecutive statements
on a line must be separated by ';'`: the parser reads a modifier and
then a stray identifier.

That is a parsing problem, not a reason to avoid the word. A
contextual keyword means what its position says it means, and the two
positions never look alike:

> `package` followed by an identifier that does not begin a
> declaration is the clause. Followed by a declaration keyword or
> another modifier, it is the access level.

Both stay, nothing new is reserved, and no valid Swift file changes
meaning. The cost is one lookahead in the declaration parser.

### What stays Swift

Everything else, and the reason the grammar is all that is needed:
Swift's namespace model is already the right one and is already
implemented here.

Swift has exactly one namespace, and it is the **module**. There is no
`namespace` keyword and never has been. What it has instead:

- **Modules.** `Foundation.Data` and `Units.Metric` are qualified
  names, and `import Units` puts `Metric` in scope unqualified. This
  compiler models it today — `c.modules[name]` is a scope per module,
  `recordModule` fills it, and `isModuleRef` decides that a bare
  `Units` in expression position is a module and not a variable.
- **Caseless enums**, the in-language substitute: `enum Math { static
  func f() }` is a namespace that cannot be instantiated, idiomatic
  precisely because Swift offers nothing better.
- **Nested types**, which scope names but only inside a type.
- **No submodules.** `import Foo.Bar` works for Clang modules, not
  Swift ones. It has been asked for for a decade.

And the part that decides this proposal: **a Swift file does not say
what module it belongs to.** The build system does, with
`-module-name`. A module is "the files compiled together" — a fact
about an invocation rather than about the source.

So a folder import needs no new resolution rule, no new scope kind and
no new qualified-name syntax. It needs a way to say which files are
compiled together, which is the one thing Swift never put in the
language.

### The folder convention already exists

SwiftPM has had folder-based modules the whole time. A target is a
directory under `Sources/`, every file in it is that module, and the
directory's name *is* the module name:

```
Sources/
  Geometry/       → module Geometry
    area.swift
    point.swift
```

The concept is not foreign to Swift at all. It lives in the package
manager and the manifest rather than in the language, with
`-module-name` as the seam between them.

That is what the proposal actually is: **teach the compiler the folder
convention the ecosystem already follows**, so a program can name a
directory directly instead of routing the fact through a manifest and
a build system.

### Naming a module

Two candidates, and the SwiftPM convention argues for the first.

**The folder name**, which is a path's last segment: `"std/fmt"` binds
`fmt` and `"./geometry"` binds `geometry`. Nothing is declared
anywhere, matching what SwiftPM already does and what `-module-name`
already receives, and leaving a source file silent about its module —
the existing Swift rule rather than a break from it.

**The `package` clause.** `package geometry` at the top of the file,
with a folder whose files declare nothing being an error.

The first is less to write and cannot fall out of sync, since a folder
rename has no declaration to disagree with. The second is explicit and
survives a rename.

The middle is probably right: **the folder name is the default, and a
clause overrides it.** The clause is then optional in the ordinary
case, there is a single source of truth when it is absent, and there
is an escape hatch when a folder name is not a usable identifier.

Because it must be one. `mangle.module` writes the name
length-prefixed into every symbol the module defines, so `my-lib` is a
fine directory and not a possible module. That is the one hard
constraint the language does not get to relax, and the clause is how a
directory like that still gets imported.

### Casing

A module name may be upper or lower case, and this is already true —
not a thing to add.

The proof is the compiler's own default: the entry module is `main`,
lowercase, and it is in every symbol of every program built so far.
`mangle.module` writes a name length-prefixed with its case intact, so
the two spellings differ only where you would expect:

```
$s8geometry4areays5Int32VAD_ADtF     -module geometry
$s8Geometry4areays5Int32VAD_ADtF     -module Geometry
```

Both resolve, both typecheck through a qualified call, and both mangle
into linkable symbols. So `package geometry` and `package Geometry`
can both be legal with no work in the mangler and none in the checker.

One consequence worth stating rather than discovering. Modules and
values share a namespace at the point of use: `isModuleRef` treats a
bare name as a module reference only when the local scope does not
already have it, so a local declaration shadows a module of the same
name. That is the intended rule, and it is silent:

```swift
import geometry

func main() -> int32 {
    let geometry: int32 = 7    // shadows the module; checks clean
    return geometry
}
```

Lowercase module names make that collision much more likely, because
lowercase is also what locals and parameters look like. Swift's
convention of capitalising modules is not arbitrary — it is what keeps
the two populations apart. Allowing both spellings is free; the
question is whether a shadowed module should stay silent, and a
warning when a local shadows an imported module would cost little.

### Where a path resolves

Two kinds of path, told apart by prefix, in the way Go tells a module
path from nothing at all:

| Written | Found in |
| --- | --- |
| `"std/fmt"`, `"acme/geometry"` | the global package directory |
| `"./geometry"`, `"../shared"` | relative to the importing file |

The first is the ordinary case. The package directory is one tree of
folders, it is where a package manager writes what it downloads, and a
program names a folder in it without caring how it arrived. Since a
downloaded repository is just a folder once it has landed, fetching
never becomes a language feature.

The second is for local work and for tests — a folder beside the
program that no package manager knows about. It is deliberately the
marked form: `./` says "not from the package directory", which is the
only thing the compiler needs to tell them apart.

**The root has to be configurable.** An environment variable, and a
flag that overrides it. The reason is immediate rather than
theoretical: the test corpora need to point at a directory they
populated themselves, and a build that reads a developer's home
directory is not reproducible. Go learned this twice, and the second
answer — a cache location that tests and CI can redirect — is the one
worth copying.

**Relation to `-I`.** They are not the same search path and should not
be conflated yet. `-I` finds *built* modules: a `.vertexinterface` or a
`.swiftinterface` for something already compiled, which is how a
`swiftc`-built library is reached. The package directory holds
*source* folders. A bare `import Foundation` keeps using `-I`; a
string path uses the package directory or a relative path. Whether the
two eventually merge is worth leaving open rather than deciding now.

**What lives under `std/`?** A real question, and separable from this
one. Vertex has no standard library of its own — `String`, `Array` and
Foundation come from Swift's, linked against what this compiler
produced, and `tests/interop/` is what holds that together. A `std/`
tree would be Vertex's own library code sitting beside that, and
deciding what belongs in it is a larger question than deciding how an
import finds a folder. The import mechanism should not wait on it, and
does not have to: `"std/fmt"` is an ordinary path under the package
directory, and nothing in the resolution rules knows that `std` is
special.

### What an imported folder *is*

The sharper question, and the one that decides how much work this is.
An import today names a module that has already been built: the
interface says what is in it, and its object is linked in separately.
A folder of source has not been built by anyone yet. Two readings:

**(a) Declarations only.** The folder's `.vs` files are parsed and
checked for what they declare, bodies skipped — which is exactly what
`loadImports` already does to an interface, since an interface *is*
source with the bodies removed. `analyzer.Import` is `{Name, Files,
Units}`, so a folder's parsed files drop straight into it. Building
and linking the module stays somebody else's job.

Cost: a glob and a loop in the importer. Nothing below it changes at
all.

Gap: importing a folder and then having to build it separately anyway
is most of the inconvenience the feature was meant to remove.

**(b) Built and linked.** `vsc build main.vs` finds the imported
folders, compiles each as its own module, and links the objects
together.

This sounds much larger than it is. The pieces exist: `Compile` takes
an `Options.Module`, so compiling N modules is N calls with N names;
`build.Object` turns each `Unit` into an object; and
`build.Executable` already takes a **slice** of objects rather than
one. What is missing is only the orchestration — walk the import
graph, order it, compile each folder once, link the results — and none
of that lives inside the compiler. It belongs in `internal/cli`, or in
a small package beside it.

So (b) needs no change to `analyzer`, `vil/gen`, `lower`, `mangle` or
`build`. It needs a driver they already support.

**What shipped: both.** (a) is the importer reading a folder's `.vs`
files and handing them to the checker as `analyzer.Import`, exactly as
it hands over an interface. (b) is `Unit.Packages` -- the folders a
compilation imported, in dependency order -- and a loop in
`internal/cli/build.go` that compiles each with its own module name
and adds its object to the link. The prediction held: nothing in
`analyzer`, `vil/gen`, `lower`, `mangle` or `build` changed for it.

### What changes, and what does not

| Changes | Does not change |
| --- | --- |
| the parser — string, group and rename forms of `import` | `analyzer.Import`, still `{Name, Files, Units}` |
| `importer.readAll` / `findInterface` — a folder is a candidate | `c.modules`, `recordModule`, `isModuleRef` |
| a package-directory root, and a way to override it | `mangle` — it never sees a path |
| optionally a `package` clause and its disambiguation | `vil/gen`, `lower`, `build` themselves |
| for (b), a driver that compiles and links a graph | how a qualified name resolves, at all |

The right-hand column is Swift's semantics, untouched. Everything on
the left is either syntax or bookkeeping about where files came from.

### Still open

- **A rename does not fix a symbol collision.** `import acmefmt
  "acme/fmt"` changes what the importing file calls the module. It
  does not change what the module's own symbols are mangled with,
  which is still `fmt` — so linking `std/fmt` and `acme/fmt` into one
  program is a duplicate-symbol error, rename or no rename. The
  `package` clause is the actual escape hatch, because it changes the
  identity rather than the reference. Worth stating plainly, since the
  rename form looks like it solves this and does not. (Go avoids the
  whole problem by putting the path in the symbol. Not available here:
  the mangling is fixed and shared with `swiftc`.)
- **Source or builds in the package directory?** Source is assumed
  above, which means a program builds its dependencies. Caching
  objects beside the source would avoid that, and is exactly the point
  at which something has to decide what is stale — a build system's
  job, and where this stops being a language question.
- **One folder, or nested?** SwiftPM stops at the target directory:
  subdirectories are the same module, not submodules. Matching that is
  free and avoids inventing submodules, which Swift still does not
  have.

## 5. Receiver methods

Methods written outside the type's body, with the receiver named:

```swift
struct vec2 {
    var x: float32
    var y: float32
}

func (v: borrowing vec2) length() -> float32 {
    return (v.x * v.x + v.y * v.y).squareRoot()
}

func (v: inout vec2) scale(k: float32) {
    v.x *= k
    v.y *= k
}
```

### What they are in Swift

An extension member. That is the whole of it, and saying so is what
keeps the feature from growing a second method model beside the one
Swift has:

```swift
extension vec2 {
    func length() -> float32 { ... }
    mutating func scale(k: float32) { ... }
}
```

The receiver's ownership is what Swift spells on the method:

| Receiver | Swift |
| --- | --- |
| `borrowing` | an ordinary method — `self` is borrowed, which is the default |
| `inout` | `mutating func` |
| `consuming` | `consuming func` |

### Which types, and what the receiver means on each

Anywhere an extension goes, which is any nominal type. The receiver's
ownership means what it means for that kind of type, and that is not
uniform -- which is the reason to write the three out rather than say
"it works on types":

```swift
struct vec2 { var x: float32; var y: float32 }
class  Box  { var v: int32 }
enum   Dir  { case up, down }

func (v: borrowing vec2) length() -> float32 { ... }   // reads a value
func (v: inout vec2) scale(k: float32)       { ... }   // mutating func
func (b: borrowing Box) doubled() -> int32   { ... }   // reads through a reference
func (b: borrowing Box) bump()               { b.v += 1 }  // still borrowing
func (d: borrowing Dir) code() -> int32      { ... }   // reads a value
```

| Receiver kind | `borrowing` | `inout` | `consuming` |
| --- | --- | --- | --- |
| `struct`, `enum` | ordinary method | `mutating func` | `consuming func` |
| `class` | ordinary method | **no meaning** | `consuming func` |

The class row is the one worth stating. A class receiver is a
reference, so a method that changes a property changes the object
without the receiver itself being mutable -- `bump()` above is
`borrowing` and still assigns to `b.v`, which is exactly how Swift
behaves and how this compiler already behaves for a class extension.
Swift has no `mutating` on a class method at all; `inout` there would
have to mean rebinding the reference, which no Swift method can do. So
an `inout` receiver on a class should be refused, and refused by name
rather than quietly treated as `borrowing`.

That refusal has to be written, not inherited: this compiler currently
accepts `mutating func` inside a `class`, which Swift rejects, so
there is no existing check to lean on. See `TODO.md`.

**Protocols are the fourth case and are blocked.** A receiver method
on a protocol is a protocol extension -- a default implementation --
and protocol extensions do not work here yet: a member declared in one
cannot see the protocol's own requirements, and conforming types do
not gain it. Until they do, a receiver method on a protocol has
nothing to desugar into. When they arrive it brings Swift's sharpest
dispatch rule with it: a method in a protocol extension that is *not*
also a requirement is statically dispatched, so the conforming type's
own version is not called through the protocol.

Verified as it stands today: extensions on `struct`, `class` and
`enum` all work, including a class extension assigning to a property
with no `mutating` anywhere; an extension on a protocol does not.

Everything else follows from being an extension member, and none of it
is a choice this language gets to make differently:

- **Statically dispatched.** A method in a Swift extension is not in
  the type's table and cannot be overridden. That is a live
  distinction here rather than a theoretical one -- this compiler
  models vtables, and an override through a base reference already
  dispatches dynamically -- so a receiver method has to be kept *out*
  of the table on purpose.
- **No stored properties.** An extension may add methods, computed
  properties and initializers, and may not add storage. A receiver
  method is a method, so the question does not arise, but the rule is
  what stops the syntax from being read as "a second place to declare
  a field".
- **Any type, including an imported one.** Extensions reach types from
  other modules, so receiver methods do too. What they cannot do is
  change the type's layout or its table, which is the same sentence
  as the first two points.
- **May satisfy a conformance.** Swift lets an extension's method
  fulfil a protocol requirement, and there is no reason for this to
  differ.

### The one thing Swift does not have

The name. Swift's receiver is always `self`; here it is whatever the
declaration calls it, and `self` should keep working beside it for a
file that mixes the two forms.

So the desugaring is: an extension member whose body has one extra
name in scope, bound to the receiver. That is a scope entry, not a
parameter -- the method already receives `self`, and `v` is another
way to say it rather than a second thing to pass.

### Where the compiler already is

Most of it, which is why this is small:

- `resolveExtensions` in `analyzer/decl.go` resolves an extension's
  type and hands its members to `readMembers` with the type's own
  sinks, so an extension member is already an ordinary method of the
  type.
- `vil/gen` already lowers a method as a function with the receiver as
  a parameter, called through a `function_ref`.
- `mangle` already spells a method as a member of its type.

The work is the parser -- a receiver clause between `func` and the
name -- plus building the `ExtensionDecl` the rest of the pipeline
already understands, and binding the receiver's name in the body's
scope.

### What is in the way

Two gaps that a receiver method with an `inout` receiver would hit
immediately, and that `mutating` already hits today. Both belong in
`TODO.md` rather than here, and both should be fixed first, because a
receiver method built on top of them would look broken for reasons
that are not its own:

- Assignment to an implicit-`self` property in a `mutating` method is
  rejected: `mutating func scale(_ k: int32) { x = x * k }` reports
  that the method has to be declared `mutating`, which it is. Reading
  a bare `x` works; assigning to one does not.
- Assignment through explicit `self` is not lowered: `self.x = self.x
  * k` typechecks and then reports `cannot lower an assignment to
  this expression yet`.

### Still open

- **Where may one be written?** Swift lets an extension sit anywhere
  in the module. Matching that is simplest. Requiring the same file as
  the type would be a smaller feature but a different one, and would
  not match the "flatter source layout in large packages" this is for.
- **`self` as well as the name?** Allowing both is one scope entry
  more and keeps a mixed file readable. Allowing only the name is
  stricter and makes a receiver method impossible to move into a type
  body unedited.
- **Generic receivers.** `func (s: borrowing Stack<T>) peek() -> T`
  needs the generic parameters bound, which is what
  `extension Stack { ... }` does implicitly. Worth settling with the
  syntax rather than after it.
- **Does the receiver clause admit a label?** `func (v: borrowing
  vec2)` reads as a parameter and is not one. Keeping it label-free --
  a name, a colon, an ownership word and a type -- is the narrower
  grammar and the one that cannot be confused with the parameter list
  that follows.
