package parser

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// parseMarkup parses an element or a fragment; the scanner has already
// said where markup is (see scanner/markup.go), so the parser follows
// the tokens it made.
func (p *parser) parseMarkup() *ast.MarkupElement {
	p.depth++
	defer func() { p.depth-- }()
	lo := p.pos()
	el := &ast.MarkupElement{Open: p.expect(token.MARKUP_OPEN)}
	if p.tooDeep() {
		el.Span = p.span(lo)
		return el
	}
	if p.at(token.MARKUP_NAME) {
		el.Name = p.markupName()
	}
	for !p.at(token.MARKUP_END) && !p.at(token.MARKUP_SELF_CLOSE) && !p.at(token.EOF) {
		if a := p.parseMarkupAttr(); a != nil {
			if el.Name == nil {
				p.errAt(a, "a fragment takes no attributes")
			}
			el.Attrs = append(el.Attrs, a)
			continue
		}
		p.errHere("expected an attribute, '>' or '/>'")
		p.next()
	}
	if p.at(token.MARKUP_SELF_CLOSE) {
		if el.Name == nil {
			p.errHere("a fragment cannot close itself; write <></>")
		}
		el.SelfClosing = true
		el.Close = p.pos()
		p.next()
		el.Span = p.span(lo)
		return el
	}
	p.expect(token.MARKUP_END)

	for !p.at(token.MARKUP_CLOSE_OPEN) && !p.at(token.EOF) {
		switch {
		case p.at(token.MARKUP_TEXT):
			el.Children = append(el.Children, &ast.MarkupText{Span: ast.Span{Lo: p.pos(), Hi: p.tok().End}})
			p.next()
		case p.at(token.LBRACE):
			el.Children = append(el.Children, p.parseMarkupCode())
		case p.at(token.MARKUP_OPEN):
			el.Children = append(el.Children, p.parseMarkup())
		default:
			p.errHere("expected text, '{', a tag or a closing tag")
			p.next()
		}
	}
	p.expect(token.MARKUP_CLOSE_OPEN)
	if p.at(token.MARKUP_NAME) {
		el.CloseName = p.markupName()
	}
	switch {
	case el.Name == nil && el.CloseName != nil:
		p.errAt(el.CloseName, "a fragment closes with </>")
	case el.Name != nil && el.CloseName == nil:
		p.errHere("expected </" + el.Name.Text(p.f) + ">")
	case el.Name != nil && el.Name.Text(p.f) != el.CloseName.Text(p.f):
		p.errAt(el.CloseName, "expected </"+el.Name.Text(p.f)+"> to close <"+el.Name.Text(p.f)+">")
	}
	el.Close = p.expect(token.MARKUP_END)
	el.Span = p.span(lo)
	return el
}

func (p *parser) markupName() *ast.MarkupName {
	n := &ast.MarkupName{Span: ast.Span{Lo: p.pos(), Hi: p.tok().End}}
	p.next()
	return n
}

// parseMarkupAttr parses `name`, `name="text"`, `name={code}` or
// `{...attrs}`, or returns nil where none begins.
func (p *parser) parseMarkupAttr() ast.MarkupAttr {
	lo := p.pos()
	switch {
	case p.at(token.MARKUP_NAME):
		a := &ast.MarkupAttribute{Name: p.markupName()}
		if p.at(token.ASSIGN) {
			a.Assign = p.pos()
			p.next()
			switch {
			case p.at(token.MARKUP_STRING):
				a.Value = &ast.MarkupString{Span: ast.Span{Lo: p.pos(), Hi: p.tok().End}}
				p.next()
			case p.at(token.LBRACE):
				a.Value = p.parseMarkupCode()
			default:
				p.errHere("expected a quoted value or '{' after '='")
			}
		}
		a.Span = p.span(lo)
		return a
	case p.at(token.LBRACE) && p.peek(1) == token.OPER_PREFIX && p.text(p.peekTok(1)) == "...":
		s := &ast.MarkupSpread{Lbrace: p.pos()}
		p.next()
		p.next() // the `...`
		s.X = p.parseExpr(0)
		s.Rbrace = p.expect(token.RBRACE)
		s.Span = p.span(lo)
		return s
	}
	return nil
}

// parseMarkupCode parses `{…}`, its contents as a closure's.
func (p *parser) parseMarkupCode() *ast.MarkupCode {
	lo := p.pos()
	body := p.parseClosure()
	return &ast.MarkupCode{Span: p.span(lo), Body: body}
}
