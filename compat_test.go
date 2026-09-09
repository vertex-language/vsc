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

// TestAccessorSelectedByKeyword: which accessor a block is was decided
// by which came first, not by its keyword -- so a property writing
// `set` before `get` had its setter emitted as the getter. It
// compiled and ran, answering whatever the setter's body left behind.
func TestAccessorSelectedByKeyword(t *testing.T) {
	const src = `
struct Reading {
    var raw: int32
    var doubled: int32 {
        set { raw = newValue / 2 }
        get { return raw * 2 }
    }
    var tripled: int32 {
        get { return raw * 3 }
        set { raw = newValue / 3 }
    }
    var negated: int32 { return -raw }
}

func use() -> int32 {
    let r = Reading(raw: 7)
    return r.doubled + r.tripled + r.negated
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestAssignToComputedCallsSetter: a computed property is written by
// calling its setter. There is no storage to store into, and taking
// an address named a field the type does not have -- which the
// backend reported about a struct_element_addr, with no line to look
// at.
func TestAssignToComputedCallsSetter(t *testing.T) {
	const src = `
struct S {
    var raw: int32
    var doubled: int32 {
        get { return raw * 2 }
        set { raw = newValue / 2 }
    }
    var named: int32 {
        get { return raw }
        set(v) { raw = v + 1 }
    }
}

class Box {
    var n: int32 = 0
    var doubled: int32 {
        get { return n * 2 }
        set { n = newValue / 2 }
    }
}

func use() -> int32 {
    var s = S(raw: 5)
    s.doubled = 20
    s.named = 6
    let b = Box()
    b.doubled = 14
    return s.raw + b.n
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestCompoundAssignToComputed: `c.d += n` is a read and a write
// around an operator, so it is a call, the operator, and a call. The
// plain assignment path could not do it, and it was refused.
func TestCompoundAssignToComputed(t *testing.T) {
	const src = `
struct T {
    var raw: int32
    var d: int32 {
        get { return raw * 2 }
        set { raw = newValue / 2 }
    }
}

class B {
    var n: int32 = 4
    var d: int32 {
        get { return n * 2 }
        set { n = newValue / 2 }
    }
}

func use() -> int32 {
    var t = T(raw: 5)
    t.d += 10
    t.d -= 4
    t.d *= 2
    let b = B()
    b.d += 8
    return t.raw + b.n
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestRangePattern: a range matches everything between its bounds, so
// what agrees with the subject is the element and not the range.
// Comparing the range itself made every range pattern an error, at
// every subject type -- and the bounds defaulted to Int rather than
// taking the subject's width.
func TestRangePattern(t *testing.T) {
	const src = `
func grade(_ n: int32) -> int32 {
    switch n {
    case 0..<10: return 1
    case 10...20: return 2
    default: return 9
    }
}

func wide(_ n: int) -> int {
    switch n {
    case 1...5: return 2
    default: return 4
    }
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestCaseLetWhere: a binding in a case pattern is what the where
// clause is written about, so it has to be declared before the
// condition is read. It was read first.
func TestCaseLetWhere(t *testing.T) {
	const src = `
func classify(_ n: int32) -> int32 {
    switch n {
    case let k where k < 0: return -k
    case let k where k > 100: return 100
    default: return 0
    }
}
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestSwitchBindingAndWhere: a case may bind what it matched and
// guard it with a condition. Neither was lowered -- a binding pattern
// was refused, and a where clause with it.
func TestSwitchBindingAndWhere(t *testing.T) {
	const src = `
func classify(_ n: int32) -> int32 {
    switch n {
    case 0: return 1
    case let k where k < 0: return -k
    case 1..<10: return 2
    case 10...20 where n % 2 == 0: return 3
    case let k where k > 100: return k / 10
    default: return 9
    }
}

func doubleIt(_ n: int32) -> int32 {
    switch n {
    case 0: return 0
    case let k: return k * 2
    }
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestMemberwiseDefaults: a memberwise initializer may leave a
// property to its default. The default is an expression on the
// declaration rather than at the call, so the call had nothing to
// lower and every such construction was refused.
func TestMemberwiseDefaults(t *testing.T) {
	const src = `
struct Config {
    var width: int32 = 10
    var height: int32 = 20
    var depth: int32
}

struct All {
    var a: int32 = 1
    var b: int32 = 2
}

struct Derived {
    var base: int32 = 6 * 7
}

func use() -> int32 {
    let a = Config(depth: 3)
    let b = Config(width: 5, depth: 7)
    let d = All()
    let e = All(a: 9)
    return a.width + b.height + d.b + e.a + Derived().base
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestTuplePattern: a tuple pattern matches element by element, so
// each is checked against the subject's element at that position.
// Passing the whole tuple down made every element an error.
func TestTuplePattern(t *testing.T) {
	const src = `
func pairKind(_ p: (int32, int32)) -> int32 {
    switch p {
    case (0, 0): return 1
    case (let a, 0): return a
    case (0, let b): return b
    case (let a, let b): return a + b
    }
}
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestObservedPropertyWrite: willSet and didSet were parsed and
// dropped, so a write stored the value and ran neither observer -- a
// program that compiled, ran, and did half of what its source says.
// A write is the store with the observers around it now.
func TestObservedPropertyWrite(t *testing.T) {
	const src = `
struct S {
    var log: int32 = 0
    var both: int32 = 0 {
        willSet { log = log * 10 + 1 }
        didSet { log = log * 10 + 2 }
    }
    var onlyWill: int32 = 0 { willSet { log = newValue } }
    var onlyDid: int32 = 0 { didSet { log = oldValue } }
    var named: int32 = 0 {
        willSet(nv) { log = nv }
        didSet(ov) { log = ov }
    }
}

class C {
    var log: int32 = 0
    var n: int32 = 0 { didSet { log = oldValue + 7 } }
}

func use() -> int32 {
    var s = S()
    s.both = 1
    s.onlyWill = 2
    s.onlyDid = 3
    s.named = 4
    s.both += 5
    let c = C()
    c.n = 3
    return s.log + c.log
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestUnobservedPropertyStillWrites is the other half: an ordinary
// stored property is unaffected.
func TestUnobservedPropertyStillWrites(t *testing.T) {
	const src = `
struct S {
    var n: int32 = 0
}

func use() -> int32 {
    var s = S()
    s.n = 5
    s.n += 2
    return s.n
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestOptionalChainIsOptional: `p?.x` is the member where p holds
// something and nothing where it does not. The lookup ran on a doubly
// optional type -- `p?` wraps whatever it followed, and p was already
// optional -- found nothing, and answered Invalid without reporting
// it. So the chain was assignable to anything, and `??` saw a left
// side that was not an optional at all.
func TestOptionalChainIsOptional(t *testing.T) {
	const src = `
struct P { var x: int32 }

func chain(_ p: P?) -> int32? { return p?.x }
func withDefault(_ p: P?) -> int32 { return p?.x ?? -1 }
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestOptionalChainNotAssignableToBare is the other half: losing the
// optionality let a chain be assigned to a non-optional, which Swift
// refuses.
func TestOptionalChainNotAssignableToBare(t *testing.T) {
	const src = `
struct P { var x: int32 }

func chain(_ p: P?) -> int32 {
    let v: int32 = p?.x
    return v
}
`
	_, diags := compile(t, src, vsc.Options{Stop: vsc.Checked})
	if !vsc.Errors(diags) {
		t.Fatal("an optional chain was assigned to a non-optional")
	}
	if !strings.Contains(diags[0].Message, "Int32?") {
		t.Errorf("diagnostic does not name the optional: %s", diags[0])
	}
}

// TestInitReturn: `return` in an initializer leaves early with what
// has been built. It was checked against the type being made, so a
// bare return was Void where the type was wanted -- the one return
// statement an initializer may write did not compile.
func TestInitReturn(t *testing.T) {
	const src = `
struct Clamped {
    var n: int32
    init(_ v: int32) {
        self.n = v
        if v > 10 { return }
        self.n = v * 2
    }
}

final class Counter {
    var n: int32
    init(_ v: int32) {
        self.n = v
        return
    }
}

func use() -> int32 { return Clamped(20).n + Clamped(3).n + Counter(4).n }
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestFailableInitChecks: `init?` may return nil, which is the one
// thing it exists to do and was a type error. Lowering one is still
// refused, so this stops at the checker.
func TestFailableInitChecks(t *testing.T) {
	const src = `
struct Even {
    var n: int32
    init?(_ v: int32) {
        if v % 2 != 0 { return nil }
        self.n = v
    }
}
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestCustomOperatorOverLiterals: a literal operand has no type of
// its own to match a declaration with, and an operator's operands
// have no context until the operator is known. Both defaulted to Int,
// so an operator declared over Int32 did not fit and the result fell
// back to the left operand's type -- wrong whatever the operator
// returns, and silently so where the two agree.
func TestCustomOperatorOverLiterals(t *testing.T) {
	const src = `
infix operator <+> : AdditionPrecedence
func <+> (a: int32, b: int32) -> int32 { return a * 10 + b }

infix operator <=> : ComparisonPrecedence
func <=> (a: int32, b: int32) -> bool { return a < b }

func sum() -> int32 { return 2 <+> 3 }
func less() -> bool { return 1 <=> 2 }
`
	if _, diags := compile(t, src, vsc.Options{Stop: vsc.Checked}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestCustomOperatorResultType is the other half: the result is the
// operator's, not the left operand's, so misusing it is reported
// against the type it actually answers.
func TestCustomOperatorResultType(t *testing.T) {
	const src = `
infix operator <=> : ComparisonPrecedence
func <=> (a: int32, b: int32) -> bool { return a < b }

func use() -> int32 {
    let r: int32 = 1 <=> 2
    return r
}
`
	_, diags := compile(t, src, vsc.Options{Stop: vsc.Checked})
	if !vsc.Errors(diags) {
		t.Fatal("a Bool-returning operator was accepted as an int32")
	}
	if !strings.Contains(diags[0].Message, "Bool") {
		t.Errorf("diagnostic names the operand's type, not the operator's: %s", diags[0])
	}
}

// TestEnumRawValue: the raw type was never recorded, so `rawValue`
// did not exist -- and exposing it without lowering it answered the
// case's tag, which is not the value the source wrote.
func TestEnumRawValue(t *testing.T) {
	const src = `
enum Code: int32 {
    case ok = 1
    case bad = 7
}

enum Step: int32 {
    case first
    case second
    case third
}

enum Port: int32 {
    case low = 10
    case mid
}

func use() -> int32 {
    return Code.bad.rawValue + Step.third.rawValue + Port.mid.rawValue
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestStringRawValueRefused: a String raw value needs a string
// constant to answer with, and there is no making one yet -- so it is
// refused rather than answered with something else.
func TestStringRawValueRefused(t *testing.T) {
	const src = `
enum Name: string {
    case a = "x"
    case b = "y"
}

func use() -> string { return Name.a.rawValue }
`
	if _, diags := compile(t, src, vsc.Options{}); !vsc.Errors(diags) {
		t.Error("a string rawValue was lowered")
	}
}

// TestForceUnwrapAndNilComparison: `o!` had no case in lowering at
// all, and `o == nil` none either -- nil is not a value of a type
// core declares an operator over, so both are questions about which
// case the optional holds.
func TestForceUnwrapAndNilComparison(t *testing.T) {
	const src = `
func force(_ o: int32?) -> int32 { return o! }
func chained(_ o: int32?) -> int32 { return o! * 2 + o! }

func present(_ o: int32?) -> int32 {
    let a: int32 = o == nil ? 1 : 0
    let b: int32 = o != nil ? 10 : 0
    let c: int32 = nil == o ? 100 : 0
    return a + b + c
}
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}

// TestOptionalChainLowers: the chain is a switch with the member read
// inside one arm, which is what makes it a chain -- the read only
// happens where there is something to read from.
func TestOptionalChainLowers(t *testing.T) {
	const src = `
struct Reading { var value: int32 }

func read(_ r: Reading?) -> int32 { return r?.value ?? -1 }
func present(_ r: Reading?) -> int32 { return r?.value != nil ? 1 : 0 }
func forced(_ r: Reading?) -> int32 { return (r?.value)! }
`
	if _, diags := compile(t, src, vsc.Options{}); vsc.Errors(diags) {
		for _, d := range diags {
			t.Errorf("%s", d)
		}
	}
}
