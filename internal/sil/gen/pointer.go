package gen

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
)

// pointerOf is the unsafe pointer a type is, if it is one.
func pointerOf(t types.Type) (*types.Pointer, bool) {
	if t == nil {
		return nil, false
	}
	p, ok := t.Underlying().(*types.Pointer)
	return p, ok
}

// isPointee reports whether a member expression is `p.pointee` on a
// pointer, and hands back the pointer it is on.
func (g *gen) isPointee(e *ast.MemberExpr) (*types.Pointer, bool) {
	if e == nil || e.Name == nil || g.text(e.Name) != "pointee" {
		return nil, false
	}
	p, ok := pointerOf(g.typeOf(e.X))
	if !ok || !p.Dereferenceable() {
		return nil, false
	}
	return p, true
}

// pointeeAddr returns the address for p.pointee cast to the element type's address.
func (g *gen) pointeeAddr(e *ast.MemberExpr, p *types.Pointer) *sil.Value {
	v := g.rvalue(e.X)
	if v == nil {
		return nil
	}
	return g.blk.PointerToAddress(v, lowerType(p.Elem).Address())
}

// pointeeRead lowers a read through a pointer.
func (g *gen) pointeeRead(e *ast.MemberExpr, p *types.Pointer) *sil.Value {
	addr := g.pointeeAddr(e, p)
	if addr == nil {
		return nil
	}
	t := lowerType(p.Elem)
	return g.loaded(g.blk.Load(addr, loadQualifier(t)), t)
}

// pointeeAddrForWrite returns the destination address for a write through a mutable pointer.
func (g *gen) pointeeAddrForWrite(e *ast.MemberExpr, p *types.Pointer) *sil.Value {
	if !p.Mutable {
		g.errorAt(e, "cannot write through '"+p.String()+"': it points at "+
			"something it may only read. 'UnsafeMutablePointer' is the one "+
			"that writes")
		return nil
	}
	return g.pointeeAddr(e, p)
}

// pointerConvert lowers pointer type conversions (e.g. UnsafeRawPointer(p)).
// The bits are the same; the value is retyped by passing it through an
// address of the new pointee, so that what comes out is typed as the
// conversion says and can be returned or stored as one.
func (g *gen) pointerConvert(e *ast.CallExpr, arg ast.Expr, from, to types.Type) (*sil.Value, bool) {
	target, ok := pointerOf(to)
	if !ok {
		return nil, false
	}
	if _, ok := pointerOf(from); !ok {
		if o, isOpt := optionalOf(from); isOpt {
			if _, wraps := pointerOf(o.Wrapped); wraps {
				g.refuse(e, "converting an optional pointer, which would have to "+
					"answer an optional and does not say so yet")
				return nil, true
			}
		}
		return nil, false
	}
	v := g.rvalue(arg)
	if v == nil {
		return nil, true
	}
	elem := target.Elem
	if elem == nil {
		elem = types.Typ[types.UInt8]
	}
	addr := g.blk.PointerToAddress(v, lowerType(elem).Address())
	return g.blk.AddressToPointer(addr, lowerType(to)), true
}

// pointerEquality lowers pointer comparison (p == q and p != q).
func (g *gen) pointerEquality(e *ast.BinaryExpr, op string) (*sil.Value, bool) {
	if op != "==" && op != "!=" {
		return nil, false
	}
	if _, ok := pointerOf(g.typeOf(e.X)); !ok {
		return nil, false
	}
	if _, ok := pointerOf(g.typeOf(e.Y)); !ok {
		return nil, false
	}
	lhs, rhs := g.rvalue(e.X), g.rvalue(e.Y)
	if lhs == nil || rhs == nil {
		return nil, true
	}
	verb := "cmp_eq_RawPointer"
	if op == "!=" {
		verb = "cmp_ne_RawPointer"
	}
	bit := g.blk.Builtin(verb, sil.Object(sil.BuiltinInt1), lhs, rhs)
	return g.boolOf(bit), true
}

// pointerOffset lowers a pointer moved by a count of its elements, or of
// bytes for a raw pointer: `p + n`, `n + p` and `p - n`. It reports false
// where the operands are not a pointer and an offset.
func (g *gen) pointerOffset(e *ast.BinaryExpr) (*sil.Value, bool) {
	op := g.text(e.Op)
	if op != "+" && op != "-" {
		return nil, false
	}
	ptrX, offX := e.X, e.Y
	p, ok := pointerOf(g.typeOf(e.X))
	if !ok {
		if op != "+" {
			return nil, false
		}
		if p, ok = pointerOf(g.typeOf(e.Y)); !ok {
			return nil, false
		}
		ptrX, offX = e.Y, e.X
	}
	intT := types.Typ[types.Int]
	if b, ok := g.typeOf(offX).Underlying().(*types.Basic); !ok || b.Kind() != types.Int {
		g.refuse(offX, "a pointer moved by something other than an Int")
		return nil, true
	}
	base, off := g.expr(ptrX), g.expr(offX)
	if base == nil || off == nil {
		return nil, true
	}
	n := g.machine(off, intT)
	word := sil.Object(builtinFor(intT))
	if op == "-" {
		bit := sil.Object(sil.BuiltinInt1)
		pair := sil.Object(&types.Tuple{Elements: []*types.TupleElement{
			{Type: builtinFor(intT)},
			{Type: sil.BuiltinInt1},
		}})
		both := g.blk.Builtin("ssub_with_overflow_Int64", pair,
			g.blk.IntegerLiteral(word, 0), n, g.blk.IntegerLiteral(bit, -1))
		n = g.blk.TupleExtract(both, 0, word)
		g.blk.CondFail(g.blk.TupleExtract(both, 1, bit), "arithmetic overflow")
	}
	elem := p.Elem
	if elem == nil {
		elem = types.Typ[types.UInt8]
	}
	addr := g.blk.PointerToAddress(base, lowerType(elem).Address())
	moved := g.blk.IndexAddr(addr, n)
	return g.blk.AddressToPointer(moved, lowerType(g.typeOf(e))), true
}
