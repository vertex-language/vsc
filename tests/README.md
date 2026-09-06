# tests

Three corpora, asking three different questions.

## syntax/

Does it parse? 94 files covering the grammar, from identifiers to the
syntactic torture tests. Nothing here has to mean anything or run --
several files are deliberately nonsense that happens to be well-formed
-- so the only question asked of them is whether the parser accepts
what swiftc accepts.

Used by `parser` and by `analyzer`'s crash tests.

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
