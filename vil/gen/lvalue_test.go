package gen

import (
	"strings"
	"testing"
)

// Writing through a name.
//
// The bug these hold the line against gave a wrong answer rather than
// a refusal: lvalue() knew how to address a class's property and fell
// through to nil for a struct's, assign() saw the nil and returned,
// and `p.y = p.y + 1` did nothing at all in a program that compiled,
// linked and ran. The read that had been emitted to find the base was
// left behind, so the output even looked busy.

// TestAStructFieldIsAssignedThroughItsAddress: the base has to be an
// address too, because a struct's property is inside the struct's own
// storage — reading the struct out into a value first would write
// into the copy.
func TestAStructFieldIsAssignedThroughItsAddress(t *testing.T) {
	got, diags := generate(t, "main", `
struct P { var x: Int32; var y: Int32 }
func main() -> Int32 {
    var p = P(x: 40, y: 1)
    p.y = p.y + 1
    return p.x + p.y
}`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	if !strings.Contains(got, "struct_element_addr") {
		t.Errorf("the field was not addressed:\n%s", got)
	}
	// Something has to be written back, or the assignment is a read
	// and nothing else.
	if !strings.Contains(got, "assign ") && !strings.Contains(got, "store ") {
		t.Errorf("the assignment wrote nothing:\n%s", got)
	}
}

// TestAClassPropertyIsAddressedThroughTheReference: the other half of
// the same choice. A class value is a reference, so the base is a
// value and the property is arithmetic on it.
func TestAClassPropertyIsAddressedThroughTheReference(t *testing.T) {
	got, diags := generate(t, "main", `
final class Box { var n: Int32 = 1 }
func main() -> Int32 {
    let b = Box()
    let v: Int32 = 42
    b.n = v
    return b.n
}`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	if !strings.Contains(got, "ref_element_addr") {
		t.Errorf("the property was not addressed through the reference:\n%s", got)
	}
	if strings.Contains(got, "struct_element_addr") {
		t.Errorf("a class property was addressed as a struct's:\n%s", got)
	}
}

// TestAnAssignmentThatCannotBeLoweredIsReported: silence here is the
// dangerous case, so it is the one under test.
func TestAnAssignmentThatCannotBeLoweredIsReported(t *testing.T) {
	_, diags := generate(t, "main", `
func main() -> Int32 {
    var a = [1, 2, 3]
    a[0] = 4
    return 0
}`)
	if len(diags) == 0 {
		t.Fatal("an assignment that could not be lowered was passed over in silence")
	}
	if msg := diags[0].Message; !strings.Contains(msg, "cannot lower") {
		t.Errorf("said %q", msg)
	}
}
