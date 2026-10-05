package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
)

// missingReturns has a body that falls off its end in each kind of
// function Swift names, and bodies that only look as though they might:
// every path returns, the loop never ends, or the body is one expression.
const missingReturns = `func f(_ b: Bool) -> Int32 {
    if b { return 1 }
}
struct S {
    func m(_ b: Bool) -> Int { if b { return 1 } }
    static func s(_ b: Bool) -> Int { if b { return 1 } }
    var g: Int { if true { } }
}
func forever() -> Int32 { while true { } }
func both(_ b: Bool) -> Int32 { if b { return 1 } else { return 2 } }
func cases(_ x: Int) -> Int32 { switch x { case 0: return 0; default: return 1 } }
func implicit() -> Int32 { 7 }
func outer() -> Int { func inner(_ b: Bool) -> Int { if b { return 1 } }; return inner(true) }
let c = { (b: Bool) -> Int in if b { return 1 } }
func main() -> Int32 { return 0 }
`

// missingReturnErrors is what this compiler reports for src, as
// "line:col: message", sorted.
func missingReturnErrors(t *testing.T, src string) []string {
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
	_, gens := File("main", file, info)
	var out []string
	for _, d := range gens {
		out = append(out, strings.TrimPrefix(d.Print(f), "main.swift:"))
	}
	sort.Strings(out)
	return out
}

// TestMissingReturn: each body that can reach its end is reported once,
// at its closing brace, and no other is.
func TestMissingReturn(t *testing.T) {
	got := missingReturnErrors(t, missingReturns)
	want := []string{
		"13:72: error: missing return in local function expected to return 'Int'",
		"14:49: error: missing return in closure expected to return 'Int'",
		"3:1: error: missing return in global function expected to return 'Int32'",
		"5:50: error: missing return in instance method expected to return 'Int'",
		"6:57: error: missing return in static method expected to return 'Int'",
		"7:30: error: missing return in getter expected to return 'Int'",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("reported:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// TestMissingReturnAgreesWithSwiftc holds TestMissingReturn's list to
// swiftc's own, which reports these from its SIL diagnostics.
func TestMissingReturnAgreesWithSwiftc(t *testing.T) {
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH")
	}
	path := filepath.Join(t.TempDir(), "main.swift")
	if err := os.WriteFile(path, []byte(missingReturns), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _ := exec.Command(swiftc, "-emit-sil", "-o", os.DevNull, path).CombinedOutput()
	var want []string
	for _, line := range strings.Split(string(out), "\n") {
		if rest, ok := strings.CutPrefix(line, path+":"); ok && strings.Contains(rest, ": error: ") {
			want = append(want, rest)
		}
	}
	sort.Strings(want)
	got := missingReturnErrors(t, missingReturns)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("vsc:\n  %s\nswiftc:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
