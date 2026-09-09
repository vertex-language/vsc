package vsc_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/ir"

	"github.com/vertex-language/vsc"
)

// The Vertex additions, each asked the one question that would go
// wrong if it were built the obvious way instead of the careful one.

// TestLowercasePrimitives is the alias table: a lowercase spelling and
// its capitalised counterpart are one type, so a program may mix them.
func TestLowercasePrimitives(t *testing.T) {
	const src = `
func widen(_ a: int32, _ b: Int32) -> int64 {
    let ok: bool = a < b
    return ok ? int64(1) : Int64(0)
}
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestOptionalLabels is both directions: a label left off where the
// declaration asks for one, and a label written where it says `_`.
func TestOptionalLabels(t *testing.T) {
	const src = `
func addUp(a: int32, b: int32) -> int32 { return a + b }
func mulBy(_ x: int32, _ y: int32) -> int32 { return x * y }

func use() -> int32 {
    return addUp(1, 2) + addUp(a: 1, b: 2) + mulBy(1, 2) + mulBy(x: 1, y: 2)
}
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestLabelsStillDisambiguate is the one thing a lax rule may not
// take away. Two declarations differing only in their label mangle to
// two symbols, so a call that names neither has to be refused rather
// than guessed at.
func TestLabelsStillDisambiguate(t *testing.T) {
	const src = `
func label(a: int32) -> int32 { return a }
func label(b: int32) -> int32 { return b }

func use() -> int32 { return label(1) }
`
	_, diags := compile(t, src, vsc.Options{Stop: vsc.Checked})
	if !vsc.Errors(diags) {
		t.Fatal("an unlabelled call to two label-only overloads was accepted")
	}
	if !strings.Contains(diags[0].Message, "ambiguous") {
		t.Errorf("diagnostic does not name the ambiguity: %s", diags[0])
	}
}

// TestLabelledOverloadsStillResolve is the other half: with the
// labels written, each call reaches its own declaration.
func TestLabelledOverloadsStillResolve(t *testing.T) {
	const src = `
func label(a: int32) -> int32 { return a }
func label(b: int32) -> int32 { return b }

func use() -> int32 { return label(a: 1) + label(b: 2) }
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestExecModifierRefused says a kernel is refused rather than
// lowered as an ordinary function. Lowering it would produce a
// program that runs on the CPU and returns a right-looking answer,
// which is the failure worth having a test for.
func TestExecModifierRefused(t *testing.T) {
	for _, word := range []string{"kernel", "graph"} {
		src := "func f(_ a: float32) " + word + " -> float32 { return a }"
		if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
			t.Errorf("%s did not typecheck: %v", word, diags)
		}
		_, diags := compile(t, src, vsc.Options{})
		if !vsc.Errors(diags) {
			t.Errorf("%s was lowered as an ordinary function", word)
		}
	}
}

// TestExecWordsStayIdentifiers is what makes the modifier contextual:
// after the parameter list nothing else may appear, so `kernel` there
// is the modifier and `kernel` anywhere else is a name.
func TestExecWordsStayIdentifiers(t *testing.T) {
	const src = `
func kernel(_ graph: int32) -> int32 {
    let kernel: int32 = graph
    return kernel
}
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestPackageClauseIsContextual is the disambiguation rule. `package`
// before a plain identifier is the Vertex clause; before a
// declaration or another modifier it is Swift's access level, and
// both have to keep working in one file.
func TestPackageClauseIsContextual(t *testing.T) {
	const src = `
package geometry

package func hidden() -> int32 { return 1 }
package open class C {}
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestFolderImport is the whole of a string-form import: a folder of
// source is read as the module it declares, its qualified name
// resolves, and the folder comes back as a Package for the caller to
// build and link.
func TestFolderImport(t *testing.T) {
	dir := t.TempDir()
	pkg := filepath.Join(dir, "geometry")
	if err := os.MkdirAll(pkg, 0o777); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(pkg, "area.vs"),
		"package geometry\n\npublic func area(_ w: int32, _ h: int32) -> int32 { return w * h }\n")
	main := filepath.Join(dir, "main.vs")
	write(t, main, "import \"./geometry\"\n\nfunc main() -> int32 { return geometry.area(6, 7) }\n")

	src, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	u, diags := vsc.Compile([]vsc.Source{{Name: main, Text: src}},
		vsc.Options{Module: "main", Target: ir.AArch64MacOS})
	if vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
		t.FailNow()
	}
	if len(u.Packages) != 1 {
		t.Fatalf("got %d packages, want 1", len(u.Packages))
	}
	if u.Packages[0].Name != "geometry" {
		t.Errorf("package named %q, want geometry", u.Packages[0].Name)
	}
	if len(u.Packages[0].Sources) != 1 {
		t.Errorf("package has %d sources, want 1", len(u.Packages[0].Sources))
	}
}

// TestFolderImportNamedByFolder is the default the SwiftPM convention
// argues for: with no package clause, the folder's own name is the
// module's, which is a path's last segment.
func TestFolderImportNamedByFolder(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root", "std", "fmt")
	if err := os.MkdirAll(root, 0o777); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "fmt.vs"), "public func width() -> int32 { return 10 }\n")
	main := filepath.Join(dir, "main.vs")
	write(t, main, "import \"std/fmt\"\n\nfunc main() -> int32 { return fmt.width() }\n")

	src, _ := os.ReadFile(main)
	u, diags := vsc.Compile([]vsc.Source{{Name: main, Text: src}}, vsc.Options{
		Module:       "main",
		Target:       ir.AArch64MacOS,
		PackagePaths: []string{filepath.Join(dir, "root")},
	})
	if vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
		return
	}
	if len(u.Packages) != 1 || u.Packages[0].Name != "fmt" {
		t.Fatalf("packages = %+v, want one named fmt", u.Packages)
	}
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o666); err != nil {
		t.Fatal(err)
	}
}
