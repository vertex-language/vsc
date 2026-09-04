package parser

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// parseStringLit reads one string literal from the pieces the scanner
// made of it, and returns nil if there is no literal here. An
// interpolation's expression is parsed like any other, because that
// is what it is.
func (p *parser) parseStringLit() *ast.StringLit {
	lo := p.pos()
	s := &ast.StringLit{}

	if p.at(token.POUND_DELIM) {
		if k := p.peek(1); k != token.STRING_QUOTE && k != token.MULTILINE_STRING_QUOTE {
			return nil
		}
		s.Pounds = int(p.tok().End - p.tok().Pos)
		p.next()
	}
	if !p.at(token.STRING_QUOTE) && !p.at(token.MULTILINE_STRING_QUOTE) {
		return nil
	}
	quote := p.kind()
	s.Multiline = quote == token.MULTILINE_STRING_QUOTE
	s.Open = p.pos()
	p.next()

	for !p.at(token.EOF) {
		switch {
		case p.at(quote):
			s.Close = p.pos()
			p.next()
			if s.Pounds > 0 && p.at(token.POUND_DELIM) {
				p.next()
			}
			s.Span = p.span(lo)
			return s

		case p.at(token.STRING_SEGMENT):
			tlo := p.pos()
			p.next()
			s.Segments = append(s.Segments, &ast.StringText{Span: p.span(tlo)})

		case p.at(token.BACKSLASH):
			ilo := p.pos()
			in := &ast.Interpolation{Backslash: p.pos()}
			p.next()
			if p.at(token.POUND_DELIM) {
				p.next()
			}
			in.Lparen = p.expect(token.LPAREN)
			in.Args = p.parseCallArgList(token.RPAREN)
			if len(in.Args) == 1 && in.Args[0].Label == nil {
				in.X = in.Args[0].X
			}
			in.Rparen = p.expect(token.RPAREN)
			in.Span = p.span(ilo)
			s.Segments = append(s.Segments, in)

		default:
			// The scanner emits nothing else between the quotes; if
			// something is here, the literal was never closed.
			p.errHere("expected the end of the string literal")
			s.Span = p.span(lo)
			return s
		}
	}
	s.Span = p.span(lo)
	return s
}

// parseParenOrTuple reads ( Expression ), a TupleExpression, or the
// empty tuple. One unlabeled element in parentheses is that element
// in parentheses: there is no one-element tuple.
func (p *parser) parseParenOrTuple() ast.Expr {
	lo := p.pos()
	lparen := p.pos()
	p.next()

	var elems []*ast.TupleElem
	for !p.at(token.RPAREN) && !p.at(token.EOF) {
		start := p.i
		elo := p.pos()
		e := &ast.TupleElem{}
		if p.atArgLabel() {
			e.Label = p.ident()
			e.Colon = p.expect(token.COLON)
		}
		e.X = p.parseExpr(0)
		e.Span = p.span(elo)
		elems = append(elems, e)
		if !p.more(start) {
			break
		}
	}
	rparen := p.expect(token.RPAREN)

	if len(elems) == 1 && elems[0].Label == nil {
		return &ast.ParenExpr{Span: p.span(lo), Lparen: lparen, X: elems[0].X, Rparen: rparen}
	}
	return &ast.TupleExpr{Span: p.span(lo), Lparen: lparen, Elems: elems, Rparen: rparen}
}

// parseCollectionLit reads an ArrayLiteral or a DictionaryLiteral.
// They share an opening bracket and are told apart by the first
// item's colon — or, when there are no items, by the colon the empty
// dictionary literal is written with.
func (p *parser) parseCollectionLit() ast.Expr {
	lo := p.pos()
	lsquare := p.pos()
	p.next()

	// '[' : ']' — the empty dictionary.
	if p.at(token.COLON) {
		colon := p.pos()
		p.next()
		rsquare := p.expect(token.RSQUARE)
		return &ast.DictLit{Span: p.span(lo), Lsquare: lsquare, Colon: colon, Rsquare: rsquare}
	}
	if p.at(token.RSQUARE) {
		rsquare := p.pos()
		p.next()
		return &ast.ArrayLit{Span: p.span(lo), Lsquare: lsquare, Rsquare: rsquare}
	}

	first := p.parseExpr(0)
	if p.at(token.COLON) {
		return p.parseDictLitRest(lo, lsquare, first)
	}

	arr := &ast.ArrayLit{Lsquare: lsquare, Items: []ast.Expr{first}}
	for p.at(token.COMMA) {
		comma := p.pos()
		p.next()
		if p.at(token.RSQUARE) {
			arr.Comma = comma
			break
		}
		arr.Items = append(arr.Items, p.parseExpr(0))
	}
	arr.Rsquare = p.expect(token.RSQUARE)
	arr.Span = p.span(lo)
	return arr
}

func (p *parser) parseDictLitRest(lo, lsquare token.Pos, first ast.Expr) ast.Expr {
	d := &ast.DictLit{Lsquare: lsquare}
	item := &ast.DictLitItem{Key: first, Colon: p.pos()}
	p.next()
	item.Value = p.parseExpr(0)
	item.Span = ast.Span{Lo: first.Pos(), Hi: item.Value.End()}
	d.Items = append(d.Items, item)

	for p.at(token.COMMA) {
		comma := p.pos()
		p.next()
		if p.at(token.RSQUARE) {
			d.Comma = comma
			break
		}
		ilo := p.pos()
		it := &ast.DictLitItem{Key: p.parseExpr(0)}
		it.Colon = p.expect(token.COLON)
		it.Value = p.parseExpr(0)
		it.Span = p.span(ilo)
		d.Items = append(d.Items, it)
	}
	d.Rsquare = p.expect(token.RSQUARE)
	d.Span = p.span(lo)
	return d
}

// ---- closures ----

// parseClosure reads '{' [AttributeList] [ClosureSignature]
// [Statements] '}'.
//
// Whether a signature is there cannot be decided by looking: `{ [a] in
// … }` captures a, and `{ [a] }` is a closure returning an array. So
// the signature is attempted, and the attempt is undone if it does not
// reach the `in` that ends one.
func (p *parser) parseClosure() *ast.ClosureExpr {
	lo := p.pos()
	c := &ast.ClosureExpr{Lbrace: p.expect(token.LBRACE)}
	c.Attrs = p.parseAttrs()

	m := p.mark()
	if sig := p.parseClosureSig(); sig != nil {
		c.Sig = sig
	} else {
		p.reset(m)
	}

	if p.mode&SkipBodies != 0 && c.Sig == nil {
		p.i-- // step back onto the '{' so the whole body is skipped
		p.skipBalanced()
		c.Rbrace = p.prevEnd() - 1
		c.Span = p.span(lo)
		return c
	}

	c.Stmts = p.parseStmtList(stopBrace)
	c.Rbrace = p.expect(token.RBRACE)
	c.Span = p.span(lo)
	return c
}

// parseClosureSig returns the signature, or nil if what is here is
// not one. It reports nothing: a failed attempt is undone by the
// caller, and whatever it read is read again as statements.
func (p *parser) parseClosureSig() *ast.ClosureSig {
	lo := p.pos()
	sig := &ast.ClosureSig{}

	if p.at(token.LSQUARE) {
		sig.Captures = p.parseCaptureList()
		if sig.Captures == nil {
			return nil
		}
	}

	switch {
	case p.at(token.LPAREN):
		sig.Params = p.parseClosureParams()
		if sig.Params == nil {
			return nil
		}
	case p.at(token.IDENT) || p.at(token.UNDERSCORE):
		// The bare IdentifierList form: { a, b in … }.
		if !p.atClosureIdentList() {
			return nil
		}
		plo := p.pos()
		params := &ast.ClosureParams{}
		for {
			nlo := p.pos()
			n := p.ident()
			params.Params = append(params.Params,
				&ast.ClosureParam{Span: p.span(nlo), Name: n})
			if !p.at(token.COMMA) {
				break
			}
			p.next()
		}
		params.Span = p.span(plo)
		sig.Params = params
	}

	sig.Async = p.takeWord("async")
	sig.Throws = p.parseThrowsClause()
	if p.at(token.ARROW) {
		rlo := p.pos()
		arrow := p.pos()
		p.next()
		t := p.parseType()
		sig.Result = &ast.FuncResult{Span: p.span(rlo), Arrow: arrow, Type: t}
	}

	if !p.at(token.IN) {
		return nil
	}
	sig.In = p.pos()
	p.next()
	sig.Span = p.span(lo)
	return sig
}

// atClosureIdentList reports whether the identifiers at the cursor
// are a closure's bare parameter list — names separated by commas and
// closed by `in`, or by the rest of a signature.
func (p *parser) atClosureIdentList() bool {
	n := 0
	for {
		if k := p.peek(n); k != token.IDENT && k != token.UNDERSCORE {
			return false
		}
		switch p.peek(n + 1) {
		case token.COMMA:
			n += 2
		case token.IN, token.ARROW:
			return true
		default:
			// `async`, `throws` and `rethrows` may follow too.
			if p.wordAt(n+1, "async") || p.peek(n+1) == token.THROWS ||
				p.peek(n+1) == token.RETHROWS {
				return true
			}
			return false
		}
	}
}

func (p *parser) parseCaptureList() *ast.CaptureList {
	lo := p.pos()
	c := &ast.CaptureList{Lsquare: p.pos()}
	p.next()
	for !p.at(token.RSQUARE) && !p.at(token.EOF) {
		start := p.i
		ilo := p.pos()
		item := &ast.CaptureItem{}
		if p.atCaptureSpec() {
			item.Spec = p.parseModifier()
		}
		item.X = p.parseExpr(0)
		item.Span = p.span(ilo)
		c.Items = append(c.Items, item)
		if !p.at(token.COMMA) {
			break
		}
		p.next()
		if p.i == start {
			return nil
		}
	}
	if !p.at(token.RSQUARE) {
		return nil
	}
	c.Rsquare = p.pos()
	p.next()
	c.Span = p.span(lo)
	return c
}

// atCaptureSpec reports whether a CaptureSpecifier is here: weak,
// unowned, unowned(safe) or unowned(unsafe).
func (p *parser) atCaptureSpec() bool {
	if p.atWord("weak") {
		return true
	}
	if !p.atWord("unowned") {
		return false
	}
	return true
}

// parseClosureParams reads a parenthesized ClosureParameterList.
func (p *parser) parseClosureParams() *ast.ClosureParams {
	lo := p.pos()
	c := &ast.ClosureParams{Lparen: p.pos()}
	p.next()
	for !p.at(token.RPAREN) && !p.at(token.EOF) {
		start := p.i
		plo := p.pos()
		param := &ast.ClosureParam{}
		if !p.at(token.IDENT) && !p.at(token.UNDERSCORE) {
			return nil
		}
		param.Name = p.ident()
		if p.at(token.IDENT) || p.at(token.UNDERSCORE) {
			param.Label, param.Name = param.Name, p.ident()
		}
		if p.at(token.COLON) {
			param.Colon = p.pos()
			p.next()
			// The annotation carries the same modifiers a
			// declaration's parameter does: `{ (x: inout Int) in … }`.
			for p.atParamModifier() {
				param.Mods = append(param.Mods, p.parseModifier())
			}
			param.Type = p.parseType()
		}
		param.Span = p.span(plo)
		c.Params = append(c.Params, param)
		if !p.at(token.COMMA) {
			break
		}
		p.next()
		if p.i == start {
			return nil
		}
	}
	if !p.at(token.RPAREN) {
		return nil
	}
	c.Rparen = p.pos()
	p.next()
	c.Span = p.span(lo)
	return c
}

// ---- key paths ----

// parseKeyPath reads \ [Type] [.] KeyPathComponents, or the subscript
// form.
//
// The root type is read as a name and its generic arguments, and
// stops at the first dot: everything after it is a component. That
// makes `\Outer.Inner.x` a root of Outer and two components, even
// where Inner is a nested type — telling those apart is name
// resolution, and the analyzer does it with the components in hand.
func (p *parser) parseKeyPath() ast.Expr {
	lo := p.pos()
	k := &ast.KeyPathExpr{Backslash: p.pos()}
	p.next()

	switch {
	case p.at(token.LPAREN):
		// A parenthesized root: `\(Int, String).1`. The type reader
		// stops before the first component, since a number is not a
		// member of a type.
		k.Type = p.parseType()
	case !p.at(token.PERIOD) && !p.at(token.PERIOD_PREFIX) && !p.at(token.LSQUARE):
		k.Type = p.parseSimpleTypeNoMember()
	}
	for {
		clo := p.pos()
		c := &ast.KeyPathComponent{}
		switch {
		case p.at(token.PERIOD) || p.at(token.PERIOD_PREFIX):
			c.Dot = p.pos()
			p.next()
			switch {
			case p.at(token.SELF):
				c.Self = p.pos()
				p.next()
			case p.at(token.LSQUARE):
				slo := p.pos()
				s := &ast.SubscriptExpr{Lsquare: p.pos()}
				p.next()
				s.Args = p.parseCallArgList(token.RSQUARE)
				s.Rsquare = p.expect(token.RSQUARE)
				s.Span = p.span(slo)
				c.Sub = s
			// A tuple's elements are named by number here too, and
			// `\.0.1` arrives as one float literal for the same
			// reason `t.0.1` does: it is split back into two
			// components.
			case p.at(token.INT_LIT):
				nlo := p.pos()
				p.next()
				c.Name = &ast.Ident{Span: p.span(nlo)}
			case p.at(token.FLOAT_LIT) && p.dotInFloat() > 0:
				t := p.tok()
				mid := t.Pos + token.Pos(p.dotInFloat())
				p.next()
				c.Name = &ast.Ident{Span: ast.Span{Lo: t.Pos, Hi: mid}}
				c.Span = ast.Span{Lo: clo, Hi: mid}
				k.Components = append(k.Components, c)
				clo = mid
				c = &ast.KeyPathComponent{
					Dot:  mid,
					Name: &ast.Ident{Span: ast.Span{Lo: mid + 1, Hi: t.End}},
				}
			case p.at(token.IDENT) || p.kind().IsKeyword():
				c.Name = p.ident()
				if p.at(token.LPAREN) && !p.nl() {
					c.Args = p.parseCallArgs()
				}
			default:
				p.errHere("expected a key path component after '.'")
			}

		case p.at(token.LSQUARE):
			slo := p.pos()
			s := &ast.SubscriptExpr{Lsquare: p.pos()}
			p.next()
			s.Args = p.parseCallArgList(token.RSQUARE)
			s.Rsquare = p.expect(token.RSQUARE)
			s.Span = p.span(slo)
			c.Sub = s

		case p.at(token.QUESTION_POSTFIX):
			c.Question = p.pos()
			p.next()

		case p.at(token.EXCLAIM_POSTFIX):
			c.Exclaim = p.pos()
			p.next()

		default:
			if len(k.Components) == 0 {
				p.errHere("expected a key path component")
			}
			k.Span = p.span(lo)
			return k
		}
		c.Span = p.span(clo)
		k.Components = append(k.Components, c)
	}
}
