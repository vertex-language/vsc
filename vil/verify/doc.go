// Package verify holds a VIL module to the rules a sound one keeps.
//
// A package rather than a method on vil.Module, and for the reason
// ir/verify gives: a verifier is the most demanding reader the IR's
// public surface has, and a rule it cannot state through exported
// methods is a missing method rather than a licence to reach inside.
// Everything here is written against vil's own API.
//
// # What is checked
//
// The structural rules first, because the ownership rules are stated
// over a graph and a malformed graph makes them meaningless: a
// function has an entry block, every block ends in exactly one
// terminator, every block is reachable, the entry block is nobody's
// target, a branch's arguments match its destination's, a body agrees
// with the type it was declared with, and every use is dominated by
// its definition.
//
// Then the two rules the IR exists for:
//
//  1. An owned value is consumed exactly once on every path out of
//     its definition. Not consumed is a leak; consumed twice is a
//     double free; used after being consumed is a use-after-free; and
//     consumed on one path into a block and not the other is all
//     three waiting to happen.
//
//  2. A guaranteed value is used only within the borrow scope that
//     produced it. A scope some path leaves open, a use after it
//     closed, and a consume of the value a borrow was taken from
//     while the borrow is still live are the three ways that fails.
//
// Get these right and ARC insertion is bookkeeping over a property
// already proved, rather than an analysis that has to be trusted.
//
// # When to run it
//
// After vil/gen, before vil/pass, and again after: raw VIL must
// already satisfy both rules — that is what makes them rules rather
// than goals — and a pass that breaks one has a bug the next run
// finds. It is cheap enough to run always.
package verify
