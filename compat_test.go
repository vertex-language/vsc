package vsc_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/ir"

	"github.com/vertex-language/vsc"
)

// Gaps in the Swift compatibility layer that were once wrong answers
// rather than missing features. Each test is the program that was
// mis-compiled, kept so it cannot become one again.

// TestNilCoalescingNarrowWrapped: `a ?? 0` against an int32? left the
// literal at its default of Int, failed the assignability test, and
// fell out of the case still optional -- a wrong type, reported as a
// return-type mismatch the source could not fix.
func TestNilCoalescingNarrowWrapped(t *testing.T) {
	const src = `
func pick(_ a: int32?) -> int32 { return a ?? 0 }
func widen(_ a: int?) -> int { return a ?? 0 }
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestNilCoalescingKeepsOptionalForm is Swift's other overload:
// `T? ?? T?` is a T?, not a T.
func TestNilCoalescingKeepsOptionalForm(t *testing.T) {
	const src = `
func pick(_ a: int32?, _ b: int32?) -> int32? { return a ?? b }
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestNilCoalescingMismatch: operands that genuinely disagree are a
// diagnostic, not a quietly optional result.
func TestNilCoalescingMismatch(t *testing.T) {
	const src = `
func pick(_ a: int32?, _ b: bool) -> int32 { return a ?? b }
`
	_, diags := compile(t, src, vsc.Options{Stop: vsc.Checked})
	if !vsc.Errors(diags) {
		t.Fatal("'??' with mismatched operands was accepted")
	}
	if !strings.Contains(diags[0].Message, "'??'") {
		t.Errorf("diagnostic does not name the operator: %s", diags[0])
	}
}

// TestMutatingOnClassRefused: Swift rejects `mutating` on a class
// method outright. Accepting it silently let the modifier mean
// nothing.
func TestMutatingOnClassRefused(t *testing.T) {
	const src = `
class Box {
    var v: int32 = 0
    mutating func bump() { v = v + 1 }
}
`
	_, diags := compile(t, src, vsc.Options{Stop: vsc.Checked})
	if !vsc.Errors(diags) {
		t.Fatal("'mutating' on a class method was accepted")
	}
	if !strings.Contains(diags[0].Message, "'mutating' is not valid on instance methods") {
		t.Errorf("diagnostic does not say why: %s", diags[0])
	}
}

// TestMutatingOnStructAccepted is the other side of it: the modifier
// still means what it means on a value type.
func TestMutatingOnStructAccepted(t *testing.T) {
	const src = `
struct S {
    var v: int32 = 0
    mutating func bump() { self.v = 1 }
}
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestTopLevelCodeReported: statements at file scope were checked and
// then discarded, and the only symptom was an undefined _main at the
// link. They are reported now.
func TestTopLevelCodeReported(t *testing.T) {
	const src = `
func side(_ a: int32) -> int32 { return a }

side(3)

func main() -> int32 { return 0 }
`
	_, diags := compile(t, src, vsc.Options{Module: "main"})
	if !vsc.Errors(diags) {
		t.Fatal("top-level code was accepted and dropped")
	}
	if !strings.Contains(diags[0].Message, "top-level code") {
		t.Errorf("diagnostic does not name the problem: %s", diags[0])
	}
}

// TestNoEntryPointReported: a program with no main is the compiler's
// to report, in its own words, rather than the linker's as an
// undefined symbol.
func TestNoEntryPointReported(t *testing.T) {
	const src = `func add(_ a: int32) -> int32 { return a }`
	_, diags := compile(t, src, vsc.Options{Module: "main"})
	if !vsc.Errors(diags) {
		t.Fatal("a program with no entry point was accepted")
	}
	if !strings.Contains(diags[0].Message, "no entry point") {
		t.Errorf("diagnostic does not name the problem: %s", diags[0])
	}
}

// TestLibraryNeedsNoEntryPoint: the rule is about the entry module.
// Any other module's main is an ordinary function, and a library need
// not have one at all.
func TestLibraryNeedsNoEntryPoint(t *testing.T) {
	const src = `public func add(_ a: int32) -> int32 { return a }`
	if _, diags := compile(t, src, vsc.Options{Module: "lib"}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestTwoModulesSharingAName: every import was declared into one
// shared scope, and Scope.Insert keeps the first symbol of a name --
// so the second module's same-named declaration existed nowhere, and
// its qualified name could not be resolved. Each module now has a
// scope of its own, with no parent, so a qualified name is exact.
func TestTwoModulesSharingAName(t *testing.T) {
	iface := func(name string) vsc.Source {
		return vsc.Source{Name: name + ".vertexinterface", Text: []byte(
			"// vertex-interface-format-version: 1.0\n" +
				"// vertex-module-name: " + name + "\n\n" +
				"public func width() -> Int32\n")}
	}
	dir := t.TempDir()
	for _, name := range []string{"A", "B"} {
		src := iface(name)
		write(t, filepath.Join(dir, src.Name), string(src.Text))
	}
	const prog = `
import A
import B

func main() -> int32 { return A.width() + B.width() }
`
	_, diags := vsc.Compile(
		[]vsc.Source{{Name: "m.vs", Text: []byte(prog)}},
		vsc.Options{Module: "main", Target: ir.AArch64MacOS, Stop: vsc.Checked,
			ImportPaths: []string{dir}})
	for _, d := range diags {
		t.Errorf("%s", d)
	}
}
