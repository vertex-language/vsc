package gen

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
)

// Shift operator lowering for smart and masking shifts.

// isShift reports whether an operator is one of the four shifts.
func isShift(op string) bool {
	switch op {
	case "<<", ">>", "&<<", "&>>":
		return true
	}
	return false
}

// shift lowers a smart or masking shift expression.
func (g *gen) shift(at ast.Node, op string, operand, results types.Type, lhs, rhs *sil.Value) *sil.Value {
	b, isBasic := operand.Underlying().(*types.Basic)
	if !isBasic || b.Info()&types.IsInteger == 0 {
		return nil
	}
	_, machine, ok := core.Layout(operand)
	if !ok {
		return nil
	}
	unsigned := b.Info()&types.IsUnsigned != 0
	width := int64(types.Sizeof(operand, types.DefaultTarget64) * 8)
	if width <= 0 {
		return nil
	}

	s := &shifter{
		g:        g,
		word:     sil.Object(builtinNamed(machine)),
		machine:  machine,
		unsigned: unsigned,
		width:    width,
		value:    g.machine(lhs, operand),
		count:    g.machine(rhs, operand),
	}

	var raw *sil.Value
	switch op {
	case "&<<", "&>>":
		raw = s.masking(op == "&<<")
	case "<<":
		raw = s.smart(true)
	case ">>":
		raw = s.smart(false)
	default:
		return nil
	}
	if raw == nil {
		return nil
	}
	return g.blk.Struct(lowerType(results), raw)
}

// shifter lowers shift operations for integer machine types.
type shifter struct {
	g        *gen
	word     sil.Type
	machine  string
	unsigned bool
	width    int64
	value    *sil.Value
	count    *sil.Value
}

func (s *shifter) blk() *sil.Block          { return s.g.blk }
func (s *shifter) konst(n int64) *sil.Value { return s.blk().IntegerLiteral(s.word, n) }

// left emits a machine left shift.
//
// A narrow register holds an unsigned value zero-extended and a signed one
// sign-extended, but shl_IntN names no signedness and the lowering keeps
// its result sign-extended. An unsigned narrow shift is masked back to its
// width, so `uint16(0xBE) << 8 == 0xBE00` compares the same bits.
func (s *shifter) left(v, by *sil.Value) *sil.Value {
	r := s.blk().Builtin("shl_"+s.machine, s.word, v, by)
	if s.unsigned && s.width < 32 {
		r = s.blk().Builtin("and_"+s.machine, s.word, r, s.konst(int64(1)<<s.width-1))
	}
	return r
}

// right emits an arithmetic or logical right shift based on signedness.
func (s *shifter) right(v, by *sil.Value) *sil.Value {
	name := "ashr_"
	if s.unsigned {
		name = "lshr_"
	}
	return s.blk().Builtin(name+s.machine, s.word, v, by)
}

// compare emits a comparison respecting the operand's signedness.
func (s *shifter) compare(rel string, a, b *sil.Value) *sil.Value {
	sign := "s"
	if s.unsigned {
		sign = "u"
	}
	return s.blk().Builtin("cmp_"+sign+rel+"_"+s.machine,
		sil.Object(sil.BuiltinInt1), a, b)
}

// saturated returns the sign-extended value for signed types or zero for unsigned.
func (s *shifter) saturated() *sil.Value {
	if s.unsigned {
		return s.konst(0)
	}
	return s.right(s.value, s.konst(s.width-1))
}

// negated returns -count via non-trapping subtraction.
func (s *shifter) negated() *sil.Value {
	quiet := s.blk().IntegerLiteral(sil.Object(sil.BuiltinInt1), 0)
	pair := sil.Object(&types.Tuple{Elements: []*types.TupleElement{
		{Type: builtinNamed(s.machine)},
		{Type: sil.BuiltinInt1},
	}})
	both := s.blk().Builtin("ssub_with_overflow_"+s.machine, pair,
		s.konst(0), s.count, quiet)
	return s.blk().TupleExtract(both, 0, s.word)
}

// masking lowers masking shifts (&<<, &>>) by masking count with width-1.
func (s *shifter) masking(toLeft bool) *sil.Value {
	by := s.blk().Builtin("and_"+s.machine, s.word, s.count, s.konst(s.width-1))
	if toLeft {
		return s.left(s.value, by)
	}
	return s.right(s.value, by)
}

// smart lowers Swift smart shifts (<<, >>) handling over-shifts and negative counts.
func (s *shifter) smart(toLeft bool) *sil.Value {
	// Unsigned counts are non-negative, requiring only the over-shift check.
	if s.unsigned {
		return s.oneTest(toLeft)
	}

	g := s.g
	fn := g.fn

	over, notOver := fn.Block(), fn.Block()
	under, inRange := fn.Block(), fn.Block()
	forward, backward := fn.Block(), fn.Block()
	join := fn.Block()
	answer := join.Arg(s.word, sil.None)

	// b >= width: the whole value is shifted out.
	g.blk.CondBr(s.compare("ge", s.count, s.konst(s.width)), over, nil, notOver, nil)

	g.blk = over
	if toLeft {
		g.blk.Br(join, s.konst(0))
	} else {
		g.blk.Br(join, s.saturated())
	}

	// b <= -width: the same, in the other direction.
	g.blk = notOver
	g.blk.CondBr(s.compare("le", s.count, s.konst(-s.width)), under, nil, inRange, nil)

	g.blk = under
	if toLeft {
		g.blk.Br(join, s.saturated())
	} else {
		g.blk.Br(join, s.konst(0))
	}

	// In range: shift forward if count >= 0, backward otherwise.
	g.blk = inRange
	g.blk.CondBr(s.compare("ge", s.count, s.konst(0)), forward, nil, backward, nil)

	g.blk = forward
	if toLeft {
		g.blk.Br(join, s.left(s.value, s.count))
	} else {
		g.blk.Br(join, s.right(s.value, s.count))
	}

	g.blk = backward
	by := s.negated()
	if toLeft {
		g.blk.Br(join, s.right(s.value, by))
	} else {
		g.blk.Br(join, s.left(s.value, by))
	}

	g.blk = join
	return answer
}

// oneTest lowers smart shifts for unsigned counts.
func (s *shifter) oneTest(toLeft bool) *sil.Value {
	g := s.g
	over, inRange, join := g.fn.Block(), g.fn.Block(), g.fn.Block()
	answer := join.Arg(s.word, sil.None)

	g.blk.CondBr(s.compare("ge", s.count, s.konst(s.width)), over, nil, inRange, nil)

	g.blk = over
	// Over-shifted unsigned values saturate to zero.
	g.blk.Br(join, s.konst(0))

	g.blk = inRange
	if toLeft {
		g.blk.Br(join, s.left(s.value, s.count))
	} else {
		g.blk.Br(join, s.right(s.value, s.count))
	}

	g.blk = join
	return answer
}
