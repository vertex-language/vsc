package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
)

// Key paths used as values. A KeyPath is core's struct of two functions,
// the one reading through the path and, where every step of it can be
// written, the one writing through it; the checker made both closures of
// the path (analyzer's keyPathValue).

// keyPathParts is a KeyPath type's root and value, and the types of its
// two functions: (Root) -> Value, and ((inout Root, Value) -> Void)?.
func keyPathParts(t types.Type) (root, value types.Type, get, set types.Type, ok bool) {
	gi, isInst := t.(*types.GenericInstance)
	if !isInst || len(gi.Args) != 2 {
		return nil, nil, nil, nil, false
	}
	root, value = gi.Args[0], gi.Args[1]
	get = &types.Signature{Params: []*types.Param{{Type: root}}, Results: value}
	set = &types.Optional{Wrapped: &types.Signature{
		Params:  []*types.Param{{Type: root, Ownership: types.InOut}, {Type: value}},
		Results: types.Typ[types.Void]}}
	return root, value, get, set, true
}

// keyPathValue makes the KeyPath of a key path's closures.
func (g *gen) keyPathValue(kv *analyzer.KeyPathValue) *sil.Value {
	t := g.substituted(kv.Type)
	_, _, _, setT, ok := keyPathParts(t)
	if !ok {
		return nil
	}
	get := g.rvalue(kv.Get)
	if get == nil {
		return nil
	}
	var set *sil.Value
	if kv.Set != nil {
		s := g.rvalue(kv.Set)
		if s == nil {
			return nil
		}
		set = g.blk.Enum(lowerType(setT), optionalSome, s)
	} else {
		set = g.blk.Enum(lowerType(setT), optionalNone, nil)
	}
	v := g.blk.Struct(lowerType(t), get, set)
	g.destroyLater(v)
	return v
}

// keyPathRead is `x[keyPath: k]`: k's reading function applied to x.
func (g *gen) keyPathRead(n *ast.SubscriptExpr) *sil.Value {
	kt := g.substituted(g.info.KeyPathReads[n])
	_, value, getT, _, ok := keyPathParts(kt)
	if !ok {
		g.unsupported(n)
		return nil
	}
	kp := g.expr(n.Args[0].X)
	root := g.expr(n.X)
	if kp == nil || root == nil {
		return nil
	}
	b := g.blk.BeginBorrow(kp)
	fn := g.blk.StructExtract(b, memberName(kt, "_get"), lowerType(getT))
	out := g.blk.Apply(fn, lowerType(value), root)
	g.blk.EndBorrow(b)
	g.destroyLater(out)
	return out
}

// keyPathWrite is `x[keyPath: k] = v`: k's writing function applied to
// x's storage and v, which traps where k cannot write.
func (g *gen) keyPathWrite(n *ast.SubscriptExpr, from ast.Expr) {
	kt := g.substituted(g.info.KeyPathReads[n])
	_, value, _, setT, ok := keyPathParts(kt)
	if !ok {
		g.unsupported(n)
		return
	}
	kp := g.expr(n.Args[0].X)
	if kp == nil {
		return
	}
	v := g.rvalue(from)
	if v == nil {
		return
	}
	v = g.optionalFor(from, v, g.typeOf(from), value)
	addr := g.lvalue(n.X)
	if addr == nil {
		g.refuse(n.X, "a write through a key path to something that is not storage")
		return
	}
	b := g.blk.BeginBorrow(kp)
	// The switch takes its operand, so it is handed a copy.
	opt := g.blk.CopyValue(g.blk.StructExtract(b, memberName(kt, "_set"), lowerType(setT)))
	g.blk.EndBorrow(b)
	fnT := lowerType(setT.(*types.Optional).Wrapped)
	some, none := g.fn.Block(), g.fn.Block()
	fn := some.Arg(fnT, sil.Owned)
	g.blk.SwitchEnum(opt,
		sil.Case{Member: optionalSome, Dest: some},
		sil.Case{Member: optionalNone, Dest: none})
	g.blk = none
	always := g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt1), -1)
	g.blk.CondFail(always, "a key path that cannot write was written through")
	g.blk.Unreachable()
	g.blk = some
	access := g.blk.BeginAccess(addr, "modify", "unknown")
	g.blk.Apply(fn, sil.Object(types.Typ[types.Void]), access, v)
	g.blk.EndAccess(access)
	g.blk.DestroyValue(fn)
	if lt := lowerType(value); !lt.Trivial() && v.Ownership() == sil.Owned {
		g.blk.DestroyValue(v)
	}
}
