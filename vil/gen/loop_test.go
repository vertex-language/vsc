package gen

import (
	"strings"
	"testing"
)

// Leaving a loop early: where break and continue branch to, and what
// happens when they name a loop that is not there.

// TestBreakAndContinueBranch: continue goes back to the header and
// break goes past the loop, which in a three-block loop means the
// body branches to bb1 and to bb3 and never falls out.
func TestBreakAndContinueBranch(t *testing.T) {
	got, diags := generate(t, "main", `
func main() -> Int32 {
    var i: Int32 = 0
    while i < 10 {
        i = i + 1
        if i == 3 { continue }
        if i == 7 { break }
    }
    return i
}`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	// The header is bb1, and continue is a branch back to it from
	// inside the body — so bb1 has more predecessors than the one
	// block that enters it.
	if !strings.Contains(got, "br bb1") {
		t.Errorf("continue did not branch to the header:\n%s", got)
	}
	if n := strings.Count(got, "br bb1"); n < 2 {
		t.Errorf("the header has %d branches to it, want the entry and at least one continue:\n%s", n, got)
	}
	if !strings.Contains(got, "cond_br") {
		t.Errorf("no loop test:\n%s", got)
	}
}

// TestLabelledBreakLeavesTheOuterLoop: the label decides which loop,
// and an unlabelled break in the same place would leave the inner
// one.
func TestLabelledBreakLeavesTheOuterLoop(t *testing.T) {
	src := `
func main() -> Int32 {
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
}`
	got, diags := generate(t, "main", src)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	if !strings.Contains(got, "cond_br") {
		t.Fatalf("no loops lowered:\n%s", got)
	}
	// Nothing here proves which block it went to by name — that is
	// what the running test in build/ is for. What this holds is that
	// the label was accepted rather than refused, and that both loops
	// are there to choose between.
	if n := strings.Count(got, "cond_br"); n < 3 {
		t.Errorf("want two loop tests and a condition, got %d:\n%s", n, got)
	}
}

// TestBreakOutsideALoop is an error about the program rather than a
// refusal about the compiler, and says so without a "yet".
func TestBreakOutsideALoop(t *testing.T) {
	_, diags := generate(t, "main", `
func main() -> Int32 {
    break
    return 0
}`)
	if len(diags) == 0 {
		t.Fatal("a break outside a loop was accepted")
	}
	msg := diags[0].Message
	if !strings.Contains(msg, "'break' is not inside a loop") {
		t.Errorf("said %q", msg)
	}
	if strings.Contains(msg, "yet") {
		t.Errorf("a break outside a loop is not a missing feature: %q", msg)
	}
}

// TestBreakNamingNoLoop: a misspelled label and a misplaced break read
// differently, so they say different things.
func TestBreakNamingNoLoop(t *testing.T) {
	_, diags := generate(t, "main", `
func main() -> Int32 {
    var i: Int32 = 0
    while i < 3 {
        i = i + 1
        break nope
    }
    return i
}`)
	if len(diags) == 0 {
		t.Fatal("a break naming no loop was accepted")
	}
	if msg := diags[0].Message; !strings.Contains(msg, "labelled 'nope'") {
		t.Errorf("said %q, want it to name the label", msg)
	}
}

// TestContinueOutsideALoop names continue rather than break.
func TestContinueOutsideALoop(t *testing.T) {
	_, diags := generate(t, "main", `
func main() -> Int32 {
    continue
    return 0
}`)
	if len(diags) == 0 {
		t.Fatal("a continue outside a loop was accepted")
	}
	if msg := diags[0].Message; !strings.Contains(msg, "'continue'") {
		t.Errorf("said %q, want it to name continue", msg)
	}
}

// Short-circuit operators, which are control flow rather than
// arithmetic.
//
// These were the third silent miscompile of the same shape: binary()
// returned nil for && without reporting, condition() passed the nil
// on, ifStmt saw it and returned, and the whole `if` — condition,
// body and all — was gone from a program that compiled and ran.

// TestShortCircuitShape is canonical SIL's, which is the one that
// makes the right operand conditional: a branch on the left, the
// right in its own block, a constant in another, and a join taking
// the answer as a parameter because the two paths produce it in
// different blocks.
func TestShortCircuitShape(t *testing.T) {
	for _, c := range []struct {
		name, src string
		konst     string
	}{
		{"and", `func main() -> Int32 {
    let a: Int32 = 3
    if a > 1 && a > 2 { return 1 }
    return 0
}`, "integer_literal $Builtin.Int1, 0"},
		{"or", `func main() -> Int32 {
    let a: Int32 = 3
    if a > 1 || a > 2 { return 1 }
    return 0
}`, "integer_literal $Builtin.Int1, 1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, diags := generate(t, "main", c.src)
			for _, d := range diags {
				t.Fatalf("gen: %s", d.Message)
			}
			// The answer arrives as a block argument: an SSA value is
			// usable only where it dominates its readers, and neither
			// path dominates the join.
			if !strings.Contains(got, "bb3(%") || !strings.Contains(got, ": $Bool)") {
				t.Errorf("the join does not take the answer as a parameter:\n%s", got)
			}
			// The operand that was skipped decided the answer: false
			// for &&, true for ||.
			if !strings.Contains(got, c.konst) {
				t.Errorf("missing %q — the skipped path has no answer:\n%s", c.konst, got)
			}
		})
	}
}

// TestShortCircuitDoesNotEvaluateBoth: two cond_brs for one `if` with
// one `&&` in it, which is the left operand's and the `if`'s. A
// lowering that evaluated both operands would have one.
func TestShortCircuitDoesNotEvaluateBoth(t *testing.T) {
	got, diags := generate(t, "main", `
func main() -> Int32 {
    let a: Int32 = 3
    if a > 1 && a > 2 { return 1 }
    return 0
}`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	if n := strings.Count(got, "cond_br"); n != 2 {
		t.Errorf("cond_br appears %d times, want 2:\n%s", n, got)
	}
}

// TestRepeatTestsAtTheEnd: the body is entered before anything is
// tested, which is the whole difference from a while loop. In the
// blocks that means the entry branches straight to the body and the
// condition is somewhere it branches back to.
func TestRepeatTestsAtTheEnd(t *testing.T) {
	got, diags := generate(t, "main", `
func main() -> Int32 {
    var i: Int32 = 0
    repeat { i = i + 1 } while i < 3
    return i
}`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	// bb0 goes to the body unconditionally: no test precedes it.
	body := strings.Index(got, "br bb1")
	test := strings.Index(got, "cond_br")
	if body < 0 || test < 0 || body > test {
		t.Errorf("the loop tests before it runs the body:\n%s", got)
	}
}

// TestGuardInvertsItsArms: the condition holding is what continues,
// and the else is the branch taken when it does not.
func TestGuardInvertsItsArms(t *testing.T) {
	got, diags := generate(t, "main", `
func main() -> Int32 {
    let a: Int32 = 5
    guard a > 1 else { return 0 }
    return a
}`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	if !strings.Contains(got, "cond_br") {
		t.Errorf("a guard without a branch:\n%s", got)
	}
}

// TestGuardMustNotFallThrough: everything after a guard may assume
// the condition held, and that is only sound because the else cannot
// reach it. Nothing before this package models the rule.
func TestGuardMustNotFallThrough(t *testing.T) {
	_, diags := generate(t, "main", `
func main() -> Int32 {
    let a: Int32 = 5
    guard a > 1 else { let x: Int32 = 0 }
    return a
}`)
	if len(diags) == 0 {
		t.Fatal("a guard whose else falls through was accepted")
	}
	msg := diags[0].Message
	if !strings.Contains(msg, "must not fall through") {
		t.Errorf("said %q", msg)
	}
	if strings.Contains(msg, "yet") {
		t.Errorf("this is the language's rule, not a missing feature: %q", msg)
	}
}

// TestConditionalJoinsTwoArms: `a ? b : c` produces the answer in two
// different blocks, so the join takes it as a parameter for the same
// reason && and || do.
func TestConditionalJoinsTwoArms(t *testing.T) {
	got, diags := generate(t, "main", `
func main() -> Int32 {
    let a: Int32 = 5
    return a > 3 ? 10 : 20
}`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	if !strings.Contains(got, ": $Int32)") {
		t.Errorf("the join does not take the answer as a parameter:\n%s", got)
	}
	for _, want := range []string{"integer_literal $Builtin.Int32, 10", "integer_literal $Builtin.Int32, 20"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q — an arm was not lowered:\n%s", want, got)
		}
	}
}
