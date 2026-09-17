package ast

import "github.com/vertex-language/vsc/token"

// BadStmt covers a statement the parser gave up on.
type BadStmt struct {
	Span
}

// ExprStmt is an Expression in statement position.
type ExprStmt struct {
	Span
	X Expr
}

// DeclStmt adapts a Decl into a statement. A declaration is a
// statement in this grammar, at the top level and inside a body
// alike.
type DeclStmt struct {
	Span
	D Decl
}

// EmptyStmt keeps a stray semicolon visible.
type EmptyStmt struct {
	Span
	Semi token.Pos
}

// CodeBlock is '{' [Statements] '}'.
type CodeBlock struct {
	Span
	Lbrace token.Pos
	Stmts  []Stmt
	Rbrace token.Pos
}

// ForInStmt is for [try] [await] [case] Pattern in Expression
// [WhereClause] CodeBlock.
type ForInStmt struct {
	Span
	For   token.Pos
	Try   token.Pos // NoPos unless a throwing sequence
	Await token.Pos // NoPos unless an async sequence
	Case  token.Pos // NoPos unless the pattern is a matching one
	Pat   Pattern
	In    token.Pos
	Seq   Expr
	Where *WhereClause
	Body  *CodeBlock
}

// WhereClause represents a `where` condition clause on loops, cases, or catches.
type WhereClause struct {
	Span
	Where token.Pos
	Cond  Expr
}

// WhileStmt is while ConditionList CodeBlock.
type WhileStmt struct {
	Span
	While token.Pos
	Conds []Node // Expr, *AvailabilityCond, *CaseCond, or *OptionalBinding
	Body  *CodeBlock
}

// RepeatWhileStmt is repeat CodeBlock while Expression.
type RepeatWhileStmt struct {
	Span
	Repeat token.Pos
	Body   *CodeBlock
	While  token.Pos
	Cond   Expr
}

// IfStmt is if ConditionList CodeBlock [ElseClause]. The else clause
// holds a *CodeBlock or the *IfStmt of an `else if`.
type IfStmt struct {
	Span
	If      token.Pos
	Conds   []Node
	Body    *CodeBlock
	ElsePos token.Pos // NoPos when there is no else
	Else    Stmt
}

// GuardStmt is guard ConditionList else CodeBlock.
type GuardStmt struct {
	Span
	Guard   token.Pos
	Conds   []Node
	ElsePos token.Pos
	Body    *CodeBlock
}

// SwitchStmt is switch Expression '{' {SwitchCase} '}'. Cases holds
// *CaseClause and the *IfConfigStmt of a conditional case.
type SwitchStmt struct {
	Span
	Switch  token.Pos
	Subject Expr
	Lbrace  token.Pos
	Cases   []Stmt
	Rbrace  token.Pos
}

// CaseClause is one CaseLabel or DefaultLabel and the statements
// under it. Kind is CASE or DEFAULT; a default label has no items.
type CaseClause struct {
	Span
	Attrs   []*Attr
	Keyword token.Pos
	Kind    token.Kind
	Items   []*CaseItem
	Colon   token.Pos
	Stmts   []Stmt
}

// CaseItem is Pattern [WhereClause].
type CaseItem struct {
	Span
	Pat   Pattern
	Where *WhereClause
}

// LabeledStmt is StatementLabel Statement. The grammar labels only
// loops, if, switch and do; enforcing that is a check, not a shape.
type LabeledStmt struct {
	Span
	Label *Ident
	Colon token.Pos
	Stmt  Stmt
}

// BreakStmt is break [LabelName].
type BreakStmt struct {
	Span
	Break token.Pos
	Label *Ident
}

// ContinueStmt is continue [LabelName].
type ContinueStmt struct {
	Span
	Continue token.Pos
	Label    *Ident
}

// FallthroughStmt is fallthrough.
type FallthroughStmt struct {
	Span
	Keyword token.Pos
}

// ReturnStmt is return [Expression].
type ReturnStmt struct {
	Span
	Return token.Pos
	X      Expr
}

// ThrowStmt is throw Expression.
type ThrowStmt struct {
	Span
	Throw token.Pos
	X     Expr
}

// DiscardStmt is discard Expression.
type DiscardStmt struct {
	Span
	Keyword token.Pos
	X       Expr
}

// YieldStmt represents a `yield` statement in accessor bodies (such as _read or _modify).
type YieldStmt struct {
	Span
	Keyword token.Pos
	Amp     token.Pos // the `&` of `yield &x`, NoPos if absent
	X       Expr
}

// DeferStmt is defer CodeBlock.
type DeferStmt struct {
	Span
	Defer token.Pos
	Body  *CodeBlock
}

// DoStmt is do [ThrowsClause] CodeBlock {CatchClause}.
type DoStmt struct {
	Span
	Do      token.Pos
	Throws  *ThrowsClause
	Body    *CodeBlock
	Catches []*CatchClause
}

// CatchClause is catch [CatchPatternList] CodeBlock. A bare `catch`
// has no items and binds the error to `error`.
type CatchClause struct {
	Span
	Catch token.Pos
	Items []*CaseItem // CatchPattern is Pattern [WhereClause]
	Body  *CodeBlock
}

// ---- conditions ----

// AvailabilityCond is #available ( … ) or #unavailable ( … ); Kind
// says which.
type AvailabilityCond struct {
	Span
	Pound  token.Pos
	Kind   token.Kind
	Lparen token.Pos
	Args   []*AvailabilityArg
	Rparen token.Pos
}

// AvailabilityArg is PlatformName PlatformVersion, or the `*` that
// stands for every other platform.
type AvailabilityArg struct {
	Span
	Platform *Ident      // nil for the wildcard
	Version  *VersionLit // nil for the wildcard, and for a bare platform name
	Star     token.Pos   // NoPos unless the wildcard
}

// VersionLit represents a dotted version literal (e.g. `15.1`, `5.9.2`).
type VersionLit struct {
	Span
}

// CaseCond is case Pattern Initializer, in a condition list.
type CaseCond struct {
	Span
	Case   token.Pos
	Pat    Pattern
	Assign token.Pos
	Value  Expr
}

// OptionalBinding is let Pattern [Initializer] or var Pattern
// [Initializer]; Kind says which. The shorthand `if let x` has no
// initializer, and binds the name to itself.
type OptionalBinding struct {
	Span
	Keyword token.Pos
	Kind    token.Kind
	Pat     Pattern
	Assign  token.Pos
	Value   Expr
}

// ---- compiler control statements ----

// IfConfigStmt represents a conditional compilation block (`#if`, `#elseif`, `#else`, `#endif`).
type IfConfigStmt struct {
	Span
	Clauses []*IfConfigClause
	Endif   token.Pos
}

// IfConfigClause is one #if, #elseif, or #else and the statements
// under it. Kind is the POUND_* kind; Cond is nil under #else.
type IfConfigClause struct {
	Span
	Pound token.Pos
	Kind  token.Kind
	Cond  Expr
	Stmts []Stmt
}

// PlatformCond represents a platform or compiler condition query (e.g. os(), arch(), canImport(), swift()).
type PlatformCond struct {
	Span
	Name     *Ident
	Lparen   token.Pos
	Op       token.Pos   // the >= or < of swift() and compiler(), else NoPos
	OpKind   token.Kind  // OPER_* or OPER_BINARY spelling of that operator
	Arg      *Ident      // the operating system, architecture, environment, or attribute
	Path     []*Ident    // canImport's dotted ImportPath
	VerLabel *Ident      // canImport's _version or _underlyingVersion
	Ver      *VersionLit // a version written as digits and dots
	VerStr   *StringLit  // a version written as a string
	Rparen   token.Pos
}

// SourceLocationStmt is #sourceLocation ( file : … , line : … ) or
// the argumentless #sourceLocation ( ) that returns to the real one.
type SourceLocationStmt struct {
	Span
	Pound  token.Pos
	Lparen token.Pos
	File   *StringLit // nil in the reset form
	Line   *BasicLit  // nil in the reset form
	Rparen token.Pos
}

// DiagnosticStmt is #error ( StringLiteral ) or #warning ( … ); Kind
// says which.
type DiagnosticStmt struct {
	Span
	Pound   token.Pos
	Kind    token.Kind
	Lparen  token.Pos
	Message *StringLit
	Rparen  token.Pos
}

func (*BadStmt) stmtNode()            {}
func (*ExprStmt) stmtNode()           {}
func (*DeclStmt) stmtNode()           {}
func (*EmptyStmt) stmtNode()          {}
func (*CodeBlock) stmtNode()          {}
func (*ForInStmt) stmtNode()          {}
func (*WhileStmt) stmtNode()          {}
func (*RepeatWhileStmt) stmtNode()    {}
func (*IfStmt) stmtNode()             {}
func (*GuardStmt) stmtNode()          {}
func (*SwitchStmt) stmtNode()         {}
func (*CaseClause) stmtNode()         {}
func (*LabeledStmt) stmtNode()        {}
func (*BreakStmt) stmtNode()          {}
func (*ContinueStmt) stmtNode()       {}
func (*FallthroughStmt) stmtNode()    {}
func (*ReturnStmt) stmtNode()         {}
func (*ThrowStmt) stmtNode()          {}
func (*DiscardStmt) stmtNode()        {}
func (*DeferStmt) stmtNode()          {}
func (*YieldStmt) stmtNode()          {}
func (*DoStmt) stmtNode()             {}
func (*IfConfigStmt) stmtNode()       {}
func (*SourceLocationStmt) stmtNode() {}
func (*DiagnosticStmt) stmtNode()     {}

// PlatformCond is an Expr because a CompilationCondition is built
// from ordinary expression syntax — !, &&, ||, parentheses — around
// these leaves.
func (*PlatformCond) exprNode() {}

// VersionLit is an Expr for the same reason: it is a leaf of a
// compilation condition.
func (*VersionLit) exprNode() {}
