package pass

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/vil"
	"github.com/vertex-language/vsc/vil/gen"
	"github.com/vertex-language/vsc/vil/text"
)

// mandatory lowers src and runs the mandatory passes over it,
// returning the SIL text of what came out.
//
// The oracle tests' canonical() takes a corpus path and goes further,
// through LowerOwnership; this stops where Mandatory does, because
// what these tests are about is the shape it leaves behind.
func mandatory(t *testing.T, src string) string {
	t.Helper()
	f := token.NewFile("t.swift", []byte(src))
	file, ds := parser.ParseFile(f, 0)
	for _, d := range ds {
		t.Fatalf("parse: %s", d.Print(f))
	}
	info, cs := analyzer.Check([]*ast.File{file})
	for _, d := range cs {
		t.Fatalf("check: %s", d.Print(f))
	}
	m, gs := gen.File("t", file, info)
	for _, d := range gs {
		t.Fatalf("gen: %s", d.Print(f))
	}
	if err := Mandatory(m); err != nil {
		t.Fatalf("mandatory: %v", err)
	}
	if got := m.Stage(); got != vil.StageCanonical {
		t.Fatalf("stage is %s, want canonical", got)
	}
	var buf bytes.Buffer
	if err := text.Print(&buf, m); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

const varProgram = `
func counter(_ k: Int32) -> Int32 {
    var n: Int32 = 0
    n = n + k
    return n
}
`

// TestBoxBecomesAStackSlot is allocbox-to-stack against the oracle's
// answer.
//
// `swiftc -emit-silgen` writes alloc_box, begin_borrow [var_decl] and
// project_box for this function; `swiftc -emit-sil` writes
// alloc_stack and dealloc_stack and none of the other three. This is
// the pass that gets from the first to the second.
func TestBoxBecomesAStackSlot(t *testing.T) {
	got := mandatory(t, varProgram)

	for _, gone := range []string{"alloc_box", "project_box", "begin_borrow", "end_borrow", "destroy_value"} {
		if strings.Contains(got, gone) {
			t.Errorf("%s survived the pass:\n%s", gone, got)
		}
	}
	for _, want := range []string{"alloc_stack", "dealloc_stack"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %s after the pass:\n%s", want, got)
		}
	}
}

// TestTheSlotIsWhatTheBoxHeld: the readers have to survive the
// rewrite pointing at the slot, or the variable is written in one
// place and read from another.
func TestTheSlotIsWhatTheBoxHeld(t *testing.T) {
	got := mandatory(t, varProgram)
	// %0 is the parameter, so the slot is %1: every reader of what
	// the box held has to name it and no other value.
	for _, want := range []string{
		"%1 = alloc_stack",
		"store %3 to [trivial] %1",
		"begin_access [read] [unknown] %1",
		"begin_access [modify] [unknown] %1",
		"dealloc_stack %1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestAssignBecomesAStore is the half of definite initialization this
// package does: a trivial type owns nothing, so there is nothing to
// destroy and the answer needs no dataflow.
func TestAssignBecomesAStore(t *testing.T) {
	got := mandatory(t, varProgram)
	if strings.Contains(got, "assign") {
		t.Errorf("an assign to a trivial location survived:\n%s", got)
	}
	if strings.Count(got, "store") < 2 {
		t.Errorf("the assignment did not become a store:\n%s", got)
	}
}

// TestMandatoryVerifies: the pass has to leave a module the verifier
// still accepts, which is why Mandatory runs it again afterwards.
// mandatory() would have failed already if it did not — this says so
// on purpose, since that is the property and not a side effect.
func TestMandatoryVerifies(t *testing.T) {
	mandatory(t, varProgram)
}

// TestAWhileLoopSurvives: three blocks, and the body branching back
// to the header rather than forward.
func TestAWhileLoopSurvives(t *testing.T) {
	got := mandatory(t, `
func total(_ k: Int32) -> Int32 {
    var n: Int32 = 0
    var i: Int32 = 0
    while i < k {
        n = n + i
        i = i + 1
    }
    return n
}
`)
	for _, want := range []string{"br bb1", "cond_br", "bb3"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Two variables, two slots, and both freed. Counted as "= alloc_stack"
	// because "dealloc_stack" contains "alloc_stack".
	if n := strings.Count(got, "= alloc_stack"); n != 2 {
		t.Errorf("alloc_stack appears %d times, want 2:\n%s", n, got)
	}
	if n := strings.Count(got, "dealloc_stack"); n != 2 {
		t.Errorf("dealloc_stack appears %d times, want 2:\n%s", n, got)
	}
	// Freed in reverse order of allocation, which is the stack
	// discipline SIL asks for.
	if i, j := strings.Index(got, "dealloc_stack %4"), strings.Index(got, "dealloc_stack %1"); i < 0 || j < 0 || i > j {
		t.Errorf("the slots are not freed in reverse order:\n%s", got)
	}
}
