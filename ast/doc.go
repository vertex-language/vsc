// Package ast defines the syntax tree vsc's parser builds.
//
// Five hierarchies — Expr, Stmt, Decl, Type, and Pattern — mirroring
// the five families of docs/swift_grammar.md. Types and patterns are
// first-class because the grammar makes them so: a type is written
// where a type goes and nowhere else, and a pattern is what binds
// names in a declaration, a for-in, a case label, and a catch clause
// alike.
//
// Invariants:
//
//   - Every node embeds a Span. Pos and End are stored, not derived,
//     so even error-recovery nodes have a real, non-empty extent.
//   - Nodes hold no text. An Ident is two positions; a literal is two
//     positions and a token.Kind. Decoding — a number's value, a
//     string's escapes, a multiline literal's indentation — belongs
//     to phases above this one. Anything reading spelling takes the
//     *token.File.
//   - Nothing here is resolved. An operator is a span, not a
//     precedence: what `a + b * c` groups into is decided by the
//     precedencegroup declarations the analyzer collects, so the
//     parser leaves a flat SequenceExpr and the analyzer folds it.
package ast
