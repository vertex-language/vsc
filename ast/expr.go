package ast

import "github.com/vertex-language/vsc/token"

// BadExpr covers tokens the parser gave up on. Its span is non-empty
// even when nothing was consumed.
type BadExpr struct {
	Span
}

// BasicLit represents an undecoded scalar or keyword literal (INT, FLOAT,
// REGEX, TRUE, FALSE, NIL). Negative numeric literals are represented
// as a PrefixExpr over BasicLit.
type BasicLit struct {
	Span
	Kind token.Kind
}

// MagicLit is one of the compile-time literals that name their own
// source location: #file, #fileID, #filePath, #line, #column,
// #function, #dsohandle. Kind is the POUND_* kind.
type MagicLit struct {
	Span
	Kind token.Kind
}

// StringLit represents a single string literal. Segments contains *StringText
// and *Interpolation entries in source order. Pounds records the number of
// '#' characters in raw string delimiters.
type StringLit struct {
	Span
	Open      token.Pos
	Segments  []Node
	Close     token.Pos
	Pounds    int
	Multiline bool
}

// StringText is one undecoded run of a string literal's own text.
type StringText struct {
	Span
}

// Interpolation is \( Expression ) or \( [Label:] Expression, ... ) inside a string literal.
type Interpolation struct {
	Span
	Backslash token.Pos
	Lparen    token.Pos
	X         Expr       // non-nil when single unlabeled expression
	Args      []*CallArg // all arguments including labels
	Rparen    token.Pos
}

// IdentExpr represents an identifier or compound name reference in expression position.
type IdentExpr struct {
	Span
	Name   *Ident
	Args   *GenericArgs // nil when the reference carries none
	Lparen token.Pos    // NoPos without an argument-name list
	Names  []*ArgumentName
	Rparen token.Pos
}

// TypeExpr represents an explicit Type written in expression position (e.g. `any P`, `some P`).
type TypeExpr struct {
	Span
	Type Type
}

// StmtExpr represents an if or switch statement used in expression position.
type StmtExpr struct {
	Span
	Stmt Stmt
}

// SelfExpr is the bare `self`; a member of it is a MemberExpr over
// this node.
type SelfExpr struct {
	Span
}

// SuperExpr is the bare `super`.
type SuperExpr struct {
	Span
}

// WildcardExpr is `_` in expression position.
type WildcardExpr struct {
	Span
}

// OperatorExpr represents an operator reference in operand, prefix, postfix, or sequence position.
type OperatorExpr struct {
	Span
	Kind token.Kind
}

// PrefixExpr is PrefixOperator PostfixExpression.
type PrefixExpr struct {
	Span
	Op *OperatorExpr
	X  Expr
}

// PostfixExpr is PostfixExpression PostfixOperator.
type PostfixExpr struct {
	Span
	X  Expr
	Op *OperatorExpr
}

// SequenceExpr represents an unfolded flat sequence of operands and operators,
// resolved into a tree during semantic analysis based on operator precedence.
type SequenceExpr struct {
	Span
	Elements []Expr
}

// BinaryExpr is a folded binary operator application: X Op Y.
type BinaryExpr struct {
	Span
	X  Expr
	Op *OperatorExpr
	Y  Expr
}

// ConditionalExpr is a folded ternary conditional: Cond ? Then : Else.
type ConditionalExpr struct {
	Span
	Cond     Expr
	Question token.Pos
	Then     Expr
	Colon    token.Pos
	Else     Expr
}

// TernaryExpr represents a `? Then :` operator slice within an unfolded SequenceExpr.
type TernaryExpr struct {
	Span
	Question token.Pos
	Then     Expr
	Colon    token.Pos
}

// CastExpr represents a type casting operator (`is`, `as`, `as?`, `as!`).
type CastExpr struct {
	Span
	X        Expr // nil when in flat SequenceExpr; set when folded
	Keyword  token.Pos
	Kind     token.Kind
	Question token.Pos
	Exclaim  token.Pos
	Type     Type
}

// TryExpr is try, try ?, or try ! applied to the rest of the
// expression. Question and Exclaim are NoPos in the plain form.
type TryExpr struct {
	Span
	Try      token.Pos
	Question token.Pos
	Exclaim  token.Pos
	X        Expr
}

// AwaitExpr is await Expression.
type AwaitExpr struct {
	Span
	Await token.Pos
	X     Expr
}

// InOutExpr is & Expression.
type InOutExpr struct {
	Span
	Amp token.Pos
	X   Expr
}

// ConsumeExpr represents the ownership operator `consume X`.
type ConsumeExpr struct {
	Span
	Keyword token.Pos
	X       Expr
}

// CopyExpr represents the ownership operator `copy X`.
type CopyExpr struct {
	Span
	Keyword token.Pos
	X       Expr
}

// BorrowExpr represents the ownership operator `borrow X`.
type BorrowExpr struct {
	Span
	Keyword token.Pos
	X       Expr
}

// PackExpansionExpr is repeat Expression.
type PackExpansionExpr struct {
	Span
	Repeat token.Pos
	X      Expr
}

// PackReferenceExpr is each Expression (also known as a pack element expression).
type PackReferenceExpr struct {
	Span
	Each token.Pos
	X    Expr
}

// ParenExpr is ( Expression ).
type ParenExpr struct {
	Span
	Lparen token.Pos
	X      Expr
	Rparen token.Pos
}

// TupleExpr is ( TupleElement {, TupleElement} ).
type TupleExpr struct {
	Span
	Lparen token.Pos
	Elems  []*TupleElem
	Rparen token.Pos
}

// TupleElem is [Identifier :] Expression.
type TupleElem struct {
	Span
	Label *Ident
	Colon token.Pos
	X     Expr
}

// ArrayLit is '[' [ArrayLiteralItems] ']'. Comma records a trailing
// comma's position, NoPos if absent.
type ArrayLit struct {
	Span
	Lsquare token.Pos
	Items   []Expr
	Comma   token.Pos
	Rsquare token.Pos
}

// DictLit is '[' DictionaryLiteralItems ']' or the empty '[' : ']'.
// In the empty form Items is nil and Colon is the colon written
// between the brackets.
type DictLit struct {
	Span
	Lsquare token.Pos
	Items   []*DictLitItem
	Colon   token.Pos
	Comma   token.Pos
	Rsquare token.Pos
}

// DictLitItem is Expression : Expression.
type DictLitItem struct {
	Span
	Key   Expr
	Colon token.Pos
	Value Expr
}

// PlaygroundLit is #colorLiteral, #fileLiteral, or #imageLiteral with
// its labeled arguments. Kind is the POUND_* kind.
type PlaygroundLit struct {
	Span
	Pound token.Pos
	Kind  token.Kind
	Args  *CallArgs
}

// ClosureExpr is '{' [AttributeList] [ClosureSignature] [Statements]
// '}'.
type ClosureExpr struct {
	Span
	Lbrace token.Pos
	Attrs  []*Attr
	Sig    *ClosureSig // nil when the closure takes its arguments as $0, $1, …
	Stmts  []Stmt
	Rbrace token.Pos
}

// ClosureSig is everything a closure writes before `in`.
type ClosureSig struct {
	Span
	Captures *CaptureList
	Params   *ClosureParams
	Async    token.Pos
	Throws   *ThrowsClause
	Result   *FuncResult
	In       token.Pos
}

// CaptureList is '[' CaptureListItems ']'.
type CaptureList struct {
	Span
	Lsquare token.Pos
	Items   []*CaptureItem
	Rsquare token.Pos
}

// CaptureItem represents a single capture list item with an optional specifier (`weak`, `unowned`).
type CaptureItem struct {
	Span
	Spec *Modifier // nil for a plain capture
	X    Expr
}

// ClosureParams is a closure's parameter clause. Lparen is NoPos for
// the bare IdentifierList form, `{ a, b in … }`.
type ClosureParams struct {
	Span
	Lparen token.Pos
	Params []*ClosureParam
	Rparen token.Pos
}

// ClosureParam represents a single closure parameter with optional label, name, and type annotation.
type ClosureParam struct {
	Span
	Label *Ident
	Name  *Ident
	Colon token.Pos // NoPos without an annotation
	Mods  []*Modifier
	Type  Type
}

// CallExpr represents a function or method invocation with arguments and trailing closures.
type CallExpr struct {
	Span
	Fun      Expr
	Args     *CallArgs
	Trailing []*TrailingClosure
}

// CallArgs is ( [FunctionCallArgumentList] ).
type CallArgs struct {
	Span
	Lparen token.Pos
	Args   []*CallArg
	Rparen token.Pos
}

// CallArg represents a positional or labeled call argument.
type CallArg struct {
	Span
	Label *Ident
	Colon token.Pos
	X     Expr
}

// TrailingClosure represents a trailing closure attached to a call.
type TrailingClosure struct {
	Span
	Label   *Ident
	Colon   token.Pos
	Closure *ClosureExpr
}

// SubscriptExpr is PostfixExpression '[' FunctionCallArgumentList ']'.
type SubscriptExpr struct {
	Span
	X       Expr
	Lsquare token.Pos
	Args    []*CallArg
	Rsquare token.Pos
}

// MemberExpr represents an explicit member access (`x.y` or `x.f(a:b:)`).
type MemberExpr struct {
	Span
	X      Expr
	Dot    token.Pos
	Name   *Ident
	Args   *GenericArgs
	Lparen token.Pos // NoPos unless argument names were written
	Names  []*ArgumentName
	Rparen token.Pos
}

// ArgumentName is one `label:` of an ArgumentNames list.
type ArgumentName struct {
	Span
	Name  *Ident
	Colon token.Pos
}

// ImplicitMemberExpr is a leading `. Identifier`, whose base is the
// type the context expects.
type ImplicitMemberExpr struct {
	Span
	Dot  token.Pos
	Name *Ident
	Args *GenericArgs
}

// InitRefExpr represents a `.init` reference expression (e.g. `self.init`, `T.init(x:)`).
type InitRefExpr struct {
	Span
	X      Expr
	Dot    token.Pos
	Init   token.Pos
	Lparen token.Pos // NoPos without an argument-name list
	Names  []*ArgumentName
	Rparen token.Pos
}

// PostfixSelfExpr represents `X.self`.
type PostfixSelfExpr struct {
	Span
	X    Expr
	Dot  token.Pos
	Self token.Pos
}

// ForceExpr is PostfixExpression ! — the ForcedValueExpression.
type ForceExpr struct {
	Span
	X       Expr
	Exclaim token.Pos
}

// OptionalExpr is PostfixExpression ? — the OptionalChainingExpression.
type OptionalExpr struct {
	Span
	X        Expr
	Question token.Pos
}

// KeyPathExpr is \ [Type] [.] KeyPathComponents, or the subscript
// form \ [Type] '[' … ']' [KeyPathComponents].
type KeyPathExpr struct {
	Span
	Backslash  token.Pos
	Type       Type // nil when the root type is inferred
	Components []*KeyPathComponent
}

// KeyPathComponent is one step of a key path: a name, a subscript, a
// `?`, a `!`, or `self`. Exactly one of the fields is set.
type KeyPathComponent struct {
	Span
	Dot      token.Pos // NoPos on the first component after a subscript
	Name     *Ident
	Args     *CallArgs      // a call on the named component
	Sub      *SubscriptExpr // the '[' … ']' form; its X is nil
	Question token.Pos
	Exclaim  token.Pos
	Self     token.Pos
}

// SelectorExpr is #selector ( Expression ), or its getter: and
// setter: forms — Label holds the one that was written.
type SelectorExpr struct {
	Span
	Pound  token.Pos
	Lparen token.Pos
	Label  *Ident
	Colon  token.Pos
	X      Expr
	Rparen token.Pos
}

// KeyPathStringExpr is #keyPath ( Expression ).
type KeyPathStringExpr struct {
	Span
	Pound  token.Pos
	Lparen token.Pos
	X      Expr
	Rparen token.Pos
}

// MacroExpansionExpr is # Identifier [GenericArgumentClause]
// [FunctionCallArgumentClause] [TrailingClosures].
type MacroExpansionExpr struct {
	Span
	Pound    token.Pos
	Name     *Ident
	Generics *GenericArgs
	Args     *CallArgs
	Trailing []*TrailingClosure
}

func (*BadExpr) exprNode()            {}
func (*BasicLit) exprNode()           {}
func (*MagicLit) exprNode()           {}
func (*StringLit) exprNode()          {}
func (*IdentExpr) exprNode()          {}
func (*TypeExpr) exprNode()           {}
func (*StmtExpr) exprNode()           {}
func (*SelfExpr) exprNode()           {}
func (*SuperExpr) exprNode()          {}
func (*WildcardExpr) exprNode()       {}
func (*OperatorExpr) exprNode()       {}
func (*PrefixExpr) exprNode()         {}
func (*PostfixExpr) exprNode()        {}
func (*SequenceExpr) exprNode()       {}
func (*BinaryExpr) exprNode()         {}
func (*ConditionalExpr) exprNode()    {}
func (*TernaryExpr) exprNode()        {}
func (*CastExpr) exprNode()           {}
func (*TryExpr) exprNode()            {}
func (*AwaitExpr) exprNode()          {}
func (*InOutExpr) exprNode()          {}
func (*ConsumeExpr) exprNode()        {}
func (*CopyExpr) exprNode()           {}
func (*BorrowExpr) exprNode()         {}
func (*PackExpansionExpr) exprNode()  {}
func (*PackReferenceExpr) exprNode()  {}
func (*ParenExpr) exprNode()          {}
func (*TupleExpr) exprNode()          {}
func (*ArrayLit) exprNode()           {}
func (*DictLit) exprNode()            {}
func (*PlaygroundLit) exprNode()      {}
func (*ClosureExpr) exprNode()        {}
func (*CallExpr) exprNode()           {}
func (*SubscriptExpr) exprNode()      {}
func (*MemberExpr) exprNode()         {}
func (*ImplicitMemberExpr) exprNode() {}
func (*InitRefExpr) exprNode()        {}
func (*PostfixSelfExpr) exprNode()    {}
func (*ForceExpr) exprNode()          {}
func (*OptionalExpr) exprNode()       {}
func (*KeyPathExpr) exprNode()        {}
func (*SelectorExpr) exprNode()       {}
func (*KeyPathStringExpr) exprNode()  {}
func (*MacroExpansionExpr) exprNode() {}
