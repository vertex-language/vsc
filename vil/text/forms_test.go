package text

import (
	"strings"
	"testing"

	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// One instruction, one line, one expected form.
//
// The forms are SIL's, read out of `swiftc -emit-silgen` for a
// program that produces each: `tuple (%0, %1)` carries no type,
// `store %1 to [trivial] %0` writes `to` rather than a comma,
// `metatype $@thin Int.Type` names its representation, `enum $Shape,
// #Shape.line!enumelt, %0` puts the payload last. Getting one of
// these wrong is invisible until a diff fails, so each is held here
// on its own.
//
// The builder methods this file does not reach are the ones nothing
// can emit yet: an instruction with no builder cannot appear in a
// module, and one arrives with its form when a construct needs it.

func TestInstructionForms(t *testing.T) {
	intT := vil.Object(types.Typ[types.Int])
	boolT := vil.Object(types.Typ[types.Bool])
	box := types.NewNamed("Box", "", &types.Class{Name: "Box"})
	boxT := vil.Object(box)
	point := types.NewNamed("Point", "", &types.Struct{Name: "Point", Fields: []*types.Field{
		{Name: "x", Type: types.Typ[types.Int]},
		{Name: "y", Type: types.Typ[types.Int]},
	}})
	pointT := vil.Object(point)
	shape := types.NewNamed("Shape", "", &types.Enum{Name: "Shape", Cases: []*types.EnumCase{
		{Name: "dot"}, {Name: "line", AssociatedType: types.Typ[types.Int]},
	}})
	shapeT := vil.Object(shape)

	cases := []struct {
		name string
		emit func(b *vil.Block, args []*vil.Value)
		want []string
	}{
		{"ownership", func(b *vil.Block, a []*vil.Value) {
			c := b.CopyValue(a[0])
			bo := b.BeginBorrow(c)
			b.EndBorrow(bo)
			m := b.MoveValue(c, "lexical", "var_decl")
			b.ExtendLifetime(m)
			b.DestroyValue(m)
		}, []string{
			"%3 = copy_value %0",
			"%4 = begin_borrow %3",
			"end_borrow %4",
			"%5 = move_value [lexical] [var_decl] %3",
			"extend_lifetime %5",
			"destroy_value %5",
		}},

		{"stack memory", func(b *vil.Block, a []*vil.Value) {
			s := b.AllocStack(intT)
			b.Store(a[1], s, "trivial")
			b.Load(s, "trivial")
			b.DeallocStack(s)
		}, []string{
			"%3 = alloc_stack $Int",
			"store %1 to [trivial] %3",
			"%4 = load [trivial] %3",
			"dealloc_stack %3",
		}},

		{"a box is a var", func(b *vil.Block, a []*vil.Value) {
			bx := b.AllocBox(intT, "total", "var")
			bo := b.BeginBorrow(bx, "var_decl")
			addr := b.ProjectBox(bo, 0, intT)
			b.Assign(a[1], addr)
			b.EndBorrow(bo)
			b.DestroyValue(bx)
		}, []string{
			`%3 = alloc_box ${ var Int }, var, name "total"`,
			"%4 = begin_borrow [var_decl] %3",
			"%5 = project_box %4, 0",
			"assign %1 to %5",
			"end_borrow %4",
			"destroy_value %3",
		}},

		{"access scopes", func(b *vil.Block, a []*vil.Value) {
			addr := b.RefElementAddr(a[0], "Box.n", intT)
			acc := b.BeginAccess(addr, "read", "dynamic")
			b.Load(acc, "trivial")
			b.EndAccess(acc)
		}, []string{
			"%3 = ref_element_addr %0, #Box.n",
			"%4 = begin_access [read] [dynamic] %3",
			"%5 = load [trivial] %4",
			"end_access %4",
		}},

		{"aggregates", func(b *vil.Block, a []*vil.Value) {
			p := b.Struct(pointT, a[1], a[1])
			b.StructExtract(p, "Point.x", intT)
			b.StructElementAddr(b.AllocStack(pointT), "Point.y", intT)
			tup := b.Tuple(vil.Object(&types.Tuple{Elements: []*types.TupleElement{
				{Type: types.Typ[types.Int]}, {Type: types.Typ[types.Int]},
			}}), a[1], a[1])
			b.TupleExtract(tup, 1, intT)
			b.DestructureTuple(tup, intT, intT)
		}, []string{
			"%3 = struct $Point (%1, %1)",
			"%4 = struct_extract %3, #Point.x",
			"%5 = alloc_stack $Point",
			"%6 = struct_element_addr %5, #Point.y",
			"%7 = tuple (%1, %1)",
			"%8 = tuple_extract %7, 1",
			"(%9, %10) = destructure_tuple %7",
		}},

		{"enums", func(b *vil.Block, a []*vil.Value) {
			dot := b.Enum(shapeT, "Shape.dot!enumelt", nil)
			line := b.Enum(shapeT, "Shape.line!enumelt", a[1])
			b.UncheckedEnumData(line, "Shape.line!enumelt", intT)
			_ = dot
		}, []string{
			"%3 = enum $Shape, #Shape.dot!enumelt",
			"%4 = enum $Shape, #Shape.line!enumelt, %1",
			"%5 = unchecked_enum_data %4, #Shape.line!enumelt",
		}},

		{"literals and metatypes", func(b *vil.Block, a []*vil.Value) {
			b.IntegerLiteral(vil.Object(vil.BuiltinInt64), 42)
			b.IntegerLiteral(vil.Object(vil.BuiltinInt1), -1)
			b.StringLiteral("hi", "utf8")
			b.Metatype(intT)
		}, []string{
			"%3 = integer_literal $Builtin.Int64, 42",
			"%4 = integer_literal $Builtin.Int1, -1",
			`%5 = string_literal utf8 "hi"`,
			"%6 = metatype $@thin Int.Type",
		}},

		{"builtins", func(b *vil.Block, a []*vil.Value) {
			raw := b.StructExtract(a[1], "Int._value", vil.Object(vil.BuiltinInt64))
			bit := b.Builtin("cmp_slt_Int64", vil.Object(vil.BuiltinInt1), raw, raw)
			b.CondFail(bit, "arithmetic overflow")
		}, []string{
			"%3 = struct_extract %1, #Int._value",
			`%4 = builtin "cmp_slt_Int64"(%3, %3) : $Builtin.Int1`,
			`cond_fail %4, "arithmetic overflow"`,
		}},

		{"references and calls", func(b *vil.Block, a []*vil.Value) {
			b.AllocRef(boxT)
			fn := b.FunctionRef(b.Func().Module().Func("callee"))
			b.Apply(fn, intT)
			b.DebugValue(a[1], "n", "let", "argno 2")
		}, []string{
			"%3 = alloc_ref $Box",
			"%4 = function_ref @callee : $@convention(thin) () -> ()",
			"%5 = apply %4() : $@convention(thin) () -> ()",
			`debug_value %1, let, name "n", argno 2`,
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := vil.NewModule("t", vil.StageRaw)
			m.Func("callee")
			f := m.Func("f").SetAttr("ossa")
			b := f.Param(boxT, vil.ParamGuaranteed)
			n := f.Param(intT, vil.ParamUnowned)
			flag := f.Param(boolT, vil.ParamUnowned)
			_ = flag

			bb := f.Entry()
			c.emit(bb, []*vil.Value{b, n, flag})
			bb.Unreachable()

			got := funcText(t, f)
			for _, want := range c.want {
				if !strings.Contains(got, "  "+want+"\n") {
					t.Errorf("printed:\n%s\nmissing line:\n  %s", got, want)
				}
			}
		})
	}
}

// TestTerminatorForms covers the instructions that end a block.
func TestTerminatorForms(t *testing.T) {
	intT := vil.Object(types.Typ[types.Int])
	shape := types.NewNamed("Shape", "", &types.Enum{Name: "Shape"})

	m := vil.NewModule("t", vil.StageRaw)
	f := m.Func("terminators").SetAttr("ossa")
	s := f.Param(vil.Object(shape), vil.ParamUnowned)
	cond := f.Param(vil.Object(vil.BuiltinInt1), vil.ParamUnowned)
	f.SetResult(intT, vil.ResultUnowned)

	entry := f.Entry()
	dot, line, join, dead := f.Block(), f.Block(), f.Block(), f.Block()
	entry.SwitchEnum(s,
		vil.Case{Member: "Shape.dot!enumelt", Dest: dot},
		vil.Case{Member: "Shape.line!enumelt", Dest: line})
	dot.CondBr(cond, join, []*vil.Value{dot.IntegerLiteral(intT, 1)},
		dead, nil)
	line.Br(join, line.IntegerLiteral(intT, 2))
	out := join.Arg(intT, vil.None)
	join.Return(out)
	dead.Unreachable()

	got := funcText(t, f)
	for _, want := range []string{
		"switch_enum %0, case #Shape.dot!enumelt: bb1, case #Shape.line!enumelt: bb2",
		"cond_br %1, bb3(%2), bb4",
		"br bb3(%3)",
		"return %4",
		"unreachable",
	} {
		if !strings.Contains(got, "  "+want+"\n") {
			t.Errorf("printed:\n%s\nmissing line:\n  %s", got, want)
		}
	}
}
