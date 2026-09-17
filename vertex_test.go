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

// TestLowercaseSpellingsConstruct: a lowercase name has to work
// where its capitalised counterpart does, in expression position as
// well as in type position. The aliases lived only in
// LookupUniverse and had no symbol, so `int32(x)` reached lowering as
// a constructor call with no type behind it while `Int32(x)` was a
// conversion.
func TestLowercaseSpellingsConstruct(t *testing.T) {
	const src = `
func use() -> int32 {
    let a: int64 = 3
    return int32(a) + int32(int8(4)) + int32(uint16(6)) + int32(int(9))
}

func widen(_ n: int32) -> double { return double(n) }
func narrowFloat(_ n: int32) -> float { return float(n) }
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
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

// TestReceiverMethods is the desugaring: a method written outside its
// type's body is a member of that type, and reaches its fields both
// through the receiver's name and through implicit self.
func TestReceiverMethods(t *testing.T) {
	const src = `
struct vec2 { var x: int32; var y: int32 }
class Box { var v: int32 = 5 }
enum Dir { case up, down }

func (v: borrowing vec2) sum() -> int32 { return v.x + v.y }
func (v: borrowing vec2) implicit() -> int32 { return x + y }
func (b: borrowing Box) doubled() -> int32 { return b.v * 2 }
func (b: borrowing Box) bump() -> int32 { b.v = b.v + 1; return b.v }
func (d: borrowing Dir) code() -> int32 { return d == Dir.up ? 1 : 2 }
func (v: consuming vec2) taken() -> int32 { return v.x }

func use() -> int32 {
    let p = vec2(x: 3, y: 4)
    let bx = Box()
    return p.sum() + p.implicit() + bx.doubled() + bx.bump() + Dir.up.code() + p.taken()
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestInoutReceiverOnClassRefused is the one case the ownership word
// cannot mean what it means elsewhere. A class receiver is a
// reference, Swift has no mutating method on a class, and inout there
// would have to rebind the reference -- so it is refused by name
// rather than read as borrowing.
func TestInoutReceiverOnClassRefused(t *testing.T) {
	const src = `
class Box { var v: int32 = 0 }
func (b: inout Box) bad() { b.v = 1 }
`
	_, diags := compile(t, src, vsc.Options{Stop: vsc.Checked})
	if !vsc.Errors(diags) {
		t.Fatal("an inout receiver on a class was accepted")
	}
	if !strings.Contains(diags[0].Message, "'inout' receiver is not allowed on a class") {
		t.Errorf("diagnostic does not say why: %s", diags[0])
	}
}

// TestReceiverIsNotATopLevelName says the desugaring went the whole
// way: a receiver method is a member of its type, so its name is not
// callable on its own.
func TestReceiverIsNotATopLevelName(t *testing.T) {
	const src = `
struct S { var x: int32 }
func (s: borrowing S) get() -> int32 { return s.x }

func use() -> int32 { return get() }
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); !vsc.Errors(diags) {
		t.Error("a receiver method was callable as a free function")
	}
}
