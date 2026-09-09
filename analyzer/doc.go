// Package analyzer reads a parsed program: it resolves names, folds
// the expression sequences the parser left flat, gives every
// expression a type, and decodes what every literal says.
//
// The passes run in order, over every file at once, because a Swift
// program is not read top to bottom: a function may call one declared
// below it, and a type may be used before the line that declares it.
// So the names come first — precedence groups and operators, then
// nominal types, then their members, then extensions, then functions
// — and only when all of them are known are the bodies checked.
//
// The rule the whole package is built on:
//
//	Where the checker does not know, it says nothing. It never
//	invents an answer.
//
// An invented type is worse than no type. A parser that rejects valid
// Swift fails loudly and someone fixes it; a checker that answers Int
// where it cannot work out the answer hands the phase below it a
// well-typed module describing a program nobody wrote. So a type this
// package cannot read is Invalid, a diagnostic whose subject is
// Invalid is not reported, and one mistake in the source is one
// diagnostic in the output.
//
// A protocol may leave a type to whoever conforms to it -- an
// associated type -- and then `C.Item` in a generic is a dependency
// rather than a type: which one it is follows from what C turns out
// to be. The conformer chooses, with a typealias or by writing the
// implementation, and a `where` clause is how a caller says more
// about the choice than the angle brackets can. See
// analyzer/associated.go.
//
// A member may belong to the type rather than to an instance of it.
// `Vec.zero()` is one and `v.zero()` is not Swift, so a static and an
// instance member of the same name are two members: which of the two
// a name finds depends on whether it was read off the type.
//
// A property may be a function that looks like a field. A computed
// property has no storage and a static one is the type's rather than
// an instance's, so neither is a field -- and counting one as a field
// is not a wrong member but a wrong layout, which is a wrong answer
// for everything after it. See readMembers, which keeps them apart.
//
// A type may be declared inside another type, and then it is reached
// through the outer name: `Chart.Point`, `String.Index`. Nesting
// decides the name and nothing else -- the type is a struct like any
// other -- but the name is what an interface writes and what a symbol
// has to say.
//
// A name may be said through the module it came from. `Swift.Int32`
// is Int32, `Foundation.Data` is Foundation's Data and not somebody
// else's, and both spellings resolve to one symbol -- a module's name
// is how a name is found, not part of what is found. This is what
// reading a .swiftinterface needs, because an interface qualifies
// every name in it; see resolveMemberType and moduleMemberValue.
// Nothing declares a module, so its name means one only where nothing
// else has taken it, which is how swiftc resolves it too.
//
// What is modelled and what is passed over in silence is written down
// in analyzer/README.md, and tests/check is where both halves are held
// to Swift's own verdicts.
package analyzer
