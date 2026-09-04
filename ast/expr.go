package ast

import "github.com/vertex-language/vsc/token"

// BadExpr covers tokens the parser gave up on. Its span is non-empty
// even when nothing was consumed.
type BadExpr struct {
	Span
}

// BasicLit is an undecoded literal with no structure: INT_LIT,
// FLOAT_LIT, REGEX_LIT, or one of the keyword literals TRUE, FALSE
// and NIL. Slice the span for the spelling.
//
// A NumericLiteral's leading '-' is not here. Only spacing tells `-1`
// from subtraction, and spacing is what the scanner already read, so
// a negative literal is a PrefixExpr over this node.
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

// StringLit is one string literal, opening delimiter to closing one.
//
// Segments holds *StringText and *Interpolation in written order, so
// "a\(b)c" is three of them. A literal with no interpolation has one
// segment, or none if it is empty. Pounds is the length of the
// ExtendedStringLiteralDelimiter, which decides what an escape is;
// Multiline says the delimiter was """, which decides how the text is
// stripped of its indentation. Both readings happen above this tree.
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

// IdentExpr is Identifier [GenericArgumentClause] in expression
// position — a reference to whatever the name denotes.
//
// Names holds the argument labels of a compound name — the `(_:)` of
// `#selector(tap(_:))` and of `map(f(_:))` — which names one
// declaration among the overloads rather than calling it.
type IdentExpr struct {
	Span
	Name   *Ident
	Args   *GenericArgs // nil when the reference carries none
	Lparen token.Pos    // NoPos without an argument-name list
	Names  []*ArgumentName
	Rparen token.Pos
}

// TypeExpr is a Type written where an expression goes.
//
// Most types can be read as expressions and are: `Int`, `[Int]` and
// `Int?` reach the analyzer as an IdentExpr, an ArrayLit and an
// OptionalExpr, and it is the analyzer that decides `[Int].self` names
// a metatype. A few spellings are types and nothing else — `any P`,
// `some P`, and a type written with attributes — and those arrive
// here, so that `[any Error]()` and `(@convention(c) (Int) -> Int)
// .self` parse as what they are.
type TypeExpr struct {
	Span
	Type Type
}

// StmtExpr is a statement standing where a value goes: the if and
// switch expressions of `let x = if c { a } else { b }`. Stmt is an
// *IfStmt or a *SwitchStmt.
//
// Which positions may hold one, and the rule that every branch must
// produce a value of the same type, are conditions on the value
// rather than on the syntax — so the parser reads one wherever an
// expression may begin and the analyzer decides whether it belongs.
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

// OperatorExpr is an operator written where an operand's neighbor
// goes: the operator of a PrefixExpr or PostfixExpr, an element of a
// SequenceExpr, or — the grammar admits this — an argument, as in
// `reduce(0, +)`.
//
// Kind is OPER_PREFIX, OPER_BINARY, OPER_POSTFIX, or one of the
// reserved operator kinds (ASSIGN and the rest). The spelling is the
// span, and what it means is a precedencegroup's business.
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

// SequenceExpr is PrefixExpression {BinaryExpression}: a flat,
// unfolded run of operands and the operators between them.
//
// It stays flat because it cannot be folded yet. Precedence and
// associativity are declared, not built in — a precedencegroup in
// some other file may be what decides how this expression groups —
// so the parser records the sequence and the analyzer, holding every
// declaration, resolves it into a tree.
//
// Elements holds the operands and the operators between them in
// written order. An operator element is an *OperatorExpr, a
// *TernaryExpr, or a *CastExpr — and a *CastExpr is the one that
// carries its own right operand, because `as Type` takes a type and
// not an expression, so nothing follows it.
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

// TernaryExpr is the ConditionalOperator `? Expression :` as it
// appears in a sequence: the operator and its middle operand, with
// neither of the two operands the fold will attach.
type TernaryExpr struct {
	Span
	Question token.Pos
	Then     Expr
	Colon    token.Pos
}

// CastExpr is a TypeCastingOperator as it appears in a sequence: `is
// Type`, `as Type`, `as? Type`, `as! Type`. Kind is IS or AS, and
// Question or Exclaim marks which of the two `as` forms was written.
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

// ConsumeExpr, CopyExpr and BorrowExpr are the ownership operators.
// They are separate nodes because they do separate things — one ends
// a value's lifetime, one duplicates it, one reads it in place — and
// because their keywords are contextual: `consume` is an ordinary
// name everywhere the operator is not.
type ConsumeExpr struct {
	Span
	Keyword token.Pos
	X       Expr
}

type CopyExpr struct {
	Span
	Keyword token.Pos
	X       Expr
}

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

// CaptureItem is [CaptureSpecifier] Expression. The specifier is
// weak, unowned, unowned(safe) or unowned(unsafe) — the same shape as
// a declaration modifier, so it is one.
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

// ClosureParam is [ClosureParameterName] [TypeAnnotation].
//
// Label is the first of two names, as in `{ (_ fn: () -> Void) in … }`
// — the grammar gives a closure's parameter one name; see the
// parser's README.
type ClosureParam struct {
	Span
	Label *Ident
	Name  *Ident
	Colon token.Pos // NoPos without an annotation
	Mods  []*Modifier
	Type  Type
}

// CallExpr is PostfixExpression FunctionCallArgumentClause
// [TrailingClosures], or a call written with trailing closures alone
// (Args nil).
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

// CallArg is Expression, or Identifier : Expression. An argument that
// is a bare Operator — `reduce(0, +)` — has that operator as its X.
type CallArg struct {
	Span
	Label *Ident
	Colon token.Pos
	X     Expr
}

// TrailingClosure is a closure written after a call's parens. The
// first one is unlabeled; the rest carry a label.
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

// MemberExpr is . Identifier [GenericArgumentClause] or
// . Identifier ( ArgumentNames ) — the ExplicitMemberExpression. A
// member named by its argument labels, `f(x:y:)`, fills Names.
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

// InitRefExpr is . init — the InitializerArgumentClause, and the
// `self.init` and `super.init` of the Self and Superclass
// expressions. A call's arguments hang off it as a CallExpr; the
// Names here are the other form, `String.init(describing:)`, which
// names one initializer among the overloads rather than calling it.
type InitRefExpr struct {
	Span
	X      Expr
	Dot    token.Pos
	Init   token.Pos
	Lparen token.Pos // NoPos without an argument-name list
	Names  []*ArgumentName
	Rparen token.Pos
}

// PostfixSelfExpr is PostfixExpression . self — the value itself,
// written where a metatype would otherwise be read.
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
