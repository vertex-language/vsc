package build_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

// Finding and refusing a module.
//
// A package does not ship what this compiler used to look for. swiftc
// writes `<Module>.swiftinterface` when asked for one by path, and a
// built module -- an SDK framework, a package's build directory -- is
// a `<Module>.swiftmodule` directory with one interface per target
// inside it. Both are found now, and the header of whichever is found
// is read before its declarations are.

const geometry = `// swift-interface-format-version: 1.0
// swift-module-flags: -target arm64-apple-macosx26.0 -enable-objc-interop -module-name Geometry
public func addAll(_ a: Swift.Int32, _ b: Swift.Int32) -> Swift.Int32
`

const resilient = `// swift-interface-format-version: 1.0
// swift-module-flags: -target arm64-apple-macosx26.0 -enable-library-evolution -module-name Geometry
public func addAll(_ a: Swift.Int32, _ b: Swift.Int32) -> Swift.Int32
`

func compileAgainst(t *testing.T, dir string) []vsc.Diagnostic {
	t.Helper()
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	src := []byte("import Geometry\nfunc main() -> Int32 { return addAll(40, 2) }\n")
	_, diags := vsc.Compile([]vsc.Source{{Name: "program.swift", Text: src}},
		vsc.Options{Module: "main", Target: target, Stop: vsc.Checked, ImportPaths: []string{dir}})
	return diags
}

// TestAnInterfaceIsFoundWhereOneIsShipped: the three layouts.
func TestAnInterfaceIsFoundWhereOneIsShipped(t *testing.T) {
	// A built module holds one interface per target, named for it,
	// and the compiler reads the one for the machine it is running
	// on. So the case is written for that machine rather than for a
	// particular one: the layout is what is being tested, and a file
	// called arm64e-apple-macos.swiftinterface tests it only where
	// arm64 is what the host answers.
	built := "Geometry.swiftmodule/" + hostArch() + "-apple-macos.swiftinterface"
	for _, c := range []struct{ name, path string }{
		{"written by this compiler", "Geometry.vertexinterface"},
		{"written by swiftc, by path", "Geometry.swiftinterface"},
		{"inside a built module", built},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			full := filepath.Join(dir, c.path)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(geometry), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, d := range compileAgainst(t, dir) {
				t.Errorf("%s", d.String())
			}
		})
	}
}

// hostArch is the architecture spelling a built module's interfaces
// are named with, for this machine: Swift's, which is Go's with
// amd64 spelled the way everyone else spells it.
func hostArch() string {
	if runtime.GOARCH == "amd64" {
		return "x86_64"
	}
	return runtime.GOARCH
}

// TestAModuleThatIsNotThereSaysWhereItLooked.
func TestAModuleThatIsNotThereSaysWhereItLooked(t *testing.T) {
	diags := compileAgainst(t, t.TempDir())
	if len(diags) != 1 {
		t.Fatalf("want one diagnostic, got %d", len(diags))
	}
	for _, want := range []string{"no such module", "Geometry.vertexinterface",
		"Geometry.swiftinterface", "Geometry.swiftmodule/"} {
		if !strings.Contains(diags[0].Message, want) {
			t.Errorf("said %q, want it to mention %q", diags[0].Message, want)
		}
	}
}

// TestALibraryBuiltForTheOtherABIIsRefused: the hazard this closes.
//
// Under -enable-library-evolution a struct's stored properties are
// not its layout and a field is a getter call. This compiler reads
// the same declarations and compiles a fixed-offset field read, so
// the program links and dies on the first access -- a bus error, with
// nothing in it to connect to the cause. The only thing that tells
// the two builds apart is the header line.
func TestALibraryBuiltForTheOtherABIIsRefused(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Geometry.swiftinterface"),
		[]byte(resilient), 0o644); err != nil {
		t.Fatal(err)
	}
	diags := compileAgainst(t, dir)
	if len(diags) == 0 {
		t.Fatal("a library built for the resilient ABI was accepted")
	}
	for _, want := range []string{"-enable-library-evolution", "different ABI", "@frozen"} {
		if !strings.Contains(diags[0].Message, want) {
			t.Errorf("said %q, want it to mention %q", diags[0].Message, want)
		}
	}
}
