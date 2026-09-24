# tests

A ladder: one small thing per file, numbered `001`–`259` in the order the
rungs climb, from an empty program to a closing program that uses most of
the language.

Nothing here writes down an expected value. Every file is built twice,
once by vsc and once by swiftc, and the two results are compared. swiftc's
answer is the oracle, and a disagreement with it is a bug in vsc by
definition.

| Ladder | Files | The oracle | Compared |
| --- | --- | --- | --- |
| `tests/` | `001`–`259` `.swift` | swiftc on Apple Silicon | stdout and how the run ended |

Each file is a `main.swift` of top-level code, and both compilers are
given it unchanged. There is no entry point to rename and no harness
around it.

## `001`–`259`

| | |
| --- | --- |
| 001–020 | the smallest programs and `print`, then integer arithmetic one operator at a time: add, sub, mul, div, remainder, unsigned, the overflow trap, the wrapping operators, shifts, bitwise, compares, logical, `?:`, compound assignment |
| 021–040 | every scalar type: the integer widths and their bounds, conversions exact, truncating, clamping and trapping, `Bool`, `Double`, `Float`, float to integer, NaN and infinity, literals, inference, bit counts, the overflow-reporting methods, `typealias` |
| 041–060 | control flow: `if`, `else if`, the loops, ranges, `stride`, `break` and `continue`, labels, `switch` on integers, ranges and tuples, `where`, `fallthrough`, `guard`, `defer`, recursion, and `if` and `switch` as expressions |
| 061–080 | functions and closures: labels, defaults, variadics, `inout`, overloading, nested functions, function values, closures and their shorthand, captures and capture lists, `@escaping`, `@autoclosure`, composition, trailing closures, generic functions, `Never`, operators of a program's own and precedence groups |
| 081–100 | strings, optionals and tuples: literals, multi-line and raw strings, interpolation, characters, comparison, the everyday methods, Unicode views, substrings, conversions; `if let`, `??`, `?.`, the `!` trap, `map` on an optional; tuples; then `Array` |
| 101–120 | collections: the index trap, mutation, copy on write, iteration, `map`/`filter`/`reduce`, sorting, slices, searching, nested arrays, `Dictionary`, `Set` and its algebra, ranges as values, `zip`, `lazy`, `compactMap` and `flatMap`, the other sequence algorithms, `joined` |
| 121–140 | structs and enums: memberwise and custom initializers, methods and `mutating`, computed properties, observers, `lazy`, statics, subscripts, value semantics; plain, raw-valued and payload enums, `CaseIterable`, `indirect`, nested patterns, nested types, extensions, `init?` |
| 141–160 | classes: reference semantics and `===`, inheritance, `override` and `super`, `class` and `final` members, convenience and required initializers, `deinit`, `weak`, `unowned`, closure cycles, `is` and `as?`, `Any` and `AnyObject`, access control, property wrappers, key paths, two-phase initialization, copy on write by hand, a hierarchy |
| 161–180 | protocols and generics: requirements, protocol extensions, mutating and static requirements, `any`, `some`, composition, associated types, generic types, constraints, conditional conformance, `Equatable`, `Hashable`, `Comparable`, `CustomStringConvertible`, a `Sequence` and a `Collection` of a program's own, refinement, primary associated types, generic subscripts |
| 181–200 | errors and concurrency, and the rest: `throw` and `catch`, propagation, `try?`, `rethrows`, typed throws, `Result`, an uncaught error; `async`/`await`, `async let`, task groups, `Task`, actors, async sequences; result builders, `@dynamicMemberLookup`, `callAsFunction`, parameter packs, `~Copyable`, `#if`, and a closing program |
| 201–210 | numbers, further: operators as static methods, the integer methods, how a `Double` prints across its range, `Float16`, code over `BinaryInteger` and `FloatingPoint`, a `~=` of a program's own, the literal protocols, `OptionSet`, the defined edges of integer arithmetic |
| 211–220 | control flow, further: `for case`, `while let`, condition lists, `switch` on strings and characters, several patterns in one case, scopes and shadowing, labelled `do` and `if`, `defer` on every exit, `guard case`, `switch` over optionals |
| 221–230 | functions and values, further: stored closures, recursion through local functions and a fixed point, currying, closures inferred through generics, method and initializer references, static subscripts, tuple destructuring, `inout` writeback through properties and subscripts, mutation deep inside nested values |
| 231–240 | types, further: generic enums, recursive structs, associated type defaults, opening an existential, generic class inheritance, class-only protocols and weak delegates, `Identifiable`, static factories, raw values of every kind, a struct holding a class |
| 241–250 | programs: a Caesar cipher over Unicode scalars, word frequencies, a multi-key sort, generic binary search and insertion sort, matrices, a tokenizer, a linked list freed in order, a stack-machine interpreter, an async pipeline, and a closing program replaying a transaction log |
| 251–259 | what the packages needed: an existential as a result and as a stored property, casts to a protocol, `if let` of a tuple, an optional of a struct holding a `Bool`, pointer and buffer-pointer subscripts, `append(contentsOf:)` of a slice, an `AsyncSequence` that is its own iterator, a conversion through a narrow integer nested in one expression |

## Rules

- **One thing per file.** A failure should name what broke. When a test
  turns out to be asking two questions, split it.
- **Every rung is Swift.** swiftc has to build it unchanged; a file only
  vsc accepts belongs in a package's own tests, not here.
- **Nothing unspecified.** No dictionary or set is printed with more than
  one entry, no hash values, no addresses, no clock, no tasks racing to
  print: an unordered result is sorted before it is printed.
- **A trapping rung prints nothing before the trap.** A trap ends the
  program by a signal, and output buffered before it may be lost -- the
  two runtimes buffer differently -- so only the signal is compared.
- **Each run is capped** at 10 seconds and 1 MB of output, so a
  miscompiled loop fails its own test instead of the whole run.
- **A refusal is a failure.** A diagnostic from vsc fails the rung rather
  than skipping it, so the ladder says plainly how far up the compiler
  has climbed.
- **Every fix lands with the smallest numbered file that shows it.**

## The other corpora

Suites that are not one file, or that ask a question other than "does
it run the way swiftc's build does", live beside the package that runs
them:

| Corpus | Asks | Run by |
| --- | --- | --- |
| `parser/testdata/syntax` | does it parse? | `parser` (which parses this ladder too) |
| `analyzer/testdata/check` | does it typecheck, and say what swiftc says when it does not? | `analyzer` |
| `build/testdata/interop` | is what vsc builds the same thing swiftc builds, linked in one process? | `build`, `TestInteropCorpus` |
| `build/testdata/cinterop` | can C call what vsc builds, and can it call C? | `build`, `TestCInteropCorpus` |
| `build/testdata/packages` | does a SwiftPM package build as `swift build` builds it? | `build`, `TestPackagesMatchSwiftPM`; `pkg` |
| `tests/kernel` | does a kernel print what it says, on the CPU device and on Metal alike? | the root package, `TestKernels`; see its README |

## Running

```console
$ cd build && go test -run TestCorpus .          # the ladder
$ cd build && go test -run 'TestCorpus/113' .    # one rung
$ vsc run tests/042-else-if.swift                # one rung, by hand
```
