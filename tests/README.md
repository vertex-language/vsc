# tests

Four corpora, asking four different questions. Each is named for its
question, and a file belongs in exactly one of them.

## syntax/

Does it parse? 94 files covering the grammar, from identifiers to the
combinations that only break a parser when they meet. Nothing here has
to mean anything or run -- several files are deliberately nonsense
that happens to be well-formed -- so the only question asked of them
is whether the parser accepts what swiftc accepts.

Used by `parser`, and by `analyzer`'s crash tests.

## check/

Does it typecheck, and does it say the right thing when it does not?
Files named `ok-*` must check clean; the rest must be rejected, and
the diagnostic is compared against swiftc's.

Used by `analyzer`.

## compiler/

Does the program do what it says? Whole programs, compiled twice --
once by this compiler, once by swiftc -- run twice, and compared.

Nothing here writes down an expected value. A number beside a program
is a claim about Swift that has to be maintained by hand and is wrong
the moment it drifts; swiftc's answer cannot drift, because it is
Swift's answer. So the runner compares outcomes: exit status for a
program that returns, and the same signal for one that traps.

Each file is a whole program with `func main() -> Int32`, this
compiler's entry point. swiftc has no such convention, so for its half
the function is renamed and called from top-level code -- that rewrite
is the only difference between what the two compilers are given.

The files are numbered in the order they get harder, and the early
ones are one idea each -- a loop, a struct, an override -- so that a
failure names the thing that broke rather than the last thing added.
A file belongs here once the compiler can build it: a refusal fails
the suite rather than being skipped, so the corpus is a live statement
of what works rather than a wishlist.

Used by `build`, in `corpus_test.go`.

## interop/

Is what this compiler builds the same thing swiftc builds?

The only way to ask that which cannot be fudged is to put both
compilers' output in one process. A library written in Swift is built
by swiftc; a program is built by this compiler and linked against it.
The symbol asked of the linker has to be the symbol swiftc defined,
and the registers the arguments go in have to be the ones swiftc's
code reads them from.

The libraries use the parts of Swift this compiler does not have --
String, Array, Dictionary, Optional, Codable, Foundation's Calendar,
JSON and URL. That is the point rather than a limitation: a compiler
does not have to implement a library to call it. It has to agree with
it about names and registers, and nothing short of running the two
together shows that it does.

Each case is a directory of three files:

    library.swift             built by swiftc, with -parse-as-library
    <Module>.vertexinterface  what this compiler is told about it
    program.swift             built by this compiler, and run

A case about what happens between two modules has two libraries
instead, named for the order they must be built in and the module they
are: `1-Units.swift`, `2-Scale.swift`, with an interface each. The
order is the point of such a case, so it is in the filename rather
than in a manifest.

The interface is a claim about the library, and the test is whether
the claim is true: naming something swiftc did not build fails at the
link, with the demangled name in the error.

Most of the interfaces here are written by hand, which keeps a case to
the one thing it is about. Case 006 does not: its interface is what
swiftc emitted for its own library, copied in unedited, because a
.swiftinterface is valid Swift with the bodies taken out and reading a
real one is the thing that has to keep working.

Two facts about emitted interfaces are worth writing down, because
getting them the wrong way round costs a day. swiftc emits one under
`-emit-module-interface-path` whatever the mode, warning but not
refusing outside `-enable-library-evolution`; and the two interfaces
are the same text apart from the header. So the interface does not say
which ABI the library was built for -- its `// swift-module-flags:`
line does, and a library built with library evolution is resilient:
its non-@frozen types have no layout the client may rely on, and this
compiler reads them as if they did. Linking against one is a bus
error, not a diagnostic. That is the reason to read the flags line,
and it is not read yet.

A program returns 42 when it is satisfied and the number of the check
that failed otherwise. There is no oracle to compare against here --
the program is this compiler's alone -- so it checks itself, which is
what tests/compiler does for the same reason.

Used by `build`, in `interop_test.go`.
