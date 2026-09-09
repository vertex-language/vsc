package ast

import "github.com/vertex-language/vsc/token"

// Types are kept exactly as written. [[Int]] stays an ArrayType of an
// ArrayType, `T?` stays an OptionalType over an IdentType, and what
// those denote — that they are sugar for Array<Array<Int>> and
// Optional<T> — is the analyzer's reading, not this tree's.

// BadType covers a type the parser gave up on.
type BadType struct {
	Span
}

// IdentType is the head of a TypeIdentifier: a TypeName and its
// generic arguments.
type IdentType struct {
	Span
	Name *Ident
	Args *GenericArgs // nil when the name carries none
}

// MemberType is one qualified step of a TypeIdentifier — the
// `.Element` of `Array<Int>.Element`.
type MemberType struct {
	Span
	X    Type
	Dot  token.Pos
	Name *Ident
	Args *GenericArgs
}

// ParenType is ( Type ). It is not a one-element TupleType: Swift has
// no one-element tuple, and `(Int)` is an Int in parentheses.
type ParenType struct {
	Span
	Lparen token.Pos
	X      Type
	Rparen token.Pos
}

// TupleType is ( [TupleTypeElementList] ).
type TupleType struct {
	Span
	Lparen token.Pos
	Elems  []*TupleTypeElem
	Rparen token.Pos
}

// TupleTypeElem is [Identifier :] Type.
type TupleTypeElem struct {
	Span
	Name  *Ident    // nil when the element is unlabeled
	Colon token.Pos // NoPos with no label
	Type  Type
}

// FuncType is [AttributeList] ( [ParameterList] ) [async]
// [ThrowsClause] -> Type.
type FuncType struct {
	Span
	Attrs  []*Attr
	Lparen token.Pos
	Params []*Param
	Rparen token.Pos
	Async  token.Pos     // NoPos when absent
	Throws *ThrowsClause // nil when absent; covers rethrows too
	Arrow  token.Pos
	Result Type
}

// ArrayType is '[' Type ']'.
type ArrayType struct {
	Span
	Lsquare token.Pos
	Elem    Type
	Rsquare token.Pos
}

// DictType is '[' Type : Type ']'.
type DictType struct {
	Span
	Lsquare token.Pos
	Key     Type
	Colon   token.Pos
	Value   Type
	Rsquare token.Pos
}

// SizedArrayType is '[' Expr 'of' Type ']' (SE-0483).
type SizedArrayType struct {
	Span
	Lsquare token.Pos
	Size    Expr
	Of      token.Pos
	Elem    Type
	Rsquare token.Pos
}

// OptionalType is Type ?
type OptionalType struct {
	Span
	Base     Type
	Question token.Pos
}

// UnwrappedType is Type ! — the ImplicitlyUnwrappedOptionalType.
type UnwrappedType struct {
	Span
	Base    Type
	Exclaim token.Pos
}

// MetatypeType is Type . Type or Type . Protocol. Keyword names which
// of the two was written; the grammar reserves neither spelling, so
// it is an Ident.
type MetatypeType struct {
	Span
	Base    Type
	Dot     token.Pos
	Keyword *Ident
}

// IntegerType is an integer written where a type goes: the argument
// of a value generic parameter, as in `InlineArray<3, Int>`. The sign
// is part of it — `A<-1>` is written that way.
type IntegerType struct {
	Span
	Minus token.Pos // NoPos unless negated
}

// PlaceholderType is `_` written where a type goes: the type is
// there, and the analyzer is being asked to work out which one.
// `Array<_>` and `[_]` are the common spellings.
type PlaceholderType struct {
	Span
}

// AnyType is the bare `Any`.
type AnyType struct {
	Span
}

// SelfType is the bare `Self`.
type SelfType struct {
	Span
}

// OpaqueType is [AttributeList] some Type.
type OpaqueType struct {
	Span
	Attrs   []*Attr
	Keyword token.Pos // the `some`
	Base    Type
}

// BoxedType is [AttributeList] any Type — the BoxedProtocolType.
type BoxedType struct {
	Span
	Attrs   []*Attr
	Keyword token.Pos // the `any`
	Base    Type
}

// IsolationType is nonisolated(nonsending) Type: a function type
// whose async calls run on the caller's executor rather than hopping
// to a nonisolated one. The grammar does not have it; see the
// parser's README.
type IsolationType struct {
	Span
	Mod  *Modifier // the `nonisolated(nonsending)`
	Base Type
}

// SendingType is sending Type: a value the caller hands over rather
// than shares, on a parameter or on a result. The grammar does not
// have it; see the parser's README.
type SendingType struct {
	Span
	Keyword token.Pos
	Base    Type
}

// InverseType is ~ Type: a suppressed implicit conformance, as in
// `struct S: ~Copyable`. It is a type wherever a constraint goes —
// an inheritance clause, a generic parameter's bound, a requirement,
// or one member of a composition — so `~Copyable & ~Escapable` is a
// CompositionType of two of these.
type InverseType struct {
	Span
	Tilde token.Pos
	Base  Type
}

// CompositionType is TypeIdentifier {& TypeIdentifier}. Amps holds
// the ampersands, one fewer than Types.
type CompositionType struct {
	Span
	Types []Type
	Amps  []token.Pos
}

// PackExpansionType is repeat Type.
type PackExpansionType struct {
	Span
	Repeat token.Pos
	Base   Type
}

// PackReferenceType is each Type. It is also the `each Type` form of
// GenericArgument, which is the same syntax in the same position.
type PackReferenceType struct {
	Span
	Each token.Pos
	Base Type
}

// MacroExpansionType is # Identifier [GenericArgumentClause]
// [FunctionCallArgumentClause] [TrailingClosures].
type MacroExpansionType struct {
	Span
	Pound    token.Pos
	Name     *Ident
	Args     *GenericArgs
	Call     *CallArgs
	Trailing []*TrailingClosure
}

// ---- pieces shared by types and declarations ----

// Param is one entry of a ParameterList.
//
// The grammar writes a single Parameter production, `[ArgumentLabel
// :] [ParameterModifierList] Type [...]`, and uses it for a function
// type's parameters and a function declaration's alike. A declaration
// needs two things that production does not carry — the local name in
// `func move(to dest: point)`, and a default value — so this node
// holds them, and the parser fills them only where a declaration is
// being read. See the parser's README for the list of these gaps.
type Param struct {
	Span
	Attrs    []*Attr
	Label    *Ident      // the ArgumentLabel, or nil; `_` is an Ident too
	Name     *Ident      // the local name of a declaration's parameter
	Colon    token.Pos   // NoPos when the parameter is a bare type
	Mods     []*Modifier // inout, borrowing, consuming
	Type     Type
	Ellipsis token.Pos // the variadic `...`, NoPos if absent
	Assign   token.Pos // NoPos when the parameter has no default
	Default  Expr
}

// ThrowsClause is throws, throws ( Type ), or rethrows. Kind is
// THROWS or RETHROWS.
type ThrowsClause struct {
	Span
	Keyword token.Pos
	Kind    token.Kind
	Lparen  token.Pos // NoPos in the unparenthesized form
	Type    Type
	Rparen  token.Pos
}

// FuncResult is -> Type.
type FuncResult struct {
	Span
	Arrow token.Pos
	Type  Type
}

func (*BadType) typeNode()            {}
func (*IdentType) typeNode()          {}
func (*MemberType) typeNode()         {}
func (*ParenType) typeNode()          {}
func (*TupleType) typeNode()          {}
func (*FuncType) typeNode()           {}
func (*ArrayType) typeNode()          {}
func (*DictType) typeNode()           {}
func (*SizedArrayType) typeNode()     {}
func (*OptionalType) typeNode()       {}
func (*UnwrappedType) typeNode()      {}
func (*MetatypeType) typeNode()       {}
func (*IntegerType) typeNode()        {}
func (*PlaceholderType) typeNode()    {}
func (*AnyType) typeNode()            {}
func (*SelfType) typeNode()           {}
func (*OpaqueType) typeNode()         {}
func (*BoxedType) typeNode()          {}
func (*IsolationType) typeNode()      {}
func (*SendingType) typeNode()        {}
func (*InverseType) typeNode()        {}
func (*CompositionType) typeNode()    {}
func (*PackExpansionType) typeNode()  {}
func (*PackReferenceType) typeNode()  {}
func (*MacroExpansionType) typeNode() {}
