// Package parser turns a *token.File into an *ast.File plus a sorted
// diagnostic slice.
//
// Recursive descent, with three things the grammar forces on it.
//
// Expressions are not folded. Precedence is declared — a
// precedencegroup somewhere else in the program decides how `a ~> b +
// c` groups — so the parser produces the grammar's own shape, a flat
// SequenceExpr of operands and operators, and the analyzer folds it
// once it has read every declaration.
//
// Some decisions need lookahead. `<` opens a generic argument list or
// compares two values; `(` opens a tuple, a parenthesized expression,
// or a function type's parameters. Where a token cannot settle it,
// the parser marks its position, tries one reading, and resets — see
// mark and reset, which restore the diagnostics too, so a speculative
// attempt reports nothing.
//
// A brace is ambiguous by design. `if x { … }` and `f { … }` are the
// same two tokens with different meanings, so a construct that is
// followed by a body — a condition, a switch subject, a for-in
// sequence — parses its expression in basic mode, where a brace ends
// the expression rather than opening a trailing closure. This is the
// rule Swift itself uses.
//
// The parser interprets nothing. It decides which production applies
// and where each node begins and ends; it does not decode literals,
// resolve names, or check that a modifier belongs where it is written
// — except where the grammar's own production says so, in which case
// the mismatch is reported as a syntax error and parsing continues.
//
// A partial parse is a usable one: every entry point returns a node —
// a Bad* placeholder if it must — so consumers read a tree, not a
// success flag.
package parser
