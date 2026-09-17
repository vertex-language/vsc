package parser

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// parseType reads a Type.
func (p *parser) parseType() ast.Type {
	p.depth++
	defer func() { p.depth-- }()
	lo := p.pos()
	if p.tooDeep() {
		return &ast.BadType{Span: p.span(lo)}
	}

	attrs := p.parseAttrs()

	switch {
	case p.atWord("some"):
		kw := p.pos()
		p.next()
		base := p.parseType()
		return &ast.OpaqueType{Span: p.span(lo), Attrs: attrs, Keyword: kw, Base: base}

	case p.atWord("any"):
		kw := p.pos()
		p.next()
		base := p.parseType()
		return &ast.BoxedType{Span: p.span(lo), Attrs: attrs, Keyword: kw, Base: base}

	case p.at(token.REPEAT):
		kw := p.pos()
		p.next()
		base := p.parseType()
		return &ast.PackExpansionType{Span: p.span(lo), Repeat: kw, Base: base}

	case p.atWord("each"):
		kw := p.pos()
		p.next()
		base := p.parseType()
		return &ast.PackReferenceType{Span: p.span(lo), Each: kw, Base: base}

	// `nonisolated(nonsending) (Int) async -> Void`. In type
	// position the word can be nothing else: a declaration's
	// modifiers were read before the type began.
	case p.atWord("nonisolated") && p.peek(1) == token.LPAREN &&
		p.peekTok(1).Flags.Has(token.FlagAdjacent):
		mod := p.parseModifier()
		base := p.parseType()
		return &ast.IsolationType{Span: p.span(lo), Mod: mod, Base: base}

	case p.atWord("sending") && p.atTypeStartAt(1):
		kw := p.pos()
		p.next()
		base := p.parseType()
		return &ast.SendingType{Span: p.span(lo), Keyword: kw, Base: base}
	}

	// A parenthesized list is a tuple type, a parenthesized type, or a
	// function type's parameters — and only what follows it says
	// which, so it is read as parameters and converted if it turns out
	// to be a tuple.
	if p.at(token.LPAREN) {
		lparen := p.pos()
		p.next()
		params := p.parseParamList(token.RPAREN, false)
		rparen := p.expect(token.RPAREN)

		if p.atFuncTypeTail() {
			return p.parseFuncTypeTail(lo, attrs, lparen, params, rparen)
		}
		t := p.tupleFromParams(lo, lparen, params, rparen)
		return p.parseTypeSuffixes(t, lo)
	}

	base := p.parseTypeSuffixes(p.parseTypeAtom(), lo)

	// ProtocolCompositionType: A & B & C.
	if p.atOper("&") {
		c := &ast.CompositionType{Types: []ast.Type{base}}
		for p.atOper("&") {
			c.Amps = append(c.Amps, p.pos())
			p.next()
			c.Types = append(c.Types, p.parseTypeSuffixes(p.parseTypeAtom(), p.pos()))
		}
		c.Span = p.span(lo)
		base = c
	}

	// A function type whose parameters were not parenthesized: the
	// grammar requires them to be, so this is reported and read anyway.
	if p.atFuncTypeTail() {
		p.errHere("a function type's parameters must be written in parentheses")
		return p.parseFuncTypeTail(lo, attrs, token.NoPos,
			[]*ast.Param{{Span: ast.Span{Lo: base.Pos(), Hi: base.End()}, Type: base}}, token.NoPos)
	}
	return base
}

// atFuncTypeTail reports whether what follows is the rest of a
// function type: the effects and the arrow.
func (p *parser) atFuncTypeTail() bool {
	return p.at(token.ARROW) || p.at(token.THROWS) || p.at(token.RETHROWS) ||
		(p.atWord("async") && !p.nl())
}

func (p *parser) parseFuncTypeTail(lo token.Pos, attrs []*ast.Attr,
	lparen token.Pos, params []*ast.Param, rparen token.Pos) ast.Type {

	f := &ast.FuncType{Attrs: attrs, Lparen: lparen, Params: params, Rparen: rparen}
	f.Async = p.takeWord("async")
	f.Throws = p.parseThrowsClause()
	f.Arrow = p.expect(token.ARROW)
	f.Result = p.parseType()
	f.Span = p.span(lo)
	return f
}

// tupleFromParams turns a parenthesized parameter list into what it
// was: a tuple type, or a type in parentheses. A parameter's
// modifiers and default value have no meaning here, and saying so is
// better than dropping them.
func (p *parser) tupleFromParams(lo, lparen token.Pos, params []*ast.Param, rparen token.Pos) ast.Type {
	if len(params) == 1 && params[0].Label == nil && params[0].Name == nil &&
		len(params[0].Mods) == 0 && !params[0].Ellipsis.IsValid() {
		return &ast.ParenType{Span: p.span(lo), Lparen: lparen, X: params[0].Type, Rparen: rparen}
	}
	t := &ast.TupleType{Lparen: lparen, Rparen: rparen}
	for _, param := range params {
		if len(param.Mods) > 0 {
			p.errAt(param.Mods[0], "a parameter modifier is only allowed in a function type")
		}
		if param.Ellipsis.IsValid() {
			p.errAt(param, "a variadic parameter is only allowed in a function type")
		}
		if param.Name != nil {
			p.errAt(param.Name, "a tuple type's element takes one name")
		}
		t.Elems = append(t.Elems, &ast.TupleTypeElem{
			Span: param.Span, Name: param.Label, Colon: param.Colon, Type: param.Type,
		})
	}
	t.Span = p.span(lo)
	return t
}

// atTypeStartAt reports whether a type may begin at the token n
// ahead. It is what tells the `sending` that modifies a type from an
// ordinary name that happens to be spelled the same.
func (p *parser) atTypeStartAt(n int) bool {
	switch p.peek(n) {
	case token.IDENT, token.ANY, token.SELF_TYPE, token.LPAREN,
		token.LSQUARE, token.POUND, token.AT, token.REPEAT, token.UNDERSCORE:
		return true
	}
	return false
}

// parseTypeAtom reads a type with nothing hanging off it.
func (p *parser) parseTypeAtom() ast.Type {
	lo := p.pos()
	switch {
	// A suppressed conformance. It binds tighter than the `&` of a
	// composition — `~Copyable & ~Escapable` suppresses two — so it
	// is read here rather than once per constraint.
	case p.atOper("~"):
		tilde := p.pos()
		p.next()
		return &ast.InverseType{Span: p.span(lo), Tilde: tilde, Base: p.parseTypeAtom()}

	case p.at(token.IDENT):
		return p.parseSimpleType()

	case p.at(token.ANY):
		p.next()
		return &ast.AnyType{Span: p.span(lo)}

	case p.at(token.SELF_TYPE):
		p.next()
		return &ast.SelfType{Span: p.span(lo)}

	case p.at(token.UNDERSCORE):
		p.next()
		return &ast.PlaceholderType{Span: p.span(lo)}

	// A value generic's argument is a number, not a type.
	case p.at(token.INT_LIT), p.atOper("-") && p.peek(1) == token.INT_LIT:
		t := &ast.IntegerType{}
		if !p.at(token.INT_LIT) {
			t.Minus = p.pos()
			p.next()
		}
		p.next()
		t.Span = p.span(lo)
		return t

	case p.at(token.LSQUARE):
		if p.isSizedArrayType() {
			lsquare := p.pos()
			p.next()
			size := p.parseExpr(0)
			of := p.pos()
			p.takeWord("of")
			elem := p.parseType()
			rsquare := p.expect(token.RSQUARE)
			return &ast.SizedArrayType{
				Span:    p.span(lo),
				Lsquare: lsquare,
				Size:    size,
				Of:      of,
				Elem:    elem,
				Rsquare: rsquare,
			}
		}
		lsquare := p.pos()
		p.next()
		first := p.parseType()
		if p.at(token.COLON) {
			colon := p.pos()
			p.next()
			value := p.parseType()
			rsquare := p.expect(token.RSQUARE)
			return &ast.DictType{Span: p.span(lo), Lsquare: lsquare, Key: first,
				Colon: colon, Value: value, Rsquare: rsquare}
		}
		rsquare := p.expect(token.RSQUARE)
		return &ast.ArrayType{Span: p.span(lo), Lsquare: lsquare, Elem: first, Rsquare: rsquare}

	case p.at(token.LPAREN):
		return p.parseType()

	case p.at(token.POUND):
		m := &ast.MacroExpansionType{Pound: p.pos()}
		p.next()
		m.Name = p.expectIdent()
		if p.atLt() {
			m.Args = p.parseGenericArgs()
		}
		if p.at(token.LPAREN) && !p.nl() {
			m.Call = p.parseCallArgs()
		}
		m.Span = p.span(lo)
		return m
	}

	p.errHere("expected a type")
	if !p.at(token.EOF) && !p.nl() {
		p.next()
	}
	return &ast.BadType{Span: p.span(lo)}
}

// parseSimpleType reads a TypeIdentifier: a name, its generic
// arguments, and any dotted members. A member spelled Type or
// Protocol is a MetatypeType instead, which is why this loop reads
// both.
func (p *parser) parseSimpleType() ast.Type {
	lo := p.pos()
	var t ast.Type
	switch {
	case p.at(token.ANY):
		p.next()
		t = &ast.AnyType{Span: p.span(lo)}
	case p.at(token.SELF_TYPE):
		p.next()
		t = &ast.SelfType{Span: p.span(lo)}
	default:
		name := p.expectIdent()
		if name == nil {
			return &ast.BadType{Span: p.span(lo)}
		}
		it := &ast.IdentType{Name: name}
		if p.atLt() {
			it.Args = p.parseGenericArgs()
		}
		it.Span = p.span(lo)
		t = it
	}
	return p.parseMemberTypes(t, lo)
}

// parseMemberTypes reads the dotted tail of a type.
func (p *parser) parseMemberTypes(t ast.Type, lo token.Pos) ast.Type {
	for p.at(token.PERIOD) || p.at(token.PERIOD_PREFIX) {
		if p.peek(1) != token.IDENT {
			return t
		}
		dot := p.pos()
		p.next()
		if p.atWord("Type") || p.atWord("Protocol") {
			kw := p.ident()
			t = &ast.MetatypeType{Span: p.span(lo), Base: t, Dot: dot, Keyword: kw}
			continue
		}
		m := &ast.MemberType{X: t, Dot: dot, Name: p.ident()}
		if p.atLt() {
			m.Args = p.parseGenericArgs()
		}
		m.Span = p.span(lo)
		t = m
	}
	return t
}

// parseTypeSuffixes reads the optional and metatype markers a type
// carries: T?, T!, T.Type.
func (p *parser) parseTypeSuffixes(t ast.Type, lo token.Pos) ast.Type {
	for {
		switch {
		// The marker must touch the type: `Int?` is an optional and
		// `Int ?? x` is an operator. It may also be the head of a
		// longer run — the `?>` that ends [Key: Any]?> is one token.
		case p.atOperHead('?') && p.glued():
			q := p.takeOperChar()
			t = &ast.OptionalType{Span: p.span(lo), Base: t, Question: q}
		case p.atOperHead('!') && p.glued():
			e := p.takeOperChar()
			t = &ast.UnwrappedType{Span: p.span(lo), Base: t, Exclaim: e}
		case (p.at(token.PERIOD) || p.at(token.PERIOD_PREFIX)) && p.peek(1) == token.IDENT:
			t = p.parseMemberTypes(t, lo)
		default:
			return t
		}
	}
}

// ---- parameters ----

// parseParamList reads a ParameterList up to close. defaults says
// whether a parameter may carry `= Expression`, which a declaration's
// may and a function type's may not.
func (p *parser) parseParamList(close token.Kind, defaults bool) []*ast.Param {
	var out []*ast.Param
	for !p.at(close) && !p.at(token.EOF) {
		start := p.i
		out = append(out, p.parseParam(defaults))
		if !p.more(start) {
			break
		}
	}
	return out
}

// parseParam reads one Parameter.
//
// The grammar writes `[ArgumentLabel :] [ParameterModifierList] Type
// [...]` and uses it for a function type and a function declaration
// alike. A declaration's parameter carries two things that leaves
// out — the local name of `func move(to dest: point)`, and a default
// value — so both are read here, and only the default is refused
// where it means nothing. See the README.
func (p *parser) parseParam(defaults bool) *ast.Param {
	lo := p.pos()
	param := &ast.Param{Attrs: p.parseAttrs()}

	switch {
	case p.atParamLabel(0) && p.atParamName(1) && p.peek(2) == token.COLON:
		param.Label = p.ident()
		param.Name = p.ident()
		param.Colon = p.pos()
		p.next()
	case p.atParamLabel(0) && p.peek(1) == token.COLON:
		param.Label = p.ident()
		param.Colon = p.pos()
		p.next()
	}

	for p.atParamModifier() {
		param.Mods = append(param.Mods, p.parseModifier())
	}

	param.Type = p.parseType()
	if p.atOper("...") {
		param.Ellipsis = p.pos()
		p.next()
	}
	if p.at(token.ASSIGN) {
		param.Assign = p.pos()
		p.next()
		param.Default = p.parseExpr(0)
		if !defaults {
			p.errAt(param.Default, "a function type's parameter cannot have a default value")
		}
	}
	param.Span = p.span(lo)
	return param
}

// atParamModifier reports whether a ParameterModifier is here.
// `sending` is not in the grammar's list and is one; see the README.
func (p *parser) atParamModifier() bool {
	return p.at(token.INOUT) || p.atWord("borrowing") || p.atWord("consuming") ||
		p.atWord("isolated") || p.atWord("__owned") || p.atWord("__shared") ||
		(p.atWord("_const") && p.atTypeStartAt(1))
}

// atParamLabel reports whether the token n ahead can be an
// ArgumentLabel, and atParamName whether it can be a parameter's
// local name. Both are the same test: any word may name a parameter —
// `func f(in range: …)` and `func f(_ default: Value)` are both
// ordinary — so a keyword counts, except `inout`, which is a
// ParameterModifier and would be read as one here.
//
// A name that is a keyword still needs backticks at the use site, but
// that is the caller's problem, not this production's.
func (p *parser) atParamLabel(n int) bool {
	switch k := p.peek(n); {
	case k == token.IDENT || k == token.UNDERSCORE:
		return true
	case k == token.INOUT:
		return false
	default:
		return k.IsKeyword()
	}
}

func (p *parser) atParamName(n int) bool { return p.atParamLabel(n) }

// parseThrowsClause reads throws, throws ( Type ), or rethrows.
func (p *parser) parseThrowsClause() *ast.ThrowsClause {
	if !p.at(token.THROWS) && !p.at(token.RETHROWS) {
		return nil
	}
	lo := p.pos()
	c := &ast.ThrowsClause{Keyword: p.pos(), Kind: p.kind()}
	p.next()
	// `throws (E)` names the error type; `throws (x)` after a
	// signature would be a call, so only a type may follow, and only
	// on the same line.
	if c.Kind == token.THROWS && p.at(token.LPAREN) && !p.nl() {
		c.Lparen = p.pos()
		p.next()
		c.Type = p.parseType()
		c.Rparen = p.expect(token.RPAREN)
	}
	c.Span = p.span(lo)
	return c
}

// ---- generics ----

// parseGenericParams reads < GenericParameterList >.
func (p *parser) parseGenericParams() *ast.GenericParams {
	if !p.atLt() {
		return nil
	}
	lo := p.pos()
	g := &ast.GenericParams{Lt: p.takeLt()}
	for !p.atGt() && !p.at(token.EOF) {
		start := p.i
		plo := p.pos()
		param := &ast.GenericParam{Each: p.takeWord("each")}
		if p.at(token.LET) {
			param.Let = p.pos()
			p.next()
		}
		// A generic parameter may be named Self. Nothing else in
		// scope can be, but here the name is being introduced rather
		// than referred to, and Swift admits it.
		if p.at(token.SELF_TYPE) {
			slo := p.pos()
			p.next()
			param.Name = &ast.Ident{Span: p.span(slo)}
		} else {
			param.Name = p.expectIdent()
		}
		param.Inherit = p.parseGenericParamInheritance()
		param.Span = p.span(plo)
		g.Params = append(g.Params, param)
		if !p.more(start) {
			break
		}
	}
	if p.atGt() {
		g.Gt = p.takeGt()
	} else {
		p.errHere("expected '>' to close a generic parameter list")
	}
	g.Span = p.span(lo)
	return g
}

// parseGenericArgs reads < GenericArgumentList >, unconditionally: in
// type position a '<' can be nothing else.
func (p *parser) parseGenericArgs() *ast.GenericArgs {
	lo := p.pos()
	g := &ast.GenericArgs{Lt: p.takeLt()}
	for !p.atGt() && !p.at(token.EOF) {
		start := p.i
		g.Args = append(g.Args, p.parseType())
		if !p.more(start) {
			break
		}
	}
	if p.atGt() {
		g.Gt = p.takeGt()
	} else {
		p.errHere("expected '>' to close a generic argument list")
	}
	g.Span = p.span(lo)
	return g
}

// tryGenericArgs is parseGenericArgs in expression position, where a
// '<' is more often a comparison. It parses speculatively and keeps
// the result only if the '>' is followed by something that may follow
// a generic argument list: `a < b > c` is two comparisons, and
// `f<Int>(x)` is a call.
func (p *parser) tryGenericArgs() *ast.GenericArgs {
	if !p.atLt() {
		return nil
	}
	m := p.mark()
	g := p.parseGenericArgs()
	if !g.Gt.IsValid() || !p.genericArgsFollow() {
		p.reset(m)
		return nil
	}
	return g
}

// genericArgsFollow is the set of tokens that may follow a generic
// argument list in expression position.
func (p *parser) genericArgsFollow() bool {
	switch p.kind() {
	case token.LPAREN, token.RPAREN, token.LSQUARE, token.RSQUARE,
		token.LBRACE, token.RBRACE, token.COMMA, token.COLON, token.SEMI,
		token.PERIOD, token.PERIOD_PREFIX, token.QUESTION_POSTFIX,
		token.EXCLAIM_POSTFIX, token.ASSIGN, token.ARROW:
		return p.split == 0
	}
	return false
}

// parseInheritance reads a TypeInheritanceClause.
func (p *parser) parseInheritance() *ast.InheritanceClause {
	if !p.at(token.COLON) {
		return nil
	}
	lo := p.pos()
	c := &ast.InheritanceClause{Colon: p.pos()}
	p.next()
	for {
		ilo := p.pos()
		item := &ast.InheritanceItem{Attrs: p.parseAttrs()}
		item.Nonisolated = p.takeWordBeforeType("nonisolated")
		item.Type = p.parseType()
		item.Span = p.span(ilo)
		c.Items = append(c.Items, item)
		if !p.at(token.COMMA) {
			break
		}
		p.next()
	}
	c.Span = p.span(lo)
	return c
}

// parseGenericParamInheritance reads a single-item TypeInheritanceClause
// for a generic parameter. In a generic parameter list, comma separates
// parameters, never inheritance items.
func (p *parser) parseGenericParamInheritance() *ast.InheritanceClause {
	if !p.at(token.COLON) {
		return nil
	}
	lo := p.pos()
	c := &ast.InheritanceClause{Colon: p.pos()}
	p.next()
	ilo := p.pos()
	item := &ast.InheritanceItem{Attrs: p.parseAttrs()}
	item.Nonisolated = p.takeWordBeforeType("nonisolated")
	item.Type = p.parseType()
	item.Span = p.span(ilo)
	c.Items = []*ast.InheritanceItem{item}
	c.Span = p.span(lo)
	return c
}

// parseGenericWhere reads a GenericWhereClause.
func (p *parser) parseGenericWhere() *ast.GenericWhereClause {
	if !p.at(token.WHERE) {
		return nil
	}
	lo := p.pos()
	w := &ast.GenericWhereClause{Where: p.pos()}
	p.next()
	for {
		rlo := p.pos()
		left := p.parseType()
		switch {
		case p.at(token.COLON):
			r := &ast.ConformanceReq{Left: left, Colon: p.pos()}
			p.next()
			r.Right = p.parseType()
			r.Span = p.span(rlo)
			w.Reqs = append(w.Reqs, r)
		case p.atOper("=="):
			r := &ast.SameTypeReq{Left: left, EqEq: p.pos()}
			p.next()
			r.Right = p.parseType()
			r.Span = p.span(rlo)
			w.Reqs = append(w.Reqs, r)
		default:
			p.errHere("expected ':' or '==' in a generic requirement")
		}
		if !p.at(token.COMMA) {
			break
		}
		p.next()
	}
	w.Span = p.span(lo)
	return w
}

func (p *parser) isSizedArrayType() bool {
	n := 1
	parenDepth := 0
	bracketDepth := 0
	for {
		k := p.peek(n)
		if k == token.EOF {
			return false
		}
		if bracketDepth == 0 && parenDepth == 0 {
			if k == token.RSQUARE || k == token.COLON {
				return false
			}
			if p.wordAt(n, "of") {
				return true
			}
		}
		switch k {
		case token.LPAREN:
			parenDepth++
		case token.RPAREN:
			if parenDepth > 0 {
				parenDepth--
			}
		case token.LSQUARE:
			bracketDepth++
		case token.RSQUARE:
			if bracketDepth > 0 {
				bracketDepth--
			}
		}
		n++
	}
}
