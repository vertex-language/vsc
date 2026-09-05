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
// What is modelled and what is passed over in silence is written down
// in analyzer/README.md, and tests/check is where both halves are held
// to Swift's own verdicts.
package analyzer
