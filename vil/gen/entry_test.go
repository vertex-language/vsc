package gen

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/vil/text"
)

// generate lowers src as the named module and returns the SIL text
// and whatever this package said about it. Unlike lower(), a
// diagnostic is returned rather than fatal: half of what the entry
// point owes is refusing the shapes it cannot use.
func generate(t *testing.T, module, src string) (string, []token.Diagnostic) {
	t.Helper()
	f := token.NewFile("main.swift", []byte(src))
	file, diags := parser.ParseFile(f, 0)
	for _, d := range diags {
		t.Fatalf("parse: %s", d.Print(f))
	}
	info, checks := analyzer.Check([]*ast.File{file})
	for _, d := range checks {
		t.Fatalf("check: %s", d.Print(f))
	}
	m, gens := File(module, file, info)
	var buf bytes.Buffer
	if err := text.Print(&buf, m); err != nil {
		t.Fatal(err)
	}
	return buf.String(), gens
}

// TestEntryPoint: main in module main is the symbol a linker looks
// for, spelled the way SILGen spells Swift's own.
//
// The oracle is `swiftc -emit-silgen` on a file with top-level code,
// which writes
//
//	sil [ossa] @main : $@convention(c) (Int32, ...) -> Int32
//
// and ends it with an integer_literal, a struct, and a return. The
// parameters are the documented difference: argc and argv are a
// runtime's to hand over and there is no runtime yet.
func TestEntryPoint(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      []string
	}{
		{"returns nothing, exits zero", `func main() {}`, []string{
			"sil [ossa] @main : $@convention(c) () -> Int32 {",
			"%0 = integer_literal $Builtin.Int32, 0",
			"%1 = struct $Int32 (%0)",
			"return %1",
		}},
		{"returns a status", `func main() -> Int32 { return 3 }`, []string{
			"sil [ossa] @main : $@convention(c) () -> Int32 {",
			"%0 = integer_literal $Builtin.Int32, 3",
			"return %1",
		}},
		{"an empty return still exits zero", `
func main() {
    return
}`, []string{
			"%0 = integer_literal $Builtin.Int32, 0",
			"return %1",
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, diags := generate(t, "main", c.src)
			for _, d := range diags {
				t.Errorf("gen: %s", d.Message)
			}
			for _, want := range c.want {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in\n%s", want, got)
				}
			}
		})
	}
}

// TestEntryPointIsTheModuleNamed: a library that declares a helper
// called main keeps its mangled symbol.
//
// Which module is the program is the caller's to say, and this is why
// it has to be: two objects that both defined `main` would be two
// programs, and the linker would refuse them together.
func TestEntryPointIsTheModuleNamed(t *testing.T) {
	got, diags := generate(t, "lib", `public func main() {}`)
	for _, d := range diags {
		t.Errorf("gen: %s", d.Message)
	}
	if strings.Contains(got, "@main :") {
		t.Errorf("a library's main became the entry point:\n%s", got)
	}
	if !strings.Contains(got, "@$s3lib4mainyyF") {
		t.Errorf("a library's main was not mangled:\n%s", got)
	}
}

// TestEntryPointRefused: a main this compiler cannot start a program
// at is a diagnostic here rather than an unresolved symbol later.
//
// Leaving it mangled would link to nothing and say so from the
// linker, which is three phases too late and in a vocabulary that
// does not mention the source.
func TestEntryPointRefused(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"parameters", `func main(_ n: Int) {}`, "takes parameters"},
		{"a result that is not a status", `func main() -> Int { return 0 }`, "returns Int"},
		{"throws", `func main() throws {}`, "throws"},
		{"async", `func main() async {}`, "is async"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, diags := generate(t, "main", c.src)
			if len(diags) == 0 {
				t.Fatalf("accepted %q:\n%s", c.src, got)
			}
			if !strings.Contains(diags[0].Message, c.want) {
				t.Errorf("said %q, want it to mention %q", diags[0].Message, c.want)
			}
			if strings.Contains(got, "@main") {
				t.Errorf("refused it and emitted it anyway:\n%s", got)
			}
		})
	}
}
