package analyzer

import (
	"fmt"
	"strconv"
	"strings"

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

	// Strict first, then lax. A label that was written the way the
	// declaration asks for it resolves exactly as it always did, so
	// no program that compiled before can reach the second pass and
	// change meaning. Only a call the strict rule rejects -- one that
	// left a label out, or wrote one where the declaration says `_`
	// -- is tried again with labels optional.
	fits := c.candidatesFitting(candidates, args, argTypes, c.labelFits)
	if len(fits) != 1 {
		lax := c.candidatesFitting(candidates, args, argTypes, c.labelFitsLax)
		// More than one only because the labels were left off: the
		// labels are what told these declarations apart, and without
		// them the call names all of them. Saying which is the
		// caller's business, and guessing is how a program calls a
		// function nobody asked for.
		if len(lax) > 1 && len(fits) == 0 {
			c.errorf(fun.Pos(), "ambiguous use of '%s': %s",
				id.Name.Text(c.file), candidateLabels(lax))
			return nil
		}
		fits = lax
	}
	if len(fits) != 1 {
		return nil
	}
	// The winner, not just its signature. Everything downstream reads
	// the name's meaning out of Uses -- lowering mangles a call from
	// the symbol it finds there -- and leaving the first declaration
	// in place meant every call to an overloaded name reached the
	// first one, whichever the arguments actually chose. It compiled
	// and it ran and it called the wrong function.
	c.info.Uses[id.Name] = fits[0]
	return fits[0].Signature()
}

// candidatesFitting is every candidate the arguments fit, with fitsLabel
// deciding how strictly a label is read.
func (c *checker) candidatesFitting(candidates []*FuncSymbol, args []*ast.CallArg,
	argTypes []types.Type, fitsLabel func(*ast.CallArg, *types.Param) bool) []*FuncSymbol {
	var fits []*FuncSymbol
	for _, cand := range candidates {
		sig := cand.Signature()
		if sig == nil || len(sig.Params) != len(argTypes) {
			continue
		}
		ok := true
		for i, t := range argTypes {
			if !fitsLabel(args[i], sig.Params[i]) {
				ok = false
				break
			}
			if types.AssignableTo(t, sig.Params[i].Type) {
				continue
			}
			// A literal argument was checked with no context, so it
			// came out as the default -- an Int -- and is not
			// assignable to an Int32 parameter it would have fitted
			// perfectly well. It has no type of its own to insist on,
			// so it fits any numeric parameter, and checkCallArguments
			// re-reads it as that parameter's type once a candidate is
			// chosen. Without this, `f(2, 3)` never reached the
			// two-parameter f and was reported against the
			// one-parameter one as the wrong argument count.
			if c.isLiteralTree(args[i].X) && isNumericType(sig.Params[i].Type) {
				continue
			}
			ok = false
			break
		}
		if ok {
			fits = append(fits, cand)
		}
	}
	return fits
}

// candidateLabels spells the candidates for an ambiguity message, as
// the labels that would have told them apart.
func candidateLabels(fits []*FuncSymbol) string {
	var sb strings.Builder
	for i, f := range fits {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("'")
		sig := f.Signature()
		if sig == nil {
			sb.WriteString("?")
		} else {
			for _, p := range sig.Params {
				sb.WriteString(paramName(p))
				sb.WriteString(":")
			}
		}
		sb.WriteString("'")
	}
	return sb.String()
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

// labelFitsLax is labelFits with the label optional, which is what
// Vertex allows: a label may be left off, and one may be written
// where the declaration says `_`. Labels are carried for
// compatibility rather than as grammar, so both spellings reach the
// same parameter.
//
// A written label still has to name this parameter -- by its label or
// by the name its body uses -- because a label that names nothing is
// a mistake rather than a spelling.
func (c *checker) labelFitsLax(arg *ast.CallArg, param *types.Param) bool {
	if arg.Label == nil {
		return true
	}
	written := arg.Label.Text(c.file)
	return written == param.Label || written == param.Name
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
	sig = c.inferGenericCall(call, sig, args, scope)
	if sig.Params == nil {
		return sig
	}
	vi, isVariadic := variadicIndex(sig)
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
	switch {
	case isVariadic:
		params = variadicParams(sig, vi, args, c.file)
	case len(args) != len(params):
		params = c.matchByLabel(call, sig.Params, args)
	}

	for i, arg := range args {
		var param *types.Param
		if i < len(params) {
			param = params[i]
		}
		if param == nil {
			break
		}

		// A label may be left off, and one may be written where the
		// declaration says `_`: labels are carried for compatibility
		// rather than as grammar, so both spellings reach the same
		// parameter. What is still wrong is a label naming no
		// parameter here, which is a mistake and not a spelling.
		//
		// This is the path where the argument count matches the
		// parameter count, so every argument is positional and a
		// label decorates rather than decides. Where that is not true
		// -- a short list skipping defaults, or a variadic -- the
		// label is what says which parameter is meant, and
		// matchByLabel and variadicParams still require it.
		if arg.Label != nil && !c.labelFitsLax(arg, param) {
			c.errorf(arg.Pos(), "incorrect argument label (have '%s:', expected '%s:')",
				arg.Label.Text(c.file), paramName(param))
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
func (c *checker) inferGenericCall(e *ast.CallExpr, sig *types.Signature, args []*ast.CallArg, scope *Scope) *types.Signature {
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
	// A type argument has to satisfy what the parameter promised.
	// Nothing checked, so a type with none of the requirements could
	// be passed to a constrained generic and the mistake surfaced --
	// if at all -- as a call to a method the type does not have.
	c.checkConstraints(e, sig.TypeParams, subst)

	// What each parameter became, in the order the declaration wrote
	// them. Lowering makes one copy of the body per set of these, and
	// needs them rather than the substituted signature: it walks the
	// body substituting as it goes.
	if e != nil {
		spec := Specialization{Params: sig.TypeParams}
		for _, p := range sig.TypeParams {
			spec.Args = append(spec.Args, subst[p])
		}
		c.info.Specializations[e] = spec
	}
	if out, ok := types.Substitute(sig, subst).(*types.Signature); ok {
		return out
	}
	return sig
}

// isNumericType reports whether a literal could take this type.
func isNumericType(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsNumeric != 0
}

// checkConstraints reports a type argument that does not satisfy its
// parameter's constraints.
//
// The message names the function, the type and the protocol, which is
// what swiftc's does: the three things needed to see whether the
// declaration or the call is the one that is wrong.
func (c *checker) checkConstraints(e *ast.CallExpr, params []*types.TypeParam, subst map[*types.TypeParam]types.Type) {
	if e == nil {
		return
	}
	for _, p := range params {
		arg, ok := subst[p]
		if !ok || arg == nil {
			continue
		}
		for _, con := range p.Constraints {
			proto, ok := protocolOf(con)
			if !ok {
				// A constraint this compiler does not model as a
				// protocol -- a superclass bound, a where clause --
				// is not checked rather than wrongly refused.
				continue
			}
			if types.ConformsTo(arg, proto) {
				continue
			}
			c.typeErrorf(e.Pos(), "%s requires that '%s' conform to '%s'",
				calleeDescription(e, c), arg, proto.Name)
		}
	}
}

// protocolOf is the protocol a constraint names.
func protocolOf(t types.Type) (*types.Protocol, bool) {
	if t == nil {
		return nil, false
	}
	if p, ok := t.(*types.Protocol); ok {
		return p, true
	}
	p, ok := t.Underlying().(*types.Protocol)
	return p, ok
}

// calleeDescription names the function a call is to, for a message
// about its constraints.
func calleeDescription(e *ast.CallExpr, c *checker) string {
	if id, ok := e.Fun.(*ast.IdentExpr); ok && id.Name != nil {
		return "global function '" + id.Name.Text(c.file) + "'"
	}
	return "this call"
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

// variadicParams is the parameter each argument of a call to a
// variadic function is for.
//
// A variadic need not be last: `print(_ items: Any…, separator:
// String = " ")` has one after it, and which arguments belong to
// which is decided by label. The parameters before the list take one
// argument each; the list takes every argument written without a
// label; a labelled argument after that belongs to the parameter of
// that name.
func variadicParams(sig *types.Signature, vi int, args []*ast.CallArg,
	file *token.File) []*types.Param {
	out := make([]*types.Param, len(args))
	next := 0
	for i := 0; i < vi && next < len(args); i++ {
		out[next] = sig.Params[i]
		next++
	}
	list := sig.Params[vi]
	for ; next < len(args); next++ {
		if !argLabelFits(args[next], list, file) {
			break
		}
		out[next] = list
	}
	for i := vi + 1; i < len(sig.Params) && next < len(args); i++ {
		if !argLabelFits(args[next], sig.Params[i], file) {
			continue
		}
		out[next] = sig.Params[i]
		next++
	}
	return out
}

// argLabelFits reports whether an argument was written for a
// parameter, which is a question about the label and nothing else.
func argLabelFits(a *ast.CallArg, p *types.Param, file *token.File) bool {
	label := p.Label
	if label == "_" {
		label = ""
	}
	if a.Label == nil {
		return label == ""
	}
	return a.Label.Text(file) == label
}
