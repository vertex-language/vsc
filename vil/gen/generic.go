package gen

import (
	"strings"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Generics, by monomorphisation.
//
// A generic function is not lowered as itself. It is lowered once per
// set of type arguments some call inferred, with the body walked as
// though the parameters had been written out -- `identity<T>` called
// with an Int32 becomes a function taking an Int32 and returning one,
// and the T is gone by the time anything asks what a value's type is.
//
// This is not what SILGen does, and the departure is the same one
// for-in makes and for the same reason. SILGen emits the generic
// function once, with its type arguments passed at run time as
// metadata and its constrained operations dispatched through witness
// tables; that is what makes a Swift generic callable from a module
// that has never seen it. None of that machinery is written here.
// What swiftc's optimizer then does to the common case -- specialize
// per call site until the metadata is unnecessary -- is what this
// does to every case.
//
// # What it costs
//
// Generics stop at the module boundary. A specialization is not the
// symbol swiftc would call, so a generic function declared here
// cannot be called from Swift with a type argument this module never
// saw, and a generic function declared in Swift cannot be called from
// here at all -- it wants metadata and witness tables that are not
// being passed. Everything inside one module works; the interface
// writes nothing generic out.
//
// That is a real limitation and it is the reason witness tables are
// the next thing rather than an optional one.

// callGeneric lowers a call to a generic function by lowering the
// function for these type arguments and calling that.
func (g *gen) callGeneric(e *ast.CallExpr, sym *analyzer.FuncSymbol, spec analyzer.Specialization) *vil.Value {
	for _, a := range spec.Args {
		if a == nil {
			g.refuse(e, "a call whose type arguments could not be inferred")
			return nil
		}
	}
	subst := spec.Subst()
	sig, ok := types.Substitute(sym.Signature(), subst).(*types.Signature)
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

	var args []*vil.Value
	want := existentialParams(sig)
	if e.Args != nil {
		for i, a := range e.Args.Args {
			v := g.rvalue(a.X)
			if v == nil {
				return nil
			}
			args = append(args, g.boxArg(a.X, v, want, i))
		}
	}
	v := g.blk.Apply(ref, lowerType(sig.Results), args...)
	g.destroyLater(v)
	return v
}

// emitSpecialization lowers the generic function's body once for this
// substitution, if it has not been lowered for it already.
//
// The declaration is found from the symbol, so a generic function is
// lowered where it is first called rather than where it is written --
// which is the whole of monomorphisation: there is nothing to lower
// until something says what the types are.
func (g *gen) emitSpecialization(sym *analyzer.FuncSymbol, name string, subst map[*types.TypeParam]types.Type) error {
	if g.specialized[name] {
		return nil
	}
	if g.specialized == nil {
		g.specialized = map[string]bool{}
	}
	// Marked before the body is walked, so that a generic function
	// that calls itself at the same types stops rather than
	// instantiating for ever.
	g.specialized[name] = true

	decl, _ := sym.Decl().(*ast.FuncDecl)
	if decl == nil || decl.Body == nil {
		return errNoGenericBody(sym.Name())
	}

	outer := struct {
		fn      *vil.Func
		entry   bool
		blk     *vil.Block
		scopes  []*scope
		locals  map[analyzer.Symbol]*local
		loops   []loop
		pending string
		recv    types.Type
		self    *local
		subst   map[*types.TypeParam]types.Type
	}{g.fn, g.entry, g.blk, g.scopes, g.locals, g.loops, g.pending, g.recv, g.self, g.subst}
	defer func() {
		g.fn, g.entry, g.blk = outer.fn, outer.entry, outer.blk
		g.scopes, g.locals = outer.scopes, outer.locals
		g.loops, g.pending = outer.loops, outer.pending
		g.recv, g.self, g.subst = outer.recv, outer.self, outer.subst
	}()

	g.subst = subst
	g.functionNamed(decl, nil, name)
	return nil
}

// specializedSymbol names one instantiation.
//
// The substituted signature, so that a reader sees what the function
// actually takes, and a suffix so that it cannot collide with a
// function somebody wrote with that signature by hand. swiftc marks
// its own specializations the same way and for the same reason; the
// spelling differs because these are not swiftc's specializations and
// nothing outside this module resolves them.
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

type genericBodyError string

func (e genericBodyError) Error() string { return string(e) }

func errNoGenericBody(name string) error {
	return genericBodyError("cannot lower '" + name + "': a generic function with no body to specialize")
}

// witness is the method a concrete type provides for a requirement.
//
// Monomorphisation is what makes this a lookup rather than a runtime
// table. A generic body is lowered once per type argument, so by the
// time a constrained call is reached the type is known and the
// implementation can be named -- there is no metadata to pass and no
// table to index, because there is nothing left to decide.
//
// That is the whole of what a witness table would do here, and its
// whole limitation: it works because the type is known, and the type
// is known because the body was specialized. An existential -- a
// value whose type is not known until it arrives -- has none of that
// and is still refused.
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
	found, m := methodOn(recv, ref.Method.Name)
	if m == nil {
		return nil, false
	}
	return &analyzer.MethodRef{Recv: found, Method: m}, true
}

// methodOn is the method a concrete type declares under this name.
func methodOn(t types.Type, name string) (types.Type, *types.Method) {
	if t == nil {
		return nil, nil
	}
	if inst, ok := t.(*types.GenericInstance); ok {
		if _, m := methodOn(inst.Underlying(), name); m != nil {
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
		return methodOn(cl.Superclass, name)
	}
	return nil, nil
}
