package gen

import (
	"strconv"
	"strings"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
)

// Generic functions are lowered by monomorphisation: specialized for concrete type arguments at each call site.

// callGeneric lowers a call to a generic function by lowering the
// function for these type arguments and calling that.
// specializedValue is a generic function, specialized, as a function
// value: `<` on tuples handed to sorted(by:).
func (g *gen) specializedValue(at ast.Node, sym *analyzer.FuncSymbol, spec analyzer.Specialization) *sil.Value {
	if len(g.subst) > 0 {
		args := make([]types.Type, len(spec.Args))
		for i, a := range spec.Args {
			args[i] = types.Substitute(a, g.subst)
		}
		spec = analyzer.Specialization{Params: spec.Params, Args: args}
	}
	subst := spec.Subst()
	sig, ok := types.Substitute(sym.Signature(), subst).(*types.Signature)
	if !ok {
		g.refuse(at, "a generic signature this compiler cannot substitute")
		return nil
	}
	name := g.specializedSymbol(sym, sig, spec)
	if name == "" {
		g.refuse(at, "a generic function this compiler cannot name")
		return nil
	}
	if err := g.emitSpecialization(sym, name, subst); err != nil {
		g.errorAt(at, err.Error())
		return nil
	}
	callee := g.m.Func(name).SetSourceName(sym.Name())
	if g.needsType(callee) {
		g.declareSignature(callee, sig)
	}
	v := g.blk.ThinToThickFunction(g.blk.FunctionRef(callee), lowerType(sig))
	g.destroyLater(v)
	return v
}

func (g *gen) callGeneric(e *ast.CallExpr, sym *analyzer.FuncSymbol, spec analyzer.Specialization, optional bool) *sil.Value {
	for _, a := range spec.Args {
		if a == nil {
			g.refuse(e, "a call whose type arguments could not be inferred")
			return nil
		}
	}
	// A call made inside a specialization was checked against the
	// enclosing function's own type parameters -- `ReadToEnd(&r)` inside
	// `ReadText<R>` is ReadToEnd<R> -- which are this specialization's
	// types now.
	if len(g.subst) > 0 {
		args := make([]types.Type, len(spec.Args))
		for i, a := range spec.Args {
			args[i] = types.Substitute(a, g.subst)
		}
		spec = analyzer.Specialization{Params: spec.Params, Args: args}
	}
	subst := spec.Subst()
	// An associated type of a built-in the specialization is for --
	// [A.Element] with A a String -- is what core's extension names it.
	sig, ok := g.builtinDependents(types.Substitute(sym.Signature(), subst)).(*types.Signature)
	if !ok {
		g.refuse(e, "a generic signature this compiler cannot substitute")
		return nil
	}

	name := g.specializedSymbol(sym, sig, spec)
	if name == "" {
		g.refuse(e, "a generic function this compiler cannot name")
		return nil
	}
	if err := g.emitSpecialization(sym, name, subst); err != nil {
		g.errorAt(e, err.Error())
		return nil
	}

	callee := g.m.Func(name).SetSourceName(sym.Name())
	if g.needsType(callee) {
		g.declareSignature(callee, sig)
	}
	ref := g.blk.FunctionRef(callee)

	// As an ordinary call's arguments are, defaults included: a
	// parameter left out -- `ReadToEnd(&r)` and its `limit:` -- is given
	// its default, found through the substituted parameter's origin.
	args, ok := g.arguments(e, sig)
	if !ok {
		return nil
	}
	// A throwing specialization is try_applied as any throwing call is,
	// so what it throws is caught, made optional or raised (callFuncTry).
	if sig.Throws {
		if pendingOptional, pendingTrap := g.tryOn(e); pendingOptional || pendingTrap {
			optional = optional || pendingOptional
			g.tryBang = pendingTrap
		}
		if sig.Rethrows && !optional && !g.argumentThrows(e) {
			g.tryBang = true
		}
		return g.tryApply(e, ref, args, sig.Results, optional, false)
	}
	v := g.blk.Apply(ref, lowerType(sig.Results), args...)
	g.destroyLater(v)
	return v
}

// emitSpecialization lowers the generic function's body for the given type substitution.
func (g *gen) emitSpecialization(sym *analyzer.FuncSymbol, name string, subst map[*types.TypeParam]types.Type) error {
	if g.specialized[name] {
		return nil
	}
	// Another file of the module may have lowered it already: each file
	// has a gen of its own, and the module one function of the name.
	if existing := g.m.Lookup(name); existing != nil && !existing.IsDeclaration() {
		return nil
	}
	if g.specialized == nil {
		g.specialized = map[string]bool{}
	}
	g.specialized[name] = true

	decl, _ := sym.Decl().(*ast.FuncDecl)
	if decl == nil || decl.Body == nil {
		return errNoGenericBody(sym.Name())
	}

	outer := struct {
		fn      *sil.Func
		entry   bool
		blk     *sil.Block
		scopes  []*scope
		locals  map[analyzer.Symbol]*local
		loops   []loop
		pending string
		recv    types.Type
		self    *local
		subst   map[*types.TypeParam]types.Type
		throws  bool
		catches []catchTarget
		tryBang bool
		tryCall *ast.CallExpr
	}{g.fn, g.entry, g.blk, g.scopes, g.locals, g.loops, g.pending, g.recv, g.self, g.subst,
		g.throws, g.catches, g.tryBang, g.tryCall}
	// The instantiation is lowered in the middle of its caller, which may
	// be inside a `do`: what the caller's throws are caught by is the
	// caller's, and has to be there again when its lowering resumes.
	defer func() {
		g.fn, g.entry, g.blk = outer.fn, outer.entry, outer.blk
		g.scopes, g.locals = outer.scopes, outer.locals
		g.loops, g.pending = outer.loops, outer.pending
		g.recv, g.self, g.subst = outer.recv, outer.self, outer.subst
		g.throws, g.catches, g.tryBang, g.tryCall = outer.throws, outer.catches, outer.tryBang, outer.tryCall
	}()

	// The body is read in the file it was written in, which is not
	// always the file of the call that asked for it.
	prevFile := g.file
	defer func() { g.file = prevFile }()
	if f := g.fileOf(decl); f != nil {
		g.file = f
	}
	g.subst = subst
	defer g.asSpecialization()()
	g.functionNamed(decl, nil, name)
	return nil
}

// asSpecialization marks what is lowered from here on as a specialization,
// until the function it returns is called.
func (g *gen) asSpecialization() func() {
	was := g.specializing
	g.specializing = true
	return func() { g.specializing = was }
}

// specializedSymbol generates a unique mangled name for a specialized function instantiation.
func (g *gen) specializedSymbol(sym *analyzer.FuncSymbol, sig *types.Signature, spec analyzer.Specialization) string {
	d := mangle.Decl{
		Module:    g.moduleOf(sym),
		Name:      sym.Name(),
		Signature: sig,
		ModuleOf:  g.moduleOfType,
	}
	name, err := mangle.Function(d)
	if err != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(name)
	b.WriteString("Tv")
	for _, a := range spec.Args {
		b.WriteString(identifierSafe(a.String()))
	}
	return b.String()
}

// identifierSafe keeps a type's spelling to what a symbol admits.
func identifierSafe(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// spellingHash is a short FNV-1a hash of s, for a name that identifierSafe
// alone would make ambiguous.
func spellingHash(s string) string {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return strconv.FormatUint(h, 16)
}

type genericBodyError string

func (e genericBodyError) Error() string { return string(e) }

func errNoGenericBody(name string) error {
	return genericBodyError("cannot lower '" + name + "': a generic function with no body to specialize")
}

// witness returns the concrete method fulfilling a protocol requirement on recv.
func (g *gen) witness(ref *analyzer.MethodRef, recv types.Type) (*analyzer.MethodRef, bool) {
	if ref == nil || ref.Method == nil || recv == nil {
		return nil, false
	}
	// Only when the checker resolved against something abstract. A
	// call already on a concrete type is the one it resolved.
	switch ref.Recv.(type) {
	case *types.Protocol, *types.TypeParam:
	default:
		return nil, false
	}
	found, m := g.methodOn(recv, ref.Method.Name)
	// Of several of the name, the one of the requirement's labels:
	// index(after:), not index(before:).
	if m != nil && ref.Method.Sig != nil {
		if f2, m2 := g.methodLabelled(recv, ref.Method.Name, ref.Method.Sig); m2 != nil {
			found, m = f2, m2
		} else {
			found, m = nil, nil
		}
	}
	if m == nil {
		// A requirement the type does not implement is the default an
		// extension of the protocol gives.
		if p, ok := ref.Recv.(*types.Protocol); ok && ref.Method.Sig != nil && !isExistentialType(recv) {
			n := len(ref.Method.Sig.Params)
			for _, q := range g.protocolClosure(append([]*types.Protocol{p}, g.conformancesOf(recv)...)) {
				em := q.OwnExtensionMethod(ref.Method.Name, ref.Method.IsStatic, func(em *types.Method) bool {
					if len(em.Sig.Params) != n || !g.info.SelfConditionsMet(em, recv) {
						return false
					}
					for i, prm := range em.Sig.Params {
						if prm.Label != ref.Method.Sig.Params[i].Label {
							return false
						}
					}
					return true
				})
				if em != nil {
					return &analyzer.MethodRef{Recv: q, Method: em}, true
				}
			}
		}
		return nil, false
	}
	return &analyzer.MethodRef{Recv: found, Method: m}, true
}

// methodOn is the method a concrete type declares under this name.
func (g *gen) methodOn(t types.Type, name string) (types.Type, *types.Method) {
	if t == nil {
		return nil, nil
	}
	// A method an extension gives a built-in type is declared on the type
	// as the extension sees it: Int, or [Element].
	if b := builtinOf(g.info, t); b != nil {
		for _, m := range b.Methods {
			if m != nil && m.Name == name {
				return b.Type, m
			}
		}
		return nil, nil
	}
	if inst, ok := t.(*types.GenericInstance); ok {
		if _, m := g.methodOn(inst.Underlying(), name); m != nil {
			return t, m
		}
		return nil, nil
	}
	var methods []*types.Method
	switch b := t.Underlying().(type) {
	case *types.Struct:
		methods = b.Methods
	case *types.Class:
		methods = b.Methods
	case *types.Enum:
		methods = b.Methods
	default:
		return nil, nil
	}
	for _, m := range methods {
		if m != nil && m.Name == name {
			return t, m
		}
	}
	if cl, ok := t.Underlying().(*types.Class); ok && cl.Superclass != nil {
		return g.methodOn(cl.Superclass, name)
	}
	return nil, nil
}

// requirementOperator lowers an operator a generic parameter's constraint
// promises, in a specialization, as the concrete type's own: a builtin on
// a number or a String, the static method a struct or class declares or
// has derived, a comparison of tags on an enum of cases alone.
func (g *gen) requirementOperator(at ast.Expr, ref *analyzer.MethodRef, xs []ast.Expr, vals []*sil.Value) *sil.Value {
	op := ref.Method.Name
	concrete := g.typeOf(xs[0])
	results := g.substituted(ref.Method.Sig.Results)
	if concrete == nil || len(vals) != 2 {
		g.refuse(at, "an operator of a generic parameter this compiler cannot resolve")
		return nil
	}
	if _, basic := concrete.Underlying().(*types.Basic); basic {
		if v := g.operate(at, op, concrete, results, vals[0], vals[1]); v != nil {
			return v
		}
	}
	want := &types.Signature{Params: []*types.Param{{Label: "_", Type: concrete}, {Label: "_", Type: concrete}}, Results: results}
	recv, methods := staticMethodsNamed(concrete, op)
	if op == "==" && len(methods) == 0 {
		recv, methods = staticMethodsNamed(concrete, analyzer.DerivedStructEquals)
	}
	if op == "==" && len(methods) == 0 {
		recv, methods = staticMethodsNamed(concrete, analyzer.DerivedEnumEquals)
	}
	for _, m := range methods {
		if m.Sig != nil && len(m.Sig.Params) == 2 &&
			types.Identical(m.Sig.Params[0].Type, want.Params[0].Type) &&
			types.Identical(m.Sig.Params[1].Type, want.Params[1].Type) {
			return g.operatorApply(at, &analyzer.MethodRef{Recv: recv, Method: m}, nil, xs, vals)
		}
	}
	if en, ok := enumFor(concrete); ok && op == "==" && !hasPayloadCase(en) {
		raw := g.blk.Builtin("cmp_eq_"+enumMachine(en), sil.Object(sil.BuiltinInt1), vals[0], vals[1])
		return g.blk.Struct(lowerType(results), raw)
	}
	g.refuse(at, "'"+op+"' on "+typeNameOf(concrete)+" in a generic function")
	return nil
}

// methodLabelled is the method of t of the name whose parameters have
// want's labels, where t declares one.
func (g *gen) methodLabelled(t types.Type, name string, want *types.Signature) (types.Type, *types.Method) {
	var methods []*types.Method
	owner := t
	if b := builtinOf(g.info, t); b != nil {
		methods, owner = b.Methods, b.Type
	} else {
		base := t
		if inst, ok := t.(*types.GenericInstance); ok {
			base = inst.Underlying()
		}
		for cur := base; cur != nil; {
			var next types.Type
			switch u := cur.Underlying().(type) {
			case *types.Struct:
				methods = append(methods, u.Methods...)
			case *types.Enum:
				methods = append(methods, u.Methods...)
			case *types.Class:
				methods = append(methods, u.Methods...)
				next = u.Superclass
			}
			cur = next
		}
	}
	for _, m := range methods {
		if m == nil || m.Name != name || m.Sig == nil || len(m.Sig.Params) != len(want.Params) {
			continue
		}
		same := true
		for i, p := range m.Sig.Params {
			if p.Label != want.Params[i].Label {
				same = false
			}
		}
		if same {
			return owner, m
		}
	}
	return nil, nil
}
