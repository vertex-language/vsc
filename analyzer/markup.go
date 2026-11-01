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

// Markup in a .vsx file is checked by lowering each element, fragment,
// attribute, and child expression to typed calls against the built-in .vsx
// prelude (or an imported ui/component module).

// checkMarkup checks an element as the calls it is rewritten to.
func (c *checker) checkMarkup(e *ast.MarkupElement, expected types.Type, scope *Scope) types.Type {
	call := c.markupCalls[e]
	if call == nil {
		if c.modules["component"] == nil && c.lookupValue(scope, "component") == nil && scope.LookupType("component") == nil {
			c.errorf(e.Pos(), "markup needs ui/component: import it, or compile as a .vsx file")
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
		return mkCall(mkPath(at, "component", "Fragment"), at, mkArray(c.markupChildren(e.Children, scope), at))
	}
	name := e.Name.Text(c.file)
	if isComponentName(name) {
		return c.lowerComponent(e, name, scope)
	}
	return c.lowerIntrinsic(e, name, scope)
}

// isComponentName reports whether a tag names a component rather than an
// intrinsic HTML/SVG element: capitalized, or dotted (and not xml:tag).
func isComponentName(name string) bool {
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r) || strings.Contains(name, ".")
}

// lowerComponent lowers a component invocation (function component,
// Props-struct component, class component, or Context provider).
func (c *checker) lowerComponent(e *ast.MarkupElement, name string, scope *Scope) ast.Expr {
	at := span(e)
	fun := c.nameExpr(e.Name, name, e.TypeArgs)

	// <MyContext value={...}>...</MyContext> -> <MyContext.Provider value={...}>...</MyContext.Provider>
	if c.isContextValue(fun, scope) {
		s := span(e.Name)
		fun = &ast.MemberExpr{
			Span: s,
			X:    fun,
			Dot:  s.Hi,
			Name: &ast.Ident{Span: s, Synth: "Provider"},
		}
	}

	// Class component: `class Grid<T>: Component<GridProps<T>, EmptyState>`
	if cl, clType := c.lookupClassComponent(fun, scope); cl != nil {
		return c.lowerClassComponent(e, name, fun, cl, clType, scope)
	}

	params := c.componentParams(fun, scope)

	// Single Props-struct function component: `func DataCard<T>(_ props: CardProps<T>) -> Element`
	if propStruct, propType, paramLabel, ok := c.singlePropsStructParam(e, params); ok {
		return c.lowerPropsStructFunctionComponent(e, name, fun, propStruct, propType, paramLabel, scope)
	}

	var args []*ast.CallArg
	var keyExpr, refExpr ast.Expr
	spreadFields := map[string]ast.Expr{}
	misnamed := false

	for _, a := range e.Attrs {
		switch a := a.(type) {
		case *ast.MarkupSpread:
			st := c.checkExpr(a.X, nil, scope)
			if st != nil && !isInvalid(st) {
				if u, ok := st.Underlying().(*types.Struct); ok {
					for _, f := range u.Fields {
						if f != nil {
							s := span(a)
							spreadFields[f.Name] = &ast.MemberExpr{
								Span: s,
								X:    a.X,
								Dot:  s.Lo,
								Name: &ast.Ident{Span: s, Synth: f.Name},
							}
						}
					}
					continue
				}
			}
			c.errorf(a.Pos(), "spread on component '%s' requires a struct of properties", name)
			misnamed = true

		case *ast.MarkupAttribute:
			label := a.Name.Text(c.file)
			if strings.ContainsAny(label, ":-.") {
				c.errorf(a.Name.Pos(), "'%s' is not a parameter name; '%s' is a component", label, name)
				continue
			}
			if label == "key" && (len(params) == 0 || !params.has("key")) {
				keyExpr = c.attrValue(a, scope)
				continue
			}
			if label == "ref" && (len(params) == 0 || !params.has("ref")) {
				refExpr = c.attrValue(a, scope)
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
			x := c.componentAttrExpr(a, label, params, scope)
			if x == nil {
				continue
			}
			args = setOrAppendCallArg(args, &ast.CallArg{
				Span:  span(a),
				Label: &ast.Ident{Span: span(a.Name), Synth: label},
				X:     x,
			})
		}
	}

	// Fill in any parameters provided by `{...spread}` that were not overridden by explicit attributes.
	if len(spreadFields) > 0 {
		given := map[string]bool{}
		for _, a := range args {
			if a.Label != nil {
				given[a.Label.Synth] = true
			}
		}
		for _, l := range params.labels() {
			if !given[l] {
				if x, ok := spreadFields[l]; ok {
					s := span(x)
					args = append(args, &ast.CallArg{
						Span:  s,
						Label: &ast.Ident{Span: s, Synth: l},
						X:     x,
					})
					given[l] = true
				}
			}
		}
	}

	if kids := significant(c.file, e.Children); len(kids) > 0 {
		cat := ast.Span{Lo: kids[0].Pos(), Hi: kids[len(kids)-1].End()}
		label := &ast.Ident{Span: cat, Synth: "children"}
		if code, ok := kids[0].(*ast.MarkupCode); ok && len(kids) == 1 && code.Body.Sig != nil {
			args = setOrAppendCallArg(args, &ast.CallArg{Span: cat, Label: label, X: code.Body})
		} else {
			frag := mkCall(mkPath(cat, "component", "Fragment"), cat, mkArray(c.markupChildren(e.Children, scope), cat))
			switch params.childrenKind() {
			case childrenArray:
				args = setOrAppendCallArg(args, &ast.CallArg{Span: cat, Label: label, X: mkArray(c.markupChildren(e.Children, scope), cat)})
			case childrenNode:
				args = setOrAppendCallArg(args, &ast.CallArg{Span: cat, Label: label, X: frag})
			default:
				body := &ast.ClosureExpr{Span: cat, Lbrace: cat.Lo, Rbrace: cat.Hi,
					Stmts: []ast.Stmt{&ast.ExprStmt{Span: cat, X: frag}}}
				args = setOrAppendCallArg(args, &ast.CallArg{Span: cat, Label: label, X: body})
			}
		}
	}

	if misnamed {
		return nil
	}
	if len(params) == 1 {
		given := map[string]bool{}
		for _, a := range args {
			if a.Label != nil {
				given[a.Label.Synth] = true
			}
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
		args = orderCallArgs(args, params[0].Params)
	}

	call := mkCallArgs(fun, at, args)
	return c.wrapComponentNode(e, name, call, keyExpr, refExpr, scope)
}

func (c *checker) wrapComponentNode(e *ast.MarkupElement, name string, call, keyExpr, refExpr ast.Expr, scope *Scope) ast.Expr {
	at := span(e)
	once := &ast.ClosureExpr{Span: at, Lbrace: at.Lo, Rbrace: at.Hi, Stmts: []ast.Stmt{&ast.ExprStmt{Span: at, X: call}}}
	node := mkCall(mkPath(at, "component", "Component"), at, once)
	if keyExpr != nil {
		node = mkCall(mkPath(at, "component", "Key"), at, keyExpr, node)
	}
	if refExpr != nil {
		node = mkCall(mkPath(at, "component", "Ref"), at, refExpr, node)
	}
	if sheet := c.ownStyleSheet(scope); sheet != "" && c.namesImportedComponent(e.Name, name) {
		return mkCall(mkPath(at, "component", "Styled"), at, mkPath(at, sheet), node)
	}
	return node
}

func (c *checker) componentAttrExpr(a *ast.MarkupAttribute, label string, params componentParams, scope *Scope) ast.Expr {
	switch v := a.Value.(type) {
	case nil:
		return &ast.BasicLit{Span: span(a.Name), Kind: token.TRUE}
	case *ast.MarkupString:
		return mkString(v.Value(c.file), span(v))
	case *ast.MarkupElement:
		return v
	case *ast.MarkupCode:
		arity, known := params.fn(label)
		return c.codeArg(v, arity, known, scope)
	}
	return nil
}

// isContextValue reports whether fun resolves to a Context<T> instance,
// allowing `<TabsContext value={...}>` as shorthand for `<TabsContext.Provider value={...}>`.
func (c *checker) isContextValue(fun ast.Expr, scope *Scope) bool {
	if id, ok := fun.(*ast.IdentExpr); ok && id.Args == nil {
		if vs, ok := c.lookupValue(scope, id.Name.Synth).(*VarSymbol); ok && vs.Type() != nil {
			if gi, ok := vs.Type().(*types.GenericInstance); ok && gi.Base != nil {
				if cl, isCl := gi.Base.Underlying().(*types.Class); isCl && cl.Name == "Context" {
					return true
				}
				if st, isSt := gi.Base.Underlying().(*types.Struct); isSt && st.Name == "Context" {
					return true
				}
			}
		}
	}
	return false
}

// lookupClassComponent returns the Class and Type if fun names a class component.
func (c *checker) lookupClassComponent(fun ast.Expr, scope *Scope) (*types.Class, types.Type) {
	if id, ok := fun.(*ast.IdentExpr); ok {
		if tn := scope.LookupType(id.Name.Synth); tn != nil && tn.Type() != nil {
			if cl, ok := tn.Type().Underlying().(*types.Class); ok {
				return cl, tn.Type()
			}
		}
	}
	if m, ok := fun.(*ast.MemberExpr); ok {
		if base, ok := m.X.(*ast.IdentExpr); ok {
			if mod := c.modules[base.Name.Synth]; mod != nil && m.Name != nil {
				if tn := mod.LookupType(m.Name.Synth); tn != nil && tn.Type() != nil {
					if cl, ok := tn.Type().Underlying().(*types.Class); ok {
						return cl, tn.Type()
					}
				}
			}
		}
	}
	return nil, nil
}

// lowerClassComponent lowers `<Grid<bool> cells={...} cell={...} />` where Grid is a class component.
func (c *checker) lowerClassComponent(e *ast.MarkupElement, name string, fun ast.Expr, cl *types.Class, clType types.Type, scope *Scope) ast.Expr {
	at := span(e)
	// Find `props` field on cl or its superclass chain (`Component<P, S>`).
	var propsFieldType types.Type
	var subst map[*types.TypeParam]types.Type
	if e.TypeArgs != nil && len(cl.TypeParams) == len(e.TypeArgs.Args) {
		subst = map[*types.TypeParam]types.Type{}
		for i, tp := range cl.TypeParams {
			subst[tp] = c.resolveType(e.TypeArgs.Args[i], scope)
		}
	}
	for cur := clType; cur != nil; {
		if gi, ok := cur.(*types.GenericInstance); ok && gi.Base != nil {
			var params []*types.TypeParam
			switch b := gi.Base.Underlying().(type) {
			case *types.Class:
				params = b.TypeParams
			case *types.Struct:
				params = b.TypeParams
			}
			if len(params) == len(gi.Args) {
				if subst == nil {
					subst = map[*types.TypeParam]types.Type{}
				}
				for i, p := range params {
					t := gi.Args[i]
					if subst != nil {
						t = types.Substitute(t, subst)
					}
					subst[p] = t
				}
			}
		}
		u, ok := cur.Underlying().(*types.Class)
		if !ok {
			break
		}
		for _, f := range u.Fields {
			if f != nil && f.Name == "props" {
				propsFieldType = f.Type
				if o, isOpt := propsFieldType.(*types.Optional); isOpt {
					propsFieldType = o.Wrapped
				}
				if subst != nil && propsFieldType != nil {
					propsFieldType = types.Substitute(propsFieldType, subst)
				}
				break
			}
		}
		if propsFieldType != nil {
			break
		}
		cur = u.Superclass
	}

	if propsFieldType != nil {
		hasZeroArgInit := len(cl.Inits) == 0
		for _, sig := range cl.Inits {
			if sig != nil && sig.Minimum() == 0 {
				hasZeroArgInit = true
				break
			}
		}
		if st, ok := propsFieldType.Underlying().(*types.Struct); ok && hasZeroArgInit {
			propsExpr, keyExpr, refExpr, ok := c.buildPropsStructExpr(e, name, st, propsFieldType, scope)
			if !ok {
				return nil
			}
			instCall := mkCallArgs(fun, at, nil)
			cIdent := &ast.IdentExpr{Span: at, Name: &ast.Ident{Span: at, Synth: "__c"}}
			pIdent := &ast.IdentExpr{Span: at, Name: &ast.Ident{Span: at, Synth: "__p"}}
			renderCall := mkCall(&ast.MemberExpr{
				Span: at,
				X:    cIdent,
				Dot:  at.Lo,
				Name: &ast.Ident{Span: at, Synth: "render"},
			}, at)
			renderClosure := &ast.ClosureExpr{
				Span:   at,
				Lbrace: at.Lo,
				Stmts:  []ast.Stmt{&ast.ExprStmt{Span: at, X: renderCall}},
				Rbrace: at.Hi,
			}
			mountInner := mkCall(&ast.MemberExpr{
				Span: at,
				X:    cIdent,
				Dot:  at.Lo,
				Name: &ast.Ident{Span: at, Synth: "_mount"},
			}, at, pIdent, renderClosure)
			mountClosure := &ast.ClosureExpr{
				Span:   at,
				Lbrace: at.Lo,
				Sig: &ast.ClosureSig{
					Span: at,
					Params: &ast.ClosureParams{
						Span: at,
						Params: []*ast.ClosureParam{
							{Span: at, Name: &ast.Ident{Span: at, Synth: "__c"}},
							{Span: at, Name: &ast.Ident{Span: at, Synth: "__p"}},
						},
					},
					In: at.Lo,
				},
				Stmts:  []ast.Stmt{&ast.ExprStmt{Span: at, X: mountInner}},
				Rbrace: at.Hi,
			}
			mountCall := mkCall(mkPath(at, "component", "MountClass"), at, instCall, propsExpr, mountClosure)
			if refExpr != nil {
				mountCall = mkCall(mkPath(at, "component", "Ref"), at, refExpr, mountCall)
			}
			if keyExpr != nil {
				mountCall = mkCall(mkPath(at, "component", "Key"), at, keyExpr, mountCall)
			}
			return mountCall
		}
	}

	// Fallback for classes with explicit initializers: call init(...) then .render()
	var initSigs componentParams
	for _, sig := range cl.Inits {
		if sig != nil {
			initSigs = append(initSigs, sig)
		}
	}
	var args []*ast.CallArg
	var keyExpr, refExpr ast.Expr
	for _, a := range e.Attrs {
		if attr, ok := a.(*ast.MarkupAttribute); ok {
			label := attr.Name.Text(c.file)
			if label == "key" && !initSigs.has("key") {
				keyExpr = c.attrValue(attr, scope)
				continue
			}
			if label == "ref" && !initSigs.has("ref") {
				refExpr = c.attrValue(attr, scope)
				continue
			}
			if x := c.componentAttrExpr(attr, label, initSigs, scope); x != nil {
				args = setOrAppendCallArg(args, &ast.CallArg{
					Span:  span(attr),
					Label: &ast.Ident{Span: span(attr.Name), Synth: label},
					X:     x,
				})
			}
		}
	}
	if len(initSigs) == 1 {
		args = orderCallArgs(args, initSigs[0].Params)
	}
	instCall := mkCallArgs(fun, at, args)
	renderCall := mkCall(&ast.MemberExpr{
		Span: at,
		X:    instCall,
		Dot:  at.Lo,
		Name: &ast.Ident{Span: at, Synth: "render"},
	}, at)
	return c.wrapComponentNode(e, name, renderCall, keyExpr, refExpr, scope)
}

func (c *checker) singlePropsStructParam(e *ast.MarkupElement, params componentParams) (*types.Struct, types.Type, string, bool) {
	if len(params) != 1 || len(params[0].Params) != 1 {
		return nil, nil, "", false
	}
	p := params[0].Params[0]
	if p.Type == nil {
		return nil, nil, "", false
	}
	st, ok := p.Type.Underlying().(*types.Struct)
	if !ok {
		return nil, nil, "", false
	}
	label := p.Label
	if label == "" {
		label = p.Name
	}
	// If the function parameter is unlabeled (`_ props: CardProps`) or conforms to `Props`
	// and the tag didn't pass `props={...}` directly, build the Props struct from the tag attributes.
	if label != "_" && !types.ConformsToNamed(p.Type, "Props") {
		return nil, nil, "", false
	}
	for _, a := range e.Attrs {
		if attr, ok := a.(*ast.MarkupAttribute); ok && attr.Name.Text(c.file) == label {
			return nil, nil, "", false
		}
	}
	return st, p.Type, label, true
}

func (c *checker) lowerPropsStructFunctionComponent(e *ast.MarkupElement, name string, fun ast.Expr, st *types.Struct, propType types.Type, paramLabel string, scope *Scope) ast.Expr {
	at := span(e)
	propsExpr, keyExpr, refExpr, ok := c.buildPropsStructExpr(e, name, st, propType, scope)
	if !ok {
		return nil
	}
	arg := &ast.CallArg{Span: at, X: propsExpr}
	if paramLabel != "" && paramLabel != "_" {
		arg.Label = &ast.Ident{Span: at, Synth: paramLabel}
	}
	// If fun is generic and TypeArgs were placed on fun, infer or pass propsExpr.
	call := mkCallArgs(fun, at, []*ast.CallArg{arg})
	return c.wrapComponentNode(e, name, call, keyExpr, refExpr, scope)
}

func (c *checker) buildPropsStructExpr(e *ast.MarkupElement, compName string, st *types.Struct, propType types.Type, scope *Scope) (propsExpr, keyExpr, refExpr ast.Expr, ok bool) {
	at := span(e)
	// Fast path: `<Comp {...props} />` with a single spread and no overriding attributes or children.
	if len(e.Attrs) == 1 && len(significant(c.file, e.Children)) == 0 {
		if sp, isSpread := e.Attrs[0].(*ast.MarkupSpread); isSpread {
			return sp.X, nil, nil, true
		}
	}

	mw := st.Memberwise()
	var sigs componentParams
	if mw != nil {
		sigs = componentParams{mw}
	}

	var args []*ast.CallArg
	spreadFields := map[string]ast.Expr{}
	misnamed := false

	for _, a := range e.Attrs {
		switch a := a.(type) {
		case *ast.MarkupSpread:
			for _, f := range st.Fields {
				if f != nil {
					s := span(a)
					spreadFields[f.Name] = &ast.MemberExpr{
						Span: s,
						X:    a.X,
						Dot:  s.Lo,
						Name: &ast.Ident{Span: s, Synth: f.Name},
					}
				}
			}
		case *ast.MarkupAttribute:
			label := a.Name.Text(c.file)
			if label == "key" && !sigs.has("key") {
				keyExpr = c.attrValue(a, scope)
				continue
			}
			if label == "ref" && !sigs.has("ref") {
				refExpr = c.attrValue(a, scope)
				continue
			}
			if len(sigs) > 0 && !sigs.has(label) {
				msg := "'" + compName + "' has no property '" + label + "'"
				if s := closest(label, sigs.labels()); s != "" {
					msg += "; did you mean '" + s + "'?"
				}
				c.errorf(a.Name.Pos(), "%s", msg)
				misnamed = true
				continue
			}
			x := c.componentAttrExpr(a, label, sigs, scope)
			if x == nil {
				continue
			}
			args = setOrAppendCallArg(args, &ast.CallArg{
				Span:  span(a),
				Label: &ast.Ident{Span: span(a.Name), Synth: label},
				X:     x,
			})
		}
	}

	if len(spreadFields) > 0 {
		given := map[string]bool{}
		for _, a := range args {
			if a.Label != nil {
				given[a.Label.Synth] = true
			}
		}
		for _, f := range st.Fields {
			if f != nil && !given[f.Name] {
				if x, has := spreadFields[f.Name]; has {
					s := span(x)
					args = append(args, &ast.CallArg{
						Span:  s,
						Label: &ast.Ident{Span: s, Synth: f.Name},
						X:     x,
					})
					given[f.Name] = true
				}
			}
		}
	}

	if kids := significant(c.file, e.Children); len(kids) > 0 && sigs.has("children") {
		cat := ast.Span{Lo: kids[0].Pos(), Hi: kids[len(kids)-1].End()}
		label := &ast.Ident{Span: cat, Synth: "children"}
		frag := mkCall(mkPath(cat, "component", "Fragment"), cat, mkArray(c.markupChildren(e.Children, scope), cat))
		switch sigs.childrenKind() {
		case childrenArray:
			args = setOrAppendCallArg(args, &ast.CallArg{Span: cat, Label: label, X: mkArray(c.markupChildren(e.Children, scope), cat)})
		case childrenClosure:
			body := &ast.ClosureExpr{Span: cat, Lbrace: cat.Lo, Rbrace: cat.Hi,
				Stmts: []ast.Stmt{&ast.ExprStmt{Span: cat, X: frag}}}
			args = setOrAppendCallArg(args, &ast.CallArg{Span: cat, Label: label, X: body})
		default:
			args = setOrAppendCallArg(args, &ast.CallArg{Span: cat, Label: label, X: frag})
		}
	}

	if misnamed {
		return nil, nil, nil, false
	}

	if mw != nil {
		given := map[string]bool{}
		for _, a := range args {
			if a.Label != nil {
				given[a.Label.Synth] = true
			}
		}
		missing := false
		for _, p := range mw.Params {
			l := p.Label
			if l == "" {
				l = p.Name
			}
			if !p.HasDefault && !given[l] && l != "_" {
				if l == "children" {
					c.errorf(e.Pos(), "<%s> needs children", compName)
				} else {
					c.errorf(e.Pos(), "<%s> needs '%s'", compName, l)
				}
				missing = true
			}
		}
		if missing {
			return nil, nil, nil, false
		}
		args = orderCallArgs(args, mw.Params)
	}

	var typeCallee ast.Expr = &ast.IdentExpr{Span: at, Name: &ast.Ident{Span: at, Synth: st.Name}}
	if e.TypeArgs != nil && len(st.TypeParams) == len(e.TypeArgs.Args) {
		typeCallee = &ast.IdentExpr{Span: at, Name: &ast.Ident{Span: at, Synth: st.Name}, Args: e.TypeArgs}
	}
	return mkCallArgs(typeCallee, at, args), keyExpr, refExpr, true
}

func setOrAppendCallArg(args []*ast.CallArg, next *ast.CallArg) []*ast.CallArg {
	if next.Label != nil {
		for i, a := range args {
			if a.Label != nil && a.Label.Synth == next.Label.Synth {
				args[i] = next
				return args
			}
		}
	}
	return append(args, next)
}

func orderCallArgs(args []*ast.CallArg, params []*types.Param) []*ast.CallArg {
	byLabel := make(map[string]*ast.CallArg, len(args))
	var unlabelled []*ast.CallArg
	for _, a := range args {
		if a.Label != nil && a.Label.Synth != "" {
			byLabel[a.Label.Synth] = a
		} else {
			unlabelled = append(unlabelled, a)
		}
	}
	var ordered []*ast.CallArg
	used := map[string]bool{}
	for _, p := range params {
		l := p.Label
		if l == "" {
			l = p.Name
		}
		if a, ok := byLabel[l]; ok && !used[l] {
			ordered = append(ordered, a)
			used[l] = true
		}
	}
	for _, a := range args {
		if a.Label != nil && !used[a.Label.Synth] {
			ordered = append(ordered, a)
		}
	}
	return append(ordered, unlabelled...)
}

// namesImportedComponent reports whether a component's name is a module's:
// `kit.Button`, written through its package.
func (c *checker) namesImportedComponent(n *ast.MarkupName, name string) bool {
	dot := strings.IndexByte(name, '.')
	if dot < 0 {
		return false
	}
	return c.modules[name[:dot]] != nil
}

// nameExpr is a component's name as an expression: `Card`, `app.Window`, `List<string>`.
func (c *checker) nameExpr(n *ast.MarkupName, name string, typeArgs *ast.GenericArgs) ast.Expr {
	parts := strings.Split(name, ".")
	lo := n.Pos()
	var x ast.Expr
	for i, p := range parts {
		s := ast.Span{Lo: lo, Hi: lo + token.Pos(len(p))}
		var ga *ast.GenericArgs
		if i == len(parts)-1 {
			ga = typeArgs
		}
		if x == nil {
			x = &ast.IdentExpr{Span: s, Name: &ast.Ident{Span: s, Synth: p}, Args: ga}
		} else {
			x = &ast.MemberExpr{Span: ast.Span{Lo: n.Pos(), Hi: s.Hi}, X: x, Dot: lo - 1, Name: &ast.Ident{Span: s, Synth: p}, Args: ga}
		}
		lo = s.Hi + 1
	}
	return x
}

// componentParams are the parameters a component's function declares,
// where they can be known before its call is checked.
type componentParams []*types.Signature

type childrenParamKind int

const (
	childrenClosure childrenParamKind = iota
	childrenNode
	childrenArray
)

func (ps componentParams) childrenKind() childrenParamKind {
	for _, sig := range ps {
		for _, p := range sig.Params {
			l := p.Label
			if l == "" {
				l = p.Name
			}
			if l != "children" || p.Type == nil {
				continue
			}
			t := p.Type.Underlying()
			if o, ok := t.(*types.Optional); ok && o.Wrapped != nil {
				t = o.Wrapped.Underlying()
			}
			switch t.(type) {
			case *types.Signature:
				return childrenClosure
			case *types.Array:
				return childrenArray
			default:
				return childrenNode
			}
		}
	}
	return childrenClosure
}

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
	if m, ok := fun.(*ast.MemberExpr); ok {
		if base, ok := m.X.(*ast.IdentExpr); ok {
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
		for _, p := range sig.Params {
			if p.Label != "" || (p.Name != "" && !strings.HasPrefix(p.Name, "$")) {
				return componentParams{sig}
			}
		}
	}
	return nil
}

// lowerIntrinsic is an HTML/SVG element's component.Element call.
func (c *checker) lowerIntrinsic(e *ast.MarkupElement, tag string, scope *Scope) ast.Expr {
	at := span(e)
	if !isKnownIntrinsicElement(tag) {
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
	if sheet := c.ownStyleSheet(scope); sheet != "" {
		attrs = append(attrs, attr(e.Name, "Package", mkPath(span(e.Name), sheet)))
	}
	for _, a := range e.Attrs {
		switch a := a.(type) {
		case *ast.MarkupSpread:
			attrs = append(attrs, attr(a, "Spread", a.X))
		case *ast.MarkupAttribute:
			name := a.Name.Text(c.file)
			canon := canonicalHTMLAttribute(name)
			nameLit := mkString(canon, span(a.Name))
			switch {
			case name == "key":
				if v := c.attrValue(a, scope); v != nil {
					attrs = append(attrs, attr(a, "Key", v))
				}
				continue
			case name == "ref":
				if v := c.attrValue(a, scope); v != nil {
					attrs = append(attrs, attr(a, "Ref", v))
				}
			case strings.HasPrefix(name, "on") && len(name) > 2 && unicode.IsUpper(rune(name[2])):
				ev, ok := lookupHTMLEvent(name)
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
				case *ast.MarkupElement:
					attrs = append(attrs, attr(a, "Value", nameLit, v))
				case *ast.MarkupCode:
					if f, live := c.liveCode(v, scope); live {
						attrs = append(attrs, attr(a, "Live", nameLit, f))
					} else if f != nil {
						attrs = append(attrs, attr(a, "Value", nameLit, f))
					}
				}
			}
		}
	}
	return mkCall(mkPath(at, "component", "Element"), at,
		mkString(tag, span(e.Name)), mkArray(attrs, at), mkArray(c.markupChildren(e.Children, scope), at))
}

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

// attrValue is what `key=…` or `ref=…` is given.
func (c *checker) attrValue(a *ast.MarkupAttribute, scope *Scope) ast.Expr {
	switch v := a.Value.(type) {
	case nil:
		return &ast.BasicLit{Span: span(a.Name), Kind: token.TRUE}
	case *ast.MarkupString:
		return mkString(v.Value(c.file), span(v))
	case *ast.MarkupElement:
		return v
	case *ast.MarkupCode:
		return c.codeArg(v, -1, true, scope)
	}
	return nil
}

// normalizeStyleCall reorders arguments to `Style(...)` to match `struct Style`'s
// field declaration order so CSS properties can be written in any order.
func (c *checker) normalizeStyleCall(x ast.Expr, scope *Scope) ast.Expr {
	call, ok := x.(*ast.CallExpr)
	if !ok || call.Args == nil || len(call.Args.Args) <= 1 || scope == nil {
		return x
	}
	isStyle := false
	switch fn := call.Fun.(type) {
	case *ast.IdentExpr:
		isStyle = fn.Name.Text(c.file) == "Style"
	case *ast.MemberExpr:
		if base, ok := fn.X.(*ast.IdentExpr); ok && base.Name.Text(c.file) == "ui" && fn.Name != nil && fn.Name.Text(c.file) == "Style" {
			isStyle = true
		}
	}
	if !isStyle {
		return x
	}
	tn := scope.LookupType("Style")
	if tn == nil || tn.Type() == nil {
		return x
	}
	st, ok := tn.Type().Underlying().(*types.Struct)
	if !ok {
		return x
	}
	mw := st.Memberwise()
	if mw == nil {
		return x
	}
	for _, a := range call.Args.Args {
		if a.Label != nil && a.Label.Synth == "" {
			a.Label.Synth = a.Label.Text(c.file)
		}
	}
	call.Args.Args = orderCallArgs(call.Args.Args, mw.Params)
	return call
}

// codeArg is what `{…}` gives where a function of arity arguments, or
// (arity < 0) a value, is wanted.
func (c *checker) codeArg(code *ast.MarkupCode, arity int, known bool, scope *Scope) ast.Expr {
	body := code.Body
	single := singleExpr(body)
	if arity >= 0 {
		// Unwrap double-brace shorthand `{{ stmts }}` when a closure is wanted.
		if body.Sig == nil {
			if inner, ok := single.(*ast.ClosureExpr); ok {
				body = inner
				single = singleExpr(body)
			}
		}
		if body.Sig == nil && single != nil && isReference(single) && c.namesFunction(single, scope) {
			// If a 0-parameter function reference `onClick={reset}` is given where a
			// 1-parameter event handler `(Event) -> void` is expected, adapt it as `{ _ in reset() }`.
			if arity > 0 && c.isZeroArgFunction(single, scope) {
				at := span(single)
				ps := &ast.ClosureParams{Span: at}
				for i := 0; i < arity; i++ {
					ps.Params = append(ps.Params, &ast.ClosureParam{Span: at, Name: &ast.Ident{Span: at, Synth: "_"}})
				}
				call := mkCall(single, at)
				return &ast.ClosureExpr{
					Span:   at,
					Lbrace: at.Lo,
					Sig:    &ast.ClosureSig{Span: at, Params: ps, In: at.Lo},
					Stmts:  []ast.Stmt{&ast.ExprStmt{Span: at, X: call}},
					Rbrace: at.Hi,
				}
			}
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
		return c.normalizeStyleCall(c.rewriteLogicalAndMarkup(single), scope)
	}
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

// rewriteLogicalAndMarkup rewrites `{cond && <elem/>}` in markup expressions
// into `cond ? <elem/> : component.Fragment([])` so short-circuit conditional
// rendering works identically to TSX.
func (c *checker) rewriteLogicalAndMarkup(x ast.Expr) ast.Expr {
	seq, ok := x.(*ast.SequenceExpr)
	if !ok || c.pg == nil {
		return x
	}
	folded, err := FoldSequence(c.file, seq, c.pg)
	if err != nil || folded == nil {
		return x
	}
	if rew, changed := c.rewriteBinaryAnd(folded); changed {
		return rew
	}
	return x
}

func (c *checker) rewriteBinaryAnd(x ast.Expr) (ast.Expr, bool) {
	bin, ok := x.(*ast.BinaryExpr)
	if !ok || bin.Op == nil || bin.Op.Text(c.file) != "&&" {
		return x, false
	}
	if _, isMarkup := bin.Y.(*ast.MarkupElement); isMarkup {
		at := span(bin.Y)
		empty := mkCall(mkPath(at, "component", "Fragment"), at, mkArray(nil, at))
		return &ast.ConditionalExpr{
			Span:     bin.Span,
			Cond:     bin.X,
			Question: bin.Op.Pos(),
			Then:     bin.Y,
			Colon:    bin.Op.End(),
			Else:     empty,
		}, true
	}
	if rhs, changed := c.rewriteBinaryAnd(bin.Y); changed {
		at := span(rhs)
		empty := mkCall(mkPath(at, "component", "Fragment"), at, mkArray(nil, at))
		return &ast.ConditionalExpr{
			Span:     bin.Span,
			Cond:     bin.X,
			Question: bin.Op.Pos(),
			Then:     rhs,
			Colon:    bin.Op.End(),
			Else:     empty,
		}, true
	}
	return x, false
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
// children gives.
func (c *checker) liveCode(code *ast.MarkupCode, scope *Scope) (ast.Expr, bool) {
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
		rew := c.normalizeStyleCall(c.rewriteLogicalAndMarkup(single), scope)
		if rew != single {
			body.Stmts[0] = &ast.ExprStmt{Span: span(rew), X: rew}
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
	return mkCall(body, span(code)), false
}

// isLiteral reports whether x is a literal that reads nothing.
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

// markupChildren are an element's children as values.
func (c *checker) markupChildren(kids []ast.MarkupChild, scope *Scope) []ast.Expr {
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
			if f, live := c.liveCode(k, scope); live {
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

func singleExpr(b *ast.ClosureExpr) ast.Expr {
	if len(b.Stmts) != 1 {
		return nil
	}
	if s, ok := b.Stmts[0].(*ast.ExprStmt); ok {
		return s.X
	}
	return nil
}

func (c *checker) isZeroArgFunction(x ast.Expr, scope *Scope) bool {
	if scope == nil {
		return false
	}
	switch x := x.(type) {
	case *ast.IdentExpr:
		switch sym := c.lookupValue(scope, x.Name.Text(c.file)).(type) {
		case *FuncSymbol:
			for _, o := range sym.Overloads() {
				if sig := o.Signature(); sig != nil && len(sig.Params) > 0 {
					return false
				}
			}
			return len(sym.Overloads()) > 0
		case *VarSymbol:
			if sig, ok := sym.Type().Underlying().(*types.Signature); ok {
				return len(sig.Params) == 0
			}
		}
	case *ast.MemberExpr:
		base := c.checkExpr(x.X, nil, scope)
		if base == nil || isInvalid(base) || x.Name == nil {
			return false
		}
		name := x.Name.Text(c.file)
		if _, m := c.findMethod(base, name); m != nil && m.Sig != nil {
			return len(m.Sig.Params) == 0
		}
		if sig, ok := c.lookupMember(base, name).(*types.Signature); ok {
			return len(sig.Params) == 0
		}
	}
	return false
}

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
