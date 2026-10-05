package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
)

// Imported generic functions are called via Swift's metadata convention, passing type metadata and indirect addresses.

// callImportedGeneric lowers a call to a generic function this module
// did not declare.
func (g *gen) callImportedGeneric(e *ast.CallExpr, sym *analyzer.FuncSymbol,
	spec analyzer.Specialization) *sil.Value {
	sig := sym.Signature()
	for _, a := range spec.Args {
		if a == nil {
			g.refuse(e, "a call whose type arguments could not be inferred")
			return nil
		}
	}
	// The metadata for each type argument, in the order the signature
	// declares its parameters. A type declared here has none, which is
	// where this stops.
	metas := make([]*sil.Value, 0, len(sig.TypeParams))
	for _, tp := range sig.TypeParams {
		arg := spec.Subst()[tp]
		if arg == nil {
			g.refuse(e, "a call whose type arguments could not be inferred")
			return nil
		}
		meta, ok := g.stdlibMetadata(e, arg)
		if !ok {
			return nil
		}
		metas = append(metas, meta)
	}
	if !g.genericShapeIsPlain(e, sig) {
		return nil
	}

	name, err := mangle.Function(mangle.Decl{
		Module:    g.moduleOf(sym),
		Name:      sym.Name(),
		Signature: sig,
		ModuleOf:  g.moduleOfType,
	})
	if err != nil {
		g.refuse(e, "a generic function this compiler cannot name: "+err.Error())
		return nil
	}
	// One the runtime implements says which of its functions it is.
	if silgen, has := g.silgenName(sym); has && silgen != "" {
		name = silgen
	}
	callee := g.m.Func(name).SetSourceName(sym.Name())
	if g.needsType(callee) {
		callee.SetLinkage(sil.PublicExternal)
		g.declareImportedGeneric(callee, sig)
	}

	// The arguments the source wrote. One whose parameter is a type
	// parameter goes into storage and travels as its address.
	var args, slots []*sil.Value
	if e.Args != nil {
		for i, a := range e.Args.Args {
			// An inout argument is its storage already.
			if i < len(sig.Params) && sig.Params[i].Ownership == types.InOut {
				v := g.expr(a.X)
				if v == nil {
					return nil
				}
				args = append(args, v)
				continue
			}
			v := g.rvalue(a.X)
			if v == nil {
				return nil
			}
			if i < len(sig.Params) && isTypeParam(sig.Params[i].Type) {
				slot := g.blk.AllocStack(v.Type())
				g.blk.Store(v, slot, storeQualifier(v.Type()))
				slots = append(slots, slot)
				v = slot
			}
			args = append(args, v)
		}
	}
	args = append(args, metas...)

	result := lowerType(types.Substitute(sig.Results, spec.Subst()))
	v := g.blk.Apply(g.blk.FunctionRef(callee), result, args...)
	// The storage the arguments travelled in, and what is in it, end with
	// the call: the callee borrowed them.
	for i := len(slots) - 1; i >= 0; i-- {
		if !slots[i].Type().Object().Trivial() {
			g.blk.DestroyAddr(slots[i])
		}
		g.blk.DeallocStack(slots[i])
	}
	g.destroyLater(v)
	return v
}

// declareImportedGeneric states the callee's lowered type: the
// declared parameters, a pointer per type parameter, and a result
// that is `@out` where it is one of them.
func (g *gen) declareImportedGeneric(f *sil.Func, sig *types.Signature) {
	ft := f.Type()
	ft.Convention = sil.Thin
	// Async is in the type, as for any declaration. See declareSignature.
	ft.Async = sig.Async
	for _, p := range sig.Params {
		t := lowerType(p.Type)
		if p.Ownership == types.InOut {
			ft.Params = append(ft.Params, sil.Param{Type: t.Address(), Convention: sil.ParamInout})
			continue
		}
		if isTypeParam(p.Type) {
			ft.Params = append(ft.Params,
				sil.Param{Type: t.Address(), Convention: sil.ParamInGuaranteed})
			continue
		}
		ft.Params = append(ft.Params,
			sil.Param{Type: t, Convention: paramConvention(p, t)})
	}
	for range sig.TypeParams {
		ft.Params = append(ft.Params,
			sil.Param{Type: sil.Object(sil.BuiltinRawPointer)})
	}
	if sig.Results == nil || isVoid(sig.Results) {
		return
	}
	t := lowerType(sig.Results)
	if isTypeParam(sig.Results) {
		f.SetResult(t, sil.ResultOut)
		return
	}
	f.SetResult(t, resultConvention(t))
}

// genericShapeIsPlain checks that the generic signature uses supported unconstrained parameter types.
func (g *gen) genericShapeIsPlain(e *ast.CallExpr, sig *types.Signature) bool {
	for _, tp := range sig.TypeParams {
		if tp != nil && len(tp.Constraints) > 0 {
			g.refuse(e, "a call to a generic constrained by '"+tp.Constraints[0].String()+
				"': the call carries a witness table for the conformance, which this "+
				"compiler does not name the way swiftc names one")
			return false
		}
	}
	for _, p := range sig.Params {
		if mentionsTypeParam(p.Type) && !isTypeParam(p.Type) {
			g.refuse(e, "a call whose parameter '"+p.Type.String()+"' is built out of a "+
				"type parameter rather than being one")
			return false
		}
	}
	if mentionsTypeParam(sig.Results) && !isTypeParam(sig.Results) {
		g.refuse(e, "a call whose result '"+sig.Results.String()+"' is built out of a "+
			"type parameter rather than being one")
		return false
	}
	return true
}

// isTypeParam reports whether a type is a generic parameter itself.
func isTypeParam(t types.Type) bool {
	_, ok := t.(*types.TypeParam)
	return ok
}

// mentionsTypeParam reports whether a type parameter appears anywhere
// in a type.
func mentionsTypeParam(t types.Type) bool {
	switch n := t.(type) {
	case nil:
		return false
	case *types.TypeParam, *types.Dependent:
		return true
	case *types.Optional:
		return mentionsTypeParam(n.Wrapped)
	case *types.Array:
		return mentionsTypeParam(n.Elem)
	case *types.Dictionary:
		return mentionsTypeParam(n.Key) || mentionsTypeParam(n.Value)
	case *types.Set:
		return mentionsTypeParam(n.Elem)
	case *types.Tuple:
		for _, e := range n.Elements {
			if mentionsTypeParam(e.Type) {
				return true
			}
		}
	case *types.GenericInstance:
		for _, a := range n.Args {
			if mentionsTypeParam(a) {
				return true
			}
		}
	}
	return false
}
