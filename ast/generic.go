package ast

import "github.com/vertex-language/vsc/token"

// GenericParams is < GenericParameterList >.
type GenericParams struct {
	Span
	Lt     token.Pos
	Params []*GenericParam
	Gt     token.Pos
}

// GenericParam is [each] TypeName [TypeInheritanceClause], or the
// value form, `let n: Int`, whose argument is a number rather than a
// type — the `n` of `InlineArray<let n: Int, Element>`.
type GenericParam struct {
	Span
	Each    token.Pos // NoPos unless a parameter pack
	Let     token.Pos // NoPos unless a value parameter
	Name    *Ident
	Inherit *InheritanceClause
}

// GenericArgs is < GenericArgumentList >. An `each Type` argument is
// a PackReferenceType, which is the same syntax.
type GenericArgs struct {
	Span
	Lt   token.Pos
	Args []Type
	Gt   token.Pos
}

// GenericWhereClause is where RequirementList. Reqs holds
// *ConformanceReq and *SameTypeReq.
type GenericWhereClause struct {
	Span
	Where token.Pos
	Reqs  []Node
}

// ConformanceReq is TypeIdentifier : TypeIdentifier, or
// TypeIdentifier : ProtocolCompositionType. A suppressed conformance
// — the `~Copyable` of `T: ~Copyable` — is an InverseType on the
// right, not a flag here.
type ConformanceReq struct {
	Span
	Left  Type
	Colon token.Pos
	Right Type
}

// SameTypeReq is TypeIdentifier == Type.
type SameTypeReq struct {
	Span
	Left  Type
	EqEq  token.Pos
	Right Type
}

// InheritanceClause is : InheritanceList.
type InheritanceClause struct {
	Span
	Colon token.Pos
	Items []*InheritanceItem
}

// InheritanceItem is one entry of an InheritanceList. A suppressed
// conformance — the `~Copyable` of `struct S: ~Copyable` — is an
// InverseType in Type.
//
// Attrs are the attributes written on the item — the @unchecked of
// `: @unchecked Sendable`, which says the conformance is asserted
// rather than checked. The grammar leaves them out of this position;
// see the parser's README.
type InheritanceItem struct {
	Span
	Attrs       []*Attr
	Nonisolated token.Pos // NoPos unless the conformance is nonisolated
	Type        Type
}
