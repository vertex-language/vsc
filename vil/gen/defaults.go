package gen

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Default arguments.
//
// A parameter with a default may be left out of a call, and the
// caller supplies the value: `defaulted(39)` where the declaration
// says `by k: Int32 = 3` passes 39 and 3. Swift evaluates the default
// at the call, which is why a client's object file references the
// function and nothing else -- there is no argument sitting in the
// callee waiting to be filled in.
//
// So a call with fewer arguments than parameters is not a call with
// fewer arguments. Passing what was written and stopping there leaves
// the callee reading a register nobody wrote, which is what this used
// to do: the VIL said `apply %0(%2)` against a type taking two, and
// what saved the program was a refusal further down rather than
// anything noticing.

// arguments is a call's values in parameter order, with the defaults
// filled in, and reports whether they could all be supplied.
//
// The arguments written are matched to parameters by label, which is
// what lets a defaulted parameter in the middle be left out. A
// parameter with no argument and no default is not this function's
// mistake to report -- the checker already did -- so it stops.
func (g *gen) arguments(e *ast.CallExpr, sig *types.Signature) ([]*vil.Value, bool) {
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	if sig == nil {
		return nil, false
	}
	// A variadic parameter collects whatever is left into an array,
	// which the caller builds. See variadicArguments.
	if i, ok := variadicIndex(sig); ok {
		return g.variadicArguments(e, args, sig, i)
	}
	// The common case, and the only one before defaults: one argument
	// per parameter, in order.
	if len(args) == len(sig.Params) {
		return g.argumentValues(args, sig), true
	}
	if len(args) > len(sig.Params) {
		return nil, false
	}

	want := existentialParams(sig)
	out := make([]*vil.Value, 0, len(sig.Params))
	next := 0
	for i, p := range sig.Params {
		if next < len(args) && g.argFits(args[next], p) {
			v := g.expr(args[next].X)
			if v == nil {
				return nil, false
			}
			out = append(out, g.boxArg(args[next].X, v, want, i))
			next++
			continue
		}
		def := g.info.Defaults[p]
		if !p.HasDefault || def == nil {
			g.refuse(e, "a call leaving out '"+p.Name+"', whose default this cannot reach")
			return nil, false
		}
		v := g.defaultValue(e, def)
		if v == nil {
			return nil, false
		}
		out = append(out, g.boxArg(def, v, want, i))
	}
	if next != len(args) {
		g.refuse(e, "a call whose arguments this could not match to parameters")
		return nil, false
	}
	return out, true
}

// argumentValues is the straightforward case: one argument per
// parameter, in order.
func (g *gen) argumentValues(args []*ast.CallArg, sig *types.Signature) []*vil.Value {
	want := existentialParams(sig)
	out := make([]*vil.Value, 0, len(args))
	for i, a := range args {
		v := g.expr(a.X)
		if v == nil {
			return nil
		}
		out = append(out, g.boxArg(a.X, v, want, i))
	}
	return out
}

// argFits reports whether an argument was written for this parameter,
// which is a question about the label and nothing else.
func (g *gen) argFits(a *ast.CallArg, p *types.Param) bool {
	written := ""
	if a.Label != nil {
		written = g.text(a.Label)
	}
	label := p.Label
	if label == "_" {
		label = ""
	}
	if a.Label == nil {
		return label == ""
	}
	return written == label
}

// defaultValue lowers a default at the call, which is where Swift
// evaluates one.
//
// Only a constant the checker folded. A default that computes
// something is a function of its own in swiftc -- a default argument
// generator, named after the declaration and the parameter's place --
// and calling one is a symbol this does not name yet.
func (g *gen) defaultValue(at ast.Node, e ast.Expr) *vil.Value {
	if v := g.constant(e); v != nil {
		return v
	}
	// A string literal is a call rather than a constant -- the
	// standard library's own initializer builds it -- so it is not
	// folded onto the expression the way a number is. See string.go.
	if lit, ok := e.(*ast.StringLit); ok {
		if v := g.stringLiteral(lit); v != nil {
			return v
		}
		return nil
	}
	g.errorAt(at, "cannot lower a default argument that is not a constant: Swift "+
		"computes one by calling a generator this does not name yet")
	return nil
}

// variadicIndex is where a signature's variadic parameter is, and
// whether it has one.
func variadicIndex(sig *types.Signature) (int, bool) {
	for i, p := range sig.Params {
		if p != nil && p.Variadic {
			return i, true
		}
	}
	return 0, false
}

// variadicArguments is a call's values where one parameter takes
// however many arguments are written for it.
//
// The array is the caller's to build: swiftc's own code for
// `total(40, 2)` allocates storage for two Int32s, stores them, and
// passes the array -- the callee's signature is
// `(@guaranteed Array<Int32>) -> Int32` and knows nothing about how
// many arguments were written. So this is makeArray over what was
// collected, and the same three instructions an array literal is.
//
// A variadic need not be last. `print(_ items: Any…, separator: String
// = " ", terminator: String = "\n")` has two parameters after it, and
// which arguments belong to which is decided by label: the list takes
// what is written without one, and a labelled argument belongs to the
// parameter of that name.
func (g *gen) variadicArguments(e *ast.CallExpr, args []*ast.CallArg,
	sig *types.Signature, vi int) ([]*vil.Value, bool) {
	want := existentialParams(sig)
	out := make([]*vil.Value, 0, len(sig.Params))

	// The parameters before the list take one argument each, in
	// order.
	next := 0
	for i := 0; i < vi; i++ {
		if next >= len(args) || !g.argFits(args[next], sig.Params[i]) {
			g.refuse(e, "a call whose arguments this could not match to parameters")
			return nil, false
		}
		v := g.expr(args[next].X)
		if v == nil {
			return nil, false
		}
		out = append(out, g.boxArg(args[next].X, v, want, i))
		next++
	}

	// The list takes every argument written without a label of its
	// own, which is what stops it before a `separator:`.
	last := sig.Params[vi]
	elems := make([]*vil.Value, 0, len(args)-next)
	for ; next < len(args); next++ {
		if !g.argFits(args[next], last) {
			break
		}
		v := g.rvalue(args[next].X)
		if v == nil {
			return nil, false
		}
		from := g.typeOf(args[next].X)
		elems = append(elems, g.existentialFor(args[next].X, v, from, last.Type))
	}
	list := g.makeArray(e, last.BodyType(), last.Type, elems)
	if list == nil {
		return nil, false
	}
	out = append(out, list)

	// What follows the list is matched by label, with a default for
	// anything left out.
	for i := vi + 1; i < len(sig.Params); i++ {
		p := sig.Params[i]
		if next < len(args) && g.argFits(args[next], p) {
			v := g.expr(args[next].X)
			if v == nil {
				return nil, false
			}
			out = append(out, g.boxArg(args[next].X, v, want, i))
			next++
			continue
		}
		def := g.info.Defaults[p]
		if !p.HasDefault || def == nil {
			g.refuse(e, "a call leaving out '"+p.Name+"', whose default this cannot reach")
			return nil, false
		}
		v := g.defaultValue(e, def)
		if v == nil {
			return nil, false
		}
		out = append(out, g.boxArg(def, v, want, i))
	}
	if next != len(args) {
		g.refuse(e, "a call whose arguments this could not match to parameters")
		return nil, false
	}
	return out, true
}
