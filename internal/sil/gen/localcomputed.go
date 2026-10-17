package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
)

// localComputed is a computed variable declared in a function: no
// storage, only a getter -- and a setter, where it has one -- each a
// closure over what the function around it has, made where the
// variable is declared.
type localComputed struct {
	get, set *sil.Value
	typ      types.Type
}

// computedLocalDecl lowers `var x: T { get { ... } set { ... } }` in a
// function body to its accessors.
func (g *gen) computedLocalDecl(b *ast.PatternBinding) {
	_, sym := g.binding(b)
	if sym == nil {
		g.refuse(b, "a computed variable bound to a pattern other than a name")
		return
	}
	t := g.substituted(sym.Type())
	var get, set *ast.Accessor
	if b.Accessors != nil {
		for _, a := range b.Accessors.Accessors {
			if a.Keyword == nil {
				continue
			}
			switch g.text(a.Keyword) {
			case "get":
				get = a
			case "set":
				set = a
			default:
				g.refuse(a, "an observer on a variable declared in a function")
				return
			}
		}
	}
	body := b.Body
	if get != nil {
		body = get.Body
	}
	if body == nil {
		g.refuse(b, "a computed variable with no getter")
		return
	}
	lc := &localComputed{typ: t}
	getSig := &types.Signature{Results: t}
	lc.get = g.localAccessor(b, getSig, body, nil)
	if lc.get == nil {
		return
	}
	if set != nil && set.Body != nil {
		setSig := &types.Signature{
			Params:  []*types.Param{{Name: "newValue", Type: t}},
			Results: types.Typ[types.Void],
		}
		lc.set = g.localAccessor(b, setSig, set.Body, []analyzer.Symbol{g.accessorValue(set)})
		if lc.set == nil {
			return
		}
	}
	if g.computedLocals == nil {
		g.computedLocals = map[analyzer.Symbol]*localComputed{}
	}
	g.computedLocals[sym] = lc
}

// localAccessor is one accessor of a computed local, as the closure it
// is.
func (g *gen) localAccessor(b *ast.PatternBinding, sig *types.Signature, body *ast.CodeBlock, syms []analyzer.Symbol) *sil.Value {
	caps, refused := g.closureCaptures(body)
	if refused != "" {
		g.refuse(b, "a computed variable whose accessor captures '"+refused+"'")
		return nil
	}
	var x ast.Expr
	implicit := false
	if !isVoid(sig.Results) && len(body.Stmts) == 1 {
		if st, ok := body.Stmts[0].(*ast.ExprStmt); ok {
			x, implicit = st.X, true
		}
	}
	f := g.captureBody(sig, caps, syms, body.Stmts, x, implicit, body.Lbrace, body.Rbrace, "computed variable")
	if f == nil {
		return nil
	}
	return g.closureValue(f, sig, caps)
}

// computedLocalRead is a read of a computed local: a call of its getter.
func (g *gen) computedLocalRead(lc *localComputed) *sil.Value {
	v := g.blk.Apply(g.borrow(lc.get), lowerType(lc.typ))
	g.destroyLater(v)
	return v
}

// computedLocalAddr is a computed local as a destination written
// through: its value read into a temporary that is written back
// through its setter once the statement ends, as a subscript's is.
func (g *gen) computedLocalAddr(e ast.Node, lc *localComputed) *sil.Value {
	if lc.set == nil {
		g.refuse(e, "a write to a computed variable that has no setter")
		return nil
	}
	t := lowerType(lc.typ)
	cur := g.blk.Apply(g.borrow(lc.get), t)
	slot := g.blk.AllocStack(t)
	g.blk.Store(cur, slot, storeQualifier(t))
	g.writebacks = append(g.writebacks, func() {
		v := g.blk.Load(slot, loadQualifierTake(t))
		g.blk.Apply(g.borrow(lc.set), lowerType(types.Typ[types.Void]), v)
		if !t.Trivial() {
			g.blk.DestroyValue(v)
		}
		g.blk.DeallocStack(slot)
	})
	return slot
}
