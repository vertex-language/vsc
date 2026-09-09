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
