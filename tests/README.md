# tests

A ladder: one small thing per file, numbered `001`–`385` in the order the
rungs climb, from an empty program to a closing program that uses most of
the language.

Nothing here writes down an expected value. Every file is built twice,
once by vsc and once by swiftc, and the two results are compared. swiftc's
answer is the oracle, and a disagreement with it is a bug in vsc by
definition.

| Ladder | Files | The oracle | Compared |
| --- | --- | --- | --- |
| `tests/` | `001`–`272` `.swift` | swiftc on Apple Silicon | stdout and how the run ended |

Each file is a `main.swift` of top-level code, and both compilers are
given it unchanged. There is no entry point to rename and no harness
around it.

## `001`–`272`

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
| 260 | a default argument written for a type parameter, typed at each call by what the call makes it |
| 261 | `Float16` built in: arithmetic rounding once, comparisons, conversions both ways, rounding, special values, bit patterns, in generic code and arrays |
| 262 | a pointer's bit pattern: `Int(bitPattern:)` and `UInt(bitPattern:)` of a pointer, and the failable `init?(bitPattern:)` back, nil at zero |
| 263 | a generic function's `T` beside the `T` of the generic methods it calls: `map`, `reduce`, `compactMap` |
| 264 | a literal as an associated type its protocol makes `Numeric`: `v < 0` and `v = 0` for a `T.Acc` |
| 265 | an array literal appended with `+=`, typed by the array: `bytes += [0x80]` on a `[UInt8]` |
| 266 | `split` of a `Substring`: lines split into fields, `maxSplits`, empty pieces kept |
| 267 | a `Substring` sliced as a `String` is (`dropFirst`, `prefix`, index arithmetic, range subscripts), and compared with a `String` |
| 268 | a generic function's `T` inferred from the pointer passed: `UnsafePointer<T>` of `Int32`s and `UInt8`s, `UnsafeMutablePointer<T>` |
| 269 | a class initializer that throws before its properties are all set: caught, `try?`, through `super.init`; the unfinished instance's `deinit` does not run |
| 270 | closures whose parameter is a tuple of mixed registers, `(Int, Double)`: called, and mapped, filtered and reduced over an array of them |
| 271 | a generic class's instance as `AnyObject`, stored, tested with `is` and cast back with `as?` |
| 272 | pointers made writable with `init(mutating:)`, typed and raw |
| 331–367 | gaps `vsc_TODO.md` recorded, one rung each: generic initializers, in place `dict[k, default:]` appends and element stores, extension methods on existentials, synthesized `Comparable`, implicit class `init()`, `nonmutating set` through a `let`, type parameters of one name, `Task(priority:)`, `async let` under an executor preference, temporary and global lifetimes, optional ternaries, local functions in a `switch` case, label overloads, mutually recursive local functions, thrown structs and payload enums, unsigned conversions and shifts, `Substring.utf8`, tuple patterns over payload enums, inherited members from a subclass, escaping closures in closures, `ObjectIdentifier`, `String.removeFirst`/`removeLast`, nested empty dictionary literals, an overload chosen by function type, generic method overrides, a type nested in an extension, stored properties through a class-constrained `T`, `AnyObject?` downcasts |
| 368 | one existential into another of fewer protocols: `any Q` and `any Error` into `Any`, `any P & Q` into `any Q` -- the value, its type and the tables the destination names, and no more |
| 369 | a property wrapper on a local: storage of the wrapper's type, reads and writes through it, `$name`, captured and written by a closure, a generic class wrapper passed by its projection |
| 370 | an if or switch expression whose value is an existential: each branch a different conforming type, as a result, an initializer and a closure's value |
| 371 | a mutating call through a computed property written back when its statement ends, where an argument's getter is specialized mid-statement |
| 372 | a computed property named alone in a method or initializer written through its setter, and an observed one through its observers, except inside its own |
| 373 | `removeLast(k)` in place: a large array popped four at a time stays linear |
| 374 | `try?` on a call that answers an optional, a tuple's or an `Int`'s: one level of optional, compared with nil and bound |
| 375 | a `for`-`in` variable's annotation typing the array literal it walks: `for c: UInt32 in [0x30, 0x31]` |
| 376 | a closure capturing an existential held in memory -- a parameter, a let, a catch block's `error`, `any AnyObject`-bound, `Any` -- copied into a box in its context: not escaping, escaping and freed, nested, throwing |
| 377 | `a ?? b` whose type is an existential, with b a concrete value made into it: a retry loop's last error, a protocol value with a default, a class payload freed once |
| 378 | A closure passed to a generic function returns what the call's result is wanted as: `.array(xs.map { .string($0) })`, `let v: [Value] = ….map { .number($0) }` |
| 379 | An operator whose result goes where an `Any` is wanted reads its operands on their own: `print("a" + (ok ? "b" : ""))` |
| 380 | A Substring's `firstIndex(of:)`, `lastIndex(of:)` and `contains(_:)`, its indices the base string's |
| 381 | `firstIndex(where:)`/`lastIndex(where:)` on String and Substring; an optional Character compared with a literal (`s.first == "#"`) |
| 382 | `String(chars[i..<j])`: a String from an `ArraySlice<Character>` |
| 383 | `UInt32(text, radix: 8)` and the rest are failable for every integer type; `ok ? [] : xs` types the empty literal from the other branch |
| 384 | A `try?` passed straight to a call -- `Box(try? read())` -- ends with its statement instead of leaking |
| 385 | `var x: P` given its value on each branch of an async function: the first assignment ends nothing, whatever an earlier frame left in the slot, and the scope ends the last value |
| 386–450 | the rest of `vsc_TODO.md`, one rung per gap a single file can show: generic class properties, subscripts and subclasses (#2, #3, #75), generic code reaching globals and private types (#4, #62, #63), `[any P]` methods (#5), overload ranking (#6), casts to classes nested in an enum and through its typealias (#8, #22), `required init()` through `E.Type` (#9), local computed variables (#10), a wrapper on a class property (#11), `Any` holding arrays and generic payload enums (#12, #13), T solved from a trailing closure (#15), stored defaults calling later functions (#17), `try?` of `Void` (#18), `Dictionary.removeAll` (#19), closure results from context (#20, #50), `String.utf16`, `String(decoding:as: UTF16.self)`, `Unicode.Scalar` (#23, #37, #38), literals typed by an optional, a tuple or a `?:` (#24, #27, #46, #49, #51, #70, #72, #86), argument temporaries ending in reverse (#25), local functions declared later or recursive (#31, #56), optional tuple bindings (#34, #68), empty collection defaults (#35), tuples of existentials (#36, #57, #58), `if case` against an optional (#39), an async `if let` over `??` (#40), `|=` as `Void` (#41), dictionaries of existentials (#42), `Array(set)`, `split(whereSeparator:)`, multi-character `split`, `reverse()` (#43, #44), internal default arguments (#45), `[[T]](repeating:count:)` (#47, #85), `map` over struct fields (#48), pointers: inout captures, conversions, raw buffers, `update(from:count:)` (#52, #53, #81, #82), failable class inits (#54), `$0` in a range's `first(where:)` (#55), written `hash(into:)` (#61), `for var` (#69), large slice copies (#74), `try` in `self.init` arguments (#77), tuple switches over releasing payloads (#78), closures over `self` in an init (#80), recursive async after async work (#28) |

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
