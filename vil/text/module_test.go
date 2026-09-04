package text

import (
	"strings"
	"testing"

	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// A module is more than its functions. What it holds besides — the
// stage it is at, what it imports, its globals, and the two tables
// that say how dynamic dispatch and protocol conformance resolve —
// all print, and this is what they print as. Every form here is the
// one `swiftc -emit-silgen` writes for the same declaration.

func TestModuleText(t *testing.T) {
	m := vil.NewModule("tbl", vil.StageRaw)
	m.Import("Builtin")
	m.Import("Swift")
	m.Global("counter", vil.Object(types.Typ[types.Int]), vil.Hidden)

	// A declaration: no blocks, and so no braces.
	ext := m.Func("draw")
	ext.Type().Convention = vil.Method
	ext.SetResult(vil.Object(types.Typ[types.Int]), vil.ResultUnowned)

	m.VTable("Shape").
		Entry("Shape.draw", "draw").
		Entry("Shape.deinit!deallocator", "deinit")

	m.WitnessTable("Shape", "Drawable", "tbl", vil.Hidden).
		Entry("Drawable.draw", "witness")

	want := `sil_stage raw

import Builtin
import Swift

sil_global hidden @counter : $Int

sil @draw : $@convention(method) () -> Int

sil_vtable Shape {
  #Shape.draw: @draw
  #Shape.deinit!deallocator: @deinit
}

sil_witness_table hidden Shape: Drawable module tbl {
  method #Drawable.draw: @witness
}

`
	if got := String(m); got != want {
		t.Errorf("printed:\n%s\nwant:\n%s", got, want)
	}
}

// TestStagePrints: which stage a module is at is its first line,
// because what may be assumed about the rest depends on the answer.
func TestStagePrints(t *testing.T) {
	for _, stage := range []vil.Stage{vil.StageRaw, vil.StageCanonical, vil.StageLowered} {
		m := vil.NewModule("m", stage)
		want := "sil_stage " + string(stage) + "\n"
		if got := String(m); !strings.HasPrefix(got, want) {
			t.Errorf("a %s module starts %q, want %q", stage, got, want)
		}
	}
}

// TestDeclarationHasNoBody: a function with no blocks is a
// declaration, and SIL writes it without braces — which is how an
// external symbol is referred to.
func TestDeclarationHasNoBody(t *testing.T) {
	m := vil.NewModule("m", vil.StageRaw)
	f := m.Func("external").SetLinkage(vil.PublicExternal)
	f.SetResult(vil.Object(types.Typ[types.Int]), vil.ResultUnowned)

	got := String(m)
	if strings.Contains(got, "{") {
		t.Errorf("a declaration should have no body:\n%s", got)
	}
	if !strings.Contains(got, "sil public_external @external : $@convention(thin) () -> Int") {
		t.Errorf("declaration printed as:\n%s", got)
	}
}
