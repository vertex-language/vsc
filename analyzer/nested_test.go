package analyzer

import (
	"strings"
	"testing"
)

// A type declared inside another type.
//
// Nesting decides the name and nothing else: `Chart.Point` is a
// struct like any other, reached through the outer name rather than
// on its own. It is how Swift's own libraries are shaped --
// String.Index, Data.Deallocator, Font.Weight -- so an interface that
// names one and a compiler that cannot read it is a compiler that
// cannot read the library. It was the largest single kind of failure
// across the SDK before this: 53,160 of them.

func TestANestedTypeIsReachedThroughTheOuterName(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"in a parameter", `
struct Chart { struct Point { var x: Int32 } }
func plot(_ p: Chart.Point) -> Int32 { return p.x }
`},
		{"in an annotation and a constructor", `
struct Chart { struct Point { var x: Int32 } }
func use() -> Int32 {
    let p: Chart.Point = Chart.Point(x: 1)
    return p.x
}
`},
		{"an enum inside a struct", `
struct Chart { enum Axis { case horizontal, vertical } }
func along(_ a: Chart.Axis) -> Int32 {
    switch a { case .horizontal: return 1; case .vertical: return 2 }
}
`},
		{"two levels down", `
struct Outer { struct Middle { struct Inner { var v: Int32 } } }
func deep(_ i: Outer.Middle.Inner) -> Int32 { return i.v }
`},
		{"the inner name alone, from inside the outer type", `
struct Chart {
    var scale: Int32
    struct Point { var x: Int32 }
    func origin() -> Point { return Point(x: scale) }
}
`},
		{"a nested type in another module's spelling", `
struct Chart { struct Point { var x: Int32 } }
func plot(_ p: Chart.Point) -> Int32 { return p.x }
`},
	} {
		t.Run(c.name, func(t *testing.T) {
			if msgs := checkSnippet(t, c.src); len(msgs) != 0 {
				t.Errorf("%v", msgs)
			}
		})
	}
}

// TestANestedNameThatIsNotThereIsReported: the refusal that stands
// where a name is neither a member nor a type.
func TestANestedNameThatIsNotThereIsReported(t *testing.T) {
	msgs := checkSnippet(t, `
struct Chart { struct Point { var x: Int32 } }
func plot(_ p: Chart.Nonesuch) -> Int32 { return 1 }
`)
	if len(msgs) != 1 {
		t.Fatalf("want one diagnostic, got %v", msgs)
	}
	if want := "'Chart.Nonesuch'"; !strings.Contains(msgs[0], want) {
		t.Errorf("said %q, want it to name %s", msgs[0], want)
	}
}

// TestAnInstanceHasNoNestedType: `Chart.Point` names a type;
// `chart.Point` does not name anything, and swiftc says so too.
func TestAnInstanceHasNoNestedType(t *testing.T) {
	msgs := checkSnippet(t, `
struct Chart { var scale: Int32; struct Point { var x: Int32 } }
func use(_ c: Chart) -> Int32 {
    let p = c.Point
    return 1
}
`)
	if len(msgs) == 0 {
		t.Error("a nested type was reached through an instance")
	}
}
