package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Closures.
//
// A closure body is a function of its own, and SILGen says so: the
// body is emitted as a separate `sil private` function, and the
// expression that wrote it becomes a reference to that function turned
// into a value. What swiftc prints for `let f: (Int) -> Int = { n in n * 2 }`:
//
//	%0 = function_ref @$s2cl9nocaptureSiyFS2icfU_ : $@convention(thin) (Int) -> Int
//	%1 = thin_to_thick_function %0 to $@callee_guaranteed (Int) -> Int
//
// So a closure is two things — a function, and a value naming it —
// and this file is where the first becomes the second.
//
// # What captures would take, and why they wait
//
// A closure that captures is the same shape with the captured values
// bound in. swiftc for `{ n in n + k }` inside a function holding k:
//
//	%2 = function_ref @…U_ : $@convention(thin) (Int, Int) -> Int
//	%3 = partial_apply [callee_guaranteed] %2(%0)
//
// The captures are trailing parameters and partial_apply binds from
// the right, which is the same rule the method convention follows for
// self. What makes it more than a bigger case here is everything
// below: a thick function is then a pair of a code address and a heap
// context, the context is reference counted because the value escapes,
// and calling one means reaching a function whose parameters do not
// match what the caller holds — IRGen synthesizes a forwarder for
// exactly that. None of that is written, so a closure that captures is
// refused by name rather than lowered into a guess.
//
// While captures are refused, a function value owns nothing: its
// context is provably null, so there is no retain, no release, and no
// second word to carry. That is why lower represents one as a single
// pointer and why vil.trivial answers true for a signature. Both stop
// being true the day partial_apply is emitted, and both are written
// down where they are relied on.

// closure lowers a closure expression to a function value.
func (g *gen) closure(e *ast.ClosureExpr) *vil.Value {
	sig, _ := g.info.Types[e].Underlying().(*types.Signature)
	if sig == nil {
		g.refuse(e, "a closure whose type is not known")
		return nil
	}
	if e.Sig != nil {
		switch {
		case e.Sig.Captures != nil:
			g.refuse(e, "a closure with a capture list")
			return nil
		case e.Sig.Async.IsValid():
			g.refuse(e, "an async closure")
			return nil
		case e.Sig.Throws != nil:
			g.refuse(e, "a throwing closure")
			return nil
		}
	}
	if name, ok := g.captured(e); ok {
		g.refuse(e, "a closure that captures '"+name+"'")
		return nil
	}

	f := g.closureBody(e, sig)
	if f == nil {
		return nil
	}

	// The reference is thin — a code address, named statically — and
	// the value is thick, because a value of function type is what
	// something can be assigned to and passed. SILGen writes the
	// conversion out, so this does too, even though the context it
	// makes room for is empty until captures land.
	ref := g.blk.FunctionRef(f)
	return g.blk.ThinToThickFunction(ref, lowerType(sig))
}

// closureBody emits the closure's statements as a function of their
// own and returns it.
//
// The function is private: a closure has no name in the language, so
// nothing outside this object file could name it either.
func (g *gen) closureBody(e *ast.ClosureExpr, sig *types.Signature) *vil.Func {
	f := g.m.Func(g.closureSymbol()).SetLinkage(vil.Private).SetAttr("ossa")

	// Everything the enclosing function is in the middle of, put down
	// and picked up again. A closure is lowered where it is written,
	// which is inside another function's block, and none of that
	// bookkeeping means anything in here.
	outer := struct {
		fn      *vil.Func
		entry   bool
		blk     *vil.Block
		scopes  []*scope
		locals  map[analyzer.Symbol]*local
		loops   []loop
		pending string
		recv    types.Type
	}{g.fn, g.entry, g.blk, g.scopes, g.locals, g.loops, g.pending, g.recv}
	defer func() {
		g.fn, g.entry, g.blk = outer.fn, outer.entry, outer.blk
		g.scopes, g.locals = outer.scopes, outer.locals
		g.loops, g.pending, g.recv = outer.loops, outer.pending, outer.recv
	}()

	g.fn = f
	g.entry = false
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes = nil
	g.loops, g.pending = nil, ""
	// A closure that captures nothing has no receiver either: a bare
	// name inside it cannot mean the enclosing method's self, because
	// self would be a capture.
	g.recv = nil
	g.push()
	g.blk = f.Entry()

	syms := g.closureParams(e, sig)
	for i, p := range sig.Params {
		t := lowerType(p.Type)
		v := f.Param(t, paramConvention(p, t))
		if i < len(syms) && syms[i] != nil {
			g.locals[syms[i]] = &local{value: v, typ: t}
		}
		if p.Name != "" {
			g.blk.DebugValue(v, p.Name, "let", "argno "+itoa(i+1))
		}
		g.destroyLater(v)
	}
	if sig.Results != nil && !isVoid(sig.Results) {
		f.SetResult(lowerType(sig.Results), resultConvention(lowerType(sig.Results)))
	}

	// `{ n in n * 2 }` returns n * 2. A closure whose body is one
	// expression returns it without writing `return`, which is a rule
	// about closures rather than about statements, so it is applied
	// here and not in stmt.go.
	if x, ok := implicitResult(e, sig); ok {
		v := g.rvalue(x)
		g.unwind()
		if v == nil {
			g.blk.Unreachable()
			return f
		}
		g.blk.Return(v)
		return f
	}

	for _, st := range e.Stmts {
		g.stmt(st)
		if g.blk == nil || g.blk.Term() != nil {
			break
		}
	}
	if g.blk != nil && g.blk.Term() == nil {
		g.unwind()
		if sig.Results != nil && !isVoid(sig.Results) {
			// Falling off the end of a closure that owes a value is
			// the checker's to report; what must not happen is
			// returning something in its place.
			g.blk.Unreachable()
		} else {
			g.blk.Return(g.void())
		}
	}
	return f
}

// implicitResult is the expression a single-expression closure returns
// without saying so, or false where the body is not one.
func implicitResult(e *ast.ClosureExpr, sig *types.Signature) (ast.Expr, bool) {
	if sig.Results == nil || isVoid(sig.Results) || len(e.Stmts) != 1 {
		return nil, false
	}
	st, ok := e.Stmts[0].(*ast.ExprStmt)
	if !ok {
		return nil, false
	}
	return st.X, true
}

// closureParams is the symbol each parameter binds.
//
// Two spellings, because Swift has two. A closure that writes its
// parameters declares them, and the checker records each under its
// name; one that does not gets $0, $1 and so on, which are declared in
// the closure's scope under those names and have no syntax to be
// recorded against.
func (g *gen) closureParams(e *ast.ClosureExpr, sig *types.Signature) []analyzer.Symbol {
	out := make([]analyzer.Symbol, len(sig.Params))
	if e.Sig != nil && e.Sig.Params != nil {
		for i, p := range e.Sig.Params.Params {
			if i < len(out) && p.Name != nil {
				out[i] = g.info.Defs[p.Name]
			}
		}
		return out
	}
	scope := g.info.Scopes[e]
	if scope == nil {
		return out
	}
	for i, p := range sig.Params {
		if p.Name != "" {
			out[i] = scope.Lookup(p.Name)
		}
	}
	return out
}

// closureSymbol is the name the closure's function gets.
//
// Not swiftc's spelling. Swift mangles a closure as its enclosing
// function's symbol with a `U` discriminator appended, and reproducing
// that means reproducing where in the enclosing declaration the
// closure was written. The name only has to be stable and distinct
// within the object file — a private symbol never leaves it, so
// nothing outside can depend on which string it was — which is the
// same argument mangle.Discriminator already makes for its own hash.
func (g *gen) closureSymbol() string {
	base := "closure"
	if g.fn != nil {
		base = g.fn.Name()
	}
	g.closures++
	return base + "U" + itoa(g.closures-1) + "_"
}

// captured is the first name a closure uses from outside itself, and
// whether there was one.
//
// A name is captured when it resolves to something the enclosing
// function bound: a local, or one of its parameters. A global, a
// function, a type and the closure's own parameters are none of them —
// each is reachable from the closure's body without carrying anything
// into it, which is the whole of what a capture is.
//
// `self` counts, and is asked separately: inside a method a bare name
// may be one of the receiver's properties, and reading it captures the
// receiver just as writing `self` does.
func (g *gen) captured(e *ast.ClosureExpr) (string, bool) {
	var name string
	var found bool
	ast.Inspect(e, func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.SelfExpr:
			if g.recv != nil {
				name, found = "self", true
			}
		case *ast.IdentExpr:
			if x.Name == nil {
				return true
			}
			sym := g.info.Uses[x.Name]
			if sym != nil {
				if _, ok := g.locals[sym]; ok {
					name, found = g.text(x.Name), true
					return true
				}
			}
			// A bare name in a method body may be one of the
			// receiver's stored properties, and reading it captures
			// the receiver. The analyzer resolves such a name to the
			// symbol in the type's own scope rather than to a local,
			// so it is not caught above and has to be asked for
			// separately.
			if g.recv != nil {
				if _, ok := storedField(g.recv, g.text(x.Name)); ok {
					name, found = g.text(x.Name), true
				}
			}
		}
		return true
	})
	return name, found
}
