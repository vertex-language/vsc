package gen

import (
	"strings"
	"testing"
)

// Making an instance.
//
// A struct with no initializer of its own is made by the memberwise
// one, and `swiftc -emit-sil` shows that initializer's whole body to
// be a `struct` instruction and a return — so this emits the
// instruction rather than a call to a function nothing declares. It
// is the decision literal() already documents, for the same reason.

// TestStructIsBuiltInPlace: the constructor call becomes the
// instruction the initializer would have run.
func TestStructIsBuiltInPlace(t *testing.T) {
	got, diags := generate(t, "main", `
struct P { var x: Int32; var y: Int32 }
func main() -> Int32 {
    let p = P(x: 40, y: 2)
    return p.x
}`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	if !strings.Contains(got, "struct $P (") {
		t.Errorf("no struct instruction:\n%s", got)
	}
	// A call to an initializer nothing declares would be worse than
	// the body of the one that would have been called.
	if strings.Contains(got, "function_ref @$s4main1P") {
		t.Errorf("called an initializer that does not exist:\n%s", got)
	}
	for _, want := range []string{"integer_literal $Builtin.Int32, 40", "integer_literal $Builtin.Int32, 2"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q — an argument was not lowered:\n%s", want, got)
		}
	}
}

// TestArgumentsKeepTheirOrder: the fields are positional once the
// labels are gone, so an argument in the wrong slot is a different
// struct. The checker holds the labels to declaration order, and this
// holds the lowering to the same order.
func TestArgumentsKeepTheirOrder(t *testing.T) {
	got, diags := generate(t, "main", `
struct P { var x: Int32; var y: Int32 }
func main() -> Int32 {
    let p = P(x: 40, y: 2)
    return p.x
}`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	first := strings.Index(got, "integer_literal $Builtin.Int32, 40")
	second := strings.Index(got, "integer_literal $Builtin.Int32, 2")
	if first < 0 || second < 0 || first > second {
		t.Errorf("the arguments were not lowered in field order:\n%s", got)
	}
}

// TestAClassIsAllocatedAndInitialized: SILGen calls
// __allocating_init, which allocates and then stores each stored
// property's initial value. This emits that inlined, which is the
// decision construct() and literal() both take.
func TestAClassIsAllocatedAndInitialized(t *testing.T) {
	got, diags := generate(t, "main", `
final class Box { var n: Int32 = 3 }
func main() -> Int32 {
    let b = Box()
    return b.n
}`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	for _, want := range []string{
		"alloc_ref $Box",
		"integer_literal $Builtin.Int32, 3",
		"ref_element_addr %0, #Box.n",
		"store ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// The reference is owned, so it is released where it stops being
	// held. A class that is never released is a leak in every program
	// that makes one.
	if !strings.Contains(got, "destroy_value") && !strings.Contains(got, "strong_release") {
		t.Errorf("the instance is never released:\n%s", got)
	}
}

// TestAClassWithoutADefaultIsRefused: a class gets no memberwise
// initializer — Swift gives that to structs alone — so a property
// with no initial value has nowhere to get one from.
func TestAClassWithoutADefaultIsRefused(t *testing.T) {
	_, diags := generate(t, "main", `
final class Box { var n: Int32 }
func main() -> Int32 {
    let b = Box()
    return b.n
}`)
	if len(diags) == 0 {
		t.Fatal("a class with an uninitialized property was made")
	}
	msg := diags[0].Message
	for _, want := range []string{"'n'", "no initial value"} {
		if !strings.Contains(msg, want) {
			t.Errorf("said %q, want it to mention %q", msg, want)
		}
	}
}

// TestASubclassInitializerIsRefused: a class's initializer is two
// functions rather than one -- an allocating entry point and an
// initializing one -- and both are emitted. What is not is a
// subclass's, which has to run the initializer above it on the same
// instance, and `super.init` is not lowered.
func TestASubclassInitializerIsRefused(t *testing.T) {
	_, diags := generate(t, "main", `
class Base { var n: Int32 = 1 }
final class Box: Base {
    var m: Int32
    init(m: Int32) { self.m = m }
}
func main() -> Int32 {
    let b = Box(m: 3)
    return b.m
}`)
	if len(diags) == 0 {
		t.Fatal("a subclass's initializer was lowered")
	}
	if msg := diags[0].Message; !strings.Contains(msg, "superclass") {
		t.Errorf("said %q, want it to say the superclass is what is missing", msg)
	}
}

// TestAnExplicitInitializerLowers: a struct that says how it is made
// does not also get the memberwise initializer, so the call has to
// reach the one it declared.
//
// self is a var being filled in, marked uninitialized until the body
// has done so, and the value returned is what the body left there --
// which is SILGen's shape for the same declaration.
func TestAnExplicitInitializerLowers(t *testing.T) {
	out, diags := generate(t, "main", `
struct P {
    var x: Int32
    init(both: Int32) { x = both }
}
func main() -> Int32 {
    let p = P(both: 3)
    return p.x
}`)
	for _, d := range diags {
		t.Fatalf("refused: %s", d.Message)
	}
	for _, want := range []string{
		"mark_uninitialized [rootself]",
		"struct_element_addr",
		"@thin P.Type",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the initializer has no %q in it:\n%s", want, out)
		}
	}
}
