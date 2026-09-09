package analyzer

import (
	"strings"
	"testing"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// Names said through the module they came from.
//
// A .swiftinterface writes every name that way -- `Swift.Int32`, and
// a module's own types through the module -- so reading one is what
// this is for. Both spellings have to mean one thing: `Swift.Int32`
// is Int32, not a second type that resembles it.
//
// What this replaced was worse than an error. `Swift.Int32` was
// resolved by taking `Swift` as a type, reporting that there is no
// such type, and then building a name out of what came back: every
// one of them became a type called `invalid.Int32`, which matched
// nothing and was carried into every signature that mentioned it. On
// the stdlib's own interface that was 15,411 diagnostics about a
// module and a few hundred types that quietly were not types.

// TestQualifiedNamesAreTheNamesTheyQualify: the two spellings are the
// same type, not two types that print alike.
func TestQualifiedNamesAreTheNamesTheyQualify(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
	}{
		{"a builtin", "let a: Int32 = 1\nlet b: Swift.Int32 = a\n"},
		{"a builtin the other way round", "let a: Swift.Int32 = 1\nlet b: Int32 = a\n"},
		{"an array", "let a: [Int32] = []\nlet b: Swift.Array<Int32> = a\n"},
		{"an optional", "let a: Int32? = nil\nlet b: Swift.Optional<Int32> = a\n"},
		{"a dictionary", "let a: [Int32: Bool] = [:]\nlet b: Swift.Dictionary<Int32, Bool> = a\n"},
		{"a conversion", "let a: Int = 3\nlet b: Int32 = Swift.Int32(a)\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if msgs := checkSnippet(t, c.src); len(msgs) != 0 {
				t.Errorf("%v", msgs)
			}
		})
	}
}

// TestAQualifiedNameThatIsNotThereIsReportedOnce: one diagnostic, and
// it says what was written.
//
// Reporting the qualifier separately reports a mistake nobody made --
// there is no type called Swift, and nobody said there was.
func TestAQualifiedNameThatIsNotThereIsReportedOnce(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"no such type in the module", "let x: Swift.Nonesuch = 1\n",
			"cannot find type 'Swift.Nonesuch' in scope: no such type in Swift"},
		{"no such name in the module", "let x = Swift.nonesuch\n",
			"cannot find 'Swift.nonesuch' in scope: no such name in Swift"},
		{"a member of a type", "struct S { var n: Int32 }\nlet x: S.Inner = 1\n",
			"cannot find type 'S.Inner' in scope: a type declared inside another type is not modelled yet"},
	} {
		t.Run(c.name, func(t *testing.T) {
			msgs := checkSnippet(t, c.src)
			if len(msgs) != 1 {
				t.Fatalf("want one diagnostic, got %v", msgs)
			}
			if msgs[0] != c.want {
				t.Errorf("said  %s\nwant  %s", msgs[0], c.want)
			}
		})
	}
}

// TestAValueWinsOverAModuleName: nothing declares a module, so its
// name means a module only where nothing else has taken it.
//
// This is what keeps the feature from changing the meaning of a
// program that has a variable called Swift in it. swiftc resolves the
// same way round.
func TestAValueWinsOverAModuleName(t *testing.T) {
	src := `
struct Holder { var Nonesuch: Int32 }
func use() -> Int32 {
    let Swift = Holder(Nonesuch: 7)
    return Swift.Nonesuch
}
`
	if msgs := checkSnippet(t, src); len(msgs) != 0 {
		t.Errorf("the local shadowing the module name was not used: %v", msgs)
	}
}

// TestAnImportedNameIsReachableThroughItsModule: a module's own names
// are reachable through it, and only through it.
//
// The interop corpus runs the same thing for real -- a library built
// by swiftc, called both ways. This is the part of it that does not
// need a linker.
func TestAnImportedNameIsReachableThroughItsModule(t *testing.T) {
	iface := `
public func mean(_ a: Int32, _ b: Int32) -> Int32
public struct Sample { public var n: Int32 }
`
	program := `
import Metrics
func use() -> Int32 {
    let bare = mean(4, 8)
    let through = Metrics.mean(4, 8)
    let s: Metrics.Sample = Sample(n: 1)
    return bare + through + s.n
}
`
	info, diags := checkImporting(t, program, "Metrics", iface)
	for _, d := range diags {
		t.Errorf("%s", d.Message)
	}
	// The qualified call reached a symbol, and it is the module's:
	// resolving it to nothing would have left the call with no
	// diagnostic and no callee.
	var found bool
	for name, sym := range info.Uses {
		if name != nil && sym != nil && sym.Name() == "mean" {
			if info.Imported[sym] != "Metrics" {
				t.Errorf("mean came from %q, want Metrics", info.Imported[sym])
			}
			found = true
		}
	}
	if !found {
		t.Error("no use of 'mean' was recorded, so nothing resolved it")
	}
}

// TestAnInterfaceMayNameItsOwnTypesThroughItself: swiftc writes
// `public func pointSum(_ p: Lib.Point)` in Lib's own interface, so
// reading one means answering that.
//
// It was not answered: a module got its scope only once its whole
// interface had been read, so while it was being read its own name
// meant nothing. Every self-qualified type in it resolved to Invalid,
// silently -- an interface's diagnostics are dropped by design -- and
// the first word of it was a mangling failure at the call site.
func TestAnInterfaceMayNameItsOwnTypesThroughItself(t *testing.T) {
	iface := `
public struct Point { public var x: Int32 }
public func pointSum(_ p: Lib.Point) -> Int32
public func makePoint() -> Lib.Point
`
	program := `
import Lib
func use(_ p: Point) -> Int32 { return pointSum(p) }
`
	info, diags := checkImporting(t, program, "Lib", iface)
	for _, d := range diags {
		t.Errorf("%s", d.Message)
	}
	// And the parameter is Lib's Point, not a placeholder that
	// mangles to nothing.
	for sym, module := range info.Imported {
		fs, ok := sym.(*FuncSymbol)
		if !ok || fs.Name() != "pointSum" || module != "Lib" {
			continue
		}
		sig := fs.Signature()
		if len(sig.Params) != 1 {
			t.Fatalf("pointSum has %d parameters", len(sig.Params))
		}
		if _, ok := sig.Params[0].Type.(*types.Struct); !ok {
			t.Errorf("the parameter is %s, want Lib's Point", sig.Params[0].Type)
		}
		return
	}
	t.Error("no imported 'pointSum' was recorded")
}

// TestAModuleIsNotAValue: what a module's name may be followed by is
// a name it exports, and nothing else.
func TestAModuleIsNotAValue(t *testing.T) {
	_, diags := checkImporting(t, "import Metrics\nlet x = Metrics.absent\n",
		"Metrics", "public func mean(_ a: Int32) -> Int32\n")
	if len(diags) != 1 {
		t.Fatalf("want one diagnostic, got %v", diags)
	}
	if want := "cannot find 'Metrics.absent' in scope"; !strings.Contains(diags[0].Message, want) {
		t.Errorf("said %q, want it to mention %q", diags[0].Message, want)
	}
}

// checkImporting checks a program against one hand-written interface.
func checkImporting(t *testing.T, program, module, iface string) (*Info, []token.Diagnostic) {
	t.Helper()
	pf, pu := parseSnippet(t, program)
	iu := token.NewFile(module+".vertexinterface", []byte(iface))
	ifile, diags := parser.ParseFile(iu, 0)
	for _, d := range diags {
		t.Fatalf("the interface did not parse: %s", d.Print(iu))
	}
	_ = pu
	return CheckImporting([]*ast.File{pf}, []Import{{
		Name: module, Files: []*ast.File{ifile}, Units: []*token.File{iu},
	}})
}
