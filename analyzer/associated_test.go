package analyzer

import (
	"strings"
	"testing"
)

// Associated types: what a protocol leaves to the conformer.
//
// The rule underneath every case here is that `C.Item` is a
// dependency and not a type. What makes it an answer is a conformer's
// choice, and the two ways of making one -- saying it with a
// typealias, and implying it by writing the implementation -- have to
// agree with each other and with what swiftc reads.

// TestAConformerChoosesTheAssociatedType: both ways of saying it, and
// what happens when neither is used.
func TestAConformerChoosesTheAssociatedType(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"said with a typealias", `
protocol Container { associatedtype Item
    func get() -> Item }
struct S: Container {
    typealias Item = Int32
    func get() -> Int32 { return 1 }
}
`, ""},
		{"implied by the implementation", `
protocol Container { associatedtype Item
    func get() -> Item }
struct S: Container { func get() -> Int32 { return 1 } }
`, ""},
		{"implied by a stored property", `
protocol Container { associatedtype Item
    var head: Item { get } }
struct S: Container { var head: Int32 }
`, ""},
		{"implied through a parameter", `
protocol Container { associatedtype Item
    func take(_ x: Item) -> Int32 }
struct S: Container { func take(_ x: Bool) -> Int32 { return 1 } }
`, ""},
		{"implied through a structure", `
protocol Container { associatedtype Item
    func all() -> [Item] }
struct S: Container { func all() -> [Int32] { return [] } }
`, ""},
		{"nothing says", `
protocol Container { associatedtype Item
    func count() -> Int32 }
struct S: Container { func count() -> Int32 { return 1 } }
`, "nothing says what 'Item' is"},
		{"the choice does not meet the constraint", `
protocol Number { func value() -> Int32 }
protocol Container { associatedtype Item: Number
    func get() -> Item }
struct S: Container { func get() -> Int32 { return 1 } }
`, "'Item' is 'Int32', which does not conform to 'Number'"},
		{"the implementation does not match", `
protocol Container { associatedtype Item
    func get() -> Item
    func take(_ x: Item) -> Int32 }
struct S: Container {
    func get() -> Int32 { return 1 }
    func take(_ x: Bool) -> Int32 { return 1 }
}
`, "missing 'take'"},
	} {
		t.Run(c.name, func(t *testing.T) {
			msgs := checkSnippet(t, c.src)
			if c.want == "" {
				if len(msgs) != 0 {
					t.Errorf("want a clean check, got %v", msgs)
				}
				return
			}
			if len(msgs) == 0 {
				t.Fatalf("want a diagnostic mentioning %q, got none", c.want)
			}
			for _, m := range msgs {
				if strings.Contains(m, c.want) {
					return
				}
			}
			t.Errorf("said %v\nwant one mentioning %q", msgs, c.want)
		})
	}
}

// TestARequirementIsSaidAboutWhatPromisedIt: a requirement declared as
// `-> Self.Item` promises, of a `C: Container`, a `C.Item`.
//
// Handing back the requirement's own spelling instead made `return
// c.get()` a return of Self.Item where C.Item was expected -- one
// type under two names, reported as a mismatch.
func TestARequirementIsSaidAboutWhatPromisedIt(t *testing.T) {
	src := `
protocol Container { associatedtype Item
    func get() -> Item }
struct S: Container { func get() -> Int32 { return 1 } }
func firstOf<C: Container>(_ c: C) -> C.Item { return c.get() }
`
	if msgs := checkSnippet(t, src); len(msgs) != 0 {
		t.Errorf("%v", msgs)
	}
}

// TestAssociatedTypesAreCheckedThroughTheirConstraints: what is known
// about C.Item before anything says what it is.
func TestAssociatedTypesAreCheckedThroughTheirConstraints(t *testing.T) {
	constrained := `
protocol Number { func value() -> Int32 }
protocol Container { associatedtype Item: Number
    func get() -> Item }
struct N: Number { func value() -> Int32 { return 1 } }
struct S: Container { func get() -> N { return N() } }
func valueOf<C: Container>(_ c: C) -> Int32 { return c.get().value() }
`
	if msgs := checkSnippet(t, constrained); len(msgs) != 0 {
		t.Errorf("a constrained associated type did not carry its constraint: %v", msgs)
	}
}

// TestAWhereClauseSaysWhatTheBracketsCannot: `<C: Container>` says
// what C is and cannot say anything about C.Item, because C.Item is
// not a parameter.
func TestAWhereClauseSaysWhatTheBracketsCannot(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"a same-type requirement", `
protocol Container { associatedtype Item
    func get() -> Item }
struct S: Container { func get() -> Int32 { return 1 } }
func doubled<C: Container>(_ c: C) -> Int32 where C.Item == Int32 { return c.get() * 2 }
`},
		{"a conformance requirement", `
protocol Number { func value() -> Int32 }
protocol Container { associatedtype Item
    func get() -> Item }
struct N: Number { func value() -> Int32 { return 1 } }
struct S: Container { func get() -> N { return N() } }
func valued<C: Container>(_ c: C) -> Int32 where C.Item: Number { return c.get().value() }
`},
		{"a constraint on the parameter itself", `
protocol Container { associatedtype Item
    func get() -> Item }
struct S: Container { func get() -> Int32 { return 1 } }
func firstOf<C>(_ c: C) -> C.Item where C: Container { return c.get() }
`},
	} {
		t.Run(c.name, func(t *testing.T) {
			if msgs := checkSnippet(t, c.src); len(msgs) != 0 {
				t.Errorf("%v", msgs)
			}
		})
	}
}

// TestAnAssociatedTypeThatIsNotPromisedIsReported: `C.Nonesuch` where
// nothing C conforms to declares one.
//
// The message names the parameter and the name, because the mistake
// is one or the other: a misspelling, or a constraint that was not
// written.
func TestAnAssociatedTypeThatIsNotPromisedIsReported(t *testing.T) {
	src := `
protocol Container { associatedtype Item
    func get() -> Item }
struct S: Container { func get() -> Int32 { return 1 } }
func firstOf<C: Container>(_ c: C) -> C.Nonesuch { return c.get() }
`
	msgs := checkSnippet(t, src)
	if len(msgs) == 0 {
		t.Fatal("an associated type nothing declares was accepted")
	}
	want := "nothing 'C' conforms to declares an associated type 'Nonesuch'"
	if !strings.Contains(msgs[0], want) {
		t.Errorf("said %q\nwant it to mention %q", msgs[0], want)
	}
}

// TestAProtocolMayNameATypeDeclaredBelowIt: a protocol's requirements
// are read with the other types' members rather than with the
// declarations, so the order of the file does not matter.
//
// This was not true before associated types needed a second pass
// anyway, and Swift's rule is that it should be.
func TestAProtocolMayNameATypeDeclaredBelowIt(t *testing.T) {
	src := `
protocol Container { func get() -> Later }
struct Later { var n: Int32 }
struct S: Container { func get() -> Later { return Later(n: 1) } }
`
	if msgs := checkSnippet(t, src); len(msgs) != 0 {
		t.Errorf("%v", msgs)
	}
}
