package lower

import (
	"strings"
	"testing"
)

// Nesting.
//
// A struct with a struct inside it is the same list of scalars with
// the inner one's spliced in where it sat, and the nesting is a fact
// about the source rather than about the registers. These hold that
// to what it implies: an inner field ends up at the offset the memory
// layout gives it, and the packed form and the memory form agree
// because both are driven by the same list.

func TestNestingIsFlatInTheRegisters(t *testing.T) {
	m, err := lowerSrc(t, `
struct Inner { var a: Int32; var b: Int32 }
struct Outer { var inner: Inner; var n: Int32 }
func flat(_ o: Outer) -> Int32 { return o.inner.a + o.inner.b + o.n }
`)
	if err != nil {
		t.Fatalf("a nested struct did not lower: %v", err)
	}
	// Three Int32s is twelve bytes, so two words — and the nesting
	// changes nothing about that.
	got := dump(t, m)
	if !strings.Contains(got, "(%a0 i64, %a1 i64) i32") {
		t.Errorf("a nested struct was not passed in two words:\n%s", got)
	}
}

// TestAnInnerStructIsAWindow: extracting the inner struct whole takes
// no instructions, because its scalars are already there — the
// answer is a window onto the parts the outer one is made of.
func TestAnInnerStructIsAWindow(t *testing.T) {
	if _, err := lowerSrc(t, `
struct Inner { var a: Int32; var b: Int32 }
struct Outer { var inner: Inner; var n: Int32 }
func sum(_ i: Inner) -> Int32 { return i.a + i.b }
func use(_ o: Outer) -> Int32 { return sum(o.inner) + o.n }
`); err != nil {
		t.Errorf("an inner struct could not be taken out whole: %v", err)
	}
}

// TestNestingInMemory: a nested field's offset is the sum of the two,
// which is what the layout says and what lets the register form and
// the memory form meet without either knowing about the other.
func TestNestingInMemory(t *testing.T) {
	if _, err := lowerSrc(t, `
struct Inner { var a: Int32; var b: Int32 }
struct Outer { var inner: Inner; var n: Int32 }
func use() -> Int32 {
    var o = Outer(inner: Inner(a: 1, b: 2), n: 3)
    o.inner.b = o.inner.b + 1
    return o.inner.a + o.inner.b + o.n
}
`); err != nil {
		t.Errorf("a nested struct could not be written in memory: %v", err)
	}
}

// TestLeavesRefuseWhatTheyCannotPlace: a field with no register makes
// the whole struct unplaceable, because half a list would put every
// later scalar in the wrong one.
func TestLeavesRefuseWhatTheyCannotPlace(t *testing.T) {
	if _, err := lowerSrc(t, `
struct Holder { var pair: (Int32, Int32); var n: Int32 }
func use(_ h: Holder) -> Int32 { return h.n }
`); err == nil {
		t.Error("a struct with a field that has no register was passed in registers")
	}
}

// TestAnOptionalFieldIsItsPayloadAndTag: an optional that needs a tag is
// laid out as its payload with the tag byte after it, so a struct holding
// one is those scalars and its others, and the fields after it are found
// where the memory layout puts them.
func TestAnOptionalFieldIsItsPayloadAndTag(t *testing.T) {
	if _, err := lowerSrc(t, `
struct Holder { var maybe: Int32?; var name: String; var n: Int32 }
func use(_ h: Holder) -> Int32 { return h.n + (h.maybe ?? 0) }
func set() -> Int32 {
    var h = Holder(maybe: nil, name: "x", n: 1)
    h.maybe = 4
    return use(h)
}
`); err != nil {
		t.Errorf("a struct holding an optional could not be lowered: %v", err)
	}
}

// TestAStringFieldIsItsTwoWords: a String is two words wherever it is
// held, so a struct with one is those words and its other scalars, and
// passes and returns in registers like any struct of four or fewer.
func TestAStringFieldIsItsTwoWords(t *testing.T) {
	if _, err := lowerSrc(t, `
struct Holder { var name: String; var n: Int32 }
func make(_ s: String) -> Holder { return Holder(name: s, n: 3) }
func use(_ h: Holder) -> Int32 { return h.n }
`); err != nil {
		t.Errorf("a struct holding a String could not be lowered: %v", err)
	}
}
