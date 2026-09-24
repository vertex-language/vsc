package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// computedProperty is the property a member expression names, where
// it names one, together with the type that declares it.
func (g *gen) computedProperty(e *ast.MemberExpr) (types.Type, *types.Field, bool) {
	if e == nil || e.Name == nil {
		return nil, nil, false
	}
	recv := g.typeOf(e.X)
	name := g.text(e.Name)
	for t := recv; t != nil; {
		for _, f := range g.computedOf(t) {
			if f != nil && f.Name == name {
				return t, f, true
			}
		}
		cl, ok := t.Underlying().(*types.Class)
		if !ok || cl.Superclass == nil {
			break
		}
		t = cl.Superclass
	}
	return nil, nil, false
}

// computedOf is a type's computed properties.
func (g *gen) computedOf(t types.Type) []*types.Field {
	if t == nil {
		return nil
	}
	if b := builtinOf(g.info, t); b != nil {
		return b.Computed
	}
	if inst, ok := t.(*types.GenericInstance); ok {
		t = inst.Base
	}
	switch n := t.Underlying().(type) {
	case *types.Struct:
		return n.Computed
	case *types.Class:
		return n.Computed
	case *types.Enum:
		return n.Computed
	}
	return nil
}

// getter lowers `v.magnitude` as a call to the property's getter.
func (g *gen) getter(e *ast.MemberExpr, recv types.Type, f *types.Field) *sil.Value {
	return g.getterOn(e, recv, f, e.X)
}

// getterCall lowers a getter call for a supplied receiver.
func (g *gen) getterCall(at ast.Node, recv types.Type, f *types.Field,
	receiver func() *sil.Value) *sil.Value {

	if cl, ok := receiverClass(recv); ok && g.poly[cl] && f.LazyStorage == "" {
		return g.dynamicGetter(cl, f, receiver)
	}
	resultType := f.Type
	name, spec, generic := g.genericAccessor(at, recv, f, false)
	if generic {
		if name == "" {
			return nil
		}
		resultType = spec
	} else {
		sig := &types.Signature{Results: f.Type}
		d := mangle.Decl{
			Module:    g.memberModule(recv, f),
			Context:   memberChain(recv),
			Extended:  extendedBuiltin(recv),
			Name:      f.Name,
			Signature: sig,
			ModuleOf:  g.moduleOfType,
		}
		var err error
		name, err = mangle.Getter(d)
		if err != nil {
			g.errorAt(at, "cannot name the getter of '"+f.Name+"': "+err.Error())
			return nil
		}
	}
	if !generic {
		g.emitCoreGetter(recv, f, name, false)
	}
	// The receiver is passed @guaranteed.
	self := receiver()
	if self == nil {
		return nil
	}
	callee := g.m.Func(name).SetSourceName(f.Name)
	if g.needsType(callee) {
		st := lowerType(recv)
		callee.Type().Params = append(callee.Type().Params,
			sil.Param{Type: st, Convention: selfConvention(st)})
		callee.Type().Convention = sil.Method
		callee.SetResult(lowerType(resultType), resultConvention(lowerType(resultType)))
	}
	ref := g.blk.FunctionRef(callee)
	v := g.blk.Apply(ref, lowerType(resultType), self)
	g.destroyLater(v)
	return v
}

// emitCoreGetter emits, under symbol, the getter of a computed property the
// core's source gives a built-in type -- Double.pi, Double.isNaN -- the
// first time a module reads it. It is lowered privately where it is used,
// as the core's functions are; see emitCoreInit.
func (g *gen) emitCoreGetter(recv types.Type, f *types.Field, symbol string, static bool) {
	b := builtinOf(g.info, recv)
	core := g.info.CoreAlgorithms
	if b == nil || core == nil || b.Modules[f] != "Swift" || len(b.Params) > 0 {
		return
	}
	if fn := g.m.Lookup(symbol); fn != nil && !fn.IsDeclaration() {
		return
	}
	restore := g.apart()
	defer restore()
	g.file, g.specializing = core.Unit, true
	for _, st := range core.Stmts {
		decl, ok := st.(*ast.DeclStmt)
		if !ok {
			continue
		}
		ext, ok := decl.D.(*ast.ExtensionDecl)
		if !ok || ext.Body == nil || !types.Identical(g.info.Extensions[ext], recv) {
			continue
		}
		for _, mem := range ext.Body.Members {
			v, ok := mem.(*ast.VarDecl)
			if !ok || isStaticDecl(v.Mods) != static {
				continue
			}
			for _, bnd := range v.Bindings {
				if g.computedName(bnd) != f.Name {
					continue
				}
				if body := g.getterBody(bnd); body != nil {
					g.emitGetterNamed(symbol, recv, f.Name, f.Type, body, static, sil.Private)
				}
				return
			}
		}
	}
}

// emitGetters lowers the getter functions for a type's computed properties.
func (g *gen) emitGetters(body *ast.MemberBlock, recv types.Type) {
	if body == nil || recv == nil {
		return
	}
	for _, mem := range body.Members {
		m, ok := mem.(*ast.VarDecl)
		if !ok {
			continue
		}
		static := isStaticDecl(m.Mods)
		linkage := g.accessLinkage(m.Mods)
		for _, b := range m.Bindings {
			name, ftype := g.computedName(b), g.bindingType(b, recv)
			if name == "" || ftype == nil {
				continue
			}
			// A property with willSet or didSet is stored and has no
			// getter, so it needs the other half only: the store with
			// its observers around it.
			if !static {
				if f, observed := observedField(recv, name); observed && f != nil {
					g.emitObservedSetter(recv, name, ftype, b, linkage)
					continue
				}
			}
			if f, ok := g.computedField(recv, name); ok && f.LazyStorage != "" && !static && b.Value != nil {
				g.emitLazyAccessors(recv, f, b.Value, linkage)
				continue
			}
			// A wrapped property, and its projection where it has one.
			if f, ok := g.computedField(recv, name); ok && f.Wrapper != "" && !static {
				g.emitWrapperAccessors(recv, f, linkage)
				if p, ok := g.computedField(recv, "$"+name); ok && p.Projected {
					g.emitWrapperAccessors(recv, p, linkage)
				}
				continue
			}
			block := g.getterBody(b)
			if block == nil {
				continue
			}
			g.emitGetter(recv, name, ftype, block, static, linkage)
			// A setter is emitted beside it where the property
			// declares one. Static setters wait on static storage.
			if !static {
				if set := g.setterAccessor(b); set != nil {
					g.emitSetter(recv, name, ftype, set, linkage)
				}
			}
		}
	}
}

// getterBody returns the block for a property's getter, or nil if none.
func (g *gen) getterBody(b *ast.PatternBinding) *ast.CodeBlock {
	return g.accessorBody(b, "get")
}

// setterBody is the block a computed property's setter runs, or nil
// where it has none.
func (g *gen) setterBody(b *ast.PatternBinding) *ast.CodeBlock {
	return g.accessorBody(b, "set")
}

// accessorBody returns the code block for the requested accessor (get/set).
func (g *gen) accessorBody(b *ast.PatternBinding, want string) *ast.CodeBlock {
	if b == nil {
		return nil
	}
	if b.Body != nil {
		if want != "get" {
			return nil
		}
		return b.Body
	}
	if b.Accessors == nil {
		return nil
	}
	for _, a := range b.Accessors.Accessors {
		if a == nil || a.Keyword == nil || a.Body == nil {
			continue
		}
		if g.text(a.Keyword) == want {
			return a.Body
		}
	}
	return nil
}

// bindingType is the type a computed property was declared with.
func (g *gen) bindingType(b *ast.PatternBinding, recv types.Type) types.Type {
	name := g.computedName(b)
	for _, f := range g.computedOf(recv) {
		if f != nil && f.Name == name {
			return f.Type
		}
	}
	for _, f := range g.staticsOf(recv) {
		if f != nil && f.Name == name && f.IsComputed {
			return f.Type
		}
	}
	if f, ok := observedField(recv, name); ok {
		return f.Type
	}
	return nil
}

// computedName is the name a binding's pattern binds.
func (g *gen) computedName(b *ast.PatternBinding) string {
	if b == nil {
		return ""
	}
	pat := b.Pat
	if tp, ok := pat.(*ast.TypedPattern); ok {
		pat = tp.Pat
	}
	id, ok := pat.(*ast.IdentPattern)
	if !ok || id.Name == nil {
		return ""
	}
	return g.text(id.Name)
}

// isStaticDecl reports whether a member belongs to the type rather
// than to an instance.
// accessLinkage is the linkage a property's accessors get, which is the
// one its declaration asks for. A public property's getter is called from
// another module, so it has to be a symbol that module can reach.
func (g *gen) accessLinkage(mods []*ast.Modifier) sil.Linkage {
	if g.specializing {
		return sil.Private
	}
	for _, m := range mods {
		if m == nil || m.Name == nil {
			continue
		}
		switch m.Name.Text(g.file) {
		case "public", "open":
			return sil.Public
		case "package":
			return sil.PackageLinkage
		case "private", "fileprivate":
			return sil.Private
		}
	}
	return sil.Hidden
}

func isStaticDecl(mods []*ast.Modifier) bool {
	for _, m := range mods {
		if m != nil && (m.Kind == token.STATIC || m.Kind == token.CLASS) {
			return true
		}
	}
	return false
}

// emitGetter lowers one getter: a method taking the receiver and
// returning the property's value.
func (g *gen) emitGetter(recv types.Type, name string, t types.Type,
	body *ast.CodeBlock, static bool, linkage sil.Linkage) {

	sig := &types.Signature{Results: t}
	d := mangle.Decl{
		Module:    g.memberModule(recv, nil),
		Context:   memberChain(recv),
		Extended:  extendedBuiltin(recv),
		Name:      name,
		Signature: sig,
		ModuleOf:  g.moduleOfType,
	}
	symbol, err := mangle.Getter(d)
	if static {
		// A property of the type rather than of an instance: the same
		// name with Z after it, which is what swiftc writes.
		symbol, err = mangle.StaticGetter(d)
	}
	if err != nil {
		g.errorAt(body, "cannot name the getter of '"+name+"': "+err.Error())
		return
	}
	g.emitGetterNamed(symbol, recv, name, t, body, static, linkage)
}

// emitGetterNamed is emitGetter under a symbol already chosen.
func (g *gen) emitGetterNamed(symbol string, recv types.Type, name string, t types.Type,
	body *ast.CodeBlock, static bool, linkage sil.Linkage) {

	f := g.m.Func(symbol).SetSourceName(name).
		SetLinkage(linkage).SetAttr("ossa")
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

	if !static {
		st := lowerType(recv)
		f.Param(st, selfConvention(st))
		f.Type().Convention = sil.Method
	}
	f.SetResult(lowerType(t), resultConvention(lowerType(t)))

	g.block(body)
	if g.blk != nil && g.blk.Term() == nil {
		g.unwind()
		g.missingReturn(body.Lbrace, body.Rbrace, "getter", t)
	}
	g.pop()
	g.recv = nil
}

// staticProperty is the stored property of a type a member
// expression names, where it names one.
func (g *gen) staticProperty(e *ast.MemberExpr) (types.Type, *types.Field, bool) {
	if e == nil || e.Name == nil {
		return nil, nil, false
	}
	t := g.typeOf(e.X)
	meta, ok := t.(*types.Metatype)
	if !ok {
		return nil, nil, false
	}
	name := g.text(e.Name)
	for _, f := range g.staticsOf(meta.Instance) {
		if f != nil && f.Name == name {
			return meta.Instance, f, true
		}
	}
	return nil, nil, false
}

// staticsOf is a type's stored static properties.
func (g *gen) staticsOf(t types.Type) []*types.Field {
	if t == nil {
		return nil
	}
	if b := builtinOf(g.info, t); b != nil {
		return b.Statics
	}
	if inst, ok := t.(*types.GenericInstance); ok {
		t = inst.Base
	}
	switch n := t.Underlying().(type) {
	case *types.Struct:
		return n.Statics
	case *types.Class:
		return n.Statics
	case *types.Enum:
		return n.Statics
	}
	return nil
}

// staticRead lowers a static stored property access via its addressor function.
func (g *gen) staticRead(e *ast.MemberExpr, recv types.Type, f *types.Field) *sil.Value {
	if f.IsComputed {
		return g.staticGetter(e, recv, f)
	}
	addr := g.staticAddr(e, recv, f)
	if addr == nil {
		return nil
	}
	t := lowerType(f.Type)
	v := g.blk.Load(addr, loadQualifier(t))
	return g.loaded(v, t)
}

// staticAddr is where a static stored property lives, through its
// addressor: the storage a read loads from and a write stores to.
func (g *gen) staticAddr(e ast.Node, recv types.Type, f *types.Field) *sil.Value {
	d := mangle.Decl{
		Module:    g.memberModule(recv, f),
		Context:   memberChain(recv),
		Extended:  extendedBuiltin(recv),
		Name:      f.Name,
		Signature: &types.Signature{Results: f.Type},
		ModuleOf:  g.moduleOfType,
	}
	name, err := mangle.Addressor(d)
	if err != nil {
		g.errorAt(e, "cannot name the storage of '"+f.Name+"': "+err.Error())
		return nil
	}
	t := lowerType(f.Type)
	callee := g.m.Func(name).SetSourceName(f.Name)
	if g.needsType(callee) {
		callee.SetResult(rawPointerType(), sil.ResultUnowned)
	}
	// The addressor hands back a raw pointer to the storage, which is
	// then an address: the shape Swift's addressors have.
	p := g.blk.Apply(g.blk.FunctionRef(callee), rawPointerType())
	return g.blk.PointerToAddress(p, t.Address())
}

// implicitStaticAddr is the storage of a static stored property named
// alone inside a member of its type: `made += 1` in a static method.
func (g *gen) implicitStaticAddr(e *ast.IdentExpr) *sil.Value {
	recv := g.recv
	if recv == nil {
		recv = g.staticRecv
	}
	if recv == nil || e.Name == nil {
		return nil
	}
	name := g.text(e.Name)
	for _, f := range g.staticsOf(recv) {
		if f != nil && f.Name == name && !f.IsComputed {
			return g.staticAddr(e, recv, f)
		}
	}
	return nil
}

// staticGetter lowers a call to a static computed property's getter.
func (g *gen) staticGetter(e *ast.MemberExpr, recv types.Type, f *types.Field) *sil.Value {
	sig := &types.Signature{Results: f.Type}
	d := mangle.Decl{
		Module:    g.memberModule(recv, f),
		Context:   memberChain(recv),
		Extended:  extendedBuiltin(recv),
		Name:      f.Name,
		Signature: sig,
		ModuleOf:  g.moduleOfType,
	}
	name, err := mangle.StaticGetter(d)
	if err != nil {
		g.errorAt(e, "cannot name the getter of '"+f.Name+"': "+err.Error())
		return nil
	}
	g.emitCoreGetter(recv, f, name, true)
	callee := g.m.Func(name).SetSourceName(f.Name)
	if g.needsType(callee) {
		callee.Type().Params = nil
		callee.SetResult(lowerType(f.Type), resultConvention(lowerType(f.Type)))
	}
	ref := g.blk.FunctionRef(callee)
	v := g.blk.Apply(ref, lowerType(f.Type))
	g.destroyLater(v)
	return v
}

// implicitStatic lowers an unqualified reference to a static property within a member.
func (g *gen) implicitStatic(e *ast.IdentExpr) (*sil.Value, bool) {
	recv := g.recv
	if recv == nil {
		recv = g.staticRecv
	}
	if recv == nil || e.Name == nil {
		return nil, false
	}
	name := g.text(e.Name)
	for _, f := range g.staticsOf(recv) {
		if f == nil || f.Name != name {
			continue
		}
		member := &ast.MemberExpr{Name: e.Name}
		if !f.IsComputed {
			return g.staticRead(member, recv, f), true
		}
		return g.staticGetter(member, recv, f), true
	}
	return nil, false
}

// implicitComputed lowers an unqualified reference to an instance computed property as self.property.
func (g *gen) implicitComputed(e *ast.IdentExpr) (*sil.Value, bool) {
	if g.recv == nil || e.Name == nil {
		return nil, false
	}
	name := g.text(e.Name)
	for _, f := range g.computedOf(g.recv) {
		if f == nil || f.Name != name {
			continue
		}
		if lazyInPlace(g.recv, f) {
			if g.self != nil && g.self.addr != nil {
				return g.lazyGetterCall(e, g.recv, f, g.self.addr), true
			}
			return g.getterOn(e, g.recv, f, &ast.SelfExpr{Span: ast.Span{Lo: e.Pos(), Hi: e.Pos()}}), true
		}
		return g.getterCall(e, g.recv, f, func() *sil.Value {
			// Storage where the receiver is one -- an initializer's,
			// or a mutating method's inout self -- and the value
			// otherwise. Either way it is borrowed for the call.
			if g.self != nil && g.self.addr != nil && !isClass(g.recv) {
				return g.loaded(g.blk.Load(g.self.addr, loadQualifier(g.self.typ)), g.self.typ)
			}
			return g.selfValue()
		}), true
	}
	return nil, false
}

// isComputedMember reports whether a name is a computed property of
// this type rather than one of its stored fields.
func (g *gen) isComputedMember(t types.Type, name string) bool {
	for _, f := range g.computedOf(t) {
		if f != nil && f.Name == name {
			return true
		}
	}
	return false
}

// setterAccessor is the `set` clause of a computed property, or nil
// where it has none.
func (g *gen) setterAccessor(b *ast.PatternBinding) *ast.Accessor {
	if b == nil || b.Accessors == nil {
		return nil
	}
	for _, a := range b.Accessors.Accessors {
		if a != nil && a.Keyword != nil && a.Body != nil && g.text(a.Keyword) == "set" {
			return a
		}
	}
	return nil
}

// emitSetter lowers the setter a computed property declares.
//
// Its shape is the getter's turned around: the value goes in rather
// than coming out. Swift's convention puts the new value first and
// self last, and self is @inout on a value type -- writing a property
// through it is the whole point, and a copy would be written and
// dropped.
func (g *gen) emitSetter(recv types.Type, name string, t types.Type, a *ast.Accessor, linkage sil.Linkage) {
	d := mangle.Decl{
		Module:    g.memberModule(recv, nil),
		Context:   memberChain(recv),
		Extended:  extendedBuiltin(recv),
		Name:      name,
		Signature: &types.Signature{Results: t},
		ModuleOf:  g.moduleOfType,
	}
	symbol, err := mangle.Setter(d)
	if err != nil {
		g.errorAt(a.Body, "cannot name the setter of '"+name+"': "+err.Error())
		return
	}
	g.emitSetterNamed(symbol, recv, name, t, a, linkage)
}

// emitSetterNamed is emitSetter under a symbol already chosen.
func (g *gen) emitSetterNamed(symbol string, recv types.Type, name string, t types.Type, a *ast.Accessor, linkage sil.Linkage) {
	f := g.m.Func(symbol).SetSourceName(name).
		SetLinkage(linkage).SetAttr("ossa")
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

	// The new value, first.
	vt := lowerType(t)
	value := f.Param(vt, paramConvention(&types.Param{Type: t}, vt))
	if sym := g.accessorValue(a); sym != nil {
		g.locals[sym] = &local{value: value, typ: vt}
		g.blk.DebugValue(value, "newValue", "let", "argno 1")
	}

	// Then self. A class receiver is a reference and a write goes
	// through it; a value receiver has to be the caller's storage or
	// the write lands in a copy.
	st := lowerType(recv)
	if isClass(recv) {
		f.Param(st, selfConvention(st))
	} else {
		g.self = &local{addr: f.Param(st.Address(), sil.ParamInout), typ: st}
	}
	f.Type().Convention = sil.Method

	g.block(a.Body)
	// A setter returns nothing, so falling off the end is how it
	// ordinarily finishes.
	if g.blk != nil && g.blk.Term() == nil {
		g.unwind()
		g.blk.Return(g.void())
	}
	g.pop()
	g.fn, g.blk, g.recv, g.self = nil, nil, nil, nil
}

// accessorValue is the symbol a setter's incoming value was declared
// under, which the analyzer put in the accessor's own scope.
func (g *gen) accessorValue(a *ast.Accessor) analyzer.Symbol {
	scope := g.info.Scopes[a]
	if scope == nil {
		return nil
	}
	if a.Name != nil {
		return scope.Lookup(g.text(a.Name))
	}
	name := "newValue"
	if a.Keyword != nil && g.text(a.Keyword) == "didSet" {
		name = "oldValue"
	}
	return scope.Lookup(name)
}

// computedField is the computed property of this type with this name.
func (g *gen) computedField(t types.Type, name string) (*types.Field, bool) {
	for _, f := range g.computedOf(t) {
		if f != nil && f.Name == name {
			return f, true
		}
	}
	return nil, false
}

// setterCall lowers an assignment to a computed property via its setter.
func (g *gen) setterCall(mem *ast.MemberExpr, recv types.Type, f *types.Field, value ast.Expr) {
	v := g.rvalue(value)
	if v == nil {
		return
	}
	g.setterCallValue(mem, recv, f, v)
}

// setterCallValue is setterCall over a value already lowered, which
// is what a compound assignment has: it read the property, applied
// the operator, and has the answer in hand.
func (g *gen) setterCallValue(mem *ast.MemberExpr, recv types.Type, f *types.Field, v *sil.Value) {
	if cl, ok := receiverClass(recv); ok && g.poly[cl] && f.LazyStorage == "" {
		g.dynamicSetter(mem, cl, f, v)
		return
	}
	valueType := f.Type
	name, spec, generic := g.genericAccessor(mem, recv, f, true)
	if generic {
		if name == "" {
			return
		}
		valueType = spec
	} else {
		d := mangle.Decl{
			Module:    g.memberModule(recv, f),
			Context:   memberChain(recv),
			Extended:  extendedBuiltin(recv),
			Name:      f.Name,
			Signature: &types.Signature{Results: f.Type},
			ModuleOf:  g.moduleOfType,
		}
		var err error
		name, err = mangle.Setter(d)
		if err != nil {
			g.errorAt(mem, "cannot name the setter of '"+f.Name+"': "+err.Error())
			return
		}
	}
	var self *sil.Value
	if isClass(recv) {
		self = g.expr(mem.X)
	} else {
		self = g.lvalue(mem.X)
	}
	if self == nil {
		g.refuse(mem, "an assignment to a computed property of something that is "+
			"not storage the setter can write through")
		return
	}

	callee := g.m.Func(name).SetSourceName(f.Name)
	if g.needsType(callee) {
		vt := lowerType(valueType)
		st := lowerType(recv)
		callee.Type().Params = append(callee.Type().Params,
			sil.Param{Type: vt, Convention: paramConvention(&types.Param{Type: valueType}, vt)})
		if isClass(recv) {
			callee.Type().Params = append(callee.Type().Params,
				sil.Param{Type: st, Convention: selfConvention(st)})
		} else {
			callee.Type().Params = append(callee.Type().Params,
				sil.Param{Type: st.Address(), Convention: sil.ParamInout})
		}
		callee.Type().Convention = sil.Method
	}
	ref := g.blk.FunctionRef(callee)
	g.blk.Apply(ref, sil.Object(types.Typ[types.Void]), v, self)
	// The setter borrows the new value; one this side owns, and nothing
	// else will end, ends here.
	vt := lowerType(valueType)
	if !vt.Trivial() && v.Ownership() == sil.Owned &&
		paramConvention(&types.Param{Type: valueType}, vt) == sil.ParamGuaranteed && !g.pendingDestroy(v) {
		g.blk.DestroyValue(v)
	}
}

// observedField returns the stored property if it has willSet or didSet observers.
func observedField(t types.Type, name string) (*types.Field, bool) {
	fields, _, _, _, _, _ := storedSinks(t)
	for _, f := range fields {
		if f != nil && f.Name == name && f.HasObservers {
			return f, true
		}
	}
	return nil, false
}

// storedSinks is a type's stored properties, seen through whatever
// name it was declared under.
func storedSinks(t types.Type) ([]*types.Field, []*types.Field, []*types.Field, []*types.Field, []*types.Field, []*types.Field) {
	if t == nil {
		return nil, nil, nil, nil, nil, nil
	}
	if inst, ok := t.(*types.GenericInstance); ok {
		t = inst.Base
	}
	switch n := t.Underlying().(type) {
	case *types.Struct:
		return n.Fields, nil, nil, nil, nil, nil
	case *types.Class:
		return n.Fields, nil, nil, nil, nil, nil
	}
	return nil, nil, nil, nil, nil, nil
}

// accessorNamed is the accessor of a binding with this keyword.
func (g *gen) accessorNamed(b *ast.PatternBinding, want string) *ast.Accessor {
	if b == nil || b.Accessors == nil {
		return nil
	}
	for _, a := range b.Accessors.Accessors {
		if a != nil && a.Keyword != nil && a.Body != nil && g.text(a.Keyword) == want {
			return a
		}
	}
	return nil
}

// emitObservedSetter lowers the setter for a stored property with willSet/didSet observers.
func (g *gen) emitObservedSetter(recv types.Type, name string, t types.Type, b *ast.PatternBinding, linkage sil.Linkage) {
	will, did := g.accessorNamed(b, "willSet"), g.accessorNamed(b, "didSet")
	if will == nil && did == nil {
		return
	}
	d := mangle.Decl{
		Module:    g.memberModule(recv, nil),
		Context:   memberChain(recv),
		Extended:  extendedBuiltin(recv),
		Name:      name,
		Signature: &types.Signature{Results: t},
		ModuleOf:  g.moduleOfType,
	}
	symbol, err := mangle.Setter(d)
	if err != nil {
		g.errorAt(b, "cannot name the setter of '"+name+"': "+err.Error())
		return
	}

	f := g.m.Func(symbol).SetSourceName(name).
		SetLinkage(linkage).SetAttr("ossa")
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

	vt := lowerType(t)
	value := f.Param(vt, paramConvention(&types.Param{Type: t}, vt))

	st := lowerType(recv)
	var selfValue *sil.Value
	if isClass(recv) {
		selfValue = f.Param(st, selfConvention(st))
	} else {
		selfValue = f.Param(st.Address(), sil.ParamInout)
		g.self = &local{addr: selfValue, typ: st}
	}
	f.Type().Convention = sil.Method

	member := memberName(recv, name)
	addr := func() *sil.Value {
		if isClass(recv) {
			return g.blk.RefElementAddr(selfValue, member, vt)
		}
		return g.blk.StructElementAddr(g.self.addr, member, vt.Address())
	}

	// The old value, before the store replaces it.
	var old *sil.Value
	if did != nil {
		a := addr()
		access := g.blk.BeginAccess(a, "read", "unknown")
		old = g.loaded(g.blk.Load(access, loadQualifier(vt)), vt)
		g.blk.EndAccess(access)
	}

	if will != nil {
		if sym := g.accessorValue(will); sym != nil {
			g.locals[sym] = &local{value: value, typ: vt}
		}
		g.block(will.Body)
	}

	if g.blk != nil && g.blk.Term() == nil {
		a := addr()
		access := g.blk.BeginAccess(a, "modify", "unknown")
		g.blk.Assign(value, access)
		g.blk.EndAccess(access)
	}

	if did != nil && g.blk != nil && g.blk.Term() == nil {
		if sym := g.accessorValue(did); sym != nil {
			g.locals[sym] = &local{value: old, typ: vt}
		}
		g.block(did.Body)
	}

	if g.blk != nil && g.blk.Term() == nil {
		g.unwind()
		g.blk.Return(g.void())
	}
	g.pop()
	g.fn, g.blk, g.recv, g.self = nil, nil, nil, nil
}

// setterField is the property a write goes through a setter for:
// a computed one, whose storage does not exist, or a stored one with
// observers, whose write is more than a store.
func (g *gen) setterField(t types.Type, name string) (*types.Field, bool) {
	if f, ok := g.computedField(t, name); ok {
		return f, true
	}
	return observedField(t, name)
}

// lazyInPlace reports whether f is a lazy property of a value type,
// whose getter writes the receiver and so is handed its storage.
func lazyInPlace(recv types.Type, f *types.Field) bool {
	return f != nil && f.LazyStorage != "" && !isClass(recv)
}

// getterOn reads property f of the value x lowers to: through its getter,
// which for a lazy property of a value type is handed x's storage -- or,
// where x is not storage, a copy's, which it fills and lets go.
func (g *gen) getterOn(at ast.Node, recv types.Type, f *types.Field, x ast.Expr) *sil.Value {
	if !lazyInPlace(recv, f) {
		return g.getterCall(at, recv, f, func() *sil.Value { return g.expr(x) })
	}
	said := len(g.diags)
	if addr := g.lvalue(x); addr != nil {
		return g.lazyGetterCall(at, recv, f, addr)
	}
	g.diags = g.diags[:said]
	v := g.rvalue(x)
	if v == nil {
		return nil
	}
	st := lowerType(recv)
	tmp := g.blk.AllocStack(st)
	g.blk.Store(v, tmp, storeQualifier(st))
	out := g.lazyGetterCall(at, recv, f, tmp)
	g.blk.DestroyAddr(tmp)
	g.blk.DeallocStack(tmp)
	return out
}

// lazyGetterCall calls a value type's lazy getter on the receiver's
// storage, as a mutating method is called.
func (g *gen) lazyGetterCall(at ast.Node, recv types.Type, f *types.Field, addr *sil.Value) *sil.Value {
	d := mangle.Decl{
		Module:    g.memberModule(recv, f),
		Context:   memberChain(recv),
		Extended:  extendedBuiltin(recv),
		Name:      f.Name,
		Signature: &types.Signature{Results: f.Type},
		ModuleOf:  g.moduleOfType,
	}
	name, err := mangle.Getter(d)
	if err != nil {
		g.errorAt(at, "cannot name the getter of '"+f.Name+"': "+err.Error())
		return nil
	}
	t := lowerType(f.Type)
	callee := g.m.Func(name).SetSourceName(f.Name)
	if g.needsType(callee) {
		st := lowerType(recv)
		callee.Type().Params = append(callee.Type().Params,
			sil.Param{Type: st.Address(), Convention: sil.ParamInout})
		callee.Type().Convention = sil.Method
		callee.SetResult(t, resultConvention(t))
	}
	access := g.blk.BeginAccess(addr, "modify", "unknown")
	v := g.blk.Apply(g.blk.FunctionRef(callee), t, access)
	g.blk.EndAccess(access)
	g.destroyLater(v)
	return v
}

// emitLazyAccessors lowers a lazy property's getter and setter, which
// Swift's `lazy var x: T = e` is: its storage is an optional, nil until
// the getter first runs; the getter answers what the storage holds, or
// runs e with self in scope, keeps it and answers it; the setter keeps
// what it is given.
func (g *gen) emitLazyAccessors(recv types.Type, f *types.Field, value ast.Expr, linkage sil.Linkage) {
	d := mangle.Decl{
		Module:    g.memberModule(recv, nil),
		Context:   memberChain(recv),
		Extended:  extendedBuiltin(recv),
		Name:      f.Name,
		Signature: &types.Signature{Results: f.Type},
		ModuleOf:  g.moduleOfType,
	}
	getter, err := mangle.Getter(d)
	if err != nil {
		g.errorAt(value, "cannot name the getter of '"+f.Name+"': "+err.Error())
		return
	}
	setter, err := mangle.Setter(d)
	if err != nil {
		g.errorAt(value, "cannot name the setter of '"+f.Name+"': "+err.Error())
		return
	}
	t := lowerType(f.Type)
	optType := &types.Optional{Wrapped: f.Type}
	opt := lowerType(optType)
	st := lowerType(recv)
	member := memberName(recv, f.LazyStorage)

	// begin opens the function and its self; storage is the optional's
	// place in self, accessed for kind.
	begin := func(symbol string) *sil.Func {
		fn := g.m.Func(symbol).SetSourceName(f.Name).SetLinkage(linkage).SetAttr("ossa")
		g.fn, g.recv, g.entry = fn, recv, false
		fn.Type().Params = nil
		g.locals = map[analyzer.Symbol]*local{}
		g.scopes = nil
		g.loops, g.pending = nil, ""
		g.self = nil
		g.push()
		g.blk = fn.Entry()
		return fn
	}
	self := func(fn *sil.Func) {
		if isClass(recv) {
			fn.Param(st, selfConvention(st))
		} else {
			g.self = &local{addr: fn.Param(st.Address(), sil.ParamInout), typ: st}
		}
		fn.Type().Convention = sil.Method
	}
	storage := func(kind string) *sil.Value {
		if isClass(recv) {
			return g.blk.BeginAccess(g.blk.RefElementAddr(g.selfValue(), member, opt), kind, "dynamic")
		}
		return g.blk.BeginAccess(g.blk.StructElementAddr(g.self.addr, member, opt.Address()), kind, "unknown")
	}
	end := func() {
		g.pop()
		g.fn, g.blk, g.recv, g.self = nil, nil, nil, nil
	}

	// The getter.
	fn := begin(getter)
	self(fn)
	fn.SetResult(t, resultConvention(t))
	access := storage("read")
	held := g.blk.Load(access, loadQualifier(opt))
	g.blk.EndAccess(access)
	own := sil.Owned
	if t.Trivial() {
		own = sil.Unowned
	}
	some, none := fn.Block(), fn.Block()
	payload := some.Arg(t, own)
	g.blk.SwitchEnum(held,
		sil.Case{Member: optionalSome, Dest: some},
		sil.Case{Member: optionalNone, Dest: none})
	g.blk = some
	g.unwind()
	g.blk.Return(payload)
	g.blk = none
	v := g.rvalue(value)
	if v == nil {
		end()
		return
	}
	v = g.optionalFor(value, v, g.typeOf(value), f.Type)
	kept := v
	if !t.Trivial() {
		kept = g.blk.CopyValue(v)
	}
	access = storage("modify")
	g.blk.Assign(g.blk.Enum(opt, optionalSome, kept), access)
	g.blk.EndAccess(access)
	g.unwind()
	g.blk.Return(v)
	end()

	// The setter: the new value first, then self.
	fn = begin(setter)
	nv := fn.Param(t, paramConvention(&types.Param{Type: f.Type}, t))
	self(fn)
	if !t.Trivial() && nv.Ownership() != sil.Owned {
		nv = g.blk.CopyValue(nv)
	}
	access = storage("modify")
	g.blk.Assign(g.blk.Enum(opt, optionalSome, nv), access)
	g.blk.EndAccess(access)
	g.unwind()
	g.blk.Return(g.void())
	_ = optType
	end()
}

// dynamicGetter reads a computed property of a class with subclasses
// through the table the instance carries, where an override is found.
func (g *gen) dynamicGetter(cl *types.Class, f *types.Field, receiver func() *sil.Value) *sil.Value {
	self := receiver()
	if self == nil {
		return nil
	}
	intro := propertyIntroducer(cl, f.Name)
	t := lowerType(f.Type)
	st := lowerType(intro)
	ft := &sil.FuncType{Convention: sil.Method}
	ft.Params = append(ft.Params, sil.Param{Type: st, Convention: selfConvention(st)})
	ft.Results = append(ft.Results, sil.Result{Type: t, Convention: resultConvention(t)})
	method := g.blk.ClassMethod(self, intro.Name+"."+f.Name+"!getter", sil.Object(ft))
	v := g.blk.Apply(method, t, self)
	g.destroyLater(v)
	return v
}

// dynamicSetter writes a computed property of a class with subclasses
// through the table the instance carries.
func (g *gen) dynamicSetter(mem *ast.MemberExpr, cl *types.Class, f *types.Field, v *sil.Value) {
	self := g.expr(mem.X)
	if self == nil {
		return
	}
	intro := propertyIntroducer(cl, f.Name)
	t := lowerType(f.Type)
	st := lowerType(intro)
	conv := paramConvention(&types.Param{Type: f.Type}, t)
	ft := &sil.FuncType{Convention: sil.Method}
	ft.Params = append(ft.Params, sil.Param{Type: t, Convention: conv}, sil.Param{Type: st, Convention: selfConvention(st)})
	method := g.blk.ClassMethod(self, intro.Name+"."+f.Name+"!setter", sil.Object(ft))
	g.blk.Apply(method, sil.Object(types.Typ[types.Void]), v, self)
	if !t.Trivial() && v.Ownership() == sil.Owned && conv == sil.ParamGuaranteed && !g.pendingDestroy(v) {
		g.blk.DestroyValue(v)
	}
}

// emitWrapperAccessors lowers the getter and setter of a property a
// property wrapper gives -- `level` of `@Clamped(0...10) var level`, or
// its projection `$level` -- which read and write the wrapper's
// wrappedValue (projectedValue) in the storage holding it, `_level`.
func (g *gen) emitWrapperAccessors(recv types.Type, f *types.Field, linkage sil.Linkage) {
	_, storage, ok := storedField(recv, f.Wrapper)
	if !ok {
		return
	}
	wt := storage.Type
	inner := "wrappedValue"
	if f.Projected {
		inner = "projectedValue"
	}
	wf := wrapperField(wt, inner)
	if wf == nil {
		return
	}
	d := mangle.Decl{
		Module:    g.memberModule(recv, nil),
		Context:   memberChain(recv),
		Extended:  extendedBuiltin(recv),
		Name:      f.Name,
		Signature: &types.Signature{Results: f.Type},
		ModuleOf:  g.moduleOfType,
	}
	getter, err := mangle.Getter(d)
	if err != nil {
		return
	}
	t := lowerType(f.Type)
	st := lowerType(recv)
	wlt := lowerType(wt)
	member := memberName(recv, f.Wrapper)
	begin := func(symbol string) *sil.Func {
		fn := g.m.Func(symbol).SetSourceName(f.Name).SetLinkage(linkage).SetAttr("ossa")
		g.fn, g.recv, g.entry = fn, recv, false
		fn.Type().Params = nil
		g.locals = map[analyzer.Symbol]*local{}
		g.scopes = nil
		g.loops, g.pending = nil, ""
		g.self = nil
		g.push()
		g.blk = fn.Entry()
		return fn
	}
	end := func() {
		g.pop()
		g.fn, g.blk, g.recv, g.self = nil, nil, nil, nil
	}

	// The getter: the wrapper out of self, and its property out of that.
	fn := begin(getter)
	self := fn.Param(st, selfConvention(st))
	fn.Type().Convention = sil.Method
	fn.SetResult(t, resultConvention(t))
	var wrapper *sil.Value
	if isClass(recv) {
		access := g.blk.BeginAccess(g.blk.RefElementAddr(self, member, wlt), "read", "dynamic")
		wrapper = g.blk.Load(access, loadQualifier(wlt))
		g.blk.EndAccess(access)
		g.destroyLater(wrapper)
	} else {
		wrapper = g.blk.StructExtract(self, member, wlt)
	}
	var out *sil.Value
	if wf.IsComputed {
		out = g.getterCall(nil, wt, wf, func() *sil.Value { return wrapper })
		out = g.consume(out)
	} else {
		v := g.blk.StructExtract(wrapper, memberName(wt, inner), t)
		if !t.Trivial() {
			v = g.blk.CopyValue(v)
		}
		out = v
	}
	g.unwind()
	g.blk.Return(out)
	end()

	if !f.HasSetter {
		return
	}
	setter, err := mangle.Setter(d)
	if err != nil {
		return
	}
	// The setter: the new value into the wrapper's property, in place.
	fn = begin(setter)
	nv := fn.Param(t, paramConvention(&types.Param{Type: f.Type}, t))
	var place *sil.Value
	if isClass(recv) {
		obj := fn.Param(st, selfConvention(st))
		place = g.blk.RefElementAddr(obj, member, wlt)
	} else {
		selfAddr := fn.Param(st.Address(), sil.ParamInout)
		place = g.blk.StructElementAddr(selfAddr, member, wlt.Address())
	}
	fn.Type().Convention = sil.Method
	owned := nv
	if !t.Trivial() && nv.Ownership() != sil.Owned {
		owned = g.blk.CopyValue(nv)
	}
	if wf.IsComputed {
		g.setterOnAddr(wt, wf, owned, place)
	} else {
		access := g.blk.BeginAccess(g.blk.StructElementAddr(place, memberName(wt, inner), t.Address()), "modify", "unknown")
		g.blk.Assign(owned, access)
		g.blk.EndAccess(access)
	}
	g.unwind()
	g.blk.Return(g.void())
	end()
}

// wrapperField is a wrapper type's property of a name, stored or computed.
func wrapperField(t types.Type, name string) *types.Field {
	if f, ok := storedFieldOf(t, name); ok {
		return f
	}
	var computed []*types.Field
	switch u := t.Underlying().(type) {
	case *types.Struct:
		computed = u.Computed
	case *types.Class:
		computed = u.Computed
	}
	for _, f := range computed {
		if f != nil && f.Name == name {
			return f
		}
	}
	return nil
}

// storedFieldOf is t's stored property of a name.
func storedFieldOf(t types.Type, name string) (*types.Field, bool) {
	_, f, ok := storedField(t, name)
	return f, ok
}

// setterOnAddr calls computed property f's setter on the value of type
// recv held at addr -- a struct's through its storage, a class's through
// the reference held there -- with v, which the caller owns.
func (g *gen) setterOnAddr(recv types.Type, f *types.Field, v, addr *sil.Value) {
	d := mangle.Decl{
		Module:    g.memberModule(recv, f),
		Context:   memberChain(recv),
		Extended:  extendedBuiltin(recv),
		Name:      f.Name,
		Signature: &types.Signature{Results: f.Type},
		ModuleOf:  g.moduleOfType,
	}
	name, err := mangle.Setter(d)
	if err != nil {
		return
	}
	vt := lowerType(f.Type)
	st := lowerType(recv)
	conv := paramConvention(&types.Param{Type: f.Type}, vt)
	callee := g.m.Func(name).SetSourceName(f.Name)
	if g.needsType(callee) {
		callee.Type().Params = append(callee.Type().Params, sil.Param{Type: vt, Convention: conv})
		if isClass(recv) {
			callee.Type().Params = append(callee.Type().Params, sil.Param{Type: st, Convention: selfConvention(st)})
		} else {
			callee.Type().Params = append(callee.Type().Params, sil.Param{Type: st.Address(), Convention: sil.ParamInout})
		}
		callee.Type().Convention = sil.Method
	}
	var self *sil.Value
	if isClass(recv) {
		self = g.blk.Load(addr, loadQualifier(st))
	} else {
		self = g.blk.BeginAccess(addr, "modify", "unknown")
	}
	g.blk.Apply(g.blk.FunctionRef(callee), sil.Object(types.Typ[types.Void]), v, self)
	if isClass(recv) {
		if !st.Trivial() {
			g.blk.DestroyValue(self)
		}
	} else {
		g.blk.EndAccess(self)
	}
	if !vt.Trivial() && conv == sil.ParamGuaranteed {
		g.blk.DestroyValue(v)
	}
}
