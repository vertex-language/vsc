package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
)

// Class dispatch table (sil_vtable) generation.

// vtables emits one dispatch table per class declared in the module.
func (g *gen) vtables(files []*ast.File) {
	for _, f := range files {
		for _, stmt := range f.Stmts {
			decl, ok := stmt.(*ast.DeclStmt)
			if !ok {
				continue
			}
			var name *ast.Ident
			switch d := decl.D.(type) {
			case *ast.ClassDecl:
				name = d.Name
			case *ast.ActorDecl:
				name = d.Name
			default:
				continue
			}
			if name == nil {
				continue
			}
			sym, _ := g.info.Defs[name].(*analyzer.TypeNameSymbol)
			if sym == nil {
				continue
			}
			cl, ok := sym.Type().Underlying().(*types.Class)
			if !ok {
				continue
			}
			// A generic class has a table for each instance it is made
			// as, emitted with the initializer that makes it.
			if len(cl.TypeParams) > 0 {
				continue
			}
			t := g.m.VTable(cl.Name)
			t.Layout = sym.Type()
			if declaresDeinit(decl.D) {
				t.Deinit = deinitSymbol(g.module, sym.Type())
			}
			// Every class has metadata, so that an instance can say what
			// its dynamic type is whatever static type it is held as.
			if len(cl.TypeParams) == 0 {
				g.needMetadata(name, sym.Type())
			}
			for _, s := range g.slots(sym.Type()) {
				t.Entry(s.member, s.impl)
			}
		}
	}
}

// A slot is one row of a dispatch table.
type slot struct {
	member string
	impl   string
}

// slots returns the dispatch table slots for class t in inheritance order.
// t may be an instance of a generic class, and any class it inherits from
// may be one: a row of such a class is its method specialized for it.
func (g *gen) slots(t types.Type) []slot {
	cl, ok := t.Underlying().(*types.Class)
	if !ok {
		return nil
	}
	var out []slot
	at := map[string]int{}
	levels := typeChain(t)
	for li, c := range classChain(cl) {
		var inst *types.GenericInstance
		if li < len(levels) {
			inst, _ = levels[li].(*types.GenericInstance)
		}
		for mi, m := range c.Methods {
			if m == nil || m.Sig == nil {
				continue
			}
			// A method with type parameters of its own is specialized
			// where it is called, and has no one row to be.
			if len(m.Sig.TypeParams) > 0 {
				continue
			}
			ref := &analyzer.MethodRef{Recv: c, Method: m}
			var impl string
			switch {
			case inst != nil && !m.IsStatic:
				impl = g.instanceMethodSymbol(inst, mi)
			case m.IsStatic:
				// A class method is reached through a metatype's table,
				// and is the static function it is.
				impl = g.staticSymbol(ref, c)
			default:
				impl = g.methodSymbol(ref)
			}
			if impl == "" {
				continue
			}
			key := m.Name + m.Sig.String()
			if i, ok := at[key]; ok {
				out[i].impl = impl
				continue
			}
			at[key] = len(out)
			out = append(out, slot{member: c.Name + "." + m.Name, impl: impl})
		}
		// A required initializer is called through a metatype --
		// `type.init(id:)` -- and each class's own makes that class, so
		// its allocating entry is a row too.
		for _, sig := range c.Inits {
			if sig == nil || !sig.Required {
				continue
			}
			made := *sig
			made.Results = c
			impl := g.initSymbol(c, &made)
			if impl == "" {
				continue
			}
			key := initSlotKey(sig)
			if i, ok := at[key]; ok {
				out[i].impl = impl
				continue
			}
			at[key] = len(out)
			out = append(out, slot{member: c.Name + "." + key, impl: impl})
		}
		// A computed property's getter and setter are overridable as a
		// method is, and are rows of the table after the methods.
		for _, f := range c.Computed {
			if f == nil {
				continue
			}
			accessors := []string{"getter"}
			if f.HasSetter {
				accessors = append(accessors, "setter")
			}
			for _, kind := range accessors {
				impl := g.accessorSymbol(c, f, kind)
				if impl == "" {
					continue
				}
				key := f.Name + "!" + kind
				if i, ok := at[key]; ok {
					out[i].impl = impl
					continue
				}
				at[key] = len(out)
				out = append(out, slot{member: c.Name + "." + key, impl: impl})
			}
		}
	}
	return out
}

// typeChain is t and the classes it inherits from, base-most first, as
// types: Container<Int> where IntBag inherits from that instance.
func typeChain(t types.Type) []types.Type {
	var chain []types.Type
	seen := map[*types.Class]bool{}
	for cur := t; cur != nil; {
		cl, ok := cur.Underlying().(*types.Class)
		if !ok || seen[cl] {
			break
		}
		seen[cl] = true
		chain = append([]types.Type{cur}, chain...)
		cur = cl.Superclass
	}
	return chain
}

// instanceMethodSymbol is the i'th method of generic class instance inst,
// specialized for it -- Container<Int>'s add -- lowered once, as a call
// of it through the instance is (genericMethod).
func (g *gen) instanceMethodSymbol(inst *types.GenericInstance, i int) string {
	base := inst.Base
	declared, ok := base.Underlying().(*types.Class)
	params := nominalTypeParams(base)
	if !ok || i >= len(declared.Methods) || len(params) != len(inst.Args) {
		return ""
	}
	m := declared.Methods[i]
	decl := g.genericMethodDecl(genericMethodKey{typ: base, name: m.Name}, m)
	if decl == nil {
		return ""
	}
	subst := make(map[*types.TypeParam]types.Type, len(params))
	for k, v := range g.subst {
		subst[k] = v
	}
	for j, p := range params {
		subst[p] = inst.Args[j]
	}
	sig, ok := types.Substitute(m.Sig, subst).(*types.Signature)
	if !ok {
		return ""
	}
	mangled, err := mangle.Function(mangle.Decl{
		Module:    g.memberModule(base, m),
		Context:   memberChain(base),
		Name:      m.Name,
		Signature: sig,
		ModuleOf:  g.moduleOfType,
	})
	if err != nil {
		return ""
	}
	name := mangled + "Tv"
	for _, a := range inst.Args {
		name += identifierSafe(a.String())
	}
	g.emitMethodSpecialization(decl, inst, name, subst)
	return name
}

// initSlotKey names a required initializer's row by its parameters.
func initSlotKey(sig *types.Signature) string {
	key := "init("
	for _, p := range sig.Params {
		key += p.Label + ":" + p.Type.String() + ","
	}
	return key + ")!allocator"
}

// initIntroducer is the base-most class declaring required initializer sig.
func initIntroducer(cl *types.Class, sig *types.Signature) *types.Class {
	key := initSlotKey(sig)
	for _, c := range classChain(cl) {
		for _, own := range c.Inits {
			if own != nil && own.Required && initSlotKey(own) == key {
				return c
			}
		}
	}
	return cl
}

// accessorSymbol is the symbol of a class's computed property's getter
// or setter, or "".
func (g *gen) accessorSymbol(cl *types.Class, f *types.Field, kind string) string {
	d := mangle.Decl{
		Module:    g.memberModule(cl, f),
		Context:   memberChain(cl),
		Name:      f.Name,
		Signature: &types.Signature{Results: f.Type},
		ModuleOf:  g.moduleOfType,
	}
	var name string
	var err error
	if kind == "setter" {
		name, err = mangle.Setter(d)
	} else {
		name, err = mangle.Getter(d)
	}
	if err != nil {
		return ""
	}
	return name
}

// propertyIntroducer is the base-most class declaring computed property
// name: the class its table rows are named for.
func propertyIntroducer(cl *types.Class, name string) *types.Class {
	for _, c := range classChain(cl) {
		for _, f := range c.Computed {
			if f != nil && f.Name == name {
				return c
			}
		}
	}
	return cl
}

// classChain returns the inheritance chain from root base class to cl.
func classChain(cl *types.Class) []*types.Class {
	var chain []*types.Class
	seen := map[*types.Class]bool{}
	for c := cl; c != nil && !seen[c]; {
		seen[c] = true
		chain = append([]*types.Class{c}, chain...)
		next, _ := c.Superclass.(*types.Class)
		if next == nil && c.Superclass != nil {
			next, _ = c.Superclass.Underlying().(*types.Class)
		}
		c = next
	}
	return chain
}

// slotIndex is where a member sits in cl's table, for a call that
// knows the member and the static class.
func slotIndex(slots []slot, member string) (int, bool) {
	for i, s := range slots {
		if s.member == member {
			return i, true
		}
	}
	return 0, false
}

// declaresDeinit reports whether a class declaration has a deinit.
func declaresDeinit(d ast.Decl) bool {
	var body *ast.MemberBlock
	switch c := d.(type) {
	case *ast.ClassDecl:
		body = c.Body
	case *ast.ActorDecl:
		body = c.Body
	}
	if body == nil {
		return false
	}
	for _, m := range body.Members {
		if _, ok := m.(*ast.DeinitDecl); ok {
			return true
		}
	}
	return false
}
