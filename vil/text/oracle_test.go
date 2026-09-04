package text

import (
	"os"
	"testing"

	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
	"github.com/vertex-language/vsc/vil/verify"
)

// The differential harness.
//
// testdata holds SIL as `swiftc -emit-silgen` printed it. A test
// builds the same function in VIL, prints it, normalizes both sides,
// and requires them to agree. Once vil/gen exists this is what it
// will be held to; until then it is what holds the text form to
// Swift's, which is the only reason the text form is Swift's.
//
// Normalization is the one licence taken, and it covers exactly three
// things that differ without meaning anything:
//
//   - symbols, because VIL does not clone Swift's mangling
//   - the '%n' numbering, which follows from the symbols
//   - the trailing '// user:' and '// id:' cross-references, which
//     restate the def-use graph the instructions already carry

// TestMatchesSwiftSIL builds `func borrows(_ b: Box) -> Int` in VIL
// and requires it to print as swiftc printed the same function.
func TestMatchesSwiftSIL(t *testing.T) {
	want, err := os.ReadFile("testdata/borrows.sil")
	if err != nil {
		t.Skip("no testdata")
	}

	box := types.NewNamed("Box", "", &types.Class{Name: "Box"})
	intT := vil.Object(types.Typ[types.Int])

	m := vil.NewModule("a", vil.StageRaw)
	f := m.Func("borrows").SetLinkage(vil.Hidden).SetAttr("ossa")
	b := f.Param(vil.Object(box), vil.ParamGuaranteed)
	f.SetResult(intT, vil.ResultUnowned)

	bb := f.Entry()
	bb.DebugValue(b, "b", "let", "argno 1")
	addr := bb.RefElementAddr(b, "Box.n", intT)
	access := bb.BeginAccess(addr, "read", "dynamic")
	v := bb.Load(access, "trivial")
	bb.EndAccess(access)
	bb.Return(v)

	got := Normalize(funcText(t, f))
	if w := Normalize(string(want)); got != w {
		t.Errorf("VIL and SIL disagree.\n--- vil\n%s\n--- sil\n%s", got, w)
	}
	// Anything that prints as the SIL swiftc printed must also be
	// sound by our own rules. If it is not, one of the two is wrong.
	if err := verify.Func(f); err != nil {
		t.Errorf("prints as SIL but does not verify: %v", err)
	}
}

// TestOwnedParameter is `func consumes(_ b: __owned Box) -> Box`: the
// function that made every ownership decision this IR exists for —
// borrow, copy, end the borrow, destroy what was given, return what
// was made.
func TestOwnedParameter(t *testing.T) {
	want, err := os.ReadFile("testdata/consumes.sil")
	if err != nil {
		t.Skip("no testdata")
	}

	box := types.NewNamed("Box", "", &types.Class{Name: "Box"})
	m := vil.NewModule("a", vil.StageRaw)
	f := m.Func("consumes").SetLinkage(vil.Hidden).SetAttr("ossa")
	b := f.Param(vil.Object(box), vil.ParamOwned)
	f.SetResult(vil.Object(box), vil.ResultOwned)

	bb := f.Entry()
	bb.DebugValue(b, "b", "let", "argno 1")
	borrow := bb.BeginBorrow(b)
	copied := bb.CopyValue(borrow)
	bb.EndBorrow(borrow)
	bb.DestroyValue(b)
	bb.Return(copied)

	got := Normalize(funcText(t, f))
	if w := Normalize(string(want)); got != w {
		t.Errorf("VIL and SIL disagree.\n--- vil\n%s\n--- sil\n%s", got, w)
	}
	if err := verify.Func(f); err != nil {
		t.Errorf("prints as SIL but does not verify: %v", err)
	}

	// And the ownership the builder gave each value is the ownership
	// the text says: the copy is owned, the borrow is not.
	if copied.Ownership() != vil.Owned {
		t.Errorf("copy_value produced %v, want @owned", copied.Ownership())
	}
	if borrow.Ownership() != vil.Guaranteed {
		t.Errorf("begin_borrow produced %v, want @guaranteed", borrow.Ownership())
	}
	if got := len(b.Consumers()); got != 1 {
		t.Errorf("the owned parameter has %d consumers, want exactly one", got)
	}
}
