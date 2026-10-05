package build_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil/gen"
	"github.com/vertex-language/vsc/internal/sil/pass"
	"github.com/vertex-language/vsc/lower"

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
    // A memberwise initializer is internal, so a client needs this one.
    public init(x: Int32, y: Int32) { self.x = x; self.y = y }
    public func sum() -> Int32 { return x + y }
    // A subscript crosses as its accessors, which the interface lists.
    public subscript(scale: Int32) -> Int32 {
        get { return (x + y) * scale }
        set { x = newValue; y = 0 }
    }
    public static subscript(n: Int32) -> Int32 { return n + 1 }
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
	ifFile := token.NewFile("Lib.vinterface", ifb.Bytes())
	ifAST, ds := parser.ParseFile(ifFile, 0)
	for _, d := range ds {
		t.Fatalf("interface does not parse: %s", d.Print(ifFile))
	}

	// 4. Check the client against it.
	const appSrc = `
func main() -> Int32 {
    var p = Point(x: 20, y: 8)
    let scaled = p[2] // 56
    p[0] = 12 // x = 12, y = 0
    // 12 + 12 + 12 - 18 + 56 - 56 + 2 - 2 + 24 + 0 = 42
    return p.sum() + triple(4) + p.x - 18 + scaled - 56 + Point[1] - 2 + p[2] + p.y
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

	// Linked by this project rather than by clang. The two objects
	// are what the test made and the runtime is what Executable adds,
	// so the link needs nothing installed and the test runs wherever
	// the compiler does.
	runProgram(t, target, []build.Input{
		{Name: "app.o", Data: object(t, appVIR)},
		{Name: "lib.o", Data: object(t, lib.VIR)},
	}, 42)
}

// object lowers one module and returns the object file's bytes.
func object(t *testing.T, m *ir.Module) []byte {
	t.Helper()
	obj, err := build.Object(m, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return obj
}

// runProgram links objects into an executable, runs it, and checks
// the exit status.
func runProgram(t *testing.T, target ir.Target, objs []build.Input, want int) {
	t.Helper()
	exe, err := build.Executable(objs, build.LinkOptions{Target: target})
	if err != nil {
		t.Fatal(err)
	}
	bin := vsc.ImageName(target, filepath.Join(t.TempDir(), "prog"))
	if err := os.WriteFile(bin, exe, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin)
	_ = cmd.Run()
	if got := cmd.ProcessState.ExitCode(); got != want {
		t.Errorf("exit status = %d, want %d", got, want)
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
    // A memberwise initializer is internal, so a client needs this one.
    public init(x: Int32, y: Int32) { self.x = x; self.y = y }
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

		runProgram(t, target, []build.Input{
			{Name: "app.o", Data: object(t, app.VIR)},
			{Name: "lib.o", Data: object(t, lib.VIR)},
		}, 42)
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

// TestCrossModuleAPI compiles a client against the whole of a library's
// public face: methods declared with a receiver, static methods, computed
// and static stored properties, default arguments, a signature naming a
// standard-library generic, and a cast to a public error type.
//
// Every one of these went through the interface as text, so each is two
// things at once: the printer wrote something the parser reads back, and
// what the client then emitted links against what the library emitted.
// They were all broken in different ways, and in the same direction --
// the interface said less than the module did, so a client either could
// not see a declaration or named a symbol nobody defined.
func TestCrossModuleAPI(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend")
	}
	const libSrc = `
public struct Options {
    public var Backlog: Int32 = 128
    public init() {}
    // A static stored property, reached through its addressor, and the
    // implicit member a default argument writes.
    public static let ` + "`default`" + ` = Options()
    // A static method, which the interface has to say is static.
    public static func Make(_ n: Int32) -> Options {
        var o = Options()
        o.Backlog = n
        return o
    }
    // A computed property: the client calls a getter this module has to
    // export, not lay out a field that is not there.
    public var Doubled: Int32 { return Backlog * 2 }
}

public enum LibError: Error {
    case tooSmall(Int32)
}

// A method declared with a receiver belongs to Options, not to the
// module: the interface used to say both.
public func (o: borrowing Options) Plus(_ n: Int32) -> Int32 {
    return o.Backlog + n
}

// A default argument is evaluated at the call, so the interface carries
// the expression and the client's own code evaluates it.
public func Sized(_ n: Int32, options: Options = .default) -> Int32 {
    return n + options.Backlog
}

// A signature naming a standard-library generic: both modules have to
// mangle ArraySlice as the standard library's, not as their own.
public func Total(_ xs: borrowing ArraySlice<Int32>) -> Int32 {
    return Int32(xs.count) * 20
}

public func Check(_ n: Int32) throws -> Int32 {
    if n < 10 {
        throw LibError.tooSmall(n)
    }
    return n
}
`
	dir := t.TempDir()
	lib, diags := vsc.Compile([]vsc.Source{{Name: "lib.swift", Text: []byte(libSrc)}},
		vsc.Options{Module: "Lib", Target: target})
	for _, d := range diags {
		t.Fatalf("lib: %v", d)
	}
	var ifb bytes.Buffer
	if err := iface.Print(&ifb, iface.Module{
		Name: "Lib", Files: lib.Files, Units: lib.Positions, Info: lib.Info,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Lib"+iface.Extension), ifb.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	// 131 (Sized: 3 and the default's 128) + 128 (Options()) + 256
	// (Doubled) + 5 (Make(2).Plus(3)) + 60 (Total over three elements)
	// + 11 (the error's payload and 10) = 591, less 549 to exit with 42.
	app, diags := vsc.Compile([]vsc.Source{{Name: "app.swift", Text: []byte(`
import Lib

func main() -> Int32 {
    var total: Int32 = 0
    total += Sized(3)
    let o = Options()
    total += o.Backlog
    total += Options.default.Doubled
    total += Options.Make(2).Plus(3)
    let xs: [Int32] = [10, 20, 30, 40]
    total += Total(xs[1..<4])
    do {
        _ = try Check(1)
    } catch let e as LibError {
        switch e {
        case .tooSmall(let n): total += n + 10
        }
    } catch {
        return 1
    }
    return total - 549
}
`)}}, vsc.Options{Module: "main", Target: target, ImportPaths: []string{dir}})
	for _, d := range diags {
		t.Fatalf("app: %v", d)
	}

	runProgram(t, target, []build.Input{
		{Name: "app.o", Data: object(t, app.VIR)},
		{Name: "lib.o", Data: object(t, lib.VIR)},
	}, 42)
}

// TestCrossModuleAsync is an `await` on a function another module
// defines, which is the case the frame size makes hard: whoever calls an
// async function allocates its frame, and how big that frame is is not
// in the function's type.
//
// Swift's answer is a record beside every async function -- `Tu`, a
// relative address and a size -- and the caller reads the size out of
// it. That record is what makes this link at all, and it is the same
// record an async function value names. See lower/asyncsplit.go.
//
// Until it existed this call was refused at lowering rather than
// miscompiled, so the regression this guards against is a compiler that
// goes back to guessing.
func TestCrossModuleAsync(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend")
	}
	// The library's own function suspends, so its frame is bigger than
	// the header and a caller that assumed the header would overlap it.
	const libSrc = `
public func step(_ n: Int32) async -> Int32 { return n + 1 }

public func twice(_ n: Int32) async -> Int32 {
    let a = await step(n)
    return await step(a)
}

// A method, whose declaration in the client is made by a different path
// from a function's, and was once made without saying it is async -- so
// the call linked as an ordinary one.
public struct Counter {
    public var base: Int32
    // A memberwise initializer is internal, so a client needs this one.
    public init(base: Int32) { self.base = base }
}

public func (c: borrowing Counter) Add(_ n: Int32) async -> Int32 {
    return await step(c.base + n) - 1
}
`
	lib, diags := vsc.Compile([]vsc.Source{{Name: "lib.swift", Text: []byte(libSrc)}},
		vsc.Options{Module: "Lib", Target: target})
	for _, d := range diags {
		t.Fatalf("lib: %v", d)
	}

	var ifb bytes.Buffer
	if err := iface.Print(&ifb, iface.Module{
		Name: "Lib", Files: lib.Files, Units: lib.Positions, Info: lib.Info,
	}); err != nil {
		t.Fatal(err)
	}
	ifFile := token.NewFile("Lib.vinterface", ifb.Bytes())
	ifAST, ds := parser.ParseFile(ifFile, 0)
	for _, d := range ds {
		t.Fatalf("interface does not parse: %s", d.Print(ifFile))
	}
	if !strings.Contains(ifb.String(), "async") {
		t.Fatalf("the interface does not say the functions are async:\n%s", ifb.String())
	}

	const appSrc = `
func main() async -> Int32 {
    let c = Counter(base: 30)
    let got = await c.Add(10)          // 40
    return await twice(got)            // 42
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

	runProgram(t, target, []build.Input{
		{Name: "app.o", Data: object(t, appVIR)},
		{Name: "lib.o", Data: object(t, lib.VIR)},
	}, 42)
}

// TestCrossModuleAccess holds a client to what a library made public. The
// interface lists a struct's internal stored property -- the client lays the
// struct out -- and says it is internal, and it lists no internal
// initializer; either named from the client is refused as swiftc refuses
// it (an internal initializer by its label, since the interface has none by
// that one), and so is the memberwise initializer, which is internal too.
func TestCrossModuleAccess(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend")
	}
	const libSrc = `
public struct Span {
    let nanos: Int64
    public let label: String
    init(nanos: Int64) { self.nanos = nanos; self.label = "" }
    public init(label: String) { self.nanos = 7; self.label = label }
    public func Nanos() -> Int64 { return nanos }
}

public struct Plain {
    public let n: Int
}
`
	lib, diags := vsc.Compile([]vsc.Source{{Name: "lib.swift", Text: []byte(libSrc)}},
		vsc.Options{Module: "Lib", Target: target})
	for _, d := range diags {
		t.Fatalf("lib: %v", d)
	}
	var ifb bytes.Buffer
	if err := iface.Print(&ifb, iface.Module{
		Name: "Lib", Files: lib.Files, Units: lib.Positions, Info: lib.Info,
	}); err != nil {
		t.Fatal(err)
	}
	text := ifb.String()
	for _, want := range []string{"internal let nanos: int64", "public let label: string", "public init(label: string)"} {
		if !strings.Contains(text, want) {
			t.Errorf("the interface has no %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "init(nanos") {
		t.Errorf("the interface lists the internal initializer:\n%s", text)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Lib"+iface.Extension), ifb.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	compile := func(body string) []vsc.Diagnostic {
		_, diags := vsc.Compile([]vsc.Source{{Name: "app.swift", Text: []byte("import Lib\n" + body)}},
			vsc.Options{Module: "main", Target: target, ImportPaths: []string{dir}})
		return diags
	}
	for _, c := range []struct{ body, want string }{
		{"func main() -> Int32 { let s = Span(label: \"x\"); return Int32(s.nanos) }",
			"'nanos' is inaccessible due to 'internal' protection level"},
		{"func main() -> Int32 { let s = Span(nanos: 3); return Int32(s.Nanos()) }",
			// Not in the interface at all, so what is wrong is the label.
			"incorrect argument label"},
		{"func main() -> Int32 { let p = Plain(n: 3); return Int32(p.n) }",
			"'Plain' initializer is inaccessible due to 'internal' protection level"},
	} {
		diags := compile(c.body)
		found := false
		for _, d := range diags {
			if strings.Contains(d.Message, c.want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no diagnostic %q, got %v", c.body, c.want, diags)
		}
	}

	// What is public is usable, and the struct with an internal property
	// is laid out right: Nanos reads the 7 the library stored.
	app, diags := vsc.Compile([]vsc.Source{{Name: "app.swift", Text: []byte(`
import Lib

func main() -> Int32 {
    let s = Span(label: "x")
    return Int32(s.Nanos()) + Int32(s.label.count) * 34
}
`)}}, vsc.Options{Module: "main", Target: target, ImportPaths: []string{dir}})
	for _, d := range diags {
		t.Fatalf("app: %v", d)
	}
	runProgram(t, target, []build.Input{
		{Name: "app.o", Data: object(t, app.VIR)},
		{Name: "lib.o", Data: object(t, lib.VIR)},
	}, 41)
}
