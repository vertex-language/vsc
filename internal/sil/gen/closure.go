package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// closure lowers a closure expression to a function value.
func (g *gen) closure(e *ast.ClosureExpr) *sil.Value {
	sig, _ := g.typeOf(e).Underlying().(*types.Signature)
	if sig == nil {
		g.refuse(e, "a closure whose type is not known")
		return nil
	}
	// A capture list of self alone says what capturing self already
	// does; anything else in one -- a weak reference, a value copied
	// when the closure is made -- is not lowered yet.
	if e.Sig != nil && e.Sig.Captures != nil && !g.selfCaptureList(e.Sig.Captures) {
		g.refuse(e, "a closure with a capture list")
		return nil
	}
	caps, refused := g.closureCaptures(e)
	if refused != "" {
		g.refuse(e, "a closure that captures '"+refused+"'")
		return nil
	}

	f := g.closureBody(e, sig, caps)
	if f == nil {
		return nil
	}
	return g.closureValue(f, sig, caps)
}

// closureValue is a function value of body over what it captures: the
// function itself where it captures nothing, and otherwise the function
// applied to copies of its captures, in a context the value owns.
func (g *gen) closureValue(f *sil.Func, sig *types.Signature, caps []closureCapture) *sil.Value {
	ref := g.blk.FunctionRef(f)
	var v *sil.Value
	if len(caps) == 0 {
		v = g.blk.ThinToThickFunction(ref, lowerType(sig))
	} else {
		// A closure that captures is its body applied to copies of what it
		// captures, which the context it makes owns.
		args := make([]*sil.Value, 0, len(caps))
		for _, c := range caps {
			if c.boxed() {
				// The box itself, so the closure and this scope see one
				// variable.
				args = append(args, g.blk.CopyValue(c.loc.box))
				continue
			}
			val := c.loc.value
			if !c.loc.typ.Trivial() {
				val = g.blk.CopyValue(val)
			}
			args = append(args, val)
		}
		v = g.blk.PartialApply(ref, lowerType(sig), args...)
	}
	// A function value owns its context, so it is destroyed when its
	// scope ends -- unless something takes it first.
	g.destroyLater(v)
	return v
}

// A closureCapture is a value a closure uses from the scope it is written in,
// or a variable, whose box it shares.
type closureCapture struct {
	sym  analyzer.Symbol
	loc  *local
	self bool // the receiver, which follows every other capture
}

// selfCaptureList reports whether a capture list names only self,
// strongly.
func (g *gen) selfCaptureList(list *ast.CaptureList) bool {
	for _, item := range list.Items {
		if _, ok := item.X.(*ast.SelfExpr); !ok || item.Spec != nil || g.recv == nil {
			return false
		}
	}
	return true
}

// boxed reports whether the capture is a variable's box.
func (c closureCapture) boxed() bool { return c.loc.box != nil }

// closureCaptures is what a closure captures -- the values of names bound in
// the scope around it, each once -- or the name of something it captures
// that cannot be captured yet: storage with nothing to share, or a self
// that is a place rather than a value -- a mutating method's.
//
// Self is captured by naming it, or by naming one of its stored
// properties or methods without it, and is captured last, so inside the
// closure it is the last argument, where a method's self is.
func (g *gen) closureCaptures(e ast.Node) ([]closureCapture, string) {
	var caps []closureCapture
	seen := map[analyzer.Symbol]bool{}
	refused := ""
	var self *sil.Value
	useSelf := func(name string) {
		if self != nil {
			return
		}
		v := g.selfValue()
		if g.self != nil || v == nil || v.Type().IsAddress() {
			refused = name
			return
		}
		self = v
	}
	ast.Inspect(e, func(n ast.Node) bool {
		if refused != "" {
			return false
		}
		switch x := n.(type) {
		case *ast.SelfExpr:
			if g.recv != nil {
				useSelf("self")
			}
		case *ast.IdentExpr:
			if x.Name == nil {
				return true
			}
			if sym := g.info.Uses[x.Name]; sym != nil {
				if l, ok := g.locals[sym]; ok {
					// A value, or a variable in a box of its own. Anything
					// else -- an inout parameter, storage on the stack, an
					// existential in memory -- has no storage to share.
					value := l.value != nil && l.addr == nil && l.box == nil && !l.mem
					variable := l.box != nil && l.addr != nil && !l.mem
					if !value && !variable {
						refused = g.text(x.Name)
						return false
					}
					if !seen[sym] {
						seen[sym] = true
						caps = append(caps, closureCapture{sym: sym, loc: l})
					}
					return true
				}
			}
			if g.recv != nil {
				if _, _, ok := storedField(g.recv, g.text(x.Name)); ok {
					useSelf(g.text(x.Name))
				} else if _, ok := g.implicitMethod(x); ok {
					useSelf(g.text(x.Name))
				}
			}
		}
		return true
	})
	if self != nil && refused == "" {
		caps = append(caps, closureCapture{loc: &local{value: self, typ: self.Type()}, self: true})
	}
	return caps, refused
}

// closureBody emits the closure's statements as a private SIL function.
func (g *gen) closureBody(e *ast.ClosureExpr, sig *types.Signature, caps []closureCapture) *sil.Func {
	x, implicit := implicitResult(e, sig)
	return g.captureBody(sig, caps, g.closureParams(e, sig), e.Stmts, x, implicit, e.Lbrace, e.Rbrace, "closure")
}

// captureBody emits a function that runs stmts with its parameters bound
// to params and what it captures after them -- a closure's body, or a
// nested function's. Where implicit, the body is the one expression x and
// returns it.
func (g *gen) captureBody(sig *types.Signature, caps []closureCapture, syms []analyzer.Symbol,
	stmts []ast.Stmt, x ast.Expr, implicit bool, start, end token.Pos, kind string) *sil.Func {
	f := g.m.Func(g.closureSymbol()).SetLinkage(sil.Private).SetAttr("ossa")

	outer := struct {
		fn      *sil.Func
		entry   bool
		blk     *sil.Block
		scopes  []*scope
		locals  map[analyzer.Symbol]*local
		loops   []loop
		pending string
		recv    types.Type
		throws  bool
		catches []catchTarget
		tryBang bool
		tryCall *ast.CallExpr
	}{g.fn, g.entry, g.blk, g.scopes, g.locals, g.loops, g.pending, g.recv,
		g.throws, g.catches, g.tryBang, g.tryCall}
	defer func() {
		g.fn, g.entry, g.blk = outer.fn, outer.entry, outer.blk
		g.scopes, g.locals = outer.scopes, outer.locals
		g.loops, g.pending, g.recv = outer.loops, outer.pending, outer.recv
		g.throws, g.catches, g.tryBang, g.tryCall = outer.throws, outer.catches, outer.tryBang, outer.tryCall
	}()

	g.fn = f
	g.entry = false
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes = nil
	g.loops, g.pending = nil, ""
	g.recv = nil
	// The closure's own errors leave it, whatever catches the code
	// around it: a catch outside a closure is not on its path.
	g.throws, g.catches, g.tryBang, g.tryCall = sig.Throws, nil, false, nil
	if sig.Throws {
		f.SetThrows(sil.Object(sil.BuiltinNativeObj))
	}
	// An async closure's type says so, as a declared function's does.
	// `Task { … }` takes an `() async -> T`, so the body it runs is an
	// async function whether or not anything in it awaits, and running
	// it as an ordinary one would call it with the wrong convention.
	f.Type().Async = sig.Async
	g.push()
	g.blk = f.Entry()

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
	// What it captures follows its parameters, borrowed from its context.
	for _, c := range caps {
		if c.boxed() {
			box := f.Param(c.loc.box.Type(), sil.ParamGuaranteed)
			addr := g.blk.ProjectBox(box, 0, c.loc.typ)
			g.locals[c.sym] = &local{addr: addr, box: box, typ: c.loc.typ}
			continue
		}
		conv := sil.ParamUnowned
		if !c.loc.typ.Trivial() {
			conv = sil.ParamGuaranteed
		}
		v := f.Param(c.loc.typ, conv)
		if c.self {
			// The body is the method's body again as far as self goes:
			// its members resolve against the receiver, and selfValue
			// finds this last argument.
			g.recv = outer.recv
			continue
		}
		g.locals[c.sym] = &local{value: v, typ: c.loc.typ}
	}
	if sig.Results != nil && !isVoid(sig.Results) {
		f.SetResult(lowerType(sig.Results), resultConvention(lowerType(sig.Results)))
	}

	// Single-expression closures return their value implicitly.
	if implicit {
		v := g.rvalue(x)
		g.unwind()
		if v == nil {
			g.blk.Unreachable()
			return f
		}
		g.blk.Return(v)
		return f
	}

	for _, st := range stmts {
		g.stmt(st)
		if g.blk == nil || g.blk.Term() != nil {
			break
		}
	}
	if g.blk != nil && g.blk.Term() == nil {
		g.unwind()
		if sig.Results != nil && !isVoid(sig.Results) {
			g.missingReturn(start, end, kind, sig.Results)
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

// closureParams returns the symbol bound by each parameter.
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

// closureSymbol generates a unique symbol name for a closure.
func (g *gen) closureSymbol() string {
	base := "closure"
	if g.fn != nil {
		base = g.fn.Name()
	}
	g.closures++
	return base + "U" + itoa(g.closures-1) + "_"
}

// captured returns the first name captured from an outer scope, if any.
func (g *gen) captured(e ast.Node) (string, bool) {
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
			// Stored property access in a method body captures self.
			if g.recv != nil {
				if _, _, ok := storedField(g.recv, g.text(x.Name)); ok {
					name, found = g.text(x.Name), true
				}
			}
		}
		return true
	})
	return name, found
}

// nestedFunc lowers a function declared inside another one.
func (g *gen) nestedFunc(d *ast.FuncDecl) {
	if _, ok := g.captured(d); ok {
		g.capturingNestedFunc(d)
		return
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
		throws  bool
		catches []catchTarget
		tryBang bool
		tryCall *ast.CallExpr
	}{g.fn, g.entry, g.blk, g.scopes, g.locals, g.loops, g.pending, g.recv,
		g.throws, g.catches, g.tryBang, g.tryCall}
	defer func() {
		g.fn, g.entry, g.blk = outer.fn, outer.entry, outer.blk
		g.scopes, g.locals = outer.scopes, outer.locals
		g.loops, g.pending, g.recv = outer.loops, outer.pending, outer.recv
		g.throws, g.catches, g.tryBang, g.tryCall = outer.throws, outer.catches, outer.tryBang, outer.tryCall
	}()

	prevLocal := g.localFunc
	g.localFunc = true
	g.function(d, nil)
	g.localFunc = prevLocal
}

// capturingNestedFunc lowers a nested function that uses names from the
// function around it.
//
// Such a function is a closure in all but syntax -- Swift captures for it
// exactly what a closure written in the same place would -- so it is built
// as one, where it is declared, and its name is bound to the function
// value. A call of the name is then a call through that value.
func (g *gen) capturingNestedFunc(d *ast.FuncDecl) {
	sym, _ := g.info.Defs[d.Name].(*analyzer.FuncSymbol)
	if sym == nil || d.Body == nil {
		g.refuse(d, "a nested function this compiler did not resolve")
		return
	}
	if d.Generics != nil {
		g.refuse(d, "a generic nested function that captures")
		return
	}
	sig := sym.Signature()
	if callsItself(d.Body, sym, g.info) {
		g.refuse(d, "a nested function that captures and calls itself")
		return
	}
	caps, refused := g.closureCaptures(d.Body)
	if refused != "" {
		g.refuse(d, "a nested function that captures '"+refused+"'")
		return
	}
	syms := paramSymbols(d, g.info, g.file)
	var x ast.Expr
	implicit := false
	if sig.Results != nil && !isVoid(sig.Results) && len(d.Body.Stmts) == 1 {
		if st, ok := d.Body.Stmts[0].(*ast.ExprStmt); ok {
			x, implicit = st.X, true
		}
	}
	f := g.captureBody(sig, caps, syms, d.Body.Stmts, x, implicit, d.Body.Lbrace, d.Body.Rbrace, "local function")
	if f == nil {
		return
	}
	v := g.closureValue(f, sig, caps)
	g.locals[sym] = &local{value: v, typ: lowerType(sig)}
}

// callsItself reports whether a body names the function it belongs to.
func callsItself(body *ast.CodeBlock, sym analyzer.Symbol, info *analyzer.Info) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if id, ok := n.(*ast.IdentExpr); ok && id.Name != nil && info.Uses[id.Name] == sym {
			found = true
		}
		return !found
	})
	return found
}

// declKind names a declaration for a refusal.
func declKind(d ast.Decl) string {
	switch d.(type) {
	case *ast.StructDecl:
		return "a struct declared inside a function"
	case *ast.ClassDecl:
		return "a class declared inside a function"
	case *ast.EnumDecl:
		return "an enum declared inside a function"
	case *ast.ProtocolDecl:
		return "a protocol declared inside a function"
	case *ast.TypealiasDecl:
		return "a typealias declared inside a function"
	case *ast.ExtensionDecl:
		return "an extension declared inside a function"
	}
	return "this declaration inside a function"
}

// registerNested records every function declared directly in a body as enclosed by enclosing.
func (g *gen) registerNested(body *ast.CodeBlock, enclosing string) {
	if body == nil {
		return
	}
	for _, st := range body.Stmts {
		decl, ok := st.(*ast.DeclStmt)
		if !ok {
			continue
		}
		fd, ok := decl.D.(*ast.FuncDecl)
		if !ok || fd.Name == nil {
			continue
		}
		sym, ok := g.info.Defs[fd.Name].(*analyzer.FuncSymbol)
		if !ok {
			continue
		}
		if g.nested == nil {
			g.nested = map[analyzer.Symbol]string{}
		}
		g.nested[sym] = enclosing
	}
}
