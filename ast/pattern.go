package ast

import "github.com/vertex-language/vsc/token"

// BadPattern covers a pattern the parser gave up on.
type BadPattern struct {
	Span
}

// WildcardPattern is _.
type WildcardPattern struct {
	Span
}

// IdentPattern is an Identifier bound by the pattern.
type IdentPattern struct {
	Span
	Name *Ident
}

// TypedPattern is Pattern TypeAnnotation.
//
// The grammar hangs an optional `: Type` off four of the eight
// pattern alternatives; this node carries it for all of them, so
// there is one place a type annotation lives and one place to read it
// from.
type TypedPattern struct {
	Span
	Pat   Pattern
	Colon token.Pos
	Type  Type
}

// ValueBindingPattern is var Pattern or let Pattern; Kind says which.
type ValueBindingPattern struct {
	Span
	Keyword token.Pos
	Kind    token.Kind // LET or VAR
	Pat     Pattern
}

// TuplePattern is ( [TuplePatternElementList] ).
type TuplePattern struct {
	Span
	Lparen token.Pos
	Elems  []*TuplePatternElem
	Rparen token.Pos
}

// TuplePatternElem is Pattern or Identifier : Pattern.
type TuplePatternElem struct {
	Span
	Label *Ident    // nil when unlabeled
	Colon token.Pos // NoPos with no label
	Pat   Pattern
}

// EnumCasePattern is [TypeIdentifier] . EnumCaseName [TuplePattern].
type EnumCasePattern struct {
	Span
	Type Type // nil for the inferred form, `.some(x)`
	Dot  token.Pos
	Name *Ident
	Args *TuplePattern // nil when the case carries no associated values
}

// OptionalPattern is IdentifierPattern ?
type OptionalPattern struct {
	Span
	Pat      Pattern
	Question token.Pos
}

// IsPattern is is Type — the type-testing TypeCastingPattern.
type IsPattern struct {
	Span
	Is   token.Pos
	Type Type
}

// AsPattern is Pattern as Type — the type-casting one.
type AsPattern struct {
	Span
	Pat  Pattern
	As   token.Pos
	Type Type
}

// ExprPattern is an Expression used as a pattern: what a case label
// matches with ~=.
type ExprPattern struct {
	Span
	X Expr
}

func (*BadPattern) patternNode()          {}
func (*WildcardPattern) patternNode()     {}
func (*IdentPattern) patternNode()        {}
func (*TypedPattern) patternNode()        {}
func (*ValueBindingPattern) patternNode() {}
func (*TuplePattern) patternNode()        {}
func (*EnumCasePattern) patternNode()     {}
func (*OptionalPattern) patternNode()     {}
func (*IsPattern) patternNode()           {}
func (*AsPattern) patternNode()           {}
func (*ExprPattern) patternNode()         {}
