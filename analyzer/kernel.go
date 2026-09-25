package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/types"
)

// Kernels: `func f(...) kernel`, the functions a GPU runs once per
// work-item, and the launches that run them. proposed_vertex_kernel.md
// says what they mean; this file is what the checker makes of them.
//
// A kernel's signature is checked where it is declared: what a device
// can be handed (§5.1), and no async or throws. What its body reaches is
// checked where it is compiled for the device, where every call it makes
// is known (vsc's device compile).
//
// A launch, `k.Launch(args…, over: n, workgroup: w)`, is typed against
// the kernel's own parameters, a gpu.Span<T> taken as a gpu.Buffer<T>,
// and then written as the chain of the built-in gpu module's _Launch
// methods that does it:
//
//	gpu._Launch(_kernel: gpu._kernelDescriptor())._float32(a)._buffer(x)._over1(n)._run()
//
// where the descriptor call is recorded against the kernel, and lowered
// to the address of the descriptor the compiler writes beside it.

// kernelScalar is what a kernel parameter of a scalar type is launched
// with: the _Launch method that takes it.
var kernelScalar = map[types.BasicKind]string{
	types.Int8:     "_int8",
	types.UInt8:    "_uint8",
	types.Int16:    "_int16",
	types.UInt16:   "_uint16",
	types.Int32:    "_int32",
	types.UInt32:   "_uint32",
	types.Int:      "_int",
	types.UInt:     "_uint",
	types.Int64:    "_int64",
	types.UInt64:   "_uint64",
	types.Float:    "_float32",
	types.Double:   "_float64",
	types.Float16:  "_float16",
	types.BFloat16: "_bfloat16",
	types.Bool:     "_bool",
}

// kernelOf is the kernel an expression names, if it names one.
func (c *checker) kernelOf(x ast.Expr, scope *Scope) (*FuncSymbol, *ast.FuncDecl, bool) {
	id, ok := unparen(x).(*ast.IdentExpr)
	if !ok || id.Name == nil {
		return nil, nil, false
	}
	sym, ok := scope.Lookup(id.Name.Text(c.file)).(*FuncSymbol)
	if !ok {
		return nil, nil, false
	}
	d, ok := sym.Decl().(*ast.FuncDecl)
	if !ok || d.Sig == nil || d.Sig.Exec != ast.ExecKernel {
		return nil, nil, false
	}
	return sym, d, true
}

// gpuType is the built-in gpu module's type of the name, where this file
// imports gpu.
func (c *checker) gpuType(name string) types.Type {
	if c.gpuAlias == "" {
		return nil
	}
	s := c.modules[c.gpuAlias]
	if s == nil {
		return nil
	}
	tn, ok := s.Lookup(name).(*TypeNameSymbol)
	if !ok {
		return nil
	}
	return tn.Type()
}

// spanElement is the element type of a gpu.Span<T> or gpu.MutableSpan<T>.
func (c *checker) spanElement(t types.Type) (types.Type, bool) {
	gi, ok := t.(*types.GenericInstance)
	if !ok || len(gi.Args) != 1 {
		return nil, false
	}
	for _, name := range []string{"Span", "MutableSpan"} {
		if base := c.gpuType(name); base != nil && types.Identical(gi.Base, base) {
			return gi.Args[0], true
		}
	}
	return nil, false
}

// kernelScalarOf is the _Launch method a scalar kernel parameter takes.
func kernelScalarOf(t types.Type) (string, bool) {
	if t == nil {
		return "", false
	}
	b, ok := t.Underlying().(*types.Basic)
	if !ok {
		return "", false
	}
	m, ok := kernelScalar[b.Kind()]
	return m, ok
}

// checkKernelDecl checks what a kernel's signature says against what a
// device can do (§5): no async or throws, and parameters and a result a
// device can be handed.
func (c *checker) checkKernelDecl(d *ast.FuncDecl, sig *types.Signature) {
	name := ""
	if d.Name != nil {
		name = d.Name.Text(c.file)
		if sym, ok := c.info.Defs[d.Name].(*FuncSymbol); ok {
			c.info.Kernels = append(c.info.Kernels, sym)
		}
	}
	at := d.Pos()
	if d.Name != nil {
		at = d.Name.Pos()
	}
	if c.gpuAlias == "" {
		c.errorf(at, "'%s' is a kernel, and a kernel needs the gpu module: add import \"gpu\"", name)
		return
	}
	if sig.Async {
		c.errorf(at, "'%s' is a kernel, and a kernel cannot be async: there is nothing on a device to resume it", name)
	}
	if sig.Throws {
		c.errorf(at, "'%s' is a kernel, and a kernel cannot throw: there is nothing on a device to catch it. "+
			"Report failure through a buffer, or trap", name)
	}
	// A generic kernel is built for each set of type arguments it is
	// launched with (kernelLaunch): a parameter of a type parameter's type,
	// or a span of one, is checked there, once the type is known.
	generic := func(t types.Type) bool {
		_, ok := t.(*types.TypeParam)
		return ok
	}
	for _, p := range sig.Params {
		if p == nil {
			continue
		}
		if p.Ownership == types.InOut {
			c.errorf(at, "kernel '%s' takes '%s' inout; a kernel writes through a gpu.MutableSpan", name, p.Name)
			continue
		}
		if elem, ok := c.spanElement(p.Type); ok {
			if generic(elem) {
				continue
			}
			if _, scalar := kernelScalarOf(elem); !scalar {
				c.errorf(at, "kernel '%s' takes a span of '%s'; a span on a device holds numbers or bools", name, elem)
			}
			continue
		}
		if generic(p.Type) {
			continue
		}
		if _, ok := kernelScalarOf(p.Type); !ok {
			c.errorf(at, "kernel '%s' takes '%s' of type '%s'; a kernel takes numbers, bools, "+
				"gpu.Span and gpu.MutableSpan", name, p.Name, p.Type)
		}
	}
	if sig.Results != nil && !isVoidType(sig.Results) && !generic(sig.Results) {
		if _, ok := kernelScalarOf(sig.Results); !ok {
			c.errorf(at, "kernel '%s' returns '%s'; an element kernel returns a number or a bool", name, sig.Results)
		}
	}
}

// kernelCall checks a call that involves a kernel: `k.Launch(...)`,
// which is written as the launch it is, and `k(...)`, which is an error
// outside a kernel. ok is false for any other call.
func (c *checker) kernelCall(e *ast.CallExpr, scope *Scope) (types.Type, bool) {
	if sym, d, ok := c.kernelOf(e.Fun, scope); ok {
		grid := sym.Signature().Results == nil || isVoidType(sym.Signature().Results)
		switch {
		case grid:
			c.errorf(e.Pos(), "'%s' is a kernel: launch it with %s.Launch(…, over: n)", sym.Name(), sym.Name())
		case !c.inKernel:
			c.errorf(e.Pos(), "'%s' is an element kernel: apply it over buffers with %s.Map(…), "+
				"or call it from another kernel", sym.Name(), sym.Name())
		default:
			return nil, false
		}
		_ = d
		return types.Typ[types.Invalid], true
	}
	mem, ok := e.Fun.(*ast.MemberExpr)
	if !ok || mem.Name == nil {
		return nil, false
	}
	sym, _, ok := c.kernelOf(mem.X, scope)
	if !ok {
		return nil, false
	}
	switch verb := mem.Name.Text(c.file); verb {
	case "Launch":
		return c.kernelLaunch(e, sym, scope), true
	case "Map":
		return c.kernelMap(e, sym, scope), true
	case "Enqueue":
		c.errorf(mem.Name.Pos(), "%s.Enqueue is not built yet: launch the kernel with %s.Launch(…, over: n)",
			sym.Name(), sym.Name())
		return types.Typ[types.Invalid], true
	default:
		c.errorf(mem.Name.Pos(), "a kernel has Launch, Enqueue and Map, not '%s'", verb)
		return types.Typ[types.Invalid], true
	}
}

// kernelLaunch checks `k.Launch(args…, over: grid, workgroup: size)`
// and writes it as the _Launch chain that does it.
func (c *checker) kernelLaunch(e *ast.CallExpr, sym *FuncSymbol, scope *Scope) types.Type {
	invalid := types.Typ[types.Invalid]
	if c.gpuAlias == "" {
		c.errorf(e.Pos(), "launching '%s' needs the gpu module: add import \"gpu\"", sym.Name())
		return invalid
	}
	sig := sym.Signature()
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	label := func(a *ast.CallArg) string {
		if a.Label == nil {
			return ""
		}
		return a.Label.Text(c.file)
	}
	n := len(sig.Params)
	// A generic kernel's type arguments come from the arguments, as a
	// generic function's do: a gpu.Buffer<X> passed for a span of T, or
	// an X passed for a T, makes T X. The launch is of that specialization.
	var spec *Specialization
	if len(sig.TypeParams) > 0 && len(args) >= n {
		inferred, ok := c.inferKernelArgs(sym, sig, args[:n], scope)
		if !ok {
			return invalid
		}
		spec = inferred
		c.checkConstraints(e, sig.TypeParams, spec.Subst(), scope)
		concrete, isSig := types.Substitute(sig, spec.Subst()).(*types.Signature)
		if !isSig {
			return invalid
		}
		sig = concrete
	}
	if len(args) < n+1 || label(args[n]) != "over" {
		c.errorf(e.Pos(), "%s.Launch takes %s's %d argument(s), then over: the grid", sym.Name(), sym.Name(), n)
		return invalid
	}
	var group *ast.CallArg
	switch len(args) - n {
	case 1:
	case 2:
		if label(args[n+1]) != "workgroup" {
			c.errorf(args[n+1].Pos(), "after over:, a launch takes workgroup: and nothing else")
			return invalid
		}
		group = args[n+1]
	default:
		c.errorf(args[n].Pos(), "after over:, a launch takes workgroup: and nothing else")
		return invalid
	}

	sp := ast.Span{Lo: e.Pos(), Hi: e.End()}
	ident := func(name string) *ast.Ident { return &ast.Ident{Span: sp, Synth: name} }
	gpu := func() ast.Expr { return &ast.IdentExpr{Span: sp, Name: ident(c.gpuAlias)} }
	call := func(recv ast.Expr, method string, label string, x ast.Expr) *ast.CallExpr {
		fun := &ast.MemberExpr{Span: sp, X: recv, Dot: sp.Lo, Name: ident(method)}
		var list []*ast.CallArg
		if x != nil {
			a := &ast.CallArg{Span: ast.Span{Lo: x.Pos(), Hi: x.End()}, X: x}
			if label != "" {
				a.Label = ident(label)
			}
			list = append(list, a)
		}
		return &ast.CallExpr{Span: sp, Fun: fun, Args: &ast.CallArgs{Span: sp, Args: list}}
	}

	ok := true
	buffer := c.gpuType("Buffer")
	type step struct {
		method string
		x      ast.Expr
	}
	var steps []step
	for i, p := range sig.Params {
		a := args[i]
		want := p.Label
		if want == "_" {
			want = ""
		}
		if label(a) != want {
			if want == "" {
				c.errorf(a.Pos(), "%s's argument %d has no label", sym.Name(), i+1)
			} else {
				c.errorf(a.Pos(), "%s's argument %d is labelled '%s:'", sym.Name(), i+1, want)
			}
			ok = false
			continue
		}
		var method string
		var expect types.Type
		if elem, isSpan := c.spanElement(p.Type); isSpan {
			if buffer == nil {
				ok = false
				continue
			}
			method, expect = "_buffer", &types.GenericInstance{Base: buffer, Args: []types.Type{elem}}
		} else if m, scalar := kernelScalarOf(p.Type); scalar {
			method, expect = m, p.Type
		} else if _, outer := p.Type.(*types.TypeParam); outer {
			// A generic function's own type parameter, a number once
			// it is specialized: gpu._Launch._value.
			method, expect = "_value", p.Type
		} else {
			// The declaration said why already.
			ok = false
			continue
		}
		got := c.checkExpr(a.X, expect, scope)
		if isInvalid(got) {
			ok = false
			continue
		}
		if !types.AssignableTo(got, expect) {
			c.typeErrorf(a.X.Pos(), "cannot launch %s with '%s' for '%s': it takes '%s'", sym.Name(), got, p.Name, expect)
			ok = false
			continue
		}
		steps = append(steps, step{method, a.X})
	}
	dims := func(a *ast.CallArg, what, prefix string) (string, bool) {
		t := c.checkExpr(a.X, nil, scope)
		if isInvalid(t) {
			return "", false
		}
		intT := types.Typ[types.Int]
		if types.Identical(t, intT) {
			return prefix + "1", true
		}
		if tu, isTuple := t.(*types.Tuple); isTuple && (len(tu.Elements) == 2 || len(tu.Elements) == 3) {
			all := true
			for _, el := range tu.Elements {
				if el == nil || !types.Identical(el.Type, intT) {
					all = false
				}
			}
			if all {
				if len(tu.Elements) == 2 {
					return prefix + "2", true
				}
				return prefix + "3", true
			}
		}
		c.typeErrorf(a.X.Pos(), "%s: is a count of work-items, n, (w, h) or (w, h, d), in int; not '%s'", what, t)
		return "", false
	}
	over, overOK := dims(args[n], "over", "_over")
	ok = ok && overOK
	var groupM string
	if group != nil {
		m, groupOK := dims(group, "workgroup", "_group")
		ok = ok && groupOK
		groupM = m
	}
	if !ok {
		return invalid
	}

	desc := call(gpu(), "_kernelDescriptor", "", nil)
	c.info.KernelDescriptors[desc] = sym
	if spec != nil {
		c.info.Specializations[desc] = *spec
	}
	chain := call(gpu(), "_Launch", "_kernel", desc)
	for _, s := range steps {
		chain = call(chain, s.method, "", s.x)
	}
	chain = call(chain, over, "", args[n].X)
	if group != nil {
		chain = call(chain, groupM, "", group.X)
	}
	chain = call(chain, "_run", "", nil)
	c.info.ImplicitSelf[e] = chain
	return c.checkExpr(chain, nil, scope)
}

// kernelMap checks `k.Map(args…)` and `k.Map(args…, into: out)` on an
// element kernel (§4.2): an argument passed as a gpu.Buffer<T> where k
// takes a T is mapped, element by element; any other is the same for
// every element. It is written as the launch of the grid kernel the
// compiler makes for that choice of mapped parameters:
//
//	gpu._Launch(_kernel: gpu._kernelDescriptor())._mapped(xs)._float32(t)._mapRun_float32()
//
// and returns the new buffer, or nothing with into:.
func (c *checker) kernelMap(e *ast.CallExpr, sym *FuncSymbol, scope *Scope) types.Type {
	invalid := types.Typ[types.Invalid]
	sig := sym.Signature()
	if sig.Results == nil || isVoidType(sig.Results) {
		c.errorf(e.Pos(), "'%s' is a grid kernel, which returns nothing to map: launch it with %s.Launch(…, over: n)",
			sym.Name(), sym.Name())
		return invalid
	}
	if c.gpuAlias == "" {
		c.errorf(e.Pos(), "mapping '%s' needs the gpu module: add import \"gpu\"", sym.Name())
		return invalid
	}
	result, ok := kernelScalarOf(sig.Results)
	if !ok {
		return invalid
	}
	if d, isDecl := sym.Decl().(*ast.FuncDecl); !isDecl || c.info.Imported[sym] != "" {
		_ = d
		c.errorf(e.Pos(), "'%s' is another module's element kernel; Map builds the grid kernel for "+
			"one of this module's", sym.Name())
		return invalid
	}
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	label := func(a *ast.CallArg) string {
		if a.Label == nil {
			return ""
		}
		return a.Label.Text(c.file)
	}
	n := len(sig.Params)
	var into *ast.CallArg
	switch {
	case len(args) == n:
	case len(args) == n+1 && label(args[n]) == "into":
		into = args[n]
	default:
		c.errorf(e.Pos(), "%s.Map takes %s's %d argument(s), and into: an output buffer or nothing", sym.Name(), sym.Name(), n)
		return invalid
	}
	buffer := c.gpuType("Buffer")
	if buffer == nil {
		return invalid
	}
	sp := ast.Span{Lo: e.Pos(), Hi: e.End()}
	ident := func(name string) *ast.Ident { return &ast.Ident{Span: sp, Synth: name} }
	call := func(recv ast.Expr, method string, lab string, x ast.Expr) *ast.CallExpr {
		fun := &ast.MemberExpr{Span: sp, X: recv, Dot: sp.Lo, Name: ident(method)}
		var list []*ast.CallArg
		if x != nil {
			a := &ast.CallArg{Span: ast.Span{Lo: x.Pos(), Hi: x.End()}, X: x}
			if lab != "" {
				a.Label = ident(lab)
			}
			list = append(list, a)
		}
		return &ast.CallExpr{Span: sp, Fun: fun, Args: &ast.CallArgs{Span: sp, Args: list}}
	}
	type step struct {
		method string
		x      ast.Expr
	}
	var steps []step
	var mask uint64
	good := true
	for i, p := range sig.Params {
		a := args[i]
		want := p.Label
		if want == "_" {
			want = ""
		}
		if label(a) != want {
			c.errorf(a.Pos(), "%s's argument %d is labelled '%s:'", sym.Name(), i+1, want)
			good = false
			continue
		}
		scalar, _ := kernelScalarOf(p.Type)
		// A buffer is mapped; anything else is checked as the parameter's
		// own type, which types a literal as it.
		quiet := len(c.info.Diagnostics)
		t := c.checkExpr(a.X, nil, scope)
		c.info.Diagnostics = c.info.Diagnostics[:quiet]
		if gi, isInst := t.(*types.GenericInstance); isInst && types.Identical(gi.Base, buffer) && len(gi.Args) == 1 {
			if !types.Identical(gi.Args[0], p.Type) {
				c.typeErrorf(a.X.Pos(), "cannot map %s over '%s' for '%s': it takes '%s'", sym.Name(), t, p.Name, p.Type)
				good = false
				continue
			}
			mask |= 1 << uint(i)
			steps = append(steps, step{"_mapped", a.X})
			continue
		}
		got := c.checkExpr(a.X, p.Type, scope)
		if isInvalid(got) {
			good = false
			continue
		}
		if !types.AssignableTo(got, p.Type) {
			c.typeErrorf(a.X.Pos(), "cannot map %s with '%s' for '%s': it takes '%s', or a gpu.Buffer of it",
				sym.Name(), got, p.Name, p.Type)
			good = false
			continue
		}
		steps = append(steps, step{scalar, a.X})
	}
	if into != nil {
		want := &types.GenericInstance{Base: buffer, Args: []types.Type{sig.Results}}
		got := c.checkExpr(into.X, want, scope)
		if !isInvalid(got) && !types.AssignableTo(got, want) {
			c.typeErrorf(into.X.Pos(), "%s.Map writes '%s' into: it takes a '%s'", sym.Name(), got, want)
			good = false
		}
	} else if mask == 0 {
		c.errorf(e.Pos(), "%s.Map maps over buffers, and was given none: pass one, or into: the output", sym.Name())
		good = false
	}
	if !good {
		return invalid
	}
	gpu := &ast.IdentExpr{Span: sp, Name: ident(c.gpuAlias)}
	desc := call(gpu, "_kernelDescriptor", "", nil)
	c.info.KernelDescriptors[desc] = sym
	c.info.KernelMaps[desc] = mask
	chain := call(&ast.IdentExpr{Span: sp, Name: ident(c.gpuAlias)}, "_Launch", "_kernel", desc)
	for _, s := range steps {
		chain = call(chain, s.method, "", s.x)
	}
	if into != nil {
		chain = call(call(chain, "_into", "", into.X), "_run", "", nil)
	} else {
		chain = call(chain, "_mapRun"+result, "", nil)
	}
	c.info.ImplicitSelf[e] = chain
	return c.checkExpr(chain, nil, scope)
}

// inferKernelArgs works out a generic kernel's type arguments from the
// arguments of a launch of it, and checks each: it satisfies its type
// parameter's constraints, and it is a type a device has.
func (c *checker) inferKernelArgs(sym *FuncSymbol, sig *types.Signature, args []*ast.CallArg, scope *Scope) (*Specialization, bool) {
	subst := map[*types.TypeParam]types.Type{}
	buffer := c.gpuType("Buffer")
	// Buffers first: what a buffer holds is its type, where a literal
	// passed for a T would only be a default. Then the rest, each checked
	// against what its type parameter is already, if anything.
	for pass := 0; pass < 2; pass++ {
		for i, p := range sig.Params {
			var tp *types.TypeParam
			span := false
			if elem, ok := c.spanElement(p.Type); ok {
				tp, _ = elem.(*types.TypeParam)
				span = true
			} else {
				tp, _ = p.Type.(*types.TypeParam)
			}
			if tp == nil || span != (pass == 0) {
				continue
			}
			got := c.checkExpr(args[i].X, subst[tp], scope)
			if isInvalid(got) {
				return nil, false
			}
			arg := got
			if span {
				gi, ok := got.(*types.GenericInstance)
				if !ok || buffer == nil || !types.Identical(gi.Base, buffer) || len(gi.Args) != 1 {
					c.typeErrorf(args[i].X.Pos(), "cannot launch %s with '%s' for '%s': it takes a gpu.Buffer",
						sym.Name(), got, p.Name)
					return nil, false
				}
				arg = gi.Args[0]
			}
			if was, bound := subst[tp]; bound && !types.Identical(was, arg) {
				c.typeErrorf(args[i].X.Pos(), "cannot launch %s: '%s' is '%s' here and '%s' before",
					sym.Name(), tp, arg, was)
				return nil, false
			}
			subst[tp] = arg
		}
	}
	spec := &Specialization{Params: sig.TypeParams}
	for _, tp := range sig.TypeParams {
		t, ok := subst[tp]
		if !ok {
			c.errorf(args[0].Pos(), "cannot launch %s: nothing it is passed says what '%s' is", sym.Name(), tp)
			return nil, false
		}
		// A launch inside a generic function passes that function's own
		// type parameter, which is a number once the function is
		// specialized, and checked then.
		_, outer := t.(*types.TypeParam)
		if _, scalar := kernelScalarOf(t); !scalar && !outer {
			c.typeErrorf(args[0].Pos(), "cannot launch %s with '%s' as '%s': a kernel's types are numbers and bools",
				sym.Name(), t, tp)
			return nil, false
		}
		spec.Args = append(spec.Args, t)
	}
	return spec, true
}
