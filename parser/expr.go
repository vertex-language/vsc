package parser

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// exprFlags configures contextual constraints during expression parsing.
type exprFlags uint

const (
	// exprBasic prevents '{' from being parsed as a trailing closure (used in if/switch/for headers).
	exprBasic exprFlags = 1 << iota

	// exprNoAssign prevents '=' from being consumed as an assignment operator.
	exprNoAssign
)

// parseExpr parses an expression, including prefix effect operators and operator sequences.
func (p *parser) parseExpr(fl exprFlags) ast.Expr {
	p.depth++
	defer func() { p.depth-- }()
	lo := p.pos()
	if p.tooDeep() {
		return &ast.BadExpr{Span: p.span(lo)}
	}

	switch {
	case p.at(token.TRY):
		t := &ast.TryExpr{Try: p.pos()}
		p.next()
		switch {
		case p.at(token.QUESTION_POSTFIX) || p.at(token.QUESTION_INFIX):
			t.Question = p.pos()
			p.next()
		case p.at(token.EXCLAIM_POSTFIX):
			t.Exclaim = p.pos()
			p.next()
		}
		t.X = p.parseExpr(fl)
		t.Span = p.span(lo)
		return t

	case p.atWord("await") && p.atExprStartAt(1):
		kw := p.pos()
		p.next()
		x := p.parseExpr(fl)
		return &ast.AwaitExpr{Span: p.span(lo), Await: kw, X: x}

	case p.at(token.REPEAT):
		kw := p.pos()
		p.next()
		x := p.parseExpr(fl)
		return &ast.PackExpansionExpr{Span: p.span(lo), Repeat: kw, X: x}
	}
	return p.parseSequence(fl)
}

// parseSequence parses a flat sequence of prefix expressions and binary operators for later folding.
func (p *parser) parseSequence(fl exprFlags) ast.Expr {
	lo := p.pos()
	first := p.parsePrefixExpr(fl)
	var elems []ast.Expr

	for {
		op := p.parseBinaryOp(fl)
		if op == nil {
			break
		}
		if elems == nil {
			elems = []ast.Expr{first}
		}
		elems = append(elems, op)
		if _, isCast := op.(*ast.CastExpr); isCast {
			continue // `as Type` brought its own right operand
		}
		// Assignments and ternary branches parse following expressions as full sub-expressions.
		whole := false
		switch o := op.(type) {
		case *ast.OperatorExpr:
			whole = o.Kind == token.ASSIGN
		case *ast.TernaryExpr:
			whole = true
		}
		if whole || p.atEffectOperator() {
			elems = append(elems, p.parseExpr(fl))
			continue
		}
		elems = append(elems, p.parsePrefixExpr(fl))
	}
	if elems == nil {
		return first
	}
	return &ast.SequenceExpr{Span: p.span(lo), Elements: elems}
}

// atEffectOperator reports whether an operator that takes the whole
// of what follows it begins here.
func (p *parser) atEffectOperator() bool {
	return p.at(token.TRY) ||
		((p.atWord("await") || p.at(token.REPEAT)) && p.atExprStartAt(1))
}

// parseBinaryOp reads one BinaryExpression's operator, or returns nil
// where the sequence ends.
func (p *parser) parseBinaryOp(fl exprFlags) ast.Expr {
	lo := p.pos()
	switch {
	case p.at(token.ASSIGN):
		if fl&exprNoAssign != 0 {
			return nil
		}
		return p.oper()

	case p.at(token.QUESTION_INFIX):
		t := &ast.TernaryExpr{Question: p.pos()}
		p.next()
		t.Then = p.parseExpr(fl)
		t.Colon = p.expect(token.COLON)
		t.Span = p.span(lo)
		return t

	case p.at(token.IS), p.at(token.AS):
		c := &ast.CastExpr{Keyword: p.pos(), Kind: p.kind()}
		p.next()
		if c.Kind == token.AS {
			switch {
			case p.at(token.QUESTION_POSTFIX) || p.at(token.QUESTION_INFIX):
				c.Question = p.pos()
				p.next()
			case p.at(token.EXCLAIM_POSTFIX):
				c.Exclaim = p.pos()
				p.next()
			}
		}
		c.Type = p.parseType()
		c.Span = p.span(lo)
		return c

	case p.kind() == token.OPER_BINARY:
		return p.oper()
	}
	return nil
}

// parsePrefixExpr reads a PrefixExpression: one operand with whatever
// binds to its left.
func (p *parser) parsePrefixExpr(fl exprFlags) ast.Expr {
	p.depth++
	defer func() { p.depth-- }()
	lo := p.pos()
	if p.tooDeep() {
		return &ast.BadExpr{Span: p.span(lo)}
	}

	switch {
	case p.at(token.AMP_PREFIX):
		amp := p.pos()
		p.next()
		x := p.parsePrefixExpr(fl)
		return &ast.InOutExpr{Span: p.span(lo), Amp: amp, X: x}

	case p.kind() == token.OPER_PREFIX:
		op := p.oper()
		x := p.parsePostfixExpr(fl)
		return &ast.PrefixExpr{Span: p.span(lo), Op: op, X: x}

	case p.atOwnership("consume"):
		kw := p.pos()
		p.next()
		x := p.parsePrefixExpr(fl)
		return &ast.ConsumeExpr{Span: p.span(lo), Keyword: kw, X: x}

	case p.atOwnership("copy"):
		kw := p.pos()
		p.next()
		x := p.parsePrefixExpr(fl)
		return &ast.CopyExpr{Span: p.span(lo), Keyword: kw, X: x}

	case p.atOwnership("borrow"):
		kw := p.pos()
		p.next()
		x := p.parsePrefixExpr(fl)
		return &ast.BorrowExpr{Span: p.span(lo), Keyword: kw, X: x}

	case p.atPackRef():
		kw := p.pos()
		p.next()
		x := p.parsePrefixExpr(fl)
		return &ast.PackReferenceExpr{Span: p.span(lo), Each: kw, X: x}
	}
	return p.parsePostfixExpr(fl)
}

// atPackRef settles `each` the pack element reference operator from `each`
// an ordinary identifier. The operand is an expression starting on the same line.
func (p *parser) atPackRef() bool {
	if !p.atWord("each") {
		return false
	}
	t := p.peekTok(1)
	if t.Flags.Has(token.FlagNLBefore) {
		return false
	}
	return p.atExprStartAt(1)
}

// atOwnership settles the ownership operators from the ordinary names
// they are spelled with. `consume x` is the operator; `copy(x)` is a
// call to something called copy, and `let borrow = 1` is a variable.
// The operand is a name or self, written on the same line.
func (p *parser) atOwnership(word string) bool {
	if !p.atWord(word) {
		return false
	}
	t := p.peekTok(1)
	if t.Flags.Has(token.FlagNLBefore) {
		return false
	}
	return t.Kind == token.IDENT || t.Kind == token.SELF
}

// parsePostfixExpr reads a PrimaryExpression and everything that
// hangs off its right.
func (p *parser) parsePostfixExpr(fl exprFlags) ast.Expr {
	lo := p.pos()
	x := p.parsePrimaryExpr(fl)
	return p.parsePostfixSuffixes(x, lo, fl)
}

func (p *parser) parsePostfixSuffixes(x ast.Expr, lo token.Pos, fl exprFlags) ast.Expr {
	for {
		switch {
		case p.at(token.PERIOD) || p.at(token.PERIOD_PREFIX):
			x = p.parseMemberSuffix(x, lo)

		case p.at(token.LPAREN) && !p.nl():
			c := &ast.CallExpr{Fun: x, Args: p.parseCallArgs()}
			c.Span = p.span(lo)
			x = c

		case p.at(token.LSQUARE) && !p.nl():
			s := &ast.SubscriptExpr{X: x, Lsquare: p.pos()}
			p.next()
			s.Args = p.parseCallArgList(token.RSQUARE)
			s.Rsquare = p.expect(token.RSQUARE)
			s.Span = p.span(lo)
			x = s

		case p.at(token.QUESTION_POSTFIX):
			q := p.pos()
			p.next()
			x = &ast.OptionalExpr{Span: p.span(lo), X: x, Question: q}

		case p.at(token.EXCLAIM_POSTFIX):
			e := p.pos()
			p.next()
			x = &ast.ForceExpr{Span: p.span(lo), X: x, Exclaim: e}

		case p.kind() == token.OPER_POSTFIX:
			op := p.oper()
			x = &ast.PostfixExpr{Span: p.span(lo), X: x, Op: op}

		// Avoid treating property accessor/observer blocks as trailing closures.
		case p.at(token.LBRACE) && fl&exprBasic == 0 && !p.nl() && !p.atAccessorBlock():
			x = p.attachTrailing(x, lo)

		default:
			return x
		}
	}
}

// parseMemberSuffix reads what follows a '.': a member, an
// initializer reference, `self`, or a tuple element's number.
func (p *parser) parseMemberSuffix(x ast.Expr, lo token.Pos) ast.Expr {
	dot := p.pos()
	p.next()

	switch {
	case p.at(token.INIT):
		i := p.pos()
		p.next()
		r := &ast.InitRefExpr{X: x, Dot: dot, Init: i}
		r.Lparen, r.Names, r.Rparen = p.parseArgumentNames()
		r.Span = p.span(lo)
		return r

	case p.at(token.SELF):
		s := p.pos()
		p.next()
		return &ast.PostfixSelfExpr{Span: p.span(lo), X: x, Dot: dot, Self: s}

	// Tuple indexing (e.g. `t.0`).
	case p.at(token.INT_LIT):
		nlo := p.pos()
		p.next()
		name := &ast.Ident{Span: p.span(nlo)}
		m := &ast.MemberExpr{X: x, Dot: dot, Name: name}
		m.Span = p.span(lo)
		return m

	// Split float literals like `0.0` into consecutive tuple indices (e.g. `t.0.0`).
	case p.at(token.FLOAT_LIT) && p.dotInFloat() > 0:
		t := p.tok()
		mid := t.Pos + token.Pos(p.dotInFloat())
		p.next()
		outer := &ast.MemberExpr{
			Span: ast.Span{Lo: lo, Hi: mid},
			X:    x, Dot: dot,
			Name: &ast.Ident{Span: ast.Span{Lo: t.Pos, Hi: mid}},
		}
		return &ast.MemberExpr{
			Span: p.span(lo),
			X:    outer, Dot: mid,
			Name: &ast.Ident{Span: ast.Span{Lo: mid + 1, Hi: t.End}},
		}
	}

	m := &ast.MemberExpr{X: x, Dot: dot}
	if p.at(token.IDENT) || p.kind().IsKeyword() {
		m.Name = p.ident()
	} else {
		p.errHere("expected a member name after '.'")
	}
	m.Args = p.tryGenericArgs()
	m.Lparen, m.Names, m.Rparen = p.parseArgumentNames()
	m.Span = p.span(lo)
	return m
}

// parseArgumentNames parses compound function name labels (e.g. `f(x:y:)`).
func (p *parser) parseArgumentNames() (lparen token.Pos, names []*ast.ArgumentName, rparen token.Pos) {
	if !p.at(token.LPAREN) || p.nl() || !p.atArgumentNames() {
		return token.NoPos, nil, token.NoPos
	}
	lparen = p.pos()
	p.next()
	for p.at(token.IDENT) || p.at(token.UNDERSCORE) {
		alo := p.pos()
		n := p.ident()
		c := p.expect(token.COLON)
		names = append(names, &ast.ArgumentName{Span: p.span(alo), Name: n, Colon: c})
	}
	return lparen, names, p.expect(token.RPAREN)
}

// dotInFloat returns the offset of the dot in the float literal at
// the cursor when the literal is two tuple indices written together —
// digits, one dot, digits, and nothing else — and 0 when it is not.
// `t.0.0` holds one; `t.0e1` and `t.0x1p0` do not.
func (p *parser) dotInFloat() int {
	s := p.cur()
	dot := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= '0' && c <= '9':
		case c == '.' && dot == 0 && i > 0:
			dot = i
		default:
			return 0
		}
	}
	if dot == 0 || dot == len(s)-1 {
		return 0
	}
	return dot
}

// atArgumentNames reports whether the paren at the cursor holds an
// ArgumentNames list — `label:` repeated with nothing between the
// colon and the next label — rather than a call's arguments.
func (p *parser) atArgumentNames() bool {
	n := 1
	if p.peek(n) == token.RPAREN {
		return false // `f()` is a call
	}
	for {
		if k := p.peek(n); k != token.IDENT && k != token.UNDERSCORE {
			return false
		}
		if p.peek(n+1) != token.COLON {
			return false
		}
		n += 2
		if p.peek(n) == token.RPAREN {
			return true
		}
	}
}

// attachTrailing parses trailing closures and attaches them to x as a CallExpr.
func (p *parser) attachTrailing(x ast.Expr, lo token.Pos) ast.Expr {
	var trailing []*ast.TrailingClosure
	first := p.parseClosure()
	trailing = append(trailing, &ast.TrailingClosure{
		Span: ast.Span{Lo: first.Pos(), Hi: first.End()}, Closure: first})

	for p.at(token.IDENT) && p.peek(1) == token.COLON && p.peek(2) == token.LBRACE {
		tlo := p.pos()
		label := p.ident()
		colon := p.expect(token.COLON)
		cl := p.parseClosure()
		trailing = append(trailing, &ast.TrailingClosure{
			Span: p.span(tlo), Label: label, Colon: colon, Closure: cl})
	}

	if c, ok := x.(*ast.CallExpr); ok && c.Trailing == nil {
		c.Trailing = trailing
		c.Span = p.span(lo)
		c.Args = withTrailing(c.Args, trailing)
		return c
	}
	return &ast.CallExpr{Span: p.span(lo), Fun: x, Args: withTrailing(nil, trailing), Trailing: trailing}
}

// withTrailing is a call's arguments with its trailing closures after
// them, as the arguments they are: `apply(3) { $0 * 2 }` passes two, and
// everything that matches arguments to parameters sees both.
func withTrailing(args *ast.CallArgs, trailing []*ast.TrailingClosure) *ast.CallArgs {
	if args == nil {
		args = &ast.CallArgs{}
		if len(trailing) > 0 {
			args.Span = trailing[0].Span
		}
	}
	for _, tc := range trailing {
		args.Args = append(args.Args, &ast.CallArg{Span: tc.Span, Label: tc.Label, Colon: tc.Colon, X: tc.Closure})
	}
	return args
}

// parseCallArgs reads a FunctionCallArgumentClause.
func (p *parser) parseCallArgs() *ast.CallArgs {
	lo := p.pos()
	a := &ast.CallArgs{Lparen: p.expect(token.LPAREN)}
	a.Args = p.parseCallArgList(token.RPAREN)
	a.Rparen = p.expect(token.RPAREN)
	a.Span = p.span(lo)
	return a
}

// parseCallArgList reads a FunctionCallArgumentList up to close.
func (p *parser) parseCallArgList(close token.Kind) []*ast.CallArg {
	var out []*ast.CallArg
	for !p.at(close) && !p.at(token.EOF) {
		start := p.i
		lo := p.pos()
		arg := &ast.CallArg{}

		// Argument label (`name:`).
		if p.atArgLabel() {
			arg.Label = p.ident()
			arg.Colon = p.expect(token.COLON)
		}

		// Operator passed as argument (e.g. `reduce(0, +)`).
		if p.atAnyOper() && (p.peek(1) == close || p.peek(1) == token.COMMA) {
			arg.X = p.oper()
		} else {
			arg.X = p.parseExpr(0)
		}
		arg.Span = p.span(lo)
		out = append(out, arg)

		if !p.more(start) {
			break
		}
	}
	return out
}

// atArgLabel reports whether the cursor is on a `label:` — a name,
// then a colon, then something that is not the second half of a
// ternary.
func (p *parser) atArgLabel() bool {
	if p.peek(1) != token.COLON {
		return false
	}
	return p.at(token.IDENT) || p.kind().IsKeyword()
}

// ---- primary expressions ----

func (p *parser) parsePrimaryExpr(fl exprFlags) ast.Expr {
	lo := p.pos()
	switch k := p.kind(); {
	case k == token.INT_LIT, k == token.FLOAT_LIT, k == token.REGEX_LIT,
		k == token.TRUE, k == token.FALSE, k == token.NIL:
		p.next()
		return &ast.BasicLit{Span: p.span(lo), Kind: k}

	case k == token.STRING_QUOTE, k == token.MULTILINE_STRING_QUOTE,
		k == token.POUND_DELIM:
		if s := p.parseStringLit(); s != nil {
			return s
		}
		p.errHere("expected an expression")
		return &ast.BadExpr{Span: p.span(lo)}

	// Explicit existential or opaque type in expression position (`any P`, `some P`).
	case (p.atWord("any") || p.atWord("some")) && p.atTypeStartAt(1):
		return &ast.TypeExpr{Span: p.span(lo), Type: p.parseType()}

	case k == token.IDENT:
		name := p.ident()
		e := &ast.IdentExpr{Name: name, Args: p.tryGenericArgs()}
		e.Lparen, e.Names, e.Rparen = p.parseArgumentNames()
		e.Span = p.span(lo)
		return e

	case k == token.SELF:
		p.next()
		return &ast.SelfExpr{Span: p.span(lo)}

	// `Self` and `Any` in expression position.
	case k == token.SELF_TYPE, k == token.ANY:
		slo := p.pos()
		p.next()
		e := &ast.IdentExpr{Name: &ast.Ident{Span: p.span(slo)}}
		e.Span = p.span(lo)
		return e

	// If and switch expressions.
	case k == token.IF:
		return &ast.StmtExpr{Span: p.span(lo), Stmt: p.parseIf()}

	case k == token.SWITCH:
		return &ast.StmtExpr{Span: p.span(lo), Stmt: p.parseSwitch()}

	case k == token.SUPER:
		p.next()
		return &ast.SuperExpr{Span: p.span(lo)}

	case k == token.UNDERSCORE:
		p.next()
		return &ast.WildcardExpr{Span: p.span(lo)}

	case k == token.LBRACE:
		return p.parseClosure()

	case k == token.LPAREN:
		return p.parseParenOrTuple()

	case k == token.LSQUARE:
		return p.parseCollectionLit()

	case k == token.PERIOD, k == token.PERIOD_PREFIX:
		dot := p.pos()
		p.next()
		e := &ast.ImplicitMemberExpr{Dot: dot}
		if p.at(token.IDENT) || p.kind().IsKeyword() {
			e.Name = p.ident()
		} else {
			p.errHere("expected a member name after '.'")
		}
		e.Args = p.tryGenericArgs()
		e.Span = p.span(lo)
		return e

	case k == token.BACKSLASH:
		return p.parseKeyPath()

	case k == token.POUND_SELECTOR:
		return p.parseSelector()

	case k == token.POUND_KEYPATH:
		e := &ast.KeyPathStringExpr{Pound: p.pos()}
		p.next()
		e.Lparen = p.expect(token.LPAREN)
		e.X = p.parseExpr(0)
		e.Rparen = p.expect(token.RPAREN)
		e.Span = p.span(lo)
		return e

	case k == token.POUND_FILE, k == token.POUND_FILEID, k == token.POUND_FILEPATH,
		k == token.POUND_LINE, k == token.POUND_COLUMN, k == token.POUND_FUNCTION,
		k == token.POUND_DSOHANDLE:
		p.next()
		return &ast.MagicLit{Span: p.span(lo), Kind: k}

	case k == token.POUND_COLORLITERAL, k == token.POUND_FILELITERAL,
		k == token.POUND_IMAGELITERAL:
		e := &ast.PlaygroundLit{Pound: p.pos(), Kind: k}
		p.next()
		e.Args = p.parseCallArgs()
		e.Span = p.span(lo)
		return e

	case k == token.POUND_AVAILABLE, k == token.POUND_UNAVAILABLE:
		// Only a condition list may hold one, and this is not one.
		p.errHere("'" + k.String() + "' may only appear in a condition")
		if c := p.parseAvailability(); c != nil {
			return &ast.BadExpr{Span: ast.Span{Lo: c.Pos(), Hi: c.End()}}
		}
		return &ast.BadExpr{Span: p.span(lo)}

	case k == token.POUND:
		return p.parseMacroExpansion()

	case k == token.AT:
		// Attribute prefix on closure or type expression.
		m := p.mark()
		attrs := p.parseAttrs()
		if p.at(token.LBRACE) {
			cl := p.parseClosure()
			cl.Attrs = append(attrs, cl.Attrs...)
			cl.Span = p.span(lo)
			return cl
		}
		p.reset(m)
		return &ast.TypeExpr{Span: p.span(lo), Type: p.parseType()}
	}

	p.errHere("expected an expression")
	if !p.at(token.EOF) && !p.nl() {
		p.next()
	}
	return &ast.BadExpr{Span: p.span(lo)}
}

// parseMacroExpansion parses a freestanding macro expansion expression (`#name<...>(...)`).
func (p *parser) parseMacroExpansion() ast.Expr {
	lo := p.pos()
	e := &ast.MacroExpansionExpr{Pound: p.pos()}
	p.next()
	e.Name = p.expectIdent()
	e.Generics = p.tryGenericArgs()
	if p.at(token.LPAREN) && !p.nl() {
		e.Args = p.parseCallArgs()
	}
	if p.at(token.LBRACE) && !p.nl() {
		if c, ok := p.attachTrailing(&ast.BadExpr{Span: p.span(lo)}, lo).(*ast.CallExpr); ok {
			e.Trailing = c.Trailing
		}
	}
	e.Span = p.span(lo)
	return e
}

func (p *parser) parseSelector() ast.Expr {
	lo := p.pos()
	e := &ast.SelectorExpr{Pound: p.pos()}
	p.next()
	e.Lparen = p.expect(token.LPAREN)
	if (p.atWord("getter") || p.atWord("setter")) && p.peek(1) == token.COLON {
		e.Label = p.ident()
		e.Colon = p.expect(token.COLON)
	}
	e.X = p.parseExpr(0)
	e.Rparen = p.expect(token.RPAREN)
	e.Span = p.span(lo)
	return e
}

// atExprStart reports whether an expression may begin here. It is
// what tells `return` with a value from `return` without one.
func (p *parser) atExprStart() bool { return p.exprStartAt(0) }

func (p *parser) atExprStartAt(n int) bool { return p.exprStartAt(n) }

func (p *parser) exprStartAt(n int) bool {
	switch p.peek(n) {
	case token.IDENT, token.INT_LIT, token.FLOAT_LIT, token.REGEX_LIT,
		token.STRING_QUOTE, token.MULTILINE_STRING_QUOTE, token.POUND_DELIM,
		token.TRUE, token.FALSE, token.NIL, token.SELF, token.SELF_TYPE,
		token.SUPER, token.UNDERSCORE, token.ANY,
		token.LPAREN, token.LSQUARE, token.LBRACE,
		token.PERIOD, token.PERIOD_PREFIX, token.BACKSLASH, token.AMP_PREFIX,
		token.TRY, token.IS, token.AT, token.POUND, token.REPEAT,
		token.OPER_PREFIX, token.IF, token.SWITCH:
		return true
	}
	return p.peek(n).IsPound()
}
