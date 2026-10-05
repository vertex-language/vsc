package analyzer

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vss"
)

// Markup is checked in its check form (proposed_vsx.md §9.2, phase one):
// each element is rewritten as the ordinary calls it means, and those are
// checked and lowered in its place, so a misspelled prop is a call's
// error and a handler's type is a closure's. The calls are:
//
//	<Card title="T">…</Card>       Card(title: "T", children: { component.Fragment([…]) })
//	<p class="x" onClick={…}>…</p>  component.Element("p", [
//	                                    component.Attribute.Static("class", "x"),
//	                                    component.Attribute.On("click", dom.MouseEvent.self, { _ in … }),
//	                                ], […])
//	<>…</>                          component.Fragment([…])
//
// A child is anything component.Renderable: an element, a string, a
// number, an array or an optional of them. `component` and `dom` are the
// names ui/component and web/dom are imported as; a program without them
// may declare that API itself, as the tests in tests/vsx do.

// checkMarkup checks an element as the calls it is rewritten to.
func (c *checker) checkMarkup(e *ast.MarkupElement, expected types.Type, scope *Scope) types.Type {
	call := c.markupCalls[e]
	if call == nil {
		if c.modules["component"] == nil && c.lookupValue(scope, "component") == nil && scope.LookupType("component") == nil {
			c.errorf(e.Pos(), "markup needs ui/component: import it, or declare its check-form API (proposed_vsx.md §9.2)")
			return types.Typ[types.Invalid]
		}
		call = c.lowerMarkup(e, scope)
		if call == nil {
			return types.Typ[types.Invalid]
		}
		if c.markupCalls == nil {
			c.markupCalls = map[*ast.MarkupElement]ast.Expr{}
		}
		c.markupCalls[e] = call
	}
	t := c.checkExpr(call, expected, scope)
	c.info.ImplicitSelf[e] = call
	return t
}

// lowerMarkup is the call an element means, or nil where it has none.
func (c *checker) lowerMarkup(e *ast.MarkupElement, scope *Scope) ast.Expr {
	at := span(e)
	if e.Name == nil {
		return mkCall(mkPath(at, "component", "Fragment"), at, mkArray(c.markupChildren(e.Children), at))
	}
	name := e.Name.Text(c.file)
	if isComponentName(name) {
		return c.lowerComponent(e, name, scope)
	}
	return c.lowerIntrinsic(e, name, scope)
}

// isComponentName reports whether a tag names a component -- a function
// in scope -- rather than an HTML element: capitalized, or dotted.
func isComponentName(name string) bool {
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r) || strings.Contains(name, ".")
}

// ---- components ----

// lowerComponent is a component's call: its attributes are its labeled
// arguments, and its children its `children:` argument.
func (c *checker) lowerComponent(e *ast.MarkupElement, name string, scope *Scope) ast.Expr {
	at := span(e)
	fun := c.nameExpr(e.Name, name)
	params := c.componentParams(fun, scope)
	var args []*ast.CallArg
	misnamed := false
	for _, a := range e.Attrs {
		switch a := a.(type) {
		case *ast.MarkupSpread:
			c.errorf(a.Pos(), "a spread of attributes applies to an HTML element; pass a component its arguments by name")
		case *ast.MarkupAttribute:
			label := a.Name.Text(c.file)
			if strings.ContainsAny(label, ":-.") {
				c.errorf(a.Name.Pos(), "'%s' is not a parameter name; '%s' is a component", label, name)
				continue
			}
			if label == "key" && len(params) > 0 && !params.has("key") {
				c.errorf(a.Name.Pos(), "'key' is not an attribute; keys belong to <For>")
				continue
			}
			if len(params) > 0 && !params.has(label) {
				msg := "'" + name + "' has no parameter '" + label + "'"
				if s := closest(label, params.labels()); s != "" {
					msg += "; did you mean '" + s + "'?"
				}
				c.errorf(a.Name.Pos(), "%s", msg)
				misnamed = true
				continue
			}
			var x ast.Expr
			switch v := a.Value.(type) {
			case nil:
				x = &ast.BasicLit{Span: span(a.Name), Kind: token.TRUE}
			case *ast.MarkupString:
				x = mkString(v.Value(c.file), span(v))
			case *ast.MarkupCode:
				arity, known := params.fn(label)
				x = c.codeArg(v, arity, known, scope)
			}
			if x == nil {
				continue
			}
			args = append(args, &ast.CallArg{Span: span(a), Label: &ast.Ident{Span: span(a.Name), Synth: label}, X: x})
		}
	}
	if kids := significant(c.file, e.Children); len(kids) > 0 {
		cat := ast.Span{Lo: kids[0].Pos(), Hi: kids[len(kids)-1].End()}
		label := &ast.Ident{Span: cat, Synth: "children"}
		if code, ok := kids[0].(*ast.MarkupCode); ok && len(kids) == 1 && code.Body.Sig != nil {
			// A single `{t in …}` is the children: a function of what
			// the component hands each child, as <For> does.
			args = append(args, &ast.CallArg{Span: cat, Label: label, X: code.Body})
		} else {
			// Children is `() -> Node`: the children as a fragment, made
			// inside the component's scope when it asks.
			frag := mkCall(mkPath(cat, "component", "Fragment"), cat, mkArray(c.markupChildren(e.Children), cat))
			body := &ast.ClosureExpr{Span: cat, Lbrace: cat.Lo, Rbrace: cat.Hi,
				Stmts: []ast.Stmt{&ast.ExprStmt{Span: cat, X: frag}}}
			args = append(args, &ast.CallArg{Span: cat, Label: label, X: body})
		}
	}
	// Where the component is one function, what it needs and was not
	// given is said in markup's terms.
	if misnamed {
		return nil
	}
	if len(params) == 1 {
		given := map[string]bool{}
		for _, a := range args {
			given[a.Label.Synth] = true
		}
		missing := false
		for _, p := range params[0].Params {
			l := p.Label
			if l == "" {
				l = p.Name
			}
			if !p.HasDefault && !given[l] && l != "_" {
				if l == "children" {
					c.errorf(e.Pos(), "<%s> needs children", name)
				} else {
					c.errorf(e.Pos(), "<%s> needs '%s'", name, l)
				}
				missing = true
			}
		}
		if missing {
			return nil
		}
	}
	// Run once, untracked: a change to what its body read does not run
	// the component again; its own live parts follow what they read.
	call := mkCallArgs(fun, at, args)
	once := &ast.ClosureExpr{Span: at, Lbrace: at.Lo, Rbrace: at.Hi, Stmts: []ast.Stmt{&ast.ExprStmt{Span: at, X: call}}}
	return mkCall(mkPath(at, "component", "Component"), at, once)
}

// nameExpr is a component's name as an expression: `Card`, `app.Window`.
func (c *checker) nameExpr(n *ast.MarkupName, name string) ast.Expr {
	parts := strings.Split(name, ".")
	lo := n.Pos()
	var x ast.Expr
	for _, p := range parts {
		s := ast.Span{Lo: lo, Hi: lo + token.Pos(len(p))}
		if x == nil {
			x = &ast.IdentExpr{Span: s, Name: &ast.Ident{Span: s, Synth: p}}
		} else {
			x = &ast.MemberExpr{Span: ast.Span{Lo: n.Pos(), Hi: s.Hi}, X: x, Dot: lo - 1, Name: &ast.Ident{Span: s, Synth: p}}
		}
		lo = s.Hi + 1
	}
	return x
}

// componentParams are the parameters a component's function declares,
// where they can be known before its call is checked.
type componentParams []*types.Signature

// fn is, for the parameter labeled label, how many arguments it takes
// where it is a function in every overload that has it, and -1 where it
// is not a function; known is whether any overload has it.
func (ps componentParams) fn(label string) (arity int, known bool) {
	arity = -2
	for _, sig := range ps {
		for _, p := range sig.Params {
			if p.Label != label && !(p.Label == "" && p.Name == label) {
				continue
			}
			known = true
			t := p.Type
			if t != nil {
				t = t.Underlying()
			}
			if o, ok := t.(*types.Optional); ok && o.Wrapped != nil {
				t = o.Wrapped.Underlying()
			}
			n := -1
			if f, isFn := t.(*types.Signature); isFn {
				n = len(f.Params)
			}
			if arity == -2 || arity == n {
				arity = n
			} else {
				arity = -1
			}
		}
	}
	if arity == -2 {
		arity = -1
	}
	return arity, known
}

// has reports whether any overload has a parameter labeled label.
func (ps componentParams) has(label string) bool {
	for _, l := range ps.labels() {
		if l == label {
			return true
		}
	}
	return false
}

// labels are the parameter labels of every overload.
func (ps componentParams) labels() []string {
	var out []string
	for _, sig := range ps {
		for _, p := range sig.Params {
			l := p.Label
			if l == "" {
				l = p.Name
			}
			out = append(out, l)
		}
	}
	return out
}

func (c *checker) componentParams(fun ast.Expr, scope *Scope) componentParams {
	if id, ok := fun.(*ast.IdentExpr); ok {
		if fs, ok := c.lookupValue(scope, id.Name.Synth).(*FuncSymbol); ok {
			var out componentParams
			for _, o := range fs.Overloads() {
				if sig := o.Signature(); sig != nil {
					out = append(out, sig)
				}
			}
			return out
		}
		return nil
	}
	// A dotted name -- a module's component, a static function -- is
	// looked up as reading it would.
	if m, ok := fun.(*ast.MemberExpr); ok {
		if base, ok := m.X.(*ast.IdentExpr); ok {
			// `kit.Button`: the module's function.
			if mod := c.modules[base.Name.Synth]; mod != nil && m.Name != nil {
				if fs, ok := mod.Lookup(m.Name.Synth).(*FuncSymbol); ok {
					var out componentParams
					for _, o := range fs.Overloads() {
						if sig := o.Signature(); sig != nil {
							out = append(out, sig)
						}
					}
					return out
				}
			}
			if c.lookupValue(scope, base.Name.Synth) == nil && scope.LookupType(base.Name.Synth) == nil {
				return nil
			}
		}
	}
	if sig, ok := c.checkExpr(fun, nil, scope).(*types.Signature); ok {
		// A method reached through a value (`<Theme.Provider>`) has a
		// function type, without its labels: the lowered call checks them.
		for _, p := range sig.Params {
			if p.Label != "" || (p.Name != "" && !strings.HasPrefix(p.Name, "$")) {
				return componentParams{sig}
			}
		}
	}
	return nil
}

// ---- HTML elements ----

// lowerIntrinsic is an HTML element's component.Element call.
func (c *checker) lowerIntrinsic(e *ast.MarkupElement, tag string, scope *Scope) ast.Expr {
	at := span(e)
	if _, ok := htmlElements[tag]; !ok {
		msg := "unknown element <" + tag + ">"
		if s := closest(tag, htmlElementNames()); s != "" {
			msg += "; did you mean <" + s + ">?"
		} else {
			msg += "; a component's name starts with a capital letter"
		}
		c.errorf(e.Name.Pos(), "%s", msg)
	}
	attr := func(a ast.Node, fn string, args ...ast.Expr) ast.Expr {
		s := span(a)
		return mkCall(mkPath(s, "component", "Attribute", fn), s, args...)
	}
	var attrs []ast.Expr
	// An element of a package with styles (.vss) is stamped with the
	// package, which scopes them, and carries the package's sheet.
	if sheet := c.ownStyleSheet(scope); sheet != "" {
		attrs = append(attrs, attr(e.Name, "Package", mkPath(span(e.Name), sheet)))
	}
	for _, a := range e.Attrs {
		switch a := a.(type) {
		case *ast.MarkupSpread:
			attrs = append(attrs, attr(a, "Spread", a.X))
		case *ast.MarkupAttribute:
			name := a.Name.Text(c.file)
			nameLit := mkString(name, span(a.Name))
			switch {
			case name == "key":
				c.errorf(a.Name.Pos(), "'key' is not an attribute; keys belong to <For>")
				continue
			case strings.HasPrefix(name, "class:"), strings.HasPrefix(name, "style:"):
				kind, prop := "Class", strings.TrimPrefix(name, "class:")
				if strings.HasPrefix(name, "style:") {
					kind, prop = "Style", strings.TrimPrefix(name, "style:")
				}
				if prop == "" {
					c.errorf(a.Name.Pos(), "'%s' names no %s", name, strings.ToLower(kind))
					continue
				}
				if kind == "Style" && strings.HasPrefix(prop, "--") && !c.knownToken(prop, a.Name.Pos()+token.Pos(len("style:"))) {
					continue
				}
				// `class:on={on}` follows on: the braces are live.
				if code, ok := a.Value.(*ast.MarkupCode); ok {
					if f, live := c.liveCode(code); live {
						attrs = append(attrs, attr(a, "Live"+kind, mkString(prop, span(a.Name)), f))
						continue
					} else if f != nil {
						attrs = append(attrs, attr(a, kind, mkString(prop, span(a.Name)), f))
					}
					continue
				}
				v := c.attrValue(a)
				if v == nil {
					continue
				}
				attrs = append(attrs, attr(a, kind, mkString(prop, span(a.Name)), v))
			case name == "ref":
				if v := c.attrValue(a); v != nil {
					attrs = append(attrs, attr(a, "Ref", v))
				}
			case strings.HasPrefix(name, "on") && len(name) > 2 && unicode.IsUpper(rune(name[2])):
				ev, ok := htmlEvents[name]
				if !ok {
					msg := "<" + tag + "> has no event '" + name + "'"
					if s := closest(name, htmlAttributesOf(tag)); s != "" {
						msg += "; did you mean '" + s + "'?"
					}
					c.errorf(a.Name.Pos(), "%s", msg)
					continue
				}
				code, ok := a.Value.(*ast.MarkupCode)
				if !ok {
					c.errorf(a.Name.Pos(), "'%s' is a handler: write it in braces, %s={…}", name, name)
					continue
				}
				s := span(a.Name)
				typ := &ast.PostfixSelfExpr{Span: s, X: mkPath(s, "dom", ev.typ), Dot: s.Lo, Self: s.Lo}
				attrs = append(attrs, attr(a, "On", mkString(ev.event, s), typ, c.codeArg(code, 1, true, scope)))
			default:
				if !htmlAttributeKnown(tag, name) {
					msg := "<" + tag + "> has no attribute '" + name + "'"
					if s := closest(name, htmlAttributesOf(tag)); s != "" {
						msg += "; did you mean '" + s + "'?"
					}
					c.errorf(a.Name.Pos(), "%s", msg)
					continue
				}
				switch v := a.Value.(type) {
				case nil:
					attrs = append(attrs, attr(a, "Value", nameLit, &ast.BasicLit{Span: span(a.Name), Kind: token.TRUE}))
				case *ast.MarkupString:
					attrs = append(attrs, attr(a, "Static", nameLit, mkString(v.Value(c.file), span(v))))
				case *ast.MarkupCode:
					// `title={x}` follows x: the braces are live.
					if f, live := c.liveCode(v); live {
						attrs = append(attrs, attr(a, "Live", nameLit, f))
					} else if f != nil {
						attrs = append(attrs, attr(a, "Value", nameLit, f))
					}
				}
			}
		}
	}
	return mkCall(mkPath(at, "component", "Element"), at,
		mkString(tag, span(e.Name)), mkArray(attrs, at), mkArray(c.markupChildren(e.Children), at))
}

// knownToken reports whether a custom property named for a styled package
// -- `--kit-accent`, with `kit` imported and its .vss registering tokens --
// is one of that package's Tokens, and reports it where it is not.
// Anything else is no package's token and is not checked.
func (c *checker) knownToken(prop string, at token.Pos) bool {
	rest := strings.TrimPrefix(prop, "--")
	dash := strings.IndexByte(rest, '-')
	if dash <= 0 {
		return true
	}
	pkg := rest[:dash]
	mod := c.modules[pkg]
	if mod == nil {
		return true
	}
	tn := mod.LookupType("Tokens")
	if tn == nil || tn.Type() == nil {
		return true
	}
	en, ok := tn.Type().Underlying().(*types.Enum)
	if !ok {
		return true
	}
	member := vss.Token{Name: prop}.Member(pkg)
	var names []string
	for _, f := range en.Statics {
		if f.Name == member {
			return true
		}
		names = append(names, "--"+pkg+"-"+kebab(f.Name))
	}
	msg := "package " + pkg + " has no token " + prop
	if s := closest(prop, names); s != "" {
		msg += "; did you mean " + s + "?"
	}
	c.errorf(at, "%s", msg)
	return false
}

// kebab is a token's member name as its custom property writes it:
// TextColor is text-color.
func kebab(member string) string {
	var b strings.Builder
	for i, r := range member {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('-')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ownStyleSheet is the name of the stylesheet the package being checked
// carries -- vsc generates `__vssSheet` for a package with .vss files --
// or "" where it has none. Another package's, visible by import, is not
// this one's.
func (c *checker) ownStyleSheet(scope *Scope) string {
	sym := c.lookupValue(scope, vss.SheetName)
	if sym == nil {
		return ""
	}
	if _, imported := c.info.Imported[sym]; imported {
		return ""
	}
	return vss.SheetName
}

// attrValue is what `class:x=…` or `style:x=…` or `ref=…` is given.
func (c *checker) attrValue(a *ast.MarkupAttribute) ast.Expr {
	switch v := a.Value.(type) {
	case nil:
		return &ast.BasicLit{Span: span(a.Name), Kind: token.TRUE}
	case *ast.MarkupString:
		return mkString(v.Value(c.file), span(v))
	case *ast.MarkupCode:
		return c.codeArg(v, -1, true, nil)
	}
	return nil
}

// ---- code and children ----

// codeArg is what `{…}` gives where a function of arity arguments, or
// (arity < 0) a value, is wanted. For a function, the braces are the
// closure -- `{count += 1}` taking arguments it ignores -- unless they
// hold only a reference to one, `{store.Toggle}`. For a value, they hold
// an expression, or statements whose value is the closure's result:
// `{if a { <x/> } else { <y/> }}`.
func (c *checker) codeArg(code *ast.MarkupCode, arity int, known bool, scope *Scope) ast.Expr {
	body := code.Body
	single := singleExpr(body)
	if arity >= 0 {
		// `{store.Toggle}`: a function, given as it is. `{items}` where a
		// `() -> [T]` is wanted is a value, which the braces make live.
		if body.Sig == nil && single != nil && isReference(single) && c.namesFunction(single, scope) {
			return single
		}
		if body.Sig == nil && arity > 0 {
			at := ast.Span{Lo: body.Lbrace, Hi: body.Lbrace}
			ps := &ast.ClosureParams{Span: at}
			for i := 0; i < arity; i++ {
				ps.Params = append(ps.Params, &ast.ClosureParam{Span: at, Name: &ast.Ident{Span: at, Synth: "_"}})
			}
			body.Sig = &ast.ClosureSig{Span: at, Params: ps, In: at.Lo}
		}
		return body
	}
	if body.Sig != nil {
		if known {
			c.errorf(code.Pos(), "a closure is given where a value is wanted")
		}
		return body
	}
	if single != nil {
		return single
	}
	// `{if a { <x/> } else { <y/> }}` and `{switch s { … }}` are the
	// expressions those statements make. An `if` with no final `else`
	// gives an empty fragment where no branch is taken.
	if len(body.Stmts) == 1 {
		switch st := body.Stmts[0].(type) {
		case *ast.IfStmt:
			completeIf(st)
			return &ast.StmtExpr{Span: span(st), Stmt: st}
		case *ast.SwitchStmt:
			return &ast.StmtExpr{Span: span(st), Stmt: st}
		}
	}
	if len(body.Stmts) == 0 {
		c.errorf(code.Pos(), "empty braces give no value")
		return nil
	}
	return mkCall(body, span(code))
}

// completeIf gives an `if` chain that ends without an `else` one that
// makes an empty fragment, so the chain is an expression every way
// through it.
func completeIf(st *ast.IfStmt) {
	for {
		next, ok := st.Else.(*ast.IfStmt)
		if !ok {
			break
		}
		st = next
	}
	if st.Else != nil {
		return
	}
	at := ast.Span{Lo: st.End(), Hi: st.End()}
	empty := mkCall(mkPath(at, "component", "Fragment"), at, mkArray(nil, at))
	st.ElsePos = at.Lo
	st.Else = &ast.CodeBlock{Span: at, Lbrace: at.Lo, Rbrace: at.Hi,
		Stmts: []ast.Stmt{&ast.ExprStmt{Span: at, X: empty}}}
}

// liveCode is what `{…}` in an HTML element's attribute or among its
// children gives: a closure the runtime runs as a binding (live), or, for
// a literal, the value itself. The closure is the braces' own: `{count}`
// is `{ count }`, and `{if a { <x/> } else { <y/> }}` the if expression.
func (c *checker) liveCode(code *ast.MarkupCode) (ast.Expr, bool) {
	body := code.Body
	if body.Sig != nil {
		c.errorf(code.Pos(), "a closure is given where a value is wanted")
		return nil, false
	}
	if len(body.Stmts) == 0 {
		c.errorf(code.Pos(), "empty braces give no value")
		return nil, false
	}
	if single := singleExpr(body); single != nil {
		if isLiteral(single) {
			return single, false
		}
		return body, true
	}
	if len(body.Stmts) == 1 {
		switch st := body.Stmts[0].(type) {
		case *ast.IfStmt:
			completeIf(st)
			body.Stmts[0] = &ast.ExprStmt{Span: span(st), X: &ast.StmtExpr{Span: span(st), Stmt: st}}
			return body, true
		case *ast.SwitchStmt:
			body.Stmts[0] = &ast.ExprStmt{Span: span(st), X: &ast.StmtExpr{Span: span(st), Stmt: st}}
			return body, true
		}
	}
	// Statements whose value is the closure's result, run once.
	return mkCall(body, span(code)), false
}

// isLiteral reports whether x is a literal that reads nothing: a number,
// a Boolean, or a string with no interpolation.
func isLiteral(x ast.Expr) bool {
	switch x := x.(type) {
	case *ast.BasicLit:
		return true
	case *ast.StringLit:
		for _, seg := range x.Segments {
			if _, text := seg.(*ast.StringText); !text {
				return false
			}
		}
		return true
	}
	return false
}

// markupChildren are an element's children as values: text folded as JSX
// folds it, `{…}` as the value it holds, and elements as they are.
// Comments and empty braces are nothing.
func (c *checker) markupChildren(kids []ast.MarkupChild) []ast.Expr {
	var out []ast.Expr
	for _, k := range kids {
		switch k := k.(type) {
		case *ast.MarkupText:
			if s := k.Value(c.file); s != "" {
				out = append(out, mkString(s, span(k)))
			}
		case *ast.MarkupCode:
			if len(k.Body.Stmts) == 0 && k.Body.Sig == nil {
				continue
			}
			if k.Body.Sig != nil {
				c.errorf(k.Pos(), "a function as a child is passed to a component, and alone")
				continue
			}
			// `{count}` is a live region; `{"text"}` is text.
			if f, live := c.liveCode(k); live {
				out = append(out, mkCall(mkPath(span(k), "component", "Live"), span(k), f))
			} else if f != nil {
				out = append(out, f)
			}
		case *ast.MarkupElement:
			out = append(out, k)
		}
	}
	return out
}

// significant are the children that are something: not whitespace alone,
// not a comment.
func significant(f *token.File, kids []ast.MarkupChild) []ast.MarkupChild {
	var out []ast.MarkupChild
	for _, k := range kids {
		switch k := k.(type) {
		case *ast.MarkupText:
			if k.Value(f) == "" {
				continue
			}
		case *ast.MarkupCode:
			if len(k.Body.Stmts) == 0 && k.Body.Sig == nil {
				continue
			}
		}
		out = append(out, k)
	}
	return out
}

// singleExpr is the one expression a closure body holds, or nil.
func singleExpr(b *ast.ClosureExpr) ast.Expr {
	if len(b.Stmts) != 1 {
		return nil
	}
	if s, ok := b.Stmts[0].(*ast.ExprStmt); ok {
		return s.X
	}
	return nil
}

// namesFunction reports whether a name, or a member of one, is a function
// -- a method, a function, a closure held in a variable -- rather than a
// value of another type.
func (c *checker) namesFunction(x ast.Expr, scope *Scope) bool {
	if scope == nil {
		return true
	}
	switch x := x.(type) {
	case *ast.IdentExpr:
		switch sym := c.lookupValue(scope, x.Name.Text(c.file)).(type) {
		case *FuncSymbol:
			return true
		case *VarSymbol:
			_, fn := sym.Type().Underlying().(*types.Signature)
			return fn
		case nil:
			// self's member, named alone: a method where the type has one.
			if c.currType != nil {
				if _, m := c.findMethod(c.currType, x.Name.Text(c.file)); m != nil {
					return true
				}
			}
			return false
		}
		return false
	case *ast.MemberExpr:
		base := c.checkExpr(x.X, nil, scope)
		if base == nil || isInvalid(base) || x.Name == nil {
			return true
		}
		name := x.Name.Text(c.file)
		if _, m := c.findMethod(base, name); m != nil {
			return true
		}
		_, fn := c.lookupMember(base, name).(*types.Signature)
		return fn
	}
	return false
}

// isReference reports whether x names a function rather than calling or
// computing anything: `store.Toggle`, `onSave`.
func isReference(x ast.Expr) bool {
	switch x := x.(type) {
	case *ast.IdentExpr:
		return true
	case *ast.MemberExpr:
		return isReference(x.X) || isSelfExpr(x.X)
	}
	return false
}

func isSelfExpr(x ast.Expr) bool {
	_, ok := x.(*ast.SelfExpr)
	return ok
}

// closest is the candidate nearest name by edit distance, where it is
// near enough to be what was meant, or "".
func closest(name string, candidates []string) string {
	limit := min(max(len(name)/2, 1), 3)
	best, bestD := "", limit+1
	for _, c := range candidates {
		d := editDistance(strings.ToLower(name), strings.ToLower(c))
		if d < bestD || (d == bestD && c < best) {
			best, bestD = c, d
		}
	}
	return best
}

// editDistance counts the edits between a and b -- an insertion, a
// deletion, a change, or two neighbours swapped -- so that `dvi` is one
// edit from `div`.
func editDistance(a, b string) int {
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(a)][len(b)]
}

// ---- building ----

func span(n ast.Node) ast.Span { return ast.Span{Lo: n.Pos(), Hi: n.End()} }

// mkPath is `a.b.c` written at at.
func mkPath(at ast.Span, names ...string) ast.Expr {
	var x ast.Expr = &ast.IdentExpr{Span: at, Name: &ast.Ident{Span: at, Synth: names[0]}}
	for _, n := range names[1:] {
		x = &ast.MemberExpr{Span: at, X: x, Dot: at.Lo, Name: &ast.Ident{Span: at, Synth: n}}
	}
	return x
}

// mkCall is fun(xs…), each argument unlabeled.
func mkCall(fun ast.Expr, at ast.Span, xs ...ast.Expr) ast.Expr {
	var args []*ast.CallArg
	for _, x := range xs {
		args = append(args, &ast.CallArg{Span: span(x), X: x})
	}
	return mkCallArgs(fun, at, args)
}

// mkCallArgs is fun(args…).
func mkCallArgs(fun ast.Expr, at ast.Span, args []*ast.CallArg) ast.Expr {
	return &ast.CallExpr{Span: at, Fun: fun, Args: &ast.CallArgs{Span: at, Lparen: at.Lo, Args: args, Rparen: at.Hi}}
}

// mkArray is `[items…]`.
func mkArray(items []ast.Expr, at ast.Span) ast.Expr {
	return &ast.ArrayLit{Span: at, Lsquare: at.Lo, Items: items, Rsquare: at.Hi}
}

// mkString is a string literal of s, written at at.
func mkString(s string, at ast.Span) ast.Expr {
	return &ast.StringLit{Span: at, Open: at.Lo, Close: at.Hi,
		Segments: []ast.Node{&ast.StringText{Span: at, Synthesized: true, Synth: s}}}
}
