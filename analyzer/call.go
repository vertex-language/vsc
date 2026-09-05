package analyzer

import (
	"fmt"
	"strconv"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// Calls: which declaration a call names, and what it produces.
//
// Three questions, in order. Which of the declarations that share the
// name is being called, if there is more than one. What its generic
// parameters stand for, if it has any. And whether each argument fits
// the parameter it lands on.
//
// A call this compiler cannot resolve is not an error — a name whose
// declaration lives in a library there is no reader for is simply not
// known — so what it does not understand it passes over in silence,
// having read the arguments, which are expressions whatever is done
// with them.

// resolveOverload picks the declaration a call names, where the name
// has more than one. It returns nil where there is nothing to choose
// — one declaration, or no single fit — and the caller goes on with
// what it had.
//
// The fit is the arguments: how many, and whether each is assignable
// to the parameter it lands on. That is enough for the calls a name
// is usually overloaded for, and short of what Swift does, which
// ranks the candidates rather than requiring exactly one to fit.
func (c *checker) resolveOverload(fun ast.Expr, args []*ast.CallArg, scope *Scope) *types.Signature {
	id, ok := fun.(*ast.IdentExpr)
	if !ok || id.Name == nil {
		return nil
	}
	sym, ok := c.info.Uses[id.Name].(*FuncSymbol)
	if !ok {
		return nil
	}
	candidates := sym.Overloads()
	if len(candidates) < 2 {
		return nil
	}

	quiet := len(c.info.Diagnostics)
	argTypes := make([]types.Type, len(args))
	for i, arg := range args {
		argTypes[i] = c.checkExpr(arg.X, nil, scope)
	}
	c.info.Diagnostics = c.info.Diagnostics[:quiet]

	var fits []*types.Signature
	for _, cand := range candidates {
		sig := cand.Signature()
		if sig == nil || len(sig.Params) != len(argTypes) {
			continue
		}
		ok := true
		for i, t := range argTypes {
			if !c.labelFits(args[i], sig.Params[i]) || !types.AssignableTo(t, sig.Params[i].Type) {
				ok = false
				break
			}
		}
		if ok {
			fits = append(fits, sig)
		}
	}
	if len(fits) == 1 {
		return fits[0]
	}
	return nil
}

// labelFits reports whether an argument's label is the one a
// parameter asks for. Two declarations of a name may differ in
// nothing else — `label(a:)` and `label(b:)` — so a call is not
// resolved without reading them.
func (c *checker) labelFits(arg *ast.CallArg, param *types.Param) bool {
	want := param.Label
	if want == "" {
		want = param.Name
	}
	if arg.Label == nil {
		return want == "" || want == "_"
	}
	return arg.Label.Text(c.file) == want
}

// inferInstance is the type an initializer call produces. For a
// generic type it is the instance the arguments imply: `Box(v: 3)`
// makes a Box<Int>, which the memberwise initializer's parameters —
// the stored properties, in order — are enough to work out.
func (c *checker) inferInstance(instance types.Type, call *ast.CallExpr, scope *Scope) types.Type {
	if call.Args == nil {
		return instance
	}
	fields := storedFieldsOf(instance)
	params := typeParamsOf(instance)

	// The arguments are read whatever comes of them: they are
	// expressions, and every expression in a program is checked even
	// where what it is passed to is not yet modelled.
	subst := make(map[*types.TypeParam]types.Type, len(params))
	for i, arg := range call.Args.Args {
		field := fieldFor(fields, arg, c.file, i)
		var want types.Type
		if field != nil && len(params) == 0 {
			want = field.Type
		}
		argType := c.checkExpr(arg.X, want, scope)
		if field != nil {
			types.Unify(field.Type, argType, subst)
		}
	}
	if len(params) == 0 || len(fields) == 0 {
		return instance
	}

	args := make([]types.Type, len(params))
	for i, p := range params {
		bound, ok := subst[p]
		if !ok {
			return instance // not every parameter was said; say nothing
		}
		args[i] = bound
	}
	return &types.GenericInstance{Base: instance, Args: args}
}

// fieldFor matches one argument of an initializer call to the stored
// property it initializes: by label where the call gives one, and by
// position otherwise.
func fieldFor(fields []*types.Field, arg *ast.CallArg, f *token.File, i int) *types.Field {
	if arg.Label != nil {
		name := arg.Label.Text(f)
		for _, field := range fields {
			if field.Name == name {
				return field
			}
		}
		return nil
	}
	if i < len(fields) {
		return fields[i]
	}
	return nil
}

// typeParamsOf is a nominal type's generic parameters.
func typeParamsOf(t types.Type) []*types.TypeParam {
	switch n := t.(type) {
	case *types.Struct:
		return n.TypeParams
	case *types.Class:
		return n.TypeParams
	case *types.Enum:
		return n.TypeParams
	}
	return nil
}

// storedFieldsOf is a nominal type's stored properties, which are the
// parameters of the initializer the compiler writes for it.
func storedFieldsOf(t types.Type) []*types.Field {
	switch n := t.(type) {
	case *types.Struct:
		return n.Fields
	case *types.Class:
		return n.Fields
	}
	return nil
}

// checkCallArguments checks a call's arguments against a signature
// and returns the signature the call was made with: the same one,
// unless it was generic, in which case it is the one its type
// parameters were inferred into.
func (c *checker) checkCallArguments(call *ast.CallExpr, sig *types.Signature, args []*ast.CallArg, scope *Scope) *types.Signature {
	sig = c.inferGenericCall(sig, args, scope)
	if sig.Params == nil {
		return sig
	}
	isVariadic := len(sig.Params) > 0 && sig.Params[len(sig.Params)-1].Variadic
	least := sig.Minimum()
	if isVariadic && least > 0 {
		least--
	}

	if len(args) < least || (!isVariadic && len(args) > len(sig.Params)) {
		c.errorf(call.Pos(), "incorrect argument count: expected %s, got %d",
			arity(least, len(sig.Params), isVariadic), len(args))
		return sig
	}

	// Which parameter each argument is for.
	//
	// As many arguments as parameters is the ordinary case and they
	// pair up in order. Fewer means a parameter with a default was
	// left out, and then the labels are what says which — a caller
	// may skip any defaulted parameter, not only the last, so
	// position alone cannot answer it.
	params := sig.Params
	if len(args) != len(params) && !isVariadic {
		params = c.matchByLabel(call, sig.Params, args)
	}

	for i, arg := range args {
		var param *types.Param
		switch {
		case i < len(params):
			param = params[i]
		case isVariadic:
			param = sig.Params[len(sig.Params)-1]
		}
		if param == nil {
			break
		}

		// Check label matching
		if param.Label != "" && param.Label != "_" {
			if arg.Label == nil || arg.Label.Text(c.file) != param.Label {
				var actualLabel string
				if arg.Label != nil {
					actualLabel = arg.Label.Text(c.file)
				}
				c.errorf(arg.Pos(), "incorrect argument label (have '%s:', expected '%s:')", actualLabel, param.Label)
			}
		}

		argType := c.checkExpr(arg.X, param.Type, scope)
		if !types.AssignableTo(argType, param.Type) {
			c.typeErrorf(arg.Pos(), "cannot convert value of type '%s' to expected argument type '%s'", argType, param.Type)
		}
	}
	return sig
}

// matchByLabel pairs a short argument list with the parameters it
// named, skipping the ones that were left out.
//
// A parameter may be skipped only where it has a default; one without
// is a missing argument, and saying which is missing is the whole
// value of matching rather than counting. The result is one parameter
// per argument, in the arguments' order, so the caller's loop is
// unchanged.
func (c *checker) matchByLabel(call *ast.CallExpr, params []*types.Param, args []*ast.CallArg) []*types.Param {
	out := make([]*types.Param, 0, len(args))
	pi := 0
	for _, arg := range args {
		label := ""
		if arg.Label != nil {
			label = arg.Label.Text(c.file)
		}
		// Walk past the parameters this argument is not for. Each one
		// skipped has to be able to supply itself.
		for pi < len(params) && !labels(params[pi], label) && params[pi].HasDefault {
			pi++
		}
		if pi >= len(params) {
			break
		}
		if !labels(params[pi], label) {
			// The argument is not for this parameter, and this
			// parameter has no default to fall back on — so it was
			// left out. Naming it is the useful half of the message,
			// and reporting the label mismatch as well would be two
			// complaints about one mistake.
			c.errorf(call.Pos(), "missing argument for parameter '%s'", paramName(params[pi]))
			return out
		}
		out = append(out, params[pi])
		pi++
	}
	// Anything left unmatched has to have a default of its own.
	for ; pi < len(params); pi++ {
		if !params[pi].HasDefault {
			c.errorf(call.Pos(), "missing argument for parameter '%s'", paramName(params[pi]))
		}
	}
	return out
}

// labels reports whether a parameter answers to this argument label.
func labels(p *types.Param, label string) bool {
	if p.Label == "" || p.Label == "_" {
		return label == ""
	}
	return p.Label == label
}

// paramName is what to call a parameter in a message: its label where
// it has one, since that is what the caller would have written.
func paramName(p *types.Param) string {
	if p.Label != "" && p.Label != "_" {
		return p.Label
	}
	if p.Name != "" {
		return p.Name
	}
	return "_"
}

// arity is how many arguments a signature takes, said the way the
// signature admits: a number, a range, or a floor.
func arity(least, most int, variadic bool) string {
	switch {
	case variadic:
		return fmt.Sprintf("at least %d", least)
	case least == most:
		return strconv.Itoa(most)
	default:
		return fmt.Sprintf("%d to %d", least, most)
	}
}

// inferGenericCall binds a generic signature's type parameters to the
// argument types and returns the signature that results. `identity(3)`
// is a call to `(Int) -> Int`, and that is the signature the rest of
// the check should see.
//
// An argument that binds nothing leaves its parameter as it was, and
// the call is checked against a type parameter it cannot satisfy —
// which is the honest outcome: this compiler has no constraint solver
// yet, and a call it cannot infer is a call it does not understand.
func (c *checker) inferGenericCall(sig *types.Signature, args []*ast.CallArg, scope *Scope) *types.Signature {
	if len(sig.TypeParams) == 0 || len(args) == 0 {
		return sig
	}
	// The arguments are read twice: once to infer, once to check
	// against what was inferred. Only the second reading reports —
	// a speculative pass that says nothing is the same rule the
	// parser follows when it tries a production and backs out.
	subst := make(map[*types.TypeParam]types.Type, len(sig.TypeParams))
	quiet := len(c.info.Diagnostics)
	for i, arg := range args {
		if i >= len(sig.Params) {
			break
		}
		types.Unify(sig.Params[i].Type, c.checkExpr(arg.X, nil, scope), subst)
	}
	c.info.Diagnostics = c.info.Diagnostics[:quiet]
	if len(subst) == 0 {
		return sig
	}
	if out, ok := types.Substitute(sig, subst).(*types.Signature); ok {
		return out
	}
	return sig
}
