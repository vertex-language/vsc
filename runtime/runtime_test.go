package runtime_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vertex-language/ir"
	irtext "github.com/vertex-language/ir/text"
	irverify "github.com/vertex-language/ir/verify"

	"github.com/vertex-language/vsc/runtime"
)

func build(t *testing.T, prefix string) *ir.Module {
	t.Helper()
	m, err := runtime.Module(ir.AArch64MacOS, runtime.Options{SymbolPrefix: prefix})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return m
}

func text(t *testing.T, m *ir.Module) string {
	t.Helper()
	var buf bytes.Buffer
	if err := irtext.Print(&buf, m); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestTheRuntimeVerifies is the whole claim: the runtime is a VIR
// module this compiler builds for itself, so it is held to the same
// rules as anything else it emits.
func TestTheRuntimeVerifies(t *testing.T) {
	if err := irverify.Module(build(t, "_")); err != nil {
		t.Errorf("%v\n\n%s", err, text(t, build(t, "_")))
	}
}

// TestTheSymbolsAreExported: a program in another object refers to
// these by name, so an internal one would compile and fail to link.
func TestTheSymbolsAreExported(t *testing.T) {
	got := text(t, build(t, "_"))
	for _, want := range []string{
		"export func @_vertex_alloc",
		"export func @_vertex_retain",
		"export func @_vertex_release",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestThePrefixIsTheTarget's: the runtime's definitions and the
// program's calls to them have to agree about the underscore, and
// they agree by being given the same answer rather than by each
// working it out.
func TestThePrefixIsTheTargets(t *testing.T) {
	bare := text(t, build(t, ""))
	if strings.Contains(bare, "@_vertex_alloc") {
		t.Errorf("a prefix was applied where the target wants none:\n%s", bare)
	}
	if !strings.Contains(bare, "@vertex_alloc") {
		t.Errorf("the symbol is missing entirely:\n%s", bare)
	}
	// The platform's own symbols take the prefix too: a link against
	// libSystem asks for _malloc.
	if got := text(t, build(t, "_")); !strings.Contains(got, "_malloc") {
		t.Errorf("malloc was not prefixed:\n%s", got)
	}
}

// TestReleaseFreesOnlyTheLastReference: the branch is the whole of
// the logic, so the shape of it is worth holding — a decrement, a
// comparison against one, and a call to free on one side only.
func TestReleaseFreesOnlyTheLastReference(t *testing.T) {
	got := text(t, build(t, "_"))
	release := got[strings.Index(got, "@_vertex_release"):]
	if i := strings.Index(release, "export func"); i > 0 {
		release = release[:i]
	}
	for _, want := range []string{"atomic_rmwsub", "brif", "_free"} {
		if !strings.Contains(release, want) {
			t.Errorf("missing %q in release:\n%s", want, release)
		}
	}
	// Freeing on both edges would free every reference, not the last.
	if n := strings.Count(release, "_free"); n != 1 {
		t.Errorf("free is called %d times, want 1:\n%s", n, release)
	}
}

// TestAllocStartsAtOneReference: the caller of vertex_alloc holds the
// reference it gets back, so an object that starts at zero is one
// that the first release frees while it is still in use.
func TestAllocStartsAtOneReference(t *testing.T) {
	got := text(t, build(t, "_"))
	alloc := got[strings.Index(got, "@_vertex_alloc"):]
	if i := strings.Index(alloc, "export func"); i > 0 {
		alloc = alloc[:i]
	}
	if !strings.Contains(alloc, "i64.const 1") {
		t.Errorf("the count does not start at one:\n%s", alloc)
	}
	if !strings.Contains(alloc, "_malloc") {
		t.Errorf("nothing was allocated:\n%s", alloc)
	}
}
