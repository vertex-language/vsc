package gen

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// enumEquality lowers `a == b` and `a != b` over enums, and reports
// whether the operands were a pair it could compare.
func (g *gen) enumEquality(e *ast.BinaryExpr, op string) (*sil.Value, bool) {
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
	raw := g.blk.Builtin(verb+enumMachine(le), sil.Object(sil.BuiltinInt1), a, b)
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

// rawValue lowers `c.rawValue`: a switch over the cases, each answering
// with its raw value.
func (g *gen) rawValue(e *ast.MemberExpr, en *types.Enum) *sil.Value {
	for _, k := range en.Cases {
		if !g.hasRawValue(en, k) {
			g.refuse(e, "rawValue of '"+en.Name+"', a case of which has no raw value this knows")
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

	cases := make([]sil.Case, 0, len(en.Cases))
	arms := make([]*sil.Block, 0, len(en.Cases))
	for _, k := range en.Cases {
		arm := g.fn.Block()
		arms = append(arms, arm)
		cases = append(cases, sil.Case{Member: memberName(en, k.Name), Dest: arm})
	}
	// Unreachable default block for exhaustive enum switch.
	fallthroughBlk := g.fn.Block()
	cases = append(cases, sil.Case{Dest: fallthroughBlk})
	g.blk.SwitchEnum(subject, cases...)
	g.blk = fallthroughBlk
	g.blk.Unreachable()

	for i, k := range en.Cases {
		g.blk = arms[i]
		k := k
		v := g.branchArm(func() *sil.Value { return g.rawCaseValue(en, k) })
		if v == nil {
			return nil
		}
		g.blk.Br(join, v)
	}
	g.blk = join
	g.destroyLater(answer)
	return answer
}

// hasRawValue reports whether k's raw value is one rawCaseValue makes:
// a number counted or written, a literal written, or for a String the
// case's own name.
func (g *gen) hasRawValue(en *types.Enum, k *types.EnumCase) bool {
	if k == nil {
		return false
	}
	if k.HasRawInt || g.info.RawValues[k] != nil {
		return true
	}
	b, ok := en.RawType.Underlying().(*types.Basic)
	return ok && b.Kind() == types.String
}

// rawCaseValue is case k's raw value, owned.
func (g *gen) rawCaseValue(en *types.Enum, k *types.EnumCase) *sil.Value {
	raw := lowerType(en.RawType)
	if k.HasRawInt {
		return g.blk.Struct(raw, g.blk.IntegerLiteral(sil.Object(builtinFor(en.RawType)), k.RawInt))
	}
	if x := g.info.RawValues[k]; x != nil {
		return g.consume(g.rvalue(x))
	}
	return g.consume(g.makeString(k.Name))
}

// rawInit lowers `E(rawValue: x)`: a comparison against each case's raw
// value in turn, the first that matches answering with that case, and nil
// when none does.
func (g *gen) rawInit(e *ast.CallExpr, en *types.Enum) *sil.Value {
	for _, k := range en.Cases {
		if !g.hasRawValue(en, k) {
			g.refuse(e, "init(rawValue:) of '"+en.Name+"', a case of which has no raw value this knows")
			return nil
		}
	}
	x := g.expr(e.Args.Args[0].X)
	if x == nil {
		return nil
	}
	enumT := g.typeOf(e).Underlying().(*types.Optional).Wrapped
	optT := lowerType(g.typeOf(e))

	join := g.fn.Block()
	answer := join.Arg(optT, sil.None)
	for _, k := range en.Cases {
		same := g.rawEquals(e, en, k, x)
		if same == nil {
			return nil
		}
		hit, miss := g.fn.Block(), g.fn.Block()
		g.blk.CondBr(same, hit, nil, miss, nil)
		g.blk = hit
		c := g.blk.Enum(lowerType(enumT), memberName(en, k.Name), nil)
		g.blk.Br(join, g.blk.Enum(optT, optionalSome, c))
		g.blk = miss
	}
	g.blk.Br(join, g.blk.Enum(optT, optionalNone, nil))
	g.blk = join
	return answer
}

// rawEquals is whether x is case k's raw value, as a bit.
func (g *gen) rawEquals(at ast.Node, en *types.Enum, k *types.EnumCase, x *sil.Value) *sil.Value {
	rawT := en.RawType
	if k.HasRawInt {
		builtin := builtinFor(rawT)
		lit := g.blk.IntegerLiteral(sil.Object(builtin), k.RawInt)
		verb := "cmp_eq_" + builtin.String()[len("Builtin."):]
		return g.blk.Builtin(verb, sil.Object(sil.BuiltinInt1), g.machine(x, rawT), lit)
	}
	v := g.branchArm(func() *sil.Value { return g.rawCaseValue(en, k) })
	if v == nil {
		return nil
	}
	b, _ := rawT.Underlying().(*types.Basic)
	switch {
	case b != nil && b.Kind() == types.String:
		bit := g.externCompare(x, v, rawT, core.Extern{Symbol: stdlib.StringEqual})
		g.blk.DestroyValue(v)
		return bit
	case b != nil && b.Kind() == types.Character:
		// A Character is the String of its cluster.
		str := types.Typ[types.String]
		st := lowerType(str)
		lhs := g.blk.BeginBorrow(x)
		rhs := g.blk.BeginBorrow(v)
		bit := g.externCompare(
			g.blk.StructExtract(lhs, memberName(rawT, "_string"), st),
			g.blk.StructExtract(rhs, memberName(rawT, "_string"), st),
			str, core.Extern{Symbol: stdlib.StringEqual})
		g.blk.EndBorrow(rhs)
		g.blk.EndBorrow(lhs)
		g.blk.DestroyValue(v)
		return bit
	}
	if op, ok := core.Lower("==", rawT); ok && op.Result == "Int1" {
		return g.blk.Builtin(op.Name, sil.Object(sil.BuiltinInt1), g.machine(x, rawT), g.machine(v, rawT))
	}
	g.refuse(at, "init(rawValue:) of '"+en.Name+"', whose raw type this cannot compare")
	return nil
}
