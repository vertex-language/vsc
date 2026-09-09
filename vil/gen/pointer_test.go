package gen

import (
	"strings"
	"testing"
)

// Unsafe pointers.
//
// The shape to hold to is swiftc's, minus one instruction: Swift's
// pointer is a struct around a Builtin.RawPointer and reaches through
// it with struct_extract before every use, and this one is the
// address itself. Everything after that line is the same.

// TestPointeeIsALoad: `p.pointee` is pointer_to_address and a load,
// which is what swiftc emits for it.
func TestPointeeIsALoad(t *testing.T) {
	got, said := refusals(t, `
func rd(_ p: UnsafePointer<Int32>) -> Int32 { return p.pointee }
func main() -> Int32 { return 0 }`)
	if said != "" {
		t.Fatalf("refused: %s", said)
	}
	for _, want := range []string{"pointer_to_address", "load [trivial]"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestPointeeWriteIsAStore: the same address, written through.
func TestPointeeWriteIsAStore(t *testing.T) {
	got, said := refusals(t, `
func wr(_ p: UnsafeMutablePointer<Int32>, _ v: Int32) { p.pointee = v }
func main() -> Int32 { return 0 }`)
	if said != "" {
		t.Fatalf("refused: %s", said)
	}
	if !strings.Contains(got, "pointer_to_address") || !strings.Contains(got, "assign") {
		t.Errorf("not a store through the address:\n%s", got)
	}
}

// TestAmpersandIsAnAddress: `&x` where a pointer is wanted hands over
// where x lives. The access is still opened -- the callee may write
// through it, and the scope is what says so.
func TestAmpersandIsAnAddress(t *testing.T) {
	got, said := refusals(t, `
func bump(_ p: UnsafeMutablePointer<Int32>) { p.pointee += 1 }
func main() -> Int32 { var x: Int32 = 1; bump(&x); return x }`)
	if said != "" {
		t.Fatalf("refused: %s", said)
	}
	for _, want := range []string{"begin_access [modify]", "address_to_pointer"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestPointerConversionMovesNothing: every unsafe pointer is the same
// address, so reading one as another is no instruction at all.
func TestPointerConversionMovesNothing(t *testing.T) {
	got, said := refusals(t, `
func f(_ p: UnsafeMutablePointer<Int32>) -> Int32 {
    let q = UnsafePointer<Int32>(OpaquePointer(UnsafeRawPointer(p)))
    return q.pointee
}
func main() -> Int32 { return 0 }`)
	if said != "" {
		t.Fatalf("refused: %s", said)
	}
	// One load, one address, and nothing in between: three
	// conversions that each produced an instruction would show up
	// here as three more values.
	if n := strings.Count(got, "pointer_to_address"); n != 1 {
		t.Errorf("pointer_to_address appears %d times, want 1:\n%s", n, got)
	}
}

// TestWritingThroughAnImmutablePointer: `UnsafePointer<T>` is C's
// `const T *`, and a write through one is refused.
//
// `p.pointee` on a raw or an opaque pointer is refused too, and by
// the checker rather than here -- see tests/check, where the
// diagnostic is compared against swiftc's.
func TestWritingThroughAnImmutablePointer(t *testing.T) {
	_, said := refusals(t, `
func f(_ p: UnsafePointer<Int32>) { p.pointee = 1 }
func main() -> Int32 { return 0 }`)
	if said == "" {
		t.Fatal("built it and said nothing")
	}
	if !strings.Contains(said, "may only read") {
		t.Errorf("said %q, want it to say the pointer may only read", said)
	}
}
