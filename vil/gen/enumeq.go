package gen

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Equality on enums.
//
// Swift synthesizes `==` for an enum that asks to be Equatable, and
// for one with no associated values what it synthesizes compares the
// tags -- there is nothing else in the value to compare. There is no
// standard library here to declare that function and no synthesis to
// produce it, so the comparison is emitted where the call would have
// been, which is the decision literal() and convert() already take
// for the same reason.
//
// The tag is the value: lower holds a payload-free enum in an integer
// register, so the operands go to the builtin as they are rather than
// through the struct_extract an Int would need. That is the one thing
// that makes this different from comparing two Ints.

// enumEquality lowers `a == b` and `a != b` over enums, and reports
// whether the operands were a pair it could compare.
func (g *gen) enumEquality(e *ast.BinaryExpr, op string) (*vil.Value, bool) {
	if op != "==" && op != "!=" {
		return nil, false
	}
	lt, rt := g.typeOf(e.X), g.typeOf(e.Y)
	le, ok := enumFor(lt)
	if !ok {
		return nil, false
	}
	re, ok := enumFor(rt)
	if !ok || le != re {
		return nil, false
	}
	for _, c := range le.Cases {
		if c != nil && c.AssociatedType != nil {
			g.refuse(e, "a comparison of an enum that carries a value")
			return nil, true
		}
	}

	a, b := g.expr(e.X), g.expr(e.Y)
	if a == nil || b == nil {
		return nil, true
	}
	verb := "cmp_eq_"
	if op == "!=" {
		verb = "cmp_ne_"
	}
	raw := g.blk.Builtin(verb+enumMachine(le), vil.Object(vil.BuiltinInt1), a, b)
	return g.blk.Struct(lowerType(g.typeOf(e)), raw), true
}

// enumFor is the enum a type is.
func enumFor(t types.Type) (*types.Enum, bool) {
	if t == nil {
		return nil, false
	}
	e, ok := t.Underlying().(*types.Enum)
	return e, ok
}

// enumMachine is the integer a tag is held in, named the way a
// builtin names it. It has to agree with lower's own answer for the
// same enum, which is why both read it from the number of cases.
func enumMachine(e *types.Enum) string {
	switch size := types.Sizeof(e, types.DefaultTarget64); {
	case size <= 1:
		return "Int8"
	case size <= 2:
		return "Int16"
	case size <= 4:
		return "Int32"
	}
	return "Int64"
}

// rawValueRead reports whether a member read is `rawValue` on an enum
// that declares a raw type.
func rawValueRead(t types.Type, name string) (*types.Enum, bool) {
	if name != "rawValue" || t == nil {
		return nil, false
	}
	en, ok := t.Underlying().(*types.Enum)
	if !ok || en.RawType == nil {
		return nil, false
	}
	return en, true
}

// rawValue lowers `c.rawValue`: the number the case was declared
// with, which is not its tag.
//
// A switch over the case, each arm handing the join the constant the
// source wrote. The tag says which case is there; the raw value says
// what that case was numbered, and `case bad = 7` is the second case
// carrying a 7 -- so answering the tag would answer 1.
func (g *gen) rawValue(e *ast.MemberExpr, en *types.Enum) *vil.Value {
	for _, k := range en.Cases {
		if k == nil || !k.HasRawInt {
			// A string raw value needs a string constant to answer
			// with, and there is no making one yet. Refused rather
			// than answered with something else.
			g.refuse(e, "rawValue of '"+en.Name+"', whose cases are not all numbers")
			return nil
		}
	}
	subject := g.rvalue(e.X)
	if subject == nil {
		return nil
	}
	raw := lowerType(en.RawType)
	join := g.fn.Block()
	answer := join.Arg(raw, joinOwnership(raw))

	cases := make([]vil.Case, 0, len(en.Cases))
	arms := make([]*vil.Block, 0, len(en.Cases))
	for _, k := range en.Cases {
		arm := g.fn.Block()
		arms = append(arms, arm)
		cases = append(cases, vil.Case{Member: memberName(en, k.Name), Dest: arm})
	}
	// Every case is named, so the default is unreachable -- but the
	// switch needs an edge for it all the same, and a block with no
	// predecessors is not a well formed one either. Unreachable is
	// what says both.
	fallthroughBlk := g.fn.Block()
	cases = append(cases, vil.Case{Dest: fallthroughBlk})
	g.blk.SwitchEnum(subject, cases...)
	g.blk = fallthroughBlk
	g.blk.Unreachable()

	for i, k := range en.Cases {
		g.blk = arms[i]
		v := g.blk.Struct(raw, g.blk.IntegerLiteral(vil.Object(builtinFor(en.RawType)), k.RawInt))
		g.blk.Br(join, v)
	}
	g.blk = join
	return answer
}
