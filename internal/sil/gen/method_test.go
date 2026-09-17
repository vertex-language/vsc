package gen

import (
	"strings"
	"testing"
)

// Methods.
//
// A method is an ordinary function with the receiver as its last
// parameter, which is what `swiftc -emit-sil` shows for a struct's and
// for a final class's alike:
//
//	sil @P.add : $@convention(method) (Int, Int, P) -> Int
//	%6 = function_ref @P.add
//	%7 = apply %6(%3, %5, %0)
//
// Self last is the convention's shape rather than a choice: it is what
// puts the receiver where a method looks for it, and reversing it would
// pass the first argument as the receiver.

// TestAMethodTakesSelfLast holds the shape at both ends — the
// declaration's parameter list and the call's argument list.
func TestAMethodTakesSelfLast(t *testing.T) {
	got, diags := generate(t, "main", `
struct P {
    var x: Int32
    func add(_ n: Int32) -> Int32 { return x + n }
}
func main() -> Int32 {
    let p = P(x: 20)
    return p.add(22)
}`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	if !strings.Contains(got, "$@convention(method) (Int32, P) -> Int32") {
		t.Errorf("the receiver is not the last parameter:\n%s", got)
	}
	if !strings.Contains(got, "function_ref @$s4main1PV3add") {
		t.Errorf("the method was not named inside its type:\n%s", got)
	}
}

// TestAMethodBodyIsLowered: a method is a function, and a walk that
// looked only at top-level declarations left its symbol declared and
// undefined — which links to nothing.
func TestAMethodBodyIsLowered(t *testing.T) {
	got, diags := generate(t, "main", `
struct P {
    var x: Int32
    func doubled() -> Int32 { return x * 2 }
}
func main() -> Int32 {
    let p = P(x: 21)
    return p.doubled()
}`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	// A declaration prints with no braces; a definition has a body.
	if !strings.Contains(got, "7doubleds5Int32VyF : $@convention(method) (P) -> Int32 {") {
		t.Errorf("the method has no body:\n%s", got)
	}
}

// TestAPropertyIsReachedThroughTheReceiver: a bare name in a method
// body that is a stored property means self's, and the analyzer
// resolved it to the symbol the type's own scope holds.
func TestAPropertyIsReachedThroughTheReceiver(t *testing.T) {
	got, diags := generate(t, "main", `
struct P {
    var x: Int32
    func get() -> Int32 { return x }
}
func main() -> Int32 { return P(x: 42).get() }`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	if !strings.Contains(got, "struct_extract %0, #P.x") {
		t.Errorf("the property was not read out of the receiver:\n%s", got)
	}
}

// TestAClassReceiverIsBorrowed: self is @guaranteed, so the caller
// keeps it alive across the call and the callee does not consume it.
// Copying it instead hands over a value nothing afterwards destroys,
// which the verifier catches as a leak.
func TestAClassReceiverIsBorrowed(t *testing.T) {
	got, diags := generate(t, "main", `
final class Box {
    var n: Int32 = 21
    func doubled() -> Int32 { return n * 2 }
}
func main() -> Int32 {
    let b = Box()
    return b.doubled()
}`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	if !strings.Contains(got, "@guaranteed Box") {
		t.Errorf("the receiver is not guaranteed:\n%s", got)
	}
	if strings.Contains(got, "copy_value") {
		t.Errorf("the receiver was copied for a borrowed parameter:\n%s", got)
	}
}

// TestABareCallInAMethodIsSelfs: `doubled()` inside another method of
// the same type is `self.doubled()`, and its symbol is mangled inside
// the type. Named as a free function it links to nothing.
func TestABareCallInAMethodIsSelfs(t *testing.T) {
	got, diags := generate(t, "main", `
struct P {
    var x: Int32
    func doubled() -> Int32 { return x * 2 }
    func quad() -> Int32 { return doubled() + doubled() }
}
func main() -> Int32 { return P(x: 10).quad() }`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	if strings.Contains(got, "@$s4main7doubled") {
		t.Errorf("a method was named as a free function:\n%s", got)
	}
	if !strings.Contains(got, "@$s4main1PV7doubled") {
		t.Errorf("the method was not named inside its type:\n%s", got)
	}
}

// TestWritingThroughAValueReceiverIsRefused: a struct receiver arrives
// by value, so there is nothing to write into that the caller would
// see. Swift says the same by requiring `mutating`; neither that nor an
// inout self is modelled, so this refuses rather than writing to a copy
// and losing it.
func TestWritingThroughAValueReceiverIsRefused(t *testing.T) {
	_, diags := generate(t, "main", `
struct P {
    var x: Int32
    func bad(_ v: Int32) -> Int32 { x = v; return x }
}
func main() -> Int32 { return P(x: 1).bad(2) }`)
	if len(diags) == 0 {
		t.Fatal("a write through a value receiver was lowered")
	}
	if msg := diags[0].Message; !strings.Contains(msg, "mutating") {
		t.Errorf("said %q, want it to name what is missing", msg)
	}
	// One mistake, one diagnostic: assign must not add a second, vaguer
	// one on top of the specific one lvalue already gave.
	if len(diags) != 1 {
		t.Errorf("one mistake produced %d diagnostics", len(diags))
	}
}
