# mangle

Swift's symbol mangling, cloned.

A compiler has to turn `func fib(_ n: Int) -> Int` into one name a linker can
hold, and that name has to encode enough of the declaration that two functions
differing only in their types get different symbols. Swift's answer is
`$s3vsc3fibyS2iF`, and this package produces that answer rather than one of its
own — so a program built here can be called by, and can call into, something
built by swiftc, and a symbol in a crash log demangles with the tools that
already exist.

## The shape

```
$s   3vsc    3fib   y        S2i     F
     module  name   labels   types   a function
```

Types are written result-first and parameters-second, which is the opposite of
how a demangler prints them.

A few rules are worth stating because they are not guessable:

- **A list marks where it begins rather than separating what is in it.** The
  tuple `(Int, Bool, Int)` is `Si_SbSit` — one underscore, after the first
  element.
- **A lone unlabelled parameter is written as its own type**, not as a tuple of
  one. A labelled one keeps the tuple, because `(a: Int)` is not `Int`. A
  parameter of type `Void` keeps it too, because collapsing `(())` to `()`
  would turn a function of one argument into a function of none.
- **A `Void` result is `y`, the empty list**, while `Void` written anywhere else
  is `yt`, the empty tuple. It is the one place the two spellings of nothing
  are not interchangeable.
- **`Z` comes after `F`.** A static function is a function that then says so.

## Substitutions

The compression is most of the difficulty. Every identifier and every nominal
type that has been spelled once is numbered, and the next mention is a
back-reference. The numbering counts what a *demangler* would build, in mangled
order — so an identifier is numbered even though nothing ever refers to one by
itself, because leaving it out would shift every index after it.

References fold two ways, and both can happen at once:

| | |
|---|---|
| `ACAC` → `A2C` | the same index twice carries a count |
| `AFAD` → `AfD` | different indices merge, lowercase for all but the last |

Standard-library types with a letter of their own — `Si`, `Sb`, `SS` — are never
numbered, and fold only the first way: `SiSi` is `S2i`.

## Held to it

`oracle_test.go` compiles each corpus program with `swiftc -emit-sil`, reads the
mangled names out of it, asks `swift demangle` which source function each one
belongs to, and requires exact string equality with what this package produces.
Forty-eight symbols across five programs, covering primitives, structs, classes,
enums, tuples, optionals, arrays, function types, labels, `inout` and `throws`.

`mangle_test.go` makes the same claims without a toolchain to compare against,
and covers the two things the oracle cannot reach through top-level functions:
methods, and statics.

## What is refused

Generics, protocols and existentials, variadic parameters, anything local to a
function body, and any name that is not ASCII — Swift punycodes those, and
writing the marker without the encoding would produce a symbol that demangles
to something else.

Each is refused by name. A symbol that is merely plausible is worse than no
symbol: it links, and it links to the wrong thing.
