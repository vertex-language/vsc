# Where the grammar stops and Swift keeps going

`docs/swift_grammar.md` is the published Swift grammar. It is the
specification this front end follows, and it is not the whole
language: the reference book is edited by hand and Swift moves, so
there are productions Swift accepts that the grammar does not spell
out, and productions it spells out loosely enough that only the
compiler settles them.

Swift is the core dialect, so this file is about Swift and nothing
else. What Vertex adds — the lowercase type names, package
declarations, receivers, `kernel` and `graph` — is optional and sits
on top; none of it appears below, because none of it changes what a
Swift program means.

Everything below is a place this parser reads more, or reads
differently, than `docs/swift_grammar.md` says. Each entry names what
the grammar has, what Swift has, and how that was established — in
almost every case by asking `swiftc` (`swiftc -frontend -parse`) or by
reading Apple's own `.swiftinterface` files, which are Swift source
the compiler both writes and reads back.

Two tests keep this file honest, in `oracle_test.go`:

- `TestSDKInterfaces` parses every module interface in every installed
  SDK — about 4,600 files and 50MB of Swift — and requires that none
  of them produces a diagnostic.
- `TestSwiftcAgreement` runs the corpus in `tests/` and a table of
  malformed sources past both parsers and compares the verdicts.

Both skip themselves where no toolchain is installed.

## Declarations

**An import takes an access level.** The grammar writes `attributes?
import ImportKind? ImportPath`. Swift 6 admits an access-level
modifier — `internal import Darwin` says the module is not
re-exported to a client (SE-0409) — so `ImportDecl` carries `Mods` and
the parser refuses only the modifiers that are not access levels.

**A parameter's name may be a keyword.** The grammar's
`ArgumentLabel` is an identifier. Swift admits any word but `inout` in
both name positions, which is how `func f(_ default: Value)` is
written — and the SDK is full of it. `atParamLabel` is therefore the
same test for the label and for the local name.

**A declaration's parameter carries what a function type's cannot.**
One `Parameter` production serves both, so `Param` holds the local
name of `func move(to dest: Point)` and a default value; the parser
fills them only where a declaration is being read, and reports a
default written in a function type.

**Modifiers the grammar's list leaves out.** `indirect enum`,
`borrowing func`, `consuming func`, `async let`, and `distributed
actor` are modifiers like any other. So are the underscored SPI
spellings every SDK interface is written with: `__consuming`,
`__owned`, `__shared`, `_const`, `_local`.

**Accessors the grammar leaves out.** It has `get`, `set`, `willSet`
and `didSet`. A module interface is written with `_read`, `_modify`,
`unsafeAddress` and `unsafeMutableAddress` as well, and `yield` is a
statement inside the first two.

**A typealias takes a where clause.** `typealias CountableRange<Bound>
= Range<Bound> where Bound: Strideable` — the grammar gives the clause
to every other generic declaration and not to this one.

**An enum case's associated values are a parameter list.** The grammar
writes a tuple pattern. `case point(x: Int, y: Int = 0)` declares two
values, with labels and defaults, which a pattern cannot hold.

**A protocol declares primary associated types.** `protocol
Collection<Element>` names which associated types may be written as
generic arguments. The grammar has no such clause.

**An inheritance item takes attributes and `nonisolated`.**
`: @retroactive Equatable`, `: @unchecked Sendable`, and — since Swift
6.2 — `extension S: nonisolated P`, whose conformance is not tied to
an actor.

**A generic parameter may be a value or be named `Self`.**
`InlineArray<let count: Int, Element>` declares a parameter whose
argument is a number rather than a type (SE-0452), and
`func f<Self>(…) where Self: P` is a parameter that shadows nothing,
which the SDK's UIKit interface uses.

## Types

**`~` is a type.** The grammar attaches a suppressed conformance to
one constraint. Swift lets it be one member of a composition, so
`~Copyable & ~Escapable` suppresses two, and an `InverseType` may
appear anywhere a type does — including inside `any (~Copyable &
~Escapable).Type`.

**`_` is a type.** A placeholder: `Array<_>` and `[_]` ask the
analyzer which type belongs there (SE-0315).

**An integer is a type.** The argument of a value generic parameter:
`InlineArray<3, Int>`, and `A<-1>` with the sign.

**`sending` and `nonisolated(nonsending)` qualify a type.** Neither is
in the grammar. The first says a value is handed over rather than
shared; the second says an async function runs on its caller's
executor (SE-0461), and `_Concurrency`'s own interface is written with
it.

## Expressions

**`Self` and `Any` name types in expression position.** The grammar's
`PrimaryExpression` has neither, and both are ordinary there:
`Self("x")` calls an initializer, `Any.self` names a metatype.

**A type may be written where an expression goes.** Most types are
already expressions and are read as such — `[Int].self` is an array
literal until the analyzer knows better. Three spellings are types and
nothing else, and reach the analyzer as a `TypeExpr`: `any P`,
`some P`, and a type written with attributes, as in
`(@convention(c) (Int) -> Int).self`. Both forms appear in the
stdlib's own interface.

**`if` and `switch` are expressions.** SE-0380. Which positions may
hold one is a rule about the value, so the parser reads one wherever
an expression may begin, as Swift's does, and leaves the rest to the
analyzer.

**A tuple index arrives inside a number.** The scanner reads the `0.0`
of `t.0.0` as one float literal, because that is what it is made of.
The member and key-path readers split it back into two indices — the
same thing Swift's parser does at the same point.

**A name may be written with its argument labels.**
`handle(_:)`, `move(to:from:)`, `String.init(describing:)` and the
`#selector(tapped(_:))` that every AppKit and UIKit target is
registered with name one declaration among the overloads rather than
calling it. The grammar gives `ArgumentNames` to a member access
only; Swift admits it after a bare name and after `init` as well.

**An attribute that takes no arguments does not open a clause.**
`@escaping() -> Void` is an attribute and a function type whose
parameters are empty — a spelling Xcode's own templates use. The
grammar writes `BalancedTokens` after every attribute name, so only
knowing the attribute's arity settles it; `bareAttrs` is the half of
Swift's table the difference turns on.

**An effect operator takes the whole of what follows it.** Not only
after `=`: `count += try predicate(e) ? 1 : 0` is one `try` over the
rest of the sequence, and the stdlib is written that way.

**`repeat` opens a loop or a pack expansion.** `repeat { … } while c`
is the statement; `repeat each x` is an expression, and may stand as a
statement of its own. The brace is what tells them apart.

**A closure's parameter takes two names and a modifier.**
`{ (_ fn: () -> Void) in … }` gives it a label as well as a name, and
`{ (state: inout (Int, Bool)) in … }` a parameter modifier, which the
grammar's `ClosureParameter` has no room for.

**A brace that opens an accessor block is not a trailing closure.**
`var x = 0 { didSet { … } }` is a binding with an observer. Swift's
own parser makes this test in the same place.

**An accessor block may open on the line below.** `var count: Int`
and then `{ return 3 }` under it is a computed property, not a
declaration followed by a closure. Where a line break separates them,
only a brace that opens accessors — or the getter of a binding with
no initializer — belongs to the declaration.

## Statements

**A `#if` in a switch body may hold case labels.** When the directive's
first line is followed by `case`, `default`, or `@unknown default`, the
case being read ends there and the switch reads the whole directive as
a conditional case.

**The compilation conditions the grammar lists are not all of them.**
Swift answers `hasFeature`, `_runtime`, `_endian`, `_pointerBitWidth`,
`_hasAtomicBitWidth`, `_ptrauth` and `_compiler_version` as well, and
`canImport` takes a version: `canImport(Cxx, _version: 6.2.0.9)`.

## Lexis

These belong to `scanner`, and are listed here because they are the
same kind of departure.

**Identifiers are ranges, not Unicode categories.** The grammar says
the letter categories. Swift names ranges, and they are not the same
set: `let 😀 = 1` is a Swift program because U+1F600 falls in
U+10000–U+1FFFD, while U+00A9 © is not an identifier character though
it is a symbol. `scanner/ident.go` holds the ranges, and
`TestIdentCodePoints` holds their boundaries — every row read back
from `swiftc`.

**A backtick has a second job.** It escapes a keyword, and since Swift
6.2 it also introduces a raw identifier, whose contents need not be an
identifier at all: `` `hello world` ``, `` `f(x)` ``, `` `123` ``
(SE-0451). What it may not hold is a backslash, a line break, or
whitespace other than the space; what it may not be is empty, all
spaces, or a name made only of operator characters.

**A unicode escape names a scalar.** `"\u{110000}"` and `"\u{D800}"`
are not scalars, and Swift rejects both.
