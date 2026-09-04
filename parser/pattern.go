package parser

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// patternMode says which of the two jobs a pattern is doing.
//
// A binding pattern declares names: `let (x, y) = p` binds x and y,
// and every identifier in it is a new name. A matching pattern tests
// a value: `case (x, y)` compares against whatever x and y already
// name, and only a `let` or `var` inside it turns an identifier back
// into a binding. The two are the same syntax and different readings,
// which is why this flag rides along.
type patternMode uint

const (
	patternBinding  patternMode = 0
	patternMatching patternMode = 1 << iota
	patternInBinding
)

// parsePattern reads a Pattern.
func (p *parser) parsePattern(mode patternMode) ast.Pattern {
	p.depth++
	defer func() { p.depth-- }()
	lo := p.pos()
	if p.tooDeep() {
		return &ast.BadPattern{Span: p.span(lo)}
	}

	pat := p.parsePatternAtom(mode)

	// `x?` — the OptionalPattern.
	for p.at(token.QUESTION_POSTFIX) {
		q := p.pos()
		p.next()
		pat = &ast.OptionalPattern{Span: p.span(lo), Pat: pat, Question: q}
	}

	// `p as T` — the casting half of TypeCastingPattern.
	if mode&patternMatching != 0 && p.at(token.AS) {
		as := p.pos()
		p.next()
		t := p.parseType()
		pat = &ast.AsPattern{Span: p.span(lo), Pat: pat, As: as, Type: t}
	}

	// A type annotation binds a name to a type, and only a binding
	// pattern has a name to bind. In a case label the colon ends the
	// label, which is the other reason this is not read there.
	if mode&patternMatching == 0 && p.at(token.COLON) {
		colon := p.pos()
		p.next()
		t := p.parseType()
		pat = &ast.TypedPattern{Span: p.span(lo), Pat: pat, Colon: colon, Type: t}
	}
	return pat
}

func (p *parser) parsePatternAtom(mode patternMode) ast.Pattern {
	lo := p.pos()
	switch {
	case p.at(token.UNDERSCORE):
		p.next()
		return &ast.WildcardPattern{Span: p.span(lo)}

	case p.at(token.LET), p.at(token.VAR):
		b := &ast.ValueBindingPattern{Keyword: p.pos(), Kind: p.kind()}
		p.next()
		b.Pat = p.parsePattern(mode | patternInBinding)
		b.Span = p.span(lo)
		return b

	case p.at(token.LPAREN):
		return p.parseTuplePattern(mode)

	case p.at(token.IS) && mode&patternMatching != 0:
		is := p.pos()
		p.next()
		t := p.parseType()
		return &ast.IsPattern{Span: p.span(lo), Is: is, Type: t}

	case p.at(token.PERIOD), p.at(token.PERIOD_PREFIX):
		return p.parseEnumCasePattern(nil, lo, mode)

	// `if let self = self`, and the shorthand `if let self`: the
	// grammar's IdentifierPattern is an Identifier, and self is not
	// one, but this is the one place it binds like a name.
	case p.at(token.SELF) && mode&patternMatching == 0:
		slo := p.pos()
		p.next()
		return &ast.IdentPattern{Span: p.span(lo), Name: &ast.Ident{Span: p.span(slo)}}

	case p.at(token.IDENT):
		if mode&patternMatching != 0 {
			if pat := p.tryEnumCasePattern(mode); pat != nil {
				return pat
			}
			if mode&patternInBinding == 0 {
				// A name that binds nothing is a value to compare with.
				x := p.parseExpr(exprBasic | exprNoAssign)
				return &ast.ExprPattern{Span: p.span(lo), X: x}
			}
		}
		name := p.ident()
		return &ast.IdentPattern{Span: p.span(lo), Name: name}
	}

	if mode&patternMatching != 0 && p.atExprStart() {
		x := p.parseExpr(exprBasic | exprNoAssign)
		return &ast.ExprPattern{Span: p.span(lo), X: x}
	}
	p.errHere("expected a pattern")
	if !p.at(token.EOF) && !p.nl() {
		p.next()
	}
	return &ast.BadPattern{Span: p.span(lo)}
}

// tryEnumCasePattern reads Type . CaseName [TuplePattern], and
// returns nil if that is not what is here. The dot is what makes it
// one: a bare name in a matching pattern is a value, not a case.
func (p *parser) tryEnumCasePattern(mode patternMode) ast.Pattern {
	m := p.mark()
	lo := p.pos()
	t := p.parseSimpleTypeNoMember()
	if !p.at(token.PERIOD) && !p.at(token.PERIOD_PREFIX) {
		p.reset(m)
		return nil
	}
	pat := p.parseEnumCasePattern(t, lo, mode)
	if !p.atPatternFollow() {
		p.reset(m)
		return nil
	}
	return pat
}

// parseEnumCasePattern reads the . CaseName [TuplePattern] of an enum
// case pattern, whose type has already been read (or was left out).
func (p *parser) parseEnumCasePattern(t ast.Type, lo token.Pos, mode patternMode) ast.Pattern {
	e := &ast.EnumCasePattern{Type: t, Dot: p.pos()}
	p.next()
	if p.at(token.IDENT) || p.kind().IsKeyword() {
		e.Name = p.ident()
	} else {
		p.errHere("expected a case name after '.'")
	}
	if p.at(token.LPAREN) && !p.nl() {
		if tp, ok := p.parseTuplePattern(mode).(*ast.TuplePattern); ok {
			e.Args = tp
		}
	}
	e.Span = p.span(lo)
	return e
}

// parseSimpleTypeNoMember reads a type name and its generic
// arguments, and stops at the dot: the member of an enum case pattern
// is the case, not a nested type.
func (p *parser) parseSimpleTypeNoMember() ast.Type {
	lo := p.pos()
	name := p.expectIdent()
	if name == nil {
		return &ast.BadType{Span: p.span(lo)}
	}
	t := &ast.IdentType{Name: name}
	if p.atLt() {
		t.Args = p.parseGenericArgs()
	}
	t.Span = p.span(lo)
	return t
}

// atPatternFollow reports whether the cursor is somewhere a pattern
// may end: what closes a case item, a binding, or a for-in clause.
func (p *parser) atPatternFollow() bool {
	switch p.kind() {
	case token.COLON, token.COMMA, token.SEMI, token.RPAREN, token.RSQUARE,
		token.WHERE, token.IN, token.ASSIGN, token.LBRACE, token.EOF,
		token.QUESTION_POSTFIX, token.AS:
		return true
	}
	return false
}

func (p *parser) parseTuplePattern(mode patternMode) ast.Pattern {
	lo := p.pos()
	t := &ast.TuplePattern{Lparen: p.pos()}
	p.next()
	for !p.at(token.RPAREN) && !p.at(token.EOF) {
		start := p.i
		elo := p.pos()
		e := &ast.TuplePatternElem{}
		if p.at(token.IDENT) && p.peek(1) == token.COLON {
			e.Label = p.ident()
			e.Colon = p.pos()
			p.next()
		}
		e.Pat = p.parsePattern(mode)
		e.Span = p.span(elo)
		t.Elems = append(t.Elems, e)
		if !p.at(token.COMMA) {
			break
		}
		p.next()
		if p.i == start {
			p.next()
		}
	}
	t.Rparen = p.expect(token.RPAREN)
	t.Span = p.span(lo)
	return t
}
