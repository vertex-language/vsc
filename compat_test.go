package vsc_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/ir"

	"github.com/vertex-language/vsc"
)

// Gaps in the Swift compatibility layer that were once wrong answers
// rather than missing features. Each test is the program that was
// mis-compiled, kept so it cannot become one again.

// TestNilCoalescingNarrowWrapped: `a ?? 0` against an int32? left the
// literal at its default of Int, failed the assignability test, and
// fell out of the case still optional -- a wrong type, reported as a
// return-type mismatch the source could not fix.
func TestNilCoalescingNarrowWrapped(t *testing.T) {
	const src = `
func pick(_ a: int32?) -> int32 { return a ?? 0 }
func widen(_ a: int?) -> int { return a ?? 0 }
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestNilCoalescingKeepsOptionalForm is Swift's other overload:
// `T? ?? T?` is a T?, not a T.
func TestNilCoalescingKeepsOptionalForm(t *testing.T) {
	const src = `
func pick(_ a: int32?, _ b: int32?) -> int32? { return a ?? b }
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestNilCoalescingMismatch: operands that genuinely disagree are a
// diagnostic, not a quietly optional result.
func TestNilCoalescingMismatch(t *testing.T) {
	const src = `
func pick(_ a: int32?, _ b: bool) -> int32 { return a ?? b }
`
	_, diags := compile(t, src, vsc.Options{Stop: vsc.Checked})
	if !vsc.Errors(diags) {
		t.Fatal("'??' with mismatched operands was accepted")
	}
	if !strings.Contains(diags[0].Message, "'??'") {
		t.Errorf("diagnostic does not name the operator: %s", diags[0])
	}
}

// TestMutatingOnClassRefused: Swift rejects `mutating` on a class
// method outright. Accepting it silently let the modifier mean
// nothing.
func TestMutatingOnClassRefused(t *testing.T) {
	const src = `
class Box {
    var v: int32 = 0
    mutating func bump() { v = v + 1 }
}
`
	_, diags := compile(t, src, vsc.Options{Stop: vsc.Checked})
	if !vsc.Errors(diags) {
		t.Fatal("'mutating' on a class method was accepted")
	}
	if !strings.Contains(diags[0].Message, "'mutating' is not valid on instance methods") {
		t.Errorf("diagnostic does not say why: %s", diags[0])
	}
}

// TestMutatingOnStructAccepted is the other side of it: the modifier
// still means what it means on a value type.
func TestMutatingOnStructAccepted(t *testing.T) {
	const src = `
struct S {
    var v: int32 = 0
    mutating func bump() { self.v = 1 }
}
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestTopLevelCodeReported: statements at file scope were checked and
// then discarded, and the only symptom was an undefined _main at the
// link. They are reported now.
func TestTopLevelCodeReported(t *testing.T) {
	const src = `
func side(_ a: int32) -> int32 { return a }

side(3)

func main() -> int32 { return 0 }
`
	_, diags := compile(t, src, vsc.Options{Module: "main"})
	if !vsc.Errors(diags) {
		t.Fatal("top-level code was accepted and dropped")
	}
	if !strings.Contains(diags[0].Message, "top-level code") {
		t.Errorf("diagnostic does not name the problem: %s", diags[0])
	}
}

// TestNoEntryPointReported: a program with no main is the compiler's
// to report, in its own words, rather than the linker's as an
// undefined symbol.
func TestNoEntryPointReported(t *testing.T) {
	const src = `func add(_ a: int32) -> int32 { return a }`
	_, diags := compile(t, src, vsc.Options{Module: "main"})
	if !vsc.Errors(diags) {
		t.Fatal("a program with no entry point was accepted")
	}
	if !strings.Contains(diags[0].Message, "no entry point") {
		t.Errorf("diagnostic does not name the problem: %s", diags[0])
	}
}

// TestLibraryNeedsNoEntryPoint: the rule is about the entry module.
// Any other module's main is an ordinary function, and a library need
// not have one at all.
func TestLibraryNeedsNoEntryPoint(t *testing.T) {
	const src = `public func add(_ a: int32) -> int32 { return a }`
	if _, diags := compile(t, src, vsc.Options{Module: "lib"}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestTwoModulesSharingAName: every import was declared into one
// shared scope, and Scope.Insert keeps the first symbol of a name --
// so the second module's same-named declaration existed nowhere, and
// its qualified name could not be resolved. Each module now has a
// scope of its own, with no parent, so a qualified name is exact.
func TestTwoModulesSharingAName(t *testing.T) {
	iface := func(name string) vsc.Source {
		return vsc.Source{Name: name + ".vertexinterface", Text: []byte(
			"// vertex-interface-format-version: 1.0\n" +
				"// vertex-module-name: " + name + "\n\n" +
				"public func width() -> Int32\n")}
	}
	dir := t.TempDir()
	for _, name := range []string{"A", "B"} {
		src := iface(name)
		write(t, filepath.Join(dir, src.Name), string(src.Text))
	}
	const prog = `
import A
import B

func main() -> int32 { return A.width() + B.width() }
`
	_, diags := vsc.Compile(
		[]vsc.Source{{Name: "m.vs", Text: []byte(prog)}},
		vsc.Options{Module: "main", Target: ir.AArch64MacOS, Stop: vsc.Checked,
			ImportPaths: []string{dir}})
	for _, d := range diags {
		t.Errorf("%s", d)
	}
}

// TestMutatingMethodWritesThroughSelf: `self` was passed by value and
// never @inout, so a mutating method had nothing to write to that the
// caller would see -- the assignment was refused rather than lowered.
// It is handed the receiver's storage now.
func TestMutatingMethodWritesThroughSelf(t *testing.T) {
	const src = `
struct Counter {
    var n: int32
    mutating func bump() { n = n + 1 }
    mutating func bumpTwice() { bump(); bump() }
    mutating func setTo(_ k: int32) { self.n = k }
    func read() -> int32 { return n }
}

func use() -> int32 {
    var c = Counter(n: 0)
    c.bumpTwice()
    c.setTo(7)
    return c.read()
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestNonMutatingWriteStillRefused is the other half: a method that
// changes a value receiver without saying `mutating` writes to a copy
// nobody sees, and is still refused.
func TestNonMutatingWriteStillRefused(t *testing.T) {
	const src = `
struct S {
    var x: int32
    func bad(_ k: int32) { x = x * k }
}
`
	if _, diags := compile(t, src, vsc.Options{}); !vsc.Errors(diags) {
		t.Error("a non-mutating method wrote to its receiver")
	}
}

// TestInoutReceiverLowers: an inout receiver is what `mutating` is on
// a method written outside the braces, and it reaches the same
// convention.
func TestInoutReceiverLowers(t *testing.T) {
	const src = `
struct S { var x: int32 }
func (s: inout S) scale(_ k: int32) { s.x = s.x * k }

func use() -> int32 {
    var v = S(x: 7)
    v.scale(6)
    return v.x
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestNonMutatingMethodAfterMutating: self is cleared per function.
// Left set, an ordinary method lowered after a mutating one read its
// properties through an address belonging to the other function.
func TestNonMutatingMethodAfterMutating(t *testing.T) {
	const src = `
struct S {
    var x: int32
    mutating func bump() { x = x + 1 }
    func read() -> int32 { return x }
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestFloatToIntegerConversion: `Int32(d)` was refused because the
// bound Swift traps on was never computed. It is computed against the
// source's own type now -- a signed destination of n bits holds
// [-2^(n-1), 2^(n-1)), and both powers of two are exact in binary
// floating point -- so the test is the range rather than an
// approximation of it, and a NaN fails it and traps.
func TestFloatToIntegerConversion(t *testing.T) {
	const src = `
func toInt32(_ d: double) -> int32 { return int32(d) }
func toInt64(_ d: double) -> int64 { return int64(d) }
func fromFloat(_ f: float) -> int32 { return int32(f) }
func unsigned(_ d: double) -> uint32 { return uint32(d) }
func toInt8(_ d: double) -> int8 { return int8(d) }
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestPayloadEnumAcrossCalls: a payload enum that fits in one word
// had no register. Its image is words, and a multi-word one is passed
// as those words -- but one word is one leaf, so that path did not
// apply and machineOf refused every payload enum outright. It was
// usable inside a function and had no machine type at a boundary.
func TestPayloadEnumAcrossCalls(t *testing.T) {
	const src = `
enum Op { case add(int32), scale(int32), neg }
enum Wide { case box(int32, int32), empty }

func make(_ k: int32) -> Op { return Op.add(k) }
func roundTrip(_ o: Op) -> Op { return o }

func apply(_ o: Op, _ n: int32) -> int32 {
    switch o {
    case .add(let k): return n + k
    case .scale(let k): return n * k
    case .neg: return -n
    }
}

func area(_ w: Wide) -> int32 {
    switch w {
    case .box(let a, let b): return a * b
    case .empty: return 0
    }
}

func use() -> int32 {
    return apply(make(5), 10) + apply(roundTrip(Op.neg), 3) + area(Wide.box(2, 5))
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestStaticComputedProperty: a static may be stored or computed and
// the two shared one list, so a computed one was counted among the
// properties needing storage and a one-time initializer -- which a
// getter with nothing behind it has no use for. It is a call now, to
// a getter with no receiver.
func TestStaticComputedProperty(t *testing.T) {
	const src = `
struct Vec {
    var x: int32
    static var zero: Vec { return Vec(x: 0) }
    static var unit: Vec { return Vec(x: 1) }
}

class Limits {
    static var high: int32 { return 100 }
}

func use() -> int32 { return Vec.zero.x + Vec.unit.x + Limits.high }
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestStaticStoredPropertyStillRefused is the other half: a stored
// static does need storage and a one-time initializer, and there are
// no globals yet, so it is still refused -- and for that reason.
func TestStaticStoredPropertyStillRefused(t *testing.T) {
	const src = `
struct C { static var base: int32 = 100 }
func use() -> int32 { return C.base }
`
	_, diags := compile(t, src, vsc.Options{})
	if !vsc.Errors(diags) {
		t.Fatal("a stored static was lowered with no storage behind it")
	}
	if !strings.Contains(diags[0].Message, "storage of its own") {
		t.Errorf("diagnostic does not say why: %s", diags[0])
	}
}

// TestEnumComputedAndStaticMembers: readMembers took a var
// declaration only where the type had somewhere to put all three
// kinds, and an enum has no stored properties and so no field sink --
// which dropped its computed and static ones with them.
func TestEnumComputedAndStaticMembers(t *testing.T) {
	const src = `
enum Dir {
    case up, down
    static var count: int32 { return 2 }
    var code: int32 { return self == Dir.up ? 1 : 2 }
    func flipped() -> Dir { return self == Dir.up ? Dir.down : Dir.up }
}

func use() -> int32 {
    let d = Dir.up
    return Dir.count + d.code + d.flipped().code
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestStoredPropertyInEnumRefused: an enum's storage is its cases.
// Swift refuses a stored property there; this used to drop it.
func TestStoredPropertyInEnumRefused(t *testing.T) {
	const src = `
enum E {
    case a
    var x: int32 = 5
}
`
	_, diags := compile(t, src, vsc.Options{Stop: vsc.Checked})
	if !vsc.Errors(diags) {
		t.Fatal("a stored property in an enum was accepted")
	}
	if !strings.Contains(diags[0].Message, "enums must not contain stored properties") {
		t.Errorf("diagnostic is not swiftc's: %s", diags[0])
	}
}

// TestBareStaticNameInMember: a type's statics are in scope
// unqualified inside its own members, the way its properties are
// through implicit self. The analyzer resolved one; lowering stopped
// at the name.
func TestBareStaticNameInMember(t *testing.T) {
	const src = `
struct Vec {
    var x: int32
    static var unit: Vec { return Vec(x: 1) }
    static var two: Vec { return Vec(x: unit.x * 2) }
    var scaled: int32 { return x * unit.x }
}

func use() -> int32 { return Vec.two.x + Vec(x: 2).scaled }
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestComputedPropertyOnClassBorrowsReceiver: a getter's self is
// @guaranteed the way a method's is, so handing over an owned copy
// left a class reference nothing destroyed -- invalid VIL rather than
// a refusal, caught by the verifier.
func TestComputedPropertyOnClassBorrowsReceiver(t *testing.T) {
	const src = `
class Box {
    var n: int32 = 5
    var doubled: int32 { return n * 2 }
}

func use() -> int32 {
    let b = Box()
    return b.doubled + Box().doubled
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestBareComputedNameInMember: `doubled` inside another member means
// `self.doubled`, the way a bare stored name means `self.n`. It
// resolved and had no case in lowering.
func TestBareComputedNameInMember(t *testing.T) {
	const src = `
struct Vec {
    var x: int32
    var doubled: int32 { return x * 2 }
    var quad: int32 { return doubled * 2 }
    func viaMethod() -> int32 { return doubled + 1 }
    mutating func grow() { x = x + doubled }
}

enum Flag {
    case on
    var one: int32 { return 1 }
    var two: int32 { return one * 2 }
}

func use() -> int32 {
    var v = Vec(x: 3)
    v.grow()
    return v.quad + v.viaMethod() + Flag.on.two
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}
