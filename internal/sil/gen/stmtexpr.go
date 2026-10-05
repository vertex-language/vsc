package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
)

// An if or a switch used as a value (SE-0380) is lowered as the statement
// it is, except that each branch, being one expression, ends by leaving
// the statement with its value: as a `break` would, unwinding what the
// branch and the statement opened, to a block that takes the value as its
// argument. That block is where the expression's value is.

// A valueJoin is where the branches of one if or switch expression go.
type valueJoin struct {
	typ   types.Type
	depth int // the scopes open where the expression began
	join  *sil.Block
	value *sil.Value // join's argument
	// slot is, for a value held in memory -- an existential -- where each
	// branch moves its value, in place of join's argument.
	slot *sil.Value
}

// stmtExpr lowers an if or a switch expression.
func (g *gen) stmtExpr(e *ast.StmtExpr) *sil.Value {
	vals, ok := analyzer.BranchValues(e.Stmt)
	if !ok {
		g.refuse(e, "an if or switch expression whose branches are not single expressions")
		return nil
	}
	t := g.typeOf(e)
	j := &valueJoin{typ: t, depth: len(g.scopes)}
	if !types.Identical(t, types.Typ[types.Never]) {
		j.join = g.fn.Block()
		vt := lowerType(t)
		if _, inMemory := existentialOf(t); inMemory {
			// An existential is a place, not a value: the branches
			// write it, and the join has nothing to take.
			j.slot = g.blk.AllocStack(vt)
		} else {
			own := sil.Owned
			if vt.Trivial() {
				own = sil.None
			}
			j.value = j.join.Arg(vt, own)
		}
	}
	if g.branchValues == nil {
		g.branchValues = map[*ast.ExprStmt]*valueJoin{}
	}
	for _, v := range vals {
		g.branchValues[v] = j
	}
	g.stmt(e.Stmt)
	for _, v := range vals {
		delete(g.branchValues, v)
	}
	// Every branch left, with its value or by throwing: nothing comes
	// out of the statement itself.
	if g.blk != nil && g.blk.Term() == nil {
		g.blk.Unreachable()
	}
	if j.join == nil {
		g.blk = nil
		return nil
	}
	g.blk = j.join
	if j.slot != nil {
		return j.slot
	}
	g.destroyLater(j.value)
	return j.value
}

// branchValue lowers the expression a branch of an if or switch
// expression is, and leaves the statement with its value.
func (g *gen) branchValue(s *ast.ExprStmt, j *valueJoin) {
	from := g.typeOf(s.X)
	v := g.consume(g.rvalue(s.X))
	if g.blk == nil || g.blk.Term() != nil {
		return
	}
	if types.Identical(from, types.Typ[types.Never]) || j.join == nil {
		g.blk.Unreachable()
		return
	}
	if v == nil {
		g.blk.Unreachable()
		return
	}
	v = g.optionalFor(s.X, v, from, j.typ)
	v = g.existentialFor(s.X, v, from, j.typ)
	if j.slot != nil && v != nil {
		if v.Type().IsAddress() {
			g.blk.CopyAddr(v, j.slot, "take", "init")
		} else {
			g.blk.Store(v, j.slot, storeQualifier(v.Type()))
		}
		g.unwindTo(j.depth)
		g.blk.Br(j.join)
		return
	}
	g.unwindTo(j.depth)
	g.blk.Br(j.join, v)
}
