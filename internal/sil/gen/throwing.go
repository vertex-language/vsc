package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// Errors: throw, try, and do/catch.
//
// A thrown value is an `any Error`, moved into a runtime box
// (vertex_error_box) whose pointer is what a function that may fail hands
// back in the error register. try_apply gives a call two edges; on the one
// that failed, the box goes to the innermost enclosing catch -- after the
// cleanups and defers of the scopes it leaves -- or out of the function with
// `throw`. A catch copies the `any Error` out of the box and releases it.

// catchTarget is an enclosing do/catch: the block a throw in its body goes
// to, taking the box, and how many scopes were open outside the body, whose
// cleanups a throw into it does not run.
type catchTarget struct {
	dispatch *sil.Block
	depth    int
}

// errorExistential is `any Error`, what a thrown value becomes.
func errorExistential() *types.Existential {
	return &types.Existential{Protocols: []*types.Protocol{types.ErrorProtocol}}
}

func errorBoxType() sil.Type   { return sil.Object(sil.BuiltinNativeObj) }
func rawPointerType() sil.Type { return sil.Object(sil.BuiltinRawPointer) }

// runtimeResult calls a runtime entry point that takes arguments and
// returns a value.
func (g *gen) runtimeResult(symbol string, params []sil.Param, result sil.Type, args ...*sil.Value) *sil.Value {
	callee := g.m.Func(symbol).SetSourceName(symbol)
	if g.needsType(callee) {
		callee.SetLinkage(sil.PublicExternal)
		callee.Type().Convention = sil.Thin
		// A runtime call that gives up the thread is an async function,
		// and saying so is the whole of what makes calling it a
		// suspension: the split does the rest.
		callee.Type().Async = stdlib.Suspends(symbol)
		callee.Type().Params = params
		callee.SetResult(result, resultConvention(result))
	}
	return g.blk.Apply(g.blk.FunctionRef(callee), result, args...)
}

// boxError makes a thrown value the box the error register carries.
func (g *gen) boxError(at ast.Node, v *sil.Value, from types.Type) *sil.Value {
	if ex, already := existentialOf(from); already {
		// Rethrowing what a catch bound: the existential is copied out of
		// where it is into a container of its own, which the box takes.
		if len(ex.Protocols) != 1 || ex.Protocols[0] != types.ErrorProtocol {
			g.refuse(at, "throwing an existential other than 'any Error'")
			return nil
		}
		slot := g.blk.AllocStack(lowerType(errorExistential()))
		if v.Type().IsAddress() {
			g.blk.CopyAddr(v, slot, "init")
		} else {
			// One held as a value -- a Result's failure -- is its words.
			if v.Ownership() == sil.Owned {
				v = g.consume(v)
			} else {
				v = g.blk.CopyValue(v)
			}
			g.blk.Store(v, slot, "init")
		}
		ptr := g.blk.AddressToPointer(slot, rawPointerType())
		return g.runtimeResult(stdlib.ErrorBox,
			[]sil.Param{{Type: rawPointerType(), Convention: sil.ParamUnowned}},
			errorBoxType(), ptr)
	}
	before := len(g.diags)
	slot := g.existentialFor(at, v, from, errorExistential())
	if slot == nil || slot == v {
		if len(g.diags) == before {
			g.errorAt(at, "cannot throw a value of type '"+typeNameOf(from)+
				"': it does not conform to 'Error'")
		}
		return nil
	}
	ptr := g.blk.AddressToPointer(slot, rawPointerType())
	return g.runtimeResult(stdlib.ErrorBox,
		[]sil.Param{{Type: rawPointerType(), Convention: sil.ParamUnowned}},
		errorBoxType(), ptr)
}

// raise sends an error box out of the current point: to the innermost
// enclosing catch, or out of the function when it throws.
func (g *gen) raise(at ast.Node, box *sil.Value) {
	if len(g.catches) > 0 {
		t := g.catches[len(g.catches)-1]
		g.unwindTo(t.depth)
		if g.blk != nil && g.blk.Term() == nil {
			g.blk.Br(t.dispatch, box)
		}
		return
	}
	// main.swift's code may throw, as though in a throwing function;
	// what reaches the top is reported, and the program traps.
	if !g.throws && g.topLevel != nil && g.fn == g.topLevel {
		g.unwind()
		if g.blk != nil && g.blk.Term() == nil {
			g.runtimeResult(stdlib.ErrorInMain,
				[]sil.Param{{Type: errorBoxType(), Convention: sil.ParamOwned}},
				lowerType(types.Typ[types.Void]), box)
			g.blk.Unreachable()
		}
		return
	}
	if !g.throws {
		g.errorAt(at, "errors thrown from here are not handled: the enclosing "+
			"function is not declared 'throws'")
		g.blk.Unreachable()
		return
	}
	g.unwind()
	if g.blk != nil && g.blk.Term() == nil {
		g.blk.Throw(box)
	}
}

// throwStmt lowers `throw value`.
func (g *gen) throwStmt(s *ast.ThrowStmt) {
	v := g.rvalue(s.X)
	if v == nil {
		return
	}
	box := g.boxError(s.X, v, g.typeOf(s.X))
	if box == nil {
		return
	}
	g.raise(s, box)
}

// tryApply emits the two-edged call. Under try? the result is an optional
// and the error is released; under try! a failure traps; otherwise the error
// is raised where the call is.
func (g *gen) tryApply(e *ast.CallExpr, callee *sil.Value, args []*sil.Value,
	result types.Type, optional, imported bool) *sil.Value {

	bang := g.tryBang
	g.tryBang = false

	if optional {
		if isVoid(result) {
			// Nothing to make optional: both edges go on, the failed one
			// letting its error go.
			normal := g.fn.Block()
			failed := g.fn.Block()
			join := g.fn.Block()
			var box *sil.Value
			if !imported {
				box = failed.Arg(errorBoxType(), sil.Owned)
			}
			g.blk.TryApply(callee, normal, failed, args...)
			g.blk = failed
			if box != nil {
				g.blk.DestroyValue(box)
			}
			g.blk.Br(join)
			g.blk = normal
			g.blk.Br(join)
			g.blk = join
			return g.void()
		}
		opt := &types.Optional{Wrapped: result}
		normal := g.fn.Block()
		failed := g.fn.Block()
		join := g.fn.Block()
		value := normal.Arg(lowerType(result), sil.Owned)
		made := join.Arg(lowerType(opt), sil.Owned)
		// A box Vertex made is released here; one a module swiftc built
		// threw is Swift's, and is left alone.
		var box *sil.Value
		if !imported {
			box = failed.Arg(errorBoxType(), sil.Owned)
		}

		g.blk.TryApply(callee, normal, failed, args...)

		g.blk = normal
		some := g.blk.Enum(lowerType(opt), optionalSome, value)
		g.blk.Br(join, some)

		g.blk = failed
		if box != nil {
			g.blk.DestroyValue(box)
		}
		none := g.blk.Enum(lowerType(opt), optionalNone, nil)
		g.blk.Br(join, none)

		g.blk = join
		return made
	}

	if imported {
		g.refuse(e, "'try' on a call into a module swiftc built: its error box is "+
			"Swift's, and a catch here opens Vertex's")
		return nil
	}

	normal := g.fn.Block()
	failed := g.fn.Block()
	var value *sil.Value
	if !isVoid(result) {
		value = normal.Arg(lowerType(result), sil.Owned)
	}
	box := failed.Arg(errorBoxType(), sil.Owned)

	g.blk.TryApply(callee, normal, failed, args...)

	g.blk = failed
	if bang {
		// The runtime says what was raised, as Swift's does, and traps.
		g.runtimeResult(stdlib.TryFailed,
			[]sil.Param{{Type: errorBoxType(), Convention: sil.ParamOwned}},
			lowerType(types.Typ[types.Void]), box)
		g.blk.Unreachable()
	} else {
		g.raise(e, box)
	}

	g.blk = normal
	if value == nil {
		return g.void()
	}
	g.destroyLater(value)
	return value
}

// tryExpr lowers try, try? and try!.
func (g *gen) tryExpr(e *ast.TryExpr) *sil.Value {
	optional := e.Question != token.NoPos
	bang := e.Exclaim != token.NoPos
	if !optional && !bang {
		// Plain try covers the expression; each call in it that may fail
		// raises its error where it is.
		return g.expr(e.X)
	}
	keyword := "'try?'"
	if bang {
		keyword = "'try!'"
	}
	// `try? await f()` is a try on the call the await is written on.
	x := e.X
	for {
		if aw, isAwait := x.(*ast.AwaitExpr); isAwait {
			x = aw.X
			continue
		}
		if p, isParen := x.(*ast.ParenExpr); isParen {
			x = p.X
			continue
		}
		break
	}
	call, ok := x.(*ast.CallExpr)
	if !ok || g.throwsInside(call) {
		return g.tryScope(e, x, optional)
	}
	// A function named through its module, `time.Sleep(…)`, is the
	// function, as a bare name is.
	var name *ast.Ident
	switch f := call.Fun.(type) {
	case *ast.IdentExpr:
		name = f.Name
	case *ast.MemberExpr:
		if _, isFunc := g.info.Uses[f.Name].(*analyzer.FuncSymbol); isFunc {
			name = f.Name
		}
	}
	if name == nil {
		// A method: the call takes the keyword when it reaches its apply,
		// if it is one that may fail.
		g.tryCall, g.tryOptional, g.tryTrap = call, optional, bang
		v := g.expr(call)
		if g.tryCall == call {
			g.tryCall, g.tryOptional, g.tryTrap = nil, false, false
			if v != nil {
				g.refuse(e, keyword+" on a call this compiler cannot name")
			}
			return nil
		}
		return v
	}
	sym, _ := g.info.Uses[name].(*analyzer.FuncSymbol)
	if sym == nil {
		g.refuse(e, keyword+" on a call this compiler could not resolve")
		return nil
	}
	g.tryBang = bang
	v := g.callFuncTry(call, sym, optional)
	g.tryBang = false
	return v
}

// throwsInside reports whether a call's arguments make a call that may
// fail: `try? half(half(8))` covers both.
func (g *gen) throwsInside(call *ast.CallExpr) bool {
	if call.Args == nil {
		return false
	}
	found := false
	for _, a := range call.Args.Args {
		ast.Inspect(a.X, func(n ast.Node) bool {
			if found {
				return false
			}
			if _, isClosure := n.(*ast.ClosureExpr); isClosure {
				return false
			}
			if c, isCall := n.(*ast.CallExpr); isCall {
				if sig, ok := g.typeOf(c.Fun).(*types.Signature); ok && sig.Throws {
					found = true
				}
			}
			return !found
		})
	}
	return found
}

// tryScope lowers `try? x` or `try! x` for any x: x is lowered as a do
// block's body would be, each call in it that fails going to a catch of
// its own. Under try? that catch lets the error go and makes nil, and x's
// value is made optional -- once: `try?` on a T? is a T?, as SE-0230 has
// it. Under try! it reports the error and traps.
func (g *gen) tryScope(e *ast.TryExpr, x ast.Expr, optional bool) *sil.Value {
	dispatch := g.fn.Block()
	box := dispatch.Arg(errorBoxType(), sil.Owned)
	g.catches = append(g.catches, catchTarget{dispatch: dispatch, depth: len(g.scopes)})
	v := g.rvalue(x)
	g.catches = g.catches[:len(g.catches)-1]
	if v == nil {
		g.fn.RemoveBlock(dispatch)
		return nil
	}
	if len(dispatch.Preds()) == 0 {
		g.fn.RemoveBlock(dispatch)
		if optional {
			v = g.optionalFor(x, v, g.typeOf(x), g.typeOf(e))
		}
		g.destroyLater(v)
		return v
	}
	if !optional {
		after := g.blk
		g.blk = dispatch
		g.runtimeResult(stdlib.TryFailed,
			[]sil.Param{{Type: errorBoxType(), Convention: sil.ParamOwned}},
			lowerType(types.Typ[types.Void]), box)
		g.blk.Unreachable()
		g.blk = after
		g.destroyLater(v)
		return v
	}
	want := g.substituted(g.typeOf(e))
	some := g.optionalFor(x, v, g.typeOf(x), want)
	join := g.fn.Block()
	made := join.Arg(lowerType(want), sil.Owned)
	g.blk.Br(join, some)
	g.blk = dispatch
	g.blk.DestroyValue(box)
	g.blk.Br(join, g.blk.Enum(lowerType(want), optionalNone, nil))
	g.blk = join
	g.destroyLater(made)
	return made
}

// doCatch lowers `do { … } catch { … }`.
func (g *gen) doCatch(s *ast.DoStmt) {
	dispatch := g.fn.Block()
	box := dispatch.Arg(errorBoxType(), sil.Owned)

	g.catches = append(g.catches, catchTarget{dispatch: dispatch, depth: len(g.scopes)})
	g.block(s.Body)
	g.catches = g.catches[:len(g.catches)-1]

	var after *sil.Block
	join := func() *sil.Block {
		if after == nil {
			after = g.fn.Block()
		}
		return after
	}
	if g.blk != nil && g.blk.Term() == nil {
		g.blk.Br(join())
	}
	// A body that throws nothing reaches no catch: Swift checks them and
	// warns, and there is nothing of them to run.
	if len(dispatch.Preds()) == 0 {
		g.fn.RemoveBlock(dispatch)
		if after != nil {
			g.blk = after
		}
		return
	}

	g.blk = dispatch
	caught := false
	for _, cl := range s.Catches {
		all, ok := g.catchClause(cl, box, join)
		if !ok {
			g.blk.DestroyValue(box)
			g.blk.Unreachable()
			caught = true
			break
		}
		// A clause that names the error catches all of it: any after it
		// are never reached.
		if all {
			caught = true
			break
		}
	}
	if !caught {
		g.raise(s, box)
	}
	if after != nil {
		g.blk = after
	}
}

// catchClause lowers one catch clause at g.blk, where box holds the error.
// all is true for a clause that catches every error. One that tests the
// error -- `is T`, `let e as T`, `E.case(let x)` -- leaves g.blk on the
// path where the test failed, with box still held, for the next clause.
func (g *gen) catchClause(cl *ast.CatchClause, box *sil.Value, join func() *sil.Block) (all, ok bool) {
	if len(cl.Items) > 1 {
		g.refuse(cl, "a catch clause with more than one pattern")
		return false, false
	}
	if len(cl.Items) == 1 && cl.Items[0].Where != nil {
		g.refuse(cl, "a catch clause with a where clause")
		return false, false
	}
	if sym, name, isName := g.catchBinding(cl); isName {
		// A body that throws one type binds `error` as that type: a copy
		// out of the box, which is let go.
		if sym != nil && len(cl.Items) == 0 {
			if t := sym.Type(); t != nil {
				if _, isEx := existentialOf(t); !isEx {
					g.push()
					lt := lowerType(t)
					p := g.runtimeResult(stdlib.ErrorProject,
						[]sil.Param{{Type: errorBoxType(), Convention: sil.ParamGuaranteed}},
						rawPointerType(), box)
					bound := g.blk.MoveValue(g.blk.Load(g.blk.PointerToAddress(p, lt.Address()), loadQualifier(lt)), "lexical", "var_decl")
					g.blk.DebugValue(bound, name, "let")
					g.blk.DestroyValue(box)
					g.destroyLater(bound)
					g.locals[sym] = &local{value: bound, typ: lt}
					g.catchBody(cl, join)
					return true, true
				}
			}
		}
		g.push()
		t := lowerType(errorExistential())
		slot := g.blk.AllocStackFor(t, name, "let")
		contents := g.runtimeResult(stdlib.ErrorContents,
			[]sil.Param{{Type: errorBoxType(), Convention: sil.ParamGuaranteed}},
			rawPointerType(), box)
		g.blk.CopyAddr(g.blk.PointerToAddress(contents, t.Address()), slot, "init")
		g.blk.DestroyValue(box)
		g.destroyAddrLater(slot)
		if sym != nil {
			g.locals[sym] = &local{addr: slot, typ: t, mem: true}
		}
		g.catchBody(cl, join)
		return true, true
	}

	pat := cl.Items[0].Pat
	if vb, isBinding := pat.(*ast.ValueBindingPattern); isBinding {
		pat = vb.Pat
	}
	t := g.info.PatternTypes[pat]
	if t == nil {
		g.refuse(pat, "this pattern in a catch clause")
		return false, false
	}
	meta, ok := g.stdlibMetadata(pat, t)
	if !ok {
		return false, false
	}
	matched := g.runtimeResult(stdlib.ErrorMatches,
		[]sil.Param{
			{Type: errorBoxType(), Convention: sil.ParamGuaranteed},
			{Type: rawPointerType(), Convention: sil.ParamUnowned},
		},
		sil.Object(sil.BuiltinInt64), box, meta)
	bit := g.blk.Builtin("cmp_ne_Int64", sil.Object(sil.BuiltinInt1), matched,
		g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt64), 0))
	yes, next := g.fn.Block(), g.fn.Block()
	g.blk.CondBr(bit, yes, nil, next, nil)

	g.blk = yes
	lt := lowerType(t)
	project := func() *sil.Value {
		p := g.runtimeResult(stdlib.ErrorProject,
			[]sil.Param{{Type: errorBoxType(), Convention: sil.ParamGuaranteed}},
			rawPointerType(), box)
		return g.blk.PointerToAddress(p, lt.Address())
	}
	switch p := pat.(type) {
	case *ast.IsPattern:
		g.blk.DestroyValue(box)
		g.push()

	case *ast.AsPattern:
		g.push()
		sub := p.Pat
		if vb, isBinding := sub.(*ast.ValueBindingPattern); isBinding {
			sub = vb.Pat
		}
		switch id := sub.(type) {
		case *ast.WildcardPattern:
		case *ast.IdentPattern:
			sym := g.info.Defs[id.Name]
			name := id.Name.Text(g.file)
			if _, isEx := existentialOf(t); isEx {
				// An existential is held in memory, as a declared one is.
				slot := g.blk.AllocStackFor(lt, name, "let")
				g.blk.CopyAddr(project(), slot, "init")
				g.destroyAddrLater(slot)
				if sym != nil {
					g.locals[sym] = &local{addr: slot, typ: lt, mem: true}
				}
				break
			}
			// Anything else is a copy out of the box, bound as a let is.
			bound := g.blk.MoveValue(g.blk.Load(project(), loadQualifier(lt)), "lexical", "var_decl")
			g.blk.DebugValue(bound, name, "let")
			g.destroyLater(bound)
			if sym != nil {
				g.locals[sym] = &local{value: bound, typ: lt}
			}
		default:
			g.refuse(sub, "this pattern before 'as' in a catch clause")
			g.scopes = g.scopes[:len(g.scopes)-1]
			return false, false
		}
		g.blk.DestroyValue(box)

	case *ast.EnumCasePattern:
		// The test comes before the box is let go, so that a case this
		// clause does not match still has the error for the next one.
		//
		// What is switched on is a copy out of the box. Where the case
		// matches, what it carries leaves the copy for the clause's
		// names and is let go with the clause; where it does not, the
		// copy is let go on the way to the next clause, and the box --
		// untouched -- goes on to it.
		v := g.blk.Load(project(), loadQualifier(lt))
		body, miss := g.fn.Block(), g.fn.Block()
		if p.Args != nil && !g.bindCasePayload(t, p, body) {
			return false, false
		}
		g.blk.SwitchEnum(v,
			sil.Case{Member: memberName(t, g.text(p.Name)), Dest: body},
			sil.Case{Dest: miss})
		if !lt.Trivial() {
			// The default edge is handed the copy back, as OSSA's
			// switch_enum hands every edge what it consumed.
			miss.DestroyValue(miss.Arg(lt, sil.Owned))
		}
		miss.Br(next)
		g.blk = body
		g.blk.DestroyValue(box)
		g.push()
		g.destroyArmOwned(body)

	default:
		g.refuse(pat, "this pattern in a catch clause")
		return false, false
	}
	g.catchBody(cl, join)
	g.blk = next
	return false, true
}

// catchBody lowers a clause's body in the scope its bindings were pushed
// in, and goes on to what follows the do statement.
func (g *gen) catchBody(cl *ast.CatchClause, join func() *sil.Block) {
	g.block(cl.Body)
	g.popReachable()
	if g.blk != nil && g.blk.Term() == nil {
		g.blk.Br(join())
	}
}

// catchBinding is the name a catch clause binds the error to, and its
// symbol: `error` for a bare catch, or the name of `catch let e`. ok is
// false for a clause whose pattern tests the error.
func (g *gen) catchBinding(cl *ast.CatchClause) (analyzer.Symbol, string, bool) {
	if len(cl.Items) == 0 {
		var sym analyzer.Symbol
		if sc := g.info.Scopes[cl]; sc != nil {
			sym = sc.LookupLocal("error")
		}
		return sym, "error", true
	}
	if len(cl.Items) != 1 || cl.Items[0].Where != nil {
		return nil, "", false
	}
	pat := cl.Items[0].Pat
	if vb, isBinding := pat.(*ast.ValueBindingPattern); isBinding {
		pat = vb.Pat
	}
	id, isIdent := pat.(*ast.IdentPattern)
	if !isIdent || id.Name == nil {
		return nil, "", false
	}
	return g.info.Defs[id.Name], id.Name.Text(g.file), true
}

// tryOn is what a `try?` or `try!` asked of this call: whether its result
// is an optional, and whether a failure traps. Neither, for a call no
// keyword was written on.
func (g *gen) tryOn(e *ast.CallExpr) (optional, trap bool) {
	if g.tryCall != e {
		return false, false
	}
	optional, trap = g.tryOptional, g.tryTrap
	g.tryCall, g.tryOptional, g.tryTrap = nil, false, false
	return optional, trap
}

// throwingCall takes a pending `try?` or `try!` for a call of a function
// that may fail, and reports whether its result is made optional. A
// rethrows function given nothing that throws cannot fail, so nothing
// has to catch it: its error edge is never taken, and traps if it is.
func (g *gen) throwingCall(e *ast.CallExpr, sig *types.Signature) bool {
	optional, trap := g.tryOn(e)
	g.tryBang = trap
	if sig != nil && sig.Rethrows && !optional && !g.argumentThrows(e) {
		g.tryBang = true
	}
	return optional
}

// argumentThrows reports whether any argument of a call is a function
// that may throw, which is what makes a call to a rethrows function one
// that may fail.
func (g *gen) argumentThrows(e *ast.CallExpr) bool {
	for _, tc := range e.Trailing {
		if tc != nil && tc.Closure != nil && closureMayThrow(tc.Closure) {
			return true
		}
	}
	if e.Args == nil {
		return false
	}
	for _, a := range e.Args.Args {
		if a == nil || a.X == nil {
			continue
		}
		// A closure written in the call throws only if it can: it
		// takes a throwing type from the parameter either way. So does
		// the one a method or initializer named as a value is.
		x := a.X
		if r, ok := g.info.ImplicitSelf[x]; ok {
			x = r
		}
		if cl, isClosure := x.(*ast.ClosureExpr); isClosure {
			if closureMayThrow(cl) {
				return true
			}
			continue
		}
		if t := g.typeOf(a.X); t != nil {
			if sig, ok := t.Underlying().(*types.Signature); ok && sig.Throws {
				return true
			}
		}
	}
	return false
}

// closureMayThrow reports whether a closure can fail: it says `throws`, or
// its own body -- not a closure or function inside it -- throws or has a
// plain `try`.
func closureMayThrow(cl *ast.ClosureExpr) bool {
	if cl.Sig != nil && cl.Sig.Throws != nil {
		return true
	}
	may := false
	for _, st := range cl.Stmts {
		ast.Inspect(st, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.ClosureExpr, *ast.FuncDecl:
				return false
			case *ast.ThrowStmt:
				may = true
			case *ast.TryExpr:
				if x.Question == token.NoPos && x.Exclaim == token.NoPos {
					may = true
				}
			}
			return !may
		})
		if may {
			return true
		}
	}
	return false
}
