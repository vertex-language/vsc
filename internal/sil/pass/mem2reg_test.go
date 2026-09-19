package pass

import (
	"strings"
	"testing"
)

// optimized is src through the mandatory passes and Optimize, as SIL text.
func optimized(t *testing.T, src string) string {
	t.Helper()
	m := canonicalOf(t, src)
	if err := Optimize(m); err != nil {
		t.Fatalf("optimize: %v", err)
	}
	return printed(t, m)
}

// TestLoopCountersBecomeValues: the counter and the running total of a
// while loop are trivial vars that are only loaded and stored, so neither
// is left in memory; the loop header takes both as block arguments.
func TestLoopCountersBecomeValues(t *testing.T) {
	got := optimized(t, `
func total(_ n: Int) -> Int {
    var sum = 0
    var i = 0
    while i < n {
        sum = sum + i
        i += 1
    }
    return sum
}
`)
	for _, gone := range []string{"alloc_stack", "dealloc_stack", "begin_access", "load", "store"} {
		if strings.Contains(got, gone) {
			t.Errorf("%s survived promotion:\n%s", gone, got)
		}
	}
	if !strings.Contains(got, "(%") || !strings.Contains(got, ": $Int, %") {
		t.Errorf("no block takes the two counters as arguments:\n%s", got)
	}
}

// TestAnEscapingSlotStays: a var passed inout has its address taken, and
// what the callee does with it is not something promotion can follow.
func TestAnEscapingSlotStays(t *testing.T) {
	got := optimized(t, `
func bump(_ x: inout Int) { x += 1 }

func twice() -> Int {
    var n = 0
    bump(&n)
    bump(&n)
    return n
}
`)
	if !strings.Contains(got, "alloc_stack $Int") {
		t.Errorf("the inout slot was promoted:\n%s", got)
	}
}

// TestANonTrivialSlotStays: a String var copies and destroys what it
// holds, which a block argument cannot do for it.
func TestANonTrivialSlotStays(t *testing.T) {
	got := optimized(t, `
func pick(_ b: Bool) -> String {
    var s = "a"
    if b {
        s = "b"
    }
    return s
}
`)
	if !strings.Contains(got, "alloc_stack $String") {
		t.Errorf("the String slot was promoted:\n%s", got)
	}
}

// TestOnlyOneArmStores: a value set on one arm of an if and read after
// it meets the value from before the if at the join.
func TestOnlyOneArmStores(t *testing.T) {
	got := optimized(t, `
func clamp(_ x: Int) -> Int {
    var y = x
    if y < 0 {
        y = 0
    }
    return y
}
`)
	if strings.Contains(got, "alloc_stack") {
		t.Errorf("the slot survived:\n%s", got)
	}
	if !strings.Contains(got, "br bb3(%0)") {
		t.Errorf("the path that did not store does not pass x on:\n%s", got)
	}
}
