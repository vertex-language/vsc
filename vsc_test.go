package vsc_test

import (
	"strings"
	"testing"

	"github.com/vertex-language/ir"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/vil"
)

const fib = `
public func fib(_ n: Int) -> Int {
  if n < 2 { return n }
  return fib(n - 1) + fib(n - 2)
}
`

func compile(t *testing.T, src string, opts vsc.Options) (*vsc.Unit, []vsc.Diagnostic) {
	t.Helper()
	if opts.Module == "" {
		opts.Module = "t"
	}
	if opts.Target.Use() == "" {
		opts.Target = ir.AArch64MacOS
	}
	return vsc.Compile([]vsc.Source{{Name: "t.swift", Text: []byte(src)}}, opts)
}

// TestCompile is the whole compiler in one call: Swift in, VIR out,
// nothing said along the way.
func TestCompile(t *testing.T) {
	u, diags := compile(t, fib, vsc.Options{})
	for _, d := range diags {
		t.Errorf("%s", d)
	}
	if u.VIR == nil {
		t.Fatal("no machine IR")
	}
	if len(u.Files) != 1 || u.Info == nil || u.VIL == nil {
		t.Error("a phase left nothing behind")
	}
	if got := u.VIL.Stage(); got != vil.StageLowered {
		t.Errorf("the ownership IR is %s, want lowered", got)
	}
	if u.VIR.Lookup("_$s1t3fibyS2iF") == nil {
		t.Errorf("no symbol for fib; the module has %d functions", len(u.VIR.Funcs()))
	}
}

// TestStop: a caller that wants the syntax tree should not pay for
// instruction selection, and one that wants the ownership IR should
// get it in the form the phase it named leaves it in.
func TestStop(t *testing.T) {
	for _, c := range []struct {
		stop  vsc.Phase
		stage vil.Stage
		vil   bool
		vir   bool
	}{
		{vsc.Parsed, "", false, false},
		{vsc.Checked, "", false, false},
		{vsc.Raw, vil.StageRaw, true, false},
		{vsc.Canonical, vil.StageCanonical, true, false},
		{vsc.Lowered, vil.StageLowered, true, false},
		{vsc.All, vil.StageLowered, true, true},
	} {
		t.Run(string(c.stage)+"/"+itoa(int(c.stop)), func(t *testing.T) {
			u, diags := compile(t, fib, vsc.Options{Stop: c.stop})
			for _, d := range diags {
				t.Errorf("%s", d)
			}
			if (u.VIL != nil) != c.vil {
				t.Errorf("ownership IR present = %v, want %v", u.VIL != nil, c.vil)
			}
			if (u.VIR != nil) != c.vir {
				t.Errorf("machine IR present = %v, want %v", u.VIR != nil, c.vir)
			}
			if c.vil && u.VIL.Stage() != c.stage {
				t.Errorf("stage is %s, want %s", u.VIL.Stage(), c.stage)
			}
		})
	}
}

// TestStopsAtTheFirstPhaseThatFails: a file that does not parse is not
// then typechecked, and the diagnostics are the parser's rather than
// the consequences of a tree that was never built.
func TestStopsAtTheFirstPhaseThatFails(t *testing.T) {
	u, diags := compile(t, "func f( {", vsc.Options{})
	if !vsc.Errors(diags) {
		t.Fatal("a file that does not parse compiled")
	}
	if u.Info != nil {
		t.Error("the checker ran on a tree the parser rejected")
	}
	if u.VIR != nil {
		t.Error("something was lowered")
	}
}

// TestDiagnosticsSayWhere: a diagnostic prints as a place and a
// message, because that is what a caller has to show someone.
func TestDiagnosticsSayWhere(t *testing.T) {
	_, diags := compile(t, "func f() -> Int { return \"no\" }", vsc.Options{})
	if !vsc.Errors(diags) {
		t.Fatal("a type error compiled")
	}
	got := diags[0].String()
	if !strings.HasPrefix(got, "t.swift:") {
		t.Errorf("%q does not begin with the file and line", got)
	}
}

// TestModuleNamesTheSymbols: the module is part of every symbol, so
// compiling the same source as two modules gives two sets of symbols
// that can be linked together.
func TestModuleNamesTheSymbols(t *testing.T) {
	a, _ := compile(t, fib, vsc.Options{Module: "one"})
	b, _ := compile(t, fib, vsc.Options{Module: "two"})
	if a.VIR == nil || b.VIR == nil {
		t.Fatal("did not compile")
	}
	if a.VIR.Lookup("_$s3one3fibyS2iF") == nil || b.VIR.Lookup("_$s3two3fibyS2iF") == nil {
		t.Error("the module name is not in the symbols")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	return string(rune('0' + n))
}
