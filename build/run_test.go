package build_test

// The strongest check available: compile Swift, write the object,
// link it against a C caller with the native toolchain, and run it.
//
// Everything else in this repo says the output is well-formed. The
// parser agrees with swiftc, the checker agrees with swiftc, the
// ownership IR verifies, the VIR verifies. None of that says the
// program computes the right answer, and this does.
//
// Skipped rather than failed off Apple Silicon or with no clang: a
// missing toolchain is a fact about the machine and not a defect.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	irverify "github.com/vertex-language/ir/verify"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

func TestFibRuns(t *testing.T) {
	got := runSwift(t, "fib", `
public func fib(_ n: Int) -> Int {
  if n < 2 { return n }
  return fib(n - 1) + fib(n - 2)
}
`, `
#include <stdio.h>
long fib(long n) __asm__("_$s3fib3fibyS2iF");
int main(void) {
    for (long i = 0; i < 10; i++) printf("%ld ", fib(i));
    return 0;
}
`)
	if want := "0 1 1 2 3 5 8 13 21 34 "; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// TestArithmeticRuns is the one that would catch a backend that
// selected the wrong instruction: every operator, on values chosen so
// that a sign error or a swapped operand changes the answer.
func TestArithmeticRuns(t *testing.T) {
	got := runSwift(t, "ar", `
public func add(_ a: Int, _ b: Int) -> Int { return a + b }
public func sub(_ a: Int, _ b: Int) -> Int { return a - b }
public func mul(_ a: Int, _ b: Int) -> Int { return a * b }
public func div(_ a: Int, _ b: Int) -> Int { return a / b }
public func rem(_ a: Int, _ b: Int) -> Int { return a % b }
public func neg(_ a: Int) -> Int { return -a }
public func inv(_ a: Int) -> Int { return ~a }
public func lt(_ a: Int, _ b: Int) -> Bool { return a < b }
public func gt(_ a: Int, _ b: Int) -> Bool { return a > b }
public func band(_ a: Int, _ b: Int) -> Int { return a & b }
public func bxor(_ a: Int, _ b: Int) -> Int { return a ^ b }
`, `
#include <stdio.h>
long add(long, long) __asm__("_$s2ar3addyS2i_SitF");
long sub(long, long) __asm__("_$s2ar3subyS2i_SitF");
long mul(long, long) __asm__("_$s2ar3mulyS2i_SitF");
long xdiv(long, long) __asm__("_$s2ar3divyS2i_SitF");
long xrem(long, long) __asm__("_$s2ar3remyS2i_SitF");
long neg(long) __asm__("_$s2ar3negyS2iF");
long inv(long) __asm__("_$s2ar3invyS2iF");
_Bool lt(long, long) __asm__("_$s2ar2ltySbSi_SitF");
_Bool gt(long, long) __asm__("_$s2ar2gtySbSi_SitF");
long band(long, long) __asm__("_$s2ar4bandyS2i_SitF");
long bxor(long, long) __asm__("_$s2ar4bxoryS2i_SitF");
int main(void) {
    printf("%ld %ld %ld %ld %ld ", add(3,4), sub(3,4), mul(3,4), xdiv(-9,2), xrem(-9,2));
    printf("%ld %ld ", neg(5), inv(5));
    printf("%d %d ", lt(3,4), gt(3,4));
    printf("%ld %ld", band(12,10), bxor(12,10));
    return 0;
}
`)
	// -9/2 truncates toward zero, so it is -4 and the remainder is -1.
	// ~5 is -6. 12&10 is 8 and 12^10 is 6.
	if want := "7 -1 12 -4 -1 -5 -6 1 0 8 6"; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// TestOverflowTraps: the check Swift promises is not decoration. An
// addition that overflows has to stop the program, and the only way to
// know it does is to run one.
func TestOverflowTraps(t *testing.T) {
	bin := buildSwift(t, "ov", `
public func add(_ a: Int, _ b: Int) -> Int { return a + b }
`, `
#include <stdio.h>
long add(long, long) __asm__("_$s2ov3addyS2i_SitF");
int main(void) {
    printf("%ld\n", add(0x7fffffffffffffffL, 1));
    return 0;
}
`)
	err := exec.Command(bin).Run()
	if err == nil {
		t.Fatal("the addition overflowed and the program carried on")
	}
	if !strings.Contains(err.Error(), "trace/BPT trap") && !strings.Contains(err.Error(), "signal") {
		t.Errorf("the program failed, but not by trapping: %v", err)
	}
}

// TestProgramRuns is the entry point end to end: a Swift program
// with a main, compiled to an object, linked with no C source of any
// kind, and run for its exit status.
//
// Every other test here links against a C caller, which proves the
// code is right but leaves the platform's own question unasked: does
// this object start a process? The answer is a symbol. Mach-O calls
// the entry point _main, mangles nothing about it, and finds it by
// that name or not at all.
func TestProgramRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
func helper(_ n: Int32) -> Int32 { return n + 39 }
func main() -> Int32 { return helper(3) }
`, "")
	cmd := exec.Command(bin)
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("run: %v", err)
		}
	}
	if got := cmd.ProcessState.ExitCode(); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestLoopRuns is the imperative half of the language, end to end: a
// mutable local, a loop that reads and writes it, and an answer that
// is wrong if any of the three is.
//
// It is the first test here whose program could not be written as an
// expression. What makes it work below the front end is two mandatory
// passes — the box SILGen gives every `var` promoted to a stack slot,
// and the `assign` that promotion leaves behind resolved to a store —
// so a failure in either shows up as a number, not as a refusal.
func TestLoopRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
func total(_ k: Int32) -> Int32 {
    var n: Int32 = 0
    var i: Int32 = 0
    while i < k {
        n = n + i
        i = i + 1
    }
    return n
}

func main() -> Int32 { return total(10) }
`, "")
	cmd := exec.Command(bin)
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("run: %v", err)
		}
	}
	// 0 + 1 + ... + 9.
	if got := cmd.ProcessState.ExitCode(); got != 45 {
		t.Errorf("exit status = %d, want 45", got)
	}
}

// TestBreakAndContinueRun: the answer is wrong by a different amount
// for each way of getting the control flow wrong, which is the point
// of choosing these numbers.
func TestBreakAndContinueRun(t *testing.T) {
	bin := buildSwift(t, "main", `
func f(_ k: Int32) -> Int32 {
    var n: Int32 = 0
    var i: Int32 = 0
    while i < k {
        i = i + 1
        if i == 3 { continue }
        if i == 7 { break }
        n = n + i
    }
    return n
}

func main() -> Int32 { return f(100) }
`, "")
	// 1 + 2 + 4 + 5 + 6: three is skipped, seven stops it.
	if got := runExit(t, bin); got != 18 {
		t.Errorf("exit status = %d, want 18", got)
	}
}

// TestLabelledBreakRuns: the inner loop would stop at 5 and the outer
// would run five times, so leaving the wrong loop gives 25 and
// leaving none gives something larger.
func TestLabelledBreakRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
func f() -> Int32 {
    var n: Int32 = 0
    var i: Int32 = 0
    outer: while i < 5 {
        i = i + 1
        var j: Int32 = 0
        while j < 5 {
            j = j + 1
            n = n + 1
            if n == 7 { break outer }
        }
    }
    return n
}

func main() -> Int32 { return f() }
`, "")
	if got := runExit(t, bin); got != 7 {
		t.Errorf("exit status = %d, want 7", got)
	}
}

// TestAVariableInsideALoopRuns is the one that catches a frame slot
// reserved in the wrong place.
//
// SIL writes alloc_stack where the variable was declared, which here
// is a block that runs many times; VIR admits a frame allocation in
// the entry block only. Getting that wrong does not produce a bad
// number — it puts the IR builder into its sticky failure state, and
// what breaks is the next function to ask it for anything.
func TestAVariableInsideALoopRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
func f() -> Int32 {
    var total: Int32 = 0
    var i: Int32 = 0
    while i < 4 {
        var step: Int32 = 2
        step = step + i
        total = total + step
        i = i + 1
    }
    return total
}

func main() -> Int32 { return f() }
`, "")
	// (2+0) + (2+1) + (2+2) + (2+3) = 14.
	if got := runExit(t, bin); got != 14 {
		t.Errorf("exit status = %d, want 14", got)
	}
}

// TestShortCircuitRuns proves the skipped operand is really skipped,
// by putting something in it that would end the process.
//
// A divide by zero traps. If && evaluated its right operand anyway,
// this program would die on a signal rather than exit; if it
// evaluated neither correctly, the answer would be the other branch.
// Only a real short circuit gives 9.
func TestShortCircuitRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
func f(_ a: Int32, _ z: Int32) -> Int32 {
    if a > 5 && (a / z) > 0 { return 1 }
    if a > 0 || (a / z) > 0 { return 9 }
    return 2
}

func main() -> Int32 { return f(1, 0) }
`, "")
	cmd := exec.Command(bin)
	err := cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("run: %v", err)
		}
	}
	if !cmd.ProcessState.Exited() {
		t.Fatalf("the program was killed rather than exiting: the right operand ran")
	}
	if got := cmd.ProcessState.ExitCode(); got != 9 {
		t.Errorf("exit status = %d, want 9", got)
	}
}

// TestGuardTernaryAndRepeatRun: the three control-flow forms that are
// variations on ones already here, each with an answer that is wrong
// by a different amount if its shape is wrong.
func TestGuardTernaryAndRepeatRun(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want int
	}{
		// A guard that holds continues; one that does not takes the
		// else. Both arms are exercised by the same function.
		{"guard holds", `
func f(_ a: Int32) -> Int32 {
    guard a > 1 else { return 3 }
    return a
}
func main() -> Int32 { return f(9) }`, 9},

		{"guard does not hold", `
func f(_ a: Int32) -> Int32 {
    guard a > 1 else { return 3 }
    return a
}
func main() -> Int32 { return f(0) }`, 3},

		// A ternary picks one arm and evaluates only it.
		{"ternary", `
func main() -> Int32 {
    let a: Int32 = 5
    return a > 3 ? 10 : 20
}`, 10},

		// The body runs before the test, so a false condition still
		// runs it once. A while loop here would answer 0.
		{"repeat runs once", `
func f(_ k: Int32) -> Int32 {
    var i: Int32 = 0
    repeat { i = i + 1 } while i < k
    return i
}
func main() -> Int32 { return f(0) }`, 1},

		// continue goes to the test rather than to the body, so the
		// loop still terminates.
		{"repeat with break and continue", `
func f() -> Int32 {
    var i: Int32 = 0
    repeat {
        i = i + 1
        if i == 3 { continue }
        if i == 5 { break }
    } while i < 100
    return i
}
func main() -> Int32 { return f() }`, 5},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := runExit(t, buildSwift(t, "main", c.src, "")); got != c.want {
				t.Errorf("exit status = %d, want %d", got, c.want)
			}
		})
	}
}

// TestAStructRuns: a wrapper type, made, passed, returned, and read.
//
// A struct of one field is that field once the names are gone — Int
// and Bool are exactly this, a struct around one builtin — so a
// wrapper a program declares lowers the same way and costs nothing at
// runtime. Anything wider is memory, which this compiler does not lay
// out yet.
func TestAStructRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
struct Meters { var value: Int32 }

func add(_ a: Meters, _ b: Meters) -> Meters {
    return Meters(value: a.value + b.value)
}

func main() -> Int32 {
    let total = add(Meters(value: 40), Meters(value: 2))
    return total.value
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestAWiderStructRuns: three fields, built and read back, with the
// numbers chosen so that a field read from the wrong position gives a
// different answer.
//
// Nothing is emitted for either the building or the reading — the
// fields were computed where they were written and the struct is the
// list of them — so what this checks is that the list is in the right
// order and stays that way through the backend.
func TestAWiderStructRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
struct Split { var big: Int32; var mid: Int32; var small: Int32 }

func main() -> Int32 {
    let s = Split(big: 32, mid: 8, small: 2)
    // Weighted so that any two fields swapped changes the answer.
    return s.big + s.mid + s.mid + s.small
}
`, "")
	if got := runExit(t, bin); got != 50 {
		t.Errorf("exit status = %d, want 50", got)
	}
}

// TestAStructOfMixedFieldsRuns: the registers are not all the same
// width, so a field taken from the wrong slot is a type error in the
// backend rather than a wrong number — which is worth having a
// running test for either way.
func TestAStructOfMixedFieldsRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
struct Flagged { var n: Int32; var on: Bool }

func main() -> Int32 {
    let f = Flagged(n: 42, on: true)
    if f.on { return f.n }
    return 0
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// Classes, allocation and ARC, end to end.
//
// The runtime these link against is not a library anyone shipped: it
// is a VIR module runtime/ builds and this package compiles with the
// same backend as the program, for the same target. So what these
// exercise is the whole of it — the allocator, the reference counts,
// the free, and the compiler's agreement with all three about where
// an object's properties begin.

// TestAClassRuns: allocated on the heap, its property read back, and
// released.
func TestAClassRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
final class Box { var n: Int32 = 42 }

func main() -> Int32 {
    let b = Box()
    return b.n
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestAWideClassRuns is the one that catches an instance sized as a
// reference.
//
// Sizeof of a class is one word, because a class value is a
// reference — the right answer for a register and a catastrophic one
// for an allocation. A class with four Int32 properties given eight
// bytes writes past the end of what it was given, and the fields
// that land outside come back as whatever was there. Four fields
// chosen so that any of them reading wrong changes the answer.
func TestAWideClassRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
final class Wide {
    var a: Int32 = 1
    var b: Int32 = 2
    var c: Int32 = 4
    var d: Int32 = 8
}

func main() -> Int32 {
    let w = Wide()
    return w.a + w.b + w.c + w.d
}
`, "")
	if got := runExit(t, bin); got != 15 {
		t.Errorf("exit status = %d, want 15", got)
	}
}

// TestReferenceCountsBalance: a copy retains and both references
// release, so the object outlives the first of them and does not
// outlive the second.
//
// A retain that did not happen frees the object while it is still in
// use, and the read that follows returns whatever the allocator put
// there next. Either way the answer moves.
func TestReferenceCountsBalance(t *testing.T) {
	bin := buildSwift(t, "main", `
final class Box { var n: Int32 = 7 }

func keep(_ b: Box) -> Box {
    let kept = b
    return kept
}

func main() -> Int32 {
    let a = Box()
    let b = keep(a)
    return a.n + b.n
}
`, "")
	if got := runExit(t, bin); got != 14 {
		t.Errorf("exit status = %d, want 14", got)
	}
}

// TestAllocationInALoopDoesNotGrow: a thousand objects made and
// dropped one at a time.
//
// The count is the point. If release never reached free, this would
// hold every one of them at once — which `leaks` reports on and a
// test cannot, so what is checked here is that the arithmetic across
// a thousand allocations is still right and the process still exits.
func TestAllocationInALoopDoesNotGrow(t *testing.T) {
	bin := buildSwift(t, "main", `
final class Counter { var n: Int32 = 1 }

func main() -> Int32 {
    var total: Int32 = 0
    var i: Int32 = 0
    while i < 1000 {
        let c = Counter()
        total = total + c.n
        i = i + 1
    }
    // 1000 wraps to 232 in the exit status a process may give back.
    return total
}
`, "")
	if got := runExit(t, bin); got != 1000%256 {
		t.Errorf("exit status = %d, want %d", got, 1000%256)
	}
}

// TestAFreestandingLinkTakesNoRuntime: a program that says it needs
// none gets none, and one that calls the runtime anyway fails to
// link rather than silently resolving to something else.
func TestAFreestandingLinkTakesNoRuntime(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	u, diags := vsc.Compile([]vsc.Source{{Name: "main.swift", Text: []byte(`
final class Box { var n: Int32 = 1 }
func main() -> Int32 {
    let b = Box()
    return b.n
}
`)}}, vsc.Options{Module: "main", Target: target})
	for _, d := range diags {
		t.Fatalf("%s", d)
	}
	obj, err := build.Object(u.VIR, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = build.Executable([]build.Input{{Name: "main.o", Data: obj}},
		build.LinkOptions{Target: target, Freestanding: true})
	if err == nil {
		t.Error("a freestanding link resolved the runtime it was told not to take")
	}
}

// TestAStructAgreesWithC is the ABI held to something outside this
// compiler.
//
// A struct of eight bytes is passed in one register on AArch64, with
// the first field in the low half and the second in the high half.
// Swift does that and so does C, so a C caller passing its own struct
// to a Vertex function is a check that the packing is the platform's
// and not merely self-consistent — which is what every other test
// here would still pass if it were wrong in both directions at once.
//
// The agreement stops at sixteen bytes: C passes a larger composite by
// address, Swift passes up to thirty-two in four registers, and this
// follows Swift. So the struct here is deliberately small enough that
// the two conventions coincide.
func TestAStructAgreesWithC(t *testing.T) {
	got := runSwift(t, "ab", `
public struct Pair { public var lo: Int32; public var hi: Int32 }

public func widen(_ p: Pair) -> Int32 { return p.hi - p.lo }
`, `
#include <stdio.h>
struct Pair { int lo; int hi; };
int widen(struct Pair) __asm__("_$s2ab5widenys5Int32VAA4PairVF");
int main(void) {
    struct Pair p = { .lo = 8, .hi = 50 };
    printf("%d", widen(p));
    return 0;
}
`)
	if want := "42"; got != want {
		t.Errorf("got %q, want %q — the packing is not the platform's", got, want)
	}
}

// TestAStructReturnedToC: the same agreement in the other direction,
// which is a different half of the convention and can be wrong on its
// own.
func TestAStructReturnedToC(t *testing.T) {
	got := runSwift(t, "rb", `
public struct Pair { public var lo: Int32; public var hi: Int32 }

public func make(_ a: Int32, _ b: Int32) -> Pair { return Pair(lo: a, hi: b) }
`, `
#include <stdio.h>
struct Pair { int lo; int hi; };
struct Pair make(int, int) __asm__("_$s2rb4makeyAA4PairVs5Int32V_AFtF");
int main(void) {
    struct Pair p = make(11, 31);
    printf("%d", p.lo + p.hi);
    return 0;
}
`)
	if want := "42"; got != want {
		t.Errorf("got %q, want %q — a returned struct is not packed the platform's way", got, want)
	}
}

// TestAMutableStructRuns: a struct in a stack slot, written a field
// at a time and read back.
//
// The struct is not stored whole — there is no register wide enough,
// and going through the packed word form would write padding the
// layout does not have — so each field goes to its own offset. An
// offset computed wrong puts a field where another one lives, and the
// numbers here are chosen so that any such overlap changes the
// answer.
func TestAMutableStructRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
struct Acc { var total: Int32; var count: Int32 }

func main() -> Int32 {
    var a = Acc(total: 0, count: 0)
    var i: Int32 = 0
    while i < 5 {
        a.total = a.total + i
        a.count = a.count + 1
        i = i + 1
    }
    // 10 * 5: wrong on either field if the two share storage.
    return a.total * a.count
}
`, "")
	if got := runExit(t, bin); got != 50 {
		t.Errorf("exit status = %d, want 50", got)
	}
}

// TestAMutableStructOfMixedWidthsRuns: a Bool is one byte in memory
// and an Int32 is four, so a field written at the wrong width
// overwrites its neighbour rather than landing beside it.
func TestAMutableStructOfMixedWidthsRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
struct M { var n: Int32; var on: Bool }

func main() -> Int32 {
    var m = M(n: 40, on: false)
    m.on = true
    m.n = m.n + 2
    if m.on { return m.n }
    return 0
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestAMutableStructCrossesACall: written in memory, then passed in
// registers — the two representations have to agree about where each
// field is, which is why both are driven by the same layout.
func TestAMutableStructCrossesACall(t *testing.T) {
	bin := buildSwift(t, "main", `
struct P { var x: Int32; var y: Int32 }

func sum(_ p: P) -> Int32 { return p.x + p.y }

func main() -> Int32 {
    var p = P(x: 40, y: 1)
    p.y = p.y + 1
    return sum(p)
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestANestedStructRuns: two levels, built, passed, mutated in
// memory, and read back.
//
// The inner struct's scalars sit inside the outer one's list, and an
// inner field's offset is the sum of the two. Getting either wrong
// puts a field where another one lives, so the numbers are chosen so
// that any overlap shows.
func TestANestedStructRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
struct Inner { var a: Int32; var b: Int32 }
struct Outer { var inner: Inner; var n: Int32 }

func flat(_ o: Outer) -> Int32 { return o.inner.a + o.inner.b + o.n }

func main() -> Int32 {
    var o = Outer(inner: Inner(a: 30, b: 8), n: 1)
    o.n = o.n + 1
    o.inner.b = o.inner.b + 2
    return flat(o)
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestAnInnerStructCrossesACall: taken out of its parent whole and
// passed on, which is the case that needs a struct value to be a
// window onto scalars rather than a thing held somewhere.
func TestAnInnerStructCrossesACall(t *testing.T) {
	bin := buildSwift(t, "main", `
struct Inner { var a: Int32; var b: Int32 }
struct Outer { var inner: Inner; var n: Int32 }

func sum(_ i: Inner) -> Int32 { return i.a + i.b }

func main() -> Int32 {
    let o = Outer(inner: Inner(a: 30, b: 10), n: 2)
    return sum(o.inner) + o.n
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestMethodsRun: a struct's and a class's, end to end.
//
// The receiver travels as the last argument, so a convention that put it
// anywhere else would pass an argument as self and self as an argument.
// The numbers are chosen so that either mistake changes the answer.
func TestMethodsRun(t *testing.T) {
	bin := buildSwift(t, "main", `
struct Rect {
    var w: Int32
    var h: Int32
    func area() -> Int32 { return w * h }
    func scaled(_ k: Int32) -> Int32 { return area() * k }
}

final class Counter {
    var n: Int32 = 0
    func bump(_ by: Int32) -> Int32 { n = n + by; return n }
}

func main() -> Int32 {
    let r = Rect(w: 3, h: 4)
    // scaled calls area through the implicit receiver: 12 * 2.
    if r.scaled(2) != 24 { return 1 }

    let c = Counter()
    if c.bump(20) != 20 { return 2 }
    // The class keeps what the first call wrote: 20 + 10.
    return r.area() + c.bump(10)
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// runExit runs a program and returns its exit status, failing only if
// it did not run at all.
func runExit(t *testing.T, bin string) int {
	t.Helper()
	cmd := exec.Command(bin)
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("run: %v", err)
		}
	}
	return cmd.ProcessState.ExitCode()
}

// TestProgramExitsZero: a main that returns nothing still gives the
// operating system a status, and the status is zero.
func TestProgramExitsZero(t *testing.T) {
	bin := buildSwift(t, "main", `func main() {}`, "")
	if err := exec.Command(bin).Run(); err != nil {
		t.Errorf("run: %v", err)
	}
}

// --- the harness ---

func runSwift(t *testing.T, module, swift, mainC string) string {
	t.Helper()
	out, err := exec.Command(buildSwift(t, module, swift, mainC)).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	return string(out)
}

// buildSwift compiles the Swift, writes the object, links it against
// the C and returns the executable.
func buildSwift(t *testing.T, module, swift, mainC string) string {
	t.Helper()
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon; skipping the link-and-run check")
	}
	clang, err := exec.LookPath("clang")
	if err != nil {
		t.Skip("no clang on PATH; skipping the link-and-run check")
	}
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}

	u, diags := vsc.Compile([]vsc.Source{{
		Name: module + ".swift", Text: []byte(swift),
	}}, vsc.Options{Module: module, Target: target})
	for _, d := range diags {
		t.Fatalf("%s", d)
	}
	if err := irverify.Module(u.VIR); err != nil {
		t.Fatalf("verify: %v", err)
	}

	obj, err := build.Object(u.VIR, build.Options{})
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	objPath := filepath.Join(dir, module+".o")
	if err := os.WriteFile(objPath, obj, 0o644); err != nil {
		t.Fatal(err)
	}
	// The runtime goes in alongside, the same object build.Executable
	// would have linked. These tests hand the pieces to clang so that
	// a C caller can be linked in too, which means the linking this
	// package does for itself is not what is under test here — but a
	// program that makes a class still calls the allocator.
	rt, err := build.Runtime(target)
	if err != nil {
		t.Fatal(err)
	}
	rtPath := filepath.Join(dir, "vertex_runtime.o")
	if err := os.WriteFile(rtPath, rt.Data, 0o644); err != nil {
		t.Fatal(err)
	}

	args := []string{"-o", filepath.Join(dir, module)}
	// A program with its own main needs nothing else linked in. The
	// empty string is that case, and is not a C file with no content.
	if mainC != "" {
		mainPath := filepath.Join(dir, "main.c")
		if err := os.WriteFile(mainPath, []byte(mainC), 0o644); err != nil {
			t.Fatal(err)
		}
		args = append(args, mainPath)
	}
	args = append(args, objPath, rtPath)
	if out, err := exec.Command(clang, args...).CombinedOutput(); err != nil {
		t.Fatalf("link: %v\n%s", err, out)
	}
	return filepath.Join(dir, module)
}
