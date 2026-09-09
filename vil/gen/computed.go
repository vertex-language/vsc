package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Computed properties.
//
// A computed property is a function that looks like a field. It has
// no storage, so there is nothing to read: `v.magnitude` is a call to
// a getter that takes the receiver and hands back a value, and its
// symbol says so -- swiftc writes `$s5Wider3VecV9magnitudes5Int32Vvg`
// for `Vec.magnitude`, with `vg` for a variable's getter.
//
// This matters twice over. Reading one as a field reads bytes that
// are not there; and counting one as storage, which is the mistake
// underneath it, changes what the type's bytes are -- see
// analyzer/decl.go, where a property with a getter stopped being
// counted as a field.

// computedProperty is the property a member expression names, where
// it names one, together with the type that declares it.
func (g *gen) computedProperty(e *ast.MemberExpr) (types.Type, *types.Field, bool) {
	if e == nil || e.Name == nil {
		return nil, nil, false
	}
	recv := g.typeOf(e.X)
	name := g.text(e.Name)
	for t := recv; t != nil; {
		for _, f := range computedOf(t) {
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
func computedOf(t types.Type) []*types.Field {
	if t == nil {
		return nil
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

// getter lowers `v.magnitude`: a call to the property's getter with
// the receiver as its argument.
//
// A class whose method calls are dispatched through a table would
// dispatch its getters the same way, and the slot a getter occupies
// is not modelled -- so one is refused rather than bound to the
// static type's, which is the mistake methodCall already documents.
func (g *gen) getter(e *ast.MemberExpr, recv types.Type, f *types.Field) *vil.Value {
	return g.getterCall(e, recv, f, func() *vil.Value { return g.expr(e.X) })
}

// getterCall is the call a computed property's read becomes, over a
// receiver the caller supplies. `v.doubled` takes it from the
// expression before the dot; a bare `doubled` inside a member takes
// it from self.
func (g *gen) getterCall(at ast.Node, recv types.Type, f *types.Field,
	receiver func() *vil.Value) *vil.Value {

	if cl, ok := receiverClass(recv); ok && g.poly[cl] {
		g.refuse(at, "a computed property of a class with a subclass, whose getter is "+
			"reached through the table the instance carries")
		return nil
	}
	sig := &types.Signature{Results: f.Type}
	d := mangle.Decl{
		Module:    g.moduleOfType(recv),
		Context:   nominalChain(recv),
		Name:      f.Name,
		Signature: sig,
		ModuleOf:  g.moduleOfType,
	}
	name, err := mangle.Getter(d)
	if err != nil {
		g.errorAt(at, "cannot name the getter of '"+f.Name+"': "+err.Error())
		return nil
	}
	// Borrowed, not copied. A getter's self is @guaranteed the way a
	// method's is -- the caller keeps it alive across the call and
	// the callee does not consume it -- so an owned copy would be one
	// nothing afterwards destroys. On a class that is a reference the
	// verifier rejected: an owned value not consumed on all paths.
	self := receiver()
	if self == nil {
		return nil
	}
	callee := g.m.Func(name).SetSourceName(f.Name)
	if g.needsType(callee) {
		st := lowerType(recv)
		callee.Type().Params = append(callee.Type().Params,
			vil.Param{Type: st, Convention: selfConvention(st)})
		callee.Type().Convention = vil.Method
		callee.SetResult(lowerType(f.Type), resultConvention(lowerType(f.Type)))
	}
	ref := g.blk.FunctionRef(callee)
	v := g.blk.Apply(ref, lowerType(f.Type), self)
	g.destroyLater(v)
	return v
}

// emitGetters lowers the getters a type's computed properties are.
//
// A computed property is a function, so it needs a function emitted:
// the call site names the getter's symbol, and without a definition
// the link fails on a name that reads like a field access.
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
		for _, b := range m.Bindings {
			block := g.getterBody(b)
			if block == nil {
				continue
			}
			name, ftype := g.computedName(b), g.bindingType(b, recv)
			if name == "" || ftype == nil {
				continue
			}
			g.emitGetter(recv, name, ftype, block, static)
			// A setter is emitted beside it where the property
			// declares one. Static setters wait on static storage.
			if !static {
				if set := g.setterAccessor(b); set != nil {
					g.emitSetter(recv, name, ftype, set)
				}
			}
		}
	}
}

// getterBody is the block a computed property's getter runs, or nil
// where the binding is not a computed property with a body here.
//
// Two spellings. `var x: Int { … }` is the implicit getter and its
// block is the binding's own; `var x: Int { get { … } }` writes the
// accessor out. A `get` with no block is a protocol's requirement,
// which promises a getter without saying what it does.
func (g *gen) getterBody(b *ast.PatternBinding) *ast.CodeBlock {
	return g.accessorBody(b, "get")
}

// setterBody is the block a computed property's setter runs, or nil
// where it has none.
func (g *gen) setterBody(b *ast.PatternBinding) *ast.CodeBlock {
	return g.accessorBody(b, "set")
}

// accessorBody is the block of the accessor with this keyword, or the
// implicit getter's where the property was written as one.
func (g *gen) accessorBody(b *ast.PatternBinding, want string) *ast.CodeBlock {
	if b == nil {
		return nil
	}
	if b.Body != nil {
		// `var x: Int { … }` is the implicit getter and has no other
		// accessor to be.
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
		// The keyword decides which accessor this is. Taking the
		// first one with a body took the setter's where a property
		// wrote `set` before `get`, and emitted it as the getter --
		// which compiled, ran, and answered whatever the setter's
		// body left behind.
		if g.text(a.Keyword) == want {
			return a.Body
		}
	}
	return nil
}

// bindingType is the type a computed property was declared with,
// found among the ones the checker read off the type.
func (g *gen) bindingType(b *ast.PatternBinding, recv types.Type) types.Type {
	name := g.computedName(b)
	for _, f := range computedOf(recv) {
		if f != nil && f.Name == name {
			return f.Type
		}
	}
	// A static computed property is among the type's statics rather
	// than among its computed ones -- the two kinds share that list.
	// Looking only at the instance ones left a static getter with no
	// type, so none was emitted and the call site named a symbol
	// nothing defined.
	for _, f := range staticsOf(recv) {
		if f != nil && f.Name == name && f.IsComputed {
			return f.Type
		}
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
	body *ast.CodeBlock, static bool) {

	sig := &types.Signature{Results: t}
	d := mangle.Decl{
		Module:    g.moduleOfType(recv),
		Context:   nominalChain(recv),
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

	f := g.m.Func(symbol).SetSourceName(name).
		SetLinkage(vil.Hidden).SetAttr("ossa")
	g.fn = f
	g.recv = recv
	g.entry = false
	f.Type().Params = nil
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes = nil
	g.loops, g.pending = nil, ""
	// Cleared per function, like the locals. A getter emitted after a
	// mutating method or an initializer inherited the storage that
	// one was handed, and read its properties through an address
	// belonging to another function -- which the verifier caught as a
	// use its definition does not dominate.
	g.self = nil
	g.push()
	g.blk = f.Entry()

	// A static getter has no instance to read. What stands in for one
	// is the metatype, and a struct's is thin -- the type is known,
	// so there is nothing to carry and no parameter to declare.
	if !static {
		st := lowerType(recv)
		f.Param(st, selfConvention(st))
		f.Type().Convention = vil.Method
	}
	f.SetResult(lowerType(t), resultConvention(lowerType(t)))

	g.block(body)
	if g.blk != nil && g.blk.Term() == nil {
		g.unwind()
		g.blk.Unreachable()
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
	for _, f := range staticsOf(meta.Instance) {
		if f != nil && f.Name == name {
			return meta.Instance, f, true
		}
	}
	return nil, nil, false
}

// staticsOf is a type's stored static properties.
func staticsOf(t types.Type) []*types.Field {
	if t == nil {
		return nil
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

// staticRead lowers `Vec.unit`: the address of the type's storage,
// and a load from it.
//
// A static stored property is not a field of anything, so there is no
// offset to read it at. Swift gives it storage of its own and an
// accessor that hands back where that storage is -- initialising it
// the first time if it has to -- and swiftc's own client code calls
// that accessor and loads what comes back.
func (g *gen) staticRead(e *ast.MemberExpr, recv types.Type, f *types.Field) *vil.Value {
	// A computed static is a getter with nothing behind it, so there
	// is no storage to want: it is a call, and the same call whether
	// the type was declared here or imported.
	if f.IsComputed {
		return g.staticGetter(e, recv, f)
	}
	// Only one declared elsewhere. A static of this module's own
	// would need the storage and the one-time initializer that fills
	// it -- swiftc emits a global and a `_WZ` beside it -- and this
	// compiler has no globals yet. Reading one is a call to an
	// accessor nothing would define.
	if _, imported := g.info.ImportedTypes[recv]; !imported {
		g.refuse(e, "a stored property of a type declared here: it needs storage of "+
			"its own and the one-time initializer that fills it")
		return nil
	}
	d := mangle.Decl{
		Module:    g.moduleOfType(recv),
		Context:   nominalChain(recv),
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
		callee.SetResult(t.Address(), vil.ResultUnowned)
	}
	addr := g.blk.Apply(g.blk.FunctionRef(callee), t.Address())
	v := g.blk.Load(addr, loadQualifier(t))
	return g.loaded(v, t)
}

// staticGetter lowers `Counter.base` where base is a computed
// property of the type: a call to the getter, with no instance to
// pass and no storage to read.
func (g *gen) staticGetter(e *ast.MemberExpr, recv types.Type, f *types.Field) *vil.Value {
	sig := &types.Signature{Results: f.Type}
	d := mangle.Decl{
		Module:    g.moduleOfType(recv),
		Context:   nominalChain(recv),
		Name:      f.Name,
		Signature: sig,
		ModuleOf:  g.moduleOfType,
	}
	name, err := mangle.StaticGetter(d)
	if err != nil {
		g.errorAt(e, "cannot name the getter of '"+f.Name+"': "+err.Error())
		return nil
	}
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

// implicitStatic reads a static of the type a member is written in,
// for a bare name that is one.
//
// A type's statics are in scope unqualified inside its own members,
// the way its instance properties are through implicit self --
// `unit` in a static getter of Vec means `Vec.unit`. There is no
// receiver: a static belongs to the type, so this is the same call
// the qualified form makes.
func (g *gen) implicitStatic(e *ast.IdentExpr) (*vil.Value, bool) {
	if g.recv == nil || e.Name == nil {
		return nil, false
	}
	name := g.text(e.Name)
	for _, f := range staticsOf(g.recv) {
		if f == nil || f.Name != name {
			continue
		}
		if !f.IsComputed {
			// A stored one still needs the storage and the one-time
			// initializer that fills it. Saying so here rather than
			// falling through keeps the reason with the name.
			g.refuse(e, "a stored property of a type declared here: it needs storage of "+
				"its own and the one-time initializer that fills it")
			return nil, true
		}
		member := &ast.MemberExpr{Name: e.Name}
		return g.staticGetter(member, g.recv, f), true
	}
	return nil, false
}

// implicitComputed reads a computed property of the type a member is
// written in, for a bare name that is one.
//
// `doubled` inside another member of the same type means
// `self.doubled`, the way a bare stored name means `self.n`. The
// difference is that this one is a call: a computed property is a
// getter with nothing behind it.
func (g *gen) implicitComputed(e *ast.IdentExpr) (*vil.Value, bool) {
	if g.recv == nil || e.Name == nil {
		return nil, false
	}
	name := g.text(e.Name)
	for _, f := range computedOf(g.recv) {
		if f == nil || f.Name != name {
			continue
		}
		return g.getterCall(e, g.recv, f, func() *vil.Value {
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
func isComputedMember(t types.Type, name string) bool {
	for _, f := range computedOf(t) {
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
func (g *gen) emitSetter(recv types.Type, name string, t types.Type, a *ast.Accessor) {
	d := mangle.Decl{
		Module:    g.moduleOfType(recv),
		Context:   nominalChain(recv),
		Name:      name,
		Signature: &types.Signature{Results: t},
		ModuleOf:  g.moduleOfType,
	}
	symbol, err := mangle.Setter(d)
	if err != nil {
		g.errorAt(a.Body, "cannot name the setter of '"+name+"': "+err.Error())
		return
	}

	f := g.m.Func(symbol).SetSourceName(name).
		SetLinkage(vil.Hidden).SetAttr("ossa")
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
		g.self = &local{addr: f.Param(st.Address(), vil.ParamInout), typ: st}
	}
	f.Type().Convention = vil.Method

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
	name := "newValue"
	if a.Name != nil {
		name = g.text(a.Name)
	}
	return scope.Lookup(name)
}

// computedField is the computed property of this type with this name.
func computedField(t types.Type, name string) (*types.Field, bool) {
	for _, f := range computedOf(t) {
		if f != nil && f.Name == name {
			return f, true
		}
	}
	return nil, false
}

// setterCall lowers `c.doubled = 20`: the new value and the receiver,
// handed to the property's setter.
//
// The receiver is storage on a value type -- the setter writes a
// property through it, and a copy would be written and dropped -- and
// the reference itself on a class, where the write goes through the
// reference and the object is what changes.
func (g *gen) setterCall(mem *ast.MemberExpr, recv types.Type, f *types.Field, value ast.Expr) {
	if cl, ok := receiverClass(recv); ok && g.poly[cl] {
		g.refuse(mem, "a computed property of a class with a subclass, whose setter is "+
			"reached through the table the instance carries")
		return
	}
	v := g.rvalue(value)
	if v == nil {
		return
	}
	g.setterCallValue(mem, recv, f, v)
}

// setterCallValue is setterCall over a value already lowered, which
// is what a compound assignment has: it read the property, applied
// the operator, and has the answer in hand.
func (g *gen) setterCallValue(mem *ast.MemberExpr, recv types.Type, f *types.Field, v *vil.Value) {
	if cl, ok := receiverClass(recv); ok && g.poly[cl] {
		g.refuse(mem, "a computed property of a class with a subclass, whose setter is "+
			"reached through the table the instance carries")
		return
	}
	d := mangle.Decl{
		Module:    g.moduleOfType(recv),
		Context:   nominalChain(recv),
		Name:      f.Name,
		Signature: &types.Signature{Results: f.Type},
		ModuleOf:  g.moduleOfType,
	}
	name, err := mangle.Setter(d)
	if err != nil {
		g.errorAt(mem, "cannot name the setter of '"+f.Name+"': "+err.Error())
		return
	}
	var self *vil.Value
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
		vt := lowerType(f.Type)
		st := lowerType(recv)
		callee.Type().Params = append(callee.Type().Params,
			vil.Param{Type: vt, Convention: paramConvention(&types.Param{Type: f.Type}, vt)})
		if isClass(recv) {
			callee.Type().Params = append(callee.Type().Params,
				vil.Param{Type: st, Convention: selfConvention(st)})
		} else {
			callee.Type().Params = append(callee.Type().Params,
				vil.Param{Type: st.Address(), Convention: vil.ParamInout})
		}
		callee.Type().Convention = vil.Method
	}
	ref := g.blk.FunctionRef(callee)
	g.blk.Apply(ref, vil.Object(types.Typ[types.Void]), v, self)
}
