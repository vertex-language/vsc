package ast

import "github.com/vertex-language/vsc/token"

// Attr represents an attribute annotation (@AttributeName [arguments]).
type Attr struct {
	Span
	At     token.Pos
	Name   Type      // an Identifier or a TypeIdentifier
	Lparen token.Pos // NoPos when the attribute takes no arguments
	Tokens []token.Token
	Rparen token.Pos
}

// Modifier represents a declaration modifier (e.g. private(set), static, mutating).
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

// ImportDecl represents an import declaration.
type ImportDecl struct {
	Span
	Attrs   []*Attr
	Mods    []*Modifier
	Import  token.Pos
	Kind    token.Kind
	KindPos token.Pos
	Path    []*Ident
	// Paths holds string-form import paths (e.g. `import "std/fmt"`).
	Paths  []*ImportPath
	Lparen token.Pos
	Rparen token.Pos
}

// ImportPath represents a string-form import path with an optional alias.
type ImportPath struct {
	Span
	Alias *Ident
	Path  *StringLit
}

// PackageDecl represents a top-level `package Name` declaration.
type PackageDecl struct {
	Span
	Keyword token.Pos
	Name    *Ident
}

// VarDecl represents a `let` or `var` declaration.
type VarDecl struct {
	Span
	Attrs    []*Attr
	Mods     []*Modifier
	Keyword  token.Pos
	Kind     token.Kind
	Bindings []*PatternBinding
}

// PatternBinding represents a pattern binding, its optional initializer, and accessors.
type PatternBinding struct {
	Span
	Pat       Pattern
	Assign    token.Pos // NoPos when the binding has no initializer
	Value     Expr
	Body      *CodeBlock
	Accessors *AccessorBlock
}

// AccessorBlock is '{' … '}' holding get, set, willSet and didSet clauses.
type AccessorBlock struct {
	Span
	Lbrace    token.Pos
	Accessors []*Accessor
	Rbrace    token.Pos
}

// Accessor represents a get, set, willSet, or didSet clause.
type Accessor struct {
	Span
	Attrs   []*Attr
	Mods    []*Modifier
	Keyword *Ident // get, set, willSet, didSet
	Lparen  token.Pos
	Name    *Ident
	Rparen  token.Pos
	Async   token.Pos
	Throws  *ThrowsClause
	Body    *CodeBlock
}

// TypealiasDecl represents a typealias declaration.
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

// FuncDecl represents a function or method declaration.
type FuncDecl struct {
	Span
	Attrs    []*Attr
	Mods     []*Modifier
	Func     token.Pos
	Recv     *Receiver // optional receiver clause for method declared outside type
	Name     *Ident
	Generics *GenericParams
	Sig      *FuncSig
	Where    *GenericWhereClause
	Body     *CodeBlock
}

// Receiver represents a receiver clause `(v: borrowing Type)` on a function declaration.
type Receiver struct {
	Span
	Lparen token.Pos
	Name   *Ident
	Colon  token.Pos
	Mods   []*Modifier // ownership modifier (borrowing, inout, consuming)
	Type   Type
	Rparen token.Pos
}

// An ExecKind is a Vertex execution modifier: where a function is
// meant to run, as opposed to what it does. ExecNone is the zero
// value and means an ordinary function, which is every function
// Swift can write.
type ExecKind int

const (
	// ExecNone is an ordinary function.
	ExecNone ExecKind = iota
	// ExecKernel is `kernel`: a data-parallel unit, one thread per
	// element.
	ExecKernel
	// ExecGraph is `graph`: a node in a dataflow graph, traced rather
	// than executed.
	ExecGraph
)

// String is the modifier as it is written.
func (k ExecKind) String() string {
	switch k {
	case ExecKernel:
		return "kernel"
	case ExecGraph:
		return "graph"
	}
	return ""
}

// FuncSig is ( [ParameterList] ) [async] [ThrowsClause] [ExecKind]
// [FunctionResult]. Throws covers rethrows.
type FuncSig struct {
	Span
	Lparen token.Pos
	Params []*Param
	Rparen token.Pos
	Async  token.Pos
	Throws *ThrowsClause
	// Exec is the execution modifier, ExecNone where there is none.
	// ExecPos is where it was written, so a diagnostic can point at
	// the word rather than at the declaration.
	Exec    ExecKind
	ExecPos token.Pos
	Result  *FuncResult
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

// EnumCaseElem represents an enum case with optional associated values and raw value assignment.
type EnumCaseElem struct {
	Span
	Name   *Ident
	Lparen token.Pos // NoPos when the case carries no associated values
	Params []*Param
	Rparen token.Pos
	Assign token.Pos // NoPos without a raw value
	Value  Expr      // a NumericLiteral, StaticStringLiteral or BooleanLiteral
}

// StructDecl represents a struct declaration.
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

// ClassDecl represents a class declaration.
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

// ActorDecl represents an actor declaration.
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

// ProtocolDecl represents a protocol declaration.
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

// AssociatedTypeDecl represents an associatedtype declaration in a protocol.
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

// InitDecl represents an initializer declaration.
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

// DeinitDecl represents a deinitializer declaration.
type DeinitDecl struct {
	Span
	Attrs   []*Attr
	Mods    []*Modifier
	Keyword token.Pos
	Body    *CodeBlock
}

// ExtensionDecl represents an extension declaration.
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

// SubscriptDecl represents a subscript declaration.
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

// OperatorDecl represents an operator declaration.
type OperatorDecl struct {
	Span
	Attrs    []*Attr
	Fixity   *Ident // prefix, postfix or infix
	Operator token.Pos
	Name     *Ident
	Colon    token.Pos
	Group    *Ident
}

// PrecedenceGroupDecl represents a precedencegroup declaration.
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

// MacroDecl represents a macro declaration.
type MacroDecl struct {
	Span
	Attrs     []*Attr
	Mods      []*Modifier
	Keyword   token.Pos
	Name      *Ident
	Generics  *GenericParams
	Sig       *FuncSig
	Assign    token.Pos
	Expansion Type
	Where     *GenericWhereClause
}

func (*BadDecl) declNode()             {}
func (*ImportDecl) declNode()          {}
func (*PackageDecl) declNode()         {}
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
