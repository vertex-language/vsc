package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vertex-language/ir"
	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/token"
)

// Where a class's self is passed.
//
// swiftc's own code for a class method reads self from x20, the self
// register: `Counter.plus(_ k: Int32)` takes k in w0 and self in x20.
// A value type's method takes self as its last ordinary argument --
// `S.plus(_ k: Int32)` takes k in w0 and self.a in w1 -- and that is
// what this compiler emits for every method it calls.
//
// So a call to another module's class method used to put self where
// that method does not look, and read whatever was in x20 instead. It
// compiled, it linked, it ran, and it answered 113 where swiftc
// answered 42. ir.SwiftSelf is what closed it: the parameter is
// marked in the signature, and the backend places it in x20 at both
// ends.

const boxes = `// swift-interface-format-version: 1.0
// swift-module-flags: -target arm64-apple-macosx26.0 -module-name Boxes
public final class Counter {
  final public var n: Swift.Int32
  public init(n: Swift.Int32)
  final public func plus(_ k: Swift.Int32) -> Swift.Int32
}
public func makeCounter(_ n: Swift.Int32) -> Boxes.Counter
`

const boxesSwift = `public final class Counter {
    public var n: Int32
    public init(n: Int32) { self.n = n }
    public func plus(_ k: Int32) -> Int32 { return n + k }
}
public func makeCounter(_ n: Int32) -> Counter { return Counter(n: n) }
`

// withBoxes writes the library and its interface into a directory.
func withBoxes(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Boxes.swiftinterface"), []byte(boxes), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Boxes.swift"), []byte(boxesSwift), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestAMethodOnAnImportedClassRuns: the miscompile, and the ABI that
// closed it.
//
// Before the self register was passed, this program compiled, linked,
// ran, and returned 113. The number is the point: not a crash, not a
// link error, an answer.
func TestAMethodOnAnImportedClassRuns(t *testing.T) {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon")
	}
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH")
	}
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	dir := withBoxes(t)
	lib := buildBoxes(t, swiftc, dir)

	src := []byte("import Boxes\nfunc main() -> Int32 { return makeCounter(41).plus(1) }\n")
	if got := runAgainst(t, swiftc, target, dir, lib, src); got != 42 {
		t.Errorf("the program answered %d, want 42", got)
	}
}

// TestAPropertyOfAnImportedClassIsRead: an instance is a header and
// then the stored properties, so a field is an offset like a struct's
// -- swiftc reads `Counter.n` at 16, past the isa and the refcount,
// and so does this. Nothing about that goes through the self
// register, which is why it was right while the method call was not.
func TestAPropertyOfAnImportedClassIsRead(t *testing.T) {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon")
	}
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH")
	}
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	dir := withBoxes(t)
	lib := buildBoxes(t, swiftc, dir)

	src := []byte("import Boxes\nfunc main() -> Int32 { return makeCounter(41).n + 1 }\n")
	if got := runAgainst(t, swiftc, target, dir, lib, src); got != 42 {
		t.Errorf("the program answered %d, want 42", got)
	}
}

// buildBoxes builds the library with swiftc.
func buildBoxes(t *testing.T, swiftc, dir string) string {
	t.Helper()
	lib := filepath.Join(dir, "Boxes.o")
	cmd := exec.Command(swiftc, "-parse-as-library", "-c", "-module-name", "Boxes",
		"-o", lib, filepath.Join(dir, "Boxes.swift"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("swiftc: %v\n%s", err, out)
	}
	return lib
}

// runAgainst compiles a program with this compiler, links it against a
// library swiftc built, and runs it.
func runAgainst(t *testing.T, swiftc string, target ir.Target, dir, lib string, src []byte) int {
	t.Helper()
	unit, diags := vsc.Compile([]vsc.Source{{Name: "p.swift", Text: src}},
		vsc.Options{Module: "main", Target: target, ImportPaths: []string{dir}})
	for _, d := range diags {
		t.Fatalf("refused: %s", d.String())
	}
	obj, err := build.Object(unit.VIR, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	objPath := filepath.Join(dir, "p.o")
	if err := os.WriteFile(objPath, obj, 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := build.Runtime(target)
	if err != nil {
		t.Fatal(err)
	}
	rtPath := filepath.Join(dir, "rt.o")
	if err := os.WriteFile(rtPath, rt.Data, 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "p")
	if out, err := exec.Command(swiftc, "-o", bin, objPath, rtPath, lib).CombinedOutput(); err != nil {
		t.Fatalf("link: %s", out)
	}
	run := exec.Command(bin)
	_ = run.Run()
	return run.ProcessState.ExitCode()
}

// TestAnExistentialCrossesAModuleBoundary.
//
// Both ways now. Taking one back from a Swift library was the easy
// direction -- what comes back was built by swiftc, and reading it is
// agreeing with a layout. Filling one in and handing it over is the
// other, and it needed the type to be described at run time: the
// buffer and the table were always right, and the metadata word held
// a placeholder that said only "trivial, inline".
//
// It says the type now. See tests/interop/020-existentials-out for
// the program that runs; this is the same program compiled, which is
// what catches a refusal coming back.
func TestAnExistentialCrossesAModuleBoundary(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	dir := t.TempDir()
	iface := `// swift-interface-format-version: 1.0
// swift-module-flags: -module-name Shapes
public protocol Measured {
  func measure() -> Swift.Int32
}
public func measureIt(_ m: any Shapes.Measured) -> Swift.Int32
`
	if err := os.WriteFile(filepath.Join(dir, "Shapes.swiftinterface"), []byte(iface), 0o644); err != nil {
		t.Fatal(err)
	}
	src := []byte(`import Shapes
struct Mine: Measured { func measure() -> Int32 { return 42 } }
func main() -> Int32 { return measureIt(Mine()) }
`)
	_, diags := vsc.Compile([]vsc.Source{{Name: "p.swift", Text: src}},
		vsc.Options{Module: "main", Target: target, ImportPaths: []string{dir}})
	if vsc.Errors(diags) {
		var b strings.Builder
		for _, d := range diags {
			if d.Severity == token.Error {
				b.WriteString("\n  ")
				b.WriteString(d.Message)
			}
		}
		t.Fatalf("an existential was refused at the boundary:%s", b.String())
	}
}
