package pass

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
	"github.com/vertex-language/vsc/vil/gen"
	"github.com/vertex-language/vsc/vil/text"
	"github.com/vertex-language/vsc/vil/verify"
)

func classType(name string) types.Type {
	return types.NewNamed(name, "", &types.Class{Name: name})
}

// TestLowerOwnership is the translation, one instruction at a time.
// The shapes are what `swiftc -emit-sil` prints for the same program,
// short of the ARC optimization we do not do: a retain immediately
// followed by a release is correct and redundant, and removing it is
// vil/opt's job rather than this one's.
func TestLowerOwnership(t *testing.T) {
	boxT := vil.Object(classType("Box"))

	m := vil.NewModule("t", vil.StageRaw)
	f := m.Func("keeps").SetLinkage(vil.Hidden).SetAttr("ossa")
	b := f.Param(boxT, vil.ParamGuaranteed)
	f.SetResult(boxT, vil.ResultOwned)

	bb := f.Entry()
	copied := bb.CopyValue(b)
	borrow := bb.BeginBorrow(copied)
	again := bb.CopyValue(borrow)
	bb.EndBorrow(borrow)
	bb.DestroyValue(copied)
	bb.Return(again)

	if err := LowerOwnership(m); err != nil {
		t.Fatal(err)
	}

	want := `sil hidden @keeps : $@convention(thin) (@guaranteed Box) -> @owned Box {
bb0(%0 : $Box):
  strong_retain %0
  strong_retain %0
  strong_release %0
  return %0
} // end sil function 'keeps'
`
	var got strings.Builder
	text.Func(&got, f)
	if got.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", got.String(), want)
	}
}

// TestConventionsSurvive: what a parameter is passed as stays in the
// function's type after the pass. It is the contract between caller
// and callee — ABI, which the backend needs — rather than a fact
// about a value inside the body.
func TestConventionsSurvive(t *testing.T) {
	boxT := vil.Object(classType("Box"))
	m := vil.NewModule("t", vil.StageRaw)
	f := m.Func("consumes").SetAttr("ossa")
	b := f.Param(boxT, vil.ParamOwned)
	f.SetResult(boxT, vil.ResultOwned)
	f.Entry().Return(b)

	LowerOwnership(m)

	if got := f.Type().String(); !strings.Contains(got, "@owned Box) -> @owned Box") {
		t.Errorf("the type lost its conventions: %s", got)
	}
	if f.OSSA() {
		t.Error("the function is still marked [ossa]")
	}
	if got := f.Entry().Args()[0].Ownership(); got != vil.None {
		t.Errorf("a block argument still owns something: %v", got)
	}
}

// TestAccessScopesSurvive: an access scope is not ownership. It is
// where exclusivity is enforced, and enforcement outlives the form.
func TestAccessScopesSurvive(t *testing.T) {
	intT := vil.Object(types.Typ[types.Int])
	m := vil.NewModule("t", vil.StageRaw)
	f := m.Func("reads").SetAttr("ossa")
	f.SetResult(intT, vil.ResultUnowned)

	bb := f.Entry()
	addr := bb.AllocStack(intT)
	acc := bb.BeginAccess(addr, "read", "unknown")
	v := bb.Load(acc, "trivial")
	bb.EndAccess(acc)
	bb.DeallocStack(addr)
	bb.Return(v)

	LowerOwnership(m)

	var got strings.Builder
	text.Func(&got, f)
	for _, want := range []string{"begin_access [read] [unknown]", "end_access"} {
		if !strings.Contains(got.String(), want) {
			t.Errorf("%q did not survive:\n%s", want, got.String())
		}
	}
}

// TestStageMoves: the stage is the record of what has been done, and
// a pass that changes the module says so.
func TestStageMoves(t *testing.T) {
	m := vil.NewModule("t", vil.StageRaw)
	f := m.Func("f").SetAttr("ossa")
	f.SetResult(vil.Object(types.Typ[types.Void]), vil.ResultUnowned)
	bb := f.Entry()
	bb.Return(bb.Tuple(vil.Object(types.Typ[types.Void])))

	if err := Mandatory(m); err != nil {
		t.Fatal(err)
	}
	if m.Stage() != vil.StageCanonical {
		t.Errorf("after Mandatory the module is %s", m.Stage())
	}
	LowerOwnership(m)
	if m.Stage() != vil.StageLowered {
		t.Errorf("after LowerOwnership the module is %s", m.Stage())
	}
}

// TestMandatoryRejects: a pass here may refuse a program, which is
// the whole difference between this package and an optimizer.
func TestMandatoryRejects(t *testing.T) {
	boxT := vil.Object(classType("Box"))
	m := vil.NewModule("t", vil.StageRaw)
	f := m.Func("leaks").SetAttr("ossa")
	f.Param(boxT, vil.ParamOwned) // never consumed
	f.SetResult(vil.Object(types.Typ[types.Void]), vil.ResultUnowned)
	bb := f.Entry()
	bb.Return(bb.Tuple(vil.Object(types.Typ[types.Void])))

	if err := Mandatory(m); err == nil {
		t.Error("a leaking function reached canonical")
	}
	if m.Stage() != vil.StageRaw {
		t.Errorf("a module that was refused moved to %s", m.Stage())
	}
}

// TestCorpusLowers takes every program vil/gen is held to, runs it
// through the whole pipeline, and requires two things: that it still
// verifies, and that nothing of the ownership form is left.
//
// The second is what the pass is for. An instruction VIR has no
// notion of must not reach a backend that has no notion of it.
func TestCorpusLowers(t *testing.T) {
	files, err := filepath.Glob("../gen/testdata/*.swift")
	if err != nil || len(files) == 0 {
		t.Skip("no corpus")
	}
	ownership := []string{"copy_value", "destroy_value", "begin_borrow",
		"end_borrow", "move_value", "extend_lifetime", "[ossa]"}

	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			m := lowerFile(t, path)
			if err := Mandatory(m); err != nil {
				t.Fatalf("mandatory: %v", err)
			}
			if err := LowerOwnership(m); err != nil {
				t.Fatal(err)
			}
			if err := verify.Module(m); err != nil {
				t.Errorf("does not verify after lowering: %v\n\n%s", err, text.String(m))
			}
			out := text.String(m)
			for _, gone := range ownership {
				if strings.Contains(out, gone) {
					t.Errorf("%s survived the eliminator:\n%s", gone, out)
				}
			}
		})
	}
}

func lowerFile(t *testing.T, path string) *vil.Module {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f := token.NewFile(path, src)
	file, diags := parser.ParseFile(f, 0)
	for _, d := range diags {
		t.Fatalf("parse: %s", d.Print(f))
	}
	info, checks := analyzer.Check([]*ast.File{file})
	for _, d := range checks {
		t.Fatalf("check: %s", d.Print(f))
	}
	return gen.File("t", file, info)
}
