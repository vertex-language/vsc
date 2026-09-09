package ast

import "github.com/vertex-language/vsc/token"

// Attr is one @ AttributeName [AttributeArgumentClause].
//
// The arguments are BalancedTokens: the grammar says so, and it is
// right to. An attribute's arguments are its own language —
// @available's platform list, @objc's selector, an @attached macro's
// role — and reading them as expressions would give every one of them
// a tree that means nothing. They are kept as the tokens between the
// parens, balanced and otherwise unread.
type Attr struct {
	Span
	At     token.Pos
	Name   Type      // an Identifier or a TypeIdentifier
	Lparen token.Pos // NoPos when the attribute takes no arguments
	Tokens []token.Token
	Rparen token.Pos
}

// Modifier is one DeclarationModifier, including the parenthesized
// forms — private(set), unowned(unsafe). Kind is the modifier's
// keyword kind, or IDENT for the contextual ones, which is most of
// them: only class, static and the access levels are reserved words.
type Modifier struct {
	Span
	Kind   token.Kind
	Name   *Ident
	Lparen token.Pos // NoPos without an argument
	Arg    *Ident
	Rparen token.Pos
}

// BadDecl covers a declaration the parser gave up on.
type BadDecl struct {
	Span
}

// ImportDecl is [AttributeList] [AccessLevelModifier] import
// [ImportKind] ImportPath. Kind is the import kind's keyword (STRUCT,
// FUNC, …) or ILLEGAL when none was written; the path is one or more
// dotted identifiers.
//
// Mods holds the access level an import may carry — `internal import
// Darwin` says the module is not re-exported to a client. It is the
// only modifier the production admits.
type ImportDecl struct {
	Span
	Attrs   []*Attr
	Mods    []*Modifier
	Import  token.Pos
	Kind    token.Kind
	KindPos token.Pos
	Path    []*Ident
}

// VarDecl is a ConstantDeclaration or a VariableDeclaration; Kind is
// LET or VAR.
//
// The grammar writes them as two productions, but they differ in one
// keyword and in what may follow the binding: only a var may have a
// getter, a setter, or an observer. Keeping them one node keeps the
// binding list one shape, and the check that a let has no accessors
// is a check.
type VarDecl struct {
	Span
	Attrs    []*Attr
	Mods     []*Modifier
	Keyword  token.Pos
	Kind     token.Kind
	Bindings []*PatternBinding
}

// PatternBinding is one PatternInitializer, plus the accessors a
// variable may carry instead of — or after — its initializer.
//
// Body is the implicit-getter form, `var x: Int { 1 }`. Accessors is
// every other one: a getter and setter block, a getter keyword clause
// in a protocol, or a willSet/didSet observer block.
type PatternBinding struct {
	Span
	Pat       Pattern
	Assign    token.Pos // NoPos when the binding has no initializer
	Value     Expr
	Body      *CodeBlock
	Accessors *AccessorBlock
}

// AccessorBlock is '{' … '}' holding get, set, willSet and didSet
// clauses in written order.
type AccessorBlock struct {
	Span
	Lbrace    token.Pos
	Accessors []*Accessor
	Rbrace    token.Pos
}

// Accessor is one get, set, willSet or didSet clause. Name is the
// setter's or observer's parameter, the `newValue` of `set(newValue)`.
// Body is nil in a GetterKeywordClause or SetterKeywordClause — the
// form a protocol writes, which says an accessor exists without
// saying what it does.
type Accessor struct {
	Span
	Attrs   []*Attr
	Mods    []*Modifier // the MutationModifier, if one was written
	Keyword *Ident      // get, set, willSet, didSet — all contextual
	Lparen  token.Pos
	Name    *Ident
	Rparen  token.Pos
	Async   token.Pos
	Throws  *ThrowsClause
	Body    *CodeBlock
}

// TypealiasDecl is [AttributeList] [AccessLevelModifier] typealias
// TypealiasName [GenericParameterClause] = Type
// [GenericWhereClause].
//
// The where clause constrains the alias's own parameters —
// `typealias CountableRange<Bound> = Range<Bound> where Bound:
// Strideable`. The grammar leaves it off this declaration and gives
// it to every other generic one; see the parser's README.
type TypealiasDecl struct {
	Span
	Attrs    []*Attr
	Mods     []*Modifier
	Keyword  token.Pos
	Name     *Ident
	Generics *GenericParams
	Assign   token.Pos
	Type     Type
	Where    *GenericWhereClause
}

// FuncDecl is [AttributeList] [DeclarationModifiers] func FunctionName
// [GenericParameterClause] FunctionSignature [GenericWhereClause]
// [FunctionBody].
//
// FunctionName is an Identifier or an Operator, and an Ident is a
// span, so an operator name is recorded as one. Body is nil in a
// protocol's method requirement.
type FuncDecl struct {
	Span
	Attrs    []*Attr
	Mods     []*Modifier
	Func     token.Pos
	Name     *Ident
	Generics *GenericParams
	Sig      *FuncSig
	Where    *GenericWhereClause
	Body     *CodeBlock
}

// FuncSig is ( [ParameterList] ) [async] [ThrowsClause]
// [FunctionResult]. Throws covers rethrows.
type FuncSig struct {
	Span
	Lparen token.Pos
	Params []*Param
	Rparen token.Pos
	Async  token.Pos
	Throws *ThrowsClause
	Result *FuncResult
}

// MemberBlock is the '{' … '}' of a type declaration. Members holds
// Decl and the *IfConfigStmt of a conditional member — the grammar's
// CompilerControlStatement, which stands among members wherever it
// stands among statements.
type MemberBlock struct {
	Span
	Lbrace  token.Pos
	Members []Node
	Rbrace  token.Pos
}

// EnumDecl is an EnumDeclaration. Its members may include
// EnumCaseDecl, which no other type declaration admits.
type EnumDecl struct {
	Span
	Attrs    []*Attr
	Mods     []*Modifier
	Enum     token.Pos
	Name     *Ident
	Generics *GenericParams
	Inherit  *InheritanceClause
	Where    *GenericWhereClause
	Body     *MemberBlock
}

// EnumCaseDecl is [AttributeList] [DeclarationModifiers] [indirect]
// case EnumCasePatternList.
type EnumCaseDecl struct {
	Span
	Attrs    []*Attr
	Mods     []*Modifier
	Indirect token.Pos // NoPos unless the case is indirect
	Case     token.Pos
	Elements []*EnumCaseElem
}

// EnumCaseElem is EnumCaseName [associated values] [RawValueAssignment].
//
// The associated values are declared, not matched: `case point(x:
// Int, y: Int)` declares two values of a type, and each may carry a
// default. That makes them a parameter list — the grammar writes
// TuplePattern there, which a declaration's parentheses cannot hold;
// see the parser's README.
type EnumCaseElem struct {
	Span
	Name   *Ident
	Lparen token.Pos // NoPos when the case carries no associated values
	Params []*Param
	Rparen token.Pos
	Assign token.Pos // NoPos without a raw value
	Value  Expr      // a NumericLiteral, StaticStringLiteral or BooleanLiteral
}

// StructDecl is a StructDeclaration.
type StructDecl struct {
	Span
	Attrs    []*Attr
	Mods     []*Modifier
	Struct   token.Pos
	Name     *Ident
	Generics *GenericParams
	Inherit  *InheritanceClause
	Where    *GenericWhereClause
	Body     *MemberBlock
}

// ClassDecl is a ClassDeclaration.
type ClassDecl struct {
	Span
	Attrs    []*Attr
	Mods     []*Modifier
	Class    token.Pos
	Name     *Ident
	Generics *GenericParams
	Inherit  *InheritanceClause
	Where    *GenericWhereClause
	Body     *MemberBlock
}

// ActorDecl is an ActorDeclaration. `actor` is contextual: a program
// may still name something after it.
type ActorDecl struct {
	Span
	Attrs    []*Attr
	Mods     []*Modifier
	Actor    token.Pos
	Name     *Ident
	Generics *GenericParams
	Inherit  *InheritanceClause
	Where    *GenericWhereClause
	Body     *MemberBlock
}

// ProtocolDecl is a ProtocolDeclaration. Its members are the
// requirement forms: a method or initializer with no body, a variable
// with a GetterSetterKeywordBlock, a subscript, an associated type, a
// typealias.
//
// Primary is the primary associated type list, the <Element> of
// `protocol Collection<Element>`. It names which of the protocol's
// associated types may be written as generic arguments where the
// protocol is used. The grammar has no such clause; see the parser's
// README.
type ProtocolDecl struct {
	Span
	Attrs    []*Attr
	Mods     []*Modifier
	Protocol token.Pos
	Name     *Ident
	Primary  *GenericParams
	Inherit  *InheritanceClause
	Where    *GenericWhereClause
	Body     *MemberBlock
}

// AssociatedTypeDecl is [AttributeList] [AccessLevelModifier]
// associatedtype TypealiasName [TypeInheritanceClause]
// [TypealiasAssignment] [GenericWhereClause].
type AssociatedTypeDecl struct {
	Span
	Attrs   []*Attr
	Mods    []*Modifier
	Keyword token.Pos
	Name    *Ident
	Inherit *InheritanceClause
	Assign  token.Pos // NoPos without a default
	Type    Type
	Where   *GenericWhereClause
}

// InitDecl is an InitializerDeclaration. Question or Exclaim marks a
// failable initializer. Body is nil in a protocol's requirement.
type InitDecl struct {
	Span
	Attrs    []*Attr
	Mods     []*Modifier
	Init     token.Pos
	Question token.Pos
	Exclaim  token.Pos
	Generics *GenericParams
	Sig      *FuncSig
	Where    *GenericWhereClause
	Body     *CodeBlock
}

// DeinitDecl is [AttributeList] [DeclarationModifiers] deinit CodeBlock.
type DeinitDecl struct {
	Span
	Attrs   []*Attr
	Mods    []*Modifier
	Keyword token.Pos
	Body    *CodeBlock
}

// ExtensionDecl is an ExtensionDeclaration.
type ExtensionDecl struct {
	Span
	Attrs     []*Attr
	Mods      []*Modifier
	Extension token.Pos
	Type      Type
	Inherit   *InheritanceClause
	Where     *GenericWhereClause
	Body      *MemberBlock
}

// SubscriptDecl is a SubscriptDeclaration. Body is the
// implicit-getter form; Accessors is every other one, and both are
// nil in a protocol's requirement written with a keyword block —
// which is an AccessorBlock whose accessors have no bodies.
type SubscriptDecl struct {
	Span
	Attrs     []*Attr
	Mods      []*Modifier
	Keyword   token.Pos
	Generics  *GenericParams
	Lparen    token.Pos
	Params    []*Param
	Rparen    token.Pos
	Result    *FuncResult
	Where     *GenericWhereClause
	Body      *CodeBlock
	Accessors *AccessorBlock
}

// OperatorDecl is [AttributeList] prefix|postfix|infix operator
// Operator [InfixOperatorGroup]. It declares that an operator exists
// and, for an infix one, which precedencegroup decides how it binds.
type OperatorDecl struct {
	Span
	Attrs    []*Attr
	Fixity   *Ident // prefix, postfix or infix — all contextual
	Operator token.Pos
	Name     *Ident // the operator's spelling, as an Ident over its span
	Colon    token.Pos
	Group    *Ident
}

// PrecedenceGroupDecl is precedencegroup PrecedenceGroupName '{'
// {PrecedenceGroupAttribute} '}'. Attrs holds *PrecedenceRelation,
// *PrecedenceAssignment and *PrecedenceAssociativity.
type PrecedenceGroupDecl struct {
	Span
	Keyword token.Pos
	Name    *Ident
	Lbrace  token.Pos
	Attrs   []Node
	Rbrace  token.Pos
}

// PrecedenceRelation is higherThan : Names or lowerThan : Names.
type PrecedenceRelation struct {
	Span
	Keyword *Ident
	Colon   token.Pos
	Names   []*Ident
}

// PrecedenceAssignment is assignment : BooleanLiteral.
type PrecedenceAssignment struct {
	Span
	Keyword *Ident
	Colon   token.Pos
	Value   *BasicLit
}

// PrecedenceAssociativity is associativity : left | right | none.
type PrecedenceAssociativity struct {
	Span
	Keyword *Ident
	Colon   token.Pos
	Value   *Ident
}

// MacroDecl is a MacroDeclaration: a function-shaped declaration
// whose body, if it has one, is the TypeIdentifier of the macro that
// implements it.
type MacroDecl struct {
	Span
	Attrs     []*Attr
	Mods      []*Modifier
	Keyword   token.Pos
	Name      *Ident
	Generics  *GenericParams
	Sig       *FuncSig
	Assign    token.Pos // NoPos when the declaration has no expansion
	Expansion Type
	Where     *GenericWhereClause
}

func (*BadDecl) declNode()             {}
func (*ImportDecl) declNode()          {}
func (*VarDecl) declNode()             {}
func (*TypealiasDecl) declNode()       {}
func (*FuncDecl) declNode()            {}
func (*EnumDecl) declNode()            {}
func (*EnumCaseDecl) declNode()        {}
func (*StructDecl) declNode()          {}
func (*ClassDecl) declNode()           {}
func (*ActorDecl) declNode()           {}
func (*ProtocolDecl) declNode()        {}
func (*AssociatedTypeDecl) declNode()  {}
func (*InitDecl) declNode()            {}
func (*DeinitDecl) declNode()          {}
func (*ExtensionDecl) declNode()       {}
func (*SubscriptDecl) declNode()       {}
func (*OperatorDecl) declNode()        {}
func (*PrecedenceGroupDecl) declNode() {}
func (*MacroDecl) declNode()           {}
