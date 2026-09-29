package doc

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
)

func read(t *testing.T, opts Options, srcs ...string) *Package {
	t.Helper()
	var files []*ast.File
	var units []*token.File
	for i, src := range srcs {
		unit := token.NewFile("f"+string(rune('0'+i))+".vs", []byte(src))
		f, diags := parser.ParseFile(unit, parser.ParseComments|parser.SkipBodies)
		if len(diags) > 0 {
			t.Fatalf("parse: %v", diags)
		}
		files = append(files, f)
		units = append(units, unit)
	}
	return New(files, units, opts)
}

const lib = `// Package geo is shapes.
package geo

/// A point.
public struct Point {
    public var X: float64
    var hidden: int

    public init(x: float64) { X = x }

    /// Its length.
    public var Length: float64 {
        get { return X }
    }
}

/// Moves it.
public func (p: inout Point) Move(by d: float64) { p.X += d }

public func Origin() -> Point { return Point(x: 0) }
public func Origin(_ at: float64) -> Point { return Point(x: at) }

public protocol Shape {
    /// How big.
    func Area() -> float64
    var Name: string { get }
}

public enum Kind: int32 {
    case round = 1, square
}

extension Point: Equatable {
    public static func == (a: Point, b: Point) -> bool { return a.X == b.X }
}

public extension Array {
    func Total() -> int { return count }
}

public let Pi = 3.14159
`

func TestNew(t *testing.T) {
	p := read(t, Options{ImportPath: "geo"}, lib)
	if p.Name != "geo" || p.Doc != "Package geo is shapes." {
		t.Errorf("package = %q, doc %q", p.Name, p.Doc)
	}
	types := map[string]*Type{}
	for _, ty := range p.Types {
		types[ty.Name] = ty
	}
	pt := types["Point"]
	if pt == nil || pt.Doc != "A point." {
		t.Fatalf("Point: %+v", pt)
	}
	var names []string
	for _, d := range pt.all() {
		names = append(names, d.Name+"="+d.Doc)
	}
	got := strings.Join(names, " ")
	for _, want := range []string{"init=", "X=", "Length=Its length.", "Move=Moves it.", "==="} {
		if !strings.Contains(got, want) {
			t.Errorf("Point's members %q lack %q", got, want)
		}
	}
	if strings.Contains(got, "hidden") {
		t.Errorf("an internal member was documented: %s", got)
	}
	if len(pt.Conforms) != 1 || pt.Conforms[0] != "Equatable" {
		t.Errorf("conformances = %q", pt.Conforms)
	}
	for _, d := range pt.Members {
		if d.Name == "Length" && d.Sig != "public var Length: float64 { get }" {
			t.Errorf("computed property sig = %q", d.Sig)
		}
		if d.Name == "Move" && d.Sig != "public func (p: inout Point) Move(by d: float64)" {
			t.Errorf("receiver sig = %q", d.Sig)
		}
	}

	if sh := types["Shape"]; sh == nil || len(sh.Members) != 2 || sh.Members[0].Doc != "How big." {
		t.Errorf("a public protocol's requirements are public: %+v", sh)
	}
	if k := types["Kind"]; k == nil || len(k.Cases) != 2 || k.Cases[1].Sig != "case square" {
		t.Errorf("enum cases: %+v", k)
	}
	if len(p.Extensions) != 1 || p.Extensions[0].Name != "Array" || len(p.Extensions[0].Members) != 1 {
		t.Errorf("a public extension's members are public: %+v", p.Extensions)
	}
	if len(p.Consts) != 1 || p.Consts[0].Sig != "public let Pi = 3.14159" {
		t.Errorf("consts: %+v", p.Consts)
	}
	if len(p.Funcs) != 2 || p.Funcs[0].ID == p.Funcs[1].ID {
		t.Errorf("overloads need their own IDs: %+v", p.Funcs)
	}
}

func TestAll(t *testing.T) {
	p := read(t, Options{All: true}, lib)
	for _, ty := range p.Types {
		if ty.Name == "Point" {
			for _, d := range ty.Members {
				if d.Name == "hidden" {
					return
				}
			}
		}
	}
	t.Error("All left out an internal member")
}

func TestRender(t *testing.T) {
	p := read(t, Options{ImportPath: "geo"}, lib)
	var md, page bytes.Buffer
	if err := Markdown(&md, p); err != nil {
		t.Fatal(err)
	}
	if err := HTML(&page, p); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# package geo", "import \"geo\"", "```vertex\npublic struct Point\n```", "- [`func Origin() -> Point`](#func-Origin)"} {
		if !strings.Contains(md.String(), want) {
			t.Errorf("markdown lacks %q:\n%s", want, md.String())
		}
	}
	for _, want := range []string{`id="struct-Point"`, `<a class="t" href="#struct-Point">Point</a>`, "Package geo is shapes."} {
		if !strings.Contains(page.String(), want) {
			t.Errorf("html lacks %q", want)
		}
	}
}

func TestBlocks(t *testing.T) {
	bs := blocks("First line\nsecond.\n\n    let x = 1\n\n    print(x)\n\n- one\n- two\n# Notes")
	kinds := []blockKind{paragraph, code, list, heading}
	if len(bs) != len(kinds) {
		t.Fatalf("blocks = %+v", bs)
	}
	for i, k := range kinds {
		if bs[i].kind != k {
			t.Errorf("block %d = %v, want %v", i, bs[i].kind, k)
		}
	}
	if got := strings.Join(bs[1].lines, "|"); got != "let x = 1||print(x)" {
		t.Errorf("code = %q", got)
	}
	if s := Synopsis("Sleep stops the thread. It returns at once."); s != "Sleep stops the thread." {
		t.Errorf("synopsis = %q", s)
	}
}
