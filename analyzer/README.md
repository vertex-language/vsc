# What the checker knows

The parser's job is settled: a program either is Swift or is not, and
`swiftc` says which. The checker's job is not, because a checker is
never finished — it knows some of the language and not the rest, and
the only dangerous thing it can do is fail to know the difference.

So the rule this package is built on:

> Where the checker does not know, it says nothing. It never invents
> an answer.

An invented type is worse than no type. A parser that rejects valid
Swift fails loudly and someone fixes it; a checker that answers `Int`
when it does not know hands the next phase a well-typed module
describing a program nobody wrote. Everything below follows from
preferring silence to that.

## What it checks

- **Names and scopes.** Declaration before use, redeclaration in one
  scope, and the shape Swift gives its scopes: a body may shadow a
  parameter, a condition's binding belongs to the body it guards, and
  a `guard`'s binding outlives the guard but does not reach its else.
- **Types of expressions**, for what is modelled: literals, operators
  Swift declares itself, member access, calls, initializers, `if` and
  `switch` as expressions, string interpolation.
- **Members.** A member that a declared type does not have is
  reported. A member of a type whose members are not modelled is not,
  because not knowing is not evidence.
- **Conformance**, nominally: a type conforms to a protocol because it
  said so, never because its members line up.
- **Generic calls**, by unifying the arguments with the parameters:
  `identity(3)` is a call to `(Int) -> Int`, and `Box(v: 3)` makes a
  `Box<Int>`.
- **Literal values.** Every literal is decoded — integers in four
  bases, hexadecimal floats, escapes, pound delimiters, multiline
  indentation — and the value is recorded in `Info.Values`. This is
  the last step between a spelling and a constant a backend can emit.
- **Mutability, definite initialization, consumption**, and the two
  actor-isolation rules the model can express.

## What it does not know, and stays quiet about

- **The standard library.** There is none. `Int` has no members, `+`
  is not declared anywhere, and `Array` and `String` are modelled as
  types with no surface. A member of one is Invalid, silently.
- **Overload resolution**, beyond a fit. A name may be declared more
  than once, and a call takes the one declaration whose parameters its
  arguments fit. Swift ranks the candidates and picks the best; this
  requires exactly one to fit, and where several do or none does it
  says nothing rather than choosing.
- **Protocol requirements**, beyond the presence of a declared
  conformance. Whether a conforming type actually satisfies what it
  promised is checked only in the one pass that reports it.
- **Constraint solving.** Generic inference is a structural match of
  arguments against parameters. There is no bidirectional inference,
  no literal defaulting through constraints, no `where` clauses.
- **Regular expressions, key paths, macros, property wrappers, result
  builders, packs.** They parse; they have no type here.

Each of these is a place the checker returns Invalid without a
diagnostic. `typeErrorf` is how that stays quiet: a diagnostic whose
subject is Invalid is not reported, so one mistake is reported once
and nothing downstream of it is guessed at.

## Every expression, once

A checker that never reaches a piece of a program is worse than one
that reaches it and gives up: nothing downstream can be built from a
region that was never looked at. So every declaration that holds a
body is walked — a computed property's accessors, an initializer, a
deinitializer, a subscript, an observer, a nested type — and so is
every statement that holds one: `do` and its catches, `defer`, a
labelled loop, the branches of a `#if`.

`TestEveryExpressionIsVisited` is the invariant. Over the accepting
corpus, every expression that is not an operator's own symbol has a
recorded type; about 95% of them have a real one, and the rest are
Invalid for the reasons above.

## The oracle

`tests/check` holds programs written inside what is modelled, named
for the verdict they carry: `ok-` is a program Swift accepts and this
checker must find nothing wrong with, `bad-` is one Swift rejects and
this checker must reject too. `oracle_test.go` runs both halves, and
`TestCheckAgreesWithSwiftc` puts every one of them past `swiftc
-typecheck` so a program cannot be filed under the wrong verdict.

The first half is the one that matters. A checker that reports what it
does not understand is worse than one that stays quiet, because every
wrong diagnostic is a correct program the compiler refuses to build.
