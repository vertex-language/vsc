package parser

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// parseAttrs reads an AttributeList: zero or more @ Name
// [BalancedTokens].
//
// An attribute's arguments are kept as tokens. The grammar says
// BalancedTokens and means it — @available's platform list and
// @objc's selector are each their own small language — so they are
// matched for balance and otherwise left alone.
func (p *parser) parseAttrs() []*ast.Attr {
	var out []*ast.Attr
	for p.at(token.AT) {
		lo := p.pos()
		a := &ast.Attr{At: p.pos()}
		p.next()
		a.Name = p.parseAttrName()

		// The argument clause is glued to the name — `@available(…)`
		// — but an attribute that takes no arguments never opens one,
		// so the paren of `@escaping() -> Void` belongs to the
		// function type. Swift knows each attribute's arity; this is
		// the half of that table the difference turns on.
		if p.at(token.LPAREN) && p.tok().Flags.Has(token.FlagAdjacent) &&
			!bareAttrs[attrName(p.f, a.Name)] {
			a.Lparen = p.pos()
			p.next()
			a.Tokens = p.balancedTokens()
			a.Rparen = p.expect(token.RPAREN)
		}
		a.Span = p.span(lo)
		out = append(out, a)
	}
	return out
}

// parseAttrName reads an AttributeName: an Identifier or a
// TypeIdentifier. A reserved word counts — an attribute's name is in
// a namespace of its own, and @rethrows is one of them.
func (p *parser) parseAttrName() ast.Type {
	lo := p.pos()
	if p.kind().IsKeyword() {
		nlo := p.pos()
		p.next()
		t := &ast.IdentType{Name: &ast.Ident{Span: p.span(nlo)}}
		t.Span = p.span(lo)
		return t
	}
	return p.parseSimpleType()
}

// bareAttrs are the attributes that take no arguments and are
// written where a parenthesis can follow them for another reason:
// before a function type, whose parameters open with one.
var bareAttrs = map[string]bool{
	"autoclosure": true, "concurrent": true, "escaping": true,
	"noescape": true, "preconcurrency": true, "retroactive": true,
	"Sendable": true, "unchecked": true,
}

// attrName is an attribute's spelling, or "" if it is qualified.
func attrName(f *token.File, t ast.Type) string {
	if id, ok := t.(*ast.IdentType); ok && id.Name != nil {
		return string(f.Slice(id.Name.Lo, id.Name.Hi))
	}
	return ""
}

// balancedTokens collects tokens up to the ')' that closes the group
// already opened, keeping brackets balanced.
func (p *parser) balancedTokens() []token.Token {
	var out []token.Token
	depth := 0
	for !p.at(token.EOF) {
		switch p.kind() {
		case token.RPAREN:
			if depth == 0 {
				return out
			}
			depth--
		case token.LPAREN:
			depth++
		case token.LSQUARE, token.LBRACE:
			depth++
		case token.RSQUARE, token.RBRACE:
			depth--
			if depth < 0 {
				return out // unbalanced: let the caller's expect report it
			}
		}
		out = append(out, p.tok())
		p.next()
	}
	return out
}

// ---- modifiers ----

// modifierWords are the DeclarationModifiers that are contextual
// keywords — which is most of them. Only the access levels, class and
// static are reserved words.
var modifierWords = map[string]bool{
	"convenience": true, "dynamic": true, "final": true, "infix": true,
	"lazy": true, "mutating": true, "nonmutating": true, "nonisolated": true,
	"isolated": true, "optional": true, "override": true, "postfix": true,
	"prefix": true, "required": true, "unowned": true, "weak": true,
	"open": true, "package": true, "async": true,
	// Not in the grammar's DeclarationModifier list, and modifiers all
	// the same: `indirect enum`, `consuming func`, `async let`. See the README.
	"indirect": true, "borrowing": true, "consuming": true,
	"distributed": true,
	// The underscored spellings. They are SPI, and every module
	// interface in an SDK is written with them.
	"__consuming": true, "__owned": true, "__shared": true,
	"_const": true, "_local": true,
}

// isAccessLevel reports whether m is one of the six access levels.
// Two of them are contextual words, so this is not a Kind test.
func (p *parser) isAccessLevel(m *ast.Modifier) bool {
	switch m.Kind {
	case token.PUBLIC, token.PRIVATE, token.FILEPRIVATE, token.INTERNAL:
		return true
	case token.IDENT:
		if m.Name == nil {
			return false
		}
		switch string(p.f.Slice(m.Name.Lo, m.Name.Hi)) {
		case "open", "package":
			return true
		}
	}
	return false
}

// isModifierKeyword reports whether a reserved word is a declaration
// modifier. `class` is one in `class func`, and the head of a
// declaration in `class Foo` — either way a declaration follows, so
// the caller need not tell them apart to know that much.
func isModifierKeyword(k token.Kind) bool {
	switch k {
	case token.STATIC, token.PUBLIC, token.PRIVATE, token.INTERNAL,
		token.FILEPRIVATE, token.CLASS:
		return true
	}
	return false
}

// parseModifier reads one DeclarationModifier, including a
// parenthesized argument: private(set), unowned(unsafe).
func (p *parser) parseModifier() *ast.Modifier {
	lo := p.pos()
	m := &ast.Modifier{Kind: p.kind()}
	m.Name = p.ident()
	if p.at(token.LPAREN) && p.tok().Flags.Has(token.FlagAdjacent) {
		m.Lparen = p.pos()
		p.next()
		if p.at(token.IDENT) {
			m.Arg = p.ident()
		} else {
			p.errHere("expected a modifier argument")
		}
		m.Rparen = p.expect(token.RPAREN)
	}
	m.Span = p.span(lo)
	return m
}

// parseModifiers reads a run of them.
func (p *parser) parseModifiers() []*ast.Modifier {
	var out []*ast.Modifier
	for p.atModifier() {
		out = append(out, p.parseModifier())
	}
	return out
}

// atModifier reports whether a modifier is here — which, for the
// contextual ones, means a declaration follows it. `final class C` is
// a modifier; `final = 3` is an assignment to something called final.
func (p *parser) atModifier() bool {
	switch {
	case p.at(token.CLASS):
		return p.peek(1) != token.IDENT // `class Foo` declares; `class func` modifies
	case isModifierKeyword(p.kind()):
		return true
	case p.at(token.IDENT) && modifierWords[p.text(p.tok())] &&
		!p.tok().Flags.Has(token.FlagEscaped):
		return p.declStartAt(p.afterModifierAt(0))
	}
	return false
}

// afterModifierAt returns the index just past the modifier at n,
// including its parenthesized argument.
func (p *parser) afterModifierAt(n int) int {
	n++
	if p.peek(n) == token.LPAREN && p.peekTok(n).Flags.Has(token.FlagAdjacent) {
		return p.skipGroupAt(n)
	}
	return n
}

// skipGroupAt returns the index just past the balanced group opening
// at n.
func (p *parser) skipGroupAt(n int) int {
	depth := 0
	for {
		switch p.peek(n) {
		case token.EOF:
			return n
		case token.LPAREN, token.LSQUARE, token.LBRACE:
			depth++
		case token.RPAREN, token.RSQUARE, token.RBRACE:
			depth--
		}
		n++
		if depth <= 0 {
			return n
		}
	}
}

// skipAttrsAt returns the index just past the attributes at n. An
// attribute's name may be a reserved word — @rethrows — so any single
// token after the '@' is one.
func (p *parser) skipAttrsAt(n int) int {
	for p.peek(n) == token.AT {
		n++
		if k := p.peek(n); k == token.IDENT || k.IsKeyword() {
			n++
		}
		for p.peek(n) == token.PERIOD && p.peek(n+1) == token.IDENT {
			n += 2 // a TypeIdentifier: @Foo.Bar
		}
		if p.peek(n) == token.LPAREN && p.peekTok(n).Flags.Has(token.FlagAdjacent) {
			n = p.skipGroupAt(n)
		}
	}
	return n
}

// declStartAt reports whether a declaration begins at token n,
// stepping over the attributes and modifiers in front of it.
func (p *parser) declStartAt(n int) bool {
	n = p.skipAttrsAt(n)
	for {
		switch k := p.peek(n); {
		case isDeclKeyword(k):
			return true
		case k == token.CLASS:
			return true
		case isModifierKeyword(k):
			n = p.afterModifierAt(n)
		case k == token.IDENT:
			t := p.peekTok(n)
			if t.Flags.Has(token.FlagEscaped) {
				return false
			}
			switch text := p.text(t); {
			case text == "actor" || text == "macro":
				// Contextual declaration keywords: a name follows.
				return p.peek(n+1) == token.IDENT
			case modifierWords[text]:
				n = p.afterModifierAt(n)
			default:
				return false
			}
		default:
			return false
		}
	}
}

// isDeclKeyword reports whether a reserved word opens a declaration.
func isDeclKeyword(k token.Kind) bool {
	switch k {
	case token.IMPORT, token.LET, token.VAR, token.TYPEALIAS, token.FUNC,
		token.ENUM, token.STRUCT, token.PROTOCOL, token.INIT, token.DEINIT,
		token.EXTENSION, token.SUBSCRIPT, token.OPERATOR, token.PRECEDENCEGROUP,
		token.ASSOCIATEDTYPE, token.CASE:
		return true
	}
	return false
}

// atDeclStart reports whether a declaration begins at the cursor. A
// `case` counts only inside an enum, where the enum's member parser
// asks for one directly, so it is excluded here: at statement level a
// `case` belongs to a switch.
func (p *parser) atDeclStart() bool {
	if p.at(token.CASE) {
		return false
	}
	return p.declStartAt(0)
}
