package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Calling a generic function another module declared.
//
// A generic declared here is monomorphised: the body is lowered once
// per set of type arguments, and by the time anything asks what a
// value's type is the parameter is gone. See generic.go. That needs a
// body, and an imported generic has none -- what the library shipped
// is one function that works for every type.
//
// So it is called as it stands, which means telling it what the types
// are. Swift's answer is metadata: the arguments the source wrote,
// then a pointer per type parameter. swiftc's own code for `tag(x)`
// where x is an Int32 puts the address of x in x0 and Int32's
// metadata in x1.
//
// The address of x, not x. A generic parameter is `@in_guaranteed`:
// the callee does not know how big T is, so it is handed somewhere to
// look rather than a register to read. A result that is a type
// parameter comes back the same way, through storage the caller set
// aside -- `@out`, which is x8 whatever the size.

// callImportedGeneric lowers a call to a generic function this module
// did not declare.
func (g *gen) callImportedGeneric(e *ast.CallExpr, sym *analyzer.FuncSymbol,
	spec analyzer.Specialization) *vil.Value {
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
	metas := make([]*vil.Value, 0, len(sig.TypeParams))
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
	callee := g.m.Func(name).SetSourceName(sym.Name())
	if g.needsType(callee) {
		callee.SetLinkage(vil.PublicExternal)
		g.declareImportedGeneric(callee, sig)
	}

	// The arguments the source wrote. One whose parameter is a type
	// parameter goes into storage and travels as its address.
	var args []*vil.Value
	if e.Args != nil {
		for i, a := range e.Args.Args {
			v := g.rvalue(a.X)
			if v == nil {
				return nil
			}
			if i < len(sig.Params) && isTypeParam(sig.Params[i].Type) {
				slot := g.blk.AllocStack(v.Type())
				g.blk.Store(v, slot, storeQualifier(v.Type()))
				v = slot
			}
			args = append(args, v)
		}
	}
	args = append(args, metas...)

	result := lowerType(types.Substitute(sig.Results, spec.Subst()))
	v := g.blk.Apply(g.blk.FunctionRef(callee), result, args...)
	g.destroyLater(v)
	return v
}

// declareImportedGeneric states the callee's lowered type: the
// declared parameters, a pointer per type parameter, and a result
// that is `@out` where it is one of them.
func (g *gen) declareImportedGeneric(f *vil.Func, sig *types.Signature) {
	ft := f.Type()
	ft.Convention = vil.Thin
	for _, p := range sig.Params {
		t := lowerType(p.Type)
		if isTypeParam(p.Type) {
			ft.Params = append(ft.Params,
				vil.Param{Type: t.Address(), Convention: vil.ParamInGuaranteed})
			continue
		}
		ft.Params = append(ft.Params,
			vil.Param{Type: t, Convention: paramConvention(p, t)})
	}
	for range sig.TypeParams {
		ft.Params = append(ft.Params,
			vil.Param{Type: vil.Object(vil.BuiltinRawPointer)})
	}
	if sig.Results == nil || isVoid(sig.Results) {
		return
	}
	t := lowerType(sig.Results)
	if isTypeParam(sig.Results) {
		f.SetResult(t, vil.ResultOut)
		return
	}
	f.SetResult(t, resultConvention(t))
}

// genericShapeIsPlain refuses the generic signatures this does not
// answer for, by name rather than by producing a call that reads the
// wrong registers.
//
// Two of them. A constraint means the call also carries a witness
// table for each conformance, which is a table this compiler does not
// name swiftc's way. And a type that merely mentions a parameter --
// `[T]`, `T?` -- is passed by whatever rule its own type follows,
// which is a second set of conventions rather than the one below.
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
