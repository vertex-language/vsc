// Package token defines the lexical vocabulary of the Vertex source
// language — Swift 6 syntax, as given in docs/swift_grammar.md — and
// the per-file position space every span in the front end resolves
// through.
//
// Invariants:
//
//  1. Nothing below the parser interprets. Tokens carry no text;
//     literals arrive undecoded and resolve through the File that
//     produced them.
//  2. No cross-file address space. A Pos is per-File.
//  3. Every span is non-empty (End > Pos), including ILLEGAL. The
//     scanner's EOF token is the one zero-width exception.
//
// Two facts about Swift shape this package, and both are absences.
//
// There is no fixed operator table: an operator is a run of operator
// characters, and what it means — its precedence, its associativity,
// whether it exists at all — is decided by a precedencegroup
// declaration the analyzer resolves. So there is no Precedence method
// here and no per-operator Kind. An operator token is one of
// OPER_PREFIX, OPER_BINARY, or OPER_POSTFIX, and its spelling is its
// span. The scanner picks among the three by the whitespace around
// the run, which is the only thing that can be known lexically.
//
// There is no preprocessor: #if is a statement in the grammar, so
// conditional compilation reaches the parser as tokens like any other
// construct, and this package never sees a translated buffer. The
// text the scanner reads is the text the user typed.
package token

// Pos is a compact position within one File: byte offset into the
// source, plus one, so the zero value NoPos is invalid.
type Pos int32

// NoPos is the invalid position. Fields like a delimiter that was
// never written hold NoPos.
const NoPos Pos = 0

func (p Pos) IsValid() bool { return p > NoPos }

// Flags carry lexical facts the parser mostly ignores but
// diagnostics, formatters, and a few grammar rules need.
type Flags uint8

const (
	// FlagAdjacent: no whitespace or comment separates this token
	// from the previous one.
	FlagAdjacent Flags = 1 << iota
	// FlagNLBefore: a line terminator appeared before this token.
	// The grammar's semicolons are optional, so this is what ends a
	// statement.
	FlagNLBefore
	// FlagLeftBound, FlagRightBound: the operator-spacing facts the
	// scanner classified this token by, kept because a diagnostic
	// about `a! .b` has to say which side was bound. Set on operator
	// tokens and on the punctuation split out of them (=, ->, ., ?,
	// !, &).
	FlagLeftBound
	FlagRightBound
	// FlagMultiline: this string piece belongs to a """ literal.
	FlagMultiline
	// FlagRaw: this string or regex literal carries # delimiters.
	FlagRaw
	// FlagEscaped: this identifier was written in backticks.
	FlagEscaped
)

func (f Flags) Has(g Flags) bool { return f&g != 0 }

// Token is a kind and a span. It holds no text: spelling resolves
// through the File via Slice.
type Token struct {
	Kind  Kind
	Flags Flags
	Pos   Pos // inclusive
	End   Pos // exclusive
}
