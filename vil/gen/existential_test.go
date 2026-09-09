package gen

import (
	"strings"
	"testing"
)

// A value whose type is not known until it arrives.
//
// `any P` is a buffer with the value in it and, beside it, the table
// of that type's implementations of P. Everything below is about
// keeping the two together: a value put in without its table, or a
// table left behind when the value is replaced, gives a program that
// compiles, links, runs, and answers a question nobody asked.

// TestAnExistentialArgumentIsBoxed: the three instructions SILGen
// writes for putting a concrete value into an existential, and the
// two it writes for calling through one.
//
// swiftc -emit-silgen for the same source:
//
//	%3 = alloc_stack $any Shape
//	%4 = init_existential_addr %3 : $*any Shape, $Square
//	store %2 to [trivial] %4
//	%6 = witness_method $Square, #Shape.area
//	%7 = open_existential_addr %0 : $*any Shape
//	%8 = apply %6(%7)
func TestAnExistentialArgumentIsBoxed(t *testing.T) {
	got, diags := generate(t, "main", `
protocol Shape { func area() -> Int32 }
struct Square: Shape { var side: Int32
    func area() -> Int32 { return side * side } }
func measure(_ s: Shape) -> Int32 { return s.area() }
func main() -> Int32 { return measure(Square(side: 5)) }`)
	for _, d := range diags {
		t.Fatalf("refused: %s", d.Message)
	}
	for _, want := range []string{
		"alloc_stack $any Shape",
		"init_existential_addr",
		"witness_method #Shape.area",
		"open_existential_addr",
		"sil_witness_table hidden Square: Shape",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	// A call that named the implementation would be this compiler
	// deciding what only the value can decide. Square.area is reached
	// through a thunk in the table, so its symbol belongs there and
	// not in a function_ref inside measure.
	body := bodyOf(t, got, "$s4main7measureys5Int32VAA5Shape_pF")
	if strings.Contains(body, "function_ref") {
		t.Errorf("the implementation was named at the call site:\n%s", body)
	}
}

// bodyOf is one function out of a printed module.
func bodyOf(t *testing.T, module, symbol string) string {
	t.Helper()
	i := strings.Index(module, "@"+symbol+" :")
	if i < 0 {
		t.Fatalf("no function %s in:\n%s", symbol, module)
	}
	rest := module[i:]
	if j := strings.Index(rest, "end sil function"); j > 0 {
		rest = rest[:j]
	}
	return rest
}

// TestReassigningAnExistentialRewritesTheTable: what made this worth
// a test of its own.
//
// `v = Rect(...)` where v holds a Square replaces both halves of the
// existential. Storing over the buffer alone left Square's table in
// place, so the next call reached Square.area with Rect's bytes: the
// program returned 9 where swiftc returned 12, and nothing anywhere
// said a word. So the assignment initializes the storage again, and
// the second init_existential_addr is what says it did.
func TestReassigningAnExistentialRewritesTheTable(t *testing.T) {
	src := `
protocol Shape { func area() -> Int32 }
struct Square: Shape { var side: Int32
    func area() -> Int32 { return side * side } }
struct Rect: Shape { var w: Int32; var h: Int32
    func area() -> Int32 { return w * h } }
func main() -> Int32 {
    var v: any Shape = Square(side: 2)
    v = Rect(w: 3, h: 4)
    return v.area()
}`
	got, diags := generate(t, "main", src)
	for _, d := range diags {
		t.Fatalf("refused: %s", d.Message)
	}
	if n := strings.Count(got, "init_existential_addr"); n != 2 {
		t.Errorf("the table is written %d times, want 2 -- one for the "+
			"declaration and one for the assignment:\n%s", n, got)
	}
	// Both conformances have to be there for either to be reachable.
	for _, want := range []string{
		"sil_witness_table hidden Square: Shape",
		"sil_witness_table hidden Rect: Shape",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

// TestAnExistentialBindingIsStorage: a binding of existential type is
// a slot, not a value, and using it hands over the slot.
//
// Loading five words out of it and passing those on would pass the
// buffer's first register and nothing else. The load has no machine
// type either, so this was a lowering error rather than a wrong
// answer -- but the fix is the same fact: there is nothing to load.
func TestAnExistentialBindingIsStorage(t *testing.T) {
	got, diags := generate(t, "main", `
protocol Shape { func area() -> Int32 }
struct Square: Shape { var side: Int32
    func area() -> Int32 { return side * side } }
func inner(_ s: Shape) -> Int32 { return s.area() }
func outer(_ s: Shape) -> Int32 { return inner(s) }
func main() -> Int32 { return outer(Square(side: 5)) }`)
	for _, d := range diags {
		t.Fatalf("refused: %s", d.Message)
	}
	body := bodyOf(t, got, "$s4main5outerys5Int32VAA5Shape_pF")
	if strings.Contains(body, "load") {
		t.Errorf("the existential parameter was loaded rather than passed on:\n%s", body)
	}
	// And nothing was boxed a second time: what was already an
	// existential is passed as it is.
	if strings.Contains(body, "init_existential_addr") {
		t.Errorf("an existential was put inside another one:\n%s", body)
	}
}

// TestExistentialLimitsAreNamed: what this cannot lower, it refuses,
// and the refusal names the thing rather than its consequence.
//
// Each of these was reached by writing the program and reading what
// came out. Two of them ran: the class one produced a value the
// verifier caught, and the rest would have been leaks or releases of
// something already gone. The messages are what a reader gets instead.
func TestExistentialLimitsAreNamed(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a class in an existential", `
protocol Shape { func area() -> Int32 }
final class Box: Shape { var n: Int32 = 9
    func area() -> Int32 { return n } }
func measure(_ s: Shape) -> Int32 { return s.area() }
func main() -> Int32 { return measure(Box()) }`,
			"value witness table"},

		{"a value too wide for the buffer", `
protocol Shape { func area() -> Int32 }
struct Wide: Shape {
    var a: Int32; var b: Int32; var c: Int32
    var d: Int32; var e: Int32; var f: Int32
    var g: Int32; var h: Int32; var i: Int32; var j: Int32
    func area() -> Int32 { return a }
}
func measure(_ s: Shape) -> Int32 { return s.area() }
func main() -> Int32 { return measure(Wide(a: 1, b: 2, c: 3, d: 4, e: 5, f: 6, g: 7, h: 8, i: 9, j: 10)) }`,
			"boxes the rest"},

		{"a result that is an existential", `
protocol Shape { func area() -> Int32 }
struct Square: Shape { var side: Int32
    func area() -> Int32 { return side * side } }
func make() -> any Shape { return Square(side: 6) }
func main() -> Int32 { return 1 }`,
			"storage the caller set aside"},

		{"a stored property that is an existential", `
protocol Shape { func area() -> Int32 }
struct Square: Shape { var side: Int32
    func area() -> Int32 { return side * side } }
struct Holder { var s: any Shape }
func main() -> Int32 { return 1 }`,
			"stored property"},

		{"a protocol with an associated type", `
protocol Container { associatedtype Item
    func get() -> Item }
struct Ints: Container { func get() -> Int32 { return 7 } }
func take(_ c: any Container) -> Int32 { return 1 }
func main() -> Int32 { return take(Ints()) }`,
			"leaves 'Item' to the conforming type"},

		{"a value whose type cannot be named at run time", `
func take(_ x: Any) -> Int32 { return 1 }
func main() -> Int32 { return take((1, 2)) }`,
			"cannot name"},

		{"one existential copied into another binding", `
protocol Shape { func area() -> Int32 }
struct Square: Shape { var side: Int32
    func area() -> Int32 { return side * side } }
func measure(_ s: Shape) -> Int32 {
    let kept: any Shape = s
    return kept.area()
}
func main() -> Int32 { return measure(Square(side: 5)) }`,
			"copies what is inside it"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, said := refusals(t, c.src)
			if said == "" {
				t.Fatalf("lowered without a word about it")
			}
			if !strings.Contains(said, c.want) {
				t.Errorf("said %q\nwant it to mention %q", strings.TrimSpace(said), c.want)
			}
		})
	}
}
