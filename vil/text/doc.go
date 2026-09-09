// Package text prints a VIL module in Swift's SIL syntax.
//
// Exactly Swift's, because the point of printing it at all is that
// the output can be diffed against `swiftc -emit-silgen`. A form that
// is nearly SIL would give a diff that has to be read rather than
// run, which is the whole value gone.
//
// What is not cloned is symbol mangling: Swift's encodes Swift's
// module names and declaration grammar, and ours is not that. The
// differential harness normalizes both sides — symbols to positions,
// and value numbers with them — before comparing.
package text
