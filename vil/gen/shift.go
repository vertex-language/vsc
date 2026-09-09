package gen

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Shifts.
//
// Swift's `<<` and `>>` are not the machine's. `shl` and `ashr` say
// nothing about a shift of the width or more -- LLVM calls it poison
// and the hardware masks the count -- and Swift promises an answer
// for every count there is: shifting an Int32 by forty is zero, and
// shifting by minus two shifts the other way.
//
//	a << b   b >= 32   0
//	         b <= -32  a's sign, repeated
//	         b >= 0    shl a, b
//	         b < 0     the arithmetic right shift by -b
//
//	a >> b   b >= 32   a's sign, repeated
//	         b <= -32  0
//	         b >= 0    the right shift by b
//	         b < 0     shl a, -b
//
// A negation is safe only inside the last case: the count is between
// zero and the width there, so negating it cannot overflow, which is
// why the out-of-range tests come first and compare rather than
// negate. That is the order swiftc's own code takes.
//
// `&<<` and `&>>` are the masking shifts and are a different
// operator: they say the count is taken modulo the width and nothing
// else, so they are one `and` and one shift with no branches.
//
// An unsigned count cannot be negative, so half of this collapses:
// the two negative cases are unreachable and the sign a right shift
// repeats is zero.

// isShift reports whether an operator is one of the four shifts.
func isShift(op string) bool {
	switch op {
	case "<<", ">>", "&<<", "&>>":
		return true
	}
	return false
}

// shift lowers a shift of either kind, and answers nothing where the
// operand is not an integer core knows the layout of.
func (g *gen) shift(at ast.Node, op string, operand, results types.Type, lhs, rhs *vil.Value) *vil.Value {
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
		word:     vil.Object(builtinNamed(machine)),
		machine:  machine,
		unsigned: unsigned,
		width:    width,
		value:    g.machine(lhs, operand),
		count:    g.machine(rhs, operand),
	}

	var raw *vil.Value
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

// A shifter holds what every one of the shapes below needs: the
// machine type the shift happens in, the two operands already reached
// through their structs, and the facts about the type that decide
// which instruction and which comparison to use.
type shifter struct {
	g        *gen
	word     vil.Type
	machine  string
	unsigned bool
	width    int64
	value    *vil.Value
	count    *vil.Value
}

func (s *shifter) blk() *vil.Block          { return s.g.blk }
func (s *shifter) konst(n int64) *vil.Value { return s.blk().IntegerLiteral(s.word, n) }

// left is the machine's shift left, and right the one that matches
// the type's signedness: an arithmetic shift repeats the sign bit, a
// logical one brings in zeros.
func (s *shifter) left(v, by *vil.Value) *vil.Value {
	return s.blk().Builtin("shl_"+s.machine, s.word, v, by)
}

func (s *shifter) right(v, by *vil.Value) *vil.Value {
	name := "ashr_"
	if s.unsigned {
		name = "lshr_"
	}
	return s.blk().Builtin(name+s.machine, s.word, v, by)
}

// compare answers a bit, with the ordering the type's signedness
// calls for -- `b >= 32` on a UInt32 is not the same question as on
// an Int32, and asking it signed is how an over-shift of a large
// unsigned count came out looking negative.
func (s *shifter) compare(rel string, a, b *vil.Value) *vil.Value {
	sign := "s"
	if s.unsigned {
		sign = "u"
	}
	return s.blk().Builtin("cmp_"+sign+rel+"_"+s.machine,
		vil.Object(vil.BuiltinInt1), a, b)
}

// saturated is what a right shift past the width leaves: the sign bit
// repeated across the word, which is zero for a value that is not
// negative and all ones for one that is. An unsigned type has no sign
// bit and the answer is zero.
func (s *shifter) saturated() *vil.Value {
	if s.unsigned {
		return s.konst(0)
	}
	return s.right(s.value, s.konst(s.width-1))
}

// negated is `0 - count`, which is only ever asked for a count the
// tests above have already put inside the width, so it cannot
// overflow.
//
// Swift has no unchecked subtraction of its own: `&-` is the checked
// builtin with its third operand -- the one that says whether to
// report -- set to zero, and the overflow bit dropped. That is what
// swiftc emits for `&-`, so it is what this emits here.
func (s *shifter) negated() *vil.Value {
	quiet := s.blk().IntegerLiteral(vil.Object(vil.BuiltinInt1), 0)
	pair := vil.Object(&types.Tuple{Elements: []*types.TupleElement{
		{Type: builtinNamed(s.machine)},
		{Type: vil.BuiltinInt1},
	}})
	both := s.blk().Builtin("ssub_with_overflow_"+s.machine, pair,
		s.konst(0), s.count, quiet)
	return s.blk().TupleExtract(both, 0, s.word)
}

// masking is `&<<` and `&>>`: the count modulo the width, and the
// shift. The width is a power of two, so the modulo is an `and` with
// one less -- which is the instruction swiftc emits for it.
func (s *shifter) masking(toLeft bool) *vil.Value {
	by := s.blk().Builtin("and_"+s.machine, s.word, s.count, s.konst(s.width-1))
	if toLeft {
		return s.left(s.value, by)
	}
	return s.right(s.value, by)
}

// smart is `<<` and `>>`: the four cases, as four blocks handing one
// join the answer.
//
// The two out-of-range tests come first because they are what makes
// the negation in the last case safe, and they are tests rather than
// a negation for the same reason: negating the most negative count
// would overflow, and the answer for it is already decided.
func (s *shifter) smart(toLeft bool) *vil.Value {
	// An unsigned count is never negative, so there is one test and
	// two answers rather than three and four. Emitting the other two
	// would be blocks no path reaches, and a comparison against a
	// negative literal in a type that has none.
	if s.unsigned {
		return s.oneTest(toLeft)
	}

	g := s.g
	fn := g.fn

	over, notOver := fn.Block(), fn.Block()
	under, inRange := fn.Block(), fn.Block()
	forward, backward := fn.Block(), fn.Block()
	join := fn.Block()
	answer := join.Arg(s.word, vil.None)

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

	// In range, and which way it goes is the count's sign. The
	// negation is safe here and nowhere above: the count is between
	// the width and minus the width.
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

// oneTest is smart for an unsigned count: past the width, or not.
func (s *shifter) oneTest(toLeft bool) *vil.Value {
	g := s.g
	over, inRange, join := g.fn.Block(), g.fn.Block(), g.fn.Block()
	answer := join.Arg(s.word, vil.None)

	g.blk.CondBr(s.compare("ge", s.count, s.konst(s.width)), over, nil, inRange, nil)

	g.blk = over
	// Both directions leave nothing: an unsigned value has no sign to
	// repeat, so a right shift past the width is zero too.
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
