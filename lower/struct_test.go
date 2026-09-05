package lower

import (
	"strings"
	"testing"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/vil/gen"
	"github.com/vertex-language/vsc/vil/pass"
)

// lowerSrc takes source all the way to VIR, returning whatever this
// package said about it. It is lowered() over a string rather than a
// file, since these programs are about one type each and a testdata
// file per type would be a file per sentence.
func lowerSrc(t *testing.T, src string) (*ir.Module, error) {
	t.Helper()
	f := token.NewFile("t.swift", []byte(src))
	file, diags := parser.ParseFile(f, 0)
	for _, d := range diags {
		t.Fatalf("parse: %s", d.Print(f))
	}
	info, checks := analyzer.Check([]*ast.File{file})
	for _, d := range checks {
		t.Fatalf("check: %s", d.Print(f))
	}
	m, gens := gen.File("t", file, info)
	for _, d := range gens {
		t.Fatalf("gen: %s", d.Print(f))
	}
	if err := pass.Mandatory(m); err != nil {
		t.Fatalf("mandatory: %v", err)
	}
	if err := pass.LowerOwnership(m); err != nil {
		t.Fatal(err)
	}
	return Module(m, target, Options{})
}

// Structs, and how far one gets.
//
// A struct of one field is that field: nothing is built, nothing is
// taken apart, and the register does not know the difference. More
// than one field is more than one register — the same shape an
// overflow-reporting builtin already produces — which works inside a
// function and stops at a call boundary, because crossing one needs
// an ABI rather than a register.

// TestAStructOfOneFieldIsThatField: the rule has to hold in the
// machine type as well as in the instructions, or a value that lowers
// inside a function has no type to be passed by.
func TestAStructOfOneFieldIsThatField(t *testing.T) {
	m, err := lowerSrc(t, `
struct Meters { var value: Int32 }
func add(_ a: Meters, _ b: Meters) -> Meters {
    return Meters(value: a.value + b.value)
}
`)
	if err != nil {
		t.Fatalf("a wrapper did not lower: %v", err)
	}
	// A wrapper costs nothing: the parameters are the field's
	// register, and no aggregate appears in the output.
	text := dump(t, m)
	if !strings.Contains(text, "i32") {
		t.Errorf("the wrapper did not lower to its field's register:\n%s", text)
	}
}

// TestAStructCrossesInWords: up to four words a struct is passed in
// that many registers, which is Swift's rule and what `swiftc -O`
// emits for AArch64. Two Int32s are eight bytes, so one register.
func TestAStructCrossesInWords(t *testing.T) {
	m, err := lowerSrc(t, `
struct P { var x: Int32; var y: Int32 }
func sum(_ p: P) -> Int32 { return p.x + p.y }
`)
	if err != nil {
		t.Fatalf("a struct did not cross a call boundary: %v", err)
	}
	got := dump(t, m)
	// One parameter, not two: the fields share a word, and the callee
	// takes them apart.
	if !strings.Contains(got, "(%a0 i64) i32") {
		t.Errorf("the struct was not passed as one packed word:\n%s", got)
	}
}

// TestATooWideStructIsRefused: past four words Swift passes by
// address, which needs a memory layout this package does not do — and
// the message says which limit was crossed rather than reporting an
// unknown type.
func TestATooWideStructIsRefused(t *testing.T) {
	_, err := lowerSrc(t, `
struct TooWide {
    var a: Int; var b: Int; var c: Int; var d: Int; var e: Int
}
func first(_ w: TooWide) -> Int { return w.a }
`)
	if err == nil {
		t.Fatal("a struct too wide for registers crossed a call boundary")
	}
	msg := err.Error()
	for _, want := range []string{"32 bytes", "by address"} {
		if !strings.Contains(msg, want) {
			t.Errorf("said %q, want it to mention %q", msg, want)
		}
	}
}

// TestAWiderStructWorksInsideAFunction: constructed, and its fields
// read back, with nothing emitted for either — the fields were
// computed where they were written.
func TestAWiderStructWorksInsideAFunction(t *testing.T) {
	if _, err := lowerSrc(t, `
struct P { var x: Int32; var y: Int32 }
func total() -> Int32 {
    let p = P(x: 40, y: 2)
    return p.x + p.y
}
`); err != nil {
		t.Errorf("a struct did not lower inside a function: %v", err)
	}
}
