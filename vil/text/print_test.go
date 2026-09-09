package text

import (
	"strings"
	"testing"

	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// The expectations in this file are SIL: the text `swiftc
// -emit-silgen` prints for the same shape of function, with the
// symbols renamed. What is compared is everything else — the
// keywords, the conventions, the ownership, the block labels, the
// operand order — because that is what has to agree for the
// differential harness to mean anything.

// TestBorrowedParameter is the shape of `func borrows(_ b: Box) -> Int`.
func TestBorrowedParameter(t *testing.T) {
	box := types.NewNamed("Box", "", &types.Class{Name: "Box"})
	m := vil.NewModule("a", vil.StageRaw)
	f := m.Func("borrows").SetLinkage(vil.Hidden).SetAttr("ossa")
	b := f.Param(vil.Object(box), vil.ParamGuaranteed)
	f.SetResult(vil.Object(types.Typ[types.Int]), vil.ResultUnowned)

	bb := f.Entry()
	n := bb.RefElementAddr(b, "Box.n", vil.Object(types.Typ[types.Int]))
	v := bb.Load(n, "trivial")
	bb.Return(v)

	want := `sil hidden [ossa] @borrows : $@convention(thin) (@guaranteed Box) -> Int {
bb0(%0 : @guaranteed $Box):
  %1 = ref_element_addr %0, #Box.n
  %2 = load [trivial] %1
  return %2
} // end sil function 'borrows'
`
	got := funcText(t, f)
	if got != want {
		t.Errorf("printed:\n%s\nwant:\n%s", got, want)
	}
}

// TestSwitchEnum is the shape of a switch over an enum with a
// payload: block arguments carry the payload, exactly as SIL does it.
func TestSwitchEnum(t *testing.T) {
	shape := types.NewNamed("Shape", "", &types.Enum{
		Name: "Shape",
		Cases: []*types.EnumCase{
			{Name: "dot"},
			{Name: "line", AssociatedType: types.Typ[types.Int]},
		},
	})
	intT := vil.Object(types.Typ[types.Int])

	m := vil.NewModule("a", vil.StageRaw)
	f := m.Func("matches").SetLinkage(vil.Hidden).SetAttr("ossa")
	s := f.Param(vil.Object(shape), vil.ParamUnowned)
	f.SetResult(intT, vil.ResultUnowned)

	entry := f.Entry()
	dot, line, join := f.Block(), f.Block(), f.Block()
	entry.SwitchEnum(s,
		vil.Case{Member: "Shape.dot!enumelt", Dest: dot},
		vil.Case{Member: "Shape.line!enumelt", Dest: line})

	zero := dot.IntegerLiteral(vil.Object(vil.BuiltinIntLiteral), 0)
	dot.Br(join, zero)

	k := line.Arg(intT, vil.None)
	line.Br(join, k)

	out := join.Arg(intT, vil.None)
	join.Return(out)

	want := `sil hidden [ossa] @matches : $@convention(thin) (Shape) -> Int {
bb0(%0 : $Shape):
  switch_enum %0, case #Shape.dot!enumelt: bb1, case #Shape.line!enumelt: bb2

bb1:                                              // Preds: bb0
  %1 = integer_literal $Builtin.IntLiteral, 0
  br bb3(%1)

bb2(%2 : $Int):                                   // Preds: bb0
  br bb3(%2)

bb3(%3 : $Int):                                   // Preds: bb1 bb2
  return %3
} // end sil function 'matches'
`
	got := funcText(t, f)
	if got != want {
		t.Errorf("printed:\n%s\nwant:\n%s", got, want)
	}
}

// TestOwnership prints the instructions the whole IR exists for.
func TestOwnership(t *testing.T) {
	box := types.NewNamed("Box", "", &types.Class{Name: "Box"})
	m := vil.NewModule("a", vil.StageRaw)
	f := m.Func("owns").SetLinkage(vil.Hidden).SetAttr("ossa")
	b := f.Param(vil.Object(box), vil.ParamOwned)
	f.SetResult(vil.Object(box), vil.ResultOwned)

	bb := f.Entry()
	c := bb.CopyValue(b)
	borrow := bb.BeginBorrow(c)
	bb.EndBorrow(borrow)
	bb.DestroyValue(b)
	bb.Return(c)

	want := `sil hidden [ossa] @owns : $@convention(thin) (@owned Box) -> @owned Box {
bb0(%0 : @owned $Box):
  %1 = copy_value %0
  %2 = begin_borrow %1
  end_borrow %2
  destroy_value %0
  return %1
} // end sil function 'owns'
`
	got := funcText(t, f)
	if got != want {
		t.Errorf("printed:\n%s\nwant:\n%s", got, want)
	}
}

func funcText(t *testing.T, f *vil.Func) string {
	t.Helper()
	var b strings.Builder
	if err := Func(&b, f); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
