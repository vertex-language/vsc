// Package scanner turns a *token.File into a complete token slice.
//
// The whole file is tokenized up front. Every scan path advances at
// least one byte; malformed input yields an exact span and one
// diagnostic, never a cascade. Nothing is interpreted: literals keep
// their raw spelling, and what an operator means is a
// precedencegroup's business, not this package's.
//
// Three things about Swift's lexical grammar decide the shape of this
// scanner.
//
// Operators are open. An operator is a run of operator characters,
// and the same run is a prefix, infix, or postfix operator depending
// on the whitespace around it: `a+b` and `a + b` are infix, `-x` is
// prefix, `x!` is postfix. So the scanner classifies by binding —
// whether the run is preceded and followed by something that could
// be an operand — exactly as Swift does, and emits OPER_PREFIX,
// OPER_BINARY, or OPER_POSTFIX. The runs the grammar spells out
// itself (=, ->, ., &, ?, !) become their own kinds here, because
// nothing may redeclare them.
//
// A string literal contains expressions. An interpolation holds
// arbitrary Swift, so a literal cannot be one token: the scanner
// opens it, hands out its text a segment at a time, and lets the
// interpolated tokens arrive between BACKSLASH LPAREN and RPAREN.
// String literals nest inside interpolations, so the open literals
// are a stack.
//
// A slash is ambiguous. It divides, it opens a comment, and it opens
// a regex literal. See tryRegex for the rule this scanner uses to
// tell them apart, which is the one SE-0354 states: a bare regex
// literal appears only where an operand may begin, and neither of its
// delimiters may touch whitespace.
package scanner
