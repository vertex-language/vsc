// Package mangle spells a declaration as the symbol an object file
// carries, using Swift's scheme.
//
// A compiler has to turn `func fib(_ n: Int) -> Int` into one name a
// linker can hold, and the name has to encode enough of the
// declaration that two functions which differ only in their types get
// different symbols. Swift's answer is `$s3vsc3fibyS2iF`, and this is
// that answer rather than one of our own: a program built here can be
// called by, and can call into, something built by swiftc, and the
// symbols in a crash log demangle with the tools that already exist.
//
// # The shape
//
// A global begins `$s`, then the context the declaration lives in --
// its module, and any types it is nested inside -- then its own name,
// then what it is:
//
//	$s   3vsc   3fib   y     S2i        F
//	     module name   labels  types    a function
//
// The types are the result first and the parameters second, which is
// the opposite of how a demangler prints them.
//
// # Substitutions
//
// The compression is most of the difficulty. Every identifier and
// every nominal type that has been spelled once is numbered, and the
// next mention of it is a back-reference: `AA` is the first thing
// numbered, `AD` the fourth, and `AfD` is two of them at once. The
// numbering counts what a demangler would build, so this package keeps
// the same list in the same order -- an identifier goes in the table
// even though nothing ever refers to one by itself, because leaving it
// out would shift every index after it.
//
// A handful of types from the standard library are spelled with a
// letter instead and are never numbered: `Si` is Int, `SS` is String,
// `Sb` is Bool. The types with no letter of their own are written out
// against the standard library's own one-character module name, which
// is how Int8 becomes `s4Int8V`.
//
// # What is refused
//
// Generics, protocols and existentials, and the mangling of anything
// local to a function body. Each is refused by name. A symbol that is
// merely plausible is worse than no symbol: it links, and it links to
// the wrong thing.
package mangle
