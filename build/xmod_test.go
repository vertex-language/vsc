package build_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/lower"
	"github.com/vertex-language/vsc/vil/gen"
	"github.com/vertex-language/vsc/vil/pass"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/iface"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
)

// TestCrossModule compiles one module against another's interface,
// links the two objects, and runs the result.
//
// The interface is Swift's answer and so is the reason for it: a
// .swiftinterface is valid Swift with the bodies taken out, so a
// compiler that reads the language already reads a module's API and
// needs no binary module format to compile against one. Here the
// library is built, its interface printed, that text parsed back with
// the ordinary parser, and the client checked against what it
// declares.
//
// The layout crosses with it. swiftc without -enable-library-evolution
// compiles a client against another module's struct exactly as if it
// were local -- a field read is a struct_extract at a fixed offset --
// and the interface lists the stored properties in order so the
// client can do the same. That makes reordering a public struct's
// properties a breaking change, which is what Swift says it is
// outside of library evolution.
//
// swiftc on the same two modules also gives 42.
func TestCrossModule(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend")
	}
	const libSrc = `
public struct Point {
    public var x: Int32
    public var y: Int32
    public func sum() -> Int32 { return x + y }
}

public func triple(_ n: Int32) -> Int32 { return n * 3 }

func hidden() -> Int32 { return 99 }
`
	// 1. Build the library.
	lib, diags := vsc.Compile([]vsc.Source{{Name: "lib.swift", Text: []byte(libSrc)}},
		vsc.Options{Module: "Lib", Target: target})
	for _, d := range diags {
		t.Fatalf("lib: %v", d)
	}

	// 2. Emit its interface.
	var ifb bytes.Buffer
	if err := iface.Print(&ifb, iface.Module{
		Name: "Lib", Files: lib.Files, Units: lib.Positions, Info: lib.Info,
	}); err != nil {
		t.Fatal(err)
	}

	// 3. Parse the interface as the client will.
	ifFile := token.NewFile("Lib.vertexinterface", ifb.Bytes())
	ifAST, ds := parser.ParseFile(ifFile, 0)
	for _, d := range ds {
		t.Fatalf("interface does not parse: %s", d.Print(ifFile))
	}

	// 4. Check the client against it.
	const appSrc = `
func main() -> Int32 {
    let p = Point(x: 20, y: 8)
    return p.sum() + triple(4) + p.x - 18
}
`
	appFile := token.NewFile("app.swift", []byte(appSrc))
	appAST, ds := parser.ParseFile(appFile, 0)
	for _, d := range ds {
		t.Fatalf("app parse: %s", d.Print(appFile))
	}
	info, checks := analyzer.CheckImporting([]*ast.File{appAST}, []analyzer.Import{{
		Name: "Lib", Files: []*ast.File{ifAST}, Units: []*token.File{ifFile},
	}})
	for _, d := range checks {
		t.Fatalf("app check: %s", d.Print(appFile))
	}

	// 5. Lower the client and link it against the library.
	m, gd := gen.Files("main", []*ast.File{appAST}, info)
	for _, d := range gd {
		t.Fatalf("app lower: %s", d.Print(appFile))
	}
	if err := pass.Mandatory(m); err != nil {
		t.Fatalf("passes: %v", err)
	}
	if err := pass.LowerOwnership(m); err != nil {
		t.Fatalf("ownership: %v", err)
	}
	appVIR, err := lower.Module(m, target, lower.Options{SymbolPrefix: vsc.SymbolPrefix(target)})
	if err != nil {
		t.Fatalf("app lower: %v", err)
	}

	dir := t.TempDir()
	write := func(name string, mod *ir.Module) string {
		obj, err := build.Object(mod, build.Options{})
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, obj, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	appObj := write("app.o", appVIR)
	libObj := write("lib.o", lib.VIR)
	rt, err := build.Runtime(target)
	if err != nil {
		t.Fatal(err)
	}
	rtObj := filepath.Join(dir, "rt.o")
	if err := os.WriteFile(rtObj, rt.Data, 0o644); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(dir, "prog")
	if out, err := exec.Command("clang", "-o", bin, appObj, libObj, rtObj).CombinedOutput(); err != nil {
		t.Fatalf("link: %v\n%s", err, out)
	}
	cmd := exec.Command(bin)
	_ = cmd.Run()
	if got := cmd.ProcessState.ExitCode(); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestImportFromSearchPath is the same thing a build does: the
// library's interface is a file, `import` names the module, and the
// search path is where it is looked for.
//
// The failure modes are half the point. A module that is not there is
// an error against the line that imported it -- nothing used to be,
// so `import Anything` was accepted and the program failed later on
// every name it expected to find. And a name the library did not
// export is not in scope, which is the message swiftc gives for the
// same two modules.
func TestImportFromSearchPath(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend")
	}
	const libSrc = `
public struct Point {
    public var x: Int32
    public var y: Int32
    public func sum() -> Int32 { return x + y }
}

public func origin() -> Point { return Point(x: 3, y: 4) }
public func scale(_ p: Point, by k: Int32) -> Point {
    return Point(x: p.x * k, y: p.y * k)
}

func internalHelper() -> Int32 { return 99 }
`
	dir := t.TempDir()
	lib, diags := vsc.Compile([]vsc.Source{{Name: "lib.swift", Text: []byte(libSrc)}},
		vsc.Options{Module: "Geometry", Target: target})
	for _, d := range diags {
		t.Fatalf("lib: %v", d)
	}
	var ifb bytes.Buffer
	if err := iface.Print(&ifb, iface.Module{
		Name: "Geometry", Files: lib.Files, Units: lib.Positions, Info: lib.Info,
	}); err != nil {
		t.Fatal(err)
	}
	ifPath := filepath.Join(dir, "Geometry"+iface.Extension)
	if err := os.WriteFile(ifPath, ifb.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("a program that imports it", func(t *testing.T) {
		app, diags := vsc.Compile([]vsc.Source{{Name: "app.swift", Text: []byte(`
import Geometry

func main() -> Int32 {
    let p = origin()
    return scale(p, by: 3).sum() + 21
}
`)}}, vsc.Options{Module: "main", Target: target, ImportPaths: []string{dir}})
		for _, d := range diags {
			t.Fatalf("app: %v", d)
		}

		objDir := t.TempDir()
		write := func(name string, mod *ir.Module) string {
			obj, err := build.Object(mod, build.Options{})
			if err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(objDir, name)
			if err := os.WriteFile(p, obj, 0o644); err != nil {
				t.Fatal(err)
			}
			return p
		}
		rt, err := build.Runtime(target)
		if err != nil {
			t.Fatal(err)
		}
		rtPath := filepath.Join(objDir, "rt.o")
		if err := os.WriteFile(rtPath, rt.Data, 0o644); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(objDir, "prog")
		if out, err := exec.Command("clang", "-o", bin,
			write("app.o", app.VIR), write("lib.o", lib.VIR), rtPath).CombinedOutput(); err != nil {
			t.Fatalf("link: %v\n%s", err, out)
		}
		cmd := exec.Command(bin)
		_ = cmd.Run()
		if got := cmd.ProcessState.ExitCode(); got != 42 {
			t.Errorf("exit status = %d, want 42", got)
		}
	})

	t.Run("a module that is not there", func(t *testing.T) {
		_, diags := vsc.Compile([]vsc.Source{{Name: "app.swift", Text: []byte(`
import Nowhere
func main() -> Int32 { return 0 }
`)}}, vsc.Options{Module: "main", Target: target, ImportPaths: []string{dir}})
		if len(diags) == 0 {
			t.Fatal("importing a module that does not exist was accepted")
		}
		if !strings.Contains(diags[0].Message, "no such module 'Nowhere'") {
			t.Errorf("reported %q, want it to name the module", diags[0].Message)
		}
	})

	t.Run("a name the library did not export", func(t *testing.T) {
		_, diags := vsc.Compile([]vsc.Source{{Name: "app.swift", Text: []byte(`
import Geometry
func main() -> Int32 { return internalHelper() }
`)}}, vsc.Options{Module: "main", Target: target, ImportPaths: []string{dir}})
		if len(diags) == 0 {
			t.Fatal("an internal name was visible across the module boundary")
		}
		if !strings.Contains(diags[0].Message, "cannot find 'internalHelper' in scope") {
			t.Errorf("reported %q, want swiftc's message", diags[0].Message)
		}
	})
}
