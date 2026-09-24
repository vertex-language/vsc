package analyzer

import (
	"strconv"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/types"
)

// Methods and initializers named without being called (SE-0042, and
// Swift's initializer references) are the closures that call them:
//
//	Wrapper.init          { $r0 in Wrapper(value: $r0) }
//	String.init           { $r0 in String($r0) }
//	c.plus                { $r0 in c.plus($r0) }
//	Counter.plus          { $s in { $r0 in $s.plus($r0) } }
//
// The closure is checked where the reference is, against what the
// context wants, and lowered in its place.

// isReference reports whether e names an initializer or a method without
// calling it, and so may be one of these.
func (c *checker) isReference(e ast.Expr) bool {
	switch x := unparen(e).(type) {
	case *ast.MemberExpr:
		return x.Name != nil && !c.callees[x]
	case *ast.InitRefExpr:
		return !c.callees[x]
	}
	return false
}

// memberReference checks `x.m` or `T.init` as the closure it is, where it
// names a method or an initializer and is not called; ok is false where
// it is not one.
func (c *checker) memberReference(e *ast.MemberExpr, base types.Type, expected types.Type, scope *Scope) (types.Type, bool) {
	if e.Name == nil || c.callees[e] || base == nil || isInvalid(base) {
		return nil, false
	}
	name := e.Name.Text(c.file)
	want, _ := expected.(*types.Signature)
	if want == nil {
		if o, ok := expected.(*types.Optional); ok {
			want, _ = o.Wrapped.(*types.Signature)
		}
	}
	if meta, ok := base.(*types.Metatype); ok {
		inst := meta.Instance
		if name == "init" {
			return c.initReference(e, e.X, e.Lparen.IsValid(), e.Names, inst, want, scope)
		}
		// An instance method named on its type: a function of an
		// instance, answering the method bound to it.
		if sig := c.instanceMethodSig(inst, name); sig != nil {
			return c.unboundReference(e, inst, sig, scope)
		}
		return nil, false
	}
	sig := c.instanceMethodSig(base, name)
	if sig == nil {
		return nil, false
	}
	if want != nil && len(want.Params) != len(sig.Params) {
		return nil, false
	}
	cl := c.referenceClosure(e.X, name, sig.Params, labelsOf(sig.Params), e)
	t := c.checkExpr(cl, sig, scope)
	c.info.ImplicitSelf[e] = cl
	return t, true
}

// instanceMethodSig is the signature of the instance method of t named
// name -- one it declares, one its extensions do, or a built-in type's --
// or nil where there is none, or it is mutating or generic.
func (c *checker) instanceMethodSig(t types.Type, name string) *types.Signature {
	// A property of the name -- `first` beside `first(where:)` -- is what
	// the name means without a call.
	if _, isFunc := c.lookupMember(t, name).(*types.Signature); !isFunc {
		return nil
	}
	if c.hasProperty(t, name) {
		return nil
	}
	if _, m := c.findMethod(t, name); m != nil {
		if m.IsStatic || m.IsMutating || m.Sig == nil || len(m.Sig.TypeParams) > 0 {
			return nil
		}
		if sig, ok := c.lookupMember(t, name).(*types.Signature); ok {
			return sig
		}
		return m.Sig
	}
	if _, ms := c.builtinMethods(t, name); len(ms) == 1 {
		m := ms[0]
		if m.IsStatic || m.IsMutating || m.Sig == nil || len(m.Sig.TypeParams) > 0 {
			return nil
		}
		return m.Sig
	}
	return nil
}

// labelsOf is the argument labels of parameters, "" for none.
func labelsOf(params []*types.Param) []string {
	out := make([]string, len(params))
	for i, p := range params {
		if p.Label != "_" {
			out[i] = p.Label
		}
	}
	return out
}

// referenceClosure is `{ $r0, ... in recv.name(label0: $r0, ...) }`, or
// `{ ... in recv(...) }` for an empty name.
func (c *checker) referenceClosure(recv ast.Expr, name string, params []*types.Param, labels []string, at ast.Node) *ast.ClosureExpr {
	sp := ast.Span{Lo: at.Pos(), Hi: at.End()}
	var fun ast.Expr = recv
	if name != "" {
		fun = &ast.MemberExpr{Span: sp, X: recv, Dot: sp.Lo, Name: &ast.Ident{Span: sp, Synth: name}}
	}
	call := &ast.CallExpr{Span: sp, Fun: fun, Args: &ast.CallArgs{Span: sp}}
	cps := &ast.ClosureParams{Span: sp}
	for i := range params {
		pn := "$r" + strconv.Itoa(i)
		cps.Params = append(cps.Params, &ast.ClosureParam{Span: sp, Name: &ast.Ident{Span: sp, Synth: pn}})
		arg := &ast.CallArg{Span: sp, X: &ast.IdentExpr{Span: sp, Name: &ast.Ident{Span: sp, Synth: pn}}}
		if i < len(labels) && labels[i] != "" {
			arg.Label = &ast.Ident{Span: sp, Synth: labels[i]}
		}
		call.Args.Args = append(call.Args.Args, arg)
	}
	return &ast.ClosureExpr{Span: sp, Lbrace: sp.Lo, Rbrace: sp.Hi,
		Sig:   &ast.ClosureSig{Span: sp, Params: cps},
		Stmts: []ast.Stmt{&ast.ExprStmt{Span: sp, X: call}}}
}

// unboundReference is `T.m`: `{ $s in { ... in $s.m(...) } }`.
func (c *checker) unboundReference(e *ast.MemberExpr, inst types.Type, sig *types.Signature, scope *Scope) (types.Type, bool) {
	sp := ast.Span{Lo: e.Pos(), Hi: e.End()}
	self := &ast.IdentExpr{Span: sp, Name: &ast.Ident{Span: sp, Synth: "$s"}}
	inner := c.referenceClosure(self, e.Name.Text(c.file), sig.Params, labelsOf(sig.Params), e)
	outer := &ast.ClosureExpr{Span: sp, Lbrace: sp.Lo, Rbrace: sp.Hi,
		Sig: &ast.ClosureSig{Span: sp, Params: &ast.ClosureParams{Span: sp,
			Params: []*ast.ClosureParam{{Span: sp, Name: &ast.Ident{Span: sp, Synth: "$s"}}}}},
		Stmts: []ast.Stmt{&ast.ExprStmt{Span: sp, X: inner}}}
	want := &types.Signature{Params: []*types.Param{{Label: "_", Type: inst}}, Results: sig}
	t := c.checkExpr(outer, want, scope)
	c.info.ImplicitSelf[e] = outer
	return t, true
}

// initReference is `T.init` or `T.init(label:)`: the closure making a T
// of its arguments. With labels written, those are the initializer's;
// without, the context says how many arguments, and an initializer that
// takes them unlabelled is preferred, as Swift prefers it.
func (c *checker) initReference(e ast.Expr, x ast.Expr, named bool, names []*ast.ArgumentName, inst types.Type, want *types.Signature, scope *Scope) (types.Type, bool) {
	var labels []string
	switch {
	case named:
		for _, n := range names {
			l := ""
			if n != nil && n.Name != nil {
				l = n.Name.Text(c.file)
			}
			if l == "_" {
				l = ""
			}
			labels = append(labels, l)
		}
	case want != nil:
		labels = make([]string, len(want.Params))
		if !c.takesUnlabelled(inst, len(want.Params)) {
			// The one initializer of that many arguments, by its labels:
			// a struct's memberwise one, `Wrapper.init` for (value:).
			if sig := c.soleInitOfArity(inst, len(want.Params)); sig != nil {
				labels = labelsOf(sig.Params)
			}
		}
	default:
		if sig := c.soleInitOfArity(inst, -1); sig != nil {
			labels = labelsOf(sig.Params)
		} else {
			return nil, false
		}
	}
	params := make([]*types.Param, len(labels))
	for i := range params {
		params[i] = &types.Param{Label: "_"}
	}
	cl := c.referenceClosure(x, "", params, labels, e)
	var t types.Type
	if want != nil {
		t = c.checkExpr(cl, want, scope)
	} else if sig := c.soleInitOfArity(inst, len(labels)); sig != nil {
		out := *sig
		out.Results = inst
		t = c.checkExpr(cl, &out, scope)
	} else {
		return nil, false
	}
	c.info.ImplicitSelf[e] = cl
	return t, true
}

// initsOf is the initializers a type declares, its memberwise one
// included where it has one.
func (c *checker) initsOf(t types.Type) []*types.Signature {
	var out []*types.Signature
	switch u := t.Underlying().(type) {
	case *types.Struct:
		out = append(out, u.Inits...)
		if m := u.Memberwise(); m != nil {
			out = append(out, m)
		}
	case *types.Class:
		out = append(out, u.Inits...)
	case *types.Enum:
		out = append(out, u.Inits...)
	}
	if b := c.builtinOf(t); b != nil {
		out = append(out, b.Inits...)
	}
	return out
}

// takesUnlabelled reports whether t has an initializer of n unlabelled
// parameters -- or is a built-in type, whose conversions all are.
func (c *checker) takesUnlabelled(t types.Type, n int) bool {
	if _, basic := t.Underlying().(*types.Basic); basic {
		return true
	}
	for _, sig := range c.initsOf(t) {
		if sig == nil || len(sig.Params) != n {
			continue
		}
		all := true
		for _, p := range sig.Params {
			if p.Label != "" && p.Label != "_" {
				all = false
			}
		}
		if all {
			return true
		}
	}
	return false
}

// soleInitOfArity is t's one initializer of n parameters (any number,
// for n < 0), or nil where there is not exactly one.
func (c *checker) soleInitOfArity(t types.Type, n int) *types.Signature {
	var found *types.Signature
	for _, sig := range c.initsOf(t) {
		if sig == nil || (n >= 0 && len(sig.Params) != n) {
			continue
		}
		if found != nil {
			return nil
		}
		found = sig
	}
	return found
}

// initNamed reports whether e is `T.init` or `T.init(labels:)`.
func (c *checker) initNamed(e ast.Expr, scope *Scope) (types.Type, bool) {
	if _, ok := unparen(e).(*ast.InitRefExpr); ok {
		return nil, true
	}
	mem, ok := unparen(e).(*ast.MemberExpr)
	if !ok || mem.Name == nil || mem.Name.Text(c.file) != "init" {
		return nil, false
	}
	return nil, true
}

// collectionLiteralInit checks an array or dictionary literal written
// where a type of the program's own that is expressible by one is wanted
// as the call of its literal initializer: `let t: Tags = ["a", "b"]` is
// `Tags(arrayLiteral: "a", "b")`, and a dictionary literal's pairs are
// init(dictionaryLiteral:)'s tuples.
func (c *checker) collectionLiteralInit(e ast.Expr, expected types.Type, scope *Scope) (types.Type, bool) {
	target := expected
	if o, ok := target.(*types.Optional); ok {
		target = o.Wrapped
	}
	if target == nil {
		return nil, false
	}
	var name string
	switch u := target.Underlying().(type) {
	case *types.Struct:
		name = u.Name
	case *types.Class:
		name = u.Name
	case *types.Enum:
		name = u.Name
	default:
		return nil, false
	}
	sp := ast.Span{Lo: e.Pos(), Hi: e.End()}
	var label string
	var args []*ast.CallArg
	switch lit := e.(type) {
	case *ast.ArrayLit:
		if !types.ConformsToNamed(target, "ExpressibleByArrayLiteral") {
			return nil, false
		}
		label = "arrayLiteral"
		for _, el := range lit.Items {
			args = append(args, &ast.CallArg{Span: ast.Span{Lo: el.Pos(), Hi: el.End()}, X: el})
		}
	case *ast.DictLit:
		if !types.ConformsToNamed(target, "ExpressibleByDictionaryLiteral") {
			return nil, false
		}
		label = "dictionaryLiteral"
		for _, it := range lit.Items {
			isp := ast.Span{Lo: it.Pos(), Hi: it.End()}
			pair := &ast.TupleExpr{Span: isp, Lparen: isp.Lo, Rparen: isp.Hi,
				Elems: []*ast.TupleElem{{Span: isp, X: it.Key}, {Span: isp, X: it.Value}}}
			args = append(args, &ast.CallArg{Span: isp, X: pair})
		}
	default:
		return nil, false
	}
	if len(args) > 0 {
		args[0].Label = &ast.Ident{Span: sp, Synth: label}
	}
	if tn, ok := scope.Lookup(name).(*TypeNameSymbol); !ok || !types.Identical(tn.Type(), target) {
		return nil, false
	}
	call := &ast.CallExpr{Span: sp, Fun: &ast.IdentExpr{Span: sp, Name: &ast.Ident{Span: sp, Synth: name}},
		Args: &ast.CallArgs{Span: sp, Args: args}}
	t := c.checkExpr(call, target, scope)
	c.info.ImplicitSelf[e] = call
	if _, opt := expected.(*types.Optional); opt {
		return expected, true
	}
	return t, true
}

// hasProperty reports whether t has a stored or computed property of the
// name, whatever its type.
func (c *checker) hasProperty(t types.Type, name string) bool {
	if gi, ok := t.(*types.GenericInstance); ok {
		t = gi.Base
	}
	var lists [][]*types.Field
	switch u := t.Underlying().(type) {
	case *types.Struct:
		lists = append(lists, u.Fields, u.Computed)
	case *types.Class:
		for cl := u; cl != nil; {
			lists = append(lists, cl.Fields, cl.Computed)
			sup, ok := cl.Superclass.(*types.Class)
			if !ok || cl.Superclass == nil {
				if cl.Superclass != nil {
					sup, ok = cl.Superclass.Underlying().(*types.Class)
				}
				if !ok {
					break
				}
			}
			cl = sup
		}
	case *types.Enum:
		lists = append(lists, u.Computed)
	}
	if b := c.builtinOf(t); b != nil {
		lists = append(lists, b.Fields, b.Computed)
	}
	for _, l := range lists {
		for _, f := range l {
			if f != nil && f.Name == name {
				return true
			}
		}
	}
	return false
}
