package parser

import (
	"fmt"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// parseDecl reads a Declaration. Its attributes and modifiers come
// first because every form admits them, and some forms are named by
// one: `prefix operator` is an OperatorDeclaration whose fixity is
// written as a modifier.
func (p *parser) parseDecl() ast.Decl {
	p.depth++
	defer func() { p.depth-- }()
	lo := p.pos()
	if p.tooDeep() {
		return &ast.BadDecl{Span: p.span(lo)}
	}

	attrs := p.parseAttrs()
	mods := p.parseModifiers()

	switch {
	case p.at(token.IMPORT):
		return p.parseImport(lo, attrs, mods)
	case p.at(token.LET), p.at(token.VAR):
		return p.parseVarDecl(lo, attrs, mods)
	case p.at(token.TYPEALIAS):
		return p.parseTypealias(lo, attrs, mods)
	case p.at(token.FUNC):
		return p.parseFunc(lo, attrs, mods)
	case p.at(token.ENUM):
		return p.parseEnum(lo, attrs, mods)
	case p.at(token.CASE):
		return p.parseEnumCase(lo, attrs, mods)
	case p.at(token.STRUCT):
		return p.parseStruct(lo, attrs, mods)
	case p.at(token.CLASS):
		return p.parseClass(lo, attrs, mods)
	case p.at(token.PROTOCOL):
		return p.parseProtocol(lo, attrs, mods)
	case p.at(token.INIT):
		return p.parseInit(lo, attrs, mods)
	case p.at(token.DEINIT):
		return p.parseDeinit(lo, attrs, mods)
	case p.at(token.EXTENSION):
		return p.parseExtension(lo, attrs, mods)
	case p.at(token.SUBSCRIPT):
		return p.parseSubscript(lo, attrs, mods)
	case p.at(token.OPERATOR):
		return p.parseOperatorDecl(lo, attrs, mods)
	case p.at(token.PRECEDENCEGROUP):
		return p.parsePrecedenceGroup(lo)
	case p.at(token.ASSOCIATEDTYPE):
		return p.parseAssociatedType(lo, attrs, mods)
	case p.atWord("actor"):
		return p.parseActor(lo, attrs, mods)
	case p.atWord("macro"):
		return p.parseMacro(lo, attrs, mods)
	}

	p.errHere("expected a declaration")
	p.advanceTo(declFollow)
	if p.at(token.SEMI) {
		p.next()
	}
	return &ast.BadDecl{Span: p.span(lo)}
}

// ---- import ----

func (p *parser) parseImport(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.ImportDecl{Attrs: attrs, Mods: mods, Import: p.pos()}
	p.next()
	for _, m := range mods {
		if !p.isAccessLevel(m) {
			p.errAt(m, "an import declaration takes only an access-level modifier")
		}
	}

	// The ImportKind says what one name in the module is being
	// imported: `import func Darwin.sqrt`.
	switch k := p.kind(); k {
	case token.TYPEALIAS, token.STRUCT, token.CLASS, token.ENUM,
		token.PROTOCOL, token.LET, token.VAR, token.FUNC:
		d.Kind, d.KindPos = k, p.pos()
		p.next()
	default:
		if p.atWord("macro") && p.peek(1) == token.IDENT {
			d.Kind, d.KindPos = token.IDENT, p.pos()
			p.next()
		}
	}

	for {
		// A submodule may be named by an operator — `import Swift.+` —
		// so any single token that is not a dot ends one step.
		id := p.expectIdent()
		if id == nil {
			break
		}
		d.Path = append(d.Path, id)
		if !p.at(token.PERIOD) {
			break
		}
		p.next()
	}
	d.Span = p.span(lo)
	return d
}

// ---- let and var ----

func (p *parser) parseVarDecl(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.VarDecl{Attrs: attrs, Mods: mods, Keyword: p.pos(), Kind: p.kind()}
	p.next()

	for {
		blo := p.pos()
		b := &ast.PatternBinding{Pat: p.parsePattern(patternBinding)}
		if p.at(token.ASSIGN) {
			b.Assign = p.pos()
			p.next()
			b.Value = p.parseExpr(0)
		}
		// The brace need not be on the declaration's own line —
		// `var v: Int` and then `{ get { … } }` below it is ordinary
		// — but where a line break separates them, only a brace that
		// opens accessors, or one that is the getter of a binding
		// with no initializer, belongs to the declaration. Anything
		// else on a line of its own is a statement.
		if p.at(token.LBRACE) && (!p.nl() || p.atAccessorBlock() || b.Value == nil) {
			if p.atAccessorBlock() {
				b.Accessors = p.parseAccessorBlock()
			} else {
				b.Body = p.parseCodeBlock()
			}
			if d.Kind == token.LET {
				p.errAt(b, "a 'let' declaration cannot have accessors")
			}
		}
		b.Span = p.span(blo)
		d.Bindings = append(d.Bindings, b)
		if !p.at(token.COMMA) {
			break
		}
		p.next()
	}
	d.Span = p.span(lo)
	return d
}

// atAccessorBlock reports whether the brace at the cursor opens a
// getter and setter block rather than a body. The introducers are
// contextual words, so this is what tells `{ get { … } }` from `{
// get() }` — a body that calls something called get.
func (p *parser) atAccessorBlock() bool {
	n := p.skipAttrsAt(1) // an accessor may carry attributes
	for p.peek(n) == token.IDENT && modifierWords[p.text(p.peekTok(n))] {
		n = p.afterModifierAt(n)
	}
	if !p.accessorWordAt(n) {
		return false
	}
	// `get` opening an accessor is followed by its body, its
	// parameter, an effect, or the end of the block. A call to a
	// function named get is followed by its arguments.
	switch p.peek(n + 1) {
	case token.LBRACE, token.RBRACE, token.THROWS, token.AT, token.LPAREN:
		return true
	}
	return p.wordAt(n+1, "async") || p.accessorWordAt(n+1) ||
		p.wordAt(n+1, "mutating") || p.wordAt(n+1, "nonmutating")
}

// accessorWordAt reports whether the token n ahead introduces an
// accessor. The grammar has get, set, willSet and didSet; the
// underscored four are the addressor and coroutine accessors every
// .swiftinterface in the SDK is written with. See the README.
func (p *parser) accessorWordAt(n int) bool {
	switch {
	case p.wordAt(n, "get"), p.wordAt(n, "set"),
		p.wordAt(n, "willSet"), p.wordAt(n, "didSet"),
		p.wordAt(n, "_read"), p.wordAt(n, "_modify"),
		p.wordAt(n, "unsafeAddress"), p.wordAt(n, "unsafeMutableAddress"):
		return true
	}
	return false
}

// isCoroutine reports whether an accessor is one of the two that
// yield rather than return.
func (p *parser) isCoroutine(kw *ast.Ident) bool {
	if kw == nil {
		return false
	}
	switch string(p.f.Slice(kw.Lo, kw.Hi)) {
	case "_read", "_modify":
		return true
	}
	return false
}

func (p *parser) parseAccessorBlock() *ast.AccessorBlock {
	lo := p.pos()
	b := &ast.AccessorBlock{Lbrace: p.expect(token.LBRACE)}
	for !p.at(token.RBRACE) && !p.at(token.EOF) {
		start := p.i
		alo := p.pos()
		a := &ast.Accessor{Attrs: p.parseAttrs()}
		for p.at(token.IDENT) && modifierWords[p.text(p.tok())] {
			a.Mods = append(a.Mods, p.parseModifier())
		}
		switch {
		case p.accessorWordAt(0):
			a.Keyword = p.ident()
		default:
			p.errHere("expected 'get', 'set', 'willSet' or 'didSet'")
			p.advanceTo(map[token.Kind]bool{token.RBRACE: true})
			continue
		}
		if p.at(token.LPAREN) {
			a.Lparen = p.pos()
			p.next()
			a.Name = p.expectIdent()
			a.Rparen = p.expect(token.RPAREN)
		}
		a.Async = p.takeWord("async")
		a.Throws = p.parseThrowsClause()
		if p.at(token.LBRACE) {
			was := p.inCoroutine
			p.inCoroutine = p.isCoroutine(a.Keyword)
			a.Body = p.parseCodeBlock()
			p.inCoroutine = was
		}
		a.Span = p.span(alo)
		b.Accessors = append(b.Accessors, a)
		if p.i == start {
			p.next()
		}
	}
	b.Rbrace = p.expect(token.RBRACE)
	b.Span = p.span(lo)
	return b
}

// ---- typealias and associatedtype ----

func (p *parser) parseTypealias(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.TypealiasDecl{Attrs: attrs, Mods: mods, Keyword: p.pos()}
	p.next()
	d.Name = p.expectIdent()
	d.Generics = p.parseGenericParams()
	d.Assign = p.expect(token.ASSIGN)
	d.Type = p.parseType()
	d.Where = p.parseGenericWhere()
	d.Span = p.span(lo)
	return d
}

func (p *parser) parseAssociatedType(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.AssociatedTypeDecl{Attrs: attrs, Mods: mods, Keyword: p.pos()}
	p.next()
	d.Name = p.expectIdent()
	d.Inherit = p.parseInheritance()
	if p.at(token.ASSIGN) {
		d.Assign = p.pos()
		p.next()
		d.Type = p.parseType()
	}
	d.Where = p.parseGenericWhere()
	d.Span = p.span(lo)
	return d
}

// ---- functions, initializers, subscripts ----

func (p *parser) parseFunc(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.FuncDecl{Attrs: attrs, Mods: mods, Func: p.pos()}
	p.next()
	d.Name = p.parseFuncName()
	d.Generics = p.parseGenericParams()
	d.Sig = p.parseFuncSig()
	d.Where = p.parseGenericWhere()
	if p.at(token.LBRACE) {
		d.Body = p.parseCodeBlock()
	}
	d.Span = p.span(lo)
	return d
}

// parseFuncName reads a FunctionName: an Identifier or an Operator.
// An operator's name is its spelling, so it is recorded as an
// identifier over the operator's span.
func (p *parser) parseFuncName() *ast.Ident {
	if p.atAnyOper() || p.at(token.ASSIGN) || p.at(token.QUESTION_INFIX) ||
		p.at(token.QUESTION_POSTFIX) || p.at(token.EXCLAIM_POSTFIX) ||
		p.at(token.AMP_PREFIX) {
		lo := p.pos()
		p.next()
		return &ast.Ident{Span: p.span(lo)}
	}
	return p.expectIdent()
}

func (p *parser) parseFuncSig() *ast.FuncSig {
	lo := p.pos()
	s := &ast.FuncSig{Lparen: p.expect(token.LPAREN)}
	s.Params = p.parseParamList(token.RPAREN, true)
	s.Rparen = p.expect(token.RPAREN)
	s.Async = p.takeWord("async")
	s.Throws = p.parseThrowsClause()
	if p.at(token.ARROW) {
		rlo := p.pos()
		arrow := p.pos()
		p.next()
		t := p.parseType()
		s.Result = &ast.FuncResult{Span: p.span(rlo), Arrow: arrow, Type: t}
	}
	s.Span = p.span(lo)
	return s
}

func (p *parser) parseInit(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.InitDecl{Attrs: attrs, Mods: mods, Init: p.pos()}
	p.next()
	// The marker of a failable initializer, which may be glued to the
	// '<' of a generic parameter list: `init?<T>(…)` is one token.
	switch {
	case p.atOperHead('?'):
		d.Question = p.takeOperChar()
	case p.atOperHead('!'):
		d.Exclaim = p.takeOperChar()
	}
	d.Generics = p.parseGenericParams()
	d.Sig = p.parseFuncSig()
	if d.Sig.Result != nil {
		p.errAt(d.Sig.Result, "an initializer cannot have a return type")
	}
	d.Where = p.parseGenericWhere()
	if p.at(token.LBRACE) {
		d.Body = p.parseCodeBlock()
	}
	d.Span = p.span(lo)
	return d
}

func (p *parser) parseDeinit(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.DeinitDecl{Attrs: attrs, Mods: mods, Keyword: p.pos()}
	p.next()
	// The body is optional here, as it is for a function: whether one
	// is required depends on what the file is — a source file, a
	// protocol's requirements, a serialized module interface — and
	// that is not a question about syntax.
	if p.at(token.LBRACE) {
		d.Body = p.parseCodeBlock()
	}
	d.Span = p.span(lo)
	return d
}

func (p *parser) parseSubscript(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.SubscriptDecl{Attrs: attrs, Mods: mods, Keyword: p.pos()}
	p.next()
	d.Generics = p.parseGenericParams()
	d.Lparen = p.expect(token.LPAREN)
	d.Params = p.parseParamList(token.RPAREN, true)
	d.Rparen = p.expect(token.RPAREN)
	if p.at(token.ARROW) {
		rlo := p.pos()
		arrow := p.pos()
		p.next()
		t := p.parseType()
		d.Result = &ast.FuncResult{Span: p.span(rlo), Arrow: arrow, Type: t}
	} else {
		p.errHere("a subscript must declare its result type with '->'")
	}
	d.Where = p.parseGenericWhere()
	if p.at(token.LBRACE) {
		if p.atAccessorBlock() {
			d.Accessors = p.parseAccessorBlock()
		} else {
			d.Body = p.parseCodeBlock()
		}
	}
	d.Span = p.span(lo)
	return d
}

// ---- type declarations ----

func (p *parser) parseEnum(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.EnumDecl{Attrs: attrs, Mods: mods, Enum: p.pos()}
	p.next()
	d.Name = p.expectIdent()
	d.Generics = p.parseGenericParams()
	d.Inherit = p.parseInheritance()
	d.Where = p.parseGenericWhere()
	d.Body = p.parseMemberBlock(inEnum)
	d.Span = p.span(lo)
	return d
}

// parseEnumCase reads an EnumCaseDeclaration. The associated values
// are a tuple type: `case point(x: Int, y: Int)` declares two values
// of a type, which is what a case's parentheses hold.
func (p *parser) parseEnumCase(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.EnumCaseDecl{Attrs: attrs, Mods: mods, Case: p.pos()}
	for _, m := range mods {
		if p.text(token.Token{Pos: m.Pos(), End: m.End()}) == "indirect" {
			d.Indirect = m.Pos()
		}
	}
	p.next()
	for {
		elo := p.pos()
		e := &ast.EnumCaseElem{Name: p.expectIdent()}
		if p.at(token.LPAREN) && !p.nl() {
			e.Lparen = p.pos()
			p.next()
			e.Params = p.parseParamList(token.RPAREN, true)
			for _, param := range e.Params {
				if len(param.Mods) > 0 {
					p.errAt(param.Mods[0], "a case's associated value takes no parameter modifiers")
				}
				if param.Name != nil {
					p.errAt(param.Name, "a case's associated value takes one name")
				}
			}
			e.Rparen = p.expect(token.RPAREN)
		}
		if p.at(token.ASSIGN) {
			e.Assign = p.pos()
			p.next()
			e.Value = p.parseExpr(0)
			p.checkRawValue(e.Value)
		}
		e.Span = p.span(elo)
		d.Elements = append(d.Elements, e)
		if !p.at(token.COMMA) {
			break
		}
		p.next()
	}
	d.Span = p.span(lo)
	return d
}

// checkRawValue enforces RawValueLiteral: a raw value is a number, a
// static string, or a boolean, and nothing else — not nil, not a
// regex, not an expression that would have to be evaluated.
func (p *parser) checkRawValue(x ast.Expr) {
	switch v := x.(type) {
	case *ast.BasicLit:
		switch v.Kind {
		case token.INT_LIT, token.FLOAT_LIT, token.TRUE, token.FALSE:
			return
		}
	case *ast.StringLit:
		for _, seg := range v.Segments {
			if _, ok := seg.(*ast.Interpolation); ok {
				p.errAt(seg, "a raw value must be a literal, and an interpolated string is not one")
				return
			}
		}
		return
	case *ast.PrefixExpr:
		// The '-' of a negative NumericLiteral.
		if lit, ok := v.X.(*ast.BasicLit); ok &&
			(lit.Kind == token.INT_LIT || lit.Kind == token.FLOAT_LIT) {
			return
		}
	}
	p.errAt(x, "a raw value must be a number, a string, or a boolean literal")
}

func (p *parser) parseStruct(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.StructDecl{Attrs: attrs, Mods: mods, Struct: p.pos()}
	p.next()
	d.Name = p.expectIdent()
	d.Generics = p.parseGenericParams()
	d.Inherit = p.parseInheritance()
	d.Where = p.parseGenericWhere()
	d.Body = p.parseMemberBlock(inType)
	d.Span = p.span(lo)
	return d
}

func (p *parser) parseClass(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.ClassDecl{Attrs: attrs, Mods: mods, Class: p.pos()}
	p.next()
	d.Name = p.expectIdent()
	d.Generics = p.parseGenericParams()
	d.Inherit = p.parseInheritance()
	d.Where = p.parseGenericWhere()
	d.Body = p.parseMemberBlock(inType)
	d.Span = p.span(lo)
	return d
}

func (p *parser) parseActor(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.ActorDecl{Attrs: attrs, Mods: mods, Actor: p.pos()}
	p.next()
	d.Name = p.expectIdent()
	d.Generics = p.parseGenericParams()
	d.Inherit = p.parseInheritance()
	d.Where = p.parseGenericWhere()
	d.Body = p.parseMemberBlock(inType)
	d.Span = p.span(lo)
	return d
}

func (p *parser) parseProtocol(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.ProtocolDecl{Attrs: attrs, Mods: mods, Protocol: p.pos()}
	p.next()
	d.Name = p.expectIdent()
	d.Primary = p.parseGenericParams()
	d.Inherit = p.parseInheritance()
	d.Where = p.parseGenericWhere()
	d.Body = p.parseMemberBlock(inProtocol)
	d.Span = p.span(lo)
	return d
}

func (p *parser) parseExtension(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.ExtensionDecl{Attrs: attrs, Mods: mods, Extension: p.pos()}
	p.next()
	d.Type = p.parseSimpleType()
	d.Inherit = p.parseInheritance()
	d.Where = p.parseGenericWhere()
	d.Body = p.parseMemberBlock(inType)
	d.Span = p.span(lo)
	return d
}

// ---- operators and precedence groups ----

func (p *parser) parseOperatorDecl(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.OperatorDecl{Attrs: attrs, Operator: p.pos()}
	p.next()

	// The fixity is written in front of the keyword, where a modifier
	// goes, so that is what it was parsed as.
	for _, m := range mods {
		switch p.text(token.Token{Pos: m.Pos(), End: m.End()}) {
		case "prefix", "postfix", "infix":
			d.Fixity = m.Name
		default:
			p.errAt(m, "an operator declaration takes only 'prefix', 'postfix' or 'infix'")
		}
	}
	if d.Fixity == nil {
		p.errAt(d, "an operator declaration must say 'prefix', 'postfix' or 'infix'")
	}

	if p.atAnyOper() || p.at(token.ASSIGN) || p.at(token.QUESTION_INFIX) ||
		p.at(token.QUESTION_POSTFIX) || p.at(token.EXCLAIM_POSTFIX) ||
		p.at(token.AMP_PREFIX) || p.at(token.PERIOD) || p.at(token.PERIOD_PREFIX) {
		nlo := p.pos()
		p.next()
		d.Name = &ast.Ident{Span: p.span(nlo)}
	} else {
		p.errHere("expected an operator")
	}

	if p.at(token.COLON) {
		d.Colon = p.pos()
		p.next()
		d.Group = p.expectIdent()
	}
	d.Span = p.span(lo)
	return d
}

func (p *parser) parsePrecedenceGroup(lo token.Pos) ast.Decl {
	d := &ast.PrecedenceGroupDecl{Keyword: p.pos()}
	p.next()
	d.Name = p.expectIdent()
	d.Lbrace = p.expect(token.LBRACE)
	for !p.at(token.RBRACE) && !p.at(token.EOF) {
		start := p.i
		alo := p.pos()
		switch {
		case p.atWord("higherThan"), p.atWord("lowerThan"):
			r := &ast.PrecedenceRelation{Keyword: p.ident()}
			r.Colon = p.expect(token.COLON)
			for {
				n := p.expectIdent()
				if n == nil {
					break
				}
				r.Names = append(r.Names, n)
				if !p.at(token.COMMA) {
					break
				}
				p.next()
			}
			r.Span = p.span(alo)
			d.Attrs = append(d.Attrs, r)

		case p.atWord("assignment"):
			a := &ast.PrecedenceAssignment{Keyword: p.ident()}
			a.Colon = p.expect(token.COLON)
			if p.at(token.TRUE) || p.at(token.FALSE) {
				vlo := p.pos()
				k := p.kind()
				p.next()
				a.Value = &ast.BasicLit{Span: p.span(vlo), Kind: k}
			} else {
				p.errHere("expected 'true' or 'false'")
			}
			a.Span = p.span(alo)
			d.Attrs = append(d.Attrs, a)

		case p.atWord("associativity"):
			a := &ast.PrecedenceAssociativity{Keyword: p.ident()}
			a.Colon = p.expect(token.COLON)
			if p.atWord("left") || p.atWord("right") || p.atWord("none") {
				a.Value = p.ident()
			} else {
				p.errHere("expected 'left', 'right' or 'none'")
			}
			a.Span = p.span(alo)
			d.Attrs = append(d.Attrs, a)

		default:
			p.errHere("expected 'higherThan', 'lowerThan', 'assignment' or 'associativity'")
			p.advanceTo(map[token.Kind]bool{token.RBRACE: true})
		}
		if p.i == start {
			p.next()
		}
	}
	d.Rbrace = p.expect(token.RBRACE)
	d.Span = p.span(lo)
	return d
}

func (p *parser) parseMacro(lo token.Pos, attrs []*ast.Attr, mods []*ast.Modifier) ast.Decl {
	d := &ast.MacroDecl{Attrs: attrs, Mods: mods, Keyword: p.pos()}
	p.next()
	d.Name = p.expectIdent()
	d.Generics = p.parseGenericParams()
	d.Sig = p.parseFuncSig()
	if p.at(token.ASSIGN) {
		d.Assign = p.pos()
		p.next()
		d.Expansion = p.parseType()
	}
	d.Where = p.parseGenericWhere()
	d.Span = p.span(lo)
	return d
}

// ---- member blocks ----

// container says which member list the grammar gives the declaration
// being read, so a member that is not in it can be named as such.
type container int

const (
	inType container = iota
	inEnum
	inProtocol
)

func (c container) String() string {
	switch c {
	case inEnum:
		return "an enum"
	case inProtocol:
		return "a protocol"
	}
	return "a type"
}

func (p *parser) parseMemberBlock(c container) *ast.MemberBlock {
	lo := p.pos()
	b := &ast.MemberBlock{Lbrace: p.expect(token.LBRACE)}
	for !p.at(token.RBRACE) && !p.at(token.EOF) {
		start := p.i
		switch {
		case p.at(token.SEMI):
			p.next()
			continue
		case p.at(token.POUND_IF):
			b.Members = append(b.Members, p.parseIfConfigMembers(c))
		default:
			d := p.parseDecl()
			p.checkMember(c, d)
			b.Members = append(b.Members, d)
		}
		if p.i == start {
			p.advanceTo(declFollow)
			if p.at(token.SEMI) {
				p.next()
			}
			if p.i == start {
				p.next()
			}
		}
	}
	b.Rbrace = p.expect(token.RBRACE)
	b.Span = p.span(lo)
	return b
}

// parseIfConfigMembers reads a conditional compilation block among
// members. Its clauses hold declarations, which are statements, so
// each arrives wrapped the way it would anywhere else.
func (p *parser) parseIfConfigMembers(c container) ast.Stmt {
	lo := p.pos()
	blk := &ast.IfConfigStmt{}
	for {
		clo := p.pos()
		cl := &ast.IfConfigClause{Pound: p.pos(), Kind: p.kind()}
		p.next()
		if cl.Kind != token.POUND_ELSE {
			cl.Cond = p.parseCompilationCond()
		}
		for !p.at(token.RBRACE) && !p.at(token.EOF) &&
			!p.at(token.POUND_ELSEIF) && !p.at(token.POUND_ELSE) && !p.at(token.POUND_ENDIF) {
			start := p.i
			if p.at(token.SEMI) {
				p.next()
				continue
			}
			if p.at(token.POUND_IF) {
				cl.Stmts = append(cl.Stmts, p.parseIfConfigMembers(c))
			} else {
				d := p.parseDecl()
				p.checkMember(c, d)
				cl.Stmts = append(cl.Stmts, &ast.DeclStmt{
					Span: ast.Span{Lo: d.Pos(), Hi: d.End()}, D: d})
			}
			if p.i == start {
				p.next()
			}
		}
		cl.Span = p.span(clo)
		blk.Clauses = append(blk.Clauses, cl)
		if !p.at(token.POUND_ELSEIF) && !p.at(token.POUND_ELSE) {
			break
		}
	}
	blk.Endif = p.expect(token.POUND_ENDIF)
	blk.Span = p.span(lo)
	return blk
}

// checkMember reports a member the grammar does not give this
// container. The parse is not narrowed to the member list — reading
// the declaration and then saying what is wrong with it beats
// refusing to read it — but the grammar is still what decides.
func (p *parser) checkMember(c container, d ast.Decl) {
	if _, ok := d.(*ast.EnumCaseDecl); ok && c != inEnum {
		p.errAt(d, fmt.Sprintf("an enum case is not allowed in %s", c))
		return
	}
	if c != inProtocol {
		if _, ok := d.(*ast.AssociatedTypeDecl); ok {
			p.errAt(d, fmt.Sprintf("an associated type is not allowed in %s", c))
		}
		return
	}
	// A protocol declares requirements, not implementations.
	switch d := d.(type) {
	case *ast.FuncDecl:
		if d.Body != nil {
			p.errAt(d.Body, "a protocol's method requirement cannot have a body")
		}
	case *ast.InitDecl:
		if d.Body != nil {
			p.errAt(d.Body, "a protocol's initializer requirement cannot have a body")
		}
	case *ast.SubscriptDecl:
		if d.Body != nil {
			p.errAt(d.Body, "a protocol's subscript requirement cannot have a body")
		}
	case *ast.VarDecl:
		for _, b := range d.Bindings {
			switch {
			case b.Body != nil:
				p.errAt(b.Body, "a protocol's property requirement cannot have a body")
			case b.Accessors == nil:
				p.errAt(b, "a protocol's property requirement must say '{ get }' or '{ get set }'")
			case b.Value != nil:
				p.errAt(b.Value, "a protocol's property requirement cannot have an initializer")
			}
		}
	case *ast.EnumDecl, *ast.StructDecl, *ast.ClassDecl, *ast.ActorDecl,
		*ast.ExtensionDecl, *ast.DeinitDecl, *ast.MacroDecl, *ast.OperatorDecl:
		p.errAt(d, "this declaration is not allowed in a protocol")
	}
}
