package gen

import (
	"strings"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
)

// Subscripts a type declares are its getter and setter, as swiftc has
// them: the getter takes the indices and then self, and answers the
// element; the setter takes the new value, the indices, and self -- a
// value receiver by address, so that the write lands in the caller's
// storage. `s[i]` is a call of the getter, `s[i] = v` one of the setter,
// and a write through `s[i]` -- `m[1][0] = 7`, `m[1].append(8)` -- reads
// the element into a temporary, writes there, and sets it back once the
// statement is done, which is what swiftc's modify accessor amounts to.

// subscriptSymbol is the symbol of a subscript's getter or setter.
func (g *gen) subscriptSymbol(at ast.Node, recv types.Type, sub *types.Subscript, setter bool) string {
	d := mangle.Decl{
		Module:    g.memberModule(recv, sub),
		Context:   memberChain(recv),
		Extended:  extendedBuiltin(recv),
		Signature: &types.Signature{Params: sub.Params, Results: sub.Result},
		Static:    sub.IsStatic,
		ModuleOf:  g.moduleOfType,
	}
	var name string
	var err error
	if setter {
		name, err = mangle.SubscriptSetter(d)
	} else {
		name, err = mangle.SubscriptGetter(d)
	}
	if err != nil {
		g.errorAt(at, "cannot name the subscript: "+err.Error())
		return ""
	}
	return name
}

// emitSubscripts lowers the accessors of a type's subscripts.
func (g *gen) emitSubscripts(body *ast.MemberBlock, recv types.Type) {
	if body == nil || recv == nil {
		return
	}
	for _, mem := range body.Members {
		m, ok := mem.(*ast.SubscriptDecl)
		if !ok {
			continue
		}
		sub := g.info.SubscriptDecls[m]
		if sub == nil {
			continue
		}
		linkage := g.accessLinkage(m.Mods)
		getter := m.Body
		var setter *ast.Accessor
		if m.Accessors != nil {
			for _, a := range m.Accessors.Accessors {
				if a == nil || a.Keyword == nil || a.Body == nil {
					continue
				}
				switch g.text(a.Keyword) {
				case "get":
					getter = a.Body
				case "set":
					setter = a
				}
			}
		}
		if getter != nil {
			g.emitSubscriptAccessor(m, recv, sub, getter, nil, linkage)
		}
		if setter != nil {
			g.emitSubscriptAccessor(m, recv, sub, setter.Body, setter, linkage)
		}
	}
}

// emitSubscriptAccessor lowers one accessor of a subscript: the getter
// where set is nil, the setter otherwise.
func (g *gen) emitSubscriptAccessor(m *ast.SubscriptDecl, recv types.Type, sub *types.Subscript,
	body *ast.CodeBlock, set *ast.Accessor, linkage sil.Linkage) {
	symbol := g.subscriptSymbol(m, recv, sub, set != nil)
	if symbol == "" {
		return
	}
	g.emitSubscriptAccessorNamed(m, recv, sub, body, set, linkage, symbol)
}

// emitSubscriptAccessorNamed is emitSubscriptAccessor under a symbol the
// caller says: a generic type's subscript specialized for an instance.
func (g *gen) emitSubscriptAccessorNamed(m *ast.SubscriptDecl, recv types.Type, sub *types.Subscript,
	body *ast.CodeBlock, set *ast.Accessor, linkage sil.Linkage, symbol string) {
	f := g.m.Func(symbol).SetSourceName("subscript").SetLinkage(linkage).SetAttr("ossa")
	g.fn = f
	g.recv = recv
	g.entry = false
	f.Type().Params = nil
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes = nil
	g.loops, g.pending = nil, ""
	g.self = nil
	g.push()
	g.blk = f.Entry()

	argno := 1
	// The setter's new value comes first.
	if set != nil {
		vt := lowerType(sub.Result)
		value := f.Param(vt, paramConvention(&types.Param{Type: sub.Result}, vt))
		if sym := g.accessorValue(set); sym != nil {
			g.locals[sym] = &local{value: value, typ: vt}
			g.blk.DebugValue(value, "newValue", "let", "argno 1")
		}
		argno++
	}
	// Then the indices, bound to the names the declaration gives them.
	scope := g.info.Scopes[m]
	for i, p := range sub.Params {
		t := lowerType(p.BodyType())
		conv := paramConvention(p, t)
		var sym analyzer.Symbol
		if scope != nil && i < len(m.Params) {
			name := m.Params[i].Name
			if name == nil {
				name = m.Params[i].Label
			}
			if name != nil {
				sym = scope.Lookup(g.text(name))
			}
		}
		if byAddress(conv) {
			v := f.Param(t.Address(), conv)
			if sym != nil {
				_, isEx := existentialOf(p.Type)
				g.locals[sym] = &local{addr: v, typ: t, mem: isEx}
			}
			argno++
			continue
		}
		v := f.Param(t, conv)
		if sym != nil {
			g.locals[sym] = &local{value: v, typ: t}
			g.blk.DebugValue(v, p.Name, "let", "argno "+itoa(argno))
		}
		g.destroyLater(v)
		argno++
	}
	// Then self: a value receiver by address for the setter, which
	// writes through it, and borrowed otherwise.
	if !sub.IsStatic {
		st := lowerType(recv)
		if set != nil && !isClass(recv) {
			g.self = &local{addr: f.Param(st.Address(), sil.ParamInout), typ: st}
		} else {
			f.Param(st, selfConvention(st))
		}
		f.Type().Convention = sil.Method
	}
	if set == nil {
		f.SetResult(lowerType(sub.Result), resultConvention(lowerType(sub.Result)))
	}

	g.block(body)
	if g.blk != nil && g.blk.Term() == nil {
		g.unwind()
		if set == nil {
			g.missingReturn(body.Lbrace, body.Rbrace, "getter", sub.Result)
		} else {
			g.blk.Return(g.void())
		}
	}
	g.pop()
	g.fn, g.blk, g.recv, g.self = nil, nil, nil, nil
}

// subscriptCallee declares a subscript accessor's type where this module
// has not defined it: the indices, self, and the result for a getter;
// the value, the indices and self for a setter.
func (g *gen) subscriptCallee(at ast.Node, recv types.Type, sub *types.Subscript, setter bool) *sil.Func {
	symbol := g.subscriptSymbol(at, recv, sub, setter)
	if symbol == "" {
		return nil
	}
	return g.subscriptCalleeNamed(recv, sub, setter, symbol)
}

// subscriptCalleeNamed is subscriptCallee under a symbol the caller says.
func (g *gen) subscriptCalleeNamed(recv types.Type, sub *types.Subscript, setter bool, symbol string) *sil.Func {
	callee := g.m.Func(symbol).SetSourceName("subscript")
	if !g.needsType(callee) {
		return callee
	}
	callee.Type().Params = nil
	if setter {
		vt := lowerType(sub.Result)
		callee.Type().Params = append(callee.Type().Params,
			sil.Param{Type: vt, Convention: paramConvention(&types.Param{Type: sub.Result}, vt)})
	}
	for _, p := range sub.Params {
		t := lowerType(p.BodyType())
		conv := paramConvention(p, t)
		if byAddress(conv) {
			t = t.Address()
		}
		callee.Type().Params = append(callee.Type().Params, sil.Param{Type: t, Convention: conv})
	}
	if !sub.IsStatic {
		st := lowerType(recv)
		if setter && !isClass(recv) {
			callee.Type().Params = append(callee.Type().Params, sil.Param{Type: st.Address(), Convention: sil.ParamInout})
		} else {
			callee.Type().Params = append(callee.Type().Params, sil.Param{Type: st, Convention: selfConvention(st)})
		}
		callee.Type().Convention = sil.Method
	}
	if setter {
		callee.SetResult(sil.Object(types.Typ[types.Void]), sil.ResultUnowned)
	} else {
		callee.SetResult(lowerType(sub.Result), resultConvention(lowerType(sub.Result)))
	}
	return callee
}

// subscriptIndices lowers the arguments of a subscript use as the
// accessor's parameters take them.
func (g *gen) subscriptIndices(e *ast.SubscriptExpr, sub *types.Subscript) ([]*sil.Value, bool) {
	want := make([]types.Type, len(sub.Params))
	for i, p := range sub.Params {
		want[i] = p.Type
	}
	args := make([]*sil.Value, 0, len(e.Args))
	for i, a := range e.Args {
		v := g.expr(a.X)
		if v == nil {
			return nil, false
		}
		args = append(args, g.boxArg(a.X, v, want, i))
	}
	return args, true
}

// subscriptReceiver is the self a subscript accessor is given: nothing
// for a static one, a class instance, or a value -- borrowed for a
// getter, its storage for a setter.
func (g *gen) subscriptReceiver(e *ast.SubscriptExpr, ref *analyzer.SubscriptRef, setter bool) (*sil.Value, bool) {
	if ref.Subscript.IsStatic {
		return nil, true
	}
	if setter && !isClass(ref.Recv) {
		addr := g.lvalue(e.X)
		if addr == nil {
			g.refuse(e, "an assignment through a subscript of something that is not storage")
			return nil, false
		}
		return addr, true
	}
	self := g.expr(e.X)
	return self, self != nil
}

// declaredSubscriptRead lowers `s[i]` through the subscript's getter.
func (g *gen) declaredSubscriptRead(e *ast.SubscriptExpr, ref *analyzer.SubscriptRef) *sil.Value {
	ref, callee, handled := g.genericSubscript(e, ref, false)
	if !handled {
		callee = g.subscriptCallee(e, ref.Recv, ref.Subscript, false)
	}
	if callee == nil {
		return nil
	}
	args, ok := g.subscriptIndices(e, ref.Subscript)
	if !ok {
		return nil
	}
	self, ok := g.subscriptReceiver(e, ref, false)
	if !ok {
		return nil
	}
	if self != nil {
		args = append(args, self)
	}
	rt := lowerType(ref.Subscript.Result)
	v := g.blk.Apply(g.blk.FunctionRef(callee), rt, args...)
	g.destroyLater(v)
	return v
}

// declaredSubscriptWrite lowers `s[i] = v` through the subscript's
// setter, which borrows v: v is the caller's to end.
func (g *gen) declaredSubscriptWrite(e *ast.SubscriptExpr, ref *analyzer.SubscriptRef, v *sil.Value) bool {
	ref, callee, handled := g.genericSubscript(e, ref, true)
	if !handled {
		callee = g.subscriptCallee(e, ref.Recv, ref.Subscript, true)
	}
	if callee == nil {
		return false
	}
	args, ok := g.subscriptIndices(e, ref.Subscript)
	if !ok {
		return false
	}
	self, ok := g.subscriptReceiver(e, ref, true)
	if !ok {
		return false
	}
	all := append([]*sil.Value{v}, args...)
	if self != nil {
		all = append(all, self)
	}
	g.blk.Apply(g.blk.FunctionRef(callee), sil.Object(types.Typ[types.Void]), all...)
	return true
}

// declaredSubscriptAddr is `s[i]` as a destination written through --
// `m[1][0] = 7` -- where the subscript is one a type declares: the
// element read into a temporary that is written back through the setter
// once the statement ends.
func (g *gen) declaredSubscriptAddr(e *ast.SubscriptExpr, ref *analyzer.SubscriptRef) *sil.Value {
	if !ref.Subscript.Settable {
		g.refuse(e, "a write through a subscript that has no setter")
		return nil
	}
	cur := g.declaredSubscriptRead(e, ref)
	if cur == nil {
		return nil
	}
	t := lowerType(ref.Subscript.Result)
	slot := g.blk.AllocStack(t)
	g.blk.Store(g.consume(cur), slot, storeQualifier(t))
	g.writebacks = append(g.writebacks, func() {
		v := g.blk.Load(slot, loadQualifierTake(t))
		g.declaredSubscriptWrite(e, ref, v)
		if !t.Trivial() {
			g.blk.DestroyValue(v)
		}
		g.blk.DeallocStack(slot)
	})
	return slot
}

// flushWritebacks writes back what the statement wrote through
// subscripts' temporaries, in the order the temporaries were made.
func (g *gen) flushWritebacks() {
	pending := g.writebacks
	g.writebacks = nil
	for i := len(pending) - 1; i >= 0; i-- {
		if g.blk == nil || g.blk.Term() != nil {
			return
		}
		pending[i]()
	}
}

// genericSubscript is a subscript of a generic type, specialized for the
// instance it is used on, as a generic type's methods are: the reference
// with the subscript substituted, and its accessor, lowered once per
// instance under the method's symbol with Tv and the instance's
// arguments after it. It reports false for any other subscript.
func (g *gen) genericSubscript(e *ast.SubscriptExpr, ref *analyzer.SubscriptRef, setter bool) (*analyzer.SubscriptRef, *sil.Func, bool) {
	base := ref.Recv
	if gi, ok := base.(*types.GenericInstance); ok {
		base = gi.Base
	}
	params := nominalTypeParams(base)
	if len(params) == 0 && len(ref.Subst) == 0 {
		return ref, nil, false
	}
	inst, _ := g.typeOf(e.X).(*types.GenericInstance)
	if meta, ok := g.typeOf(e.X).(*types.Metatype); ok {
		inst, _ = meta.Instance.(*types.GenericInstance)
	}
	if inst == nil {
		inst, _ = g.recv.(*types.GenericInstance)
	}
	// A type with no parameters of its own is its own instance.
	if inst == nil && len(params) == 0 {
		inst = &types.GenericInstance{Base: base}
	}
	if inst == nil || len(inst.Args) != len(params) {
		g.refuse(e, "a subscript of a generic type on something whose type arguments are not known")
		return ref, nil, true
	}
	decl := g.subscriptDecl(ref.Subscript)
	if decl == nil {
		g.refuse(e, "a subscript of a generic type whose declaration this cannot find")
		return ref, nil, true
	}
	subst := make(map[*types.TypeParam]types.Type, len(g.subst)+len(params))
	for k, v := range g.subst {
		subst[k] = v
	}
	for i, p := range params {
		subst[p] = inst.Args[i]
	}
	// And the subscript's own, as this use infers them.
	var own []types.Type
	for _, p := range ref.Subscript.TypeParams {
		if a, ok := ref.Subst[p]; ok {
			a = types.Substitute(a, g.subst)
			subst[p] = a
			own = append(own, a)
		}
	}
	sub := *ref.Subscript
	sub.TypeParams = nil
	sub.Params = make([]*types.Param, len(ref.Subscript.Params))
	for i, p := range ref.Subscript.Params {
		q := *p
		q.Type = types.Substitute(p.Type, subst)
		sub.Params[i] = &q
	}
	sub.Result = types.Substitute(ref.Subscript.Result, subst)
	mangled := g.subscriptSymbol(e, base, &sub, setter)
	if mangled == "" {
		return ref, nil, true
	}
	var b strings.Builder
	b.WriteString(mangled)
	b.WriteString("Tv")
	for _, a := range inst.Args {
		b.WriteString(identifierSafe(a.String()))
	}
	for _, a := range own {
		b.WriteString("_")
		b.WriteString(identifierSafe(a.String()))
	}
	name := b.String()
	out := &analyzer.SubscriptRef{Recv: inst, Subscript: &sub}
	// The accessor's body, once, read in the file it was written in, with
	// the type's parameters the instance's.
	if !g.specialized[name] {
		if existing := g.m.Lookup(name); existing == nil || existing.IsDeclaration() {
			if g.specialized == nil {
				g.specialized = map[string]bool{}
			}
			g.specialized[name] = true
			if body, set := subscriptAccessor(g, decl, setter); body != nil {
				restore := g.apart()
				if f := g.fileOf(decl); f != nil {
					g.file = f
				}
				g.subst = subst
				done := g.asSpecialization()
				g.emitSubscriptAccessorNamed(decl, inst, &sub, body, set, sil.Private, name)
				done()
				restore()
			}
		}
	}
	return out, g.subscriptCalleeNamed(inst, &sub, setter, name), true
}

// subscriptDecl is the declaration a subscript was read from.
func (g *gen) subscriptDecl(sub *types.Subscript) *ast.SubscriptDecl {
	for d, s := range g.info.SubscriptDecls {
		if s == sub {
			return d
		}
	}
	return nil
}

// subscriptAccessor is a subscript declaration's getter body, or its
// setter's with the setter, where setter asks for that.
func subscriptAccessor(g *gen, m *ast.SubscriptDecl, setter bool) (*ast.CodeBlock, *ast.Accessor) {
	getter := m.Body
	var set *ast.Accessor
	if m.Accessors != nil {
		for _, a := range m.Accessors.Accessors {
			if a == nil || a.Keyword == nil || a.Body == nil {
				continue
			}
			switch g.text(a.Keyword) {
			case "get":
				getter = a.Body
			case "set":
				set = a
			}
		}
	}
	if setter {
		if set == nil {
			return nil, nil
		}
		return set.Body, set
	}
	return getter, nil
}
