package lower

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/internal/sil/gen"
	"github.com/vertex-language/vsc/internal/sil/pass"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/token"
)

// planOf is planAsync for a test: the plan wants a lowerer for the
// question of which results come back through storage, and a bare one
// answers it the same way for every target this file uses.
func planOf(t *testing.T, f *sil.Func) *asyncPlan {
	t.Helper()
	p, err := planAsync(&lowerer{}, f)
	if err != nil {
		t.Fatalf("plan %s: %v", f.Name(), err)
	}
	return p
}

// awaits is a plan's suspensions that are the function's own awaits,
// without the hops the generator puts around them: to where the
// function runs on entry and after each await, which are suspensions
// too, on the runtime's vertex_task_hop. The tests here are about what
// an await keeps and frees, so they look past the hops.
func awaits(p *asyncPlan) []*suspension {
	var out []*suspension
	for _, s := range p.suspends {
		if args := s.at.Args(); len(args) > 0 && args[0] != nil && args[0].Inst() != nil &&
			args[0].Inst().Aux().Name == stdlib.TaskHop {
			continue
		}
		out = append(out, s)
	}
	return out
}

// silOf lowers a source string to SIL, the way lowered() does for a file.
func silOf(t *testing.T, src string) *sil.Module {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "a.vs")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	f := token.NewFile(path, []byte(src))
	file, diags := parser.ParseFile(f, 0)
	for _, d := range diags {
		t.Fatalf("parse: %s", d.Print(f))
	}
	info, checks := analyzer.Check([]*ast.File{file})
	for _, d := range checks {
		t.Fatalf("check: %s", d.Print(f))
	}
	m, gens := gen.File("t", file, info)
	for _, d := range gens {
		t.Fatalf("gen: %s", d.Print(f))
	}
	if err := pass.Mandatory(m); err != nil {
		t.Fatalf("mandatory: %v", err)
	}
	if err := pass.LowerOwnership(m); err != nil {
		t.Fatal(err)
	}
	return m
}

func funcNamed(t *testing.T, m *sil.Module, source string) *sil.Func {
	t.Helper()
	for _, f := range m.Funcs() {
		if f.SourceName() == source {
			return f
		}
	}
	t.Fatalf("no function called %q", source)
	return nil
}

// TestPlanAsyncFindsSuspensions: a call to an async function gives up
// the thread and a call to an ordinary one does not, however long it
// takes. That is the rule `await` stands for.
func TestPlanAsyncFindsSuspensions(t *testing.T) {
	m := silOf(t, `
func plain(_ n: int) -> int { return n + 1 }
func inner(_ n: int) async -> int { return n + 1 }

func none(_ n: int) async -> int { return plain(n) }
func one(_ n: int) async -> int { return await inner(n) }
func two(_ n: int) async -> int {
    let a = await inner(n)
    let b = await inner(a)
    return b
}
`)
	for _, c := range []struct {
		fn   string
		want int
	}{
		{"none", 0},
		{"one", 1},
		{"two", 2},
	} {
		p := planOf(t, funcNamed(t, m, c.fn))
		if p == nil {
			t.Fatalf("%s: no plan for an async function", c.fn)
		}
		if got := len(awaits(p)); got != c.want {
			t.Errorf("%s: %d suspensions, want %d", c.fn, got, c.want)
		}
		if p.hasSuspensions() != (c.want > 0) {
			t.Errorf("%s: hasSuspensions = %v", c.fn, p.hasSuspensions())
		}
	}
}

// TestPlanAsyncSkipsOrdinaryFunctions: a function that is not async has
// no plan at all, which is what keeps this off every other function in
// the module.
func TestPlanAsyncSkipsOrdinaryFunctions(t *testing.T) {
	m := silOf(t, `func plain(_ n: int) -> int { return n + 1 }`)
	if p := planOf(t, funcNamed(t, m, "plain")); p != nil {
		t.Errorf("an ordinary function got a plan: %+v", p)
	}
}

// TestLiveAcrossSuspension is the analysis that matters: a value
// computed before an await and used after it has to survive, and one
// that is finished with before it must not take up room in the frame.
//
// The counts are exact on purpose. A live set that is merely non-empty
// would pass while holding everything in the function, which is the
// mistake this is here to catch: it would still run, and every frame
// would be bigger than it needs to be forever.
func TestLiveAcrossSuspension(t *testing.T) {
	m := silOf(t, `
func inner(_ n: int) async -> int { return n + 1 }

// Nothing is wanted after the await.
func nothingCrosses(_ a: int) async -> int { return await inner(a) }

// x is wanted after it; d is finished with before it.
func oneCrosses(_ a: int, _ b: int) async -> int {
    let x = a * 2
    let d = b * 3
    let y = await inner(d)
    return x + y
}

// Two are.
func twoCross(_ a: int, _ b: int) async -> int {
    let x = a * 2
    let z = b * 7
    let y = await inner(a)
    return x + y + z
}
`)
	for _, c := range []struct {
		fn   string
		want int
	}{
		{"nothingCrosses", 0},
		{"oneCrosses", 1},
		{"twoCross", 2},
	} {
		p := planOf(t, funcNamed(t, m, c.fn))
		if len(awaits(p)) != 1 {
			t.Fatalf("%s: %d suspensions, want 1", c.fn, len(awaits(p)))
		}
		if got := len(awaits(p)[0].live); got != c.want {
			t.Errorf("%s: %d values live across the await, want %d", c.fn, got, c.want)
		}
	}

	p := planOf(t, funcNamed(t, m, "oneCrosses"))
	live := awaits(p)[0].live
	// Whatever the live set is, the frame has to be big enough to hold
	// it past the header, and every live value must have a slot.
	for _, v := range live {
		off, ok := p.slot[v]
		if !ok {
			t.Errorf("a live value has no slot: %v", v.Type().Formal())
			continue
		}
		if off < int64(stdlib.AsyncContextBytes) {
			t.Errorf("slot %d is inside the context header", off)
		}
		if off >= p.size {
			t.Errorf("slot %d is past the end of a %d-byte context", off, p.size)
		}
	}
}

// TestFrameSlotsDoNotOverlap: two values live at the same time are in
// different places, which is the one way this analysis can be wrong that
// nothing else would catch.
func TestFrameSlotsDoNotOverlap(t *testing.T) {
	m := silOf(t, `
func inner(_ n: int) async -> int { return n + 1 }

func several(_ a: int, _ b: int, _ c: int) async -> int {
    let x = a * 2
    let y = b * 3
    let z = c * 5
    let got = await inner(a)
    return x + y + z + got
}
`)
	p := planOf(t, funcNamed(t, m, "several"))
	if len(awaits(p)) != 1 {
		t.Fatalf("%d suspensions, want 1", len(awaits(p)))
	}
	type span struct{ lo, hi int64 }
	var spans []span
	for _, v := range awaits(p)[0].live {
		off, ok := p.slot[v]
		if !ok {
			t.Fatalf("a live value has no slot")
		}
		size, _ := frameSlot(v)
		spans = append(spans, span{off, off + size})
	}
	for i := range spans {
		for j := i + 1; j < len(spans); j++ {
			a, b := spans[i], spans[j]
			if a.lo < b.hi && b.lo < a.hi {
				t.Errorf("slots [%d,%d) and [%d,%d) overlap", a.lo, a.hi, b.lo, b.hi)
			}
		}
	}
	// The callee's context slot is its own, and past the live values.
	cs := p.suspends[0].calleeSlot
	for _, s := range spans {
		if cs < s.hi && s.lo < cs+8 {
			t.Errorf("the callee context slot at %d overlaps a live value [%d,%d)", cs, s.lo, s.hi)
		}
	}
}

// TestCalleeSlotsArePerSuspension: each call keeps its own context
// while it runs, because the funclet that comes back has to free that
// call's frame and no other.
func TestCalleeSlotsArePerSuspension(t *testing.T) {
	m := silOf(t, `
func inner(_ n: int) async -> int { return n + 1 }
func twice(_ n: int) async -> int {
    let a = await inner(n)
    let b = await inner(a)
    return b
}
`)
	p := planOf(t, funcNamed(t, m, "twice"))
	calls := awaits(p)
	if len(calls) != 2 {
		t.Fatalf("%d suspensions, want 2", len(calls))
	}
	if calls[0].calleeSlot == calls[1].calleeSlot {
		t.Errorf("two calls share one context slot at %d", calls[0].calleeSlot)
	}
	for i, s := range p.suspends {
		if s.index != i {
			t.Errorf("suspension %d is numbered %d", i, s.index)
		}
	}
}

// TestContextIsAtLeastItsHeader: a function that suspends has a context
// of at least the two header words, aligned so anything may live in it.
func TestContextIsAtLeastItsHeader(t *testing.T) {
	m := silOf(t, `
func inner(_ n: int) async -> int { return n + 1 }
func one(_ n: int) async -> int { return await inner(n) }
`)
	p := planOf(t, funcNamed(t, m, "one"))
	if p.size < int64(stdlib.AsyncContextBytes) {
		t.Errorf("a context of %d bytes is smaller than its header", p.size)
	}
	if p.size%16 != 0 {
		t.Errorf("a context of %d bytes is not 16-byte aligned", p.size)
	}
}

// TestSuspensionOnACallThatMayFail: `try await` is a try_apply, and it
// gives up the thread exactly as an apply of the same function does.
//
// It is a separate case because the answer arrives somewhere else: an
// apply has a result, a try_apply has two edges and the answer is the
// argument of one of them. Reading it off the apply would find nothing
// and carry nothing across, which links and loses the value.
func TestSuspensionOnACallThatMayFail(t *testing.T) {
	m := silOf(t, `
struct Bad: Error { var code: int }

func inner(_ n: int) async throws -> int {
    if n < 0 { throw Bad(code: n) }
    return n + 1
}

func outer(_ n: int) async throws -> int {
    return (try await inner(n)) + 1
}
`)
	p := planOf(t, funcNamed(t, m, "outer"))
	if len(awaits(p)) != 1 {
		t.Fatalf("%d suspensions, want 1", len(awaits(p)))
	}
	s := awaits(p)[0]
	if !s.throws {
		t.Error("a try await is not marked as a call that may fail")
	}
	if s.normal == nil || s.failed == nil {
		t.Fatal("a call that may fail with fewer than both edges")
	}
	if s.errSlot < int64(stdlib.AsyncContextBytes) || s.errSlot >= p.size {
		t.Errorf("the error slot at %d is not inside a %d-byte context", s.errSlot, p.size)
	}
	// What the call answers with is the edge's argument, and it has a
	// place to arrive in.
	if len(s.results) != 1 {
		t.Fatalf("%d results, want the one the normal edge carries", len(s.results))
	}
	if _, ok := p.slot[s.results[0]]; !ok {
		t.Error("the answer has no slot")
	}
}

// TestFrameHoldsWhatTheStackCannot: a variable the source declared is an
// alloc_stack, and in a function that gives up its thread it cannot live
// on the machine stack -- that stack is gone at every suspension and
// something else is standing on it at every resume.
//
// So the plan puts it in the frame. Getting this wrong reads whatever
// the executor left behind, which is a wrong answer rather than a crash,
// and only shows up once the callee is deep enough to have written over
// it.
func TestFrameHoldsWhatTheStackCannot(t *testing.T) {
	m := silOf(t, `
func inner(_ n: int) async -> int { return n + 1 }

func counting(_ n: int) async -> int {
    var total = 0
    var i = 0
    while i < n {
        total = total + (await inner(i))
        i = i + 1
    }
    return total
}
`)
	p := planOf(t, funcNamed(t, m, "counting"))
	if len(p.storage) != 2 {
		t.Fatalf("%d pieces of storage in the frame, want the two variables", len(p.storage))
	}
	for v, off := range p.storage {
		if off < int64(stdlib.AsyncContextBytes) {
			t.Errorf("storage at %d is inside the context header", off)
		}
		if off >= p.size {
			t.Errorf("storage at %d is past the end of a %d-byte context", off, p.size)
		}
		if p.kind[v] != storageStack {
			t.Errorf("a variable is held as %v, want an alloc_stack", p.kind[v])
		}
		// It is reached by address, so it needs no slot of its own.
		if _, slotted := p.slot[v]; slotted {
			t.Error("a value in frame storage was given a slot as well")
		}
	}
}

// TestADeclarationHasNoPlan: a function this module only declares has no
// body to split and no frame this module can work out. Answering with
// the header would be a guess, and the wrong one for anything that
// suspends -- the size comes from the record beside it instead.
func TestADeclarationHasNoPlan(t *testing.T) {
	m := silOf(t, `
@_silgen_name("elsewhere")
func elsewhere(_ n: int) async -> int

func here(_ n: int) async -> int { return await elsewhere(n) }
`)
	for _, f := range m.Funcs() {
		if !f.IsDeclaration() {
			continue
		}
		if p := planOf(t, f); p != nil {
			t.Errorf("%s is only declared and got a plan of %d bytes", f.Name(), p.size)
		}
	}
}
