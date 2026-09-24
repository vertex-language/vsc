package analyzer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// resolveOverload selects the matching overload among multiple candidates.
// Returns nil if resolution fails or is unambiguous without selection.
func (c *checker) resolveOverload(fun ast.Expr, args []*ast.CallArg, expected types.Type, scope *Scope) *types.Signature {
	// A function named alone, or through its module: `tcp.Listen`.
	var name *ast.Ident
	switch f := fun.(type) {
	case *ast.IdentExpr:
		name = f.Name
	case *ast.MemberExpr:
		name = f.Name
	}
	if name == nil {
		return nil
	}
	sym, ok := c.info.Uses[name].(*FuncSymbol)
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

	// Try strict label matching first, falling back to lax matching if needed.
	// Labels decide first. Only where none fits by its labels are they
	// relaxed: relaxing them where several fit let `Sleep(until:)` in
	// beside `Sleep(_:)` and its async twin, and then nothing was chosen.
	fits := c.candidatesFitting(candidates, args, argTypes, c.labelFits)
	if len(fits) == 0 {
		lax := c.candidatesFitting(candidates, args, argTypes, c.labelFitsLax)
		if len(lax) > 1 {
			c.errorf(fun.Pos(), "ambiguous use of '%s': %s",
				name.Text(c.file), candidateLabels(lax))
			return nil
		}
		fits = lax
	}
	fits = c.byContext(fits)
	fits = byResult(fits, expected)
	if len(fits) > 1 {
		var sigs []*types.Signature
		for _, f := range fits {
			sigs = append(sigs, f.Signature())
		}
		if keep := c.byLiteralDefaults(sigs, args); len(keep) > 0 {
			var narrowed []*FuncSymbol
			for _, i := range keep {
				narrowed = append(narrowed, fits[i])
			}
			fits = narrowed
		}
	}
	if len(fits) != 1 {
		return nil
	}
	c.info.Uses[name] = fits[0]
	return fits[0].Signature()
}

// byResult is which of several functions that fit the arguments return
// what the call is wanted to be -- `let s: String = make()` beside an
// Int-returning make -- the one returning exactly that first; all of them
// where nothing is wanted or none returns it.
func byResult(fits []*FuncSymbol, expected types.Type) []*FuncSymbol {
	if len(fits) < 2 || expected == nil {
		return fits
	}
	for _, exact := range []bool{true, false} {
		var suited []*FuncSymbol
		for _, f := range fits {
			sig := f.Signature()
			if sig == nil || sig.Results == nil {
				continue
			}
			if exact && types.Identical(sig.Results, expected) || !exact && types.AssignableTo(sig.Results, expected) {
				suited = append(suited, f)
			}
		}
		if len(suited) > 0 {
			return suited
		}
	}
	return fits
}

// meetsConstraints reports whether t conforms to every protocol the
// type parameter is constrained to: what an argument for a parameter of
// that type has to do to fit it.
func (c *checker) meetsConstraints(t types.Type, tp *types.TypeParam) bool {
	for _, con := range tp.Constraints {
		proto, ok := protocolOf(con)
		if !ok {
			continue
		}
		if !c.conformsTo(t, proto) {
			return false
		}
	}
	return true
}

// byLiteralDefaults is which of several signatures that all fit take a
// literal argument as the type the literal is on its own -- an integer
// literal as an Int, a float literal as a Double -- which is the one
// Swift prefers: `stride(from: 0, to: 10, by: 3)` is the Int stride
// beside the Double one. It is the indices of those, or none.
func (c *checker) byLiteralDefaults(sigs []*types.Signature, args []*ast.CallArg) []int {
	var keep []int
	for i, sig := range sigs {
		if sig == nil {
			continue
		}
		suits := true
		for j, a := range args {
			if j >= len(sig.Params) || !c.isLiteralTree(a.X) {
				continue
			}
			pt := sig.Params[j].Type
			if hasFloatLiteral(a.X) {
				if !types.Identical(pt, types.Typ[types.Double]) {
					suits = false
				}
			} else if !types.Identical(pt, types.Typ[types.Int]) {
				suits = false
			}
		}
		if suits {
			keep = append(keep, i)
		}
	}
	return keep
}

// checkAsyncCall holds a call to an async function to Swift's two rules: it
// is written under `await`, and it is made from somewhere that can suspend.
func (c *checker) checkAsyncCall(call *ast.CallExpr, sig *types.Signature) {
	if sig == nil || !sig.Async {
		return
	}
	switch {
	case !c.currAsync:
		c.errorf(call.Pos(), "'async' call in a function that does not support concurrency")
	case !c.inAwait:
		c.errorf(call.Pos(), "expression is 'async' but is not marked with 'await'")
	}
}

// byContext narrows overloads that fit equally to the ones that suit the
// context by async. Swift allows `func load()` beside `func load() async`
// and picks the async one in an async function -- where the call then
// needs `await` -- and the other everywhere else, so a function that could
// suspend never silently blocks instead.
func (c *checker) byContext(fits []*FuncSymbol) []*FuncSymbol {
	if len(fits) < 2 {
		return fits
	}
	var suited []*FuncSymbol
	for _, f := range fits {
		if sig := f.Signature(); sig != nil && sig.Async == c.currAsync {
			suited = append(suited, f)
		}
	}
	if len(suited) == 0 {
		return fits
	}
	return suited
}

// awaitsIn reports whether statements await, outside any closure nested
// in them, which is what makes a closure with no signature async.
// Awaits reports whether the statements suspend anywhere: an `await`
// outside any closure of their own.
func Awaits(stmts []ast.Stmt) bool { return awaitsIn(stmts) }

func awaitsIn(stmts []ast.Stmt) bool {
	found := false
	for _, s := range stmts {
		ast.Inspect(s, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.AwaitExpr:
				found = true
			case *ast.ForInStmt:
				found = found || x.Await.IsValid()
			case *ast.ClosureExpr:
				return false
			}
			return !found
		})
	}
	return found
}

// TopLevelAwaits reports whether a file's top-level code -- not the
// bodies of what it declares, nor its closures -- awaits, or starts an
// `async let`.
func TopLevelAwaits(f *ast.File) bool {
	found := false
	for _, s := range f.Stmts {
		if ds, ok := s.(*ast.DeclStmt); ok {
			vd, isVar := ds.D.(*ast.VarDecl)
			if !isVar {
				continue
			}
			for _, m := range vd.Mods {
				if m != nil && m.Name != nil && f.Unit != nil && m.Name.Text(f.Unit) == "async" {
					return true
				}
			}
		}
		ast.Inspect(s, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.AwaitExpr:
				found = true
			case *ast.ForInStmt:
				found = found || x.Await.IsValid()
			case *ast.ClosureExpr, *ast.FuncDecl:
				return false
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

// candidatesFitting returns candidate functions matching argument counts and types.
func (c *checker) candidatesFitting(candidates []*FuncSymbol, args []*ast.CallArg,
	argTypes []types.Type, fitsLabel func(*ast.CallArg, *types.Param) bool) []*FuncSymbol {
	var fits []*FuncSymbol
	for _, cand := range candidates {
		if c.sigFits(cand.Signature(), args, argTypes, fitsLabel) {
			fits = append(fits, cand)
		}
	}
	return fits
}

// sigFits reports whether arguments fit a signature: in order, each by
// label and type, leaving out a parameter with a default when nothing is
// written for it. An argument whose type could not be found alone -- an
// implicit member, which needs the parameter's type -- fits by its label.
func (c *checker) sigFits(sig *types.Signature, args []*ast.CallArg, argTypes []types.Type,
	fitsLabel func(*ast.CallArg, *types.Param) bool) bool {
	if sig == nil || (len(args) > len(sig.Params) && !hasVariadic(sig)) {
		return false
	}
	next := 0
	for _, p := range sig.Params {
		// A variadic parameter takes none or any number of arguments:
		// the first by the parameter's label, the others by none.
		if p.Variadic {
			for first := true; next < len(args); first = false {
				fits := fitsLabel(args[next], p)
				if !first {
					fits = args[next].Label == nil
				}
				if !fits || !c.argFitsParam(args[next], argTypes[next], p) {
					break
				}
				next++
			}
			continue
		}
		if next < len(args) && fitsLabel(args[next], p) && c.argFitsParam(args[next], argTypes[next], p) {
			next++
			continue
		}
		if !p.HasDefault {
			return false
		}
	}
	return next == len(args)
}

// argFitsParam reports whether one argument's type fits a parameter.
func (c *checker) argFitsParam(arg *ast.CallArg, t types.Type, p *types.Param) bool {
	if t == nil || isInvalid(t) || types.AssignableTo(t, p.Type) {
		return true
	}
	if fn, ok := p.Type.Underlying().(*types.Signature); ok && p.Autoclosure {
		return types.AssignableTo(t, fn.Results)
	}
	// A parameter whose type is a type parameter takes any argument that
	// meets its constraints: `Count<R: AsyncReader>(_: inout R)` fits a
	// Net that is an AsyncReader, and a Reader-constrained twin does not.
	// (A variadic parameter's Type is its element's, which each of its
	// arguments is.)
	if tp, ok := p.Type.(*types.TypeParam); ok {
		return c.meetsConstraints(t, tp)
	}
	// One whose type holds type parameters -- `[B]` -- takes an argument
	// of its shape, each parameter bound to what stands in its place.
	if mentionsTypeParam(p.Type) {
		subst := map[*types.TypeParam]types.Type{}
		if types.Unify(p.Type, literalDefaults(t), subst) {
			for tp, bound := range subst {
				if !c.meetsConstraints(bound, tp) {
					return false
				}
			}
			return true
		}
	}
	// A closure written at the call takes its parameters' types from the
	// parameter it is passed as, so it fits any function parameter of its
	// arity -- and one with `$0`, `$1` any function parameter at all.
	if cl, ok := unparen(arg.X).(*ast.ClosureExpr); ok {
		if want, isFunc := p.Type.Underlying().(*types.Signature); isFunc {
			if cl.Sig == nil || cl.Sig.Params == nil {
				return true
			}
			return len(cl.Sig.Params.Params) == len(want.Params)
		}
	}
	// Untyped numeric literal arguments fit any numeric parameter -- an
	// integer literal any number, a float literal a floating-point one.
	if !c.isLiteralTree(arg.X) || !isNumericType(p.Type) {
		return false
	}
	if hasFloatLiteral(arg.X) {
		return isFloatingType(p.Type)
	}
	return true
}

// hasFloatLiteral reports whether a literal tree has a float literal in it.
func hasFloatLiteral(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.FLOAT_LIT {
			found = true
		}
		return !found
	})
	return found
}

// isFloatingType reports whether t is Float or Double.
func isFloatingType(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsFloat != 0
}

// resolveMethodOverload picks, among the methods a member call could name
// -- bind(_:) and bind(host:port:) -- the one its arguments fit, and
// records it for what lowers the call.
func (c *checker) resolveMethodOverload(mem *ast.MemberExpr, args []*ast.CallArg, scope *Scope) *types.Signature {
	if mem.Name == nil {
		return nil
	}
	base := c.info.Types[mem.X]
	if base == nil {
		return nil
	}
	recv, methods := methodsNamed(base, mem.Name.Text(c.file))
	// A method an extension gives Array, Dictionary, Set or Optional is
	// typed for the elements of the one it is called on: [Int].first()
	// answers an Int?, not an Element?, and its arguments are matched
	// against Int too.
	var subst map[*types.TypeParam]types.Type
	// The same for a generic type of the program's own: Box<Int>'s f
	// takes a P<Int> where it was declared taking a P<T>.
	if inst := genericInstanceOf(base); inst != nil {
		params := typeParamsOf(inst.Base)
		subst = make(map[*types.TypeParam]types.Type, len(params))
		for i, p := range params {
			if i < len(inst.Args) {
				subst[p] = inst.Args[i]
			}
		}
	}
	if len(methods) == 0 {
		recv, methods = c.builtinMethods(base, mem.Name.Text(c.file))
		if b := c.builtinOf(base); b != nil {
			subst = b.Subst(base)
		}
	}
	if len(methods) == 0 {
		return c.extensionOverload(mem, base, args, scope)
	}
	if len(methods) < 2 {
		return nil
	}
	// Those an extension gives only where the arguments are something
	// this instance's are not -- joined() of an array of arrays, on an
	// array of strings -- are not among them.
	var held []*types.Method
	for _, m := range methods {
		if c.conditionsMet(m, base) {
			held = append(held, m)
		}
	}
	if len(held) > 0 {
		methods = held
	}
	candidates := methods
	if len(subst) > 0 {
		candidates = make([]*types.Method, len(methods))
		for i, m := range methods {
			copied := *m
			if s, ok := types.Substitute(m.Sig, subst).(*types.Signature); ok {
				copied.Sig = s
			}
			candidates[i] = &copied
		}
	}
	m := c.methodByArguments(candidates, args, scope)
	if m == nil && len(candidates) == 1 {
		m = candidates[0]
	}
	if m == nil {
		return nil
	}
	sig := m.Sig
	for i, cand := range candidates {
		if cand == m {
			m = methods[i]
		}
	}
	c.info.Methods[mem] = &MethodRef{Recv: recv, Method: m}
	c.info.Types[mem] = sig
	return sig
}

// methodByArguments is the one method of several the arguments fit, by
// label and type, and by async where that is all that differs; or nil.
func (c *checker) methodByArguments(methods []*types.Method, args []*ast.CallArg, scope *Scope) *types.Method {
	quiet := len(c.info.Diagnostics)
	argTypes := make([]types.Type, len(args))
	for i, arg := range args {
		argTypes[i] = c.checkExpr(arg.X, nil, scope)
	}
	c.info.Diagnostics = c.info.Diagnostics[:quiet]
	pick := func(fitsLabel func(*ast.CallArg, *types.Param) bool) []*types.Method {
		var out []*types.Method
		for _, m := range methods {
			if c.sigFits(m.Sig, args, argTypes, fitsLabel) {
				out = append(out, m)
			}
		}
		return out
	}
	fits := pick(c.labelFits)
	if len(fits) == 0 {
		fits = pick(c.labelFitsLax)
	}
	// Methods that differ only in async are chosen by context too; see byContext.
	if len(fits) > 1 {
		var suited []*types.Method
		for _, m := range fits {
			if m.Sig != nil && m.Sig.Async == c.currAsync {
				suited = append(suited, m)
			}
		}
		if len(suited) > 0 {
			fits = suited
		}
	}
	if len(fits) > 1 {
		var sigs []*types.Signature
		for _, m := range fits {
			sigs = append(sigs, m.Sig)
		}
		if keep := c.byLiteralDefaults(sigs, args); len(keep) > 0 {
			var narrowed []*types.Method
			for _, i := range keep {
				narrowed = append(narrowed, fits[i])
			}
			fits = narrowed
		}
	}
	if len(fits) != 1 {
		return nil
	}
	return fits[0]
}

// methodsNamed is every method named name a member of t could call, and
// the type declaring them.
func methodsNamed(t types.Type, name string) (types.Type, []*types.Method) {
	onType := false
	if meta, ok := t.(*types.Metatype); ok {
		onType = true
		t = meta.Instance
	}
	if inst, ok := t.(*types.GenericInstance); ok {
		t = inst.Base
	}
	var methods []*types.Method
	switch b := t.Underlying().(type) {
	case *types.Struct:
		methods = b.Methods
	case *types.Class:
		methods = b.Methods
	case *types.Enum:
		methods = b.Methods
	}
	var out []*types.Method
	for _, m := range methods {
		if m != nil && m.Name == name && m.IsStatic == onType {
			out = append(out, m)
		}
	}
	return t, out
}

// genericInstanceOf is the instance t is, or the metatype of, or nil.
func genericInstanceOf(t types.Type) *types.GenericInstance {
	if meta, ok := t.(*types.Metatype); ok {
		t = meta.Instance
	}
	inst, _ := t.(*types.GenericInstance)
	return inst
}

// candidateLabels formats candidate parameter labels for ambiguity error diagnostics.
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

// labelFits reports whether an argument's label matches a parameter's expected label.
func (c *checker) labelFits(arg *ast.CallArg, param *types.Param) bool {
	want := param.Label
	if want == "" {
		want = param.Name
	}
	if arg.Label == nil {
		if want == "" || want == "_" {
			return true
		}
		if _, ok := unparen(arg.X).(*ast.ClosureExpr); ok && param.Type != nil {
			if _, isFunc := param.Type.Underlying().(*types.Signature); isFunc {
				return true
			}
		}
		return false
	}
	return arg.Label.Text(c.file) == want
}

// labelFitsLax checks label compatibility allowing omitted labels or matching internal names.
func (c *checker) labelFitsLax(arg *ast.CallArg, param *types.Param) bool {
	if arg.Label == nil {
		return true
	}
	written := arg.Label.Text(c.file)
	return written == param.Label || written == param.Name
}

// inferFromInits infers a generic type's arguments from the declared
// initializer the call's labels fit, or is nil where none fits or some
// type parameter is left unbound by it.
func (c *checker) inferFromInits(instance types.Type, params []*types.TypeParam, call *ast.CallExpr, scope *Scope) types.Type {
	var inits []*types.Signature
	switch u := instance.Underlying().(type) {
	case *types.Struct:
		inits = u.Inits
	case *types.Class:
		inits = u.Inits
	case *types.Enum:
		inits = u.Inits
	}
	if len(inits) == 0 || call.Args == nil {
		return nil
	}
	args := call.Args.Args
	quiet := len(c.info.Diagnostics)
	argTypes := make([]types.Type, len(args))
	for i, arg := range args {
		argTypes[i] = c.checkExpr(arg.X, nil, scope)
	}
	c.info.Diagnostics = c.info.Diagnostics[:quiet]
	for _, sig := range inits {
		if sig == nil || !c.sigFits(sig, args, argTypes, c.labelFits) {
			continue
		}
		// The parameters the arguments went to, in the order sigFits
		// matched them, skipping those left to their defaults.
		subst := make(map[*types.TypeParam]types.Type, len(params))
		next := 0
		for _, p := range sig.Params {
			if next < len(args) && c.labelFits(args[next], p) && c.argFitsParam(args[next], argTypes[next], p) {
				if argTypes[next] != nil {
					types.Unify(p.BodyType(), argTypes[next], subst)
				}
				next++
			}
		}
		// A parameter no argument binds may be one the initializer's
		// extension fixes: `Result(catching:)` is declared where
		// `Failure == any Error`.
		for _, cond := range c.info.conditions[sig] {
			if cond.same != nil && cond.param != nil {
				if _, ok := subst[cond.param]; !ok {
					subst[cond.param] = cond.same
				}
			}
		}
		bound := make([]types.Type, len(params))
		complete := true
		for i, p := range params {
			t, ok := subst[p]
			if !ok {
				complete = false
				break
			}
			bound[i] = t
		}
		if complete {
			return &types.GenericInstance{Base: instance, Args: bound}
		}
	}
	return nil
}

// inferInstance infers specialized generic types from memberwise initializer arguments.
func (c *checker) inferInstance(instance types.Type, call *ast.CallExpr, scope *Scope) types.Type {
	if call.Args == nil {
		return instance
	}
	fields := storedFieldsOf(instance)
	params := typeParamsOf(instance)
	if len(params) > 0 && len(fields) == 0 {
		if _, isEnum := instance.Underlying().(*types.Enum); isEnum {
			if inst := c.inferFromInits(instance, params, call, scope); inst != nil {
				return inst
			}
		}
	}
	if len(params) == 0 || len(fields) == 0 {
		return instance
	}

	// A declared initializer says what each argument is for, by its
	// labels: `Pair(a, to: b)` binds B through `init(_:to:)`, which no
	// stored field's name could.
	if inst := c.inferFromInits(instance, params, call, scope); inst != nil {
		return inst
	}

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
			return instance
		}
		args[i] = bound
	}
	return &types.GenericInstance{Base: instance, Args: args}
}

// fieldFor matches an initializer argument to a stored property by label or position.
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

// typeParamsOf returns the generic type parameters of a nominal type.
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

// storedFieldsOf returns the stored fields of a struct or class.
func storedFieldsOf(t types.Type) []*types.Field {
	switch n := t.(type) {
	case *types.Struct:
		return n.Fields
	case *types.Class:
		return n.Fields
	}
	return nil
}

// checkCallArguments checks argument types and labels against a signature.
func (c *checker) checkCallArguments(call *ast.CallExpr, sig *types.Signature, args []*ast.CallArg, scope *Scope) *types.Signature {
	// A default of #line or #function is the call's: where it is, and
	// the function it is in.
	if call != nil && sig != nil {
		for _, p := range sig.Params {
			if p != nil && p.HasDefault {
				c.info.CallSites[call] = CallSite{Func: c.currFuncName, Pos: call.Pos()}
				break
			}
		}
	}
	c.checkAsyncCall(call, sig)
	what, name := c.calleeWords(call, sig)
	c.checkIsolatedCall(call, sig, what, name)
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

		if arg.Label != nil && !c.labelFitsLax(arg, param) {
			c.errorf(arg.Pos(), "incorrect argument label (have '%s:', expected '%s:')",
				arg.Label.Text(c.file), paramName(param))
		}

		// What is written for an @autoclosure parameter is the value the
		// function it becomes returns.
		if fn, ok := param.Type.Underlying().(*types.Signature); ok && param.Autoclosure {
			argType := c.checkExpr(arg.X, fn.Results, scope)
			if !types.AssignableTo(argType, fn.Results) {
				c.typeErrorf(arg.Pos(), "cannot convert value of type '%s' to expected argument type '%s'", argType, fn.Results)
			}
			c.info.Autoclosures[arg.X] = fn
			continue
		}
		if param.Builder != nil {
			c.applyBuilder(arg.X, param.Builder)
		}
		argType := c.checkExpr(arg.X, param.Type, scope)
		if !types.AssignableTo(argType, param.Type) {
			if isString(argType) && cStringParam(param.Type) {
				c.info.CStrings[arg.X] = param.Type
				continue
			}
			c.typeErrorf(arg.Pos(), "cannot convert value of type '%s' to expected argument type '%s'", argType, param.Type)
		}
	}
	return sig
}

// isString reports whether t is String.
func isString(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.String
}

// cStringParam reports whether a parameter of type t takes a String as a C
// string: an immutable pointer to CChar, Int8 or UInt8, or an immutable raw
// pointer, optional or not. Swift's rule, which is what lets
// `strlen("hello")` and every C API taking `const char *` be called with a
// String.
func cStringParam(t types.Type) bool {
	if t == nil {
		return false
	}
	if o, ok := t.Underlying().(*types.Optional); ok {
		t = o.Wrapped
	}
	p, ok := t.Underlying().(*types.Pointer)
	if !ok || p.Mutable || p.Opaque {
		return false
	}
	if p.Elem == nil {
		return true
	}
	b, ok := p.Elem.Underlying().(*types.Basic)
	if !ok {
		return false
	}
	switch b.Kind() {
	case types.Int8, types.UInt8:
		return true
	}
	return false
}

// matchByLabel pairs arguments with parameters by label, skipping defaulted parameters.
func (c *checker) matchByLabel(call *ast.CallExpr, params []*types.Param, args []*ast.CallArg) []*types.Param {
	out := make([]*types.Param, 0, len(args))
	pi := 0
	for _, arg := range args {
		// Skip defaulted parameters that this argument does not match.
		// A closure written without a label skips a defaulted parameter
		// that takes no function, as a trailing closure does in Swift.
		for pi < len(params) && params[pi].HasDefault &&
			(!c.labelFits(arg, params[pi]) || closureForNonFunction(arg, params[pi])) {
			pi++
		}
		if pi >= len(params) {
			break
		}
		if !c.labelFits(arg, params[pi]) {
			c.errorf(call.Pos(), "missing argument for parameter '%s'", paramName(params[pi]))
			return out
		}
		out = append(out, params[pi])
		pi++
	}
	for ; pi < len(params); pi++ {
		if !params[pi].HasDefault {
			c.errorf(call.Pos(), "missing argument for parameter '%s'", paramName(params[pi]))
		}
	}
	return out
}

// closureForNonFunction reports whether an argument is a closure written
// without a label and the parameter takes something other than a function.
func closureForNonFunction(arg *ast.CallArg, p *types.Param) bool {
	if arg.Label != nil || p.Type == nil {
		return false
	}
	if _, isClosure := unparen(arg.X).(*ast.ClosureExpr); !isClosure {
		return false
	}
	t := p.Type.Underlying()
	if o, ok := t.(*types.Optional); ok && o.Wrapped != nil {
		t = o.Wrapped.Underlying()
	}
	_, isFunc := t.(*types.Signature)
	return !isFunc
}

// labels reports whether a parameter matches an argument label.
func labels(p *types.Param, label string) bool {
	if p.Label == "" || p.Label == "_" {
		return label == ""
	}
	return p.Label == label
}

// paramName returns the display name of a parameter for diagnostics.
func paramName(p *types.Param) string {
	if p.Label != "" && p.Label != "_" {
		return p.Label
	}
	if p.Name != "" {
		return p.Name
	}
	return "_"
}

// arity returns a descriptive string for expected argument counts.
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

// inferGenericCall infers type parameters from argument types and returns the specialized signature.
func (c *checker) inferGenericCall(e *ast.CallExpr, sig *types.Signature, args []*ast.CallArg, scope *Scope) *types.Signature {
	if len(sig.TypeParams) == 0 || len(args) == 0 {
		return sig
	}
	subst := make(map[*types.TypeParam]types.Type, len(sig.TypeParams))
	quiet := len(c.info.Diagnostics)
	var closures, operators, literals []int
	for i, arg := range args {
		if i >= len(sig.Params) {
			break
		}
		if _, isClosure := arg.X.(*ast.ClosureExpr); isClosure {
			closures = append(closures, i)
			continue
		}
		// A method or initializer named as a value is read as a closure
		// is, against what the rest have said.
		if c.isReference(arg.X) {
			if _, isInit := c.initNamed(arg.X, scope); isInit {
				closures = append(closures, i)
				continue
			}
		}
		// An operator named as a value, and a literal, say what they are
		// once the others have: `xs.reduce(0, +)` over [T] is a T.
		if _, isOp := unparen(arg.X).(*ast.OperatorExpr); isOp {
			operators = append(operators, i)
			continue
		}
		if literalOperand(arg.X) {
			literals = append(literals, i)
			continue
		}
		types.Unify(sig.Params[i].Type, c.checkExpr(arg.X, nil, scope), subst)
	}
	for _, i := range operators {
		want := types.Substitute(sig.Params[i].Type, subst)
		if fn, ok := want.(*types.Signature); ok {
			want = alikeOperands(fn, sig.TypeParams, subst)
		}
		got := c.checkExpr(args[i].X, want, scope)
		if !mentionsParamOf(got, sig.TypeParams) && !mentionsInvalid(got) {
			types.Unify(sig.Params[i].Type, got, subst)
		}
	}
	for _, i := range literals {
		want := types.Substitute(sig.Params[i].Type, subst)
		if mentionsParamOf(want, sig.TypeParams) {
			want = nil
		}
		types.Unify(sig.Params[i].Type, c.checkExpr(args[i].X, want, scope), subst)
	}
	// A closure is read after the other arguments, against the function
	// type it is passed as with what they have said already in place: its
	// parameters then have types, and what its body returns can say what
	// the rest of the call's parameters are. One whose type still names a
	// parameter says nothing about it.
	for _, i := range closures {
		want := types.Substitute(sig.Params[i].Type, subst)
		got := c.checkExpr(args[i].X, want, scope)
		// A type parameter of the code around the call -- the Element of
		// an extension of Array -- is a type like any other there; only
		// the call's own, still to be inferred, say nothing.
		if !mentionsParamOf(got, sig.TypeParams) && !mentionsInvalid(got) {
			types.Unify(sig.Params[i].Type, got, subst)
		}
	}
	c.info.Diagnostics = c.info.Diagnostics[:quiet]
	if len(subst) == 0 {
		return sig
	}
	c.checkConstraints(e, sig.TypeParams, subst)

	// Record type specialization arguments for code generation.
	if e != nil {
		spec := Specialization{Params: sig.TypeParams}
		for _, p := range sig.TypeParams {
			spec.Args = append(spec.Args, subst[p])
		}
		c.info.Specializations[e] = spec
	}
	if out, ok := c.builtinDependents(types.Substitute(sig, subst)).(*types.Signature); ok {
		return out
	}
	return sig
}

// isNumericType reports whether t is a numeric basic type.
func isNumericType(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsNumeric != 0
}

// checkConstraints checks that type arguments satisfy parameter protocol constraints.
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
				continue
			}
			if c.conformsTo(arg, proto) {
				continue
			}
			c.typeErrorf(e.Pos(), "%s requires that '%s' conform to '%s'",
				calleeDescription(e, c), arg, proto.Name)
		}
	}
}

// protocolOf returns the protocol referenced by t, if any.
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

// calleeDescription names the function a call is to, for a message about its constraints.
func calleeDescription(e *ast.CallExpr, c *checker) string {
	if id, ok := e.Fun.(*ast.IdentExpr); ok && id.Name != nil {
		return "global function '" + id.Name.Text(c.file) + "'"
	}
	return "this call"
}

// variadicIndex returns the index of the variadic parameter, if present.
func variadicIndex(sig *types.Signature) (int, bool) {
	for i, p := range sig.Params {
		if p != nil && p.Variadic {
			return i, true
		}
	}
	return 0, false
}

// variadicParams maps call arguments to parameters for a variadic signature.
func variadicParams(sig *types.Signature, vi int, args []*ast.CallArg,
	file *token.File) []*types.Param {
	out := make([]*types.Param, len(args))
	next := 0
	for i := 0; i < vi && next < len(args); i++ {
		out[next] = sig.Params[i]
		next++
	}
	list := sig.Params[vi]
	for first := true; next < len(args); next++ {
		// The first by the list's label, the rest by none.
		if (first && !argLabelFits(args[next], list, file)) || (!first && args[next].Label != nil) {
			break
		}
		first = false
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

// argLabelFits reports whether an argument label matches the parameter label.
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

// mentionsTypeParam reports whether a type still has a generic parameter
// in it: one no call has given an argument for yet.
func mentionsTypeParam(t types.Type) bool {
	return mentionsParamWhere(t, func(*types.TypeParam) bool { return true })
}

// mentionsOpenParam reports whether t mentions a type parameter that is
// not in scope here: a callee's, still to be inferred, rather than the
// enclosing generic function's own.
func (c *checker) mentionsOpenParam(t types.Type, scope *Scope) bool {
	return mentionsParamWhere(t, func(tp *types.TypeParam) bool {
		tn := scope.LookupType(tp.Name)
		return tn == nil || tn.Type() != tp
	})
}

// mentionsParamWhere reports whether t mentions a type parameter open is
// true of.
func mentionsParamWhere(t types.Type, open func(*types.TypeParam) bool) bool {
	mentionsTypeParam := func(t types.Type) bool { return mentionsParamWhere(t, open) }
	switch x := t.(type) {
	case nil:
		return false
	case *types.TypeParam:
		return open(x)
	case *types.Optional:
		return mentionsTypeParam(x.Wrapped)
	case *types.Array:
		return mentionsTypeParam(x.Elem)
	case *types.Set:
		return mentionsTypeParam(x.Elem)
	case *types.Dictionary:
		return mentionsTypeParam(x.Key) || mentionsTypeParam(x.Value)
	case *types.Tuple:
		for _, e := range x.Elements {
			if e != nil && mentionsTypeParam(e.Type) {
				return true
			}
		}
	case *types.Signature:
		for _, p := range x.Params {
			if p != nil && mentionsTypeParam(p.Type) {
				return true
			}
		}
		return mentionsTypeParam(x.Results)
	case *types.GenericInstance:
		for _, a := range x.Args {
			if mentionsTypeParam(a) {
				return true
			}
		}
	}
	return false
}

// mentionsParamOf reports whether t names one of params.
func mentionsParamOf(t types.Type, params []*types.TypeParam) bool {
	if len(params) == 0 || t == nil {
		return false
	}
	sentinel := types.Typ[types.Invalid]
	subst := make(map[*types.TypeParam]types.Type, len(params))
	for _, p := range params {
		subst[p] = sentinel
	}
	return mentionsInvalid(types.Substitute(t, subst)) && !mentionsInvalid(t)
}

// mentionsInvalid reports whether t is, or is built from, the invalid type.
func mentionsInvalid(t types.Type) bool {
	switch x := t.(type) {
	case nil:
		return false
	case *types.Basic:
		return x.Kind() == types.Invalid
	case *types.Optional:
		return mentionsInvalid(x.Wrapped)
	case *types.Array:
		return mentionsInvalid(x.Elem)
	case *types.Set:
		return mentionsInvalid(x.Elem)
	case *types.Dictionary:
		return mentionsInvalid(x.Key) || mentionsInvalid(x.Value)
	case *types.Tuple:
		for _, e := range x.Elements {
			if e != nil && mentionsInvalid(e.Type) {
				return true
			}
		}
	case *types.Signature:
		for _, p := range x.Params {
			if p != nil && mentionsInvalid(p.Type) {
				return true
			}
		}
		return mentionsInvalid(x.Results)
	case *types.GenericInstance:
		for _, a := range x.Args {
			if mentionsInvalid(a) {
				return true
			}
		}
	}
	return false
}

// alikeOperands is the function type an operator named as a value is
// wanted as, with the call's parameters still to infer taken to be the
// type of an operand the call has settled: an operator's operands and
// result are alike as a rule, so reduce's Result beside a T is a T.
func alikeOperands(fn *types.Signature, open []*types.TypeParam, subst map[*types.TypeParam]types.Type) types.Type {
	isOpen := func(t types.Type) bool {
		tp, ok := t.(*types.TypeParam)
		if !ok {
			return false
		}
		for _, p := range open {
			if p == tp {
				_, bound := subst[p]
				return !bound
			}
		}
		return false
	}
	var known types.Type
	for _, p := range fn.Params {
		if p != nil && p.Type != nil && !isOpen(p.Type) && !mentionsParamOf(p.Type, open) {
			known = p.Type
			break
		}
	}
	if known == nil {
		return fn
	}
	out := *fn
	out.Params = make([]*types.Param, len(fn.Params))
	for i, p := range fn.Params {
		q := *p
		if isOpen(q.Type) {
			q.Type = known
		}
		out.Params[i] = &q
	}
	if isOpen(out.Results) {
		out.Results = known
	}
	return &out
}

// hasVariadic reports whether a signature has a variadic parameter.
func hasVariadic(sig *types.Signature) bool {
	for _, p := range sig.Params {
		if p != nil && p.Variadic {
			return true
		}
	}
	return false
}

// extensionOverload picks among the methods of a name that extensions of
// the protocols base conforms to, or is constrained to, give it --
// Collection's firstIndex(of:) beside firstIndex(where:) -- the one the
// arguments fit, and records it as the protocol's.
func (c *checker) extensionOverload(mem *ast.MemberExpr, base types.Type, args []*ast.CallArg, scope *Scope) *types.Signature {
	static := false
	t := base
	if meta, ok := t.(*types.Metatype); ok {
		static, t = true, meta.Instance
	}
	var protocols []*types.Protocol
	switch tt := t.(type) {
	case *types.TypeParam:
		for _, con := range tt.Constraints {
			if p, ok := con.Underlying().(*types.Protocol); ok {
				protocols = append(protocols, p)
			}
		}
	case *types.Dependent:
		for _, con := range associatedConstraints(tt) {
			if p, ok := con.Underlying().(*types.Protocol); ok {
				protocols = append(protocols, p)
			}
		}
	default:
		protocols = c.conformancesOfType(t)
	}
	name := mem.Name.Text(c.file)
	var owners []*types.Protocol
	var candidates []*types.Method
	seen := map[*types.Method]bool{}
	for _, p := range allProtocols(protocols) {
		// What the protocol requires -- Collection's index(after:) beside
		// BidirectionalCollection's index(before:) -- where t stands for
		// its Self, a type parameter or an associated type.
		if _, abstract := t.(*types.TypeParam); abstract || isDependent(t) {
			for _, r := range p.Requirements {
				if r == nil || r.Sig == nil || r.Name != name || r.IsStatic != static {
					continue
				}
				sig, _ := throughParam(p, t, r.Sig).(*types.Signature)
				if sig == nil {
					continue
				}
				owners = append(owners, p)
				candidates = append(candidates, &types.Method{Name: r.Name, Sig: sig, IsStatic: r.IsStatic, IsMutating: r.IsMutating})
			}
		}
		for _, m := range p.ExtMethods {
			if m == nil || m.Name != name || m.IsStatic != static || seen[m] {
				continue
			}
			seen[m] = true
			sig, _ := types.Substitute(m.Sig, map[*types.TypeParam]types.Type{p.Self: t}).(*types.Signature)
			if sig == nil {
				continue
			}
			owners = append(owners, p)
			candidates = append(candidates, &types.Method{Name: m.Name, Sig: sig, IsStatic: m.IsStatic, IsMutating: m.IsMutating, Origin: m})
		}
	}
	if len(candidates) < 2 {
		return nil
	}
	picked := c.methodByArguments(candidates, args, scope)
	if picked == nil {
		return nil
	}
	for i, m := range candidates {
		if m == picked {
			c.info.Methods[mem] = &MethodRef{Recv: owners[i], Method: m}
			c.info.Types[mem] = m.Sig
			return m.Sig
		}
	}
	return nil
}

// isDependent reports whether t is an associated type path.
func isDependent(t types.Type) bool {
	_, ok := t.(*types.Dependent)
	return ok
}
