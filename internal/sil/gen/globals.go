package gen

import (
	"strings"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
)

// A moduleVar is a variable declared at the top level of a file other than
// main.swift. It has storage of its own, which the addressor every use goes
// through initializes on first use, as Swift's globals are.
type moduleVar struct {
	name      string
	addressor string
	storage   string
	typ       types.Type
	binding   *ast.PatternBinding // nil for a static stored property
	value     ast.Expr            // the initializer
	// static is a type's static stored property.
	static bool
	recv   types.Type // the type a static stored property belongs to
}

// A moduleGetter is a computed variable declared at the top level of a file:
// a getter and nothing stored, which every read calls.
type moduleGetter struct {
	name    string
	symbol  string
	typ     types.Type
	binding *ast.PatternBinding
}

// moduleGetterCall reads a computed module-level variable by calling its
// getter.
func (g *gen) moduleGetterCall(sym analyzer.Symbol) (*sil.Value, bool) {
	gt, ok := g.getters[sym]
	if !ok || g.blk == nil {
		return nil, false
	}
	t := lowerType(gt.typ)
	callee := g.m.Func(gt.symbol).SetSourceName(gt.name)
	if g.needsType(callee) {
		callee.Type().Params = nil
		callee.SetResult(t, resultConvention(t))
	}
	v := g.blk.Apply(g.blk.FunctionRef(callee), t)
	g.destroyLater(v)
	return v, true
}

// declareModuleVars records the module-level variables a file declares.
func (g *gen) declareModuleVars(f *ast.File) {
	for _, stmt := range f.Stmts {
		decl, ok := stmt.(*ast.DeclStmt)
		if !ok {
			continue
		}
		d, ok := decl.D.(*ast.VarDecl)
		if !ok {
			continue
		}
		for _, b := range d.Bindings {
			name, sym := g.binding(b)
			if sym == nil {
				g.refuse(b, "a module-level variable bound to a pattern other than a name")
				continue
			}
			if b.Accessors != nil || b.Body != nil {
				// A computed variable is its getter, which a read calls.
				getter, err := mangle.Getter(mangle.Decl{
					Module:    g.module,
					ModuleOf:  g.moduleOfType,
					Name:      name,
					Signature: &types.Signature{Results: sym.Type()},
				})
				if err != nil {
					g.refuse(b, "a computed variable this compiler cannot name: "+err.Error())
					continue
				}
				g.getters[sym] = &moduleGetter{name: name, symbol: getter, typ: sym.Type(), binding: b}
				continue
			}
			if b.Value == nil {
				g.refuse(b, "a module-level variable with no initial value")
				continue
			}
			t := sym.Type()
			addressor, err := mangle.Addressor(mangle.Decl{
				Module:    g.module,
				ModuleOf:  g.moduleOfType,
				Name:      name,
				Signature: &types.Signature{Results: t},
			})
			if err != nil {
				g.refuse(b, "a module-level variable this compiler cannot name: "+err.Error())
				continue
			}
			g.vars[sym] = &moduleVar{
				name:      name,
				addressor: addressor,
				storage:   strings.TrimSuffix(addressor, "au") + "p",
				typ:       t,
				binding:   b,
				value:     b.Value,
			}
		}
	}
}

// emitModuleVars emits the storage and addressor of each variable d declares.
func (g *gen) emitModuleVars(d *ast.VarDecl) {
	for _, b := range d.Bindings {
		_, sym := g.binding(b)
		if gt, ok := g.getters[sym]; ok && gt.binding == b {
			if body := g.getterBody(b); body != nil {
				g.emitGetter(nil, gt.name, gt.typ, body, false, g.accessLinkage(d.Mods))
			}
			continue
		}
		if v, ok := g.vars[sym]; ok && v.binding == b {
			g.emitModuleVar(v)
		}
	}
}

// emitModuleVar emits a variable's addressor: the first call runs the
// initializer into the storage, and every call returns where it is.
func (g *gen) emitModuleVar(v *moduleVar) {
	t := lowerType(v.typ)
	storage := g.m.Global(v.storage, t, sil.Private)
	once := g.m.Global(v.storage+"_once", lowerType(types.Typ[types.Bool]), sil.Private)

	f := g.m.Func(v.addressor).SetSourceName(v.name).SetLinkage(sil.Public).SetAttr("ossa")
	f.Type().Params = nil
	f.Type().Convention = sil.Thin
	// A raw pointer, as Swift's own addressors return: an address-typed
	// result is a different thing to the call, and read as one it was not
	// where the value is.
	f.SetResult(rawPointerType(), sil.ResultUnowned)

	outer := struct {
		fn      *sil.Func
		entry   bool
		blk     *sil.Block
		scopes  []*scope
		locals  map[analyzer.Symbol]*local
		loops   []loop
		pending string
		recv    types.Type
		throws  bool
		catches []catchTarget
		statics types.Type
	}{g.fn, g.entry, g.blk, g.scopes, g.locals, g.loops, g.pending, g.recv, g.throws, g.catches, g.staticRecv}
	defer func() {
		g.fn, g.entry, g.blk = outer.fn, outer.entry, outer.blk
		g.scopes, g.locals = outer.scopes, outer.locals
		g.loops, g.pending, g.recv = outer.loops, outer.pending, outer.recv
		g.throws, g.catches, g.staticRecv = outer.throws, outer.catches, outer.statics
	}()
	g.fn, g.entry = f, false
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes, g.loops, g.pending, g.recv = nil, nil, "", nil
	g.throws, g.catches = false, nil
	// A static's initializer names the type's other statics unqualified.
	g.staticRecv = v.recv
	g.push()
	g.blk = f.Entry()

	done := g.blk.Load(g.blk.GlobalAddr(once), "trivial")
	bit := g.machine(done, types.Typ[types.Bool])
	first, ready := g.fn.Block(), g.fn.Block()
	g.blk.CondBr(bit, ready, nil, first, nil)

	g.blk = first
	yes := g.boolOf(g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt1), 1))
	g.blk.Store(yes, g.blk.GlobalAddr(once), "trivial")
	if value := g.rvalue(v.value); value != nil {
		value = g.optionalFor(v.value, value, g.typeOf(v.value), v.typ)
		g.blk.Store(value, g.blk.GlobalAddr(storage), storeQualifier(t))
	}
	g.unwind()
	if g.blk != nil && g.blk.Term() == nil {
		g.blk.Br(ready)
	}

	g.blk = ready
	g.blk.Return(g.blk.AddressToPointer(g.blk.GlobalAddr(storage), rawPointerType()))
}

// emitStatics emits the addressor of each static stored property a type
// declared here has: storage of its own, filled by its initializer on
// first use, under the symbol a read of the property calls.
func (g *gen) emitStatics(recv types.Type) {
	for _, f := range g.staticsOf(recv) {
		if f == nil || f.IsComputed {
			continue
		}
		value := g.info.FieldDefaults[f]
		if value == nil {
			g.errorAt(nil, "static property '"+f.Name+"' has no initial value")
			continue
		}
		name, err := mangle.Addressor(mangle.Decl{
			Module:    g.memberModule(recv, f),
			Context:   memberChain(recv),
			Extended:  extendedBuiltin(recv),
			Name:      f.Name,
			Signature: &types.Signature{Results: f.Type},
			ModuleOf:  g.moduleOfType,
		})
		if err != nil {
			g.refuse(value, "a static property this compiler cannot name: "+err.Error())
			continue
		}
		g.emitModuleVar(&moduleVar{
			name:      f.Name,
			addressor: name,
			storage:   strings.TrimSuffix(name, "au") + "p",
			typ:       f.Type,
			value:     value,
			static:    true,
			recv:      recv,
		})
	}
}

// moduleVarAddr is the address of a module-level variable, from its
// addressor, and the type stored there.
func (g *gen) moduleVarAddr(sym analyzer.Symbol) (*sil.Value, sil.Type, bool) {
	v, ok := g.vars[sym]
	if !ok || g.blk == nil {
		return nil, sil.Type{}, false
	}
	callee := g.m.Func(v.addressor).SetSourceName(v.name)
	if g.needsType(callee) {
		callee.Type().Convention = sil.Thin
		callee.SetResult(rawPointerType(), sil.ResultUnowned)
	}
	t := lowerType(v.typ)
	p := g.blk.Apply(g.blk.FunctionRef(callee), rawPointerType())
	return g.blk.PointerToAddress(p, t.Address()), t, true
}
