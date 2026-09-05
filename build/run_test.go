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
	"syscall"
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

// TestSwitchOnEnumRuns: an enum with no associated values is its tag,
// and a switch over one is a jump table indexed by it. Every case is
// taken here, including the last, because a table that is one short is
// a table that runs off its end.
func TestSwitchOnEnumRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
enum Color { case red, green, blue }

func code(_ c: Color) -> Int32 {
    switch c {
    case .red: return 1
    case .green: return 10
    case .blue: return 100
    }
}

func main() -> Int32 {
    // 1 + 10 + 100, then the same three again through a variable, so
    // that the tag is read out of storage as well as made on the spot.
    var total: Int32 = code(.red) + code(.green) + code(.blue)
    let c = Color.green
    total = total + code(c)
    return total - 79
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestSwitchOnValueRuns: a subject that is not an enum is a chain of
// comparisons rather than a table — Swift's `~=` — so what is checked
// is that a later case is reached only when every earlier one failed,
// and that the default catches what none of them matched.
func TestSwitchOnValueRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
func classify(_ n: Int32) -> Int32 {
    switch n {
    case 0: return 3
    case 1: return 5
    case 2: return 7
    default: return 11
    }
}

func main() -> Int32 {
    // 3 + 5 + 7 + 11 + 11: the two out-of-range subjects both fall to
    // the default rather than to the nearest case.
    return classify(0) + classify(1) + classify(2) + classify(3) + classify(-1)
}
`, "")
	if got := runExit(t, bin); got != 37 {
		t.Errorf("exit status = %d, want 37", got)
	}
}

// TestSwitchFallsOutToWhatFollows: a case body does not fall into the
// next one and does not end the function either — it goes to whatever
// comes after the switch. `break` goes to the same place, and both
// have to leave the continuation reachable.
func TestSwitchFallsOutToWhatFollows(t *testing.T) {
	bin := buildSwift(t, "main", `
enum Step { case one, two, three }

func main() -> Int32 {
    var n: Int32 = 0
    let s = Step.two
    switch s {
    case .one:
        n = 1
    case .two:
        n = 20
        break
    case .three:
        n = 300
    }
    // Reached from the case body, which is the point: had .two fallen
    // into .three this would be 300, and had it returned, 20.
    return n + 22
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestSwitchInsideLoop: `break` inside a switch leaves the switch and
// `continue` leaves the iteration, which is the one place the two
// keywords mean different statements. The loop is a while rather than
// a for-in because for-in does not lower yet.
func TestSwitchInsideLoop(t *testing.T) {
	bin := buildSwift(t, "main", `
func main() -> Int32 {
    var i: Int32 = 0
    var total: Int32 = 0
    while i < 6 {
        i = i + 1
        switch i {
        case 3:
            // Leaves the switch, not the loop: the add below still runs.
            break
        case 5:
            // Leaves the iteration: the add below is skipped.
            continue
        default:
            total = total + 100
        }
        total = total + i
    }
    // default runs for 1, 2, 4 and 6 -> 400, and the trailing add for
    // every iteration but the fifth -> 1+2+3+4+6 = 16.
    return total - 374
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestEnumInStorageRuns: an enum with no associated values is one byte
// of tag, so it has to survive being stored — in a struct field, in a
// variable written to more than once — and come back as the same case.
// It also exercises the leading dot, which names a case of whatever
// type the context wants.
func TestEnumInStorageRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
enum Suit { case clubs, diamonds, hearts, spades }

struct Card {
    var suit: Suit
    var rank: Int32
}

func points(_ c: Card) -> Int32 {
    switch c.suit {
    case .clubs: return c.rank
    case .diamonds: return c.rank * 2
    default: return c.rank * 10
    }
}

func main() -> Int32 {
    var s = Suit.clubs
    var total: Int32 = points(Card(suit: s, rank: 1))
    s = .diamonds
    total = total + points(Card(suit: s, rank: 3))
    // .spades takes the default, so the last term is 30 rather than 3.
    s = .spades
    total = total + points(Card(suit: s, rank: 3))
    return total + 5
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestForInRangeRuns holds the counting loop to what Swift's for-in
// over a range actually does. Every expected value here was checked by
// compiling the same program with swiftc and running it.
//
// The three that are easy to get wrong: a bound is evaluated once
// rather than once per iteration, a closed range includes its upper
// bound, and `continue` in a closed range still has to reach the test
// that ends the loop.
func TestForInRangeRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
final class Counter {
    var calls = 0
    func bound() -> Int { calls = calls + 1; return 4 }
}

func main() -> Int32 {
    var t: Int32 = 0

    // The upper bound is evaluated once, not once per iteration.
    let k = Counter()
    for _ in 0..<k.bound() { t = t + 1 }
    if t != 4 { return 91 }
    if k.calls != 1 { return 92 }

    // An empty half-open range runs zero times.
    var n: Int32 = 0
    for _ in 3..<3 { n = n + 1 }
    if n != 0 { return 93 }

    // A closed range includes its upper bound: 1+2+3 = 6.
    var s = 0
    for i in 1...3 { s = s + i }
    if s != 6 { return 94 }

    // A single-element closed range runs exactly once.
    var one = 0
    for _ in 7...7 { one = one + 1 }
    if one != 1 { return 95 }

    // break leaves at the third value.
    var b = 0
    for i in 0..<100 {
        if i == 3 { break }
        b = b + 1
    }
    if b != 3 { return 96 }

    // continue skips one iteration and no more.
    var c = 0
    for i in 0..<5 {
        if i == 2 { continue }
        c = c + 1
    }
    if c != 4 { return 97 }

    // A continue in a closed range still has to ask whether that was
    // the last iteration: a loop that skipped the test would spin.
    var d = 0
    for i in 1...4 {
        if i == 4 { continue }
        d = d + i
    }
    if d != 6 { return 98 }

    // The loop variable may carry its type, and a for-in binds a name
    // of its own rather than writing to one already in scope.
    let i = 100
    var e: Int32 = 0
    for i: Int in 0..<3 { e = e + 1 }
    if e != 3 { return 99 }
    if i != 100 { return 100 }

    return 42
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestForInLabelsRun: a labelled break leaves the outer loop and a
// labelled continue goes round it, which is the only way to tell the
// two apart from inside a nested loop. swiftc run on this program
// gives 6.
func TestForInLabelsRun(t *testing.T) {
	bin := buildSwift(t, "main", `
func main() -> Int32 {
    var t: Int32 = 0
    outer: for i in 0..<4 {
        for j in 0..<4 {
            if j == 2 { continue outer }
            if i == 3 { break outer }
            t = t + 1
        }
    }
    // i = 0, 1, 2 each contribute j = 0 and j = 1; i = 3 leaves at once.
    return t
}
`, "")
	if got := runExit(t, bin); got != 6 {
		t.Errorf("exit status = %d, want 6", got)
	}
}

// TestReversedRangeTraps: `for i in 5..<3` is a mistake about the
// program, not an empty loop, and Swift kills the process for it. A
// naive `i < upper` test would run zero times and say nothing, which
// is the whole reason the check is emitted before the loop.
//
// swiftc's own binary dies on SIGTRAP with the same message.
func TestReversedRangeTraps(t *testing.T) {
	bin := buildSwift(t, "main", `
func main() -> Int32 {
    var t: Int32 = 0
    var lo = 5
    var hi = 3
    for _ in lo..<hi { t = t + 1 }
    return t
}
`, "")
	cmd := exec.Command(bin)
	err := cmd.Run()
	if err == nil {
		t.Fatalf("the program exited normally; want a trap")
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGTRAP {
		t.Errorf("exited %v, want a SIGTRAP", cmd.ProcessState)
	}
}

// TestClosuresRun exercises every closure shape that lowers: written
// inline, bound to a local and called through it, spelled with
// shorthand argument names, and a declared function used as a value.
// swiftc run on this same program gives 42.
func TestClosuresRun(t *testing.T) {
	bin := buildSwift(t, "main", `
func apply(_ f: (Int32) -> Int32, _ x: Int32) -> Int32 { return f(x) }
func combine(_ f: (Int32) -> Int32, _ g: (Int32) -> Int32, _ x: Int32) -> Int32 {
    return f(g(x))
}

func triple(_ n: Int32) -> Int32 { return n * 3 }

func main() -> Int32 {
    // A closure called directly through the local that holds it.
    let inc: (Int32) -> Int32 = { n in n + 1 }
    if inc(4) != 5 { return 91 }

    // Two closures in one function, so the naming has to tell them apart.
    let dbl: (Int32) -> Int32 = { n in n * 2 }
    if combine(inc, dbl, 10) != 21 { return 92 }

    // More than one statement, and an explicit return.
    let clamp: (Int32) -> Int32 = { n in
        if n > 10 { return 10 }
        return n
    }
    if clamp(50) != 10 { return 93 }
    if clamp(3) != 3 { return 94 }

    // Written where it is passed.
    if apply({ n in n - 1 }, 8) != 7 { return 95 }

    // A declared function is a value of function type: the same pair
    // of instructions a closure produces, with no body to emit.
    let t: (Int32) -> Int32 = triple
    if t(5) != 15 { return 96 }
    if apply(triple, 5) != 15 { return 97 }

    // $0, where the parameters are not written.
    let sq: (Int32) -> Int32 = { $0 * $0 }
    if sq(6) != 36 { return 98 }

    return 42
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestFunctionValuesAreCalledIndirectly: a closure reaches its body
// through the value that holds it, not through its name -- which is
// the whole point of one. Passing two different closures to the same
// parameter has to call two different bodies, and a lowering that
// resolved the callee statically would call one of them twice.
func TestFunctionValuesAreCalledIndirectly(t *testing.T) {
	bin := buildSwift(t, "main", `
func twice(_ f: (Int32) -> Int32, _ x: Int32) -> Int32 { return f(f(x)) }

func main() -> Int32 {
    let add3: (Int32) -> Int32 = { n in n + 3 }
    let mul3: (Int32) -> Int32 = { n in n * 3 }
    // 1+3+3 = 7, and 2*3*3 = 18, so the two parameters cannot be the
    // same body: 7 + 18 + 17.
    return twice(add3, 1) + twice(mul3, 2) + 17
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestOverrideIsNotSilentlyIgnored: a method call on a class that
// takes part in inheritance either reaches the right body or is
// refused. What it must not do is what it did -- bind the
// superclass's body statically, return the wrong number, and say
// nothing.
//
// swiftc on this program gives 5: 2 from the override through a
// base-typed local, 2 through a base-typed parameter, and 1 from the
// base itself. A diagnostic passes, because a refusal is honest; a
// clean compile that answers anything but 5 does not.
func TestOverrideIsNotSilentlyIgnored(t *testing.T) {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon; skipping the link-and-run check")
	}
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	const src = `
class A { func get() -> Int32 { return 1 } }
class B: A { override func get() -> Int32 { return 2 } }

func ask(_ a: A) -> Int32 { return a.get() }

func main() -> Int32 {
    let b: A = B()
    return b.get() + ask(B()) + ask(A())
}
`
	// The target matters: without one, lowering runs on defaults that
	// do not describe this machine and reports something unrelated,
	// which would let this test pass by mistaking that for the
	// refusal.
	_, diags := vsc.Compile([]vsc.Source{{Name: "main.swift", Text: []byte(src)}},
		vsc.Options{Module: "main", Target: target})
	if len(diags) > 0 {
		return // refused, and said so
	}
	if got := runExit(t, buildSwift(t, "main", src, "")); got != 5 {
		t.Errorf("exit status = %d, want 5: the override was ignored", got)
	}
}

// TestDynamicDispatchRuns holds dispatch to what the object is rather
// than to what the expression said.
//
// Three levels, because two do not distinguish the interesting cases:
// legs is overridden at the middle level and inherited by the bottom
// one, so Penguin's slot has to find Bird's body and not Animal's;
// noise is overridden at the bottom only; fly is introduced by the
// middle class and overridden below it, so it occupies a slot the base
// has never heard of. swiftc run on this program gives 42.
func TestDynamicDispatchRuns(t *testing.T) {
	bin := buildSwift(t, "main", `
class Animal {
    func legs() -> Int32 { return 4 }
    func noise() -> Int32 { return 1 }
}
class Bird: Animal {
    override func legs() -> Int32 { return 2 }
    func fly() -> Int32 { return 100 }
}
class Penguin: Bird {
    override func noise() -> Int32 { return 3 }
    override func fly() -> Int32 { return 0 }
}

func legsOf(_ a: Animal) -> Int32 { return a.legs() }
func noiseOf(_ a: Animal) -> Int32 { return a.noise() }
func flightOf(_ b: Bird) -> Int32 { return b.fly() }

func main() -> Int32 {
    // An override reached through the base's static type.
    if legsOf(Animal()) != 4 { return 91 }
    if legsOf(Bird()) != 2 { return 92 }
    // Inherited two levels down: Penguin does not override legs, so
    // Bird's body must be found and not Animal's.
    if legsOf(Penguin()) != 2 { return 93 }

    // A slot overridden at the bottom level only.
    if noiseOf(Animal()) != 1 { return 94 }
    if noiseOf(Bird()) != 1 { return 95 }
    if noiseOf(Penguin()) != 3 { return 96 }

    // A slot the middle class introduced, overridden below it.
    if flightOf(Bird()) != 100 { return 97 }
    if flightOf(Penguin()) != 0 { return 98 }

    // Through a local typed as the base.
    let p: Animal = Penguin()
    if p.legs() != 2 { return 99 }
    if p.noise() != 3 { return 100 }

    return 42
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestEvaluationOrder: Swift evaluates a method call's receiver before
// its arguments, and everything left to right.
//
// The receiver goes last in the argument list, because that is where
// the method convention puts self -- and gen was evaluating it last as
// well, which is a different claim. `r.recv(1).take(r.step(2))` ran
// step before recv and gave 21 where swiftc gave 12, with nothing
// said. SILGen shows the order plainly: it applies recv, then step,
// then take.
//
// Each Rec records the order its methods ran in as decimal digits, so
// the expected value is the sequence read left to right. swiftc on
// this program gives 42.
func TestEvaluationOrder(t *testing.T) {
	bin := buildSwift(t, "main", `
final class Rec {
    var seq: Int32 = 0
    func recv(_ tag: Int32) -> Rec { seq = seq * 10 + tag; return self }
    func step(_ tag: Int32) -> Int32 { seq = seq * 10 + tag; return 0 }
    func two(_ a: Int32, _ b: Int32) -> Int32 { return 0 }
    func read() -> Int32 { return seq }
}

func free2(_ a: Int32, _ b: Int32) -> Int32 { return 0 }

func main() -> Int32 {
    // The receiver, then the arguments left to right.
    let r = Rec()
    r.recv(1).two(r.step(2), r.step(3))
    if r.read() != 123 { return 91 }

    // A free function's arguments, left to right.
    let s = Rec()
    free2(s.step(1), s.step(2))
    if s.read() != 12 { return 92 }

    // Both operands of a binary operator, left to right.
    let t = Rec()
    let sum = t.step(1) + t.step(2)
    if sum != 0 { return 93 }
    if t.read() != 12 { return 94 }

    // A chain: each receiver before the next call's arguments.
    let u = Rec()
    u.recv(1).recv(2).two(u.step(3), u.step(4))
    if u.read() != 1234 { return 95 }

    // A closure's argument, before the call.
    let v = Rec()
    let f: (Int32) -> Int32 = { n in n }
    if f(v.step(7)) != 0 { return 96 }
    if v.read() != 7 { return 97 }

    return 42
}
`, "")
	if got := runExit(t, bin); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}
