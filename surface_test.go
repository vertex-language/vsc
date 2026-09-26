package vsc

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
)

func surfaceOf(t *testing.T, src string) [32]byte {
	t.Helper()
	tf := token.NewFile("p/a.vs", []byte(src))
	f, ds := parser.ParseFile(tf, 0)
	if len(ds) > 0 {
		t.Fatalf("parse %q: %v", src, ds)
	}
	return surface([]*ast.File{f}, []*token.File{tf})
}

// What a client can depend on of a package, and what it cannot: an edit
// of the second kind must leave the surface as it was, and one of the
// first must not.
func TestSurface(t *testing.T) {
	const base = `package p

struct Point { var x: int }

public func area(_ p: Point) -> int {
    return p.x * 2
}

extension Point {
    func twice() -> int { return x * 2 }
    var half: int { return x / 2 }
}

func (p: borrowing Point) thrice() -> int {
    return p.x * 3
}
`
	same := []struct{ name, src string }{
		{"a function body", `package p

struct Point { var x: int }

public func area(_ p: Point) -> int {
    let y = p.x
    return y * 3
}

extension Point {
    func twice() -> int { return x * 2 }
    var half: int { return x / 2 }
}

func (p: borrowing Point) thrice() -> int {
    return p.x * 3
}
`},
		{"a method, a computed property and a receiver of a non-generic type", `package p

struct Point { var x: int }

public func area(_ p: Point) -> int {
    return p.x * 2
}

extension Point {
    func twice() -> int { return x + x }
    var half: int { return x >> 1 }
}

func (p: borrowing Point) thrice() -> int {
    return p.x + p.x + p.x
}
`},
	}
	for _, c := range same {
		if surfaceOf(t, c.src) != surfaceOf(t, base) {
			t.Errorf("%s: the surface changed, but no client compiles it", c.name)
		}
	}

	differ := []struct{ name, src string }{
		{"a declaration", `package p

struct Point { var x: int; var y: int }

public func area(_ p: Point) -> int {
    return p.x * 2
}

extension Point {
    func twice() -> int { return x * 2 }
    var half: int { return x / 2 }
}

func (p: borrowing Point) thrice() -> int {
    return p.x * 3
}
`},
		{"a signature", `package p

struct Point { var x: int }

public func area(_ p: Point, scale: int = 2) -> int {
    return p.x * 2
}

extension Point {
    func twice() -> int { return x * 2 }
    var half: int { return x / 2 }
}

func (p: borrowing Point) thrice() -> int {
    return p.x * 3
}
`},
	}
	for _, c := range differ {
		if surfaceOf(t, c.src) == surfaceOf(t, base) {
			t.Errorf("%s: the surface is unchanged, but a client depends on it", c.name)
		}
	}
}

// The bodies a client compiles are part of the surface.
func TestSurfaceKeepsWhatClientsCompile(t *testing.T) {
	for _, c := range []struct{ name, a, b string }{
		{"a generic function",
			"func f<T>(_ x: T) -> T { return x }\n",
			"func f<T>(_ x: T) -> T { let y = x; return y }\n"},
		{"an @inlinable function",
			"@inlinable public func f() -> int { return 1 }\n",
			"@inlinable public func f() -> int { return 2 }\n"},
		{"a member of a generic type",
			"struct Box<T> { var v: T\n func get() -> T { return v } }\n",
			"struct Box<T> { var v: T\n func get() -> T { let w = v; return w } }\n"},
		{"a generic method of a non-generic type",
			"struct S { func f<T>(_ x: T) -> T { return x } }\n",
			"struct S { func f<T>(_ x: T) -> T { let y = x; return y } }\n"},
		{"an extension of a type declared elsewhere, which may be generic",
			"extension Array { func first2() -> int { return 1 } }\n",
			"extension Array { func first2() -> int { return 2 } }\n"},
		{"a receiver of a type declared elsewhere",
			"func (a: borrowing Other) f() -> int { return 1 }\n",
			"func (a: borrowing Other) f() -> int { return 2 }\n"},
		{"a stored property's initializer, which a client may infer the type from",
			"struct S { var x = 1 }\n",
			"struct S { var x = 1.5 }\n"},
	} {
		if surfaceOf(t, "package p\n"+c.a) == surfaceOf(t, "package p\n"+c.b) {
			t.Errorf("%s: the edit left the surface unchanged", c.name)
		}
	}
}

// A line added to a body moves every position after it. That matters only
// where a body a client compiles, or a #line, follows in the same file.
func TestSurfacePositions(t *testing.T) {
	const tail = "func g<T>(_ x: T) -> T { return x }\n"
	one := "package p\nfunc f() -> int {\n    return 1\n}\n"
	two := "package p\nfunc f() -> int {\n    let a = 1\n    return a\n}\n"
	if surfaceOf(t, one) != surfaceOf(t, two) {
		t.Error("a line added to the last body changed the surface, though nothing after it reads a position")
	}
	if surfaceOf(t, one+tail) == surfaceOf(t, two+tail) {
		t.Error("a line added above a generic function left the surface unchanged, though its lines moved")
	}
	if surfaceOf(t, one+"let here = #line\n") == surfaceOf(t, two+"let here = #line\n") {
		t.Error("a line added above a #line left the surface unchanged")
	}
	if surfaceOf(t, tail+one) != surfaceOf(t, tail+two) {
		t.Error("a line added below the generic function changed the surface")
	}
}

// The pruned text an importer reads keeps every position, parses, and has
// the source's surface: whichever a build read, its keys are the same.
func TestPrune(t *testing.T) {
	const src = `package p

struct Point { var x: int }

public func area(_ p: Point) -> int {
    let s = "a } b"   // a brace in a string
    return p.x * 2
}

func g<T>(_ x: T) -> T {
    precondition(true)
    return x
}

extension Point {
    func twice() -> int { return x * 2 }
    var half: int { return x / 2 }
}
`
	tf := token.NewFile("p/a.vs", []byte(src))
	f, ds := parser.ParseFile(tf, 0)
	if len(ds) > 0 {
		t.Fatal(ds)
	}
	pruned := prune([]*ast.File{f}, []*token.File{tf})[0]
	if len(pruned) != len(src) || bytes.Count(pruned, []byte("\n")) != strings.Count(src, "\n") {
		t.Fatalf("pruning moved positions:\n%s", pruned)
	}
	if bytes.Contains(pruned, []byte("p.x * 2")) || bytes.Contains(pruned, []byte("x * 2 }")) {
		t.Errorf("a body no client compiles is still there:\n%s", pruned)
	}
	if !bytes.Contains(pruned, []byte("precondition(true)")) {
		t.Errorf("the generic function's body was taken out:\n%s", pruned)
	}
	ptf := token.NewFile("p/a.vs", pruned)
	pf, ds := parser.ParseFile(ptf, 0)
	if len(ds) > 0 {
		t.Fatalf("the pruned text does not parse: %v\n%s", ds, pruned)
	}
	if surface([]*ast.File{pf}, []*token.File{ptf}) != surface([]*ast.File{f}, []*token.File{tf}) {
		t.Error("the pruned text's surface differs from the source's")
	}
}

// A #line in a body no client compiles reads nothing a client depends on.
func TestSurfaceLineInStrippedBody(t *testing.T) {
	one := "package p\nfunc f() -> int {\n    return 1\n}\nfunc h() -> int { return #line }\n"
	two := "package p\nfunc f() -> int {\n    let a = 1\n    return a\n}\nfunc h() -> int { return #line }\n"
	if surfaceOf(t, one) != surfaceOf(t, two) {
		t.Error("a #line in a stripped body made an earlier body's lines count")
	}
}
