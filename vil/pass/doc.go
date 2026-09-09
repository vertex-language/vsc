// Package pass transforms a VIL module into another VIL module.
//
// Everything here takes a module and gives the same module back,
// changed. Nothing in it knows about VIR, about a target, or about
// linking: a pass is a fact about the language, and the language is
// what VIL holds.
//
// # The pipeline
//
// Swift's order, because the order is load-bearing. A module arrives
// raw from vil/gen — ownership written down, nothing checked — and
// leaves canonical, meaning every mandatory pass has run and what
// remains is a program that compiles. Then the ownership form itself
// is lowered away, leaving retain and release, which is what a
// backend can be given.
//
//	raw ──▶ mandatory passes ──▶ canonical ──▶ LowerOwnership ──▶ lowered
//
// The distinction that keeps this honest: a pass here may reject a
// program, and an optimization may never. They are different
// packages for that reason, and vil/opt does not exist yet.
//
// # What is here
//
// Three, and each is on the path to a program that runs rather than
// on the path to a program that is diagnosed.
//
// Mandatory runs the two that a module cannot be lowered without.
// allocbox-to-stack turns the box SILGen gives every `var` into a
// stack slot, because nothing below VIL can execute a box; box.go
// says how it decides a box may be promoted, and errs towards leaving
// one alone. Beside it runs the half of definite initialization that
// needs no dataflow — an `assign` to a location holding something
// trivial is a store, whatever came before it — which di.go bounds
// and explains.
//
// LowerOwnership erases the ownership form.
//
// The rest of the diagnostic passes — the other half of definite
// initialization, exclusivity, move-only checking — are not here.
// They come later in the sequence than the eliminator because the
// first thing this compiler needs is a program that runs; what is
// written down of them so far is where di.go stops and why.
package pass
