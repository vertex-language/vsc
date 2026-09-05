package gen

import (
	"strings"
	"testing"
)

// What this package will not lower, it says.
//
// The bug these hold the line against was silent, and the shape of it
// is worth writing down. A constructor call returned no value and no
// diagnostic; the `return` above it saw nil and substituted the value
// a return with nothing to return produces; the module verified,
// lowered, linked, and the program exited 0 instead of computing
// anything. Nothing between the generator and the process noticed,
// because at every step the program was well-formed — just not the
// one that was written.
//
// So: every refusal reports, and no refusal is patched over with a
// substitute value.

// refusals lowers src and returns what this package said about it,
// with the parse and the check required to be clean — a diagnostic
// from either would be a different test.
func refusals(t *testing.T, src string) (string, string) {
	t.Helper()
	got, diags := generate(t, "main", src)
	var b strings.Builder
	for _, d := range diags {
		b.WriteString(d.Message)
		b.WriteString("\n")
	}
	return got, b.String()
}

// TestRefusalsAreReported: each of these lowers to nothing, and each
// has to say so.
func TestNothingIsDroppedSilently(t *testing.T) {
	// Each program's main returns something only a correct lowering
	// produces, and each uses one construct. Two outcomes are fine:
	// the construct lowers, or it is refused and said so. The third
	// is the bug — no diagnostic and nothing emitted, leaving the
	// fallback `return 0` behind in a program that compiles and runs.
	//
	// This list is written so that implementing a construct never
	// invalidates the test. A case that starts lowering moves from
	// one accepted outcome to the other and keeps passing, which is
	// the point: what is being held is the invariant, not the state
	// of the compiler on the day it was written.
	for _, c := range []struct{ name, src string }{
		{"a constructor call", `
struct P { var x: Int32; var y: Int32 }
func main() -> Int32 {
    let p = P(x: 40, y: 2)
    return p.x
}`},
		{"a class with an initializer of its own", `
final class Box {
    var n: Int32
    init(n: Int32) { self.n = n }
}
func main() -> Int32 {
    let b = Box(n: 3)
    return b.n
}`},
		{"a switch", `
enum E { case a, b }
func main() -> Int32 {
    let e = E.a
    switch e { case .a: return 1; case .b: return 2 }
}`},
		{"a for-in loop", `
func main() -> Int32 {
    var n: Int32 = 0
    for i in 0..<5 { n = n + 1 }
    return n
}`},
		{"a binding condition", `
func main() -> Int32 {
    let opt: Int32 = 3
    if let x = opt { return x }
    return 0
}`},
		{"a guard", `
func main() -> Int32 {
    guard 1 < 2 else { return 0 }
    return 1
}`},
		{"a repeat-while loop", `
func main() -> Int32 {
    var i: Int32 = 0
    repeat { i = i + 1 } while i < 3
    return i
}`},
		{"a ternary", `
func main() -> Int32 {
    let a: Int32 = 5
    return a > 3 ? 1 : 2
}`},
		{"a short circuit", `
func main() -> Int32 {
    let a: Int32 = 5
    if a > 3 && a > 4 { return 1 }
    return 2
}`},
		{"a do block", `
func main() -> Int32 {
    do { return 1 }
}`},
		{"a defer", `
func main() -> Int32 {
    defer { }
    return 1
}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, said := refusals(t, c.src)
			if said != "" {
				return // refused, and said which construct
			}
			if isOnlyTheFallback(got) {
				t.Errorf("lowered nothing and reported nothing:\n%s", got)
			}
		})
	}
}

// isOnlyTheFallback reports whether the entry point is nothing but
// the `return 0` a body that emitted nothing leaves behind.
//
// That is the signature of the bug: a well-typed Int32 zero, a module
// that verifies, and a program that runs and answers nothing anyone
// asked for.
func isOnlyTheFallback(sil string) bool {
	body, ok := entryBody(sil)
	if !ok {
		return false
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "", strings.HasPrefix(line, "bb"):
			continue
		case strings.Contains(line, "integer_literal $Builtin.Int32, 0"),
			strings.HasPrefix(line, "%1 = struct $Int32"),
			strings.HasPrefix(line, "return "):
			continue
		}
		return false // something real was emitted
	}
	return true
}

// entryBody is the text between the entry point's braces.
func entryBody(sil string) (string, bool) {
	const open = "sil [ossa] @main : $@convention(c) () -> Int32 {"
	i := strings.Index(sil, open)
	if i < 0 {
		return "", false
	}
	rest := sil[i+len(open):]
	j := strings.Index(rest, "}")
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

// TestARefusedReturnInventsNothing is the miscompile itself.
//
// `return <something this package cannot lower>` must not become
// `return 0`. In the entry point the substitute is a well-typed
// Int32, so nothing downstream can tell it apart from a program that
// meant to exit zero — which is why the check is on what was emitted
// and not on whether it verifies.
func TestARefusedReturnInventsNothing(t *testing.T) {
	got, said := refusals(t, `
final class Box {
    var n: Int32
    init(n: Int32) { self.n = n }
}
func main() -> Int32 {
    let b = Box(n: 3)
    return b.n
}`)
	if said == "" {
		t.Fatal("refused the body and said nothing")
	}
	if strings.Contains(got, "return %") {
		t.Errorf("a refused return produced a value anyway:\n%s", got)
	}
	if !strings.Contains(got, "unreachable") {
		t.Errorf("a refused return did not terminate its block honestly:\n%s", got)
	}
}

// TestAnEmptyStatementIsNotAnError: the default that catches dropped
// statements must not catch the ones that are meant to do nothing.
func TestAnEmptyStatementIsNotAnError(t *testing.T) {
	_, said := refusals(t, `
func main() -> Int32 {
    ;
    return 4
}`)
	if said != "" {
		t.Errorf("an empty statement was refused: %s", said)
	}
}

// TestWhatStillLowers guards the other direction: the refusals above
// are narrow, and what worked before them still works.
func TestWhatStillLowers(t *testing.T) {
	got, said := refusals(t, `
func fib(_ n: Int32) -> Int32 {
    if n < 2 { return n }
    return fib(n - 1) + fib(n - 2)
}
func main() -> Int32 { return fib(10) }`)
	if said != "" {
		t.Fatalf("refused a program that lowers: %s", said)
	}
	for _, want := range []string{"@main :", "apply", "cond_br"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestRefusalsCoverTheirSpan: a caret under the first byte of a
// twenty-line statement says less than one under the whole of it, and
// the node knows its extent.
func TestRefusalsCoverTheirSpan(t *testing.T) {
	src := `
func main() -> Int32 {
    var n = 0
    for x in 0..<3 where x > 1 { n = n + x }
    return Int32(n)
}`
	_, diags := generate(t, "main", src)
	if len(diags) == 0 {
		t.Fatal("no diagnostic")
	}
	if diags[0].End <= diags[0].Pos {
		t.Errorf("the diagnostic is a point, not a span: %d..%d", diags[0].Pos, diags[0].End)
	}
}

// TestTheDetectorIsNotVacuous: a test that can only pass is not a
// test. This is the module the bug actually produced — a `main` whose
// whole body is the fallback — and the check above has to call it out
// or it is guarding nothing.
func TestTheDetectorIsNotVacuous(t *testing.T) {
	dropped := `sil_stage raw

import Builtin

sil [ossa] @main : $@convention(c) () -> Int32 {
bb0:
  %0 = integer_literal $Builtin.Int32, 0
  %1 = struct $Int32 (%0)
  return %1
} // end sil function 'main'
`
	if !isOnlyTheFallback(dropped) {
		t.Error("a body that emitted nothing was not recognised as empty")
	}

	real := `sil_stage raw

import Builtin

sil [ossa] @main : $@convention(c) () -> Int32 {
bb0:
  %0 = integer_literal $Builtin.Int32, 7
  %1 = struct $Int32 (%0)
  return %1
} // end sil function 'main'
`
	if isOnlyTheFallback(real) {
		t.Error("a body that returned something was called empty")
	}
}
