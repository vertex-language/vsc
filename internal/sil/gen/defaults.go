package gen

import (
	"path/filepath"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// arguments returns call argument values in parameter order, filling in defaults.
func (g *gen) arguments(e *ast.CallExpr, sig *types.Signature) ([]*sil.Value, bool) {
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	if sig == nil {
		return nil, false
	}
	if i, ok := variadicIndex(sig); ok {
		return g.variadicArguments(e, args, sig, i)
	}
	if len(args) == len(sig.Params) {
		vals := g.argumentValues(args, sig)
		if vals == nil && len(args) > 0 {
			return nil, false
		}
		return vals, true
	}
	if len(args) > len(sig.Params) {
		return nil, false
	}

	want := existentialParams(sig)
	out := make([]*sil.Value, 0, len(sig.Params))
	next := 0
	for i, p := range sig.Params {
		if next < len(args) && g.argFits(args[next], p) && !(p.HasDefault && closureSkips(args[next], p)) {
			v := g.expr(args[next].X)
			if v == nil {
				return nil, false
			}
			boxed := g.boxArg(args[next].X, v, want, i)
			if boxed == nil {
				return nil, false
			}
			out = append(out, boxed)
			next++
			continue
		}
		def := g.defaultOf(p)
		if !p.HasDefault || def == nil {
			g.refuse(e, "a call leaving out '"+p.Name+"', whose default this cannot reach")
			return nil, false
		}
		v := g.defaultValue(e, def)
		if v == nil {
			return nil, false
		}
		boxed := g.boxArg(def, v, want, i)
		if boxed == nil {
			return nil, false
		}
		out = append(out, boxed)
	}
	if next != len(args) {
		g.refuse(e, "a call whose arguments this could not match to parameters")
		return nil, false
	}
	return out, true
}

// argumentValues is the straightforward case: one argument per
// parameter, in order.
func (g *gen) argumentValues(args []*ast.CallArg, sig *types.Signature) []*sil.Value {
	want := existentialParams(sig)
	out := make([]*sil.Value, 0, len(args))
	for i, a := range args {
		v := g.expr(a.X)
		if v == nil {
			return nil
		}
		boxed := g.boxArg(a.X, v, want, i)
		if boxed == nil {
			return nil
		}
		out = append(out, boxed)
	}
	return out
}

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
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
		if label == "" {
			return true
		}
		if _, ok := unparen(a.X).(*ast.ClosureExpr); ok && p.Type != nil {
			if _, isFunc := p.Type.Underlying().(*types.Signature); isFunc {
				return true
			}
		}
		return false
	}
	return written == label
}

// closureSkips reports whether an unlabelled closure passes over a
// parameter that takes no function, as a trailing closure does.
func closureSkips(a *ast.CallArg, p *types.Param) bool {
	if a.Label != nil || p.Type == nil {
		return false
	}
	if _, ok := unparen(a.X).(*ast.ClosureExpr); !ok {
		return false
	}
	t := p.Type.Underlying()
	if o, ok := t.(*types.Optional); ok && o.Wrapped != nil {
		t = o.Wrapped.Underlying()
	}
	_, isFunc := t.(*types.Signature)
	return !isFunc
}

// defaultValue lowers a constant default argument expression at the call site.
func (g *gen) defaultValue(at ast.Node, e ast.Expr) *sil.Value {
	if v, ok := g.callSiteMagic(at, e); ok {
		return v
	}
	if v := g.constant(e); v != nil {
		return v
	}
	if lit, ok := e.(*ast.StringLit); ok {
		if v := g.stringLiteral(lit); v != nil {
			return v
		}
		return nil
	}
	// Anything else is evaluated where the call is, read out of the file
	// it was written in. A default of an imported function has no file
	// here: Swift calls a generator for it, which this does not name yet.
	f := g.fileOf(e)
	if f == nil {
		// A default of a function another Vertex module declares: its
		// expression was checked in that module's file.
		f = g.info.DefaultFiles[e]
	}
	if f != nil {
		saved := g.file
		g.file = f
		v := g.expr(e)
		g.file = saved
		return v
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

// variadicArguments lowers call arguments when a variadic parameter collects trailing items into an array.
func (g *gen) variadicArguments(e *ast.CallExpr, args []*ast.CallArg,
	sig *types.Signature, vi int) ([]*sil.Value, bool) {
	want := existentialParams(sig)
	out := make([]*sil.Value, 0, len(sig.Params))

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
		boxed := g.boxArg(args[next].X, v, want, i)
		if boxed == nil {
			return nil, false
		}
		out = append(out, boxed)
		next++
	}

	// The list takes every argument written without a label of its
	// own, which is what stops it before a `separator:`.
	last := sig.Params[vi]
	elems := make([]*sil.Value, 0, len(args)-next)
	for first := true; next < len(args); next++ {
		// The first by the list's label, the rest by none.
		if (first && !g.argFits(args[next], last)) || (!first && args[next].Label != nil) {
			break
		}
		first = false
		from := g.typeOf(args[next].X)
		var v *sil.Value
		// print shows a CustomStringConvertible value by its description,
		// which is read here, where the type is still known.
		if f, ok := g.descriptionOf(from); ok && g.callsCorePrint(e) {
			x := args[next].X
			v = g.getterCall(x, from, f, func() *sil.Value { return g.expr(x) })
			from = types.Typ[types.String]
		} else {
			v = g.rvalue(args[next].X)
		}
		if v == nil {
			return nil, false
		}
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
			boxed := g.boxArg(args[next].X, v, want, i)
			if boxed == nil {
				return nil, false
			}
			out = append(out, boxed)
			next++
			continue
		}
		def := g.defaultOf(p)
		if !p.HasDefault || def == nil {
			g.refuse(e, "a call leaving out '"+p.Name+"', whose default this cannot reach")
			return nil, false
		}
		v := g.defaultValue(e, def)
		if v == nil {
			return nil, false
		}
		boxed := g.boxArg(def, v, want, i)
		if boxed == nil {
			return nil, false
		}
		out = append(out, boxed)
	}
	if next != len(args) {
		g.refuse(e, "a call whose arguments this could not match to parameters")
		return nil, false
	}
	return out, true
}

// callsCorePrint reports whether a call is to core's print.
func (g *gen) callsCorePrint(e *ast.CallExpr) bool {
	id, ok := e.Fun.(*ast.IdentExpr)
	if !ok || id.Name == nil {
		return false
	}
	sym, ok := g.info.Uses[id.Name].(*analyzer.FuncSymbol)
	return ok && sym.Name() == "print" && g.info.Imported[sym] == "Swift"
}

// defaultOf is the expression a parameter's default is, following a
// substituted parameter back to the one it was made from.
func (g *gen) defaultOf(p *types.Param) ast.Expr {
	for ; p != nil; p = p.Origin {
		if def := g.info.Defaults[p]; def != nil {
			return def
		}
	}
	return nil
}

// callSiteMagic is a default of #line, #column, #function or a #file form
// as the call leaving it out sees it: Swift evaluates those where the
// call is, not where the parameter is declared.
func (g *gen) callSiteMagic(at ast.Node, e ast.Expr) (*sil.Value, bool) {
	m, ok := e.(*ast.MagicLit)
	if !ok {
		return nil, false
	}
	call, ok := at.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	site, ok := g.info.CallSites[call]
	if !ok {
		return nil, false
	}
	switch m.Kind {
	case token.POUND_LINE, token.POUND_COLUMN:
		t := g.typeOf(m)
		if t == nil || !types.Identical(t.Underlying(), types.Typ[types.Int]) {
			return nil, false
		}
		p := g.file.Position(site.Pos)
		n := p.Line
		if m.Kind == token.POUND_COLUMN {
			n = p.Column
		}
		raw := g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt64), int64(n))
		return g.blk.Struct(lowerType(t), raw), true
	case token.POUND_FUNCTION:
		return g.stringValue(m, site.Func), true
	case token.POUND_FILEID:
		return g.stringValue(m, g.module+"/"+filepath.Base(g.file.Name())), true
	case token.POUND_FILE, token.POUND_FILEPATH:
		return g.stringValue(m, g.file.Name()), true
	}
	return nil, false
}
