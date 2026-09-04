package mangle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// TestAgreesWithSwiftc is this package's whole claim: for a program
// both compilers can read, the symbols are the same string.
//
// A mangling that is merely plausible is worse than none, because it
// links -- to the wrong thing, or to nothing, and either way the
// failure surfaces a long way from here. So the test is exact
// equality against the names swiftc puts in its own SIL.
func TestAgreesWithSwiftc(t *testing.T) {
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH")
	}
	files, err := filepath.Glob("testdata/*.swift")
	if err != nil || len(files) == 0 {
		t.Skip("no corpus")
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// The module name swiftc uses is the file's base name, so
			// ours has to be the same for the symbols to be.
			module := strings.TrimSuffix(filepath.Base(path), ".swift")
			theirs := swiftcSymbols(t, swiftc, path, module)
			ours := ourSymbols(t, path, src, module)

			if len(theirs) == 0 {
				t.Fatalf("nothing to compare: swiftc named no top-level function")
			}
			t.Logf("%d symbols", len(theirs))
			for name, want := range theirs {
				got, ok := ours[name]
				if !ok {
					t.Errorf("%s: no symbol (swiftc: %s)", name, want)
					continue
				}
				if got != want {
					t.Errorf("%s:\n ours: %s\n them: %s", name, got, want)
				}
			}
		})
	}
}

// ourSymbols mangles every function the checker found, by name.
func ourSymbols(t *testing.T, path string, src []byte, module string) map[string]string {
	t.Helper()
	f := token.NewFile(path, src)
	file, diags := parser.ParseFile(f, 0)
	for _, d := range diags {
		t.Fatalf("parse: %s", d.Print(f))
	}
	info, checks := analyzer.Check([]*ast.File{file})
	for _, d := range checks {
		t.Fatalf("check: %s", d.Print(f))
	}

	out := make(map[string]string)
	for _, stmt := range file.Stmts {
		decl, ok := stmt.(*ast.DeclStmt)
		if !ok {
			continue
		}
		fn, ok := decl.D.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		name := string(f.Slice(fn.Name.Pos(), fn.Name.End()))
		sym, ok := info.Defs[fn.Name]
		if !ok {
			continue
		}
		fs, ok := sym.(*analyzer.FuncSymbol)
		if !ok {
			continue
		}
		got, err := mangle.Function(mangle.Decl{
			Module:    module,
			Name:      name,
			Signature: fs.Signature(),
		})
		if err != nil {
			t.Logf("%s: %v", name, err)
			continue
		}
		out[name] = got
	}
	return out
}

// swiftcSymbols reads the mangled name of each top-level function out
// of swiftc's own SIL, keyed by the source name.
func swiftcSymbols(t *testing.T, swiftc, path, module string) map[string]string {
	t.Helper()
	out, err := exec.Command(swiftc, "-emit-sil", path).Output()
	if err != nil {
		t.Skipf("swiftc: %v", err)
	}
	syms := make(map[string]string)
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "sil ") {
			continue
		}
		i := strings.Index(line, "@$s")
		if i < 0 {
			continue
		}
		sym := line[i+1:]
		if j := strings.IndexAny(sym, " :"); j >= 0 {
			sym = sym[:j]
		}
		// The demangler is the only thing that knows which source name
		// a symbol belongs to, so ask it.
		if name, ok := sourceName(t, sym, module); ok {
			syms[name] = sym
		}
	}
	return syms
}

// sourceName asks the demangler what a symbol is called, and keeps
// only the plain top-level functions of the module under test.
func sourceName(t *testing.T, sym, module string) (string, bool) {
	t.Helper()
	out, err := exec.Command("swift", "demangle", "--compact", sym).Output()
	if err != nil {
		return "", false
	}
	s := strings.TrimSpace(string(out))
	if i := strings.Index(s, "---> "); i >= 0 {
		s = s[i+len("---> "):]
	}
	prefix := module + "."
	if !strings.HasPrefix(s, prefix) {
		return "", false
	}
	s = s[len(prefix):]
	i := strings.Index(s, "(")
	if i <= 0 {
		return "", false
	}
	name := s[:i]
	// Nested things -- methods, accessors, initializers -- have a dot
	// in what is left, and are not what this test covers yet.
	if strings.ContainsAny(name, ". ") {
		return "", false
	}
	return name, true
}

var _ = types.Typ
