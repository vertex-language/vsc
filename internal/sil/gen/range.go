package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
)

// Ranges are the core's Range<Bound> and ClosedRange<Bound>, structs of
// a lowerBound and an upperBound. Swift makes one with its generic `..<`
// and `...`, which check the bounds are in order; here `a..<b` is that
// struct made in place, and a loop or a slice over one written out takes
// its bounds without making it at all.

// rangeFormation lowers `a..<b` or `a...b` as a value: the bounds, checked
// to be in order, in a range. It reports false for any other operator.
func (g *gen) rangeFormation(e *ast.BinaryExpr) (*sil.Value, bool) {
	if !isRangeOperator(g.text(e.Op)) {
		return nil, false
	}
	t := g.typeOf(e)
	if _, _, ok := g.info.RangeOf(t); !ok {
		return nil, false
	}
	lo, hi := g.rangeBounds(e)
	if lo == nil || hi == nil {
		return nil, true
	}
	v := g.blk.Struct(lowerType(t), lo, hi)
	g.destroyLater(v)
	return v, true
}

// rangeBounds is a range's lower and upper bound. A range written out is
// its operands, evaluated once and checked to be in order, as the
// operator that would make it checks them; any other is a range value,
// and its bounds are taken out of it.
func (g *gen) rangeBounds(e ast.Expr) (lo, hi *sil.Value) {
	t := g.typeOf(e)
	bound, _, ok := g.rangeOf(e)
	if !ok {
		return nil, nil
	}
	if bin, isBin := g.fold(e).(*ast.BinaryExpr); isBin && bin.Op != nil && isRangeOperator(g.text(bin.Op)) {
		lo, hi = g.rvalue(bin.X), g.rvalue(bin.Y)
		if lo == nil || hi == nil {
			return nil, nil
		}
		if bad := g.compare("<", hi, lo, bound); bad != nil {
			g.blk.CondFail(bad, "Range requires lowerBound <= upperBound")
		}
		return lo, hi
	}
	v := g.expr(e)
	if v == nil {
		return nil, nil
	}
	bt := lowerType(bound)
	return g.blk.StructExtract(v, memberName(t, "lowerBound"), bt),
		g.blk.StructExtract(v, memberName(t, "upperBound"), bt)
}

// isRangeOperator reports whether op makes a range.
func isRangeOperator(op string) bool {
	return op == "..<" || op == "..."
}

// postfix lowers a postfix operator: one the program declares, or `a...`.
func (g *gen) postfix(e *ast.PostfixExpr) *sil.Value {
	sym, _ := g.info.Operators[e].(*analyzer.FuncSymbol)
	if ref := g.info.OperatorMethods[e]; ref != nil || sym != nil {
		return g.operatorCall(e, ref, sym, e.X)
	}
	if v, isRange := g.partialRange(e, e.X); isRange {
		return v
	}
	g.expr(e.X)
	g.unsupported(e)
	return nil
}

// partialRange lowers `..<b`, `...b` or `a...` as a value: a range of one
// bound, x. It reports false for an expression that makes no such range.
func (g *gen) partialRange(e, x ast.Expr) (*sil.Value, bool) {
	t := g.typeOf(e)
	if _, name, ok := g.info.AnyRangeOf(t); !ok || name == "Range" || name == "ClosedRange" {
		return nil, false
	}
	v := g.rvalue(x)
	if v == nil {
		return nil, true
	}
	r := g.blk.Struct(lowerType(t), v)
	g.destroyLater(r)
	return r, true
}

// rangeTest is whether subject lies in the range pat writes out -- `1...5`,
// `..<0`, `100...` -- as the comparisons with its bounds that `~=` makes;
// nil where pat writes out no range.
func (g *gen) rangeTest(pat ast.Expr, subject *sil.Value, t types.Type) (*sil.Value, bool) {
	var lower, upper ast.Expr
	below := "<="
	switch x := g.fold(pat).(type) {
	case *ast.BinaryExpr:
		if x.Op == nil || !isRangeOperator(g.text(x.Op)) {
			return nil, false
		}
		lower, upper = x.X, x.Y
		if g.text(x.Op) == "..<" {
			below = "<"
		}
	case *ast.PrefixExpr:
		if x.Op == nil || !isRangeOperator(g.text(x.Op)) {
			return nil, false
		}
		upper = x.X
		if g.text(x.Op) == "..<" {
			below = "<"
		}
	case *ast.PostfixExpr:
		if x.Op == nil || g.text(x.Op) != "..." {
			return nil, false
		}
		lower = x.X
	default:
		return nil, false
	}
	// The bounds are compared, not kept: they end with the test's scope.
	var lo, hi, atLeast, atMost *sil.Value
	if lower != nil {
		if lo = g.expr(lower); lo == nil {
			return nil, true
		}
	}
	if upper != nil {
		if hi = g.expr(upper); hi == nil {
			return nil, true
		}
	}
	if lo != nil {
		if atLeast = g.compare("<=", lo, subject, t); atLeast == nil {
			return nil, true
		}
	}
	if hi != nil {
		if atMost = g.compare(below, subject, hi, t); atMost == nil {
			return nil, true
		}
	}
	switch {
	case atLeast == nil:
		return atMost, true
	case atMost == nil:
		return atLeast, true
	}
	return g.blk.Builtin("and_Int1", sil.Object(sil.BuiltinInt1), atLeast, atMost), true
}

// rangeOf is info.RangeOf for an expression, under the specialization in
// force: self in a range's own extension is the range of Bound, which
// the checker recorded and a specialization says the bound of.
func (g *gen) rangeOf(e ast.Expr) (bound types.Type, closed, ok bool) {
	t := g.info.Types[e]
	if g.chainActive[e] {
		t = g.info.ChainInner[e]
	}
	if bound, closed, ok = g.info.RangeOf(t); ok {
		return g.substituted(bound), closed, true
	}
	return g.info.RangeOf(g.typeOf(e))
}

// anyRangeOf is rangeOf for any of the core's ranges, one-sided ones
// too, with the range's name.
func (g *gen) anyRangeOf(e ast.Expr) (bound types.Type, name string, ok bool) {
	t := g.info.Types[e]
	if g.chainActive[e] {
		t = g.info.ChainInner[e]
	}
	if bound, name, ok = g.info.AnyRangeOf(t); ok {
		return g.substituted(bound), name, true
	}
	return g.info.AnyRangeOf(g.typeOf(e))
}

// partialBound is the one bound of a one-sided range: its operand where
// it is written out -- `..<n`, `n...` -- and the field otherwise.
func (g *gen) partialBound(e ast.Expr) *sil.Value {
	bound, name, ok := g.anyRangeOf(e)
	if !ok {
		return nil
	}
	switch x := g.fold(e).(type) {
	case *ast.PrefixExpr:
		if x.Op != nil && isRangeOperator(g.text(x.Op)) {
			return g.rvalue(x.X)
		}
	case *ast.PostfixExpr:
		if x.Op != nil && g.text(x.Op) == "..." {
			return g.rvalue(x.X)
		}
	}
	v := g.expr(e)
	if v == nil {
		return nil
	}
	field := "upperBound"
	if name == "PartialRangeFrom" {
		field = "lowerBound"
	}
	return g.blk.StructExtract(v, memberName(g.typeOf(e), field), lowerType(bound))
}
