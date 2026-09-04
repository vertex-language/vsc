package analyzer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
)

// The checker's oracle is the same one the parser has: Swift itself.
//
// tests/check holds programs written inside what this checker models
// — no standard library beyond the builtin types — and named for the
// verdict they carry. An `ok-` program is one Swift accepts, and this
// checker must find nothing wrong with it; a `bad-` program is one
// Swift rejects, and this checker must reject it too.
//
// Both halves matter, and the first half matters more: a checker that
// reports what it does not understand is worse than one that stays
// quiet, because every wrong diagnostic is a program the compiler
// refuses to build.

// checkFile parses and checks one file, returning its diagnostics.
func checkFile(t *testing.T, path string) []string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f := token.NewFile(path, src)
	file, diags := parser.ParseFile(f, 0)
	for _, d := range diags {
		t.Errorf("parser: %s", d.Print(f))
	}
	_, checkDiags := Check([]*ast.File{file})
	msgs := make([]string, 0, len(checkDiags))
	for _, d := range checkDiags {
		if d.Severity == token.Error {
			msgs = append(msgs, d.Print(f))
		}
	}
	return msgs
}

// TestCheckCorpus runs the semantic corpus. The verdicts are the
// file names, so the test runs with no toolchain installed;
// TestCheckAgreesWithSwiftc is what keeps the names honest.
func TestCheckCorpus(t *testing.T) {
	files, _ := filepath.Glob("../tests/check/*.swift")
	if len(files) == 0 {
		t.Skip("no semantic corpus")
	}
	for _, path := range files {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			msgs := checkFile(t, path)
			switch {
			case strings.HasPrefix(name, "ok-") && len(msgs) > 0:
				t.Errorf("Swift accepts this program; the checker reported:\n  %s",
					strings.Join(msgs, "\n  "))
			case strings.HasPrefix(name, "bad-") && len(msgs) == 0:
				t.Errorf("Swift rejects this program; the checker found nothing wrong")
			}
		})
	}
}

// TestCheckAgreesWithSwiftc holds the corpus's names to Swift's own
// verdicts, so that a program cannot be filed under the wrong half.
func TestCheckAgreesWithSwiftc(t *testing.T) {
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH")
	}
	files, _ := filepath.Glob("../tests/check/*.swift")
	if len(files) == 0 {
		t.Skip("no semantic corpus")
	}
	for _, path := range files {
		name := filepath.Base(path)
		accepted := exec.Command(swiftc, "-typecheck", path).Run() == nil
		switch {
		case strings.HasPrefix(name, "ok-") && !accepted:
			t.Errorf("%s: swiftc rejects it, so it is not an accepting case", name)
		case strings.HasPrefix(name, "bad-") && accepted:
			t.Errorf("%s: swiftc accepts it, so it is not a rejecting case", name)
		}
	}
}

// TestEveryExpressionIsVisited holds the checker to walking all of a
// program. Not every expression can be given a type — a member of a
// type this compiler does not model is Invalid, and says so — but
// every one must be looked at, because an expression the checker
// never reaches is a region of the program nothing downstream can be
// built from.
//
// The exception is an operator's own symbol. A SequenceExpr holds its
// operators as expressions so that the tree says what was written;
// they are not values and have no type.
func TestEveryExpressionIsVisited(t *testing.T) {
	files, _ := filepath.Glob("../tests/check/ok-*.swift")
	if len(files) == 0 {
		t.Skip("no semantic corpus")
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f := token.NewFile(path, src)
			file, _ := parser.ParseFile(f, 0)
			info, _ := Check([]*ast.File{file})

			ast.Inspect(file, func(n ast.Node) bool {
				e, ok := n.(ast.Expr)
				if !ok {
					return true
				}
				if _, isOp := e.(*ast.OperatorExpr); isOp {
					return true
				}
				if _, found := info.Types[e]; !found {
					t.Errorf("%T at %s was never checked", e, f.Position(e.Pos()))
				}
				return true
			})
		})
	}
}

// TestCheckCorpusDoesNotCrash runs the checker over every file in
// tests/. Those programs use names this checker cannot resolve yet —
// the standard library is not modelled — so what is required here is
// not a clean check but a finished one: a front end that gives up on
// a construct says so with a diagnostic, never with a panic.
func TestCheckCorpusDoesNotCrash(t *testing.T) {
	files, err := filepath.Glob("../tests/*.swift")
	if err != nil || len(files) == 0 {
		t.Skip("no corpus")
	}
	for _, name := range files {
		t.Run(filepath.Base(name), func(t *testing.T) {
			src, err := os.ReadFile(name)
			if err != nil {
				t.Skip(err)
			}
			f := token.NewFile(name, src)
			file, diags := parser.ParseFile(f, 0)
			for _, d := range diags {
				t.Errorf("parser diag: %s", d.Print(f))
			}
			Check([]*ast.File{file})
		})
	}
}
