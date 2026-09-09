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
	if cl, ok := receiverClass(recv); ok && g.poly[cl] {
		g.refuse(e, "a computed property of a class with a subclass, whose getter is "+
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
		g.errorAt(e, "cannot name the getter of '"+f.Name+"': "+err.Error())
		return nil
	}
	self := g.rvalue(e.X)
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
		if !ok || isStaticDecl(m.Mods) {
			continue
		}
		for _, b := range m.Bindings {
			block := getterBody(b)
			if block == nil {
				continue
			}
			name, ftype := g.computedName(b), g.bindingType(b, recv)
			if name == "" || ftype == nil {
				continue
			}
			g.emitGetter(recv, name, ftype, block)
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
func getterBody(b *ast.PatternBinding) *ast.CodeBlock {
	if b == nil {
		return nil
	}
	if b.Body != nil {
		return b.Body
	}
	if b.Accessors == nil {
		return nil
	}
	for _, a := range b.Accessors.Accessors {
		if a != nil && a.Keyword != nil && a.Body != nil {
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
func (g *gen) emitGetter(recv types.Type, name string, t types.Type, body *ast.CodeBlock) {
	sig := &types.Signature{Results: t}
	d := mangle.Decl{
		Module:    g.moduleOfType(recv),
		Context:   nominalChain(recv),
		Name:      name,
		Signature: sig,
		ModuleOf:  g.moduleOfType,
	}
	symbol, err := mangle.Getter(d)
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
	g.push()
	g.blk = f.Entry()

	st := lowerType(recv)
	f.Param(st, selfConvention(st))
	f.Type().Convention = vil.Method
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
